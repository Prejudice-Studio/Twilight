"""
Bangumi API 客户端

基于 Bangumi API
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


class BangumiError(Exception):
    """Bangumi API 错误"""
    pass


class SubjectType(Enum):
    """条目类型"""
    BOOK = 1      # 书籍
    ANIME = 2     # 动画
    MUSIC = 3     # 音乐
    GAME = 4      # 游戏
    REAL = 6      # 三次元


@dataclass
class BangumiSubject:
    """Bangumi 条目信息"""
    id: int
    name: str
    name_cn: str
    type: int
    summary: str
    air_date: str
    images: Dict[str, str]
    score: float
    rank: int
    tags: List[Dict[str, Any]]
    
    @property
    def title(self) -> str:
        """优先返回中文名"""
        return self.name_cn if self.name_cn else self.name
    
    @property
    def cover_url(self) -> Optional[str]:
        if self.images:
            return self.images.get('large') or self.images.get('medium') or self.images.get('small')
        return None
    
    @property
    def bgm_url(self) -> str:
        return f"https://bgm.tv/subject/{self.id}"
    
    @property
    def type_name(self) -> str:
        type_map = {
            1: '书籍',
            2: '动画',
            3: '音乐',
            4: '游戏',
            6: '三次元',
        }
        return type_map.get(self.type, '未知')
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'BangumiSubject':
        return cls(
            id=data.get('id', 0),
            name=data.get('name', ''),
            name_cn=data.get('name_cn', ''),
            type=data.get('type', 0),
            summary=data.get('summary', ''),
            air_date=data.get('date', '') or data.get('air_date', ''),
            images=data.get('images', {}),
            score=data.get('score', 0) or data.get('rating', {}).get('score', 0),
            rank=data.get('rank', 0) or data.get('rating', {}).get('rank', 0),
            tags=data.get('tags', []),
        )
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            'id': self.id,
            'title': self.title,
            'original_title': self.name,
            'media_type': self.type_name,
            'type_id': self.type,
            'overview': self.summary[:300] + '...' if len(self.summary) > 300 else self.summary,
            'release_date': self.air_date,
            'year': self.air_date[:4] if self.air_date and len(self.air_date) >= 4 else None,
            'poster_url': self.cover_url,
            'vote_average': self.score,
            'rank': self.rank,
            'bgm_url': self.bgm_url,
            'tags': [t.get('name') for t in self.tags[:5]] if self.tags else [],
            'source': 'bangumi',
        }


class BangumiClient:
    """Bangumi API 客户端"""
    
    def __init__(
        self,
        access_token: Optional[str] = None,
        proxy: Optional[str] = None,
        timeout: float = 30.0
    ):
        self.access_token = access_token or Config.BANGUMI_TOKEN
        self.base_url = Config.BANGUMI_API_URL
        self.proxy = proxy or Config.PROXY
        self.timeout = timeout
        self._client: Optional[httpx.AsyncClient] = None
    
    async def _get_client(self) -> httpx.AsyncClient:
        if self._client is None or self._client.is_closed:
            headers = {
                'User-Agent': 'Twilight/1.0 (https://github.com/user/twilight)',
                'Accept': 'application/json',
            }
            if self.access_token:
                headers['Authorization'] = f'Bearer {self.access_token}'
            
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                proxy=self.proxy,
                headers=headers,
            )
        return self._client
    
    async def close(self) -> None:
        if self._client and not self._client.is_closed:
            await self._client.aclose()
            self._client = None
    
    async def _request(self, endpoint: str, params: Dict[str, Any] = None) -> Any:
        """发送请求"""
        client = await self._get_client()
        
        try:
            response = await client.get(endpoint, params=params)
            
            if response.status_code == 401:
                raise BangumiError("Bangumi Token 无效")
            elif response.status_code == 404:
                return None
            elif response.status_code != 200:
                raise BangumiError(f"Bangumi 请求失败: {response.status_code}")
            
            return response.json()
        except httpx.RequestError as e:
            raise BangumiError(f"Bangumi 请求错误: {e}")
    
    async def _post(self, endpoint: str, json_data: Dict[str, Any] = None) -> Any:
        """POST 请求"""
        client = await self._get_client()
        
        try:
            response = await client.post(endpoint, json=json_data)
            
            if response.status_code == 401:
                raise BangumiError("Bangumi Token 无效")
            elif response.status_code == 404:
                return None
            elif response.status_code not in (200, 201):
                raise BangumiError(f"Bangumi 请求失败: {response.status_code}")
            
            return response.json()
        except httpx.RequestError as e:
            raise BangumiError(f"Bangumi 请求错误: {e}")
    
    async def search(
        self,
        keyword: str,
        subject_type: SubjectType = None,
        limit: int = 20,
        offset: int = 0
    ) -> List[BangumiSubject]:
        """
        搜索条目
        
        支持中文、日文、英文、罗马音
        
        :param keyword: 搜索关键词
        :param subject_type: 条目类型（默认搜索全部）
        :param limit: 返回数量
        :param offset: 偏移量
        """
        # 新版 API 使用 POST 搜索
        search_data = {
            'keyword': keyword,
            'filter': {},
        }
        
        if subject_type:
            search_data['filter']['type'] = [subject_type.value]
        
        # 默认搜索动画和三次元（电视剧/电影）
        if not subject_type:
            search_data['filter']['type'] = [2, 6]  # 动画和三次元
        
        data = await self._post(f'/v0/search/subjects?limit={limit}&offset={offset}', search_data)
        
        if not data:
            return []
        
        results = []
        for item in data.get('data', []):
            results.append(BangumiSubject.from_dict(item))
        
        return results
    
    async def search_legacy(self, keyword: str, subject_type: int = None, max_results: int = 20) -> List[BangumiSubject]:
        """
        使用旧版搜索 API（备用）
        """
        params = {'max_results': max_results}
        if subject_type:
            params['type'] = subject_type
        
        # 旧版 API
        endpoint = f'/search/subject/{keyword}'
        data = await self._request(endpoint, params)
        
        if not data:
            return []
        
        results = []
        for item in data.get('list', []):
            results.append(BangumiSubject.from_dict(item))
        
        return results
    
    async def get_subject(self, subject_id: int) -> Optional[BangumiSubject]:
        """获取条目详情"""
        data = await self._request(f'/v0/subjects/{subject_id}')
        if not data:
            return None
        return BangumiSubject.from_dict(data)
    
    async def get_by_id(self, subject_id: int) -> Optional[BangumiSubject]:
        """根据 ID 获取条目"""
        return await self.get_subject(subject_id)
    
    @staticmethod
    def parse_bgm_url(url: str) -> Optional[int]:
        """
        解析 Bangumi URL
        
        支持格式:
        - https://bgm.tv/subject/123
        - https://bangumi.tv/subject/456
        - bgm:123
        - 纯数字
        
        :return: subject_id 或 None
        """
        # URL 格式
        url_pattern = r'(?:bgm|bangumi)\.tv/subject/(\d+)'
        match = re.search(url_pattern, url)
        if match:
            return int(match.group(1))
        
        # 短格式
        short_pattern = r'bgm:(\d+)'
        match = re.search(short_pattern, url, re.IGNORECASE)
        if match:
            return int(match.group(1))
        
        # 纯数字
        if url.isdigit():
            return int(url)
        
        return None


# 全局客户端
_bgm_client: Optional[BangumiClient] = None


def get_bangumi_client() -> BangumiClient:
    """获取 Bangumi 客户端"""
    global _bgm_client
    if _bgm_client is None:
        _bgm_client = BangumiClient()
    return _bgm_client


async def close_bangumi_client() -> None:
    """关闭 Bangumi 客户端"""
    global _bgm_client
    if _bgm_client:
        await _bgm_client.close()
        _bgm_client = None

