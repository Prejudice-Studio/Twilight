# 安全加固

本文用于 Twilight 生产部署前后的安全检查与日常运维基线，并记录已经在 Go 后端落地的安全、逻辑与性能加固点。所有结论均对照实际代码（`internal/api`、`internal/config`、`internal/security`）核对，与代码不符的历史描述已在文中改正。

相关文档：部署见 [安装部署](./install.md)，配置项全表见 [Go 后端架构与配置](../reference/backend.md)，鉴权与错误码见 [后端 API 详参](../reference/backend-api.md)，外部接入见 [API Key 外部接入](../reference/api-key.md)。

## 1. 敏感配置与密钥管理

- 不要把真实密钥写入仓库版本历史。
- 推荐做法：
  - 把通用配置放在 `config.toml`。
  - 把真实密钥放在 `config.local.toml`（同名 `.local` 文件会被自动合并覆盖，且应被 `.gitignore` 忽略；也可通过环境变量 `TWILIGHT_CONFIG_LOCAL_FILE` 指定其他路径）。
  - 或者使用环境变量（`TWILIGHT_*`，见 `internal/config/config.go` 的 `applyEnv`），环境变量优先级最高。
- 如果密钥曾经泄露（提交进 Git 历史、日志、截图），请立即轮换：
  - Telegram Bot Token（`Telegram.bot_token`）
  - Emby API Token / 管理员凭据（`Emby.emby_token` / `emby_username` / `emby_password`）
  - TMDB / Bangumi Token（`Global.tmdb_api_key` / `Global.bangumi_token`）
  - 内部回调密钥 `Security.bot_internal_secret`
  - PostgreSQL 连接凭据（`Database.postgres_password` 等）

## 2. 出站远端 URL 的 SSRF 防护

所有从配置读出后送进 `net/http` 的远端 base URL（Emby / Bangumi / Telegram / TMDB）都必须经过 `internal/api/outbound_url.go` 的 `validateOutboundBaseURL` 校验：

- scheme 仅允许 `http` / `https`，其它（`file:`、`javascript:` 等误粘贴）一律拒绝；
- host 不能为空；
- base URL 不允许携带 query / fragment，避免后续路径拼接被污染；
- 若 host 是字面 IP，交由 `refuseUnsafeOutboundIP` 否决典型 SSRF 目标：链路本地（`169.254.0.0/16`、IPv6 `fe80::/10`）、未指定地址（`0.0.0.0` / `::`）、云元数据 magic IP（如 `100.100.100.200`）。
- 显式放行 loopback（`127.0.0.1` / `::1`）：自托管同机 docker-compose、反代回环部署普遍依赖该路径；不强制 HTTPS（HTTPS 由部署侧反向代理承担）。

配置面被入侵或管理员误填时，这一层避免可信的 `X-Emby-Token` / Bot Token / TMDB Key 被发往攻击者控制的内部地址。

## 3. CORS 与会话安全

### 3.1 CORS

CORS 由 `internal/api/app.go` 的 `applyCORS` 处理，启动 / 热重载时由 `validateCORSOriginsStartup` 体检。

- 生产环境不要使用 `cors_origins = ["*"]`。`applyCORS` 默认附带 `Access-Control-Allow-Credentials: true`，而浏览器规范禁止 `*` 与 credentials 同用；运行期会跳过 `*` 条目，启动期会打 `Error` 日志告警，错配置等于静默无效。
- `cors_origins` 只填 Origin（`scheme://host[:port]`），不带路径、查询串或片段。尾斜杠会被 `normalizeCORSOrigin` 规范化；带 `path`/`query`/`fragment` 或非 `http`/`https` 的条目会被判为无效并忽略。
- 只允许你的前端域名：

```toml
[API]
cors_origins = ["https://app.example.com"]
allow_credential = true
```

- `allow_credential = true` 同时放行 `localhost` / `127.0.0.1` / `[::1]` 在生产环境是高危（任何人在本机起 dev 前端即可携带 cookie 跨站访问），启动期会打 `Error` 提示，请仅在 dev profile 使用。
- 允许的请求头列表为 `Content-Type, Authorization, X-API-Key, X-Twilight-Client, X-Twilight-Intent`。`X-Twilight-Client` 不参与鉴权；`X-Twilight-Intent` 仅用于少数有副作用 GET 的显式意图校验。

### 3.2 会话与 Cookie

- 若通过 HTTPS 对外提供服务（生产基线），保持：
  - `session_cookie_secure = true`（默认即 `true`；纯 HTTP 调试才显式关闭，否则会话明文走线）；
  - 合理的 `session_cookie_samesite`（默认 `lax`，可选 `strict` / `none`）。
