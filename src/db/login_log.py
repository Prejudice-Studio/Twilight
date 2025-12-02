"""
登录日志数据库模块

记录用户登录历史、设备信息、IP 地址等
"""
import time
from typing import Optional, List
from sqlalchemy import select, func, String, Integer, Boolean, desc, and_
from sqlalchemy.ext.asyncio import AsyncAttrs, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config
from src.db import create_database


class LoginLogDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class LoginLogModel(LoginLogDatabaseModel):
    """登录日志"""
    __tablename__ = "login_log"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    EMBY_USER_ID: Mapped[str] = mapped_column(String, index=True, nullable=True)
    IP_ADDRESS: Mapped[str] = mapped_column(String, index=True, nullable=False)
    DEVICE_ID: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    DEVICE_NAME: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    CLIENT: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 客户端类型
    CLIENT_VERSION: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    LOGIN_TIME: Mapped[int] = mapped_column(Integer, nullable=False)
    COUNTRY: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 国家
    CITY: Mapped[Optional[str]] = mapped_column(String, nullable=True)  # 城市
    IS_BLOCKED: Mapped[bool] = mapped_column(Boolean, default=False)  # 是否被拦截


class UserDeviceModel(LoginLogDatabaseModel):
    """用户设备记录"""
    __tablename__ = "user_device"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    DEVICE_ID: Mapped[str] = mapped_column(String, index=True, nullable=False)
    DEVICE_NAME: Mapped[str] = mapped_column(String, nullable=True)
    CLIENT: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    FIRST_SEEN: Mapped[int] = mapped_column(Integer, nullable=False)  # 首次使用时间
    LAST_SEEN: Mapped[int] = mapped_column(Integer, nullable=False)  # 最后使用时间
    IS_TRUSTED: Mapped[bool] = mapped_column(Boolean, default=False)  # 是否信任设备
    IS_BLOCKED: Mapped[bool] = mapped_column(Boolean, default=False)  # 是否禁止


class IPWhitelistModel(LoginLogDatabaseModel):
    """IP 白名单"""
    __tablename__ = "ip_whitelist"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    IP_ADDRESS: Mapped[str] = mapped_column(String, nullable=False)
    CREATED_AT: Mapped[int] = mapped_column(Integer, nullable=False)
    NOTE: Mapped[Optional[str]] = mapped_column(String, nullable=True)


class IPBlacklistModel(LoginLogDatabaseModel):
    """IP 黑名单（全局）"""
    __tablename__ = "ip_blacklist"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True, autoincrement=True)
    IP_ADDRESS: Mapped[str] = mapped_column(String, unique=True, nullable=False)
    REASON: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    CREATED_AT: Mapped[int] = mapped_column(Integer, nullable=False)
    EXPIRE_AT: Mapped[int] = mapped_column(Integer, default=-1)  # -1 = 永久


create_database("login_log", LoginLogDatabaseModel)
DATABASE_URL = f'sqlite+aiosqlite:///{Config.DATABASES_DIR / "login_log.db"}'
ENGINE = create_async_engine(DATABASE_URL, echo=Config.SQLALCHEMY_LOG)
LoginLogSessionFactory = async_sessionmaker(bind=ENGINE, expire_on_commit=False)


class LoginLogOperate:
    """登录日志操作"""
    
    @staticmethod
    async def add_log(log: LoginLogModel) -> None:
        """添加登录日志"""
        async with LoginLogSessionFactory() as session:
            async with session.begin():
                session.add(log)
    
    @staticmethod
    async def get_user_logs(uid: int, limit: int = 50) -> List[LoginLogModel]:
        """获取用户登录日志"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(LoginLogModel)
                .filter_by(UID=uid)
                .order_by(desc(LoginLogModel.LOGIN_TIME))
                .limit(limit)
            )
            return list(result.scalars().all())
    
    @staticmethod
    async def get_user_unique_ips(uid: int, days: int = 30) -> List[str]:
        """获取用户在指定天数内使用的不重复 IP"""
        async with LoginLogSessionFactory() as session:
            since = int(time.time()) - days * 86400
            result = await session.execute(
                select(LoginLogModel.IP_ADDRESS)
                .filter(LoginLogModel.UID == uid, LoginLogModel.LOGIN_TIME >= since)
                .distinct()
            )
            return [row[0] for row in result.all()]
    
    @staticmethod
    async def get_user_ip_count(uid: int, days: int = 30) -> int:
        """获取用户 IP 数量"""
        ips = await LoginLogOperate.get_user_unique_ips(uid, days)
        return len(ips)
    
    @staticmethod
    async def get_recent_logs(limit: int = 100) -> List[LoginLogModel]:
        """获取最近的登录日志"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(LoginLogModel)
                .order_by(desc(LoginLogModel.LOGIN_TIME))
                .limit(limit)
            )
            return list(result.scalars().all())
    
    @staticmethod
    async def get_suspicious_logins(hours: int = 24) -> List[LoginLogModel]:
        """获取可疑登录（被拦截的）"""
        async with LoginLogSessionFactory() as session:
            since = int(time.time()) - hours * 3600
            result = await session.execute(
                select(LoginLogModel)
                .filter(LoginLogModel.LOGIN_TIME >= since, LoginLogModel.IS_BLOCKED == True)
                .order_by(desc(LoginLogModel.LOGIN_TIME))
            )
            return list(result.scalars().all())


