import time
from typing import Optional, List

from sqlalchemy import select, update, String, Integer
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db.utils import create_database
class ScoreDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass
class ScoreModel(ScoreDatabaseModel):
    __tablename__ = "scores"
    UID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True)
    TELEGRAM_ID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True, index=True)
    SCORE: Mapped[int] = mapped_column(Integer, nullable=False)
    CHECKIN_TIME: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    CHECKIN_COUNT: Mapped[int] = mapped_column(Integer, default=0)
    TOTAL_EARNED: Mapped[Optional[int]] = mapped_column(Integer, default=0, nullable=True)  # 累计获得积分
    TOTAL_SPENT: Mapped[Optional[int]] = mapped_column(Integer, default=0, nullable=True)  # 累计消费积分


class RedPacketModel(ScoreDatabaseModel):
    __tablename__ = "red_packets"
    RPID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    SENDER_UID: Mapped[int] = mapped_column(Integer, nullable=False)
    SENDER_TELEGRAM_ID: Mapped[int] = mapped_column(Integer, nullable=False)
    AMOUNT: Mapped[int] = mapped_column(Integer, nullable=False)
    COUNT: Mapped[int] = mapped_column(Integer, nullable=False)
    CURRENT_AMOUNT: Mapped[int] = mapped_column(Integer, nullable=False)
    STATUS: Mapped[int] = mapped_column(Integer, default=0, nullable=False)  # 0 未领取 1 已领完 2 已撤回
    TYPE: Mapped[int] = mapped_column(Integer, default=1, nullable=False)    # 1 随机 2 均分 0 定向
    CREATE_TIME: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)
    HISTORY: Mapped[Optional[str]] = mapped_column(String, default='')
    RP_KEY: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    OTHER: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)


class ScoreHistoryModel(ScoreDatabaseModel):
    """积分变动历史"""
    __tablename__ = "score_history"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    TYPE: Mapped[str] = mapped_column(String, nullable=False)  # checkin, transfer, renew, admin, redpacket 等
    AMOUNT: Mapped[int] = mapped_column(Integer, nullable=False)  # 变动数量（正数为增加，负数为减少）
    BALANCE_AFTER: Mapped[int] = mapped_column(Integer, nullable=False)  # 变动后余额
    NOTE: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 备注
    RELATED_UID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)  # 关联用户（如转账）
    CREATED_AT: Mapped[int] = mapped_column(Integer, nullable=False)  # 创建时间戳


