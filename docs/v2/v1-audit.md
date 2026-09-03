# Twilight V1 审计基线

> 审计时间：2026-09-04
>
> 本文以当前分支 `codex/v2-audit-foundation` 的 Go 源码、Next.js 源码、配置读取器、PostgreSQL 初始化代码、测试和 Git 历史为准。它是 V2 重构的事实基线，不把设计意图当成已经存在的行为。

## 1. 结论摘要

当前 V1 已经是一个可运行的 PostgreSQL + Go + Next.js 系统，安全和性能方面有多轮修复，但业务数据仍主要集中在 PostgreSQL 的单行 `twilight_state.state` JSONB 文档中。这个模型在小规模部署下实现简单，在用户、注册码、邀请关系、工单和求片数量增长后，会把局部写入放大成整份状态序列化、版本竞争和较大的内存峰值。

V2 的优先级应保持为：安全性 > 数据完整性 > 性能 > 响应速度 > 网络效率 > 渲染效率 > 页面切换体验 > 可维护性。V2 不应通过前端隐藏控件或更长 TTL 掩盖后端状态机、权限和一致性问题。

### 已确认的 V1 事实

- 后端入口是 `cmd/twilight`，运行时存储只接受 PostgreSQL；旧 JSON 仅由 `migrate-json` 一次性导入。
- API 路由集中注册于 `internal/api/routes.go`，当前共 336 条路由：Public 31、User 110、Admin 182、API Key 13。
- 统一响应 envelope、统一 JSON 解码限制、共享 HTTP transport、路径安全 helper、审计 helper 和错误码已经存在。
- 活动日志与播放记录仍然保留；旧播放统计页面、统计路由、统计配置已移除。V2 若重新实现观看统计，必须建立在活动日志/播放记录的明确事件模型上。
- 工单回复使用 `replies` 时间线，`admin_note` 是兼容字段和最新管理员摘要，不应再次成为覆盖用户回复的唯一存储。
- 设备/IP 审查为手动刷新，保留设备按用户、设备名、客户端名和版本聚合，并排除 Twilight 自身客户端。
- Dashboard 的 Emby 展示契约是当前在线人数；谁在观看和播放内容只允许管理员在有鉴权的专用工具中读取。

### 仍需在 V2 根治的主要问题

1. 单 JSONB 状态文档使高频局部实体操作发生写放大，难以对用户、工单、注册码、邀请关系和媒体求片建立数据库级约束与高选择性索引。
2. V1 同时保留 PostgreSQL、Redis、进程内 map/cache 和前端短缓存。虽然已有失效规则，但 V2 必须为每种缓存定义权威来源、作用域、TTL、失效事件和降级结果，避免“读取正常但另一模块仍显示旧状态”。
3. V1 仍有较多按页面组合的请求编排。V2 应按页面数据依赖提供聚合读取，同时保留独立刷新和错误隔离，避免首屏重复请求与无效轮询。
4. V1 前端仍有若干重型客户端页面和 `framer-motion` 依赖。V2 应将公共壳层、首屏数据和重型管理面板拆开，移动端先保证可用宽度、局部滚动和不丢失操作。
5. 路由数量较多且存在历史兼容别名。V2 可以换接口内部实现，但必须维护明确的兼容层和迁移表，不能悄悄把旧配置或旧数据丢掉。

## 2. 功能清单

