"""TestWeb 演示接口。

这些接口只返回预设假数据，供 `/testweb*` 前端演示页面使用。它们不读取登录态、
不读写数据库、不调用 Emby/Telegram，也不复用真实业务服务。演示写操作也不读取
请求体，避免用户误输入的内容被回显或进入日志/缓存。
"""

from __future__ import annotations

from copy import deepcopy
from typing import Any

from flask import Blueprint, request

from src import __version__
from src.api.v1.auth import api_response

demo_bp = Blueprint("demo", __name__, url_prefix="/demo")


DEMO_USER = {
    "uid": 1001,
    "username": "demo_user",
    "telegram_id": 123456789,
    "email": "de***@example.com",
    "role": 1,
    "role_name": "普通用户",
    "active": True,
    "expire_status": "42 天后到期",
    "expired_at": 1770000000,
    "emby_id": "demo-emby-user-id",
    "emby_username": "demo_user",
    "emby_bound": True,
    "pending_emby": False,
    "pending_emby_days": None,
    "bgm_mode": True,
    "avatar": None,
    "register_time": 1760000000,
    "created_at": 1760000000,
}

DEMO_ADMIN = {
    **DEMO_USER,
    "uid": 1,
    "username": "demo_admin",
    "role": 0,
    "role_name": "管理员",
    "expire_status": "永久",
    "expired_at": -1,
}

DEMO_BOOTSTRAP = {
    "version": __version__,
    "mode": "demo",
    "readonly": True,
    "notice": "TestWeb 使用后端模拟接口，只返回假数据，不执行真实操作。",
    "user": DEMO_USER,
    "admin": DEMO_ADMIN,
    "metrics": {
        "user": [
            {"label": "账号状态", "value": "正常", "description": "Emby 已绑定"},
            {"label": "剩余天数", "value": "42", "description": "到期提醒开启"},
            {"label": "积分", "value": "1,280", "description": "今日已签到"},
            {"label": "求片", "value": "3", "description": "1 个已完成"},
        ],
        "admin": [
            {"label": "总用户", "value": "186", "description": "+12 本月"},
            {"label": "Emby 绑定", "value": "143", "description": "77%"},
            {"label": "待处理求片", "value": "8", "description": "3 个下载中"},
            {"label": "定时任务", "value": "11", "description": "9 个启用"},
        ],
    },
    "announcements": [
        {"id": 1, "title": "维护窗口", "tag": "置顶", "level": "warning", "content": "周六 02:00-03:00 进行线路维护，期间播放可能短暂中断。"},
        {"id": 2, "title": "新片补全", "tag": "更新", "level": "info", "content": "本周新增 42 部电影与 13 部剧集，求片队列处理完成率 93%。"},
        {"id": 3, "title": "安全提醒", "tag": "提醒", "level": "notice", "content": "请勿复用弱密码，演示站不会保存或提交任何输入内容。"},
    ],
    "requests": [
        {"id": 101, "title": "The Bear S03", "user": "mika", "status": "pending", "source": "TMDB", "note": "等待下载"},
        {"id": 102, "title": "葬送的芙莉莲", "user": "akira", "status": "downloading", "source": "BGM", "note": "已加入队列"},
        {"id": 103, "title": "Dune: Part Two", "user": "nova", "status": "completed", "source": "TMDB", "note": "已入库"},
    ],
    "scheduler_runs": [
        {"name": "过期用户检查", "status": "success", "time": "03:00", "logs": ["扫描用户 186", "禁用过期账号 2", "同步 Emby 完成"]},
        {"name": "系统自动更新", "status": "success", "time": "04:00", "logs": ["git fetch origin main", "git pull --ff-only", "未安装 Python 依赖"]},
        {"name": "清理未绑定 Emby", "status": "failed", "time": "手动", "logs": ["dry_run=false", "跳过注册队列 UID 12", "失败：示例错误"]},
    ],
    "users": [
        {"uid": 1, "username": "admin", "role": "管理员", "active": True, "emby": "已绑定", "expire": "永久"},
        {"uid": 28, "username": "mika", "role": "普通用户", "active": True, "emby": "待补建", "expire": "未绑定"},
        {"uid": 42, "username": "nova", "role": "白名单", "active": True, "emby": "已绑定", "expire": "永久"},
        {"uid": 77, "username": "guest", "role": "普通用户", "active": False, "emby": "未绑定", "expire": "未绑定"},
    ],
    "regcodes": [
        {"code": "TW-register-ABCDEFGHJKLMNPQRST", "type": 1, "type_name": "注册", "status": "available", "days": 30, "use_count": 0, "use_count_limit": 1},
        {"code": "TW-renew-23456789ABCDEFGHJK", "type": 2, "type_name": "续期", "status": "used_up", "days": 90, "use_count": 1, "use_count_limit": 1},
        {"code": "TW-whitelist-DEMO-CODE-01", "type": 3, "type_name": "白名单", "status": "available", "days": -1, "use_count": 2, "use_count_limit": -1},
    ],
    "media": [
        {"title": "The Bear", "type": "剧集", "year": "2022", "status": "可求片", "rating": "8.6"},
        {"title": "Dune: Part Two", "type": "电影", "year": "2024", "status": "已入库", "rating": "8.4"},
        {"title": "Frieren", "type": "动画", "year": "2023", "status": "处理中", "rating": "9.1"},
    ],
    "audit_events": [
        {"actor": "admin", "action": "授予注册队列用户资格", "target": "mika", "level": "warning"},
        {"actor": "system", "action": "自动更新完成", "target": "main", "level": "success"},
        {"actor": "bot", "action": "Telegram 换绑审核", "target": "nova", "level": "info"},
    ],
    "notifications": [
        {"type": "bell", "text": "你有 1 个求片已完成"},
        {"type": "calendar", "text": "账号将在 12 天后到期"},
        {"type": "search", "text": "模拟搜索不会访问真实业务接口"},
    ],
}


