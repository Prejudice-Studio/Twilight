"""
积分中心 + Inline 面板处理器

/score - 积分查询
/checkin - 签到（支持群组快捷签到）
/transfer - 转账
/ranking - 排行榜
/sendpack - 发红包
/grabpack - 抢红包
"""
import logging

from telegram import Update, InlineKeyboardMarkup, InlineKeyboardButton
from telegram.ext import ContextTypes, CommandHandler, CallbackQueryHandler

from src.bot.handlers.common import (
    require_registered, require_subscribe, require_private,
    require_panel, group_allowed, safe_edit_message, answer_callback_safe,
    back_button, close_button,
)
from src.db.user import UserOperate
from src.db.score import ScoreOperate
from src.services.score_service import ScoreService, RedPacketService, CheckinResult
from src.config import ScoreAndRegisterConfig

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.application
    score_name = ScoreAndRegisterConfig.SCORE_NAME

    # ======================== 积分面板入口 ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_panel_score(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """积分中心面板（callback）"""
        query = update.callback_query
        await answer_callback_safe(query)

        score_record = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score_record.SCORE if score_record else 0

        text = (
            f"💰 **积分中心**\n\n"
            f"👤 用户: `{user.USERNAME}`\n"
            f"💵 余额: **{balance}** {score_name}\n"
        )
        kb = _score_menu_kb()
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 签到 ========================

    @require_panel
    @group_allowed(delete_after=8, brief=True)
    @require_subscribe
    @require_registered
    async def cmd_checkin(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """签到命令（群组 & 私聊）"""
        result, response = await ScoreService.checkin(user.UID)
        if result == CheckinResult.SUCCESS:
            text = (
                f"✅ 签到成功！+**{response.score}** {score_name}\n"
                f"💰 余额: **{response.balance}** | 📅 连续: **{response.streak}**天"
            )
        elif result == CheckinResult.ALREADY_CHECKED:
            text = f"⚠️ 今日已签到 | 💰 余额: **{response.balance}** {score_name}"
        else:
            text = f"❌ 签到失败: {response.message}"
        await update.message.reply_text(text, parse_mode="Markdown")

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_score_checkin(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """签到回调"""
        query = update.callback_query
        result, response = await ScoreService.checkin(user.UID)
        if result == CheckinResult.SUCCESS:
            await answer_callback_safe(query, f"✅ +{response.score} {score_name}！连续{response.streak}天")
        elif result == CheckinResult.ALREADY_CHECKED:
            await answer_callback_safe(query, "⚠️ 今日已签到", show_alert=True)
        else:
            await answer_callback_safe(query, f"❌ {response.message}", show_alert=True)

        # 刷新面板
        score_record = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score_record.SCORE if score_record else 0
        text = (
            f"💰 **积分中心**\n\n"
            f"👤 用户: `{user.USERNAME}`\n"
            f"💵 余额: **{balance}** {score_name}\n"
        )
        await safe_edit_message(query.message, text, reply_markup=_score_menu_kb())

    # ======================== 排行榜 ========================

    @require_panel
    @require_subscribe
    async def cb_score_ranking(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """积分排行榜"""
        query = update.callback_query
        await answer_callback_safe(query)

        ranking = await ScoreService.get_ranking(limit=10)
        if not ranking:
            text = "📊 暂无排行数据"
        else:
            lines = [f"🏆 **{score_name}排行榜**\n"]
            medals = ["🥇", "🥈", "🥉"]
            for i, item in enumerate(ranking):
                medal = medals[i] if i < 3 else f"{i + 1}."
                lines.append(f"{medal} `{item['username']}` - **{item['score']}** {score_name}")
            text = "\n".join(lines)

        kb = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔄 刷新", callback_data="score_ranking")],
            [InlineKeyboardButton("🔙 积分中心", callback_data="panel_score")],
        ])
        await safe_edit_message(query.message, text, reply_markup=kb)

    # ======================== 转账（提示输入） ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_score_transfer(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """转账提示"""
        query = update.callback_query
        await answer_callback_safe(query)
        if not ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE:
            await answer_callback_safe(query, "⚠️ 转账功能未开启", show_alert=True)
            return
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="panel_score")]])
        await safe_edit_message(
            query.message,
            f"💸 **转账**\n\n请使用命令:\n`/transfer <用户名> <金额>`",
            reply_markup=kb,
        )

    # ======================== 红包 ========================

    @require_panel
    @require_subscribe
    @require_registered
    async def cb_score_redpacket(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """红包面板"""
        query = update.callback_query
        await answer_callback_safe(query)
        if not ScoreAndRegisterConfig.RED_PACKET_MODE:
            await answer_callback_safe(query, "⚠️ 红包功能未开启", show_alert=True)
            return
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔙 返回", callback_data="panel_score")]])
        await safe_edit_message(
            query.message,
            f"🧧 **红包**\n\n"
            f"• 发红包: `/sendpack <总金额> <数量>`\n"
            f"• 抢红包: `/grabpack <红包ID>`",
            reply_markup=kb,
        )

    # ======================== 传统命令（兼容） ========================

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_score(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """查看积分"""
        score_record = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score_record.SCORE if score_record else 0
        text = (
            f"💰 **积分信息**\n\n"
            f"👤 用户: `{user.USERNAME}`\n"
            f"💵 余额: **{balance}** {score_name}\n\n"
            f"📋 使用 /checkin 每日签到获取积分"
        )
        await update.message.reply_text(text, parse_mode="Markdown")

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_transfer(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """转账"""
        if not ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE:
            await update.message.reply_text("⚠️ 转账功能未开启")
            return
        if not context.args or len(context.args) < 2:
            await update.message.reply_text("用法: `/transfer <用户名> <金额>`", parse_mode="Markdown")
            return
        target_username = context.args[0]
        try:
            amount = int(context.args[1])
        except ValueError:
            await update.message.reply_text("❌ 金额必须是数字")
            return
        if amount <= 0:
            await update.message.reply_text("❌ 金额必须大于 0")
            return
        target_user = await UserOperate.get_user_by_username(target_username)
        if not target_user:
            await update.message.reply_text("❌ 目标用户不存在")
            return
        if target_user.UID == user.UID:
            await update.message.reply_text("❌ 不能转账给自己")
            return
        success, msg = await ScoreService.transfer(from_uid=user.UID, to_uid=target_user.UID, amount=amount)
        if success:
            await update.message.reply_text(
                f"✅ 向 `{target_username}` 转账 **{amount}** {score_name}", parse_mode="Markdown"
            )
        else:
            await update.message.reply_text(f"❌ 转账失败: {msg}")

    @require_private
    @require_panel
    @require_subscribe
    async def cmd_ranking(update: Update, context: ContextTypes.DEFAULT_TYPE):
        """积分排行榜"""
        ranking = await ScoreService.get_ranking(limit=10)
        if not ranking:
            await update.message.reply_text("📊 暂无排行数据")
            return
        lines = [f"🏆 **{score_name}排行榜**\n"]
        medals = ["🥇", "🥈", "🥉"]
        for i, item in enumerate(ranking):
            medal = medals[i] if i < 3 else f"{i + 1}."
            lines.append(f"{medal} `{item['username']}` - **{item['score']}** {score_name}")
        await update.message.reply_text("\n".join(lines), parse_mode="Markdown")

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_sendpack(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """发红包"""
        if not ScoreAndRegisterConfig.RED_PACKET_MODE:
            await update.message.reply_text("⚠️ 红包功能未开启")
            return
        if not context.args or len(context.args) < 2:
            await update.message.reply_text(
                "用法: `/sendpack <总金额> <数量>`\n示例: `/sendpack 100 5`", parse_mode="Markdown"
            )
            return
        try:
            total_amount = int(context.args[0])
            count = int(context.args[1])
        except ValueError:
            await update.message.reply_text("❌ 参数必须是数字")
            return
        if total_amount <= 0 or count <= 0:
            await update.message.reply_text("❌ 参数必须大于 0")
            return
        if total_amount < count:
            await update.message.reply_text("❌ 总金额不能小于红包数量")
            return
        success, msg, rp_key = await RedPacketService.create_red_packet(
            sender_uid=user.UID, total_amount=total_amount, count=count
        )
        if success:
            await update.message.reply_text(
                f"🧧 红包已发送！\n"
                f"💰 {total_amount} {score_name} × {count}个\n"
                f"🆔 `/grabpack {rp_key}`",
                parse_mode="Markdown",
            )
        else:
            await update.message.reply_text(f"❌ {msg}")

    @require_private
    @require_panel
    @require_subscribe
    @require_registered
    async def cmd_grabpack(update: Update, context: ContextTypes.DEFAULT_TYPE, user=None):
        """抢红包"""
        if not ScoreAndRegisterConfig.RED_PACKET_MODE:
            await update.message.reply_text("⚠️ 红包功能未开启")
            return
        if not context.args:
            await update.message.reply_text("用法: `/grabpack <红包Key>`", parse_mode="Markdown")
            return
        rp_key = context.args[0].strip()
        success, msg, amount = await RedPacketService.grab_red_packet(rp_key=rp_key, user_uid=user.UID)
        if success:
            await update.message.reply_text(f"🎉 抢到了！+**{amount}** {score_name}", parse_mode="Markdown")
        else:
            await update.message.reply_text(f"❌ {msg}")

    # ======================== 注册处理器 ========================

    # 命令
    app.add_handler(CommandHandler("score", cmd_score))
    app.add_handler(CommandHandler("checkin", cmd_checkin))
    app.add_handler(CommandHandler("transfer", cmd_transfer))
    app.add_handler(CommandHandler("ranking", cmd_ranking))
    app.add_handler(CommandHandler("sendpack", cmd_sendpack))
    app.add_handler(CommandHandler("grabpack", cmd_grabpack))

    # 面板回调
    app.add_handler(CallbackQueryHandler(cb_panel_score, pattern="^panel_score$"))
    app.add_handler(CallbackQueryHandler(cb_score_checkin, pattern="^score_checkin$"))
    app.add_handler(CallbackQueryHandler(cb_score_ranking, pattern="^score_ranking$"))
    app.add_handler(CallbackQueryHandler(cb_score_transfer, pattern="^score_transfer$"))
    app.add_handler(CallbackQueryHandler(cb_score_redpacket, pattern="^score_redpacket$"))


# ======================== 辅助函数 ========================

def _score_menu_kb() -> InlineKeyboardMarkup:
    """积分中心键盘"""
    buttons = [
        [
            InlineKeyboardButton("🎯 签到", callback_data="score_checkin"),
            InlineKeyboardButton("🏆 排行榜", callback_data="score_ranking"),
        ],
        [
            InlineKeyboardButton("💸 转账", callback_data="score_transfer"),
            InlineKeyboardButton("🧧 红包", callback_data="score_redpacket"),
        ],
        [InlineKeyboardButton("♻️ 主菜单", callback_data="back_start")],
    ]
    return InlineKeyboardMarkup(buttons)