| 领域 | V1 可见能力 | 主要代码位置 | V2 保留要求 |
| --- | --- | --- | --- |
| 初始化与配置 | 初始化向导、TOML/env 读取、热重载、脱敏 schema、配置备份恢复 | `internal/config`、`internal/api/config_admin.go` | 兼容旧 key；保存、恢复和生效范围可见 |
| 认证与用户 | 注册、登录、登出、找回密码、会话、用户名/密码、邮箱验证 | `auth_handlers.go`、`session.go`、`handlers.go` | 后端最终鉴权；身份读取必须会话绑定且不可被缓存串号 |
| 用户资产 | 头像、背景图、资源读取与删除 | `upload_handlers.go`、`safepath.go` | 保留路径隔离、MIME/大小检查和原子写入 |
| Emby | 绑定/注册/解绑、账号同步、线路、设备、会话、状态、活动日志 | `emby*.go`、`emby_activity.go` | 保留活动日志；外部副作用前先校验本地权限和状态 |
| Telegram | Bot、绑定码、换绑、花名册、面板、内置/自定义指令、JS 沙箱 | `telegram*.go` | 保留强类型协议、批处理、令牌脱敏和沙箱边界 |
| 注册码 | 生成、查询、启停、编辑、删除、批量删除、使用记录、补建资格 | `regcode_handlers.go`、`code_use_handlers.go` | 消费必须原子；缓存不能绕过数据库事实 |
| 邀请 | 邀请码、关系树、续期码、下级断开/删除 Emby、管理员维护 | `invite*.go` | 关系删除要幂等且刷新后不可复活；关闭开关不应阻断管理员历史维护 |
| 工单 | 创建、关闭/重开、类型、优先级、双方回复、图片、通知 | `ticket_handlers.go`、`store_tickets.go` | 回复追加不能覆盖；列表摘要和详情时间线分离 |
| 媒体求片 | Bangumi/TMDB/Emby 搜索、详情、库存、求片、同名聚合、批量处理 | `media_request_handlers.go`、`media_service.go` | 保留来源区分、revision 并发保护和管理员处理能力 |
| Bangumi | Token、收藏同步、集合、封面、Webhook、历史 | `bangumi*.go` | 全局 subject cache 与用户集合状态分离 |
| 签到与邀请续期 | 签到、积分、手动续期、自动续期 | `signin_handlers.go`、`scheduler_runner.go` | 所有条件在后端原子复核；没有 Emby 绑定不能消费积分续期 |
| 公告 | 公告 CRUD、强制阅读、安全渲染 | `announcement_handlers.go`、`safe-render.tsx` | 保留渲染白名单和安全 URL |
| 调度与运维 | 过期检查、清理、活动同步、备份、运行日志、健康检查、更新 | `scheduler*.go`、`database_admin.go` | 三类健康检查独立；任务需审计、并发上限和取消 |
| 审计与安全 | 操作日志、违规日志、设备信任、IP 黑名单、API Key | `audit_handlers.go`、`violation_handlers.go` | 关键状态变更均可追溯且不记录秘密 |

## 3. API 清单与鉴权

### 3.1 数量与登记方式

`internal/api/routes.go` 通过 `App.add(method, pattern, auth, handler)` 统一登记路由，并在启动期建立方法、路径段数和业务域索引。按源码统计，当前路由数量如下：

| 鉴权级别 | 数量 | 含义 |
| --- | ---: | --- |
| `AuthPublic` | 31 | 公共信息、登录、注册、公开检查和公开回调 |
| `AuthUser` | 110 | 当前登录用户的自助功能 |
| `AuthAdmin` | 182 | 管理员操作与完整运维信息 |
| `AuthAPIKey` | 13 | 第三方 API Key 接入，按 scope 再限制 |
| **合计** | **336** | 不含前端路由 |

完整的逐条路由、方法和鉴权索引仍以 `docs/reference/api-index.md` 和管理员接口 `/api/v1/system/admin/apis` 为准。公开 OpenAPI 只输出公开路由；完整库存不得对未鉴权用户暴露。

### 3.2 V1 API 领域分布

| 前缀领域 | 数量 | 备注 |
| --- | ---: | --- |
| `/admin` | 129 | 管理后台、审计、配置、工单、求片、Emby 等 |
| `/users` | 49 | 注册、自助资料、绑定、设备和密码 |
| `/system` | 40 | 系统信息、健康、配置、数据库和运行态 |
| `/media` | 18 | 搜索、详情、库存和求片 |
| `/auth` | 16 | 登录、会话和旧 API Key 兼容接口 |
| `/batch` | 13 | 管理员批量用户操作 |
| `/apikey` | 13 | API Key 外部接入 |
| `/invite` | 10 | 邀请用户流程 |
| `/tickets` | 9 | 用户工单 |
| `/emby` | 9 | Emby 用户侧接口与兼容路径 |
| `/bangumi` | 8 | Bangumi 用户侧接口 |
| `/signin` | 5 | 签到与积分续期 |
| 其他 | 9 | 根、文档、公告、setup、security |

