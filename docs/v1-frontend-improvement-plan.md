# V1 前端渐进式改进计划

> **历史文档（已归档）**：本文描述的是 2026 年上半年的**渐进式**迁移方案，其中的逐模块开关
> （`NEXT_PUBLIC_USE_V2_AUTH` 等）与 `useV2` 标志**均已不存在**。迁移最终一次性完成：
> `webui/` 默认调用 `/api/v2/*`，唯一回退开关是 `NEXT_PUBLIC_USE_V1_COMPAT=true`。
> 当前状态请看 [`docs/plan-v2-completion-and-optimization.md`](./plan-v2-completion-and-optimization.md)。
> 保留本文仅供追溯当时的决策依据。

## 目标概述

基于**选项 A：渐进式改进**，优先级如下：
1. **API 迁移到 V2**（最优先）
2. **性能优化** 和 **移动端 UI**（同级）

## 第一阶段：API 迁移到 V2（当前重点）

### 迁移策略

采用**渐进式迁移**，保持向后兼容：
- V1 API 和 V2 API 并存
- 按模块逐步迁移，优先迁移高频使用的关键模块
- 每个模块迁移后独立测试验证

### V1 前端结构分析

**文件统计**：
- 总文件数：134 个 TS/TSX 文件
- 主目录：webui/src/app（页面）、webui/src/components（组件）、webui/src/lib（工具库）

**API 调用层**：
- `webui/src/lib/api-request.ts` - 底层 HTTP 请求封装
- `webui/src/lib/api.ts` - 业务 API 调用封装（ApiClient 类）
- `webui/src/lib/api-types.ts` - TypeScript 类型定义

### V2 后端 API 覆盖情况

V2 后端已实现的模块（39 个文件）：
- 用户管理（admin_user_v2.go）
- 工单系统（tickets_v2.go）
- Telegram（admin_telegram_v2.go, telegram_handlers_v2.go）
- Emby 管理（admin_emby_v2.go, emby_v2.go）
- 公告系统（announcements_v2.go, admin_announcements_v2.go）
- 邮箱验证（admin_email_v2.go, email_v2.go）
- 邀请系统（invite_v2.go）
- 求片系统（media_requests_v2.go）
- 审计日志（admin_audit_v2.go）
- 配置管理（config_v2.go）
- 认证（auth_v2.go）
- Bangumi（bangumi_v2.go）

### 迁移优先级（第一批）

按照使用频率和重要性排序：

#### 1. 认证模块（最高优先级）
**影响范围**：所有用户
**文件**：
- `webui/src/lib/api.ts` - login, logout, refreshToken
- `webui/src/app/(auth)/login/page.tsx`
- `webui/src/app/(auth)/register/page.tsx`

**V2 端点**：
- POST `/api/v2/auth/login`
- POST `/api/v2/auth/logout`
- POST `/api/v2/auth/refresh`
- GET `/api/v2/auth/me`

**迁移复杂度**：低
**预计时间**：1-2 天

#### 2. 用户管理模块（高优先级）
**影响范围**：管理员高频使用
**文件**：
- `webui/src/lib/api.ts` - getUsers, getUser, updateUser, disableUser 等
- `webui/src/app/(main)/admin/users/page.tsx`

**V2 端点**：
- GET `/api/v2/admin/users` - 用户列表（支持游标分页）
- GET `/api/v2/admin/users/:uid` - 用户详情
- PATCH `/api/v2/admin/users/:uid` - 更新用户
- POST `/api/v2/admin/users/:uid/disable` - 禁用/启用用户

**迁移复杂度**：中
**预计时间**：2-3 天

#### 3. Telegram 模块（高优先级）
**影响范围**：Telegram Bot 核心功能
**文件**：
- `webui/src/lib/api.ts` - getTelegramStatus, sendTelegramNotify 等
- `webui/src/app/(main)/admin/telegram/page.tsx`

**V2 端点**：
- GET `/api/v2/admin/telegram/commands/catalog`
- POST `/api/v2/admin/telegram/notify`
- GET `/api/v2/admin/telegram/rebind-requests`

