# Agent Instructions

本文件适用于整个仓库。后续 LLM/Agent 修改代码前必须先阅读本文件，并优先遵守这里的项目约定；若子目录未来出现更近的 `AGENTS.md`，以更近文件为准。

## 项目概览

- Twilight 是 Emby / Jellyfin 用户管理面板，当前主线是 Go 后端 + Next.js 前端。
- 后端模块路径为 `github.com/prejudice-studio/twilight`，入口在 `cmd/twilight`，目标部署环境是 Linux + systemd。
- 前端位于 `webui/`，使用 Next.js App Router、TypeScript、Tailwind CSS、Radix/shadcn 风格组件、Zustand 与 TanStack Query。
- 重要文档优先级：`docs/guides/development.md`、`docs/guides/modular-architecture.md`、`docs/reference/backend.md`、`docs/reference/api-index.md`、`docs/reference/backend-api.md`、`docs/guides/security.md`、`README.md`。
- 若旧文件或本地笔记提到 Python 后端、SQLite 多库、uvicorn、`requirements.txt`、旧 `db/*.db` 迁移等内容，以当前 Go 源码和 `docs/guides/development.md` 为准，不要重新引入旧后端运行入口。

## 目录速览

- `cmd/twilight`：Go CLI 入口，支持 `api`、`all`、`scheduler`、`bot`、`version`。
- `internal/api`：HTTP 路由、鉴权、限流、统一响应、handler、外部服务 client、调度器和运维接口。
- `internal/api/routes.go`：全部后端路由的集中注册点。
- `internal/config`：读取 `config.toml`、`config.local.toml` 和 `TWILIGHT_*` 环境变量。
- `internal/store`：单一状态文档存储，支持 JSON 文件或 PostgreSQL。
- `internal/redis`：Redis RESP 客户端，用于会话与限流共享。
- `internal/security`：Token、密码哈希与安全随机数。
- `webui/src/app`：Next.js App Router 页面。
- `webui/src/lib/api-request.ts`：底层请求、凭据、超时与 `ApiError`。
- `webui/src/lib/api.ts`：前端 API 客户端，新增后端接口时通常要同步这里和 `api-types.ts`。
- `deploy/`：systemd unit 与安装脚本。部署 unit 必须指向 `bin/twilight`。
- `webui/src/lib/safe-render.tsx`：公告 Markdown / BBCode / plain 三种渲染模式的手写安全解析器，含 URL 白名单过滤。
- `webui/src/components/announcement-board.tsx`：可复用公告板组件（仪表盘 + 独立公告页），支持置顶拆分、折叠和长内容截断。

## 功能与代码定位（速查）

定位某功能时：先在本表找到「功能域」→ 看后端文件（handler/业务/外部 client）与 `internal/store` 方法 → 路由在 `internal/api/routes.go` 按前缀搜 → 前端在 `webui/src/app` 找页面、`webui/src/lib/api.ts` 找客户端方法。文案键见 `webui/src/locales/`（基底 `basic.json`）。路径前缀省略：后端 `internal/api/`、存储 `internal/store/`、页面 `webui/src/app/`、组件 `webui/src/components/`。

