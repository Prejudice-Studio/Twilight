# 后端 API 详参

本文是 Twilight 后端 REST API 的完整参考手册，覆盖鉴权方式、请求/响应规范、错误码、限流策略，以及各业务模块下逐个端点的请求体、响应示例与 cURL 调用。完整路由速查见 [API 路由索引](../reference/api-index.md)，API Key 外部接入见 [API Key 外部接入](../reference/api-key.md)，注册码与卡码规则见 [注册码与卡码](../features/regcodes.md)，安全机制（CORS、限流、脱敏等）见 [安全加固](../guides/security.md)。

> 播放统计实现参考 `ichinosekotomi11/Twilight`：Emby ActivityLog 同步会配对 `playback.start` / `playback.stop` 并幂等写入 `store.PlaybackRecords`；播放统计接口聚合这些持久化记录。在线人数只统计 `/Sessions` 中存在 `NowPlayingItem` 的正在播放会话。

> 路由的鉴权级别、方法与路径以 `internal/api/routes.go` 为准；本文已据此校准。接口若与下方示例冲突，以后端实际行为与运行时 Swagger 为准。

## 1. 文档说明

- Base URL：`http://localhost:5000/api/v1`
- OpenAPI 文档：`GET /api/v1/openapi.json`
- Swagger UI：`http://localhost:5000/api/v1/docs`
- 响应统一为 JSON 信封（envelope），结构见下文 [2.4 响应结构](#24-响应结构)。
- 变更接口时需同步更新 [API 路由索引](../reference/api-index.md)；若接口有请求体、响应体、限流或安全注意事项，还需更新本文对应章节。

### 1.1 文档分工

| 文档 | 用途 |
| ---- | ---- |
| 本文（`backend-api.md`） | 通用规范、鉴权、错误码、各端点请求/响应说明 |
| [API 路由索引](../reference/api-index.md) | `/api/v1` 完整路由清单、鉴权级别、归属模块 |
| [API Key 外部接入](../reference/api-key.md) | 外部系统 API Key 接入方式、权限矩阵、专用示例 |
| [注册码与卡码](../features/regcodes.md) | 注册码 / 续期码 / 白名单码规则、兼容性与安全口径 |
| `/api/v1/docs` | 运行时 Swagger UI，按当前代码自动生成 |

## 2. 鉴权与请求规范

> `/api/v1/apikey/*` 前缀接口的权限矩阵与完整调用示例已拆分到 [API Key 外部接入](../reference/api-key.md)。本文保留通用规范与主接口。

### 2.1 鉴权级别

后端在 `internal/api/routes.go` 中为每条路由声明一个鉴权级别（`AuthLevel`），由 `internal/api/app.go` 的调度器在分发前统一校验：

| 级别 | 含义 | 凭据来源 |
| ---- | ---- | ---- |
| `AuthPublic` | 免登录，任何人可访问 | 无 |
| `AuthUser` | 已登录用户 | 登录会话 Cookie 或 `Authorization: Bearer <token>` |
| `AuthAdmin` | 已登录且角色为管理员（`Role == RoleAdmin`） | 同 `AuthUser`，但额外校验管理员角色 |
| `AuthAPIKey` | 外部 API Key | `X-API-Key`，或 `Authorization: ApiKey/Bearer <key>`，或查询参数 `?apikey=` |

本文各端点标注的"认证"即对应上述级别。`AuthAdmin` 路由已在调度器层拦住非管理员；部分共用 handler 还会做二次 `requireAdmin` 断言（belt-and-suspenders）。

### 2.2 鉴权方式

#### 登录会话 / Bearer Token（前端）

前端登录后，既可使用会话 Cookie（浏览器自动携带），也可在请求头携带 Bearer Token：

```http
Authorization: Bearer <token>
```

#### API Key（外部系统）

`/api/v1/apikey/*` 接口仅支持 API Key 鉴权，不接受普通登录 Token。支持以下任一形式：

```http
X-API-Key: <api_key>
```

```http
Authorization: Bearer <api_key>
```

```http
Authorization: ApiKey <api_key>
```

```http
GET /api/v1/apikey/status?apikey=<api_key>
```

### 2.3 浏览器写请求

Cookie 鉴权的变更类请求（`POST` / `PUT` / `DELETE`）不要求 CSRF 令牌，也不做额外来源校验。后端只校验有效登录会话、Bearer Token 或 API Key。

`X-Twilight-Client: webui` 仅作为前端请求识别与 CORS 允许头保留，不参与鉴权。少数有副作用的 `GET`（例如绑定码创建）还要求 `X-Twilight-Intent: create-bind-code` 显式声明操作意图，用于拦截浏览器预取、链接探测或代理误触发。

双子域部署时，如需两个子域都能携带同一登录会话，应设置 `session_cookie_domain` 让前端站点与 API 站点共享 `HttpOnly` session cookie。生产环境的凭据型 CORS 仍必须显式列出可信 HTTPS Origin，不能使用 `*`。WebUI 登录态以浏览器请求 `/users/me` 的后端响应为准。

更多机制说明见 [安全加固](../guides/security.md)。

### 2.4 响应结构

所有响应统一为 JSON 信封，包含 `success`、`code`、`message`、`data`、`timestamp` 字段；失败时还会带一个字符串 `error_code`（业务级错误码）。

| 字段 | 类型 | 说明 |
| ---- | ---- | ---- |
| `success` | bool | 是否成功 |
| `code` | int | HTTP 状态码（与响应行状态一致） |
| `error_code` | string | 仅失败时出现，业务级错误码，如 `UNAUTHORIZED`、`USER_USERNAME_TAKEN`（见下文错误码表） |
| `message` | string | 人类可读消息（失败消息已做敏感信息脱敏） |
| `data` | object/null | 业务数据，成功且无数据时可省略 |
| `timestamp` | int | 服务端 Unix 时间戳（秒） |

成功示例：

```json
{
  "success": true,
  "code": 200,
  "message": "操作成功",
  "data": { },
  "timestamp": 1680000000
}
```

失败示例：

```json
{
  "success": false,
  "code": 401,
  "error_code": "UNAUTHORIZED",
  "message": "未认证，请先登录",
  "data": null,
  "timestamp": 1680000000
}
```

## 3. 错误码

### 3.1 HTTP 状态码

| HTTP 状态码 | 含义 |
| ---------- | ---- |
| 200 | 请求成功 |
| 201 | 创建成功（如注册） |
| 400 | 参数错误 / 请求格式不合法 |
| 401 | 未认证 / Token 或 API Key 无效 |
| 403 | 权限不足 / 账号或 API Key 被禁用 |
| 404 | 资源不存在 |
| 409 | 冲突（如用户名已被占用、绑定冲突） |
| 410 | 端点已弃用（如 `/emby/urls`，请改用文档指明的替代接口） |
| 413 | 请求体过大 |
| 429 | 触发限流；响应 `message` 形如 `请求过于频繁，请在 N 秒后重试` |
| 500 | 服务器内部错误 |

### 3.2 业务级错误码（error_code）

失败响应除 HTTP 状态码外，还在 `error_code` 字段返回业务级错误码（定义于 `internal/api/errcode.go`），前端据此区分场景。未显式指定时，按 HTTP 状态自动推导（如 401→`UNAUTHORIZED`、404→`NOT_FOUND`、429→`RATE_LIMITED`，未知 5xx→`INTERNAL_ERROR`）。常见值：

| error_code | 触发场景 |
| ---------- | -------- |
| `AUTH_LOGIN_INVALID` | 用户名或密码错误 |
| `AUTH_LOGIN_RATE_LIMITED` | 登录限流 |
| `AUTH_ACCOUNT_DISABLED` | 账号被禁用 |
| `AUTH_ACCOUNT_EXPIRED` | 账号已过期 |
| `AUTH_APIKEY_INVALID` | API Key 无效 |
| `AUTH_PASSWORD_OLD_MISMATCH` | 修改密码时原密码不符 |
| `AUTH_PASSWORD_WEAK` | 新密码强度不足 |
| `USER_REGISTER_RATE_LIMITED` | 注册限流 |
| `USER_USERNAME_INVALID` / `USER_USERNAME_TAKEN` | 用户名不合法 / 已被占用 |
| `USER_NOT_FOUND` | 用户不存在 |
| `USER_LIMIT_REACHED` | 系统用户数达上限 |
| `TG_BIND_REQUIRED` | 需要先绑定 Telegram |
| `TG_BIND_CODE_*` | 绑定码格式 / 过期 / 未确认 / 场景错误 |
| `TG_ALREADY_BOUND` | Telegram 已被绑定 |
| `EMBY_AUTH_FAILED` | Emby 账号校验失败 |
| `EMBY_ACCOUNT_UNLINKED` | 当前用户未绑定 Emby |
| `EMBY_CAPACITY_REACHED` | Emby 用户数达上限 |
| `CODE_EMPTY` / `CODE_INVALID` | 卡码为空 / 无效或过期 |
| `SCHEDULER_JOB_NOT_FOUND` / `SCHEDULER_JOB_RUNNING` | 定时任务不存在 / 正在运行 |
| `UPDATE_*` | Git 自动更新相关（仓库 / 分支 / git 缺失 / 重启失败等） |
| `RATE_LIMITED` | 通用限流兜底 |
| `INTERNAL_ERROR` | 服务端内部错误 |

### 3.3 速率限制

为防止暴力破解与公开端点被刷，部分接口启用基于 IP / UID / 资源 key 的滑动窗口限流（实现：`internal/api/ratelimit.go`）。被限流时返回 HTTP `429`。

| 接口 | 维度 | 阈值 | 说明 |
| ---- | ---- | ---- | ---- |
| `POST /auth/login` 系列 | IP | 滑动窗口 | 登录接口按客户端 IP 限速，详见 `internal/api/handlers.go::handleLogin` |
| `POST /auth/forgot-password/emby` | IP + Emby 用户名 | 5/10 分钟 + 5/30 分钟 | 验证 Emby 账号密码后重置 Web 密码；新密码只返回一次 |
| `POST /users/register` | IP | 5 / 10 分钟 | 防批量注册 |
| `GET  /users/check-available` | IP | 60 / 60 秒 | 防扫描可用用户名 |
| `GET  /users/register/emby/status` | request_id + IP | 60/60s + 240/60s | Emby 注册队列轮询 |
| `GET  /users/telegram/register/bind-code` | IP | 5 / 10 分钟 | 生成注册绑定码；需 `X-Twilight-Intent: create-bind-code` |
| `GET  /users/telegram/register/bind-code/status` | code + IP | 10s timeout / 进程内状态 | 注册绑定码 GET fallback 查询；主流程使用 WebSocket |
| `GET  /users/telegram/register/bind-code/ws` | IP | 30/min 或登录限流配置较大者 | WebSocket 订阅注册绑定码状态 |
| `GET  /users/me/telegram/bind-code` | UID | 5 / 10 分钟 | 已登录用户生成 TG 绑定码；需 `X-Twilight-Intent: create-bind-code` |
| `POST /users/me/telegram/unbind` | UID | 5 / 10 分钟 | 防恶意频繁解绑 |
| `POST /users/me/telegram/rebind-request` | UID | 3 / 1 小时 | 换绑申请会进管理员队列，从严限制 |
| `GET  /users/regcode/check` | IP | 10 / 60 秒 | 防注册码枚举 |
| `GET  /invite/check` | IP | 10 / 60 秒 | 邀请码校验 |
| `POST /users/me/avatar/upload` 等上传 | UID | 10 / 60 秒 | 防滥用上传 |

上传后的头像/背景通过 `GET /users/assets/{kind}/{filename}` 读取（`kind` 为 `avatar` 或 `background`）。该接口要求登录，并校验资源类型、服务端生成的文件名格式和最终文件路径；不要直接公开 `/uploads` 目录。

> 限速命中只写日志告警，不会写入安全日志 / 登录历史。

#### Telegram 注册绑定码状态通道

注册绑定码状态主通道是 `GET /users/telegram/register/bind-code/ws` WebSocket，后端会在握手前校验绑定码格式。`GET /users/telegram/register/bind-code/status` 只作为 WebSocket 不可用或提交前的 fallback 查询。

绑定码本身只保存在当前 App 进程内存中，重启后失效；多进程部署时 Web API 与 Bot 需要运行在同一 App 实例内才能共享绑定码状态。

## 4. 模块总览

> 完整端点列表见 [API 路由索引](../reference/api-index.md)。本节只保留模块边界和维护口径。

| 模块 | 路径前缀 | 说明 |
| ---- | -------- | ---- |
| Auth | `/auth` | 登录、登出、会话、Token 刷新、登录端 API Key 管理 |
| Users | `/users` | 注册、个人信息、密码、Emby 绑定、续期、设备、Telegram、头像背景、个人 API Key |
| Media | `/media` | TMDB/Bangumi 搜索、求片、库存检查 |
| Emby | `/emby` | Emby 账号状态、库、搜索、会话、Bangumi Webhook |
| Admin | `/admin` | 管理用户、Emby 同步、注册码、广播、定时任务、邀请树、公告、Telegram 管理 |
| System | `/system` | 健康、系统信息、配置、运行时状态/日志、数据库、自动更新、路由列表 |
| API Key | `/apikey` | 外部系统专用 API Key 接口 |
| Security | `/security` | 设备、登录历史、IP 黑名单、可疑行为 |
| Batch | `/batch` | 批量用户操作、导出、观看统计 |
| Stats / Invite / Signin / Announcements | `/stats` `/invite` `/signin` `/announcements` | 播放统计、邀请树、签到（装饰性）、公告 |

### 4.1 命名与归属约定

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

## 5. Auth 模块

### 5.1 登录

`POST /auth/login`

- 说明：用户名/密码登录
- 认证：公开（`AuthPublic`）
- 请求头：`Content-Type: application/json`
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

### 5.2 Emby 找回 Web 密码

`POST /auth/forgot-password/emby`

- 说明：用 Emby 账号密码校验身份后重置 Web 密码，新密码只返回一次。
- 认证：公开（`AuthPublic`）
- 限流：IP + Emby 用户名双维度（见限流表）。
- 请求头：`Content-Type: application/json`

### 5.3 Telegram 直接登录（已禁用）

`POST /auth/login/telegram`

- 说明：固定返回"直接登录不可用"。Telegram 仅用于绑定，不作为登录入口。
- 认证：公开（`AuthPublic`）

### 5.4 API Key 登录

`POST /auth/login/apikey`

- 说明：用 API Key 换取登录态。
- 认证：公开（`AuthPublic`）

### 5.5 登出

`POST /auth/logout`

- 说明：注销当前登录会话
- 认证：登录用户（`AuthUser`）

```bash
curl -X POST "http://localhost:5000/api/v1/auth/logout" \
  -H "Authorization: Bearer <token>"
```

### 5.6 登出全部会话

`POST /auth/logout/all`

- 说明：注销当前用户的所有会话。
- 认证：登录用户（`AuthUser`）

### 5.7 当前用户

`GET /auth/me`

- 说明：获取当前登录用户信息（与 `GET /users/me` 同 handler）。
- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/auth/me" \
  -H "Authorization: Bearer <token>"
```

### 5.8 刷新 Token

`POST /auth/refresh`

- 说明：刷新用户 Token
- 认证：登录用户（`AuthUser`）

```bash
curl -X POST "http://localhost:5000/api/v1/auth/refresh" \
  -H "Authorization: Bearer <token>"
```

### 5.9 登录端 API Key 管理（旧版兼容）

以下接口为登录态下管理"用户级 API Key"的兼容接口；新的多 Key 管理见 [6.10 个人 API Key](#610-个人-api-key)，外部接入见 [API Key 外部接入](../reference/api-key.md)。

#### 获取当前用户 API Key

`GET /auth/apikey`

- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/auth/apikey" \
  -H "Authorization: Bearer <token>"
```

#### 生成 / 刷新 API Key

`POST /auth/apikey`

- 认证：登录用户（`AuthUser`）

```bash
curl -X POST "http://localhost:5000/api/v1/auth/apikey" \
  -H "Authorization: Bearer <token>"
```

#### 删除当前 API Key

`DELETE /auth/apikey`

- 认证：登录用户（`AuthUser`）

#### 启用当前 API Key

`POST /auth/apikey/enable`

- 认证：登录用户（`AuthUser`）

#### 获取 API Key 权限列表

`GET /auth/apikey/permissions`

- 认证：登录用户（`AuthUser`）

#### 更新 API Key 权限

`PUT /auth/apikey/permissions`

- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{
  "permissions": ["account:read", "emby:read"]
}
```

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

- 说明：新用户注册（成功返回 `201`）
- 认证：公开（`AuthPublic`）
- 限流：IP，5 / 10 分钟
- 请求头：`Content-Type: application/json`
- 请求体（可携带注册码 / Telegram 绑定码，视配置而定）：

```json
{
  "username": "newuser",
  "password": "Password123!",
  "email": "newuser@example.com"
}
```

```bash
curl -X POST "http://localhost:5000/api/v1/users/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","password":"Password123!","email":"newuser@example.com"}'
```

#### 检查用户名是否可用

`GET /users/check-available?username=<name>`

- 说明：检查用户名是否可用
- 认证：公开（`AuthPublic`）
- 限流：IP，60 / 60 秒

```bash
curl -X GET "http://localhost:5000/api/v1/users/check-available?username=newuser"
```

#### 校验注册码

`GET /users/regcode/check?reg_code=<code>`

- 说明：校验注册码 / 卡码是否有效，供注册页预检。
- 认证：公开（`AuthPublic`）
- 限流：IP，10 / 60 秒
- 规则细节见 [注册码与卡码](../features/regcodes.md)。

#### Telegram 注册绑定码

`GET /users/telegram/register/bind-code` — 生成注册阶段的 Telegram 绑定码（公开，IP 限流 5/10 分钟）。该 GET 有副作用，调用方必须带 `X-Twilight-Client: webui` 与 `X-Twilight-Intent: create-bind-code`，后端会拒绝预取请求。

`GET /users/telegram/register/bind-code/ws?code=<code>` — WebSocket 订阅绑定码状态（公开）。这是 Web 注册页主路径。

`GET /users/telegram/register/bind-code/status?code=<code>` — GET fallback 查询绑定码状态（公开，主要用于 WebSocket 不可用或提交前兜底）。

常见响应字段：`status`（`pending` / `confirmed` / `expired` / `not_found` / `invalid_format` / `wrong_scene` / `telegram_taken`）、`confirmed`、`terminal`、`expires_in`、`telegram_id`、`telegram_username`、`message`。绑定码只保存在当前 App 进程内存，服务重启后失效；注册提交时，后端会消费已确认绑定码、复检 Telegram ID 唯一性并创建用户。

`POST /users/me/telegram/bind-confirm` — 注册流程中确认绑定（路由为 `AuthPublic`，由绑定码本身承载身份）。
请求体：

```json
{ "code": "123456" }
```

#### Emby 注册队列状态

`GET /users/register/emby/status`

- 说明：轮询 Emby 注册队列中本次请求的结果。
- 认证：公开（`AuthPublic`）
- 限流：request_id + IP（见限流表）。

### 6.2 当前用户信息

#### 获取当前用户信息

`GET /users/me`

- 说明：获取当前用户详细信息
- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/users/me" \
  -H "Authorization: Bearer <token>"
```

