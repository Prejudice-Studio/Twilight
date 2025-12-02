"""
媒体搜索服务

统一的媒体搜索接口，支持 TMDB 和 Bangumi
"""
import re
import logging
from typing import Optional, List, Dict, Any, Tuple
from dataclasses import dataclass
from enum import Enum

from src.services.tmdb import get_tmdb_client, TMDBClient, TMDBMedia, TMDBError
from src.services.bangumi import get_bangumi_client, BangumiClient, BangumiSubject, BangumiError, SubjectType
from src.db.bangumi import BangumiRequireModel, BangumiRequireOperate, ReqStatus
from src.db.user import UserOperate
from src.core.utils import timestamp

logger = logging.getLogger(__name__)


class MediaSource(Enum):
    """媒体来源"""
    TMDB = 'tmdb'
    BANGUMI = 'bangumi'
    ALL = 'all'


@dataclass
class MediaSearchResult:
    """统一的媒体搜索结果"""
    id: int
    title: str
    original_title: str
    media_type: str
    overview: str
    release_date: str
    year: Optional[str]
    poster_url: Optional[str]
    vote_average: float
    source: str  # 'tmdb' or 'bangumi'
    source_url: str
    extra: Dict[str, Any] = None
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            'id': self.id,
            'title': self.title,
            'original_title': self.original_title,
            'media_type': self.media_type,
            'overview': self.overview,
            'release_date': self.release_date,
            'year': self.year,
            'poster_url': self.poster_url,
            'vote_average': self.vote_average,
            'source': self.source,
            'source_url': self.source_url,
            'extra': self.extra or {},
        }


