# 开发维护

Twilight 当前开发和部署目标均为 Linux。systemd 集成由 `deploy/setup-systemd.sh` 维护。

## 目录结构

| 路径 | 说明 |
| ---- | ---- |
| `cmd/twilight` | Go 后端入口，提供 `api`、`all`、`scheduler`、`bot` 等子命令。 |
| `internal/api` | HTTP 路由、认证、限流、会话、统一响应、业务 handler、外部服务 client 和运维接口。 |
| `internal/api/*_client.go` | Emby、TMDB、Bangumi、Telegram 等外部服务客户端。 |
| `internal/api/*_handlers.go` | 按功能域拆分的 HTTP handler，例如求片、邀请、注册码、调度、数据库和系统更新。 |
| `internal/store` | JSON 和 PostgreSQL 状态存储。 |
| `internal/config` | TOML 与 `TWILIGHT_*` 环境变量配置加载。 |
| `internal/security` | 密码哈希、安全随机数和兼容校验。 |
| `webui` | Next.js 前端。 |

## 后端命令

```bash
go test ./...
go vet ./...
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml --debug
go build -o bin/twilight ./cmd/twilight
```

## 前端命令

```bash
cd webui
pnpm install --frozen-lockfile
pnpm lint
pnpm build
```

## API 开发规范

- 新路由统一添加到 `internal/api/routes.go`。
- handler 只负责参数校验、鉴权、调用服务和整理响应，复用逻辑放到功能域文件中。
- Emby、TMDB、Bangumi、Telegram 等外部服务调用必须通过独立 client/helper，不要直接散落在 handler 内。
- 公开接口、登录接口、验证码/绑定码/邀请码检查接口必须考虑限流。
- 管理员破坏性操作必须有明确权限边界，并尽量返回结构化的 `skipped`、`failed`、`details` 信息。
- 响应 envelope 需要与 `webui/src/lib/api.ts` 保持兼容。

## 文件与路径安全规范

- 上传文件必须使用 `http.MaxBytesReader` 和 `io.LimitReader` 双层限制大小。
- 上传文件类型必须以内容探测结果为准，不信任用户上传的文件名和扩展名。
- 可读取的上传资源文件名必须是服务端生成的白名单格式。
- 用户背景配置只能保存安全渐变表达式和本系统上传的背景资源，不允许保存任意外部 URL、`url()` 注入或复杂 CSS 函数。
- 所有由请求参数参与的文件路径都必须经过 `filepath.Abs`、`filepath.Rel` 和目录约束校验。
- 备份恢复只允许读取备份目录内的普通 `.json` 文件，禁止绝对路径、`..`、子目录跳转和符号链接。
- 数据库迁移到 JSON 时，目标文件必须在数据库目录内且扩展名为 `.json`。
- 数据库恢复和迁移这类高风险操作必须实现预览、二次确认和操作前备份；后端不能只依赖前端确认弹窗。
- Git 更新、systemd 设置和其他命令执行必须使用参数数组，不允许拼接 shell 命令字符串。
- Git 更新 URL 必须拒绝凭据、query string 和 fragment，避免把 token 写入 remote 或响应日志。

## 数据模型

默认状态文件为 `db/twilight_go_state.json`，也可使用 PostgreSQL。

更换存储后端前，必须先调用 `/api/v1/system/admin/database/migrate` 并传入 `dry_run=true`。预检会返回实体数量、快照大小、目标连通性和重启/配置告警。

旧部署迁移应使用显式的一次性导入流程，不应在启动时隐式修改或猜测旧数据。

## 更新流程

管理员 Git 更新接口支持 `dry_run` 预检，默认拒绝脏工作区。实现必须保持 `exec.Command` 参数化调用，禁止引入 shell 字符串拼接。

## systemd 流程

- 安装前先执行 `sudo bash deploy/setup-systemd.sh --dry-run`。
- 脚本会检测路径、配置、二进制、用户/组、端口、空白/`%` 等 systemd 特殊字符和旧 Python 版 Twilight unit。
- 部署 unit 必须指向 `bin/twilight`，不要重新引入旧后端启动命令。

## 发布前检查

- `gofmt` 已执行。
- `go test ./...` 已通过。
- `go vet ./...` 已通过。
- 前端相关变更已执行 lint 和 build。
- 已扫描敏感信息。
- 已扫描旧后端残留。
- 已检查鉴权、路径穿越、文件类型白名单和 CORS 配置。
