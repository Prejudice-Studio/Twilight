# Twilight 后端 API 文档

本文档为 Twilight 后端 API 的统一参考指南，覆盖认证、请求格式、错误码、核心接口与管理员功能。完整路由清单见 [API_INDEX.md](./API_INDEX.md)，API Key 专用接口见 [API_KEY_API.md](./API_KEY_API.md)，注册码规则见 [REGCODES.md](./REGCODES.md)。

## 1. 文档说明

- Base URL：`http://localhost:5000/api/v1`
- Swagger UI：`http://localhost:5000/api/v1/docs`
- 格式：JSON 响应结构为 `success` + `message` + `data` + `timestamp`
- 说明：接口变更优先以 Swagger、[API_INDEX.md](./API_INDEX.md) 和后端实际返回为准。

### 1.1 文档分工

| 文档 | 用途 |
| ---- | ---- |
| [BACKEND_API.md](./BACKEND_API.md) | 通用规范、认证、错误码、重点接口说明 |
| [API_INDEX.md](./API_INDEX.md) | `/api/v1` 完整路由索引、认证级别、归属模块 |
| [API_KEY_API.md](./API_KEY_API.md) | 外部 API Key 接入方式、权限矩阵、专用示例 |
| [REGCODES.md](./REGCODES.md) | 注册码/续期码/白名单码规则、兼容性与安全口径 |
| `/api/v1/docs` | 运行时 Swagger UI，按当前代码自动生成 |

## 2. 认证与请求规范

> 说明：`/api/v1/apikey` 相关的权限矩阵与完整调用示例已拆分到 [API_KEY_API.md](./API_KEY_API.md)。本文件保留通用规范与后端主接口。

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
| ---------- | ---- |
| 200 | 请求成功 |
| 400 | 参数错误 / 请求格式不合法 |
| 401 | 未认证 / Token 或 API Key 无效 |
| 403 | 权限不足 / 账号或 API Key 被禁用 |
| 404 | 资源不存在 |
| 410 | 端点已弃用（如 `/emby/urls`，请改用文档指明的替代接口） |
| 429 | 触发限流；响应 `message` 形如 `请求过于频繁，请在 N 秒后重试` |
| 500 | 服务器内部错误 |

### 3.1 速率限制

为了防止暴力破解与公开端点被刷，部分接口启用了基于 IP / UID / 资源 key 的滑动窗口限流（实现：`internal/api/ratelimit.go`）。被限流时返回 HTTP `429`。

| 接口 | 维度 | 阈值 | 说明 |
| ---- | ---- | ---- | ---- |
| `POST /auth/login` 系列 | IP | 滑动窗口 | 登录接口按客户端 IP 限速，详见 `internal/api/handlers.go::handleLogin` |
| `POST /auth/forgot-password/emby` | IP + Emby 用户名 | 5/10 分钟 + 5/30 分钟 | 验证 Emby 账号密码后重置 Web 密码；新密码只返回一次 |
| `POST /users/register` | IP | 5 / 10 分钟 | 防批量注册 |
| `GET  /users/check-available` | IP | 60 / 60 秒 | 防扫描可用用户名 |
| `GET  /users/register/emby/status` | request_id + IP | 60/60s + 240/60s | Emby 注册队列轮询 |
| `POST /users/telegram/register/bind-code` | IP | 5 / 10 分钟 | 生成注册绑定码 |
| `GET  /users/telegram/register/bind-code/status` | code + IP + 404 防御 | 90/60s + 600/60s + 失效缓存 + IP 404 封禁 | 注册绑定码轮询；详见下方 |
| `POST /users/me/telegram/bind-code` | UID | 5 / 10 分钟 | 已登录用户生成 TG 绑定码 |
| `POST /users/me/telegram/unbind` | UID | 5 / 10 分钟 | 防恶意频繁解绑 |
| `POST /users/me/telegram/rebind-request` | UID | 3 / 1 小时 | 换绑申请会进管理员队列，从严限制 |
| `POST /users/regcode/check` | IP | 10 / 60 秒 | 防注册码枚举 |
| `POST /invite/check` | IP | 10 / 60 秒 | 邀请码校验 |
| `POST /users/me/avatar/upload` 等上传 | UID | 10 / 60 秒 | 防滥用上传 |

