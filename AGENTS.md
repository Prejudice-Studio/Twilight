# Agent Instructions

本文件适用于整个仓库。后续 LLM/Agent 修改代码前必须先阅读本文件，并优先遵守这里的项目约定；若子目录未来出现更近的 `AGENTS.md`，以更近文件为准。

## 项目概览

- Twilight 是 Emby / Jellyfin 用户管理面板，当前主线是 Go 后端 + Next.js 前端。
- 后端模块路径为 `github.com/prejudice-studio/twilight`，入口在 `cmd/twilight`，目标部署环境是 Linux + systemd。
- 前端位于 `webui/`，使用 Next.js App Router、TypeScript、Tailwind CSS、Radix/shadcn 风格组件、Zustand 与 TanStack Query。
- 重要文档优先级：`docs/guides/development.md`、`docs/reference/backend.md`、`docs/reference/api-index.md`、`docs/reference/backend-api.md`、`docs/guides/security.md`、`README.md`。
- 若旧文件或本地笔记提到 Python 后端、SQLite 多库、uvicorn、`requirements.txt`、旧 `db/*.db` 迁移等内容，以当前 Go 源码和 `docs/guides/development.md` 为准，不要重新引入旧后端运行入口。

## 目录速览

- `cmd/twilight`：Go CLI 入口，支持 `api`、`all`、`scheduler`、`bot`、`version`。
- `internal/api`：HTTP 路由、鉴权、限流、统一响应、handler、外部服务 client、调度器和运维接口。
- `internal/api/routes.go`：全部后端路由的集中注册点。
- `internal/config`：读取 `config.toml`、`config.local.toml` 和 `TWILIGHT_*` 环境变量。
- `internal/store`：单一状态文档存储，支持 JSON 文件或 PostgreSQL。
- `internal/redis`：Redis RESP 客户端，用于会话与限流共享。
- `internal/security`：Token、密码哈希与安全随机数。
- `webui/src/app`：Next.js App Router 页面。
- `webui/src/lib/api-request.ts`：底层请求、凭据、超时与 `ApiError`。
- `webui/src/lib/api.ts`：前端 API 客户端，新增后端接口时通常要同步这里和 `api-types.ts`。
- `deploy/`：systemd unit 与安装脚本。部署 unit 必须指向 `bin/twilight`。

## 常用命令

