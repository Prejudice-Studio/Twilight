# Twilight V2 Web UI

V2 是独立的 SvelteKit SSR 前端，使用 `@sveltejs/adapter-node` 运行。它与 V1 `webui/` 并存，按功能逐步迁移；迁移完成前不要把 V2 当作完整的生产替代品。

## 开发

```bash
pnpm install --frozen-lockfile
pnpm check
pnpm dev
```

默认从 `BACKEND_URL`（未设置时为 `http://127.0.0.1:5000`）访问 Go API。复制 `.env.example` 为本地环境文件并按部署环境修改；环境文件不要提交。

## 当前范围

目前已实现 SSR 壳层、登录/注册/登出、服务端会话读取、个人设置（偏好、邮箱验证、密码安全、Emby 绑定）、公告、在线人数摘要、签到与积分、用户邀请中心、用户工单、Bangumi 用户端、求片中心、管理员服务器状态页、管理员用户管理页、管理员工单处理页、管理员邀请系统页、管理员操作日志页、管理员公告管理页、管理员运行日志页、管理员配置管理页、管理员数据库维护页、管理员 Emby 管理页、管理员 Telegram 管理页、Telegram 换绑审批页和管理员邮箱管理页，以及仅限 `/api/v1` /`api/v2` 的同源代理。Bangumi 首屏使用 `/api/v2/bangumi/summary` 聚合本地同步状态、公开账号资料和有限预览；收藏详情使用服务端分页，Token 只留在服务端请求链路中。管理员状态、用户、工单、邀请、操作日志、公告、运行日志、配置、数据库、Emby、Telegram、换绑审批和邮箱页由管理员服务端布局保护；邀请森林只把当前 300 行批次与当前详情发送给浏览器，邀请维护、Telegram 配置、Bot 测试、换绑审核和邮箱维护使用 form action；用户/管理员工单列表与操作日志、公告列表和邮箱审查使用摘要分页，详情、回复、附件和审计详情按需读取；Emby 账号、设备/IP 审查、活动日志和 Telegram 花名册均为手动读取，运行日志不使用浏览器 SSE/轮询，配置、数据库、Telegram、换绑和邮箱页的敏感数据、快照和高风险操作均在 SSR/form action 边界处理。其余 V1 功能必须按照 [V2 SSR 前端迁移规则](../docs/v2/frontend-ssr.md)逐模块迁移后才能声明支持。

认证辅助页 `/forgot-password` 与 `/setup` 也已经迁移为服务端 `load`/`actions`：找回密码凭据只经过 SSR action 转发，初始化完成使用服务端复制的 host-only HttpOnly 会话 Cookie；浏览器不保存 Token、密码或 SMTP/Bot Secret。管理员调度器 `/admin/scheduler` 和 Telegram 管理 `/admin/telegram` 使用 SSR 摘要、按需读取和 form action，不使用浏览器轮询。其余未迁移功能继续使用 V1 回退入口。

## 设计边界

管理员入口页、违规审计页、邀请系统页、求片管理页、注册码管理页和邮箱管理页已迁移到 `/(app)/admin`、`/(app)/admin/violations`、`/(app)/admin/invite`、`/(app)/admin/requests`、`/(app)/admin/regcodes`、`/(app)/admin/email`，使用 SSR 摘要/分页筛选和服务端表单操作；其余未迁移功能仍以 V1 为回退入口。

- Go 后端是唯一认证、权限、限流、审计和业务状态边界。
- 认证 Cookie 只由服务端转发，浏览器脚本不持有 Bearer Token。
- 当前用户与权限数据 no-store；没有跨用户共享缓存。
- API JSON 响应有 8 MiB 读取上限，代理请求/响应使用 32 MiB 流式上限。
- 所有移动端页面必须以 Firefox 为基准并拥有自己的有限滚动区域。
