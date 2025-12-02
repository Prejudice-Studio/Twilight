"""
用户相关命令处理器

/start - 开始
/help - 帮助
/me - 我的信息
/bindtg - 绑定 Telegram
/unbindtg - 解绑 Telegram
/reg - 注册
"""
import logging

from pyrogram import filters
from pyrogram.types import Message, InlineKeyboardMarkup, InlineKeyboardButton

from src.bot.handlers.common import (
    require_registered, require_subscribe, 
    format_user_info, private_filter, escape_markdown
)
from src.db.user import UserOperate, Role
from src.db.regcode import RegCodeOperate
from src.services.user_service import UserService, RegisterResult
from src.config import Config, ScoreAndRegisterConfig

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.app
    
    @app.on_message(filters.command("start") & private_filter())
    async def cmd_start(client, message: Message):
        """开始命令"""
        user_name = message.from_user.first_name if message.from_user else "用户"
        
        text = f"""
👋 你好，**{escape_markdown(user_name)}**！

欢迎使用 **Twilight** Emby 管理机器人

📋 **常用命令**:
• /help - 查看帮助
• /me - 查看个人信息
• /score - 查看积分
• /checkin - 每日签到

🔗 **账号绑定**:
• /bindtg <用户名> - 绑定 Telegram
• /unbindtg - 解绑 Telegram

📝 **注册**:
• /reg <注册码> - 使用注册码注册
"""
        
        keyboard = InlineKeyboardMarkup([
            [InlineKeyboardButton("📖 帮助", callback_data="help")],
        ])
        
        await message.reply(text, reply_markup=keyboard)
    
    @app.on_message(filters.command("help") & private_filter())
    async def cmd_help(client, message: Message):
        """帮助命令"""
        text = """
📖 **命令帮助**

**👤 用户命令**
• /start - 开始使用
• /help - 显示此帮助
• /me - 查看个人信息
• /bindtg <用户名> - 绑定 Telegram
• /unbindtg - 解绑 Telegram

**💰 积分命令**
• /score - 查看积分余额
• /checkin - 每日签到
• /transfer <用户名> <金额> - 转账
• /ranking - 积分排行榜

**🎬 Emby 命令**
• /emby - Emby 服务器信息
• /lines - 查看线路
• /resetpwd - 重置密码

**📝 注册命令**
• /reg <注册码> - 注册账号

**🔴 红包命令**
• /sendpack <金额> <数量> - 发红包
• /grabpack <ID> - 抢红包
"""
        await message.reply(text)
    
    @app.on_message(filters.command("me") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_me(client, message: Message, user=None):
        """查看个人信息"""
        text = f"📋 **个人信息**\n\n{format_user_info(user)}"
        await message.reply(text)
    
    @app.on_message(filters.command("bindtg") & private_filter())
    async def cmd_bindtg(client, message: Message):
        """绑定 Telegram"""
        if not message.from_user:
            return
        
        telegram_id = message.from_user.id
        
        # 检查是否已绑定
        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            await message.reply(
                f"⚠️ 您已绑定账号: `{existing.USERNAME}`\n"
                "如需更换，请先使用 /unbindtg 解绑"
            )
            return
        
        # 获取用户名参数
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供用户名\n"
                "用法: `/bindtg <用户名>`"
            )
            return
        
        username = args[1]
        
        # 查找用户
        user = await UserOperate.get_user_by_username(username)
        if not user:
            await message.reply("❌ 用户不存在")
            return
        
        if user.TELEGRAM_ID:
            await message.reply("❌ 该账号已被其他 Telegram 绑定")
            return
        
        # 绑定
        await UserOperate.update_user(
            uid=user.UID,
            telegram_id=telegram_id
        )
        
        await message.reply(f"✅ 成功绑定账号: `{username}`")
        logger.info(f"用户 {username} 绑定 Telegram: {telegram_id}")
    
    @app.on_message(filters.command("unbindtg") & private_filter())
    @require_registered
    async def cmd_unbindtg(client, message: Message, user=None):
        """解绑 Telegram"""
        if Config.FORCE_BIND_TELEGRAM:
            await message.reply("⚠️ 系统要求强制绑定 Telegram，无法解绑")
            return
        
        await UserOperate.update_user(
            uid=user.UID,
            telegram_id=None
        )
        
        await message.reply("✅ 已解绑 Telegram")
        logger.info(f"用户 {user.USERNAME} 解绑 Telegram")
    
    @app.on_message(filters.command("reg") & private_filter())
    async def cmd_reg(client, message: Message):
        """注册命令"""
        if not message.from_user:
            return
        
        if not ScoreAndRegisterConfig.REGISTER_MODE:
            await message.reply("⚠️ 注册功能未开启")
            return
        
        telegram_id = message.from_user.id
        
        # 检查是否已有账号
        existing = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing:
            await message.reply(
                f"⚠️ 您已有账号: `{existing.USERNAME}`\n"
                "每个 Telegram 只能绑定一个账号"
            )
            return
        
        args = message.text.split()
        
        # 注册码注册
        if ScoreAndRegisterConfig.REGISTER_CODE_LIMIT:
            if len(args) < 2:
                await message.reply(
                    "❌ 请提供注册码\n"
                    "用法: `/reg <注册码>`"
                )
                return
            
            regcode = args[1]
            
            # 检查注册码
            code_record = await RegCodeOperate.get_regcode_by_code(regcode)
            if not code_record:
                await message.reply("❌ 无效的注册码")
                return
            
            if not code_record.ACTIVE:
                await message.reply("❌ 注册码已被使用")
                return
            
            # 生成用户名（使用 Telegram 用户名或 ID）
            tg_username = message.from_user.username
            if tg_username:
                username = tg_username
            else:
                username = f"user_{telegram_id}"
            
            # 检查用户名是否存在
            if await UserOperate.get_user_by_username(username):
                username = f"{username}_{telegram_id % 10000}"
            
            # 注册
            result, response = await UserService.register_with_regcode(
                username=username,
                regcode=regcode,
                telegram_id=telegram_id
            )
            
            if result == RegisterResult.SUCCESS:
                await message.reply(
                    f"✅ 注册成功！\n\n"
                    f"👤 用户名: `{response.username}`\n"
                    f"🔑 密码: `{response.password}`\n\n"
                    "请妥善保管您的密码！"
                )
            else:
                await message.reply(f"❌ 注册失败: {response.message}")
        else:
            # 自由注册
            tg_username = message.from_user.username
            if tg_username:
                username = tg_username
            else:
                username = f"user_{telegram_id}"
            
            if await UserOperate.get_user_by_username(username):
                username = f"{username}_{telegram_id % 10000}"
            
            result, response = await UserService.register(
                username=username,
                telegram_id=telegram_id
            )
            
            if result == RegisterResult.SUCCESS:
                await message.reply(
                    f"✅ 注册成功！\n\n"
                    f"👤 用户名: `{response.username}`\n"
                    f"🔑 密码: `{response.password}`\n\n"
                    "请妥善保管您的密码！"
                )
            else:
                await message.reply(f"❌ 注册失败: {response.message}")
    
    # 回调查询处理
    @app.on_callback_query(filters.regex("^help$"))
    async def callback_help(client, callback: CallbackQuery):
        """帮助回调"""
        await callback.answer()
        await cmd_help(client, callback.message)

