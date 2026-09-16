# V2 API 迁移指南

## 概述

V2 架构是 Twilight 项目的重要里程碑，提供了更清晰的分层结构、更完善的安全机制和更好的可维护性。本指南帮助开发者理解 V2 与 V1 的差异，并平滑迁移现有集成。

## 迁移状态

- **V2 路由覆盖率**: 321/340 (94.4%)
- **数据库兼容性**: ✅ 完全兼容，无需迁移脚本
- **V1 保留**: 19 个外部集成端点保留在 V1（API Key 鉴权）
- **前端状态**: webui-v2 (SvelteKit SSR) 已迁移，webui (Next.js) 作为紧急回滚保留

## V2 架构特性

### 1. 应用服务层

V2 引入了应用服务层，将业务逻辑从 HTTP 传输层分离：

```go
// V1 模式：业务逻辑耦合在 handler 中
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
    // 密码验证、会话创建、审计日志全部混在一起
}

// V2 模式：handler 委托到应用服务
func (a *App) handleV2Login(w http.ResponseWriter, r *http.Request, p Params) {
    w.Header().Set("Cache-Control", "no-store")
    a.handleLogin(w, r, p) // 委托到 V1 实现
}

// 应用服务（独立）
type loginService struct { app *App }
func (s *loginService) authenticate(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
    // 纯业务逻辑，可被 V1/V2 共享
}
```

**关键服务文件**：
- `internal/api/registration_service.go` - 注册流程
- `internal/api/login_service.go` - 登录流程
- `internal/api/user_service.go` - 用户自服务
- `internal/api/password_reset_service.go` - 密码重置
- `internal/api/setup_service.go` - 初始化向导

### 2. 缓存控制

所有 V2 端点明确声明缓存策略：

| 端点类型 | Cache-Control | 说明 |
|----------|---------------|------|
| 用户数据 | `private, no-store` | 会话范围数据，禁止缓存 |
| 公共资源 | `public, max-age=3600` | 背景图、公告等可缓存 |
| 系统状态 | `private, max-age=60` | 短期缓存，快速失效 |

### 3. 安全增强

- ✅ **CSRF 保护**: Double Submit Cookie 机制
- ✅ **速率限制**: Redis + 内存降级，按用户/角色分级
- ✅ **路径遍历防护**: 文件操作全面防护
- ✅ **输入脱敏**: 38 个文件包含敏感信息脱敏逻辑
- ✅ **审计日志**: 所有状态修改操作记录审计

详见 [V2 安全审计报告](./security-audit-report.md)。

## API 端点映射

### 认证模块

| V1 端点 | V2 端点 | 变更 |
|---------|---------|------|
| POST `/api/v1/auth/login` | POST `/api/v2/auth/login` | 响应增加 Cache-Control |
| POST `/api/v1/auth/logout` | POST `/api/v2/auth/logout` | 同上 |
| POST `/api/v1/auth/refresh` | POST `/api/v2/auth/refresh` | 同上 |
| GET `/api/v1/auth/me` | GET `/api/v2/auth/me` | 私有 no-store |

### 用户自服务

| V1 端点 | V2 端点 | 变更 |
|---------|---------|------|
| GET `/api/v1/users/me` | GET `/api/v2/users/me` | 私有 no-store |
| PATCH `/api/v1/users/me/username` | PATCH `/api/v2/users/me/username` | 同上 |
| PATCH `/api/v1/users/me/password` | PATCH `/api/v2/users/me/password` | 同上 |
| POST `/api/v1/users/me/emby/register` | POST `/api/v2/users/me/emby/register` | 队列化处理 |

### 管理功能

| V1 端点 | V2 端点 | 变更 |
|---------|---------|------|
| GET `/api/v1/admin/users` | GET `/api/v2/admin/users` | 游标分页优先 |
| POST `/api/v1/admin/users/{uid}/disable` | POST `/api/v2/admin/users/{uid}/disable` | 增强审计 |
| POST `/api/v1/admin/regcodes/generate` | POST `/api/v2/admin/regcodes/generate` | 幂等键支持 |

### 未迁移端点（保留 V1）

以下端点因使用 API Key 鉴权，保留在 V1 不迁移到 V2：

```
/api/v1/apikey/*          - API Key 集成端点（13 个）
/api/v1/auth/apikey/*     - API Key 认证端点（6 个）
```

**理由**：这些端点是外部系统集成接口，V1 已稳定，无需迁移。

## 前端集成指南

### SvelteKit SSR (webui-v2)

V2 前端采用服务端渲染 + 渐进增强模式：

```typescript
// src/routes/+page.server.ts
export const load: PageServerLoad = async ({ fetch }) => {
  // 服务端获取首屏数据
  const res = await fetch('/api/v2/users/me');
  return { user: await res.json() };
};

export const actions: Actions = {
  updateProfile: async ({ request, fetch }) => {
    // form action 处理写操作
    const data = await request.formData();
    const res = await fetch('/api/v2/users/me/username', {
      method: 'PATCH',
      body: JSON.stringify({ username: data.get('username') })
    });
    return { success: res.ok };
  }
};
```

