# 开发工作流

本分支是 Go 后端分支，后端源码以 `cmd/` 和 `internal/` 为准。

## 后端

- 执行检查：`go test ./...`。
- 本地启动：`bash start_backend_dev.sh`。
- 构建生产二进制：`go build -o bin/twilight ./cmd/twilight`。
- 生产模式启动：`bash start_backend_prod.sh`。

## 前端

- 在 `webui/` 安装依赖：`pnpm install --frozen-lockfile`。
- 前端 API 调用统一维护在 `webui/src/lib/api.ts`。
- 新增后端接口时，需要在 `internal/api/routes.go` 注册路由；涉及鉴权、文件、路径、密钥、迁移或共享行为时补充聚焦测试。

## 安全基线

- 生产环境优先配置 Redis，用于共享会话和限流计数。
- 破坏性管理操作必须保留明确确认步骤或 dry-run 预检。
- 除一次性生成的密码或 API Key 创建/重置响应外，不返回密钥明文。
- 上传和资产读取必须使用 `http.MaxBytesReader`、MIME 白名单、目录约束和统一响应 envelope。
- 数据库备份、恢复、迁移、Git 更新和 systemd 操作不得拼接 shell 字符串。

## 发布检查

- `go test ./...`
- `go vet ./...`
- 前端或 API 客户端变更后，在 `webui/` 执行 `npm run lint` 和 `npm run build`。
- 确认 `start_backend_prod.sh` 和 `deploy/*.service` 指向 `bin/twilight`，没有重新引入旧后端运行入口。
