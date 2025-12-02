"""
Twilight - Emby 用户管理系统

一个功能完善的 Emby/Jellyfin 用户管理系统，支持：
- 用户注册、续期、管理
- 积分系统（签到、红包、转账）
- 注册码管理
- Bangumi 番剧求片
- REST API 接口
- 可选的 Telegram Bot 接口
"""

__version__ = '1.0.0'
__author__ = 'MoYuanCN'

from src.config import Config, EmbyConfig, TelegramConfig, ScoreAndRegisterConfig

__all__ = [
    '__version__',
    '__author__',
    'Config',
    'EmbyConfig', 
    'TelegramConfig',
    'ScoreAndRegisterConfig',
]

