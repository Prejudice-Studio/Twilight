# 邮箱验证与找回密码

本文说明 Twilight 的邮箱验证子系统：SMTP 发信、验证码格式与有效期、绑定 / 验证邮箱、强制绑定门、改密二次校验、登出态邮箱找回密码、邮箱域名黑白名单，以及管理员的邮箱验证管理区。已对照后端代码（`internal/api/email_handlers.go`、`internal/api/email_verify_service.go`、`internal/api/email_client.go`、`internal/store/email_verification.go`、`internal/config/config.go`、`internal/api/config_admin.go`、`internal/api/routes.go`、`internal/api/errcode.go`）核对了字段、限流、原子消费与鉴权。

配置项速查见 [`config.production.toml`](../../config.production.toml) 的 `[Email]` / `[SAR]` / `[RateLimit]` 段（密钥如 SMTP 密码放同目录 `config.local.toml`）；管理员也可在后台「系统配置 → 邮箱验证」可视化修改并热重载。邮箱属功能配置，统一在 `config.toml` 配置，不建议写进 `.env`。下表括号内的 `TWILIGHT_*` 仅为可选的环境变量覆盖名。

## 总开关与降级

`[Email].enabled`（环境变量 `TWILIGHT_EMAIL_ENABLED`）是整个子系统的总开关。后端实际是否「可用」由 `emailConfigured`（`email_client.go`）判定：

> `enabled=true` **且** `smtp_host` 非空 **且**（`smtp_from_address` 或 `smtp_username` 至少一个非空）。

任一不满足时，所有发码 / 邮箱找回 / 强制绑定都降级：发码接口返回 `EMAIL_DISABLED`，前端隐藏邮箱相关入口，`features.email_enabled` 公开标志为 `false`。

**防锁死约定**：即便 `force_bind=true`，只要 `emailConfigured` 为 `false`，强制验证门 `emailGateActive` 也会返回 `false`（`email_verify_service.go`）。这样「开了强制绑定却没配好 SMTP」不会把用户永久挡在仪表盘外。

## 存储模型

和全部业务状态一样，邮箱验证记录保存在「单一状态文档」（JSON 文件 `db/twilight_go_state.json` 或 PostgreSQL `twilight_state` 表的 `jsonb` 行）中，**没有**独立的邮箱数据库或表。

- `EmailVerification` 以随机 `id`（32 位）为键存放在状态文档的 `EmailVerifications` 映射里。**只存验证码的 HMAC 哈希 `CodeHash`，永不存明文。**
- 用户记录上的邮箱状态由三字段表达：`Email`、`EmailVerified`（布尔）、`EmailVerifiedAt`（验证时间戳，撤销时清零）。

每个 `(purpose, email)` 至多保留一条在用记录：`PutEmailVerification` 落库前会原子清掉同一用途 + 邮箱的旧记录，因此重复发码不累积、永远以最新码为准。

## 配置项

### `[Email]` 段（SMTP / 验证码 / 强制绑定 / 模板）

