"""邀请树业务服务

约定术语
========
- 树根 (root)：邀请关系图中没有 PARENT_UID 的节点。一个用户初始默认就是根；
  被某个上级邀请后，其根属性转移到上级链路。
- 层级 (depth)：从该节点向下递归延伸的层数。INVITE_MAX_DEPTH 控制全局深度上限，
  例如 3 表示允许 B->A->C 三层（B 是根、A 是中间、C 是叶子）。
- 子树 (subtree)：以某个节点为根的所有后代集合。

操作语义
========
- ``ensure_can_invite(inviter)``：邀请人是否可以生成新邀请码（启用开关 + 自身深度 + 上限）。
- ``apply_invite(invitee, code)``：被邀请人使用邀请码注册成功后调用，建立 parent->child 关系。
- ``delete_user_keep_subtree(uid)``：仅删除该用户，子节点全部晋升为新树根。
- ``delete_user_with_subtree(uid, depth)``：以该用户为根做 BFS，仅 depth 层级及以下被一并删除。
"""
from __future__ import annotations

import logging
from collections import deque
from typing import Optional

from src.config import RegisterConfig
from src.db.invite import (
    InviteCodeOperate,
    InviteRelationOperate,
)
from src.db.user import UserOperate, UserModel

logger = logging.getLogger(__name__)


