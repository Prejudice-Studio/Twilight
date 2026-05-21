"""
API 模块

提供 Flask Web API
"""

import os
import logging

from flask import Flask, jsonify, request

from src.api.routes import api, admin_api
from src.api.v1 import register_v1_blueprints
from src.config import Config, APIConfig, normalize_storage_settings
from src.core.utils import setup_logging, timestamp
from src import __version__
from src.core.request_utils import get_real_client_ip  # noqa: F401 - re-export for convenience


from flask_cors import CORS


logger = logging.getLogger(__name__)


def _log_fd_limit() -> None:
    try:
        import resource

        soft, hard = resource.getrlimit(resource.RLIMIT_NOFILE)
        if soft < 4096:
            logger.warning(
                "进程 NOFILE 上限较低 soft=%s hard=%s；高并发下可能触发 Too many open files。"
                "建议设置 LimitNOFILE=65535 或容器 ulimit nofile=65535:65535。",
                soft,
                hard,
            )
        else:
            logger.info("进程 NOFILE 上限 soft=%s hard=%s", soft, hard)
    except Exception:
        # Windows / restricted environments may not provide resource.
        pass


def create_app() -> Flask:
    """创建 Flask 应用"""
    from pathlib import Path

    normalize_storage_settings()

    # 获取上传目录配置（规范化后的绝对路径）
    uploads_path = Path(APIConfig.UPLOAD_FOLDER).resolve()

    # 确保上传目录存在
    uploads_path.mkdir(parents=True, exist_ok=True)

    # Uploaded files are served by authenticated, path-checked API endpoints instead of
    # Flask's generic static handler.
    app = Flask(__name__, static_folder=None)

    # 配置
    app.config["JSON_AS_ASCII"] = False  # 支持中文
    app.config["JSON_SORT_KEYS"] = False
    app.config["MAX_CONTENT_LENGTH"] = APIConfig.MAX_UPLOAD_SIZE  # 最大上传文件大小
    app.config["UPLOAD_FOLDER"] = str(uploads_path)
    # uploads_path 目录仅用于存储上传文件，不直接暴露为静态目录。

    # CORS 跨域支持
    if APIConfig.CORS_ENABLED:
        cors_origins = APIConfig.CORS_ORIGINS or ["*"]
        allow_all = False
        if cors_origins == "*":
            allow_all = True
        elif isinstance(cors_origins, (list, tuple, set)) and "*" in cors_origins:
            allow_all = True

        CORS(
            app,
            resources={r"/api/*": {"origins": cors_origins}},
            supports_credentials=not allow_all,
            allow_headers=["Content-Type", "Authorization", "X-API-Key", "X-Twilight-Client"],
            methods=["GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"],
            max_age=3600,
        )

    # 注册旧版 API（兼容）
    app.register_blueprint(api)
    app.register_blueprint(admin_api)

    # 注册 v1 API（推荐前端使用）
    # 这里将 /api/v1 下的蓝图全部挂载到主应用，以保持版本化路由结构。
    register_v1_blueprints(app)

    # 配置日志
    if Config.LOGGING:
        setup_logging(level=Config.LOG_LEVEL)
    _log_fd_limit()

    # 兜底：直接运行 `uvicorn asgi:app` / `python main.py api` 时也启动定时任务。
    # start_backend_*.sh 已先起独立 scheduler；这里会因锁文件存在而跳过，避免重复调度。
    if os.getenv("TWILIGHT_API_AUTOSTART_SCHEDULER", "1") == "1":
        try:
            from src.core.background import submit_background
            from src.services.scheduler_service import SchedulerService

            async def _start_scheduler_once():
                ok, msg = await SchedulerService.start_singleton()
                if ok:
                    app.logger.info("Scheduler 自动启动: %s", msg)
                else:
                    app.logger.info("Scheduler 自动启动跳过: %s", msg)

            submit_background(_start_scheduler_once())
        except Exception as exc:  # pragma: no cover
            app.logger.warning("Scheduler 自动启动失败: %s", exc)

    # 根路由
    @app.route("/")
    def index():
        return jsonify(
            {
                "name": "Twilight API",
                "version": __version__,
                "api_versions": ["v1"],
                "docs": "/api/v1/docs",
            }
        )

    # API 文档路由
    @app.route("/api/v1/openapi.json")
    def openapi_json():
        from src.api.v1.openapi import generate_openapi_spec

        return jsonify(generate_openapi_spec())

    @app.route("/api/v1/docs")
    def api_docs():
        from src.api.swagger_template import SWAGGER_UI_HTML

        return SWAGGER_UI_HTML

    @app.after_request
    def apply_security_headers(response):
        # 基础安全响应头
        response.headers.setdefault("X-Content-Type-Options", "nosniff")
        response.headers.setdefault("X-Frame-Options", "DENY")
        response.headers.setdefault("Referrer-Policy", "strict-origin-when-cross-origin")

        # 认证相关接口禁止缓存，避免令牌与敏感响应被浏览器缓存
        if request.path.startswith("/api/v1/auth"):
            response.headers["Cache-Control"] = "no-store, no-cache, must-revalidate, max-age=0"
            response.headers["Pragma"] = "no-cache"
            response.headers["Expires"] = "0"
        return response

    # 错误处理
    @app.errorhandler(404)
    def not_found(e):
        path = request.path or ""
        method = request.method or ""
        if path.startswith("/api/"):
            message = f"接口不存在: {method} {path}"
        else:
            message = f"资源不存在: {method} {path}"
        return (
            jsonify(
                {
                    "success": False,
                    "code": 404,
                    "message": message,
                    "data": {
                        "method": method,
                        "path": path,
                        "hint": "请确认前后端版本一致、后端服务已重启，且路由已注册。",
                    },
                    "timestamp": timestamp(),
                }
            ),
            404,
        )

    @app.errorhandler(500)
    def internal_error(e):
        # 统一 error handler，保证所有未捕获异常也按照统一 JSON 结构返回
        return (
            jsonify(
                {
                    "success": False,
                    "code": 500,
                    "message": "服务器内部错误",
                    "data": None,
                    "timestamp": timestamp(),
                }
            ),
            500,
        )

    @app.errorhandler(405)
    def method_not_allowed(e):
        path = request.path or ""
        method = request.method or ""
        return (
            jsonify(
                {
                    "success": False,
                    "code": 405,
                    "message": f"请求方法不允许: {method} {path}",
                    "data": {
                        "method": method,
                        "path": path,
                        "hint": "请确认前端使用的 HTTP 方法与后端路由一致。",
                    },
                    "timestamp": timestamp(),
                }
            ),
            405,
        )

    return app


__all__ = ["create_app", "api", "admin_api"]