- session cookie（默认名 `twilight_session`）为 `HttpOnly`。`session_cookie_domain` 单 origin 部署留空；双子域部署（webui 与 API 不同子域）需设为 `.example.com` 让两子域共享 cookie。
- WebUI 登录态以后端 `/users/me` 响应为准；跨 origin API 场景需确保浏览器请求能携带有效 cookie 或使用其它受支持鉴权方式。
- 会话 TTL 由 `session_cookie_ttl`（默认 7 天）控制；登出会清除 session cookie。

### 3.3 Cookie 写请求

后端不做 CSRF 令牌校验，也不做额外来源校验；Cookie 鉴权写请求只要求有效的 `HttpOnly` session cookie。Bearer Token / API Key 仍按各自鉴权路径处理。

## 4. 鉴权级别与统一响应

后端按路由声明鉴权级别（`internal/api/routes.go`、`internal/api/app.go`）：

| 级别 | 含义 |
| --- | --- |
| `AuthPublic` | 免登录 |
| `AuthUser` | 登录会话（cookie）或 `Authorization: Bearer <session token>` |
| `AuthAdmin` | 登录且 `Role == RoleAdmin` |
| `AuthAPIKey` | `X-API-Key` 头、`Authorization: ApiKey/Bearer <key>`，或 `?apikey=`（仅当该 Key 允许 query 传参） |

所有响应使用统一 envelope：`{ success, code, error_code, message, data, timestamp }`（`error_code` 为协议层错误码，便于前端按码而非文案分支；`data` 为空时省略）。

被禁用 / 到期账号即便持有有效 session 也会被拒：`authenticate` 在 `!Active` 时区分「到期」（`ACCOUNT_EXPIRED`）与「被禁用」（`ACCOUNT_DISABLED`），让前端把「续费」与「申诉」两条引导分开。

## 5. 密码哈希

`internal/security/password.go`：

- 当前算法为 PBKDF2-HMAC-SHA256，默认迭代数 `600000`（对齐 OWASP 2024 下限）；哈希格式 `salt$iterations$hex(dk)`。
- 兼容旧哈希：旧 Python 时代的 `salt$sha256(salt+password)` 两段格式仍可校验；`VerifyPassword` 接受迭代数在 `10000 ~ 1000000` 范围内的存量哈希。
- 透明迁移：登录校验通过后调用 `NeedsRehash`，对 legacy 格式或迭代数低于门槛的哈希在登录路径无感重哈希，无需强制用户重置密码。
- 所有比对走 `subtle.ConstantTimeCompare`，避免时序侧信道。

## 6. API 速率限制

限流由 `internal/api/ratelimit.go` 实现（滑动窗口计数），总开关 `RateLimit.enabled`（默认开）。命中后返回 `HTTP 429` + 对应错误码（如 `AUTH_LOGIN_RATE_LIMITED`、`REGISTER_RATE_LIMITED`、`RATE_LIMITED`），并写 `logger.warning`。

> 纠正：旧文档称 429 响应体携带 `retry_after` 字段。当前 Go 实现的限流器仅返回是否放行，429 响应走标准 envelope，**不附带 `retry_after`**。

主要限流维度（配置项见 `[RateLimit]`，默认值见 `internal/config/config.go`）：

| 维度 | 配置项 | 默认 | 说明 |
| --- | --- | --- | --- |
| 全局（每 IP） | `global_per_minute` | 1200 | 所有请求进入路由前先过这道闸 |
| 登录（每 IP） | `login_per_minute` | 60 | 登录 / API Key 登录 |
| 登录（每账号 5 分钟） | `login_user_per_5m` | 10 | 桶判定在「用户名是否存在」之前消耗，避免账号枚举时序差 |
| 注册（每 IP 10 分钟） | `register_per_10m` | 30 | |
| 找回密码（每 IP 10 分钟） | `forgot_password_ip_per_10m` | 20 | |
| 找回密码（每账号 30 分钟） | `forgot_password_user_per_30m` | 10 | |
| 上传（每用户每分钟） | `upload_per_minute` | 60 | 头像 / 背景上传 |
| 管理员服务器图标（每用户每分钟） | `admin_icon_per_minute` | 20 | |
| API Key 默认配额（每分钟） | `api_key_default_per_minute` | 300 | 单 Key 可在管理端覆盖 |

注册队列状态查询另保留 request 与 IP 双维度限流。

## 7. 多进程与限流一致性

