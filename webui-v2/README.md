# Twilight V2 Web UI

V2 是 Twilight 的默认 SvelteKit SSR 前端，使用 `@sveltejs/adapter-node` 运行。旧的 V1 `webui/` Next.js 实现只作为紧急回滚和迁移对照保留；V2 页面通过服务端 `load` 和 form action 连接同一个 Go API 与 PostgreSQL 数据。V2 根布局强制 SSR，应用壳层和响应式导航由 `src/lib/components/AppShell.svelte` 统一承载，业务页面不得重新实现会话导航。

## 开发

```bash
pnpm install --frozen-lockfile
pnpm check
pnpm dev
```

默认从 `BACKEND_URL`（未设置时为 `http://127.0.0.1:5000`）访问 Go API。复制 `.env.example` 为本地环境文件并按部署环境修改；环境文件不要提交。

## 当前范围

V2 是默认生产 WebUI。Linux + systemd 使用 `twilight-webui-v2.service` 启动 `build`，Docker Compose 使用本目录的 `Dockerfile` 构建 adapter-node；旧 `webui/` 只作为整站紧急回滚和行为对照，不与 V2 共享浏览器状态。

目前已实现 SSR 壳层、登录/注册/登出、服务端会话读取、公开 Wiki、个人设置（偏好、邮箱验证、密码安全、Emby 绑定）、用户外观与头像、用户 API Key 管理、公告、在线人数摘要、签到与积分、用户邀请中心、用户工单、Bangumi 用户端、求片中心、管理员服务器状态页、管理员用户管理页、管理员工单处理页、管理员安全中心、管理员 Bangumi 管理页、管理员邀请系统页、管理员操作日志页、管理员公告管理页、管理员运行日志页、管理员配置管理页、管理员数据库维护页、管理员 Emby 管理页、管理员 Telegram 管理页、Telegram 换绑审批页和管理员邮箱管理页，以及仅限 `/api/v1` /`api/v2` 的同源代理。Bangumi 首屏使用 `/api/v2/bangumi/summary` 聚合本地同步状态、公开账号资料和有限预览；收藏详情使用服务端分页，Token 只留在服务端请求链路中；管理员 Bangumi 列表使用分页和按需详情，记录统计通过后端批量读取。管理员状态、用户、工单、邀请、操作日志、公告、运行日志、配置、数据库、Emby、Bangumi、Telegram、换绑审批和邮箱页由管理员服务端布局保护；邀请森林只把当前 300 行批次与当前详情发送给浏览器，邀请维护、Telegram 配置、Bot 测试、换绑审核和邮箱维护使用 form action；用户/管理员工单列表与操作日志、公告列表和邮箱审查使用摘要分页，详情、回复、附件和审计详情按需读取；Emby 账号、设备/IP 审查、活动日志和 Telegram 花名册均为手动读取，运行日志不使用浏览器 SSE/轮询，配置、数据库、Telegram、换绑、Bangumi 和邮箱页的敏感数据、快照和高风险操作均在 SSR/form action 边界处理。应用壳层使用共享导航元数据和原生可访问菜单，在窄 Firefox 视口将长管理员菜单限制在独立滚动区；账户头像只允许受保护的本地资源 URL。用户 API Key 列表只返回掩码，明文仅在创建 action 的当前响应中显示。其余 V1 功能必须按照 [V2 SSR 前端迁移规则](../docs/v2/frontend-ssr.md)逐模块迁移后才能声明支持。
旧的 `/admin/stats`、`/admin/test`、`/admin/device-audit`、`/admin/telegram/commands`、`/admin/developer/js-docs` 和 `/settings/background` 入口由服务端重定向到合并后的 V2 页面，不会产生重复的数据读取或健康探测。

认证辅助页 `/forgot-password` 与 `/setup` 也使用服务端 `load`/`actions`：找回密码凭据只经过 SSR action 转发，初始化完成使用服务端复制的 host-only HttpOnly 会话 Cookie；浏览器不保存 Token、密码或 SMTP/Bot Secret。管理员调度器 `/admin/scheduler` 和 Telegram 管理 `/admin/telegram` 使用 SSR 摘要、按需读取和 form action，不使用浏览器轮询。

## 设计边界

管理员入口页、违规审计页、邀请系统页、求片管理页、注册码管理页和邮箱管理页均位于 `/(app)/admin`、`/(app)/admin/violations`、`/(app)/admin/invite`、`/(app)/admin/requests`、`/(app)/admin/regcodes`、`/(app)/admin/email`，使用 SSR 摘要/分页筛选和服务端表单操作。V1 仅作为整站回滚入口，不与 V2 共享页面状态。

- Go 后端是唯一认证、权限、限流、审计和业务状态边界。
- 根布局显式关闭 prerender；认证页面和管理员页面不会输出静态会话 HTML。
- SSR API 转发使用 15 秒请求截止时间、响应大小上限和当前请求取消信号；超时只显示通用错误，不泄露上游诊断。
- 认证 Cookie 只由服务端转发，浏览器脚本不持有 Bearer Token。
- 当前用户与权限数据 no-store；没有跨用户共享缓存。
- API JSON 响应有 8 MiB 读取上限，代理请求/响应使用 32 MiB 流式上限。
- 所有移动端页面必须以 Firefox 为基准并拥有自己的有限滚动区域。
V2 已覆盖当前 V1 的全部页面路由；旧版前端仅保留为整站回滚和行为对照，不参与默认开发、构建或运行时数据边界。
