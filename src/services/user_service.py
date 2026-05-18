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

from src.config import Config, RegisterConfig
from src.db.user import UserModel, UserOperate, Role, TelegramRebindRequestOperate
from src.services.emby import get_emby_client, EmbyError
from src.core.utils import generate_password, hash_password, timestamp, days_to_seconds, is_valid_username
from src.core.registration_lock import (
    acquire_global_registration_lock,
    acquire_registration_lock,
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
    async def create_telegram_rebind_request(user: UserModel, reason: Optional[str] = None) -> Tuple[bool, str, Optional[object]]:
        if not user.TELEGRAM_ID:
            return False, "当前账号尚未绑定 Telegram"

        existing = await TelegramRebindRequestOperate.get_request_by_uid(user.UID)
        if existing and existing.STATUS == 'pending':
            return False, "您已有待处理的换绑请求，请等待管理员处理"

        request = await TelegramRebindRequestOperate.create_request(user.UID, user.TELEGRAM_ID, reason)
        return True, "换绑请求已提交，管理员审核后您可以重新绑定 Telegram", request

    @staticmethod
    async def list_telegram_rebind_requests(status: Optional[str] = None, page: int = 1, per_page: int = 20):
        return await TelegramRebindRequestOperate.list_requests(status=status, page=page, per_page=per_page)

    @staticmethod
    async def approve_telegram_rebind_request(request_id: int, reviewer_uid: int, admin_note: Optional[str] = None) -> Tuple[bool, str]:
        request = await TelegramRebindRequestOperate.get_request_by_id(request_id)
        if not request:
            return False, "换绑请求不存在"
        if request.STATUS != 'pending':
            return False, "该请求已处理"

        user = await UserOperate.get_user_by_uid(request.UID)
        if not user:
            return False, "关联用户不存在"

        if user.TELEGRAM_ID:
            await UserOperate.unbind_telegram_user(user)

        success = await TelegramRebindRequestOperate.update_request_status(
            request_id,
            'approved',
            reviewer_uid=reviewer_uid,
            admin_note=admin_note,
        )
        if not success:
            return False, "更新请求状态失败"

        return True, "已批准换绑请求并解绑当前 Telegram，用户可重新绑定新账号"

    @staticmethod
    async def reject_telegram_rebind_request(request_id: int, reviewer_uid: int, admin_note: Optional[str] = None) -> Tuple[bool, str]:
        request = await TelegramRebindRequestOperate.get_request_by_id(request_id)
        if not request:
            return False, "换绑请求不存在"
        if request.STATUS != 'pending':
            return False, "该请求已处理"

        success = await TelegramRebindRequestOperate.update_request_status(
            request_id,
            'rejected',
            reviewer_uid=reviewer_uid,
            admin_note=admin_note,
        )
        if not success:
            return False, "更新请求状态失败"

        return True, "已拒绝换绑请求"

    @staticmethod
    async def register_by_code(
        telegram_id: Optional[int],
        username: str,
        reg_code: str,
        email: Optional[str] = None,
        password: Optional[str] = None
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
        )

        # 注册码消费：只在系统账号创建成功时计数
        if response.result == RegisterResult.SUCCESS:
            await RegCodeOperate.update_regcode_use_count(reg_code, 1)
            # 重新组装一份更贴合"已使用注册码"的提示文案
            days_text = "永久" if pending_days <= 0 else f"{pending_days} 天"
            response.message = (
                f"注册成功！注册码已使用，Emby 账号开通时长 {days_text}，请在首次登录后补建 Emby 账号。"
            )

        return response

    @staticmethod
    async def register_direct_emby(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str] = None,
        password: Optional[str] = None,
    ) -> RegisterResponse:
        """直接创建 Emby 账号（用于 Emby 自由注册队列）。"""
        if not RegisterConfig.EMBY_DIRECT_REGISTER_ENABLED:
            return RegisterResponse(RegisterResult.ERROR, "Emby 自由注册未开启")

        if not telegram_id:
            return RegisterResponse(RegisterResult.TELEGRAM_NOT_BOUND, "请先完成 Telegram 绑定")

        locks = await acquire_registration_lock(username, telegram_id)
        if locks is None:
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        try:
            available, msg = await UserService.check_registration_available(use_cache=False)
            if not available:
                return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, msg)

            existing_tg = await UserOperate.get_user_by_telegram_id(telegram_id)
            if existing_tg and existing_tg.EMBYID:
                return RegisterResponse(RegisterResult.USER_EXISTS, "该 Telegram 账号已绑定 Emby 账户")

            existing_name = await UserOperate.get_user_by_username(username)
            if existing_name and existing_name.EMBYID:
                return RegisterResponse(RegisterResult.USER_EXISTS, "该用户名已被占用")
            if existing_name and existing_name.TELEGRAM_ID and existing_name.TELEGRAM_ID != telegram_id:
                return RegisterResponse(RegisterResult.USER_EXISTS, "该用户名已被占用")

            return await UserService._create_emby_user(
                telegram_id=telegram_id,
                username=username,
                email=email,
                days=max(int(RegisterConfig.EMBY_DIRECT_REGISTER_DAYS or 30), 1),
                password=password,
            )
        finally:
            await release_registration_lock(locks)

    @staticmethod
    async def register_pending(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str] = None,
        password: Optional[str] = None,
        pending_emby_days: Optional[int] = None,
        skip_pending_check: bool = False,
    ) -> RegisterResponse:
        """
        待激活注册（仅创建系统账号，未绑定 Emby）。

        :param pending_emby_days: 注册码授予的开通天数；为 None 时由 Emby 注册流程使用默认值
        :param skip_pending_check: 走注册码路径时不需要再校验 ALLOW_PENDING_REGISTER
        """
        from src.config import RegisterConfig

        # 检查是否允许无码注册（除非来自注册码路径）
        if not skip_pending_check and not RegisterConfig.ALLOW_PENDING_REGISTER:
            return RegisterResponse(RegisterResult.ERROR, "暂不开放注册，请使用注册码")

        locks = await acquire_registration_lock(username, telegram_id)
        if locks is None:
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        global_lock = await acquire_global_registration_lock()
        if global_lock is None:
            await release_registration_lock(locks)
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        try:
            # 二次校验（避免锁竞争窗口）
            if not skip_pending_check and not RegisterConfig.ALLOW_PENDING_REGISTER:
                return RegisterResponse(RegisterResult.ERROR, "暂不开放注册，请使用注册码")

            # 检查注册是否开放
            available, msg = await UserService.check_registration_available(use_cache=False)
            if not available:
                return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, msg)
            
            # 检查用户名是否已存在
            existing = await UserOperate.get_user_by_username(username)
            if existing:
                return RegisterResponse(RegisterResult.USER_EXISTS, "用户名已被使用")
            
            # 如果有 telegram_id，检查是否已注册
            if telegram_id:
                existing_tg = await UserOperate.get_user_by_telegram_id(telegram_id)
                if existing_tg:
                    return RegisterResponse(RegisterResult.USER_EXISTS, "该 Telegram 账号已注册")
            
            # 先获取新 UID
            new_uid = await UserOperate.get_new_uid()
            user_password = password if password else generate_password(12)
            
            # 检查是否是预设管理员或白名单（优先使用 UID，其次使用用户名）
            is_admin = False
            is_whitelist = False
            
            # 先检查管理员 UID 列表
            admin_uids = RegisterConfig.ADMIN_UIDS
            if admin_uids:
                uid_list = [int(u.strip()) for u in admin_uids.split(',') if u.strip().isdigit()]
                is_admin = new_uid in uid_list
            
            # 如果 UID 未匹配，再检查管理员用户名列表
            if not is_admin:
                admin_usernames = RegisterConfig.ADMIN_USERNAMES
                if admin_usernames:
                    name_list = [n.strip().lower() for n in admin_usernames.split(',') if n.strip()]
                    is_admin = username.lower() in name_list
            
            # 检查白名单 UID 列表
            if not is_admin:
                whitelist_uids = RegisterConfig.WHITE_LIST_UIDS
                if whitelist_uids:
                    uid_list = [int(u.strip()) for u in whitelist_uids.split(',') if u.strip().isdigit()]
                    is_whitelist = new_uid in uid_list
            
            # 如果 UID 未匹配，再检查白名单用户名列表
            if not is_admin and not is_whitelist:
                whitelist_usernames = RegisterConfig.WHITE_LIST_USERNAMES
                if whitelist_usernames:
                    name_list = [n.strip().lower() for n in whitelist_usernames.split(',') if n.strip()]
                    is_whitelist = username.lower() in name_list
            
            # 9999-12-31 的时间戳（管理员和白名单使用）
            permanent_expire = 253402214400

            # 管理员默认激活，到期时间为 9999 年
            if is_admin:
                user = UserModel(
                    UID=new_uid,
                    TELEGRAM_ID=telegram_id,
                    USERNAME=username,
                    EMAIL=email,
                    EMBYID=None,  # 稍后创建 Emby 账户
                    PASSWORD=hash_password(user_password),
                    ROLE=Role.ADMIN.value,
                    ACTIVE_STATUS=True,  # 管理员默认激活
                    EXPIRED_AT=permanent_expire,
                    REGISTER_TIME=timestamp(),
                    PENDING_EMBY=True,
                    PENDING_EMBY_DAYS=pending_emby_days,
                )
            elif is_whitelist:
                # 白名单用户默认激活，到期时间为 9999 年
                user = UserModel(
                    UID=new_uid,
                    TELEGRAM_ID=telegram_id,
                    USERNAME=username,
                    EMAIL=email,
                    EMBYID=None,  # 稍后创建 Emby 账户
                    PASSWORD=hash_password(user_password),
                    ROLE=Role.WHITE_LIST.value,
                    ACTIVE_STATUS=True,
                    EXPIRED_AT=permanent_expire,
                    REGISTER_TIME=timestamp(),
                    PENDING_EMBY=True,
                    PENDING_EMBY_DAYS=pending_emby_days,
                )
            else:
                # 普通用户：已激活但无 Emby 账户
                user = UserModel(
                    UID=new_uid,
                    TELEGRAM_ID=telegram_id,
                    USERNAME=username,
                    EMAIL=email,
                    EMBYID=None,  # 无 Emby 账户
                    PASSWORD=hash_password(user_password),
                    ROLE=Role.NORMAL.value,
                    ACTIVE_STATUS=True,  # 账户激活，可以登录
                    EXPIRED_AT=-1,
                    REGISTER_TIME=timestamp(),
                    PENDING_EMBY=True,
                    PENDING_EMBY_DAYS=pending_emby_days,
                )
            await UserOperate.add_user(user)

            logger.info(
                f"待激活用户注册: {username} (UID: {new_uid}, pending_emby_days={pending_emby_days})"
            )

            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message="注册成功！您可以登录并使用基础功能。",
                user=user,
                emby_password=user_password if not password else None
            )
        finally:
            await release_global_registration_lock(global_lock)
            await release_registration_lock(locks)

    @staticmethod
    async def complete_emby_registration(
        user: UserModel,
        emby_username: str,
        emby_password: str,
    ) -> RegisterResponse:
        """已登录用户补建 Emby 账号；失败保留 PENDING_EMBY 标记便于重试。"""
        from src.config import RegisterConfig
        import json

        if user.EMBYID:
            return RegisterResponse(RegisterResult.USER_EXISTS, "您已绑定 Emby 账号")

        emby_username = (emby_username or '').strip()
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

        locks = await acquire_registration_lock(emby_username, user.TELEGRAM_ID)
        if locks is None:
            return RegisterResponse(RegisterResult.ERROR, "当前注册请求较多，请稍后重试")

        emby = get_emby_client()
        try:
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

            days = user.PENDING_EMBY_DAYS
            if days is None:
                days = int(RegisterConfig.EMBY_DIRECT_REGISTER_DAYS or 30)
            days = UserService._normalize_code_days(days, default=30)

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
            other_data['emby_username'] = emby_username
            user.OTHER = json.dumps(other_data)

            await UserOperate.update_user(user)

            # 把启用/到期状态同步到 Emby（启用 Policy 等）
            try:
                await UserService.sync_user_to_emby(user)
            except Exception as exc:  # pragma: no cover
                logger.warning(f"补建 Emby 后同步状态失败: {exc}")

            days_text = UserService._format_days_text(days)
            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message=f"Emby 账号已创建并绑定，开通时长 {days_text}",
                user=user,
            )
        finally:
            await release_registration_lock(locks)

    @staticmethod
    async def _create_emby_user(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str],
        days: int,
        reg_code: Optional[str] = None,
        password: Optional[str] = None
    ) -> RegisterResponse:
        """创建 Emby 用户（内部方法）"""
        emby = get_emby_client()
        
        try:
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
            
            # 创建或更新本地用户记录
            existing_user = None
            if telegram_id:
                existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
            
            if existing_user:
                existing_user.EMBYID = emby_user.id
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
                existing_user.REGISTER_TIME = timestamp()
                import json
                other_data = {}
                if existing_user.OTHER:
                    try:
                        other_data = json.loads(existing_user.OTHER)
                    except (json.JSONDecodeError, TypeError):
                        other_data = {}
                other_data['emby_username'] = username
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
                    uid_list = [int(u.strip()) for u in admin_uids.split(',') if u.strip().isdigit()]
                    is_admin = new_uid in uid_list
                if not is_admin:
                    admin_usernames = RegisterConfig.ADMIN_USERNAMES
                    if admin_usernames:
                        name_list = [n.strip().lower() for n in admin_usernames.split(',') if n.strip()]
                        is_admin = username.lower() in name_list
                
                # 检查白名单
                if not is_admin:
                    whitelist_uids = RegisterConfig.WHITE_LIST_UIDS
                    if whitelist_uids:
                        uid_list = [int(u.strip()) for u in whitelist_uids.split(',') if u.strip().isdigit()]
                        is_whitelist = new_uid in uid_list
                if not is_admin and not is_whitelist:
                    whitelist_usernames = RegisterConfig.WHITE_LIST_USERNAMES
                    if whitelist_usernames:
                        name_list = [n.strip().lower() for n in whitelist_usernames.split(',') if n.strip()]
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
                
                user = UserModel(
                    UID=new_uid,
                    TELEGRAM_ID=telegram_id,  # 可以为 None
                    USERNAME=username,
                    EMAIL=email,
                    EMBYID=emby_user.id,
                    PASSWORD=hash_password(user_password),
                    ROLE=user_role,
                    EXPIRED_AT=user_expire,
                    REGISTER_TIME=timestamp(),
                    OTHER=json.dumps({'emby_username': username}),
                )
                await UserOperate.add_user(user)

            # 更新注册码使用记录
            if reg_code:
                from src.db.regcode import RegCodeOperate
                await RegCodeOperate.update_regcode_use_count(reg_code, 1)

            logger.info(f"用户注册成功: {username} (TG: {telegram_id})")
            
            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message=f"注册成功！有效期 {days} 天",
                user=user,
                emby_password=user_password if not password else None  # 仅自动生成时返回
            )
            
        except EmbyError as e:
            logger.error(f"Emby 错误: {e}")
            return RegisterResponse(RegisterResult.EMBY_ERROR, f"Emby 服务器错误: {e}")
        except Exception as e:
            logger.error(f"注册错误: {e}")
            return RegisterResponse(RegisterResult.ERROR, f"注册失败: {e}")

    @staticmethod
    async def renew_user(
        user: UserModel,
        days: int,
        reg_code: Optional[str] = None
    ) -> Tuple[bool, str]:
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
            
            if code_info.TYPE != RegCodeType.RENEW.value:
                return False, "这不是续期码"
            
            if not code_info.ACTIVE:
                return False, "续期码已停用"
            
            if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
                return False, "续期码已被使用完"
            
            days = UserService._normalize_code_days(code_info.DAYS, default=days)
            await RegCodeOperate.update_regcode_use_count(reg_code, 1)

        if days <= 0:
            user.EXPIRED_AT = -1
            await UserOperate.update_user(user)
        else:
            await UserOperate.renew_user_expire_time(user, days)
        
        # 如果用户被禁用，重新启用
        if not user.ACTIVE_STATUS:
            user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)
            
            # 同时启用 Emby 账户
            if user.EMBYID:
                emby = get_emby_client()
                await emby.set_user_enabled(user.EMBYID, True)
        
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
                await emby.set_user_enabled(user.EMBYID, True)
            
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
                other_data.pop('emby_username', None)
                user.OTHER = json.dumps(other_data) if other_data else ''
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
        
        if not code_info.ACTIVE:
            return False, "注册码/续期码已停用", None
        
        if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
            return False, "注册码/续期码已被使用完", None
        
        # 检查有效期
        if code_info.VALIDITY_TIME != -1:
            expire_time = code_info.CREATED_TIME + code_info.VALIDITY_TIME * 3600
            if timestamp() > expire_time:
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

            emby_username = (emby_username or '').strip()
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
            
            try:
                existing_emby = await emby.get_user_by_name(emby_username)
                if existing_emby:
                    return False, "该 Emby 用户名已被占用", None
                
                emby_user = await emby.create_user(emby_username, emby_password or '')
                if not emby_user:
                    return False, "创建 Emby 账户失败", None
                
                user.EMBYID = emby_user.id
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
                other_data['emby_username'] = emby_username
                user.OTHER = json.dumps(other_data)
                await UserOperate.update_user(user)
                
                await RegCodeOperate.update_regcode_use_count(code_str, 1)
                logger.info(f"注册码创建 Emby 账户成功: system={user.USERNAME}, emby={emby_username}")
                return True, f"Emby 账户创建成功！有效期 {UserService._format_days_text(days)}", None
            except EmbyError as e:
                logger.error(f"注册码创建 Emby 账户失败: {e}")
                return False, f"Emby 服务器错误: {e}", None
        
        # ========== 白名单码 ==========
        if code_type == RegCodeType.WHITELIST.value:
            created_emby_account = False
            
            # 如果没有 Emby 账户，自动创建
            if not user.EMBYID:
                emby_username = (emby_username or '').strip()
                if not emby_username:
                    return False, "使用白名单码创建 Emby 账号时，请填写 Emby 用户名", None

                if not is_valid_username(emby_username):
                    return False, "Emby 用户名格式不正确（3-20位字母数字下划线，不能以数字开头）", None

                pwd_ok, pwd_msg = UserService._validate_emby_register_password(emby_password)
                if not pwd_ok:
                    return False, pwd_msg, None

                emby = get_emby_client()
                
                try:
                    existing_emby = await emby.get_user_by_name(emby_username)
                    if existing_emby:
                        return False, "该 Emby 用户名已被占用", None
                    
                    emby_user = await emby.create_user(emby_username, emby_password or '')
                    if not emby_user:
                        return False, "创建 Emby 账户失败", None
                    
                    user.EMBYID = emby_user.id
                    created_emby_account = True
                    other_data = {}
                    if user.OTHER:
                        try:
                            other_data = json.loads(user.OTHER)
                        except (json.JSONDecodeError, TypeError):
                            other_data = {}
                    other_data['emby_username'] = emby_username
                    user.OTHER = json.dumps(other_data)
                except EmbyError as e:
                    logger.error(f"白名单码创建 Emby 账户失败: {e}")
                    return False, f"Emby 服务器错误: {e}", None
            
            # 赋予白名单角色 + 永久有效期
            user.ROLE = Role.WHITE_LIST.value
            user.ACTIVE_STATUS = True
            user.EXPIRED_AT = 253402214400  # 9999-12-31
            await UserOperate.update_user(user)
            
            await RegCodeOperate.update_regcode_use_count(code_str, 1)
            
            msg = "白名单授权成功！已获得永久有效期"
            if created_emby_account:
                msg += "，Emby 账户已创建"
            logger.info(f"白名单码激活: {user.USERNAME}")
            return True, msg, None
        
        return False, "未知的注册码/续期码类型", None

    @staticmethod
    async def reset_password(user: UserModel) -> Tuple[bool, str, Optional[str]]:
        """
        重置用户密码
        
        :return: (成功, 消息, 新密码)
        """
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户", None
        
        try:
            emby = get_emby_client()
            new_password = generate_password(12)
            
            # 先重置再设置新密码
            await emby.reset_user_password(user.EMBYID)
            success = await emby.set_user_password(user.EMBYID, new_password)
            
            if success:
                user.PASSWORD = hash_password(new_password)
                await UserOperate.update_user(user)
                logger.info(f"密码已重置: {user.USERNAME}")
                return True, "密码重置成功", new_password
            else:
                return False, "密码重置失败", None
        except Exception as e:
            logger.error(f"重置密码失败: {e}")
            return False, f"重置失败: {e}", None

    @staticmethod
    async def change_password(
        user: UserModel, old_password: str, new_password: str
    ) -> Tuple[bool, str]:
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
    async def change_system_password(
        user: UserModel, old_password: str, new_password: str
    ) -> Tuple[bool, str]:
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
            await emby.set_user_enabled(user.EMBYID, user.ACTIVE_STATUS)
            logger.info(
                f"用户状态已同步到 Emby: {user.USERNAME} (UID: {user.UID}), "
                f"状态: {'启用' if user.ACTIVE_STATUS else '禁用'}"
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
        value = data.get('telegram_username')
        if isinstance(value, str) and value.strip():
            return value.strip().lstrip('@')
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
        normalized = (username or '').strip().lstrip('@') or None

        try:
            data = json.loads(user.OTHER) if user.OTHER else {}
        except (json.JSONDecodeError, TypeError):
            data = {}
        if not isinstance(data, dict):
            data = {}

        current = data.get('telegram_username') or None
        if (current or None) == (normalized or None):
            return False

        if normalized:
            data['telegram_username'] = normalized
        else:
            data.pop('telegram_username', None)

        user.OTHER = json.dumps(data) if data else ''
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
                embay_username = other_data.get('emby_username')
            except (json.JSONDecodeError, TypeError):
                embay_username = None

        if not embay_username and user.EMBYID:
            embay_username = user.USERNAME

        info = {
            "uid": user.UID,
            "username": user.USERNAME,
            "telegram_id": user.TELEGRAM_ID,
            "email": mask_email(user.EMAIL) if user.EMAIL else None,
            "role": user.ROLE,  # 保留数字角色
            "role_name": role_name,  # 添加角色名称
            "active": user.ACTIVE_STATUS,
            "expire_status": format_expire_time(user.EXPIRED_AT),
            "expired_at": user.EXPIRED_AT,
            "bgm_mode": user.BGM_MODE,
            "avatar": user.AVATAR or None,
            "register_time": user.REGISTER_TIME,
            "created_at": user.REGISTER_TIME,  # 前端兼容字段
            "emby_id": user.EMBYID,  # 添加 Emby ID
            "emby_username": embay_username,
            "pending_emby": bool(getattr(user, 'PENDING_EMBY', False)) and not user.EMBYID,
            "pending_emby_days": getattr(user, 'PENDING_EMBY_DAYS', None),
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
            success = await emby.update_user(user.EMBYID, {'Name': new_username})
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
            other_data['emby_username'] = new_username
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
    async def create_whitelist_user(
        telegram_id: int,
        username: str,
        email: Optional[str] = None
    ) -> RegisterResponse:
        """创建白名单用户（永久有效）"""
        # 检查用户是否已存在
        existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing_user and existing_user.EMBYID:
            return RegisterResponse(RegisterResult.USER_EXISTS, "用户已存在")
        
        emby = get_emby_client()
        
        try:
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
            user = UserModel(
                UID=new_uid,
                TELEGRAM_ID=telegram_id,
                USERNAME=username,
                EMAIL=email,
                EMBYID=emby_user.id,
                PASSWORD=hash_password(password),
                ROLE=Role.WHITE_LIST.value,
                EXPIRED_AT=-1,  # 永不过期
                REGISTER_TIME=timestamp(),
            )
            await UserOperate.add_user(user)
            
            logger.info(f"白名单用户创建成功: {username}")
            
            return RegisterResponse(
                result=RegisterResult.SUCCESS,
                message="白名单用户创建成功（永久有效）",
                user=user,
                emby_password=password
            )
        except EmbyError as e:
            logger.error(f"创建白名单用户失败: {e}")
            return RegisterResponse(RegisterResult.EMBY_ERROR, f"Emby 错误: {e}")

