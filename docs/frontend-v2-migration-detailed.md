# 前端 V2 迁移详细分析

生成时间：2026-09-13  
分析对象：`webui/src/lib/api.ts` 前端 API 调用层

## 一、前端 useV2 开关使用现状

### 实际使用的 useV2 开关（共 6 个）

根据代码分析，**只有以下 6 个模块**在前端代码中实际检查了 `useV2` 开关：

| 模块 | 检查次数 | 典型方法 | 状态 |
|------|----------|---------|------|
| `tickets` | 18 | getTickets, createTicket, replyTicket | ✅ 已实现条件分支 |
| `auth` | 18 | login, logout, refresh, register | ✅ 已实现条件分支 |
| `emby` | 17 | getEmbyStats, getEmbyDevices | ✅ 已实现条件分支 |
| `users` | 8 | getUsers, updateUser | ✅ 已实现条件分支 |
| `announcements` | 7 | getActiveAnnouncements, ackAnnouncement | ✅ 已实现条件分支 |
| `telegram` | 4 | getTelegramStatus, getBindCode | ✅ 已实现条件分支 |

**总计**：72 处 `if (this.useV2.*)` 条件判断

### 声明但从未使用的 useV2 开关（共 6 个）

以下模块在 `useV2` 对象中有声明，但**整个 api.ts 文件中没有任何一处检查这些开关**：

| 模块 | 后端 V2 状态 | 影响 |
|------|-------------|------|
| `invite` | ⚠️ 部分实现 | 邀请功能所有调用仍走 V1 |
| `mediaRequests` | ❓ 未确认 | 求片系统所有调用仍走 V1 |
| `audit` | ❓ 未确认 | 审计日志所有调用仍走 V1 |
| `config` | ❓ 未确认 | 配置管理所有调用仍走 V1 |
| `bangumi` | ❌ 无 V2 实现 | Bangumi 功能所有调用仍走 V1 |
| `email` | ❓ 未确认 | 邮箱功能所有调用仍走 V1 |

### 完全没有 useV2 开关的模块

以下功能模块**既没有声明 useV2 开关，也没有条件分支**，全部硬编码调用 V1 API：

| 模块 | 典型端点示例 | 影响 |
|------|-------------|------|
| APIKey 认证 | `/auth/apikey`, `/apikey/info` | API Key 用户完全依赖 V1 |
| 签到系统 | `/signin`, `/signin/me` | 签到功能完全依赖 V1 |
| 批量操作 | `/batch/users/delete`, `/batch/users/disable` | 批量管理完全依赖 V1 |
| 工单类型 | `/admin/ticket-types` | 工单分类管理完全依赖 V1 |

## 二、前端调用路径逐模块分析

### 1. Bangumi 模块（硬编码 V1）❌

**问题**：`useV2.bangumi` 有声明但从未被检查

**影响方法**（共 9 个）：
```typescript
// 所有方法直接调用 V1，无条件判断
getBangumiSyncStatus()       → GET /bangumi/sync/status
getBangumiMe()               → GET /bangumi/me
getBangumiCollections()      → GET /bangumi/collections
updateBangumiCollection()    → PATCH /bangumi/collections/:id
triggerBangumiSync()         → POST /bangumi/sync/trigger
getBangumiSyncHistory()      → GET /bangumi/sync/history
clearBangumiSyncHistory()    → DELETE /bangumi/sync/history
adminBangumiUsers()          → GET /admin/bangumi/users
adminBangumiRecords()        → GET /admin/bangumi/records/:uid
adminBangumiSyncUser()       → POST /admin/bangumi/sync/:uid
adminBangumiSyncLogs()       → GET /admin/bangumi/logs/:uid
adminBangumiClearLogs()      → DELETE /admin/bangumi/logs/:uid
```

**代码位置**：`api.ts:593-685`

**修复方案**：
1. 后端实现 V2 Bangumi 端点（9 个）
2. 前端为每个方法添加 `if (this.useV2.bangumi)` 条件分支

### 2. Invite 模块（硬编码 V1）⚠️

**问题**：`useV2.invite` 有声明但从未被检查

**影响方法**（共 13 个）：
```typescript
// 所有方法直接调用 V1，无条件判断
getInviteConfig()                    → GET /invite/config
getInviteMe()                        → GET /invite/me
createInviteCode()                   → POST /invite/codes
getInviteCodes()                     → GET /invite/codes
deleteInviteCode()                   → DELETE /invite/codes/:code
createRenewInviteCode()              → POST /invite/renew-codes
detachExpiredInviter()               → POST /invite/me/detach-expired
detachExpiredInvitee()               → POST /invite/children/:uid/detach-expired
adminGetInviteTree()                 → GET /admin/invite/tree
adminDetachInvite()                  → POST /admin/invite/users/:uid/detach
adminDetachInviteDeleteEmby()        → POST /admin/invite/users/:uid/detach-delete-emby
adminBatchDetachInvite()             → POST /admin/invite/users/detach-batch
adminQuickMaintenance()              → POST /admin/invite/quick-maintenance
adminGetInviteCodes()                → GET /admin/invite/codes
```

