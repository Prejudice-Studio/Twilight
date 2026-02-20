# Twilight 后端 API 文档

本文档详细介绍了 Twilight 系统的后端 RESTful API。

## 认证方式

系统支持两种主要的认证方式：

### 1. Bearer Token (JWT 替代方案)
主要用于前端 Web 界面。登录成功后，服务器会返回一个 Token。
在后续请求中，需在 Header 中包含：
```http
Authorization: Bearer <your_token>
```

### 2. API Key
主要用于外部系统集成（如机器人、脚本等）。API Key 具有较长有效期，且权限受控。
在请求中，可以通过以下两种方式之一提供：
- **Header**: `X-API-Key: <your_api_key>`
- **Header**: `Authorization: Bearer <your_api_key>`

---

## 模块说明

| 模块 | 基础路径 | 说明 | 认证需求 |
|------|------|------|---------|
| **Auth** | `/api/v1/auth` | 处理登录、登出、Token 转换 | 部分需要 |
| **Users** | `/api/v1/users` | 用户资料、设备管理、注册 | 需要 |
| **Score** | `/api/v1/score` | 积分操作、签到、红包、排行榜 | 需要 |
| **Emby** | `/api/v1/emby` | Emby 服务器状态、搜索、会话 | 需要 |
| **Media** | `/api/v1/media` | TMDB/Bangumi 搜索、求片申请 | 需要 |
| **Admin** | `/api/v1/admin` | 用户管理、系统统计、配置编辑 | 管理员权限 |
| **Stats** | `/api/v1/stats` | 个人及全站播放统计 | 需要 |
| **Webhook** | `/api/v1/webhook` | 接收 Emby/Jellyfin 事件 | Secret 验证 |
| **System** | `/api/v1/system` | 系统公开信息、健康检查 | - |
| **API Key** | `/api/v1/apikey` | 外部专用简化接口 | API Key |

---

## 常用接口示例

### 用户注册
`POST /api/v1/users/register`
```json
{
    "username": "newuser",
    "password": "strongpassword",
    "reg_code": "optional-code",
    "telegram_id": 12345678
}
```

### 签到
`POST /api/v1/score/checkin`
响应：
```json
{
    "success": true,
    "data": {
        "score": 15,
        "balance": 250,
        "streak": 5
    }
}
```

### 媒体搜索
`GET /api/v1/media/search?q=进击的巨人&source=all`

---

## 更多文档

- **API Key 专用接口**: [API_KEY_API.md](./API_KEY_API.md) - 详细介绍了为外部集成设计的专用接口。
- **Swagger UI**: 系统运行后，访问 `/api/v1/docs` 可查看交互式 API 文档。