上传后的头像/背景通过 `GET /users/assets/{avatar|background}/{filename}` 读取。该接口要求登录，并校验资源类型、服务端生成的文件名格式和最终文件路径；不要直接公开 `/uploads` 目录。

> 限速命中只写 `logger.warning`，不会写入 SecurityLog / login_history。

#### `bind-code/status` 的三层防御

线上观测到攻击者会盯着一个 8 位 code 反复刷（每次都是 404），所以这个端点
在普通双层限速之上又加了两层（实现：`internal/api/handlers.go`）：

1. **失效 code 短路缓存**：第一次 DB 查不到该 code 时，写入 `_INVALID_CODE_CACHE`（TTL 5 分钟）。期间同 code 任何请求 → 直接 404，**不查 DB、不消费 code 维度限速配额**。
2. **IP 累计 404 封禁**：同 IP 60s 内累计 ≥60 次 404（包括短路命中的）→ 把 IP 加入 `_IP_404_BAN`（5 分钟）。期间该 IP 任何请求 → 直接 429。
3. **前端配合**：`webui/src/app/(auth)/register/page.tsx` 的轮询 `useEffect` 收到 message 含"无效/过期/不存在"或"IP 已被/429/请求过于频繁"时立即停止轮询，避免合法用户被误锁。

状态都是进程内存的，重启清零。多 worker 部署每个进程独立计数，但攻击者 keep-alive 会粘在同一进程上，足以拦住。

## 4. 模块总览

> 完整端点列表见 [API_INDEX.md](./API_INDEX.md)。本节只保留模块边界和维护口径。

| 模块 | 路径前缀 | 说明 |
| ---- | -------- | ---- |
| Auth | `/auth` | 登录、会话、Token 刷新、API Key 管理 |
| Users | `/users` | 注册、个人信息、Emby 绑定、续期、设备、Telegram |
| Media | `/media` | TMDB/Bangumi 搜索、求片、库存管理 |
| Emby | `/emby` | Emby 账号状态、库、搜索、会话 |
| Admin | `/admin` | 管理用户、Emby 同步、注册码、广播、定时任务 |
| Stats | `/stats` | 播放统计 |
| System | `/system` | 健康、系统信息、配置、路由列表 |
| API Key | `/apikey` | 外部系统专用 API Key 接口 |
| Demo | `/demo` | TestWeb 演示专用模拟接口，只返回假数据 |

### 4.1 TestWeb 演示接口

`/api/v1/demo/*` 仅供 `/testweb`、`/testwebuser`、`/testwebadmin` 演示页面使用。

- 认证：公开，不读取登录态。
- 数据：全部为后端静态预设假数据，不读取数据库、不读取登录态、不调用真实业务服务。
- 写操作：统一返回 `simulated=true`、`readonly=true`、`mutated=false`，忽略请求体，不回显用户输入，不写数据库、不调用 Emby、不调用 Telegram。
- 动作名：`/demo/action/{action_name}` 只接受安全白名单字符，拒绝路径片段、控制字符和 URL 编码绕过。
- 响应：统一带 `Cache-Control: no-store` 和 `X-Twilight-Demo: true`，避免缓存演示响应。
- 生产建议：可通过反向代理限制公开访问，避免访客误以为是真实后台。

### 4.2 命名与归属约定

| 场景 | 约定 |
| ---- | ---- |
| 当前登录用户 | 使用 `/users/me/*`，不要新增 `/user/current/*` 一类别名 |
| 管理用户 | 使用 `/admin/users/*` |
| 系统配置管理 | 使用 `/system/admin/config/*` |
| 定时任务管理 | 使用 `/admin/scheduler/*` |
| 用户可见系统信息 | 使用 `/system/info` 或 `/system/config` |
| Emby 线路下发 | 只使用 `/system/emby-urls`，按登录用户角色和 Emby 绑定状态判断 |
| 上传头像/背景读取 | 只使用 `/users/assets/{avatar|background}/{filename}` |
| 外部系统 API Key 调用 | 使用 `/apikey/*`，不混用登录 Token |
| 废弃接口 | 保留时返回明确错误和替代路径，例如 `/emby/urls` 返回 410 |

