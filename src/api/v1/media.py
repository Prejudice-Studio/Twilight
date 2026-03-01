"""
媒体搜索 API

提供 TMDB 和 Bangumi 统一搜索接口
支持库存检查和求片功能
"""
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, api_response
from src.services import MediaService, MediaRequestService, MediaSource, InventoryService
from src.db.bangumi import ReqStatus

media_bp = Blueprint('media', __name__, url_prefix='/media')


# ==================== 媒体搜索 ====================

@media_bp.route('/search', methods=['GET'])
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
    year = request.args.get('year', type=int)
    bgm_type = request.args.get('type', type=int)  # Bangumi 类型过滤 (2=动画, 6=三次元)
    
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
        results = await MediaService.search(query, media_source, limit, year=year, bgm_type=bgm_type)
        
        return api_response(True, f"找到 {len(results)} 个结果", {
            'query': query,
            'source': source,
            'count': len(results),
            'results': [r.to_dict() for r in results],
        })
    except Exception as e:
        return api_response(False, f"搜索失败: {e}", code=500)


@media_bp.route('/search/tmdb', methods=['GET'])
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


@media_bp.route('/search/id/<string:source_type>/<int:media_id>', methods=['GET'])
@require_auth
async def search_media_by_id(source_type: str, media_id: int):
    """
    通过来源和 ID 直接获取媒体详情（快捷接口）
    
    URL Parameters:
        source_type: str - 来源类型 (tmdb/bangumi/bgm)
        media_id: int - 媒体 ID
    
    Query Parameters:
        type: str - 媒体类型 (仅 TMDB: movie/tv，默认 movie)
        include_details: bool - 是否包含详细信息（默认 true）
    
    Examples:
        GET /api/v1/media/search/id/tmdb/123?type=movie
        GET /api/v1/media/search/id/tmdb/456?type=tv
        GET /api/v1/media/search/id/bangumi/789
        GET /api/v1/media/search/id/bgm/789
    
    Response:
        {
            "success": true,
            "message": "获取成功",
            "data": {
                "id": 123,
                "title": "电影名称",
                "original_title": "Original Title",
                "media_type": "movie",
                "overview": "简介...",
                "release_date": "2023-01-01",
                "year": "2023",
                "poster": "https://...",
                "poster_url": "https://...",
                "backdrop_url": "https://...",
                "vote_average": 8.5,
                "rating": 8.5,
                "vote_count": 1000,
                "source": "tmdb",
                "source_url": "https://...",
                "genres": ["动作", "科幻"],
                "cast": [...],
                "runtime": 120,
                ...
            }
        }
    """
    source_type = source_type.lower()
    
    if source_type not in ('tmdb', 'bangumi', 'bgm'):
        return api_response(False, "无效的来源类型，支持: tmdb, bangumi, bgm", code=400)
    
    if source_type == 'bgm':
        source_type = 'bangumi'
    
    media_type = request.args.get('type', 'movie')
    include_details = request.args.get('include_details', 'true').lower() == 'true'
    
    try:
        result = await MediaService.get_by_source_id(
            source=source_type,
            media_id=media_id,
            media_type=media_type,
            include_details=include_details
        )
        
        if result:
            return api_response(True, "获取成功", result.to_dict())
        return api_response(False, "媒体不存在", code=404)
    
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"获取媒体详情失败 (source={source_type}, id={media_id}): {e}", exc_info=True)
        return api_response(False, f"获取失败: {e}", code=500)


