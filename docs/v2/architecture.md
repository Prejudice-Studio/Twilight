# Twilight V2 架构设计

> 本文承接 [V1 审计基线](./v1-audit.md)，描述当前默认 WebUI 的 V2 架构和兼容边界。V2 允许更换接口与前端实现，但必须完整保留 V1 用户可见功能并兼容现有配置；旧 V1 只作为紧急回滚版本保留。

## 1. 设计原则

1. 后端权限、状态机和数据完整性优先于界面便利；前端只负责表达和交互。
2. 一个业务字段只有一个权威来源。缓存是可丢弃加速层，不是第二数据库。
3. 写操作使用明确的事务、幂等键或版本条件；外部副作用与本地状态要有可恢复边界。
4. 首屏只加载当前视图需要的数据；列表、详情、统计、健康检查和外部服务探测彼此隔离。
5. 移动端和 Firefox 是基准。任何固定宽度控件必须在窄视口拥有可计算的降级布局和自己的滚动上下文。
6. 每次拆分只改变一个清晰边界，保留兼容适配和可回滚迁移，不进行无证据的大面积重写。

## 2. 目标分层

```text
cmd/twilight
  -> transport / route adapter
       -> auth + policy
            -> application service
                 -> domain state transition
                      -> repository / transaction
                      -> external integration
       -> response / audit / metrics

SvelteKit SSR route shell (V2)
  -> server load / form action
       -> typed server API client
            -> same-origin API proxy or backend
                 -> Go auth / application service
```

### 后端层

| 层 | 责任 | 禁止事项 |
| --- | --- | --- |
| transport | 路由匹配、JSON/multipart 解码、超时、响应 envelope | 业务判断、直接写数据库 |
| auth/policy | session/API Key、资源归属、角色和 feature gate | 依赖前端隐藏按钮 |
| application | 编排一个完整用例、事务边界、幂等和审计事件 | 读取 `http.Request` 或拼前端字段 |
| domain | 状态机、规则、值对象、纯计算 | 直接发 HTTP、写日志副作用 |
| repository | 参数化 SQL、事务、分页、版本条件 | 暴露整个数据库连接给 handler |
| integration | Emby、Telegram、TMDB、Bangumi、SMTP 协议 | 越权决定、直接修改业务状态 |

### 前端层

| 层 | 责任 |
| --- | --- |
| route shell | 路由、布局、权限入口和懒加载 |
| feature | 单一业务页面和局部状态，不持有全局配置副本 |
| query controller | AbortController、序列号、重试、可见性轮询、错误隔离 |
| API client | 类型、路径、请求参数、资源 URL 归一化 |
| transport | credentials、超时、缓存/合流、响应解析、错误码 |
| UI primitives | 稳定尺寸、Firefox 滚动、无障碍名称和 i18n |

V2 前端实现约定：

- `webui-v2` 使用 SvelteKit SSR 和 `@sveltejs/adapter-node`，服务端 `load` 负责首屏数据，form action 负责写操作，默认启用渐进增强而不是把整页变成客户端应用。
- 浏览器只访问 V2 自身的同源路径。V2 服务端 API client 将请求转发到 Go 后端，认证 Cookie 只在服务端读取和转发；浏览器端不保存 Bearer Token，也不把用户身份放进跨页面 JS 缓存。
- V2 的 `/api/[...path]` 代理只允许 `v1` / `v2` API 版本，限制请求体、移除 hop-by-hop 与浏览器 Origin/Referer 头，并保持上游状态码。它不是鉴权替代品，真正权限仍由 Go 后端执行。
- V1 Next.js 应用保留为整站紧急回滚；每个 V2 模块仍必须记录 V1 页面、V1 API、V2 route/load/action、权限、审计、移动端验证和回滚入口。

## 3. V2 API 策略

### 3.1 版本与兼容

- 新接口采用 `/api/v2`，不直接改变 `/api/v1` 的响应 envelope、错误码和鉴权语义。
- V1 接口先由兼容 adapter 调用 V2 application service；同一状态转移不能保留两份业务实现。
- 每个 V1 route 在迁移表中标记 `adapter`、`native`、`deprecated` 或 `removed-with-migration`。
- 只有明确无法安全兼容的字段才版本化删除，并在 API 文档和启动诊断中给出迁移说明。