新增或修改接口时，需要同步更新 [API_INDEX.md](./API_INDEX.md)，如果接口有请求体、响应体、限流或安全注意事项，还需要更新本文对应章节。

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
  "permissions": ["account:read", "emby:read"]
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/auth/apikey/permissions" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"permissions":["account:read","emby:read"]}'
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
  "bgm_mode": true,
  "bgm_token": "new-bgm-token"
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/users/me" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"email":"updated@example.com","bgm_mode":true,"bgm_token":"new-bgm-token"}'
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

### 6.4 续期与注册码/续期码

注册码、续期码、白名单码的类型、生成格式、旧码兼容与安全口径见 [REGCODES.md](./REGCODES.md)。

#### 管理员续期用户

`POST /users/me/renew`

- 说明：使用续期码续期账号
- 认证：登录 Token
- 请求体：

```json
{
  "reg_code": "code-abc123",
  "emby_username": "emby_name",
  "emby_password": "Password123"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/renew" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-abc123"}'
```

#### 使用注册码 / 续期码

`POST /users/me/use-code`

- 说明：统一使用注册码 / 续期码 / 白名单码
- 认证：登录 Token
- 请求体：

```json
{
  "reg_code": "code-abc123",
  "emby_username": "emby_name",
  "emby_password": "Password123"
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/use-code" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-abc123"}'
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

## 7. Media 模块

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

## 8. Emby 模块

### 查询当前用户 Emby 状态

`GET /emby/status`

- 说明：查询当前用户 Emby 状态
- 认证：登录 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/emby/status" \
  -H "Authorization: Bearer <token>"
```

### 获取 Emby 服务 URLs（已弃用）

`GET /emby/urls`

- 状态：**已弃用，固定返回 `410 Gone`**。该端点早期为未鉴权返回全量线路，存在泄露风险。
- 替代：改用 `GET /system/emby-urls`，按用户角色和 Emby 绑定状态下发线路（未绑定 Emby 的普通用户返回空列表）。

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

## 9. Admin 模块

### 9.1 用户管理

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

#### 获取用户媒体库权限

`GET /admin/users/<int:uid>/libraries`

- 认证：管理员 Token
- 响应：

```json
{
  "all_libraries": [{"id": "xxx", "name": "电影", "type": "movies"}],
  "enabled_ids": ["xxx", "yyy"],
  "enable_all": false,
  "has_emby": true
}
```

#### 更新用户媒体库权限

`PUT /admin/users/<int:uid>/libraries`

- 认证：管理员 Token
- 说明：设置用户可访问的媒体库。`enable_all=true` 时用户可见所有库；
  否则仅 `library_names` / `library_ids` 列出的库可见。
  写入由 `apply_library_policy` 走单次 `POST /Users/{Id}/Policy` 完成。
- 请求体（推荐使用 `library_names`）：

```json
{
  "library_names": ["电影", "电视剧"],
  "enable_all": false
}
```

或按 ID（GUID）传入：

```json
{
  "library_ids": ["f137a2dd-21bb-c1b9-9aa5-c0f6bf02a805"],
  "enable_all": false
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/admin/users/123/libraries" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"library_names":["电影","电视剧"],"enable_all":false}'
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

### 9.2 Emby 管理

#### 同步所有 Emby 用户数据

`POST /admin/emby/sync`

- 认证：管理员 Token
- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/sync" \
  -H "Authorization: Bearer <admin_token>"
```

#### 获取 Emby 媒体库列表（Admin 模块）

`GET /admin/emby/libraries`

- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/emby/libraries" \
  -H "Authorization: Bearer <admin_token>"
```

### 9.3 规则与配置

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
  "type": 1,
  "validity_time": -1,
  "use_count_limit": 1,
  "days": 30,
  "count": 1,
  "format": "TW-{type}-{random}",
  "random_algorithm": "base32-20",
  "decoy": false
}
```

