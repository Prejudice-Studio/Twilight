"""
配置管理模块

提供基于TOML文件和环境变量的配置管理功能
"""
import logging
import os
from pathlib import Path
from typing import List, Union, Any, Optional

import toml

# 从 .env 文件加载环境变量（如果存在）
try:
    from dotenv import load_dotenv
    load_dotenv()
except ImportError:
    pass  # python-dotenv 未安装，继续使用系统环境变量

logger = logging.getLogger(__name__)

ROOT_PATH: Path = Path(__file__).parent.parent.resolve()


class BaseConfig:
    """
    配置管理的基类
    
    提供从TOML文件读取和保存配置的能力
    """
    toml_file_path: str = str(ROOT_PATH / 'config.toml')
    toml_override_file_path: str = str(ROOT_PATH / 'config.local.toml')
    _section: Optional[str] = None

    @classmethod
    def _merge_dict(cls, base: dict, override: dict) -> dict:
        """递归合并 dict（override 覆盖 base）。"""
        result = dict(base)
        for key, value in (override or {}).items():
            if isinstance(value, dict) and isinstance(result.get(key), dict):
                result[key] = cls._merge_dict(result[key], value)
            else:
                result[key] = value
        return result

    @classmethod
    def _load_toml_config(cls) -> dict:
        """加载配置：主配置 + 本地覆盖配置。"""
        config: dict = {}

        primary_path = os.environ.get("TWILIGHT_CONFIG_FILE", cls.toml_file_path)
        local_override_path = os.environ.get(
            "TWILIGHT_CONFIG_LOCAL_FILE",
            cls.toml_override_file_path,
        )

        try:
            config = toml.load(primary_path)
        except FileNotFoundError:
            logger.warning(f'配置文件不存在: {primary_path}')
        except toml.TomlDecodeError as err:
            logger.error(f'TOML配置文件格式错误 ({primary_path}): {err}')
        except Exception as err:
            logger.error(f'加载配置文件时发生错误 ({primary_path}): {err}')

        if local_override_path:
            try:
                override_config = toml.load(local_override_path)
                config = cls._merge_dict(config, override_config)
                logger.debug(f"已加载本地覆盖配置: {local_override_path}")
            except FileNotFoundError:
                pass
            except toml.TomlDecodeError as err:
                logger.error(f'TOML配置文件格式错误 ({local_override_path}): {err}')
            except Exception as err:
                logger.error(f'加载本地覆盖配置时发生错误 ({local_override_path}): {err}')

        return config

    @classmethod
    def update_from_toml(cls, section: Optional[str] = None) -> None:
        """
        从TOML配置文件和环境变量中加载配置
        
        :param section: TOML文件中的配置节名称，为None时加载根级配置
        """
        cls._section = section
        config = cls._load_toml_config()
            
        items = config.get(section, {}) if section else config
        
        # 2. 从类属性更新（合并 TOML 与类默认值）
        for key in dir(cls):
            if not key.isupper() or key.startswith('_'):
                continue
                
            attr_name = key
            toml_key = key.lower()
            
            # 优先级: 环境变量 > TOML > 类默认值
            
            # 获取 TOML 值
            value = items.get(toml_key)
            
            # 获取环境变量值
            env_prefix = f"TWILIGHT_{section.upper()}_" if section else "TWILIGHT_"
            env_key = env_prefix + attr_name
            env_value = os.environ.get(env_key)
            
            if env_value is not None:
                # 环境变量转换类型
                current_value = getattr(cls, attr_name)
                try:
                    if isinstance(current_value, bool):
                        value = env_value.lower() in ('true', '1', 'yes', 'on')
                    elif isinstance(current_value, int):
                        value = int(env_value)
                    elif isinstance(current_value, float):
                        value = float(env_value)
                    elif isinstance(current_value, list):
                        value = [v.strip() for v in env_value.split(',')]
                    else:
                        value = env_value
                except ValueError:
                    logger.warning(f"无法将环境变量 {env_key} 的值 {env_value} 转换为 {type(current_value)}")
            
            if value is not None:
                # 如果原始值是 Path 类型，将字符串转换为 Path
                current_value = getattr(cls, attr_name)
                if isinstance(current_value, Path) and isinstance(value, str):
                    value = ROOT_PATH / value if not os.path.isabs(value) else Path(value)
                setattr(cls, attr_name, value)

    @classmethod
    def _serialize_config_value(cls, value: Any) -> Any:
        if isinstance(value, Path):
            try:
                return str(value.relative_to(ROOT_PATH))
            except ValueError:
                return str(value)
        return value

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
                    config_data[key.lower()] = cls._serialize_config_value(
                        getattr(cls, key)
                    )

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

    @classmethod
    def _get_default_values(cls) -> dict:
        """获取类定义的所有默认配置键值（小写键名 -> 默认值）"""
        defaults = {}
        # 遍历 MRO 获取原始类定义的默认值
        for klass in reversed(cls.__mro__):
            for key, value in vars(klass).items():
                if key.isupper() and not key.startswith('_'):
                    defaults[key.lower()] = value
        return defaults

    @classmethod
    def fill_missing_to_toml(cls) -> bool:
        """
        检查 TOML 文件中是否缺少当前类定义的配置项，
        如缺少则用类默认值补全并写回文件。
        
        :return: 是否有新增配置项
        """
        if not cls._section:
            return False
        
        try:
            config = toml.load(cls.toml_file_path)
        except (FileNotFoundError, toml.TomlDecodeError):
            config = {}
        
        section_data = config.get(cls._section, {})
        defaults = cls._get_default_values()
        
        missing = {}
        for key, default_value in defaults.items():
            if key not in section_data:
                # 将 Path 转为字符串以便 TOML 序列化
                if isinstance(default_value, Path):
                    default_value = cls._serialize_config_value(default_value)
                missing[key] = default_value
        
        if not missing:
            return False
        
        # 补全缺失项
        if cls._section not in config:
            config[cls._section] = {}
        config[cls._section].update(missing)
        
        try:
            with open(cls.toml_file_path, 'w', encoding='utf-8') as f:
                toml.dump(config, f)
            logger.info(f"[{cls._section}] 已补全 {len(missing)} 个缺失配置项: {', '.join(missing.keys())}")
            return True
        except Exception as err:
            logger.error(f"补全配置文件时发生错误: {err}")
            return False


