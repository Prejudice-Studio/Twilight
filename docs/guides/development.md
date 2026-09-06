# 开发指南

本文面向贡献者与维护者，覆盖 Twilight 的目录结构、后端与前端的本地开发流程、API 与安全编码规范、数据模型与迁移约定，以及验证与发布流程。Twilight 的当前开发与部署目标都是 Linux + systemd；同时提供完整的 Docker 支持。

相关文档：

- 安装部署见 [安装部署](./install.md)。
- Docker 部署见 [Docker 部署](./docker.md)。
- 安全加固见 [安全加固](./security.md)。
- 模块边界与渐进式解耦见 [模块化架构与解耦指南](./modular-architecture.md)。
- 后端架构与配置见 [Go 后端架构与配置](../reference/backend.md)。
- 路由总览见 [API 路由索引](../reference/api-index.md)，单接口细节见 [后端 API 详参](../reference/backend-api.md)。

## 目录结构

| 路径 | 说明 |
| ---- | ---- |
| `cmd/twilight` | Go 后端入口；解析子命令 `api` / `all` / `scheduler` / `bot` / `version`。 |
| `internal/api` | HTTP 路由、鉴权、限流、会话、统一响应 envelope、业务 handler、外部服务 client 与运维接口。 |
| `internal/api/routes.go` | 全部路由的集中注册点（含方法、鉴权级别、handler）。 |
| `internal/api/*_client.go` | Emby、TMDB、Bangumi、Telegram 等外部服务客户端。 |
| `internal/api/*_handlers.go` | 按功能域拆分的 HTTP handler，例如求片、邀请、注册码、调度、数据库与系统更新。 |
| `internal/store` | 状态存储层：唯一运行后端 PostgreSQL，定义单一状态文档 `State`；`Store` 仅经 `store.OpenPostgres` 构造。 |
| `internal/config` | TOML 配置与 `TWILIGHT_*` 环境变量加载。 |
| `internal/security` | 密码哈希、安全随机数与兼容校验。 |
| `webui-v2` | 默认 SvelteKit SSR + adapter-node 前端应用；`webui` 仅保留为整站紧急回滚版本。 |
| `webui-v2/src/lib/server/api.ts` | V2 SSR 服务端 API 客户端，集中维护 Cookie 转发、响应上限和同源代理。 |
| `start_backend_dev.sh` / `start_backend_prod.sh` | 后端本地启动脚本（开发 / 生产）。 |
| `deploy/` | systemd unit 与安装脚本（`setup-systemd.sh`）。 |

后端二进制构建产物固定为 `bin/twilight`。

## 后端开发

### 常用命令

```bash
# 单元测试与静态检查
go test ./...
go vet ./...

# 格式化（提交前必须执行）
gofmt -w ./cmd ./internal

# 直接以源码运行 API 服务
go run ./cmd/twilight api --host 0.0.0.0 --port 5000 --config config.toml --debug

# 构建生产二进制
go build -o bin/twilight ./cmd/twilight
```

### 本地启动脚本

```bash
# 开发模式：自动追加 --debug，按 TWILIGHT_GO_BIN → ./bin/twilight → go run 顺序启动
bash start_backend_dev.sh

# 生产模式：先尝试抬高 NOFILE 上限，再启动（无 --debug）
bash start_backend_prod.sh
```

两个脚本的行为约定：

- 监听地址来自环境变量 `TWILIGHT_API_HOST`（默认 `0.0.0.0`）与 `TWILIGHT_API_PORT`（默认 `5000`）。
- 配置文件固定为工作目录下的 `config.toml`；运行时不接受指向其他路径或其他文件名的 `--config`（见 `cmd/twilight/main.go` 的 `runtimeConfigPath`）。
- 二进制选取优先级：环境变量 `TWILIGHT_GO_BIN` 指定的可执行文件 → `./bin/twilight` → 回退到 `go run ./cmd/twilight`。
- `start_backend_prod.sh` 额外尝试把 `NOFILE` 抬高到 `TWILIGHT_NOFILE_LIMIT`（默认 `65535`）；抬不动时打印告警，提示改由 systemd `LimitNOFILE` 或容器 ulimit 设置。

### 子命令

后端入口 `cmd/twilight/main.go` 支持以下子命令（不带子命令时等价于 `api`）：

| 子命令 | 作用 |
| ---- | ---- |
| `api` | 仅启动 HTTP API 服务。 |
| `all` | 在同一进程内并行启动 API、调度器（scheduler）与 Telegram Bot。 |
| `scheduler` | 仅启动后台调度器。 |
| `bot` | 仅启动 Telegram Bot；未启用或未配置 token 时会循环等待配置生效。 |
| `version` | 打印版本号并退出（`--version` / `-v` 同义）。 |

`api` 与 `all` 支持 `--host`、`--port`、`--config`、`--debug` 标志；`scheduler` 与 `bot` 仅支持 `--config`。`--debug` 会把日志级别提升到 debug。

> `all` 模式下，Telegram Bot 在未配置 token 时会立即正常返回（return nil），此时进程进入「API + 调度器继续运行、Bot 不参与」的模式，不会拖垮其他服务。这是设计行为，不是错误。

### Telegram Bot 协议性能回归

