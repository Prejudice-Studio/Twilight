"""邀请树 API

提供：
- ``POST /invite/codes``：当前用户生成 Emby 邀请码（受配置开关与层级上限约束）
- ``GET  /invite/codes``：列出我生成的邀请码及使用情况
- ``DELETE /invite/codes/<code>``：撤销/删除我生成的邀请码（未使用的）
- ``GET  /invite/me``：返回我的上级、下级、当前层级与可邀请状态
- ``POST /invite/use``：当前已登录用户使用邀请码注册 Emby（无 Emby 账号时）

管理员相关接口在 ``admin.py`` 里以 ``/admin/invite/*`` 暴露。
"""
from __future__ import annotations

import logging
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, api_response
from src.config import RegisterConfig
from src.core.utils import (
    is_valid_username,
    timestamp,
    days_to_seconds,
    hash_password,
)
from src.db.invite import InviteCodeOperate, InviteRelationOperate
from src.db.user import UserOperate, Role
from src.services import InviteService, UserService
from src.services.emby import get_emby_client, EmbyError

logger = logging.getLogger(__name__)

invite_bp = Blueprint('invite', __name__, url_prefix='/invite')


def _serialize_code(model) -> dict:
    return {
        'code': model.CODE,
        'inviter_uid': model.INVITER_UID,
        'days': model.DAYS,
        'use_count_limit': model.USE_COUNT_LIMIT,
        'use_count': model.USE_COUNT,
        'expires_at': model.EXPIRES_AT,
        'active': bool(model.ACTIVE),
        'created_at': model.CREATED_AT,
        'used_by_uid': model.USED_BY_UID,
        'used_at': model.USED_AT,
        'note': model.NOTE,
    }


def _require_invite_enabled():
    if not InviteService.is_enabled():
        return api_response(False, "邀请系统未启用", code=403)
    return None


@invite_bp.route('/config', methods=['GET'])
async def get_invite_config():
    """公开配置：前端登录页 / 注册页可读，用于判断是否展示「使用邀请码」入口。"""
    return api_response(True, "获取成功", {
        'enabled': InviteService.is_enabled(),
        'max_depth': InviteService.max_depth(),
        'invite_limit': RegisterConfig.INVITE_LIMIT,
        'require_emby': bool(RegisterConfig.INVITE_REQUIRE_EMBY),
        'default_days': int(RegisterConfig.INVITE_CODE_DEFAULT_DAYS or 30),
    })


@invite_bp.route('/me', methods=['GET'])
@require_auth
async def get_my_invite_status():
    blocked = _require_invite_enabled()
    if blocked:
        return blocked

    user = g.current_user
    ancestor_depth = await InviteService.get_ancestor_depth(user.UID)
    parent_uid = await InviteRelationOperate.get_parent_uid(user.UID)
    children = await InviteRelationOperate.get_children(user.UID)
    can_invite, reason = await InviteService.ensure_can_invite(user)

    parent_info = None
    if parent_uid:
        parent_user = await UserOperate.get_user_by_uid(parent_uid)
        if parent_user:
            parent_info = {
                'uid': parent_user.UID,
                'username': parent_user.USERNAME,
            }

    children_info = []
    for child_uid in children:
        child = await UserOperate.get_user_by_uid(child_uid)
        if child:
            children_info.append({
                'uid': child.UID,
                'username': child.USERNAME,
                'active': bool(child.ACTIVE_STATUS),
                'has_emby': bool(child.EMBYID),
            })

    return api_response(True, "获取成功", {
        'enabled': True,
        'is_root': parent_uid is None,
        'parent': parent_info,
        'children': children_info,
        'depth': ancestor_depth,
        'max_depth': InviteService.max_depth(),
        'can_invite': can_invite,
        'invite_block_reason': '' if can_invite else reason,
    })


