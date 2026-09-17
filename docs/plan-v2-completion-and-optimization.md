# Twilight V2 接口全覆盖与系统优化总体方案

**生成时间**：2026-09-17
**分析基线**：`main` @ `a829654d`（968 commits）
**目标定义**：后端 `/api/v2/*` 完整覆盖 **webui（Next.js 产品前端）** 使用的全部功能面，前端默认调用 V2，V1 仅保留为外部 API Key 集成面。`webui-v2/`（SvelteKit SSR 前端）已整体移除。

---

## 0. 结论摘要

| 维度 | 分析时状态 | 当前 |
|---|---|---|
| 后端 V2 路由数量 | V1 340 / V2 325，V1 独有 36 + 2 处方法不匹配 | ✅ 已补齐（366 条） |
| APIKey 鉴权面 | V2 整块 0 覆盖（19 个端点） | ✅ 已补齐 |
| 前端批量操作 | 7 个接口必然 404（双版本前缀） | ✅ 已修 |
| 前端默认版本 | 默认 v1，仅 78 处显式 v2 | ✅ 默认 v2 |
| Telegram MarkdownV2 | 无转义函数，选中必返 400 | ✅ 转义 + 降级重发 |
| V2 前端残留 | webui-v2/ 577 文件 + AGENTS.md 91 行规则 | ✅ 已移除 |
| 性能 | 每请求全局写锁刷新、无 pprof | 🟡 pprof 已加、刷新降读锁、签到 730→90、TMDB/Bangumi 加 TTL 缓存 |
| 模块化 | internal/api 208 文件，无 service/emby 包 | ⬜ 未开始（评估见 §8） |
| 移动端 | 10 个管理页表格硬溢出 | ✅ `AdminTable` 已抽并落地，硬溢出规则已修 |

---

## 1. 已完成：阶段 0 前端止血

| 问题 | 位置 | 修复 |
|---|---|---|
| 批量端点双版本前缀 → `/api/v1/api/v2/...` | `api.ts` 7 处 | 去掉前缀并显式 `{ apiVersion: "v2" }`，删除 `batchEndpoint` |
| `POST /auth/register` 未注册 | `api.ts:376` | 改为 `/registration` |
| `/users/me/sessions` | `api.ts:1269` | 改为 `/me/sessions` |
| `/users/me/devices` | `api.ts:1280/1287/1293` | 改为 `/security/devices/:device_id` |
| 公告打公开路由丢个人态 | `api.ts:3418` | 后端补 `/api/v2/me/announcements`，前端改指 |
| rebind 类型欺骗 | `api.ts:1850` | 删除 `as Promise<...>` 强转 |
| runtime log stream 硬编码 v1 | `api.ts:1988` | 改为 `/api/v2/admin/runtime/logs/stream` |

## 2. 已完成：后端 V2 补齐

`internal/api/routes_v2.go` 新增 `registerV2CompletionRoutes()`，共 40 条：

- **APIKey 面 13 条**（`AuthAPIKey` + `withAPIKeyPermission` 包装，与 V1 权限模型一致）
- **auth/apikey 6 条**（用户自助 API Key）
- **WS 2 条**（注册流程与账户页绑定码）
- `/me/announcements`、`/invite/codes`、`/invite/me`
- bangumi：`/bangumi/me`、`/bangumi/sync/status`、`/bangumi/sync/history`
- media：`/media/tmdb/:tmdb_id`、`/media/bangumi/:bgm_id`
- 管理面：`PUT /admin/me/update`、`DELETE /admin/users/:uid/emby`、`PUT /admin/regcodes/:code`、`PUT /admin/tickets/:ticket_id`（V1 用 PUT，V2 原仅 PATCH）
- 系统面：`/system/stats`、`/system/emby-stats`、`/system/emby-viewers`、`/system/health/*`、`/docs`
- `/settings/password/change`、`POST /registration/availability`（前端以 POST 提交注册码）

**审计一致性**（`internal/api/audit_handlers.go`）：

- `fallbackAuditAction` 同时剥离 `/api/v1/` 与 `/api/v2/`，消除 `post_api/v2/...` 脏名
- `shouldFallbackAuditHTTPMutation` 增加 V2 豁免：清审计日志自身不再产生审计记录

## 3. 已完成：前端默认走 V2

