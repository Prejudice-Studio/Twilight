# Go 后端说明

Twilight 后端已提供 Go 实现，入口为 `cmd/twilight`。项目按 Linux/systemd 部署设计，前端调用路径保持 `/api/v1/*`。

## 目录结构

| 路径 | 说明 |
| ---- | ---- |
| `cmd/twilight` | Go 后端 CLI 入口，支持 `api`、`all`、`scheduler`、`bot` 子命令。 |
| `internal/api` | HTTP 路由、统一响应、认证、限流、上传、安全头，以及按业务拆分的 handler/client/service。 |
| `internal/config` | 读取 `config.toml`、`config.local.toml` 与 `TWILIGHT_*` 环境变量，未指定 `--config` 时优先当前目录 `config.toml`。 |
| `internal/store` | JSON 状态存储，默认写入 `db/twilight_go_state.json`。 |
| `internal/redis` | 无第三方依赖的 Redis RESP 客户端，用于会话和限流共享。 |
| `internal/security` | Token、PBKDF2-SHA256 密码哈希与旧 SHA256 密码兼容校验。 |

`internal/api` 已按维护边界拆分：`emby_client.go`、`emby_library.go`、`emby_inventory.go` 负责 Emby；`tmdb_client.go`、`bangumi_client.go`、`bangumi_webhook.go` 负责外部媒体源；`media_service.go` 负责搜索/详情聚合；`media_request_handlers.go` 负责求片 HTTP；`code_use_handlers.go`、`regcode_handlers.go`、`invite_handlers.go` 负责卡码和邀请；`scheduler_handlers.go`、`scheduler_runner.go` 负责调度；`database_admin.go`、`system_update.go`、`runtime_logs.go` 负责数据库运维、Git 更新、运行状态和实时日志。

## 启动

开发模式：

```bash
bash start_backend.sh dev
```

生产模式建议先构建二进制：

```bash
go build -o bin/twilight ./cmd/twilight
bash start_backend.sh prod
```

systemd 一键安装：

```bash
sudo bash deploy/setup-systemd.sh --dry-run
sudo bash deploy/setup-systemd.sh --restart
```

脚本会检测项目目录、配置文件、二进制、service 用户/组和旧 Python 版 Twilight systemd 残留。

也可以直接运行：

```bash
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml
```

## 首个管理员

JSON 状态为空时，第一个通过 `/api/v1/users/register` 注册的用户会自动成为管理员。后续用户默认为普通用户。

如果你已经有旧 JSON 状态文件，但提前把 `database.driver` 改成了 `postgres`，而 PostgreSQL 目标库还没有迁移数据，后端会在启动时检测到空 PostgreSQL 和已有 JSON 管理员，并临时使用 JSON 状态启动。这样原管理员仍可登录前端，完成数据库迁移预检和二次确认后再重启切换到 PostgreSQL。

如果没有 Go JSON 状态，但存在旧 Python 版 `db/users.db`，Go 后端会在当前状态没有 active 管理员时尝试通过系统 `sqlite3` 命令只读导入旧库里的 active 管理员账号。该流程只用于恢复后台入口，不会在启动时隐式写入全部旧业务数据。

旧 SQLite 全量迁移应在 Web 管理端显式执行：“数据库迁移”页会检测 `db/*.db`、`*.db-wal`、`*.db-shm`，预览阶段按固定文件名读取 `users.db`、`api_keys.db`、`regcode.db`、`invites.db`、`announcements.db`、`bangumi.db`、`signin.db`、`score.db`、`login_log.db`、`playback.db`、`scheduler_run.db`、`scheduler_schedule.db`、`telegram_roster.db`。执行阶段会先备份当前 Go 状态和旧 SQLite 文件集，再把映射后的状态快照写入 JSON 或 PostgreSQL。

## 配置

Go 后端继续读取现有 TOML，主配置文件选择顺序为：

1. 显式传入的 `--config` 路径。
2. 未传 `--config` 时，优先当前工作目录的 `config.toml`。
3. 如果当前目录没有 `config.toml`，再使用 `TWILIGHT_CONFIG_FILE`。
4. 主配置同目录的 `config.local.toml` 会作为私密覆盖文件加载。
5. 最后由 `TWILIGHT_*` 环境变量覆盖具体字段。

常用环境变量：

