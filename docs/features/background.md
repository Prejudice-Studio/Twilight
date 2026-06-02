# 背景与头像

Twilight 允许用户自定义个人主题背景（渐变 + 背景图）与头像。本文说明这套「受控资源」模型：上传的图片**不**直接对外暴露磁盘上的 `uploads/` 目录，而是统一通过带鉴权的 API 路由 `/api/v1/users/assets/{kind}/{filename}` 读取；同时说明相关配置项、API 列表与安全规则。

实现集中在 `internal/api/upload_handlers.go`（上传 / 资源访问 / 背景配置净化）、`internal/api/handlers.go`（`handleGetBackground` / `handleGetAvatar` 等读取入口）、`internal/api/safepath.go`（路径穿越防护）与 `internal/config/config.go`（上传目录与大小上限）。

## 受控资源读取模型

传统做法是把上传目录直接挂到 Web 服务器（例如 Nginx 暴露 `/uploads/`）。Twilight 不这么做，原因是：

- 直接暴露目录会让任意人可以遍历、探测其他用户上传的文件名；
- 文件名若可预测或可被用户控制，端点会沦为目录探测 / SSRF 的跳板。

Twilight 的做法是：

1. **上传时由服务端生成随机文件名**：文件名为 `crypto/rand` 产生的 16 位十六进制串加上由 MIME 嗅探推断出的扩展名，例如 `0123456789abcdef.png`。客户端无法控制最终文件名（见 `randomCode` 与 `uploadImageExtension`）。
2. **资源按 `kind` + `filename` 落盘到上传目录下的子目录**：头像写到 `<upload_folder>/avatar/`，背景写到 `<upload_folder>/background/`，Server Icon 写到 `<upload_folder>/server-icon/`。
3. **读取统一走鉴权路由**：前端引用图片时使用 `/api/v1/users/assets/{kind}/{filename}`，由 `handleAsset` 校验 `kind` 与文件名后再 `http.ServeFile` 返回。该路由需要登录会话或 Bearer Token（鉴权级别 `AuthUser`）。

> 部署时**不要**用 Nginx 等反向代理直接暴露 `uploads/` 目录；所有图片都应经由上述 API 路由读取。

## 配置项

上传目录与单文件大小上限由 `internal/config/config.go` 读取，对应的 TOML 字段如下：

```toml
[API]
# 上传根目录，默认 "uploads"（相对工作目录）。
# 配置键：API.upload_folder（也兼容顶层裸键 upload_folder）。
upload_folder = "./uploads"

# 单个上传文件大小上限（字节），默认 5 MiB（5 * 1024 * 1024 = 5242880）。
# 配置键：API.max_upload_size（也兼容顶层裸键 max_upload_size）。
max_upload_size = 5242880
```

| 字段 | 配置键 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 上传目录 | `API.upload_folder` / `upload_folder` | `uploads` | 头像 / 背景 / Server Icon 的落盘根目录；为空时回退为 `uploads` |
| 上传大小上限 | `API.max_upload_size` / `max_upload_size` | `5242880`（5 MiB） | 头像与背景上传的字节上限；Server Icon 另有 2 MiB 上限（见下文） |

此外还有两个限流配置（`internal/config/config.go`，单位为「每分钟允许次数」）：

| 字段 | 配置键 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 上传限流 | `RateLimit.upload_per_minute` / `rate_limit_upload_per_minute` | `60` | 头像 / 背景上传的每用户每分钟次数 |
| Server Icon 限流 | `RateLimit.admin_icon_per_minute` / `rate_limit_admin_icon_per_minute` | `20` | 管理员上传 Server Icon 的每分钟次数 |

## API 列表

