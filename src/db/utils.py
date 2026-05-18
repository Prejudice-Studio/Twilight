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


def _add_missing_columns(engine, model: Type[DeclarativeBase]) -> None:
    """对已存在的 SQLite 表做轻量迁移：补齐 model 中新增的列。

    仅支持 SQLite，且只处理"加列"这种向后兼容的变更。
    """
    from sqlalchemy import inspect

    inspector = inspect(engine)
    with engine.connect() as conn:
        for table_name, table in model.metadata.tables.items():
            if not inspector.has_table(table_name):
                continue
            existing_cols = {col['name'] for col in inspector.get_columns(table_name)}
            for column in table.columns:
                if column.name in existing_cols:
                    continue
                col_type = column.type.compile(dialect=engine.dialect)
                nullable = "" if column.nullable else " NOT NULL"
                default_sql = ""
                if column.default is not None and getattr(column.default, 'is_scalar', False):
                    val = column.default.arg
                    if isinstance(val, bool):
                        default_sql = f" DEFAULT {1 if val else 0}"
                    elif isinstance(val, (int, float)):
                        default_sql = f" DEFAULT {val}"
                    elif isinstance(val, str):
                        escaped = val.replace("'", "''")
                        default_sql = f" DEFAULT '{escaped}'"
                ddl = f'ALTER TABLE "{table_name}" ADD COLUMN "{column.name}" {col_type}{nullable}{default_sql}'
                try:
                    conn.execute(text(ddl))
                    conn.commit()
                    logger.info(f"[迁移] 表 {table_name} 新增列 {column.name}")
                except Exception as exc:  # pragma: no cover - 失败也不阻塞启动
                    logger.warning(f"[迁移] 表 {table_name} 加列 {column.name} 失败: {exc}")


def create_database(database_name: str, model: Type[DeclarativeBase]) -> None:
    """
    创建SQLite数据库并初始化表结构

    :param database_name: 数据库名称
    :param model: SQLAlchemy ORM模型基类
    """
    os.makedirs(Config.DATABASES_DIR, exist_ok=True)

    db_path = Path(Config.DATABASES_DIR) / f'{database_name}.db'
    database_url = f"sqlite:///{db_path.as_posix()}"

    engine = create_engine(database_url)
    db_existed = db_path.exists() and db_path.stat().st_size > 0
    model.metadata.create_all(engine)

    # 已有数据库：补齐 model 中新增的列（轻量迁移）
    if db_existed:
        _add_missing_columns(engine, model)

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


from sqlalchemy.ext.asyncio import create_async_engine, async_sessionmaker
from sqlalchemy.pool import NullPool

def get_async_database_url(database_name: str) -> str:
    """
    获取异步数据库连接URL
    
    :param database_name: 数据库名称（不含.db后缀）
    :return: 异步数据库连接URL
    """
    return f'sqlite+aiosqlite:///{get_database_path(database_name).as_posix()}'


def init_async_db(database_name: str, model: Type[DeclarativeBase]):
    """
    初始化异步数据库
    """
    create_database(database_name, model)
    url = get_async_database_url(database_name)
    engine = create_async_engine(url, echo=Config.SQLALCHEMY_LOG, poolclass=NullPool)
    session_factory = async_sessionmaker(bind=engine, expire_on_commit=False)
    return engine, session_factory

