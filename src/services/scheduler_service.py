import asyncio
import logging
import os
import time
from datetime import datetime
from pathlib import Path
from typing import Any, Awaitable, Callable, Optional

from apscheduler.schedulers.asyncio import AsyncIOScheduler
from apscheduler.triggers.cron import CronTrigger
from apscheduler.triggers.interval import IntervalTrigger
from src.config import RegisterConfig, SchedulerConfig, SystemUpdateConfig, TelegramConfig, get_primary_config_path
from src.db.scheduler_run import SchedulerRunOperate
from src.db.scheduler_schedule import (
    MAX_INTERVAL_SECONDS,
    MIN_INTERVAL_SECONDS,
    SchedulerScheduleOperate,
    TRIGGER_CRON_DAILY,
    TRIGGER_INTERVAL,
)
from src.db.user import UserOperate
from src.services import get_emby_client, EmbyService
from src.core.utils import timestamp, format_duration
from src.services.user_service import UserService

logger = logging.getLogger(__name__)


class RunContext:
    """传给 job 函数的上下文：累加 summary 字段、追加日志。

    用法：
        async def my_job(ctx: RunContext):
            ctx.log("开始处理…")
            ctx.summary['scanned'] = 10
            ctx.summary['disabled'] = 0

    ``trigger`` 记录此次执行的来源（``scheduled`` / ``manual`` / ``startup``）。
    每条日志行会自动带上 ``[auto]`` / ``[manual]`` / ``[startup]`` 前缀，方便
    审计和前端区分手动触发 vs 自动调度。
    """

    # trigger → 日志前缀
    _TRIGGER_TAG_MAP = {
        "scheduled": "auto",
        "manual": "manual",
        "startup": "startup",
    }

    def __init__(
        self,
        job_id: str,
        trigger: str = "scheduled",
        params: Optional[dict] = None,
    ):
        self.job_id = job_id
        self.trigger = trigger
        # 手动触发可携带 params 覆盖任务行为（如 cleanup_no_emby 的 days 阈值）；
        # 自动调度走 config 默认值，params 为空 dict。
        self.params: dict = dict(params or {})
        self.summary: dict[str, Any] = {}
        self.logs: list[str] = []
        self._max_logs = 800  # 内存里挡一道，落库会再截断

    @property
    def _trigger_tag(self) -> str:
        return self._TRIGGER_TAG_MAP.get(self.trigger, self.trigger or "auto")

    def log(self, message: str) -> None:
        tag = self._trigger_tag
        line = f"[{time.strftime('%H:%M:%S')}] [{tag}] {message}"
        if len(self.logs) >= self._max_logs:
            # 丢掉最早的，保留最新
            del self.logs[: len(self.logs) - self._max_logs + 1]
        self.logs.append(line)
        logger.info(f"[{self.job_id}][{tag}] {message}")


