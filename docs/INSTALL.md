# 安装部署

Twilight Go 后端面向 Linux 部署设计，生产环境建议使用 systemd 管理服务，并通过 HTTPS 反向代理暴露前端和 API。

## 环境要求

- Go 1.23 或更高版本。
- Node.js 22 或更高版本。
- pnpm。
- 可访问的 Emby 或 Jellyfin 服务。
- 可选：`sqlite3` 命令。仅在存在旧 Python 版 `db/users.db` 且 Go 状态没有管理员时，用于只读导入旧管理员账号完成后台引导登录。
- 生产环境建议配置 Redis，用于共享会话和限流计数。

## 后端部署

```bash
go build -o bin/twilight ./cmd/twilight
cp config.production.toml config.toml
bash start_backend_prod.sh
```

API 默认端口为 `5000`，监听地址以 `config.toml` 或启动参数为准；也可以通过 `TWILIGHT_API_HOST` 和 `TWILIGHT_API_PORT` 覆盖。

HTTPS 反向代理部署时必须注意：

- `TWILIGHT_SESSION_COOKIE_SECURE=true`
- `TWILIGHT_API_CORS_ORIGINS=https://你的前端域名`
- 不要把 `*` 用作带 Cookie 登录态接口的 CORS Origin。

## PostgreSQL 配置

默认配置使用 JSON 状态文件。需要 PostgreSQL 时，在 `config.toml` 中修改 `[Database]`：

```toml
[Database]
driver = "postgres"
state_file = ""
url = ""
backup_dir = ""
postgres_host = "127.0.0.1"
postgres_port = 5432
postgres_user = "twilight"
postgres_password = "请替换为高强度密码"
postgres_database = "twilight"
postgres_sslmode = "disable"
postgres_max_open_conns = 8
postgres_max_idle_conns = 4
```

也可以只填写完整 DSN：

```toml
[Database]
driver = "postgres"
url = "postgres://twilight:请替换为高强度密码@127.0.0.1:5432/twilight?sslmode=disable"
```

环境变量覆盖：

```bash
TWILIGHT_DATABASE_DRIVER=postgres
TWILIGHT_DATABASE_URL=postgres://twilight:密码@127.0.0.1:5432/twilight?sslmode=disable
# 也可使用等价别名：TWILIGHT_POSTGRES_DSN=postgres://...
TWILIGHT_POSTGRES_MAX_OPEN_CONNS=8
TWILIGHT_POSTGRES_MAX_IDLE_CONNS=4
```

切换前必须先在管理端执行数据库迁移预检；预检会验证源快照和目标 PostgreSQL，并在目标库不存在且权限允许时自动创建数据库和 `twilight_state` 状态表，但不会写入业务快照数据。确认 `target_ready.connected=true` 且 `target_ready.schema_ready=true` 后，再在前端二次确认执行迁移。后端会在实际迁移前自动创建保护性备份，并在响应中返回 `pre_operation_backup`。低配 1Panel 或同机 PostgreSQL 建议先保持连接池较小，避免数据库连接数被面板、备份任务和 Twilight 同时打满。

如果已经有 JSON 状态文件，不要先删除它。即使你提前把 `driver` 改成 `postgres`，只要 PostgreSQL 还没有管理员且 JSON 里已有管理员，后端会临时回退到 JSON 启动，让原管理员能登录管理端完成迁移。迁移确认成功后，再重启服务切换到 PostgreSQL。

如果只有旧 Python 版 `db/users.db`，请在 Linux 上安装 `sqlite3`。Go 后端会在自身状态没有 active 管理员时只读导入旧库 active 管理员账号；这只是登录引导，不会在启动时隐式全量迁移旧 SQLite 业务数据。

登录管理端后进入“数据库迁移”，来源选择“旧 SQLite”即可预览旧库导入结果。页面左侧显示源数据库，右侧显示目标数据库，底部展示字段映射、实体计数、备份策略和告警；后端会按固定库名扫描并映射多份旧库，确认执行前会先备份当前 Go 状态和旧 SQLite 文件集，再写入 JSON 或 PostgreSQL。

如果 PostgreSQL 用户已经存在但数据库还不存在，后端启动和迁移预检时会尝试自动创建 `postgres_database` / URL 中的目标数据库并准备 `twilight_state` 状态表。该用户必须拥有 `CREATEDB` 权限；没有权限时，需要先让 PostgreSQL 管理员执行 `ALTER USER <用户名> CREATEDB;` 或手动创建数据库。

