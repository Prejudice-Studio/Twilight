# Twilight V2 SSR 前端

本文记录 V2 前端的实际实现。V2 不在 V1 的 Next.js/React 页面上继续堆叠客户端状态，而是在 `webui-v2/` 使用 SvelteKit SSR 和 `@sveltejs/adapter-node`，完整承载当前用户端与管理员端页面。V1 `webui/` 仅作为出现发布问题时的整站回滚入口。

## 目标

- 首屏由服务端 `load` 生成，避免页面挂载后重复请求身份、系统信息和当前页面摘要。
- 身份 Cookie 只在 V2 服务端读取并转发给 Go 后端；浏览器不接触 Bearer Token，页面 JS 不维护跨用户全局身份缓存。
- 写操作使用 SvelteKit form action，默认支持无 JavaScript 提交；增强脚本只能改善反馈，不能成为权限或数据一致性的边界。
- 页面数据按路由隔离，详情、回复、附件和管理员大列表按需加载，避免把整个 V1 状态文档搬进浏览器。
- 手机、平板、桌面和窄比例 Firefox 视口优先，公共布局使用可收缩轨道、边界滚动和安全区内边距。

## 目录

```text
webui-v2/
  src/
    hooks.server.ts          # 每次页面请求读取当前会话
    lib/
      types.ts               # 跨端安全 DTO
      server/api.ts          # 服务端请求、Cookie、有限响应解析、同源代理
      i18n.ts                # V2 文案入口，迁移模块继续扩展 locale
    routes/
      +layout.server.ts      # 根布局会话数据
      +layout.svelte         # 轻量 SSR 壳层
      login/                  # 登录 form action
      forgot-password/        # 邮箱/Emby 找回密码 form actions
      setup/                   # 一次性初始化向导 form action
      logout/                # 登出 server endpoint
      (app)/dashboard/        # 首个已迁移的仪表盘摘要
      (app)/announcements/    # 公告和强制阅读确认
      (app)/settings/         # 个人设置与账号绑定
      (app)/settings/appearance/ # 外观、背景上传和头像维护
      (app)/score/            # 签到、积分续期和签到历史
      (app)/invite/           # 邀请摘要、邀请码和直属下级维护
      (app)/tickets/          # 用户工单摘要和会话
      (app)/bangumi/          # Bangumi 账号摘要、同步和收藏分页
      (app)/media/            # 媒体搜索、详情、库存检查和用户求片
      wiki/                   # 公开 SSR 使用指南
      (app)/admin/            # 管理员模块索引和轻量系统摘要
      (app)/admin/status/     # 管理员服务器状态与独立健康检查
      (app)/admin/config/     # 管理员配置 schema、TOML、备份和背景图维护
      (app)/admin/database/   # 管理员数据库状态、备份和迁移维护
      (app)/admin/emby/       # 管理员 Emby 账号、设备/IP 审查与活动日志
      (app)/admin/bangumi/    # 管理员 Bangumi 用户同步状态与按需详情
      (app)/admin/audit-logs/ # 管理员操作日志分页、筛选与保留策略维护
      (app)/admin/violations/ # 管理员违规审计分页、筛选和清理
      (app)/admin/invite/     # 管理员邀请树、邀请码摘要和邀请配置
      (app)/admin/requests/   # 管理员求片筛选、同名聚合和冲突校验处理
      (app)/admin/regcodes/   # 管理员注册码分页、生成、编辑和使用记录维护
      (app)/admin/announcements/ # 管理员公告分页、发布、编辑与状态维护
      (app)/admin/logs/         # 管理员运行状态与手动运行日志快照
      (app)/admin/email/        # 管理员邮箱验证审查、维护和 SMTP 测试
      api/[...path]/          # 仅 v1/v2 的同源 API 代理
```

## 请求与会话

`src/lib/server/api.ts` 是 V2 的服务端传输边界：

1. 后端地址来自 `BACKEND_URL`，仅允许 `http` / `https`，拒绝 URL userinfo，并移除 query/fragment。
2. 服务器页面请求只转发入站 Cookie；显式丢弃入站 `Authorization` 和 `X-API-Key`，避免把浏览器提交的 Bearer/API Key 凭据混入 SSR 会话链路。
3. JSON 响应先检查 `Content-Length`，再通过有界流读取，超过 8 MiB 时取消读取并返回空结果；解析失败不会把上游正文回显到页面。API 代理对请求和响应使用 32 MiB 流式上限，避免为上传/下载一次性复制整个正文。
4. 登录 action 只接受配置的会话 Cookie（默认 `twilight_session`），解析上游 `Set-Cookie` 时忽略后端 Domain，只向当前 V2 站点写 host-only、HttpOnly Cookie，避免跨域扩大 Cookie 作用范围。
5. SSR 读取默认为 `no-store`。当前用户身份、权限和会话状态不能进入共享浏览器缓存。

