"""
管理员 API

提供管理员专用的操作接口
"""
import logging
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, require_admin, api_response
from src.db.user import UserOperate, UserModel, Role
from src.db.regcode import RegCodeOperate
from src.services import UserService, EmbyService
from src.services.emby import get_emby_client, EmbyError, EmbyConnectionError

logger = logging.getLogger(__name__)
admin_bp = Blueprint('admin', __name__, url_prefix='/admin')


# ==================== 用户管理 ====================

@admin_bp.route('/users', methods=['GET'])
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
    page = max(1, request.args.get('page', 1, type=int))
    per_page = min(max(1, request.args.get('per_page', 20, type=int)), 100)
    role = request.args.get('role', type=int)
    active = request.args.get('active')
    emby = (request.args.get('emby') or '').strip().lower()
    search = request.args.get('search', '').strip()
    sort_by = (request.args.get('sort') or '').strip() or None

    # 显式三态：true=只看启用，false=只看禁用，省略=全部
    active_status: bool | None = None
    if active is not None:
        if active.lower() == 'true':
            active_status = True
        elif active.lower() == 'false':
            active_status = False

    # Emby 绑定筛选：bound=只看已绑定，unbound=只看未绑定，省略/其它=全部
    has_emby: bool | None = None
    if emby == 'bound':
        has_emby = True
    elif emby == 'unbound':
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

        user_list.append({
            'uid': user.UID,
            'telegram_id': user.TELEGRAM_ID,
            'telegram_username': telegram_username,
            'username': user.USERNAME,
            'email': user.EMAIL,
            'role': user.ROLE,
            'role_name': Role(user.ROLE).name if user.ROLE in [r.value for r in Role] else 'UNKNOWN',
            'active': user.ACTIVE_STATUS,
            'emby_id': user.EMBYID,
            'expired_at': user.EXPIRED_AT,
            'register_time': user.REGISTER_TIME,
            'last_login_time': user.LAST_LOGIN_TIME,
            'bgm_mode': user.BGM_MODE,
        })
    
    return api_response(True, f"共 {len(user_list)} 个用户", {
        'users': user_list,
        'total': total,
        'page': page,
        'per_page': per_page,
        'pages': (total + per_page - 1) // per_page,
    })


@admin_bp.route('/me/update', methods=['PUT'])
@require_auth
@require_admin
async def update_my_info():
    """
    管理员更新自己的信息
    
    Body:
        暂无可更新字段
    """
    return api_response(False, "没有可更新的字段", code=400)


