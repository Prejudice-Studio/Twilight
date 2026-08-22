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
| `webui` | Next.js 前端应用。 |
| `webui/src/lib/api.ts` | 前端 API 客户端，集中维护所有后端调用。 |
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

前端是位于 `webui/` 的 Next.js 应用，使用 pnpm 作为包管理器（见 `package.json` 中的 `packageManager`）。

### 常用命令

```bash
cd webui

# 安装依赖（锁定 lockfile）
pnpm install --frozen-lockfile

# 本地开发服务器
pnpm dev

# 代码检查（eslint src --ext .ts,.tsx）
pnpm lint

# TypeScript 类型检查
pnpm typecheck

# 生产构建（next build，输出 standalone）
pnpm build
```

> CI / 非交互环境说明：`webui/.npmrc` 关闭了 `node_modules` 重建确认，避免 `ERR_PNPM_ABORTED_REMOVE_MODULES_DIR_NO_TTY`；`webui/pnpm-workspace.yaml` 显式批准 `esbuild`、`sharp`、`unrs-resolver`、`workerd` 的构建脚本。新增带 postinstall 的依赖时，必须同步更新该白名单或说明为什么应阻断，不能只在本机交互式运行 `pnpm approve-builds` 后不提交配置。

后端可单独启动配合调试：

```bash
bash start_backend_dev.sh
```

### 前后端联调与环境变量

前端通过环境变量决定如何访问后端（见 `webui/.env.example` 与 `webui/next.config.mjs`）：

- 设置了 `NEXT_PUBLIC_API_URL` 时，浏览器端直连该后端地址。
- 未设置 `NEXT_PUBLIC_API_URL` 时（典型的本地开发），Next 的 `rewrites` 会把 `/api/*` 代理到 `BACKEND_URL`（默认 `http://localhost:5000`），避免跨域。
- 设置了 `NEXT_PUBLIC_API_URL` 时，根布局会为该 API origin 注入 `dns-prefetch` / `preconnect`，减少分离域名部署下登录与首批数据请求的握手延迟。
- 受保护路由由客户端 layout 调 `/users/me` 校验登录态，避免 Web 域读不到 API 域 cookie 导致登录后被踢回 `/login`。
- `SITE_NAME` / `SITE_TITLE` / `SITE_DESCRIPTION` / `SITE_ICON` 是运行时可注入的展示文案，由 `app/layout.tsx` 每次请求读取，改完即生效，无需重新构建；默认图标路径是 `/favicon.png`，不要写成源码目录形式的 `public/favicon.png`。

**认证页外观定制**（全部可选，`NEXT_PUBLIC_*` 前缀，构建时注入）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `NEXT_PUBLIC_AUTH_TEXT_COLOR` | — | 登录/注册页文字颜色（CSS 颜色值），经正则防注入 |
| `NEXT_PUBLIC_AUTH_ICON_URL` | — | 认证页品牌图标，优先于后端 `server_icon` |
| `NEXT_PUBLIC_AUTH_BG_OVERLAY_OPACITY` | `0.4` | 背景图叠加层不透明度（0-1） |
| `NEXT_PUBLIC_AUTH_PANEL_OPACITY` | `0.82` | 右侧面板毛玻璃不透明度（0-1），有背景图自动 +0.08 |

> 所有前置变量均在 `webui/.env.example` 中列出，可直接复制到 `.env.local` 按需开启。

### 前端文案与多语言

- 轻量 i18n 入口位于 `webui/src/lib/i18n.tsx`，语言文件统一存放在 `webui/src/locales/`。
- 主分支只提供 `zh-Hans.json`、`zh-Hant.json`、`en-US.json` 三个显示语言，文件名使用 BCP 47 风格命名。
- 新增文案、翻译或语言时，按 [前端多语言开发与翻译指南](./i18n.md) 操作。

### 前端契约

