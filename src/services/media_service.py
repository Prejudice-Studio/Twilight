"""
媒体搜索服务

统一的媒体搜索接口，支持 TMDB 和 Bangumi
支持库存检查功能
"""
import re
import logging
from typing import Optional, List, Dict, Any, Tuple
from dataclasses import dataclass
from enum import Enum

from src.services.tmdb import get_tmdb_client, TMDBClient, TMDBMedia, TMDBError
from src.services.bangumi import get_bangumi_client, BangumiClient, BangumiSubject, BangumiError, SubjectType
from src.services.emby import get_emby_client, EmbyItem, EmbyError
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
            'poster': self.poster_url,  # 前端使用 poster 字段
            'poster_url': self.poster_url,  # 保留兼容性
            'vote_average': self.vote_average,
            'rating': self.vote_average,  # 前端使用 rating 字段
            'source': self.source,
            'source_url': self.source_url,
            'extra': self.extra or {},
        }


class MediaService:
    """媒体搜索服务"""
    
    @staticmethod
    def _tmdb_to_result(media: TMDBMedia, include_details: bool = False) -> MediaSearchResult:
        """将 TMDB 结果转换为统一格式"""
        # 根据是否包含详细信息决定是否截断简介
        overview = media.overview if include_details else (media.overview[:300] + '...' if len(media.overview) > 300 else media.overview)
        
        extra = {
            'vote_count': media.vote_count,
            'backdrop_url': media.backdrop_url,
            'original_language': media.original_language,
            'popularity': media.popularity,
        }
        
        # 如果包含详细信息，使用 media.to_dict() 中的完整信息
        if include_details and hasattr(media, 'to_dict'):
            media_dict = media.to_dict()
            if 'genres' in media_dict:
                extra['genres'] = media_dict['genres']
            if 'cast' in media_dict:
                extra['cast'] = media_dict['cast']
            if 'seasons' in media_dict:
                extra['seasons'] = media_dict['seasons']
            if 'runtime' in media_dict:
                extra['runtime'] = media_dict['runtime']
            if 'number_of_seasons' in media_dict:
                extra['number_of_seasons'] = media_dict['number_of_seasons']
            if 'number_of_episodes' in media_dict:
                extra['number_of_episodes'] = media_dict['number_of_episodes']
        
        return MediaSearchResult(
            id=media.id,
            title=media.title,
            original_title=media.original_title,
            media_type=media.media_type,
            overview=overview,
            release_date=media.release_date,
            year=media.release_date[:4] if media.release_date else None,
            poster_url=media.poster_url,
            vote_average=media.vote_average,
            source='tmdb',
            source_url=media.tmdb_url,
            extra=extra
        )
    
    @staticmethod
    def _bgm_to_result(subject: BangumiSubject, include_details: bool = False) -> MediaSearchResult:
        """将 Bangumi 结果转换为统一格式"""
        # 处理类型名称
        type_names = {
            1: '书籍',
            2: '动画',
            3: '音乐',
            4: '游戏',
            6: '三次元',
        }
        type_name = type_names.get(subject.type, '未知')
        
        # 处理评分
        score = 0.0
        rank = 0
        if subject.rating:
            score = subject.rating.get('score', 0.0) or 0.0
            rank = subject.rating.get('rank', 0) or 0
        
        # 处理简介（可能为 None 或空字符串）
        summary = subject.summary or ''
        # 根据是否包含详细信息决定是否截断简介
        overview = summary if include_details else (summary[:300] + '...' if len(summary) > 300 else summary)
        
        # 处理日期
        date = subject.date or ''
        year = date[:4] if date and len(date) >= 4 else None
        
        # 处理标签
        tags = []
        if subject.tags:
            for tag in subject.tags[:5]:
                if isinstance(tag, dict):
                    tags.append(tag.get('name', ''))
                elif isinstance(tag, str):
                    tags.append(tag)
        
        extra = {
            'rank': rank,
            'tags': tags,
            'type_id': subject.type,
            'eps': subject.eps,
            'volumes': subject.volumes,
        }
        
        # 如果包含详细信息，添加更多字段
        if include_details:
            extra.update({
                'name_cn': subject.name_cn,
                'name': subject.name,
                'infobox': getattr(subject, 'infobox', []),
            })
        
        return MediaSearchResult(
            id=subject.id,
            title=subject.title,
            original_title=subject.name,
            media_type=type_name,
            overview=overview,
            release_date=date,
            year=year,
            poster_url=subject.cover_url,
            vote_average=score,
            source='bangumi',
            source_url=subject.bgm_url,
            extra=extra
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
        limit: int = 20,
        year: Optional[int] = None,
        bgm_type: Optional[int] = None
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
                media = await tmdb.get_by_id(media_id, media_type, include_details=True)
                if media:
                    results.append(cls._tmdb_to_result(media, include_details=True))
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
                    results.append(cls._bgm_to_result(subject, include_details=True))
            except BangumiError as e:
                logger.error(f"Bangumi 查询失败: {e}")
            return results
        
        elif input_type == 'tmdb_id':
            media_type, media_id = value
            tmdb = get_tmdb_client()
            try:
                media = await tmdb.get_by_id(media_id, media_type, include_details=True)
                if media:
                    results.append(cls._tmdb_to_result(media, include_details=True))
            except TMDBError as e:
                logger.error(f"TMDB 查询失败: {e}")
            return results
        
        # 关键词搜索
        keyword = value
        half_limit = limit // 2
        
        # TMDB 搜索（只搜索电影和电视剧）
        if source in (MediaSource.ALL, MediaSource.TMDB):
            tmdb = get_tmdb_client()
            try:
                tmdb_results = await tmdb.search_multi(keyword)
                # 过滤：只保留 movie 和 tv 类型
                filtered_results = [
                    media for media in tmdb_results 
                    if media.media_type in ('movie', 'tv')
                ]
                for media in filtered_results[:half_limit if source == MediaSource.ALL else limit]:
                    results.append(cls._tmdb_to_result(media, include_details=False))
            except TMDBError as e:
                logger.warning(f"TMDB 搜索失败: {e}")
        
        # Bangumi 搜索（只搜索动画和三次元）
        if source in (MediaSource.ALL, MediaSource.BANGUMI):
            bgm = get_bangumi_client()
            try:
                from src.services.bangumi import SubjectType
                
                # 如果指定了 bgm_type，只搜索该类型
                if bgm_type is not None:
                    bgm_results = await bgm.search(
                        keyword,
                        subject_type=bgm_type,
                        limit=half_limit if source == MediaSource.ALL else limit
                    )
                    for subject in bgm_results:
                        # 年份过滤
                        if year and subject.date:
                            subject_year = subject.date[:4] if len(subject.date) >= 4 else None
                            if subject_year and int(subject_year) != year:
                                continue
                        results.append(cls._bgm_to_result(subject, include_details=False))
                else:
                    # 默认搜索动画（type=2）和三次元（type=6）
                    limit_per_type = (half_limit if source == MediaSource.ALL else limit) // 2
                    
                    # 并行搜索两种类型
                    import asyncio
                    anime_task = bgm.search(
                        keyword, 
                        subject_type=SubjectType.ANIME.value,  # 2
                        limit=limit_per_type
                    )
                    real_task = bgm.search(
                        keyword,
                        subject_type=SubjectType.REAL.value,  # 6
                        limit=limit_per_type
                    )
                    anime_results, real_results = await asyncio.gather(anime_task, real_task)
                    
                    # 合并结果并应用年份过滤
                    bgm_results = anime_results + real_results
                    for subject in bgm_results:
                        # 年份过滤
                        if year and subject.date:
                            subject_year = subject.date[:4] if len(subject.date) >= 4 else None
                            if subject_year and int(subject_year) != year:
                                continue
                        results.append(cls._bgm_to_result(subject, include_details=False))
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
    async def get_by_source_id(cls, source: str, media_id: int, media_type: str = None, include_details: bool = True) -> Optional[MediaSearchResult]:
        """根据来源和 ID 获取详情"""
        if source == 'tmdb':
            tmdb = get_tmdb_client()
            try:
                media = await tmdb.get_by_id(media_id, media_type or 'movie', include_details)
                if media:
                    return cls._tmdb_to_result(media, include_details)
            except TMDBError as e:
                logger.error(f"TMDB 获取详情失败: {e}")
                pass
        elif source == 'bangumi' or source == 'bgm':
            bgm = get_bangumi_client()
            try:
                subject = await bgm.get_by_id(media_id)
                if subject:
                    return cls._bgm_to_result(subject, include_details)
            except BangumiError as e:
                logger.error(f"Bangumi 获取详情失败: {e}")
                pass
        return None


@dataclass
class InventoryCheckResult:
    """库存检查结果"""
    exists: bool
    item: Optional[EmbyItem] = None
    message: str = ''
    seasons_available: List[int] = None  # 已有的季度列表
    season_requested: Optional[int] = None  # 请求的季度
    
    def __post_init__(self):
        if self.seasons_available is None:
            self.seasons_available = []
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            'exists': self.exists,
            'message': self.message,
            'item': self.item.to_dict() if self.item else None,
            'seasons_available': self.seasons_available,
            'season_requested': self.season_requested,
        }