- 配置 `Global.redis_url` 后，会话状态与限流计数共享到 Redis；未配置 Redis 时降级为单进程内存计数与内存会话。
- 多副本 / 多进程部署务必配置 Redis，否则每副本各自维护一份内存桶，限流上限实际被放大 N 倍，会话也只在当前进程生效。
- Redis 命令失败时限流器会自动回退到内存桶并累加 `fallbackCount`（可经 `/system/stats` 观察）；该值持续增长意味着 Redis 失联、限流已退化。
- Emby 注册队列在多进程部署下仍是单进程内队列；强一致的名额控制应依赖数据库与业务锁兜底，不要依赖该队列。

## 8. 反向代理与暴露面

- 用 Nginx / Caddy 暴露单一入口，仅开放 80/443；后端服务端口尽量仅监听内网或本机；限制管理接口访问来源（网段 / IP / WAF）。
- 后端对所有响应附带安全响应头（`applySecurityHeaders`）：`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: strict-origin-when-cross-origin`、`Permissions-Policy`、`X-Permitted-Cross-Domain-Policies: none`、`Cross-Origin-Opener-Policy`、`Cross-Origin-Resource-Policy`，以及一条收紧的 `Content-Security-Policy`（`default-src 'none'`，后端只吐 JSON / 静态上传资源）。前端 Next.js 的 CSP 由 webui 自身负责。
- 反向代理若覆盖这些头，应保持同等或更严格策略。
- 信任代理头需谨慎：仅当 `API.trust_proxy_headers = true` **且** 直接上游落在 `API.trusted_proxy_cidrs` 列表内时，`clientIP` 才消费 `CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For`；否则一律用 TCP 对端地址（fail-closed）。`trusted_proxy_cidrs` 为空时即便 `trust_proxy_headers = true` 也不会消费任何代理头，启动期会打 `Error` 提示。`X-Forwarded-For` 按从右向左逐跳验证，避免客户端伪造最左端 IP 绕过 IP 限流 / 黑名单。

## 9. 前端资源、背景图与头像

上传与背景的安全约束集中在 `internal/api/upload_handlers.go`：

- `uploads` 目录不再作为静态目录直接暴露；用户头像和背景统一通过 `/api/v1/users/assets/{avatar|background}/{filename}` 读取，该接口要求登录，并校验 `kind` 白名单、文件名模式（随机 16 hex + 已知图片扩展名）与路径不越界（`ResolveWithinRoot`）。历史 `/uploads/...` 引用在读取时会被改写为受控 API URL。
- 上传链路：限流 → multipart 解析 → 用 `http.DetectContentType` 嗅探真实 MIME（仅放行 jpg/png/gif/webp/bmp）→ 随机文件名 → `ResolveWithinRoot` 防穿越 + `O_NOFOLLOW` 拒绝符号链接 TOCTOU → `WriteFileAtomicSync`（tmp + fsync + rename）写盘，权限 `0o600`，目录 `0o700`。
- 背景配置（`sanitizedBackgroundConfig`）：
  - CSS 背景只允许渐变函数（`linear-gradient` / `radial-gradient` / `conic-gradient` 及其 `repeating-` 变体），并拒绝含 `url(`、`@`、`;`、`{}`、`<>`、换行的值，避免 `paint()`、`element()`、`image-set()`、`url()`、`@import` 扩大攻击面或做 XSS。
  - 背景图片只接受本系统上传的 `/api/v1/users/assets/background/{filename}` 资源，且文件名必须匹配白名单；不保存任意外部 URL。
- 前端侧也会丢弃不安全的 URL scheme（`javascript:`、非图片 `data:`、跨域绝对地址）。如允许外部图片，请优先 HTTPS，避免混合内容与第三方 Referer 泄漏。
- 新增定时任务 `cleanup_unused_uploads`：清理未被任何用户头像 / 背景 / 服务器图标引用的上传文件，对新文件保留 24 小时宽限期。
- Next.js 已关闭 `X-Powered-By` 指纹头；标准 Node / 1Panel 部署应禁用 Next 图片优化器，避免服务端代拉任意远程图片 URL。若通过 CDN / 反代暴露前端，请同步隐藏上游技术栈指纹。
- 服务器图标不再以本地路径形式作为公开配置面暴露：管理员通过上传接口写入受控资源，公开信息端点固定返回内置图标。

## 10. 日志与审计