| 变量 | 说明 |
| ---- | ---- |
| `TWILIGHT_API_HOST` | 监听地址。 |
| `TWILIGHT_API_PORT` | 监听端口。 |
| `TWILIGHT_CONFIG_FILE` | 主配置文件路径。 |
| `TWILIGHT_STATE_FILE` | Go JSON 状态文件路径。 |
| `TWILIGHT_DATABASE_DRIVER` | 数据库后端：`json` 或 `postgres`。 |
| `TWILIGHT_DATABASE_URL` | PostgreSQL 完整 DSN，优先级高于分项配置。 |
| `TWILIGHT_POSTGRES_DSN` | PostgreSQL 完整 DSN 别名，等价于 `TWILIGHT_DATABASE_URL`。 |
| `TWILIGHT_POSTGRES_HOST` / `TWILIGHT_POSTGRES_PORT` | PostgreSQL 主机和端口。 |
| `TWILIGHT_POSTGRES_USER` / `TWILIGHT_POSTGRES_PASSWORD` / `TWILIGHT_POSTGRES_DATABASE` | PostgreSQL 用户、密码和库名。 |
| `TWILIGHT_POSTGRES_MAX_OPEN_CONNS` / `TWILIGHT_POSTGRES_MAX_IDLE_CONNS` | PostgreSQL 连接池大小。 |
| `TWILIGHT_REDIS_URL` | Redis URL，例如 `redis://:password@127.0.0.1:6379/0`。 |
| `TWILIGHT_API_CORS_ORIGINS` | 逗号分隔的前端 Origin。 |
| `TWILIGHT_SESSION_COOKIE_SECURE` | HTTPS 部署时设为 `true`。 |

配置通过 Web 管理端保存时会先写备份，再热重载可在线生效的字段；监听端口、存储 driver 等启动期字段仍需要进程重启。

生产环境必须把 `TWILIGHT_API_CORS_ORIGINS` / `API.cors_origins` 显式设置为前端 HTTPS 域名。Go 后端不会把 `*` 当作凭据接口的可信 Origin，避免跨站页面携带 Cookie 调用管理接口。Origin 只允许协议、主机和端口；配置里的尾斜杠会自动规范化，带路径、查询串或片段的值会被拒绝。

## Redis 优化

如果配置了 `redis_url` 或 `TWILIGHT_REDIS_URL`，Go 后端会启用 Redis：

- 会话 Token 写入 Redis，支持多进程/多实例共享登录态。
- 限流计数写入 Redis，避免多实例下每个进程单独计数。
- Redis 不可用时自动降级为本地内存实现，并在日志中记录 warning。
- Redis 客户端使用连接池、短超时、`SETEX`、`INCR` + `EXPIRE`，避免热路径阻塞。

## 已复刻的核心业务

Go 后端不是空接口骨架，以下模块已经按旧 Python 行为实现本地业务状态和前端响应形状：

| 模块 | 已实现范围 |
| ---- | ---- |
| 用户/认证 | 注册、首个管理员、登录/刷新/登出、Cookie 会话、API Key、多 Key 管理、密码修改、头像/背景上传。 |
| 求片 | TMDB/Bangumi 搜索入口、详情入口、库存检查响应、求片创建、`require_key`、状态流转、外部密钥更新、用户/管理员列表、所有者/管理员权限校验。 |
| 邀请树 | 邀请码生成/撤销/检查/使用、父子关系、深度计算、管理员森林视图、detach、前端需要的 `nodes`/`edges`/`roots`。 |
| 注册码/卡码 | 注册码/续期码/白名单码创建、`hex20`/base32 随机码、有效期、使用次数、诱饵码隐藏、预览不消费、消费后更新用户权益。 |
| 管理端 | 用户筛选/分页/排序、启用/禁用/删除级联、续期、强制解绑、Telegram 绑定、待补建 Emby 资格、批量用户操作、导出。 |
| 安全 | 登录历史、设备信任/阻止、IP 黑名单、可疑登录查询、安全响应头、上传 MIME 校验。 |
| 调度 | 旧 job ID 全量保留、任务列表、手动运行记录、last-run/history、调度覆盖/恢复默认。 |
| 外部集成 | Emby、TMDB、Bangumi、Telegram Bot API 均有 HTTP 客户端边界；未配置密钥时安全降级，不阻塞公开系统信息。 |

需要真实第三方服务才能完成的动作，例如 Emby 建号、删除远端用户、踢会话、Telegram 群管理、Bangumi 点格子同步，会通过对应客户端调用远端 API；本地状态会先保持和前端兼容。

## 数据库与迁移

