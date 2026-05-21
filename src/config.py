"""
配置管理模块

提供基于TOML文件和环境变量的配置管理功能
"""

import logging
import os
import shutil
from datetime import datetime
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


def resolve_storage_path(value: Union[str, Path], field_name: str) -> Path:
    """解析并规范化存储路径。

    - 相对路径: 相对于项目根目录，并要求最终路径仍位于项目根目录内。
    - 绝对路径: 允许使用，按 ``resolve`` 规范化。
    """
    raw = Path(value) if isinstance(value, Path) else Path(str(value or "").strip())
    if not str(raw):
        raise ValueError(f"{field_name} 不能为空")

    if raw.is_absolute():
        return raw.resolve()

    resolved = (ROOT_PATH / raw).resolve()
    try:
        resolved.relative_to(ROOT_PATH)
    except ValueError as exc:
        raise ValueError(f"{field_name} 使用相对路径时不能逃逸项目目录: {value}") from exc
    return resolved


def get_primary_config_path() -> Path:
    """返回主配置文件路径（支持环境变量覆盖）。"""
    return Path(os.environ.get("TWILIGHT_CONFIG_FILE", str(ROOT_PATH / "config.toml")))


def _restrict_perms(path: Path) -> None:
    """把文件权限收紧到 0o600（仅当前用户可读写）。

    Windows 下 ``os.chmod`` 只控制只读属性，效果有限；Linux 下能正确生效，
    防止 config 备份里的 secret 被 group/other 读到。失败不抛错——某些
    文件系统（如 FAT）不支持 chmod。
    """
    try:
        os.chmod(path, 0o600)
    except Exception as exc:  # pragma: no cover
        logger.debug(f"chmod 600 失败（可忽略）{path}: {exc}")


# 备份目录最多保留多少份历史 *.bak；超过的按 mtime 从旧到新淘汰。
# 启动期 sweep + 后台 fill-missing + 管理员手动保存都会触发备份，必须设上限
# 避免长跑实例把磁盘塞满 / 让用户在恢复时迷失在几百份历史里。
_CONFIG_BACKUP_RETENTION = int(os.environ.get("TWILIGHT_CONFIG_BACKUP_RETENTION", "20"))


def _trim_backup_dir(backup_dir: Path, keep: int) -> int:
    """保留 ``backup_dir`` 里最近 ``keep`` 个 *.bak 文件，旧的删除。

    返回实际删除的数量。``keep <= 0`` 视为不裁剪（全部保留）。
    """
    if keep <= 0 or not backup_dir.is_dir():
        return 0
    try:
        entries = [p for p in backup_dir.iterdir() if p.is_file() and p.suffix == ".bak"]
    except Exception as exc:  # pragma: no cover
        logger.debug(f"读取备份目录失败 {backup_dir}: {exc}")
        return 0
    if len(entries) <= keep:
        return 0
    entries.sort(key=lambda p: p.stat().st_mtime, reverse=True)
    removed = 0
    for stale in entries[keep:]:
        try:
            stale.unlink()
            removed += 1
        except Exception as exc:  # pragma: no cover - safety
            logger.debug(f"清理旧备份失败 {stale}: {exc}")
    return removed


def backup_config_file(config_path: Optional[Path] = None, reason: str = "manual") -> Optional[Path]:
    """创建配置备份（时间戳轮转 + 兼容单文件 backup + 数量上限裁剪）。

    备份文件可能包含 ``bot_token`` / ``emby_token`` 等敏感字段，写出后立刻
    chmod 0o600，避免 ``config_backups/`` 整个目录被同机其它账号读取。

    备份目录里最多保留 ``_CONFIG_BACKUP_RETENTION`` 份历史（环境变量
    ``TWILIGHT_CONFIG_BACKUP_RETENTION`` 可覆盖，<=0 表示不裁剪）。
    """
    path = Path(config_path) if config_path else get_primary_config_path()
    if not path.exists():
        return None

    safe_reason = "".join(ch for ch in str(reason) if ch.isalnum() or ch in ("-", "_")) or "manual"
    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    backup_dir = path.parent / "config_backups"
    backup_dir.mkdir(parents=True, exist_ok=True)
    rotated_backup = backup_dir / f"{path.name}.{timestamp}.{safe_reason}.bak"

    try:
        shutil.copy2(path, rotated_backup)
        _restrict_perms(rotated_backup)
    except Exception as err:
        logger.warning(f"创建轮转备份失败: {err}")
        return None

    # 兼容旧逻辑：保留一个固定 backup 文件，便于人工快速恢复
    legacy_backup = path.parent / f"{path.name}.backup"
    try:
        shutil.copy2(path, legacy_backup)
        _restrict_perms(legacy_backup)
    except Exception as err:
        logger.warning(f"更新兼容备份文件失败: {err}")

    # 裁剪：超出保留份数的最旧备份直接删除。
    removed = _trim_backup_dir(backup_dir, _CONFIG_BACKUP_RETENTION)
    if removed:
        logger.info(
            "已清理 %d 份过旧的配置备份 (目录=%s, 保留=%d)",
            removed,
            backup_dir,
            _CONFIG_BACKUP_RETENTION,
        )

    return rotated_backup


