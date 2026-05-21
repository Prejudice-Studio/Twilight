"""
Bangumi API 客户端

基于 Bangumi API v0
文档: https://bangumi.github.io/api/
"""

import re
import logging
from typing import Optional, List, Dict, Any
from dataclasses import dataclass
from enum import Enum

import httpx

from src.config import Config

logger = logging.getLogger(__name__)

DEFAULT_BASE_URL = "https://api.bgm.tv/v0"


def _get_base_url() -> str:
    base_url = (Config.BANGUMI_API_URL or DEFAULT_BASE_URL).rstrip("/")
    return base_url if base_url.endswith("/v0") else f"{base_url}/v0"


class BangumiError(Exception):
    """Bangumi API 错误"""

    pass


class SubjectType(Enum):
    """条目类型"""

    BOOK = 1  # 书籍
    ANIME = 2  # 动画
    MUSIC = 3  # 音乐
    GAME = 4  # 游戏
    REAL = 6  # 三次元


class EpStatus(Enum):
    """章节状态"""

    WISH = 1  # 想看
    WATCHING = 2  # 在看
    WATCHED = 3  # 看过
    ON_HOLD = 4  # 搁置
    DROPPED = 5  # 抛弃


@dataclass
class BangumiSubject:
    """Bangumi 条目信息"""

    id: int
    type: int
    name: str
    name_cn: str
    date: str
    summary: str
    eps: int
    volumes: int
    rating: Dict[str, Any]
    images: Dict[str, str]
    tags: List[Dict[str, Any]]
    infobox: List[Dict[str, Any]]

    @property
    def title(self) -> str:
        """优先返回中文名"""
        return self.name_cn if self.name_cn else self.name

    @property
    def cover_url(self) -> Optional[str]:
        """获取封面 URL"""
        if self.images:
            return self.images.get("large") or self.images.get("common") or self.images.get("medium")
        return None

    @property
    def bgm_url(self) -> str:
        """获取 Bangumi 链接"""
        return f"https://bgm.tv/subject/{self.id}"

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "BangumiSubject":
        """从 API 响应创建对象"""
        return cls(
            id=data.get("id", 0),
            type=data.get("type", 0),
            name=data.get("name", ""),
            name_cn=data.get("name_cn", ""),
            date=data.get("date", ""),
            summary=data.get("summary", ""),
            eps=data.get("eps", 0),
            volumes=data.get("volumes", 0),
            rating=data.get("rating", {}),
            images=data.get("images", {}),
            tags=data.get("tags", []),
            infobox=data.get("infobox", []),
        )


@dataclass
class BangumiEpisode:
    """Bangumi 章节信息"""

    id: int
    type: int
    name: str
    name_cn: str
    ep: int
    sort: int
    airdate: str
    duration: str
    desc: str
    comment: int
    disc: int

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "BangumiEpisode":
        """从 API 响应创建对象"""
        return cls(
            id=data.get("id", 0),
            type=data.get("type", 0),
            name=data.get("name", ""),
            name_cn=data.get("name_cn", ""),
            ep=data.get("ep", 0),
            sort=data.get("sort", data.get("ep", 0)),
            airdate=data.get("airdate", ""),
            duration=data.get("duration", ""),
            desc=data.get("desc", ""),
            comment=data.get("comment", 0),
            disc=data.get("disc", 0),
        )