| 功能域 | 后端（handler / 业务 / client） | store | 路由前缀 | 前端页面 / 组件 | 配置段 · 专题文档 |
| ---- | ---- | ---- | ---- | ---- | ---- |
| 登录 / 会话 / 找回密码 | `auth_handlers.go`、`password_verify.go`、`session.go` | `login_log.go`、sessions() | `/auth/*` | `(auth)/login`、`(auth)/forgot-password` | `[Security]` |
| 用户自助（资料/改密/头像/背景/登录通知） | `handlers.go`、`upload_handlers.go`、`safepath.go` | `store.go`(User) | `/users/me/*` | `(main)/settings`、`settings/background`、`settings/appearance` | [背景与头像](docs/features/background.md) |
| 邮箱验证 / 找回 / 强制绑定 / 验证记录审查 / 自动清理 | `email_handlers.go`、`email_verify_service.go`、`email_client.go` | `email_verification.go` | `/users/me/email/*`、`/auth/password/email/*`、`/admin/email/*` | `components/email-*.tsx`、`admin/users/admin-email-dialog.tsx`、`(main)/admin/email` | `[Email]`/`[SAR]`名单/`[RateLimit]` · [邮箱](docs/features/email.md) |
| Emby 绑定/注册/同步/设备·IP 审查 | `emby.go`、`emby_client.go`、`emby_inventory.go`、`emby_url_probe.go`、`emby_device_audit.go`(`handleAdminEmbyDeviceAudit`/`buildEmbyDeviceAudit`/`parseRemoteIP`)、`handlers.go`(`handleSessions`) | `store.go`(User.EmbyID) | `/emby/*`、`/admin/emby/*` | `(main)/admin/emby`（含设备/IP 审查页签；`(main)/admin/device-audit` 仅兼容旧入口）、`(main)/dashboard` | `[Emby]` |
| Telegram Bot / 绑定 / 花名册 / 换绑 | `telegram_bot.go`、`telegram.go`、`telegram_commands.go`、`telegram_inline.go`、`telegram_bind_*.go`、`bind_status_hub.go` | `store.go`(roster/rebind) | `/users/me/telegram/*`、`/admin/telegram/*` | `admin/telegram-rebind-requests` | `[Telegram]` · [Bot](docs/features/telegram-bot.md) |
| 注册码 / 续期码 / 白名单码 | `regcode_handlers.go`、`code_use_handlers.go`、`business.go`(生成/消费) | `store.go`(RegCode) | `/admin/regcodes/*`、`/users/me/use-code` | `(main)/admin/regcodes` | `[SAR]` · [卡码](docs/features/regcodes.md) |
| 邀请树 | `invite_handlers.go`、`invite_admin_handlers.go`、`business.go`(`inviteForest`) | `store.go`(InviteCode/Relations) | `/invite/*`、`/admin/invite/*` | `(main)/invite`、`(main)/admin/invite` | `[SAR]` · [邀请树](docs/features/invite.md) |
| 求片 | `media_request_handlers.go`、`media_service.go`、`tmdb_client.go`、`bangumi_client.go`、`emby_inventory.go` | `store.go`(MediaRequest) | `/media/*` | `(main)/media`、`(main)/admin/requests` | `[SAR]` |
| 签到 / 积分 | `signin_handlers.go` | `signin.go` | `/signin/*` | `(main)/score` | `[SAR]` signin_* |
| 公告 | `announcement_handlers.go` | `store.go`(Announcement) | `/announcements`、`/admin/announcements/*` | `(main)/announcements`、`admin/announcements` | [公告](docs/features/announcements.md) |
| 设备 / 登录历史 / IP 黑名单 | `handlers.go`(`handleDevices`/`handleLoginHistory`)、`auth_handlers.go`(登录写设备) | `device.go`、`login_log.go`、`ip_blacklist.go` | `/security/*`、`/users/me/devices` | （并入 settings/admin） | `[DeviceLimit]` |
| Bangumi 同步 | `bangumi_webhook.go`、`bangumi_client.go`、`bangumi_sync_service.go`、`bangumi_sync_handlers.go` | `store.go`(User.BgmToken, BangumiSyncLog)、`playback.go` | `/bangumi/sync/*`、`/admin/bangumi/*`、`/emby/bangumi/webhook` | `(main)/bangumi`、`(main)/admin/bangumi` | `[BangumiSync]` · [Bangumi](docs/features/bangumi.md) |
| API Key | `apikey_handlers.go` | `store.go`(APIKey) | `/apikey/*`、`/users/me/apikeys` | `settings/apikey` | [API Key](docs/reference/api-key.md) |
| 登录通知 | `auth_handlers.go`(登录发通知)、`email_client.go`、`telegram.go` | `store.go`(User.NotifyOnLoginTelegram/Email) | — | `(main)/settings` | `[Notification]` · config 模板字段 |
| 批量用户操作 | `batch_user_handlers.go`、`batch_helpers.go`、`handlers.go`(`filteredBatchUserUIDs`) | `store.go`(Users) | `/batch/*`、`/admin/users/bulk-*` | `(main)/admin/users` | — |
| 调度任务 | `scheduler_handlers.go`、`scheduler_daemon.go`、`scheduler_runner.go`、`admin_jobs.go` | `scheduler.go` | `/admin/scheduler/*` | `(main)/admin/scheduler` | `[Scheduler]` |
| 配置管理（可视化/TOML/备份） | `config_admin.go`、`internal/config` | — | `/system/admin/config/*` | `(main)/admin/config` | `config.production.toml` |
| 实时日志 / 运行状态 | `runtime_logs.go`、`app.go`(状态) | `runtime_log.go` | `/system/admin/runtime/*` | `(main)/admin/logs` | `[Global]` log_* |
| 违规审计（诱饵码等） | `violation_handlers.go` | `store.go`(ViolationLog) | `/admin/violations/*` | `(main)/admin/violations` | `[SAR]` decoy_action |
| 数据库 / 备份 / 迁移 | `database_admin.go`、`storage_guard.go` | `internal/store`(postgres/json) | `/system/admin/database/*` | `(main)/admin/database` | `[Database]` |
| 实时日志 / 运行状态 | `runtime_logs.go`、`app.go`(状态) | `runtime_log.go` | `/system/admin/runtime/*` | `(main)/admin/logs` | `[Global]` log_* |
| 违规审计（诱饵码等） | `violation_handlers.go` | `store.go`(ViolationLog) | `/admin/violations/*` | `(main)/admin/violations` | `[SAR]` decoy_action |
| 系统自动更新（Git） | `system_update.go`、`system_update_handler.go` | — | `/system/admin/update` | `admin/config`(更新页) | `[SystemUpdate]` |
| 统计 / 服务器状态 | `handlers.go`(`handleSystemStats`/`handleWatchStats`) | `playback.go` | `/system/admin/stats`、`/batch/watch-stats/*` | `(main)/admin/stats` | — |
| 操作审计日志 | `audit_handlers.go`(含 `audit()` helper) | `store.go`(AuditLog) | `/admin/audit-logs/*` | `(main)/admin/audit-logs` | — |

### 关键函数速查（按文件）

