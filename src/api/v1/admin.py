"""
管理员 API

提供管理员专用的操作接口
"""

import logging
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, require_admin, api_response
from src.config import RegisterConfig
from src.db.user import UserOperate, UserModel, Role
from src.db.regcode import RegCodeOperate
from src.core.utils import parse_bool
from src.services import UserService, EmbyService
from src.services.emby import get_emby_client, EmbyError, EmbyConnectionError

logger = logging.getLogger(__name__)
admin_bp = Blueprint("admin", __name__, url_prefix="/admin")


# ==================== 用户管理 ====================


@admin_bp.route("/users", methods=["GET"])
@require_auth
@require_admin
async def list_users():
    """
    获取用户列表

    Query:
        page: int - 页码（从1开始，默认1）
        per_page: int - 每页数量（默认20，最大100）
        role: int - 按角色筛选 (0=管理员, 1=普通用户, 2=白名单)
        active: bool - 按状态筛选 (true=仅启用 / false=仅禁用，省略=不过滤)
        search: str - 搜索 UID / 用户名 / Telegram ID
        sort: str - 排序字段+方向，形如 ``uid_desc`` / ``username_asc`` /
                    ``register_time_desc`` / ``expired_at_asc`` / ``role_asc``
                    / ``active_desc`` / ``last_login_time_desc``
    """
    page = max(1, request.args.get("page", 1, type=int))
    per_page = min(max(1, request.args.get("per_page", 20, type=int)), 100)
    role = request.args.get("role", type=int)
    active = request.args.get("active")
    emby = (request.args.get("emby") or "").strip().lower()
    search = request.args.get("search", "").strip()
    sort_by = (request.args.get("sort") or "").strip() or None

    # 显式三态：true=只看启用，false=只看禁用，省略=全部
    active_status: bool | None = None
    if active is not None:
        if active.lower() == "true":
            active_status = True
        elif active.lower() == "false":
            active_status = False

    # Emby 绑定筛选：bound=只看已绑定，unbound=只看未绑定，省略/其它=全部
    has_emby: bool | None = None
    if emby == "bound":
        has_emby = True
    elif emby == "unbound":
        has_emby = False

    offset = (page - 1) * per_page

    users, total = await UserOperate.get_all_users(
        offset=offset,
        limit=per_page,
        role=role,
        active_status=active_status,
        include_inactive=True,  # 让 active_status 完全主导筛选
        search=search or None,
        sort_by=sort_by,
        has_emby=has_emby,
    )

    # Telegram username 优先取 user.OTHER 里缓存的（用户每次 /start /bind 会刷新）。
    # 缓存里没有，且 Bot 在线时，best-effort 拉一次 get_chat 并写回缓存，
    # 但对单次列表请求最多只拉前 `_MAX_LIVE_TG_FETCH` 个用户，避免 Bot API 限流。
    _MAX_LIVE_TG_FETCH = 10
    live_fetch_used = 0

    from src.services.telegram_runtime import run_bot_operation

    user_list = []
    for user in users:
        telegram_username = UserService.get_cached_telegram_username(user)

        # 没缓存 + Bot 在线 + 本次请求还有 live fetch 配额 → 试一次
        if telegram_username is None and user.TELEGRAM_ID and live_fetch_used < _MAX_LIVE_TG_FETCH:
            try:

                async def _resolve_chat(bot):
                    return await bot.get_chat(user.TELEGRAM_ID)

                tg_user = await run_bot_operation(_resolve_chat, timeout=8)
                resolved = tg_user.username or None
                live_fetch_used += 1
                if resolved:
                    telegram_username = resolved
                    # 写回缓存；下次列表请求直接读 DB
                    await UserService.cache_telegram_username(user, resolved)
            except Exception:
                live_fetch_used += 1  # 失败也算一次，免得一直死磕同一个

        # EMBYID 非空即视为已绑定；历史数据可能残留 PENDING_EMBY=True，不能因此把到期显示成未绑定。
        emby_bound = bool(user.EMBYID)
        emby_disabled_by_expiry = UserService.is_emby_access_expired(user)
        bgm_token_set = bool((user.BGM_TOKEN or "").strip())
        user_list.append(
            {
                "uid": user.UID,
                "telegram_id": user.TELEGRAM_ID,
                "telegram_username": telegram_username,
                "username": user.USERNAME,
                "email": user.EMAIL,
                "role": user.ROLE,
                "role_name": Role(user.ROLE).name if user.ROLE in [r.value for r in Role] else "UNKNOWN",
                "active": user.ACTIVE_STATUS,
                "emby_id": user.EMBYID,
                "emby_bound": emby_bound,
                "pending_emby": bool(getattr(user, "PENDING_EMBY", False)) and not emby_bound,
                "pending_emby_days": getattr(user, "PENDING_EMBY_DAYS", None) if not emby_bound else None,
                "expired_at": user.EXPIRED_AT if emby_bound else None,
                "emby_disabled_by_expiry": emby_disabled_by_expiry,
                "register_time": user.REGISTER_TIME,
                "created_at": user.CREATE_AT or user.REGISTER_TIME,
                "last_login_time": user.LAST_LOGIN_TIME,
                "bgm_mode": bool(user.BGM_MODE),
                "bgm_token_set": bgm_token_set,
                "bgm_sync_ready": bool(user.BGM_MODE and bgm_token_set),
                "library_self_service": bool(getattr(user, "LIBRARY_SELF_SERVICE", False)),
            }
        )

    return api_response(
        True,
        f"共 {len(user_list)} 个用户",
        {
            "users": user_list,
            "total": total,
            "page": page,
            "per_page": per_page,
            "pages": (total + per_page - 1) // per_page,
        },
    )


@admin_bp.route("/me/update", methods=["PUT"])
@require_auth
@require_admin
async def update_my_info():
    """
    管理员更新自己的信息

    Body:
        暂无可更新字段
    """
    return api_response(False, "没有可更新的字段", code=400)