Telegram JSON API 的统一协议层位于 `internal/api/telegram_transport.go`，入站及身份、聊天、成员 DTO 位于 `internal/api/telegram_update_types.go`。`getUpdates`、`getMe`、`getChat`、`getChatMember` 与 `getChatAdministrators` 必须直接返回对应窄 DTO，不能恢复嵌套动态 map；批次调度只传索引和原切片元素指针。稳定请求字段使用固定 DTO，普通写操作使用不保留 `result` 的丢弃类型。修改该路径后，应在配置好 `TWILIGHT_TEST_DSN` 的环境运行：

```bash
go test -run '^$' -bench '^(BenchmarkTelegramUpdateEnvelopeDecode|BenchmarkTelegramChatAdministratorsDecode|BenchmarkTelegramPanelTemplateRender)$' -benchmem -count=3 ./internal/api
```

更新与成员基准都保留动态/强类型及交错对比，模板基准保留旧 `strings.Replacer` 与单次扫描对比。CPU 睿频会让顺序运行的绝对 `ns/op` 漂移，应以 `interleaved_compare` 的 `typed/dynamic` 比率，以及各强类型路径的 `B/op` / `allocs/op` 为主要证据。还必须运行完整 `internal/api` 真库测试，验证 message/callback/chat/member 字段、配置管理员索引刷新、模板兼容、批次顺序、请求字段、父级 deadline、4 MiB 响应边界、429 退避和 Bot Token 脱敏。

## 前端开发

默认前端位于 `webui-v2/`，使用 SvelteKit SSR + adapter-node；`webui/` 的 Next.js 实现仅保留为整站紧急回滚和行为对照，不与 V2 共享运行时页面状态。

V2 管理页面的页面级标题与主要分区使用 `webui-v2/src/lib/components/PageHeader.svelte` 和 `Panel.svelte`。这两个组件是无状态的 SSR 结构基线，只负责语义标题、边界和窄视口换行；业务页不得重新定义一套全局标题/面板样式，领域样式仍留在对应路由内。用户管理与工单管理已按此方式迁移，写操作仍只通过 SvelteKit form action 完成。

### 常用命令

```bash
cd webui-v2

# 安装依赖（锁定 lockfile）
pnpm install --frozen-lockfile

# 本地开发服务器
pnpm dev

# Svelte 类型检查
pnpm check

# adapter-node 生产构建
pnpm build
```

生产预览使用 `pnpm preview`，正式运行使用 `node build`。V2 不使用 Next.js rewrites、浏览器端全局身份 store 或 V1 React 页面状态。

V2 服务端通过 `BACKEND_URL` 访问 Go API（默认 `http://127.0.0.1:5000`），并在服务端转发请求 Cookie；部署 adapter-node 时由 `ORIGIN`、`HOST`、`PORT` 等运行环境变量决定外部地址。`/api/v1/*` 与 `/api/v2/*` 的同源代理只为后续渐进增强和兼容调用提供传输通道，不能替代 Go 后端鉴权。V2 登录和登出使用 SvelteKit form action，身份读取放在服务端 `load`，首屏不依赖浏览器启动后再拉取 `/users/me`。管理员 Emby 页面 `/(app)/admin/emby` 使用同一边界：账号、设备/IP 审查和活动日志按页签按需读取，管理写操作通过服务端 action 转发，避免浏览器持有凭据或重复请求大型列表。

后端可单独启动配合调试：

```bash
bash start_backend_dev.sh
```

### 前后端联调与环境变量

旧 V1 前端的环境变量仅用于紧急回滚。V2 通过 `webui-v2/.env.example` 配置服务端运行时：

- `BACKEND_URL` 只在 V2 服务端使用，默认 `http://127.0.0.1:5000`，不会成为浏览器公开环境变量。
- `HOST`、`PORT`、`ORIGIN` 控制 adapter-node 监听地址、端口和浏览器实际访问 Origin；反向代理后 `ORIGIN` 必须填写外部 HTTPS Origin。
- V2 的 form action 和 SSR `load` 只在服务端转发会话 Cookie，浏览器不读取 `/users/me` 作为全局身份缓存，也不持有后端 Bearer/API Key。

### V2 SSR 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BACKEND_URL` | `http://127.0.0.1:5000` | 仅由 SSR 服务端访问 Go API |
| `HOST` | `127.0.0.1` | adapter-node 监听地址 |
| `PORT` | `3001` | adapter-node 监听端口 |
| `ORIGIN` | `http://127.0.0.1:3001` | 浏览器实际访问的完整 Origin |
| `SESSION_COOKIE_NAME` | `twilight_session` | 后端会话 Cookie 名称，只有改名时才需要同步 |

完整示例见 `webui-v2/.env.example`。`BACKEND_URL`、Cookie 和密码等值不得使用 `PUBLIC_` 前缀，也不得写入浏览器端代码。

### 前端文案与多语言

- V2 的文案入口位于 `webui-v2/src/lib/i18n.ts`，当前由服务端安全渲染的简体中文消息表提供稳定键名。
- V1 的 `webui/src/locales/` 仅供旧版回滚维护；新 V2 页面不得重新依赖 V1 React locale 或客户端 store。
- 新增文案、翻译或语言时，按 [前端多语言开发与翻译指南](./i18n.md) 操作。

### 前端契约

