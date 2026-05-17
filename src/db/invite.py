"""邀请树数据模型

设计要点
========
- ``invite_relations``：单亲多子的森林，每条边 (PARENT_UID, CHILD_UID) 唯一，
  CHILD_UID 同时也是主键（一个用户最多一个父级；删除父级则该用户晋升为新树根）。
- ``invite_codes``：邀请人生成的 Emby 注册码，单次使用，被使用后写入 ``USED_BY_UID``
  并在 ``invite_relations`` 中落下一条边。
- 不在这里维护层级缓存；层级、子树等聚合在服务层通过迭代 BFS 实时计算，
  避免数据漂移（小规模下用户量足够）。
"""
from __future__ import annotations

import time
from typing import Optional, List

from sqlalchemy import (
    select, update, delete, func, String, Integer, Boolean,
)
from sqlalchemy.ext.asyncio import AsyncAttrs
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.db.utils import init_async_db


class InviteDatabaseModel(AsyncAttrs, DeclarativeBase):
    pass


class InviteRelationModel(InviteDatabaseModel):
    """邀请关系（边）：CHILD_UID 是主键，强制 1 个用户只有 1 个父级。"""

    __tablename__ = 'invite_relations'

    CHILD_UID: Mapped[int] = mapped_column(Integer, primary_key=True)
    PARENT_UID: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    CODE: Mapped[Optional[str]] = mapped_column(String(64), nullable=True, index=True)
    CREATED_AT: Mapped[int] = mapped_column(
        Integer, default=lambda: int(time.time()), nullable=False
    )


class InviteCodeModel(InviteDatabaseModel):
    __tablename__ = 'invite_codes'

    CODE: Mapped[str] = mapped_column(String(64), primary_key=True)
    INVITER_UID: Mapped[int] = mapped_column(Integer, nullable=False, index=True)
    # 被邀请人 Emby 账号开通天数；-1/0 表示永久
    DAYS: Mapped[int] = mapped_column(Integer, default=30, nullable=False)
    # 使用次数限制；-1 无限
    USE_COUNT_LIMIT: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    USE_COUNT: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    EXPIRES_AT: Mapped[int] = mapped_column(Integer, default=-1, nullable=False, index=True)
    # 用一次后写下使用者；未限制次数时仅记录最后一次
    USED_BY_UID: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    USED_AT: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    ACTIVE: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    CREATED_AT: Mapped[int] = mapped_column(
        Integer, default=lambda: int(time.time()), nullable=False
    )
    # 备注（生成人填写，例如送给谁的）
    NOTE: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)


_, InviteSessionFactory = init_async_db("invites", InviteDatabaseModel)


class InviteRelationOperate:
    @staticmethod
    async def add_relation(parent_uid: int, child_uid: int, code: Optional[str] = None) -> None:
        if parent_uid == child_uid:
            return
        async with InviteSessionFactory() as session:
            async with session.begin():
                existing = await session.get(InviteRelationModel, child_uid)
                if existing:
                    # 一个用户只能有一个父级；若已存在，保留旧关系
                    return
                rel = InviteRelationModel(
                    CHILD_UID=child_uid,
                    PARENT_UID=parent_uid,
                    CODE=code,
                    CREATED_AT=int(time.time()),
                )
                session.add(rel)

    @staticmethod
    async def get_parent_uid(child_uid: int) -> Optional[int]:
        async with InviteSessionFactory() as session:
            res = await session.get(InviteRelationModel, child_uid)
            return res.PARENT_UID if res else None

    @staticmethod
    async def get_children(parent_uid: int) -> List[int]:
        async with InviteSessionFactory() as session:
            rows = await session.execute(
                select(InviteRelationModel.CHILD_UID).where(
                    InviteRelationModel.PARENT_UID == parent_uid
                )
            )
            return [row[0] for row in rows.all()]

    @staticmethod
    async def get_all_relations() -> list[tuple[int, int]]:
        async with InviteSessionFactory() as session:
            rows = await session.execute(
                select(InviteRelationModel.PARENT_UID, InviteRelationModel.CHILD_UID)
            )
            return [(r[0], r[1]) for r in rows.all()]

    @staticmethod
    async def detach_child(child_uid: int) -> bool:
        """断开某个用户的上级关系（晋升为新树根）。"""
        async with InviteSessionFactory() as session:
            async with session.begin():
                res = await session.execute(
                    delete(InviteRelationModel).where(InviteRelationModel.CHILD_UID == child_uid)
                )
                return (res.rowcount or 0) > 0

    @staticmethod
    async def delete_relations_for_uid(uid: int) -> None:
        """删除涉及该 UID 的所有关系（既作父也作子）。子节点会因此晋升新树根。"""
        async with InviteSessionFactory() as session:
            async with session.begin():
                await session.execute(
                    delete(InviteRelationModel).where(
                        (InviteRelationModel.CHILD_UID == uid)
                        | (InviteRelationModel.PARENT_UID == uid)
                    )
                )

    @staticmethod
    async def reparent_children(old_parent_uid: int, new_parent_uid: Optional[int]) -> int:
        """将 old_parent 的所有子节点改挂到 new_parent；new_parent 为 None 时晋升为根。
        返回受影响的子节点数量。"""
        async with InviteSessionFactory() as session:
            async with session.begin():
                if new_parent_uid is None:
                    res = await session.execute(
                        delete(InviteRelationModel).where(
                            InviteRelationModel.PARENT_UID == old_parent_uid
                        )
                    )
                    return int(res.rowcount or 0)
                # 防止把孩子挂到自己头上造成环
                if new_parent_uid == old_parent_uid:
                    return 0
                res = await session.execute(
                    update(InviteRelationModel)
                    .where(InviteRelationModel.PARENT_UID == old_parent_uid)
                    .values(PARENT_UID=new_parent_uid)
                )
                return int(res.rowcount or 0)


