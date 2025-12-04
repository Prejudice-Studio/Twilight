from enum import Enum
import time
import hashlib
import random
import logging
from typing import Optional, List, Union

from sqlalchemy import select, func, String, Integer, Boolean
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from sqlalchemy.exc import SQLAlchemyError

from src.config import Config
from src.db.utils import create_database

logger = logging.getLogger(__name__)
class Type(Enum):
    REGISTER = 1   # 注册
    RENEW = 2      # 续期
    WHITELIST = 3  # 白名单


class RegCodeDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class RegCodeModel(RegCodeDatabaseModel):
    __tablename__ = "regcode"
    CODE: Mapped[str] = mapped_column(String, primary_key=True, index=True, nullable=False)
    VALIDITY_TIME: Mapped[int] = mapped_column(Integer, default=-1, nullable=False)  # 有效时间(小时), -1永久
    TYPE: Mapped[int] = mapped_column(Integer, nullable=False)  # 类型 1:注册 2:续期 3:白名单
    UID: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 使用用户UID/UID列表
    TELEGRAM_ID: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 使用者的telegram_id
    USE_COUNT_LIMIT: Mapped[int] = mapped_column(Integer, default=1, nullable=False)  # 使用次数限制, -1无限制
    USE_COUNT: Mapped[int] = mapped_column(Integer, default=0, nullable=False)  # 已使用次数
    CREATED_TIME: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)
    DAYS: Mapped[Optional[int]] = mapped_column(Integer, default=30, nullable=True)  # 增加的天数
    ACTIVE: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)  # 是否启用
    OTHER: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 其他信息(json)


create_database("RegCode", RegCodeDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "regcode.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
RegCodeSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)


class RegCodeOperate:
    @staticmethod
    def _generate_code(vali_time: int, use_count_limit: int, day: int) -> str:
        """生成唯一的注册码"""
        unique_part = f"{vali_time}-{use_count_limit}-{day}-{random.randint(10000, 99999)}-{time.time()}"
        return "code-" + hashlib.sha1(unique_part.encode()).hexdigest()[:20]

    @staticmethod
    async def create_regcode(
        vali_time: int,
        type_: int,
        use_count_limit: int = 1,
        count: int = 1,
        day: int = 30
    ) -> Union[str, List[str]]:
        """创建指定数量的注册码并添加到数据库中"""
        codes = []
        async with RegCodeSessionFactory() as session:
            for _ in range(count):
                code = RegCodeOperate._generate_code(vali_time, use_count_limit, day)
                reg_code = RegCodeModel(
                    CODE=code,
                    VALIDITY_TIME=vali_time,
                    TYPE=type_,
                    USE_COUNT_LIMIT=use_count_limit,
                    DAYS=day
                )
                try:
                    session.add(reg_code)
                    await session.commit()
                    codes.append(code)
                except SQLAlchemyError as e:
                    await session.rollback()
                    logger.error(f"数据库操作失败: {e}")
                    raise

        return codes[0] if len(codes) == 1 else codes

    @staticmethod
    async def get_regcode_by_code(code: str) -> Optional[RegCodeModel]:
        """根据注册码获取注册码信息"""
        async with RegCodeSessionFactory() as session:
            scalar = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_regcodes_by_type(type_: int) -> List[RegCodeModel]:
        """根据类型获取所有注册码"""
        async with RegCodeSessionFactory() as session:
            result = await session.execute(select(RegCodeModel).filter_by(TYPE=type_))
            return list(result.scalars().all())
    
    @staticmethod
    async def get_all_regcodes() -> List[RegCodeModel]:
        """获取所有注册码"""
        async with RegCodeSessionFactory() as session:
            result = await session.execute(select(RegCodeModel))
            return list(result.scalars().all())

    @staticmethod
    async def update_regcode_use_count(code: str, increment: int = 1) -> bool:
        """更新注册码的使用次数"""
        async with RegCodeSessionFactory() as session:
            try:
                result = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
                reg_code = result.scalar_one_or_none()
                if reg_code:
                    reg_code.USE_COUNT += increment
                    await session.merge(reg_code)
                    await session.commit()
                    return True
                else:
                    logger.warning(f"未找到注册码: {code}")
                    return False
            except SQLAlchemyError as e:
                await session.rollback()
                logger.error(f"数据库操作失败: {e}")
                return False

    @staticmethod
    async def delete_regcode(code: str) -> bool:
        """删除指定的注册码"""
        async with RegCodeSessionFactory() as session:
            try:
                result = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
                reg_code = result.scalar_one_or_none()
                if reg_code:
                    await session.delete(reg_code)
                    await session.commit()
                    return True
                else:
                    logger.warning(f"未找到注册码: {code}")
                    return False
            except SQLAlchemyError as e:
                await session.rollback()
                logger.error(f"数据库操作失败: {e}")
                return False

    @staticmethod
    async def get_regcodes_by_uid(uid: int) -> List[RegCodeModel]:
        """根据UID获取所有注册码"""
        async with RegCodeSessionFactory() as session:
            # UID存储为字符串，需要模糊匹配
            result = await session.execute(
                select(RegCodeModel).filter(RegCodeModel.UID.contains(str(uid)))
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_active_regcodes_count() -> int:
        """获取活跃注册码数量（使用次数未达限制）"""
        async with RegCodeSessionFactory() as session:
            result = await session.execute(
                select(func.count()).select_from(RegCodeModel).where(
                    (RegCodeModel.USE_COUNT_LIMIT == -1) |
                    (RegCodeModel.USE_COUNT < RegCodeModel.USE_COUNT_LIMIT)
                )
            )
            return result.scalar_one()

    @staticmethod
    async def get_code_info(code: str) -> Optional[RegCodeModel]:
        """获取注册码详细信息"""
        return await RegCodeOperate.get_regcode_by_code(code)

    @staticmethod
    async def deactivate_regcode(code: str) -> bool:
        """停用注册码"""
        async with RegCodeSessionFactory() as session:
            try:
                result = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
                reg_code = result.scalar_one_or_none()
                if reg_code:
                    reg_code.ACTIVE = False
                    session.merge(reg_code)
                    await session.commit()
                    return True
                return False
            except SQLAlchemyError as e:
                await session.rollback()
                logger.error(f"数据库操作失败: {e}")
                return False