- V2 所有 SSR 后端调用集中在 `webui-v2/src/lib/server/api.ts`；页面 `load` 和 form action 是服务端数据边界，浏览器端不得直接持有后端 Bearer/API Key。
- V2 页面不使用 V1 `useAsyncResource`、React store 或客户端全局请求缓存；重读通过导航、form action 返回或明确的手动刷新完成，不恢复无边界轮询。
- 响应统一为 envelope 结构 `{ success, code, message, data, timestamp }`；前端按 HTTP 状态码与 `error_code` 分流处理（401 跳登录、403 权限提示、429 退避、5xx 通用故障，以及自定义业务 error_code）。
- 新增或调整接口时，需同步检查前端调用路径、请求方法、鉴权等级、错误提示文案与移动端展示。
- 登录支持用户名和邮箱两种方式：V2 登录 form action 将输入交给后端统一判断；旧 V1 `api.ts` 的 `login()` 仍兼容自动检测 `@`。
- 认证页、用户页和管理员页按 `webui-v2/src/routes` 分组；新增页面优先使用 server `load`、form action、原生可访问控件和局部 CSS，不把页面改回客户端 SPA。
- 求片搜索结果必须保持接口返回的搜索顺序；图片加载完成后按自然尺寸分成横版封面与竖版海报两个分区，分区内继续保持原顺序。卡片图片使用受控的横版 / 竖版比例框与 `object-contain`，确保完整显示图片、不裁切，也不让横竖比例混在同一网格中。
- 后台总入口为 `webui-v2/src/routes/(app)/admin/+page.svelte`（管理导航）。迁移出的配置模块必须有独立管理页：邮箱管理、Telegram 管理、邀请系统管理、安全中心；配置管理只保留默认折叠的兼容入口和跳转提示。
- 独立管理页若需要编辑配置，必须通过 V2 form action 复用 `/system/admin/config/schema` 的后端契约，写回同一个 `config.toml`；不要在前端或 store 中复制第二套配置源。旧 V1 客户端仍使用 `api.updateConfigBySchema()` 兼容同一接口。
- 用户管理页的单用户与批量操作必须按领域分组展示（账号状态、Emby、身份绑定、注册资格、危险操作），避免把所有操作平铺成过长菜单或按钮栏；新增用户操作时同步维护后端返回的 `admin_action_state` 与前端 `UserInfo` 类型，让前端能显示禁用原因。
- 用户管理的单用户操作菜单、桌面表格和危险清理预览必须使用受限 `dvh` Firefox 滚动区域；桌面表头保持可见，手机上的预览表允许横纵滚动。继续使用服务端分页和移动端卡片，不要把完整用户库一次挂载到浏览器。
- 用户管理筛选变化必须只加载第一页，不能先请求已经失效的旧页；每页数量变化要清除跨页选择。选择“拥有 Emby 的用户”时，当前页全选只统计当前页已绑定 Emby 的行。
- Emby 管理与设备/IP 审查共用 `webui-v2/src/routes/(app)/admin/emby`，设备审查是页签级入口；`/admin/device-audit` 只作为兼容直达页面保留。设备审查不展示 Twilight 自身连接 Emby 时产生的设备/会话；全量 Emby 设备记录清理只放在调度器 `cleanup_emby_devices`，不要在审查页重新添加“清理全部/踢出全部”入口。
- 工单页必须展示 `replies` 双方回复时间线；`admin_note` 仅作为最新管理员摘要和旧数据兼容字段。状态、优先级、类型归一、关闭/重开时间戳、开放工单计数与“更新时保留 replies/附件”逻辑统一放在 `internal/store`，前端和 handler 不要重复判断。
- 管理员工单列表是处理队列，只返回并展示回复数、图片数、正文摘要和内部备注摘要；完整 `replies`、附件 URL、图片预览与双方对话只在单工单详情接口和对话页加载，避免历史消息随列表分页重复传输和渲染。
- 邮箱管理的验证码与邮箱账号列表使用后端搜索和分页；前端必须传 `view=pending|accounts`，切换筛选、分页或页签时取消旧请求。手机和平板显示信息卡片，桌面显示可滚动表格；进入邮箱配置页签不额外请求两份列表。无 `view` 的全量响应只保留给旧客户端兼容。
- 注册码管理的自由搜索使用 250ms 防抖后才请求后端；类型、状态、来源、排序和顺序改变都回到第一页。注册码表格、生成结果和使用记录弹窗使用 Firefox 兼容滚动容器，不能恢复为每次按键请求或无边界撑高页面。批量选择、复制、导出和删除在手机上使用两列按钮网格，不能让多个 `flex: 1` 文本按钮在同一行缩成逐字竖排。
- 注册码的复制、启停、编辑、删除、备注保存、使用记录和分页等纯图标操作必须提供本地化 `aria-label`；`title` 只能作为鼠标提示，不能替代可访问名称。
- 审计日志和违规审查列表必须使用 `cache: no-store` / `cacheRead: false` 并传递 `AbortSignal`；筛选、分页或路由切换取消旧请求。违规页顶部危险按钮和记录元数据在手机 Firefox 中允许换行，不能把内容挤出视口。
- 后台重型面板应按需加载：非默认页签不要在首屏自动请求大接口；公共系统信息走 `useSystemStore.fetchInfo()` 的 TTL 与 inflight 复用，配置保存后调用 `invalidate()`。
- 后台弹窗和覆盖层优先复用现有 Radix 公共组件与 CSS 过渡；未引用的客户端组件应及时删除，不能让一次性抽屉或面板残留重型动画运行时依赖。
- 管理首页和统计页不加载 Framer Motion，也不使用装饰性渐变圆形；这两个页面优先保证首屏包体、稳定尺寸和窄屏可读性，交互反馈使用 CSS/Tailwind。
- V2 管理列表、详情和筛选结果通过 URL 状态和 SSR `load` 读取；form action 完成写入后以重定向或重新读取获得权威状态，不恢复客户端轮询或全局数据 store。旧 V1 页面才使用 `useAsyncResource` / `AbortSignal` 取消过期读取。
- 侧边栏、移动菜单、管理导航等密集导航区域使用 `Link prefetch={false}`，避免首屏预载大量不一定访问的后台页面 chunk；只对明确的高频下一步保留预取。

