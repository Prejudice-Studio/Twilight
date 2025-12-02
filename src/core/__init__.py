"""
核心模块

提供通用工具和基础设施
"""
from src.core.utils import (
    # 字符串工具
    generate_random_string,
    generate_password,
    hash_password,
    verify_password,
    is_valid_email,
    is_valid_username,
    mask_string,
    mask_email,
    # 时间工具
    timestamp,
    timestamp_ms,
    days_to_seconds,
    seconds_to_days,
    is_expired,
    format_duration,
    format_expire_time,
    # 数值工具
    clamp,
    safe_int,
    # 装饰器
    retry,
    singleton,
    # 日志
    setup_logging,
)

__all__ = [
    'generate_random_string',
    'generate_password',
    'hash_password',
    'verify_password',
    'is_valid_email',
    'is_valid_username',
    'mask_string',
    'mask_email',
    'timestamp',
    'timestamp_ms',
    'days_to_seconds',
    'seconds_to_days',
    'is_expired',
    'format_duration',
    'format_expire_time',
    'clamp',
    'safe_int',
    'retry',
    'singleton',
    'setup_logging',
]

