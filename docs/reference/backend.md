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
| `internal/store` | 单一状态文档存储，默认写入 JSON 文件 `db/twilight_go_state.json`，或写入 PostgreSQL 的 `twilight_state` 表。 |
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

配置项使用 TOML 分段（如 `[Global]`、`[API]`、`[Database]`、`[Emby]`、`[Telegram]`、`[SAR]`、`[Email]`、`[RateLimit]`、`[Scheduler]`、`[SystemUpdate]`、`[Security]` 等）。读取时对每个字段都准备了多个候选键（含分段键、历史扁平键和裸键），存在历史命名兼容；例如签到相关项同时识别 `SAR.*` 与历史的 `Signin.*`。

### 关键默认值

| 字段 | 默认值 |
| ---- | ---- |
| `API.host` / `API.port` | `0.0.0.0` / `5000` |
| `Database.driver` | `postgres` |
| `Database` PostgreSQL 默认 | host `127.0.0.1`、port `5432`、user `twilight`、database `twilight`、sslmode `prefer`、连接池 open=8 / idle=4 |
| 状态文件 | `StateFile` 为空时回退到 `<databases_dir>/twilight_go_state.json`（`databases_dir` 默认 `db`） |
| 备份目录 | `DatabaseBackupDir` 为空时回退到 `<databases_dir>/backups` |
| 上传目录 | `uploads`，单文件上限 5 MiB |
| 日志 | `log_level=info`、`runtime_log_limit=1000`、`runtime_memory_limit_mb=128` |
| CORS | 空列表时反射合法 `http`/`https` Origin；显式列表时只允许列表内 Origin |
| 会话 Cookie | 名 `twilight_session`、`Secure=true`、`SameSite=lax`、TTL 7 天 |
| 注册 | `register_mode`、`emby_direct_register_enabled`、`allow_pending_register` 均默认 `false`（secure-by-default） |
| 限流 | 默认开启，全局 1200/分钟、登录 60/分钟等 |
| 调度 | 默认开启，过期检查 `03:00`、到期提醒 `09:00`、每日统计 `00:05` |

> `Database.driver` 默认就是 `postgres`，因此空配置启动会尝试连接 PostgreSQL；若要使用 JSON 文件存储，需显式设置 `driver = "json"`（或留空时按 JSON 处理，详见下文）。

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
| `TWILIGHT_DATABASE_DRIVER` | 状态后端：`json`（或空/`file`）或 `postgres`（或 `postgresql`）。 |
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
| `TWILIGHT_SIGNIN_*` | 签到相关（开关、货币名、每日积分、连签奖励、积分续期开关/消耗/天数等）。 |
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

更多按业务划分的配置项见各功能文档：[注册码与卡码](../features/regcodes.md)、[邮箱验证与找回密码](../features/email.md)、[邀请树](../features/invite.md)、[求片系统](../features/media-requests.md)、[Telegram Bot 命令](../features/telegram-bot.md)、[Bangumi 同步](../features/bangumi.md)、[背景与头像](../features/background.md)。

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

后端把**全部持久业务状态保存在单一状态文档**里（`internal/store/store.go` 的 `State` 结构）。该文档包含用户、API Key、求片、公告、邀请码、邀请关系（`invite_relations`）、注册码、签到、调度记录、设备、登录日志、IP 黑名单、播放记录、改绑申请、Telegram 花名册、违规日志、Bangumi 同步日志与 Bangumi 收藏缓存等实体——它们都是同一份 `State` 里的字段（map / slice），而**不是各自独立的数据库或表**。Telegram 注册/绑定码是当前 App 进程内存中的临时票据，旧 `bind_codes` 字段只作为历史状态字段保留，运行期不再用于新绑定码持久化。

Bangumi 收藏缓存采用两层结构：`BangumiSubjectCache` 以 Bangumi `subject_id`（BGMID）全局缓存作品详情，`BangumiCollectionCache` 以 `uid:type` 缓存用户收藏索引与进度/评分等用户态字段；读取时再按 `subject_id` 回填作品详情。这个设计避免不同用户收藏同一作品时在 state 中重复保存封面、标签和评分等大对象。

> 旧文档把邀请、公告等描述为「新增 `db/invites.db` / `announcements.db`」「`invite_relations` 单表」「`ALTER TABLE announcements 增列`」「自动建表」等，均为过时说法。当前实现中这些都是单一状态文档（`internal/store`）里的字段。

支持两种运行后端（`openStore` / `OpenPostgres`）：

| 后端 | 说明 |
| ---- | ---- |
| JSON（`driver = "json"`、空值或 `file`） | 整份 `State` 序列化为 `db/twilight_go_state.json`（可由 `Database.state_file` / `TWILIGHT_STATE_FILE` 指定）。 |
| PostgreSQL（`driver = "postgres"` / `postgresql`） | 整份 `State` 作为 jsonb 存进 `twilight_state` 表 `id = 1` 的单行；运行日志另存独立表 `twilight_runtime_logs`，会话另存独立表 `twilight_sessions`。 |