create_database("score", ScoreDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "score.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
ScoreSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)
class ScoreOperate:
    @staticmethod
    async def add_score(score: ScoreModel) -> None:
        """添加积分记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.add(score)

    @staticmethod
    async def get_score_by_uid(uid: int) -> Optional[ScoreModel]:
        """根据UID获取积分记录"""
        async with ScoreSessionFactory() as session:
            scalar = await session.execute(select(ScoreModel).filter_by(UID=uid).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_score_by_telegram_id(telegram_id: int) -> Optional[ScoreModel]:
        """根据Telegram ID获取积分记录"""
        async with ScoreSessionFactory() as session:
            scalar = await session.execute(select(ScoreModel).filter_by(TELEGRAM_ID=telegram_id).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def update_score(score: ScoreModel) -> None:
        """更新积分记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                await session.merge(score)

    @staticmethod
    async def reset_score(uid: int) -> None:
        """重置用户积分为0"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(ScoreModel).where(ScoreModel.UID == uid).values(SCORE=0)
                )

    @staticmethod
    async def set_score_by_uid(uid: int, score: int) -> None:
        """根据 UID 设置积分值"""
        # 先通过 UID 查找积分记录
        score_record = await ScoreOperate.get_score_by_uid(uid)
        
        if score_record:
            # 更新现有记录
            score_record.SCORE = score
            # 如果积分增加，更新累计获得
            if score > score_record.SCORE:
                if score_record.TOTAL_EARNED is None:
                    score_record.TOTAL_EARNED = 0
                score_record.TOTAL_EARNED = (score_record.TOTAL_EARNED or 0) + (score - score_record.SCORE)
            await ScoreOperate.update_score(score_record)
        else:
            # 如果不存在，获取用户信息创建新记录
            from src.db.user import UserOperate
            user = await UserOperate.get_user_by_uid(uid)
            if user:
                # 创建新记录
                new_score = ScoreModel(
                    UID=uid,
                    TELEGRAM_ID=user.TELEGRAM_ID,
                    SCORE=score,
                    CHECKIN_TIME=0,
                    CHECKIN_COUNT=0,
                    TOTAL_EARNED=score if score > 0 else 0,
                    TOTAL_SPENT=0
                )
                await ScoreOperate.add_score(new_score)
    
    @staticmethod
    async def set_score(telegram_id: int, score: int) -> None:
        """根据 Telegram ID 设置积分值（兼容旧方法）"""
        from src.db.user import UserOperate
        user = await UserOperate.get_user_by_telegram_id(telegram_id)
        if not user:
            raise ValueError(f"未找到绑定 Telegram ID {telegram_id} 的用户")
        await ScoreOperate.set_score_by_uid(user.UID, score)

    @staticmethod
    async def delete_score(score: ScoreModel) -> None:
        """删除积分记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                existing = await session.execute(select(ScoreModel).filter_by(UID=score.UID))
                db_score = existing.scalar_one_or_none()
                if db_score:
                    await session.delete(db_score)

    @staticmethod
    async def add_red_packet(red_packet: RedPacketModel) -> None:
        """添加红包记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.add(red_packet)

    @staticmethod
    async def get_red_packet_by_rpid(rpid: int) -> Optional[RedPacketModel]:
        """根据红包ID获取红包记录"""
        async with ScoreSessionFactory() as session:
            scalar = await session.execute(select(RedPacketModel).filter_by(RPID=rpid).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_red_packets_by_sender_uid(sender_uid: int) -> List[RedPacketModel]:
        """根据发送者UID获取红包记录列表"""
        async with ScoreSessionFactory() as session:
            result = await session.execute(select(RedPacketModel).filter_by(SENDER_UID=sender_uid))
            return list[RedPacketModel](result.scalars().all())

    @staticmethod
    async def get_red_packets_by_sender_telegram_id(sender_telegram_id: int) -> List[RedPacketModel]:
        """根据发送者Telegram ID获取红包记录列表"""
        async with ScoreSessionFactory() as session:
            result = await session.execute(select(RedPacketModel).filter_by(SENDER_TELEGRAM_ID=sender_telegram_id))
            return list[RedPacketModel](result.scalars().all())

    @staticmethod
    async def update_red_packet(red_packet: RedPacketModel) -> None:
        """更新红包记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                await session.merge(red_packet)

    @staticmethod
    async def delete_red_packet(red_packet: RedPacketModel) -> None:
        """删除红包记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                existing = await session.execute(select(RedPacketModel).filter_by(RPID=red_packet.RPID))
                db_rp = existing.scalar_one_or_none()
                if db_rp:
                    await session.delete(db_rp)

    @staticmethod
    async def get_active_red_packets() -> List[RedPacketModel]:
        """获取所有未领取的红包记录"""
        async with ScoreSessionFactory() as session:
            result = await session.execute(select(RedPacketModel).filter_by(STATUS=0))
            return list[RedPacketModel](result.scalars().all())

    @staticmethod
    async def get_user_score_ranking(limit: int = 10) -> List[ScoreModel]:
        """获取积分排行榜"""
        async with ScoreSessionFactory() as session:
            result = await session.execute(
                select(ScoreModel).order_by(ScoreModel.SCORE.desc()).limit(limit)
            )
            return list[ScoreModel](result.scalars().all())


class ScoreHistoryOperate:
    """积分历史操作"""
    
    @staticmethod
    async def add_history(
        uid: int,
        type_: str,
        amount: int,
        balance_after: int,
        note: str = None,
        related_uid: int = None
    ) -> None:
        """添加积分历史记录"""
        async with ScoreSessionFactory() as session:
            async with session.begin():
                history = ScoreHistoryModel(
                    UID=uid,
                    TYPE=type_,
                    AMOUNT=amount,
                    BALANCE_AFTER=balance_after,
                    NOTE=note,
                    RELATED_UID=related_uid,
                    CREATED_AT=int(time.time())
                )
                session.add(history)
    
    @staticmethod
    async def get_history_by_uid(uid: int, limit: int = 20, offset: int = 0) -> List[ScoreHistoryModel]:
        """根据 UID 获取积分历史记录"""
        from sqlalchemy import desc
        async with ScoreSessionFactory() as session:
            result = await session.execute(
                select(ScoreHistoryModel)
                .filter_by(UID=uid)
                .order_by(desc(ScoreHistoryModel.CREATED_AT))
                .limit(limit)
                .offset(offset)
            )
            return list(result.scalars().all())
    
    @staticmethod
    async def get_history_count(uid: int) -> int:
        """获取用户的积分历史记录总数"""
        from sqlalchemy import func
        async with ScoreSessionFactory() as session:
            result = await session.execute(
                select(func.count())
                .select_from(ScoreHistoryModel)
                .filter_by(UID=uid)
            )
            return result.scalar_one() or 0