## 前端部署

```bash
cd webui
pnpm install --frozen-lockfile
pnpm build
pnpm start -p 3000
```

生产环境建议让前端域名和 API 域名保持明确的 HTTPS Origin，并在后端 CORS 白名单中逐项填写。`cors_origins` 只能填写协议、主机和端口，例如 `https://panel.example.com`；尾斜杠会自动处理，但不要带 `/admin` 这类路径。

## 特殊部署环境注意事项

- 1Panel Go 运行环境：运行目录指向项目根目录，启动命令使用 `./bin/twilight api --host 0.0.0.0 --port 5000 --config config.toml`，不要把 `config.toml`、`.env`、1Panel 运行配置提交到 Git。
- 反向代理：推荐后端只监听 `127.0.0.1:5000`，由 Nginx/Caddy/1Panel OpenResty 暴露 HTTPS；跨域部署时 `session_cookie_samesite` 必须与域名关系匹配。
- 同域部署：前端 `/api/*` 反代到后端时，`NEXT_PUBLIC_API_URL` 可以留空；分离域名部署时必须设置为后端 HTTPS 地址，并同步后端 `cors_origins`。
- Cloudflare/OpenNext：标准 Node/1Panel 部署不需要启用 OpenNext dev 初始化；只有 Cloudflare 本地开发需要设置 `TWILIGHT_OPENNEXT_DEV=true`。
- systemd：项目路径、配置路径和二进制路径不要包含空白或 `%`，setup 脚本会拒绝这类路径，避免 unit 解析歧义。

## systemd 一键设置

项目提供 Linux-only 一键脚本：

```bash
sudo bash deploy/setup-systemd.sh --dry-run
sudo bash deploy/setup-systemd.sh --restart
```

脚本会执行以下检查和操作：

- 检查当前系统是否为 Linux。
- 检查是否以 root 权限运行。
- 检查项目目录是否包含 Go 后端入口。
- 检查配置文件、二进制路径、服务用户/组和端口合法性。
- 如果发现旧 `db/users.db` 但系统缺少 `sqlite3`，输出引导导入告警。
- 在缺少 `bin/twilight` 时自动构建后端二进制。
- 创建 `db/`、`db/backups/`、`uploads/`、`config_backups/` 等运行目录。
- 扫描 `twilight.service`、`twilight-bot.service`、`twilight-scheduler.service` 是否仍指向旧 Python 入口。
- 检测到旧 Python unit 时，会停止、禁用并备份旧 unit，再写入 Go 版 unit。

常用覆盖参数：

```bash
sudo TWILIGHT_PROJECT_ROOT=/opt/Twilight \
  TWILIGHT_CONFIG_FILE=/opt/Twilight/config.toml \
  TWILIGHT_API_HOST=127.0.0.1 \
  TWILIGHT_API_PORT=5000 \
  TWILIGHT_SYSTEMD_USER=twilight \
  bash deploy/setup-systemd.sh --restart
```

`twilight-scheduler` 保留为兼容旧部署的服务名，实际定时任务由 API 进程提供和管理。`twilight-bot` 在未启用 Telegram 或未配置 Bot Token 时会安全等待退出信号，不会反复失败重启。

## 运行数据与备份

默认运行数据：

- JSON 状态文件：`db/twilight_go_state.json`
- 数据库备份目录：`db/backups/`
- 上传目录：`uploads/`
- 配置备份目录：`config_backups/`

管理员可以在前端“数据库迁移”页执行数据库备份、恢复和迁移预检。恢复和迁移必须先预览、再二次确认；后端在实际写入前会自动创建保护性备份。创建备份时若检测到旧 SQLite 文件，会同时复制 `.db`、`.db-wal`、`.db-shm` 文件集。备份恢复只接受备份目录内的普通 `.json` 文件，旧 SQLite 备份用于迁移前回滚，不通过恢复接口直接覆盖运行库。

## 本地配置与密钥

以下内容不应提交到 Git：

- `config.toml`
- `.env`
- `config.local.toml`
- 1Panel 本地运行配置和环境变量文件。
- Emby API Key、Telegram Bot Token、PostgreSQL 密码、Redis 密码。

`.gitignore` 已覆盖常见本地配置、运行数据和 1Panel 文件名模式；新增部署文件前仍应先确认是否包含密钥。
