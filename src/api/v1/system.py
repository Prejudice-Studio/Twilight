"""
系统信息 API

提供系统配置、状态等信息
"""

from typing import Optional, Any
from pathlib import Path
import shutil
import os
import time

from flask import Blueprint, request, g, send_file
from sqlalchemy import text

from src.api.v1.auth import require_auth, require_admin, api_response
from src.core.utils import parse_bool, rate_limit_check
from src.core.request_utils import get_real_client_ip
from src.config import (
    Config,
    EmbyConfig,
    RegisterConfig,
    DeviceLimitConfig,
    APIConfig,
    SecurityConfig,
    SchedulerConfig,
    SystemUpdateConfig,
    NotificationConfig,
    TelegramConfig,
    BangumiSyncConfig,
    ROOT_PATH,
    backup_config_file,
    fill_missing_config_items,
    sweep_config_toml,
    get_primary_config_path,
    normalize_storage_settings,
    reload_runtime_config as _config_reload_runtime_config,
)
from src import __version__
from src.db.user import UsersSessionFactory
from src.services.system_update_service import apply_git_update

import asyncio
import logging
import threading
import mimetypes

_reload_logger = logging.getLogger(__name__)

_CONFIG_CLASSES = [
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

_REGCODE_RANDOM_ALGORITHMS = {
    "base32-20",
    "base32-24",
    "hex32",
    "hex20",
    "base32-16",
    "alnum-24",
    "alnum-16",
    "urlsafe-24",
    "digits-16",
    "digits-12",
    "uuid",
    "legacy-sha1",
}

_CONFIG_SELECT_ALLOWED_VALUES = {
    ("Global", "log_level"): {10, 20, 30, 40},
    ("API", "session_cookie_samesite"): {"Strict", "Lax", "None"},
    ("SAR", "regcode_random_algorithm"): _REGCODE_RANDOM_ALGORITHMS,
    ("SAR", "regcode_decoy_action"): {"none", "disable_user", "disable_user_and_deactivate_code"},
    ("SystemUpdate", "auto_update_trigger_type"): {"interval", "cron_daily"},
}


def _schedule_process_restart(delay: float = 1.5) -> None:
    """安排整个进程在短暂延迟后退出，由进程管理器/启动脚本负责拉起。

    用主动退出替代旧的"热重载"逻辑，避免跨事件循环、单实例锁残留等问题。
    要求部署方使用 systemd / docker / 守护脚本等具备自动重启能力的方式拉起进程。
    """
    import os
    import signal
    import time as _time_mod

    def _do_exit():
        try:
            _time_mod.sleep(max(0.1, float(delay)))
        except Exception:
            _time_mod.sleep(1.5)

        _reload_logger.warning("🔄 配置已更新，进程即将退出以完成重启 (PID=%s)", os.getpid())

        # 先尝试 SIGTERM 进程组，确保 API + Bot 等同组进程一同退出
        try:
            if hasattr(os, "killpg") and hasattr(os, "getpgrp"):
                os.killpg(os.getpgrp(), signal.SIGTERM)
                # SIGTERM 后给一点时间让信号传达，再强制退出
                _time_mod.sleep(2.0)
        except Exception as exc:
            _reload_logger.warning("发送进程组 SIGTERM 失败: %s", exc)

        # 兜底：直接退出当前进程
        try:
            os._exit(0)
        except SystemExit:
            raise
        except Exception:
            os._exit(1)

    threading.Thread(target=_do_exit, daemon=True, name="twilight-restart").start()
    _reload_logger.info("🛑 已请求重启整个程序，将在 %.1fs 后退出，请确保由进程管理器拉起", delay)


def _reload_runtime_config() -> None:
    """重新加载运行时配置类。"""
    _config_reload_runtime_config()


async def _apply_runtime_hot_reload() -> dict:
    """刷新当前进程配置，并在本进程调度器存在时重装任务。"""
    _reload_runtime_config()
    payload = {"config": True, "scheduler": None, "bot": None}
    try:
        from src.services.scheduler_service import SchedulerService

        ok, message = await SchedulerService.reload_from_config(reload_config=False)
        payload["scheduler"] = {"success": ok, "message": message}
    except Exception as exc:  # pragma: no cover
        _reload_logger.warning("调度器热重载失败: %s", exc, exc_info=True)
        payload["scheduler"] = {"success": False, "message": str(exc)}
    try:
        from src.bot.bot import reload_bot_from_config

        ok, message = await reload_bot_from_config()
        payload["bot"] = {"success": ok, "message": message}
    except Exception as exc:  # pragma: no cover
        _reload_logger.warning("Bot 热重载失败: %s", exc, exc_info=True)
        payload["bot"] = {"success": False, "message": str(exc)}
    return payload


def _infer_schema_field_type(key: str, value: Any) -> str:
    """根据键名和值推断可视化配置字段类型。"""
    key_lower = (key or "").lower()
    if any(token in key_lower for token in ("token", "password", "secret", "api_key", "apikey")):
        return "secret"
    if isinstance(value, bool):
        return "bool"
    if isinstance(value, int):
        return "int"
    if isinstance(value, float):
        return "float"
    if isinstance(value, list):
        return "list"
    return "string"


_SCHEMA_HIDDEN_FIELDS: dict[str, set[str]] = {
    # 这些字段在配置类里仍是合法默认值，但「配置管理」UI 不再暴露；
    # Scheduler 的逐 job 触发器在「定时任务」页编辑，避免双入口冲突。
    "Scheduler": {
        "expired_check_time",
        "expiring_check_time",
        "daily_stats_time",
        "session_cleanup_interval",
        "emby_sync_interval",
    },
}


def _augment_schema_with_missing_fields(schema: dict) -> None:
    """将配置类中存在但 schema 未声明的字段自动补进可视化配置。"""
    section_class_map = {
        "Global": Config,
        "Emby": EmbyConfig,
        "Telegram": TelegramConfig,
        "SAR": RegisterConfig,
        "DeviceLimit": DeviceLimitConfig,
        "API": APIConfig,
        "Security": SecurityConfig,
        "Scheduler": SchedulerConfig,
        "SystemUpdate": SystemUpdateConfig,
        "Notification": NotificationConfig,
        "BangumiSync": BangumiSyncConfig,
    }
    sections = schema.get("sections", [])
    section_map = {section.get("key"): section for section in sections}

    for section_key, conf_cls in section_class_map.items():
        section = section_map.get(section_key)
        if not section:
            continue

        hidden = _SCHEMA_HIDDEN_FIELDS.get(section_key, set())
        fields = section.get("fields", [])
        existing = {field.get("key") for field in fields}
        defaults = conf_cls._get_default_values()

        for field_key, default_value in defaults.items():
            if field_key in existing or field_key in hidden:
                continue

            value = getattr(conf_cls, field_key.upper(), default_value)
            fields.append(
                {
                    "key": field_key,
                    "label": field_key,
                    "type": _infer_schema_field_type(field_key, value),
                    "description": "自动识别的配置项（尚未补充专用说明）",
                    "value": value,
                }
            )


def _backup_config_before_update(reason: str) -> Optional[Path]:
    """更新前备份当前配置文件。"""
    config_file = get_primary_config_path()
    return backup_config_file(config_file, reason=reason)


def _parse_csv_ids(value: str) -> set:
    if not value:
        return set()
    out: set = set()
    for token in str(value).split(","):
        token = token.strip()
        if not token:
            continue
        try:
            out.add(int(token))
        except ValueError:
            continue
    return out


def _parse_csv_names(value: str) -> set:
    if not value:
        return set()
    return {n.strip().lower() for n in str(value).split(",") if n.strip()}


async def _sync_admin_role_from_config():
    """根据 RegisterConfig 中的 admin_uids/admin_usernames/white_list_* 完全同步数据库 ROLE。

    策略：
    - 加入 admin_uids/admin_usernames 的用户 ROLE → ADMIN
    - 加入 white_list_uids/white_list_usernames 的用户 ROLE → WHITE_LIST
    - 当前 ROLE 是 ADMIN/WHITE_LIST 但既不在 admin 也不在 white_list 配置中 → NORMAL
    - 其它角色（NORMAL / UNRECOGNIZED）不变
    """
    from sqlalchemy import select, update
    from src.db.user import Role, UserModel, UsersSessionFactory

    admin_uids = _parse_csv_ids(RegisterConfig.ADMIN_UIDS)
    admin_names = _parse_csv_names(RegisterConfig.ADMIN_USERNAMES)
    white_uids = _parse_csv_ids(RegisterConfig.WHITE_LIST_UIDS)
    white_names = _parse_csv_names(RegisterConfig.WHITE_LIST_USERNAMES)

    promote_admin: list = []
    promote_white: list = []
    demote: list = []

    async with UsersSessionFactory() as session:
        async with session.begin():
            result = await session.execute(select(UserModel.UID, UserModel.USERNAME, UserModel.ROLE))
            for uid, username, role in result.all():
                uname = (username or "").lower()
                is_cfg_admin = uid in admin_uids or (uname and uname in admin_names)
                is_cfg_white = not is_cfg_admin and (uid in white_uids or (uname and uname in white_names))
                if is_cfg_admin:
                    if role != Role.ADMIN.value:
                        promote_admin.append(uid)
                elif is_cfg_white:
                    if role != Role.WHITE_LIST.value:
                        promote_white.append(uid)
                elif role in (Role.ADMIN.value, Role.WHITE_LIST.value):
                    demote.append(uid)

            if promote_admin:
                await session.execute(
                    update(UserModel).where(UserModel.UID.in_(promote_admin)).values(ROLE=Role.ADMIN.value)
                )
            if promote_white:
                await session.execute(
                    update(UserModel).where(UserModel.UID.in_(promote_white)).values(ROLE=Role.WHITE_LIST.value)
                )
            if demote:
                await session.execute(update(UserModel).where(UserModel.UID.in_(demote)).values(ROLE=Role.NORMAL.value))

    _reload_logger.info(
        f"管理员配置同步完成: 升级 admin={len(promote_admin)}, 升级 white={len(promote_white)}, 降级 normal={len(demote)}"
    )


system_bp = Blueprint("system", __name__, url_prefix="/system")


_IMAGE_MIME_PREFIXES = ("image/",)


def _check_public_system_rate_limit(scope: str, *, max_requests: int = 60, window_seconds: int = 60):
    client_ip = get_real_client_ip()
    allowed, retry_after = rate_limit_check(
        f"system_public:{scope}",
        client_ip,
        max_requests=max_requests,
        window_seconds=window_seconds,
    )
    if not allowed:
        return api_response(False, f"请求过于频繁，请在 {retry_after} 秒后重试", code=429)
    return None


def _resolve_local_server_icon_path() -> Optional[Path]:
    """Resolve Config.SERVER_ICON when it points at a local image file."""
    raw = (Config.SERVER_ICON or "").strip()
    if not raw:
        return None
    lowered = raw.lower()
    if lowered.startswith(("http://", "https://", "//", "data:", "blob:")):
        return None
    path = Path(raw).expanduser()
    if not path.is_absolute():
        path = (Path.cwd() / path).resolve()
    else:
        path = path.resolve()
    try:
        path.relative_to(ROOT_PATH)
    except ValueError:
        return None
    if not path.is_file():
        return None
    guessed, _ = mimetypes.guess_type(str(path))
    if not guessed or not guessed.startswith(_IMAGE_MIME_PREFIXES):
        return None
    return path


def _public_server_icon_value() -> str:
    raw = (Config.SERVER_ICON or "").strip()
    if not raw:
        return ""
    if _resolve_local_server_icon_path():
        return "/api/v1/system/server-icon"
    # Do not leak explicit local filesystem paths when the configured file is missing/invalid.
    if raw.startswith("~") or "\\" in raw or (len(raw) >= 3 and raw[1:3] in (":\\", ":/")):
        return ""
    return raw


# ==================== 公开信息 ====================


@system_bp.route("/info", methods=["GET"])
async def get_system_info():
    """
    获取系统公开信息

    不需要登录即可访问
    """
    limited = _check_public_system_rate_limit("info", max_requests=60, window_seconds=60)
    if limited:
        return limited

    # 暴露 Bot 用户名供前端展示/跳转
    telegram_bot_username: Optional[str] = None
    if Config.TELEGRAM_MODE:
        try:
            from src.services.telegram_runtime import run_bot_operation

            async def _read_bot_username(bot):
                if getattr(bot, "username", None):
                    return bot.username
                me = await bot.get_me()
                return me.username or None

            telegram_bot_username = await run_bot_operation(_read_bot_username, timeout=8)
        except Exception:
            telegram_bot_username = None

    telegram_bot_url = f"https://t.me/{telegram_bot_username}" if telegram_bot_username else None

    return api_response(
        True,
        "获取成功",
        {
            "name": Config.SERVER_NAME or "Twilight",
            "icon": _public_server_icon_value(),
            "version": __version__,
            "features": {
                "register": RegisterConfig.REGISTER_MODE,
                "emby_direct_register": RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED,
                "telegram": Config.TELEGRAM_MODE,
                "force_bind_telegram": Config.FORCE_BIND_TELEGRAM,
                "bangumi_sync": BangumiSyncConfig.ENABLED,
            },
            "limits": {
                "user_limit": RegisterConfig.USER_LIMIT,
                "stream_limit": DeviceLimitConfig.MAX_STREAMS if DeviceLimitConfig.DEVICE_LIMIT_ENABLED else None,
            },
            "telegram_bot": {
                "username": telegram_bot_username,
                "url": telegram_bot_url,
            },
        },
    )


@system_bp.route("/server-icon", methods=["GET"])
async def get_server_icon():
    """Serve the configured local server icon, if SERVER_ICON is a local image path."""
    limited = _check_public_system_rate_limit("server_icon", max_requests=120, window_seconds=60)
    if limited:
        return limited
    path = _resolve_local_server_icon_path()
    if not path:
        return api_response(False, "未配置本地图标或文件不存在", code=404)
    guessed, _ = mimetypes.guess_type(str(path))
    return send_file(path, mimetype=guessed or "application/octet-stream", conditional=True, max_age=3600)


@system_bp.route("/health", methods=["GET"])
async def health_check():
    """健康检查"""
    limited = _check_public_system_rate_limit("health", max_requests=30, window_seconds=60)
    if limited:
        return limited

    from src.services import get_emby_client

    status = {
        "api": True,
        "database": False,
        "emby": False,
    }

    # 检查数据库连接
    try:
        async with UsersSessionFactory() as session:
            await session.execute(text("SELECT 1"))
        status["database"] = True
    except Exception:
        status["database"] = False

    # 检查 Emby 连接
    try:
        emby = get_emby_client()
        info = await emby.get_public_info()
        status["emby"] = bool(info)
    except Exception:
        pass

    all_healthy = all(status.values())

    return api_response(all_healthy, "OK" if all_healthy else "部分服务异常", status)


@system_bp.route("/stats", methods=["GET"])
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
        "timestamp": int(time.time()),
        "cpu_count": os.cpu_count(),
        "cpu_percent": None,
        "memory": None,
        "disk": None,
    }

    if psutil:
        stats["cpu_percent"] = psutil.cpu_percent(interval=None)

        mem = psutil.virtual_memory()
        stats["memory"] = {"total": mem.total, "available": mem.available, "percent": mem.percent, "used": mem.used}

        disk = psutil.disk_usage("/")
        stats["disk"] = {"total": disk.total, "free": disk.free, "percent": disk.percent}

    # 获取应用级统计 (如总用户数，今日活跃等，这里由于性能原因可以简化或异步获取，暂时只返回系统级)
    # 若需业务统计，可复用 src.api.v1.stats

    return api_response(True, "获取成功", stats)


