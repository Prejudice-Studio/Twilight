from enum import Enum
import time
import hashlib
import random
from sqlalchemy import insert , select , func
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from sqlalchemy.exc import SQLAlchemyError
from src.config import Config
from src.db import create_database

class Type(Enum):
    REGISTER = 1  # 注册
    RENEW = 2     # 续期
    WHITELIST = 3 # 白名单

class RegCodeDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass

class RegCodeModel(RegCodeDatabaseModel):
    __tablename__ = "regcode"
    CODE: Mapped[str] = mapped_column(primary_key=True, index=True, nullable=False)             # 注册码
    VALIDITY_TIME: Mapped[int] = mapped_column(default=-1, nullable=False)                      # 有效时间 单位为小时 . 默认为-1(永久)
    TYPE: Mapped[int] = mapped_column(nullable=False)                                           # 类型 1:注册 2:续期 3:白名单
    UID: Mapped[int | list[int] | None] = mapped_column(nullable=True)                          # 使用用户UID/UID列表 , 因为生成时候不一定使用可以为空
    TELEGRAM_ID: Mapped[int | None] = mapped_column(nullable=True)                              # 注册码使用者对应的telegram_id
    USE_COUNT_LIMIT: Mapped[int] = mapped_column(default=1, nullable=False)                     # 注册码使用次数限制 , 默认为1次(-1为无限制),其余为正常数字
    USE_COUNT: Mapped[int] = mapped_column(default=0, nullable=False)                           # 注册码已被使用次数 , 默认为0
    CREATED_TIME: Mapped[int] = mapped_column(default=lambda: int(time.time()), nullable=False) # 注册码创建时间
    DAYS: Mapped[int] = mapped_column(default=30, nullable=True)                                # 注册码/续期码增加的天数 , 默认为30天

create_database("RegCode", RegCodeDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "regcode.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
RegCodeSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)

class RegCodeOperate:
    @staticmethod
    async def create_regcode(vali_time: int, type: int, use_count_limit: int = 1, count: int = 1, day: int = 30) -> str | list[str]:
        """
        创建指定数量的注册码并添加到数据库中
        """
        codes = []
        async with RegCodeSessionFactory() as session:
            for _ in range(count):
                code = RegCodeOperate._generate_code(vali_time, use_count_limit, day)
                reg_code = RegCodeModel(CODE=code, VALIDITY_TIME=vali_time, TYPE=type, USE_COUNT_LIMIT=use_count_limit, DAYS=day)
                try:
                    session.add(reg_code)
                    await session.commit()
                    codes.append(code)
                except SQLAlchemyError as e:
                    await session.rollback()
                    print(f"数据库操作失败: {e}")
                    return f"错误: {e}"
        return codes if len(codes) > 1 else codes[0]
    
    @staticmethod
    def _generate_code(vali_time: int, use_count_limit: int, day: int) -> str:
        """
        生成唯一的注册码
        """
        unique_part = f"{vali_time}-{use_count_limit}-{day}-{random.randint(10000, 99999)}"
        return "code-" + hashlib.sha1(unique_part.encode()).hexdigest()[:20]  # 仅取前20个字符以适应code-xxxx-yyyy-zzzz格式
    
    @staticmethod
    async def get_regcode_by_code(code: str) -> RegCodeModel | None:
        """
        根据注册码获取注册码信息
        """
        async with RegCodeSessionFactory() as session:
            scalar = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
            return scalar.scalar_one_or_none()
        
    @staticmethod
    async def get_regcodes_by_type(type: int) -> list[RegCodeModel]:
        """
        根据类型获取所有注册码
        """
        async with RegCodeSessionFactory() as session:
            result = await session.execute(select(RegCodeModel).filter_by(TYPE=type))
            return result.scalars().all()
        
    @staticmethod
    async def update_regcode_use_count(code: str, increment: int = -1):
        """
        更新注册码的使用次数
        """
        async with RegCodeSessionFactory() as session:
            try:
                reg_code = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
                reg_code = reg_code.scalar_one_or_none()
                if reg_code:
                    reg_code.USE_COUNT += increment
                    session.merge(reg_code)
                    await session.commit()
                else:
                    print(f"未找到注册码: {code}")
            except SQLAlchemyError as e:
                await session.rollback()
                print(f"数据库操作失败: {e}")
                return f"错误: {e}"
            
    @staticmethod
    async def delete_regcode(code: str):
        """
        删除指定的注册码
        """
        async with RegCodeSessionFactory() as session:
            try:
                reg_code = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
                reg_code = reg_code.scalar_one_or_none()
                if reg_code:
                    session.delete(reg_code)
                    await session.commit()
                else:
                    print(f"未找到注册码: {code}")
            except SQLAlchemyError as e:
                await session.rollback()
                print(f"数据库操作失败: {e}")
                return f"错误: {e}"
            
    @staticmethod
    async def get_regcodes_by_uid(uid: int) -> list[RegCodeModel]:
        """
        根据UID获取所有注册码
        """
        async with RegCodeSessionFactory() as session:
            result = await session.execute(select(RegCodeModel).filter(RegCodeModel.UID == uid))
            return result.scalars().all()
        
    @staticmethod
    async def get_active_regcodes_count() -> int:
        """
        获取活跃注册码数量
        排除已被使用的注册码，如果使用次数达到限制则视为不活跃
        """
        async with RegCodeSessionFactory() as session:
            result = await session.execute(
                select(func.count()).select_from(RegCodeModel).where(
                    RegCodeModel.USE_COUNT < RegCodeModel.USE_COUNT_LIMIT
                )
            )
            return result.scalar_one()
        
    @staticmethod
    async def get_code_info(code: str) -> RegCodeModel | None:
        """
        获取注册码详细信息
        """
        async with RegCodeSessionFactory() as session:
            scalar = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
            return scalar.scalar_one_or_none()