@admin_bp.route('/users/<int:uid>', methods=['GET'])
@require_auth
@require_admin
async def get_user(uid: int):
    """获取用户详情"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    user_info = await UserService.get_user_info(user)
    status = await EmbyService.get_user_status(user)

    user_info['emby_status'] = {
        'is_synced': status.is_synced,
        'is_active': status.is_active,
        'active_sessions': status.active_sessions,
        'message': status.message,
    }

    return api_response(True, "获取成功", user_info)


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
        return False, "用户不存在", {'code': 404}

    if cascade_depth_raw <= 0 or cascade_depth_raw >= 999:
        cascade_depth: int | None = None
    else:
        cascade_depth = max(1, cascade_depth_raw)

    if cascade_depth == 1:
        target_uids = [root_uid]
    else:
        target_uids = await InviteService.collect_uids_to_delete(
            root_uid, cascade_depth if cascade_depth is not None else 9999,
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
            skipped.append({'uid': tid, 'reason': '管理员账户不可被级联启停'})
            continue
        # 已是目标状态：跳过但不算失败
        if bool(target.ACTIVE_STATUS) == enable:
            skipped.append({'uid': tid, 'reason': f"已经处于{'启用' if enable else '禁用'}状态"})
            continue

        if enable:
            ok, msg = await UserService.enable_user(target)
        else:
            ok, msg = await UserService.disable_user(target, reason)

        if ok:
            affected.append(tid)
        else:
            failed.append({'uid': tid, 'reason': msg})

    action = "启用" if enable else "禁用"
    if len(safe_targets) <= 1:
        prefix = f"{action}完成"
    else:
        prefix = f"{action}级联完成"
    cascade_display = "整棵子树" if cascade_depth is None else cascade_depth
    return True, (
        f"{prefix}：成功 {len(affected)}，跳过 {len(skipped)}，失败 {len(failed)}"
    ), {
        'affected': affected,
        'skipped': skipped,
        'failed': failed,
        'cascade_depth': cascade_display,
        'enable': enable,
    }


@admin_bp.route('/users/<int:uid>/disable', methods=['POST'])
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
    reason = data.get('reason', '')
    try:
        cascade_depth_raw = int(data.get('cascade_depth', request.args.get('cascade_depth', 1)))
    except (TypeError, ValueError):
        cascade_depth_raw = 1

    ok, message, payload = await _cascade_toggle_active(
        root_uid=uid,
        enable=False,
        cascade_depth_raw=cascade_depth_raw,
        reason=reason,
    )
    if not ok:
        return api_response(False, message, code=int(payload.get('code', 400)))
    return api_response(True, message, payload)


@admin_bp.route('/users/<int:uid>/enable', methods=['POST'])
@require_auth
@require_admin
async def enable_user(uid: int):
    """启用用户（支持邀请树级联）。

    Request body:
        cascade_depth: int - 同 `/disable`。
    """
    data = request.get_json(silent=True) or {}
    try:
        cascade_depth_raw = int(data.get('cascade_depth', request.args.get('cascade_depth', 1)))
    except (TypeError, ValueError):
        cascade_depth_raw = 1

    ok, message, payload = await _cascade_toggle_active(
        root_uid=uid,
        enable=True,
        cascade_depth_raw=cascade_depth_raw,
    )
    if not ok:
        return api_response(False, message, code=int(payload.get('code', 400)))
    return api_response(True, message, payload)


@admin_bp.route('/users/<int:uid>', methods=['PUT'])
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
    if 'role' in data and data['role'] == Role.ADMIN.value and uid != g.current_user.UID:
        return api_response(False, "不允许将其他用户设置为管理员", code=403)
    
    try:
        # 更新角色
        if 'role' in data:
            role = data['role']
            if role not in [r.value for r in Role]:
                return api_response(False, "无效的角色值", code=400)
            target_user.ROLE = role

        # 更新 Emby ID
        if 'emby_id' in data:
            target_user.EMBYID = data['emby_id'] or None

        # 更新启用状态
        active_changed = False
        new_active: bool = target_user.ACTIVE_STATUS
        if 'active' in data:
            new_active = bool(data['active'])
            active_changed = new_active != target_user.ACTIVE_STATUS
            target_user.ACTIVE_STATUS = new_active

        # 保存到数据库
        await UserOperate.update_user(target_user)

        # 启用/禁用变更时同步 Emby 账户
        emby_sync_msg = ""
        if active_changed and target_user.EMBYID:
            try:
                emby = get_emby_client()
                await emby.set_user_enabled(target_user.EMBYID, new_active)
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


@admin_bp.route('/users/<int:uid>', methods=['DELETE'])
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

    body = request.get_json(silent=True) or {}

    # 优先 mode；老调用方仍可用 delete_emby
    mode = (body.get('mode') or request.args.get('mode') or '').strip().lower()
    if mode not in ('with_emby', 'local_only', 'emby_only'):
        raw_delete = body.get('delete_emby', request.args.get('delete_emby', 'true'))
        mode = 'with_emby' if str(raw_delete).lower() not in ('false', '0', 'no') else 'local_only'

    try:
        cascade_depth_raw = int(body.get('cascade_depth', request.args.get('cascade_depth', 1)))
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
            uid, cascade_depth if cascade_depth is not None else 9999,
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
            skipped.append({'uid': tid, 'reason': '管理员账户不可被级联删除'})
            continue

        if mode == 'emby_only':
            # 仅删 Emby 账号：本地账户与邀请关系完全保留
            if not target.EMBYID:
                skipped.append({'uid': tid, 'reason': '未绑定 Emby 账户'})
                continue
            ok, msg = await UserService.delete_emby_only(target)
        else:
            # local_only / with_emby：都删本地账户 + 清理邀请关系
            try:
                await InviteService.delete_user_keep_subtree(tid)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"清理邀请关系失败 uid={tid}: {exc}")
            ok, msg = await UserService.delete_user(target, delete_emby=(mode == 'with_emby'))

        if ok:
            deleted.append(tid)
        else:
            failed.append({'uid': tid, 'reason': msg})

    cascade_display = "整棵子树" if cascade_depth is None else cascade_depth

    if mode == 'emby_only':
        message_prefix = "Emby 级联删除完成" if len(safe_targets) > 1 else "Emby 删除完成"
    elif mode == 'local_only':
        message_prefix = "本地账户级联删除完成" if len(safe_targets) > 1 else "本地账户删除完成"
    else:
        message_prefix = "级联删除完成" if len(safe_targets) > 1 else "删除完成"

    return api_response(
        len(failed) == 0,
        f"{message_prefix}：成功 {len(deleted)}，跳过 {len(skipped)}，失败 {len(failed)}",
        {
            'deleted': deleted,
            'skipped': skipped,
            'failed': failed,
            'mode': mode,
            'cascade_depth': cascade_display,
        },
    )


@admin_bp.route('/users/<int:uid>/emby', methods=['DELETE'])
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


@admin_bp.route('/users/<int:uid>/renew', methods=['POST'])
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
    days = data.get('days', 30)
    
    if days <= 0:
        return api_response(False, "天数必须大于0", code=400)
    
    success, message = await UserService.renew_user(user, days)
    return api_response(success, message)


@admin_bp.route('/emby/force-set-password', methods=['POST'])
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
    emby_username = (data.get('emby_username') or '').strip()
    new_password = data.get('new_password')

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
    if bool(emby_user.policy.get('IsAdministrator', False)):
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

    return api_response(True, "Emby 密码已重置", {
        'emby_id': emby_user.id,
        'emby_username': emby_user.name,
        'linked_local_user': bool(local),
        'new_password': new_password if auto_generated else new_password,
    })


@admin_bp.route('/users/<int:uid>/reset-password', methods=['POST'])
@require_auth
@require_admin
async def reset_user_password(uid: int):
    """重置用户密码并返回新密码（管理员）。"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)

    # 不允许重置其他管理员密码，降低越权风险
    if user.ROLE == Role.ADMIN.value and user.UID != g.current_user.UID:
        return api_response(False, "不允许重置其他管理员密码", code=403)

    success, message, new_password = await UserService.reset_password(user)
    if not success:
        return api_response(False, message, code=400)

    return api_response(True, message, {
        'new_password': new_password,
    })


