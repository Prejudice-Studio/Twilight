"""
积分自动续期服务

用户开启后，到期前自动扣分续期
"""
import logging
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
        
        :return: 续期结果统计
        """
        if not ScoreAndRegisterConfig.AUTO_RENEW_ENABLED:
            return {'enabled': False, 'message': '自动续期功能未启用'}
        
        before_days = ScoreAndRegisterConfig.AUTO_RENEW_BEFORE_DAYS
        renew_days = ScoreAndRegisterConfig.AUTO_RENEW_DAYS
        renew_cost = ScoreAndRegisterConfig.AUTO_RENEW_COST
        
        # 获取即将到期且开启了自动续期的用户
        expiring_users = await UserOperate.get_expiring_users(days=before_days)
        
        renewed = 0
        failed = 0
        insufficient = 0
        details = []
        
        for user in expiring_users:
            # 检查用户是否开启了自动续期
            if not user.AUTO_RENEW:
                continue
            
            # 检查积分是否足够
            score = await ScoreOperate.get_score_by_uid(user.UID)
            balance = score.SCORE if score else 0
            
            if balance < renew_cost:
                insufficient += 1
                details.append({
                    'uid': user.UID,
                    'username': user.USERNAME,
                    'status': 'insufficient',
                    'balance': balance,
                    'cost': renew_cost,
                })
                
                # 通知用户积分不足
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
                
                continue
            
            # 执行续期
            try:
                # 扣除积分
                score.SCORE -= renew_cost
                await ScoreOperate.update_score(score)
                
                # 续期
                success, msg = await UserService.renew(user.UID, renew_days)
                
                if success:
                    renewed += 1
                    details.append({
                        'uid': user.UID,
                        'username': user.USERNAME,
                        'status': 'success',
                        'days': renew_days,
                        'cost': renew_cost,
                    })
                    
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
                else:
                    # 续期失败，退还积分
                    score.SCORE += renew_cost
                    await ScoreOperate.update_score(score)
                    
                    failed += 1
                    details.append({
                        'uid': user.UID,
                        'username': user.USERNAME,
                        'status': 'failed',
                        'error': msg,
                    })
                    
            except Exception as e:
                failed += 1
                details.append({
                    'uid': user.UID,
                    'username': user.USERNAME,
                    'status': 'error',
                    'error': str(e),
                })
                logger.error(f"自动续期出错: {user.USERNAME} - {e}")
        
        result = {
            'enabled': True,
            'checked': len(expiring_users),
            'renewed': renewed,
            'failed': failed,
            'insufficient': insufficient,
            'details': details,
        }
        
        logger.info(f"自动续期检查完成: 检查 {len(expiring_users)}, 成功 {renewed}, 失败 {failed}, 积分不足 {insufficient}")
        
        return result
    
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
        
        await UserOperate.update_user(uid=uid, auto_renew=enabled)
        
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

