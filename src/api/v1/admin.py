"""
管理员 API

提供管理员专用的操作接口
"""
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, require_admin, api_response
from src.db.user import UserOperate, Role
from src.db.regcode import RegCodeOperate
from src.services import UserService, ScoreService, EmbyService

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
        active: bool - 按状态筛选 (true/false)
        search: str - 搜索用户名
    """
    page = request.args.get('page', 1, type=int)
    per_page = min(request.args.get('per_page', 20, type=int), 100)
    role = request.args.get('role', type=int)
    active = request.args.get('active')
    search = request.args.get('search', '').strip()
    
    # 转换 active 参数
    include_inactive = True
    if active is not None:
        include_inactive = active.lower() != 'true'  # 如果 active=true，则只包含激活用户
    
    # 计算偏移量
    offset = (page - 1) * per_page
    
    # 获取用户列表（包含总数）
    users, total = await UserOperate.get_all_users(
        offset=offset,
        limit=per_page,
        role=role,
        include_inactive=include_inactive
    )
    
    # 如果有搜索条件，在内存中过滤（简单实现）
    if search:
        filtered_users = [u for u in users if search.lower() in (u.USERNAME or '').lower()]
        # 如果进行了搜索过滤，需要重新计算总数（这里简化处理，使用过滤后的数量）
        # 实际应该在前端或后端进行更精确的搜索
        users = filtered_users
        if search:
            # 搜索时总数不准确，但至少显示当前页的结果数
            total = len(users)
    
    # 转换为字典
    user_list = []
    # 尝试获取 bot 实例用于获取 Telegram 用户名
    bot_instance = None
    try:
        from src.bot.bot import get_bot_instance
        bot_instance = get_bot_instance()
    except Exception:
        pass
    
    for user in users:
        # 获取用户积分
        from src.db.score import ScoreOperate
        score_record = await ScoreOperate.get_score_by_uid(user.UID)
        
        # 尝试获取 Telegram 用户名
        telegram_username = None
        if user.TELEGRAM_ID and bot_instance and bot_instance.app:
            try:
                tg_user = await bot_instance.app.get_users(user.TELEGRAM_ID)
                telegram_username = tg_user.username or f"{tg_user.first_name or ''} {tg_user.last_name or ''}".strip() or None
            except Exception:
                pass  # 如果获取失败，忽略
        
        user_list.append({
            'uid': user.UID,
            'telegram_id': user.TELEGRAM_ID,
            'telegram_username': telegram_username,  # 添加 Telegram 用户名
            'username': user.USERNAME,
            'email': user.EMAIL,
            'role': user.ROLE,
            'role_name': Role(user.ROLE).name if user.ROLE in [r.value for r in Role] else 'UNKNOWN',
            'active': user.ACTIVE_STATUS,
            'emby_id': user.EMBYID,
            'expired_at': user.EXPIRED_AT,
            'register_time': user.REGISTER_TIME,
            'last_login_time': user.LAST_LOGIN_TIME,
            'auto_renew': user.AUTO_RENEW,
            'bgm_mode': user.BGM_MODE,
            'score': score_record.SCORE if score_record else 0,
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
        score: int - 积分
        其他字段...
    """
    data = request.get_json() or {}
    
    # 只允许管理员修改自己的某些字段
    allowed_fields = {'score'}  # 可以扩展
    update_data = {k: v for k, v in data.items() if k in allowed_fields}
    
    if not update_data:
        return api_response(False, "没有可更新的字段", code=400)
    
    try:
        user = await UserOperate.get_user_by_uid(g.current_user.UID)
        if not user:
            return api_response(False, "用户不存在", code=404)
        
        # 更新积分
        if 'score' in update_data:
            from src.db.score import ScoreOperate
            if not g.current_user.TELEGRAM_ID:
                return api_response(False, "用户未绑定 Telegram，无法设置积分", code=400)
            await ScoreOperate.set_score(g.current_user.TELEGRAM_ID, update_data['score'])
        
        return api_response(True, "更新成功")
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"管理员更新自己信息失败: {e}", exc_info=True)
        return api_response(False, f"更新失败: {e}", code=500)


