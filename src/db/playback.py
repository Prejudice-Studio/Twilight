"""
播放记录数据库模块

存储用户播放记录，用于统计排行榜
"""
import time
from typing import Optional, List
from sqlalchemy import select, func, String, Integer, Boolean, desc
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database


class PlaybackDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class PlaybackModel(PlaybackDatabaseModel):
    """播放记录"""
    __tablename__ = "playback"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)  # 本地用户 UID
    EMBY_USER_ID: Mapped[str] = mapped_column(String, index=True, nullable=False)  # Emby 用户 ID
    ITEM_ID: Mapped[str] = mapped_column(String, index=True, nullable=False)  # 媒体项 ID
    ITEM_NAME: Mapped[str] = mapped_column(String, nullable=True)  # 媒体名称
    ITEM_TYPE: Mapped[str] = mapped_column(String, nullable=True)  # 媒体类型
    SERIES_NAME: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 剧集名称
    SEASON_NAME: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 季度名称
    PLAY_METHOD: Mapped[str] = mapped_column(String, nullable=True)  # 播放方式
    CLIENT: Mapped[str] = mapped_column(String, nullable=True)  # 客户端
    DEVICE_NAME: Mapped[str] = mapped_column(String, nullable=True)  # 设备名称
    START_TIME: Mapped[int] = mapped_column(Integer, nullable=False)  # 开始时间戳
    END_TIME: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)  # 结束时间戳
    DURATION: Mapped[int] = mapped_column(Integer, default=0)  # 播放时长（秒）
    POSITION_TICKS: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)  # 播放位置
    IS_PAUSED: Mapped[bool] = mapped_column(Boolean, default=False)  # 是否暂停
    IP_ADDRESS: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # IP 地址


class DailyStatsModel(PlaybackDatabaseModel):
    """每日统计"""
    __tablename__ = "daily_stats"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    DATE: Mapped[str] = mapped_column(String, index=True, nullable=False)  # 日期 YYYY-MM-DD
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    PLAY_COUNT: Mapped[int] = mapped_column(Integer, default=0)  # 播放次数
    PLAY_DURATION: Mapped[int] = mapped_column(Integer, default=0)  # 播放时长（秒）
    UNIQUE_ITEMS: Mapped[int] = mapped_column(Integer, default=0)  # 不重复媒体数


create_database("playback", PlaybackDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "playback.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
PlaybackSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)


