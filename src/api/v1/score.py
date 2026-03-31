"""
积分 API

提供积分相关操作
"""
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, api_response
from src.services import ScoreService, RedPacketService, RedPacketType
from src.db.score import ScoreOperate
from src.db.user import UserOperate
from src.config import ScoreAndRegisterConfig

score_bp = Blueprint('score', __name__, url_prefix='/score')


# ==================== 积分操作 ====================

@score_bp.route('/balance', methods=['GET'])
@require_auth
async def get_balance():
    """获取我的积分余额"""
    score_record = await ScoreOperate.get_score_by_uid(g.current_user.UID)
    
    return api_response(True, "获取成功", {
        'score': score_record.SCORE if score_record else 0,
        'checkin_days': score_record.CHECKIN_COUNT if score_record else 0,
        'score_name': ScoreAndRegisterConfig.SCORE_NAME,
    })


@score_bp.route('/info', methods=['GET'])
@require_auth
async def get_score_info():
    """
    获取积分详细信息（包括签到状态）
    
    Response:
        {
            "success": true,
            "data": {
                "balance": 150,
                "score_name": "暮光币",
                "today_checkin": false,
                "checkin_streak": 7,
                "total_earned": 1000,
                "total_spent": 500
            }
        }
    """
    score_record = await ScoreOperate.get_score_by_uid(g.current_user.UID)
    
    # 检查今日是否已签到
    today_checkin = False
    if score_record and score_record.CHECKIN_TIME:
        from src.services.score_service import ScoreService
        today_start = ScoreService._get_today_start()
        today_checkin = score_record.CHECKIN_TIME >= today_start
    
    return api_response(True, "获取成功", {
        'balance': score_record.SCORE if score_record else 0,
        'score_name': ScoreAndRegisterConfig.SCORE_NAME,
        'today_checkin': today_checkin,
        'checkin_streak': score_record.CHECKIN_COUNT if score_record else 0,
        'total_earned': score_record.TOTAL_EARNED if score_record and hasattr(score_record, 'TOTAL_EARNED') else 0,
        'total_spent': score_record.TOTAL_SPENT if score_record and hasattr(score_record, 'TOTAL_SPENT') else 0,
    })


@score_bp.route('/checkin', methods=['POST'])
@require_auth
async def checkin():
    """
    签到
    
    Response:
        {
            "success": true,
            "data": {
                "score": 15,
                "balance": 150,
                "streak": 7
            }
        }
    """
    # 检查无 Emby 账户用户是否允许签到
    if not g.current_user.EMBYID and not ScoreAndRegisterConfig.ALLOW_NO_EMBY_CHECKIN:
        return api_response(False, "请先激活 Emby 账户后再签到", code=403)
    
    result_type, response = await ScoreService.checkin(g.current_user.UID)
    
    return api_response(
        result_type.value == 'success',
        response.message,
        {
            'score': response.score,
            'balance': response.balance,
            'streak': response.streak,
            'score_name': ScoreAndRegisterConfig.SCORE_NAME,
        }
    )


@score_bp.route('/history', methods=['GET'])
@require_auth
async def get_history():
    """
    获取积分变动历史
    
    Query:
        page: 页码 (默认 1)
        per_page: 每页数量 (默认 20)
    
    Response:
        {
            "success": true,
            "data": {
                "records": [...],
                "total": 100
            }
        }
    """
    from src.db.score import ScoreHistoryOperate
    
    page = request.args.get('page', 1, type=int)
    per_page = min(request.args.get('per_page', 20, type=int), 100)  # 最多 100 条
    offset = (page - 1) * per_page
    
    records = await ScoreHistoryOperate.get_history_by_uid(g.current_user.UID, per_page, offset)
    total = await ScoreHistoryOperate.get_history_count(g.current_user.UID)
    
    return api_response(True, "获取成功", {
        'records': [
            {
                'id': r.ID,
                'type': r.TYPE,
                'amount': r.AMOUNT,
                'balance_after': r.BALANCE_AFTER,
                'note': r.NOTE,
                'related_uid': r.RELATED_UID,
                'created_at': r.CREATED_AT,
            }
            for r in records
        ],
        'total': total,
        'page': page,
        'per_page': per_page,
    })


@score_bp.route('/transfer', methods=['POST'])
@require_auth
async def transfer():
    """
    积分转账
    
    Request:
        {
            "to_username": "targetuser",  // 目标用户名
            "to_uid": 123,                // 或目标用户 UID
            "amount": 50
        }
    """
    data = request.get_json() or {}
    to_username = data.get('to_username')
    to_uid = data.get('to_uid')
    amount = data.get('amount')
    
    if not amount:
        return api_response(False, "缺少转账金额", code=400)
    
    # 查找目标用户
    if to_uid:
        target_user = await UserOperate.get_user_by_uid(to_uid)
    elif to_username:
        target_user = await UserOperate.get_user_by_username(to_username)
    else:
        return api_response(False, "请提供目标用户名或 UID", code=400)
    
    if not target_user:
        return api_response(False, "目标用户不存在", code=404)
    
    success, message = await ScoreService.transfer(
        g.current_user.UID,
        target_user.UID,
        amount
    )
    
    if success:
        score_record = await ScoreOperate.get_score_by_uid(g.current_user.UID)
        return api_response(True, message, {
            'remaining_score': score_record.SCORE if score_record else 0
        })
    
    return api_response(False, message)


