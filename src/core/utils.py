"""
核心工具模块

提供通用的工具函数和装饰器
"""
import hashlib
import string
import time
import re
import logging
import secrets
from typing import Optional, Callable, Any, List
from functools import wraps

logger = logging.getLogger(__name__)

def generate_random_string(length: int = 16, include_special: bool = False) -> str:
    """
    生成随机字符串 (加密安全)
    
    :param length: 字符串长度
    :param include_special: 是否包含特殊字符
    """
    chars = string.ascii_letters + string.digits
    if include_special:
        chars += "!@#$%^&*"
    return ''.join(secrets.choice(chars) for _ in range(length))


def generate_password(length: int = 12) -> str:
    """生成加密安全的随机密码"""
    # 确保至少包含一个大写、小写、数字
    uppercase = string.ascii_uppercase
    lowercase = string.ascii_lowercase
    digits = string.digits
    
    password = [
        secrets.choice(uppercase),
        secrets.choice(lowercase),
        secrets.choice(digits),
    ]
    # 填充剩余长度
    all_chars = uppercase + lowercase + digits
    password.extend(secrets.choice(all_chars) for _ in range(length - 3))
    
    # 打乱顺序
    secrets.SystemRandom().shuffle(password)
    return ''.join(password)


def hash_password(password: str, salt: Optional[str] = None, iterations: int = 100000) -> str:
    """
    对密码进行哈希处理 (使用 PBKDF2-SHA256)
    
    :param password: 原始密码
    :param salt: 盐值，为空则自动生成
    :param iterations: 迭代次数
    :return: 格式为 salt$iterations$hash 的字符串
    """
    if salt is None:
        salt = generate_random_string(16)
    
    # PBKDF2 哈希
    dk = hashlib.pbkdf2_hmac('sha256', password.encode(), salt.encode(), iterations)
    hashed = dk.hex()
    return f"{salt}${iterations}${hashed}"


def verify_password(password: str, hashed: str) -> bool:
    """验证密码是否正确 (兼容旧格式)"""
    if '$' not in hashed:
        return False
        
    parts = hashed.split('$')
    
    # 旧格式: salt$hash (SHA256)
    if len(parts) == 2:
        salt, _ = parts
        expected = f"{salt}${hashlib.sha256(f'{salt}{password}'.encode()).hexdigest()}"
        return expected == hashed
        
    # 新格式: salt$iterations$hash (PBKDF2)
    if len(parts) == 3:
        salt, iterations_str, _ = parts
        try:
            iterations = int(iterations_str)
            return hash_password(password, salt, iterations) == hashed
        except ValueError:
            return False
            
    return False


def is_valid_email(email: str) -> bool:
    """验证邮箱格式"""
    pattern = r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$'
    return bool(re.match(pattern, email))


def is_valid_username(username: str, min_length: int = 3, max_length: int = 20) -> bool:
    """
    验证用户名格式
    
    允许字母、数字、下划线，不能以数字开头
    """
    if not username or len(username) < min_length or len(username) > max_length:
        return False
    pattern = r'^[a-zA-Z_][a-zA-Z0-9_]*$'
    return bool(re.match(pattern, username))


def mask_string(s: str, show_chars: int = 4, mask_char: str = '*') -> str:
    """
    遮罩字符串
    
    例如: "1234567890" -> "1234******"
    """
    if len(s) <= show_chars:
        return s
    return s[:show_chars] + mask_char * (len(s) - show_chars)


def mask_email(email: str) -> str:
    """
    遮罩邮箱
    
    例如: "test@example.com" -> "te**@example.com"
    """
    if '@' not in email:
        return mask_string(email)
    local, domain = email.rsplit('@', 1)
    if len(local) <= 2:
        return f"{local[0]}*@{domain}"
    return f"{local[:2]}{'*' * (len(local) - 2)}@{domain}"


# ==================== 时间工具 ====================

def timestamp() -> int:
    """获取当前时间戳（秒）"""
    return int(time.time())


def timestamp_ms() -> int:
    """获取当前时间戳（毫秒）"""
    return int(time.time() * 1000)


def days_to_seconds(days: int) -> int:
    """天数转秒数"""
    return days * 86400


def seconds_to_days(seconds: int) -> float:
    """秒数转天数"""
    return seconds / 86400


def is_expired(expire_timestamp: int) -> bool:
    """检查时间戳是否已过期（-1 表示永不过期）"""
    if expire_timestamp == -1:
        return False
    return timestamp() > expire_timestamp


def format_duration(seconds: int) -> str:
    """
    格式化时长
    
    :return: 如 "3天5小时20分钟"
    """
    if seconds < 0:
        return "永久"
    
    days, remainder = divmod(seconds, 86400)
    hours, remainder = divmod(remainder, 3600)
    minutes, _ = divmod(remainder, 60)
    
    parts = []
    if days > 0:
        parts.append(f"{days}天")
    if hours > 0:
        parts.append(f"{hours}小时")
    if minutes > 0 or not parts:
        parts.append(f"{minutes}分钟")
    
    return ''.join(parts)


def format_expire_time(expire_timestamp: int) -> str:
    """格式化过期时间"""
    if expire_timestamp == -1 or expire_timestamp >= 253402214400:
        return "永不过期"
    
    remaining = expire_timestamp - timestamp()
    if remaining <= 0:
        return "已过期"
    
    return f"剩余 {format_duration(remaining)}"


# ==================== 数值工具 ====================

def clamp(value: int, min_val: int, max_val: int) -> int:
    """将数值限制在指定范围内"""
    return max(min_val, min(max_val, value))


def safe_int(value: Any, default: int = 0) -> int:
    """安全地转换为整数"""
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


# ==================== 装饰器 ====================

def retry(max_attempts: int = 3, delay: float = 1.0, exceptions: tuple = (Exception,)):
    """
    重试装饰器
    
    :param max_attempts: 最大重试次数
    :param delay: 重试间隔（秒）
    :param exceptions: 需要重试的异常类型
    """
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        async def async_wrapper(*args, **kwargs):
            last_exception = None
            for attempt in range(max_attempts):
                try:
                    return await func(*args, **kwargs)
                except exceptions as e:
                    last_exception = e
                    logger.warning(f"{func.__name__} 失败 (尝试 {attempt + 1}/{max_attempts}): {e}")
                    if attempt < max_attempts - 1:
                        await __import__('asyncio').sleep(delay)
            raise last_exception
        
        @wraps(func)
        def sync_wrapper(*args, **kwargs):
            last_exception = None
            for attempt in range(max_attempts):
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    last_exception = e
                    logger.warning(f"{func.__name__} 失败 (尝试 {attempt + 1}/{max_attempts}): {e}")
                    if attempt < max_attempts - 1:
                        time.sleep(delay)
            raise last_exception
        
        import asyncio
        if asyncio.iscoroutinefunction(func):
            return async_wrapper
        return sync_wrapper
    
    return decorator


def singleton(cls):
    """单例模式装饰器"""
    instances = {}
    
    @wraps(cls)
    def get_instance(*args, **kwargs):
        if cls not in instances:
            instances[cls] = cls(*args, **kwargs)
        return instances[cls]
    
    return get_instance


# ==================== 日志工具 ====================

def setup_logging(
    level: int = logging.INFO,
    format_string: str = '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
) -> None:
    """配置日志"""
    logging.basicConfig(
        level=level,
        format=format_string,
        handlers=[
            logging.StreamHandler(),
        ]
    )

