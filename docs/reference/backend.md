# Go 后端架构与配置

本文介绍 Twilight Go 后端的目录结构、启动方式、配置解析规则、环境变量、状态存储模型以及运行运维相关能力，供部署和二次开发参考。后端入口为 `cmd/twilight`，按 Linux + systemd 部署设计，前端调用路径统一为 `/api/v1/*`。

## Emby 活动日志与播放记录

Emby 活动日志同步 `/System/ActivityLog/Entries` 后，会按 `playback.start` / `playback.stop` 配对并幂等写入 `store.PlaybackRecords`，供 Bangumi 同步等内部流程复用。播放统计页面、统计 API 与导出入口已移除；在线人数只统计 `/Sessions` 中带 `NowPlayingItem` 的正在播放会话。

## 目录结构

| 路径 | 说明 |
| ---- | ---- |
| `cmd/twilight` | Go 后端 CLI 入口，支持 `api`、`all`、`scheduler`、`bot`、`version` 子命令。 |
| `internal/api` | HTTP 路由、统一响应 envelope、鉴权、CORS、限流、上传、安全头，以及按业务拆分的 handler / client / service。 |
| `internal/config` | 读取运行目录 `config.toml`、同目录 `config.local.toml` 与 `TWILIGHT_*` 字段级环境变量；运行入口固定使用当前目录 `config.toml`。 |
| `internal/store` | PostgreSQL 存储：主要业务 `State` 作为 JSONB 存进 `twilight_state`，审计、运行日志、会话与播放记录使用独立表；`Store` 仅能经 `store.OpenPostgres` 构造。JSON 仅作为迁移面板的一次性导出目标与 `migrate-json` 导入源保留，已无 JSON 文件运行后端、无文件锁、无 `.bak`/旁路文件。 |
| `internal/redis` | 无第三方依赖的 Redis RESP 客户端，用于会话和限流跨进程共享。 |
| `internal/security` | Token 生成、PBKDF2-SHA256 密码哈希与旧 SHA256 密码兼容校验。 |

`internal/api` 已按维护边界拆分。常见文件包括：`emby_client.go`、`emby_inventory.go`、`emby_url_probe.go` 负责 Emby；`tmdb_client.go`、`bangumi_client.go`、`bangumi_webhook.go` 负责外部媒体源；`media_service.go` 负责搜索/详情聚合；`media_request_handlers.go` 负责求片 HTTP；`code_use_handlers.go`、`regcode_handlers.go`、`invite_handlers.go` 负责卡码和邀请；`scheduler_handlers.go`、`scheduler_runner.go` 负责调度；`database_admin.go`、`system_update.go`、`runtime_logs.go` 负责数据库运维、Git 更新、运行状态与实时日志。

相关文档：路由清单见 [API 路由索引](./api-index.md)，接口字段见 [后端 API 详参](./backend-api.md)，安全加固见 [安全加固](../guides/security.md)，部署步骤见 [安装部署](../guides/install.md)。

## 启动方式

构建二进制：

```bash
go build -o bin/twilight ./cmd/twilight
```

子命令（`cmd/twilight/main.go`）：

| 命令 | 作用 |
| ---- | ---- |
| `api` | 仅启动 HTTP API 服务。 |
| `all` | 在单进程内同时跑 API、调度器（Scheduler）和 Telegram Bot。 |
| `scheduler` | 仅启动调度器。 |
| `bot` | 仅启动 Telegram Bot；未配置 token / 未开启 `telegram_mode` 时会持续等待配置重载，不会立刻退出。 |
| `version`（`--version` / `-v`） | 打印后端版本号。 |
| `help`（`--help` / `-h`） | 打印用法。 |

直接运行示例：

```bash
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml
```

`api` 与 `all` 支持 `--host`、`--port`、`--config`、`--debug` 标志；`--debug` 会把日志等级强制提升为 `debug`。`--host` / `--port` 非空时覆盖配置文件中的监听地址。所有子命令的 `--config` 都受运行入口约束（见下文「配置解析规则」），只接受当前目录的 `config.toml`。

进程通过 `SIGINT` / `SIGTERM` 触发优雅停机：API server 先 `Shutdown`（10 秒超时），随后等待 scheduler / bot 的 goroutine 全部 drain 完毕再关闭状态存储，避免在 store 关闭后被解引用。`all` 模式下，Bot 因未配置 token 而干净退出（返回 nil）不会被当作致命错误，API 与 scheduler 会继续运行。

systemd 部署对应三个服务单元：`twilight`、`twilight-bot`、`twilight-scheduler`（命名规则 `^twilight(-[a-z0-9]+)?$`，被 Git 更新后的自动重启逻辑复用）。具体部署步骤参见 [安装部署](../guides/install.md)。

## 管理员引导

管理员身份**只能**来自配置文件的 `Admin.uids` / `Admin.usernames` 列表。默认不配置时这两个列表为空，即系统中没有任何管理员，必须由运维显式指定。

> 安全说明：旧版存在「空库引导」通道——状态存储为空时第一个注册的用户无条件成为管理员。该通道已被移除，因为它是一个抢注风险：生产部署后、运维注册前的窗口内，任何访问者抢先 `POST /api/v1/users/register` 即可拿到管理员权限。现在首个注册用户只是普通用户（`RoleNormal`），除非其 UID / 用户名命中配置列表才会在创建后被提升。

