"""
安全服务模块

设备限制、IP 限制、登录验证等
"""
import logging
from typing import Optional, Tuple, List, Dict, Any
from dataclasses import dataclass
from enum import Enum

from src.config import DeviceLimitConfig, Config
from src.db.login_log import (
    LoginLogOperate, UserDeviceOperate, IPListOperate,
    LoginLogModel, UserDeviceModel
)
from src.db.user import UserOperate
from src.services.emby import get_emby_client
from src.core.utils import timestamp

logger = logging.getLogger(__name__)


class LoginCheckResult(Enum):
    """登录检查结果"""
    ALLOWED = "allowed"
    IP_BLACKLISTED = "ip_blacklisted"
    IP_LIMIT_EXCEEDED = "ip_limit_exceeded"
    DEVICE_LIMIT_EXCEEDED = "device_limit_exceeded"
    DEVICE_BLOCKED = "device_blocked"
    STREAM_LIMIT_EXCEEDED = "stream_limit_exceeded"


@dataclass
class LoginCheckResponse:
    """登录检查响应"""
    result: LoginCheckResult
    allowed: bool
    message: str
    data: Optional[Dict[str, Any]] = None


class SecurityService:
    """安全服务"""
    
    @classmethod
    async def check_login(
        cls,
        uid: int,
        ip_address: str,
        device_id: str = None,
        device_name: str = None,
        client: str = None
    ) -> LoginCheckResponse:
        """
        检查登录是否允许
        
        :param uid: 用户 UID
        :param ip_address: IP 地址
        :param device_id: 设备 ID
        :param device_name: 设备名称
        :param client: 客户端类型
        """
        # 1. 检查 IP 黑名单
        if await IPListOperate.is_ip_blacklisted(ip_address):
            await cls._log_login(uid, ip_address, device_id, device_name, client, blocked=True)
            return LoginCheckResponse(
                result=LoginCheckResult.IP_BLACKLISTED,
                allowed=False,
                message="您的 IP 已被封禁"
            )
        
        # 2. 检查设备是否被封禁
        if device_id:
            devices = await UserDeviceOperate.get_user_devices(uid)
            for d in devices:
                if d.DEVICE_ID == device_id and d.IS_BLOCKED:
                    await cls._log_login(uid, ip_address, device_id, device_name, client, blocked=True)
                    return LoginCheckResponse(
                        result=LoginCheckResult.DEVICE_BLOCKED,
                        allowed=False,
                        message="该设备已被禁止使用"
                    )
        
        # 3. 检查设备数量限制
        if DeviceLimitConfig.DEVICE_LIMIT_ENABLED and device_id:
            device_count = await UserDeviceOperate.get_user_device_count(uid)
            if device_count >= DeviceLimitConfig.MAX_DEVICES:
                # 检查是否是已知设备
                devices = await UserDeviceOperate.get_user_devices(uid)
                known_device_ids = [d.DEVICE_ID for d in devices]
                
                if device_id not in known_device_ids:
                    await cls._log_login(uid, ip_address, device_id, device_name, client, blocked=True)
                    return LoginCheckResponse(
                        result=LoginCheckResult.DEVICE_LIMIT_EXCEEDED,
                        allowed=False,
                        message=f"设备数量已达上限 ({DeviceLimitConfig.MAX_DEVICES})"
                    )
        
        # 4. 检查 IP 数量限制（如果配置了）
        # 这个可以在配置中添加，暂时跳过
        
        # 记录登录和设备
        await cls._log_login(uid, ip_address, device_id, device_name, client, blocked=False)
        if device_id:
            await UserDeviceOperate.add_or_update_device(uid, device_id, device_name, client)
        
        return LoginCheckResponse(
            result=LoginCheckResult.ALLOWED,
            allowed=True,
            message="登录允许"
        )
    
    @classmethod
    async def check_stream_limit(cls, uid: int) -> LoginCheckResponse:
        """
        检查同时播放数限制
        
        :param uid: 用户 UID
        """
        if not DeviceLimitConfig.DEVICE_LIMIT_ENABLED:
            return LoginCheckResponse(
                result=LoginCheckResult.ALLOWED,
                allowed=True,
                message="未启用限制"
            )
        
        user = await UserOperate.get_user_by_uid(uid)
        if not user or not user.EMBYID:
            return LoginCheckResponse(
                result=LoginCheckResult.ALLOWED,
                allowed=True,
                message="用户无 Emby 账户"
            )
        
        # 获取用户当前会话
        try:
            emby = get_emby_client()
            sessions = await emby.get_sessions()
            
            # 统计用户的活跃播放
            user_streams = [
                s for s in sessions 
                if s.user_id == user.EMBYID and s.is_active
            ]
            
            if len(user_streams) >= DeviceLimitConfig.MAX_STREAMS:
                if DeviceLimitConfig.KICK_OLDEST_SESSION and user_streams:
                    # 踢掉最早的会话
                    oldest = min(user_streams, key=lambda s: s.last_activity_date or 0)
                    await emby.kill_session(oldest.id)
                    logger.info(f"踢出用户 {uid} 的旧会话: {oldest.device_name}")
                    
                    return LoginCheckResponse(
                        result=LoginCheckResult.ALLOWED,
                        allowed=True,
                        message="已踢出旧设备"
                    )
                else:
                    return LoginCheckResponse(
                        result=LoginCheckResult.STREAM_LIMIT_EXCEEDED,
                        allowed=False,
                        message=f"同时播放数已达上限 ({DeviceLimitConfig.MAX_STREAMS})",
                        data={'current_streams': len(user_streams)}
                    )
        except Exception as e:
            logger.error(f"检查播放限制失败: {e}")
        
        return LoginCheckResponse(
            result=LoginCheckResult.ALLOWED,
            allowed=True,
            message="允许"
        )
    
    @staticmethod
    async def _log_login(
        uid: int,
        ip_address: str,
        device_id: str = None,
        device_name: str = None,
        client: str = None,
        blocked: bool = False
    ) -> None:
        """记录登录日志"""
        user = await UserOperate.get_user_by_uid(uid)
        
        log = LoginLogModel(
            UID=uid,
            EMBY_USER_ID=user.EMBYID if user else None,
            IP_ADDRESS=ip_address,
            DEVICE_ID=device_id,
            DEVICE_NAME=device_name,
            CLIENT=client,
            LOGIN_TIME=timestamp(),
            IS_BLOCKED=blocked
        )
        
        await LoginLogOperate.add_log(log)
    
    @classmethod
    async def get_user_login_history(cls, uid: int, limit: int = 50) -> List[Dict[str, Any]]:
        """获取用户登录历史"""
        logs = await LoginLogOperate.get_user_logs(uid, limit)
        
        return [{
            'id': log.ID,
            'ip': log.IP_ADDRESS,
            'device': log.DEVICE_NAME or '未知',
            'client': log.CLIENT or '未知',
            'time': log.LOGIN_TIME,
            'blocked': log.IS_BLOCKED,
            'country': log.COUNTRY,
            'city': log.CITY,
        } for log in logs]
    
    @classmethod
    async def get_user_devices(cls, uid: int) -> List[Dict[str, Any]]:
        """获取用户设备列表"""
        devices = await UserDeviceOperate.get_user_devices(uid)
        
        return [{
            'device_id': d.DEVICE_ID,
            'device_name': d.DEVICE_NAME,
            'client': d.CLIENT,
            'first_seen': d.FIRST_SEEN,
            'last_seen': d.LAST_SEEN,
            'is_trusted': d.IS_TRUSTED,
        } for d in devices]
    
    @classmethod
    async def block_user_device(cls, uid: int, device_id: str) -> Tuple[bool, str]:
        """封禁用户设备"""
        success = await UserDeviceOperate.block_device(uid, device_id)
        if success:
            return True, "设备已封禁"
        return False, "设备不存在"
    
    @classmethod
    async def trust_user_device(cls, uid: int, device_id: str) -> Tuple[bool, str]:
        """信任用户设备"""
        success = await UserDeviceOperate.trust_device(uid, device_id)
        if success:
            return True, "设备已设为信任"
        return False, "设备不存在"
    
    @classmethod
    async def add_ip_to_blacklist(cls, ip: str, reason: str = None, hours: int = -1) -> Tuple[bool, str]:
        """添加 IP 到黑名单"""
        try:
            await IPListOperate.add_to_blacklist(ip, reason, hours)
            expire_msg = f"({hours}小时)" if hours > 0 else "(永久)"
            return True, f"IP {ip} 已加入黑名单 {expire_msg}"
        except Exception as e:
            return False, f"添加失败: {e}"
    
    @classmethod
    async def remove_ip_from_blacklist(cls, ip: str) -> Tuple[bool, str]:
        """从黑名单移除 IP"""
        success = await IPListOperate.remove_from_blacklist(ip)
        if success:
            return True, f"IP {ip} 已从黑名单移除"
        return False, "IP 不在黑名单中"
    
    @classmethod
    async def get_suspicious_activity(cls, hours: int = 24) -> List[Dict[str, Any]]:
        """获取可疑活动"""
        logs = await LoginLogOperate.get_suspicious_logins(hours)
        
        return [{
            'uid': log.UID,
            'ip': log.IP_ADDRESS,
            'device': log.DEVICE_NAME,
            'time': log.LOGIN_TIME,
            'reason': 'blocked',
        } for log in logs]