- 示例 cURL：

```bash
curl -X POST "http://localhost:5000/api/v1/admin/regcodes" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"type":1,"validity_time":-1,"use_count_limit":1,"days":30,"count":1}'
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

### 9.4 定时任务管理

定时任务持久化分为两类表：

- 固定信息 / 计划信息：`db/scheduler_schedule.db` 的 `scheduler_schedule` 表，记录任务名称、描述、是否手动任务、当前触发器、默认触发器、下次执行时间、最近自动/手动执行时间等。
- 执行信息 / 状态日志：`db/scheduler_run.db` 的 `scheduler_run` 表，记录每次执行的状态、日志、summary、开始/结束时间。

每个 `job_id` 的执行历史默认保留最近 50 条记录；超出后自动按 ID 升序裁剪。

进程启动时会调用 `reconcile_orphans()`：把所有起始于 6 小时前仍处于 `running`
状态的记录改判为 `failed`（避免崩溃后前端永远转圈）。

`GET /admin/scheduler/jobs` 返回的每个 job 额外包含触发器结构化描述：

| 字段                   | 说明                                                            |
| ---------------------- | --------------------------------------------------------------- |
| `trigger_spec`         | 当前生效的触发规则，结构同上述 PUT 请求体                       |
| `default_trigger_spec` | config.toml 算出的默认值（用于"恢复默认"按钮显示）              |
| `is_custom`            | 是否已被管理员覆盖（true 时前端显示"已自定义"徽章）             |
| `last_auto_run_at`     | 最近一次自动执行开始时间                                         |
| `last_manual_run_at`   | 最近一次手动执行开始时间                                         |

`SchedulerJobRun` 字段：

| 字段          | 说明                                                                  |
| ------------- | --------------------------------------------------------------------- |
| `id`          | 数据库主键                                                            |
| `job_id`      | 任务标识（如 `check_expired`、`emby_sync`）                           |
| `type`        | 执行类型：`auto` / `manual`                                           |
| `trigger`     | 触发来源：`scheduled` / `manual` / `startup`                          |
| `status`      | `running` / `success` / `failed`                                      |
| `started_at`  | 起始时间戳（秒）                                                      |
| `finished_at` | 结束时间戳（秒），运行中为 `null`                                     |
| `error`       | 失败时的异常摘要（最长 1000 字符）                                    |
| `summary`     | 结构化指标，如 `{"scanned": 12, "disabled": 3, "failed": 0}`           |
| `logs`        | 任务内部 `ctx.log()` 累积的日志行（list；列表接口不返回，详情接口返回） |

#### 列出全部定时任务

`GET /admin/scheduler/jobs`

- 说明：列出内置定时任务定义、计划时间、下次执行时间和最近一次运行摘要（不含 logs 正文，体积小适合轮询）。
- 认证：管理员 Token
- 响应示例：

```json
{
  "success": true,
  "data": {
    "jobs": [
      {
        "id": "check_expired",
        "name": "过期用户检查",
        "description": "...",
        "enabled": true,
        "schedule": "cron[hour='4', minute='0']",
        "next_run_at": 1715990400,
        "is_running": false,
        "last_run": {
          "status": "success",
          "started_at": 1715904000,
          "finished_at": 1715904005,
          "type": "auto",
          "trigger": "scheduled",
          "error": null,
          "summary": {"scanned": 12, "disabled": 3, "failed": 0}
        }
      }
    ]
  }
}
```

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/admin/scheduler/jobs" \
  -H "Authorization: Bearer <admin_token>"
```

#### 手动触发一次任务

`POST /admin/scheduler/jobs/<job_id>/run`

- 说明：将指定任务排入后台执行；接口立即返回，前端通过轮询 `/admin/scheduler/jobs` 拿到结束状态。
- 认证：管理员 Token
- 响应：`data.last_run` 是触发时的快照（通常为 `running`）。