- 所有后端调用集中维护在 `webui/src/lib/api.ts`，底层请求逻辑在 `webui/src/lib/api-request.ts`。
- 响应统一为 envelope 结构 `{ success, code, message, data, timestamp }`；前端按 HTTP 状态码与 `error_code` 分流处理（401 跳登录、403 权限提示、429 退避、5xx 通用故障，以及自定义业务 error_code）。
- 新增或调整接口时，需同步检查前端调用路径、请求方法、鉴权等级、错误提示文案与移动端展示。
- 登录支持用户名和邮箱两种方式：`api.ts` 的 `login()` 自动检测 `@` 将 payload 从 `{username}` 切换为 `{email}`，后端 `handleLogin` 对应走 `FindUserByEmail`。
- 认证页采用右侧固定面板布局：面板外壳由 `(auth)/layout.tsx` 渲染（跨页持久化，无闪动），各页面仅提供表单内容；共享样式常量与组件定义在 `(auth)/auth-ui.tsx`。
- 页面按目录分组：`webui/src/app/(auth)`（登录 / 注册 / 找回密码）、`webui/src/app/(main)`（用户面板与各管理页）。新增页面时优先复用 `webui/src/components` 下的既有组件。
- 后台总入口为 `webui/src/app/(main)/admin/page.tsx`（管理导航）。迁移出的配置模块必须有独立管理页：邮箱管理、Telegram 管理、邀请系统管理、安全中心；配置管理只保留默认折叠的兼容入口和跳转提示。
- 独立管理页若需要编辑配置，必须复用 `/system/admin/config/schema` 与 `api.updateConfigBySchema()`，写回同一个 `config.toml`；不要在前端或 store 中复制第二套配置源。
- 用户管理页的单用户与批量操作必须按领域分组展示（账号状态、Emby、身份绑定、注册资格、危险操作），避免把所有操作平铺成过长菜单或按钮栏；新增用户操作时同步维护后端返回的 `admin_action_state` 与前端 `UserInfo` 类型，让前端能显示禁用原因。
- 用户管理筛选变化必须只加载第一页，不能先请求已经失效的旧页；每页数量变化要清除跨页选择。选择“拥有 Emby 的用户”时，当前页全选只统计当前页已绑定 Emby 的行。
- Emby 管理与设备/IP 审查共用 `webui/src/app/(main)/admin/emby`，设备审查是页签级入口；`/admin/device-audit` 只作为兼容直达页面保留。设备审查不展示 Twilight 自身连接 Emby 时产生的设备/会话；全量 Emby 设备记录清理只放在调度器 `cleanup_emby_devices`，不要在审查页重新添加“清理全部/踢出全部”入口。
- 工单页必须展示 `replies` 双方回复时间线；`admin_note` 仅作为最新管理员摘要和旧数据兼容字段。状态、优先级、类型归一、关闭/重开时间戳、开放工单计数与“更新时保留 replies/附件”逻辑统一放在 `internal/store`，前端和 handler 不要重复判断。
- 管理员工单列表是处理队列，只返回并展示回复数、图片数、正文摘要和内部备注摘要；完整 `replies`、附件 URL、图片预览与双方对话只在单工单详情接口和对话页加载，避免历史消息随列表分页重复传输和渲染。
- 邮箱管理的验证码与邮箱账号列表使用后端搜索和分页；前端必须传 `view=pending|accounts`，切换筛选、分页或页签时取消旧请求。手机和平板显示信息卡片，桌面显示可滚动表格；进入邮箱配置页签不额外请求两份列表。无 `view` 的全量响应只保留给旧客户端兼容。
- 注册码管理的自由搜索使用 250ms 防抖后才请求后端；类型、状态、来源、排序和顺序改变都回到第一页。注册码表格、生成结果和使用记录弹窗使用 Firefox 兼容滚动容器，不能恢复为每次按键请求或无边界撑高页面。
- 后台重型面板应按需加载：非默认页签不要在首屏自动请求大接口；公共系统信息走 `useSystemStore.fetchInfo()` 的 TTL 与 inflight 复用，配置保存后调用 `invalidate()`。
- 管理列表、详情和筛选结果应把 `AbortSignal` 从 `useAsyncResource` 或页面控制器传到 `api.ts`；分页、筛选、手动刷新、路由切换和卸载时取消旧读取，并用请求序列阻止旧响应覆盖新状态。取消请求不弹失败提示。
- 侧边栏、移动菜单、管理导航等密集导航区域使用 `Link prefetch={false}`，避免首屏预载大量不一定访问的后台页面 chunk；只对明确的高频下一步保留预取。

## API 与安全规范

### 路由与 handler 约定

- 新路由统一在 `internal/api/routes.go` 注册，通过 `a.add(method, pattern, auth, handler)` 声明方法、路径、鉴权级别和 handler；按功能域分布在 `registerAdminRoutes` / `registerAPIKeyRoutes` / `registerSecurityRoutes` / `registerBatchRoutes` 等分组函数中。
- handler 只负责参数校验、鉴权、调用服务和整理响应；可复用的业务逻辑放到对应功能域文件，外部服务调用必须走独立 client/helper（Emby、TMDB、Bangumi、Telegram），不要散落在 handler 内。
- 响应必须使用统一 envelope，并与 `webui/src/lib/api.ts` 保持兼容。
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