class BaseConfig:
    """
    配置管理的基类

    提供从TOML文件读取和保存配置的能力
    """

    toml_file_path: str = str(ROOT_PATH / "config.toml")
    toml_override_file_path: str = str(ROOT_PATH / "config.local.toml")
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
            logger.warning(f"配置文件不存在: {primary_path}")
        except toml.TomlDecodeError as err:
            logger.error(f"TOML配置文件格式错误 ({primary_path}): {err}")
        except Exception as err:
            logger.error(f"加载配置文件时发生错误 ({primary_path}): {err}")

        if local_override_path:
            try:
                override_config = toml.load(local_override_path)
                config = cls._merge_dict(config, override_config)
                logger.debug(f"已加载本地覆盖配置: {local_override_path}")
            except FileNotFoundError:
                pass
            except toml.TomlDecodeError as err:
                logger.error(f"TOML配置文件格式错误 ({local_override_path}): {err}")
            except Exception as err:
                logger.error(f"加载本地覆盖配置时发生错误 ({local_override_path}): {err}")

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
            if not key.isupper() or key.startswith("_"):
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
                        value = env_value.lower() in ("true", "1", "yes", "on")
                    elif isinstance(current_value, int):
                        value = int(env_value)
                    elif isinstance(current_value, float):
                        value = float(env_value)
                    elif isinstance(current_value, list):
                        value = [v.strip() for v in env_value.split(",")]
                    else:
                        value = env_value
                except ValueError:
                    logger.warning(f"无法将环境变量 {env_key} 的值 {env_value} 转换为 {type(current_value)}")

            if value is not None:
                # 如果原始值是 Path 类型，将字符串转换为 Path
                current_value = getattr(cls, attr_name)
                if isinstance(current_value, Path) and isinstance(value, str):
                    try:
                        value = resolve_storage_path(value, f"{section or 'Global'}.{toml_key}")
                    except ValueError as err:
                        logger.warning(str(err))
                        continue
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
            primary_path = get_primary_config_path()
            # 读取现有配置
            try:
                config = toml.load(primary_path)
            except FileNotFoundError:
                config = {}

            # 收集类的配置属性
            config_data = {}
            for key in dir(cls):
                if key.isupper() and not key.startswith("_"):
                    config_data[key.lower()] = cls._serialize_config_value(getattr(cls, key))

            # 更新配置
            if cls._section:
                if cls._section not in config:
                    config[cls._section] = {}
                config[cls._section].update(config_data)
            else:
                config.update(config_data)

            # 写入文件
            with open(primary_path, "w", encoding="utf-8") as f:
                toml.dump(config, f)
            return True

        except Exception as err:
            logger.error(f"保存配置文件时发生错误: {err}")
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
                if key.isupper() and not key.startswith("_"):
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

        primary_path = get_primary_config_path()

        try:
            config = toml.load(primary_path)
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
            with open(primary_path, "w", encoding="utf-8") as f:
                toml.dump(config, f)
            logger.info(f"[{cls._section}] 已补全 {len(missing)} 个缺失配置项: {', '.join(missing.keys())}")
            return True
        except Exception as err:
            logger.error(f"补全配置文件时发生错误: {err}")
            return False


class Config(BaseConfig):
    """全局配置管理类"""

    _section = "Global"
    SERVER_NAME: str = "Twilight"  # 服务器名称，用于前端显示
    SERVER_ICON: str = ""  # 服务器图标 URL，用于前端显示
    LOGGING: bool = True
    LOG_LEVEL: int = 20  # 日志等级，数字越大，日志越详细
    SQLALCHEMY_LOG: bool = False
    MAX_RETRY: int = 3
    DATABASES_DIR: Path = ROOT_PATH / "db"
    REDIS_URL: str = ""  # Token/缓存存储的 Redis 连接串，如 redis://localhost:6379/0
    BANGUMI_TOKEN: str = ""
    TELEGRAM_MODE: bool = False
    FORCE_BIND_TELEGRAM: bool = True
    # TMDB 配置
    TMDB_API_KEY: str = ""  # TMDB API Key (v3)
    TMDB_API_URL: str = "https://api.themoviedb.org/3"
    TMDB_IMAGE_URL: str = "https://image.tmdb.org/t/p"
    # Bangumi 配置
    BANGUMI_API_URL: str = "https://api.bgm.tv"
    BANGUMI_APP_ID: str = ""  # Bangumi App ID (可选)


