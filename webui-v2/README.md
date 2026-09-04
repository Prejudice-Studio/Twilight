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

目前已实现 SSR 壳层、登录/注册/登出、服务端会话读取、个人设置（偏好、邮箱验证、密码安全、Emby 绑定）、公告、在线人数摘要、签到与积分、用户邀请中心、用户工单、Bangumi 用户端、求片中心、管理员服务器状态页、管理员用户管理页、管理员工单处理页和管理员配置管理页，以及仅限 `/api/v1` / `/api/v2` 的同源代理。Bangumi 首屏使用 `/api/v2/bangumi/summary` 聚合本地同步状态、公开账号资料和有限收藏预览；收藏详情使用服务端分页，Token 只留在服务端请求链路中。管理员状态、用户管理、工单和配置页由管理员服务端布局保护；用户/管理员工单列表使用摘要分页，详情、回复和附件按需读取；配置页的 schema、脱敏 TOML 和备份元数据由 SSR 并行读取，写操作使用 form action。其余 V1 功能必须按照 [V2 SSR 前端迁移规则](../docs/v2/frontend-ssr.md)逐模块迁移后才能声明支持。

## 设计边界

- Go 后端是唯一认证、权限、限流、审计和业务状态边界。
- 认证 Cookie 只由服务端转发，浏览器脚本不持有 Bearer Token。
- 当前用户与权限数据 no-store；没有跨用户共享缓存。
- API JSON 响应有 8 MiB 读取上限，代理请求/响应使用 32 MiB 流式上限。
- 所有移动端页面必须以 Firefox 为基准并拥有自己的有限滚动区域。