```bash
curl -X POST "http://localhost:5000/api/v1/admin/scheduler/jobs/check_expired/run" \
  -H "Authorization: Bearer <admin_token>"
```

#### 获取最近一次完整运行（含日志）

`GET /admin/scheduler/jobs/<job_id>/last-run`

- 说明：返回包含 `logs` 正文的完整 `SchedulerJobRun`。前端"查看日志"弹窗使用。
- 认证：管理员 Token

```bash
curl -X GET "http://localhost:5000/api/v1/admin/scheduler/jobs/emby_sync/last-run" \
  -H "Authorization: Bearer <admin_token>"
```

#### 获取历史运行列表

`GET /admin/scheduler/jobs/<job_id>/history?limit=20`

- 说明：按时间倒序返回历史运行；`limit` 范围 1–100，默认 20。
- 认证：管理员 Token

```bash
curl -X GET "http://localhost:5000/api/v1/admin/scheduler/jobs/emby_sync/history?limit=20" \
  -H "Authorization: Bearer <admin_token>"
```

#### 修改触发器（覆盖 config.toml 默认值）

`PUT /admin/scheduler/jobs/<job_id>/schedule`

- 说明：把新的触发规则写入 `db/scheduler_schedule.db`，并实时 `reschedule_job`；
  下次进程重启后仍生效。每个 `job_id` 至多一条覆盖。
- 认证：管理员 Token
- 请求体（二选一）：

```json
{ "type": "cron_daily", "hour": 3, "minute": 0 }
```

```json
{ "type": "interval", "seconds": 3600 }
```

- 约束：
  - `cron_daily`：`hour ∈ [0, 23]`，`minute ∈ [0, 59]`
  - `interval`：`seconds ∈ [60, 604800]`（最短 1 分钟，最长 7 天）

- 响应示例：

```json
{
  "success": true,
  "data": {
    "job_id": "emby_sync",
    "trigger_spec": { "type": "interval", "seconds": 1800 },
    "is_custom": true
  }
}
```

#### 重置触发器（恢复 config.toml 默认值）

`DELETE /admin/scheduler/jobs/<job_id>/schedule`

- 说明：删除该 job 的覆盖记录，并按 `default_trigger_spec` 重新 `reschedule_job`。
- 认证：管理员 Token

```bash
curl -X DELETE "http://localhost:5000/api/v1/admin/scheduler/jobs/emby_sync/schedule" \
  -H "Authorization: Bearer <admin_token>"
```

## 10. Stats 模块

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

## 11. System 模块

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

- 说明：系统公开信息。`icon` 固定为内置 `/favicon.svg`；`telegram_bot` 会在 Telegram 已启用且可连通时返回 Bot 用户名；`telegram_links.groups/channels` 只返回通过白名单校验的公开 `t.me` 链接，不暴露纯数字私有群 ID。
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

### 获取 Emby 服务线路（按角色下发）

`GET /system/emby-urls`

- 说明：替代已弃用的 `/emby/urls`。后端会根据当前用户的角色与 Emby 绑定状态过滤：
  - **未绑定 Emby 的普通用户**：返回空 `lines`，响应体附带 `requires_emby_account: true`，前端据此隐藏整块线路 UI，避免在用户尚未持有 Emby 账号时先把服务器地址泄露给浏览器。
  - **已绑定 Emby 的普通用户**：返回普通线路列表 `lines`。
  - **白名单 / 管理员**：额外返回 `whitelist_lines` 专属线路。
- 认证：登录 Token
- 响应示例：

```json
{
  "success": true,
  "data": {
    "lines": [
      {"name": "Direct", "url": "https://emby.example.com"},
      {"name": "CDN",    "url": "https://cdn.example.com"}
    ],
    "whitelist_lines": [
      {"name": "Premium", "url": "https://vip.example.com"}
    ]
  }
}
```

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/emby-urls" \
  -H "Authorization: Bearer <token>"
