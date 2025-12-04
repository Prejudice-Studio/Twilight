"""
TMDB API 客户端

基于 The Movie Database (TMDB) API v3
文档: https://developer.themoviedb.org/docs
"""
import re
import logging
from typing import Optional, List, Dict, Any
from dataclasses import dataclass
from enum import Enum

import httpx

from src.config import Config

logger = logging.getLogger(__name__)


class TMDBError(Exception):
    """TMDB API 错误"""
    pass


class MediaType(Enum):
    """媒体类型"""
    MOVIE = 'movie'
    TV = 'tv'
    PERSON = 'person'
    MULTI = 'multi'


@dataclass
class TMDBMedia:
    """TMDB 媒体信息"""
    id: int
    title: str
    original_title: str
    media_type: str  # movie, tv
    overview: str
    release_date: str
    poster_path: Optional[str]
    backdrop_path: Optional[str]
    vote_average: float
    vote_count: int
    popularity: float
    genre_ids: List[int]
    original_language: str
    
    @property
    def poster_url(self) -> Optional[str]:
        if self.poster_path:
            return f"{Config.TMDB_IMAGE_URL}/w500{self.poster_path}"
        return None
    
    @property
    def backdrop_url(self) -> Optional[str]:
        if self.backdrop_path:
            return f"{Config.TMDB_IMAGE_URL}/w1280{self.backdrop_path}"
        return None
    
    @property
    def tmdb_url(self) -> str:
        return f"https://www.themoviedb.org/{self.media_type}/{self.id}"
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any], media_type: str = None) -> 'TMDBMedia':
        # 处理 movie 和 tv 不同的字段名
        if media_type == 'movie' or data.get('title'):
            title = data.get('title', '')
            original_title = data.get('original_title', '')
            release_date = data.get('release_date', '')
            m_type = 'movie'
        else:
            title = data.get('name', '')
            original_title = data.get('original_name', '')
            release_date = data.get('first_air_date', '')
            m_type = 'tv'
        
        return cls(
            id=data.get('id', 0),
            title=title,
            original_title=original_title,
            media_type=data.get('media_type', m_type),
            overview=data.get('overview', ''),
            release_date=release_date,
            poster_path=data.get('poster_path'),
            backdrop_path=data.get('backdrop_path'),
            vote_average=data.get('vote_average', 0),
            vote_count=data.get('vote_count', 0),
            popularity=data.get('popularity', 0),
            genre_ids=data.get('genre_ids', []),
            original_language=data.get('original_language', ''),
        )
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            'id': self.id,
            'title': self.title,
            'original_title': self.original_title,
            'media_type': self.media_type,
            'overview': self.overview[:300] + '...' if len(self.overview) > 300 else self.overview,
            'release_date': self.release_date,
            'year': self.release_date[:4] if self.release_date else None,
            'poster_url': self.poster_url,
            'backdrop_url': self.backdrop_url,
            'vote_average': self.vote_average,
            'vote_count': self.vote_count,
            'tmdb_url': self.tmdb_url,
            'source': 'tmdb',
        }


