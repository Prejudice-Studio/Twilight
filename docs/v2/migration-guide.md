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