@admin_bp.route("/users/<int:uid>", methods=["GET"])
@require_auth
@require_admin
async def get_user(uid: int):
    """获取用户详情"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    user_info = await UserService.get_user_info(user)
    status = await EmbyService.get_user_status(user)

    user_info["emby_status"] = {
        "is_synced": status.is_synced,
        "is_active": status.is_active,
        "active_sessions": status.active_sessions,
        "message": status.message,
    }

    return api_response(True, "获取成功", user_info)


@admin_bp.route("/users/<int:uid>/registration-queue/clear", methods=["POST"])
@require_auth
@require_admin
async def clear_user_registration_queue(uid: int):
    """清理指定用户残留的注册/卡码队列状态。"""
    from src.core.utils import rate_limit_check
    from src.services import EmbyRegisterQueueService, RegcodeUseQueueService

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    allowed, retry_after = rate_limit_check(
        "admin_clear_user_registration_queue",
        str(g.current_user.UID),
        max_requests=30,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"操作过于频繁，请 {retry_after} 秒后再试", code=429)

    data = request.get_json(silent=True) or {}
    # 默认只清理尚未开始处理的队列项；processing 可能已在外部创建账号，不能静默摘除。
    queued_only = parse_bool(data.get("queued_only"), default=True)

    emby_result = await EmbyRegisterQueueService.clear_for_uid(uid, queued_only=queued_only)
    regcode_result = await RegcodeUseQueueService.clear_for_uid(uid, queued_only=queued_only)
    logger.warning(
        "管理员 %s 清理用户注册队列 uid=%s username=%s emby=%s regcode=%s",
        getattr(g.current_user, "USERNAME", None),
        uid,
        user.USERNAME,
        emby_result,
        regcode_result,
    )
    cleared = bool(emby_result.get("cleared") or regcode_result.get("cleared"))
    return api_response(
        True,
        "已清理该用户的注册队列状态" if cleared else "该用户没有残留的注册队列状态",
        {
            "uid": uid,
            "username": user.USERNAME,
            "cleared": cleared,
            "queued_only": queued_only,
            "emby_register_queue": emby_result,
            "regcode_use_queue": regcode_result,
        },
    )


async def _registration_queue_uids() -> set[int]:
    from src.services import EmbyRegisterQueueService, RegcodeUseQueueService

    return set(EmbyRegisterQueueService.pending_uids()) | set(RegcodeUseQueueService.pending_uids())


def _parse_registration_entitlement_days(raw_days) -> tuple[bool, int, str]:
    try:
        days = int(raw_days if raw_days is not None else RegisterConfig.EMBY_DIRECT_REGISTER_DAYS)
    except (TypeError, ValueError):
        return False, 0, "days 必须是整数"
    if days == 0:
        days = -1
    if days < -1:
        return False, 0, "days 不能小于 -1"
    if days > 3650:
        return False, 0, "days 不能超过 3650"
    return True, days, ""


@admin_bp.route("/users/registration-queue/clear", methods=["POST"])
@require_auth
@require_admin
async def clear_registration_queue_users():
    """批量清理当前注册队列中尚未开始处理的用户。"""
    from src.core.utils import rate_limit_check
    from src.services import EmbyRegisterQueueService, RegcodeUseQueueService

    allowed, retry_after = rate_limit_check(
        "admin_clear_registration_queue_users",
        str(g.current_user.UID),
        max_requests=10,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"操作过于频繁，请 {retry_after} 秒后再试", code=429)

    data = request.get_json(silent=True) or {}
    dry_run = parse_bool(data.get("dry_run"), default=True)
    confirm = (data.get("confirm") or "").strip()
    uids = sorted(await _registration_queue_uids())

    if dry_run:
        return api_response(True, "获取成功", {"dry_run": True, "count": len(uids), "uids": uids[:500]})
    if confirm != "CLEAR_REGISTRATION_QUEUE":
        return api_response(False, "缺少确认标记", code=400)

    results = []
    cleared = 0
    blocked = 0
    for uid in uids[:1000]:
        emby_result = await EmbyRegisterQueueService.clear_for_uid(uid, queued_only=True)
        regcode_result = await RegcodeUseQueueService.clear_for_uid(uid, queued_only=True)
        item_cleared = bool(emby_result.get("cleared") or regcode_result.get("cleared"))
        item_blocked = bool(emby_result.get("reason") or regcode_result.get("reason"))
        cleared += 1 if item_cleared else 0
        blocked += 1 if item_blocked else 0
        results.append({"uid": uid, "cleared": item_cleared, "blocked": item_blocked})

    logger.warning(
        "管理员 %s 批量清理注册队列: matched=%d cleared=%d blocked=%d",
        getattr(g.current_user, "USERNAME", None),
        len(uids),
        cleared,
        blocked,
    )
    return api_response(
        True,
        f"已清理 {cleared} 个未处理队列项" + (f"；{blocked} 个已开始处理，未移出" if blocked else ""),
        {"dry_run": False, "matched": len(uids), "cleared": cleared, "blocked": blocked, "results": results},
    )


@admin_bp.route("/users/registration-queue/grant-entitlement-and-clear", methods=["POST"])
@require_auth
@require_admin
async def grant_registration_queue_users_entitlement_and_clear():
    """给当前注册队列用户授予 Emby 补建资格，并移出未处理队列。"""
    from src.core.utils import rate_limit_check
    from src.services import EmbyRegisterQueueService, RegcodeUseQueueService

    allowed, retry_after = rate_limit_check(
        "admin_grant_registration_queue_users_entitlement",
        str(g.current_user.UID),
        max_requests=10,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"操作过于频繁，请 {retry_after} 秒后再试", code=429)

    data = request.get_json(silent=True) or {}
    dry_run = parse_bool(data.get("dry_run"), default=True)
    confirm = (data.get("confirm") or "").strip()
    ok, days, msg = _parse_registration_entitlement_days(data.get("days", RegisterConfig.EMBY_DIRECT_REGISTER_DAYS))
    if not ok:
        return api_response(False, msg, code=400)

    queue_uids = sorted(await _registration_queue_uids())
    users = []
    skipped = []
    for uid in queue_uids[:1000]:
        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            skipped.append({"uid": uid, "reason": "用户不存在"})
            continue
        if user.EMBYID:
            skipped.append({"uid": uid, "username": user.USERNAME, "reason": "已绑定 Emby"})
            continue
        if not user.ACTIVE_STATUS:
            skipped.append({"uid": uid, "username": user.USERNAME, "reason": "账号已禁用"})
            continue
        users.append(user)

    if dry_run:
        return api_response(
            True,
            "获取成功",
            {
                "dry_run": True,
                "days": days,
                "matched": len(queue_uids),
                "eligible": len(users),
                "skipped": skipped[:200],
                "users": [{"uid": u.UID, "username": u.USERNAME} for u in users[:200]],
            },
        )
    if confirm != "GRANT_AND_CLEAR_REGISTRATION_QUEUE":
        return api_response(False, "缺少确认标记", code=400)

    capacity_lock = await UserService.acquire_emby_capacity_lock()
    if capacity_lock is None:
        return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)

    granted = 0
    dequeued = 0
    blocked = 0
    failed = []
    try:
        for user in users:
            try:
                if not getattr(user, "PENDING_EMBY", False):
                    cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
                    if not cap_ok:
                        failed.append({"uid": user.UID, "username": user.USERNAME, "reason": cap_msg})
                        continue
                user_limit_ok, user_limit_msg = await UserService.check_normal_user_capacity_for_grant(user)
                if not user_limit_ok:
                    failed.append({"uid": user.UID, "username": user.USERNAME, "reason": user_limit_msg})
                    continue

                user.PENDING_EMBY = True
                user.PENDING_EMBY_DAYS = days
                if user.ROLE == Role.UNRECOGNIZED.value:
                    user.ROLE = Role.NORMAL.value
                if not user.CREATE_AT:
                    user.CREATE_AT = user.REGISTER_TIME
                await UserOperate.update_user(user)
                granted += 1

                emby_result = await EmbyRegisterQueueService.clear_for_uid(user.UID, queued_only=True)
                regcode_result = await RegcodeUseQueueService.clear_for_uid(user.UID, queued_only=True)
                if emby_result.get("cleared") or regcode_result.get("cleared"):
                    dequeued += 1
                if emby_result.get("reason") or regcode_result.get("reason"):
                    blocked += 1
            except Exception as exc:  # pragma: no cover
                failed.append({"uid": user.UID, "username": user.USERNAME, "reason": str(exc)})
    finally:
        await UserService.release_emby_capacity_lock(capacity_lock)

    logger.warning(
        "管理员 %s 批量授权注册队列用户: matched=%d eligible=%d granted=%d dequeued=%d blocked=%d failed=%d days=%s",
        getattr(g.current_user, "USERNAME", None),
        len(queue_uids),
        len(users),
        granted,
        dequeued,
        blocked,
        len(failed),
        days,
    )
    return api_response(
        True,
        f"已授予 {granted} 个用户 Emby 注册资格，并移出 {dequeued} 个未处理队列项",
        {
            "dry_run": False,
            "days": days,
            "matched": len(queue_uids),
            "eligible": len(users),
            "granted": granted,
            "dequeued": dequeued,
            "blocked": blocked,
            "skipped": skipped[:200],
            "failed": failed,
        },
    )


@admin_bp.route("/users/<int:uid>/registration-entitlement", methods=["POST"])
@require_auth
@require_admin
async def grant_user_registration_entitlement(uid: int):
    """授予未绑定 Emby 的系统账号一次补建 Emby 的资格。"""
    from src.core.utils import rate_limit_check
    from src.services import EmbyRegisterQueueService, RegcodeUseQueueService

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.EMBYID:
        return api_response(False, "该用户已绑定 Emby 账号，无需授予注册资格", code=400)
    if not user.ACTIVE_STATUS:
        return api_response(False, "该用户已被禁用，请先启用账号再授予注册资格", code=400)

    allowed, retry_after = rate_limit_check(
        "admin_grant_user_registration_entitlement",
        str(g.current_user.UID),
        max_requests=30,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"操作过于频繁，请 {retry_after} 秒后再试", code=429)

    data = request.get_json(silent=True) or {}
    raw_days = data.get("days", RegisterConfig.EMBY_DIRECT_REGISTER_DAYS)
    try:
        days = int(raw_days if raw_days is not None else 30)
    except (TypeError, ValueError):
        return api_response(False, "days 必须是整数", code=400)
    if days == 0:
        days = -1
    if days < -1:
        return api_response(False, "days 不能小于 -1", code=400)
    if days > 3650:
        return api_response(False, "days 不能超过 3650", code=400)

    capacity_lock = await UserService.acquire_emby_capacity_lock()
    if capacity_lock is None:
        return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)
    try:
        if not getattr(user, "PENDING_EMBY", False):
            cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
            if not cap_ok:
                return api_response(False, cap_msg, code=409)
        user_limit_ok, user_limit_msg = await UserService.check_normal_user_capacity_for_grant(user)
        if not user_limit_ok:
            return api_response(False, user_limit_msg, code=409)

        emby_queue = await EmbyRegisterQueueService.clear_for_uid(uid)
        regcode_queue = await RegcodeUseQueueService.clear_for_uid(uid)

        user.PENDING_EMBY = True
        user.PENDING_EMBY_DAYS = days
        if user.ROLE == Role.UNRECOGNIZED.value:
            user.ROLE = Role.NORMAL.value
        if not user.CREATE_AT:
            user.CREATE_AT = user.REGISTER_TIME
        await UserOperate.update_user(user)
    finally:
        await UserService.release_emby_capacity_lock(capacity_lock)

    logger.warning(
        "管理员 %s 授予用户 Emby 补建资格 uid=%s username=%s days=%s",
        getattr(g.current_user, "USERNAME", None),
        uid,
        user.USERNAME,
        days,
    )
    return api_response(
        True,
        "已授予该用户 Emby 注册资格",
        {
            "uid": uid,
            "username": user.USERNAME,
            "pending_emby": True,
            "pending_emby_days": days,
            "queue_cleared": bool(emby_queue.get("cleared") or regcode_queue.get("cleared")),
            "emby_register_queue": emby_queue,
            "regcode_use_queue": regcode_queue,
        },
    )


@admin_bp.route("/users/<int:uid>/registration-entitlement/dequeue", methods=["POST"])
@require_auth
@require_admin
async def grant_user_registration_entitlement_and_dequeue(uid: int):
    """授予 Emby 补建资格，并仅把该用户从未处理注册队列中移出。"""
    from src.core.utils import rate_limit_check
    from src.services import EmbyRegisterQueueService, RegcodeUseQueueService

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.EMBYID:
        return api_response(False, "该用户已绑定 Emby 账号，无需授予注册资格", code=400)
    if not user.ACTIVE_STATUS:
        return api_response(False, "该用户已被禁用，请先启用账号再授予注册资格", code=400)

    allowed, retry_after = rate_limit_check(
        "admin_grant_user_registration_entitlement_dequeue",
        str(g.current_user.UID),
        max_requests=30,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"操作过于频繁，请 {retry_after} 秒后再试", code=429)

    data = request.get_json(silent=True) or {}
    raw_days = data.get("days", RegisterConfig.EMBY_DIRECT_REGISTER_DAYS)
    try:
        days = int(raw_days if raw_days is not None else 30)
    except (TypeError, ValueError):
        return api_response(False, "days 必须是整数", code=400)
    if days == 0:
        days = -1
    if days < -1:
        return api_response(False, "days 不能小于 -1", code=400)
    if days > 3650:
        return api_response(False, "days 不能超过 3650", code=400)

    capacity_lock = await UserService.acquire_emby_capacity_lock()
    if capacity_lock is None:
        return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)
    try:
        if not getattr(user, "PENDING_EMBY", False):
            cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
            if not cap_ok:
                return api_response(False, cap_msg, code=409)
        user_limit_ok, user_limit_msg = await UserService.check_normal_user_capacity_for_grant(user)
        if not user_limit_ok:
            return api_response(False, user_limit_msg, code=409)

        user.PENDING_EMBY = True
        user.PENDING_EMBY_DAYS = days
        if user.ROLE == Role.UNRECOGNIZED.value:
            user.ROLE = Role.NORMAL.value
        if not user.CREATE_AT:
            user.CREATE_AT = user.REGISTER_TIME
        await UserOperate.update_user(user)

        emby_queue = await EmbyRegisterQueueService.clear_for_uid(uid, queued_only=True)
        regcode_queue = await RegcodeUseQueueService.clear_for_uid(uid, queued_only=True)
    finally:
        await UserService.release_emby_capacity_lock(capacity_lock)
    processing_blocked = [
        item.get("reason")
        for item in (emby_queue, regcode_queue)
        if item.get("reason")
    ]

    logger.warning(
        "管理员 %s 授权并移出未处理队列 uid=%s username=%s days=%s emby=%s regcode=%s",
        getattr(g.current_user, "USERNAME", None),
        uid,
        user.USERNAME,
        days,
        emby_queue,
        regcode_queue,
    )
    return api_response(
        True,
        "已授予 Emby 注册资格；已移出未处理队列" if not processing_blocked else "已授予 Emby 注册资格；部分请求已开始处理，未强制移出",
        {
            "uid": uid,
            "username": user.USERNAME,
            "pending_emby": True,
            "pending_emby_days": days,
            "dequeued": bool(emby_queue.get("cleared") or regcode_queue.get("cleared")),
            "processing_blocked": processing_blocked,
            "emby_register_queue": emby_queue,
            "regcode_use_queue": regcode_queue,
        },
    )


async def _cascade_toggle_active(
    *,
    root_uid: int,
    enable: bool,
    cascade_depth_raw: int,
    reason: str = "",
) -> tuple[bool, str, dict]:
    """通用启停级联实现。

    - ``cascade_depth_raw <= 0 or >= 999``：整棵子树（不限层级）。
    - ``cascade_depth_raw == 1``：仅本人，等同旧 ``disable_user/enable_user``。
    - 否则：本人 + 向下 N-1 层。
    - 不会影响其它管理员账号（除非当前管理员就是被操作者本人）。
    - 邀请树结构完全不变，只翻转 ``ACTIVE_STATUS`` 并同步 Emby。
    """
    from src.services import InviteService

    root_user = await UserOperate.get_user_by_uid(root_uid)
    if not root_user:
        return False, "用户不存在", {"code": 404}
    if root_user.ROLE == Role.ADMIN.value and root_user.UID != g.current_user.UID:
        return False, "不允许启停其他管理员账号", {"code": 403}

    if cascade_depth_raw <= 0 or cascade_depth_raw >= 999:
        cascade_depth: int | None = None
    else:
        cascade_depth = max(1, cascade_depth_raw)

    if cascade_depth == 1:
        target_uids = [root_uid]
    else:
        target_uids = await InviteService.collect_uids_to_delete(
            root_uid,
            cascade_depth if cascade_depth is not None else 9999,
        )

    # 保护：除非当前管理员主动操作自己，否则不要把自己卷进去
    safe_targets: list[int] = []
    for tid in target_uids:
        if tid == g.current_user.UID and root_uid != g.current_user.UID:
            continue
        safe_targets.append(tid)

    affected: list[int] = []
    skipped: list[dict] = []
    failed: list[dict] = []

    for tid in safe_targets:
        target = await UserOperate.get_user_by_uid(tid)
        if not target:
            continue
        # 不要级联翻动其他管理员
        if target.ROLE == Role.ADMIN.value and target.UID != g.current_user.UID and tid != root_uid:
            skipped.append({"uid": tid, "reason": "管理员账户不可被级联启停"})
            continue
        # 已是目标状态：跳过但不算失败
        if bool(target.ACTIVE_STATUS) == enable:
            skipped.append({"uid": tid, "reason": f"已经处于{'启用' if enable else '禁用'}状态"})
            continue

        if enable:
            ok, msg = await UserService.enable_user(target)
        else:
            ok, msg = await UserService.disable_user(target, reason)

        if ok:
            affected.append(tid)
        else:
            failed.append({"uid": tid, "reason": msg})

    action = "启用" if enable else "禁用"
    if len(safe_targets) <= 1:
        prefix = f"{action}完成"
    else:
        prefix = f"{action}级联完成"
    cascade_display = "整棵子树" if cascade_depth is None else cascade_depth
    return (
        True,
        (f"{prefix}：成功 {len(affected)}，跳过 {len(skipped)}，失败 {len(failed)}"),
        {
            "affected": affected,
            "skipped": skipped,
            "failed": failed,
            "cascade_depth": cascade_display,
            "enable": enable,
        },
    )


@admin_bp.route("/users/<int:uid>/disable", methods=["POST"])
@require_auth
@require_admin
async def disable_user(uid: int):
    """禁用用户（支持邀请树级联）。

    Request body:
        reason: str - 可选，禁用原因
        cascade_depth: int - 邀请树级联层级，默认 1（仅本人）。
            1 = 仅本人；2 = 本人 + 直接下级；N = 本人 + 下 N-1 层；
            0 或 >= 999 = 整棵子树。
    """
    data = request.get_json(silent=True) or {}
    reason = data.get("reason", "")
    try:
        cascade_depth_raw = int(data.get("cascade_depth", request.args.get("cascade_depth", 1)))
    except (TypeError, ValueError):
        cascade_depth_raw = 1

    ok, message, payload = await _cascade_toggle_active(
        root_uid=uid,
        enable=False,
        cascade_depth_raw=cascade_depth_raw,
        reason=reason,
    )
    if not ok:
        return api_response(False, message, code=int(payload.get("code", 400)))
    return api_response(True, message, payload)


@admin_bp.route("/users/<int:uid>/enable", methods=["POST"])
@require_auth
@require_admin
async def enable_user(uid: int):
    """启用用户（支持邀请树级联）。

    Request body:
        cascade_depth: int - 同 `/disable`。
    """
    data = request.get_json(silent=True) or {}
    try:
        cascade_depth_raw = int(data.get("cascade_depth", request.args.get("cascade_depth", 1)))
    except (TypeError, ValueError):
        cascade_depth_raw = 1

    ok, message, payload = await _cascade_toggle_active(
        root_uid=uid,
        enable=True,
        cascade_depth_raw=cascade_depth_raw,
    )
    if not ok:
        return api_response(False, message, code=int(payload.get("code", 400)))
    return api_response(True, message, payload)


@admin_bp.route("/users/<int:uid>", methods=["PUT"])
@require_auth
@require_admin
async def update_user(uid: int):
    """
    更新用户信息

    Body:
        role: int - 角色 (0=管理员, 1=普通用户, 2=白名单)
        emby_id: str - Emby ID
        active: bool - 启用状态
    """
    data = request.get_json() or {}

    # 获取目标用户
    target_user = await UserOperate.get_user_by_uid(uid)
    if not target_user:
        return api_response(False, "用户不存在", code=404)

    # 权限检查：不允许修改其他管理员
    if target_user.ROLE == Role.ADMIN.value and target_user.UID != g.current_user.UID:
        return api_response(False, "不允许修改其他管理员的信息", code=403)

    # 权限检查：不允许将其他用户设置为管理员
    if "role" in data and data["role"] == Role.ADMIN.value and uid != g.current_user.UID:
        return api_response(False, "不允许将其他用户设置为管理员", code=403)

    capacity_lock = None
    try:
        # 更新角色
        if "role" in data:
            role = data["role"]
            if role not in [r.value for r in Role]:
                return api_response(False, "无效的角色值", code=400)
            if role == Role.NORMAL.value and target_user.ROLE == Role.UNRECOGNIZED.value:
                user_limit_ok, user_limit_msg = await UserService.check_normal_user_capacity_for_grant(target_user)
                if not user_limit_ok:
                    return api_response(False, user_limit_msg, code=409)
            target_user.ROLE = role

        emby_changed = False
        old_emby_id = (target_user.EMBYID or "").strip()
        # 更新 Emby ID
        if "emby_id" in data:
            new_emby_id = (data.get("emby_id") or "").strip()
            if new_emby_id != old_emby_id:
                emby_changed = True
                if new_emby_id:
                    emby = get_emby_client()
                    try:
                        remote_user = await emby.get_user(new_emby_id)
                    except EmbyError as exc:
                        return api_response(False, f"无法校验 Emby 用户: {exc}", code=502)
                    if not remote_user:
                        return api_response(False, "目标 Emby ID 不存在", code=404)
                    occupant = await UserOperate.get_user_by_embyid(new_emby_id)
                    if occupant and occupant.UID != target_user.UID:
                        return api_response(False, f"该 Emby ID 已绑定 UID {occupant.UID}", code=409)
                    if not old_emby_id and not getattr(target_user, "PENDING_EMBY", False):
                        capacity_lock = await UserService.acquire_emby_capacity_lock()
                        if capacity_lock is None:
                            return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)
                        cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=target_user.UID)
                        if not cap_ok:
                            return api_response(False, cap_msg, code=409)
                    target_user.PENDING_EMBY = False
                    target_user.PENDING_EMBY_DAYS = None
                    target_user.EMBYID = new_emby_id
                else:
                    target_user.EMBYID = None
                    target_user.PENDING_EMBY = False
                    target_user.PENDING_EMBY_DAYS = None

        # 更新启用状态
        active_changed = False
        new_active: bool = target_user.ACTIVE_STATUS
        if "active" in data:
            new_active = bool(data["active"])
            active_changed = new_active != target_user.ACTIVE_STATUS
            target_user.ACTIVE_STATUS = new_active

        # 保存到数据库
        await UserOperate.update_user(target_user)

        # 启用/禁用变更时同步 Emby 账户
        emby_sync_msg = ""
        if (active_changed or emby_changed) and target_user.EMBYID:
            try:
                ok, sync_msg = await UserService.sync_user_to_emby(target_user)
                if not ok:
                    emby_sync_msg = f"，但同步 Emby 账户状态失败：{sync_msg}"
            except Exception as emby_err:
                logger.error(
                    f"同步 Emby 启用状态失败 (uid={target_user.UID}): {emby_err}",
                    exc_info=True,
                )
                emby_sync_msg = "，但同步 Emby 账户状态失败"

        return api_response(True, "更新成功" + emby_sync_msg)
    except Exception as e:
        logger.error(f"更新用户信息失败: {e}", exc_info=True)
        return api_response(False, f"更新失败: {e}", code=500)
    finally:
        await UserService.release_emby_capacity_lock(capacity_lock)


@admin_bp.route("/users/<int:uid>", methods=["DELETE"])
@require_auth
@require_admin
async def delete_user(uid: int):
    """删除用户（支持邀请树级联）。

    Body:
        mode: str - 'with_emby'（默认）/ 'local_only' / 'emby_only'
              - with_emby：同时删除本地账户与 Emby 账户，清理邀请关系
              - local_only：仅删除本地账户与邀请关系，保留 Emby 账户
              - emby_only：仅删除 Emby 账户，本地账户与邀请关系完全保留
        cascade_depth: int - 邀请树级联层级，默认 1（仅本人）。
              1 = 仅本人；2 = 本人 + 直接邀请下级；以此类推。
              传 0 / 负数或 >= 999 视为「整棵子树」。
        delete_emby: bool - 兼容旧字段；未传 mode 时使用，
              true → with_emby，false → local_only。
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许删除其他管理员账号", code=403)

    body = request.get_json(silent=True) or {}

    # 优先 mode；老调用方仍可用 delete_emby
    mode = (body.get("mode") or request.args.get("mode") or "").strip().lower()
    if mode not in ("with_emby", "local_only", "emby_only"):
        raw_delete = body.get("delete_emby", request.args.get("delete_emby", "true"))
        mode = "with_emby" if str(raw_delete).lower() not in ("false", "0", "no") else "local_only"

    try:
        cascade_depth_raw = int(body.get("cascade_depth", request.args.get("cascade_depth", 1)))
    except (TypeError, ValueError):
        cascade_depth_raw = 1
    # 0 / 负数 / 极大值 视为「整棵子树」
    if cascade_depth_raw <= 0 or cascade_depth_raw >= 999:
        cascade_depth = None  # None = 不限层级
    else:
        cascade_depth = max(1, cascade_depth_raw)

    from src.services import InviteService

    # ---------- 收集目标 UID 列表 ----------
    if cascade_depth == 1:
        target_uids = [uid]
    else:
        # cascade_depth=None → 整棵子树；其它走 BFS 收集到指定层
        target_uids = await InviteService.collect_uids_to_delete(
            uid,
            cascade_depth if cascade_depth is not None else 9999,
        )

    # 安全保护：不允许把"当前操作管理员自己"被动卷进级联里
    safe_targets: list[int] = []
    for tid in target_uids:
        if tid == g.current_user.UID and uid != g.current_user.UID:
            continue
        safe_targets.append(tid)

    # ---------- 执行 ----------
    deleted: list[int] = []
    skipped: list[dict] = []
    failed: list[dict] = []

    # 叶子优先，避免外键/关系悬空
    for tid in reversed(safe_targets):
        target = await UserOperate.get_user_by_uid(tid)
        if not target:
            continue
        # 非主动操作的下级里若混入其他管理员账号，跳过保护
        if target.ROLE == Role.ADMIN.value and target.UID != g.current_user.UID and tid != uid:
            skipped.append({"uid": tid, "reason": "管理员账户不可被级联删除"})
            continue

        if mode == "emby_only":
            # 仅删 Emby 账号：本地账户与邀请关系完全保留
            if not target.EMBYID:
                skipped.append({"uid": tid, "reason": "未绑定 Emby 账户"})
                continue
            ok, msg = await UserService.delete_emby_only(target)
        else:
            # local_only / with_emby：都删本地账户 + 清理邀请关系
            try:
                await InviteService.delete_user_keep_subtree(tid)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"清理邀请关系失败 uid={tid}: {exc}")
            ok, msg = await UserService.delete_user(target, delete_emby=(mode == "with_emby"))

        if ok:
            deleted.append(tid)
        else:
            failed.append({"uid": tid, "reason": msg})

    cascade_display = "整棵子树" if cascade_depth is None else cascade_depth

    if mode == "emby_only":
        message_prefix = "Emby 级联删除完成" if len(safe_targets) > 1 else "Emby 删除完成"
    elif mode == "local_only":
        message_prefix = "本地账户级联删除完成" if len(safe_targets) > 1 else "本地账户删除完成"
    else:
        message_prefix = "级联删除完成" if len(safe_targets) > 1 else "删除完成"

    return api_response(
        len(failed) == 0,
        f"{message_prefix}：成功 {len(deleted)}，跳过 {len(skipped)}，失败 {len(failed)}",
        {
            "deleted": deleted,
            "skipped": skipped,
            "failed": failed,
            "mode": mode,
            "cascade_depth": cascade_display,
        },
    )


