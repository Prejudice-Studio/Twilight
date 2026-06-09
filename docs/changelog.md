# 版本历史

本文按版本从新到旧记录 Twilight 的主要变更；文末附发布检查清单。术语与跨文档引用见 [文档导航](../README.md)。

## 未发布（开发中）

- **邮箱管理页（新增）**：独立管理页 `(main)/admin/email`，集中查看所有邮箱验证记录——在用验证码（脱敏，绝不含验证码/哈希）与已绑邮箱账号及验证状态，含统计、搜索、按验证状态归类、撤销单条验证码、清理过期。接口 `GET /admin/email/verifications`、`POST .../cleanup`、`DELETE .../:id`。
- **设备 / IP 审查独立页（新增 + 修复）**：从「Emby 管理」拆出独立页 `(main)/admin/device-audit`，按 Emby 用户聚合设备数量、去重登录 IP 与完整网页/Emby/Telegram 账号，支持归类/排序/搜索。接口改为 `GET /admin/emby/device-audit`；新增 `parseRemoteIP` 修复 `RemoteEndPoint` 带端口、IPv6 被截断导致读不到登录 IP 的问题，并从活动日志补历史登录 IP（离线设备也能审查来源）。详见 [安全加固 §10](./guides/security.md)。
- **邮箱验证与找回密码（新增）**：SMTP 发信、绑定/验证邮箱、登出态找回密码、改密二次校验、强制绑定门。验证码只存 HMAC 哈希 + 常量时间比较 + 尝试上限/TTL + 多维限流（IP/uid/收件地址）+ 重发冷却；强制门为不可绕过的服务端硬门，SMTP 未配齐时自动失效。配置见 `[Email]`、`[SAR]` 名单、`[RateLimit]`。详见 [邮箱验证](./features/email.md)。
- **注册码 / 续期码管理增强**：`PUT /admin/regcodes/:code` 扩展为部分更新（启用/有效期/天数/次数上限），DTO 新增 `expires_at`。详见 [注册码](./features/regcodes.md)。
- **Telegram 用户名自动刷新**：Bot 处理已绑定用户的任意更新时被动刷新 `telegram_username`（无额外 API 调用）。详见 [Telegram Bot](./features/telegram-bot.md)。
- **本地登录设备记录修复**：登录改用 `UpdateDevice` 读改写，保留信任/封禁标记与首登时间；新增 `LastIP` 与可选设备数上限 `[DeviceLimit]`。
- **配置收敛**：功能配置统一放 `config.toml`（新增 `[RateLimit]`），密钥放 `config.local.toml`；`.env` 仅保留监听地址/站点名。
- **管理员邀请树性能**：渲染由每行 O(n²) 优化为一次 O(n) 预计算查表。

## 0.0.4 - 2026-05-23（当前）

Go 后端重构版：生产主线正式切换到 Go 后端（`twilight api / all / bot / scheduler`），旧 Python 后端退出生产路径。

- **架构**：业务按域拆分（Emby、TMDB、Bangumi、求片、邀请、卡码、调度、数据库、运维），外部调用收敛到独立 client；`internal/api` 按 handler/client/service/运维模块维护。
- **数据库与迁移**：默认推荐 PostgreSQL（保留 Go JSON 兼容选项）；新增数据库状态、备份、恢复、迁移预检与执行接口；支持旧 SQLite 只读迁移（用户/卡码/邀请/公告/求片/播放/调度/Telegram 花名册）；恢复与迁移前强制保护性备份 + 预览 + 二次确认；备份/迁移路径限制在指定目录、拒绝符号链接与穿越。
- **可观测性**：实时日志页 + 服务器/Go Runtime/内存/主机/数据库/Redis 状态；日志脱敏 Token/Cookie/密码/API Key/DSN；日志等级与保留行数热重载。
- **安全加固**：凭据型 CORS 拒绝通配符；Cookie 写请求依赖有效会话/Bearer/API Key（不再要求额外令牌）；非系统管理员绑定 Emby 管理员账号即被限制敏感操作并从源头禁止绑定；自助续期必须消耗有效注册码；上传/背景/备份恢复/Git 更新强化路径与类型校验；最后一个活跃管理员受保护。
- **违规审计（新增）**：诱饵码与指名码越权检测 + 处罚动作（`disable_user`/`disable_emby`/`log_only`）；`/admin/violations` 管理页与接口。
- **邀请与卡码**：邀请码支持指名使用；邀请制用户到期保留登录以便自助续期；Emby 容量计入未使用邀请码防超发。
- **部署与 CI**：Linux systemd 一键设置脚本（unit 指向 `bin/twilight`，API/Bot/Scheduler 分服务）；Git 更新支持 dry-run、分支校验、脏工作区保护与 `--ff-only`；CI 迁移到 Go（多平台 `go test -race` + `go vet` + govulncheck + Nix 检查）。
- **配置**：固定使用运行目录 `config.toml`，`config.local.toml` 私密覆盖，`TWILIGHT_*` 环境变量覆盖。

> 频道 <https://t.me/Twilightpanel> · 交流群 <https://t.me/TwilightPanelChat> · 仓库 <https://github.com/Prejudice-Studio/Twilight>。升级前请备份配置、数据库与上传目录；旧 SQLite 用户先用后台数据库迁移页预检后再执行；生产建议 PostgreSQL + HTTPS + 明确 CORS Origin。

## 0.0.3

- 新增 Bangumi 同步流程并对齐前端状态显示。
- 邀请码行为更细粒度配置；使用码（注册/续期/白名单/邀请）支持消费前预览。
- 加强接口限流、错误响应一致性，以及 Emby 密码/会话/容量校验。

## 0.0.2

- 公网部署安全护栏：代理感知的客户端 IP 识别、Redis 限流与会话（支持多实例共享状态）。
- SQLite pragma + 事务化注册、注册队列、过期清理、待补建 Emby 处理与注册权益补发。
- 注册码格式/随机算法/诱饵码隐藏/使用队列；通过注册码授予 Emby 权益、重置、过期与取消永久。
- 新增 Git 自动更新 API 与管理端入口。

## 0.0.1

- 项目基础：注册、登录、角色检查、管理员用户管理、Emby 绑定/注册；dashboard、设置、用户、注册码、邀请、Emby、公告、服务信息等页面。
- Telegram Bot 集成、自定义消息与群组管理员工具。
- 邀请码/注册码/续期码/白名单码与早期管理员运维流程；中文项目说明与生产启动指导。

## 发布检查清单

- 更新版本号：`cmd/twilight/main.go`、`internal/config/config.go`、`webui/package.json`、`webui/package-lock.json`。
- 后端：`gofmt -w ./cmd ./internal`、`go test ./...`、`go vet ./...`。
- 前端（涉及前端或 API 客户端时）：`cd webui && pnpm lint`，必要时 `pnpm build`。
- 发布前扫描敏感信息、旧后端残留、路径穿越、文件类型白名单与鉴权边界。
