"""
签到与积分 API

仅装饰用途，无排行榜；货币名/每日范围/连签加成均由配置驱动。
"""
import logging

from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, api_response
from src.services.signin_service import SigninService


logger = logging.getLogger(__name__)

signin_bp = Blueprint('signin', __name__, url_prefix='/signin')


@signin_bp.route('/config', methods=['GET'])
async def get_public_config():
    """获取签到系统的公开规则（无需登录）。"""
    return api_response(True, "获取成功", SigninService.public_config())


@signin_bp.route('/me', methods=['GET'])
@require_auth
async def get_my_summary():
    """获取当前用户的签到/积分摘要。"""
    summary = await SigninService.get_summary(g.current_user)
    return api_response(True, "获取成功", {
        'enabled': summary.enabled,
        'currency_name': summary.currency_name,
        'current_points': summary.current_points,
        'current_streak': summary.current_streak,
        'longest_streak': summary.longest_streak,
        'total_points': summary.total_points,
        'last_signin_date': summary.last_signin_date,
        'today_signed': summary.today_signed,
        'next_bonus_in_days': summary.next_bonus_in_days,
        'next_bonus_points': summary.next_bonus_points,
    })


@signin_bp.route('', methods=['POST'])
@require_auth
async def do_signin():
    """触发签到。"""
    result = await SigninService.signin(g.current_user)
    payload = {
        'today_signed': result.today_signed,
        'daily_points': result.daily_points,
        'bonus_points': result.bonus_points,
        'total_today': (result.daily_points or 0) + (result.bonus_points or 0),
        'current_streak': result.current_streak,
        'longest_streak': result.longest_streak,
        'current_points': result.current_points,
        'currency_name': result.currency_name,
    }
    if not result.success:
        # 今日已签到也走 400，便于前端拦截
        return api_response(False, result.message, payload, code=400)
    return api_response(True, result.message, payload)


@signin_bp.route('/history', methods=['GET'])
@require_auth
async def list_history():
    """获取签到历史。"""
    try:
        limit = int(request.args.get('limit') or 30)
    except (TypeError, ValueError):
        limit = 30
    records = await SigninService.list_history(g.current_user, limit=limit)
    return api_response(True, "获取成功", {
        'records': records,
        'currency_name': SigninService.currency_name(),
    })
