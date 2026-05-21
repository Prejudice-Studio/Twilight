from enum import Enum
import json
import time
import hashlib
from typing import Optional

from sqlalchemy import select, update, delete, func, String, Integer, Boolean
from sqlalchemy.ext.asyncio import AsyncAttrs
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.config import Config


class UsersDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class Role(Enum):
    ADMIN = 0  # 管理员
    NORMAL = 1  # 普通注册用户
    WHITE_LIST = 2  # 白名单用户
    UNRECOGNIZED = -1  # 未注册用户


class UserModel(UsersDatabaseModel):
    __tablename__ = "users"
    UID: Mapped[int] = mapped_column(Integer, primary_key=True, index=True)
    TELEGRAM_ID: Mapped[Optional[int]] = mapped_column(Integer, index=True, nullable=True)
    USERNAME: Mapped[Optional[str]] = mapped_column(String, index=True, nullable=True)
    EMAIL: Mapped[Optional[str]] = mapped_column(String, index=True, nullable=True)
    ROLE: Mapped[int] = mapped_column(Integer, default=Role.UNRECOGNIZED.value, nullable=False)
    ACTIVE_STATUS: Mapped[Optional[bool]] = mapped_column(Boolean, default=True, nullable=True)
    CREATE_AT: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    REGISTER_TIME: Mapped[Optional[int]] = mapped_column(Integer, default=lambda: int(time.time()), nullable=True)
    EXPIRED_AT: Mapped[Optional[int]] = mapped_column(Integer, default=-1, nullable=True)
    EMBYID: Mapped[Optional[str]] = mapped_column(String, index=True, default="", nullable=True)
    PASSWORD: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)
    BGM_MODE: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    BGM_TOKEN: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)
    LAST_LOGIN_TIME: Mapped[Optional[int]] = mapped_column(Integer, default=0, nullable=True)
    LAST_LOGIN_IP: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)
    LAST_LOGIN_UA: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)
    DEVICE_LIST: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)
    APIKEY_STATUS: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    APIKEY: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)
    APIKEY_PERMISSIONS: Mapped[Optional[str]] = mapped_column(
        String, default="", nullable=True
    )  # JSON: API Key 权限范围
    AVATAR: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)  # 用户头像 URL
    # 是否处于"待补建 Emby 账号"状态：注册码注册后未绑定 Emby 时为 True；首次登录后由用户补完。
    PENDING_EMBY: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    # 待补建时，注册码给定的开通天数（None 时回退到 EMBY_DIRECT_REGISTER_DAYS）
    PENDING_EMBY_DAYS: Mapped[Optional[int]] = mapped_column(Integer, default=None, nullable=True)
    # 是否允许该用户在个人设置中自助显示/隐藏管理员开放的特定媒体库。
    LIBRARY_SELF_SERVICE: Mapped[Optional[bool]] = mapped_column(Boolean, default=False, nullable=True)
    OTHER: Mapped[Optional[str]] = mapped_column(String, default="", nullable=True)


class TelegramRebindRequestModel(UsersDatabaseModel):
    __tablename__ = "telegram_rebind_requests"
    ID: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    OLD_TELEGRAM_ID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    STATUS: Mapped[str] = mapped_column(String, default="pending", nullable=False, index=True)
    REASON: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    ADMIN_NOTE: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    REVIEWER_UID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    CREATED_AT: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)
    UPDATED_AT: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)
    REVIEWED_AT: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)


class TelegramBindCodeModel(UsersDatabaseModel):
    __tablename__ = "telegram_bind_codes"

    CODE: Mapped[str] = mapped_column(String(16), primary_key=True)
    SCENE: Mapped[str] = mapped_column(String(16), index=True, nullable=False)  # register | user
    UID: Mapped[Optional[int]] = mapped_column(Integer, index=True, nullable=True)
    USERNAME: Mapped[Optional[str]] = mapped_column(String, nullable=True)
    CONFIRMED_TELEGRAM_ID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    CREATED_AT: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)
    EXPIRES_AT: Mapped[int] = mapped_column(Integer, index=True, nullable=False)