class Config(BaseConfig):
    """全局配置管理类"""
    SERVER_NAME: str = 'Twilight'  # 服务器名称，用于前端显示
    SERVER_ICON: str = ''  # 服务器图标 URL，用于前端显示
    LOGGING: bool = True
    LOG_LEVEL: int = 20  # 日志等级，数字越大，日志越详细
    SQLALCHEMY_LOG: bool = False
    MAX_RETRY: int = 3
    DATABASES_DIR: Path = ROOT_PATH / 'db'
    REDIS_URL: str = ''  # Token/缓存存储的 Redis 连接串，如 redis://localhost:6379/0
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
    EMBY_USERNAME: str = ''  # 管理员用户名（API Key 无效时的备用认证）
    EMBY_PASSWORD: str = ''  # 管理员密码（API Key 无效时的备用认证）
    EMBY_URL_LIST: List[str] = [
        'Direct : http://127.0.0.1:8096/',
        'Sample : http://192.168.1.1:8096/'
    ]
    EMBY_URL_LIST_FOR_WHITELIST: List[str] = [
        'Direct : http://127.0.0.1:8096/',
        'Sample : http://192.168.1.1:8096/'
    ]


class TelegramConfig(BaseConfig):
    """Telegram配置管理类"""
    TELEGRAM_API_URL: str = 'https://api.telegram.org/bot'
    BOT_TOKEN: str = ''
    BIND_CONFIRM_API_URL: str = ''  # Bot 绑定确认回调地址（可填完整接口或后端基础地址）
    ADMIN_ID: Union[int, List[int]] = []
    GROUP_ID: Union[int, str, List[Union[int, str]]] = []  # 支持数字ID或 @channelusername
    CHANNEL_ID: Union[int, str, List[Union[int, str]]] = []  # 支持数字ID或 @channelusername
    FORCE_SUBSCRIBE: bool = False
    PROXY_URL: str = ''  # HTTP 代理地址，如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080
    ENABLE_TG_PANEL: bool = False  # 是否开启 TG Bot 完整面板（关闭时仅允许绑定和查看基础信息）
    REQUIRE_GROUP_MEMBERSHIP: bool = False  # 是否强制要求绑定/已绑定用户保持在配置中的群组内
    GROUP_CHECK_INTERVAL_MINUTES: int = 30  # 定时检查间隔（分钟），开启上面开关后生效


