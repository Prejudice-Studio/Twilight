"""
管理员命令 + Inline 面板处理器

/admin - 管理面板（inline 按钮）
支持用户管理、注册码、统计、Emby、广播等
"""

import logging
import secrets
import time

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.ext import ContextTypes, CommandHandler, CallbackQueryHandler, MessageHandler, filters

from src.bot.handlers.common import (
    require_admin,
    require_private,
    format_user_info,
    is_admin,
    safe_edit_message,
    answer_callback_safe,
    back_button,
    close_button,
)
from src.db.user import UserOperate, Role
from src.db.regcode import RegCodeOperate
from src.services.user_service import UserService, RegisterResult
from src.services.emby_service import EmbyService
from src.services.emby import get_emby_client
from src.core.utils import generate_random_string, generate_password, days_to_seconds, timestamp
from src.config import Config, TelegramConfig, RegisterConfig

logger = logging.getLogger(__name__)

# 会话状态存储（admin_id -> state dict）
_admin_states = {}
_ADMIN_STATE_TTL = 15 * 60
_group_admin_contexts: dict[str, dict] = {}
_GROUP_ADMIN_CONTEXT_TTL = 10 * 60


def _set_admin_state(uid: int, state: dict):
    payload = dict(state)
    payload["_ts"] = int(time.time())
    _admin_states[uid] = payload


def _get_admin_state(uid: int):
    state = _admin_states.get(uid)
    if not state:
        return None

    ts = int(state.get("_ts", 0))
    if ts <= 0 or int(time.time()) - ts > _ADMIN_STATE_TTL:
        _admin_states.pop(uid, None)
        return None

    payload = dict(state)
    payload.pop("_ts", None)
    return payload


def _clear_admin_state(uid: int) -> bool:
    return _admin_states.pop(uid, None) is not None


def _new_group_admin_context(payload: dict) -> str:
    now = int(time.time())
    stale = [k for k, v in _group_admin_contexts.items() if now - int(v.get("ts", 0)) > _GROUP_ADMIN_CONTEXT_TTL]
    for key in stale:
        _group_admin_contexts.pop(key, None)
    token = secrets.token_urlsafe(8)
    _group_admin_contexts[token] = {**payload, "ts": now}
    return token


def _get_group_admin_context(token: str) -> dict | None:
    payload = _group_admin_contexts.get(token)
    if not payload:
        return None
    if int(time.time()) - int(payload.get("ts", 0)) > _GROUP_ADMIN_CONTEXT_TTL:
        _group_admin_contexts.pop(token, None)
        return None
    return payload


def _is_anonymous_group_command(update: Update) -> bool:
    message = update.message
    return bool(message and message.sender_chat and (not update.effective_user or not is_admin(update.effective_user.id)))


def _render_custom_text(template: str) -> str:
    if not template:
        return ""
    try:
        return template.format(server_name=Config.SERVER_NAME or "Twilight")
    except (KeyError, IndexError, ValueError):
        return template