- 后端日志统一经 `redactSensitiveText`（`internal/api/runtime_logs.go`）脱敏：`Bearer` 令牌、`key-` 前缀 API Key、以及 `password` / `token` / `secret` 等敏感 key=value（含带引号变体）会被替换为 `[REDACTED]`。
- 该脱敏在 5xx envelope、panic 日志、配置热重载日志、Git 自动更新输出（含 stderr 里可能出现的 `https://user:PAT@host`）等路径上兜底。
- 不要在自建日志里打印 Token、密码、密钥原文或完整 `Authorization` 头。
- 建议保留并审计：管理员关键操作日志、登录失败与封禁日志、API Key 调用轨迹。

## 11. 最小权限原则

- API Key 仅授予必要 scope；`?apikey=` query 传参仅对显式允许 query 的 Key 生效（默认不允许）。
- API Key 不能自行修改权限；权限变更必须通过已登录 Web 端完成，避免只读 Key 自提权。
- 管理员账号数量最小化，长期不用的高权限账号及时停用。
- Telegram 管理员 ID 仅配置必要人员。
- Telegram 管理员私聊仅保留只读查询与统计能力；添加用户、生成注册码、广播、强制绑定、踢出会话等写操作统一走 Web 后台。

## 12. Telegram 相关安全

- 启用 Bot 内部回调时，必须配置强随机的 `Security.bot_internal_secret`。内部绑定确认端点 `POST /api/v1/users/me/telegram/bind-confirm`（`internal/api/telegram_bind_secure.go`）虽然挂在免登录路由上，但要求请求携带 `X-Internal-Secret`（或 `Authorization: Bearer <secret>`）并与配置值做常量时间比对；未配置密钥时该端点直接拒绝。
- 开启群组 / 频道强制校验时，确保 Bot 在目标群有足够权限，避免误判。
- **退群完全封禁模式（`Telegram.ban_on_leave`）**：
  - 默认 `false`；开启后定时巡检发现退群用户会被 Bot 永久封禁（不会自动解封），无法重新加入。
  - 依赖 Bot 在每个 `group_id` 群里是管理员且具备「封禁成员」权限。
  - 开启时巡检的「重新入群识别」分支会被跳过；如需放行某个被永封 ID，需手动在 TG 群里解封。
  - 误判不可逆，上线前请先以 `require_group_membership = true` + `ban_on_leave = false` 观察 1～2 周巡检日志。

## 13. Emby 管理员账号安全隔离

集中在 `internal/api/emby.go` 的 `blockRestrictedEmbyAdmin`（在请求分发层统一拦截，无需逐 handler 校验）：

- **核心规则**：若某用户绑定的 Emby 账号在 Emby 端具有管理员权限（`IsAdministrator`），但该用户在 Twilight 中不是管理员（`Role != RoleAdmin`），则其请求被**默认拒绝**，仅放行一小撮只读 / 登出端点：`GET /api/v1/auth/me`、`GET /api/v1/users/me`、`POST /api/v1/auth/logout`、`POST /api/v1/auth/logout/all`。
- 被拒的操作返回 `HTTP 403` + `EMBY_ADMIN_RESTRICTED`，并记一条 `Warn` 日志。
- 其余被禁止的能力包括但不限于：修改系统 / Emby 密码、改个人资料、解绑 Emby 等。
- 绑定时非系统管理员不允许绑定 Emby 管理员账号；已存在此类绑定的用户需联系系统管理员处理。
- 系统管理员（`Role == RoleAdmin`）不受此限制。
- 该机制防止用户通过绑定 Emby 管理员账号、借密码修改等功能间接控制 Emby 服务器。

## 14. 管理员保护

- 系统禁止移除最后一个管理员的权限：降级管理员前会检查剩余活跃管理员数量，至少保留一个，否则返回 `HTTP 409` + `ADMIN_LAST_ADMIN_PROTECTED`（`store.ErrLastAdmin` 在 `statusFromError` 集中映射）。
- 管理员不能撤销自己的管理员权限。
- 管理员不能通过旧接口修改其他管理员的 Telegram 绑定。

## 15. Emby 用户上限口径

`Register.emby_user_limit`（默认 `-1` 不限制）使用统一容量口径：

- 占用名额的来源：已绑定 Emby 的系统用户、`PendingEmby` 待开通资格、自由注册 / 卡码队列中正在创建的请求。
- 注册码注册、邀请码开通、用户自助补建、手动绑定、管理员授予开通资格、独立 Emby 账号创建等路径都应在创建或新增绑定前检查容量。
- 删除 Emby 账号或清理待开通资格后释放对应名额；独立 Emby 账号不写入本地用户表，因此还会额外读取 Emby 端总用户数做兜底。

