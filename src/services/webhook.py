"""
Webhook 服务

接收 Emby/Jellyfin 的 Webhook 事件
支持与外部系统联动
"""
import json
import hmac
import hashlib
import logging
from typing import Optional, Dict, Any, List, Callable, Awaitable
from dataclasses import dataclass
from enum import Enum

from src.db.user import UserOperate
from src.services.stats_service import StatsService
from src.config import Config
from src.core.utils import timestamp

logger = logging.getLogger(__name__)


class WebhookEvent(Enum):
    """Webhook 事件类型"""
    # Emby/Jellyfin 播放事件
    PLAYBACK_START = 'playback.start'
    PLAYBACK_STOP = 'playback.stop'
    PLAYBACK_PAUSE = 'playback.pause'
    PLAYBACK_UNPAUSE = 'playback.unpause'
    PLAYBACK_PROGRESS = 'playback.progress'
    
    # 用户事件
    USER_CREATED = 'user.created'
    USER_DELETED = 'user.deleted'
    USER_AUTHENTICATED = 'user.authenticated'
    USER_LOCKED_OUT = 'user.locked_out'
    
    # 媒体库事件
    LIBRARY_SCAN_COMPLETE = 'library.scan_complete'
    ITEM_ADDED = 'item.added'
    ITEM_REMOVED = 'item.removed'
    
    # 系统事件
    SERVER_STARTED = 'server.started'
    SERVER_SHUTDOWN = 'server.shutdown'
    
    # 自定义事件
    CUSTOM = 'custom'


@dataclass
class WebhookPayload:
    """Webhook 载荷"""
    event: str
    timestamp: int
    server_id: str
    server_name: str
    user_id: Optional[str]
    user_name: Optional[str]
    item_id: Optional[str]
    item_name: Optional[str]
    item_type: Optional[str]
    series_name: Optional[str]
    season_name: Optional[str]
    client: Optional[str]
    device_name: Optional[str]
    device_id: Optional[str]
    play_method: Optional[str]
    position_ticks: Optional[int]
    is_paused: Optional[bool]
    ip_address: Optional[str]
    raw_data: Dict[str, Any]
    
    @classmethod
    def from_emby(cls, data: Dict[str, Any]) -> 'WebhookPayload':
        """从 Emby Webhook 数据创建"""
        event_type = data.get('Event', data.get('NotificationType', 'custom'))
        
        # 映射 Emby 事件类型
        event_map = {
            'playbackstart': 'playback.start',
            'playbackstop': 'playback.stop',
            'playbackprogress': 'playback.progress',
            'usercreated': 'user.created',
            'userdeleted': 'user.deleted',
            'authenticationfailed': 'user.locked_out',
            'authenticationSuccess': 'user.authenticated',
            'librarychanged': 'library.scan_complete',
            'itemadded': 'item.added',
            'itemremoved': 'item.removed',
        }
        
        event = event_map.get(event_type.lower(), event_type.lower())
        
        # 提取用户信息
        user_data = data.get('User', {})
        session_data = data.get('Session', {})
        item_data = data.get('Item', {})
        server_data = data.get('Server', {})
        
        # 播放信息
        play_state = data.get('PlayState', {}) or session_data.get('PlayState', {})
        
        return cls(
            event=event,
            timestamp=timestamp(),
            server_id=server_data.get('Id', ''),
            server_name=server_data.get('Name', ''),
            user_id=user_data.get('Id') or data.get('UserId'),
            user_name=user_data.get('Name') or data.get('UserName'),
            item_id=item_data.get('Id') or data.get('ItemId'),
            item_name=item_data.get('Name') or data.get('ItemName'),
            item_type=item_data.get('Type') or data.get('ItemType'),
            series_name=item_data.get('SeriesName'),
            season_name=item_data.get('SeasonName'),
            client=session_data.get('Client'),
            device_name=session_data.get('DeviceName'),
            device_id=session_data.get('DeviceId'),
            play_method=play_state.get('PlayMethod'),
            position_ticks=play_state.get('PositionTicks'),
            is_paused=play_state.get('IsPaused', False),
            ip_address=session_data.get('RemoteEndPoint'),
            raw_data=data,
        )
    
    @classmethod
    def from_jellyfin(cls, data: Dict[str, Any]) -> 'WebhookPayload':
        """从 Jellyfin Webhook 数据创建（格式类似 Emby）"""
        return cls.from_emby(data)


# Webhook 处理器类型
WebhookHandler = Callable[[WebhookPayload], Awaitable[None]]


