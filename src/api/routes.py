"""
API 路由模块

提供 RESTful API 接口
"""
import logging
from functools import wraps
from typing import Callable

from flask import Blueprint, request, jsonify, g

from src.config import TelegramConfig
from src.db.user import UserOperate, Role
from src.services import (
    UserService,
    ScoreService,
    RedPacketService,
    RedPacketType,
    EmbyService,
    get_emby_client,
)
from src.schemas import APIResponse

logger = logging.getLogger(__name__)

# 创建蓝图
api = Blueprint('api', __name__, url_prefix='/api')
admin_api = Blueprint('admin', __name__, url_prefix='/api/admin')


# ==================== 中间件与装饰器 ====================

def api_response(success: bool, message: str, data=None, status_code: int = 200):
    """统一 API 响应格式"""
    return jsonify({
        'success': success,
        'message': message,
        'data': data
    }), status_code


def async_route(f: Callable) -> Callable:
    """异步路由装饰器"""
    @wraps(f)
    def wrapper(*args, **kwargs):
        import asyncio
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        try:
            return loop.run_until_complete(f(*args, **kwargs))
        finally:
            loop.close()
    return wrapper


def require_api_key(f: Callable) -> Callable:
    """需要 API Key 认证"""
    @wraps(f)
    async def wrapper(*args, **kwargs):
        api_key = request.headers.get('X-API-Key') or request.args.get('api_key')
        if not api_key:
            return api_response(False, "缺少 API Key", status_code=401)
        
        # 查找用户
        from src.db.user import UserModel
        # 这里需要实现通过 API Key 查找用户的逻辑
        # 暂时跳过验证
        return await f(*args, **kwargs)
    return wrapper


def require_admin(f: Callable) -> Callable:
    """需要管理员权限"""
    @wraps(f)
    async def wrapper(*args, **kwargs):
        admin_key = request.headers.get('X-Admin-Key')
        # 简单的管理员验证（生产环境应该更安全）
        if not admin_key:
            return api_response(False, "需要管理员权限", status_code=403)
        return await f(*args, **kwargs)
    return wrapper


# ==================== 公共 API ====================

@api.route('/health', methods=['GET'])
def health_check():
    """健康检查"""
    return api_response(True, "服务正常")


@api.route('/emby/status', methods=['GET'])
@async_route
async def emby_status():
    """Emby 服务器状态"""
    emby = get_emby_client()
    try:
        is_online = await emby.ping()
        if is_online:
            info = await emby.get_server_info()
            return api_response(True, "Emby 在线", {
                'online': True,
                'server_name': info.get('ServerName'),
                'version': info.get('Version'),
            })
        else:
            return api_response(False, "Emby 离线", {'online': False})
    except Exception as e:
        return api_response(False, f"检查失败: {e}", {'online': False})


# ==================== 用户 API ====================

@api.route('/user/register', methods=['POST'])
@async_route
async def user_register():
    """用户注册"""
    data = request.get_json() or {}
    
    telegram_id = data.get('telegram_id')
    username = data.get('username')
    reg_code = data.get('reg_code')
    email = data.get('email')
    use_score = data.get('use_score', False)
    
    if not telegram_id or not username:
        return api_response(False, "缺少必要参数", status_code=400)
    
    if use_score:
        result = await UserService.register_by_score(telegram_id, username, email)
    elif reg_code:
        result = await UserService.register_by_code(telegram_id, username, reg_code, email)
    else:
        return api_response(False, "需要提供注册码或使用积分注册", status_code=400)
    
    if result.result.value == 'success':
        return api_response(True, result.message, {
            'username': result.user.USERNAME if result.user else None,
            'password': result.emby_password,
        })
    else:
        return api_response(False, result.message, status_code=400)


@api.route('/user/info', methods=['GET'])
@async_route
async def user_info():
    """获取用户信息"""
    telegram_id = request.args.get('telegram_id', type=int)
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    info = await UserService.get_user_info(user)
    return api_response(True, "获取成功", info)