@system_bp.route("/emby-urls", methods=["GET"])
@require_auth
async def get_emby_urls():
    """
    获取 Emby 服务器线路列表

    根据用户角色返回不同线路：
    - 普通用户：返回普通线路列表
    - 白名单/管理员：额外返回白名单专属线路

    未绑定 Emby 账号的用户会拿到空 `lines`，避免在用户尚不持有 Emby 账号时就
    把服务器地址泄露给浏览器。

    返回结构化数据: { lines: [{name, url, tag?}], whitelist_lines?: [{name, url, tag?}] }
    """
    from src.db.user import Role
    from src.services.user_service import UserService

    user = getattr(g, "current_user", None)
    is_admin = bool(user and user.ROLE == Role.ADMIN.value)
    # 与 user_service.get_user_info / admin 列表用同一套口径：
    # EMBYID 非空即视为已绑定。历史数据可能残留 PENDING_EMBY=True，不能因此拒绝下发线路。
    emby_bound = bool(user and user.EMBYID)

    # 注册码授予的待补建用户已具备开通资格，允许查看线路以完成后续配置；
    # 普通无码待激活账号仍不下发线路。
    entitled_pending = bool(user and getattr(user, "PENDING_EMBY_DAYS", None) is not None)
    if user and not emby_bound and not is_admin and not entitled_pending:
        return api_response(
            True,
            "未绑定 Emby 账号，未下发线路",
            {
                "lines": [],
                "requires_emby_account": True,
            },
        )

    if user and not is_admin and UserService.is_emby_access_expired(user):
        return api_response(
            True,
            "Emby 账号已到期并禁用，续期后恢复线路访问",
            {
                "lines": [],
                "requires_renewal": True,
                "emby_disabled_by_expiry": True,
            },
        )

    def parse_url_entry(entry: str) -> dict:
        """解析 'Label : http://...' 格式"""
        if " : " in entry:
            parts = entry.split(" : ", 1)
            return {"name": parts[0].strip(), "url": parts[1].strip()}
        return {"name": "默认线路", "url": entry.strip()}

    lines = [parse_url_entry(u) for u in EmbyConfig.EMBY_URL_LIST]

    result = {"lines": lines}

    # 白名单/管理员用户额外返回专属线路
    if user and user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
        whitelist_lines = [parse_url_entry(u) for u in EmbyConfig.EMBY_URL_LIST_FOR_WHITELIST]
        if whitelist_lines:
            result["whitelist_lines"] = whitelist_lines

    return api_response(True, "获取成功", result)


# ==================== 需要登录 ====================


@system_bp.route("/config", methods=["GET"])
@require_auth
async def get_user_config():
    """获取用户可见的配置"""
    return api_response(
        True,
        "获取成功",
        {
            "device_limit": {
                "enabled": DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
                "max_devices": DeviceLimitConfig.MAX_DEVICES,
                "max_streams": DeviceLimitConfig.MAX_STREAMS,
            },
            "bangumi_sync": {
                "enabled": BangumiSyncConfig.ENABLED,
            },
        },
    )


# ==================== 管理员专用 ====================


