"""
系统信息 API

提供系统配置、状态等信息
"""
from flask import Blueprint, request, g
from sqlalchemy import text

from src.api.v1.auth import require_auth, require_admin, api_response
from src.config import (
    Config, EmbyConfig, ScoreAndRegisterConfig, WebhookConfig,
    DeviceLimitConfig, APIConfig, SecurityConfig, SchedulerConfig,
    NotificationConfig, TelegramConfig
)
from src import __version__
from src.db.user import UsersSessionFactory

system_bp = Blueprint('system', __name__, url_prefix='/system')


# ==================== 公开信息 ====================

@system_bp.route('/info', methods=['GET'])
async def get_system_info():
    """
    获取系统公开信息
    
    不需要登录即可访问
    """
    return api_response(True, "获取成功", {
        'name': Config.SERVER_NAME or 'Twilight',
        'icon': Config.SERVER_ICON or '',
        'version': __version__,
        'features': {
            'register': ScoreAndRegisterConfig.REGISTER_MODE,
            'score_register': ScoreAndRegisterConfig.SCORE_REGISTER_MODE,
            'telegram': Config.TELEGRAM_MODE,
            'webhook': WebhookConfig.WEBHOOK_ENABLED,
            'red_packet': ScoreAndRegisterConfig.RED_PACKET_MODE,
            'transfer': ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE,
            'auto_renew': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
            'invite': ScoreAndRegisterConfig.INVITE_ENABLED,
        },
        'limits': {
            'user_limit': ScoreAndRegisterConfig.USER_LIMIT,
            'device_limit': DeviceLimitConfig.MAX_DEVICES if DeviceLimitConfig.DEVICE_LIMIT_ENABLED else None,
            'stream_limit': DeviceLimitConfig.MAX_STREAMS if DeviceLimitConfig.DEVICE_LIMIT_ENABLED else None,
        },
        'score': {
            'name': ScoreAndRegisterConfig.SCORE_NAME,
            'register_need': ScoreAndRegisterConfig.SCORE_REGISTER_NEED,
            'auto_renew_cost': ScoreAndRegisterConfig.AUTO_RENEW_COST,
        },
    })


@system_bp.route('/health', methods=['GET'])
async def health_check():
    """健康检查"""
    from src.services import get_emby_client
    
    status = {
        'api': True,
        'database': False,
        'emby': False,
    }
    
    # 检查数据库连接
    try:
        async with UsersSessionFactory() as session:
            await session.execute(text('SELECT 1'))
        status['database'] = True
    except Exception:
        status['database'] = False

    # 检查 Emby 连接
    try:
        emby = get_emby_client()
        info = await emby.get_public_info()
        status['emby'] = bool(info)
    except Exception:
        pass
    
    all_healthy = all(status.values())
    
    return api_response(all_healthy, "OK" if all_healthy else "部分服务异常", status)


@system_bp.route('/stats', methods=['GET'])
@require_auth
@require_admin
async def system_stats():
    """获取系统运行时统计信息（管理员）"""
    import os
    import time
    try:
        import psutil
    except ImportError:
        psutil = None

    stats = {
        'timestamp': int(time.time()),
        'cpu_count': os.cpu_count(),
        'cpu_percent': None,
        'memory': None,
        'disk': None,
    }

    if psutil:
        stats['cpu_percent'] = psutil.cpu_percent(interval=None)
        
        mem = psutil.virtual_memory()
        stats['memory'] = {
            'total': mem.total,
            'available': mem.available,
            'percent': mem.percent,
            'used': mem.used
        }
        
        disk = psutil.disk_usage('/')
        stats['disk'] = {
            'total': disk.total,
            'free': disk.free,
            'percent': disk.percent
        }
    
    # 获取应用级统计 (如总用户数，今日活跃等，这里由于性能原因可以简化或异步获取，暂时只返回系统级)
    # 若需业务统计，可复用 src.api.v1.stats
    
    return api_response(True, "获取成功", stats)


@system_bp.route('/emby-urls', methods=['GET'])
async def get_emby_urls():
    """获取 Emby 服务器地址列表"""
    return api_response(True, "获取成功", {
        'urls': EmbyConfig.EMBY_URL_LIST,
    })


# ==================== 需要登录 ====================

@system_bp.route('/config', methods=['GET'])
@require_auth
async def get_user_config():
    """获取用户可见的配置"""
    return api_response(True, "获取成功", {
        'score': {
            'name': ScoreAndRegisterConfig.SCORE_NAME,
            'checkin': {
                'base': ScoreAndRegisterConfig.CHECKIN_BASE_SCORE,
                'streak_bonus': ScoreAndRegisterConfig.CHECKIN_STREAK_BONUS,
                'max_streak': ScoreAndRegisterConfig.CHECKIN_MAX_STREAK_BONUS,
                'random_range': [ScoreAndRegisterConfig.CHECKIN_RANDOM_MIN, ScoreAndRegisterConfig.CHECKIN_RANDOM_MAX],
            },
            'transfer': {
                'enabled': ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE,
                'min': ScoreAndRegisterConfig.TRANSFER_MIN_AMOUNT,
                'max': ScoreAndRegisterConfig.TRANSFER_MAX_AMOUNT,
                'fee_rate': ScoreAndRegisterConfig.TRANSFER_FEE_RATE,
            },
            'red_packet': {
                'enabled': ScoreAndRegisterConfig.RED_PACKET_MODE,
                'min_amount': ScoreAndRegisterConfig.RED_PACKET_MIN_AMOUNT,
                'max_amount': ScoreAndRegisterConfig.RED_PACKET_MAX_AMOUNT,
                'min_count': ScoreAndRegisterConfig.RED_PACKET_MIN_COUNT,
                'max_count': ScoreAndRegisterConfig.RED_PACKET_MAX_COUNT,
            },
        },
        'auto_renew': {
            'enabled': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
            'days': ScoreAndRegisterConfig.AUTO_RENEW_DAYS,
            'cost': ScoreAndRegisterConfig.AUTO_RENEW_COST,
            'before_days': ScoreAndRegisterConfig.AUTO_RENEW_BEFORE_DAYS,
        },
        'device_limit': {
            'enabled': DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
            'max_devices': DeviceLimitConfig.MAX_DEVICES,
            'max_streams': DeviceLimitConfig.MAX_STREAMS,
        },
    })


