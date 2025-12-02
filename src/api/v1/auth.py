"""
认证 API

提供用户登录、Token 管理、API Key 认证等功能
支持两种认证方式：
1. Token 认证（登录后获取，有效期 7 天）
2. API Key 认证（用户生成，长期有效）
"""
import secrets
import logging
from functools import wraps
from typing import Callable, Optional

from flask import Blueprint, request, jsonify, g

from src.db.user import UserOperate, UserModel, Role
from src.core.utils import timestamp

logger = logging.getLogger(__name__)

auth_bp = Blueprint('auth', __name__, url_prefix='/auth')

# Token 存储 (生产环境建议使用 Redis)
_tokens: dict = {}
TOKEN_EXPIRE_SECONDS = 86400 * 7  # 7 天


# ==================== Token 管理 ====================

def generate_token(user_id: int) -> str:
    """生成访问 Token"""
    token = f"tw_{secrets.token_urlsafe(32)}"
    _tokens[token] = {
        'user_id': user_id,
        'created_at': timestamp(),
        'expires_at': timestamp() + TOKEN_EXPIRE_SECONDS,
    }
    return token


def verify_token(token: str) -> Optional[int]:
    """验证 Token，返回用户 ID"""
    if not token or token not in _tokens:
        return None
    
    token_data = _tokens[token]
    if timestamp() > token_data['expires_at']:
        del _tokens[token]
        return None
    
    return token_data['user_id']


def revoke_token(token: str) -> bool:
    """撤销 Token"""
    if token in _tokens:
        del _tokens[token]
        return True
    return False


def revoke_user_tokens(user_id: int) -> int:
    """撤销用户所有 Token"""
    count = 0
    tokens_to_remove = [
        t for t, data in _tokens.items() 
        if data['user_id'] == user_id
    ]
    for token in tokens_to_remove:
        del _tokens[token]
        count += 1
    return count


# ==================== 认证装饰器 ====================

def require_auth(f: Callable) -> Callable:
    """
    需要登录认证的装饰器
    
    支持两种认证方式：
    1. Authorization: Bearer <token>
    2. X-API-Key: <apikey>
    """
    @wraps(f)
    async def wrapper(*args, **kwargs):
        user = None
        auth_method = None
        
        # 方式1: Bearer Token
        auth_header = request.headers.get('Authorization', '')
        if auth_header.startswith('Bearer '):
            token = auth_header[7:]
            user_id = verify_token(token)
            if user_id:
                user = await UserOperate.get_user_by_uid(user_id)
                auth_method = 'token'
                g.token = token
        
        # 方式2: API Key
        if not user:
            api_key = request.headers.get('X-API-Key') or request.args.get('api_key')
            if api_key:
                user = await UserOperate.get_user_by_apikey(api_key)
                if user:
                    auth_method = 'apikey'
                    g.token = None
        
        if not user:
            return jsonify({
                'success': False,
                'code': 401,
                'message': '未登录或认证信息无效',
                'data': None,
                'timestamp': timestamp(),
            }), 401
        
        if not user.ACTIVE_STATUS:
            return jsonify({
                'success': False,
                'code': 403,
                'message': '账户已被禁用',
                'data': None,
                'timestamp': timestamp(),
            }), 403
        
        g.current_user = user
        g.auth_method = auth_method
        return await f(*args, **kwargs)
    
    return wrapper


def require_admin(f: Callable) -> Callable:
    """需要管理员权限的装饰器"""
    @wraps(f)
    async def wrapper(*args, **kwargs):
        if not hasattr(g, 'current_user'):
            return jsonify({
                'success': False,
                'code': 401,
                'message': '未登录',
                'data': None,
                'timestamp': timestamp(),
            }), 401
        
        if g.current_user.ROLE != Role.ADMIN.value:
            return jsonify({
                'success': False,
                'code': 403,
                'message': '需要管理员权限',
                'data': None,
                'timestamp': timestamp(),
            }), 403
        
        return await f(*args, **kwargs)
    
    return wrapper