class InviteCodeOperate:
    @staticmethod
    async def create_code(
        inviter_uid: int,
        days: int = 30,
        use_count_limit: int = 1,
        expires_at: int = -1,
        note: Optional[str] = None,
    ) -> InviteCodeModel:
        import secrets
        # 16 字节随机；和现有 regcode-* 区分前缀
        code = f"inv-{secrets.token_hex(16)}"
        item = InviteCodeModel(
            CODE=code,
            INVITER_UID=int(inviter_uid),
            DAYS=int(days) if days is not None else 30,
            USE_COUNT_LIMIT=int(use_count_limit) if use_count_limit is not None else 1,
            USE_COUNT=0,
            EXPIRES_AT=int(expires_at) if expires_at is not None else -1,
            ACTIVE=True,
            CREATED_AT=int(time.time()),
            NOTE=(note or '').strip()[:255] or None,
        )
        async with InviteSessionFactory() as session:
            async with session.begin():
                session.add(item)
                await session.flush()
                snapshot = InviteCodeModel(
                    CODE=item.CODE,
                    INVITER_UID=item.INVITER_UID,
                    DAYS=item.DAYS,
                    USE_COUNT_LIMIT=item.USE_COUNT_LIMIT,
                    USE_COUNT=item.USE_COUNT,
                    EXPIRES_AT=item.EXPIRES_AT,
                    USED_BY_UID=item.USED_BY_UID,
                    USED_AT=item.USED_AT,
                    ACTIVE=item.ACTIVE,
                    CREATED_AT=item.CREATED_AT,
                    NOTE=item.NOTE,
                )
        return snapshot

    @staticmethod
    async def get_code(code: str) -> Optional[InviteCodeModel]:
        async with InviteSessionFactory() as session:
            return await session.get(InviteCodeModel, code)

    @staticmethod
    async def list_by_inviter(inviter_uid: int) -> List[InviteCodeModel]:
        async with InviteSessionFactory() as session:
            rows = await session.execute(
                select(InviteCodeModel)
                .where(InviteCodeModel.INVITER_UID == inviter_uid)
                .order_by(InviteCodeModel.CREATED_AT.desc())
            )
            return list(rows.scalars().all())

    @staticmethod
    async def count_active_by_inviter(inviter_uid: int) -> int:
        """活跃 = ACTIVE 且未达使用上限。"""
        async with InviteSessionFactory() as session:
            rows = await session.execute(
                select(func.count()).select_from(InviteCodeModel).where(
                    InviteCodeModel.INVITER_UID == inviter_uid,
                    InviteCodeModel.ACTIVE == True,  # noqa: E712
                    (InviteCodeModel.USE_COUNT_LIMIT == -1)
                    | (InviteCodeModel.USE_COUNT < InviteCodeModel.USE_COUNT_LIMIT),
                )
            )
            return int(rows.scalar_one() or 0)

    @staticmethod
    async def consume(code: str, used_by_uid: int) -> bool:
        """消费一次邀请码：原子地把 USE_COUNT+1，并写入最后一次使用者。"""
        async with InviteSessionFactory() as session:
            async with session.begin():
                model = await session.get(InviteCodeModel, code)
                if not model or not model.ACTIVE:
                    return False
                if model.USE_COUNT_LIMIT != -1 and model.USE_COUNT >= model.USE_COUNT_LIMIT:
                    return False
                if model.EXPIRES_AT != -1 and int(time.time()) > model.EXPIRES_AT:
                    return False
                model.USE_COUNT += 1
                model.USED_BY_UID = used_by_uid
                model.USED_AT = int(time.time())
                # 用完后停用
                if model.USE_COUNT_LIMIT != -1 and model.USE_COUNT >= model.USE_COUNT_LIMIT:
                    model.ACTIVE = False
                await session.flush()
                return True

    @staticmethod
    async def deactivate(code: str, inviter_uid: Optional[int] = None) -> bool:
        async with InviteSessionFactory() as session:
            async with session.begin():
                model = await session.get(InviteCodeModel, code)
                if not model:
                    return False
                if inviter_uid is not None and model.INVITER_UID != inviter_uid:
                    return False
                model.ACTIVE = False
                await session.flush()
                return True

    @staticmethod
    async def delete(code: str, inviter_uid: Optional[int] = None) -> bool:
        async with InviteSessionFactory() as session:
            async with session.begin():
                model = await session.get(InviteCodeModel, code)
                if not model:
                    return False
                if inviter_uid is not None and model.INVITER_UID != inviter_uid:
                    return False
                await session.delete(model)
                return True

    @staticmethod
    async def delete_for_inviter(inviter_uid: int) -> int:
        async with InviteSessionFactory() as session:
            async with session.begin():
                res = await session.execute(
                    delete(InviteCodeModel).where(InviteCodeModel.INVITER_UID == inviter_uid)
                )
                return int(res.rowcount or 0)
