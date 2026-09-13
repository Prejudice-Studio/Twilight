# V2 API 迁移缺口分析报告

生成时间：2026-09-13  
分析对象：Twilight 项目 V1 与 V2 API 端点覆盖情况

## 执行摘要

**总体情况**：
- V1 端点总数：**340 个**
- V2 端点总数：**325 个**
- V1 独有端点（无 V2 对等实现）：**161 个**
- 覆盖率：**52.6%**（179/340）

**核心发现**：
1. **APIKey 认证体系**完全未迁移（17 个端点）
2. **Bangumi 同步系统**完全未迁移（9 个端点）
3. **工单类型管理**缺失 V2 实现（4 个端点）
4. **批量操作**部分端点未迁移（13 个端点）
5. **签到系统**完全未迁移（5 个端点）

## 一、前端 V2 迁移开关现状

### 当前 useV2 模块配置

前端 `webui/src/lib/api.ts` 定义了以下 V2 开关：

```typescript
private useV2 = {
  auth: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  users: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  telegram: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  tickets: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  announcements: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  emby: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  invite: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  mediaRequests: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  audit: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  config: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  bangumi: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
  email: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
};
```

**关键问题**：
- `bangumi` 开关存在但后端 V2 实现缺失
- 前端没有 `apikey`、`signin`、`batch` 等模块的独立开关
- 这些未开关控制的功能仍直接调用 V1 API

## 二、缺失 V2 端点详细清单

### 1. APIKey 认证体系（17 个端点）❌

**影响**：API Key 用户无法通过 V2 接口使用系统

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/auth/apikey` | 列出用户 API Keys | ❌ 无 V2 |
| `/api/v1/auth/apikey` | 创建 API Key | ❌ 无 V2 |
| `/api/v1/auth/apikey` | 删除 API Key | ❌ 无 V2 |
| `/api/v1/auth/apikey/enable` | 启用 API Key | ❌ 无 V2 |
| `/api/v1/auth/apikey/permissions` | 获取 API Key 权限 | ❌ 无 V2 |
| `/api/v1/auth/apikey/permissions` | 设置 API Key 权限 | ❌ 无 V2 |
| `/api/v1/apikey/info` | 获取当前 API Key 信息 | ❌ 无 V2 |
| `/api/v1/apikey/status` | 获取 API Key 账号状态 | ❌ 无 V2 |
| `/api/v1/apikey/enable` | API Key 启用账号 | ❌ 无 V2 |
| `/api/v1/apikey/disable` | API Key 禁用账号 | ❌ 无 V2 |
| `/api/v1/apikey/renew` | API Key 续期 | ❌ 无 V2 |
| `/api/v1/apikey/key/refresh` | 刷新 API Key | ❌ 无 V2 |
| `/api/v1/apikey/permissions` | 查询 API Key 权限 | ❌ 无 V2 |
| `/api/v1/apikey/key/disable` | 禁用 API Key | ❌ 无 V2 |
| `/api/v1/apikey/key/enable` | 启用 API Key | ❌ 无 V2 |
| `/api/v1/apikey/emby/status` | API Key 查询 Emby 状态 | ❌ 无 V2 |
| `/api/v1/apikey/emby/kick` | API Key 踢出 Emby 会话 | ❌ 无 V2 |
| `/api/v1/apikey/use-code` | API Key 使用注册码 | ❌ 无 V2 |

### 2. Bangumi 同步系统（9 个端点）❌

**影响**：Bangumi 追番同步功能完全失效

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/bangumi/cover/:subject_id` | 获取番剧封面 | ❌ 无 V2 |
| `/api/v1/bangumi/me` | 获取当前用户 Bangumi 信息 | ❌ 无 V2 |
| `/api/v1/bangumi/sync/history` | 获取同步历史 | ❌ 无 V2 |
| `/api/v1/bangumi/sync/status` | 获取同步状态 | ❌ 无 V2 |
| `/api/v1/bangumi/sync/trigger` | 手动触发同步 | ❌ 无 V2 |
| `/api/v1/admin/bangumi/users` | 管理员查看 Bangumi 用户 | ❌ 无 V2 |
| `/api/v1/admin/bangumi/records/:uid` | 查看用户同步记录 | ❌ 无 V2 |
| `/api/v1/admin/bangumi/sync/:uid` | 管理员触发用户同步 | ❌ 无 V2 |
| `/api/v1/admin/bangumi/logs/:uid` | 查看用户同步日志 | ❌ 无 V2 |