```

### 管理员运行状态与实时日志

`GET /system/admin/runtime/status`

- 说明：返回 Go 进程、主机、内存、数据库、路由数和 Redis 状态，供管理端实时状态卡片使用。
- 认证：管理员 Token。
- 安全口径：只读取进程内统计和 Linux `/proc` 汇总信息，不暴露环境变量、命令行参数、配置明文或任意文件内容。

响应 `data` 主要字段：

| 字段 | 说明 |
| ---- | ---- |
| `started_at` | Go 进程启动时间戳 |
| `uptime_seconds` | Go 进程运行秒数 |
| `host_uptime_seconds` | Linux 主机运行秒数，可用时返回 |
| `go_version` / `goos` / `goarch` | Go 运行时与平台 |
| `goroutines` / `cpu_count` | 协程数和 CPU 数 |
| `active_database` / `config_database` | 当前存储后端与配置中的目标后端 |
| `memory` | Go runtime 内存统计 |
| `host_memory` | `/proc/meminfo` 摘要，可用时返回 |
| `load_average` | `/proc/loadavg` 的 1/5/15 分钟负载，可用时返回 |

`GET /system/admin/runtime/logs?limit=200&after=0`

- 说明：读取后端进程内最近日志快照，`limit` 范围 1-1000。
- 认证：管理员 Token。
- 响应：`entries` 为日志数组，`next_cursor` 用于下一次增量读取。

`GET /system/admin/runtime/logs/stream?limit=100&after=0`

- 说明：SSE 实时日志流，事件类型包括 `snapshot`、`logs`、`ping`。
- 认证：管理员 Token；浏览器端使用同源 Cookie 或显式可信 CORS Origin。
- 安全口径：日志来自 Go 进程内 `slog` 处理链，敏感键、API Key、Bearer Token、Cookie、密码、DSN 会被尽力脱敏；接口不读取 journald、系统日志文件或用户指定路径。

示例：

```bash
curl -N "http://localhost:5000/api/v1/system/admin/runtime/logs/stream?limit=100" \
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

### 写入 config.toml（保存后热重载）

`PUT /system/admin/config/toml`

- 说明：写入 config.toml；保存前会创建备份，保存后会热重载可在线生效字段。监听端口、数据库 driver 等启动期字段仍需重启进程。
- 认证：管理员 Token
- 请求体：