class TMDBClient:
    """TMDB API 客户端"""
    
    def __init__(
        self,
        api_key: Optional[str] = None,
        language: str = 'zh-CN',
        proxy: Optional[str] = None,
        timeout: float = 30.0
    ):
        self.api_key = api_key or Config.TMDB_API_KEY
        self.base_url = Config.TMDB_API_URL
        self.language = language
        self.proxy = proxy
        self.timeout = timeout
        self._client: Optional[httpx.AsyncClient] = None
    
    async def _get_client(self) -> httpx.AsyncClient:
        # 每次请求都创建新的客户端，避免事件循环问题
        # 使用 context manager 确保正确关闭
        return httpx.AsyncClient(
            base_url=self.base_url,
            timeout=self.timeout,
            proxy=self.proxy,
            params={'api_key': self.api_key},
        )
    
    async def close(self) -> None:
        if self._client and not self._client.is_closed:
            await self._client.aclose()
            self._client = None
    
    async def _request(self, endpoint: str, params: Dict[str, Any] = None) -> Dict[str, Any]:
        """发送请求"""
        if not self.api_key:
            raise TMDBError("TMDB API Key 未配置")
        
        request_params = {'language': self.language}
        if params:
            request_params.update(params)
        
        # 使用 context manager 确保客户端正确关闭
        async with httpx.AsyncClient(
            base_url=self.base_url,
            timeout=self.timeout,
            proxy=self.proxy,
            params={'api_key': self.api_key},
        ) as client:
            try:
                response = await client.get(endpoint, params=request_params)
                
                if response.status_code == 401:
                    raise TMDBError("TMDB API Key 无效")
                elif response.status_code == 404:
                    return None
                elif response.status_code != 200:
                    raise TMDBError(f"TMDB 请求失败: {response.status_code}")
                
                return response.json()
            except httpx.RequestError as e:
                raise TMDBError(f"TMDB 请求错误: {e}")
    
    async def search_multi(self, query: str, page: int = 1) -> List[TMDBMedia]:
        """
        多类型搜索（电影、电视剧、人物）
        
        支持中文、英文、日文、罗马音等
        """
        data = await self._request('/search/multi', {
            'query': query,
            'page': page,
            'include_adult': 'false',
        })
        
        if not data:
            return []
        
        results = []
        for item in data.get('results', []):
            media_type = item.get('media_type')
            if media_type in ('movie', 'tv'):
                results.append(TMDBMedia.from_dict(item))
        
        return results
    
    async def search_movie(self, query: str, page: int = 1, year: int = None) -> List[TMDBMedia]:
        """搜索电影"""
        params = {'query': query, 'page': page}
        if year:
            params['year'] = year
        
        data = await self._request('/search/movie', params)
        if not data:
            return []
        
        return [TMDBMedia.from_dict(item, 'movie') for item in data.get('results', [])]
    
    async def search_tv(self, query: str, page: int = 1, year: int = None) -> List[TMDBMedia]:
        """搜索电视剧"""
        params = {'query': query, 'page': page}
        if year:
            params['first_air_date_year'] = year
        
        data = await self._request('/search/tv', params)
        if not data:
            return []
        
        return [TMDBMedia.from_dict(item, 'tv') for item in data.get('results', [])]
    
    async def get_movie(self, movie_id: int, include_details: bool = True) -> Optional[TMDBMedia]:
        """获取电影详情"""
        endpoint = f'/movie/{movie_id}'
        if include_details:
            # 包含额外信息：演员、类型、视频等
            endpoint += '?append_to_response=credits,genres,videos,images'
        data = await self._request(endpoint)
        if not data:
            return None
        return TMDBMedia.from_dict(data, 'movie')
    
    async def get_tv(self, tv_id: int, include_details: bool = True) -> Optional[TMDBMedia]:
        """获取电视剧详情"""
        endpoint = f'/tv/{tv_id}'
        if include_details:
            # 包含额外信息：演员、类型、视频、季度等
            endpoint += '?append_to_response=credits,genres,videos,images,seasons'
        data = await self._request(endpoint)
        if not data:
            return None
        return TMDBMedia.from_dict(data, 'tv')
    
    async def get_by_id(self, media_id: int, media_type: str = 'movie', include_details: bool = True) -> Optional[TMDBMedia]:
        """根据 ID 获取媒体详情"""
        if media_type == 'movie':
            return await self.get_movie(media_id, include_details)
        elif media_type == 'tv':
            return await self.get_tv(media_id, include_details)
        return None
    
    @staticmethod
    def parse_tmdb_url(url: str) -> Optional[tuple]:
        """
        解析 TMDB URL
        
        支持格式:
        - https://www.themoviedb.org/movie/123
        - https://www.themoviedb.org/tv/456
        - tmdb:movie:123
        - tmdb:tv:456
        
        :return: (media_type, media_id) 或 None
        """
        # URL 格式
        url_pattern = r'themoviedb\.org/(movie|tv)/(\d+)'
        match = re.search(url_pattern, url)
        if match:
            return match.group(1), int(match.group(2))
        
        # 短格式
        short_pattern = r'tmdb:(movie|tv):(\d+)'
        match = re.search(short_pattern, url, re.IGNORECASE)
        if match:
            return match.group(1), int(match.group(2))
        
        # 纯数字（默认电影）
        if url.isdigit():
            return 'movie', int(url)
        
        return None


# 全局客户端
_tmdb_client: Optional[TMDBClient] = None


def get_tmdb_client() -> TMDBClient:
    """获取 TMDB 客户端"""
    global _tmdb_client
    if _tmdb_client is None:
        _tmdb_client = TMDBClient()
    return _tmdb_client


async def close_tmdb_client() -> None:
    """关闭 TMDB 客户端"""
    global _tmdb_client
    if _tmdb_client:
        await _tmdb_client.close()
        _tmdb_client = None