# ==================== 管理员专用 ====================

@system_bp.route('/admin/config', methods=['GET'])
@require_auth
@require_admin
async def get_admin_config():
    """获取完整的系统配置（管理员）"""
    return api_response(True, "获取成功", {
        'global': {
            'logging': Config.LOGGING,
            'log_level': Config.LOG_LEVEL,
            'telegram_mode': Config.TELEGRAM_MODE,
            'email_bind': Config.EMAIL_BIND,
            'force_bind_email': Config.FORCE_BIND_EMAIL,
            'force_bind_telegram': Config.FORCE_BIND_TELEGRAM,
        },
        'emby': {
            'url': EmbyConfig.EMBY_URL,
            'url_list': EmbyConfig.EMBY_URL_LIST,
            'nsfw_library': EmbyConfig.EMBY_NSFW,
        },
        'telegram': {
            'enabled': Config.TELEGRAM_MODE,
            'admin_ids': TelegramConfig.ADMIN_ID,
            'group_ids': TelegramConfig.GROUP_ID,
            'force_subscribe': TelegramConfig.FORCE_SUBSCRIBE,
        },
        'sar': {
            'score_name': ScoreAndRegisterConfig.SCORE_NAME,
            'register_mode': ScoreAndRegisterConfig.REGISTER_MODE,
            'register_code_limit': ScoreAndRegisterConfig.REGISTER_CODE_LIMIT,
            'score_register_mode': ScoreAndRegisterConfig.SCORE_REGISTER_MODE,
            'score_register_need': ScoreAndRegisterConfig.SCORE_REGISTER_NEED,
            'user_limit': ScoreAndRegisterConfig.USER_LIMIT,
            'red_packet_mode': ScoreAndRegisterConfig.RED_PACKET_MODE,
            'transfer_mode': ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE,
            'auto_renew_enabled': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
            'invite_enabled': ScoreAndRegisterConfig.INVITE_ENABLED,
        },
        'device_limit': {
            'enabled': DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
            'max_devices': DeviceLimitConfig.MAX_DEVICES,
            'max_streams': DeviceLimitConfig.MAX_STREAMS,
            'kick_oldest': DeviceLimitConfig.KICK_OLDEST_SESSION,
        },
        'webhook': {
            'enabled': WebhookConfig.WEBHOOK_ENABLED,
            'has_secret': bool(WebhookConfig.WEBHOOK_SECRET),
            'endpoints_count': len(WebhookConfig.WEBHOOK_ENDPOINTS),
            'playback_stats': WebhookConfig.PLAYBACK_STATS_ENABLED,
            'ranking_enabled': WebhookConfig.RANKING_ENABLED,
        },
        'security': {
            'ip_limit': SecurityConfig.IP_LIMIT_ENABLED,
            'max_ips': SecurityConfig.MAX_IPS_PER_USER,
            'login_fail_threshold': SecurityConfig.LOGIN_FAIL_THRESHOLD,
            'lockout_minutes': SecurityConfig.LOCKOUT_MINUTES,
        },
        'api': {
            'host': APIConfig.HOST,
            'port': APIConfig.PORT,
            'debug': APIConfig.DEBUG,
            'token_expire': APIConfig.TOKEN_EXPIRE,
            'cors_enabled': APIConfig.CORS_ENABLED,
        },
        'scheduler': {
            'enabled': SchedulerConfig.ENABLED,
            'timezone': SchedulerConfig.TIMEZONE,
        },
        'notification': {
            'enabled': NotificationConfig.ENABLED,
            'expiry_remind_days': NotificationConfig.EXPIRY_REMIND_DAYS,
        },
    })


@system_bp.route('/admin/stats', methods=['GET'])
@require_auth
@require_admin
async def get_system_stats():
    """获取系统统计信息（管理员）"""
    from src.db.user import UserOperate
    from src.db.regcode import RegCodeOperate
    from src.services import EmbyService
    
    # 用户统计
    total_users = await UserOperate.get_registered_users_count()
    active_users = await UserOperate.get_active_users_count()
    
    # 注册码统计（使用数据库层面计数，避免全量加载到内存）
    regcode_stats = await RegCodeOperate.get_regcode_stats()
    
    # Emby 状态
    try:
        emby_status = await EmbyService.get_server_status()
    except Exception:
        emby_status = {'online': False}
    
    return api_response(True, "获取成功", {
        'users': {
            'total': total_users,
            'active': active_users,
            'limit': ScoreAndRegisterConfig.USER_LIMIT,
            'usage_percent': round(total_users / ScoreAndRegisterConfig.USER_LIMIT * 100, 1) if ScoreAndRegisterConfig.USER_LIMIT > 0 else 0,
        },
        'regcodes': regcode_stats,
        'emby': emby_status,
    })


@system_bp.route('/admin/config/toml', methods=['GET'])
@require_auth
@require_admin
async def get_config_toml():
    """获取 config.toml 文件内容（管理员）"""
    import os
    from pathlib import Path
    from src.config import ROOT_PATH
    
    config_file = ROOT_PATH / 'config.toml'
    
    if not config_file.exists():
        return api_response(False, "配置文件不存在", code=404)
    
    try:
        with open(config_file, 'r', encoding='utf-8') as f:
            content = f.read()
        return api_response(True, "获取成功", {
            'content': content,
            'path': str(config_file),
        })
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"读取配置文件失败: {e}", exc_info=True)
        return api_response(False, f"读取配置文件失败: {e}", code=500)