@invite_bp.route('/codes', methods=['POST'])
@require_auth
async def generate_invite_code():
    blocked = _require_invite_enabled()
    if blocked:
        return blocked

    user = g.current_user
    ok, msg = await InviteService.ensure_can_invite(user)
    if not ok:
        return api_response(False, msg, code=403)

    data = request.get_json(silent=True) or {}
    raw_days = data.get('days', RegisterConfig.INVITE_CODE_DEFAULT_DAYS)
    try:
        days = int(raw_days)
    except (TypeError, ValueError):
        return api_response(False, "days 必须是整数", code=400)
    if days < -1 or days == 0:
        days = -1 if days <= 0 else days
    # 限定一个合理上限，防止刷
    if days > 3650:
        return api_response(False, "天数过大", code=400)

    expires_at = data.get('expires_at', -1)
    try:
        expires_at = int(expires_at) if expires_at is not None else -1
    except (TypeError, ValueError):
        return api_response(False, "expires_at 必须是整数", code=400)

    note = (data.get('note') or '').strip()
    if len(note) > 255:
        return api_response(False, "备注过长，最多 255 字符", code=400)

    code = await InviteCodeOperate.create_code(
        inviter_uid=user.UID,
        days=days,
        use_count_limit=1,
        expires_at=expires_at,
        note=note or None,
    )
    logger.info(f"用户 {user.USERNAME} 生成邀请码 {code.CODE}")
    return api_response(True, "邀请码已生成", _serialize_code(code))


@invite_bp.route('/codes', methods=['GET'])
@require_auth
async def list_my_invite_codes():
    blocked = _require_invite_enabled()
    if blocked:
        return blocked
    codes = await InviteCodeOperate.list_by_inviter(g.current_user.UID)
    return api_response(True, "获取成功", {
        'codes': [_serialize_code(c) for c in codes],
        'total': len(codes),
    })


@invite_bp.route('/codes/<code>', methods=['DELETE'])
@require_auth
async def revoke_invite_code(code: str):
    blocked = _require_invite_enabled()
    if blocked:
        return blocked
    existing = await InviteCodeOperate.get_code(code)
    if not existing or existing.INVITER_UID != g.current_user.UID:
        return api_response(False, "邀请码不存在或无权操作", code=404)
    if existing.USE_COUNT > 0:
        # 已使用的邀请码只允许停用，不能物理删除（保留可追溯性）
        await InviteCodeOperate.deactivate(code, inviter_uid=g.current_user.UID)
        return api_response(True, "邀请码已停用（已被使用，无法删除）")
    ok = await InviteCodeOperate.delete(code, inviter_uid=g.current_user.UID)
    return api_response(ok, "邀请码已删除" if ok else "删除失败", code=200 if ok else 400)


@invite_bp.route('/check', methods=['POST'])
async def check_invite_code():
    """无需登录：检查邀请码是否可用。供注册页前置校验。"""
    blocked = _require_invite_enabled()
    if blocked:
        return blocked

    from src.core.utils import rate_limit_check
    client_ip = request.remote_addr or 'unknown'
    allowed, retry_after = rate_limit_check(
        'invite_check', client_ip, max_requests=10, window_seconds=60,
    )
    if not allowed:
        return api_response(False, f"操作过于频繁，请在 {retry_after} 秒后重试", code=429)

    data = request.get_json(silent=True) or {}
    code = (data.get('code') or '').strip()
    if not code:
        return api_response(False, "缺少邀请码", code=400)
    info = await InviteCodeOperate.get_code(code)
    if not info or not info.ACTIVE:
        return api_response(False, "邀请码无效或已停用", code=404)
    if info.USE_COUNT_LIMIT != -1 and info.USE_COUNT >= info.USE_COUNT_LIMIT:
        return api_response(False, "邀请码已被使用", code=400)
    if info.EXPIRES_AT != -1 and timestamp() > info.EXPIRES_AT:
        return api_response(False, "邀请码已过期", code=400)
    inviter = await UserOperate.get_user_by_uid(info.INVITER_UID)
    return api_response(True, "邀请码有效", {
        'days': info.DAYS,
        'inviter': inviter.USERNAME if inviter else None,
    })