| 键（环境变量） | 默认 | 说明 |
| ---- | ---- | ---- |
| `enabled`（`TWILIGHT_EMAIL_ENABLED`） | `false` | 子系统总开关。 |
| `smtp_host`（`TWILIGHT_SMTP_HOST`） | `""` | 发信服务器地址。 |
| `smtp_port`（`TWILIGHT_SMTP_PORT`） | `587` | 端口：465(SSL) / 587(STARTTLS) / 25(明文)。 |
| `smtp_username`（`TWILIGHT_SMTP_USERNAME`） | `""` | 登录用户名；留空表示不认证（仅内网中继）。 |
| `smtp_password`（`TWILIGHT_SMTP_PASSWORD`） | `""` | 密码或授权码。后台展示为 `secret`，错误信息经脱敏。 |
| `smtp_encryption`（`TWILIGHT_SMTP_ENCRYPTION`） | `starttls` | `ssl`=隐式 TLS；`starttls`=显式 TLS；`none`/`plain`=明文。加密方式与端口解耦。 |
| `smtp_from_address`（`TWILIGHT_SMTP_FROM_ADDRESS`） | `""` | From 地址；留空回落到 `smtp_username`。 |
| `smtp_from_name`（`TWILIGHT_SMTP_FROM_NAME`） | `""` | From 显示名；留空回落到站点名 `server_name`。 |
| `smtp_timeout_seconds`（`TWILIGHT_SMTP_TIMEOUT_SECONDS`） | `10` | 单次发信超时秒数。 |
| `force_bind`（`TWILIGHT_EMAIL_FORCE_BIND`） | `false` | 强制绑定邮箱，见下文「强制绑定门」。 |
| `code_length`（`TWILIGHT_EMAIL_CODE_LENGTH`） | `6` | 验证码长度，取值夹取到 `4`-`12`。 |
| `code_type`（`TWILIGHT_EMAIL_CODE_TYPE`） | `numeric` | `numeric`=纯数字；`alphanumeric`=大写字母+数字（去除 `0/1/I/O` 等易混淆字符）。 |
| `code_ttl_minutes`（`TWILIGHT_EMAIL_CODE_TTL_MINUTES`） | `10` | 验证码有效期（分钟）；`<=0` 回落 10。 |
| `resend_cooldown_seconds`（`TWILIGHT_EMAIL_RESEND_COOLDOWN_SECONDS`） | `60` | 同一邮箱两次发码的最小间隔（秒）。 |
| `max_attempts`（`TWILIGHT_EMAIL_MAX_ATTEMPTS`） | `5` | 单个验证码允许的错误尝试次数，超过即作废；`<=0` 回落 5。 |
| `auto_cleanup_expired_verifications`（`TWILIGHT_EMAIL_AUTO_CLEANUP_EXPIRED_VERIFICATIONS`） | `true` | 随调度器会话巡检自动删除已过期邮箱验证码记录。 |
| `auto_cleanup_unverified`（`TWILIGHT_EMAIL_AUTO_CLEANUP_UNVERIFIED`） | `true` | 自动清空长期未验证账号的邮箱字段，释放邮箱地址；不影响已验证邮箱。 |
| `auto_cleanup_unverified_days`（`TWILIGHT_EMAIL_AUTO_CLEANUP_UNVERIFIED_DAYS`） | `1` | 账号创建超过该天数仍未验证邮箱时自动清空邮箱字段。 |
| `subject_template`（`TWILIGHT_EMAIL_SUBJECT_TEMPLATE`） | `{site} 邮箱验证码` | 邮件标题模板。 |
| `body_template`（`TWILIGHT_EMAIL_BODY_TEMPLATE`） | 见下 | 邮件正文模板；env 覆写用字面量 `\n` 表示换行（加载时自动还原）。 |

模板占位符：`{site}`=站点名（`server_name`，缺省 `Twilight`）、`{code}`=验证码、`{ttl}`=有效分钟数。内置正文默认：

```
您正在 {site} 进行邮箱验证。

验证码：{code}

验证码 {ttl} 分钟内有效，请勿向任何人泄露。如非本人操作，请忽略本邮件。
```

### `[SAR]` 段（邮箱域名黑白名单）

域名名单与「注册」共用，归在 `[SAR]` 段（后台「注册/邀请」配置区）：

| 键（环境变量） | 默认 | 说明 |
| ---- | ---- | ---- |
| `email_validation_mode`（`TWILIGHT_EMAIL_VALIDATION_MODE`） | `""` | `whitelist`=仅允许名单内域名；`blacklist`=禁止名单内域名；留空=不限制。 |
| `email_whitelist`（`TWILIGHT_EMAIL_WHITELIST`） | `[]` | 白名单域名，如 `["gmail.com", "qq.com"]`。env 用逗号分隔。 |
| `email_blacklist`（`TWILIGHT_EMAIL_BLACKLIST`） | `[]` | 黑名单域名，如 `["mailinator.com", "10minutemail.com"]`。env 用逗号分隔。 |

匹配规则（`internal/validate/validate.go` 的 `CheckEmailWhitelist` / `CheckEmailBlacklist`）：大小写不敏感；条目不含 `@` 时按域名匹配并命中子域（`example.com` 命中 `a.example.com`），含 `@` 时按完整邮箱精确匹配。**发码时白名单优先**：配了白名单则必须命中白名单，否则若命中黑名单则拒绝，均返回 `USER_EMAIL_INVALID`。

### 限流

