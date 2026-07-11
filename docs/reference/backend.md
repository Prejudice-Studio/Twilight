# Go 后端参考

## 运行入口

后端模块路径为 `github.com/prejudice-studio/twilight`，CLI 入口位于 `cmd/twilight`。

常用命令：

- `api`：启动 HTTP API 与 WebUI 服务。
- `scheduler`：启动调度器。
- `bot`：启动 Telegram Bot。
- `all`：组合模式。
- `version`：输出版本。

## 配置来源

配置按以下顺序加载并覆盖：

1. `config.toml`
2. `config.local.toml`
3. `TWILIGHT_*` 环境变量

环境变量优先级最高。真实密钥应放在本地配置或环境变量中，不应进入 Git。

## 存储模型

Twilight 使用单一状态文档保存业务状态，可落在 JSON 文件或 PostgreSQL 中。会话、运行日志等可使用独立存储路径。

主要状态域包括：

- 用户、角色、到期时间、Emby 绑定、Telegram 绑定和 Bangumi Token。
- RegCode、InviteCode、邀请关系与使用记录。
- 审计日志、违规日志、登录日志和设备记录。
- 公告、求片、工单、调度记录和 Bangumi 缓存。
- Emby ActivityLog 播放统计记录。

## 路由

全部路由集中注册在 `internal/api/routes.go`。当前路由表见 [API 路由索引](./api-index.md)。

## 调度器

调度器相关代码位于 `scheduler_handlers.go`、`scheduler_daemon.go`、`scheduler_runner.go` 与 `admin_jobs.go`。管理员可在后台调度器页面查看和执行任务。

## 运行日志

运行状态与日志接口位于 `/system/admin/runtime/*`，前端入口为管理员日志页面。