**关键特性**：
- 服务端 `load` 函数获取首屏数据（SEO 友好）
- form action 处理写操作（渐进增强）
- 浏览器不持有 Bearer Token（安全）
- 同源 API 代理（避免 CORS）

### Next.js (webui - 遗留)

旧前端保留作为紧急回滚，不再活跃开发：

```typescript
// pages/index.tsx (V1 模式)
export const getServerSideProps = async (ctx) => {
  const res = await fetch('/api/v1/users/me');
  return { props: { user: await res.json() } };
};
```

## 客户端迁移清单

### 1. 检查端点可用性

```bash
# 测试 V2 端点
curl -X POST https://your-domain/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test123"}'
```

### 2. 更新 API 基础路径

```typescript
// 旧代码
const API_BASE = '/api/v1';

// 新代码
const API_BASE = '/api/v2';
```

### 3. 处理 Cache-Control 响应头

```typescript
// V2 端点会返回明确的缓存策略
fetch('/api/v2/users/me').then(res => {
  console.log(res.headers.get('Cache-Control')); // "private, no-store"
});
```

### 4. 适配游标分页（可选）

```typescript
// V1: 页码分页
GET /api/v1/admin/users?page=1&per_page=20

// V2: 游标分页（推荐）
GET /api/v2/admin/users?cursor=abc123&limit=20

// V2 仍支持页码分页（兼容）
GET /api/v2/admin/users?page=1&per_page=20
```

## 数据库兼容性

### 无需迁移

V2 API **完全兼容**现有 PostgreSQL schema，无需执行任何迁移脚本：

- ✅ 共享同一 `twilight_state` JSONB 状态存储
- ✅ 共享独立表（sessions, audit_logs, runtime_logs, playback_records）
- ✅ V1/V2 可同时运行，逐步切换流量
- ✅ 回滚到 V1 无数据丢失风险

### 数据流

```
V2 Handler → Application Service → V1 Store Implementation → PostgreSQL
     ↓              ↓                        ↓
  Cache-Control   业务逻辑              数据持久化
  响应适配        事务边界              JSONB + 关系表
```

## 测试验证

### 后端测试

```bash
# 运行所有测试
go test ./...

# 仅测试 V2 路由
go test ./internal/api -run TestV2

# 检查文档同步
go run ./scripts/check_docs_drift.go
```

### 前端测试

```bash
cd webui-v2
pnpm check       # 类型检查
pnpm build       # 构建验证
pnpm preview     # 预览构建产物
```

### 集成测试

```bash
# 启动后端
go run ./cmd/twilight api --config config.toml --debug

# 启动前端
cd webui-v2 && pnpm dev

# 测试关键流程
curl -X POST http://localhost:5000/api/v2/auth/login ...
curl -X GET http://localhost:5000/api/v2/users/me ...
```

## 性能对比

| 指标 | V1 | V2 | 改进 |
|------|----|----|------|
| 响应头大小 | ~200B | ~250B | +25% (Cache-Control) |
| 业务逻辑耦合 | 高 | 低 | 可测试性提升 |
| 代码复用 | 低 | 高 | 服务层共享 |
| 安全审计覆盖 | 部分 | 100% | 全面防护 |

## 回滚策略

### 前端回滚

```bash
# 切换到 V1 前端
cd webui
pnpm install --frozen-lockfile
pnpm build

# 更新反向代理配置指向 webui/out
```

### API 回滚

V1 API 保持不变，前端可随时切回：

```typescript
// 修改 API_BASE 即可
const API_BASE = '/api/v1'; // 回滚到 V1
```

### 数据回滚

无需回滚 - V1/V2 共享同一数据库，无数据迁移。

## 常见问题

### Q: V2 是否向后兼容？

**A**: 是。V2 委托到 V1 实现，数据库完全兼容。V1 端点继续可用。

### Q: 前端必须迁移到 V2 吗？

**A**: 否。可以逐步迁移，甚至继续使用 V1 端点。webui-v2 已切换到 V2，但 webui (Next.js) 仍使用 V1 作为回滚选项。

### Q: API Key 集成是否受影响？

**A**: 否。API Key 端点保留在 V1 (`/api/v1/apikey/*`)，外部集成无需修改。

### Q: 性能是否有影响？

**A**: 无显著影响。V2 仅增加 Cache-Control 响应头和应用服务层抽象，实际业务逻辑与 V1 相同。

### Q: 如何验证迁移成功？

**A**: 运行完整测试套件（`go test ./...` 和 `pnpm check`），确保所有测试通过。在暂存环境测试关键流程（登录、注册、用户管理）。

## 模板参数系统增强

### 概述

V2 引入了统一的模板参数系统，大幅扩展了可在通知模板中使用的参数数量（从原来的约10个扩展到60+个），使管理员可以创建更丰富、更个性化的通知消息。

### 新增参数分类

