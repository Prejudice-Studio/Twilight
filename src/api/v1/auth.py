"""
认证 API

提供用户认证、登录、登出等功能
"""

import hashlib
import logging
import time
from collections import defaultdict
from functools import wraps
from typing import Optional, Callable, Any
from flask import Blueprint, request, g, jsonify

from src.config import APIConfig, Config, SecurityConfig
from src.core.utils import verify_password, timestamp, hash_password, generate_password, rate_limit_check
from src.core.request_utils import get_real_client_ip
from src.db.user import UserOperate, UserModel, Role, AuthTokenOperate
from src.db.login_log import LoginLogOperate, LoginLogModel
from src.services import UserService

try:
    from redis.asyncio import Redis
except ImportError:  # pragma: no cover - optional dependency
    Redis = None  # type: ignore

logger = logging.getLogger(__name__)

auth_bp = Blueprint("auth", __name__, url_prefix="/auth")

# token 存储：优先使用 Redis，未配置时回退数据库（支持多进程）
_redis_client: Optional["Redis"] = None
_AUTH_COOKIE_NAME = "twilight_session"

# 登录速率限制：IP -> (失败次数, 首次失败时间戳)
_login_rate_limit: dict[str, dict] = defaultdict(lambda: {"count": 0, "first_fail": 0})
_MAX_TRACKED_LOGIN_IPS = 50000


def _login_rate_window() -> int:
    """登录限流窗口（秒），跟随配置 lockout_minutes。"""
    return max(60, int(SecurityConfig.LOCKOUT_MINUTES or 15) * 60)


def _cleanup_login_rate_limit(now: int) -> None:
    """清理过期与低价值记录，防止 IP 字典无限增长。"""
    window = _login_rate_window()
    stale_ips = [ip for ip, rec in _login_rate_limit.items() if now - int(rec.get("first_fail", 0) or 0) > window]
    for ip in stale_ips:
        _login_rate_limit.pop(ip, None)

    if len(_login_rate_limit) <= _MAX_TRACKED_LOGIN_IPS:
        return

    overflow = len(_login_rate_limit) - _MAX_TRACKED_LOGIN_IPS
    sorted_items = sorted(_login_rate_limit.items(), key=lambda item: int(item[1].get("first_fail", 0) or 0))
    for ip, _ in sorted_items[:overflow]:
        _login_rate_limit.pop(ip, None)


def _check_login_rate_limit(ip: str) -> Optional[str]:
    """
    检查 IP 是否超过登录失败阈值

    :return: 如果超限则返回错误消息，否则返回 None
    """
    threshold = SecurityConfig.LOGIN_FAIL_THRESHOLD
    if threshold <= 0:
        return None

    now = timestamp()
    _cleanup_login_rate_limit(now)
    window = _login_rate_window()
    record = _login_rate_limit[ip]

    # 窗口过期则重置
    if now - record["first_fail"] > window:
        record["count"] = 0
        record["first_fail"] = 0
        return None

    if record["count"] >= threshold:
        remaining = window - (now - record["first_fail"])
        return f"登录尝试过于频繁，请在 {max(remaining // 60, 1)} 分钟后重试"

    return None


def _record_login_failure(ip: str):
    """记录一次登录失败"""
    now = timestamp()
    _cleanup_login_rate_limit(now)
    window = _login_rate_window()
    record = _login_rate_limit[ip]
    if record["count"] == 0 or now - record["first_fail"] > window:
        record["count"] = 1
        record["first_fail"] = now
    else:
        record["count"] += 1


def _clear_login_failures(ip: str):
    """登录成功后清除失败记录"""
    _login_rate_limit.pop(ip, None)


# ==================== 工具函数 ====================


def api_response(success: bool, message: str, data: Any = None, code: int = 200):
    """
    统一的 API 响应格式

    :param success: 是否成功
    :param message: 消息
    :param data: 数据
    :param code: HTTP 状态码
    """
    response = {
        "success": success,
        "message": message,
        "data": data,
        "timestamp": timestamp(),
    }
    return jsonify(response), code


def _token_key(token: str) -> str:
    return f"tw:token:{token}"


def _user_tokens_key(uid: int) -> str:
    return f"tw:user:{uid}:tokens"


