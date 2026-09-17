# 安装部署

本文介绍如何在生产环境部署 Twilight：Go 后端和 Next.js 前端面向 Linux + systemd 设计，业务状态存储在 PostgreSQL，并通过 HTTPS 反向代理统一对外暴露。

> **推荐**: Linux + systemd 原生部署（`deploy/setup-systemd.sh`）。[Docker 部署](./docker.md) 也有，但开发者不推荐、且未经实测。

## 环境要求

| 组件 | 要求 | 说明 |
| --- | --- | --- |
| Go | 1.25 或更高 | 仅构建后端二进制时需要；运行时不依赖 Go。 |
| PostgreSQL | 必需 | 唯一运行时存储后端。 |
| Emby / Jellyfin | 可访问的实例 | 后端通过 API Token 调用，需在 Emby 后台生成 API 密钥。 |
| Redis | 可选，生产建议 | 用于会话缓存与分布式速率限制计数；留空时退化为进程内内存。 |

部署目标是 Linux + systemd，构建产物为二进制 `bin/twilight`，提供 `api` / `all` / `scheduler` / `bot` / `version` 等子命令（见 `cmd/twilight/main.go`）。

> 说明：即使配置了 Redis，会话仍会持久化到 PostgreSQL，确保 Redis 或进程重启后会话自动恢复；Redis 留空时速率限制计数在重启后重置。

## 后端构建与启动

构建二进制后直接启动：

```bash
go build -o bin/twilight ./cmd/twilight
cp config.production.toml config.toml
./bin/twilight api --host 0.0.0.0 --port 5000 --config config.toml
```

生产环境推荐用 systemd 托管，见 `deploy/setup-systemd.sh`（支持 `--no-build`、`--no-build-webui`）。容器部署见 [Docker 部署](./docker.md)。

可用环境变量覆盖配置（完整清单见 `internal/config/config.go` 的 `applyEnvOverrides`，命名规则为 `TWILIGHT_` + 配置键大写）：

| 环境变量 | 对应配置 |
| -------- | -------- |
| `TWILIGHT_API_HOST` / `TWILIGHT_API_PORT` | `API.host` / `API.port` |
| `TWILIGHT_CONFIG_FILE` | 配置文件路径 |
| `TWILIGHT_CONFIG_LOCAL_FILE` | 额外叠加的本地覆盖文件（默认 `<config>.local.toml`） |
| `TWILIGHT_POSTGRES_DSN` | `Database.url` |
| `TWILIGHT_REDIS_URL` | `Global.redis_url` |
| `TWILIGHT_LOG_LEVEL` | `Global.log_level` |
| `TWILIGHT_ADMIN_UIDS` / `TWILIGHT_ADMIN_USERNAMES` | 管理员名单 |
| `TWILIGHT_PPROF_ADDR` | pprof 监听地址，默认 `127.0.0.1:6060`，设为 `off` 关闭 |

文件描述符上限请在 systemd 单元里用 `LimitNOFILE` 设置（或在容器里设 ulimit），程序自身不再尝试提升 `NOFILE`。

子命令对照：

| 子命令 | 作用 |
| --- | --- |
| `api` | 仅启动 HTTP API 服务（默认子命令，不带子命令时等同 `api`）。 |
| `all` | 在单进程内同时跑 API、定时任务和 Telegram Bot。 |
| `scheduler` | 仅运行定时任务调度器。 |
| `bot` | 仅运行 Telegram Bot 桥接。 |
| `migrate-json` | 把 JSON 状态迁移到 PostgreSQL（见下文的迁移章节）。 |
| `version` | 打印版本号后退出。 |

> 配置文件路径被固定为「运行目录下的 `config.toml`」。`--config` 只能指向同一个文件，传入其它路径会被运行入口拒绝（见 `runtimeConfigPath`），避免特殊部署环境误读到别处的配置。可用环境变量 `TWILIGHT_CONFIG_FILE` 指定。
>
> **启动不会改写 `config.toml`。** `config.Load` 只读（外加可选的 `config.local.toml` 覆盖），缺失的键一律用代码默认值兜底。配置文件只在管理员主动保存或恢复时才会被写入，且写前会自动备份到 `<Database.backup_dir>/config`（默认 `db/backups/config`），不是 `config_backups/`。想让某个键真正落盘，在管理端「系统设置 → 配置」里保存一次即可（保存是合并语义，不会删掉手写的段，见 [Go 后端架构与配置](../reference/backend.md)）。