def _demo_data() -> dict[str, Any]:
    return deepcopy(DEMO_BOOTSTRAP)


def _demo_response(success: bool, message: str, data: Any = None, code: int = 200):
    """演示接口统一响应：禁止缓存，不设置 Cookie，不回显真实请求内容。"""
    response, status = api_response(success, message, data, code)
    response.headers["Cache-Control"] = "no-store, no-cache, must-revalidate, max-age=0"
    response.headers["Pragma"] = "no-cache"
    response.headers["Expires"] = "0"
    response.headers["X-Twilight-Demo"] = "true"
    response.headers["X-Robots-Tag"] = "noindex, nofollow"
    return response, status


@demo_bp.route("/bootstrap", methods=["GET"])
async def demo_bootstrap():
    role = (request.args.get("role") or "user").strip().lower()
    data = _demo_data()
    data["role"] = "admin" if role == "admin" else "user"
    data["demo_user"] = data["admin"] if data["role"] == "admin" else data["user"]
    return _demo_response(True, "获取演示数据成功", data)


@demo_bp.route("/auth/me", methods=["GET"])
async def demo_auth_me():
    role = (request.args.get("role") or "user").strip().lower()
    data = _demo_data()["admin" if role == "admin" else "user"]
    return _demo_response(True, "获取成功", data)


@demo_bp.route("/system/info", methods=["GET"])
async def demo_system_info():
    return _demo_response(
        True,
        "获取成功",
        {
            "name": "Twilight TestWeb Demo",
            "version": __version__,
            "demo": True,
            "readonly": True,
            "features": {
                "register": True,
                "media_request": True,
                "telegram": True,
                "api_key": True,
            },
        },
    )


@demo_bp.route("/admin/users", methods=["GET"])
async def demo_admin_users():
    users = _demo_data()["users"]
    return _demo_response(True, f"共 {len(users)} 个演示用户", {"users": users, "total": len(users), "page": 1, "per_page": 20})


@demo_bp.route("/admin/regcodes", methods=["GET"])
async def demo_admin_regcodes():
    regcodes = _demo_data()["regcodes"]
    return _demo_response(True, f"共 {len(regcodes)} 个演示卡码", {"regcodes": regcodes, "total": len(regcodes), "page": 1, "per_page": 20})


@demo_bp.route("/media/search", methods=["GET"])
async def demo_media_search():
    keyword = (request.args.get("q") or request.args.get("keyword") or "").strip().lower()
    media = _demo_data()["media"]
    if keyword:
        media = [item for item in media if keyword in item["title"].lower()]
    return _demo_response(True, "搜索成功", {"results": media, "total": len(media), "demo": True})


@demo_bp.route("/action", methods=["POST"])
@demo_bp.route("/action/<path:action_name>", methods=["POST", "PUT", "DELETE"])
async def demo_action(action_name: str = "generic"):
    safe_action = "".join(ch for ch in str(action_name or "generic") if ch.isalnum() or ch in ("-", "_"))[:64]
    return _demo_response(
        True,
        "演示操作已模拟完成，未执行真实写入",
        {
            "simulated": True,
            "readonly": True,
            "action": safe_action or "generic",
            "request_body_ignored": True,
        },
    )
