from enum import Enum
import random
import time
import hashlib
from sqlalchemy import delete, select, update, func , String , Integer, Boolean
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database

class UsersDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass

class Role(Enum):
    NORMAL = 1  # 普通注册用户
    ADMIN = 0  # 管理员
    WHITE_LIST = 2  # 白名单用户
    UNRECOGNIZED = -1  # 未注册用户
    
class UserModel(UsersDatabaseModel):
    __tablename__ = 'users'
    UID: Mapped[int] = mapped_column(Integer , primary_key=True, index=True)                                               # 用户UID
    TELEGRAM_ID: Mapped[int] = mapped_column(Integer , index=True, nullable=True)                                          # 用户的Telegram ID
    USERNAME: Mapped[str] = mapped_column(String , index=True, nullable=True)                                             # 用户的Emby用户名
    EMAIL: Mapped[str] = mapped_column(String , index=True, nullable=True)                                                 # 用户的邮箱
    ROLE: Mapped[int] = mapped_column(Integer , default=Role.UNRECOGNIZED.value, nullable=False)                          # 用户的角色
    ACTIVE_STATUS: Mapped[bool] = mapped_column(Boolean , default=True, nullable=True)                                    # 用户是否启用
    CREATE_AT: Mapped[int] = mapped_column(Integer , nullable=True)                                                       # 用户Emby注册时间    
    REGISTER_TIME: Mapped[int] = mapped_column(Integer , default=int(time.time()), nullable=True)                         # 用户创建时间
    EXPIRED_AT: Mapped[int] = mapped_column(Integer , default=-1, nullable=True)                                          # 用户过期时间，时间戳，-1表示永不过期
    EMBYID: Mapped[str] = mapped_column(String , index=True, default='', nullable=True)                                  # 用户的Emby账户ID
    PASSWORD: Mapped[str] = mapped_column(String , default='', nullable=True)                                            # 用户的Emby密码hash
    NSFW: Mapped[bool] = mapped_column(Boolean , default=False, nullable=True)                                            # 用户是否开启NSFW库
    BGM_MODE: Mapped[bool] = mapped_column(Boolean , default=False, nullable=True)                                        # 用户是否开启BGM点格子模式
    BGM_TOKEN: Mapped[str] = mapped_column(String , default='', nullable=True)                                           # 用户的BGM Token
    LAST_LOGIN_TIME: Mapped[int] = mapped_column(Integer , default=0, nullable=True)                                      # 用户上次登录时间，时间戳
    LAST_LOGIN_IP: Mapped[str] = mapped_column(String , default='', nullable=True)                                       # 用户上次登录IP
    LAST_LOGIN_UA: Mapped[str] = mapped_column(String , default='', nullable=True)                                       # 用户上次登录UA
    DEVICE_LIST: Mapped[str] = mapped_column(String , default='', nullable=True)                                         # 用户设备列表
    APIKEY_STATUS: Mapped[bool] = mapped_column(Boolean , default=False, nullable=True)                                   # 用户API Key是否启用
    APIKEY: Mapped[str] = mapped_column(String , default='', nullable=True)                                              # 用户API Key , 用于API访问认证
    OTHER: Mapped[str] = mapped_column(String , default='', nullable=True)                                          # 用户其他信息 , 使用json存储
    
create_database("users", UsersDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "users.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
UsersSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)

class UserOperate:
    @classmethod
    async def get_new_uid(cls) -> int:
        """
        生成一个新的UID
        """
        async with UsersSessionFactory() as session:
            result = await session.execute(select(func.max(UserModel.UID)).limit(1))
            max_uid = result.scalar_one_or_none()
            if max_uid is None:
                return 1
            else:
                return max_uid + 1
    
    @classmethod
    async def add_user(user: UserModel):
        """
        添加用户
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.add(user)
            await session.commit()
    
    @staticmethod        
    async def get_user_by_uid(uid: int) -> UserModel | None:
        """
        根据UID获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(UID=uid).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def get_user_by_telegram_id(telegram_id: int) -> UserModel | None:
        """
        根据Telegram ID获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(TELEGRAM_ID=telegram_id).limit(1))
            return scalar.scalar_one_or_none()
        
    @staticmethod
    async def get_user_by_username(username: str) -> UserModel | None:
        """
        根据Emby用户名获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(USERNAME=username).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def get_user_by_embyid(embyid: str) -> UserModel | None:
        """
        根据Emby ID获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(EMBYID=embyid).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def update_user(user: UserModel):
        """
        更新用户信息
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.merge(user)
            await session.commit()
    
    @staticmethod
    async def delete_user(user: UserModel):
        """
        删除用户
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.delete(user)
            await session.commit()
            
    @staticmethod
    async def unbind_telegram_user(user: UserModel):
        """
        将用户的Emby账号与Telegram解绑
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(update(UserModel).where(UserModel.UID == user.UID).values(TELEGRAM_ID=None))
        
    @staticmethod
    async def renew_user_expire_time(user: UserModel, duration: int):
        """
        续期指定时长给指定用户
        duration: 续期时长，单位为天
        需要转换为时间戳
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                new_expired_at = user.EXPIRED_AT
                if user.EXPIRED_AT == -1:
                    # 永不过期
                    return
                else:
                    current_time = int(time.time())
                    if user.EXPIRED_AT < current_time:
                        # 已过期，从当前时间开始续期
                        new_expired_at = current_time + duration * 86400
                    else:
                        # 未过期，从原有过期时间开始续期
                        new_expired_at = user.EXPIRED_AT + duration * 86400
                await session.execute(update(UserModel).where(UserModel.UID == user.UID).values(EXPIRED_AT=new_expired_at))
                
    @staticmethod
    async def get_registered_users_count() -> int:
        """
        获取注册用户数量
        排除未注册用户 , 白名单用户 , 管理员
        """
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
        """
        获取活跃用户数量
        排除未注册用户 , 白名单用户 , 管理员 , 过期用户
        """
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
        使用特殊算法生成唯一的apikey
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                # 根据当前时间和UID生成唯一API Key
                # apikey的格式为 key-xxxxxxxxxxxxxxxx-yyyyyyyy
                # 其中 xxxxxxxxxxxxxxxx 为16位随机字符串,由当前时间决定，yyyyyyyy 为8位检验码
                # 其中 x 由数字和字母组成，yyyyyyyy 由数字组成
                random_part = hashlib.sha256(f'{usr.UID}_{int(time.time())}'.encode()).hexdigest()[:16]
                check_part = ''.join(random.choices('0123456789', k=8))
                usr.APIKEY = f'key-{random_part}-{check_part}'
                await session.execute(update(UserModel).where(UserModel.UID == usr.UID).values(APIKEY=usr.APIKEY))
                return usr.APIKEY