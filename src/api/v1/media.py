"""
媒体搜索 API

提供 TMDB 和 Bangumi 统一搜索接口
"""
from flask import Blueprint, request, g

from src.api.v1.auth import async_route, require_auth, api_response
from src.services import MediaService, MediaRequestService, MediaSource
from src.db.bangumi import ReqStatus

media_bp = Blueprint('media', __name__, url_prefix='/media')


# ==================== 媒体搜索 ====================

@media_bp.route('/search', methods=['GET'])
@async_route
async def search_media():
    """
    统一媒体搜索
    
    支持输入：
    - 中文名、英文名、日文名、罗马音
    - TMDB URL: https://www.themoviedb.org/movie/123
    - Bangumi URL: https://bgm.tv/subject/456
    - TMDB ID: tmdb:movie:123 或 tmdb:tv:123
    - Bangumi ID: bgm:456
    
    Query:
        q: str - 搜索关键词/URL/ID
        source: str - 来源 (all/tmdb/bangumi，默认 all)
        limit: int - 返回数量（默认 20，最大 50）
    
    Response:
        {
            "success": true,
            "data": {
                "query": "进击的巨人",
                "source": "all",
                "results": [
                    {
                        "id": 123,
                        "title": "进击的巨人",
                        "original_title": "進撃の巨人",
                        "media_type": "tv",
                        "overview": "简介...",
                        "release_date": "2013-04-07",
                        "year": "2013",
                        "poster_url": "https://...",
                        "vote_average": 8.5,
                        "source": "tmdb",
                        "source_url": "https://www.themoviedb.org/tv/123"
                    },
                    {
                        "id": 456,
                        "title": "进击的巨人",
                        "original_title": "進撃の巨人",
                        "media_type": "动画",
                        "overview": "简介...",
                        "release_date": "2013-04-07",
                        "year": "2013",
                        "poster_url": "https://...",
                        "vote_average": 8.8,
                        "source": "bangumi",
                        "source_url": "https://bgm.tv/subject/456"
                    }
                ]
            }
        }
    """
    query = request.args.get('q', '').strip()
    source = request.args.get('source', 'all').lower()
    limit = request.args.get('limit', 20, type=int)
    
    if not query:
        return api_response(False, "缺少搜索关键词", code=400)
    
    limit = min(max(limit, 1), 50)
    
    # 确定搜索来源
    if source == 'tmdb':
        media_source = MediaSource.TMDB
    elif source == 'bangumi' or source == 'bgm':
        media_source = MediaSource.BANGUMI
    else:
        media_source = MediaSource.ALL
    
    try:
        results = await MediaService.search(query, media_source, limit)
        
        return api_response(True, f"找到 {len(results)} 个结果", {
            'query': query,
            'source': source,
            'count': len(results),
            'results': [r.to_dict() for r in results],
        })
    except Exception as e:
        return api_response(False, f"搜索失败: {e}", code=500)


@media_bp.route('/search/tmdb', methods=['GET'])
@async_route
async def search_tmdb():
    """
    仅搜索 TMDB
    
    Query:
        q: str - 搜索关键词
        type: str - 类型 (movie/tv，可选)
        limit: int - 返回数量
    """
    query = request.args.get('q', '').strip()
    media_type = request.args.get('type')
    limit = request.args.get('limit', 20, type=int)
    
    if not query:
        return api_response(False, "缺少搜索关键词", code=400)
    
    limit = min(max(limit, 1), 50)
    
    try:
        results = await MediaService.search_tmdb(query, limit)
        
        # 如果指定类型，过滤结果
        if media_type:
            results = [r for r in results if r.media_type == media_type]
        
        return api_response(True, f"找到 {len(results)} 个结果", {
            'query': query,
            'count': len(results),
            'results': [r.to_dict() for r in results],
        })
    except Exception as e:
        return api_response(False, f"搜索失败: {e}", code=500)


@media_bp.route('/search/bangumi', methods=['GET'])
@async_route
async def search_bangumi():
    """
    仅搜索 Bangumi
    
    Query:
        q: str - 搜索关键词
        type: int - 类型 (2=动画, 6=三次元，可选)
        limit: int - 返回数量
    """
    query = request.args.get('q', '').strip()
    subject_type = request.args.get('type', type=int)
    limit = request.args.get('limit', 20, type=int)
    
    if not query:
        return api_response(False, "缺少搜索关键词", code=400)
    
    limit = min(max(limit, 1), 50)
    
    try:
        results = await MediaService.search_bangumi(query, limit)
        
        # 如果指定类型，过滤结果
        if subject_type:
            results = [r for r in results if r.extra and r.extra.get('type_id') == subject_type]
        
        return api_response(True, f"找到 {len(results)} 个结果", {
            'query': query,
            'count': len(results),
            'results': [r.to_dict() for r in results],
        })
    except Exception as e:
        return api_response(False, f"搜索失败: {e}", code=500)


