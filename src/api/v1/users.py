"""
用户 API

提供用户相关的 CRUD 操作
"""
import logging
from flask import Blueprint, request, g

from src.api.v1.auth import async_route, require_auth, api_response
from src.db.user import UserOperate, Role
from src.services import UserService

logger = logging.getLogger(__name__)

users_bp = Blueprint('users', __name__, url_prefix='/users')


# ==================== 用户注册 ====================

@users_bp.route('/register', methods=['POST'])
@async_route
async def register():
    """
    用户注册
    
    Request:
        {
            "username": "myusername",       // 必填
            "password": "mypassword",       // Web 端必填，Telegram 端可选（自动生成）
            "telegram_id": 123456789,       // 可选，Telegram 用户 ID
            "reg_code": "code-xxx",         // 注册码注册
            "email": "user@example.com",    // 可选
            "use_score": false              // 是否使用积分注册
        }
    
    Response:
        {
            "success": true,
            "data": {
                "username": "myusername",
                "password": "密码（仅自动生成时返回）",
                "user": { ... }
            }
        }
    """
    from src.config import Config
    
    data = request.get_json() or {}
    
    telegram_id = data.get('telegram_id')
    username = data.get('username')
    password = data.get('password')  # Web 端用户设置的密码
    reg_code = data.get('reg_code')
    email = data.get('email')
    use_score = data.get('use_score', False)
    
    # 验证必要参数
    if not username:
        return api_response(False, "缺少用户名", code=400)
    
    # 如果强制绑定 Telegram 但未提供 telegram_id
    if Config.FORCE_BIND_TELEGRAM and not telegram_id:
        return api_response(False, "系统要求绑定 Telegram，请提供 telegram_id", code=400)
    
    # Web 端注册：没有 telegram_id 时必须提供密码
    if not telegram_id and not password:
        return api_response(False, "请设置密码", code=400)
    
    # 密码长度验证
    if password and len(password) < 6:
        return api_response(False, "密码长度至少 6 位", code=400)
    
    # 用户名验证
    from src.core.utils import is_valid_username
    if not is_valid_username(username):
        return api_response(False, "用户名格式不正确（3-20位字母数字下划线，不能以数字开头）", code=400)
    
    # 邮箱验证
    if email:
        from src.core.utils import is_valid_email
        if not is_valid_email(email):
            return api_response(False, "邮箱格式不正确", code=400)
    
    # 注册
    if use_score:
        result = await UserService.register_by_score(telegram_id, username, email, password)
    elif reg_code:
        result = await UserService.register_by_code(telegram_id, username, reg_code, email, password)
    else:
        # 无码注册（待激活状态，只能签到）
        result = await UserService.register_pending(telegram_id, username, email, password)
    
    if result.result.value == 'success':
        user_info = await UserService.get_user_info(result.user) if result.user else None
        return api_response(True, result.message, {
            'username': result.user.USERNAME if result.user else None,
            'password': result.emby_password if not password else None,  # 仅自动生成时返回
            'user': user_info,
        })
    
    return api_response(False, result.message, code=400)


@users_bp.route('/check-available', methods=['GET'])
@async_route
async def check_registration_available():
    """
    检查是否可以注册
    
    Response:
        {
            "success": true,
            "data": {
                "available": true,
                "message": "可以注册",
                "current_users": 50,
                "max_users": 200
            }
        }
    """
    from src.config import ScoreAndRegisterConfig
    
    available, msg = await UserService.check_registration_available()
    current_count = await UserOperate.get_registered_users_count()
    
    return api_response(True, msg, {
        'available': available,
        'message': msg,
        'current_users': current_count,
        'max_users': ScoreAndRegisterConfig.USER_LIMIT,
        'register_mode': ScoreAndRegisterConfig.REGISTER_MODE,
        'score_register_mode': ScoreAndRegisterConfig.SCORE_REGISTER_MODE,
        'score_register_need': ScoreAndRegisterConfig.SCORE_REGISTER_NEED,
        'allow_pending_register': ScoreAndRegisterConfig.ALLOW_PENDING_REGISTER,
    })