`src/lib/components/AppShell.svelte` 是唯一的应用壳层：它承载登录前后的站点标题、主导航、账号菜单、管理员分组导航和移动端滚动边界；`+layout.svelte` 不再复制导航或页面样式，只负责注入全局 CSS 并传入 SSR 子内容。`src/lib/app.css` 提供全局盒模型、字体、焦点可见性、最小视口和 reduced-motion 基线，各业务页继续拥有自己的局部布局，避免页面间互相污染。

根布局显式设置 `ssr = true`、`csr = true`、`prerender = false`。这不是性能开关，而是身份安全约束：任何带会话的页面都必须由 adapter-node 在请求时渲染，不能因为未来新增路由或更换 adapter 被静态化。根 `+error.svelte` 只展示本地化通用错误文案，不回显 Go、Emby、数据库或文件系统错误原文。

SSR 到 Go API 的请求以及同源 `/api/v1/*`、`/api/v2/*` 流式代理共享 15 秒有界截止时间，并合并 SvelteKit 当前请求的取消信号；该信号传递到响应体读取阶段。上游无响应或响应体读取超时会被页面/代理的通用错误状态吸收，不会让 Node worker 无限等待或把外部错误泄露给浏览器。该边界不修改后端 CORS 策略。

`webui-v2/scripts/check-architecture.mjs` 会在 `pnpm check` 中执行迁移验收：扫描旧 `webui/src/app` 的页面并确认 V2 存在对应 `+page.svelte` 或服务端兼容入口，同时禁止 V2 引入 React/Next/Zustand，禁止业务页面绕过 SSR API client 直接调用 `fetch`。旧前端仍可作为回滚版本存在，但不能重新成为默认依赖或数据边界。

`src/routes/api/[...path]/+server.ts` 只允许代理 `/api/v1/*` 与 `/api/v2/*`，通过流式上限限制请求体，移除 hop-by-hop、Origin、Referer、Authorization、API Key 和 Host 等头。Go 后端仍是唯一认证、权限、限流和业务状态边界。SvelteKit form action 默认启用同源 Origin 校验。

## 应用壳层与导航

`src/lib/navigation.ts` 是 V2 的唯一导航元数据源，集中定义主导航、账号菜单和管理员分组，并由 `isActivePath` 统一计算当前页面。响应式布局只改变展示方式，不复制目的地列表；管理员路由最多只有一个项目带 `aria-current="page"`。

顶栏使用原生 `details` 菜单，不依赖常驻客户端状态。账号和管理员长菜单拥有受限 `dvh` 滚动区与 Firefox 标准滚动条，手机与窄开发者工具宽度下仍保持可滚动、可聚焦。头像仅渲染服务端生成的受保护资源路径，历史任意 URL 不会进入图片请求。

## 兼容入口

为避免旧书签在迁移期间失效，V2 保留以下服务端重定向：`/admin/stats`、`/admin/test` → `/admin/status`；`/admin/device-audit` → `/admin/emby?tab=devices`；`/admin/telegram/commands` → `/admin/telegram`；`/admin/developer/js-docs` → `/admin/developer`；`/settings/background` → `/settings/appearance`。这些入口不创建第二套页面、请求或状态探测逻辑。

## 生产入口与回滚

Linux + systemd 部署由 `deploy/setup-systemd.sh` 管理 `twilight-webui-v2.service`，构建产物为 `webui-v2/build`，默认监听 `127.0.0.1:3001`。`deploy/nginx-twilight.conf` 将 `/` 和 `/_app/` 反代到 adapter-node，`/api/` 仍按原配置直接转发到 Go API；这只是传输拓扑变化，不改变 CORS 策略。

Docker Compose 的 `webui` 服务同样从 `webui-v2/Dockerfile` 构建。反向代理部署时需要设置 `WEBUI_ORIGIN` 为浏览器实际访问的完整 Origin；本地直连可使用默认的 `http://localhost:3000`。升级前应保留旧 V1 构建和上一版 V2 构建，回滚只切换反向代理前端目标，不回滚 PostgreSQL 数据或 Go API。

## 迁移规则

一个功能完成迁移必须同时具备：

- V1 页面/API 到 V2 route、load、action 和 DTO 的映射。
- 服务端权限复核、状态变更审计、重复提交/并发处理和失败回滚边界。
- 列表摘要与详情分离，分页/游标和附件按需读取。
- 手机/平板/窄 Firefox 视口验证，长表格、对话框和聊天区域拥有自己的 `dvh` 滚动上下文。
- V1 配置/数据兼容、回退入口和文档更新。

V2 页面覆盖 V1 当前用户可见的全部路由；兼容别名只负责旧书签跳转，不复制业务读取或写入逻辑。后端 feature gate、权限和状态机仍是唯一安全边界。

## 已迁移模块：个人中心

`/(app)/settings` 是第一个完整迁移的用户侧模块。页面首屏由服务端读取 `/users/me/settings`，其中包含当前用户摘要、Telegram/Emby 状态、通知偏好和密码安全策略；设置保存、邮箱发码/验证、系统密码、Emby 密码、绑定、开通和解绑均通过 SvelteKit form action 转发到 Go API。