class MediaService:
    """媒体搜索服务"""
    
    @staticmethod
    def _tmdb_to_result(media: TMDBMedia) -> MediaSearchResult:
        """将 TMDB 结果转换为统一格式"""
        return MediaSearchResult(
            id=media.id,
            title=media.title,
            original_title=media.original_title,
            media_type=media.media_type,
            overview=media.overview[:300] + '...' if len(media.overview) > 300 else media.overview,
            release_date=media.release_date,
            year=media.release_date[:4] if media.release_date else None,
            poster_url=media.poster_url,
            vote_average=media.vote_average,
            source='tmdb',
            source_url=media.tmdb_url,
            extra={
                'vote_count': media.vote_count,
                'backdrop_url': media.backdrop_url,
            }
        )
    
    @staticmethod
    def _bgm_to_result(subject: BangumiSubject) -> MediaSearchResult:
        """将 Bangumi 结果转换为统一格式"""
        return MediaSearchResult(
            id=subject.id,
            title=subject.title,
            original_title=subject.name,
            media_type=subject.type_name,
            overview=subject.summary[:300] + '...' if len(subject.summary) > 300 else subject.summary,
            release_date=subject.air_date,
            year=subject.air_date[:4] if subject.air_date and len(subject.air_date) >= 4 else None,
            poster_url=subject.cover_url,
            vote_average=subject.score,
            source='bangumi',
            source_url=subject.bgm_url,
            extra={
                'rank': subject.rank,
                'tags': [t.get('name') for t in subject.tags[:5]] if subject.tags else [],
                'type_id': subject.type,
            }
        )
    
    @staticmethod
    def detect_input_type(query: str) -> Tuple[str, Any]:
        """
        检测用户输入类型
        
        :return: (type, value)
            type: 'tmdb_url', 'bgm_url', 'tmdb_id', 'bgm_id', 'keyword'
            value: 解析后的值
        """
        query = query.strip()
        
        # TMDB URL
        tmdb_result = TMDBClient.parse_tmdb_url(query)
        if tmdb_result:
            return 'tmdb_url', tmdb_result
        
        # Bangumi URL
        bgm_id = BangumiClient.parse_bgm_url(query)
        if bgm_id and not query.isdigit():  # 排除纯数字（可能是关键词）
            return 'bgm_url', bgm_id
        
        # TMDB ID 格式: tmdb:123 或 tmdb:movie:123
        tmdb_id_pattern = r'^tmdb:(?:(movie|tv):)?(\d+)$'
        match = re.match(tmdb_id_pattern, query, re.IGNORECASE)
        if match:
            media_type = match.group(1) or 'movie'
            return 'tmdb_id', (media_type, int(match.group(2)))
        
        # Bangumi ID 格式: bgm:123
        bgm_id_pattern = r'^bgm:(\d+)$'
        match = re.match(bgm_id_pattern, query, re.IGNORECASE)
        if match:
            return 'bgm_id', int(match.group(1))
        
        # 默认为关键词搜索
        return 'keyword', query
    
    @classmethod
    async def search(
        cls,
        query: str,
        source: MediaSource = MediaSource.ALL,
        limit: int = 20
    ) -> List[MediaSearchResult]:
        """
        统一搜索接口
        
        支持：
        - 中文名、英文名、日文名、罗马音
        - TMDB/Bangumi URL
        - TMDB/Bangumi ID
        
        :param query: 搜索关键词或 URL/ID
        :param source: 搜索来源
        :param limit: 返回数量
        """
        input_type, value = cls.detect_input_type(query)
        results = []
        
        # 根据输入类型处理
        if input_type == 'tmdb_url':
            # 直接获取 TMDB 详情
            media_type, media_id = value
            tmdb = get_tmdb_client()
            try:
                media = await tmdb.get_by_id(media_id, media_type)
                if media:
                    results.append(cls._tmdb_to_result(media))
            except TMDBError as e:
                logger.error(f"TMDB 查询失败: {e}")
            return results
        
        elif input_type == 'bgm_url' or input_type == 'bgm_id':
            # 直接获取 Bangumi 详情
            subject_id = value if input_type == 'bgm_id' else value
            bgm = get_bangumi_client()
            try:
                subject = await bgm.get_by_id(subject_id)
                if subject:
                    results.append(cls._bgm_to_result(subject))
            except BangumiError as e:
                logger.error(f"Bangumi 查询失败: {e}")
            return results
        
        elif input_type == 'tmdb_id':
            media_type, media_id = value
            tmdb = get_tmdb_client()
            try:
                media = await tmdb.get_by_id(media_id, media_type)
                if media:
                    results.append(cls._tmdb_to_result(media))
            except TMDBError as e:
                logger.error(f"TMDB 查询失败: {e}")
            return results
        
        # 关键词搜索
        keyword = value
        half_limit = limit // 2
        
        # TMDB 搜索
        if source in (MediaSource.ALL, MediaSource.TMDB):
            tmdb = get_tmdb_client()
            try:
                tmdb_results = await tmdb.search_multi(keyword)
                for media in tmdb_results[:half_limit if source == MediaSource.ALL else limit]:
                    results.append(cls._tmdb_to_result(media))
            except TMDBError as e:
                logger.warning(f"TMDB 搜索失败: {e}")
        
        # Bangumi 搜索
        if source in (MediaSource.ALL, MediaSource.BANGUMI):
            bgm = get_bangumi_client()
            try:
                bgm_results = await bgm.search(keyword, limit=half_limit if source == MediaSource.ALL else limit)
                for subject in bgm_results:
                    results.append(cls._bgm_to_result(subject))
            except BangumiError as e:
                logger.warning(f"Bangumi 搜索失败: {e}")
        
        # 按评分排序
        results.sort(key=lambda x: x.vote_average or 0, reverse=True)
        
        return results[:limit]
    
    @classmethod
    async def search_tmdb(cls, query: str, media_type: str = None, limit: int = 20) -> List[MediaSearchResult]:
        """仅搜索 TMDB"""
        return await cls.search(query, MediaSource.TMDB, limit)
    
    @classmethod
    async def search_bangumi(cls, query: str, limit: int = 20) -> List[MediaSearchResult]:
        """仅搜索 Bangumi"""
        return await cls.search(query, MediaSource.BANGUMI, limit)
    
    @classmethod
    async def get_by_source_id(cls, source: str, media_id: int, media_type: str = None) -> Optional[MediaSearchResult]:
        """根据来源和 ID 获取详情"""
        if source == 'tmdb':
            tmdb = get_tmdb_client()
            try:
                media = await tmdb.get_by_id(media_id, media_type or 'movie')
                if media:
                    return cls._tmdb_to_result(media)
            except TMDBError:
                pass
        elif source == 'bangumi':
            bgm = get_bangumi_client()
            try:
                subject = await bgm.get_by_id(media_id)
                if subject:
                    return cls._bgm_to_result(subject)
            except BangumiError:
                pass
        return None