```json
{
  "content": "[Global]\nlogging = true\n..."
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/system/admin/config/toml" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"content":"[Global]\nlogging = true\n..."}'
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

### 按 Schema 更新配置

`PUT /system/admin/config/schema`

- 说明：按结构化 schema 写入配置；保存前会创建备份，保存后会热重载可在线生效字段。
- 认证：管理员 Token
- 请求体：

```json
{
  "sections": {
    "BangumiSync": {
      "enabled": true,
      "min_progress_percent": 80
    }
  }
}
```

- 示例 cURL：

```bash
curl -X PUT "http://localhost:5000/api/v1/system/admin/config/schema" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"sections":{"BangumiSync":{"enabled":true,"min_progress_percent":80}}}'
```

### 数据库状态、备份、恢复、迁移

`GET /system/admin/database/status`

- 说明：返回当前 active driver、配置 driver、状态文件、备份目录、PostgreSQL 配置状态和用户数。
- 认证：管理员 Token

`GET /system/admin/database/backups`

- 说明：列出备份目录中的数据库快照。
- 认证：管理员 Token

`POST /system/admin/database/backup`

- 说明：创建当前数据库快照备份。
- 认证：管理员 Token

`POST /system/admin/database/restore`

- 说明：从指定备份恢复。未传确认短语时只返回预览，不写入数据；确认执行前会自动创建保护性备份。备份名会限制在配置的备份目录内。
- 认证：管理员 Token
- 预览请求体：

```json
{"name":"twilight_state_20260522_120000_123456789.json","dry_run":true}
```

- 执行请求体：

```json
{"name":"twilight_state_20260522_120000_123456789.json","confirm":"RESTORE_DATABASE_BACKUP"}
```

`POST /system/admin/database/migrate`

- 说明：迁移当前 Go 状态快照或旧 SQLite 文件集到 `json` / `postgres`。未传确认短语时只返回预检，不写入数据；确认执行前会自动创建保护性备份。
- 认证：管理员 Token
- 预检请求体：

```json
{
  "source_driver": "sqlite",
  "target_driver": "postgres",
  "dry_run": true,
  "database_url": "postgres://user:pass@127.0.0.1:5432/twilight?sslmode=disable"
}
```

`source_driver` 可省略，省略时表示当前 Go 状态；传 `sqlite` / `legacy_sqlite` 时，后端只扫描固定数据库目录中的旧 SQLite 文件，不接受前端传入任意路径。

- 执行请求体：

```json
{
  "target_driver": "postgres",
  "confirm": "MIGRATE_DATABASE"
}
```

- 预检响应 `data` 包含 `source_driver`、`configured_driver`、`target_driver`、`snapshot_bytes`、`target_ready`、`backup_ready`、`warnings`、`counts`、`requires_confirmation`、`confirm`，并保留 `users`、`regcodes`、`invite_codes` 等兼容字段。PostgreSQL 目标会在权限允许时自动创建缺失数据库并准备 `twilight_state` 状态表，`target_ready.database_created` / `target_ready.schema_ready` 会反映结果。
- 旧 SQLite 来源会额外返回 `legacy_sqlite` 与 `legacy_sqlite_import`，其中包含检测到的文件、表计数、已映射表和未映射表。
- 执行响应会额外返回 `pre_operation_backup` / `pre_migration_backup`；旧 SQLite 来源还会返回 `legacy_sqlite_backup`，用于确认写入前已自动备份旧文件集。

### Git 自动更新

`POST /system/admin/update`

- 说明：从 HTTPS Git 仓库拉取指定分支。默认拒绝 dirty worktree，使用 `git pull --ff-only`，不会 reset/rebase/merge。
- 认证：管理员 Token
- 请求体：

```json
{
  "repo_url": "https://github.com/Prejudice-Studio/Twilight.git",
  "branch": "golang",
  "dry_run": true,
  "restart_services": true
}
```

- 安全约束：仓库 URL 不允许携带凭据；分支名只允许安全字符；`dry_run` 只做预检；响应中 `repo_url` 与 `before.remote_url` 会移除凭据。
- 重启策略：只有 commit 实际变化且请求 `restart_services=true` 时才调度重启；优先使用 `systemd-run --on-active=2` 延迟重启 `twilight`、`twilight-bot`、`twilight-scheduler`，失败时回退为后台 `systemctl restart`。
- 响应字段：`updated` 表示 commit 是否变化，`restart_requested` 表示请求是否要求重启，`restart_scheduled` 表示是否成功安排重启，`restart_method` 表示使用的调度方式。

### 测试 Telegram Bot 连通性

`POST /system/admin/bot/test`

- 说明：通过独立 `httpx` 直接调用 Telegram Bot HTTP API（`getMe` + `sendMessage`），
  不复用全局运行的 Bot 实例，避免跨事件循环异常。
- 认证：管理员 Token
- 请求体（可选）：

```json
{
  "target": "@my_channel"
}
```

不传 `target` 时会发到所有配置的 `Telegram.group_id` / `Telegram.channel_id`。

- 响应示例：

```json
{
  "success": true,
  "message": "测试完成",
  "data": {
    "bot": {"id": 12345, "username": "MyBot", "first_name": "My Bot"},
    "results": [{"target": "@my_channel", "success": true, "error": null}]
  }
}
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

### 获取 Emby 媒体库列表（System 模块）

`GET /system/admin/emby/libraries`

- 说明：获取 Emby 媒体库列表
- 认证：管理员 Token

- 示例 cURL：

```bash
curl -X GET "http://localhost:5000/api/v1/system/admin/emby/libraries" \
  -H "Authorization: Bearer <admin_token>"
```

## 12. 附录

### 12.1 API Key 文档

API Key 相关接口请参考 `docs/API_KEY_API.md`。

### 12.2 说明

- 管理员接口需要管理员登录 Token。
- 外部系统推荐使用 API Key 访问 `/api/v1/apikey/*`。
- 如果配置与接口行为不一致，以后端 Swagger 为准。
