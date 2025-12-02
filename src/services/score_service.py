"""
积分业务服务层

处理签到、红包、转账等积分相关业务
"""
import random
import time
import json
import hashlib
import logging
from typing import Optional, Tuple, List
from dataclasses import dataclass
from enum import Enum

from src.config import ScoreAndRegisterConfig
from src.db.score import ScoreModel, ScoreOperate, RedPacketModel
from src.db.user import UserModel, UserOperate
from src.core.utils import timestamp

logger = logging.getLogger(__name__)


class CheckinResult(Enum):
    """签到结果"""
    SUCCESS = "success"
    ALREADY_CHECKED = "already_checked"
    USER_NOT_FOUND = "user_not_found"
    ERROR = "error"


@dataclass
class CheckinResponse:
    """签到响应"""
    result: CheckinResult
    message: str
    score: int = 0  # 本次获得
    balance: int = 0  # 当前余额
    streak: int = 0  # 连签天数
    
    # 兼容旧字段名
    @property
    def score_gained(self) -> int:
        return self.score
    
    @property
    def total_score(self) -> int:
        return self.balance
    
    @property
    def checkin_days(self) -> int:
        return self.streak


class RedPacketType(Enum):
    """红包类型"""
    RANDOM = 1   # 拼手气
    EQUAL = 2    # 均分
    TARGETED = 0 # 定向


class RedPacketStatus(Enum):
    """红包状态"""
    ACTIVE = 0      # 未领完
    FINISHED = 1    # 已领完
    WITHDRAWN = 2   # 已撤回