@media_bp.route('/detail', methods=['GET'])
@require_auth
async def get_media_detail():
    """
    获取媒体详情
    
    Query:
        source: str - 来源 (tmdb/bangumi)
        media_id: int - 媒体 ID（或使用 id）
        media_type: str - 类型 (tmdb: movie/tv，可选，默认 movie)
        include_details: bool - 是否包含详细信息（演员、类型等，默认 true）
    
    Examples:
        GET /api/v1/media/detail?source=tmdb&media_id=123&media_type=movie
        GET /api/v1/media/detail?source=tmdb&id=456&type=tv
        GET /api/v1/media/detail?source=bangumi&media_id=789
    """
    source = request.args.get('source', '').lower()
    media_id = request.args.get('media_id', type=int) or request.args.get('id', type=int)
    media_type = request.args.get('media_type', '') or request.args.get('type', 'movie')
    include_details = request.args.get('include_details', 'true').lower() == 'true'
    
    if not source or not media_id:
        return api_response(False, "缺少必要参数 (source, media_id)", code=400)
    
    if source not in ('tmdb', 'bangumi', 'bgm'):
        return api_response(False, "无效的来源，支持: tmdb, bangumi", code=400)
    
    if source == 'bgm':
        source = 'bangumi'
    
    try:
        result = await MediaService.get_by_source_id(source, media_id, media_type, include_details)
        
        if result:
            return api_response(True, "获取成功", result.to_dict())
        return api_response(False, "媒体不存在", code=404)
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"获取媒体详情失败: {e}", exc_info=True)
        return api_response(False, f"获取失败: {e}", code=500)


@media_bp.route('/tmdb/<int:tmdb_id>', methods=['GET'])
@require_auth
async def get_tmdb_detail(tmdb_id: int):
    """
    通过 TMDB ID 获取媒体详情（快捷接口）
    
    Query:
        type: str - 媒体类型 (movie/tv，默认 movie)
        include_details: bool - 是否包含详细信息（默认 true）
    
    Examples:
        GET /api/v1/media/tmdb/123?type=movie
        GET /api/v1/media/tmdb/456?type=tv
    """
    media_type = request.args.get('type', 'movie')
    include_details = request.args.get('include_details', 'true').lower() == 'true'
    
    try:
        result = await MediaService.get_by_source_id('tmdb', tmdb_id, media_type, include_details)
        
        if result:
            return api_response(True, "获取成功", result.to_dict())
        return api_response(False, "媒体不存在", code=404)
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"获取 TMDB 详情失败: {e}", exc_info=True)
        return api_response(False, f"获取失败: {e}", code=500)


@media_bp.route('/bangumi/<int:bgm_id>', methods=['GET'])
@require_auth
async def get_bangumi_detail(bgm_id: int):
    """
    通过 Bangumi ID 获取媒体详情（快捷接口）
    
    Query:
        include_details: bool - 是否包含详细信息（默认 true）
    
    Examples:
        GET /api/v1/media/bangumi/123
        GET /api/v1/media/bangumi/456?include_details=false
    """
    include_details = request.args.get('include_details', 'true').lower() == 'true'
    
    try:
        result = await MediaService.get_by_source_id('bangumi', bgm_id, None, include_details)
        
        if result:
            return api_response(True, "获取成功", result.to_dict())
        return api_response(False, "媒体不存在", code=404)
    except Exception as e:
        import logging
        logger = logging.getLogger(__name__)
        logger.error(f"获取 Bangumi 详情失败: {e}", exc_info=True)
        return api_response(False, f"获取失败: {e}", code=500)


# ==================== 库存检查 ====================

@media_bp.route('/inventory/check', methods=['POST'])
async def check_inventory():
    """
    检查媒体库存（支持季度检查）
    
    Request:
        {
            "source": "tmdb",           // tmdb 或 bangumi
            "media_id": 123,            // 媒体 ID
            "media_type": "tv",         // 可选，tmdb 的类型
            "season": 1                 // 可选，要检查的季度
        }
    
    或者直接通过标题检查：
        {
            "title": "进击的巨人",
            "year": 2013,               // 可选
            "original_title": "進撃の巨人",  // 可选
            "media_type": "tv",         // 可选
            "season": 4                 // 可选，要检查的季度
        }
    
    Response (库中已有该季度):
        {
            "success": true,
            "data": {
                "exists": true,
                "message": "库中已有：进击的巨人 第 1 季",
                "seasons_available": [1, 2, 3],
                "season_requested": 1
            }
        }
    
    Response (库中有剧集但缺少该季度):
        {
            "success": true,
            "data": {
                "exists": false,
                "message": "库中有 进击的巨人，但缺少第 4 季\\n已有季度：1, 2, 3",
                "seasons_available": [1, 2, 3],
                "season_requested": 4
            }
        }
    """
    data = request.get_json() or {}
    
    source = data.get('source')
    media_id = data.get('media_id')
    title = data.get('title')
    season = data.get('season')
    
    # 转换 season 为整数
    if season is not None:
        try:
            season = int(season)
        except (ValueError, TypeError):
            season = None
    
    if source and media_id:
        # 通过来源和 ID 检查
        media_type = data.get('media_type', 'movie')
        
        # 获取媒体信息
        result = await MediaService.get_by_source_id(source, media_id, media_type)
        if not result:
            return api_response(False, "媒体不存在", code=404)
        
        media_info = {
            'id': result.id,
            'title': result.title,
            'original_title': result.original_title,
            'media_type': result.media_type,
            'release_date': result.release_date,
        }
        
        inventory_result = await InventoryService.check_media(media_info, source, season)
        
    elif title:
        # 通过标题直接检查
        year = data.get('year')
        original_title = data.get('original_title')
        media_type = data.get('media_type')
        
        inventory_result = await InventoryService.check_by_title(
            title=title,
            year=year,
            original_title=original_title,
            media_type=media_type,
            season=season
        )
    
    else:
        return api_response(False, "缺少必要参数 (source+media_id 或 title)", code=400)
    
    return api_response(True, inventory_result.message, inventory_result.to_dict())