@system_bp.route("/admin/config", methods=["GET"])
@require_auth
@require_admin
async def get_admin_config():
    """获取完整的系统配置（管理员）"""
    return api_response(
        True,
        "获取成功",
        {
            "global": {
                "logging": Config.LOGGING,
                "log_level": Config.LOG_LEVEL,
                "telegram_mode": Config.TELEGRAM_MODE,
                "force_bind_telegram": Config.FORCE_BIND_TELEGRAM,
            },
            "emby": {
                "url": EmbyConfig.EMBY_URL,
                "url_list": EmbyConfig.EMBY_URL_LIST,
            },
            "telegram": {
                "enabled": Config.TELEGRAM_MODE,
                "admin_ids": TelegramConfig.ADMIN_ID,
                "group_ids": TelegramConfig.GROUP_ID,
                "channel_ids": TelegramConfig.CHANNEL_ID,
                "force_subscribe": TelegramConfig.FORCE_SUBSCRIBE,
            },
            "sar": {
                "register_mode": RegisterConfig.REGISTER_MODE,
                "register_code_limit": RegisterConfig.REGISTER_CODE_LIMIT,
                "emby_direct_register_enabled": RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED,
                "emby_direct_register_days": RegisterConfig.EMBY_DIRECT_REGISTER_DAYS,
                "emby_user_limit": RegisterConfig.EMBY_USER_LIMIT,
                "emby_direct_register_workers": RegisterConfig.EMBY_DIRECT_REGISTER_WORKERS,
                "emby_direct_register_max_queue": RegisterConfig.EMBY_DIRECT_REGISTER_MAX_QUEUE,
                "emby_direct_register_status_ttl": RegisterConfig.EMBY_DIRECT_REGISTER_STATUS_TTL,
                "user_limit": RegisterConfig.USER_LIMIT,
            },
            "device_limit": {
                "enabled": DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
                "max_devices": DeviceLimitConfig.MAX_DEVICES,
                "max_streams": DeviceLimitConfig.MAX_STREAMS,
                "kick_oldest": DeviceLimitConfig.KICK_OLDEST_SESSION,
            },
            "security": {
                "login_fail_threshold": SecurityConfig.LOGIN_FAIL_THRESHOLD,
                "lockout_minutes": SecurityConfig.LOCKOUT_MINUTES,
            },
            "api": {
                "host": APIConfig.HOST,
                "port": APIConfig.PORT,
                "debug": APIConfig.DEBUG,
                "token_expire": APIConfig.TOKEN_EXPIRE,
                "cors_enabled": APIConfig.CORS_ENABLED,
            },
            "scheduler": {
                "enabled": SchedulerConfig.ENABLED,
                "timezone": SchedulerConfig.TIMEZONE,
            },
            "notification": {
                "enabled": NotificationConfig.ENABLED,
                "expiry_remind_days": NotificationConfig.EXPIRY_REMIND_DAYS,
            },
            "bangumi_sync": {
                "enabled": BangumiSyncConfig.ENABLED,
                "webhook_secret_set": bool(BangumiSyncConfig.WEBHOOK_SECRET),
                "min_progress_percent": BangumiSyncConfig.MIN_PROGRESS_PERCENT,
            },
        },
    )


@system_bp.route("/admin/stats", methods=["GET"])
@require_auth
@require_admin
async def get_system_stats():
    """获取系统统计信息（管理员）"""
    from src.db.user import UserOperate
    from src.db.regcode import RegCodeOperate
    from src.services import EmbyService, UserService

    # 用户统计
    total_users = await UserOperate.get_registered_users_count()
    active_users = await UserOperate.get_active_users_count()
    emby_bound_users = await UserOperate.get_emby_bound_users_count()

    # 注册码统计（使用数据库层面计数，避免全量加载到内存）
    regcode_stats = await RegCodeOperate.get_regcode_stats()

    # Emby 状态
    try:
        emby_status = await EmbyService.get_server_status()
    except Exception:
        emby_status = {"online": False}

    # 把自由注册/卡码队列里 in-flight 的待创建请求也算进 Emby 占用统计，便于运维一眼看到真实余量
    emby_pending = UserService.get_emby_capacity_queue_pending_count()
    emby_projected = emby_bound_users + emby_pending

    return api_response(
        True,
        "获取成功",
        {
            "users": {
                "total": total_users,
                "active": active_users,
                "limit": RegisterConfig.USER_LIMIT,
                "usage_percent": (
                    round(total_users / RegisterConfig.USER_LIMIT * 100, 1) if RegisterConfig.USER_LIMIT > 0 else 0
                ),
                "emby_bound": emby_bound_users,
                "emby_pending": emby_pending,
                "emby_projected": emby_projected,
                "emby_limit": RegisterConfig.EMBY_USER_LIMIT,
                "emby_usage_percent": (
                    round(emby_projected / RegisterConfig.EMBY_USER_LIMIT * 100, 1)
                    if RegisterConfig.EMBY_USER_LIMIT > 0
                    else 0
                ),
            },
            "regcodes": regcode_stats,
            "emby": emby_status,
        },
    )


@system_bp.route("/admin/update", methods=["POST"])
@require_auth
@require_admin
async def admin_update_from_git():
    """从管理员指定的 Git 仓库拉取指定分支并重启 systemd 服务。"""
    data = request.get_json(silent=True) or {}
    repo_url = (data.get("repo_url") or "").strip()
    branch = (data.get("branch") or "main").strip()
    restart_services = parse_bool(data.get("restart_services"), default=True)

    result = apply_git_update(repo_url, branch, restart_services=restart_services)
    if not result.get("success"):
        _reload_logger.error("自动更新失败: %s", result)
        return api_response(False, result["message"], result, code=int(result.get("code") or 500))

    _reload_logger.warning(
        "管理员 %s 触发自动更新: repo=%s branch=%s restart=%s",
        getattr(g.current_user, "USERNAME", None),
        repo_url,
        branch,
        result.get("restart_scheduled"),
    )
    return api_response(
        True,
        result["message"],
        result,
    )


@system_bp.route("/admin/config/toml", methods=["GET"])
@require_auth
@require_admin
async def get_config_toml():
    """获取 config.toml 文件内容（管理员）"""
    config_file = get_primary_config_path()

    if not config_file.exists():
        return api_response(False, "配置文件不存在", code=404)

    try:
        with open(config_file, "r", encoding="utf-8") as f:
            content = f.read()
        return api_response(
            True,
            "获取成功",
            {
                "content": content,
                "path": str(config_file),
            },
        )
    except Exception as e:
        import logging

        logger = logging.getLogger(__name__)
        logger.error(f"读取配置文件失败: {e}", exc_info=True)
        return api_response(False, f"读取配置文件失败: {e}", code=500)


@system_bp.route("/admin/config/toml", methods=["PUT"])
@require_auth
@require_admin
async def update_config_toml():
    """更新 config.toml 文件内容（管理员）"""
    import toml

    data = request.get_json() or {}
    content = data.get("content")

    if content is None:
        return api_response(False, "缺少 content 参数", code=400)

    config_file = get_primary_config_path()

    # 验证 TOML 格式
    try:
        toml.loads(content)
    except Exception as e:
        return api_response(False, f"TOML 格式错误: {e}", code=400)

    backup_file: Optional[Path] = None
    try:
        backup_file = _backup_config_before_update("admin-toml")
    except Exception as e:
        _reload_logger.warning(f"备份配置文件失败: {e}")

    # 写入新内容
    try:
        with open(config_file, "w", encoding="utf-8") as f:
            f.write(content)

        # sweep 会顺手补缺失项 + 迁移历史字段 + 清理无效条目；admin 手编时同样适用
        sweep_result = sweep_config_toml(config_classes=_CONFIG_CLASSES, auto_backup=False)
        reload_result = await _apply_runtime_hot_reload()

        return api_response(
            True,
            "配置已保存并热重载",
            {
                "path": str(config_file),
                "backup_path": str(backup_file) if backup_file else None,
                "filled": sweep_result.get("filled") or {},
                "removed": sweep_result.get("removed") or {},
                "migrated": sweep_result.get("migrated") or [],
                "restart": False,
                "reload": reload_result,
            },
        )
    except Exception as e:
        import logging

        logger = logging.getLogger(__name__)
        logger.error(f"更新配置文件失败: {e}", exc_info=True)

        # 尝试恢复备份
        if backup_file and backup_file.exists():
            try:
                shutil.copy2(backup_file, config_file)
            except Exception:
                pass

        return api_response(False, f"更新配置文件失败: {e}", code=500)