class AuthTokenModel(UsersDatabaseModel):
    __tablename__ = "auth_tokens"

    TOKEN: Mapped[str] = mapped_column(String(64), primary_key=True)
    UID: Mapped[int] = mapped_column(Integer, index=True, nullable=False)
    CREATED_AT: Mapped[int] = mapped_column(Integer, default=lambda: int(time.time()), nullable=False)
    EXPIRES_AT: Mapped[int] = mapped_column(Integer, index=True, nullable=False)


class TelegramBindCodeOperate:
    @staticmethod
    async def upsert_code(
        code: str,
        scene: str,
        uid: Optional[int] = None,
        username: Optional[str] = None,
        confirmed_telegram_id: Optional[int] = None,
        created_at: Optional[int] = None,
        expires_at: Optional[int] = None,
    ) -> TelegramBindCodeModel:
        now = int(time.time())
        created_at = created_at or now
        expires_at = expires_at or (created_at + 300)

        async with UsersSessionFactory() as session:
            async with session.begin():
                existing = await session.get(TelegramBindCodeModel, code)
                if existing:
                    existing.SCENE = scene
                    existing.UID = uid
                    existing.USERNAME = username
                    existing.CONFIRMED_TELEGRAM_ID = confirmed_telegram_id
                    existing.CREATED_AT = created_at
                    existing.EXPIRES_AT = expires_at
                    await session.flush()
                    return existing

                item = TelegramBindCodeModel(
                    CODE=code,
                    SCENE=scene,
                    UID=uid,
                    USERNAME=username,
                    CONFIRMED_TELEGRAM_ID=confirmed_telegram_id,
                    CREATED_AT=created_at,
                    EXPIRES_AT=expires_at,
                )
                session.add(item)
                await session.flush()
                return item

    @staticmethod
    async def get_code(code: str) -> Optional[TelegramBindCodeModel]:
        now = int(time.time())
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(TelegramBindCodeModel)
                .where(
                    TelegramBindCodeModel.CODE == code,
                    TelegramBindCodeModel.EXPIRES_AT > now,
                )
                .limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def delete_code(code: str) -> None:
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(delete(TelegramBindCodeModel).where(TelegramBindCodeModel.CODE == code))

    @staticmethod
    async def delete_user_codes(uid: int) -> None:
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    delete(TelegramBindCodeModel).where(
                        TelegramBindCodeModel.UID == uid,
                        TelegramBindCodeModel.SCENE == "user",
                    )
                )

    @staticmethod
    async def get_latest_user_code(uid: int) -> Optional[str]:
        now = int(time.time())
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(TelegramBindCodeModel.CODE)
                .where(
                    TelegramBindCodeModel.UID == uid,
                    TelegramBindCodeModel.SCENE == "user",
                    TelegramBindCodeModel.EXPIRES_AT > now,
                )
                .order_by(TelegramBindCodeModel.CREATED_AT.desc())
                .limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def cleanup_expired() -> None:
        now = int(time.time())
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(delete(TelegramBindCodeModel).where(TelegramBindCodeModel.EXPIRES_AT <= now))

    @staticmethod
    async def count_active() -> int:
        now = int(time.time())
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(func.count()).select_from(TelegramBindCodeModel).where(TelegramBindCodeModel.EXPIRES_AT > now)
            )
            return int(scalar.scalar_one() or 0)

    @staticmethod
    async def trim_to_max(max_codes: int) -> None:
        if max_codes <= 0:
            return

        now = int(time.time())
        async with UsersSessionFactory() as session:
            async with session.begin():
                scalar = await session.execute(
                    select(func.count())
                    .select_from(TelegramBindCodeModel)
                    .where(TelegramBindCodeModel.EXPIRES_AT > now)
                )
                total = int(scalar.scalar_one() or 0)
                overflow = total - max_codes
                if overflow <= 0:
                    return

                rows = await session.execute(
                    select(TelegramBindCodeModel.CODE)
                    .where(TelegramBindCodeModel.EXPIRES_AT > now)
                    .order_by(TelegramBindCodeModel.CREATED_AT.asc())
                    .limit(overflow)
                )
                codes = [row[0] for row in rows.all()]
                if not codes:
                    return

                await session.execute(delete(TelegramBindCodeModel).where(TelegramBindCodeModel.CODE.in_(codes)))