**`internal/api/` — HTTP 层：**
| 文件 | 主要函数 |
|------|---------|
| `routes.go` | `registerAllRoutes()`、`registerAPIRoutes()`、`registerAdminRoutes()`、`registerAPIKeyRoutes()`、`registerSecurityRoutes()`、`registerBatchRoutes()` |
| `auth_handlers.go` | `handleLogin`、`handleRegister`、`handleLogout`、`handleAuthMe`、`handleForgotPasswordByEmby` |
| `handlers.go` | `handleAdminUsers`(行1624)、`handleAdminCreateUser`(行1681)、`handleAdminUpdateUser`(行1755)、`handleAdminDeleteUser`(行1888)、`handleAdminToggleUser`(行1956)、`handleAdminToggleEmby`(行2021)、`handleAdminForceUnbind`(行2096)、`handleAdminRenewUser`(行2296)、`handleAdminSetUserExpiry`(行2344)、`handleAdminResetPassword`(行2381)、`handleAdminSetRole`(行2420)、`handleAdminUnbindTelegram`(行2466)、`handleUpdateMe`(行62)、`handleUpdateUsername`、`handleChangePassword`、`handleGeneratedPassword`、`handleBindEmby`、`handleRegisterEmby`、`handleUnbindEmby`、`handleRenew`、`handleTelegramStatus`、`handleUnbindTelegram`、`handleTelegramRebindRequest` |
| `regcode_handlers.go` | `handleListRegcodes`、`handleCreateRegcodes`、`handleUpdateRegcode`、`handleDeleteRegcode`、`handleBatchDeleteRegcodes`、`handleRegcodeUsers`、`handleClearRegcodeUsage` |
| `code_use_handlers.go` | `handleUseCode`、`handleQueueStatus` |
| `invite_handlers.go` | `handleInviteConfig`、`handleInviteMe`、`handleCreateInviteCode`、`handleCreateInviteRenewCode`、`handleInviteCodes`、`handleDeleteInviteCode`、`handleDetachExpiredInviteChild`、`handleInviteCheck`、`handleInviteUse` |
| `media_request_handlers.go` | `handleMediaSearch`、`handleMediaDetail`、`handleCreateMediaRequest`、`handleMyMediaRequests`、`handleAdminMediaRequests`、`handleUpdateMediaRequestStatus`、`handleExternalMediaUpdate` |
| `signin_handlers.go` | `handleSigninConfig`、`handleSigninSummary`、`handleSignin`、`handleSigninRenew`、`handleSigninHistory` |
| `email_handlers.go` | `handleSendEmailCode`、`handleVerifyEmailCode`、`handleForgotPasswordEmailRequest`、`handleForgotPasswordEmailReset`、`handleAdminBindUserEmail`、`handleAdminSetUserEmailVerified`、`handleAdminEmailTest`、`handleAdminEmailVerifications`、`handleAdminCleanupEmailVerifications`、`handleAdminClearUnverifiedEmails` |
| `announcement_handlers.go` | `handleListAnnouncements`、`handleAdminAnnouncements`、`handleCreateAnnouncement`、`handleUpdateAnnouncement`、`handleDeleteAnnouncement` |
| `audit_handlers.go` | `audit(r, action, category, targetUID, detail)`(行12)、`auditEntryIP`(行30)、`handleListAuditLogs`、`handleDeleteAuditLog`、`handleClearAuditLogs`、`handlePruneAuditLogs`(行112) |
| `bangumi_webhook.go` | `handleBangumiWebhook`、`constantTimeStringEqual` |
| `bangumi_sync_handlers.go` | `handleBangumiSyncStatus`、`handleBangumiSyncTrigger`、`handleBangumiSyncHistory`、`handleBangumiClearHistory`、`handleAdminBangumiUsers`、`handleAdminBangumiRecords`、`handleAdminBangumiSyncUser`、`handleAdminBangumiSyncLogs`、`handleAdminBangumiClearLogs` |
| `batch_user_handlers.go` | `handleBatchToggleUsers`、`handleBatchRenewUsers`、`handleBatchDeleteUsers`、`handleBatchLockEmbyUnbind`、`handleBatchClearEmbyGrant`、`filteredBatchUserUIDs`(行418) |
| `app.go` | `ServeHTTP`(行558)、`authenticate`(行698)、`current(r)`(行800)、`clientIP`、`principal`(行147) |

**`internal/api/` — 业务/客户端层：**
| 文件 | 主要函数 |
|------|---------|
| `business.go` | `regcodeDTO`(行498+544)、`regcodeStatus`(行484)、`generateRegCode`(行628)、`generateInviteCode`、`previewCode`、`inviteForest`(行1226)、`inviteTreeFor`(行1154)、`canInvite`(行1183)、`maxCodeDays`(行1050)、`sortUsers`(行909)、`batchResult`(行1324)、`addBatchOutcome`(行1331) |
| `emby_client.go` | `embyGet`、`embyPost`、`embyDelete`、`embyConfigured` |
| `email_client.go` | `emailConfigured`(行21) |
| `email_verify_service.go` | `issueEmailCode`、`verifyEmailCodeByID` |
| `bangumi_sync_service.go` | `syncBangumiForUser`、`matchBangumiSubject`、`ensureBangumiCollection`、`markBangumiEpisode` |
| `scheduler_runner.go` | `runCheckExpired`、`runExpiryReminder`、`runDailyStats` |
| `config_admin.go` | `configSectionDefs()`(行847)、`configValues()`(行973) |

**`internal/store/store.go` — 状态模型：**
| 实体 | 类型/字段 | 行 |
|------|---------|-----|
| `State` | 单一状态文档，含所有实体 map | 行55-99 |
| `User` | UID, Username, Email, PasswordHash, Role, Active, EmbyID, EmbyDisabled, TelegramID, ExpiredAt, BgmToken… | 行101-145 |
| `RegCode` | Code, Type, Days, ValidityTime, UseCount, Active, Source, CreatorUID… | 行215-239 |
| `InviteCode` | Code, UID, InviterUID, Days, UseCount, Active… | 行198-213 |
| `InviteRelation` | ParentUID, ChildUID, Code, CreatedAt | 行241-246 |
| `AuditLog` | ID, UID, Username, Action, Category, TargetUID, Detail, IP, CreatedAt | 行274-285 |
| `ViolationLog` | ID, UID, Username, Code, CodeType, Reason, Action, IP… | 行259-272 |
| `Announcement` | ID, Title, Content, Level, RenderMode, Pinned, Visible… | 行248-257 |
| `Device` | UID, DeviceID, DeviceName, Client, LastIP, Trusted, Blocked… | 行336-347 |
| `LoginLog` | ID, UID, IP, DeviceID, DeviceName, Client, Time, Blocked… | 行349-361 |
| `SchedulerRun` | ID, JobID, Type, Trigger, Status, Message, Summary, Logs… | 行313-326 |

**`internal/store/store.go` — 关键方法：**
| 方法 | 行 | 说明 |
|------|-----|------|
| `ListUsers()` → `[]User` | — | 所有用户 |
| `User(uid)` → `(User, bool)` | — | 按 UID 查找 |
| `FindUserByUsername` / `FindUserByTelegramID` | — | 按用户名/TG ID 查找 |
| `CreateUser` / `UpdateUser` / `DeleteUser` | — | 用户 CRUD |
| `SetUserActiveAtomic` / `SetUserRoleAtomic` | 行2015/1982 | 原子启停/角色（含 last-admin 保护） |
| `CleanupExpiredEmailVerifications` | 行2944/2937 | 清除过期邮箱验证码记录 |
| `ClearUnverifiedEmails` / `CleanupUnverifiedEmailsByAge` | 行3063/2988 | 清理未验证邮箱（后者有 24h 年龄门限） |
| `UpsertInviteCode` / `InviteCode` / `ConsumeInviteCodeAndUpdateUser` | 行2726/2750/2843 | 邀请码管理 |
| `ParentOf` / `ChildrenOf` / `DetachInvite` | 行2908/2915/2928 | 邀请关系 |
| `AddAuditLog` / `ListAuditLogs` / `ClearAuditLogs` / `PruneAuditLogs` / `PruneAuditLogsByAge` | 行3502/3520/3547/3909/3924 | 审计日志（含条件裁剪） |
| `AddViolationLog` / `ListViolationLogs` | 行3445/3460 | 违规日志 |
| `AddLoginLog` / `LoginHistory` | —/— | 登录历史 |
| `ListDevices` / `UpdateDevice` / `DeleteDevice` | —/—/— | 设备管理 |