class InventoryService:
    """库存检查服务"""
    
    @staticmethod
    async def check_by_title(
        title: str,
        year: Optional[int] = None,
        original_title: str = None,
        media_type: str = None,
        season: Optional[int] = None
    ) -> InventoryCheckResult:
        """
        通过标题检查库存
        
        :param title: 标题
        :param year: 年份
        :param original_title: 原标题
        :param media_type: 媒体类型 (movie/tv/anime)
        :param season: 季度（仅对剧集有效）
        """
        try:
            emby = get_emby_client()
            
            # 如果是电影，直接检查
            if media_type == 'movie':
                exists, item = await emby.check_media_exists(
                    title=title,
                    year=year,
                    original_title=original_title,
                    media_type='movie'
                )
                
                if exists and item:
                    return InventoryCheckResult(
                        exists=True,
                        item=item,
                        message=f"库中已有：{item.name}" + (f" ({item.year})" if item.year else "")
                    )
                
                return InventoryCheckResult(
                    exists=False,
                    message="库中暂无此电影"
                )
            
            # 剧集类型，检查季度
            exists, series, seasons = await emby.check_series_with_seasons(
                title=title,
                season=season,
                year=year,
                original_title=original_title
            )
            
            if not series:
                return InventoryCheckResult(
                    exists=False,
                    message="库中暂无此剧集",
                    season_requested=season
                )
            
            # 剧集存在，检查季度
            if season is not None:
                if season in seasons:
                    return InventoryCheckResult(
                        exists=True,
                        item=series,
                        message=f"库中已有：{series.name} 第 {season} 季",
                        seasons_available=seasons,
                        season_requested=season
                    )
                else:
                    seasons_str = ', '.join(str(s) for s in seasons) if seasons else "无"
                    return InventoryCheckResult(
                        exists=False,
                        item=series,
                        message=f"库中有 {series.name}，但缺少第 {season} 季\n已有季度：{seasons_str}",
                        seasons_available=seasons,
                        season_requested=season
                    )
            else:
                # 未指定季度，只检查剧集是否存在
                seasons_str = ', '.join(str(s) for s in seasons) if seasons else "无"
                return InventoryCheckResult(
                    exists=True,
                    item=series,
                    message=f"库中已有：{series.name}\n已有季度：{seasons_str}",
                    seasons_available=seasons
                )
            
        except EmbyError as e:
            logger.warning(f"库存检查失败: {e}")
            return InventoryCheckResult(
                exists=False,
                message=f"库存检查失败: {e}"
            )
    
    @staticmethod
    async def check_by_tmdb_id(
        tmdb_id: int,
        media_type: str = 'movie',
        season: Optional[int] = None
    ) -> InventoryCheckResult:
        """
        通过 TMDB ID 检查库存
        
        :param tmdb_id: TMDB ID
        :param media_type: 媒体类型
        :param season: 季度（仅对剧集有效）
        """
        try:
            emby = get_emby_client()
            emby_type = 'Movie' if media_type == 'movie' else 'Series'
            item = await emby.find_by_tmdb_id(tmdb_id, emby_type)
            
            if not item:
                return InventoryCheckResult(
                    exists=False,
                    message="库中暂无此媒体",
                    season_requested=season
                )
            
            # 如果是电影
            if media_type == 'movie':
                return InventoryCheckResult(
                    exists=True,
                    item=item,
                    message=f"库中已有：{item.name}" + (f" ({item.year})" if item.year else "")
                )
            
            # 如果是剧集，获取季度信息
            seasons = await emby.get_series_seasons(item.id)
            season_numbers = []
            
            for s in seasons:
                if s.index_number is not None:
                    season_numbers.append(s.index_number)
                elif s.name:
                    import re
                    match = re.search(r'(?:Season|第)\s*(\d+)', s.name, re.IGNORECASE)
                    if match:
                        season_numbers.append(int(match.group(1)))
            
            season_numbers.sort()
            
            if season is not None:
                if season in season_numbers:
                    return InventoryCheckResult(
                        exists=True,
                        item=item,
                        message=f"库中已有：{item.name} 第 {season} 季",
                        seasons_available=season_numbers,
                        season_requested=season
                    )
                else:
                    seasons_str = ', '.join(str(s) for s in season_numbers) if season_numbers else "无"
                    return InventoryCheckResult(
                        exists=False,
                        item=item,
                        message=f"库中有 {item.name}，但缺少第 {season} 季\n已有季度：{seasons_str}",
                        seasons_available=season_numbers,
                        season_requested=season
                    )
            else:
                seasons_str = ', '.join(str(s) for s in season_numbers) if season_numbers else "无"
                return InventoryCheckResult(
                    exists=True,
                    item=item,
                    message=f"库中已有：{item.name}\n已有季度：{seasons_str}",
                    seasons_available=season_numbers
                )
            
        except EmbyError as e:
            logger.warning(f"库存检查失败: {e}")
            return InventoryCheckResult(
                exists=False,
                message=f"库存检查失败: {e}"
            )
    
    @classmethod
    async def check_media(
        cls,
        media_info: Dict[str, Any],
        source: str,
        season: Optional[int] = None
    ) -> InventoryCheckResult:
        """
        根据媒体信息检查库存
        
        :param media_info: 媒体信息字典
        :param source: 来源 (tmdb/bangumi)
        :param season: 季度（仅对剧集有效）
        """
        title = media_info.get('title', '')
        original_title = media_info.get('original_title', '')
        year = None
        release_date = media_info.get('release_date', '')
        if release_date and len(release_date) >= 4:
            try:
                year = int(release_date[:4])
            except ValueError:
                pass
        
        media_type = media_info.get('media_type', '')
        if media_type in ('movie', '电影'):
            media_type = 'movie'
        elif media_type in ('tv', '剧集', '动画', 'anime'):
            media_type = 'tv'
        
        # 如果是 TMDB 来源且有 ID，优先通过 ID 查找
        if source == 'tmdb':
            tmdb_id = media_info.get('id')
            if tmdb_id:
                return await cls.check_by_tmdb_id(tmdb_id, media_type, season)
        
        # 通过标题搜索
        return await cls.check_by_title(
            title=title,
            year=year,
            original_title=original_title,
            media_type=media_type,
            season=season
        )


