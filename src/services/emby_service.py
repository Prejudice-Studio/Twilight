"""
Emby 业务服务层

封装 Emby 相关的高级业务操作
"""

import json
import logging
from typing import Optional, List, Tuple, Dict, Any
from dataclasses import dataclass

from src.config import EmbyConfig, RegisterConfig
from src.db.user import UserModel, UserOperate, Role
from src.services.emby import (
    get_emby_client,
    EmbyClient,
    EmbyUser,
    EmbyLibrary,
    EmbySession,
    EmbyItem,
    EmbyError,
)
from src.core.utils import is_expired

logger = logging.getLogger(__name__)


@dataclass
class EmbyUserStatus:
    """Emby 用户状态"""

    user: Optional[UserModel]
    emby_user: Optional[EmbyUser]
    is_synced: bool  # 本地与 Emby 是否同步
    is_active: bool  # 是否活跃
    active_sessions: int  # 活跃会话数
    message: str


class EmbyService:
    """Emby 业务服务"""

    # ==================== 用户同步 ====================

    @staticmethod
    async def sync_user_from_emby(emby_id: str) -> Tuple[bool, str]:
        """
        从 Emby 同步用户信息到本地

        :param emby_id: Emby 用户 ID
        """
        emby = get_emby_client()

        try:
            emby_user = await emby.get_user(emby_id)
            if not emby_user:
                return False, "Emby 用户不存在"

            # 查找本地用户
            local_user = await UserOperate.get_user_by_embyid(emby_id)
            if not local_user:
                return False, "本地用户不存在"

            # 同步 Emby 用户名到 OTHER，不覆盖本地系统用户名
            other_data = {}
            if local_user.OTHER:
                try:
                    other_data = json.loads(local_user.OTHER)
                except (json.JSONDecodeError, TypeError):
                    other_data = {}
            if other_data.get("emby_username") != emby_user.name:
                other_data["emby_username"] = emby_user.name
                local_user.OTHER = json.dumps(other_data)
                await UserOperate.update_user(local_user)

            return True, "同步成功"
        except EmbyError as e:
            logger.error(f"同步用户失败: {e}")
            return False, f"同步失败: {e}"

    @staticmethod
    async def sync_all_users() -> Tuple[int, int, List[str]]:
        """
        批量同步所有用户状态（Twilight DB ↔ Emby）

        - 检测并清理孤儿记录（Emby 用户已删除但 DB 仍有 EMBYID）
        - 同步用户名不一致的情况
        - 同步启用/禁用状态
        - 强制关闭非管理员的「允许下载内容」权限（EnableContentDownloading=False）

        :return: (成功数, 失败数, 错误列表)
        """
        emby = get_emby_client()
        success_count = 0
        fail_count = 0
        errors = []
        download_disabled_count = 0

        try:
            emby_users = await emby.get_users()
            emby_user_map = {u.id: u for u in emby_users}

            local_users = await UserOperate.get_all_emby_users()

            for user in local_users:
                try:
                    emby_user = emby_user_map.get(user.EMBYID)

                    if emby_user is None:
                        # Emby 用户已被外部删除，清理本地记录
                        logger.warning(
                            f"Emby 用户已不存在，清理 EMBYID: "
                            f"{user.USERNAME} (UID: {user.UID}, EMBYID: {user.EMBYID})"
                        )
                        user.EMBYID = None
                        await UserOperate.update_user(user)
                        errors.append(f"{user.USERNAME}: Emby 用户不存在，已清理 EMBYID")
                        fail_count += 1
                        continue

                    # 同步 Emby 用户名到 OTHER，不覆盖本地系统用户名
                    other_data = {}
                    if user.OTHER:
                        try:
                            other_data = json.loads(user.OTHER)
                        except (json.JSONDecodeError, TypeError):
                            other_data = {}
                    if other_data.get("emby_username") != emby_user.name:
                        logger.info(
                            f"Emby 用户名不一致，更新 OTHER.emby_username: {other_data.get('emby_username')} -> {emby_user.name}"
                        )
                        other_data["emby_username"] = emby_user.name
                        user.OTHER = json.dumps(other_data)
                        await UserOperate.update_user(user)

                    from src.services.user_service import UserService

                    # 同步启用/禁用状态：过期导致的 Emby 禁用不应反向禁用系统账号。
                    emby_disabled = bool(emby_user.policy.get("IsDisabled", False))
                    if emby_disabled and user.ACTIVE_STATUS and not UserService.is_emby_access_expired(user):
                        logger.info(f"Emby 账户已被禁用，更新本地状态: {user.USERNAME} (UID: {user.UID})")
                        user.ACTIVE_STATUS = False
                        await UserOperate.update_user(user)

                    # 强制关闭非管理员的下载权限（防止历史遗留账户残留 True）
                    is_emby_admin = bool(emby_user.policy.get("IsAdministrator", False))
                    if not is_emby_admin and bool(emby_user.policy.get("EnableContentDownloading", True)):
                        try:
                            ok = await emby.update_user_policy(user.EMBYID, {"EnableContentDownloading": False})
                            if ok:
                                download_disabled_count += 1
                                logger.info(f"已关闭下载权限: {user.USERNAME} (UID: {user.UID})")
                        except EmbyError as exc:
                            logger.warning(f"关闭下载权限失败 {user.USERNAME} (UID: {user.UID}): {exc}")

                    ok, msg = await UserService.sync_user_to_emby(user)
                    if ok:
                        success_count += 1
                    else:
                        fail_count += 1
                        errors.append(f"{user.USERNAME}: {msg}")

                except Exception as e:
                    fail_count += 1
                    errors.append(f"{user.USERNAME}: {str(e)}")
                    logger.error(f"同步用户 {user.USERNAME} 失败: {e}")

            logger.info(
                f"批量同步完成: 成功 {success_count}, 失败 {fail_count}, " f"关闭下载权限 {download_disabled_count}"
            )
            return success_count, fail_count, errors
        except EmbyError as e:
            logger.error(f"同步所有用户失败: {e}")
            return 0, 0, [str(e)]

    # ==================== 用户状态检查 ====================

    @staticmethod
    async def get_user_status(user: UserModel) -> EmbyUserStatus:
        """获取用户完整状态"""
        emby = get_emby_client()

        emby_user = None
        is_synced = True
        active_sessions = 0
        message = "正常"

        if user.EMBYID:
            try:
                emby_user = await emby.get_user(user.EMBYID)
                if emby_user:
                    # 检查同步状态
                    emby_disabled = bool(emby_user.policy.get("IsDisabled", False))
                    from src.services.user_service import UserService

                    expected_disabled = not UserService.should_enable_emby_access(user)
                    if emby_user.name != user.USERNAME:
                        is_synced = False
                        message = "用户名不同步"
                    elif emby_disabled != expected_disabled:
                        is_synced = False
                        message = "账户启用状态与 Emby 不一致"

                    # 获取活跃会话
                    sessions = await emby.get_user_sessions(user.EMBYID)
                    active_sessions = len([s for s in sessions if s.is_active])
                else:
                    is_synced = False
                    message = "Emby 账户不存在"
            except EmbyError as e:
                message = f"无法连接 Emby: {e}"
        else:
            message = "未绑定 Emby 账户"

        is_active = user.ACTIVE_STATUS and not is_expired(user.EXPIRED_AT) and emby_user is not None

        return EmbyUserStatus(
            user=user,
            emby_user=emby_user,
            is_synced=is_synced,
            is_active=is_active,
            active_sessions=active_sessions,
            message=message,
        )

    @staticmethod
    async def check_expired_users() -> Tuple[List[UserModel], int]:
        """
        检查并处理过期用户

        :return: (过期用户列表, 禁用数量)
        """
        # TODO: 需要添加批量查询过期用户的数据库方法
        expired_users = []
        disabled_count = 0

        emby = get_emby_client()

        for user in expired_users:
            if user.EMBYID:
                try:
                    await emby.set_user_enabled(user.EMBYID, False)
                    disabled_count += 1
                except EmbyError as e:
                    logger.error(f"禁用用户 {user.USERNAME} 失败: {e}")

        return expired_users, disabled_count

    # ==================== 会话管理 ====================

    @staticmethod
    async def get_all_sessions() -> List[Dict[str, Any]]:
        """获取所有活跃会话及用户信息"""
        emby = get_emby_client()

        try:
            sessions = await emby.get_sessions()
            result = []

            for session in sessions:
                # 查找本地用户
                local_user = None
                if session.user_id:
                    local_user = await UserOperate.get_user_by_embyid(session.user_id)

                result.append(
                    {
                        "session_id": session.id,
                        "user_id": session.user_id,
                        "user_name": session.user_name,
                        "client": session.client,
                        "device_name": session.device_name,
                        "device_id": session.device_id,
                        "is_active": session.is_active,
                        "now_playing": session.now_playing_item.get("Name") if session.now_playing_item else None,
                        "local_user": (
                            {
                                "uid": local_user.UID,
                                "telegram_id": local_user.TELEGRAM_ID,
                            }
                            if local_user
                            else None
                        ),
                    }
                )

            return result
        except EmbyError as e:
            logger.error(f"获取会话失败: {e}")
            return []

    @staticmethod
    async def kick_user_sessions(user: UserModel) -> Tuple[bool, int]:
        """
        踢出用户所有会话

        :return: (成功, 踢出数量)
        """
        if not user.EMBYID:
            return False, 0

        emby = get_emby_client()

        try:
            sessions = await emby.get_user_sessions(user.EMBYID)
            kicked = 0

            for session in sessions:
                if await emby.kill_session(session.id):
                    kicked += 1

            return True, kicked
        except EmbyError as e:
            logger.error(f"踢出会话失败: {e}")
            return False, 0

    @staticmethod
    async def broadcast_message(header: str, text: str, user_ids: Optional[List[str]] = None) -> int:
        """
        广播消息到会话

        :param header: 消息标题
        :param text: 消息内容
        :param user_ids: 指定用户ID列表，为空则发送给所有人
        :return: 发送成功数量
        """
        emby = get_emby_client()

        try:
            sessions = await emby.get_sessions()
            sent = 0

            for session in sessions:
                if user_ids and session.user_id not in user_ids:
                    continue

                if await emby.send_message(session.id, header, text):
                    sent += 1

            return sent
        except EmbyError as e:
            logger.error(f"广播消息失败: {e}")
            return 0

    # ==================== 媒体库管理 ====================

    @staticmethod
    async def get_libraries_info() -> List[Dict[str, Any]]:
        """获取媒体库详细信息"""
        emby = get_emby_client()

        try:
            libraries = await emby.get_libraries()
            result = []

            for lib in libraries:
                result.append(
                    {
                        "id": lib.id,
                        "name": lib.name,
                        "type": lib.collection_type,
                    }
                )

            return result
        except EmbyError as e:
            logger.error(f"获取媒体库失败: {e}")
            return []

    @staticmethod
    def _normalize_library_names(names: Any) -> List[str]:
        """清理媒体库名称列表，去重保序。"""
        if not names:
            return []
        if isinstance(names, str):
            raw_items = [names]
        else:
            raw_items = list(names) if isinstance(names, (list, tuple, set)) else []

        result: List[str] = []
        seen: set[str] = set()
        for item in raw_items:
            value = str(item or "").strip()
            key = value.lower()
            if not value or key in seen:
                continue
            result.append(value)
            seen.add(key)
        return result

    @staticmethod
    def default_hidden_library_names() -> List[str]:
        """配置中的默认隐藏媒体库。"""
        return EmbyService._normalize_library_names(EmbyConfig.EMBY_DEFAULT_HIDDEN_LIBRARIES)

    @staticmethod
    def self_service_library_names() -> List[str]:
        """配置中允许用户自行显示/隐藏的媒体库。"""
        return EmbyService._normalize_library_names(EmbyConfig.EMBY_SELF_SERVICE_LIBRARIES)

    @staticmethod
    def can_self_service_libraries(user: UserModel) -> bool:
        """用户是否被管理员授予媒体库自助显隐入口权限。"""
        return bool(getattr(user, "LIBRARY_SELF_SERVICE", False))

    @staticmethod
    async def resolve_library_names_to_ids(library_names: List[str]) -> Tuple[List[str], List[str]]:
        """
        将媒体库名称列表解析为 Emby 内部 ID 列表

        :param library_names: 媒体库名称列表
        :return: (解析成功的 ID 列表, 未找到的名称列表)
        """
        if not library_names:
            return [], []

        emby = get_emby_client()

        try:
            libraries = await emby.get_libraries()
            # 构建名称到ID的映射（不区分大小写）
            name_to_id = {lib.name.strip().lower(): lib.id for lib in libraries}

            resolved_ids = []
            not_found = []
            for name in library_names:
                name_lower = name.strip().lower()
                if name_lower in name_to_id:
                    resolved_ids.append(name_to_id[name_lower])
                else:
                    not_found.append(name)

            return resolved_ids, not_found
        except EmbyError as e:
            logger.error(f"解析媒体库名称失败: {e}")
            return [], list(library_names)

    @staticmethod
    async def set_user_library_access(
        user: UserModel, library_ids: List[str], enable_all: bool = False
    ) -> Tuple[bool, str]:
        """设置用户可访问的媒体库（管理员严格白名单场景）。

        - enable_all=True：用户可见所有库
        - enable_all=False：仅 library_ids 指定的库可见，其余按名称加入 BlockedMediaFolders

        实现委托给 `EmbyClient.set_user_libraries`（基于 Sakura 单次 POST 模式）。
        """
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"

        emby = get_emby_client()

        try:
            success = await emby.set_user_libraries(
                user_id=user.EMBYID,
                allowed_library_ids=list(library_ids or []),
                enable_all=bool(enable_all),
            )
            if success:
                return True, "媒体库权限已更新"
            return False, "更新失败"
        except EmbyError as e:
            logger.error(f"设置媒体库权限失败: {e}")
            return False, f"操作失败: {e}"

    @staticmethod
    async def set_user_library_visibility(user: UserModel, library_names: List[str], visible: bool) -> Tuple[bool, str]:
        """按名称增量显示或隐藏用户的指定媒体库。"""
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"

        names = EmbyService._normalize_library_names(library_names)
        if not names:
            return False, "媒体库名称不能为空"

        emby = get_emby_client()
        try:
            success = await (
                emby.show_folders_by_names(user.EMBYID, names)
                if visible
                else emby.hide_folders_by_names(user.EMBYID, names)
            )
            if success:
                return True, "媒体库已显示" if visible else "媒体库已隐藏"
            return False, "更新失败"
        except EmbyError as e:
            logger.error(f"更新媒体库显隐失败: {e}")
            return False, f"操作失败: {e}"

    @staticmethod
    async def apply_default_hidden_libraries_by_emby_id(emby_id: str) -> bool:
        """对指定 Emby 用户应用默认隐藏库配置；配置为空时直接成功。"""
        names = EmbyService.default_hidden_library_names()
        if not emby_id or not names:
            return True
        try:
            return await get_emby_client().hide_folders_by_names(emby_id, names)
        except EmbyError as e:
            logger.warning(f"应用默认隐藏媒体库失败 emby_id={emby_id}: {e}")
            return False

    @staticmethod
    async def apply_default_hidden_libraries(user: UserModel) -> bool:
        """为普通用户应用默认隐藏库；管理员/白名单不自动收紧。"""
        if not user or not user.EMBYID:
            return True
        if user.ROLE in (Role.ADMIN.value, Role.WHITE_LIST.value):
            return True
        return await EmbyService.apply_default_hidden_libraries_by_emby_id(user.EMBYID)

    @staticmethod
    async def get_user_library_access(user: UserModel) -> Tuple[List[str], bool]:
        """
        获取用户媒体库访问权限

        :return: (可访问的媒体库ID列表, 是否全部可访问)
        """
        if not user.EMBYID:
            return [], False

        emby = get_emby_client()

        try:
            emby_user = await emby.get_user(user.EMBYID)
            if not emby_user:
                return [], False

            enable_all = emby_user.policy.get("EnableAllFolders", True)
            enabled_folders = emby_user.policy.get("EnabledFolders", [])

            return enabled_folders, enable_all
        except EmbyError as e:
            logger.error(f"获取媒体库权限失败: {e}")
            return [], False

    @staticmethod
    async def get_user_library_access_detail(
        user: UserModel,
        *,
        include_self_service_config: bool = False,
    ) -> Dict[str, Any]:
        """获取用户媒体库权限详情，供前端管理和自助显隐使用。"""
        all_libraries = await EmbyService.get_libraries_info()
        default_hidden = EmbyService.default_hidden_library_names()
        self_service_enabled = EmbyService.can_self_service_libraries(user)
        self_service_config = EmbyService.self_service_library_names()
        self_service = self_service_config if (include_self_service_config or self_service_enabled) else []

        if not user.EMBYID:
            return {
                "has_emby": False,
                "enable_all": False,
                "enabled_ids": [],
                "blocked_names": [],
                "all_libraries": [],
                "libraries": [],
                "default_hidden_libraries": default_hidden,
                "self_service_libraries": self_service,
                "self_service_enabled": self_service_enabled,
            }

        enabled_ids: List[str] = []
        enable_all = False
        blocked_names: List[str] = []
        try:
            state = await get_emby_client().get_current_folder_state(user.EMBYID)
            if state is not None:
                enabled_ids, enable_all, blocked_names = state
        except EmbyError as e:
            logger.error(f"获取媒体库权限详情失败: {e}")

        blocked_keys = {(name or "").strip().lower() for name in blocked_names}
        if enable_all:
            visible_libraries = [lib for lib in all_libraries if (lib.get("name") or "").strip().lower() not in blocked_keys]
        else:
            enabled_set = set(enabled_ids)
            visible_libraries = [lib for lib in all_libraries if lib.get("id") in enabled_set]

        return {
            "has_emby": True,
            "enable_all": enable_all,
            "enabled_ids": enabled_ids,
            "blocked_names": blocked_names,
            "all_libraries": all_libraries,
            "libraries": visible_libraries,
            "default_hidden_libraries": default_hidden,
            "self_service_libraries": self_service,
            "self_service_enabled": self_service_enabled,
        }

    # ==================== 设备管理 ====================

    @staticmethod
    async def get_user_devices(user: UserModel) -> List[Dict[str, Any]]:
        """获取用户的设备列表"""
        if not user.EMBYID:
            return []

        emby = get_emby_client()

        try:
            all_devices = await emby.get_devices()
            user_devices = [d for d in all_devices if d.get("UserId") == user.EMBYID]

            return [
                {
                    "id": d.get("Id"),
                    "name": d.get("Name"),
                    "app_name": d.get("AppName"),
                    "app_version": d.get("AppVersion"),
                    "last_user_name": d.get("LastUserName"),
                    "date_last_activity": d.get("DateLastActivity"),
                }
                for d in user_devices
            ]
        except EmbyError as e:
            logger.error(f"获取设备失败: {e}")
            return []

    @staticmethod
    async def remove_user_device(user: UserModel, device_id: str) -> Tuple[bool, str]:
        """移除用户设备"""
        if not user.EMBYID:
            return False, "用户没有关联的 Emby 账户"

        emby = get_emby_client()

        try:
            # 验证设备属于该用户
            devices = await emby.get_devices()
            device = next((d for d in devices if d.get("Id") == device_id), None)

            if not device:
                return False, "设备不存在"

            if device.get("UserId") != user.EMBYID:
                return False, "该设备不属于此用户"

            success = await emby.delete_device(device_id)
            if success:
                return True, "设备已移除"
            return False, "移除失败"
        except EmbyError as e:
            logger.error(f"移除设备失败: {e}")
            return False, f"操作失败: {e}"

    # ==================== 服务器管理 ====================

    @staticmethod
    async def get_server_status() -> Dict[str, Any]:
        """获取服务器状态"""
        emby = get_emby_client()

        try:
            is_online = await emby.ping()
            if not is_online:
                return {
                    "online": False,
                    "message": "服务器离线",
                }

            info = await emby.get_server_info()
            sessions = await emby.get_sessions()

            return {
                "online": True,
                "server_name": info.get("ServerName"),
                "version": info.get("Version"),
                "operating_system": info.get("OperatingSystemDisplayName"),
                "active_sessions": len([s for s in sessions if s.is_active]),
                "total_sessions": len(sessions),
            }
        except EmbyError as e:
            return {
                "online": False,
                "message": str(e),
            }

    @staticmethod
    async def get_activity_log(limit: int = 50) -> List[Dict[str, Any]]:
        """获取活动日志"""
        emby = get_emby_client()

        try:
            data = await emby.get_activity_log(limit=limit)
            items = data.get("Items", []) if data else []

            return [
                {
                    "id": item.get("Id"),
                    "name": item.get("Name"),
                    "type": item.get("Type"),
                    "date": item.get("Date"),
                    "user_id": item.get("UserId"),
                    "severity": item.get("Severity"),
                    "short_overview": item.get("ShortOverview"),
                }
                for item in items
            ]
        except EmbyError as e:
            logger.error(f"获取活动日志失败: {e}")
            return []

    # ==================== 媒体搜索 ====================

    @staticmethod
    async def search_media(query: str, limit: int = 20) -> List[Dict[str, Any]]:
        """搜索媒体"""
        emby = get_emby_client()

        try:
            items = await emby.search_items(query, limit)

            return [
                {
                    "id": item.id,
                    "name": item.name,
                    "type": item.type,
                    "year": item.year,
                    "overview": item.overview[:200] + "..." if len(item.overview) > 200 else item.overview,
                }
                for item in items
            ]
        except EmbyError as e:
            logger.error(f"搜索媒体失败: {e}")
            return []

    @staticmethod
    async def get_latest_media(item_types: List[str] = None, limit: int = 20) -> List[Dict[str, Any]]:
        """获取最新媒体"""
        emby = get_emby_client()

        try:
            if item_types is None:
                item_types = ["Movie", "Series"]

            data = await emby.get_items(
                item_types=item_types, limit=limit, sort_by="DateCreated", sort_order="Descending"
            )

            items = data.get("Items", []) if data else []

            return [
                {
                    "id": item.get("Id"),
                    "name": item.get("Name"),
                    "type": item.get("Type"),
                    "year": item.get("ProductionYear"),
                    "date_created": item.get("DateCreated"),
                }
                for item in items
            ]
        except EmbyError as e:
            logger.error(f"获取最新媒体失败: {e}")
            return []
