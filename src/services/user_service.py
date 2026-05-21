"""
用户业务服务层

处理用户注册、续期、绑定等业务逻辑
"""

import time
import json
import logging
from typing import List, Optional, Tuple
from dataclasses import dataclass
from enum import Enum

from sqlalchemy import func, select

from src.config import Config, RegisterConfig
from src.db.user import UserModel, UserOperate, Role, TelegramRebindRequestOperate, UsersSessionFactory
from src.services.emby import get_emby_client, EmbyError
from src.core.utils import generate_password, hash_password, timestamp, days_to_seconds, is_valid_username
from src.core.registration_lock import (
    acquire_lock,
    acquire_global_registration_lock,
    acquire_registration_lock,
    release_lock,
    release_global_registration_lock,
    release_registration_lock,
    get_cached_registered_user_count,
    set_cached_registered_user_count,
)

logger = logging.getLogger(__name__)


class RegisterResult(Enum):
    """注册结果"""

    SUCCESS = "success"
    USER_EXISTS = "user_exists"
    EMBY_EXISTS = "emby_exists"
    USER_LIMIT_REACHED = "user_limit_reached"
    EMBY_ERROR = "emby_error"
    INVALID_CODE = "invalid_code"
    CODE_EXPIRED = "code_expired"
    CODE_USED = "code_used"
    TELEGRAM_NOT_BOUND = "telegram_not_bound"
    ERROR = "error"


@dataclass
class RegisterResponse:
    """注册响应"""

    result: RegisterResult
    message: str
    user: Optional[UserModel] = None
    emby_password: Optional[str] = None


