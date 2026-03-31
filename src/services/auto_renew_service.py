"""
积分自动续期服务

用户开启后，到期前自动扣分续期
"""
import logging
import time
from typing import List, Dict, Any, Tuple

from src.config import ScoreAndRegisterConfig
from src.db.user import UserOperate, UserModel
from src.db.score import ScoreOperate
from src.services.user_service import UserService
from src.services.notification import NotificationService, Notification, NotificationType
from src.core.utils import timestamp, format_duration, days_to_seconds

logger = logging.getLogger(__name__)


class AutoRenewService:
    """积分自动续期服务"""
    
    @classmethod
    async def check_and_renew(cls) -> Dict[str, Any]:
        """
        检查并执行自动续期
        
        流程:
        1. 对进入续期窗口（到期前 before_days 天）的用户执行续期
        2. 对已到期的用户再次检查并尝试续期（二次兜底）
        3. 提前通知即将进入续期窗口的用户
        
        :return: 续期结果统计
        """
        if not ScoreAndRegisterConfig.AUTO_RENEW_ENABLED:
            return {'enabled': False, 'message': '自动续期功能未启用'}
        
        before_days = ScoreAndRegisterConfig.AUTO_RENEW_BEFORE_DAYS
        renew_days = ScoreAndRegisterConfig.AUTO_RENEW_DAYS
        renew_cost = ScoreAndRegisterConfig.AUTO_RENEW_COST
        
        renewed = 0
        failed = 0
        insufficient = 0
        notified = 0
        details = []
        
        # 1. 获取即将到期且开启了自动续期的用户（正常续期窗口）
        expiring_users = await UserOperate.get_expiring_users(days=before_days)
        
        for user in expiring_users:
            if not user.AUTO_RENEW:
                continue
            
            result = await cls._try_renew_user(user, renew_days, renew_cost)
            details.append(result)
            
            if result['status'] == 'success':
                renewed += 1
            elif result['status'] == 'insufficient':
                insufficient += 1
            elif result['status'] in ('failed', 'error'):
                failed += 1
        
        # 2. 到期兜底: 查找刚刚到期的用户（过期不超过 25 小时，覆盖两次检查间隔），再尝试一次续期
        expired_users = await cls._get_just_expired_users(max_hours=25)
        already_processed = {d['uid'] for d in details}
        for user in expired_users:
            if user.UID in already_processed or not user.AUTO_RENEW:
                continue
            
            result = await cls._try_renew_user(user, renew_days, renew_cost)
            result['expired_retry'] = True
            details.append(result)
            
            if result['status'] == 'success':
                renewed += 1
            elif result['status'] == 'insufficient':
                insufficient += 1
            elif result['status'] in ('failed', 'error'):
                failed += 1
        
        # 3. 提前通知: 在续期窗口之前再多 2 天提醒即将自动续期
        if ScoreAndRegisterConfig.AUTO_RENEW_NOTIFY:
            notify_days = before_days + 2
            pre_notify_users = await UserOperate.get_expiring_users(days=notify_days)
            already_processed = {d['uid'] for d in details}
            
            for user in pre_notify_users:
                if user.UID in already_processed or not user.AUTO_RENEW:
                    continue
                
                if user.TELEGRAM_ID:
                    try:
                        score = await ScoreOperate.get_score_by_uid(user.UID)
                        balance = score.SCORE if score else 0
                        
                        days_left = (user.EXPIRED_AT - int(time.time())) // 86400
                        status_text = f"✅ 积分充足，届时将自动续期" if balance >= renew_cost else f"⚠️ 积分不足，请及时充值"
                        
                        await NotificationService.send(Notification(
                            type=NotificationType.USER_EXPIRED,
                            title="📢 自动续期提醒",
                            content=f"您的账号将在 {days_left} 天后到期\n"
                                    f"续期费用: {renew_cost} {ScoreAndRegisterConfig.SCORE_NAME}\n"
                                    f"当前余额: {balance} {ScoreAndRegisterConfig.SCORE_NAME}\n"
                                    f"{status_text}",
                            target_users=[user.TELEGRAM_ID]
                        ))
                        notified += 1
                    except Exception as e:
                        logger.error(f"发送提前通知失败: {e}")
        
        total_checked = len(expiring_users) + len(expired_users)
        result = {
            'enabled': True,
            'checked': total_checked,
            'renewed': renewed,
            'failed': failed,
            'insufficient': insufficient,
            'notified': notified,
            'details': details,
        }
        
        logger.info(
            f"自动续期检查完成: 检查 {total_checked}, 成功 {renewed}, "
            f"失败 {failed}, 积分不足 {insufficient}, 提前通知 {notified}"
        )
        
        return result
    
    @classmethod
    async def _try_renew_user(
        cls, user: UserModel, renew_days: int, renew_cost: int
    ) -> Dict[str, Any]:
        """
        尝试为单个用户执行续期
        
        :return: 续期结果详情
        """
        # 检查积分
        score = await ScoreOperate.get_score_by_uid(user.UID)
        balance = score.SCORE if score else 0
        
        if balance < renew_cost:
            # 积分不足，通知用户
            if user.TELEGRAM_ID and ScoreAndRegisterConfig.AUTO_RENEW_NOTIFY:
                try:
                    await NotificationService.send(Notification(
                        type=NotificationType.USER_EXPIRED,
                        title="⚠️ 自动续期失败",
                        content=f"您的账号即将到期，但积分不足以自动续期\n"
                                f"需要: {renew_cost} {ScoreAndRegisterConfig.SCORE_NAME}\n"
                                f"当前: {balance} {ScoreAndRegisterConfig.SCORE_NAME}",
                        target_users=[user.TELEGRAM_ID]
                    ))
                except Exception as e:
                    logger.error(f"发送通知失败: {e}")
            
            return {
                'uid': user.UID,
                'username': user.USERNAME,
                'status': 'insufficient',
                'balance': balance,
                'cost': renew_cost,
            }
        
        # 执行续期
        try:
            # 先续期，再扣分（续期更重要，扣分失败可以后续补扣）
            success, msg = await UserService.renew_user(user, renew_days)
            
            if not success:
                return {
                    'uid': user.UID,
                    'username': user.USERNAME,
                    'status': 'failed',
                    'error': msg,
                }
            
            # 扣除积分
            score.SCORE -= renew_cost
            if hasattr(score, 'TOTAL_SPENT'):
                score.TOTAL_SPENT = (score.TOTAL_SPENT or 0) + renew_cost
            await ScoreOperate.update_score(score)
            
            # 记录积分历史
            from src.db.score import ScoreHistoryOperate
            await ScoreHistoryOperate.add_history(
                uid=user.UID,
                type_='renew',
                amount=-renew_cost,
                balance_after=score.SCORE,
                note=f"自动续期 {renew_days} 天"
            )
            
            logger.info(f"自动续期成功: {user.USERNAME}, {renew_days}天, 扣除 {renew_cost} 积分")
            
            # 通知用户
            if user.TELEGRAM_ID and ScoreAndRegisterConfig.AUTO_RENEW_NOTIFY:
                try:
                    await NotificationService.send(Notification(
                        type=NotificationType.USER_RENEWED,
                        title="✅ 自动续期成功",
                        content=f"您的账号已自动续期 {renew_days} 天\n"
                                f"扣除: {renew_cost} {ScoreAndRegisterConfig.SCORE_NAME}\n"
                                f"余额: {score.SCORE} {ScoreAndRegisterConfig.SCORE_NAME}",
                        target_users=[user.TELEGRAM_ID]
                    ))
                except Exception as e:
                    logger.error(f"发送通知失败: {e}")
            
            return {
                'uid': user.UID,
                'username': user.USERNAME,
                'status': 'success',
                'days': renew_days,
                'cost': renew_cost,
            }
                
        except Exception as e:
            logger.error(f"自动续期出错: {user.USERNAME} - {e}")
            return {
                'uid': user.UID,
                'username': user.USERNAME,
                'status': 'error',
                'error': str(e),
            }
    
    @classmethod
    async def _get_just_expired_users(cls, max_hours: int = 1) -> list:
        """
        获取刚刚到期的用户（二次兜底检查）
        
        :param max_hours: 过期不超过该小时数的用户
        :return: 刚到期的用户列表
        """
        from sqlalchemy import select
        from src.db.user import UserModel, UsersSessionFactory
        
        current_time = int(time.time())
        earliest = current_time - max_hours * 3600
        
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.EXPIRED_AT != -1,
                    UserModel.EXPIRED_AT <= current_time,    # 已过期
                    UserModel.EXPIRED_AT >= earliest,        # 刚到期（不超过 max_hours）
                    UserModel.AUTO_RENEW == True,
                )
            )
            return list(result.scalars().all())
    
    @classmethod
    async def set_user_auto_renew(cls, uid: int, enabled: bool) -> Tuple[bool, str]:
        """
        设置用户自动续期开关
        
        :param uid: 用户 UID
        :param enabled: 是否开启
        """
        if not ScoreAndRegisterConfig.AUTO_RENEW_ENABLED:
            return False, "系统未启用自动续期功能"
        
        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            return False, "用户不存在"
        
        user.AUTO_RENEW = enabled
        await UserOperate.update_user(user)
        
        status = "开启" if enabled else "关闭"
        return True, f"自动续期已{status}"
    
    @classmethod
    async def get_auto_renew_info(cls) -> Dict[str, Any]:
        """获取自动续期配置信息"""
        return {
            'enabled': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
            'days': ScoreAndRegisterConfig.AUTO_RENEW_DAYS,
            'cost': ScoreAndRegisterConfig.AUTO_RENEW_COST,
            'before_days': ScoreAndRegisterConfig.AUTO_RENEW_BEFORE_DAYS,
            'notify': ScoreAndRegisterConfig.AUTO_RENEW_NOTIFY,
            'score_name': ScoreAndRegisterConfig.SCORE_NAME,
        }

