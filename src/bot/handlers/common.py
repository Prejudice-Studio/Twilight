"""
通用处理器工具

提供装饰器、公共函数和隐私保护逻辑
"""
import asyncio
import logging
from functools import wraps
from typing import Callable, List, Union, Optional

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton, Bot
from telegram.ext import ContextTypes
from telegram.error import BadRequest, TimedOut, NetworkError

from src.config import Config, TelegramConfig, EmbyConfig
from src.db.user import UserOperate, Role

logger = logging.getLogger(__name__)

# ==================== 常量 ====================
GROUP_MSG_DELETE_DELAY = 10
PRIVATE_HINT_DELETE_DELAY = 8


# ==================== 辅助函数 ====================

def get_admin_ids() -> List[int]:
    """获取管理员 ID 列表"""
    admin_id = TelegramConfig.ADMIN_ID
    if isinstance(admin_id, int):
        return [admin_id] if admin_id else []
    return admin_id or []


def is_admin(user_id: int) -> bool:
    return user_id in get_admin_ids()


def is_private(update: Update) -> bool:
    return update.effective_chat and update.effective_chat.type == "private"


def is_group(update: Update) -> bool:
    return update.effective_chat and update.effective_chat.type in ("group", "supergroup")


def get_bot_username(context: ContextTypes.DEFAULT_TYPE) -> str:
    return context.bot.username or ""


async def safe_delete_message(message, delay: float = 0):
    """安全删除消息（忽略错误）"""
    try:
        if delay > 0:
            await asyncio.sleep(delay)
        await message.delete()
    except Exception:
        pass


async def safe_edit_message(message, text: str, reply_markup=None, parse_mode="Markdown"):
    """安全编辑消息"""
    try:
        return await message.edit_text(text, reply_markup=reply_markup, parse_mode=parse_mode)
    except BadRequest as e:
        if "Message is not modified" not in str(e):
            logger.warning(f"编辑消息失败: {e}")
        return None
    except Exception as e:
        logger.warning(f"编辑消息失败: {e}")
        return None


async def redirect_to_private(update: Update, context: ContextTypes.DEFAULT_TYPE, hint: str = "请在私聊中使用此功能"):
    """群组中引导用户到私聊"""
    bot_username = get_bot_username(context)
    keyboard = InlineKeyboardMarkup([
        [InlineKeyboardButton("📨 前往私聊", url=f"https://t.me/{bot_username}")]
    ])
    reply = await update.message.reply_text(f"🔒 {hint}", reply_markup=keyboard)
    asyncio.create_task(safe_delete_message(update.message, GROUP_MSG_DELETE_DELAY))
    asyncio.create_task(safe_delete_message(reply, GROUP_MSG_DELETE_DELAY))


async def answer_callback_safe(query, text: str = None, show_alert: bool = False):
    """安全应答 callback query"""
    try:
        await query.answer(text=text, show_alert=show_alert)
    except Exception:
        pass


# ==================== 装饰器 ====================

def require_admin(func: Callable) -> Callable:
    """要求管理员权限（支持 command + callback）"""
    @wraps(func)
    async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE, *args, **kwargs):
        user_id = update.effective_user.id if update.effective_user else 0
        if not is_admin(user_id):
            if update.callback_query:
                await answer_callback_safe(update.callback_query, "⚠️ 此操作仅限管理员", show_alert=True)
            elif update.message:
                await update.message.reply_text("⚠️ 此命令仅限管理员使用")
            return
        return await func(update, context, *args, **kwargs)
    return wrapper


def require_private(func: Callable) -> Callable:
    """要求私聊；callback query 跳过检查"""
    @wraps(func)
    async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE, *args, **kwargs):
        if update.callback_query:
            return await func(update, context, *args, **kwargs)
        if not is_private(update):
            await redirect_to_private(update, context)
            return
        return await func(update, context, *args, **kwargs)
    return wrapper


def group_allowed(delete_after: int = 0, brief: bool = False):
    """允许群组使用，自动管理消息生命周期"""
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE, *args, **kwargs):
            kwargs['_is_group'] = is_group(update)
            kwargs['_brief'] = brief and is_group(update)
            result = await func(update, context, *args, **kwargs)
            if is_group(update) and delete_after > 0 and update.message:
                asyncio.create_task(safe_delete_message(update.message, delete_after))
            return result
        return wrapper
    return decorator


def require_registered(func: Callable) -> Callable:
    """要求已注册用户，自动注入 user"""
    @wraps(func)
    async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE, *args, **kwargs):
        if not update.effective_user:
            return
        user = await UserOperate.get_user_by_telegram_id(update.effective_user.id)
        if not user:
            msg = (
                "⚠️ 您尚未绑定账号\n\n"
                "请使用 /bind <绑定码> 绑定您的账号\n"
                "或使用 /reg <注册码> 注册新账号"
            )
            if update.callback_query:
                await answer_callback_safe(update.callback_query, "请先绑定或注册账号", show_alert=True)
            elif update.message:
                await update.message.reply_text(msg)
            return
        kwargs['user'] = user
        return await func(update, context, *args, **kwargs)
    return wrapper