- 浏览器不读取或保存会话令牌，密码成功轮换会话时由 action 转发上游 `Set-Cookie`。
- 复选框在服务端转换成 JSON 布尔值，不能把字符串或前端显示状态当作权限边界。
- 邮箱验证码记录 ID 只在当前 action 结果中回显，改密所需验证码仍由 Go 后端核验。
- Emby 绑定/解绑的资格、邮箱验证、管理员保护和远端副作用仍由 Go 后端决定；V2 页面不自行复制这些规则。
- 页面使用两列桌面布局、单列移动布局和自然换行，密码输入使用浏览器原生自动填充语义。

## 已迁移模块：用户 API Key 管理

`/(app)/settings/apikey` 使用 SSR `load` 读取 `/users/me/apikeys`，只向浏览器发送当前账号的掩码、权限、状态和受限使用摘要。创建、编辑和删除均为 SvelteKit form action；服务端 action 会限制名称、ID、布尔值和限速范围，但账号归属、权限、审计和持久化仍由 Go 后端最终判断。

新建 API Key 的明文只放在本次创建 action 的结果中显示一次，不写入 URL、缓存、持久化页面状态或日志；后续列表只显示掩码。页面提供独立的更新/删除确认和刷新入口，列表在桌面与窄 Firefox 视口使用稳定换行及有界滚动，不会把全部 Key 历史复制到浏览器缓存。

## 已迁移模块：认证注册

`/register` 使用服务端 `load` 读取注册开关/容量摘要与公开系统能力，默认 action 将用户名、邮箱、注册码和 Telegram 注册绑定码提交到 Go 的 `/users/register`。密码确认只用于用户体验，密码强度、注册码消费、Telegram 绑定码原子消费、邮箱冲突和容量限制仍由 Go 后端决定。

Telegram 注册绑定码生成使用独立的 `createBindCode` form action；启用 JavaScript 时通过 `use:enhance` 只更新当前页的临时码，不把会话凭据或 Bot Token 放入浏览器。注册提交不依赖客户端状态判断，服务器会再次验证绑定码是否已确认，因此刷新、重复提交或旧页面状态不会绕过注册条件。

## 已迁移模块：认证辅助页面

`/forgot-password` 使用服务端 `load` 读取公开功能开关，邮箱发码、邮箱重置和 Emby 验证分别由 SvelteKit form action 转发到 Go 后端。浏览器不会直接调用找回密码 API，也不会接触上游错误原文；邮箱流程保留统一成功/失败文案以避免账号枚举，Emby 流程返回的临时 Web 密码只存在当前 action 结果中，不进入 URL、缓存或持久化状态。

`/setup` 使用服务端 `load` 读取初始化可用状态和站点摘要，初始化表单由服务端构造严格的嵌套 payload，并携带 `X-Twilight-Client: webui` 与 `X-Twilight-Intent: complete-setup`。服务器接收后端下发的 host-only、HttpOnly 会话 Cookie 后再重定向到管理员状态页；初始化资格、密码强度、配置字段校验和一次性关闭入口仍由 Go 后端最终决定。Emby Token、Bot Token、SMTP 密码只在 action 请求体中传输，不被写入浏览器状态。

## 已迁移模块：用户工单

`/(app)/tickets` 直接使用后端的工单摘要分页和单条详情契约。SSR `load` 只请求当前页摘要；只有 URL 中存在 `ticket` 编号时才读取该工单的正文、回复时间线和附件。创建、回复、关闭、重开和 Telegram 通知切换均为服务端 form action，成功后使用 303 回到列表或当前会话，失败只回传通用页面错误与后端业务消息。

页面桌面端使用列表/会话双栏，移动端切为单列；列表和会话各自拥有 `overflow`、`overscroll-behavior` 和 Firefox 可见滚动条。用户侧详情仍由 Go 后端执行归属校验，V2 不依据隐藏按钮决定是否可操作，也不会把管理员内部备注渲染进用户会话。

仪表盘使用 `/api/v2/dashboard/summary` 一次读取当前用户、公开能力和在线人数。Emby 失败时只显示“暂不可用”，不会把失败伪装为 0；本地用户和能力数据不受单个外部依赖影响。

## 已迁移模块：公告

`/(app)/announcements` 在服务端读取 `/users/me/announcements`，首屏同时包含可见公告和当前账号尚未确认的强制阅读公告，不在浏览器端重复请求公告列表。公告正文以 Svelte 文本节点输出，保持字符转义，不复刻 V1 的富文本渲染路径；需要确认的公告通过 `acknowledge` form action 调用后端批量确认接口，并使用去重后的正整数 ID。

公告列表按级别使用低饱和语义颜色区分，列表在桌面分为两列，在窄 Firefox 视口收为单列。长标题和正文允许换行，页面不使用无限滚动或定时刷新；用户需要新数据时使用浏览器原生刷新，避免公告页产生持续网络开支。

