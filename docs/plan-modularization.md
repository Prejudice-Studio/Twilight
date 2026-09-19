# 模块化与解耦计划

本文记录 2026-09-19 对全项目做的一轮结构审计结论，以及**已经落地**和**待执行**的改造项。目标不是推倒重写，而是让每一步都能单独提交、单独跑通测试。

相关文档：安全边界见 [安全加固](../guides/security.md)，播放榜单见 [播放排行榜](../features/playback-rank.md)。

## 1. 现状

| 文件 | 规模 | 说明 |
| --- | --- | --- |
| `internal/store/store.go` | ~180 KB / 5480 行 | 全项目核心巨型文件，但**内部按领域连续聚集** |
| `internal/api/handlers.go` | ~100 KB | 已拆分过一次，剩余部分仍混装多个领域 |
| `internal/api/config_admin.go` | ~92 KB | 配置管理 |
| `internal/api/developer_handlers.go` | ~92 KB | 开发者 JS 沙箱 |
| `internal/api/telegram_js_commands.go` | ~77 KB | 沙箱命令 |
| `internal/api/app.go` | ~71 KB | 路由 / 鉴权基础设施 |

包结构本身是健康的：`store` **不** import `api`，不存在循环依赖；`App` 的热字段用 `atomic.Pointer` 整体切换（刻意的无锁热重载，并发安全，不要改成逐字段加锁）。

## 2. 已落地

### 2.1 请求参数解析收敛到 `internal/api/params.go`

此前 `queryInt` / `clamp` / `max` 躺在 `business.go`、`minInt` 躺在 `runtime_logs.go`，其余 handler 则各写一遍 `strconv.ParseInt` + 忽略错误。散落的代价不是"重复代码"，而是**每个 call site 都要自己决定解析失败怎么办**：多数写成 `_` 吞掉错误，`limit=abc` 于是静默变成 `0`，而这个 `0` 究竟表示"没传"（无害）还是"取第 0 条 / UID 0"（越界），只有读那个 handler 的人才知道。

现在统一为 `queryInt` / `queryInt64` / `queryIntClamped`（含上下界夹取），已替换 `playback_handlers.go`、`runtime_logs.go`、`audit_handlers.go` 共 8 处内联解析。约定：

- 可选展示类参数（limit / since / 筛选条件）解析失败回退 fallback，**不返回 400**——把 `limit=abc` 变成请求失败会打破既有客户端；
- 必填路径参数继续走 `int64Param`（返回 error，由调用方决定 400）；
- 上限/下限写成 call site 上可见的数字，不再埋在 `if parsed > 0 && parsed <= 1000` 里。

### 2.2 出站 SSRF 补上拨号阶段校验

原先 URL 层校验**只对字面 IP 生效**，配置里填域名就完全绕过（含 DNS rebinding）。现在 `sharedHTTPTransport` 的 `net.Dialer.Control` 上挂了 `guardOutboundDialAddress`，在拿到解析后的 IP 时再判一次。详见 [安全加固 §2](../guides/security.md)。

## 3. 待执行（按性价比排序）

### 3.1 冻结 V1，消除双套实现的行为漂移 —— 优先级最高

V1（~382 路由）与 V2（~220 路由）并存。V1 主要服务老客户端与外部 API Key 集成，本身有存在理由；真正的问题在于**部分 V2 handler 是独立重实现而非委托**：

- 用户侧工单是**干净的 facade**（`handleV2CreateTicket` 直接转调 `handleCreateTicket`），无漂移；
- 媒体请求（`handleV2CreateMediaRequest` vs `handleCreateMediaRequest`）、邀请码、regcode、admin users 则是**各写一遍**。

`route_shadow` 门禁只能查重复注册与覆盖缺口，**查不出行为差异**——两套管不通的校验、不一致的错误码，只有逐对比对才看得见。这是真实的设计漏洞，优先级高于纯结构问题。

做法：新端点只进 V2；把上述独立实现改为委托 V1，逐对补齐协议测试（同一请求打 V1/V2，断言响应一致）。

### 3.2 `store.go` 按领域切分 —— 机械搬移，风险低

边界已经清晰，按行段连续聚集：

| 领域 | 行段 | 跨依赖 |
| --- | --- | --- |
| State/Store 核心 | 665–1655 | 高，最后切 |
| User | 1657–3424 | 最高，最后切 |
| APIKey | 3487–3737 | 低 |
| MediaRequest | 3737–3935 | 低 |
| BindCode | 3935–4048 | 低 |
| Announcement | 4183–4331 | 低 |
| InviteCode | 4476–4790 | 低 |
| RegCode | 4814–5102 | 低 |
| RebindRequest | 5111–5258 | 低 |
| ViolationLog | 5313–5369 | 低 |
| Telegram offset | 5369–5476 | 低 |

**先切叶子**（Telegram / Violation / Announcement / Invite / RegCode，零跨依赖），最后再动 User 与核心段。每次只搬一个领域、签名不变，验证靠 `go test ./internal/store` + `route_shadow` + `check_docs_drift`。

### 3.3 `session.go` 的 SQL 收进 store

`internal/api/session.go` 直接操作 `twilight_sessions` 表（`session.go:64,66,168,184,229,260,318,335`），是 store 之外的**第二条数据访问路径**。隔离目前没问题，但破坏了"数据访问只走 store"的约定，审计时容易漏看。建议新增 `Session*` 方法后替换。风险中等（涉及登录链路）。

### 3.4 `handlers.go` 拆出 telegram / device / export 子文件

机械移动，无逻辑改动。`app.go` 是路由与鉴权基础设施，**建议暂缓**——它的风险收益比最差。

## 4. 明确不做的事

- **不推倒重写**：现网部署在跑，任何大爆炸式重构的回归面都无法在本项目的测试覆盖下兜住。
- **不动 `applyCORS`**：源码里标了「严禁修改 CSRF/CORS - V1 生产兼容性要求」。其风险组合（`cors_origins` 留空 / `*` + `allow_credential = true`）已在安全文档 §3.1 标注，运维侧显式填白名单即可规避，不需要改代码。
- **不拦 RFC1918 出站**：自托管 / docker-compose 普遍把 Emby 指向内网地址，一刀切会打死现网部署。需要更严格限制的应在出口防火墙层做。
- **不统一行尾**：仓库里大量文件是 CRLF，`gofmt -l internal/api` 会因此报出近百个"未格式化"文件——那是换行差异造成的整文件重写，不是真的格式问题。**只对自己改动过的文件跑 `gofmt -w`**，不要批量格式化整个包。
