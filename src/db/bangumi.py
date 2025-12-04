"""
Bangumi 番剧求片模块
"""
from enum import Enum
from typing import Optional, List

from sqlalchemy import or_, select
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db.utils import create_database


class ReqStatus(Enum):
    """请求状态"""
    UNHANDLED = 0  # 未处理
    ACCEPTED = 1   # 已接受
    REJECTED = 2   # 已拒绝
    COMPLETED = 3  # 已完成


class BangumiDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class BangumiUserModel(BangumiDatabaseModel):
    """Bangumi 用户配置"""
    __tablename__ = 'user'
    telegram_id: Mapped[int] = mapped_column(primary_key=True, index=True)
    access_token: Mapped[Optional[str]] = mapped_column(nullable=True)
    auto_update: Mapped[bool] = mapped_column(default=True)  # 每天自动同步看完的番剧
    data: Mapped[Optional[str]] = mapped_column(nullable=True)  # 预留的其他配置(JSON)


class BangumiRequireModel(BangumiDatabaseModel):
    """番剧求片请求"""
    __tablename__ = 'require'
    id: Mapped[int] = mapped_column(primary_key=True, index=True, autoincrement=True)
    telegram_id: Mapped[int] = mapped_column(index=True)  # 发起者 Telegram ID
    bangumi_id: Mapped[int] = mapped_column(index=True)   # Bangumi 番剧ID
    status: Mapped[int] = mapped_column(default=ReqStatus.UNHANDLED.value)
    timestamp: Mapped[int] = mapped_column()  # 发起时间戳
    other_info: Mapped[Optional[str]] = mapped_column(nullable=True)  # 预留信息(JSON)


create_database("bangumi", BangumiDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "bangumi.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
BangumiSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)


class BangumiUserOperate:
    """Bangumi 用户操作"""

    @staticmethod
    async def add_user(user: BangumiUserModel) -> None:
        """添加用户"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                session.add(user)

    @staticmethod
    async def get_user(telegram_id: int) -> Optional[BangumiUserModel]:
        """根据 Telegram ID 获取用户"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiUserModel).filter_by(telegram_id=telegram_id).limit(1)
            )
            return result.scalar_one_or_none()

    @staticmethod
    async def update_user(user: BangumiUserModel) -> None:
        """更新用户信息"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                await session.merge(user)

    @staticmethod
    async def delete_user(telegram_id: int) -> bool:
        """删除用户"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    select(BangumiUserModel).filter_by(telegram_id=telegram_id)
                )
                user = result.scalar_one_or_none()
                if user:
                    await session.delete(user)
                    return True
                return False

    @staticmethod
    async def get_auto_update_users() -> List[BangumiUserModel]:
        """获取所有开启自动更新的用户"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiUserModel).filter_by(auto_update=True)
            )
            return list(result.scalars().all())


class BangumiRequireOperate:
    """Bangumi 求片操作"""

    @staticmethod
    async def add_require(data: BangumiRequireModel) -> None:
        """添加求片请求"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                session.add(data)

    @staticmethod
    async def get_require(req_id: int) -> Optional[BangumiRequireModel]:
        """根据请求ID获取求片"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiRequireModel).filter_by(id=req_id).limit(1)
            )
            return result.scalar_one_or_none()

    @staticmethod
    async def update_require(data: BangumiRequireModel) -> None:
        """更新求片请求"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                await session.merge(data)

    @staticmethod
    async def update_status(req_id: int, status: ReqStatus) -> bool:
        """更新求片状态"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    select(BangumiRequireModel).filter_by(id=req_id)
                )
                req = result.scalar_one_or_none()
                if req:
                    req.status = status.value
                    await session.merge(req)
                    return True
                return False

    @staticmethod
    async def is_bangumi_exist(bangumi_id: int) -> Optional[BangumiRequireModel]:
        """检查番剧是否已被请求过"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiRequireModel).filter_by(bangumi_id=bangumi_id).limit(1)
            )
            return result.scalar_one_or_none()

    @staticmethod
    async def get_pending_list() -> List[BangumiRequireModel]:
        """获取所有待处理的求片（未处理 + 已接受）"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiRequireModel).filter(
                    or_(
                        BangumiRequireModel.status == ReqStatus.UNHANDLED.value,
                        BangumiRequireModel.status == ReqStatus.ACCEPTED.value
                    )
                )
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_requires_by_user(telegram_id: int) -> List[BangumiRequireModel]:
        """获取用户的所有求片请求"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiRequireModel).filter_by(telegram_id=telegram_id)
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_requires_by_status(status: ReqStatus) -> List[BangumiRequireModel]:
        """根据状态获取求片列表"""
        async with BangumiSessionFactory() as session:
            result = await session.execute(
                select(BangumiRequireModel).filter_by(status=status.value)
            )
            return list(result.scalars().all())

    @staticmethod
    async def delete_require(req_id: int) -> bool:
        """删除求片请求"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    select(BangumiRequireModel).filter_by(id=req_id)
                )
                req = result.scalar_one_or_none()
                if req:
                    await session.delete(req)
                    return True
                return False

