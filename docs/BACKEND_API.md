# Twilight 后端 API 文档

本文档介绍 Twilight 后端 API 的认证方式、模块划分与调用约定。

## 基础信息

- API Base URL：`http://localhost:5000/api/v1`
- Swagger UI：`http://localhost:5000/api/v1/docs`
- 响应格式：默认返回 `success / message / data / timestamp`

> 说明：接口会持续演进，完整字段请以 Swagger 和实际响应为准。

## 认证方式

### 1) Bearer Token（Web 前端常用）

登录成功后，在请求头带上：

```http
Authorization: Bearer <token>
```

### 2) API Key（外部系统集成）

在请求头带上：

```http
X-API-Key: <api_key>
```

API Key 专用接口详见 [API_KEY_API.md](./API_KEY_API.md)。

## 常见公共接口

### 健康检查

- `GET /system/health`
- 用途：检查 Database / Redis / Emby 连通性
- 认证：不需要

### 系统统计（管理员）

- `GET /system/stats`
- 用途：获取 CPU / Memory / Disk 运行状态
- 认证：需要管理员权限

## 模块总览

| 模块 | 前缀 | 说明 | 权限 |
|------|------|------|------|
| Auth | `/auth` | 登录、登出、认证、API Key 权限管理 | 部分公开 |
| Users | `/users` | 用户资料、账号与设备能力、背景自定义 | 用户 |
| Score | `/score` | 签到、积分流转、排行榜 | 用户 |
| Media | `/media` | TMDB/Bangumi 搜索与求片 | 用户 |
| Emby | `/emby` | Emby 账户与会话能力 | 用户 |
| Admin | `/admin` | 用户/注册码/系统管理 | 管理员 |
| Stats | `/stats` | 播放与使用统计 | 用户/管理员 |
| Webhook | `/webhook` | Emby/Jellyfin/Plex 事件接收 | Secret 校验 |
| System | `/system` | 健康、系统公开信息、服务器名称/图标 | 公开/管理员 |
| API Key | `/apikey` | 外部系统专用接口（含权限管理） | API Key |

## 调用示例

### 用户注册

`POST /users/register`

```json
{
  "username": "newuser",
  "password": "strongpassword",
  "reg_code": "optional-code",
  "telegram_id": 12345678
}
```

### 签到

`POST /score/checkin`

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

`GET /media/search?q=进击的巨人&source=all`

## 错误处理约定

- 未认证：`401`
- 权限不足：`403`
- 参数错误：`400`
- 资源不存在：`404`
- 服务器错误：`500`

建议外部调用方统一处理 `success=false` 与 HTTP 状态码。

## 推荐阅读

- [API Key 专用接口](./API_KEY_API.md)
- [前端开发文档](./FRONTEND_API.md)
- [开发指南](./DEVELOPMENT.md)