> 通用约定：错误码定义 `internal/api/errcode.go` ↔ 前端 `webui/src/lib/errcode.ts`/`validators.ts`；确认短语 `internal/api/confirm_phrases.go` ↔ `webui/src/lib/confirm-phrases.ts`；校验规则 `internal/validate` ↔ `webui/src/lib/password.ts`/`validators.ts`；全局状态 `webui/src/store/{auth,system}.ts`。完整路由清单见 [API 路由索引](docs/reference/api-index.md)。

### Feature Gate 约定

功能开关（`InviteEnabled`、`EmailEnabled`、`MediaRequestEnabled`、`SigninEnabled` 等）必须在**所有相关 handler 入口**处进行检查，不仅是前端隐藏 UI。关闭功能时后端接口必须返回明确错误（如 `INVITE_DISABLED`、`EMAIL_DISABLED`），不允许"前端隐藏但接口仍可用"。

| 开关 | 检查位置 | 错误码 |
|------|---------|--------|
| `InviteEnabled` | `handleCreateInviteCode`、`handleInviteCheck`、`handleInviteUse`、`handleInviteCodes`、`handleDeleteInviteCode`、`handleDetachExpiredInviteChild`、`handleUseCode`(邀请码路径) | `INVITE_DISABLED` |
| `SigninEnabled` | `handleSignin`、`handleSigninRenew` | `SIGNIN_DISABLED` |
| `MediaRequestEnabled` | `handleCreateMediaRequest`、`handleMyMediaRequests` | — |
| `emailConfigured()` | 所有发码/验证/找回密码 handler | `EMAIL_DISABLED` |
| `BangumiEnabled` | `handleBangumiWebhook`、`handleUpdateMe`(bgm 字段)、`handleBangumiSyncTrigger`、`handleAdminBangumiSyncUser` | `BANGUMI_SYNC_DISABLED` |
| `RegisterEnabled` | `handleRegister` | — |

注意：`TelegramMode` 控制的是 **Bot 模式**（`Global.telegram_mode`），不是 Telegram 绑定功能。用户自助绑定/解绑/换绑的 handler **不应**受 `TelegramMode` 限制。Bot 相关 handler 本身已有独立的 `telegramConfigured()` 检查。

续期码（`POST /invite/renew-codes`）在 `InviteEnabled=false` 时**仍可用**——这是刻意设计（测试 `TestInviteDisabledStillAllowsExistingChildRenewCodes` 验证），续期码是已有邀请树的维护功能，不是新邀请入口。

### 操作审计日志约定

`internal/api/audit_handlers.go` 提供 `a.audit(r, action, category, targetUID, detail)` 便捷方法：
- 自动从 `current(r)` 提取操作者 UID/Username
- 自动记录客户端 IP
- category: `"admin"` / `"user"` / `"system"`
- 保留上限 10000 条，超出自动裁剪
- 所有**状态变更操作**（创建/更新/删除/启停）都应在成功后调用 `a.audit()`
- 只读类接口（list/get/search）不需要审计

已覆盖的审计点：注册码 CRUD、邀请码生成/消费、用户自助（改资料/改密/绑定Emby/签到/使用卡码）、管理员（启停用户/删除/改角色/改到期/重置密码/解绑 Telegram/强制解绑）、批量操作等。

### RegCode Source 字段约定

`RegCode.Source` 区分卡码来源：
- `"admin"` — 管理员在后台手动生成（`POST /admin/regcodes`）
- `"invite"` — 邀请系统自动生成（`POST /invite/renew-codes`，邀请人为下级生成续期码）
- 空字符串（历史数据）— 视作 `"admin"`

前端管理员卡码页面：注册码 Tab 默认筛选 `source=admin`；邀请码 Tab 展示 `source=invite` 的 RegCode 与 InviteCode 条目合并显示。

### 用户列表筛选约定

管理员用户列表 `/admin/users` 支持多维独立筛选，`filteredBatchUserUIDs` 必须保持口径一致：

| 参数 | 含义 | 可选值 |
|------|------|--------|
| `role` | 角色 | `0`(管理员) / `1`(普通) / `2`(白名单) |
| `active` | Web 账号启停 | `true` / `false` |
| `emby` | Emby 绑定状态 | `bound` / `unbound` |
| `emby_status` | Emby 启停状态（独立于 Web） | `active` / `disabled` |
| `email_status` | 邮箱验证状态 | `verified` / `unverified` / `bound` / `none` |

### RegCode 有效期暂停约定

`RegCode` 支持停用期间暂停计算有效期：
- `PausedSeconds` — 累计暂停时长（秒），停用期间暂停计算 `ValidityTime` 倒计时
- `PauseStart` — 当前暂停起始时间戳（秒），0 表示未处于暂停状态
- `handleUpdateRegcode` 在 `active` 变化时自动记录/结算暂停时间
- `regcodeStatus` 和 `consumableRegCodeLocked` 在判断过期时扣除 `PausedSeconds` 和当前暂停时长
- 使用次数耗尽 (`use_count >= use_count_limit`) 的优先级高于 admin 手动停用，确保 `status` 返回 `used_up` 而非 `disabled`

### 认证页背景图约定