### 3.2 读取接口

- 列表统一 `cursor` 优先，保留受限 `page/per_page` 兼容参数；服务端强制上限。
- 列表响应只放摘要和稳定的 `next_cursor`，详情、replies、图片和大字段按需读取。
- 聚合接口只聚合同一页面真实需要的资源，返回每个子资源的独立状态，不能因一个外部服务失败丢弃成功数据。
- 健康检查按 API、数据库、Emby 三个独立接口；公共 liveness 不探测私有依赖。
- 任何“当前用户”读取为 no-store、会话范围数据，不进跨用户共享缓存。

### 3.3 写接口

- 关键写操作支持 `Idempotency-Key` 或业务唯一键；重复提交返回原操作结果，不重复扣费、续期、发码或发消息。
- 并发编辑使用 `If-Match`/revision；冲突返回明确错误和当前版本摘要。
- 请求体限制大小、字段数量、嵌套深度和字符串长度；后端拒绝未知危险字段或明确按 schema 忽略并记录诊断。
- 外部副作用按“本地预检 → 远程操作 → 本地状态确认/补偿”执行；不能在权限失败后先触碰 Emby/Telegram。
- 所有状态变更生成结构化 audit event；审计失败策略按风险级别定义，不能静默吞掉安全关键事件。

## 4. 数据库迁移路线

### 4.1 拆分顺序

1. 先建立 repository/application 接口，仍由 V1 Store 实现，保持行为不变。
2. 迁移会话、审计、运行日志和播放事件等已经有独立表的数据访问边界。
3. 迁移工单、回复/附件、注册码和邀请关系，建立唯一约束、外键或可验证的软删除规则。
4. 迁移用户身份、Emby/Telegram 绑定和用户偏好，保留 UID 稳定性。
5. 迁移媒体求片和 Bangumi 用户集合/全局元数据。
6. 最后收缩 `twilight_state`：只保留配置快照、迁移标记以及尚未拆出的低频兼容字段。

### 4.2 双读/双写规则

- 迁移批次有 schema version 和完成标记；服务启动重复执行必须幂等。
- 双写期间以旧 Store 的事务结果为主，写入新表失败则阻止切换或生成待修复任务，不能悄悄丢数据。
- 双读期间比较实体数量、哈希/版本和关键字段；差异进入运行诊断和管理员修复列表。
- 切换后保留只读回退窗口，删除旧 JSONB 字段前必须有备份和演练记录。
- 任何跨表写入都明确事务边界；不能用内存 map 先改、异步稍后补库来模拟原子操作。

## 5. 缓存与网络模型

### 权威来源

| 数据 | 权威来源 | 可缓存层 | 失效事件 |
| --- | --- | --- | --- |
| 当前用户/权限 | PostgreSQL + session | 仅请求内 | 登出、密码/角色/绑定变更 |
| 配置 | 运行时 immutable snapshot | 进程内 snapshot | 热重载成功 |
| Emby sessions/status | Emby 当前返回 | 短 TTL snapshot | 手动刷新、URL/token 变化、TTL 到期 |
| 设备审查 | 本地聚合结果 + Emby 手动刷新 | 手动作用域 | 手动刷新、Emby 配置变化 |
| Bangumi/TMDB 元数据 | 外部源 | 有上限 TTL cache | TTL 到期、明确源版本变化 |
| 审计/工单/注册码 | PostgreSQL | 浏览器 no-store 或短本地查询缓存 | 任何成功写操作 |

缓存实现必须记录容量、TTL、失效和降级方式。V2 不得把“缓存命中”当成权限检查，不能对 session-scoped data 做跨账号合流。

### 前端请求控制

- `api-request` 继续是唯一低层请求入口；GET/HEAD 合流只允许无 caller abort 的安全读取。
- 每个页面拥有一个加载控制器；新筛选、分页、刷新、卸载都取消旧控制器并提升序列号。
- 同一页面的统计/列表/详情并行请求使用 `Promise.allSettled`，各自更新，不让一个失败清空其他成功数据。
- 轮询采用可见性优先、固定最小间隔和请求完成后再排程；页面隐藏时停止，手动刷新始终可用。
- 图片使用 `loading=lazy`、尺寸占位、来源白名单和按需详情；不预取用户不会看到的全量海报。