class SchedulerService:
    _scheduler = None
    _scheduler_loop: Optional[asyncio.AbstractEventLoop] = None
    # 每个 job 的最近一次执行情况（运行中态用，DB 持久化负责完成态）
    # { job_id: {status, started_at, finished_at, error, summary?, trigger?} }
    _last_runs: dict[str, dict] = {}
    # 当前正在「运行中」的 job ID（用于幂等避免重复触发）
    _running: set[str] = set()
    _config_watch_task: Optional[asyncio.Task] = None
    _config_mtime: Optional[float] = None
    _lock_path: Optional[Path] = None

    # ============== Job 元数据注册表（用于前端列表 / 手动触发权限） ==============
    # 每条目: id, name, description, default_trigger(默认触发器规格)
    # default_trigger 取值：
    #   {'type': 'cron_daily', 'hour_from': 'EXPIRED_CHECK_TIME', 'offset_minutes': 0}
    #   {'type': 'interval', 'seconds_from': ('SchedulerConfig', 'SESSION_CLEANUP_INTERVAL', 3600)}
    # 解析在 `_resolve_default_trigger` 里完成，避免硬编码具体配置字段读法
    JOB_DEFINITIONS = [
        {
            "id": "check_expired",
            "name": "过期用户检查",
            "description": "查找已过期账号，禁用本地状态并同步禁用 Emby 账户。",
            "default_trigger": {"type": "cron_daily", "config_field": "EXPIRED_CHECK_TIME"},
        },
        {
            "id": "check_expiring",
            "name": "即将过期检查",
            "description": "记录 3 天内到期的账号，供后续提醒任务使用。",
            "default_trigger": {"type": "cron_daily", "config_field": "EXPIRING_CHECK_TIME"},
        },
        {
            "id": "expiry_reminders",
            "name": "到期提醒推送",
            "description": "向到期前 N 天的用户发送提醒消息。",
            "default_trigger": {
                "type": "cron_daily",
                "config_field": "EXPIRING_CHECK_TIME",
                "offset_minutes": 5,
            },
        },
        {
            "id": "daily_stats",
            "name": "每日统计汇总",
            "description": "汇总注册用户 / 活跃用户 / 注册码 / Emby 状态写入日志。",
            "default_trigger": {"type": "cron_daily", "config_field": "DAILY_STATS_TIME"},
        },
        {
            "id": "cleanup_sessions",
            "name": "不活跃会话清理",
            "description": "巡检 Emby 当前会话数。",
            "default_trigger": {
                "type": "interval",
                "config_field": "SESSION_CLEANUP_INTERVAL",
                "unit": "hours",
            },
        },
        {
            "id": "emby_sync",
            "name": "Emby 与系统记录同步（手动）",
            "description": "手动校对本地 EMBYID、用户名、启停状态与下载权限；默认不自动执行。",
            "default_trigger": None,
            "manual_only": True,
        },
        {
            "id": "cleanup_no_emby",
            "name": "清理 x 天以上未绑定 Emby 的系统用户",
            "description": "按 [SAR].auto_cleanup_no_emby_days 清理注册超过 N 天仍未绑定 Emby 的系统用户；[SAR].auto_cleanup_no_emby 控制定时自动执行，手动执行可临时覆盖天数。",
            "default_trigger": {
                "type": "cron_daily",
                "config_field": "EXPIRED_CHECK_TIME",
                "offset_minutes": 30,
            },
        },
        {
            "id": "enforce_group_membership",
            "name": "Telegram 群组成员资格巡检",
            "description": "检查已绑定 Telegram 的用户是否仍在必需群组内；退群禁用/永封、回群自动启用由 Telegram 配置控制。",
            "default_trigger": {
                "type": "interval",
                "config_field": "GROUP_CHECK_INTERVAL_MINUTES",
                "unit": "minutes",
                "source": "TelegramConfig",
            },
        },
        {
            "id": "check_telegram_bindings",
            "name": "Telegram 绑定状态一致性检查",
            "description": "检查系统用户的 Telegram 绑定是否重复、非法，及换绑申请状态是否与用户绑定状态一致；仅记录问题，不自动改库。",
            "default_trigger": {
                "type": "cron_daily",
                "config_field": "DAILY_STATS_TIME",
                "offset_minutes": 15,
            },
        },
        {
            "id": "system_auto_update",
            "name": "系统自动更新",
            "description": "按 [SystemUpdate] 配置从 Git 仓库自动拉取代码更新；不会安装 Python 依赖。",
            "default_trigger": {"type": "system_update"},
        },
        {
            "id": "cleanup_unused_uploads",
            "name": "未使用头像/背景图片清理",
            "description": "清理未被任何用户头像或背景配置引用的上传图片；新上传文件保留 24 小时宽限期。",
            "default_trigger": {
                "type": "cron_daily",
                "config_field": "DAILY_STATS_TIME",
                "offset_minutes": 45,
            },
        },
        {
            "id": "kick_unknown_group_members",
            "name": "Telegram 群组非系统成员清理（手动）",
            "description": (
                "⚠️ 仅手动触发：扫描已知 Telegram 用户 ID（来自 users 表与 telegram_bind_codes），"
                '把其中"已经不在系统活跃用户里"且仍在群组中的 TG 账号"临时踢出"（ban + 立刻 unban）。'
                "排除 Bot/群管理员/群主以及配置中的管理员账号。"
                "受 Bot API 限制，无法枚举系统从未见过的全量群成员。"
            ),
            "default_trigger": None,
            "manual_only": True,
        },
    ]

    @classmethod
    def _record_run_start(cls, job_id: str, trigger: str) -> int:
        started = int(time.time())
        run_type = "manual" if trigger == "manual" else "auto"
        cls._last_runs[job_id] = {
            "status": "running",
            "type": run_type,
            "started_at": started,
            "finished_at": None,
            "error": None,
            "summary": None,
            "trigger": trigger,
        }
        cls._running.add(job_id)
        return started

    @classmethod
    def _record_run_end(
        cls,
        job_id: str,
        started: int,
        trigger: str,
        error: Optional[str],
        summary: Optional[dict],
    ) -> None:
        run_type = "manual" if trigger == "manual" else "auto"
        cls._last_runs[job_id] = {
            "status": "failed" if error else "success",
            "type": run_type,
            "started_at": started,
            "finished_at": int(time.time()),
            "error": (error or None) and str(error)[:500],
            "summary": summary or None,
            "trigger": trigger,
        }
        cls._running.discard(job_id)

    @classmethod
    async def _run_with_tracking(
        cls,
        job_id: str,
        fn: Callable[..., Awaitable[Any]],
        *,
        trigger: str = "scheduled",
        params: Optional[dict] = None,
    ) -> dict:
        """执行 job 并把 last-run 落库。供 APScheduler 调度与管理员手动触发共用。

        `fn` 既兼容老签名 `async def f()`，也接受新签名 `async def f(ctx: RunContext)`。
        ``params`` 仅在手动触发时使用，自动调度路径传 ``None``。
        """
        if job_id in cls._running:
            return cls._last_runs.get(job_id, {"status": "running"})

        started = cls._record_run_start(job_id, trigger)
        run_type = "manual" if trigger == "manual" else "auto"
        logger.info(f"▶️ 任务 {job_id} 开始执行 ({trigger})")

        # 落库一条「运行中」记录，结束后回填
        try:
            run_id = await SchedulerRunOperate.start_run(job_id, trigger=trigger)
        except Exception as exc:  # pragma: no cover - 数据库不可用时不阻塞主任务
            logger.warning(f"无法创建 scheduler_run 记录: {exc}")
            run_id = 0

        ctx = RunContext(job_id, trigger=trigger, params=params)
        error_text: Optional[str] = None
        try:
            await fn(ctx)
        except Exception as exc:
            error_text = str(exc) or exc.__class__.__name__
            ctx.log(f"❌ 任务执行异常: {exc}")
            logger.exception(f"❌ 任务 {job_id} 执行异常: {exc}")
        finally:
            cls._record_run_end(
                job_id,
                started,
                trigger,
                error_text,
                dict(ctx.summary) if ctx.summary else None,
            )
            if run_id:
                try:
                    await SchedulerRunOperate.finish_run(
                        run_id,
                        status="failed" if error_text else "success",
                        error=error_text,
                        summary=ctx.summary or None,
                        logs=ctx.logs or None,
                    )
                    await SchedulerRunOperate.trim_history(job_id)
                except Exception as exc:  # pragma: no cover
                    logger.warning(f"无法回填 scheduler_run #{run_id}: {exc}")
            try:
                await SchedulerScheduleOperate.mark_run_time(job_id, run_type=run_type, run_at=started)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"无法更新 scheduler 固定信息运行时间: {exc}")

        result = cls._last_runs[job_id]
        elapsed = (result["finished_at"] or started) - started
        if error_text:
            logger.info(f"⏹️ 任务 {job_id} 失败结束 (耗时 {elapsed}s)")
        else:
            logger.info(f"✅ 任务 {job_id} 完成 (耗时 {elapsed}s)")
        return result

    @classmethod
    def _make_scheduled(cls, job_id: str, fn: Callable[..., Awaitable[Any]]):
        """生成给 APScheduler 用的 async wrapper（带 last-run 追踪）。"""

        async def runner():
            await cls._run_with_tracking(job_id, fn, trigger="scheduled")

        runner.__name__ = f"_run_{job_id}"
        return runner

    # ============== 触发器解析 ==============

    @staticmethod
    def _parse_time_str(time_str: str) -> tuple[int, int]:
        try:
            hour, minute = map(int, time_str.split(":"))
            return hour, minute
        except Exception:
            return 0, 0

    @classmethod
    def _resolve_default_trigger(cls, definition: dict) -> dict:
        """根据 JOB_DEFINITIONS 的 default_trigger 描述，解析出当前 config.toml 下的实际触发器。

        返回结构：
            {'type': 'cron_daily', 'hour': 3, 'minute': 0}
            {'type': 'interval', 'seconds': 3600}
        手动专属任务（``default_trigger=None``）返回 ``{'type': 'manual'}``。
        """
        if definition.get("manual_only"):
            return {"type": "manual"}

        spec = definition.get("default_trigger") or {}
        field = spec.get("config_field")
        source = spec.get("source", "SchedulerConfig")
        config_map = {
            "SchedulerConfig": SchedulerConfig,
            "TelegramConfig": TelegramConfig,
            "SystemUpdateConfig": SystemUpdateConfig,
        }
        config_obj = config_map.get(source, SchedulerConfig)
        raw = getattr(config_obj, field, None) if field else None

        if spec.get("type") == "system_update":
            trigger_type = (SystemUpdateConfig.AUTO_UPDATE_TRIGGER_TYPE or "interval").strip().lower()
            if trigger_type == "cron_daily":
                h, m = cls._parse_time_str(str(SystemUpdateConfig.AUTO_UPDATE_TIME or "04:00"))
                return {"type": TRIGGER_CRON_DAILY, "hour": h, "minute": m}
            try:
                hours = int(SystemUpdateConfig.AUTO_UPDATE_INTERVAL_HOURS or 24)
            except (TypeError, ValueError):
                hours = 24
            return {"type": TRIGGER_INTERVAL, "seconds": max(MIN_INTERVAL_SECONDS, hours * 3600)}

        if spec.get("type") == "cron_daily":
            h, m = cls._parse_time_str(str(raw or "00:00"))
            offset = int(spec.get("offset_minutes", 0))
            total = (h * 60 + m + offset) % (24 * 60)
            return {"type": TRIGGER_CRON_DAILY, "hour": total // 60, "minute": total % 60}

        if spec.get("type") == "interval":
            unit = spec.get("unit", "hours")
            try:
                value = int(raw or 1)
            except (TypeError, ValueError):
                value = 1
            multiplier = {"seconds": 1, "minutes": 60, "hours": 3600}.get(unit, 3600)
            seconds = max(MIN_INTERVAL_SECONDS, min(MAX_INTERVAL_SECONDS, value * multiplier))
            return {"type": TRIGGER_INTERVAL, "seconds": seconds}

        # 兜底：每 1 小时
        return {"type": TRIGGER_INTERVAL, "seconds": 3600}

    @classmethod
    async def _effective_trigger(cls, definition: dict) -> tuple[dict, bool]:
        """优先取 DB override，其次回退默认。返回 (spec, is_custom)。"""
        # 手动专属任务：不接受 cron/interval override
        if definition.get("manual_only"):
            return {"type": "manual"}, False
        override = await SchedulerScheduleOperate.get_override(definition["id"])
        if override and override.get("is_custom"):
            if override["type"] == TRIGGER_CRON_DAILY and override.get("hour") is not None:
                return (
                    {
                        "type": TRIGGER_CRON_DAILY,
                        "hour": int(override["hour"]),
                        "minute": int(override.get("minute") or 0),
                    },
                    True,
                )
            if override["type"] == TRIGGER_INTERVAL and override.get("seconds"):
                return (
                    {"type": TRIGGER_INTERVAL, "seconds": int(override["seconds"])},
                    True,
                )
        return cls._resolve_default_trigger(definition), False

    @staticmethod
    def _trigger_from_spec(spec: dict):
        """把内部 spec 转成 APScheduler 触发器对象。"""
        if spec["type"] == TRIGGER_CRON_DAILY:
            return CronTrigger(
                hour=int(spec["hour"]),
                minute=int(spec["minute"]),
                timezone=SchedulerConfig.TIMEZONE,
            )
        return IntervalTrigger(
            seconds=int(spec["seconds"]),
            timezone=SchedulerConfig.TIMEZONE,
        )

    @classmethod
    def _get_definition(cls, job_id: str) -> Optional[dict]:
        for d in cls.JOB_DEFINITIONS:
            if d["id"] == job_id:
                return d
        return None

    @staticmethod
    def _pid_alive(pid: int) -> bool:
        if pid <= 0:
            return False
        try:
            os.kill(pid, 0)
            return True
        except OSError:
            return False

    @staticmethod
    def _default_lock_path() -> Path:
        raw = os.getenv("TWILIGHT_SCHEDULER_LOCK_FILE")
        return Path(raw).expanduser() if raw else Path.cwd() / "db" / "scheduler.lock"

    @classmethod
    def external_scheduler_active(cls) -> bool:
        """是否已有其它进程持有 scheduler lock。"""
        lock_path = cls._lock_path or cls._default_lock_path()
        try:
            pid = int(lock_path.read_text(encoding="utf-8").strip() or "0")
        except (OSError, ValueError):
            return False
        return pid != os.getpid() and cls._pid_alive(pid)

    @classmethod
    async def start_singleton(cls) -> tuple[bool, str]:
        """带进程锁启动调度器，供 API 进程兜底自动启动使用。"""
        if cls._scheduler is not None and cls._scheduler.running:
            return True, "调度器已在当前进程运行"

        lock_path = cls._default_lock_path()
        lock_path.parent.mkdir(parents=True, exist_ok=True)
        if lock_path.exists():
            try:
                existing_pid = int(lock_path.read_text(encoding="utf-8").strip() or "0")
            except (OSError, ValueError):
                existing_pid = 0
            if existing_pid > 0 and existing_pid != os.getpid() and cls._pid_alive(existing_pid):
                return False, f"已有 Scheduler 进程运行 (PID={existing_pid})"
            try:
                lock_path.unlink()
            except OSError:
                pass

        try:
            lock_path.write_text(str(os.getpid()), encoding="utf-8")
            cls._lock_path = lock_path
        except OSError as exc:
            logger.warning(f"写入 Scheduler 锁文件失败: {exc}")

        await cls.start()
        return True, "调度器已启动"

    @classmethod
    def _config_files_mtime(cls) -> float:
        paths = [get_primary_config_path()]
        local_override = os.getenv("TWILIGHT_CONFIG_LOCAL_FILE")
        if local_override:
            paths.append(Path(local_override).expanduser())
        else:
            paths.append(get_primary_config_path().with_name("config.local.toml"))
        mtimes: list[float] = []
        for path in paths:
            try:
                mtimes.append(path.stat().st_mtime)
            except OSError:
                continue
        return max(mtimes) if mtimes else 0.0

    @classmethod
    async def _watch_config_changes(cls) -> None:
        """调度进程内热重载 config.toml，避免必须重启才能更新触发器。"""
        while True:
            await asyncio.sleep(10)
            try:
                current_mtime = cls._config_files_mtime()
                if cls._config_mtime is None:
                    cls._config_mtime = current_mtime
                    continue
                if current_mtime <= cls._config_mtime:
                    continue
                cls._config_mtime = current_mtime
                ok, message = await cls.reload_from_config()
                if ok:
                    logger.info("Scheduler 配置热重载完成: %s", message)
                else:
                    logger.warning("Scheduler 配置热重载跳过: %s", message)
            except asyncio.CancelledError:
                raise
            except Exception as exc:  # pragma: no cover
                logger.warning("Scheduler 配置热重载失败: %s", exc, exc_info=True)

    @classmethod
    def _ensure_config_watcher(cls) -> None:
        if cls._config_watch_task is not None and not cls._config_watch_task.done():
            return
        try:
            loop = asyncio.get_running_loop()
        except RuntimeError:
            return
        cls._config_mtime = cls._config_files_mtime()
        cls._config_watch_task = loop.create_task(cls._watch_config_changes())

    @classmethod
    async def reload_from_config(cls, *, reload_config: bool = True) -> tuple[bool, str]:
        """重新加载配置并重装调度任务。"""
        sched_loop = cls._scheduler_loop
        try:
            running_loop = asyncio.get_running_loop()
        except RuntimeError:
            running_loop = None
        if sched_loop is not None and sched_loop is not running_loop:
            fut = asyncio.run_coroutine_threadsafe(
                cls.reload_from_config(reload_config=reload_config),
                sched_loop,
            )
            try:
                return fut.result(timeout=10)
            except Exception as exc:
                return False, f"调度器热重载失败: {exc}"

        if reload_config:
            from src.config import reload_runtime_config

            reload_runtime_config()

        scheduler = cls._scheduler
        if scheduler is None or not scheduler.running:
            return False, "调度器未在当前进程运行"

        scheduler.remove_all_jobs()
        if SchedulerConfig.ENABLED:
            await cls._install_all_jobs()
            try:
                await cls.list_jobs()
            except Exception as exc:  # pragma: no cover
                logger.warning("重载后同步 scheduler 固定信息失败: %s", exc)
            return True, f"已重载 {len(scheduler.get_jobs())} 个定时任务"
        return True, "调度器全局开关已关闭，已移除所有定时任务"

    # ============== 手动触发入口（管理员 API 调用） ==============

    @classmethod
    def _resolve_job(cls, job_id: str) -> Optional[Callable[..., Awaitable[Any]]]:
        return cls._job_fn_map().get(job_id)

    @classmethod
    async def trigger_job(
        cls,
        job_id: str,
        *,
        params: Optional[dict] = None,
    ) -> tuple[bool, str, Optional[dict]]:
        """手动触发指定 job。运行在调度器所在事件循环上（如果可用），
        否则就在当前协程里 await（API 线程）。

        :param params: 手动触发时携带的参数，写进 ``RunContext.params``。
            每个 job 自行决定识别哪些键，不识别的字段被忽略。

        Returns:
            (ok, message, run_record) —— ok 仅表示触发成功，job 本身的结果在
            run_record 中（status / error）。
        """
        fn = cls._resolve_job(job_id)
        if fn is None:
            return False, f"未知任务: {job_id}", None
        if job_id in cls._running:
            return False, f"任务 {job_id} 正在执行中，请稍候", cls._last_runs.get(job_id)

        sched_loop = cls._scheduler_loop
        try:
            running_loop = asyncio.get_running_loop()
        except RuntimeError:
            running_loop = None

        coro_factory = lambda: cls._run_with_tracking(
            job_id,
            fn,
            trigger="manual",
            params=params,
        )

        if sched_loop is not None and sched_loop is not running_loop:
            # API 线程触发 → 把任务安排到调度器所在 loop，立即返回，不阻塞 API
            asyncio.run_coroutine_threadsafe(coro_factory(), sched_loop)
            return True, "已触发，正在后台执行", cls._last_runs.get(job_id, {"status": "running"})

        # 没有独立 scheduler loop：交给共享后台 loop，避免 asgiref/WsgiToAsgi
        # 在请求结束后销毁 per-request executor 导致孤儿任务崩溃。
        from src.core.background import submit_background

        submit_background(coro_factory())
        return True, "已触发", cls._last_runs.get(job_id, {"status": "running"})

    @classmethod
    async def list_jobs(cls) -> list[dict]:
        """返回 job 列表 + 计划时间 + 上次运行情况，供管理员前端展示。

        last_run 优先取数据库里的最后一条（重启后仍有效），
        正在运行中的 job 用内存 `_last_runs` 覆盖以拿到「未结束」状态。
        每个 job 同时返回 `trigger_spec`（结构化的 cron_daily/interval 描述）
        和 `is_custom`（是否启用了管理员手动覆盖）供前端编辑器使用。
        """
        sched = cls._scheduler
        scheduled_map: dict[str, object] = {}
        if sched is not None and sched.running:
            for j in sched.get_jobs():
                scheduled_map[j.id] = j
        external_running = not scheduled_map and cls.external_scheduler_active()

        items = []
        for definition in cls.JOB_DEFINITIONS:
            jid = definition["id"]
            scheduled = scheduled_map.get(jid)
            next_run = None
            schedule_str = None
            enabled = scheduled is not None
            if scheduled is not None:
                if getattr(scheduled, "next_run_time", None):
                    next_run = int(scheduled.next_run_time.timestamp())
                trigger = scheduled.trigger
                schedule_str = str(trigger) if trigger else None

            is_running = jid in cls._running
            last_run: Optional[dict]
            if is_running:
                last_run = cls._last_runs.get(jid)
            else:
                try:
                    last_run = await SchedulerRunOperate.get_last_run_summary(jid)
                except Exception as exc:  # pragma: no cover
                    logger.warning(f"读取 scheduler_run 失败: {exc}")
                    last_run = cls._last_runs.get(jid)

            trigger_spec, is_custom = await cls._effective_trigger(definition)
            default_spec = cls._resolve_default_trigger(definition)

            if not enabled and external_running and SchedulerConfig.ENABLED and not definition.get("manual_only"):
                try:
                    from src.services.telegram_membership import TelegramMembershipService

                    if jid != "enforce_group_membership" or TelegramMembershipService.enforcement_enabled():
                        trigger = cls._trigger_from_spec(trigger_spec)
                        tz = getattr(trigger, "timezone", None)
                        now = datetime.now(tz) if tz is not None else datetime.now()
                        fire_time = trigger.get_next_fire_time(None, now)
                        next_run = int(fire_time.timestamp()) if fire_time is not None else None
                        schedule_str = str(trigger)
                        enabled = True
                except Exception as exc:  # pragma: no cover
                    logger.debug("计算外部 Scheduler 下次执行时间失败 (%s): %s", jid, exc)

            try:
                persisted_info = await SchedulerScheduleOperate.upsert_job_info(
                    jid,
                    name=str(definition.get("name") or jid),
                    description=str(definition.get("description") or ""),
                    manual_only=bool(definition.get("manual_only")),
                    enabled=bool(enabled),
                    trigger_spec=trigger_spec,
                    default_trigger_spec=default_spec,
                    next_run_at=next_run,
                )
            except Exception as exc:  # pragma: no cover
                logger.warning("写入 scheduler 固定信息失败 (%s): %s", jid, exc)
                persisted_info = None

            items.append(
                {
                    **{k: v for k, v in definition.items() if k != "default_trigger"},
                    "manual_only": bool(definition.get("manual_only")),
                    "enabled": enabled,
                    "schedule": schedule_str,
                    "next_run_at": next_run,
                    "last_run": last_run,
                    "is_running": is_running,
                    "trigger_spec": trigger_spec,
                    "default_trigger_spec": default_spec,
                    "is_custom": is_custom,
                    "persisted_info": persisted_info,
                    "last_auto_run_at": (persisted_info or {}).get("last_auto_run_at"),
                    "last_manual_run_at": (persisted_info or {}).get("last_manual_run_at"),
                    "runtime_params": (
                        {
                            "days": int(RegisterConfig.AUTO_CLEANUP_NO_EMBY_DAYS),
                            "auto_enabled": bool(RegisterConfig.AUTO_CLEANUP_NO_EMBY),
                        }
                        if jid == "cleanup_no_emby"
                        else None
                    ),
                }
            )
        return items

    @classmethod
    async def get_job_history(cls, job_id: str, *, limit: int = 20) -> list[dict]:
        return await SchedulerRunOperate.get_history(job_id, limit=limit)

    @classmethod
    async def get_last_run_detail(cls, job_id: str) -> Optional[dict]:
        """完整的最近一次运行（含 logs）。"""
        if job_id in cls._running:
            return cls._last_runs.get(job_id)
        return await SchedulerRunOperate.get_last_run(job_id)

    @classmethod
    def get_scheduler(cls):
        if cls._scheduler is None:
            cls._scheduler = AsyncIOScheduler(timezone=SchedulerConfig.TIMEZONE)
        return cls._scheduler

    @staticmethod
    async def _run_bounded(items, limit: int, handler):
        """Run async item handlers with bounded concurrency for high-cardinality jobs."""
        sem = asyncio.Semaphore(max(1, min(int(limit or 1), 64)))

        async def _one(item):
            async with sem:
                return await handler(item)

        return await asyncio.gather(*(_one(item) for item in items), return_exceptions=True)

    @staticmethod
    async def check_expired_users(ctx: RunContext):
        """检查过期用户并禁用其 Emby 账号，保留系统账号登录能力。"""
        ctx.log("🔍 开始检查过期用户...")
        try:
            expired_users = await UserOperate.get_expired_users()
            ctx.summary["scanned"] = len(expired_users)
            ctx.summary["disabled"] = 0
            ctx.summary["failed"] = 0
            if not expired_users:
                ctx.log("✅ 没有需要处理的过期用户")
                return

            ctx.log(f"📋 发现 {len(expired_users)} 个过期用户")
            emby = get_emby_client()

            async def _disable_one(user):
                try:
                    if user.EMBYID:
                        await emby.set_user_enabled(user.EMBYID, False)
                    return True, user, None
                except Exception as e:
                    return False, user, e

            results = await SchedulerService._run_bounded(expired_users, 12, _disable_one)
            for result in results:
                if isinstance(result, Exception):
                    ctx.summary["failed"] += 1
                    ctx.log(f"  ❌ 禁用任务异常: {result}")
                    continue
                ok, user, err = result
                if ok:
                    ctx.summary["disabled"] += 1
                    ctx.log(f"  ⏹️ 已禁用 Emby: {user.USERNAME} (UID: {user.UID})")
                else:
                    ctx.summary["failed"] += 1
                    ctx.log(f"  ❌ 禁用失败: {user.USERNAME} - {err}")
            ctx.log(f"✅ 过期用户检查完成: 禁用 {ctx.summary['disabled']} 个, " f"失败 {ctx.summary['failed']} 个")
        except Exception as e:
            ctx.log(f"❌ 检查过期用户时发生错误: {e}")
            raise

    @staticmethod
    async def check_expiring_users(ctx: RunContext):
        """检查即将过期的用户（用于提醒）"""
        ctx.log("🔔 检查即将过期的用户...")
        try:
            expiring_users = await UserOperate.get_expiring_users(days=3)
            ctx.summary["scanned"] = len(expiring_users)
            if not expiring_users:
                ctx.log("✅ 没有即将过期的用户")
                return

            ctx.log(f"📋 发现 {len(expiring_users)} 个即将过期的用户:")
            current = timestamp()
            for user in expiring_users:
                remaining = user.EXPIRED_AT - current
                remaining_str = format_duration(remaining)
                ctx.log(f"  ⚠️ {user.USERNAME} (UID: {user.UID}) - {remaining_str}后过期")
        except Exception as e:
            ctx.log(f"❌ 检查即将过期用户时发生错误: {e}")
            raise

    @staticmethod
    async def cleanup_inactive_sessions(ctx: RunContext):
        """清理不活跃的会话"""
        ctx.log("🧹 清理不活跃会话...")
        try:
            emby = get_emby_client()
            sessions = await emby.get_sessions()
            active = len([s for s in sessions if s.is_active])
            total = len(sessions)
            ctx.summary["active"] = active
            ctx.summary["total"] = total
            ctx.log(f"📊 当前会话: {active} 活跃 / {total} 总计")
        except Exception as e:
            ctx.log(f"❌ 清理会话时发生错误: {e}")
            raise

    @staticmethod
    async def daily_stats(ctx: RunContext):
        """每日统计"""
        ctx.log("📊 生成每日统计...")
        try:
            from src.db.regcode import RegCodeOperate

            registered = await UserOperate.get_registered_users_count()
            active = await UserOperate.get_active_users_count()
            regcodes = await RegCodeOperate.get_active_regcodes_count()
            server_status = await EmbyService.get_server_status()

            ctx.summary.update(
                {
                    "registered": registered,
                    "user_limit": RegisterConfig.USER_LIMIT,
                    "active": active,
                    "available_regcodes": regcodes,
                    "emby_online": bool(server_status.get("online")),
                    "active_sessions": server_status.get("active_sessions", 0) if server_status.get("online") else 0,
                }
            )

            ctx.log("=" * 30)
            ctx.log(f"👥 注册用户: {registered} / {RegisterConfig.USER_LIMIT}")
            ctx.log(f"✅ 活跃用户: {active}")
            ctx.log(f"🎫 可用注册码: {regcodes}")
            ctx.log(f"📺 Emby 状态: {'在线' if server_status.get('online') else '离线'}")
            if server_status.get("online"):
                ctx.log(f"   活跃会话: {server_status.get('active_sessions', 0)}")
            ctx.log("=" * 30)
        except Exception as e:
            ctx.log(f"❌ 生成统计时发生错误: {e}")
            raise

    @staticmethod
    async def send_expiry_reminders(ctx: RunContext):
        """发送到期提醒"""
        from src.services.admin_service import ReminderService

        ctx.log("📧 发送到期提醒...")
        try:
            result = await ReminderService.send_expiry_reminders()
            sent = int(result.get("sent", 0)) if isinstance(result, dict) else 0
            ctx.summary["sent"] = sent
            ctx.log(f"✅ 到期提醒发送完成: {sent} 条")
        except Exception as e:
            ctx.log(f"❌ 发送到期提醒出错: {e}")
            raise

    @staticmethod
    async def emby_sync(ctx: RunContext):
        """定期同步 Emby 用户数据"""
        ctx.log("🔄 开始 Emby 用户数据同步...")
        try:
            success, failed, errors = await EmbyService.sync_all_users()
            ctx.summary["success"] = int(success or 0)
            ctx.summary["failed"] = int(failed or 0)
            ctx.log(f"✅ Emby 同步完成: 成功 {success}, 失败 {failed}")
            if errors:
                for e in errors[:10]:
                    ctx.log(f"  ⚠️ {e}")
        except Exception as e:
            ctx.log(f"❌ Emby 同步出错: {e}")
            raise

    @staticmethod
    async def enforce_group_membership(ctx: RunContext):
        """定时巡检：绑定了 TG 但已退出必需群组的用户 → 禁用本地账号 + 禁用 Emby。

        仅在 `TelegramConfig.REQUIRE_GROUP_MEMBERSHIP` 开启且配置了 `GROUP_ID` 时执行。
        管理员、白名单不会被本任务处理（在 SQL 层面就过滤掉了）。

        额外逻辑：
        - 对已禁用且仍绑定 Telegram 的用户做“重新入群”识别；
        - 默认仅记录到日志与 summary；开启 AUTO_ENABLE_REJOINED 后自动恢复未到期用户。
        """
        from src.config import TelegramConfig
        from src.services.telegram_membership import TelegramMembershipService

        if not TelegramMembershipService.enforcement_enabled():
            ctx.summary["enabled"] = False
            ctx.log("ℹ️ 群组成员巡检未启用")
            return

        ban_on_leave = bool(getattr(TelegramConfig, "BAN_ON_LEAVE", False))
        auto_enable_rejoined = bool(getattr(TelegramConfig, "AUTO_ENABLE_REJOINED", False))

        # Bot 未就绪时直接退出。继续往下跑会让 check_user_in_groups 对每个
        # 用户都返回 (True, []) —— 这会把所有用户误判为「仍在群」，并产生
        # 一段误导的"815 仍在群、0 已禁用"日志（其实根本没真正检查）。
        if not TelegramMembershipService.is_bot_available():
            ctx.summary["enabled"] = True
            ctx.summary["bot_unavailable"] = True
            ctx.summary["scanned"] = 0
            ctx.log("⚠️ Bot 未就绪，无法发起群组成员检查；本次跳过，等待 Bot 初始化后下次再跑")
            return

        ctx.summary["enabled"] = True
        ctx.summary["ban_on_leave"] = ban_on_leave
        ctx.summary["auto_enable_rejoined"] = auto_enable_rejoined
        if ban_on_leave:
            ctx.log("⚠️ 退群完全封禁模式已开启：检测到退群的用户将被永久 ban，且不再做重新入群识别")
        elif auto_enable_rejoined:
            ctx.log("↩️ 回群自动启用已开启：重新入群且未到期的用户会被自动恢复")
        ctx.log("🛂 开始群组成员资格巡检...")
        try:
            users = await UserOperate.get_active_telegram_bound_users()
            ctx.summary["scanned"] = len(users)
            ctx.summary["in_group"] = 0
            ctx.summary["disabled"] = 0
            ctx.summary["failed"] = 0
            ctx.summary["permanently_banned"] = 0
            ctx.summary["ban_failed"] = 0
            ctx.summary["rejoin_scanned"] = 0
            ctx.summary["rejoin_candidates"] = 0
            ctx.summary["rejoin_uids"] = []
            ctx.summary["rejoin_expired_skipped"] = 0
            ctx.summary["rejoin_auto_enabled"] = 0
            ctx.summary["rejoin_auto_failed"] = 0
            if not users:
                ctx.log("✅ 没有需要检查的用户")
            else:
                tg_ids = []
                for u in users:
                    try:
                        tg_ids.append(int(u.TELEGRAM_ID))
                    except (TypeError, ValueError):
                        ctx.summary["failed"] += 1
                        ctx.log(f"  ⚠️ 跳过非法 Telegram ID: {u.USERNAME} (UID: {u.UID})")

                # 以系统内绑定用户为基准做批量对照，不扫描群全量成员列表。
                missing_map = await TelegramMembershipService.check_users_in_groups(tg_ids, strict=False)

                try:
                    action_concurrency = int(getattr(TelegramConfig, "GROUP_ACTION_CONCURRENCY", 8) or 8)
                except (TypeError, ValueError):
                    action_concurrency = 8
                action_sem = asyncio.Semaphore(max(1, min(action_concurrency, 24)))

                async def _handle_missing_user(u):
                    async with action_sem:
                        try:
                            user_tg_id = int(u.TELEGRAM_ID)
                        except (TypeError, ValueError):
                            return {"kind": "skip"}

                        missing = missing_map.get(user_tg_id, [])
                        if not missing:
                            return {"kind": "in_group"}

                        item = {
                            "kind": "missing",
                            "username": u.USERNAME,
                            "uid": u.UID,
                            "telegram_id": user_tg_id,
                            "missing": [m.id for m in missing],
                            "disabled": False,
                            "disable_error": None,
                            "banned": 0,
                            "ban_failed": 0,
                            "ban_failed_ids": [],
                        }
                        try:
                            success, msg = await UserService.disable_user(u, reason="未加入必需 Telegram 群组")
                            item["disabled"] = bool(success)
                            item["disable_error"] = None if success else msg
                        except Exception as exc:  # pragma: no cover
                            item["disable_error"] = str(exc)

                        if ban_on_leave:
                            try:
                                ban_result = await TelegramMembershipService.ban_user_permanently(
                                    user_tg_id,
                                    reason="leave_required_group",
                                )
                                banned_ids = ban_result.get("banned_groups") or []
                                failed_groups = ban_result.get("failed_groups") or []
                                item["banned"] = 1 if banned_ids else 0
                                item["ban_failed"] = 1 if failed_groups else 0
                                item["ban_failed_ids"] = [fg.get("id", "?") for fg in failed_groups]
                            except Exception as ban_exc:  # pragma: no cover
                                item["ban_failed"] = 1
                                item["ban_failed_ids"] = [str(ban_exc)]
                        return item

                action_results = await asyncio.gather(*(_handle_missing_user(u) for u in users))
                for item in action_results:
                    if item.get("kind") == "in_group":
                        ctx.summary["in_group"] += 1
                        continue
                    if item.get("kind") != "missing":
                        continue

                    if item.get("disabled"):
                        ctx.summary["disabled"] += 1
                        ctx.log(
                            f"  ⏹️ 已禁用 {item['username']} (UID: {item['uid']}, "
                            f"TG: {item['telegram_id']}) — 缺失群组: "
                            f"{', '.join(item.get('missing') or []) or '未知'}"
                        )
                    else:
                        ctx.summary["failed"] += 1
                        ctx.log(f"  ⚠️ 禁用 {item['username']} 失败: {item.get('disable_error') or '未知错误'}")

                    if ban_on_leave:
                        if item.get("banned"):
                            ctx.summary["permanently_banned"] += 1
                            ctx.log(f"    🚫 已永久封禁 (TG: {item['telegram_id']})")
                        if item.get("ban_failed"):
                            ctx.summary["ban_failed"] += 1
                            ctx.log(
                                f"    ⚠️ 永封部分群失败 (TG: {item['telegram_id']}): "
                                f"{', '.join(item.get('ban_failed_ids') or [])}"
                            )

            # 已禁用账号重新入群识别：只上报管理员，不自动恢复，避免误恢复风控账号。
            # 退群完全封禁模式下用户被永封根本进不来群里，本分支永远拿不到结果，
            # 跑一遍只是浪费 RTT，直接跳过。
            if ban_on_leave:
                ctx.summary["rejoin_skipped_due_to_ban_mode"] = True
                ctx.log("ℹ️ 永封模式已开启，跳过重新入群识别")
                inactive_users = []
            else:
                inactive_users = await UserOperate.get_inactive_telegram_bound_users()
            ctx.summary["rejoin_scanned"] = len(inactive_users)
            if inactive_users:
                inactive_tg_ids: list[int] = []
                for u in inactive_users:
                    try:
                        inactive_tg_ids.append(int(u.TELEGRAM_ID))
                    except (TypeError, ValueError):
                        continue

                inactive_missing_map = await TelegramMembershipService.check_users_in_groups(
                    inactive_tg_ids,
                    strict=False,
                )

                rejoin_candidates: list[dict[str, object]] = []
                now_ts = timestamp()
                for u in inactive_users:
                    try:
                        tg_id = int(u.TELEGRAM_ID)
                    except (TypeError, ValueError):
                        continue

                    if inactive_missing_map.get(tg_id):
                        continue

                    exp_raw = getattr(u, "EXPIRED_AT", None)
                    if exp_raw not in (None, -1, 0, "-1", "0"):
                        try:
                            exp_ts = int(exp_raw)
                        except (TypeError, ValueError):
                            exp_ts = 0
                        if exp_ts > 0 and exp_ts < now_ts:
                            ctx.summary["rejoin_expired_skipped"] += 1
                            continue

                    rejoin_candidates.append(
                        {
                            "uid": int(u.UID),
                            "username": u.USERNAME,
                            "telegram_id": tg_id,
                        }
                    )

                if rejoin_candidates:
                    ctx.summary["rejoin_candidates"] = len(rejoin_candidates)
                    ctx.summary["rejoin_uids"] = [item["uid"] for item in rejoin_candidates[:50]]
                    if auto_enable_rejoined:
                        for item in rejoin_candidates:
                            target = await UserOperate.get_user_by_uid(int(item["uid"]))
                            if not target:
                                ctx.summary["rejoin_auto_failed"] += 1
                                continue
                            ok, msg = await UserService.enable_user(target)
                            if ok:
                                ctx.summary["rejoin_auto_enabled"] += 1
                            else:
                                ctx.summary["rejoin_auto_failed"] += 1
                                ctx.log(f"    ⚠️ 自动启用失败: {item['username']} (UID: {item['uid']}) - {msg}")
                        ctx.log(
                            f"  ✅ 发现 {len(rejoin_candidates)} 个回群用户，"
                            f"已自动启用 {ctx.summary['rejoin_auto_enabled']} 个"
                        )
                    else:
                        ctx.log(
                            f"  ℹ️ 发现 {len(rejoin_candidates)} 个已禁用但重新入群用户，"
                            "可在定时任务页面一键重新校验并启用"
                        )
                    for item in rejoin_candidates[:20]:
                        ctx.log(
                            f"    ↩️ 候选恢复: {item['username']} " f"(UID: {item['uid']}, TG: {item['telegram_id']})"
                        )
                    if len(rejoin_candidates) > 20:
                        ctx.log(f"    ... 其余 {len(rejoin_candidates) - 20} 个请查看 summary.rejoin_uids")

            extra_summary = ""
            if ban_on_leave:
                ban_failed_count = ctx.summary["ban_failed"]
                extra_summary = f", 永封 {ctx.summary['permanently_banned']} 个"
                if ban_failed_count:
                    extra_summary += f", 永封失败 {ban_failed_count} 个"
            rejoin_tail = (
                f"自动恢复 {ctx.summary['rejoin_auto_enabled']} 个, 自动恢复失败 {ctx.summary['rejoin_auto_failed']} 个"
                if auto_enable_rejoined and not ban_on_leave
                else f"待人工复核恢复 {ctx.summary['rejoin_candidates']} 个"
            )
            ctx.log(
                f"✅ 群组成员资格巡检完成: 仍在群 {ctx.summary['in_group']} 个, "
                f"已禁用 {ctx.summary['disabled']} 个, 失败 {ctx.summary['failed']} 个, "
                f"{rejoin_tail}{extra_summary}"
            )
        except Exception as exc:
            ctx.log(f"❌ 群组成员资格巡检异常: {exc}")
            raise

    @staticmethod
    async def kick_unknown_group_members(ctx: RunContext):
        """手动专属：参照 Sakura_EmbyBoss 的"踢出群里没有 Emby 账号的人"语义。

        以群组花名册（Bot 被动观察到的群成员）为基准，逐个反查 ``users`` 表：

        - 没绑系统账号（含 UNRECOGNIZED）→ kick (``no_account``)
        - 系统账号被禁用 → kick (``disabled``)
        - 系统账号未绑 Emby（含 PENDING_EMBY） → kick (``no_emby``)
        - 系统账号正常 + 已绑 Emby → 留人

        群管理员 / TelegramConfig.ADMIN_ID / ADMIN / WHITE_LIST 一律排除。
        踢出策略沿用 ``ban + unban``（临时移除，被踢者将来仍可重新加入）。

        受 Bot API 限制，无法主动枚举从未被 chat_member 事件/消息观察过的成员；
        roster 是长期累积的最佳近似。
        """
        from src.services.telegram_membership import TelegramMembershipService

        ctx.summary["enabled"] = False

        if not TelegramMembershipService.is_bot_available():
            ctx.log("⚠️ Bot 未就绪或 Telegram 未启用，跳过")
            return
        group_ids = TelegramMembershipService.required_group_ids()
        if not group_ids:
            ctx.log("⚠️ 未配置 TelegramConfig.GROUP_ID，跳过")
            return
        chat_id = group_ids[0]
        ctx.summary["enabled"] = True
        ctx.summary["chat_id"] = str(chat_id)

        # 手动参数：dry_run / max_per_run
        params = getattr(ctx, "params", {}) or {}
        dry_run = bool(params.get("dry_run", False))
        try:
            max_per_run = int(params.get("max_per_run", 200))
        except (TypeError, ValueError):
            max_per_run = 200
        max_per_run = max(1, min(max_per_run, 500))

        ctx.log("📋 构建踢人计划（按花名册反查系统账号）...")
        plan = await TelegramMembershipService.build_unbound_kick_plan(chat_id)
        reasons: dict[int, str] = plan["reasons"]  # type: ignore[assignment]
        targets: list[int] = plan["targets"]  # type: ignore[assignment]
        excluded_ids: set[int] = plan["excluded_ids"]  # type: ignore[assignment]

        reason_counts = {"no_account": 0, "no_emby": 0, "disabled": 0}
        for r in reasons.values():
            reason_counts[r] = reason_counts.get(r, 0) + 1

        ctx.summary["roster_size"] = plan["roster_size"]
        ctx.summary["bots_in_roster"] = plan["bots_in_roster"]
        ctx.summary["preserved_bound"] = plan["preserved_bound"]
        ctx.summary["admins_excluded"] = len(plan["group_admin_ids"])  # type: ignore[arg-type]
        ctx.summary["excluded_total"] = len(excluded_ids)
        ctx.summary["targets"] = len(targets)
        ctx.summary["reason_no_account"] = reason_counts["no_account"]
        ctx.summary["reason_no_emby"] = reason_counts["no_emby"]
        ctx.summary["reason_disabled"] = reason_counts["disabled"]
        ctx.summary["dry_run"] = dry_run

        if not targets:
            ctx.log("✅ 没有可处理的候选 TG ID（roster 中的成员全部已绑 Emby/管理员）")
            return

        ctx.log(
            f"🛠️ 待清理 {len(targets)} 个 TG ID — "
            f"无账号 {reason_counts['no_account']} · 无 Emby {reason_counts['no_emby']} · "
            f"已禁用 {reason_counts['disabled']}（max_per_run={max_per_run}）"
        )

        if dry_run:
            ctx.log("ℹ️ dry_run=true，跳过实际踢人")
            ctx.summary["kicked"] = 0
            ctx.summary["skipped"] = 0
            ctx.summary["failed"] = 0
            ctx.summary["not_in_group"] = 0
            ctx.summary["scanned"] = 0
            return

        result = await TelegramMembershipService.kick_unknown_members(
            chat_id,
            list(targets),
            excluded_ids=set(excluded_ids),
            max_per_run=max_per_run,
        )

        for key in ("scanned", "kicked", "skipped", "failed", "not_in_group"):
            ctx.summary[key] = int(result.get(key, 0) or 0)

        details = result.get("details") or []
        for item in details[:20]:
            ctx.log(f"  ℹ️ {item}")

        ctx.log(
            "✅ 清理完成: kicked={kicked} skipped={skipped} "
            "not_in_group={not_in_group} failed={failed}".format(**ctx.summary)
        )

    @staticmethod
    async def cleanup_no_emby_users(ctx: RunContext):
        """清理注册后长期未创建 Emby 账户的用户。

        触发参数（仅手动触发时识别）：
        - ``days``：覆盖 ``AUTO_CLEANUP_NO_EMBY_DAYS``，最少 1，最多 3650。
        - ``preserve_tg_bound``：True 时跳过已绑 TG 但还没补建 Emby 的"半完成"账号
          （默认沿用 ``EMBY_DIRECT_REGISTER_ENABLED``：开启时认为他们随时可自助补建，
          先保留；关闭时按老规矩一并清理）。
        - ``ignore_enabled_flag``：True 时即便 ``AUTO_CLEANUP_NO_EMBY=False`` 也允许
          手动跑一次（避免管理员临时清扫还得改 config）。
        """
        from src.services.emby_register_queue import EmbyRegisterQueueService

        params = getattr(ctx, "params", {}) or {}
        manual_days = params.get("days")
        ignore_enabled_flag = bool(params.get("ignore_enabled_flag", False))
        preserve_tg_bound_param = params.get("preserve_tg_bound")

        # AUTO_CLEANUP_NO_EMBY 是给定时调度看的；手动触发时管理员可显式 override
        if not RegisterConfig.AUTO_CLEANUP_NO_EMBY and not (ctx.trigger == "manual" and ignore_enabled_flag):
            ctx.summary["enabled"] = False
            ctx.log("ℹ️ AUTO_CLEANUP_NO_EMBY 未启用，跳过（手动触发可传 ignore_enabled_flag=true 强制执行）")
            return

        # 解析 days：手动 > 配置
        days = RegisterConfig.AUTO_CLEANUP_NO_EMBY_DAYS
        if manual_days is not None:
            try:
                days = int(manual_days)
            except (TypeError, ValueError):
                ctx.log(f"⚠️ 忽略非法 days 参数: {manual_days!r}，回退到配置默认 {days}")
                days = RegisterConfig.AUTO_CLEANUP_NO_EMBY_DAYS
        days = max(1, min(3650, int(days)))

        if preserve_tg_bound_param is None:
            preserve_tg_bound = bool(RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED)
        else:
            preserve_tg_bound = bool(preserve_tg_bound_param)

        # 把队列里还在补建 Emby 的 UID 摘出来，避免临界窗口误删
        try:
            pending_uids = EmbyRegisterQueueService.pending_uids()
        except Exception as exc:  # pragma: no cover - 队列没启动也不应阻塞清理
            ctx.log(f"⚠️ 读取 Emby 注册队列状态失败，按「无在飞」处理: {exc}")
            pending_uids = set()

        ctx.summary["enabled"] = True
        ctx.summary["days_threshold"] = days
        ctx.summary["preserve_tg_bound"] = preserve_tg_bound
        ctx.summary["pending_register_excluded"] = len(pending_uids)
        ctx.log(
            f"🧹 开始清理注册超过 {days} 天无 Emby 账户的用户"
            f"（保留TG绑定: {preserve_tg_bound}, 在飞注册排除: {len(pending_uids)}）..."
        )
        try:
            users = await UserOperate.get_no_emby_users(
                days,
                exclude_uids=pending_uids or None,
                preserve_tg_bound=preserve_tg_bound,
            )
            ctx.summary["scanned"] = len(users)
            ctx.summary["deleted"] = 0
            ctx.summary["failed"] = 0
            if not users:
                ctx.log("✅ 没有需要清理的无 Emby 账户用户")
                return

            for user in users:
                try:
                    success, msg = await UserService.delete_user(user, delete_emby=False)
                    if success:
                        ctx.summary["deleted"] += 1
                        ctx.log(f"  🗑️ 已删除: {user.USERNAME} (UID: {user.UID})")
                    else:
                        ctx.summary["failed"] += 1
                        ctx.log(f"  ⚠️ 删除失败: {user.USERNAME} - {msg}")
                except Exception as e:
                    ctx.summary["failed"] += 1
                    ctx.log(f"  ❌ 删除失败: {user.USERNAME} - {e}")
            ctx.log(
                f"✅ 无 Emby 账户用户清理完成: 删除 {ctx.summary['deleted']} 个, " f"失败 {ctx.summary['failed']} 个"
            )
        except Exception as e:
            ctx.log(f"❌ 清理无 Emby 账户用户时发生错误: {e}")
            raise

    @staticmethod
    async def cleanup_unused_uploads(ctx: RunContext):
        """Clean user-uploaded image files that are no longer referenced."""
        ctx.log("🧹 开始清理未使用头像/背景图片...")
        try:
            from src.api.v1.users import cleanup_unused_upload_assets

            result = await cleanup_unused_upload_assets(max_age_seconds=24 * 3600)
            ctx.summary.update(result)
            ctx.log(
                "✅ 上传图片清理完成: 扫描 {scanned}, 删除 {deleted}, "
                "保留新文件 {skipped_recent}, 失败 {failed}".format(**result)
            )
        except Exception as e:
            ctx.log(f"❌ 清理上传图片时发生错误: {e}")
            raise

    @staticmethod
    async def check_telegram_bindings(ctx: RunContext):
        """检查 Telegram 与系统账号绑定状态一致性。"""
        from src.db.user import TelegramRebindRequestOperate

        ctx.log("🔎 开始检查 Telegram 绑定状态一致性...")
        users, total = await UserOperate.get_all_users(include_inactive=True, limit=100000, offset=0)
        tg_map: dict[int, list] = {}
        invalid = []
        for user in users:
            if user.TELEGRAM_ID is None:
                continue
            try:
                tg_id = int(user.TELEGRAM_ID)
            except (TypeError, ValueError):
                invalid.append(user)
                continue
            if tg_id <= 0:
                invalid.append(user)
                continue
            tg_map.setdefault(tg_id, []).append(user)

        duplicates = {tg_id: rows for tg_id, rows in tg_map.items() if len(rows) > 1}
        rebind_mismatch = []
        for user in users:
            try:
                req = await TelegramRebindRequestOperate.get_request_by_uid(user.UID)
            except Exception:
                req = None
            if not req:
                continue
            if req.STATUS == "pending" and not user.TELEGRAM_ID:
                rebind_mismatch.append((user, "pending_without_bound_tg"))
            if req.STATUS == "approved" and user.TELEGRAM_ID == req.OLD_TELEGRAM_ID:
                rebind_mismatch.append((user, "approved_but_old_tg_still_bound"))

        ctx.summary["users"] = int(total or 0)
        ctx.summary["telegram_bound"] = sum(len(v) for v in tg_map.values())
        ctx.summary["invalid_telegram_id"] = len(invalid)
        ctx.summary["duplicate_telegram_ids"] = len(duplicates)
        ctx.summary["rebind_state_mismatch"] = len(rebind_mismatch)

        for user in invalid[:20]:
            ctx.log(f"  ⚠️ 非法 TG ID: UID={user.UID} username={user.USERNAME} tg={user.TELEGRAM_ID}")
        for tg_id, rows in list(duplicates.items())[:20]:
            ctx.log("  ⚠️ TG ID 重复: %s -> %s" % (tg_id, ", ".join(f"{u.UID}/{u.USERNAME}" for u in rows)))
        for user, reason in rebind_mismatch[:20]:
            ctx.log(f"  ⚠️ 换绑状态不一致: UID={user.UID} username={user.USERNAME} reason={reason}")
        ctx.log("✅ Telegram 绑定状态一致性检查完成")

    @staticmethod
    async def system_auto_update(ctx: RunContext):
        """按配置执行 Git 自动更新。"""
        from src.services.system_update_service import apply_git_update

        if not SystemUpdateConfig.AUTO_UPDATE_ENABLED:
            ctx.log("系统自动更新未开启，跳过")
            ctx.summary["skipped"] = True
            return

        ctx.log(f"开始自动更新: repo={SystemUpdateConfig.REPO_URL} branch={SystemUpdateConfig.BRANCH}")
        result = await asyncio.to_thread(
            apply_git_update,
            SystemUpdateConfig.REPO_URL,
            SystemUpdateConfig.BRANCH,
            restart_services=SystemUpdateConfig.RESTART_SERVICES,
        )
        ctx.summary.update(
            {
                "success": bool(result.get("success")),
                "message": result.get("message"),
                "restart_scheduled": bool(result.get("restart_scheduled")),
                "commands": len(result.get("results") or []),
            }
        )
        for item in result.get("results") or []:
            ctx.log(f"$ {item.get('command')} -> exit={item.get('returncode')} ({item.get('duration_ms')}ms)")
        if not result.get("success"):
            raise RuntimeError(result.get("message") or "系统自动更新失败")
        ctx.log(result.get("message") or "系统自动更新完成")

    @classmethod
    async def start(cls):
        """启动调度器"""
        scheduler = cls.get_scheduler()
        if scheduler.running:
            cls._ensure_config_watcher()
            logger.info("ℹ️ 调度器已在运行，跳过重复启动")
            return

        # 进程上一次崩溃前的「running」状态行先回写为 failed，避免前端永远转圈
        try:
            reconciled = await SchedulerRunOperate.reconcile_orphans()
            if reconciled:
                logger.info(f"已将 {reconciled} 条残留运行中记录标记为失败")
        except Exception as exc:  # pragma: no cover
            logger.warning(f"reconcile orphans 失败: {exc}")

        try:
            cls._scheduler_loop = asyncio.get_running_loop()
        except RuntimeError:
            cls._scheduler_loop = None

        if SchedulerConfig.ENABLED:
            await cls._install_all_jobs()
        else:
            logger.info("ℹ️ 调度器全局开关已关闭：保持空调度器运行，用于监听配置热重载")
        scheduler.start()
        cls._ensure_config_watcher()
        try:
            await cls.list_jobs()
        except Exception as exc:  # pragma: no cover
            logger.warning("同步 scheduler 固定信息到数据库失败: %s", exc)

        logger.info("=" * 50)
        logger.info(f"🌙 Twilight Scheduler 已启动 ({SchedulerConfig.TIMEZONE})")
        for j in scheduler.get_jobs():
            logger.info(f"  - {j.id}: {j.trigger}")
        logger.info("=" * 50)

        # 立即运行一次统计（走 tracking 包装，复用同一份 ctx/落库逻辑）
        if SchedulerConfig.ENABLED:
            await cls._run_with_tracking("daily_stats", cls.daily_stats, trigger="startup")

    @classmethod
    async def _install_all_jobs(cls) -> None:
        """把 JOB_DEFINITIONS 里所有 job 注册（或重新注册）到 APScheduler。

        每个 job 的实际触发器 = DB override（如有）else 默认（解析自 config）。
        `enforce_group_membership` 只在 `REQUIRE_GROUP_MEMBERSHIP` + 群组配置齐备
        时才注册；管理员后续通过 UI 改 schedule 也只有在功能开启时才会生效。
        """
        from src.services.telegram_membership import TelegramMembershipService

        scheduler = cls.get_scheduler()
        fn_map = cls._job_fn_map()

        for definition in cls.JOB_DEFINITIONS:
            jid = definition["id"]
            if definition.get("manual_only"):
                # 手动专属任务不进入 APScheduler；trigger_job 直接拉起
                continue
            if jid == "enforce_group_membership" and not TelegramMembershipService.enforcement_enabled():
                continue
            if jid == "system_auto_update" and not SystemUpdateConfig.AUTO_UPDATE_ENABLED:
                continue
            spec, _custom = await cls._effective_trigger(definition)
            scheduler.add_job(
                cls._make_scheduled(jid, fn_map[jid]),
                trigger=cls._trigger_from_spec(spec),
                id=jid,
                replace_existing=True,
            )

    @classmethod
    def _job_fn_map(cls) -> dict[str, Callable[..., Awaitable[Any]]]:
        return {
            "check_expired": cls.check_expired_users,
            "check_expiring": cls.check_expiring_users,
            "expiry_reminders": cls.send_expiry_reminders,
            "daily_stats": cls.daily_stats,
            "cleanup_sessions": cls.cleanup_inactive_sessions,
            "emby_sync": cls.emby_sync,
            "cleanup_no_emby": cls.cleanup_no_emby_users,
            "cleanup_unused_uploads": cls.cleanup_unused_uploads,
            "check_telegram_bindings": cls.check_telegram_bindings,
            "system_auto_update": cls.system_auto_update,
            "enforce_group_membership": cls.enforce_group_membership,
            "kick_unknown_group_members": cls.kick_unknown_group_members,
        }

    # ============== 管理 API：在线修改 / 重置触发器 ==============

    @classmethod
    async def set_job_schedule(
        cls,
        job_id: str,
        *,
        trigger_type: str,
        hour: Optional[int] = None,
        minute: Optional[int] = None,
        seconds: Optional[int] = None,
    ) -> tuple[bool, str, Optional[dict]]:
        """落库覆盖 + 实时 reschedule。返回 (ok, message, effective_spec)。"""
        definition = cls._get_definition(job_id)
        if not definition:
            return False, f"未知任务: {job_id}", None
        if definition.get("manual_only"):
            return False, "该任务仅支持手动触发，不能配置定时触发器", None

        try:
            override = await SchedulerScheduleOperate.upsert_override(
                job_id,
                trigger_type=trigger_type,
                hour=hour,
                minute=minute,
                seconds=seconds,
            )
        except ValueError as exc:
            return False, str(exc), None

        spec, _custom = await cls._effective_trigger(definition)
        ok, msg = await cls._apply_trigger(job_id, spec)
        if not ok:
            return False, msg, spec
        return True, "已更新", spec

    @classmethod
    async def reset_job_schedule(cls, job_id: str) -> tuple[bool, str, Optional[dict]]:
        """清除覆盖，恢复到 config.toml 默认值。"""
        definition = cls._get_definition(job_id)
        if not definition:
            return False, f"未知任务: {job_id}", None
        if definition.get("manual_only"):
            return False, "该任务仅支持手动触发，无可恢复的默认触发器", None

        await SchedulerScheduleOperate.delete_override(job_id)
        spec = cls._resolve_default_trigger(definition)
        ok, msg = await cls._apply_trigger(job_id, spec)
        if not ok:
            return False, msg, spec
        return True, "已恢复默认", spec

    @classmethod
    async def _apply_trigger(cls, job_id: str, spec: dict) -> tuple[bool, str]:
        """在调度器所在 loop 上 reschedule。如果 job 尚未注册（例如群组功能未启用），
        只更新 DB 覆盖，不报错——下次满足启用条件时会读到正确值。
        """
        scheduler = cls.get_scheduler()
        if not scheduler.running:
            return True, "调度器未启动，已落库待生效"

        def _do():
            try:
                if scheduler.get_job(job_id):
                    scheduler.reschedule_job(job_id, trigger=cls._trigger_from_spec(spec))
                else:
                    # 任务从未注册（如 enforce_group_membership 未启用）；安静返回
                    pass
            except Exception as exc:  # pragma: no cover - APScheduler 抛错时由外层捕获
                raise RuntimeError(str(exc))

        sched_loop = cls._scheduler_loop
        try:
            running_loop = asyncio.get_running_loop()
        except RuntimeError:
            running_loop = None

        try:
            if sched_loop is not None and sched_loop is not running_loop:
                fut = asyncio.run_coroutine_threadsafe(
                    asyncio.to_thread(_do),
                    sched_loop,
                )
                # reschedule 是个轻量同步调用，等一会拿结果即可
                fut.result(timeout=5)
            else:
                _do()
        except Exception as exc:
            return False, f"reschedule 失败: {exc}"
        return True, "已应用"

    @classmethod
    async def stop(cls):
        """停止调度器"""
        if cls._config_watch_task is not None:
            cls._config_watch_task.cancel()
            cls._config_watch_task = None
        if cls._scheduler and cls._scheduler.running:
            cls._scheduler.shutdown()
            logger.info("👋 调度器已关闭")
        if cls._lock_path is not None:
            try:
                if cls._lock_path.exists() and cls._lock_path.read_text(encoding="utf-8").strip() == str(os.getpid()):
                    cls._lock_path.unlink()
            except OSError:
                pass
            cls._lock_path = None
