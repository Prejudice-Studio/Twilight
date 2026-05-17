"""
全站公告板数据库模块

公告由管理员在面板创建/编辑，未登录用户也可读取（用于登录页等场景）。
支持多条同时存在，按置顶 > 发布时间倒序排列。
"""
import time
from typing import Optional, List

from sqlalchemy import (
    select, update, delete, func, String, Integer, Boolean,
)
from sqlalchemy.ext.asyncio import AsyncAttrs
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.db.utils import init_async_db


class AnnouncementsDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class AnnouncementModel(AnnouncementsDatabaseModel):
    __tablename__ = 'announcements'

    ID: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    TITLE: Mapped[Optional[str]] = mapped_column(String, default='', nullable=True)
    CONTENT: Mapped[str] = mapped_column(String, default='', nullable=False)
    # 级别：info / notice / warning / critical
    LEVEL: Mapped[str] = mapped_column(String(16), default='info', nullable=False, index=True)
    # 渲染方式：plain / markdown / bbcode
    RENDER_MODE: Mapped[str] = mapped_column(String(16), default='plain', nullable=False)
    PINNED: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False, index=True)
    VISIBLE: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False, index=True)
    # -1 表示永不过期
    EXPIRES_AT: Mapped[int] = mapped_column(Integer, default=-1, nullable=False)
    CREATED_AT: Mapped[int] = mapped_column(
        Integer, default=lambda: int(time.time()), nullable=False, index=True
    )
    UPDATED_AT: Mapped[int] = mapped_column(
        Integer, default=lambda: int(time.time()), nullable=False
    )
    CREATED_BY_UID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)


_, AnnouncementsSessionFactory = init_async_db("announcements", AnnouncementsDatabaseModel)


ALLOWED_LEVELS = {'info', 'notice', 'warning', 'critical'}
ALLOWED_RENDER_MODES = {'plain', 'markdown', 'bbcode'}


def normalize_level(level: Optional[str]) -> str:
    raw = (level or 'info').strip().lower()
    return raw if raw in ALLOWED_LEVELS else 'info'


def normalize_render_mode(mode: Optional[str]) -> str:
    raw = (mode or 'plain').strip().lower()
    # 兼容别名：text -> plain
    if raw in ('text', 'plaintext', 'txt'):
        raw = 'plain'
    if raw in ('md',):
        raw = 'markdown'
    if raw in ('bb',):
        raw = 'bbcode'
    return raw if raw in ALLOWED_RENDER_MODES else 'plain'


def _column_exists(conn, table: str, column: str) -> bool:
    rows = list(conn.exec_driver_sql(f"PRAGMA table_info({table})"))
    return any((row[1] or '').upper() == column.upper() for row in rows)


def _ensure_render_mode_column() -> None:
    """老库升级：缺失 RENDER_MODE 列时自动 ALTER。"""
    from sqlalchemy import create_engine, text as _text
    from src.db.utils import get_database_path

    db_path = get_database_path("announcements")
    if not db_path.exists():
        return
    engine = create_engine(f"sqlite:///{db_path.as_posix()}")
    try:
        with engine.begin() as conn:
            if not _column_exists(conn, 'announcements', 'RENDER_MODE'):
                conn.exec_driver_sql(
                    "ALTER TABLE announcements ADD COLUMN RENDER_MODE VARCHAR(16) "
                    "NOT NULL DEFAULT 'plain'"
                )
    finally:
        engine.dispose()


_ensure_render_mode_column()


