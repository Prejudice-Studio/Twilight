from flask import current_app, jsonify
from src import __version__

def generate_openapi_spec():
    """生成 OpenAPI 3.0.0 规范的 JSON"""
    spec = {
        "openapi": "3.0.0",
        "info": {
            "title": "Twilight API",
            "description": "Emby User Management System API",
            "version": __version__,
            "contact": {
                "name": "Twilight Team"
            }
        },
        "servers": [
            {"url": "/api/v1", "description": "V1 API Server"}
        ],
        "paths": {},
        "components": {
            "securitySchemes": {
                "BearerAuth": {
                    "type": "http",
                    "scheme": "bearer",
                    "bearerFormat": "JWT"
                },
                "ApiKeyAuth": {
                    "type": "apiKey",
                    "in": "header",
                    "name": "X-API-Key"
                }
            }
        }
    }

    # 获取所有路由
    for rule in current_app.url_map.iter_rules():
        if rule.endpoint == 'static' or not rule.rule.startswith('/api/v1'):
            continue
            
        # 移除 /api/v1 前缀
        path = rule.rule[7:]
        if not path: path = "/"
        
        # 将 Flask 的路径参数转换成 OpenAPI 格式
        # e.g., /users/<int:uid> -> /users/{uid}
        import re
        swagger_path = re.sub(r'<(?:\w+:)?(\w+)>', r'{\1}', path)
        
        if swagger_path not in spec["paths"]:
            spec["paths"][swagger_path] = {}
            
        for method in rule.methods:
            if method in ("GET", "POST", "PUT", "DELETE", "PATCH"):
                # 确定 Tag
                tag = rule.endpoint.split(".")[1] if "." in rule.endpoint else "Other"
                
                # 确定参数
                parameters = []
                for param in re.findall(r'{\w+}', swagger_path):
                    parameters.append({
                        "name": param.strip("{}"),
                        "in": "path",
                        "required": True,
                        "schema": {"type": "string"}
                    })
                
                spec["paths"][swagger_path][method.lower()] = {
                    "tags": [tag.capitalize()],
                    "summary": rule.endpoint,
                    "parameters": parameters,
                    "responses": {
                        "200": {
                            "description": "OK",
                            "content": {
                                "application/json": {
                                    "example": {
                                        "success": True,
                                        "message": "Success",
                                        "data": {}
                                    }
                                }
                            }
                        }
                    },
                    "security": [{"BearerAuth": []}, {"ApiKeyAuth": []}]
                }

    return spec
