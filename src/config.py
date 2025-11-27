import logging
import os
from pathlib import Path
from typing import List, Union

import toml

ROOT_PATH: Path = Path(__file__ + '/../..').resolve()


class BaseConfig:
    """
    配置管理的基类。
    """
    toml_file_path = os.path.join(ROOT_PATH, 'config.toml')
    section = None

    @classmethod
    def update_from_toml(cls, section: str = None):
        try:
            cls.section = section
            config = toml.load(cls.toml_file_path)
            items = config.get(section, {}) if section else config
            for key, value in items.items():
                if hasattr(cls, key.upper()):
                    setattr(cls, key.upper(), value)
        except Exception as err:
            logging.error(f'Error occurred while loading config file: {err}')

    @classmethod
    def save_to_toml(cls):
        try:
            config = toml.load(cls.toml_file_path)
            if cls.section:
                if cls.section not in config:
                    config[cls.section] = {}
                for key in dir(cls):
                    if key.isupper():
                        config[cls.section][key] = getattr(cls, key)
            else:
                for key in dir(cls):
                    if key.isupper():
                        config[key] = getattr(cls, key)
            with open(cls.toml_file_path, 'w') as f:
                toml.dump(config, f)
        except Exception as err:
            logging.error(f'Error occurred while saving config file: {err}')

class Config(BaseConfig):
    """
    全局配置管理类。
    """
    LOGGING: bool = True # 是否开启日志 Boolean
    LOG_LEVEL: int = 20 # 日志等级，数字越大，日志越详细 Integer
    SQLALCHEMY_LOG: bool = False  # 是否开启SQLAlchemy日志
    PROXY: str = None  # 代理
    MAX_RETRY: int = 3  # 重试次数
    DATABASES_DIR: Path = ROOT_PATH / 'db'  # 数据库路径
    BANGUMI_TOKEN: str = ''  # Bangumi Token
    GLOBAL_BGM_MODE: bool = False  # 是否允许BGM点格子
    TELEGRAM_MODE: bool = False  # 是否开启Telegram模式
    EMAIL_BIND: bool = False  # 是否绑定邮箱
    FORCE_BIND_EMAIL: bool = False  # 是否强制绑定邮箱
    FORCE_BIND_TELEGRAM: bool = True  # 是否强制绑定Telegram
    
class EmbyConfig(BaseConfig):
    """
    Emby配置管理类。
    """
    EMBY_URL: str = 'http://127.0.0.1:8096/'  # Emby地址
    EMBY_TOKEN: str = ''  # Emby Token/ApiKey
    EMBY_URL_LIST: List[str] = ['http://127.0.0.1:8096/']  # Emby地址列表

class TelegramConfig(BaseConfig):
    """
    Telegram配置管理类。
    """
    TELEGRAM_API_URL: str = 'https://api.telegram.org/bot'  # Telegram Bot API URL
    BOT_TOKEN: str = ''  # Telegram Bot Token
    ADMIN_ID: Union[int, List[int]] = []  # 管理员ID
    GROUP_ID: Union[int, List[int]] = []  # 群组ID
    CHANNEL_ID: Union[int, List[int]] = []  # 频道ID
    FORCE_SUBSCRIBE: bool = False  # 是否强制加入群组/频道
    
class ScoreAndRegisterConfig(BaseConfig):
    """
    积分及注册配置管理类。
    """
    SCORE_NAME: str = '暮光币'  # 积分名称
    REGISTER_MODE: bool = False  # 是否允许注册功能
    REGISTER_CODE_LIMIT: bool = False  # REGISTER_MODE为True时，是否允许注册码注册
    SCORE_REGISTER_MODE: bool = False  # 是否允许积分注册
    SCORE_REGISTER_NEED: int = 100  # SCORE_REGISTER_MODE为True时，注册所需积分
    USER_LIMIT: int = 200 # 允许的已注册用户数量上限
    NEW_USER_NOTICE_STATUS: bool = False  # 用户注册/续期/白名单通知开关
    NEW_USER_NOTICE_LINK: bool = False  # 用户注册/续期/白名单通知是否指向个人主页通知是否指向个人简介
    RED_PACKET_MODE: bool = False  # 是否开启红包功能
    PRIVATE_TRANSFER_MODE: bool = False # 是否开启私人转账

Config.update_from_toml("Global")
EmbyConfig.update_from_toml('Emby')
TelegramConfig.update_from_toml('Telegram')
ScoreAndRegisterConfig.update_from_toml('SAR')