后端：

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build -o bin/twilight ./cmd/twilight
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml --debug
```

启动脚本：

```bash
bash start_backend_dev.sh
bash start_backend_prod.sh
```

前端：

```bash
cd webui
pnpm install --frozen-lockfile
pnpm dev
pnpm lint
pnpm build
```

按改动范围选择验证命令。涉及 Go 代码至少运行 `gofmt` 和相关 `go test`；涉及前端或 API 客户端时运行 `pnpm lint`，必要时运行 `pnpm build`。如果本机缺依赖或外部服务不可用，必须在回复中明确说明未执行的验证和原因。

## 后端约定

- 新路由统一在 `internal/api/routes.go` 注册，使用 `a.add(method, pattern, auth, handler)` 声明方法、路径、鉴权级别和 handler。
- 鉴权级别为 `AuthPublic`、`AuthUser`、`AuthAdmin`、`AuthAPIKey`。管理员破坏性操作必须明确权限边界，优先支持 `dry_run`、确认短语和结构化结果。
- Handler 负责参数校验、鉴权上下文读取、调用业务逻辑和整理响应；可复用业务逻辑放在对应领域文件，外部服务调用走独立 client/helper，不要散落在 handler 中。
- 后端新增能力优先按业务域组织代码：路由只做入口，handler 只做 HTTP 适配，核心业务放在领域 service/helper，外部系统访问放在 client，持久化只通过 `internal/store` 暴露的方法完成。
- 保持低耦合：不要让业务逻辑直接依赖 `http.Request`、全局配置、全局 logger 或具体第三方 API 响应结构。需要这些信息时，由 handler 解析后以明确参数传入。
- 抽象要适度且有边界：只有在存在复用、需要替换外部依赖、需要隔离副作用或能显著降低测试成本时才引入接口/抽象；不要为了“架构感”增加空泛接口、过深目录或单方法包装层。
- 领域代码应优先表达业务概念和状态转移，避免把注册、续期、邀请、求片、Emby、Telegram、调度等不同领域混进同一个大函数或通用杂物文件。新增大功能时先选择清晰文件名，必要时拆成 `*_handlers.go`、`*_service.go`、`*_client.go`、`*_test.go`。
- 新人可维护性优先：函数命名要能说明业务动作；复杂流程拆成可顺序阅读的小步骤；非显而易见的权限、并发、回滚、远端副作用和确认短语必须用短注释说明原因。
- 避免循环依赖和隐式副作用。不要在包初始化、全局变量或 helper 中偷偷读取/修改配置、store、环境变量或网络资源；启动、reload、scheduler、bot 的生命周期由现有入口显式管理。
- 优化前先确认瓶颈。缓存、并发、批处理、后台任务和 Redis 使用必须有清晰失效策略、限流/超时和测试覆盖；不要为小路径引入难以验证的全局缓存或共享可变状态。
- JSON 响应必须使用统一 envelope：`success`、`code`、`message`、`data`、`timestamp`，失败响应应带稳定 `error_code`，与前端 `ApiError` / `errcode.ts` 保持兼容。
- Cookie 鉴权的变更类请求不再做 CSRF 令牌校验。鉴权依赖 HttpOnly session cookie、Bearer token 或 API Key；`X-Twilight-Client: webui` 只是允许的 CORS 头，不是鉴权手段。
- 运行时可热重载的 `cfg/store/sessions/limiter/redis` 通过 `runtimeState` 原子快照管理。读配置或 store 时优先使用 `a.cfg()`、`a.store()` 等访问器，不要缓存会跨 reload 失效的句柄。
- 配置入口固定为工作目录下的 `config.toml`；`--config` 只接受同一个工作目录的 `config.toml`。私密覆盖使用同目录 `config.local.toml` 或 `TWILIGHT_CONFIG_LOCAL_FILE`，环境变量以 `TWILIGHT_*` 覆盖字段。
- 存储模型是单一 `store.State` 文档。新增业务实体通常是在 `internal/store/store.go` 的 `State` 上加字段，并在 `ensure()` 中补默认值；不要为邀请、公告、注册码等业务重新创建独立 SQLite 文件或独立表。
- PostgreSQL 后端只把主状态存为 `twilight_state` 的单行 `jsonb`，并有独立 `twilight_sessions` 与 `twilight_runtime_logs`。迁移、备份、恢复必须保持快照完整性。
- JSON 后端是单进程独占，依赖 state 文件锁；多进程或多实例部署使用 PostgreSQL。
- 写入状态时复用 store 层已有原子写、备份、回滚和锁语义，不要绕过 `Store` 直接改 `db/twilight_go_state.json`。
- 上传、备份、恢复、迁移、Git 更新、systemd 操作等高风险路径必须做服务端边界校验。路径必须约束在允许目录内，拒绝绝对路径、`..`、符号链接和任意外部 URL。
- 执行外部命令必须使用 `exec.Command` 参数数组，禁止拼接 shell 字符串。Git remote URL、日志和响应中不得泄露凭据。

## 前端约定

- 所有后端调用集中维护在 `webui/src/lib/api.ts`，底层请求统一走 `webui/src/lib/api-request.ts`。不要在页面中散落裸 `fetch`，除非有明确理由并保持相同的 credentials、超时和错误处理语义。
- `apiRequest` 会自动使用 `${NEXT_PUBLIC_API_URL}/api/v1`；未设置 `NEXT_PUBLIC_API_URL` 时，`next.config.mjs` 将 `/api/*` rewrite 到 `BACKEND_URL`，默认 `http://localhost:5000`。
- 新增或调整接口时，同步检查 `api.ts`、`api-types.ts`、后端路由、请求方法、鉴权级别、错误码、确认短语和移动端展示。
- 页面分组：`webui/src/app/(auth)` 放登录、注册、找回密码；`webui/src/app/(main)` 放用户面板与管理页。新增 UI 优先复用 `webui/src/components`、`hooks` 和 `lib` 中已有模式。
- 保持现有中文文案、暗色/亮色主题、Tailwind token 和组件风格。前端改动必须兼顾桌面与移动端。
- 资产 URL、背景 CSS、头像、上传结果等必须沿用 `api.ts` 中的安全归一化逻辑，不允许保存任意外部 `url()` 或不受控协议。

## 安全与敏感信息

- 当前工作区的 `config.toml` 可能包含真实 Token、密码、API Key、Telegram ID、Emby 凭据等。不要在回答、日志、测试输出、文档或提交中复制这些值。
- 不要读取或展示非必要的运行数据：`db/`、`uploads/`、`memory/`、`tmp/`、`.venv/`、`bin/`、`twilight.exe` 等通常是本地运行产物或敏感数据来源。
- 示例配置使用 `.env.example`、`config.production.toml` 或占位符；新增文档中的密钥必须写成 `<PLACEHOLDER>` 或脱敏形式。
- 除一次性创建/重置密码、API Key 的既有响应外，不返回密钥明文。日志必须脱敏 `token`、`secret`、`password`、`api_key`、`Authorization`、`Cookie`、DSN 等字段。
- 生产 CORS 必须显式列出可信 HTTPS Origin，携带凭据接口不接受 `*`。信任代理头时必须配置可信代理 CIDR。
- HTTPS 生产环境保持 `session_cookie_secure = true`。本地 HTTP 调试才可显式关闭。

## 代码风格

- 遵守 `.editorconfig`：UTF-8、LF、文件末尾换行、去除尾随空白；Markdown 允许保留行尾空格。
- Go 代码必须 `gofmt`，保持小而清晰的函数边界，优先复用现有 store、api、config helper。
- Go 文件和函数要面向后续维护者组织：同一业务域的类型、校验、服务逻辑和测试尽量就近；跨领域复用的 helper 才放到更通用的位置。
- 包内私有函数优先于导出符号；只有跨包确实需要使用时才导出，并为非显而易见的导出类型/函数补充说明。
- 测试应覆盖业务边界而不是只覆盖实现细节。涉及 store 状态转移、权限分支、确认短语、外部 client 降级、并发锁和回滚时，优先写表驱动或聚焦单元测试。
- TypeScript/React 使用 2 空格缩进，遵守现有 ESLint/Next 配置。不要无必要引入新状态库、UI 库或工具函数。
- 注释只解释非显而易见的安全、并发、迁移或兼容原因，避免重复代码字面含义。
- 保持最小正确改动。不要为没有实际需求的旧行为添加兼容层。

## 验证与交付

- 后端改动提交前检查：`gofmt -w ./cmd ./internal`、`go test ./...`、`go vet ./...`。
- 前端改动提交前检查：`cd webui && pnpm lint`，涉及构建、路由、Next 配置或 API 契约时运行 `pnpm build`。
- 涉及鉴权、上传、路径、配置保存、数据库迁移、Git 更新、实时日志、限流或会话的改动必须补充聚焦测试或明确说明无法测试的边界。
- 不要修改用户未要求的本地配置、数据库、上传文件、生成产物或部署状态。不要执行破坏性命令，不要重置工作区，不要替用户提交或推送，除非用户明确要求。
