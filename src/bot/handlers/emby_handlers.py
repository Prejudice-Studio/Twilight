"""
Emby 相关命令处理器

/emby - Emby 信息
/lines - 线路信息
/resetpwd - 重置密码
/playinfo - 播放统计
"""
import logging

from pyrogram import filters
from pyrogram.types import Message, InlineKeyboardMarkup, InlineKeyboardButton

from src.bot.handlers.common import (
    require_registered, require_subscribe, require_admin, private_filter
)
from src.db.user import UserOperate
from src.services.emby_service import EmbyService
from src.services.user_service import UserService
from src.services.stats_service import StatsService
from src.config import EmbyConfig

logger = logging.getLogger(__name__)


def register(bot):
    """注册处理器"""
    app = bot.app
    
    @app.on_message(filters.command("emby") & private_filter())
    @require_subscribe
    async def cmd_emby(client, message: Message):
        """Emby 服务器信息"""
        try:
            status = await EmbyService.get_server_status()
            
            text = f"""
🎬 **Emby 服务器**

📊 **状态**: ✅ 在线
🏷️ **名称**: {status.get('server_name', '未知')}
📌 **版本**: {status.get('version', '未知')}

使用 /lines 查看可用线路
"""
        except Exception as e:
            text = f"""
🎬 **Emby 服务器**

📊 **状态**: ❌ 离线
⚠️ 服务器暂时无法连接
"""
        
        await message.reply(text)
    
    @app.on_message(filters.command("lines") & private_filter())
    @require_subscribe
    async def cmd_lines(client, message: Message):
        """线路信息"""
        lines = EmbyConfig.EMBY_URL_LIST
        
        if not lines:
            await message.reply("⚠️ 暂无可用线路")
            return
        
        text = "🌐 **可用线路**\n\n"
        for i, line in enumerate(lines, 1):
            text += f"{i}. {line}\n"
        
        text += "\n💡 请选择延迟最低的线路使用"
        
        await message.reply(text)
    
    @app.on_message(filters.command("resetpwd") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_resetpwd(client, message: Message):
        """重置密码"""
        user = await UserOperate.get_user_by_telegram_id(message.from_user.id)
        if not user:
            await message.reply("❌ 请先绑定账号")
            return
        
        # 生成新密码
        from src.core.utils import generate_password
        new_password = generate_password(12)
        
        # 重置 Emby 密码
        if user.EMBYID:
            try:
                await EmbyService.reset_password(user.EMBYID, new_password)
            except Exception as e:
                await message.reply(f"❌ 重置失败: {e}")
                return
        
        # 更新本地密码（如果存储的话）
        # 这里假设密码只在 Emby 端管理
        
        await message.reply(
            f"✅ **密码已重置**\n\n"
            f"🔑 新密码: `{new_password}`\n\n"
            "⚠️ 请妥善保管您的新密码！"
        )
    
    @app.on_message(filters.command("playinfo") & private_filter())
    @require_subscribe
    @require_registered
    async def cmd_playinfo(client, message: Message, user=None):
        """播放统计"""
        stats = await StatsService.get_user_stats(user.UID)
        
        if not stats:
            await message.reply("📊 暂无播放记录")
            return
        
        text = f"""
📊 **播放统计**

👤 用户: `{stats['username']}`

**📈 总计**
• 播放时长: {stats['total']['duration_str']}
• 播放次数: {stats['total']['play_count']} 次

**📅 今日**
• 播放时长: {stats['today']['duration_str']}
• 播放次数: {stats['today']['play_count']} 次
"""
        await message.reply(text)
    
    @app.on_message(filters.command("playrank") & private_filter())
    @require_subscribe
    async def cmd_playrank(client, message: Message):
        """播放排行榜"""
        args = message.text.split()
        period = args[1] if len(args) > 1 else 'today'
        
        if period not in ('today', 'week', 'month', 'all'):
            period = 'today'
        
        period_names = {
            'today': '今日',
            'week': '本周',
            'month': '本月',
            'all': '总'
        }
        
        ranking = await StatsService.get_ranking(period=period, limit=10)
        
        if not ranking:
            await message.reply("📊 暂无排行数据")
            return
        
        lines = [f"🏆 **{period_names[period]}播放排行榜**\n"]
        
        medals = ["🥇", "🥈", "🥉"]
        for item in ranking:
            medal = medals[item['rank'] - 1] if item['rank'] <= 3 else f"{item['rank']}."
            lines.append(f"{medal} `{item['username']}` - {item['value_str']}")
        
        keyboard = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("今日", callback_data="playrank_today"),
                InlineKeyboardButton("本周", callback_data="playrank_week"),
                InlineKeyboardButton("本月", callback_data="playrank_month"),
                InlineKeyboardButton("总榜", callback_data="playrank_all"),
            ]
        ])
        
        await message.reply("\n".join(lines), reply_markup=keyboard)
    
    @app.on_callback_query(filters.regex("^playrank_"))
    async def callback_playrank(client, callback):
        """排行榜回调"""
        period = callback.data.replace("playrank_", "")
        
        period_names = {
            'today': '今日',
            'week': '本周',
            'month': '本月',
            'all': '总'
        }
        
        ranking = await StatsService.get_ranking(period=period, limit=10)
        
        if not ranking:
            await callback.answer("暂无数据", show_alert=True)
            return
        
        lines = [f"🏆 **{period_names[period]}播放排行榜**\n"]
        
        medals = ["🥇", "🥈", "🥉"]
        for item in ranking:
            medal = medals[item['rank'] - 1] if item['rank'] <= 3 else f"{item['rank']}."
            lines.append(f"{medal} `{item['username']}` - {item['value_str']}")
        
        keyboard = InlineKeyboardMarkup([
            [
                InlineKeyboardButton("今日", callback_data="playrank_today"),
                InlineKeyboardButton("本周", callback_data="playrank_week"),
                InlineKeyboardButton("本月", callback_data="playrank_month"),
                InlineKeyboardButton("总榜", callback_data="playrank_all"),
            ]
        ])
        
        await callback.answer()
        await callback.message.edit_text("\n".join(lines), reply_markup=keyboard)
    
    @app.on_message(filters.command("sessions") & private_filter())
    @require_admin
    async def cmd_sessions(client, message: Message):
        """查看活跃会话（管理员）"""
        try:
            sessions = await EmbyService.get_all_sessions()
            
            if not sessions:
                await message.reply("📺 当前没有活跃会话")
                return
            
            lines = [f"📺 **活跃会话** ({len(sessions)} 个)\n"]
            
            for session in sessions[:10]:
                user_name = session.get('user_name', '未知')
                now_playing = session.get('now_playing', {})
                device = session.get('device_name', '未知设备')
                
                if now_playing:
                    media_name = now_playing.get('name', '未知')
                    lines.append(f"• **{user_name}** @ {device}\n  正在播放: {media_name}")
                else:
                    lines.append(f"• **{user_name}** @ {device}\n  空闲中")
            
            if len(sessions) > 10:
                lines.append(f"\n... 还有 {len(sessions) - 10} 个会话")
            
            await message.reply("\n".join(lines))
        except Exception as e:
            await message.reply(f"❌ 获取会话失败: {e}")
    
    @app.on_message(filters.command("kick") & private_filter())
    @require_admin
    async def cmd_kick(client, message: Message):
        """踢出用户会话（管理员）"""
        args = message.text.split()
        if len(args) < 2:
            await message.reply(
                "❌ 请提供用户名\n"
                "用法: `/kick <用户名>`"
            )
            return
        
        username = args[1]
        user = await UserOperate.get_user_by_username(username)
        
        if not user or not user.EMBYID:
            await message.reply("❌ 用户不存在或未绑定 Emby")
            return
        
        try:
            count = await EmbyService.kick_user_sessions(user.EMBYID)
            await message.reply(f"✅ 已踢出 `{username}` 的 {count} 个会话")
        except Exception as e:
            await message.reply(f"❌ 踢出失败: {e}")