@system_bp.route('/admin/config/toml', methods=['PUT'])
@require_auth
@require_admin
async def update_config_toml():
    """更新 config.toml 文件内容（管理员）"""
    import os
    from pathlib import Path
    from src.config import ROOT_PATH
    import toml
    
    data = request.get_json() or {}
    content = data.get('content')
    
    if content is None:
        return api_response(False, "缺少 content 参数", code=400)
    
    config_file = ROOT_PATH / 'config.toml'
    
    # 验证 TOML 格式
    try:
        toml.loads(content)
    except Exception as e:
        return api_response(False, f"TOML 格式错误: {e}", code=400)
    
    # 备份原文件
    backup_file = ROOT_PATH / 'config.toml.backup'
    try:
        if config_file.exists():
            import shutil
            shutil.copy2(config_file, backup_file)
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.warning(f"备份配置文件失败: {e}")
    
    # 写入新内容
    try:
        with open(config_file, 'w', encoding='utf-8') as f:
            f.write(content)
        
        # 重新加载配置
        from src.config import (
            Config, EmbyConfig, ScoreAndRegisterConfig, WebhookConfig,
            DeviceLimitConfig, APIConfig, SecurityConfig, SchedulerConfig,
            NotificationConfig, TelegramConfig, BangumiSyncConfig
        )
        Config.update_from_toml("Global")
        EmbyConfig.update_from_toml('Emby')
        TelegramConfig.update_from_toml('Telegram')
        ScoreAndRegisterConfig.update_from_toml('SAR')
        WebhookConfig.update_from_toml('Webhook')
        DeviceLimitConfig.update_from_toml('DeviceLimit')
        APIConfig.update_from_toml('API')
        SecurityConfig.update_from_toml('Security')
        SchedulerConfig.update_from_toml('Scheduler')
        NotificationConfig.update_from_toml('Notification')
        BangumiSyncConfig.update_from_toml('BangumiSync')
        
        return api_response(True, "配置已更新并重新加载", {
            'path': str(config_file),
        })
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"更新配置文件失败: {e}", exc_info=True)
        
        # 尝试恢复备份
        if backup_file.exists():
            try:
                import shutil
                shutil.copy2(backup_file, config_file)
            except Exception:
                pass
        
        return api_response(False, f"更新配置文件失败: {e}", code=500)


