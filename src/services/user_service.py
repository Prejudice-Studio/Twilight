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
        telegram_id: int,
        username: str,
        reg_code: str,
        email: Optional[str] = None
    ) -> RegisterResponse:
        """
        通过注册码注册
        
        :param telegram_id: Telegram ID
        :param username: Emby 用户名
        :param reg_code: 注册码
        :param email: 邮箱（可选）
        """
        from src.db.regcode import RegCodeOperate, Type as RegCodeType
        
        # 检查注册是否开放
        available, msg = await UserService.check_registration_available()
        if not available:
            return RegisterResponse(RegisterResult.USER_LIMIT_REACHED, msg)
        
        # 检查用户是否已存在
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
            reg_code=reg_code
        )

    @staticmethod
    async def register_by_score(
        telegram_id: int,
        username: str,
        email: Optional[str] = None
    ) -> RegisterResponse:
        """通过积分注册"""
        if not ScoreAndRegisterConfig.SCORE_REGISTER_MODE:
            return RegisterResponse(RegisterResult.ERROR, "积分注册未开启")
        
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
            days=30
        )

    @staticmethod
    async def _create_emby_user(
        telegram_id: int,
        username: str,
        email: Optional[str],
        days: int,
        reg_code: Optional[str] = None
    ) -> RegisterResponse:
        """创建 Emby 用户（内部方法）"""
        emby = get_emby_client()
        
        try:
            # 检查 Emby 用户名是否已存在
            existing_emby = await emby.get_user_by_name(username)
            if existing_emby:
                return RegisterResponse(RegisterResult.EMBY_EXISTS, "该用户名在 Emby 中已存在")
            
            # 生成密码并创建 Emby 用户
            password = generate_password(12)
            emby_user = await emby.create_user(username, password)
            
            if not emby_user:
                return RegisterResponse(RegisterResult.EMBY_ERROR, "创建 Emby 账户失败")
            
            # 计算过期时间
            expire_at = timestamp() + days_to_seconds(days) if days > 0 else -1
            
            # 创建或更新本地用户记录
            existing_user = await UserOperate.get_user_by_telegram_id(telegram_id)
            
            if existing_user:
                existing_user.USERNAME = username
                existing_user.EMBYID = emby_user.id
                existing_user.PASSWORD = hash_password(password)
                existing_user.EXPIRED_AT = expire_at
                existing_user.ROLE = Role.NORMAL.value
                existing_user.EMAIL = email
                existing_user.REGISTER_TIME = timestamp()
                await UserOperate.update_user(existing_user)
                user = existing_user
            else:
                new_uid = await UserOperate.get_new_uid()
                user = UserModel(
                    UID=new_uid,
                    TELEGRAM_ID=telegram_id,
                    USERNAME=username,
                    EMAIL=email,
                    EMBYID=emby_user.id,
                    PASSWORD=hash_password(password),
                    ROLE=Role.NORMAL.value,
                    EXPIRED_AT=expire_at,
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
                emby_password=password
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
        """切换 NSFW 库访问权限"""
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"
        
        try:
            emby = get_emby_client()
            
            if enable:
                success = await emby.grant_nsfw_access(user.EMBYID)
            else:
                success = await emby.revoke_nsfw_access(user.EMBYID)
            
            if success:
                user.NSFW = enable
                await UserOperate.update_user(user)
                status = "开启" if enable else "关闭"
                return True, f"NSFW 库已{status}"
            else:
                return False, "操作失败"
        except Exception as e:
            logger.error(f"切换 NSFW 失败: {e}")
            return False, f"操作失败: {e}"

    @staticmethod
    async def get_user_info(user: UserModel) -> dict:
        """获取用户详细信息"""
        from src.core.utils import format_expire_time, mask_email
        
        info = {
            "uid": user.UID,
            "username": user.USERNAME,
            "telegram_id": user.TELEGRAM_ID,
            "email": mask_email(user.EMAIL) if user.EMAIL else None,
            "role": Role(user.ROLE).name,
            "active": user.ACTIVE_STATUS,
            "expire_status": format_expire_time(user.EXPIRED_AT),
            "expired_at": user.EXPIRED_AT,
            "nsfw_enabled": user.NSFW,
            "bgm_mode": user.BGM_MODE,
            "register_time": user.REGISTER_TIME,
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