首次部署可使用网页初始化向导：先在 `config.toml` 任意结构块临时写入 `setup_mode = true` 或 `SetupMode = true`；`GET /api/v1/setup/status` 在该标记启用、用户数为 0 且当前配置没有 `Admin.uids` / `Admin.usernames` 时返回可用；`POST /api/v1/setup/complete` 需要显式 WebUI intent 头，成功后创建管理员、写入 `[Admin].usernames`、移除 setup 标记、下发会话 Cookie，并因用户与管理员配置已存在而永久关闭。

机制（`internal/api/auth_handlers.go` + `internal/api/configured_admins.go`）：

- 注册成功后，`configuredAdminMatch` 按新用户的实际 UID / 用户名比对配置列表（大小写不敏感）；命中则在创建后提升为 `RoleAdmin` 且 `Active=true`。
- `applyConfiguredAdmins` 在启动和配置热重载时遍历现有用户，把命中 `Admin.uids` / `Admin.usernames` 的账号强制设为 `RoleAdmin` 且 `Active=true`。这是把指定账号提权为管理员的稳定机制。
- `Admin.uids` / `Admin.usernames`、网页初始化 `setup_mode` / `SetupMode` 标记以及 `[SystemUpdate].repo_url` 属于**受保护配置字段**：普通网页配置接口（schema PUT / raw TOML PUT）无法持久改写它们，提交的新值会被剥离、重渲染丢弃或就地还原为磁盘原值。初始化向导有独立的一次性写入路径，只能在显式 setup 标记 + 空系统硬门控下追加首个管理员用户名，并在保存时移除 setup 标记。这避免被盗管理员会话自行增删管理员或把 git 自动更新的来源仓库指向攻击者 fork。

> 注意：旧文档描述的「从旧 Python `db/users.db` 只读导入 active 管理员做引导登录」「检测空 PostgreSQL + 已有 JSON 管理员时临时回退 JSON」「空库注册首用户成为管理员」等流程，当前代码中均已不存在。引导管理员的路径是网页初始化向导或由运维在配置文件指定 `Admin.uids` / `Admin.usernames`。

## 配置解析规则

运行入口固定使用当前工作目录下的 `config.toml`（`cmd/twilight/main.go` 的 `runtimeConfigPath`）：

1. 未传 `--config` 时读取当前工作目录的 `config.toml`。
2. 显式传 `--config` 时，只允许 `config.toml`、`./config.toml`，或指向同一个当前目录文件的绝对路径。
3. 其它文件名、或其它目录下的 `config.toml` 会被直接拒绝并报错，避免 1Panel、systemd 或环境变量把进程带到错误的配置上。

配置加载顺序（`internal/config/config.go` 的 `Load`）：

1. 先加载一组内置默认值（`defaults()`）。
2. 合并主配置文件 `config.toml`。
3. 合并同目录的私密覆盖文件 `config.local.toml`（默认按主配置文件名推导为 `*.local.toml`，可用 `TWILIGHT_CONFIG_LOCAL_FILE` 指定另一路径）。
4. 最后由 `TWILIGHT_*` 环境变量覆盖具体字段（`applyEnv`）。

> 运行入口不会读取 `TWILIGHT_CONFIG_FILE` 指向的其它路径；这保证 1Panel、systemd 与手动启动都默认使用项目目录下的 `config.toml`。需要临时测试其它配置时，请切换到对应测试目录后再启动进程。

配置项使用 TOML 分段（如 `[Global]`、`[API]`、`[Database]`、`[Emby]`、`[Telegram]`、`[SAR]`、`[Email]`、`[RateLimit]`、`[Scheduler]`、`[SystemUpdate]`、`[Security]` 等）。读取时对每个字段都准备了多个候选键（含分段键、历史扁平键和裸键），存在历史命名兼容；例如签到相关项同时识别 `SAR.*` 与历史的 `Signin.*`。自动积分续期的管理员许可为 `SAR.signin_auto_renewal_enabled`，历史分段兼容键为 `Signin.auto_renewal_enabled`，默认关闭。

### 关键默认值

| 字段 | 默认值 |
| ---- | ---- |
| `API.host` / `API.port` | `0.0.0.0` / `5000` |
| `Database.driver` | `postgres` |
| `Database` PostgreSQL 默认 | host `127.0.0.1`、port `5432`、user `twilight`、database `twilight`、sslmode `prefer`、连接池 open=8 / idle=4 |
| 状态文件（仅遗留/导出用途） | `StateFile` 为空时回退到 `<databases_dir>/twilight_go_state.json`（`databases_dir` 默认 `db`）；运行期不再读写此文件，仅作为迁移面板 JSON 导出目标与 `migrate-json` 默认导入源 |
| 备份目录 | `DatabaseBackupDir` 为空时回退到 `<databases_dir>/backups` |
| 上传目录 | `uploads`，单文件上限 5 MiB |
| 日志 | `log_level=info`、`runtime_log_limit=1000`、`runtime_memory_limit_mb=128` |
| CORS | 空列表时反射合法 `http`/`https` Origin；显式列表时只允许列表内 Origin |
| 会话 Cookie | 名 `twilight_session`、`Secure=true`、`SameSite=lax`、TTL 7 天 |
| 注册 | `register_mode`、`emby_direct_register_enabled`、`allow_pending_register` 均默认 `false`（secure-by-default） |
| 限流 | 默认开启，全局 1200/分钟、登录 60/分钟等 |
| 调度 | 默认开启，过期检查 `03:00`、到期提醒 `09:00`、每日统计 `00:05` |

> 运行期后端已收敛为单一 PostgreSQL：`Database.driver` 只接受 `postgres` / `postgresql`（或留空按 `postgres` 处理）。设为其它值（含历史的 `json` / `file`）会在启动时直接报错，不再回退到 JSON 文件存储。历史 JSON 部署用 `twilight migrate-json` 一次性导入（详见下文）。