@media_bp.route('/detail', methods=['GET'])
@async_route
async def get_media_detail():
    """
    获取媒体详情
    
    Query:
        source: str - 来源 (tmdb/bangumi)
        id: int - 媒体 ID
        type: str - 类型 (tmdb: movie/tv，可选)
    """
    source = request.args.get('source', '').lower()
    media_id = request.args.get('id', type=int)
    media_type = request.args.get('type', 'movie')
    
    if not source or not media_id:
        return api_response(False, "缺少必要参数 (source, id)", code=400)
    
    if source not in ('tmdb', 'bangumi', 'bgm'):
        return api_response(False, "无效的来源，支持: tmdb, bangumi", code=400)
    
    if source == 'bgm':
        source = 'bangumi'
    
    try:
        result = await MediaService.get_by_source_id(source, media_id, media_type)
        
        if result:
            return api_response(True, "获取成功", result.to_dict())
        return api_response(False, "媒体不存在", code=404)
    except Exception as e:
        return api_response(False, f"获取失败: {e}", code=500)


# ==================== 求片功能 ====================

@media_bp.route('/request', methods=['POST'])
@async_route
@require_auth
async def create_media_request():
    """
    创建求片请求
    
    Request:
        {
            "source": "tmdb",           // tmdb 或 bangumi
            "media_id": 123,            // 媒体 ID
            "media_type": "movie",      // 可选，tmdb 的类型
            "title": "电影名称",         // 可选，用于记录
            "note": "备注信息"           // 可选
        }
    
    或者直接搜索后选择：
        {
            "query": "进击的巨人",       // 搜索关键词
            "index": 0                  // 选择搜索结果的索引
        }
    """
    data = request.get_json() or {}
    
    # 方式1: 直接指定
    source = data.get('source')
    media_id = data.get('media_id')
    
    # 方式2: 搜索后选择
    query = data.get('query')
    index = data.get('index')
    
    media_info = None
    
    if source and media_id:
        # 直接指定方式
        media_type = data.get('media_type', 'movie')
        
        # 获取媒体信息
        result = await MediaService.get_by_source_id(source, media_id, media_type)
        if result:
            media_info = {
                'title': result.title,
                'original_title': result.original_title,
                'media_type': result.media_type,
                'year': result.year,
                'source_url': result.source_url,
            }
        
        if data.get('title'):
            media_info = media_info or {}
            media_info['title'] = data.get('title')
        if data.get('note'):
            media_info = media_info or {}
            media_info['note'] = data.get('note')
    
    elif query and index is not None:
        # 搜索后选择方式
        try:
            results = await MediaService.search(query, MediaSource.ALL, 20)
            if index < 0 or index >= len(results):
                return api_response(False, f"索引超出范围 (0-{len(results)-1})", code=400)
            
            selected = results[index]
            source = selected.source
            media_id = selected.id
            media_info = {
                'title': selected.title,
                'original_title': selected.original_title,
                'media_type': selected.media_type,
                'year': selected.year,
                'source_url': selected.source_url,
            }
        except Exception as e:
            return api_response(False, f"搜索失败: {e}", code=500)
    
    else:
        return api_response(False, "缺少必要参数", code=400)
    
    # 创建请求
    success, message, request_id = await MediaRequestService.create_request(
        g.current_user.TELEGRAM_ID,
        source,
        media_id,
        media_info
    )
    
    if success:
        return api_response(True, message, {
            'request_id': request_id,
            'source': source,
            'media_id': media_id,
            'media_info': media_info,
        })
    return api_response(False, message, code=400)


@media_bp.route('/request/my', methods=['GET'])
@async_route
@require_auth
async def get_my_requests():
    """获取我的求片列表"""
    requests = await MediaRequestService.get_user_requests(g.current_user.TELEGRAM_ID)
    return api_response(True, f"共 {len(requests)} 个求片", requests)


@media_bp.route('/request/pending', methods=['GET'])
@async_route
@require_auth
async def get_pending_requests():
    """获取待处理的求片列表（需要登录）"""
    requests = await MediaRequestService.get_pending_requests()
    return api_response(True, f"共 {len(requests)} 个待处理", requests)


@media_bp.route('/request/<int:request_id>/status', methods=['PUT'])
@async_route
@require_auth
async def update_request_status(request_id: int):
    """
    更新求片状态（管理员）
    
    Request:
        {
            "status": "ACCEPTED"  // UNHANDLED, ACCEPTED, REJECTED, COMPLETED
        }
    """
    from src.db.user import Role
    
    # 检查权限
    if g.current_user.ROLE != Role.ADMIN.value:
        return api_response(False, "需要管理员权限", code=403)
    
    data = request.get_json() or {}
    status_str = data.get('status', '').upper()
    
    try:
        status = ReqStatus[status_str]
    except KeyError:
        valid_statuses = [s.name for s in ReqStatus]
        return api_response(False, f"无效状态，支持: {', '.join(valid_statuses)}", code=400)
    
    success, message = await MediaRequestService.update_request_status(request_id, status)
    return api_response(success, message)

