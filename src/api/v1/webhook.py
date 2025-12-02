"""
Webhook API

接收 Emby/Jellyfin 的 Webhook 事件
"""
from flask import Blueprint, request, g

from src.api.v1.auth import async_route, require_auth, require_admin, api_response
from src.services.webhook import WebhookService, WebhookPushService
from src.core.utils import timestamp

webhook_bp = Blueprint('webhook', __name__, url_prefix='/webhook')


# ==================== 接收 Webhook ====================

@webhook_bp.route('/emby', methods=['POST'])
@async_route
async def receive_emby_webhook():
    """
    接收 Emby Webhook
    
    Emby 设置方法:
    1. 进入 Emby 管理面板 -> 通知
    2. 添加 Webhook 通知
    3. URL: https://your-domain/api/v1/webhook/emby
    4. 选择需要通知的事件
    """
    # 验证签名（如果设置了密钥）
    signature = request.headers.get('X-Emby-Signature', '')
    if WebhookService._secret and not WebhookService.verify_signature(request.data, signature):
        return api_response(False, "签名验证失败", code=401)
    
    data = request.get_json() or {}
    
    if not data:
        return api_response(False, "无效的请求数据", code=400)
    
    try:
        result = await WebhookService.process(data, source='emby')
        return api_response(True, "处理成功", result)
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


@webhook_bp.route('/jellyfin', methods=['POST'])
@async_route
async def receive_jellyfin_webhook():
    """
    接收 Jellyfin Webhook
    
    需要安装 Jellyfin Webhook 插件
    """
    signature = request.headers.get('X-Jellyfin-Signature', '')
    if WebhookService._secret and not WebhookService.verify_signature(request.data, signature):
        return api_response(False, "签名验证失败", code=401)
    
    data = request.get_json() or {}
    
    if not data:
        return api_response(False, "无效的请求数据", code=400)
    
    try:
        result = await WebhookService.process(data, source='jellyfin')
        return api_response(True, "处理成功", result)
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


@webhook_bp.route('/custom', methods=['POST'])
@async_route
async def receive_custom_webhook():
    """
    接收自定义 Webhook
    
    Request:
        {
            "event": "custom.event",
            "user_id": "xxx",
            "data": { ... }
        }
    """
    data = request.get_json() or {}
    
    if not data.get('event'):
        return api_response(False, "缺少 event 字段", code=400)
    
    try:
        result = await WebhookService.process(data, source='custom')
        return api_response(True, "处理成功", result)
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


# ==================== Webhook 推送管理 ====================

@webhook_bp.route('/endpoints', methods=['GET'])
@async_route
@require_auth
@require_admin
async def list_endpoints():
    """获取已注册的推送端点"""
    endpoints = [{
        'url': e['url'],
        'events': e['events'],
        'has_secret': bool(e['secret']),
    } for e in WebhookPushService._endpoints]
    
    return api_response(True, "获取成功", endpoints)


@webhook_bp.route('/endpoints', methods=['POST'])
@async_route
@require_auth
@require_admin
async def add_endpoint():
    """
    添加推送端点
    
    Request:
        {
            "url": "https://example.com/webhook",
            "events": ["playback.start", "playback.stop"],  // 可选，空=全部
            "secret": "your-secret"  // 可选
        }
    """
    data = request.get_json() or {}
    
    url = data.get('url')
    if not url:
        return api_response(False, "缺少 url", code=400)
    
    events = data.get('events', ['*'])
    secret = data.get('secret')
    
    WebhookPushService.add_endpoint(url, events, secret)
    
    return api_response(True, "添加成功", {
        'url': url,
        'events': events,
    })


@webhook_bp.route('/endpoints', methods=['DELETE'])
@async_route
@require_auth
@require_admin
async def remove_endpoint():
    """
    移除推送端点
    
    Request:
        {
            "url": "https://example.com/webhook"
        }
    """
    data = request.get_json() or {}
    url = data.get('url')
    
    if not url:
        return api_response(False, "缺少 url", code=400)
    
    WebhookPushService.remove_endpoint(url)
    return api_response(True, "移除成功")


@webhook_bp.route('/test', methods=['POST'])
@async_route
@require_auth
@require_admin
async def test_webhook():
    """
    测试 Webhook 推送
    
    Request:
        {
            "event": "test",
            "data": { ... }
        }
    """
    data = request.get_json() or {}
    event = data.get('event', 'test')
    payload = data.get('data', {'message': 'This is a test webhook'})
    
    count = await WebhookPushService.push(event, payload)
    
    return api_response(True, f"已推送到 {count} 个端点", {
        'sent_count': count,
        'total_endpoints': len(WebhookPushService._endpoints),
    })


# ==================== Webhook 配置 ====================

@webhook_bp.route('/config', methods=['GET'])
@async_route
@require_auth
@require_admin
async def get_webhook_config():
    """获取 Webhook 配置"""
    return api_response(True, "获取成功", {
        'receive_urls': {
            'emby': '/api/v1/webhook/emby',
            'jellyfin': '/api/v1/webhook/jellyfin',
            'custom': '/api/v1/webhook/custom',
        },
        'has_secret': bool(WebhookService._secret),
        'endpoints_count': len(WebhookPushService._endpoints),
        'supported_events': [
            'playback.start',
            'playback.stop',
            'playback.pause',
            'playback.unpause',
            'playback.progress',
            'user.created',
            'user.deleted',
            'user.authenticated',
            'item.added',
            'item.removed',
            'library.scan_complete',
        ],
    })


@webhook_bp.route('/config/secret', methods=['POST'])
@async_route
@require_auth
@require_admin
async def set_webhook_secret():
    """
    设置 Webhook 验证密钥
    
    Request:
        {
            "secret": "your-secret"
        }
    """
    data = request.get_json() or {}
    secret = data.get('secret')
    
    if not secret:
        return api_response(False, "缺少 secret", code=400)
    
    WebhookService.set_secret(secret)
    return api_response(True, "密钥已设置")