class UserDeviceOperate:
    """用户设备操作"""
    
    @staticmethod
    async def add_or_update_device(uid: int, device_id: str, device_name: str = None, client: str = None) -> UserDeviceModel:
        """添加或更新设备"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(UserDeviceModel).filter_by(UID=uid, DEVICE_ID=device_id)
            )
            device = result.scalar_one_or_none()
            
            now = int(time.time())
            
            if device:
                device.LAST_SEEN = now
                if device_name:
                    device.DEVICE_NAME = device_name
                if client:
                    device.CLIENT = client
                async with session.begin():
                    await session.merge(device)
            else:
                device = UserDeviceModel(
                    UID=uid,
                    DEVICE_ID=device_id,
                    DEVICE_NAME=device_name or "未知设备",
                    CLIENT=client,
                    FIRST_SEEN=now,
                    LAST_SEEN=now,
                )
                async with session.begin():
                    session.add(device)
            
            return device
    
    @staticmethod
    async def get_user_devices(uid: int) -> List[UserDeviceModel]:
        """获取用户所有设备"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(UserDeviceModel)
                .filter_by(UID=uid, IS_BLOCKED=False)
                .order_by(desc(UserDeviceModel.LAST_SEEN))
            )
            return list(result.scalars().all())
    
    @staticmethod
    async def get_user_device_count(uid: int) -> int:
        """获取用户设备数量"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(func.count())
                .select_from(UserDeviceModel)
                .filter_by(UID=uid, IS_BLOCKED=False)
            )
            return result.scalar_one()
    
    @staticmethod
    async def block_device(uid: int, device_id: str) -> bool:
        """封禁设备"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(UserDeviceModel).filter_by(UID=uid, DEVICE_ID=device_id)
            )
            device = result.scalar_one_or_none()
            if device:
                device.IS_BLOCKED = True
                async with session.begin():
                    await session.merge(device)
                return True
            return False
    
    @staticmethod
    async def trust_device(uid: int, device_id: str) -> bool:
        """信任设备"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(UserDeviceModel).filter_by(UID=uid, DEVICE_ID=device_id)
            )
            device = result.scalar_one_or_none()
            if device:
                device.IS_TRUSTED = True
                async with session.begin():
                    await session.merge(device)
                return True
            return False


class IPListOperate:
    """IP 黑白名单操作"""
    
    @staticmethod
    async def add_to_whitelist(uid: int, ip: str, note: str = None) -> None:
        """添加 IP 到用户白名单"""
        async with LoginLogSessionFactory() as session:
            async with session.begin():
                session.add(IPWhitelistModel(
                    UID=uid,
                    IP_ADDRESS=ip,
                    CREATED_AT=int(time.time()),
                    NOTE=note
                ))
    
    @staticmethod
    async def get_user_whitelist(uid: int) -> List[str]:
        """获取用户 IP 白名单"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(IPWhitelistModel.IP_ADDRESS).filter_by(UID=uid)
            )
            return [row[0] for row in result.all()]
    
    @staticmethod
    async def is_ip_whitelisted(uid: int, ip: str) -> bool:
        """检查 IP 是否在用户白名单中"""
        whitelist = await IPListOperate.get_user_whitelist(uid)
        return ip in whitelist
    
    @staticmethod
    async def add_to_blacklist(ip: str, reason: str = None, expire_hours: int = -1) -> None:
        """添加 IP 到全局黑名单"""
        async with LoginLogSessionFactory() as session:
            now = int(time.time())
            expire_at = now + expire_hours * 3600 if expire_hours > 0 else -1
            async with session.begin():
                session.add(IPBlacklistModel(
                    IP_ADDRESS=ip,
                    REASON=reason,
                    CREATED_AT=now,
                    EXPIRE_AT=expire_at
                ))
    
    @staticmethod
    async def is_ip_blacklisted(ip: str) -> bool:
        """检查 IP 是否在黑名单中"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(IPBlacklistModel).filter_by(IP_ADDRESS=ip)
            )
            record = result.scalar_one_or_none()
            if not record:
                return False
            # 检查是否过期
            if record.EXPIRE_AT != -1 and record.EXPIRE_AT < int(time.time()):
                return False
            return True
    
    @staticmethod
    async def remove_from_blacklist(ip: str) -> bool:
        """从黑名单移除 IP"""
        async with LoginLogSessionFactory() as session:
            result = await session.execute(
                select(IPBlacklistModel).filter_by(IP_ADDRESS=ip)
            )
            record = result.scalar_one_or_none()
            if record:
                async with session.begin():
                    await session.delete(record)
                return True
            return False