**代码位置**：`api.ts:1100-1250`（估计）

**后端状态**：部分 V2 端点已实现（config、check、use），但核心的邀请码 CRUD 和关系树管理缺失

### 3. 签到系统（完全无开关）❌

**问题**：既无 `useV2.signin` 声明，也无条件分支

**影响方法**（共 5 个）：
```typescript
getSigninConfig()       → GET /signin/config
getSigninMe()           → GET /signin/me
signin()                → POST /signin
signinRenew()           → POST /signin/renew
getSigninHistory()      → GET /signin/history
```

**后端状态**：
- V2 已实现：`/api/v2/signin/config`, `/api/v2/signin/history`
- V2 缺失：`/api/v2/signin/me`, `/api/v2/signin`, `/api/v2/signin/renew`

### 4. APIKey 认证（完全无开关）❌

**问题**：既无 `useV2.apikey` 声明，也无条件分支

**影响方法**（共 17 个）：
```typescript
// 用户端 APIKey 管理
listAPIKeys()                → GET /auth/apikey
createAPIKey()               → POST /auth/apikey
deleteAPIKey()               → DELETE /auth/apikey
enableAPIKey()               → POST /auth/apikey/enable
getAPIKeyPermissions()       → GET /auth/apikey/permissions
setAPIKeyPermissions()       → POST /auth/apikey/permissions

// APIKey 会话端点
getAPIKeyInfo()              → GET /apikey/info
getAPIKeyStatus()            → GET /apikey/status
apikeyEnableAccount()        → POST /apikey/enable
apikeyDisableAccount()       → POST /apikey/disable
apikeyRenewAccount()         → POST /apikey/renew
apikeyRefreshKey()           → POST /apikey/key/refresh
apikeyGetPermissions()       → GET /apikey/permissions
apikeyDisableKey()           → POST /apikey/key/disable
apikeyEnableKey()            → POST /apikey/key/enable
apikeyGetEmbyStatus()        → GET /apikey/emby/status
apikeyKickEmby()             → POST /apikey/emby/kick
apikeyUseCode()              → POST /apikey/use-code
```

**后端状态**：完全无 V2 实现（17/17 缺失）

### 5. 批量操作（完全无开关）⚠️

**问题**：既无 `useV2.batch` 声明，也无条件分支

**影响方法**（共 10 个）：
```typescript
batchDeleteUsers()           → DELETE /batch/users/delete
batchDisableUsers()          → POST /batch/users/disable
batchEnableUsers()           → POST /batch/users/enable
batchRenewUsers()            → POST /batch/users/renew
batchEnableEmby()            → POST /batch/users/emby/enable
batchDisableEmby()           → POST /batch/users/emby/disable
batchRefreshStatus()         → POST /batch/users/refresh-status
getExpiringUsers()           → GET /batch/expiring-users       ✅ V2: /api/v2/admin/users/expiring
exportUsers()                → GET /batch/export/users         ✅ V2: /api/v2/admin/export/users
batchSendReminders()         → POST /batch/send-reminders      ✅ V2: /api/v2/admin/users/send-reminders
```

**后端状态**：部分迁移（3/10 已有 V2 等价端点）

### 6. 工单类型管理（完全无开关）❌

**问题**：既无开关声明，也无条件分支

**影响方法**（共 4 个）：
```typescript
getTicketTypes()             → GET /admin/ticket-types
addTicketType()              → POST /admin/ticket-types
deleteTicketType()           → DELETE /admin/ticket-types
renameTicketType()           → PUT /admin/ticket-types
```

**后端状态**：完全无 V2 实现（4/4 缺失）

### 7. 其他未迁移模块

#### MediaRequests（有声明但未使用）
```typescript
// useV2.mediaRequests 存在但所有方法硬编码 V1
getMediaRequests()
createMediaRequest()
updateMediaRequestStatus()
// ...
```

#### Audit（有声明但未使用）
```typescript
// useV2.audit 存在但所有方法硬编码 V1
getAuditLogs()
getAuditStats()
// ...
```

#### Config（有声明但未使用）
```typescript
// useV2.config 存在但所有方法硬编码 V1
getConfig()
updateConfig()
// ...
```

#### Email（有声明但未使用）
```typescript
// useV2.email 存在但所有方法硬编码 V1
sendTestEmail()
verifyEmail()
// ...
```

## 三、前端迁移路径建议

### 阶段 1：修复已声明但未使用的开关（1 周）

**目标**：让前端代码与 useV2 声明保持一致

1. **Invite 模块**（优先级：高）
   - 等待后端补齐 V2 邀请码 CRUD 端点
   - 为所有 invite 方法添加 `if (this.useV2.invite)` 条件分支
   - 测试 V1/V2 双路径

2. **Bangumi 模块**（优先级：高）
   - 等待后端实现 V2 Bangumi 端点
   - 为所有 bangumi 方法添加条件分支
   - 测试 V1/V2 双路径