@system_bp.route("/admin/config/schema", methods=["GET"])
@require_auth
@require_admin
async def get_config_schema():
    """获取配置项的结构化描述信息（管理员）。

    每个 section 带 ``category``，便于前端把同类配置归组展示，避免一字长龙。
    """
    schema = {
        # 前端按下列顺序渲染分组；section 的 category 必须与其中之一匹配
        "categories": [
            {"key": "base", "title": "基础设置"},
            {"key": "media", "title": "媒体服务"},
            {"key": "integration", "title": "第三方接入"},
            {"key": "user", "title": "用户与注册"},
            {"key": "api", "title": "API 与安全"},
            {"key": "automation", "title": "自动化与通知"},
        ],
        "sections": [
            {
                "key": "Global",
                "category": "base",
                "title": "全局配置",
                "description": "站点名称、日志、数据库等基础设置",
                "fields": [
                    {
                        "key": "server_name",
                        "label": "服务器名称",
                        "type": "string",
                        "description": "服务器名称，用于前端和通知中显示",
                        "value": Config.SERVER_NAME,
                    },
                    {
                        "key": "server_icon",
                        "label": "服务器图标",
                        "type": "string",
                        "description": "服务器图标 URL，留空使用默认",
                        "value": Config.SERVER_ICON,
                    },
                    {
                        "key": "logging",
                        "label": "日志开关",
                        "type": "bool",
                        "description": "是否启用日志记录",
                        "value": Config.LOGGING,
                    },
                    {
                        "key": "log_level",
                        "label": "日志等级",
                        "type": "select",
                        "description": "日志等级。生产建议 INFO 或 WARNING；DEBUG 会输出更多诊断信息，仅建议临时排障使用。",
                        "value": Config.LOG_LEVEL,
                        "options": [
                            {"label": "DEBUG 调试", "value": 10},
                            {"label": "INFO 常规", "value": 20},
                            {"label": "WARNING 警告", "value": 30},
                            {"label": "ERROR 错误", "value": 40},
                        ],
                    },
                    {
                        "key": "sqlalchemy_log",
                        "label": "SQLAlchemy 日志",
                        "type": "bool",
                        "description": "是否输出 SQLAlchemy ORM 日志（调试用）",
                        "value": Config.SQLALCHEMY_LOG,
                    },
                    {
                        "key": "max_retry",
                        "label": "最大重试次数",
                        "type": "int",
                        "description": "HTTP 请求失败时的最大重试次数",
                        "value": Config.MAX_RETRY,
                    },
                    {
                        "key": "databases_dir",
                        "label": "数据库目录",
                        "type": "string",
                        "description": "SQLite 数据库文件存储目录",
                        "value": str(Config.DATABASES_DIR),
                    },
                    {
                        "key": "redis_url",
                        "label": "Redis 连接",
                        "type": "string",
                        "description": "Redis 连接串，如 redis://localhost:6379/0；留空则不使用 Redis，加锁/会话回退数据库",
                        "value": Config.REDIS_URL,
                    },
                    {
                        "key": "bangumi_token",
                        "label": "Bangumi Token",
                        "type": "secret",
                        "description": "Bangumi API 访问令牌",
                        "value": Config.BANGUMI_TOKEN,
                    },
                    {
                        "key": "telegram_mode",
                        "label": "Telegram 模式",
                        "type": "bool",
                        "description": "是否启用 Telegram Bot 功能",
                        "value": Config.TELEGRAM_MODE,
                    },
                    {
                        "key": "force_bind_telegram",
                        "label": "强制绑定 Telegram",
                        "type": "bool",
                        "description": "是否强制用户绑定 Telegram",
                        "value": Config.FORCE_BIND_TELEGRAM,
                    },
                    {
                        "key": "tmdb_api_key",
                        "label": "TMDB API Key",
                        "type": "secret",
                        "description": "TMDB API Key (v3)，用于获取影视元数据",
                        "value": Config.TMDB_API_KEY,
                    },
                    {
                        "key": "tmdb_api_url",
                        "label": "TMDB API 地址",
                        "type": "string",
                        "description": "TMDB API 服务器地址",
                        "value": Config.TMDB_API_URL,
                    },
                    {
                        "key": "tmdb_image_url",
                        "label": "TMDB 图片地址",
                        "type": "string",
                        "description": "TMDB 图片 CDN 地址",
                        "value": Config.TMDB_IMAGE_URL,
                    },
                    {
                        "key": "bangumi_api_url",
                        "label": "Bangumi API 地址",
                        "type": "string",
                        "description": "Bangumi API 服务器地址",
                        "value": Config.BANGUMI_API_URL,
                    },
                    {
                        "key": "bangumi_app_id",
                        "label": "Bangumi App ID",
                        "type": "string",
                        "description": "Bangumi OAuth App ID（可选）",
                        "value": Config.BANGUMI_APP_ID,
                    },
                ],
                # 已删除字段（数据上仍可能存在历史值，但不再向 UI 暴露）：
                # global_bgm_mode / email_bind / force_bind_email
            },
            {
                "key": "Emby",
                "category": "media",
                "title": "Emby 配置",
                "description": "Emby/Jellyfin 媒体服务器连接配置",
                "fields": [
                    {
                        "key": "emby_url",
                        "label": "Emby 地址",
                        "type": "string",
                        "description": "Emby 服务器地址，如 http://127.0.0.1:8096/",
                        "value": EmbyConfig.EMBY_URL,
                    },
                    {
                        "key": "emby_token",
                        "label": "API Key",
                        "type": "secret",
                        "description": "Emby 管理后台生成的 API Key（主要认证方式）",
                        "value": EmbyConfig.EMBY_TOKEN,
                    },
                    {
                        "key": "emby_username",
                        "label": "管理员用户名",
                        "type": "string",
                        "description": "Emby 管理员用户名（API Key 无效时的备用认证）",
                        "value": EmbyConfig.EMBY_USERNAME,
                    },
                    {
                        "key": "emby_password",
                        "label": "管理员密码",
                        "type": "secret",
                        "description": "Emby 管理员密码（API Key 无效时的备用认证）",
                        "value": EmbyConfig.EMBY_PASSWORD,
                    },
                    {
                        "key": "emby_url_list",
                        "label": "线路列表",
                        "type": "list",
                        "description": '提供给用户的 Emby 服务器线路列表，格式: "线路名 : URL"',
                        "value": EmbyConfig.EMBY_URL_LIST,
                    },
                    {
                        "key": "emby_url_list_for_whitelist",
                        "label": "白名单线路列表",
                        "type": "list",
                        "description": "白名单用户专用的 Emby 服务器线路列表",
                        "value": EmbyConfig.EMBY_URL_LIST_FOR_WHITELIST,
                    },
                    {
                        "key": "emby_default_hidden_libraries",
                        "label": "默认隐藏媒体库",
                        "type": "list",
                        "description": "普通用户新建/补建 Emby 账号后自动隐藏的媒体库名称；按 Emby 媒体库名称填写，留空不处理。",
                        "value": EmbyConfig.EMBY_DEFAULT_HIDDEN_LIBRARIES,
                    },
                    {
                        "key": "emby_self_service_libraries",
                        "label": "用户自助显隐媒体库",
                        "type": "list",
                        "description": "管理员开放给已授予自助显隐权限用户自行显示/隐藏的媒体库名称；按 Emby 媒体库名称填写，留空则无法自助操作。",
                        "value": EmbyConfig.EMBY_SELF_SERVICE_LIBRARIES,
                    },
                ],
            },
            {
                "key": "Telegram",
                "category": "integration",
                "title": "Telegram 配置",
                "description": "Telegram Bot 相关设置",
                "fields": [
                    {
                        "key": "telegram_api_url",
                        "label": "API 地址",
                        "type": "string",
                        "description": "Telegram Bot API 地址，可用于自建 API 代理",
                        "value": TelegramConfig.TELEGRAM_API_URL,
                    },
                    {
                        "key": "bot_token",
                        "label": "Bot Token",
                        "type": "secret",
                        "description": "从 @BotFather 获取的 Bot Token",
                        "value": TelegramConfig.BOT_TOKEN,
                    },
                    {
                        "key": "admin_id",
                        "label": "管理员 ID",
                        "type": "list",
                        "description": "Telegram 管理员用户 ID 列表",
                        "value": TelegramConfig.ADMIN_ID,
                    },
                    {
                        "key": "group_id",
                        "label": "群组 ID",
                        "type": "list",
                        "description": "Telegram 群组 ID 列表，支持数字ID（如 -1001234567890）或 @用户名（如 @mygroup）",
                        "value": TelegramConfig.GROUP_ID,
                    },
                    {
                        "key": "channel_id",
                        "label": "频道 ID",
                        "type": "list",
                        "description": "Telegram 频道 ID 列表，支持数字ID（如 -1001234567890）或 @用户名（如 @mychannel）",
                        "value": TelegramConfig.CHANNEL_ID,
                    },
                    {
                        "key": "force_subscribe",
                        "label": "强制订阅",
                        "type": "bool",
                        "description": "是否要求用户订阅频道后才能使用",
                        "value": TelegramConfig.FORCE_SUBSCRIBE,
                    },
                    {
                        "key": "enable_tg_panel",
                        "label": "启用 TG 面板",
                        "type": "bool",
                        "description": "启用后 Bot 提供完整的内联键盘面板功能，关闭则仅保留 /help、/bind、/me 基础命令",
                        "value": TelegramConfig.ENABLE_TG_PANEL,
                    },
                    {
                        "key": "require_group_membership",
                        "label": "强制群组成员资格",
                        "type": "bool",
                        "description": "开启后绑定时校验，且定时巡检；用户退出必需群组将被禁用并同步禁用 Emby",
                        "value": TelegramConfig.REQUIRE_GROUP_MEMBERSHIP,
                    },
                    {
                        "key": "group_check_interval_minutes",
                        "label": "群组检查间隔（分钟）",
                        "type": "int",
                        "description": "群组成员资格定时巡检间隔（分钟），开启上述开关后生效",
                        "value": TelegramConfig.GROUP_CHECK_INTERVAL_MINUTES,
                    },
                    {
                        "key": "group_check_concurrency",
                        "label": "群组巡检并发数",
                        "type": "int",
                        "description": "批量 get_chat_member 的有界并发数，过高可能触发 Telegram 限流，建议 8-32",
                        "value": TelegramConfig.GROUP_CHECK_CONCURRENCY,
                    },
                    {
                        "key": "group_action_concurrency",
                        "label": "群组操作并发数",
                        "type": "int",
                        "description": "批量禁用/踢出/封禁等写操作并发数，过高可能触发 Telegram 限流，建议 4-12",
                        "value": TelegramConfig.GROUP_ACTION_CONCURRENCY,
                    },
                    {
                        "key": "ban_on_leave",
                        "label": "退群完全封禁模式",
                        "type": "bool",
                        "description": '⚠️ 危险操作：开启后巡检发现退群用户会被 Bot 在所有 GROUP_ID 群里永久 ban（不会自动解封）。依赖 Bot 是群管理员且有封禁权限；开启后"重新入群识别"分支会被跳过。默认关闭，谨慎使用',
                        "value": TelegramConfig.BAN_ON_LEAVE,
                    },
                    {
                        "key": "auto_enable_rejoined",
                        "label": "回群后自动启用",
                        "type": "bool",
                        "description": "开启后定时巡检发现已禁用用户重新加入必需群组且账号未到期时，会自动启用系统账号并按到期状态恢复 Emby；退群完全封禁模式开启时不会生效",
                        "value": TelegramConfig.AUTO_ENABLE_REJOINED,
                    },
                    {
                        "key": "proxy_url",
                        "label": "代理地址",
                        "type": "string",
                        "description": "Telegram Bot 代理地址（如 socks5://127.0.0.1:1080），留空不使用代理",
                        "value": TelegramConfig.PROXY_URL,
                    },
                    {
                        "key": "bot_start_text",
                        "label": "/start 完整文本",
                        "type": "textarea",
                        "description": "完整覆盖私聊 /start 文本（Markdown），留空使用内置默认；支持 {server_name}/{user_name}/{bot_username}",
                        "value": TelegramConfig.BOT_START_TEXT,
                    },
                    {
                        "key": "bot_group_start_text",
                        "label": "群组 /start 文本",
                        "type": "textarea",
                        "description": "群组内发送 /start 时的提示文本，留空使用默认；支持 {server_name}/{bot_username}",
                        "value": TelegramConfig.BOT_GROUP_START_TEXT,
                    },
                    {
                        "key": "bot_start_title",
                        "label": "/start 标题",
                        "type": "string",
                        "description": "自定义 /start 第一行标题（Markdown），留空使用默认；支持 {server_name} 占位符",
                        "value": TelegramConfig.BOT_START_TITLE,
                    },
                    {
                        "key": "bot_start_intro",
                        "label": "/start 简介",
                        "type": "string",
                        "description": "自定义 /start 简介段落，留空使用默认",
                        "value": TelegramConfig.BOT_START_INTRO,
                    },
                    {
                        "key": "bot_bind_prompt_text",
                        "label": "/bind 引导文本",
                        "type": "textarea",
                        "description": "用户发送 /bind 但未带绑定码时的完整提示文本，留空使用默认；支持 {server_name}/{user_name}/{bot_username}",
                        "value": TelegramConfig.BOT_BIND_PROMPT_TEXT,
                    },
                    {
                        "key": "bot_help_text",
                        "label": "/twihelp 完整文本",
                        "type": "textarea",
                        "description": "完整覆盖普通帮助文本（Markdown），留空使用内置默认；支持 {server_name} 占位符",
                        "value": TelegramConfig.BOT_HELP_TEXT,
                    },
                    {
                        "key": "bot_admin_help_text",
                        "label": "/twishelp 完整文本",
                        "type": "textarea",
                        "description": "完整覆盖管理员帮助文本（Markdown），留空使用内置默认；支持 {server_name} 占位符",
                        "value": TelegramConfig.BOT_ADMIN_HELP_TEXT,
                    },
                    {
                        "key": "bot_help_header",
                        "label": "/help 顶部段",
                        "type": "textarea",
                        "description": "旧配置：仅追加到内置普通帮助命令列表前；bot_help_text 非空时不会使用",
                        "value": TelegramConfig.BOT_HELP_HEADER,
                    },
                    {
                        "key": "bot_help_footer",
                        "label": "/help 底部段",
                        "type": "textarea",
                        "description": "旧配置：仅追加到内置普通帮助命令列表末尾；bot_help_text 非空时不会使用",
                        "value": TelegramConfig.BOT_HELP_FOOTER,
                    },
                    {
                        "key": "bot_about",
                        "label": "关于 / 服务说明",
                        "type": "string",
                        "description": "关于 Bot / 站点的简介，预留用于 /about 等场景",
                        "value": TelegramConfig.BOT_ABOUT,
                    },
                ],
            },
            {
                "key": "SAR",
                "category": "user",
                "title": "注册与用户策略",
                "description": "注册、用户限制与 Emby 自由注册配置",
                "fields": [
                    {
                        "key": "register_mode",
                        "label": "注册模式",
                        "type": "bool",
                        "description": "是否开放注册",
                        "value": RegisterConfig.REGISTER_MODE,
                    },
                    {
                        "key": "register_code_limit",
                        "label": "注册码限制",
                        "type": "bool",
                        "description": "是否限制必须使用注册码注册",
                        "value": RegisterConfig.REGISTER_CODE_LIMIT,
                    },
                    {
                        "key": "user_limit",
                        "label": "用户上限",
                        "type": "int",
                        "description": "系统允许的最大注册用户数量",
                        "value": RegisterConfig.USER_LIMIT,
                    },
                    {
                        "key": "media_request_enabled",
                        "label": "启用求片功能",
                        "type": "bool",
                        "description": "是否允许用户提交求片请求，默认关闭",
                        "value": RegisterConfig.MEDIA_REQUEST_ENABLED,
                    },
                    {
                        "key": "max_concurrent_requests_per_user",
                        "label": "每用户最大同时求片数",
                        "type": "int",
                        "description": "每个用户允许同时存在的待处理或下载中的求片请求数量，-1 表示不限制",
                        "value": RegisterConfig.MAX_CONCURRENT_REQUESTS_PER_USER,
                    },
                    {
                        "key": "regcode_format",
                        "label": "卡码生成格式",
                        "type": "string",
                        "description": "可包含自定义文本；支持 {random}=随机部分、{type}=类型、{days}=天数、{index}=批量序号、{validity}=有效期、{limit}=次数上限，例如 TW-{type}-{random}",
                        "value": RegisterConfig.REGCODE_FORMAT,
                    },
                    {
                        "key": "regcode_random_algorithm",
                        "label": "卡码随机算法",
                        "type": "select",
                        "description": "随机部分生成算法。推荐 base32-20 或 base32-24；digits 仅适合口头传递，安全性低于字母数字混合；legacy-sha1 仅用于兼容旧样式。",
                        "value": RegisterConfig.REGCODE_RANDOM_ALGORITHM,
                        "options": [
                            {"label": "base32-20 推荐，易抄写", "value": "base32-20"},
                            {"label": "base32-24 高强度，易抄写", "value": "base32-24"},
                            {"label": "hex32 128-bit 十六进制", "value": "hex32"},
                            {"label": "hex20 旧默认", "value": "hex20"},
                            {"label": "base32-16 短码", "value": "base32-16"},
                            {"label": "alnum-24 高强度字母数字", "value": "alnum-24"},
                            {"label": "alnum-16 字母数字", "value": "alnum-16"},
                            {"label": "urlsafe-24 URL 安全字符", "value": "urlsafe-24"},
                            {"label": "digits-16 纯数字增强", "value": "digits-16"},
                            {"label": "digits-12 纯数字短码", "value": "digits-12"},
                            {"label": "uuid UUID v4", "value": "uuid"},
                            {"label": "legacy-sha1 旧版风格", "value": "legacy-sha1"},
                        ],
                    },
                    {
                        "key": "regcode_decoy_action",
                        "label": "诱饵卡码动作",
                        "type": "select",
                        "description": "假卡码被已登录用户使用后执行的动作",
                        "value": RegisterConfig.REGCODE_DECOY_ACTION,
                        "options": [
                            {"label": "只记录", "value": "none"},
                            {"label": "禁用用户", "value": "disable_user"},
                            {"label": "禁用用户并停用该码", "value": "disable_user_and_deactivate_code"},
                        ],
                    },
                    {
                        "key": "allow_pending_register",
                        "label": "允许无码注册",
                        "type": "bool",
                        "description": "是否允许无注册码注册（待激活状态）",
                        "value": RegisterConfig.ALLOW_PENDING_REGISTER,
                    },
                    {
                        "key": "allow_no_emby_view",
                        "label": "无Emby查看",
                        "type": "bool",
                        "description": "是否允许未激活 Emby 账户的用户查看部分信息",
                        "value": RegisterConfig.ALLOW_NO_EMBY_VIEW,
                    },
                    {
                        "key": "emby_direct_register_enabled",
                        "label": "开启 Emby 自由注册",
                        "type": "bool",
                        "description": "开启后用户可直接申请 Emby 账号（需先完成 Telegram 绑定）",
                        "value": RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED,
                    },
                    {
                        "key": "emby_direct_register_days",
                        "label": "自由注册开通天数",
                        "type": "int",
                        "description": "所有自由注册账号统一使用的开通天数（管理员固定，-1=永久）",
                        "value": RegisterConfig.EMBY_DIRECT_REGISTER_DAYS,
                    },
                    {
                        "key": "emby_user_limit",
                        "label": "Emby 用户上限",
                        "type": "int",
                        "description": "已绑定、待开通和队列待创建 Emby 的本站用户总上限，-1 表示不限制（自由注册队列、卡码队列、绑定已有 Emby 账户、管理员强制绑定都受此上限管控）",
                        "value": RegisterConfig.EMBY_USER_LIMIT,
                    },
                    {
                        "key": "emby_direct_register_workers",
                        "label": "自由注册并发 Worker",
                        "type": "int",
                        "description": "队列并发处理 worker 数（建议 4-16）",
                        "value": RegisterConfig.EMBY_DIRECT_REGISTER_WORKERS,
                    },
                    {
                        "key": "emby_direct_register_max_queue",
                        "label": "自由注册队列上限",
                        "type": "int",
                        "description": "允许排队的最大请求数，超过后直接拒绝",
                        "value": RegisterConfig.EMBY_DIRECT_REGISTER_MAX_QUEUE,
                    },
                    {
                        "key": "emby_direct_register_status_ttl",
                        "label": "注册状态保留秒数",
                        "type": "int",
                        "description": "注册完成后状态保留秒数，便于前端查询结果",
                        "value": RegisterConfig.EMBY_DIRECT_REGISTER_STATUS_TTL,
                    },
                    {
                        "key": "auto_cleanup_no_emby",
                        "label": "自动清理未绑定 Emby 用户",
                        "type": "bool",
                        "description": "开启后定时任务会自动删除注册超过指定天数但仍未绑定 Emby 的普通系统用户；管理员、白名单、未注册占位用户会被跳过。",
                        "value": RegisterConfig.AUTO_CLEANUP_NO_EMBY,
                    },
                    {
                        "key": "auto_cleanup_no_emby_days",
                        "label": "清理未绑定 Emby 天数",
                        "type": "int",
                        "description": "注册后超过多少天仍未绑定 Emby 才会被清理。定时任务默认读取这里的最新值；手动触发时可临时覆盖。",
                        "value": RegisterConfig.AUTO_CLEANUP_NO_EMBY_DAYS,
                    },
                    {
                        "key": "admin_uids",
                        "label": "管理员 UID",
                        "type": "string",
                        "description": '管理员 UID 列表，逗号分隔（如 "1,2,3"）',
                        "value": RegisterConfig.ADMIN_UIDS,
                    },
                    {
                        "key": "admin_usernames",
                        "label": "管理员用户名",
                        "type": "string",
                        "description": "管理员用户名列表，逗号分隔",
                        "value": RegisterConfig.ADMIN_USERNAMES,
                    },
                    {
                        "key": "white_list_uids",
                        "label": "白名单 UID",
                        "type": "string",
                        "description": "白名单 UID 列表，逗号分隔",
                        "value": RegisterConfig.WHITE_LIST_UIDS,
                    },
                    {
                        "key": "white_list_usernames",
                        "label": "白名单用户名",
                        "type": "string",
                        "description": "白名单用户名列表，逗号分隔",
                        "value": RegisterConfig.WHITE_LIST_USERNAMES,
                    },
                    {
                        "key": "invite_enabled",
                        "label": "启用邀请树",
                        "type": "bool",
                        "description": "启用后，已绑定 Emby 的用户可生成邀请码，被邀请人成为其下级，形成树状关系",
                        "value": RegisterConfig.INVITE_ENABLED,
                    },
                    {
                        "key": "invite_max_depth",
                        "label": "邀请最大层级",
                        "type": "int",
                        "description": "邀请树最大层级（B→A→C 计为 3）。1 表示禁止任何邀请",
                        "value": RegisterConfig.INVITE_MAX_DEPTH,
                    },
                    {
                        "key": "invite_limit",
                        "label": "单人邀请上限",
                        "type": "int",
                        "description": "每人最多同时存在多少未使用的邀请码，-1 = 无限制",
                        "value": RegisterConfig.INVITE_LIMIT,
                    },
                    {
                        "key": "invite_root_user_limit",
                        "label": "树根邀请用户上限",
                        "type": "int",
                        "description": "每棵邀请树最多可成功邀请多少用户，不含树根本人，-1 = 无限制",
                        "value": RegisterConfig.INVITE_ROOT_USER_LIMIT,
                    },
                    {
                        "key": "invite_require_emby",
                        "label": "邀请需绑定 Emby",
                        "type": "bool",
                        "description": "是否要求邀请人已绑定 Emby 账号才能生成邀请码",
                        "value": RegisterConfig.INVITE_REQUIRE_EMBY,
                    },
                    {
                        "key": "invite_code_default_days",
                        "label": "邀请码默认天数",
                        "type": "int",
                        "description": "被邀请人 Emby 账号默认开通天数（0 或 -1 表示永久）",
                        "value": RegisterConfig.INVITE_CODE_DEFAULT_DAYS,
                    },
                    # —— 签到 / 积分（原 [Signin] 节，并入此处。仅装饰用途，无排行榜）
                    {
                        "key": "signin_enabled",
                        "label": "启用签到",
                        "type": "bool",
                        "description": "是否开放签到功能（原 [Signin].enabled）",
                        "value": RegisterConfig.SIGNIN_ENABLED,
                    },
                    {
                        "key": "currency_name",
                        "label": "货币名称",
                        "type": "string",
                        "description": "展示用的货币名（如 星币 / 金币 / 云币）",
                        "value": RegisterConfig.CURRENCY_NAME,
                    },
                    {
                        "key": "daily_min",
                        "label": "每日最少奖励",
                        "type": "int",
                        "description": "单日签到获得的最少积分（含 0）",
                        "value": RegisterConfig.DAILY_MIN,
                    },
                    {
                        "key": "daily_max",
                        "label": "每日最多奖励",
                        "type": "int",
                        "description": "单日签到获得的最多积分（≥ 最少）",
                        "value": RegisterConfig.DAILY_MAX,
                    },
                    {
                        "key": "streak_bonus_enabled",
                        "label": "启用连签奖励",
                        "type": "bool",
                        "description": "关闭后只发放每日奖励，连签天数仍记录但不再额外赠送积分",
                        "value": RegisterConfig.STREAK_BONUS_ENABLED,
                    },
                    {
                        "key": "streak_bonus_days",
                        "label": "连签加成天数",
                        "type": "list",
                        "description": '达到该连签天数时获得额外奖励，与下方"加成积分"列表一一对应（仅在"启用连签奖励"开启时生效）',
                        "value": RegisterConfig.STREAK_BONUS_DAYS,
                    },
                    {
                        "key": "streak_bonus_points",
                        "label": "加成积分",
                        "type": "list",
                        "description": "上方天数对应的额外奖励积分",
                        "value": RegisterConfig.STREAK_BONUS_POINTS,
                    },
                    {
                        "key": "reset_after_miss",
                        "label": "漏签清零连签",
                        "type": "bool",
                        "description": "关闭后即使漏签也保留并累计连签",
                        "value": RegisterConfig.RESET_AFTER_MISS,
                    },
                ],
            },
            {
                "key": "DeviceLimit",
                "category": "media",
                "title": "设备限制",
                "description": "用户设备和播放流数限制",
                "fields": [
                    {
                        "key": "device_limit_enabled",
                        "label": "启用设备限制",
                        "type": "bool",
                        "description": "是否限制用户的设备数量",
                        "value": DeviceLimitConfig.DEVICE_LIMIT_ENABLED,
                    },
                    {
                        "key": "max_devices",
                        "label": "最大设备数",
                        "type": "int",
                        "description": "每个用户允许的最大设备数",
                        "value": DeviceLimitConfig.MAX_DEVICES,
                    },
                    {
                        "key": "max_streams",
                        "label": "最大同时播放",
                        "type": "int",
                        "description": "每个用户允许的最大同时播放流数",
                        "value": DeviceLimitConfig.MAX_STREAMS,
                    },
                    {
                        "key": "kick_oldest_session",
                        "label": "踢出最早会话",
                        "type": "bool",
                        "description": "超过限制时是否自动踢掉最早的会话",
                        "value": DeviceLimitConfig.KICK_OLDEST_SESSION,
                    },
                ],
            },
            {
                "key": "API",
                "category": "api",
                "title": "API 服务器",
                "description": "Web API 服务器配置",
                "fields": [
                    {
                        "key": "host",
                        "label": "监听地址",
                        "type": "string",
                        "description": "API 服务器监听地址（0.0.0.0 表示所有接口）",
                        "value": APIConfig.HOST,
                    },
                    {
                        "key": "port",
                        "label": "端口",
                        "type": "int",
                        "description": "API 服务器监听端口",
                        "value": APIConfig.PORT,
                    },
                    {
                        "key": "debug",
                        "label": "调试模式",
                        "type": "bool",
                        "description": "是否开启调试模式（生产环境请关闭）",
                        "value": APIConfig.DEBUG,
                    },
                    {
                        "key": "token_expire",
                        "label": "Token 有效期",
                        "type": "int",
                        "description": "用户登录 Token 有效期（秒）",
                        "value": APIConfig.TOKEN_EXPIRE,
                    },
                    {
                        "key": "cors_enabled",
                        "label": "启用 CORS",
                        "type": "bool",
                        "description": "是否允许跨域请求",
                        "value": APIConfig.CORS_ENABLED,
                    },
                    {
                        "key": "cors_origins",
                        "label": "CORS 白名单",
                        "type": "list",
                        "description": '允许跨域请求的源地址列表；含 "*" 时浏览器禁用 Cookie（无法登录），生产请显式填前端域名',
                        "value": APIConfig.CORS_ORIGINS,
                    },
                    {
                        "key": "upload_folder",
                        "label": "上传目录",
                        "type": "string",
                        "description": "文件上传目录（绝对路径或相对项目根目录）",
                        "value": APIConfig.UPLOAD_FOLDER,
                    },
                    {
                        "key": "max_upload_size",
                        "label": "最大上传大小（字节）",
                        "type": "int",
                        "description": "单文件上传字节上限，默认 5 MiB",
                        "value": APIConfig.MAX_UPLOAD_SIZE,
                    },
                    {
                        "key": "session_cookie_name",
                        "label": "会话 Cookie 名",
                        "type": "string",
                        "description": "保存登录会话的 Cookie 名称",
                        "value": APIConfig.SESSION_COOKIE_NAME,
                    },
                    {
                        "key": "session_cookie_secure",
                        "label": "Cookie 仅 HTTPS",
                        "type": "bool",
                        "description": "生产强烈建议开启；HTTP 调试时关闭",
                        "value": APIConfig.SESSION_COOKIE_SECURE,
                    },
                    {
                        "key": "session_cookie_samesite",
                        "label": "Cookie SameSite",
                        "type": "select",
                        "description": "Strict 最严格；前后端跨域部署需要 None（同时 secure=true）",
                        "value": APIConfig.SESSION_COOKIE_SAMESITE,
                        "options": [
                            {"label": "Strict", "value": "Strict"},
                            {"label": "Lax", "value": "Lax"},
                            {"label": "None", "value": "None"},
                        ],
                    },
                    {
                        "key": "session_cookie_domain",
                        "label": "Cookie 域",
                        "type": "string",
                        "description": "为空使用请求 Host；跨子域共享会话时填顶级域，如 .example.com",
                        "value": APIConfig.SESSION_COOKIE_DOMAIN,
                    },
                    {
                        "key": "session_cookie_path",
                        "label": "Cookie 路径",
                        "type": "string",
                        "description": "默认 / 即可",
                        "value": APIConfig.SESSION_COOKIE_PATH,
                    },
                ],
                # 已删除字段：api_key_length（apikey 模块用固定长度生成）
            },
            {
                "key": "Security",
                "category": "api",
                "title": "安全配置",
                "description": "登录失败锁定等安全策略",
                "fields": [
                    {
                        "key": "login_fail_threshold",
                        "label": "登录失败阈值",
                        "type": "int",
                        "description": "连续登录失败多少次后锁定账号",
                        "value": SecurityConfig.LOGIN_FAIL_THRESHOLD,
                    },
                    {
                        "key": "lockout_minutes",
                        "label": "锁定时间",
                        "type": "int",
                        "description": "账号锁定持续时间（分钟）",
                        "value": SecurityConfig.LOCKOUT_MINUTES,
                    },
                    {
                        "key": "telegram_direct_login_enabled",
                        "label": "Telegram 直登",
                        "type": "bool",
                        "description": "是否允许仅凭 Telegram ID 直接换取登录会话（高风险）",
                        "value": SecurityConfig.TELEGRAM_DIRECT_LOGIN_ENABLED,
                    },
                    {
                        "key": "apikey_direct_login_enabled",
                        "label": "API Key 直登",
                        "type": "bool",
                        "description": "是否允许通过 API Key 直接换取完整会话 Token",
                        "value": SecurityConfig.APIKEY_DIRECT_LOGIN_ENABLED,
                    },
                    {
                        "key": "bot_internal_secret",
                        "label": "Bot 内部密钥",
                        "type": "secret",
                        "description": "Bot 调用内部接口的密钥（建议显式配置）",
                        "value": SecurityConfig.BOT_INTERNAL_SECRET,
                    },
                ],
            },
            {
                "key": "Scheduler",
                "category": "automation",
                "title": "定时任务",
                "description": "仅保留全局开关与时区；每个任务的执行时间 / 间隔请前往「定时任务」页面编辑。",
                "fields": [
                    {
                        "key": "enabled",
                        "label": "启用定时任务",
                        "type": "bool",
                        "description": "是否启用整个定时任务系统（关闭后所有任务都不再触发）",
                        "value": SchedulerConfig.ENABLED,
                    },
                    {
                        "key": "timezone",
                        "label": "时区",
                        "type": "string",
                        "description": "定时任务使用的时区，如 Asia/Shanghai",
                        "value": SchedulerConfig.TIMEZONE,
                    },
                ],
                # 已迁出的字段（仍在 toml 中作为缺省值；不再向「配置管理」UI 暴露）：
                #   expired_check_time / expiring_check_time / daily_stats_time
                #   session_cleanup_interval / emby_sync_interval
                # 改为在「定时任务」页面按 job 维度调整，避免双入口冲突。
            },
            {
                "key": "Notification",
                "category": "automation",
                "title": "通知配置",
                "description": "系统通知相关设置",
                "fields": [
                    {
                        "key": "enabled",
                        "label": "启用通知",
                        "type": "bool",
                        "description": "是否启用通知系统",
                        "value": NotificationConfig.ENABLED,
                    },
                    {
                        "key": "expiry_remind_days",
                        "label": "到期提醒天数",
                        "type": "int",
                        "description": "提前多少天提醒用户即将到期",
                        "value": NotificationConfig.EXPIRY_REMIND_DAYS,
                    },
                    {
                        "key": "new_media_notify",
                        "label": "新媒体通知",
                        "type": "bool",
                        "description": "有新媒体入库时是否通知",
                        "value": NotificationConfig.NEW_MEDIA_NOTIFY,
                    },
                ],
            },
            {
                "key": "SystemUpdate",
                "category": "automation",
                "title": "系统自动更新",
                "description": "从 Git 仓库自动拉取更新；不会安装 Python 依赖。",
                "fields": [
                    {
                        "key": "auto_update_enabled",
                        "label": "启用自动更新",
                        "type": "bool",
                        "description": "默认关闭；开启后由调度器按下方策略自动拉取 Git 更新",
                        "value": SystemUpdateConfig.AUTO_UPDATE_ENABLED,
                    },
                    {
                        "key": "repo_url",
                        "label": "Git 仓库地址",
                        "type": "string",
                        "description": "仅支持 https 仓库地址",
                        "value": SystemUpdateConfig.REPO_URL,
                    },
                    {
                        "key": "branch",
                        "label": "分支",
                        "type": "string",
                        "description": "要拉取的分支，默认 main",
                        "value": SystemUpdateConfig.BRANCH,
                    },
                    {
                        "key": "restart_services",
                        "label": "更新后重启服务",
                        "type": "bool",
                        "description": "Linux/systemd 环境下自动重启 twilight / twilight-bot / twilight-scheduler",
                        "value": SystemUpdateConfig.RESTART_SERVICES,
                    },
                    {
                        "key": "auto_update_trigger_type",
                        "label": "自动更新时间类型",
                        "type": "select",
                        "description": "interval=固定间隔；cron_daily=每日固定时间",
                        "value": SystemUpdateConfig.AUTO_UPDATE_TRIGGER_TYPE,
                        "options": [
                            {"label": "固定间隔", "value": "interval"},
                            {"label": "每日固定时间", "value": "cron_daily"},
                        ],
                    },
                    {
                        "key": "auto_update_interval_hours",
                        "label": "自动更新间隔（小时）",
                        "type": "int",
                        "description": "选择固定间隔时生效，最小 1 小时",
                        "value": SystemUpdateConfig.AUTO_UPDATE_INTERVAL_HOURS,
                    },
                    {
                        "key": "auto_update_time",
                        "label": "每日自动更新时间",
                        "type": "string",
                        "description": "选择每日固定时间时生效，格式 HH:MM",
                        "value": SystemUpdateConfig.AUTO_UPDATE_TIME,
                    },
                ],
            },
            {
                "key": "BangumiSync",
                "category": "integration",
                "title": "Bangumi 点格子",
                "description": "通过 Emby Webhook 在用户看完后同步 Bangumi 观看进度",
                "fields": [
                    {
                        "key": "enabled",
                        "label": "启用同步",
                        "type": "bool",
                        "description": "是否启用 Bangumi 点格子功能。关闭时用户侧 Bangumi 配置面板不会显示，Webhook 也会拒绝处理。",
                        "value": BangumiSyncConfig.ENABLED,
                    },
                    {
                        "key": "webhook_secret",
                        "label": "Webhook 密钥",
                        "type": "secret",
                        "description": "Emby 通知 Webhook 调用时携带的共享密钥；建议设置高熵随机值，并在 URL 中追加 ?token=该值。留空则不校验密钥。",
                        "value": BangumiSyncConfig.WEBHOOK_SECRET,
                    },
                    {
                        "key": "auto_add_collection",
                        "label": "自动收藏",
                        "type": "bool",
                        "description": "用户未收藏该条目时，是否自动加入 Bangumi 收藏并设为在看。关闭后未收藏条目不会被同步。",
                        "value": BangumiSyncConfig.AUTO_ADD_COLLECTION,
                    },
                    {
                        "key": "private_collection",
                        "label": "私有收藏",
                        "type": "bool",
                        "description": "自动收藏或调整收藏状态时是否设为私有。单集点格子仍使用 Bangumi 的章节收藏接口。",
                        "value": BangumiSyncConfig.PRIVATE_COLLECTION,
                    },
                    {
                        "key": "block_keywords",
                        "label": "屏蔽关键词",
                        "type": "list",
                        "description": "不同步的条目关键词列表",
                        "value": BangumiSyncConfig.BLOCK_KEYWORDS,
                    },
                    {
                        "key": "min_progress_percent",
                        "label": "最小播放进度",
                        "type": "int",
                        "description": "Emby playback.stop 未携带 PlayedToCompletion=true 时，播放进度达到多少百分比才算看完并同步。",
                        "value": BangumiSyncConfig.MIN_PROGRESS_PERCENT,
                    },
                ],
            },
        ],
    }

    _augment_schema_with_missing_fields(schema)
    return api_response(True, "获取成功", schema)