class RegisterConfig(BaseConfig):
    """注册及用户策略配置管理类"""
    REGISTER_MODE: bool = False
    REGISTER_CODE_LIMIT: bool = False  # 是否限制注册码注册
    USER_LIMIT: int = 200  # 允许的已注册用户数量上限
    MAX_CONCURRENT_REQUESTS_PER_USER: int = -1  # 每个用户允许同时存在的求片请求上限，-1 表示不限制
    
    # 无码注册（待激活）配置
    ALLOW_PENDING_REGISTER: bool = True  # 是否允许无码注册（待激活状态）
    ALLOW_NO_EMBY_VIEW: bool = True  # 是否允许无 Emby 账户的用户查看部分信息
    EMBY_DIRECT_REGISTER_ENABLED: bool = False  # 是否开启 Emby 自由注册
    EMBY_DIRECT_REGISTER_DAYS: int = 30  # Emby 自由注册默认开通天数
    EMBY_DIRECT_REGISTER_WORKERS: int = 8  # Emby 自由注册队列 worker 数
    EMBY_DIRECT_REGISTER_MAX_QUEUE: int = 1000  # Emby 自由注册队列最大排队数
    EMBY_DIRECT_REGISTER_STATUS_TTL: int = 1800  # Emby 自由注册状态保留秒数
    
    # 管理员配置（二选一，优先使用 UID）
    ADMIN_UIDS: str = ''  # 管理员 UID 列表，逗号分隔（推荐，如 "1,2,3"）
    ADMIN_USERNAMES: str = ''  # 管理员用户名列表，逗号分隔（如 "admin,superuser"）
    
    # 白名单配置（二选一，优先使用 UID）
    WHITE_LIST_UIDS: str = ''  # 白名单 UID 列表，逗号分隔（如 "10,11,12"）
    WHITE_LIST_USERNAMES: str = ''  # 白名单用户名列表，逗号分隔（如 "vip1,vip2"）
    
    # 无 Emby 账户用户自动清理
    AUTO_CLEANUP_NO_EMBY: bool = False  # 是否自动清理没有 Emby 账户的用户
    AUTO_CLEANUP_NO_EMBY_DAYS: int = 7  # 注册后多少天未创建 Emby 账户则自动删除

    # 邀请系统（树状邀请：用户 B 生成 Emby 注册码，A 使用后成为 B 的下级）
    INVITE_ENABLED: bool = False  # 是否启用邀请系统（关闭时所有邀请相关 API 直接返回禁用）
    INVITE_LIMIT: int = 10  # 每人最多邀请数量 (-1 = 无限制)
    INVITE_MAX_DEPTH: int = 3  # 邀请树最大层级，B->A->C 计为 3 层。1 表示禁止任何邀请
    INVITE_REQUIRE_EMBY: bool = True  # 是否要求邀请人已绑定 Emby 账号才能生码
    INVITE_CODE_DEFAULT_DAYS: int = 30  # 被邀请人 Emby 账号的默认开通天数


class DeviceLimitConfig(BaseConfig):
    """设备限制配置"""
    DEVICE_LIMIT_ENABLED: bool = False  # 是否启用设备限制
    MAX_DEVICES: int = 5  # 最大设备数
    MAX_STREAMS: int = 2  # 最大同时播放数
    KICK_OLDEST_SESSION: bool = False  # 超限时是否踢掉最早的会话


