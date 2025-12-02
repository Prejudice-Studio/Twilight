"""
通知服务

支持多渠道通知推送（Telegram、Webhook 等）
参考: https://github.com/berry8838/Sakura_embyboss
"""
import logging
from typing import Optional, List, Dict, Any
from dataclasses import dataclass
from enum import Enum

from src.config import Config, TelegramConfig, WebhookConfig
from src.services.webhook import WebhookPushService

logger = logging.getLogger(__name__)


class NotificationType(Enum):
    """通知类型"""
    # 用户相关
    USER_REGISTERED = 'user.registered'
    USER_RENEWED = 'user.renewed'
    USER_EXPIRED = 'user.expired'
    USER_DISABLED = 'user.disabled'
    
    # 播放相关
    PLAYBACK_START = 'playback.start'
    PLAYBACK_STOP = 'playback.stop'
    
    # 排行榜
    DAILY_RANKING = 'ranking.daily'
    WEEKLY_RANKING = 'ranking.weekly'
    
    # 媒体相关
    MEDIA_REQUESTED = 'media.requested'
    MEDIA_ADDED = 'media.added'
    
    # 系统相关
    SYSTEM_ALERT = 'system.alert'


@dataclass
class Notification:
    """通知数据"""
    type: NotificationType
    title: str
    content: str
    data: Optional[Dict[str, Any]] = None
    target_users: Optional[List[int]] = None  # None 表示广播


class NotificationChannel:
    """通知渠道基类"""
    
    async def send(self, notification: Notification) -> bool:
        """发送通知"""
        raise NotImplementedError


class TelegramNotificationChannel(NotificationChannel):
    """Telegram 通知渠道"""
    
    def __init__(self):
        self.enabled = Config.TELEGRAM_MODE
        self.bot_token = TelegramConfig.BOT_TOKEN
        self.admin_ids = TelegramConfig.ADMIN_ID
        if isinstance(self.admin_ids, int):
            self.admin_ids = [self.admin_ids]
    
    async def send(self, notification: Notification) -> bool:
        """通过 Telegram 发送通知"""
        if not self.enabled or not self.bot_token:
            return False
        
        import httpx
        
        # 构建消息
        text = f"*{notification.title}*\n\n{notification.content}"
        
        # 确定接收者
        if notification.target_users:
            chat_ids = notification.target_users
        else:
            # 默认发送给管理员
            chat_ids = self.admin_ids
        
        success = True
        for chat_id in chat_ids:
            try:
                url = f"https://api.telegram.org/bot{self.bot_token}/sendMessage"
                async with httpx.AsyncClient(timeout=10) as client:
                    response = await client.post(url, json={
                        'chat_id': chat_id,
                        'text': text,
                        'parse_mode': 'Markdown',
                    })
                    
                    if response.status_code != 200:
                        logger.warning(f"Telegram 通知发送失败: {response.text}")
                        success = False
            except Exception as e:
                logger.error(f"Telegram 通知错误: {e}")
                success = False
        
        return success


class WebhookNotificationChannel(NotificationChannel):
    """Webhook 通知渠道"""
    
    def __init__(self):
        self.enabled = WebhookConfig.WEBHOOK_ENABLED
    
    async def send(self, notification: Notification) -> bool:
        """通过 Webhook 推送通知"""
        if not self.enabled:
            return False
        
        count = await WebhookPushService.push(
            event=notification.type.value,
            data={
                'title': notification.title,
                'content': notification.content,
                'data': notification.data,
                'target_users': notification.target_users,
            }
        )
        
        return count > 0


class NotificationService:
    """通知服务"""
    
    _channels: List[NotificationChannel] = []
    _initialized = False
    
    @classmethod
    def initialize(cls) -> None:
        """初始化通知渠道"""
        if cls._initialized:
            return
        
        # 添加 Telegram 渠道
        if Config.TELEGRAM_MODE:
            cls._channels.append(TelegramNotificationChannel())
        
        # 添加 Webhook 渠道
        if WebhookConfig.WEBHOOK_ENABLED:
            cls._channels.append(WebhookNotificationChannel())
        
        cls._initialized = True
        logger.info(f"通知服务初始化完成，已注册 {len(cls._channels)} 个渠道")
    
    @classmethod
    def add_channel(cls, channel: NotificationChannel) -> None:
        """添加自定义通知渠道"""
        cls._channels.append(channel)
    
    @classmethod
    async def send(cls, notification: Notification) -> int:
        """
        发送通知
        
        :return: 成功发送的渠道数
        """
        if not cls._initialized:
            cls.initialize()
        
        success_count = 0
        for channel in cls._channels:
            try:
                if await channel.send(notification):
                    success_count += 1
            except Exception as e:
                logger.error(f"通知发送失败: {e}")
        
        return success_count
    
    # ==================== 便捷方法 ====================
    
    @classmethod
    async def notify_user_registered(cls, username: str, telegram_id: int = None) -> int:
        """通知新用户注册"""
        return await cls.send(Notification(
            type=NotificationType.USER_REGISTERED,
            title="🎉 新用户注册",
            content=f"用户 {username} 已成功注册",
            data={'username': username, 'telegram_id': telegram_id},
        ))
    
    @classmethod
    async def notify_user_renewed(cls, username: str, days: int) -> int:
        """通知用户续期"""
        return await cls.send(Notification(
            type=NotificationType.USER_RENEWED,
            title="🔄 用户续期",
            content=f"用户 {username} 续期 {days} 天",
            data={'username': username, 'days': days},
        ))
    
    @classmethod
    async def notify_user_expired(cls, username: str) -> int:
        """通知用户过期"""
        return await cls.send(Notification(
            type=NotificationType.USER_EXPIRED,
            title="⏰ 用户过期",
            content=f"用户 {username} 已过期",
            data={'username': username},
        ))
    
    @classmethod
    async def notify_daily_ranking(cls, ranking: List[Dict[str, Any]], date: str) -> int:
        """通知每日排行榜"""
        if not ranking:
            return 0
        
        lines = [f"📊 {date} 播放排行榜", ""]
        for item in ranking[:10]:
            lines.append(f"{item['rank']}. {item['username']} - {item['value_str']}")
        
        return await cls.send(Notification(
            type=NotificationType.DAILY_RANKING,
            title=f"📊 {date} 播放排行榜",
            content="\n".join(lines),
            data={'ranking': ranking, 'date': date},
        ))
    
    @classmethod
    async def notify_media_requested(cls, username: str, media_name: str, source: str) -> int:
        """通知媒体求片"""
        return await cls.send(Notification(
            type=NotificationType.MEDIA_REQUESTED,
            title="📺 新的求片请求",
            content=f"用户 {username} 请求: {media_name} ({source})",
            data={'username': username, 'media_name': media_name, 'source': source},
        ))
    
    @classmethod
    async def notify_media_added(cls, media_name: str, media_type: str) -> int:
        """通知媒体入库"""
        emoji = "🎬" if media_type == 'movie' else "📺"
        return await cls.send(Notification(
            type=NotificationType.MEDIA_ADDED,
            title=f"{emoji} 新媒体入库",
            content=f"{media_name} 已添加到媒体库",
            data={'media_name': media_name, 'media_type': media_type},
        ))
    
    @classmethod
    async def notify_system_alert(cls, title: str, message: str) -> int:
        """系统警告通知"""
        return await cls.send(Notification(
            type=NotificationType.SYSTEM_ALERT,
            title=f"⚠️ {title}",
            content=message,
        ))