## PostgreSQL 配置

唯一运行后端为 PostgreSQL（`config.go` 中 `DatabaseDriver` 默认 `postgres`；设为非 postgres 值会在启动期报错）。在运行目录下的 `config.toml` 中配置 `[Database]`：

```toml
[Database]
# 存储后端：只支持 postgres（唯一运行后端）
driver = "postgres"
# 完整 PostgreSQL DSN；填写后优先于下面的分项字段
url = ""
# 数据库备份目录；留空时使用 <databases_dir>/backups
backup_dir = ""
# 数据库迁移面板与迁移 API，默认关闭；需要迁移时临时改为 true，完成后建议关闭
migration_panel_enabled = false
postgres_host = "127.0.0.1"
postgres_port = 5432
postgres_user = "twilight"
postgres_password = "请替换为高强度密码"
postgres_database = "twilight"
# 本机/内网可用 disable；公网或云数据库建议 require / verify-full 并正确配置证书
postgres_sslmode = "disable"
# 连接池；1Panel 或低配机器建议保持较小
postgres_max_open_conns = 8
postgres_max_idle_conns = 4
```

也可以只填写完整 DSN（填写后优先于分项字段）：

```toml
[Database]
driver = "postgres"
url = "postgres://twilight:请替换为高强度密码@127.0.0.1:5432/twilight?sslmode=disable"
```

### 状态存储模型

全部业务状态（用户、注册码/卡码、邀请关系与邀请码、公告等）都保存在「单一状态文档」中：状态文档是 `twilight_state` 表里 `id = 1` 的那一行 `jsonb`。

另有十张独立表：`twilight_audit_logs`（操作审计）、`twilight_sessions`（会话）、`twilight_runtime_logs`（后台实时日志）、`twilight_telegram_roster`（Telegram 群成员花名册）、`twilight_telegram_runtime`（Telegram 更新确认游标）、`twilight_telegram_identity_history`（Telegram 身份变更留痕）、`twilight_playback_records`（播放记录，排行榜与同步的数据源）、`twilight_playback_events`（可信观看原始事件）、`twilight_playback_segments`（播放段状态）、`twilight_playback_daily`（可重建日桶）。这些表会由后端幂等创建，无需手工执行迁移 SQL；升级时旧主状态中的花名册会自动迁移并从 JSONB 清除。

> 不存在「邀请单表」「公告单独建表 / ALTER TABLE 加列」「`db/invites.db`、`db/signin.db` 之类独立数据库」这种结构——邀请、公告等都是上述单一状态文档里的字段（见 `internal/store`）。

### 字段级环境变量覆盖

配置文件加载后，`TWILIGHT_*` 环境变量仍会覆盖对应字段（命名规律为 `TWILIGHT_{SECTION}_{KEY}`，部分字段另有简写别名，见 `internal/config/config.go` 的 `applyEnv`）。数据库相关变量：

```bash
TWILIGHT_DATABASE_DRIVER=postgres
TWILIGHT_DATABASE_URL=postgres://twilight:密码@127.0.0.1:5432/twilight?sslmode=disable
# 等价别名：填写后同样写入 DatabaseURL
TWILIGHT_POSTGRES_DSN=postgres://twilight:密码@127.0.0.1:5432/twilight?sslmode=disable
TWILIGHT_POSTGRES_HOST=127.0.0.1
TWILIGHT_POSTGRES_PORT=5432
TWILIGHT_POSTGRES_USER=twilight
TWILIGHT_POSTGRES_PASSWORD=密码
TWILIGHT_POSTGRES_DATABASE=twilight
TWILIGHT_POSTGRES_SSLMODE=disable
TWILIGHT_POSTGRES_MAX_OPEN_CONNS=8
TWILIGHT_POSTGRES_MAX_IDLE_CONNS=4
TWILIGHT_DATABASE_BACKUP_DIR=/opt/Twilight/db/backups
TWILIGHT_DATABASE_MIGRATION_PANEL_ENABLED=false
# 仅作迁移面板 JSON 导出目标与 migrate-json 默认导入源；运行期不读写此文件
TWILIGHT_STATE_FILE=/opt/Twilight/db/twilight_go_state.json
TWILIGHT_DATABASES_DIR=/opt/Twilight/db
```

