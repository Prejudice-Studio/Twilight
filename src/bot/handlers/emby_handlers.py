"""
Emby 服务 + Inline 面板处理器

/emby - Emby 信息
/lines - 线路信息
/playinfo - 播放统计
/playrank - 播放排行

注意：密码重置等敏感操作已移至网页端
"""
import logging

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.ext import ContextTypes, CommandHandler, CallbackQueryHandler

from src.bot.handlers.common import (
    require_registered, require_subscribe, require_admin, require_private,
    require_panel, safe_edit_message, answer_callback_safe, back_button, close_button,
)
from src.db.user import UserOperate, Role
from src.services.emby_service import EmbyService
from src.services.stats_service import StatsService
from src.config import EmbyConfig

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.application

    # ======================== Emby 面板入口 ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_panel_emby(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """Emby 面板回调"""
        query = update.callback_query
        await answer_callback_safe(query)

        try:
            status = await EmbyService.get_server_status()
            status_text = (
                f"📊 状态: ✅ 在线\n"
                f"🏷️ 名称: {status.get('server_name', '未知')}\n"
                f"📌 版本: {status.get('version', '未知')}"
            )
        except Exception:
            status_text = "📊 状态: ❌ 离线"

        text = f"🎬 **Emby 服务**\n\n{status_text}"
        await safe_edit_message(query.message, text, reply_markup=_emby_menu_kb())

    # ======================== 线路信息 ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_emby_lines(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """线路信息"""
        query = update.callback_query
        await answer_callback_safe(query)
        text = _format_lines_text(user)
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="panel_emby")]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 重置密码（已禁用，引导到网页端） ========================

    @require_subscribe
    @require_registered
    async def cb_emby_resetpwd(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """重置密码 - 引导到网页端"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔙 返回", callback_data="panel_emby")],
        ])
        await safe_edit_message(
            query.message,
            "🔒 **密码重置已移至网页端**\n\n"
            "出于安全考虑，密码重置等敏感操作请在网页端「个人设置」中进行。",
            reply_markup=kb,
        )

    # ======================== 播放统计 ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_emby_playinfo(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """播放统计"""
        query = update.callback_query
        await answer_callback_safe(query)

        stats = await StatsService.get_user_stats(user.UID)
        if not stats:
            text = "📊 暂无播放记录"
        else:
            text = (
                f"📊 **播放统计**\n\n"
                f"👤 用户: `{stats['username']}`\n\n"
                f"📈 **总计**\n"
                f"• 时长: {stats['total']['duration_str']}\n"
                f"• 次数: {stats['total']['play_count']} 次\n\n"
                f"📅 **今日**\n"
                f"• 时长: {stats['today']['duration_str']}\n"
                f"• 次数: {stats['today']['play_count']} 次"
            )

        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔄 刷新", callback_data="emby_playinfo")],
            [InlineKeyboardButton("🔙 返回", callback_data="panel_emby")],
        ])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 播放排行 ========================

    @require_panel
    @require_subscribe
    async def cb_emby_playrank(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """播放排行入口"""
        query = update.callback_query
        await answer_callback_safe(query)
        await _show_playrank(query.message, "today")

    async def cb_playrank_period(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """播放排行切换周期"""
        query = update.callback_query
        await answer_callback_safe(query)
        period = query.data.replace("playrank_", "")
        if period not in ("today", "week", "month", "all"):
            period = "today"
        await _show_playrank(query.message, period)

    # ======================== 传统命令（兼容） ========================

    @require_private
    @require_panel
    @require_subscribe
    async def cmd_emby(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """Emby 服务器信息"""
        try:
            status = await EmbyService.get_server_status()
            text = (
                f"🎬 **Emby 服务器**\n\n"
                f"📊 状态: ✅ 在线\n"
                f"🏷️ 名称: {status.get('server_name', '未知')}\n"
                f"📌 版本: {status.get('version', '未知')}"
            )
        except Exception:
            text = "🎬 **Emby 服务器**\n\n📊 状态: ❌ 离线"
        await update.message.reply_text(text, parse_mode="Markdown")

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_lines(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """线路信息"""
        text = _format_lines_text(user)
        await update.message.reply_text(text, parse_mode="Markdown")

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_resetpwd(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """重置密码 - 引导到网页端"""
        await update.message.reply_text(
            "\ud83d\udd12 **密码重置已移至网页端**\n\n"
            "出于安全考虑，请在网页端「个人设置」中进行密码重置。",
            parse_mode="Markdown",
        )

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_playinfo(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """播放统计"""
        stats = await StatsService.get_user_stats(user.UID)
        if not stats:
            await update.message.reply_text("📊 暂无播放记录")
            return
        text = (
            f"📊 **播放统计**\n\n"
            f"📈 总计: {stats['total']['duration_str']} ({stats['total']['play_count']}次)\n"
            f"📅 今日: {stats['today']['duration_str']} ({stats['today']['play_count']}次)"
        )
        await update.message.reply_text(text, parse_mode="Markdown")

    @require_private
    @require_panel
    @require_subscribe
    async def cmd_playrank(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """播放排行"""
        period = context.args[0] if context.args else "today"
        if period not in ("today", "week", "month", "all"):
            period = "today"

        period_names = {"today": "今日", "week": "本周", "month": "本月", "all": "总"}
        ranking = await StatsService.get_ranking(period=period, limit=10)
        if not ranking:
            await update.message.reply_text("📊 暂无排行数据")
            return

        lines = [f"🏆 **{period_names[period]}播放排行榜**\n"]
        medals = ["🥇", "🥈", "🥉"]
        for item in ranking:
            medal = medals[item['rank'] - 1] if item['rank'] <= 3 else f"{item['rank']}."
            lines.append(f"{medal} `{item['username']}` - {item['value_str']}")

        kb = InlineKeyboardMarkup([[
            InlineKeyboardButton("今日", callback_data="playrank_today"),
            InlineKeyboardButton("本周", callback_data="playrank_week"),
            InlineKeyboardButton("本月", callback_data="playrank_month"),
            InlineKeyboardButton("总榜", callback_data="playrank_all"),
        ]])
        await update.message.reply_text("\n".join(lines), reply_markup=kb, parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_sessions(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """查看活跃会话"""
        try:
            sessions = await EmbyService.get_all_sessions()
            if not sessions:
                await update.message.reply_text("📺 当前没有活跃会话")
                return
            lines = [f"📺 **活跃会话** ({len(sessions)} 个)\n"]
            for s in sessions[:10]:
                name = s.get('user_name', '未知')
                dev = s.get('device_name', '?')
                np = s.get('now_playing', {})
                if np:
                    lines.append(f"• **{name}** @ {dev}\n  ▶️ {np.get('name', '?')}")
                else:
                    lines.append(f"• **{name}** @ {dev} (空闲)")
            await update.message.reply_text("\n".join(lines), parse_mode="Markdown")
        except Exception as e:
            await update.message.reply_text(f"❌ {e}")

    @require_private
    @require_admin
    async def cmd_kick(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """踢出用户会话"""
        if not context.args:
            await update.message.reply_text("用法: `/kick <用户名>`", parse_mode="Markdown")
            return
        user = await UserOperate.get_user_by_username(context.args[0])
        if not user or not user.EMBYID:
            await update.message.reply_text("❌ 用户不存在或未绑定 Emby")
            return
        try:
            success, count = await EmbyService.kick_user_sessions(user)
            await update.message.reply_text(f"✅ 已踢出 `{context.args[0]}` 的 {count} 个会话", parse_mode="Markdown")
        except Exception as e:
            await update.message.reply_text(f"❌ {e}")

    # ======================== 注册处理器 ========================

    # 命令
    app.add_handler(CommandHandler("emby", cmd_emby))
    app.add_handler(CommandHandler("lines", cmd_lines))
    app.add_handler(CommandHandler("resetpwd", cmd_resetpwd))
    app.add_handler(CommandHandler("playinfo", cmd_playinfo))
    app.add_handler(CommandHandler("playrank", cmd_playrank))
    app.add_handler(CommandHandler("sessions", cmd_sessions))
    app.add_handler(CommandHandler("kick", cmd_kick))

    # 面板回调
    app.add_handler(CallbackQueryHandler(cb_panel_emby, pattern="^panel_emby$"))
    app.add_handler(CallbackQueryHandler(cb_emby_lines, pattern="^emby_lines$"))
    app.add_handler(CallbackQueryHandler(cb_emby_resetpwd, pattern="^emby_resetpwd$"))
    app.add_handler(CallbackQueryHandler(cb_emby_playinfo, pattern="^emby_playinfo$"))
    app.add_handler(CallbackQueryHandler(cb_emby_playrank, pattern="^emby_playrank$"))
    app.add_handler(CallbackQueryHandler(cb_playrank_period, pattern=r"^playrank_"))


# ======================== 辅助函数 ========================

def _emby_menu_kb() -> InlineKeyboardMarkup:
    """Emby 面板键盘"""
    return InlineKeyboardMarkup([
        [
            InlineKeyboardButton("🌐 线路信息", callback_data="emby_lines"),
            InlineKeyboardButton("📊 播放统计", callback_data="emby_playinfo"),
        ],
        [
            InlineKeyboardButton("🏆 播放排行", callback_data="emby_playrank"),
        ],
        [InlineKeyboardButton("♻️ 主菜单", callback_data="back_start")],
    ])


def _parse_line_entry(entry: str) -> tuple:
    """解析 'Name : URL' 格式的线路条目"""
    if ' : ' in entry:
        parts = entry.split(' : ', 1)
        return parts[0].strip(), parts[1].strip()
    return '默认线路', entry.strip()


def _format_lines_text(user=None) -> str:
    """格式化线路信息文本，根据用户角色显示不同线路"""
    lines_list = EmbyConfig.EMBY_URL_LIST
    if not lines_list:
        return "⚠️ 暂无可用线路"

    parts = ["🌐 **可用线路**\n"]
    for i, entry in enumerate(lines_list, 1):
        name, url = _parse_line_entry(entry)
        parts.append(f"{i}. **{name}**\n   `{url}`")

    # 白名单/管理员用户显示专属线路
    if user and hasattr(user, 'ROLE') and user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
        wl_list = EmbyConfig.EMBY_URL_LIST_FOR_WHITELIST
        if wl_list:
            parts.append("\n⭐ **专属线路**\n")
            for i, entry in enumerate(wl_list, 1):
                name, url = _parse_line_entry(entry)
                parts.append(f"{i}. **{name}**\n   `{url}`")

    parts.append("\n💡 请选择延迟最低的线路使用")
    return "\n".join(parts)


async def _show_playrank(message, period: str):
    """显示播放排行"""
    period_names = {"today": "今日", "week": "本周", "month": "本月", "all": "总"}
    ranking = await StatsService.get_ranking(period=period, limit=10)

    if not ranking:
        text = "📊 暂无排行数据"
    else:
        lines = [f"🏆 **{period_names.get(period, '今日')}播放排行榜**\n"]
        medals = ["🥇", "🥈", "🥉"]
        for item in ranking:
            medal = medals[item['rank'] - 1] if item['rank'] <= 3 else f"{item['rank']}."
            lines.append(f"{medal} `{item['username']}` - {item['value_str']}")
        text = "\n".join(lines)

    kb = InlineKeyboardMarkup([
        [
            InlineKeyboardButton("今日", callback_data="playrank_today"),
            InlineKeyboardButton("本周", callback_data="playrank_week"),
            InlineKeyboardButton("本月", callback_data="playrank_month"),
            InlineKeyboardButton("总榜", callback_data="playrank_all"),
        ],
        [InlineKeyboardButton("🔙 Emby", callback_data="panel_emby")],
    ])
    await safe_edit_message(message, text, reply_markup=kb)
