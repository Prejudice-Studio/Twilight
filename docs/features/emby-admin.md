# Emby 管理前端

管理员 Emby 页面默认入口为 `webui-v2/src/routes/(app)/admin/emby`，使用 SvelteKit SSR、服务端 `load` 和 form action。旧 `webui/` 页面仅保留作紧急回滚，不与 V2 共享浏览器状态。

页面分为账号、设备/IP 审查和 ActivityLog 三个 URL 页签。账号、设备审查和活动日志按当前页签读取；设备/IP 审查和 ActivityLog 同步均为手动操作，不恢复浏览器轮询、SSE 或自动刷新。Emby 活动日志继续由后端同步并保存到数据库，页面重构不会将其误删为播放统计。

V2 页面标题、账号/设备/活动主要容器使用共享 `PageHeader.svelte` / `Panel.svelte`。账号分页、设备/IP 聚合、Twilight 自身设备排除、后端本地连通性检测、危险操作确认和管理审计仍由 Go 后端与 SSR action 负责；共享组件只提供结构、滚动和窄 Firefox 视口布局。

长账号表、孤立账号表、设备/IP 明细和 ActivityLog 使用独立的 `dvh` 滚动区域。手机和平板下页签、筛选、危险操作和工具表单自动换行，避免操作按钮被挤出视口。
