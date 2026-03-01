"""
Webhook API

接收 Emby/Jellyfin 的 Webhook 事件
支持 Bangumi 同步功能
"""
from flask import Blueprint, request, g

from src.api.v1.auth import require_auth, require_admin, api_response
from src.services.webhook import WebhookService, WebhookPushService
from src.services.bangumi_sync import BangumiSyncService, SyncRequest
from src.core.utils import timestamp

webhook_bp = Blueprint('webhook', __name__, url_prefix='/webhook')


# ==================== 接收 Webhook ====================

@webhook_bp.route('/emby', methods=['POST'])
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


# ==================== Bangumi 同步 ====================

@webhook_bp.route('/bangumi/emby', methods=['POST'])
async def bangumi_emby_webhook():
    """
    Bangumi 同步 - Emby Webhook
    
    Emby 通知配置:
    1. 进入 Emby 管理面板 -> 应用程序设置 -> 通知
    2. 添加 Webhook 通知
    3. URL: https://your-domain/api/v1/webhook/bangumi/emby
    4. 请求内容类型: application/json
    5. Events: 勾选「播放-停止」和「用户-标记为已播放」
    """
    data = request.get_json() or {}
    
    if not data:
        return api_response(False, "无效的请求数据", code=400)
    
    # 检查事件类型
    event = data.get('Event', data.get('NotificationType', ''))
    if event.lower() not in ('playbackstop', 'markedplayed'):
        return api_response(True, "忽略非播放事件")
    
    try:
        result = await BangumiSyncService.process_webhook(data, source='emby')
        return api_response(result.success, result.message, {
            'subject_id': result.subject_id,
            'subject_name': result.subject_name,
            'episode': result.episode,
        })
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


@webhook_bp.route('/bangumi/jellyfin', methods=['POST'])
async def bangumi_jellyfin_webhook():
    """
    Bangumi 同步 - Jellyfin Webhook
    
    需要安装 Jellyfin Webhook 插件
    
    配置方法:
    1. 插件 -> 目录 -> Webhook -> 安装
    2. 插件 -> 我的插件 -> Webhook
    3. Server Url 填写 Jellyfin 地址
    4. Add Generic Destination
    5. Webhook Url: https://your-domain/api/v1/webhook/bangumi/jellyfin
    6. Notification Type: 只选 Playback Stop
    7. Item Type: 只选 Episodes
    8. Template:
       {"media_type": "{{{ItemType}}}","title": "{{{SeriesName}}}",
        "ori_title": " ","season": {{{SeasonNumber}}},
        "episode": {{{EpisodeNumber}}},"release_date": "{{{Year}}}-01-01",
        "user_name": "{{{NotificationUsername}}}"}
    """
    data = request.get_json() or {}
    
    if not data:
        return api_response(False, "无效的请求数据", code=400)
    
    # 检查是否播放完成
    if data.get('PlayedToCompletion') == 'False':
        return api_response(True, "播放未完成，跳过同步")
    
    try:
        result = await BangumiSyncService.process_webhook(data, source='jellyfin')
        return api_response(result.success, result.message, {
            'subject_id': result.subject_id,
            'subject_name': result.subject_name,
            'episode': result.episode,
        })
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


@webhook_bp.route('/bangumi/plex', methods=['POST'])
async def bangumi_plex_webhook():
    """
    Bangumi 同步 - Plex Webhook
    
    需要 Plex Pass 订阅
    
    配置方法:
    1. Plex 设置 -> Webhooks -> 添加 Webhook
    2. URL: https://your-domain/api/v1/webhook/bangumi/plex
    """
    # Plex 发送 multipart/form-data
    if request.content_type and 'multipart' in request.content_type:
        import json
        payload = request.form.get('payload')
        if payload:
            data = json.loads(payload)
        else:
            return api_response(False, "无效的请求数据", code=400)
    else:
        data = request.get_json() or {}
    
    if not data:
        return api_response(False, "无效的请求数据", code=400)
    
    # 检查事件类型
    event = data.get('event', '')
    if event != 'media.scrobble':
        return api_response(True, "忽略非 scrobble 事件")
    
    try:
        result = await BangumiSyncService.process_webhook(data, source='plex')
        return api_response(result.success, result.message, {
            'subject_id': result.subject_id,
            'subject_name': result.subject_name,
            'episode': result.episode,
        })
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


@webhook_bp.route('/bangumi/custom', methods=['POST'])
async def bangumi_custom_webhook():
    """
    Bangumi 同步 - 自定义 Webhook
    
    Request:
        {
            "media_type": "episode",
            "title": "我心里危险的东西",
            "ori_title": "僕の心のヤバイやつ",
            "season": 2,
            "episode": 12,
            "release_date": "2023-04-01",
            "user_name": "YourUsername"
        }
    """
    data = request.get_json() or {}
    
    if not data.get('title') or not data.get('user_name'):
        return api_response(False, "缺少必要字段 (title, user_name)", code=400)
    
    try:
        result = await BangumiSyncService.process_webhook(data, source='custom')
        return api_response(result.success, result.message, {
            'subject_id': result.subject_id,
            'subject_name': result.subject_name,
            'episode': result.episode,
        })
    except Exception as e:
        return api_response(False, f"处理失败: {e}", code=500)


