<div align="center">

![Twilight Logo](Twilight%20Logo.png)

# Twilight 暮光

面向 Emby / Jellyfin 的用户、邀请码、注册码、求片与 Telegram Bot 管理面板。

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![Version](https://img.shields.io/badge/Version-0.0.4-blue)](docs/VERSION_HISTORY.md)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

[Telegram 频道](https://t.me/Twilightpanel) · [Telegram 群组](https://t.me/TwilightPanelChat) · [文档导航](docs/README.md)

</div>

## 项目说明

Twilight 的 `golang` 分支以 Go 后端为主线，目标部署环境是 Linux。旧 Python 后端不再作为生产运行路径，部署、systemd、数据库迁移、日志与自动更新都应以 Go 后端文档为准。

本项目大量代码由 LLM 辅助生成，并经过人工修缮。请在正式开放前自行审查配置、权限、反向代理、数据库备份和外部服务密钥。任何因使用本项目造成的损失与责任均与项目维护者无关。

当前版本：`0.0.4`。版本历史和本次 Go 后端重构内容见 [docs/VERSION_HISTORY.md](docs/VERSION_HISTORY.md)。

## 功能概览

| 模块 | 能力 |
| ---- | ---- |
| 用户管理 | 注册、登录、续期、禁用、删除、白名单、待补建 Emby 账号、批量操作 |
| 注册码体系 | 注册码、续期码、白名单码、假卡码、备注、使用记录、批量生成、批量删除 |
| 邀请系统 | 邀请码、续期邀请码、邀请树、管理员邀请森林星图、层级启用/禁用/删除 |
| Emby / Jellyfin | 账号创建、绑定、解绑、媒体库权限、播放会话、孤儿账号清理 |
| 求片系统 | TMDB / Bangumi 搜索、库存检查、求片提交、管理员审核、外部回调更新 |
| Telegram Bot | 用户绑定、个人查询、注册码使用、管理员私聊命令、群组成员安全管理 |
| 安全与审计 | Session、API Key、登录记录、设备管理、IP 黑名单、运行日志脱敏 |
| 系统维护 | 配置热重载、数据库备份/恢复/迁移、Git 拉取更新、systemd 延迟重启 |
| 前端面板 | Next.js 管理端、自适应布局、移动端优化、实时日志、服务器状态 |
| TestWeb | 只读演示接口和模拟页面，不读取真实登录态，不执行真实写入 |

## 架构

```text
webui/                  Next.js 前端
cmd/twilight/           Go 后端入口，支持 api / all / bot / scheduler
internal/api/           REST API、鉴权、业务处理、外部服务客户端
internal/store/         JSON / PostgreSQL 存储抽象
internal/config/        TOML 配置加载、兼容旧配置、环境变量覆盖
deploy/                 Nginx、systemd、一键安装脚本
docs/                   中文部署、API、安全、版本历史文档
```

Go 后端默认使用 JSON 状态文件，生产环境可切换 PostgreSQL。切换数据库前必须先在管理端执行迁移预览，确认后端会在真正迁移或恢复前自动创建保护性备份。如果配置成 PostgreSQL 但目标库仍没有管理员，同时旧 JSON 状态文件里已有 active 管理员，后端会临时回退到 JSON 状态启动，确保原管理员可以登录前端完成迁移。若只有旧 Python 版 `db/users.db`，且系统安装了 `sqlite3` 命令，Go 后端会在自身状态还没有管理员时只读导入旧库中的 active 管理员账号用于引导登录。

独立的“数据库迁移”管理页会自动扫描 `db/*.db`、`*.db-wal`、`*.db-shm` 旧 SQLite 文件。备份会同时复制旧 SQLite 文件集；迁移来源可选择“旧 SQLite”，后端按固定库名只读导入 `users.db`、`regcode.db`、`invites.db`、`api_keys.db`、`announcements.db`、`bangumi.db`、`signin.db`、`login_log.db`、`playback.db`、`scheduler_*.db`、`telegram_roster.db` 等数据，再写入 JSON 或 PostgreSQL。

## 环境要求

- Linux 服务器，建议使用 systemd 管理后端服务。
- Go `1.23+`。
- Node.js `22+`，用于构建 Next.js 前端。
- 已部署并可由后端访问的 Emby 或 Jellyfin。
- 可选：PostgreSQL，用于更好的并发和长期运行性能。
- 可选：Redis，用于多实例或更稳定的 Session / 限流存储。

## 快速开始

```bash
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight
git checkout golang

go test ./...
go build -o bin/twilight ./cmd/twilight

cp config.production.toml config.toml
vim config.toml

./bin/twilight api --config config.toml
```

前端开发：

```bash
cd webui
npm ci
NEXT_PUBLIC_API_URL=http://127.0.0.1:5000 npm run dev
```

生产部署请优先阅读 [docs/INSTALL.md](docs/INSTALL.md)。如果使用 1Panel 的 Go 运行环境，后端入口建议配置为 `./cmd/twilight` 或已构建的 `bin/twilight`，运行参数使用 `api --config /path/to/config.toml`，本地运行配置、环境变量文件和面板生成文件不要提交到 Git。

## systemd 一键设置

项目提供 Linux systemd 设置脚本，会检查路径、依赖、端口、旧 Python 版 unit 残留和 systemd 特殊字符：

```bash
sudo bash deploy/setup-systemd.sh --dry-run
sudo bash deploy/setup-systemd.sh --restart
```

脚本默认写入 `twilight`、`twilight-bot`、`twilight-scheduler` 三个服务。Telegram 未启用时 bot 子命令会安全空转，避免 systemd 重启循环。

## 配置与数据库

主要配置文件为 `config.toml`，本地私密覆盖建议使用 `config.local.toml`，该文件已被 `.gitignore` 忽略。未显式传 `--config` 时，后端优先读取当前目录 `config.toml`；如果不存在，再读取 `TWILIGHT_CONFIG_FILE` 指向的文件；配置字段最终仍可被 `TWILIGHT_*` 环境变量覆盖。

服务器图标不再从 `config.toml` 配置。后端公开信息固定返回内置 `/favicon.svg`；如需替换图标，请走前端静态资源或前端构建环境变量，不要把本地图片路径写入后端配置。

数据库配置支持：

```toml
[Database]
driver = "json"        # json 或 postgres
state_file = ""        # 留空时使用 <databases_dir>/twilight_go_state.json
backup_dir = "db/backups"

# PostgreSQL 可使用 url，也可拆分配置 host/user/password/database。
url = ""
postgres_host = "127.0.0.1"
postgres_port = 5432
postgres_user = "twilight"
postgres_password = ""
postgres_database = "twilight"
postgres_sslmode = "disable"
postgres_max_open_conns = 8
postgres_max_idle_conns = 4
```

数据库恢复和迁移均需要预览与二次确认。执行前后端会创建 `pre_operation_backup`，避免误操作后没有回滚点。已有 JSON 数据切换 PostgreSQL 时，先保持 JSON 能登录，进入管理端完成 PostgreSQL 迁移预检和确认后，再重启到 PostgreSQL。旧 SQLite 管理员引导只用于恢复后台入口，不会在启动时全量迁移旧业务数据。

PostgreSQL 启动和迁移预检时如果目标数据库不存在，后端会尝试用同一连接用户连到 `postgres` / `template1` 维护库并创建目标数据库；该用户需要 `CREATEDB` 权限。数据库创建完成后会继续初始化 `twilight_state` 状态表，预检响应会显示 `database_created` 和 `schema_ready` 状态。

## 安全边界

生产环境至少确认以下项目：

- 只允许可信前端域名写入 `cors_origins`，不要把带凭据 CORS 配成通配；填写 Origin 时只写 `https://app.example.com` 这种协议、域名和端口，不要带路径。
- HTTPS 部署时启用安全 Cookie：`session_cookie_secure = true`，并按反向代理情况配置 SameSite。
- 不要直接公开 `uploads/` 目录；头像和背景图必须通过 `/api/v1/users/assets/{kind}/{filename}` 受控读取。
- 管理接口均要求管理员鉴权，包括数据库、运行日志、Git 更新、注册码批量删除、用户批量操作和 Telegram 管理操作。
- 使用 Cookie 鉴权的写操作需要 `X-Twilight-Client: webui`，降低 CSRF 风险。
- Git 自动更新只接受不含凭据的 HTTPS 仓库地址，分支名经过校验，默认拒绝 dirty worktree，并使用 `git pull --ff-only`。
- 运行日志接口会脱敏常见密钥字段，但仍应只开放给管理员。
- 配置文件、数据库文件、备份、上传目录、`.env`、1Panel 本地配置和密钥文件不要提交。

更完整的清单见 [docs/SECURITY.md](docs/SECURITY.md) 和 [docs/SECURITY_AND_PERFORMANCE_REVIEW.md](docs/SECURITY_AND_PERFORMANCE_REVIEW.md)。

## 验证命令

后端：

```bash
gofmt -w internal cmd
go test ./...
go vet ./...
```

前端：

```bash
cd webui
npm run lint
npm run build
```

提交前建议额外检查：

```bash
rg -n "window\\.(alert|confirm|prompt)|\\balert\\(" webui/src -S
git diff --check
```

## 文档导航

| 文档 | 说明 |
| ---- | ---- |
| [安装部署](docs/INSTALL.md) | Linux、systemd、1Panel、Nginx、PostgreSQL 部署 |
| [Go 后端说明](docs/GO_BACKEND.md) | Go 后端启动、配置、Redis、验证 |
| [后端 API](docs/BACKEND_API.md) | REST API 规范与重点接口 |
| [API 索引](docs/API_INDEX.md) | 路由、鉴权和接口分类 |
| [Telegram Bot 命令](docs/TG_BOT_COMMANDS.md) | Bot 命令、私聊边界、管理员命令 |
| [注册码说明](docs/REGCODES.md) | 注册码/续期码/白名单码算法和使用规则 |
| [Bangumi 同步](docs/BANGUMI_SYNC.md) | Emby Webhook 与 Bangumi Token 配置 |
| [安全加固](docs/SECURITY.md) | 生产安全检查清单 |
| [前端说明](docs/FRONTEND.md) | 前端结构、构建、体验优化 |
| [开发者流程](docs/DEVELOPER_WORKFLOW.md) | 分支、验证、合并与发布流程 |
| [版本历史](docs/VERSION_HISTORY.md) | 0.0.1 到当前版本更新记录 |

服务启动后可访问 `/api/v1/docs` 查看简易 API 文档入口。

## 社区

- Telegram 频道：<https://t.me/Twilightpanel>
- Telegram 群组：<https://t.me/TwilightPanelChat>
- GitHub Issues：用于反馈可复现问题和功能建议

## 鸣谢

- [Emby](https://emby.media/)
- [Jellyfin](https://jellyfin.org/)
- [TMDB](https://www.themoviedb.org/)
- [Bangumi 番组计划](https://bgm.tv/)
- [Next.js](https://nextjs.org/)
- [Sakura_embyboss](https://github.com/berry8838/Sakura_embyboss)
- [Bangumi-syncer](https://github.com/SanaeMio/Bangumi-syncer)

## 贡献者

<div align="center">

[![Contributors](https://contrib.rocks/image?repo=Prejudice-Studio/Twilight)](https://github.com/Prejudice-Studio/Twilight/graphs/contributors)

</div>

## Star

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=Prejudice-Studio/Twilight&type=Date)](https://star-history.com/#Prejudice-Studio/Twilight&Date)

如果 Twilight 对你有帮助，欢迎点一个 Star。

Made with love by [Prejudice Studio](https://github.com/Prejudice-Studio/)

</div>