class PlaybackOperate:
    """播放记录操作"""
    
    @staticmethod
    async def add_playback(record: PlaybackModel) -> None:
        """添加播放记录"""
        async with PlaybackSessionFactory() as session:
            async with session.begin():
                session.add(record)
    
    @staticmethod
    async def update_playback(record_id: int, **kwargs) -> bool:
        """更新播放记录"""
        from sqlalchemy import update
        async with PlaybackSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(PlaybackModel).where(PlaybackModel.ID == record_id).values(**kwargs)
                )
                return True
    
    @staticmethod
    async def get_user_playback(uid: int, limit: int = 50) -> List[PlaybackModel]:
        """获取用户播放记录"""
        async with PlaybackSessionFactory() as session:
            result = await session.execute(
                select(PlaybackModel)
                .filter_by(UID=uid)
                .order_by(desc(PlaybackModel.START_TIME))
                .limit(limit)
            )
            return list(result.scalars().all())
    
    @staticmethod
    async def get_active_session(emby_user_id: str, item_id: str) -> Optional[PlaybackModel]:
        """获取活跃的播放会话"""
        async with PlaybackSessionFactory() as session:
            result = await session.execute(
                select(PlaybackModel)
                .filter_by(EMBY_USER_ID=emby_user_id, ITEM_ID=item_id, END_TIME=None)
                .order_by(desc(PlaybackModel.START_TIME))
                .limit(1)
            )
            return result.scalar_one_or_none()
    
    @staticmethod
    async def get_user_total_duration(uid: int) -> int:
        """获取用户总播放时长"""
        async with PlaybackSessionFactory() as session:
            result = await session.execute(
                select(func.sum(PlaybackModel.DURATION))
                .filter_by(UID=uid)
            )
            return result.scalar_one() or 0
    
    @staticmethod
    async def get_user_play_count(uid: int) -> int:
        """获取用户播放次数"""
        async with PlaybackSessionFactory() as session:
            result = await session.execute(
                select(func.count())
                .select_from(PlaybackModel)
                .filter_by(UID=uid)
            )
            return result.scalar_one() or 0
    
    @staticmethod
    async def get_play_ranking(
        start_time: int = None,
        end_time: int = None,
        limit: int = 10,
        by: str = 'duration'  # duration 或 count
    ) -> List[dict]:
        """
        获取播放排行榜
        
        :param start_time: 开始时间戳
        :param end_time: 结束时间戳
        :param limit: 返回数量
        :param by: 排序方式 (duration=时长, count=次数)
        """
        async with PlaybackSessionFactory() as session:
            # 构建查询
            if by == 'duration':
                query = (
                    select(
                        PlaybackModel.UID,
                        func.sum(PlaybackModel.DURATION).label('total')
                    )
                    .group_by(PlaybackModel.UID)
                    .order_by(desc('total'))
                    .limit(limit)
                )
            else:
                query = (
                    select(
                        PlaybackModel.UID,
                        func.count().label('total')
                    )
                    .group_by(PlaybackModel.UID)
                    .order_by(desc('total'))
                    .limit(limit)
                )
            
            # 添加时间筛选
            if start_time:
                query = query.filter(PlaybackModel.START_TIME >= start_time)
            if end_time:
                query = query.filter(PlaybackModel.START_TIME <= end_time)
            
            result = await session.execute(query)
            
            return [{'uid': row[0], 'total': row[1]} for row in result.all()]
    
    @staticmethod
    async def get_media_ranking(
        start_time: int = None,
        end_time: int = None,
        limit: int = 10
    ) -> List[dict]:
        """获取媒体播放排行"""
        async with PlaybackSessionFactory() as session:
            query = (
                select(
                    PlaybackModel.ITEM_ID,
                    PlaybackModel.ITEM_NAME,
                    PlaybackModel.ITEM_TYPE,
                    func.count().label('play_count'),
                    func.sum(PlaybackModel.DURATION).label('total_duration')
                )
                .group_by(PlaybackModel.ITEM_ID)
                .order_by(desc('play_count'))
                .limit(limit)
            )
            
            if start_time:
                query = query.filter(PlaybackModel.START_TIME >= start_time)
            if end_time:
                query = query.filter(PlaybackModel.START_TIME <= end_time)
            
            result = await session.execute(query)
            
            return [{
                'item_id': row[0],
                'item_name': row[1],
                'item_type': row[2],
                'play_count': row[3],
                'total_duration': row[4],
            } for row in result.all()]


class DailyStatsOperate:
    """每日统计操作"""
    
    @staticmethod
    async def update_daily_stats(date: str, uid: int, play_count: int, duration: int, items: int) -> None:
        """更新每日统计"""
        from sqlalchemy import update
        async with PlaybackSessionFactory() as session:
            # 检查是否存在
            result = await session.execute(
                select(DailyStatsModel).filter_by(DATE=date, UID=uid)
            )
            existing = result.scalar_one_or_none()
            
            if existing:
                async with session.begin():
                    await session.execute(
                        update(DailyStatsModel)
                        .where(DailyStatsModel.DATE == date, DailyStatsModel.UID == uid)
                        .values(
                            PLAY_COUNT=DailyStatsModel.PLAY_COUNT + play_count,
                            PLAY_DURATION=DailyStatsModel.PLAY_DURATION + duration,
                            UNIQUE_ITEMS=items
                        )
                    )
            else:
                async with session.begin():
                    session.add(DailyStatsModel(
                        DATE=date,
                        UID=uid,
                        PLAY_COUNT=play_count,
                        PLAY_DURATION=duration,
                        UNIQUE_ITEMS=items
                    ))
    
    @staticmethod
    async def get_daily_ranking(date: str, limit: int = 10) -> List[dict]:
        """获取日榜"""
        async with PlaybackSessionFactory() as session:
            result = await session.execute(
                select(DailyStatsModel)
                .filter_by(DATE=date)
                .order_by(desc(DailyStatsModel.PLAY_DURATION))
                .limit(limit)
            )
            
            return [{
                'uid': r.UID,
                'play_count': r.PLAY_COUNT,
                'duration': r.PLAY_DURATION,
                'items': r.UNIQUE_ITEMS,
            } for r in result.scalars().all()]