class EmbyConfig(BaseConfig):
    """Emby配置管理类"""

    _section = "Emby"
    EMBY_URL: str = "http://127.0.0.1:8096/"
    EMBY_TOKEN: str = ""
    EMBY_USERNAME: str = ""  # 管理员用户名（API Key 无效时的备用认证）
    EMBY_PASSWORD: str = ""  # 管理员密码（API Key 无效时的备用认证）
    EMBY_URL_LIST: List[str] = ["Direct : http://127.0.0.1:8096/", "Sample : http://192.168.1.1:8096/"]
    EMBY_URL_LIST_FOR_WHITELIST: List[str] = ["Direct : http://127.0.0.1:8096/", "Sample : http://192.168.1.1:8096/"]
    # 新建/补建普通用户 Emby 账号后默认隐藏的媒体库名称；留空不改媒体库策略。
    EMBY_DEFAULT_HIDDEN_LIBRARIES: List[str] = []
    # 管理员开放给“已授予自助显隐权限的用户”自行显示/隐藏的媒体库名称；留空则无法自助操作。
    EMBY_SELF_SERVICE_LIBRARIES: List[str] = []


class TelegramConfig(BaseConfig):
    """Telegram配置管理类"""

    _section = "Telegram"
    TELEGRAM_API_URL: str = "https://api.telegram.org/bot"
    BOT_TOKEN: str = ""
    BIND_CONFIRM_API_URL: str = ""  # Bot 绑定确认回调地址（可填完整接口或后端基础地址）
    ADMIN_ID: Union[int, List[int]] = []
    GROUP_ID: Union[int, str, List[Union[int, str]]] = []  # 支持数字ID或 @channelusername
    CHANNEL_ID: Union[int, str, List[Union[int, str]]] = []  # 支持数字ID或 @channelusername
    FORCE_SUBSCRIBE: bool = False
    PROXY_URL: str = ""  # HTTP 代理地址，如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080
    ENABLE_TG_PANEL: bool = False  # 是否开启 TG Bot 完整面板（关闭时仅允许绑定和查看基础信息）
    REQUIRE_GROUP_MEMBERSHIP: bool = False  # 是否强制要求绑定/已绑定用户保持在配置中的群组内
    GROUP_CHECK_INTERVAL_MINUTES: int = 30  # 定时检查间隔（分钟），开启上面开关后生效
    GROUP_CHECK_CONCURRENCY: int = 24  # 群组成员资格巡检 get_chat_member 并发数
    GROUP_ACTION_CONCURRENCY: int = 8  # 群组禁用/踢出/封禁等写操作并发数
    # 退群完全封禁模式：开启后，定时巡检发现某绑定用户已离开必需群组时，
    # 除禁用本地账号 + Emby 外，还会对该 TG 用户在所有 GROUP_ID 列出的群里
    # 执行 ban_chat_member（不 unban），使其无法重新加入。默认关闭，谨慎开启。
    # 依赖：Bot 必须是群管理员且具有"封禁成员"权限；BAN_ON_LEAVE 开启后
    # 巡检任务会跳过"重新入群识别"分支（永封后该分支永远不会命中）。
    BAN_ON_LEAVE: bool = False
    # 回群自动启用：关闭时只记录回群候选，管理员手动恢复；开启后巡检确认回群且未到期时自动启用。
    # BAN_ON_LEAVE=true 时永封模式优先，本配置不会生效。
    AUTO_ENABLE_REJOINED: bool = False
    # —— Bot 文案自定义（留空使用内置默认）。所有字符串都按 Markdown 渲染，
    # 注意自行转义 _ * [ 等特殊字符；其中可以用 {server_name}/{user_name}/{bot_username} 占位符。
    BOT_START_TEXT: str = ""  # /start 完整文本，留空使用内置默认
    BOT_GROUP_START_TEXT: str = ""  # 群组内 /start 提示文本，留空使用内置默认
    BOT_START_TITLE: str = ""  # /start 标题行，例如 "🌙 {server_name} 控制中心"
    BOT_START_INTRO: str = ""  # /start 简介段，例如 "欢迎使用 Emby 管理机器人"
    BOT_BIND_PROMPT_TEXT: str = ""  # /bind 无参数时的绑定码输入提示，留空使用内置默认
    BOT_HELP_TEXT: str = ""  # /twihelp 完整文本，留空使用内置默认
    BOT_ADMIN_HELP_TEXT: str = ""  # /twishelp 完整文本，留空使用内置默认
    BOT_HELP_HEADER: str = ""  # /help 顶部段（旧配置，命令列表前），可用于公告
    BOT_HELP_FOOTER: str = ""  # /help 底部段（旧配置），可放群组链接、规则等
    BOT_ABOUT: str = ""  # 关于 Bot / 站点说明（暂留给将来 /about 使用）