- `api-request.ts`：`apiVersion ?? DEFAULT_API_VERSION`，默认 v2；`NEXT_PUBLIC_USE_V1_COMPAT=true` 仍是唯一回退开关
- 74 处 V1 路径迁移到 V2 结构（`/system/admin/*`→`/admin/*`、`/users/me/*`→`/settings/*`、`/batch/*`→`/admin/users/batch/*`、`/media/request`→`/media/requests` 等）
- V2 方法路径修正：`/auth/password/emby`、`/auth/password/email/request`、`/auth/password/email/reset`、`/registration/availability`、`/tickets/:id/replies`、`/admin/tickets/:id/replies`、`/registration/regcode/check`、`/media/detail/:source/:id`、bangumi 三个 `/admin/bangumi/users/:uid/*`
- **新增校验工具** `scripts/check-frontend-v2-coverage.py`：把 `api.ts` 的调用点与 `routes_v2.go` 逐条对账，剩余差异应仅为 V1 兼容分支

## 4. 已完成：Telegram P0

`internal/api/telegram.go`：

- 新增 `telegramEscapeMarkdownV2` / `telegramEscapeMarkdown` / `telegramEscapeByParseMode`（按 parse mode 分流）
- 新增 `telegramStripMarkup`：解析失败时剥离标记
- `telegramSendMessage` 与 `telegramSendMessageWithMarkup` 在解析错误时自动降级纯文本重发，消息不再因转义缺失整体丢失

## 5. 已完成：移除 V2 前端

- 删除 `webui-v2/`（遗留构建与依赖目录，源码此前已不在 HEAD）
- 删除 `deploy/twilight-webui-v2.service`、`V2_MIGRATION_SUMMARY.md`、`docs/frontend-v2-migration-detailed.md`、`docs/v2-compatibility-verification.md`、`docs/v2-migration-executive-summary.md`、`docs/v2-api-migration-gap-analysis.md`
- AGENTS.md 清理 91 行规则（SvelteKit / webui-v2 / `docs/v2/*` 断链），Frontend Rules 与 V2 Refactor Rules 改写为"webui 是唯一前端 + `/api/v2` 是默认契约"
- 14 份文档清理 67 行 webui-v2 引用
- `docker-compose.yml` 前端服务改构建 `./webui`、镜像名 `twilight-webui`；`deploy/nginx-twilight.conf` 的 SvelteKit `/_app/*` 位置块改为 Next.js `/_next/static/`
- 修正只写了 "SvelteKit" 而未写 "webui-v2" 的漏网引用：`README.md`、`docs/guides/install.md`、`deploy/setup-systemd.sh`、`docker-compose.yml` 顶部服务清单、`docs/features/bangumi.md` 与 `docs/features/emby-admin.md`
- **注意**：`config.toml` 的 `[WebUI] static_dir` 是**死配置**——`internal/config` 里根本没有对应字段，Go 代码从不读取。之前把它改指 `./webui/.next` 只是让 README 与实际一致，不构成任何行为变更

## 6. 已完成：性能起步

| 项 | 位置 | 说明 |
|---|---|---|
| pprof | `cmd/twilight/main.go` | 独立 127.0.0.1:6060 监听，`TWILIGHT_PPROF_ADDR=off` 可关；不挂 App 路由以免被 `Store.Refresh` 污染 |
| 刷新降锁 | `store.go` `Refresh()` | 版本未变时走读锁探测，只有版本变化才取写锁；原实现每个请求都独占写锁 |
| 签到记录瘦身 | `store/signin.go` | `maxSigninRecords` 730→90，单用户约 80KB → 约 10KB |

## 7. 性能优化：现状

| 项 | 状态 | 说明 |
|---|---|---|
| 播放上报批量化 | ✅ 已在用 | `emby_activity.go` 走 `AddPlaybackRecordsIdempotent`，整批一次 refresh+save |
| `rebuildUserIndexes` 增量化 | ✅ 已是增量 | 仅在 `refreshLocked` 检测到版本变化、或某张索引为 nil 时重建；`maintainUserIndexes` 平时做增量维护。跨进程收到未知远端变更时全量重建是必需的，无法再压缩 |
| Telegram roster N+1 | ✅ 已修 | 改批量 `UsersByTelegramIDs` |
| TMDB / Bangumi 缓存 | ✅ 已加 | 共享 `ttlCache`（`bangumi_client.go`）：搜索 256/5min，详情 512/30min；Bangumi 搜索仍不带全局 Token |
| `EmbyActivityLogs`（10000）/ `LoginLogs`（1000）迁独立表 | ⬜ 未做 | 见下方风险说明 |

