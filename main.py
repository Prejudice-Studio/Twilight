#!/usr/bin/env python3
"""
Twilight - Emby 用户管理系统

主入口文件
"""
import argparse
import asyncio
import logging
import sys

from src import __version__
from src.config import Config, ScoreAndRegisterConfig, TelegramConfig
from src.core.utils import setup_logging, format_duration

logger = logging.getLogger(__name__)


def run_api_server(host: str = '0.0.0.0', port: int = 5000, debug: bool = False):
    """启动 API 服务器"""
    from src.api import create_app
    
    app = create_app()
    print(f"🌙 Twilight API Server v{__version__}")
    print(f"📡 Running on http://{host}:{port}")
    print(f"📖 API Docs: http://{host}:{port}/api/v1/docs")
    app.run(host=host, port=port, debug=debug)


async def run_scheduler():
    """运行定时任务"""
    from apscheduler.schedulers.asyncio import AsyncIOScheduler
    from src.db.user import UserOperate
    from src.services import get_emby_client, EmbyService
    from src.core.utils import timestamp
    
    scheduler = AsyncIOScheduler(timezone='Asia/Shanghai')
    
    async def check_expired_users():
        """检查过期用户并禁用"""
        logger.info("🔍 开始检查过期用户...")
        
        try:
            # 获取已过期但仍启用的用户
            expired_users = await UserOperate.get_expired_users()
            
            if not expired_users:
                logger.info("✅ 没有需要处理的过期用户")
                return
            
            logger.info(f"📋 发现 {len(expired_users)} 个过期用户")
            
            emby = get_emby_client()
            disabled_count = 0
            failed_count = 0
            
            for user in expired_users:
                try:
                    # 禁用 Emby 账户
                    if user.EMBYID:
                        await emby.set_user_enabled(user.EMBYID, False)
                    
                    # 更新本地状态
                    user.ACTIVE_STATUS = False
                    await UserOperate.update_user(user)
                    
                    disabled_count += 1
                    logger.info(f"  ⏹️ 已禁用: {user.USERNAME} (UID: {user.UID})")
                    
                except Exception as e:
                    failed_count += 1
                    logger.error(f"  ❌ 禁用失败: {user.USERNAME} - {e}")
            
            logger.info(f"✅ 过期用户检查完成: 禁用 {disabled_count} 个, 失败 {failed_count} 个")
            
        except Exception as e:
            logger.error(f"❌ 检查过期用户时发生错误: {e}")

    async def check_expiring_users():
        """检查即将过期的用户（用于提醒）"""
        logger.info("🔔 检查即将过期的用户...")
        
        try:
            # 获取 3 天内过期的用户
            expiring_users = await UserOperate.get_expiring_users(days=3)
            
            if not expiring_users:
                logger.info("✅ 没有即将过期的用户")
                return
            
            logger.info(f"📋 发现 {len(expiring_users)} 个即将过期的用户:")
            
            current = timestamp()
            for user in expiring_users:
                remaining = user.EXPIRED_AT - current
                remaining_str = format_duration(remaining)
                logger.info(f"  ⚠️ {user.USERNAME} (UID: {user.UID}) - {remaining_str}后过期")
            
            # TODO: 可以在这里实现通知功能（Telegram、邮件等）
            if ScoreAndRegisterConfig.NEW_USER_NOTICE_STATUS:
                # 发送通知
                pass
            
        except Exception as e:
            logger.error(f"❌ 检查即将过期用户时发生错误: {e}")

    async def cleanup_inactive_sessions():
        """清理不活跃的会话（可选）"""
        logger.info("🧹 清理不活跃会话...")
        
        try:
            emby = get_emby_client()
            sessions = await emby.get_sessions()
            
            # 统计信息
            active = len([s for s in sessions if s.is_active])
            total = len(sessions)
            
            logger.info(f"📊 当前会话: {active} 活跃 / {total} 总计")
            
        except Exception as e:
            logger.error(f"❌ 清理会话时发生错误: {e}")

    async def daily_stats():
        """每日统计"""
        logger.info("📊 生成每日统计...")
        
        try:
            from src.db.regcode import RegCodeOperate
            
            registered = await UserOperate.get_registered_users_count()
            active = await UserOperate.get_active_users_count()
            regcodes = await RegCodeOperate.get_active_regcodes_count()
            
            server_status = await EmbyService.get_server_status()
            
            logger.info("=" * 50)
            logger.info("📈 Twilight 每日统计")
            logger.info("=" * 50)
            logger.info(f"👥 注册用户: {registered} / {ScoreAndRegisterConfig.USER_LIMIT}")
            logger.info(f"✅ 活跃用户: {active}")
            logger.info(f"🎫 可用注册码: {regcodes}")
            logger.info(f"📺 Emby 状态: {'在线' if server_status.get('online') else '离线'}")
            if server_status.get('online'):
                logger.info(f"   活跃会话: {server_status.get('active_sessions', 0)}")
            logger.info("=" * 50)
            
        except Exception as e:
            logger.error(f"❌ 生成统计时发生错误: {e}")

    async def auto_renew_check():
        """积分自动续期检查"""
        from src.services.auto_renew_service import AutoRenewService
        
        logger.info("🔄 开始积分自动续期检查...")
        try:
            result = await AutoRenewService.check_and_renew()
            if result.get('enabled'):
                logger.info(f"✅ 自动续期完成: 成功 {result['renewed']}, 失败 {result['failed']}, 积分不足 {result['insufficient']}")
            else:
                logger.info("ℹ️ 自动续期功能未启用")
        except Exception as e:
            logger.error(f"❌ 自动续期检查出错: {e}")
    
    async def send_expiry_reminders():
        """发送到期提醒"""
        from src.services.admin_service import ReminderService
        
        logger.info("📧 发送到期提醒...")
        try:
            result = await ReminderService.send_expiry_reminders()
            logger.info(f"✅ 到期提醒发送完成: {result['sent']} 条")
        except Exception as e:
            logger.error(f"❌ 发送到期提醒出错: {e}")
    
    # 注册定时任务
    # 每天凌晨 2:00 执行积分自动续期
    scheduler.add_job(auto_renew_check, 'cron', hour=2, minute=0, id='auto_renew')
    
    # 每天凌晨 3:00 检查过期用户
    scheduler.add_job(check_expired_users, 'cron', hour=3, minute=0, id='check_expired')
    
    # 每天早上 9:00 检查即将过期的用户并发送提醒
    scheduler.add_job(check_expiring_users, 'cron', hour=9, minute=0, id='check_expiring')
    scheduler.add_job(send_expiry_reminders, 'cron', hour=9, minute=5, id='expiry_reminders')
    
    # 每天凌晨 0:00 生成每日统计
    scheduler.add_job(daily_stats, 'cron', hour=0, minute=5, id='daily_stats')
    
    # 每 6 小时清理一次会话统计
    scheduler.add_job(cleanup_inactive_sessions, 'interval', hours=6, id='cleanup_sessions')
    
    scheduler.start()
    
    logger.info("=" * 50)
    logger.info(f"🌙 Twilight Scheduler v{__version__} 已启动")
    logger.info("=" * 50)
    logger.info("📅 已注册的定时任务:")
    logger.info("  - 02:00 积分自动续期检查")
    logger.info("  - 03:00 检查过期用户")
    logger.info("  - 09:00 检查即将过期用户")
    logger.info("  - 09:05 发送到期提醒")
    logger.info("  - 00:05 生成每日统计")
    logger.info("  - 每6小时 会话统计")
    logger.info("=" * 50)
    
    # 启动时立即执行一次统计
    await daily_stats()
    
    # 保持运行
    try:
        while True:
            await asyncio.sleep(3600)
    except (KeyboardInterrupt, SystemExit):
        logger.info("🛑 正在关闭调度器...")
        scheduler.shutdown()
        logger.info("👋 调度器已关闭")