@admin_bp.route("/users/<int:uid>/emby", methods=["DELETE"])
@require_auth
@require_admin
async def delete_user_emby(uid: int):
    """仅删除该用户绑定的 Emby 账户，本地账户保留。"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许操作其他管理员的 Emby 账户", code=403)

    success, message = await UserService.delete_emby_only(user)
    return api_response(success, message, code=200 if success else 400)


@admin_bp.route("/users/<int:uid>/force-unbind", methods=["POST"])
@require_auth
@require_admin
async def force_unbind_user(uid: int):
    """仅解除本地绑定关系，不删除 Telegram/Emby 外部账号。

    Body:
        scope: "telegram" | "emby" | "both"，默认 both
    """
    import json

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许操作其他管理员账号", code=403)

    data = request.get_json(silent=True) or {}
    scope = (data.get("scope") or "both").strip().lower()
    if scope not in {"telegram", "tg", "emby", "both"}:
        return api_response(False, "scope 仅支持 telegram/emby/both", code=400)

    changed: list[str] = []
    old = {"telegram_id": user.TELEGRAM_ID, "emby_id": user.EMBYID}
    if scope in {"telegram", "tg", "both"} and user.TELEGRAM_ID:
        user.TELEGRAM_ID = None
        changed.append("telegram")
    if scope in {"emby", "both"} and user.EMBYID:
        user.EMBYID = None
        user.PENDING_EMBY = False
        user.PENDING_EMBY_DAYS = None
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}
            if isinstance(other_data, dict):
                other_data.pop("emby_username", None)
                user.OTHER = json.dumps(other_data) if other_data else ""
        changed.append("emby")

    if changed:
        await UserOperate.update_user(user)
        logger.warning(
            "管理员 %s 强制解绑用户 uid=%s scope=%s old=%s",
            g.current_user.USERNAME,
            uid,
            scope,
            old,
        )

    return api_response(True, "已解除绑定" if changed else "没有可解除的绑定", {"changed": changed, "old": old})


@admin_bp.route("/users/sync-bindings", methods=["POST"])
@require_auth
@require_admin
async def sync_user_bindings():
    """强制同步用户 TG/Emby 绑定状态。

    Body:
        scope: "telegram" | "emby" | "both"
        uid: 可选，单个用户
        filter: 可选，role/active/emby/search，用于批量指定类型
        repair: bool，默认 true；为 true 时会清理非法/重复 TG 与失效 EMBYID
    """
    import json
    from src.services.telegram_runtime import run_bot_operation

    data = request.get_json(silent=True) or {}
    scope = (data.get("scope") or "both").strip().lower()
    if scope not in {"telegram", "tg", "emby", "both"}:
        return api_response(False, "scope 仅支持 telegram/emby/both", code=400)
    repair = parse_bool(data.get("repair"), default=True)
    uid = data.get("uid")

    if uid is not None:
        try:
            uid = int(uid)
        except (TypeError, ValueError):
            return api_response(False, "uid 必须是整数", code=400)
        one = await UserOperate.get_user_by_uid(uid)
        users = [one] if one else []
    else:
        f = data.get("filter") if isinstance(data.get("filter"), dict) else {}
        emby_filter = (f.get("emby") or "").strip().lower() if isinstance(f.get("emby"), str) else ""
        has_emby = True if emby_filter == "bound" else False if emby_filter == "unbound" else None
        users, _ = await UserOperate.get_all_users(
            include_inactive=True,
            role=f.get("role") if isinstance(f.get("role"), int) else None,
            active_status=f.get("active") if isinstance(f.get("active"), bool) else None,
            has_emby=has_emby,
            search=(f.get("search") or None) if isinstance(f.get("search"), str) else None,
            limit=100000,
            offset=0,
        )

    if not users:
        return api_response(False, "没有匹配用户", code=404)

    result = {
        "matched": len(users),
        "telegram_checked": 0,
        "telegram_repaired": 0,
        "emby_checked": 0,
        "emby_repaired": 0,
        "synced": 0,
        "failed": [],
        "details": [],
    }

    # TG 重复绑定：保留 UID 最小的一条，其他在 repair=true 时清除。
    if scope in {"telegram", "tg", "both"}:
        tg_map: dict[int, list[UserModel]] = {}
        for u in users:
            if u.TELEGRAM_ID is None:
                continue
            result["telegram_checked"] += 1
            try:
                tg_id = int(u.TELEGRAM_ID)
            except (TypeError, ValueError):
                tg_id = 0
            if tg_id <= 0:
                if repair:
                    u.TELEGRAM_ID = None
                    await UserOperate.update_user(u)
                    result["telegram_repaired"] += 1
                result["details"].append({"uid": u.UID, "scope": "telegram", "action": "invalid_cleared"})
                continue
            tg_map.setdefault(tg_id, []).append(u)

        for tg_id, rows in tg_map.items():
            rows.sort(key=lambda x: int(x.UID))
            for dup in rows[1:]:
                if repair:
                    dup.TELEGRAM_ID = None
                    await UserOperate.update_user(dup)
                    result["telegram_repaired"] += 1
                result["details"].append(
                    {"uid": dup.UID, "scope": "telegram", "action": "duplicate_cleared", "telegram_id": tg_id}
                )
            keeper = rows[0]
            try:

                async def _resolve_chat(bot):
                    return await bot.get_chat(tg_id)

                tg_user = await run_bot_operation(_resolve_chat, timeout=8)
                username = tg_user.username or None
                if username:
                    await UserService.cache_telegram_username(keeper, username)
            except Exception:
                pass

    if scope in {"emby", "both"}:
        emby = get_emby_client()
        for u in users:
            if not u.EMBYID:
                continue
            result["emby_checked"] += 1
            try:
                remote = await emby.get_user(u.EMBYID)
                if not remote:
                    if repair:
                        old_emby_id = u.EMBYID
                        u.EMBYID = None
                        u.PENDING_EMBY = False
                        u.PENDING_EMBY_DAYS = None
                        if u.OTHER:
                            try:
                                other_data = json.loads(u.OTHER)
                            except (json.JSONDecodeError, TypeError):
                                other_data = {}
                            if isinstance(other_data, dict):
                                other_data.pop("emby_username", None)
                                u.OTHER = json.dumps(other_data) if other_data else ""
                        await UserOperate.update_user(u)
                        result["emby_repaired"] += 1
                        result["details"].append(
                            {"uid": u.UID, "scope": "emby", "action": "missing_remote_cleared", "emby_id": old_emby_id}
                        )
                    continue
                if getattr(u, "PENDING_EMBY", False):
                    u.PENDING_EMBY = False
                    u.PENDING_EMBY_DAYS = None
                    await UserOperate.update_user(u)
                    result["emby_repaired"] += 1
                ok, msg = await UserService.sync_user_to_emby(u)
                if ok:
                    result["synced"] += 1
                else:
                    result["failed"].append({"uid": u.UID, "scope": "emby", "reason": msg})
            except Exception as exc:
                result["failed"].append({"uid": u.UID, "scope": "emby", "reason": str(exc)})

    logger.warning("管理员 %s 强制同步绑定状态: %s", g.current_user.USERNAME, result)
    return api_response(True, "同步完成", result)


@admin_bp.route("/users/<int:uid>/renew", methods=["POST"])
@require_auth
@require_admin
async def renew_user(uid: int):
    """
    为用户续期

    Request:
        {
            "days": 30
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    data = request.get_json() or {}
    try:
        days = int(data.get("days", 30))
    except (TypeError, ValueError):
        return api_response(False, "天数必须是整数", code=400)

    # days <= 0 表示设置为永久，与注册码/批量过期策略保持一致。

    success, message = await UserService.renew_user(user, days)
    return api_response(success, message)


@admin_bp.route("/users/<int:uid>/cancel-permanent", methods=["POST"])
@require_auth
@require_admin
async def cancel_user_permanent_expiry(uid: int):
    """取消用户 Emby 账号永久有效期，并设置为从现在起 N 天后到期。

    Request:
        {
            "days": 30
        }
    """
    from src.core.utils import timestamp, days_to_seconds

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if not user.EMBYID:
        return api_response(False, "该用户未绑定 Emby 账号，不能设置 Emby 到期时间", code=400)
    if user.ROLE == Role.ADMIN.value:
        return api_response(False, "不允许取消管理员账号的永久有效期", code=403)

    current_expired_at = int(user.EXPIRED_AT or 0)
    if current_expired_at != -1 and current_expired_at < 253402214400:
        return api_response(False, "该用户当前不是永久有效期", code=400)

    data = request.get_json(silent=True) or {}
    try:
        days = int(data.get("days", 30))
    except (TypeError, ValueError):
        return api_response(False, "days 必须是整数", code=400)
    if days <= 0:
        return api_response(False, "取消永久有效期时 days 必须大于 0", code=400)
    if days > 3650:
        return api_response(False, "days 不能超过 3650", code=400)

    previous_role = user.ROLE
    user.EXPIRED_AT = timestamp() + days_to_seconds(days)
    if user.ROLE == Role.WHITE_LIST.value:
        user.ROLE = Role.NORMAL.value
    if not user.ACTIVE_STATUS:
        user.ACTIVE_STATUS = True
    await UserOperate.update_user(user)
    try:
        await UserService.sync_user_to_emby(user)
    except Exception as exc:
        logger.warning("取消永久后同步 Emby 状态失败 uid=%s: %s", user.UID, exc)

    logger.warning(
        "管理员 %s 取消用户永久有效期 uid=%s username=%s days=%s previous_role=%s new_expired_at=%s",
        getattr(g.current_user, "USERNAME", None),
        user.UID,
        user.USERNAME,
        days,
        previous_role,
        user.EXPIRED_AT,
    )
    return api_response(
        True,
        f"已取消永久有效期，改为 {days} 天后到期" + ("；白名单角色已降级为普通用户" if previous_role == Role.WHITE_LIST.value else ""),
        {
            "uid": user.UID,
            "days": days,
            "expired_at": user.EXPIRED_AT,
            "role": user.ROLE,
            "role_name": Role(user.ROLE).name if user.ROLE in [r.value for r in Role] else "UNKNOWN",
            "downgraded_whitelist": previous_role == Role.WHITE_LIST.value,
        },
    )


@admin_bp.route("/emby/force-set-password", methods=["POST"])
@require_auth
@require_admin
async def admin_force_set_emby_password():
    """直接根据 Emby 用户名重置该 Emby 账号的密码（即使没有绑定本地用户）。

    Request:
        {
            "emby_username": "ada",
            "new_password": "Abcd1234"   // 可选，省略则随机生成 12 位强密码
        }

    Response data:
        {
            "emby_id": "...",
            "emby_username": "ada",
            "new_password": "..."    // 仅当现场生成或显式指定时返回
        }
    """
    from src.services.user_service import UserService
    from src.services.emby import get_emby_client, EmbyError
    from src.core.utils import generate_password, hash_password

    data = request.get_json() or {}
    emby_username = (data.get("emby_username") or "").strip()
    new_password = data.get("new_password")

    if not emby_username:
        return api_response(False, "缺少 emby_username", code=400)

    auto_generated = False
    if new_password:
        ok, msg = UserService.validate_password_strength(new_password, label="新密码")
        if not ok:
            return api_response(False, msg, code=400)
    else:
        new_password = generate_password(12)
        auto_generated = True

    emby = get_emby_client()
    try:
        emby_user = await emby.get_user_by_name(emby_username)
    except EmbyError as e:
        return api_response(False, f"查询 Emby 用户失败: {e}", code=502)
    if not emby_user:
        return api_response(False, f"Emby 中找不到用户「{emby_username}」", code=404)

    # 禁止操作 Emby 管理员，避免越权
    if bool(emby_user.policy.get("IsAdministrator", False)):
        return api_response(False, "不允许通过此接口重置 Emby 管理员密码", code=403)

    try:
        await emby.reset_user_password(emby_user.id)
        ok = await emby.set_user_password(emby_user.id, new_password)
        if not ok:
            return api_response(False, "Emby 设置新密码失败", code=502)
    except EmbyError as e:
        logger.error(f"重置 Emby 密码失败 ({emby_username}): {e}", exc_info=True)
        return api_response(False, f"重置失败: {e}", code=502)

    # 如有本地账号绑定到这个 EMBYID，同步刷新本地系统密码哈希以免双密码漂移
    local = await UserOperate.get_user_by_embyid(emby_user.id)
    if local is not None:
        try:
            local.PASSWORD = hash_password(new_password)
            await UserOperate.update_user(local)
        except Exception as exc:  # pragma: no cover - DB safety
            logger.warning(f"同步本地密码哈希失败 ({local.USERNAME}): {exc}")

    logger.info(
        f"管理员 {g.current_user.USERNAME} 强制重置 Emby 密码: {emby_username} "
        f"(EMBYID={emby_user.id}{', 本地账号已同步' if local else ', 无本地绑定'})"
    )

    return api_response(
        True,
        "Emby 密码已重置",
        {
            "emby_id": emby_user.id,
            "emby_username": emby_user.name,
            "linked_local_user": bool(local),
            "new_password": new_password if auto_generated else new_password,
        },
    )


@admin_bp.route("/users/<int:uid>/reset-password", methods=["POST"])
@require_auth
@require_admin
async def reset_user_password(uid: int):
    """重置用户密码（管理员）。

    Request body (全部可选)：
        {
            "scope": "system" | "emby" | "both",  // 默认 both，保持向后兼容
            "password": "Custom1234"               // 可选，省略时自动生成 12 位强密码
        }

    Response data：
        {"scope": "...", "new_password": "...", "auto_generated": true/false}
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    # 不允许重置其他管理员密码，降低越权风险
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许重置其他管理员密码", code=403)

    data = request.get_json(silent=True) or {}
    scope = (data.get("scope") or "both").strip().lower()
    if scope not in ("system", "emby", "both"):
        return api_response(False, "scope 必须是 system / emby / both", code=400)

    custom_password = data.get("password")
    if custom_password is not None and not isinstance(custom_password, str):
        return api_response(False, "password 必须是字符串", code=400)
    custom_password = (custom_password or "").strip() or None
    auto_generated = custom_password is None

    success, message, new_password = await UserService.reset_password(
        user,
        scope=scope,
        custom_password=custom_password,
    )
    if not success:
        return api_response(False, message, code=400)

    logger.warning(
        "管理员 %s 重置用户密码: uid=%d username=%s scope=%s auto=%s",
        g.current_user.USERNAME,
        user.UID,
        user.USERNAME,
        scope,
        auto_generated,
    )
    return api_response(
        True,
        message,
        {
            "scope": scope,
            "new_password": new_password,
            "auto_generated": auto_generated,
        },
    )


@admin_bp.route("/users/<int:uid>/kick", methods=["POST"])
@require_auth
@require_admin
async def kick_user(uid: int):
    """踢出用户所有会话"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    success, kicked = await EmbyService.kick_user_sessions(user)

    if success:
        return api_response(True, f"已踢出 {kicked} 个会话", {"kicked_count": kicked})
    return api_response(False, "操作失败")


@admin_bp.route("/users/<int:uid>/libraries", methods=["GET"])
@require_auth
@require_admin
async def get_user_libraries(uid: int):
    """
    获取用户媒体库访问权限（管理员）

    Response:
        {
            "all_libraries": [{"id": "...", "name": "...", "type": "..."}],
            "enabled_ids": ["id1", "id2"],
            "enable_all": false
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    if not user.EMBYID:
        return api_response(
            True,
            "用户尚未绑定 Emby",
            {
                "all_libraries": [],
                "enabled_ids": [],
                "enable_all": False,
                "has_emby": False,
                "blocked_names": [],
                "libraries": [],
                "default_hidden_libraries": EmbyService.default_hidden_library_names(),
                "self_service_libraries": EmbyService.self_service_library_names(),
                "self_service_enabled": EmbyService.can_self_service_libraries(user),
            },
        )

    detail = await EmbyService.get_user_library_access_detail(user, include_self_service_config=True)

    return api_response(
        True,
        "获取成功",
        detail,
    )


@admin_bp.route("/users/<int:uid>/libraries", methods=["PUT"])
@require_auth
@require_admin
async def set_user_libraries(uid: int):
    """
    设置用户媒体库权限

    支持按名称或ID设置，优先使用名称。

    Request:
        {
            "action": "set",                     // set/show/hide/enable_all/disable_all
            "library_names": ["电影", "电视剧"],   // 按名称（推荐）
            "library_ids": ["id1", "id2"],          // 按ID（兼容）
            "enable_all": false
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    if not user.EMBYID:
        return api_response(False, "用户没有关联的 Emby 账户", code=400)

    data = request.get_json() or {}
    library_names = EmbyService._normalize_library_names(data.get("library_names", []))
    raw_library_ids = data.get("library_ids", [])
    library_ids = [str(i or "").strip() for i in raw_library_ids if str(i or "").strip()] if isinstance(raw_library_ids, list) else []
    enable_all = parse_bool(data.get("enable_all"), default=False)
    action = str(data.get("action") or "set").strip().lower()

    if action in {"show", "hide"}:
        if not library_names and library_ids:
            resolved_names, not_found_ids = await get_emby_client().get_folder_names_by_ids(library_ids)
            if not_found_ids:
                return api_response(False, f"未找到以下媒体库 ID: {', '.join(not_found_ids)}", code=400)
            library_names = resolved_names
        if not library_names:
            return api_response(False, "媒体库名称不能为空", code=400)
        success, message = await EmbyService.set_user_library_visibility(
            user,
            library_names,
            visible=(action == "show"),
        )
    elif action == "enable_all":
        success, message = await EmbyService.set_user_library_access(user, [], True)
    elif action == "disable_all":
        success = await get_emby_client().disable_all_folders_for_user(user.EMBYID)
        message = "已隐藏全部媒体库" if success else "更新失败"
    elif action == "set":
        # 优先使用名称解析
        if library_names:
            resolved_ids, not_found = await EmbyService.resolve_library_names_to_ids(library_names)
            if not_found:
                return api_response(False, f"未找到以下媒体库: {', '.join(not_found)}", code=400)
            library_ids = resolved_ids

        success, message = await EmbyService.set_user_library_access(user, library_ids, enable_all)
    else:
        return api_response(False, "action 必须是 set/show/hide/enable_all/disable_all", code=400)

    if not success:
        return api_response(False, message, code=500)
    detail = await EmbyService.get_user_library_access_detail(user, include_self_service_config=True)
    return api_response(True, message, detail)


@admin_bp.route("/users/<int:uid>/library-self-service", methods=["PUT"])
@require_auth
@require_admin
async def set_user_library_self_service(uid: int):
    """管理员开启/关闭单个用户的媒体库自助显隐权限。"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.ROLE == Role.UNRECOGNIZED.value:
        return api_response(False, "未注册用户不能授予媒体库自助显隐权限", code=400)

    data = request.get_json(silent=True) or {}
    if "enabled" not in data:
        return api_response(False, "缺少 enabled 字段", code=400)
    raw_enabled = data.get("enabled")
    if not isinstance(raw_enabled, (bool, int, float, str)):
        return api_response(False, "enabled 必须是布尔值", code=400)
    enabled = parse_bool(raw_enabled, default=False)

    updated = await UserOperate.set_library_self_service(uid, enabled)
    if not updated:
        return api_response(False, "更新失败", code=500)

    logger.info(
        "管理员 %s %s用户媒体库自助显隐权限 uid=%s username=%s",
        g.current_user.USERNAME,
        "开启" if enabled else "关闭",
        user.UID,
        user.USERNAME,
    )
    return api_response(True, "已更新", {"uid": uid, "library_self_service": enabled})


@admin_bp.route("/users/library-self-service/bulk-enable", methods=["POST"])
@require_auth
@require_admin
async def bulk_enable_library_self_service():
    """为所有已注册用户开启媒体库自助显隐权限。"""
    data = request.get_json(silent=True) or {}
    confirm = str(data.get("confirm") or "").strip()
    if confirm != "ENABLE_LIBRARY_SELF_SERVICE":
        return api_response(False, "缺少确认字段", code=400)

    updated = await UserOperate.enable_library_self_service_for_all()
    logger.warning(
        "管理员 %s 批量开启媒体库自助显隐权限 updated=%s",
        g.current_user.USERNAME,
        updated,
    )
    return api_response(True, f"已为 {updated} 个用户开启媒体库自助显隐权限", {"updated": updated})


@admin_bp.route("/users/<int:uid>/admin", methods=["PUT"])
@require_auth
@require_admin
async def set_user_admin(uid: int):
    """
    设置/取消管理员权限

    Request:
        {
            "is_admin": true
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许修改其他管理员权限", code=403)

    data = request.get_json() or {}
    is_admin = bool(data.get("is_admin", False))
    if user.UID == g.current_user.UID and not is_admin:
        return api_response(False, "不允许撤销自己的管理员权限", code=403)

    success, message = await UserService.set_user_admin(user, is_admin)
    return api_response(success, message)


@admin_bp.route("/users/<int:uid>/unbind-telegram", methods=["POST"])
@require_auth
@require_admin
async def unbind_user_telegram(uid: int):
    """
    解绑用户的 Telegram

    解绑后用户将无法通过 Telegram 登录，但可以通过 API Key 或其他方式访问。
    解绑后 Telegram ID 会被清空，用户可以重新绑定其他 Telegram 账号。
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许解绑其他管理员的 Telegram", code=403)

    if not user.TELEGRAM_ID:
        return api_response(False, "该用户未绑定 Telegram", code=400)

    old_telegram_id = user.TELEGRAM_ID
    user.TELEGRAM_ID = None
    await UserOperate.update_user(user)

    return api_response(
        True,
        f"已解绑 Telegram (原 ID: {old_telegram_id})",
        {
            "uid": uid,
            "username": user.USERNAME,
            "old_telegram_id": old_telegram_id,
        },
    )


@admin_bp.route("/users/<int:uid>/bind-telegram", methods=["POST"])
@require_auth
@require_admin
async def bind_user_telegram(uid: int):
    """
    为用户绑定 Telegram

    Request:
        {
            "telegram_id": 123456789
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许修改其他管理员的 Telegram", code=403)

    data = request.get_json() or {}
    telegram_id = data.get("telegram_id")

    if not telegram_id:
        return api_response(False, "缺少 telegram_id", code=400)

    if not isinstance(telegram_id, int) or telegram_id <= 0:
        return api_response(False, "telegram_id 格式无效", code=400)

    # 检查该 Telegram ID 是否已被其他用户绑定
    existing = await UserOperate.get_user_by_telegram_id(telegram_id)
    if existing and existing.UID != uid:
        return api_response(False, f"该 Telegram ID 已被用户 {existing.USERNAME} 绑定", code=400)

    old_telegram_id = user.TELEGRAM_ID
    user.TELEGRAM_ID = telegram_id
    await UserOperate.update_user(user)

    return api_response(
        True,
        "绑定成功",
        {
            "uid": uid,
            "username": user.USERNAME,
            "telegram_id": telegram_id,
            "old_telegram_id": old_telegram_id,
        },
    )


@admin_bp.route("/telegram/rebind-requests", methods=["GET"])
@require_auth
@require_admin
async def list_telegram_rebind_requests():
    """获取 Telegram 换绑请求列表"""
    page = max(request.args.get("page", 1, type=int) or 1, 1)
    per_page = min(max(request.args.get("per_page", 20, type=int) or 20, 1), 100)
    status = (request.args.get("status") or "").strip().lower() or None
    if status and status not in {"pending", "approved", "rejected"}:
        return api_response(False, "无效状态", code=400)

    requests, total = await UserService.list_telegram_rebind_requests(status=status, page=page, per_page=per_page)
    payload = []
    for req in requests:
        user = await UserOperate.get_user_by_uid(req.UID)
        payload.append(
            {
                "id": req.ID,
                "uid": req.UID,
                "username": user.USERNAME if user else None,
                "old_telegram_id": req.OLD_TELEGRAM_ID,
                "status": req.STATUS,
                "reason": req.REASON,
                "admin_note": req.ADMIN_NOTE,
                "reviewer_uid": req.REVIEWER_UID,
                "created_at": req.CREATED_AT,
                "reviewed_at": req.REVIEWED_AT,
            }
        )

    return api_response(
        True,
        "获取成功",
        {
            "requests": payload,
            "total": total,
        },
    )


@admin_bp.route("/telegram/rebind-requests/<int:request_id>/approve", methods=["POST"])
@require_auth
@require_admin
async def approve_telegram_rebind_request(request_id: int):
    data = request.get_json() or {}
    admin_note = data.get("admin_note")
    success, message = await UserService.approve_telegram_rebind_request(request_id, g.current_user.UID, admin_note)
    return api_response(success, message, code=200 if success else 400)


@admin_bp.route("/telegram/rebind-requests/<int:request_id>/reject", methods=["POST"])
@require_auth
@require_admin
async def reject_telegram_rebind_request(request_id: int):
    data = request.get_json() or {}
    admin_note = data.get("admin_note")
    success, message = await UserService.reject_telegram_rebind_request(request_id, g.current_user.UID, admin_note)
    return api_response(success, message, code=200 if success else 400)


@admin_bp.route("/telegram/rebind-requests/batch", methods=["POST"])
@require_auth
@require_admin
async def batch_review_telegram_rebind_requests():
    data = request.get_json(silent=True) or {}
    action = (data.get("action") or "").strip().lower()
    if action not in {"approve", "reject"}:
        return api_response(False, "action 仅支持 approve/reject", code=400)
    ids = data.get("ids")
    if not isinstance(ids, list) or not ids:
        return api_response(False, "ids 必须是非空数组", code=400)
    if len(ids) > 100:
        return api_response(False, "单次最多处理 100 条换绑请求", code=400)

    result = await UserService.batch_review_telegram_rebind_requests(
        ids,
        action,
        g.current_user.UID,
        data.get("admin_note"),
    )
    message = f"批量处理完成：成功 {result['success']}，失败 {result['failed']}"
    return api_response(True, message, result)


@admin_bp.route("/users/by-telegram/<int:telegram_id>", methods=["GET"])
@require_auth
@require_admin
async def get_user_by_telegram(telegram_id: int):
    """根据 Telegram ID 查找用户"""
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "未找到绑定该 Telegram ID 的用户", code=404)

    user_info = await UserService.get_user_info(user)
    return api_response(True, "找到用户", user_info)


# ==================== Emby 同步 ====================


@admin_bp.route("/emby/sync", methods=["POST"])
@require_auth
@require_admin
async def sync_all_emby():
    """
    批量同步所有 Emby 用户数据

    检测孤儿记录、同步用户名、同步状态和权限。

    Response:
        {
            "success": 5,
            "failed": 1,
            "errors": ["username: detail"]
        }
    """
    success, failed, errors = await EmbyService.sync_all_users()
    return api_response(
        True,
        f"同步完成: 成功 {success}, 失败 {failed}",
        {
            "success": success,
            "failed": failed,
            "errors": errors,
        },
    )


# ==================== 注册码管理 ====================


@admin_bp.route("/regcodes", methods=["GET"])
@require_auth
@require_admin
async def list_regcodes():
    """
    获取注册码列表

    Query: page, per_page, type, status, search, sort, order
    """
    page = request.args.get("page", 1, type=int)
    per_page = min(max(request.args.get("per_page", 20, type=int), 1), 100)
    code_type = request.args.get("type", type=int)
    status = (request.args.get("status") or "all").strip().lower()
    search = (request.args.get("search") or "").strip().lower()
    sort = (request.args.get("sort") or "created_time").strip().lower()
    order = (request.args.get("order") or "desc").strip().lower()
    active_only = request.args.get("active", "false").lower() == "true"

    if code_type:
        codes = await RegCodeOperate.get_regcodes_by_type(code_type)
    else:
        codes = await RegCodeOperate.get_all_regcodes()

    # 兼容旧 active=true 参数。
    if active_only:
        status = "available"

    def _is_used_up(c) -> bool:
        return c.USE_COUNT_LIMIT > 0 and c.USE_COUNT >= c.USE_COUNT_LIMIT

    now = int(__import__("time").time())

    def _is_expired(c) -> bool:
        return c.VALIDITY_TIME > 0 and now > c.CREATED_TIME + c.VALIDITY_TIME * 3600

    if status == "available":
        codes = [c for c in codes if c.ACTIVE and not _is_used_up(c) and not _is_expired(c)]
    elif status == "disabled":
        codes = [c for c in codes if not c.ACTIVE]
    elif status == "used_up":
        codes = [c for c in codes if _is_used_up(c)]
    elif status == "expired":
        codes = [c for c in codes if _is_expired(c)]
    elif status == "active":
        codes = [c for c in codes if c.ACTIVE]
    elif status == "decoy":
        codes = [c for c in codes if RegCodeOperate.is_decoy(c)]

    if search:
        codes = [
            c for c in codes
            if search in c.CODE.lower()
            or search in (RegCodeOperate.get_note(c) or "").lower()
            or search in (c.UID or "").lower()
        ]

    sort_getters = {
        "code": lambda c: c.CODE,
        "type": lambda c: c.TYPE,
        "days": lambda c: c.DAYS if c.DAYS is not None else 0,
        "use_count": lambda c: c.USE_COUNT,
        "use_count_limit": lambda c: c.USE_COUNT_LIMIT,
        "created_time": lambda c: c.CREATED_TIME,
        "status": lambda c: (0 if c.ACTIVE and not _is_used_up(c) else 1),
        "note": lambda c: RegCodeOperate.get_note(c),
    }
    codes.sort(key=sort_getters.get(sort, sort_getters["created_time"]), reverse=(order != "asc"))

    # 分页处理
    total = len(codes)
    start = (page - 1) * per_page
    end = start + per_page
    paginated_codes = codes[start:end]

    return api_response(
        True,
        f"共 {total} 个注册码",
        {
            "regcodes": [
                {
                    "code": c.CODE,
                    "type": c.TYPE,
                    "type_name": {1: "注册", 2: "续期", 3: "白名单"}.get(c.TYPE, "未知"),
                    "is_decoy": RegCodeOperate.is_decoy(c),
                    "validity_time": c.VALIDITY_TIME,
                    "use_count": c.USE_COUNT,
                    "use_count_limit": c.USE_COUNT_LIMIT,
                    "days": c.DAYS,
                    "active": c.ACTIVE,
                    "status": "disabled" if not c.ACTIVE else "expired" if _is_expired(c) else "used_up" if _is_used_up(c) else "available",
                    "note": RegCodeOperate.get_note(c),
                    "created_time": c.CREATED_TIME,
                    "used_by": c.UID,
                    "used_by_uids": [int(x) for x in (c.UID or "").split(",") if x.strip().isdigit()],
                    "used_by_telegram_ids": [int(x) for x in (c.TELEGRAM_ID or "").split(",") if x.strip().isdigit()],
                }
                for c in paginated_codes
            ],
            "total": total,
            "page": page,
            "per_page": per_page,
        },
    )


# ==================== 求片管理 ====================


@admin_bp.route("/media-requests", methods=["GET"])
@require_auth
@require_admin
async def list_media_requests():
    """
    获取求片请求列表（管理员）

    Query:
        page: int - 页码（默认 1）
        status: str - 状态筛选 (pending/accepted/rejected/completed，默认 pending)
    """
    from src.services import MediaRequestService
    from src.db.bangumi import BangumiRequireOperate, ReqStatus
    import json

    page = request.args.get("page", 1, type=int)
    per_page = min(max(request.args.get("per_page", 20, type=int), 1), 100)
    status_filter = request.args.get("status", "pending").lower()

    # 转换状态
    status_map = {
        "pending": ReqStatus.UNHANDLED,
        "unhandled": ReqStatus.UNHANDLED,
        "accepted": ReqStatus.ACCEPTED,
        "rejected": ReqStatus.REJECTED,
        "completed": ReqStatus.COMPLETED,
        "downloading": ReqStatus.DOWNLOADING,
    }

    # 获取请求列表
    if status_filter == "all":
        # 全部状态
        requests = await BangumiRequireOperate.get_all_requires()
    elif status_filter == "pending":
        # 待处理：获取所有未处理/已接受/下载中的
        requests = await BangumiRequireOperate.get_all_pending_list()
    elif status_filter in status_map:
        # 其他单一状态
        requests = await BangumiRequireOperate.get_all_requires_by_status(status_map[status_filter])
    else:
        # 未识别状态默认 pending
        requests = await BangumiRequireOperate.get_all_pending_list()

    telegram_ids = [req.telegram_id for req in requests if req.telegram_id is not None]
    users_map = await UserOperate.get_users_by_telegram_ids(telegram_ids)

    # 转换为字典格式
    results = []
    for req in requests:
        other = {}
        if req.other_info:
            try:
                other = json.loads(req.other_info)
            except:
                pass

        user = users_map.get(req.telegram_id)

        status_name = ReqStatus(req.status).name.lower()
        if status_name == "unhandled":
            status_name = "pending"

        # 整合媒体信息
        m_info = other.get("media_info", other) if other else {}
        if not m_info.get("title"):
            m_info["title"] = req.title
        if not m_info.get("season"):
            m_info["season"] = req.season
        if not m_info.get("media_type"):
            m_info["media_type"] = req.media_type

        results.append(
            {
                "id": req.id,
                "media_id": getattr(req, "bangumi_id", getattr(req, "tmdb_id", None)),
                "source": "bangumi" if hasattr(req, "bangumi_id") else "tmdb",
                "status": status_name,
                "timestamp": req.timestamp,
                "title": req.title,
                "season": req.season,
                "media_type": req.media_type,
                "require_key": req.require_key,
                "admin_note": req.admin_note,
                "media_info": m_info,
                "user": (
                    {
                        "telegram_id": req.telegram_id,
                        "username": user.USERNAME if user else None,
                        "uid": user.UID if user else None,
                    }
                    if user
                    else None
                ),
            }
        )

    # 分页
    total = len(results)
    start = (page - 1) * per_page
    end = start + per_page
    paginated_results = results[start:end]

    return api_response(
        True,
        "获取成功",
        {
            "requests": paginated_results,
            "total": total,
            "page": page,
            "per_page": per_page,
        },
    )


@admin_bp.route("/media-requests/<int:request_id>", methods=["PUT", "DELETE"])
@require_auth
@require_admin
async def update_or_delete_media_request(request_id: int):
    """更新或删除求片请求（管理员，按数值 id；同 id 存在于两个 source 表时建议改用 by-key 接口）"""
    from src.db.bangumi import BangumiRequireOperate

    if request.method == "DELETE":
        req = await BangumiRequireOperate.get_require(request_id)
        if not req:
            return api_response(False, "请求不存在", code=404)
        source = "bangumi" if hasattr(req, "bangumi_id") else "tmdb"
        success = await BangumiRequireOperate.delete_require(request_id, source)
        return api_response(success, "请求已删除" if success else "删除失败")

    from src.services import MediaRequestService
    from src.db.bangumi import ReqStatus

    data = request.get_json() or {}
    status_str = data.get("status", "").lower()
    note = (data.get("note") or "").strip()

    if len(note) > 1000:
        return api_response(False, "管理员备注过长，最多 1000 字符", code=400)

    # 转换状态
    status_map = {
        "pending": ReqStatus.UNHANDLED,
        "accepted": ReqStatus.ACCEPTED,
        "rejected": ReqStatus.REJECTED,
        "completed": ReqStatus.COMPLETED,
        "downloading": ReqStatus.DOWNLOADING,
    }

    if status_str not in status_map:
        return api_response(False, f"无效状态，支持: {', '.join(status_map.keys())}", code=400)

    target_status = status_map[status_str]

    # 尝试从 body 获取 source 或通过 ID 自动寻找
    source = data.get("source")

    # 更新状态
    success, message = await MediaRequestService.update_request_status(request_id, target_status, note, source)

    if success:
        return api_response(True, message or f"状态已更新为 {status_str}")
    else:
        return api_response(False, message or "请求不存在", code=404)


@admin_bp.route("/media-requests/by-key/<string:require_key>", methods=["PUT", "DELETE"])
@require_auth
@require_admin
async def update_or_delete_media_request_by_key(require_key: str):
    """按 require_key（全局唯一）更新或删除求片请求。

    推荐用这条而非按 id 的版本：Bangumi 与 TMDB 两张 require 表各自自增 id，
    数值 id 可能撞车，会让操作落到错误的求片上。
    """
    from src.db.bangumi import BangumiRequireOperate
    from src.services import MediaRequestService
    from src.db.bangumi import ReqStatus

    if not require_key or len(require_key) > 64:
        return api_response(False, "require_key 缺失或格式不合法", code=400)

    if request.method == "DELETE":
        success = await BangumiRequireOperate.delete_require_by_key(require_key)
        if success:
            return api_response(True, "请求已删除")
        return api_response(False, "请求不存在", code=404)

    data = request.get_json() or {}
    status_str = data.get("status", "").lower()
    note = (data.get("note") or "").strip()

    if len(note) > 1000:
        return api_response(False, "管理员备注过长，最多 1000 字符", code=400)

    status_map = {
        "pending": ReqStatus.UNHANDLED,
        "accepted": ReqStatus.ACCEPTED,
        "rejected": ReqStatus.REJECTED,
        "completed": ReqStatus.COMPLETED,
        "downloading": ReqStatus.DOWNLOADING,
    }
    if status_str not in status_map:
        return api_response(False, f"无效状态，支持: {', '.join(status_map.keys())}", code=400)

    # update_status_by_key 接收 ReqStatus + 可选 note
    success = await BangumiRequireOperate.update_status_by_key(
        require_key,
        status_map[status_str],
        note=note or None,
    )
    if success:
        return api_response(True, f"状态已更新为 {status_str}")
    return api_response(False, "请求不存在", code=404)


@admin_bp.route("/regcodes", methods=["POST"])
@require_auth
@require_admin
async def create_regcode():
    """
    创建注册码

    Request:
        {
            "type": 1,              // 1=注册, 2=续期, 3=白名单
            "validity_time": -1,    // 有效期（小时），-1 永久
            "use_count_limit": 1,   // 使用次数限制，-1 无限
            "days": 30,             // 有效天数（0 或 -1 表示永久）
            "count": 1              // 生成数量
        }
    """
    data = request.get_json() or {}

    try:
        code_type = int(data.get("type", 1))
        validity_time = int(data.get("validity_time", -1))
        use_count_limit = int(data.get("use_count_limit", 1))
        days = int(data.get("days", 30))
        count = int(data.get("count", 1))
    except (TypeError, ValueError):
        return api_response(False, "参数类型错误，请检查 type/validity_time/use_count_limit/days/count", code=400)

    decoy = parse_bool(data.get("decoy"), default=False)
    code_format = (data.get("format") or "").strip() or None
    random_algorithm = (data.get("random_algorithm") or "").strip() or None

    if code_type not in (1, 2, 3):
        return api_response(False, "type 仅支持 1=注册, 2=续期, 3=白名单", code=400)

    if validity_time < -1:
        return api_response(False, "validity_time 不能小于 -1", code=400)

    if use_count_limit < -1 or use_count_limit == 0:
        return api_response(False, "use_count_limit 仅支持 -1 或大于 0 的整数", code=400)

    allowed_algorithms = {
        "base32-20",
        "base32-24",
        "hex32",
        "hex20",
        "base32-16",
        "alnum-24",
        "alnum-16",
        "urlsafe-24",
        "digits-16",
        "digits-12",
        "uuid",
        "legacy-sha1",
    }
    if random_algorithm and random_algorithm.lower() not in allowed_algorithms:
        return api_response(False, "random_algorithm 不支持", code=400)

    # 0 和 -1 都表示永久
    if days <= 0:
        days = -1

    if count < 1 or count > 100:
        return api_response(False, "生成数量必须在 1-100 之间", code=400)

    try:
        codes = await RegCodeOperate.create_regcode(
            validity_time,
            code_type,
            use_count_limit,
            count,
            days,
            code_format=code_format,
            random_algorithm=random_algorithm,
            decoy=decoy,
        )
    except Exception as exc:
        logger.warning("生成卡码失败: %s", exc, exc_info=True)
        return api_response(False, f"生成失败: {exc}", code=400)

    return api_response(
        True,
        "创建成功",
        {
            "codes": codes if isinstance(codes, list) else [codes],
            "count": count,
            "decoy": decoy,
        },
    )


@admin_bp.route("/regcodes/<code>", methods=["DELETE"])
@require_auth
@require_admin
async def delete_regcode(code: str):
    """删除注册码"""
    success = await RegCodeOperate.delete_regcode(code)

    if success:
        return api_response(True, "删除成功")
    return api_response(False, "注册码不存在或删除失败")


@admin_bp.route("/regcodes/<code>", methods=["PUT"])
@require_auth
@require_admin
async def update_regcode(code: str):
    """更新单个注册码元信息。目前支持 note 备注。"""
    data = request.get_json(silent=True) or {}
    note = (data.get("note") or "").strip()
    if len(note) > 120:
        return api_response(False, "备注最多 120 个字符", code=400)
    success = await RegCodeOperate.update_note(code, note)
    if not success:
        return api_response(False, "注册码不存在或更新失败", code=404)
    return api_response(True, "更新成功", {"code": code, "note": note})


@admin_bp.route("/regcodes/<code>/users", methods=["GET"])
@require_auth
@require_admin
async def get_regcode_users(code: str):
    """查看单个注册码的使用者详情。"""
    reg_code = await RegCodeOperate.get_regcode_by_code(code)
    if not reg_code:
        return api_response(False, "注册码不存在", code=404)

    uid_values = [int(x) for x in (reg_code.UID or "").split(",") if x.strip().isdigit()]
    tg_values = [int(x) for x in (reg_code.TELEGRAM_ID or "").split(",") if x.strip().isdigit()]

    users: list[dict] = []
    seen_uids: set[int] = set()
    for uid in uid_values:
        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            users.append({"uid": uid, "found": False, "source": "uid"})
            continue
        seen_uids.add(int(user.UID))
        info = await UserService.get_user_info(user)
        info["found"] = True
        info["source"] = "uid"
        users.append(info)

    telegram_only: list[dict] = []
    for tg_id in tg_values:
        user = await UserOperate.get_user_by_telegram_id(tg_id)
        if user and int(user.UID) not in seen_uids:
            seen_uids.add(int(user.UID))
            info = await UserService.get_user_info(user)
            info["found"] = True
            info["source"] = "telegram"
            users.append(info)
        elif not user:
            telegram_only.append({"telegram_id": tg_id, "found": False, "source": "telegram"})

    return api_response(
        True,
        "获取成功",
        {
            "code": code,
            "use_count": reg_code.USE_COUNT,
            "users": users,
            "telegram_only": telegram_only,
        },
    )


# ==================== Emby 管理 ====================


@admin_bp.route("/emby/sessions", methods=["GET"])
@require_auth
@require_admin
async def list_sessions():
    """获取所有活动会话"""
    sessions = await EmbyService.get_all_sessions()
    return api_response(True, "获取成功", sessions)


@admin_bp.route("/emby/activity", methods=["GET"])
@require_auth
@require_admin
async def get_activity_log():
    """
    获取活动日志

    Query:
        limit: int - 返回数量（默认 50，最大 200）
    """
    limit = request.args.get("limit", 50, type=int)
    limit = min(max(limit, 1), 200)

    logs = await EmbyService.get_activity_log(limit)
    return api_response(True, "获取成功", logs)


@admin_bp.route("/emby/broadcast", methods=["POST"])
@require_auth
@require_admin
async def broadcast_message():
    """
    广播消息到所有会话

    Request:
        {
            "header": "通知",
            "text": "消息内容"
        }
    """
    data = request.get_json() or {}
    header = data.get("header", "通知")
    text = data.get("text")

    if not text:
        return api_response(False, "缺少消息内容", code=400)

    sent = await EmbyService.broadcast_message(header, text)
    return api_response(True, f"已发送到 {sent} 个会话", {"sent_count": sent})


# ==================== 白名单用户 ====================


@admin_bp.route("/whitelist", methods=["POST"])
@require_auth
@require_admin
async def create_whitelist_user():
    """
    创建白名单用户（永久有效）

    Request:
        {
            "telegram_id": 123456789,
            "username": "whiteuser",
            "email": "user@example.com"
        }
    """
    data = request.get_json() or {}

    telegram_id = data.get("telegram_id")
    username = data.get("username")
    email = data.get("email")

    if not telegram_id or not username:
        return api_response(False, "缺少必要参数", code=400)

    result = await UserService.create_whitelist_user(telegram_id, username, email)

    if result.result.value == "success":
        return api_response(
            True,
            result.message,
            {
                "username": result.user.USERNAME if result.user else None,
                "password": result.emby_password,
            },
        )

    return api_response(False, result.message, code=400)


# ==================== 统计信息 ====================


@admin_bp.route("/stats", methods=["GET"])
@require_auth
@require_admin
async def get_stats():
    """获取系统统计信息"""
    from src.config import RegisterConfig

    registered_count = await UserOperate.get_registered_users_count()
    active_count = await UserOperate.get_active_users_count()
    regcode_count = await RegCodeOperate.get_active_regcodes_count()
    server_status = await EmbyService.get_server_status()

    return api_response(
        True,
        "获取成功",
        {
            "users": {
                "registered": registered_count,
                "active": active_count,
                "limit": RegisterConfig.USER_LIMIT,
            },
            "regcodes": {
                "active": regcode_count,
            },
            "emby": {
                "online": server_status.get("online", False),
                "active_sessions": server_status.get("active_sessions", 0),
            },
        },
    )


# ==================== Emby 管理 ====================


@admin_bp.route("/emby/test", methods=["POST"])
@require_auth
@require_admin
async def test_emby_connectivity():
    """一键测试 Emby 连通性（网络、认证、用户列表、媒体库）"""
    from src.config import EmbyConfig
    import time as _time

    results = {
        "emby_url": EmbyConfig.EMBY_URL,
        "tests": [],
        "overall": True,
    }
    emby = get_emby_client()

    # Test 1: Ping
    t0 = _time.time()
    try:
        ok = await emby.ping()
        latency = round((_time.time() - t0) * 1000)
        results["tests"].append(
            {
                "name": "网络连通",
                "success": ok,
                "latency_ms": latency,
                "message": f"延迟 {latency}ms" if ok else "无法连接到 Emby 服务器",
            }
        )
        if not ok:
            results["overall"] = False
    except Exception as e:
        results["tests"].append({"name": "网络连通", "success": False, "message": str(e)})
        results["overall"] = False

    # Test 2: Server Info (tests API auth)
    t0 = _time.time()
    try:
        info = await emby.get_server_info()
        latency = round((_time.time() - t0) * 1000)
        results["tests"].append(
            {
                "name": "API 认证",
                "success": True,
                "latency_ms": latency,
                "message": f"服务器: {info.get('ServerName', '?')}, 版本: {info.get('Version', '?')}",
            }
        )
        results["server_info"] = {
            "name": info.get("ServerName"),
            "version": info.get("Version"),
            "os": info.get("OperatingSystemDisplayName"),
            "id": info.get("Id"),
        }
    except EmbyError as e:
        results["tests"].append({"name": "API 认证", "success": False, "message": f"认证失败: {e}"})
        results["overall"] = False

    # Test 3: User list
    t0 = _time.time()
    try:
        users = await emby.get_users()
        latency = round((_time.time() - t0) * 1000)
        results["tests"].append(
            {
                "name": "用户列表",
                "success": True,
                "latency_ms": latency,
                "message": f"共 {len(users)} 个 Emby 用户",
            }
        )
    except EmbyError as e:
        results["tests"].append({"name": "用户列表", "success": False, "message": str(e)})
        results["overall"] = False

    # Test 4: Libraries
    t0 = _time.time()
    try:
        libs = await emby.get_libraries()
        latency = round((_time.time() - t0) * 1000)
        results["tests"].append(
            {
                "name": "媒体库",
                "success": True,
                "latency_ms": latency,
                "message": f"共 {len(libs)} 个媒体库",
            }
        )
    except EmbyError as e:
        results["tests"].append({"name": "媒体库", "success": False, "message": str(e)})
        results["overall"] = False

    return api_response(True, "测试完成", results)


@admin_bp.route("/emby/users", methods=["GET"])
@require_auth
@require_admin
async def list_emby_users():
    """获取 Emby 用户列表，与本地数据库对比，返回绑定状态和孤儿记录"""
    emby = get_emby_client()

    try:
        emby_users = await emby.get_users()
    except EmbyError as e:
        return api_response(False, f"无法连接 Emby: {e}", code=500)

    local_emby_users = await UserOperate.get_all_emby_users()
    local_by_embyid = {u.EMBYID: u for u in local_emby_users}

    result = []
    for eu in emby_users:
        local_user = local_by_embyid.get(eu.id)
        sync_status = "unlinked"
        if local_user:
            sync_status = "synced" if local_user.USERNAME == eu.name else "name_mismatch"

        result.append(
            {
                "emby_id": eu.id,
                "emby_name": eu.name,
                "has_password": eu.has_password,
                "is_admin": eu.policy.get("IsAdministrator", False),
                "is_disabled": eu.policy.get("IsDisabled", False),
                "is_hidden": eu.policy.get("IsHidden", False),
                "last_login": eu.last_login_date,
                "last_activity": eu.last_activity_date,
                "local_user": (
                    {
                        "uid": local_user.UID,
                        "username": local_user.USERNAME,
                        "telegram_id": local_user.TELEGRAM_ID,
                        "active": local_user.ACTIVE_STATUS,
                        "role": local_user.ROLE,
                    }
                    if local_user
                    else None
                ),
                "sync_status": sync_status,
            }
        )

    # 本地有 EMBYID 但 Emby 端不存在的孤儿记录
    emby_id_set = {eu.id for eu in emby_users}
    orphans = [
        {
            "uid": u.UID,
            "username": u.USERNAME,
            "emby_id": u.EMBYID,
            "telegram_id": u.TELEGRAM_ID,
        }
        for u in local_emby_users
        if u.EMBYID not in emby_id_set
    ]

    return api_response(
        True,
        "获取成功",
        {
            "emby_users": result,
            "orphans": orphans,
            "total_emby": len(emby_users),
            "total_linked": sum(1 for r in result if r["sync_status"] != "unlinked"),
            "total_orphans": len(orphans),
        },
    )


@admin_bp.route("/emby/cleanup-orphans", methods=["POST"])
@require_auth
@require_admin
async def cleanup_orphan_emby_ids():
    """清理孤儿 EMBYID（本地记录指向已不存在的 Emby 用户），将 EMBYID 置空"""
    emby = get_emby_client()

    try:
        emby_users = await emby.get_users()
    except EmbyError as e:
        return api_response(False, f"无法连接 Emby: {e}", code=500)

    emby_id_set = {eu.id for eu in emby_users}
    local_emby_users = await UserOperate.get_all_emby_users()

    cleaned = []
    for user in local_emby_users:
        if user.EMBYID not in emby_id_set:
            old_emby_id = user.EMBYID
            user.EMBYID = None
            await UserOperate.update_user(user)
            cleaned.append({"uid": user.UID, "username": user.USERNAME, "old_emby_id": old_emby_id})

    return api_response(
        True,
        f"已清理 {len(cleaned)} 条孤儿记录",
        {
            "cleaned": cleaned,
            "count": len(cleaned),
        },
    )


@admin_bp.route("/emby/import-users", methods=["POST"])
@require_auth
@require_admin
async def import_emby_users():
    """
    扫描 Emby 中未绑定本地系统的用户。
    不会自动链接或创建本地用户，仅返回未绑定的 Emby 用户列表。

    Request body (optional): { "emby_ids": ["id1", "id2"] }
    为空则扫描全部未绑定的非管理员用户。
    """
    emby = get_emby_client()

    try:
        emby_users = await emby.get_users()
    except EmbyError as e:
        return api_response(False, f"无法连接 Emby: {e}", code=500)

    data = request.get_json() or {}
    emby_ids = data.get("emby_ids", [])
    if emby_ids and not isinstance(emby_ids, list):
        return api_response(False, "emby_ids 必须为数组", code=400)
    target_ids = {str(i) for i in emby_ids if isinstance(i, (str, int))}

    local_emby_users = await UserOperate.get_all_emby_users()
    linked_emby_ids = {u.EMBYID for u in local_emby_users}

    skipped = []
    unlinked = []

    for eu in emby_users:
        if eu.policy.get("IsAdministrator", False):
            skipped.append({"emby_id": eu.id, "name": eu.name, "reason": "管理员账户"})
            continue
        if target_ids and eu.id not in target_ids:
            skipped.append({"emby_id": eu.id, "name": eu.name, "reason": "未在筛选列表中"})
            continue
        if eu.id in linked_emby_ids:
            skipped.append({"emby_id": eu.id, "name": eu.name, "reason": "已绑定本地用户"})
            continue

        # 不做用户名匹配、不做本地用户创建，仅返回未绑定的 Emby 用户列表
        unlinked.append(
            {
                "emby_id": eu.id,
                "emby_name": eu.name,
                "is_disabled": eu.policy.get("IsDisabled", False),
                "is_hidden": eu.policy.get("IsHidden", False),
            }
        )

    return api_response(
        True,
        f"扫描完成，共 {len(unlinked)} 个未绑定 Emby 用户",
        {
            "unlinked": unlinked,
            "skipped": skipped,
            "unlinked_count": len(unlinked),
            "skipped_count": len(skipped),
        },
    )


@admin_bp.route("/emby/reset-bindings", methods=["POST"])
@require_auth
@require_admin
async def reset_all_emby_bindings():
    """
    重置所有用户的 Emby 绑定（清空所有 EMBYID）。
    ⚠️ 危险操作，用于测试环境重置。不会删除 Emby 端用户。
    Request body: { "confirm": "RESET_ALL_EMBY" }
    """
    data = request.get_json() or {}
    if data.get("confirm") != "RESET_ALL_EMBY":
        return api_response(False, "需要提供确认字符串 confirm='RESET_ALL_EMBY'", code=400)

    local_emby_users = await UserOperate.get_all_emby_users()
    count = 0
    for user in local_emby_users:
        user.EMBYID = None
        await UserOperate.update_user(user)
        count += 1

    return api_response(True, f"已重置 {count} 个用户的 Emby 绑定", {"count": count})


@admin_bp.route("/emby/delete-unlinked", methods=["POST"])
@require_auth
@require_admin
async def delete_unlinked_emby_users():
    """
    删除所有未绑定本地用户的 Emby 用户。
    只删除非管理员账户，默认直接执行。

    Request body:
        {
            "dry_run": false
        }
    """
    data = request.get_json() or {}
    dry_run = parse_bool(data.get("dry_run"), default=False)

    emby = get_emby_client()
    try:
        emby_users = await emby.get_users()
    except EmbyError as e:
        return api_response(False, f"无法连接 Emby: {e}", code=500)

    local_emby_users = await UserOperate.get_all_emby_users()
    linked_emby_ids = {u.EMBYID for u in local_emby_users if u.EMBYID}

    candidates = []
    deleted = []
    failed = []

    for eu in emby_users:
        if eu.policy.get("IsAdministrator", False):
            continue
        if eu.id in linked_emby_ids:
            continue

        record = {
            "emby_id": eu.id,
            "emby_name": eu.name,
            "is_disabled": eu.policy.get("IsDisabled", False),
            "is_hidden": eu.policy.get("IsHidden", False),
        }
        candidates.append(record)

        if not dry_run:
            ok = await emby.delete_user(eu.id)
            if ok:
                deleted.append(record)
            else:
                failed.append({"emby_id": eu.id, "emby_name": eu.name, "reason": "删除失败"})

    return api_response(
        True,
        f"{'预览' if dry_run else '删除'}完成: 共 {len(candidates)} 个未绑定 Emby 用户"
        + (f"，成功删除 {len(deleted)} 个" if not dry_run else ""),
        {
            "candidates": candidates,
            "deleted": deleted,
            "failed": failed,
            "count": len(candidates),
            "dry_run": dry_run,
        },
    )


# ==================== 批量到期时间调控 ====================


@admin_bp.route("/users/bulk-expire", methods=["POST"])
@require_auth
@require_admin
async def admin_bulk_set_expire():
    """一键批量调控用户到期时间（管理员）。

    Body:
        {
            "expired_at": -1,                 // -1 永久；> 0 unix 时间戳（秒）
            "days": 30,                       // 与 expired_at 二选一；正数 = 从现在起 N 天，
                                              //   0/负 视为永久 (= expired_at=-1)
            "filter": {                       // 默认空：覆盖"全部可见用户(普通用户)"
                "role": 1,                    //   选填，对应 Role 枚举值
                "active": true,
                "emby": "bound"               //   "bound" / "unbound"
            },
            "include_admin": false,           // 默认 false，谨防误伤
            "include_whitelist": false,
            "confirm": "BULK_EXPIRE_OK"       // 必填，强制确认串
        }

    Note:
        未绑定 Emby（EMBYID 为空 / PENDING_EMBY=true）的账号一律强制跳过，
        ``EXPIRED_AT=0`` 是"未开通 Emby"的 sentinel，业务上不可被批量覆盖。
        早期接口存在 ``include_pending_emby`` 开关，已下线。

    Response data:
        {
            "matched": <int>, "updated": <int>,
            "expired_at": -1 | <ts>,
            "skipped_pending_emby": <int>,
            "skipped_admins": <int>, "skipped_whitelist": <int>
        }
    """
    from src.core.utils import (
        days_to_seconds,
        rate_limit_check,
        timestamp,
    )

    data = request.get_json(silent=True) or {}
    confirm = (data.get("confirm") or "").strip()
    if confirm != "BULK_EXPIRE_OK":
        return api_response(False, '需要提供 confirm="BULK_EXPIRE_OK" 以确认本次批量操作', code=400)

    # 解析目标到期时间
    expired_at_raw = data.get("expired_at")
    days_raw = data.get("days")
    expired_at: int
    if expired_at_raw is not None:
        try:
            expired_at = int(expired_at_raw)
        except (TypeError, ValueError):
            return api_response(False, "expired_at 必须是整数", code=400)
    elif days_raw is not None:
        try:
            days = int(days_raw)
        except (TypeError, ValueError):
            return api_response(False, "days 必须是整数", code=400)
        if days <= 0:
            expired_at = -1
        else:
            if days > 365 * 100:
                return api_response(False, "days 过大，请直接选择永久", code=400)
            expired_at = timestamp() + days_to_seconds(days)
    else:
        return api_response(False, "必须提供 expired_at 或 days 之一", code=400)

    if expired_at == 0:
        return api_response(False, "expired_at=0 是未开通 sentinel，禁止通过批量接口设置", code=400)
    if expired_at != -1 and expired_at <= 0:
        return api_response(False, "expired_at 必须 > 0 或 = -1", code=400)
    # 上限：不能比 9999-12-31 还远
    if expired_at != -1 and expired_at > 253402214400:
        return api_response(False, "expired_at 超出允许范围", code=400)

    # 操作员速率限制：只防误连点，不阻碍管理员连续调整。
    allowed, retry_after = rate_limit_check(
        "admin_bulk_expire",
        str(g.current_user.UID),
        max_requests=10,
        window_seconds=60,
    )
    if not allowed:
        return api_response(
            False,
            f"批量到期操作过于频繁，请 {retry_after} 秒后再试",
            code=429,
        )

    include_admin = parse_bool(data.get("include_admin"), default=False)
    include_whitelist = parse_bool(data.get("include_whitelist"), default=False)
    # 未绑定 Emby 的账号一律强制跳过（EXPIRED_AT=0 是 sentinel，不允许批量覆盖）。
    # 兼容旧前端：忽略请求里残留的 include_pending_emby。
    include_pending_emby = False

    filt = data.get("filter") or {}
    if not isinstance(filt, dict):
        return api_response(False, "filter 必须是对象", code=400)

    role_filter = filt.get("role")
    active_filter = filt.get("active")
    emby_filter = (filt.get("emby") or "").strip().lower()

    # 角色过滤合法性
    if role_filter is not None:
        try:
            role_filter = int(role_filter)
        except (TypeError, ValueError):
            return api_response(False, "filter.role 必须是整数", code=400)
        if role_filter not in [r.value for r in Role]:
            return api_response(False, "filter.role 非法", code=400)

    # 拉一遍全部满足条件的用户
    has_emby_flag: bool | None = None
    if emby_filter == "bound":
        has_emby_flag = True
    elif emby_filter == "unbound":
        has_emby_flag = False

    all_matching, _total = await UserOperate.get_all_users(
        include_inactive=True,
        role=role_filter,
        active_status=active_filter if isinstance(active_filter, bool) else None,
        has_emby=has_emby_flag,
        limit=100000,
        offset=0,
    )

    # 二次过滤：尊重 include_admin/include_whitelist 默认值，跳过 PENDING_EMBY
    target_uids: list[int] = []
    skipped_admins = 0
    skipped_whitelist = 0
    skipped_pending_emby = 0
    skipped_unrecognized = 0
    for u in all_matching:
        if u.ROLE == Role.UNRECOGNIZED.value:
            skipped_unrecognized += 1
            continue
        if u.ROLE == Role.ADMIN.value and not include_admin:
            skipped_admins += 1
            continue
        if u.ROLE == Role.WHITE_LIST.value and not include_whitelist:
            skipped_whitelist += 1
            continue
        # 当前管理员自己强制保留，避免误把自己锁死
        if u.UID == g.current_user.UID and expired_at != -1:
            skipped_admins += 1
            continue
        # 未绑定 Emby 的账号默认跳过：它们 EXPIRED_AT=0 是 sentinel，不应该被随意覆盖
        if not include_pending_emby and (not u.EMBYID or bool(getattr(u, "PENDING_EMBY", False))):
            skipped_pending_emby += 1
            continue
        target_uids.append(int(u.UID))

    matched = len(target_uids)

    # 上限保护：单次最多 5000 个，超出要求用户更精细地筛选
    if matched > 5000:
        return api_response(
            False,
            f"匹配到 {matched} 个用户，超过单次上限 5000；请收紧筛选条件后重试",
            code=400,
        )

    if not target_uids:
        return api_response(
            True,
            "没有匹配的用户，未做任何更改",
            {
                "matched": 0,
                "updated": 0,
                "expired_at": expired_at,
                "skipped_admins": skipped_admins,
                "skipped_whitelist": skipped_whitelist,
                "skipped_pending_emby": skipped_pending_emby,
                "skipped_unrecognized": skipped_unrecognized,
            },
        )

    try:
        updated = await UserOperate.batch_set_expired_at(target_uids, expired_at)
    except ValueError as exc:
        return api_response(False, str(exc), code=400)

    logger.warning(
        "管理员 %s 批量调控到期时间: matched=%d updated=%d target=%s "
        "filter=%s include_admin=%s include_whitelist=%s include_pending_emby=%s",
        g.current_user.USERNAME,
        matched,
        updated,
        expired_at,
        filt,
        include_admin,
        include_whitelist,
        include_pending_emby,
    )

    return api_response(
        True,
        f"已更新 {updated} 个用户到期时间",
        {
            "matched": matched,
            "updated": updated,
            "expired_at": expired_at,
            "skipped_admins": skipped_admins,
            "skipped_whitelist": skipped_whitelist,
            "skipped_pending_emby": skipped_pending_emby,
            "skipped_unrecognized": skipped_unrecognized,
        },
    )


@admin_bp.route("/users/bulk-enable-disabled", methods=["POST"])
@require_auth
@require_admin
async def admin_bulk_enable_disabled_users():
    """按 UID 列表或当前筛选条件批量启用已禁用账号。"""
    from src.core.utils import rate_limit_check

    data = request.get_json(silent=True) or {}
    if not isinstance(data, dict):
        return api_response(False, "请求体必须是 JSON 对象", code=400)
    confirm = (data.get("confirm") or "").strip()
    if confirm != "BULK_ENABLE_DISABLED_OK":
        return api_response(False, '需要提供 confirm="BULK_ENABLE_DISABLED_OK" 以确认本次批量启用', code=400)

    allowed, retry_after = rate_limit_check(
        "admin_bulk_enable_disabled",
        str(g.current_user.UID),
        max_requests=20,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"批量启用操作过于频繁，请 {retry_after} 秒后再试", code=429)

    include_admin = parse_bool(data.get("include_admin"), default=False)
    include_whitelist = parse_bool(data.get("include_whitelist"), default=False)
    raw_uids = data.get("uids")
    requested_uids: list[int] = []
    users: list[UserModel] = []
    skipped: list[dict] = []

    if raw_uids is not None:
        if not isinstance(raw_uids, list):
            return api_response(False, "uids 必须是数组", code=400)
        seen: set[int] = set()
        for raw_uid in raw_uids:
            try:
                uid = int(raw_uid)
            except (TypeError, ValueError):
                return api_response(False, "uids 中只能包含正整数 UID", code=400)
            if uid <= 0:
                return api_response(False, "uids 中只能包含正整数 UID", code=400)
            if uid in seen:
                continue
            seen.add(uid)
            requested_uids.append(uid)
        if not requested_uids:
            return api_response(False, "请提供至少一个 UID", code=400)
        if len(requested_uids) > 5000:
            return api_response(False, "单次最多处理 5000 个 UID", code=400)
        for uid in requested_uids:
            user = await UserOperate.get_user_by_uid(uid)
            if user:
                users.append(user)
            else:
                skipped.append({"uid": uid, "reason": "用户不存在"})
    else:
        filt = data.get("filter") or {}
        if not isinstance(filt, dict):
            return api_response(False, "filter 必须是对象", code=400)

        role_filter = filt.get("role")
        if role_filter is not None:
            try:
                role_filter = int(role_filter)
            except (TypeError, ValueError):
                return api_response(False, "filter.role 必须是整数", code=400)
            if role_filter not in [r.value for r in Role]:
                return api_response(False, "filter.role 非法", code=400)

        active_filter = filt.get("active")
        if active_filter is not None and not isinstance(active_filter, bool):
            return api_response(False, "filter.active 必须是布尔值", code=400)

        search = (filt.get("search") or "").strip()
        if len(search) > 100:
            return api_response(False, "filter.search 过长", code=400)

        emby_filter = (filt.get("emby") or "").strip().lower()
        has_emby_flag: bool | None = None
        if emby_filter == "bound":
            has_emby_flag = True
        elif emby_filter == "unbound":
            has_emby_flag = False
        elif emby_filter:
            return api_response(False, "filter.emby 只能是 bound 或 unbound", code=400)

        # 与前端筛选对齐：当前列表若明确“仅已启用”，这里不会暗中处理隐藏的禁用账号。
        if active_filter is True:
            users = []
        else:
            users, _ = await UserOperate.get_all_users(
                include_inactive=True,
                role=role_filter,
                active_status=False,
                search=search or None,
                has_emby=has_emby_flag,
                limit=100000,
                offset=0,
            )

    eligible: list[UserModel] = []
    skipped_admins = 0
    skipped_whitelist = 0
    skipped_unrecognized = 0
    skipped_active = 0
    for user in users:
        if user.ROLE == Role.UNRECOGNIZED.value:
            skipped_unrecognized += 1
            skipped.append({"uid": user.UID, "reason": "未识别角色不可批量启用"})
            continue
        if user.ROLE == Role.ADMIN.value and not include_admin:
            skipped_admins += 1
            skipped.append({"uid": user.UID, "reason": "管理员账号默认跳过"})
            continue
        if user.ROLE == Role.WHITE_LIST.value and not include_whitelist:
            skipped_whitelist += 1
            skipped.append({"uid": user.UID, "reason": "白名单账号默认跳过"})
            continue
        if bool(user.ACTIVE_STATUS):
            skipped_active += 1
            skipped.append({"uid": user.UID, "reason": "账号已经启用"})
            continue
        eligible.append(user)

    if len(eligible) > 5000:
        return api_response(
            False,
            f"匹配到 {len(eligible)} 个待启用账号，超过单次上限 5000；请收紧筛选条件后重试",
            code=400,
        )

    enabled: list[dict] = []
    failed: list[dict] = []
    for user in eligible:
        ok, msg = await UserService.enable_user(user)
        if ok:
            enabled.append({"uid": int(user.UID), "username": user.USERNAME})
        else:
            failed.append({"uid": int(user.UID), "username": user.USERNAME, "reason": msg})

    logger.warning(
        "管理员 %s 批量启用禁用账号: matched=%d eligible=%d enabled=%d failed=%d include_admin=%s include_whitelist=%s",
        g.current_user.USERNAME,
        len(users),
        len(eligible),
        len(enabled),
        len(failed),
        include_admin,
        include_whitelist,
    )

    return api_response(
        True,
        f"批量启用完成：成功 {len(enabled)}，失败 {len(failed)}",
        {
            "matched": len(users),
            "eligible": len(eligible),
            "enabled": len(enabled),
            "failed": failed,
            "skipped": skipped,
            "skipped_admins": skipped_admins,
            "skipped_whitelist": skipped_whitelist,
            "skipped_unrecognized": skipped_unrecognized,
            "skipped_active": skipped_active,
            "enabled_users": enabled,
        },
    )


# ==================== 无效用户清理 ====================


@admin_bp.route("/users/cleanup-invalid", methods=["POST"])
@require_auth
@require_admin
async def cleanup_invalid_users():
    """
    清理长期无效用户（既未绑定 TG 也未绑定 Emby 的非管理员/非白名单用户）

    Request:
        {
            "min_days": 7,      // 注册超过多少天仍无绑定则视为无效（默认7）
            "dry_run": false    // 试运行模式，只返回列表不删除（默认false）
        }

    Response:
        {
            "users": [...],     // 匹配的用户列表
            "count": 5,         // 匹配/删除数量
            "dry_run": false
        }
    """
    import time as _time

    data = request.get_json() or {}
    min_days = max(1, data.get("min_days", 7))
    dry_run = data.get("dry_run", False)

    threshold = int(_time.time()) - min_days * 86400

    # 查询所有用户
    all_users, _ = await UserOperate.get_all_users(include_inactive=True, limit=100000, offset=0)

    invalid_users = []
    for u in all_users:
        # 跳过管理员和白名单
        if u.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
            continue
        # 必须同时没有 TG 和 Emby 绑定
        has_tg = bool(u.TELEGRAM_ID)
        has_emby = bool(u.EMBYID)
        if has_tg or has_emby:
            continue
        # 注册时间判定
        reg_time = u.CREATE_AT or u.REGISTER_TIME or 0
        if reg_time > threshold:
            continue  # 注册时间不够久
        invalid_users.append(u)

    result_list = []
    for u in invalid_users:
        result_list.append(
            {
                "uid": u.UID,
                "username": u.USERNAME,
                "role": u.ROLE,
                "active": u.ACTIVE_STATUS,
                "register_time": u.REGISTER_TIME,
                "created_at": u.CREATE_AT or u.REGISTER_TIME,
            }
        )

    deleted_count = 0
    if not dry_run:
        for u in invalid_users:
            try:
                await UserOperate.delete_user(u)
                deleted_count += 1
            except Exception as e:
                logger.warning(f"删除无效用户 {u.USERNAME}(UID:{u.UID}) 失败: {e}")

    action = "预览" if dry_run else "清理"
    return api_response(
        True,
        f"{action}完成: 共 {len(invalid_users)} 个无效用户" + (f"，已删除 {deleted_count} 个" if not dry_run else ""),
        {
            "users": result_list,
            "count": deleted_count if not dry_run else len(invalid_users),
            "dry_run": dry_run,
        },
    )


@admin_bp.route("/users/clear-stale-pending-emby", methods=["POST"])
@require_auth
@require_admin
async def clear_stale_pending_emby_users():
    """清理旧自由注册残留的 Emby 待补建资格。

    仅处理同时满足以下条件的账号：
      - 未绑定 Emby；
      - ``PENDING_EMBY=True``；
      - ``PENDING_EMBY_DAYS`` 为空。

    使用注册码产生的待补建资格会带有 ``PENDING_EMBY_DAYS``，不会被本接口清理。
    """
    from src.core.utils import rate_limit_check

    data = request.get_json(silent=True) or {}
    dry_run = parse_bool(data.get("dry_run"), default=True)

    if not dry_run:
        confirm = (data.get("confirm") or "").strip()
        if confirm != "CLEAR_PENDING_EMBY_OK":
            return api_response(
                False,
                '需要提供 confirm="CLEAR_PENDING_EMBY_OK" 以确认实际清理',
                code=400,
            )
        allowed, retry_after = rate_limit_check(
            "admin_clear_stale_pending_emby",
            str(g.current_user.UID),
            max_requests=10,
            window_seconds=60,
        )
        if not allowed:
            return api_response(False, f"操作过于频繁，请 {retry_after} 秒后再试", code=429)

    all_users, _ = await UserOperate.get_all_users(
        include_inactive=True,
        limit=100000,
        offset=0,
    )

    targets: list[UserModel] = []
    for u in all_users:
        if u.EMBYID:
            continue
        if not bool(getattr(u, "PENDING_EMBY", False)):
            continue
        if getattr(u, "PENDING_EMBY_DAYS", None) is not None:
            continue
        targets.append(u)

    users_view = [
        {
            "uid": int(u.UID),
            "username": u.USERNAME,
            "telegram_id": u.TELEGRAM_ID,
            "register_time": u.REGISTER_TIME,
            "created_at": u.CREATE_AT or u.REGISTER_TIME,
        }
        for u in targets
    ]

    cleared = 0
    failed: list[dict] = []
    if not dry_run:
        for u in targets:
            try:
                u.PENDING_EMBY = False
                u.PENDING_EMBY_DAYS = None
                await UserOperate.update_user(u)
                cleared += 1
            except Exception as exc:
                logger.warning("清理旧 Emby 待补建资格失败 uid=%s username=%s: %s", u.UID, u.USERNAME, exc)
                failed.append({"uid": int(u.UID), "username": u.USERNAME, "error": str(exc)})

    action = "预览" if dry_run else "清理"
    return api_response(
        True,
        f"{action}完成：匹配 {len(targets)} 个旧自由注册残留资格" + (f"，已清理 {cleared} 个" if not dry_run else ""),
        {
            "users": users_view,
            "count": len(targets),
            "cleared": cleared,
            "failed": failed,
            "dry_run": dry_run,
        },
    )


@admin_bp.route("/users/kick-no-emby", methods=["POST"])
@require_auth
@require_admin
async def admin_kick_no_emby_users():
    """一键踢出所有未绑定 Emby 的系统账号。

    与 ``/users/cleanup-invalid`` 的区别：
      - 不要求"同时无 TG 绑定"，只看 ``EMBYID`` 是否为空（兼顾 PENDING_EMBY=True 的待激活账号）。
      - 默认不看注册时间长短，可选传 ``min_days`` 限定"注册超过 N 天"才清理。
      - 管理员 / 白名单 / 未识别角色 强制跳过；操作者自身也强制跳过。
      - 可选 ``preserve_pending_register`` 保留"已绑 TG 但还没补建 Emby"的待激活账号，
        他们在前端 Modal 仍可自助补建（默认沿用 ``EMBY_DIRECT_REGISTER_ENABLED``）。
      - 在 Emby 注册队列里"还没拿到 Emby ID"的 UID 强制排除，避免临界窗口误删。

    Request:
        {
            "dry_run": false,
            "min_days": 0,                          // 0 = 不卡注册时间
            "preserve_pending_register": null,      // null 表示沿用配置默认
            "confirm": "KICK_NO_EMBY_OK"
        }
    """
    import time as _time
    from src.core.utils import rate_limit_check
    from src.config import RegisterConfig
    from src.services.emby_register_queue import EmbyRegisterQueueService

    data = request.get_json(silent=True) or {}
    dry_run = parse_bool(data.get("dry_run"), default=False)

    try:
        min_days = int(data.get("min_days", 0) or 0)
    except (TypeError, ValueError):
        min_days = 0
    min_days = max(0, min(min_days, 3650))

    preserve_raw = data.get("preserve_pending_register")
    if preserve_raw is None:
        preserve_pending_register = bool(RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED)
    else:
        preserve_pending_register = parse_bool(preserve_raw, default=False)

    if not dry_run:
        confirm = (data.get("confirm") or "").strip()
        if confirm != "KICK_NO_EMBY_OK":
            return api_response(
                False,
                '需要提供 confirm="KICK_NO_EMBY_OK" 以确认实际删除',
                code=400,
            )

    if not dry_run:
        allowed, retry_after = rate_limit_check(
            "admin_kick_no_emby",
            str(g.current_user.UID),
            max_requests=10,
            window_seconds=60,
        )
        if not allowed:
            return api_response(
                False,
                f"操作过于频繁，请 {retry_after} 秒后再试",
                code=429,
            )

    try:
        pending_register_uids = EmbyRegisterQueueService.pending_uids()
    except Exception:
        pending_register_uids = set()

    # 拉全量用户后筛 EMBYID 为空 / PENDING_EMBY=True 的
    all_users, _ = await UserOperate.get_all_users(
        include_inactive=True,
        has_emby=False,  # 仓库层直接过滤 EMBYID is None
        limit=100000,
        offset=0,
    )

    threshold_ts = (int(_time.time()) - min_days * 86400) if min_days > 0 else None

    skipped_admins = 0
    skipped_whitelist = 0
    skipped_unrecognized = 0
    skipped_pending_register = 0
    skipped_too_recent = 0
    skipped_in_queue = 0
    candidates: list[UserModel] = []
    for u in all_users:
        if u.UID == g.current_user.UID:
            skipped_admins += 1
            continue
        if u.ROLE == Role.ADMIN.value:
            skipped_admins += 1
            continue
        if u.ROLE == Role.WHITE_LIST.value:
            skipped_whitelist += 1
            continue
        if u.ROLE == Role.UNRECOGNIZED.value:
            skipped_unrecognized += 1
            continue
        # 双重保险：has_emby=False 已经过滤过 EMBYID is None 的，但还可能存在
        # EMBYID 为空字符串等历史脏数据，这里再判一次
        if u.EMBYID:
            continue
        if int(u.UID) in pending_register_uids:
            skipped_in_queue += 1
            continue
        if preserve_pending_register and u.TELEGRAM_ID:
            # 已绑 TG 且 PENDING_EMBY，前端仍可自助补建 → 保留
            skipped_pending_register += 1
            continue
        if threshold_ts is not None:
            reg_time = u.REGISTER_TIME or u.CREATE_AT or 0
            if reg_time > threshold_ts:
                skipped_too_recent += 1
                continue
        candidates.append(u)

    candidate_view = [
        {
            "uid": int(u.UID),
            "username": u.USERNAME,
            "role": u.ROLE,
            "register_time": u.REGISTER_TIME,
            "has_telegram": bool(u.TELEGRAM_ID),
            "pending_emby": bool(getattr(u, "PENDING_EMBY", False)),
        }
        for u in candidates
    ]

    summary_skips = {
        "skipped_admins": skipped_admins,
        "skipped_whitelist": skipped_whitelist,
        "skipped_unrecognized": skipped_unrecognized,
        "skipped_pending_register": skipped_pending_register,
        "skipped_too_recent": skipped_too_recent,
        "skipped_in_queue": skipped_in_queue,
        "min_days": min_days,
        "preserve_pending_register": preserve_pending_register,
    }

    if dry_run:
        return api_response(
            True,
            f"干跑结束：匹配 {len(candidates)} 个未绑 Emby 账号",
            {
                "candidates": candidate_view,
                "candidate_count": len(candidates),
                "deleted_count": 0,
                "failed": [],
                "dry_run": True,
                **summary_skips,
            },
        )

    if not candidates:
        return api_response(
            True,
            "没有需要清理的账号",
            {
                "candidates": [],
                "candidate_count": 0,
                "deleted_count": 0,
                "failed": [],
                "dry_run": False,
                **summary_skips,
            },
        )

    deleted_count = 0
    failed: list[dict] = []
    for u in candidates:
        try:
            success, msg = await UserService.delete_user(u, delete_emby=False)
            if success:
                deleted_count += 1
            else:
                failed.append({"uid": int(u.UID), "username": u.USERNAME, "error": msg})
        except Exception as exc:
            logger.warning(f"踢出未绑 Emby 账号失败: uid={u.UID} username={u.USERNAME}: {exc}")
            failed.append({"uid": int(u.UID), "username": u.USERNAME, "error": str(exc)})

    logger.warning(
        "管理员 %s 一键踢出未绑 Emby 账号: 候选=%d 已删除=%d 失败=%d "
        "(min_days=%d preserve_pending=%s skip admin=%d white=%d unknown=%d pending=%d recent=%d queue=%d)",
        g.current_user.USERNAME,
        len(candidates),
        deleted_count,
        len(failed),
        min_days,
        preserve_pending_register,
        skipped_admins,
        skipped_whitelist,
        skipped_unrecognized,
        skipped_pending_register,
        skipped_too_recent,
        skipped_in_queue,
    )

    return api_response(
        True,
        f"已删除 {deleted_count} 个未绑 Emby 的系统账号",
        {
            "candidates": candidate_view,
            "candidate_count": len(candidates),
            "deleted_count": deleted_count,
            "failed": failed,
            "dry_run": False,
            **summary_skips,
        },
    )


# ==================== 邀请树管理 ====================


@admin_bp.route("/invite/tree", methods=["GET"])
@require_auth
@require_admin
async def admin_invite_tree():
    """返回完整邀请森林：节点 + 边 + 根节点列表，供前端星图渲染。"""
    from src.services import InviteService

    payload = await InviteService.build_forest_view()
    return api_response(True, "获取成功", payload)


@admin_bp.route("/invite/users/<int:uid>/detach", methods=["POST"])
@require_auth
@require_admin
async def admin_detach_user_from_tree(uid: int):
    """让某用户从其上级断开，自身晋升为新树根。"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    from src.db.invite import InviteRelationOperate

    ok = await InviteRelationOperate.detach_child(uid)
    if not ok:
        return api_response(
            True,
            "用户原本就是树根，无需操作",
            {
                "uid": uid,
                "is_root": True,
            },
        )
    return api_response(True, "已断开上级关系", {"uid": uid, "is_root": True})


@admin_bp.route("/invite/codes", methods=["GET"])
@require_auth
@require_admin
async def admin_list_invite_codes():
    """管理员视角：列出指定用户的邀请码（缺省返回全部）。"""
    from src.db.invite import InviteSessionFactory, InviteCodeModel
    from sqlalchemy import select as _select

    inviter_uid = request.args.get("inviter_uid", type=int)
    page = max(1, request.args.get("page", 1, type=int))
    per_page = min(max(1, request.args.get("per_page", 50, type=int)), 200)

    async with InviteSessionFactory() as session:
        q = _select(InviteCodeModel)
        if inviter_uid is not None:
            q = q.where(InviteCodeModel.INVITER_UID == inviter_uid)
        q = q.order_by(InviteCodeModel.CREATED_AT.desc())
        rows = await session.execute(q.offset((page - 1) * per_page).limit(per_page))
        codes = list(rows.scalars().all())

    return api_response(
        True,
        f"共 {len(codes)} 条",
        {
            "codes": [
                {
                    "code": c.CODE,
                    "inviter_uid": c.INVITER_UID,
                    "days": c.DAYS,
                    "use_count_limit": c.USE_COUNT_LIMIT,
                    "use_count": c.USE_COUNT,
                    "expires_at": c.EXPIRES_AT,
                    "active": bool(c.ACTIVE),
                    "used_by_uid": c.USED_BY_UID,
                    "used_at": c.USED_AT,
                    "created_at": c.CREATED_AT,
                    "note": c.NOTE,
                }
                for c in codes
            ],
        },
    )


# ==================== 公告板管理 ====================


@admin_bp.route("/announcements", methods=["GET"])
@require_auth
@require_admin
async def admin_list_announcements():
    """获取公告列表（管理员视角，含历史与隐藏条目）。

    Query:
        page: 页码（默认 1）
        per_page: 每页条数（默认 20，上限 100）
        include_invisible: 是否包含已隐藏（默认 true）
        include_expired: 是否包含已过期（默认 true）
    """
    from src.db.announcement import AnnouncementOperate
    from src.api.v1.announcements import serialize_announcement

    page = request.args.get("page", 1, type=int)
    per_page = request.args.get("per_page", 20, type=int)
    include_invisible = request.args.get("include_invisible", "true").lower() != "false"
    include_expired = request.args.get("include_expired", "true").lower() != "false"

    items, total = await AnnouncementOperate.list_all(
        include_invisible=include_invisible,
        include_expired=include_expired,
        page=page,
        per_page=per_page,
    )
    return api_response(
        True,
        f"共 {total} 条公告",
        {
            "announcements": [serialize_announcement(it, include_internal=True) for it in items],
            "total": total,
            "page": page,
            "per_page": per_page,
            "pages": (total + per_page - 1) // per_page if per_page > 0 else 0,
        },
    )


def _validate_announcement_payload(data: dict, require_content: bool = True) -> tuple[bool, str]:
    title = (data.get("title") or "").strip()
    content = (data.get("content") or "").strip()
    level = (data.get("level") or "info").strip().lower()
    render_mode_raw = data.get("render_mode")
    render_mode = (render_mode_raw or "").strip().lower() if render_mode_raw is not None else ""

    if require_content and not content:
        return False, "公告内容不能为空"
    if title and len(title) > 200:
        return False, "公告标题最多 200 字符"
    if len(content) > 10000:
        return False, "公告内容最多 10000 字符"
    if level and level not in {"info", "notice", "warning", "critical"}:
        return False, "公告级别仅支持 info / notice / warning / critical"
    if render_mode and render_mode not in {"plain", "markdown", "bbcode", "text", "plaintext", "md", "bb"}:
        return False, "公告渲染方式仅支持 plain / markdown / bbcode"
    return True, ""


@admin_bp.route("/announcements", methods=["POST"])
@require_auth
@require_admin
async def admin_create_announcement():
    """创建公告。

    Request:
        {
            "title": "可选标题",
            "content": "公告正文（必填，最多 10000 字符）",
            "level": "info",          // info/notice/warning/critical
            "pinned": false,
            "visible": true,
            "expires_at": -1            // unix 秒；-1 永不过期
        }
    """
    from src.db.announcement import AnnouncementOperate
    from src.api.v1.announcements import serialize_announcement

    data = request.get_json() or {}
    ok, msg = _validate_announcement_payload(data, require_content=True)
    if not ok:
        return api_response(False, msg, code=400)

    expires_at = data.get("expires_at", -1)
    try:
        expires_at = int(expires_at) if expires_at is not None else -1
    except (TypeError, ValueError):
        return api_response(False, "expires_at 必须是整数", code=400)

    item = await AnnouncementOperate.create(
        title=data.get("title"),
        content=data["content"],
        level=data.get("level", "info"),
        render_mode=data.get("render_mode", "plain"),
        pinned=parse_bool(data.get("pinned"), default=False),
        visible=parse_bool(data.get("visible"), default=True),
        expires_at=expires_at,
        created_by_uid=getattr(g.current_user, "UID", None),
    )
    logger.info(f"管理员 {g.current_user.USERNAME} 创建公告 ID={item.ID}")
    return api_response(True, "公告已创建", serialize_announcement(item, include_internal=True))


@admin_bp.route("/announcements/<int:announcement_id>", methods=["PUT"])
@require_auth
@require_admin
async def admin_update_announcement(announcement_id: int):
    """更新公告（部分字段更新）。"""
    from src.db.announcement import AnnouncementOperate
    from src.api.v1.announcements import serialize_announcement

    existing = await AnnouncementOperate.get_by_id(announcement_id)
    if not existing:
        return api_response(False, "公告不存在", code=404)

    data = request.get_json() or {}
    ok, msg = _validate_announcement_payload(data, require_content=False)
    if not ok:
        return api_response(False, msg, code=400)

    expires_at = data.get("expires_at", None)
    if expires_at is not None:
        try:
            expires_at = int(expires_at)
        except (TypeError, ValueError):
            return api_response(False, "expires_at 必须是整数", code=400)

    item = await AnnouncementOperate.update_fields(
        announcement_id=announcement_id,
        title=data.get("title") if "title" in data else None,
        content=data.get("content") if "content" in data else None,
        level=data.get("level") if "level" in data else None,
        render_mode=data.get("render_mode") if "render_mode" in data else None,
        pinned=parse_bool(data.get("pinned"), default=False) if "pinned" in data else None,
        visible=parse_bool(data.get("visible"), default=True) if "visible" in data else None,
        expires_at=expires_at,
    )
    if not item:
        return api_response(False, "公告不存在", code=404)
    logger.info(f"管理员 {g.current_user.USERNAME} 更新公告 ID={announcement_id}")
    return api_response(True, "公告已更新", serialize_announcement(item, include_internal=True))


@admin_bp.route("/announcements/<int:announcement_id>", methods=["DELETE"])
@require_auth
@require_admin
async def admin_delete_announcement(announcement_id: int):
    """删除公告（不可恢复）。"""
    from src.db.announcement import AnnouncementOperate

    ok = await AnnouncementOperate.delete(announcement_id)
    if not ok:
        return api_response(False, "公告不存在", code=404)
    logger.info(f"管理员 {g.current_user.USERNAME} 删除公告 ID={announcement_id}")
    return api_response(True, "公告已删除")


# ==================== 定时任务管理 ====================


@admin_bp.route("/scheduler/jobs", methods=["GET"])
@require_auth
@require_admin
async def admin_list_scheduler_jobs():
    """列出全部内置定时任务及其计划时间、上次运行情况。"""
    from src.services.scheduler_service import SchedulerService

    jobs = await SchedulerService.list_jobs()
    return api_response(
        True,
        "获取成功",
        {
            "jobs": jobs,
        },
    )


@admin_bp.route("/scheduler/jobs/<string:job_id>/run", methods=["POST"])
@require_auth
@require_admin
async def admin_trigger_scheduler_job(job_id: str):
    """立即手动触发一次指定定时任务。任务在后台执行，本接口立即返回。

    Request body 可选携带 ``params``（dict），任务自己识别。例如：
        cleanup_no_emby: ``{"days": 14, "preserve_tg_bound": true, "ignore_enabled_flag": true}``
        kick_unknown_group_members: ``{"dry_run": true, "max_per_run": 50}``
    """
    from src.services.scheduler_service import SchedulerService

    data = request.get_json(silent=True) or {}
    raw_params = data.get("params")
    params = raw_params if isinstance(raw_params, dict) else None
    if params is not None:
        if len(params) > 10:
            return api_response(False, "params 字段过多", code=400)
        allowed_params = {
            "cleanup_no_emby": {"days", "preserve_tg_bound", "ignore_enabled_flag"},
            "kick_unknown_group_members": {"dry_run", "max_per_run"},
        }.get(job_id)
        if allowed_params is None:
            return api_response(False, "该任务不支持自定义参数", code=400)
        unknown = set(params.keys()) - allowed_params
        if unknown:
            return api_response(False, f"不支持的参数: {', '.join(sorted(map(str, unknown)))}", code=400)
        for key, value in params.items():
            if isinstance(value, str) and len(value) > 100:
                return api_response(False, f"参数 {key} 过长", code=400)

    ok, message, record = await SchedulerService.trigger_job(job_id, params=params)
    logger.info(
        f"管理员 {g.current_user.USERNAME} 手动触发定时任务: {job_id} -> " f"ok={ok} message={message} params={params}"
    )
    return api_response(
        ok,
        message,
        {
            "job_id": job_id,
            "last_run": record,
        },
        code=200 if ok else 400,
    )


@admin_bp.route("/scheduler/jobs/<string:job_id>/last-run", methods=["GET"])
@require_auth
@require_admin
async def admin_scheduler_job_last_run(job_id: str):
    """获取指定 job 的最近一次完整运行记录（含日志正文）。"""
    from src.services.scheduler_service import SchedulerService

    detail = await SchedulerService.get_last_run_detail(job_id)
    if not detail:
        return api_response(True, "暂无运行记录", {"job_id": job_id, "last_run": None})
    return api_response(True, "获取成功", {"job_id": job_id, "last_run": detail})


@admin_bp.route("/scheduler/jobs/<string:job_id>/history", methods=["GET"])
@require_auth
@require_admin
async def admin_scheduler_job_history(job_id: str):
    """获取指定 job 的历史运行列表。"""
    from src.services.scheduler_service import SchedulerService

    try:
        limit = int(request.args.get("limit", 20))
    except (TypeError, ValueError):
        limit = 20
    history = await SchedulerService.get_job_history(job_id, limit=limit)
    return api_response(
        True,
        "获取成功",
        {
            "job_id": job_id,
            "history": history,
            "total": len(history),
        },
    )


@admin_bp.route("/scheduler/jobs/<string:job_id>/schedule", methods=["PUT"])
@require_auth
@require_admin
async def admin_set_scheduler_job_schedule(job_id: str):
    """覆盖指定 job 的触发器并实时 reschedule，同时落库。

    Request body:
        {"type": "cron_daily", "hour": 3, "minute": 0}
        {"type": "interval", "seconds": 3600}
    """
    from src.services.scheduler_service import SchedulerService

    data = request.get_json(silent=True) or {}
    trigger_type = (data.get("type") or "").strip()
    if not trigger_type:
        return api_response(False, "缺少 type 字段", code=400)

    hour = data.get("hour")
    minute = data.get("minute")
    seconds = data.get("seconds")

    try:
        hour_value = int(hour) if hour is not None else None
        minute_value = int(minute) if minute is not None else None
        seconds_value = int(seconds) if seconds is not None else None
    except (TypeError, ValueError):
        return api_response(False, "hour、minute、seconds 必须是整数", code=400)

    ok, message, spec = await SchedulerService.set_job_schedule(
        job_id,
        trigger_type=trigger_type,
        hour=hour_value,
        minute=minute_value,
        seconds=seconds_value,
    )
    logger.info(
        f"管理员 {g.current_user.USERNAME} 修改 scheduler 触发器: "
        f"{job_id} -> ok={ok} type={trigger_type} message={message}"
    )
    return api_response(
        ok,
        message,
        {"job_id": job_id, "trigger_spec": spec, "is_custom": True} if ok else None,
        code=200 if ok else 400,
    )


@admin_bp.route("/scheduler/jobs/<string:job_id>/schedule", methods=["DELETE"])
@require_auth
@require_admin
async def admin_reset_scheduler_job_schedule(job_id: str):
    """删除指定 job 的触发器覆盖，恢复到 config.toml 默认值。"""
    from src.services.scheduler_service import SchedulerService

    ok, message, spec = await SchedulerService.reset_job_schedule(job_id)
    logger.info(f"管理员 {g.current_user.USERNAME} 重置 scheduler 触发器: " f"{job_id} -> ok={ok} message={message}")
    return api_response(
        ok,
        message,
        {"job_id": job_id, "trigger_spec": spec, "is_custom": False} if ok else None,
        code=200 if ok else 400,
    )


# ==================== Emby 独立账号 / 强制绑定 ====================


@admin_bp.route("/emby/create-standalone", methods=["POST"])
@require_auth
@require_admin
async def create_standalone_emby_user():
    """创建一个独立的 Emby 用户（不绑定任何系统账号）。

    Request:
        {
            "username": "name",       // 必填，Emby 用户名
            "password": "Pass1234",   // 必填，至少 8 位，含大小写 + 数字
            "email": "u@example.com"  // 可选，仅写入备注
        }
    """
    from src.services import UserService

    data = request.get_json() or {}
    username = (data.get("username") or "").strip()
    password = data.get("password") or ""

    if not username:
        return api_response(False, "缺少 Emby 用户名", code=400)
    if len(username) > 64:
        return api_response(False, "Emby 用户名过长", code=400)

    ok, msg = UserService.validate_password_strength(password, label="Emby 密码")
    if not ok:
        return api_response(False, msg, code=400)

    emby = get_emby_client()
    try:
        existing = await emby.get_user_by_name(username)
    except EmbyError as e:
        return api_response(False, f"无法连接 Emby: {e}", code=502)
    if existing:
        return api_response(False, "该 Emby 用户名已存在", code=409)

    capacity_lock = await UserService.acquire_emby_capacity_lock()
    if capacity_lock is None:
        return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)

    cap_ok, cap_msg = await UserService.check_emby_user_capacity()
    if not cap_ok:
        await UserService.release_emby_capacity_lock(capacity_lock)
        return api_response(False, cap_msg, code=409)

    emby_limit = UserService.get_emby_user_limit()
    if emby_limit > 0:
        try:
            remote_users = await emby.get_users()
        except EmbyError as e:
            await UserService.release_emby_capacity_lock(capacity_lock)
            return api_response(False, f"无法读取 Emby 用户数: {e}", code=502)
        if len(remote_users) >= emby_limit:
            await UserService.release_emby_capacity_lock(capacity_lock)
            return api_response(False, f"Emby 全局用户数已达上限（{len(remote_users)}/{emby_limit}）", code=409)

    try:
        emby_user = await emby.create_user(username, password)
    except EmbyError as e:
        await UserService.release_emby_capacity_lock(capacity_lock)
        logger.error(f"管理员 {g.current_user.USERNAME} 创建独立 Emby 账号失败: {e}")
        return api_response(False, f"创建 Emby 账号失败: {e}", code=502)

    if not emby_user:
        await UserService.release_emby_capacity_lock(capacity_lock)
        return api_response(False, "创建 Emby 账号失败：未返回用户信息", code=502)

    try:
        await EmbyService.apply_default_hidden_libraries_by_emby_id(emby_user.id)
    except Exception as exc:  # pragma: no cover
        logger.warning(f"创建独立 Emby 账号后应用默认隐藏媒体库失败: {exc}")

    logger.info(f"管理员 {g.current_user.USERNAME} 创建独立 Emby 账号: " f"name={emby_user.name}, id={emby_user.id}")
    await UserService.release_emby_capacity_lock(capacity_lock)
    return api_response(
        True,
        "Emby 账号创建成功",
        {
            "emby_id": emby_user.id,
            "emby_username": emby_user.name,
        },
    )


@admin_bp.route("/users/<int:uid>/bind-emby", methods=["POST"])
@require_auth
@require_admin
async def bind_emby_to_user(uid: int):
    """将一个 Emby 用户强制绑定到指定系统账号。

    Request:
        {
            "emby_username": "name",  // 二选一
            "emby_id": "guid",        // 二选一
            "force": false             // 目标 Emby 已被其他系统账号占用时是否夺取
        }
    """
    import json

    data = request.get_json() or {}
    emby_username = (data.get("emby_username") or "").strip()
    emby_id_input = (data.get("emby_id") or "").strip()
    force = parse_bool(data.get("force"), default=False)

    if not emby_username and not emby_id_input:
        return api_response(False, "请提供 emby_username 或 emby_id", code=400)

    target_user = await UserOperate.get_user_by_uid(uid)
    if not target_user:
        return api_response(False, "目标系统账号不存在", code=404)

    emby = get_emby_client()
    try:
        emby_user = None
        if emby_id_input:
            emby_user = await emby.get_user(emby_id_input)
        if emby_user is None and emby_username:
            emby_user = await emby.get_user_by_name(emby_username)
    except EmbyError as e:
        return api_response(False, f"无法连接 Emby: {e}", code=502)

    if emby_user is None:
        return api_response(False, "目标 Emby 用户不存在", code=404)

    capacity_lock = None
    # 已被其他系统账号占用？
    occupant = await UserOperate.get_user_by_embyid(emby_user.id)
    # Emby 绑定上限检查：只在"目标账号还没有 EMBYID 且这次会净增一个绑定"时强制
    # （从占用者那里夺取 → 净增 0；目标已经有 EMBYID → 替换；目标无 EMBYID → 净增 1）
    if not target_user.EMBYID and not getattr(target_user, "PENDING_EMBY", False) and (occupant is None or occupant.UID == target_user.UID):
        capacity_lock = await UserService.acquire_emby_capacity_lock()
        if capacity_lock is None:
            return api_response(False, "Emby 名额检查繁忙，请稍后重试", code=409)
        cap_ok, cap_msg = await UserService.check_emby_user_capacity(
            exclude_uid=target_user.UID,
        )
        if not cap_ok:
            await UserService.release_emby_capacity_lock(capacity_lock)
            return api_response(False, cap_msg, code=409)
    if occupant and occupant.UID != target_user.UID:
        if not force:
            await UserService.release_emby_capacity_lock(capacity_lock)
            # 返回 200 以便前端读取 conflict 详情，由 success=false 表示业务未完成
            return api_response(
                False,
                f"该 Emby 用户已绑定到系统账号 UID={occupant.UID}（{occupant.USERNAME}），需要强制夺取",
                {
                    "conflict": True,
                    "conflict_uid": occupant.UID,
                    "conflict_username": occupant.USERNAME,
                    "emby_id": emby_user.id,
                    "emby_username": emby_user.name,
                },
                code=200,
            )
        # 夺取：清空旧账号绑定，并标记其需要重新绑定
        occupant.EMBYID = None
        occupant.PENDING_EMBY = True
        await UserOperate.update_user(occupant)
        logger.warning(
            f"管理员 {g.current_user.USERNAME} 强制夺取 Emby 绑定: "
            f"emby_id={emby_user.id} 旧UID={occupant.UID} -> 新UID={target_user.UID}"
        )

    # 绑定到目标账号
    target_user.EMBYID = emby_user.id
    target_user.PENDING_EMBY = False
    target_user.PENDING_EMBY_DAYS = None
    # 管理员直接绑定（非系统注册流程）：到期时间默认为永久。
    # 仅当账号尚未拥有真实到期时间（None / 0 sentinel）时覆盖，
    # 避免重新绑定时把之前手工设置的天数误改成永久。
    if target_user.EXPIRED_AT in (None, 0):
        target_user.EXPIRED_AT = -1
    # 把 emby 用户名记入 OTHER，便于后续展示
    other_data = {}
    if target_user.OTHER:
        try:
            other_data = json.loads(target_user.OTHER)
        except (json.JSONDecodeError, TypeError):
            other_data = {}
    other_data["emby_username"] = emby_user.name
    target_user.OTHER = json.dumps(other_data)
    await UserOperate.update_user(target_user)

    # 同步状态到 Emby（按本地启用/到期状态调整 Emby 的 IsDisabled）
    try:
        await UserService.sync_user_to_emby(target_user)
        await EmbyService.apply_default_hidden_libraries(target_user)
    except Exception as exc:
        logger.warning(f"绑定后同步 Emby 状态或应用默认隐藏媒体库失败: {exc}")

    logger.info(
        f"管理员 {g.current_user.USERNAME} 绑定 Emby 到系统账号: "
        f"uid={target_user.UID} emby_id={emby_user.id} emby_name={emby_user.name} force={force}"
    )
    await UserService.release_emby_capacity_lock(capacity_lock)
    return api_response(
        True,
        "绑定成功",
        {
            "uid": target_user.UID,
            "emby_id": emby_user.id,
            "emby_username": emby_user.name,
            "force_taken": bool(occupant and force),
            "previous_uid": occupant.UID if occupant else None,
        },
    )


# ==================== Telegram 群组花名册 / 一键清理未绑账号 ====================


@admin_bp.route("/telegram/roster/stats", methods=["GET"])
@require_auth
@require_admin
async def admin_telegram_roster_stats():
    """返回 Bot 被动观察到的群组花名册概况（配置中的第一个群）。

    Response data:
        {
            "available": true,
            "chat_id": "@xxx",
            "active": 123,        // 状态仍视为「在群」的人数
            "inactive": 4,        // 已退群/被踢的历史记录
            "bots": 1,            // 花名册里的 Bot 数
            "first_seen_at": ..., // 该群最早一次被观察到的时间戳
            "last_seen_at": ...
        }
    """
    from src.config import TelegramConfig
    from src.services.telegram_membership import TelegramMembershipService
    from src.db.telegram_roster import TelegramRosterOperate

    if not TelegramMembershipService.is_bot_available():
        return api_response(
            True,
            "Bot 未就绪",
            {
                "available": False,
                "reason": "bot_unavailable",
            },
        )

    group_ids = TelegramMembershipService.required_group_ids()
    if not group_ids:
        return api_response(
            True,
            "未配置群组",
            {
                "available": False,
                "reason": "no_group_configured",
            },
        )

    chat_id = group_ids[0]
    stats = await TelegramRosterOperate.stats(chat_id)
    stats["available"] = True
    stats["chat_id"] = str(chat_id)
    return api_response(True, "获取成功", stats)


@admin_bp.route("/telegram/rejoined-users/enable", methods=["POST"])
@require_auth
@require_admin
async def enable_rejoined_telegram_users():
    """重新校验并启用已禁用但已回到必需 Telegram 群组的用户。"""
    from src.config import TelegramConfig
    from src.core.utils import rate_limit_check, timestamp
    from src.services.telegram_membership import TelegramMembershipService

    data = request.get_json(silent=True) or {}
    if not isinstance(data, dict):
        return api_response(False, "请求体必须是 JSON 对象", code=400)
    confirm = (data.get("confirm") or "").strip()
    if confirm != "ENABLE_REJOINED_OK":
        return api_response(False, '需要提供 confirm="ENABLE_REJOINED_OK" 以确认恢复启用', code=400)

    if not TelegramMembershipService.enforcement_enabled():
        return api_response(False, "群组成员资格巡检未启用或未配置必需群组", code=400)
    if bool(getattr(TelegramConfig, "BAN_ON_LEAVE", False)):
        return api_response(False, "退群永久封禁模式已开启，禁止批量恢复启用", code=400)
    if not TelegramMembershipService.is_bot_available():
        return api_response(False, "Bot 未就绪或 Telegram 未启用，无法安全校验回群状态", code=400)

    try:
        max_per_run = int(data.get("max_per_run", 500))
    except (TypeError, ValueError):
        max_per_run = 500
    max_per_run = max(1, min(max_per_run, 1000))

    allowed, retry_after = rate_limit_check(
        "admin_enable_rejoined_users",
        str(g.current_user.UID),
        max_requests=20,
        window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"恢复启用操作过于频繁，请 {retry_after} 秒后再试", code=429)

    inactive_users = await UserOperate.get_inactive_telegram_bound_users()
    user_by_tg: dict[int, UserModel] = {}
    invalid_tg = 0
    for user in inactive_users:
        try:
            tg_id = int(user.TELEGRAM_ID)
        except (TypeError, ValueError):
            invalid_tg += 1
            continue
        if tg_id <= 0:
            invalid_tg += 1
            continue
        user_by_tg[tg_id] = user

    missing_map = await TelegramMembershipService.check_users_in_groups(
        list(user_by_tg.keys()),
        strict=True,
    )

    candidates: list[UserModel] = []
    for tg_id, user in user_by_tg.items():
        if missing_map.get(tg_id):
            continue
        candidates.append(user)

    skipped: list[dict] = []
    eligible_candidates: list[UserModel] = []
    now_ts = timestamp()
    for user in candidates:
        exp_raw = getattr(user, "EXPIRED_AT", None)
        if exp_raw not in (None, -1, 0, "-1", "0"):
            try:
                exp_ts = int(exp_raw)
            except (TypeError, ValueError):
                exp_ts = 0
            if exp_ts > 0 and exp_ts < now_ts:
                skipped.append(
                    {"uid": int(user.UID), "username": user.USERNAME, "reason": "账号已过期，不能通过回群按钮启用"}
                )
                continue
        eligible_candidates.append(user)

    limited = len(eligible_candidates) > max_per_run
    to_enable = eligible_candidates[:max_per_run]
    enabled: list[dict] = []
    failed: list[dict] = []
    for user in to_enable:
        if bool(user.ACTIVE_STATUS):
            skipped.append({"uid": int(user.UID), "username": user.USERNAME, "reason": "账号已经启用"})
            continue
        ok, msg = await UserService.enable_user(user)
        if ok:
            enabled.append(
                {
                    "uid": int(user.UID),
                    "username": user.USERNAME,
                    "telegram_id": int(user.TELEGRAM_ID),
                }
            )
        else:
            failed.append({"uid": int(user.UID), "username": user.USERNAME, "reason": msg})

    logger.warning(
        "管理员 %s 一键启用回群用户: scanned=%d candidates=%d enabled=%d failed=%d limited=%s",
        g.current_user.USERNAME,
        len(inactive_users),
        len(candidates),
        len(enabled),
        len(failed),
        limited,
    )
    return api_response(
        True,
        f"回群用户恢复完成：启用 {len(enabled)} 个，失败 {len(failed)} 个",
        {
            "scanned": len(inactive_users),
            "valid_telegram_users": len(user_by_tg),
            "invalid_telegram_id": invalid_tg,
            "candidates": len(candidates),
            "eligible": len(eligible_candidates),
            "enabled": len(enabled),
            "failed": failed,
            "skipped": skipped,
            "enabled_users": enabled,
            "max_per_run": max_per_run,
            "limited": limited,
        },
    )


@admin_bp.route("/telegram/kick-unbound", methods=["POST"])
@require_auth
@require_admin
async def admin_telegram_kick_unbound():
    """一键踢出群里未绑定 Web 账号的成员（仅配置的第一个群）。

    Request body (可选):
        {
            "dry_run": true,    // true 时只统计目标 ID 不真踢；默认 false
            "max_per_run": 200, // 单次最多处理多少个；默认 200，上限 500
            "confirm": "KICK_UNBOUND_OK"  // dry_run=false 时必填
        }

    Response data:
        - 同调度任务 ``kick_unknown_group_members`` 的 summary
        - dry_run=true 时 ``scanned/kicked/skipped`` 全为 0，仅返回 ``targets`` 计数
    """
    from src.core.utils import rate_limit_check
    from src.services.telegram_membership import TelegramMembershipService

    data = request.get_json(silent=True) or {}
    dry_run = parse_bool(data.get("dry_run"), default=False)
    try:
        max_per_run = int(data.get("max_per_run", 200))
    except (TypeError, ValueError):
        max_per_run = 200
    max_per_run = max(1, min(max_per_run, 500))

    if not dry_run:
        confirm = (data.get("confirm") or "").strip()
        if confirm != "KICK_UNBOUND_OK":
            return api_response(
                False,
                '需要提供 confirm="KICK_UNBOUND_OK" 以确认实际踢人',
                code=400,
            )

    if not dry_run:
        allowed, retry_after = rate_limit_check(
            "admin_kick_unbound",
            str(g.current_user.UID),
            max_requests=10,
            window_seconds=60,
        )
        if not allowed:
            return api_response(
                False,
                f"操作过于频繁，请 {retry_after} 秒后再试",
                code=429,
            )

    if not TelegramMembershipService.is_bot_available():
        return api_response(False, "Bot 未就绪或 Telegram 未启用", code=400)

    group_ids = TelegramMembershipService.required_group_ids()
    if not group_ids:
        return api_response(False, "未配置 TelegramConfig.GROUP_ID", code=400)
    chat_id = group_ids[0]

    # Sakura_EmbyBoss 风格：以群组花名册为基准反查 users 表，确定踢人原因
    plan = await TelegramMembershipService.build_unbound_kick_plan(chat_id)
    reasons: dict[int, str] = plan["reasons"]  # type: ignore[assignment]
    targets: list[int] = plan["targets"]  # type: ignore[assignment]
    excluded_ids: set[int] = plan["excluded_ids"]  # type: ignore[assignment]

    reason_counts = {"no_account": 0, "no_emby": 0, "disabled": 0}
    for r in reasons.values():
        reason_counts[r] = reason_counts.get(r, 0) + 1

    summary = {
        "chat_id": str(chat_id),
        "roster_size": plan["roster_size"],
        "bots_in_roster": plan["bots_in_roster"],
        "preserved_bound": plan["preserved_bound"],
        "admins_excluded": len(plan["group_admin_ids"]),  # type: ignore[arg-type]
        "excluded_total": len(excluded_ids),
        "targets": len(targets),
        "reason_no_account": reason_counts["no_account"],
        "reason_no_emby": reason_counts["no_emby"],
        "reason_disabled": reason_counts["disabled"],
        "dry_run": dry_run,
        "max_per_run": max_per_run,
        "kicked": 0,
        "skipped": 0,
        "failed": 0,
        "not_in_group": 0,
        "scanned": 0,
    }

    if dry_run:
        # 返回前 50 个 target + 原因供前端预览
        preview = [{"tg_id": tid, "reason": reasons.get(tid, "unknown")} for tid in targets[:50]]
        summary["preview_targets"] = preview
        return api_response(True, "干跑结束（未实际踢人）", summary)

    if not targets:
        return api_response(True, "没有需要清理的成员", summary)

    result = await TelegramMembershipService.kick_unknown_members(
        chat_id,
        list(targets),
        excluded_ids=set(excluded_ids),
        max_per_run=max_per_run,
    )
    for key in ("scanned", "kicked", "skipped", "failed", "not_in_group"):
        summary[key] = int(result.get(key, 0) or 0)

    logger.warning(
        "管理员 %s 触发一键踢未绑成员: chat=%s targets=%d kicked=%d failed=%d "
        "(no_account=%d no_emby=%d disabled=%d)",
        g.current_user.USERNAME,
        chat_id,
        len(targets),
        summary["kicked"],
        summary["failed"],
        reason_counts["no_account"],
        reason_counts["no_emby"],
        reason_counts["disabled"],
    )
    return api_response(True, f"已踢出 {summary['kicked']} 个未绑账号", summary)
