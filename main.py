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
from src.config import Config, ScoreAndRegisterConfig, TelegramConfig, APIConfig
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
    from src.services.scheduler_service import SchedulerService
    
    await SchedulerService.start()
    
    # 保持运行
    try:
        while True:
            await asyncio.sleep(3600)
    except (KeyboardInterrupt, SystemExit):
        await SchedulerService.stop()


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
    from src.services.scheduler_service import SchedulerService
    
    logger.info("=" * 50)
    logger.info(f"🌙 Twilight v{__version__} - 全功能模式")
    logger.info("=" * 50)
    
    # 1. 在单独线程中运行 API
    def run_api_in_thread():
        from src.api import create_app
        app = create_app()
        # 全功能模式下关闭 debug 以避免两次初始化
        app.run(host=APIConfig.HOST, port=APIConfig.PORT, debug=False, use_reloader=False)
    
    api_thread = threading.Thread(target=run_api_in_thread, daemon=True)
    api_thread.start()
    logger.info(f"✅ API 服务器已在后台启动 (端口 {APIConfig.PORT})")
    
    # 2. 启动 Bot（如果启用）
    bot = None
    if Config.TELEGRAM_MODE and TelegramConfig.BOT_TOKEN:
        from src.bot import start_bot
        bot = await start_bot()
        if bot:
            logger.info("✅ Telegram Bot 已启动")
        else:
            logger.warning("⚠️ Telegram Bot 启动失败")
    else:
        logger.info("ℹ️ Telegram Bot 未启用")
    
    # 3. 启动调度器
    await SchedulerService.start()
    
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
        await SchedulerService.stop()
        logger.info("👋 服务已关闭")


def main():
    """主入口"""
    parser = argparse.ArgumentParser(
        description='Twilight - Emby 用户管理系统 v{}'.format(__version__),
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
快速开始:
  python main.py api                    # 启动 API 服务器（开发）
  python main.py api --debug            # 调试模式
  python main.py bot                    # 启动 Telegram Bot
  python main.py scheduler              # 启动定时任务
  python main.py all                    # 启动所有服务

生产部署:
  pip install uvicorn
  uvicorn asgi:app --host 0.0.0.0 --port 5000 --workers 4

文档和帮助:
  📖 安装指南:      docs/INSTALL.md
  🔧 开发指南:      docs/DEVELOPMENT.md
  🌐 API 文档:      docs/BACKEND_API.md
  🚀 快速开始:      README.md

配置:
  1. 复制 .env.example 为 .env
  2. 编辑 .env 文件配置相关参数
  3. （可选）编辑 config.toml 进行高级配置

更多信息: https://github.com/Prejudice-Studio/Twilight
        '''
    )
    
    parser.add_argument(
        '--version', '-v',
        action='version',
        version=f'Twilight v{__version__}'
    )
    
    subparsers = parser.add_subparsers(dest='command', help='可用命令')
    
    # API 服务器命令
    api_parser = subparsers.add_parser('api', help='(仅开发用) 启动 API 服务器')
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