**迁移复杂度**：中
**预计时间**：2-3 天

#### 4. 工单系统（中优先级）
**影响范围**：用户支持
**文件**：
- `webui/src/lib/api.ts` - getTickets, createTicket, replyTicket 等
- `webui/src/app/(main)/tickets/page.tsx`
- `webui/src/app/(main)/admin/tickets/page.tsx`

**V2 端点**：
- GET `/api/v2/tickets`
- POST `/api/v2/tickets`
- POST `/api/v2/tickets/:id/reply`

**迁移复杂度**：低
**预计时间**：1-2 天

#### 5. 公告系统（低优先级）
**影响范围**：用户查看公告
**文件**：
- `webui/src/lib/api.ts` - getAnnouncements, createAnnouncement 等
- `webui/src/app/(main)/announcements/page.tsx`

**V2 端点**：
- GET `/api/v2/announcements`
- POST `/api/v2/admin/announcements`

**迁移复杂度**：低
**预计时间**：1 天

### 迁移实施步骤（通用流程）

针对每个模块，执行以下步骤：

#### 步骤 1：创建 V2 API 类型定义
在 `webui/src/lib/api-types-v2.ts` 中定义 V2 API 的 TypeScript 类型：

```typescript
// 示例：V2 用户列表响应
export interface V2UserListParams {
  page?: number;
  per_page?: number;
  cursor?: string;
  search?: string;
  filter_emby?: boolean;
  filter_disabled?: boolean;
}

export interface V2UserListResponse {
  users: User[];
  pagination: {
    total: number;
    page: number;
    per_page: number;
    next_cursor?: string;
  };
}
```

#### 步骤 2：在 ApiClient 中添加 V2 方法
在 `webui/src/lib/api.ts` 的 ApiClient 类中添加 V2 API 方法：

```typescript
// V2 用户列表
async getUsersV2(params: V2UserListParams): Promise<ApiResponse<V2UserListResponse>> {
  const query = new URLSearchParams();
  if (params.page) query.set("page", params.page.toString());
  if (params.per_page) query.set("per_page", params.per_page.toString());
  if (params.cursor) query.set("cursor", params.cursor);
  if (params.search) query.set("search", params.search);
  if (params.filter_emby) query.set("filter_emby", "true");
  if (params.filter_disabled) query.set("filter_disabled", "true");

  return apiRequest<V2UserListResponse>(`/api/v2/admin/users?${query}`);
}
```

#### 步骤 3：添加功能开关
在 `webui/src/lib/api.ts` 中添加功能开关，允许逐步切换：

```typescript
class ApiClient {
  private useV2 = {
    auth: false,      // 认证模块
    users: false,     // 用户管理
    telegram: false,  // Telegram
    tickets: false,   // 工单系统
    announcements: false, // 公告系统
  };

  enableV2(module: keyof typeof this.useV2) {
    this.useV2[module] = true;
  }

  disableV2(module: keyof typeof this.useV2) {
    this.useV2[module] = false;
  }
}
```

#### 步骤 4：修改现有方法支持 V2
修改现有 API 方法，根据功能开关调用 V1 或 V2：

```typescript
async getUsers(params: AdminUserListParams): Promise<ApiResponse<AdminUserListResponse>> {
  if (this.useV2.users) {
    // 调用 V2 API
    const v2Params: V2UserListParams = {
      page: params.page,
      per_page: params.per_page,
      search: params.search,
      filter_emby: params.filter_emby,
      filter_disabled: params.filter_disabled,
    };
    return this.getUsersV2(v2Params);
  } else {
    // 调用 V1 API（现有逻辑）
    const query = new URLSearchParams();
    // ... 现有代码
    return apiRequest<AdminUserListResponse>(`/api/v1/admin/users?${query}`);
  }
}
```

#### 步骤 5：单元测试
为每个 V2 API 方法添加测试：