class WebhookService:
    """Webhook 服务"""
    
    # 注册的处理器
    _handlers: Dict[str, List[WebhookHandler]] = {}
    
    # Webhook 密钥（用于验证）
    _secret: Optional[str] = None
    
    @classmethod
    def set_secret(cls, secret: str) -> None:
        """设置 Webhook 密钥"""
        cls._secret = secret
    
    @classmethod
    def verify_signature(cls, payload: bytes, signature: str) -> bool:
        """验证 Webhook 签名"""
        if not cls._secret:
            return True  # 未设置密钥则跳过验证
        
        expected = hmac.new(
            cls._secret.encode(),
            payload,
            hashlib.sha256
        ).hexdigest()
        
        return hmac.compare_digest(f"sha256={expected}", signature)
    
    @classmethod
    def register_handler(cls, event: str, handler: WebhookHandler) -> None:
        """
        注册事件处理器
        
        :param event: 事件类型 (支持通配符 * 匹配所有)
        :param handler: 处理函数
        """
        if event not in cls._handlers:
            cls._handlers[event] = []
        cls._handlers[event].append(handler)
        logger.debug(f"注册 Webhook 处理器: {event}")
    
    @classmethod
    def unregister_handler(cls, event: str, handler: WebhookHandler) -> None:
        """注销事件处理器"""
        if event in cls._handlers and handler in cls._handlers[event]:
            cls._handlers[event].remove(handler)
    
    @classmethod
    async def process(cls, data: Dict[str, Any], source: str = 'emby') -> Dict[str, Any]:
        """
        处理 Webhook 请求
        
        :param data: Webhook 数据
        :param source: 来源 (emby/jellyfin/custom)
        :return: 处理结果
        """
        # 解析载荷
        if source in ('emby', 'jellyfin'):
            payload = WebhookPayload.from_emby(data)
        else:
            # 自定义格式
            payload = WebhookPayload(
                event=data.get('event', 'custom'),
                timestamp=data.get('timestamp', timestamp()),
                server_id='',
                server_name='',
                user_id=data.get('user_id'),
                user_name=data.get('user_name'),
                item_id=data.get('item_id'),
                item_name=data.get('item_name'),
                item_type=data.get('item_type'),
                series_name=None,
                season_name=None,
                client=None,
                device_name=None,
                device_id=None,
                play_method=None,
                position_ticks=None,
                is_paused=False,
                ip_address=None,
                raw_data=data,
            )
        
        logger.info(f"收到 Webhook: {payload.event} (用户: {payload.user_name})")
        
        # 内置处理
        await cls._builtin_handler(payload)
        
        # 调用注册的处理器
        handlers_called = 0
        
        # 匹配具体事件
        if payload.event in cls._handlers:
            for handler in cls._handlers[payload.event]:
                try:
                    await handler(payload)
                    handlers_called += 1
                except Exception as e:
                    logger.error(f"Webhook 处理器错误: {e}")
        
        # 匹配通配符
        if '*' in cls._handlers:
            for handler in cls._handlers['*']:
                try:
                    await handler(payload)
                    handlers_called += 1
                except Exception as e:
                    logger.error(f"Webhook 处理器错误: {e}")
        
        return {
            'success': True,
            'event': payload.event,
            'handlers_called': handlers_called,
        }
    
    @classmethod
    async def _builtin_handler(cls, payload: WebhookPayload) -> None:
        """内置处理器 - 处理播放统计"""
        # 查找本地用户
        if not payload.user_id:
            return
        
        user = await UserOperate.get_user_by_embyid(payload.user_id)
        if not user:
            logger.debug(f"Webhook: 未找到本地用户 (Emby ID: {payload.user_id})")
            return
        
        # 播放开始
        if payload.event == 'playback.start':
            await StatsService.record_play_start(
                uid=user.UID,
                emby_user_id=payload.user_id,
                item_id=payload.item_id,
                item_name=payload.item_name or '未知',
                item_type=payload.item_type or '',
                client=payload.client,
                device_name=payload.device_name,
                play_method=payload.play_method,
                series_name=payload.series_name,
                season_name=payload.season_name,
                ip_address=payload.ip_address,
            )
        
        # 播放停止
        elif payload.event == 'playback.stop':
            await StatsService.record_play_stop(
                emby_user_id=payload.user_id,
                item_id=payload.item_id,
                position_ticks=payload.position_ticks,
            )
        
        # 播放进度
        elif payload.event == 'playback.progress':
            await StatsService.record_play_progress(
                emby_user_id=payload.user_id,
                item_id=payload.item_id,
                position_ticks=payload.position_ticks,
                is_paused=payload.is_paused,
            )


# 外部 Webhook 推送服务
class WebhookPushService:
    """Webhook 推送服务 - 向外部系统推送事件"""
    
    _endpoints: List[Dict[str, Any]] = []
    
    @classmethod
    def add_endpoint(cls, url: str, events: List[str] = None, secret: str = None) -> None:
        """
        添加推送端点
        
        :param url: 推送 URL
        :param events: 监听的事件列表（空=全部）
        :param secret: 签名密钥
        """
        cls._endpoints.append({
            'url': url,
            'events': events or ['*'],
            'secret': secret,
        })
    
    @classmethod
    def remove_endpoint(cls, url: str) -> None:
        """移除推送端点"""
        cls._endpoints = [e for e in cls._endpoints if e['url'] != url]
    
    @classmethod
    async def push(cls, event: str, data: Dict[str, Any]) -> int:
        """
        推送事件到所有端点
        
        :return: 成功推送数量
        """
        import httpx
        
        success_count = 0
        payload = {
            'event': event,
            'timestamp': timestamp(),
            'data': data,
        }
        
        for endpoint in cls._endpoints:
            # 检查事件是否匹配
            if '*' not in endpoint['events'] and event not in endpoint['events']:
                continue
            
            try:
                headers = {'Content-Type': 'application/json'}
                
                # 添加签名
                if endpoint['secret']:
                    body = json.dumps(payload).encode()
                    signature = hmac.new(
                        endpoint['secret'].encode(),
                        body,
                        hashlib.sha256
                    ).hexdigest()
                    headers['X-Webhook-Signature'] = f"sha256={signature}"
                
                async with httpx.AsyncClient(timeout=10) as client:
                    response = await client.post(
                        endpoint['url'],
                        json=payload,
                        headers=headers
                    )
                    
                    if response.status_code < 400:
                        success_count += 1
                    else:
                        logger.warning(f"Webhook 推送失败: {endpoint['url']} - {response.status_code}")
            
            except Exception as e:
                logger.error(f"Webhook 推送错误: {endpoint['url']} - {e}")
        
        return success_count

