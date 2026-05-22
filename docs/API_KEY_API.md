# API Key 专用接口文档

本文档面向外部系统集成，说明 `/api/v1/apikey` 前缀下的 API Key 认证接口、权限机制、常用调用、错误码与安全建议。

## 1. 接口说明

- Base URL：`https://your-domain.com/api/v1/apikey`
- 认证方式：`X-API-Key` 或 `Authorization: Bearer/ApiKey`
- 响应格式：统一返回 `success`、`message`、`data`、`timestamp`

## 2. 认证方式

认证头格式与通用响应结构与 [BACKEND_API.md](./BACKEND_API.md) 保持一致；本节仅列 API Key 特有写法。

### 2.1 Header 方式（推荐）

```http
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

### 2.2 Authorization 方式

```http
Authorization: Bearer key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

或者

```http
Authorization: ApiKey key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

## 3. 通用响应格式

请直接参考 [BACKEND_API.md](./BACKEND_API.md) 中“认证与请求规范”小节，避免重复维护。

## 4. 权限与范围

API Key 支持细粒度权限控制，部分接口只在特定权限下可用。

### 4.1 可用权限列表

| 权限 | 说明 |
| ---- | ---- |
| `account:read` | 读取账号信息、状态 |
| `account:write` | 启用 / 禁用 / 续期账号 |
| `emby:read` | 查看 Emby 状态 |
| `emby:write` | 处理 Emby 会话 |

### 4.2 权限不足时返回

如果 API Key 缺少接口所需权限，接口返回 HTTP `403`。

```json
{
  "success": false,
  "message": "API Key 缺少权限: account:write",
  "data": null,
  "timestamp": 1680000000
}
```

### 4.3 接口权限映射

| 接口 | 所需权限 |
| ---- | --------- |
| `/info` | `account:read` |
| `/status` | `account:read` |
| `/enable` | `account:write` |
| `/disable` | `account:write` |
| `/renew` | `account:write` |
| `/key/refresh` | `account:write` |
| `/key/disable` | `account:write` |
| `/key/enable` | `account:write` |
| `/emby/status` | `emby:read` |
| `/emby/kick` | `emby:write` |
| `GET /permissions` | 无（仅需有效 API Key） |
| `PUT /permissions` | 禁止 API Key 自行调用；请在 Web 端管理 |
| `/use-code` | `account:write` |

## 5. 关键接口

### 5.1 账号信息与状态

#### 获取账号信息

`GET /api/v1/apikey/info`

- 认证：API Key
- 权限：`account:write`
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：查询当前 API Key 对应账号的完整信息。
- 示例 cURL：

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/info" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

#### 查询账号状态

`GET /api/v1/apikey/status`

- 认证：API Key
- 权限：`account:write`
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：获取账号激活状态、过期时间、剩余天数和是否被禁用。
- 示例 cURL：

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/status" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

### 5.2 账号管理

#### 启用账号

`POST /api/v1/apikey/enable`

- 认证：API Key
- 权限：`account:write`
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：启用当前账号。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/enable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

#### 禁用账号

`POST /api/v1/apikey/disable`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
  - `Content-Type: application/json`
- 请求体：

```json
{
  "reason": "违规操作"
}
```

- 说明：禁用当前账号。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"reason":"违规操作"}'
```

#### 续期账号

`POST /api/v1/apikey/renew`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
  - `Content-Type: application/json`
- 请求体：

```json
{
  "days": 30
}
```

- 说明：为当前账号续期。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/renew" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"days":30}'
```

### 5.3 API Key 管理

#### 刷新 API Key

`POST /api/v1/apikey/key/refresh`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：刷新当前 API Key，旧 Key 立即失效。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/key/refresh" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

#### 禁用当前 API Key

`POST /api/v1/apikey/key/disable`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：禁止当前 API Key 继续访问。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/key/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

#### 启用当前 API Key

`POST /api/v1/apikey/key/enable`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：启用当前 API Key，若已失效则重新恢复访问。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/key/enable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

### 5.4 Emby 相关

#### 获取 Emby 状态

`GET /api/v1/apikey/emby/status`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：查询当前账号绑定的 Emby 服务状态。
- 示例 cURL：

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/emby/status" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

#### 踢出 Emby 会话

`POST /api/v1/apikey/emby/kick`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
- 说明：踢出当前账号所属 Emby 会话。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/emby/kick" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"
```

