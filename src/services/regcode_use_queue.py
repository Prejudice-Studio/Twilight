"""Bounded background queue for user regcode usage.

The HTTP endpoint should enqueue quickly and return a polling token. This avoids
holding many request sockets open while regcode operations serialize on DB,
registration locks, and optional Emby API calls.
"""

from __future__ import annotations

import asyncio
import secrets
import time
from dataclasses import dataclass
from typing import Any, Dict, Optional

from src.db.user import UserOperate
from src.services.user_service import UserService


@dataclass
class _RegcodeUseTask:
    request_id: str
    status_token: str
    uid: int
    reg_code: str
    emby_username: Optional[str]
    emby_password: Optional[str]
    created_at: int


class RegcodeUseQueueService:
    """Small in-process queue for costly regcode use operations."""

    _queue: Optional[asyncio.Queue[_RegcodeUseTask]] = None
    _worker_task: Optional[asyncio.Task] = None
    _state_lock: Optional[asyncio.Lock] = None
    _status: Dict[str, Dict[str, Any]] = {}
    _pending_by_uid: Dict[int, str] = {}
    _max_queue_size = 200
    _status_ttl = 15 * 60

    @classmethod
    def _now(cls) -> int:
        return int(time.time())

    @classmethod
    def _ensure_started(cls) -> None:
        if cls._queue is None:
            cls._queue = asyncio.Queue(maxsize=cls._max_queue_size)
        if cls._state_lock is None:
            cls._state_lock = asyncio.Lock()
        if cls._worker_task is None or cls._worker_task.done():
            cls._worker_task = asyncio.create_task(cls._worker_loop())

    @classmethod
    async def enqueue(
        cls,
        *,
        uid: int,
        reg_code: str,
        emby_username: Optional[str],
        emby_password: Optional[str],
    ) -> tuple[Optional[Dict[str, Any]], str]:
        cls._ensure_started()
        assert cls._queue is not None
        assert cls._state_lock is not None

        async with cls._state_lock:
            cls._cleanup_expired_status_locked()
            existing_request_id = cls._pending_by_uid.get(int(uid))
            if existing_request_id:
                item = cls._status.get(existing_request_id)
                if item:
                    return {
                        "request_id": existing_request_id,
                        "status_token": item.get("status_token"),
                        "status": item.get("status"),
                        "queue_position": cls._queue_position_unlocked(existing_request_id),
                        "reused": True,
                    }, "您已有正在处理的卡码请求"

            if cls._queue.full():
                return None, "卡码处理队列已满，请稍后重试"

            request_id = f"rcq_{secrets.token_hex(8)}"
            status_token = secrets.token_urlsafe(20)
            now = cls._now()
            task = _RegcodeUseTask(
                request_id=request_id,
                status_token=status_token,
                uid=int(uid),
                reg_code=reg_code,
                emby_username=emby_username,
                emby_password=emby_password,
                created_at=now,
            )
            cls._pending_by_uid[int(uid)] = request_id
            cls._status[request_id] = {
                "request_id": request_id,
                "status_token": status_token,
                "uid": int(uid),
                "status": "queued",
                "message": "已进入卡码处理队列",
                "queue_position": cls._queue.qsize() + 1,
                "created_at": now,
                "updated_at": now,
            }
            cls._queue.put_nowait(task)
            return {
                "request_id": request_id,
                "status_token": status_token,
                "status": "queued",
                "queue_position": cls._status[request_id]["queue_position"],
                "reused": False,
            }, "已加入卡码处理队列"

    @classmethod
    def _queue_position_unlocked(cls, request_id: str) -> Optional[int]:
        if cls._queue is None:
            return None
        for idx, task in enumerate(list(cls._queue._queue), start=1):  # noqa: SLF001
            if task.request_id == request_id:
                return idx
        return None

    @classmethod
    def pending_uids(cls) -> set[int]:
        """当前卡码处理队列中仍未完成的 UID 集合。"""
        return {int(uid) for uid in cls._pending_by_uid.keys() if uid is not None}

    @classmethod
    def in_flight_count(cls) -> int:
        return len(cls._pending_by_uid)

    @classmethod
    async def get_status(cls, request_id: str, status_token: str) -> Optional[Dict[str, Any]]:
        cls._ensure_started()
        assert cls._state_lock is not None
        async with cls._state_lock:
            cls._cleanup_expired_status_locked()
            item = cls._status.get(request_id)
            if not item or item.get("status_token") != status_token:
                return None
            out = cls._serialize(item)
            if out.get("status") == "queued":
                out["queue_position"] = cls._queue_position_unlocked(request_id)
            return out

    @classmethod
    async def clear_for_uid(cls, uid: int, *, queued_only: bool = False) -> Dict[str, Any]:
        """管理员清理指定用户残留的卡码处理队列状态。

        queued_only=True 时只移除尚未被 worker 取走的 queued 任务。
        """
        cls._ensure_started()
        assert cls._state_lock is not None
        async with cls._state_lock:
            cls._cleanup_expired_status_locked()
            uid = int(uid)
            request_id = cls._pending_by_uid.get(uid)
            removed_from_queue = 0
            removed_status = False
            status_before = None

            item = cls._status.get(request_id) if request_id else None
            if item:
                status_before = item.get("status")
            if queued_only and request_id and status_before != "queued":
                return {
                    "request_id": request_id,
                    "status_before": status_before,
                    "removed_from_queue": 0,
                    "removed_status": False,
                    "cleared": False,
                    "reason": "该卡码请求已经开始处理，不能按未处理队列移出",
                }

            if request_id and cls._queue is not None:
                kept = []
                for task in list(cls._queue._queue):  # noqa: SLF001
                    if task.request_id == request_id:
                        removed_from_queue += 1
                        continue
                    kept.append(task)
                if removed_from_queue:
                    cls._queue._queue.clear()  # noqa: SLF001
                    cls._queue._queue.extend(kept)  # noqa: SLF001
                    for _ in range(removed_from_queue):
                        try:
                            cls._queue.task_done()
                        except ValueError:
                            break

            if request_id:
                cls._pending_by_uid.pop(uid, None)
                item = cls._status.pop(request_id, None)
                removed_status = item is not None

            return {
                "request_id": request_id,
                "status_before": status_before,
                "removed_from_queue": removed_from_queue,
                "removed_status": removed_status,
                "cleared": bool(request_id or removed_from_queue or removed_status),
            }

    @classmethod
    async def _worker_loop(cls) -> None:
        assert cls._queue is not None
        while True:
            task = await cls._queue.get()
            try:
                await cls._mark_processing(task)
                user = await UserOperate.get_user_by_uid(task.uid)
                if not user:
                    await cls._mark_failed(task, "用户不存在")
                    continue

                success, message, generated_password = await UserService.use_code(
                    user,
                    task.reg_code,
                    emby_username=task.emby_username,
                    emby_password=task.emby_password,
                )
                if success:
                    refreshed = await UserOperate.get_user_by_uid(task.uid) or user
                    user_info = await UserService.get_user_info(refreshed)
                    await cls._mark_success(task, message, {
                        "emby_password": generated_password,
                        "expire_status": user_info["expire_status"],
                        "expired_at": user_info["expired_at"],
                        "role": user_info["role"],
                        "role_name": user_info["role_name"],
                    })
                else:
                    await cls._mark_failed(task, message)
            except Exception as exc:  # pragma: no cover - defensive worker guard
                await cls._mark_failed(task, f"卡码处理失败: {exc}")
            finally:
                cls._queue.task_done()

    @classmethod
    async def _mark_processing(cls, task: _RegcodeUseTask) -> None:
        assert cls._state_lock is not None
        async with cls._state_lock:
            item = cls._status.get(task.request_id)
            if item:
                item["status"] = "processing"
                item["message"] = "正在处理卡码"
                item["queue_position"] = None
                item["updated_at"] = cls._now()

    @classmethod
    async def _mark_success(cls, task: _RegcodeUseTask, message: str, data: Dict[str, Any]) -> None:
        assert cls._state_lock is not None
        async with cls._state_lock:
            cls._pending_by_uid.pop(task.uid, None)
            item = cls._status.get(task.request_id)
            if item:
                item.update({
                    "status": "success",
                    "message": message,
                    "data": data,
                    "queue_position": None,
                    "updated_at": cls._now(),
                    "finished_at": cls._now(),
                })

    @classmethod
    async def _mark_failed(cls, task: _RegcodeUseTask, message: str) -> None:
        assert cls._state_lock is not None
        async with cls._state_lock:
            cls._pending_by_uid.pop(task.uid, None)
            item = cls._status.get(task.request_id)
            if item:
                item.update({
                    "status": "failed",
                    "message": message,
                    "queue_position": None,
                    "updated_at": cls._now(),
                    "finished_at": cls._now(),
                })

    @classmethod
    def _serialize(cls, item: Dict[str, Any]) -> Dict[str, Any]:
        out = dict(item)
        out.pop("status_token", None)
        return out

    @classmethod
    def _cleanup_expired_status_locked(cls) -> None:
        now = cls._now()
        expired = [
            request_id
            for request_id, item in cls._status.items()
            if item.get("status") in {"success", "failed"}
            and now - int(item.get("finished_at") or item.get("updated_at") or now) > cls._status_ttl
        ]
        for request_id in expired:
            cls._status.pop(request_id, None)