认证页（登录/注册）背景图：
- 上传接口：`POST /admin/config/upload-auth-background`，文件名固定为 `background.<ext>`
- 提供接口：`GET /system/auth-background`，优先返回 `background.<ext>`，不存在时兼容旧版 `?file=` 参数
- 配置键：`Global.auth_background_url`，默认空，支持环境变量 `TWILIGHT_AUTH_BACKGROUND_URL` 覆盖
- 前端通过 `handleSystemInfo` 返回的 `auth_background_url` 加载，AuthLayout 自动处理路径拼接

### 工单类型保护

`Ticket.types` 配置受最小保护：
- 通过配置保存页面（schema 编辑器或原始 TOML 编辑器）保存时，`ensureTicketDefaults` 自动确保至少保留一个类型（默认 `"all"`），防止管理员清空所有类型导致 fallback 失效
- 管理员可在工单管理页自由增删改类型，默认仅含 `"all"` 一个类型

### 注册邮箱验证约定

注册时如果填写了邮箱且 SMTP 已配置，自动发送验证邮件：
- `handleRegister` 在创建用户成功后调用 `issueEmailCode` 发送绑定验证码
- 验证码发送失败不影响注册结果（仅记录日志）
- 响应中包含 `email_verification_sent` 字段（验证记录 ID，发送失败时为空）
- 邮箱管理页面支持一键清空未验证邮箱（`POST /admin/email/verifications/clear-unverified`）

### 认证页 UI 约定

认证页（登录/注册/找回密码）采用右侧固定毛玻璃面板 + 左侧背景图/装饰布局：

**布局结构：**
- 面板由 `AuthLayout`（`webui/src/app/(auth)/layout.tsx`）直接渲染 `<main className="auth-panel">`，不放在各页面组件内
- 因为 Next.js App Router 的 layout 跨页面导航持久化，面板本身不随页面切换重新挂载——无闪动
- 各页面（login / register / forgot-password）仅返回表单内容，共享 `webui/src/app/(auth)/auth-ui.tsx` 中的 `AuthBrand`、`AuthStepDots`、按钮/链接样式常量
- 桌面端面板宽度 `clamp(440px, 38vw, 560px)`，移动端全宽

**环境变量定制（全部可选的 `NEXT_PUBLIC_*`，构建时注入）：**

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `NEXT_PUBLIC_AUTH_TEXT_COLOR` | — | 覆盖文字颜色（CSS 颜色值），经正则防注入 |
| `NEXT_PUBLIC_AUTH_ICON_URL` | — | 覆盖品牌图标 URL，优先于后端 `server_icon` |
| `NEXT_PUBLIC_AUTH_BG_OVERLAY_OPACITY` | `0.4` | 背景图叠加层不透明度（0-1） |
| `NEXT_PUBLIC_AUTH_PANEL_OPACITY` | `0.82` | 右侧面板毛玻璃不透明度（0-1），有背景图时自动 +0.08 |

**背景与可读性：**
- 有自定义背景图时，叠加层 `opacity` 由 `NEXT_PUBLIC_AUTH_BG_OVERLAY_OPACITY` 控制，面板不透明度由 `NEXT_PUBLIC_AUTH_PANEL_OPACITY` 叠加 +0.08 控制
- 文字使用多层 `text-shadow`（`.auth-card-text`）确保在任何背景上可读
- 注册向导使用 `.auth-step-dot` 进度点指示当前步骤

**动画与过渡：**
- `animate-auth-enter` 仅首屏触发，页面内导航时不重播
- `prefers-reduced-motion` 时所有动画降级为瞬态

### 邮箱登录约定

后端 `handleLogin` 同时支持用户名和邮箱登录：
- 请求体含 `email` 字段时走 `FindUserByEmail`（`internal/store/store.go`）
- `username` 字段含 `@` 也自动走邮箱查找（前端 `api.ts` 的 `login()` 自动检测）
- 邮箱查找大小写不敏感（`strings.EqualFold`）
- 失败时统一返回 `AUTH_LOGIN_INVALID`，不区分"用户不存在"与"密码错误"（防枚举）

### 邮箱 SMTP 发件限制处理

SMTP 服务商（如 QQ 邮箱、Outlook 等）通常有每小时/每日发件数量限制。后端处理：
- `smtpDeliver` 返回错误时，`issueEmailCode` 使用 `ErrEmailSendFailed`（HTTP 502）
- 错误日志经 `redactSensitiveText` 脱敏，不向用户暴露 SMTP 原始拒绝原因
- 后端不做 SMTP 层面重试（服务商限流期间重试无意义）

前端处理：
- `EMAIL_SEND_FAILED` 加入 `isThrottleErrorCode` 集合，触发本地冷却（120s，高于普通限流的 60s）
- `throttleCooldownSeconds(code)` 函数返回不同错误码对应的冷却时长
- 友好文案提示用户"邮件服务暂时不可用，可能是发件数量达到上限"，引导稍后重试或联系管理员
- 管理员应在 `config.toml` 的 `[RateLimit]` 段合理设置 `email_code_ip_per_10m` / `email_code_uid_per_10m` / `email_code_addr_per_10m`，使应用侧限流严于 SMTP 服务商限额

### 线路测速约定

- 测速前先通过 `GET /emby/status` 检测 Emby 在线状态
- 如果 Emby 不在线（`online: false`），跳过所有线路测速，直接标记所有线路为不可达
- 如果 Emby 状态接口异常（网络错误等），同样标记所有线路为不可达（不继续测速）
- 用户可手动刷新重新测速，每次测速都会重新请求状态接口

## 常用命令

