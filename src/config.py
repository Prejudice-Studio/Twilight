"""
配置管理模块

提供基于TOML文件的配置管理功能
"""
import logging
import os
from pathlib import Path
from typing import List, Union, Any, Optional

import toml

logger = logging.getLogger(__name__)

ROOT_PATH: Path = Path(__file__).parent.parent.resolve()


class BaseConfig:
    """
    配置管理的基类
    
    提供从TOML文件读取和保存配置的能力
    """
    toml_file_path: str = os.path.join(ROOT_PATH, 'config.toml')
    _section: Optional[str] = None

    @classmethod
    def update_from_toml(cls, section: Optional[str] = None) -> None:
        """
        从TOML配置文件中加载配置
        
        :param section: TOML文件中的配置节名称，为None时加载根级配置
        """
        try:
            cls._section = section
            config = toml.load(cls.toml_file_path)
            items = config.get(section, {}) if section else config
            
            for key, value in items.items():
                attr_name = key.upper()
                if hasattr(cls, attr_name):
                    setattr(cls, attr_name, value)
                    
        except FileNotFoundError:
            logger.warning(f'配置文件不存在: {cls.toml_file_path}')
        except toml.TomlDecodeError as err:
            logger.error(f'TOML配置文件格式错误: {err}')
        except Exception as err:
            logger.error(f'加载配置文件时发生错误: {err}')

    @classmethod
    def save_to_toml(cls) -> bool:
        """
        将当前配置保存到TOML文件
        
        :return: 保存是否成功
        """
        try:
            # 读取现有配置
            try:
                config = toml.load(cls.toml_file_path)
            except FileNotFoundError:
                config = {}

            # 收集类的配置属性
            config_data = {}
            for key in dir(cls):
                if key.isupper() and not key.startswith('_'):
                    config_data[key.lower()] = getattr(cls, key)

            # 更新配置
            if cls._section:
                if cls._section not in config:
                    config[cls._section] = {}
                config[cls._section].update(config_data)
            else:
                config.update(config_data)

            # 写入文件
            with open(cls.toml_file_path, 'w', encoding='utf-8') as f:
                toml.dump(config, f)
            return True
            
        except Exception as err:
            logger.error(f'保存配置文件时发生错误: {err}')
            return False

    @classmethod
    def get(cls, key: str, default: Any = None) -> Any:
        """
        获取配置值
        
        :param key: 配置键名（不区分大小写）
        :param default: 默认值
        :return: 配置值
        """
        return getattr(cls, key.upper(), default)


class Config(BaseConfig):
    """全局配置管理类"""
    LOGGING: bool = True
    LOG_LEVEL: int = 20  # 日志等级，数字越大，日志越详细
    SQLALCHEMY_LOG: bool = False
    PROXY: Optional[str] = None
    MAX_RETRY: int = 3
    DATABASES_DIR: Path = ROOT_PATH / 'db'
    BANGUMI_TOKEN: str = ''
    GLOBAL_BGM_MODE: bool = False  # 是否允许BGM点格子
    TELEGRAM_MODE: bool = False
    EMAIL_BIND: bool = False
    FORCE_BIND_EMAIL: bool = False
    FORCE_BIND_TELEGRAM: bool = True


class EmbyConfig(BaseConfig):
    """Emby配置管理类"""
    EMBY_URL: str = 'http://127.0.0.1:8096/'
    EMBY_TOKEN: str = ''
    EMBY_URL_LIST: List[str] = [
        'Direct : http://127.0.0.1:8096/',
        'Sample : http://192.168.1.1:8096/'
    ]
    EMBY_URL_LIST_FOR_WHITELIST: List[str] = [
        'Direct : http://127.0.0.1:8096/',
        'Sample : http://192.168.1.1:8096/'
    ]
    EMBY_NSFW: str = ''


class TelegramConfig(BaseConfig):
    """Telegram配置管理类"""
    TELEGRAM_API_URL: str = 'https://api.telegram.org/bot'
    BOT_TOKEN: str = ''
    ADMIN_ID: Union[int, List[int]] = []
    GROUP_ID: Union[int, List[int]] = []
    CHANNEL_ID: Union[int, List[int]] = []
    FORCE_SUBSCRIBE: bool = False


class ScoreAndRegisterConfig(BaseConfig):
    """积分及注册配置管理类"""
    SCORE_NAME: str = '暮光币'
    REGISTER_MODE: bool = False
    REGISTER_CODE_LIMIT: bool = False  # 是否限制注册码注册
    SCORE_REGISTER_MODE: bool = False
    SCORE_REGISTER_NEED: int = 100  # 注册所需积分
    USER_LIMIT: int = 200  # 允许的已注册用户数量上限
    NEW_USER_NOTICE_STATUS: bool = False  # 用户注册/续期/白名单通知开关
    NEW_USER_NOTICE_LINK: bool = False  # 通知是否指向个人主页
    RED_PACKET_MODE: bool = False
    PRIVATE_TRANSFER_MODE: bool = False


# 自动加载配置
Config.update_from_toml("Global")
EmbyConfig.update_from_toml('Emby')
TelegramConfig.update_from_toml('Telegram')
ScoreAndRegisterConfig.update_from_toml('SAR')
