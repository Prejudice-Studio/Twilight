"""
注册过程同步与缓存工具

为高并发注册场景提供分布式锁和短期缓存支持，防止重复用户名/Telegram 注册、用户数超限
"""
import asyncio
import logging
import time
import uuid
from typing import Dict, Optional

from src.config import Config

try:
    from redis.asyncio import Redis
except ImportError:  # pragma: no cover - optional dependency
    Redis = None  # type: ignore

logger = logging.getLogger(__name__)

_redis_client: Optional["Redis"] = None
_local_locks: Dict[str, asyncio.Lock] = {}


def _get_redis() -> Optional["Redis"]:
    if not Config.REDIS_URL or Redis is None:
        return None
    global _redis_client
    if _redis_client is None:
        _redis_client = Redis.from_url(Config.REDIS_URL, decode_responses=True, encoding="utf-8")
    return _redis_client


async def acquire_lock(key: str, timeout: float = 5.0, ttl: int = 30) -> Optional[str]:
    """尝试获取分布式锁，返回锁标识 token。"""
    redis_client = _get_redis()
    if redis_client is not None:
        token = uuid.uuid4().hex
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                if await redis_client.set(key, token, nx=True, ex=ttl):
                    return token
            except Exception as exc:  # pragma: no cover
                logger.warning("Redis 锁获取失败，回退到本地锁：%s", exc)
                break
            await asyncio.sleep(0.1)
        return None

    lock = _local_locks.get(key)
    if lock is None:
        lock = asyncio.Lock()
        _local_locks[key] = lock

    try:
        await asyncio.wait_for(lock.acquire(), timeout=timeout)
        return key
    except asyncio.TimeoutError:
        return None


async def release_lock(key: str, token: str) -> None:
    """释放分布式锁。"""
    redis_client = _get_redis()
    if redis_client is not None:
        try:
            script = (
                "if redis.call('get', KEYS[1]) == ARGV[1] then "
                "return redis.call('del', KEYS[1]) else return 0 end"
            )
            await redis_client.eval(script, 1, key, token)
        except Exception as exc:  # pragma: no cover
            logger.warning("Redis 锁释放失败：%s", exc)
        return

    lock = _local_locks.get(key)
    if lock is not None and lock.locked():
        try:
            lock.release()
        except RuntimeError:
            pass


async def acquire_registration_lock(
    username: str,
    telegram_id: Optional[int] = None,
    reg_code: Optional[str] = None,
    timeout: float = 5.0,
    ttl: int = 30,
) -> Optional[Dict[str, str]]:
    """按用户名/Telegram ID/注册码获取注册锁，避免重复注册冲突。"""
    keys = [f"tw:register:username:{username.lower()}"]
    if telegram_id is not None:
        keys.append(f"tw:register:telegram:{telegram_id}")
    if reg_code:
        keys.append(f"tw:register:regcode:{reg_code}")

    acquired: Dict[str, str] = {}
    for key in keys:
        token = await acquire_lock(key, timeout=timeout, ttl=ttl)
        if token is None:
            for held_key, held_token in acquired.items():
                await release_lock(held_key, held_token)
            return None
        acquired[key] = token
    return acquired


async def release_registration_lock(locks: Dict[str, str]) -> None:
    for key, token in locks.items():
        await release_lock(key, token)


async def acquire_global_registration_lock(timeout: float = 5.0, ttl: int = 30) -> Optional[str]:
    return await acquire_lock("tw:register:global", timeout=timeout, ttl=ttl)


async def release_global_registration_lock(token: str) -> None:
    await release_lock("tw:register:global", token)


async def get_cached_registered_user_count() -> Optional[int]:
    """获取短期缓存的已注册用户数。"""
    redis_client = _get_redis()
    if redis_client is None:
        return None

    try:
        raw = await redis_client.get("tw:register:count")
        if raw is None:
            return None
        return int(raw)
    except Exception as exc:  # pragma: no cover
        logger.warning("读取注册用户数缓存失败：%s", exc)
        return None


async def set_cached_registered_user_count(count: int, ttl: int = 2) -> None:
    redis_client = _get_redis()
    if redis_client is None:
        return

    try:
        await redis_client.set("tw:register:count", count, ex=ttl)
    except Exception as exc:  # pragma: no cover
        logger.warning("设置注册用户数缓存失败：%s", exc)


async def incr_cached_registered_user_count(delta: int = 1, ttl: int = 2) -> None:
    redis_client = _get_redis()
    if redis_client is None:
        return

    try:
        await redis_client.incrby("tw:register:count", delta)
        await redis_client.expire("tw:register:count", ttl)
    except Exception as exc:  # pragma: no cover
        logger.warning("增加注册数缓存失败：%s", exc)
