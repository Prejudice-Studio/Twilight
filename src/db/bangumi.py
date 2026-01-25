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
    UNHANDLED = 0  # 待处理
    ACCEPTED = 1   # 已接受
    REJECTED = 2   # 已拒绝
    COMPLETED = 3  # 已完成
    DOWNLOADING = 4 # 正在下载 (外部状态)


class BangumiDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class BangumiUserModel(BangumiDatabaseModel):
    """Bangumi 用户配置"""
    __tablename__ = 'user'
    telegram_id: Mapped[int] = mapped_column(primary_key=True, index=True)
    access_token: Mapped[Optional[str]] = mapped_column(nullable=True)
    auto_update: Mapped[bool] = mapped_column(default=True)  # 每天自动同步看完的番剧
    data: Mapped[Optional[str]] = mapped_column(nullable=True)  # 预留的其他配置(JSON)


class BaseRequireModel:
    """求片请求基础类"""
    id: Mapped[int] = mapped_column(primary_key=True, index=True, autoincrement=True)
    telegram_id: Mapped[int] = mapped_column(index=True)  # 发起者 Telegram ID
    status: Mapped[int] = mapped_column(default=ReqStatus.UNHANDLED.value)
    timestamp: Mapped[int] = mapped_column()  # 发起时间戳
    
    # 结构化信息
    title: Mapped[str] = mapped_column(nullable=True)
    season: Mapped[Optional[int]] = mapped_column(nullable=True)
    year: Mapped[Optional[str]] = mapped_column(nullable=True)
    media_type: Mapped[str] = mapped_column(nullable=True)  # movie/tv/anime
    
    # 外部标识
    require_key: Mapped[str] = mapped_column(unique=True, index=True) # 外部修改状态的唯一Key
    
    # 管理员信息
    admin_note: Mapped[Optional[str]] = mapped_column(nullable=True)
    
    other_info: Mapped[Optional[str]] = mapped_column(nullable=True)  # 预留信息(JSON)


class BangumiRequireModel(BangumiDatabaseModel, BaseRequireModel):
    """番剧求片请求"""
    __tablename__ = 'require_bangumi'
    bangumi_id: Mapped[int] = mapped_column(index=True)   # Bangumi 番剧ID


class TMDBRequireModel(BangumiDatabaseModel, BaseRequireModel):
    """TMDB 求片请求"""
    __tablename__ = 'require_tmdb'
    tmdb_id: Mapped[str] = mapped_column(index=True)      # TMDB ID (tv:123 or movie:123)


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
    """求片操作 (支持 Bangumi 和 TMDB)"""

    @staticmethod
    async def add_require(data: BangumiDatabaseModel) -> None:
        """添加求片请求"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                session.add(data)

    @staticmethod
    async def get_require(req_id: int, source: str = None) -> Optional[BangumiDatabaseModel]:
        """获取求片"""
        async with BangumiSessionFactory() as session:
            if source:
                model = BangumiRequireModel if source == 'bangumi' else TMDBRequireModel
                result = await session.execute(
                    select(model).filter_by(id=req_id).limit(1)
                )
                return result.scalar_one_or_none()
            else:
                # 依次尝试
                for model in [BangumiRequireModel, TMDBRequireModel]:
                    result = await session.execute(
                        select(model).filter_by(id=req_id).limit(1)
                    )
                    req = result.scalar_one_or_none()
                    if req:
                        return req
                return None

    @staticmethod
    async def get_require_by_key(require_key: str) -> Optional[BangumiDatabaseModel]:
        """通过 require_key 获取求片"""
        async with BangumiSessionFactory() as session:
            # 检查 Bangumi
            result = await session.execute(
                select(BangumiRequireModel).filter_by(require_key=require_key).limit(1)
            )
            req = result.scalar_one_or_none()
            if req:
                return req
            
            # 检查 TMDB
            result = await session.execute(
                select(TMDBRequireModel).filter_by(require_key=require_key).limit(1)
            )
            return result.scalar_one_or_none()

    @staticmethod
    async def update_require(data: BangumiDatabaseModel) -> None:
        """更新求片请求"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                await session.merge(data)

    @staticmethod
    async def update_status_by_key(require_key: str, status: ReqStatus, note: str = None) -> bool:
        """根据 Key 更新求片状态"""
        async with BangumiSessionFactory() as session:
            async with session.begin():
                req = await BangumiRequireOperate.get_require_by_key(require_key)
                if req:
                    req.status = status.value
                    if note:
                        req.admin_note = note
                    await session.merge(req)
                    return True
                return False

    @staticmethod
    async def is_exist(unique_id: str, source: str = 'bangumi', season: int = None) -> Optional[BangumiDatabaseModel]:
        """检查媒体是否已被请求过"""
        async with BangumiSessionFactory() as session:
            if source == 'bangumi':
                stmt = select(BangumiRequireModel).filter_by(bangumi_id=int(unique_id))
                if season is not None:
                    stmt = stmt.filter_by(season=season)
            else:
                stmt = select(TMDBRequireModel).filter_by(tmdb_id=str(unique_id))
                if season is not None:
                    stmt = stmt.filter_by(season=season)
            
            result = await session.execute(stmt.limit(1))
            return result.scalar_one_or_none()

    @staticmethod
    async def get_all_pending_list() -> List[BangumiDatabaseModel]:
        """获取所有待处理的求片"""
        async with BangumiSessionFactory() as session:
            results = []
            for model in [BangumiRequireModel, TMDBRequireModel]:
                result = await session.execute(
                    select(model).filter(
                        or_(
                            model.status == ReqStatus.UNHANDLED.value,
                            model.status == ReqStatus.ACCEPTED.value,
                            model.status == ReqStatus.DOWNLOADING.value
                        )
                    )
                )
                results.extend(list(result.scalars().all()))
            return results

    @staticmethod
    async def get_all_requires_by_user(telegram_id: int) -> List[BangumiDatabaseModel]:
        """获取用户的所有求片请求"""
        async with BangumiSessionFactory() as session:
            results = []
            for model in [BangumiRequireModel, TMDBRequireModel]:
                result = await session.execute(
                    select(model).filter_by(telegram_id=telegram_id)
                )
                results.extend(list(result.scalars().all()))
            return results

    @staticmethod
    async def get_all_requires_by_status(status: ReqStatus) -> List[BangumiDatabaseModel]:
        """根据状态获取所有求片列表"""
        async with BangumiSessionFactory() as session:
            results = []
            for model in [BangumiRequireModel, TMDBRequireModel]:
                result = await session.execute(
                    select(model).filter_by(status=status.value)
                )
                results.extend(list(result.scalars().all()))
            return results

    @staticmethod
    async def delete_require(req_id: int, source: str = 'bangumi') -> bool:
        """删除求片请求"""
        model = BangumiRequireModel if source == 'bangumi' else TMDBRequireModel
        async with BangumiSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    select(model).filter_by(id=req_id)
                )
                req = result.scalar_one_or_none()
                if req:
                    await session.delete(req)
                    return True
                return False