# ==================== Bangumi 映射管理 ====================

@webhook_bp.route('/bangumi/mappings', methods=['GET'])
@require_auth
@require_admin
async def get_bangumi_mappings():
    """获取自定义番剧映射"""
    mappings = BangumiSyncService.get_custom_mappings()
    return api_response(True, "获取成功", {
        'count': len(mappings),
        'mappings': mappings,
    })


@webhook_bp.route('/bangumi/mappings', methods=['POST'])
@require_auth
@require_admin
async def add_bangumi_mapping():
    """
    添加自定义番剧映射
    
    Request:
        {
            "title": "番剧名称",
            "subject_id": 12345
        }
    """
    data = request.get_json() or {}
    title = data.get('title')
    subject_id = data.get('subject_id')
    
    if not title or not subject_id:
        return api_response(False, "缺少 title 或 subject_id", code=400)
    
    BangumiSyncService.add_custom_mapping(title, int(subject_id))
    return api_response(True, "添加成功", {
        'title': title,
        'subject_id': subject_id,
    })


@webhook_bp.route('/bangumi/mappings', methods=['DELETE'])
@require_auth
@require_admin
async def remove_bangumi_mapping():
    """
    移除自定义番剧映射
    
    Request:
        {
            "title": "番剧名称"
        }
    """
    data = request.get_json() or {}
    title = data.get('title')
    
    if not title:
        return api_response(False, "缺少 title", code=400)
    
    if BangumiSyncService.remove_custom_mapping(title):
        return api_response(True, "移除成功")
    return api_response(False, "映射不存在", code=404)


@webhook_bp.route('/bangumi/mappings/import', methods=['POST'])
@require_auth
@require_admin
async def import_bangumi_mappings():
    """
    批量导入映射
    
    Request:
        {
            "mappings": {
                "番剧名称1": 12345,
                "番剧名称2": 67890
            }
        }
    """
    data = request.get_json() or {}
    mappings = data.get('mappings', {})
    
    if not mappings:
        return api_response(False, "缺少 mappings", code=400)
    
    import json
    count = BangumiSyncService.load_mappings_from_json(json.dumps(mappings))
    return api_response(True, f"已导入 {count} 条映射", {'count': count})


@webhook_bp.route('/bangumi/mappings/export', methods=['GET'])
@require_auth
@require_admin
async def export_bangumi_mappings():
    """导出所有映射为 JSON"""
    import json
    mappings = BangumiSyncService.get_custom_mappings()
    return api_response(True, "导出成功", {
        'json': json.dumps(mappings, ensure_ascii=False, indent=2),
        'mappings': mappings,
    })


@webhook_bp.route('/bangumi/sync', methods=['POST'])
@require_auth
async def manual_bangumi_sync():
    """
    手动同步单集到 Bangumi
    
    Request:
        {
            "title": "番剧名称",
            "season": 1,
            "episode": 12,
            "original_title": "原名",  // 可选
            "release_date": "2023-01-01"  // 可选
        }
    """
    data = request.get_json() or {}
    
    title = data.get('title')
    season = data.get('season', 1)
    episode = data.get('episode')
    
    if not title or not episode:
        return api_response(False, "缺少 title 或 episode", code=400)
    
    result = await BangumiSyncService.sync_for_user(
        uid=g.current_user.UID,
        title=title,
        season=int(season),
        episode=int(episode),
        original_title=data.get('original_title', ''),
        release_date=data.get('release_date', '')
    )
    
    return api_response(result.success, result.message, {
        'subject_id': result.subject_id,
        'subject_name': result.subject_name,
        'episode': result.episode,
    })


@webhook_bp.route('/bangumi/config', methods=['GET'])
@require_auth
@require_admin
async def get_bangumi_sync_config():
    """获取 Bangumi 同步配置"""
    return api_response(True, "获取成功", {
        'receive_urls': {
            'emby': '/api/v1/webhook/bangumi/emby',
            'jellyfin': '/api/v1/webhook/bangumi/jellyfin',
            'plex': '/api/v1/webhook/bangumi/plex',
            'custom': '/api/v1/webhook/bangumi/custom',
        },
        'mappings_count': len(BangumiSyncService.get_custom_mappings()),
        'emby_template': '''在 Emby 通知设置中:
1. 添加 Webhook 通知
2. URL: {host}/api/v1/webhook/bangumi/emby
3. 请求内容类型: application/json
4. Events: 勾选「播放-停止」''',
        'jellyfin_template': '''{"media_type": "{{{ItemType}}}","title": "{{{SeriesName}}}","ori_title": " ","season": {{{SeasonNumber}}},"episode": {{{EpisodeNumber}}},"release_date": "{{{Year}}}-01-01","user_name": "{{{NotificationUsername}}}","NotificationType": "{{{NotificationType}}}","PlayedToCompletion": "{{{PlayedToCompletion}}}"}''',
    })