@api.route('/user/renew', methods=['POST'])
@async_route
async def user_renew():
    """用户续期"""
    data = request.get_json() or {}
    
    telegram_id = data.get('telegram_id')
    reg_code = data.get('reg_code')
    
    if not telegram_id or not reg_code:
        return api_response(False, "缺少必要参数", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await UserService.renew_user(user, 30, reg_code)
    return api_response(success, message)


@api.route('/user/reset-password', methods=['POST'])
@async_route
async def user_reset_password():
    """重置密码"""
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message, new_password = await UserService.reset_password(user)
    if success:
        return api_response(True, message, {'new_password': new_password})
    return api_response(False, message)


# ==================== 积分 API ====================

@api.route('/score/checkin', methods=['POST'])
@async_route
async def score_checkin():
    """签到"""
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    result = await ScoreService.checkin(telegram_id)
    return api_response(
        result.result.value == 'success',
        result.message,
        {
            'score_gained': result.score_gained,
            'total_score': result.total_score,
            'checkin_days': result.checkin_days,
        }
    )


@api.route('/score/balance', methods=['GET'])
@async_route
async def score_balance():
    """查询积分余额"""
    telegram_id = request.args.get('telegram_id', type=int)
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    score, checkin_days = await ScoreService.get_balance(telegram_id)
    return api_response(True, "查询成功", {
        'score': score,
        'checkin_days': checkin_days,
    })


@api.route('/score/transfer', methods=['POST'])
@async_route
async def score_transfer():
    """积分转账"""
    data = request.get_json() or {}
    
    from_id = data.get('from_telegram_id')
    to_id = data.get('to_telegram_id')
    amount = data.get('amount')
    
    if not all([from_id, to_id, amount]):
        return api_response(False, "缺少必要参数", status_code=400)
    
    success, message = await ScoreService.transfer(from_id, to_id, amount)
    return api_response(success, message)


@api.route('/score/ranking', methods=['GET'])
@async_route
async def score_ranking():
    """积分排行榜"""
    limit = request.args.get('limit', 10, type=int)
    limit = min(limit, 50)  # 最多50
    
    ranking = await ScoreService.get_ranking(limit)
    return api_response(True, "获取成功", ranking)


# ==================== 红包 API ====================

@api.route('/redpacket/create', methods=['POST'])
@async_route
async def redpacket_create():
    """创建红包"""
    data = request.get_json() or {}
    
    telegram_id = data.get('telegram_id')
    amount = data.get('amount')
    count = data.get('count')
    packet_type = data.get('type', 1)
    
    if not all([telegram_id, amount, count]):
        return api_response(False, "缺少必要参数", status_code=400)
    
    rp_type = RedPacketType.EQUAL if packet_type == 2 else RedPacketType.RANDOM
    success, message, rp_key = await RedPacketService.create_red_packet(
        telegram_id, amount, count, rp_type
    )
    
    if success:
        return api_response(True, message, {'rp_key': rp_key})
    return api_response(False, message)


@api.route('/redpacket/grab', methods=['POST'])
@async_route
async def redpacket_grab():
    """抢红包"""
    data = request.get_json() or {}
    
    rp_key = data.get('rp_key')
    telegram_id = data.get('telegram_id')
    
    if not rp_key or not telegram_id:
        return api_response(False, "缺少必要参数", status_code=400)
    
    success, message, amount = await RedPacketService.grab_red_packet(rp_key, telegram_id)
    return api_response(success, message, {'amount': amount})


# ==================== 管理员 API ====================

@admin_api.route('/user/list', methods=['GET'])
@async_route
@require_admin
async def admin_user_list():
    """获取用户列表"""
    # TODO: 实现分页查询
    return api_response(True, "功能开发中")


@admin_api.route('/user/disable', methods=['POST'])
@async_route
@require_admin
async def admin_disable_user():
    """禁用用户"""
    data = request.get_json() or {}
    uid = data.get('uid')
    reason = data.get('reason', '')
    
    if not uid:
        return api_response(False, "缺少 uid", status_code=400)
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await UserService.disable_user(user, reason)
    return api_response(success, message)


@admin_api.route('/user/enable', methods=['POST'])
@async_route
@require_admin
async def admin_enable_user():
    """启用用户"""
    data = request.get_json() or {}
    uid = data.get('uid')
    
    if not uid:
        return api_response(False, "缺少 uid", status_code=400)
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await UserService.enable_user(user)
    return api_response(success, message)


@admin_api.route('/user/delete', methods=['POST'])
@async_route
@require_admin
async def admin_delete_user():
    """删除用户"""
    data = request.get_json() or {}
    uid = data.get('uid')
    delete_emby = data.get('delete_emby', True)
    
    if not uid:
        return api_response(False, "缺少 uid", status_code=400)
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await UserService.delete_user(user, delete_emby)
    return api_response(success, message)


@admin_api.route('/score/adjust', methods=['POST'])
@async_route
@require_admin
async def admin_adjust_score():
    """调整积分"""
    data = request.get_json() or {}
    uid = data.get('uid')
    amount = data.get('amount')
    reason = data.get('reason', '')
    
    if uid is None or amount is None:
        return api_response(False, "缺少必要参数", status_code=400)
    
    success, message = await ScoreService.admin_adjust_score(uid, amount, reason)
    return api_response(success, message)


@admin_api.route('/regcode/create', methods=['POST'])
@async_route
@require_admin
async def admin_create_regcode():
    """创建注册码"""
    from src.db.regcode import RegCodeOperate
    
    data = request.get_json() or {}
    validity_time = data.get('validity_time', -1)
    code_type = data.get('type', 1)
    use_count_limit = data.get('use_count_limit', 1)
    count = data.get('count', 1)
    days = data.get('days', 30)
    
    codes = await RegCodeOperate.create_regcode(
        validity_time, code_type, use_count_limit, count, days
    )
    
    return api_response(True, "创建成功", {
        'codes': codes if isinstance(codes, list) else [codes]
    })


# ==================== Emby 管理 API ====================

@api.route('/emby/server', methods=['GET'])
@async_route
async def emby_server_status():
    """获取 Emby 服务器详细状态"""
    status = await EmbyService.get_server_status()
    return api_response(status.get('online', False), status.get('message', ''), status)


@api.route('/emby/libraries', methods=['GET'])
@async_route
async def emby_libraries():
    """获取媒体库列表"""
    libraries = await EmbyService.get_libraries_info()
    return api_response(True, "获取成功", libraries)


@api.route('/emby/sessions', methods=['GET'])
@async_route
async def emby_sessions():
    """获取所有活动会话"""
    sessions = await EmbyService.get_all_sessions()
    return api_response(True, "获取成功", {
        'sessions': sessions,
        'count': len(sessions),
    })


@api.route('/emby/search', methods=['GET'])
@async_route
async def emby_search():
    """搜索媒体"""
    query = request.args.get('q', '')
    limit = request.args.get('limit', 20, type=int)
    
    if not query:
        return api_response(False, "缺少搜索关键词", status_code=400)
    
    results = await EmbyService.search_media(query, min(limit, 50))
    return api_response(True, "搜索成功", results)


@api.route('/emby/latest', methods=['GET'])
@async_route
async def emby_latest():
    """获取最新媒体"""
    limit = request.args.get('limit', 20, type=int)
    item_type = request.args.get('type')  # Movie, Series
    
    types = [item_type] if item_type else None
    results = await EmbyService.get_latest_media(types, min(limit, 50))
    return api_response(True, "获取成功", results)


@api.route('/user/libraries', methods=['GET'])
@async_route
async def user_libraries():
    """获取用户可访问的媒体库"""
    telegram_id = request.args.get('telegram_id', type=int)
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    library_ids, enable_all = await EmbyService.get_user_library_access(user)
    return api_response(True, "获取成功", {
        'library_ids': library_ids,
        'enable_all': enable_all,
    })


@api.route('/user/devices', methods=['GET'])
@async_route
async def user_devices():
    """获取用户设备列表"""
    telegram_id = request.args.get('telegram_id', type=int)
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    devices = await EmbyService.get_user_devices(user)
    return api_response(True, "获取成功", devices)


@api.route('/user/devices/remove', methods=['POST'])
@async_route
async def user_remove_device():
    """移除用户设备"""
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    device_id = data.get('device_id')
    
    if not telegram_id or not device_id:
        return api_response(False, "缺少必要参数", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await EmbyService.remove_user_device(user, device_id)
    return api_response(success, message)


@api.route('/user/nsfw', methods=['POST'])
@async_route
async def user_toggle_nsfw():
    """切换 NSFW 库访问权限"""
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    enable = data.get('enable', False)
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await UserService.toggle_nsfw(user, enable)
    return api_response(success, message)


@api.route('/user/change-username', methods=['POST'])
@async_route
async def user_change_username():
    """修改用户名"""
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    new_username = data.get('new_username')
    
    if not telegram_id or not new_username:
        return api_response(False, "缺少必要参数", status_code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await UserService.change_username(user, new_username)
    return api_response(success, message)


# ==================== 管理员 Emby API ====================

@admin_api.route('/emby/activity', methods=['GET'])
@async_route
@require_admin
async def admin_emby_activity():
    """获取 Emby 活动日志"""
    limit = request.args.get('limit', 50, type=int)
    
    logs = await EmbyService.get_activity_log(min(limit, 200))
    return api_response(True, "获取成功", logs)


@admin_api.route('/emby/broadcast', methods=['POST'])
@async_route
@require_admin
async def admin_emby_broadcast():
    """广播消息到所有会话"""
    data = request.get_json() or {}
    header = data.get('header', '通知')
    text = data.get('text')
    
    if not text:
        return api_response(False, "缺少消息内容", status_code=400)
    
    sent = await EmbyService.broadcast_message(header, text)
    return api_response(True, f"已发送到 {sent} 个会话", {'sent_count': sent})


@admin_api.route('/user/kick', methods=['POST'])
@async_route
@require_admin
async def admin_kick_user():
    """踢出用户所有会话"""
    data = request.get_json() or {}
    uid = data.get('uid')
    
    if not uid:
        return api_response(False, "缺少 uid", status_code=400)
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, kicked = await EmbyService.kick_user_sessions(user)
    if success:
        return api_response(True, f"已踢出 {kicked} 个会话", {'kicked_count': kicked})
    return api_response(False, "操作失败")


@admin_api.route('/user/libraries', methods=['POST'])
@async_route
@require_admin
async def admin_set_user_libraries():
    """设置用户媒体库权限"""
    data = request.get_json() or {}
    uid = data.get('uid')
    library_ids = data.get('library_ids', [])
    enable_all = data.get('enable_all', False)
    
    if not uid:
        return api_response(False, "缺少 uid", status_code=400)
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", status_code=404)
    
    success, message = await EmbyService.set_user_library_access(user, library_ids, enable_all)
    return api_response(success, message)


@admin_api.route('/user/whitelist', methods=['POST'])
@async_route
@require_admin
async def admin_create_whitelist():
    """创建白名单用户"""
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    username = data.get('username')
    email = data.get('email')
    
    if not telegram_id or not username:
        return api_response(False, "缺少必要参数", status_code=400)
    
    result = await UserService.create_whitelist_user(telegram_id, username, email)
    
    if result.result.value == 'success':
        return api_response(True, result.message, {
            'username': result.user.USERNAME if result.user else None,
            'password': result.emby_password,
        })
    return api_response(False, result.message, status_code=400)

