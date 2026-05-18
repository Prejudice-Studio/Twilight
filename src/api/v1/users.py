"""
用户 API

提供用户相关的 CRUD 操作
"""
import json
import hmac
import logging
import re
from urllib.parse import urlparse
from typing import Any
import time as _time
import secrets as _secrets
import string as _string
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, api_response
from src.db.user import UserOperate, Role, TelegramBindCodeOperate
from src.services import UserService

logger = logging.getLogger(__name__)

users_bp = Blueprint('users', __name__, url_prefix='/users')


# ==================== 用户注册 ====================

@users_bp.route('/register', methods=['POST'])
async def register():
    """
    统一账号注册

    使用注册码（可选）创建系统账号；Emby 账号由首次登录后在前端 Modal 补建。

    Request:
        {
            "username": "myusername",       // 必填
            "password": "mypassword",       // Web 端必填
            "telegram_bind_code": "123456", // 全局强制绑定 Telegram 时必填
            "reg_code": "code-xxx",         // 可选；若有则按注册码授予 Emby 开通天数
            "email": "user@example.com"     // 可选
        }
    """
    from src.config import Config

    data = request.get_json() or {}

    telegram_id = data.get('telegram_id')
    telegram_bind_code = (data.get('telegram_bind_code') or '').strip().upper()
    username = data.get('username')
    password = data.get('password')
    reg_code = (data.get('reg_code') or '').strip() or None
    email = data.get('email')

    if not username:
        return api_response(False, "缺少用户名", code=400)

    # 注册时不允许直接提交 Telegram ID，必须走绑定码
    if telegram_id is not None:
        return api_response(False, "请使用 Telegram Bot 绑定码验证 Telegram ID", code=400)

    if telegram_bind_code:
        telegram_id = await _get_register_bind_telegram_id(telegram_bind_code)
        if not telegram_id:
            return api_response(
                False,
                "Telegram 绑定码无效或尚未通过 Bot 验证，请先在 Bot 中完成绑定",
                code=400,
            )

    if telegram_id is not None and telegram_id != '':
        if isinstance(telegram_id, str) and telegram_id.isdigit():
            telegram_id = int(telegram_id)
        if not isinstance(telegram_id, int) or telegram_id <= 0:
            return api_response(False, "telegram_id 格式无效", code=400)
    else:
        telegram_id = None

    if Config.FORCE_BIND_TELEGRAM and not telegram_id:
        return api_response(False, "系统要求绑定 Telegram，请先获取绑定码并通过 Bot 验证", code=400)

    # Web 端注册：始终要求设置密码
    if not password:
        return api_response(False, "请设置密码", code=400)
    if len(password) < 8:
        return api_response(False, "密码长度至少 8 位", code=400)

    from src.core.utils import is_valid_username
    if not is_valid_username(username):
        return api_response(
            False,
            "用户名格式不正确（3-20位字母数字下划线，不能以数字开头）",
            code=400,
        )

    if email:
        from src.core.utils import is_valid_email
        if not is_valid_email(email):
            return api_response(False, "邮箱格式不正确", code=400)

    if reg_code:
        result = await UserService.register_by_code(telegram_id, username, reg_code, email, password)
    else:
        result = await UserService.register_pending(telegram_id, username, email, password)

    if result.result.value == 'success':
        if telegram_bind_code:
            await _delete_bind_code(telegram_bind_code)

        user_info = await UserService.get_user_info(result.user) if result.user else None
        return api_response(True, result.message, {
            'username': result.user.USERNAME if result.user else None,
            'pending_emby': True,
            'user': user_info,
        })

    return api_response(False, result.message, code=400)


@users_bp.route('/me/emby/register', methods=['POST'])
@require_auth
async def complete_emby_account_for_me():
    """
    已登录用户补建 Emby 账号。

    需要当前用户 PENDING_EMBY=True 且 EMBYID 为空。
    失败会保留 PENDING_EMBY 标记，前端可重复尝试。

    Request:
        { "emby_username": "name", "emby_password": "Pwd1234X" }
    """
    user = g.current_user
    if user.EMBYID:
        return api_response(False, "您已绑定 Emby 账号，无需再次注册", code=400)
    if not getattr(user, 'PENDING_EMBY', False):
        return api_response(
            False,
            "您的账号不在待补建 Emby 状态。如果需要绑定 Emby，请联系管理员。",
            code=400,
        )

    data = request.get_json() or {}
    emby_username = (data.get('emby_username') or '').strip()
    emby_password = data.get('emby_password') or ''

    result = await UserService.complete_emby_registration(user, emby_username, emby_password)

    if result.result.value == 'success':
        user_info = await UserService.get_user_info(result.user)
        return api_response(True, result.message, {
            'user': user_info,
        })

    code = 409 if result.result.value == 'emby_exists' else 400
    return api_response(False, result.message, code=code)


@users_bp.route('/register/emby/status', methods=['GET'])
async def get_emby_register_status():
    """查询 Emby 注册队列状态。"""
    from src.services import EmbyRegisterQueueService

    request_id = (request.args.get('request_id') or '').strip()
    status_token = (request.args.get('status_token') or '').strip()

    if not request_id or not status_token:
        return api_response(False, "缺少 request_id 或 status_token", code=400)

    status = await EmbyRegisterQueueService.get_status(request_id, status_token)
    if not status:
        return api_response(False, "注册请求不存在或凭证无效", code=404)

    return api_response(True, "获取成功", status)


@users_bp.route('/check-available', methods=['GET'])
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
    from src.config import RegisterConfig
    
    available, msg = await UserService.check_registration_available()
    current_count = await UserService.get_registered_user_count()
    
    return api_response(True, msg, {
        'available': available,
        'message': msg,
        'current_users': current_count,
        'max_users': RegisterConfig.USER_LIMIT,
        'register_mode': RegisterConfig.REGISTER_MODE,
        'allow_pending_register': RegisterConfig.ALLOW_PENDING_REGISTER,
        'emby_direct_register_enabled': RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED,
        'emby_direct_register_days': RegisterConfig.EMBY_DIRECT_REGISTER_DAYS,
    })


# ==================== 用户信息 ====================

