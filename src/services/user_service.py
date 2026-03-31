"""
用户业务服务层

处理用户注册、续期、绑定等业务逻辑
"""
import time
import logging
from typing import Optional, Tuple
from dataclasses import dataclass
from enum import Enum

from src.config import Config, ScoreAndRegisterConfig
from src.db.user import UserModel, UserOperate, Role
from src.db.score import ScoreModel, ScoreOperate
from src.services.emby import get_emby_client, EmbyError
from src.core.utils import generate_password, hash_password, timestamp, days_to_seconds

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
    INSUFFICIENT_SCORE = "insufficient_score"
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
    async def check_registration_available() -> Tuple[bool, str]:
        """检查是否可以注册"""
        if not ScoreAndRegisterConfig.REGISTER_MODE:
            return False, "注册功能已关闭"
        
        current_count = await UserOperate.get_registered_users_count()
        if current_count >= ScoreAndRegisterConfig.USER_LIMIT:
            return False, f"已达到用户数量上限 ({ScoreAndRegisterConfig.USER_LIMIT})"
        
        return True, "可以注册"

    @staticmethod
    async def register_by_code(
        telegram_id: Optional[int],
        username: str,
        reg_code: str,
        email: Optional[str] = None,
        password: Optional[str] = None
    ) -> RegisterResponse:
        """
        通过注册码注册
        
        :param telegram_id: Telegram ID（Web 注册时可为空）
        :param username: Emby 用户名
        :param reg_code: 注册码
        :param email: 邮箱（可选）
        :param password: 密码（Web 注册时使用，为空则自动生成）
        """
        from src.db.regcode import RegCodeOperate, Type as RegCodeType
        
        # 检查注册是否开放
        available, msg = await UserService.check_registration_available()
        if not available:
            return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, msg)
        
        # 检查用户是否已存在（有 telegram_id 时检查）
        if telegram_id:
            existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
            if existing_user and existing_user.EMBYID:
                return RegisterResponse(RegisterResult.USER_EXISTS, "您已经注册过了")
        
        # 验证注册码
        code_info = await RegCodeOperate.get_regcode_by_code(reg_code)
        if not code_info:
            return RegisterResponse(RegisterResult.INVALID_CODE, "注册码无效")
        
        if code_info.TYPE != RegCodeType.REGISTER.value:
            return RegisterResponse(RegisterResult.INVALID_CODE, "这不是注册码")
        
        if not code_info.ACTIVE:
            return RegisterResponse(RegisterResult.CODE_EXPIRED, "注册码已停用")
        
        # 检查使用次数
        if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
            return RegisterResponse(RegisterResult.CODE_USED, "注册码已被使用完")
        
        # 检查有效期
        if code_info.VALIDITY_TIME != -1:
            expire_time = code_info.CREATED_TIME + code_info.VALIDITY_TIME * 3600
            if timestamp() > expire_time:
                return RegisterResponse(RegisterResult.CODE_EXPIRED, "注册码已过期")
        
        # 创建 Emby 账户
        return await UserService._create_emby_user(
            telegram_id=telegram_id,
            username=username,
            email=email,
            days=code_info.DAYS or 30,
            reg_code=reg_code,
            password=password
        )

    @staticmethod
    async def register_by_score(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str] = None,
        password: Optional[str] = None
    ) -> RegisterResponse:
        """通过积分注册"""
        if not ScoreAndRegisterConfig.SCORE_REGISTER_MODE:
            return RegisterResponse(RegisterResult.ERROR, "积分注册未开启")
        
        # 积分注册需要 telegram_id
        if not telegram_id:
            return RegisterResponse(RegisterResult.ERROR, "积分注册需要绑定 Telegram")
        
        # 检查注册是否开放
        available, msg = await UserService.check_registration_available()
        if not available:
            return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, msg)
        
        # 检查用户是否已存在
        existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
        if existing_user and existing_user.EMBYID:
            return RegisterResponse(RegisterResult.USER_EXISTS, "您已经注册过了")
        
        # 检查积分
        score_record = await ScoreOperate.get_score_by_telegram_id(telegram_id)
        needed = ScoreAndRegisterConfig.SCORE_REGISTER_NEED
        
        if not score_record or score_record.SCORE < needed:
            current = score_record.SCORE if score_record else 0
            return RegisterResponse(
                RegisterResult.INSUFFICIENT_SCORE,
                f"积分不足，需要 {needed} {ScoreAndRegisterConfig.SCORE_NAME}，当前 {current}"
            )
        
        # 扣除积分
        score_record.SCORE -= needed
        await ScoreOperate.update_score(score_record)
        
        # 创建 Emby 账户
        return await UserService._create_emby_user(
            telegram_id=telegram_id,
            username=username,
            email=email,
            days=30,
            password=password
        )

    @staticmethod
    async def register_pending(
        telegram_id: Optional[int],
        username: str,
        email: Optional[str] = None,
        password: Optional[str] = None
    ) -> RegisterResponse:
        """
        无码注册（待激活状态）
        
        用户注册后不创建 Emby 账户，只能签到赚积分。
        积分够了可以使用积分激活账户。
        """
        from src.config import ScoreAndRegisterConfig
        
        # 检查是否允许无码注册
        if not ScoreAndRegisterConfig.ALLOW_PENDING_REGISTER:
            return RegisterResponse(RegisterResult.ERROR, "暂不开放注册，请使用注册码")
        
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
        admin_uids = ScoreAndRegisterConfig.ADMIN_UIDS
        if admin_uids:
            uid_list = [int(u.strip()) for u in admin_uids.split(',') if u.strip().isdigit()]
            is_admin = new_uid in uid_list
        
        # 如果 UID 未匹配，再检查管理员用户名列表
        if not is_admin:
            admin_usernames = ScoreAndRegisterConfig.ADMIN_USERNAMES
            if admin_usernames:
                name_list = [n.strip().lower() for n in admin_usernames.split(',') if n.strip()]
                is_admin = username.lower() in name_list
        
        # 检查白名单 UID 列表
        if not is_admin:
            whitelist_uids = ScoreAndRegisterConfig.WHITE_LIST_UIDS
            if whitelist_uids:
                uid_list = [int(u.strip()) for u in whitelist_uids.split(',') if u.strip().isdigit()]
                is_whitelist = new_uid in uid_list
        
        # 如果 UID 未匹配，再检查白名单用户名列表
        if not is_admin and not is_whitelist:
            whitelist_usernames = ScoreAndRegisterConfig.WHITE_LIST_USERNAMES
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
                ACTIVE_STATUS=True,  # 白名单默认激活
                EXPIRED_AT=permanent_expire,
                REGISTER_TIME=timestamp(),
            )
        else:
            # 普通用户：已激活但无 Emby 账户（需要积分激活 Emby 功能）
            user = UserModel(
                UID=new_uid,
                TELEGRAM_ID=telegram_id,
                USERNAME=username,
                EMAIL=email,
                EMBYID=None,  # 无 Emby 账户
                PASSWORD=hash_password(user_password),
                ROLE=Role.NORMAL.value,
                ACTIVE_STATUS=True,  # 账户激活，可以登录、签到
                EXPIRED_AT=-1,
                REGISTER_TIME=timestamp(),
            )
        await UserOperate.add_user(user)
        
        # 初始化积分记录（获取或创建）
        from src.db.score import ScoreOperate
        score_record = await ScoreOperate.get_score_by_uid(new_uid)
        if not score_record:
            from src.db.score import ScoreModel
            score_record = ScoreModel(
                UID=new_uid,
                TELEGRAM_ID=telegram_id,
                SCORE=ScoreAndRegisterConfig.PENDING_REGISTER_BONUS,  # 赠送初始积分
            )
            await ScoreOperate.add_score(score_record)
        else:
            # 已有记录，增加积分
            score_record.SCORE += ScoreAndRegisterConfig.PENDING_REGISTER_BONUS
            await ScoreOperate.update_score(score_record)
        
        logger.info(f"待激活用户注册: {username} (UID: {new_uid})")
        
        activate_cost = ScoreAndRegisterConfig.SCORE_REGISTER_NEED
        return RegisterResponse(
            result=RegisterResult.SUCCESS,
            message=f"注册成功！您可以登录使用基础功能，积攒 {activate_cost} 积分后可激活 Emby 账户",
            user=user,
            emby_password=user_password if not password else None
        )

    @staticmethod
    async def activate_pending_user(user: UserModel) -> Tuple[bool, str]:
        """
        激活待激活用户（使用积分创建 Emby 账户）
        """
        from src.config import ScoreAndRegisterConfig
        
        if user.EMBYID:
            return False, "账户已激活"
        
        # 检查积分
        from src.db.score import ScoreOperate
        score_record = await ScoreOperate.get_score_by_uid(user.UID)
        needed = ScoreAndRegisterConfig.SCORE_REGISTER_NEED
        
        if not score_record or score_record.SCORE < needed:
            current = score_record.SCORE if score_record else 0
            return False, f"积分不足，需要 {needed}，当前 {current}"
        
        # 扣除积分
        score_record.SCORE -= needed
        await ScoreOperate.update_score(score_record)
        
        # 创建 Emby 账户
        emby = get_emby_client()
        password = generate_password(12)
        
        try:
            emby_user = await emby.create_user(user.USERNAME, password)
            if not emby_user:
                # 退还积分
                score_record.SCORE += needed
                await ScoreOperate.update_score(score_record)
                return False, "创建 Emby 账户失败"
            
            # 更新用户
            user.EMBYID = emby_user.id
            user.ACTIVE_STATUS = True
            user.EXPIRED_AT = timestamp() + days_to_seconds(30)  # 30天有效期
            await UserOperate.update_user(user)
            
            logger.info(f"用户激活成功: {user.USERNAME}")
            return True, f"账户激活成功！Emby 密码: {password}，有效期 30 天"
            
        except Exception as e:
            # 退还积分
            score_record.SCORE += needed
            await ScoreOperate.update_score(score_record)
            logger.error(f"激活失败: {e}")
            return False, f"激活失败: {e}"

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
                existing_user.USERNAME = username
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
                await UserOperate.update_user(existing_user)
                user = existing_user
            else:
                new_uid = await UserOperate.get_new_uid()
                
                # 检查是否是管理员或白名单
                is_admin = False
                is_whitelist = False
                
                # 检查管理员
                admin_uids = ScoreAndRegisterConfig.ADMIN_UIDS
                if admin_uids:
                    uid_list = [int(u.strip()) for u in admin_uids.split(',') if u.strip().isdigit()]
                    is_admin = new_uid in uid_list
                if not is_admin:
                    admin_usernames = ScoreAndRegisterConfig.ADMIN_USERNAMES
                    if admin_usernames:
                        name_list = [n.strip().lower() for n in admin_usernames.split(',') if n.strip()]
                        is_admin = username.lower() in name_list
                
                # 检查白名单
                if not is_admin:
                    whitelist_uids = ScoreAndRegisterConfig.WHITE_LIST_UIDS
                    if whitelist_uids:
                        uid_list = [int(u.strip()) for u in whitelist_uids.split(',') if u.strip().isdigit()]
                        is_whitelist = new_uid in uid_list
                if not is_admin and not is_whitelist:
                    whitelist_usernames = ScoreAndRegisterConfig.WHITE_LIST_USERNAMES
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
            
            days = code_info.DAYS or days
            await RegCodeOperate.update_regcode_use_count(reg_code, 1)
        
        await UserOperate.renew_user_expire_time(user, days)
        
        # 如果用户被禁用，重新启用
        if not user.ACTIVE_STATUS:
            user.ACTIVE_STATUS = True
            await UserOperate.update_user(user)
            
            # 同时启用 Emby 账户
            if user.EMBYID:
                emby = get_emby_client()
                await emby.set_user_enabled(user.EMBYID, True)
        
        logger.info(f"用户续期成功: {user.USERNAME} +{days}天")
        return True, f"续期成功！增加 {days} 天"

    @staticmethod
    async def renew_by_score(user: UserModel) -> Tuple[bool, str]:
        """使用积分续期"""
        if not ScoreAndRegisterConfig.AUTO_RENEW_ENABLED:
            return False, "积分续期功能未开启"
        
        renew_days = ScoreAndRegisterConfig.AUTO_RENEW_DAYS
        renew_cost = ScoreAndRegisterConfig.AUTO_RENEW_COST
        
        # 检查积分
        score = await ScoreOperate.get_score_by_uid(user.UID)
        if not score or score.SCORE < renew_cost:
            current = score.SCORE if score else 0
            return False, f"积分不足，需要 {renew_cost} {ScoreAndRegisterConfig.SCORE_NAME}，当前 {current}"
        
        # 扣除积分
        score.SCORE -= renew_cost
        if hasattr(score, 'TOTAL_SPENT'):
            score.TOTAL_SPENT = (score.TOTAL_SPENT or 0) + renew_cost
        await ScoreOperate.update_score(score)
        
        # 执行续期
        success, msg = await UserService.renew_user(user, renew_days)
        
        if success:
            # 记录历史
            from src.db.score import ScoreHistoryOperate
            await ScoreHistoryOperate.add_history(
                uid=user.UID,
                type_='renew',
                amount=-renew_cost,
                balance_after=score.SCORE,
                note=f"使用积分续期 {renew_days} 天"
            )
            
            # 通知（可选）
            if user.TELEGRAM_ID and ScoreAndRegisterConfig.AUTO_RENEW_NOTIFY:
                from src.services.notification import NotificationService, Notification, NotificationType
                try:
                    await NotificationService.send(Notification(
                        type=NotificationType.USER_RENEWED,
                        title="✅ 积分续期成功",
                        content=f"使用 {renew_cost} {ScoreAndRegisterConfig.SCORE_NAME} 续期 {renew_days} 天成功！",
                        target_users=[user.TELEGRAM_ID]
                    ))
                except:
                    pass
            
            return True, f"续期成功！增加 {renew_days} 天，花费 {renew_cost} 积分"
        else:
            # 退还积分
            score.SCORE += renew_cost
            if hasattr(score, 'TOTAL_SPENT'):
                score.TOTAL_SPENT = (score.TOTAL_SPENT or 0) - renew_cost
            await ScoreOperate.update_score(score)
            return False, f"续期失败: {msg}"

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
            
            # 删除积分记录
            score = await ScoreOperate.get_score_by_uid(user.UID)
            if score:
                await ScoreOperate.delete_score(score)
            
            # 删除用户记录
            await UserOperate.delete_user(user)
            
            logger.info(f"用户已删除: {user.USERNAME}")
            return True, "用户已删除"
        except Exception as e:
            logger.error(f"删除用户失败: {e}")
            return False, f"删除失败: {e}"

    @staticmethod
    async def use_code(user: UserModel, code_str: str) -> Tuple[bool, str, Optional[str]]:
        """
        已登录用户统一使用授权码（注册码/续期码/白名单码）
        
        - 注册码(TYPE=1)：为无 Emby 账户的用户创建 Emby 账户
        - 续期码(TYPE=2)：续期
        - 白名单码(TYPE=3)：赋予白名单角色，如果没有 Emby 账户则自动创建
        
        :return: (成功, 消息, 新Emby密码 或 None)
        """
        from src.db.regcode import RegCodeOperate, Type as RegCodeType
        
        code_info = await RegCodeOperate.get_regcode_by_code(code_str)
        if not code_info:
            return False, "授权码无效", None
        
        if not code_info.ACTIVE:
            return False, "授权码已停用", None
        
        if code_info.USE_COUNT_LIMIT != -1 and code_info.USE_COUNT >= code_info.USE_COUNT_LIMIT:
            return False, "授权码已被使用完", None
        
        # 检查有效期
        if code_info.VALIDITY_TIME != -1:
            expire_time = code_info.CREATED_TIME + code_info.VALIDITY_TIME * 3600
            if timestamp() > expire_time:
                return False, "授权码已过期", None
        
        code_type = code_info.TYPE
        
        # ========== 续期码 ==========
        if code_type == RegCodeType.RENEW.value:
            success, msg = await UserService.renew_user(user, 30, code_str)
            return success, msg, None
        
        # ========== 注册码 ==========
        if code_type == RegCodeType.REGISTER.value:
            if user.EMBYID:
                return False, "您已拥有 Emby 账户，无需使用注册码", None
            
            # 为已有系统账户的用户创建 Emby 账户
            emby = get_emby_client()
            emby_password = generate_password(12)
            days = code_info.DAYS or 30
            
            try:
                existing_emby = await emby.get_user_by_name(user.USERNAME)
                if existing_emby:
                    return False, "该用户名在 Emby 中已存在", None
                
                emby_user = await emby.create_user(user.USERNAME, emby_password)
                if not emby_user:
                    return False, "创建 Emby 账户失败", None
                
                user.EMBYID = emby_user.id
                user.ACTIVE_STATUS = True
                if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
                    user.EXPIRED_AT = 253402214400
                else:
                    user.EXPIRED_AT = timestamp() + days_to_seconds(days)
                await UserOperate.update_user(user)
                
                await RegCodeOperate.update_regcode_use_count(code_str, 1)
                logger.info(f"注册码激活 Emby 账户: {user.USERNAME}")
                return True, f"Emby 账户创建成功！有效期 {days} 天", emby_password
            except EmbyError as e:
                logger.error(f"注册码创建 Emby 账户失败: {e}")
                return False, f"Emby 服务器错误: {e}", None
        
        # ========== 白名单码 ==========
        if code_type == RegCodeType.WHITELIST.value:
            emby_password = None
            
            # 如果没有 Emby 账户，自动创建
            if not user.EMBYID:
                emby = get_emby_client()
                emby_password = generate_password(12)
                
                try:
                    existing_emby = await emby.get_user_by_name(user.USERNAME)
                    if existing_emby:
                        return False, "该用户名在 Emby 中已存在", None
                    
                    emby_user = await emby.create_user(user.USERNAME, emby_password)
                    if not emby_user:
                        return False, "创建 Emby 账户失败", None
                    
                    user.EMBYID = emby_user.id
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
            if emby_password:
                msg += f"，Emby 账户已自动创建"
            logger.info(f"白名单码激活: {user.USERNAME}")
            return True, msg, emby_password
        
        return False, "未知的授权码类型", None

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
    async def toggle_nsfw(user: UserModel, enable: bool) -> Tuple[bool, str]:
        """
        切换 NSFW 库显示状态并同步到 Emby
        
        当用户开启/关闭 NSFW 显示时，会同步更新 Emby 中的 NSFW 库访问权限。
        """
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"
            
        if enable and not user.NSFW_ALLOWED:
            return False, "管理员未授予您访问 NSFW 媒体库的权限"
        
        from src.services.emby_service import EmbyService
        
        # 通过名称查找NSFW库ID
        nsfw_library_id = await EmbyService.find_nsfw_library_id()
        if not nsfw_library_id:
            return False, "系统未配置 NSFW 媒体库"
        
        try:
            emby = get_emby_client()
            
            # 同步到 Emby
            if enable:
                # 开启：授予 NSFW 库访问权限
                success = await emby.grant_nsfw_access(user.EMBYID)
                if not success:
                    return False, "授予 NSFW 库访问权限失败"
            else:
                # 关闭：撤销 NSFW 库访问权限
                success = await emby.revoke_nsfw_access(user.EMBYID)
                if not success:
                    return False, "撤销 NSFW 库访问权限失败"
            
            # 更新数据库中的显示状态
            user.NSFW = enable
            await UserOperate.update_user(user)
            
            status = "开启" if enable else "关闭"
            return True, f"NSFW 显示已{status}，已同步到 Emby"
        except Exception as e:
            logger.error(f"切换 NSFW 显示状态失败: {e}")
            return False, f"操作失败: {e}"

    @staticmethod
    async def sync_user_to_emby(user: UserModel) -> Tuple[bool, str]:
        """
        同步用户状态到 Emby
        
        同步内容包括：
        - 账号禁用状态（ACTIVE_STATUS）
        - NSFW 库访问权限（基于 NSFW_ALLOWED 和 NSFW 字段）
        """
        if not user.EMBYID:
            return True, "用户未绑定 Emby 账户，跳过同步"
        
        try:
            emby = get_emby_client()
            
            # 同步账号禁用状态
            await emby.set_user_enabled(user.EMBYID, user.ACTIVE_STATUS)
            
            # 同步 NSFW 访问权限
            # 逻辑：只有管理员允许 (NSFW_ALLOWED) 且 用户开启显示 (NSFW) 时，才在 Emby 中授予访问权限
            if user.NSFW_ALLOWED and user.NSFW:
                await emby.grant_nsfw_access(user.EMBYID)
            else:
                await emby.revoke_nsfw_access(user.EMBYID)
            
            logger.info(f"用户状态已同步到 Emby: {user.USERNAME} (UID: {user.UID}), 状态: {'启用' if user.ACTIVE_STATUS else '禁用'}, NSFW: {'开启' if user.NSFW_ALLOWED and user.NSFW else '关闭'}")
            return True, "同步成功"
        except Exception as e:
            logger.error(f"同步用户状态到 Emby 失败: {e}")
            return False, f"同步失败: {e}"

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
            "nsfw_enabled": user.NSFW,
            "nsfw_allowed": user.NSFW_ALLOWED,
            "bgm_mode": user.BGM_MODE,
            "auto_renew": user.AUTO_RENEW or False,
            "avatar": user.AVATAR or None,
            "register_time": user.REGISTER_TIME,
            "created_at": user.REGISTER_TIME,  # 前端兼容字段
            "emby_id": user.EMBYID,  # 添加 Emby ID
        }
        
        # 获取积分
        score = await ScoreOperate.get_score_by_uid(user.UID)
        if score:
            info["score"] = score.SCORE
            info["checkin_count"] = score.CHECKIN_COUNT
        
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