class RegisterConfig(BaseConfig):
    """注册及用户策略配置管理类（含原 [Signin] 节字段）"""

    _section = "SAR"
    REGISTER_MODE: bool = False
    REGISTER_CODE_LIMIT: bool = False  # 是否限制注册码注册
    USER_LIMIT: int = 200  # 允许的已注册用户数量上限
    MEDIA_REQUEST_ENABLED: bool = False  # 是否启用求片功能
    MAX_CONCURRENT_REQUESTS_PER_USER: int = -1  # 每个用户允许同时存在的求片请求上限，-1 表示不限制
    REGCODE_FORMAT: str = "code-{random}"  # 卡码生成格式，支持 {random}/{type}/{days}/{index}
    REGCODE_RANDOM_ALGORITHM: str = "base32-20"  # 推荐 base32-20；兼容 hex20/base32-16/alnum-16/digits-12/uuid/legacy-sha1
    REGCODE_DECOY_ACTION: str = "disable_user"  # none/disable_user/disable_user_and_deactivate_code

    # 无码注册（待激活）配置
    ALLOW_PENDING_REGISTER: bool = True  # 是否允许无码注册（待激活状态）
    ALLOW_NO_EMBY_VIEW: bool = True  # 是否允许无 Emby 账户的用户查看部分信息
    EMBY_DIRECT_REGISTER_ENABLED: bool = False  # 是否开启 Emby 自由注册
    EMBY_DIRECT_REGISTER_DAYS: int = 30  # Emby 自由注册统一开通天数（管理员单值，-1=永久）
    # 已绑定 / 待开通 / 队列待创建 Emby 的本站用户总上限（-1=不限制）。所有路径都走这一个值：
    #   1) /users/me/emby/register 自由注册队列
    #   2) /users/me/emby/bind 绑定已有 Emby 账号
    #   3) 管理员 /admin/users/{uid}/bind-emby 强制绑定
    # 拒绝再拆出"绑定专属上限"——业务上 Emby 容量是同一个计数，多上限只会让运维心累。
    EMBY_USER_LIMIT: int = -1
    EMBY_DIRECT_REGISTER_WORKERS: int = 8  # Emby 自由注册队列 worker 数
    EMBY_DIRECT_REGISTER_MAX_QUEUE: int = 1000  # Emby 自由注册队列最大排队数
    EMBY_DIRECT_REGISTER_STATUS_TTL: int = 1800  # Emby 自由注册状态保留秒数

    # 管理员配置（二选一，优先使用 UID）
    ADMIN_UIDS: str = ""  # 管理员 UID 列表，逗号分隔（推荐，如 "1,2,3"）
    ADMIN_USERNAMES: str = ""  # 管理员用户名列表，逗号分隔（如 "admin,superuser"）

    # 白名单配置（二选一，优先使用 UID）
    WHITE_LIST_UIDS: str = ""  # 白名单 UID 列表，逗号分隔（如 "10,11,12"）
    WHITE_LIST_USERNAMES: str = ""  # 白名单用户名列表，逗号分隔（如 "vip1,vip2"）

    # 无 Emby 账户用户自动清理
    AUTO_CLEANUP_NO_EMBY: bool = False  # 是否自动清理没有 Emby 账户的用户
    AUTO_CLEANUP_NO_EMBY_DAYS: int = 7  # 注册后多少天未创建 Emby 账户则自动删除

    # 邀请系统（树状邀请：用户 B 生成 Emby 注册码，A 使用后成为 B 的下级）
    INVITE_ENABLED: bool = False  # 是否启用邀请系统（关闭时所有邀请相关 API 直接返回禁用）
    INVITE_LIMIT: int = 10  # 每人最多同时存在的未使用邀请码数量 (-1 = 无限制)
    INVITE_ROOT_USER_LIMIT: int = -1  # 每棵邀请树最多可成功邀请的用户数，不含树根 (-1 = 无限制)
    INVITE_MAX_DEPTH: int = 3  # 邀请树最大层级，B->A->C 计为 3 层。1 表示禁止任何邀请
    INVITE_REQUIRE_EMBY: bool = True  # 是否要求邀请人已绑定 Emby 账号才能生码
    INVITE_CODE_DEFAULT_DAYS: int = 30  # 被邀请人 Emby 账号的默认开通天数

    # ───────── 签到 / 积分（原 [Signin] 节并入此处）─────────
    # 重命名说明：[Signin].enabled → SAR.signin_enabled，避免与 SAR 节的语义重名；
    # 其余字段（currency_name / daily_min / ...）按原名保留，调用点改读 RegisterConfig。
    SIGNIN_ENABLED: bool = True  # 签到功能开关（原 [Signin].enabled）
    CURRENCY_NAME: str = "星币"  # 货币展示名
    DAILY_MIN: int = 5  # 每日签到最少奖励
    DAILY_MAX: int = 20  # 每日签到最多奖励
    # 连签加成总开关：关闭后即使 STREAK_BONUS_DAYS / STREAK_BONUS_POINTS 有值也不发放
    STREAK_BONUS_ENABLED: bool = True
    STREAK_BONUS_DAYS: List[int] = [3, 7, 14, 30]
    STREAK_BONUS_POINTS: List[int] = [10, 50, 100, 300]
    RESET_AFTER_MISS: bool = True  # 漏签是否清零连签