## 已迁移模块：签到与积分续期

`/(app)/score` 通过受保护的 `/api/v2/signin/summary` 一次读取签到摘要、公开奖励规则和最近 30 条记录。签到、手动积分续期和用户自动续期开关分别使用服务端 form action；成功后采用 303 重新读取页面权威状态，避免浏览器本地积分、连续天数、Emby 绑定状态与 PostgreSQL 快照分叉。

手动续期和自动续期使用不同的显示条件：手动续期只依据积分余额和 Emby 绑定提示，自动续期还必须满足到期、账号状态、管理员保护和后端续期资格。最终条件始终由 Go handler 和 Store 原子复核，页面上的 disabled 只是操作提示，不能作为安全边界。签到页不轮询，历史记录使用有界 Firefox 滚动区域。

## 已迁移模块：用户邀请中心

`/(app)/invite` 首屏使用受保护的 `/api/v2/invite/summary` 一次读取非敏感邀请配置、当前用户的上级、直属下级、邀请树和本人邀请码；不再分别请求配置、关系和邀请码列表。摘要只包含关系展示和操作资格所需字段，不包含下级的邮箱、Telegram 身份或 Emby 内部凭据。

- 生成邀请码、删除邀请码、生成直属下级续期码、本人历史关系清理和下级 Emby 清理均使用 SvelteKit 服务端 form action。
- 删除邀请码是物理删除，已使用的码也不会被错误显示为“仅停用”；删除码不会解除已经建立的邀请关系，关系清理必须使用断开接口。
- 邀请系统关闭时页面保留历史关系和清理入口，但新邀请码按钮由摘要状态禁用；最终是否允许生成、续期或清理仍由 Go handler、关系归属和 Store 原子操作判断。
- 本人断开只在后端确认存在邀请上级且 Emby 已到期、Emby 已禁用或 Web 已禁用且仍绑定 Emby 时可用；执行会删除远端 Emby、清空本地绑定和待开通状态，并清理旧邀请码占用，Web 账号不会被删除。
- 页面没有轮询；刷新通过普通 GET 重新获取 no-store 摘要，续期码只在当前 action 结果中展示。树和下级列表允许换行，窄 Firefox 视口下操作组会堆叠。

## 已迁移模块：Bangumi 用户端

`/(app)/bangumi` 使用受保护的 `/api/v2/bangumi/summary` 生成首屏。摘要由 Go 后端一次返回本地同步状态、Bangumi 公开账号字段、五类收藏的数量和有限预览；收藏预览在后端并行读取并分别降级，某一类上游失败不会清空同步状态或其他分类。Token 仅在服务端向 Bangumi 发起请求时使用，既不写入 V2 envelope，也不进入浏览器脚本。

`/(app)/bangumi/collections/[type]` 使用服务端 `load` 读取当前分类的受限分页，状态修改、同步、清理历史和 Token/开关设置使用 form action 转发至现有 Go handler。收藏页不加载五类完整集合，不在浏览器端复制用户收藏数据库；分页、标签展示和收藏状态最终以 Bangumi 响应及后端权限为准。页面使用响应式单列/多列布局、有限滚动和原生表单降级，刷新由用户主动触发。

## 已迁移模块：用户求片中心

`/(app)/media` 使用 URL 参数驱动的 SSR 页面：`q`、`source`、`type`、`media_id` 和 `tab=requests`。名称搜索只在服务端读取 TMDB/Bangumi 聚合结果；点击结果后，服务端并行读取媒体详情和 Emby 库存，详情接口失败时保留搜索结果中的标题和海报。浏览器不会接触后端会话令牌，也不会为详情再发起一组客户端 API 请求。

- “我的求片”只有在打开 `tab=requests` 或显式刷新时才读取，避免求片搜索首屏携带不必要的历史列表。
- 创建和删除求片是 SvelteKit form action，邮箱验证、Telegram 绑定、库存检查、队列上限、同源同季去重、权限和审计仍由 Go 后端最终判断。服务端页面只做参数长度和格式限制，不复制业务资格判断。
- 搜索结果、详情和求片记录中的图片 URL 只接受无 userinfo 的 HTTP(S) 地址。真实海报保持固有比例，使用 `height: auto` 和 `object-contain`，不使用固定比例黑色容器，因此横向图片和竖向海报不会被裁切或压缩。
- 详情页使用来源、媒体类型、评分、上映信息、简介、别名、主创/演员、来源链接和库存状态等安全 DTO；缺失的单个外部字段不会让整个详情区域消失。

## 已迁移模块：管理员服务器状态

`/(app)/admin/status` 由独立的管理员服务端布局保护：未登录用户回到登录页，已登录但非管理员的用户收到 403，不能依靠隐藏导航作为权限边界。页面首屏由服务端并行读取三个独立健康接口、系统信息和管理员统计，浏览器只接收一次 SSR 页面数据，不再为状态页发起五组客户端请求。