class APIConfig(BaseConfig):
    """API 服务器配置"""
    HOST: str = "0.0.0.0"
    PORT: int = 5000
    DEBUG: bool = False
    TOKEN_EXPIRE: int = 864000  # Token 过期时间（秒）
    API_KEY_LENGTH: int = 32
    CORS_ENABLED: bool = True
    CORS_ORIGINS: List[str] = ["*"]
    UPLOAD_FOLDER: str = str(ROOT_PATH / 'uploads')  # 文件上传目录
    MAX_UPLOAD_SIZE: int = 5 * 1024 * 1024  # 最大上传文件大小（字节）
    SESSION_COOKIE_NAME: str = 'twilight_session'
    SESSION_COOKIE_SECURE: bool = False
    SESSION_COOKIE_SAMESITE: str = 'Lax'  # Strict / Lax / None
    SESSION_COOKIE_DOMAIN: str = ''
    SESSION_COOKIE_PATH: str = '/'


class SecurityConfig(BaseConfig):
    """安全配置"""
    LOGIN_FAIL_THRESHOLD: int = 5  # 登录失败锁定阈值
    LOCKOUT_MINUTES: int = 30  # 锁定时间
    TELEGRAM_DIRECT_LOGIN_ENABLED: bool = False  # 是否允许仅凭 telegram_id 直接登录
    APIKEY_DIRECT_LOGIN_ENABLED: bool = False  # 是否允许通过 API Key 直接换取完整会话 token
    BOT_INTERNAL_SECRET: str = ''  # Bot 调用内部接口的密钥（建议显式配置）


class SchedulerConfig(BaseConfig):
    """定时任务配置"""
    TIMEZONE: str = "Asia/Shanghai"
    ENABLED: bool = True
    EXPIRED_CHECK_TIME: str = "03:00"
    EXPIRING_CHECK_TIME: str = "09:00"
    DAILY_STATS_TIME: str = "00:05"
    SESSION_CLEANUP_INTERVAL: int = 6
    EMBY_SYNC_INTERVAL: int = 6


class NotificationConfig(BaseConfig):
    """通知配置"""
    ENABLED: bool = True
    EXPIRY_REMIND_DAYS: int = 3
    NEW_MEDIA_NOTIFY: bool = False


class BangumiSyncConfig(BaseConfig):
    """Bangumi 同步配置"""
    ENABLED: bool = False  # 是否启用 Bangumi 同步
    AUTO_ADD_COLLECTION: bool = True  # 同步时是否自动添加到收藏（设为"在看"）
    PRIVATE_COLLECTION: bool = False  # 观看记录是否设为私有
    BLOCK_KEYWORDS: List[str] = []  # 屏蔽关键词列表
    MIN_PROGRESS_PERCENT: int = 80  # 最小播放进度（百分比）才算看完


class SigninConfig(BaseConfig):
    """签到与积分系统配置（积分仅装饰用途，无排行榜）"""
    ENABLED: bool = True
    CURRENCY_NAME: str = '星币'  # 货币展示名
    DAILY_MIN: int = 5  # 每日签到最少奖励
    DAILY_MAX: int = 20  # 每日签到最多奖励
    # 连签加成：达到列表中的连签天数（含当日），额外奖励对应位置的积分
    STREAK_BONUS_DAYS: List[int] = [3, 7, 14, 30]
    STREAK_BONUS_POINTS: List[int] = [10, 50, 100, 300]
    RESET_AFTER_MISS: bool = True  # 漏签是否清零连签


# 自动加载配置
Config.update_from_toml("Global")
EmbyConfig.update_from_toml('Emby')
TelegramConfig.update_from_toml('Telegram')
RegisterConfig.update_from_toml('SAR')
DeviceLimitConfig.update_from_toml('DeviceLimit')
APIConfig.update_from_toml('API')
SecurityConfig.update_from_toml('Security')
SchedulerConfig.update_from_toml('Scheduler')
NotificationConfig.update_from_toml('Notification')
BangumiSyncConfig.update_from_toml('BangumiSync')
SigninConfig.update_from_toml('Signin')

# 启动时自动补全缺失的配置项
_config_classes = [
    Config, EmbyConfig, TelegramConfig, RegisterConfig,
    DeviceLimitConfig, APIConfig, SecurityConfig,
    SchedulerConfig, NotificationConfig, BangumiSyncConfig,
    SigninConfig,
]
_filled = sum(1 for c in _config_classes if c.fill_missing_to_toml())
if _filled:
    logger.info(f"已补全 {_filled} 个配置节的缺失项")