def require_panel(func: Callable) -> Callable:
    """要求 TG 面板已开启（enable_tg_panel=true），管理员同样受限"""
    @wraps(func)
    async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE, *args, **kwargs):
        if TelegramConfig.ENABLE_TG_PANEL:
            return await func(update, context, *args, **kwargs)
        hint = "⚠️ TG 面板功能未开启，请在网页端操作"
        if update.callback_query:
            await answer_callback_safe(update.callback_query, hint, show_alert=True)
        elif update.message:
            await update.message.reply_text(hint)
        return
    return wrapper


def is_panel_enabled() -> bool:
    """检查 TG 面板是否启用"""
    return bool(TelegramConfig.ENABLE_TG_PANEL)


def require_subscribe(func: Callable) -> Callable:
    """要求订阅频道/加入群组"""
    @wraps(func)
    async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE, *args, **kwargs):
        if not TelegramConfig.FORCE_SUBSCRIBE:
            return await func(update, context, *args, **kwargs)
        user_id = update.effective_user.id if update.effective_user else 0
        if is_admin(user_id):
            return await func(update, context, *args, **kwargs)

        not_joined = []
        channel_ids = TelegramConfig.CHANNEL_ID
        if isinstance(channel_ids, (int, str)):
            channel_ids = [channel_ids] if channel_ids else []
        for cid in channel_ids:
            try:
                member = await context.bot.get_chat_member(cid, user_id)
                if member.status in ['left', 'kicked']:
                    label = str(cid)[1:] if str(cid).startswith('@') else str(cid)
                    not_joined.append(("📢 加入频道", f"https://t.me/{label}"))
            except Exception:
                pass

        group_ids = TelegramConfig.GROUP_ID
        if isinstance(group_ids, (int, str)):
            group_ids = [group_ids] if group_ids else []
        for gid in group_ids:
            try:
                member = await context.bot.get_chat_member(gid, user_id)
                if member.status in ['left', 'kicked']:
                    label = str(gid)[1:] if str(gid).startswith('@') else str(gid)
                    not_joined.append(("💬 加入群组", f"https://t.me/{label}"))
            except Exception:
                pass

        if not_joined:
            keyboard = InlineKeyboardMarkup([
                [InlineKeyboardButton(t, url=u)] for t, u in not_joined
            ])
            target = update.callback_query.message if update.callback_query else update.message
            if update.callback_query:
                await answer_callback_safe(update.callback_query, "请先加入频道和群组", show_alert=True)
            if target:
                await target.reply_text("🔔 请先加入以下频道/群组：", reply_markup=keyboard)
            return

        return await func(update, context, *args, **kwargs)
    return wrapper


# ==================== 工具函数 ====================

def escape_markdown(text: str) -> str:
    """转义 Markdown 特殊字符"""
    special_chars = ['_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!']
    for char in special_chars:
        text = text.replace(char, f'\\{char}')
    return text


def format_user_info(user, brief: bool = False) -> str:
    """格式化用户信息"""
    from src.core.utils import format_expire_time

    role_map = {
        Role.ADMIN.value: "👑 管理员",
        Role.WHITE_LIST.value: "⭐ 白名单",
        Role.NORMAL.value: "👤 普通用户",
    }

    if brief:
        return (
            f"👤 `{user.USERNAME}` | "
            f"{role_map.get(user.ROLE, '未知')} | "
            f"{'✅' if user.ACTIVE_STATUS else '❌'} | "
            f"{format_expire_time(user.EXPIRED_AT)}"
        )

    lines = [
        f"👤 **用户名**: `{user.USERNAME}`",
        f"🆔 **UID**: `{user.UID}`",
        f"🎬 **Emby**: {'已绑定' if user.EMBYID else '未绑定'}",
    ]
    if user.TELEGRAM_ID:
        lines.append(f"📱 **Telegram**: 已绑定")
    lines.append(f"👑 **角色**: {role_map.get(user.ROLE, '未知')}")
    lines.append(f"⏰ **到期时间**: {format_expire_time(user.EXPIRED_AT)}")
    lines.append(f"📊 **状态**: {'✅ 活跃' if user.ACTIVE_STATUS else '❌ 禁用'}")
    return "\n".join(lines)


# ==================== 公用键盘 ====================

def back_button(callback_data: str = "back_start", text: str = "♻️ 主菜单") -> InlineKeyboardButton:
    return InlineKeyboardButton(text, callback_data=callback_data)


def close_button() -> InlineKeyboardButton:
    return InlineKeyboardButton("❌ 关闭", callback_data="close_msg")


def main_menu_keyboard(user_id: int) -> InlineKeyboardMarkup:
    panel_on = is_panel_enabled()
    buttons = [
        [
            InlineKeyboardButton("👤 个人中心", callback_data="panel_user"),
        ],
    ]
    if panel_on:
        buttons[0].append(InlineKeyboardButton("💰 积分中心", callback_data="panel_score"))
        buttons.append([
            InlineKeyboardButton("🎬 Emby", callback_data="panel_emby"),
            InlineKeyboardButton("📋 帮助", callback_data="panel_help"),
        ])
    else:
        buttons.append([InlineKeyboardButton("📋 帮助", callback_data="panel_help")])
    if is_admin(user_id) and panel_on:
        buttons.append([InlineKeyboardButton("🔧 管理面板", callback_data="panel_admin")])
    return InlineKeyboardMarkup(buttons)