@users_bp.route('/me', methods=['GET'])
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
@require_auth
async def update_my_info():
    """
    更新当前用户信息
    
    Request:
        {
            "email": "new@example.com",
            "bgm_mode": true,
            "bgm_token": "your_bgm_access_token"
        }
    """
    from src.config import Config

    data = request.get_json() or {}
    user = g.current_user
    updated = False

    # 更新邮箱
    if 'email' in data:
        email = data['email']
        if email:
            from src.core.utils import is_valid_email
            if not is_valid_email(email):
                return api_response(False, "邮箱格式不正确", code=400)
        user.EMAIL = email
        updated = True

    # 更新 Bangumi 同步设置
    if 'bgm_token' in data:
        bgm_token = data.get('bgm_token') or ''
        user.BGM_TOKEN = bgm_token
        updated = True

    if 'bgm_mode' in data:
        bgm_mode = bool(data['bgm_mode'])
        if bgm_mode and not (user.BGM_TOKEN or Config.BANGUMI_TOKEN):
            return api_response(False, "请先设置 Bangumi Token 后启用 BGM 同步", code=400)
        user.BGM_MODE = bgm_mode
        updated = True

    if updated:
        await UserOperate.update_user(user)

    user_info = await UserService.get_user_info(user)
    return api_response(True, "更新成功", user_info)


@users_bp.route('/me/username', methods=['PUT'])
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
@require_auth
async def reset_my_password():
    """重置密码"""
    success, message, new_password = await UserService.reset_password(g.current_user)
    
    if success:
        return api_response(True, message, {'new_password': new_password})
    return api_response(False, message)


@users_bp.route('/me/password/change', methods=['POST'])
@require_auth
async def change_my_password():
    """
    修改密码（同时修改系统密码和 Emby 密码）
    
    Request:
        {
            "old_password": "current_password",
            "new_password": "new_password"
        }
    """
    data = request.get_json() or {}
    old_password = data.get('old_password', '')
    new_password = data.get('new_password', '')

    if not old_password or not new_password:
        return api_response(False, "请提供当前密码和新密码", code=400)

    ok, msg = UserService.validate_password_strength(new_password, label="新密码")
    if not ok:
        return api_response(False, msg, code=400)

    success, message = await UserService.change_password(g.current_user, old_password, new_password)

    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@users_bp.route('/me/password/system', methods=['POST'])
@require_auth
async def change_my_system_password():
    """
    修改系统登录密码（不影响 Emby 密码）

    Request:
        {
            "old_password": "current_password",
            "new_password": "new_password"
        }
    """
    data = request.get_json() or {}
    old_password = data.get('old_password', '')
    new_password = data.get('new_password', '')

    if not old_password or not new_password:
        return api_response(False, "请提供当前密码和新密码", code=400)

    ok, msg = UserService.validate_password_strength(new_password, label="新密码")
    if not ok:
        return api_response(False, msg, code=400)

    success, message = await UserService.change_system_password(g.current_user, old_password, new_password)
    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@users_bp.route('/me/password/emby', methods=['POST'])
@require_auth
async def change_my_emby_password():
    """
    修改 Emby 密码（仅更新绑定的 Emby 账户）

    Request:
        {
            "new_password": "new_password"
        }
    """
    data = request.get_json() or {}
    new_password = data.get('new_password', '')

    if not new_password:
        return api_response(False, "请提供新密码", code=400)

    ok, msg = UserService.validate_password_strength(new_password, label="新密码")
    if not ok:
        return api_response(False, msg, code=400)

    success, message = await UserService.change_emby_password(g.current_user, new_password)
    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@users_bp.route('/me/emby/bind', methods=['POST'])
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
    
    # 区分"未传字段"和"传了空字符串"——Emby 支持空密码账号
    raw_password = None
    for key in ('emby_password', 'password', 'embyPassword'):
        if key in data:
            raw_password = data[key]
            break
    if isinstance(raw_password, str):
        emby_password = raw_password
    elif raw_password is None:
        emby_password = None
    else:
        emby_password = ''

    logger.debug(
        f"绑定 Emby 账号请求: username={emby_username}, "
        f"password_provided={emby_password is not None}, "
        f"password_length={len(emby_password) if emby_password is not None else 0}, "
        f"data_keys={list(data.keys())}"
    )

    if not emby_username:
        return api_response(False, "请输入 Emby 用户名", code=400)

    if emby_password is None:
        return api_response(False, "请提供 Emby 密码字段（空密码请传空字符串）", code=400)
    
    user = g.current_user
    
    # 检查用户是否已绑定 Emby 账号
    if user.EMBYID:
        return api_response(False, "您已绑定 Emby 账号，请先解绑", code=400)
    
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
        import json
        other_data = {}
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}
        other_data['emby_username'] = emby_user.name
        user.OTHER = json.dumps(other_data)

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
        return api_response(False, "绑定失败，请检查用户名密码或稍后重试", code=500)
    except Exception as e:
        logger.error(f"绑定 Emby 账号失败: {e}")
        return api_response(False, "绑定失败，请稍后重试", code=500)


@users_bp.route('/me/emby/unbind', methods=['POST'])
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
    import json
    if user.OTHER:
        try:
            other_data = json.loads(user.OTHER)
        except (json.JSONDecodeError, TypeError):
            other_data = {}
        else:
            other_data.pop('emby_username', None)
            user.OTHER = json.dumps(other_data) if other_data else ''
    # 不修改用户名，保留原用户名
    await UserOperate.update_user(user)
    
    logger.info(f"用户解绑 Emby 账号: {user.USERNAME} (原 Emby ID: {old_emby_id})")
    
    return api_response(True, "解绑成功")


# ==================== 用户续期 ====================

