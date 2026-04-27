# Twilight 后端 API 文档

本文档为 Twilight 后端 API 的统一参考指南，采用模块化结构，覆盖认证、请求格式、错误码、常用接口与管理员功能。

## 1. 文档说明

- Base URL：`http://localhost:5000/api/v1`
- Swagger UI：`http://localhost:5000/api/v1/docs`
- 格式：JSON 响应结构为 `success` + `message` + `data` + `timestamp`
- 说明：接口变更优先以 Swagger 和后端实际返回为准。

## 2. 认证与请求规范

### 2.1 认证方式

#### 登录 Token（前端）

前端登录后接口调用使用：

```http
Authorization: Bearer <token>
```

#### API Key（外部系统）

API Key 接口支持：

```http
X-API-Key: <api_key>
```

或：

```http
Authorization: Bearer <api_key>
```

或：

```http
Authorization: ApiKey <api_key>
```

> 注意：`/api/v1/apikey` 前缀的接口仅支持 API Key 认证，不支持普通登录 Token。

### 2.2 通用请求头

- `Content-Type: application/json`
- `Authorization: Bearer <token>`（前端 Token）
- `X-API-Key: <api_key>` 或 `Authorization: ApiKey <api_key>`（API Key）

### 2.3 响应结构

成功示例：

```json
{
  "success": true,
  "message": "操作成功",
  "data": { ... },
  "timestamp": 1680000000
}
```

失败示例：

```json
{
  "success": false,
  "message": "错误信息",
  "data": null,
  "timestamp": 1680000000
}
```

## 3. 错误码

| HTTP 状态码 | 含义 |
|------------|------|
| 200 | 请求成功 |
| 400 | 参数错误 / 请求格式不合法 |
| 401 | 未认证 / Token 或 API Key 无效 |
| 403 | 权限不足 / 账号或 API Key 被禁用 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

## 4. 模块总览

| 模块 | 路径前缀 | 说明 |
|------|----------|------|
| Auth | `/auth` | 登录、会话、Token 刷新、API Key 管理 |
| Users | `/users` | 注册、个人信息、Emby 绑定、续期、设备、Telegram |
| Score | `/score` | 积分、签到、转账、排行榜、红包 |
| Media | `/media` | TMDB/Bangumi 搜索、求片、库存管理 |
| Emby | `/emby` | Emby 账号状态、库、搜索、会话 |
| Admin | `/admin` | 管理用户、Emby 同步与审查、注册码、广播 |
| Stats | `/stats` | 播放统计、排行榜 |
| Webhook | `/webhook` | 事件接收、Webhook 管理、Bangumi 推送 |
| System | `/system` | 健康、系统信息、配置、路由列表 |
| API Key | `/apikey` | 外部系统专用 API Key 接口 |

## 5. Auth 模块

### 5.1 登录

`POST /auth/login`

- 说明：用户名/密码登录
- 认证：公开
- 请求头：
  - `Content-Type: application/json`

- 请求体：

```json
{
  "username": "user123",
  "password": "strongpassword"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"user123","password":"strongpassword"}'
```

### 5.2 登出

`POST /auth/logout`

- 说明：注销当前登录会话
- 认证：登录 Token
- 请求头：
  - `Authorization: Bearer <token>`

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/auth/logout" \
  -H "Authorization: Bearer <token>"
```

### 5.3 当前用户

`GET /auth/me`

- 说明：获取当前登录用户信息
- 认证：登录 Token
- 请求头：
  - `Authorization: Bearer <token>`

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/auth/me" \
  -H "Authorization: Bearer <token>"
```

### 5.4 刷新 Token

`POST /auth/refresh`

- 说明：刷新用户 Token
- 认证：登录 Token
- 请求头：
  - `Authorization: Bearer <token>`

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/auth/refresh" \
  -H "Authorization: Bearer <token>"
```

### 5.5 API Key 登录端管理

#### 获取当前用户 API Key

`GET /auth/apikey`

- 认证：登录 Token
- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/auth/apikey" \
  -H "Authorization: Bearer <token>"
```

