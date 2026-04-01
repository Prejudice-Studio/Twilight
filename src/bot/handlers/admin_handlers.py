"""
管理员命令 + Inline 面板处理器

/admin - 管理面板（inline 按钮）
支持用户管理、注册码、统计、Emby、广播等
"""
import logging

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.ext import ContextTypes, CommandHandler, CallbackQueryHandler, MessageHandler, filters

from src.bot.handlers.common import (
    require_admin, require_private,
    format_user_info, is_admin, safe_edit_message, answer_callback_safe,
    back_button, close_button,
)
from src.db.user import UserOperate, Role
from src.db.regcode import RegCodeOperate
from src.db.score import ScoreOperate
from src.services.user_service import UserService, RegisterResult
from src.services.emby_service import EmbyService
from src.services.emby import get_emby_client
from src.core.utils import generate_random_string, generate_password, days_to_seconds, timestamp

logger = logging.getLogger(__name__)

# 会话状态存储（admin_id -> state dict）
_admin_states = {}


def register(bot):
    """注册处理器"""
    app = bot.application

    # ======================== 管理面板入口 ========================

    @require_private
    @require_admin
    async def cmd_admin(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """管理面板命令"""
        await update.message.reply_text(
            "🔧 **管理面板**\n\n请选择功能：",
            reply_markup=_admin_menu_kb(),
            parse_mode="Markdown",
        )

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
        kb = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("➕ 添加用户", callback_data="adm_adduser"),
                InlineKeyboardButton("🔍 查询用户", callback_data="adm_queryuser"),
            ],
            [
                InlineKeyboardButton("👥 用户列表", callback_data="adm_userlist:1"),
                InlineKeyboardButton("🚫 禁用/解禁", callback_data="adm_banmenu"),
            ],
            [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
        ])
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

        kb = InlineKeyboardMarkup([
            nav,
            [InlineKeyboardButton("🔙 返回", callback_data="admin_users")],
        ])
        await safe_edit_message(query.message, "\n".join(lines), reply_markup=kb)

    @require_admin
    async def cb_adm_queryuser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """查询用户 - 提示输入"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _admin_states[uid] = {"action": "query_user"}
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="admin_users")]])
        await safe_edit_message(query.message, "🔍 **查询用户**\n\n请发送用户名：", reply_markup=kb)

    @require_admin
    async def cb_adm_adduser(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """添加用户 - 提示输入"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _admin_states[uid] = {"action": "add_user"}
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="admin_users")]])
        await safe_edit_message(
            query.message,
            "➕ **添加用户**\n\n请发送: `用户名 天数`\n示例: `test 30`",
            reply_markup=kb,
        )

    @require_admin
    async def cb_adm_banmenu(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """禁用/解禁菜单"""
        query = update.callback_query
        await answer_callback_safe(query)
        uid = update.effective_user.id
        _admin_states[uid] = {"action": "ban_user"}
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="admin_users")]])
        await safe_edit_message(
            query.message,
            "🚫 **禁用/解禁用户**\n\n请发送用户名，将自动切换其状态：",
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
            kb = InlineKeyboardMarkup([
                [
                    InlineKeyboardButton("⚠️ 确认删除", callback_data=f"adm_act:del:{username}"),
                    InlineKeyboardButton("🔙 取消", callback_data=f"adm_userdetail:{username}"),
                ]
            ])
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
        _admin_states[uid] = {"action": "renew_user", "username": username}
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data=f"adm_userdetail:{username}")]])
        await safe_edit_message(query.message, f"🔄 **续期 `{username}`**\n\n请发送天数：", reply_markup=kb)

    # ======================== 注册码管理 ========================

    @require_admin
    async def cb_admin_regcode(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """注册码面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("🆕 生成注册码", callback_data="adm_regcode_gen"),
                InlineKeyboardButton("📋 查看列表", callback_data="adm_regcode_list"),
            ],
            [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
        ])
        await safe_edit_message(query.message, "🎫 **注册码管理**\n\n请选择操作：", reply_markup=kb)

    @require_admin
    async def cb_adm_regcode_gen(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """生成注册码选择天数"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup([
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
        ])
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

        text = f"✅ **生成 {count} 个注册码** ({days}天)\n\n" + "\n".join(codes)
        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔁 再生成", callback_data="adm_regcode_gen")],
            [InlineKeyboardButton("🔙 返回", callback_data="admin_regcode")],
        ])
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
                lines.append(f"• `{c.CODE}` - {c.DAYS}天")
            if len(codes) > 15:
                lines.append(f"\n... 还有 {len(codes) - 15} 个")
            text = "\n".join(lines)
        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🆕 生成", callback_data="adm_regcode_gen")],
            [InlineKeyboardButton("🔙 返回", callback_data="admin_regcode")],
        ])
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
            emby_info = (
                f"🎬 **Emby**\n"
                f"• 状态: ✅ 在线\n"
                f"• 版本: {emby_status.get('version', '未知')}"
            )
        except Exception:
            emby_info = "🎬 **Emby**\n• 状态: ❌ 离线"

        text = (
            f"📊 **系统统计**\n\n"
            f"👥 **用户**: {total_count} (活跃: {active_count})\n"
            f"🎫 **注册码**: {len(all_codes)} (可用: {len(active_codes_list)})\n\n"
            f"{emby_info}"
        )
        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔄 刷新", callback_data="admin_stats")],
            [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
        ])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== Emby 管理 ========================

    @require_admin
    async def cb_admin_emby(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """Emby 管理面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        kb = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("🔌 连接测试", callback_data="adm_emby_test"),
                InlineKeyboardButton("📺 活跃会话", callback_data="adm_emby_sessions"),
            ],
            [
                InlineKeyboardButton("👥 Emby 用户", callback_data="adm_emby_users"),
                InlineKeyboardButton("🧹 清理孤儿", callback_data="adm_emby_cleanup"),
            ],
            [InlineKeyboardButton("🔙 管理面板", callback_data="panel_admin")],
        ])
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
                    name = s.get('user_name', '未知')
                    dev = s.get('device_name', '?')
                    np = s.get('now_playing', {})
                    if np:
                        lines.append(f"• **{name}** @ {dev}\n  ▶️ {np.get('name', '?')}")
                    else:
                        lines.append(f"• **{name}** @ {dev} (空闲)")
                text = "\n".join(lines)
        except Exception as e:
            text = f"❌ 获取失败: {e}"
        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔄 刷新", callback_data="adm_emby_sessions")],
            [InlineKeyboardButton("🔙 返回", callback_data="admin_emby")],
        ])
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
        kb = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("⚠️ 确认清理", callback_data="adm_emby_cleanup_confirm"),
                InlineKeyboardButton("🔙 取消", callback_data="admin_emby"),
            ]
        ])
        await safe_edit_message(query.message, "🧹 **清理孤儿用户**\n\n将删除 Emby 中存在但系统中无记录的用户。\n确认执行？", reply_markup=kb)

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
        _admin_states[uid] = {"action": "broadcast"}
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("❌ 取消", callback_data="panel_admin")]])
        await safe_edit_message(query.message, "📢 **广播消息**\n\n请发送要广播的内容：", reply_markup=kb)

    # ======================== 文本消息路由（admin 状态机）========================

    @require_admin
    async def handle_admin_text(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """处理管理员文本输入（根据状态机路由）"""
        uid = update.effective_user.id
        state = _admin_states.pop(uid, None)
        if not state:
            return  # 无状态，忽略

        action = state.get("action")
        text = update.message.text.strip()

        if action == "query_user":
            user = await UserOperate.get_user_by_username(text)
            if not user:
                await update.message.reply_text(f"❌ 用户 `{text}` 不存在", parse_mode="Markdown")
                return
            score = await ScoreOperate.get_score_by_uid(user.UID)
            balance = score.SCORE if score else 0
            info = f"📋 **用户详情**\n\n{format_user_info(user)}\n💰 积分: **{balance}**"
            kb = _user_action_kb(user)
            await update.message.reply_text(info, reply_markup=kb, parse_mode="Markdown")

        elif action == "add_user":
            parts = text.split()
            username = parts[0]
            days = int(parts[1]) if len(parts) > 1 else 30
            if await UserOperate.get_user_by_username(username):
                await update.message.reply_text("❌ 用户名已存在")
                return
            resp = await UserService._create_emby_user(
                telegram_id=None, username=username, email=None, days=days
            )
            if resp.result == RegisterResult.SUCCESS:
                await update.message.reply_text(
                    f"✅ **用户创建成功**\n\n"
                    f"👤 用户名: `{username}`\n"
                    f"🔑 密码: `{resp.emby_password}`\n"
                    f"⏰ 有效期: **{days}** 天",
                    parse_mode="Markdown",
                )
            else:
                await update.message.reply_text(f"❌ 创建失败: {resp.message}")

        elif action == "ban_user":
            user = await UserOperate.get_user_by_username(text)
            if not user:
                await update.message.reply_text(f"❌ 用户 `{text}` 不存在", parse_mode="Markdown")
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

        elif action == "renew_user":
            username = state.get("username", "")
            try:
                days = int(text)
            except ValueError:
                await update.message.reply_text("❌ 请输入数字天数")
                return
            user = await UserOperate.get_user_by_username(username)
            if not user:
                await update.message.reply_text("❌ 用户不存在")
                return
            success, msg = await UserService.renew_user(user, days)
            if success:
                await update.message.reply_text(f"✅ 已为 `{username}` 续期 **{days}** 天", parse_mode="Markdown")
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
        resp = await UserService._create_emby_user(
            telegram_id=None, username=username, email=None, days=days
        )
        if resp.result == RegisterResult.SUCCESS:
            await update.message.reply_text(
                f"✅ 创建成功\n👤 `{username}`\n🔑 `{resp.emby_password}`\n⏰ {days}天",
                parse_mode="Markdown",
            )
        else:
            await update.message.reply_text(f"❌ {resp.message}")

    @require_private
    @require_admin
    async def cmd_regcode(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/regcode new [天数] [数量]` | `/regcode list`", parse_mode="Markdown")
            return
        action = context.args[0].lower()
        if action == "new":
            days = int(context.args[1]) if len(context.args) > 1 else 30
            count = min(int(context.args[2]) if len(context.args) > 2 else 1, 10)
            result = await RegCodeOperate.create_regcode(vali_time=-1, type_=1, day=days, count=count)
            if isinstance(result, str):
                result = [result]
            codes = [f"`{c}` ({days}天)" for c in result]
            await update.message.reply_text(f"✅ 生成 {count} 个注册码\n\n" + "\n".join(codes), parse_mode="Markdown")
        elif action == "list":
            all_codes = await RegCodeOperate.get_all_regcodes()
            codes = [c for c in all_codes if c.ACTIVE]
            if not codes:
                await update.message.reply_text("📋 暂无可用注册码")
            else:
                lines = [f"🎫 **可用注册码** ({len(codes)}个)\n"]
                for c in codes[:20]:
                    lines.append(f"• `{c.CODE}` - {c.DAYS}天")
                await update.message.reply_text("\n".join(lines), parse_mode="Markdown")

    @require_private
    @require_admin
    async def cmd_broadcast(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if not context.args:
            await update.message.reply_text("用法: `/broadcast <内容>`", parse_mode="Markdown")
            return
        content = update.message.text.split(None, 1)[1]
        _admin_states[update.effective_user.id] = None  # clear
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
        score = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score.SCORE if score else 0
        text = f"📋 **用户详情**\n\n{format_user_info(user)}\n💰 积分: **{balance}**"
        await update.message.reply_text(text, reply_markup=_user_action_kb(user), parse_mode="Markdown")

    # ======================== 注册处理器 ========================

    # 命令
    app.add_handler(CommandHandler("admin", cmd_admin))
    app.add_handler(CommandHandler("adduser", cmd_adduser))
    app.add_handler(CommandHandler("regcode", cmd_regcode))
    app.add_handler(CommandHandler("broadcast", cmd_broadcast))
    app.add_handler(CommandHandler("stats", cmd_stats))
    app.add_handler(CommandHandler("userinfo", cmd_userinfo))

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

    # noop
    app.add_handler(CallbackQueryHandler(lambda u, c: answer_callback_safe(u.callback_query), pattern="^noop$"))

    # 文本消息（admin 状态机）- 优先级较低
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND & filters.ChatType.PRIVATE, handle_admin_text), group=1)


# ======================== 辅助函数 ========================

def _admin_menu_kb() -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup([
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
    ])


def _user_action_kb(user) -> InlineKeyboardMarkup:
    """用户操作键盘"""
    username = user.USERNAME
    buttons = []
    if user.ACTIVE_STATUS:
        buttons.append([InlineKeyboardButton("🚫 禁用", callback_data=f"adm_act:ban:{username}")])
    else:
        buttons.append([InlineKeyboardButton("✅ 解禁", callback_data=f"adm_act:unban:{username}")])
    buttons.append([
        InlineKeyboardButton("🔄 续期", callback_data=f"adm_renew:{username}"),
        InlineKeyboardButton("🗑️ 删除", callback_data=f"adm_act:del_confirm:{username}"),
    ])
    buttons.append([InlineKeyboardButton("🔙 用户管理", callback_data="admin_users")])
    return InlineKeyboardMarkup(buttons)


async def _show_user_detail(message, user):
    """显示用户详情"""
    score = await ScoreOperate.get_score_by_uid(user.UID)
    balance = score.SCORE if score else 0
    text = f"📋 **用户详情**\n\n{format_user_info(user)}\n💰 积分: **{balance}**"
    await safe_edit_message(message, text, reply_markup=_user_action_kb(user))