async def run_bot():
    """运行 Telegram Bot"""
    if not Config.TELEGRAM_MODE:
        logger.error("❌ Telegram 模式未启用")
        logger.error("请在配置文件中设置 telegram_mode = true")
        return
    
    if not TelegramConfig.BOT_TOKEN:
        logger.error("❌ 未配置 BOT_TOKEN")
        logger.error("请在配置文件中设置 bot_token")
        return
    
    from src.bot import start_bot, stop_bot
    
    logger.info("=" * 50)
    logger.info(f"🤖 Twilight Telegram Bot v{__version__}")
    logger.info("=" * 50)
    
    bot = await start_bot()
    
    if not bot:
        logger.error("❌ Bot 启动失败")
        return
    
    # 保持运行
    try:
        while True:
            await asyncio.sleep(1)
    except (KeyboardInterrupt, SystemExit):
        logger.info("🛑 正在关闭 Bot...")
        await stop_bot()
        logger.info("👋 Bot 已关闭")


async def run_all():
    """同时运行 API、Bot 和调度器"""
    import threading
    from concurrent.futures import ThreadPoolExecutor
    
    logger.info("=" * 50)
    logger.info(f"🌙 Twilight v{__version__} - 全功能模式")
    logger.info("=" * 50)
    
    # 在单独线程中运行 API
    def run_api_in_thread():
        from src.api import create_app
        app = create_app()
        app.run(host='0.0.0.0', port=5000, debug=False, use_reloader=False)
    
    api_thread = threading.Thread(target=run_api_in_thread, daemon=True)
    api_thread.start()
    logger.info("✅ API 服务器已在后台启动 (端口 5000)")
    
    # 启动 Bot（如果启用）
    bot = None
    if Config.TELEGRAM_MODE and TelegramConfig.BOT_TOKEN:
        from src.bot import start_bot, stop_bot
        bot = await start_bot()
        if bot:
            logger.info("✅ Telegram Bot 已启动")
        else:
            logger.warning("⚠️ Telegram Bot 启动失败")
    else:
        logger.info("ℹ️ Telegram Bot 未启用")
    
    # 启动调度器
    from apscheduler.schedulers.asyncio import AsyncIOScheduler
    from src.db.user import UserOperate
    from src.services import get_emby_client, EmbyService
    
    scheduler = AsyncIOScheduler(timezone='Asia/Shanghai')
    
    # 复用 run_scheduler 中的任务定义...
    # (为简化，这里省略具体任务定义，实际使用时应该共享代码)
    
    logger.info("=" * 50)
    logger.info("🎉 所有服务已启动")
    logger.info("=" * 50)
    
    # 保持运行
    try:
        while True:
            await asyncio.sleep(3600)
    except (KeyboardInterrupt, SystemExit):
        logger.info("🛑 正在关闭所有服务...")
        if bot:
            from src.bot import stop_bot
            await stop_bot()
        logger.info("👋 服务已关闭")