class AuthTokenOperate:
    @staticmethod
    async def upsert_token(token: str, uid: int, created_at: int, expires_at: int) -> AuthTokenModel:
        async with UsersSessionFactory() as session:
            async with session.begin():
                existing = await session.get(AuthTokenModel, token)
                if existing:
                    existing.UID = uid
                    existing.CREATED_AT = created_at
                    existing.EXPIRES_AT = expires_at
                    await session.flush()
                    return existing

                item = AuthTokenModel(
                    TOKEN=token,
                    UID=uid,
                    CREATED_AT=created_at,
                    EXPIRES_AT=expires_at,
                )
                session.add(item)
                await session.flush()
                return item

    @staticmethod
    async def get_token(token: str) -> Optional[AuthTokenModel]:
        now = int(time.time())
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(AuthTokenModel)
                .where(
                    AuthTokenModel.TOKEN == token,
                    AuthTokenModel.EXPIRES_AT > now,
                )
                .limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def delete_token(token: str) -> None:
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(delete(AuthTokenModel).where(AuthTokenModel.TOKEN == token))

    @staticmethod
    async def delete_user_tokens(uid: int) -> None:
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(delete(AuthTokenModel).where(AuthTokenModel.UID == uid))

    @staticmethod
    async def cleanup_expired() -> None:
        now = int(time.time())
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(delete(AuthTokenModel).where(AuthTokenModel.EXPIRES_AT <= now))


class TelegramRebindRequestOperate:
    @staticmethod
    async def create_request(
        uid: int, old_telegram_id: Optional[int], reason: Optional[str] = None
    ) -> TelegramRebindRequestModel:
        request = TelegramRebindRequestModel(
            UID=uid,
            OLD_TELEGRAM_ID=old_telegram_id,
            STATUS="pending",
            REASON=reason,
            CREATED_AT=int(time.time()),
            UPDATED_AT=int(time.time()),
        )
        async with UsersSessionFactory() as session:
            async with session.begin():
                session.add(request)
        return request

    @staticmethod
    async def get_request_by_uid(uid: int) -> Optional[TelegramRebindRequestModel]:
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(TelegramRebindRequestModel)
                .filter_by(UID=uid)
                .order_by(TelegramRebindRequestModel.CREATED_AT.desc())
                .limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def get_request_by_id(request_id: int) -> Optional[TelegramRebindRequestModel]:
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(TelegramRebindRequestModel).filter_by(ID=request_id).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def list_requests(
        status: Optional[str] = None, page: int = 1, per_page: int = 20
    ) -> tuple[list[TelegramRebindRequestModel], int]:
        async with UsersSessionFactory() as session:
            query = select(TelegramRebindRequestModel)
            if status:
                query = query.filter_by(STATUS=status)
            query = query.order_by(TelegramRebindRequestModel.CREATED_AT.desc())
            result = await session.execute(query.offset((page - 1) * per_page).limit(per_page))
            requests = list(result.scalars().all())

            count_query = select(func.count()).select_from(TelegramRebindRequestModel)
            if status:
                count_query = count_query.filter_by(STATUS=status)
            total_result = await session.execute(count_query)
            total = total_result.scalar_one()

            return requests, total

    @staticmethod
    async def update_request_status(
        request_id: int,
        status: str,
        reviewer_uid: Optional[int] = None,
        admin_note: Optional[str] = None,
    ) -> bool:
        async with UsersSessionFactory() as session:
            async with session.begin():
                values = {
                    "STATUS": status,
                    "UPDATED_AT": int(time.time()),
                }
                if admin_note is not None:
                    values["ADMIN_NOTE"] = admin_note
                if reviewer_uid is not None:
                    values["REVIEWER_UID"] = reviewer_uid
                if status in ("approved", "rejected"):
                    values["REVIEWED_AT"] = int(time.time())

                result = await session.execute(
                    update(TelegramRebindRequestModel)
                    .where(TelegramRebindRequestModel.ID == request_id)
                    .values(**values)
                )
                return result.rowcount > 0


from src.db.utils import init_async_db

ENGINE, UsersSessionFactory = init_async_db("users", UsersDatabaseModel)