@score_bp.route('/ranking', methods=['GET'])
async def get_ranking():
    """
    获取积分排行榜
    
    Query:
        limit: int (默认 10，最大 50)
    """
    limit = request.args.get('limit', 10, type=int)
    limit = min(max(limit, 1), 50)
    
    ranking = await ScoreService.get_ranking(limit)
    
    return api_response(True, "获取成功", {
        'ranking': ranking,
        'score_name': ScoreAndRegisterConfig.SCORE_NAME,
    })


@score_bp.route('/config', methods=['GET'])
async def get_score_config():
    """获取积分配置信息（公开）"""
    return api_response(True, "获取成功", {
        'score_name': ScoreAndRegisterConfig.SCORE_NAME,
        'checkin': {
            'base_score': ScoreAndRegisterConfig.CHECKIN_BASE_SCORE,
            'streak_bonus': ScoreAndRegisterConfig.CHECKIN_STREAK_BONUS,
            'max_streak_bonus': ScoreAndRegisterConfig.CHECKIN_MAX_STREAK_BONUS,
            'random_range': [ScoreAndRegisterConfig.CHECKIN_RANDOM_MIN, ScoreAndRegisterConfig.CHECKIN_RANDOM_MAX],
        },
        'transfer': {
            'enabled': ScoreAndRegisterConfig.PRIVATE_TRANSFER_MODE,
            'min_amount': ScoreAndRegisterConfig.TRANSFER_MIN_AMOUNT,
            'max_amount': ScoreAndRegisterConfig.TRANSFER_MAX_AMOUNT,
            'fee_rate': ScoreAndRegisterConfig.TRANSFER_FEE_RATE,
        },
        'red_packet': {
            'enabled': ScoreAndRegisterConfig.RED_PACKET_MODE,
            'min_amount': ScoreAndRegisterConfig.RED_PACKET_MIN_AMOUNT,
            'max_amount': ScoreAndRegisterConfig.RED_PACKET_MAX_AMOUNT,
            'min_count': ScoreAndRegisterConfig.RED_PACKET_MIN_COUNT,
            'max_count': ScoreAndRegisterConfig.RED_PACKET_MAX_COUNT,
        },
        'auto_renew': {
            'enabled': ScoreAndRegisterConfig.AUTO_RENEW_ENABLED,
            'days': ScoreAndRegisterConfig.AUTO_RENEW_DAYS,
            'cost': ScoreAndRegisterConfig.AUTO_RENEW_COST,
        },
    })


# ==================== 红包操作 ====================

@score_bp.route('/redpacket', methods=['POST'])
@require_auth
async def create_redpacket():
    """
    创建红包
    
    Request:
        {
            "amount": 100,       // 总金额
            "count": 5,          // 红包个数
            "type": 1            // 1=拼手气, 2=均分
        }
    """
    if not ScoreAndRegisterConfig.RED_PACKET_MODE:
        return api_response(False, "红包功能未开启", code=403)
    
    data = request.get_json() or {}
    amount = data.get('amount')
    count = data.get('count')
    packet_type = data.get('type', 1)
    
    if not amount or not count:
        return api_response(False, "缺少必要参数", code=400)
    
    rp_type = RedPacketType.EQUAL if packet_type == 2 else RedPacketType.RANDOM
    
    success, message, rp_key = await RedPacketService.create_red_packet(
        g.current_user.UID,
        amount,
        count,
        rp_type
    )
    
    if success:
        return api_response(True, message, {
            'rp_key': rp_key,
            'amount': amount,
            'count': count,
            'type': 'equal' if packet_type == 2 else 'random',
        })
    
    return api_response(False, message)


@score_bp.route('/redpacket/<rp_key>/grab', methods=['POST'])
@require_auth
async def grab_redpacket(rp_key: str):
    """抢红包"""
    if not ScoreAndRegisterConfig.RED_PACKET_MODE:
        return api_response(False, "红包功能未开启", code=403)
    
    success, message, amount = await RedPacketService.grab_red_packet(
        rp_key,
        g.current_user.UID
    )
    
    if success:
        score_record = await ScoreOperate.get_score_by_uid(g.current_user.UID)
        return api_response(True, message, {
            'amount': amount,
            'total_score': score_record.SCORE if score_record else 0,
        })
    
    return api_response(False, message)


@score_bp.route('/redpacket/<rp_key>/withdraw', methods=['POST'])
@require_auth
async def withdraw_redpacket(rp_key: str):
    """撤回红包"""
    success, message = await RedPacketService.withdraw_red_packet(
        rp_key,
        g.current_user.UID
    )
    return api_response(success, message)