3. **其他有声明模块**（优先级：中）
   - MediaRequests、Audit、Config、Email
   - 确认后端 V2 状态后逐一接线

### 阶段 2：添加缺失模块的开关（1-2 周）

**目标**：为所有功能模块建立 V2 切换机制

1. **添加新 useV2 开关**：
   ```typescript
   private useV2 = {
     // 现有的 12 个...
     apikey: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
     signin: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
     batch: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
     ticketTypes: process.env.NEXT_PUBLIC_USE_V1_COMPAT !== 'true',
   };
   ```

2. **为每个模块实现条件分支**：
   - APIKey 认证（17 个方法）
   - 签到系统（5 个方法）
   - 批量操作（10 个方法）
   - 工单类型（4 个方法）

### 阶段 3：清理与验证（1 周）

1. **删除所有 V1 分支代码**
2. **移除 `useV2` 对象**
3. **删除 `NEXT_PUBLIC_USE_V1_COMPAT` 环境变量支持**
4. **全功能回归测试**

## 四、快速参考表

### 前端 V2 迁移状态汇总

| 模块 | useV2 声明 | 实际检查 | 后端 V2 状态 | 迁移优先级 |
|------|-----------|---------|-------------|-----------|
| auth | ✅ | ✅ (18次) | ✅ 完整 | 🟢 已完成 |
| users | ✅ | ✅ (8次) | ✅ 完整 | 🟢 已完成 |
| telegram | ✅ | ✅ (4次) | ⚠️ 部分 | 🟡 需补齐 |
| tickets | ✅ | ✅ (18次) | ✅ 完整 | 🟢 已完成 |
| announcements | ✅ | ✅ (7次) | ✅ 完整 | 🟢 已完成 |
| emby | ✅ | ✅ (17次) | ✅ 完整 | 🟢 已完成 |
| **invite** | ✅ | ❌ 0次 | ⚠️ 部分 | 🔴 高优先级 |
| **bangumi** | ✅ | ❌ 0次 | ❌ 无 | 🔴 高优先级 |
| mediaRequests | ✅ | ❌ 0次 | ❓ 未确认 | 🟡 中优先级 |
| audit | ✅ | ❌ 0次 | ❓ 未确认 | 🟡 中优先级 |
| config | ✅ | ❌ 0次 | ❓ 未确认 | 🟡 中优先级 |
| email | ✅ | ❌ 0次 | ❓ 未确认 | 🟡 中优先级 |
| **apikey** | ❌ | ❌ | ❌ 无 (17端点) | 🔴 高优先级 |
| **signin** | ❌ | ❌ | ⚠️ 部分 (3/5) | 🟡 中优先级 |
| **batch** | ❌ | ❌ | ⚠️ 部分 (3/10) | 🔴 高优先级 |
| **ticketTypes** | ❌ | ❌ | ❌ 无 (4端点) | 🔴 高优先级 |

### 迁移工作量估算

| 任务 | 方法数 | 预计工时 | 前置条件 |
|------|--------|---------|---------|
| Invite 接线 | 13 | 2 天 | 后端补齐邀请码 CRUD V2 端点 |
| Bangumi 接线 | 12 | 2 天 | 后端实现 Bangumi V2 端点 |
| APIKey 接线 | 17 | 3 天 | 后端实现 APIKey V2 端点 |
| 批量操作接线 | 10 | 2 天 | 后端补齐批量 V2 端点 |
| 签到系统接线 | 5 | 1 天 | 后端补齐签到 V2 端点 |
| 工单类型接线 | 4 | 1 天 | 后端实现工单类型 V2 端点 |
| 其他模块确认 | 20+ | 3 天 | 逐模块确认后端状态 |
| 集成测试 | - | 5 天 | 所有模块迁移完成 |
| **总计** | **81+** | **19 天** | - |

## 五、立即行动建议

### 快速验证脚本

```bash
# 检查哪些 useV2 开关被实际使用
grep -E 'if \(this\.useV2\.\w+' webui/src/lib/api.ts | \
  sed -E 's/.*if \(this\.useV2\.(\w+).*/\1/' | \
  sort | uniq -c | sort -rn

# 输出：
#      18 tickets
#      18 auth
#      17 emby
#       8 users
#       7 announcements
#       4 telegram
# 缺失：invite, bangumi, mediaRequests, audit, config, email
```

### 前端开发者检查清单

- [ ] 确认 `useV2.invite` 为何有声明但无使用
- [ ] 确认 `useV2.bangumi` 为何有声明但无使用
- [ ] 检查 mediaRequests/audit/config/email 是否有 V2 等价端点
- [ ] 为 APIKey、Signin、Batch、TicketTypes 添加 useV2 开关
- [ ] 统一所有模块的条件分支模式
- [ ] 添加单元测试覆盖 V1/V2 双路径

---

**附录**：完整方法清单见主报告 `v2-api-migration-gap-analysis.md`