#### 更新当前用户信息

`PUT /users/me`

- 说明：更新当前用户信息（如邮箱、Bangumi 同步设置）
- 认证：登录用户（`AuthUser`）
- 请求体示例：

```json
{
  "email": "updated@example.com",
  "bgm_mode": true,
  "bgm_token": "new-bgm-token"
}
```

```bash
curl -X PUT "http://localhost:5000/api/v1/users/me" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"email":"updated@example.com","bgm_mode":true,"bgm_token":"new-bgm-token"}'
```

#### 修改用户名

`PUT /users/me/username`

- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{ "username": "newusername" }
```

#### 修改密码

`PUT /users/me/password`

- 说明：生成 / 修改 Web 密码。
- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{
  "old_password": "oldpass",
  "new_password": "newPassword123!"
}
```

#### 验证并修改密码

`POST /users/me/password/change`（别名 `POST /users/me/password/system`）

- 说明：验证当前密码并修改 Web 密码。
- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{
  "current_password": "oldpass",
  "new_password": "newPassword123!"
}
```

#### 修改 Emby 密码

`POST /users/me/password/emby`

- 说明：修改已绑定 Emby 账号的密码。
- 认证：登录用户（`AuthUser`）

### 6.3 Emby 绑定与设置

#### 绑定 Emby 账号

`POST /users/me/emby/bind`

- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{
  "emby_id": "user_emby_id",
  "emby_password": "emby_password"
}
```