@invite_bp.route('/use', methods=['POST'])
@require_auth
async def use_invite_code():
    """已登录但还没有 Emby 账号的用户使用邀请码 → 创建 Emby 账号 + 落邀请关系。"""
    blocked = _require_invite_enabled()
    if blocked:
        return blocked

    user = g.current_user
    if user.EMBYID:
        return api_response(False, "您已拥有 Emby 账号，无需使用邀请码", code=400)

    data = request.get_json(silent=True) or {}
    code = (data.get('code') or '').strip()
    emby_username = (data.get('emby_username') or '').strip()
    raw_password = data.get('emby_password')
    emby_password = raw_password if isinstance(raw_password, str) else None

    if not code:
        return api_response(False, "缺少邀请码", code=400)
    if not emby_username:
        return api_response(False, "请填写 Emby 用户名", code=400)
    if not is_valid_username(emby_username):
        return api_response(False, "Emby 用户名格式不正确（3-20 位字母数字下划线，不能以数字开头）", code=400)
    pwd_ok, pwd_msg = UserService.validate_password_strength(emby_password, label="Emby 密码")
    if not pwd_ok:
        return api_response(False, pwd_msg, code=400)

    info = await InviteCodeOperate.get_code(code)
    if not info or not info.ACTIVE:
        return api_response(False, "邀请码无效或已停用", code=404)
    if info.USE_COUNT_LIMIT != -1 and info.USE_COUNT >= info.USE_COUNT_LIMIT:
        return api_response(False, "邀请码已被使用", code=400)
    if info.EXPIRES_AT != -1 and timestamp() > info.EXPIRES_AT:
        return api_response(False, "邀请码已过期", code=400)
    if info.INVITER_UID == user.UID:
        return api_response(False, "不能使用自己生成的邀请码", code=400)

    # 层级校验
    inviter_depth = await InviteService.get_ancestor_depth(info.INVITER_UID)
    if inviter_depth + 1 > InviteService.max_depth():
        return api_response(False, "该邀请会超过最大层级限制", code=400)

    emby = get_emby_client()
    try:
        existing = await emby.get_user_by_name(emby_username)
        if existing:
            return api_response(False, "该 Emby 用户名已被占用", code=400)
        emby_user = await emby.create_user(emby_username, emby_password or '')
        if not emby_user:
            return api_response(False, "创建 Emby 账户失败", code=502)
    except EmbyError as exc:
        logger.error(f"邀请码创建 Emby 账户失败: {exc}")
        return api_response(False, f"Emby 服务器错误: {exc}", code=502)

    days = info.DAYS
    if days is None:
        days = int(RegisterConfig.INVITE_CODE_DEFAULT_DAYS or 30)
    expire_at = -1 if days <= 0 else timestamp() + days_to_seconds(days)

    user.EMBYID = emby_user.id
    user.ACTIVE_STATUS = True
    if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
        user.EXPIRED_AT = 253402214400
    else:
        user.EXPIRED_AT = expire_at
        if user.ROLE == Role.UNRECOGNIZED.value:
            user.ROLE = Role.NORMAL.value
    import json as _json
    other_data = {}
    if user.OTHER:
        try:
            other_data = _json.loads(user.OTHER)
        except (ValueError, TypeError):
            other_data = {}
    if not isinstance(other_data, dict):
        other_data = {}
    other_data['emby_username'] = emby_username
    user.OTHER = _json.dumps(other_data)
    await UserOperate.update_user(user)

    # 同步系统登录密码哈希
    try:
        user.PASSWORD = hash_password(emby_password or '')
        await UserOperate.update_user(user)
    except Exception:  # pragma: no cover
        pass

    ok, msg, inviter_uid = await InviteService.apply_invite(user.UID, code)
    if not ok:
        logger.warning(f"邀请关系建立失败: {msg}")
        # Emby 已建好，仅记录邀请关系失败，不回滚

    return api_response(True, "邀请使用成功", {
        'emby_id': user.EMBYID,
        'emby_username': emby_username,
        'expired_at': user.EXPIRED_AT,
        'inviter_uid': inviter_uid,
        'days': days,
    })
