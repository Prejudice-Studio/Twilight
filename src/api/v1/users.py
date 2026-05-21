"""
用户 API

提供用户相关的 CRUD 操作
"""

import json
import asyncio
import hmac
import logging
import re
from pathlib import Path
from urllib.parse import urlparse
from typing import Any
import time as _time
import secrets as _secrets
import string as _string
from flask import Blueprint, request, g, send_file

from src.api.v1.auth import require_auth, api_response
from src.core.request_utils import get_real_client_ip
from src.db.user import UserOperate, Role, TelegramBindCodeOperate
from src.core.utils import parse_bool
from src.services import UserService

logger = logging.getLogger(__name__)

users_bp = Blueprint("users", __name__, url_prefix="/users")


# ==================== 注册绑定码轮询：404 / IP 防滥用 ====================
# 公开轮询接口 /telegram/register/bind-code/status 容易被脚本反复请求一个
# 已过期 / 不存在的 code（每次都打 DB）。为了既不影响正常前端 2s 轮询，
# 又能拦住"明知会 404 还狂打"的客户端，这里维护两份模块级缓存：
#
# 1) `_INVALID_CODE_CACHE`: code → 失效时间戳；第一次 DB 查到 404 后写入，
#    TTL 内同 code 直接 404，不再消费限速配额、不再查 DB。
# 2) `_IP_404_BAN`:        ip   → 解封时间戳；同 IP 60s 内累计 ``_IP_404_BAN_THRESHOLD``
#    次 404，整 IP 进短期封禁名单（``_IP_404_BAN_DURATION`` 秒），期间任何
#    请求该端点直接 429，不消费限速、不查 DB。
#
# 状态全局保存在进程内存里，重启清空；多 worker 部署下每个进程独立计数，
# 但因为攻击者通常长链接打同一进程，单进程就足以挡住。
_INVALID_CODE_TTL = 300  # 已知失效 code 的缓存秒数
_INVALID_CODE_CACHE_MAX = 2048  # 缓存大小上限，防止恶意写满
_IP_404_THRESHOLD = 60  # 60s 内同 IP 累计 404 阈值
_IP_404_WINDOW = 60  # 计数窗口（秒）
_IP_404_BAN_DURATION = 300  # 触发后的封禁时长（秒）

_INVALID_CODE_CACHE: dict[str, float] = {}
_IP_404_BAN: dict[str, float] = {}

# Regcode paths are high-cost under bursts: DB checks + serialized locks + optional Emby calls.
# Keep a small in-process admission gate so bursts fail fast instead of opening too many DB/socket FDs.
_REGCODE_OP_SEMAPHORE = asyncio.Semaphore(8)


async def _try_acquire_regcode_slot(timeout: float = 0.2) -> bool:
    try:
        await asyncio.wait_for(_REGCODE_OP_SEMAPHORE.acquire(), timeout=timeout)
        return True
    except asyncio.TimeoutError:
        return False


def _release_regcode_slot() -> None:
    try:
        _REGCODE_OP_SEMAPHORE.release()
    except ValueError:
        pass


def _is_known_invalid_code(code: str) -> bool:
    """该 code 是否在 ``_INVALID_CODE_CACHE`` 内且未过期。"""
    exp = _INVALID_CODE_CACHE.get(code)
    if exp is None:
        return False
    if _time.time() >= exp:
        _INVALID_CODE_CACHE.pop(code, None)
        return False
    return True


def _mark_invalid_code(code: str) -> None:
    """把刚刚 404 的 code 标记成已知失效，``_INVALID_CODE_TTL`` 内复用。"""
    now = _time.time()
    _INVALID_CODE_CACHE[code] = now + _INVALID_CODE_TTL
    # 顺手做一次按需 GC：写入达到上限时把过期项清掉，避免无界增长
    if len(_INVALID_CODE_CACHE) > _INVALID_CODE_CACHE_MAX:
        stale = [k for k, exp in _INVALID_CODE_CACHE.items() if exp < now]
        for k in stale:
            _INVALID_CODE_CACHE.pop(k, None)
        # 如果清完仍超限（极端攻击）就按 expire 时间最早开始淘汰
        if len(_INVALID_CODE_CACHE) > _INVALID_CODE_CACHE_MAX:
            excess = len(_INVALID_CODE_CACHE) - _INVALID_CODE_CACHE_MAX
            oldest = sorted(_INVALID_CODE_CACHE.items(), key=lambda kv: kv[1])[:excess]
            for k, _ in oldest:
                _INVALID_CODE_CACHE.pop(k, None)


def _is_ip_404_banned(ip: str) -> int:
    """返回该 IP 在 404 黑名单里的剩余封禁秒数，0 表示未封禁。"""
    exp = _IP_404_BAN.get(ip)
    if exp is None:
        return 0
    remaining = int(exp - _time.time())
    if remaining <= 0:
        _IP_404_BAN.pop(ip, None)
        return 0
    return remaining


def _record_404_and_maybe_ban(ip: str, code: str) -> None:
    """记一次 404，必要时把 IP 加入短期封禁名单。

    用 ``rate_limit_check`` 实现 ``_IP_404_THRESHOLD/_IP_404_WINDOW`` 窗口计数：
    第一次 ``rate_limit_check`` 返回 ``allowed=False`` 时说明本次刚好顶满阈值
    （上一次调用已经填到第 N 次），把 IP 写进 ``_IP_404_BAN``。
    """
    from src.core.utils import rate_limit_check

    allowed, _retry = rate_limit_check(
        "tg_bind_status_404_count",
        ip,
        max_requests=_IP_404_THRESHOLD,
        window_seconds=_IP_404_WINDOW,
    )
    if not allowed and ip not in _IP_404_BAN:
        _IP_404_BAN[ip] = _time.time() + _IP_404_BAN_DURATION
        logger.warning(
            "🚫 IP %s 因 60s 内连续 >=%d 次 /telegram/register/bind-code/status 404 " "被临时封禁 %ds (最后 code=%s)",
            ip,
            _IP_404_THRESHOLD,
            _IP_404_BAN_DURATION,
            code,
        )


# ==================== 用户注册 ====================


