"""
安全管理 API

设备管理、IP 管理、登录日志等
"""
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, require_admin, api_response
from src.services.security_service import SecurityService
from src.db.login_log import LoginLogOperate, IPListOperate

security_bp = Blueprint('security', __name__, url_prefix='/security')


# ==================== 用户设备管理 ====================

@security_bp.route('/devices', methods=['GET'])
@require_auth
async def get_my_devices():
    """获取我的设备列表"""
    devices = await SecurityService.get_user_devices(g.current_user.UID)
    return api_response(True, "获取成功", devices)


@security_bp.route('/devices/<device_id>/block', methods=['POST'])
@require_auth
async def block_my_device(device_id: str):
    """封禁我的设备"""
    success, msg = await SecurityService.block_user_device(g.current_user.UID, device_id)
    return api_response(success, msg)


@security_bp.route('/devices/<device_id>/trust', methods=['POST'])
@require_auth
async def trust_my_device(device_id: str):
    """信任我的设备"""
    success, msg = await SecurityService.trust_user_device(g.current_user.UID, device_id)
    return api_response(success, msg)


# ==================== 登录日志 ====================

@security_bp.route('/login-history', methods=['GET'])
@require_auth
async def get_my_login_history():
    """获取我的登录历史"""
    limit = request.args.get('limit', 50, type=int)
    limit = min(max(limit, 1), 100)
    
    history = await SecurityService.get_user_login_history(g.current_user.UID, limit)
    return api_response(True, "获取成功", history)


@security_bp.route('/login-history/<int:uid>', methods=['GET'])
@require_auth
@require_admin
async def get_user_login_history(uid: int):
    """获取指定用户的登录历史（管理员）"""
    limit = request.args.get('limit', 50, type=int)
    history = await SecurityService.get_user_login_history(uid, limit)
    return api_response(True, "获取成功", history)


# ==================== IP 管理（管理员） ====================

@security_bp.route('/ip/blacklist', methods=['GET'])
@require_auth
@require_admin
async def get_ip_blacklist():
    """获取 IP 黑名单"""
    from src.db.login_log import LoginLogSessionFactory, IPBlacklistModel
    from sqlalchemy import select
    
    async with LoginLogSessionFactory() as session:
        result = await session.execute(select(IPBlacklistModel))
        records = result.scalars().all()
    
    return api_response(True, "获取成功", [{
        'ip': r.IP_ADDRESS,
        'reason': r.REASON,
        'created_at': r.CREATED_AT,
        'expire_at': r.EXPIRE_AT,
    } for r in records])


@security_bp.route('/ip/blacklist', methods=['POST'])
@require_auth
@require_admin
async def add_ip_to_blacklist():
    """添加 IP 到黑名单"""
    data = request.get_json() or {}
    ip = data.get('ip')
    reason = data.get('reason', '')
    hours = data.get('hours', -1)
    
    if not ip:
        return api_response(False, "缺少 IP", code=400)
    
    success, msg = await SecurityService.add_ip_to_blacklist(ip, reason, hours)
    return api_response(success, msg)


@security_bp.route('/ip/blacklist', methods=['DELETE'])
@require_auth
@require_admin
async def remove_ip_from_blacklist():
    """从黑名单移除 IP"""
    data = request.get_json() or {}
    ip = data.get('ip')
    
    if not ip:
        return api_response(False, "缺少 IP", code=400)
    
    success, msg = await SecurityService.remove_ip_from_blacklist(ip)
    return api_response(success, msg)


# ==================== 可疑活动 ====================

@security_bp.route('/suspicious', methods=['GET'])
@require_auth
@require_admin
async def get_suspicious_activity():
    """获取可疑活动"""
    hours = request.args.get('hours', 24, type=int)
    activity = await SecurityService.get_suspicious_activity(hours)
    return api_response(True, "获取成功", activity)


# ==================== 管理员设备管理 ====================

@security_bp.route('/users/<int:uid>/devices', methods=['GET'])
@require_auth
@require_admin
async def get_user_devices(uid: int):
    """获取用户设备（管理员）"""
    devices = await SecurityService.get_user_devices(uid)
    return api_response(True, "获取成功", devices)


@security_bp.route('/users/<int:uid>/devices/<device_id>/block', methods=['POST'])
@require_auth
@require_admin
async def admin_block_device(uid: int, device_id: str):
    """封禁用户设备（管理员）"""
    success, msg = await SecurityService.block_user_device(uid, device_id)
    return api_response(success, msg)