class DeviceLimitConfig(BaseConfig):
    """设备限制配置"""

    _section = "DeviceLimit"
    DEVICE_LIMIT_ENABLED: bool = False  # 是否启用设备限制
    MAX_DEVICES: int = 5  # 最大设备数
    MAX_STREAMS: int = 2  # 最大同时播放数
    KICK_OLDEST_SESSION: bool = False  # 超限时是否踢掉最早的会话


class APIConfig(BaseConfig):
    """API 服务器配置"""

    _section = "API"
    HOST: str = "0.0.0.0"
    PORT: int = 5000
    DEBUG: bool = False
    TOKEN_EXPIRE: int = 864000  # Token 过期时间（秒）
    CORS_ENABLED: bool = True
    CORS_ORIGINS: List[str] = ["*"]
    UPLOAD_FOLDER: str = str(ROOT_PATH / "uploads")  # 文件上传目录
    MAX_UPLOAD_SIZE: int = 5 * 1024 * 1024  # 最大上传文件大小（字节）
    SESSION_COOKIE_NAME: str = "twilight_session"
    SESSION_COOKIE_SECURE: bool = False
    SESSION_COOKIE_SAMESITE: str = "Lax"  # Strict / Lax / None
    SESSION_COOKIE_DOMAIN: str = ""
    SESSION_COOKIE_PATH: str = "/"


def normalize_storage_settings() -> None:
    """规范化数据库目录与上传目录路径。"""
    try:
        Config.DATABASES_DIR = resolve_storage_path(
            Config.DATABASES_DIR,
            "Global.databases_dir",
        )
    except ValueError as err:
        logger.warning("%s，回退默认数据库目录", err)
        Config.DATABASES_DIR = (ROOT_PATH / "db").resolve()

    try:
        upload_dir = resolve_storage_path(
            APIConfig.UPLOAD_FOLDER,
            "API.upload_folder",
        )
    except ValueError as err:
        logger.warning("%s，回退默认上传目录", err)
        upload_dir = (ROOT_PATH / "uploads").resolve()
    APIConfig.UPLOAD_FOLDER = str(upload_dir)


class SecurityConfig(BaseConfig):
    """安全配置"""

    _section = "Security"
    LOGIN_FAIL_THRESHOLD: int = 5  # 登录失败锁定阈值
    LOCKOUT_MINUTES: int = 30  # 锁定时间
    TELEGRAM_DIRECT_LOGIN_ENABLED: bool = False  # 是否允许仅凭 telegram_id 直接登录
    APIKEY_DIRECT_LOGIN_ENABLED: bool = False  # 是否允许通过 API Key 直接换取完整会话 token
    BOT_INTERNAL_SECRET: str = ""  # Bot 调用内部接口的密钥（建议显式配置）


class SchedulerConfig(BaseConfig):
    """定时任务配置"""

    _section = "Scheduler"
    TIMEZONE: str = "Asia/Shanghai"
    ENABLED: bool = True
    EXPIRED_CHECK_TIME: str = "03:00"
    EXPIRING_CHECK_TIME: str = "09:00"
    DAILY_STATS_TIME: str = "00:05"
    SESSION_CLEANUP_INTERVAL: int = 6
    EMBY_SYNC_INTERVAL: int = 6