| 键（环境变量） | 默认 | 说明 |
| ---- | ---- | ---- |
| `RateLimit.email_code_ip_per_10m`（`TWILIGHT_RATE_LIMIT_EMAIL_CODE_IP_PER_10M`） | `20` | 每 IP 每 10 分钟发码上限。 |
| `RateLimit.email_code_addr_per_10m`（`TWILIGHT_RATE_LIMIT_EMAIL_CODE_ADDR_PER_10M`） | `5` | 每收件邮箱每 10 分钟发码上限。 |
| `RateLimit.email_code_uid_per_10m`（`TWILIGHT_RATE_LIMIT_EMAIL_CODE_UID_PER_10M`） | `10` | 每登录账号每 10 分钟发码上限，防同一账号轮换收件邮箱绕过「按地址」限流滥刷。 |
| `RateLimit.forgot_password_ip_per_10m`（`TWILIGHT_RATE_LIMIT_FORGOT_PASSWORD_IP_PER_10M`） | `20` | 邮箱找回密码每 IP 每 10 分钟上限。 |

上表各项在 `config.production.toml` 的 `[RateLimit]` 段已列出，也可在后台「系统配置 → 限流策略」可视化修改（热重载）；括号内 `TWILIGHT_*` 为可选环境变量覆盖名。

> **多维发码限流**：`issueEmailCode` 依次过 IP → 登录账号（uid）→ 收件地址三道闸（任一超限即 `EMAIL_RATE_LIMITED`），再叠加「同一邮箱重发冷却」。`email_code_uid_per_10m` 是登录态（绑定 / 改密）的单账号闸，专门拦截「同一账号不断换收件邮箱」这种能绕过按地址限流的滥刷。前端在收到限流响应后还会本地起一段冷却，让发送按钮自禁、减少无效重试。

## 验证码安全模型

`email_verify_service.go` + `email_verification.go`：

- **永不存明文**：API 层用服务端密钥对 `(id|code)` 做 HMAC-SHA256（`hashEmailCode`），store 只存哈希并做**定长常量时间比较**（`crypto/subtle.ConstantTimeCompare`）。即使状态文档泄露也读不出明文码。
- **HMAC 密钥来源**：优先用 `[Security].bot_internal_secret` 派生（加 `twilight-email-code|` 前缀做域分隔，跨重启稳定）；未配置时回退到进程级随机密钥——重启即让在飞码作废，属可接受的 fail-closed（而非退回可预测密钥）。
- **尝试上限**：每条记录 `Attempts/MaxAttempts`，错码累加，达上限即删除记录并返回 `EMAIL_CODE_TOO_MANY`，挡在线爆破。
- **有效期**：`ExpiresAt` 到点即作废（`EMAIL_CODE_EXPIRED`）；过期记录由定时任务清理。
- **原子消费**：`ConsumeEmailVerificationAtomic` 在 store 全局写锁内完成「校验 + 计数 + 删除」，命中即一次性消费删除，杜绝并发重放。
- **发信优先于落库**：先发信成功才把记录落库，避免 SMTP 失败却在 store 留下用户永远拿不到的「幽灵码」。

## 验证码用途（purpose）

| purpose | 触发场景 | 鉴权 | 校验目标 |
| ---- | ---- | ---- | ---- |
| `bind` | 绑定 / 验证邮箱 | AuthUser | 给新邮箱发码；校验后写入 `User.Email` 并置 `EmailVerified=true`。 |
| `reset_password` | 登出态找回系统密码 | AuthPublic | 按「已验证邮箱」命中账号发码；校验后重置系统密码。 |
| `change_password` | 面板内改系统密码 | AuthUser | 给已绑定邮箱发码；强制绑定开启时改密需附带。 |
| `change_emby_password` | 面板内改 Emby 密码 | AuthUser | 同上，要求账号已关联 Emby。 |

跨用途防越权：校验时同时核对 `rec.UID == 当前用户` 与 `rec.Purpose == 期望用途`，不能拿 `bind` 的码去改密、也不能用别人申请的码。

## 强制绑定门

`force_bind=true` 时（且 `emailConfigured` 为真）：

- **作用对象**：普通用户与白名单用户（`emailGateActive`）。**管理员豁免**，仅在仪表盘底部显示可关闭的提示横幅，不阻断。
- **服务端硬门**：`requireEmailVerified`（`email_handlers.go`）是不可绕过的服务端防线，前端守卫只做体验。未验证邮箱的受约束用户访问「价值型接口」（如使用卡码 `code_use_handlers.go`、求片 `media_request_handlers.go` 等）会被 `403` + `USER_EMAIL_VERIFICATION_REQUIRED` 拦截。
- **改密二次校验**：受约束用户改系统密码 / Emby 密码时，`consumePasswordChangeEmailCode` 要求附带 `verification_id` + `email_code`（命中本人、对应用途的有效码）；未强制时此步直接放行，保持向后兼容。
- 前端入口：全屏接管守卫见 `webui/src/components/email-verify-guard.tsx`（挂载于 `(main)/layout.tsx`）。

