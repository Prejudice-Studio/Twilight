# 前端开发

前端是位于 `webui/` 的 Next.js 应用。

## 本地启动

```bash
cd webui
pnpm install --frozen-lockfile
pnpm dev
```

后端单独启动：

```bash
bash start_backend_dev.sh
```

## API 契约

- 前端 API 调用统一维护在 `webui/src/lib/api.ts`。
- 后端路由统一注册在 `internal/api/routes.go`。
- 响应结构保持 `{ success, code, message, data, timestamp }` envelope。
- 新增或调整接口时，同时检查前端调用路径、请求方法、鉴权等级、错误提示和移动端展示。

## 验证

- UI 或 API 客户端变更后执行前端 lint/build。
- 后端变更后执行 `go test ./...` 和 `go vet ./...`。
- 涉及鉴权、上传、路径、配置保存、数据库迁移、Git 更新或实时日志时，补充安全边界测试。