- 默认 JSON 状态文件：`db/twilight_go_state.json`。
- 现有 Go JSON 状态文件可通过 `[Database] state_file = "/path/to/twilight_go_state.json"` 或 `TWILIGHT_STATE_FILE` 指定。
- 可选 PostgreSQL 后端：配置 `database.driver = "postgres"` 和 DSN，适合用户量、播放记录、调度记录较多的部署。
- 生产模板 `config.production.toml` 已包含 `[Database]` 示例，可使用完整 `url` 或 `postgres_host/postgres_user/postgres_password/postgres_database` 分项配置。
- PostgreSQL 目标数据库不存在时，启动阶段和数据库迁移预检都会尝试用同一连接用户连接 `postgres` / `template1` 维护库并执行 `CREATE DATABASE`；连接用户需要 `CREATEDB` 权限，已有数据库则不会重复创建。
- 管理端提供数据库状态、备份、恢复、迁移预检和执行。
- 恢复和迁移都必须先走预览；后端在缺少确认短语时只返回 `dry_run=true` 的预览结果，不会写入数据。
- 迁移预检返回源/目标 driver、实体计数、快照大小、目标连通性和重启/配置告警；PostgreSQL 目标会准备目标数据库和 `twilight_state` 状态表，但不会写入业务快照数据。
- 恢复备份和执行迁移前都会自动创建保护性备份，响应中返回 `pre_operation_backup` 便于回滚审计。
- 恢复备份路径经过目录约束，拒绝路径穿越。
- 启动时如果配置为 PostgreSQL，但 PostgreSQL 状态没有管理员且 JSON 状态文件已有管理员，会临时回退到 JSON，让原管理员可以登录并执行迁移。
- 若 Go 状态没有管理员但存在旧 `db/users.db`，后端会尝试只读导入旧 SQLite 中的 active 管理员，依赖系统存在 `sqlite3` 命令；如果旧库也没有管理员，则按新安装流程创建首个管理员。

## 运行状态与实时日志

- 管理端新增 `/admin/logs`，对应后端 `/api/v1/system/admin/runtime/status`、`/runtime/logs`、`/runtime/logs/stream`。
- 实时日志只接入 Go 进程内 `slog`，不会开放任意日志文件、journald 或路径参数读取。
- 日志缓冲保留最近 1200 条，前端默认展示最近 500 条，避免长时间运行后撑爆内存或页面。
- 日志输出会尽力脱敏 `Authorization`、`Cookie`、`token`、`secret`、`password`、`api_key`、`bot_token`、`dsn`、`Bearer`、`key-*` 等敏感内容。
- 状态接口只读取 Go runtime 和 Linux `/proc` 摘要信息，不返回环境变量、配置明文、命令行参数或进程列表。

## Git 更新

`POST /api/v1/system/admin/update` 只接受不带凭据的 HTTPS 仓库 URL 和受限分支名。更新前会读取当前分支、commit、remote 和 `git status --porcelain`：

- 默认拒绝有未提交改动的 worktree，避免覆盖本地变更。
- 支持 `dry_run` 只做安全预检，不执行 `fetch/pull`。
- 实际更新使用 `git fetch`、`git checkout`、`git pull --ff-only`，不执行 shell 字符串。
- 响应中的仓库 URL 会去除凭据信息，并返回 before/after commit 便于审计。
- 只有 `before.commit != after.commit` 且请求 `restart_services=true` 时才安排服务重启；优先使用 `systemd-run --on-active=2` 延迟重启 `twilight`、`twilight-bot`、`twilight-scheduler`。

## 安全基线

- 所有 JSON 响应使用统一 envelope：`success`、`code`、`message`、`data`、`timestamp`。
- Cookie 会话默认 `HttpOnly`、`SameSite=Lax`。
- Cookie 写请求必须包含 `X-Twilight-Client: webui`，否则返回 `403`。
- CORS 必须显式列出可信 Origin；带凭据接口不会接受 `*`。
- API Key 只保存 SHA-256 哈希，明文仅创建时返回一次。
- 上传接口只接受明确白名单内的栅格图片 MIME，写入受控目录，不直接暴露整个 `uploads/`。
- 上传资源读取只接受服务端生成的文件名格式，并在返回前重新校验绝对路径仍位于上传目录内。
- 数据库备份恢复只接受备份目录内的普通 `.json` 文件，拒绝绝对路径、`..`、子目录跳转和符号链接。
- JSON 状态迁移目标必须位于数据库目录内且扩展名为 `.json`，防止误写配置、脚本或其他文件类型。
- 默认响应安全头：`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy`、`Permissions-Policy`。

## 验证

```bash
go test ./...
go vet ./...
```

如果本机安装了 `govulncheck`：

```bash
govulncheck ./...
```

## 旧后端说明

`golang` 分支以后端 Go 实现为准，旧 Python 后端源码和依赖元数据已移除。为了避免迁移前管理员被锁在后台外，Go 后端会在没有 active 管理员时只读读取旧 `db/users.db` 中的 active 管理员账号做引导登录；其他旧 SQLite 业务数据通过数据库管理页的“旧 SQLite”来源显式预览、备份并迁移到 Go 状态存储或 PostgreSQL。