class SystemUpdateConfig(BaseConfig):
    """系统在线更新配置"""

    _section = "SystemUpdate"
    AUTO_UPDATE_ENABLED: bool = False
    REPO_URL: str = "https://github.com/Prejudice-Studio/Twilight.git"
    BRANCH: str = "main"
    RESTART_SERVICES: bool = True
    AUTO_UPDATE_TRIGGER_TYPE: str = "interval"  # interval / cron_daily
    AUTO_UPDATE_INTERVAL_HOURS: int = 24
    AUTO_UPDATE_TIME: str = "04:00"


class NotificationConfig(BaseConfig):
    """通知配置"""

    _section = "Notification"
    ENABLED: bool = True
    EXPIRY_REMIND_DAYS: int = 3
    NEW_MEDIA_NOTIFY: bool = False


class BangumiSyncConfig(BaseConfig):
    """Bangumi 同步配置"""

    _section = "BangumiSync"
    ENABLED: bool = False  # 是否启用 Bangumi 同步
    WEBHOOK_SECRET: str = ""  # Emby Webhook 共享密钥；配置后请求必须携带 ?token= 或 X-Twilight-Bangumi-Token
    AUTO_ADD_COLLECTION: bool = True  # 同步时是否自动添加到收藏（设为"在看"）
    PRIVATE_COLLECTION: bool = False  # 观看记录是否设为私有
    BLOCK_KEYWORDS: List[str] = []  # 屏蔽关键词列表
    MIN_PROGRESS_PERCENT: int = 80  # 最小播放进度（百分比）才算看完


# （历史 [Signin] 节已并入 [SAR]；SigninConfig 类删除）


# ============================================================
#  迁移 / 清理：处理历史遗留 section 与字段名
# ============================================================


# 历史 section.key → 新 section.key 映射；用于把弃用节里的字段搬到新归属
_LEGACY_SECTION_KEY_MIGRATIONS: dict[tuple[str, str], tuple[str, str]] = {
    # [Signin] 节并入 [SAR]
    ("Signin", "enabled"): ("SAR", "signin_enabled"),
    ("Signin", "currency_name"): ("SAR", "currency_name"),
    ("Signin", "daily_min"): ("SAR", "daily_min"),
    ("Signin", "daily_max"): ("SAR", "daily_max"),
    ("Signin", "streak_bonus_enabled"): ("SAR", "streak_bonus_enabled"),
    ("Signin", "streak_bonus_days"): ("SAR", "streak_bonus_days"),
    ("Signin", "streak_bonus_points"): ("SAR", "streak_bonus_points"),
    ("Signin", "reset_after_miss"): ("SAR", "reset_after_miss"),
}


def _apply_legacy_migrations(config: dict) -> tuple[bool, list[str]]:
    """就地把已弃用 section / 字段搬到新归属。返回 ``(变动, 迁移日志)``。"""
    changed = False
    notes: list[str] = []
    legacy_sections_seen: set[str] = set()

    for (old_section, old_key), (new_section, new_key) in _LEGACY_SECTION_KEY_MIGRATIONS.items():
        old_block = config.get(old_section)
        if not isinstance(old_block, dict) or old_key not in old_block:
            continue
        value = old_block.pop(old_key)
        new_block = config.setdefault(new_section, {})
        if not isinstance(new_block, dict):
            # 新 section 在 toml 里被写成了非表格类型，跳过避免毁坏数据
            old_block[old_key] = value
            continue
        # 已经存在新键时优先保留新值，丢弃旧值（提示一下）
        if new_key in new_block:
            notes.append(f"[{old_section}].{old_key} 已在 [{new_section}].{new_key} 存在，丢弃旧值")
        else:
            new_block[new_key] = value
            notes.append(f"[{old_section}].{old_key} → [{new_section}].{new_key}")
        changed = True
        legacy_sections_seen.add(old_section)

    # 旧 section 搬空之后顺手清空残留键的 dict（让 prune 阶段把它整段删掉）
    for section in legacy_sections_seen:
        block = config.get(section)
        if isinstance(block, dict) and not block:
            del config[section]
            changed = True

    return changed, notes


def _collect_known_section_keys() -> dict[str, set[str]]:
    """返回 ``{section: 该 section 在代码里声明的合法键集合}``。"""
    known: dict[str, set[str]] = {}
    for cls in _config_classes:
        section = getattr(cls, "_section", None)
        if not section:
            continue
        known[section] = set(cls._get_default_values().keys())
    return known


