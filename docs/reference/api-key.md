# API Key 外部接入

本文面向外部系统集成，说明 `/api/v1/apikey/*` 前缀下的 API Key 认证接口：认证方式、通用响应、权限模型、关键接口、错误码以及调用示例与安全建议。配置项与整体架构见 [Go 后端架构与配置](../reference/backend.md)，完整路由清单见 [API 路由索引](../reference/api-index.md)，逐接口字段见 [后端 API 详参](../reference/backend-api.md)。

> 与浏览器端的 Cookie / Bearer 会话不同，`/apikey/*` 接口专为「机器对机器」的长期凭证设计：使用 `X-API-Key`（或 `Authorization`）携带密钥，不依赖浏览器会话 Cookie。

## 1. 接口概览

| 项目 | 说明 |
| ---- | ---- |
| Base URL | `https://your-domain.com/api/v1/apikey` |
| 认证方式 | `X-API-Key` 头，或 `Authorization: ApiKey/Bearer <key>`，或 `?apikey=<key>`（需开启 `allow_query`） |
| 鉴权级别 | 全部路由为 `AuthAPIKey`（见 `internal/api/routes.go` 的 `registerAPIKeyRoutes`） |
| 响应格式 | 统一 envelope：`success` / `code` / `error_code` / `message` / `data` / `timestamp` |
| 密钥格式 | `key-` 前缀加 40 位随机串，例如 `key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` |
| 默认限速 | 每把 Key 每分钟默认 300 次（`RateLimit.api_key_default_per_minute`），可逐 Key 覆盖 |

API Key 始终绑定到「生成它的那个用户账号」。通过 `/apikey/*` 接口执行的所有操作（查看信息、续期、启停账号、踢 Emby 会话等）都只作用于该账号本身，无法操作其他用户。

## 2. 认证方式

后端在 `internal/api/app.go` 的 `authenticateAPIKey` 中按以下顺序提取密钥（命中即止）：

1. `X-API-Key` 请求头；
2. `Authorization: Bearer <key>` 或 `Authorization: ApiKey <key>`（大小写不敏感）；
3. URL 查询参数 `?apikey=<key>`。

### 2.1 Header 方式（推荐）

```http
X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### 2.2 Authorization 方式

```http
Authorization: Bearer key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

或：

```http
Authorization: ApiKey key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### 2.3 查询参数方式（受限）

```http
GET /api/v1/apikey/status?apikey=key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

查询参数方式仅在同时满足以下两个条件时才被接受，否则按密钥无效处理（HTTP 401）：

- 该 Key 是「多键」类型（即在「个人设置」里通过 `/api/v1/users/me/apikeys` 创建的 Key）；
- 该 Key 显式开启了 `allow_query`（创建或更新时设置）。

「单键（legacy）」类型的 Key（即通过 `/api/v1/auth/apikey` 生成、与账号一对一的旧式 Key）**不支持查询参数认证**，只能走请求头。由于查询参数会出现在访问日志、浏览器历史、Referer 等位置，除非确有必要，否则建议始终使用请求头方式。

### 2.4 校验流程

`authenticateAPIKey` 命中密钥后依次校验：

1. 用 SHA-256 哈希后在状态存储中查找对应 Key 记录（`FindAPIKeyByHash`）；
2. 绑定的账号必须 `Active`（被禁用 / 已到期的账号一律拒绝）；
3. 查询参数方式额外要求 Key 为多键类型且 `allow_query` 为真；
4. 命中该 Key 的每分钟限速（默认 300 次，可逐 Key 配置）；
5. 多键类型会记录一次使用（`request_count` 自增、刷新 `last_used`）。

上述任一步骤失败都会让认证返回「无效」，由路由分发层统一回 HTTP 401（`error_code: AUTH_APIKEY_INVALID`）。注意：**超出限速也表现为 401**，而非 429——这是机器接入侧需要留意的差异。

## 3. 通用响应格式

所有接口返回同一 envelope 结构（`internal/api/response.go`）：

```json
{
  "success": true,
  "code": 200,
  "message": "OK",
  "data": { },
  "timestamp": 1680000000
}
```