@users_bp.route('/me/activate', methods=['POST'])
@async_route
@require_auth
async def activate_my_account():
    """
    激活待激活账户（使用积分创建 Emby 账户）
    
    需要足够的积分才能激活。
    
    Response:
        {
            "success": true,
            "message": "账户激活成功！Emby 密码: xxx，有效期 30 天"
        }
    """
    user = g.current_user
    
    # 检查是否已激活
    if user.EMBYID or user.ACTIVE_STATUS:
        return api_response(False, "账户已激活", code=400)
    
    success, message = await UserService.activate_pending_user(user)
    
    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


# ==================== 用户信息 ====================

@users_bp.route('/me', methods=['GET'])
@async_route
@require_auth
async def get_my_info():
    """获取当前用户详细信息"""
    user_info = await UserService.get_user_info(g.current_user)
    
    # 获取 Emby 状态
    from src.services import EmbyService
    status = await EmbyService.get_user_status(g.current_user)
    
    user_info['emby_status'] = {
        'is_synced': status.is_synced,
        'is_active': status.is_active,
        'active_sessions': status.active_sessions,
        'message': status.message,
    }
    
    return api_response(True, "获取成功", user_info)


@users_bp.route('/me', methods=['PUT'])
@async_route
@require_auth
async def update_my_info():
    """
    更新当前用户信息
    
    Request:
        {
            "email": "new@example.com"
        }
    """
    data = request.get_json() or {}
    user = g.current_user
    
    # 更新邮箱
    if 'email' in data:
        email = data['email']
        if email:
            from src.core.utils import is_valid_email
            if not is_valid_email(email):
                return api_response(False, "邮箱格式不正确", code=400)
        user.EMAIL = email
        await UserOperate.update_user(user)
    
    user_info = await UserService.get_user_info(user)
    return api_response(True, "更新成功", user_info)


@users_bp.route('/me/username', methods=['PUT'])
@async_route
@require_auth
async def change_my_username():
    """
    修改用户名
    
    Request:
        {
            "new_username": "newname"
        }
    """
    data = request.get_json() or {}
    new_username = data.get('new_username')
    
    if not new_username:
        return api_response(False, "缺少 new_username", code=400)
    
    from src.core.utils import is_valid_username
    if not is_valid_username(new_username):
        return api_response(False, "用户名格式不正确", code=400)
    
    success, message = await UserService.change_username(g.current_user, new_username)
    return api_response(success, message)


@users_bp.route('/me/password', methods=['PUT'])
@async_route
@require_auth
async def reset_my_password():
    """重置密码"""
    success, message, new_password = await UserService.reset_password(g.current_user)
    
    if success:
        return api_response(True, message, {'new_password': new_password})
    return api_response(False, message)


@users_bp.route('/me/nsfw', methods=['GET'])
@async_route
@require_auth
async def get_nsfw_status():
    """
    获取 NSFW 权限状态
    
    Response:
        {
            "success": true,
            "data": {
                "enabled": true,           // 用户是否已开启 NSFW
                "has_permission": true,    // 用户在 Emby 中是否有 NSFW 库访问权限
                "nsfw_library_id": "xxx",  // NSFW 库 ID
                "can_toggle": true         // 用户是否可以切换（有权限才能切换）
            }
        }
    """
    from src.services import EmbyService
    from src.config import EmbyConfig
    
    user = g.current_user
    nsfw_library_id = EmbyConfig.EMBY_NSFW
    
    # 检查是否配置了 NSFW 库
    if not nsfw_library_id:
        return api_response(True, "NSFW 库未配置", {
            'enabled': False,
            'has_permission': False,
            'nsfw_library_id': None,
            'can_toggle': False,
            'message': '系统未配置 NSFW 媒体库'
        })
    
    # 获取用户在 Emby 中的媒体库访问权限
    library_ids, enable_all = await EmbyService.get_user_library_access(user)
    
    # 判断用户是否有 NSFW 库访问权限
    has_permission = enable_all or (nsfw_library_id in library_ids)
    
    return api_response(True, "获取成功", {
        'enabled': user.NSFW,
        'has_permission': has_permission,
        'nsfw_library_id': nsfw_library_id,
        'can_toggle': has_permission,
        'message': '有访问权限，可自行开关' if has_permission else '您没有 NSFW 库的访问权限，请联系管理员'
    })


