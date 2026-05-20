"""
Emby 自由注册队列服务

目的：
    `/users/me/emby/register` 在高并发下会同时打 Emby create_user，既容易撞 Emby 的限流，
    又会让我们这边的"已绑用户上限"出现 race condition（一堆请求都看到 `bound < limit`
    然后一起入库越线）。这里把它收敛成一个串行入队 + N 个 worker 的模型：

    - 任务都跑在 ``src.core.background`` 提供的长生命周期事件循环上，避免 WsgiToAsgi
      per-request loop 销毁带来的 "CurrentThreadExecutor already quit" 坑。
    - 入队前在 ``_state_lock`` 内做 *capacity check*，把队列里 in-flight 的人头算进
      `EMBY_USER_LIMIT` 占用里，保证总数不会越线。
    - Worker 调用 ``UserService.complete_emby_registration``，这是已登录用户补建 Emby
      的统一入口（``register_direct_emby`` 这条历史路径已无人调用，删掉）。
    - API 端可以选择 ``enqueue_and_wait``（同步等结果，超时则返回 queued/processing 状态
      让前端继续轮询 ``/users/register/emby/status``）。

不解决的事：
    - 跨进程并发：本队列是 *单进程内* 的串行器，多 uvicorn worker 之间没有协调；
      如果你真的开了多 worker，``EMBY_USER_LIMIT`` 的硬卡仍然要靠数据库层面（已绑用户
      总量 + ``UserService.check_emby_user_capacity``）兜底。这一层只是把单 worker 内的
      并发收敛。
"""

from __future__ import annotations

import asyncio
import logging
import secrets
import time
from dataclasses import dataclass
from typing import Any, Dict, Optional, Tuple

from src.config import RegisterConfig
from src.core.background import get_background_loop
from src.db.user import UserOperate, UserModel
from src.services.user_service import RegisterResult, UserService

logger = logging.getLogger(__name__)


@dataclass
class _QueueTask:
    """队列中的一条 Emby 补建任务。

    `uid` 指向 ``users.UID``；worker 取出后再加载最新 ``UserModel``，避免任务对象
    在队列里堆积时 cache 过时数据。完成信号在 ``EmbyRegisterQueueService._events``
    里按 ``request_id`` 索引；放队列对象里会让"队列已 dequeue 但 worker 还在跑"的
    阶段拿不到 Event，引发等不到完成的竞态。
    """

    request_id: str
    status_token: str
    uid: int
    emby_username: str
    emby_password: str
    created_at: int