## API 与安全规范

### 路由与 handler 约定

- 新路由统一在 `internal/api/routes.go` 注册，通过 `a.add(method, pattern, auth, handler)` 声明方法、路径、鉴权级别和 handler；按功能域分布在 `registerAdminRoutes` / `registerAPIKeyRoutes` / `registerSecurityRoutes` / `registerBatchRoutes` 等分组函数中。
- handler 只负责参数校验、鉴权、调用服务和整理响应；可复用的业务逻辑放到对应功能域文件，外部服务调用必须走独立 client/helper（Emby、TMDB、Bangumi、Telegram），不要散落在 handler 内。
- 响应必须使用统一 envelope，并与 V2 `webui-v2/src/lib/server/api.ts` 及保留的 V1 客户端保持兼容。
- JSON 请求体必须经统一解码器读取，限制为单个 JSON 值、256 KiB 和 32 层嵌套；不得只解码第一个值后忽略尾随第二份 JSON 文档。
- 公开接口、登录接口，以及验证码 / 绑定码 / 邀请码 / 注册码检查类接口必须考虑限流。
- 管理员的破坏性操作必须有明确权限边界，并尽量返回结构化的 `skipped`、`failed`、`details` 等字段，便于前端展示处理结果。
- 涉及鉴权、文件、路径、密钥、迁移或共享行为的改动，必须补充聚焦测试。

### 鉴权级别

路由的鉴权级别（`internal/api/app.go` 中的 `AuthLevel`）有四种：

| 级别 | 含义 |
| ---- | ---- |
| `AuthPublic` | 免登录，任何人可访问。 |
| `AuthUser` | 需登录会话或 Bearer Token，且账号 `Active`。 |
| `AuthAdmin` | 在 `AuthUser` 基础上要求 `Role == RoleAdmin`。 |
| `AuthAPIKey` | 需 API Key（`X-API-Key` 头、`Authorization: ApiKey/Bearer`，或查询参数 `?apikey=`）。 |

被禁用账号会按到期（`AccountExpired`）与手动禁用（`AccountDisabled`）返回不同 error_code，便于前端区分「续费」与「申诉」两条引导。

新增后台页面、接口和 Bot 管理能力的默认权限边界：

| 场景 | 要求 |
| ---- | ---- |
| Web 管理页 | 路由/API 使用 `AuthAdmin`；前端只做体验守卫，不能替代后端鉴权。 |
| 状态变更 | 成功后调用 `a.audit()` 或无 HTTP 上下文时调用 `a.auditEntryIP()`。 |
| 配置查看/编辑 | 必须脱敏显示 secret，未修改 secret 通过服务端哨兵保留，禁止明文回显。 |
| Telegram Bot 自定义 JS | 仅开发者模式文档/预检入口可编辑；生产 Bot 仅执行 `bot_custom_commands` 中 `js:` 前缀脚本，运行在受控 Goja 沙箱。 |

### Cookie 写请求

Twilight 不对 Cookie 鉴权的变更类请求做 CSRF 令牌校验，也不做额外来源校验。登录态依赖 `HttpOnly` session cookie，机器调用可使用 Bearer Token 或 API Key。`X-Twilight-Client: webui` 仅用于前端请求识别/CORS，不是鉴权手段。

### 前端网络与布局性能

