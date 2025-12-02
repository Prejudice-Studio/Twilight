"""
Emby/Jellyfin API 客户端

基于官方 API 实现的异步客户端
参考:
- https://github.com/MediaBrowser/Emby.ApiClients
- https://github.com/jellyfin/jellyfin-apiclient-python
"""
import logging
from typing import Optional, Dict, Any, List
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
    server_id: str = ''
    policy: Dict[str, Any] = field(default_factory=dict)
    configuration: Dict[str, Any] = field(default_factory=dict)
    has_password: bool = False
    has_configured_password: bool = False
    last_login_date: Optional[str] = None
    last_activity_date: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'EmbyUser':
        return cls(
            id=data.get('Id', ''),
            name=data.get('Name', ''),
            server_id=data.get('ServerId', ''),
            policy=data.get('Policy', {}),
            configuration=data.get('Configuration', {}),
            has_password=data.get('HasPassword', False),
            has_configured_password=data.get('HasConfiguredPassword', False),
            last_login_date=data.get('LastLoginDate'),
            last_activity_date=data.get('LastActivityDate'),
        )


@dataclass
class EmbyLibrary:
    """Emby 媒体库信息"""
    id: str
    name: str
    collection_type: str
    item_id: str = ''
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'EmbyLibrary':
        return cls(
            id=data.get('Id', data.get('ItemId', '')),
            name=data.get('Name', ''),
            collection_type=data.get('CollectionType', ''),
            item_id=data.get('ItemId', ''),
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
    def from_dict(cls, data: Dict[str, Any]) -> 'EmbySession':
        return cls(
            id=data.get('Id', ''),
            user_id=data.get('UserId', ''),
            user_name=data.get('UserName', ''),
            client=data.get('Client', ''),
            device_name=data.get('DeviceName', ''),
            device_id=data.get('DeviceId', ''),
            application_version=data.get('ApplicationVersion', ''),
            is_active=data.get('IsActive', False),
            now_playing_item=data.get('NowPlayingItem'),
        )


@dataclass
class EmbyItem:
    """Emby 媒体项"""
    id: str
    name: str
    type: str
    overview: str = ''
    year: Optional[int] = None
    parent_id: str = ''
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'EmbyItem':
        return cls(
            id=data.get('Id', ''),
            name=data.get('Name', ''),
            type=data.get('Type', ''),
            overview=data.get('Overview', ''),
            year=data.get('ProductionYear'),
            parent_id=data.get('ParentId', ''),
        )


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
        device_name: str = 'Twilight',
        device_id: str = 'twilight-client',
        app_name: str = 'Twilight',
        app_version: str = '1.0.0'
    ):
        self.base_url = (base_url or EmbyConfig.EMBY_URL).rstrip('/')
        self.api_key = api_key or EmbyConfig.EMBY_TOKEN
        self.proxy = proxy or Config.PROXY
        self.timeout = timeout
        
        # 设备信息
        self.device_name = device_name
        self.device_id = device_id
        self.app_name = app_name
        self.app_version = app_version
        
        self._client: Optional[httpx.AsyncClient] = None

    def _get_auth_header(self) -> str:
        """生成认证头"""
        return (
            f'MediaBrowser Client="{self.app_name}", '
            f'Device="{self.device_name}", '
            f'DeviceId="{self.device_id}", '
            f'Version="{self.app_version}", '
            f'Token="{self.api_key}"'
        )

    async def _get_client(self) -> httpx.AsyncClient:
        """获取或创建 HTTP 客户端"""
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                proxy=self.proxy,
                headers={
                    'X-Emby-Token': self.api_key,
                    'X-Emby-Authorization': self._get_auth_header(),
                    'Content-Type': 'application/json',
                    'Accept': 'application/json',
                }
            )
        return self._client

    async def close(self) -> None:
        """关闭客户端连接"""
        if self._client and not self._client.is_closed:
            await self._client.aclose()
            self._client = None

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()

    async def _request(
        self,
        method: str,
        endpoint: str,
        **kwargs
    ) -> Optional[Any]:
        """发送 HTTP 请求"""
        client = await self._get_client()
        
        for attempt in range(Config.MAX_RETRY):
            try:
                response = await client.request(method, endpoint, **kwargs)
                
                if response.status_code == 401:
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

    # ==================== 系统 API ====================
    
    async def get_server_info(self) -> Dict[str, Any]:
        """获取服务器信息"""
        return await self._request('GET', '/System/Info')

    async def get_public_info(self) -> Dict[str, Any]:
        """获取服务器公开信息（无需认证）"""
        return await self._request('GET', '/System/Info/Public')

    async def ping(self) -> bool:
        """测试服务器连接"""
        try:
            await self._request('GET', '/System/Ping')
            return True
        except EmbyError:
            return False

    async def restart_server(self) -> bool:
        """重启服务器"""
        try:
            await self._request('POST', '/System/Restart')
            return True
        except EmbyError:
            return False

    async def shutdown_server(self) -> bool:
        """关闭服务器"""
        try:
            await self._request('POST', '/System/Shutdown')
            return True
        except EmbyError:
            return False

    # ==================== 用户管理 API ====================
    
    async def get_users(self, is_hidden: Optional[bool] = None, is_disabled: Optional[bool] = None) -> List[EmbyUser]:
        """获取用户列表"""
        params = {}
        if is_hidden is not None:
            params['IsHidden'] = str(is_hidden).lower()
        if is_disabled is not None:
            params['IsDisabled'] = str(is_disabled).lower()
        
        data = await self._request('GET', '/Users', params=params or None)
        return [EmbyUser.from_dict(u) for u in (data or [])]

    async def get_user(self, user_id: str) -> Optional[EmbyUser]:
        """根据 ID 获取用户"""
        try:
            data = await self._request('GET', f'/Users/{user_id}')
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

    async def create_user(self, username: str, password: str = '') -> Optional[EmbyUser]:
        """创建新用户"""
        data = await self._request('POST', '/Users/New', json={'Name': username})
        
        if data and password:
            user_id = data.get('Id')
            await self.set_user_password(user_id, password)
        
        return EmbyUser.from_dict(data) if data else None

    async def delete_user(self, user_id: str) -> bool:
        """删除用户"""
        try:
            await self._request('DELETE', f'/Users/{user_id}')
            return True
        except EmbyError as e:
            logger.error(f"删除用户失败: {e}")
            return False

    async def update_user(self, user_id: str, user_data: Dict[str, Any]) -> bool:
        """更新用户信息"""
        try:
            await self._request('POST', f'/Users/{user_id}', json=user_data)
            return True
        except EmbyError as e:
            logger.error(f"更新用户失败: {e}")
            return False

    async def set_user_password(self, user_id: str, new_password: str, current_password: str = '') -> bool:
        """设置用户密码"""
        try:
            await self._request(
                'POST',
                f'/Users/{user_id}/Password',
                json={'CurrentPw': current_password, 'NewPw': new_password}
            )
            return True
        except EmbyError as e:
            logger.error(f"设置密码失败: {e}")
            return False

    async def reset_user_password(self, user_id: str) -> bool:
        """重置用户密码"""
        try:
            await self._request('POST', f'/Users/{user_id}/Password', json={'ResetPassword': True})
            return True
        except EmbyError as e:
            logger.error(f"重置密码失败: {e}")
            return False

    async def update_user_policy(self, user_id: str, policy: Dict[str, Any]) -> bool:
        """更新用户策略"""
        try:
            user = await self.get_user(user_id)
            if not user:
                return False
            
            current_policy = user.policy.copy()
            current_policy.update(policy)
            
            await self._request('POST', f'/Users/{user_id}/Policy', json=current_policy)
            return True
        except EmbyError as e:
            logger.error(f"更新用户策略失败: {e}")
            return False

    async def set_user_enabled(self, user_id: str, enabled: bool) -> bool:
        """启用或禁用用户"""
        return await self.update_user_policy(user_id, {'IsDisabled': not enabled})

    async def set_user_admin(self, user_id: str, is_admin: bool) -> bool:
        """设置用户管理员权限"""
        return await self.update_user_policy(user_id, {'IsAdministrator': is_admin})

    async def set_user_hidden(self, user_id: str, is_hidden: bool) -> bool:
        """设置用户是否隐藏"""
        return await self.update_user_policy(user_id, {'IsHidden': is_hidden})

    async def set_user_libraries(self, user_id: str, library_ids: List[str], enable_all: bool = False) -> bool:
        """设置用户可访问的媒体库"""
        return await self.update_user_policy(user_id, {
            'EnableAllFolders': enable_all,
            'EnabledFolders': library_ids if not enable_all else [],
        })

    # ==================== 媒体库 API ====================
    
    async def get_libraries(self) -> List[EmbyLibrary]:
        """获取所有媒体库"""
        data = await self._request('GET', '/Library/VirtualFolders')
        return [EmbyLibrary.from_dict(lib) for lib in (data or [])]

    async def get_media_folders(self) -> List[EmbyLibrary]:
        """获取媒体文件夹"""
        data = await self._request('GET', '/Library/MediaFolders')
        items = data.get('Items', []) if data else []
        return [EmbyLibrary.from_dict(lib) for lib in items]

    async def get_user_views(self, user_id: str) -> List[EmbyLibrary]:
        """获取用户可见的媒体库视图"""
        data = await self._request('GET', f'/Users/{user_id}/Views')
        items = data.get('Items', []) if data else []
        return [EmbyLibrary.from_dict(lib) for lib in items]

    async def refresh_library(self) -> bool:
        """刷新媒体库"""
        try:
            await self._request('POST', '/Library/Refresh')
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
        sort_by: str = 'SortName',
        sort_order: str = 'Ascending',
    ) -> Dict[str, Any]:
        """获取媒体项列表"""
        params = {
            'Limit': limit,
            'StartIndex': start_index,
            'SortBy': sort_by,
            'SortOrder': sort_order,
            'Recursive': 'true',
        }
        
        if parent_id:
            params['ParentId'] = parent_id
        if item_types:
            params['IncludeItemTypes'] = ','.join(item_types)
        if search_term:
            params['SearchTerm'] = search_term
        
        endpoint = f'/Users/{user_id}/Items' if user_id else '/Items'
        return await self._request('GET', endpoint, params=params)

    async def get_item(self, item_id: str, user_id: Optional[str] = None) -> Optional[EmbyItem]:
        """获取单个媒体项"""
        try:
            endpoint = f'/Users/{user_id}/Items/{item_id}' if user_id else f'/Items/{item_id}'
            data = await self._request('GET', endpoint)
            return EmbyItem.from_dict(data) if data else None
        except EmbyNotFoundError:
            return None

    async def search_items(self, search_term: str, limit: int = 20) -> List[EmbyItem]:
        """搜索媒体项"""
        data = await self._request(
            'GET',
            '/Items',
            params={'SearchTerm': search_term, 'Limit': limit, 'Recursive': 'true'}
        )
        items = data.get('Items', []) if data else []
        return [EmbyItem.from_dict(item) for item in items]

    # ==================== 会话 API ====================
    
    async def get_sessions(self) -> List[EmbySession]:
        """获取所有活动会话"""
        data = await self._request('GET', '/Sessions')
        return [EmbySession.from_dict(s) for s in (data or [])]

    async def get_user_sessions(self, user_id: str) -> List[EmbySession]:
        """获取指定用户的会话"""
        sessions = await self.get_sessions()
        return [s for s in sessions if s.user_id == user_id]

    async def kill_session(self, session_id: str) -> bool:
        """终止会话"""
        try:
            await self._request('POST', f'/Sessions/{session_id}/Logout')
            return True
        except EmbyError:
            return False

    async def send_message(self, session_id: str, header: str, text: str, timeout_ms: int = 5000) -> bool:
        """向会话发送消息"""
        try:
            await self._request(
                'POST',
                f'/Sessions/{session_id}/Message',
                json={'Header': header, 'Text': text, 'TimeoutMs': timeout_ms}
            )
            return True
        except EmbyError:
            return False

    # ==================== 设备 API ====================
    
    async def get_devices(self) -> List[Dict[str, Any]]:
        """获取所有设备"""
        data = await self._request('GET', '/Devices')
        return data.get('Items', []) if data else []

    async def delete_device(self, device_id: str) -> bool:
        """删除设备"""
        try:
            await self._request('DELETE', '/Devices', params={'Id': device_id})
            return True
        except EmbyError:
            return False

    # ==================== 活动日志 API ====================
    
    async def get_activity_log(
        self,
        start_index: int = 0,
        limit: int = 100,
        min_date: Optional[str] = None
    ) -> Dict[str, Any]:
        """获取活动日志"""
        params = {'StartIndex': start_index, 'Limit': limit}
        if min_date:
            params['MinDate'] = min_date
        return await self._request('GET', '/System/ActivityLog/Entries', params=params)

    # ==================== NSFW 库管理 ====================
    
    async def grant_nsfw_access(self, user_id: str) -> bool:
        """授予用户 NSFW 库访问权限"""
        if not EmbyConfig.EMBY_NSFW:
            logger.warning("未配置 NSFW 库ID")
            return False
        
        user = await self.get_user(user_id)
        if not user:
            return False
        
        current_folders = list(user.policy.get('EnabledFolders', []))
        if EmbyConfig.EMBY_NSFW not in current_folders:
            current_folders.append(EmbyConfig.EMBY_NSFW)
        
        return await self.update_user_policy(user_id, {
            'EnableAllFolders': False,
            'EnabledFolders': current_folders,
        })

    async def revoke_nsfw_access(self, user_id: str) -> bool:
        """撤销用户 NSFW 库访问权限"""
        if not EmbyConfig.EMBY_NSFW:
            return False
        
        user = await self.get_user(user_id)
        if not user:
            return False
        
        current_folders = list(user.policy.get('EnabledFolders', []))
        if EmbyConfig.EMBY_NSFW in current_folders:
            current_folders.remove(EmbyConfig.EMBY_NSFW)
        
        return await self.update_user_policy(user_id, {
            'EnabledFolders': current_folders,
        })


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