class UserService:
    """用户业务服务"""

    EMBY_CAPACITY_LOCK_KEY = "tw:emby:capacity"

    @staticmethod
    async def acquire_emby_capacity_lock(timeout: float = 5.0, ttl: int = 90) -> Optional[str]:
        """Serialize capacity-sensitive Emby grant/create paths across workers with Redis when configured."""
        return await acquire_lock(UserService.EMBY_CAPACITY_LOCK_KEY, timeout=timeout, ttl=ttl)

    @staticmethod
    async def release_emby_capacity_lock(token: Optional[str]) -> None:
        if token:
            await release_lock(UserService.EMBY_CAPACITY_LOCK_KEY, token)

    @staticmethod
    async def check_normal_user_capacity_for_grant(user: UserModel) -> Tuple[bool, str]:
        """Check USER_LIMIT only when an existing unrecognized user will be promoted to normal."""
        if not user or user.ROLE != Role.UNRECOGNIZED.value:
            return True, ""
        try:
            limit = int(RegisterConfig.USER_LIMIT)
        except (TypeError, ValueError):
            limit = 0
        if limit <= 0:
            return False, "用户数量上限配置无效"
        current = await UserService.get_registered_user_count(use_cache=False)
        if current >= limit:
            return False, f"已达到用户数量上限 ({limit})"
        return True, ""

    @staticmethod
    def is_emby_access_expired(user: UserModel) -> bool:
        """Return whether the bound Emby account is past its paid expiry."""
        if not user or not user.EMBYID:
            return False
        try:
            expired_at = int(user.EXPIRED_AT or 0)
        except (TypeError, ValueError):
            return True
        return expired_at > 0 and expired_at < timestamp()

    @staticmethod
    def should_enable_emby_access(user: UserModel) -> bool:
        """Emby is enabled only when the system account is active and not expired."""
        return bool(user and user.ACTIVE_STATUS and not UserService.is_emby_access_expired(user))

    @staticmethod
    def _normalize_code_days(days: Optional[int], default: int = 30) -> int:
        """规范化卡码天数：0/-1 都视为永久（-1）。"""
        try:
            parsed = int(days) if days is not None else int(default)
        except (TypeError, ValueError):
            parsed = int(default)
        return -1 if parsed <= 0 else parsed

    @staticmethod
    def _format_days_text(days: int) -> str:
        return "永久" if days <= 0 else f"{days} 天"

    @staticmethod
    def _is_regcode_expired(code_info) -> bool:
        """卡码自身有效期检查；VALIDITY_TIME=-1 表示永久。"""
        validity_time = getattr(code_info, "VALIDITY_TIME", -1)
        if validity_time == -1:
            return False
        try:
            validity_hours = int(validity_time)
            created_time = int(getattr(code_info, "CREATED_TIME", 0) or 0)
        except (TypeError, ValueError):
            return True
        if validity_hours < -1:
            return True
        return timestamp() > created_time + validity_hours * 3600

    @staticmethod
    def _normalize_emby_user_limit() -> int:
        try:
            limit = int(getattr(RegisterConfig, "EMBY_USER_LIMIT", -1))
        except (TypeError, ValueError):
            limit = -1
        return -1 if limit <= 0 else limit

    @staticmethod
    def get_emby_user_limit() -> int:
        """读取 Emby 绑定用户总上限（-1 表示不限制）。

        所有"会让本站用户拿到 Emby 账号"的路径都走这一个值；不要再额外引入
        "绑定专属上限"之类的拆分配置，业务侧已确认共用同一个计数器。
        """
        return UserService._normalize_emby_user_limit()

    @staticmethod
    async def get_emby_bound_user_count() -> int:
        return await UserOperate.get_emby_bound_users_count()

    @staticmethod
    def get_emby_capacity_queue_pending_uids(*, exclude_uid: Optional[int] = None) -> set[int]:
        """Return queued UIDs that have reserved or may soon consume an Emby slot."""
        pending_uids: set[int] = set()
        for module_name, class_name in (
            ("src.services.emby_register_queue", "EmbyRegisterQueueService"),
            ("src.services.regcode_use_queue", "RegcodeUseQueueService"),
        ):
            try:
                module = __import__(module_name, fromlist=[class_name])
                service = getattr(module, class_name)
                pending_uids.update(service.emby_capacity_pending_uids(exclude_uid=exclude_uid))
            except Exception as exc:  # pragma: no cover - queue modules are best-effort capacity hints
                logger.debug("读取 Emby 队列占用失败 %s.%s: %s", module_name, class_name, exc)
        if exclude_uid is not None:
            try:
                pending_uids.discard(int(exclude_uid))
            except (TypeError, ValueError):
                pass
        return pending_uids

    @staticmethod
    def get_emby_capacity_queue_pending_count(*, exclude_uid: Optional[int] = None) -> int:
        return len(UserService.get_emby_capacity_queue_pending_uids(exclude_uid=exclude_uid))

    @staticmethod
    async def check_emby_user_capacity(
        *,
        extra_pending: int = 0,
        exclude_uid: Optional[int] = None,
    ) -> Tuple[bool, str]:
        """检查 Emby 绑定用户总量是否达到上限。

        :param extra_pending: 调用方额外传入的占用量；常规队列占用会在本函数内统一读取。
        :param exclude_uid: 当前正在消费自身队列预留名额的 UID，避免把自己算成额外占用。
        """
        limit = UserService.get_emby_user_limit()
        if limit <= 0:
            return True, ""

        current = await UserService.get_emby_bound_user_count()
        queue_pending = UserService.get_emby_capacity_queue_pending_count(exclude_uid=exclude_uid)
        try:
            extra = max(0, int(extra_pending or 0))
        except (TypeError, ValueError):
            extra = 0
        projected = current + queue_pending + extra
        if projected >= limit:
            parts = [f"已绑定/待开通 {current}"]
            if queue_pending:
                parts.append(f"队列待创建 {queue_pending}")
            if extra:
                parts.append(f"额外预留 {extra}")
            return False, f"Emby 名额已达上限（{'，'.join(parts)}，合计 {projected}/{limit}）"
        return True, ""

    @staticmethod
    def resolve_emby_direct_register_days(
        user: UserModel,
    ) -> Tuple[bool, Optional[int], str]:
        """解析用户补建 Emby 时应使用的开通天数。

        策略：天数由管理员通过 ``RegisterConfig.EMBY_DIRECT_REGISTER_DAYS`` 单一固定值决定，
        前端不再提供 "选套餐 / 自定义天数" 入口。注册码授予的 PENDING_EMBY_DAYS 仍优先生效。
        """
        pending_days = getattr(user, "PENDING_EMBY_DAYS", None)
        if pending_days is not None:
            days = UserService._normalize_code_days(pending_days, default=30)
            return True, days, ""

        if not RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED:
            return False, None, "当前未开启自由注册 Emby"

        if not getattr(user, "TELEGRAM_ID", None):
            return False, None, "自由注册 Emby 前请先绑定 Telegram"

        days = UserService._normalize_code_days(
            RegisterConfig.EMBY_DIRECT_REGISTER_DAYS,
            default=30,
        )
        return True, days, ""

    @staticmethod
    def validate_password_strength(password: Optional[str], label: str = "密码") -> Tuple[bool, str]:
        """统一的密码强度校验：≥ 8 位，且包含大小写字母与数字。"""
        if password is None or password == "":
            return False, f"请提供{label}"
        if len(password) < 8:
            return False, f"{label}强度不足：至少 8 位，且包含大小写字母和数字"
        if len(password) > 128:
            return False, f"{label}过长，最多 128 位"
        if not any(ch.islower() for ch in password):
            return False, f"{label}强度不足：至少包含一个小写字母"
        if not any(ch.isupper() for ch in password):
            return False, f"{label}强度不足：至少包含一个大写字母"
        if not any(ch.isdigit() for ch in password):
            return False, f"{label}强度不足：至少包含一个数字"
        return True, ""

    @staticmethod
    def _validate_emby_register_password(password: Optional[str]) -> Tuple[bool, str]:
        """校验 Emby 注册密码强度（保留旧入口，复用统一规则）。"""
        return UserService.validate_password_strength(password, label="Emby 密码")

    @staticmethod
    async def get_registered_user_count(use_cache: bool = True) -> int:
        """获取当前已注册用户数量。

        :param use_cache: 是否优先使用短期缓存。
        """
        if use_cache:
            cached = await get_cached_registered_user_count()
            if cached is not None:
                return cached

        current_count = await UserOperate.get_registered_users_count()
        await set_cached_registered_user_count(current_count)
        return current_count

    @staticmethod
    async def check_registration_available(use_cache: bool = True) -> Tuple[bool, str]:
        """检查是否可以注册"""
        if not RegisterConfig.REGISTER_MODE:
            return False, "注册功能已关闭"

        current_count = await UserService.get_registered_user_count(use_cache=use_cache)
        if current_count >= RegisterConfig.USER_LIMIT:
            return False, f"已达到用户数量上限 ({RegisterConfig.USER_LIMIT})"

        return True, "可以注册"

    @staticmethod
    async def get_telegram_rebind_request(uid: int):
        return await TelegramRebindRequestOperate.get_request_by_uid(uid)

    @staticmethod
    def _normalize_text_note(note: Optional[str], *, max_length: int = 300) -> Optional[str]:
        """清理备注字段，防止超长输入写库。"""
        if note is None:
            return None
        if not isinstance(note, str):
            note = str(note)
        note = note.strip()
        if not note:
            return None
        if len(note) > max_length:
            return note[:max_length]
        return note

    @staticmethod
    async def create_telegram_rebind_request(
        user: UserModel, reason: Optional[str] = None
    ) -> Tuple[bool, str, Optional[object]]:
        if not user.TELEGRAM_ID:
            return False, "当前账号尚未绑定 Telegram"

        existing = await TelegramRebindRequestOperate.get_request_by_uid(user.UID)
        if existing and existing.STATUS == "pending":
            return False, "您已有待处理的换绑请求，请等待管理员处理"

        clean_reason = UserService._normalize_text_note(reason, max_length=500)
        request = await TelegramRebindRequestOperate.create_request(user.UID, user.TELEGRAM_ID, clean_reason)
        return True, "换绑请求已提交，管理员审核后您可以重新绑定 Telegram", request

    @staticmethod
    async def list_telegram_rebind_requests(status: Optional[str] = None, page: int = 1, per_page: int = 20):
        return await TelegramRebindRequestOperate.list_requests(status=status, page=page, per_page=per_page)

    @staticmethod
    async def approve_telegram_rebind_request(
        request_id: int, reviewer_uid: int, admin_note: Optional[str] = None
    ) -> Tuple[bool, str]:
        request = await TelegramRebindRequestOperate.get_request_by_id(request_id)
        if not request:
            return False, "换绑请求不存在"
        if request.STATUS != "pending":
            return False, "该请求已处理"

        user = await UserOperate.get_user_by_uid(request.UID)
        if not user:
            return False, "关联用户不存在"

        if user.TELEGRAM_ID != request.OLD_TELEGRAM_ID:
            return False, "用户当前 Telegram 与申请时不一致，请刷新核实后再处理"

        if user.TELEGRAM_ID:
            await UserOperate.unbind_telegram_user(user)

        clean_note = UserService._normalize_text_note(admin_note, max_length=500)
        success = await TelegramRebindRequestOperate.update_request_status(
            request_id,
            "approved",
            reviewer_uid=reviewer_uid,
            admin_note=clean_note,
        )
        if not success:
            return False, "更新请求状态失败"

        return True, "已批准换绑请求并解绑当前 Telegram，用户可重新绑定新账号"

    @staticmethod
    async def reject_telegram_rebind_request(
        request_id: int, reviewer_uid: int, admin_note: Optional[str] = None
    ) -> Tuple[bool, str]:
        request = await TelegramRebindRequestOperate.get_request_by_id(request_id)
        if not request:
            return False, "换绑请求不存在"
        if request.STATUS != "pending":
            return False, "该请求已处理"

        clean_note = UserService._normalize_text_note(admin_note, max_length=500)
        success = await TelegramRebindRequestOperate.update_request_status(
            request_id,
            "rejected",
            reviewer_uid=reviewer_uid,
            admin_note=clean_note,
        )
        if not success:
            return False, "更新请求状态失败"

        return True, "已拒绝换绑请求"

    @staticmethod
    async def batch_review_telegram_rebind_requests(
        request_ids: list[int],
        action: str,
        reviewer_uid: int,
        admin_note: Optional[str] = None,
    ) -> dict:
        action = (action or "").strip().lower()
        clean_ids = []
        seen = set()
        for raw_id in request_ids:
            try:
                request_id = int(raw_id)
            except (TypeError, ValueError):
                continue
            if request_id <= 0 or request_id in seen:
                continue
            seen.add(request_id)
            clean_ids.append(request_id)

        results = []
        success_count = 0
        failed_count = 0
        for request_id in clean_ids:
            if action == "approve":
                ok, msg = await UserService.approve_telegram_rebind_request(request_id, reviewer_uid, admin_note)
            else:
                ok, msg = await UserService.reject_telegram_rebind_request(request_id, reviewer_uid, admin_note)
            results.append({"id": request_id, "success": ok, "message": msg})
            if ok:
                success_count += 1
            else:
                failed_count += 1

        return {
            "action": action,
            "total": len(clean_ids),
            "success": success_count,
            "failed": failed_count,
            "results": results,
        }

    @staticmethod
    async def register_by_code(
        telegram_id: Optional[int],
        username: str,
        reg_code: str,
        email: Optional[str] = None,
        password: Optional[str] = None,
    ) -> RegisterResponse:
        """
        通过注册码注册：仅创建系统账号并标记 PENDING_EMBY=True，
        Emby 账号由用户首次登录后在前端 Modal 补完。

        :param telegram_id: Telegram ID（Web 注册时可为空）
        :param username: 系统/Emby 用户名（首次提交即为期望的用户名）
        :param reg_code: 注册码
        :param email: 邮箱（可选）
        :param password: 密码（Web 注册时使用，为空则自动生成）
        """
        from src.db.regcode import RegCodeOperate, Type as RegCodeType

        # 仅校验注册码合法性，再委派 register_pending 完成系统账号创建。
        # 这样既复用了管理员/白名单识别、并发锁等逻辑，又确保 Emby 账号不会在此立即创建。
        code_info = await RegCodeOperate.get_regcode_by_code(reg_code)
        if not code_info:
            return RegisterResponse(RegisterResult.INVALID_CODE, "注册码无效")

        if RegCodeOperate.is_decoy(code_info):
            await RegCodeOperate.record_regcode_use(reg_code, telegram_id=telegram_id, increment=1)
            logger.warning("诱饵注册码被注册接口提交: telegram_id=%s code=%s", telegram_id, reg_code)
            return RegisterResponse(RegisterResult.INVALID_CODE, "注册码无效")

        if code_info.TYPE != RegCodeType.REGISTER.value:
            return RegisterResponse(RegisterResult.INVALID_CODE, "这不是注册码")

        if not code_info.ACTIVE:
            return RegisterResponse(RegisterResult.CODE_EXPIRED, "注册码已停用")

        if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
            return RegisterResponse(RegisterResult.CODE_USED, "注册码已被使用完")

        if code_info.VALIDITY_TIME != -1:
            expire_time = code_info.CREATED_TIME + code_info.VALIDITY_TIME * 3600
            if timestamp() > expire_time:
                return RegisterResponse(RegisterResult.CODE_EXPIRED, "注册码已过期")

        pending_days = UserService._normalize_code_days(code_info.DAYS, default=30)

        response = await UserService.register_pending(
            telegram_id=telegram_id,
            username=username,
            email=email,
            password=password,
            pending_emby_days=pending_days,
            skip_pending_check=True,
            reg_code_lock=reg_code,
        )

        # 注册码消费：只在系统账号创建成功时计数
        if response.result == RegisterResult.SUCCESS:
            await RegCodeOperate.record_regcode_use(
                reg_code,
                uid=response.user.UID if response.user else None,
                telegram_id=telegram_id,
            )
            # 重新组装一份更贴合"已使用注册码"的提示文案
            days_text = "永久" if pending_days <= 0 else f"{pending_days} 天"
            response.message = f"注册成功！注册码已使用，Emby 账号开通时长 {days_text}，请在首次登录后补建 Emby 账号。"

        return response

    # 注：历史 ``register_direct_emby`` 已删除——它的 Bot 自由注册流程从未接通，
    # 当前所有"为已登录用户补建 Emby"都统一走 ``complete_emby_registration``，
    # 由 ``EmbyRegisterQueueService`` 在前面串行 + 限流。

    @staticmethod
    async def register_pending(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str] = None,
        password: Optional[str] = None,
        pending_emby_days: Optional[int] = None,
        skip_pending_check: bool = False,
        reg_code_lock: Optional[str] = None,
    ) -> RegisterResponse:
        """
        注册系统账号（仅创建系统账号，未绑定 Emby）。

        :param pending_emby_days: 注册码授予的开通天数；为 None 时不默认授予 Emby 开通资格
        :param skip_pending_check: 走注册码路径时不需要再校验 ALLOW_PENDING_REGISTER
        """
        from src.config import RegisterConfig

        # 检查是否允许无码注册（除非来自注册码路径）
        if not skip_pending_check and not RegisterConfig.ALLOW_PENDING_REGISTER:
            return RegisterResponse(RegisterResult.ERROR, "暂不开放注册，请使用注册码")

        locks = await acquire_registration_lock(username, telegram_id, reg_code=reg_code_lock)
        if locks is None:
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        global_lock = await acquire_global_registration_lock()
        if global_lock is None:
            await release_registration_lock(locks)
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        emby_capacity_lock: Optional[str] = None
        try:
            if pending_emby_days is not None:
                emby_capacity_lock = await UserService.acquire_emby_capacity_lock()
                if emby_capacity_lock is None:
                    return RegisterResponse(RegisterResult.ERROR, "Emby 名额检查繁忙，请稍后重试")
                cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
                if not cap_ok:
                    return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, cap_msg)

            # 二次校验（避免锁竞争窗口）
            if not skip_pending_check and not RegisterConfig.ALLOW_PENDING_REGISTER:
                return RegisterResponse(RegisterResult.ERROR, "暂不开放注册，请使用注册码")

            async with UsersSessionFactory() as session:
                async with session.begin():
                    # 注册临界区只占用一个 DB session，降低高并发注册时的连接占用和锁竞争。
                    if not RegisterConfig.REGISTER_MODE:
                        return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, "注册功能已关闭")

                    count_result = await session.execute(
                        select(func.count())
                        .select_from(UserModel)
                        .where(
                            UserModel.ROLE != Role.UNRECOGNIZED.value,
                            UserModel.ROLE != Role.WHITE_LIST.value,
                            UserModel.ROLE != Role.ADMIN.value,
                        )
                    )
                    current_count = int(count_result.scalar_one() or 0)
                    if current_count >= RegisterConfig.USER_LIMIT:
                        return RegisterResponse(
                            RegisterResult.USER_LIMIT_REACHED,
                            f"已达到用户数量上限 ({RegisterConfig.USER_LIMIT})",
                        )

                    existing = await session.execute(select(UserModel.UID).filter_by(USERNAME=username).limit(1))
                    if existing.scalar_one_or_none() is not None:
                        return RegisterResponse(RegisterResult.USER_EXISTS, "用户名已被使用")

                    if telegram_id:
                        existing_tg = await session.execute(
                            select(UserModel.UID).filter_by(TELEGRAM_ID=telegram_id).limit(1)
                        )
                        if existing_tg.scalar_one_or_none() is not None:
                            return RegisterResponse(RegisterResult.USER_EXISTS, "该 Telegram 账号已注册")

                    max_uid_result = await session.execute(select(func.max(UserModel.UID)).limit(1))
                    max_uid = max_uid_result.scalar_one_or_none()
                    new_uid = 1 if max_uid is None else int(max_uid) + 1
                    user_password = password if password else generate_password(12)

                    is_admin = False
                    is_whitelist = False

                    admin_uids = RegisterConfig.ADMIN_UIDS
                    if admin_uids:
                        uid_list = [int(u.strip()) for u in admin_uids.split(",") if u.strip().isdigit()]
                        is_admin = new_uid in uid_list

                    if not is_admin:
                        admin_usernames = RegisterConfig.ADMIN_USERNAMES
                        if admin_usernames:
                            name_list = [n.strip().lower() for n in admin_usernames.split(",") if n.strip()]
                            is_admin = username.lower() in name_list

                    if not is_admin:
                        whitelist_uids = RegisterConfig.WHITE_LIST_UIDS
                        if whitelist_uids:
                            uid_list = [int(u.strip()) for u in whitelist_uids.split(",") if u.strip().isdigit()]
                            is_whitelist = new_uid in uid_list

                    if not is_admin and not is_whitelist:
                        whitelist_usernames = RegisterConfig.WHITE_LIST_USERNAMES
                        if whitelist_usernames:
                            name_list = [n.strip().lower() for n in whitelist_usernames.split(",") if n.strip()]
                            is_whitelist = username.lower() in name_list

                    permanent_expire = 253402214400
                    created_at = timestamp()
                    has_emby_entitlement = pending_emby_days is not None

                    if is_admin:
                        user = UserModel(
                            UID=new_uid,
                            TELEGRAM_ID=telegram_id,
                            USERNAME=username,
                            EMAIL=email,
                            EMBYID=None,
                            PASSWORD=hash_password(user_password),
                            ROLE=Role.ADMIN.value,
                            ACTIVE_STATUS=True,
                            EXPIRED_AT=permanent_expire,
                            CREATE_AT=created_at,
                            REGISTER_TIME=created_at,
                            PENDING_EMBY=has_emby_entitlement,
                            PENDING_EMBY_DAYS=pending_emby_days,
                        )
                    elif is_whitelist:
                        user = UserModel(
                            UID=new_uid,
                            TELEGRAM_ID=telegram_id,
                            USERNAME=username,
                            EMAIL=email,
                            EMBYID=None,
                            PASSWORD=hash_password(user_password),
                            ROLE=Role.WHITE_LIST.value,
                            ACTIVE_STATUS=True,
                            EXPIRED_AT=permanent_expire,
                            CREATE_AT=created_at,
                            REGISTER_TIME=created_at,
                            PENDING_EMBY=has_emby_entitlement,
                            PENDING_EMBY_DAYS=pending_emby_days,
                        )
                    else:
                        user = UserModel(
                            UID=new_uid,
                            TELEGRAM_ID=telegram_id,
                            USERNAME=username,
                            EMAIL=email,
                            EMBYID=None,
                            PASSWORD=hash_password(user_password),
                            ROLE=Role.NORMAL.value,
                            ACTIVE_STATUS=True,
                            EXPIRED_AT=0,
                            CREATE_AT=created_at,
                            REGISTER_TIME=created_at,
                            PENDING_EMBY=has_emby_entitlement,
                            PENDING_EMBY_DAYS=pending_emby_days,
                        )

                    session.add(user)
                    await session.flush()

            cache_count = current_count + 1 if user.ROLE == Role.NORMAL.value else current_count
            await set_cached_registered_user_count(cache_count)

            logger.info(
                f"系统用户注册: {username} (UID: {new_uid}, "
                f"pending_emby={has_emby_entitlement}, pending_emby_days={pending_emby_days})"
            )

            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message="注册成功！您可以登录并使用基础功能。",
                user=user,
                emby_password=user_password if not password else None,
            )
        finally:
            await UserService.release_emby_capacity_lock(emby_capacity_lock)
            await release_global_registration_lock(global_lock)
            await release_registration_lock(locks)

    @staticmethod
    async def complete_emby_registration(
        user: UserModel,
        emby_username: str,
        emby_password: str,
    ) -> RegisterResponse:
        """已登录用户补建 Emby 账号；失败保留 PENDING_EMBY 标记便于重试。

        开通天数已统一由管理员通过 ``RegisterConfig.EMBY_DIRECT_REGISTER_DAYS`` 配置，
        用户不再传入 ``days``；老版本前端如果继续传字段，由 API 层忽略。
        """
        import json

        if user.EMBYID:
            return RegisterResponse(RegisterResult.USER_EXISTS, "您已绑定 Emby 账号")

        emby_username = (emby_username or "").strip()
        if not emby_username:
            return RegisterResponse(RegisterResult.ERROR, "Emby 用户名不能为空")
        if not is_valid_username(emby_username):
            return RegisterResponse(
                RegisterResult.ERROR,
                "Emby 用户名格式不正确（3-20位字母数字下划线，不能以数字开头）",
            )

        ok, msg = UserService.validate_password_strength(emby_password, label="Emby 密码")
        if not ok:
            return RegisterResponse(RegisterResult.ERROR, msg)

        days_ok, days, days_msg = UserService.resolve_emby_direct_register_days(user)
        if not days_ok or days is None:
            return RegisterResponse(RegisterResult.ERROR, days_msg)

        locks = await acquire_registration_lock(emby_username, user.TELEGRAM_ID)
        if locks is None:
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        emby = get_emby_client()
        emby_capacity_lock: Optional[str] = None
        try:
            emby_capacity_lock = await UserService.acquire_emby_capacity_lock()
            if emby_capacity_lock is None:
                return RegisterResponse(RegisterResult.ERROR, "Emby 名额检查繁忙，请稍后重试")
            if not getattr(user, "PENDING_EMBY", False):
                cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
                if not cap_ok:
                    return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, cap_msg)

            existing_emby = await emby.get_user_by_name(emby_username)
            if existing_emby:
                return RegisterResponse(RegisterResult.EMBY_EXISTS, "该用户名在 Emby 中已存在")

            try:
                emby_user = await emby.create_user(emby_username, emby_password)
            except EmbyError as exc:
                logger.error(f"用户 {user.UID} 补建 Emby 账号失败: {exc}")
                return RegisterResponse(RegisterResult.EMBY_ERROR, f"创建 Emby 账户失败: {exc}")

            if not emby_user:
                return RegisterResponse(RegisterResult.EMBY_ERROR, "创建 Emby 账户失败：未返回用户信息")

            # 管理员/白名单永久；其它账号按 days 计算
            if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
                user.EXPIRED_AT = 253402214400
            else:
                user.EXPIRED_AT = (timestamp() + days_to_seconds(days)) if days > 0 else -1

            user.EMBYID = emby_user.id
            user.PENDING_EMBY = False
            user.PENDING_EMBY_DAYS = None

            other_data = {}
            if user.OTHER:
                try:
                    other_data = json.loads(user.OTHER)
                except (json.JSONDecodeError, TypeError):
                    other_data = {}
            other_data["emby_username"] = emby_username
            user.OTHER = json.dumps(other_data)

            await UserOperate.update_user(user)

            # 把启用/到期状态同步到 Emby（启用 Policy 等）
            try:
                await UserService.sync_user_to_emby(user)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"补建 Emby 后同步状态失败: {exc}")

            try:
                from src.services.emby_service import EmbyService

                await EmbyService.apply_default_hidden_libraries(user)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"补建 Emby 后应用默认隐藏媒体库失败: {exc}")

            days_text = UserService._format_days_text(days)
            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message=f"Emby 账号已创建并绑定，开通时长 {days_text}",
                user=user,
            )
        finally:
            await UserService.release_emby_capacity_lock(emby_capacity_lock)
            await release_registration_lock(locks)

    @staticmethod
    async def _create_emby_user(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str],
        days: int,
        reg_code: Optional[str] = None,
        password: Optional[str] = None,
    ) -> RegisterResponse:
        """创建 Emby 用户（内部方法）"""
        emby = get_emby_client()
        emby_capacity_lock: Optional[str] = None

        try:
            emby_capacity_lock = await UserService.acquire_emby_capacity_lock()
            if emby_capacity_lock is None:
                return RegisterResponse(RegisterResult.ERROR, "Emby 名额检查繁忙，请稍后重试")
            existing_user = None
            if telegram_id:
                existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
            if not getattr(existing_user, "PENDING_EMBY", False):
                cap_ok, cap_msg = await UserService.check_emby_user_capacity(
                    exclude_uid=existing_user.UID if existing_user else None,
                )
                if not cap_ok:
                    return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, cap_msg)

            # 检查 Emby 用户名是否已存在
            existing_emby = await emby.get_user_by_name(username)
            if existing_emby:
                return RegisterResponse(RegisterResult.EMBY_EXISTS, "该用户名在 Emby 中已存在")

            # 使用提供的密码或生成随机密码
            user_password = password if password else generate_password(12)
            emby_user = await emby.create_user(username, user_password)

            if not emby_user:
                return RegisterResponse(RegisterResult.EMBY_ERROR, "创建 Emby 账户失败")

            # 计算过期时间
            expire_at = timestamp() + days_to_seconds(days) if days > 0 else -1
            now_ts = timestamp()

            # 创建或更新本地用户记录
            if existing_user:
                user_limit_ok, user_limit_msg = await UserService.check_normal_user_capacity_for_grant(existing_user)
                if not user_limit_ok:
                    await emby.delete_user(emby_user.id)
                    return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, user_limit_msg)

                existing_user.EMBYID = emby_user.id
                existing_user.PENDING_EMBY = False
                existing_user.PENDING_EMBY_DAYS = None
                existing_user.PASSWORD = hash_password(user_password)
                # 如果是管理员或白名单，保持永久有效期
                if existing_user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
                    existing_user.EXPIRED_AT = 253402214400  # 9999-12-31
                else:
                    existing_user.EXPIRED_AT = expire_at
                # 如果角色是未注册，更新为普通用户
                if existing_user.ROLE == Role.UNRECOGNIZED.value:
                    existing_user.ROLE = Role.NORMAL.value
                existing_user.EMAIL = email
                # 历史数据修复：仅在缺失时回填，避免补建 Emby 覆盖账号创建时间
                if not existing_user.CREATE_AT:
                    existing_user.CREATE_AT = existing_user.REGISTER_TIME or now_ts
                if not existing_user.REGISTER_TIME:
                    existing_user.REGISTER_TIME = existing_user.CREATE_AT or now_ts
                import json

                other_data = {}
                if existing_user.OTHER:
                    try:
                        other_data = json.loads(existing_user.OTHER)
                    except (json.JSONDecodeError, TypeError):
                        other_data = {}
                other_data["emby_username"] = username
                existing_user.OTHER = json.dumps(other_data)
                await UserOperate.update_user(existing_user)
                user = existing_user
            else:
                new_uid = await UserOperate.get_new_uid()

                # 检查是否是管理员或白名单
                is_admin = False
                is_whitelist = False

                # 检查管理员
                admin_uids = RegisterConfig.ADMIN_UIDS
                if admin_uids:
                    uid_list = [int(u.strip()) for u in admin_uids.split(",") if u.strip().isdigit()]
                    is_admin = new_uid in uid_list
                if not is_admin:
                    admin_usernames = RegisterConfig.ADMIN_USERNAMES
                    if admin_usernames:
                        name_list = [n.strip().lower() for n in admin_usernames.split(",") if n.strip()]
                        is_admin = username.lower() in name_list

                # 检查白名单
                if not is_admin:
                    whitelist_uids = RegisterConfig.WHITE_LIST_UIDS
                    if whitelist_uids:
                        uid_list = [int(u.strip()) for u in whitelist_uids.split(",") if u.strip().isdigit()]
                        is_whitelist = new_uid in uid_list
                if not is_admin and not is_whitelist:
                    whitelist_usernames = RegisterConfig.WHITE_LIST_USERNAMES
                    if whitelist_usernames:
                        name_list = [n.strip().lower() for n in whitelist_usernames.split(",") if n.strip()]
                        is_whitelist = username.lower() in name_list

                # 确定角色和到期时间
                if is_admin:
                    user_role = Role.ADMIN.value
                    user_expire = 253402214400  # 9999-12-31
                elif is_whitelist:
                    user_role = Role.WHITE_LIST.value
                    user_expire = 253402214400  # 9999-12-31
                else:
                    user_role = Role.NORMAL.value
                    user_expire = expire_at

                if user_role == Role.NORMAL.value:
                    current_count = await UserService.get_registered_user_count(use_cache=False)
                    if current_count >= RegisterConfig.USER_LIMIT:
                        await emby.delete_user(emby_user.id)
                        return RegisterResponse(
                            RegisterResult.USER_LIMIT_REACHED,
                            f"已达到用户数量上限 ({RegisterConfig.USER_LIMIT})",
                        )

                user = UserModel(
                    UID=new_uid,
                    TELEGRAM_ID=telegram_id,  # 可以为 None
                    USERNAME=username,
                    EMAIL=email,
                    EMBYID=emby_user.id,
                    PASSWORD=hash_password(user_password),
                    ROLE=user_role,
                    EXPIRED_AT=user_expire,
                    CREATE_AT=now_ts,
                    REGISTER_TIME=now_ts,
                    OTHER=json.dumps({"emby_username": username}),
                )
                await UserOperate.add_user(user)

            # 更新注册码使用记录
            if reg_code:
                from src.db.regcode import RegCodeOperate

                await RegCodeOperate.record_regcode_use(reg_code, uid=user.UID, telegram_id=telegram_id)

            try:
                from src.services.emby_service import EmbyService

                await EmbyService.apply_default_hidden_libraries(user)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"注册后应用默认隐藏媒体库失败: {exc}")

            logger.info(f"用户注册成功: {username} (TG: {telegram_id})")

            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message=f"注册成功！有效期 {days} 天",
                user=user,
                emby_password=user_password if not password else None,  # 仅自动生成时返回
            )

        except EmbyError as e:
            logger.error(f"Emby 错误: {e}")
            return RegisterResponse(RegisterResult.EMBY_ERROR, f"Emby 服务器错误: {e}")
        except Exception as e:
            logger.error(f"注册错误: {e}")
            return RegisterResponse(RegisterResult.ERROR, f"注册失败: {e}")
        finally:
            await UserService.release_emby_capacity_lock(emby_capacity_lock)

    @staticmethod
    async def renew_user(user: UserModel, days: int, reg_code: Optional[str] = None) -> Tuple[bool, str]:
        """
        续期用户

        :param user: 用户对象
        :param days: 续期天数
        :param reg_code: 续期码（可选）
        """
        if reg_code:
            from src.db.regcode import RegCodeOperate, Type as RegCodeType

            code_info = await RegCodeOperate.get_regcode_by_code(reg_code)
            if not code_info:
                return False, "续期码无效"

            if RegCodeOperate.is_decoy(code_info):
                ok, msg, _pwd = await UserService._handle_decoy_regcode(user, code_info, reg_code)
                return ok, msg

            if code_info.TYPE != RegCodeType.RENEW.value:
                return False, "这不是续期码"

            if not code_info.ACTIVE:
                return False, "续期码已停用"

            if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
                return False, "续期码已被使用完"

            if UserService._is_regcode_expired(code_info):
                return False, "续期码已过期"

            invite_renew_meta = RegCodeOperate.get_invite_renew_meta(code_info)
            target_uid = invite_renew_meta.get("target_uid") if invite_renew_meta else None
            if target_uid is not None:
                try:
                    target_uid_int = int(target_uid)
                except (TypeError, ValueError):
                    return False, "续期码绑定信息异常，请联系管理员"
                if int(user.UID) != target_uid_int:
                    await RegCodeOperate.record_regcode_use(reg_code, uid=user.UID, telegram_id=user.TELEGRAM_ID, increment=0)
                    user.ACTIVE_STATUS = False
                    await UserOperate.update_user(user)
                    try:
                        await UserService.sync_user_to_emby(user)
                    except Exception as exc:  # pragma: no cover
                        logger.warning("专属续期码误用后同步禁用 Emby 失败: %s", exc)
                    logger.warning(
                        "专属续期码被非目标用户使用并已禁用: code=%s target_uid=%s actual_uid=%s",
                        reg_code,
                        target_uid_int,
                        user.UID,
                    )
                    return False, "该续期码仅限指定下级使用，当前账号已按安全策略禁用"

            days = UserService._normalize_code_days(code_info.DAYS, default=days)
            await RegCodeOperate.record_regcode_use(reg_code, uid=user.UID, telegram_id=user.TELEGRAM_ID)

        # 待开通 Emby 的用户没有真实到期概念，续期没有意义；后续 complete_emby_registration
        # 会用 PENDING_EMBY_DAYS 重新计算到期时间，此处续期反而会被覆盖掉。
        if not user.EMBYID:
            return False, "用户尚未绑定 Emby 账号，无法续期。请先完成 Emby 账号开通。"

        if days <= 0:
            user.EXPIRED_AT = -1
            await UserOperate.update_user(user)
        else:
            current_time = timestamp()
            base_expired_at = int(user.EXPIRED_AT or 0)
            new_expired_at = (current_time if base_expired_at < current_time else base_expired_at) + days_to_seconds(days)
            await UserOperate.renew_user_expire_time(user, days)
            user.EXPIRED_AT = new_expired_at

        # 如果用户被禁用，重新启用系统账号；续期后无论原本是否禁用 Emby，都按最新到期状态同步。
        if not user.ACTIVE_STATUS:
            user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)

        if user.EMBYID:
            emby = get_emby_client()
            await emby.set_user_enabled(user.EMBYID, UserService.should_enable_emby_access(user))

        logger.info(f"用户续期成功: {user.USERNAME}, days={days}")
        if days <= 0:
            return True, "续期成功！有效期已设置为永久"
        return True, f"续期成功！增加 {days} 天"

    @staticmethod
    async def disable_user(user: UserModel, reason: str = "") -> Tuple[bool, str]:
        """禁用用户"""
        try:
            user.ACTIVE_STATUS = False
            await UserOperate.update_user(user)

            # 禁用 Emby 账户
            if user.EMBYID:
                emby = get_emby_client()
                await emby.set_user_enabled(user.EMBYID, False)

            logger.info(f"用户已禁用: {user.USERNAME}, 原因: {reason}")
            return True, "用户已禁用"
        except Exception as e:
            logger.error(f"禁用用户失败: {e}")
            return False, f"禁用失败: {e}"

    @staticmethod
    async def enable_user(user: UserModel) -> Tuple[bool, str]:
        """启用用户"""
        try:
            user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)

            if user.EMBYID:
                emby = get_emby_client()
                await emby.set_user_enabled(user.EMBYID, UserService.should_enable_emby_access(user))

            logger.info(f"用户已启用: {user.USERNAME}")
            return True, "用户已启用"
        except Exception as e:
            logger.error(f"启用用户失败: {e}")
            return False, f"启用失败: {e}"

    @staticmethod
    async def delete_user(user: UserModel, delete_emby: bool = True) -> Tuple[bool, str]:
        """
        删除用户

        :param user: 用户对象
        :param delete_emby: 是否同时删除 Emby 账户
        """
        try:
            # 删除 Emby 账户
            if delete_emby and user.EMBYID:
                emby = get_emby_client()
                await emby.delete_user(user.EMBYID)

            # 清理邀请关系（如果启用）：子节点自动晋升为新树根
            try:
                from src.db.invite import InviteRelationOperate, InviteCodeOperate

                await InviteRelationOperate.delete_relations_for_uid(user.UID)
                await InviteCodeOperate.delete_for_inviter(user.UID)
            except Exception as exc:  # pragma: no cover - 邀请表缺失不应阻塞主删除
                logger.warning(f"清理邀请关系失败 uid={user.UID}: {exc}")

            # 删除用户记录
            await UserOperate.delete_user(user)

            logger.info(f"用户已删除: {user.USERNAME}")
            return True, "用户已删除"
        except Exception as e:
            logger.error(f"删除用户失败: {e}")
            return False, f"删除失败: {e}"

    @staticmethod
    async def delete_emby_only(user: UserModel) -> Tuple[bool, str]:
        """删除用户在 Emby 中的账号，但保留本地账户。"""
        if not user.EMBYID:
            return False, "用户未绑定 Emby 账户"

        old_emby_id = user.EMBYID
        try:
            emby = get_emby_client()
            ok = await emby.delete_user(old_emby_id)
            if not ok:
                return False, "删除 Emby 账户失败"
        except Exception as e:
            logger.error(f"删除 Emby 账户失败: {e}", exc_info=True)
            return False, f"删除 Emby 账户失败: {e}"

        user.EMBYID = None
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
            except (json.JSONDecodeError, TypeError):
                other_data = {}
            if isinstance(other_data, dict):
                other_data.pop("emby_username", None)
                user.OTHER = json.dumps(other_data) if other_data else ""
        await UserOperate.update_user(user)

        logger.info(f"已删除 Emby 账户: {user.USERNAME} (原 Emby ID: {old_emby_id})")
        return True, "Emby 账户已删除，本地账户已保留"

    @staticmethod
    async def use_code(
        user: UserModel,
        code_str: str,
        emby_username: Optional[str] = None,
        emby_password: Optional[str] = None,
    ) -> Tuple[bool, str, Optional[str]]:
        """串行化同一用户/同一卡码的兑换，避免多次卡码并发串绑。"""
        locks = await acquire_registration_lock(f"uid-{user.UID}", user.TELEGRAM_ID, reg_code=code_str, timeout=0.5)
        if locks is None:
            return False, "当前卡码或账号正在处理中，请稍后重试", None
        try:
            fresh_user = await UserOperate.get_user_by_uid(user.UID)
            if not fresh_user:
                return False, "用户不存在", None
            return await UserService._use_code_unlocked(fresh_user, code_str, emby_username, emby_password)
        finally:
            await release_registration_lock(locks)

    @staticmethod
    async def _handle_decoy_regcode(user: UserModel, code_info, code_str: str) -> Tuple[bool, str, Optional[str]]:
        """处理诱饵卡码。返回失败，必要时按配置惩罚当前已登录账号。"""
        from src.db.regcode import RegCodeOperate

        action = (RegisterConfig.REGCODE_DECOY_ACTION or "disable_user").strip().lower()
        await RegCodeOperate.record_regcode_use(code_str, uid=user.UID, telegram_id=user.TELEGRAM_ID, increment=1)
        if action in ("disable_user", "disable_user_and_deactivate_code"):
            user.ACTIVE_STATUS = False
            await UserOperate.update_user(user)
            try:
                await UserService.sync_user_to_emby(user)
            except Exception as exc:  # pragma: no cover
                logger.warning("诱饵卡码触发后同步禁用 Emby 失败: %s", exc)
        if action == "disable_user_and_deactivate_code":
            await RegCodeOperate.deactivate_regcode(code_str)

        logger.warning(
            "诱饵卡码被使用: uid=%s username=%s code=%s action=%s",
            user.UID,
            user.USERNAME,
            code_str,
            action,
        )
        return False, "该卡码无效，账号状态已按站点安全策略处理", None

    @staticmethod
    async def _use_code_unlocked(
        user: UserModel,
        code_str: str,
        emby_username: Optional[str] = None,
        emby_password: Optional[str] = None,
    ) -> Tuple[bool, str, Optional[str]]:
        """
        已登录用户统一使用注册码/续期码/白名单码

        - 注册码(TYPE=1)：为无 Emby 账户的用户创建 Emby 账户
        - 续期码(TYPE=2)：续期
        - 白名单码(TYPE=3)：赋予白名单角色，如果没有 Emby 账户则自动创建

        :return: (成功, 消息, 新Emby密码 或 None)
        """
        from src.db.regcode import RegCodeOperate, Type as RegCodeType

        code_info = await RegCodeOperate.get_regcode_by_code(code_str)
        if not code_info:
            return False, "注册码/续期码无效", None

        if RegCodeOperate.is_decoy(code_info):
            return await UserService._handle_decoy_regcode(user, code_info, code_str)

        if not code_info.ACTIVE:
            return False, "注册码/续期码已停用", None

        if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
            return False, "注册码/续期码已被使用完", None

        if UserService._is_regcode_expired(code_info):
            return False, "注册码/续期码已过期", None

        code_type = code_info.TYPE

        # ========== 续期码 ==========
        if code_type == RegCodeType.RENEW.value:
            success, msg = await UserService.renew_user(user, 30, code_str)
            return success, msg, None

        # ========== 注册码 ==========
        if code_type == RegCodeType.REGISTER.value:
            if user.EMBYID:
                return False, "您已拥有 Emby 账户，无需使用注册码", None

            emby_username = (emby_username or "").strip()
            if not emby_username:
                return False, "使用注册码创建 Emby 账号时，请填写 Emby 用户名", None

            if not is_valid_username(emby_username):
                return False, "Emby 用户名格式不正确（3-20位字母数字下划线，不能以数字开头）", None

            pwd_ok, pwd_msg = UserService._validate_emby_register_password(emby_password)
            if not pwd_ok:
                return False, pwd_msg, None

            # 为已有系统账户的用户创建 Emby 账户
            emby = get_emby_client()
            days = UserService._normalize_code_days(code_info.DAYS, default=30)
            reserved_code = False
            emby_capacity_lock = await UserService.acquire_emby_capacity_lock()
            if emby_capacity_lock is None:
                return False, "Emby 名额检查繁忙，请稍后重试", None

            try:
                if not getattr(user, "PENDING_EMBY", False):
                    cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
                    if not cap_ok:
                        return False, cap_msg, None

                if not await RegCodeOperate.record_regcode_use(
                    code_str,
                    uid=user.UID,
                    telegram_id=user.TELEGRAM_ID,
                ):
                    return False, "注册码/续期码已被使用完", None
                reserved_code = True

                existing_emby = await emby.get_user_by_name(emby_username)
                if existing_emby:
                    await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                    return False, "该 Emby 用户名已被占用", None

                emby_user = await emby.create_user(emby_username, emby_password or "")
                if not emby_user:
                    await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                    return False, "创建 Emby 账户失败", None

                user.EMBYID = emby_user.id
                user.PENDING_EMBY = False
                user.PENDING_EMBY_DAYS = None
                user.ACTIVE_STATUS = True
                if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
                    user.EXPIRED_AT = 253402214400
                else:
                    user.EXPIRED_AT = -1 if days <= 0 else (timestamp() + days_to_seconds(days))

                other_data = {}
                if user.OTHER:
                    try:
                        other_data = json.loads(user.OTHER)
                    except (json.JSONDecodeError, TypeError):
                        other_data = {}
                other_data["emby_username"] = emby_username
                user.OTHER = json.dumps(other_data)
                await UserOperate.update_user(user)

                logger.info(f"注册码创建 Emby 账户成功: system={user.USERNAME}, emby={emby_username}")
                return True, f"Emby 账户创建成功！有效期 {UserService._format_days_text(days)}", None
            except EmbyError as e:
                if reserved_code:
                    await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                logger.error(f"注册码创建 Emby 账户失败: {e}")
                return False, f"Emby 服务器错误: {e}", None
            finally:
                await UserService.release_emby_capacity_lock(emby_capacity_lock)

        # ========== 白名单码 ==========
        if code_type == RegCodeType.WHITELIST.value:
            created_emby_account = False
            reserved_code = False

            if not await RegCodeOperate.record_regcode_use(
                code_str,
                uid=user.UID,
                telegram_id=user.TELEGRAM_ID,
            ):
                return False, "注册码/续期码已被使用完", None
            reserved_code = True

            # 如果没有 Emby 账户，自动创建
            if not user.EMBYID:
                emby_capacity_lock = await UserService.acquire_emby_capacity_lock()
                if emby_capacity_lock is None:
                    await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                    return False, "Emby 名额检查繁忙，请稍后重试", None
                try:
                    if not getattr(user, "PENDING_EMBY", False):
                        cap_ok, cap_msg = await UserService.check_emby_user_capacity(exclude_uid=user.UID)
                        if not cap_ok:
                            await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                            return False, cap_msg, None

                    emby_username = (emby_username or "").strip()
                    if not emby_username:
                        await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                        return False, "使用白名单码创建 Emby 账号时，请填写 Emby 用户名", None

                    if not is_valid_username(emby_username):
                        await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                        return False, "Emby 用户名格式不正确（3-20位字母数字下划线，不能以数字开头）", None

                    pwd_ok, pwd_msg = UserService._validate_emby_register_password(emby_password)
                    if not pwd_ok:
                        await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                        return False, pwd_msg, None

                    emby = get_emby_client()
                    existing_emby = await emby.get_user_by_name(emby_username)
                    if existing_emby:
                        await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                        return False, "该 Emby 用户名已被占用", None

                    emby_user = await emby.create_user(emby_username, emby_password or "")
                    if not emby_user:
                        await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                        return False, "创建 Emby 账户失败", None

                    user.EMBYID = emby_user.id
                    user.PENDING_EMBY = False
                    user.PENDING_EMBY_DAYS = None
                    created_emby_account = True
                    other_data = {}
                    if user.OTHER:
                        try:
                            other_data = json.loads(user.OTHER)
                        except (json.JSONDecodeError, TypeError):
                            other_data = {}
                    other_data["emby_username"] = emby_username
                    user.OTHER = json.dumps(other_data)
                except EmbyError as e:
                    if reserved_code:
                        await RegCodeOperate.record_regcode_use(code_str, increment=-1)
                    logger.error(f"白名单码创建 Emby 账户失败: {e}")
                    return False, f"Emby 服务器错误: {e}", None
                finally:
                    await UserService.release_emby_capacity_lock(emby_capacity_lock)

            # 赋予白名单角色 + 永久有效期
            user.ROLE = Role.WHITE_LIST.value
            user.ACTIVE_STATUS = True
            user.EXPIRED_AT = 253402214400  # 9999-12-31
            await UserOperate.update_user(user)

            msg = "白名单授权成功！已获得永久有效期"
            if created_emby_account:
                msg += "，Emby 账户已创建"
            logger.info(f"白名单码激活: {user.USERNAME}")
            return True, msg, None

        return False, "未知的注册码/续期码类型", None

    @staticmethod
    async def reset_password(
        user: UserModel,
        *,
        scope: str = "both",
        custom_password: Optional[str] = None,
    ) -> Tuple[bool, str, Optional[str]]:
        """管理员重置用户密码，可按作用域拆分系统密码 / Emby 密码。

        :param scope: 取值 ``system`` / ``emby`` / ``both``。
            - ``system``：仅重置本站登录密码，不触碰 Emby；用户没绑 Emby 时也能用。
            - ``emby``：仅重置 Emby 密码，本站登录密码保持不变；前提是用户有 EMBYID。
            - ``both``（默认，保持向后兼容）：两边都重置成同一个密码。
        :param custom_password: 管理员显式指定的新密码；为空时自动生成 12 位强密码。
            指定时需通过 ``validate_password_strength`` 校验。
        :return: ``(success, message, new_password)``；
            ``new_password`` 是本次实际写入的明文（自动生成时返回给前端展示一次）。
        """
        scope = (scope or "both").strip().lower()
        if scope not in ("system", "emby", "both"):
            return False, f"不支持的 scope: {scope}", None

        if scope in ("emby", "both") and not user.EMBYID:
            if scope == "emby":
                return False, "用户没有关联的 Emby 账户", None
            # scope=both 但没绑 Emby：降级为只重置系统密码，避免管理员一个按钮卡死
            scope = "system"

        # 准备密码：admin 显式指定 > 自动生成
        if custom_password:
            ok, msg = UserService.validate_password_strength(custom_password, label="新密码")
            if not ok:
                return False, msg, None
            new_password = custom_password
        else:
            new_password = generate_password(12)

        emby_done = False
        try:
            if scope in ("emby", "both"):
                emby = get_emby_client()
                # 先重置再设新密码——Emby 不支持直接 set 已有密码到新值
                await emby.reset_user_password(user.EMBYID)
                ok = await emby.set_user_password(user.EMBYID, new_password)
                if not ok:
                    return False, "Emby 密码重置失败", None
                emby_done = True

            if scope in ("system", "both"):
                user.PASSWORD = hash_password(new_password)
                await UserOperate.update_user(user)

            label = {
                "system": "系统",
                "emby": "Emby",
                "both": "系统 + Emby",
            }[scope]
            logger.info(f"密码已重置: {user.USERNAME} (scope={scope})")
            return True, f"{label}密码重置成功", new_password
        except Exception as e:
            # 失败时不回滚 Emby 改动（Emby 一旦改了就改了），但会把状态写进日志，
            # 方便管理员手动复核。系统侧的 user.PASSWORD 还没 commit 就不会落库。
            logger.error(
                f"重置密码失败: {e} (scope={scope}, emby_done={emby_done})",
                exc_info=True,
            )
            return False, f"重置失败: {e}", None

    @staticmethod
    async def change_password(user: UserModel, old_password: str, new_password: str) -> Tuple[bool, str]:
        """
        修改用户密码（同时修改系统密码和 Emby 密码）

        :return: (成功, 消息)
        """
        from src.core.utils import verify_password as _verify

        # 验证旧密码
        if not user.PASSWORD or not _verify(old_password, user.PASSWORD):
            return False, "当前密码错误"

        ok, msg = UserService.validate_password_strength(new_password, label="新密码")
        if not ok:
            return False, msg

        try:
            # 更新系统密码
            user.PASSWORD = hash_password(new_password)
            await UserOperate.update_user(user)

            # 同步修改 Emby 密码
            if user.EMBYID:
                emby = get_emby_client()
                # 先重置再设定新密码
                await emby.reset_user_password(user.EMBYID)
                await emby.set_user_password(user.EMBYID, new_password)

            logger.info(f"用户修改密码成功: {user.USERNAME}")
            return True, "密码修改成功"
        except Exception as e:
            logger.error(f"修改密码失败: {e}")
            return False, f"修改密码失败: {e}"

    @staticmethod
    async def change_system_password(user: UserModel, old_password: str, new_password: str) -> Tuple[bool, str]:
        """
        修改系统登录密码（不影响 Emby 密码）

        :return: (成功, 消息)
        """
        from src.core.utils import verify_password as _verify

        if not user.PASSWORD or not _verify(old_password, user.PASSWORD):
            return False, "当前密码错误"

        ok, msg = UserService.validate_password_strength(new_password, label="新密码")
        if not ok:
            return False, msg

        try:
            user.PASSWORD = hash_password(new_password)
            await UserOperate.update_user(user)
            logger.info(f"系统密码修改成功: {user.USERNAME}")
            return True, "系统密码修改成功"
        except Exception as e:
            logger.error(f"修改系统密码失败: {e}")
            return False, f"修改系统密码失败: {e}"

    @staticmethod
    async def change_emby_password(user: UserModel, new_password: str) -> Tuple[bool, str]:
        """
        修改 Emby 密码（仅更新绑定的 Emby 账户）

        :return: (成功, 消息)
        """
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"

        ok, msg = UserService.validate_password_strength(new_password, label="新密码")
        if not ok:
            return False, msg

        try:
            emby = get_emby_client()
            await emby.reset_user_password(user.EMBYID)
            await emby.set_user_password(user.EMBYID, new_password)
            logger.info(f"Emby 密码修改成功: {user.USERNAME}")
            return True, "Emby 密码修改成功"
        except Exception as e:
            logger.error(f"修改 Emby 密码失败: {e}")
            return False, f"修改 Emby 密码失败: {e}"

    @staticmethod
    async def sync_user_to_emby(user: UserModel) -> Tuple[bool, str]:
        """同步用户启用/禁用状态到 Emby。"""
        if not user.EMBYID:
            return True, "用户未绑定 Emby 账户，跳过同步"

        try:
            emby = get_emby_client()
            emby_enabled = UserService.should_enable_emby_access(user)
            await emby.set_user_enabled(user.EMBYID, emby_enabled)
            logger.info(
                f"用户状态已同步到 Emby: {user.USERNAME} (UID: {user.UID}), "
                f"状态: {'启用' if emby_enabled else '禁用'}"
            )
            return True, "同步成功"
        except Exception as e:
            logger.error(f"同步用户状态到 Emby 失败: {e}", exc_info=True)
            return False, f"同步失败: {e}"

    @staticmethod
    def get_cached_telegram_username(user: UserModel) -> Optional[str]:
        """从 user.OTHER JSON 读取上次缓存的 Telegram username（不含 @）。"""
        if not user or not user.OTHER:
            return None
        try:
            data = json.loads(user.OTHER)
        except (json.JSONDecodeError, TypeError):
            return None
        if not isinstance(data, dict):
            return None
        value = data.get("telegram_username")
        if isinstance(value, str) and value.strip():
            return value.strip().lstrip("@")
        return None

    @staticmethod
    async def cache_telegram_username(
        user: UserModel,
        username: Optional[str],
    ) -> bool:
        """把 Telegram username 写进 user.OTHER 缓存。返回 True 表示有变更。

        Bot 端在 `/bind /start /me` 等命令里有 `update.effective_user.username`，
        每次都调一下这个方法，admin 列表端就能从 DB 直接读到 username，
        避免在每次刷新列表时对几百个用户重复打 `bot.get_chat()`。
        """
        if user is None:
            return False
        normalized = (username or "").strip().lstrip("@") or None

        try:
            data = json.loads(user.OTHER) if user.OTHER else {}
        except (json.JSONDecodeError, TypeError):
            data = {}
        if not isinstance(data, dict):
            data = {}

        current = data.get("telegram_username") or None
        if (current or None) == (normalized or None):
            return False

        if normalized:
            data["telegram_username"] = normalized
        else:
            data.pop("telegram_username", None)

        user.OTHER = json.dumps(data) if data else ""
        await UserOperate.update_user(user)
        return True

    @staticmethod
    async def get_user_info(user: UserModel) -> dict:
        """获取用户详细信息"""
        from src.core.utils import format_expire_time, mask_email

        # 角色名称映射
        role_name_map = {
            Role.ADMIN.value: "管理员",
            Role.NORMAL.value: "普通用户",
            Role.WHITE_LIST.value: "白名单",
            Role.UNRECOGNIZED.value: "未注册",
        }
        role_name = role_name_map.get(user.ROLE, "未知")

        embay_username = None
        if user.OTHER:
            try:
                other_data = json.loads(user.OTHER)
                embay_username = other_data.get("emby_username")
            except (json.JSONDecodeError, TypeError):
                embay_username = None

        if not embay_username and user.EMBYID:
            embay_username = user.USERNAME

        is_pending_emby = bool(getattr(user, "PENDING_EMBY", False)) and not user.EMBYID
        # 未绑定 Emby（无 EMBYID 或处于 pending）时，覆盖默认的 expire_status 文案，
        # 避免展示"已过期"/"剩余 x"误导，同时让前端可以靠这串直接判断渲染。
        emby_disabled_by_expiry = UserService.is_emby_access_expired(user)
        if is_pending_emby or not user.EMBYID:
            expire_status = "未绑定 Emby"
        elif emby_disabled_by_expiry:
            expire_status = "已到期，Emby 已禁用"
        else:
            expire_status = format_expire_time(user.EXPIRED_AT)

        emby_bound = bool(user.EMBYID) and not is_pending_emby
        bgm_token_set = bool((user.BGM_TOKEN or "").strip())
        info = {
            "uid": user.UID,
            "username": user.USERNAME,
            "telegram_id": user.TELEGRAM_ID,
            "email": mask_email(user.EMAIL) if user.EMAIL else None,
            "role": user.ROLE,  # 保留数字角色
            "role_name": role_name,  # 添加角色名称
            "active": user.ACTIVE_STATUS,
            "expire_status": expire_status,
            # 未绑定 Emby 时不下发 EXPIRED_AT 数值，避免 sentinel(0) 被 UI 误解
            "expired_at": user.EXPIRED_AT if emby_bound else None,
            "bgm_mode": user.BGM_MODE,
            "bgm_token_set": bgm_token_set,
            "bgm_sync_ready": bool(user.BGM_MODE and bgm_token_set),
            "avatar": user.AVATAR or None,
            "register_time": user.REGISTER_TIME,
            "created_at": user.CREATE_AT or user.REGISTER_TIME,  # 前端兼容字段
            "emby_id": user.EMBYID,  # 添加 Emby ID
            "emby_username": embay_username,
            "emby_bound": emby_bound,
            "pending_emby": is_pending_emby,
            "pending_emby_days": getattr(user, "PENDING_EMBY_DAYS", None),
            "emby_disabled_by_expiry": emby_disabled_by_expiry,
            "library_self_service": bool(getattr(user, "LIBRARY_SELF_SERVICE", False)),
        }

        return info

    @staticmethod
    async def change_username(user: UserModel, new_username: str) -> Tuple[bool, str]:
        """
        修改用户名

        同时修改 Emby 和本地用户名
        """
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"

        emby = get_emby_client()

        try:
            # 检查新用户名是否已存在
            existing = await emby.get_user_by_name(new_username)
            if existing and existing.id != user.EMBYID:
                return False, "该用户名已被使用"

            # 获取当前 Emby 用户信息
            emby_user = await emby.get_user(user.EMBYID)
            if not emby_user:
                return False, "Emby 用户不存在"

            # 更新 Emby 用户名
            success = await emby.update_user(user.EMBYID, {"Name": new_username})
            if not success:
                return False, "更新 Emby 用户名失败"

            # 更新本地用户名
            old_username = user.USERNAME
            user.USERNAME = new_username
            import json

            other_data = {}
            if user.OTHER:
                try:
                    other_data = json.loads(user.OTHER)
                except (json.JSONDecodeError, TypeError):
                    other_data = {}
            other_data["emby_username"] = new_username
            user.OTHER = json.dumps(other_data)
            await UserOperate.update_user(user)

            logger.info(f"用户名已修改: {old_username} -> {new_username}")
            return True, "用户名修改成功"
        except EmbyError as e:
            logger.error(f"修改用户名失败: {e}")
            return False, f"修改失败: {e}"

    @staticmethod
    async def set_user_admin(user: UserModel, is_admin: bool) -> Tuple[bool, str]:
        """设置用户管理员权限"""
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"

        emby = get_emby_client()

        try:
            success = await emby.set_user_admin(user.EMBYID, is_admin)
            if success:
                user.ROLE = Role.ADMIN.value if is_admin else Role.NORMAL.value
                await UserOperate.update_user(user)

                status = "授予" if is_admin else "撤销"
                return True, f"已{status}管理员权限"
            return False, "操作失败"
        except EmbyError as e:
            logger.error(f"设置管理员权限失败: {e}")
            return False, f"操作失败: {e}"

    @staticmethod
    async def create_whitelist_user(telegram_id: int, username: str, email: Optional[str] = None) -> RegisterResponse:
        """创建白名单用户（永久有效）"""
        emby_capacity_lock = await UserService.acquire_emby_capacity_lock()
        if emby_capacity_lock is None:
            return RegisterResponse(RegisterResult.ERROR, "Emby 名额检查繁忙，请稍后重试")

        try:
            cap_ok, cap_msg = await UserService.check_emby_user_capacity()
            if not cap_ok:
                return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, cap_msg)

            # 检查用户是否已存在
            existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
            if existing_user and existing_user.EMBYID:
                return RegisterResponse(RegisterResult.USER_EXISTS, "用户已存在")

            emby = get_emby_client()

            # 检查 Emby 用户名
            existing_emby = await emby.get_user_by_name(username)
            if existing_emby:
                return RegisterResponse(RegisterResult.EMBY_EXISTS, "Emby 用户名已存在")

            # 创建 Emby 用户
            from src.core.utils import generate_password

            password = generate_password(12)
            emby_user = await emby.create_user(username, password)

            if not emby_user:
                return RegisterResponse(RegisterResult.EMBY_ERROR, "创建 Emby 账户失败")

            # 创建本地用户（永久有效）
            new_uid = await UserOperate.get_new_uid()
            now_ts = timestamp()
            user = UserModel(
                UID=new_uid,
                TELEGRAM_ID=telegram_id,
                USERNAME=username,
                EMAIL=email,
                EMBYID=emby_user.id,
                PASSWORD=hash_password(password),
                ROLE=Role.WHITE_LIST.value,
                EXPIRED_AT=-1,  # 永不过期
                CREATE_AT=now_ts,
                REGISTER_TIME=now_ts,
            )
            await UserOperate.add_user(user)

            logger.info(f"白名单用户创建成功: {username}")

            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message="白名单用户创建成功（永久有效）",
                user=user,
                emby_password=password,
            )
        except EmbyError as e:
            logger.error(f"创建白名单用户失败: {e}")
            return RegisterResponse(RegisterResult.EMBY_ERROR, f"Emby 错误: {e}")
        finally:
            await UserService.release_emby_capacity_lock(emby_capacity_lock)