### CORS 约束

`API.cors_origins` / `TWILIGHT_API_CORS_ORIGINS` 留空时，后端会反射任意合法的 `http`/`https` Origin，便于自托管、反代和双子域部署排障。填写显式列表后，跨 origin 请求只允许列表内 Origin；`*` 等价于兼容模式。Origin 只允许协议 + 主机 + 端口；尾斜杠会被规范化，带路径、查询串或片段的值会被拒绝。若请求 Origin 与当前请求 Host 的协议、主机和端口完全一致，后端会按同源请求放行，无需把 API 自身 Origin 重复加入列表。

旧的隐藏 `cors_allow_any_origin` 开关已移除；请使用空 `cors_origins` 或 `["*"]` 进入兼容模式，或填写可信 Origin 列表进入限制模式。

## 常用环境变量

> **配置优先级与建议**：`config.toml`（参考 `config.production.toml`，后台「系统配置」可视化编辑 + 热重载）是**唯一推荐的配置源**，敏感密钥放同目录 `config.local.toml`。`.env` 仅建议保留后端监听地址、站点名称等极少数部署级项目（见 `.env.example`），**不要**在 `.env` 里堆叠邮箱 / Telegram / 注册 / 限流等功能配置。下表列出的 `TWILIGHT_*` 变量仍可在配置文件之上覆盖对应字段（容器 / CI 等场景备用），但默认部署以 `config.toml` 为准。

所有键以 `TWILIGHT_` 前缀开头，由 `applyEnv` 在配置文件之上覆盖。下表与 `internal/config/config.go` 对齐，仅列出常用项。

### 服务与日志

| 变量 | 说明 |
| ---- | ---- |
| `TWILIGHT_API_HOST` | 监听地址。 |
| `TWILIGHT_API_PORT` | 监听端口。 |
| `TWILIGHT_SERVER_NAME` / `TWILIGHT_GLOBAL_SERVER_NAME` | 站点名称。 |
| `TWILIGHT_SERVER_ICON` | 站点图标。 |
| `TWILIGHT_LOG_LEVEL` | 日志等级 `debug` / `info` / `warn` / `error`，兼容旧数字 `10/20/30/40`。 |
| `TWILIGHT_RUNTIME_LOG_LIMIT` | 实时日志保留行数（最终被夹在 100–50000），默认 1000。 |
| `TWILIGHT_RUNTIME_MEMORY_LIMIT_MB` | Go 运行时内存软上限，单位 MiB；默认 128，设为 0 表示不限制。 |
| `TWILIGHT_REDIS_URL` / `TWILIGHT_GLOBAL_REDIS_URL` | Redis URL，例如 `redis://:password@127.0.0.1:6379/0`，支持 `rediss://`。 |
| `TWILIGHT_API_CORS_ORIGINS` | 逗号分隔的可信前端 Origin。 |
| `TWILIGHT_TRUST_PROXY_HEADERS` | 是否信任上游反代头。 |
| `TWILIGHT_TRUSTED_PROXY_CIDRS` | 可信反代 CIDR 列表，仅在 `trust_proxy_headers=true` 时生效。 |
| `TWILIGHT_API_UPLOAD_FOLDER` / `TWILIGHT_API_MAX_UPLOAD_SIZE` | 上传目录与单文件上限（字节）。 |

### 数据库与状态存储

| 变量 | 说明 |
| ---- | ---- |
| `TWILIGHT_DATABASE_DRIVER` | 状态后端：只接受 `postgres` / `postgresql`（或留空按 `postgres` 处理）；其它值启动即报错。 |
| `TWILIGHT_DATABASE_URL` / `TWILIGHT_POSTGRES_DSN` | PostgreSQL 完整 DSN，优先级高于分项配置（二者等价）。 |
| `TWILIGHT_POSTGRES_HOST` / `TWILIGHT_POSTGRES_PORT` | PostgreSQL 主机与端口。 |
| `TWILIGHT_POSTGRES_USER` / `TWILIGHT_POSTGRES_PASSWORD` / `TWILIGHT_POSTGRES_DATABASE` | PostgreSQL 用户、密码、库名。 |
| `TWILIGHT_POSTGRES_SSLMODE` | PostgreSQL SSL 模式（默认 `prefer`）。 |
| `TWILIGHT_POSTGRES_MAX_OPEN_CONNS` / `TWILIGHT_POSTGRES_MAX_IDLE_CONNS` | 连接池大小。 |
| `TWILIGHT_STATE_FILE` | JSON 状态文件路径。 |
| `TWILIGHT_DATABASES_DIR` | 数据库/状态目录（默认 `db`）。 |
| `TWILIGHT_DATABASE_BACKUP_DIR` | 备份目录。 |
| `TWILIGHT_DATABASE_MIGRATION_PANEL_ENABLED` | 是否开启 Web 端数据库迁移面板。 |

### 会话与安全

| 变量 | 说明 |
| ---- | ---- |
| `TWILIGHT_SESSION_COOKIE_NAME` | 会话 Cookie 名称。 |
| `TWILIGHT_SESSION_COOKIE_SECURE` | HTTPS 部署设为 `true`（默认即 `true`）。 |
| `TWILIGHT_SESSION_COOKIE_SAMESITE` | `lax` / `strict` / `none`。 |
| `TWILIGHT_SESSION_COOKIE_DOMAIN` | 跨子域共享会话时填父域，例如 `.example.com`。 |
| `TWILIGHT_SESSION_TTL_SECONDS` | 会话有效期（秒）。 |
| `TWILIGHT_BOT_INTERNAL_SECRET` | Bot 内部回调密钥。 |
| `TWILIGHT_ADMIN_UIDS` / `TWILIGHT_ADMIN_USERNAMES` | 启动时强制提权的管理员 UID / 用户名列表。 |