@users_bp.route('/regcode/check', methods=['POST'])
async def check_regcode():
    """
    检查注册码/续期码类型

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
    from src.core.utils import rate_limit_check

    # 公开端点，按 IP 限流防止枚举注册码（10 次 / 分钟）
    client_ip = request.remote_addr or 'unknown'
    allowed, retry_after = rate_limit_check(
        'regcode_check', client_ip, max_requests=10, window_seconds=60,
    )
    if not allowed:
        return api_response(
            False, f"操作过于频繁，请在 {retry_after} 秒后重试", code=429,
        )

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
    days = UserService._normalize_code_days(code_info.DAYS, default=30)
    
    return api_response(True, "注册码有效", {
        'type': code_info.TYPE,
        'type_name': type_names.get(code_info.TYPE, '未知'),
        'days': days,
        'valid': True,
    })


@users_bp.route('/me/renew', methods=['POST'])
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


@users_bp.route('/me/use-code', methods=['POST'])
@require_auth
async def use_code():
    """
    统一使用注册码/续期码/白名单码
    
    已登录用户根据卡码类型自动处理：
    - 注册码：为无 Emby 账户的用户创建 Emby 账户
    - 续期码：续期
    - 白名单码：赋予白名单角色，自动创建 Emby 账户（如没有）
    
    Request:
        {
            "reg_code": "code-xxx",
            "emby_username": "emby_name",   // 创建 Emby 账户时必填
            "emby_password": "Password123"   // 创建 Emby 账户时必填
        }
    
    Response:
        {
            "success": true,
            "message": "操作成功",
            "data": {
                "emby_password": "xxx",  // 仅创建 Emby 账户时返回
                "expire_status": "...",
                "expired_at": 12345678
            }
        }
    """
    data = request.get_json() or {}
    reg_code = data.get('reg_code', '').strip()
    emby_username = (data.get('emby_username') or '').strip() or None

    raw_password = data.get('emby_password')
    if isinstance(raw_password, str):
        emby_password = raw_password
    elif raw_password is None:
        emby_password = None
    else:
        emby_password = ''
    
    if not reg_code:
        return api_response(False, "缺少注册码/续期码", code=400)
    
    success, message, generated_emby_password = await UserService.use_code(
        g.current_user,
        reg_code,
        emby_username=emby_username,
        emby_password=emby_password,
    )
    
    if success:
        user_info = await UserService.get_user_info(g.current_user)
        return api_response(True, message, {
            'emby_password': generated_emby_password,
            'expire_status': user_info['expire_status'],
            'expired_at': user_info['expired_at'],
            'role': user_info['role'],
            'role_name': user_info['role_name'],
        })
    return api_response(False, message, code=400)


# ==================== 用户设备 ====================

@users_bp.route('/me/devices', methods=['GET'])
@require_auth
async def get_my_devices():
    """获取我的设备列表"""
    from src.services import EmbyService
    devices = await EmbyService.get_user_devices(g.current_user)
    return api_response(True, "获取成功", devices)


@users_bp.route('/me/devices/<device_id>', methods=['DELETE'])
@require_auth
async def remove_my_device(device_id: str):
    """移除我的设备"""
    from src.services import EmbyService
    success, message = await EmbyService.remove_user_device(g.current_user, device_id)
    return api_response(success, message)


# ==================== 用户媒体库 ====================

@users_bp.route('/me/libraries', methods=['GET'])
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
            from src.services.telegram_runtime import run_bot_operation

            async def _resolve_username(bot):
                tg_user = await bot.get_chat(user.TELEGRAM_ID)
                return (
                    tg_user.username
                    or f"{tg_user.first_name or ''} {tg_user.last_name or ''}".strip()
                    or None
                )

            telegram_username = await run_bot_operation(_resolve_username, timeout=8)
        except Exception:
            pass  # Bot 未初始化或获取失败，忽略
    
    pending_request = await UserService.get_telegram_rebind_request(user.UID)
    has_pending_rebind_request = bool(pending_request and pending_request.STATUS == 'pending')
    return api_response(True, "获取成功", {
        'bound': bool(user.TELEGRAM_ID),
        'telegram_id': masked_id,
        'telegram_id_full': user.TELEGRAM_ID,  # 完整 ID（用于前端判断）
        'telegram_username': telegram_username,  # Telegram 用户名
        'force_bind': force_bind,
        'can_unbind': not force_bind and bool(user.TELEGRAM_ID),
        'can_change': bool(user.TELEGRAM_ID) and not has_pending_rebind_request,
        'pending_rebind_request': has_pending_rebind_request,
        'rebind_request_status': pending_request.STATUS if pending_request else None,
        'rebind_request_id': pending_request.ID if pending_request else None,
    })


@users_bp.route('/me/telegram/rebind-request', methods=['POST'])
@require_auth
async def create_tg_rebind_request():
    from src.config import Config

    if not Config.TELEGRAM_MODE:
        return api_response(False, "Telegram 功能未启用", code=400)

    user = g.current_user
    if not user.TELEGRAM_ID:
        return api_response(False, "当前账号尚未绑定 Telegram", code=400)

    data = request.get_json() or {}
    reason = data.get('reason')

    success, message, request_obj = await UserService.create_telegram_rebind_request(user, reason)
    if success:
        return api_response(True, message, {
            'request_id': request_obj.ID,
            'status': request_obj.STATUS,
        })
    return api_response(False, message, code=400)


@users_bp.route('/me/telegram/unbind', methods=['POST'])
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



# ==================== Telegram 绑定码 ====================

_BIND_CODE_EXPIRE = 300  # 绑定码有效期（秒）
_MAX_BIND_CODES = 20000
_BIND_SCENE_REGISTER = 'register'
_BIND_SCENE_USER = 'user'


def _detect_image_extension(header: bytes) -> str | None:
    """根据魔数识别图片扩展名。"""
    if header.startswith(b'\xff\xd8\xff'):
        return 'jpg'
    if header.startswith(b'\x89PNG\r\n\x1a\n'):
        return 'png'
    if header.startswith(b'GIF87a') or header.startswith(b'GIF89a'):
        return 'gif'
    if len(header) >= 12 and header.startswith(b'RIFF') and header[8:12] == b'WEBP':
        return 'webp'
    return None

async def _cleanup_expired_codes():
    """清理过期绑定码并维持上限。"""
    await TelegramBindCodeOperate.cleanup_expired()
    await TelegramBindCodeOperate.trim_to_max(_MAX_BIND_CODES)


async def _delete_bind_code(bind_code: str) -> None:
    await TelegramBindCodeOperate.delete_code(bind_code)


async def _get_register_bind_telegram_id(bind_code: str) -> int | None:
    """根据注册绑定码获取已确认的 Telegram ID"""
    await _cleanup_expired_codes()
    code_info = await TelegramBindCodeOperate.get_code(bind_code)
    if not code_info or code_info.SCENE != _BIND_SCENE_REGISTER:
        return None
    return code_info.CONFIRMED_TELEGRAM_ID

def _generate_bind_code() -> str:
    """生成 8 位高强度绑定码（大写字母+数字）。"""
    alphabet = _string.ascii_uppercase + _string.digits
    return ''.join(_secrets.choice(alphabet) for _ in range(8))


@users_bp.route('/me/telegram/bind-code', methods=['POST'])
@require_auth
async def generate_tg_bind_code():
    """
    生成 Telegram 绑定码
    
    用户获取绑定码后，在 Bot 中发送 /bind <绑定码> 完成绑定。
    
    Response:
        {
            "success": true,
            "data": {
                "bind_code": "123456",
                "expires_in": 300
            }
        }
    """
    from src.config import Config, TelegramConfig
    
    if not Config.TELEGRAM_MODE or not TelegramConfig.BOT_TOKEN:
        return api_response(False, "Telegram Bot 未启用", code=400)
    
    user = g.current_user
    
    # 已绑定则不允许再生成
    if user.TELEGRAM_ID:
        return api_response(False, "您已绑定 Telegram，如需更换请先解绑", code=400)
    
    # 清理过期绑定码
    await _cleanup_expired_codes()
    if await TelegramBindCodeOperate.count_active() >= _MAX_BIND_CODES:
        return api_response(False, "系统繁忙，请稍后重试", code=503)

    # 撤销该用户之前未使用的绑定码
    await TelegramBindCodeOperate.delete_user_codes(user.UID)
    
    # 生成新绑定码（确保不重复）
    code = _generate_bind_code()
    while await TelegramBindCodeOperate.get_code(code):
        code = _generate_bind_code()

    now = int(_time.time())
    await TelegramBindCodeOperate.upsert_code(
        code=code,
        scene=_BIND_SCENE_USER,
        uid=user.UID,
        username=user.USERNAME,
        created_at=now,
        expires_at=now + _BIND_CODE_EXPIRE,
    )
    
    return api_response(True, "绑定码已生成", {
        'bind_code': code,
        'expires_in': _BIND_CODE_EXPIRE,
    })


@users_bp.route('/telegram/register/bind-code', methods=['POST'])
async def generate_tg_register_bind_code():
    """
    生成注册时使用的 Telegram 绑定码（无需登录）
    """
    from src.config import Config, TelegramConfig
    from src.core.utils import rate_limit_check

    # 公开端点：按 IP 限制单位时间内生成的绑定码数量（5 次 / 10 分钟），
    # 防止单 IP 把全局 _MAX_BIND_CODES 配额填满造成 DoS。
    client_ip = request.remote_addr or 'unknown'
    allowed, retry_after = rate_limit_check(
        'tg_register_bind_code', client_ip, max_requests=5, window_seconds=600,
    )
    if not allowed:
        return api_response(
            False, f"请求过于频繁，请在 {retry_after} 秒后重试", code=429,
        )

    if not Config.TELEGRAM_MODE or not TelegramConfig.BOT_TOKEN:
        return api_response(False, "Telegram Bot 未启用", code=400)

    # 清理过期绑定码
    await _cleanup_expired_codes()
    if await TelegramBindCodeOperate.count_active() >= _MAX_BIND_CODES:
        return api_response(False, "系统繁忙，请稍后重试", code=503)

    # 生成新绑定码（确保不重复）
    code = _generate_bind_code()
    while await TelegramBindCodeOperate.get_code(code):
        code = _generate_bind_code()

    now = int(_time.time())
    await TelegramBindCodeOperate.upsert_code(
        code=code,
        scene=_BIND_SCENE_REGISTER,
        created_at=now,
        expires_at=now + _BIND_CODE_EXPIRE,
    )

    return api_response(True, "绑定码已生成", {
        'bind_code': code,
        'expires_in': _BIND_CODE_EXPIRE,
    })


@users_bp.route('/telegram/register/bind-code/status', methods=['GET'])
async def query_tg_register_bind_code_status():
    """注册阶段轮询绑定码是否已被 Bot 端确认。无需登录。

    Query:
        code: str - 注册绑定码（8 位）

    Response data:
        {
            "code": "ABCDEFGH",
            "confirmed": true,                  // 用户已通过 /bind 完成验证
            "expires_in": 117                   // 剩余秒数；过期为 0
        }
    """
    from src.config import Config, TelegramConfig

    code = (request.args.get('code') or '').strip().upper()
    if not code or len(code) != 8 or not code.isalnum():
        return api_response(False, "绑定码格式无效", code=400)

    if not Config.TELEGRAM_MODE or not TelegramConfig.BOT_TOKEN:
        return api_response(False, "Telegram Bot 未启用", code=400)

    code_info = await TelegramBindCodeOperate.get_code(code)
    if not code_info or code_info.SCENE != _BIND_SCENE_REGISTER:
        # 不暴露具体过期/无效，避免被探测
        return api_response(False, "绑定码无效或已过期", code=404)

    remaining = max(0, int(code_info.EXPIRES_AT - _time.time()))
    return api_response(True, "获取成功", {
        'code': code_info.CODE,
        'confirmed': bool(code_info.CONFIRMED_TELEGRAM_ID),
        'expires_in': remaining,
    })


async def confirm_tg_bind_internal(bind_code: str, telegram_id: int) -> tuple[bool, str, dict[str, Any], int]:
    """供 Bot 与内部接口复用的 Telegram 绑定确认逻辑。"""
    bind_code = (bind_code or '').strip().upper()
    if not bind_code or not telegram_id:
        return False, "参数缺失", {}, 400

    try:
        telegram_id = int(telegram_id)
    except (TypeError, ValueError):
        return False, "telegram_id 无效", {}, 400

    await _cleanup_expired_codes()
    code_info = await TelegramBindCodeOperate.get_code(bind_code)
    if not code_info:
        return False, "绑定码无效或已过期", {}, 400

    if code_info.SCENE == _BIND_SCENE_USER:
        uid = code_info.UID
        if not uid:
            return False, "绑定码数据损坏", {}, 400

        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing and existing.UID != uid:
            return False, "该 Telegram 已绑定其他账号，一个 Telegram 只能绑定一个账号", {}, 400

        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            return False, "用户不存在", {}, 404

        if user.TELEGRAM_ID:
            await _delete_bind_code(bind_code)
            return False, "该账号已绑定 Telegram", {}, 400

        # 强制要求加入指定群组（仅在配置了 GROUP_ID 时生效）。
        from src.services.telegram_membership import TelegramMembershipService
        ok, missing = await TelegramMembershipService.check_user_in_groups(telegram_id, strict=True)
        if not ok:
            return (
                False,
                TelegramMembershipService.format_missing_message(missing),
                {
                    'reason': 'not_in_required_group',
                    'missing_groups': [m.to_dict() for m in missing],
                },
                403,
            )

        user.TELEGRAM_ID = telegram_id
        await UserOperate.update_user(user)
        await _delete_bind_code(bind_code)

        logger.info(f"用户 {user.USERNAME} 通过 Bot 绑定 Telegram: {telegram_id}")

        from src.core.utils import format_expire_time
        from src.db.user import Role
        role_map = {
            Role.ADMIN.value: "管理员",
            Role.WHITE_LIST.value: "白名单",
            Role.NORMAL.value: "普通用户",
        }

        return True, "Telegram 绑定成功", {
            'uid': uid,
            'username': user.USERNAME,
            'telegram_id': telegram_id,
            'emby_id': user.EMBYID or None,
            'role': role_map.get(user.ROLE, '未知'),
            'active': user.ACTIVE_STATUS,
            'expired_at': format_expire_time(user.EXPIRED_AT),
        }, 200

    if code_info.SCENE == _BIND_SCENE_REGISTER:
        if code_info.CONFIRMED_TELEGRAM_ID and code_info.CONFIRMED_TELEGRAM_ID != telegram_id:
            return False, "该绑定码已被其他 Telegram 账号使用", {}, 400

        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            return False, "该 Telegram 已绑定其他账号，一个 Telegram 只能绑定一个账号", {}, 400

        # 注册阶段也强制检查群组成员资格，避免绕过 Bot 后再注册。
        from src.services.telegram_membership import TelegramMembershipService
        ok, missing = await TelegramMembershipService.check_user_in_groups(telegram_id, strict=True)
        if not ok:
            return (
                False,
                TelegramMembershipService.format_missing_message(missing),
                {
                    'reason': 'not_in_required_group',
                    'missing_groups': [m.to_dict() for m in missing],
                },
                403,
            )

        await TelegramBindCodeOperate.upsert_code(
            code=code_info.CODE,
            scene=code_info.SCENE,
            uid=code_info.UID,
            username=code_info.USERNAME,
            confirmed_telegram_id=telegram_id,
            created_at=code_info.CREATED_AT,
            expires_at=code_info.EXPIRES_AT,
        )

        logger.info(f"注册绑定码 {bind_code} 已由 Telegram {telegram_id} 验证")
        return True, "Telegram 绑定码验证成功", {
            'telegram_id': telegram_id,
        }, 200

    return False, "绑定码类型无效", {}, 400


@users_bp.route('/me/telegram/bind-confirm', methods=['POST'])
async def confirm_tg_bind():
    """
    Bot 调用此接口完成绑定（内部接口）
    
    Request:
        {
            "bind_code": "123456",
            "telegram_id": 123456789,
            "bot_secret": "..."
        }
    """
    data = request.get_json() or {}
    bind_code = data.get('bind_code', '').strip().upper()
    telegram_id = data.get('telegram_id')
    bot_secret = data.get('bot_secret', '')

    from src.config import SecurityConfig
    expected_secret = (SecurityConfig.BOT_INTERNAL_SECRET or '').strip()
    if not bot_secret or not expected_secret or not hmac.compare_digest(str(bot_secret), str(expected_secret)):
        return api_response(False, "未授权", code=403)

    ok, message, payload, status_code = await confirm_tg_bind_internal(bind_code, telegram_id)
    return api_response(ok, message, payload, code=status_code)

# ==================== 用户设置 ====================

@users_bp.route('/me/settings', methods=['GET'])
@require_auth
async def get_my_settings():
    """获取用户所有设置"""
    from src.config import RegisterConfig, DeviceLimitConfig, Config, EmbyConfig
    from src.services import EmbyService
    
    user = g.current_user
    
    status = await EmbyService.get_user_status(user)

    return api_response(True, "获取成功", {
        # 用户设置
        'bgm_mode': user.BGM_MODE,
        'bgm_token_set': bool(user.BGM_TOKEN),
        'api_key_enabled': user.APIKEY_STATUS,
        'emby_status': {
            'is_synced': status.is_synced,
            'is_active': status.is_active,
            'active_sessions': status.active_sessions,
            'message': status.message,
        },
        # Telegram 绑定
        'telegram': {
            'bound': bool(user.TELEGRAM_ID),
            'force_bind': Config.FORCE_BIND_TELEGRAM,
            'can_unbind': not Config.FORCE_BIND_TELEGRAM and bool(user.TELEGRAM_ID),
            'can_change': bool(user.TELEGRAM_ID),
        },
        # 系统配置
        'system_config': {
            'device_limit_enabled': DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
            'max_devices': DeviceLimitConfig.MAX_DEVICES,
            'max_streams': DeviceLimitConfig.MAX_STREAMS,
        },
    })


# ==================== 背景管理 ====================

_CSS_URL_RE = re.compile(r'^\s*url\(\s*([\'"]?)([^\'")]+)\1\s*\)\s*$', re.IGNORECASE)
_CSS_BG_FUNC_RE = re.compile(
    r'^\s*(linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|'
    r'repeating-radial-gradient|repeating-conic-gradient|image-set|cross-fade|'
    r'paint|element)\s*\(',
    re.IGNORECASE,
)


def _is_disallowed_bg_host(host: str) -> bool:
    """拦截会让访客浏览器去探测内网/元数据服务的危险主机名。

    背景图片 URL 会被所有访问页面的用户的浏览器请求，攻击者可以利用这一点
    把内网地址写进背景图配置，让被访问者代为发起 SSRF 探测。这里在「写入时」
    就拒绝掉所有解析为私网 / 回环 / 链路本地 / 多播 / 云元数据 (169.254.169.254)
    的 host，以及裸 IP 之外的常见 localhost 别名。
    """
    import ipaddress
    if not host:
        return True
    h = host.strip().strip('[').strip(']').lower()
    # localhost 别名 / metadata.google.internal 等
    bad_names = {
        'localhost', 'localhost.localdomain', 'ip6-localhost', 'ip6-loopback',
        'metadata.google.internal', 'metadata.goog',
    }
    if h in bad_names or h.endswith('.localhost'):
        return True
    # 尝试解析为 IP；解析不出来就当外网域名放行
    try:
        ip = ipaddress.ip_address(h)
    except ValueError:
        return False
    return bool(
        ip.is_private
        or ip.is_loopback
        or ip.is_link_local
        or ip.is_multicast
        or ip.is_reserved
        or ip.is_unspecified
    )


def _is_valid_background_url(value: str) -> bool:
    """允许:
    - 空字符串
    - 站内相对路径 ("/uploads/...")
    - 裸 http(s):// URL（host 不能解析到私网/回环/元数据地址）
    - CSS url("...") 包装（内部 URL 同样需通过校验）
    - linear-gradient / radial-gradient 等 CSS 背景函数
    """
    if not value:
        return True
    stripped = value.strip()
    if not stripped:
        return True

    # 站内相对路径
    if stripped.startswith('/'):
        return True

    # CSS url("...") 包装：解析内部 URL 后递归校验
    url_match = _CSS_URL_RE.match(stripped)
    if url_match:
        inner = url_match.group(2).strip()
        if not inner:
            return False
        if inner.startswith('/'):
            return True
        try:
            parsed = urlparse(inner)
        except Exception:
            return False
        if parsed.scheme not in ('http', 'https') or not parsed.netloc:
            return False
        return not _is_disallowed_bg_host(parsed.hostname or '')

    # gradient / image-set 等 CSS 函数：直接放行（仅做长度限制）
    if _CSS_BG_FUNC_RE.match(stripped):
        return True

    # 裸 http(s):// URL
    try:
        parsed = urlparse(stripped)
    except Exception:
        return False
    if parsed.scheme not in ('http', 'https') or not parsed.netloc:
        return False
    return not _is_disallowed_bg_host(parsed.hostname or '')

@users_bp.route('/<int:uid>/background', methods=['GET'])
@require_auth
async def get_user_background(uid: int):
    """获取用户背景配置"""
    if g.current_user.UID != uid and g.current_user.ROLE != Role.ADMIN.value:
        return api_response(False, "无权限查看其他用户背景", code=403)

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    # 从 OTHER 字段解析背景配置
    background_config = {}
    if user.OTHER:
        try:
            import json
            data = json.loads(user.OTHER)
            background_config = data.get('background', {})
        except:
            pass
    
    return api_response(True, "获取成功", {
        'background': json.dumps(background_config) if background_config else None
    })


@users_bp.route('/me/background', methods=['PUT'])
@require_auth
async def update_user_background():
    """
    更新用户背景配置
    
    Request:
        {
            "lightBg": "linear-gradient(...)",  // 浅色主题背景
            "darkBg": "linear-gradient(...)",    // 暗色主题背景
            "lightBgImage": "url(...)",         // 浅色背景图片
            "darkBgImage": "url(...)",          // 暗色背景图片
            "lightFlow": false,                   // 浅色背景流光开关
            "darkFlow": false,                    // 暗色背景流光开关
            "lightBlur": 0,                       // 浅色背景模糊(px)
            "darkBlur": 0,                        // 暗色背景模糊(px)
            "lightOpacity": 100,                  // 浅色背景透明度(0-100)
            "darkOpacity": 100                    // 暗色背景透明度(0-100)
        }
    """
    import json
    from src.core.utils import timestamp
    
    # 检查认证
    if not hasattr(g, 'current_user') or g.current_user is None:
        return api_response(False, "需要认证", code=401)
    
    user = g.current_user
    data = request.get_json() or {}
    
    # 验证输入
    light_bg = data.get('lightBg', '').strip()
    dark_bg = data.get('darkBg', '').strip()
    light_bg_image = data.get('lightBgImage', '').strip()
    dark_bg_image = data.get('darkBgImage', '').strip()
    
    if not light_bg and not dark_bg and not light_bg_image and not dark_bg_image:
        return api_response(False, "至少需要一个背景配置", code=400)
    
    # 背景URL或CSS长度限制
    MAX_BG_LENGTH = 2000
    if len(light_bg) > MAX_BG_LENGTH or len(dark_bg) > MAX_BG_LENGTH:
        return api_response(False, f"背景配置过长，最多 {MAX_BG_LENGTH} 字符", code=400)
    if len(light_bg_image) > MAX_BG_LENGTH or len(dark_bg_image) > MAX_BG_LENGTH:
        return api_response(False, f"背景图片URL过长", code=400)
    if not _is_valid_background_url(light_bg_image) or not _is_valid_background_url(dark_bg_image):
        return api_response(
            False,
            "背景图片格式不合法，支持站内相对路径、http(s) URL、CSS url(...) 包装或 linear-gradient 等背景函数",
            code=400,
        )
    
    # 保存到 OTHER 字段
    try:
        other_data = {}
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}

        existing_background = other_data.get('background', {}) if isinstance(other_data.get('background', {}), dict) else {}
        light_flow = bool(data.get('lightFlow')) if 'lightFlow' in data else bool(existing_background.get('lightFlow', False))
        dark_flow = bool(data.get('darkFlow')) if 'darkFlow' in data else bool(existing_background.get('darkFlow', False))

        def _clamp_int(value, default, min_value, max_value):
            try:
                num = int(value)
            except Exception:
                num = default
            return max(min_value, min(max_value, num))

        light_blur = _clamp_int(
            data.get('lightBlur', existing_background.get('lightBlur', 0)),
            0,
            0,
            30,
        )
        dark_blur = _clamp_int(
            data.get('darkBlur', existing_background.get('darkBlur', 0)),
            0,
            0,
            30,
        )
        light_opacity = _clamp_int(
            data.get('lightOpacity', existing_background.get('lightOpacity', 100)),
            100,
            10,
            100,
        )
        dark_opacity = _clamp_int(
            data.get('darkOpacity', existing_background.get('darkOpacity', 100)),
            100,
            10,
            100,
        )
        
        other_data['background'] = {
            'lightBg': light_bg,
            'darkBg': dark_bg,
            'lightBgImage': light_bg_image,
            'darkBgImage': dark_bg_image,
            'lightFlow': light_flow,
            'darkFlow': dark_flow,
            'lightBlur': light_blur,
            'darkBlur': dark_blur,
            'lightOpacity': light_opacity,
            'darkOpacity': dark_opacity,
            'updated_at': timestamp()
        }
        
        user.OTHER = json.dumps(other_data)
        await UserOperate.update_user(user)
        
        return api_response(True, "背景更新成功", {
            'background': json.dumps(other_data['background'])
        })
    except Exception as e:
        logger.error(f"保存背景配置失败: {e}")
        return api_response(False, "保存失败", code=500)


@users_bp.route('/me/background', methods=['DELETE'])
@require_auth
async def delete_user_background():
    """删除用户背景配置，恢复默认背景"""
    import json
    
    # 检查认证
    if not hasattr(g, 'current_user') or g.current_user is None:
        return api_response(False, "需要认证", code=401)
    
    user = g.current_user
    
    try:
        other_data = {}
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}
        
        # 删除背景配置
        if 'background' in other_data:
            del other_data['background']
        
        user.OTHER = json.dumps(other_data) if other_data else ''
        await UserOperate.update_user(user)
        
        return api_response(True, "背景已重置为默认")
    except Exception as e:
        logger.error(f"删除背景配置失败: {e}")
        return api_response(False, "删除失败", code=500)


@users_bp.route('/me/background/upload', methods=['POST'])
@require_auth
async def upload_background_image():
    """
    上传背景图片
    
    Request:
        Form-data:
            file: 图片文件 (max 5MB)
            type: 'light' 或 'dark' - 指定这是浅色或暗色背景
    
    Response:
        {
            "success": true,
            "data": {
                "url": "/uploads/backgrounds/xxx.jpg",
                "type": "light"
            }
        }
    """
    import os
    import uuid
    from pathlib import Path
    from flask import current_app
    from src.core.utils import rate_limit_check

    # 检查认证
    if not hasattr(g, 'current_user') or g.current_user is None:
        return api_response(False, "需要认证", code=401)

    user = g.current_user

    # 限流：每个用户 10 次 / 分钟，防止刷爆磁盘
    allowed, retry_after = rate_limit_check(
        'upload_background', f'uid:{user.UID}', max_requests=10, window_seconds=60,
    )
    if not allowed:
        return api_response(
            False, f"上传过于频繁，请在 {retry_after} 秒后重试", code=429,
        )

    # 检查文件
    if 'file' not in request.files:
        return api_response(False, "未找到文件", code=400)

    file = request.files['file']
    if file.filename == '':
        return api_response(False, "文件名为空", code=400)

    # 检查背景类型
    bg_type = request.form.get('type', 'light').lower()
    if bg_type not in ['light', 'dark']:
        return api_response(False, "背景类型必须为 'light' 或 'dark'", code=400)
    
    # 读取文件头并识别图片类型
    header = file.read(32)
    file.seek(0)
    detected_ext = _detect_image_extension(header)
    if not detected_ext:
        return api_response(False, "文件内容不是有效的图片", code=400)
    
    # 验证文件大小
    file.seek(0, os.SEEK_END)
    file_size = file.tell()
    file.seek(0)
    
    MAX_SIZE = current_app.config.get('MAX_CONTENT_LENGTH', 5 * 1024 * 1024)
    if file_size > MAX_SIZE:
        return api_response(False, f"文件过大，最大 {MAX_SIZE // (1024*1024)}MB", code=400)
    
    try:
        # 创建上传目录
        upload_dir = os.path.join(current_app.config['UPLOAD_FOLDER'], 'backgrounds')
        os.makedirs(upload_dir, exist_ok=True)
        
        # 生成唯一文件名
        filename = f"{uuid.uuid4().hex}.{detected_ext}"
        filepath = os.path.join(upload_dir, filename)
        
        # 保存文件
        file.save(filepath)
        
        # 生成 URL
        file_url = f"/uploads/backgrounds/{filename}"
        
        return api_response(True, "上传成功", {
            'url': file_url,
            'type': bg_type,
            'filename': filename
        })
    except Exception as e:
        logger.error(f"上传背景图片失败: {e}")
        return api_response(False, "上传失败", code=500)

# ==================== 头像管理 ====================

@users_bp.route('/<int:uid>/avatar', methods=['GET'])
@require_auth
async def get_user_avatar(uid: int):
    """获取用户头像"""
    if g.current_user.UID != uid and g.current_user.ROLE != Role.ADMIN.value:
        return api_response(False, "无权限查看其他用户头像", code=403)

    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    return api_response(True, "获取成功", {
        'avatar': user.AVATAR or None,
        'uid': user.UID,
        'username': user.USERNAME,
    })


@users_bp.route('/me/avatar/upload', methods=['POST'])
@require_auth
async def upload_avatar():
    """
    上传用户头像
    
    Request:
        Form-data:
            file: 头像图片文件 (max 2MB，推荐 200x200px)
    
    Response:
        {
            "success": true,
            "data": {
                "avatar_url": "/uploads/avatars/xxx.jpg"
            }
        }
    """
    import os
    import uuid
    from pathlib import Path
    from flask import current_app
    from src.core.utils import rate_limit_check

    user = g.current_user

    # 限流：每个用户 10 次 / 分钟
    allowed, retry_after = rate_limit_check(
        'upload_avatar', f'uid:{user.UID}', max_requests=10, window_seconds=60,
    )
    if not allowed:
        return api_response(
            False, f"上传过于频繁，请在 {retry_after} 秒后重试", code=429,
        )

    # 检查文件
    if 'file' not in request.files:
        return api_response(False, "缺少文件", code=400)

    file = request.files['file']
    if file.filename == '':
        return api_response(False, "未选择文件", code=400)
    
    try:
        # 验证文件类型（Content-Type + magic bytes）
        allowed_types = {'image/jpeg', 'image/png', 'image/gif', 'image/webp'}
        if file.content_type not in allowed_types:
            return api_response(False, "只支持 JPG、PNG、GIF、WebP 格式的图片", code=400)
        
        # 读取文件头并识别图片类型
        header = file.read(32)
        file.seek(0)
        detected_ext = _detect_image_extension(header)
        if not detected_ext:
            return api_response(False, "文件内容不是有效的图片", code=400)
        
        # 验证文件大小
        file.seek(0, 2)
        file_size = file.tell()
        file.seek(0)
        
        if file_size > 2 * 1024 * 1024:  # 2MB
            return api_response(False, "文件大小不能超过 2MB", code=400)
        
        # 创建上传目录
        upload_dir = os.path.join(current_app.config['UPLOAD_FOLDER'], 'avatars')
        os.makedirs(upload_dir, exist_ok=True)
        
        # 生成唯一文件名
        filename = f"{uuid.uuid4().hex}.{detected_ext}"
        filepath = os.path.join(upload_dir, filename)
        
        # 保存文件
        file.save(filepath)
        
        # 生成 URL
        avatar_url = f"/uploads/avatars/{filename}"
        
        # 更新用户头像
        user.AVATAR = avatar_url
        await UserOperate.update_user(user)
        
        logger.info(f"用户上传头像: {user.USERNAME} -> {avatar_url}")
        
        return api_response(True, "头像上传成功", {
            'avatar_url': avatar_url,
        })
    except Exception as e:
        logger.error(f"上传头像失败: {e}")
        return api_response(False, "上传失败", code=500)


@users_bp.route('/me/avatar', methods=['DELETE'])
@require_auth
async def delete_avatar():
    """删除用户头像"""
    user = g.current_user
    
    if not user.AVATAR:
        return api_response(False, "用户未设置头像", code=400)
    
    # 删除头像文件
    try:
        from pathlib import Path
        from flask import current_app

        avatar_url = (user.AVATAR or '').strip()
        if avatar_url.startswith('/uploads/avatars/'):
            upload_root = Path(current_app.config['UPLOAD_FOLDER']).resolve()
            relative_path = avatar_url.removeprefix('/uploads/')
            file_path = (upload_root / relative_path).resolve()
            if str(file_path).startswith(str(upload_root)) and file_path.exists() and file_path.is_file():
                file_path.unlink()
    except Exception as e:
        logger.warning(f"删除头像文件失败: {e}")
    
    # 清除头像 URL
    user.AVATAR = None
    await UserOperate.update_user(user)
    
    return api_response(True, "头像已删除")


# ==================== API Key 管理 ====================

def _serialize_api_key(model) -> dict:
    """API Key 列表序列化（不返回明文）。"""
    from src.db.apikey import ApiKeyOperate
    masked = f"{model.KEY_PREFIX}…{model.KEY_SUFFIX}" if (model.KEY_PREFIX or model.KEY_SUFFIX) else "****"
    return {
        'id': model.ID,
        'name': model.NAME,
        'key': masked,            # 仅展示用，明文已不再保留
        'key_prefix': model.KEY_PREFIX,
        'key_suffix': model.KEY_SUFFIX,
        'enabled': model.ENABLED,
        'allow_query': model.ALLOW_QUERY,
        'permissions': ApiKeyOperate.get_permissions(model),
        'rate_limit': model.RATE_LIMIT,
        'request_count': model.REQUEST_COUNT,
        'last_used': model.LAST_USED_AT,
        'created_at': model.CREATED_AT,
        'expired_at': model.EXPIRED_AT,
    }


@users_bp.route('/me/apikeys', methods=['GET'])
@require_auth
async def get_my_api_keys():
    """获取我的 API Keys 列表（不返回明文，明文仅创建时返回一次）。"""
    from src.db.apikey import ApiKeyOperate

    api_keys = await ApiKeyOperate.get_user_api_keys(g.current_user.UID)
    keys_list = [_serialize_api_key(k) for k in api_keys]

    return api_response(True, "获取成功", {
        'keys': keys_list,
        'total': len(keys_list),
    })


@users_bp.route('/me/apikeys', methods=['POST'])
@require_auth
async def generate_api_key():
    """
    生成新的 API Key（明文仅在响应中返回一次，请妥善保存）

    Request:
        {
            "name": "My API Key",
            "allow_query": true,
            "rate_limit": 100,
            "expired_at": -1,
            "permissions": ["account:read"]   // 可选
        }
    """
    from src.db.apikey import ApiKeyOperate

    data = request.get_json() or {}
    name = data.get('name')
    allow_query = bool(data.get('allow_query', True))
    rate_limit = data.get('rate_limit', 100)
    expired_at = data.get('expired_at', -1)
    permissions = data.get('permissions')

    try:
        rate_limit = int(rate_limit)
    except (TypeError, ValueError):
        return api_response(False, "速率限制必须是整数", code=400)
    if rate_limit < 0:
        return api_response(False, "速率限制不能为负数", code=400)

    try:
        api_key, plaintext = await ApiKeyOperate.create_api_key(
            uid=g.current_user.UID,
            name=name,
            allow_query=allow_query,
            rate_limit=rate_limit,
            expired_at=int(expired_at) if expired_at is not None else -1,
            permissions=permissions if isinstance(permissions, list) else None,
        )

        logger.info(f"用户生成 API Key: {g.current_user.USERNAME} -> {api_key.ID}")

        return api_response(True, "API Key 生成成功，请立即保存（明文不会再次显示）", {
            'id': api_key.ID,
            'key': plaintext,
            'name': api_key.NAME,
            'created_at': api_key.CREATED_AT,
        })
    except Exception as e:
        logger.error(f"生成 API Key 失败: {e}", exc_info=True)
        return api_response(False, "生成失败", code=500)


@users_bp.route('/me/apikeys/<int:key_id>', methods=['PUT'])
@require_auth
async def update_api_key(key_id: int):
    """
    更新 API Key 配置

    Request:
        {
            "name": "Updated Name",
            "enabled": true,
            "allow_query": true,
            "rate_limit": 100,
            "expired_at": -1,
            "permissions": ["account:read"]
        }
    """
    from src.db.apikey import ApiKeyOperate

    api_key = await ApiKeyOperate.get_api_key_by_id(key_id)
    if not api_key or api_key.UID != g.current_user.UID:
        return api_response(False, "API Key 不存在或无权限修改", code=404)

    data = request.get_json() or {}
    perms = data.get('permissions')
    if perms is not None and not isinstance(perms, list):
        return api_response(False, "permissions 必须是数组", code=400)

    try:
        updated_key = await ApiKeyOperate.update_api_key(
            key_id=key_id,
            name=data.get('name'),
            enabled=data.get('enabled'),
            allow_query=data.get('allow_query'),
            rate_limit=data.get('rate_limit'),
            expired_at=data.get('expired_at'),
            permissions=perms,
        )

        logger.info(f"用户更新 API Key: {g.current_user.USERNAME} -> {key_id}")

        return api_response(True, "API Key 更新成功", _serialize_api_key(updated_key))
    except Exception as e:
        logger.error(f"更新 API Key 失败: {e}", exc_info=True)
        return api_response(False, "更新失败", code=500)


@users_bp.route('/me/apikeys/<int:key_id>', methods=['DELETE'])
@require_auth
async def delete_api_key(key_id: int):
    """删除 API Key（不可恢复）。"""
    from src.db.apikey import ApiKeyOperate

    api_key = await ApiKeyOperate.get_api_key_by_id(key_id)
    if not api_key or api_key.UID != g.current_user.UID:
        return api_response(False, "API Key 不存在或无权限删除", code=404)

    try:
        success = await ApiKeyOperate.delete_api_key(key_id)
        if success:
            logger.info(f"用户删除 API Key: {g.current_user.USERNAME} -> {key_id}")
            return api_response(True, "API Key 已删除")
        return api_response(False, "删除失败", code=500)
    except Exception as e:
        logger.error(f"删除 API Key 失败: {e}", exc_info=True)
        return api_response(False, "删除失败", code=500)