class UserOperate:
    @staticmethod
    def _hash_apikey(apikey: str) -> str:
        """对 API Key 做单向哈希，避免数据库明文存储。"""
        return hashlib.sha256(apikey.encode("utf-8")).hexdigest()

    @staticmethod
    def _escape_like_pattern(value: str) -> str:
        """转义 SQL LIKE 模式字符，避免 %/_ 被当作通配符。"""
        return value.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")

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
    async def get_users_by_telegram_ids(telegram_ids: list[int]) -> dict[int, UserModel]:
        """批量根据 Telegram ID 获取用户，返回 {telegram_id: user}。"""
        if not telegram_ids:
            return {}

        unique_ids = list(dict.fromkeys(tid for tid in telegram_ids if tid is not None))
        if not unique_ids:
            return {}

        async with UsersSessionFactory() as session:
            result = await session.execute(select(UserModel).where(UserModel.TELEGRAM_ID.in_(unique_ids)))
            users = list(result.scalars().all())
            return {u.TELEGRAM_ID: u for u in users if u.TELEGRAM_ID is not None}

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
                    UserModel.EMBYID != "",
                )
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_user_by_emby_username(username: str) -> Optional[UserModel]:
        """根据 Emby/Jellyfin 用户名查找用户（不再兼容旧 OTHER 扫描逻辑）。"""
        async with UsersSessionFactory() as session:
            scalar = await session.execute(
                select(UserModel).where(func.lower(UserModel.USERNAME) == username.lower()).limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def update_user(user: UserModel) -> None:
        """更新用户信息"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.merge(user)

    @staticmethod
    async def set_library_self_service(uid: int, enabled: bool) -> bool:
        """设置单个用户是否允许自助显隐媒体库。"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    update(UserModel).where(UserModel.UID == uid).values(LIBRARY_SELF_SERVICE=bool(enabled))
                )
                return result.rowcount > 0

    @staticmethod
    async def enable_library_self_service_for_all() -> int:
        """为所有已注册系统用户开启媒体库自助显隐权限。"""
        from sqlalchemy import or_

        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    update(UserModel)
                    .where(
                        UserModel.ROLE != Role.UNRECOGNIZED.value,
                        or_(UserModel.LIBRARY_SELF_SERVICE.is_(None), UserModel.LIBRARY_SELF_SERVICE != True),
                    )
                    .values(LIBRARY_SELF_SERVICE=True)
                )
                return int(result.rowcount or 0)

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
                await session.execute(update(UserModel).where(UserModel.UID == user.UID).values(TELEGRAM_ID=None))

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
        if user.EXPIRED_AT == 0:
            # 待开通（未绑定 Emby），不应被续期 —— 走到这里多半是上层未做拦截
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
                select(func.count())
                .select_from(UserModel)
                .where(
                    UserModel.ROLE != Role.UNRECOGNIZED.value,
                    UserModel.ROLE != Role.WHITE_LIST.value,
                    UserModel.ROLE != Role.ADMIN.value,
                )
            )
            return result.scalar_one()

    @staticmethod
    async def get_active_users_count() -> int:
        """获取活跃用户数量（排除未注册用户、白名单用户、管理员、过期用户）"""
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(func.count())
                .select_from(UserModel)
                .where(
                    UserModel.ROLE != Role.UNRECOGNIZED.value,
                    UserModel.ROLE != Role.WHITE_LIST.value,
                    UserModel.ROLE != Role.ADMIN.value,
                    UserModel.ACTIVE_STATUS == True,
                    UserModel.EXPIRED_AT > int(time.time()),
                )
            )
            return result.scalar_one()

    @staticmethod
    async def get_emby_bound_users_count() -> int:
        """获取当前占用 Emby 名额的用户数量。

        已创建 Emby 账号与已获得注册码资格但尚未补建 Emby 的用户都占用名额，
        避免用户关掉页面后留下的 PENDING_EMBY 资格绕过上限。
        """
        from sqlalchemy import or_

        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(func.count())
                .select_from(UserModel)
                .where(
                    or_(
                        UserModel.EMBYID.is_not(None) & (UserModel.EMBYID != ""),
                        UserModel.PENDING_EMBY == True,
                    )
                )
            )
            return result.scalar_one()

    @staticmethod
    async def reset_apikey(usr: UserModel) -> str:
        """
        重置用户 API Key（数据库仅保存哈希，明文仅返回一次）
        """
        import secrets

        new_apikey = f"key-{secrets.token_hex(24)}"
        apikey_hash = UserOperate._hash_apikey(new_apikey)

        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel).where(UserModel.UID == usr.UID).values(APIKEY=apikey_hash, APIKEY_STATUS=True)
                )
        return new_apikey

    @staticmethod
    async def get_user_by_apikey(apikey: str) -> Optional[UserModel]:
        """根据 API Key 获取用户"""
        apikey_hash = UserOperate._hash_apikey(apikey)
        async with UsersSessionFactory() as session:
            scalar = await session.execute(select(UserModel).filter_by(APIKEY=apikey_hash, APIKEY_STATUS=True).limit(1))
            return scalar.scalar_one_or_none()

    @staticmethod
    async def set_apikey_status(uid: int, enabled: bool) -> bool:
        """设置 API Key 状态"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(update(UserModel).where(UserModel.UID == uid).values(APIKEY_STATUS=enabled))
                return True

    @staticmethod
    async def update_login_info(uid: int, ip: str = "", ua: str = "") -> None:
        """更新用户登录信息"""
        async with UsersSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(UserModel)
                    .where(UserModel.UID == uid)
                    .values(LAST_LOGIN_TIME=int(time.time()), LAST_LOGIN_IP=ip, LAST_LOGIN_UA=ua)
                )

    @staticmethod
    async def get_expired_users() -> list[UserModel]:
        """
        获取所有已过期但仍处于启用状态的用户
        排除永不过期(-1)与待开通(0)的用户
        """
        current_time = int(time.time())
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.EXPIRED_AT != -1,  # 排除永不过期
                    UserModel.EXPIRED_AT > 0,  # 排除待开通 sentinel
                    UserModel.EXPIRED_AT < current_time,  # 已过期
                    UserModel.ACTIVE_STATUS == True,  # 仍然启用
                    UserModel.EMBYID != "",  # 有 Emby 账户
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
                    UserModel.EXPIRED_AT > 0,  # 排除待开通 sentinel
                    UserModel.EXPIRED_AT > current_time,  # 还未过期
                    UserModel.EXPIRED_AT <= expire_threshold,  # 但即将过期
                    UserModel.ACTIVE_STATUS == True,
                    UserModel.EMBYID != "",  # 没绑 Emby 也跳过
                    UserModel.EMBYID.isnot(None),
                )
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_no_emby_users(
        days: int = 7,
        *,
        exclude_uids: Optional[set[int]] = None,
        preserve_tg_bound: bool = False,
    ) -> list[UserModel]:
        """获取注册超过指定天数但仍无 Emby 账户的用户。

        :param days: 注册超过多少天才视为可清理。
        :param exclude_uids: 额外强制保留的 UID 集合（例如正在 Emby 注册队列里处理的）。
        :param preserve_tg_bound: True 时跳过仍持有 TELEGRAM_ID 的"半完成"账号——
            他们仍可在前端继续补建 Emby，不应该被一刀切删掉。

        过滤口径：
        - EMBYID 为 NULL 或空字符串；
        - 注册时间 ≤ 阈值，时间来源 ``COALESCE(REGISTER_TIME, CREATE_AT)``，
          兼容老数据只有 CREATE_AT 而 REGISTER_TIME 是 NULL 的情况；
        - 角色不在 {ADMIN, WHITE_LIST, UNRECOGNIZED}——UNRECOGNIZED 通常是
          注册流程半路出错留下的占位记录，由人工核对，不能让定时任务一刀切。
        """
        from sqlalchemy import or_, func as _func

        threshold = int(time.time()) - days * 86400
        reg_time = _func.coalesce(UserModel.REGISTER_TIME, UserModel.CREATE_AT)

        conditions = [
            or_(UserModel.EMBYID.is_(None), UserModel.EMBYID == ""),
            reg_time.isnot(None),
            reg_time <= threshold,
            UserModel.ROLE.notin_(
                [
                    Role.ADMIN.value,
                    Role.WHITE_LIST.value,
                    Role.UNRECOGNIZED.value,
                ]
            ),
        ]
        if preserve_tg_bound:
            conditions.append(UserModel.TELEGRAM_ID.is_(None))

        async with UsersSessionFactory() as session:
            result = await session.execute(select(UserModel).where(*conditions))
            users = list(result.scalars().all())

        if exclude_uids:
            users = [u for u in users if int(u.UID) not in exclude_uids]
        return users

    @staticmethod
    async def get_all_users(
        include_inactive: bool = False,
        role: Optional[int] = None,
        search: Optional[str] = None,
        limit: int = 100,
        offset: int = 0,
        active_status: Optional[bool] = None,
        sort_by: Optional[str] = None,
        has_emby: Optional[bool] = None,
    ) -> tuple[list[UserModel], int]:
        """分页 + 筛选 + 排序获取用户列表。

        :param include_inactive: 兼容字段。``active_status`` 未传时回退使用它：
                                 False（默认）→ 仅活跃，True → 不限制。
        :param role: 角色过滤，传 ``Role.value``。
        :param search: 模糊匹配 ``UID`` / ``USERNAME`` / ``EMAIL`` / ``EMBYID`` / ``TELEGRAM_ID`` / ``OTHER`` 缓存信息。
        :param active_status: 显式启停过滤。None=不过滤；True=仅启用；False=仅禁用。
        :param sort_by: 排序字段，形如 ``uid_desc`` / ``username_asc`` / ``register_time_desc`` 等。
        :return: ``(users, total)``。
        """
        async with UsersSessionFactory() as session:
            conditions = []

            if active_status is True:
                conditions.append(UserModel.ACTIVE_STATUS == True)
            elif active_status is False:
                conditions.append(UserModel.ACTIVE_STATUS == False)
            elif not include_inactive:
                # 旧路径：未显式指定时，默认仅返回活跃用户。
                conditions.append(UserModel.ACTIVE_STATUS == True)

            if role is not None:
                conditions.append(UserModel.ROLE == role)
            if has_emby is True:
                # 占用 Emby 名额：已绑定或持有待补建资格
                from sqlalchemy import or_ as _or_emby

                conditions.append(
                    _or_emby(
                        UserModel.EMBYID.isnot(None) & (UserModel.EMBYID != ""),
                        UserModel.PENDING_EMBY == True,
                    )
                )
            elif has_emby is False:
                # 未占用 Emby 名额：未绑定且无待补建资格
                from sqlalchemy import or_ as _or_emby

                conditions.append(_or_emby(UserModel.EMBYID.is_(None), UserModel.EMBYID == ""))
                conditions.append(_or_emby(UserModel.PENDING_EMBY.is_(None), UserModel.PENDING_EMBY != True))
            if search:
                escaped = UserOperate._escape_like_pattern(search)
                like = f"%{escaped}%"
                or_clauses = [
                    UserModel.USERNAME.ilike(like, escape="\\"),
                    UserModel.EMAIL.ilike(like, escape="\\"),
                    UserModel.EMBYID.ilike(like, escape="\\"),
                    UserModel.OTHER.ilike(like, escape="\\"),
                ]
                if search.isdigit():
                    try:
                        as_int = int(search)
                        or_clauses.append(UserModel.UID == as_int)
                        or_clauses.append(UserModel.TELEGRAM_ID == as_int)
                    except ValueError:
                        pass
                from sqlalchemy import or_ as _or

                conditions.append(_or(*or_clauses))

            count_query = select(func.count()).select_from(UserModel)
            if conditions:
                count_query = count_query.where(*conditions)
            total_result = await session.execute(count_query)
            total = total_result.scalar_one()

            # 排序：字段名映射
            sort_map = {
                "uid": UserModel.UID,
                "username": UserModel.USERNAME,
                "role": UserModel.ROLE,
                "active": UserModel.ACTIVE_STATUS,
                "expired_at": UserModel.EXPIRED_AT,
                "register_time": UserModel.REGISTER_TIME,
                "last_login_time": UserModel.LAST_LOGIN_TIME,
            }
            order_field = UserModel.UID
            order_desc = True
            if isinstance(sort_by, str) and sort_by:
                base = sort_by.strip()
                direction = "desc"
                for suffix in ("_desc", "_asc"):
                    if base.endswith(suffix):
                        direction = suffix.strip("_")
                        base = base[: -len(suffix)]
                        break
                order_field = sort_map.get(base, UserModel.UID)
                order_desc = direction != "asc"

            order_clause = order_field.desc() if order_desc else order_field.asc()
            tiebreak = UserModel.UID.desc() if order_field is not UserModel.UID else None

            query = select(UserModel).order_by(*(c for c in (order_clause, tiebreak) if c is not None))
            if conditions:
                query = query.where(*conditions)
            query = query.limit(limit).offset(offset)
            result = await session.execute(query)
            return list(result.scalars().all()), total

    @staticmethod
    async def get_active_telegram_bound_users() -> list[UserModel]:
        """获取所有「ACTIVE_STATUS=True 且绑定了 Telegram」的普通用户。

        供定时群组成员资格检查使用。
        """
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.ACTIVE_STATUS == True,
                    UserModel.TELEGRAM_ID.isnot(None),
                    UserModel.ROLE != Role.ADMIN.value,
                    UserModel.ROLE != Role.WHITE_LIST.value,
                    UserModel.ROLE != Role.UNRECOGNIZED.value,
                )
            )
            return list(result.scalars().all())

    @staticmethod
    async def get_inactive_telegram_bound_users() -> list[UserModel]:
        """获取所有「ACTIVE_STATUS=False 且绑定了 Telegram」的普通用户。

        用于群组巡检时识别“已禁用但已重新入群”的候选账号，供管理员人工复核是否恢复。
        """
        async with UsersSessionFactory() as session:
            result = await session.execute(
                select(UserModel).where(
                    UserModel.ACTIVE_STATUS == False,
                    UserModel.TELEGRAM_ID.isnot(None),
                    UserModel.ROLE != Role.ADMIN.value,
                    UserModel.ROLE != Role.WHITE_LIST.value,
                    UserModel.ROLE != Role.UNRECOGNIZED.value,
                )
            )
            return list(result.scalars().all())

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

    @staticmethod
    async def list_uids_for_bulk_expire(
        *,
        include_admin: bool = False,
        include_whitelist: bool = False,
        only_with_emby: bool = True,
        only_active: bool = True,
    ) -> list[int]:
        """
        枚举批量到期调控的目标 UID 列表。

        默认排除：管理员、白名单、待开通 Emby、已禁用账号；只返回普通用户。
        调用方需要自行确认目标集合，避免误伤。
        """
        async with UsersSessionFactory() as session:
            conditions = []
            roles_to_exclude: list[int] = []
            if not include_admin:
                roles_to_exclude.append(Role.ADMIN.value)
            if not include_whitelist:
                roles_to_exclude.append(Role.WHITE_LIST.value)
            # 永远排除未识别角色（理论上不会有 EMBYID，但保险起见）
            roles_to_exclude.append(Role.UNRECOGNIZED.value)
            if roles_to_exclude:
                conditions.append(~UserModel.ROLE.in_(roles_to_exclude))
            if only_with_emby:
                conditions.append(UserModel.EMBYID.isnot(None))
                conditions.append(UserModel.EMBYID != "")
            if only_active:
                conditions.append(UserModel.ACTIVE_STATUS == True)

            query = select(UserModel.UID)
            if conditions:
                query = query.where(*conditions)
            result = await session.execute(query)
            return [int(row[0]) for row in result.all()]

    @staticmethod
    async def batch_set_expired_at(uids: list[int], expired_at: int) -> int:
        """
        批量设置 EXPIRED_AT。

        :param uids: 目标 UID 列表，调用方负责筛选/排除管理员等。
        :param expired_at: 目标到期时间戳；``-1`` 永久，``> 0`` 指定时刻。
                            ``0`` 不允许（避免误把已开通账号打回"未开通"）。
        :return: 实际更新行数。
        """
        if not uids:
            return 0
        if expired_at == 0:
            raise ValueError("expired_at=0 仅用于待开通 Emby 账号，禁止通过批量接口设置")
        if expired_at != -1 and expired_at <= 0:
            raise ValueError("expired_at 必须 > 0 或为 -1（永久）")

        async with UsersSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    update(UserModel).where(UserModel.UID.in_(uids)).values(EXPIRED_AT=expired_at)
                )
                return int(result.rowcount or 0)