class MediaRequestService:
    """媒体求片服务"""
    
    @staticmethod
    async def check_inventory(
        source: str,
        media_id: int,
        media_info: Dict[str, Any] = None,
        season: Optional[int] = None
    ) -> InventoryCheckResult:
        """
        检查库存
        
        :param source: 来源 ('tmdb' 或 'bangumi')
        :param media_id: 媒体 ID
        :param media_info: 媒体详细信息
        :param season: 季度（仅对剧集有效）
        """
        if not media_info:
            # 获取媒体信息
            result = await MediaService.get_by_source_id(source, media_id)
            if result:
                media_info = result.to_dict()
            else:
                return InventoryCheckResult(
                    exists=False,
                    message="无法获取媒体信息"
                )
        
        return await InventoryService.check_media(media_info, source, season)
    
    @staticmethod
    async def create_request(
        telegram_id: int,
        source: str,
        media_id: int,
        media_info: Dict[str, Any] = None,
        skip_inventory_check: bool = False,
        season: Optional[int] = None
    ) -> Tuple[bool, str, Optional[int]]:
        """
        创建求片请求
        
        :param telegram_id: 用户 Telegram ID
        :param source: 来源 ('tmdb' 或 'bangumi')
        :param media_id: 媒体 ID
        :param media_info: 媒体信息（可选，用于存储额外信息）
        :param skip_inventory_check: 是否跳过库存检查
        :param season: 季度（仅对剧集有效）
        :return: (成功, 消息, 请求ID)
        """
        import json
        
        # 检查用户是否存在
        user = await UserOperate.get_user_by_telegram_id(telegram_id)
        if not user:
            return False, "用户不存在", None
        
        # 生成唯一 ID（考虑季度）
        unique_id = media_id
        if season is not None:
            # 使用 media_id * 1000 + season 作为唯一标识
            unique_id = media_id * 1000 + season
        
        # 检查是否已有相同请求
        existing = await BangumiRequireOperate.is_bangumi_exist(unique_id)
        if existing:
            status = ReqStatus(existing.status)
            status_msg = {
                ReqStatus.UNHANDLED: "待处理",
                ReqStatus.ACCEPTED: "已接受，正在处理中",
                ReqStatus.REJECTED: "已被拒绝",
                ReqStatus.COMPLETED: "已完成",
            }.get(status, "未知状态")
            season_str = f" 第 {season} 季" if season else ""
            return False, f"该媒体{season_str}已被请求过（{status_msg}）", existing.id
        
        # 库存检查（除非明确跳过）
        if not skip_inventory_check:
            inventory_result = await MediaRequestService.check_inventory(source, media_id, media_info, season)
            if inventory_result.exists:
                item = inventory_result.item
                msg = inventory_result.message
                if not msg:
                    msg = f"📦 库中已有：{item.name}" if item else "📦 库中已有此媒体"
                msg = f"📦 {msg}\n无需再次请求，请在媒体库中搜索观看。"
                return False, msg, None
        
        # 添加季度信息到 media_info
        if media_info is None:
            media_info = {}
        if season is not None:
            media_info['season'] = season
        
        # 创建请求
        other_info = json.dumps({
            'source': source,
            'media_info': media_info,
            'season': season,
        }, ensure_ascii=False)
        
        request = BangumiRequireModel(
            telegram_id=telegram_id,
            bangumi_id=unique_id,
            status=ReqStatus.UNHANDLED.value,
            timestamp=timestamp(),
            other_info=other_info,
        )
        
        await BangumiRequireOperate.add_require(request)
        
        season_str = f" 第 {season} 季" if season else ""
        return True, f"✅ 求片请求{season_str}已提交，请等待管理员处理", request.id
    
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

