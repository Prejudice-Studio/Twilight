"""
API v1 模块

为前端提供完整的 RESTful API
"""
from flask import Blueprint

from src.api.v1.auth import auth_bp
from src.api.v1.users import users_bp
from src.api.v1.score import score_bp
from src.api.v1.emby import emby_bp
from src.api.v1.admin import admin_bp
from src.api.v1.media import media_bp
from src.api.v1.stats import stats_bp
from src.api.v1.webhook import webhook_bp
from src.api.v1.security import security_bp
from src.api.v1.batch import batch_bp
from src.api.v1.system import system_bp
from src.api.v1.apikey import apikey_bp

# 创建 v1 API 蓝图
api_v1 = Blueprint('api_v1', __name__, url_prefix='/api/v1')


def register_v1_blueprints(app):
    """注册所有 v1 API 蓝图"""
    app.register_blueprint(auth_bp, url_prefix='/api/v1/auth')
    app.register_blueprint(users_bp, url_prefix='/api/v1/users')
    app.register_blueprint(score_bp, url_prefix='/api/v1/score')
    app.register_blueprint(emby_bp, url_prefix='/api/v1/emby')
    app.register_blueprint(admin_bp, url_prefix='/api/v1/admin')
    app.register_blueprint(media_bp, url_prefix='/api/v1/media')
    app.register_blueprint(stats_bp, url_prefix='/api/v1/stats')
    app.register_blueprint(webhook_bp, url_prefix='/api/v1/webhook')
    app.register_blueprint(security_bp, url_prefix='/api/v1/security')
    app.register_blueprint(batch_bp, url_prefix='/api/v1/batch')
    app.register_blueprint(system_bp, url_prefix='/api/v1/system')
    app.register_blueprint(apikey_bp, url_prefix='/api/v1/apikey')


__all__ = [
    'api_v1',
    'register_v1_blueprints',
    'auth_bp',
    'users_bp',
    'score_bp',
    'emby_bp',
    'admin_bp',
    'media_bp',
    'stats_bp',
    'webhook_bp',
    'security_bp',
    'batch_bp',
    'system_bp',
    'apikey_bp',
]