#### 注册 Emby 账号（队列）

`POST /users/me/emby/register`

- 说明：为当前账号在 Emby 上创建账号，走注册队列；同步等待结果，超时降级到前端轮询 `GET /users/register/emby/status`。
- 认证：登录用户（`AuthUser`）

#### 解绑 Emby 账号

`POST /users/me/emby/unbind`

- 认证：登录用户（`AuthUser`）
- 限制：如果用户记录 `emby_grant_locked=true`，说明该账号曾通过注册码、白名单码、邀请码、后台授予、Telegram 面板授予或自助创建获得 Emby 注册资格，自助解绑会返回 `403` + `EMBY_UNBIND_FORBIDDEN`。该判定只看用户自身字段，不依赖注册码使用记录或邀请关系，删除卡码、清理卡码使用记录、断开邀请关系都不会解除锁。
- 自助解绑会先调用 Emby API 禁用远端账号。远端账号不存在时允许清理本地绑定；远端存在但禁用失败时返回 `502` + `EMBY_DISABLE_FAILED`，并保留本地绑定，避免产生仍可登录的孤儿 Emby 账号。
- 成功响应会返回解绑后的用户公开字段，并额外包含 `remote_emby_disabled` 与 `old_emby_id`：`remote_emby_disabled=true` 表示本次已确认禁用远端 Emby；为 `false` 通常表示原本没有远端绑定或远端已不存在。
- 管理员解绑 / 强制解绑仍是管理员特权，也会同步禁用远端 Emby；这些操作不会清除 `emby_grant_locked`，被解绑用户不能借此再次自助创建远端 Emby。

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/emby/unbind" \
  -H "Authorization: Bearer <token>"
```

### 6.4 续期与注册码/续期码

注册码、续期码、白名单码的类型、生成格式、旧码兼容与安全口径见 [注册码与卡码](../features/regcodes.md)。

#### 续期当前账号

`POST /users/me/renew`

- 说明：用户使用续期码自助续期当前账号。**必须** 提供 `reg_code`，不接受无码续期。
- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{
  "reg_code": "code-abc123",
  "emby_username": "emby_name",
  "emby_password": "Password123"
}
```

```bash
curl -X POST "http://localhost:5000/api/v1/users/me/renew" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-abc123"}'
```

#### 使用注册码 / 续期码 / 白名单码

`POST /users/me/use-code`

- 说明：统一入口，自动识别注册码 / 续期码 / 白名单码并执行相应动作。
- 认证：登录用户（`AuthUser`）
- 请求体（`reg_code` 也可写作 `code`）：

```json
{
  "reg_code": "code-abc123",
  "emby_username": "emby_name",
  "emby_password": "Password123"
}
```

#### 使用卡码队列状态

`GET /users/me/use-code/status`

- 说明：轮询使用卡码后触发的 Emby 队列结果（与注册队列状态同 handler）。
- 认证：登录用户（`AuthUser`）

### 6.5 设备与登录历史

#### 查看当前设备列表

`GET /users/me/devices`

- 认证：登录用户（`AuthUser`）

#### 移除指定设备

`DELETE /users/me/devices/{device_id}`

- 认证：登录用户（`AuthUser`）

```bash
curl -X DELETE "http://localhost:5000/api/v1/users/me/devices/abc123" \
  -H "Authorization: Bearer <token>"
```

#### 查看当前登录会话

`GET /users/me/sessions`

- 认证：登录用户（`AuthUser`）

#### 查看登录历史

`GET /users/me/login-history`

- 认证：登录用户（`AuthUser`）

### 6.6 Telegram 绑定

#### 查询绑定状态

`GET /users/me/telegram`

- 认证：登录用户（`AuthUser`）

#### 生成绑定验证码

`GET /users/me/telegram/bind-code`

- 认证：登录用户（`AuthUser`）
- 限流：UID，5 / 10 分钟
- 请求头：`X-Twilight-Client: webui`、`X-Twilight-Intent: create-bind-code`

#### 申请换绑 Telegram

`POST /users/me/telegram/rebind-request`

- 说明：发起换绑申请，进入管理员审核队列。
- 认证：登录用户（`AuthUser`）
- 限流：UID，3 / 1 小时

#### 解绑 Telegram

`POST /users/me/telegram/unbind`

- 认证：登录用户（`AuthUser`）
- 限流：UID，5 / 10 分钟

> 注册流程中的绑定确认接口是 `POST /users/me/telegram/bind-confirm`（公开，见 [6.1](#61-注册与校验)），由绑定码本身承载身份。

### 6.7 个人设置

`GET /users/me/settings`

- 认证：登录用户（`AuthUser`）

### 6.8 头像与背景

| 方法/路径 | 认证 | 说明 |
| --------- | ---- | ---- |
| `GET /users/{uid}/avatar` | `AuthUser` | 读取指定用户头像 |
| `POST /users/me/avatar/upload` | `AuthUser` | 上传头像（UID 限流 10/60s） |
| `DELETE /users/me/avatar` | `AuthUser` | 删除头像 |
| `GET /users/{uid}/background` | `AuthUser` | 读取指定用户背景 |
| `PUT /users/me/background` | `AuthUser` | 设置背景（URL 或预设） |
| `POST /users/me/background/upload` | `AuthUser` | 上传背景（UID 限流 10/60s） |
| `DELETE /users/me/background` | `AuthUser` | 删除背景 |
| `GET /users/assets/{kind}/{filename}` | `AuthUser` | 读取上传产物，`kind` ∈ `avatar`/`background` |

背景与头像的存储、URL 校验（反 SSRF）见 [背景与头像](../features/background.md)。

### 6.9 个人 API Key

| 方法/路径 | 认证 | 说明 |
| --------- | ---- | ---- |
| `GET /users/me/apikeys` | `AuthUser` | 列出当前用户的 API Key |
| `POST /users/me/apikeys` | `AuthUser` | 创建新 API Key |
| `PUT /users/me/apikeys/{key_id}` | `AuthUser` | 更新指定 Key（权限/启用状态等） |
| `DELETE /users/me/apikeys/{key_id}` | `AuthUser` | 删除指定 Key |

外部系统如何使用这些 Key 见 [API Key 外部接入](../reference/api-key.md)。

## 7. Media 模块

> 求片状态更新（`PUT /media/request/{request_id}/status`、`GET /media/request/pending`）需要 **管理员**；其余检索与创建/查看自己的求片为登录用户。

### 7.1 媒体检索

#### 通用媒体搜索

`GET /media/search?keyword=<keyword>&page=1&per_page=20`

- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/media/search?keyword=matrix&page=1&per_page=20" \
  -H "Authorization: Bearer <token>"
```

#### TMDB 搜索

`GET /media/search/tmdb?query=<query>&page=1`

- 认证：登录用户（`AuthUser`）

#### Bangumi 搜索

`GET /media/search/bangumi?query=<query>&page=1`

- 认证：登录用户（`AuthUser`）

#### 通过 source_type 和 media_id 查询详情

`GET /media/search/id/{source_type}/{media_id}`

- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/media/search/id/tmdb/12345" \
  -H "Authorization: Bearer <token>"
```

#### 媒体详情

`GET /media/detail?source=tmdb&id=12345`

- 认证：登录用户（`AuthUser`）

#### TMDB 详情

`GET /media/tmdb/{tmdb_id}`

- 认证：登录用户（`AuthUser`）

#### Bangumi 详情

`GET /media/bangumi/{bgm_id}`

- 认证：登录用户（`AuthUser`）

### 7.2 库存

#### 库存检查

`POST /media/inventory/check`

- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{ "tmdb_id": 550, "source": "tmdb" }
```

#### 库存搜索

`GET /media/inventory/search?keyword=<keyword>&page=1&per_page=20`

- 认证：登录用户（`AuthUser`）

### 7.3 求片请求

#### 创建求片请求

`POST /media/request`

- 认证：登录用户（`AuthUser`）
- 请求体：

```json
{
  "title": "电影名称",
  "source": "bangumi",
  "remarks": "请尽快添加"
}
```

#### 查询我的求片请求

`GET /media/request/my`

- 认证：登录用户（`AuthUser`）

#### 查询待处理求片请求

`GET /media/request/pending`

- 说明：列出全部待处理求片，供管理端审批。
- 认证：管理员（`AuthAdmin`）

#### 更新求片请求状态

`PUT /media/request/{request_id}/status`

- 说明：更新求片状态。`status` 必须显式传入；不会把空请求体或 malformed JSON 默认视为 `accepted`。可用值：`pending` / `accepted` / `rejected` / `completed` / `downloading` 及兼容别名。
- 认证：管理员（`AuthAdmin`）
- 请求体：

```json
{ "status": "approved", "remarks": "已处理" }
```

```bash
curl -X PUT "http://localhost:5000/api/v1/media/request/123/status" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"status":"approved","remarks":"已处理"}'
```

#### 外部求片更新

`POST /media/request/external/update`

- 说明：供外部下载系统回写求片状态。路由级别为公开（`AuthPublic`），但 handler 内强制校验 **内部密钥**：通过 `X-Internal-Secret: <secret>` 或 `Authorization: Bearer <secret>` 传入，值必须匹配配置中的 `bot_internal_secret`。`status` 必须显式传入；`key` 也可写作 `require_key`。
- 请求体：

```json
{
  "key": "req_xxx",
  "status": "completed",
  "note": "外部系统同步"
}
```

```bash
curl -X POST "http://localhost:5000/api/v1/media/request/external/update" \
  -H "X-Internal-Secret: <secret>" \
  -H "Content-Type: application/json" \
  -d '{"key":"req_xxx","status":"completed","note":"外部系统同步"}'