### 自动建库与 CREATEDB 权限

当 `driver=postgres` 且目标数据库不存在时，后端会用同一用户连接维护库（`postgres` / `template1`）并执行 `CREATE DATABASE`，随后准备 `twilight_state` 等表（见 `internal/store/store.go`）。因此该 PostgreSQL 用户需要 `CREATEDB` 权限；没有权限时会明确报错（SQLSTATE 42501），此时请让 PostgreSQL 管理员执行 `ALTER USER <用户名> CREATEDB;` 或手动建库。维护库本身（`postgres` / `template1`）不会被自动创建。

低配 1Panel 或与 Twilight 同机的 PostgreSQL，建议保持连接池较小，避免数据库连接数被面板、备份任务和 Twilight 同时打满。

### 数据库迁移面板

数据库迁移面板和迁移 API 默认关闭（`migration_panel_enabled = false`）。需要迁移到另一套 PostgreSQL、或把当前状态一次性导出成 `state.json` 文件时，临时把该字段改为 `true`，完成后建议关回。目标 driver 支持 `postgres`（迁移到另一套 PG，可运行后端）与 `json`（一次性导出目标，产物用于归档或喂给 `twilight migrate-json` 重新导入，非可运行后端）；旧 Python 时代的 SQLite 数据源已被禁用，迁移接口会直接拒绝 `sqlite` / `legacy_sqlite` 作为来源（返回 403 `DB_SQLITE_DISABLED`）。

迁移流程：管理端「数据库迁移」页先做预检（dry-run，预检会验证源快照和目标 PostgreSQL，并在目标库不存在且权限允许时自动创建数据库和 `twilight_state` 状态表，但不写入业务快照）；确认目标可用后再用确认短语二次确认执行。后端在实际写入前会自动创建保护性备份，并在响应中返回 `pre_operation_backup`。

## 前端部署

前端是 `webui/`（Next.js，`output: "standalone"`），构建产物为 `.next/standalone/server.js`：

```bash
corepack enable
pnpm install --frozen-lockfile
pnpm build
# standalone 产物需要 static 与 public 一起就位
cp -r .next/static .next/standalone/.next/static
cp -r public .next/standalone/public
HOST=127.0.0.1 PORT=3001 node .next/standalone/server.js
```

部署形态：

- 同域部署：Nginx 将 `/` 转发到前端，将 `/api/` 保留给 Go API；浏览器只看到一个 Origin。
- 前端是浏览器直连 Go API 的客户端应用：`NEXT_PUBLIC_API_URL` 在构建时注入，留空时走 Next.js rewrite 代理（运行时用 `BACKEND_URL` 指定上游）。
- `ORIGIN` 必须填写浏览器实际访问的完整 Origin，例如 `https://panel.example.com`；本地直连可以使用 `http://127.0.0.1:3001`。
- Docker 不是推荐部署路径，优先 Linux + systemd（`deploy/setup-systemd.sh`）。

`cors_origins` 只能填写协议、主机和端口，例如 `https://panel.example.com`；尾斜杠会被自动处理，但不要带 `/admin` 这类路径。

## HTTPS 反向代理与 Cookie / CORS 注意事项

生产推荐让后端只监听 `127.0.0.1:5000`，由 Nginx / Caddy / 1Panel OpenResty 在前面终止 HTTPS。关键配置：

| 配置项 | 取值 | 说明 |
| --- | --- | --- |
| `session_cookie_secure` / `TWILIGHT_SESSION_COOKIE_SECURE` | `true` | 生产基线，默认即为 `true`；只有 HTTP 调试时才临时关闭。 |
| `cors_origins` / `TWILIGHT_API_CORS_ORIGINS` | 前端 Origin 策略 | 留空为兼容模式；生产限制模式可逐项列出，例如 `https://app.example.com`。 |
| `session_cookie_samesite` / `TWILIGHT_SESSION_COOKIE_SAMESITE` | `Strict` 或 `None` | 同站部署用 `Strict`；前后端跨域必须用 `None`（且 `secure=true`）。 |
| `session_cookie_domain` / `TWILIGHT_SESSION_COOKIE_DOMAIN` | `.example.com` | 仅在前端与 API 是不同子域时设置为共同顶级域，让会话 cookie 跨子域共享；单 origin 部署应留空，写 Domain 反而扩大暴露面。 |