三个健康结果分别保留“正常、异常、读取失败、未配置”状态；数据库 Ping 失败会被标记为异常，不会因为状态快照仍可读而误报正常。读取失败的单项不会丢弃其它成功结果。刷新使用普通 GET 表单，页面不轮询、不自动探测 Emby 会话。

V2 页面只接收状态展示所需的安全 DTO。Emby 内部地址、数据库/网络错误原文、Token 和连接串不会进入浏览器；后端健康响应也只返回通用错误说明和有限运行指标。系统状态与运行统计使用 Firefox 兼容的自然换行、独立网格和稳定的移动端堆叠布局。

## 已迁移模块：管理员调度器

`/(app)/admin/scheduler` 由管理员服务端布局保护，首屏只读取任务摘要；任务运行、终止、计划保存/恢复和参数化清理均通过 SvelteKit form action 转发到 Go 后端。运行记录只有管理员明确打开某个任务日志时才按需读取最近一条和最多 20 条历史，日志正文保持有界滚动，不进入共享缓存。

V2 调度器不使用浏览器轮询或 SSE。任务仍在后端异步运行，管理员可通过“刷新任务”主动读取权威状态；这降低了长时间打开后台页面的请求数和 CPU 开支，也避免多个标签页重复轮询。手动参数在 SSR action 内按任务类型重新组装并限制范围，不能依赖隐藏字段或前端控件绕过后端互斥、权限和参数校验。

## 已迁移模块：管理员用户管理

`/(app)/admin/users` 是 V2 的首个管理员业务页面。页面首屏由服务端 `load` 读取 `/api/v1/admin/users` 的当前分页，并将搜索、角色、Web 状态、Emby 状态、邮箱状态和排序参数限制在已知集合内。浏览器只接收当前页的摘要用户，不会把 2000+ 用户完整复制到 DOM 或客户端缓存；列表区域使用有界 Firefox 滚动容器。

单用户操作按账号状态、Emby、身份绑定、权限/危险操作分组。启停、续期、Emby 启停、状态刷新、Telegram/Emby 解绑、角色调整和删除均使用 SvelteKit form action，由服务端转发当前 HttpOnly 会话到 Go API；成功后 303 回到当前筛选结果，重新读取权威状态。页面根据 `admin_action_state` 提示不可用操作，但不会以隐藏按钮替代后端鉴权。

创建账号仅在当前 action 结果中显示一次性临时密码，不把密码放进 URL、持久化状态或共享缓存。删除、角色调整、账号启停和 Emby 操作均保留确认步骤；级联深度由服务端再次限制。旧 V1 用户管理页只作为整站紧急回滚入口。

## 已迁移模块：管理员工单

`/(app)/admin/tickets` 使用服务端分页读取管理员工单摘要，默认只读取 `open` / `in_progress` 队列；状态、类型、优先级和 UID 筛选均转换为后端查询参数。列表只渲染标题、提交人、状态、优先级、回复/附件数量等摘要，打开编号后跳转到 `/(app)/admin/tickets/[ticketId]`，不会把全量工单正文和回复历史装入浏览器。

管理员详情页通过 `GET /admin/tickets/{ticket_id}` 读取单个完整会话。文字回复固定提交到 `POST /admin/tickets/{ticket_id}/reply`，不会把 `admin_note` 当成聊天内容，也不会在保存属性时替换 `replies`。状态、优先级、类型和内部备注各自使用独立的 form action，减少两个管理员并发处理时的陈旧字段覆盖。

附件上传、预览和删除继续走后端统一的 `/tickets/{ticket_id}/images` 接口。V2 使用同源服务端 multipart action 转发文件，浏览器只看到受鉴权的相对图片地址；服务端仍由 Go 校验工单归属、管理员权限、关闭状态、图片真实类型、大小、数量和安全文件名。管理员可以在关闭工单上继续追加排查回复和维护附件。

工单类型维护也使用服务端 action，并在 Go store/config 层持久化。列表与详情均使用 `no-store`，不轮询；手动刷新通过普通 GET 重新读取数据库权威状态。长队列、对话、附件和移动端操作组均有独立的 Firefox 兼容有界滚动区域。

## 已迁移模块：管理员配置管理

`/(app)/admin/config` 使用管理员服务端布局保护，首屏由一个 SSR `load` 并行读取配置 schema、脱敏后的当前 TOML 和配置备份元数据。浏览器不会调用配置 API，也不会接触真实 Token、密码、连接串或 Bot Secret；后端返回的 secret 哨兵只作为“保持现值”的提交标记。

结构化配置与原始 TOML 是两个互斥编辑模式：结构化模式只提交当前 schema 允许的字段，列表和 Telegram 自定义指令使用可视化行编辑；TOML 模式只提交受限大小的原文。两种保存都通过 SvelteKit form action 转发到 Go 后端，由后端执行类型归一、敏感值回填、受保护字段保留、备份、解析校验和热重载。