```

#### 通过 key 查询/删除求片

`GET /media/request/by-key/{require_key}` — 按业务 key 查询求片（登录用户）。

`DELETE /media/request/by-key/{require_key}` — 按业务 key 删除自己的求片（登录用户）。

#### 查询单个求片请求

`GET /media/request/{request_id}`

- 认证：登录用户（`AuthUser`）

#### 取消求片请求

`DELETE /media/request/{request_id}`

- 认证：登录用户（`AuthUser`）

## 8. Emby 模块

### 8.1 查询当前用户 Emby 状态

`GET /emby/status`

- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/emby/status" \
  -H "Authorization: Bearer <token>"
```

### 8.2 获取 Emby 服务 URLs（已弃用）

`GET /emby/urls`

- 状态：**已弃用，固定返回 `410 Gone`**。该端点早期为未鉴权返回全量线路，存在泄露风险（路由级别为 `AuthPublic`，仅用于回 410）。
- 替代：改用 `GET /system/emby-urls`，按用户角色和 Emby 绑定状态下发线路（未绑定 Emby 的普通用户返回空列表）。

### 8.3 Emby 内容搜索

`GET /emby/search?query=<keyword>&page=1&per_page=20`

- 认证：登录用户（`AuthUser`）

### 8.4 获取最新媒体

`GET /emby/latest`

- 认证：登录用户（`AuthUser`）

### 8.5 查询 Emby 活跃会话数量

`GET /emby/sessions/count`

- 认证：登录用户（`AuthUser`）

### 8.6 Bangumi 观看进度 Webhook

`POST /emby/bangumi/webhook`

- 说明：接收 Emby 播放事件，按规则回写 Bangumi 观看进度。
- 认证：公开（`AuthPublic`），但携带 `X-Twilight-Bangumi-Timestamp` 时会做 ±300 秒 replay-window 校验，落在窗口外的请求被拒。
- 详见 [Bangumi 同步](../features/bangumi.md)。

## 9. Admin 模块

> 本模块全部为管理员级别（`AuthAdmin`）。

### 9.1 用户管理

#### 查询用户列表

`GET /admin/users?status=active&page=1&per_page=20`

```bash
curl -X GET "http://localhost:5000/api/v1/admin/users?status=active&page=1&per_page=20" \
  -H "Authorization: Bearer <admin_token>"
```

#### 获取单个用户信息

`GET /admin/users/{uid}`

#### 更新用户信息

`PUT /admin/users/{uid}`

#### 删除用户

`DELETE /admin/users/{uid}`

`POST /admin/users/{uid}/delete` — 推荐用于需要 JSON body 的删除操作，支持 `mode` 与 `cascade_depth`，避免依赖 `DELETE` body 被代理/WAF 保留。

#### 禁用 / 启用用户

`POST /admin/users/{uid}/disable` — 请求体可带 `{"reason":"违规使用"}`。

`POST /admin/users/{uid}/enable`

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/123/disable" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"reason":"违规使用"}'
```

#### 解绑 / 强制解绑 Emby

`DELETE /admin/users/{uid}/emby` — 解绑指定用户的 Emby 账号。

`POST /admin/users/{uid}/force-unbind` — 强制解绑（处理冲突场景）。

管理员解绑 / 强制解绑属于管理员特权操作，会先禁用远端 Emby 账号，再清空本地绑定字段；远端存在但禁用失败时不会清本地绑定。该操作不会清除用户记录上的 `emby_grant_locked`。因此被解绑用户若原本来自注册码、邀请码、后台授予、Telegram 授予或自助创建，仍不能通过自助解绑/自助开通流程重复创建 Emby 账号。

#### 续期用户 / 取消永久

`POST /admin/users/{uid}/renew` — 管理员为指定用户续期。`days=-1` 表示永久，`days=0` 按 30 天处理。请求体：

```json
{ "days": 30 }
```

`POST /admin/users/{uid}/cancel-permanent` — 取消永久授权（与续期同 handler）。

#### 重置密码

`POST /admin/users/{uid}/reset-password`

#### 踢出用户 Emby 会话

`POST /admin/users/{uid}/kick`

#### 切换管理员身份

`PUT /admin/users/{uid}/admin`

- 请求体：

```json
{ "admin": true }
```

#### Telegram 绑定管理

`POST /admin/users/{uid}/unbind-telegram` — 强制解绑指定用户的 Telegram。

`POST /admin/users/{uid}/bind-telegram` — 为指定用户绑定 Telegram。

`GET /admin/users/by-telegram/{telegram_id}` — 按 Telegram ID 反查用户。

#### 注册队列与授权处理

| 方法/路径 | 说明 |
| --------- | ---- |
| `POST /admin/users/{uid}/registration-queue/clear` | 清理单用户在飞注册队列 |
| `POST /admin/users/registration-queue/clear` | 清理全部在飞注册队列 |
| `POST /admin/users/registration-queue/grant-entitlement-and-clear` | 批量补发授权并清队列 |
| `POST /admin/users/{uid}/registration-entitlement` | 为单用户补发注册授权 |
| `POST /admin/users/{uid}/registration-entitlement/dequeue` | 补发授权并出队 |
| `POST /admin/users/sync-bindings` | 同步本地与 Emby 的绑定关系 |

### 9.2 Emby 管理

前端主入口为「Emby 管理」，设备/IP 审查已整合为该页面的独立页签；旧 `/admin/device-audit` 页面保留为兼容入口。

| 方法/路径 | 说明 |
| --------- | ---- |
| `POST /admin/emby/sync` | 同步所有 Emby 用户数据 |
| `GET /admin/emby/sessions` | 当前 Emby 会话 |
| `GET /admin/emby/activity` | Emby 活动记录 |
| `GET /admin/emby/users` | Emby 用户列表 |
| `POST /admin/emby/broadcast` | 发送 Emby 广播消息 |
| `POST /admin/emby/test` | 测试与指定 Emby 账号的连通性 |
| `POST /admin/emby/cleanup-orphans` | 清理孤立 Emby 用户 |
| `POST /admin/emby/import-users` | 从 Emby 导入用户 |
| `POST /admin/emby/reset-bindings` | 重置绑定关系 |
| `POST /admin/emby/delete-unlinked` | 删除未绑定的 Emby 用户 |
| `POST /admin/emby/create-standalone` | 直建独立 Emby 账号（不写本地 users） |
| `POST /admin/emby/force-set-password` | 强制设置 Emby 密码 |
| `POST /admin/users/{uid}/bind-emby` | 管理员为用户直绑 Emby；冲突时返回 `200 + success=false` 携带冲突详情 |

#### 同步所有 Emby 用户数据

`POST /admin/emby/sync`

```bash
curl -X POST "http://localhost:5000/api/v1/admin/emby/sync" \
  -H "Authorization: Bearer <admin_token>"
```

#### 发送 Emby 广播消息

`POST /admin/emby/broadcast`

- 请求体：

```json
{
  "title": "系统通知",
  "message": "Emby 服务器将在夜间维护。"
}
```

#### 测试 Emby 连通性

`POST /admin/emby/test`

- 请求体：

```json
{ "emby_id": "user_emby_id" }
```

#### 导入 Emby 用户

`POST /admin/emby/import-users`

- 请求体：

```json
{
  "source": "emby",
  "user_ids": [123, 456]
}
```

### 9.3 注册码与卡码

> 规则细节见 [注册码与卡码](../features/regcodes.md)。

#### 查询注册码列表

`GET /admin/regcodes`

#### 创建注册码

`POST /admin/regcodes`

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
  "decoy": false,
  "target_username": "",
  "target_telegram_username": "",
  "target_telegram_id": null
}
```

- `target_username`、`target_telegram_username`、`target_telegram_id` 三者只能填写一个，用于生成指名卡码。

```bash
curl -X POST "http://localhost:5000/api/v1/admin/regcodes" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"type":1,"validity_time":-1,"use_count_limit":1,"days":30,"count":1}'
```

#### 批量删除注册码

`POST /admin/regcodes/batch-delete`

- 说明：物理删除选中的注册码；已有使用记录的注册码也会直接移除。

#### 更新注册码备注

`PUT /admin/regcodes/{code}`

- 请求体：

```json
{ "note": "活动发放" }
```

#### 删除注册码

`DELETE /admin/regcodes/{code}`

- 说明：物理删除指定注册码；已有使用记录的注册码也会直接移除。

#### 查询注册码使用者

`GET /admin/regcodes/{code}/users`

#### 清理注册码使用记录

`POST /admin/regcodes/{code}/clear-usage`

