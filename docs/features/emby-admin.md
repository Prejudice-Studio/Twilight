# Emby 管理

本文覆盖管理端的 Emby 运维面：账号管理、设备/IP 审查、活动日志同步、线路下发与探测，以及播放数据的可见性边界。用户侧的播放日榜/周榜见 [播放排行榜](./playback-rank.md)。

## 页面结构

管理页 `/admin/emby` 分为**账号**、**设备/IP 审查**和 **ActivityLog** 三个页签。

- 账号、设备审查和活动日志按当前页签读取，不做浏览器轮询、SSE 或自动刷新；设备/IP 审查与 ActivityLog 同步都是**手动触发**。
- 账号分页、设备/IP 聚合、Twilight 自身设备排除、后端本地连通性检测、危险操作确认与管理审计**全部由 Go 后端负责**，前端只负责展示与提交。
- 窄屏下账号表切换为卡片视图（`AdminTable`），不依赖横向拖动；长账号表、孤立账号表、设备/IP 明细和 ActivityLog 使用独立的 `dvh` 滚动区域。

## 相关配置

配置键位于 `config.toml` 的 `[Emby]` 段，均可在管理端「系统设置 → 配置」里可视化编辑（改完热重载，不需要重启）。

| 键 | 说明 |
| -- | ---- |
| `emby_url` | 后端访问 Emby/Jellyfin 的地址 |
| `emby_token` | Emby API Key（secret，UI 只回传遮蔽哨兵，不回传明文） |
| `emby_username` / `emby_password` | 备用鉴权凭据（secret） |
| `emby_url_list` | 普通线路，格式 `名称 : URL` |
| `emby_url_list_for_whitelist` | 白名单线路，仅管理员与白名单用户可见 |
| `emby_whitelist_url` | 白名单单线路地址；留空则只使用上面的白名单线路列表 |
| `emby_public_url` | 浏览器侧访问 Emby 的地址；留空则使用后端地址 |
| `emby_stats_enabled` | 首页仪表盘 Emby 卡片是否显示电影/剧集/集数统计 |
| `play_rank_enabled` | 播放日榜/周榜总开关，见 [播放排行榜](./playback-rank.md) |
| `play_rank_user_visible` | 是否允许普通用户查看脱敏后的排行榜 |

## 接口清单

完整字段语义见 [API 路由索引](../reference/api-index.md)。以下均为 `/api/v2` 路由。

### 连接状态与统计

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/emby/status` | User | 当前用户的 Emby 健康快照（含本账号绑定态）；开启库统计时附带三项计数 |
| GET | `/api/v2/emby/stats` | User | 库统计；关闭或未配置时字段为零值 |
| GET | `/api/v2/system/emby-stats` | User | 同上，但未启用/未配置时**省略**计数字段而非返回零值 |
| GET | `/api/v2/system/health/emby` | Admin | 独立连通性检测，失败只返回固定文案，不泄露上游诊断 |

### 会话与在线人数

会话类接口有一条硬边界：**只给数字的接口不暴露任何身份信息**。

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/emby/online` | User | `online` + `current_online`，`users` 恒为空数组 |
| GET | `/api/v2/emby/viewer-count` | User | 只返回 `{viewers}` |
| GET | `/api/v2/system/emby-viewers` | User | 同上（前端仪表盘实际使用的入口） |
| GET | `/api/v2/emby/sessions/count` | User | `{active, total}` 计数 |
| GET | `/api/v2/emby/sessions` | User | **只返回本人**会话，不含 `user_name` |
| GET | `/api/v2/me/sessions` | User | 同上（按 `UserId == 本人 EmbyID` 过滤） |
| GET | `/api/v2/admin/emby/sessions` | Admin | 全部会话，含用户名、IP 与本地账号映射 |
| GET | `/api/v2/admin/emby/now-playing` | Admin | 「谁在看什么」明细（用户名 + 片名 + 进度） |