| 字段 | 类型 | 说明 |
| ---- | ---- | ---- |
| `success` | bool | 是否成功 |
| `code` | int | HTTP 状态码（与响应状态一致） |
| `error_code` | string | 失败时的业务错误码（成功时省略），见下文「错误码」 |
| `message` | string | 人类可读说明（失败文案会经脱敏处理） |
| `data` | object/null | 业务数据，成功时存在；失败时通常省略 |
| `timestamp` | int | Unix 秒级时间戳 |

失败响应示例：

```json
{
  "success": false,
  "code": 401,
  "error_code": "AUTH_APIKEY_INVALID",
  "message": "API Key 无效",
  "timestamp": 1680000000
}
```

## 4. 权限与范围

API Key 记录上带有一个 `permissions` 字段（字符串数组），可取值如下：

| 权限 | 含义 |
| ---- | ---- |
| `account:read` | 读取账号信息、状态 |
| `account:write` | 启用 / 禁用 / 续期账号 |
| `emby:read` | 查看 Emby 状态 |
| `emby:write` | 处理 Emby 会话（踢出） |

新创建的 Key 默认携带上述全部 4 项权限（`internal/api/apikey_handlers.go` 的 `defaultPermissions`）。

> 重要：当前实现中，`/apikey/*` 路由**只校验「密钥有效 + 账号 Active」这一层（`AuthAPIKey`），并不在各端点上逐项强制 `permissions`**。也就是说，只要持有一把有效且账号正常的 Key，即可调用本前缀下的全部接口。`permissions` 字段目前主要用于展示与未来扩展；`/apikey/info`、`/apikey/permissions` 会把它原样回显，但不会因为缺少某项权限而拒绝请求。
>
> 这一点与旧文档「接口权限映射表 / 缺少 `account:write` 返回 403」的描述不同，旧描述并不符合当前代码，已据实改写。

### 4.1 API Key 不能自行修改权限

`PUT /api/v1/apikey/permissions` 被固定拒绝：对应 handler 直接返回 HTTP 403（`error_code: API_KEY_SELF_PERMISSION_FORBIDDEN`，文案「不允许通过当前 API Key 修改自身权限」）。换言之，**持有 Key 的一方无法用 Key 给自己提权**。权限只能在 Web 端「个人设置」里管理。

## 5. 关键接口