备份列表只展示受限元数据。预览 action 读取服务端脱敏内容，恢复先生成 dry-run 结果，再要求明确的恢复确认短语；删除、整理配置和认证页背景图上传也不依赖客户端 API token。备份内容和长 TOML 预览拥有独立的 Firefox `dvh` 滚动区域，配置段导航在手机和平板改为横向可滚动列表。

## 已迁移模块：管理员数据库维护

`/(app)/admin/database` 在管理员服务端布局内运行，首屏通过 `Promise.allSettled` 并行读取数据库状态和备份元数据。页面不会加载完整状态快照、数据库连接串或任意文件路径；备份列表在 Firefox 中使用有界滚动区域，手机端操作按钮按列堆叠。

创建备份、查看快照摘要、删除备份、恢复和数据库迁移均使用 SvelteKit form action。恢复先提交 dry-run 生成目标/当前计数和保护性备份提示，再以 `RESTORE_DATABASE_BACKUP` 明确确认；迁移先进行目标预检，再以 `MIGRATE_DATABASE` 确认执行。目标驱动、状态文件名和备份名在 SSR 层限制格式，最终的 Zip/路径/快照校验、数据库连通性、功能开关和原子写入仍由 Go 后端负责。

## 已迁移模块：管理员 Emby 管理

`/(app)/admin/emby` 由管理员服务端布局保护。账号页通过 `/admin/emby/users` 做服务端搜索、筛选和分页，只把当前页发送到浏览器；失效本地绑定单独分页。设备/IP 页签只在管理员主动打开或刷新时读取 `/admin/emby/device-audit`，按 Emby 用户聚合设备，过滤 Twilight 自身连接，离线记录按设备名、客户端和版本合并，并使用独立的 Firefox 有界滚动区域。活动日志页签默认读取数据库中的 Emby ActivityLog，只有点击同步才从 Emby 拉取并入库，不恢复播放统计页面。

连通性检测、用户列表、媒体库列表和本机回环候选探测都在 Go 后端发起；页面只接收服务器基本信息和通用失败文案，不接收 Emby URL、Token 或网络错误原文。同步、导入、清理、绑定重置、广播、独立账号创建、强制改密、账号启停和踢会话均通过 SvelteKit form action 转发 HttpOnly 会话，后端负责最终鉴权、参数校验、外部副作用和审计。生成的独立账号密码或强制重置密码只作为当前 action 结果显示，不进入 URL、缓存或持久化页面状态。

## 已迁移模块：管理员 Bangumi 管理

`/(app)/admin/bangumi` 由管理员服务端布局保护。首屏使用 `/admin/bangumi/users` 的分页结果和系统功能摘要，搜索、分页和详情类型都由 URL 表示；列表只返回用户 Bangumi 开关、Token 是否配置、就绪状态以及有限统计，不把 Token、播放记录或同步日志装入浏览器。

播放记录和同步日志只有在管理员打开指定 UID 的详情时才通过 SSR `load` 读取，详情表格使用 Firefox 有界滚动并保留水平滚动空间。手动同步和清除日志通过 SvelteKit form action 转发，提交前的确认只是体验保护，Go 后端继续负责功能开关、用户校验、Bangumi 外部调用、持久化和审计。管理员用户列表的记录数和最近 100 条日志成功数由 Store 批量计算，避免在 2000+ 用户场景按用户形成 N+1 查询。

## 已迁移模块：管理员操作日志

`/(app)/admin/audit-logs` 使用管理员服务端布局保护。筛选条件、排序、时间范围、UID 和分页均由 URL 表示，SSR `load` 只请求 `/admin/audit-logs` 当前页；不会把独立的 PostgreSQL 审计表完整复制到浏览器，也不会使用客户端轮询或长驻缓存。预设筛选包含管理员、用户、系统、高风险和安全相关操作，时间范围与动作筛选最终仍由 Go 后端白名单和 SQL 参数化查询执行。

单条删除、全部清空和按天数/条数裁剪均通过 SvelteKit form action 转发 HttpOnly 会话。页面只做数值范围和交互确认，Go 后端再次校验管理员权限、确认短语、清理参数和审计表操作；清理审计表不会向同一张表追加递归日志。详情 JSON 作为转义文本放入可折叠区域，长列表和详情区域使用有界 Firefox 滚动容器，窄屏下危险操作会堆叠而不会把日志内容挤出视口。

## 已迁移模块：管理员公告

`/(app)/admin/announcements` 使用管理员服务端布局和 `/admin/announcements` 分页接口。筛选条件（显示隐藏公告、显示过期公告、页码和每页数量）由 URL 表示，SSR `load` 只向浏览器发送当前页；页面不再在客户端保留完整公告历史，也没有自动轮询。创建、编辑、置顶/取消置顶、显示/隐藏和删除均通过 SvelteKit form action，成功后 303 回到原筛选条件并重新读取后端权威数据。