- 旧 V1 WebUI 通过 `webui/src/lib/api-request.ts` 处理浏览器请求合流和短缓存；V2 不复制这套跨页面缓存，SSR `load` 与 form action 通过 `webui-v2/src/lib/server/api.ts` 按请求读取会话数据。
- 前端对成功状态的空响应按协议错误处理，不显示误导性的 `OK`；后端 handler 必须保证返回数据可 JSON 序列化，尤其是聚合 DTO 不得形成循环引用。
- 成功的公开读请求（调用方明确使用 `credentials: "omit"`）会进入 3 秒内存短缓存，也会合并同一时刻的重复请求，覆盖路由切换、组件重挂载和相邻组件同时挂载时的额外网络压力；缓存采用最近使用淘汰，最多保留 32 项，单响应源文本不超过 64K 字符，总源文本预算不超过 256K 字符。Cookie 登录态读请求不会参与共享缓存或在途合并，因为 HttpOnly Cookie 无法安全加入 JavaScript 缓存键。大型列表超过预算时不会深拷贝或缓存；任意写请求返回后会清空缓存。`/users/me`、带 `refresh=1` 的请求、带 `X-Twilight-Intent` 的有意图 GET、`no-store` / `reload` 请求和显式 `cacheRead: false` 的调用不进入短缓存。
- 读请求默认使用浏览器 `no-cache` 语义，允许复用连接但仍向服务端确认 freshness；写请求继续使用 `no-store`。
- API 响应解析由 V2 `webui-v2/src/lib/server/api.ts` 通过有界流读取，最大接受 8 MiB 的 JSON 响应；代理请求/响应使用 32 MiB 流式上限。读取过程中超过上限会取消流并拒绝解析，错误诊断必须带实际请求版本（`/api/v1` 或 `/api/v2`），不能把 V2 请求误报为 V1。V1 浏览器包装器只为回滚保留。
- 绑定码、状态卡片等轮询必须在页面不可见时暂停请求并中断在途请求，回到前台再按上次执行时间补跑；绑定码 TTL / deadline 可继续计时，但后台页签不应持续打状态接口。
- 仪表盘加载 Emby 线路时只读取线路列表，主页只显示线路入口和数量摘要；用户打开详情后才能查看具体线路。系统不自动发起逐线路探测，也不为测速额外预检 Emby 状态，测速由用户在详情中主动触发，避免首屏形成随线路数量增长的 N+1 请求。
- V2 只对 `/_app/immutable/` 下的哈希静态资源设置长期缓存；`/_app/version.json`、其它 `/_app/` 资源、SSR HTML、form action 和会话页面必须使用 `Cache-Control: no-store`。由于 adapter-node 会在 SvelteKit `handle` 之前直接提供版本清单，默认 Nginx 必须使用精确的 `location = /_app/version.json` 覆盖上游缓存头；不要让反向代理覆盖这个区分。
- 默认 `favicon.png` 应保持小尺寸和合理压缩，避免每个新访客为浏览器图标下载数百 KB 资源；需要高清品牌图时优先通过后台 `server_icon` 或环境变量覆盖。
- 管理后台页面要优先使用稳定尺寸、可换行按钮、可横向滚动表格和移动端卡片视图，避免手机、平板或浏览器打开开发者工具后的窄比例下文字越界、按钮互相覆盖。
- 共享 `Button`、`Input`、`Textarea`、`SelectTrigger` 原子控件统一使用 40px/36px 高度基线，并默认允许 `min-width: 0`、`max-width: 100%` 与安全断词；页面不得用移动端单独改高度的方式修补溢出，长表格应在自己的滚动区域内处理。
- 后台卡片标题中带搜索框、刷新按钮或运行状态元数据时，手机宽度必须纵向堆叠并让输入框占满可用宽度；只有合计固有宽度足够时才恢复横排，不能把固定宽度输入框和操作按钮无条件塞进一行。
- 登录后的公共布局、侧栏和全局守卫只使用 CSS/Tailwind 处理简单状态与一次性入场效果，不直接引入 `framer-motion`。复杂页面动画可以在具体路由内按需使用，避免无动画页面也下载并初始化动画运行时。
- 普通路由的一次性淡入应使用共享 `page-enter` CSS 类；只有确实包含交互、编排或连续动画的页面才按需引入 `framer-motion`，避免单个包装元素增加路由包体和运行时初始化。
- 下拉菜单、弹窗和 Select 内容应限制到视口宽度内，危险操作保持明确标签、二次确认和结果反馈。
- 窄视口优先保证可操作性：工具栏与危险操作区在手机上折行或纵向排列；数据表保留列语义并使用容器级触控横向滚动，不能靠压缩文字、隐藏关键字段或让整页横向溢出来“适配”。共享按钮允许多行标签并使用最小高度，Tabs 必须在自身容器内滚动。
- WebUI 以 Firefox 当前稳定版作为浏览器兼容基准。共享滚动区域必须使用标准的 `scrollbar-width`、`scrollbar-color`、`overscroll-behavior` 与 `dvh` 视口约束；`::-webkit-scrollbar` 只能作为非基准浏览器的补充，不能成为唯一实现。横向表格和 Tabs 仍需允许纵向页面手势，长弹窗、下拉菜单、Select 与侧栏则必须在视口内显示可见滚动条。
- 桌面侧栏与移动菜单复用同一套路由高亮和管理分组；扁平导航只能有一个 `aria-current=page`，打开菜单时应把当前项滚动到菜单自己的可视区。移动菜单保留完整可换行标签、领域分组和底部安全区，不能用截断文案换取窄屏适配。
- 配置管理页使用受控页签和单一编辑来源：可视化配置与 TOML 编辑存在另一侧未保存改动时，禁止直接覆盖；刷新、恢复备份和离开页面会提示未保存内容。配置段导航在桌面使用有边界的独立滚动列表，在平板和手机使用视口内 Select，并保持模块跳转链接与折叠按钮为两个独立操作。

### 文件与路径安全

- 上传文件必须使用 `http.MaxBytesReader` 与 `io.LimitReader` 双层限制大小。
- 上传文件类型以服务端内容探测结果为准，不信任用户提交的文件名和扩展名。
- 可被读取的上传资源文件名必须是服务端生成的白名单格式（如背景资源固定为 `[a-f0-9]{16}.(jpg|png|gif|webp|bmp)`）。
- 用户背景配置只能保存安全的渐变表达式和本系统上传的背景资源；不允许保存任意外部 URL、`url()` 注入或复杂 CSS 函数。
- 所有由请求参数参与构造的文件路径都必须经过 `filepath.Abs`、`filepath.Rel` 和目录约束校验（见 `internal/api/safepath.go`）。
- 备份恢复只允许读取备份目录内的普通 `.json` 文件，禁止绝对路径、`..`、子目录跳转和符号链接。
- 数据库迁移到 JSON 时，目标文件必须在数据库目录内且扩展名为 `.json`。
- 数据库恢复 / 迁移这类高风险操作必须实现预览、二次确认与操作前备份，后端不能只依赖前端确认弹窗。
- Git 更新、systemd 设置等命令执行必须使用 `exec.Command` 参数数组，禁止拼接 shell 命令字符串。
- Git 更新 URL 必须拒绝凭据、query string 和 fragment，避免把 token 写入 remote 或响应日志。
- 除一次性生成的密码、API Key 创建 / 重置响应外，不返回任何密钥明文。