下表为 `registerAPIKeyRoutes` 中登记的全部路由（均为 `AuthAPIKey`）：

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/api/v1/apikey/info` | 账号信息 + 当前 Key 权限 |
| GET | `/api/v1/apikey/status` | 账号激活状态与到期时间 |
| POST | `/api/v1/apikey/enable` | 启用当前账号 |
| POST | `/api/v1/apikey/disable` | 禁用当前账号（并失效全部会话） |
| POST | `/api/v1/apikey/renew` | 用续期码为当前账号续期 |
| POST | `/api/v1/apikey/key/refresh` | 刷新（轮换）单键 Key，旧值立即失效 |
| GET | `/api/v1/apikey/permissions` | 查看当前 Key 权限与完整权限列表 |
| PUT | `/api/v1/apikey/permissions` | 固定拒绝（见 4.1） |
| POST | `/api/v1/apikey/key/disable` | 禁用当前 Key |
| POST | `/api/v1/apikey/key/enable` | 启用当前 Key |
| GET | `/api/v1/apikey/emby/status` | 查看 Emby 服务状态 |
| POST | `/api/v1/apikey/emby/kick` | 踢出当前账号的 Emby 会话 |
| POST | `/api/v1/apikey/use-code` | 使用注册码 / 邀请码 / 续期码 |

### 5.1 账号信息与状态

#### 获取账号信息

`GET /api/v1/apikey/info`

返回当前 Key 绑定账号的完整信息（`user`）以及当前 Key 的权限列表（`permissions`）。

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/info" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

`data` 形如：

```json
{
  "user": { "uid": 1, "username": "alice", "active": true, "expired_at": 1700000000 },
  "permissions": ["account:read", "account:write", "emby:read", "emby:write"]
}
```

#### 查询账号状态

`GET /api/v1/apikey/status`

返回账号是否激活与到期时间。

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/status" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

`data` 形如：

```json
{
  "active": true,
  "expired_at": 1700000000
}
```

> `expired_at=-1` 表示永久；`expired_at=0` 表示未设置到期 / 无 Emby 权益哨兵；正数为 Unix 秒。是否已过期可由调用方对照当前时间判断；账号被管理员禁用或到期失活时，`active` 会为 `false`，且后续认证会直接返回 403/401。

### 5.2 账号管理

#### 启用账号

`POST /api/v1/apikey/enable`

将当前账号置为激活（`active=true`），返回更新后的 `user`。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/enable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

#### 禁用账号

`POST /api/v1/apikey/disable`

将当前账号置为非激活（`active=false`），并**立即失效该账号在所有设备上的会话**，返回更新后的 `user`。该接口无需请求体（旧文档要求传 `{"reason": ...}` 的说法不符合当前实现）。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

> 注意：禁用账号后，该账号关联的 API Key 也会因「账号非 Active」而无法再通过认证。若要重新启用，需另一条仍有效的途径（如 Web 端管理员），单凭被禁用账号自己的 Key 无法再调用 `/enable`。

#### 续期账号

`POST /api/v1/apikey/renew`

为当前账号续期。**续期天数来自所提供的续期码本身，而非请求体里的天数**——必须在请求体中提供一个有效的续期码（`reg_code`），不能裸传 `days`。该接口复用与 Web 端自助续费相同的逻辑（`handleRenew`）。

请求体：

```json
{
  "reg_code": "code-xxxxxxxx"
}
```

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/renew" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-xxxxxxxx"}'
```

约束（与 Web 端一致）：

- 缺少 `reg_code` → 400（`AUTH` 类业务码 `REGCODE` 相关，文案「续期需要提供注册码」）；
- 该码必须是「续期码」类型且属于当前用户、未用完、未过期，否则 400；
- 续期成功后返回 `expire_status`、`expired_at` 与最新 `user`。

关于续期码 / 注册码的类型区分，见 [注册码与卡码](../features/regcodes.md)。

### 5.3 API Key 自管理

Twilight 同时支持两类 Key：

- **单键（legacy）**：与账号一对一，通过 `/api/v1/auth/apikey` 生成；
- **多键（v2）**：通过 `/api/v1/users/me/apikeys` 创建，单账号可有多把，各自独立的限速、`allow_query` 与启停状态。

下面的自管理接口对当前正在使用的那把 Key 生效。

#### 刷新（轮换）Key

`POST /api/v1/apikey/key/refresh`

生成一把新的**单键** Key 并返回明文，旧的单键 Key 立即失效。响应 `data.apikey` 为新明文，请妥善保存（仅此一次返回）。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/key/refresh" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

> 该端点本质是「重新生成单键 Key」。如果你用的是多键（v2）Key，要轮换它请改用 Web 端 / `/api/v1/users/me/apikeys` 删除并重建。

#### 禁用当前 Key

`POST /api/v1/apikey/key/disable`

- 若当前请求用的是多键（v2）Key：将该 Key 置为 `enabled=false`；
- 若用的是单键（legacy）Key：将账号上的单键状态关闭。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/key/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

> 自我禁用后，这把 Key 即刻失去访问能力，无法再用它自己调用 `/key/enable`。多键 Key 可由账号在 Web 端重新启用；单键可重新生成。

#### 启用当前 Key

`POST /api/v1/apikey/key/enable`

仅对多键（v2）Key 有效：将其 `enabled=true`。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/key/enable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

#### 查看 / 拒绝修改权限

- `GET /api/v1/apikey/permissions`：返回当前 Key 的 `permissions` 与全部可选权限 `all_permissions`；
- `PUT /api/v1/apikey/permissions`：固定返回 403（见 4.1），用于明确告知调用方「不能自助提权」。

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/permissions" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

### 5.4 Emby 相关

#### 获取 Emby 状态