@system_bp.route("/admin/config/schema", methods=["PUT"])
@require_auth
@require_admin
async def update_config_by_schema():
    """通过结构化数据更新配置（管理员）"""
    import toml

    data = request.get_json() or {}
    sections = data.get("sections", {})

    if not sections:
        return api_response(False, "缺少配置数据", code=400)

    for section_key, fields in sections.items():
        if not isinstance(fields, dict):
            return api_response(False, f"配置节 {section_key} 格式错误", code=400)
        for field_key, value in fields.items():
            allowed_values = _CONFIG_SELECT_ALLOWED_VALUES.get((section_key, field_key))
            if allowed_values is None:
                continue
            normalized_value = value
            if (section_key, field_key) == ("Global", "log_level"):
                try:
                    normalized_value = int(value)
                except (TypeError, ValueError):
                    return api_response(False, "log_level 必须从预设日志等级中选择", code=400)
            if normalized_value not in allowed_values:
                return api_response(False, f"{section_key}.{field_key} 必须从预设选项中选择", code=400)

    config_file = get_primary_config_path()

    # 读取当前配置
    try:
        config = toml.load(config_file)
    except Exception as e:
        return api_response(False, f"读取配置文件失败: {e}", code=500)

    backup_file: Optional[Path] = None
    try:
        backup_file = _backup_config_before_update("admin-schema")
    except Exception as e:
        _reload_logger.warning(f"备份配置文件失败: {e}")

    # 更新配置
    for section_key, fields in sections.items():
        if section_key not in config:
            config[section_key] = {}
        for field_key, value in fields.items():
            if (section_key, field_key) == ("Global", "log_level"):
                value = int(value)
            config[section_key][field_key] = value

    # 写入文件
    try:
        with open(config_file, "w", encoding="utf-8") as f:
            toml.dump(config, f)

        sweep_result = sweep_config_toml(config_classes=_CONFIG_CLASSES, auto_backup=False)

        reload_result = await _apply_runtime_hot_reload()

        return api_response(
            True,
            "配置已保存并热重载",
            {
                "backup_path": str(backup_file) if backup_file else None,
                "filled": sweep_result.get("filled") or {},
                "removed": sweep_result.get("removed") or {},
                "migrated": sweep_result.get("migrated") or [],
                "restart": False,
                "reload": reload_result,
            },
        )
    except Exception as e:
        import logging

        logging.getLogger(__name__).error(f"更新配置文件失败: {e}", exc_info=True)

        # 尝试恢复备份
        if backup_file and backup_file.exists():
            try:
                shutil.copy2(backup_file, config_file)
            except Exception:
                pass

        return api_response(False, f"更新配置文件失败: {e}", code=500)