@users_bp.route('/me/emby/bind', methods=['POST'])
@async_route
@require_auth
async def bind_emby_account():
    """
    绑定已有的 Emby 账号（需要验证用户名和密码）
    
    Request:
        {
            "emby_username": "existing_username",  // Emby 用户名
            "emby_password": "password"           // Emby 密码
        }
    
    Response:
        {
            "success": true,
            "message": "绑定成功",
            "data": {
                "emby_id": "xxx",
                "emby_username": "existing_username"
            }
        }
    """
    from src.services.emby import get_emby_client, EmbyError
    
    data = request.get_json() or {}
    
    # 尝试多种可能的字段名
    emby_username = (
        data.get('emby_username') or 
        data.get('username') or 
        data.get('embyUsername') or 
        ''
    )
    if isinstance(emby_username, str):
        emby_username = emby_username.strip()
    else:
        emby_username = ''
    
    emby_password = (
        data.get('emby_password') or 
        data.get('password') or 
        data.get('embyPassword') or 
        ''
    )
    if isinstance(emby_password, str):
        emby_password = emby_password.strip()
    else:
        emby_password = ''
    
    # 调试日志
    logger.debug(f"绑定 Emby 账号请求: username={emby_username}, password_length={len(emby_password)}, data_keys={list(data.keys())}")
    
    if not emby_username:
        return api_response(False, "请输入 Emby 用户名", code=400)
    
    if not emby_password:
        logger.warning(f"密码为空: data keys={list(data.keys())}, emby_password value={repr(data.get('emby_password'))}, password value={repr(data.get('password'))}")
        return api_response(False, "请输入 Emby 密码", code=400)
    
    user = g.current_user
    
    # 检查用户是否已绑定 Emby 账号
    if user.EMBYID:
        return api_response(False, "您已绑定 Emby 账号，请先解绑", code=400)
    
    # 检查用户名是否已被其他用户使用
    existing_user = await UserOperate.get_user_by_username(emby_username)
    if existing_user and existing_user.UID != user.UID:
        return api_response(False, "该用户名已被其他用户使用", code=400)
    
    # 验证 Emby 用户名和密码
    emby = get_emby_client()
    try:
        # 首先验证用户名和密码
        emby_user = await emby.authenticate_by_name(emby_username, emby_password)
        if not emby_user:
            return api_response(False, "用户名或密码错误", code=401)
        
        # 验证用户名是否匹配
        if emby_user.name.lower() != emby_username.lower():
            return api_response(False, "用户名不匹配", code=400)
        
        # 检查该 Emby 账号是否已被其他本地用户绑定
        existing_bind = await UserOperate.get_user_by_embyid(emby_user.id)
        if existing_bind and existing_bind.UID != user.UID:
            return api_response(False, "该 Emby 账号已被其他用户绑定", code=400)
        
        # 绑定账号
        user.EMBYID = emby_user.id
        user.USERNAME = emby_username
        
        # 如果是管理员或白名单，保持永久有效期
        if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
            user.EXPIRED_AT = 253402214400  # 9999-12-31
        # 如果用户是未注册状态，更新为普通用户
        elif user.ROLE == Role.UNRECOGNIZED.value:
            user.ROLE = Role.NORMAL.value
        
        # 如果用户是待激活状态，激活用户
        if not user.ACTIVE_STATUS:
            user.ACTIVE_STATUS = True
            # 如果不是管理员/白名单且没有到期时间，设置默认30天
            if user.EXPIRED_AT == -1 and user.ROLE == Role.NORMAL.value:
                from src.core.utils import days_to_seconds, timestamp
                user.EXPIRED_AT = timestamp() + days_to_seconds(30)
        
        await UserOperate.update_user(user)
        
        logger.info(f"用户绑定 Emby 账号成功: {user.USERNAME} -> {emby_username} (ID: {emby_user.id})")
        
        return api_response(True, "绑定成功", {
            'emby_id': emby_user.id,
            'emby_username': emby_username,
        })
        
    except EmbyError as e:
        logger.error(f"绑定 Emby 账号失败: {e}")
        return api_response(False, f"绑定失败: {e}", code=500)
    except Exception as e:
        logger.error(f"绑定 Emby 账号失败: {e}")
        return api_response(False, f"绑定失败: {e}", code=500)


