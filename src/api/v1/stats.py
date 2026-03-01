"""
统计 API

播放统计、排行榜等
"""
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, require_admin, api_response
from src.services.stats_service import StatsService

stats_bp = Blueprint('stats', __name__, url_prefix='/stats')


# ==================== 个人统计 ====================

@stats_bp.route('/me', methods=['GET'])
@require_auth
async def get_my_stats():
    """
    获取我的播放统计
    
    Response:
        {
            "success": true,
            "data": {
                "uid": 1,
                "username": "test",
                "total": {
                    "duration": 36000,
                    "duration_str": "10小时",
                    "play_count": 50
                },
                "today": {
                    "duration": 3600,
                    "duration_str": "1小时",
                    "play_count": 5
                }
            }
        }
    """
    stats = await StatsService.get_user_stats(g.current_user.UID)
    
    if stats:
        return api_response(True, "获取成功", stats)
    return api_response(False, "获取失败", code=500)


@stats_bp.route('/playback/my', methods=['GET'])
@require_auth
async def get_my_playback_stats():
    """
    获取我的播放统计（前端专用）
    
    Response:
        {
            "success": true,
            "data": {
                "total_plays": 100,
                "total_time": 36000,
                "favorite_genres": ["动作", "科幻"],
                "recent_items": [
                    {
                        "name": "电影名称",
                        "type": "Movie",
                        "played_at": "2024-01-01T00:00:00Z"
                    }
                ]
            }
        }
    """
    stats = await StatsService.get_playback_stats(g.current_user.UID)
    
    if stats:
        return api_response(True, "获取成功", stats)
    return api_response(False, "获取失败", code=500)


@stats_bp.route('/user/<int:uid>', methods=['GET'])
@require_auth
async def get_user_stats(uid: int):
    """获取指定用户的统计（需要登录）"""
    stats = await StatsService.get_user_stats(uid)
    
    if stats:
        return api_response(True, "获取成功", stats)
    return api_response(False, "用户不存在", code=404)


# ==================== 排行榜 ====================

@stats_bp.route('/ranking', methods=['GET'])
async def get_ranking():
    """
    获取播放排行榜
    
    Query:
        period: str - 时间范围 (all=总榜, today=日榜, week=周榜, month=月榜)
        by: str - 排序方式 (duration=时长, count=次数)
        limit: int - 返回数量（默认 10，最大 50）
    
    Response:
        {
            "success": true,
            "data": {
                "period": "today",
                "by": "duration",
                "ranking": [
                    {
                        "rank": 1,
                        "uid": 1,
                        "username": "test",
                        "value": 36000,
                        "value_str": "10小时"
                    }
                ]
            }
        }
    """
    period = request.args.get('period', 'all')
    by = request.args.get('by', 'duration')
    limit = request.args.get('limit', 10, type=int)
    
    if period not in ('all', 'today', 'week', 'month'):
        return api_response(False, "无效的 period，支持: all, today, week, month", code=400)
    
    if by not in ('duration', 'count'):
        return api_response(False, "无效的 by，支持: duration, count", code=400)
    
    limit = min(max(limit, 1), 50)
    
    ranking = await StatsService.get_ranking(period, by, limit)
    
    return api_response(True, "获取成功", {
        'period': period,
        'by': by,
        'ranking': ranking,
    })


@stats_bp.route('/ranking/media', methods=['GET'])
async def get_media_ranking():
    """
    获取媒体播放排行
    
    Query:
        period: str - 时间范围
        limit: int - 返回数量
    """
    period = request.args.get('period', 'all')
    limit = request.args.get('limit', 10, type=int)
    
    limit = min(max(limit, 1), 50)
    
    ranking = await StatsService.get_media_ranking(period, limit)
    
    return api_response(True, "获取成功", {
        'period': period,
        'ranking': ranking,
    })


@stats_bp.route('/ranking/daily', methods=['GET'])
async def get_daily_ranking():
    """
    获取日榜（按日期）
    
    Query:
        date: str - 日期 YYYY-MM-DD（默认今天）
        limit: int - 返回数量
    """
    from datetime import datetime
    from src.db.playback import DailyStatsOperate
    
    date = request.args.get('date')
    limit = request.args.get('limit', 10, type=int)
    
    if not date:
        date = datetime.now().strftime('%Y-%m-%d')
    
    limit = min(max(limit, 1), 50)
    
    ranking = await DailyStatsOperate.get_daily_ranking(date, limit)
    
    # 填充用户信息
    from src.db.user import UserOperate
    from src.core.utils import format_duration
    
    results = []
    for i, item in enumerate(ranking, 1):
        user = await UserOperate.get_user_by_uid(item['uid'])
        results.append({
            'rank': i,
            'uid': item['uid'],
            'username': user.USERNAME if user else '未知',
            'play_count': item['play_count'],
            'duration': item['duration'],
            'duration_str': format_duration(item['duration']),
        })
    
    return api_response(True, "获取成功", {
        'date': date,
        'ranking': results,
    })