### 开发者模式

- 仪表盘输入 `DEBUGMODE` 时，前端必须要求管理员二次输入当前密码，并调用 `POST /admin/developer-mode/activate`；成功只在当前浏览器会话记录开发者模式状态。
- 开发者模式页面提供 Telegram JS 自定义命令文档、示例、风险提示、服务端 JS 预设管理和 `POST /admin/developer/js-sandbox` 预检。沙箱仅暴露 `ctx`、`args`、`user`、`constants`、`reply(text)`、`log(text)`、`auth(role)`、`config(key)`、`env(key)`；`config`/`env` 只读白名单，不提供网络、文件或进程能力。
- Bot 运行时保持向后兼容：普通 `bot_custom_commands` 仍是固定文本回复；只有回复内容以 `js:` 开头才按 JS 执行。执行结果和日志不得包含 Token、密码、API Key、服务器线路等敏感信息。

## 数据模型与迁移约定

### 单一状态文档

全部业务状态都保存在「单一状态文档」（`internal/store` 中的 `State` 结构体）里，包括用户、注册码、邀请码 `invite_codes`、邀请关系 `invite_relations`、公告 `announcements`、求片、签到、设备、登录日志、IP 黑名单、调度计划等。它们以 `State` 结构体的字段（多为 `map`）形式存在，并非独立数据库或独立表。

唯一运行后端是 **PostgreSQL**：

- 主要业务状态写入 `twilight_state` 表中 `id = 1` 的单行 `jsonb`。另有独立表：`twilight_audit_logs`（操作审计）、`twilight_sessions`（会话）、`twilight_runtime_logs`（运行时日志）、`twilight_playback_records`（播放记录）、`twilight_telegram_roster`（Telegram 群成员花名册）、`twilight_telegram_runtime`（Telegram 更新确认游标）。高频或独立生命周期数据拆表用于避免无关运行时写入反复重写整份主状态，并避免历史花名册长期占用 Go 堆。
- `twilight_state.version` 是跨进程一致性的必要部分。Store 刷新会先比较版本，版本未变化时不会传回整份 JSONB；调试或人工维护数据库时，修改 `state` 必须同时执行 `version = version + 1`，正式代码不得绕过 `internal/store` 直接写这行。
- `Store` 仅能经 `store.OpenPostgres` 构造，`database.driver` 设为非 postgres 值时后端启动即报错。JSON 仅作为迁移面板的一次性导出目标与 `twilight migrate-json` 的导入源保留，不再是可运行后端；已无 JSON 文件后端、文件锁、`.bak` 影子文件或旁路日志文件。

> 不存在旧 Python 时代的 `db/invites.db`、`invite_relations` 单表，也没有「新增 xx.db / 新增表 / `ALTER TABLE announcements 增列` / 启动时自动建表」这类邀请或公告相关的迁移说法。新增邀请 / 公告字段，是在 `State` 结构体上加字段，并在加载时补默认值（见 `store.go` 中对 nil map 的初始化），不要引入独立表或单独的 SQLite 文件。

### 数据库性能与接口一致性