@users_bp.route("/register", methods=["POST"])
async def register():
    """
    统一账号注册

    使用注册码（可选）创建系统账号；Emby 账号由首次登录后在前端 Modal 补建。

    Request:
        {
            "username": "myusername",       // 必填
            "password": "mypassword",       // Web 端必填
            "telegram_bind_code": "123456", // 全局强制绑定 Telegram 时必填
            "reg_code": "code-xxx",         // 可选；若有则按注册码授予 Emby 开通天数
            "email": "user@example.com"     // 可选
        }
    """
    from src.config import Config, BangumiSyncConfig
    from src.core.utils import rate_limit_check

    # 公开端点：注册接口防批量创建，按 IP 限 5 次/10 分钟。
    client_ip = get_real_client_ip()
    allowed, retry_after = rate_limit_check(
        "user_register",
        client_ip,
        max_requests=5,
        window_seconds=600,
    )
    if not allowed:
        logger.warning(
            "/users/register 限流命中 ip=%s retry_after=%ss",
            client_ip,
            retry_after,
        )
        return api_response(
            False,
            f"注册过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    data = request.get_json() or {}

    telegram_id = data.get("telegram_id")
    telegram_bind_code = (data.get("telegram_bind_code") or "").strip().upper()
    username = (data.get("username") or "").strip()
    password = data.get("password")
    reg_code = (data.get("reg_code") or "").strip() or None
    email = (data.get("email") or "").strip() or None

    regcode_slot_acquired = False
    if reg_code:
        allowed_code, retry_after_code = rate_limit_check(
            "user_register_regcode",
            client_ip,
            max_requests=3,
            window_seconds=600,
        )
        if not allowed_code:
            return api_response(
                False,
                f"注册码注册过于频繁，请在 {retry_after_code} 秒后重试",
                code=429,
            )
        regcode_slot_acquired = await _try_acquire_regcode_slot()
        if not regcode_slot_acquired:
            return api_response(False, "当前注册码使用请求较多，请稍后重试", code=429)

    try:
        if not username:
            return api_response(False, "缺少用户名", code=400)

        # 注册时不允许直接提交 Telegram ID，必须走绑定码
        if telegram_id is not None:
            return api_response(False, "请使用 Telegram Bot 绑定码验证 Telegram ID", code=400)

        if telegram_bind_code:
            telegram_id = await _get_register_bind_telegram_id(telegram_bind_code)
            if not telegram_id:
                return api_response(
                    False,
                    "Telegram 绑定码无效或尚未通过 Bot 验证，请先在 Bot 中完成绑定",
                    code=400,
                )

        if telegram_id is not None and telegram_id != "":
            if isinstance(telegram_id, str) and telegram_id.isdigit():
                telegram_id = int(telegram_id)
            if not isinstance(telegram_id, int) or telegram_id <= 0:
                return api_response(False, "telegram_id 格式无效", code=400)
        else:
            telegram_id = None

        if Config.FORCE_BIND_TELEGRAM and not telegram_id:
            return api_response(False, "系统要求绑定 Telegram，请先获取绑定码并通过 Bot 验证", code=400)

        # Web 端注册：始终要求设置密码
        if not password:
            return api_response(False, "请设置密码", code=400)
        if len(password) < 8:
            return api_response(False, "密码长度至少 8 位", code=400)

        from src.core.utils import is_valid_username

        if not is_valid_username(username):
            return api_response(
                False,
                "用户名格式不正确（3-20位字母数字下划线，不能以数字开头）",
                code=400,
            )

        if email:
            from src.core.utils import is_valid_email

            if not is_valid_email(email):
                return api_response(False, "邮箱格式不正确", code=400)

        if reg_code:
            result = await UserService.register_by_code(telegram_id, username, reg_code, email, password)
        else:
            result = await UserService.register_pending(telegram_id, username, email, password)

        if result.result.value == "success":
            if telegram_bind_code:
                await _delete_bind_code(telegram_bind_code)

            user_info = await UserService.get_user_info(result.user) if result.user else None
            return api_response(
                True,
                result.message,
                {
                    "username": result.user.USERNAME if result.user else None,
                    "pending_emby": bool(getattr(result.user, "PENDING_EMBY", False)) if result.user else False,
                    "user": user_info,
                },
            )

        return api_response(False, result.message, code=400)
    finally:
        if regcode_slot_acquired:
            _release_regcode_slot()


@users_bp.route("/me/emby/register", methods=["POST"])
@require_auth
async def complete_emby_account_for_me():
    """
    已登录用户补建 Emby 账号。

    注册码用户需要当前用户 PENDING_EMBY=True 且 EMBYID 为空。
    自由注册用户不再预先写入 PENDING_EMBY，按当前配置即时校验。

    实际执行走 ``EmbyRegisterQueueService``：API handler 入队后同步等结果（默认 60s），
    在等待窗口内完成就直接返回最终态；超时未完成会返回 ``status='queued'/'processing'``，
    前端凭 ``request_id`` + ``status_token`` 调 ``/users/register/emby/status`` 继续轮询。

    Request:
        {
            "emby_username": "name",
            "emby_password": "Pwd1234X"
        }

    开通天数由管理员在 ``[SAR].emby_direct_register_days`` 统一固定，用户不再选择。
    老版本前端继续传 ``days`` 字段时由本接口静默忽略，不再报错。
    """
    user = g.current_user
    if user.EMBYID:
        return api_response(False, "您已绑定 Emby 账号，无需再次注册", code=400)

    from src.config import RegisterConfig

    has_regcode_entitlement = (
        bool(getattr(user, "PENDING_EMBY", False))
        and getattr(user, "PENDING_EMBY_DAYS", None) is not None
    )
    if not has_regcode_entitlement and not RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED:
        return api_response(
            False,
            "当前未开启 Emby 自由注册。如果需要绑定 Emby，请联系管理员或使用注册码。",
            code=400,
        )

    days_ok, _days, days_msg = UserService.resolve_emby_direct_register_days(user)
    if not days_ok:
        return api_response(False, days_msg or "当前账号没有 Emby 注册资格", code=400)

    data = request.get_json() or {}
    emby_username = (data.get("emby_username") or "").strip()
    emby_password = data.get("emby_password") or ""

    if not emby_username:
        return api_response(False, "请输入 Emby 用户名", code=400)
    from src.core.utils import is_valid_username

    if not is_valid_username(emby_username):
        return api_response(False, "Emby 用户名格式不正确（3-20位字母数字下划线，不能以数字开头）", code=400)
    ok, msg = UserService.validate_password_strength(emby_password, label="Emby 密码")
    if not ok:
        return api_response(False, msg, code=400)

    from src.services import EmbyRegisterQueueService

    status = await EmbyRegisterQueueService.enqueue_and_wait(
        user,
        emby_username,
        emby_password,
        timeout=60.0,
    )

    final_state = status.get("status")
    request_id = status.get("request_id")
    status_token = status.get("status_token") or status.get("token")
    message = status.get("message") or ""

    if final_state == "success":
        # 拿最新 user（complete_emby_registration 已经写过库），刷新返回信息
        refreshed = await UserOperate.get_user_by_uid(user.UID) or user
        user_info = await UserService.get_user_info(refreshed)
        return api_response(
            True,
            message or "Emby 账号已开通",
            {
                "user": user_info,
                "request_id": request_id,
            },
        )

    if final_state == "failed":
        # 失败 + 名额满 / 用户名冲突时按 409，其它一律 400
        text = (message or "").lower()
        code = 409 if any(k in text for k in ("已达上限", "已被占用", "已存在", "限")) else 400
        return api_response(
            False,
            message or "Emby 注册失败",
            {
                "request_id": request_id,
            },
            code=code,
        )

    if final_state == "rejected":
        # 队列前置拒绝（注册关闭 / 队列满 / 容量上限）
        return api_response(False, message or "Emby 注册请求被拒绝", code=400)

    # queued / processing：等待超时仍未结束，前端继续轮询
    return api_response(
        True,
        message or "Emby 注册已加入队列，请稍后查询结果",
        {
            "pending": True,
            "request_id": request_id,
            "status_token": status_token,
            "status": final_state,
            "queue_position": status.get("queue_position"),
        },
    )


@users_bp.route("/register/emby/status", methods=["GET"])
async def get_emby_register_status():
    """查询 Emby 注册队列状态。"""
    from src.services import EmbyRegisterQueueService
    from src.core.utils import rate_limit_check

    request_id = (request.args.get("request_id") or "").strip()
    status_token = (request.args.get("status_token") or "").strip()

    if not request_id or not status_token:
        return api_response(False, "缺少 request_id 或 status_token", code=400)

    # 轮询端点：按 request_id 限频 + 按 IP 兜底，防止被无限刷。
    client_ip = get_real_client_ip()
    allowed_req, retry_after_req = rate_limit_check(
        "emby_register_status:req",
        request_id,
        max_requests=60,
        window_seconds=60,
    )
    if not allowed_req:
        logger.warning(
            "/register/emby/status 单 request_id 限流命中 req=%s ip=%s retry_after=%ss",
            request_id,
            client_ip,
            retry_after_req,
        )
        return api_response(
            False,
            f"查询过于频繁，请在 {retry_after_req} 秒后重试",
            code=429,
        )
    allowed_ip, retry_after_ip = rate_limit_check(
        "emby_register_status:ip",
        client_ip,
        max_requests=240,
        window_seconds=60,
    )
    if not allowed_ip:
        logger.warning(
            "/register/emby/status IP 维度限流命中 ip=%s retry_after=%ss",
            client_ip,
            retry_after_ip,
        )
        return api_response(
            False,
            f"查询过于频繁，请在 {retry_after_ip} 秒后重试",
            code=429,
        )

    status = await EmbyRegisterQueueService.get_status(request_id, status_token)
    if not status:
        return api_response(False, "注册请求不存在或凭证无效", code=404)

    return api_response(True, "获取成功", status)


@users_bp.route("/check-available", methods=["GET"])
async def check_registration_available():
    """
    检查是否可以注册

    Response:
        {
            "success": true,
            "data": {
                "available": true,
                "message": "可以注册",
                "current_users": 50,
                "max_users": 200
            }
        }
    """
    from src.config import RegisterConfig
    from src.core.utils import rate_limit_check

    # 公开端点：注册页可能多次刷新查询，给个相对宽松的 IP 限速。
    client_ip = get_real_client_ip()
    allowed, retry_after = rate_limit_check(
        "register_check_available",
        client_ip,
        max_requests=60,
        window_seconds=60,
    )
    if not allowed:
        logger.warning(
            "/users/check-available 限流命中 ip=%s retry_after=%ss",
            client_ip,
            retry_after,
        )
        return api_response(
            False,
            f"请求过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    available, msg = await UserService.check_registration_available()
    current_count = await UserService.get_registered_user_count()
    emby_bound_count = await UserService.get_emby_bound_user_count()
    emby_user_limit = UserService.get_emby_user_limit()

    # 自由注册天数现在由管理员单值固定，不再返回候选/自定义区间字段；
    # 老版本前端仍可能读取这些字段，回传一个等值的单项数组兜底，避免 UI 报错。
    direct_days = int(RegisterConfig.EMBY_DIRECT_REGISTER_DAYS or 30)

    return api_response(
        True,
        msg,
        {
            "available": available,
            "message": msg,
            "current_users": current_count,
            "max_users": RegisterConfig.USER_LIMIT,
            "register_mode": RegisterConfig.REGISTER_MODE,
            "allow_pending_register": RegisterConfig.ALLOW_PENDING_REGISTER,
            "emby_direct_register_enabled": RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED,
            "emby_direct_register_days": direct_days,
            # 下面两个字段是兼容老前端的兜底，永远固定为单值 + 不允许自定义。
            "emby_direct_register_day_options": [direct_days],
            "emby_direct_register_allow_custom_days": False,
            "emby_user_limit": emby_user_limit,
            "emby_bound_users": emby_bound_count,
        },
    )


# ==================== 用户信息 ====================


@users_bp.route("/me", methods=["GET"])
@require_auth
async def get_my_info():
    """获取当前用户详细信息"""
    user_info = await UserService.get_user_info(g.current_user)

    # 获取 Emby 状态
    from src.services import EmbyService

    status = await EmbyService.get_user_status(g.current_user)

    user_info["emby_status"] = {
        "is_synced": status.is_synced,
        "is_active": status.is_active,
        "active_sessions": status.active_sessions,
        "message": status.message,
    }

    return api_response(True, "获取成功", user_info)


@users_bp.route("/me", methods=["PUT"])
@require_auth
async def update_my_info():
    """
    更新当前用户信息

    Request:
        {
            "email": "new@example.com",
            "bgm_mode": true,
            "bgm_token": "your_bgm_access_token"
        }
    """
    from src.config import BangumiSyncConfig

    data = request.get_json() or {}
    user = g.current_user
    updated = False

    # 更新邮箱
    if "email" in data:
        email = data["email"]
        if email:
            from src.core.utils import is_valid_email

            if not is_valid_email(email):
                return api_response(False, "邮箱格式不正确", code=400)
        user.EMAIL = email
        updated = True

    # 更新 Bangumi 同步设置
    bgm_fields_present = "bgm_token" in data or "bgm_mode" in data
    if bgm_fields_present and not BangumiSyncConfig.ENABLED:
        return api_response(False, "Bangumi 点格子功能未启用", code=400)

    if "bgm_token" in data:
        bgm_token = str(data.get("bgm_token") or "").strip()
        if len(bgm_token) > 4096:
            return api_response(False, "Bangumi Token 过长", code=400)
        user.BGM_TOKEN = bgm_token
        updated = True

    if "bgm_mode" in data:
        bgm_mode = parse_bool(data["bgm_mode"], default=False)
        if bgm_mode and not (user.BGM_TOKEN or "").strip():
            return api_response(False, "请先设置个人 Bangumi Token 后启用 BGM 同步", code=400)
        user.BGM_MODE = bgm_mode
        updated = True

    if updated:
        await UserOperate.update_user(user)

    user_info = await UserService.get_user_info(user)
    return api_response(True, "更新成功", user_info)


@users_bp.route("/me/username", methods=["PUT"])
@require_auth
async def change_my_username():
    """
    修改用户名

    Request:
        {
            "new_username": "newname"
        }
    """
    data = request.get_json() or {}
    new_username = data.get("new_username")

    if not new_username:
        return api_response(False, "缺少 new_username", code=400)

    from src.core.utils import is_valid_username

    if not is_valid_username(new_username):
        return api_response(False, "用户名格式不正确", code=400)

    success, message = await UserService.change_username(g.current_user, new_username)
    return api_response(success, message)


@users_bp.route("/me/password", methods=["PUT"])
@require_auth
async def reset_my_password():
    """重置密码（需验证当前密码）

    Request:
        {
            "current_password": "old_password"  // 必填，验证身份
        }

    Response:
        {
            "success": true,
            "data": { "new_password": "随机生成的新密码" }
        }
    """
    from src.core.utils import verify_password, rate_limit_check

    # 防止被盗 token 频繁重置密码
    uid = str(g.current_user.UID)
    allowed, retry_after = rate_limit_check(
        "password_reset_self",
        uid,
        max_requests=3,
        window_seconds=3600,
    )
    if not allowed:
        return api_response(False, f"密码重置过于频繁，请在 {retry_after} 秒后重试", code=429)

    data = request.get_json() or {}
    current_password = data.get("current_password", "")

    if not current_password:
        return api_response(False, "请提供当前密码以验证身份", code=400)

    # 验证当前密码
    if not g.current_user.PASSWORD or not verify_password(current_password, g.current_user.PASSWORD):
        return api_response(False, "当前密码错误", code=401)

    success, message, new_password = await UserService.reset_password(g.current_user)

    if success:
        return api_response(True, message, {"new_password": new_password})
    return api_response(False, message)


@users_bp.route("/me/password/change", methods=["POST"])
@require_auth
async def change_my_password():
    """
    修改密码（同时修改系统密码和 Emby 密码）

    Request:
        {
            "old_password": "current_password",
            "new_password": "new_password"
        }
    """
    data = request.get_json() or {}
    old_password = data.get("old_password", "")
    new_password = data.get("new_password", "")

    if not old_password or not new_password:
        return api_response(False, "请提供当前密码和新密码", code=400)

    ok, msg = UserService.validate_password_strength(new_password, label="新密码")
    if not ok:
        return api_response(False, msg, code=400)

    success, message = await UserService.change_password(g.current_user, old_password, new_password)

    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@users_bp.route("/me/password/system", methods=["POST"])
@require_auth
async def change_my_system_password():
    """
    修改系统登录密码（不影响 Emby 密码）

    Request:
        {
            "old_password": "current_password",
            "new_password": "new_password"
        }
    """
    data = request.get_json() or {}
    old_password = data.get("old_password", "")
    new_password = data.get("new_password", "")

    if not old_password or not new_password:
        return api_response(False, "请提供当前密码和新密码", code=400)

    ok, msg = UserService.validate_password_strength(new_password, label="新密码")
    if not ok:
        return api_response(False, msg, code=400)

    success, message = await UserService.change_system_password(g.current_user, old_password, new_password)
    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@users_bp.route("/me/password/emby", methods=["POST"])
@require_auth
async def change_my_emby_password():
    """
    修改 Emby 密码（仅更新绑定的 Emby 账户）

    Request:
        {
            "new_password": "new_password"
        }
    """
    data = request.get_json() or {}
    new_password = data.get("new_password", "")

    if not new_password:
        return api_response(False, "请提供新密码", code=400)

    ok, msg = UserService.validate_password_strength(new_password, label="新密码")
    if not ok:
        return api_response(False, msg, code=400)

    success, message = await UserService.change_emby_password(g.current_user, new_password)
    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@users_bp.route("/me/emby/bind", methods=["POST"])
@require_auth
async def bind_emby_account():
    """
    绑定已有的 Emby 账号（需要验证用户名和密码）

    Request:
        {
            "emby_username": "existing_username",  // Emby 用户名
            "emby_password": "password"           // Emby 密码
        }

    Response:
        {
            "success": true,
            "message": "绑定成功",
            "data": {
                "emby_id": "xxx",
                "emby_username": "existing_username"
            }
        }
    """
    from src.services.emby import get_emby_client, EmbyError

    data = request.get_json() or {}

    # 尝试多种可能的字段名
    emby_username = data.get("emby_username") or data.get("username") or data.get("embyUsername") or ""
    if isinstance(emby_username, str):
        emby_username = emby_username.strip()
    else:
        emby_username = ""

    # 区分"未传字段"和"传了空字符串"——Emby 支持空密码账号
    raw_password = None
    for key in ("emby_password", "password", "embyPassword"):
        if key in data:
            raw_password = data[key]
            break
    if isinstance(raw_password, str):
        emby_password = raw_password
    elif raw_password is None:
        emby_password = None
    else:
        emby_password = ""

    logger.debug(
        f"绑定 Emby 账号请求: username={emby_username}, "
        f"password_provided={emby_password is not None}, "
        f"password_length={len(emby_password) if emby_password is not None else 0}, "
        f"data_keys={list(data.keys())}"
    )

    if not emby_username:
        return api_response(False, "请输入 Emby 用户名", code=400)

    if emby_password is None:
        return api_response(False, "请提供 Emby 密码字段（空密码请传空字符串）", code=400)

    user = g.current_user

    # 检查用户是否已绑定 Emby 账号
    if user.EMBYID:
        return api_response(False, "您已绑定 Emby 账号，请先解绑", code=400)

    # 验证 Emby 用户名和密码
    emby = get_emby_client()
    capacity_lock = None
    try:
        # 首先验证用户名和密码
        emby_user = await emby.authenticate_by_name(emby_username, emby_password)
        if not emby_user:
            return api_response(False, "用户名或密码错误", code=401)

        # 验证用户名是否匹配
        if emby_user.name.lower() != emby_username.lower():
            return api_response(False, "用户名不匹配", code=400)

        # 检查该 Emby 账号是否已被其他本地用户绑定
        existing_bind = await UserOperate.get_user_by_embyid(emby_user.id)
        if existing_bind and existing_bind.UID != user.UID:
            return api_response(False, "该 Emby 账号已被其他用户绑定", code=400)

        # 绑定路径与自由注册队列/卡码队列共享同一个 Emby 用户上限。
        capacity_lock = await UserService.acquire_emby_capacity_lock()
        if capacity_lock is None:
            return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)
        user_limit_ok, user_limit_msg = await UserService.check_normal_user_capacity_for_grant(user)
        if not user_limit_ok:
            return api_response(False, user_limit_msg, code=409)

        if not getattr(user, "PENDING_EMBY", False):
            cap_ok, cap_msg = await UserService.check_emby_user_capacity(
                exclude_uid=user.UID,
            )
            if not cap_ok:
                return api_response(False, cap_msg, code=409)

        # 绑定账号
        user.EMBYID = emby_user.id
        import json

        other_data = {}
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}
        other_data["emby_username"] = emby_user.name
        user.OTHER = json.dumps(other_data)

        # 决定开通后到期时间：
        #   管理员/白名单 → 永久；
        #   其它：优先使用注册码授予的 PENDING_EMBY_DAYS；否则用配置默认值；
        #     days <= 0 视为永久（-1）；否则 now + days。
        from src.core.utils import days_to_seconds, timestamp
        from src.config import RegisterConfig

        if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
            user.EXPIRED_AT = 253402214400  # 9999-12-31
        else:
            if user.ROLE == Role.UNRECOGNIZED.value:
                user.ROLE = Role.NORMAL.value

            pending_days = getattr(user, "PENDING_EMBY_DAYS", None)
            if pending_days is None:
                try:
                    pending_days = int(RegisterConfig.EMBY_DIRECT_REGISTER_DAYS or 30)
                except (TypeError, ValueError):
                    pending_days = 30
            try:
                pending_days = int(pending_days)
            except (TypeError, ValueError):
                pending_days = 30
            # 仅当当前不是合法的「正在使用的到期时间」时才覆盖
            # （EXPIRED_AT in (-1, 0) 或已过期都视为需要重新计算；正数且未过期则保留原值，避免回退）
            current_exp = user.EXPIRED_AT or 0
            now_ts = timestamp()
            if current_exp in (-1, 0) or current_exp < now_ts:
                user.EXPIRED_AT = -1 if pending_days <= 0 else (now_ts + days_to_seconds(pending_days))

        # 一次性清掉 PENDING 标记
        user.PENDING_EMBY = False
        user.PENDING_EMBY_DAYS = None

        # 待激活账号补激活
        if not user.ACTIVE_STATUS:
            user.ACTIVE_STATUS = True

        await UserOperate.update_user(user)

        try:
            from src.services import EmbyService

            await UserService.sync_user_to_emby(user)
            await EmbyService.apply_default_hidden_libraries(user)
        except Exception as exc:  # pragma: no cover
            logger.warning(f"绑定 Emby 后同步状态或应用默认隐藏媒体库失败: {exc}")

        logger.info(f"用户绑定 Emby 账号成功: {user.USERNAME} -> {emby_username} (ID: {emby_user.id})")

        return api_response(
            True,
            "绑定成功",
            {
                "emby_id": emby_user.id,
                "emby_username": emby_username,
            },
        )

    except EmbyError as e:
        logger.error(f"绑定 Emby 账号失败: {e}")
        return api_response(False, "绑定失败，请检查用户名密码或稍后重试", code=500)
    except Exception as e:
        logger.error(f"绑定 Emby 账号失败: {e}")
        return api_response(False, "绑定失败，请稍后重试", code=500)
    finally:
        await UserService.release_emby_capacity_lock(capacity_lock)


