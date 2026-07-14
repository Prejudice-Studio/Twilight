<div align="center">

![Twilight Logo](Twilight%20Logo.png)

# Twilight 暮光

面向 Emby / Jellyfin 的用户、邀请、卡码、Bot 与运维管理面板。

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

[文档中心](docs/README.md) · [安装部署](docs/guides/install.md) · [Docker 部署](docs/guides/docker.md) · [Telegram 频道](https://t.me/Twilightpanel) · [Telegram 群组](https://t.me/TwilightPanelChat)

</div>

## 项目定位

Twilight 是一个 Go 后端 + Next.js 前端的 Emby / Jellyfin 用户管理系统，适合需要注册审核、卡码续期、邀请关系、Telegram Bot 绑定、设备/IP 审查和后台运维能力的媒体服务器站点。

当前主线架构：

- 后端：Go，入口为 `cmd/twilight`，部署目标为 Linux + systemd，也支持 Docker。
- 前端：Next.js App Router、TypeScript、Tailwind CSS、Radix/shadcn 风格组件。
- 存储：单一状态文档模型，支持 JSON 文件或 PostgreSQL；PostgreSQL 下业务状态仍在 `twilight_state` 单行 jsonb 中，只有会话与运行日志使用独立表。
- 配置：统一读取 `config.toml`、`config.local.toml` 与 `TWILIGHT_*` 环境变量覆盖。

## 核心能力

| 模块 | 能力 |
| ---- | ---- |
| 用户管理 | 注册、登录、续期、禁用、删除、白名单、邮箱验证、设备与登录记录 |
| 媒体服务 | Emby / Jellyfin 账号绑定、开通、解绑、同步、线路下发、在线状态 |
| 卡码体系 | 注册码、续期码、白名单码、诱饵码、指名码、批量生成、使用审计 |
| 邀请系统管理 | 邀请码、续期邀请、邀请关系树、搜索、统计、管理员审查 |
| Telegram | Bot 绑定、通知、换绑审核、群组成员审查、自定义命令与开发者模式沙箱 |
| 求片系统 | TMDB / Bangumi 搜索、库存检查、用户提交、管理员审核、外部回调 |
| 安全中心 | 操作审计、实时日志、违规风控、设备/IP 审查入口、安全配置管理 |
| 运维后台 | 管理导航、配置热重载、数据库备份/恢复/迁移、调度任务、Git 更新 |

## 快速开始

### Docker Compose

```bash
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight
cp deploy/docker/config.docker.toml config.toml
cp deploy/docker/.env.example .env

# 编辑 .env，修改 PostgreSQL 密码、内部密钥等部署项
docker compose up -d --build

# 默认访问
# WebUI: http://localhost:3000
# API:   默认仅在 Docker 网络内开放，由 WebUI 代理访问
```

首次初始化前，在 `config.toml` 任意结构块中临时写入 `setup_mode = true` 或 `SetupMode = true`，再打开 WebUI。若系统没有用户且没有管理员配置，会进入初始化向导。向导会创建首个管理员、写入 `[Admin].usernames` 与基础配置；Emby、Telegram 和邮箱等非必要项可以跳过，稍后在对应管理页继续完善。完成后初始化标记会从主配置中移除，入口会永久关闭。

完整说明见 [Docker 部署](docs/guides/docker.md)。

### Linux / systemd

生产部署建议使用 Linux + systemd + Nginx / HTTPS + PostgreSQL：

```bash
go build -o bin/twilight ./cmd/twilight
sudo bash deploy/setup-systemd.sh --dry-run
sudo bash deploy/setup-systemd.sh
```

完整流程见 [安装部署](docs/guides/install.md)。

## 本地开发

后端：

```bash
gofmt -w ./cmd ./internal
go test ./...
go build -o bin/twilight ./cmd/twilight
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml --debug
```

前端：

```bash
cd webui
pnpm install --frozen-lockfile
pnpm dev
pnpm lint
pnpm typecheck
pnpm build
```

更多约定见 [开发指南](docs/guides/development.md) 与 [模块化架构与解耦指南](docs/guides/modular-architecture.md)。

## 文档导航

| 文档 | 说明 |
| ---- | ---- |
| [文档中心](docs/README.md) | 全部指南、参考文档和功能专题入口 |
| [安装部署](docs/guides/install.md) | Linux、systemd、1Panel、Nginx、PostgreSQL 部署 |
| [Docker 部署](docs/guides/docker.md) | Docker / Docker Compose 部署指南 |
| [开发指南](docs/guides/development.md) | 目录结构、开发命令、API 与安全规范、发布流程 |
| [模块化架构与解耦指南](docs/guides/modular-architecture.md) | 分层边界、依赖方向、大文件拆分顺序与 review 清单 |
| [安全加固](docs/guides/security.md) | 生产安全基线、敏感信息处理和上线检查清单 |
| [Go 后端架构与配置](docs/reference/backend.md) | 后端架构、配置加载、环境变量、Redis、迁移 |
| [API 路由索引](docs/reference/api-index.md) | `/api/v1` 路由清单与鉴权级别 |
| [后端 API 详参](docs/reference/backend-api.md) | REST API 规范、认证、错误码、示例 |
| [API Key 外部接入](docs/reference/api-key.md) | 第三方集成与权限矩阵 |
| [开发者 JS 沙箱参考](docs/reference/developer-js.md) | Telegram Bot 自定义 JS 的内置对象、函数、权限边界与示例 |
| [邮箱验证](docs/features/email.md) | SMTP、验证码、强制绑定、找回密码和邮箱管理 |
| [注册码与卡码](docs/features/regcodes.md) | 注册码、续期码、白名单码算法和使用规则 |
| [邀请系统](docs/features/invite.md) | 邀请关系管理、级联删除与启停语义 |
| [邀请历史脏数据修复](docs/features/invite-data-repair.md) | 防止断开上下级后被旧关系或旧邀请码占用记录恢复 |
| [Telegram Bot 命令](docs/features/telegram-bot.md) | Bot 命令、权限边界、JS 自定义命令 |
| [Bangumi 同步](docs/features/bangumi.md) | Emby Webhook 与 Bangumi Token 配置 |

## 安全提示

### 开发者模式

开发者模式通过仪表盘输入 `DEBUGMODE` 并二次验证管理员密码开启；再次输入 `DEBUGMODE` 可关闭。关闭后已保存的 JS 预设和 Telegram 指令配置会保留，但所有 `js:` / `js:preset:<id>` 指令及相关 JS 交互会被服务端阻断。JS 沙箱使用 Goja，提供受控 `users.*`、`db.*`、`interactions.*`、`exit()`、`assert()`、受限 `fetch()` 和风险提示；不会暴露原始数据库、敏感配置、Token、密码、Telegram ID 或 Emby ID。完整接口见 [开发者 JS 沙箱参考](docs/reference/developer-js.md)。

- 生产环境请启用 HTTPS，并设置安全的 session cookie。
- 首次部署请在配置文件临时启用 `setup_mode = true` 后使用网页初始化向导，或直接在配置文件中明确指定管理员；普通首个注册用户不会自动成为管理员。
- Token、密码、API Key、数据库 URL 等敏感信息不要写入公开 issue、日志或截图。
- 配置查看和编辑必须依赖后台 schema 的脱敏逻辑，禁止明文回显 secret。
- 公开接口、验证码、发信、绑定码和卡码检查类接口应配置合理限流。

## 贡献

提交前建议至少执行：

```bash
gofmt -w ./cmd ./internal
go test ./...
cd webui && pnpm lint && pnpm typecheck && pnpm build
```

涉及架构、权限、配置、审计、缓存或外部副作用的改动，请先阅读 [模块化架构与解耦指南](docs/guides/modular-architecture.md)。

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

[![Star History Chart](https://api.star-history.com/svg?repos=Prejudice-Studio/Twilight&type=Date)](https://star-history.com/#Prejudice-Studio/Twilight&Date)

<div align="center">

如果 Twilight 对你有帮助，欢迎点一个 Star。

Made by [Prejudice Studio](https://github.com/Prejudice-Studio/)

</div>