### 外部集成与业务开关

| 变量 | 说明 |
| ---- | ---- |
| `TWILIGHT_EMBY_TOKEN` | Emby API Token。 |
| `TWILIGHT_TMDB_API_KEY` / `TWILIGHT_BANGUMI_TOKEN` | TMDB / Bangumi 凭据。 |
| `TWILIGHT_TELEGRAM_BOT_TOKEN` / `TWILIGHT_TELEGRAM_ADMIN_ID` | Telegram Bot Token 与管理员 ID 列表。 |
| `TWILIGHT_TELEGRAM_GROUP_ID` / `TWILIGHT_TELEGRAM_CHANNEL_ID` | 群组 / 频道 ID 列表。 |
| `TWILIGHT_TELEGRAM_FORCE_SUBSCRIBE` | 强制订阅（同时联动强制绑群/绑频道）。 |
| `TWILIGHT_TELEGRAM_REQUIRE_GROUP_MEMBERSHIP` / `TWILIGHT_TELEGRAM_FORCE_BIND_GROUP` / `TWILIGHT_TELEGRAM_FORCE_BIND_CHANNEL` / `TWILIGHT_TELEGRAM_BAN_ON_LEAVE` | Telegram 成员资格策略。 |
| `TWILIGHT_TELEGRAM_GROUP_USER_PANEL_TEMPLATE` | `/twguser` 群组用户面板模板；支持 `\n` 表示换行，可使用 `{telegram_username}` / `{telegram_userid}` 等占位符，推荐在 Web 后台配置页填写。 |
| `TWILIGHT_SYSTEM_UPDATE_ENABLED` / `TWILIGHT_SYSTEM_UPDATE_REPO_URL` / `TWILIGHT_SYSTEM_UPDATE_BRANCH` | Git 自动更新开关与目标仓库/分支。 |
| `TWILIGHT_USER_LIMIT` / `TWILIGHT_EMBY_USER_LIMIT` | 系统用户与 Emby 用户上限（`-1` 表示不限）。 |
| `TWILIGHT_REGCODE_FORMAT` / `TWILIGHT_REGCODE_RANDOM_ALGORITHM` | 注册码格式与随机算法。 |
| `TWILIGHT_MEDIA_REQUEST_ENABLED` | 求片开关。 |
| `TWILIGHT_SIGNIN_*` | 签到相关（开关、货币名、每日积分、连签奖励、积分续期开关/自动续期许可/消耗/天数等）；自动续期环境变量为 `TWILIGHT_SIGNIN_AUTO_RENEWAL_ENABLED`。 |
| `TWILIGHT_NOTIFICATION_ENABLED` / `TWILIGHT_NOTIFICATION_EXPIRY_REMIND_DAYS` | 到期提醒。 |
| `TWILIGHT_AUTO_CLEANUP_PENDING_EMBY` / `TWILIGHT_AUTO_CLEANUP_PENDING_EMBY_DAYS` | 待补建 Emby 自动清理。 |
| `TWILIGHT_RATE_LIMIT_*` | 各类限流阈值（全局、登录、注册、找回密码、邮箱发码、上传、管理员图标、API Key 默认）。 |

### 邮箱验证 / SMTP

| 变量 | 说明 |
| ---- | ---- |
| `TWILIGHT_EMAIL_ENABLED` | 邮箱验证子系统总开关。 |
| `TWILIGHT_SMTP_HOST` / `TWILIGHT_SMTP_PORT` | 发信服务器地址与端口（465/587/25）。 |
| `TWILIGHT_SMTP_USERNAME` / `TWILIGHT_SMTP_PASSWORD` | SMTP 登录用户名与密码/授权码。 |
| `TWILIGHT_SMTP_ENCRYPTION` | `ssl` / `starttls` / `none`。 |
| `TWILIGHT_SMTP_FROM_ADDRESS` / `TWILIGHT_SMTP_FROM_NAME` | 发件地址与显示名（留空回落到用户名/站点名）。 |
| `TWILIGHT_SMTP_TIMEOUT_SECONDS` | 单次发信超时秒数。 |
| `TWILIGHT_EMAIL_FORCE_BIND` | 强制绑定邮箱（管理员豁免；SMTP 未配好时自动失效）。 |
| `TWILIGHT_EMAIL_CODE_LENGTH` / `TWILIGHT_EMAIL_CODE_TYPE` | 验证码长度（4-12）与类型（`numeric` / `alphanumeric`）。 |
| `TWILIGHT_EMAIL_CODE_TTL_MINUTES` / `TWILIGHT_EMAIL_RESEND_COOLDOWN_SECONDS` / `TWILIGHT_EMAIL_MAX_ATTEMPTS` | 验证码有效期、重发冷却、尝试上限。 |
| `TWILIGHT_EMAIL_SUBJECT_TEMPLATE` / `TWILIGHT_EMAIL_BODY_TEMPLATE` | 邮件标题 / 正文模板（占位符 `{site}`/`{code}`/`{ttl}`；正文换行用 `\n`）。 |
| `TWILIGHT_EMAIL_VALIDATION_MODE` / `TWILIGHT_EMAIL_WHITELIST` / `TWILIGHT_EMAIL_BLACKLIST` | 邮箱域名校验模式与黑白名单（与注册共用，逗号分隔）。 |
| `TWILIGHT_RATE_LIMIT_EMAIL_CODE_IP_PER_10M` / `TWILIGHT_RATE_LIMIT_EMAIL_CODE_ADDR_PER_10M` / `TWILIGHT_RATE_LIMIT_EMAIL_CODE_UID_PER_10M` | 发码限流（每 IP / 每收件地址 / 每登录账号）。 |