#### 生成 / 刷新 API Key

`POST /auth/apikey`

- 认证：登录 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/auth/apikey" \
  -H "Authorization: Bearer <token>"
```

#### 删除当前 API Key

`DELETE /auth/apikey`

- 认证：登录 Token
- 示例 cURL：

```bash
curl -X DELETE "http://localhost:5000/api/v1/auth/apikey" \
  -H "Authorization: Bearer <token>"
```

#### 启用当前 API Key

`POST /auth/apikey/enable`

- 认证：登录 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/auth/apikey/enable" \
  -H "Authorization: Bearer <token>"
```

#### 获取 API Key 权限列表

`GET /auth/apikey/permissions`

- 认证：登录 Token
- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/auth/apikey/permissions" \
  -H "Authorization: Bearer <token>"
```

#### 更新 API Key 权限

`PUT /auth/apikey/permissions`

- 认证：登录 Token
- 请求体：

```json
{
  "permissions": ["account:read", "score:read"]
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/auth/apikey/permissions" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"permissions":["account:read","score:read"]}'
```

## 6. Users 模块

### 6.1 注册与校验

#### 新用户注册

`POST /users/register`

- 说明：新用户注册
- 请求头：
  - `Content-Type: application/json`
- 请求体：

```json
{
  "username": "newuser",
  "password": "Password123!",
  "email": "newuser@example.com"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","password":"Password123!","email":"newuser@example.com"}'
```

#### 检查用户名是否可用

`GET /users/check-available?username=<name>`

- 说明：检查用户名是否可用
- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/check-available?username=newuser"
```

### 6.2 当前用户信息

#### 获取当前用户信息

`GET /users/me`

- 说明：获取当前用户详细信息
- 认证：登录 Token
- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me" \
  -H "Authorization: Bearer <token>"
```

#### 更新当前用户信息

`PUT /users/me`

- 说明：更新当前用户信息
- 认证：登录 Token
- 请求体示例：

```json
{
  "email": "updated@example.com",
  "bgm_mode": "token",
  "bgm_token": "new-bgm-token"
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/users/me" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"email":"updated@example.com","bgm_mode":"token","bgm_token":"new-bgm-token"}'
```

#### 修改用户名

`PUT /users/me/username`

- 说明：修改用户名
- 认证：登录 Token
- 请求体：

```json
{
  "username": "newusername"
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/users/me/username" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"newusername"}'
```

#### 修改密码

`PUT /users/me/password`

- 说明：修改密码
- 认证：登录 Token
- 请求体：

```json
{
  "old_password": "oldpass",
  "new_password": "newPassword123!"
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/users/me/password" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"oldpass","new_password":"newPassword123!"}'
```

#### 验证并修改密码

`POST /users/me/password/change`

- 说明：验证当前密码并修改密码
- 认证：登录 Token
- 请求体：

```json
{
  "current_password": "oldpass",
  "new_password": "newPassword123!"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/password/change" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"current_password":"oldpass","new_password":"newPassword123!"}'
```

### 6.3 Emby 绑定与设置

#### 绑定 Emby 账号

`POST /users/me/emby/bind`

- 说明：绑定 Emby 账号
- 认证：登录 Token
- 请求体：

```json
{
  "emby_id": "user_emby_id",
  "emby_password": "emby_password"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/emby/bind" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"emby_id":"user_emby_id","emby_password":"emby_password"}'
```

#### 解绑 Emby 账号

`POST /users/me/emby/unbind`

- 说明：解绑 Emby 账号
- 认证：登录 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/emby/unbind" \
  -H "Authorization: Bearer <token>"
```

#### 查询 NSFW 访问状态

`GET /users/me/nsfw`

- 说明：查询当前用户 NSFW 访问状态
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me/nsfw" \
  -H "Authorization: Bearer <token>"
```

#### 切换 NSFW 权限

`PUT /users/me/nsfw`

- 说明：切换当前用户 NSFW 权限
- 认证：登录 Token
- 请求体：

```json
{
  "enable": true
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/users/me/nsfw" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"enable":true}'
```

### 6.4 续期与授权码

#### 续期用户

`POST /users/me/renew`

- 说明：已激活用户续期
- 认证：登录 Token
- 请求体：

```json
{
  "days": 30
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/renew" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"days":30}'
```

#### 使用注册码 / 续期码

`POST /users/me/use-code`

- 说明：使用注册码 / 续期码
- 认证：登录 Token
- 请求体：

```json
{
  "reg_code": "code-abc123"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/use-code" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-abc123"}'
```

#### 使用积分续期

`POST /users/me/renew-by-score`

- 说明：使用积分续期
- 认证：登录 Token
- 请求体：

```json
{
  "score": 1000
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/renew-by-score" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"score":1000}'
```

### 6.5 设备与登录历史

#### 查看当前设备列表

`GET /users/me/devices`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me/devices" \
  -H "Authorization: Bearer <token>"
```

#### 移除指定设备

`DELETE /users/me/devices/<device_id>`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X DELETE "http://localhost:5000/api/v1/users/me/devices/abc123" \
  -H "Authorization: Bearer <token>"
```

#### 查看当前登录会话

`GET /users/me/sessions`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me/sessions" \
  -H "Authorization: Bearer <token>"
```

#### 查看登录历史

`GET /users/me/login-history`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me/login-history" \
  -H "Authorization: Bearer <token>"
```

### 6.6 Telegram 绑定

#### 查询绑定状态

`GET /users/me/telegram`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me/telegram" \
  -H "Authorization: Bearer <token>"
```

#### 生成绑定验证码

`POST /users/me/telegram/bind-code`

- 认证：登录 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/telegram/bind-code" \
  -H "Authorization: Bearer <token>"
```

#### 确认绑定 Telegram

`POST /users/me/telegram/bind-confirm`

- 认证：登录 Token
- 请求体：

```json
{
  "code": "123456"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/telegram/bind-confirm" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"code":"123456"}'
```

#### 解绑 Telegram

`POST /users/me/telegram/unbind`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/telegram/unbind" \
  -H "Authorization: Bearer <token>"
```

### 6.7 个人设置

`GET /users/me/settings`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/users/me/settings" \
  -H "Authorization: Bearer <token>"
```

## 7. Score 模块

### 获取积分余额

`GET /score/balance`

- 说明：获取积分余额
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/score/balance" \
  -H "Authorization: Bearer <token>"
```

### 获取积分信息

`GET /score/info`

- 说明：获取积分基本信息与签到状态
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/score/info" \
  -H "Authorization: Bearer <token>"
```

### 每日签到

`POST /score/checkin`

- 说明：每日签到
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/score/checkin" \
  -H "Authorization: Bearer <token>"
```

### 获取积分历史

`GET /score/history?page=1&per_page=20&type=checkin`

- 说明：获取积分历史，支持 `page`、`per_page`、`type`
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/score/history?page=1&per_page=20&type=checkin" \
  -H "Authorization: Bearer <token>"
```

### 转账给其他用户

`POST /score/transfer`

- 说明：转账给其他用户
- 认证：登录 Token
- 请求体：

```json
{
  "target_uid": 123,
  "amount": 500,
  "note": "感谢帮忙"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/score/transfer" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"target_uid":123,"amount":500,"note":"感谢帮忙"}'
```

### 获取积分排行榜

`GET /score/ranking?limit=20`

- 说明：获取积分排行榜
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/score/ranking?limit=20" \
  -H "Authorization: Bearer <token>"
```

### 获取积分规则配置

`GET /score/config`

- 说明：获取积分规则配置
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/score/config" \
  -H "Authorization: Bearer <token>"
```

### 创建红包

`POST /score/redpacket`

- 说明：创建红包
- 认证：登录 Token
- 请求体：

```json
{
  "total_amount": 1000,
  "count": 10,
  "message": "春季活动"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/score/redpacket" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"total_amount":1000,"count":10,"message":"春季活动"}'
```

### 抢红包

`POST /score/redpacket/<rp_key>/grab`

- 说明：抢红包
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/score/redpacket/abc123/grab" \
  -H "Authorization: Bearer <token>"
```

### 提现红包

`POST /score/redpacket/<rp_key>/withdraw`

- 说明：提现红包
- 认证：登录 Token
- 请求体：

```json
{
  "amount": 100
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/score/redpacket/abc123/withdraw" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"amount":100}'
```

## 8. Media 模块

### 通用媒体搜索

`GET /media/search?keyword=<keyword>&page=1&per_page=20`

- 说明：通用媒体搜索
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/search?keyword=matrix&page=1&per_page=20" \
  -H "Authorization: Bearer <token>"
```

### TMDB 搜索

`GET /media/search/tmdb?query=<query>&page=1`

- 说明：TMDB 搜索
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/search/tmdb?query=Inception&page=1" \
  -H "Authorization: Bearer <token>"
```

### Bangumi 搜索

`GET /media/search/bangumi?query=<query>&page=1`

- 说明：Bangumi 搜索
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/search/bangumi?query=你的名字&page=1" \
  -H "Authorization: Bearer <token>"
```

### 通过 source_type 和 media_id 查询详情

`GET /media/search/id/<source_type>/<media_id>`

- 说明：通过源类型和媒体 ID 查询详情
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/search/id/tmdb/12345" \
  -H "Authorization: Bearer <token>"
```

### 媒体详情

`GET /media/detail?source=tmdb&id=12345`

- 说明：查询媒体详情
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/detail?source=tmdb&id=12345" \
  -H "Authorization: Bearer <token>"
```

### TMDB 详情

`GET /media/tmdb/<tmdb_id>`

- 说明：TMDB 详情
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/tmdb/550" \
  -H "Authorization: Bearer <token>"
```

### Bangumi 详情

`GET /media/bangumi/<bgm_id>`

- 说明：Bangumi 详情
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/bangumi/1234" \
  -H "Authorization: Bearer <token>"
```

### 库存检查

`POST /media/inventory/check`

- 说明：库存检查
- 认证：登录 Token
- 请求体：

```json
{
  "tmdb_id": 550,
  "source": "tmdb"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/media/inventory/check" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"tmdb_id":550,"source":"tmdb"}'
```

### 库存搜索

`GET /media/inventory/search?keyword=<keyword>&page=1&per_page=20`

- 说明：库存搜索
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/inventory/search?keyword=matrix&page=1&per_page=20" \
  -H "Authorization: Bearer <token>"
```

### 创建求片请求

`POST /media/request`

- 说明：创建求片请求
- 认证：登录 Token
- 请求体：

```json
{
  "title": "电影名称",
  "source": "bangumi",
  "remarks": "请尽快添加"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/media/request" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"title":"电影名称","source":"bangumi","remarks":"请尽快添加"}'
```

### 查询我的求片请求

`GET /media/request/my`

- 说明：查询我的求片请求
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/request/my" \
  -H "Authorization: Bearer <token>"
```

### 查询待处理求片请求

`GET /media/request/pending`

- 说明：查询待处理求片请求
- 认证：登录 Token
- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/request/pending" \
  -H "Authorization: Bearer <token>"
```

### 更新求片请求状态

`PUT /media/request/<int:request_id>/status`

- 说明：更新求片请求状态
- 认证：登录 Token
- 请求体：

```json
{
  "status": "approved",
  "remarks": "已处理"
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/media/request/123/status" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"status":"approved","remarks":"已处理"}'
```

### 外部求片更新

`POST /media/request/external/update`

- 说明：外部求片更新
- 认证：登录 Token
- 请求体：

```json
{
  "request_id": 123,
  "status": "updated",
  "note": "外部系统同步"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/media/request/external/update" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"request_id":123,"status":"updated","note":"外部系统同步"}'
```

### 查询单个求片请求

`GET /media/request/<int:request_id>`

- 说明：查询单个求片请求
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/media/request/123" \
  -H "Authorization: Bearer <token>"
```

### 取消求片请求

`DELETE /media/request/<int:request_id>`

- 说明：取消求片请求
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X DELETE "http://localhost:5000/api/v1/media/request/123" \
  -H "Authorization: Bearer <token>"
```

## 9. Emby 模块

### 查询当前用户 Emby 状态

`GET /emby/status`

- 说明：查询当前用户 Emby 状态
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/status" \
  -H "Authorization: Bearer <token>"
```

### 获取 Emby 服务 URLs

`GET /emby/urls`

- 说明：获取 Emby 服务 URLs
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/urls" \
  -H "Authorization: Bearer <token>"
```

### 获取 Emby 媒体库列表

`GET /emby/libraries`

- 说明：获取 Emby 媒体库列表
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/libraries" \
  -H "Authorization: Bearer <token>"
```

### Emby 内容搜索

`GET /emby/search?query=<keyword>&page=1&per_page=20`

- 说明：Emby 内容搜索
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/search?query=Inception&page=1&per_page=20" \
  -H "Authorization: Bearer <token>"
```

### 获取最新媒体

`GET /emby/latest`

- 说明：获取最新媒体
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/latest" \
  -H "Authorization: Bearer <token>"
```

### 查询 Emby 活跃会话数量

`GET /emby/sessions/count`

- 说明：查询 Emby 活跃会话数量
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/sessions/count" \
  -H "Authorization: Bearer <token>"
```

## 10. Admin 模块

### 10.1 用户管理

#### 查询用户列表

`GET /admin/users?status=active&page=1&per_page=20`

- 说明：查询用户列表
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/users?status=active&page=1&per_page=20" \
  -H "Authorization: Bearer <admin_token>"
```

#### 获取单个用户信息

`GET /admin/users/<int:uid>`

- 说明：获取单个用户信息
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/users/123" \
  -H "Authorization: Bearer <admin_token>"
```

#### 禁用用户

`POST /admin/users/<int:uid>/disable`

- 认证：管理员 Token
- 请求体：

```json
{
  "reason": "违规使用"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/123/disable" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"reason":"违规使用"}'
```

#### 启用用户

`POST /admin/users/<int:uid>/enable`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/123/enable" \
  -H "Authorization: Bearer <admin_token>"
```

#### 续期用户

`POST /admin/users/<int:uid>/renew`

- 认证：管理员 Token
- 请求体：

```json
{
  "days": 30
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/123/renew" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"days":30}'
```

#### 踢出用户 Emby 会话

`POST /admin/users/<int:uid>/kick`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/123/kick" \
  -H "Authorization: Bearer <admin_token>"
```

#### 更新用户媒体库权限

`PUT /admin/users/<int:uid>/libraries`

- 认证：管理员 Token
- 请求体：

```json
{
  "libraries": [1, 2, 3]
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/admin/users/123/libraries" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"libraries":[1,2,3]}'
```

#### 更新用户 NSFW 权限

`PUT /admin/users/<int:uid>/nsfw`

- 认证：管理员 Token
- 请求体：

```json
{
  "enable": true
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/admin/users/123/nsfw" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"enable":true}'
```

#### 切换管理员身份

`PUT /admin/users/<int:uid>/admin`

- 认证：管理员 Token
- 请求体：

```json
{
  "admin": true
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/admin/users/123/admin" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"admin":true}'
```

#### 根据 Telegram ID 查询用户

`GET /admin/users/by-telegram/<int:telegram_id>`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/users/by-telegram/987654321" \
  -H "Authorization: Bearer <admin_token>"
```

### 10.2 Emby 管理

#### 同步所有 Emby 用户数据

`POST /admin/emby/sync`

- 认证：管理员 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/sync" \
  -H "Authorization: Bearer <admin_token>"
```

#### 获取 Emby 媒体库列表

`GET /admin/emby/libraries`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/emby/libraries" \
  -H "Authorization: Bearer <admin_token>"
```

#### 手动触发不活跃用户审查

`POST /admin/emby/review/inactive`

- 认证：管理员 Token
- 请求体：

```json
{
  "threshold_days": 21,
  "action": "disable",
  "delete_emby": false
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/review/inactive" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"threshold_days":21,"action":"disable","delete_emby":false}'
```

#### 手动触发设备使用审查

`POST /admin/emby/review/devices`

- 认证：管理员 Token
- 请求体：

```json
{
  "max_devices": 5,
  "threshold_days": 30,
  "action": "kick_oldest"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/review/devices" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"max_devices":5,"threshold_days":30,"action":"kick_oldest"}'
```

#### 获取 Emby 审查配置

`GET /admin/emby/review/settings`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/emby/review/settings" \
  -H "Authorization: Bearer <admin_token>"
```

### 10.3 规则与配置

#### 查询注册码列表

`GET /admin/regcodes`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/regcodes" \
  -H "Authorization: Bearer <admin_token>"
```

#### 创建注册码

`POST /admin/regcodes`

- 认证：管理员 Token
- 请求体：

```json
{
  "days": 30,
  "count": 1,
  "remark": "推广码"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/regcodes" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"days":30,"count":1,"remark":"推广码"}'
```

#### 删除注册码

`DELETE /admin/regcodes/<code>`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X DELETE "http://localhost:5000/api/v1/admin/regcodes/code-abc123" \
  -H "Authorization: Bearer <admin_token>"
```

#### 发送 Emby 广播消息

`POST /admin/emby/broadcast`

- 认证：管理员 Token
- 请求体：

```json
{
  "title": "系统通知",
  "message": "Emby 服务器将在夜间维护。"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/broadcast" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"title":"系统通知","message":"Emby 服务器将在夜间维护。"}'
```

#### 管理白名单

`POST /admin/whitelist`

- 认证：管理员 Token
- 请求体：

```json
{
  "ip": "192.168.1.100"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/whitelist" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"ip":"192.168.1.100"}'
```

#### 查询管理员统计

`GET /admin/stats`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/stats" \
  -H "Authorization: Bearer <admin_token>"
```

#### 测试 Emby 连通性

`POST /admin/emby/test`

- 认证：管理员 Token
- 请求体：

```json
{
  "emby_id": "user_emby_id"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/test" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"emby_id":"user_emby_id"}'
```

#### 查询 Emby 用户列表

`GET /admin/emby/users`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/emby/users" \
  -H "Authorization: Bearer <admin_token>"
```

#### 清理孤立 Emby 用户

`POST /admin/emby/cleanup-orphans`

- 认证：管理员 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/cleanup-orphans" \
  -H "Authorization: Bearer <admin_token>"
```

#### 导入 Emby 用户

`POST /admin/emby/import-users`

- 认证：管理员 Token
- 请求体：

```json
{
  "source": "emby",
  "user_ids": [123, 456]
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/import-users" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"source":"emby","user_ids":[123,456]}'
```

#### 重置绑定关系

`POST /admin/emby/reset-bindings`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/reset-bindings" \
  -H "Authorization: Bearer <admin_token>"
```

#### 删除未绑定 Emby 用户

`POST /admin/emby/delete-unlinked`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/delete-unlinked" \
  -H "Authorization: Bearer <admin_token>"
```

#### 清理无效用户

`POST /admin/users/cleanup-invalid`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/cleanup-invalid" \
  -H "Authorization: Bearer <admin_token>"
```

## 11. Stats 模块

### 当前用户统计

`GET /stats/me`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/stats/me" \
  -H "Authorization: Bearer <token>"
```

### 当前用户播放统计

`GET /stats/playback/my`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/stats/playback/my" \
  -H "Authorization: Bearer <token>"
```

### 指定用户播放统计

`GET /stats/user/<int:uid>`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/stats/user/123" \
  -H "Authorization: Bearer <token>"
```

### 用户积分排行榜

`GET /stats/ranking?limit=20`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/stats/ranking?limit=20" \
  -H "Authorization: Bearer <token>"
```

### 媒体排行榜

`GET /stats/ranking/media?limit=20`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/stats/ranking/media?limit=20" \
  -H "Authorization: Bearer <token>"
```

### 日排行榜

`GET /stats/ranking/daily?limit=20`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/stats/ranking/daily?limit=20" \
  -H "Authorization: Bearer <token>"
```

## 12. Webhook 模块

### 接收 Emby Webhook

`POST /webhook/emby`

- 说明：接收 Emby Webhook
- 认证：视配置而定，通常由 Emby 服务器调用
- 请求体示例：

```json
{
  "event": "PlaybackStart",
  "user_id": "emby_user_id",
  "item_id": "movie_123"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/emby" \
  -H "Content-Type: application/json" \
  -d '{"event":"PlaybackStart","user_id":"emby_user_id","item_id":"movie_123"}'
```

### 接收 Jellyfin Webhook

`POST /webhook/jellyfin`

- 请求体示例：

```json
{
  "event": "PlaybackStart",
  "user_id": "jellyfin_user_id"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/jellyfin" \
  -H "Content-Type: application/json" \
  -d '{"event":"PlaybackStart","user_id":"jellyfin_user_id"}'
```

### 接收自定义 Webhook

`POST /webhook/custom`

- 请求体示例：

```json
{
  "type": "custom_event",
  "payload": {"foo":"bar"}
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/custom" \
  -H "Content-Type: application/json" \
  -d '{"type":"custom_event","payload":{"foo":"bar"}}'
```

### 查询 Webhook 订阅列表

`GET /webhook/endpoints`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/webhook/endpoints" \
  -H "Authorization: Bearer <token>"
```

### 添加 Webhook 订阅

`POST /webhook/endpoints`

- 认证：登录 Token
- 请求体：

```json
{
  "url": "https://example.com/webhook",
  "events": ["PlaybackStart", "PlaybackStop"]
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/endpoints" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/webhook","events":["PlaybackStart","PlaybackStop"]}'
```

### 删除 Webhook 订阅

`DELETE /webhook/endpoints`

- 认证：登录 Token
- 请求体：

```json
{
  "url": "https://example.com/webhook"
}
```

- 示例 cURL：

```bash
curl -X DELETE "http://localhost:5000/api/v1/webhook/endpoints" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/webhook"}'
```

### 发送测试 Webhook

`POST /webhook/test`

- 认证：登录 Token
- 请求体：

```json
{
  "url": "https://example.com/webhook",
  "payload": {"test":"data"}
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/test" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/webhook","payload":{"test":"data"}}'
```

### Bangumi 与 Emby 同步事件

`POST /webhook/bangumi/emby`

- 请求体示例：

```json
{
  "bangumi_id": 123,
  "emby_id": "emby_user_id"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/emby" \
  -H "Content-Type: application/json" \
  -d '{"bangumi_id":123,"emby_id":"emby_user_id"}'
```

### Bangumi 与 Jellyfin 同步事件

`POST /webhook/bangumi/jellyfin`

- 请求体示例：

```json
{
  "bangumi_id": 123,
  "jellyfin_id": "jellyfin_user_id"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/jellyfin" \
  -H "Content-Type: application/json" \
  -d '{"bangumi_id":123,"jellyfin_id":"jellyfin_user_id"}'
```

### Bangumi 与 Plex 同步事件

`POST /webhook/bangumi/plex`

- 请求体示例：

```json
{
  "bangumi_id": 123,
  "plex_id": "plex_user_id"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/plex" \
  -H "Content-Type: application/json" \
  -d '{"bangumi_id":123,"plex_id":"plex_user_id"}'
```

### 自定义 Bangumi 同步事件

`POST /webhook/bangumi/custom`

- 请求体示例：

```json
{
  "source": "custom",
  "payload": {"id": 123, "name": "测试"}
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/custom" \
  -H "Content-Type: application/json" \
  -d '{"source":"custom","payload":{"id":123,"name":"测试"}}'
```

### 查询 Bangumi 映射

`GET /webhook/bangumi/mappings`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/webhook/bangumi/mappings" \
  -H "Authorization: Bearer <token>"
```

### 创建 Bangumi 映射

`POST /webhook/bangumi/mappings`

- 认证：登录 Token
- 请求体：

```json
{
  "bangumi_id": 123,
  "emby_id": "emby_media_id"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/mappings" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"bangumi_id":123,"emby_id":"emby_media_id"}'
```

### 删除 Bangumi 映射

`DELETE /webhook/bangumi/mappings`

- 认证：登录 Token
- 请求体：

```json
{
  "mapping_id": 456
}
```

- 示例 cURL：

```bash
curl -X DELETE "http://localhost:5000/api/v1/webhook/bangumi/mappings" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"mapping_id":456}'
```

### 导入 Bangumi 映射

`POST /webhook/bangumi/mappings/import`

- 认证：登录 Token
- 请求体：

```json
{
  "mappings": [
    {"bangumi_id":123,"emby_id":"emby_media_id"}
  ]
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/mappings/import" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"mappings":[{"bangumi_id":123,"emby_id":"emby_media_id"}]}'
```

### 导出 Bangumi 映射

`GET /webhook/bangumi/mappings/export`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/webhook/bangumi/mappings/export" \
  -H "Authorization: Bearer <token>"
```

### 触发 Bangumi 同步

`POST /webhook/bangumi/sync`

- 认证：登录 Token
- 请求体：

```json
{
  "type": "full",
  "target": "emby"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/webhook/bangumi/sync" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"type":"full","target":"emby"}'
```

### 获取 Bangumi Webhook 配置

`GET /webhook/bangumi/config`

- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/webhook/bangumi/config" \
  -H "Authorization: Bearer <token>"
```

## 13. System 模块

### 健康检查

`GET /system/health`

- 说明：健康检查
- 认证：公开

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/health"
```

### 系统信息

`GET /system/info`

- 说明：系统信息
- 认证：公开

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/info"
```

### 读取运行时配置

`GET /system/config`

- 说明：获取运行时配置
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/config" \
  -H "Authorization: Bearer <admin_token>"
```

### 读取当前 config.toml

`GET /system/admin/config/toml`

- 说明：读取当前 config.toml
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/admin/config/toml" \
  -H "Authorization: Bearer <admin_token>"
```

### 写入 config.toml 并热重载

`PUT /system/admin/config/toml`

- 说明：写入 config.toml 并热重载
- 认证：管理员 Token
- 请求体示例：

```json
{
  "config": "[Service]..."
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/system/admin/config/toml" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"config":"[Service]..."}'
```

### 获取配置 Schema

`GET /system/admin/config/schema`

- 说明：获取配置 schema
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/admin/config/schema" \
  -H "Authorization: Bearer <admin_token>"
```

### 更新配置并热重载

`PUT /system/admin/config/schema`

- 说明：更新配置并热重载
- 认证：管理员 Token
- 请求体：

```json
{
  "sections": {
    "EmbyReview": {
      "enabled": true,
      "review_time": "04:00"
    }
  }
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/system/admin/config/schema" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"sections":{"EmbyReview":{"enabled":true,"review_time":"04:00"}}}'
```

### 获取全部路由列表

`GET /system/admin/apis`

- 说明：获取全部路由列表
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/admin/apis" \
  -H "Authorization: Bearer <admin_token>"
```

### 获取 Emby 媒体库列表

`GET /system/admin/emby/libraries`

- 说明：获取 Emby 媒体库列表
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/admin/emby/libraries" \
  -H "Authorization: Bearer <admin_token>"
```

## 14. 附录

### 14.1 API Key 文档

API Key 相关接口请参考 `docs/API_KEY_API.md`。

### 14.2 说明

- 管理员接口需要管理员登录 Token。
- 外部系统推荐使用 API Key 访问 `/api/v1/apikey/*`。
- 如果配置与接口行为不一致，以后端 Swagger 为准。
