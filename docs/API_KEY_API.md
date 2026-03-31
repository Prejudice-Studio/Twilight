# API Key 专用接口文档

## 概述

本套接口专门为外部系统设计，使用 API Key 进行认证，与前端使用的接口完全独立。

**基础 URL**: `https://your-domain.com/api/v1/apikey`

**认证方式**: 所有接口都需要在请求头中提供 API Key

## 认证方式

### 方式一：X-API-Key Header（推荐）

```http
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

### 方式二：Authorization Header

```http
Authorization: Bearer key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

或

```http
Authorization: ApiKey key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

## 响应格式

所有接口都遵循统一的响应格式：

```json
{
    "success": true,
    "message": "操作成功",
    "data": { ... },
    "timestamp": 1234567890
}
```

**错误响应**:

```json
{
    "success": false,
    "message": "错误信息",
    "data": null,
    "timestamp": 1234567890
}
```

## 权限系统

API Key 支持细粒度的权限控制。每个 API Key 可以被限制为只允许特定操作。

### 权限范围

| 权限 | 说明 |
|------|------|
| `account:read` | 读取账号信息、状态 |
| `account:write` | 启用/禁用账号、续期 |
| `score:read` | 查看积分、排行榜、历史 |
| `score:write` | 签到等积分写操作 |
| `emby:read` | 查看 Emby 状态 |
| `emby:write` | 踢出 Emby 会话 |

默认情况下，新创建的 API Key 拥有全部权限（向后兼容）。

### 获取权限列表

```http
GET /api/v1/apikey/permissions
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "data": {
        "permissions": ["account:read", "account:write", "score:read"],
        "all_permissions": ["account:read", "account:write", "score:read", "score:write", "emby:read", "emby:write"]
    }
}
```

### 更新权限

```http
PUT /api/v1/apikey/permissions
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
Content-Type: application/json
```

```json
{
    "permissions": ["account:read", "score:read"]
}
```

也可通过前端 Token 认证接口管理：
- `GET /api/v1/auth/apikey/permissions`
- `PUT /api/v1/auth/apikey/permissions`

### 权限不足错误

当 API Key 缺少所需权限时，返回 `403`:
```json
{
    "success": false,
    "message": "API Key 缺少权限: account:write"
}
```

### 各接口所需权限

| 接口 | 需要权限 |
|------|---------|
| 获取账号信息 / 状态 | `account:read` |
| 启用 / 禁用 / 续期账号 | `account:write` |
| 获取积分 / 历史 / 排行榜 | `score:read` |
| 签到 | `score:write` |
| 获取 Emby 状态 | `emby:read` |
| 踢出 Emby 会话 | `emby:write` |
| 权限管理 / Key 管理 | 无额外权限（仅需有效 Key） |

---

## 接口列表

### 1. 获取账号信息

获取完整的账号信息。

**请求**:
```http
GET /api/v1/apikey/info
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "获取成功",
    "data": {
        "uid": 1,
        "username": "user123",
        "email": "user@example.com",
        "role": 1,
        "role_name": "NORMAL",
        "active": true,
        "emby_id": "xxx",
        "expired_at": 1735689600,
        "is_expired": false,
        "is_permanent": false,
        "days_left": 30,
        "score": 1000,
        "auto_renew": false
    }
}
```

### 2. 获取账号状态（简化版）

获取账号的基本状态信息。

**请求**:
```http
GET /api/v1/apikey/status
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "获取成功",
    "data": {
        "active": true,
        "emby_id": "xxx",
        "is_expired": false,
        "days_left": 30
    }
}
```

**说明**:
- `days_left`: 剩余天数，`-1` 表示永久有效

### 3. 启用账号

启用当前账号。

**请求**:
```http
POST /api/v1/apikey/enable
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "账号已启用",
    "data": {
        "uid": 1,
        "active": true
    }
}
```

### 4. 禁用账号

禁用当前账号。

**请求**:
```http
POST /api/v1/apikey/disable
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
Content-Type: application/json

{
    "reason": "违规操作"  // 可选
}
```

**响应**:
```json
{
    "success": true,
    "message": "账号已禁用",
    "data": {
        "uid": 1,
        "active": false
    }
}
```

### 5. 续期账号

为账号续期指定天数。

**请求**:
```http
POST /api/v1/apikey/renew
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
Content-Type: application/json

