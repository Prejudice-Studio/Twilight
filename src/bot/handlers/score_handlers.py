"""
积分相关命令处理器

/score - 查看积分
/checkin - 签到
/transfer - 转账
/ranking - 排行榜
/sendpack - 发红包
/grabpack - 抢红包
"""
import logging

from pyrogram import filters
from pyrogram.types import Message, InlineKeyboardMarkup, InlineKeyboardButton

from src.bot.handlers.common import (
    require_registered, require_subscribe, private_filter
)
from src.db.user import UserOperate
from src.db.score import ScoreOperate
from src.services.score_service import ScoreService, RedPacketService, CheckinResult
from src.config import ScoreAndRegisterConfig

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.app
    score_name = ScoreAndRegisterConfig.SCORE_NAME
    
    @app.on_message(filters.command("score") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_score(client, message: Message, user=None):
        """查看积分"""
        score_record = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score_record.BALANCE if score_record else 0
        
        text = f"""
💰 **积分信息**

👤 用户: `{user.USERNAME}`
💵 余额: **{balance}** {score_name}

📋 使用 /checkin 每日签到获取积分
"""
        await message.reply(text)
    
    @app.on_message(filters.command("checkin") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_checkin(client, message: Message, user=None):
        """每日签到"""
        result, response = await ScoreService.checkin(user.UID)
        
        if result == CheckinResult.SUCCESS:
            text = f"""
✅ **签到成功！**

🎁 获得: **+{response.score}** {score_name}
💰 当前余额: **{response.balance}** {score_name}
📅 连续签到: **{response.streak}** 天
"""
        elif result == CheckinResult.ALREADY_CHECKED:
            text = f"""
⚠️ **今日已签到**

💰 当前余额: **{response.balance}** {score_name}
📅 连续签到: **{response.streak}** 天

明天再来吧！
"""
        else:
            text = f"❌ 签到失败: {response.message}"
        
        await message.reply(text)
    
    @app.on_message(filters.command("transfer") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_transfer(client, message: Message, user=None):
        """转账"""
        if not ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE:
            await message.reply("⚠️ 转账功能未开启")
            return
        
        args = message.text.split()
        if len(args) < 3:
            await message.reply(
                "❌ 参数不足\n"
                "用法: `/transfer <用户名> <金额>`"
            )
            return
        
        target_username = args[1]
        try:
            amount = int(args[2])
        except ValueError:
            await message.reply("❌ 金额必须是数字")
            return
        
        if amount <= 0:
            await message.reply("❌ 金额必须大于 0")
            return
        
        # 查找目标用户
        target_user = await UserOperate.get_user_by_username(target_username)
        if not target_user:
            await message.reply("❌ 目标用户不存在")
            return
        
        if target_user.UID == user.UID:
            await message.reply("❌ 不能转账给自己")
            return
        
        # 执行转账
        success, msg = await ScoreService.transfer(
            from_uid=user.UID,
            to_uid=target_user.UID,
            amount=amount
        )
        
        if success:
            await message.reply(
                f"✅ **转账成功！**\n\n"
                f"💸 向 `{target_username}` 转账 **{amount}** {score_name}"
            )
        else:
            await message.reply(f"❌ 转账失败: {msg}")
    
    @app.on_message(filters.command("ranking") & private_filter())
    @require_subscribe
    async def cmd_ranking(client, message: Message):
        """积分排行榜"""
        ranking = await ScoreService.get_ranking(limit=10)
        
        if not ranking:
            await message.reply("📊 暂无排行数据")
            return
        
        lines = [f"🏆 **{score_name}排行榜**\n"]
        
        medals = ["🥇", "🥈", "🥉"]
        for i, item in enumerate(ranking):
            medal = medals[i] if i < 3 else f"{i + 1}."
            lines.append(f"{medal} `{item['username']}` - **{item['balance']}** {score_name}")
        
        await message.reply("\n".join(lines))
    
    @app.on_message(filters.command("sendpack") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_sendpack(client, message: Message, user=None):
        """发红包"""
        if not ScoreAndRegisterConfig.RED_PACKET_MODE:
            await message.reply("⚠️ 红包功能未开启")
            return
        
        args = message.text.split()
        if len(args) < 3:
            await message.reply(
                "❌ 参数不足\n"
                "用法: `/sendpack <总金额> <数量>`\n"
                "示例: `/sendpack 100 5` 发 100 积分分给 5 人"
            )
            return
        
        try:
            total_amount = int(args[1])
            count = int(args[2])
        except ValueError:
            await message.reply("❌ 参数必须是数字")
            return
        
        if total_amount <= 0 or count <= 0:
            await message.reply("❌ 参数必须大于 0")
            return
        
        if total_amount < count:
            await message.reply("❌ 总金额不能小于红包数量")
            return
        
        # 发红包
        success, result = await RedPacketService.create(
            sender_uid=user.UID,
            total_amount=total_amount,
            count=count,
            chat_id=message.chat.id
        )
        
        if success:
            await message.reply(
                f"🧧 **红包已发送！**\n\n"
                f"💰 总金额: **{total_amount}** {score_name}\n"
                f"📦 数量: **{count}** 个\n"
                f"🆔 红包ID: `{result['packet_id']}`\n\n"
                f"使用 `/grabpack {result['packet_id']}` 抢红包"
            )
        else:
            await message.reply(f"❌ 发红包失败: {result}")
    
    @app.on_message(filters.command("grabpack") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_grabpack(client, message: Message, user=None):
        """抢红包"""
        if not ScoreAndRegisterConfig.RED_PACKET_MODE:
            await message.reply("⚠️ 红包功能未开启")
            return
        
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供红包 ID\n"
                "用法: `/grabpack <红包ID>`"
            )
            return
        
        try:
            packet_id = int(args[1])
        except ValueError:
            await message.reply("❌ 无效的红包 ID")
            return
        
        # 抢红包
        success, result = await RedPacketService.grab(
            packet_id=packet_id,
            user_uid=user.UID
        )
        
        if success:
            await message.reply(
                f"🎉 **抢到了！**\n\n"
                f"💰 获得: **{result['amount']}** {score_name}"
            )
        else:
            await message.reply(f"❌ {result}")

