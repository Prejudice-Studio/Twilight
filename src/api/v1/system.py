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
    """更新 NSFW 库配置（管理员）"""
    import toml
    from pathlib import Path
    from src.config import ROOT_PATH
    
    data = request.get_json() or {}
    nsfw_library_id = data.get('library_id', '')
    
    config_file = ROOT_PATH / 'config.toml'
    
    if not config_file.exists():
        return api_response(False, "配置文件不存在", code=404)
    
    try:
        # 读取现有配置
        config = toml.load(config_file)
        
        # 更新 NSFW 库 ID
        if 'Emby' not in config:
            config['Emby'] = {}
        config['Emby']['emby_nsfw'] = nsfw_library_id
        
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
            'nsfw_library_id': nsfw_library_id,
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