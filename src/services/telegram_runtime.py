"""Telegram 调用桥接层。

统一处理两类场景：
1) 进程内存在运行中的 Bot：把调用投递到 Bot 所在线程/事件循环执行，避免跨线程冲突。
2) API 与 Bot 分离进程：回退到独立 Bot API 客户端直接调用 Telegram。
"""
from __future__ import annotations

import asyncio
import logging
from typing import Awaitable, Callable, Optional, TypeVar

from telegram import Bot
from telegram.request import HTTPXRequest

from src.config import Config, TelegramConfig

logger = logging.getLogger(__name__)

T = TypeVar("T")
BotOperation = Callable[[Bot], Awaitable[T]]


def has_telegram_api_access() -> bool:
    """是否具备 Telegram API 访问条件。"""
    return bool(Config.TELEGRAM_MODE and TelegramConfig.BOT_TOKEN)


def has_running_bot_instance() -> bool:
    """当前进程是否存在运行中的 Bot 实例。"""
    try:
        from src.bot.bot import get_bot_instance
    except Exception:
        return False

    bot_instance = get_bot_instance()
    return bool(
        bot_instance
        and getattr(bot_instance, "application", None)
        and getattr(bot_instance.application, "bot", None)
    )


def _build_request(*, pool_size: int = 8) -> HTTPXRequest:
    proxy_url = (TelegramConfig.PROXY_URL or "").strip()
    kwargs = {
        "connect_timeout": 60.0,
        "read_timeout": 60.0,
        "write_timeout": 60.0,
        "connection_pool_size": pool_size,
    }
    if proxy_url:
        kwargs["proxy"] = proxy_url
    return HTTPXRequest(**kwargs)


def _build_standalone_bot() -> Bot:
    base_url = (TelegramConfig.TELEGRAM_API_URL or "https://api.telegram.org/bot").strip()
    kwargs = {
        "token": TelegramConfig.BOT_TOKEN,
        "request": _build_request(pool_size=8),
    }
    if base_url:
        kwargs["base_url"] = base_url
    return Bot(**kwargs)


async def _await_threadsafe_future(future, timeout: Optional[float]) -> T:
    wrapped = asyncio.wrap_future(future)
    if timeout and timeout > 0:
        return await asyncio.wait_for(wrapped, timeout=timeout)
    return await wrapped


async def run_bot_operation(
    operation: BotOperation[T],
    *,
    require_running_instance: bool = False,
    timeout: Optional[float] = None,
) -> T:
    """执行一段 Telegram Bot 操作。

    优先使用当前进程内运行中的 Bot；若不存在则回退到独立客户端。
    """
    if not has_telegram_api_access():
        raise RuntimeError("Telegram API 未启用或 BOT_TOKEN 未配置")

    try:
        from src.bot.bot import get_bot_instance, get_bot_loop
    except Exception as exc:
        if require_running_instance:
            raise RuntimeError("Bot 运行实例不可用") from exc
        get_bot_instance = lambda: None  # type: ignore[assignment]
        get_bot_loop = lambda: None  # type: ignore[assignment]

    bot_instance = get_bot_instance()
    running_bot = None
    if bot_instance and getattr(bot_instance, "application", None):
        running_bot = getattr(bot_instance.application, "bot", None)

    bot_loop = get_bot_loop()

    if running_bot is not None:
        if bot_loop is None or not bot_loop.is_running():
            if require_running_instance:
                raise RuntimeError("Bot 事件循环未就绪")
            logger.warning("检测到 Bot 实例但事件循环不可用，回退到独立 Telegram API 客户端")
            running_bot = None

    if running_bot is not None:
        async def _invoke() -> T:
            return await operation(running_bot)

        try:
            current_loop = asyncio.get_running_loop()
        except RuntimeError:
            current_loop = None

        if bot_loop is not current_loop:
            future = asyncio.run_coroutine_threadsafe(_invoke(), bot_loop)
            return await _await_threadsafe_future(future, timeout)

        return await _invoke()

    if require_running_instance:
        raise RuntimeError("Bot 尚未就绪")

    logger.debug("当前进程无 Bot 实例，回退到独立 Telegram API 客户端")
    standalone_bot = _build_standalone_bot()
    async with standalone_bot:
        return await operation(standalone_bot)