所有路由定义见 `internal/api/routes.go`。统一返回 envelope `{ success, code, message, data, timestamp }`。

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/users/{uid}/avatar` | `AuthUser` | 读取头像（返回 `avatar` / `uid` / `username`）；仅限本人或管理员 |
| `POST` | `/api/v1/users/me/avatar/upload` | `AuthUser` | 上传头像（multipart，字段名 `file`） |
| `DELETE` | `/api/v1/users/me/avatar` | `AuthUser` | 清空头像 |
| `GET` | `/api/v1/users/{uid}/background` | `AuthUser` | 读取背景配置（返回 `background` JSON 字符串）；仅限本人或管理员 |
| `PUT` | `/api/v1/users/me/background` | `AuthUser` | 更新背景配置（渐变 / 背景图 / 模糊 / 透明度等） |
| `POST` | `/api/v1/users/me/background/upload` | `AuthUser` | 上传背景图（multipart，字段名 `file`，可带 `type=light\|dark`） |
| `DELETE` | `/api/v1/users/me/background` | `AuthUser` | 清空背景配置 |
| `GET` | `/api/v1/users/assets/{kind}/{filename}` | `AuthUser` | 读取已上传资源；`kind` 限 `avatar` / `background` |
| `GET` | `/api/v1/system/server-icon` | `AuthPublic` | 读取站点 Server Icon（免登录） |
| `POST` | `/api/v1/system/admin/server-icon/upload` | `AuthAdmin` | 管理员上传 Server Icon |

> `handleGetBackground` 与 `handleGetAvatar` 都做了越权收口：若路径 `uid` 既不是当前登录用户、当前用户也不是管理员，会直接返回 `USER_NOT_FOUND`（404），避免登录用户通过枚举 `uid` 反查他人用户名 / 背景偏好。

### 头像上传响应示例

```json
{
  "success": true,
  "code": "OK",
  "message": "上传成功",
  "data": {
    "avatar_url": "/api/v1/users/assets/avatar/0123456789abcdef.png",
    "url": "/api/v1/users/assets/avatar/0123456789abcdef.png",
    "filename": "0123456789abcdef.png"
  },
  "timestamp": 0
}
```

### 背景上传响应示例

背景上传可携带 `type=light` 或 `type=dark` 表单字段，分别只更新浅色 / 深色背景图，保留另一侧设置；不带 `type` 时按旧行为同时覆盖浅深两侧（兼容旧客户端）。

```json
{
  "success": true,
  "code": "OK",
  "message": "上传成功",
  "data": {
    "url": "/api/v1/users/assets/background/0123456789abcdef.png",
    "type": "light",
    "filename": "0123456789abcdef.png"
  },
  "timestamp": 0
}
```

## 背景配置数据模型

背景配置以 **JSON 字符串**形式存放在用户记录的 `Background` 字段中（即单一状态文档内的用户字段，参见 `internal/store`，并非独立数据表）。`handleUpdateBackground` 通过 `sanitizedBackgroundConfig` 净化后写入，结构如下：

| 字段 | 类型 | 取值范围 | 说明 |
| --- | --- | --- | --- |
| `lightBg` / `darkBg` | string | 安全渐变表达式或空 | 浅 / 深色背景的 CSS 渐变 |
| `lightBgImage` / `darkBgImage` | string | `url("…")` 或空 | 浅 / 深色背景图，只允许本系统上传资源 |
| `lightFlow` / `darkFlow` | bool | — | 是否启用流动动画 |
| `lightBlur` / `darkBlur` | int | `0`–`30` | 模糊强度（超界会被夹紧） |
| `lightOpacity` / `darkOpacity` | int | `10`–`100` | 不透明度（超界会被夹紧） |

`PUT /api/v1/users/me/background` 也兼容只传一个 `background` 或 `url` 字符串字段：若该字符串本身是合法 JSON 配置则按上表解析，否则当作单一渐变值处理（同时写入 `lightBg` 与 `darkBg`）。

## 安全规则

上传与资源访问链路的固定模板为「限流 → multipart 解析 → MIME 嗅探 → 路径净化 → 原子写盘 → 更新用户 / 配置」。具体安全措施：

### 上传

- **限流**：头像 / 背景上传按用户限流（默认每分钟 60 次），Server Icon 按管理员限流（默认每分钟 20 次）。超限返回 `UPLOAD_RATE_LIMITED`（429）。
- **大小上限**：先以 `MaxUploadSize` 解析 multipart，再用 `io.LimitReader` 读取至多 `MaxUploadSize+1` 字节并校验实际长度，超限返回 `UPLOAD_FILE_TOO_LARGE`（413）。Server Icon 上限为 `min(2 MiB, MaxUploadSize)`。
- **仅允许图片**：通过 `http.DetectContentType` 嗅探内容（不信任客户端声明的 `Content-Type`），仅接受 `image/jpeg`、`image/png`、`image/gif`、`image/webp`、`image/bmp`，否则返回 `UPLOAD_TYPE_NOT_ALLOWED`（400）。扩展名由嗅探结果决定，而非来自原始文件名。
- **服务端生成文件名**：文件名为 16 位 `crypto/rand` 十六进制串加白名单扩展名，客户端不可控。
- **写盘前再次校验文件名**：落盘前用 `uploadFilenamePattern`（`^[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$`）复核，任何不匹配直接判失败。
- **路径穿越防护**：`UploadDir` 来自管理员可改的配置，可能被填成 `../etc`、相对路径或父目录为符号链接。落盘前先用 `ResolveWithinRoot`（`internal/api/safepath.go`，按 `Abs → Clean → Rel` 三步校验路径未越出根目录）解析出 `kind` 子目录与目标文件，再 `os.Lstat` 复核目录不是符号链接、确为目录，挡住 `MkdirAll` 之后被人 race 换成符号链接的 TOCTOU 路径。
- **原子写盘**：用 `store.WriteFileAtomicSync`（临时文件 → fsync → rename → 目录 sync，且临时文件以 `O_NOFOLLOW|O_EXCL` 打开）写入，避免上传途中崩溃在目标位置留下 0 字节 / 半字节文件，并挡住「攻击者提前把 `target.tmp` 换成符号链接」的 TOCTOU 攻击。目录权限 `0o700`，文件权限 `0o600`。

### 资源访问

- **`kind` 白名单**：`handleAsset` 仅接受 `avatar` 与 `background`，其它一律返回 `ASSET_NOT_FOUND`（404）。
- **文件名白名单**：返回前用同一条 `uploadFilenamePattern` 校验文件名；任何不匹配当作 404，避免端点被用作目录探测。
- **返回前重新解析绝对路径**：通过 `resolveUploadAssetPath` → `ResolveWithinRoot` 再次确认目标绝对路径落在上传目录内，越界返回 404。
- **需登录**：路由级别为 `AuthUser`，未登录无法直接读取资源。

### 背景配置净化（`sanitizedBackgroundConfig`）

- **渐变 CSS（`lightBg` / `darkBg`）**：仅允许安全渐变表达式。长度 ≤ 2000，且不得含 `\x00 \r \n < > ; { }`、不得含 `url(`、不得含 `@`；并且必须以 `linear-gradient` / `radial-gradient` / `conic-gradient` / `repeating-linear-gradient` / `repeating-radial-gradient` 开头，否则返回 `USER_BACKGROUND_INVALID`（400）。这是为了防止普通用户用恶意背景对管理员发起 XSS 或资源外链。
- **背景图（`lightBgImage` / `darkBgImage`）**：**只允许引用本系统已上传的背景资源**。值会剥掉可选的 `url("…")` 包裹后，要求严格以 `/api/v1/users/assets/background/` 开头，且剩余文件名匹配 `uploadFilenamePattern`、不含路径分隔符；任何**外部 URL**（包括指向 `http://127.0.0.1/…` 等私网 / 回环地址的 URL）都会被直接拒绝。长度 ≤ 1000，且不得含 `\x00 \r \n < >`。

> 说明：背景图字段并不是「先放行外部 URL，再用 SSRF 黑名单拦截私网 IP」，而是「整体拒绝一切外部 URL，只放行本系统上传的背景资源」，从根上消除背景图引入 SSRF / 外链的可能。项目里确有一套面向出站 IP 的 SSRF 黑名单（拒绝链路本地 `169.254.0.0/16`、IPv6 `fe80::/10`、未指定地址、云元数据 `100.100.100.200` 等，见 `internal/api/outbound_url.go`），但它的作用对象是 Emby / Bangumi / Telegram / TMDB 这类后端**出站**客户端的 base URL，与背景图设置是两条独立链路。

## 相关错误码

| 错误码 | HTTP | 触发场景 |
| --- | --- | --- |
| `UPLOAD_RATE_LIMITED` | 429 | 上传过于频繁 |
| `UPLOAD_INVALID_PAYLOAD` | 400 | multipart 解析失败 |
| `UPLOAD_FILE_MISSING` | 400 | 缺少 `file` 字段 |
| `UPLOAD_FILE_TOO_LARGE` | 413 | 超过大小上限 |
| `UPLOAD_TYPE_NOT_ALLOWED` | 400 | 非允许的图片类型 |
| `UPLOAD_DIR_INVALID` | 500 | 上传目录无效（路径越界 / 符号链接 / 非目录） |
| `UPLOAD_DIR_CREATE_FAILED` | 500 | 创建上传目录失败 |
| `UPLOAD_SAVE_FAILED` | 500 | 写盘失败 |
| `ASSET_NOT_FOUND` | 404 | 资源不存在 / `kind` 或文件名非法 |
| `USER_BACKGROUND_INVALID` | 400 | 背景配置非法（渐变 / 背景图未通过净化） |
| `USER_NOT_FOUND` | 404 | 越权读取他人头像 / 背景 |

## 相关文档

- 鉴权级别、统一响应 envelope 与 CORS 机制：见 [Go 后端架构与配置](../reference/backend.md)。
- 完整 API 详参：见 [后端 API 详参](../reference/backend-api.md) 与 [API 路由索引](../reference/api-index.md)。
- 出站 SSRF 防护与整体安全加固：见 [安全加固](../guides/security.md)。