`GET /api/v1/apikey/emby/status`

复用通用的 Emby 健康检查（`handleEmbyStatus`），返回服务在线状态、版本、活跃会话数，以及当前账号是否已同步 Emby（`is_synced`）等。

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/emby/status" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

#### 踢出 Emby 会话

`POST /api/v1/apikey/emby/kick`

踢出**当前账号自己**的 Emby 会话（内部以当前账号 UID 调用踢出逻辑），无法踢其他用户。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/emby/kick" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

### 5.5 注册码 / 邀请码 / 续期码

#### 使用卡码

`POST /api/v1/apikey/use-code`

通用「用码」入口（`handleUseCode`），可消费注册码、邀请码或续期码。请求体接受 `reg_code` 或 `code`（二选一）：

```json
{
  "reg_code": "code-xxxxxxxx",
  "emby_username": "emby_name",
  "emby_password": "Password123"
}
```

| 字段 | 必填 | 说明 |
| ---- | ---- | ---- |
| `reg_code` / `code` | 是 | 要使用的卡码（注册码 / 邀请码 / 续期码） |
| `emby_username` | 视情况 | 当该码会触发「为账号创建 Emby 用户」时需要 |
| `emby_password` | 视情况 | 同上 |
| `check_only` | 否 | 传 `true` 时只预览该码（不消费），返回校验结果 |

行为要点：

- 先 `previewCode` 校验该码是否有效；无效 → 400；
- 若该码会授予 Emby 注册而账号已绑定 Emby，则返回 400（提示改用续期码）；
- 创建 Emby 用户前会检查 Emby 容量上限，超限返回 409（`EMBY_CAPACITY_REACHED`）；
- 邀请码还会校验「不能用自己的码、不能重复加入邀请树、目标用户匹配、邀请人可用」等约束。

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/use-code" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-xxxxxxxx"}'
```

注册码、邀请码、续期码的语义与区别详见 [注册码与卡码](../features/regcodes.md) 与 [邀请树](../features/invite.md)。

## 6. 错误码

失败响应同时给出 HTTP 状态码（`code`）与业务错误码（`error_code`）。本前缀下常见的错误码：

| HTTP | error_code | 触发场景 |
| ---- | ---- | ---- |
| 400 | `BAD_REQUEST` / 领域码 | 请求参数错误（如缺续期码、卡码为空等，领域码见下） |
| 401 | `AUTH_APIKEY_INVALID` | Key 缺失 / 无效 / 已禁用 / 账号非 Active；或超出每分钟限速 |
| 403 | `AUTH_ACCOUNT_DISABLED` | 账号被管理员禁用 |
| 403 | `AUTH_ACCOUNT_EXPIRED` | 账号有效期已到期 |
| 403 | `API_KEY_SELF_PERMISSION_FORBIDDEN` | 试图用 Key 修改自身权限（`PUT /permissions`） |
| 404 | `NOT_FOUND` / `INVITE_NOT_FOUND` 等 | 资源不存在 |
| 409 | `EMBY_CAPACITY_REACHED` | 用码创建 Emby 用户时容量已满 |
| 500 | `INTERNAL` | 服务器内部错误 |

部分领域级错误码（来自 `internal/api/errcode.go`）：

| error_code | 含义 |
| ---- | ---- |
| `AUTH_APIKEY_EMPTY` | Key 为空（多见于 API Key 登录入口） |
| `CODE_EMPTY` / `CODE_INVALID` | 卡码为空 / 无效 |
| `CODE_ALREADY_EMBY_BOUND` | 账号已绑定 Emby，应改用续期码 |
| `INVITE_*` | 邀请码相关约束（自用、已有上级、目标不符等） |

> 旧文档列出的「`API Key 缺少权限: account:write` + 403」并不存在于当前代码，已删除。当前实现下，缺少某项 `permissions` 不会被拒；真正会拒绝的是「Key 无效 / 账号非 Active / 自助改权限」这几类。

## 7. 调用示例

### 7.1 cURL

```bash
# 查看账号信息
curl -X GET "https://your-domain.com/api/v1/apikey/info" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