- 性能优化优先从现有访问模式入手：分页 / 游标、批量读取、索引、短超时、限流、前端按需加载和必要缓存；不要为了局部慢查询把业务实体拆成独立表，除非先更新架构文档并明确快照一致性、迁移、备份恢复方案。
- 管理员用户列表在大规模数据下必须先用轻量用户结构完成筛选、排序和分页，再为当前页构造公开 DTO；不得为页外用户提前创建 `map[string]any`。`per_page` 继续使用有上限的服务端参数，避免请求通过扩大页面大小制造内存峰值。
- 管理员用户列表的前端查询缓存必须同时设置条目数和行数上限，并在命中时更新最近使用顺序；筛选、排序和分页组合不能无限保留用户对象。
- 邀请森林读取只需要关系两端用户和（启用邀请时）邀请码持有人；应使用 Store 的 UID 范围快照，不要为一次树展示复制全量用户或完整邀请码列表。
- 批量 `select_all` 只需要目标 UID 时应使用 Store 的 UID-only 匹配方法；它必须返回完整匹配计数，同时只保留请求上限内的 UID，避免用空回调触发全量 `[]User` 分配。
- 有明确目标 UID 的批量处理应通过 Store 一次性读取目标用户，不要用 `ListUsers()` 构建全量 UID map；远端逐用户副作用仍按原有顺序和错误降级语义执行。
- 过滤用户的统一批量本地变更应使用带完整匹配计数的 Store 读取和一次 `UpdateUsers` 落盘；只有本地批量更新成功的 UID 才能进入后续远端副作用。
- Telegram 已有花名册时只按花名册中的 Telegram ID hydrate 用户；没有花名册的兼容 fallback 才允许扫描全量用户生成完整预览。
- Emby 设备/IP 审查只需本地 Emby 绑定用户，应使用紧凑的 `UsersWithEmby` 读取；扫描状态文档仍然需要遍历用户，但不要保留无关 Web 账号副本。
- 工单 Telegram 通知目标只需启用通知的管理员，应先筛选 UID 再 hydrate 管理员；不要每次工单事件复制全量用户。
- Emby 活动日志转播放记录时，先从当前事件批次提取用户身份 key，再用有界 `UsersMatching` 构造映射；不要为少量活动事件复制全量用户。
- Bangumi 管理用户列表应先用 UID-only 扫描完成搜索与计数，再按当前页 UID hydrate 用户；每页的同步日志/播放记录统计保持原有逐用户语义。
- PostgreSQL 的 `twilight_runtime_logs` 是高写入运行日志表，允许独立优化：最新快照按 `id DESC` 取最近 N 条，增量读取按 `id > after ORDER BY id ASC LIMIT N`，裁剪按 cutoff id 保留最近 N 条。状态接入前的内存 fallback 缓冲区必须保持相同 cursor 语义。普通快速成功请求不写运行日志，只保留失败请求和 2 秒以上慢成功请求，避免每个 HTTP 请求额外执行一次日志 INSERT。
- 管理员运行日志页的“加载更多”只调整快照上限，随后由同一条受 AbortSignal 管理的读取刷新；不能在按钮处理器里再发一条重复的全量请求。日志窗口使用 Firefox 兼容的有限高度滚动区。
- `twilight_telegram_runtime` 只保存一行单调 `getUpdates` offset。它是运行确认状态而非业务快照内容；旧 `State.TelegramBotOffset` 只作为升级/历史 JSON 导入种子，迁移后必须清零，运行期推进不得调用 `mutateAndSaveLocked`。
- `twilight_telegram_roster` 以 `(chat_id, telegram_id)` 为主键。普通群消息先命中进程内最多 4096 项的热观察缓存，同成员状态未变时五分钟内不访问数据库；冷缓存仍由 SQL 条件阻止近期行产生物理 UPDATE。定时成员检查先在 Go 内合并重复项，再通过一次 JSONB UPSERT 落库。启动会幂等迁移旧 `State.TelegramRoster`，备份/恢复则由 `Snapshot` / `LoadSnapshot` 合并和拆分，运行期不得把全量花名册重新常驻主状态。
- 新增列表接口应保持统一响应口径：数据数组放在 `items` 或既有兼容字段，增量游标使用 `next_cursor`；变更字段名或排序语义前必须同步后端 API 文档、前端 API 类型和调用方。
- 新增缓存必须写清作用域（进程 / Redis / 前端内存）、TTL、容量上限、失效条件和降级行为；配置热重载后不能继续读取旧配置或旧 store 句柄。外部服务的 URL 或凭据变化时必须清空以服务器身份为作用域的缓存；当前 Emby 热重载会清理会话、设备审查与管理员判定缓存。
- 配置文件签名探测由 API、Scheduler 与 Bot 共用 500ms 进程级节流；不得在普通 HTTP 请求路径恢复每请求两次文件系统 `stat`。绑定码 HTTP 长轮询应复用 `bindStatusHub` 状态通知和到期/超时定时器，不得恢复 500ms 周期扫描。

### 迁移与引导

- 更换存储后端前，必须先调用 `/api/v1/system/admin/database/migrate` 并传入 `dry_run=true`。预检会返回实体数量、快照大小、目标连通性以及重启 / 配置告警。
- 旧部署迁移应使用显式的一次性导入流程，不应在启动时隐式修改或猜测旧业务数据。
- 管理员身份只来自配置文件：启动时 `applyConfiguredAdmins` 会按 `config.toml` 的 `admin_uids` / `admin_usernames`（大小写不敏感）把匹配到的用户提升为管理员并置为 active；注册时命中同一配置列表的账号也会被提升。默认不配置时列表为空，没有任何账号是管理员。已移除「空库首注册者无条件成为管理员」通道，避免部署窗口期被陌生人抢注提权。
- 首次部署使用网页初始化向导：先在 `config.toml` 任意结构块临时写入 `setup_mode = true` 或 `SetupMode = true`；`GET /api/v1/setup/status` 仅在该标记启用、用户数为 0 且没有管理员配置时返回可用；`POST /api/v1/setup/complete` 需要 `X-Twilight-Client: webui` 与 `X-Twilight-Intent: complete-setup`，成功后创建管理员、写入 `[Admin].usernames`、移除 setup 标记并永久关闭入口。
- `admin_uids` / `admin_usernames`、网页初始化 `setup_mode` / `SetupMode` 标记以及 `[SystemUpdate].repo_url` 都禁止经普通网页配置接口（schema / 原始 TOML 保存）持久改写：保存时提交值会被剥离、重渲染丢弃或就地还原为磁盘原值，只能由运维在配置文件 / 环境变量侧设定。初始化向导是唯一网页侧一次性写入管理员名单的例外，并受显式 setup 标记 + 空系统硬门控保护。

## Docker 本地开发

项目提供完整的 Docker Compose 环境用于本地开发和测试：