编辑表单覆盖标题、内容、级别、渲染模式、过期时间、强制阅读和置顶/可见状态。前端只做长度、数值范围和确认交互，Go 后端继续负责安全渲染模式白名单、字段归一、审计和状态保存。旧 API 响应中的过期字段可能叫 `expired_at`，写入仍使用 `expires_at`；V2 DTO 同时兼容两者，避免历史数据在迁移期间丢失过期状态。SSR 预览使用文本节点和 `white-space: pre-wrap`，不会把公告正文当作 HTML 执行；安全富文本渲染器需要另行完成审查后才可接入。

## 已迁移模块：管理员违规审计

`/(app)/admin/violations` 使用管理员服务端布局保护。违规类型、搜索关键词、分页和每页条数由 URL 表示，SSR `load` 只读取后端当前分页，不把历史违规记录复制到浏览器，也不使用轮询或客户端缓存。

单条删除和清空全部均通过 SvelteKit form action 转发到 Go 接口。action 会在服务端再次校验正整数记录 ID 和 `CLEAR_VIOLATIONS` 确认短语，后端继续负责管理员鉴权、状态写入和审计。违规用户名、注册码、原因、IP 等字段通过 Svelte 文本节点输出，列表拥有有界 Firefox 滚动区域，窄屏下危险操作会堆叠。

## 已迁移模块：管理员注册码

`/(app)/admin/regcodes` 使用管理员服务端布局保护。类型、状态、来源、关键词、排序、每页条数和页码由 URL 表示；SSR `load` 只读取当前分页，注册码使用者只有在打开指定卡码详情时才读取，不会把完整注册码历史或使用者列表复制到浏览器。

生成、编辑、启停、单条删除、批量删除和清理使用记录均使用 SvelteKit form action。批量删除支持当前筛选全部匹配，但请求仍按 Go 后端的 `BATCH_DELETE_REGCODES` 确认短语和数量上限执行。创建结果只放在当前 action 响应中，不进入 URL、缓存或持久化状态；服务端继续负责随机码生成、目标用户校验、容量/存储状态和审计。注册码、备注、用户名、Telegram 标识和使用记录均作为转义文本渲染，长列表与使用者区域使用有界 Firefox 滚动。

## 已迁移模块：管理员运行日志

`/(app)/admin/logs` 只在服务端并行读取 `/system/admin/runtime/status` 和 `/system/admin/runtime/logs` 的当前快照，日志条数通过 URL 限制在后端允许的有限范围。V2 不打开旧页面的 EventSource/SSE，也不在浏览器定时轮询；管理员需要最新数据时点击重新读取，避免持续连接、重复请求和长时间保留日志数组。

运行日志消息和属性作为转义文本显示，日志列表与详情状态使用有界 Firefox 滚动区。运行时状态只显示运维所需的有限指标，读取失败使用通用文案，不把进程错误、配置密钥、Cookie 或上游网络诊断交给浏览器。V1 的实时流接口继续保留给兼容入口，但新 SSR 页面不会依赖它。

## 已迁移模块：管理员求片管理

`/(app)/admin/requests` 通过 SSR `load` 调用后端分页接口，状态、来源、搜索、页码和手动拆分状态保存在 URL。Bangumi 与 TMDB 使用不同来源标识；同名请求由后端返回为一组，默认只渲染一个组卡片，管理员可以把该组拆开查看并单独处理。整组更新走后端批量接口，单条更新和删除均携带当前 `revision`，服务端使用 `If-Match` 处理并发冲突。

页面没有浏览器轮询或后端 Token，图片地址经过协议和凭据校验，备注、媒体名称、来源元数据均作为 Svelte 转义文本输出。列表拥有 `70dvh` 上限和 Firefox 原生滚动条，移动端操作表单会堆叠而不是挤压到视口外。

## 已迁移模块：管理员邀请系统

`/(app)/admin/invite` 由管理员服务端布局保护，并以 `view=tree|codes|config` 分开读取。邀请树仍由 Go 后端提供关系快照，但 SSR `load` 在服务端用 UID 映射、单次 DFS/BFS 和叶子优先回溯计算根节点、层级及后代规模；浏览器只收到当前筛选结果的一个有界批次（默认 300 行）、当前选中详情和根节点摘要，不再保存完整邀请森林。搜索会保留命中节点的祖先路径，折叠状态、根筛选、页码和邀请码搜索均由 URL 表示，便于刷新后回到同一视图。

批量断开、只清理已禁用 Emby、选中/子树/全量断开并续期、级联启停、级联删除和配置保存都通过 SvelteKit form action 转发当前 HttpOnly 会话。action 会重新校验 UID、数量、层级、续期天数和配置字段；确认短语、管理员保护、`renew_days=-1`、已禁用账号不续期以及邀请系统关闭后的历史维护仍由 Go 后端最终决定并写入审计。邀请码管理当前是服务端筛选分页的安全摘要读取，没有为不存在的管理员删除接口伪造删除能力。