后端：

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build -o bin/twilight ./cmd/twilight
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml --debug
```

启动脚本：

```bash
bash start_backend_dev.sh
bash start_backend_prod.sh
```

前端：

```bash
cd webui
pnpm install --frozen-lockfile
pnpm dev
pnpm lint
pnpm build
```

按改动范围选择验证命令。涉及 Go 代码至少运行 `gofmt` 和相关 `go test`；涉及前端或 API 客户端时运行 `pnpm lint`，必要时运行 `pnpm build`。如果本机缺依赖或外部服务不可用，必须在回复中明确说明未执行的验证和原因。

## 后端约定

- 新路由统一在 `internal/api/routes.go` 注册，使用 `a.add(method, pattern, auth, handler)` 声明方法、路径、鉴权级别和 handler。
- 不要新增或恢复 `TestWeb`、`demo`、mock、调试绕过或临时公开测试路由。保留的连通性测试接口（如 Emby/Telegram test）必须是正式功能，走明确鉴权，使用统一响应并脱敏错误。
- 鉴权级别为 `AuthPublic`、`AuthUser`、`AuthAdmin`、`AuthAPIKey`。管理员破坏性操作必须明确权限边界，优先支持 `dry_run`、确认短语和结构化结果。
- Handler 负责参数校验、鉴权上下文读取、调用业务逻辑和整理响应；可复用业务逻辑放在对应领域文件，外部服务调用走独立 client/helper，不要散落在 handler 中。
- 后端新增能力优先按业务域组织代码：路由只做入口，handler 只做 HTTP 适配，核心业务放在领域 service/helper，外部系统访问放在 client，持久化只通过 `internal/store` 暴露的方法完成。
- 保持低耦合：不要让业务逻辑直接依赖 `http.Request`、全局配置、全局 logger 或具体第三方 API 响应结构。需要这些信息时，由 handler 解析后以明确参数传入。
- 抽象要适度且有边界：只有在存在复用、需要替换外部依赖、需要隔离副作用或能显著降低测试成本时才引入接口/抽象；不要为了“架构感”增加空泛接口、过深目录或单方法包装层。
- 领域代码应优先表达业务概念和状态转移，避免把注册、续期、邀请、求片、Emby、Telegram、调度等不同领域混进同一个大函数或通用杂物文件。新增大功能时先选择清晰文件名，必要时拆成 `*_handlers.go`、`*_service.go`、`*_client.go`、`*_test.go`。
- 新人可维护性优先：函数命名要能说明业务动作；复杂流程拆成可顺序阅读的小步骤；非显而易见的权限、并发、回滚、远端副作用和确认短语必须用短注释说明原因。
- 避免循环依赖和隐式副作用。不要在包初始化、全局变量或 helper 中偷偷读取/修改配置、store、环境变量或网络资源；启动、reload、scheduler、bot 的生命周期由现有入口显式管理。
- 优化前先确认瓶颈。缓存、并发、批处理、后台任务和 Redis 使用必须有清晰失效策略、限流/超时和测试覆盖；不要为小路径引入难以验证的全局缓存或共享可变状态。
- JSON 响应必须使用统一 envelope：`success`、`code`、`message`、`data`、`timestamp`，失败响应应带稳定 `error_code`，与前端 `ApiError` / `errcode.ts` 保持兼容。
- 游标类列表接口应保持字段稳定：`entries`/`items` 承载数据，`next_cursor` 表示下一次增量读取起点；运行日志 HTTP 响应固定为 `{entries,next_cursor,limit}`，SSE 事件固定为 `{entries,next_cursor}`，不得为短期 UI 需求改名或改变排序语义。
- Cookie 鉴权的变更类请求不再做 CSRF 令牌校验。鉴权依赖 HttpOnly session cookie、Bearer token 或 API Key；`X-Twilight-Client: webui` 只是允许的 CORS 头，不是鉴权手段。
- Telegram 直接登录当前不可用。Telegram 只用于绑定、通知、管理员工具和 Bot 交互，不要重新引入 Telegram 一键登录或信任 Telegram ID 的免密登录。
- Telegram Bot 账号类操作优先私聊；群聊只保留必要管理员工具。Bot/面板输出不得展示密码、Token、Emby ID、服务器线路、API Key 等敏感信息，按钮/面板操作必须重新校验管理员身份、目标权限和面板过期时间。
- Emby/Jellyfin 外部副作用必须先完成本地权限、容量、过期状态和绑定冲突校验；非系统管理员不得绑定或操作 Emby 管理员账号。Emby 线路下发统一走 `/api/v1/system/emby-urls` 并按用户状态/权限过滤。
- 运行时可热重载的 `cfg/store/sessions/limiter/redis` 通过 `runtimeState` 原子快照管理。读配置或 store 时优先使用 `a.cfg()`、`a.store()` 等访问器，不要缓存会跨 reload 失效的句柄。
- 配置入口固定为工作目录下的 `config.toml`；`--config` 只接受同一个工作目录的 `config.toml`。私密覆盖使用同目录 `config.local.toml` 或 `TWILIGHT_CONFIG_LOCAL_FILE`，环境变量以 `TWILIGHT_*` 覆盖字段。新增配置项一律落到 `config.toml`（在 `config.production.toml` 模板与后台 schema 中体现），**不要**把功能配置写进 `.env.example`——后端 `.env` 仅保留后端监听地址、站点名称等极少数部署级项目，前端展示项（API 基址 / 站点名 / 介绍 / 图标）走 `webui/.env`。
- 存储模型是单一 `store.State` 文档。新增业务实体通常是在 `internal/store/store.go` 的 `State` 上加字段，并在 `ensure()` 中补默认值；不要为邀请、公告、注册码等业务重新创建独立 SQLite 文件或独立表。
- PostgreSQL 后端只把主状态存为 `twilight_state` 的单行 `jsonb`，并有独立 `twilight_sessions` 与 `twilight_runtime_logs`。迁移、备份、恢复必须保持快照完整性。
- `twilight_runtime_logs` 是高写入运行日志表：读取使用 `id` 游标递增，最新快照使用最近 N 条，裁剪必须保留最近 N 条并优先使用 cutoff id / 索引友好的 SQL；不要把业务实体迁移成独立表来“优化性能”。
- PostgreSQL 除现有 `twilight_sessions` 与 `twilight_runtime_logs` 外，不为业务实体新增独立表，除非先更新架构文档并说明快照一致性、迁移和备份恢复方案。
- JSON 后端是单进程独占，依赖 state 文件锁；多进程或多实例部署使用 PostgreSQL。
- 旧 SQLite 只允许用于显式只读迁移/引导兼容；不要在启动流程中新增隐式迁移、自动建表或旧 Python 后端兼容入口。
- 写入状态时复用 store 层已有原子写、备份、回滚和锁语义，不要绕过 `Store` 直接改 `db/twilight_go_state.json`。
- 上传、备份、恢复、迁移、Git 更新、systemd 操作等高风险路径必须做服务端边界校验。路径必须约束在允许目录内，拒绝绝对路径、`..`、符号链接和任意外部 URL。
- 执行外部命令必须使用 `exec.Command` 参数数组，禁止拼接 shell 字符串。Git remote URL、日志和响应中不得泄露凭据。
- 新增缓存必须说明作用域（App 实例/进程/Redis）、TTL、容量上限、热重载失效条件和降级行为；避免包级可变缓存导致测试串扰或 reload 后读旧配置。
- 高频外部调用（Emby/TMDB/Bangumi/Telegram）必须设置超时、限流/退避和必要短缓存；不要为低频路径引入难以验证的全局缓存或后台 goroutine。
- 验证码 / 发信 / 找回密码这类「可外发或易被滥刷」的接口遵循既有邮箱验证模式（`internal/api/email_*.go`、`internal/store/email_verification.go`，详见 [邮箱验证文档](docs/features/email.md)）：验证码只存服务端 HMAC 哈希（不存明文）+ 常量时间比较 + 尝试上限 + TTL；发码限流必须多维（IP + 登录账号 uid + 目标地址）并叠加重发冷却，新增发信入口不要只做单一维度限流；登出态找回走统一成功响应防枚举。强制验证门（如 `requireEmailVerified`）必须是服务端硬门、前端守卫只做体验，且依赖的外部能力（SMTP 等）未配置完整时强制门要自动失效，避免把用户锁死在面板外。
- 列表筛选若同时用于「跨页全选 + 批量操作」，筛选口径必须在列表 handler 与 `filteredBatchUserUIDs` 两处完全一致（如 `email_status`），否则「按筛选全选」会把筛选外用户卷入批量操作。
- 设备 / 登录 / IP 数据有**两套互不相同的来源**，不要混淆：① Emby 侧——登录用户的真实设备与 IP 由 Emby API 提供（`/Sessions` 的 `RemoteEndPoint`=客户端 IP、`DeviceName`/`Client`/`UserId`；`/Devices` 为设备清单 + `LastUserId`/`DateLastActivity`），用于管理员审查 Emby 用户的设备/IP；② 本地侧——`store.Device` 只记录 **Web 面板**自身的登录设备。涉及「Emby 用户设备/IP 审查」必须查 Emby API，不要拿本地 `store.Device` 充当 Emby 数据。
- 写本地 Web 登录设备用 `UpdateDevice`（读改写，保留 `FirstSeen`/`Trusted`/`Blocked`），不要用 `UpsertDevice` 整条覆盖（会把信任/封禁标记和首次时间冲掉）。
- 注册码更新走 `PUT /admin/regcodes/:code` 的**部分更新**（仅改 payload 中出现的字段：`note`/`active`/`validity_time`/`days`/`use_count_limit`），`UpsertRegCode` 的「强制 active=true」兜底只对新建码生效，更新已存在码可正常停用。

## 后台模块迁移约定（2026-06）

- 后台总入口为「管理导航」：`webui/src/app/(main)/admin/page.tsx`。新增后台页面需同步侧边栏、管理导航、API 客户端和文档。
- 邮箱、Telegram、邀请、安全配置已迁移到独立管理页：`/admin/email`、`/admin/telegram`、`/admin/invite`、`/admin/security`。配置管理只保留默认折叠的兼容入口和跳转提示。
- 迁移模块仍必须使用同一个配置源：通过 `/system/admin/config/schema` 读写 `config.toml`，不得新增前端本地配置、store 配置实体或重复保存逻辑。
- `Ticket.types` 不在配置管理中编辑，统一使用「工单处理 → 工单类型配置」。工单类型变更需要同步写回 `config.toml`，避免热重载或重启后回退。
- 配置查看、schema 编辑和备份查看不得泄露 API Key、Token、密码、Secret 等敏感字段；未修改的 secret 必须使用服务端哨兵保留，禁止明文回显。
- 新增后台接口默认使用 `AuthAdmin`；所有创建、更新、删除、启停、沙箱执行等状态变更成功后必须写审计日志。无 HTTP 上下文的 Bot 路径使用 `a.auditEntryIP("telegram", ...)`。
- 开发者模式入口固定为仪表盘输入 `DEBUGMODE`，再调用 `POST /admin/developer-mode/activate` 做管理员密码二次验证。Telegram JS 自定义命令仅允许 `bot_custom_commands` 中 `js:` 前缀脚本，必须运行在受控沙箱中，不得暴露网络、文件、进程、环境变量或配置读取能力。

## 前端约定

- Emby 管理与设备/IP 审查共用 `/admin/emby`，设备审查是页签级入口；`/admin/device-audit` 仅作为兼容直达页面保留。
- 后台重型面板应按需加载，非默认页签不得首屏自动请求大接口；公共系统信息复用 `useSystemStore.fetchInfo()` 的 TTL 与 inflight 缓存，配置保存后调用 `invalidate()`。
- 侧边栏、移动菜单、管理导航这类密集导航区域的 `Link` 默认设置 `prefetch={false}`，避免首屏预载大量后台页面 chunk；仅对明确高频下一跳保留预取。
- 所有后端调用集中维护在 `webui/src/lib/api.ts`，底层请求统一走 `webui/src/lib/api-request.ts`。不要在页面中散落裸 `fetch`，除非有明确理由并保持相同的 credentials、超时和错误处理语义。
- `apiRequest` 会自动使用 `${NEXT_PUBLIC_API_URL}/api/v1`；未设置 `NEXT_PUBLIC_API_URL` 时，`next.config.mjs` 将 `/api/*` rewrite 到 `BACKEND_URL`，默认 `http://localhost:5000`。
- 新增或调整接口时，同步检查 `api.ts`、`api-types.ts`、后端路由、请求方法、鉴权级别、错误码、确认短语和移动端展示。
- 新增前端 API 错误码时同步 `webui/src/lib/errcode.ts` 和友好文案；移除后端错误码或路由时同步删除前端类型、客户端方法、页面引用和文档索引。
- 限流 / 冷却类错误码（见 `isThrottleErrorCode`）命中后，发送 / 重发按钮应本地起一段冷却自禁，避免无效重试放大滥刷压力。
- 新增面向用户的文案必须走 `t()` i18n。locale 文件架构（见 `i18n.tsx`）：`basic.json` 是**全键兜底基底**（简体），`zh-Hant.json` / `en-US.json` 是**完整镜像**，`zh-Hans.json` 是**稀疏覆盖**（缺的键回退 basic）。新增键只加到 `basic.json` + `zh-Hant.json` + `en-US.json` 三份并保持键对齐，**不要**加到 `zh-Hans.json`；漏键回退到 basic.json（不是英文）。
- 页面分组：`webui/src/app/(auth)` 放登录、注册、找回密码；`webui/src/app/(main)` 放用户面板与管理页。新增 UI 优先复用 `webui/src/components`、`hooks` 和 `lib` 中已有模式。
- 保持现有中文文案、暗色/亮色主题、Tailwind token 和组件风格。前端改动必须兼顾桌面与移动端。
- 资产 URL、背景 CSS、头像、上传结果等必须沿用 `api.ts` 中的安全归一化逻辑，不允许保存任意外部 `url()` 或不受控协议。
- 前端资产、背景、头像和外链跳转必须复用 `safe-url` / API 客户端归一化逻辑，不要把后端返回值直接拼进 CSS `url()`、`href` 或图片地址。
- 轮询、SSE、后台刷新和批量操作必须有停止条件、可见性判断或退避策略；终态任务要及时清理本地轮询状态，避免隐藏标签页持续请求。
- 公告内容渲染统一走 `SafeAnnouncementContent`（`webui/src/lib/safe-render.tsx`），支持 Markdown / BBCode / plain 三种模式。Markdown 解析器为手写实现（无第三方 MD 库），仅支持安全子集（标题、列表、代码块、引用、分割线、行内格式、链接、图片）。列表项使用 `list-inside` 避免圆点被容器裁剪。公告板组件 `AnnouncementBoard` 支持长内容折叠（maxHeight + Expand/Collapse）。
- 新增配置项时：后端 config struct 加字段 → `defaults()` 补默认值 → `config_admin.go` 的 `configSectionDefs()` 加 schema 定义 + `configValues()` 加值映射 → `config.production.toml` 模板中体现。邮箱白名单/黑名单/验证模式属于 Email section，不属于 SAR/Register section。

## 安全与敏感信息

- 当前工作区的 `config.toml` 可能包含真实 Token、密码、API Key、Telegram ID、Emby 凭据等。不要在回答、日志、测试输出、文档或提交中复制这些值。
- 除非任务明确需要配置分析，不要读取或引用本地 `config.toml`。需要配置示例时使用 `.env.example`、`config.production.toml` 或脱敏占位符。
- 不要读取或展示非必要的运行数据：`db/`、`uploads/`、`memory/`、`tmp/`、`.venv/`、`bin/`、`twilight.exe` 等通常是本地运行产物或敏感数据来源。
- 示例配置使用 `.env.example`、`config.production.toml` 或占位符；新增文档中的密钥必须写成 `<PLACEHOLDER>` 或脱敏形式。
- 除一次性创建/重置密码、API Key 的既有响应外，不返回密钥明文。日志必须脱敏 `token`、`secret`、`password`、`api_key`、`Authorization`、`Cookie`、DSN 等字段。
- 生产 CORS 必须显式列出可信 HTTPS Origin，携带凭据接口不接受 `*`。信任代理头时必须配置可信代理 CIDR。
- HTTPS 生产环境保持 `session_cookie_secure = true`。本地 HTTP 调试才可显式关闭。

## 代码风格

- 遵守 `.editorconfig`：UTF-8、LF、文件末尾换行、去除尾随空白；Markdown 允许保留行尾空格。
- Go 代码必须 `gofmt`，保持小而清晰的函数边界，优先复用现有 store、api、config helper。
- Go 文件和函数要面向后续维护者组织：同一业务域的类型、校验、服务逻辑和测试尽量就近；跨领域复用的 helper 才放到更通用的位置。
- 后端模块拆分遵循 [模块化架构与解耦指南](docs/guides/modular-architecture.md)：handler 只做 HTTP 适配，service 表达业务状态转移，client 只处理外部协议，store 只负责单一状态文档与原子写入；不要为了抽象新增空泛单方法接口。
- 包内私有函数优先于导出符号；只有跨包确实需要使用时才导出，并为非显而易见的导出类型/函数补充说明。
- 测试应覆盖业务边界而不是只覆盖实现细节。涉及 store 状态转移、权限分支、确认短语、外部 client 降级、并发锁和回滚时，优先写表驱动或聚焦单元测试。
- TypeScript/React 使用 2 空格缩进，遵守现有 ESLint/Next 配置。不要无必要引入新状态库、UI 库或工具函数。
- 注释只解释非显而易见的安全、并发、迁移或兼容原因，避免重复代码字面含义。
- 保持最小正确改动。不要为没有实际需求的旧行为添加兼容层。

## 验证与交付

- 后端改动提交前检查：`gofmt -w ./cmd ./internal`、`go test ./...`、`go vet ./...`。
- 前端改动提交前检查：`cd webui && pnpm lint`，涉及构建、路由、Next 配置或 API 契约时运行 `pnpm build`。
- 涉及鉴权、Telegram、Emby、API Key、上传、路径、配置保存、数据库迁移、Git 更新、systemd、实时日志、限流、会话、缓存或并发的改动必须补充聚焦测试或明确说明无法测试的边界。
- 不要修改用户未要求的本地配置、数据库、上传文件、生成产物或部署状态。不要执行破坏性命令，不要重置工作区，不要替用户提交或推送，除非用户明确要求。
