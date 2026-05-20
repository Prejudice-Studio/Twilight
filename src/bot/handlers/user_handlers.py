"""
用户命令 + Inline 面板处理器

/start - 主菜单（inline 按钮）
/help  - 帮助
/bind  - 绑定 TG
/me    - 个人信息
"""

import asyncio
import logging
from typing import Any

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.ext import ContextTypes, CommandHandler, CallbackQueryHandler, MessageHandler, filters

from src.bot.handlers.common import (
    require_registered,
    require_subscribe,
    require_private,
    require_panel,
    format_user_info,
    escape_markdown,
    is_admin,
    is_panel_enabled,
    safe_edit_message,
    answer_callback_safe,
    main_menu_keyboard,
    back_button,
    close_button,
    redirect_to_private,
    is_group,
    safe_delete_message,
    GROUP_MSG_DELETE_DELAY,
)
from src.db.user import UserOperate, Role
from src.config import Config, TelegramConfig

logger = logging.getLogger(__name__)
BIND_STATE_KEY = "bind_wait_code"


def _render_custom_text(template: str) -> str:
    """把 ``{server_name}`` 之类的占位符替换成真实值。

    单一占位符当前只有 ``server_name``；以后要加占位符在这里扩展。
    模板为空时返回空串，调用方据此决定是否插入对应段落。
    """
    if not template:
        return ""
    try:
        return template.format(server_name=Config.SERVER_NAME or "Twilight")
    except (KeyError, IndexError, ValueError):
        # 模板里出现未支持的占位符时不要崩，直接原样返回
        return template