- WebUI 统一通过 `webui/src/lib/api-request.ts` 发起应用 API 请求。无请求体的 `GET` / `HEAD` 会在短时间窗口内合并相同的在途请求，降低重复刷新、多个组件同时挂载和窄屏重排时的额外网络压力；写请求、带调用方 `signal` 的请求和显式 `dedupe: false` 的请求不参与合并。
- 成功的普通读请求会进入 3 秒内存短缓存，覆盖路由切换、组件重挂载和相邻组件重复读；缓存采用最近使用淘汰，最多保留 32 项，单响应源文本不超过 64K 字符，总源文本预算不超过 256K 字符。大型列表超过预算时不会深拷贝或缓存；任意写请求返回后会清空缓存。`/users/me`、带 `refresh=1` 的请求、带 `X-Twilight-Intent` 的有意图 GET、`no-store` / `reload` 请求和显式 `cacheRead: false` 的调用不进入短缓存。
- 读请求默认使用浏览器 `no-cache` 语义，允许复用连接但仍向服务端确认 freshness；写请求继续使用 `no-store`。
- 绑定码、状态卡片等轮询必须在页面不可见时暂停请求并中断在途请求，回到前台再按上次执行时间补跑；绑定码 TTL / deadline 可继续计时，但后台页签不应持续打状态接口。
- 仪表盘加载 Emby 线路时只读取线路列表，不自动发起逐线路探测，也不为测速额外预检 Emby 状态。线路测速由用户点击“测速线路”后按需触发，避免首屏形成随线路数量增长的 N+1 请求。
- Next 自己管理 `/_next/static` 下 hashed 构建产物的 `Cache-Control`。不要在 `next.config.mjs` 的 `headers()` 里覆盖这一路径，否则会触发 Next 构建警告并可能破坏开发模式缓存行为。
- 默认 `favicon.png` 应保持小尺寸和合理压缩，避免每个新访客为浏览器图标下载数百 KB 资源；需要高清品牌图时优先通过后台 `server_icon` 或环境变量覆盖。
- 管理后台页面要优先使用稳定尺寸、可换行按钮、可横向滚动表格和移动端卡片视图，避免手机、平板或浏览器打开开发者工具后的窄比例下文字越界、按钮互相覆盖。
- 登录后的公共布局、侧栏和全局守卫只使用 CSS/Tailwind 处理简单状态与一次性入场效果，不直接引入 `framer-motion`。复杂页面动画可以在具体路由内按需使用，避免无动画页面也下载并初始化动画运行时。
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

与 Docker 环境并行或替代使用——前端 dev server 可单独启动，指向 Docker 中的后端：

```bash
# 终端 1: Docker 后端 (PostgreSQL + Redis + API)
docker compose up -d postgres redis twilight
# 终端 2: 前端 dev server (hot reload)
cd webui && pnpm dev
```

前端 dev server 通过 Next.js rewrites 将 `/api/*` 代理到 `localhost:5000`（Docker 后端暴露的端口）。

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
- [ ] 前端或 API 客户端有变更时，在 `webui/` 执行 `pnpm lint`、`pnpm typecheck` 与 `pnpm build`。
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

- 管理员 Git 更新接口（`/api/v1/system/admin/update`）支持 `dry_run` 预检，默认拒绝脏工作区；实现保持 `exec.Command` 参数化调用，禁止 shell 字符串拼接。
- systemd 安装前先执行 `sudo bash deploy/setup-systemd.sh --dry-run`。脚本会检测路径、配置、二进制、用户 / 组、端口、空白与 `%` 等 systemd 特殊字符，以及旧 Python 版 Twilight 的 unit。
- 部署的 unit 必须指向 `bin/twilight`，不要重新引入旧后端启动命令。

### 分支与合并发布流程

- 在 `main` 之外的特性分支上开发；提交保持原子化，便于 review 与回退。
- 提交前完成上面的「提交前检查清单」。
- 维护者合并前确认：测试与静态检查通过、前端 lint/build 通过（如涉及）、无敏感信息泄漏、鉴权与路径安全无回归、`deploy` 与启动脚本仍指向 `bin/twilight`。
