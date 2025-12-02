"""
数据模式模块

定义 API 请求/响应的数据结构
"""
from dataclasses import dataclass, asdict
from typing import Optional, List, Any
from enum import Enum


# ==================== 通用响应 ====================

@dataclass
class APIResponse:
    """通用 API 响应"""
    success: bool
    message: str
    data: Any = None
    
    def to_dict(self) -> dict:
        return asdict(self)


# ==================== 用户相关 ====================

@dataclass
class UserRegisterRequest:
    """用户注册请求"""
    telegram_id: int
    username: str
    reg_code: Optional[str] = None
    email: Optional[str] = None
    use_score: bool = False


@dataclass
class UserInfo:
    """用户信息"""
    uid: int
    username: str
    telegram_id: Optional[int]
    email: Optional[str]
    role: str
    active: bool
    expire_status: str
    expired_at: int
    score: int = 0
    checkin_days: int = 0
    nsfw_enabled: bool = False
    bgm_mode: bool = False


# ==================== 积分相关 ====================

@dataclass
class CheckinRequest:
    """签到请求"""
    telegram_id: int


@dataclass
class TransferRequest:
    """转账请求"""
    from_telegram_id: int
    to_telegram_id: int
    amount: int


@dataclass
class RedPacketCreateRequest:
    """创建红包请求"""
    telegram_id: int
    amount: int
    count: int
    packet_type: int = 1  # 1=随机, 2=均分


@dataclass
class RedPacketGrabRequest:
    """抢红包请求"""
    rp_key: str
    telegram_id: int


# ==================== 注册码相关 ====================

@dataclass
class RegCodeCreateRequest:
    """创建注册码请求"""
    validity_time: int = -1  # 有效小时数, -1永久
    type: int = 1  # 1=注册, 2=续期, 3=白名单
    use_count_limit: int = 1
    count: int = 1
    days: int = 30


@dataclass
class RegCodeInfo:
    """注册码信息"""
    code: str
    type: int
    validity_time: int
    use_count: int
    use_count_limit: int
    days: int
    active: bool
    created_time: int


# ==================== Bangumi 相关 ====================

@dataclass 
class BangumiRequireRequest:
    """番剧求片请求"""
    telegram_id: int
    bangumi_id: int
    other_info: Optional[str] = None


@dataclass
class BangumiRequireInfo:
    """番剧求片信息"""
    id: int
    telegram_id: int
    bangumi_id: int
    status: int
    timestamp: int


__all__ = [
    'APIResponse',
    'UserRegisterRequest',
    'UserInfo',
    'CheckinRequest',
    'TransferRequest',
    'RedPacketCreateRequest',
    'RedPacketGrabRequest',
    'RegCodeCreateRequest',
    'RegCodeInfo',
    'BangumiRequireRequest',
    'BangumiRequireInfo',
]

