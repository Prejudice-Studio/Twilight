from enum import Enum
import random
import time
import logging
from typing import List, Optional

from sqlalchemy import select, Integer, String
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db.utils import create_database

logging.basicConfig(level=logging.INFO)
class RequireDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass
class Status(Enum):
    """请求状态"""
    UNHANDLED = 0   # 未处理
    ACCEPTED = 1    # 已接受
    REJECTED = 2    # 已拒绝
    COMPLETED = 3   # 已完成


class Type(Enum):
    """请求类型"""
    NEW = 1  # 新增
    SUB = 2  # 字幕
    RES = 3  # 画质


class RequireModel(RequireDatabaseModel):
    __tablename__ = 'require'
    REQUIRE_ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True)
    TYPE: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    STATUS: Mapped[int] = mapped_column(Integer, default=Status.UNHANDLED.value, nullable=False, index=True)
    CREATE_TIME: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    STATUS_CHANGE_TIME: Mapped[Optional[int]] = mapped_column(Integer, nullable=True, index=True)
    FIN_TIME: Mapped[Optional[int]] = mapped_column(Integer, nullable=True, index=True)
    URL: Mapped[str] = mapped_column(String, index=True, nullable=False)
    SEASON: Mapped[Optional[int]] = mapped_column(Integer, nullable=True, index=True)
    REQ_KEY: Mapped[str] = mapped_column(String, nullable=False, index=True)
    OTHER: Mapped[Optional[str]] = mapped_column(String, nullable=True)


create_database("require", RequireDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "require.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
RequireSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)


class RequireOperate:
    @staticmethod
    async def generate_key() -> str:
        """
        生成请求Key
        格式为 req-xxxxxx-yyyyyy
        xxxxxx为随机生成的6位数字
        yyyyyy为随机生成的6位大小写字母
        """
        uid_random = random.randint(100000, 999999)
        letters = ''.join(random.choices("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", k=6))
        return f"req-{uid_random:06d}-{letters}"

    @staticmethod
    async def check_require_key_exists(key: str) -> bool:
        """
        检查请求Key是否已存在
        返回: True 表示已存在, False 表示不存在
        """
        async with RequireSessionFactory() as session:
            result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
            require = result.scalars().first()
            return require is not None

    @classmethod
    async def add_require(cls, require: RequireModel) -> bool:
        """
        添加请求记录，自动生成唯一的请求Key
        """
        async with RequireSessionFactory() as session:
            async with session.begin():
                # 生成唯一的Key
                max_attempts = 10
                for _ in range(max_attempts):
                    key = await cls.generate_key()
                    if not await cls.check_require_key_exists(key):
                        require.REQ_KEY = key
                        session.add(require)
                        return True
                    logging.warning(f"Key {key} already exists, generating a new one.")
                
                logging.error("Failed to generate unique key after max attempts")
                return False

    @staticmethod
    async def get_require_by_key(key: str) -> Optional[RequireModel]:
        """根据Key获取请求"""
        async with RequireSessionFactory() as session:
            result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
            return result.scalars().first()

    @staticmethod
    async def update_require_status_by_key(key: str, status: int) -> bool:
        """根据Key更新请求状态"""
        async with RequireSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                require = result.scalars().first()
                if require:
                    require.STATUS = status
                    require.STATUS_CHANGE_TIME = int(time.time())
                    session.add(require)
                    return True
                return False

    @staticmethod
    async def delete_require_by_key(key: str) -> bool:
        """根据Key删除请求"""
        async with RequireSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                require = result.scalars().first()
                if require:
                    await session.delete(require)
                    return True
                return False

    @staticmethod
    async def update_require_by_key(key: str, require: RequireModel) -> bool:
        """根据Key更新请求"""
        async with RequireSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                existing = result.scalars().first()
                if existing:
                    await session.merge(require)
                    return True
                return False

    @staticmethod
    async def get_requires_by_uid(uid: int) -> List[RequireModel]:
        """根据用户UID获取所有请求"""
        async with RequireSessionFactory() as session:
            result = await session.execute(select(RequireModel).filter_by(UID=uid))
            return list[RequireModel](result.scalars().all())

    @staticmethod
    async def get_requires_by_status(status: int) -> List[RequireModel]:
        """根据状态获取所有请求"""
        async with RequireSessionFactory() as session:
            result = await session.execute(select(RequireModel).filter_by(STATUS=status))
            return list[RequireModel](result.scalars().all())

    @staticmethod
    async def get_requires_by_type(type_: int) -> List[RequireModel]:
        """根据类型获取所有请求"""
        async with RequireSessionFactory() as session:
            result = await session.execute(select(RequireModel).filter_by(TYPE=type_))
            return list[RequireModel](result.scalars().all())