def _set_auth_cookie(response, token: str) -> None:
    """写入 HttpOnly 会话 Cookie。"""
    cookie_name = (APIConfig.SESSION_COOKIE_NAME or _AUTH_COOKIE_NAME).strip() or _AUTH_COOKIE_NAME
    response.set_cookie(
        cookie_name,
        token,
        max_age=APIConfig.TOKEN_EXPIRE,
        httponly=True,
        secure=bool(APIConfig.SESSION_COOKIE_SECURE),
        samesite=APIConfig.SESSION_COOKIE_SAMESITE,
        domain=(APIConfig.SESSION_COOKIE_DOMAIN or None),
        path=APIConfig.SESSION_COOKIE_PATH or "/",
    )


def _clear_auth_cookie(response) -> None:
    """清理会话 Cookie。"""
    cookie_name = (APIConfig.SESSION_COOKIE_NAME or _AUTH_COOKIE_NAME).strip() or _AUTH_COOKIE_NAME
    response.set_cookie(
        cookie_name,
        "",
        max_age=0,
        expires=0,
        httponly=True,
        secure=bool(APIConfig.SESSION_COOKIE_SECURE),
        samesite=APIConfig.SESSION_COOKIE_SAMESITE,
        domain=(APIConfig.SESSION_COOKIE_DOMAIN or None),
        path=APIConfig.SESSION_COOKIE_PATH or "/",
    )


async def _get_redis() -> Optional["Redis"]:
    """延迟初始化 Redis 客户端，未配置时返回 None（会回退 DB token 存储）。"""
    global _redis_client
    if not Config.REDIS_URL:
        return None
    if Redis is None:
        logger.warning("检测到 REDIS_URL 但未安装 redis 依赖，回退为数据库 token 存储")
        return None
    if _redis_client is None:
        _redis_client = Redis.from_url(Config.REDIS_URL, decode_responses=True, encoding="utf-8")
    return _redis_client


async def _load_token(token: str) -> Optional[dict]:
    redis_client = await _get_redis()
    if redis_client:
        try:
            data = await redis_client.hgetall(_token_key(token))
            if not data:
                return None
            return {
                "uid": int(data["uid"]),
                "created_at": int(data["created_at"]),
                "expires_at": int(data["expires_at"]),
            }
        except (KeyError, ValueError):
            await redis_client.delete(_token_key(token))
            return None
        except Exception as exc:  # pragma: no cover - redis 挂掉时回退
            logger.warning("Redis token store 读取失败，回退数据库：%s", exc)
    token_model = await AuthTokenOperate.get_token(token)
    if not token_model:
        return None
    return {
        "uid": int(token_model.UID),
        "created_at": int(token_model.CREATED_AT),
        "expires_at": int(token_model.EXPIRES_AT),
    }


async def _store_token(token: str, uid: int) -> dict:
    now = timestamp()
    payload = {
        "uid": uid,
        "created_at": now,
        "expires_at": now + APIConfig.TOKEN_EXPIRE,
    }
    redis_client = await _get_redis()
    if redis_client:
        try:
            pipe = redis_client.pipeline()
            pipe.hset(_token_key(token), mapping=payload)
            pipe.expire(_token_key(token), APIConfig.TOKEN_EXPIRE)
            pipe.sadd(_user_tokens_key(uid), token)
            pipe.expire(_user_tokens_key(uid), APIConfig.TOKEN_EXPIRE)
            await pipe.execute()
        except Exception as exc:  # pragma: no cover
            logger.warning("Redis token store 写入失败，回退数据库：%s", exc)
            await AuthTokenOperate.upsert_token(token, uid, payload["created_at"], payload["expires_at"])
    else:
        await AuthTokenOperate.upsert_token(token, uid, payload["created_at"], payload["expires_at"])
    return payload