class BangumiClient:
    """Bangumi API 客户端"""

    def __init__(self, access_token: Optional[str] = None):
        """
        初始化客户端

        :param access_token: Bangumi Access Token（可选）
        """
        self.access_token = (access_token or "").strip()
        self._client: Optional[httpx.AsyncClient] = None
        self._current_username: Optional[str] = None

    async def _get_client(self) -> httpx.AsyncClient:
        """获取 HTTP 客户端"""
        if self._client is None:
            headers = {
                "User-Agent": "Twilight/1.0 (https://github.com/your-repo)",
                "Accept": "application/json",
            }
            if self.access_token:
                headers["Authorization"] = f"Bearer {self.access_token}"

            self._client = httpx.AsyncClient(
                base_url=_get_base_url(),
                headers=headers,
                timeout=30.0,
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

    async def _request(self, method: str, endpoint: str, **kwargs) -> Optional[Any]:
        """发送 HTTP 请求"""
        allow_404 = bool(kwargs.pop("allow_404", False))
        headers = {
            "User-Agent": "Twilight/1.0 (https://github.com/your-repo)",
            "Accept": "application/json",
        }
        if self.access_token:
            headers["Authorization"] = f"Bearer {self.access_token}"

        async with httpx.AsyncClient(
            base_url=_get_base_url(),
            headers=headers,
            timeout=30.0,
        ) as client:
            try:
                response = await client.request(method, endpoint, **kwargs)

                if response.status_code == 401:
                    raise BangumiError("Access Token 无效或已过期")
                elif response.status_code == 404:
                    if allow_404:
                        return None
                    raise BangumiError(f"资源未找到: {endpoint}")
                elif response.status_code >= 400:
                    raise BangumiError(f"请求失败: {response.status_code} - {response.text}")

                if response.content:
                    return response.json()
                return None

            except httpx.RequestError as e:
                logger.error(f"Bangumi 请求失败: {e}")
                raise BangumiError(f"网络请求失败: {e}")

    async def get_current_user(self) -> Optional[Dict[str, Any]]:
        """获取当前 Access Token 对应的 Bangumi 用户。"""
        if not self.access_token:
            return None
        data = await self._request("GET", "/me")
        if isinstance(data, dict):
            self._current_username = str(data.get("username") or data.get("id") or "") or None
            return data
        return None

    async def get_current_username(self) -> Optional[str]:
        """获取当前 Bangumi 用户名，供收藏查询 API 使用。"""
        if self._current_username:
            return self._current_username
        data = await self.get_current_user()
        if not data:
            return None
        return self._current_username

    async def get_subject(self, subject_id: int) -> Optional[BangumiSubject]:
        """
        获取条目信息

        :param subject_id: 条目 ID
        """
        try:
            data = await self._request("GET", f"/subjects/{subject_id}")
            if data:
                return BangumiSubject.from_dict(data)
            return None
        except BangumiError:
            raise
        except Exception as e:
            logger.error(f"获取条目信息失败: {e}")
            return None

    async def get_user_collection(self, subject_id: int) -> Optional[Dict[str, Any]]:
        """
        获取用户收藏信息

        :param subject_id: 条目 ID
        """
        if not self.access_token:
            return None

        try:
            username = await self.get_current_username()
            if not username:
                return None

            data = await self._request("GET", f"/users/{username}/collections/{subject_id}", allow_404=True)
            return data
        except BangumiError:
            raise
        except Exception as e:
            logger.error(f"获取用户收藏失败: {e}")
            return None

    async def update_collection(
        self,
        subject_id: int,
        status: int = 3,
        comment: Optional[str] = None,
        rating: Optional[int] = None,
        private: bool = False,
    ) -> bool:
        """
        更新用户收藏

        :param subject_id: 条目 ID
        :param status: 状态 (1=想看, 2=在看, 3=看过, 4=搁置, 5=抛弃)
        :param comment: 备注
        :param rating: 评分 (1-10)
        :param private: 是否私有
        """
        if not self.access_token:
            raise BangumiError("需要 Access Token")

        payload = {
            "type": status,
            "private": private,
        }
        if comment:
            payload["comment"] = comment
        if rating:
            payload["rating"] = rating

        try:
            await self._request("POST", f"/users/-/collections/{subject_id}", json=payload)
            return True
        except BangumiError:
            raise
        except Exception as e:
            logger.error(f"更新收藏失败: {e}")
            return False

    async def get_episode_collection(self, episode_id: int) -> Optional[Dict[str, Any]]:
        """获取当前用户对指定章节的收藏进度。"""
        if not self.access_token:
            return None
        try:
            data = await self._request("GET", f"/users/-/collections/-/episodes/{episode_id}", allow_404=True)
            return data if isinstance(data, dict) else None
        except BangumiError:
            raise
        except Exception as e:
            logger.error(f"获取章节收藏失败: {e}")
            return None

    async def update_episode_status(self, episode_id: int, status: int = 2) -> bool:
        """
        更新章节观看状态

        :param episode_id: 章节 ID
        :param status: Bangumi 章节收藏状态；2=看过
        """
        if not self.access_token:
            raise BangumiError("需要 Access Token")

        payload = {"type": status}

        try:
            await self._request("PUT", f"/users/-/collections/-/episodes/{episode_id}", json=payload)
            return True
        except BangumiError:
            raise
        except Exception as e:
            logger.error(f"更新章节状态失败: {e}")
            return False

    async def get_subject_episodes(self, subject_id: int, ep_type: int = 0, limit: int = 100) -> List[BangumiEpisode]:
        """获取条目的章节列表。默认只取本篇章节(type=0)。"""
        episodes: List[BangumiEpisode] = []
        offset = 0
        while True:
            data = await self._request(
                "GET",
                "/episodes",
                params={"subject_id": subject_id, "type": ep_type, "limit": limit, "offset": offset},
            )
            if not data:
                break
            items = data.get("data", []) if isinstance(data, dict) else []
            for item in items:
                try:
                    episodes.append(BangumiEpisode.from_dict(item))
                except Exception as e:
                    logger.warning(f"解析章节失败: {e}")
            total = int(data.get("total") or len(episodes)) if isinstance(data, dict) else len(episodes)
            if not items or len(episodes) >= total:
                break
            offset += len(items)
        return episodes

    async def mark_episode_by_ep_number(self, subject_id: int, ep_number: int, status: int = 2) -> bool:
        """按 Bangumi 本篇章节序号打格子。"""
        episodes = await self.get_subject_episodes(subject_id, ep_type=0)
        target = next((ep for ep in episodes if ep.sort == ep_number), None)
        if not target:
            target = next(
                (ep for ep in episodes if int(ep.ep or 0) == int(ep_number) and int(ep.ep or 0) <= int(ep.sort or 0)),
                None,
            )
        if not target:
            logger.warning("未找到 Bangumi 章节: subject_id=%s ep=%s", subject_id, ep_number)
            return False

        collection = await self.get_episode_collection(target.id)
        if collection and collection.get("type") == 2:
            return True
        return await self.update_episode_status(target.id, status=status)

    async def search(self, keyword: str, subject_type: Optional[int] = None, limit: int = 20) -> List[BangumiSubject]:
        """
        搜索条目

        基于 Bangumi API v0: POST /search/subjects
        文档: https://bangumi.github.io/api/#/search

        :param keyword: 搜索关键词
        :param subject_type: 条目类型 (1=书籍, 2=动画, 3=音乐, 4=游戏, 6=三次元)，None 表示所有类型
        :param limit: 返回数量限制
        :return: 搜索结果列表
        """
        # 清理关键词，移除可能影响搜索的标点
        sanitized_keyword = re.sub(r'[,?!()\[\]"\'。，、？！（）【】「」]', " ", keyword).strip()

        if not sanitized_keyword:
            return []

        # 构建搜索请求体
        filter_dict = {"nsfw": True}  # 允许 NSFW 内容
        if subject_type is not None:
            if isinstance(subject_type, SubjectType):
                subject_type = subject_type.value
            filter_dict["type"] = [subject_type]

        payload = {
            "keyword": sanitized_keyword,
            "sort": "rank",  # 按排名排序
            "filter": filter_dict,
        }

        try:
            data = await self._request("POST", "/search/subjects", json=payload)

            if not data or "data" not in data:
                return []

            subjects = []
            for item in data["data"][:limit]:
                try:
                    subject = BangumiSubject.from_dict(item)
                    subjects.append(subject)
                except Exception as e:
                    logger.warning(f"解析搜索结果失败: {e}")
                    continue

            return subjects

        except BangumiError:
            raise
        except Exception as e:
            logger.error(f"搜索失败: {e}")
            return []

    async def get_by_id(self, subject_id: int) -> Optional[BangumiSubject]:
        """
        根据 ID 获取条目信息（别名方法，兼容性）

        :param subject_id: 条目 ID
        :return: 条目信息
        """
        return await self.get_subject(subject_id)

    @staticmethod
    def parse_bgm_url(url: str) -> Optional[int]:
        """
        解析 Bangumi URL

        支持格式:
        - https://bgm.tv/subject/123
        - https://bangumi.tv/subject/456
        - bgm:123
        - 123 (纯数字)

        :return: 条目 ID 或 None
        """
        # URL 格式
        url_pattern = r"(?:bgm|bangumi)\.tv/subject/(\d+)"
        match = re.search(url_pattern, url)
        if match:
            return int(match.group(1))

        # 短格式
        short_pattern = r"bgm:(\d+)"
        match = re.search(short_pattern, url, re.IGNORECASE)
        if match:
            return int(match.group(1))

        # 纯数字
        if url.isdigit():
            return int(url)

        return None


# 全局客户端
_bangumi_client: Optional[BangumiClient] = None


def get_bangumi_client(access_token: Optional[str] = None) -> BangumiClient:
    """获取 Bangumi 客户端"""
    global _bangumi_client
    if access_token is not None:
        return BangumiClient(access_token=access_token)
    if _bangumi_client is None:
        _bangumi_client = BangumiClient(access_token=Config.BANGUMI_TOKEN)
    return _bangumi_client


async def close_bangumi_client() -> None:
    """关闭 Bangumi 客户端"""
    global _bangumi_client
    if _bangumi_client:
        await _bangumi_client.close()
        _bangumi_client = None
