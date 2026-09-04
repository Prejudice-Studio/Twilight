# Twilight V2 SSR 前端

本文记录 V2 前端的实际基础实现。V2 不在 V1 的 Next.js/React 页面上继续堆叠客户端状态，而是在 `webui-v2/` 使用 SvelteKit SSR 和 `@sveltejs/adapter-node`，逐模块迁移 V1 能力。迁移完成前，V1 `webui/` 仍是生产回退入口。

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
      logout/                # 登出 server endpoint
      (app)/dashboard/        # 首个已迁移的仪表盘摘要
      api/[...path]/          # 仅 v1/v2 的同源 API 代理
```

## 请求与会话

`src/lib/server/api.ts` 是 V2 的服务端传输边界：

1. 后端地址来自 `BACKEND_URL`，仅允许 `http` / `https`，拒绝 URL userinfo，并移除 query/fragment。
2. 服务器页面请求只转发入站 Cookie；显式丢弃入站 `Authorization` 和 `X-API-Key`，避免把浏览器提交的 Bearer/API Key 凭据混入 SSR 会话链路。
3. JSON 响应先检查 `Content-Length`，再通过有界流读取，超过 8 MiB 时取消读取并返回空结果；解析失败不会把上游正文回显到页面。API 代理对请求和响应使用 32 MiB 流式上限，避免为上传/下载一次性复制整个正文。
4. 登录 action 只接受配置的会话 Cookie（默认 `twilight_session`），解析上游 `Set-Cookie` 时忽略后端 Domain，只向当前 V2 站点写 host-only、HttpOnly Cookie，避免跨域扩大 Cookie 作用范围。
5. SSR 读取默认为 `no-store`。当前用户身份、权限和会话状态不能进入共享浏览器缓存。

`src/routes/api/[...path]/+server.ts` 只允许代理 `/api/v1/*` 与 `/api/v2/*`，通过流式上限限制请求体，移除 hop-by-hop、Origin、Referer、Authorization、API Key 和 Host 等头。Go 后端仍是唯一认证、权限、限流和业务状态边界。SvelteKit form action 默认启用同源 Origin 校验。

## 迁移规则

一个功能完成迁移必须同时具备：

- V1 页面/API 到 V2 route、load、action 和 DTO 的映射。
- 服务端权限复核、状态变更审计、重复提交/并发处理和失败回滚边界。
- 列表摘要与详情分离，分页/游标和附件按需读取。
- 手机/平板/窄 Firefox 视口验证，长表格、对话框和聊天区域拥有自己的 `dvh` 滚动上下文。
- V1 配置/数据兼容、回退入口和文档更新。

未完成迁移的 V1 模块不能被 V2 壳层伪装成已支持，也不能通过前端隐藏按钮替代后端 feature gate。