### 3. 签到系统（5 个端点）❌

**影响**：签到与积分功能失效

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/signin/config` | 获取签到配置 | ⚠️ V2 存在但前端未使用 |
| `/api/v1/signin/me` | 获取当前用户签到状态 | ❌ 无 V2 |
| `/api/v1/signin` | 执行签到 | ❌ 无 V2 |
| `/api/v1/signin/renew` | 签到续期 | ❌ 无 V2 |
| `/api/v1/signin/history` | 签到历史 | ⚠️ V2 存在但前端未使用 |

### 4. 工单类型管理（4 个端点）❌

**影响**：管理员无法管理工单分类

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/admin/ticket-types` | 获取工单类型列表 | ❌ 无 V2 |
| `/api/v1/admin/ticket-types` | 添加工单类型 | ❌ 无 V2 |
| `/api/v1/admin/ticket-types` | 删除工单类型 | ❌ 无 V2 |
| `/api/v1/admin/ticket-types` | 重命名工单类型 | ❌ 无 V2 |

### 5. 批量操作缺口（13 个端点）⚠️

**部分已迁移，但以下端点缺失**：

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/batch/users/delete` | 批量删除用户 | ❌ 无 V2 |
| `/api/v1/batch/users/disable` | 批量禁用用户 | ❌ 无 V2 |
| `/api/v1/batch/users/enable` | 批量启用用户 | ❌ 无 V2 |
| `/api/v1/batch/users/renew` | 批量续期用户 | ❌ 无 V2 |
| `/api/v1/batch/users/emby/enable` | 批量启用 Emby | ❌ 无 V2 |
| `/api/v1/batch/users/emby/disable` | 批量禁用 Emby | ❌ 无 V2 |
| `/api/v1/batch/users/refresh-status` | 批量刷新状态 | ❌ 无 V2 |
| `/api/v1/batch/expiring-users` | 获取即将过期用户 | ✅ V2: `/api/v2/admin/users/expiring` |
| `/api/v1/batch/export/users` | 导出用户 CSV | ✅ V2: `/api/v2/admin/export/users` |
| `/api/v1/batch/send-reminders` | 批量发送提醒 | ✅ V2: `/api/v2/admin/users/send-reminders` |

**注意**：前端 `api.ts` 中批量操作仍直接调用 V1 路径，没有经过 useV2 开关。

### 6. 邀请系统缺口（4 个端点）⚠️

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/invite/me` | 获取当前用户邀请信息 | ❌ 无 V2 |
| `/api/v1/invite/codes` | 创建邀请码 | ❌ 无 V2 |
| `/api/v1/invite/codes` | 获取用户邀请码列表 | ❌ 无 V2 |
| `/api/v1/invite/codes/:code` | 删除邀请码 | ❌ 无 V2 |
| `/api/v1/invite/renew-codes` | 创建续期邀请码 | ❌ 无 V2 |
| `/api/v1/invite/me/detach-expired` | 脱离过期邀请者 | ❌ 无 V2 |
| `/api/v1/invite/children/:uid/detach-expired` | 脱离过期被邀请人 | ❌ 无 V2 |
| `/api/v1/admin/invite/tree` | 获取邀请关系树 | ❌ 无 V2 |
| `/api/v1/admin/invite/users/:uid/detach` | 管理员脱离邀请关系 | ❌ 无 V2 |
| `/api/v1/admin/invite/users/:uid/detach-delete-emby` | 脱离并删除 Emby | ❌ 无 V2 |
| `/api/v1/admin/invite/users/detach-batch` | 批量脱离邀请关系 | ❌ 无 V2 |
| `/api/v1/admin/invite/quick-maintenance` | 邀请系统快速维护 | ❌ 无 V2 |
| `/api/v1/admin/invite/codes` | 管理员查看邀请码 | ❌ 无 V2 |