更多按业务划分的配置项见各功能文档：[签到与积分续期](../features/signin.md)、[注册码与卡码](../features/regcodes.md)、[邮箱验证与找回密码](../features/email.md)、[邀请树](../features/invite.md)、[求片系统](../features/media-requests.md)、[工单系统](../features/tickets.md)、[Telegram Bot 命令](../features/telegram-bot.md)、[Bangumi 同步](../features/bangumi.md)、[背景与头像](../features/background.md)。

## Redis 优化

配置了 `Global.redis_url` 或 `TWILIGHT_REDIS_URL` 时启用 Redis（`internal/redis/redis.go`）：

- 会话 Token 与限流计数写入 Redis，支持多进程 / 多实例共享。
- Redis 不可用时自动降级为本地内存实现，并记录 warning。
- 客户端使用连接池、短超时（2 秒），并以 `SETEX`、`EVAL` Lua 脚本原子完成 `INCR + EXPIRE`，避免出现「已 INCR 但无 TTL」导致限流桶永久命中的退化态。
- 对 Redis 回复长度设上限（单条 bulk 最大 64 MB、数组元素最多 256K），防止伪造超大长度造成 OOM。
- 支持 `redis://` 与 `rediss://`（TLS），URL 路径段可选择 DB 号。

## 已复刻的核心业务

Go 后端按统一的业务状态与前端响应形状实现，主要模块：

| 模块 | 已实现范围 |
| ---- | ---- |
| 用户 / 鉴权 | 注册、首个管理员、登录/刷新/登出、Cookie 会话、API Key 与多 Key 管理、密码修改、头像/背景上传。 |
| 求片 | TMDB/Bangumi 搜索与详情入口、库存检查、求片创建（`require_key`）、状态流转、外部密钥更新、用户/管理员列表、所有者/管理员权限校验。 |
| 邀请树 | 邀请码生成/撤销/检查/使用、父子关系、深度计算、管理员森林视图、detach、前端 `nodes`/`edges`/`roots`。 |
| 注册码 / 卡码 | 注册码/续期码/白名单码创建、随机码生成、有效期、使用次数、诱饵码隐藏、预览不消费、消费后更新用户权益。 |
| 公告 | 公告创建/编辑/可见性/置顶/分级/到期。 |
| 管理端 | 用户筛选/分页/排序、启用/禁用/删除级联、续期、强制解绑、Telegram 绑定、待补建 Emby 资格、批量操作、导出。 |
| 安全 | 登录历史、设备信任/阻止、IP 黑名单、可疑登录查询、违规日志、安全响应头、上传 MIME 校验。 |
| 调度 | 任务列表、手动运行记录、last-run/history、调度覆盖/恢复默认。 |
| 外部集成 | Emby、TMDB、Bangumi、Telegram Bot API 均有 HTTP 客户端边界；未配置密钥时安全降级，不阻塞公开系统信息。 |

需要真实第三方服务才能完成的动作（Emby 建号、删除远端用户、踢会话、Telegram 群管理、Bangumi 点格子同步），通过对应客户端调用远端 API，本地状态保持与前端兼容。

## 数据库与状态模型

后端把**主要持久业务状态保存在单一状态文档**里（`internal/store/store.go` 的 `State` 结构）。该文档包含用户、API Key、求片、公告、邀请码、邀请关系（`invite_relations`）、注册码、签到、调度记录、设备、登录日志、IP 黑名单、播放记录、改绑申请、Telegram 花名册、违规日志、Bangumi 同步日志与 Bangumi 收藏缓存等实体——它们都是同一份 `State` 里的字段（map / slice），而**不是各自独立的数据库或表**。高频追加的操作审计和运行日志例外，分别落在 `twilight_audit_logs` 与 `twilight_runtime_logs`。Telegram 注册/绑定码是当前 App 进程内存中的临时票据，旧 `bind_codes` 字段只作为历史状态字段保留，运行期不再用于新绑定码持久化。

Bangumi 收藏缓存采用两层结构：`BangumiSubjectCache` 以 Bangumi `subject_id`（BGMID）全局缓存作品详情，`BangumiCollectionCache` 以 `uid:type` 缓存用户收藏索引与进度/评分等用户态字段；读取时再按 `subject_id` 回填作品详情。这个设计避免不同用户收藏同一作品时在 state 中重复保存封面、标签和评分等大对象。

> 旧文档把邀请、公告等描述为「新增 `db/invites.db` / `announcements.db`」「`invite_relations` 单表」「`ALTER TABLE announcements 增列`」「自动建表」等，均为过时说法。当前实现中这些都是单一状态文档（`internal/store`）里的字段。

运行期只有一种后端（`OpenPostgres`）：

| 后端 | 说明 |
| ---- | ---- |
| PostgreSQL（`driver = "postgres"` / `postgresql` / 空值） | 主要业务 `State` 作为 jsonb 存进 `twilight_state` 表 `id = 1` 的单行；操作审计、运行日志、会话、播放记录、Telegram 更新游标分别使用 `twilight_audit_logs`、`twilight_runtime_logs`、`twilight_sessions`、`twilight_playback_records`、`twilight_telegram_runtime`。`Store` 仅能经 `store.OpenPostgres` 构造，`s.db` 恒非 nil。 |