@admin_bp.route('/users/<int:uid>', methods=['GET'])
@require_auth
@require_admin
async def get_user(uid: int):
    """获取用户详情"""
    from src.config import EmbyConfig
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    user_info = await UserService.get_user_info(user)
    status = await EmbyService.get_user_status(user)
    
    # 获取 NSFW 权限信息
    nsfw_library_id = await EmbyService.find_nsfw_library_id()
    has_nsfw_permission = False
    if nsfw_library_id and user.EMBYID:
        library_ids, enable_all = await EmbyService.get_user_library_access(user)
        has_nsfw_permission = enable_all or (nsfw_library_id in library_ids)
    
    user_info['emby_status'] = {
        'is_synced': status.is_synced,
        'is_active': status.is_active,
        'active_sessions': status.active_sessions,
        'message': status.message,
    }
    
    user_info['nsfw'] = {
        'enabled': user.NSFW,
        'has_permission': user.NSFW_ALLOWED,
        'nsfw_library_id': nsfw_library_id,
    }
    
    return api_response(True, "获取成功", user_info)


@admin_bp.route('/users/<int:uid>/disable', methods=['POST'])
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
@require_auth
@require_admin
async def enable_user(uid: int):
    """启用用户"""
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    success, message = await UserService.enable_user(user)
    return api_response(success, message)


@admin_bp.route('/users/<int:uid>', methods=['PUT'])
@require_auth
@require_admin
async def update_user(uid: int):
    """
    更新用户信息
    
    Body:
        role: int - 角色 (0=管理员, 1=普通用户, 2=白名单)
        score: int - 积分
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
        
        # 更新积分
        if 'score' in data:
            from src.db.score import ScoreOperate
            await ScoreOperate.set_score_by_uid(uid, data['score'])
        
        # 更新 Emby ID
        if 'emby_id' in data:
            target_user.EMBYID = data['emby_id'] or None
        
        # 更新启用状态
        if 'active' in data:
            target_user.ACTIVE_STATUS = bool(data['active'])
        
        # 保存到数据库
        await UserOperate.update_user(target_user)
        
        return api_response(True, "更新成功")
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"更新用户信息失败: {e}", exc_info=True)
        return api_response(False, f"更新失败: {e}", code=500)


@admin_bp.route('/users/<int:uid>', methods=['DELETE'])
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


@admin_bp.route('/users/<int:uid>/nsfw', methods=['PUT'])
@require_auth
@require_admin
async def set_user_nsfw_permission(uid: int):
    """
    设置用户 NSFW 库访问权限（管理员）
    
    Request:
        {
            "grant": true  // true=授予权限, false=撤销权限
        }
    """
    from src.config import EmbyConfig
    from src.services.emby import get_emby_client
    
    user = await UserOperate.get_user_by_uid(uid)
    if not user:
        return api_response(False, "用户不存在", code=404)
    
    if not user.EMBYID:
        return api_response(False, "用户未绑定 Emby 账户", code=400)
    
    # 查找NSFW库ID（支持通过名称或ID匹配）
    nsfw_library_id = await EmbyService.find_nsfw_library_id()
    if not nsfw_library_id:
        return api_response(False, "系统未配置 NSFW 媒体库", code=400)
    
    data = request.get_json() or {}
    grant = data.get('grant', True)
    
    try:
        # 更新数据库中的权限状态
        user.NSFW_ALLOWED = grant
        if not grant:
            # 如果取消权限，强制关闭显示状态
            user.NSFW = False
            
        await UserOperate.update_user(user)
        
        # 同步到 Emby
        success, message = await UserService.sync_user_to_emby(user)
        
        if success:
            status_msg = "已授予" if grant else "已撤销"
            return api_response(True, f"{status_msg} NSFW 库访问权限")
        else:
            return api_response(False, f"同步到 Emby 失败: {message}", code=500)
            
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"设置用户 NSFW 权限失败: {e}", exc_info=True)
        return api_response(False, f"操作失败: {e}", code=500)


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


# ==================== 积分管理 ====================

@admin_bp.route('/users/<int:uid>/score', methods=['PUT'])
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
    
    # 转换为字典格式
    results = []
    for req in requests:
        other = {}
        if req.other_info:
            try:
                other = json.loads(req.other_info)
            except:
                pass
        
        # 获取用户信息
        user = await UserOperate.get_user_by_telegram_id(req.telegram_id)
        
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
    per_page = 20
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
    note = data.get('note', '')
    
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

