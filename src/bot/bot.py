"""
Telegram Bot 核心模块

基于 python-telegram-bot 实现的 Telegram Bot
参考: https://github.com/Prejudice-Studio/Telegram-Jellyfin-Bot
"""
import asyncio
import logging
from typing import Optional, List, Union

from telegram import Bot, Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.error import TimedOut, NetworkError, RetryAfter, BadRequest
from telegram.ext import Application, CommandHandler, CallbackQueryHandler, MessageHandler, filters
from telegram.request import HTTPXRequest

from src.config import Config, TelegramConfig

logger = logging.getLogger(__name__)

# 全局 Bot 实例
_bot_instance: Optional['TelegramBot'] = None


class TelegramBot:
    """Telegram Bot 主类"""
    
    def __init__(self):
        if not Config.TELEGRAM_MODE:
            raise RuntimeError("Telegram 模式未启用，请在配置文件中设置 telegram_mode = true")
        
        if not TelegramConfig.BOT_TOKEN:
            raise RuntimeError("未配置 BOT_TOKEN")
        
        self.bot_token = TelegramConfig.BOT_TOKEN
        self.admin_ids = self._normalize_ids(TelegramConfig.ADMIN_ID)
        self.group_ids = self._normalize_ids(TelegramConfig.GROUP_ID)
        self.channel_ids = self._normalize_ids(TelegramConfig.CHANNEL_ID)
        self.force_subscribe = TelegramConfig.FORCE_SUBSCRIBE
        self._running = False
        
        # 创建 python-telegram-bot Application
        builder = Application.builder().token(self.bot_token)
        
        # 自定义 Telegram API URL（用于代理/自建 API）
        base_url = TelegramConfig.TELEGRAM_API_URL
        if base_url and base_url != 'https://api.telegram.org/bot':
            builder = builder.base_url(base_url)
        
        # 代理配置
        proxy_url = TelegramConfig.PROXY_URL
        if proxy_url:
            logger.info(f"Bot 使用代理: {proxy_url}")
            request = HTTPXRequest(
                proxy=proxy_url,
                connect_timeout=60.0,
                read_timeout=60.0,
                write_timeout=60.0,
                connection_pool_size=16,
            )
            builder = builder.request(request)
            # 同时给 get_updates 用的 request 也设置代理
            get_updates_request = HTTPXRequest(
                proxy=proxy_url,
                connect_timeout=60.0,
                read_timeout=60.0,
                write_timeout=60.0,
                connection_pool_size=4,
            )
            builder = builder.get_updates_request(get_updates_request)
        else:
            builder = builder.connect_timeout(60)
            builder = builder.read_timeout(60)
            builder = builder.write_timeout(60)
        
        builder = builder.concurrent_updates(True)
        
        self.application = builder.build()
        
        # 注册处理器
        self._register_handlers()
        
        # 注册全局错误处理
        self.application.add_error_handler(self._error_handler)
        
        logger.info("Telegram Bot 初始化完成")
    
    @staticmethod
    def _normalize_ids(ids: Union[int, str, List[Union[int, str]]]) -> List[Union[int, str]]:
        """标准化 ID 列表，支持数字ID和 @channelusername 格式"""
        if isinstance(ids, (int, str)):
            return [ids] if ids else []
        return ids or []
    
    def is_admin(self, user_id: int) -> bool:
        """检查是否为管理员"""
        return user_id in self.admin_ids
    
    def _register_handlers(self):
        """注册消息处理器"""
        from src.bot.handlers import user_handlers, admin_handlers, score_handlers, emby_handlers
        
        # 注册用户命令
        user_handlers.register(self)
        
        # 注册管理员命令
        admin_handlers.register(self)
        
        # 注册积分命令
        score_handlers.register(self)
        
        # 注册 Emby 命令
        emby_handlers.register(self)
    
    @staticmethod
    async def _error_handler(update: object, context) -> None:
        """全局错误处理"""
        error = context.error
        
        if isinstance(error, RetryAfter):
            logger.warning(f"Flood control: 等待 {error.retry_after}s")
            await asyncio.sleep(error.retry_after)
            return
        
        if isinstance(error, TimedOut):
            logger.warning(f"请求超时: {error}")
            return
        
        if isinstance(error, NetworkError):
            logger.warning(f"网络错误 (将自动重试): {error}")
            return
        
        if isinstance(error, BadRequest):
            if "Message is not modified" in str(error):
                return  # 忽略消息未修改
            if "Query is too old" in str(error):
                return  # 忽略过期 callback
            logger.warning(f"BadRequest: {error}")
            return
        
        # 其他错误
        logger.error(f"Bot 未处理异常: {error}", exc_info=context.error)
    
    @property
    def bot(self) -> Bot:
        """获取底层 Bot 对象"""
        return self.application.bot
    
    @property
    def is_running(self) -> bool:
        """检查 Bot 是否正在运行"""
        return self._running
    
    async def start(self):
        """启动 Bot（非阻塞，使用 polling）"""
        logger.info("正在启动 Telegram Bot...")
        
        await self.application.initialize()
        await self.application.start()
        
        # 启动 polling（不阻塞）
        await self.application.updater.start_polling(
            allowed_updates=Update.ALL_TYPES,
            drop_pending_updates=True,
        )
        
        self._running = True
        
        me = await self.bot.get_me()
        logger.info(f"Telegram Bot 已启动: @{me.username}")
    
    async def stop(self):
        """停止 Bot"""
        logger.info("正在停止 Telegram Bot...")
        if self.application.updater and self.application.updater.running:
            await self.application.updater.stop()
        await self.application.stop()
        await self.application.shutdown()
        self._running = False
        logger.info("Telegram Bot 已停止")
    
    async def send_message(
        self,
        chat_id: Union[int, str],
        text: str,
        reply_markup=None,
        parse_mode: str = "Markdown",
    ):
        """发送消息"""
        try:
            return await self.bot.send_message(
                chat_id=chat_id,
                text=text,
                reply_markup=reply_markup,
                parse_mode=parse_mode,
            )
        except Exception as e:
            logger.error(f"发送消息失败: {e}")
            return None
    
    async def broadcast(
        self,
        text: str,
        chat_ids: List[Union[int, str]] = None,
        reply_markup=None,
    ) -> int:
        """
        广播消息
        
        :param text: 消息内容
        :param chat_ids: 目标用户列表，为空则发送给所有管理员
        :return: 成功发送数量
        """
        if not chat_ids:
            chat_ids = self.admin_ids
        
        success = 0
        for chat_id in chat_ids:
            if await self.send_message(chat_id, text, reply_markup):
                success += 1
        
        return success


def get_bot() -> Optional[TelegramBot]:
    """获取 Bot 实例"""
    return _bot_instance


def get_bot_instance() -> Optional[TelegramBot]:
    """获取 Bot 实例（别名）"""
    return _bot_instance


async def start_bot() -> Optional[TelegramBot]:
    """启动 Bot"""
    global _bot_instance
    
    if not Config.TELEGRAM_MODE:
        logger.info("Telegram 模式未启用，跳过 Bot 启动")
        return None
    
    if _bot_instance is not None:
        logger.warning("Bot 已在运行")
        return _bot_instance
    
    try:
        _bot_instance = TelegramBot()
        await _bot_instance.start()
        return _bot_instance
    except Exception as e:
        logger.error(f"启动 Bot 失败: {e}")
        return None


async def stop_bot():
    """停止 Bot"""
    global _bot_instance
    
    if _bot_instance is not None:
        await _bot_instance.stop()
        _bot_instance = None