def require_auth(f: Callable) -> Callable:
    """要求认证的装饰器（必须提供有效 Bearer Token）"""

    @wraps(f)
    async def wrapper(*args, **kwargs):
        # 从请求头获取 token
        auth_header = request.headers.get("Authorization", "")
        token = ""
        token_from_cookie = False
        if auth_header and auth_header.startswith("Bearer "):
            token = auth_header[7:]
        else:
            cookie_name = (APIConfig.SESSION_COOKIE_NAME or _AUTH_COOKIE_NAME).strip() or _AUTH_COOKIE_NAME
            token = (request.cookies.get(cookie_name) or "").strip()
            token_from_cookie = bool(token)

        if not token:
            return api_response(False, "需要认证", code=401)

        if token_from_cookie and request.method not in {"GET", "HEAD", "OPTIONS"}:
            # Cookie 会自动随跨站请求发送；写操作必须带 JS 可设置的自定义头，阻断表单/图片等 CSRF。
            if request.headers.get("X-Twilight-Client") != "webui":
                return api_response(False, "缺少 CSRF 防护请求头", code=403)

        # 验证 token 格式（应该是 64 位十六进制字符串）
        if len(token) != 64 or not all(c in "0123456789abcdef" for c in token):
            return api_response(False, "认证令牌格式无效", code=401)

        # 验证 token
        token_data = await _load_token(token)
        if not token_data:
            return api_response(False, "认证令牌无效或已过期", code=401)

        # 检查 token 是否过期
        if timestamp() > token_data["expires_at"]:
            # 清理过期 token
            await revoke_token(token, token_data.get("uid"))
            return api_response(False, "认证令牌已过期", code=401)

        # 获取用户
        user = await UserOperate.get_user_by_uid(token_data["uid"])
        if not user:
            await revoke_token(token, token_data.get("uid"))
            return api_response(False, "用户不存在", code=401)

        # 检查用户状态
        if not user.ACTIVE_STATUS:
            return api_response(False, "账户已被禁用", code=403)

        # 将用户存储到 g 对象中
        g.current_user = user
        g.token = token

        return await f(*args, **kwargs)

    return wrapper


def require_admin(f: Callable) -> Callable:
    """要求管理员权限的装饰器"""

    @wraps(f)
    @require_auth
    async def wrapper(*args, **kwargs):
        # 检查是否已认证
        if not hasattr(g, "current_user") or g.current_user is None:
            return api_response(False, "需要登录", code=401)

        # 检查用户是否为管理员
        if g.current_user.ROLE != Role.ADMIN.value:
            return api_response(False, "需要管理员权限", code=403)

        return await f(*args, **kwargs)

    return wrapper


async def generate_token(uid: int) -> str:
    """生成认证 token (加密安全)并持久化。"""
    import secrets

    token = secrets.token_hex(32)
    await _store_token(token, uid)
    return token


async def revoke_token(token: str, uid: Optional[int] = None):
    """撤销 token"""
    redis_client = await _get_redis()
    if redis_client:
        try:
            pipe = redis_client.pipeline()
            pipe.delete(_token_key(token))
            if uid is not None:
                pipe.srem(_user_tokens_key(uid), token)
            await pipe.execute()
        except Exception as exc:  # pragma: no cover
            logger.warning("Redis token 撤销失败，回退数据库：%s", exc)
    await AuthTokenOperate.delete_token(token)


async def revoke_user_tokens(uid: int):
    """撤销用户的所有 token"""
    redis_client = await _get_redis()
    if redis_client:
        try:
            tokens = await redis_client.smembers(_user_tokens_key(uid))
            if tokens:
                pipe = redis_client.pipeline()
                for token in tokens:
                    pipe.delete(_token_key(token))
                pipe.delete(_user_tokens_key(uid))
                await pipe.execute()
        except Exception as exc:  # pragma: no cover
            logger.warning("Redis 批量撤销 token 失败，回退数据库：%s", exc)
    await AuthTokenOperate.delete_user_tokens(uid)


# ==================== 登录相关 ====================


