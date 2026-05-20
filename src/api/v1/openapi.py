import inspect
import re

from flask import current_app
from src import __version__


PUBLIC_PATHS = {
    "/auth/login",
    "/auth/login/telegram",
    "/auth/login/apikey",
    "/users/register",
    "/users/register/emby/status",
    "/users/check-available",
    "/users/regcode/check",
    "/users/telegram/register/bind-code",
    "/users/telegram/register/bind-code/status",
    "/system/info",
    "/system/server-icon",
    "/system/health",
    "/announcements",
    "/invite/config",
    "/invite/check",
    "/signin/config",
}

API_KEY_PREFIXES = ("/apikey",)

TAG_NAMES = {
    "auth": "Auth",
    "users": "Users",
    "emby": "Emby",
    "admin": "Admin",
    "media": "Media",
    "stats": "Stats",
    "security": "Security",
    "batch": "Batch",
    "system": "System",
    "apikey": "API Key",
    "announcements": "Announcements",
    "invite": "Invite",
    "signin": "Signin",
}


def _swagger_path(rule_path: str) -> str:
    path = rule_path[7:] or "/"
    return re.sub(r"<(?:\w+:)?(\w+)>", r"{\1}", path)


def _path_parameters(swagger_path: str) -> list[dict]:
    return [
        {"name": param.strip("{}"), "in": "path", "required": True, "schema": {"type": "string"}}
        for param in re.findall(r"{\w+}", swagger_path)
    ]


def _tag_from_endpoint(endpoint: str) -> str:
    blueprint = endpoint.split(".", 1)[0]
    return TAG_NAMES.get(blueprint, blueprint.capitalize())


def _summary_for_endpoint(endpoint: str) -> str:
    view_func = current_app.view_functions.get(endpoint)
    doc = inspect.getdoc(view_func) if view_func else None
    if doc:
        return doc.splitlines()[0].strip()
    return endpoint


def _security_for_path(swagger_path: str) -> list[dict]:
    if swagger_path in PUBLIC_PATHS:
        return []
    if swagger_path.startswith(API_KEY_PREFIXES):
        return [{"ApiKeyAuth": []}]
    return [{"BearerAuth": []}]


def generate_openapi_spec():
    """生成 OpenAPI 3.0.0 规范的 JSON"""
    spec = {
        "openapi": "3.0.0",
        "info": {
            "title": "Twilight API",
            "description": "Emby User Management System API",
            "version": __version__,
            "contact": {"name": "Twilight Team"},
        },
        "servers": [{"url": "/api/v1", "description": "V1 API Server"}],
        "paths": {},
        "components": {
            "securitySchemes": {
                "BearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
                "ApiKeyAuth": {"type": "apiKey", "in": "header", "name": "X-API-Key"},
            }
        },
    }

    # 遍历当前 Flask 应用的路由，自动生成 OpenAPI 规范中的 paths 部分。
    for rule in current_app.url_map.iter_rules():
        if rule.endpoint == "static" or not rule.rule.startswith("/api/v1"):
            continue

        swagger_path = _swagger_path(rule.rule)

        if swagger_path not in spec["paths"]:
            spec["paths"][swagger_path] = {}

        for method in rule.methods:
            if method in ("GET", "POST", "PUT", "DELETE", "PATCH"):
                spec["paths"][swagger_path][method.lower()] = {
                    "tags": [_tag_from_endpoint(rule.endpoint)],
                    "summary": _summary_for_endpoint(rule.endpoint),
                    "operationId": f"{rule.endpoint}.{method.lower()}",
                    "parameters": _path_parameters(swagger_path),
                    "responses": {
                        "200": {
                            "description": "OK",
                            "content": {
                                "application/json": {"example": {"success": True, "message": "Success", "data": {}}}
                            },
                        }
                    },
                    "security": _security_for_path(swagger_path),
                }

    return spec