- 说明：清空指定注册码的使用次数、使用者 UID 与 Telegram ID 记录，并重新激活该码；不影响已注册用户账号。
- 请求体：`confirm` 必须为 `CLEAR_REGCODE_USAGE`

```json
{ "confirm": "CLEAR_REGCODE_USAGE" }
```

### 9.4 求片管理（Admin 别名）

`GET /admin/media-requests` — 列出全部求片（与 `/media/request/pending` 同 handler）。

`PUT /admin/media-requests/{request_id}` — 更新求片状态。

`DELETE /admin/media-requests/{request_id}` — 删除求片。

`PUT /admin/media-requests/by-key/{require_key}` — 按 key 更新。

`DELETE /admin/media-requests/by-key/{require_key}` — 按 key 删除。

### 9.5 白名单与统计

#### 管理白名单

`POST /admin/whitelist`

- 请求体：

```json
{ "ip": "192.168.1.100" }
```

#### 查询管理员统计

`GET /admin/stats`

- 说明：管理面板汇总统计（与 `/system/stats`、`/system/admin/stats` 同 handler）。

### 9.6 批量用户操作

| 方法/路径 | 说明 |
| --------- | ---- |
| `POST /admin/users/bulk-expire` | 批量置过期 |
| `POST /admin/users/bulk-enable-disabled` | 批量启用被禁用用户 |
| `POST /admin/users/cleanup-invalid` | 清理无效用户（见下） |
| `POST /admin/users/clear-stale-pending-emby` | 清理长期 `PENDING_EMBY` 用户 |
| `POST /admin/users/clear-emails` | 清空所有用户邮箱设置（见下） |
| `POST /admin/users/kick-no-emby` | 踢出无 Emby 账号用户 |

#### 清理无效用户

`POST /admin/users/cleanup-invalid`

- 请求体：`dry_run` 默认为 `true`，仅预览候选用户；执行删除时必须传 `{"dry_run":false,"confirm":"CLEANUP_INVALID_USERS"}`。

```bash
curl -X POST "http://localhost:5000/api/v1/admin/users/cleanup-invalid" \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"min_days":7,"dry_run":true}'
```

#### 清空所有用户邮箱

`POST /admin/users/clear-emails`

- 请求体：`dry_run` 默认为 `true`，仅预览当前设置了邮箱的用户数量；执行清空时必须传 `{"dry_run":false,"confirm":"CLEAR_ALL_EMAILS"}`。
- 行为：仅把本地用户的 `email` 字段设置为空，不影响用户名、密码、Telegram 绑定、Emby 绑定或有效期。

### 9.7 邀请树管理

> 邀请关系（`invite_relations`）、邀请码等均以字段形式保存在单一状态文档（`internal/store`）中，并非独立数据库或单表；详见 [邀请树](../features/invite.md)。

`GET /admin/invite/tree` — 查看邀请树。

`POST /admin/invite/users/{uid}/detach` — 将指定用户从邀请树脱离。

`GET /admin/invite/codes` — 列出全部邀请码。

### 9.8 违规与 Telegram 管理

| 方法/路径 | 说明 |
| --------- | ---- |
| `GET /admin/violations` | 列出违规记录 |
| `DELETE /admin/violations/{violation_id}` | 删除单条违规 |
| `POST /admin/violations/clear` | 清空违规记录 |
| `GET /admin/telegram/rebind-requests` | 列出换绑申请 |
| `POST /admin/telegram/rebind-requests/{request_id}/approve` | 批准换绑 |
| `POST /admin/telegram/rebind-requests/{request_id}/reject` | 拒绝换绑 |
| `POST /admin/telegram/rebind-requests/batch` | 批量审核换绑 |
| `GET /admin/telegram/roster/stats` | Telegram 群花名册统计 |
| `POST /admin/telegram/rejoined-users/enable` | 启用重新入群用户 |
| `POST /admin/telegram/kick-unbound` | 踢出未绑定用户 |

Telegram 相关行为见 [Telegram Bot 命令](../features/telegram-bot.md)。

### 9.9 定时任务管理

定时任务的计划信息与执行历史持久化在状态存储中（`internal/store`）；每个 `job_id` 默认保留最近若干条执行记录，超出后自动裁剪。进程启动时会把长时间残留在 `running` 状态的记录改判为 `failed`，避免崩溃后前端永远转圈。

`GET /admin/scheduler/jobs` 返回的每个 job 含触发器结构化描述：

| 字段 | 说明 |
| ---- | ---- |
| `trigger_spec` | 当前生效的触发规则，结构同下方 PUT 请求体 |
| `default_trigger_spec` | config.toml 算出的默认值（用于"恢复默认"按钮显示） |
| `is_custom` | 是否已被管理员覆盖（true 时前端显示"已自定义"徽章） |
| `last_auto_run_at` | 最近一次自动执行开始时间 |
| `last_manual_run_at` | 最近一次手动执行开始时间 |

单次运行（`SchedulerJobRun`）字段：

| 字段 | 说明 |
| ---- | ---- |
| `id` | 运行记录主键 |
| `job_id` | 任务标识（如 `check_expired`、`emby_sync`） |
| `type` | 执行类型：`auto` / `manual` |
| `trigger` | 触发来源：`scheduled` / `manual` / `startup` |
| `status` | `running` / `success` / `failed` |
| `started_at` | 起始时间戳（秒） |
| `finished_at` | 结束时间戳（秒），运行中为 `null` |
| `error` | 失败时的异常摘要 |
| `summary` | 结构化指标，如 `{"scanned": 12, "disabled": 3, "failed": 0}` |
| `logs` | 任务内部累积的日志行（列表接口不返回，详情接口返回） |

#### 列出全部定时任务

`GET /admin/scheduler/jobs`

- 说明：列出内置定时任务定义、计划时间、下次执行时间和最近一次运行摘要（不含 logs 正文，体积小适合轮询）。
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

#### 手动触发一次任务

`POST /admin/scheduler/jobs/{job_id}/run`

- 说明：将指定任务排入后台执行；接口立即返回，前端通过轮询 `/admin/scheduler/jobs` 拿到结束状态。
- 响应：`data.last_run` 是触发时的快照（通常为 `running`）。

```bash
curl -X POST "http://localhost:5000/api/v1/admin/scheduler/jobs/check_expired/run" \
  -H "Authorization: Bearer <admin_token>"
```

#### 终止运行中的任务

`POST /admin/scheduler/jobs/{job_id}/terminate`

- 说明：请求终止正在运行的任务。

#### 获取最近一次完整运行（含日志）

`GET /admin/scheduler/jobs/{job_id}/last-run`

- 说明：返回包含 `logs` 正文的完整运行记录，前端"查看日志"弹窗使用。

#### 获取历史运行列表

`GET /admin/scheduler/jobs/{job_id}/history?limit=20`

- 说明：按时间倒序返回历史运行；`limit` 范围 1–100，默认 20。

#### 修改触发器（覆盖 config.toml 默认值）

`PUT /admin/scheduler/jobs/{job_id}/schedule`

- 说明：把新的触发规则写入状态存储并实时重排；下次进程重启后仍生效。每个 `job_id` 至多一条覆盖。
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

`DELETE /admin/scheduler/jobs/{job_id}/schedule`

- 说明：删除该 job 的覆盖记录，并按 `default_trigger_spec` 重新排程。

### 9.10 公告管理