### 5.5 注册码/续期码

#### 使用注册码/续期码

`POST /api/v1/apikey/use-code`

- 认证：API Key
- 请求头：
  - `X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy`
  - `Content-Type: application/json`
- 请求体：

```json
{
  "reg_code": "code-xxx",
  "emby_username": "emby_name",
  "emby_password": "Password123"
}
```

- 说明：使用注册码/续期码；当会触发创建 Emby 账号时，需要额外提供 `emby_username` 与 `emby_password`。
- 示例 cURL：

```bash
curl -X POST "https://your-domain.com/api/v1/apikey/use-code" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"reg_code":"code-xxx"}'
```

## 6. 错误码

| HTTP 状态码 | 描述 |
| ---------- | ---- |
| 200 | 成功 |
| 400 | 请求参数错误 |
| 401 | 认证失败（API Key 无效、已禁用或格式错误） |
| 403 | 权限不足或账号被禁用 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

## 7. 使用示例

### 7.1 cURL 示例

```bash
curl -X GET "https://your-domain.com/api/v1/apikey/info" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"

curl -X POST "https://your-domain.com/api/v1/apikey/renew" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"days": 30}'

curl -X POST "https://your-domain.com/api/v1/apikey/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"reason": "违规操作"}'
```

### 7.2 Go 示例

```go
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

const apiBase = "https://your-domain.com/api/v1/apikey"
const apiKey = "key-xxxxxxxxxxxxxxxx-yyyyyyyy"

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

	renew, _ := request(http.MethodPost, "/renew", `{"days":30}`)
	fmt.Println(string(renew))

	disable, _ := request(http.MethodPost, "/disable", `{"reason":"违规操作"}`)
	fmt.Println(string(disable))
}
```

### 7.3 JavaScript 示例

```javascript
const API_BASE = 'https://your-domain.com/api/v1/apikey';
const API_KEY = 'key-xxxxxxxxxxxxxxxx-yyyyyyyy';

const headers = {
  'X-API-Key': API_KEY,
  'Content-Type': 'application/json'
};

async function getInfo() {
  const res = await fetch(`${API_BASE}/info`, { headers });
  return res.json();
}

async function renew(days) {
  const res = await fetch(`${API_BASE}/renew`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ days })
  });
  return res.json();
}

async function disable(reason) {
  const res = await fetch(`${API_BASE}/disable`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ reason })
  });
  return res.json();
}
```

## 8. 安全建议

1. **保护 API Key**:
   - 不要将 API Key 提交到版本控制系统。
   - 不要在公开场合分享 API Key。
   - 定期更换 API Key。

2. **使用 HTTPS**:
   - 生产环境必须使用 HTTPS 传输 API Key。

3. **限制访问**:
   - 仅在需要的系统中使用 API Key。
   - 如果 API Key 泄露，立即刷新并禁用旧 Key。

4. **监控使用**:
   - 定期检查 API Key 的使用情况。
   - 发现异常访问及时禁用 Key。

## 9. 常见问题

### Q: API Key 在哪里获取？

A: 登录前端界面，进入“个人设置” -> “API Key 管理”，可以生成、查看和管理 API Key。

### Q: API Key 可以用于哪些接口？

A: 本套接口（`/api/v1/apikey/*`）专门为 API Key 设计。前端 Web 或移动端使用的接口（如 `/api/v1/users/*`）一般要求 Token 认证。

### Q: 刷新 API Key 后，旧的还能用吗？

A: 不能。刷新后旧 API Key 立即失效，请及时更新所有外部系统中的配置。

### Q: 如何判断账号是否过期？

A: 使用 `/api/v1/apikey/status` 接口，检查返回的 `is_expired` 字段。

### Q: 续期天数有限制吗？

A: 是的，单次续期天数通常限制在 1-3650 天（10 年）之间。


### Q: 如何查看当前 API Key 的可用权限？

A: 使用 `/api/v1/apikey/permissions`（GET）获取当前权限与完整权限列表；权限修改只能在 Web 端个人设置中完成，API Key 不能自行调用 `PUT /permissions` 提权。