class MediaRequestService:
    """媒体求片服务"""
    
    @staticmethod
    async def create_request(
        telegram_id: int,
        source: str,
        media_id: int,
        media_info: Dict[str, Any] = None
    ) -> Tuple[bool, str, Optional[int]]:
        """
        创建求片请求
        
        :param telegram_id: 用户 Telegram ID
        :param source: 来源 ('tmdb' 或 'bangumi')
        :param media_id: 媒体 ID
        :param media_info: 媒体信息（可选，用于存储额外信息）
        :return: (成功, 消息, 请求ID)
        """
        import json
        
        # 检查用户是否存在
        user = await UserOperate.get_user_by_telegram_id(telegram_id)
        if not user:
            return False, "用户不存在", None
        
        # 检查是否已有相同请求
        existing = await BangumiRequireOperate.is_bangumi_exist(media_id)
        if existing:
            return False, "该媒体已被请求过", existing.id
        
        # 创建请求
        other_info = json.dumps({
            'source': source,
            'media_info': media_info,
        }, ensure_ascii=False) if media_info else json.dumps({'source': source})
        
        request = BangumiRequireModel(
            telegram_id=telegram_id,
            bangumi_id=media_id,
            status=ReqStatus.UNHANDLED.value,
            timestamp=timestamp(),
            other_info=other_info,
        )
        
        await BangumiRequireOperate.add_require(request)
        
        return True, "求片请求已提交", request.id
    
    @staticmethod
    async def get_user_requests(telegram_id: int) -> List[Dict[str, Any]]:
        """获取用户的求片列表"""
        import json
        
        requests = await BangumiRequireOperate.get_requires_by_user(telegram_id)
        
        results = []
        for req in requests:
            other = {}
            if req.other_info:
                try:
                    other = json.loads(req.other_info)
                except:
                    pass
            
            results.append({
                'id': req.id,
                'media_id': req.bangumi_id,
                'source': other.get('source', 'unknown'),
                'status': ReqStatus(req.status).name,
                'timestamp': req.timestamp,
                'media_info': other.get('media_info'),
            })
        
        return results
    
    @staticmethod
    async def get_pending_requests() -> List[Dict[str, Any]]:
        """获取待处理的求片列表"""
        import json
        
        requests = await BangumiRequireOperate.get_pending_list()
        
        results = []
        for req in requests:
            other = {}
            if req.other_info:
                try:
                    other = json.loads(req.other_info)
                except:
                    pass
            
            # 获取用户信息
            user = await UserOperate.get_user_by_telegram_id(req.telegram_id)
            
            results.append({
                'id': req.id,
                'media_id': req.bangumi_id,
                'source': other.get('source', 'unknown'),
                'status': ReqStatus(req.status).name,
                'timestamp': req.timestamp,
                'media_info': other.get('media_info'),
                'user': {
                    'telegram_id': req.telegram_id,
                    'username': user.USERNAME if user else None,
                } if user else None,
            })
        
        return results
    
    @staticmethod
    async def update_request_status(request_id: int, status: ReqStatus) -> Tuple[bool, str]:
        """更新求片状态"""
        success = await BangumiRequireOperate.update_status(request_id, status)
        if success:
            return True, f"状态已更新为 {status.name}"
        return False, "请求不存在"

