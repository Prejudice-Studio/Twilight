"""
批量操作 API

批量禁用/启用/续期/删除用户
"""
from flask import Blueprint, request, g

from src.api.v1.auth import async_route, require_auth, require_admin, api_response
from src.services.admin_service import BatchOperationService, DataExportService, WatchHistoryService, ReminderService

batch_bp = Blueprint('batch', __name__, url_prefix='/batch')


# ==================== 批量用户操作 ====================

@batch_bp.route('/users/disable', methods=['POST'])
@async_route
@require_auth
@require_admin
async def batch_disable_users():
    """
    批量禁用用户
    
    Request:
        {
            "uids": [1, 2, 3],
            "reason": "违规"
        }
    """
    data = request.get_json() or {}
    uids = data.get('uids', [])
    reason = data.get('reason', '')
    
    if not uids:
        return api_response(False, "请提供用户 UID 列表", code=400)
    
    if not isinstance(uids, list):
        return api_response(False, "uids 必须是数组", code=400)
    
    result = await BatchOperationService.batch_disable_users(uids, reason)
    return api_response(True, f"操作完成: 成功 {result['success']}, 失败 {result['failed']}", result)


@batch_bp.route('/users/enable', methods=['POST'])
@async_route
@require_auth
@require_admin
async def batch_enable_users():
    """批量启用用户"""
    data = request.get_json() or {}
    uids = data.get('uids', [])
    
    if not uids:
        return api_response(False, "请提供用户 UID 列表", code=400)
    
    result = await BatchOperationService.batch_enable_users(uids)
    return api_response(True, f"操作完成: 成功 {result['success']}, 失败 {result['failed']}", result)


@batch_bp.route('/users/renew', methods=['POST'])
@async_route
@require_auth
@require_admin
async def batch_renew_users():
    """
    批量续期用户
    
    Request:
        {
            "uids": [1, 2, 3],
            "days": 30
        }
    """
    data = request.get_json() or {}
    uids = data.get('uids', [])
    days = data.get('days', 30)
    
    if not uids:
        return api_response(False, "请提供用户 UID 列表", code=400)
    
    if days <= 0:
        return api_response(False, "续期天数必须大于 0", code=400)
    
    result = await BatchOperationService.batch_renew_users(uids, days)
    return api_response(True, f"操作完成: 成功 {result['success']}, 失败 {result['failed']}", result)


@batch_bp.route('/users/delete', methods=['POST'])
@async_route
@require_auth
@require_admin
async def batch_delete_users():
    """
    批量删除用户
    
    Request:
        {
            "uids": [1, 2, 3],
            "delete_emby": true
        }
    """
    data = request.get_json() or {}
    uids = data.get('uids', [])
    delete_emby = data.get('delete_emby', True)
    
    if not uids:
        return api_response(False, "请提供用户 UID 列表", code=400)
    
    result = await BatchOperationService.batch_delete_users(uids, delete_emby)
    return api_response(True, f"操作完成: 成功 {result['success']}, 失败 {result['failed']}", result)


@batch_bp.route('/users/score', methods=['POST'])
@async_route
@require_auth
@require_admin
async def batch_adjust_score():
    """
    批量调整积分
    
    Request:
        {
            "uids": [1, 2, 3],
            "amount": 100,  // 正数增加，负数扣除
            "reason": "活动奖励"
        }
    """
    data = request.get_json() or {}
    uids = data.get('uids', [])
    amount = data.get('amount', 0)
    reason = data.get('reason', '')
    
    if not uids:
        return api_response(False, "请提供用户 UID 列表", code=400)
    
    if amount == 0:
        return api_response(False, "调整数量不能为 0", code=400)
    
    result = await BatchOperationService.batch_adjust_score(uids, amount, reason)
    return api_response(True, f"操作完成: 成功 {result['success']}, 失败 {result['failed']}", result)


# ==================== 数据导出 ====================

@batch_bp.route('/export/users', methods=['GET'])
@async_route
@require_auth
@require_admin
async def export_users():
    """
    导出用户数据
    
    Query:
        format: csv/json (默认 csv)
        include_score: bool (默认 true)
        include_playback: bool (默认 false)
        active_only: bool (默认 false)
    """
    from flask import Response
    
    fmt = request.args.get('format', 'csv')
    include_score = request.args.get('include_score', 'true').lower() == 'true'
    include_playback = request.args.get('include_playback', 'false').lower() == 'true'
    active_only = request.args.get('active_only', 'false').lower() == 'true'
    
    if fmt == 'json':
        data = await DataExportService.export_users_json(include_score)
        return Response(
            data,
            mimetype='application/json',
            headers={'Content-Disposition': 'attachment; filename=users.json'}
        )
    else:
        data = await DataExportService.export_users_csv(include_score, include_playback, active_only)
        return Response(
            data,
            mimetype='text/csv',
            headers={'Content-Disposition': 'attachment; filename=users.csv'}
        )


@batch_bp.route('/export/playback', methods=['GET'])
@async_route
@require_auth
@require_admin
async def export_playback():
    """
    导出播放统计
    
    Query:
        days: int (默认 30，0 = 全部)
    """
    from flask import Response
    
    days = request.args.get('days', 30, type=int)
    
    data = await DataExportService.export_playback_stats_csv(days)
    return Response(
        data,
        mimetype='text/csv',
        headers={'Content-Disposition': 'attachment; filename=playback_stats.csv'}
    )


# ==================== 观看统计 ====================

@batch_bp.route('/watch-stats', methods=['GET'])
@async_route
@require_auth
async def get_my_watch_stats():
    """获取我的观看统计"""
    stats = await WatchHistoryService.get_user_watch_stats(g.current_user.UID)
    return api_response(True, "获取成功", stats)


@batch_bp.route('/watch-stats/<int:uid>', methods=['GET'])
@async_route
@require_auth
@require_admin
async def get_user_watch_stats(uid: int):
    """获取用户观看统计（管理员）"""
    stats = await WatchHistoryService.get_user_watch_stats(uid)
    return api_response(True, "获取成功", stats)


@batch_bp.route('/watch-stats/global', methods=['GET'])
@async_route
@require_auth
@require_admin
async def get_global_watch_stats():
    """获取全站观看统计"""
    days = request.args.get('days', 7, type=int)
    stats = await WatchHistoryService.get_global_watch_stats(days)
    return api_response(True, "获取成功", stats)


# ==================== 到期提醒 ====================

@batch_bp.route('/expiring-users', methods=['GET'])
@async_route
@require_auth
@require_admin
async def get_expiring_users():
    """获取即将到期的用户"""
    days = request.args.get('days', 3, type=int)
    users = await ReminderService.get_expiring_users(days)
    return api_response(True, "获取成功", {
        'days': days,
        'count': len(users),
        'users': users,
    })


@batch_bp.route('/send-reminders', methods=['POST'])
@async_route
@require_auth
@require_admin
async def send_expiry_reminders():
    """发送到期提醒"""
    result = await ReminderService.send_expiry_reminders()
    return api_response(True, f"已发送 {result['sent']} 条提醒", result)

