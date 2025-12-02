"""
Telegram Bot 模块

默认不开启，需要在配置文件中设置 telegram_mode = true 才能启用
"""
from src.bot.bot import TelegramBot, get_bot, start_bot, stop_bot

__all__ = [
    'TelegramBot',
    'get_bot',
    'start_bot',
    'stop_bot',
]

