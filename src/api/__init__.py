"""
API 模块

提供 Flask Web API
"""
from flask import Flask, jsonify, request

from src.api.routes import api, admin_api
from src.api.v1 import register_v1_blueprints
from src.config import Config, APIConfig
from src.core.utils import setup_logging, timestamp


from flask_cors import CORS

def create_app() -> Flask:
    """创建 Flask 应用"""
    import os
    from pathlib import Path
    from src.config import APIConfig
    
    # 获取上传目录配置
    uploads_path = APIConfig.UPLOAD_FOLDER
    
    # 确保上传目录存在
    os.makedirs(uploads_path, exist_ok=True)
    
    app = Flask(__name__, static_folder=uploads_path, static_url_path='/uploads')
    
    # 配置
    app.config['JSON_AS_ASCII'] = False  # 支持中文
    app.config['JSON_SORT_KEYS'] = False
    app.config['MAX_CONTENT_LENGTH'] = APIConfig.MAX_UPLOAD_SIZE  # 最大上传文件大小
    app.config['UPLOAD_FOLDER'] = uploads_path
    
    # CORS 跨域支持
    if APIConfig.CORS_ENABLED:
        cors_origins = APIConfig.CORS_ORIGINS if APIConfig.CORS_ORIGINS else "*"
        if cors_origins == "*":
            import logging
            logging.getLogger(__name__).warning(
                "⚠️ CORS 允许所有来源 (*)，建议在生产环境中配置 cors_origins 白名单"
            )
        CORS(
            app,
            resources={r"/api/*": {"origins": cors_origins}},
            supports_credentials=bool(APIConfig.CORS_ORIGINS),
            allow_headers=['Content-Type', 'Authorization', 'X-API-Key'],
            methods=['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS', 'PATCH'],
            max_age=3600,
        )
    
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
    @app.route('/api/v1/openapi.json')
    def openapi_json():
        from src.api.v1.openapi import generate_openapi_spec
        return jsonify(generate_openapi_spec())

    @app.route('/api/v1/docs')
    def api_docs():
        from src.api.swagger_template import SWAGGER_UI_HTML
        return SWAGGER_UI_HTML
    
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