> 注意：邀请关系、注册码、公告等业务状态以**字段形式存在于单一状态文档**（`internal/store`，即 JSON 文件 `db/twilight_go_state.json` 或 PostgreSQL `twilight_state` 表的 jsonb 行），不存在独立的 `invites.db` / `invite_relations` 单表 / `ALTER TABLE` 这类旧描述。

## 16. 配置文件自动备份

由 `internal/api/config_admin.go` 实现：

- 在任何「保存配置 / 恢复配置」写回 `config.toml` 之前，后端会先把当前配置内容备份到数据库备份目录（`Database.backup_dir`，默认 `db/backups`），文件名形如 `<base>_<YYYYMMDD_HHMMSS>_<nanos>.toml`，权限 `0o600`，目录 `0o700`。
- 备份列表 / 查看 / 删除走管理员接口，并通过 `ResolveLeafFile` 限制只能落在备份目录直下、拒绝符号链接与路径穿越。
- 备份目录应加入 `.gitignore`，不要提交。

> 纠正：旧文档描述的 Python 时代 `sweep_config_toml` 自动整理、`config_backups/<file>.<timestamp>.<reason>.bak` 命名、单文件 `config.toml.backup`、环境变量 `TWILIGHT_CONFIG_BACKUP_RETENTION` 保留份数裁剪等机制，在当前 Go 后端代码中不存在，已据实改写为上述行为。

## 17. 数据库与自动更新安全

数据库管理（`internal/api/database_admin.go`）：

- 备份、恢复、迁移接口均要求管理员登录；备份 / 恢复目标通过 `store.ResolveBackupPath` 限制在配置的备份目录内，拒绝 `../` 路径穿越。
- 恢复与迁移前会自动创建保护性备份（如「数据库恢复前保护性备份」「数据库迁移前保护性备份」）。
- 迁移到 PostgreSQL 前先用管理端预检，确认目标连接成功、快照与实体计数符合预期。
- 切换 `Database.driver` 后需重启后端；仅迁移数据不会让当前进程自动切换已打开的 store（热重载会在 driver / 路径 / DSN 变化时重开 store，但监听地址变化需重启）。
- 单一状态文档之外另有独立表 `twilight_sessions`、`twilight_runtime_logs`。

Git 自动更新（`internal/api/system_update.go`）：

- 仅允许完整的 HTTPS 仓库 URL；拒绝非 https scheme、空路径、URL 内携带凭据（userinfo）、以及带 query / fragment 的 URL。分支名经白名单正则校验。
- 使用 `git pull --ff-only`，不做 rebase / merge / reset。
- 先执行 dry-run 预检（报告 worktree 是否 dirty）。正式更新时若 worktree 有本地改动，会先 `git stash push --include-untracked` 暂存，拉取后再 `git stash pop` 恢复；恢复出现冲突时会在响应里报告 `stash_conflicts`。
- 自动更新命令输出与 stderr 经脱敏后才记日志，避免泄漏 `https://user:PAT@host` 形式的凭据。

> 纠正：旧文档称「自动更新默认拒绝 dirty worktree」。当前实现并不拒绝，而是**先 stash 本地改动、拉取、再尝试恢复**。需要长期保留本地补丁时仍建议先提交或合并，避免依赖自动 stash/restore。

## 18. 上线前检查清单

- [ ] 所有默认 / 示例密钥已替换
- [ ] `config.local.toml` 与 `.env` 未入库
- [ ] CORS 为明确域名列表，非通配符
- [ ] HTTPS 与安全 Cookie（`session_cookie_secure` / 合理 `samesite`）已启用
- [ ] 双子域部署已正确设置 `session_cookie_domain`，前端和 API 能共享 session cookie
- [ ] `bot_internal_secret` 已配置并验证（若启用内部回调）
- [ ] 多副本部署已配置 Redis（会话 + 限流共享）
- [ ] `trust_proxy_headers` 与 `trusted_proxy_cidrs` 配置一致（启用代理头时 CIDR 不为空）
- [ ] 公开端点的速率限制阈值已按预期流量评估
- [ ] `Telegram.ban_on_leave` 已评估：开启前确认 Bot 在每个群有封禁权限，并备好「误封解除」运维流程
- [ ] 数据库备份 / 恢复 / 迁移预检已在测试环境跑过
- [ ] Git 自动更新预检显示行为可接受（worktree 状态、stash 策略），且仓库 URL 不含凭据
- [ ] 关键日志可追溯但不泄密（依赖统一脱敏，自建日志不要打印明文凭据）