## 登出态找回密码（防枚举）

两步式，均按 IP 限流（`forgot_password_ip_per_10m`）：

1. `POST /api/v1/auth/password/email/request`：只有当邮箱已被某 `Active` 账号**验证**时才真正发码；无论命中与否都返回统一成功文案，且吞掉内部失败（限流/冷却/SMTP），防止账号枚举。
2. `POST /api/v1/auth/password/email/reset`：校验 `email + code + new_password`，命中 `reset_password` 用途且属于该账号才重置；通过后删除该用户全部会话。

> 只认 `EmailVerified=true` 的账号（`FindUserByEmailVerified`）：未验证邮箱不足以证明归属，否则把别人邮箱写成自己的未验证邮箱即可劫持对方找回入口。

## 管理员：邮箱验证管理区

后台「用户管理」页（`webui/src/app/(main)/admin/users/`）：

- **筛选**：邮箱功能开启时新增「邮箱验证」筛选下拉——`verified`（已验证）/ `unverified`（已填邮箱未验证）/ `bound`（已填邮箱不论验证）/ `none`（未填邮箱）。该口径在后端 `listUsers`（`handlers.go`）与跨页全选 `filteredBatchUserUIDs`（`batch_user_handlers.go`）两处保持一致，避免「按邮箱筛选后全选跨页」误伤筛选外用户。
- **行内状态**：用户列表邮箱旁显示「已验证 / 未验证」徽标。
- **强制绑定 / 置验证状态**：行操作菜单「绑定邮箱」打开管理弹窗（`admin-email-dialog.tsx`），可把用户强制绑定到指定邮箱（默认直接标记已验证），或在不改邮箱的前提下置 / 撤销验证状态。
- **发信测试**：后台「系统配置 → 邮箱验证」段内置「发送测试邮件」按钮，用当前 SMTP 配置发一封测试信验证连通性。

管理接口（鉴权 AuthAdmin）：

| 方法与路径 | 处理函数 | 说明 |
| ---- | ---- | ---- |
| `POST /api/v1/admin/users/:uid/bind-email` | `handleAdminBindUserEmail` | 强制绑定指定邮箱。`mark_verified`（默认 true）控制是否同时标记已验证；`force=true` 时跳过黑白名单与占用冲突校验（管理员断言归属）。 |
| `POST /api/v1/admin/users/:uid/email/verified` | `handleAdminSetUserEmailVerified` | 不改邮箱，仅置 / 撤销验证状态。 |
| `POST /api/v1/admin/email/test` | `handleAdminEmailTest` | 用当前 SMTP 配置发测试邮件，结果脱敏返回。 |
| `POST /api/v1/admin/users/clear-emails` | `handleAdminClearUserEmails` | 批量清空未绑定关系用户的邮箱（需确认短语）。 |

> 邮箱占用唯一性：`SetUserEmailVerifiedAtomic` 在 `verified=true` 且非 `force` 时校验是否被其它**已验证**账号占用，冲突返回 `USER_EMAIL_CONFLICT`。

## 接口速览

| 入口 | 鉴权 | 限流 / 防护 |
| ---- | ---- | ---- |
| `POST /api/v1/users/me/email/send-code` | AuthUser | 发码：IP + 收件地址双限流 + 重发冷却；`bind` 用途先查邮箱占用 |
| `POST /api/v1/users/me/email/verify` | AuthUser | 校验 `bind` 码并完成绑定 + 置已验证 |
| `POST /api/v1/auth/password/email/request` | AuthPublic | 按 IP 限流；统一成功响应防枚举 |
| `POST /api/v1/auth/password/email/reset` | AuthPublic | 按 IP 限流；仅命中已验证账号 + 对应用途码 |
| `POST /api/v1/admin/users/:uid/bind-email` | AuthAdmin | 管理员强制绑定，可 `force` 越过名单/冲突 |
| `POST /api/v1/admin/users/:uid/email/verified` | AuthAdmin | 置 / 撤销验证状态 |
| `POST /api/v1/admin/email/test` | AuthAdmin | SMTP 发信连通性测试 |