def _prune_stale_keys(config: dict) -> tuple[bool, dict[str, list[str]]]:
    """删除 toml 里已经不再被任何配置类声明的 section / 字段。

    返回 ``(是否变更, {section: [被删字段...]})``，``__section_removed__`` 列出整段被删的 section。
    Global 节比较特殊：它在 toml 里平铺在根，``_collect_known_section_keys`` 拿到的是该
    类的字段集，我们用同一逻辑校验根级键。
    """
    known = _collect_known_section_keys()
    global_keys = known.get("Global", set())
    removed: dict[str, list[str]] = {}
    changed = False
    section_removed: list[str] = []

    # 根级（[Global] 的字段平铺在 toml 根，没有专门的 section header）
    for key in list(config.keys()):
        value = config.get(key)
        if isinstance(value, dict):
            continue  # 是 section header，留给下面的逻辑
        if key not in global_keys:
            removed.setdefault("Global", []).append(key)
            del config[key]
            changed = True

    # 各 section
    for section, block in list(config.items()):
        if not isinstance(block, dict):
            continue
        valid = known.get(section)
        if valid is None:
            # 整段都不再被代码声明：整段移除
            del config[section]
            section_removed.append(section)
            changed = True
            continue
        for key in list(block.keys()):
            if key not in valid:
                removed.setdefault(section, []).append(key)
                del block[key]
                changed = True

    if section_removed:
        removed["__section_removed__"] = section_removed
    return changed, removed


# 启动时自动补全缺失的配置项
_config_classes = [
    Config,
    EmbyConfig,
    TelegramConfig,
    RegisterConfig,
    DeviceLimitConfig,
    APIConfig,
    SecurityConfig,
    SchedulerConfig,
    SystemUpdateConfig,
    NotificationConfig,
    BangumiSyncConfig,
]


def _load_all_configs() -> None:
    """把所有 BaseConfig 子类从 toml 重新加载一次（顺序敏感：先 Global 再 normalize）。"""
    Config.update_from_toml("Global")
    EmbyConfig.update_from_toml("Emby")
    TelegramConfig.update_from_toml("Telegram")
    RegisterConfig.update_from_toml("SAR")
    DeviceLimitConfig.update_from_toml("DeviceLimit")
    APIConfig.update_from_toml("API")
    SecurityConfig.update_from_toml("Security")
    SchedulerConfig.update_from_toml("Scheduler")
    SystemUpdateConfig.update_from_toml("SystemUpdate")
    NotificationConfig.update_from_toml("Notification")
    BangumiSyncConfig.update_from_toml("BangumiSync")
    normalize_storage_settings()


def reload_runtime_config() -> None:
    """公开的运行时配置热重载入口。"""
    _load_all_configs()


def fill_missing_config_items(
    config_classes: Optional[List[type]] = None,
    auto_backup: bool = False,
) -> dict:
    """补全所有配置节的缺失项，并可选在写回前自动备份。"""
    classes = config_classes or _config_classes
    primary_path = get_primary_config_path()

    try:
        config = toml.load(primary_path)
    except FileNotFoundError:
        config = {}
    except toml.TomlDecodeError as err:
        logger.error(f"配置文件格式错误，跳过缺项补全 ({primary_path}): {err}")
        return {"filled_sections": 0, "filled_items": 0, "backup_path": None, "error": str(err)}

    missing_by_section: dict[str, list[str]] = {}
    filled_items = 0

    for conf_cls in classes:
        section = getattr(conf_cls, "_section", None)
        if not section:
            continue

        raw_section_data = config.get(section, {})
        section_data = raw_section_data if isinstance(raw_section_data, dict) else {}
        defaults = conf_cls._get_default_values()

        section_missing: dict[str, Any] = {}
        for key, default_value in defaults.items():
            if key in section_data:
                continue
            if isinstance(default_value, Path):
                default_value = conf_cls._serialize_config_value(default_value)
            section_missing[key] = default_value

        if not section_missing:
            continue

        if section not in config or not isinstance(config.get(section), dict):
            config[section] = {}
        config[section].update(section_missing)
        missing_by_section[section] = sorted(section_missing.keys())
        filled_items += len(section_missing)

    if filled_items == 0:
        return {"filled_sections": 0, "filled_items": 0, "backup_path": None}

    backup_path: Optional[Path] = None
    if auto_backup and primary_path.exists():
        backup_path = backup_config_file(primary_path, reason="fill-missing")

    try:
        with open(primary_path, "w", encoding="utf-8") as f:
            toml.dump(config, f)
    except Exception as err:
        logger.error(f"写回补全后的配置失败: {err}")
        return {
            "filled_sections": 0,
            "filled_items": 0,
            "backup_path": str(backup_path) if backup_path else None,
            "error": str(err),
        }

    for section, keys in missing_by_section.items():
        logger.info(f"[{section}] 已补全 {len(keys)} 个缺失配置项: {', '.join(keys)}")

    return {
        "filled_sections": len(missing_by_section),
        "filled_items": filled_items,
        "backup_path": str(backup_path) if backup_path else None,
    }