def register(bot):
    """注册处理器"""
    app = bot.application

    def _build_help_text(panel_on: bool, admin_mode: bool = False) -> str:
        custom_header = _render_custom_text(TelegramConfig.BOT_HELP_HEADER or "")
        custom_footer = _render_custom_text(TelegramConfig.BOT_HELP_FOOTER or "")

        lines: list[str] = []
        if custom_header:
            lines += [custom_header, ""]
        lines += [
            "📚 **普通命令**\n",
            "**👤 常用功能**",
            "• /start - 打开主菜单",
            "• /bind - 开始绑定 Telegram",
            "• /me - 查看个人信息",
            "• /twihelp - 查看普通帮助",
        ]

        if panel_on:
            lines += [
                "",
                "**🎬 Emby 功能**",
                "• /emby - 查看 Emby 服务状态",
                "• /playinfo - 查看播放统计",
                "",
                "💡 服务器线路请前往网页端查看",
            ]

        if admin_mode:
            lines += [
                "",
                "**🔧 管理员功能**",
                "管理员命令请发送 /twishelp",
            ]

        lines += [
            "",
            "🧭 输入型操作可随时使用 /cancel 取消",
            "⚠️ 密码重置、注册等敏感操作请在网页端进行",
        ]

        if custom_footer:
            lines += ["", custom_footer]
        return "\n".join(lines)

    # ======================== /start 主菜单 ========================

    async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """主菜单（群组中简短提示，私聊显示 inline 面板）"""
        if is_group(update):
            import asyncio

            bot_username = context.bot.username or ""
            kb = InlineKeyboardMarkup([[InlineKeyboardButton("📨 前往私聊", url=f"https://t.me/{bot_username}")]])
            reply = await update.message.reply_text("🌙 请在私聊中使用 Bot", reply_markup=kb)
            asyncio.create_task(safe_delete_message(update.message, GROUP_MSG_DELETE_DELAY))
            asyncio.create_task(safe_delete_message(reply, GROUP_MSG_DELETE_DELAY))
            return

        user_id = update.effective_user.id if update.effective_user else 0
        user_name = update.effective_user.first_name if update.effective_user else "用户"
        # 顺手刷新 Telegram username 缓存（仅在用户已绑定的情况下），admin 列表会用
        try:
            tg_username = getattr(update.effective_user, "username", None) if update.effective_user else None
            if user_id and tg_username:
                bound_user = await UserOperate.get_user_by_telegram_id(user_id)
                if bound_user:
                    from src.services import UserService

                    await UserService.cache_telegram_username(bound_user, tg_username)
        except Exception as exc:  # pragma: no cover
            logger.debug(f"刷新 Telegram username 缓存失败: {exc}")

        server_name = Config.SERVER_NAME or "Twilight"
        panel_on = is_panel_enabled()
        admin_mode = is_admin(user_id)

        custom_title = _render_custom_text(TelegramConfig.BOT_START_TITLE or "")
        custom_intro = _render_custom_text(TelegramConfig.BOT_START_INTRO or "")
        title_line = custom_title or f"🌙 **{server_name} 控制中心**"
        intro_line = custom_intro or "欢迎使用 Emby 管理机器人"

        if panel_on:
            text = (
                f"{title_line}\n\n"
                f"你好，**{escape_markdown(user_name)}**！\n"
                f"{intro_line}\n\n"
                f"🧭 推荐先点下方菜单按钮操作"
            )
            if admin_mode:
                text += "\n🔧 你拥有管理员权限，可发送 /admin"
            text += "\n\n请选择功能："
            await update.message.reply_text(
                text,
                reply_markup=main_menu_keyboard(user_id),
                parse_mode="Markdown",
            )
        else:
            text = (
                f"{title_line}\n\n"
                f"你好，**{escape_markdown(user_name)}**！\n"
                f"{intro_line}\n\n"
                "可用命令：\n"
                "• /start \\- 打开主菜单\n"
                "• /help \\- 帮助信息\n"
                "• /bind \\- 开始绑定 Telegram\n"
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

        custom_title = _render_custom_text(TelegramConfig.BOT_START_TITLE or "")
        title_line = custom_title or f"🌙 **{server_name}**"

        text = f"{title_line}\n\n" f"你好，**{escape_markdown(user_name)}**！\n" f"请选择功能："
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
        text = _build_help_text(panel_on=panel_on, admin_mode=is_admin(user_id))
        kb = InlineKeyboardMarkup([[back_button()]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    @require_private
    async def cmd_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """帮助命令"""
        panel_on = is_panel_enabled()
        text = _build_help_text(panel_on=panel_on, admin_mode=False)
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

        text = f"👤 **个人中心**\n\n" f"{format_user_info(user)}"

        panel_on = is_panel_enabled()
        buttons = []

        # TG 绑定信息按钮（始终可用）
        buttons.append(
            [
                InlineKeyboardButton("📱 TG 绑定", callback_data="user_tg_info"),
            ]
        )

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
        text = f"👤 **个人中心**\n\n" f"{format_user_info(user)}"
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
            text = f"📱 **Telegram 绑定信息**\n\n" f"✅ 已绑定 (ID: `{user.TELEGRAM_ID}`)\n"
            buttons = []
            # 仅面板开启且未强制绑定时允许解绑
            if panel_on and not Config.FORCE_BIND_TELEGRAM:
                buttons.append([InlineKeyboardButton("🔓 解绑 Telegram", callback_data="user_unbindtg_confirm")])
            buttons.append([InlineKeyboardButton("🔙 返回", callback_data="panel_user")])
        else:
            text = (
                f"📱 **Telegram 绑定信息**\n\n"
                f"❌ 未绑定\n\n"
                f"发送 `/bind` 后按提示输入绑定码\n"
                f"（可发送 /cancel 取消）"
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
        text = "✅ 已解绑 Telegram\n\n重新绑定请发送 /bind"
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

    async def _confirm_bind_via_api(bind_code: str, telegram_id: int) -> tuple[bool, str, dict[str, Any] | None, bool]:
        """优先通过 API 回调确认绑定，返回 (ok, message, data, should_fallback_internal)。"""
        import requests
        from src.config import SecurityConfig, APIConfig, TelegramConfig

        bot_secret = (SecurityConfig.BOT_INTERNAL_SECRET or "").strip()
        if not bot_secret:
            return False, "Bot 内部密钥未配置，无法通过 API 回调确认绑定", None, True

        api_urls = []
        custom_url = (TelegramConfig.BIND_CONFIRM_API_URL or "").strip()
        if custom_url:
            if custom_url.endswith("/api/v1/users/me/telegram/bind-confirm"):
                api_urls.append(custom_url)
            else:
                api_urls.append(f"{custom_url.rstrip('/')}/api/v1/users/me/telegram/bind-confirm")

        api_urls.extend(
            [
                f"http://127.0.0.1:{APIConfig.PORT}/api/v1/users/me/telegram/bind-confirm",
                f"http://localhost:{APIConfig.PORT}/api/v1/users/me/telegram/bind-confirm",
            ]
        )

        last_err = ""
        for api_url in api_urls:
            try:
                resp = await asyncio.to_thread(
                    requests.post,
                    api_url,
                    json={
                        "bind_code": bind_code,
                        "telegram_id": telegram_id,
                        "bot_secret": bot_secret,
                    },
                    timeout=8,
                )
                result = resp.json() if resp.content else {}

                if isinstance(result, dict):
                    ok = bool(result.get("success"))
                    message = str(result.get("message") or ("绑定成功" if ok else "绑定失败"))
                    data = result.get("data") if isinstance(result.get("data"), dict) else {}
                    # API 已给出业务结果时，不再回退内部逻辑，避免重复处理
                    return ok, message, data, False

                last_err = f"接口响应格式无效: {api_url}"
            except Exception as exc:
                last_err = f"调用失败 {api_url}: {exc}"

        return False, last_err or "API 回调不可用", None, True

    async def _confirm_bind_and_reply(update: Update, telegram_id: int, bind_code: str) -> bool:
        ok, message, d, should_fallback_internal = await _confirm_bind_via_api(bind_code, telegram_id)

        # 多 worker 且未启用共享存储时，绑定码可能落在其他 worker 的内存中；
        # 对“绑定码无效或已过期”做短重试，尽量命中正确 worker。
        if (not ok) and (not should_fallback_internal) and ("绑定码无效或已过期" in (message or "")):
            for _ in range(6):
                await asyncio.sleep(0.25)
                ok, message, d, should_fallback_internal = await _confirm_bind_via_api(bind_code, telegram_id)
                if ok or should_fallback_internal or ("绑定码无效或已过期" not in (message or "")):
                    break

        if should_fallback_internal:
            try:
                from src.api.v1.users import confirm_tg_bind_internal

                ok, message, d, _ = await confirm_tg_bind_internal(bind_code, telegram_id)
            except Exception as exc:
                logger.error("TG 绑定内部回退失败: %s", exc)
                ok = False
                message = f"绑定回调不可用，且内部回退失败: {exc}"
                d = {}

        d = d or {}
        if ok:
            try:
                from src.services.telegram_membership import TelegramMembershipService

                await TelegramMembershipService.check_user_in_groups(telegram_id, strict=False, update_roster=True)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"绑定成功后刷新 Telegram 花名册失败: {exc}")

            # 顺手把 Telegram username 缓存进 user.OTHER，admin 列表后续可以
            # 直接读，不必每次都打 bot.get_chat()（既慢也容易触发限流）。
            try:
                tg_username = getattr(update.effective_user, "username", None)
                if tg_username:
                    bound_user = await UserOperate.get_user_by_telegram_id(telegram_id)
                    if bound_user:
                        from src.services import UserService

                        await UserService.cache_telegram_username(bound_user, tg_username)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"缓存 Telegram username 失败: {exc}")

            # 注册绑定码验证成功时仅返回 telegram_id，这里给出更友好提示
            if not d.get("username"):
                await update.message.reply_text("✅ Telegram 绑定码验证成功！\n\n请返回网页继续提交注册。")
                return True

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
            return True

        if "绑定码无效或已过期" in (message or ""):
            await update.message.reply_text(
                "❌ 绑定失败: 绑定码无效或已过期\n\n"
                "请确认刚刚生成的是最新 8 位绑定码后重试。\n"
                "若你刚刚完成 /bind，请等待 1-2 秒后再提交一次，"
                "仍失败请重新生成绑定码。\n\n"
                "你可以重新发送绑定码，或发送 /cancel 取消。"
            )
        elif d.get("reason") == "not_in_required_group":
            missing = d.get("missing_groups") or []
            buttons = []
            for g in missing:
                if g.get("url"):
                    label = g.get("title") or g.get("id") or "加入群组"
                    buttons.append([InlineKeyboardButton(f"👥 加入 {label}", url=g["url"])])
            reply_markup = InlineKeyboardMarkup(buttons) if buttons else None
            await update.message.reply_text(
                f"❌ 绑定失败: 你尚未加入必需群组\n\n{message or ''}\n\n"
                "加入后请重新发送绑定码再试一次，或发送 /cancel 取消。",
                reply_markup=reply_markup,
            )
        else:
            await update.message.reply_text(
                f"❌ 绑定失败: {message or '未知错误'}\n\n请重新发送 8 位绑定码，或发送 /cancel 取消"
            )
        return False

    @require_private
    async def cmd_cancel(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """取消用户输入流程（包含绑定流程和管理员输入流程）"""
        uid = update.effective_user.id if update.effective_user else 0
        if not uid:
            return

        cancelled = []
        if context.user_data.pop(BIND_STATE_KEY, None):
            cancelled.append("绑定流程")

        try:
            from src.bot.handlers.admin_handlers import _clear_admin_state  # type: ignore

            if _clear_admin_state(uid):
                cancelled.append("管理员输入流程")
        except Exception:
            pass

        if cancelled:
            await update.message.reply_text(f"✅ 已取消: {'、'.join(cancelled)}")
        else:
            await update.message.reply_text("ℹ️ 当前没有进行中的输入流程")

    @require_private
    async def cmd_bind(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """通过绑定码绑定 Telegram"""
        if not update.effective_user:
            return

        telegram_id = update.effective_user.id
        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            await update.message.reply_text(
                f"⚠️ 您已绑定账号: `{existing.USERNAME}`\n" "如需更换，请在网页端操作",
                parse_mode="Markdown",
            )
            return

        if not context.args or len(context.args) < 1:
            context.user_data[BIND_STATE_KEY] = True
            await update.message.reply_text(
                "📨 请输入 8 位绑定码以完成绑定\n\n" "💡 获取方式: 网页端个人中心/注册页\n" "💡 取消流程: /cancel",
                parse_mode="Markdown",
            )
            return

        bind_code = context.args[0].strip().upper()
        if len(bind_code) != 8 or not bind_code.isalnum():
            context.user_data[BIND_STATE_KEY] = True
            await update.message.reply_text("❌ 绑定码格式不正确，请发送 8 位字母数字绑定码，或发送 /cancel 取消")
            return

        try:
            if await _confirm_bind_and_reply(update, telegram_id, bind_code):
                context.user_data.pop(BIND_STATE_KEY, None)
            else:
                context.user_data[BIND_STATE_KEY] = True
        except Exception as e:
            logger.error(f"TG 绑定回调失败: {e}")
            context.user_data[BIND_STATE_KEY] = True
            await update.message.reply_text(
                "❌ 绑定失败，请稍后重试或联系管理员。你也可以重新发送绑定码，或发送 /cancel 取消"
            )

    @require_private
    async def handle_bind_text(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """处理绑定流程中的文本输入（/bind 后发送绑定码）。"""
        if not update.effective_user or not update.message:
            return

        telegram_id = update.effective_user.id
        if not context.user_data.get(BIND_STATE_KEY):
            return

        text = (update.message.text or "").strip()
        if not text:
            return

        if text.lower() in {"/cancel", "cancel", "取消", "返回"}:
            context.user_data.pop(BIND_STATE_KEY, None)
            await update.message.reply_text("✅ 已取消绑定流程")
            return

        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            context.user_data.pop(BIND_STATE_KEY, None)
            await update.message.reply_text(
                f"⚠️ 您已绑定账号: `{existing.USERNAME}`\n" "如需更换，请在网页端操作",
                parse_mode="Markdown",
            )
            return

        bind_code = text.upper()
        if bind_code.startswith("/BIND"):
            parts = bind_code.split()
            bind_code = parts[1].strip().upper() if len(parts) > 1 else ""

        if len(bind_code) != 8 or not bind_code.isalnum():
            await update.message.reply_text("❌ 绑定码格式不正确，请发送 8 位字母数字绑定码，或发送 /cancel 取消")
            return

        try:
            if await _confirm_bind_and_reply(update, telegram_id, bind_code):
                context.user_data.pop(BIND_STATE_KEY, None)
        except Exception as e:
            logger.error(f"TG 绑定处理失败: {e}")
            await update.message.reply_text(
                "❌ 绑定失败，请稍后重试或联系管理员。你也可以重新发送绑定码，或发送 /cancel 取消"
            )

    # ======================== 注册处理器 ========================

    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("help", cmd_help))
    app.add_handler(CommandHandler("twihelp", cmd_help))
    app.add_handler(CommandHandler("me", cmd_me))
    app.add_handler(CommandHandler("bind", cmd_bind))
    app.add_handler(CommandHandler("cancel", cmd_cancel))

    # 主菜单 & 导航
    app.add_handler(CallbackQueryHandler(cb_back_start, pattern="^back_start$"))
    app.add_handler(CallbackQueryHandler(cb_close_msg, pattern="^close_msg$"))
    app.add_handler(CallbackQueryHandler(cb_panel_help, pattern="^panel_help$"))

    # 个人中心
    app.add_handler(CallbackQueryHandler(cb_panel_user, pattern="^panel_user$"))
    app.add_handler(CallbackQueryHandler(cb_user_tg_info, pattern="^user_tg_info$"))
    app.add_handler(CallbackQueryHandler(cb_user_unbindtg_confirm, pattern="^user_unbindtg_confirm$"))
    app.add_handler(CallbackQueryHandler(cb_user_playinfo, pattern="^user_playinfo$"))

    # 文本消息（bind 状态机）- 在 admin 文本处理之后执行
    app.add_handler(
        MessageHandler(filters.TEXT & ~filters.COMMAND & filters.ChatType.PRIVATE, handle_bind_text), group=2
    )
