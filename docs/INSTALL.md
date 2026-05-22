# 安装部署

Twilight Go 后端面向 Linux 部署设计，生产环境建议使用 systemd 管理服务，并通过 HTTPS 反向代理暴露前端和 API。

## 环境要求

- Go 1.23 或更高版本。
- Node.js 22 或更高版本。
- pnpm。
- 可访问的 Emby 或 Jellyfin 服务。
- 生产环境建议配置 Redis，用于共享会话和限流计数。

## 后端部署

```bash
go build -o bin/twilight ./cmd/twilight
cp config.production.toml config.toml
bash start_backend_prod.sh
```

API 默认监听 `127.0.0.1:5000`，也可以通过 `TWILIGHT_API_HOST` 和 `TWILIGHT_API_PORT` 覆盖。

HTTPS 反向代理部署时必须注意：

- `TWILIGHT_SESSION_COOKIE_SECURE=true`
- `TWILIGHT_API_CORS_ORIGINS=https://你的前端域名`
- 不要把 `*` 用作带 Cookie 登录态接口的 CORS Origin。

## PostgreSQL 配置

默认配置使用 JSON 状态文件。需要 PostgreSQL 时，在 `config.toml` 中修改 `[Database]`：

```toml
[Database]
driver = "postgres"
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

切换前必须先在管理端执行数据库迁移预检；预检只验证连接和快照信息，不会创建 PostgreSQL 表或写入数据。确认 `target_ready.connected=true` 后，再在前端二次确认执行迁移。后端会在实际迁移前自动创建保护性备份，并在响应中返回 `pre_operation_backup`。低配 1Panel 或同机 PostgreSQL 建议先保持连接池较小，避免数据库连接数被面板、备份任务和 Twilight 同时打满。

## 前端部署

```bash
cd webui
pnpm install --frozen-lockfile
pnpm build
pnpm start -p 3000
```

生产环境建议让前端域名和 API 域名保持明确的 HTTPS Origin，并在后端 CORS 白名单中逐项填写。

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

管理员可以在前端配置页执行数据库备份、恢复和迁移预检。恢复和迁移必须先预览、再二次确认；后端在实际写入前会自动创建保护性备份。备份恢复只接受备份目录内的普通 `.json` 文件。

## 本地配置与密钥

以下内容不应提交到 Git：

- `config.toml`
- `.env`
- `config.local.toml`
- 1Panel 本地运行配置和环境变量文件。
- Emby API Key、Telegram Bot Token、PostgreSQL 密码、Redis 密码。

`.gitignore` 已覆盖常见本地配置、运行数据和 1Panel 文件名模式；新增部署文件前仍应先确认是否包含密钥。