```typescript
// webui/src/lib/__tests__/api-v2.test.ts
describe('ApiClient V2', () => {
  it('should call V2 user list endpoint', async () => {
    // Mock fetch
    global.fetch = jest.fn(() =>
      Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ success: true, data: { users: [], pagination: {} } }),
      })
    );

    const api = new ApiClient();
    api.enableV2('users');
    await api.getUsers({ page: 1, per_page: 20 });

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v2/admin/users'),
      expect.any(Object)
    );
  });
});
```

#### 步骤 6：集成测试
在实际页面中启用 V2 功能开关，手动测试：

```typescript
// webui/src/app/(main)/admin/users/page.tsx
import { api } from "@/lib/api";

// 在页面组件中启用 V2
useEffect(() => {
  api.enableV2('users');
}, []);
```

#### 步骤 7：灰度发布
通过环境变量控制 V2 功能开关：

```typescript
// webui/src/lib/api.ts
class ApiClient {
  private useV2 = {
    auth: process.env.NEXT_PUBLIC_USE_V2_AUTH === 'true',
    users: process.env.NEXT_PUBLIC_USE_V2_USERS === 'true',
    telegram: process.env.NEXT_PUBLIC_USE_V2_TELEGRAM === 'true',
    tickets: process.env.NEXT_PUBLIC_USE_V2_TICKETS === 'true',
    announcements: process.env.NEXT_PUBLIC_USE_V2_ANNOUNCEMENTS === 'true',
  };
}
```

### 迁移时间表

**第一批（1-2 周）**：
- 认证模块（1-2 天）
- 用户管理模块（2-3 天）
- Telegram 模块（2-3 天）
- 工单系统（1-2 天）
- 公告系统（1 天）

**第二批（2-3 周）**：
- Emby 管理模块
- 邀请系统
- 求片系统
- 审计日志
- 配置管理

**第三批（1-2 周）**：
- Bangumi 同步
- 邮箱验证
- 其他边缘模块

### 验收标准

每个模块迁移完成后，需满足：
1. ✅ 所有 V2 API 调用正常工作
2. ✅ 功能与 V1 行为一致
3. ✅ 单元测试覆盖率 > 80%
4. ✅ 手动测试通过
5. ✅ 性能无明显下降（响应时间 < V1 的 110%）

### 风险与缓解

**风险 1：V2 API 行为与 V1 不一致**
- 缓解：详细对比 V1 和 V2 API 响应格式，编写适配层

**风险 2：迁移过程中引入 Bug**
- 缓解：严格的单元测试 + 手动测试，灰度发布

**风险 3：性能回退**
- 缓解：性能监控，对比 V1 和 V2 响应时间

---

## 第二阶段：性能优化（同级优先级）

### 优化方向

#### 1. 代码分割和懒加载
- 使用 Next.js 动态导入（dynamic import）
- 路由级别的代码分割
- 组件级别的懒加载

#### 2. 渲染优化
- 识别不必要的重渲染（React DevTools Profiler）
- 使用 useMemo 和 useCallback 优化
- 虚拟滚动（长列表优化）

#### 3. 数据获取优化
- 实现请求去重
- 使用 SWR 或 React Query 进行数据缓存
- 预加载关键数据

#### 4. 构建优化
- Tree Shaking
- 压缩和混淆
- CDN 静态资源

---

## 第三阶段：移动端 UI 优化（同级优先级）

### 优化方向

#### 1. 响应式设计改进
- Tailwind CSS 断点优化
- 移动端专用布局
- 触摸交互优化

#### 2. 移动端专用组件
- 底部导航栏
- 抽屉式菜单
- 移动端表单优化

#### 3. 性能优化
- 图片懒加载
- 减少网络请求
- 离线支持（PWA）

---

## 执行建议

**立即开始**：
1. 从**认证模块**开始迁移（最高优先级）
2. 创建 `webui/src/lib/api-types-v2.ts`
3. 在 `webui/src/lib/api.ts` 中添加 V2 功能开关

**并行进行**：
- API 迁移（主线）
- 性能瓶颈分析（收集数据）
- 移动端 UI 设计（原型设计）

需要我立即开始实施吗？请确认是否立即开始**第一批迁移（认证模块）**。