@users_bp.route('/me/emby/unbind', methods=['POST'])
@async_route
@require_auth
async def unbind_emby_account():
    """
    解绑 Emby 账号
    
    注意：解绑后用户将无法访问 Emby，但不会删除 Emby 中的账号
    """
    user = g.current_user
    
    if not user.EMBYID:
        return api_response(False, "您未绑定 Emby 账号", code=400)
    
    # 解绑（不清除 Emby 账号，只清除本地关联）
    old_emby_id = user.EMBYID
    user.EMBYID = None
    # 不修改用户名，保留原用户名
    await UserOperate.update_user(user)
    
    logger.info(f"用户解绑 Emby 账号: {user.USERNAME} (原 Emby ID: {old_emby_id})")
    
    return api_response(True, "解绑成功")


@users_bp.route('/me/nsfw', methods=['PUT'])
@async_route
@require_auth
async def toggle_my_nsfw():
    """
    切换 NSFW 库访问权限
    
    只有在 Emby 中有 NSFW 库访问权限的用户才能切换。
    此设置控制用户是否"显示" NSFW 内容，而非权限本身。
    
    Request:
        {
            "enable": true
        }
    """
    from src.services import EmbyService
    from src.config import EmbyConfig
    
    data = request.get_json() or {}
    enable = data.get('enable', False)
    user = g.current_user
    
    nsfw_library_id = EmbyConfig.EMBY_NSFW
    
    # 检查是否配置了 NSFW 库
    if not nsfw_library_id:
        return api_response(False, "系统未配置 NSFW 媒体库", code=400)
    
    # 获取用户在 Emby 中的媒体库访问权限
    library_ids, enable_all = await EmbyService.get_user_library_access(user)
    
    # 判断用户是否有 NSFW 库访问权限
    has_permission = enable_all or (nsfw_library_id in library_ids)
    
    if not has_permission:
        return api_response(False, "您没有 NSFW 库的访问权限，无法切换此选项", code=403)
    
    # 有权限，执行切换
    success, message = await UserService.toggle_nsfw(user, enable)
    return api_response(success, message)


# ==================== 用户续期 ====================

@users_bp.route('/regcode/check', methods=['POST'])
@async_route
async def check_regcode():
    """
    检查注册码类型
    
    Request:
        {
            "reg_code": "code-xxx"
        }
    
    Response:
        {
            "success": true,
            "data": {
                "type": 1,  // 1=注册, 2=续期, 3=白名单
                "type_name": "注册",
                "days": 30,
                "valid": true
            }
        }
    """
    from src.db.regcode import RegCodeOperate
    
    data = request.get_json() or {}
    reg_code = data.get('reg_code', '').strip()
    
    if not reg_code:
        return api_response(False, "缺少注册码", code=400)
    
    code_info = await RegCodeOperate.get_regcode_by_code(reg_code)
    
    if not code_info:
        return api_response(False, "注册码不存在", code=404)
    
    if not code_info.ACTIVE:
        return api_response(False, "注册码已禁用", code=400)
    
    # 检查是否已用完
    if code_info.USE_COUNT_LIMIT > 0 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
        return api_response(False, "注册码已用完", code=400)
    
    type_names = {1: '注册', 2: '续期', 3: '白名单'}
    
    return api_response(True, "注册码有效", {
        'type': code_info.TYPE,
        'type_name': type_names.get(code_info.TYPE, '未知'),
        'days': code_info.DAYS or 30,
        'valid': True,
    })


@users_bp.route('/me/renew', methods=['POST'])
@async_route
@require_auth
async def renew_my_account():
    """
    使用续期码续期
    
    Request:
        {
            "reg_code": "code-xxx"
        }
    """
    data = request.get_json() or {}
    reg_code = data.get('reg_code')
    
    if not reg_code:
        return api_response(False, "缺少续期码", code=400)
    
    success, message = await UserService.renew_user(g.current_user, 30, reg_code)
    
    if success:
        user_info = await UserService.get_user_info(g.current_user)
        return api_response(True, message, {
            'expire_status': user_info['expire_status'],
            'expired_at': user_info['expired_at'],
        })
    return api_response(False, message)


# ==================== 用户设备 ====================

@users_bp.route('/me/devices', methods=['GET'])
@async_route
@require_auth
async def get_my_devices():
    """获取我的设备列表"""
    from src.services import EmbyService
    devices = await EmbyService.get_user_devices(g.current_user)
    return api_response(True, "获取成功", devices)