表格、邀请码列表和移动端操作组使用 Firefox 有界滚动/换行规则；页面没有轮询或浏览器端 API 请求。邀请配置只提交白名单字段，服务端 action 会先重新读取完整 schema，再合并邀请字段并保存，因而不会因旧页面或并发编辑覆盖其它配置段，也不会向浏览器暴露 secret 字段。

## 已迁移模块：管理员 Telegram 管理

`/(app)/admin/telegram` 是 Telegram 管理的 SSR 页面。服务端 `load` 并行读取 Telegram 配置 schema、后端权威指令目录和花名册摘要；失败项互不覆盖，浏览器只收到 Telegram 页面需要的白名单字段，不包含 Bot Token、API 地址或其它敏感配置。页面显示 Bot 手动连通性测试、群组/频道策略、Bot 文案、`/twguser` 面板模板、占位符和自定义文本/JS 指令编辑器。

配置保存和 Bot 测试都通过 SvelteKit form action 转发当前 HttpOnly 会话。action 会重新限制字段类型、列表数量、文本大小、并发范围和自定义指令结构，Go 后端继续负责最终配置归一、热重载、权限和审计。Bot 测试只回传成功摘要或通用失败文案，不回传 Telegram 上游错误、内部地址或 Token。

内置指令目录来自 `/admin/telegram/commands/catalog`，禁用状态写回 `Telegram.disabled_commands`；自定义指令写回 `Telegram.bot_custom_commands`，不建立第二份运行时存储。内置指令不能被覆盖，JS 回复仍受开发者模式沙箱约束。页面不使用浏览器轮询/SSE，指令列表和编辑区域具有 Firefox 兼容的有界滚动，窄视口下控件堆叠并允许文本换行。

## 已迁移模块：管理员安全中心

`/(app)/admin/security` 是不加载安全历史或配置副本的 SSR 管理入口。它只将管理员导向已经迁移的操作审计、运行日志、违规记录、Emby 设备/IP 审查和配置管理页面；页面本身不引入浏览器轮询、额外 API 请求或第二套安全策略状态。

安全配置继续只在 `/admin/config` 的受限 schema 编辑器中保存。审计/日志/违规/设备模块分别保留自己的服务端读取、确认和 Go 后端鉴权、参数校验及审计边界，安全中心不通过前端隐藏或本地状态替代这些约束。

## 已迁移模块：管理员 Telegram 换绑审批

`/(app)/admin/telegram-rebind-requests` 使用管理员服务端布局和 URL 分页筛选，默认只读取待处理请求的当前 20 条摘要。单项批准/拒绝通过当前页面的 form action 提交；待处理请求可以用标准 checkbox 多选后批量处理，选中 ID 在服务端重新校验并限制在后端允许的批量范围内。

撤销全部已批准但未使用的换绑许可是单独的危险操作，V2 action 要求输入固定确认短语后才会转发，并只显示通用失败文案。所有结果依赖 Go 后端的最终权限、状态转换、持久化和审计；页面没有浏览器缓存、轮询或 SSE，长列表使用 Firefox 有界滚动并在窄视口堆叠操作表单。

## 已迁移模块：管理员邮箱管理

`/(app)/admin/email` 使用 URL 表示当前视图、筛选和分页，服务端只读取 `/admin/email/verifications` 的当前验证码或邮箱账号视图。验证码列表只显示脱敏邮箱、用途、关联 UID、尝试次数和有效期，不向浏览器传递验证码或哈希；邮箱账号列表只返回验证状态、账号状态和 Telegram 摘要。

撤销验证码、清理过期验证码、清空未验证邮箱和 SMTP 测试均通过 SvelteKit form action 转发当前 HttpOnly 会话。危险维护动作要求服务端确认标记并在浏览器端提供确认提示；Go 后端继续负责管理员权限、参数校验、数据库写入、审计和发信配置校验。SMTP 失败只显示通用文案，页面不会回显上游主机、认证错误或网络诊断。邮箱配置统一从 `/admin/config` 维护，邮箱页不复制第二套配置编辑状态。

## 已迁移模块：管理员模块索引

`/(app)/admin` 使用管理员服务端布局，只并行读取公开系统摘要和管理员轻量统计，不读取完整用户、日志或外部服务列表。页面按用户、内容、安全、运维和外部服务分组展示 V2 的全部管理入口；摘要读取失败只影响摘要区域，具体模块仍可直接打开并各自处理错误。

## 已迁移模块：公开 Wiki

`/wiki` 是不需要登录的静态 SSR 使用指南。内容在服务端渲染为转义文本，快速入口只链接到 V2 已有的仪表盘、设置、邀请、工单、公告和 `/api/v1/docs`；页面不读取用户状态、配置密钥或外部服务，也没有客户端数据请求和自动刷新。FAQ 使用原生 `details`，在手机和平板上保持单列和自然换行。