@system_bp.route("/admin/config/sweep", methods=["POST"])
@require_auth
@require_admin
async def admin_trigger_config_sweep():
    """手动触发 config.toml 整理：迁移历史 section、清理无效字段、补齐缺失默认值。

    Request body (可选):
        {
            "auto_backup": true,    // 默认 true；写入前在 config_backups/ 备份
            "restart": false        // 默认 false；为 true 时整理完成后调度重启进程
        }

    Response:
        {
            "filled": {section: [keys...]},      // 本次补齐的字段
            "removed": {section: [keys...]},     // 本次删除的字段；__section_removed__ 列整段被删的 section
            "migrated": [string, ...],           // 本次迁移的历史 section.key → 新归属 描述
            "backup_path": "<rotated.bak>" | null,
            "restart": true | false              // 实际是否调度了重启
        }
    """
    data = request.get_json(silent=True) or {}
    auto_backup = parse_bool(data.get("auto_backup"), default=True)
    restart = parse_bool(data.get("restart"), default=False)

    try:
        result = sweep_config_toml(
            config_classes=_CONFIG_CLASSES,
            auto_backup=auto_backup,
        )
    except Exception as exc:  # pragma: no cover - 防御性
        _reload_logger.error("管理员触发 sweep 失败: %s", exc, exc_info=True)
        return api_response(False, f"整理配置失败: {exc}", code=500)

    if result.get("error"):
        return api_response(False, result["error"], result, code=500)

    # 内存里的 Config 对象同步刷新一次，避免前端看到旧值；本进程调度器也顺手重装任务。
    reload_result = await _apply_runtime_hot_reload()

    payload = {
        "filled": result.get("filled") or {},
        "removed": result.get("removed") or {},
        "migrated": result.get("migrated") or [],
        "backup_path": result.get("backup_path"),
        "restart": False,
        "reload": reload_result,
    }

    # 整理本身不强求重启；但若调用方显式要求 或 实际产生过变更，则按现有流程退出进程
    has_change = bool(payload["filled"] or payload["removed"] or payload["migrated"])
    if restart and has_change:
        payload["restart"] = True
        _schedule_process_restart()

    msg = "已整理 config.toml" if has_change else "config.toml 已是最新，无需整理"
    return api_response(True, msg, payload)


