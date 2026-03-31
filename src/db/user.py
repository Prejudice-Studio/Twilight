from enum import Enum
import random
import time
import hashlib
from typing import Optional

from sqlalchemy import select, update, func, String, Integer, Boolean
from sqlalchemy.ext.asyncio import AsyncAttrs
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
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
    NSFW: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)  # 用户是否开启 NSFW 显示
    NSFW_ALLOWED: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)  # 管理员是否允许用户访问 NSFW
    BGM_MODE: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    BGM_TOKEN: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    LAST_LOGIN_TIME: Mapped[Optional[int]] = mapped_column(Integer, default=0, nullable=True)
    LAST_LOGIN_IP: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    LAST_LOGIN_UA: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    DEVICE_LIST: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    APIKEY_STATUS: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    APIKEY: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    APIKEY_PERMISSIONS: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)  # JSON: API Key 权限范围
    AUTO_RENEW: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)  # 自动续期开关
    AVATAR: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)  # 用户头像 URL
    OTHER: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)


from src.db.utils import init_async_db

ENGINE, UsersSessionFactory = init_async_db("users", UsersDatabaseModel)


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
    async def get_all_emby_users() -> list[UserModel]:
        """获取所有绑定了 Emby 的用户"""
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.EMBYID.isnot(None),
                    UserModel.EMBYID != '',
                )
            )
            return list(result.scalars().all())
    
    @staticmethod
    async def get_user_by_emby_username(username: str) -> Optional[UserModel]:
        """根据 Emby/Jellyfin 用户名获取用户（与 get_user_by_username 相同）"""
        return await UserOperate.get_user_by_username(username)

    @staticmethod
    async def update_user(user: UserModel) -> None:
        """更新用户信息"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.merge(user)

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
        重置用户API Key (加密安全)
        格式为 key-xxxxxxxxxxxxxxxx-yyyyyyyy
        其中 xxxxxxxxxxxxxxxx 为16位随机字符串，yyyyyyyy 为8位数字校验码
        """
        import secrets
        random_part = secrets.token_hex(8)  # 16 字符
        check_part = ''.join(secrets.choice('0123456789') for _ in range(8))
        new_apikey = f'key-{random_part}-{check_part}'

        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel).where(UserModel.UID == usr.UID).values(
                        APIKEY=new_apikey,
                        APIKEY_STATUS=True
                    )
                )
        return new_apikey

    @staticmethod
    async def get_user_by_apikey(apikey: str) -> Optional[UserModel]:
        """根据 API Key 获取用户"""
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(UserModel).filter_by(APIKEY=apikey, APIKEY_STATUS=True).limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def set_apikey_status(uid: int, enabled: bool) -> bool:
        """设置 API Key 状态"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel).where(UserModel.UID == uid).values(APIKEY_STATUS=enabled)
                )
                return True

    @staticmethod
    async def update_login_info(uid: int, ip: str = '', ua: str = '') -> None:
        """更新用户登录信息"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel).where(UserModel.UID == uid).values(
                        LAST_LOGIN_TIME=int(time.time()),
                        LAST_LOGIN_IP=ip,
                        LAST_LOGIN_UA=ua
                    )
                )

    @staticmethod
    async def get_expired_users() -> list[UserModel]:
        """
        获取所有已过期但仍处于启用状态的用户
        排除永不过期(-1)的用户
        """
        current_time = int(time.time())
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.EXPIRED_AT != -1,  # 排除永不过期
                    UserModel.EXPIRED_AT < current_time,  # 已过期
                    UserModel.ACTIVE_STATUS == True,  # 仍然启用
                    UserModel.EMBYID != '',  # 有 Emby 账户
                    UserModel.EMBYID.isnot(None),
                )
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_expiring_users(days: int = 3) -> list[UserModel]:
        """
        获取即将过期的用户（用于提醒通知）
        
        :param days: 几天内过期
        """
        current_time = int(time.time())
        expire_threshold = current_time + days * 86400
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.EXPIRED_AT != -1,
                    UserModel.EXPIRED_AT > current_time,  # 还未过期
                    UserModel.EXPIRED_AT <= expire_threshold,  # 但即将过期
                    UserModel.ACTIVE_STATUS == True,
                )
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_all_users(
        include_inactive: bool = False,
        role: Optional[int] = None,
        limit: int = 100,
        offset: int = 0
    ) -> tuple[list[UserModel], int]:
        """
        分页获取用户列表
        
        :return: (用户列表, 总数)
        """
        async with UsersSessionFactory() as session:
            # 构建查询条件
            conditions = []
            if not include_inactive:
                conditions.append(UserModel.ACTIVE_STATUS == True)
            if role is not None:
                conditions.append(UserModel.ROLE == role)
            
            # 查询总数
            count_query = select(func.count()).select_from(UserModel)
            if conditions:
                count_query = count_query.where(*conditions)
            total_result = await session.execute(count_query)
            total = total_result.scalar_one()
            
            # 查询用户
            query = select(UserModel).order_by(UserModel.UID.desc()).limit(limit).offset(offset)
            if conditions:
                query = query.where(*conditions)
            result = await session.execute(query)
            
            return list(result.scalars().all()), total

    @staticmethod
    async def batch_disable_users(uids: list[int]) -> int:
        """批量禁用用户"""
        if not uids:
            return 0
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    update(UserModel).where(UserModel.UID.in_(uids)).values(ACTIVE_STATUS=False)
                )
                return result.rowcount