class ScoreService:
    """积分业务服务"""

    @classmethod
    async def checkin(cls, uid: int) -> Tuple[CheckinResult, 'CheckinResponse']:
        """
        用户签到
        
        :param uid: 用户 UID
        :return: (结果, 响应)
        """
        # 获取用户
        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            return CheckinResult.USER_NOT_FOUND, CheckinResponse(
                result=CheckinResult.USER_NOT_FOUND,
                message="用户不存在"
            )

        # 获取或创建积分记录
        score_record = await ScoreOperate.get_score_by_uid(uid)
        if not score_record:
            score_record = ScoreModel(
                UID=uid,
                TELEGRAM_ID=user.TELEGRAM_ID or 0,
                SCORE=0,
                CHECKIN_TIME=0,
                CHECKIN_COUNT=0
            )
            await ScoreOperate.add_score(score_record)

        # 检查今日是否已签到
        today_start = cls._get_today_start()
        if score_record.CHECKIN_TIME >= today_start:
            return CheckinResult.ALREADY_CHECKED, CheckinResponse(
                result=CheckinResult.ALREADY_CHECKED,
                message="今天已经签到过了",
                balance=score_record.SCORE,
                streak=score_record.CHECKIN_COUNT
            )

        # 计算连签天数
        yesterday_start = today_start - 86400
        if score_record.CHECKIN_TIME >= yesterday_start:
            # 连续签到
            score_record.CHECKIN_COUNT += 1
        else:
            # 断签，重新计数
            score_record.CHECKIN_COUNT = 1

        # 从配置读取奖励参数
        base_score = ScoreAndRegisterConfig.CHECKIN_BASE_SCORE
        streak_bonus_per_day = ScoreAndRegisterConfig.CHECKIN_STREAK_BONUS
        max_streak_bonus = ScoreAndRegisterConfig.CHECKIN_MAX_STREAK_BONUS
        random_min = ScoreAndRegisterConfig.CHECKIN_RANDOM_MIN
        random_max = ScoreAndRegisterConfig.CHECKIN_RANDOM_MAX

        # 计算奖励
        streak_bonus = min(
            score_record.CHECKIN_COUNT * streak_bonus_per_day,
            max_streak_bonus
        )
        random_bonus = random.randint(random_min, random_max)
        total_gained = base_score + streak_bonus + random_bonus

        # 更新记录
        score_record.SCORE += total_gained
        score_record.CHECKIN_TIME = timestamp()
        await ScoreOperate.update_score(score_record)

        score_name = ScoreAndRegisterConfig.SCORE_NAME
        logger.info(f"用户签到成功: UID={uid}, +{total_gained} {score_name}")

        return CheckinResult.SUCCESS, CheckinResponse(
            result=CheckinResult.SUCCESS,
            message=f"签到成功！获得 {total_gained} {score_name}\n"
                    f"(基础 {base_score} + 连签 {streak_bonus} + 随机 {random_bonus})",
            score=total_gained,
            balance=score_record.SCORE,
            streak=score_record.CHECKIN_COUNT
        )

    @staticmethod
    def _get_today_start() -> int:
        """获取今日0点的时间戳"""
        now = time.time()
        return int(now - now % 86400)

    @staticmethod
    async def get_balance_by_uid(uid: int) -> Tuple[int, int]:
        """
        获取用户积分余额
        
        :param uid: 用户 UID
        :return: (积分, 连签天数)
        """
        score_record = await ScoreOperate.get_score_by_uid(uid)
        if not score_record:
            return 0, 0

        return score_record.SCORE, score_record.CHECKIN_COUNT
    
    @staticmethod
    async def get_balance(uid: int) -> Tuple[int, int]:
        """获取用户积分余额（使用 UID）"""
        return await ScoreService.get_balance_by_uid(uid)

    @staticmethod
    async def transfer(
        from_uid: int,
        to_uid: int,
        amount: int
    ) -> Tuple[bool, str]:
        """
        积分转账
        
        :param from_uid: 转出方 UID
        :param to_uid: 转入方 UID
        :param amount: 转账数量
        """
        if not ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE:
            return False, "私人转账功能未开启"

        # 金额验证
        min_amount = ScoreAndRegisterConfig.TRANSFER_MIN_AMOUNT
        max_amount = ScoreAndRegisterConfig.TRANSFER_MAX_AMOUNT
        
        if amount < min_amount:
            return False, f"转账金额不能少于 {min_amount}"
        
        if amount > max_amount:
            return False, f"转账金额不能超过 {max_amount}"

        if from_uid == to_uid:
            return False, "不能给自己转账"

        # 获取双方用户
        from_user = await UserOperate.get_user_by_uid(from_uid)
        to_user = await UserOperate.get_user_by_uid(to_uid)

        if not from_user:
            return False, "转出方用户不存在"
        if not to_user:
            return False, "转入方用户不存在"

        # 获取积分记录
        from_score = await ScoreOperate.get_score_by_uid(from_uid)
        to_score = await ScoreOperate.get_score_by_uid(to_uid)

        # 计算手续费
        fee_rate = ScoreAndRegisterConfig.TRANSFER_FEE_RATE
        fee = int(amount * fee_rate) if fee_rate > 0 else 0
        total_deduct = amount + fee

        if not from_score or from_score.SCORE < total_deduct:
            return False, f"积分不足，需要 {total_deduct} (含手续费 {fee})，当前余额: {from_score.SCORE if from_score else 0}"

        # 执行转账
        from_score.SCORE -= total_deduct
        await ScoreOperate.update_score(from_score)

        if to_score:
            to_score.SCORE += amount
            await ScoreOperate.update_score(to_score)
        else:
            to_score = ScoreModel(
                UID=to_uid,
                TELEGRAM_ID=to_user.TELEGRAM_ID or 0,
                SCORE=amount,
                CHECKIN_TIME=0,
                CHECKIN_COUNT=0
            )
            await ScoreOperate.add_score(to_score)

        score_name = ScoreAndRegisterConfig.SCORE_NAME
        fee_msg = f"(手续费 {fee})" if fee > 0 else ""
        logger.info(f"积分转账: {from_uid} -> {to_uid}, {amount} {score_name} {fee_msg}")

        return True, f"转账成功！已转出 {amount} {score_name} {fee_msg}"

    @staticmethod
    async def admin_adjust_score(
        uid: int,
        amount: int,
        reason: str = ""
    ) -> Tuple[bool, str]:
        """
        管理员调整积分
        
        :param uid: 用户 UID
        :param amount: 调整数量（正数增加，负数减少）
        :param reason: 调整原因
        """
        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            return False, "用户不存在"

        score_record = await ScoreOperate.get_score_by_uid(uid)
        if not score_record:
            if amount < 0:
                return False, "用户没有积分记录，无法扣除"
            score_record = ScoreModel(
                UID=uid,
                TELEGRAM_ID=user.TELEGRAM_ID,
                SCORE=amount,
                CHECKIN_TIME=0,
                CHECKIN_COUNT=0
            )
            await ScoreOperate.add_score(score_record)
        else:
            new_score = score_record.SCORE + amount
            if new_score < 0:
                return False, f"积分不足，当前: {score_record.SCORE}"
            score_record.SCORE = new_score
            await ScoreOperate.update_score(score_record)

        action = "增加" if amount > 0 else "扣除"
        logger.info(f"管理员调整积分: UID={uid}, {action} {abs(amount)}, 原因: {reason}")

        return True, f"已{action} {abs(amount)} 积分"

    @staticmethod
    async def get_ranking(limit: int = 10) -> List[dict]:
        """获取积分排行榜"""
        records = await ScoreOperate.get_user_score_ranking(limit)
        ranking = []

        for i, record in enumerate(records, 1):
            user = await UserOperate.get_user_by_uid(record.UID)
            ranking.append({
                "rank": i,
                "uid": record.UID,
                "username": user.USERNAME if user else "未知",
                "score": record.SCORE,
                "checkin_days": record.CHECKIN_COUNT,
            })

        return ranking