{
    "days": 30  // 必填，续期天数（1-3650）
}
```

**响应**:
```json
{
    "success": true,
    "message": "续期成功",
    "data": {
        "uid": 1,
        "expired_at": 1735689600,
        "days_left": 30
    }
}
```

**限制**:
- `days` 必须大于 0
- `days` 不能超过 3650（10年）

### 6. 刷新 API Key

生成新的 API Key，旧的立即失效。

**请求**:
```http
POST /api/v1/apikey/key/refresh
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "API Key 已刷新",
    "data": {
        "new_apikey": "key-xxxxxxxxxxxxxxxx-yyyyyyyy",
        "enabled": true,
        "warning": "旧的 API Key 已立即失效，请更新所有使用该 Key 的外部系统"
    }
}
```

**重要提示**: 刷新后，旧的 API Key 立即失效，请及时更新所有使用该 Key 的外部系统。

### 7. 禁用 API Key

禁用当前使用的 API Key。

**请求**:
```http
POST /api/v1/apikey/key/disable
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "API Key 已禁用",
    "data": {
        "uid": 1,
        "enabled": false,
        "warning": "此 API Key 已禁用，无法再使用此 Key 访问任何接口"
    }
}
```

**注意**: 禁用后，此 API Key 将无法再使用，但可以通过前端界面重新启用。

### 8. 启用 API Key

启用 API Key（如果不存在则生成新的）。

**请求**:
```http
POST /api/v1/apikey/key/enable
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "API Key 已启用",
    "data": {
        "uid": 1,
        "enabled": true,
        "apikey": "key-xxxxxxxxxxxxxxxx-yyyyyyyy"
    }
}
```

### 9. 获取 Emby 状态

获取 Emby 账号的同步状态和活动会话。

**请求**:
```http
GET /api/v1/apikey/emby/status
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "获取成功",
    "data": {
        "emby_id": "xxx",
        "is_synced": true,
        "is_active": true,
        "active_sessions": 2,
        "message": "账号正常"
    }
}
```

### 10. 踢出 Emby 会话

踢出所有 Emby 活动会话。

**请求**:
```http
POST /api/v1/apikey/emby/kick
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "已踢出 2 个会话",
    "data": {
        "kicked_count": 2
    }
}
```

### 11. 获取积分信息

获取当前积分余额和签到状态。

**请求**:
```http
GET /api/v1/apikey/score
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "获取成功",
    "data": {
        "balance": 1000,
        "score_name": "暮光币",
        "today_checkin": false,
        "checkin_streak": 7,
        "total_earned": 1000,
        "total_spent": 500
    }
}
```

### 12. 签到

每日签到获取积分。

**请求**:
```http
POST /api/v1/apikey/score/checkin
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**响应**:
```json
{
    "success": true,
    "message": "签到成功！获得 15 暮光币\n(基础 10 + 连签 5 + 随机 0)",
    "data": {
        "score": 15,
        "balance": 1015,
        "streak": 8,
        "score_name": "暮光币"
    }
}
```

**错误响应**（已签到）:
```json
{
    "success": false,
    "message": "今天已经签到过了",
    "data": null
}
```

### 13. 获取积分历史

获取积分变动历史记录。

**请求**:
```http
GET /api/v1/apikey/score/history?page=1&per_page=20&type=checkin
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**Query Parameters**:
- `page`: int - 页码（默认 1）
- `per_page`: int - 每页数量（默认 20，最大 100）
- `type`: str - 类型筛选（可选，如 checkin, transfer, renew 等）

**响应**:
```json
{
    "success": true,
    "message": "获取成功",
    "data": {
        "records": [
            {
                "id": 1,
                "type": "checkin",
                "amount": 15,
                "balance_after": 1015,
                "note": "连续签到 8 天",
                "related_uid": null,
                "created_at": 1234567890
            }
        ],
        "total": 100,
        "page": 1,
        "per_page": 20
    }
}
```

### 14. 获取积分排行榜

获取积分排行榜。

**请求**:
```http
GET /api/v1/apikey/score/ranking?limit=10
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy
```

**Query Parameters**:
- `limit`: int - 返回数量（默认 10，最大 100）

**响应**:
```json
{
    "success": true,
    "message": "获取成功",
    "data": {
        "ranking": [
            {
                "rank": 1,
                "uid": 1,
                "username": "user1",
                "score": 10000
            }
        ],
        "my_rank": 5,
        "my_score": 5000
    }
}
```

## 错误码

| HTTP 状态码 | 说明 |
|------------|------|
| 200 | 请求成功 |
| 400 | 请求参数错误 |
| 401 | 认证失败（API Key 无效、已禁用或格式错误） |
| 403 | 权限不足（账号被禁用或 API Key 缺少所需权限范围） |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

## 使用示例

### cURL 示例

```bash
# 获取账号信息
curl -X GET "https://your-domain.com/api/v1/apikey/info" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy"