> `/admin/emby/now-playing` 是**唯一**能看到具体谁在看什么的接口，级别是 Admin，且**前端已不再调用**，只留给运维工具。产品口径是仪表盘只显示在线总数：曾经存在一个 `AuthPublic`（完全匿名）的 `/api/v2/emby/now-playing`，会把观看者身份泄露给未登录访客，已删除，不要再恢复。

### 账号操作

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/api/v2/admin/emby/users/{embyId}/enable` | Admin | 启用远端 Emby 账号；已关联本地用户时须通过 `embyShouldEnableUser`（Web 禁用/过期返回 409，禁止绕过有效期） |
| POST | `/api/v2/admin/emby/users/{embyId}/disable` | Admin | 停用远端 Emby 账号（不受有效期约束） |
| POST | `/api/v2/admin/emby/users/{embyId}/kick` | Admin | 踢出全部在线会话并清掉离线访问设备 |
| POST | `/api/v2/admin/users/{uid}/bind-emby` | Admin | 把远端账号绑给指定用户，`force` 可从原用户抢绑 |
| DELETE | `/api/v2/admin/users/{uid}/emby` | Admin | 解绑用户 Emby |
| POST | `/api/v2/admin/users/{uid}/kick` | Admin | 踢出该用户的所有 Emby 会话 |

启用/禁用会同步本地 `EmbyDisabled` 镜像并写审计（`emby_user_enable` / `emby_user_disable` / `emby_user_kick`）。受保护账号（管理员、配置管理员、白名单）与远端 Emby 管理员一律拒绝修改。

### 线路下发与探测

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/system/emby-urls` | User | **推荐入口**，`{lines:[{name,url}]}` |
| GET | `/api/v2/admin/emby/urls` | Admin | 角色化下发，对 Admin/Whitelist 追加 `whitelist_lines` |
| POST | `/api/v2/system/emby-urls/probe` | User | 后端代发 HEAD 测速 |

`/api/v1/emby/urls` 已弃用（固定返回 410），`/api/v2/emby/urls` 与推荐入口共用 handler，新增调用请统一走 `/system/emby-urls`。

探测接口是**后端代发**，目的是规避浏览器跨域与 HTTPS/HTTP 混合内容，但它同时也是一条 SSRF 面：

- 目标必须命中已配置的 `emby_url` / `emby_public_url` / `emby_url_list` 之一（白名单专属线路还需 Admin 或 Whitelist 角色，否则 403）；
- 再经 `validateOutboundBaseURL` 拦截 link-local、云元数据地址等内网目标；
- 实际探测 `<目标>/web/favicon.ico`，5 秒超时，每用户每分钟限 20 次。

### 设备 / IP 审查与活动日志

设备与登录历史接口见 [API 路由索引](../reference/api-index.md#安全与设备)：

- `GET /api/v2/admin/security/users/{uid}/devices`、`POST .../devices/{device_id}/block`
- `GET /api/v2/admin/security/users/{uid}/login-history`、`GET /api/v2/admin/security/suspicious`
- `GET|POST|DELETE /api/v2/admin/security/ip-blacklist`

Emby 活动日志由后端同步写入 `twilight_playback_records`（配对播放开始/停止事件、幂等写入），页面上的「同步」是手动触发，不会自动轮询。同步写入后会立即失效排行榜缓存。

## 播放数据的可见性约定

| 数据 | 谁可以看 | 说明 |
| ---- | -------- | ---- |
| 在线人数 | 所有登录用户 | 只给总数，不暴露身份 |
| 「谁在看什么」 | 仅 Admin | 前端不展示，只留给运维工具 |
| 个人播放记录 | 本人 / Admin | 他人一律 403 |
| 全站播放汇总 | Admin | 只有次数、时长、去重人数 |
| 播放排行榜 | 登录用户（脱敏）/ Admin（完整） | 见 [播放排行榜](./playback-rank.md) |

无账号访客没有任何播放数据入口。