> JSON 不再是运行后端：`driver` 设为 `json` / `file` 会在 `openStore` 直接报错。JSON 仅在两处以「文件」形式出现——迁移面板的一次性导出目标（把当前状态 dump 成 `state.json`）与 `twilight migrate-json` 的历史导入源。历史上的进程文件锁（`*.lock`）、`.bak` 影子文件、`state.json.runtimelog` 旁路日志文件、`refreshLocked` 变更门控均随 JSON 运行后端一并移除。

数据库行为要点：

- **多进程并发写用版本列串行化**：`twilight_state` 带 `version bigint` 列，`saveLocked` 走「version = 读到值」守卫的 UPSERT，命中 0 行（被他进程抢先递增）即回 `errStateVersionConflict`，`mutateAndSaveLocked` 有界重试（重新 `refreshLocked` 拉最新 state + version 后重放 mutate）。这取代了此前「refresh 与整份 jsonb 盲写之间被他进程插入提交后仍整份覆盖」的丢更新——正是「用户建了工单、TG 也收到通知，工单却随后凭空消失」的根因。`saveLockedForce` 无视守卫强制覆盖并把 version 推到「读到值 +1」，仅用于冷启动播种、`LoadSnapshot` 恢复 / 迁移。
- `refreshLocked` 每次先由 PostgreSQL 比较 `version`，查询使用 `CASE WHEN version = $1 THEN NULL ELSE state END`：版本没变时只返回版本号，不传输、不反序列化整份 JSONB，也不重建用户/API Key 索引；版本变化时才在同一查询中取回完整 state。读写都走 30s 超时 context，连接假死时到期自行释放，避免整个 `s.mu` 连带全站 handler 卡死。任何绕过 Store 的人工 SQL 修复若修改 `state`，必须同时递增 `version`，否则各进程会按未变化版本继续使用本地快照。
- 写盘失败一律回滚：`Store.stateRaw` 常驻保存与 `stateVersion` 对齐的权威序列化字节；`snapshotStateLocked` 只引用这份只读字节，save 成功后才替换为新 JSON，失败则由 `restoreStateLocked` 延迟反序列化并重建索引。常见成功路径不再为了回滚额外序列化或复制完整对象图。
- **PostgreSQL 建库建表**：目标库不存在时，会尝试用同一连接用户连接 `postgres` / `template1` 维护库执行 `CREATE DATABASE`（连接用户需要 `CREATEDB` 权限，已存在则不重复创建）；随后自动建表 `twilight_state`、`twilight_audit_logs`、`twilight_runtime_logs`、`twilight_sessions`、`twilight_playback_records`、`twilight_telegram_runtime` 及相关索引（`CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` / `ALTER TABLE … ADD COLUMN IF NOT EXISTS version`，全部幂等，可反复执行）。
- **操作审计表性能口径**：`twilight_audit_logs` 使用自增 ID 与状态时间、分类、动作、操作者、目标用户索引。新增审计只插入独立表并在同一事务内执行条数保留，不再为一条审计记录全量读取、反序列化和重写 `twilight_state`。列表的筛选、排序、计数、分页由 PostgreSQL 完成，避免把完整审计历史复制进 Go 堆。旧 `state.audit_logs` 在启动时幂等迁移；备份时会重新合并到 JSON 快照，恢复时再拆回独立表。
- **运行日志表性能口径**：`twilight_runtime_logs` 只服务 Go 进程运行日志，按 `id` 游标读取增量，按 `id DESC` 读取最新快照，并用 cutoff id 删除旧行以保留最近 N 条；其写入同样不会改动主状态文档。
- **Telegram 游标性能与一致性口径**：`twilight_telegram_runtime` 只有 `id=1` 的一行，批次完成后以 `GREATEST` 单调推进 `update_offset`，不获取 Store 全局锁，也不刷新、快照、编码或重写 `twilight_state`。Bot 身份经 `getMe` 确认变化时才允许清零。旧 `State.TelegramBotOffset` 在启动或 `LoadSnapshot` 时作为迁移种子，迁移后从 JSONB 清除；新备份不携带该运行游标。
- **播放记录写路径优化**：
  - 所有 `twilight_playback_records` 的 PG 读写都走带超时的 context（`pgPlaybackReadTimeout` / `pgPlaybackWriteTimeout`，均 5s），连接假死时到期自行释放，不再无限阻塞。
  - `AddPlaybackRecordIdempotent` 的 PG 单条 INSERT 移到 `s.mu` 之外执行——记录已由 `saveLocked` 落进 `twilight_state` 的 state jsonb，独立表 `twilight_playback_records` 行只是读路径优先命中的副本、`ON CONFLICT` 保证幂等；持锁跨这段网络 I/O 会让一个卡死的 PG 连接冻结全部 store 写操作。DB 失败按旧语义吞掉（返回 `false, nil`），内存状态副本为准（这是 `internal/store` 里唯一保留内存兜底读的位置）。
  - 新增批量写 `AddPlaybackRecordsIdempotent([]PlaybackRecord)`：整批只做一次 `refreshLocked + saveLocked`，PG 侧走分块多行 INSERT（每条语句 500 行，8 列 × 500 = 4000 占位符，远低于 65535 上限）。Emby 活动同步 `persistEmbyPlaybackRecordsFromActivity`（事件上限 `embyActivityFetchLimit`=20000）改用批量方法——旧代码逐条调用，PG 模式下每条都要全量 state jsonb 序列化 + UPDATE，是灾难级写放大。去重语义与单条完全一致：`(uid, item_id, played_at)` 幂等键，`uid==0` / 空 `item_id` 记录不参与去重（沿用旧行为），批内 + 与现有状态双重去重用一张 `map[playbackKey]` 把逐条 O(N*M) 扫描压到 O(N+M)。
  - PG 连接池补 `SetConnMaxIdleTime(5min)`：低峰期回收空闲连接，规避中间件/PG 端提前掐断留下的半死连接被复用（与既有 `SetConnMaxLifetime(30min)` 硬上限配合）。
