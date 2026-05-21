from enum import Enum
import time
import hashlib
import random
import logging
import json
import secrets
import string
import uuid
from typing import Optional, List, Union

from sqlalchemy import select, func, String, Integer, Boolean, update
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from sqlalchemy.exc import SQLAlchemyError

from src.config import Config, RegisterConfig
from src.db.utils import create_database, init_async_db

logger = logging.getLogger(__name__)


class Type(Enum):
    REGISTER = 1  # 注册
    RENEW = 2  # 续期
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


ENGINE, RegCodeSessionFactory = init_async_db("regcode", RegCodeDatabaseModel)


class RegCodeOperate:
    @staticmethod
    def _random_part(algorithm: Optional[str] = None) -> str:
        algo = (algorithm or RegisterConfig.REGCODE_RANDOM_ALGORITHM or "base32-20").strip().lower()
        base32_alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
        alnum_alphabet = string.ascii_uppercase + string.digits
        urlsafe_alphabet = string.ascii_letters + string.digits + "-_"
        if algo == "hex32":
            return secrets.token_hex(16)
        if algo == "base32-24":
            return "".join(secrets.choice(base32_alphabet) for _ in range(24))
        if algo == "base32-20":
            return "".join(secrets.choice(base32_alphabet) for _ in range(20))
        if algo == "base32-16":
            return "".join(secrets.choice(base32_alphabet) for _ in range(16))
        if algo == "alnum-24":
            return "".join(secrets.choice(alnum_alphabet) for _ in range(24))
        if algo == "alnum-16":
            return "".join(secrets.choice(alnum_alphabet) for _ in range(16))
        if algo == "urlsafe-24":
            return "".join(secrets.choice(urlsafe_alphabet) for _ in range(24))
        if algo == "digits-16":
            return "".join(secrets.choice(string.digits) for _ in range(16))
        if algo == "digits-12":
            return "".join(secrets.choice(string.digits) for _ in range(12))
        if algo == "uuid":
            return str(uuid.uuid4())
        if algo == "legacy-sha1":
            unique_part = f"{random.randint(10000, 99999)}-{time.time()}-{secrets.token_hex(8)}"
            return hashlib.sha1(unique_part.encode()).hexdigest()[:20]
        return "".join(secrets.choice(base32_alphabet) for _ in range(20))

    @staticmethod
    def _type_label(type_: int) -> str:
        return {Type.REGISTER.value: "register", Type.RENEW.value: "renew", Type.WHITELIST.value: "whitelist"}.get(int(type_ or 0), "code")

    @staticmethod
    def _generate_code(vali_time: int, use_count_limit: int, day: int, type_: int = 1, index: int = 1, *, code_format: Optional[str] = None, random_algorithm: Optional[str] = None) -> str:
        """按配置格式生成卡码。支持 {random}/{type}/{days}/{index}。"""
        fmt = (code_format if code_format is not None else RegisterConfig.REGCODE_FORMAT) or "code-{random}"
        fmt = str(fmt).strip()[:128] or "code-{random}"
        if "{random}" not in fmt:
            fmt = f"{fmt}-{{random}}"
        code = fmt.format(
            random=RegCodeOperate._random_part(random_algorithm),
            type=RegCodeOperate._type_label(type_),
            days=day,
            index=index,
            validity=vali_time,
            limit=use_count_limit,
        )
        code = "".join(ch for ch in code if ch.isalnum() or ch in "._:-").strip("._:-")
        return code[:96] or ("code-" + secrets.token_hex(10))

    @staticmethod
    async def create_regcode(
        vali_time: int,
        type_: int,
        use_count_limit: int = 1,
        count: int = 1,
        day: int = 30,
        *,
        code_format: Optional[str] = None,
        random_algorithm: Optional[str] = None,
        decoy: bool = False,
    ) -> Union[str, List[str]]:
        """创建指定数量的注册码并添加到数据库中"""
        try:
            parsed_day = int(day)
        except (TypeError, ValueError):
            parsed_day = 30
        day = -1 if parsed_day <= 0 else parsed_day
        codes = []
        async with RegCodeSessionFactory() as session:
            try:
                generated_in_batch: set[str] = set()
                for index in range(1, count + 1):
                    code = RegCodeOperate._generate_code(vali_time, use_count_limit, day, type_, index, code_format=code_format, random_algorithm=random_algorithm)
                    for _retry in range(8):
                        exists = await session.execute(select(RegCodeModel.CODE).where(RegCodeModel.CODE == code).limit(1))
                        if exists.scalar_one_or_none() is None and code not in generated_in_batch:
                            break
                        code = RegCodeOperate._generate_code(vali_time, use_count_limit, day, type_, index, code_format=code_format, random_algorithm=random_algorithm)
                    else:
                        raise ValueError("生成卡码重复次数过多，请调整格式或随机算法")
                    generated_in_batch.add(code)
                    other = {"decoy": True} if decoy else None
                    reg_code = RegCodeModel(
                        CODE=code,
                        VALIDITY_TIME=vali_time,
                        TYPE=type_,
                        USE_COUNT_LIMIT=use_count_limit,
                        DAYS=day,
                        OTHER=json.dumps(other, ensure_ascii=False) if other else None,
                    )
                    session.add(reg_code)
                    codes.append(code)
                await session.commit()
            except (SQLAlchemyError, ValueError) as e:
                await session.rollback()
                logger.error(f"数据库操作失败: {e}")
                raise

        return codes[0] if len(codes) == 1 else codes

    @staticmethod
    def _other_dict(reg_code: RegCodeModel) -> dict:
        if not reg_code.OTHER:
            return {}
        try:
            data = json.loads(reg_code.OTHER)
            return data if isinstance(data, dict) else {}
        except (json.JSONDecodeError, TypeError):
            return {}

    @staticmethod
    def is_decoy(reg_code: RegCodeModel) -> bool:
        return bool(RegCodeOperate._other_dict(reg_code).get("decoy"))

    @staticmethod
    def get_note(reg_code: RegCodeModel) -> str:
        data = RegCodeOperate._other_dict(reg_code)
        return data.get("note", "") if isinstance(data, dict) else ""

    @staticmethod
    async def update_note(code: str, note: str) -> bool:
        note = (note or "").strip()
        if len(note) > 120:
            note = note[:120]
        async with RegCodeSessionFactory() as session:
            try:
                result = await session.execute(select(RegCodeModel).filter_by(CODE=code).limit(1))
                reg_code = result.scalar_one_or_none()
                if not reg_code:
                    return False
                data = {}
                if reg_code.OTHER:
                    try:
                        parsed = json.loads(reg_code.OTHER)
                        if isinstance(parsed, dict):
                            data = parsed
                    except (json.JSONDecodeError, TypeError):
                        data = {}
                if note:
                    data["note"] = note
                else:
                    data.pop("note", None)
                reg_code.OTHER = json.dumps(data, ensure_ascii=False) if data else None
                await session.commit()
                return True
            except SQLAlchemyError as e:
                await session.rollback()
                logger.error(f"数据库操作失败: {e}")
                return False

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
        """更新注册码的使用次数。"""
        async with RegCodeSessionFactory() as session:
            try:
                stmt = update(RegCodeModel).where(RegCodeModel.CODE == code)
                if increment > 0:
                    stmt = stmt.where(
                        (RegCodeModel.USE_COUNT_LIMIT == -1) | (RegCodeModel.USE_COUNT < RegCodeModel.USE_COUNT_LIMIT)
                    )
                stmt = stmt.values(USE_COUNT=RegCodeModel.USE_COUNT + increment)
                result = await session.execute(stmt)
                if result.rowcount == 0:
                    logger.warning(f"未找到或已超出使用次数的注册码: {code}")
                    await session.rollback()
                    return False
                await session.commit()
                return True
            except SQLAlchemyError as e:
                await session.rollback()
                logger.error(f"数据库操作失败: {e}")
                return False

    @staticmethod
    def _append_csv_value(current: Optional[str], value: Optional[Union[int, str]]) -> Optional[str]:
        if value is None or value == "":
            return current
        value_str = str(value)
        parts = [p.strip() for p in (current or "").split(",") if p.strip()]
        if value_str not in parts:
            parts.append(value_str)
        return ",".join(parts) if parts else None

    @staticmethod
    async def record_regcode_use(
        code: str,
        *,
        uid: Optional[Union[int, str]] = None,
        telegram_id: Optional[Union[int, str]] = None,
        increment: int = 1,
    ) -> bool:
        """原子记录卡码使用次数与使用人，供管理员追踪卡码去向。"""
        async with RegCodeSessionFactory() as session:
            try:
                if increment:
                    stmt = update(RegCodeModel).where(RegCodeModel.CODE == code)
                    if increment > 0:
                        stmt = stmt.where(
                            (RegCodeModel.USE_COUNT_LIMIT == -1)
                            | (RegCodeModel.USE_COUNT < RegCodeModel.USE_COUNT_LIMIT)
                        )
                    stmt = stmt.values(USE_COUNT=RegCodeModel.USE_COUNT + increment)
                    updated = await session.execute(stmt)
                    if updated.rowcount == 0:
                        logger.warning(f"未找到或已超出使用次数的注册码: {code}")
                        await session.rollback()
                        return False

                result = await session.execute(select(RegCodeModel).where(RegCodeModel.CODE == code).limit(1))
                reg_code = result.scalar_one_or_none()
                if not reg_code:
                    await session.rollback()
                    logger.warning(f"未找到注册码: {code}")
                    return False
                reg_code.UID = RegCodeOperate._append_csv_value(reg_code.UID, uid)
                reg_code.TELEGRAM_ID = RegCodeOperate._append_csv_value(reg_code.TELEGRAM_ID, telegram_id)
                await session.commit()
                return True
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
            result = await session.execute(select(RegCodeModel).filter(RegCodeModel.UID.contains(str(uid))))
            return list(result.scalars().all())

    @staticmethod
    async def get_active_regcodes_count() -> int:
        """获取活跃注册码数量（使用次数未达限制）"""
        async with RegCodeSessionFactory() as session:
            result = await session.execute(
                select(func.count())
                .select_from(RegCodeModel)
                .where((RegCodeModel.USE_COUNT_LIMIT == -1) | (RegCodeModel.USE_COUNT < RegCodeModel.USE_COUNT_LIMIT))
            )
            return result.scalar_one()

    @staticmethod
    async def get_regcode_stats() -> dict:
        """获取注册码统计数据（总数和启用数，仅在数据库层面计数）"""
        async with RegCodeSessionFactory() as session:
            total_result = await session.execute(select(func.count()).select_from(RegCodeModel))
            active_result = await session.execute(
                select(func.count()).select_from(RegCodeModel).where(RegCodeModel.ACTIVE == True)
            )
            return {
                "total": total_result.scalar_one(),
                "active": active_result.scalar_one(),
            }

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