class InviteService:
    @staticmethod
    def is_enabled() -> bool:
        return bool(RegisterConfig.INVITE_ENABLED)

    @staticmethod
    def max_depth() -> int:
        try:
            return max(1, int(RegisterConfig.INVITE_MAX_DEPTH or 1))
        except (TypeError, ValueError):
            return 3

    # -------------------- 树结构计算 --------------------
    @staticmethod
    async def _build_adjacency() -> tuple[dict[int, list[int]], dict[int, int]]:
        """返回 (parent->children, child->parent)。一次扫描，O(关系数)。"""
        rels = await InviteRelationOperate.get_all_relations()
        children: dict[int, list[int]] = {}
        parent_of: dict[int, int] = {}
        for parent_uid, child_uid in rels:
            children.setdefault(parent_uid, []).append(child_uid)
            parent_of[child_uid] = parent_uid
        return children, parent_of

    @staticmethod
    async def get_ancestor_depth(uid: int) -> int:
        """该节点向上回溯到根的层数 (根 = 1)。带环保护。"""
        _, parent_of = await InviteService._build_adjacency()
        depth = 1
        cur = uid
        visited = {cur}
        while cur in parent_of:
            cur = parent_of[cur]
            if cur in visited:  # 环防护（数据异常时）
                break
            visited.add(cur)
            depth += 1
            if depth > 64:  # 兜底
                break
        return depth

    @staticmethod
    async def get_subtree_depth(uid: int, children_map: dict[int, list[int]]) -> int:
        """以 uid 为根的子树高度 (单节点 = 1)。"""
        max_depth = 1
        queue: deque[tuple[int, int]] = deque([(uid, 1)])
        visited: set[int] = {uid}
        while queue:
            node, depth = queue.popleft()
            max_depth = max(max_depth, depth)
            for child in children_map.get(node, []):
                if child in visited:
                    continue
                visited.add(child)
                queue.append((child, depth + 1))
        return max_depth

    @staticmethod
    async def collect_subtree_uids(
        root_uid: int,
        max_levels: Optional[int] = None,
    ) -> list[int]:
        """以 root_uid 为根 BFS 收集 ``max_levels`` 层及以下的所有节点。

        ``max_levels=None`` 收集全部；``max_levels=1`` 只返回根本身；
        ``max_levels=2`` 返回根 + 直接子节点；以此类推。
        """
        children_map, _ = await InviteService._build_adjacency()
        result: list[int] = []
        queue: deque[tuple[int, int]] = deque([(root_uid, 1)])
        visited: set[int] = {root_uid}
        while queue:
            node, depth = queue.popleft()
            result.append(node)
            if max_levels is not None and depth >= max_levels:
                continue
            for child in children_map.get(node, []):
                if child in visited:
                    continue
                visited.add(child)
                queue.append((child, depth + 1))
        return result

    # -------------------- 校验 --------------------
    @staticmethod
    async def ensure_can_invite(inviter: UserModel) -> tuple[bool, str]:
        if not InviteService.is_enabled():
            return False, "邀请系统未启用"
        if not inviter or not getattr(inviter, 'UID', None):
            return False, "无效的邀请人"
        if not inviter.ACTIVE_STATUS:
            return False, "账户已被禁用，无法生成邀请码"
        if RegisterConfig.INVITE_REQUIRE_EMBY and not inviter.EMBYID:
            return False, "请先绑定 Emby 账号后再生成邀请码"

        max_depth = InviteService.max_depth()
        ancestor_depth = await InviteService.get_ancestor_depth(inviter.UID)
        if ancestor_depth >= max_depth:
            return False, f"已达到最大邀请层级 ({max_depth})，不能再向下邀请"

        # 数量上限
        limit = RegisterConfig.INVITE_LIMIT
        if limit is not None and limit != -1:
            active = await InviteCodeOperate.count_active_by_inviter(inviter.UID)
            if active >= int(limit):
                return False, f"未使用的邀请码已达上限 ({limit})，请先撤销旧的"
        return True, ""

    # -------------------- 用法 --------------------
    @staticmethod
    async def apply_invite(invitee_uid: int, code: str) -> tuple[bool, str, Optional[int]]:
        """被邀请人使用邀请码成功创建账号 / Emby 账号后调用，落 parent->child 关系。

        返回 (ok, msg, inviter_uid)。
        """
        if not InviteService.is_enabled():
            return False, "邀请系统未启用", None
        code_info = await InviteCodeOperate.get_code(code)
        if not code_info or not code_info.ACTIVE:
            return False, "邀请码无效或已停用", None
        inviter_uid = code_info.INVITER_UID
        if inviter_uid == invitee_uid:
            return False, "不能使用自己生成的邀请码", None
        # 已有上级则保留旧关系；这里只判断是否能再下挂一层
        ancestor_depth = await InviteService.get_ancestor_depth(inviter_uid)
        if ancestor_depth + 1 > InviteService.max_depth():
            return False, "该邀请会超过最大层级限制", None

        ok = await InviteCodeOperate.consume(code, invitee_uid)
        if not ok:
            return False, "邀请码已被使用或已过期", None

        await InviteRelationOperate.add_relation(
            parent_uid=inviter_uid, child_uid=invitee_uid, code=code,
        )
        logger.info(
            f"邀请关系建立: inviter={inviter_uid} child={invitee_uid} code={code}"
        )
        return True, "邀请关系已建立", inviter_uid

    # -------------------- 删除 --------------------
    @staticmethod
    async def delete_user_keep_subtree(uid: int) -> int:
        """仅删除该用户的关系节点；子节点全部晋升为新树根。

        实际的本地 / Emby 用户删除由 UserService 完成。这里只清理树结构。
        返回受影响子节点数量。
        """
        children = await InviteRelationOperate.get_children(uid)
        # 切断子节点的父指针（直接删除该用户为 PARENT 的所有边）
        await InviteRelationOperate.reparent_children(uid, None)
        # 删除该用户作为 CHILD 的边（如果有）
        await InviteRelationOperate.detach_child(uid)
        # 把该用户名下还没使用的邀请码作废（避免悬空）
        await InviteCodeOperate.delete_for_inviter(uid)
        return len(children)

    @staticmethod
    async def collect_uids_to_delete(uid: int, depth_levels: int) -> list[int]:
        """收集 "以 uid 为根 + 向下递归 ``depth_levels`` 层" 的所有 UID。

        depth_levels=1 仅本人；2 含直接子节点；以此类推；<=0 视为仅本人。
        """
        if depth_levels <= 0:
            return [uid]
        return await InviteService.collect_subtree_uids(uid, max_levels=depth_levels)


    # -------------------- 视图 --------------------
    @staticmethod
    async def build_forest_view() -> dict:
        """提供给可视化的整棵森林结构。

        返回结构::

            {
                "nodes": [{"uid": .., "username": .., "role": .., "emby_id": .., ...}],
                "edges": [{"parent": .., "child": .., "code": .., "created_at": ..}],
                "roots": [uid, ...],
                "max_depth": int,
                "config": {"enabled": bool, "max_depth": int, "invite_limit": int},
            }
        """
        rels = await InviteRelationOperate.get_all_relations()

        children_map: dict[int, list[int]] = {}
        parent_of: dict[int, int] = {}
        all_uids: set[int] = set()
        for parent_uid, child_uid in rels:
            children_map.setdefault(parent_uid, []).append(child_uid)
            parent_of[child_uid] = parent_uid
            all_uids.add(parent_uid)
            all_uids.add(child_uid)

        # 邀请码生成者也算节点（即便还没成功邀请到人）
        async def _inviter_uids() -> set[int]:
            # 一次性扫描所有邀请码以补全孤立节点
            from src.db.invite import InviteSessionFactory, InviteCodeModel
            from sqlalchemy import select as _select
            async with InviteSessionFactory() as session:
                rows = await session.execute(_select(InviteCodeModel.INVITER_UID).distinct())
                return {row[0] for row in rows.all()}

        all_uids |= await _inviter_uids()

        # 拉一次用户信息
        nodes: list[dict] = []
        for uid in sorted(all_uids):
            user = await UserOperate.get_user_by_uid(uid)
            if not user:
                continue
            nodes.append({
                'uid': user.UID,
                'username': user.USERNAME,
                'role': user.ROLE,
                'emby_id': user.EMBYID or None,
                'active': bool(user.ACTIVE_STATUS),
                'telegram_id': user.TELEGRAM_ID,
                'register_time': user.REGISTER_TIME,
                'expired_at': user.EXPIRED_AT,
                'is_root': uid not in parent_of,
            })

        edges = [
            {
                'parent': parent_uid,
                'child': child_uid,
            }
            for parent_uid, child_uid in rels
        ]

        roots = [n['uid'] for n in nodes if n['is_root']]
        # 整体最大深度
        global_depth = 0
        for r in roots:
            global_depth = max(
                global_depth,
                await InviteService.get_subtree_depth(r, children_map),
            )

        return {
            'nodes': nodes,
            'edges': edges,
            'roots': roots,
            'max_depth': global_depth,
            'config': {
                'enabled': InviteService.is_enabled(),
                'max_depth': InviteService.max_depth(),
                'invite_limit': RegisterConfig.INVITE_LIMIT,
                'require_emby': bool(RegisterConfig.INVITE_REQUIRE_EMBY),
            },
        }