@system_bp.route('/admin/config/schema', methods=['GET'])
@require_auth
@require_admin
async def get_config_schema():
    """获取配置项的结构化描述信息（管理员）"""
    from src.config import BangumiSyncConfig
    
    schema = {
        'sections': [
            {
                'key': 'Global',
                'title': '全局配置',
                'description': '系统全局设置',
                'fields': [
                    {'key': 'server_name', 'label': '服务器名称', 'type': 'string', 'description': '服务器名称，用于前端和通知中显示', 'value': Config.SERVER_NAME},
                    {'key': 'server_icon', 'label': '服务器图标', 'type': 'string', 'description': '服务器图标 URL，留空使用默认', 'value': Config.SERVER_ICON},
                    {'key': 'logging', 'label': '日志开关', 'type': 'bool', 'description': '是否启用日志记录', 'value': Config.LOGGING},
                    {'key': 'log_level', 'label': '日志等级', 'type': 'select', 'description': '日志等级，10=DEBUG, 20=INFO, 30=WARNING, 40=ERROR', 'value': Config.LOG_LEVEL, 'options': [{'label': 'DEBUG', 'value': 10}, {'label': 'INFO', 'value': 20}, {'label': 'WARNING', 'value': 30}, {'label': 'ERROR', 'value': 40}]},
                    {'key': 'sqlalchemy_log', 'label': 'SQLAlchemy 日志', 'type': 'bool', 'description': '是否输出 SQLAlchemy ORM 日志（调试用）', 'value': Config.SQLALCHEMY_LOG},
                    {'key': 'max_retry', 'label': '最大重试次数', 'type': 'int', 'description': 'HTTP 请求失败时的最大重试次数', 'value': Config.MAX_RETRY},
                    {'key': 'databases_dir', 'label': '数据库目录', 'type': 'string', 'description': 'SQLite 数据库文件存储目录', 'value': str(Config.DATABASES_DIR)},
                    {'key': 'redis_url', 'label': 'Redis 连接', 'type': 'string', 'description': 'Redis 连接串，如 redis://localhost:6379/0，留空则使用内存存储', 'value': Config.REDIS_URL},
                    {'key': 'bangumi_token', 'label': 'Bangumi Token', 'type': 'secret', 'description': 'Bangumi API 访问令牌', 'value': Config.BANGUMI_TOKEN},
                    {'key': 'global_bgm_mode', 'label': 'BGM 点格子', 'type': 'bool', 'description': '是否允许用户使用 Bangumi 同步点格子功能', 'value': Config.GLOBAL_BGM_MODE},
                    {'key': 'telegram_mode', 'label': 'Telegram 模式', 'type': 'bool', 'description': '是否启用 Telegram Bot 功能', 'value': Config.TELEGRAM_MODE},
                    {'key': 'email_bind', 'label': '邮箱绑定', 'type': 'bool', 'description': '是否允许用户绑定邮箱', 'value': Config.EMAIL_BIND},
                    {'key': 'force_bind_email', 'label': '强制绑定邮箱', 'type': 'bool', 'description': '是否强制用户绑定邮箱后才能使用', 'value': Config.FORCE_BIND_EMAIL},
                    {'key': 'force_bind_telegram', 'label': '强制绑定 Telegram', 'type': 'bool', 'description': '是否强制用户绑定 Telegram', 'value': Config.FORCE_BIND_TELEGRAM},
                    {'key': 'tmdb_api_key', 'label': 'TMDB API Key', 'type': 'secret', 'description': 'TMDB API Key (v3)，用于获取影视元数据', 'value': Config.TMDB_API_KEY},
                    {'key': 'tmdb_api_url', 'label': 'TMDB API 地址', 'type': 'string', 'description': 'TMDB API 服务器地址', 'value': Config.TMDB_API_URL},
                    {'key': 'tmdb_image_url', 'label': 'TMDB 图片地址', 'type': 'string', 'description': 'TMDB 图片 CDN 地址', 'value': Config.TMDB_IMAGE_URL},
                    {'key': 'bangumi_api_url', 'label': 'Bangumi API 地址', 'type': 'string', 'description': 'Bangumi API 服务器地址', 'value': Config.BANGUMI_API_URL},
                    {'key': 'bangumi_app_id', 'label': 'Bangumi App ID', 'type': 'string', 'description': 'Bangumi OAuth App ID（可选）', 'value': Config.BANGUMI_APP_ID},
                ],
            },
            {
                'key': 'Emby',
                'title': 'Emby 配置',
                'description': 'Emby/Jellyfin 媒体服务器连接配置',
                'fields': [
                    {'key': 'emby_url', 'label': 'Emby 地址', 'type': 'string', 'description': 'Emby 服务器地址，如 http://127.0.0.1:8096/', 'value': EmbyConfig.EMBY_URL},
                    {'key': 'emby_token', 'label': 'API Key', 'type': 'secret', 'description': 'Emby 管理后台生成的 API Key（主要认证方式）', 'value': EmbyConfig.EMBY_TOKEN},
                    {'key': 'emby_username', 'label': '管理员用户名', 'type': 'string', 'description': 'Emby 管理员用户名（API Key 无效时的备用认证）', 'value': EmbyConfig.EMBY_USERNAME},
                    {'key': 'emby_password', 'label': '管理员密码', 'type': 'secret', 'description': 'Emby 管理员密码（API Key 无效时的备用认证）', 'value': EmbyConfig.EMBY_PASSWORD},
                    {'key': 'emby_url_list', 'label': '线路列表', 'type': 'list', 'description': '提供给用户的 Emby 服务器线路列表，格式: "线路名 : URL"', 'value': EmbyConfig.EMBY_URL_LIST},
                    {'key': 'emby_url_list_for_whitelist', 'label': '白名单线路列表', 'type': 'list', 'description': '白名单用户专用的 Emby 服务器线路列表', 'value': EmbyConfig.EMBY_URL_LIST_FOR_WHITELIST},
                    {'key': 'emby_nsfw', 'label': 'NSFW 媒体库', 'type': 'string', 'description': 'NSFW 媒体库名称（需要单独授权的成人内容库）', 'value': EmbyConfig.EMBY_NSFW},
                ],
            },
            {
                'key': 'Telegram',
                'title': 'Telegram 配置',
                'description': 'Telegram Bot 相关设置',
                'fields': [
                    {'key': 'telegram_api_url', 'label': 'API 地址', 'type': 'string', 'description': 'Telegram Bot API 地址，可用于自建 API 代理', 'value': TelegramConfig.TELEGRAM_API_URL},
                    {'key': 'bot_token', 'label': 'Bot Token', 'type': 'secret', 'description': '从 @BotFather 获取的 Bot Token', 'value': TelegramConfig.BOT_TOKEN},
                    {'key': 'admin_id', 'label': '管理员 ID', 'type': 'list', 'description': 'Telegram 管理员用户 ID 列表', 'value': TelegramConfig.ADMIN_ID},
                    {'key': 'group_id', 'label': '群组 ID', 'type': 'list', 'description': 'Telegram 群组 ID 列表', 'value': TelegramConfig.GROUP_ID},
                    {'key': 'channel_id', 'label': '频道 ID', 'type': 'list', 'description': 'Telegram 频道 ID 列表', 'value': TelegramConfig.CHANNEL_ID},
                    {'key': 'force_subscribe', 'label': '强制订阅', 'type': 'bool', 'description': '是否要求用户订阅频道后才能使用', 'value': TelegramConfig.FORCE_SUBSCRIBE},
                ],
            },
            {
                'key': 'SAR',
                'title': '积分与注册',
                'description': '积分系统、注册、签到、自动续期等配置',
                'fields': [
                    {'key': 'score_name', 'label': '积分名称', 'type': 'string', 'description': '积分的显示名称', 'value': ScoreAndRegisterConfig.SCORE_NAME},
                    {'key': 'register_mode', 'label': '注册模式', 'type': 'bool', 'description': '是否开放注册', 'value': ScoreAndRegisterConfig.REGISTER_MODE},
                    {'key': 'register_code_limit', 'label': '注册码限制', 'type': 'bool', 'description': '是否限制必须使用注册码注册', 'value': ScoreAndRegisterConfig.REGISTER_CODE_LIMIT},
                    {'key': 'score_register_mode', 'label': '积分注册', 'type': 'bool', 'description': '是否允许使用积分注册', 'value': ScoreAndRegisterConfig.SCORE_REGISTER_MODE},
                    {'key': 'score_register_need', 'label': '注册所需积分', 'type': 'int', 'description': '注册或激活账号所需的积分数量', 'value': ScoreAndRegisterConfig.SCORE_REGISTER_NEED},
                    {'key': 'user_limit', 'label': '用户上限', 'type': 'int', 'description': '系统允许的最大注册用户数量', 'value': ScoreAndRegisterConfig.USER_LIMIT},
                    {'key': 'new_user_notice_status', 'label': '注册通知', 'type': 'bool', 'description': '用户注册/续期/白名单变更时是否发送通知', 'value': ScoreAndRegisterConfig.NEW_USER_NOTICE_STATUS},
                    {'key': 'new_user_notice_link', 'label': '通知指向主页', 'type': 'bool', 'description': '通知消息是否指向用户个人主页', 'value': ScoreAndRegisterConfig.NEW_USER_NOTICE_LINK},
                    {'key': 'allow_pending_register', 'label': '允许无码注册', 'type': 'bool', 'description': '是否允许无注册码注册（待激活状态）', 'value': ScoreAndRegisterConfig.ALLOW_PENDING_REGISTER},
                    {'key': 'pending_register_bonus', 'label': '无码注册赠送', 'type': 'int', 'description': '无码注册时赠送的初始积分', 'value': ScoreAndRegisterConfig.PENDING_REGISTER_BONUS},
                    {'key': 'allow_no_emby_checkin', 'label': '无Emby签到', 'type': 'bool', 'description': '是否允许未激活 Emby 账户的用户签到', 'value': ScoreAndRegisterConfig.ALLOW_NO_EMBY_CHECKIN},
                    {'key': 'allow_no_emby_view', 'label': '无Emby查看', 'type': 'bool', 'description': '是否允许未激活 Emby 账户的用户查看部分信息', 'value': ScoreAndRegisterConfig.ALLOW_NO_EMBY_VIEW},
                    {'key': 'admin_uids', 'label': '管理员 UID', 'type': 'string', 'description': '管理员 UID 列表，逗号分隔（如 "1,2,3"）', 'value': ScoreAndRegisterConfig.ADMIN_UIDS},
                    {'key': 'admin_usernames', 'label': '管理员用户名', 'type': 'string', 'description': '管理员用户名列表，逗号分隔', 'value': ScoreAndRegisterConfig.ADMIN_USERNAMES},
                    {'key': 'white_list_uids', 'label': '白名单 UID', 'type': 'string', 'description': '白名单 UID 列表，逗号分隔', 'value': ScoreAndRegisterConfig.WHITE_LIST_UIDS},
                    {'key': 'white_list_usernames', 'label': '白名单用户名', 'type': 'string', 'description': '白名单用户名列表，逗号分隔', 'value': ScoreAndRegisterConfig.WHITE_LIST_USERNAMES},
                    {'key': 'red_packet_mode', 'label': '红包功能', 'type': 'bool', 'description': '是否启用红包功能', 'value': ScoreAndRegisterConfig.RED_PACKET_MODE},
                    {'key': 'red_packet_min_amount', 'label': '红包最小金额', 'type': 'int', 'description': '单个红包最小金额', 'value': ScoreAndRegisterConfig.RED_PACKET_MIN_AMOUNT},
                    {'key': 'red_packet_max_amount', 'label': '红包最大金额', 'type': 'int', 'description': '单个红包最大金额', 'value': ScoreAndRegisterConfig.RED_PACKET_MAX_AMOUNT},
                    {'key': 'red_packet_min_count', 'label': '红包最小个数', 'type': 'int', 'description': '红包最小拆分个数', 'value': ScoreAndRegisterConfig.RED_PACKET_MIN_COUNT},
                    {'key': 'red_packet_max_count', 'label': '红包最大个数', 'type': 'int', 'description': '红包最大拆分个数', 'value': ScoreAndRegisterConfig.RED_PACKET_MAX_COUNT},
                    {'key': 'red_packet_expire_hours', 'label': '红包过期时间', 'type': 'int', 'description': '红包过期时间（小时）', 'value': ScoreAndRegisterConfig.RED_PACKET_EXPIRE_HOURS},
                    {'key': 'private_transfer_mode', 'label': '转账功能', 'type': 'bool', 'description': '是否启用用户间转账功能', 'value': ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE},
                    {'key': 'transfer_min_amount', 'label': '最小转账额', 'type': 'int', 'description': '单次最小转账金额', 'value': ScoreAndRegisterConfig.TRANSFER_MIN_AMOUNT},
                    {'key': 'transfer_max_amount', 'label': '最大转账额', 'type': 'int', 'description': '单次最大转账金额', 'value': ScoreAndRegisterConfig.TRANSFER_MAX_AMOUNT},
                    {'key': 'transfer_daily_limit', 'label': '每日转账限额', 'type': 'int', 'description': '每人每日转账限额', 'value': ScoreAndRegisterConfig.TRANSFER_DAILY_LIMIT},
                    {'key': 'transfer_fee_rate', 'label': '转账手续费率', 'type': 'float', 'description': '转账手续费率（0.05 = 5%）', 'value': ScoreAndRegisterConfig.TRANSFER_FEE_RATE},
                    {'key': 'checkin_base_score', 'label': '签到基础奖励', 'type': 'int', 'description': '每日签到基础积分奖励', 'value': ScoreAndRegisterConfig.CHECKIN_BASE_SCORE},
                    {'key': 'checkin_streak_bonus', 'label': '连签加成', 'type': 'int', 'description': '连续签到每天额外加成积分', 'value': ScoreAndRegisterConfig.CHECKIN_STREAK_BONUS},
                    {'key': 'checkin_max_streak_bonus', 'label': '最大连签加成', 'type': 'int', 'description': '连续签到加成的上限', 'value': ScoreAndRegisterConfig.CHECKIN_MAX_STREAK_BONUS},
                    {'key': 'checkin_random_min', 'label': '随机奖励最小', 'type': 'int', 'description': '签到随机奖励最小值', 'value': ScoreAndRegisterConfig.CHECKIN_RANDOM_MIN},
                    {'key': 'checkin_random_max', 'label': '随机奖励最大', 'type': 'int', 'description': '签到随机奖励最大值', 'value': ScoreAndRegisterConfig.CHECKIN_RANDOM_MAX},
                    {'key': 'auto_renew_enabled', 'label': '自动续期', 'type': 'bool', 'description': '是否允许用户开启积分自动续期', 'value': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED},
                    {'key': 'auto_renew_days', 'label': '续期天数', 'type': 'int', 'description': '自动续期延长的天数', 'value': ScoreAndRegisterConfig.AUTO_RENEW_DAYS},
                    {'key': 'auto_renew_cost', 'label': '续期费用', 'type': 'int', 'description': '自动续期扣除的积分', 'value': ScoreAndRegisterConfig.AUTO_RENEW_COST},
                    {'key': 'auto_renew_before_days', 'label': '提前续期天数', 'type': 'int', 'description': '到期前多少天开始自动续期', 'value': ScoreAndRegisterConfig.AUTO_RENEW_BEFORE_DAYS},
                    {'key': 'auto_renew_notify', 'label': '续期通知', 'type': 'bool', 'description': '自动续期后是否通知用户', 'value': ScoreAndRegisterConfig.AUTO_RENEW_NOTIFY},
                    {'key': 'invite_enabled', 'label': '邀请系统', 'type': 'bool', 'description': '是否启用邀请系统', 'value': ScoreAndRegisterConfig.INVITE_ENABLED},
                    {'key': 'invite_reward', 'label': '邀请奖励', 'type': 'int', 'description': '成功邀请一人的积分奖励', 'value': ScoreAndRegisterConfig.INVITE_REWARD},
                    {'key': 'invite_limit', 'label': '邀请上限', 'type': 'int', 'description': '每人最多邀请数量（-1 = 无限制）', 'value': ScoreAndRegisterConfig.INVITE_LIMIT},
                ],
            },
            {
                'key': 'DeviceLimit',
                'title': '设备限制',
                'description': '用户设备和播放流数限制',
                'fields': [
                    {'key': 'device_limit_enabled', 'label': '启用设备限制', 'type': 'bool', 'description': '是否限制用户的设备数量', 'value': DeviceLimitConfig.DEVICE_LIMIT_ENABLED},
                    {'key': 'max_devices', 'label': '最大设备数', 'type': 'int', 'description': '每个用户允许的最大设备数', 'value': DeviceLimitConfig.MAX_DEVICES},
                    {'key': 'max_streams', 'label': '最大同时播放', 'type': 'int', 'description': '每个用户允许的最大同时播放流数', 'value': DeviceLimitConfig.MAX_STREAMS},
                    {'key': 'kick_oldest_session', 'label': '踢出最早会话', 'type': 'bool', 'description': '超过限制时是否自动踢掉最早的会话', 'value': DeviceLimitConfig.KICK_OLDEST_SESSION},
                ],
            },
            {
                'key': 'Webhook',
                'title': 'Webhook 配置',
                'description': 'Webhook 推送和播放统计',
                'fields': [
                    {'key': 'webhook_enabled', 'label': '启用 Webhook', 'type': 'bool', 'description': '是否启用 Webhook 功能', 'value': WebhookConfig.WEBHOOK_ENABLED},
                    {'key': 'webhook_secret', 'label': 'Webhook 密钥', 'type': 'secret', 'description': 'Webhook 请求验证密钥', 'value': WebhookConfig.WEBHOOK_SECRET},
                    {'key': 'webhook_endpoints', 'label': '推送端点', 'type': 'list', 'description': '外部 Webhook 推送 URL 列表', 'value': WebhookConfig.WEBHOOK_ENDPOINTS},
                    {'key': 'playback_stats_enabled', 'label': '播放统计', 'type': 'bool', 'description': '是否启用播放统计功能', 'value': WebhookConfig.PLAYBACK_STATS_ENABLED},
                    {'key': 'ranking_enabled', 'label': '排行榜', 'type': 'bool', 'description': '是否启用排行榜功能', 'value': WebhookConfig.RANKING_ENABLED},
                    {'key': 'ranking_public', 'label': '公开排行榜', 'type': 'bool', 'description': '排行榜是否对未登录用户公开', 'value': WebhookConfig.RANKING_PUBLIC},
                ],
            },
            {
                'key': 'API',
                'title': 'API 服务器',
                'description': 'Web API 服务器配置',
                'fields': [
                    {'key': 'host', 'label': '监听地址', 'type': 'string', 'description': 'API 服务器监听地址（0.0.0.0 表示所有接口）', 'value': APIConfig.HOST},
                    {'key': 'port', 'label': '端口', 'type': 'int', 'description': 'API 服务器监听端口', 'value': APIConfig.PORT},
                    {'key': 'debug', 'label': '调试模式', 'type': 'bool', 'description': '是否开启调试模式（生产环境请关闭）', 'value': APIConfig.DEBUG},
                    {'key': 'token_expire', 'label': 'Token 有效期', 'type': 'int', 'description': '用户登录 Token 有效期（秒）', 'value': APIConfig.TOKEN_EXPIRE},
                    {'key': 'api_key_length', 'label': 'API Key 长度', 'type': 'int', 'description': '生成的 API Key 字符长度', 'value': APIConfig.API_KEY_LENGTH},
                    {'key': 'cors_enabled', 'label': '启用 CORS', 'type': 'bool', 'description': '是否允许跨域请求', 'value': APIConfig.CORS_ENABLED},
                    {'key': 'cors_origins', 'label': 'CORS 白名单', 'type': 'list', 'description': '允许跨域请求的源地址列表，留空则允许所有', 'value': APIConfig.CORS_ORIGINS},
                ],
            },
            {
                'key': 'Security',
                'title': '安全配置',
                'description': '登录安全与 IP 限制',
                'fields': [
                    {'key': 'ip_limit_enabled', 'label': 'IP 限制', 'type': 'bool', 'description': '是否启用用户 IP 数量限制', 'value': SecurityConfig.IP_LIMIT_ENABLED},
                    {'key': 'max_ips_per_user', 'label': '每用户最大 IP', 'type': 'int', 'description': '每个用户允许的最大 IP 地址数', 'value': SecurityConfig.MAX_IPS_PER_USER},
                    {'key': 'login_fail_threshold', 'label': '登录失败阈值', 'type': 'int', 'description': '连续登录失败多少次后锁定账号', 'value': SecurityConfig.LOGIN_FAIL_THRESHOLD},
                    {'key': 'lockout_minutes', 'label': '锁定时间', 'type': 'int', 'description': '账号锁定持续时间（分钟）', 'value': SecurityConfig.LOCKOUT_MINUTES},
                    {'key': 'log_all_logins', 'label': '记录所有登录', 'type': 'bool', 'description': '是否记录所有登录行为', 'value': SecurityConfig.LOG_ALL_LOGINS},
                ],
            },
            {
                'key': 'Scheduler',
                'title': '定时任务',
                'description': '定时任务执行时间配置',
                'fields': [
                    {'key': 'timezone', 'label': '时区', 'type': 'string', 'description': '定时任务使用的时区', 'value': SchedulerConfig.TIMEZONE},
                    {'key': 'enabled', 'label': '启用定时任务', 'type': 'bool', 'description': '是否启用定时任务系统', 'value': SchedulerConfig.ENABLED},
                    {'key': 'expired_check_time', 'label': '过期检查时间', 'type': 'string', 'description': '检查过期用户的时间（HH:MM 格式）', 'value': SchedulerConfig.EXPIRED_CHECK_TIME},
                    {'key': 'expiring_check_time', 'label': '即将过期检查', 'type': 'string', 'description': '检查即将过期用户的时间（HH:MM 格式）', 'value': SchedulerConfig.EXPIRING_CHECK_TIME},
                    {'key': 'auto_renew_time', 'label': '自动续期时间', 'type': 'string', 'description': '执行自动续期的时间（HH:MM 格式）', 'value': SchedulerConfig.AUTO_RENEW_TIME},
                    {'key': 'daily_stats_time', 'label': '统计汇总时间', 'type': 'string', 'description': '每日统计汇总的时间（HH:MM 格式）', 'value': SchedulerConfig.DAILY_STATS_TIME},
                    {'key': 'session_cleanup_interval', 'label': '会话清理间隔', 'type': 'int', 'description': '会话清理任务的执行间隔（小时）', 'value': SchedulerConfig.SESSION_CLEANUP_INTERVAL},
                    {'key': 'emby_sync_interval', 'label': 'Emby 同步间隔', 'type': 'int', 'description': 'Emby 用户数据同步的执行间隔（小时）', 'value': SchedulerConfig.EMBY_SYNC_INTERVAL},
                ],
            },
            {
                'key': 'Notification',
                'title': '通知配置',
                'description': '系统通知相关设置',
                'fields': [
                    {'key': 'enabled', 'label': '启用通知', 'type': 'bool', 'description': '是否启用通知系统', 'value': NotificationConfig.ENABLED},
                    {'key': 'expiry_remind_days', 'label': '到期提醒天数', 'type': 'int', 'description': '提前多少天提醒用户即将到期', 'value': NotificationConfig.EXPIRY_REMIND_DAYS},
                    {'key': 'daily_ranking_time', 'label': '排行榜推送时间', 'type': 'string', 'description': '每日排行榜推送时间（HH:MM 格式，留空不推送）', 'value': NotificationConfig.DAILY_RANKING_TIME},
                    {'key': 'new_media_notify', 'label': '新媒体通知', 'type': 'bool', 'description': '有新媒体入库时是否通知', 'value': NotificationConfig.NEW_MEDIA_NOTIFY},
                ],
            },
            {
                'key': 'BangumiSync',
                'title': 'Bangumi 同步',
                'description': 'Bangumi 观看记录同步设置',
                'fields': [
                    {'key': 'enabled', 'label': '启用同步', 'type': 'bool', 'description': '是否启用 Bangumi 观看记录同步', 'value': BangumiSyncConfig.ENABLED},
                    {'key': 'auto_add_collection', 'label': '自动收藏', 'type': 'bool', 'description': '同步时是否自动添加到 Bangumi 收藏（在看）', 'value': BangumiSyncConfig.AUTO_ADD_COLLECTION},
                    {'key': 'private_collection', 'label': '私有收藏', 'type': 'bool', 'description': '观看记录是否设为 Bangumi 私有', 'value': BangumiSyncConfig.PRIVATE_COLLECTION},
                    {'key': 'block_keywords', 'label': '屏蔽关键词', 'type': 'list', 'description': '不同步的条目关键词列表', 'value': BangumiSyncConfig.BLOCK_KEYWORDS},
                    {'key': 'min_progress_percent', 'label': '最小播放进度', 'type': 'int', 'description': '播放进度达到多少百分比才算看完并同步', 'value': BangumiSyncConfig.MIN_PROGRESS_PERCENT},
                ],
            },
        ],
    }
    
    return api_response(True, "获取成功", schema)


