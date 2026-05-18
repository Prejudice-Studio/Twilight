"""
签到与积分业务服务

积分仅装饰用途，不影响系统权限；无排行榜。
"""
import logging
import random
from dataclasses import dataclass
from datetime import date, timedelta
from typing import Optional, Tuple

from src.config import SigninConfig
from src.db.signin import (
    SigninOperate,
    UserPointsModel,
    SigninRecordModel,
)
from src.db.user import UserModel


logger = logging.getLogger(__name__)


@dataclass
class SigninResult:
    success: bool
    message: str
    today_signed: bool = False
    daily_points: int = 0
    bonus_points: int = 0
    current_streak: int = 0
    longest_streak: int = 0
    current_points: int = 0
    currency_name: str = ''


@dataclass
class SigninSummary:
    enabled: bool
    currency_name: str
    current_points: int
    current_streak: int
    longest_streak: int
    total_points: int
    last_signin_date: Optional[str]
    today_signed: bool
    next_bonus_in_days: Optional[int]
    next_bonus_points: Optional[int]


def _bonus_table() -> list[tuple[int, int]]:
    """返回 [(streak_day, bonus_points), ...]，按天数升序。"""
    days = list(SigninConfig.STREAK_BONUS_DAYS or [])
    points = list(SigninConfig.STREAK_BONUS_POINTS or [])
    table: list[tuple[int, int]] = []
    for i, d in enumerate(days):
        try:
            day_int = int(d)
            pt_int = int(points[i]) if i < len(points) else 0
        except (TypeError, ValueError):
            continue
        if day_int <= 0:
            continue
        table.append((day_int, pt_int))
    table.sort(key=lambda x: x[0])
    return table


def _compute_streak(last_signin_date: Optional[str], previous_streak: int, today_str: str) -> int:
    """根据上次签到推算今日签到后的连签天数。"""
    if not last_signin_date:
        return 1
    try:
        last = date.fromisoformat(last_signin_date)
        today = date.fromisoformat(today_str)
    except ValueError:
        return 1
    delta = (today - last).days
    if delta == 1:
        return max(int(previous_streak or 0) + 1, 1)
    # 同日 (delta=0) 不应到这一步——会被 already-signed 拦截
    if SigninConfig.RESET_AFTER_MISS:
        return 1
    return max(int(previous_streak or 0) + 1, 1)


def _today_str() -> str:
    return date.today().isoformat()


class SigninService:
    """签到 / 积分对外业务接口。"""

    @staticmethod
    def is_enabled() -> bool:
        return bool(SigninConfig.ENABLED)

    @staticmethod
    def currency_name() -> str:
        return SigninConfig.CURRENCY_NAME or '积分'

    @staticmethod
    def public_config() -> dict:
        table = [
            {'streak_days': d, 'bonus_points': p}
            for d, p in _bonus_table()
        ]
        return {
            'enabled': SigninService.is_enabled(),
            'currency_name': SigninService.currency_name(),
            'daily_min': int(SigninConfig.DAILY_MIN or 0),
            'daily_max': int(SigninConfig.DAILY_MAX or 0),
            'bonus_table': table,
            'reset_after_miss': bool(SigninConfig.RESET_AFTER_MISS),
        }

    @staticmethod
    async def signin(user: UserModel) -> SigninResult:
        if not SigninService.is_enabled():
            return SigninResult(success=False, message='签到功能未开启', currency_name=SigninService.currency_name())

        today = _today_str()
        already = await SigninOperate.get_today_record(user.UID, today)
        if already:
            points = await SigninOperate.get_user_points(user.UID)
            return SigninResult(
                success=False,
                message='今日已签到',
                today_signed=True,
                daily_points=already.DAILY_POINTS,
                bonus_points=already.BONUS_POINTS,
                current_streak=points.CURRENT_STREAK if points else 1,
                longest_streak=points.LONGEST_STREAK if points else 1,
                current_points=points.CURRENT_POINTS if points else 0,
                currency_name=SigninService.currency_name(),
            )

        existing = await SigninOperate.get_user_points(user.UID)
        previous_streak = int(existing.CURRENT_STREAK or 0) if existing else 0
        last_date = existing.LAST_SIGNIN_DATE if existing else None
        new_streak = _compute_streak(last_date, previous_streak, today)

        daily_min = max(0, int(SigninConfig.DAILY_MIN or 0))
        daily_max = max(daily_min, int(SigninConfig.DAILY_MAX or daily_min))
        daily_points = random.randint(daily_min, daily_max) if daily_max > 0 else 0

        bonus_points = 0
        for streak_day, pts in _bonus_table():
            if new_streak == streak_day:
                bonus_points += pts

        updated = await SigninOperate.commit_signin(
            uid=user.UID,
            date_str=today,
            daily_points=daily_points,
            bonus_points=bonus_points,
            new_streak=new_streak,
        )

        return SigninResult(
            success=True,
            message='签到成功',
            today_signed=True,
            daily_points=daily_points,
            bonus_points=bonus_points,
            current_streak=updated.CURRENT_STREAK,
            longest_streak=updated.LONGEST_STREAK,
            current_points=updated.CURRENT_POINTS,
            currency_name=SigninService.currency_name(),
        )

    @staticmethod
    async def get_summary(user: UserModel) -> SigninSummary:
        currency_name = SigninService.currency_name()
        today = _today_str()

        if not SigninService.is_enabled():
            return SigninSummary(
                enabled=False,
                currency_name=currency_name,
                current_points=0,
                current_streak=0,
                longest_streak=0,
                total_points=0,
                last_signin_date=None,
                today_signed=False,
                next_bonus_in_days=None,
                next_bonus_points=None,
            )

        # 漏签自动清零（只在读时检查，避免长期不登录后展示一个虚假连签数）
        await SigninOperate.reset_streak_if_missed(user.UID, today)
        points = await SigninOperate.get_user_points(user.UID)
        today_record = await SigninOperate.get_today_record(user.UID, today)
        today_signed = today_record is not None

        # 计算下一档加成
        next_in: Optional[int] = None
        next_pts: Optional[int] = None
        current_streak = int(points.CURRENT_STREAK or 0) if points else 0
        projected_streak = current_streak + (0 if today_signed else 1)
        for streak_day, pts in _bonus_table():
            if streak_day >= max(projected_streak, 1):
                next_in = streak_day - current_streak
                next_pts = pts
                break

        return SigninSummary(
            enabled=True,
            currency_name=currency_name,
            current_points=int(points.CURRENT_POINTS or 0) if points else 0,
            current_streak=current_streak,
            longest_streak=int(points.LONGEST_STREAK or 0) if points else 0,
            total_points=int(points.TOTAL_POINTS or 0) if points else 0,
            last_signin_date=(points.LAST_SIGNIN_DATE if points else None),
            today_signed=today_signed,
            next_bonus_in_days=next_in,
            next_bonus_points=next_pts,
        )

    @staticmethod
    async def list_history(user: UserModel, limit: int = 30) -> list[dict]:
        limit = max(1, min(int(limit or 30), 200))
        records = await SigninOperate.list_history(user.UID, limit=limit)
        return [
            {
                'date': r.SIGNIN_DATE,
                'daily_points': r.DAILY_POINTS,
                'bonus_points': r.BONUS_POINTS,
                'total': (r.DAILY_POINTS or 0) + (r.BONUS_POINTS or 0),
                'streak': r.STREAK_AT_TIME,
                'created_at': r.CREATED_AT,
            }
            for r in records
        ]