@media_bp.route('/inventory/search', methods=['GET'])
async def search_inventory():
    """
    搜索库存
    
    Query:
        q: str - 搜索关键词
        type: str - 类型过滤 (Movie/Series/Episode)
        year: int - 年份过滤
        limit: int - 返回数量
    """
    from src.services.emby import get_emby_client, EmbyError
    
    query = request.args.get('q', '').strip()
    item_type = request.args.get('type')
    year = request.args.get('year', type=int)
    limit = request.args.get('limit', 20, type=int)
    
    if not query:
        return api_response(False, "缺少搜索关键词", code=400)
    
    limit = min(max(limit, 1), 50)
    
    try:
        emby = get_emby_client()
        include_types = [item_type] if item_type else ['Movie', 'Series']
        
        items = await emby.search_media(
            search_term=query,
            include_types=include_types,
            year=year,
            limit=limit
        )
        
        return api_response(True, f"找到 {len(items)} 个结果", {
            'query': query,
            'count': len(items),
            'results': [item.to_dict() for item in items],
        })
        
    except EmbyError as e:
        return api_response(False, f"搜索失败: {e}", code=500)


# ==================== 求片功能 ====================

@media_bp.route('/request', methods=['POST'])
@require_auth
async def create_media_request():
    """
    创建求片请求（会自动检查库存，支持季度）
    
    Request:
        {
            "source": "tmdb",           // tmdb 或 bangumi
            "media_id": 123,            // 媒体 ID
            "media_type": "tv",         // 可选，tmdb 的类型
            "season": 4,                // 可选，季度（剧集需要）
            "title": "剧集名称",         // 可选，用于记录
            "note": "备注信息",          // 可选
            "skip_inventory_check": false  // 可选，是否跳过库存检查
        }
    
    或者直接搜索后选择：
        {
            "query": "进击的巨人",       // 搜索关键词
            "index": 0,                 // 选择搜索结果的索引
            "season": 4                 // 可选，季度
        }
    
    Response (库中已有该季度):
        {
            "success": false,
            "message": "📦 库中已有：进击的巨人 第 4 季\\n无需再次请求，请在媒体库中搜索观看。"
        }
    
    Response (库中有剧集但缺少该季度，可以请求):
        {
            "success": true,
            "message": "✅ 求片请求 第 4 季已提交，请等待管理员处理",
            "data": {
                "request_id": 123,
                "season": 4,
                "inventory_checked": true
            }
        }
    """
    data = request.get_json() or {}
    
    # 方式1: 直接指定
    source = data.get('source')
    media_id = data.get('media_id')
    skip_inventory_check = data.get('skip_inventory_check', False)
    season = data.get('season')
    year = data.get('year')  # 年份限制
    
    # 转换 season 为整数
    if season is not None:
        try:
            season = int(season)
        except (ValueError, TypeError):
            season = None
    
    # 转换 year 为整数
    if year is not None:
        try:
            year = int(year)
        except (ValueError, TypeError):
            year = None
    
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
            media_info = result.to_dict()
        
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
            media_info = selected.to_dict()
        except Exception as e:
            return api_response(False, f"搜索失败: {e}", code=500)
    
    else:
        return api_response(False, "缺少必要参数", code=400)
    
    # 创建请求（包含库存检查）
    success, message, request_id = await MediaRequestService.create_request(
        g.current_user.TELEGRAM_ID,
        source,
        media_id,
        media_info,
        skip_inventory_check=skip_inventory_check,
        season=season
    )
    
    if success:
        return api_response(True, message, {
            'request_id': request_id,
            'source': source,
            'media_id': media_id,
            'season': season,
            'media_info': media_info,
            'inventory_checked': not skip_inventory_check,
        })
    return api_response(False, message, code=400)


