"""
通用处理器工具

提供装饰器和公共函数
"""
import logging
from functools import wraps
from typing import Callable, List, Union

from pyrogram import filters
from pyrogram.types import Message, CallbackQuery

from src.config import Config, TelegramConfig
from src.db.user import UserOperate, Role

logger = logging.getLogger(__name__)


def get_admin_ids() -> List[int]:
    """获取管理员 ID 列表"""
    admin_id = TelegramConfig.ADMIN_ID
    if isinstance(admin_id, int):
        return [admin_id] if admin_id else []
    return admin_id or []


def is_admin(user_id: int) -> bool:
    """检查是否为管理员"""
    return user_id in get_admin_ids()


# ==================== 过滤器 ====================

def admin_filter():
    """管理员过滤器"""
    async def func(_, __, message: Message):
        return is_admin(message.from_user.id if message.from_user else 0)
    return filters.create(func)


def private_filter():
    """私聊过滤器"""
    return filters.private


def group_filter():
    """群组过滤器"""
    async def func(_, __, message: Message):
        group_ids = TelegramConfig.GROUP_ID
        if isinstance(group_ids, int):
            group_ids = [group_ids] if group_ids else []
        if not group_ids:
            return True  # 未配置则允许所有群组
        return message.chat.id in group_ids
    return filters.create(func)


# ==================== 装饰器 ====================

def require_admin(func: Callable) -> Callable:
    """要求管理员权限"""
    @wraps(func)
    async def wrapper(client, message: Message, *args, **kwargs):
        if not is_admin(message.from_user.id if message.from_user else 0):
            await message.reply("⚠️ 此命令仅限管理员使用")
            return
        return await func(client, message, *args, **kwargs)
    return wrapper


def require_registered(func: Callable) -> Callable:
    """要求已注册用户"""
    @wraps(func)
    async def wrapper(client, message: Message, *args, **kwargs):
        if not message.from_user:
            return
        
        user = await UserOperate.get_user_by_telegram_id(message.from_user.id)
        if not user:
            await message.reply(
                "⚠️ 您尚未绑定账号\n"
                "请使用 /bindtg <用户名> 绑定您的 Emby 账号"
            )
            return
        
        # 将用户信息传递给处理函数
        return await func(client, message, user=user, *args, **kwargs)
    return wrapper


def require_subscribe(func: Callable) -> Callable:
    """要求订阅频道/加入群组"""
    @wraps(func)
    async def wrapper(client, message: Message, *args, **kwargs):
        if not TelegramConfig.FORCE_SUBSCRIBE:
            return await func(client, message, *args, **kwargs)
        
        user_id = message.from_user.id if message.from_user else 0
        
        # 管理员跳过检查
        if is_admin(user_id):
            return await func(client, message, *args, **kwargs)
        
        # 检查频道订阅
        channel_ids = TelegramConfig.CHANNEL_ID
        if isinstance(channel_ids, int):
            channel_ids = [channel_ids] if channel_ids else []
        
        for channel_id in channel_ids:
            try:
                member = await client.get_chat_member(channel_id, user_id)
                if member.status in ['left', 'kicked']:
                    await message.reply("⚠️ 请先订阅频道后再使用此功能")
                    return
            except Exception:
                pass  # 无法检查则跳过
        
        return await func(client, message, *args, **kwargs)
    return wrapper


# ==================== 工具函数 ====================

def escape_markdown(text: str) -> str:
    """转义 Markdown 特殊字符"""
    special_chars = ['_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!']
    for char in special_chars:
        text = text.replace(char, f'\\{char}')
    return text


def format_user_info(user) -> str:
    """格式化用户信息"""
    from src.core.utils import format_expire_time
    
    lines = [
        f"👤 **用户名**: `{user.USERNAME}`",
        f"🆔 **UID**: `{user.UID}`",
    ]
    
    if user.EMBYID:
        lines.append(f"🎬 **Emby ID**: `{user.EMBYID}`")
    
    if user.TELEGRAM_ID:
        lines.append(f"📱 **Telegram**: `{user.TELEGRAM_ID}`")
    
    role_map = {
        Role.ADMIN.value: "管理员",
        Role.WHITELIST.value: "白名单",
        Role.NORMAL.value: "普通用户",
    }
    lines.append(f"👑 **角色**: {role_map.get(user.ROLE, '未知')}")
    
    lines.append(f"⏰ **到期时间**: {format_expire_time(user.EXPIRED_AT)}")
    lines.append(f"📊 **状态**: {'✅ 活跃' if user.ACTIVE_STATUS else '❌ 禁用'}")
    
    return "\n".join(lines)

