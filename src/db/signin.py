"""
签到与积分数据库模块

存储用户签到记录与积分余额。积分仅装饰用途，不影响系统权限。
"""
import time
from typing import Optional, List

from sqlalchemy import select, update, func, String, Integer, desc
from sqlalchemy.ext.asyncio import AsyncAttrs
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.db.utils import init_async_db


class SigninDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class UserPointsModel(SigninDatabaseModel):
    """用户积分余额"""
    __tablename__ = "user_points"
    UID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True)
    CURRENT_POINTS: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    TOTAL_POINTS: Mapped[int] = mapped_column(Integer, default=0, nullable=False)  # 累计获得（含已花费）
    CURRENT_STREAK: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    LONGEST_STREAK: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    LAST_SIGNIN_DATE: Mapped[Optional[str]] = mapped_column(String, default=None, nullable=True)  # YYYY-MM-DD
    UPDATED_AT: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)


class SigninRecordModel(SigninDatabaseModel):
    """签到记录"""
    __tablename__ = "signin_record"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    SIGNIN_DATE: Mapped[str] = mapped_column(String, index=True, nullable=False)  # YYYY-MM-DD
    DAILY_POINTS: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    BONUS_POINTS: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    STREAK_AT_TIME: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    CREATED_AT: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)


ENGINE, SigninSessionFactory = init_async_db("signin", SigninDatabaseModel)


class SigninOperate:
    """签到 / 积分数据库操作。所有跨表写入都在同一事务内完成。"""

    @staticmethod
    async def get_user_points(uid: int) -> Optional[UserPointsModel]:
        async with SigninSessionFactory() as session:
            scalar = await session.execute(
                select(UserPointsModel).filter_by(UID=uid).limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_today_record(uid: int, date_str: str) -> Optional[SigninRecordModel]:
        async with SigninSessionFactory() as session:
            scalar = await session.execute(
                select(SigninRecordModel)
                .filter_by(UID=uid, SIGNIN_DATE=date_str)
                .limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def list_history(uid: int, limit: int = 30) -> List[SigninRecordModel]:
        async with SigninSessionFactory() as session:
            result = await session.execute(
                select(SigninRecordModel)
                .filter_by(UID=uid)
                .order_by(desc(SigninRecordModel.SIGNIN_DATE))
                .limit(limit)
            )
            return list(result.scalars().all())

    @staticmethod
    async def commit_signin(
        uid: int,
        date_str: str,
        daily_points: int,
        bonus_points: int,
        new_streak: int,
    ) -> UserPointsModel:
        """写入一次签到：累加积分、刷新连签、记录历史。"""
        now = int(time.time())
        total_today = daily_points + bonus_points
        async with SigninSessionFactory() as session:
            async with session.begin():
                points = await session.get(UserPointsModel, uid)
                if points is None:
                    points = UserPointsModel(
                        UID=uid,
                        CURRENT_POINTS=total_today,
                        TOTAL_POINTS=total_today,
                        CURRENT_STREAK=new_streak,
                        LONGEST_STREAK=new_streak,
                        LAST_SIGNIN_DATE=date_str,
                        UPDATED_AT=now,
                    )
                    session.add(points)
                else:
                    points.CURRENT_POINTS = (points.CURRENT_POINTS or 0) + total_today
                    points.TOTAL_POINTS = (points.TOTAL_POINTS or 0) + total_today
                    points.CURRENT_STREAK = new_streak
                    if new_streak > (points.LONGEST_STREAK or 0):
                        points.LONGEST_STREAK = new_streak
                    points.LAST_SIGNIN_DATE = date_str
                    points.UPDATED_AT = now

                session.add(SigninRecordModel(
                    UID=uid,
                    SIGNIN_DATE=date_str,
                    DAILY_POINTS=daily_points,
                    BONUS_POINTS=bonus_points,
                    STREAK_AT_TIME=new_streak,
                    CREATED_AT=now,
                ))
            # session.begin() commit 后再次读取，避免分离对象访问
            scalar = await session.execute(
                select(UserPointsModel).filter_by(UID=uid).limit(1)
            )
            refreshed = scalar.scalar_one_or_none()
            return refreshed or points

    @staticmethod
    async def reset_streak_if_missed(uid: int, current_date_str: str) -> Optional[UserPointsModel]:
        """如果上次签到不是昨天，把 CURRENT_STREAK 清零（不写入签到记录）。
        返回当前最新的 UserPointsModel；不存在时返回 None。"""
        from datetime import date

        async with SigninSessionFactory() as session:
            async with session.begin():
                points = await session.get(UserPointsModel, uid)
                if points is None:
                    return None
                if not points.LAST_SIGNIN_DATE:
                    return points
                try:
                    last = date.fromisoformat(points.LAST_SIGNIN_DATE)
                    today = date.fromisoformat(current_date_str)
                except ValueError:
                    return points
                delta = (today - last).days
                if delta >= 2 and (points.CURRENT_STREAK or 0) > 0:
                    points.CURRENT_STREAK = 0
                    points.UPDATED_AT = int(time.time())
                return points