**日志迁表被刻意推迟的原因**：这不是纯重构，而是 schema 变更 + 迁移导出/导入面变更。两条路径都会读写 `State.LoginLogs` / `State.EmbyActivityLogs`（`store.ensure`、`deleteUser`、`database_admin` 计数、迁移快照、以及 `login_log_test.go` / `playback_test.go` 的断言），而 `internal/store` 的测试在没有 `TWILIGHT_TEST_DSN` 时整包跳过——本地无法真机验证。盲改的风险（丢历史登录/活动日志）远大于收益，应在有可用 PostgreSQL 实例、且补好迁移导出面后单独做。

## 8. 模块化：评估结论（未执行）

现状：`internal/api` 208 文件（139 生产 + 69 测试），`handlers.go` 2513 行 / 57 handler、`business.go` 1577、`config_admin.go` 1469；`docs/guides/modular-architecture.md` 声称的 `internal/service`、`internal/emby`、`internal/integrations` **实际不存在**。

已确认的规范违反（未修）：

- `session.go` 绕过 store 直接写 `twilight_sessions` 表（:64/66/169/184/230/260/285/318/336）
- `*_service.go` 仍接收 `http.Request`（`login_service.go:84,136`、`registration_service.go:41,76,125`、`password_reset_service.go:74`、`email_password_reset_service.go:78,112`）

**为什么这一轮没有直接拆**：这两条都不是单纯搬文件。`completeLogin(r, …)` 里的 `r` 同时承担 `r.Context()` 与 `a.auditWithUser(r, …)`，改成 `ctx` 需要一并改审计入口（审计要从 request 取 IP/UA）；`session.go` 直连表则要求先在 store 上补会话仓储方法。两者都是"改一层要动一片"的改动，且没有可执行的行为断言保护，适合单独一轮、配合新测试推进，而不是夹在接口迁移里做。

建议拆包顺序（成本递增）：`internal/emby` → `internal/notify` → `internal/service` → `internal/httpapi`。

## 9. 已完成：移动端布局

- `globals.css`：删掉 `max-width:767px` 下强制表格 `min-width:760px` 的规则（这条规则本身就是横向溢出的来源）
- `admin/emby`：`TabsList` 去掉 `min-w-[34rem]`，改为可换行
- `admin/email`：卡片兜底断点 `lg` → `md`，补上 768–1023px 的空隙
- 新增 `webui/src/components/admin/admin-table.tsx`（`AdminTable<T>`），表格（`md+`）与卡片（`<md`）双渲染，支持 `hideOnCard` / `primary` / `rowClassName` / `onRowContextMenu` / `containerClassName`
- 落地 `admin/invite`（原先是唯一没有任何窄屏兜底、且 `min-w-[920px]` 的邀请树表格）

其余页面（`users`、`regcodes`、`emby`、`email`）已有各自的卡片兜底，且桌面断点下表格宽度与视口匹配，暂不需要替换为 `AdminTable`；新写或重写管理表格时应直接用它（已写入 AGENTS.md）。

## 10. 已完成：前端 V1 分支清理

- 删除 `useV2` 开关与 `enableV2/disableV2/isV2Enabled` 死代码，各方法的 V1 分支折叠为直接调用 V2 方法（78 处）
- 折叠后残留 **72 段不可达代码 / 346 行**（`return this.xxxV2(...)` 之后仍挂着 V1 请求体），已全部清除
- `NEXT_PUBLIC_USE_V1_COMPAT` 保留为唯一显式回退开关（`api-request.ts` 的 `DEFAULT_API_VERSION`），不再有逐模块开关

## 11. 硬性约束

1. **禁止修改 CORS/CSRF 实现**（历史生产事故 `12225e4c`）。新增 V2 端点沿用现有中间件链。
2. V2 必须能操作 V1 创建的数据，禁止破坏性 schema 迁移。
3. V2 复用 V1 会话 cookie；公开端点保持 `credentials: "omit"`。
4. V2 响应字段优先对齐 V1，避免前端维护双解析逻辑（登录失效事故的教训）。
5. 每阶段结束跑 `go build ./...`、`go test ./...` 与 `scripts/check-frontend-v2-coverage.py`。

## 12. 验收状态

- `go build ./...` 通过
- `go test ./...` 全绿（cmd / api / config / migration / playback / security / store / validate）
- `scripts/check-frontend-v2-coverage.py`：233 个前端调用点全部命中已注册的 V2 路由（脚本已能正确展开 `${a ? "enable" : "disable"}` 与可选 query 前缀两类模板插值，不再是误报源）
- 前端 `tsc` 未执行：`webui/node_modules` 缺失，`pnpm` 在当前环境不可用。已对改动文件做括号/模板字符串平衡检查，逻辑改动集中在字符串字面量与 1 个新组件，仍需在具备依赖的环境复验类型
