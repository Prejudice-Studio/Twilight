"""
配置管理模块

提供基于TOML文件的配置管理功能
"""
import logging
import os
from pathlib import Path
from typing import List, Union, Any, Optional

import toml

logger = logging.getLogger(__name__)

ROOT_PATH: Path = Path(__file__).parent.parent.resolve()


class BaseConfig:
    """
    配置管理的基类
    
    提供从TOML文件读取和保存配置的能力
    """
    toml_file_path: str = os.path.join(ROOT_PATH, 'config.toml')
    _section: Optional[str] = None

    @classmethod
    def update_from_toml(cls, section: Optional[str] = None) -> None:
        """
        从TOML配置文件中加载配置
        
        :param section: TOML文件中的配置节名称，为None时加载根级配置
        """
        try:
            cls._section = section
            config = toml.load(cls.toml_file_path)
            items = config.get(section, {}) if section else config
            
            for key, value in items.items():
                attr_name = key.upper()
                if hasattr(cls, attr_name):
                    setattr(cls, attr_name, value)
                    
        except FileNotFoundError:
            logger.warning(f'配置文件不存在: {cls.toml_file_path}')
        except toml.TomlDecodeError as err:
            logger.error(f'TOML配置文件格式错误: {err}')
        except Exception as err:
            logger.error(f'加载配置文件时发生错误: {err}')

    @classmethod
    def save_to_toml(cls) -> bool:
        """
        将当前配置保存到TOML文件
        
        :return: 保存是否成功
        """
        try:
            # 读取现有配置
            try:
                config = toml.load(cls.toml_file_path)
            except FileNotFoundError:
                config = {}

            # 收集类的配置属性
            config_data = {}
            for key in dir(cls):
                if key.isupper() and not key.startswith('_'):
                    config_data[key.lower()] = getattr(cls, key)

            # 更新配置
            if cls._section:
                if cls._section not in config:
                    config[cls._section] = {}
                config[cls._section].update(config_data)
            else:
                config.update(config_data)

            # 写入文件
            with open(cls.toml_file_path, 'w', encoding='utf-8') as f:
                toml.dump(config, f)
            return True
            
        except Exception as err:
            logger.error(f'保存配置文件时发生错误: {err}')
            return False

    @classmethod
    def get(cls, key: str, default: Any = None) -> Any:
        """
        获取配置值
        
        :param key: 配置键名（不区分大小写）
        :param default: 默认值
        :return: 配置值
        """
        return getattr(cls, key.upper(), default)


class Config(BaseConfig):
    """全局配置管理类"""
    LOGGING: bool = True
    LOG_LEVEL: int = 20  # 日志等级，数字越大，日志越详细
    SQLALCHEMY_LOG: bool = False
    PROXY: Optional[str] = None
    MAX_RETRY: int = 3
    DATABASES_DIR: Path = ROOT_PATH / 'db'
    BANGUMI_TOKEN: str = ''
    GLOBAL_BGM_MODE: bool = False  # 是否允许BGM点格子
    TELEGRAM_MODE: bool = False
    EMAIL_BIND: bool = False
    FORCE_BIND_EMAIL: bool = False
    FORCE_BIND_TELEGRAM: bool = True
    # TMDB 配置
    TMDB_API_KEY: str = ''  # TMDB API Key (v3)
    TMDB_API_URL: str = 'https://api.themoviedb.org/3'
    TMDB_IMAGE_URL: str = 'https://image.tmdb.org/t/p'
    # Bangumi 配置
    BANGUMI_API_URL: str = 'https://api.bgm.tv'
    BANGUMI_APP_ID: str = ''  # Bangumi App ID (可选)


class EmbyConfig(BaseConfig):
    """Emby配置管理类"""
    EMBY_URL: str = 'http://127.0.0.1:8096/'
    EMBY_TOKEN: str = ''
    EMBY_URL_LIST: List[str] = [
        'Direct : http://127.0.0.1:8096/',
        'Sample : http://192.168.1.1:8096/'
    ]
    EMBY_URL_LIST_FOR_WHITELIST: List[str] = [
        'Direct : http://127.0.0.1:8096/',
        'Sample : http://192.168.1.1:8096/'
    ]
    EMBY_NSFW: str = ''


class TelegramConfig(BaseConfig):
    """Telegram配置管理类"""
    TELEGRAM_API_URL: str = 'https://api.telegram.org/bot'
    BOT_TOKEN: str = ''
    ADMIN_ID: Union[int, List[int]] = []
    GROUP_ID: Union[int, List[int]] = []
    CHANNEL_ID: Union[int, List[int]] = []
    FORCE_SUBSCRIBE: bool = False