### 3.3 权限审计基线

- `AuthUser` 由 session token 映射到 UID，再从 Store 读取当前用户；账号被禁用后 stale session 不能继续使用。
- `AuthAdmin` 在用户认证后检查当前角色，并保留最后一个管理员保护。
- `AuthAPIKey` 支持 `X-API-Key`、Bearer/APIKey 兼容格式和受控 query key；账号读取、写入、Emby 状态及会话踢出由不同权限 scope 控制。
- 管理员 UID 路径解析失败必须返回参数错误，不能回退到当前管理员 UID。
- 删除用户必须经过 `deleteLocalUser` 或同等清理，包含用户记录、会话和绑定状态清理。
- 公开 OpenAPI、公开系统信息和公开健康接口不能暴露私有路由、数据库、Emby 凭据或运行时秘密。

V2 迁移时应把权限决策集中成可测试的 policy/service，而不是在页面或每个 handler 中重新判断。所有对象读取、修改、删除都必须有资源归属检查，不能只检查“已经登录”。

## 4. 配置兼容关系

V1 的配置源顺序为默认值 → 主 TOML → `.local` TOML → `TWILIGHT_*` 环境覆盖（具体环境映射由 `internal/config` 实现）。当前配置 section 及主要兼容字段如下：

| Section | 主要字段族 | V2 兼容策略 |
| --- | --- | --- |
| `[Global]` | server_name、server_icon、redis_url、log_level、runtime 限制 | 保留名称；新增字段提供默认值 |
| `[Admin]` / `[SAR]` | uids、usernames | 合并旧别名，去重后形成同一管理员集合 |
| `[Database]` / `[PostgreSQL]` | driver、url、host、port、user、password、database、sslmode、pool、backup_dir | 继续接受旧 key；运行时只用 PostgreSQL |
| `[API]` | host、port、upload_folder、max_upload_size、cors_origins、代理头 | 不改变现行 CORS 语义；代理信任必须显式配置 |
| `[Security]` | session cookie、SameSite、Domain、bot secret | 保留安全默认值；秘密只写入脱敏 schema |
| `[Emby]` | URL、token、账号、线路、统计开关 | 保留 Emby 活动日志；V2 统计另行迁移 |
| `[Telegram]` | Bot、群组、订阅、面板、模板、指令和并发 | 保留模板占位符与自定义指令兼容解析 |
| `[BangumiSync]` | 启用、管理、Webhook、API、App ID | 保留 URL/token 语义与缓存 TTL |
| `[Register]` / `[Invite]` | 注册码、补建、邀请树、有效期和容量 | 配置值迁移时不得静默改变额度与开关 |
| `[Notification]` | 登录/到期/工单通知及模板 | 模板渲染必须统一转义 |
| `[Email]` | SMTP、验证码、强制绑定、改密证明和清理 | 失败对普通用户只返回通用文案 |
| `[RateLimit]` | 各类 IP、账号、上传和 API Key 限制 | 保留秒级归一和 Redis 降级上限 |
| `[Scheduler]` | 任务时间、间隔和清理开关 | 任务 ID 保持稳定，迁移可审计 |
| `[Ticket]` | 类型、开放上限、图片大小/数量/保留期、模板 | 类型归一由数据库/store 负责 |
| `[AuditLog]` | 启用、保留、数量和清理时间 | 不能因配置重载删除历史 |
| `[SystemUpdate]` | 仓库 URL、分支、重启和间隔 | 继续禁止凭据 URL 和非 HTTPS 仓库 |

### 兼容禁区

- 不能重新引入 JSON 文件运行时后端、旧 Python 入口或 SQLite 多库模型。
- `StateFile` 仅保留迁移命令和兼容显示用途，不能作为 V2 运行时的第二事实源。
- 配置热重载必须明确列出立即生效、重建运行时和需要重启的字段。
- 未识别的旧配置必须产生可见迁移提示，不能静默覆盖成默认值。

