"""
Emby/Jellyfin API 客户端

基于官方 API 实现的异步客户端
参考:
- https://github.com/MediaBrowser/Emby.ApiClients
- https://github.com/jellyfin/jellyfin-apiclient-python
"""

import logging
import time
from typing import Optional, Dict, Any, List, Tuple
from dataclasses import dataclass, field
from enum import Enum

import httpx

from src.config import Config, EmbyConfig

logger = logging.getLogger(__name__)


# ==================== 异常类 ====================


class EmbyError(Exception):
    """Emby API 错误基类"""

    pass


class EmbyAuthError(EmbyError):
    """认证错误"""

    pass


class EmbyNotFoundError(EmbyError):
    """资源未找到"""

    pass


class EmbyConnectionError(EmbyError):
    """连接错误"""

    pass


# ==================== 数据类 ====================


@dataclass
class EmbyUser:
    """Emby 用户信息"""

    id: str
    name: str
    server_id: str = ""
    policy: Dict[str, Any] = field(default_factory=dict)
    configuration: Dict[str, Any] = field(default_factory=dict)
    has_password: bool = False
    has_configured_password: bool = False
    last_login_date: Optional[str] = None
    last_activity_date: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "EmbyUser":
        return cls(
            id=data.get("Id", ""),
            name=data.get("Name", ""),
            server_id=data.get("ServerId", ""),
            policy=data.get("Policy", {}),
            configuration=data.get("Configuration", {}),
            has_password=data.get("HasPassword", False),
            has_configured_password=data.get("HasConfiguredPassword", False),
            last_login_date=data.get("LastLoginDate"),
            last_activity_date=data.get("LastActivityDate"),
        )


@dataclass
class EmbyLibrary:
    """Emby 媒体库信息。

    注意：`id` 优先使用 ItemId/Guid（VirtualFolders 返回的 GUID），
    因为 `EnabledFolders` 策略字段需要这个 GUID。
    """

    id: str
    name: str
    collection_type: str
    item_id: str = ""

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "EmbyLibrary":
        # /Library/VirtualFolders 返回的库以 ItemId / Guid 表示，这才是 EnabledFolders 接受的值
        # /Items 或其他端点可能返回 Id；按优先级回退取值
        item_id = data.get("ItemId") or data.get("Guid") or data.get("Id", "")
        return cls(
            id=item_id,
            name=data.get("Name", ""),
            collection_type=data.get("CollectionType", ""),
            item_id=item_id,
        )


@dataclass
class EmbySession:
    """Emby 会话信息"""

    id: str
    user_id: str
    user_name: str
    client: str
    device_name: str
    device_id: str
    application_version: str
    is_active: bool
    now_playing_item: Optional[Dict[str, Any]] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "EmbySession":
        return cls(
            id=data.get("Id", ""),
            user_id=data.get("UserId", ""),
            user_name=data.get("UserName", ""),
            client=data.get("Client", ""),
            device_name=data.get("DeviceName", ""),
            device_id=data.get("DeviceId", ""),
            application_version=data.get("ApplicationVersion", ""),
            is_active=data.get("IsActive", False),
            now_playing_item=data.get("NowPlayingItem"),
        )


@dataclass
class EmbyItem:
    """Emby 媒体项"""

    id: str
    name: str
    type: str
    overview: str = ""
    year: Optional[int] = None
    parent_id: str = ""
    series_name: str = ""
    season_name: str = ""
    index_number: Optional[int] = None
    parent_index_number: Optional[int] = None
    premiere_date: str = ""
    original_title: str = ""
    sort_name: str = ""
    external_ids: Dict[str, str] = field(default_factory=dict)
    provider_ids: Dict[str, str] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "EmbyItem":
        return cls(
            id=data.get("Id", ""),
            name=data.get("Name", ""),
            type=data.get("Type", ""),
            overview=data.get("Overview", ""),
            year=data.get("ProductionYear"),
            parent_id=data.get("ParentId", ""),
            series_name=data.get("SeriesName", ""),
            season_name=data.get("SeasonName", ""),
            index_number=data.get("IndexNumber"),
            parent_index_number=data.get("ParentIndexNumber"),
            premiere_date=data.get("PremiereDate", ""),
            original_title=data.get("OriginalTitle", ""),
            sort_name=data.get("SortName", ""),
            external_ids=data.get("ExternalIds", {}),
            provider_ids=data.get("ProviderIds", {}),
        )

    @property
    def tmdb_id(self) -> Optional[str]:
        """获取 TMDB ID"""
        return self.provider_ids.get("Tmdb")

    @property
    def imdb_id(self) -> Optional[str]:
        """获取 IMDB ID"""
        return self.provider_ids.get("Imdb")

    @property
    def tvdb_id(self) -> Optional[str]:
        """获取 TVDB ID"""
        return self.provider_ids.get("Tvdb")

    def to_dict(self) -> Dict[str, Any]:
        return {
            "id": self.id,
            "name": self.name,
            "type": self.type,
            "overview": self.overview,
            "year": self.year,
            "series_name": self.series_name,
            "original_title": self.original_title,
            "premiere_date": self.premiere_date,
            "tmdb_id": self.tmdb_id,
            "imdb_id": self.imdb_id,
        }


# ==================== 客户端 ====================