> 公告（`announcements`）以字段形式保存在单一状态文档（`internal/store`）中，不存在独立数据库或 `ALTER TABLE` 操作；前台读取见 [11.x 公告](#1112-公告)，功能说明见 [公告系统](../features/announcements.md)。

`GET /admin/announcements` — 列出全部公告。

`POST /admin/announcements` — 创建公告。

`PUT /admin/announcements/{announcement_id}` — 更新公告。

`DELETE /admin/announcements/{announcement_id}` — 删除公告。

## 10. System 模块

### 10.1 健康检查

`GET /system/health`

- 说明：返回 `api`、`database`、`emby` 布尔状态，并保留 `status/storage/redis` 等兼容字段。
- 认证：公开（`AuthPublic`）

```bash
curl -X GET "http://localhost:5000/api/v1/system/health"
```

### 10.2 系统信息

`GET /system/info`

- 说明：系统公开信息。`telegram_bot` 会在 Telegram 已启用且可连通时返回 Bot 用户名；`telegram_links.groups/channels` 只返回通过白名单校验的公开 `t.me` 链接，不暴露纯数字私有群 ID。`setup` 字段返回初始化向导状态，前端可在 `setup.available=true` 时跳转 `/setup`。
- 认证：公开（`AuthPublic`）

```bash
curl -X GET "http://localhost:5000/api/v1/system/info"
```

### 10.3 初始化向导

`GET /setup/status`

- 说明：查询网页初始化向导是否可用。
- 认证：公开（`AuthPublic`）
- 可用条件：配置文件显式启用 `setup_mode = true` 或 `SetupMode = true`，状态存储中没有任何用户，且当前运行配置中没有 `Admin.uids` / `Admin.usernames`（包括环境变量覆盖）。`setup_mode` / `SetupMode` 可写在任意 TOML table 中；任一条件不满足都会返回 `available=false`。

`POST /setup/complete`

- 说明：一次性创建初始化管理员、写入基础配置、追加 `[Admin] usernames = [...]`，并下发管理员会话 Cookie。
- 认证：公开（`AuthPublic`），但必须同时满足空系统硬门控、IP 限流和显式 WebUI intent 头。
- 必填头：`X-Twilight-Client: webui`、`X-Twilight-Intent: complete-setup`。
- 安全边界：完成后主配置会移除 `setup_mode` / `SetupMode` 标记；同时由于已有用户与管理员配置，初始化入口永久关闭。普通 `/users/register` 仍不会因为空数据库自动获得管理员权限。
- 敏感信息：请求中的密码、Token、SMTP 密码等只写入受控配置或密码哈希，不在响应和审计 detail 中明文回显。

示例请求：

```bash
curl -X POST "http://localhost:5000/api/v1/setup/complete" \
  -H "Content-Type: application/json" \
  -H "X-Twilight-Client: webui" \
  -H "X-Twilight-Intent: complete-setup" \
  -d '{
    "admin": {"username": "owner", "password": "Owner123456"},
    "global": {"server_name": "Twilight"},
    "emby": {"emby_url": "http://emby:8096"},
    "policy": {"register_mode": false, "register_code_limit": true}
  }'
```

### 10.4 服务器图标

`GET /system/server-icon` — 读取服务器图标（公开）。

`POST /system/admin/server-icon/upload` — 上传服务器图标（管理员）。

### 10.5 读取运行时配置（公开给登录用户）

`GET /system/config`

- 说明：获取面向登录用户的运行时配置（前端按此渲染开关）。
- 认证：登录用户（`AuthUser`）

```bash
curl -X GET "http://localhost:5000/api/v1/system/config" \
  -H "Authorization: Bearer <token>"
```

### 10.6 管理员配置只读视图

`GET /system/admin/config`

- 说明：返回完整运行时配置视图（管理面板使用）。
- 认证：管理员（`AuthAdmin`）

### 10.7 系统/管理员统计

`GET /system/stats` 与 `GET /system/admin/stats` — 均为管理员级别（`AuthAdmin`），与 `/admin/stats` 同 handler，返回系统聚合统计。

### 10.8 获取 Emby 服务线路（按角色下发）

`GET /system/emby-urls`

- 说明：替代已弃用的 `/emby/urls`。后端按当前用户的角色与 Emby 绑定状态过滤：
  - **未绑定 Emby 的普通用户**：返回空 `lines`，响应体附带 `requires_emby_account: true`，前端据此隐藏整块线路 UI，避免在用户尚未持有 Emby 账号时先把服务器地址泄露给浏览器。
  - **已绑定 Emby 的普通用户**：返回普通线路列表 `lines`。
  - **白名单 / 管理员**：额外返回 `whitelist_lines` 专属线路。
- 认证：登录用户（`AuthUser`）
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

### 10.9 Emby 线路探测

`POST /system/emby-urls/probe`

- 说明：服务端探测 Emby 线路连通性，供前端在多线路间选择。
- 认证：登录用户（`AuthUser`）

### 10.10 管理员运行状态与实时日志

`GET /system/admin/runtime/status`

- 说明：返回 Go 进程、主机、内存、数据库、路由数等运行状态，供管理端实时状态卡片使用。
- 认证：管理员（`AuthAdmin`）
- 安全口径：只读取进程内统计和 Linux `/proc` 汇总信息，不暴露环境变量、命令行参数、配置明文或任意文件内容。

`data` 主要字段：

| 字段 | 说明 |
| ---- | ---- |
| `started_at` | Go 进程启动时间戳 |
| `uptime_seconds` | Go 进程运行秒数 |
| `host_uptime_seconds` | Linux 主机运行秒数，可用时返回 |
| `go_version` / `goos` / `goarch` | Go 运行时与平台 |
| `goroutines` / `cpu_count` | 协程数和 CPU 数 |
| `active_database` / `config_database` | 当前存储后端与配置中的目标后端 |
| `log_level` / `runtime_log_limit` / `runtime_log_entries` | 当前日志等级、保留行数和缓冲区已有行数 |
| `memory` | Go runtime 内存统计 |
| `host_memory` | `/proc/meminfo` 摘要，可用时返回 |
| `load_average` | `/proc/loadavg` 的 1/5/15 分钟负载，可用时返回 |

`GET /system/admin/runtime/logs?limit=200&after=0`

- 说明：读取后端进程内最近日志快照，`limit` 受 `[Global].runtime_log_limit` 限制（默认配置 5000）。
- 认证：管理员（`AuthAdmin`）
- 响应：`entries` 为日志数组，`next_cursor` 用于下一次增量读取，`limit` 为本次实际读取上限。`after=0` 返回最近 N 条并按 `id` 升序排列；`after>0` 返回游标之后的前 N 条，同样按 `id` 升序排列。

`GET /system/admin/runtime/logs/stream?limit=100&after=0`

- 说明：SSE 实时日志流，事件类型包括 `snapshot`、`logs`、`ping`。
- 认证：管理员（`AuthAdmin`）；浏览器端使用同源 Cookie 或显式可信 CORS Origin。
- 事件数据：`snapshot` / `logs` 均返回 `{ "entries": [...], "next_cursor": 123 }`；`ping` 返回 `{ "time": 1234567890, "next_cursor": 123 }`。客户端应保存 `next_cursor` 作为断线重连的 `after` 参数。
- 安全口径：日志来自 Go 进程内日志处理链，敏感键、API Key、Bearer Token、Cookie、密码、DSN 会被尽力脱敏；接口不读取 journald、系统日志文件或用户指定路径。

```bash
curl -N "http://localhost:5000/api/v1/system/admin/runtime/logs/stream?limit=100" \
  -H "Authorization: Bearer <admin_token>"
```

### 10.11 config.toml 读写与备份

`GET /system/admin/config/toml` — 读取当前 config.toml（管理员）。

`PUT /system/admin/config/toml` — 写入 config.toml（管理员）。保存前会创建备份。

> 重要：配置读取与备份查看会对 API Key、Token、密码、Secret 等敏感字段脱敏；schema 保存路径使用服务端哨兵保留未修改 secret，原始 TOML 保存路径也会在文本层还原未修改 secret，禁止明文回显。邮箱、Telegram、邀请、安全等迁移模块仍可从配置管理折叠入口跳转，但推荐在对应独立管理页维护，所有页面仍写回同一个 `config.toml`。

- 请求体：

```json
{ "content": "[Global]\nlogging = true\n..." }
```

`GET /system/admin/config/schema` — 获取结构化配置 schema（管理员）。

`PUT /system/admin/config/schema` — 按 schema 写入配置（管理员），保存前创建备份。请求体：

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

配置备份与整理：

| 方法/路径 | 说明 |
| --------- | ---- |
| `GET /system/admin/config/backups` | 列出配置备份 |
| `POST /system/admin/config/backup` | 创建配置备份 |
| `GET /system/admin/config/backups/{name}` | 查看指定配置备份 |
| `DELETE /system/admin/config/backups/{name}` | 删除指定配置备份 |
| `POST /system/admin/config/restore` | 从备份恢复配置 |
| `POST /system/admin/config/sweep` | 手动触发 config.toml 自动整理（迁移历史段、删孤立键、补默认值，带备份） |

### 10.12 数据库状态、备份、恢复、迁移

> Twilight 把全部业务状态保存在 **单一状态文档** 中：JSON 文件 `db/twilight_go_state.json` 或 PostgreSQL `twilight_state` 表（`id=1` 的一行 jsonb）；另有独立表 `twilight_sessions`、`twilight_runtime_logs`。`twilight_runtime_logs` 仅用于高写入运行日志，并按 `id` 游标、索引和 cutoff id 裁剪优化；业务实体仍不得拆成独立表。下列接口围绕该状态文档操作。

`GET /system/admin/database/status`

- 说明：返回当前 active driver、配置 driver、状态文件、备份目录、PostgreSQL 配置状态和用户数。响应同时给出 `active_label/configured_label`，其中 `gojson` 表示 Go JSON 状态文件，`sqlite3` 表示旧 SQLite 迁移源，`postgresql` 表示 PostgreSQL 后端。
- 认证：管理员（`AuthAdmin`）

`GET /system/admin/database/backups` — 列出备份目录中的状态快照。

`GET /system/admin/database/backups/{name}` — 查看指定备份。

`DELETE /system/admin/database/backups/{name}` — 删除指定备份。

`POST /system/admin/database/backup` — 创建当前状态快照备份。

`POST /system/admin/database/restore`

- 说明：从指定备份恢复。未传确认短语时只返回预览，不写入数据；确认执行前会自动创建保护性备份。备份名限制在配置的备份目录内。
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

- 预检响应 `data` 包含 `source_driver`、`configured_driver`、`target_driver`、`snapshot_bytes`、`target_ready`、`backup_ready`、`warnings`、`counts`、`requires_confirmation`、`confirm`，并保留 `users`、`regcodes`、`invite_codes` 等兼容字段。PostgreSQL 目标会在权限允许时自动创建缺失数据库并准备 `twilight_state` 状态表，`target_ready.database_created` / `target_ready.schema_ready` 反映结果。
- 旧 SQLite 来源会额外返回 `legacy_sqlite` 与 `legacy_sqlite_import`，包含检测到的文件、表计数、已映射表和未映射表。
- 执行响应会额外返回 `pre_operation_backup` / `pre_migration_backup`；旧 SQLite 来源还会返回 `legacy_sqlite_backup`，确认写入前已自动备份旧文件集。

### 10.13 Git 自动更新

`POST /system/admin/update`

- 说明：从 HTTPS Git 仓库拉取指定分支。默认拒绝 dirty worktree，使用 `git pull --ff-only`，不会 reset/rebase/merge。
- 认证：管理员（`AuthAdmin`）
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

### 10.14 测试 Telegram Bot 连通性

`POST /system/admin/bot/test`

- 说明：直接调用 Telegram Bot HTTP API（`getMe` + `sendMessage`），不复用全局运行的 Bot 实例。
- 认证：管理员（`AuthAdmin`）
- 请求体（可选）：

```json
{ "target": "@my_channel" }
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

### 10.15 获取全部路由列表

`GET /system/admin/apis`

- 说明：获取后端注册的全部路由列表。
- 认证：管理员（`AuthAdmin`）

### 10.16 开发者模式与 JS 沙箱

`POST /admin/developer-mode/activate`

- 说明：管理员在仪表盘输入 `DEBUGMODE` 后，提交当前管理员密码完成二次验证。该接口现在是服务端全局开关：当前关闭时调用会开启；当前开启时再次调用会关闭。关闭后 JS 预设和相关 Telegram JS 指令保留但全部被服务端阻断。
- 认证：管理员（`AuthAdmin`）
- 审计：成功开启写入 `developer_mode_activate`；成功关闭写入 `developer_mode_deactivate`。

请求体：

```json
{ "code": "DEBUGMODE", "password": "<admin_password>" }
```

`POST /admin/developer/js-sandbox`

- 说明：在受控 Goja（`github.com/dop251/goja`）沙箱中预检/执行 Telegram JS 自定义指令片段。脚本同步执行，单次运行 8 秒墙钟超时（须大于沙箱网络预算：fetch 约 1500ms HTTP + 500ms DNS；interactions 会真实发送 Telegram 消息）。沙箱仅暴露文档列出的 `ctx`、`command`、`input`、`args`、`user`、`me`、`constants`、`roles`、`db`、`users`、`admin`、`regcodes`、`invites`、`announcements`、`system`、`text`、`arrays`、`time`、`format`、`interactions`、`getUser(uid)`、`reply(text)`、`exit(text?)`、`assert(condition, text?)`、`log(text)`、`auth(role)`、`authAdmin()`、`fetch(url)`、`config(key)`、`env(key)`、`setTimeout()`、`setInterval()`；配置与环境变量读取为只读白名单，敏感值不会返回，不提供文件、进程、模块加载器或浏览器全局对象。
- 门控：开发者模式关闭时返回 `403`，Bot 运行时也会阻断 `js:` / `js:preset:<id>` 相关指令与交互。
- 预览：`ctx.preview=true`；状态变更类辅助函数仅 dry-run，例如 `users.setLoginNotify(...)` 会返回 `dry_run=true`，不会写入用户数据。
- 认证：管理员（`AuthAdmin`）
- 审计：每次预检写入 `developer_js_sandbox_preview`；Bot 命中 `js:` 自定义命令时写入 `telegram_js_command_execute`。

请求体：

```json
{ "code": "reply('hello ' + (user.username || 'user'));" }
```

`GET /admin/developer/js-docs`

- 说明：返回开发者模式独立 JS 文档页使用的结构化接口文档，包含 JS 引擎、内置对象、受控函数、命名空间、配置键、环境变量、示例脚本、静态阻止 token 和允许但高风险 token，前端以类似 Swagger 的方式展示。每个函数条目包含 `example` 示例代码，并可包含 `params` 参数表与 `returns` 返回说明；示例集中包含 `exit-and-assert`，用于说明 `exit()` / `assert()` 的正常提前退出语义。
- 认证：管理员（`AuthAdmin`）
- 响应：`{ engine, bindings, functions, namespaces, native_objects, config_keys, env_keys, examples, blocked_tokens, risk_tokens }`。`functions` / `namespaces` / `bindings` / `native_objects` 中的条目结构为 `{ name, category, type?, description, example?, mutates?, scope?, fields?, params?, returns? }`，其中 `params[]` 为 `{ name, type?, required?, description, default? }`。
- 用户数据接口：`users.current()` / `users.describe()` 返回当前 Telegram 绑定用户脱敏快照；`getUser(uid)` / `users.get(uid)` / `users.byUID(uid)` 只允许按精确 UID 读取脱敏快照，普通用户只能读取自己，读取其他用户必须当前绑定用户为管理员；`users.search(query, limit)`、`users.find(query, limit)`（`search` 别名）、`users.list(options)`、`admin.searchUsers()`、`admin.listUsers()` 为管理员受控搜索/列表；`users.exists(uid)` 返回 UID 是否存在（跨用户查询需管理员，否则 `false`）；`users.setLoginNotify({ telegram?, email? })` 仅修改当前用户登录通知偏好；`users.setActive`、`users.enable(uid)` / `users.disable(uid)`（`setActive` 简化别名）、`users.setRole`、`users.setExpiry`、`users.extend(uid, days)`（顺延到期）、`users.update` 及 `admin.*` 对应快捷写接口仅管理员可用，运行时写审计日志并保持最后管理员保护。
- 数据库接口：`db.schema()` / `db.collections()` / `db.count(name)` / `db.currentUser()` / `db.getUser(uid)` / `db.findUsers(query, limit)` / `db.listUsers(options)` / `db.listRegcodes(options)` / `db.listInviteCodes(options)` / `db.listMediaRequests(options)` / `db.listAnnouncements(options)` / `db.listTickets(options)` / `db.listPresets(options)` / `db.updateCurrentUser(patch)` / `db.updateUser(uid, patch)` 只暴露受控结构、脱敏快照、受控计数和受控写入；不提供 SQL、原始 state、密码、Token、API Key、BGM Token 明文、Emby 内部 ID 或数据库连接信息。列表函数统一接受 `{ limit, offset }`（`limit` 上限 50）。`db.listRegcodes` / `db.listPresets` 为管理员专用（预设仅暴露 `code_length`，不含代码正文）；`db.listInviteCodes` / `db.listMediaRequests` / `db.listTickets` 对普通用户只返回属于自己的记录、工单不含正文；`db.listAnnouncements` 只返回可见公告且不含正文。`db.updateCurrentUser` 仅可修改当前绑定用户的登录通知偏好，预览模式为 dry-run。
- 指令上下文：示例 `command-context` 展示用户输入指令时可读取的全部非敏感字段，包括 `ctx.private_chat`、`ctx.preview`、`ctx.command_time`、`args`、`input.*`、当前绑定用户的 `uid`、`username`、`email`、`email_masked`、`role`、`role_name`、`active`、`expired_at`、`expire_status`、`created_at`、`register_time`、`has_emby`、`emby_username`、`emby_disabled`、`email_verified`、`email_verified_at`、`telegram_bound`、`telegram_id`、`telegram_username`、登录通知开关等；不返回 chat ID、message ID、群组 ID、密码、Token、API Key、BGM Token 明文、Emby 内部 ID 或数据库连接信息。
- 生成接口：`regcodes.list(options)` / `regcodes.get(code)` / `regcodes.generate(options)`、`invites.list(options)` / `invites.generate(options)`、`announcements.list(options)` / `announcements.create(options)` 提供受控的列表/查询与生成。`regcodes.generate`、`invites.generate`、`announcements.create`（及便捷别名 `admin.generateRegcode` / `admin.generateInviteCode` / `admin.createAnnouncement`）仅管理员可用，预览模式返回 `dry_run=true` 不写入，成功写入时来源标记 `telegram_js` 并分别记录 `telegram_js_regcode_generate` / `telegram_js_invite_generate` / `telegram_js_announcement_create` 审计日志。`regcodes.generate` 支持 `count`(1-100) / `type`(1-3) / `days`(≤36500) / `use_count_limit`(-1 无限) / `validity_time`(-1 永久) / `note` / `target_username` / `decoy`；`invites.generate` 受 `invite_enabled` 功能开关约束，`days` 受 `maxCodeDays` 上限约束；`announcements.create` 支持 `title` / `content` / `level` / `render_mode` / `visible` / `pinned` / `expires_at`。生成的注册码/邀请码为可下发兑换凭据，会在返回值中给出具体码值。另提供位置参数简化别名：`regcodes.quick(days?, count?, type?)`、`invites.quick(days?)`、`announcements.post(title, content, level?)`，完全复用对应 `generate` / `create` 的鉴权、预览与审计语义。
- 风险兼容能力：`fetch`、`eval`、`Function`、`globalThis`、`setTimeout`、`setInterval` 不再作为静态错误；`fetch` 是同步受限请求，阻断 localhost/内网/链路本地目标、禁用跳转和凭据、限制响应长度；计时器包装器同步执行回调，不创建异步任务。
- Telegram 交互：`interactions.inline(text, actions)` 支持最多 8 个静态按钮，callback 仅限创建消息的同一 Telegram 用户、同一 chat、同一 message，并在 2 分钟后过期；`interactions.waitText(options)` 只消费同一用户在 1-60 秒内发送的下一条非命令文本，支持 `prompt`、`reply_prefix`、`timeout_reply`、`max_chars`、`numbered`。
- 审计：文档接口为只读；通过 Bot 实际执行 `users.setLoginNotify` 成功写入时额外记录 `telegram_js_user_notify_update`；callback 与等待文本消费分别记录 `telegram_js_interaction_callback` / `telegram_js_interaction_wait_text`，拒绝的 callback 记录 `telegram_js_interaction_callback_denied`。

`GET /admin/developer/js-presets`

- 说明：列出开发者模式中保存的 JS 预设，供开发者编辑器和 Telegram Bot 指令管理页选择。
- 认证：管理员（`AuthAdmin`）
- 响应：`{ presets, total, developer_mode_enabled }`，其中 `developer_mode_enabled=false` 表示预设保留但运行时 JS 指令会被阻断。

`POST /admin/developer/js-presets`

- 说明：创建 JS 预设。`name` 必填；`code` 允许为空，用于创建空白草稿。非空 `code` 必须通过同一套 JS 沙箱静态校验。
- 认证：管理员（`AuthAdmin`）
- 审计：成功写入 `developer_js_preset_create`。

```json
{
  "name": "hello",
  "description": "Greeting command",
  "code": "reply('Hello ' + (user.username || 'user'));"
}
```

`PUT /admin/developer/js-presets/{preset_id}` / `DELETE /admin/developer/js-presets/{preset_id}`

- 说明：更新或删除指定 JS 预设。更新时仍允许空白 `code`，非空代码必须通过校验。
- 认证：管理员（`AuthAdmin`）
- 审计：成功分别写入 `developer_js_preset_update` / `developer_js_preset_delete`。

Telegram 管理中的「Bot 指令管理」页面不会创建第二套指令存储；纯文本和 JS 预设最终都保存回 `Telegram.bot_custom_commands`。选择 JS 预设时，前端保存为 `js:preset:<id>`，Bot 运行时按 ID 动态读取当前预设代码；旧 `js:<code>` 静态代码格式继续兼容。

## 11. 其它模块

### 11.1 Security 模块

| 方法/路径 | 认证 | 说明 |
| --------- | ---- | ---- |
| `GET /security/devices` | `AuthUser` | 当前用户设备列表 |
| `POST /security/devices/{device_id}/block` | `AuthUser` | 封禁设备 |
| `POST /security/devices/{device_id}/trust` | `AuthUser` | 信任设备 |
| `GET /security/login-history` | `AuthUser` | 当前用户登录历史 |
| `GET /security/login-history/{uid}` | `AuthAdmin` | 指定用户登录历史 |
| `GET /security/ip/blacklist` | `AuthAdmin` | 查询 IP 黑名单 |
| `POST /security/ip/blacklist` | `AuthAdmin` | 添加 IP 黑名单 |
| `DELETE /security/ip/blacklist` | `AuthAdmin` | 删除 IP 黑名单 |
| `GET /security/suspicious` | `AuthAdmin` | 可疑行为列表 |
| `GET /security/users/{uid}/devices` | `AuthAdmin` | 指定用户设备列表 |
| `POST /security/users/{uid}/devices/{device_id}/block` | `AuthAdmin` | 封禁指定用户设备 |

### 11.2 Batch 模块

| 方法/路径 | 认证 | 说明 |
| --------- | ---- | ---- |
| `POST /batch/users/disable` | `AuthAdmin` | 批量禁用 |
| `POST /batch/users/enable` | `AuthAdmin` | 批量启用 |
| `POST /batch/users/renew` | `AuthAdmin` | 批量续期 |
| `POST /batch/users/delete` | `AuthAdmin` | 批量删除 |
| `POST /batch/users/emby-unbind-lock` | `AuthAdmin` | 批量为已绑定 Emby 的用户写入 `emby_grant_locked=true`，禁止用户自助解绑 Emby；未绑定 Emby 的用户返回在 `skipped_no_emby` 中，不视为失败 |
| `POST /batch/users/emby-grant-clear` | `AuthAdmin` | 批量清理无 Emby 账号用户的注册码/邀请码使用记录：清 `emby_grant_locked` / `registration_source` / `registration_code` 并回退其占用的注册码、邀请码使用计数与邀请关系，使其可重新使用注册码/邀请码；已绑定 Emby 的用户计入 `skipped_has_emby`、`PendingEmby` 在飞用户计入 `skipped_pending`，均不视为失败 |
| `GET /batch/export/users` | `AuthAdmin` | 导出用户 |
| `GET /batch/export/playback` | `AuthAdmin` | 导出播放记录 |
| `GET /batch/watch-stats` | `AuthUser` | 当前用户观看统计 |
| `GET /batch/watch-stats/{uid}` | `AuthAdmin` | 指定用户观看统计 |
| `GET /batch/watch-stats/global` | `AuthAdmin` | 全局观看统计 |
| `GET /emby/playback-stats` | `AuthUser` | 当前用户的 Emby ActivityLog 播放统计；支持 `days`、`today=1`、`limit`、`sort`、`refresh=1` |
| `GET /admin/emby/activity-logs` | `AuthAdmin` | 原始 Emby 活动日志；默认节流自动刷新，`refresh=1` 强制拉取 |
| `GET /admin/emby/playback-stats` | `AuthAdmin` | 全站、自己或指定用户范围的播放统计；支持 `scope=self|global|user`、`uid`、`days`、`today=1`、`limit`、`sort`、`refresh=1` |
| `GET /admin/emby/playback-stats/{uid}` | `AuthAdmin` | 指定 UID 的 Emby ActivityLog 播放统计 |
| `GET /batch/expiring-users` | `AuthAdmin` | 即将过期用户 |
| `POST /batch/send-reminders` | `AuthAdmin` | 发送到期提醒 |

批量用户接口支持两种目标格式：显式 `uids`（单次最多 200 个）或 `{"select_all":true,"filter":{...}}`（按当前筛选匹配全部用户，最多 5000 个）。`filter` 支持 `role`、`active`、`emby`（`bound` / `unbound`）和 `search`。管理员 / 受保护账号由后端自动跳过。

`/batch/users/emby-unbind-lock` 在 `select_all` 模式下会先把目标收窄到已绑定 Emby 的用户；显式 `uids` 中未绑定 Emby 的用户会计入 `skipped_no_emby`，不视为失败。

`/batch/users/emby-grant-clear` 与上者对偶，用于修复历史迁移把"从未真正开通 Emby"的用户误判为"已用过注册资格"导致无法再使用注册码/邀请码的问题。`select_all` 模式下目标收窄到未绑定 Emby 的用户；对每个目标：已绑定 Emby → `skipped_has_emby`，`PendingEmby` 在飞 → `skipped_pending`（保留其待开通资格），其余清除用户侧 `emby_grant_locked` / `registration_source` / `registration_code` 并从注册码、邀请码及邀请关系回退其使用引用（`regcode_refs_removed` / `invite_refs_removed` 统计回退条数，被占用的码计数低于上限后恢复可用）。管理员账号由后端保护。

### 11.3 Stats 模块

`GET /stats/me`

- 说明：当前用户观看统计（与 `/batch/watch-stats` 同 handler）。
- 认证：登录用户（`AuthUser`）

`GET /stats/user/{uid}`

- 说明：指定用户观看统计。
- 认证：登录用户（`AuthUser`），handler 内对跨用户视图做 admin 断言。

### 11.4 Invite 模块

> 邀请关系与邀请码均以字段形式存于单一状态文档（`internal/store`）；功能详见 [邀请树](../features/invite.md)。

| 方法/路径 | 认证 | 说明 |
| --------- | ---- | ---- |
| `GET /invite/config` | `AuthPublic` | 邀请功能开关与参数 |
| `GET /invite/me` | `AuthUser` | 我的邀请信息与下级 |
| `POST /invite/codes` | `AuthUser` | 生成邀请码；`invite_enabled=false` 时拒绝 |
| `POST /invite/renew-codes` | `AuthUser` | 为已有直属下级生成指名续期码；`invite_enabled=false` 时仍允许 |
| `GET /invite/codes` | `AuthUser` | 列出我的邀请码 |
| `DELETE /invite/codes/{code}` | `AuthUser` | 删除邀请码 |
| `POST /invite/children/{uid}/detach-expired` | `AuthUser` | 删除 Emby 并断开 Emby 已到期或 Web 已禁用的直属下级 |
| `GET /invite/check` | `AuthPublic` | 校验邀请码（IP 限流 10/60s） |
| `POST /invite/use` | `AuthUser` | 使用邀请码 |

### 11.5 Signin 模块

> 签到积分默认仅记录余额；管理员可通过 `[SAR].signin_renewal_enabled` 或 `[Signin].renewal_enabled` 开启积分续期。关闭时 `/signin/me` 的 `renewal.enabled=false`，前端不展示兑换入口。

| 方法/路径 | 认证 | 说明 |
| --------- | ---- | ---- |
| `GET /signin/config` | `AuthPublic` | 签到功能配置 |
| `GET /signin/me` | `AuthUser` | 我的签到状态 |
| `POST /signin` | `AuthUser` | 执行签到 |
| `POST /signin/renew` | `AuthUser` | 使用签到积分续期；消耗积分和续期天数由管理员配置 |
| `GET /signin/history` | `AuthUser` | 签到历史 |

`POST /signin/renew` 成功后在同一次状态写入中扣减 `signin.points` 并延长当前用户 `expired_at`；积分不足返回 `SIGNIN_INSUFFICIENT_POINTS`，功能未开启返回 `SIGNIN_RENEWAL_DISABLED`。

### 11.6 公告（前台）

`GET /announcements`

- 说明：读取面向所有访客的公告列表。公告数据以字段形式存于单一状态文档。
- 认证：公开（`AuthPublic`）
- 功能详见 [公告系统](../features/announcements.md)。

## 12. API Key 模块（概要）

`/api/v1/apikey/*` 接口仅支持 API Key 鉴权（`AuthAPIKey`），用于外部系统接入。完整权限矩阵、调用示例与禁用语义见 [API Key 外部接入](../reference/api-key.md)。端点概览：

| 方法/路径 | 说明 |
| --------- | ---- |
| `GET /apikey/info` | API Key 与账号信息 |
| `GET /apikey/status` | 账号/Key 状态 |
| `POST /apikey/enable` / `POST /apikey/disable` | 启用 / 禁用账号 |
| `POST /apikey/renew` | 续期账号 |
| `POST /apikey/key/refresh` | 刷新 API Key |
| `GET /apikey/permissions` | 查询权限 |
| `PUT /apikey/permissions` | 固定拒绝（不允许自助提权） |
| `POST /apikey/key/disable` / `POST /apikey/key/enable` | 禁用 / 启用 Key 本身 |
| `GET /apikey/emby/status` | Emby 状态 |
| `POST /apikey/emby/kick` | 踢出 Emby 会话 |
| `POST /apikey/use-code` | 使用卡码 |

## 13. 附录

- 管理员接口需要管理员登录态（`AuthAdmin`）。
- 外部系统推荐使用 API Key 访问 `/api/v1/apikey/*`，见 [API Key 外部接入](../reference/api-key.md)。
- 路由速查见 [API 路由索引](../reference/api-index.md)；安全机制见 [安全加固](../guides/security.md)；后端架构与配置见 [Go 后端架构与配置](../reference/backend.md)。
- 如果配置与接口行为不一致，以后端实际返回与运行时 Swagger 为准。