@users_bp.route('/me/devices/<device_id>', methods=['DELETE'])
@async_route
@require_auth
async def remove_my_device(device_id: str):
    """移除我的设备"""
    from src.services import EmbyService
    success, message = await EmbyService.remove_user_device(g.current_user, device_id)
    return api_response(success, message)


# ==================== 用户媒体库 ====================

@users_bp.route('/me/libraries', methods=['GET'])
@async_route
@require_auth
async def get_my_libraries():
    """获取我可访问的媒体库"""
    from src.services import EmbyService
    library_ids, enable_all = await EmbyService.get_user_library_access(g.current_user)
    
    # 获取媒体库详情
    all_libraries = await EmbyService.get_libraries_info()
    
    if enable_all:
        my_libraries = all_libraries
    else:
        my_libraries = [lib for lib in all_libraries if lib['id'] in library_ids]
    
    return api_response(True, "获取成功", {
        'enable_all': enable_all,
        'libraries': my_libraries,
    })


# ==================== 用户会话 ====================

@users_bp.route('/me/sessions', methods=['GET'])
@async_route
@require_auth
async def get_my_sessions():
    """获取我的活动会话"""
    from src.services import get_emby_client
    
    if not g.current_user.EMBYID:
        return api_response(True, "获取成功", [])
    
    emby = get_emby_client()
    sessions = await emby.get_user_sessions(g.current_user.EMBYID)
    
    return api_response(True, "获取成功", [{
        'id': s.id,
        'client': s.client,
        'device_name': s.device_name,
        'is_active': s.is_active,
        'now_playing': s.now_playing_item.get('Name') if s.now_playing_item else None,
    } for s in sessions])


# ==================== 用户登录历史 ====================

@users_bp.route('/me/login-history', methods=['GET'])
@async_route
@require_auth
async def get_my_login_history():
    """获取我的登录信息"""
    user = g.current_user
    
    return api_response(True, "获取成功", {
        'last_login_time': user.LAST_LOGIN_TIME,
        'last_login_ip': user.LAST_LOGIN_IP[:3] + '***' if user.LAST_LOGIN_IP else None,  # 部分隐藏 IP
        'last_login_ua': user.LAST_LOGIN_UA,
    })


# ==================== Telegram 绑定管理 ====================

@users_bp.route('/me/telegram', methods=['GET'])
@async_route
@require_auth
async def get_telegram_status():
    """
    获取 Telegram 绑定状态
    
    Response:
        {
            "success": true,
            "data": {
                "bound": true,
                "telegram_id": 123456789,  // 部分隐藏
                "force_bind": true,        // 系统是否强制绑定 TG
                "can_unbind": false,       // 是否可以解绑（强制绑定时不可解绑）
                "can_change": true         // 是否可以换绑
            }
        }
    """
    from src.config import Config
    
    user = g.current_user
    force_bind = Config.FORCE_BIND_TELEGRAM
    
    # 隐藏部分 Telegram ID
    masked_id = None
    if user.TELEGRAM_ID:
        id_str = str(user.TELEGRAM_ID)
        if len(id_str) > 4:
            masked_id = id_str[:3] + '****' + id_str[-2:]
        else:
            masked_id = '****'
    
    # 尝试获取 Telegram 用户名
    telegram_username = None
    if user.TELEGRAM_ID:
        try:
            from src.bot.bot import get_bot_instance
            bot = get_bot_instance()
            if bot and bot.app:
                try:
                    tg_user = await bot.app.get_users(user.TELEGRAM_ID)
                    telegram_username = tg_user.username or f"{tg_user.first_name or ''} {tg_user.last_name or ''}".strip() or None
                except Exception:
                    pass  # 如果获取失败，忽略
        except Exception:
            pass  # Bot 未初始化或获取失败，忽略
    
    return api_response(True, "获取成功", {
        'bound': bool(user.TELEGRAM_ID),
        'telegram_id': masked_id,
        'telegram_id_full': user.TELEGRAM_ID,  # 完整 ID（用于前端判断）
        'telegram_username': telegram_username,  # Telegram 用户名
        'force_bind': force_bind,
        'can_unbind': not force_bind and bool(user.TELEGRAM_ID),
        'can_change': bool(user.TELEGRAM_ID),  # 已绑定才能换绑
    })