#### 1. Emby 状态相关（新增重点功能）

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{emby_enabled}` | Emby 是否启用 | 是 / 否 |
| `{emby_disabled}` | Emby 是否禁用 | 是 / 否 |
| `{emby_enabled_status}` | Emby 启用状态 | 已启用 / 已禁用 / - |
| `{emby_disabled_reason}` | Emby 禁用原因 | 未绑定 / 正常 / Web账号被禁用 / 账号已过期 / 已禁用 |
| `{emby_grant_locked}` | 是否已使用注册资格 | 是 / 否 |

#### 2. 账号状态增强

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{days_until_expiry}` | 到期剩余天数 | 永久 / 已过期 / 30天 / 不足1天 |
| `{is_expired}` | 是否已过期 | 是 / 否 |
| `{is_permanent}` | 是否永久账号 | 是 / 否 |
| `{account_enabled}` | 账号启用状态 | 已启用 / 已禁用 |
| `{expiry_time}` | 到期时间 | 2026-12-31 23:59:59 / 永久 |

#### 3. 角色和权限

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{role_name}` | 用户角色名称 | 管理员 / 普通用户 / 白名单用户 |
| `{is_admin}` | 是否管理员 | 是 / 否 |
| `{is_whitelist}` | 是否白名单用户 | 是 / 否 |
| `{is_protected}` | 是否受保护账号 | 是 / 否 |

#### 4. 绑定状态完整信息

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{telegram_bound}` | Telegram 是否绑定 | 是 / 否 |
| `{telegram_id}` | Telegram 用户ID | 123456789 / - |
| `{email_bound}` | 邮箱是否绑定 | 是 / 否 |
| `{email_verified_status}` | 邮箱验证状态 | 已验证 / 未验证 |
| `{rebinding_in_progress}` | 是否换绑中 | 是 / 否 |

#### 5. 通知设置

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{notify_login_telegram}` | 是否开启登录 TG 通知 | 是 / 否 |
| `{notify_login_email}` | 是否开启登录邮件通知 | 是 / 否 |
| `{notify_ticket_telegram}` | 是否开启工单 TG 通知 | 是 / 否 |

### 模板迁移示例

#### 登录通知模板

**V1 模板（仅支持基础参数）：**
```
新登录通知

账号：{username}
时间：{time}
IP：{ip}
设备：{device}
```

**V2 增强模板（可使用所有用户状态参数）：**
```
🔐 新登录通知

== 基本信息 ==
用户：{username} (UID: {uid})
角色：{role_name}
时间：{time}
IP：{ip}
设备：{device}

== 账号状态 ==
Web 状态：{account_enabled}
到期：{expire_status} ({days_until_expiry})

== 服务状态 ==
Emby：{emby_enabled_status}
{emby_disabled_reason}

如非本人操作，请立即修改密码！
```

#### 工单通知模板

**V1 模板：**
```
🎫 工单更新通知
{server_name}
━━━━━━━━━━━━━━
🆔 #{ticket_id}  {title}
📊 状态：{status}
🕒 {time}
{admin_note_content}
```

**V2 增强模板（新增用户状态信息）：**
```
🎫 工单更新通知
{server_name}
━━━━━━━━━━━━━━
🆔 #{ticket_id}  {title}
📊 状态：{status}
🕒 {time}

== 您的账号状态 ==
Emby：{emby_enabled_status}
到期：{days_until_expiry}

{admin_note_content}
```

### 向后兼容性

- ✅ **完全兼容**：所有 V1 参数在 V2 中继续可用
- ✅ **平滑升级**：现有模板无需修改即可工作
- ✅ **渐进增强**：管理员可按需逐步添加新参数
- ✅ **参数缺失处理**：未识别的参数占位符保持原样，不会导致错误

### 配置方式

模板配置位置保持不变：

**config.toml：**
```toml
[Notification]
login_notify_telegram_template = """
新登录通知
账号：{username} (角色：{role_name})
时间：{time}
IP：{ip}
Emby 状态：{emby_enabled_status}
到期：{days_until_expiry}
"""

[Ticket]
notify_telegram_template = """
🎫 #{ticket_id} {title}
状态：{status}
用户 Emby：{emby_enabled_status}
{admin_note_content}
"""
```

**WebUI 配置界面（即将推出）：**
- 可视化模板编辑器
- 实时参数预览
- 参数自动补全提示

### 测试新参数

```bash
# 触发登录通知查看新参数效果
curl -X POST http://localhost:5000/api/v2/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test123"}'

# 触发工单通知查看新参数效果
curl -X POST http://localhost:5000/api/v2/tickets/{ticket_id}/reply \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"content":"测试回复"}'
```

## 下一步

1. **阅读架构文档**: [V2 架构设计](./architecture.md)
2. **查看安全审计**: [V2 安全审计报告](./security-audit-report.md)
3. **前端 SSR 指南**: [V2 SSR 前端](./frontend-ssr.md)
4. **API 路由索引**: [API 路由索引](../reference/api-index.md)

## 贡献

发现问题或有改进建议？

- 提交 Issue: https://github.com/Prejudice-Studio/Twilight/issues
- 提交 PR: https://github.com/Prejudice-Studio/Twilight/pulls
- 加入讨论: https://t.me/TwilightPanelChat
