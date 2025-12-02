"""
管理员命令处理器

/admin - 管理面板
/adduser - 添加用户
/deluser - 删除用户
/ban - 禁用用户
/unban - 解禁用户
/renew - 续期
/regcode - 注册码管理
/broadcast - 广播消息
/stats - 系统统计
"""
import logging

from pyrogram import filters
from pyrogram.types import Message, InlineKeyboardMarkup, InlineKeyboardButton, CallbackQuery

from src.bot.handlers.common import (
    require_admin, private_filter, format_user_info, is_admin
)
from src.db.user import UserOperate, Role
from src.db.regcode import RegCodeOperate
from src.db.score import ScoreOperate
from src.services.user_service import UserService
from src.services.emby_service import EmbyService
from src.core.utils import generate_random_string, days_to_seconds, timestamp

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.app
    
    @app.on_message(filters.command("admin") & private_filter())
    @require_admin
    async def cmd_admin(client, message: Message):
        """管理面板"""
        keyboard = InlineKeyboardMarkup([
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
        ])
        
        await message.reply(
            "🔧 **管理面板**\n\n"
            "请选择要管理的功能:",
            reply_markup=keyboard
        )
    
    @app.on_message(filters.command("adduser") & private_filter())
    @require_admin
    async def cmd_adduser(client, message: Message):
        """添加用户"""
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 参数不足\n"
                "用法: `/adduser <用户名> [天数]`\n"
                "示例: `/adduser test 30`"
            )
            return
        
        username = args[1]
        days = int(args[2]) if len(args) > 2 else 30
        
        # 检查用户名是否存在
        if await UserOperate.get_user_by_username(username):
            await message.reply("❌ 用户名已存在")
            return
        
        # 创建用户
        result, response = await UserService.register(
            username=username,
            days=days
        )
        
        if result.value == 0:  # SUCCESS
            await message.reply(
                f"✅ **用户创建成功！**\n\n"
                f"👤 用户名: `{response.username}`\n"
                f"🔑 密码: `{response.password}`\n"
                f"⏰ 有效期: **{days}** 天"
            )
        else:
            await message.reply(f"❌ 创建失败: {response.message}")
    
    @app.on_message(filters.command("deluser") & private_filter())
    @require_admin
    async def cmd_deluser(client, message: Message):
        """删除用户"""
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供用户名\n"
                "用法: `/deluser <用户名>`"
            )
            return
        
        username = args[1]
        user = await UserOperate.get_user_by_username(username)
        
        if not user:
            await message.reply("❌ 用户不存在")
            return
        
        # 删除 Emby 用户
        if user.EMBYID:
            try:
                await EmbyService.delete_user(user.EMBYID)
            except Exception as e:
                logger.warning(f"删除 Emby 用户失败: {e}")
        
        # 删除本地用户
        await UserOperate.delete_user(user.UID)
        
        await message.reply(f"✅ 已删除用户: `{username}`")
    
    @app.on_message(filters.command("ban") & private_filter())
    @require_admin
    async def cmd_ban(client, message: Message):
        """禁用用户"""
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供用户名\n"
                "用法: `/ban <用户名>`"
            )
            return
        
        username = args[1]
        user = await UserOperate.get_user_by_username(username)
        
        if not user:
            await message.reply("❌ 用户不存在")
            return
        
        if not user.ACTIVE_STATUS:
            await message.reply("⚠️ 用户已被禁用")
            return
        
        # 禁用 Emby
        if user.EMBYID:
            try:
                await EmbyService.disable_user(user.EMBYID)
            except Exception as e:
                logger.warning(f"禁用 Emby 用户失败: {e}")
        
        # 禁用本地
        await UserOperate.update_user(user.UID, active_status=False)
        
        await message.reply(f"✅ 已禁用用户: `{username}`")
    
    @app.on_message(filters.command("unban") & private_filter())
    @require_admin
    async def cmd_unban(client, message: Message):
        """解禁用户"""
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供用户名\n"
                "用法: `/unban <用户名>`"
            )
            return
        
        username = args[1]
        user = await UserOperate.get_user_by_username(username)
        
        if not user:
            await message.reply("❌ 用户不存在")
            return
        
        if user.ACTIVE_STATUS:
            await message.reply("⚠️ 用户未被禁用")
            return
        
        # 解禁 Emby
        if user.EMBYID:
            try:
                await EmbyService.enable_user(user.EMBYID)
            except Exception as e:
                logger.warning(f"解禁 Emby 用户失败: {e}")
        
        # 解禁本地
        await UserOperate.update_user(user.UID, active_status=True)
        
        await message.reply(f"✅ 已解禁用户: `{username}`")
    
    @app.on_message(filters.command("renew") & private_filter())
    @require_admin
    async def cmd_renew(client, message: Message):
        """续期"""
        args = message.text.split()
        if len(args) < 3:
            await message.reply(
                "❌ 参数不足\n"
                "用法: `/renew <用户名> <天数>`\n"
                "示例: `/renew test 30`"
            )
            return
        
        username = args[1]
        try:
            days = int(args[2])
        except ValueError:
            await message.reply("❌ 天数必须是数字")
            return
        
        user = await UserOperate.get_user_by_username(username)
        if not user:
            await message.reply("❌ 用户不存在")
            return
        
        # 续期
        success, msg = await UserService.renew(user.UID, days)
        
        if success:
            await message.reply(f"✅ 已为 `{username}` 续期 **{days}** 天")
        else:
            await message.reply(f"❌ 续期失败: {msg}")
    
    @app.on_message(filters.command("regcode") & private_filter())
    @require_admin
    async def cmd_regcode(client, message: Message):
        """注册码管理"""
        args = message.text.split()
        
        if len(args) < 2:
            # 显示帮助
            await message.reply(
                "🎫 **注册码管理**\n\n"
                "用法:\n"
                "• `/regcode new [天数] [数量]` - 生成注册码\n"
                "• `/regcode list` - 列出注册码\n"
                "• `/regcode del <注册码>` - 删除注册码"
            )
            return
        
        action = args[1].lower()
        
        if action == "new":
            days = int(args[2]) if len(args) > 2 else 30
            count = int(args[3]) if len(args) > 3 else 1
            count = min(count, 10)  # 最多一次生成 10 个
            
            codes = []
            for _ in range(count):
                code = generate_random_string(16).upper()
                await RegCodeOperate.add_regcode(
                    code=code,
                    days=days,
                    reg_type=1  # 普通用户
                )
                codes.append(f"`{code}` ({days}天)")
            
            await message.reply(
                f"✅ **已生成 {count} 个注册码**\n\n" +
                "\n".join(codes)
            )
        
        elif action == "list":
            codes = await RegCodeOperate.get_all_regcodes(active_only=True)
            
            if not codes:
                await message.reply("📋 暂无可用注册码")
                return
            
            lines = ["🎫 **可用注册码**\n"]
            for code in codes[:20]:  # 最多显示 20 个
                lines.append(f"• `{code.CODE}` - {code.DAYS}天")
            
            if len(codes) > 20:
                lines.append(f"\n... 还有 {len(codes) - 20} 个")
            
            await message.reply("\n".join(lines))
        
        elif action == "del":
            if len(args) < 3:
                await message.reply("❌ 请提供注册码")
                return
            
            code = args[2]
            success = await RegCodeOperate.delete_regcode(code)
            
            if success:
                await message.reply(f"✅ 已删除注册码: `{code}`")
            else:
                await message.reply("❌ 注册码不存在")
    
    @app.on_message(filters.command("broadcast") & private_filter())
    @require_admin
    async def cmd_broadcast(client, message: Message):
        """广播消息"""
        if len(message.text.split(None, 1)) < 2:
            await message.reply(
                "❌ 请提供广播内容\n"
                "用法: `/broadcast <消息内容>`"
            )
            return
        
        content = message.text.split(None, 1)[1]
        
        # 获取所有有 Telegram ID 的用户
        from src.db.user import UsersSessionFactory
        from sqlalchemy import select
        from src.db.user import UserModel
        
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel.TELEGRAM_ID).where(
                    UserModel.TELEGRAM_ID != None,
                    UserModel.ACTIVE_STATUS == True
                )
            )
            telegram_ids = [row[0] for row in result.all()]
        
        if not telegram_ids:
            await message.reply("⚠️ 没有可发送的用户")
            return
        
        # 发送广播
        success = 0
        failed = 0
        
        progress_msg = await message.reply(f"📢 正在广播... (0/{len(telegram_ids)})")
        
        for i, tg_id in enumerate(telegram_ids):
            try:
                await client.send_message(
                    chat_id=tg_id,
                    text=f"📢 **系统通知**\n\n{content}"
                )
                success += 1
            except Exception:
                failed += 1
            
            # 更新进度
            if (i + 1) % 10 == 0:
                await progress_msg.edit_text(f"📢 正在广播... ({i + 1}/{len(telegram_ids)})")
        
        await progress_msg.edit_text(
            f"✅ **广播完成**\n\n"
            f"📤 成功: {success}\n"
            f"❌ 失败: {failed}"
        )
    
    @app.on_message(filters.command("stats") & private_filter())
    @require_admin
    async def cmd_stats(client, message: Message):
        """系统统计"""
        # 用户统计
        total_users, _ = await UserOperate.get_all_users(limit=1)
        active_users, _ = await UserOperate.get_all_users(active_status=True, limit=1)
        
        # 积分统计（简化）
        # 注册码统计
        all_codes = await RegCodeOperate.get_all_regcodes()
        active_codes = [c for c in all_codes if c.ACTIVE]
        
        # Emby 统计
        try:
            emby_status = await EmbyService.get_server_status()
            emby_info = (
                f"🎬 **Emby 服务器**\n"
                f"• 状态: ✅ 在线\n"
                f"• 版本: {emby_status.get('version', '未知')}"
            )
        except Exception:
            emby_info = "🎬 **Emby 服务器**\n• 状态: ❌ 离线"
        
        text = f"""
📊 **系统统计**

👥 **用户**
• 总用户数: {total_users if isinstance(total_users, int) else len(total_users)}
• 活跃用户: {active_users if isinstance(active_users, int) else len(active_users)}

🎫 **注册码**
• 总数: {len(all_codes)}
• 可用: {len(active_codes)}

{emby_info}
"""
        await message.reply(text)
    
    @app.on_message(filters.command("userinfo") & private_filter())
    @require_admin
    async def cmd_userinfo(client, message: Message):
        """查看用户详情"""
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供用户名\n"
                "用法: `/userinfo <用户名>`"
            )
            return
        
        username = args[1]
        user = await UserOperate.get_user_by_username(username)
        
        if not user:
            await message.reply("❌ 用户不存在")
            return
        
        # 获取积分
        score = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score.BALANCE if score else 0
        
        text = f"📋 **用户详情**\n\n{format_user_info(user)}\n💰 积分: **{balance}**"
        
        keyboard = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("🔄 续期", callback_data=f"admin_renew_{user.UID}"),
                InlineKeyboardButton("🚫 禁用" if user.ACTIVE_STATUS else "✅ 启用", 
                                   callback_data=f"admin_toggle_{user.UID}"),
            ],
            [
                InlineKeyboardButton("🗑️ 删除", callback_data=f"admin_delete_{user.UID}"),
            ],
        ])
        
        await message.reply(text, reply_markup=keyboard)
    
    # 回调处理
    @app.on_callback_query(filters.regex("^admin_"))
    async def callback_admin(client, callback: CallbackQuery):
        """管理回调"""
        if not is_admin(callback.from_user.id):
            await callback.answer("⚠️ 仅限管理员", show_alert=True)
            return
        
        data = callback.data
        
        if data == "admin_stats":
            await callback.answer()
            # 复用 stats 命令
            await cmd_stats(client, callback.message)
        
        elif data == "admin_users":
            await callback.answer()
            await callback.message.edit_text(
                "👥 **用户管理**\n\n"
                "请使用以下命令:\n"
                "• `/adduser <用户名> [天数]` - 添加用户\n"
                "• `/deluser <用户名>` - 删除用户\n"
                "• `/ban <用户名>` - 禁用用户\n"
                "• `/unban <用户名>` - 解禁用户\n"
                "• `/renew <用户名> <天数>` - 续期\n"
                "• `/userinfo <用户名>` - 查看详情"
            )
        
        elif data == "admin_regcode":
            await callback.answer()
            await callback.message.edit_text(
                "🎫 **注册码管理**\n\n"
                "请使用以下命令:\n"
                "• `/regcode new [天数] [数量]` - 生成\n"
                "• `/regcode list` - 列表\n"
                "• `/regcode del <注册码>` - 删除"
            )
        
        elif data == "admin_emby":
            await callback.answer()
            try:
                status = await EmbyService.get_server_status()
                text = (
                    f"🎬 **Emby 服务器**\n\n"
                    f"• 状态: ✅ 在线\n"
                    f"• 版本: {status.get('version', '未知')}\n"
                    f"• 名称: {status.get('server_name', '未知')}"
                )
            except Exception as e:
                text = f"🎬 **Emby 服务器**\n\n• 状态: ❌ 离线\n• 错误: {e}"
            
            await callback.message.edit_text(text)
        
        elif data == "admin_broadcast":
            await callback.answer()
            await callback.message.edit_text(
                "📢 **广播消息**\n\n"
                "用法: `/broadcast <消息内容>`\n\n"
                "消息将发送给所有绑定 Telegram 的用户"
            )