- **批量用户写路径优化**：新增 `Store.UpdateUsers([]int64, func(*User) error)`，把「对一组用户各跑一次 `UpdateUser`」压成一次 store 写（单次 `refreshLocked + snapshotStateLocked + saveLocked`），取代逐用户 N 次「整份 state jsonb upsert」。批量续期 `handleBatchRenewUsers`（纯本地写、无远端副作用，上限 200）改用它——旧代码逐用户 `UpdateUser`，会持 `s.mu` 做上百次整份 jsonb 落盘把所有并发读写全卡在锁上。逐用户语义完全对齐单条路径：`fn` 报错 / 身份字段冲突（`userIdentityConflictLocked`）/ 用户不存在都只记进返回的 `map[int64]error` 且不中断整批（该用户改动被丢弃不落盘），只有 `saveLocked` 失败才整批回滚到快照并抛错；重复 uid 只处理一次，无任何用户真正改动时跳过 save。身份索引随每个成功用户增量维护，同批内后来的用户能看到前面用户占用/释放的用户名/邮箱/TG/Emby 标识。注意：涉及逐用户远端调用（Emby 停用、session 清理、TG 封禁）的批量口（`handleBatchToggleEmby` / `handleBatchGrantAllLibraries` / `handleBatchRefreshStatus` / `handleBatchDeleteUsers` 及退群清理任务）**不**收敛——远端 I/O 无法批处理，且现有「单侧远端失败降级记录、不回滚本地」语义必须逐用户保留。
- DSN 优先级：`Database.url` / `TWILIGHT_DATABASE_URL` / `TWILIGHT_POSTGRES_DSN` 最高，否则由 host/port/user/password/database/sslmode 拼出 DSN。

### Web 端数据库运维

管理端提供数据库状态、备份、恢复、迁移（`internal/api/database_admin.go`）：

- 备份生成完整的业务 `State` JSON：`Snapshot` 会把独立表 `twilight_runtime_logs` 与 `twilight_audit_logs` 分别读回 `RuntimeLogs`、`AuditLogs`；恢复 / 迁移时 `LoadSnapshot` 再把两类日志拆回对应独立表（含空快照的清空语义），不会因为性能拆表丢失备份内容。`twilight_telegram_runtime` 是消费确认游标，不随业务状态恢复到旧时点；仅兼容旧快照内的 `telegram_bot_offset`，并以不回退方式迁移。
- 恢复和迁移都必须先走预览：缺少确认短语时仅返回 `dry_run=true` 的预览结果，不写入数据。恢复确认短语 `RESTORE_DATABASE`，迁移确认短语 `MIGRATE_DATABASE`。
- 恢复和迁移执行前都会自动创建保护性备份，响应中返回 `pre_operation_backup` 便于回滚审计。
- 迁移面板默认关闭，需显式开启 `Database.migration_panel_enabled`（或 `TWILIGHT_DATABASE_MIGRATION_PANEL_ENABLED`）。
- **运行后端只有 PostgreSQL 一种**；迁移面板的 `target = json` 不是一个可运行后端，只是「把当前状态一次性 dump 成 `state.json` 文件」的导出目标（对应 `/system/admin/database/status` 里 `role=export` 的 driver），产物用于归档或喂给 `migrate-json` 重新导入。SQLite 作为数据源已被禁用（请求 `source = sqlite/legacy_sqlite` 会返回 403 `DB_SQLITE_DISABLED`）。迁移预检不会写入业务快照，仅返回源/目标 driver、实体计数、快照大小、目标连通性与重启/配置告警；PostgreSQL 目标会准备好库与表。
- 备份恢复只接受备份目录内的普通 `.json` 文件，拒绝绝对路径、`..`、子目录跳转与符号链接（`ResolveBackupPath`）。

> 旧文档关于「数据库迁移页检测 `db/*.db`、按固定文件名读取 `users.db`/`api_keys.db`/`regcode.db`/… 旧 SQLite 文件并迁移」的整段流程，在当前代码中已不存在；SQLite 迁移源已被禁用。

## 运行状态与实时日志

管理端的运行状态页对应后端 `/api/v1/system/admin/runtime/status`、`/runtime/logs`、`/runtime/logs/stream`（`internal/api/runtime_logs.go`）：

