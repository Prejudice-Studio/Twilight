import time
from sqlalchemy import delete, select, update , String , Integer
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database

class ScoreDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass

class ScoreModel(ScoreDatabaseModel):
    __tablename__ = "scores"
    UID: Mapped[int] = mapped_column(Integer , primary_key=True, index=True)                                  # 用户UID
    TELEGRAM_ID: Mapped[int] = mapped_column(Integer , nullable=True, index=True)                             # Telegram ID
    SCORE: Mapped[int] = mapped_column(Integer , nullable=False)                                              # 当前积分
    CHECKIN_TIME: Mapped[int] = mapped_column(Integer , default=0, nullable=False)                            # 上次签到时间
    CHECKIN_COUNT: Mapped[int] = mapped_column(Integer , default=0)                                           # 连续签到天数

class RedPacketModel(ScoreDatabaseModel):
    __tablename__ = "red_packets"
    RPID: Mapped[int] = mapped_column(Integer , primary_key=True, index=True, autoincrement=True)             # 红包ID
    SENDER_UID: Mapped[int] = mapped_column(Integer , nullable=False)                                         # 发送者UID
    SENDER_TELEGRAM_ID: Mapped[int] = mapped_column(Integer , nullable=False)                                 # 发送者Telegram ID
    AMOUNT: Mapped[int] = mapped_column(Integer , nullable=False)                                             # 金额
    COUNT: Mapped[int] = mapped_column(Integer , nullable=False)                                              # 数量
    CURRENT_AMOUNT: Mapped[int] = mapped_column(Integer , nullable=False)                                     # 当前剩余金额
    STATUS: Mapped[int] = mapped_column(Integer , default=0, nullable=False)                                  # 状态 0 未领取 1 已领完 2 已经撤回
    TYPE: Mapped[int] = mapped_column(Integer , default=1, nullable=False)                                    # 类型 1 随机 2 均分 0 定向
    CREATE_TIME: Mapped[int] = mapped_column(Integer , default=lambda: int(time.time()), nullable=False)      # 创建时间 , 时间戳
    HISTORY: Mapped[str] = mapped_column(String , default='')                                                  # 领取记录
    RP_KEY: Mapped[str] = mapped_column(String , default='' , nullable=True)                                   # 该红包的密钥 , 外部可以通过此Key对红包进行操作
    OTHER: Mapped[str] = mapped_column(String , default='' , nullable=True)                                    # 其他信息 json

create_database("score", ScoreDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "score.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
ScoreSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)

class ScoreOperate:
    @classmethod
    async def edit_score(cls, score: ScoreModel):
        """
        修改积分记录
        """
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.merge(score)
            await session.commit()
    
    @staticmethod
    async def get_score_by_uid(uid: int) -> ScoreModel | None:
        """
        根据UID获取积分记录
        """
        async with ScoreSessionFactory() as session:
            scalar = await session.execute(select(ScoreModel).filter_by(UID=uid).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def get_score_by_telegram_id(telegram_id: int) -> ScoreModel | None:
        """
        根据Telegram ID获取积分记录
        """
        async with ScoreSessionFactory() as session:
            scalar = await session.execute(select(ScoreModel).filter_by(TELEGRAM_ID=telegram_id).limit(1))
    
    @staticmethod
    async def update_score(score: ScoreModel):
        """
        更新积分记录
        """
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.merge(score)
            await session.commit()
    
    @staticmethod
    async def delete_score(score: ScoreModel):
        """
        删除积分记录
        根据UID选出用户 , 设置积分为0
        """
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.execute(update(ScoreModel).where(ScoreModel.UID == score.UID).values(SCORE=0))
            await session.commit()
    
    @staticmethod
    async def add_red_packet(red_packet: RedPacketModel):
        """
        添加红包记录
        """
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.add(red_packet)
            await session.commit()
    
    @staticmethod
    async def get_red_packet_by_rpid(rpid: int) -> RedPacketModel | None:
        """
        根据红包ID获取红包记录
        """
        async with ScoreSessionFactory() as session:
            scalar = await session.execute(select(RedPacketModel).filter_by(RPID=rpid).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def get_red_packets_by_sender_uid(sender_uid: int) -> list[RedPacketModel]:
        """
        根据发送者UID获取红包记录列表
        """
        async with ScoreSessionFactory() as session:
            result = await session.execute(select(RedPacketModel).filter_by(SENDER_UID=sender_uid))
            return result.scalars().all()
    
    @staticmethod
    async def get_red_packets_by_sender_telegram_id(sender_telegram_id: int) -> list[RedPacketModel]:
        """
        根据发送者Telegram ID获取红包记录列表
        """
        async with ScoreSessionFactory() as session:
            result = await session.execute(select(RedPacketModel).filter_by(SENDER_TELEGRAM_ID=sender_telegram_id))
            return result.scalars().all()
    
    @staticmethod
    async def update_red_packet(red_packet: RedPacketModel):
        """
        更新红包记录
        """
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.merge(red_packet)
            await session.commit()
    
    @staticmethod
    async def delete_red_packet(red_packet: RedPacketModel):
        """
        删除红包记录
        """
        async with ScoreSessionFactory() as session:
            async with session.begin():
                session.delete(red_packet)
            await session.commit()
    
    @staticmethod
    async def get_active_red_packets() -> list[RedPacketModel]:
        """
        获取所有未领取的红包记录
        """
        async with ScoreSessionFactory() as session:
            result = await session.execute(select(RedPacketModel).filter_by(STATUS=0))
            return result.scalars().all()
    
    @staticmethod
    async def get_user_score_ranking(limit: int = 10) -> list[ScoreModel]:
        """
        获取积分排行榜
        """
        async with ScoreSessionFactory() as session:
            result = await session.execute(
                select(ScoreModel)
                .order_by(ScoreModel.SCORE.desc())
                .limit(limit)
            )
            return result.scalars().all()