@auth_bp.route("/login", methods=["POST"])
async def login():
    """
    用户名密码登录

    Request:
        {
            "username": "myusername",
            "password": "mypassword"
        }

    Response:
        {
            "success": true,
            "data": {
                "token": "xxx",
                "user": { ... }
            }
        }
    """
    data = request.get_json() or {}
    username = data.get("username", "").strip()
    password = data.get("password", "")

    if not username or not password:
        return api_response(False, "缺少用户名或密码", code=400)

    # 输入验证
    if len(username) > 50:
        return api_response(False, "用户名过长", code=400)

    if len(password) > 200:
        return api_response(False, "密码过长", code=400)

    # IP 速率限制检查
    client_ip = get_real_client_ip()
    rate_limit_msg = _check_login_rate_limit(client_ip)
    if rate_limit_msg:
        return api_response(False, rate_limit_msg, code=429)

    # 获取用户
    user = await UserOperate.get_user_by_username(username)
    if not user:
        # 记录登录失败
        _record_login_failure(client_ip)
        await _log_login_attempt(username, False, "用户不存在")
        return api_response(False, "用户名或密码错误", code=401)

    # 验证密码
    if not user.PASSWORD or not verify_password(password, user.PASSWORD):
        _record_login_failure(client_ip)
        await _log_login_attempt(username, False, "密码错误")
        return api_response(False, "用户名或密码错误", code=401)

    # 检查用户状态
    if not user.ACTIVE_STATUS:
        await _log_login_attempt(username, False, "账户已被禁用")
        return api_response(False, "账户已被禁用", code=403)

    # 登录成功，清除该 IP 的失败记录
    _clear_login_failures(client_ip)

    # 更新登录信息
    user.LAST_LOGIN_TIME = timestamp()
    user.LAST_LOGIN_IP = client_ip
    user.LAST_LOGIN_UA = request.headers.get("User-Agent", "unknown")
    await UserOperate.update_user(user)

    # 记录登录成功
    await _log_login_attempt(username, True, "登录成功")

    # 生成 token（快速操作）
    token = await generate_token(user.UID)

    # 快速返回基本用户信息，不阻塞登录
    basic_user_info = {
        "uid": user.UID,
        "username": user.USERNAME,
        "email": user.EMAIL,
        "role": user.ROLE,
        "active": user.ACTIVE_STATUS,
    }

    # 异步后台任务：同步 Emby 状态（不阻塞登录）。
    # 注意：不能 `loop.create_task(...)`，因为生产环境是 WsgiToAsgi，
    # 请求结束后 per-request executor 立即销毁，孤儿任务会触发
    # "CurrentThreadExecutor already quit or is broken"。
    async def sync_background_tasks():
        try:
            await UserService.sync_user_to_emby(user)
        except Exception as e:
            logger.warning(f"后台同步用户状态到 Emby 失败: {e}")

    try:
        from src.core.background import submit_background

        submit_background(sync_background_tasks())
    except Exception as exc:  # pragma: no cover - 投递失败不影响登录
        logger.warning(f"后台任务投递失败: {exc}")

    response, code = api_response(
        True,
        "登录成功",
        {
            "user": basic_user_info,
        },
    )
    _set_auth_cookie(response, token)
    return response, code


@auth_bp.route("/forgot-password/emby", methods=["POST"])
async def forgot_password_by_emby():
    """通过 Emby 用户名和密码验证身份后重置绑定的 Web 登录密码。"""
    client_ip = get_real_client_ip()
    allowed, retry_after = rate_limit_check("forgot_password_emby:ip", client_ip, max_requests=5, window_seconds=600)
    if not allowed:
        return api_response(False, f"请求过于频繁，请在 {retry_after} 秒后重试", code=429)

    data = request.get_json(silent=True) or {}
    emby_username = (data.get("emby_username") or "").strip()
    emby_password = data.get("emby_password") or ""
    if not emby_username or not emby_password:
        return api_response(False, "缺少 Emby 用户名或密码", code=400)
    if len(emby_username) > 100 or len(emby_password) > 200:
        return api_response(False, "输入过长", code=400)

    allowed_user, retry_after_user = rate_limit_check(
        "forgot_password_emby:user",
        emby_username.lower(),
        max_requests=5,
        window_seconds=1800,
    )
    if not allowed_user:
        return api_response(False, f"该账号尝试过于频繁，请在 {retry_after_user} 秒后重试", code=429)

    try:
        from src.services.emby import get_emby_client

        emby_user = await get_emby_client().authenticate_by_name(emby_username, emby_password)
    except Exception as exc:
        logger.warning("Emby 找回密码认证异常: %s", exc)
        return api_response(False, "验证失败", code=401)

    if not emby_user:
        _record_login_failure(client_ip)
        return api_response(False, "Emby 用户名或密码错误", code=401)

    user = await UserOperate.get_user_by_embyid(emby_user.id)
    if not user:
        return api_response(False, "该 Emby 账号未绑定 Web 账号", code=404)
    if not user.ACTIVE_STATUS:
        return api_response(False, "Web 账号已被禁用，请联系管理员", code=403)

    new_password = generate_password(18)
    user.PASSWORD = hash_password(new_password)
    await UserOperate.update_user(user)
    await revoke_user_tokens(user.UID)
    await _log_login_attempt(user.USERNAME, True, "Emby 验证找回 Web 密码")
    _clear_login_failures(client_ip)
    return api_response(
        True,
        "密码已重置，新密码只显示一次，请立即登录后修改",
        {
            "username": user.USERNAME,
            "new_password": new_password,
        },
    )