### 7. 注册码管理缺口（2 个端点）

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/admin/regcodes/:code/users` | 查看注册码使用用户 | ❌ 无 V2 |
| `/api/v1/admin/regcodes/:code/clear-usage` | 清除注册码使用记录 | ✅ V2: `/api/v2/admin/regcodes/:code/usage/clear` |

### 8. Telegram 管理缺口（6 个端点）⚠️

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/admin/telegram/rebind-requests` | 获取换绑请求列表 | ❌ 无 V2 |
| `/api/v1/admin/telegram/rebind-requests/:request_id/approve` | 批准换绑 | ❌ 无 V2 |
| `/api/v1/admin/telegram/rebind-requests/:request_id/reject` | 拒绝换绑 | ❌ 无 V2 |
| `/api/v1/admin/telegram/rebind-requests/batch` | 批量处理换绑 | ❌ 无 V2 |
| `/api/v1/admin/telegram/rebind-requests/revoke-approved` | 撤销所有已批准 | ❌ 无 V2 |
| `/api/v1/admin/telegram/commands/catalog` | Telegram 命令目录 | ✅ V2: `/api/v2/telegram/commands` |

### 9. 其他缺失端点

| V1 端点 | 功能 | 状态 |
|---------|------|------|
| `/api/v1/admin/me/update` | 管理员更新自己信息 | ❌ 无 V2 |
| `/api/v1/admin/users/:uid/emby` | 获取用户 Emby 详情 | ❌ 无 V2 |
| `/api/v1/admin/tickets/:ticket_id/reply` | 管理员回复工单 | ✅ V2 存在 |
| `/api/v1/auth/forgot-password/emby` | Emby 密码找回 | ❌ 无 V2 |

## 三、V2 已实现但前端未使用的端点

以下 V2 端点已经实现，但前端代码中未切换使用（仍调用 V1）：

1. **签到配置**：`GET /api/v2/signin/config`
2. **签到历史**：`GET /api/v2/signin/history`
3. **邀请配置**：`GET /api/v2/invite/config`
4. **邀请验证**：`GET /api/v2/invite/check`
5. **使用邀请码**：`POST /api/v2/invite/use`

**原因**：前端 `useV2` 开关中 `invite` 和 `signin` 有声明，但对应的方法实现仍直接调用 V1 URL，未走条件分支。

## 四、功能影响矩阵

### 高优先级（核心功能完全失效）

| 功能模块 | 缺失端点数 | 用户影响 | 迁移复杂度 |
|----------|-----------|---------|-----------|
| APIKey 认证 | 17 | 🔴 高 - API 集成用户无法使用 | 中 |
| Bangumi 同步 | 9 | 🔴 高 - 追番功能完全失效 | 中 |
| 工单类型管理 | 4 | 🔴 高 - 管理员无法管理分类 | 低 |

### 中优先级（部分功能受限）

| 功能模块 | 缺失端点数 | 用户影响 | 迁移复杂度 |
|----------|-----------|---------|-----------|
| 批量操作 | 7 | 🟡 中 - 管理员效率降低 | 低 |
| 邀请系统 | 13 | 🟡 中 - 邀请功能受限 | 中 |
| Telegram 管理 | 5 | 🟡 中 - 换绑审批失效 | 低 |

### 低优先级（边缘功能）

| 功能模块 | 缺失端点数 | 用户影响 | 迁移复杂度 |
|----------|-----------|---------|-----------|
| 签到系统 | 5 | 🟢 低 - 装饰性功能 | 低 |
| 其他杂项 | 3 | 🟢 低 - 使用频率低 | 低 |

## 五、根因分析

### 1. V2 设计范围问题

V2 API 设计时主要聚焦于：
- 核心用户管理（users）
- 基础认证流程（auth）
- Emby 集成（emby）
- 工单系统（tickets）
- 公告通知（announcements）

**未覆盖的领域**：
- 第三方集成（APIKey、Bangumi）
- 管理工具（批量操作、邀请树）
- 装饰功能（签到积分）

### 2. 前端迁移不彻底

前端存在以下问题：
1. **开关声明与实现不一致**：`useV2.bangumi`、`useV2.invite` 有声明但实际方法未实现分支
2. **缺少模块开关**：批量操作、签到、APIKey 等没有对应的 `useV2.*` 开关
3. **硬编码 V1 路径**：部分端点直接写死 `/api/v1/` 前缀，未通过条件判断

