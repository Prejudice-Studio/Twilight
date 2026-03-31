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
from src.core.utils import verify_password, timestamp
from src.db.user import UserOperate, UserModel, Role
from src.db.login_log import LoginLogOperate, LoginLogModel
from src.services import UserService

try:
    from redis.asyncio import Redis
except ImportError:  # pragma: no cover - optional dependency
    Redis = None  # type: ignore

logger = logging.getLogger(__name__)

auth_bp = Blueprint('auth', __name__, url_prefix='/auth')

# token 存储：优先使用 Redis，未配置时回退内存（单进程有效）
_token_store: dict[str, dict] = {}
_redis_client: Optional["Redis"] = None

# 登录速率限制：IP -> (失败次数, 首次失败时间戳)
_login_rate_limit: dict[str, dict] = defaultdict(lambda: {'count': 0, 'first_fail': 0})
_LOGIN_RATE_WINDOW = 900  # 15 分钟窗口


def _check_login_rate_limit(ip: str) -> Optional[str]:
    """
    检查 IP 是否超过登录失败阈值
    
    :return: 如果超限则返回错误消息，否则返回 None
    """
    threshold = SecurityConfig.LOGIN_FAIL_THRESHOLD
    if threshold <= 0:
        return None
    
    now = timestamp()
    record = _login_rate_limit[ip]
    
    # 窗口过期则重置
    if now - record['first_fail'] > _LOGIN_RATE_WINDOW:
        record['count'] = 0
        record['first_fail'] = 0
        return None
    
    if record['count'] >= threshold:
        remaining = _LOGIN_RATE_WINDOW - (now - record['first_fail'])
        return f"登录尝试过于频繁，请在 {max(remaining // 60, 1)} 分钟后重试"
    
    return None


def _record_login_failure(ip: str):
    """记录一次登录失败"""
    now = timestamp()
    record = _login_rate_limit[ip]
    if record['count'] == 0 or now - record['first_fail'] > _LOGIN_RATE_WINDOW:
        record['count'] = 1
        record['first_fail'] = now
    else:
        record['count'] += 1


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
        'success': success,
        'message': message,
        'data': data,
        'timestamp': timestamp(),
    }
    return jsonify(response), code




def _token_key(token: str) -> str:
    return f"tw:token:{token}"


def _user_tokens_key(uid: int) -> str:
    return f"tw:user:{uid}:tokens"


async def _get_redis() -> Optional["Redis"]:
    """延迟初始化 Redis 客户端，未配置时返回 None。"""
    global _redis_client
    if not Config.REDIS_URL:
        return None
    if Redis is None:
        logger.warning("检测到 REDIS_URL 但未安装 redis 依赖，回退为内存 token 存储")
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
                'uid': int(data['uid']),
                'created_at': int(data['created_at']),
                'expires_at': int(data['expires_at']),
            }
        except (KeyError, ValueError):
            await redis_client.delete(_token_key(token))
            return None
        except Exception as exc:  # pragma: no cover - redis 挂掉时回退
            logger.warning("Redis token store 读取失败，回退内存：%s", exc)
    return _token_store.get(token)