@auth_bp.route("/login/telegram", methods=["POST"])
async def login_telegram():
    """
    通过 Telegram ID 登录

    Request:
        {
            "telegram_id": 123456789
        }
    """
    if not SecurityConfig.TELEGRAM_DIRECT_LOGIN_ENABLED:
        return api_response(False, "Telegram 直登已禁用，请使用用户名密码登录", code=403)

    # IP 速率限制检查
    client_ip = get_real_client_ip()
    rate_limit_msg = _check_login_rate_limit(client_ip)
    if rate_limit_msg:
        return api_response(False, rate_limit_msg, code=429)

    data = request.get_json() or {}
    telegram_id = data.get("telegram_id")

    if not telegram_id:
        return api_response(False, "缺少 telegram_id", code=400)

    # 类型校验
    if not isinstance(telegram_id, int) or telegram_id <= 0:
        return api_response(False, "telegram_id 格式无效", code=400)

    # 获取用户
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        _record_login_failure(client_ip)
        return api_response(False, "认证失败", code=401)

    # 检查用户状态
    if not user.ACTIVE_STATUS:
        return api_response(False, "账户已被禁用", code=403)

    # 登录成功，清除该 IP 的失败记录
    _clear_login_failures(client_ip)

    # 更新登录信息
    user.LAST_LOGIN_TIME = timestamp()
    user.LAST_LOGIN_IP = client_ip
    user.LAST_LOGIN_UA = request.headers.get("User-Agent", "unknown")
    await UserOperate.update_user(user)

    # 异步后台同步用户状态到 Emby（不阻塞登录）。同上：必须用独立后台 loop。
    from src.services import UserService
    from src.core.background import submit_background

    async def sync_emby_async():
        try:
            await UserService.sync_user_to_emby(user)
        except Exception as e:
            import logging

            logging.getLogger(__name__).warning(f"同步用户状态到 Emby 失败: {e}")

    try:
        submit_background(sync_emby_async())
    except Exception as exc:  # pragma: no cover
        logger.warning(f"后台任务投递失败: {exc}")

    # 生成 token
    token = await generate_token(user.UID)

    # 获取用户信息
    user_info = await UserService.get_user_info(user)

    response, code = api_response(
        True,
        "登录成功",
        {
            "user": user_info,
        },
    )
    _set_auth_cookie(response, token)
    return response, code


@auth_bp.route("/login/apikey", methods=["POST"])
async def login_apikey():
    """
    通过 API Key 登录/验证

    Request:
        {
            "apikey": "key-xxxxx-xxxxx"
        }
    """
    if not SecurityConfig.APIKEY_DIRECT_LOGIN_ENABLED:
        return api_response(False, "API Key 直登已禁用，请使用标准登录流程", code=403)

    # IP 速率限制检查
    client_ip = get_real_client_ip()
    rate_limit_msg = _check_login_rate_limit(client_ip)
    if rate_limit_msg:
        return api_response(False, rate_limit_msg, code=429)

    data = request.get_json() or {}
    apikey = data.get("apikey")

    if not apikey:
        return api_response(False, "缺少 apikey", code=400)

    # 获取用户
    user = await UserOperate.get_user_by_apikey(apikey)
    if not user:
        _record_login_failure(client_ip)
        return api_response(False, "API Key 无效", code=401)

    # 检查用户状态
    if not user.ACTIVE_STATUS:
        return api_response(False, "账户已被禁用", code=403)

    # 登录成功，清除该 IP 的失败记录
    _clear_login_failures(client_ip)

    # 生成 token（API Key 登录也生成 token）
    token = await generate_token(user.UID)

    # 获取用户信息
    from src.services import UserService

    user_info = await UserService.get_user_info(user)

    response, code = api_response(
        True,
        "验证成功",
        {
            "user": user_info,
        },
    )
    _set_auth_cookie(response, token)
    return response, code


# ==================== 登出相关 ====================


@auth_bp.route("/logout", methods=["POST"])
@require_auth
async def logout():
    """登出当前设备"""
    await revoke_token(g.token, getattr(g.current_user, "UID", None))
    response, code = api_response(True, "登出成功")
    _clear_auth_cookie(response)
    return response, code


@auth_bp.route("/logout/all", methods=["POST"])
@require_auth
async def logout_all():
    """登出所有设备"""
    await revoke_user_tokens(g.current_user.UID)
    response, code = api_response(True, "已登出所有设备")
    _clear_auth_cookie(response)
    return response, code


# ==================== 用户信息 ====================