@users_bp.route('/me/telegram/bind', methods=['POST'])
@async_route
@require_auth
async def bind_my_telegram():
    """
    绑定 Telegram 账号
    
    Request:
        {
            "telegram_id": 123456789
        }
    """
    user = g.current_user
    data = request.get_json() or {}
    telegram_id = data.get('telegram_id')
    
    if not telegram_id:
        return api_response(False, "缺少 telegram_id", code=400)
    
    # 检查是否已绑定其他 Telegram
    if user.TELEGRAM_ID and user.TELEGRAM_ID != telegram_id:
        return api_response(False, "您已绑定其他 Telegram 账号，请先解绑或使用换绑功能", code=400)
    
    # 检查该 Telegram ID 是否已被其他用户绑定
    existing = await UserOperate.get_user_by_telegram_id(telegram_id)
    if existing and existing.UID != user.UID:
        return api_response(False, "该 Telegram 账号已被其他用户绑定", code=400)
    
    # 绑定
    user.TELEGRAM_ID = telegram_id
    await UserOperate.update_user(user)
    
    return api_response(True, "Telegram 绑定成功", {
        'telegram_id': telegram_id,
    })


@users_bp.route('/me/telegram/unbind', methods=['POST'])
@async_route
@require_auth
async def unbind_my_telegram():
    """
    解绑 Telegram 账号
    
    注意：如果系统强制要求绑定 Telegram，则不允许解绑
    """
    from src.config import Config
    
    user = g.current_user
    
    # 检查是否强制绑定
    if Config.FORCE_BIND_TELEGRAM:
        return api_response(False, "系统要求必须绑定 Telegram，不允许解绑。如需更换账号请使用换绑功能", code=403)
    
    # 检查是否已绑定
    if not user.TELEGRAM_ID:
        return api_response(False, "您尚未绑定 Telegram", code=400)
    
    old_telegram_id = user.TELEGRAM_ID
    user.TELEGRAM_ID = None
    await UserOperate.update_user(user)
    
    return api_response(True, "Telegram 已解绑", {
        'old_telegram_id': old_telegram_id,
    })


@users_bp.route('/me/telegram/change', methods=['POST'])
@async_route
@require_auth
async def change_my_telegram():
    """
    换绑 Telegram 账号
    
    Request:
        {
            "new_telegram_id": 987654321
        }
    """
    user = g.current_user
    data = request.get_json() or {}
    new_telegram_id = data.get('new_telegram_id')
    
    if not new_telegram_id:
        return api_response(False, "缺少 new_telegram_id", code=400)
    
    # 检查是否已绑定
    if not user.TELEGRAM_ID:
        return api_response(False, "您尚未绑定 Telegram，请使用绑定功能", code=400)
    
    # 检查新 ID 是否与旧 ID 相同
    if user.TELEGRAM_ID == new_telegram_id:
        return api_response(False, "新 Telegram ID 与当前绑定的相同", code=400)
    
    # 检查新 Telegram ID 是否已被其他用户绑定
    existing = await UserOperate.get_user_by_telegram_id(new_telegram_id)
    if existing and existing.UID != user.UID:
        return api_response(False, "该 Telegram 账号已被其他用户绑定", code=400)
    
    old_telegram_id = user.TELEGRAM_ID
    user.TELEGRAM_ID = new_telegram_id
    await UserOperate.update_user(user)
    
    return api_response(True, "Telegram 换绑成功", {
        'old_telegram_id': old_telegram_id,
        'new_telegram_id': new_telegram_id,
    })


# ==================== 自动续期 ====================

@users_bp.route('/me/auto-renew', methods=['GET'])
@async_route
@require_auth
async def get_auto_renew_status():
    """获取自动续期状态"""
    from src.services.auto_renew_service import AutoRenewService
    from src.config import ScoreAndRegisterConfig
    
    config = await AutoRenewService.get_auto_renew_info()
    
    return api_response(True, "获取成功", {
        'enabled': g.current_user.AUTO_RENEW,
        'system_enabled': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
        'config': config,
    })


@users_bp.route('/me/auto-renew', methods=['PUT'])
@async_route
@require_auth
async def set_auto_renew():
    """
    设置自动续期开关
    
    Request:
        {
            "enabled": true
        }
    """
    from src.services.auto_renew_service import AutoRenewService
    
    data = request.get_json() or {}
    enabled = data.get('enabled', False)
    
    success, message = await AutoRenewService.set_user_auto_renew(g.current_user.UID, enabled)
    
    if success:
        return api_response(True, message, {'auto_renew': enabled})
    return api_response(False, message)


