from enum import Enum
import random
import time
import hashlib
from typing import Optional

from sqlalchemy import select, update, func, String, Integer, Boolean
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database
class UsersDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass
class Role(Enum):
    ADMIN = 0       # 管理员
    NORMAL = 1      # 普通注册用户
    WHITE_LIST = 2  # 白名单用户
    UNRECOGNIZED = -1  # 未注册用户


class UserModel(UsersDatabaseModel):
    __tablename__ = 'users'
    UID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True)
    TELEGRAM_ID: Mapped[Optional[int]] = mapped_column(Integer, index=True, nullable=True)
    USERNAME: Mapped[Optional[str]] = mapped_column(String, index=True, nullable=True)
    EMAIL: Mapped[Optional[str]] = mapped_column(String, index=True, nullable=True)
    ROLE: Mapped[int] = mapped_column(Integer, default=Role.UNRECOGNIZED.value, nullable=False)
    ACTIVE_STATUS: Mapped[Optional[bool]] = mapped_column(Boolean, default=True, nullable=True)
    CREATE_AT: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    REGISTER_TIME: Mapped[Optional[int]] = mapped_column(Integer, default=lambda: int(time.time()), nullable=True)
    EXPIRED_AT: Mapped[Optional[int]] = mapped_column(Integer, default=-1, nullable=True)
    EMBYID: Mapped[Optional[str]] = mapped_column(String, index=True, default='', nullable=True)
    PASSWORD: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    NSFW: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    BGM_MODE: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    BGM_TOKEN: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    LAST_LOGIN_TIME: Mapped[Optional[int]] = mapped_column(Integer, default=0, nullable=True)
    LAST_LOGIN_IP: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    LAST_LOGIN_UA: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    DEVICE_LIST: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    APIKEY_STATUS: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    APIKEY: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    OTHER: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)


create_database("users", UsersDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "users.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
UsersSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)


class UserOperate:
    @staticmethod
    async def get_new_uid() -> int:
        """生成一个新的UID"""
        async with UsersSessionFactory() as session:
            result = await session.execute(select(func.max(UserModel.UID)).limit(1))
            max_uid = result.scalar_one_or_none()
            return 1 if max_uid is None else max_uid + 1

    @staticmethod
    async def add_user(user: UserModel) -> None:
        """添加用户"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.add(user)

    @staticmethod
    async def get_user_by_uid(uid: int) -> Optional[UserModel]:
        """根据UID获取用户"""
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(UID=uid).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_user_by_telegram_id(telegram_id: int) -> Optional[UserModel]:
        """根据Telegram ID获取用户"""
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(TELEGRAM_ID=telegram_id).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_user_by_username(username: str) -> Optional[UserModel]:
        """根据Emby用户名获取用户"""
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(USERNAME=username).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_user_by_embyid(embyid: str) -> Optional[UserModel]:
        """根据Emby ID获取用户"""
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(EMBYID=embyid).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def update_user(user: UserModel) -> None:
        """更新用户信息"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.merge(user)

    @staticmethod
    async def delete_user(user: UserModel) -> None:
        """删除用户"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                # 需要先获取session中的对象才能删除
                existing = await session.execute(select(UserModel).filter_by(UID=user.UID))
                db_user = existing.scalar_one_or_none()
                if db_user:
                    await session.delete(db_user)

    @staticmethod
    async def unbind_telegram_user(user: UserModel) -> None:
        """将用户的Emby账号与Telegram解绑"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel).where(UserModel.UID == user.UID).values(TELEGRAM_ID=None)
                )

    @staticmethod
    async def renew_user_expire_time(user: UserModel, duration: int) -> None:
        """
        续期指定时长给指定用户
        :param user: 用户对象
        :param duration: 续期时长，单位为天
        """
        if user.EXPIRED_AT == -1:
            # 永不过期，无需续期
            return

        async with UsersSessionFactory() as session:
            async with session.begin():
                current_time = int(time.time())
                if user.EXPIRED_AT < current_time:
                    # 已过期，从当前时间开始续期
                    new_expired_at = current_time + duration * 86400
                else:
                    # 未过期，从原有过期时间开始续期
                    new_expired_at = user.EXPIRED_AT + duration * 86400
                await session.execute(
                    update(UserModel).where(UserModel.UID == user.UID).values(EXPIRED_AT=new_expired_at)
                )

    @staticmethod
    async def get_registered_users_count() -> int:
        """获取注册用户数量（排除未注册用户、白名单用户、管理员）"""
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(func.count()).select_from(UserModel).where(
                    UserModel.ROLE != Role.UNRECOGNIZED.value,
                    UserModel.ROLE != Role.WHITE_LIST.value,
                    UserModel.ROLE != Role.ADMIN.value
                )
            )
            return result.scalar_one()

    @staticmethod
    async def get_active_users_count() -> int:
        """获取活跃用户数量（排除未注册用户、白名单用户、管理员、过期用户）"""
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(func.count()).select_from(UserModel).where(
                    UserModel.ROLE != Role.UNRECOGNIZED.value,
                    UserModel.ROLE != Role.WHITE_LIST.value,
                    UserModel.ROLE != Role.ADMIN.value,
                    UserModel.ACTIVE_STATUS == True,
                    UserModel.EXPIRED_AT > int(time.time())
                )
            )
            return result.scalar_one()

    @staticmethod
    async def reset_apikey(usr: UserModel) -> str:
        """
        重置用户API Key
        格式为 key-xxxxxxxxxxxxxxxx-yyyyyyyy
        其中 xxxxxxxxxxxxxxxx 为16位随机字符串，yyyyyyyy 为8位校验码
        """
        random_part = hashlib.sha256(f'{usr.UID}_{int(time.time())}'.encode()).hexdigest()[:16]
        check_part = ''.join(random.choices('0123456789', k=8))
        new_apikey = f'key-{random_part}-{check_part}'

        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel).where(UserModel.UID == usr.UID).values(APIKEY=new_apikey)
                )
        return new_apikey