@users_bp.route("/me/emby/unbind", methods=["POST"])
@require_auth
async def unbind_emby_account():
    """
    解绑 Emby 账号

    注意：解绑后用户将无法访问 Emby，但不会删除 Emby 中的账号
    """
    user = g.current_user

    if not user.EMBYID:
        return api_response(False, "您未绑定 Emby 账号", code=400)

    # 解绑（不清除 Emby 账号，只清除本地关联）
    old_emby_id = user.EMBYID
    user.EMBYID = None
    import json

    if user.OTHER:
        try:
            other_data = json.loads(user.OTHER)
        except (json.JSONDecodeError, TypeError):
            other_data = {}
        else:
            other_data.pop("emby_username", None)
            user.OTHER = json.dumps(other_data) if other_data else ""
    # 不修改用户名，保留原用户名
    await UserOperate.update_user(user)

    logger.info(f"用户解绑 Emby 账号: {user.USERNAME} (原 Emby ID: {old_emby_id})")

    return api_response(True, "解绑成功")


# ==================== 用户续期 ====================


@users_bp.route("/regcode/check", methods=["POST"])
async def check_regcode():
    """
    检查注册码/续期码类型

    Request:
        {
            "reg_code": "code-xxx"
        }

    Response:
        {
            "success": true,
            "data": {
                "type": 1,  // 1=注册, 2=续期, 3=白名单
                "type_name": "注册",
                "days": 30,
                "valid": true
            }
        }
    """
    from src.db.regcode import RegCodeOperate
    from src.core.utils import rate_limit_check

    # 公开端点，按 IP 限流防止枚举注册码（10 次 / 分钟）
    client_ip = get_real_client_ip()
    allowed, retry_after = rate_limit_check(
        "regcode_check",
        client_ip,
        max_requests=10,
        window_seconds=60,
    )
    if not allowed:
        return api_response(
            False,
            f"操作过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    slot_acquired = await _try_acquire_regcode_slot(timeout=0.1)
    if not slot_acquired:
        return api_response(False, "当前注册码请求较多，请稍后重试", code=429)

    try:
        data = request.get_json() or {}
        reg_code = data.get("reg_code", "").strip()

        if not reg_code:
            return api_response(False, "缺少注册码", code=400)

        code_info = await RegCodeOperate.get_regcode_by_code(reg_code)

        if not code_info or RegCodeOperate.is_decoy(code_info):
            return api_response(False, "注册码不存在", code=404)

        if not code_info.ACTIVE:
            return api_response(False, "注册码已禁用", code=400)

        # 检查是否已用完
        if code_info.USE_COUNT_LIMIT > 0 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
            return api_response(False, "注册码已用完", code=400)

        if UserService._is_regcode_expired(code_info):
            return api_response(False, "注册码已过期", code=400)

        type_names = {1: "注册", 2: "续期", 3: "白名单"}
        days = UserService._normalize_code_days(code_info.DAYS, default=30)

        return api_response(
            True,
            "注册码有效",
            {
                "type": code_info.TYPE,
                "type_name": type_names.get(code_info.TYPE, "未知"),
                "days": days,
                "valid": True,
            },
        )
    finally:
        _release_regcode_slot()


@users_bp.route("/me/renew", methods=["POST"])
@require_auth
async def renew_my_account():
    """
    使用续期码续期

    Request:
        {
            "reg_code": "code-xxx"
        }
    """
    data = request.get_json() or {}
    reg_code = data.get("reg_code")

    if not reg_code:
        return api_response(False, "缺少续期码", code=400)

    success, message = await UserService.renew_user(g.current_user, 30, reg_code)

    if success:
        user_info = await UserService.get_user_info(g.current_user)
        return api_response(
            True,
            message,
            {
                "expire_status": user_info["expire_status"],
                "expired_at": user_info["expired_at"],
            },
        )
    return api_response(False, message)


@users_bp.route("/me/use-code", methods=["POST"])
@require_auth
async def use_code():
    """
    统一使用注册码/续期码/白名单码

    已登录用户根据卡码类型自动处理：
    - 注册码：为无 Emby 账户的用户创建 Emby 账户
    - 续期码：续期
    - 白名单码：赋予白名单角色，自动创建 Emby 账户（如没有）

    Request:
        {
            "reg_code": "code-xxx",
            "emby_username": "emby_name",   // 创建 Emby 账户时必填
            "emby_password": "Password123"   // 创建 Emby 账户时必填
        }

    Response:
        {
            "success": true,
            "message": "操作成功",
            "data": {
                "emby_password": "xxx",  // 仅创建 Emby 账户时返回
                "expire_status": "...",
                "expired_at": 12345678
            }
        }
    """
    from src.core.utils import rate_limit_check

    client_ip = get_real_client_ip()
    uid = getattr(g.current_user, "UID", "unknown")
    allowed_uid, retry_after_uid = rate_limit_check(
        "regcode_use_uid",
        str(uid),
        max_requests=6,
        window_seconds=60,
    )
    if not allowed_uid:
        return api_response(False, f"卡码使用过于频繁，请在 {retry_after_uid} 秒后重试", code=429)
    allowed_ip, retry_after_ip = rate_limit_check(
        "regcode_use_ip",
        client_ip,
        max_requests=120,
        window_seconds=60,
    )
    if not allowed_ip:
        return api_response(False, f"该 IP 卡码请求过于频繁，请在 {retry_after_ip} 秒后重试", code=429)

    data = request.get_json() or {}
    reg_code = data.get("reg_code", "").strip()
    emby_username = (data.get("emby_username") or "").strip() or None

    raw_password = data.get("emby_password")
    if isinstance(raw_password, str):
        emby_password = raw_password
    elif raw_password is None:
        emby_password = None
    else:
        emby_password = ""

    if not reg_code:
        return api_response(False, "缺少注册码/续期码", code=400)

    from src.services import RegcodeUseQueueService

    # 低流量时走同步快路径，避免无意义入队和轮询；有队列压力时再退回后台队列。
    if RegcodeUseQueueService.in_flight_count() == 0:
        success, message, generated_password = await UserService.use_code(
            g.current_user,
            reg_code,
            emby_username=emby_username,
            emby_password=emby_password,
        )
        if success:
            refreshed = await UserOperate.get_user_by_uid(g.current_user.UID) or g.current_user
            user_info = await UserService.get_user_info(refreshed)
            return api_response(
                True,
                message,
                {
                    "pending": False,
                    "emby_password": generated_password,
                    "expire_status": user_info["expire_status"],
                    "expired_at": user_info["expired_at"],
                    "role": user_info["role"],
                    "role_name": user_info["role_name"],
                    "user": user_info,
                },
            )
        return api_response(False, message, code=400)

    payload, message = await RegcodeUseQueueService.enqueue(
        uid=int(g.current_user.UID),
        reg_code=reg_code,
        emby_username=emby_username,
        emby_password=emby_password,
    )
    if payload is None:
        return api_response(False, message, code=429)
    return api_response(True, message, {"pending": True, **payload})


@users_bp.route("/me/use-code/status", methods=["GET"])
@require_auth
async def get_use_code_status():
    """查询卡码使用队列状态。"""
    from src.core.utils import rate_limit_check
    from src.services import RegcodeUseQueueService

    request_id = (request.args.get("request_id") or "").strip()
    status_token = (request.args.get("status_token") or "").strip()
    if not request_id or not status_token:
        return api_response(False, "缺少 request_id 或 status_token", code=400)

    allowed, retry_after = rate_limit_check(
        "regcode_use_status_uid",
        str(g.current_user.UID),
        max_requests=60,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"查询过于频繁，请在 {retry_after} 秒后重试", code=429)

    status = await RegcodeUseQueueService.get_status(request_id, status_token)
    if not status:
        return api_response(False, "卡码请求不存在或凭证无效", code=404)
    if int(status.get("uid") or 0) != int(g.current_user.UID):
        return api_response(False, "无权查看该卡码请求", code=403)
    return api_response(True, "获取成功", status)


# ==================== 用户设备 ====================


@users_bp.route("/me/devices", methods=["GET"])
@require_auth
async def get_my_devices():
    """获取我的设备列表"""
    from src.services import EmbyService

    devices = await EmbyService.get_user_devices(g.current_user)
    return api_response(True, "获取成功", devices)


@users_bp.route("/me/devices/<device_id>", methods=["DELETE"])
@require_auth
async def remove_my_device(device_id: str):
    """移除我的设备"""
    from src.services import EmbyService

    success, message = await EmbyService.remove_user_device(g.current_user, device_id)
    return api_response(success, message)


# ==================== 用户媒体库 ====================


@users_bp.route("/me/libraries", methods=["GET"])
@require_auth
async def get_my_libraries():
    """获取我可访问的媒体库"""
    from src.services import EmbyService

    detail = await EmbyService.get_user_library_access_detail(g.current_user)
    return api_response(True, "获取成功", detail)


@users_bp.route("/me/libraries/visibility", methods=["PUT"])
@require_auth
async def update_my_library_visibility():
    """用户自行显示/隐藏管理员配置允许的媒体库。"""
    from src.services import EmbyService

    user = g.current_user
    if not user.EMBYID:
        return api_response(False, "请先绑定 Emby 账号", code=400)

    if not EmbyService.can_self_service_libraries(user):
        return api_response(False, "管理员未为你开启媒体库自助显隐权限", code=403)

    allowed_names = EmbyService.self_service_library_names()
    if not allowed_names:
        return api_response(False, "管理员未开放可自助显隐的媒体库", code=403)

    data = request.get_json() or {}
    action = str(data.get("action") or "").strip().lower()
    if action not in {"show", "hide"}:
        return api_response(False, "action 必须是 show 或 hide", code=400)

    raw_names = data.get("library_names")
    if raw_names is not None and not isinstance(raw_names, (str, list, tuple, set)):
        return api_response(False, "library_names 必须是字符串或字符串数组", code=400)

    requested_names = EmbyService._normalize_library_names(raw_names or allowed_names)
    if not requested_names:
        return api_response(False, "媒体库名称不能为空", code=400)
    if len(requested_names) > 50:
        return api_response(False, "一次最多操作 50 个媒体库", code=400)
    too_long = [name for name in requested_names if len(name) > 128]
    if too_long:
        return api_response(False, "媒体库名称过长", code=400)

    allowed_keys = {name.lower() for name in allowed_names}
    denied = [name for name in requested_names if name.lower() not in allowed_keys]
    if denied:
        return api_response(False, f"以下媒体库未开放自助显隐: {', '.join(denied)}", code=403)

    ok, msg = await EmbyService.set_user_library_visibility(
        user,
        requested_names,
        visible=(action == "show"),
    )
    if not ok:
        return api_response(False, msg, code=500)

    detail = await EmbyService.get_user_library_access_detail(user)
    return api_response(True, msg, detail)


# ==================== 用户会话 ====================


@users_bp.route("/me/sessions", methods=["GET"])
@require_auth
async def get_my_sessions():
    """获取我的活动会话"""
    from src.services import get_emby_client

    if not g.current_user.EMBYID:
        return api_response(True, "获取成功", [])

    emby = get_emby_client()
    sessions = await emby.get_user_sessions(g.current_user.EMBYID)

    return api_response(
        True,
        "获取成功",
        [
            {
                "id": s.id,
                "client": s.client,
                "device_name": s.device_name,
                "is_active": s.is_active,
                "now_playing": s.now_playing_item.get("Name") if s.now_playing_item else None,
            }
            for s in sessions
        ],
    )


# ==================== 用户登录历史 ====================


@users_bp.route("/me/login-history", methods=["GET"])
@require_auth
async def get_my_login_history():
    """获取我的登录信息"""
    user = g.current_user

    return api_response(
        True,
        "获取成功",
        {
            "last_login_time": user.LAST_LOGIN_TIME,
            "last_login_ip": user.LAST_LOGIN_IP[:3] + "***" if user.LAST_LOGIN_IP else None,  # 部分隐藏 IP
            "last_login_ua": user.LAST_LOGIN_UA,
        },
    )


# ==================== Telegram 绑定管理 ====================


@users_bp.route("/me/telegram", methods=["GET"])
@require_auth
async def get_telegram_status():
    """
    获取 Telegram 绑定状态

    Response:
        {
            "success": true,
            "data": {
                "bound": true,
                "telegram_id": 123456789,  // 部分隐藏
                "force_bind": true,        // 系统是否强制绑定 TG
                "can_unbind": false,       // 是否可以解绑（强制绑定时不可解绑）
                "can_change": true         // 是否可以换绑
            }
        }
    """
    from src.config import Config

    user = g.current_user
    force_bind = Config.FORCE_BIND_TELEGRAM

    # 隐藏部分 Telegram ID
    masked_id = None
    if user.TELEGRAM_ID:
        id_str = str(user.TELEGRAM_ID)
        if len(id_str) > 4:
            masked_id = id_str[:3] + "****" + id_str[-2:]
        else:
            masked_id = "****"

    # 尝试获取 Telegram 用户名
    telegram_username = None
    if user.TELEGRAM_ID:
        try:
            from src.services.telegram_runtime import run_bot_operation

            async def _resolve_username(bot):
                tg_user = await bot.get_chat(user.TELEGRAM_ID)
                return tg_user.username or f"{tg_user.first_name or ''} {tg_user.last_name or ''}".strip() or None

            telegram_username = await run_bot_operation(_resolve_username, timeout=8)
        except Exception:
            pass  # Bot 未初始化或获取失败，忽略

    pending_request = await UserService.get_telegram_rebind_request(user.UID)
    has_pending_rebind_request = bool(pending_request and pending_request.STATUS == "pending")
    can_change = bool(user.TELEGRAM_ID) and not has_pending_rebind_request
    return api_response(
        True,
        "获取成功",
        {
            "bound": bool(user.TELEGRAM_ID),
            "telegram_id": masked_id,
            "has_telegram_id": bool(user.TELEGRAM_ID),  # 前端判断是否已绑定
            "telegram_username": telegram_username,  # Telegram 用户名
            "force_bind": force_bind,
            "can_unbind": not force_bind and bool(user.TELEGRAM_ID),
            "can_change": can_change,
            "pending_rebind_request": has_pending_rebind_request,
            "rebind_request_status": pending_request.STATUS if pending_request else None,
            "rebind_request_id": pending_request.ID if pending_request else None,
        },
    )


@users_bp.route("/me/telegram/rebind-request", methods=["POST"])
@require_auth
async def create_tg_rebind_request():
    from src.config import Config
    from src.core.utils import rate_limit_check

    if not Config.TELEGRAM_MODE:
        return api_response(False, "Telegram 功能未启用", code=400)

    user = g.current_user

    # 换绑请求会进管理员审批队列，单用户每小时最多 3 次足够。
    allowed, retry_after = rate_limit_check(
        "tg_rebind_request",
        str(user.UID),
        max_requests=3,
        window_seconds=3600,
    )
    if not allowed:
        logger.warning(
            "/me/telegram/rebind-request 限流命中 uid=%s retry_after=%ss",
            user.UID,
            retry_after,
        )
        return api_response(
            False,
            f"换绑请求过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    if not user.TELEGRAM_ID:
        return api_response(False, "当前账号尚未绑定 Telegram", code=400)

    data = request.get_json() or {}
    reason = data.get("reason")

    success, message, request_obj = await UserService.create_telegram_rebind_request(user, reason)
    if success:
        return api_response(
            True,
            message,
            {
                "request_id": request_obj.ID,
                "status": request_obj.STATUS,
            },
        )
    return api_response(False, message, code=400)


@users_bp.route("/me/telegram/unbind", methods=["POST"])
@require_auth
async def unbind_my_telegram():
    """
    解绑 Telegram 账号

    注意：如果系统强制要求绑定 Telegram，则不允许解绑
    """
    from src.config import Config
    from src.core.utils import rate_limit_check

    user = g.current_user

    # 按 UID 限速：防恶意频繁解绑触发外部联动。
    allowed, retry_after = rate_limit_check(
        "tg_unbind",
        str(user.UID),
        max_requests=5,
        window_seconds=600,
    )
    if not allowed:
        logger.warning(
            "/me/telegram/unbind 限流命中 uid=%s retry_after=%ss",
            user.UID,
            retry_after,
        )
        return api_response(
            False,
            f"操作过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    # 检查是否强制绑定
    if Config.FORCE_BIND_TELEGRAM:
        return api_response(False, "系统要求必须绑定 Telegram，不允许解绑。如需更换账号请使用换绑功能", code=403)

    # 检查是否已绑定
    if not user.TELEGRAM_ID:
        return api_response(False, "您尚未绑定 Telegram", code=400)

    old_telegram_id = user.TELEGRAM_ID
    user.TELEGRAM_ID = None
    await UserOperate.update_user(user)

    return api_response(
        True,
        "Telegram 已解绑",
        {
            "old_telegram_id": old_telegram_id,
        },
    )


# ==================== Telegram 绑定码 ====================

_BIND_CODE_EXPIRE = 300  # 绑定码有效期（秒）
_MAX_BIND_CODES = 20000
_BIND_SCENE_REGISTER = "register"
_BIND_SCENE_USER = "user"


def _detect_image_extension(header: bytes) -> str | None:
    """根据魔数识别图片扩展名。"""
    if header.startswith(b"\xff\xd8\xff"):
        return "jpg"
    if header.startswith(b"\x89PNG\r\n\x1a\n"):
        return "png"
    if header.startswith(b"GIF87a") or header.startswith(b"GIF89a"):
        return "gif"
    if len(header) >= 12 and header.startswith(b"RIFF") and header[8:12] == b"WEBP":
        return "webp"
    return None


def _is_safe_upload_relative_url(value: str) -> bool:
    """校验站内 uploads 相对 URL，阻止路径穿越。"""
    if not value:
        return False
    s = value.strip()
    if not (s.startswith("/uploads/") or s.startswith("/api/v1/users/assets/")):
        return False
    if "\\" in s or "\x00" in s:
        return False

    rel_raw = s.removeprefix("/uploads/").removeprefix("/api/v1/users/assets/")
    rel = Path(rel_raw)
    # 只允许普通相对路径片段
    if rel.is_absolute() or any(part in ("..", ".", "") for part in rel.parts):
        return False
    return True


def _get_upload_root_path() -> Path:
    """获取并确保上传根目录存在。"""
    from flask import current_app

    root = Path(str(current_app.config.get("UPLOAD_FOLDER", ""))).resolve()
    root.mkdir(parents=True, exist_ok=True)
    return root


def _resolve_upload_file_path(relative_url: str, required_subdir: str | None = None) -> Path | None:
    """将 /uploads/... URL 解析为本地文件路径，并可限制到指定子目录。"""
    if not _is_safe_upload_relative_url(relative_url):
        return None

    upload_root = _get_upload_root_path()
    rel = Path(relative_url.removeprefix("/uploads/").removeprefix("/api/v1/users/assets/"))
    file_path = (upload_root / rel).resolve()

    if not file_path.is_relative_to(upload_root):
        return None

    if required_subdir:
        required_root = (upload_root / required_subdir).resolve()
        if not file_path.is_relative_to(required_root):
            return None

    return file_path


def _make_upload_asset_url(kind: str, filename: str) -> str:
    return f"/api/v1/users/assets/{kind}/{filename}"


def _legacy_upload_url(kind: str, filename: str) -> str:
    return f"/uploads/{kind}/{filename}"


def _extract_upload_ref(value: str, *, allowed_kinds: set[str] | None = None) -> tuple[str, str] | None:
    """Extract (kind, filename) from controlled or legacy upload URLs/CSS url(...)."""
    if not value:
        return None
    raw = value.strip()
    match = _CSS_URL_RE.match(raw) if "url" in raw.lower() else None
    if match:
        raw = match.group(2).strip()
    prefixes = ("/api/v1/users/assets/", "/uploads/")
    for prefix in prefixes:
        if raw.startswith(prefix):
            rel = raw.removeprefix(prefix)
            parts = rel.split("/", 1)
            if len(parts) != 2:
                return None
            kind, filename = parts[0], parts[1]
            if allowed_kinds and kind not in allowed_kinds:
                return None
            if not re.fullmatch(r"[0-9a-f]{32}\.(jpg|png|gif|webp)", filename, re.IGNORECASE):
                return None
            return kind, filename
    return None


def _rewrite_upload_urls_in_background(config: dict) -> dict:
    """Return a copy whose local background URLs use authenticated asset endpoints."""
    out = dict(config or {})
    for key in ("lightBgImage", "darkBgImage"):
        value = str(out.get(key) or "")
        ref = _extract_upload_ref(value, allowed_kinds={"backgrounds"})
        if ref:
            kind, filename = ref
            out[key] = f'url("{_make_upload_asset_url(kind, filename)}")'
    return out


def _rewrite_avatar_url(value: str | None) -> str | None:
    ref = _extract_upload_ref(value or "", allowed_kinds={"avatars"})
    if not ref:
        return value or None
    kind, filename = ref
    return _make_upload_asset_url(kind, filename)


def _referenced_upload_urls_from_background(config: dict) -> set[str]:
    refs: set[str] = set()
    for key in ("lightBgImage", "darkBgImage"):
        ref = _extract_upload_ref(str((config or {}).get(key) or ""), allowed_kinds={"backgrounds"})
        if ref:
            kind, filename = ref
            refs.add(_legacy_upload_url(kind, filename))
            refs.add(_make_upload_asset_url(kind, filename))
    return refs


async def cleanup_unused_upload_assets(max_age_seconds: int = 24 * 3600) -> dict:
    """Remove uploaded avatars/backgrounds no longer referenced by any user.

    Fresh files are kept for a grace period because upload and save background are two
    separate requests.
    """
    now = _time.time()
    upload_root = _get_upload_root_path()
    referenced: set[str] = set()
    users, _ = await UserOperate.get_all_users(include_inactive=True, limit=100000, offset=0)
    for user in users:
        if user.AVATAR:
            ref = _extract_upload_ref(user.AVATAR, allowed_kinds={"avatars"})
            if ref:
                kind, filename = ref
                referenced.add(_legacy_upload_url(kind, filename))
                referenced.add(_make_upload_asset_url(kind, filename))
        if user.OTHER:
            try:
                other = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other = {}
            if isinstance(other, dict):
                bg = other.get("background") if isinstance(other.get("background"), dict) else {}
                referenced.update(_referenced_upload_urls_from_background(bg))

    result = {"scanned": 0, "deleted": 0, "skipped_recent": 0, "failed": 0}
    for kind in ("avatars", "backgrounds"):
        folder = (upload_root / kind).resolve()
        if not folder.is_dir() or not folder.is_relative_to(upload_root):
            continue
        for path in folder.iterdir():
            if not path.is_file():
                continue
            result["scanned"] += 1
            filename = path.name
            if _legacy_upload_url(kind, filename) in referenced or _make_upload_asset_url(kind, filename) in referenced:
                continue
            try:
                if now - path.stat().st_mtime < max_age_seconds:
                    result["skipped_recent"] += 1
                    continue
                path.unlink()
                result["deleted"] += 1
            except Exception:
                result["failed"] += 1
                logger.warning("清理未引用上传文件失败: %s", path, exc_info=True)
    return result


@users_bp.route("/assets/<kind>/<filename>", methods=["GET"])
@require_auth
async def get_user_upload_asset(kind: str, filename: str):
    """Serve user-uploaded avatars/backgrounds through an authenticated safe path."""
    if kind not in {"avatars", "backgrounds"}:
        return api_response(False, "资源类型不存在", code=404)
    if not re.fullmatch(r"[0-9a-f]{32}\.(jpg|png|gif|webp)", filename, re.IGNORECASE):
        return api_response(False, "资源不存在", code=404)

    file_path = _resolve_upload_file_path(_legacy_upload_url(kind, filename), required_subdir=kind)
    if not file_path or not file_path.exists() or not file_path.is_file():
        return api_response(False, "资源不存在", code=404)

    # Users may only fetch files referenced by their own profile. Admins can fetch any
    # managed upload for support/debugging.
    current = g.current_user
    allowed = current.ROLE == Role.ADMIN.value
    if not allowed and kind == "avatars":
        allowed = _rewrite_avatar_url(current.AVATAR) == _make_upload_asset_url(kind, filename)
    if not allowed and kind == "backgrounds" and current.OTHER:
        try:
            other = json.loads(current.OTHER)
        except (json.JSONDecodeError, TypeError):
            other = {}
        bg = other.get("background") if isinstance(other, dict) and isinstance(other.get("background"), dict) else {}
        allowed = _make_upload_asset_url(kind, filename) in _referenced_upload_urls_from_background(bg)

    if not allowed:
        return api_response(False, "无权限访问该资源", code=403)

    mimetype = {
        "jpg": "image/jpeg",
        "png": "image/png",
        "gif": "image/gif",
        "webp": "image/webp",
    }.get(filename.rsplit(".", 1)[-1].lower(), "application/octet-stream")
    return send_file(file_path, mimetype=mimetype, conditional=True, max_age=3600)


async def _cleanup_expired_codes():
    """清理过期绑定码并维持上限。"""
    await TelegramBindCodeOperate.cleanup_expired()
    await TelegramBindCodeOperate.trim_to_max(_MAX_BIND_CODES)


async def _delete_bind_code(bind_code: str) -> None:
    await TelegramBindCodeOperate.delete_code(bind_code)


async def _get_register_bind_telegram_id(bind_code: str) -> int | None:
    """根据注册绑定码获取已确认的 Telegram ID"""
    await _cleanup_expired_codes()
    code_info = await TelegramBindCodeOperate.get_code(bind_code)
    if not code_info or code_info.SCENE != _BIND_SCENE_REGISTER:
        return None
    return code_info.CONFIRMED_TELEGRAM_ID


def _generate_bind_code() -> str:
    """生成 8 位高强度绑定码（大写字母+数字）。"""
    alphabet = _string.ascii_uppercase + _string.digits
    return "".join(_secrets.choice(alphabet) for _ in range(8))


@users_bp.route("/me/telegram/bind-code", methods=["POST"])
@require_auth
async def generate_tg_bind_code():
    """
    生成 Telegram 绑定码

    用户获取绑定码后，在 Bot 中发送 /bind <绑定码> 完成绑定。

    Response:
        {
            "success": true,
            "data": {
                "bind_code": "123456",
                "expires_in": 300
            }
        }
    """
    from src.config import Config, TelegramConfig
    from src.core.utils import rate_limit_check

    if not Config.TELEGRAM_MODE or not TelegramConfig.BOT_TOKEN:
        return api_response(False, "Telegram Bot 未启用", code=400)

    user = g.current_user

    # 与注册版本对齐：单账号 10 分钟最多生成 5 次绑定码。
    allowed, retry_after = rate_limit_check(
        "tg_user_bind_code",
        str(user.UID),
        max_requests=5,
        window_seconds=600,
    )
    if not allowed:
        logger.warning(
            "/me/telegram/bind-code 限流命中 uid=%s retry_after=%ss",
            user.UID,
            retry_after,
        )
        return api_response(
            False,
            f"生成绑定码过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    # 已绑定则不允许再生成
    if user.TELEGRAM_ID:
        return api_response(False, "您已绑定 Telegram，如需更换请先解绑", code=400)

    # 清理过期绑定码
    await _cleanup_expired_codes()
    if await TelegramBindCodeOperate.count_active() >= _MAX_BIND_CODES:
        return api_response(False, "系统繁忙，请稍后重试", code=503)

    # 撤销该用户之前未使用的绑定码
    await TelegramBindCodeOperate.delete_user_codes(user.UID)

    # 生成新绑定码（确保不重复）
    code = _generate_bind_code()
    while await TelegramBindCodeOperate.get_code(code):
        code = _generate_bind_code()

    now = int(_time.time())
    await TelegramBindCodeOperate.upsert_code(
        code=code,
        scene=_BIND_SCENE_USER,
        uid=user.UID,
        username=user.USERNAME,
        created_at=now,
        expires_at=now + _BIND_CODE_EXPIRE,
    )

    return api_response(
        True,
        "绑定码已生成",
        {
            "bind_code": code,
            "expires_in": _BIND_CODE_EXPIRE,
        },
    )


@users_bp.route("/telegram/register/bind-code", methods=["POST"])
async def generate_tg_register_bind_code():
    """
    生成注册时使用的 Telegram 绑定码（无需登录）
    """
    from src.config import Config, TelegramConfig
    from src.core.utils import rate_limit_check

    # 公开端点：按 IP 限制单位时间内生成的绑定码数量（5 次 / 10 分钟），
    # 防止单 IP 把全局 _MAX_BIND_CODES 配额填满造成 DoS。
    client_ip = get_real_client_ip()
    allowed, retry_after = rate_limit_check(
        "tg_register_bind_code",
        client_ip,
        max_requests=5,
        window_seconds=600,
    )
    if not allowed:
        return api_response(
            False,
            f"请求过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    if not Config.TELEGRAM_MODE or not TelegramConfig.BOT_TOKEN:
        return api_response(False, "Telegram Bot 未启用", code=400)

    # 清理过期绑定码
    await _cleanup_expired_codes()
    if await TelegramBindCodeOperate.count_active() >= _MAX_BIND_CODES:
        return api_response(False, "系统繁忙，请稍后重试", code=503)

    # 生成新绑定码（确保不重复）
    code = _generate_bind_code()
    while await TelegramBindCodeOperate.get_code(code):
        code = _generate_bind_code()

    now = int(_time.time())
    await TelegramBindCodeOperate.upsert_code(
        code=code,
        scene=_BIND_SCENE_REGISTER,
        created_at=now,
        expires_at=now + _BIND_CODE_EXPIRE,
    )

    return api_response(
        True,
        "绑定码已生成",
        {
            "bind_code": code,
            "expires_in": _BIND_CODE_EXPIRE,
        },
    )


@users_bp.route("/telegram/register/bind-code/status", methods=["GET"])
async def query_tg_register_bind_code_status():
    """注册阶段轮询绑定码是否已被 Bot 端确认。无需登录。

    Query:
        code: str - 注册绑定码（8 位）

    Response data:
        {
            "code": "ABCDEFGH",
            "confirmed": true,                  // 用户已通过 /bind 完成验证
            "expires_in": 117                   // 剩余秒数；过期为 0
        }
    """
    from src.config import Config, TelegramConfig
    from src.core.utils import rate_limit_check

    client_ip = get_real_client_ip()

    # 第 0 层：IP 因连续 404 进入短期封禁名单 → 直接拒绝，不消费配额、不查 DB。
    # 这是真正能挡住"明知 404 还死命刷"的客户端的杀手锏。
    ban_remaining = _is_ip_404_banned(client_ip)
    if ban_remaining > 0:
        return api_response(
            False,
            f"请求频次异常，IP 已被临时限制 {ban_remaining} 秒",
            code=429,
        )

    code = (request.args.get("code") or "").strip().upper()
    if not code or len(code) != 8 or not code.isalnum():
        # 格式不合法不是"业务终态"——前端不应把格式错的输入也当过期处理，
        # 保留 400 + Error 抛出路径让调用方意识到自己传错了。
        return api_response(False, "绑定码无效或已过期", code=400)

    # 第 1 层：已知失效 code 短路。第一次确认不存在/过期后会进 _INVALID_CODE_CACHE，
    # 后续同 code 直接返回 200 + data.terminal=True，让前端立即停止轮询；
    # **不**消费下面的 code/ip 限速配额、**不**查 DB。
    # 同时仍要把这一次计入 IP 404 计数，避免攻击者轮换 code 绕过封禁。
    if _is_known_invalid_code(code):
        _record_404_and_maybe_ban(client_ip, code)
        return api_response(
            False,
            "绑定码无效或已过期",
            data={
                "invalid": True,
                "terminal": True,
                "code": code,
            },
        )

    # 第 2 层：双层限速，正常前端 2s 轮询不会触发。
    # - 单 code：90 次 / 60s （给网络抖动、React 严格模式、重复点击留足余量）
    # - 单 IP：600 次 / 60s （允许同公网出口多用户注册，但仍拒绝大规模扫描）
    allowed_code, retry_after_code = rate_limit_check(
        "tg_register_bind_code_status:code",
        code,
        max_requests=90,
        window_seconds=60,
    )
    if not allowed_code:
        logger.warning(
            "bind-code/status code 维度限流命中 code=%s ip=%s retry_after=%ss",
            code,
            client_ip,
            retry_after_code,
        )
        return api_response(
            False,
            f"查询过于频繁，请在 {retry_after_code} 秒后重试",
            code=429,
        )
    allowed_ip, retry_after_ip = rate_limit_check(
        "tg_register_bind_code_status:ip",
        client_ip,
        max_requests=600,
        window_seconds=60,
    )
    if not allowed_ip:
        logger.warning(
            "bind-code/status IP 维度限流命中 ip=%s retry_after=%ss",
            client_ip,
            retry_after_ip,
        )
        return api_response(
            False,
            f"查询过于频繁，请在 {retry_after_ip} 秒后重试",
            code=429,
        )

    if not Config.TELEGRAM_MODE or not TelegramConfig.BOT_TOKEN:
        return api_response(False, "Telegram Bot 未启用", code=400)

    code_info = await TelegramBindCodeOperate.get_code(code)
    if not code_info or code_info.SCENE != _BIND_SCENE_REGISTER:
        # 注册阶段 code 在 DB 不存在（或 scene 不对）= 业务终态：前端应立即放弃轮询。
        # 返回 HTTP 200 + success:false + data.terminal=true 让前端拿到 *决定性* 信号，
        # 不再依赖 message 关键字匹配（旧逻辑脆弱，且 axios/fetch 对 4xx 抛错会绕过分支）。
        # IP 404 计数 + invalid-cache 仍然写入，保证防滥用层级照常生效。
        _mark_invalid_code(code)
        _record_404_and_maybe_ban(client_ip, code)
        return api_response(
            False,
            "绑定码无效或已过期",
            data={
                "invalid": True,
                "terminal": True,
                "code": code,
            },
        )

    remaining = max(0, int(code_info.EXPIRES_AT - _time.time()))
    # remaining<=0 表示已过期但 DB 还没清理：同样是终态，按 invalid 处理而非 success。
    if remaining <= 0:
        _mark_invalid_code(code)
        _record_404_and_maybe_ban(client_ip, code)
        return api_response(
            False,
            "绑定码无效或已过期",
            data={
                "invalid": True,
                "terminal": True,
                "code": code,
            },
        )

    return api_response(
        True,
        "获取成功",
        {
            "code": code_info.CODE,
            "confirmed": bool(code_info.CONFIRMED_TELEGRAM_ID),
            "expires_in": remaining,
            "invalid": False,
            "terminal": bool(code_info.CONFIRMED_TELEGRAM_ID),  # 已确认绑定也是终态
        },
    )


async def confirm_tg_bind_internal(bind_code: str, telegram_id: int) -> tuple[bool, str, dict[str, Any], int]:
    """供 Bot 与内部接口复用的 Telegram 绑定确认逻辑。"""
    bind_code = (bind_code or "").strip().upper()
    if not bind_code or not telegram_id:
        return False, "参数缺失", {}, 400

    try:
        telegram_id = int(telegram_id)
    except (TypeError, ValueError):
        return False, "telegram_id 无效", {}, 400

    await _cleanup_expired_codes()
    code_info = await TelegramBindCodeOperate.get_code(bind_code)
    if not code_info:
        return False, "绑定码无效或已过期", {}, 400

    if code_info.SCENE == _BIND_SCENE_USER:
        uid = code_info.UID
        if not uid:
            return False, "绑定码数据损坏", {}, 400

        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing and existing.UID != uid:
            return False, "该 Telegram 已绑定其他账号，一个 Telegram 只能绑定一个账号", {}, 400

        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            return False, "用户不存在", {}, 404

        if user.TELEGRAM_ID:
            await _delete_bind_code(bind_code)
            return False, "该账号已绑定 Telegram", {}, 400

        # 强制要求加入指定群组（仅在配置了 GROUP_ID 时生效）。
        # sync_roster=True：顺手把这次探测到的成员状态写进花名册，
        # 弥补 Bot API 无法主动枚举群成员的缺口（用户从未发言时被动收集会漏）。
        from src.services.telegram_membership import TelegramMembershipService

        ok, missing = await TelegramMembershipService.check_user_in_groups(
            telegram_id,
            strict=True,
            sync_roster=True,
        )
        if not ok:
            return (
                False,
                TelegramMembershipService.format_missing_message(missing),
                {
                    "reason": "not_in_required_group",
                    "missing_groups": [m.to_dict() for m in missing],
                },
                403,
            )

        user.TELEGRAM_ID = telegram_id
        await UserOperate.update_user(user)
        await _delete_bind_code(bind_code)

        logger.info(f"用户 {user.USERNAME} 通过 Bot 绑定 Telegram: {telegram_id}")

        from src.core.utils import format_expire_time
        from src.db.user import Role

        role_map = {
            Role.ADMIN.value: "管理员",
            Role.WHITE_LIST.value: "白名单",
            Role.NORMAL.value: "普通用户",
        }

        return (
            True,
            "Telegram 绑定成功",
            {
                "uid": uid,
                "username": user.USERNAME,
                "telegram_id": telegram_id,
                "emby_id": user.EMBYID or None,
                "role": role_map.get(user.ROLE, "未知"),
                "active": user.ACTIVE_STATUS,
                "expired_at": format_expire_time(user.EXPIRED_AT),
            },
            200,
        )

    if code_info.SCENE == _BIND_SCENE_REGISTER:
        if code_info.CONFIRMED_TELEGRAM_ID and code_info.CONFIRMED_TELEGRAM_ID != telegram_id:
            return False, "该绑定码已被其他 Telegram 账号使用", {}, 400

        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            return False, "该 Telegram 已绑定其他账号，一个 Telegram 只能绑定一个账号", {}, 400

        # 注册阶段也强制检查群组成员资格，避免绕过 Bot 后再注册。
        # 同样借这次 get_chat_member 把花名册同步一次（与 /bind 流程一致）。
        from src.services.telegram_membership import TelegramMembershipService

        ok, missing = await TelegramMembershipService.check_user_in_groups(
            telegram_id,
            strict=True,
            sync_roster=True,
        )
        if not ok:
            return (
                False,
                TelegramMembershipService.format_missing_message(missing),
                {
                    "reason": "not_in_required_group",
                    "missing_groups": [m.to_dict() for m in missing],
                },
                403,
            )

        await TelegramBindCodeOperate.upsert_code(
            code=code_info.CODE,
            scene=code_info.SCENE,
            uid=code_info.UID,
            username=code_info.USERNAME,
            confirmed_telegram_id=telegram_id,
            created_at=code_info.CREATED_AT,
            expires_at=code_info.EXPIRES_AT,
        )

        logger.info(f"注册绑定码 {bind_code} 已由 Telegram {telegram_id} 验证")
        return (
            True,
            "Telegram 绑定码验证成功",
            {
                "telegram_id": telegram_id,
            },
            200,
        )

    return False, "绑定码类型无效", {}, 400


@users_bp.route("/me/telegram/bind-confirm", methods=["POST"])
async def confirm_tg_bind():
    """
    Bot 调用此接口完成绑定（内部接口）

    Request:
        {
            "bind_code": "123456",
            "telegram_id": 123456789,
            "bot_secret": "..."
        }
    """
    data = request.get_json() or {}
    bind_code = data.get("bind_code", "").strip().upper()
    telegram_id = data.get("telegram_id")
    bot_secret = data.get("bot_secret", "")

    from src.config import SecurityConfig

    expected_secret = (SecurityConfig.BOT_INTERNAL_SECRET or "").strip()
    if not bot_secret or not expected_secret or not hmac.compare_digest(str(bot_secret), str(expected_secret)):
        return api_response(False, "未授权", code=403)

    ok, message, payload, status_code = await confirm_tg_bind_internal(bind_code, telegram_id)
    return api_response(ok, message, payload, code=status_code)


# ==================== 用户设置 ====================


@users_bp.route("/me/settings", methods=["GET"])
@require_auth
async def get_my_settings():
    """获取用户所有设置"""
    from src.config import RegisterConfig, DeviceLimitConfig, Config, EmbyConfig, BangumiSyncConfig
    from src.services import EmbyService

    user = g.current_user

    status = await EmbyService.get_user_status(user)
    pending_request = await UserService.get_telegram_rebind_request(user.UID)
    has_pending_rebind_request = bool(pending_request and pending_request.STATUS == "pending")

    return api_response(
        True,
        "获取成功",
        {
            # 用户设置
            "bgm_mode": user.BGM_MODE,
            "bgm_token_set": bool((user.BGM_TOKEN or "").strip()),
            "api_key_enabled": user.APIKEY_STATUS,
            "emby_status": {
                "is_synced": status.is_synced,
                "is_active": status.is_active,
                "active_sessions": status.active_sessions,
                "message": status.message,
            },
            # Telegram 绑定
            "telegram": {
                "bound": bool(user.TELEGRAM_ID),
                "force_bind": Config.FORCE_BIND_TELEGRAM,
                "can_unbind": not Config.FORCE_BIND_TELEGRAM and bool(user.TELEGRAM_ID),
                "can_change": bool(user.TELEGRAM_ID) and not has_pending_rebind_request,
                "pending_rebind_request": has_pending_rebind_request,
                "rebind_request_status": pending_request.STATUS if pending_request else None,
                "rebind_request_id": pending_request.ID if pending_request else None,
            },
            # 系统配置
            "system_config": {
                "device_limit_enabled": DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
                "max_devices": DeviceLimitConfig.MAX_DEVICES,
                "max_streams": DeviceLimitConfig.MAX_STREAMS,
                "bangumi_sync_enabled": BangumiSyncConfig.ENABLED,
            },
        },
    )


# ==================== 背景管理 ====================

_CSS_URL_RE = re.compile(r'^\s*url\(\s*([\'"]?)([^\'")]+)\1\s*\)\s*$', re.IGNORECASE)
_CSS_BG_FUNC_RE = re.compile(
    r"^\s*(linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|"
    r"repeating-radial-gradient|repeating-conic-gradient)\s*\(",
    re.IGNORECASE,
)


def _is_disallowed_bg_host(host: str) -> bool:
    """拦截会让访客浏览器去探测内网/元数据服务的危险主机名。

    背景图片 URL 会被所有访问页面的用户的浏览器请求，攻击者可以利用这一点
    把内网地址写进背景图配置，让被访问者代为发起 SSRF 探测。这里在「写入时」
    就拒绝掉所有解析为私网 / 回环 / 链路本地 / 多播 / 云元数据 (169.254.169.254)
    的 host，以及裸 IP 之外的常见 localhost 别名。
    """
    import ipaddress

    if not host:
        return True
    h = host.strip().strip("[").strip("]").lower()
    # localhost 别名 / metadata.google.internal 等
    bad_names = {
        "localhost",
        "localhost.localdomain",
        "ip6-localhost",
        "ip6-loopback",
        "metadata.google.internal",
        "metadata.goog",
    }
    if h in bad_names or h.endswith(".localhost"):
        return True
    # 尝试解析为 IP；解析不出来就当外网域名放行
    try:
        ip = ipaddress.ip_address(h)
    except ValueError:
        return False
    return bool(
        ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_multicast or ip.is_reserved or ip.is_unspecified
    )


def _is_valid_background_url(value: str) -> bool:
    """允许:
    - 空字符串
    - 站内相对路径 ("/uploads/...")
    - 裸 http(s):// URL（host 不能解析到私网/回环/元数据地址）
    - CSS url("...") 包装（内部 URL 同样需通过校验）
    - linear-gradient / radial-gradient 等 CSS 背景函数
    """
    if not value:
        return True
    stripped = value.strip()
    if not stripped:
        return True

    # 站内相对路径（只允许受控上传资源）
    if stripped.startswith("/"):
        return _is_safe_upload_relative_url(stripped)

    # CSS url("...") 包装：解析内部 URL 后递归校验
    url_match = _CSS_URL_RE.match(stripped)
    if url_match:
        inner = url_match.group(2).strip()
        if not inner:
            return False
        if inner.startswith("/"):
            return _is_safe_upload_relative_url(inner)
        try:
            parsed = urlparse(inner)
        except Exception:
            return False
        if parsed.scheme not in ("http", "https") or not parsed.netloc:
            return False
        return not _is_disallowed_bg_host(parsed.hostname or "")

    # gradient / image-set 等 CSS 函数：直接放行（仅做长度限制）
    if _CSS_BG_FUNC_RE.match(stripped):
        return True

    # 裸 http(s):// URL
    try:
        parsed = urlparse(stripped)
    except Exception:
        return False
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        return False
    return not _is_disallowed_bg_host(parsed.hostname or "")


@users_bp.route("/<int:uid>/background", methods=["GET"])
@require_auth
async def get_user_background(uid: int):
    """获取用户背景配置"""
    if g.current_user.UID != uid and g.current_user.ROLE != Role.ADMIN.value:
        return api_response(False, "无权限查看其他用户背景", code=403)

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    # 从 OTHER 字段解析背景配置
    background_config = {}
    if user.OTHER:
        try:
            import json

            data = json.loads(user.OTHER)
            background_config = data.get("background", {})
        except:
            pass

    background_config = _rewrite_upload_urls_in_background(background_config)
    return api_response(True, "获取成功", {"background": json.dumps(background_config) if background_config else None})


@users_bp.route("/me/background", methods=["PUT"])
@require_auth
async def update_user_background():
    """
    更新用户背景配置

    Request:
        {
            "lightBg": "linear-gradient(...)",  // 浅色主题背景
            "darkBg": "linear-gradient(...)",    // 暗色主题背景
            "lightBgImage": "url(...)",         // 浅色背景图片
            "darkBgImage": "url(...)",          // 暗色背景图片
            "lightFlow": false,                   // 浅色背景流光开关
            "darkFlow": false,                    // 暗色背景流光开关
            "lightBlur": 0,                       // 浅色背景模糊(px)
            "darkBlur": 0,                        // 暗色背景模糊(px)
            "lightOpacity": 100,                  // 浅色背景透明度(0-100)
            "darkOpacity": 100                    // 暗色背景透明度(0-100)
        }
    """
    import json
    from src.core.utils import timestamp

    # 检查认证
    if not hasattr(g, "current_user") or g.current_user is None:
        return api_response(False, "需要认证", code=401)

    user = g.current_user
    data = request.get_json() or {}

    # 验证输入
    light_bg = data.get("lightBg", "").strip()
    dark_bg = data.get("darkBg", "").strip()
    light_bg_image = data.get("lightBgImage", "").strip()
    dark_bg_image = data.get("darkBgImage", "").strip()

    if not light_bg and not dark_bg and not light_bg_image and not dark_bg_image:
        return api_response(False, "至少需要一个背景配置", code=400)

    # 背景URL或CSS长度限制
    MAX_BG_LENGTH = 2000
    if len(light_bg) > MAX_BG_LENGTH or len(dark_bg) > MAX_BG_LENGTH:
        return api_response(False, f"背景配置过长，最多 {MAX_BG_LENGTH} 字符", code=400)
    if len(light_bg_image) > MAX_BG_LENGTH or len(dark_bg_image) > MAX_BG_LENGTH:
        return api_response(False, f"背景图片URL过长", code=400)
    if not _is_valid_background_url(light_bg_image) or not _is_valid_background_url(dark_bg_image):
        return api_response(
            False,
            "背景图片格式不合法，支持站内相对路径、http(s) URL、CSS url(...) 包装或 linear-gradient 等背景函数",
            code=400,
        )

    # 保存到 OTHER 字段
    try:
        other_data = {}
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}

        existing_background = (
            other_data.get("background", {}) if isinstance(other_data.get("background", {}), dict) else {}
        )
        light_flow = parse_bool(
            data.get("lightFlow"),
            default=parse_bool(existing_background.get("lightFlow"), default=False),
        )
        dark_flow = parse_bool(
            data.get("darkFlow"),
            default=parse_bool(existing_background.get("darkFlow"), default=False),
        )

        def _clamp_int(value, default, min_value, max_value):
            try:
                num = int(value)
            except Exception:
                num = default
            return max(min_value, min(max_value, num))

        light_blur = _clamp_int(
            data.get("lightBlur", existing_background.get("lightBlur", 0)),
            0,
            0,
            30,
        )
        dark_blur = _clamp_int(
            data.get("darkBlur", existing_background.get("darkBlur", 0)),
            0,
            0,
            30,
        )
        light_opacity = _clamp_int(
            data.get("lightOpacity", existing_background.get("lightOpacity", 100)),
            100,
            10,
            100,
        )
        dark_opacity = _clamp_int(
            data.get("darkOpacity", existing_background.get("darkOpacity", 100)),
            100,
            10,
            100,
        )

        other_data["background"] = {
            "lightBg": light_bg,
            "darkBg": dark_bg,
            "lightBgImage": light_bg_image,
            "darkBgImage": dark_bg_image,
            "lightFlow": light_flow,
            "darkFlow": dark_flow,
            "lightBlur": light_blur,
            "darkBlur": dark_blur,
            "lightOpacity": light_opacity,
            "darkOpacity": dark_opacity,
            "updated_at": timestamp(),
        }

        user.OTHER = json.dumps(other_data)
        await UserOperate.update_user(user)

        response_background = _rewrite_upload_urls_in_background(other_data["background"])
        return api_response(True, "背景更新成功", {"background": json.dumps(response_background)})
    except Exception as e:
        logger.error(f"保存背景配置失败: {e}")
        return api_response(False, "保存失败", code=500)


@users_bp.route("/me/background", methods=["DELETE"])
@require_auth
async def delete_user_background():
    """删除用户背景配置，恢复默认背景"""
    import json

    # 检查认证
    if not hasattr(g, "current_user") or g.current_user is None:
        return api_response(False, "需要认证", code=401)

    user = g.current_user

    try:
        other_data = {}
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}

        # 删除背景配置
        if "background" in other_data:
            del other_data["background"]

        user.OTHER = json.dumps(other_data) if other_data else ""
        await UserOperate.update_user(user)

        return api_response(True, "背景已重置为默认")
    except Exception as e:
        logger.error(f"删除背景配置失败: {e}")
        return api_response(False, "删除失败", code=500)


@users_bp.route("/me/background/upload", methods=["POST"])
@require_auth
async def upload_background_image():
    """
    上传背景图片

    Request:
        Form-data:
            file: 图片文件 (max 5MB)
            type: 'light' 或 'dark' - 指定这是浅色或暗色背景

    Response:
        {
            "success": true,
            "data": {
                "url": "/uploads/backgrounds/xxx.jpg",
                "type": "light"
            }
        }
    """
    import os
    import uuid
    from src.core.utils import rate_limit_check

    # 检查认证
    if not hasattr(g, "current_user") or g.current_user is None:
        return api_response(False, "需要认证", code=401)

    user = g.current_user

    # 限流：每个用户 10 次 / 分钟，防止刷爆磁盘
    allowed, retry_after = rate_limit_check(
        "upload_background",
        f"uid:{user.UID}",
        max_requests=10,
        window_seconds=60,
    )
    if not allowed:
        return api_response(
            False,
            f"上传过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    # 检查文件
    if "file" not in request.files:
        return api_response(False, "未找到文件", code=400)

    file = request.files["file"]
    if file.filename == "":
        return api_response(False, "文件名为空", code=400)

    # 检查背景类型
    bg_type = request.form.get("type", "light").lower()
    if bg_type not in ["light", "dark"]:
        return api_response(False, "背景类型必须为 'light' 或 'dark'", code=400)

    # 读取文件头并识别图片类型
    header = file.read(32)
    file.seek(0)
    detected_ext = _detect_image_extension(header)
    if not detected_ext:
        return api_response(False, "文件内容不是有效的图片", code=400)

    # 验证文件大小
    file.seek(0, os.SEEK_END)
    file_size = file.tell()
    file.seek(0)

    MAX_SIZE = request.max_content_length or 5 * 1024 * 1024
    if file_size > MAX_SIZE:
        return api_response(False, f"文件过大，最大 {MAX_SIZE // (1024*1024)}MB", code=400)

    try:
        # 创建上传目录
        upload_root = _get_upload_root_path()
        upload_dir = (upload_root / "backgrounds").resolve()
        if not upload_dir.is_relative_to(upload_root):
            return api_response(False, "上传目录配置无效", code=500)
        upload_dir.mkdir(parents=True, exist_ok=True)

        # 生成唯一文件名
        filename = f"{uuid.uuid4().hex}.{detected_ext}"
        filepath = upload_dir / filename

        # 保存文件
        file.save(str(filepath))

        # 生成 URL
        file_url = _make_upload_asset_url("backgrounds", filename)

        return api_response(True, "上传成功", {"url": file_url, "type": bg_type, "filename": filename})
    except Exception as e:
        logger.error(f"上传背景图片失败: {e}")
        return api_response(False, "上传失败", code=500)


# ==================== 头像管理 ====================


@users_bp.route("/<int:uid>/avatar", methods=["GET"])
@require_auth
async def get_user_avatar(uid: int):
    """获取用户头像"""
    if g.current_user.UID != uid and g.current_user.ROLE != Role.ADMIN.value:
        return api_response(False, "无权限查看其他用户头像", code=403)

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    return api_response(
        True,
        "获取成功",
        {
            "avatar": _rewrite_avatar_url(user.AVATAR),
            "uid": user.UID,
            "username": user.USERNAME,
        },
    )


@users_bp.route("/me/avatar/upload", methods=["POST"])
@require_auth
async def upload_avatar():
    """
    上传用户头像

    Request:
        Form-data:
            file: 头像图片文件 (max 2MB，推荐 200x200px)

    Response:
        {
            "success": true,
            "data": {
                "avatar_url": "/uploads/avatars/xxx.jpg"
            }
        }
    """
    import os
    import uuid
    from src.core.utils import rate_limit_check

    user = g.current_user

    # 限流：每个用户 10 次 / 分钟
    allowed, retry_after = rate_limit_check(
        "upload_avatar",
        f"uid:{user.UID}",
        max_requests=10,
        window_seconds=60,
    )
    if not allowed:
        return api_response(
            False,
            f"上传过于频繁，请在 {retry_after} 秒后重试",
            code=429,
        )

    # 检查文件
    if "file" not in request.files:
        return api_response(False, "缺少文件", code=400)

    file = request.files["file"]
    if file.filename == "":
        return api_response(False, "未选择文件", code=400)

    try:
        # 验证文件类型（Content-Type + magic bytes）
        allowed_types = {"image/jpeg", "image/png", "image/gif", "image/webp"}
        if file.content_type not in allowed_types:
            return api_response(False, "只支持 JPG、PNG、GIF、WebP 格式的图片", code=400)

        # 读取文件头并识别图片类型
        header = file.read(32)
        file.seek(0)
        detected_ext = _detect_image_extension(header)
        if not detected_ext:
            return api_response(False, "文件内容不是有效的图片", code=400)

        # 验证文件大小
        file.seek(0, 2)
        file_size = file.tell()
        file.seek(0)

        if file_size > 2 * 1024 * 1024:  # 2MB
            return api_response(False, "文件大小不能超过 2MB", code=400)

        # 创建上传目录
        upload_root = _get_upload_root_path()
        upload_dir = (upload_root / "avatars").resolve()
        if not upload_dir.is_relative_to(upload_root):
            return api_response(False, "上传目录配置无效", code=500)
        upload_dir.mkdir(parents=True, exist_ok=True)

        # 生成唯一文件名
        filename = f"{uuid.uuid4().hex}.{detected_ext}"
        filepath = upload_dir / filename

        # 保存文件
        file.save(str(filepath))

        # 生成 URL
        avatar_url = _make_upload_asset_url("avatars", filename)

        # 更新用户头像
        user.AVATAR = avatar_url
        await UserOperate.update_user(user)

        logger.info(f"用户上传头像: {user.USERNAME} -> {avatar_url}")

        return api_response(
            True,
            "头像上传成功",
            {
                "avatar_url": avatar_url,
            },
        )
    except Exception as e:
        logger.error(f"上传头像失败: {e}")
        return api_response(False, "上传失败", code=500)


@users_bp.route("/me/avatar", methods=["DELETE"])
@require_auth
async def delete_avatar():
    """删除用户头像"""
    user = g.current_user

    if not user.AVATAR:
        return api_response(False, "用户未设置头像", code=400)

    # 删除头像文件
    try:
        avatar_url = (user.AVATAR or "").strip()
        if avatar_url.startswith("/uploads/avatars/") or avatar_url.startswith("/api/v1/users/assets/avatars/"):
            file_path = _resolve_upload_file_path(avatar_url, required_subdir="avatars")
            if file_path and file_path.exists() and file_path.is_file():
                file_path.unlink()
    except Exception as e:
        logger.warning(f"删除头像文件失败: {e}")

    # 清除头像 URL
    user.AVATAR = None
    await UserOperate.update_user(user)

    return api_response(True, "头像已删除")


# ==================== API Key 管理 ====================


def _serialize_api_key(model) -> dict:
    """API Key 列表序列化（不返回明文）。"""
    from src.db.apikey import ApiKeyOperate

    masked = f"{model.KEY_PREFIX}…{model.KEY_SUFFIX}" if (model.KEY_PREFIX or model.KEY_SUFFIX) else "****"
    return {
        "id": model.ID,
        "name": model.NAME,
        "key": masked,  # 仅展示用，明文已不再保留
        "key_prefix": model.KEY_PREFIX,
        "key_suffix": model.KEY_SUFFIX,
        "enabled": model.ENABLED,
        "allow_query": model.ALLOW_QUERY,
        "permissions": ApiKeyOperate.get_permissions(model),
        "rate_limit": model.RATE_LIMIT,
        "request_count": model.REQUEST_COUNT,
        "last_used": model.LAST_USED_AT,
        "created_at": model.CREATED_AT,
        "expired_at": model.EXPIRED_AT,
    }


@users_bp.route("/me/apikeys", methods=["GET"])
@require_auth
async def get_my_api_keys():
    """获取我的 API Keys 列表（不返回明文，明文仅创建时返回一次）。"""
    from src.db.apikey import ApiKeyOperate

    api_keys = await ApiKeyOperate.get_user_api_keys(g.current_user.UID)
    keys_list = [_serialize_api_key(k) for k in api_keys]

    return api_response(
        True,
        "获取成功",
        {
            "keys": keys_list,
            "total": len(keys_list),
        },
    )


@users_bp.route("/me/apikeys", methods=["POST"])
@require_auth
async def generate_api_key():
    """
    生成新的 API Key（明文仅在响应中返回一次，请妥善保存）

    Request:
        {
            "name": "My API Key",
            "allow_query": true,
            "rate_limit": 100,
            "expired_at": -1,
            "permissions": ["account:read"]   // 可选
        }
    """
    from src.db.apikey import ApiKeyOperate

    data = request.get_json() or {}
    name = data.get("name")
    allow_query = parse_bool(data.get("allow_query"), default=True)
    rate_limit = data.get("rate_limit", 100)
    expired_at = data.get("expired_at", -1)
    permissions = data.get("permissions")

    try:
        rate_limit = int(rate_limit)
    except (TypeError, ValueError):
        return api_response(False, "速率限制必须是整数", code=400)
    if rate_limit < 0:
        return api_response(False, "速率限制不能为负数", code=400)

    try:
        expired_at = int(expired_at) if expired_at is not None else -1
    except (TypeError, ValueError):
        return api_response(False, "过期时间必须是整数", code=400)

    try:
        api_key, plaintext = await ApiKeyOperate.create_api_key(
            uid=g.current_user.UID,
            name=name,
            allow_query=allow_query,
            rate_limit=rate_limit,
            expired_at=expired_at,
            permissions=permissions if isinstance(permissions, list) else None,
        )

        logger.info(f"用户生成 API Key: {g.current_user.USERNAME} -> {api_key.ID}")

        return api_response(
            True,
            "API Key 生成成功，请立即保存（明文不会再次显示）",
            {
                "id": api_key.ID,
                "key": plaintext,
                "name": api_key.NAME,
                "created_at": api_key.CREATED_AT,
            },
        )
    except Exception as e:
        logger.error(f"生成 API Key 失败: {e}", exc_info=True)
        return api_response(False, "生成失败", code=500)


@users_bp.route("/me/apikeys/<int:key_id>", methods=["PUT"])
@require_auth
async def update_api_key(key_id: int):
    """
    更新 API Key 配置

    Request:
        {
            "name": "Updated Name",
            "enabled": true,
            "allow_query": true,
            "rate_limit": 100,
            "expired_at": -1,
            "permissions": ["account:read"]
        }
    """
    from src.db.apikey import ApiKeyOperate

    api_key = await ApiKeyOperate.get_api_key_by_id(key_id)
    if not api_key or api_key.UID != g.current_user.UID:
        return api_response(False, "API Key 不存在或无权限修改", code=404)

    data = request.get_json() or {}
    perms = data.get("permissions")
    if perms is not None and not isinstance(perms, list):
        return api_response(False, "permissions 必须是数组", code=400)

    try:
        updated_key = await ApiKeyOperate.update_api_key(
            key_id=key_id,
            name=data.get("name"),
            enabled=data.get("enabled"),
            allow_query=data.get("allow_query"),
            rate_limit=data.get("rate_limit"),
            expired_at=data.get("expired_at"),
            permissions=perms,
        )

        logger.info(f"用户更新 API Key: {g.current_user.USERNAME} -> {key_id}")

        return api_response(True, "API Key 更新成功", _serialize_api_key(updated_key))
    except Exception as e:
        logger.error(f"更新 API Key 失败: {e}", exc_info=True)
        return api_response(False, "更新失败", code=500)


@users_bp.route("/me/apikeys/<int:key_id>", methods=["DELETE"])
@require_auth
async def delete_api_key(key_id: int):
    """删除 API Key（不可恢复）。"""
    from src.db.apikey import ApiKeyOperate

    api_key = await ApiKeyOperate.get_api_key_by_id(key_id)
    if not api_key or api_key.UID != g.current_user.UID:
        return api_response(False, "API Key 不存在或无权限删除", code=404)

    try:
        success = await ApiKeyOperate.delete_api_key(key_id)
        if success:
            logger.info(f"用户删除 API Key: {g.current_user.USERNAME} -> {key_id}")
            return api_response(True, "API Key 已删除")
        return api_response(False, "删除失败", code=500)
    except Exception as e:
        logger.error(f"删除 API Key 失败: {e}", exc_info=True)
        return api_response(False, "删除失败", code=500)