```bash
# 启动完整的 Docker 开发环境 (PostgreSQL + Redis + 后端 + 前端)
docker compose up -d --build

# 查看日志
docker compose logs -f twilight webui

# 重启某个服务
docker compose restart twilight

# 停止
docker compose down
```

### 独立启动后端/前端（不用 Docker）

与 Docker 环境并行或替代使用——V2 dev server 可单独启动，指向 Docker 中的后端：

```bash
# 终端 1: Docker 后端 (PostgreSQL + Redis + API)
docker compose up -d postgres redis twilight
# 终端 2: V2 SSR dev server (hot reload)
cd webui-v2 && pnpm dev
```

V2 通过 `BACKEND_URL=http://127.0.0.1:5000` 由服务端访问后端；同源 `/api/v1/*` 和 `/api/v2/*` 代理只用于渐进增强，不是浏览器鉴权边界。

### Docker 开发注意事项

- 后端代码改动后需重建镜像：`docker compose up -d --build twilight`
- 前端代码改动在 `pnpm dev` 模式下即时生效（HMR）
- `config.toml` 挂载为只读卷；修改后重启服务生效
- 构建时使用 `BuildKit`（Docker 默认），支持缓存复用加速重复构建

## 验证与发布

### 提交前检查清单

后端或前端改动后，按需执行：

- [ ] `gofmt` 已执行（无格式化 diff）。
- [ ] `go test ./...` 已通过。
- [ ] `go vet ./...` 已通过。
- [ ] V2 前端或 SSR API 边界有变更时，在 `webui-v2/` 执行 `pnpm check` 与 `pnpm build`；CI 默认也只对 `webui-v2/` 执行前端质量门禁，旧 `webui/` 只有回滚改动时才单独验证。
- [ ] 已扫描敏感信息（密钥、token、明文密码）。
- [ ] 已扫描旧后端残留，确认 `start_backend_prod.sh` 与 `deploy/*.service` 指向 `bin/twilight`，未重新引入旧后端运行入口。
- [ ] 已检查鉴权级别、路径穿越、文件类型白名单与 CORS 配置。
- [ ] 涉及鉴权、上传、路径、配置保存、数据库迁移、Git 更新或实时日志的改动已补充安全边界测试。

### 安全基线

- 生产环境优先配置 Redis，用于共享会话与限流计数。
- 破坏性管理操作必须保留明确的确认步骤或 dry-run 预检。
- 上传与资产读取必须使用 `http.MaxBytesReader`、MIME 白名单、目录约束和统一响应 envelope。
- 数据库备份、恢复、迁移、Git 更新和 systemd 操作都不得拼接 shell 字符串。

### Git 更新与 systemd 约定

- 管理员 Git 更新接口（`/api/v1/system/admin/update`）支持 `dry_run` 预检，默认拒绝脏工作区；实现保持 `exec.Command` 参数化调用，禁止 shell 字符串拼接。响应不得返回服务器本机项目绝对路径；仓库地址、命令输出和错误信息必须经过凭据脱敏。
- systemd 安装前先执行 `sudo bash deploy/setup-systemd.sh --dry-run`。脚本会检测路径、配置、二进制、用户 / 组、端口、空白与 `%` 等 systemd 特殊字符，以及旧 Python 版 Twilight 的 unit。
- 部署的 unit 必须指向 `bin/twilight`，不要重新引入旧后端启动命令。

### 分支与合并发布流程

- 在 `main` 之外的特性分支上开发；提交保持原子化，便于 review 与回退。
- 提交前完成上面的「提交前检查清单」。
- 维护者合并前确认：测试与静态检查通过、前端 lint/build 通过（如涉及）、无敏感信息泄漏、鉴权与路径安全无回归、`deploy` 与启动脚本仍指向 `bin/twilight`。
- 后台调度器页面的任务列表读取由手动刷新和运行中轮询共享同一套可取消请求；轮询只在 Firefox 页面可见且存在运行任务时启用，间隔为 3 秒。切换任务日志时会取消前一个任务的详情与历史请求，长日志在受限 `dvh` 滚动区域显示。
- 配置管理页的 TOML、schema 读取和各业务页嵌入的 `AdminConfigSections` 编辑器都使用可取消请求；切换页签、重新加载或离开页面时，旧响应不能覆盖当前编辑状态。配置源、备份、更新输出和待保存变更预览都使用 Firefox 兼容的受限滚动区域并阻断滚动链。
- 配置页的图片上传卡片与“搜索 + 刷新/展开/折叠/保存”工具栏在手机、平板和窄桌面比例下保持纵向分组，操作区使用明确的两列网格；到 `lg` 宽度后才与描述或搜索框共用一行，避免按钮挤压输入框和逐字换行。
- 配置搜索框的清除图标必须使用 locale 文案并提供 `aria-label`，不能在页面中硬编码无本地化的可访问名称。
- Firefox 的 `IntersectionObserver.rootMargin` 只接受像素或百分比。配置段滚动定位及后续观察器不得使用 `rem`、`em`、视口单位或 `calc()`，避免构造观察器时直接让页面进入错误边界。
- 设备/IP 表格、求片处理队列、工单会话与附件条、注册码结果与使用记录、邮箱表格、公告预览和运行日志必须在自身的 Firefox 滚动区域中显示并阻断滚动链。Flex 工单会话正文要保留 `min-h-0`，否则窄屏时 Firefox 可能让内容撑开容器而不是内部滚动；长表格固定表头并在自身处理横向滚动。