# 用续期码续期
curl -X POST "https://your-domain.com/api/v1/apikey/renew" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"reg_code": "code-xxxxxxxx"}'

# 禁用当前账号（无需请求体）
curl -X POST "https://your-domain.com/api/v1/apikey/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
```

### 7.2 Go

```go
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

const apiBase = "https://your-domain.com/api/v1/apikey"
const apiKey = "key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

func request(method, path, body string) ([]byte, error) {
	req, err := http.NewRequest(method, apiBase+path, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func main() {
	info, _ := request(http.MethodGet, "/info", "")
	fmt.Println(string(info))

	renew, _ := request(http.MethodPost, "/renew", `{"reg_code":"code-xxxxxxxx"}`)
	fmt.Println(string(renew))
}
```

### 7.3 JavaScript

```javascript
const API_BASE = 'https://your-domain.com/api/v1/apikey';
const API_KEY = 'key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx';

const headers = {
  'X-API-Key': API_KEY,
  'Content-Type': 'application/json',
};

async function getInfo() {
  const res = await fetch(`${API_BASE}/info`, { headers });
  return res.json();
}

async function renew(regCode) {
  const res = await fetch(`${API_BASE}/renew`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ reg_code: regCode }),
  });
  return res.json();
}

async function getStatus() {
  const res = await fetch(`${API_BASE}/status`, { headers });
  return res.json();
}
```

## 8. 安全建议

1. **保护密钥**：不要把 API Key 提交到版本库或在公开场合分享；尽量用环境变量 / 密钥管理服务下发。
2. **强制 HTTPS**：生产环境务必走 HTTPS，避免密钥在传输中泄露。
3. **优先请求头**：避免使用 `?apikey=` 查询参数（会进访问日志 / 历史 / Referer）；确需查询参数时再开 `allow_query`。
4. **逐 Key 限速**：多键（v2）可为每把 Key 单独设置 `rate_limit`，给高频外部系统与低频脚本分配不同配额。
5. **泄露即轮换**：怀疑泄露时立即 `/key/disable` 或在 Web 端删除重建；单键可用 `/key/refresh` 轮换，旧值即时失效。
6. **最小化分发**：仅在确需的系统中部署密钥，并定期审计 `request_count` / `last_used`。
7. **账号即边界**：一把 Key 的能力上限就是其绑定账号本身能做的事，不会因为 Key 而越权操作其他用户。

## 9. 常见问题

**Q：API Key 在哪里获取？**

A：登录 Web 端，进入「个人设置」→ API Key 管理，可生成单键、创建多键、查看与启停。详见 [开发指南](../guides/development.md) 与 [后端 API 详参](../reference/backend-api.md)。

**Q：API Key 能用于哪些接口？**

A：本前缀（`/api/v1/apikey/*`）是为 Key 设计的。此外，多数 `AuthUser` 接口也接受 `Authorization: Bearer <key>`（与会话 Token 同一通道），但浏览器端业务一般走会话 Cookie。

**Q：刷新 / 禁用后旧 Key 还能用吗？**

A：不能。`/key/refresh` 后旧的单键立即失效；`/key/disable` 后该 Key 立即无法认证。请同步更新所有外部系统配置。

**Q：怎么判断账号是否过期？**

A：调用 `/api/v1/apikey/status`，对照 `active` 与 `expired_at`（0 表示无到期）。账号到期失活后，认证会直接以 403（`AUTH_ACCOUNT_EXPIRED`）告知。

**Q：续期能传天数吗？**

A：不能。`/renew` 的续期天数由你提供的「续期码」决定，必须在请求体里传 `reg_code`，不能裸传 `days`。

**Q：用 Key 能给自己加权限吗？**

A：不能。`PUT /api/v1/apikey/permissions` 固定返回 403（`API_KEY_SELF_PERMISSION_FORBIDDEN`），权限只能在 Web 端「个人设置」中调整。需要说明的是，当前实现并不在各端点上逐项强制 `permissions`：只要 Key 有效且账号正常，即可调用本前缀下全部接口。
