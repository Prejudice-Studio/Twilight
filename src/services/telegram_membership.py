"""Telegram 群组成员资格校验服务。

集中处理「检查某个 Telegram 用户是否仍在配置中的群组里」这件事，
供绑定流程与定时任务复用。Bot 未运行 / 没有配置群组时一律放行，
仅在 Bot 报告"不是群成员"或返回明确的 BadRequest 时判定为不在群。

调用方约定：
    ok, missing = await TelegramMembershipService.check_user_in_groups(tg_id)
    - ok=True 表示满足所有必需群组（或不需要校验）
    - missing 是结构化数组，方便给到 Bot/前端做友好提示
"""
from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass
from typing import List, Optional, Tuple, Union

from src.config import TelegramConfig
from src.services.telegram_runtime import has_telegram_api_access, run_bot_operation

logger = logging.getLogger(__name__)


@dataclass
class MissingGroup:
    id: str
    title: Optional[str] = None
    url: Optional[str] = None

    def to_dict(self) -> dict:
        return {"id": self.id, "title": self.title, "url": self.url}


def _normalize_group_ids() -> List[Union[int, str]]:
    raw = TelegramConfig.GROUP_ID
    if not raw:
        return []
    if isinstance(raw, (int, str)):
        return [raw]
    if isinstance(raw, list):
        return [g for g in raw if g not in (None, "")]
    return []


def _build_invite_url(group_id: Union[int, str], chat) -> Optional[str]:
    """根据群组 ID/Chat 返回可点击的链接（仅公开群可生成 t.me 链接）。"""
    if chat is not None:
        username = getattr(chat, "username", None)
        if username:
            return f"https://t.me/{username}"
    sid = str(group_id).strip()
    if sid.startswith("@"):
        return f"https://t.me/{sid[1:]}"
    return None


class TelegramMembershipService:
    """检查/枚举 Telegram 群组成员资格。"""

    @staticmethod
    def required_group_ids() -> List[Union[int, str]]:
        return _normalize_group_ids()

    @staticmethod
    def enforcement_enabled() -> bool:
        """`TelegramConfig.REQUIRE_GROUP_MEMBERSHIP` 开启 + 至少配置了一个群组时返回 True。"""
        return bool(getattr(TelegramConfig, "REQUIRE_GROUP_MEMBERSHIP", False)) and bool(
            _normalize_group_ids()
        )

    @staticmethod
    def is_bot_available() -> bool:
        """当前配置是否允许执行 Telegram 成员资格校验。"""
        return has_telegram_api_access()

    @staticmethod
    async def check_user_in_groups(
        telegram_id: int,
        *,
        strict: bool = True,
    ) -> Tuple[bool, List[MissingGroup]]:
        """检查 Telegram 用户是否在全部必需群组内。

        :param telegram_id: 待检查的用户 ID。
        :param strict: True 时网络异常/Bot 异常视为「不在群」（拦截更紧）；
                       False 时网络异常视为「未知」放行（定时任务建议 False 避免误封）。
        :return: ``(ok, missing_groups)`` —— ``ok`` 为 True 表示通过；
                 ``missing_groups`` 是用户未加入的群组明细。
        """
        group_ids = _normalize_group_ids()
        if not group_ids:
            return True, []
        if not telegram_id:
            return False, []

        if not has_telegram_api_access():
            logger.info("Telegram API 不可用，跳过群组成员资格检查")
            return (not strict), []

        # 延迟导入 telegram.error，避免顶层依赖
        try:
            from telegram.error import BadRequest, TelegramError, Forbidden
        except Exception:
            BadRequest = TelegramError = Forbidden = Exception  # type: ignore

        async def _check_with_bot(bot) -> Tuple[bool, List[MissingGroup]]:
            missing: List[MissingGroup] = []

            # 并发查询所有群组成员资格
            async def _probe_one(gid: Union[int, str]) -> Optional[MissingGroup]:
                chat = None
                try:
                    # 先尽量拿一次群组元信息，便于失败时返回标题/链接
                    try:
                        chat = await bot.get_chat(gid)
                    except Exception:
                        chat = None

                    member = await bot.get_chat_member(gid, telegram_id)
                    status = str(getattr(member, "status", "") or "").lower()
                    if status in ("left", "kicked"):
                        return MissingGroup(
                            id=str(gid),
                            title=getattr(chat, "title", None) if chat else None,
                            url=_build_invite_url(gid, chat),
                        )
                    return None
                except BadRequest as exc:
                    msg = str(exc).lower()
                    if (
                        "not found" in msg
                        or "user not found" in msg
                        or "participant" in msg
                        or "member list is inaccessible" in msg
                    ):
                        return MissingGroup(
                            id=str(gid),
                            title=getattr(chat, "title", None) if chat else None,
                            url=_build_invite_url(gid, chat),
                        )
                    logger.warning(
                        f"检查群组 {gid} 成员资格 BadRequest (tg_id={telegram_id}): {exc}"
                    )
                    if strict:
                        return MissingGroup(
                            id=str(gid),
                            title=getattr(chat, "title", None) if chat else None,
                            url=_build_invite_url(gid, chat),
                        )
                    return None
                except Forbidden as exc:
                    logger.warning(
                        f"Bot 缺少群 {gid} 的查看权限 (tg_id={telegram_id}): {exc}"
                    )
                    # Bot 没权限就别拦人，否则一旦群里失权就全员被踢
                    return None
                except TelegramError as exc:
                    logger.warning(
                        f"检查群组 {gid} Telegram 异常 (tg_id={telegram_id}): {exc}"
                    )
                    if strict:
                        return MissingGroup(
                            id=str(gid),
                            title=getattr(chat, "title", None) if chat else None,
                            url=_build_invite_url(gid, chat),
                        )
                    return None
                except Exception as exc:  # pragma: no cover - safety net
                    logger.warning(
                        f"检查群组 {gid} 未知异常 (tg_id={telegram_id}): {exc}"
                    )
                    if strict:
                        return MissingGroup(
                            id=str(gid),
                            title=getattr(chat, "title", None) if chat else None,
                            url=_build_invite_url(gid, chat),
                        )
                    return None

            results = await asyncio.gather(*[_probe_one(gid) for gid in group_ids])
            for r in results:
                if r is not None:
                    missing.append(r)

            return (len(missing) == 0), missing

        try:
            return await run_bot_operation(_check_with_bot, timeout=30)
        except Exception as exc:
            logger.warning(f"执行群组成员资格检查失败 (tg_id={telegram_id}): {exc}")
            return (not strict), []

    @staticmethod
    def format_missing_message(missing: List[MissingGroup]) -> str:
        if not missing:
            return ""
        lines = ["请先加入以下群组后再绑定 Telegram："]
        for g in missing:
            label = g.title or g.id
            if g.url:
                lines.append(f"• {label} ({g.url})")
            else:
                lines.append(f"• {label}")
        return "\n".join(lines)
