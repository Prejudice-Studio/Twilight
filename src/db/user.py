from enum import Enum

from sqlalchemy import delete, select, update
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database

class UsersDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass

class Role(Enum):
    NORMAL = 1 # 普通注册用户
    ADMIN = 0 # 管理员
    WHITE_LIST = 2 # 白名单用户
    UNRECOGNIZED = -1 # 未注册用户
    
class UserModel(UsersDatabaseModel):
    __tablename__ = 'users'
    uid: Mapped[int] = mapped_column(primary_key=True , index=True) # 用户UID
    telegram_id: Mapped[int] = mapped_column(index=True , nullable=True) # 用户的Telegram ID
    username: Mapped[str] = mapped_column(index=True , nullable=False) # 用户的Emby用户名
    role: Mapped[int] = mapped_column(default=Role.UNRECOGNIZED.value, nullable=False) # 用户的角色
    score: Mapped[int] = mapped_column(default=0, nullable=False) # 用户的积分
    active_status: Mapped[bool] = mapped_column(default=True, nullable=False) # 用户是否启用
    created_at: Mapped[int] = mapped_column(default=None, nullable=False) # 用户创建时间 使用时间戳
    expired_at: Mapped[int] = mapped_column(default=None, nullable=False) # 用户到期时间 使用时间戳 如果为-1则永不过期
    embyid: Mapped[str] = mapped_column(index= True, default='', nullable=False) # 用户的Emby账户
    password: Mapped[str] = mapped_column(default='', nullable=False) # 用户的Emby密码hash
    NSFW: Mapped[bool] = mapped_column(default=False, nullable=False) # 用户是否开启NSFW库
    BGM_MODE: Mapped[bool] = mapped_column(default=False, nullable=False) # 用户是否开启BGM点格子模式
    BGM_TOKEN: Mapped[str] = mapped_column(default='', nullable=False) # 用户的BGM Token
    LAST_LOGIN_TIME: Mapped[int] = mapped_column(default=None, nullable=False) # 用户上次登录时间 使用时间戳
    LAST_LOGIN_IP: Mapped[str] = mapped_column(default='', nullable=False) # 用户上次登录IP
    LAST_LOGIN_UA: Mapped[str] = mapped_column(default='', nullable=False) # 用户上次登录UA
    DEVICE_LIST: Mapped[str] = mapped_column(default='', nullable=False) # 用户设备列表
    APIKEY: Mapped[str] = mapped_column(default='', nullable=False) # 用户API Key , 用于API访问认证
    OTHER_INFO: Mapped[str] = mapped_column(default='', nullable=False) # 用户其他信息 , 使用json存储
    
create_database("users", UsersDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "users.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
UsersSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)

class UserOperate:
    @classmethod
    async def add_user(cls, user: UserModel):
        """
        添加用户
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.add(user)
            await session.commit()
    
    @staticmethod        
    async def get_user_by_uid(self, uid: int) -> UserModel | None:
        """
        根据UID获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(uid=uid).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def get_user_by_telegram_id(self, telegram_id: int) -> UserModel | None:
        """
        根据Telegram ID获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(telegram_id=telegram_id).limit(1))
            return scalar.scalar_one_or_none()
        
    @staticmethod
    async def get_user_by_username(self, username: str) -> UserModel | None:
        """
        根据Emby用户名获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(username=username).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def get_user_by_embyid(self, embyid: str) -> UserModel | None:
        """
        根据Emby ID获取用户
        """
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(embyid=embyid).limit(1))
            return scalar.scalar_one_or_none()
    
    @staticmethod
    async def update_user(self, user: UserModel):
        """
        更新用户信息
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.merge(user)
            await session.commit()
    
    @staticmethod
    async def delete_user(self, user: UserModel):
        """
        删除用户
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.delete(user)
            await session.commit()
            
    @staticmethod
    async def unbind_telegram_user(self, user: UserModel):
        """
        将用户的Emby账号与Telegram解绑
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(update(UserModel).where(UserModel.uid == user.uid).values(telegram_id=None))
        
    @staticmethod
    async def renew_user_expire_time(self, user: UserModel, duration: int):
        """
        续期指定时长给指定用户
        duration: 续期时长，单位为天
        需要转换为时间戳
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                new_expired_at = user.expired_at
                if user.expired_at == -1:
                    # 永不过期
                    return
                else:
                    from time import time
                    current_time = int(time())
                    if user.expired_at < current_time:
                        # 已过期，从当前时间开始续期
                        new_expired_at = current_time + duration * 86400
                    else:
                        # 未过期，从原有过期时间开始续期
                        new_expired_at = user.expired_at + duration * 86400
                await session.execute(update(UserModel).where(UserModel.uid == user.uid).values(expired_at=new_expired_at))
                
    @staticmethod
    async def get_registered_users_count(self) -> int:
        """
        获取注册用户数量
        排除未注册用户 , 白名单用户 , 管理员
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                scalar = await session.execute(select(UserModel).filter(UserModel.role != Role.UNRECOGNIZED.value, UserModel.role != Role.WHITE_LIST.value, UserModel.role != Role.ADMIN.value).count())
                return scalar.scalar_one()
    
    @staticmethod
    async def get_active_users_count(self) -> int:
        """
        获取活跃用户数量
        排除未注册用户 , 白名单用户 , 管理员 , 过期用户
        """
        async with UsersSessionFactory() as session:
            async with session.begin():
                scalar = await session.execute(select(UserModel).filter(UserModel.role != Role.UNRECOGNIZED.value, UserModel.role != Role.WHITE_LIST.value, UserModel.role != Role.ADMIN.value, UserModel.active_status == True, UserModel.expired_at > 0).count())
                return scalar.scalar_one()