class EmbyRegisterQueueService:
    """Emby 注册请求队列（单进程内）。"""

    # —— 全局状态都跑在 background loop 上，跨线程访问统一靠 run_coroutine_threadsafe ——
    _queue: Optional["asyncio.Queue[_QueueTask]"] = None
    _workers: list["asyncio.Task"] = []
    _started: bool = False
    _start_lock = asyncio.Lock()
    _state_lock = asyncio.Lock()

    # 状态表：request_id → 状态字典（暴露给前端轮询用）
    _status: Dict[str, Dict[str, Any]] = {}
    # 去重索引：同一个 uid / username 已在队列里时直接复用 request_id
    _pending_by_uid: Dict[int, str] = {}
    _pending_by_username: Dict[str, str] = {}
    # 完成信号：request_id → Event。worker 收尾时 set() + 删除；
    # 同步等待方在持锁时取出 Event 引用，再脱锁 await，不会因 dequeue 后被 GC 而丢失。
    _events: Dict[str, "asyncio.Event"] = {}

    # —— 配置读取 helpers（每次现读 RegisterConfig，方便 sweep + 重启后立即生效） ——

    @classmethod
    def _now(cls) -> int:
        return int(time.time())

    @classmethod
    def _status_ttl(cls) -> int:
        # Terminal registration status is intentionally short-lived: enough for a user
        # to close the dialog and come back, not long enough to retain sensitive state.
        return 15 * 60

    @classmethod
    def _worker_count(cls) -> int:
        configured = int(RegisterConfig.EMBY_DIRECT_REGISTER_WORKERS or 8)
        return min(max(configured, 1), 32)

    @classmethod
    def _queue_max_size(cls) -> int:
        configured = int(RegisterConfig.EMBY_DIRECT_REGISTER_MAX_QUEUE or 1000)
        return min(max(configured, 10), 10000)

    # —— 启停 ——

    @classmethod
    async def _bootstrap_on_bg_loop(cls) -> None:
        """在 background loop 上初始化 queue + worker。必须由 ``run_coroutine_threadsafe``
        投递过来执行，确保 queue / event / task 都绑定到同一个 loop。
        """
        if cls._started:
            return

        async with cls._start_lock:
            if cls._started:
                return

            cls._queue = asyncio.Queue(maxsize=cls._queue_max_size())
            worker_count = cls._worker_count()
            cls._workers = [
                asyncio.create_task(
                    cls._worker_loop(i + 1),
                    name=f"emby-register-worker-{i + 1}",
                )
                for i in range(worker_count)
            ]
            cls._started = True
            logger.info(
                "Emby 注册队列已启动 workers=%s max_queue=%s",
                worker_count,
                cls._queue_max_size(),
            )

    @classmethod
    def ensure_started_sync(cls) -> None:
        """从任意线程/任意 loop 触发队列懒启动。

        历史上这是个 ``async def ensure_started()``，但实际调用方往往在 WSGI handler
        里 await，结果 queue/Event 绑到了一个 per-request 的临时 loop 上，请求结束后
        loop 被销毁就炸了。改成在 background loop 上做 bootstrap，所有内部 state 都
        统一在那一个 loop 上。
        """
        if cls._started:
            return
        loop = get_background_loop()
        fut = asyncio.run_coroutine_threadsafe(cls._bootstrap_on_bg_loop(), loop)
        # 5s 内必须完成 bootstrap；上不去说明 background loop 本身挂了，往上抛
        fut.result(timeout=5.0)

    # —— 状态视图 ——

    @classmethod
    def in_flight_count(cls) -> int:
        """当前队列 + 处理中的请求数，给容量计算用。

        没启动 / 没排队就是 0；多 worker 一起在跑也只算一次（按 username key 去重）。
        """
        return len(cls._pending_by_username)

    @classmethod
    def pending_uids(cls) -> set[int]:
        """当前队列里"还没拿到 Emby 账号但快了"的 UID 集合。

        清理任务用它把这部分用户从 "注册超过 N 天还没 Emby" 列表里排除，
        避免在临界窗口（worker 已 dequeue 但还没把 EMBYID 落库）误删账号。
        """
        return {int(uid) for uid in cls._pending_by_uid.keys() if uid is not None}

    @classmethod
    async def _cleanup_expired_status_locked(cls) -> None:
        """清理过期的终态记录，防止 _status dict 无限膨胀。"""
        now = cls._now()
        ttl = cls._status_ttl()
        to_delete: list[str] = []
        for request_id, item in cls._status.items():
            status = item.get("status")
            updated_at = int(item.get("updated_at") or 0)
            if status in ("success", "failed", "rejected") and (now - updated_at) > ttl:
                to_delete.append(request_id)
        for request_id in to_delete:
            cls._status.pop(request_id, None)
            cls._events.pop(request_id, None)

    @classmethod
    def _queue_position_unlocked(cls, request_id: str) -> Optional[int]:
        """估算队列里的位置；只给前端展示，不参与决策。"""
        if cls._queue is None:
            return None
        # asyncio.Queue 的内部 deque 是 SLF001，仅用于只读展示
        queue_items = list(cls._queue._queue)  # noqa: SLF001
        for idx, task in enumerate(queue_items, start=1):
            if task.request_id == request_id:
                return idx
        return None

    # —— 主入口 ——

    @classmethod
    async def enqueue_complete_for_user(
        cls,
        user: UserModel,
        emby_username: str,
        emby_password: str,
    ) -> Tuple[Optional[Dict[str, Any]], str]:
        """把"已登录用户补建 Emby"任务投递给队列，立即返回 request_id + 当前状态。

        前置校验已经在更外层（API handler）做过 PENDING_EMBY=True / EMBYID 为空，这里
        只做队列相关的去重 + 容量门控。
        """
        if not RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED:
            return None, "Emby 自由注册未开启"

        cls.ensure_started_sync()
        if cls._queue is None:
            return None, "注册队列尚未就绪"

        emby_username = (emby_username or "").strip()
        if not emby_username:
            return None, "Emby 用户名不能为空"
        username_key = emby_username.lower()

        # 把入队走的整段都搬到 background loop 上：state_lock / queue / Event 都属于那个 loop
        loop = get_background_loop()
        coro = cls._enqueue_complete_locked(user, emby_username, emby_password, username_key)
        fut = asyncio.run_coroutine_threadsafe(coro, loop)
        try:
            payload, message = await asyncio.wrap_future(fut)
        except Exception as exc:  # pragma: no cover - 防御性
            logger.error("入队失败: %s", exc, exc_info=True)
            return None, "入队失败，请稍后重试"
        return payload, message

    @classmethod
    async def _enqueue_complete_locked(
        cls,
        user: UserModel,
        emby_username: str,
        emby_password: str,
        username_key: str,
    ) -> Tuple[Optional[Dict[str, Any]], str]:
        """跑在 background loop 上的入队逻辑。这里才是真正持锁的临界区。"""
        async with cls._state_lock:
            await cls._cleanup_expired_status_locked()

            # 1) 同一用户重复点：直接返回旧 request_id 让前端复用轮询
            existing_request_id = cls._pending_by_uid.get(int(user.UID))
            if existing_request_id:
                item = cls._status.get(existing_request_id)
                if item:
                    return {
                        "request_id": existing_request_id,
                        "status_token": item.get("status_token"),
                        "status": item.get("status"),
                        "queue_position": cls._queue_position_unlocked(existing_request_id),
                        "reused": True,
                    }, "您已有正在处理的 Emby 注册请求"

            # 2) 同名 Emby 用户名冲突：直接拒（防止两个本站用户抢同一个 Emby name）
            other_request_id = cls._pending_by_username.get(username_key)
            if other_request_id and other_request_id != existing_request_id:
                return None, "该 Emby 用户名正在被另一个请求注册，请换一个"

            # 3) 队列长度兜底：超过 maxsize 直接拒（避免内存爆炸）
            if cls._queue is None:
                return None, "注册队列尚未就绪"
            if cls._queue.qsize() >= cls._queue.maxsize:
                return None, "注册请求过多，请稍后再试"

            # 4) 容量门控：当前已绑用户 + 已经在队列/处理中的人头一起算
            #    这里持着 _state_lock 做一次性快照，避免和并发入队互相覆盖。
            pending_now = len(cls._pending_by_username)
            cap_ok, cap_msg = await UserService.check_emby_user_capacity(
                extra_pending=pending_now,
            )
            if not cap_ok:
                return None, cap_msg

            # 5) 构造任务并落表
            request_id = f"erq_{secrets.token_hex(8)}"
            status_token = secrets.token_urlsafe(20)
            now = cls._now()
            task = _QueueTask(
                request_id=request_id,
                status_token=status_token,
                uid=int(user.UID),
                emby_username=emby_username,
                emby_password=emby_password,
                created_at=now,
            )

            cls._pending_by_uid[int(user.UID)] = request_id
            cls._pending_by_username[username_key] = request_id
            cls._events[request_id] = asyncio.Event()
            cls._status[request_id] = {
                "request_id": request_id,
                "status_token": status_token,
                "status": "queued",
                "created_at": now,
                "updated_at": now,
                "uid": int(user.UID),
                "username": user.USERNAME,
                "emby_username": emby_username,
                "message": "已进入注册队列，等待处理",
                "queue_position": cls._queue.qsize() + 1,
            }

            cls._queue.put_nowait(task)

            return {
                "request_id": request_id,
                "status_token": status_token,
                "status": "queued",
                "queue_position": cls._status[request_id]["queue_position"],
                "reused": False,
            }, "已加入 Emby 注册队列"

    @classmethod
    async def enqueue_and_wait(
        cls,
        user: UserModel,
        emby_username: str,
        emby_password: str,
        *,
        timeout: float = 60.0,
    ) -> Dict[str, Any]:
        """投递任务并同步等待结果（API handler 用）。

        - 成功 / 失败 / 已存在 等终态：等到完成后返回最终的 status 项
        - 超时未结束：返回当前 status 项（状态可能是 queued/processing），前端继续轮询
        - 队列拒绝（容量满 / 上限 / 注册关闭）：返回 ``status='rejected'`` 的合成项

        Returns:
            { request_id, status_token, status, message, queue_position?, data?, ... }
        """
        payload, message = await cls.enqueue_complete_for_user(user, emby_username, emby_password)
        if payload is None:
            return {
                "status": "rejected",
                "message": message,
            }

        request_id = payload["request_id"]
        status_token = payload["status_token"]

        # 等待完成要绕到 background loop 上，把 Event.wait() 转 future 回到当前 loop
        loop = get_background_loop()

        async def _wait_done() -> Dict[str, Any]:
            return await cls._wait_for_completion(request_id, timeout)

        fut = asyncio.run_coroutine_threadsafe(_wait_done(), loop)
        try:
            result = await asyncio.wrap_future(fut)
        except Exception as exc:  # pragma: no cover
            logger.error("等待 Emby 注册队列任务异常 request_id=%s: %s", request_id, exc, exc_info=True)
            return {
                "request_id": request_id,
                "status_token": status_token,
                "status": "failed",
                "message": "等待 Emby 注册任务时出错，请稍后重试",
            }

        # _serialize_status_for_response 出于轮询接口安全已经把 status_token 抹掉了；
        # 这里是 API handler 内部使用，需要把 token 还回去，方便 handler 把它下发给前端继续轮询。
        result.setdefault("status_token", status_token)
        result.setdefault("request_id", request_id)
        return result

    @classmethod
    async def _wait_for_completion(cls, request_id: str, timeout: float) -> Dict[str, Any]:
        """跑在 background loop 上：等 Event 触发或超时。"""
        item = cls._status.get(request_id)
        if not item:
            return {
                "request_id": request_id,
                "status": "failed",
                "message": "注册请求不存在或已过期",
            }

        event = cls._events.get(request_id)
        if event is None:
            # 没有 Event 说明任务已经走完终态、被 worker 清理掉了；直接返回当前快照
            return cls._serialize_status_for_response(item)

        try:
            await asyncio.wait_for(event.wait(), timeout=timeout)
        except asyncio.TimeoutError:
            logger.info("Emby 注册队列等待超时 request_id=%s timeout=%.1fs", request_id, timeout)

        latest = cls._status.get(request_id) or item
        return cls._serialize_status_for_response(latest)

    @classmethod
    def _serialize_status_for_response(cls, item: Dict[str, Any]) -> Dict[str, Any]:
        out = dict(item)
        out.pop("status_token", None)
        return out

    # —— 状态查询 ——

    @classmethod
    async def get_status(cls, request_id: str, status_token: str) -> Optional[Dict[str, Any]]:
        """供 ``/users/register/emby/status`` 用：凭 token 拉状态。

        在 background loop 上读 state，保持 lock 语义一致。
        """
        loop = get_background_loop()
        fut = asyncio.run_coroutine_threadsafe(
            cls._get_status_locked(request_id, status_token),
            loop,
        )
        return await asyncio.wrap_future(fut)

    @classmethod
    async def _get_status_locked(cls, request_id: str, status_token: str) -> Optional[Dict[str, Any]]:
        async with cls._state_lock:
            await cls._cleanup_expired_status_locked()
            item = cls._status.get(request_id)
            if not item:
                return None
            if item.get("status_token") != status_token:
                return None

            result = cls._serialize_status_for_response(item)
            if result.get("status") == "queued":
                result["queue_position"] = cls._queue_position_unlocked(request_id)
            return result

    # —— Worker 内部 ——

    @classmethod
    async def _worker_loop(cls, worker_id: int) -> None:
        assert cls._queue is not None

        while True:
            task: _QueueTask = await cls._queue.get()
            try:
                await cls._mark_processing(task)

                fresh_user = await UserOperate.get_user_by_uid(task.uid)
                if fresh_user is None:
                    await cls._mark_failed(task, "用户记录不存在，可能已被删除", terminal=True)
                    continue

                if fresh_user.EMBYID:
                    await cls._mark_already_done(task, fresh_user)
                    continue

                if not getattr(fresh_user, "PENDING_EMBY", False):
                    await cls._mark_failed(
                        task,
                        "该账号不在待补建 Emby 状态，无法继续",
                        terminal=True,
                    )
                    continue

                result = await UserService.complete_emby_registration(
                    fresh_user,
                    task.emby_username,
                    task.emby_password,
                )

                if result.result == RegisterResult.SUCCESS:
                    await cls._mark_success(task, result)
                elif result.result in (
                    RegisterResult.USER_LIMIT_REACHED,
                    RegisterResult.EMBY_EXISTS,
                    RegisterResult.USER_EXISTS,
                ):
                    # 业务侧已经返回明确不可恢复的错误（容量满、用户名冲突等），按失败终态记
                    await cls._mark_failed(task, result.message, terminal=True)
                else:
                    await cls._mark_failed(task, result.message, terminal=True)
            except Exception as exc:  # pragma: no cover - worker 兜底
                logger.error("Emby 注册队列 worker=%s 处理失败: %s", worker_id, exc, exc_info=True)
                await cls._mark_failed(task, "创建 Emby 账户失败，请稍后重试", terminal=True)
            finally:
                cls._queue.task_done()

    @classmethod
    async def _mark_processing(cls, task: _QueueTask) -> None:
        async with cls._state_lock:
            item = cls._status.get(task.request_id)
            if not item:
                return
            item["status"] = "processing"
            item["updated_at"] = cls._now()
            item["message"] = "正在向 Emby 创建账号"
            item["queue_position"] = None

    @classmethod
    def _signal_completion(cls, request_id: str) -> None:
        """统一收尾：把 Event 触发（如果还在），让所有同步等待方解开。

        Event 留在 ``_events`` 里直到 ``_cleanup_expired_status_locked`` 清掉，
        否则 enqueue_and_wait 刚 set() 完别的协程才进来 ``_wait_for_completion``
        时拿不到 Event 就会返回快照——快照本身是正确的，但代码 path 走起来更绕。
        """
        evt = cls._events.get(request_id)
        if evt is not None:
            evt.set()

    @classmethod
    async def _mark_success(cls, task: _QueueTask, result: Any) -> None:
        async with cls._state_lock:
            now = cls._now()
            item = cls._status.get(task.request_id)
            if not item:
                return

            item["status"] = "success"
            item["updated_at"] = now
            item["finished_at"] = now
            item["message"] = result.message
            item["queue_position"] = None
            item["data"] = {
                "uid": result.user.UID if result.user else None,
                "username": result.user.USERNAME if result.user else None,
                "emby_username": task.emby_username,
                "emby_id": result.user.EMBYID if result.user else None,
            }

            cls._pending_by_uid.pop(task.uid, None)
            cls._pending_by_username.pop(task.emby_username.lower(), None)

        cls._signal_completion(task.request_id)

    @classmethod
    async def _mark_already_done(cls, task: _QueueTask, user: UserModel) -> None:
        """Worker 取出任务时发现用户已经绑好 Emby（前端可能重复触发了），按成功收尾。"""
        async with cls._state_lock:
            now = cls._now()
            item = cls._status.get(task.request_id)
            if not item:
                return
            item["status"] = "success"
            item["updated_at"] = now
            item["finished_at"] = now
            item["message"] = "Emby 账号已存在并已绑定"
            item["queue_position"] = None
            item["data"] = {
                "uid": int(user.UID),
                "username": user.USERNAME,
                "emby_username": task.emby_username,
                "emby_id": user.EMBYID,
            }
            cls._pending_by_uid.pop(task.uid, None)
            cls._pending_by_username.pop(task.emby_username.lower(), None)
        cls._signal_completion(task.request_id)

    @classmethod
    async def _mark_failed(cls, task: _QueueTask, message: str, *, terminal: bool = True) -> None:
        async with cls._state_lock:
            now = cls._now()
            item = cls._status.get(task.request_id)
            if not item:
                return

            item["status"] = "failed" if terminal else "queued"
            item["updated_at"] = now
            if terminal:
                item["finished_at"] = now
            item["message"] = message
            item["queue_position"] = None

            if terminal:
                cls._pending_by_uid.pop(task.uid, None)
                cls._pending_by_username.pop(task.emby_username.lower(), None)

        if terminal:
            cls._signal_completion(task.request_id)