@system_bp.route('/admin/config/schema', methods=['PUT'])
@require_auth
@require_admin
async def update_config_by_schema():
    """通过结构化数据更新配置（管理员）"""
    import toml
    from src.config import ROOT_PATH, BangumiSyncConfig
    
    data = request.get_json() or {}
    sections = data.get('sections', {})
    
    if not sections:
        return api_response(False, "缺少配置数据", code=400)
    
    config_file = ROOT_PATH / 'config.toml'
    
    # 读取当前配置
    try:
        config = toml.load(config_file)
    except Exception as e:
        return api_response(False, f"读取配置文件失败: {e}", code=500)
    
    # 备份原文件
    backup_file = ROOT_PATH / 'config.toml.backup'
    try:
        if config_file.exists():
            import shutil
            shutil.copy2(config_file, backup_file)
    except Exception as e:
        import logging
        logging.getLogger(__name__).warning(f"备份配置文件失败: {e}")
    
    # 更新配置
    for section_key, fields in sections.items():
        if section_key not in config:
            config[section_key] = {}
        for field_key, value in fields.items():
            config[section_key][field_key] = value
    
    # 写入文件
    try:
        with open(config_file, 'w', encoding='utf-8') as f:
            toml.dump(config, f)
        
        # 重新加载所有配置
        Config.update_from_toml("Global")
        EmbyConfig.update_from_toml('Emby')
        TelegramConfig.update_from_toml('Telegram')
        ScoreAndRegisterConfig.update_from_toml('SAR')
        WebhookConfig.update_from_toml('Webhook')
        DeviceLimitConfig.update_from_toml('DeviceLimit')
        APIConfig.update_from_toml('API')
        SecurityConfig.update_from_toml('Security')
        SchedulerConfig.update_from_toml('Scheduler')
        NotificationConfig.update_from_toml('Notification')
        BangumiSyncConfig.update_from_toml('BangumiSync')
        
        return api_response(True, "配置已更新并重新加载")
    except Exception as e:
        import logging
        logging.getLogger(__name__).error(f"更新配置文件失败: {e}", exc_info=True)
        
        # 尝试恢复备份
        if backup_file.exists():
            try:
                import shutil
                shutil.copy2(backup_file, config_file)
            except Exception:
                pass
        
        return api_response(False, f"更新配置文件失败: {e}", code=500)