# ==================== 用户积分续期 ====================

@users_bp.route('/me/renew-by-score', methods=['POST'])
@async_route
@require_auth
async def renew_by_score():
    """
    使用积分手动续期
    
    Request:
        {
            "days": 30  // 可选，默认使用配置的天数
        }
    """
    from src.config import ScoreAndRegisterConfig
    from src.db.score import ScoreOperate
    
    if not ScoreAndRegisterConfig.AUTO_RENEW_ENABLED:
        return api_response(False, "积分续期功能未启用", code=403)
    
    data = request.get_json() or {}
    days = data.get('days', ScoreAndRegisterConfig.AUTO_RENEW_DAYS)
    cost = ScoreAndRegisterConfig.AUTO_RENEW_COST
    
    # 检查积分
    score = await ScoreOperate.get_score_by_uid(g.current_user.UID)
    if not score or score.SCORE < cost:
        return api_response(False, f"积分不足，需要 {cost} {ScoreAndRegisterConfig.SCORE_NAME}", code=400)
    
    # 扣除积分
    score.SCORE -= cost
    
    # 更新累计消费
    if hasattr(score, 'TOTAL_SPENT'):
        score.TOTAL_SPENT = (score.TOTAL_SPENT or 0) + cost
    
    await ScoreOperate.update_score(score)
    
    # 续期
    success, message = await UserService.renew_user(g.current_user, days)
    
    if not success:
        # 退还积分
        score.SCORE += cost
        if hasattr(score, 'TOTAL_SPENT'):
            score.TOTAL_SPENT = (score.TOTAL_SPENT or 0) - cost
        await ScoreOperate.update_score(score)
        return api_response(False, message)
    
    # 记录积分历史
    from src.db.score import ScoreHistoryOperate
    await ScoreHistoryOperate.add_history(
        uid=g.current_user.UID,
        type_='renew',
        amount=-cost,
        balance_after=score.SCORE,
        note=f"续期 {days} 天"
    )
    
    user_info = await UserService.get_user_info(g.current_user)
    return api_response(True, f"续期成功，扣除 {cost} {ScoreAndRegisterConfig.SCORE_NAME}", {
        'days': days,
        'cost': cost,
        'remaining_score': score.SCORE,
        'expire_status': user_info['expire_status'],
    })


# ==================== 用户设置 ====================

@users_bp.route('/me/settings', methods=['GET'])
@async_route
@require_auth
async def get_my_settings():
    """获取用户所有设置"""
    from src.config import ScoreAndRegisterConfig, DeviceLimitConfig, Config, EmbyConfig
    from src.services import EmbyService
    
    user = g.current_user
    
    # 检查 NSFW 权限
    nsfw_library_id = EmbyConfig.EMBY_NSFW
    has_nsfw_permission = False
    if nsfw_library_id and user.EMBYID:
        library_ids, enable_all = await EmbyService.get_user_library_access(user)
        has_nsfw_permission = enable_all or (nsfw_library_id in library_ids)
    
    return api_response(True, "获取成功", {
        # 用户设置
        'auto_renew': user.AUTO_RENEW,
        'nsfw_enabled': user.NSFW,
        'nsfw_can_toggle': has_nsfw_permission,
        'bgm_mode': user.BGM_MODE,
        'api_key_enabled': user.APIKEY_STATUS,
        # Telegram 绑定
        'telegram': {
            'bound': bool(user.TELEGRAM_ID),
            'force_bind': Config.FORCE_BIND_TELEGRAM,
            'can_unbind': not Config.FORCE_BIND_TELEGRAM and bool(user.TELEGRAM_ID),
            'can_change': bool(user.TELEGRAM_ID),
        },
        # 系统配置
        'system_config': {
            'auto_renew_enabled': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
            'auto_renew_cost': ScoreAndRegisterConfig.AUTO_RENEW_COST,
            'auto_renew_days': ScoreAndRegisterConfig.AUTO_RENEW_DAYS,
            'device_limit_enabled': DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
            'max_devices': DeviceLimitConfig.MAX_DEVICES,
            'max_streams': DeviceLimitConfig.MAX_STREAMS,
            'nsfw_library_configured': bool(nsfw_library_id),
        },
    })

