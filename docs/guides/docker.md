# Docker 部署指南

> ⚠️ **重要警告 - 请先阅读** ⚠️
>
> **开发者完全不推荐使用 Docker 部署 Twilight。**
>
> - 本 Docker 部署方案由 LLM (AI) 自动生成，**未经任何实际测试**。
> - 可能存在配置错误、安全漏洞、数据丢失风险或生产事故。
> - 开发者团队不使用也不维护 Docker 部署方式，优先方案始终是 Linux + systemd 原生部署。
> - 如执意使用：请自行完成完整的部署测试，确认功能正常、数据安全后再投入生产。
> - 数据备份务必在启用前完成，任何因 Docker 部署导致的数据丢失、服务中断开发者概不负责。
>
> **推荐部署方式**: 参见 [开发指南](development.md) 中的 Linux + systemd 部署说明。

Twilight 提供完整的 Docker 支持，包含 PostgreSQL + Redis + Go 后端 + Next.js 前端的一键部署方案。

## 目录

- [前置要求](#前置要求)
- [快速开始](#快速开始)
- [架构说明](#架构说明)
- [服务详解](#服务详解)
- [配置管理](#配置管理)
- [数据持久化](#数据持久化)
- [反向代理 (Nginx/Caddy)](#反向代理-nginxcaddy)
- [升级与维护](#升级与维护)
- [故障排查](#故障排查)
- [多进程模式 (PostgreSQL only)](#多进程模式-postgresql-only)
- [生产环境检查清单](#生产环境检查清单)

## 前置要求

| 依赖 | 最低版本 | 说明 |
|------|---------|------|
| Docker | 24.0+ | 包含 BuildKit (默认启用) |
| Docker Compose | v2.20+ | `docker compose` (非独立 `docker-compose`) |
| 磁盘空间 | ~2 GB | 镜像 + 数据库 + 上传 |

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight
```

### 2. 创建配置文件

```bash
# 后端配置
cp deploy/docker/config.docker.toml config.toml
# 编辑 config.toml，至少填写 Emby URL 和 Token
vim config.toml

# 前端环境变量
cp webui/.env.example webui/.env
# 编辑 webui/.env，设置站点名称和 API 地址
vim webui/.env
```

### 3. 设置环境变量（可选）

创建 `.env` 文件在项目根目录，覆盖敏感配置：

```bash
cp deploy/docker/.env.example .env
vim .env
```

`.env` 至少应修改 `POSTGRES_PASSWORD` 和 `BOT_INTERNAL_SECRET`。示例文件只使用占位符，不包含真实密钥。

### 4. 启动服务

```bash
# 构建镜像并启动
docker compose up -d --build

# 生产环境建议叠加只读根文件系统与资源限制
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build

# 查看日志
docker compose logs -f

# 查看运行状态
docker compose ps
```

### 5. 初次访问

- **前端界面**: http://localhost:3000
- **后端 API**: 默认不暴露到宿主机；容器内健康检查使用 `GET /api/v1/system/health`
- **首次注册**: 打开注册页面注册第一个账号，用户名必须在 `ADMIN_USERNAMES` 中才会成为管理员

## 架构说明

```
┌─────────────────────────────────────────────────────┐
│                    反向代理 (Nginx/Caddy)             │
│              :443 HTTPS → 内部路由                    │
├──────────────┬──────────────────┬───────────────────┤
│              │                  │                    │
│  twilight-webui   twilight-backend   postgres:5432  │
│  (Next.js :3000)  (Go API :5000)    redis:6379      │
│              │                  │                    │
│              └──────┬───────────┘                    │
│                     │                                │
│          twilight-net (bridge)                       │
└─────────────────────┴────────────────────────────────┘
```

**网络**: 所有服务通过 `twilight-net` 桥接网络通信，服务名即为 hostname。

**进程模式**: 默认使用 `all` 模式——Go 后端在一个进程中运行 API + 调度器 + Bot。这是 Docker 推荐的部署方式。

## 服务详解

### PostgreSQL (`postgres`)

- 镜像: `postgres:17-alpine`
- 存储所有状态数据 (单行 JSONB + sessions + runtime logs)
- 数据库和表由 Go 后端首次启动时自动创建
- 数据卷: `twilight-postgres-data`

### Redis (`redis`)

- 镜像: `redis:7-alpine`
- 用于跨进程 session 共享和限流计数器
- 默认 maxmemory 128MB，LRU 淘汰策略
- 数据卷: `twilight-redis-data`

### Go 后端 (`twilight`)

- 运行模式: `all`（API + 调度器 + Bot 合一）
- 端口: `5000` (内部)
- 健康检查: `GET /api/v1/system/health`
- 配置文件: `./config.toml` (只读挂载)
- 以非 root 用户运行，并启用 `no-new-privileges`
- Docker 日志默认按 `10m * 3` 轮转，避免宿主机日志无限增长
- 数据卷:
  - `twilight-uploads`: 用户上传（头像/背景）
  - `twilight-backups`: 数据库备份

### Next.js 前端 (`webui`)

- 构建输出: `output: 'standalone'`
- 端口: `3000`
- 通过 `BACKEND_URL` 环境变量指向后端 API
- `NEXT_PUBLIC_API_URL` 在构建时嵌入，跨域访问需要配置 CORS

## 配置管理

### 配置层级（由低到高优先级）

1. `config.toml` 文件（挂载到容器）
2. `config.local.toml` 文件（私密覆写，可选）
3. `TWILIGHT_*` 环境变量（最高优先级）

### Docker 特定配置

| 配置项 | Docker 值 | 说明 |
|--------|----------|------|
| `Database.driver` | `postgres` | Docker 必须用 PostgreSQL |
| `Database.postgres_host` | `postgres` | 服务名即 hostname |
| `Redis.redis_url` | `redis://redis:6379/0` | 服务名即 hostname |
| `API.host` | `0.0.0.0` | 监听所有接口 |
| `SystemUpdate.auto_update_enabled` | `false` | Docker 通过镜像更新 |

### 只读配置与后台配置管理

默认 compose 将 `./config.toml` 以只读方式挂载，适合把 Docker 部署视为不可变配置：修改配置后编辑宿主机文件并重启容器。

如果需要在后台配置管理页面直接保存 `config.toml`，可把挂载改为 `./config.toml:/app/config.toml:rw`，并确保宿主机文件允许容器内 UID `1001` 写入。生产环境更推荐保留只读挂载，敏感覆写放入 `config.local.toml` 或 `TWILIGHT_*` 环境变量。

### 环境变量参考

支持所有 `TWILIGHT_*` 环境变量。常用:

| 环境变量 | 示例值 | 说明 |
|---------|--------|------|
| `TWILIGHT_GLOBAL_SERVER_NAME` | 暮光 | 站点名称 |
| `TWILIGHT_ADMIN_USERNAMES` | admin,root | 管理员用户名 (逗号分隔) |
| `TWILIGHT_EMBY_TOKEN` | abc123... | Emby API Token |
| `TWILIGHT_TELEGRAM_BOT_TOKEN` | 123:abc | Telegram Bot Token |
| `TWILIGHT_SMTP_HOST` | smtp.gmail.com | SMTP 服务器 |
| `TWILIGHT_RATE_LIMIT_ENABLED` | true | 限流开关 |

完整列表见 [`docs/reference/backend.md`](reference/backend.md)。

## 数据持久化

### Docker 命名卷

| 卷名 | 挂载点 | 内容 |
|------|--------|------|
| `twilight-postgres-data` | `/var/lib/postgresql/data` | PostgreSQL 数据 |
| `twilight-redis-data` | `/data` | Redis AOF 持久化 |
| `twilight-uploads` | `/app/uploads` | 用户上传文件 |
| `twilight-backups` | `/app/db/backups` | 数据库备份 |

### 备份策略

```bash
# 创建数据库备份（容器内）
docker compose exec twilight ./twilight version  # 确认运行正常

# PostgreSQL 逻辑备份
docker compose exec postgres pg_dump -U twilight twilight > backup_$(date +%Y%m%d).sql

# 恢复
docker compose exec -T postgres psql -U twilight twilight < backup_20260101.sql
```

Twilight 后台管理页面也提供可视化备份/恢复功能（路径: 系统配置 → 数据库 → 备份）。

## 反向代理 (Nginx/Caddy)

生产环境必须在前端部署反向代理来:
- 终止 HTTPS (TLS)
- 添加安全头
- 限流和访问控制
- 统一入口（前后端同域）

### Nginx 配置示例

参考 `deploy/nginx-twilight.conf`。关键配置:

```nginx
# 后端 API 反代
location /api/ {
    proxy_pass http://127.0.0.1:5000;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# 前端静态文件
location / {
    proxy_pass http://127.0.0.1:3000;
}
```

Nginx 也配置了限流 zone，参考 `deploy/nginx-rate-limit.conf`。

### 前端配置

当使用反向代理统一域名时，前端 `.env` 配置:

```env
# 同域代理（推荐）
NEXT_PUBLIC_API_URL=

# 跨域（需 CORS 配置）
NEXT_PUBLIC_API_URL=https://api.yourdomain.com
```

`NEXT_PUBLIC_API_URL` 为空时，前端通过 Next.js rewrite 将 `/api/*` 代理到 `BACKEND_URL`。

## 升级与维护

### 更新镜像

```bash
# 拉取最新代码
git pull

# 重新构建并重启
docker compose up -d --build

# 或仅重建特定服务
docker compose up -d --build twilight webui
```

### 日常维护

```bash
# 查看日志
docker compose logs -f --tail=100 twilight

# 重启服务
docker compose restart twilight

# 进入容器调试
docker compose exec twilight sh

# 清理旧镜像
docker image prune -a
```

### 健康检查

所有服务都配置了 Docker healthcheck：

```bash
# 检查服务健康状态
docker compose ps

# 手动检查后端（从容器网络内）
docker compose exec twilight curl -fsS http://127.0.0.1:5000/api/v1/system/health

# 预期响应
# {"success":true,"data":{"api":true,"database":true,"emby":true}}
```

Compose 已为所有服务配置 `init: true`、`stop_grace_period: 30s`、`no-new-privileges` 和 Docker JSON 日志轮转。生产覆写文件额外启用后端只读根文件系统、临时 `/tmp` 与资源上限。

## 故障排查

### 后端无法连接 PostgreSQL

```bash
# 检查 PostgreSQL 日志
docker compose logs postgres

# 确保 postgres 服务健康
docker compose ps postgres

# 测试连接
docker compose exec postgres pg_isready -U twilight
```

### 前端无法连接后端

```bash
# 检查 backend URL 配置
docker compose exec webui env | grep BACKEND_URL

# 从前端容器测试后端可达性
docker compose exec webui wget -qO- http://twilight:5000/api/v1/system/health
```

### 容器不断重启

```bash
# 查看详细日志
docker compose logs twilight --tail=100

# 常见原因:
#  - config.toml 不存在或格式错误
#  - PostgreSQL 连接失败
#  - 端口冲突
```

### 重置数据库

```bash
# 停止所有服务
docker compose down

# 删除数据卷（警告：会清除所有数据！）
docker volume rm twilight-postgres-data twilight-redis-data

# 重新启动
docker compose up -d
```

## 多进程模式 (PostgreSQL only)

默认 `all` 模式适合大多数部署。如需将 API / 调度器 / Bot 拆分为独立容器（适合大规模或需要独立扩缩容场景），使用命令覆写:

```bash
# 方式一：启动多个 twilight 容器
docker compose up -d twilight  # 默认 API
docker compose run -d --name twilight-scheduler \
  twilight scheduler --config config.toml
docker compose run -d --name twilight-bot \
  twilight bot --config config.toml

# 方式二：使用生产覆写文件
# 编辑 docker-compose.prod.yml，取消 scheduler/bot 的注释
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

> **注意**: 拆分模式要求 `Database.driver = "postgres"`，因为多进程需要共享数据库。

## 生产环境检查清单

部署到生产环境前，确认以下项目:

- [ ] `config.toml` 中 `Database.driver` 设为 `postgres`
- [ ] PostgreSQL 密码已从默认值更改
- [ ] `bot_internal_secret` 已生成高熵随机字符串
- [ ] `session_cookie_secure` 设为 `true`（HTTPS 部署）
- [ ] `cors_origins` 已设置为明确的 HTTPS 域名（非 `*`）
- [ ] `trusted_proxy_cidrs` 已配置可信反代 CIDR
- [ ] `SystemUpdate.auto_update_enabled` 设为 `false`（Docker 不需要）
- [ ] 反向代理已配置 HTTPS 和 HSTS
- [ ] 数据库备份已配置（pg_dump cron 或内置备份）
- [ ] 文件卷 (`uploads`, `backups`) 已规划备份
- [ ] 日志已接入外部收集系统（可选）