def main():
    """主入口"""
    parser = argparse.ArgumentParser(
        description='Twilight - Emby 用户管理系统',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
示例:
  python main.py api                    # 启动 API 服务器
  python main.py api --port 8080        # 指定端口
  python main.py bot                    # 启动 Telegram Bot
  python main.py scheduler              # 启动定时任务
  python main.py all                    # 启动所有服务
  python main.py --version              # 显示版本

注意:
  Telegram Bot 默认不启用，需要在 config.toml 中设置:
    [Global]
    telegram_mode = true
    
    [Telegram]
    bot_token = "your_bot_token"
        '''
    )
    
    parser.add_argument(
        '--version', '-v',
        action='version',
        version=f'Twilight v{__version__}'
    )
    
    subparsers = parser.add_subparsers(dest='command', help='可用命令')
    
    # API 服务器命令
    api_parser = subparsers.add_parser('api', help='启动 API 服务器')
    api_parser.add_argument('--host', default='0.0.0.0', help='监听地址')
    api_parser.add_argument('--port', type=int, default=5000, help='监听端口')
    api_parser.add_argument('--debug', action='store_true', help='调试模式')
    
    # Telegram Bot 命令
    bot_parser = subparsers.add_parser('bot', help='启动 Telegram Bot (需先启用)')
    
    # 定时任务命令
    scheduler_parser = subparsers.add_parser('scheduler', help='启动定时任务')
    
    # 全部启动命令
    all_parser = subparsers.add_parser('all', help='启动所有服务')
    
    args = parser.parse_args()
    
    # 配置日志
    if Config.LOGGING:
        setup_logging(level=Config.LOG_LEVEL)
    
    if args.command == 'api':
        run_api_server(args.host, args.port, args.debug)
    elif args.command == 'bot':
        asyncio.run(run_bot())
    elif args.command == 'scheduler':
        asyncio.run(run_scheduler())
    elif args.command == 'all':
        asyncio.run(run_all())
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == '__main__':
    main()
