# 模块化架构与解耦指南

本文定义 Twilight 后续重构和新增功能时的模块边界。目标是在保持现有功能、路由、配置源、鉴权和审计语义不变的前提下，逐步降低 `internal/api` 与前端页面层的耦合。

## 目标

- 业务域清晰：用户、Emby、Telegram、邮箱、邀请、注册码、求片、工单、调度、安全、配置管理等模块可以独立定位、测试和演进。
- 依赖方向单一：HTTP 层适配请求，业务层表达状态转移，外部 client 只负责远端协议，store 只负责持久化和原子写入。
- 统一横切能力：鉴权、审计、限流、错误码、配置读取、敏感信息脱敏、路径安全和响应 envelope 只能有一套实现。
- 可渐进迁移：不为了“目录好看”一次性搬动大文件；每次拆分都必须有测试和行为兼容说明。

## 非目标

- 不引入旧 Python 后端、SQLite 多业务库、独立业务表或新的运行入口。
- 不把配置复制到第二套前端 store、数据库实体或环境变量。
- 不为了抽象而增加单方法接口、过深目录或空泛 service。
- 不改变现有 API 路径、错误码、权限等级和前端调用契约，除非同时提供兼容层和迁移文档。

## 后端分层

当前后端仍以 `internal/api` 为主要业务承载目录。后续拆分按以下依赖方向推进：

```text
cmd/twilight
  -> internal/api              HTTP 路由、鉴权、请求解析、响应 envelope
       -> domain service       业务规则、状态转移、跨实体编排
            -> internal/store  单一状态文档读写、原子更新
            -> integrations    Emby/Telegram/TMDB/Bangumi/SMTP 等外部协议
       -> internal/config      当前运行配置快照
       -> internal/security    密码、token、安全随机数
```

允许的依赖方向：

| 层级 | 可以依赖 | 不应依赖 |
| ---- | -------- | -------- |
| `cmd/twilight` | `internal/api`、`internal/config`、启动脚本约定 | 具体业务 handler |
| `internal/api` handler | request、当前用户、配置快照、store、service、client、统一响应 | 直接拼 shell、绕过 store 写状态、散落敏感日志 |
| domain service | 明确参数、store 方法、外部 client 接口、context | `http.Request`、全局可变配置、前端字段名、具体路由路径 |
| integrations/client | context、超时、远端 DTO、脱敏错误 | 本地业务权限、HTTP handler、store 状态写入 |
| `internal/store` | 状态模型、原子更新、快照备份 | HTTP、外部 API、配置热重载句柄 |

## 后端模块边界

新增或拆分后端代码时，先按业务域选择文件，再决定是否需要新 package。

| 业务域 | 当前主要位置 | 拆分方向 |
| ------ | ------------ | -------- |
| 用户与管理员用户 | `handlers.go`、`batch_user_handlers.go` | 用户筛选、批量操作、角色/启停保护优先拆成 user service |
| Emby | `emby*.go`、`admin_emby_user_actions.go` | Emby client、设备审查、账号同步保持独立文件；跨用户批量操作放 service |
| Telegram | `telegram*.go` | Bot 生命周期、命令、绑定、JS 沙箱分离；Bot 输出统一走脱敏 helper |
| 邮箱 | `email_*.go` | 发信 client、验证码 service、管理员审查 handler 继续分离 |
| 邀请与注册码 | `invite_*.go`、`regcode_handlers.go`、`business.go` | 邀请树、卡码生成、卡码消费从 `business.go` 逐步抽出 |
| 配置管理 | `config_admin.go` | schema 定义、值映射、TOML 保存、备份恢复按职责拆小文件 |
| 调度 | `scheduler_*.go`、`admin_jobs.go` | job 定义、执行器、运行记录、手动触发保持分离 |
| 安全与审计 | `audit_handlers.go`、`violation_handlers.go`、`ratelimit.go` | 审计写入 helper 仍作为横切能力，不复制到各业务域 |

## Handler 规则

Handler 只做 HTTP 适配：

1. 读取 `current(r)`、path/query/body。
2. 做参数校验和 feature gate。
3. 调用业务函数或外部 client。
4. 成功后写审计日志。
5. 返回统一 envelope。

Handler 不应做：

- 大段业务状态转移。
- 直接拼接远端 URL 或 shell 命令。
- 直接读写 `db/twilight_go_state.json`。
- 把 Token、密码、API Key、数据库 URL、服务器线路写入日志或响应。
- 绕过 `AuthAdmin` / `AuthUser` / `AuthAPIKey` 边界。

## Service 规则

Service 适合承载以下逻辑：

- 多实体状态转移，例如“消费邀请码并更新用户有效期和关系”。
- 需要单元测试的业务分支，例如注册码状态、邀请树生成、设备审查聚合。
- 多 handler 复用的流程，例如管理员批量用户操作。
- 外部副作用前的本地权限校验和容量校验。

Service 入参应是明确类型，不直接接收 `http.Request`。需要操作者信息时，传入 `actorUID`、`actorRole`、`clientIP` 等显式字段。

## Store 规则

`internal/store` 仍是唯一业务持久化入口。

- 新业务实体优先加到 `store.State`，并在 `ensure()` 中补默认 map/list。
- 原子变更优先做成 store 方法，例如启停用户、消费卡码、清理验证码。
- 列表筛选如果被批量操作复用，列表 handler 与批量 helper 必须共享同一筛选口径。
- PostgreSQL 除 `twilight_state`、`twilight_sessions`、`twilight_runtime_logs` 外不新增业务表，除非先更新架构文档并说明快照一致性。

## 外部 Client 规则