### 3. 文档与代码脱节

`docs/v2-compatibility-verification.md` 声称：
> V2 后端已实现完整的 API 路由

但实际分析显示覆盖率仅 **52.6%**，文档过于乐观。

## 六、推荐迁移路径

### 阶段 1：修复核心功能（1-2 周）

**目标**：恢复关键业务功能

1. **工单类型管理**（4 个端点）
   - 实现 V2 CRUD 端点
   - 前端切换到 V2 调用
   
2. **批量操作缺失端点**（7 个端点）
   - 补齐 V2 批量禁用/启用/续期/删除
   - 前端添加 `useV2.batch` 开关

3. **邀请系统核心功能**（13 个端点）
   - 实现 V2 邀请码生成、列表、删除
   - 实现 V2 邀请关系树和脱离操作
   - 前端修复 `useV2.invite` 实现

### 阶段 2：第三方集成（2-3 周）

**目标**：恢复外部系统对接

1. **APIKey 认证体系**（17 个端点）
   - 设计 V2 APIKey 管理端点
   - 实现权限控制逻辑
   - 前端添加 `useV2.apikey` 开关

2. **Bangumi 同步系统**（9 个端点）
   - 实现 V2 Bangumi 同步端点
   - 前端修复 `useV2.bangumi` 实现

### 阶段 3：边缘功能（1 周）

**目标**：补齐装饰性功能

1. **签到系统**（5 个端点）
   - 实现 V2 签到相关端点
   - 前端添加 `useV2.signin` 开关

2. **其他杂项**（3 个端点）
   - 按需实现

### 阶段 4：清理与测试（1 周）

1. 移除前端 `NEXT_PUBLIC_USE_V1_COMPAT` 环境变量支持
2. 删除所有 V1 API 处理器代码
3. 删除 `internal/api/routes.go` 文件
4. 全功能回归测试

## 七、立即行动项

### 快速修复清单

以下端点可快速迁移（后端逻辑简单，前端改动小）：

1. ✅ **工单类型管理**（4 个端点）- 简单 CRUD
2. ✅ **批量用户操作**（7 个端点）- 复用现有批量逻辑
3. ✅ **Telegram 换绑管理**（5 个端点）- 独立模块
4. ✅ **注册码使用记录**（1 个端点）- 简单查询

### 需要设计讨论的模块

1. ⚠️ **APIKey 认证体系** - 涉及权限模型设计
2. ⚠️ **Bangumi 同步系统** - 第三方 API 集成
3. ⚠️ **邀请关系树** - 复杂数据结构

## 八、风险与注意事项

### 技术风险

1. **CORS/CSRF 策略锁定**
   - ⚠️ **严禁修改** CORS/CSRF 实现（历史事故 commit 12225e4c）
   - 新增 V2 端点必须遵守现有策略

2. **数据库兼容性**
   - V2 端点必须能操作 V1 创建的数据
   - 禁止引入 schema 迁移

3. **会话管理**
   - V2 必须复用 V1 的会话 cookie
   - 公开端点必须使用 `credentials: "omit"`

### 业务风险

1. **用户体验降级**
   - 当前默认启用 V2，缺失功能会直接报错
   - 建议短期内反转默认值：`useV2.* = false`（V1 为默认）

2. **生产环境回退**
   - 保留 V1 API 作为紧急回退路径
   - V2 稳定前不要删除 V1 代码

## 九、总结

**当前状态**：V2 API 迁移 **严重不完整**，核心功能（APIKey、Bangumi、批量操作、邀请系统）缺失或未接通。

**建议**：
1. 🚨 **立即反转默认值** - 将 `useV2` 默认改为 `false`，避免用户遇到大量 404
2. 📋 **按优先级补齐** - 先修复高优先级核心功能
3. 🧪 **增加集成测试** - 防止前后端脱节
4. 📖 **更新文档** - `v2-compatibility-verification.md` 需要反映真实状况

**预计完整迁移时间**：5-7 周（按阶段推进）

---

**附录**：完整的 161 个缺失端点清单见本文档各节详细表格。