def register(bot):
    """注册处理器"""
    app = bot.application

    # ======================== 管理面板入口 ========================

    @require_private
    @require_admin
    async def cmd_twishelp(update: Update, context: ContextTypes.DEFAULT_TYPE):
        text = _render_custom_text(TelegramConfig.BOT_ADMIN_HELP_TEXT or "") or (
            "🔧 **管理员命令**\n\n"
            "• /admin - 打开管理面板\n"
            "• /twfind <关键词> - 按系统用户名/UID/TGID/TG用户名/Emby标识搜索\n"
            "• /twguser <用户> - 群组内查询用户；也可回复某人发送 /twguser\n"
            "• /twbindcheck [关键词] - 检查指定用户或全局 TG 绑定状态\n"
            "• /twforcebind <用户> <TGID> - 强制绑定 TG 到系统用户\n"
            "• /twsyncuser <用户> - 同步用户状态到 Emby\n"
            "• /stats - 系统统计\n"
            "• /cancel - 取消输入流程\n\n"
            "不会通过 Bot 展示线路、密码、Emby 用户名等隐私信息。"
        )
        await update.message.reply_text(text, parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_admin(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """管理面板命令"""
        await update.message.reply_text(
            "🔧 **管理面板**\n\n请选择功能：\n💡 输入型操作可发送 /cancel 取消",
            reply_markup=_admin_menu_kb(),
            parse_mode="Markdown",
        )

    @require_private
    @require_admin
    async def cmd_cancel(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """取消当前管理员输入流程"""
        uid = update.effective_user.id
        had_state = _clear_admin_state(uid)
        if had_state:
            await update.message.reply_text(
                "✅ 已取消当前操作\n\n可发送 /admin 返回管理面板",
                parse_mode="Markdown",
            )
        else:
            await update.message.reply_text("ℹ️ 当前没有进行中的输入流程")

    async def cb_panel_admin(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """管理面板回调入口"""
        query = update.callback_query
        user_id = update.effective_user.id if update.effective_user else 0
        if not is_admin(user_id):
            await answer_callback_safe(query, "⚠️ 仅限管理员", show_alert=True)
            return
        await answer_callback_safe(query)
        await safe_edit_message(query.message, "🔧 **管理面板**\n\n请选择功能：", reply_markup=_admin_menu_kb())

    # ======================== 用户管理 ========================

    @require_admin
    async def cb_admin_users(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """用户管理面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup(
            [
                [
                    InlineKeyboardButton("➕ 添加用户", callback_data="adm_adduser"),
                    InlineKeyboardButton("🔍 查询用户", callback_data="adm_queryuser"),
                ],
                [
                    InlineKeyboardButton("👥 用户列表", callback_data="adm_userlist:1"),
                    InlineKeyboardButton("🚫 禁用/解禁", callback_data="adm_banmenu"),
                ],
                [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
            ]
        )
        await safe_edit_message(query.message, "👥 **用户管理**\n\n请选择操作：", reply_markup=kb)

    @require_admin
    async def cb_adm_userlist(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """用户列表（分页）"""
        query = update.callback_query
        await answer_callback_safe(query)

        page = 1
        if ":" in query.data:
            try:
                page = int(query.data.split(":")[1])
            except ValueError:
                page = 1

        per_page = 8
        offset = (page - 1) * per_page
        all_users, total_count = await UserOperate.get_all_users(include_inactive=True, limit=per_page, offset=offset)
        total_pages = max(1, (total_count + per_page - 1) // per_page)
        page = max(1, min(page, total_pages))

        if not all_users:
            kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="admin_users")]])
            await safe_edit_message(query.message, "📋 暂无用户", reply_markup=kb)
            return

        lines = [f"👥 **用户列表** (第 {page}/{total_pages} 页)\n"]
        for u in all_users:
            status = "✅" if u.ACTIVE_STATUS else "❌"
            emby = "🎬" if u.EMBYID else "  "
            lines.append(f"{status}{emby} `{u.USERNAME}` (UID:{u.UID})")

        nav = []
        if page > 1:
            nav.append(InlineKeyboardButton("⬅️ 上一页", callback_data=f"adm_userlist:{page - 1}"))
        nav.append(InlineKeyboardButton(f"{page}/{total_pages}", callback_data="noop"))
        if page < total_pages:
            nav.append(InlineKeyboardButton("➡️ 下一页", callback_data=f"adm_userlist:{page + 1}"))

        kb = InlineKeyboardMarkup(
            [
                nav,
                [InlineKeyboardButton("🔙 返回", callback_data="admin_users")],
            ]
        )
        await safe_edit_message(query.message, "\n".join(lines), reply_markup=kb)

    @require_admin
    async def cb_adm_queryuser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """查询用户 - 提示输入"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _set_admin_state(uid, {"action": "query_user"})
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="admin_users")]])
        await safe_edit_message(
            query.message, "🔍 **查询用户**\n\n请发送用户名：\n💡 发送 /cancel 可取消", reply_markup=kb
        )

    @require_admin
    async def cb_adm_adduser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """添加用户 - 提示输入"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _set_admin_state(uid, {"action": "add_user"})
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="admin_users")]])
        await safe_edit_message(
            query.message,
            "➕ **添加用户**\n\n请发送: `用户名 天数`\n示例: `test 30`\n💡 发送 /cancel 可取消",
            reply_markup=kb,
        )

    @require_admin
    async def cb_adm_banmenu(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """禁用/解禁菜单"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _set_admin_state(uid, {"action": "ban_user"})
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="admin_users")]])
        await safe_edit_message(
            query.message,
            "🚫 **禁用/解禁用户**\n\n请发送用户名，将自动切换其状态：\n💡 发送 /cancel 可取消",
            reply_markup=kb,
        )

    # ---- 用户操作 callback (从用户详情) ----

    @require_admin
    async def cb_adm_user_action(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """用户操作回调 (ban/unban/del/renew)"""
        query = update.callback_query
        data = query.data  # e.g. adm_act:ban:username or adm_act:del:uid
        parts = data.split(":")
        if len(parts) < 3:
            await answer_callback_safe(query, "参数错误", show_alert=True)
            return

        action = parts[1]
        username = parts[2]
        user = await UserOperate.get_user_by_username(username)
        if not user:
            await answer_callback_safe(query, "用户不存在", show_alert=True)
            return

        if action == "ban":
            if user.EMBYID:
                try:
                    emby = get_emby_client()
                    await emby.set_user_enabled(user.EMBYID, False)
                except Exception as e:
                    logger.warning(f"禁用 Emby 用户失败: {e}")
            user.ACTIVE_STATUS = False
            await UserOperate.update_user(user)
            await answer_callback_safe(query, f"✅ 已禁用 {username}")

        elif action == "unban":
            if user.EMBYID:
                try:
                    emby = get_emby_client()
                    await emby.set_user_enabled(user.EMBYID, True)
                except Exception as e:
                    logger.warning(f"解禁 Emby 用户失败: {e}")
            user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)
            await answer_callback_safe(query, f"✅ 已解禁 {username}")

        elif action == "del":
            if user.EMBYID:
                try:
                    emby = get_emby_client()
                    await emby.delete_user(user.EMBYID)
                except Exception as e:
                    logger.warning(f"删除 Emby 用户失败: {e}")
            await UserOperate.delete_user(user)
            await answer_callback_safe(query, f"✅ 已删除 {username}")
            kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="admin_users")]])
            await safe_edit_message(query.message, f"✅ 用户 `{username}` 已删除", reply_markup=kb)
            return

        elif action == "del_confirm":
            kb = InlineKeyboardMarkup(
                [
                    [
                        InlineKeyboardButton("⚠️ 确认删除", callback_data=f"adm_act:del:{username}"),
                        InlineKeyboardButton("🔙 取消", callback_data=f"adm_userdetail:{username}"),
                    ]
                ]
            )
            await answer_callback_safe(query)
            await safe_edit_message(
                query.message,
                f"⚠️ **确定要删除用户 `{username}` 吗？**\n\n此操作不可恢复！",
                reply_markup=kb,
            )
            return

        # 刷新用户详情
        user = await UserOperate.get_user_by_username(username)
        if user:
            await _show_user_detail(query.message, user)
        else:
            kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="admin_users")]])
            await safe_edit_message(query.message, "用户已不存在", reply_markup=kb)

    @require_admin
    async def cb_adm_userdetail(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """查看用户详情"""
        query = update.callback_query
        await answer_callback_safe(query)
        username = query.data.split(":")[1] if ":" in query.data else ""
        user = await UserOperate.get_user_by_username(username)
        if not user:
            await answer_callback_safe(query, "用户不存在", show_alert=True)
            return
        await _show_user_detail(query.message, user)

    @require_admin
    async def cb_adm_renew_prompt(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """续期提示"""
        query = update.callback_query
        await answer_callback_safe(query)
        username = query.data.split(":")[1] if ":" in query.data else ""
        uid = update.effective_user.id
        _set_admin_state(uid, {"action": "renew_user", "username": username})
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data=f"adm_userdetail:{username}")]])
        await safe_edit_message(
            query.message, f"🔄 **续期 `{username}`**\n\n请发送天数：\n💡 发送 /cancel 可取消", reply_markup=kb
        )

    # ======================== 注册码管理 ========================

    @require_admin
    async def cb_admin_regcode(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """注册码面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup(
            [
                [
                    InlineKeyboardButton("🆕 生成注册码", callback_data="adm_regcode_gen"),
                    InlineKeyboardButton("📋 查看列表", callback_data="adm_regcode_list"),
                ],
                [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
            ]
        )
        await safe_edit_message(query.message, "🎫 **注册码管理**\n\n请选择操作：", reply_markup=kb)

    @require_admin
    async def cb_adm_regcode_gen(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """生成注册码选择天数"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup(
            [
                [
                    InlineKeyboardButton("7天", callback_data="adm_reggen:7:1"),
                    InlineKeyboardButton("15天", callback_data="adm_reggen:15:1"),
                    InlineKeyboardButton("30天", callback_data="adm_reggen:30:1"),
                ],
                [
                    InlineKeyboardButton("90天", callback_data="adm_reggen:90:1"),
                    InlineKeyboardButton("180天", callback_data="adm_reggen:180:1"),
                    InlineKeyboardButton("365天", callback_data="adm_reggen:365:1"),
                ],
                [InlineKeyboardButton("🔙 返回", callback_data="admin_regcode")],
            ]
        )
        await safe_edit_message(query.message, "🆕 **生成注册码**\n\n选择有效天数：", reply_markup=kb)

    @require_admin
    async def cb_adm_reggen(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """执行生成注册码"""
        query = update.callback_query
        await answer_callback_safe(query)
        parts = query.data.split(":")
        days = int(parts[1]) if len(parts) > 1 else 30
        count = int(parts[2]) if len(parts) > 2 else 1
        count = min(count, 10)

        result = await RegCodeOperate.create_regcode(vali_time=-1, type_=1, day=days, count=count)
        if isinstance(result, str):
            result = [result]
        codes = [f"`{c}`" for c in result]

        days_text = "永久" if days <= 0 else f"{days}天"
        text = f"✅ **生成 {count} 个注册码** ({days_text})\n\n" + "\n".join(codes)
        kb = InlineKeyboardMarkup(
            [
                [InlineKeyboardButton("🔁 再生成", callback_data="adm_regcode_gen")],
                [InlineKeyboardButton("🔙 返回", callback_data="admin_regcode")],
            ]
        )
        await safe_edit_message(query.message, text, reply_markup=kb)

    @require_admin
    async def cb_adm_regcode_list(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """注册码列表"""
        query = update.callback_query
        await answer_callback_safe(query)
        all_codes = await RegCodeOperate.get_all_regcodes()
        codes = [c for c in all_codes if c.ACTIVE]
        if not codes:
            text = "📋 暂无可用注册码"
        else:
            lines = [f"🎫 **可用注册码** ({len(codes)} 个)\n"]
            for c in codes[:15]:
                line_days_text = "永久" if (c.DAYS is not None and int(c.DAYS) <= 0) else f"{c.DAYS}天"
                lines.append(f"• `{c.CODE}` - {line_days_text}")
            if len(codes) > 15:
                lines.append(f"\n... 还有 {len(codes) - 15} 个")
            text = "\n".join(lines)
        kb = InlineKeyboardMarkup(
            [
                [InlineKeyboardButton("🆕 生成", callback_data="adm_regcode_gen")],
                [InlineKeyboardButton("🔙 返回", callback_data="admin_regcode")],
            ]
        )
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 统计 ========================

    @require_admin
    async def cb_admin_stats(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """系统统计"""
        query = update.callback_query
        await answer_callback_safe(query)

        total_count = await UserOperate.get_registered_users_count()
        active_count = await UserOperate.get_active_users_count()
        all_codes = await RegCodeOperate.get_all_regcodes()
        active_codes_list = [c for c in all_codes if c.ACTIVE]

        try:
            emby_status = await EmbyService.get_server_status()
            emby_info = f"🎬 **Emby**\n" f"• 状态: ✅ 在线\n" f"• 版本: {emby_status.get('version', '未知')}"
        except Exception:
            emby_info = "🎬 **Emby**\n• 状态: ❌ 离线"

        text = (
            f"📊 **系统统计**\n\n"
            f"👥 **用户**: {total_count} (活跃: {active_count})\n"
            f"🎫 **注册码**: {len(all_codes)} (可用: {len(active_codes_list)})\n\n"
            f"{emby_info}"
        )
        kb = InlineKeyboardMarkup(
            [
                [InlineKeyboardButton("🔄 刷新", callback_data="admin_stats")],
                [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
            ]
        )
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== Emby 管理 ========================

    @require_admin
    async def cb_admin_emby(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """Emby 管理面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup(
            [
                [
                    InlineKeyboardButton("🔌 连接测试", callback_data="adm_emby_test"),
                    InlineKeyboardButton("📺 活跃会话", callback_data="adm_emby_sessions"),
                ],
                [
                    InlineKeyboardButton("👥 Emby 用户", callback_data="adm_emby_users"),
                    InlineKeyboardButton("🧹 清理孤儿", callback_data="adm_emby_cleanup"),
                ],
                [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
            ]
        )
        await safe_edit_message(query.message, "🎬 **Emby 管理**\n\n请选择操作：", reply_markup=kb)

    @require_admin
    async def cb_adm_emby_test(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """Emby 连接测试"""
        query = update.callback_query
        await answer_callback_safe(query, "正在测试...")
        try:
            status = await EmbyService.get_server_status()
            text = (
                f"✅ **Emby 连接正常**\n\n"
                f"🏷️ 名称: {status.get('server_name', '未知')}\n"
                f"📌 版本: {status.get('version', '未知')}"
            )
        except Exception as e:
            text = f"❌ **Emby 连接失败**\n\n错误: {e}"
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="admin_emby")]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    @require_admin
    async def cb_adm_emby_sessions(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """活跃会话"""
        query = update.callback_query
        await answer_callback_safe(query)
        try:
            sessions = await EmbyService.get_all_sessions()
            if not sessions:
                text = "📺 当前没有活跃会话"
            else:
                lines = [f"📺 **活跃会话** ({len(sessions)} 个)\n"]
                for s in sessions[:10]:
                    name = s.get("user_name", "未知")
                    dev = s.get("device_name", "?")
                    np = s.get("now_playing", {})
                    if np:
                        lines.append(f"• **{name}** @ {dev}\n  ▶️ {np.get('name', '?')}")
                    else:
                        lines.append(f"• **{name}** @ {dev} (空闲)")
                text = "\n".join(lines)
        except Exception as e:
            text = f"❌ 获取失败: {e}"
        kb = InlineKeyboardMarkup(
            [
                [InlineKeyboardButton("🔄 刷新", callback_data="adm_emby_sessions")],
                [InlineKeyboardButton("🔙 返回", callback_data="admin_emby")],
            ]
        )
        await safe_edit_message(query.message, text, reply_markup=kb)

    @require_admin
    async def cb_adm_emby_users(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """Emby 用户列表"""
        query = update.callback_query
        await answer_callback_safe(query, "正在获取...")
        try:
            emby = get_emby_client()
            emby_users = await emby.get_users()
            if not emby_users:
                text = "👥 暂无 Emby 用户"
            else:
                lines = [f"👥 **Emby 用户** ({len(emby_users)} 个)\n"]
                for eu in emby_users[:20]:
                    lines.append(f"• `{eu.name}` (ID: {eu.id[:8]}..)")
                if len(emby_users) > 20:
                    lines.append(f"\n... 还有 {len(emby_users) - 20} 个")
                text = "\n".join(lines)
        except Exception as e:
            text = f"❌ 获取失败: {e}"
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="admin_emby")]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    @require_admin
    async def cb_adm_emby_cleanup(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """清理确认"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup(
            [
                [
                    InlineKeyboardButton("⚠️ 确认清理", callback_data="adm_emby_cleanup_confirm"),
                    InlineKeyboardButton("🔙 取消", callback_data="admin_emby"),
                ]
            ]
        )
        await safe_edit_message(
            query.message,
            "🧹 **清理孤儿用户**\n\n将删除 Emby 中存在但系统中无记录的用户。\n确认执行？",
            reply_markup=kb,
        )

    @require_admin
    async def cb_adm_emby_cleanup_confirm(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """执行清理"""
        query = update.callback_query
        await answer_callback_safe(query, "正在清理...")
        try:
            emby = get_emby_client()
            emby_users = await emby.get_users()
            local_emby_ids = set()
            all_users, _ = await UserOperate.get_all_users(include_inactive=True)
            for u in all_users:
                if u.EMBYID:
                    local_emby_ids.add(u.EMBYID)
            orphans = [eu for eu in emby_users if eu.id not in local_emby_ids]
            deleted = 0
            for orphan in orphans:
                if await emby.delete_user(orphan.id):
                    deleted += 1
            text = f"✅ **清理完成**\n\n删除了 {deleted} 个孤儿用户"
        except Exception as e:
            text = f"❌ 清理失败: {e}"
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="admin_emby")]])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 广播 ========================

    @require_admin
    async def cb_admin_broadcast(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """广播提示"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _set_admin_state(uid, {"action": "broadcast"})
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="panel_admin")]])
        await safe_edit_message(
            query.message, "📢 **广播消息**\n\n请发送要广播的内容：\n💡 发送 /cancel 可取消", reply_markup=kb
        )

    # ======================== 文本消息路由（admin 状态机）========================

    async def handle_admin_text(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """处理管理员文本输入（根据状态机路由）"""
        uid = update.effective_user.id
        if not is_admin(uid):
            return

        state = _get_admin_state(uid)
        if not state:
            return  # 无状态，忽略

        action = state.get("action")
        text = (update.message.text or "").strip()

        if text.lower() in {"/cancel", "cancel", "取消", "返回"}:
            _clear_admin_state(uid)
            await update.message.reply_text("✅ 已取消当前操作，可发送 /admin 返回管理面板")
            return

        if action == "query_user":
            user = await UserOperate.get_user_by_username(text)
            if not user:
                await update.message.reply_text(
                    f"❌ 用户 `{text}` 不存在\n请重新输入，或发送 /cancel 取消", parse_mode="Markdown"
                )
                return
            info = f"📋 **用户详情**\n\n{format_user_info(user)}"
            kb = _user_action_kb(user)
            await update.message.reply_text(info, reply_markup=kb, parse_mode="Markdown")
            _clear_admin_state(uid)

        elif action == "add_user":
            parts = text.split()
            if not parts:
                await update.message.reply_text(
                    "❌ 输入格式错误，请发送: `用户名 天数`\n示例: `test 30`", parse_mode="Markdown"
                )
                return
            username = parts[0]
            try:
                days = int(parts[1]) if len(parts) > 1 else 30
            except ValueError:
                await update.message.reply_text("❌ 天数必须是数字，请重新输入")
                return
            if days <= 0:
                await update.message.reply_text("❌ 天数必须大于 0，请重新输入")
                return
            if await UserOperate.get_user_by_username(username):
                await update.message.reply_text("❌ 用户名已存在，请换一个用户名")
                return
            resp = await UserService._create_emby_user(telegram_id=None, username=username, email=None, days=days)
            if resp.result == RegisterResult.SUCCESS:
                await update.message.reply_text(
                    f"✅ **用户创建成功**\n\n"
                    f"👤 用户名: `{username}`\n"
                    f"⏰ 有效期: **{days}** 天\n"
                    "🔒 密码不通过 Bot 展示，请在 Web 后台重置后安全交付。",
                    parse_mode="Markdown",
                )
                _clear_admin_state(uid)
            else:
                await update.message.reply_text(f"❌ 创建失败: {resp.message}")

        elif action == "ban_user":
            user = await UserOperate.get_user_by_username(text)
            if not user:
                await update.message.reply_text(
                    f"❌ 用户 `{text}` 不存在\n请重新输入，或发送 /cancel 取消", parse_mode="Markdown"
                )
                return
            if user.ACTIVE_STATUS:
                if user.EMBYID:
                    try:
                        emby = get_emby_client()
                        await emby.set_user_enabled(user.EMBYID, False)
                    except Exception:
                        pass
                user.ACTIVE_STATUS = False
                await UserOperate.update_user(user)
                await update.message.reply_text(f"✅ 已禁用用户: `{text}`", parse_mode="Markdown")
            else:
                if user.EMBYID:
                    try:
                        emby = get_emby_client()
                        await emby.set_user_enabled(user.EMBYID, True)
                    except Exception:
                        pass
                user.ACTIVE_STATUS = True
                await UserOperate.update_user(user)
                await update.message.reply_text(f"✅ 已解禁用户: `{text}`", parse_mode="Markdown")
            _clear_admin_state(uid)

        elif action == "renew_user":
            username = state.get("username", "")
            try:
                days = int(text)
            except ValueError:
                await update.message.reply_text("❌ 请输入数字天数，或发送 /cancel 取消")
                return
            if days <= 0:
                await update.message.reply_text("❌ 天数必须大于 0")
                return
            user = await UserOperate.get_user_by_username(username)
            if not user:
                await update.message.reply_text("❌ 用户不存在，已取消本次续期流程")
                _clear_admin_state(uid)
                return
            success, msg = await UserService.renew_user(user, days)
            if success:
                await update.message.reply_text(f"✅ 已为 `{username}` 续期 **{days}** 天", parse_mode="Markdown")
                _clear_admin_state(uid)
            else:
                await update.message.reply_text(f"❌ 续期失败: {msg}")

        elif action == "broadcast":
            from src.db.user import UsersSessionFactory, UserModel
            from sqlalchemy import select

            async with UsersSessionFactory() as session:
                result = await session.execute(
                    select(UserModel.TELEGRAM_ID).where(
                        UserModel.TELEGRAM_ID != None,
                        UserModel.ACTIVE_STATUS == True,
                    )
                )
                tg_ids = [row[0] for row in result.all()]

            if not tg_ids:
                await update.message.reply_text("⚠️ 没有可发送的用户")
                return

            success = failed = 0
            progress = await update.message.reply_text(f"📢 广播中... (0/{len(tg_ids)})")
            for i, tid in enumerate(tg_ids):
                try:
                    await context.bot.send_message(
                        chat_id=tid,
                        text=f"📢 **系统通知**\n\n{text}",
                        parse_mode="Markdown",
                    )
                    success += 1
                except Exception:
                    failed += 1
                if (i + 1) % 10 == 0:
                    await safe_edit_message(progress, f"📢 广播中... ({i + 1}/{len(tg_ids)})")
            await safe_edit_message(progress, f"✅ **广播完成**\n📤 成功: {success}  ❌ 失败: {failed}")
            _clear_admin_state(uid)

    # ======================== 传统命令（兼容） ========================

    @require_private
    @require_admin
    async def cmd_adduser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/adduser <用户名> [天数]`", parse_mode="Markdown")
            return
        username = context.args[0]
        days = int(context.args[1]) if len(context.args) > 1 else 30
        if await UserOperate.get_user_by_username(username):
            await update.message.reply_text("❌ 用户名已存在")
            return
        resp = await UserService._create_emby_user(telegram_id=None, username=username, email=None, days=days)
        if resp.result == RegisterResult.SUCCESS:
            await update.message.reply_text(
                f"✅ 创建成功\n👤 `{username}`\n⏰ {days}天\n🔒 密码不通过 Bot 展示，请在 Web 后台重置后安全交付。",
                parse_mode="Markdown",
            )
        else:
            await update.message.reply_text(f"❌ {resp.message}")

    @require_private
    @require_admin
    async def cmd_regcode(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text(
                "用法: `/regcode new [天数] [数量]` | `/regcode list`", parse_mode="Markdown"
            )
            return
        action = context.args[0].lower()
        if action == "new":
            days = int(context.args[1]) if len(context.args) > 1 else 30
            count = min(int(context.args[2]) if len(context.args) > 2 else 1, 10)
            result = await RegCodeOperate.create_regcode(vali_time=-1, type_=1, day=days, count=count)
            if isinstance(result, str):
                result = [result]
            days_text = "永久" if days <= 0 else f"{days}天"
            codes = [f"`{c}` ({days_text})" for c in result]
            await update.message.reply_text(f"✅ 生成 {count} 个注册码\n\n" + "\n".join(codes), parse_mode="Markdown")
        elif action == "list":
            all_codes = await RegCodeOperate.get_all_regcodes()
            codes = [c for c in all_codes if c.ACTIVE]
            if not codes:
                await update.message.reply_text("📋 暂无可用注册码")
            else:
                lines = [f"🎫 **可用注册码** ({len(codes)}个)\n"]
                for c in codes[:20]:
                    line_days_text = "永久" if (c.DAYS is not None and int(c.DAYS) <= 0) else f"{c.DAYS}天"
                    lines.append(f"• `{c.CODE}` - {line_days_text}")
                await update.message.reply_text("\n".join(lines), parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_broadcast(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/broadcast <内容>`", parse_mode="Markdown")
            return
        content = update.message.text.split(None, 1)[1]
        _admin_states.pop(update.effective_user.id, None)  # clear
        from src.db.user import UsersSessionFactory, UserModel
        from sqlalchemy import select

        async with UsersSessionFactory() as session:
            r = await session.execute(
                select(UserModel.TELEGRAM_ID).where(UserModel.TELEGRAM_ID != None, UserModel.ACTIVE_STATUS == True)
            )
            tg_ids = [row[0] for row in r.all()]
        if not tg_ids:
            await update.message.reply_text("⚠️ 没有可发送的用户")
            return
        success = failed = 0
        for tid in tg_ids:
            try:
                await context.bot.send_message(chat_id=tid, text=f"📢 **系统通知**\n\n{content}", parse_mode="Markdown")
                success += 1
            except Exception:
                failed += 1
        await update.message.reply_text(f"✅ 广播完成 | 成功: {success} 失败: {failed}")

    @require_private
    @require_admin
    async def cmd_stats(update: Update, context: ContextTypes.DEFAULT_TYPE):
        total = await UserOperate.get_registered_users_count()
        active = await UserOperate.get_active_users_count()
        codes = await RegCodeOperate.get_all_regcodes()
        active_codes_cmd = [c for c in codes if c.ACTIVE]
        text = f"📊 用户: {total} (活跃{active}) | 注册码: {len(codes)} (可用{len(active_codes_cmd)})"
        await update.message.reply_text(text)

    @require_private
    @require_admin
    async def cmd_userinfo(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/userinfo <用户名>`", parse_mode="Markdown")
            return
        user = await UserOperate.get_user_by_username(context.args[0])
        if not user:
            await update.message.reply_text("❌ 用户不存在")
            return
        text = f"📋 **用户详情**\n\n{format_user_info(user)}"
        await update.message.reply_text(text, reply_markup=_user_action_kb(user), parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_twfind(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/twfind <用户名/UID/TGID/TG用户名/Emby标识>`", parse_mode="Markdown")
            return
        query = " ".join(context.args).strip()
        users, total = await UserOperate.get_all_users(include_inactive=True, search=query, limit=10, offset=0)
        if not users:
            await update.message.reply_text("未找到匹配用户")
            return
        lines = [f"🔍 **查询结果** ({len(users)}/{total})\n"]
        for user in users:
            lines.append(format_user_info(user, brief=True))
        await update.message.reply_text("\n".join(lines), parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_twbindcheck(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if context.args:
            users, _ = await UserOperate.get_all_users(
                include_inactive=True,
                search=" ".join(context.args).strip(),
                limit=10,
                offset=0,
            )
        else:
            users, _ = await UserOperate.get_all_users(include_inactive=True, limit=100000, offset=0)
        tg_map = {}
        invalid = []
        for user in users:
            if user.TELEGRAM_ID is None:
                continue
            try:
                tg_id = int(user.TELEGRAM_ID)
            except (TypeError, ValueError):
                invalid.append(user)
                continue
            if tg_id <= 0:
                invalid.append(user)
                continue
            tg_map.setdefault(tg_id, []).append(user)
        duplicates = {tg_id: rows for tg_id, rows in tg_map.items() if len(rows) > 1}
        lines = ["📱 **TG 绑定检查**", f"扫描用户: {len(users)}", f"已绑定 TG: {sum(len(v) for v in tg_map.values())}"]
        lines.append(f"非法 TGID: {len(invalid)}")
        lines.append(f"重复 TGID: {len(duplicates)}")
        for tg_id, rows in list(duplicates.items())[:10]:
            lines.append(f"• `{tg_id}` -> " + ", ".join(f"{u.UID}/{u.USERNAME}" for u in rows))
        await update.message.reply_text("\n".join(lines), parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_twforcebind(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if len(context.args) < 2:
            await update.message.reply_text("用法: `/twforcebind <用户> <TGID>`", parse_mode="Markdown")
            return
        query, tg_raw = context.args[0], context.args[1]
        try:
            tg_id = int(tg_raw)
        except ValueError:
            await update.message.reply_text("TGID 必须是数字")
            return
        users, _ = await UserOperate.get_all_users(include_inactive=True, search=query, limit=2, offset=0)
        if len(users) != 1:
            await update.message.reply_text("请提供能唯一匹配一个用户的关键词")
            return
        target = users[0]
        occupant = await UserOperate.get_user_by_telegram_id(tg_id)
        if occupant and occupant.UID != target.UID:
            occupant.TELEGRAM_ID = None
            await UserOperate.update_user(occupant)
        target.TELEGRAM_ID = tg_id
        await UserOperate.update_user(target)
        await update.message.reply_text(f"✅ 已绑定 UID={target.UID} 到 TGID `{tg_id}`", parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_twsyncuser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/twsyncuser <用户>`", parse_mode="Markdown")
            return
        users, _ = await UserOperate.get_all_users(include_inactive=True, search=" ".join(context.args), limit=2, offset=0)
        if len(users) != 1:
            await update.message.reply_text("请提供能唯一匹配一个用户的关键词")
            return
        ok, msg = await UserService.sync_user_to_emby(users[0])
        await update.message.reply_text(("✅ " if ok else "❌ ") + msg)

    # ======================== 群组管理员工具 ========================

    async def _resolve_group_target(update: Update, context: ContextTypes.DEFAULT_TYPE) -> tuple[object | None, int | None, str]:
        reply = update.message.reply_to_message if update.message else None
        if reply and reply.from_user:
            tg_id = int(reply.from_user.id)
            user = await UserOperate.get_user_by_telegram_id(tg_id)
            label = reply.from_user.full_name or reply.from_user.username or str(tg_id)
            return user, tg_id, label

        query = " ".join(context.args or []).strip()
        if not query:
            return None, None, ""

        if query.isdigit():
            uid_user = await UserOperate.get_user_by_uid(int(query))
            if uid_user:
                return uid_user, uid_user.TELEGRAM_ID, uid_user.USERNAME
            tg_user = await UserOperate.get_user_by_telegram_id(int(query))
            if tg_user:
                return tg_user, int(query), tg_user.USERNAME

        users, _ = await UserOperate.get_all_users(include_inactive=True, search=query, limit=2, offset=0)
        if len(users) == 1:
            return users[0], users[0].TELEGRAM_ID, users[0].USERNAME
        return None, None, query

    def _format_group_user_info(user, tg_id: int | None, label: str) -> str:
        from src.core.utils import format_expire_time

        lines = ["🔎 **用户查询**"]
        if user:
            role_map = {
                Role.ADMIN.value: "管理员",
                Role.WHITE_LIST.value: "白名单",
                Role.NORMAL.value: "普通用户",
                Role.UNRECOGNIZED.value: "未注册",
            }
            lines += [
                f"系统用户: `{_md_code(user.USERNAME)}`",
                f"UID: `{user.UID}`",
                f"角色: {role_map.get(user.ROLE, '未知')}",
                f"状态: {'启用' if user.ACTIVE_STATUS else '禁用'}",
                f"Telegram: {'已绑定' if user.TELEGRAM_ID else '未绑定'}",
                f"Emby: {'已绑定' if user.EMBYID else '未绑定'}",
                f"开通资格: {'有' if bool(getattr(user, 'PENDING_EMBY', False)) and not user.EMBYID else '无'}",
                f"到期: {format_expire_time(user.EXPIRED_AT) if user.EMBYID else '未绑定 Emby'}",
            ]
        else:
            lines += [
                f"Telegram 用户: `{_md_code(label or '未知')}`",
                "系统用户: 未绑定 / 未找到",
            ]
        lines.append("\n未展示密码、线路、Emby 用户名、Emby ID、Telegram ID 等隐私信息。")
        return "\n".join(lines)

    def _group_user_action_kb(token: str, *, has_user: bool, has_tg: bool, has_emby: bool, active: bool) -> InlineKeyboardMarkup:
        rows = []
        if has_user and not has_emby:
            rows.append([InlineKeyboardButton("给予 Emby 开通资格", callback_data=f"gadm:act:grant:{token}")])
        if has_user:
            rows.append([
                InlineKeyboardButton("禁用" if active else "启用", callback_data=f"gadm:act:{'disable' if active else 'enable'}:{token}"),
                InlineKeyboardButton("删除", callback_data=f"gadm:act:delask:{token}"),
            ])
        if has_tg:
            rows.append([
                InlineKeyboardButton("踢出不封禁", callback_data=f"gadm:act:kick:{token}"),
                InlineKeyboardButton("封禁", callback_data=f"gadm:act:ban:{token}"),
            ])
        rows.append([InlineKeyboardButton("刷新", callback_data=f"gadm:act:refresh:{token}")])
        return InlineKeyboardMarkup(rows)

    async def _send_group_user_card(update: Update, context: ContextTypes.DEFAULT_TYPE, *, require_verify: bool = False):
        user, tg_id, label = await _resolve_group_target(update, context)
        if not user and not tg_id:
            await update.message.reply_text(
                "用法: `/twguser <UID/用户名/TGID/关键词>`，也可以回复某人的消息发送 `/twguser`",
                parse_mode="Markdown",
            )
            return

        token = _new_group_admin_context({
            "uid": user.UID if user else None,
            "tg_id": int(tg_id) if tg_id else None,
            "chat_id": update.effective_chat.id if update.effective_chat else None,
            "label": label,
        })
        if require_verify:
            await update.message.reply_text(
                "匿名管理员指令已收到。请点击按钮验证管理员身份后查看。",
                reply_markup=InlineKeyboardMarkup([[InlineKeyboardButton("验证管理员身份", callback_data=f"gadm:auth:{token}")]]),
            )
            return

        text = _format_group_user_info(user, tg_id, label)
        await update.message.reply_text(
            text,
            reply_markup=_group_user_action_kb(
                token,
                has_user=bool(user),
                has_tg=bool(tg_id),
                has_emby=bool(user and user.EMBYID),
                active=bool(user and user.ACTIVE_STATUS),
            ),
            parse_mode="Markdown",
        )

    async def cmd_twguser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not update.message:
            return
        if not update.effective_chat or update.effective_chat.type not in ("group", "supergroup"):
            await update.message.reply_text("此命令用于群组管理。私聊请使用 /twfind 或 /userinfo")
            return
        if _is_anonymous_group_command(update):
            await _send_group_user_card(update, context, require_verify=True)
            return
        actor_id = update.effective_user.id if update.effective_user else 0
        if not is_admin(actor_id):
            await update.message.reply_text("仅限管理员使用")
            return
        await _send_group_user_card(update, context)

    async def cb_group_admin_auth(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        actor_id = update.effective_user.id if update.effective_user else 0
        if not is_admin(actor_id):
            await answer_callback_safe(query, "仅限管理员", show_alert=True)
            return
        token = (query.data or "").split(":")[-1]
        payload = _get_group_admin_context(token)
        if not payload:
            await answer_callback_safe(query, "操作已过期", show_alert=True)
            return
        user = await UserOperate.get_user_by_uid(payload.get("uid")) if payload.get("uid") else None
        text = _format_group_user_info(user, payload.get("tg_id"), payload.get("label") or "")
        await answer_callback_safe(query)
        await safe_edit_message(
            query.message,
            text,
            reply_markup=_group_user_action_kb(
                token,
                has_user=bool(user),
                has_tg=bool(payload.get("tg_id")),
                has_emby=bool(user and user.EMBYID),
                active=bool(user and user.ACTIVE_STATUS),
            ),
        )

    async def cb_group_admin_action(update: Update, context: ContextTypes.DEFAULT_TYPE):
        query = update.callback_query
        actor_id = update.effective_user.id if update.effective_user else 0
        if not is_admin(actor_id):
            await answer_callback_safe(query, "仅限管理员", show_alert=True)
            return
        parts = (query.data or "").split(":")
        if len(parts) < 4:
            await answer_callback_safe(query, "参数错误", show_alert=True)
            return
        action, token = parts[2], parts[3]
        payload = _get_group_admin_context(token)
        if not payload:
            await answer_callback_safe(query, "操作已过期", show_alert=True)
            return

        user = await UserOperate.get_user_by_uid(payload.get("uid")) if payload.get("uid") else None
        tg_id = payload.get("tg_id")
        label = payload.get("label") or "用户"
        chat_id = payload.get("chat_id") or (query.message.chat_id if query.message else None)

        if action == "refresh":
            text = _format_group_user_info(user, tg_id, label)
            await answer_callback_safe(query, "已刷新")
            await safe_edit_message(
                query.message,
                text,
                reply_markup=_group_user_action_kb(
                    token,
                    has_user=bool(user),
                    has_tg=bool(tg_id),
                    has_emby=bool(user and user.EMBYID),
                    active=bool(user and user.ACTIVE_STATUS),
                ),
            )
            return

        if action == "delask":
            if not user:
                await answer_callback_safe(query, "未找到本地用户", show_alert=True)
                return
            await answer_callback_safe(query)
            await safe_edit_message(
                query.message,
                f"确认删除本地用户 `{_md_code(user.USERNAME)}`？\n\n将同时尝试删除其 Emby 账号；此操作不可恢复。",
                reply_markup=InlineKeyboardMarkup([
                    [InlineKeyboardButton("确认删除", callback_data=f"gadm:act:delete:{token}")],
                    [InlineKeyboardButton("取消", callback_data=f"gadm:act:refresh:{token}")],
                ]),
            )
            return

        if action in {"grant", "disable", "enable", "delete"} and not user:
            await answer_callback_safe(query, "未找到本地用户", show_alert=True)
            return
        if user and user.ROLE == Role.ADMIN.value and user.TELEGRAM_ID != actor_id and action in {"disable", "delete", "ban"}:
            await answer_callback_safe(query, "不允许操作其他管理员", show_alert=True)
            return

        message = "操作完成"
        if action == "grant":
            if user.EMBYID:
                await answer_callback_safe(query, "该用户已有 Emby 账号", show_alert=True)
                return
            user.PENDING_EMBY = True
            user.PENDING_EMBY_DAYS = int(RegisterConfig.EMBY_DIRECT_REGISTER_DAYS or 30)
            if not user.ACTIVE_STATUS:
                user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)
            message = "已授予 Emby 开通资格，用户前往 Web 即可创建账号"
        elif action == "disable":
            if user.EMBYID:
                try:
                    await get_emby_client().set_user_enabled(user.EMBYID, False)
                except Exception as exc:
                    logger.warning("群组禁用 Emby 用户失败: %s", exc)
            user.ACTIVE_STATUS = False
            await UserOperate.update_user(user)
            message = "已禁用用户"
        elif action == "enable":
            if user.EMBYID:
                try:
                    await get_emby_client().set_user_enabled(user.EMBYID, True)
                except Exception as exc:
                    logger.warning("群组启用 Emby 用户失败: %s", exc)
            user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)
            message = "已启用用户"
        elif action == "delete":
            ok, msg = await UserService.delete_user(user, delete_emby=True)
            _group_admin_contexts.pop(token, None)
            await answer_callback_safe(query, msg if ok else "删除失败", show_alert=not ok)
            await safe_edit_message(query.message, ("✅ " if ok else "❌ ") + msg)
            return
        elif action == "kick":
            if not tg_id or not chat_id:
                await answer_callback_safe(query, "缺少 Telegram 用户或群组信息", show_alert=True)
                return
            try:
                await context.bot.ban_chat_member(chat_id=chat_id, user_id=int(tg_id))
                await context.bot.unban_chat_member(chat_id=chat_id, user_id=int(tg_id), only_if_banned=True)
            except Exception as exc:
                logger.warning("群组踢出用户失败: %s", exc)
                await answer_callback_safe(query, "踢出失败，请确认 Bot 有封禁权限", show_alert=True)
                return
            message = "已踢出用户（未封禁）"
        elif action == "ban":
            if not tg_id or not chat_id:
                await answer_callback_safe(query, "缺少 Telegram 用户或群组信息", show_alert=True)
                return
            try:
                await context.bot.ban_chat_member(chat_id=chat_id, user_id=int(tg_id))
            except Exception as exc:
                logger.warning("群组封禁用户失败: %s", exc)
                await answer_callback_safe(query, "封禁失败，请确认 Bot 有封禁权限", show_alert=True)
                return
            if user:
                user.ACTIVE_STATUS = False
                if user.EMBYID:
                    try:
                        await get_emby_client().set_user_enabled(user.EMBYID, False)
                    except Exception as exc:
                        logger.warning("群组封禁时禁用 Emby 用户失败: %s", exc)
                await UserOperate.update_user(user)
            message = "已封禁用户"
        else:
            await answer_callback_safe(query, "未知操作", show_alert=True)
            return

        user = await UserOperate.get_user_by_uid(user.UID) if user else None
        text = _format_group_user_info(user, tg_id, label) + f"\n\n✅ {message}"
        await answer_callback_safe(query, message)
        await safe_edit_message(
            query.message,
            text,
            reply_markup=_group_user_action_kb(
                token,
                has_user=bool(user),
                has_tg=bool(tg_id),
                has_emby=bool(user and user.EMBYID),
                active=bool(user and user.ACTIVE_STATUS),
            ),
        )

    # ======================== 注册处理器 ========================

    # 命令
    app.add_handler(CommandHandler("admin", cmd_admin))
    app.add_handler(CommandHandler("twishelp", cmd_twishelp))
    app.add_handler(CommandHandler("cancel", cmd_cancel))
    app.add_handler(CommandHandler("adduser", cmd_adduser))
    app.add_handler(CommandHandler("regcode", cmd_regcode))
    app.add_handler(CommandHandler("broadcast", cmd_broadcast))
    app.add_handler(CommandHandler("stats", cmd_stats))
    app.add_handler(CommandHandler("userinfo", cmd_userinfo))
    app.add_handler(CommandHandler("twfind", cmd_twfind))
    app.add_handler(CommandHandler("twguser", cmd_twguser))
    app.add_handler(CommandHandler("twbindcheck", cmd_twbindcheck))
    app.add_handler(CommandHandler("twforcebind", cmd_twforcebind))
    app.add_handler(CommandHandler("twsyncuser", cmd_twsyncuser))

    # 管理面板导航
    app.add_handler(CallbackQueryHandler(cb_panel_admin, pattern="^panel_admin$"))
    app.add_handler(CallbackQueryHandler(cb_admin_users, pattern="^admin_users$"))
    app.add_handler(CallbackQueryHandler(cb_admin_regcode, pattern="^admin_regcode$"))
    app.add_handler(CallbackQueryHandler(cb_admin_stats, pattern="^admin_stats$"))
    app.add_handler(CallbackQueryHandler(cb_admin_emby, pattern="^admin_emby$"))
    app.add_handler(CallbackQueryHandler(cb_admin_broadcast, pattern="^admin_broadcast$"))

    # 用户管理
    app.add_handler(CallbackQueryHandler(cb_adm_userlist, pattern=r"^adm_userlist:"))
    app.add_handler(CallbackQueryHandler(cb_adm_queryuser, pattern="^adm_queryuser$"))
    app.add_handler(CallbackQueryHandler(cb_adm_adduser, pattern="^adm_adduser$"))
    app.add_handler(CallbackQueryHandler(cb_adm_banmenu, pattern="^adm_banmenu$"))
    app.add_handler(CallbackQueryHandler(cb_adm_user_action, pattern=r"^adm_act:"))
    app.add_handler(CallbackQueryHandler(cb_adm_userdetail, pattern=r"^adm_userdetail:"))
    app.add_handler(CallbackQueryHandler(cb_adm_renew_prompt, pattern=r"^adm_renew:"))

    # 注册码
    app.add_handler(CallbackQueryHandler(cb_adm_regcode_gen, pattern="^adm_regcode_gen$"))
    app.add_handler(CallbackQueryHandler(cb_adm_reggen, pattern=r"^adm_reggen:"))
    app.add_handler(CallbackQueryHandler(cb_adm_regcode_list, pattern="^adm_regcode_list$"))

    # Emby 管理
    app.add_handler(CallbackQueryHandler(cb_adm_emby_test, pattern="^adm_emby_test$"))
    app.add_handler(CallbackQueryHandler(cb_adm_emby_sessions, pattern="^adm_emby_sessions$"))
    app.add_handler(CallbackQueryHandler(cb_adm_emby_users, pattern="^adm_emby_users$"))
    app.add_handler(CallbackQueryHandler(cb_adm_emby_cleanup, pattern="^adm_emby_cleanup$"))
    app.add_handler(CallbackQueryHandler(cb_adm_emby_cleanup_confirm, pattern="^adm_emby_cleanup_confirm$"))

    # 群组管理员工具
    app.add_handler(CallbackQueryHandler(cb_group_admin_auth, pattern=r"^gadm:auth:"))
    app.add_handler(CallbackQueryHandler(cb_group_admin_action, pattern=r"^gadm:act:"))

    # noop
    app.add_handler(CallbackQueryHandler(lambda u, c: answer_callback_safe(u.callback_query), pattern="^noop$"))

    # 文本消息（admin 状态机）- 优先级较低
    app.add_handler(
        MessageHandler(filters.TEXT & ~filters.COMMAND & filters.ChatType.PRIVATE, handle_admin_text), group=1
    )


# ======================== 辅助函数 ========================


def _admin_menu_kb() -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        [
            [
                InlineKeyboardButton("👥 用户管理", callback_data="admin_users"),
                InlineKeyboardButton("🎫 注册码", callback_data="admin_regcode"),
            ],
            [
                InlineKeyboardButton("📊 统计", callback_data="admin_stats"),
                InlineKeyboardButton("🎬 Emby", callback_data="admin_emby"),
            ],
            [
                InlineKeyboardButton("📢 广播", callback_data="admin_broadcast"),
            ],
            [InlineKeyboardButton("♻️ 主菜单", callback_data="back_start")],
        ]
    )


def _user_action_kb(user) -> InlineKeyboardMarkup:
    """用户操作键盘"""
    username = user.USERNAME
    buttons = []
    if user.ACTIVE_STATUS:
        buttons.append([InlineKeyboardButton("🚫 禁用", callback_data=f"adm_act:ban:{username}")])
    else:
        buttons.append([InlineKeyboardButton("✅ 解禁", callback_data=f"adm_act:unban:{username}")])
    buttons.append(
        [
            InlineKeyboardButton("🔄 续期", callback_data=f"adm_renew:{username}"),
            InlineKeyboardButton("🗑️ 删除", callback_data=f"adm_act:del_confirm:{username}"),
        ]
    )
    buttons.append([InlineKeyboardButton("🔙 用户管理", callback_data="admin_users")])
    return InlineKeyboardMarkup(buttons)


async def _show_user_detail(message, user):
    """显示用户详情"""
    text = f"📋 **用户详情**\n\n{format_user_info(user)}"
    await safe_edit_message(message, text, reply_markup=_user_action_kb(user))