数据库行为要点：

- **JSON 后端**写盘走原子写 + fsync（tmp → fsync → rename → fsync 父目录），写前把上一份复制为 `.bak`；启动 / 刷新解析失败时回退到 `.bak`。文件权限收敛到 `0o600`。
- **JSON 后端为单进程独占**：启动时对 `state.json` 加进程级文件锁（`*.lock`），第二个进程会拿到 busy 错误并启动失败。多进程 / 多实例部署应使用 PostgreSQL。
- **PostgreSQL 后端**：目标库不存在时，会尝试用同一连接用户连接 `postgres` / `template1` 维护库执行 `CREATE DATABASE`（连接用户需要 `CREATEDB` 权限，已存在则不重复创建）；随后自动建表 `twilight_state`、`twilight_runtime_logs`、`twilight_sessions` 及相关索引（`CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`，幂等）。
- **运行日志表性能口径**：`twilight_runtime_logs` 只服务 Go 进程运行日志，按 `id` 游标读取增量，按 `id DESC` 读取最新快照，并用 cutoff id 删除旧行以保留最近 N 条；主业务状态仍保持 `twilight_state` 单行 jsonb，不因性能优化拆出业务表。
- DSN 优先级：`Database.url` / `TWILIGHT_DATABASE_URL` / `TWILIGHT_POSTGRES_DSN` 最高，否则由 host/port/user/password/database/sslmode 拼出 DSN。

### Web 端数据库运维

管理端提供数据库状态、备份、恢复、迁移（`internal/api/database_admin.go`）：

- 备份生成时点完整的 `State` 快照（PostgreSQL 后端会把独立表 `twilight_runtime_logs` 一并读回快照）。
- 恢复和迁移都必须先走预览：缺少确认短语时仅返回 `dry_run=true` 的预览结果，不写入数据。恢复确认短语 `RESTORE_DATABASE`，迁移确认短语 `MIGRATE_DATABASE`。
- 恢复和迁移执行前都会自动创建保护性备份，响应中返回 `pre_operation_backup` 便于回滚审计。
- 迁移面板默认关闭，需显式开启 `Database.migration_panel_enabled`（或 `TWILIGHT_DATABASE_MIGRATION_PANEL_ENABLED`）。
- **迁移源 / 目标只支持 `postgres` 与 `json` 两种 driver**。SQLite 作为数据源已被禁用（请求 `source = sqlite/legacy_sqlite` 会返回 403 `DB_SQLITE_DISABLED`）。迁移预检不会写入业务快照，仅返回源/目标 driver、实体计数、快照大小、目标连通性与重启/配置告警；PostgreSQL 目标会准备好库与表。
- 备份恢复只接受备份目录内的普通 `.json` 文件，拒绝绝对路径、`..`、子目录跳转与符号链接（`ResolveBackupPath`）。

> 旧文档关于「数据库迁移页检测 `db/*.db`、按固定文件名读取 `users.db`/`api_keys.db`/`regcode.db`/… 旧 SQLite 文件并迁移」的整段流程，在当前代码中已不存在；SQLite 迁移源已被禁用。

## 运行状态与实时日志

管理端的运行状态页对应后端 `/api/v1/system/admin/runtime/status`、`/runtime/logs`、`/runtime/logs/stream`（`internal/api/runtime_logs.go`）：

- 实时日志只接入 Go 进程内 `zap` 全局 logger（通过自定义 core 路由），不开放任意日志文件、journald 或路径参数读取。
- 日志等级、保留行数与 Go 运行时内存目标由 `Global.log_level`、`Global.runtime_log_limit`、`Global.runtime_memory_limit_mb` 控制；日志保留行数会被夹在 100–50000，默认 1000；内存目标默认 128 MiB，设为 0 表示不限制。
- 日志后端跟随状态存储：JSON 后端日志保存在 `State.RuntimeLogs`；PostgreSQL 后端落在独立表 `twilight_runtime_logs`。在状态接入前的早期日志会先缓冲在内存 fallback 缓冲区，接入后回写。`after=0` 返回最近 N 条快照；`after>0` 返回该游标之后的前 N 条，保持升序，避免增量读取跳过积压日志。
- PostgreSQL 后端写入路径只做 INSERT，并按固定节奏异步裁剪；手动裁剪和异步裁剪都按 cutoff id 保留最近 N 条，避免 `NOT IN + ORDER BY LIMIT` 形式造成高写入期反复全表反扫。
- 日志输出会脱敏：通过正则覆盖 `Authorization`、`Cookie`、`session id/token`、Emby/MediaBrowser token、`access/refresh/id token`、`client_secret`、`private_key`、`connection_string`、`database_url`、`token`、`secret`、`password`、`api_key`、`bot_token`、`dsn`、`Bearer …`、`key-…` 等敏感片段；敏感字段名（含 `key`、`*token`、`*secret` 等）直接替换为 `[REDACTED]`。
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