外部 client 只负责协议和数据转换：

- 所有请求带 `context.Context`。
- 必须设置超时、必要退避和错误脱敏。
- 返回结构化结果，不直接写本地 store。
- 高频或昂贵调用可以短缓存，但必须说明作用域、TTL、失效条件和降级行为。
- Emby/Jellyfin 副作用必须先由业务层完成本地权限、容量、到期状态和管理员账号保护校验。

## 前端分层

前端依赖方向：

```text
app routes
  -> components / feature components
       -> hooks
       -> store
       -> lib/api.ts
            -> lib/api-request.ts
```

| 层级 | 职责 |
| ---- | ---- |
| `webui/src/app` | 路由页面、布局、页面级数据加载和组合 |
| `webui/src/components` | 可复用 UI 与业务面板；复杂后台页签可放 `components/admin/*` |
| `webui/src/hooks` | 轮询、异步资源、区域刷新等复用行为 |
| `webui/src/store` | 登录态、系统信息、跨页面轻状态；不保存第二套业务配置 |
| `webui/src/lib/api.ts` | 唯一 API 客户端；处理路径、类型、资源 URL 归一化和短缓存 |
| `webui/src/lib/api-request.ts` | credentials、timeout、envelope、`ApiError` |
| `webui/src/locales` | i18n 文案；新增用户可见文案必须补 `basic`、`zh-Hant`、`en-US` |

页面组件不应直接裸 `fetch`。如确有特殊场景，必须保持同样的 credentials、超时、错误处理和脱敏语义。

## 前端模块化规则

- 页面只组合，不堆业务巨型组件；超过一个独立工作流时拆到 `components/admin/*` 或领域组件。
- 重型面板按需挂载，非默认页签不在首屏请求大接口。
- 密集导航区使用 `Link prefetch={false}`，避免首屏预载大量后台 chunk。
- 系统信息统一通过 `useSystemStore.fetchInfo()`，配置保存后调用 `invalidate()`。
- 配置编辑统一复用 `/system/admin/config/schema` 与 `api.updateConfigBySchema()`。
- 资源 URL、头像、背景、公告渲染必须走 `safe-url`、`safe-render` 或 API 客户端归一化。

## 横切能力

以下能力不得复制第二套实现：

| 能力 | 固定入口 |
| ---- | -------- |
| 路由注册 | `internal/api/routes.go` |
| 鉴权等级 | `AuthPublic`、`AuthUser`、`AuthAdmin`、`AuthAPIKey` |
| 响应 envelope | `internal/api/response.go` 与 `webui/src/lib/api-request.ts` |
| 错误码 | `internal/api/errcode.go` 与 `webui/src/lib/errcode.ts` |
| 审计日志 | `a.audit()`、`a.auditEntryIP()` |
| 配置源 | `config.toml`、`config.local.toml`、`TWILIGHT_*` 覆盖 |
| 配置 schema | `internal/api/config_admin.go` 与 `/system/admin/config/schema` |
| 敏感信息脱敏 | 后端 redaction helper、schema `secret` 类型、前端禁止明文回显 |
| 路径安全 | `internal/api/safepath.go` |
| 前端 API | `webui/src/lib/api.ts` |
| i18n | `webui/src/lib/i18n.tsx` 与 `webui/src/locales` |

## 拆分大文件的顺序

优先按风险低、边界清晰、测试可覆盖的顺序拆：

1. 从大文件中提取纯类型、常量和小 helper，文件名保持同一业务域。
2. 抽出无 HTTP 依赖的业务函数，并补表驱动测试。
3. 将多个 handler 共用的流程改为 service 函数。
4. 只在同一业务域文件稳定后，再考虑新 package。
5. 每一步保持路由、响应结构、错误码、审计日志和配置写入路径不变。

当前优先候选：

| 文件 | 建议拆分方向 |
| ---- | ------------ |
| `internal/api/handlers.go` | 用户自助、管理员用户、Emby 绑定相关逻辑分文件 |
| `internal/api/business.go` | 注册码、邀请树、批量结果、用户排序分别靠近业务域 |
| `internal/api/config_admin.go` | schema 定义、值映射、TOML 保护、备份恢复分文件 |
| `internal/api/app_test.go` | 按业务域拆成聚焦测试文件 |
| 大型前端页面 | 抽成 `components/admin/*` 业务面板，页面保留路由组合 |

## 新功能落地流程

1. 找到业务域和现有文件。
2. 确认 feature gate、权限等级、审计日志和错误码。
3. 先设计 store 状态和原子方法。
4. 写 service/helper，避免直接依赖 HTTP。
5. 写 handler 并注册路由。
6. 同步 `webui/src/lib/api.ts`、`api-types.ts`、文案和页面。
7. 更新 API 文档、功能文档和本指南相关章节。
8. 按风险运行 Go 测试、前端 lint/build。

## Review 清单

- [ ] 是否引入第二套配置源、第二套权限判断或第二套审计写入？
- [ ] 是否绕过 store 直接写状态文件？
- [ ] 是否在业务层依赖 `http.Request` 或具体前端字段？
- [ ] 是否把外部副作用放在本地权限校验之前？
- [ ] 是否泄露 Token、Secret、密码、API Key、数据库 URL 或服务器线路？
- [ ] 是否新增了未说明 TTL/失效策略的缓存？
- [ ] 是否新增后台状态变更但没有审计日志？
- [ ] 是否新增用户可见文案但没有补 i18n？
- [ ] 是否新增 API 但没有同步前端客户端和文档？
- [ ] 是否改变现有路由、错误码或响应结构但没有兼容说明？