@system_bp.route("/admin/apis", methods=["GET"])
@require_auth
@require_admin
async def list_all_apis():
    """获取所有 API 列表（管理员）"""
    from flask import current_app

    apis = []

    # 遍历所有注册的蓝图和路由
    for rule in current_app.url_map.iter_rules():
        # 过滤掉静态文件、根路径和 OPTIONS 方法
        if rule.endpoint == "static" or rule.rule == "/" or "OPTIONS" in rule.methods:
            continue

        # 只获取 /api/v1 开头的路由
        if not rule.rule.startswith("/api/v1"):
            continue

        # 获取方法
        methods = [m for m in rule.methods if m != "OPTIONS" and m != "HEAD"]
        if not methods:
            continue

        # 构建路径（移除 /api/v1 前缀以便前端使用）
        path = rule.rule[7:]  # 移除 '/api/v1'

        for method in methods:
            apis.append(
                {
                    "method": method,
                    "path": path,
                    "endpoint": rule.endpoint,
                    "full_path": rule.rule,
                }
            )

    # 按路径和方法排序
    apis.sort(key=lambda x: (x["path"], x["method"]))

    return api_response(
        True,
        "获取成功",
        {
            "apis": apis,
            "total": len(apis),
        },
    )


@system_bp.route("/admin/emby/libraries", methods=["GET"])
@require_auth
@require_admin
async def get_emby_libraries():
    """获取所有 Emby 媒体库列表（管理员）"""
    from src.services import EmbyService

    libraries = await EmbyService.get_libraries_info()
    return api_response(True, "获取成功", libraries)