@media_bp.route('/request/my', methods=['GET'])
@require_auth
async def get_my_requests():
    """获取我的求片列表"""
    requests = await MediaRequestService.get_user_requests(g.current_user.TELEGRAM_ID)
    return api_response(True, f"共 {len(requests)} 个求片", requests)


@media_bp.route('/request/pending', methods=['GET'])
@require_auth
async def get_pending_requests():
    """获取待处理的求片列表（需要登录）"""
    requests = await MediaRequestService.get_pending_requests()
    return api_response(True, f"共 {len(requests)} 个待处理", requests)


@media_bp.route('/request/<int:request_id>/status', methods=['PUT'])
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


@media_bp.route('/request/external/update', methods=['POST'])
async def external_update_request():
    """
    外部更新求片状态 (无需登录，凭 require_key 访问)
    
    Request:
        {
            "key": "...",            // 必填，求片请求的 require_key
            "status": "COMPLETED",   // 必填，新状态 (UNHANDLED, ACCEPTED, REJECTED, COMPLETED, DOWNLOADING)
            "note": "备注信息"        // 可选，管理员/系统备注
        }
    """
    data = request.get_json() or {}
    require_key = data.get('key')
    status_name = data.get('status')
    note = data.get('note', '')
    
    if not require_key or not status_name:
        return api_response(False, "缺少必要参数 (key, status)", code=400)
    
    success, message = await MediaRequestService.update_request_by_key(require_key, status_name, note)
    if success:
        return api_response(True, message)
    return api_response(False, message, code=400)


@media_bp.route('/request/<int:request_id>', methods=['GET', 'DELETE'])
@require_auth
async def handle_request_item(request_id: int):
    """获取或删除求片请求"""
    from src.db.bangumi import BangumiRequireOperate
    from src.db.user import Role
    from flask import request as flask_request
    
    # 尝试寻找请求
    req = await BangumiRequireOperate.get_require(request_id)
    if not req:
        return api_response(False, "请求不存在", code=404)
        
    if flask_request.method == 'DELETE':
        # 权限检查：要么是本人，要么是管理员
        if req.telegram_id != g.current_user.TELEGRAM_ID and g.current_user.ROLE != Role.ADMIN.value:
            return api_response(False, "无权删除他人的请求", code=403)
            
        # 执行删除
        source = 'bangumi' if hasattr(req, 'bangumi_id') else 'tmdb'
        success = await BangumiRequireOperate.delete_require(request_id, source)
        
        if success:
            return api_response(True, "请求已删除")
        return api_response(False, "删除失败")
    
    # GET 请求：返回详情
    # 这里可以复用 get_user_requests 的逻辑，但针对单条
    media_info = None
    if req.other_info:
        try:
            import json
            media_info = json.loads(req.other_info)
        except:
            pass
            
    user = await UserOperate.get_user_by_telegram_id(req.telegram_id)
    
    res = {
        'id': req.id,
        'media_id': getattr(req, 'bangumi_id', getattr(req, 'tmdb_id', None)),
        'source': 'bangumi' if hasattr(req, 'bangumi_id') else 'tmdb',
        'status': ReqStatus(req.status).name,
        'timestamp': req.timestamp,
        'title': req.title,
        'season': req.season,
        'media_type': req.media_type,
        'require_key': req.require_key,
        'admin_note': req.admin_note,
        'media_info': media_info,
        'user': {
            'telegram_id': req.telegram_id,
            'username': user.USERNAME if user else None,
        } if user else None,
    }
    return api_response(True, "获取成功", res)

