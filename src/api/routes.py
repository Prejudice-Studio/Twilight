"""
旧版 API 路由（兼容性）

这些是旧版 API 蓝图，为了向后兼容而保留。
新代码应使用 /api/v1/ 下的 API。
"""
from flask import Blueprint

# 创建空的蓝图（兼容旧代码）
api = Blueprint('api', __name__)
admin_api = Blueprint('admin_api', __name__)

__all__ = ['api', 'admin_api']

