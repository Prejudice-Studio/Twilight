# 开发指南

## 环境要求

- Go 1.25 或更新版本。
- Node.js 与 pnpm。
- PostgreSQL 可选；本地开发可使用 JSON 状态文件。

## 目录结构

| 路径 | 说明 |
| ---- | ---- |
| `cmd/twilight` | Go CLI 入口 |
| `internal/api` | 路由、鉴权、handler、外部 client、调度器和管理接口 |
| `internal/config` | TOML 与环境变量配置 |
| `internal/store` | 状态模型和持久化 |
| `internal/security` | Token、密码哈希和安全随机数 |
| `webui` | Next.js 前端 |
| `docs` | 中文项目文档 |

## 后端开发

```powershell
go test ./...
go vet ./...
go run ./cmd/twilight api
```

新增接口通常需要：

1. 在 `internal/api/routes.go` 注册路由。
2. 在对应领域 handler 文件中实现逻辑。
3. 如前端调用，同步 `webui/src/lib/api.ts` 和 `api-types.ts`。
4. 更新 `docs/reference/api-index.md` 与相关功能文档。

## 前端开发

```powershell
cd webui
pnpm install
pnpm lint
pnpm build
pnpm dev
```

前端页面应优先使用 `webui/src/lib/api.ts`，不要在业务页面中裸写 `fetch` 调用项目 API。

## i18n 约定

- 简体中文基底：`webui/src/locales/basic.json`。
- 繁体中文：`zh-Hant.json`。
- 英文：`en-US.json`。
- `zh-Hans.json` 保持稀疏，缺失键回退到 `basic.json`。

新增用户可见文案时，必须同步三个主要语言文件，并检查按钮、下拉框和工具栏在不同语言下不会挤压变形。

## 提交前检查

大范围改动建议完整运行：

```powershell
go test ./...
go vet ./...
cd webui
pnpm lint
pnpm build
```

文档和语言文件改动后，额外扫描是否出现连续问号、替换字符或常见 mojibake 片段。