- `*` 在 Twilight 中等价于兼容模式：后端反射具体合法 Origin，而不是向浏览器返回 `Access-Control-Allow-Origin: *`，因此仍可配合 Cookie 凭据使用。需要收紧来源时，请改为显式 Origin 列表。
- 双子域场景（如 webui `https://twilight.example.com`、API `https://twilightapi.example.com`）若希望浏览器对两个子域都携带会话，必须设置 `session_cookie_domain = ".example.com"`。未共享 cookie 时，API 鉴权请求会返回 401，WebUI 会按 `/users/me` 的 401 清理登录态。

### 可信代理

- 后端不要求 CSRF 令牌，也不做额外来源校验；浏览器写请求依赖登录会话 Cookie 鉴权。`X-Twilight-Client: webui` 不参与鉴权。
- 反向代理后必须正确设置 `trusted_proxy_cidrs`：只有当请求的直接上游 IP 落在该列表内时，后端才会信任 `CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For` 解析真实客户端 IP，否则一律用 `RemoteAddr`，防止任意人伪造头绕过 IP 黑名单与限流。默认只信任本机（`127.0.0.0/8`、`::1/128`）；多级代理时显式加入反代出口 IP/CIDR，不要直接填整个私网段。键名必须与代码一致（`trusted_proxy_cidrs`），写错会被静默忽略并退化到 fail-closed。

更完整的安全加固见 [安全加固](./security.md)。

## 特殊部署环境

### 1Panel Go 运行环境

- 运行目录必须指向项目根目录；启动命令使用 `./bin/twilight api --host 0.0.0.0 --port 5000 --config config.toml`。
- 运行入口会拒绝其它目录的配置文件，避免面板环境误读。
- 不要把 `config.toml`、`.env`、1Panel 运行配置提交到 Git。

### Node / 反向代理


### systemd 路径限制

项目路径、二进制路径、配置路径、`.env` 路径不能包含空白或 `%`，setup 脚本会拒绝这类路径（systemd 把 `%` 当作 specifier，空白会引发 unit 解析歧义）。

## systemd 一键设置

项目提供 Linux-only 一键脚本 `deploy/setup-systemd.sh`：

```bash
# 仅预演，不写入任何 unit
sudo bash deploy/setup-systemd.sh --dry-run
# 写入 unit 并重启服务
sudo bash deploy/setup-systemd.sh --restart
```

脚本会执行以下检查和操作：

- 校验运行在 Linux、以 root 权限运行，且系统已安装 `systemctl` / `realpath` / `install` / `mktemp`。
- 校验项目目录确实是 Twilight Go 后端（存在 `go.mod` 与 `cmd/twilight`）。
- 校验项目/二进制/配置/`.env` 路径不含空白或 `%`，主机/端口/用户/组合法（端口在 1–65535），且服务用户与组存在。
- 缺少 `config.toml` 时，若存在 `config.production.toml` 会自动复制生成（属主设为服务用户，权限 0640）；否则报错退出。
- 缺少可执行的 `bin/twilight` 时自动 `go build` 构建（`--no-build` 可跳过构建并要求二进制已存在）。
- 创建运行目录：`db/`、`db/backups/`、`uploads/`、`config_backups/`。
- 扫描 `twilight.service`、`twilight-bot.service`、`twilight-scheduler.service` 是否仍指向旧 Python 入口；检测到旧 Python unit 时会停止、禁用并备份旧 unit，再写入 Go 版 unit。
- 写入 Go API、Bot、Scheduler 和 V2 Web UI unit 后执行 `systemctl daemon-reload`、`enable`，并 `start`（带 `--restart` 时改为 `restart`），最后打印各服务状态。

常用环境变量覆盖：