# ==================== Bot 连通性测试 ====================


@system_bp.route("/admin/bot/test", methods=["POST"])
@require_auth
@require_admin
async def test_bot_connectivity():
    """
    测试 Telegram Bot 连通性

    通过独立的 HTTP 请求直接调用 Telegram Bot API，
    不复用全局运行中的 Bot 实例，避免跨事件循环导致的异常。
    """
    import time
    import httpx

    if not Config.TELEGRAM_MODE:
        return api_response(False, "Telegram 模式未启用", code=400)

    if not TelegramConfig.BOT_TOKEN:
        return api_response(False, "未配置 Bot Token", code=400)

    data = request.get_json() or {}
    target = data.get("target")  # 可选：指定群组/频道，不传则发送到所有配置的群组和频道

    # 解析目标列表
    targets = []
    if target:
        targets = [target]
    else:
        group_ids = TelegramConfig.GROUP_ID
        if isinstance(group_ids, (int, str)):
            group_ids = [group_ids] if group_ids else []
        channel_ids = TelegramConfig.CHANNEL_ID
        if isinstance(channel_ids, (int, str)):
            channel_ids = [channel_ids] if channel_ids else []
        targets = list(group_ids) + list(channel_ids)

    # 构造 Telegram Bot API 基础地址（兼容自定义代理 API）
    api_base = (TelegramConfig.TELEGRAM_API_URL or "https://api.telegram.org/bot").rstrip("/")
    if api_base.endswith("/bot"):
        api_url = f"{api_base}{TelegramConfig.BOT_TOKEN}"
    else:
        api_url = f"{api_base}/bot{TelegramConfig.BOT_TOKEN}"

    proxy = TelegramConfig.PROXY_URL or None
    timeout = 15.0

    # 先验证 Token 是否有效（getMe），独立于全局 Bot 线程
    try:
        async with httpx.AsyncClient(timeout=timeout, proxy=proxy) as client:
            resp = await client.get(f"{api_url}/getMe")
            payload = resp.json() if resp.content else {}
            if resp.status_code != 200 or not payload.get("ok"):
                desc = payload.get("description") or f"HTTP {resp.status_code}"
                return api_response(False, f"Bot Token 验证失败: {desc}", code=400)
            bot_info = payload.get("result") or {}
    except Exception as e:
        return api_response(False, f"无法连接 Telegram Bot API: {e}", code=400)

    if not targets:
        return api_response(
            True,
            "Bot Token 有效，但未配置群组或频道，跳过发送测试",
            {
                "bot": {
                    "id": bot_info.get("id"),
                    "username": bot_info.get("username"),
                    "first_name": bot_info.get("first_name"),
                },
                "results": [],
            },
        )

    test_time = time.strftime("%Y-%m-%d %H:%M:%S")
    test_text = (
        f"✅ *Twilight Bot 连通性测试*\n\n"
        f"服务器: {Config.SERVER_NAME}\n"
        f"时间: {test_time}\n\n"
        f"此消息用于验证 Bot 与群组/频道的连通性。"
    )

    results = []
    async with httpx.AsyncClient(timeout=timeout, proxy=proxy) as client:
        for chat_id in targets:
            try:
                resp = await client.post(
                    f"{api_url}/sendMessage",
                    json={
                        "chat_id": chat_id,
                        "text": test_text,
                        "parse_mode": "Markdown",
                    },
                )
                payload = resp.json() if resp.content else {}
                ok = resp.status_code == 200 and bool(payload.get("ok"))
                results.append(
                    {
                        "target": str(chat_id),
                        "success": ok,
                        "error": None if ok else (payload.get("description") or f"HTTP {resp.status_code}"),
                    }
                )
            except Exception as e:
                results.append(
                    {
                        "target": str(chat_id),
                        "success": False,
                        "error": str(e),
                    }
                )

    all_ok = all(r["success"] for r in results)
    return api_response(
        all_ok,
        "测试完成" if all_ok else "部分目标发送失败",
        {
            "bot": {
                "id": bot_info.get("id"),
                "username": bot_info.get("username"),
                "first_name": bot_info.get("first_name"),
            },
            "results": results,
        },
    )