@admin_bp.route('/users/<int:uid>/kick', methods=['POST'])
@require_auth
@require_admin
async def kick_user(uid: int):
    """踢出用户所有会话"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    success, kicked = await EmbyService.kick_user_sessions(user)
    
    if success:
        return api_response(True, f"已踢出 {kicked} 个会话", {'kicked_count': kicked})
    return api_response(False, "操作失败")


@admin_bp.route('/users/<int:uid>/libraries', methods=['GET'])
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
        return api_response(True, "用户尚未绑定 Emby", {
            'all_libraries': [],
            'enabled_ids': [],
            'enable_all': False,
            'has_emby': False,
        })

    all_libraries = await EmbyService.get_libraries_info()
    enabled_ids, enable_all = await EmbyService.get_user_library_access(user)

    return api_response(True, "获取成功", {
        'all_libraries': all_libraries,
        'enabled_ids': enabled_ids,
        'enable_all': enable_all,
        'has_emby': True,
    })


@admin_bp.route('/users/<int:uid>/libraries', methods=['PUT'])
@require_auth
@require_admin
async def set_user_libraries(uid: int):
    """
    设置用户媒体库权限
    
    支持按名称或ID设置，优先使用名称。
    
    Request:
        {
            "library_names": ["电影", "电视剧"],   // 按名称（推荐）
            "library_ids": ["id1", "id2"],          // 按ID（兼容）
            "enable_all": false
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    data = request.get_json() or {}
    library_names = data.get('library_names', [])
    library_ids = data.get('library_ids', [])
    enable_all = data.get('enable_all', False)
    
    # 优先使用名称解析
    if library_names:
        resolved_ids, not_found = await EmbyService.resolve_library_names_to_ids(library_names)
        if not_found:
            return api_response(False, f"未找到以下媒体库: {', '.join(not_found)}", code=400)
        library_ids = resolved_ids
    
    success, message = await EmbyService.set_user_library_access(user, library_ids, enable_all)
    return api_response(success, message)




@admin_bp.route('/users/<int:uid>/admin', methods=['PUT'])
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
    
    data = request.get_json() or {}
    is_admin = data.get('is_admin', False)
    
    success, message = await UserService.set_user_admin(user, is_admin)
    return api_response(success, message)


@admin_bp.route('/users/<int:uid>/unbind-telegram', methods=['POST'])
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
    
    if not user.TELEGRAM_ID:
        return api_response(False, "该用户未绑定 Telegram", code=400)
    
    old_telegram_id = user.TELEGRAM_ID
    user.TELEGRAM_ID = None
    await UserOperate.update_user(user)
    
    return api_response(True, f"已解绑 Telegram (原 ID: {old_telegram_id})", {
        'uid': uid,
        'username': user.USERNAME,
        'old_telegram_id': old_telegram_id,
    })


@admin_bp.route('/users/<int:uid>/bind-telegram', methods=['POST'])
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
    
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    
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
    
    return api_response(True, "绑定成功", {
        'uid': uid,
        'username': user.USERNAME,
        'telegram_id': telegram_id,
        'old_telegram_id': old_telegram_id,
    })


@admin_bp.route('/telegram/rebind-requests', methods=['GET'])
@require_auth
@require_admin
async def list_telegram_rebind_requests():
    """获取 Telegram 换绑请求列表"""
    page = request.args.get('page', 1, type=int)
    per_page = min(request.args.get('per_page', 20, type=int), 100)
    status = request.args.get('status')

    requests, total = await UserService.list_telegram_rebind_requests(status=status, page=page, per_page=per_page)
    payload = []
    for req in requests:
        user = await UserOperate.get_user_by_uid(req.UID)
        payload.append({
            'id': req.ID,
            'uid': req.UID,
            'username': user.USERNAME if user else None,
            'old_telegram_id': req.OLD_TELEGRAM_ID,
            'status': req.STATUS,
            'reason': req.REASON,
            'admin_note': req.ADMIN_NOTE,
            'reviewer_uid': req.REVIEWER_UID,
            'created_at': req.CREATED_AT,
            'reviewed_at': req.REVIEWED_AT,
        })

    return api_response(True, "获取成功", {
        'requests': payload,
        'total': total,
    })


@admin_bp.route('/telegram/rebind-requests/<int:request_id>/approve', methods=['POST'])
@require_auth
@require_admin
async def approve_telegram_rebind_request(request_id: int):
    data = request.get_json() or {}
    admin_note = data.get('admin_note')
    success, message = await UserService.approve_telegram_rebind_request(request_id, g.current_user.UID, admin_note)
    return api_response(success, message)


@admin_bp.route('/telegram/rebind-requests/<int:request_id>/reject', methods=['POST'])
@require_auth
@require_admin
async def reject_telegram_rebind_request(request_id: int):
    data = request.get_json() or {}
    admin_note = data.get('admin_note')
    success, message = await UserService.reject_telegram_rebind_request(request_id, g.current_user.UID, admin_note)
    return api_response(success, message)


@admin_bp.route('/users/by-telegram/<int:telegram_id>', methods=['GET'])
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

@admin_bp.route('/emby/sync', methods=['POST'])
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
    return api_response(True, f"同步完成: 成功 {success}, 失败 {failed}", {
        'success': success,
        'failed': failed,
        'errors': errors,
    })

# ==================== 注册码管理 ====================

@admin_bp.route('/regcodes', methods=['GET'])
@require_auth
@require_admin
async def list_regcodes():
    """
    获取注册码列表
    
    Query:
        page: int - 页码（默认 1）
        type: int - 类型筛选 (1=注册, 2=续期, 3=白名单)
        active: bool - 是否只显示有效的注册码
    """
    page = request.args.get('page', 1, type=int)
    code_type = request.args.get('type', type=int)
    active_only = request.args.get('active', 'false').lower() == 'true'
    
    if code_type:
        codes = await RegCodeOperate.get_regcodes_by_type(code_type)
    else:
        codes = await RegCodeOperate.get_all_regcodes()
    
    # 过滤有效的
    if active_only:
        codes = [c for c in codes if c.ACTIVE]
    
    # 分页处理
    per_page = 20
    total = len(codes)
    start = (page - 1) * per_page
    end = start + per_page
    paginated_codes = codes[start:end]
    
    return api_response(True, f"共 {total} 个注册码", {
        'regcodes': [{
            'code': c.CODE,
            'type': c.TYPE,
            'type_name': {1: '注册', 2: '续期', 3: '白名单'}.get(c.TYPE, '未知'),
            'validity_time': c.VALIDITY_TIME,
            'use_count': c.USE_COUNT,
            'use_count_limit': c.USE_COUNT_LIMIT,
            'days': c.DAYS,
            'active': c.ACTIVE,
            'created_time': c.CREATED_TIME,
        } for c in paginated_codes],
        'total': total,
        'page': page,
        'per_page': per_page,
    })


# ==================== 求片管理 ====================

@admin_bp.route('/media-requests', methods=['GET'])
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
    
    page = request.args.get('page', 1, type=int)
    per_page = min(max(request.args.get('per_page', 20, type=int), 1), 100)
    status_filter = request.args.get('status', 'pending').lower()
    
    # 转换状态
    status_map = {
        'pending': ReqStatus.UNHANDLED,
        'accepted': ReqStatus.ACCEPTED,
        'rejected': ReqStatus.REJECTED,
        'completed': ReqStatus.COMPLETED,
    }
    
    target_status = status_map.get(status_filter, ReqStatus.UNHANDLED)
    
    # 获取请求列表
    if status_filter == 'pending':
        # 待处理：获取所有未处理/已接受/下载中的
        requests = await BangumiRequireOperate.get_all_pending_list()
    else:
        # 其他状态：按状态筛选
        requests = await BangumiRequireOperate.get_all_requires_by_status(target_status)
    
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
        if status_name == 'unhandled':
            status_name = 'pending'
            
        # 整合媒体信息
        m_info = other.get('media_info', other) if other else {}
        if not m_info.get('title'):
            m_info['title'] = req.title
        if not m_info.get('season'):
            m_info['season'] = req.season
        if not m_info.get('media_type'):
            m_info['media_type'] = req.media_type
            
        results.append({
            'id': req.id,
            'media_id': getattr(req, 'bangumi_id', getattr(req, 'tmdb_id', None)),
            'source': 'bangumi' if hasattr(req, 'bangumi_id') else 'tmdb',
            'status': status_name,
            'timestamp': req.timestamp,
            'title': req.title,
            'season': req.season,
            'media_type': req.media_type,
            'require_key': req.require_key,
            'admin_note': req.admin_note,
            'media_info': m_info,
            'user': {
                'telegram_id': req.telegram_id,
                'username': user.USERNAME if user else None,
                'uid': user.UID if user else None,
            } if user else None,
        })
    
    # 分页
    total = len(results)
    start = (page - 1) * per_page
    end = start + per_page
    paginated_results = results[start:end]
    
    return api_response(True, "获取成功", {
        'requests': paginated_results,
        'total': total,
        'page': page,
        'per_page': per_page,
    })


@admin_bp.route('/media-requests/<int:request_id>', methods=['PUT', 'DELETE'])
@require_auth
@require_admin
async def update_or_delete_media_request(request_id: int):
    """更新或删除求片请求（管理员）"""
    from src.db.bangumi import BangumiRequireOperate
    
    if request.method == 'DELETE':
        req = await BangumiRequireOperate.get_require(request_id)
        if not req:
            return api_response(False, "请求不存在", code=404)
        source = 'bangumi' if hasattr(req, 'bangumi_id') else 'tmdb'
        success = await BangumiRequireOperate.delete_require(request_id, source)
        return api_response(success, "请求已删除" if success else "删除失败")

    from src.services import MediaRequestService
    from src.db.bangumi import ReqStatus
    
    data = request.get_json() or {}
    status_str = data.get('status', '').lower()
    note = (data.get('note') or '').strip()

    if len(note) > 1000:
        return api_response(False, "管理员备注过长，最多 1000 字符", code=400)
    
    # 转换状态
    status_map = {
        'pending': ReqStatus.UNHANDLED,
        'accepted': ReqStatus.ACCEPTED,
        'rejected': ReqStatus.REJECTED,
        'completed': ReqStatus.COMPLETED,
        'downloading': ReqStatus.DOWNLOADING,
    }
    
    if status_str not in status_map:
        return api_response(False, f"无效状态，支持: {', '.join(status_map.keys())}", code=400)
    
    target_status = status_map[status_str]
    
    # 尝试从 body 获取 source 或通过 ID 自动寻找
    source = data.get('source')
    
    # 更新状态
    success, message = await MediaRequestService.update_request_status(request_id, target_status, note, source)
    
    if success:
        return api_response(True, message or f"状态已更新为 {status_str}")
    else:
        return api_response(False, message or "请求不存在", code=404)


@admin_bp.route('/regcodes', methods=['POST'])
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
        code_type = int(data.get('type', 1))
        validity_time = int(data.get('validity_time', -1))
        use_count_limit = int(data.get('use_count_limit', 1))
        days = int(data.get('days', 30))
        count = int(data.get('count', 1))
    except (TypeError, ValueError):
        return api_response(False, "参数类型错误，请检查 type/validity_time/use_count_limit/days/count", code=400)

    if code_type not in (1, 2, 3):
        return api_response(False, "type 仅支持 1=注册, 2=续期, 3=白名单", code=400)

    # 0 和 -1 都表示永久
    if days <= 0:
        days = -1
    
    if count < 1 or count > 100:
        return api_response(False, "生成数量必须在 1-100 之间", code=400)
    
    codes = await RegCodeOperate.create_regcode(
        validity_time, code_type, use_count_limit, count, days
    )
    
    return api_response(True, "创建成功", {
        'codes': codes if isinstance(codes, list) else [codes],
        'count': count,
    })


@admin_bp.route('/regcodes/<code>', methods=['DELETE'])
@require_auth
@require_admin
async def delete_regcode(code: str):
    """删除注册码"""
    success = await RegCodeOperate.delete_regcode(code)
    
    if success:
        return api_response(True, "删除成功")
    return api_response(False, "注册码不存在或删除失败")


# ==================== Emby 管理 ====================

@admin_bp.route('/emby/sessions', methods=['GET'])
@require_auth
@require_admin
async def list_sessions():
    """获取所有活动会话"""
    sessions = await EmbyService.get_all_sessions()
    return api_response(True, "获取成功", sessions)


@admin_bp.route('/emby/activity', methods=['GET'])
@require_auth
@require_admin
async def get_activity_log():
    """
    获取活动日志
    
    Query:
        limit: int - 返回数量（默认 50，最大 200）
    """
    limit = request.args.get('limit', 50, type=int)
    limit = min(max(limit, 1), 200)
    
    logs = await EmbyService.get_activity_log(limit)
    return api_response(True, "获取成功", logs)


@admin_bp.route('/emby/broadcast', methods=['POST'])
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
    header = data.get('header', '通知')
    text = data.get('text')
    
    if not text:
        return api_response(False, "缺少消息内容", code=400)
    
    sent = await EmbyService.broadcast_message(header, text)
    return api_response(True, f"已发送到 {sent} 个会话", {'sent_count': sent})


# ==================== 白名单用户 ====================

@admin_bp.route('/whitelist', methods=['POST'])
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
    
    telegram_id = data.get('telegram_id')
    username = data.get('username')
    email = data.get('email')
    
    if not telegram_id or not username:
        return api_response(False, "缺少必要参数", code=400)
    
    result = await UserService.create_whitelist_user(telegram_id, username, email)
    
    if result.result.value == 'success':
        return api_response(True, result.message, {
            'username': result.user.USERNAME if result.user else None,
            'password': result.emby_password,
        })
    
    return api_response(False, result.message, code=400)


# ==================== 统计信息 ====================

@admin_bp.route('/stats', methods=['GET'])
@require_auth
@require_admin
async def get_stats():
    """获取系统统计信息"""
    from src.config import RegisterConfig
    
    registered_count = await UserOperate.get_registered_users_count()
    active_count = await UserOperate.get_active_users_count()
    regcode_count = await RegCodeOperate.get_active_regcodes_count()
    server_status = await EmbyService.get_server_status()
    
    return api_response(True, "获取成功", {
        'users': {
            'registered': registered_count,
            'active': active_count,
            'limit': RegisterConfig.USER_LIMIT,
        },
        'regcodes': {
            'active': regcode_count,
        },
        'emby': {
            'online': server_status.get('online', False),
            'active_sessions': server_status.get('active_sessions', 0),
        },
    })


# ==================== Emby 管理 ====================

@admin_bp.route('/emby/test', methods=['POST'])
@require_auth
@require_admin
async def test_emby_connectivity():
    """一键测试 Emby 连通性（网络、认证、用户列表、媒体库）"""
    from src.config import EmbyConfig
    import time as _time

    results = {
        'emby_url': EmbyConfig.EMBY_URL,
        'tests': [],
        'overall': True,
    }
    emby = get_emby_client()

    # Test 1: Ping
    t0 = _time.time()
    try:
        ok = await emby.ping()
        latency = round((_time.time() - t0) * 1000)
        results['tests'].append({
            'name': '网络连通', 'success': ok, 'latency_ms': latency,
            'message': f'延迟 {latency}ms' if ok else '无法连接到 Emby 服务器',
        })
        if not ok:
            results['overall'] = False
    except Exception as e:
        results['tests'].append({'name': '网络连通', 'success': False, 'message': str(e)})
        results['overall'] = False

    # Test 2: Server Info (tests API auth)
    t0 = _time.time()
    try:
        info = await emby.get_server_info()
        latency = round((_time.time() - t0) * 1000)
        results['tests'].append({
            'name': 'API 认证', 'success': True, 'latency_ms': latency,
            'message': f"服务器: {info.get('ServerName', '?')}, 版本: {info.get('Version', '?')}",
        })
        results['server_info'] = {
            'name': info.get('ServerName'),
            'version': info.get('Version'),
            'os': info.get('OperatingSystemDisplayName'),
            'id': info.get('Id'),
        }
    except EmbyError as e:
        results['tests'].append({'name': 'API 认证', 'success': False, 'message': f'认证失败: {e}'})
        results['overall'] = False

    # Test 3: User list
    t0 = _time.time()
    try:
        users = await emby.get_users()
        latency = round((_time.time() - t0) * 1000)
        results['tests'].append({
            'name': '用户列表', 'success': True, 'latency_ms': latency,
            'message': f'共 {len(users)} 个 Emby 用户',
        })
    except EmbyError as e:
        results['tests'].append({'name': '用户列表', 'success': False, 'message': str(e)})
        results['overall'] = False

    # Test 4: Libraries
    t0 = _time.time()
    try:
        libs = await emby.get_libraries()
        latency = round((_time.time() - t0) * 1000)
        results['tests'].append({
            'name': '媒体库', 'success': True, 'latency_ms': latency,
            'message': f'共 {len(libs)} 个媒体库',
        })
    except EmbyError as e:
        results['tests'].append({'name': '媒体库', 'success': False, 'message': str(e)})
        results['overall'] = False

    return api_response(True, "测试完成", results)


@admin_bp.route('/emby/users', methods=['GET'])
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
        sync_status = 'unlinked'
        if local_user:
            sync_status = 'synced' if local_user.USERNAME == eu.name else 'name_mismatch'

        result.append({
            'emby_id': eu.id,
            'emby_name': eu.name,
            'has_password': eu.has_password,
            'is_admin': eu.policy.get('IsAdministrator', False),
            'is_disabled': eu.policy.get('IsDisabled', False),
            'is_hidden': eu.policy.get('IsHidden', False),
            'last_login': eu.last_login_date,
            'last_activity': eu.last_activity_date,
            'local_user': {
                'uid': local_user.UID,
                'username': local_user.USERNAME,
                'telegram_id': local_user.TELEGRAM_ID,
                'active': local_user.ACTIVE_STATUS,
                'role': local_user.ROLE,
            } if local_user else None,
            'sync_status': sync_status,
        })

    # 本地有 EMBYID 但 Emby 端不存在的孤儿记录
    emby_id_set = {eu.id for eu in emby_users}
    orphans = [
        {
            'uid': u.UID, 'username': u.USERNAME,
            'emby_id': u.EMBYID, 'telegram_id': u.TELEGRAM_ID,
        }
        for u in local_emby_users if u.EMBYID not in emby_id_set
    ]

    return api_response(True, "获取成功", {
        'emby_users': result,
        'orphans': orphans,
        'total_emby': len(emby_users),
        'total_linked': sum(1 for r in result if r['sync_status'] != 'unlinked'),
        'total_orphans': len(orphans),
    })


@admin_bp.route('/emby/cleanup-orphans', methods=['POST'])
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
            cleaned.append({'uid': user.UID, 'username': user.USERNAME, 'old_emby_id': old_emby_id})

    return api_response(True, f"已清理 {len(cleaned)} 条孤儿记录", {
        'cleaned': cleaned, 'count': len(cleaned),
    })


@admin_bp.route('/emby/import-users', methods=['POST'])
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
    emby_ids = data.get('emby_ids', [])
    if emby_ids and not isinstance(emby_ids, list):
        return api_response(False, "emby_ids 必须为数组", code=400)
    target_ids = {str(i) for i in emby_ids if isinstance(i, (str, int))}

    local_emby_users = await UserOperate.get_all_emby_users()
    linked_emby_ids = {u.EMBYID for u in local_emby_users}

    skipped = []
    unlinked = []

    for eu in emby_users:
        if eu.policy.get('IsAdministrator', False):
            skipped.append({'emby_id': eu.id, 'name': eu.name, 'reason': '管理员账户'})
            continue
        if target_ids and eu.id not in target_ids:
            skipped.append({'emby_id': eu.id, 'name': eu.name, 'reason': '未在筛选列表中'})
            continue
        if eu.id in linked_emby_ids:
            skipped.append({'emby_id': eu.id, 'name': eu.name, 'reason': '已绑定本地用户'})
            continue

        # 不做用户名匹配、不做本地用户创建，仅返回未绑定的 Emby 用户列表
        unlinked.append({'emby_id': eu.id, 'emby_name': eu.name, 'is_disabled': eu.policy.get('IsDisabled', False), 'is_hidden': eu.policy.get('IsHidden', False)})

    return api_response(True, f"扫描完成，共 {len(unlinked)} 个未绑定 Emby 用户", {
        'unlinked': unlinked,
        'skipped': skipped,
        'unlinked_count': len(unlinked),
        'skipped_count': len(skipped),
    })


@admin_bp.route('/emby/reset-bindings', methods=['POST'])
@require_auth
@require_admin
async def reset_all_emby_bindings():
    """
    重置所有用户的 Emby 绑定（清空所有 EMBYID）。
    ⚠️ 危险操作，用于测试环境重置。不会删除 Emby 端用户。
    Request body: { "confirm": "RESET_ALL_EMBY" }
    """
    data = request.get_json() or {}
    if data.get('confirm') != 'RESET_ALL_EMBY':
        return api_response(False, "需要提供确认字符串 confirm='RESET_ALL_EMBY'", code=400)

    local_emby_users = await UserOperate.get_all_emby_users()
    count = 0
    for user in local_emby_users:
        user.EMBYID = None
        await UserOperate.update_user(user)
        count += 1

    return api_response(True, f"已重置 {count} 个用户的 Emby 绑定", {'count': count})


@admin_bp.route('/emby/delete-unlinked', methods=['POST'])
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
    dry_run = bool(data.get('dry_run', False))

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
        if eu.policy.get('IsAdministrator', False):
            continue
        if eu.id in linked_emby_ids:
            continue

        record = {
            'emby_id': eu.id,
            'emby_name': eu.name,
            'is_disabled': eu.policy.get('IsDisabled', False),
            'is_hidden': eu.policy.get('IsHidden', False),
        }
        candidates.append(record)

        if not dry_run:
            ok = await emby.delete_user(eu.id)
            if ok:
                deleted.append(record)
            else:
                failed.append({'emby_id': eu.id, 'emby_name': eu.name, 'reason': '删除失败'})

    return api_response(True, f"{'预览' if dry_run else '删除'}完成: 共 {len(candidates)} 个未绑定 Emby 用户" + (f"，成功删除 {len(deleted)} 个" if not dry_run else ''), {
        'candidates': candidates,
        'deleted': deleted,
        'failed': failed,
        'count': len(candidates),
        'dry_run': dry_run,
    })


# ==================== 无效用户清理 ====================

@admin_bp.route('/users/cleanup-invalid', methods=['POST'])
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
    min_days = max(1, data.get('min_days', 7))
    dry_run = data.get('dry_run', False)

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
        reg_time = u.REGISTER_TIME or u.CREATE_AT or 0
        if reg_time > threshold:
            continue  # 注册时间不够久
        invalid_users.append(u)

    result_list = []
    for u in invalid_users:
        result_list.append({
            'uid': u.UID,
            'username': u.USERNAME,
            'role': u.ROLE,
            'active': u.ACTIVE_STATUS,
            'register_time': u.REGISTER_TIME,
        })

    deleted_count = 0
    if not dry_run:
        for u in invalid_users:
            try:
                await UserOperate.delete_user(u)
                deleted_count += 1
            except Exception as e:
                logger.warning(f"删除无效用户 {u.USERNAME}(UID:{u.UID}) 失败: {e}")

    action = "预览" if dry_run else "清理"
    return api_response(True, f"{action}完成: 共 {len(invalid_users)} 个无效用户" + (f"，已删除 {deleted_count} 个" if not dry_run else ""), {
        'users': result_list,
        'count': deleted_count if not dry_run else len(invalid_users),
        'dry_run': dry_run,
    })


# ==================== 邀请树管理 ====================


@admin_bp.route('/invite/tree', methods=['GET'])
@require_auth
@require_admin
async def admin_invite_tree():
    """返回完整邀请森林：节点 + 边 + 根节点列表，供前端星图渲染。"""
    from src.services import InviteService
    payload = await InviteService.build_forest_view()
    return api_response(True, "获取成功", payload)


@admin_bp.route('/invite/users/<int:uid>/detach', methods=['POST'])
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
        return api_response(True, "用户原本就是树根，无需操作", {
            'uid': uid,
            'is_root': True,
        })
    return api_response(True, "已断开上级关系", {'uid': uid, 'is_root': True})


@admin_bp.route('/invite/codes', methods=['GET'])
@require_auth
@require_admin
async def admin_list_invite_codes():
    """管理员视角：列出指定用户的邀请码（缺省返回全部）。"""
    from src.db.invite import InviteSessionFactory, InviteCodeModel
    from sqlalchemy import select as _select

    inviter_uid = request.args.get('inviter_uid', type=int)
    page = max(1, request.args.get('page', 1, type=int))
    per_page = min(max(1, request.args.get('per_page', 50, type=int)), 200)

    async with InviteSessionFactory() as session:
        q = _select(InviteCodeModel)
        if inviter_uid is not None:
            q = q.where(InviteCodeModel.INVITER_UID == inviter_uid)
        q = q.order_by(InviteCodeModel.CREATED_AT.desc())
        rows = await session.execute(q.offset((page - 1) * per_page).limit(per_page))
        codes = list(rows.scalars().all())

    return api_response(True, f"共 {len(codes)} 条", {
        'codes': [
            {
                'code': c.CODE,
                'inviter_uid': c.INVITER_UID,
                'days': c.DAYS,
                'use_count_limit': c.USE_COUNT_LIMIT,
                'use_count': c.USE_COUNT,
                'expires_at': c.EXPIRES_AT,
                'active': bool(c.ACTIVE),
                'used_by_uid': c.USED_BY_UID,
                'used_at': c.USED_AT,
                'created_at': c.CREATED_AT,
                'note': c.NOTE,
            }
            for c in codes
        ],
    })


# ==================== 公告板管理 ====================

@admin_bp.route('/announcements', methods=['GET'])
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

    page = request.args.get('page', 1, type=int)
    per_page = request.args.get('per_page', 20, type=int)
    include_invisible = (request.args.get('include_invisible', 'true').lower() != 'false')
    include_expired = (request.args.get('include_expired', 'true').lower() != 'false')

    items, total = await AnnouncementOperate.list_all(
        include_invisible=include_invisible,
        include_expired=include_expired,
        page=page,
        per_page=per_page,
    )
    return api_response(True, f"共 {total} 条公告", {
        'announcements': [serialize_announcement(it, include_internal=True) for it in items],
        'total': total,
        'page': page,
        'per_page': per_page,
        'pages': (total + per_page - 1) // per_page if per_page > 0 else 0,
    })


def _validate_announcement_payload(data: dict, require_content: bool = True) -> tuple[bool, str]:
    title = (data.get('title') or '').strip()
    content = (data.get('content') or '').strip()
    level = (data.get('level') or 'info').strip().lower()
    render_mode_raw = data.get('render_mode')
    render_mode = (render_mode_raw or '').strip().lower() if render_mode_raw is not None else ''

    if require_content and not content:
        return False, "公告内容不能为空"
    if title and len(title) > 200:
        return False, "公告标题最多 200 字符"
    if len(content) > 10000:
        return False, "公告内容最多 10000 字符"
    if level and level not in {'info', 'notice', 'warning', 'critical'}:
        return False, "公告级别仅支持 info / notice / warning / critical"
    if render_mode and render_mode not in {'plain', 'markdown', 'bbcode', 'text', 'plaintext', 'md', 'bb'}:
        return False, "公告渲染方式仅支持 plain / markdown / bbcode"
    return True, ""


@admin_bp.route('/announcements', methods=['POST'])
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

    expires_at = data.get('expires_at', -1)
    try:
        expires_at = int(expires_at) if expires_at is not None else -1
    except (TypeError, ValueError):
        return api_response(False, "expires_at 必须是整数", code=400)

    item = await AnnouncementOperate.create(
        title=data.get('title'),
        content=data['content'],
        level=data.get('level', 'info'),
        render_mode=data.get('render_mode', 'plain'),
        pinned=bool(data.get('pinned', False)),
        visible=bool(data.get('visible', True)),
        expires_at=expires_at,
        created_by_uid=getattr(g.current_user, 'UID', None),
    )
    logger.info(f"管理员 {g.current_user.USERNAME} 创建公告 ID={item.ID}")
    return api_response(True, "公告已创建", serialize_announcement(item, include_internal=True))


@admin_bp.route('/announcements/<int:announcement_id>', methods=['PUT'])
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

    expires_at = data.get('expires_at', None)
    if expires_at is not None:
        try:
            expires_at = int(expires_at)
        except (TypeError, ValueError):
            return api_response(False, "expires_at 必须是整数", code=400)

    item = await AnnouncementOperate.update_fields(
        announcement_id=announcement_id,
        title=data.get('title') if 'title' in data else None,
        content=data.get('content') if 'content' in data else None,
        level=data.get('level') if 'level' in data else None,
        render_mode=data.get('render_mode') if 'render_mode' in data else None,
        pinned=data.get('pinned') if 'pinned' in data else None,
        visible=data.get('visible') if 'visible' in data else None,
        expires_at=expires_at,
    )
    if not item:
        return api_response(False, "公告不存在", code=404)
    logger.info(f"管理员 {g.current_user.USERNAME} 更新公告 ID={announcement_id}")
    return api_response(True, "公告已更新", serialize_announcement(item, include_internal=True))


@admin_bp.route('/announcements/<int:announcement_id>', methods=['DELETE'])
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

@admin_bp.route('/scheduler/jobs', methods=['GET'])
@require_auth
@require_admin
async def admin_list_scheduler_jobs():
    """列出全部内置定时任务及其计划时间、上次运行情况。"""
    from src.services.scheduler_service import SchedulerService
    jobs = await SchedulerService.list_jobs()
    return api_response(True, "获取成功", {
        'jobs': jobs,
    })


@admin_bp.route('/scheduler/jobs/<string:job_id>/run', methods=['POST'])
@require_auth
@require_admin
async def admin_trigger_scheduler_job(job_id: str):
    """立即手动触发一次指定定时任务。任务在后台执行，本接口立即返回。"""
    from src.services.scheduler_service import SchedulerService

    ok, message, record = await SchedulerService.trigger_job(job_id)
    logger.info(
        f"管理员 {g.current_user.USERNAME} 手动触发定时任务: {job_id} -> ok={ok} message={message}"
    )
    return api_response(ok, message, {
        'job_id': job_id,
        'last_run': record,
    }, code=200 if ok else 400)


@admin_bp.route('/scheduler/jobs/<string:job_id>/last-run', methods=['GET'])
@require_auth
@require_admin
async def admin_scheduler_job_last_run(job_id: str):
    """获取指定 job 的最近一次完整运行记录（含日志正文）。"""
    from src.services.scheduler_service import SchedulerService
    detail = await SchedulerService.get_last_run_detail(job_id)
    if not detail:
        return api_response(True, "暂无运行记录", {'job_id': job_id, 'last_run': None})
    return api_response(True, "获取成功", {'job_id': job_id, 'last_run': detail})


@admin_bp.route('/scheduler/jobs/<string:job_id>/history', methods=['GET'])
@require_auth
@require_admin
async def admin_scheduler_job_history(job_id: str):
    """获取指定 job 的历史运行列表。"""
    from src.services.scheduler_service import SchedulerService
    try:
        limit = int(request.args.get('limit', 20))
    except (TypeError, ValueError):
        limit = 20
    history = await SchedulerService.get_job_history(job_id, limit=limit)
    return api_response(True, "获取成功", {
        'job_id': job_id,
        'history': history,
        'total': len(history),
    })


@admin_bp.route('/scheduler/jobs/<string:job_id>/schedule', methods=['PUT'])
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
    trigger_type = (data.get('type') or '').strip()
    if not trigger_type:
        return api_response(False, "缺少 type 字段", code=400)

    hour = data.get('hour')
    minute = data.get('minute')
    seconds = data.get('seconds')

    ok, message, spec = await SchedulerService.set_job_schedule(
        job_id,
        trigger_type=trigger_type,
        hour=int(hour) if hour is not None else None,
        minute=int(minute) if minute is not None else None,
        seconds=int(seconds) if seconds is not None else None,
    )
    logger.info(
        f"管理员 {g.current_user.USERNAME} 修改 scheduler 触发器: "
        f"{job_id} -> ok={ok} type={trigger_type} message={message}"
    )
    return api_response(
        ok, message,
        {'job_id': job_id, 'trigger_spec': spec, 'is_custom': True} if ok else None,
        code=200 if ok else 400,
    )


@admin_bp.route('/scheduler/jobs/<string:job_id>/schedule', methods=['DELETE'])
@require_auth
@require_admin
async def admin_reset_scheduler_job_schedule(job_id: str):
    """删除指定 job 的触发器覆盖，恢复到 config.toml 默认值。"""
    from src.services.scheduler_service import SchedulerService

    ok, message, spec = await SchedulerService.reset_job_schedule(job_id)
    logger.info(
        f"管理员 {g.current_user.USERNAME} 重置 scheduler 触发器: "
        f"{job_id} -> ok={ok} message={message}"
    )
    return api_response(
        ok, message,
        {'job_id': job_id, 'trigger_spec': spec, 'is_custom': False} if ok else None,
        code=200 if ok else 400,
    )


# ==================== Emby 独立账号 / 强制绑定 ====================

@admin_bp.route('/emby/create-standalone', methods=['POST'])
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
    username = (data.get('username') or '').strip()
    password = data.get('password') or ''

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

    try:
        emby_user = await emby.create_user(username, password)
    except EmbyError as e:
        logger.error(f"管理员 {g.current_user.USERNAME} 创建独立 Emby 账号失败: {e}")
        return api_response(False, f"创建 Emby 账号失败: {e}", code=502)

    if not emby_user:
        return api_response(False, "创建 Emby 账号失败：未返回用户信息", code=502)

    logger.info(
        f"管理员 {g.current_user.USERNAME} 创建独立 Emby 账号: "
        f"name={emby_user.name}, id={emby_user.id}"
    )
    return api_response(True, "Emby 账号创建成功", {
        'emby_id': emby_user.id,
        'emby_username': emby_user.name,
    })


@admin_bp.route('/users/<int:uid>/bind-emby', methods=['POST'])
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
    emby_username = (data.get('emby_username') or '').strip()
    emby_id_input = (data.get('emby_id') or '').strip()
    force = bool(data.get('force', False))

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

    # 已被其他系统账号占用？
    occupant = await UserOperate.get_user_by_embyid(emby_user.id)
    if occupant and occupant.UID != target_user.UID:
        if not force:
            # 返回 200 以便前端读取 conflict 详情，由 success=false 表示业务未完成
            return api_response(
                False,
                f"该 Emby 用户已绑定到系统账号 UID={occupant.UID}（{occupant.USERNAME}），需要强制夺取",
                {
                    'conflict': True,
                    'conflict_uid': occupant.UID,
                    'conflict_username': occupant.USERNAME,
                    'emby_id': emby_user.id,
                    'emby_username': emby_user.name,
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
    # 把 emby 用户名记入 OTHER，便于后续展示
    other_data = {}
    if target_user.OTHER:
        try:
            other_data = json.loads(target_user.OTHER)
        except (json.JSONDecodeError, TypeError):
            other_data = {}
    other_data['emby_username'] = emby_user.name
    target_user.OTHER = json.dumps(other_data)
    await UserOperate.update_user(target_user)

    # 同步状态到 Emby（按本地启用/到期状态调整 Emby 的 IsDisabled）
    try:
        from src.services import UserService
        await UserService.sync_user_to_emby(target_user)
    except Exception as exc:
        logger.warning(f"绑定后同步 Emby 状态失败: {exc}")

    logger.info(
        f"管理员 {g.current_user.USERNAME} 绑定 Emby 到系统账号: "
        f"uid={target_user.UID} emby_id={emby_user.id} emby_name={emby_user.name} force={force}"
    )
    return api_response(True, "绑定成功", {
        'uid': target_user.UID,
        'emby_id': emby_user.id,
        'emby_username': emby_user.name,
        'force_taken': bool(occupant and force),
        'previous_uid': occupant.UID if occupant else None,
    })

