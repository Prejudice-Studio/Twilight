"""
管理员 API

提供管理员专用的操作接口
"""
from flask import Blueprint, request, g

from src.api.v1.auth import async_route, require_auth, require_admin, api_response
from src.db.user import UserOperate, Role
from src.db.regcode import RegCodeOperate
from src.services import UserService, ScoreService, EmbyService

admin_bp = Blueprint('admin', __name__, url_prefix='/admin')


# ==================== 用户管理 ====================

@admin_bp.route('/users', methods=['GET'])
@async_route
@require_auth
@require_admin
async def list_users():
    """
    获取用户列表
    
    Query:
        page: int - 页码（从1开始）
        per_page: int - 每页数量（默认20，最大100）
        role: int - 按角色筛选
        active: bool - 按状态筛选
    
    TODO: 实现分页查询
    """
    # page = request.args.get('page', 1, type=int)
    # per_page = min(request.args.get('per_page', 20, type=int), 100)
    
    return api_response(True, "功能开发中", {
        'users': [],
        'total': 0,
        'page': 1,
        'per_page': 20,
    })


@admin_bp.route('/users/<int:uid>', methods=['GET'])
@async_route
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


@admin_bp.route('/users/<int:uid>/disable', methods=['POST'])
@async_route
@require_auth
@require_admin
async def disable_user(uid: int):
    """
    禁用用户
    
    Request:
        {
            "reason": "违规操作"
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    data = request.get_json() or {}
    reason = data.get('reason', '')
    
    success, message = await UserService.disable_user(user, reason)
    return api_response(success, message)


@admin_bp.route('/users/<int:uid>/enable', methods=['POST'])
@async_route
@require_auth
@require_admin
async def enable_user(uid: int):
    """启用用户"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    success, message = await UserService.enable_user(user)
    return api_response(success, message)


@admin_bp.route('/users/<int:uid>', methods=['DELETE'])
@async_route
@require_auth
@require_admin
async def delete_user(uid: int):
    """
    删除用户
    
    Query:
        delete_emby: bool - 是否同时删除 Emby 账户（默认 true）
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    delete_emby = request.args.get('delete_emby', 'true').lower() == 'true'
    
    success, message = await UserService.delete_user(user, delete_emby)
    return api_response(success, message)


@admin_bp.route('/users/<int:uid>/renew', methods=['POST'])
@async_route
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


@admin_bp.route('/users/<int:uid>/kick', methods=['POST'])
@async_route
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


@admin_bp.route('/users/<int:uid>/libraries', methods=['PUT'])
@async_route
@require_auth
@require_admin
async def set_user_libraries(uid: int):
    """
    设置用户媒体库权限
    
    Request:
        {
            "library_ids": ["id1", "id2"],
            "enable_all": false
        }
    """
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    data = request.get_json() or {}
    library_ids = data.get('library_ids', [])
    enable_all = data.get('enable_all', False)
    
    success, message = await EmbyService.set_user_library_access(user, library_ids, enable_all)
    return api_response(success, message)


@admin_bp.route('/users/<int:uid>/admin', methods=['PUT'])
@async_route
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


# ==================== 积分管理 ====================

@admin_bp.route('/users/<int:uid>/score', methods=['PUT'])
@async_route
@require_auth
@require_admin
async def adjust_user_score(uid: int):
    """
    调整用户积分
    
    Request:
        {
            "amount": 100,      // 正数增加，负数扣除
            "reason": "奖励"
        }
    """
    data = request.get_json() or {}
    amount = data.get('amount')
    reason = data.get('reason', '')
    
    if amount is None:
        return api_response(False, "缺少 amount 参数", code=400)
    
    success, message = await ScoreService.admin_adjust_score(uid, amount, reason)
    return api_response(success, message)


# ==================== 注册码管理 ====================

@admin_bp.route('/regcodes', methods=['GET'])
@async_route
@require_auth
@require_admin
async def list_regcodes():
    """
    获取注册码列表
    
    Query:
        type: int - 类型筛选 (1=注册, 2=续期, 3=白名单)
    """
    code_type = request.args.get('type', type=int)
    
    if code_type:
        codes = await RegCodeOperate.get_regcodes_by_type(code_type)
    else:
        # TODO: 实现获取所有注册码的方法
        codes = []
    
    return api_response(True, "获取成功", [{
        'code': c.CODE,
        'type': c.TYPE,
        'validity_time': c.VALIDITY_TIME,
        'use_count': c.USE_COUNT,
        'use_count_limit': c.USE_COUNT_LIMIT,
        'days': c.DAYS,
        'active': c.ACTIVE,
        'created_time': c.CREATED_TIME,
    } for c in codes])


@admin_bp.route('/regcodes', methods=['POST'])
@async_route
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
            "days": 30,             // 有效天数
            "count": 1              // 生成数量
        }
    """
    data = request.get_json() or {}
    
    code_type = data.get('type', 1)
    validity_time = data.get('validity_time', -1)
    use_count_limit = data.get('use_count_limit', 1)
    days = data.get('days', 30)
    count = data.get('count', 1)
    
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
@async_route
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
@async_route
@require_auth
@require_admin
async def list_sessions():
    """获取所有活动会话"""
    sessions = await EmbyService.get_all_sessions()
    return api_response(True, "获取成功", sessions)


@admin_bp.route('/emby/activity', methods=['GET'])
@async_route
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
@async_route
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
@async_route
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
@async_route
@require_auth
@require_admin
async def get_stats():
    """获取系统统计信息"""
    from src.config import ScoreAndRegisterConfig
    
    registered_count = await UserOperate.get_registered_users_count()
    active_count = await UserOperate.get_active_users_count()
    regcode_count = await RegCodeOperate.get_active_regcodes_count()
    server_status = await EmbyService.get_server_status()
    
    return api_response(True, "获取成功", {
        'users': {
            'registered': registered_count,
            'active': active_count,
            'limit': ScoreAndRegisterConfig.USER_LIMIT,
        },
        'regcodes': {
            'active': regcode_count,
        },
        'emby': {
            'online': server_status.get('online', False),
            'active_sessions': server_status.get('active_sessions', 0),
        },
    })