class EmbyClient:
    """
    Emby/Jellyfin API 异步客户端

    支持 Emby 和 Jellyfin 服务器的核心 API 操作
    """

    def __init__(
        self,
        base_url: Optional[str] = None,
        api_key: Optional[str] = None,
        proxy: Optional[str] = None,
        timeout: float = 30.0,
        device_name: str = "Twilight",
        device_id: str = "twilight-client",
        app_name: str = "Twilight",
        app_version: str = "1.0.0",
    ):
        self.base_url = (base_url or EmbyConfig.EMBY_URL).rstrip("/")
        self.api_key = api_key or EmbyConfig.EMBY_TOKEN
        self.proxy = proxy
        self.timeout = timeout

        # 备用认证凭据
        self._admin_username = EmbyConfig.EMBY_USERNAME
        self._admin_password = EmbyConfig.EMBY_PASSWORD
        self._auth_fallback_attempted = False

        # Emby 用户列表缓存，减少短时间内重复请求
        self._users_cache: Dict[tuple[Optional[bool], Optional[bool]], tuple[float, List[EmbyUser]]] = {}
        self._users_cache_ttl = 15.0

        # Emby 媒体库缓存，减少短时间内重复请求
        self._libraries_cache: Optional[tuple[float, List[EmbyLibrary]]] = None
        self._libraries_cache_ttl = 30.0

        # 设备信息
        self.device_name = device_name
        self.device_id = device_id
        self.app_name = app_name
        self.app_version = app_version

        # 不再缓存客户端，每次请求通过 _make_client() 创建新实例

    def _get_auth_header(self) -> str:
        """生成认证头"""
        return (
            f'MediaBrowser Client="{self.app_name}", '
            f'Device="{self.device_name}", '
            f'DeviceId="{self.device_id}", '
            f'Version="{self.app_version}", '
            f'Token="{self.api_key}"'
        )

    def _make_client(self) -> httpx.AsyncClient:
        """创建新的 HTTP 客户端

        注意：Flask + asgiref 每次请求使用不同的 event loop，
        因此不能复用 AsyncClient，必须每次创建新实例。
        """
        transport = httpx.AsyncHTTPTransport(
            local_address="0.0.0.0",
        )
        return httpx.AsyncClient(
            base_url=self.base_url,
            timeout=self.timeout,
            proxy=self.proxy,
            transport=transport,
            headers={
                "X-Emby-Token": self.api_key,
                "X-Emby-Authorization": self._get_auth_header(),
                "Content-Type": "application/json",
                "Accept": "application/json",
            },
        )

    async def _get_client(self) -> httpx.AsyncClient:
        """获取 HTTP 客户端（兼容旧调用）"""
        return self._make_client()

    async def close(self) -> None:
        """关闭客户端连接（客户端已改为每次请求独立创建，此方法保留以兼容接口）"""
        pass

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()

    async def _request(self, method: str, endpoint: str, **kwargs) -> Optional[Any]:
        """发送 HTTP 请求"""
        for attempt in range(Config.MAX_RETRY):
            try:
                async with self._make_client() as client:
                    response = await client.request(method, endpoint, **kwargs)

                    if response.status_code == 401:
                        # API Key 无效，尝试使用管理员账号密码备用认证
                        if not self._auth_fallback_attempted and self._admin_username:
                            logger.warning("API Key 认证失败，尝试使用管理员账号密码备用认证...")
                            token = await self._authenticate_admin()
                            if token:
                                logger.info("管理员账号密码认证成功，已切换 Token")
                                self._auth_fallback_attempted = True
                                # 使用新 token 重试当前请求
                                async with self._make_client() as new_client:
                                    response = await new_client.request(method, endpoint, **kwargs)
                                    if response.status_code == 401:
                                        raise EmbyAuthError("备用认证获取的 Token 也无效")
                            else:
                                raise EmbyAuthError("API Key 无效且管理员账号密码认证也失败")
                        else:
                            raise EmbyAuthError("API Key 无效或已过期")
                    elif response.status_code == 404:
                        raise EmbyNotFoundError(f"资源未找到: {endpoint}")
                    elif response.status_code >= 400:
                        raise EmbyError(f"请求失败: {response.status_code} - {response.text}")

                    if response.content:
                        try:
                            return response.json()
                        except Exception:
                            return response.text
                    return None

            except httpx.ConnectError as e:
                logger.warning(f"连接失败 (尝试 {attempt + 1}/{Config.MAX_RETRY}): {e}")
                if attempt == Config.MAX_RETRY - 1:
                    raise EmbyConnectionError(f"无法连接到 Emby 服务器: {self.base_url}")
            except httpx.TimeoutException:
                logger.warning(f"请求超时 (尝试 {attempt + 1}/{Config.MAX_RETRY})")
                if attempt == Config.MAX_RETRY - 1:
                    raise EmbyConnectionError("请求超时")

    async def _authenticate_admin(self) -> Optional[str]:
        """
        使用管理员账号密码进行认证，获取 Access Token

        :return: 成功返回 Token，失败返回 None
        """
        if not self._admin_username:
            return None

        try:
            import hashlib

            device_id = hashlib.md5(f"twilight-admin-{self._admin_username}".encode()).hexdigest()

            auth_header = (
                f'MediaBrowser Client="{self.app_name}", '
                f'Device="{self.device_name}", '
                f'DeviceId="{device_id}", '
                f'Version="{self.app_version}"'
            )

            async with httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                proxy=self.proxy,
                transport=httpx.AsyncHTTPTransport(local_address="0.0.0.0"),
                headers={
                    "Content-Type": "application/json",
                    "Accept": "application/json",
                    "X-Emby-Authorization": auth_header,
                },
            ) as client:
                response = await client.post(
                    "/Users/authenticatebyname",
                    json={
                        "Username": self._admin_username,
                        "Pw": self._admin_password,
                    },
                )

                if response.status_code == 200:
                    data = response.json()
                    access_token = data.get("AccessToken")
                    if access_token:
                        # 更新 API Key 为新获取的 Token，下次 _make_client() 会使用新值
                        self.api_key = access_token
                        logger.info(f"管理员备用认证成功，用户: {self._admin_username}")
                        return access_token

                logger.warning(f"管理员备用认证失败: {response.status_code}")
                return None

        except Exception as e:
            logger.error(f"管理员备用认证出错: {e}")
            return None

    # ==================== 系统 API ====================

    async def get_server_info(self) -> Dict[str, Any]:
        """获取服务器信息"""
        return await self._request("GET", "/System/Info")

    async def get_public_info(self) -> Dict[str, Any]:
        """获取服务器公开信息（无需认证）"""
        return await self._request("GET", "/System/Info/Public")

    async def ping(self) -> bool:
        """测试服务器连接"""
        try:
            await self._request("GET", "/System/Ping")
            return True
        except EmbyError:
            return False

    async def restart_server(self) -> bool:
        """重启服务器"""
        try:
            await self._request("POST", "/System/Restart")
            return True
        except EmbyError:
            return False

    async def shutdown_server(self) -> bool:
        """关闭服务器"""
        try:
            await self._request("POST", "/System/Shutdown")
            return True
        except EmbyError:
            return False

    # ==================== 用户管理 API ====================

    async def get_users(self, is_hidden: Optional[bool] = None, is_disabled: Optional[bool] = None) -> List[EmbyUser]:
        """获取用户列表"""
        cache_key = (is_hidden, is_disabled)
        now = time.time()
        cached = self._users_cache.get(cache_key)
        if cached:
            timestamp, users = cached
            if now - timestamp < self._users_cache_ttl:
                return users

        params = {}
        if is_hidden is not None:
            params["IsHidden"] = str(is_hidden).lower()
        if is_disabled is not None:
            params["IsDisabled"] = str(is_disabled).lower()

        data = await self._request("GET", "/Users", params=params or None)
        users = [EmbyUser.from_dict(u) for u in (data or [])]
        self._users_cache[cache_key] = (now, users)
        return users

    async def get_user(self, user_id: str) -> Optional[EmbyUser]:
        """根据 ID 获取用户"""
        try:
            data = await self._request("GET", f"/Users/{user_id}")
            return EmbyUser.from_dict(data) if data else None
        except EmbyNotFoundError:
            return None

    async def get_user_by_name(self, username: str) -> Optional[EmbyUser]:
        """根据用户名获取用户"""
        users = await self.get_users()
        for user in users:
            if user.name.lower() == username.lower():
                return user
        return None

    async def create_user(self, username: str, password: str = "") -> Optional[EmbyUser]:
        """创建新用户。

        默认禁用媒体下载权限（EnableContentDownloading=False），避免新建账户
        自动拥有「下载到本地」能力。
        """
        data = await self._request("POST", "/Users/New", json={"Name": username})

        if data:
            user_id = data.get("Id")
            if user_id:
                # 关闭下载权限：Emby/Jellyfin 默认 True，注册场景一律收紧。
                try:
                    await self.update_user_policy(user_id, {"EnableContentDownloading": False})
                except EmbyError as exc:
                    logger.warning(f"新建 Emby 用户后禁用下载权限失败 ({username}, id={user_id}): {exc}")

            if password:
                await self.set_user_password(user_id, password)

            self._users_cache.clear()
            return EmbyUser.from_dict(data)
        return None

    async def delete_user(self, user_id: str) -> bool:
        """删除用户"""
        try:
            await self._request("DELETE", f"/Users/{user_id}")
            self._users_cache.clear()
            return True
        except EmbyError as e:
            logger.error(f"删除用户失败: {e}")
            return False

    async def update_user(self, user_id: str, user_data: Dict[str, Any]) -> bool:
        """更新用户信息"""
        try:
            await self._request("POST", f"/Users/{user_id}", json=user_data)
            self._users_cache.clear()
            return True
        except EmbyError as e:
            logger.error(f"更新用户失败: {e}")
            return False

    async def set_user_password(self, user_id: str, new_password: str, current_password: str = "") -> bool:
        """设置用户密码"""
        try:
            await self._request(
                "POST", f"/Users/{user_id}/Password", json={"CurrentPw": current_password, "NewPw": new_password}
            )
            return True
        except EmbyError as e:
            logger.error(f"设置密码失败: {e}")
            return False

    async def reset_user_password(self, user_id: str) -> bool:
        """重置用户密码"""
        try:
            await self._request("POST", f"/Users/{user_id}/Password", json={"ResetPassword": True})
            return True
        except EmbyError as e:
            logger.error(f"重置密码失败: {e}")
            return False

    async def authenticate_by_name(self, username: str, password: str) -> Optional[EmbyUser]:
        """
        通过用户名和密码验证用户

        :param username: Emby 用户名
        :param password: Emby 密码
        :return: 如果验证成功返回用户信息，否则返回 None
        """
        try:
            # 使用 Emby 的 AuthenticateByName API
            # 注意：这个端点需要特定的认证头格式
            import httpx
            import hashlib

            # 生成设备 ID（用于认证）
            device_id = hashlib.md5(f"twilight-bind-{username}".encode()).hexdigest()

            # 构建认证头（不需要 API Key）
            auth_header = (
                f'MediaBrowser Client="Twilight", '
                f'Device="Twilight Bind", '
                f'DeviceId="{device_id}", '
                f'Version="1.0.0"'
            )

            # 创建临时客户端（不使用 API Key）
            async with httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                proxy=self.proxy,
                transport=httpx.AsyncHTTPTransport(local_address="0.0.0.0"),
                headers={
                    "Content-Type": "application/json",
                    "Accept": "application/json",
                    "X-Emby-Authorization": auth_header,
                },
            ) as client:
                # 调用认证端点
                response = await client.post(
                    "/Users/authenticatebyname",
                    json={
                        "Username": username,
                        "Pw": password,
                    },
                )

                if response.status_code == 200:
                    data = response.json()
                    # 返回用户信息
                    user_data = data.get("User")
                    if user_data:
                        user_id = user_data.get("Id")
                        if user_id:
                            # 使用 API Key 获取完整用户信息
                            return await self.get_user(user_id)
                elif response.status_code == 401:
                    # 认证失败（用户名或密码错误）
                    logger.warning(f"Emby 认证失败: 用户名或密码错误")
                    return None
                else:
                    logger.warning(f"认证请求失败: {response.status_code} - {response.text}")
                    return None

        except httpx.HTTPStatusError as e:
            if e.response.status_code == 401:
                logger.warning(f"Emby 认证失败: 用户名或密码错误")
                return None
            logger.error(f"验证用户凭据失败: {e}")
            return None
        except Exception as e:
            logger.error(f"验证用户凭据失败: {e}")
            return None

    async def update_user_policy(self, user_id: str, policy: Dict[str, Any]) -> bool:
        """更新用户策略"""
        try:
            user = await self.get_user(user_id)
            if not user:
                return False

            current_policy = user.policy.copy()
            current_policy.update(policy)

            await self._request("POST", f"/Users/{user_id}/Policy", json=current_policy)
            self._users_cache.clear()
            return True
        except EmbyError as e:
            logger.error(f"更新用户策略失败: {e}")
            return False

    async def set_user_enabled(self, user_id: str, enabled: bool) -> bool:
        """启用或禁用用户"""
        return await self.update_user_policy(user_id, {"IsDisabled": not enabled})

    async def set_user_admin(self, user_id: str, is_admin: bool) -> bool:
        """设置用户管理员权限"""
        return await self.update_user_policy(user_id, {"IsAdministrator": is_admin})

    async def set_user_hidden(self, user_id: str, is_hidden: bool) -> bool:
        """设置用户是否隐藏"""
        return await self.update_user_policy(user_id, {"IsHidden": is_hidden})

    # ==================== 媒体库 API ====================

    async def get_libraries(self) -> List[EmbyLibrary]:
        """获取所有媒体库"""
        now = time.monotonic()
        if self._libraries_cache and (now - self._libraries_cache[0]) < self._libraries_cache_ttl:
            return self._libraries_cache[1]

        data = await self._request("GET", "/Library/VirtualFolders")
        libraries = [EmbyLibrary.from_dict(lib) for lib in (data or [])]
        self._libraries_cache = (now, libraries)
        return libraries

    async def get_media_folders(self) -> List[EmbyLibrary]:
        """获取媒体文件夹"""
        data = await self._request("GET", "/Library/MediaFolders")
        items = data.get("Items", []) if data else []
        return [EmbyLibrary.from_dict(lib) for lib in items]

    async def get_user_views(self, user_id: str) -> List[EmbyLibrary]:
        """获取用户可见的媒体库视图"""
        data = await self._request("GET", f"/Users/{user_id}/Views")
        items = data.get("Items", []) if data else []
        return [EmbyLibrary.from_dict(lib) for lib in items]

    async def refresh_library(self) -> bool:
        """刷新媒体库"""
        try:
            await self._request("POST", "/Library/Refresh")
            self._libraries_cache = None
            return True
        except EmbyError:
            return False

    # ==================== 媒体项 API ====================

    async def get_items(
        self,
        user_id: Optional[str] = None,
        parent_id: Optional[str] = None,
        item_types: Optional[List[str]] = None,
        limit: int = 100,
        start_index: int = 0,
        search_term: Optional[str] = None,
        sort_by: str = "SortName",
        sort_order: str = "Ascending",
    ) -> Dict[str, Any]:
        """获取媒体项列表"""
        params = {
            "Limit": limit,
            "StartIndex": start_index,
            "SortBy": sort_by,
            "SortOrder": sort_order,
            "Recursive": "true",
        }

        if parent_id:
            params["ParentId"] = parent_id
        if item_types:
            params["IncludeItemTypes"] = ",".join(item_types)
        if search_term:
            params["SearchTerm"] = search_term

        endpoint = f"/Users/{user_id}/Items" if user_id else "/Items"
        return await self._request("GET", endpoint, params=params)

    async def get_item(self, item_id: str, user_id: Optional[str] = None) -> Optional[EmbyItem]:
        """获取单个媒体项"""
        try:
            endpoint = f"/Users/{user_id}/Items/{item_id}" if user_id else f"/Items/{item_id}"
            data = await self._request("GET", endpoint)
            return EmbyItem.from_dict(data) if data else None
        except EmbyNotFoundError:
            return None

    async def search_items(self, search_term: str, limit: int = 20) -> List[EmbyItem]:
        """搜索媒体项"""
        data = await self._request(
            "GET", "/Items", params={"SearchTerm": search_term, "Limit": limit, "Recursive": "true"}
        )
        items = data.get("Items", []) if data else []
        return [EmbyItem.from_dict(item) for item in items]

    async def search_media(
        self, search_term: str, include_types: List[str] = None, year: Optional[int] = None, limit: int = 50
    ) -> List[EmbyItem]:
        """
        搜索媒体库

        :param search_term: 搜索关键词
        :param include_types: 媒体类型过滤 (Movie, Series, Episode, Season, etc.)
        :param year: 年份过滤
        :param limit: 返回数量
        """
        params = {
            "SearchTerm": search_term,
            "Limit": limit,
            "Recursive": "true",
            "Fields": "ProviderIds,Overview,OriginalTitle,PremiereDate,ProductionYear",
        }

        if include_types:
            params["IncludeItemTypes"] = ",".join(include_types)
        if year:
            params["Years"] = str(year)

        data = await self._request("GET", "/Items", params=params)
        items = data.get("Items", []) if data else []
        return [EmbyItem.from_dict(item) for item in items]

    async def find_by_tmdb_id(self, tmdb_id: int, media_type: str = "Movie") -> Optional[EmbyItem]:
        """
        根据 TMDB ID 查找媒体

        :param tmdb_id: TMDB ID
        :param media_type: 媒体类型 (Movie/Series)
        """
        # 通过 AnyProviderIdEquals 参数搜索
        params = {
            "AnyProviderIdEquals": f"Tmdb.{tmdb_id}",
            "IncludeItemTypes": media_type,
            "Recursive": "true",
            "Fields": "ProviderIds,Overview,OriginalTitle,PremiereDate,ProductionYear",
            "Limit": 1,
        }

        data = await self._request("GET", "/Items", params=params)
        items = data.get("Items", []) if data else []

        if items:
            return EmbyItem.from_dict(items[0])

        # 备用方案：搜索所有项目检查 ProviderIds
        # 这是因为某些 Emby 版本可能不支持 AnyProviderIdEquals
        return None

    async def find_by_imdb_id(self, imdb_id: str) -> Optional[EmbyItem]:
        """根据 IMDB ID 查找媒体"""
        params = {
            "AnyProviderIdEquals": f"Imdb.{imdb_id}",
            "Recursive": "true",
            "Fields": "ProviderIds,Overview,OriginalTitle,PremiereDate,ProductionYear",
            "Limit": 1,
        }

        data = await self._request("GET", "/Items", params=params)
        items = data.get("Items", []) if data else []

        if items:
            return EmbyItem.from_dict(items[0])
        return None

    async def get_series_seasons(self, series_id: str) -> List[EmbyItem]:
        """
        获取剧集的所有季度

        :param series_id: 剧集 ID
        :return: 季度列表
        """
        params = {
            "ParentId": series_id,
            "IncludeItemTypes": "Season",
            "Recursive": "false",
            "Fields": "ProviderIds,Overview,PremiereDate,ProductionYear",
            "SortBy": "SortName",
            "SortOrder": "Ascending",
        }

        data = await self._request("GET", "/Items", params=params)
        items = data.get("Items", []) if data else []
        return [EmbyItem.from_dict(item) for item in items]

    async def get_season_episodes(self, season_id: str) -> List[EmbyItem]:
        """
        获取某一季的所有剧集

        :param season_id: 季度 ID
        :return: 剧集列表
        """
        params = {
            "ParentId": season_id,
            "IncludeItemTypes": "Episode",
            "Recursive": "false",
            "Fields": "Overview,PremiereDate",
            "SortBy": "IndexNumber",
            "SortOrder": "Ascending",
        }

        data = await self._request("GET", "/Items", params=params)
        items = data.get("Items", []) if data else []
        return [EmbyItem.from_dict(item) for item in items]

    async def check_media_exists(
        self,
        title: str,
        year: Optional[int] = None,
        original_title: str = None,
        tmdb_id: int = None,
        media_type: str = None,
    ) -> Tuple[bool, Optional[EmbyItem]]:
        """
        检查媒体是否存在于库中

        :param title: 媒体标题
        :param year: 年份
        :param original_title: 原标题
        :param tmdb_id: TMDB ID
        :param media_type: 媒体类型 (movie/tv)
        :return: (是否存在, 媒体项)
        """
        # 1. 如果有 TMDB ID，优先通过 ID 查找
        if tmdb_id:
            emby_type = "Movie" if media_type == "movie" else "Series"
            item = await self.find_by_tmdb_id(tmdb_id, emby_type)
            if item:
                return True, item

        # 2. 通过标题搜索
        search_titles = [title]
        if original_title and original_title != title:
            search_titles.append(original_title)

        include_types = None
        if media_type:
            include_types = ["Movie"] if media_type == "movie" else ["Series"]

        for search_title in search_titles:
            items = await self.search_media(search_title, include_types=include_types, year=year, limit=10)

            for item in items:
                # 精确匹配标题
                item_names = [item.name.lower(), item.original_title.lower(), item.sort_name.lower()]
                if search_title.lower() in item_names:
                    # 如果有年份，也检查年份
                    if year and item.year:
                        if abs(item.year - year) <= 1:  # 允许1年误差
                            return True, item
                    else:
                        return True, item

        return False, None

    async def check_series_with_seasons(
        self,
        title: str,
        season: Optional[int] = None,
        year: Optional[int] = None,
        original_title: str = None,
        tmdb_id: int = None,
    ) -> Tuple[bool, Optional[EmbyItem], List[int]]:
        """
        检查剧集是否存在，并返回已有的季度列表

        :param title: 剧集标题
        :param season: 要检查的季度（None=检查所有）
        :param year: 年份
        :param original_title: 原标题
        :param tmdb_id: TMDB ID
        :return: (剧集是否存在, 剧集项, 已有季度列表)
        """
        # 先检查剧集是否存在
        exists, series = await self.check_media_exists(
            title=title, year=year, original_title=original_title, tmdb_id=tmdb_id, media_type="tv"
        )

        if not exists or not series:
            return False, None, []

        # 获取已有的季度
        seasons = await self.get_series_seasons(series.id)
        season_numbers = []

        for s in seasons:
            # 季度编号通常在 IndexNumber 或者名称中
            if s.index_number is not None:
                season_numbers.append(s.index_number)
            elif s.name:
                # 尝试从名称解析，如 "Season 1" 或 "第1季"
                import re

                match = re.search(r"(?:Season|第)\s*(\d+)", s.name, re.IGNORECASE)
                if match:
                    season_numbers.append(int(match.group(1)))

        season_numbers.sort()

        # 如果指定了季度，检查该季度是否存在
        if season is not None:
            if season in season_numbers:
                return True, series, season_numbers
            else:
                # 剧集存在但指定的季度不存在
                return False, series, season_numbers

        return True, series, season_numbers

    # ==================== 会话 API ====================

    async def get_sessions(self) -> List[EmbySession]:
        """获取所有活动会话"""
        data = await self._request("GET", "/Sessions")
        return [EmbySession.from_dict(s) for s in (data or [])]

    async def get_user_sessions(self, user_id: str) -> List[EmbySession]:
        """获取指定用户的会话"""
        sessions = await self.get_sessions()
        return [s for s in sessions if s.user_id == user_id]

    async def kill_session(self, session_id: str) -> bool:
        """终止会话"""
        try:
            await self._request("POST", f"/Sessions/{session_id}/Logout")
            return True
        except EmbyError:
            return False

    async def send_message(self, session_id: str, header: str, text: str, timeout_ms: int = 5000) -> bool:
        """向会话发送消息"""
        try:
            await self._request(
                "POST",
                f"/Sessions/{session_id}/Message",
                json={"Header": header, "Text": text, "TimeoutMs": timeout_ms},
            )
            return True
        except EmbyError:
            return False

    # ==================== 设备 API ====================

    async def get_devices(self) -> List[Dict[str, Any]]:
        """获取所有设备"""
        data = await self._request("GET", "/Devices")
        return data.get("Items", []) if data else []

    async def delete_device(self, device_id: str) -> bool:
        """删除设备"""
        try:
            await self._request("DELETE", "/Devices", params={"Id": device_id})
            return True
        except EmbyError:
            return False

    # ==================== 活动日志 API ====================

    async def get_activity_log(
        self, start_index: int = 0, limit: int = 100, min_date: Optional[str] = None
    ) -> Dict[str, Any]:
        """获取活动日志"""
        params = {"StartIndex": start_index, "Limit": limit}
        if min_date:
            params["MinDate"] = min_date
        return await self._request("GET", "/System/ActivityLog/Entries", params=params)

    # ==================== 媒体库访问策略（参考 Sakura_embyboss） ====================
    #
    # 关键设计原则（与 https://github.com/berry8838/Sakura_embyboss 一致）：
    # - 所有写入策略都是**单次** POST `/Users/{Id}/Policy`，完整替换式
    # - `EnabledFolders` 用媒体库 **GUID** 列表，`BlockedMediaFolders` 用媒体库 **名称** 列表，两路同时写入
    # - 增量操作（show / hide）：先读当前策略，仅修改与目标库相关的字段，其他库的状态保持原样
    # - 全量操作（enable_all / disable_all）：直接写入完整状态
    # - 不再使用"先开后关再排除"的多步流程，避免中间步骤失败导致用户被锁

    async def resolve_libraries_by_names(self, folder_names: List[str]) -> Tuple[List[EmbyLibrary], List[str]]:
        """按媒体库名称查找库对象（大小写不敏感、去重保序）。"""
        if not folder_names:
            return [], []

        wanted: list[tuple[str, str]] = []
        seen: set[str] = set()
        for name in folder_names:
            raw = (name or "").strip()
            key = raw.lower()
            if not raw or key in seen:
                continue
            wanted.append((raw, key))
            seen.add(key)

        if not wanted:
            return [], []

        libraries = await self.get_libraries()
        by_name = {(lib.name or "").strip().lower(): lib for lib in libraries if (lib.name or "").strip()}

        resolved: List[EmbyLibrary] = []
        missing: List[str] = []
        for raw, key in wanted:
            lib = by_name.get(key)
            if not lib or not lib.id:
                missing.append(raw)
                continue
            if lib.id not in {item.id for item in resolved}:
                resolved.append(lib)
        return resolved, missing

    async def get_folder_ids_by_names(self, folder_names: List[str]) -> List[str]:
        """根据媒体库名称查找 GUID（大小写不敏感、去重保序）。"""
        libraries, _missing = await self.resolve_libraries_by_names(folder_names)
        return [lib.id for lib in libraries if lib.id]

    async def get_folder_names_by_ids(self, folder_ids: List[str]) -> Tuple[List[str], List[str]]:
        """根据媒体库 GUID 查找名称（去重保序）。"""
        if not folder_ids:
            return [], []
        wanted: list[str] = []
        seen: set[str] = set()
        for folder_id in folder_ids:
            value = (folder_id or "").strip()
            if not value or value in seen:
                continue
            wanted.append(value)
            seen.add(value)

        libraries = await self.get_libraries()
        by_id = {(lib.id or "").strip(): lib for lib in libraries if (lib.id or "").strip()}
        names: List[str] = []
        missing: List[str] = []
        for folder_id in wanted:
            lib = by_id.get(folder_id)
            if not lib or not lib.name:
                missing.append(folder_id)
                continue
            names.append(lib.name)
        return names, missing

    async def get_current_folder_state(self, user_id: str) -> Optional[tuple[List[str], bool, List[str]]]:
        """读取用户当前媒体库策略：`(EnabledFolders, EnableAllFolders, BlockedMediaFolders)`。"""
        user = await self.get_user(user_id)
        if not user:
            return None
        policy = user.policy or {}
        enable_all = bool(policy.get("EnableAllFolders", True))
        blocked = list(policy.get("BlockedMediaFolders", []) or [])

        if enable_all:
            libraries = await self.get_libraries()
            enabled = [lib.id for lib in libraries if lib.id]
        else:
            enabled = list(policy.get("EnabledFolders", []) or [])
        return enabled, enable_all, blocked

    async def update_user_enabled_folder(
        self,
        user_id: str,
        enabled_folder_ids: Optional[List[str]] = None,
        blocked_media_folders: Optional[List[str]] = None,
        enable_all_folders: bool = False,
    ) -> bool:
        """单次写入媒体库策略（参考 Sakura `update_user_enabled_folder`）。

        - 读取当前完整 policy（保留 IsAdministrator、SimultaneousStreamLimit 等无关字段）
        - 仅覆盖 EnableAllFolders / EnabledFolders / BlockedMediaFolders 三个字段
        - 单次 POST 完成
        """
        user = await self.get_user(user_id)
        if not user:
            logger.error("update_user_enabled_folder: 用户不存在 user=%s", user_id)
            return False

        policy = user.policy.copy() if user.policy else {}
        policy["EnableAllFolders"] = bool(enable_all_folders)
        if enabled_folder_ids is not None:
            policy["EnabledFolders"] = list(dict.fromkeys(enabled_folder_ids))
        if blocked_media_folders is not None:
            policy["BlockedMediaFolders"] = list(dict.fromkeys(blocked_media_folders))

        logger.info(
            "update_user_enabled_folder POST: user=%s, EnableAllFolders=%s, "
            "EnabledFolders=%s, BlockedMediaFolders=%s",
            user_id,
            policy.get("EnableAllFolders"),
            policy.get("EnabledFolders"),
            policy.get("BlockedMediaFolders"),
        )

        try:
            await self._request("POST", f"/Users/{user_id}/Policy", json=policy)
            self._users_cache.clear()
            return True
        except EmbyError as e:
            logger.error("update_user_enabled_folder 写入失败: user=%s err=%s", user_id, e)
            return False

    async def hide_folders_by_names(self, user_id: str, folder_names: List[str]) -> bool:
        """按名称隐藏指定媒体库（Sakura `hide_folders_by_names` 等价实现）。

        逻辑：读当前 EnabledFolders → 把目标库 GUID 移出 → 把目标库**名称**加入 BlockedMediaFolders。
        其他库的状态完全保持原样。
        """
        names = [(n or "").strip() for n in (folder_names or []) if (n or "").strip()]
        if not names:
            return True

        hide_libraries, missing = await self.resolve_libraries_by_names(names)
        hide_ids = [lib.id for lib in hide_libraries if lib.id]
        hide_names = [(lib.name or "").strip() for lib in hide_libraries if (lib.name or "").strip()]
        if missing:
            logger.warning("hide_folders_by_names: 未找到部分媒体库 %s", missing)
        if not hide_ids:
            logger.warning("hide_folders_by_names: 未找到要隐藏的媒体库 %s", names)
            return True

        state = await self.get_current_folder_state(user_id)
        if state is None:
            return False
        current_enabled, _, current_blocked = state

        new_enabled = [fid for fid in current_enabled if fid not in hide_ids]
        new_blocked = list(current_blocked)
        blocked_keys = {(n or "").strip().lower() for n in new_blocked}
        for name in hide_names:
            key = name.lower()
            if key not in blocked_keys:
                new_blocked.append(name)
                blocked_keys.add(key)

        return await self.update_user_enabled_folder(
            user_id=user_id,
            enabled_folder_ids=new_enabled,
            blocked_media_folders=new_blocked,
            enable_all_folders=False,
        )

    async def show_folders_by_names(self, user_id: str, folder_names: List[str]) -> bool:
        """按名称显示指定媒体库（Sakura `show_folders_by_names` 等价实现）。

        逻辑：
        - 若当前 EnableAllFolders=True，仅清除目标库名的 BlockedMediaFolders 即可
        - 否则读当前 EnabledFolders → 把目标库 GUID 加入 → 把目标库名从 BlockedMediaFolders 移除
        其他库的状态完全保持原样。
        """
        names = [(n or "").strip() for n in (folder_names or []) if (n or "").strip()]
        if not names:
            return True

        show_libraries, missing = await self.resolve_libraries_by_names(names)
        show_ids = [lib.id for lib in show_libraries if lib.id]
        show_names = [(lib.name or "").strip() for lib in show_libraries if (lib.name or "").strip()]
        if missing:
            logger.warning("show_folders_by_names: 未找到部分媒体库 %s", missing)
        if not show_ids:
            logger.warning("show_folders_by_names: 未找到要显示的媒体库 %s", names)
            return True

        state = await self.get_current_folder_state(user_id)
        if state is None:
            return False
        current_enabled, enable_all, current_blocked = state

        if enable_all:
            # 用户已可见全部库，只需要从 BlockedMediaFolders 移除目标项
            remove_names = {n.lower() for n in show_names}
            new_blocked = [n for n in current_blocked if (n or "").strip().lower() not in remove_names]
            return await self.update_user_enabled_folder(
                user_id=user_id,
                blocked_media_folders=new_blocked,
                enable_all_folders=True,
            )

        new_enabled = list(dict.fromkeys(list(current_enabled) + list(show_ids)))
        remove_names = {n.lower() for n in show_names}
        new_blocked = [n for n in current_blocked if (n or "").strip().lower() not in remove_names]

        return await self.update_user_enabled_folder(
            user_id=user_id,
            enabled_folder_ids=new_enabled,
            blocked_media_folders=new_blocked,
            enable_all_folders=False,
        )

    async def enable_all_folders_for_user(self, user_id: str) -> bool:
        """让用户能看到所有媒体库（清空 BlockedMediaFolders）。"""
        libraries = await self.get_libraries()
        all_ids = [lib.id for lib in libraries if lib.id]
        return await self.update_user_enabled_folder(
            user_id=user_id,
            enabled_folder_ids=all_ids,
            blocked_media_folders=[],
            enable_all_folders=True,
        )

    async def disable_all_folders_for_user(self, user_id: str) -> bool:
        """禁止用户看到所有媒体库。"""
        libraries = await self.get_libraries()
        all_names = [(lib.name or "").strip() for lib in libraries if lib.name and lib.name.strip()]
        return await self.update_user_enabled_folder(
            user_id=user_id,
            enabled_folder_ids=[],
            blocked_media_folders=all_names,
            enable_all_folders=False,
        )

    async def set_user_libraries(
        self,
        user_id: str,
        allowed_library_ids: List[str],
        enable_all: bool = False,
    ) -> bool:
        """设置用户可访问的媒体库（管理员严格白名单场景）。

        - enable_all=True：等价于 enable_all_folders_for_user
        - enable_all=False：仅放行 allowed_library_ids，其余库按名称加入 BlockedMediaFolders
        """
        libraries = await self.get_libraries()
        if not libraries:
            logger.warning("set_user_libraries: 未获取到任何媒体库 user=%s", user_id)
            return False

        if enable_all:
            return await self.enable_all_folders_for_user(user_id)

        allowed_set = {(i or "").strip() for i in allowed_library_ids if (i or "").strip()}
        target_enabled: List[str] = []
        target_blocked: List[str] = []
        for lib in libraries:
            lib_id = (lib.id or "").strip()
            lib_name = (lib.name or "").strip()
            if not lib_id:
                continue
            if lib_id in allowed_set:
                target_enabled.append(lib_id)
            elif lib_name:
                target_blocked.append(lib_name)

        return await self.update_user_enabled_folder(
            user_id=user_id,
            enabled_folder_ids=target_enabled,
            blocked_media_folders=target_blocked,
            enable_all_folders=False,
        )


# ==================== 全局实例 ====================

_emby_client: Optional[EmbyClient] = None


def get_emby_client() -> EmbyClient:
    """获取全局 Emby 客户端实例"""
    global _emby_client
    if _emby_client is None:
        _emby_client = EmbyClient()
    return _emby_client


async def close_emby_client() -> None:
    """关闭全局 Emby 客户端"""
    global _emby_client
    if _emby_client:
        await _emby_client.close()
        _emby_client = None
