"""
系统信息 API

提供系统配置、状态等信息
"""
from flask import Blueprint, request, g

from src.api.v1.auth import async_route, require_auth, require_admin, api_response
from src.config import (
    Config, EmbyConfig, ScoreAndRegisterConfig, WebhookConfig,
    DeviceLimitConfig, APIConfig, SecurityConfig, SchedulerConfig,
    NotificationConfig, TelegramConfig
)
from src import __version__

system_bp = Blueprint('system', __name__, url_prefix='/system')


# ==================== 公开信息 ====================

@system_bp.route('/info', methods=['GET'])
@async_route
async def get_system_info():
    """
    获取系统公开信息
    
    不需要登录即可访问
    """
    return api_response(True, "获取成功", {
        'name': 'Twilight',
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
@async_route
async def health_check():
    """健康检查"""
    from src.services import get_emby_client
    
    status = {
        'api': True,
        'database': True,
        'emby': False,
    }
    
    # 检查 Emby 连接
    try:
        emby = get_emby_client()
        info = await emby.get_public_info()
        status['emby'] = bool(info)
    except Exception:
        pass
    
    all_healthy = all(status.values())
    
    return api_response(all_healthy, "OK" if all_healthy else "部分服务异常", status)


@system_bp.route('/emby-urls', methods=['GET'])
@async_route
async def get_emby_urls():
    """获取 Emby 服务器地址列表"""
    return api_response(True, "获取成功", {
        'urls': EmbyConfig.EMBY_URL_LIST,
    })


# ==================== 需要登录 ====================

@system_bp.route('/config', methods=['GET'])
@async_route
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
@async_route
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
@async_route
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
    
    # 注册码统计
    regcodes = await RegCodeOperate.get_all_regcodes()
    active_codes = len([c for c in regcodes if c.ACTIVE])
    
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
        'regcodes': {
            'total': len(regcodes),
            'active': active_codes,
        },
        'emby': emby_status,
    })