```bash
sudo TWILIGHT_PROJECT_ROOT=/opt/Twilight \
  TWILIGHT_GO_BIN=/opt/Twilight/bin/twilight \
  TWILIGHT_API_HOST=127.0.0.1 \
  TWILIGHT_API_PORT=5000 \
  TWILIGHT_SYSTEMD_USER=twilight \
  TWILIGHT_SYSTEMD_GROUP=twilight \
  bash deploy/setup-systemd.sh --restart
```

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TWILIGHT_PROJECT_ROOT` | `deploy/` 的父目录 | 项目根目录。 |
| `TWILIGHT_GO_BIN` | `<project>/bin/twilight` | 后端二进制路径。 |
| `TWILIGHT_API_HOST` | `127.0.0.1` | API 监听地址。 |
| `TWILIGHT_API_PORT` | `5000` | API 监听端口。 |
| `TWILIGHT_WEBUI_ROOT` | `<project>/webui` | 前端源码目录（需先 `pnpm build`，脚本也会按需构建）。 |
| `TWILIGHT_WEBUI_HOST` | `127.0.0.1` | 前端监听地址（写入 unit 的 `HOSTNAME`）。 |
| `TWILIGHT_WEBUI_PORT` | `3001` | 前端监听端口。 |
| `TWILIGHT_WEBUI_ORIGIN` | `http://127.0.0.1:3001` | 仅用于部署期校验，Next.js 本身不消费 `ORIGIN`。 |
| `TWILIGHT_NODE_BIN` | `PATH` 中的 `node` | Next.js standalone 运行时。 |
| `TWILIGHT_SYSTEMD_USER` | `root` | systemd 服务用户。 |
| `TWILIGHT_SYSTEMD_GROUP` | 同 `TWILIGHT_SYSTEMD_USER` | systemd 服务组。 |

生成的四个 unit：

- `twilight.service`：`ExecStart` 为 `<bin> api --host <host> --port <port> --config config.toml`，`Restart=always`，配有内存与文件描述符限制。
- `twilight-bot.service`：`<bin> bot --config config.toml`，`PartOf=twilight.service`。Bot 在未启用 Telegram 或未配置 Bot Token 时会安全等待（每 3 秒重读配置），不会反复失败重启。
- `twilight-scheduler.service`：`<bin> scheduler --config config.toml`，`PartOf=twilight.service`。
- `twilight-webui.service`：`node <project>/webui/.next/standalone/server.js`，`PartOf=twilight.service`；脚本会在构建后把 `.next/static` 与 `public` 复制进 standalone 目录（缺了会退化成无样式页面）。

> `EnvironmentFile=-$ENV_FILE` 表示项目根目录下的 `.env`（可选，存在才加载）。

## 运行数据与备份

默认运行数据目录（相对项目根，`databases_dir` 默认 `db`，可改）：

| 路径 | 内容 |
| --- | --- |
| `db/twilight_go_state.json` | 仅迁移面板 JSON 导出目标与 `migrate-json` 默认导入源；运行期不读写。业务状态存 PostgreSQL `twilight_state` 表。 |
| `db/backups/` | 数据库备份目录（`backup_dir` 留空时的默认值）。 |
| `uploads/` | 文件上传目录（`upload_folder` 留空时的默认值）。 |
| `config_backups/` | 启动时自动备份的 `config.toml` 历史。 |

管理员可在前端「数据库迁移」页执行备份、恢复和迁移预检。恢复和迁移必须先预览、再二次确认；后端在实际写入前会自动创建保护性备份。备份恢复只接受备份目录内的普通 `.json` 文件（必须是有效的 Twilight 状态快照）。

## 本地配置与密钥不入库

以下内容不应提交到 Git：

- `config.toml`、`config.local.toml`、`.env`
- 1Panel 本地运行配置和环境变量文件。
- Emby API Token、Telegram Bot Token、PostgreSQL 密码、Redis 密码、`bot_internal_secret` 等敏感值。

`.gitignore` 已覆盖常见本地配置、运行数据和 1Panel 文件名模式；新增部署文件前仍应先确认其中是否包含密钥。

## 相关文档

- [开发指南](./development.md)
- [安全加固](./security.md)
- [Go 后端架构与配置](../reference/backend.md)
- [文档导航](../README.md)