def sweep_config_toml(
    *,
    config_classes: Optional[List[type]] = None,
    auto_backup: bool = True,
) -> dict:
    """一次性把 config.toml 整理干净：迁移老 section、删无效条目、补缺失默认。

    顺序：``读 toml → 迁移历史字段 → 删陌生 section/字段 → 用类默认补齐 → 备份 → 写回``。
    只有真的变更时才会触碰文件 / 备份；返回结构方便日志或 API 复用。
    """
    classes = config_classes or _config_classes
    primary_path = get_primary_config_path()

    try:
        config = toml.load(primary_path)
    except FileNotFoundError:
        config = {}
    except toml.TomlDecodeError as err:
        logger.error(f"配置文件格式错误，跳过整理 ({primary_path}): {err}")
        return {
            "migrated": [],
            "removed": {},
            "filled": {},
            "backup_path": None,
            "error": str(err),
        }

    migrate_changed, migrate_notes = _apply_legacy_migrations(config)
    prune_changed, removed = _prune_stale_keys(config)

    # 补缺失：与 fill_missing_config_items 逻辑等价，但走同一份 config dict
    missing_by_section: dict[str, list[str]] = {}
    for conf_cls in classes:
        section = getattr(conf_cls, "_section", None)
        if not section:
            continue
        block = config.get(section)
        if not isinstance(block, dict):
            block = {}
            config[section] = block
        defaults = conf_cls._get_default_values()
        added: list[str] = []
        for key, default_value in defaults.items():
            if key in block:
                continue
            if isinstance(default_value, Path):
                default_value = conf_cls._serialize_config_value(default_value)
            block[key] = default_value
            added.append(key)
        if added:
            missing_by_section[section] = sorted(added)

    fill_changed = bool(missing_by_section)
    if not (migrate_changed or prune_changed or fill_changed):
        # 没有变更也吐一条日志，便于排查"为什么没补全/没清理"
        logger.info("配置整理: %s 无需变更（所有 section / 字段均与代码声明一致）", primary_path)
        return {
            "migrated": [],
            "removed": {},
            "filled": {},
            "backup_path": None,
        }

    backup_path: Optional[Path] = None
    if auto_backup and primary_path.exists():
        backup_path = backup_config_file(primary_path, reason="sweep")

    try:
        with open(primary_path, "w", encoding="utf-8") as f:
            toml.dump(config, f)
    except Exception as err:
        logger.error(f"写回整理后的配置失败: {err}")
        return {
            "migrated": migrate_notes,
            "removed": removed,
            "filled": missing_by_section,
            "backup_path": str(backup_path) if backup_path else None,
            "error": str(err),
        }

    for note in migrate_notes:
        logger.info("配置迁移: %s", note)
    for section, keys in removed.items():
        if section == "__section_removed__":
            for sec in keys:
                logger.info("配置清理: 整段移除 [%s]", sec)
        else:
            logger.info("配置清理: [%s] 移除 %d 个无效字段: %s", section, len(keys), ", ".join(keys))
    for section, keys in missing_by_section.items():
        logger.info("配置补齐: [%s] 新增 %d 项: %s", section, len(keys), ", ".join(keys))

    return {
        "migrated": migrate_notes,
        "removed": removed,
        "filled": missing_by_section,
        "backup_path": str(backup_path) if backup_path else None,
    }


# 模块加载时序：先把 toml 整理干净（迁移 + 清理 + 补齐），再让各 Config 类读它
_sweep_result = sweep_config_toml(auto_backup=True)
if _sweep_result.get("error"):
    logger.error("启动期 config.toml 整理失败: %s", _sweep_result["error"])
else:
    _summary_parts: list[str] = []
    _filled = _sweep_result.get("filled") or {}
    if _filled:
        _summary_parts.append("补齐 " + ", ".join(f"[{s}] +{len(v)}" for s, v in _filled.items()))
    _removed = _sweep_result.get("removed") or {}
    _removed_keys = {k: v for k, v in _removed.items() if k != "__section_removed__"}
    if _removed_keys:
        _summary_parts.append("清理 " + ", ".join(f"[{s}] -{len(v)}" for s, v in _removed_keys.items()))
    if _removed.get("__section_removed__"):
        _summary_parts.append("移除整段: " + ", ".join(_removed["__section_removed__"]))
    _migrated = _sweep_result.get("migrated") or []
    if _migrated:
        _summary_parts.append(f"迁移 {len(_migrated)} 条历史字段")
    if _summary_parts:
        logger.info("启动期 config.toml 整理完成: %s", "；".join(_summary_parts))
_load_all_configs()