响应里的邮箱经 `maskEmail` 局部遮蔽（保留首尾少量字符），避免在共享屏幕 / 日志完整暴露。统一响应 envelope 与鉴权约定见 [API 路由索引](../reference/api-index.md) 与 [后端 API 详参](../reference/backend-api.md)。

## 错误码

`internal/api/errcode.go`：`EMAIL_DISABLED`、`EMAIL_NOT_BOUND`、`EMAIL_CODE_REQUIRED`、`EMAIL_CODE_INVALID`、`EMAIL_CODE_EXPIRED`、`EMAIL_CODE_TOO_MANY`、`EMAIL_SEND_FAILED`、`EMAIL_RESEND_COOLDOWN`、`EMAIL_RATE_LIMITED`、`EMAIL_PURPOSE_INVALID`、`USER_EMAIL_INVALID`、`USER_EMAIL_CONFLICT`、`USER_EMAIL_VERIFICATION_REQUIRED`、`USER_EMAIL_ALREADY_VERIFIED`。前端文案映射见 `webui/src/lib/errcode.ts`，i18n 键在 `email.*`（`basic` / `en-US` / `zh-Hant` 三语对齐）。

## 定时清理

过期验证码和未验证邮箱由 `cleanup_sessions` 调度任务顺带清理，执行间隔由 `[Scheduler].session_cleanup_interval` 控制（小时）。后台「邮箱管理 → 邮箱配置」会同时展示 `[Email]` 的清理开关和该调度间隔。

- `auto_cleanup_expired_verifications=true` 时，`scheduler_runner.go` 调用 `CleanupExpiredEmailVerifications` 删除所有 `ExpiresAt` 已过期的记录，避免状态文档堆积废码。
- `auto_cleanup_unverified=true` 时，`CleanupUnverifiedEmailsByAge` 会清空创建超过 `auto_cleanup_unverified_days` 天且仍未验证的用户邮箱字段，释放邮箱地址；不会清空已验证邮箱，也不会删除账号。
- 手动按钮仍保留在邮箱管理页：「清理过期」立即清理已过期验证码，「清空未验证邮箱」用于管理员显式执行一次全量未验证邮箱清空。

## SMTP 服务商发件限额

常见 SMTP 服务商有每小时/每日发件数量限制（如 QQ 邮箱约 50 封/小时、Outlook 约 300 封/日）。当服务商拒绝发信时：

**后端行为：**
- `smtpDeliver` 返回任何错误（含 `421 Too many messages` 等限流拒绝）均归入 `EMAIL_SEND_FAILED`（HTTP 502）
- 原始 SMTP 拒绝原因经 `redactSensitiveText` 脱敏后记入日志，不向用户暴露
- 后端不做 SMTP 层重试：服务商限流期间重试无意义，只会浪费配额

**前端行为：**
- `EMAIL_SEND_FAILED` 加入 `isThrottleErrorCode` 集合，命中后触发 120 秒本地冷却（高于普通限流的 60 秒）
- `throttleCooldownSeconds(code)` 根据不同错误码返回差异化冷却时长
- 友好文案提示用户"邮件服务暂时不可用，可能是发件数量达到上限"，引导稍后重试或联系管理员

**运维建议：**
- 在 `[RateLimit]` 段设置应用侧限流（`email_code_ip_per_10m` / `email_code_uid_per_10m` / `email_code_addr_per_10m`），使应用侧限流**严于** SMTP 服务商限额，避免大量请求打到 SMTP 被服务商拒绝
- 若发件量大，考虑使用专业发件服务（SendGrid / Mailgun / 阿里云邮件推送）替代个人邮箱 SMTP
- 监控后端日志中 `send email verification code failed` 告警频率，持续出现说明需要调低应用侧限流或更换 SMTP 服务

## 部署提示

- 常见 587/STARTTLS 组合：Gmail（需「应用专用密码」）、QQ 邮箱（需「授权码」）、Office365。465 则用 `smtp_encryption=ssl`。
- 配好后先用后台「发送测试邮件」验证连通，再开 `enabled`；要强制时最后再开 `force_bind`，确认普通账号能正常收码后再推广。
- 安全基线（限流、密钥、SSRF 等）见 [安全加固](../guides/security.md)。