async def _store_token(token: str, uid: int) -> dict:
    payload = {
        'uid': uid,
        'created_at': timestamp(),
        'expires_at': timestamp() + APIConfig.TOKEN_EXPIRE,
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
            logger.warning("Redis token store 写入失败，回退内存：%s", exc)
            _token_store[token] = payload
            return payload
    else:
        _token_store[token] = payload
    return payload

def require_auth(f: Callable) -> Callable:
    """要求认证的装饰器（管理员可选认证）"""
    @wraps(f)
    async def wrapper(*args, **kwargs):
        # 从请求头获取 token
        auth_header = request.headers.get('Authorization', '')
        
        # 如果没有提供 token，尝试作为公开接口处理
        if not auth_header or not auth_header.startswith('Bearer '):
            # 没有认证，设置 g.current_user 为 None，让接口自行决定是否需要认证
            g.current_user = None
            return await f(*args, **kwargs)
        
        token = auth_header[7:]  # 移除 "Bearer " 前缀
        
        # 验证 token 格式（应该是 64 位十六进制字符串）
        if len(token) != 64 or not all(c in '0123456789abcdef' for c in token):
            return api_response(False, "认证令牌格式无效", code=401)
        
        # 验证 token
        token_data = await _load_token(token)
        if not token_data:
            return api_response(False, "认证令牌无效或已过期", code=401)
        
        # 检查 token 是否过期
        if timestamp() > token_data['expires_at']:
            # 清理过期 token
            await revoke_token(token, token_data.get('uid'))
            return api_response(False, "认证令牌已过期", code=401)
        
        # 获取用户
        user = await UserOperate.get_user_by_uid(token_data['uid'])
        if not user:
            await revoke_token(token, token_data.get('uid'))
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
        if not hasattr(g, 'current_user') or g.current_user is None:
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
            return
        except Exception as exc:  # pragma: no cover
            logger.warning("Redis token 撤销失败，回退内存：%s", exc)
    _token_store.pop(token, None)


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
            return
        except Exception as exc:  # pragma: no cover
            logger.warning("Redis 批量撤销 token 失败，回退内存：%s", exc)
    tokens_to_remove = [
        token for token, data in _token_store.items()
        if data.get('uid') == uid
    ]
    for token in tokens_to_remove:
        _token_store.pop(token, None)


# ==================== 登录相关 ====================

@auth_bp.route('/login', methods=['POST'])
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
    username = data.get('username', '').strip()
    password = data.get('password', '')
    
    if not username or not password:
        return api_response(False, "缺少用户名或密码", code=400)
    
    # 输入验证
    if len(username) > 50:
        return api_response(False, "用户名过长", code=400)
    
    if len(password) > 200:
        return api_response(False, "密码过长", code=400)
    
    # IP 速率限制检查
    client_ip = request.remote_addr or 'unknown'
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
    user.LAST_LOGIN_UA = request.headers.get('User-Agent', 'unknown')
    await UserOperate.update_user(user)
    
    # 记录登录成功
    await _log_login_attempt(username, True, "登录成功")
    
    # 生成 token（快速操作）
    token = await generate_token(user.UID)
    
    # 快速返回基本用户信息，不阻塞登录
    basic_user_info = {
        'uid': user.UID,
        'username': user.USERNAME,
        'email': user.EMAIL,
        'role': user.ROLE,
        'active': user.ACTIVE_STATUS,
    }
    
    # 异步后台任务：同步 Emby 状态和获取完整用户信息（不阻塞登录）
    async def sync_background_tasks():
        try:
            # 同步用户到 Emby
            await UserService.sync_user_to_emby(user)
        except Exception as e:
            logger.warning(f"后台同步用户状态到 Emby 失败: {e}")
    
    # 在后台运行，不等待
    try:
        import asyncio
        loop = asyncio.get_running_loop()
        loop.create_task(sync_background_tasks())
    except:
        pass
    
    return api_response(True, "登录成功", {
        'token': token,
        'user': basic_user_info,
    })


@auth_bp.route('/login/telegram', methods=['POST'])
async def login_telegram():
    """
    通过 Telegram ID 登录
    
    Request:
        {
            "telegram_id": 123456789
        }
    """
    # IP 速率限制检查
    client_ip = request.remote_addr or 'unknown'
    rate_limit_msg = _check_login_rate_limit(client_ip)
    if rate_limit_msg:
        return api_response(False, rate_limit_msg, code=429)

    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    
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
    user.LAST_LOGIN_UA = request.headers.get('User-Agent', 'unknown')
    await UserOperate.update_user(user)
    
    # 异步后台同步用户状态到 Emby（不阻塞登录）
    import asyncio
    from src.services import UserService
    async def sync_emby_async():
        try:
            await UserService.sync_user_to_emby(user)
        except Exception as e:
            import logging
            logger = logging.getLogger(__name__)
            logger.warning(f"同步用户状态到 Emby 失败: {e}")
    
    asyncio.create_task(sync_emby_async())
    
    # 生成 token
    token = await generate_token(user.UID)
    
    # 获取用户信息
    user_info = await UserService.get_user_info(user)
    
    return api_response(True, "登录成功", {
        'token': token,
        'user': user_info,
    })


@auth_bp.route('/login/apikey', methods=['POST'])
async def login_apikey():
    """
    通过 API Key 登录/验证
    
    Request:
        {
            "apikey": "key-xxxxx-xxxxx"
        }
    """
    # IP 速率限制检查
    client_ip = request.remote_addr or 'unknown'
    rate_limit_msg = _check_login_rate_limit(client_ip)
    if rate_limit_msg:
        return api_response(False, rate_limit_msg, code=429)

    data = request.get_json() or {}
    apikey = data.get('apikey')
    
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
    
    return api_response(True, "验证成功", {
        'token': token,
        'user': user_info,
    })


# ==================== 登出相关 ====================

@auth_bp.route('/logout', methods=['POST'])
@require_auth
async def logout():
    """登出当前设备"""
    await revoke_token(g.token, getattr(g.current_user, 'UID', None))
    return api_response(True, "登出成功")


@auth_bp.route('/logout/all', methods=['POST'])
@require_auth
async def logout_all():
    """登出所有设备"""
    await revoke_user_tokens(g.current_user.UID)
    return api_response(True, "已登出所有设备")


# ==================== 用户信息 ====================

@auth_bp.route('/me', methods=['GET'])
@require_auth
async def get_me():
    """获取当前用户信息"""
    from src.services import UserService
    user_info = await UserService.get_user_info(g.current_user)
    return api_response(True, "获取成功", user_info)


# ==================== Token 刷新 ====================

@auth_bp.route('/refresh', methods=['POST'])
@require_auth
async def refresh_token():
    """刷新 Token"""
    # 撤销旧 token
    await revoke_token(g.token, g.current_user.UID)
    
    # 生成新 token
    new_token = await generate_token(g.current_user.UID)
    
    return api_response(True, "刷新成功", {
        'token': new_token,
    })


# ==================== API Key 管理 ====================

@auth_bp.route('/apikey', methods=['GET'])
@require_auth
async def get_apikey_status():
    """获取 API Key 状态"""
    return api_response(True, "获取成功", {
        'enabled': g.current_user.APIKEY_STATUS,
        'apikey': g.current_user.APIKEY if g.current_user.APIKEY_STATUS else None,
    })


@auth_bp.route('/apikey', methods=['POST'])
@require_auth
async def generate_apikey():
    """生成新 API Key"""
    new_apikey = await UserOperate.reset_apikey(g.current_user)
    
    # 重新获取用户（更新后的 API Key）
    user = await UserOperate.get_user_by_uid(g.current_user.UID)
    
    return api_response(True, "API Key 生成成功", {
        'apikey': new_apikey,
        'enabled': True,
    })


@auth_bp.route('/apikey', methods=['DELETE'])
@require_auth
async def disable_apikey():
    """禁用 API Key"""
    await UserOperate.set_apikey_status(g.current_user.UID, False)
    return api_response(True, "API Key 已禁用")


@auth_bp.route('/apikey/enable', methods=['POST'])
@require_auth
async def enable_apikey():
    """启用 API Key（如果不存在则生成）"""
    if not g.current_user.APIKEY or not g.current_user.APIKEY_STATUS:
        # 生成新的 API Key
        new_apikey = await UserOperate.reset_apikey(g.current_user)
        return api_response(True, "API Key 已生成并启用", {
            'apikey': new_apikey,
            'enabled': True,
        })
    else:
        # 启用现有的 API Key
        await UserOperate.set_apikey_status(g.current_user.UID, True)
        return api_response(True, "API Key 已启用", {
            'apikey': g.current_user.APIKEY,
            'enabled': True,
        })


@auth_bp.route('/apikey/permissions', methods=['GET'])
@require_auth
async def get_apikey_permissions():
    """获取 API Key 的权限列表"""
    import json
    from src.api.v1.apikey import ALL_PERMISSIONS, _get_user_permissions
    
    return api_response(True, "获取成功", {
        'permissions': _get_user_permissions(g.current_user),
        'all_permissions': ALL_PERMISSIONS,
    })


@auth_bp.route('/apikey/permissions', methods=['PUT'])
@require_auth
async def update_apikey_permissions():
    """更新 API Key 的权限列表"""
    import json
    from src.api.v1.apikey import ALL_PERMISSIONS
    
    data = request.get_json() or {}
    permissions = data.get('permissions')
    
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
    
    return api_response(True, "权限已更新", {
        'permissions': permissions,
    })


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
            EMBY_USER_ID=user.EMBYID or '',
            IP_ADDRESS=request.remote_addr or 'unknown',
            DEVICE_NAME=request.headers.get('User-Agent', 'unknown')[:200],  # 限制长度
            LOGIN_TIME=timestamp(),
            IS_BLOCKED=not success,  # 登录失败时标记为被拦截
        )
        await LoginLogOperate.add_log(log)
    except Exception as e:
        # 记录失败不影响登录流程
        import logging
        logging.getLogger(__name__).warning(f"记录登录日志失败: {e}")


__all__ = [
    'auth_bp',
    'require_auth',
    'require_admin',
    'api_response',
    'generate_token',
    'revoke_token',
    'revoke_user_tokens',
]
