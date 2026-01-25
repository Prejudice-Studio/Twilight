"""
数据库工具模块

提供数据库创建等工具函数，避免循环导入
"""
import os
import logging
from pathlib import Path
from typing import Type

from sqlalchemy import create_engine, text
from sqlalchemy.orm import DeclarativeBase

from src.config import Config

logger = logging.getLogger(__name__)


def create_database(database_name: str, model: Type[DeclarativeBase]) -> None:
    """
    创建SQLite数据库并初始化表结构
    
    :param database_name: 数据库名称
    :param model: SQLAlchemy ORM模型基类
    """
    os.makedirs(Config.DATABASES_DIR, exist_ok=True)
    
    db_path = os.path.join(Config.DATABASES_DIR, f'{database_name}.db')
    database_url = f"sqlite:///{db_path}"
    
    engine = create_engine(database_url)
    model.metadata.create_all(engine)
    
    # 启用WAL模式以提高并发性能
    with engine.connect() as connection:
        connection.execute(text('PRAGMA journal_mode = WAL'))
        connection.commit()
    
    logger.debug(f"数据库 {database_name} 初始化完成: {db_path}")


def get_database_path(database_name: str) -> Path:
    """
    获取数据库文件路径
    
    :param database_name: 数据库名称
    :return: 数据库文件完整路径
    """
    return Config.DATABASES_DIR / f"{database_name}.db"


def get_async_database_url(database_name: str) -> str:
    """
    获取异步数据库连接URL
    
    :param database_name: 数据库名称（不含.db后缀）
    :return: 异步数据库连接URL
    """
    return f'sqlite+aiosqlite:///{get_database_path(database_name)}'