def api_response(success: bool, message: str, data=None, code: int = 200):
    """统一 API 响应格式"""
    status_code = code if success else (code if code != 200 else 400)
    return jsonify({
        'success': success,
        'code': status_code,
        'message': message,
        'data': data,
        'timestamp': timestamp(),
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


# ==================== 认证路由 ====================

@auth_bp.route('/login/telegram', methods=['POST'])
@async_route
async def login_by_telegram():
    """
    通过 Telegram ID 登录
    
    Request:
        {
            "telegram_id": 123456789
        }
    
    Response:
        {
            "success": true,
            "data": {
                "token": "tw_xxx...",
                "expires_in": 604800,
                "user": { ... }
            }
        }
    """
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", code=400)
    
    user = await UserOperate.get_user_by_telegram_id(telegram_id)
    if not user:
        return api_response(False, "用户不存在，请先注册", code=404)
    
    if not user.ACTIVE_STATUS:
        return api_response(False, "账户已被禁用", code=403)
    
    # 更新登录信息
    ip = request.remote_addr or ''
    ua = request.headers.get('User-Agent', '')[:200]
    await UserOperate.update_login_info(user.UID, ip, ua)
    
    # 生成 Token
    token = generate_token(user.UID)
    
    from src.services import UserService
    user_info = await UserService.get_user_info(user)
    
    return api_response(True, "登录成功", {
        'token': token,
        'token_type': 'Bearer',
        'expires_in': TOKEN_EXPIRE_SECONDS,
        'user': user_info,
    })


@auth_bp.route('/login/apikey', methods=['POST'])
@async_route
async def login_by_apikey():
    """
    通过 API Key 登录（验证 API Key 有效性）
    
    Request:
        {
            "api_key": "key-xxx-xxx"
        }
    
    也可以直接使用 API Key 进行认证，无需登录获取 Token
    """
    data = request.get_json() or {}
    api_key = data.get('api_key')
    
    if not api_key:
        return api_response(False, "缺少 api_key", code=400)
    
    user = await UserOperate.get_user_by_apikey(api_key)
    if not user:
        return api_response(False, "API Key 无效或已禁用", code=401)
    
    if not user.ACTIVE_STATUS:
        return api_response(False, "账户已被禁用", code=403)
    
    # 更新登录信息
    ip = request.remote_addr or ''
    ua = request.headers.get('User-Agent', '')[:200]
    await UserOperate.update_login_info(user.UID, ip, ua)
    
    from src.services import UserService
    user_info = await UserService.get_user_info(user)
    
    return api_response(True, "API Key 验证成功", {
        'valid': True,
        'user': user_info,
        'tip': '可直接使用 X-API-Key 请求头进行认证',
    })


@auth_bp.route('/logout', methods=['POST'])
@async_route
@require_auth
async def logout():
    """登出，撤销当前 Token"""
    if g.token:
        revoke_token(g.token)
    return api_response(True, "已登出")


@auth_bp.route('/logout/all', methods=['POST'])
@async_route
@require_auth
async def logout_all():
    """登出所有设备，撤销用户所有 Token"""
    count = revoke_user_tokens(g.current_user.UID)
    return api_response(True, f"已撤销 {count} 个登录会话", {'revoked_count': count})


@auth_bp.route('/me', methods=['GET'])
@async_route
@require_auth
async def get_current_user():
    """获取当前登录用户信息"""
    from src.services import UserService
    user_info = await UserService.get_user_info(g.current_user)
    user_info['auth_method'] = g.auth_method
    return api_response(True, "获取成功", user_info)


@auth_bp.route('/refresh', methods=['POST'])
@async_route
@require_auth
async def refresh_token():
    """刷新 Token（仅 Token 认证方式可用）"""
    if g.auth_method != 'token':
        return api_response(False, "API Key 认证方式无需刷新", code=400)
    
    # 撤销旧 Token
    if g.token:
        revoke_token(g.token)
    
    # 生成新 Token
    new_token = generate_token(g.current_user.UID)
    
    return api_response(True, "Token 已刷新", {
        'token': new_token,
        'token_type': 'Bearer',
        'expires_in': TOKEN_EXPIRE_SECONDS,
    })


# ==================== API Key 管理 ====================

@auth_bp.route('/apikey', methods=['GET'])
@async_route
@require_auth
async def get_apikey_status():
    """获取 API Key 状态"""
    user = g.current_user
    
    has_key = bool(user.APIKEY)
    key_enabled = user.APIKEY_STATUS
    
    # 部分显示 API Key
    masked_key = None
    if has_key and user.APIKEY:
        key = user.APIKEY
        masked_key = key[:8] + '*' * (len(key) - 12) + key[-4:] if len(key) > 12 else '***'
    
    return api_response(True, "获取成功", {
        'has_apikey': has_key,
        'apikey_enabled': key_enabled,
        'apikey_masked': masked_key,
    })


@auth_bp.route('/apikey', methods=['POST'])
@async_route
@require_auth
async def generate_apikey():
    """
    生成新的 API Key（会覆盖旧的）
    
    Response:
        {
            "success": true,
            "data": {
                "api_key": "key-xxx-xxx",
                "warning": "请妥善保管，此 Key 仅显示一次"
            }
        }
    """
    user = g.current_user
    
    new_key = await UserOperate.reset_apikey(user)
    
    return api_response(True, "API Key 已生成", {
        'api_key': new_key,
        'warning': '请妥善保管此 API Key，它仅显示一次！',
    })


@auth_bp.route('/apikey', methods=['DELETE'])
@async_route
@require_auth
async def disable_apikey():
    """禁用 API Key"""
    await UserOperate.set_apikey_status(g.current_user.UID, False)
    return api_response(True, "API Key 已禁用")


@auth_bp.route('/apikey/enable', methods=['POST'])
@async_route
@require_auth
async def enable_apikey():
    """启用 API Key"""
    user = g.current_user
    
    if not user.APIKEY:
        return api_response(False, "请先生成 API Key", code=400)
    
    await UserOperate.set_apikey_status(user.UID, True)
    return api_response(True, "API Key 已启用")