class RedPacketService:
    """红包业务服务"""

    @staticmethod
    def _generate_key() -> str:
        """生成红包密钥"""
        unique = f"{timestamp()}-{random.randint(10000, 99999)}"
        return f"rp-{hashlib.md5(unique.encode()).hexdigest()[:12]}"

    @staticmethod
    async def create_red_packet(
        sender_uid: int,
        total_amount: int,
        count: int,
        packet_type: RedPacketType = RedPacketType.RANDOM
    ) -> Tuple[bool, str, Optional[str]]:
        """
        创建红包
        
        :param sender_uid: 发送者 UID
        :param total_amount: 总金额
        :param count: 红包个数
        :param packet_type: 红包类型
        :return: (成功, 消息, 红包Key)
        """
        if not ScoreAndRegisterConfig.RED_PACKET_MODE:
            return False, "红包功能未开启", None

        # 金额验证
        min_amount = ScoreAndRegisterConfig.RED_PACKET_MIN_AMOUNT
        max_amount = ScoreAndRegisterConfig.RED_PACKET_MAX_AMOUNT
        min_count = ScoreAndRegisterConfig.RED_PACKET_MIN_COUNT
        max_count = ScoreAndRegisterConfig.RED_PACKET_MAX_COUNT

        if total_amount < min_amount:
            return False, f"红包金额不能少于 {min_amount}", None
        
        if total_amount > max_amount:
            return False, f"红包金额不能超过 {max_amount}", None
        
        if count < min_count:
            return False, f"红包个数不能少于 {min_count}", None
        
        if count > max_count:
            return False, f"红包个数不能超过 {max_count}", None

        if total_amount < count:
            return False, "总金额不能小于红包个数", None

        # 获取发送者
        sender = await UserOperate.get_user_by_uid(sender_uid)
        if not sender:
            return False, "用户不存在", None

        # 检查余额
        score = await ScoreOperate.get_score_by_uid(sender.UID)
        if not score or score.SCORE < total_amount:
            return False, f"积分不足，当前余额: {score.SCORE if score else 0}", None

        # 扣除积分
        score.SCORE -= total_amount
        await ScoreOperate.update_score(score)

        # 创建红包
        rp_key = RedPacketService._generate_key()
        red_packet = RedPacketModel(
            SENDER_UID=sender.UID,
            SENDER_TELEGRAM_ID=sender.TELEGRAM_ID or 0,
            AMOUNT=total_amount,
            COUNT=count,
            CURRENT_AMOUNT=total_amount,
            STATUS=RedPacketStatus.ACTIVE.value,
            TYPE=packet_type.value,
            RP_KEY=rp_key,
            HISTORY='[]',
        )
        await ScoreOperate.add_red_packet(red_packet)

        score_name = ScoreAndRegisterConfig.SCORE_NAME
        logger.info(f"红包创建: {sender.UID} 发送 {total_amount} {score_name}, {count}个")

        return True, f"红包创建成功！总计 {total_amount} {score_name}, {count}个", rp_key

    @staticmethod
    async def grab_red_packet(
        rp_key: str,
        user_uid: int
    ) -> Tuple[bool, str, int]:
        """
        抢红包
        
        :param rp_key: 红包 Key
        :param user_uid: 用户 UID
        :return: (成功, 消息, 获得金额)
        """
        # 获取红包
        red_packets = await ScoreOperate.get_active_red_packets()
        red_packet = None
        for rp in red_packets:
            if rp.RP_KEY == rp_key:
                red_packet = rp
                break

        if not red_packet:
            return False, "红包不存在或已领完", 0

        # 获取用户
        user = await UserOperate.get_user_by_uid(user_uid)
        if not user:
            return False, "用户不存在", 0

        # 检查是否已领取
        history = json.loads(red_packet.HISTORY or '[]')
        for record in history:
            if record.get('uid') == user_uid:
                return False, "您已经领取过了", 0

        # 计算金额
        remaining_count = red_packet.COUNT - len(history)
        if remaining_count <= 0:
            return False, "红包已被领完", 0

        if red_packet.TYPE == RedPacketType.EQUAL.value:
            # 均分
            amount = red_packet.CURRENT_AMOUNT // remaining_count
        else:
            # 随机（二倍均值法）
            if remaining_count == 1:
                amount = red_packet.CURRENT_AMOUNT
            else:
                max_amount = red_packet.CURRENT_AMOUNT - remaining_count + 1
                avg = red_packet.CURRENT_AMOUNT / remaining_count
                amount = random.randint(1, min(int(avg * 2), max_amount))

        # 更新红包
        red_packet.CURRENT_AMOUNT -= amount
        history.append({
            'uid': user.UID,
            'telegram_id': user.TELEGRAM_ID,
            'amount': amount,
            'time': timestamp()
        })
        red_packet.HISTORY = json.dumps(history)

        if len(history) >= red_packet.COUNT:
            red_packet.STATUS = RedPacketStatus.FINISHED.value

        await ScoreOperate.update_red_packet(red_packet)

        # 增加用户积分
        score = await ScoreOperate.get_score_by_uid(user.UID)
        if score:
            score.SCORE += amount
            await ScoreOperate.update_score(score)
        else:
            score = ScoreModel(
                UID=user.UID,
                TELEGRAM_ID=user.TELEGRAM_ID or 0,
                SCORE=amount,
                CHECKIN_TIME=0,
                CHECKIN_COUNT=0
            )
            await ScoreOperate.add_score(score)

        score_name = ScoreAndRegisterConfig.SCORE_NAME
        logger.info(f"红包领取: {user.UID} 获得 {amount} {score_name}")

        return True, f"恭喜获得 {amount} {score_name}！", amount

    @staticmethod
    async def withdraw_red_packet(rp_key: str, user_uid: int) -> Tuple[bool, str]:
        """
        撤回红包
        
        :param rp_key: 红包 Key
        :param user_uid: 用户 UID
        """
        red_packets = await ScoreOperate.get_active_red_packets()
        red_packet = None
        for rp in red_packets:
            if rp.RP_KEY == rp_key:
                red_packet = rp
                break

        if not red_packet:
            return False, "红包不存在或已失效"

        if red_packet.SENDER_UID != user_uid:
            return False, "只能撤回自己发的红包"

        # 退还剩余金额
        if red_packet.CURRENT_AMOUNT > 0:
            score = await ScoreOperate.get_score_by_uid(user_uid)
            if score:
                score.SCORE += red_packet.CURRENT_AMOUNT
                await ScoreOperate.update_score(score)

        red_packet.STATUS = RedPacketStatus.WITHDRAWN.value
        await ScoreOperate.update_red_packet(red_packet)

        score_name = ScoreAndRegisterConfig.SCORE_NAME
        return True, f"红包已撤回，退还 {red_packet.CURRENT_AMOUNT} {score_name}"