## 6. 观看统计重构边界

观看统计不是简单对 Emby session 做累加。V2 需要持久化规范化播放事件或可重建的播放段：`playback_id`、UID、Emby item、设备、事件类型、客户端事件时间、服务器接收时间、客户端序号和来源。

状态机至少包括 `started`、`playing`、`paused`、`resumed`、`stopped`、`completed`、`expired`。服务端规则：

- 事件按 playback/device/sequence 幂等；重复重试不能再次累计。
- 每个活动段有最大时长和心跳过期时间；没有停止事件也不能无限累计。
- 一个播放跨日期时按配置时区切割到多个日桶，DST/时区变更按事件发生时的 zone 记录处理。
- 多设备分别维护 playback segment，再在统计查询时按用户和日期聚合。
- 只信任已认证用户/受信 Emby 活动映射；客户端上报的 duration 只作为提示，不能直接作为计费/统计事实。
- 完成、断线、客户端关闭和 scheduler 过期收敛到同一状态机，保留原始事件便于重算。

每日统计表应由事件或播放段生成，支持重建和校验；活动日志仍独立保留，不因启用统计而删除。

## 7. 导入导出设计边界

Twilight ZIP 包建议结构：

```text
manifest.json
data/state.json
data/users.jsonl
data/tickets.jsonl
data/media-requests.jsonl
data/audit.ndjson
resources/avatars/...
resources/backgrounds/...
resources/tickets/...
checksums/sha256.json
```

`manifest.json` 至少记录格式版本、数据库结构版本、Twilight 版本、导出时间、时区、实体计数、资源计数、是否加密和 KDF 参数。默认无密码；密码模式使用随机 salt、Argon2id 派生密钥和 AES-256-GCM，nonce 不复用，认证标签覆盖 manifest 和数据。

导入顺序固定为：格式验证 → 版本验证 → 完整性校验 → 密码验证 → 安全检查 → 兼容性检查 → 冲突预览 → 用户确认 → 数据库事务导入 → 资源原子落盘 → 完整性复核。解包使用临时目录并拒绝绝对路径、`..`、符号链接、超大压缩比、超大单文件、超多文件和超过总预算的资源；失败回滚数据库并清理孤儿资源。

## 8. 移动端和管理后台设计

- 使用 CSS grid/flex 的可收缩轨道，所有密集工具栏在手机先堆叠，在足够宽度才恢复横排。
- 表格不强行压缩到不可读；手机使用卡片或内部水平滚动，表头和操作组不能跑出视口。
- 对话框和聊天区域采用 `min-height: 0`、`max-height: 80dvh`、Firefox 标准滚动条和 `overscroll-behavior: contain`。
- 操作按账号状态、Emby、身份绑定、注册资格、危险操作分组；危险操作有明确确认、结果和审计。
- 文案走 i18n，按钮和状态使用一致术语；图标按钮必须有本地化 `aria-label`。
- 首屏只渲染当前页数据，管理大列表用服务端分页/游标和显式加载更多，不能用无限滚动隐藏数据规模。

## 9. 迁移与提交流程

每个模块遵循：事实审计 → 设计文档 → Store/repository → domain/application → route adapter → API client/types → 页面 → 测试 → 安全审查 → 性能测量 → 文档 → 中文小提交。

建议提交顺序：

1. V1 审计和 V2 架构文档。
2. V2 运行时、错误码、API envelope 和请求层骨架。
3. 身份、会话和配置兼容层。
4. 数据迁移、导入导出安全基础。
5. Emby/Telegram 与活动日志。
6. 用户、注册码、邀请、工单和求片。
7. 观看事件与每日统计。
8. 管理后台、移动端和性能收口。

每次提交前运行与改动相称的 Go/前端测试，并扫描本次提交的 diff：不得出现本机绝对路径、真实凭据、调试输出、无关文件或未记录的行为改变。