# 续期账号
curl -X POST "https://your-domain.com/api/v1/apikey/renew" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"days": 30}'

# 禁用账号
curl -X POST "https://your-domain.com/api/v1/apikey/disable" \
  -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  -H "Content-Type: application/json" \
  -d '{"reason": "违规操作"}'
```

### Python 示例

```python
import requests

API_BASE = "https://your-domain.com/api/v1/apikey"
API_KEY = "key-xxxxxxxxxxxxxxxx-yyyyyyyy"

headers = {
    "X-API-Key": API_KEY,
    "Content-Type": "application/json"
}

# 获取账号信息
response = requests.get(f"{API_BASE}/info", headers=headers)
data = response.json()
print(f"账号: {data['data']['username']}, 剩余天数: {data['data']['days_left']}")

# 续期账号
response = requests.post(
    f"{API_BASE}/renew",
    headers=headers,
    json={"days": 30}
)
print(response.json()["message"])

# 禁用账号
response = requests.post(
    f"{API_BASE}/disable",
    headers=headers,
    json={"reason": "违规操作"}
)
print(response.json()["message"])
```

### JavaScript 示例

```javascript
const API_BASE = 'https://your-domain.com/api/v1/apikey';
const API_KEY = 'key-xxxxxxxxxxxxxxxx-yyyyyyyy';

const headers = {
  'X-API-Key': API_KEY,
  'Content-Type': 'application/json'
};

// 获取账号信息
async function getAccountInfo() {
  const response = await fetch(`${API_BASE}/info`, { headers });
  const data = await response.json();
  console.log(`账号: ${data.data.username}, 剩余天数: ${data.data.days_left}`);
}

// 续期账号
async function renewAccount(days) {
  const response = await fetch(`${API_BASE}/renew`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ days })
  });
  const data = await response.json();
  console.log(data.message);
}

// 禁用账号
async function disableAccount(reason) {
  const response = await fetch(`${API_BASE}/disable`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ reason })
  });
  const data = await response.json();
  console.log(data.message);
}
```

## 安全建议

1. **保护 API Key**: 
   - 不要将 API Key 提交到版本控制系统
   - 不要在公开场合分享 API Key
   - 定期更换 API Key

2. **使用 HTTPS**: 
   - 生产环境必须使用 HTTPS 传输 API Key

3. **限制访问**: 
   - 仅在需要的外部系统中使用 API Key
   - 如果 API Key 泄露，立即刷新生成新的 Key

4. **监控使用**: 
   - 定期检查 API Key 的使用情况
   - 发现异常访问及时禁用 Key

## 常见问题

### Q: API Key 在哪里获取？

A: 登录前端界面，进入"个人设置" -> "API Key 管理"，可以生成、查看和管理 API Key。

### Q: API Key 可以用于哪些接口？

A: 本套接口（`/api/v1/apikey/*`）专门为 API Key 设计。前端使用的接口（如 `/api/v1/users/*`）需要使用 Token 认证。

### Q: 刷新 API Key 后，旧的还能用吗？

A: 不能。刷新后旧的 API Key 立即失效，请及时更新所有使用该 Key 的外部系统。

### Q: 如何判断账号是否过期？

A: 使用 `/api/v1/apikey/status` 接口，检查返回的 `is_expired` 字段。

### Q: 续期天数有限制吗？

A: 是的，单次续期天数限制在 1-3650 天（10年）之间。

### Q: 如何查看积分历史？

A: 使用 `/api/v1/apikey/score/history` 接口，支持分页和类型筛选。

### Q: 签到有次数限制吗？

A: 每天只能签到一次，连续签到可获得额外奖励。