## 5. 数据模型与一致性

### 5.1 V1 主状态文档

`internal/store.State` 当前包含：

- 计数器：用户、API Key、求片、公告、登录、任务、换绑、违规、审计、工单、开发者预设等 ID。
- 用户与身份：`Users`、Emby/Telegram/邮箱绑定、密码安全偏好、通知偏好。
- 业务关系：`InviteCodes`、`InviteRelations`、`RegCodes`、`BindCodes`、`RebindRequests`。
- 内容和运营：`MediaRequests`、`Announcements`、`Tickets`、`TicketTypes`、Bangumi 集合/主题缓存。
- 运行与历史兼容字段：登录记录、设备、签到、播放会话、活动日志、违规日志、部分旧审计/运行日志、Telegram roster。

### 5.2 已拆分的独立表

| 表 | 目的 | 一致性说明 |
| --- | --- | --- |
| `twilight_state` | 主业务快照和版本 | 乐观版本保护；修改需走 Store |
| `twilight_sessions` | session token | 删除用户时必须全层清理 |
| `twilight_audit_logs` | 高写入安全审计 | 不进入主 JSONB；关键变更成功后写入 |
| `twilight_runtime_logs` | 运行诊断日志 | 有大小/数量保留约束 |
| `twilight_playback_records` | 播放记录高写入表 | 幂等键为 UID、item、played_at |
| `twilight_telegram_roster` | Telegram 花名册 | 受限热点刷新，历史不塞回运行热路径 |
| `twilight_telegram_runtime` | Telegram 更新游标 | 单调推进，不能被业务快照回滚 |

### 5.3 V1 关键不变量

- `Store.stateRaw` 与 `stateVersion` 保持一致；未变化版本不传输和解码完整 JSONB。
- 回滚必须恢复派生用户索引，不能直接赋值旧 State。
- 无远端副作用的批量用户写入使用一次 `UpdateUsers`；涉及每用户远端调用的批处理保持逐用户降级语义。
- 工单状态、类型、开放工单计数、回复追加和图片保留由 Store 归一化。
- 播放记录只能由成对活动事件产生时长，单次时长上限 12 小时，不能使用未匹配事件伪造时长。
- Bangumi 主题缓存是全局数据，集合缓存是每用户状态，不能因一个用户失效删除全局主题。

### 5.4 V2 数据拆分建议

优先把查询频繁、生命周期独立、可用数据库约束表达的实体拆成表：用户身份/会话、注册码、邀请关系、工单及回复、媒体求片、审计/运行日志、播放事件与每日聚合。保留真正适合作为版本化配置快照的内容在 JSONB，但不能让 UI 列表必须下载并解码整份状态。

每个拆分都必须提供：旧 JSONB 读取兼容期、一次性迁移、双读/双写窗口、校验计数、回滚策略和删除旧字段的版本说明。

## 6. 安全审计

### 6.1 已有防护（源码已确认）

- JSON body 限制大小、嵌套深度和单值尾部；上传使用大小、扩展名、MIME 和检测类型约束。
- `ResolveWithinRoot` / `ResolveLeafFile` 拒绝路径穿越、绝对路径、符号链接和非法备份名。
- 外部 URL 统一校验 http/https、host、link-local/未指定/云元数据地址；共享 HTTP client 禁止跨主机重定向。
- Emby、Telegram、TMDB、Bangumi 共用连接池和每请求 context 超时；错误日志有 token 脱敏边界。
- Session token 创建有冲突重试；Redis 失效时有受限的进程内/数据库降级，不能无限增长限流 map。
- 密码、邮箱证明、Emby 改密、账号启停和管理员变更由后端校验；前端开关不构成安全边界。
- 公开 OpenAPI 只输出公开路由；管理员完整 API 库存受保护。
- 公告和背景 CSS 有安全渲染/URL 白名单；Telegram JS 沙箱限制外部访问、响应体和执行窗口。
- 关键状态变更通过 HTTP、Telegram、scheduler/system 来源写入审计；删除用户有统一清理入口。