@auth_bp.route("/me", methods=["GET"])
@require_auth
async def get_me():
    """获取当前用户信息"""
    from src.services import UserService

    user_info = await UserService.get_user_info(g.current_user)
    return api_response(True, "获取成功", user_info)


# ==================== Token 刷新 ====================


@auth_bp.route("/refresh", methods=["POST"])
@require_auth
async def refresh_token():
    """刷新 Token"""
    # 撤销旧 token
    await revoke_token(g.token, g.current_user.UID)

    # 生成新 token
    new_token = await generate_token(g.current_user.UID)

    response, code = api_response(True, "刷新成功")
    _set_auth_cookie(response, new_token)
    return response, code


# ==================== API Key 管理 ====================


@auth_bp.route("/apikey", methods=["GET"])
@require_auth
async def get_apikey_status():
    """获取 API Key 状态"""
    key_exists = bool(g.current_user.APIKEY)
    return api_response(
        True,
        "获取成功",
        {
            "enabled": bool(g.current_user.APIKEY_STATUS),
            "has_key": key_exists,
        },
    )


@auth_bp.route("/apikey", methods=["POST"])
@require_auth
async def generate_apikey():
    """生成新 API Key"""
    new_apikey = await UserOperate.reset_apikey(g.current_user)

    # 重新获取用户（更新后的 API Key）
    user = await UserOperate.get_user_by_uid(g.current_user.UID)

    return api_response(
        True,
        "API Key 生成成功",
        {
            "apikey": new_apikey,
            "enabled": True,
        },
    )


@auth_bp.route("/apikey", methods=["DELETE"])
@require_auth
async def disable_apikey():
    """禁用 API Key"""
    await UserOperate.set_apikey_status(g.current_user.UID, False)
    return api_response(True, "API Key 已禁用")


@auth_bp.route("/apikey/enable", methods=["POST"])
@require_auth
async def enable_apikey():
    """启用 API Key（强制旋转新 key）"""
    new_apikey = await UserOperate.reset_apikey(g.current_user)
    return api_response(
        True,
        "API Key 已生成并启用",
        {
            "apikey": new_apikey,
            "enabled": True,
        },
    )


@auth_bp.route("/apikey/permissions", methods=["GET"])
@require_auth
async def get_apikey_permissions():
    """获取 API Key 的权限列表"""
    import json
    from src.api.v1.apikey import ALL_PERMISSIONS, _get_user_permissions

    return api_response(
        True,
        "获取成功",
        {
            "permissions": _get_user_permissions(g.current_user),
            "all_permissions": ALL_PERMISSIONS,
        },
    )


@auth_bp.route("/apikey/permissions", methods=["PUT"])
@require_auth
async def update_apikey_permissions():
    """更新 API Key 的权限列表"""
    import json
    from src.api.v1.apikey import ALL_PERMISSIONS

    data = request.get_json() or {}
    permissions = data.get("permissions")

    if permissions is None:
        return api_response(False, "缺少 permissions 参数", code=400)

    if not isinstance(permissions, list):
        return api_response(False, "permissions 必须是数组", code=400)

    invalid = [p for p in permissions if p not in ALL_PERMISSIONS]
    if invalid:
        return api_response(False, f"无效的权限: {', '.join(invalid)}", code=400)

    user = g.current_user
    user.APIKEY_PERMISSIONS = json.dumps(permissions)
    await UserOperate.update_user(user)

    return api_response(
        True,
        "权限已更新",
        {
            "permissions": permissions,
        },
    )


# ==================== 辅助函数 ====================


async def _log_login_attempt(username: str, success: bool, reason: str = ""):
    """记录登录尝试"""
    try:
        # 获取用户 UID
        user = await UserOperate.get_user_by_username(username)
        if not user:
            return  # 用户不存在，不记录日志

        log = LoginLogModel(
            UID=user.UID,
            EMBY_USER_ID=user.EMBYID or "",
            IP_ADDRESS=get_real_client_ip(),
            DEVICE_NAME=request.headers.get("User-Agent", "unknown")[:200],  # 限制长度
            LOGIN_TIME=timestamp(),
            IS_BLOCKED=not success,  # 登录失败时标记为被拦截
        )
        await LoginLogOperate.add_log(log)
    except Exception as e:
        # 记录失败不影响登录流程
        import logging

        logging.getLogger(__name__).warning(f"记录登录日志失败: {e}")


__all__ = [
    "auth_bp",
    "require_auth",
    "require_admin",
    "api_response",
    "generate_token",
    "revoke_token",
    "revoke_user_tokens",
]
