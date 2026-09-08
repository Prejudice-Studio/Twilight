# V2 前端路由矩阵

本文是旧版 `webui/src/app` 页面与默认 `webui-v2` SSR 页面之间的可审计映射。路径中的 `(auth)`、`(main)` 和 `(app)` 是 SvelteKit/Next.js 的目录分组，不会出现在浏览器 URL 中；`[param]` 表示动态路径参数。

## 旧路由覆盖

每一条旧页面路由都必须满足以下之一：

- 有对应的真实 SvelteKit `+page.svelte`，并由 `+page.server.ts` 提供 SSR `load` / form action 边界。
- 是明确的服务端兼容重定向，不复制页面、请求或状态。
- 根入口 `/` 是服务端重定向到登录页或仪表盘的特殊入口。

| 旧版路由 | V2 实现 | 类型 |
| --- | --- | --- |
| `/` | `src/routes/+page.server.ts` | 根入口重定向 |
| `/forgot-password` | `src/routes/forgot-password` | SSR 页面 |
| `/login` | `src/routes/login` | SSR 页面 |
| `/register` | `src/routes/register` | SSR 页面 |
| `/setup` | `src/routes/setup` | SSR 页面 |
| `/wiki` | `src/routes/wiki` | 公开 SSR 页面 |
| `/admin` | `src/routes/(app)/admin` | SSR 页面 |
| `/admin/announcements` | `src/routes/(app)/admin/announcements` | SSR 页面 |
| `/admin/audit-logs` | `src/routes/(app)/admin/audit-logs` | SSR 页面 |
| `/admin/bangumi` | `src/routes/(app)/admin/bangumi` | SSR 页面 |
| `/admin/config` | `src/routes/(app)/admin/config` | SSR 页面 |
| `/admin/database` | `src/routes/(app)/admin/database` | SSR 页面 |
| `/admin/developer` | `src/routes/(app)/admin/developer` | SSR 页面 |
| `/admin/developer/js-docs` | `src/routes/(app)/admin/developer/js-docs/+page.server.ts` | 服务端重定向 |
| `/admin/device-audit` | `src/routes/(app)/admin/device-audit/+page.server.ts` | 服务端重定向 |
| `/admin/email` | `src/routes/(app)/admin/email` | SSR 页面 |
| `/admin/emby` | `src/routes/(app)/admin/emby` | SSR 页面 |
| `/admin/invite` | `src/routes/(app)/admin/invite` | SSR 页面 |
| `/admin/logs` | `src/routes/(app)/admin/logs` | SSR 页面 |
| `/admin/regcodes` | `src/routes/(app)/admin/regcodes` | SSR 页面 |
| `/admin/requests` | `src/routes/(app)/admin/requests` | SSR 页面 |
| `/admin/scheduler` | `src/routes/(app)/admin/scheduler` | SSR 页面 |
| `/admin/security` | `src/routes/(app)/admin/security` | 静态导航页（由管理员布局保护） |
| `/admin/stats` | `src/routes/(app)/admin/stats/+page.server.ts` | 服务端重定向 |
| `/admin/telegram` | `src/routes/(app)/admin/telegram` | SSR 页面 |
| `/admin/telegram-rebind-requests` | `src/routes/(app)/admin/telegram-rebind-requests` | SSR 页面 |
| `/admin/telegram/commands` | `src/routes/(app)/admin/telegram/commands/+page.server.ts` | 服务端重定向 |
| `/admin/test` | `src/routes/(app)/admin/test/+page.server.ts` | 服务端重定向 |
| `/admin/tickets` | `src/routes/(app)/admin/tickets` | SSR 页面 |
| `/admin/tickets/[ticketId]` | `src/routes/(app)/admin/tickets/[ticketId]` | SSR 页面 |
| `/admin/users` | `src/routes/(app)/admin/users` | SSR 页面 |
| `/admin/violations` | `src/routes/(app)/admin/violations` | SSR 页面 |
| `/announcements` | `src/routes/(app)/announcements` | SSR 页面 |
| `/bangumi` | `src/routes/(app)/bangumi` | SSR 页面 |
| `/bangumi/collections/[type]` | `src/routes/(app)/bangumi/collections/[type]` | SSR 页面 |
| `/dashboard` | `src/routes/(app)/dashboard` | SSR 页面 |
| `/invite` | `src/routes/(app)/invite` | SSR 页面 |
| `/media` | `src/routes/(app)/media` | SSR 页面 |
| `/score` | `src/routes/(app)/score` | SSR 页面 |
| `/settings` | `src/routes/(app)/settings` | SSR 页面 |
| `/settings/apikey` | `src/routes/(app)/settings/apikey` | SSR 页面 |
| `/settings/appearance` | `src/routes/(app)/settings/appearance` | SSR 页面 |
| `/settings/background` | `src/routes/(app)/settings/background/+page.server.ts` | 服务端重定向 |
| `/tickets` | `src/routes/(app)/tickets` | SSR 页面 |

## V2 新增入口

这些入口不是旧版页面覆盖数量的一部分，但属于默认生产前端：

| 路由 | 用途 |
| --- | --- |
| `/admin/migration` | 管理员迁移归档的 SSR 上传、预览和确认 |
| `/admin/status` | 独立 API、数据库、Emby 健康探测和运行摘要 |
| `/api-docs` | 公开 OpenAPI 与管理员完整路由目录 |
| `/logout` | 服务端 form action 注销当前会话 |

## 迁移边界

V2 页面只能通过 `src/lib/server/api.ts` 在服务端读取 Go API；写操作使用 SvelteKit form action。浏览器不保存后端 Bearer/API Key，不使用跨用户全局状态、SSE、WebSocket 或客户端轮询。旧版 `webui/` 只作为紧急回滚版本保留，不能重新成为默认生产入口。

