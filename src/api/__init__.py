"""
API 模块

提供 Flask Web API
"""
from flask import Flask, jsonify

from src.api.routes import api, admin_api
from src.api.v1 import register_v1_blueprints
from src.config import Config
from src.core.utils import setup_logging, timestamp


def create_app() -> Flask:
    """创建 Flask 应用"""
    app = Flask(__name__)
    
    # 配置
    app.config['JSON_AS_ASCII'] = False  # 支持中文
    app.config['JSON_SORT_KEYS'] = False
    
    # 注册旧版 API（兼容）
    app.register_blueprint(api)
    app.register_blueprint(admin_api)
    
    # 注册 v1 API（推荐前端使用）
    register_v1_blueprints(app)
    
    # 配置日志
    if Config.LOGGING:
        setup_logging(level=Config.LOG_LEVEL)
    
    # 根路由
    @app.route('/')
    def index():
        return jsonify({
            'name': 'Twilight API',
            'version': '1.0.0',
            'api_versions': ['v1'],
            'docs': '/api/v1/docs',
        })
    
    # API 文档路由
    @app.route('/api/v1/docs')
    def api_docs():
        return jsonify({
            'auth': {
                'POST /api/v1/auth/login/telegram': '通过 Telegram ID 登录',
                'POST /api/v1/auth/login/apikey': '通过 API Key 登录/验证',
                'POST /api/v1/auth/logout': '登出当前设备',
                'POST /api/v1/auth/logout/all': '登出所有设备',
                'GET /api/v1/auth/me': '获取当前用户',
                'POST /api/v1/auth/refresh': '刷新 Token',
                'GET /api/v1/auth/apikey': '获取 API Key 状态',
                'POST /api/v1/auth/apikey': '生成新 API Key',
                'DELETE /api/v1/auth/apikey': '禁用 API Key',
                'POST /api/v1/auth/apikey/enable': '启用 API Key',
            },
            'users': {
                'POST /api/v1/users/register': '用户注册',
                'GET /api/v1/users/check-available': '检查是否可注册',
                'GET /api/v1/users/me': '获取我的信息',
                'PUT /api/v1/users/me': '更新我的信息',
                'PUT /api/v1/users/me/username': '修改用户名',
                'PUT /api/v1/users/me/password': '重置密码',
                'PUT /api/v1/users/me/nsfw': '切换 NSFW 权限',
                'POST /api/v1/users/me/renew': '使用续期码续期',
                'GET /api/v1/users/me/devices': '获取我的设备',
                'DELETE /api/v1/users/me/devices/<id>': '移除设备',
                'GET /api/v1/users/me/libraries': '获取我的媒体库',
                'GET /api/v1/users/me/sessions': '获取我的会话',
                'GET /api/v1/users/me/login-history': '获取登录历史',
            },
            'score': {
                'GET /api/v1/score/balance': '获取积分余额',
                'POST /api/v1/score/checkin': '签到',
                'POST /api/v1/score/transfer': '积分转账',
                'GET /api/v1/score/ranking': '积分排行榜',
                'POST /api/v1/score/redpacket': '创建红包',
                'POST /api/v1/score/redpacket/<key>/grab': '抢红包',
                'POST /api/v1/score/redpacket/<key>/withdraw': '撤回红包',
            },
            'emby': {
                'GET /api/v1/emby/status': '服务器状态',
                'GET /api/v1/emby/urls': '服务器地址列表',
                'GET /api/v1/emby/libraries': '媒体库列表',
                'GET /api/v1/emby/search': '搜索媒体',
                'GET /api/v1/emby/latest': '最新媒体',
                'GET /api/v1/emby/sessions/count': '会话数量',
            },
            'media': {
                'GET /api/v1/media/search': '统一搜索（TMDB+Bangumi）',
                'GET /api/v1/media/search/tmdb': '仅搜索 TMDB',
                'GET /api/v1/media/search/bangumi': '仅搜索 Bangumi',
                'GET /api/v1/media/detail': '获取媒体详情',
                'POST /api/v1/media/request': '创建求片请求',
                'GET /api/v1/media/request/my': '我的求片列表',
                'GET /api/v1/media/request/pending': '待处理求片',
                'PUT /api/v1/media/request/<id>/status': '更新求片状态',
            },
            'admin': {
                'GET /api/v1/admin/stats': '系统统计',
                'GET /api/v1/admin/users': '用户列表',
                'GET /api/v1/admin/users/<uid>': '用户详情',
                'POST /api/v1/admin/users/<uid>/disable': '禁用用户',
                'POST /api/v1/admin/users/<uid>/enable': '启用用户',
                'DELETE /api/v1/admin/users/<uid>': '删除用户',
                'POST /api/v1/admin/users/<uid>/renew': '为用户续期',
                'POST /api/v1/admin/users/<uid>/kick': '踢出会话',
                'PUT /api/v1/admin/users/<uid>/libraries': '设置媒体库',
                'PUT /api/v1/admin/users/<uid>/admin': '设置管理员',
                'PUT /api/v1/admin/users/<uid>/score': '调整积分',
                'GET /api/v1/admin/regcodes': '注册码列表',
                'POST /api/v1/admin/regcodes': '创建注册码',
                'DELETE /api/v1/admin/regcodes/<code>': '删除注册码',
                'GET /api/v1/admin/emby/sessions': '所有会话',
                'GET /api/v1/admin/emby/activity': '活动日志',
                'POST /api/v1/admin/emby/broadcast': '广播消息',
                'POST /api/v1/admin/whitelist': '创建白名单用户',
            },
        })
    
    # 错误处理
    @app.errorhandler(404)
    def not_found(e):
        return jsonify({
            'success': False,
            'code': 404,
            'message': '接口不存在',
            'data': None,
            'timestamp': timestamp(),
        }), 404
    
    @app.errorhandler(500)
    def internal_error(e):
        return jsonify({
            'success': False,
            'code': 500,
            'message': '服务器内部错误',
            'data': None,
            'timestamp': timestamp(),
        }), 500
    
    @app.errorhandler(405)
    def method_not_allowed(e):
        return jsonify({
            'success': False,
            'code': 405,
            'message': '请求方法不允许',
            'data': None,
            'timestamp': timestamp(),
        }), 405
    
    return app


__all__ = ['create_app', 'api', 'admin_api']