class ScoreAndRegisterConfig(BaseConfig):
    """积分及注册配置管理类"""
    SCORE_NAME: str = '暮光币'
    REGISTER_MODE: bool = False
    REGISTER_CODE_LIMIT: bool = False  # 是否限制注册码注册
    SCORE_REGISTER_MODE: bool = False
    SCORE_REGISTER_NEED: int = 100  # 注册所需积分
    USER_LIMIT: int = 200  # 允许的已注册用户数量上限
    NEW_USER_NOTICE_STATUS: bool = False  # 用户注册/续期/白名单通知开关
    NEW_USER_NOTICE_LINK: bool = False  # 通知是否指向个人主页
    
    # 红包配置
    RED_PACKET_MODE: bool = False
    RED_PACKET_MIN_AMOUNT: int = 1  # 红包最小金额
    RED_PACKET_MAX_AMOUNT: int = 10000  # 红包最大金额
    RED_PACKET_MIN_COUNT: int = 1  # 红包最小个数
    RED_PACKET_MAX_COUNT: int = 100  # 红包最大个数
    RED_PACKET_EXPIRE_HOURS: int = 24  # 红包过期时间（小时）
    
    # 转账配置
    PRIVATE_TRANSFER_MODE: bool = False
    TRANSFER_MIN_AMOUNT: int = 1  # 最小转账金额
    TRANSFER_MAX_AMOUNT: int = 10000  # 最大转账金额
    TRANSFER_DAILY_LIMIT: int = 50000  # 每日转账限额
    TRANSFER_FEE_RATE: float = 0.0  # 转账手续费率 (0.05 = 5%)
    
    # 签到配置
    CHECKIN_BASE_SCORE: int = 10  # 签到基础奖励
    CHECKIN_STREAK_BONUS: int = 2  # 连签每天加成
    CHECKIN_MAX_STREAK_BONUS: int = 20  # 最大连签加成
    CHECKIN_RANDOM_MIN: int = 0  # 随机奖励最小值
    CHECKIN_RANDOM_MAX: int = 5  # 随机奖励最大值
    
    # 积分自动续期
    AUTO_RENEW_ENABLED: bool = False  # 是否允许积分自动续期
    AUTO_RENEW_DAYS: int = 30  # 自动续期天数
    AUTO_RENEW_COST: int = 100  # 自动续期所需积分
    AUTO_RENEW_BEFORE_DAYS: int = 3  # 到期前几天自动续期
    AUTO_RENEW_NOTIFY: bool = True  # 续期后是否通知用户
    
    # 邀请系统
    INVITE_ENABLED: bool = False  # 是否启用邀请系统
    INVITE_REWARD: int = 50  # 邀请奖励积分
    INVITE_LIMIT: int = 10  # 每人最多邀请数量 (-1 = 无限制)


class DeviceLimitConfig(BaseConfig):
    """设备限制配置"""
    DEVICE_LIMIT_ENABLED: bool = False  # 是否启用设备限制
    MAX_DEVICES: int = 5  # 最大设备数
    MAX_STREAMS: int = 2  # 最大同时播放数
    KICK_OLDEST_SESSION: bool = False  # 超限时是否踢掉最早的会话


class WebhookConfig(BaseConfig):
    """Webhook 配置管理类"""
    WEBHOOK_ENABLED: bool = True  # 是否启用 Webhook
    WEBHOOK_SECRET: str = ''  # Webhook 验证密钥
    WEBHOOK_ENDPOINTS: List[str] = []  # 外部推送端点列表
    # 播放统计
    PLAYBACK_STATS_ENABLED: bool = True  # 是否启用播放统计
    # 排行榜
    RANKING_ENABLED: bool = True  # 是否启用排行榜
    RANKING_PUBLIC: bool = True  # 排行榜是否公开（不需要登录）


class APIConfig(BaseConfig):
    """API 服务器配置"""
    HOST: str = "0.0.0.0"
    PORT: int = 5000
    DEBUG: bool = False
    TOKEN_EXPIRE: int = 86400  # Token 过期时间（秒）
    API_KEY_LENGTH: int = 32
    CORS_ENABLED: bool = True
    CORS_ORIGINS: List[str] = []


class SecurityConfig(BaseConfig):
    """安全配置"""
    IP_LIMIT_ENABLED: bool = False  # 是否启用 IP 限制
    MAX_IPS_PER_USER: int = 10  # 每用户最大 IP 数
    LOGIN_FAIL_THRESHOLD: int = 5  # 登录失败锁定阈值
    LOCKOUT_MINUTES: int = 30  # 锁定时间
    LOG_ALL_LOGINS: bool = True  # 是否记录所有登录


class SchedulerConfig(BaseConfig):
    """定时任务配置"""
    TIMEZONE: str = "Asia/Shanghai"
    ENABLED: bool = True
    EXPIRED_CHECK_TIME: str = "03:00"
    EXPIRING_CHECK_TIME: str = "09:00"
    AUTO_RENEW_TIME: str = "02:00"
    DAILY_STATS_TIME: str = "00:05"
    SESSION_CLEANUP_INTERVAL: int = 6


class NotificationConfig(BaseConfig):
    """通知配置"""
    ENABLED: bool = True
    EXPIRY_REMIND_DAYS: int = 3
    DAILY_RANKING_TIME: str = ""
    NEW_MEDIA_NOTIFY: bool = False


# 自动加载配置
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