class AnnouncementOperate:
    @staticmethod
    async def list_active(limit: int = 50) -> List[AnnouncementModel]:
        """获取对终端用户可见的公告：visible=True 且未过期，按置顶 + 时间排序。"""
        now = int(time.time())
        async with AnnouncementsSessionFactory() as session:
            result = await session.execute(
                select(AnnouncementModel)
                .where(
                    AnnouncementModel.VISIBLE == True,  # noqa: E712
                    (AnnouncementModel.EXPIRES_AT == -1) | (AnnouncementModel.EXPIRES_AT > now),
                )
                .order_by(
                    AnnouncementModel.PINNED.desc(),
                    AnnouncementModel.CREATED_AT.desc(),
                )
                .limit(max(1, min(int(limit), 200)))
            )
            return list(result.scalars().all())

    @staticmethod
    async def list_all(
        include_invisible: bool = True,
        include_expired: bool = True,
        page: int = 1,
        per_page: int = 20,
    ) -> tuple[List[AnnouncementModel], int]:
        """管理员视角：支持分页、过滤可见性与是否过期。"""
        now = int(time.time())
        page = max(1, int(page))
        per_page = max(1, min(int(per_page), 100))

        async with AnnouncementsSessionFactory() as session:
            conditions = []
            if not include_invisible:
                conditions.append(AnnouncementModel.VISIBLE == True)  # noqa: E712
            if not include_expired:
                conditions.append(
                    (AnnouncementModel.EXPIRES_AT == -1) | (AnnouncementModel.EXPIRES_AT > now)
                )

            count_query = select(func.count()).select_from(AnnouncementModel)
            if conditions:
                count_query = count_query.where(*conditions)
            total = int((await session.execute(count_query)).scalar_one() or 0)

            query = (
                select(AnnouncementModel)
                .order_by(
                    AnnouncementModel.PINNED.desc(),
                    AnnouncementModel.CREATED_AT.desc(),
                )
                .offset((page - 1) * per_page)
                .limit(per_page)
            )
            if conditions:
                query = query.where(*conditions)
            rows = list((await session.execute(query)).scalars().all())
            return rows, total

    @staticmethod
    async def get_by_id(announcement_id: int) -> Optional[AnnouncementModel]:
        async with AnnouncementsSessionFactory() as session:
            scalar = await session.execute(
                select(AnnouncementModel).where(AnnouncementModel.ID == int(announcement_id)).limit(1)
            )
            return scalar.scalar_one_or_none()

    @staticmethod
    async def create(
        title: Optional[str],
        content: str,
        level: str = 'info',
        pinned: bool = False,
        visible: bool = True,
        expires_at: int = -1,
        created_by_uid: Optional[int] = None,
        render_mode: str = 'plain',
    ) -> AnnouncementModel:
        now = int(time.time())
        item = AnnouncementModel(
            TITLE=(title or '').strip() or None,
            CONTENT=(content or '').strip(),
            LEVEL=normalize_level(level),
            RENDER_MODE=normalize_render_mode(render_mode),
            PINNED=bool(pinned),
            VISIBLE=bool(visible),
            EXPIRES_AT=int(expires_at) if expires_at is not None else -1,
            CREATED_AT=now,
            UPDATED_AT=now,
            CREATED_BY_UID=int(created_by_uid) if created_by_uid is not None else None,
        )
        async with AnnouncementsSessionFactory() as session:
            async with session.begin():
                session.add(item)
                await session.flush()
                snapshot = AnnouncementModel(
                    ID=item.ID,
                    TITLE=item.TITLE,
                    CONTENT=item.CONTENT,
                    LEVEL=item.LEVEL,
                    RENDER_MODE=item.RENDER_MODE,
                    PINNED=item.PINNED,
                    VISIBLE=item.VISIBLE,
                    EXPIRES_AT=item.EXPIRES_AT,
                    CREATED_AT=item.CREATED_AT,
                    UPDATED_AT=item.UPDATED_AT,
                    CREATED_BY_UID=item.CREATED_BY_UID,
                )
        return snapshot

    @staticmethod
    async def update_fields(
        announcement_id: int,
        title: Optional[str] = None,
        content: Optional[str] = None,
        level: Optional[str] = None,
        pinned: Optional[bool] = None,
        visible: Optional[bool] = None,
        expires_at: Optional[int] = None,
        render_mode: Optional[str] = None,
    ) -> Optional[AnnouncementModel]:
        values: dict = {}
        if title is not None:
            values['TITLE'] = title.strip() or None
        if content is not None:
            values['CONTENT'] = content.strip()
        if level is not None:
            values['LEVEL'] = normalize_level(level)
        if render_mode is not None:
            values['RENDER_MODE'] = normalize_render_mode(render_mode)
        if pinned is not None:
            values['PINNED'] = bool(pinned)
        if visible is not None:
            values['VISIBLE'] = bool(visible)
        if expires_at is not None:
            values['EXPIRES_AT'] = int(expires_at)
        if not values:
            return await AnnouncementOperate.get_by_id(announcement_id)

        values['UPDATED_AT'] = int(time.time())

        async with AnnouncementsSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    update(AnnouncementModel)
                    .where(AnnouncementModel.ID == int(announcement_id))
                    .values(**values)
                )

        return await AnnouncementOperate.get_by_id(announcement_id)

    @staticmethod
    async def delete(announcement_id: int) -> bool:
        async with AnnouncementsSessionFactory() as session:
            async with session.begin():
                result = await session.execute(
                    delete(AnnouncementModel).where(AnnouncementModel.ID == int(announcement_id))
                )
                return (result.rowcount or 0) > 0