@system_bp.route('/admin/apis', methods=['GET'])
@require_auth
@require_admin
async def list_all_apis():
    """获取所有 API 列表（管理员）"""
    from flask import current_app
    
    apis = []
    
    # 遍历所有注册的蓝图和路由
    for rule in current_app.url_map.iter_rules():
        # 过滤掉静态文件、根路径和 OPTIONS 方法
        if rule.endpoint == 'static' or rule.rule == '/' or 'OPTIONS' in rule.methods:
            continue
        
        # 只获取 /api/v1 开头的路由
        if not rule.rule.startswith('/api/v1'):
            continue
        
        # 获取方法
        methods = [m for m in rule.methods if m != 'OPTIONS' and m != 'HEAD']
        if not methods:
            continue
        
        # 构建路径（移除 /api/v1 前缀以便前端使用）
        path = rule.rule[7:]  # 移除 '/api/v1'
        
        for method in methods:
            apis.append({
                'method': method,
                'path': path,
                'endpoint': rule.endpoint,
                'full_path': rule.rule,
            })
    
    # 按路径和方法排序
    apis.sort(key=lambda x: (x['path'], x['method']))
    
    return api_response(True, "获取成功", {
        'apis': apis,
        'total': len(apis),
    })


@system_bp.route('/admin/emby/libraries', methods=['GET'])
@require_auth
@require_admin
async def get_emby_libraries():
    """获取所有 Emby 媒体库列表（管理员）"""
    from src.services import EmbyService
    
    libraries = await EmbyService.get_libraries_info()
    return api_response(True, "获取成功", libraries)


