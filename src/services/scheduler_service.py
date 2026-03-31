import asyncio
import logging
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from src.config import Config, ScoreAndRegisterConfig, SchedulerConfig
from src.db.user import UserOperate
from src.services import get_emby_client, EmbyService
from src.core.utils import timestamp, format_duration

logger = logging.getLogger(__name__)

class SchedulerService:
    _scheduler = None

    @classmethod
    def get_scheduler(cls):
        if cls._scheduler is None:
            cls._scheduler = AsyncIOScheduler(timezone=SchedulerConfig.TIMEZONE)
        return cls._scheduler

    @staticmethod
    async def check_expired_users():
        """检查过期用户并禁用"""
        logger.info("🔍 开始检查过期用户...")
        try:
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
                    if user.EMBYID:
                        await emby.set_user_enabled(user.EMBYID, False)
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

    @staticmethod
    async def check_expiring_users():
        """检查即将过期的用户（用于提醒）"""
        logger.info("🔔 检查即将过期的用户...")
        try:
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
            
            # TODO: 实现通知功能
        except Exception as e:
            logger.error(f"❌ 检查即将过期用户时发生错误: {e}")

    @staticmethod
    async def cleanup_inactive_sessions():
        """清理不活跃的会话"""
        logger.info("🧹 清理不活跃会话...")
        try:
            emby = get_emby_client()
            sessions = await emby.get_sessions()
            active = len([s for s in sessions if s.is_active])
            total = len(sessions)
            logger.info(f"📊 当前会话: {active} 活跃 / {total} 总计")
        except Exception as e:
            logger.error(f"❌ 清理会话时发生错误: {e}")

    @staticmethod
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

    @staticmethod
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

    @staticmethod
    async def send_expiry_reminders():
        """发送到期提醒"""
        from src.services.admin_service import ReminderService
        logger.info("📧 发送到期提醒...")
        try:
            result = await ReminderService.send_expiry_reminders()
            logger.info(f"✅ 到期提醒发送完成: {result['sent']} 条")
        except Exception as e:
            logger.error(f"❌ 发送到期提醒出错: {e}")

    @staticmethod
    async def emby_sync():
        """定期同步 Emby 用户数据"""
        logger.info("🔄 开始 Emby 用户数据同步...")
        try:
            success, failed, errors = await EmbyService.sync_all_users()
            logger.info(f"✅ Emby 同步完成: 成功 {success}, 失败 {failed}")
            if errors:
                for e in errors[:10]:
                    logger.warning(f"  ⚠️ {e}")
        except Exception as e:
            logger.error(f"❌ Emby 同步出错: {e}")

    @classmethod
    async def start(cls):
        """启动调度器"""
        if not SchedulerConfig.ENABLED:
            logger.info("ℹ️ 调度器已禁用")
            return

        scheduler = cls.get_scheduler()
        
        # 解析配置时间
        def parse_time(time_str):
            try:
                hour, minute = map(int, time_str.split(':'))
                return hour, minute
            except:
                return 0, 0

        # 注册定时任务
        h, m = parse_time(SchedulerConfig.AUTO_RENEW_TIME)
        scheduler.add_job(cls.auto_renew_check, 'cron', hour=h, minute=m, id='auto_renew')
        
        h, m = parse_time(SchedulerConfig.EXPIRED_CHECK_TIME)
        scheduler.add_job(cls.check_expired_users, 'cron', hour=h, minute=m, id='check_expired')
        
        h, m = parse_time(SchedulerConfig.EXPIRING_CHECK_TIME)
        scheduler.add_job(cls.check_expiring_users, 'cron', hour=h, minute=m, id='check_expiring')
        scheduler.add_job(cls.send_expiry_reminders, 'cron', hour=h, minute=(m+5)%60, id='expiry_reminders')
        
        h, m = parse_time(SchedulerConfig.DAILY_STATS_TIME)
        scheduler.add_job(cls.daily_stats, 'cron', hour=h, minute=m, id='daily_stats')
        
        scheduler.add_job(cls.cleanup_inactive_sessions, 'interval', hours=SchedulerConfig.SESSION_CLEANUP_INTERVAL, id='cleanup_sessions')
        
        # Emby 数据同步（每 6 小时）
        scheduler.add_job(cls.emby_sync, 'interval', hours=SchedulerConfig.EMBY_SYNC_INTERVAL, id='emby_sync')
        
        scheduler.start()
        logger.info("=" * 50)
        logger.info(f"🌙 Twilight Scheduler 已启动 ({SchedulerConfig.TIMEZONE})")
        logger.info(f"  - 自动续期: {SchedulerConfig.AUTO_RENEW_TIME}")
        logger.info(f"  - 过期检查: {SchedulerConfig.EXPIRED_CHECK_TIME}")
        logger.info(f"  - 到期提醒: {SchedulerConfig.EXPIRING_CHECK_TIME}")
        logger.info(f"  - 每日统计: {SchedulerConfig.DAILY_STATS_TIME}")
        logger.info(f"  - 会话清理: 每 {SchedulerConfig.SESSION_CLEANUP_INTERVAL} 小时")
        logger.info(f"  - Emby 同步: 每 {SchedulerConfig.EMBY_SYNC_INTERVAL} 小时")
        logger.info("=" * 50)
        
        # 立即运行一次统计
        await cls.daily_stats()

    @classmethod
    async def stop(cls):
        """停止调度器"""
        if cls._scheduler and cls._scheduler.running:
            cls._scheduler.shutdown()
            logger.info("👋 调度器已关闭")
