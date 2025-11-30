from enum import Enum
import random
import time
import hashlib
import logging
from typing import List
from sqlalchemy import delete, select, update, func , Integer , String
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database

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
    NEW: 1 # 新增
    SUB: 2 # 字幕
    RES: 3 # 画质

class RequireModel(RequireDatabaseModel):
    __tablename__ = 'require'
    REQUIRE_ID: Mapped[int] = mapped_column(Integer, primary_key=True , index=True) # 请求的ID
    TYPE: Mapped[int] = mapped_column(Integer, nullable=False, index=True) # 请求的类型
    UID: Mapped[int] = mapped_column(Integer, index=True , nullable=False) # 用户的ID
    STATUS: Mapped[int] = mapped_column(Integer, default=Status.UNHANDLED.value, nullable=False , index=True) # 请求的状态
    CREATE_TIME: Mapped[int] = mapped_column(Integer, nullable=False , index=True) # 请求的创建时间戳
    STATUS_CHANGE_TIME: Mapped[int] = mapped_column(Integer, nullable=True , index=True) # 请求的接受/拒绝的时间戳
    FIN_TIME: Mapped[int] = mapped_column(Integer, nullable=True , index=True) # 请求的完成时间戳
    URL: Mapped[str] = mapped_column(String, index=True , nullable=False) # 请求的TMDB URL
    SEASON: Mapped[int] = mapped_column(Integer, nullable=True , index=True) # 请求的季度
    REQ_KEY: Mapped[str] = mapped_column(String, nullable=False , index=True) # 请求的Key , 外部可以通过此Key对Require进行操作
    OTHER: Mapped[str] = mapped_column(String, nullable=True) # 其他信息 , json格式
    
create_database("require", RequireDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "require.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
UsersSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)

class RequireOperate:
    @staticmethod
    async def generate_key() -> str:
        """
        根据UID、季度生成请求Key
        格式为 req-xxxxxx-yyyyyy
        xxxxxx为根据UID随机生成的6位数字
        yyyyyy为根据季度随机生成的6位
        """
        uid_random = random.randint(100000, 999999)
        season_random = random.randint(100000, 999999)
        key = f"req-{uid_random:06d}-{season_random:06d}"
        # 组合Key
        return key
    
    @classmethod
    async def add_require(cls, require: RequireModel) -> bool:
        async with UsersSessionFactory() as session:
            async with session.begin():
                # 检查是否存在相同的Key
                # 如果存在相同的Key，则重新生成Key , 直到生成的Key不重复
                while True:
                    key = await cls.generate_key(require.UID, require.SEASON)
                    result = await cls.check_require_key(key)
                    if result:
                        require.REQ_KEY = key
                        # 添加
                        session.add(require)
                        return True
                    else:
                        logging.warning(f"Key {key} already exists, generate a new one.")
                        continue
       
    async def check_require_key(self, key: str) -> bool:
        """
        检查请求Key是否存在
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                require = result.scalars().first()
                if require:
                    return True
                else:
                    return False
                
    @classmethod
    async def get_require_by_key(cls, key: str) -> RequireModel:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                require = result.scalars().first()
                return require
    
    @classmethod
    async def update_require_status_by_key(cls, key: str, status: int) -> bool:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                require = result.scalars().first()
                if require:
                    require.STATUS = status
                    require.STATUS_CHANGE_TIME = int(time.time())
                    session.add(require)
                    return True
                else:
                    return False
                
    @classmethod
    async def delete_require_by_key(cls, key: str) -> bool:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                require = result.scalars().first()
                if require:
                    session.delete(require)
                    return True
                else:
                    return False
                
    @classmethod
    async def update_require_by_key(cls, key: str, require: RequireModel) -> bool:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(REQ_KEY=key))
                old_require = result.scalars().first()
                if old_require:
                    require.REQUIRE_ID = old_require.REQUIRE_ID
                    require.UID = old_require.UID
                    require.TYPE = old_require.TYPE
                    require.STATUS = old_require.STATUS
                    require.CREATE_TIME = old_require.CREATE_TIME
                    require.STATUS_CHANGE_TIME = old_require.STATUS_CHANGE_TIME
                    require.FIN_TIME = old_require.FIN_TIME
                    require.URL = old_require.URL
                    require.SEASON = old_require.SEASON
                    require.REQ_KEY = old_require.REQ_KEY
                    require.OTHER = old_require.OTHER
                    session.add(require)
                    return True
                else:
                    return False
                
    @classmethod
    async def get_require_by_uid(cls, uid: int) -> RequireModel | List[RequireModel] | None:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(UID=uid))
                require = result.scalars().first()
                if require:
                    return require
                else:
                    return None
    
    @classmethod
    async def get_require_by_status(cls, status: int) -> List[RequireModel]:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(STATUS=status))
                requires = result.scalars().all()
                return requires
    
    @classmethod
    async def get_require_by_type(cls, type: int) -> List[RequireModel]:
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(select(RequireModel).filter_by(TYPE=type))
                requires = result.scalars().all()
                return requires