@system_bp.route('/admin/emby/nsfw', methods=['PUT'])
@require_auth
@require_admin
async def update_nsfw_library():
    """更新 NSFW 库配置（管理员），使用库名称标识"""
    import toml
    from pathlib import Path
    from src.config import ROOT_PATH
    from src.services import EmbyService
    from src.services.emby import get_emby_client, EmbyError
    
    data = request.get_json() or {}
    library_name = data.get('library_name', '').strip()
    
    config_file = ROOT_PATH / 'config.toml'
    
    if not config_file.exists():
        return api_response(False, "配置文件不存在", code=404)
    
    # 如果提供了库名称，验证它在 Emby 中存在
    if library_name:
        try:
            emby = get_emby_client()
            libraries = await emby.get_libraries()
            matched = any(lib.name.strip().lower() == library_name.lower() for lib in libraries)
            if not matched:
                return api_response(False, f"Emby 中不存在名为 '{library_name}' 的媒体库", code=400)
        except EmbyError as e:
            return api_response(False, f"无法连接 Emby 验证媒体库: {e}", code=500)
    
    try:
        # 读取现有配置
        config = toml.load(config_file)
        
        # 更新 NSFW 库名称
        if 'Emby' not in config:
            config['Emby'] = {}
        config['Emby']['emby_nsfw'] = library_name
        
        # 备份原文件
        backup_file = ROOT_PATH / 'config.toml.backup'
        if config_file.exists():
            import shutil
            shutil.copy2(config_file, backup_file)
        
        # 写入新配置
        with open(config_file, 'w', encoding='utf-8') as f:
            toml.dump(config, f)
        
        # 重新加载配置
        EmbyConfig.update_from_toml('Emby')
        
        return api_response(True, "NSFW 库配置已更新", {
            'nsfw_library_name': library_name,
        })
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"更新 NSFW 库配置失败: {e}", exc_info=True)
        
        # 尝试恢复备份
        backup_file = ROOT_PATH / 'config.toml.backup'
        if backup_file.exists():
            try:
                import shutil
                shutil.copy2(backup_file, config_file)
            except Exception:
                pass
        
        return api_response(False, f"更新配置失败: {e}", code=500)