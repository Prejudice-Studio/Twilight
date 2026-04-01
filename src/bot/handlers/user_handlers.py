"""
用户命令 + Inline 面板处理器

/start - 主菜单（inline 按钮）
/help  - 帮助
/bind  - 绑定 TG
/me    - 个人信息
"""
import logging

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.ext import ContextTypes, CommandHandler, CallbackQueryHandler

from src.bot.handlers.common import (
    require_registered, require_subscribe, require_private, require_panel,
    format_user_info, escape_markdown, is_admin, is_panel_enabled,
    safe_edit_message, answer_callback_safe, main_menu_keyboard,
    back_button, close_button, redirect_to_private, is_group,
    safe_delete_message, GROUP_MSG_DELETE_DELAY,
)
from src.db.user import UserOperate, Role
from src.config import Config, ScoreAndRegisterConfig

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.application

    # ======================== /start 主菜单 ========================

    async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """主菜单（群组中简短提示，私聊显示 inline 面板）"""
        if is_group(update):
            import asyncio
            bot_username = context.bot.username or ""
            kb = InlineKeyboardMarkup([
                [InlineKeyboardButton("📨 前往私聊", url=f"https://t.me/{bot_username}")]
            ])
            reply = await update.message.reply_text("🌙 请在私聊中使用 Bot", reply_markup=kb)
            asyncio.create_task(safe_delete_message(update.message, GROUP_MSG_DELETE_DELAY))
            asyncio.create_task(safe_delete_message(reply, GROUP_MSG_DELETE_DELAY))
            return

        user_id = update.effective_user.id if update.effective_user else 0
        user_name = update.effective_user.first_name if update.effective_user else "用户"
        server_name = Config.SERVER_NAME or "Twilight"
        panel_on = is_panel_enabled()

        if panel_on:
            text = (
                f"🌙 **{server_name}**\n\n"
                f"你好，**{escape_markdown(user_name)}**！\n"
                f"欢迎使用 Emby 管理机器人\n\n"
                f"请选择功能："
            )
            await update.message.reply_text(
                text,
                reply_markup=main_menu_keyboard(user_id),
                parse_mode="Markdown",
            )
        else:
            text = (
                f"🌙 **{server_name}**\n\n"
                f"你好，**{escape_markdown(user_name)}**！\n"
                f"欢迎使用 Emby 管理机器人\n\n"
                "可用命令：\n"
                "• /help \\- 帮助信息\n"
                "• /bind <绑定码> \\- 绑定 Telegram\n"
                "• /me \\- 查看个人信息"
            )
            await update.message.reply_text(text, parse_mode="Markdown")

    async def cb_back_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """回到主菜单（仅面板开启或管理员）"""
        query = update.callback_query
        await answer_callback_safe(query)
        user_id = update.effective_user.id if update.effective_user else 0
        if not is_panel_enabled():
            await safe_edit_message(query.message, "⚠️ TG 面板未开启\n\n可用命令: /help /bind /me")
            return
        user_id = update.effective_user.id if update.effective_user else 0
        user_name = update.effective_user.first_name if update.effective_user else "用户"
        server_name = Config.SERVER_NAME or "Twilight"

        text = (
            f"🌙 **{server_name}**\n\n"
            f"你好，**{escape_markdown(user_name)}**！\n"
            f"请选择功能："
        )
        await safe_edit_message(query.message, text, reply_markup=main_menu_keyboard(user_id))

    async def cb_close_msg(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """关闭/删除消息"""
        query = update.callback_query
        await answer_callback_safe(query)
        try:
            await query.message.delete()
        except Exception:
            pass

    # ======================== 帮助面板 ========================

    async def cb_panel_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """帮助面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        user_id = update.effective_user.id if update.effective_user else 0
        panel_on = is_panel_enabled()
        if not panel_on:
            await safe_edit_message(query.message, "⚠️ TG 面板未开启\n\n可用命令: /help /bind /me")
            return
        lines = [
            "📋 **帮助**\n",
            "**👤 基础功能**",
            "• /start - 主菜单",
            "• /bind <绑定码> - 绑定 Telegram",
            "• /me - 查看个人信息",
        ]
        if panel_on:
            lines += [
                "",
                "**💰 积分**",
                "• /checkin - 每日签到",
                "• /transfer <用户名> <金额> - 转账",
                "",
                "**🎬 Emby**",
                "• /lines - 查看线路",
            ]
        lines += [
            "",
            "⚠️ 密码重置、注册等敏感操作请在网页端进行",
            "",
            "💡 大部分功能可通过主菜单按钮操作",
        ]
        text = "\n".join(lines)
        kb = InlineKeyboardMarkup([[back_button()]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    @require_private
    async def cmd_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """帮助命令"""
        panel_on = is_panel_enabled()
        lines = [
            "📋 **帮助**\n",
            "**👤 基础功能**",
            "• /start - 主菜单",
            "• /bind <绑定码> - 绑定 Telegram",
            "• /me - 查看个人信息",
        ]
        if panel_on:
            lines += [
                "",
                "**💰 积分**",
                "• /checkin - 每日签到",
                "",
                "**🎬 Emby**",
                "• /lines - 查看线路",
            ]
        lines += [
            "",
            "⚠️ 密码重置、注册等敏感操作请在网页端进行",
        ]
        text = "\n".join(lines)
        if panel_on:
            kb = InlineKeyboardMarkup([[back_button()]])
            await update.message.reply_text(text, reply_markup=kb, parse_mode="Markdown")
        else:
            await update.message.reply_text(text, parse_mode="Markdown")

    # ======================== 个人中心面板 ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_panel_user(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None, **kwargs):
        """个人中心面板（需要面板开启）"""
        query = update.callback_query
        await answer_callback_safe(query)

        from src.db.score import ScoreOperate
        score = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score.SCORE if score else 0
        score_name = ScoreAndRegisterConfig.SCORE_NAME

        text = (
            f"👤 **个人中心**\n\n"
            f"{format_user_info(user)}\n"
            f"💰 **{score_name}**: {balance}"
        )

        panel_on = is_panel_enabled()
        buttons = []

        # TG 绑定信息按钮（始终可用）
        buttons.append([
            InlineKeyboardButton("📱 TG 绑定", callback_data="user_tg_info"),
        ])

        # 播放统计仅在面板开启时可用
        if panel_on and user.EMBYID:
            buttons.append([InlineKeyboardButton("📊 播放统计", callback_data="user_playinfo")])

        buttons.append([back_button()])
        await safe_edit_message(query.message, text, reply_markup=InlineKeyboardMarkup(buttons))

    @require_private
    @require_subscribe
    @require_registered
    async def cmd_me(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None, **kwargs):
        """查看个人信息（命令版，始终可用）"""
        from src.db.score import ScoreOperate
        score = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score.SCORE if score else 0
        score_name = ScoreAndRegisterConfig.SCORE_NAME
        text = (
            f"👤 **个人中心**\n\n"
            f"{format_user_info(user)}\n"
            f"💰 **{score_name}**: {balance}"
        )
        user_id = update.effective_user.id if update.effective_user else 0
        panel_on = is_panel_enabled()
        if panel_on:
            buttons = [
                [InlineKeyboardButton("📱 TG 信息", callback_data="user_tg_info")],
                [back_button()],
            ]
            await update.message.reply_text(text, reply_markup=InlineKeyboardMarkup(buttons), parse_mode="Markdown")
        else:
            await update.message.reply_text(text, parse_mode="Markdown")

    # ---- TG 绑定信息 callback ----

    @require_registered
    async def cb_user_tg_info(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None, **kwargs):
        """TG 绑定信息"""
        query = update.callback_query
        await answer_callback_safe(query)

        panel_on = is_panel_enabled()

        if user.TELEGRAM_ID:
            text = (
                f"📱 **Telegram 绑定信息**\n\n"
                f"✅ 已绑定 (ID: `{user.TELEGRAM_ID}`)\n"
            )
            buttons = []
            # 仅面板开启且未强制绑定时允许解绑
            if panel_on and not Config.FORCE_BIND_TELEGRAM:
                buttons.append([InlineKeyboardButton("🔓 解绑 Telegram", callback_data="user_unbindtg_confirm")])
            buttons.append([InlineKeyboardButton("🔙 返回", callback_data="panel_user")])
        else:
            text = (
                f"📱 **Telegram 绑定信息**\n\n"
                f"❌ 未绑定\n\n"
                f"发送 `/bind <绑定码>` 进行绑定\n"
                f"（绑定码请在网页端获取）"
            )
            buttons = [[InlineKeyboardButton("🔙 返回", callback_data="panel_user")]]

        await safe_edit_message(query.message, text, reply_markup=InlineKeyboardMarkup(buttons))

    @require_panel
    @require_registered
    async def cb_user_unbindtg_confirm(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None, **kwargs):
        """确认解绑 TG（需要面板开启）"""
        query = update.callback_query
        await answer_callback_safe(query)
        if Config.FORCE_BIND_TELEGRAM:
            await answer_callback_safe(query, "⚠️ 系统要求强制绑定，无法解绑", show_alert=True)
            return
        user.TELEGRAM_ID = None
        await UserOperate.update_user(user)
        logger.info(f"用户 {user.USERNAME} 解绑 Telegram")
        text = "✅ 已解绑 Telegram\n\n重新绑定请使用 /bind <绑定码>"
        kb = InlineKeyboardMarkup([[back_button()]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ---- 播放统计（需要面板开启） ----

    @require_panel
    @require_registered
    async def cb_user_playinfo(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None, **kwargs):
        """播放统计"""
        query = update.callback_query
        await answer_callback_safe(query)

        from src.services.stats_service import StatsService
        stats = await StatsService.get_user_stats(user.UID)
        if not stats:
            text = "📊 暂无播放记录"
        else:
            text = (
                f"📊 **播放统计**\n\n"
                f"👤 用户: `{stats['username']}`\n\n"
                f"**📈 总计**\n"
                f"• 时长: {stats['total']['duration_str']}\n"
                f"• 次数: {stats['total']['play_count']} 次\n\n"
                f"**📅 今日**\n"
                f"• 时长: {stats['today']['duration_str']}\n"
                f"• 次数: {stats['today']['play_count']} 次"
            )
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="panel_user")]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 绑定命令（始终可用） ========================

    @require_private
    async def cmd_bind(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """通过绑定码绑定 Telegram"""
        if not update.effective_user:
            return

        telegram_id = update.effective_user.id
        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            await update.message.reply_text(
                f"⚠️ 您已绑定账号: `{existing.USERNAME}`\n"
                "如需更换，请在网页端操作",
                parse_mode="Markdown",
            )
            return

        if not context.args or len(context.args) < 1:
            await update.message.reply_text(
                "❌ 请提供绑定码\n\n"
                "用法: `/bind <绑定码>`\n\n"
                "请先在网页端「个人设置」中获取 6 位绑定码。",
                parse_mode="Markdown",
            )
            return

        bind_code = context.args[0].strip()

        import httpx
        from src.config import TelegramConfig, APIConfig
        bot_secret = TelegramConfig.BOT_TOKEN[:20] if TelegramConfig.BOT_TOKEN else ''
        port = getattr(APIConfig, 'PORT', 5000)
        api_url = f"http://127.0.0.1:{port}/api/v1/users/me/telegram/bind-confirm"

        try:
            transport = httpx.AsyncHTTPTransport(local_address='0.0.0.0')
            async with httpx.AsyncClient(timeout=10, transport=transport) as http_client:
                resp = await http_client.post(api_url, json={
                    'bind_code': bind_code,
                    'telegram_id': telegram_id,
                    'bot_secret': bot_secret,
                })
                result = resp.json()
                if result.get('success'):
                    d = result.get('data', {})
                    info_lines = [
                        "✅ **绑定成功！**\n",
                        f"👤 **用户名**: `{d.get('username', '')}`",
                        f"👑 **角色**: {d.get('role', '未知')}",
                        f"📊 **状态**: {'✅ 活跃' if d.get('active') else '❌ 禁用'}",
                        f"⏰ **到期**: {d.get('expired_at', '未知')}",
                        f"🎬 **Emby**: {'已绑定' if d.get('emby_id') else '未绑定'}",
                        "\n💡 发送 /start 打开主菜单",
                    ]
                    await update.message.reply_text("\n".join(info_lines), parse_mode="Markdown")
                else:
                    await update.message.reply_text(f"❌ 绑定失败: {result.get('message', '未知错误')}")
        except Exception as e:
            logger.error(f"TG 绑定回调失败: {e}")
            await update.message.reply_text("❌ 绑定失败，请稍后重试或联系管理员")

    # ======================== 注册处理器 ========================

    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("help", cmd_help))
    app.add_handler(CommandHandler("me", cmd_me))
    app.add_handler(CommandHandler("bind", cmd_bind))

    # 主菜单 & 导航
    app.add_handler(CallbackQueryHandler(cb_back_start, pattern="^back_start$"))
    app.add_handler(CallbackQueryHandler(cb_close_msg, pattern="^close_msg$"))
    app.add_handler(CallbackQueryHandler(cb_panel_help, pattern="^panel_help$"))

    # 个人中心
    app.add_handler(CallbackQueryHandler(cb_panel_user, pattern="^panel_user$"))
    app.add_handler(CallbackQueryHandler(cb_user_tg_info, pattern="^user_tg_info$"))
    app.add_handler(CallbackQueryHandler(cb_user_unbindtg_confirm, pattern="^user_unbindtg_confirm$"))
    app.add_handler(CallbackQueryHandler(cb_user_playinfo, pattern="^user_playinfo$"))

