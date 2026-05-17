"""
全站公告板 API

- 公开接口：`GET /announcements` 列出对终端用户可见的公告（含登录页面、未登录用户）
- 管理接口：`/admin/announcements` 在 admin.py 中提供完整 CRUD
"""
import logging
from flask import Blueprint, request

from src.api.v1.auth import api_response
from src.db.announcement import AnnouncementOperate, AnnouncementModel

logger = logging.getLogger(__name__)

announcements_bp = Blueprint('announcements', __name__, url_prefix='/announcements')


def serialize_announcement(item: AnnouncementModel, include_internal: bool = False) -> dict:
    data = {
        'id': item.ID,
        'title': item.TITLE,
        'content': item.CONTENT,
        'level': item.LEVEL,
        'render_mode': getattr(item, 'RENDER_MODE', None) or 'plain',
        'pinned': bool(item.PINNED),
        'visible': bool(item.VISIBLE),
        'expires_at': item.EXPIRES_AT,
        'created_at': item.CREATED_AT,
        'updated_at': item.UPDATED_AT,
    }
    if include_internal:
        data['created_by_uid'] = item.CREATED_BY_UID
    return data


@announcements_bp.route('', methods=['GET'])
async def list_active_announcements():
    """获取对终端用户可见的公告列表（公开接口，无需认证）。

    Query:
        limit: 最多返回多少条，默认 50，上限 200
    """
    try:
        limit = int(request.args.get('limit', 50))
    except (TypeError, ValueError):
        limit = 50

    items = await AnnouncementOperate.list_active(limit=limit)
    return api_response(True, "获取成功", {
        'announcements': [serialize_announcement(it) for it in items],
        'total': len(items),
    })