### 6.2 V2 必须继续验证的风险

| 风险 | 影响 | V2 验证方式 |
| --- | --- | --- |
| IDOR/越权 | 读取或修改其他用户资产、工单、关系 | 每个资源接口使用对象归属矩阵和负向测试 |
| 状态机竞态 | 重复消费注册码、重复续期、回复覆盖、关系复活 | 数据库事务/幂等键/并发测试 |
| 缓存脏读 | Web、Bot、scheduler 显示不同绑定状态 | 统一失效事件和跨进程测试 |
| 导入包攻击 | Zip Slip、炸弹、覆盖、恶意 JSON | 解包前清单、大小/数量/压缩比、临时目录和事务回滚测试 |
| 资源耗尽 | 2000+ 用户、超大工单、批量 Emby 调用拖垮服务 | 压测、分页上限、并发信号量和取消测试 |
| 外部服务滥用 | SSRF、重定向泄密、恶意回调 | URL、Host、签名、响应大小和超时负向测试 |
| 凭据生命周期 | 权限变更后旧 session/API Key 仍可用 | 角色/Key/密码变更后会话矩阵测试 |
| XSS/注入 | 用户内容进入公告、工单、Telegram、日志 | 输出编码、富文本白名单、结构化参数化 SQL 测试 |
| CSRF/跨站写入 | Cookie 会话被第三方页面诱导提交写请求 | SameSite、Origin/意图边界和部署模式测试；不改变既定 CORS 策略 |

## 7. 性能基线与瓶颈

### 后端

- 主状态整文档序列化和版本竞争是最主要的内存/CPU 放大点。
- 设备审查、邀请树、用户列表、活动日志和工单详情必须分页/分批，不能将全量数据交给前端一次渲染。
- 外部 HTTP 已共享 transport；V2 应进一步统一查询超时、并发上限、批量接口和失败隔离，而不是无限增加缓存。
- 调度器按到期任务并行但受 semaphore 限制；每个任务需独立 panic recovery 和 context 取消。
- PostgreSQL pool 有 max open/idle、生命周期和 idle 回收；V2 应用查询耗时、行数、序列化大小而不是只看平均接口时间。

### 前端

- `api-request.ts` 是唯一请求层，负责 credentials、超时、AbortSignal、GET/HEAD 合流和有界短缓存。
- 身份接口、刷新请求、no-store 请求和可取消请求不能被读缓存/合流。
- 管理列表筛选、分页、路由切换和卸载必须取消旧请求，并拒绝旧响应覆盖新状态。
- 大表采用服务端分页、受限批次或虚拟化；桌面表格与手机卡片不能同时挂载完整数据集。
- 手机、平板和窄 F12 视口应保证工具栏、按钮、表格和对话框不越界；长内容在自己的 `dvh` 滚动区内处理。
- 普通页面不应为简单过渡加载 Framer Motion；导航使用 `prefetch={false}` 控制无效 chunk 预取。

## 8. V2 审计验收清单

- [ ] V1 功能矩阵逐项映射到 V2 route/service/UI。
- [ ] 336 条 V1 路由有保留、兼容别名或迁移说明。
- [ ] `.env`、TOML、local TOML 和实际环境覆盖均有迁移测试。
- [ ] 所有资源接口有归属校验；管理员操作有权限、确认和审计。
- [ ] 注册码、邀请、工单、求片、绑定和续期在并发下保持原子和幂等。
- [ ] 导入导出通过格式、版本、完整性、安全、冲突和回滚全流程验证。
- [ ] 观看统计只从可信播放事件产生，能处理暂停、恢复、断线、跨天、多设备和重复回调。
- [ ] 2000+ 用户、数万日志/设备/关系下内存、CPU、查询时间和前端 DOM 数量有实测数据。
- [ ] Firefox 手机/平板/窄桌面视口通过截图和交互回归，所有局部滚动可用。
- [ ] 文档、AGENTS.md、API schema、错误码、i18n 和 CHANGELOG 与实现同步。
- [ ] 提交前无本机绝对路径、密钥、Token、Cookie、密码或调试输出。

