"""
Telegram Bot 核心模块

基于 pyrogram 实现的 Telegram Bot
参考: https://github.com/berry8838/Sakura_embyboss
"""
import logging
from typing import Optional, List, Union

from pyrogram import Client, filters
from pyrogram.types import Message, CallbackQuery, InlineKeyboardMarkup, InlineKeyboardButton
from pyrogram.handlers import MessageHandler, CallbackQueryHandler

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
        
        # 创建 Pyrogram Client
        self.app = Client(
            name="twilight_bot",
            bot_token=self.bot_token,
            api_id=12345,  # 使用 bot token 时这些可以是任意值
            api_hash="0123456789abcdef0123456789abcdef",
            in_memory=True,
        )
        
        # 注册处理器
        self._register_handlers()
        
        logger.info("Telegram Bot 初始化完成")
    
    @staticmethod
    def _normalize_ids(ids: Union[int, List[int]]) -> List[int]:
        """标准化 ID 列表"""
        if isinstance(ids, int):
            return [ids] if ids else []
        return ids or []
    
    def is_admin(self, user_id: int) -> bool:
        """检查是否为管理员"""
        return user_id in self.admin_ids
    
    def _register_handlers(self):
        """注册消息处理器"""
        # 导入处理器模块
        from src.bot.handlers import user_handlers, admin_handlers, score_handlers, emby_handlers
        
        # 注册用户命令
        user_handlers.register(self)
        
        # 注册管理员命令
        admin_handlers.register(self)
        
        # 注册积分命令
        score_handlers.register(self)
        
        # 注册 Emby 命令
        emby_handlers.register(self)
    
    async def start(self):
        """启动 Bot"""
        logger.info("正在启动 Telegram Bot...")
        await self.app.start()
        
        me = await self.app.get_me()
        logger.info(f"Telegram Bot 已启动: @{me.username}")
    
    async def stop(self):
        """停止 Bot"""
        logger.info("正在停止 Telegram Bot...")
        await self.app.stop()
        logger.info("Telegram Bot 已停止")
    
    async def send_message(
        self,
        chat_id: int,
        text: str,
        reply_markup: InlineKeyboardMarkup = None,
        parse_mode: str = "Markdown"
    ) -> Optional[Message]:
        """发送消息"""
        try:
            return await self.app.send_message(
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
        chat_ids: List[int] = None,
        reply_markup: InlineKeyboardMarkup = None
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