- 实时日志只接入 Go 进程内 `zap` 全局 logger（通过自定义 core 路由），不开放任意日志文件、journald 或路径参数读取。
- 日志等级、保留行数与 Go 运行时内存目标由 `Global.log_level`、`Global.runtime_log_limit`、`Global.runtime_memory_limit_mb` 控制；日志保留行数会被夹在 100–50000，默认 1000；内存目标默认 128 MiB，设为 0 表示不限制。
- 日志落在独立表 `twilight_runtime_logs`（与 `twilight_state` 同库不同表），不进 state jsonb。在状态接入前的早期日志会先缓冲在内存 fallback 缓冲区，接入后回写。`after=0` 返回最近 N 条快照；`after>0` 返回该游标之后的前 N 条，保持升序，避免增量读取跳过积压日志。
- PostgreSQL 后端写入路径只做 INSERT，并按固定节奏异步裁剪；手动裁剪和异步裁剪都按 cutoff id 保留最近 N 条，避免 `NOT IN + ORDER BY LIMIT` 形式造成高写入期反复全表反扫。
- 日志输出会脱敏：通过正则覆盖 `Authorization`、`Cookie`、`session id/token`、Emby/MediaBrowser token、`access/refresh/id token`、`client_secret`、`private_key`、`connection_string`、`database_url`、`token`、`secret`、`password`、`api_key`、`bot_token`、`dsn`、`Bearer …`、`key-…` 等敏感片段；敏感字段名（含 `key`、`*token`、`*secret` 等）直接替换为 `[REDACTED]`。脱敏是每条日志每个字符串字段都要跑的热路径：`redactSensitiveText` 前置一层廉价触发词前缀过滤（`mightContainSecret`），文本（小写化后）不含任何敏感触发词时直接零分配原样返回，避免绝大多数普通日志（uid / path / duration 等）白跑四遍正则 + 全量字符串分配。触发词列表是四条正则「命中所必需的字面量子串」的超集，只会让快路径更保守（多跑正则），不会漏脱敏。
- 状态接口只读取 Go runtime 摘要（版本、goroutine、内存、是否启用 Redis、活动数据库后端、用户数等）和 Linux `/proc` 摘要（loadavg / meminfo / uptime），不返回环境变量、配置明文、命令行参数或进程列表。
- 实时日志流为 SSE（`text/event-stream`），按游标增量推送 `snapshot` / `logs` / `ping` 事件，单连接 25 秒空闲返回。

## Git 更新

`POST /api/v1/system/admin/update`（`internal/api/system_update.go`）只接受不带凭据的 HTTPS 仓库 URL 和受限分支名：

- 仓库 URL 必须是完整 https 链接，不得含用户名/密码、查询串或片段。
- 分支名受白名单正则约束（`^[A-Za-z0-9._/-]{1,128}$`），拒绝以 `-`/`/`/`.` 开头、`..`、`//`、`@{` 等。
- 更新前读取当前分支、commit、remote 和 `git status --porcelain`；`dry_run` 只做预检不执行 fetch/pull。
- 工作区有未提交改动时，自动 `git stash push --include-untracked` 暂存，更新后再 `git stash pop` 恢复；恢复失败时返回冲突文件清单。
- 实际更新执行 `git remote set-url`、`git fetch --prune`、`git checkout`、`git pull --ff-only`，不拼 shell 字符串；命令 stdout/stderr 与 remote URL 都经过凭据脱敏，返回 before/after commit 便于审计。
- 仅当 `before.commit != after.commit` 且 `restart_services=true` 时才安排重启：优先 `systemd-run --on-active=2` 延迟重启 `twilight`、`twilight-bot`、`twilight-scheduler`，否则降级到后台 `systemctl`。

## 安全基线

- 所有 JSON 响应使用统一 envelope：`success`、`code`、`message`、`data`、`timestamp`。
- 鉴权级别（`internal/api/routes.go`）：
  - `AuthPublic`：免登录。
  - `AuthUser`：登录会话（Cookie）或 Bearer Token。
  - `AuthAdmin`：登录且 `Role == RoleAdmin`。
  - `AuthAPIKey`：`X-API-Key` 头、`Authorization: ApiKey/Bearer` 或 `?apikey=` 查询参数。
- Cookie 鉴权写请求不要求 CSRF 令牌，也不做额外来源校验；`X-Twilight-Client: webui` 不参与鉴权。
- **客户端 IP 单次解析 + 全链路复用**：`ServeHTTP` 在 reload 之后解析一次客户端 IP 写进 request context，IP 黑名单、全局限速 key、访问日志以及各 handler 里的 `a.clientIP(r)` 全部命中缓存——保证同一请求的所有 IP 安全控制用同一个值（不再各自重复解析、也不会因中途状态变化取到不一致 IP）。受信代理判定改用 reload 期一次性解析好的匹配器（`trustedProxyMatcher`，随运行时快照原子切换），热路径不再每请求 `net.ParseCIDR` / `net.ParseIP`；`trusted_proxy_cidrs` 为空时匹配器恒判不受信（fail-closed），与旧逐条解析语义一致。
- Cookie 会话默认 `HttpOnly`、`Secure=true`、`SameSite=Lax`；可通过 `CookieDomain` 跨子域共享。
- CORS 留空或 `*` 时进入兼容模式并反射合法 Origin；填写列表后只允许可信 Origin。
- API Key 只保存哈希，明文仅创建时返回一次。
- 上传接口只接受白名单内的栅格图片 MIME，写入受控目录；读取只接受服务端生成的文件名格式，并在返回前重新校验绝对路径仍位于上传目录内。
- 数据库备份/状态迁移路径经过目录约束，拒绝路径穿越、绝对路径与符号链接。
- 默认响应安全头：`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: strict-origin-when-cross-origin`、`Permissions-Policy`、`X-Permitted-Cross-Domain-Policies: none`、`Content-Security-Policy`（后端默认 `default-src 'none'`）、`Cross-Origin-Opener-Policy`、`Cross-Origin-Resource-Policy`。

更系统的加固说明见 [安全加固](../guides/security.md)。

## 验证命令

```bash
go build -o bin/twilight ./cmd/twilight
go test ./...
go vet ./...
```

如本机安装了 `govulncheck`：

```bash
govulncheck ./...
```

更多本地开发与构建说明见 [开发指南](../guides/development.md)。
