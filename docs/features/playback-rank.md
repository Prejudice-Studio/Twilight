# 播放排行榜（日榜 / 周榜）

按 Emby 播放记录聚合出的媒体热度榜与用户观看榜，仿 Sakura EmbyBoss 的日榜/周榜形态。

## 可见性与开关

| 角色 | 能看到什么 |
| ---- | ---------- |
| 管理员 | 完整榜单，含 `uid` 与原始用户名 |
| 登录用户 | 脱敏榜单（用户名保留首尾字符、中间打码），**不含 `uid`** |
| 无账号访客 | **没有任何入口**，接口在鉴权层直接拒绝 |

两个开关都在 `config.toml` 的 `[Emby]` 段：

| 键 | 默认 | 说明 |
| -- | ---- | ---- |
| `play_rank_enabled` | `true` | 总开关。关闭后除管理员后台外全部拒绝 |
| `play_rank_user_visible` | `true` | 普通用户是否可见。关闭后只有管理员有入口 |

> **没有任何匿名入口。** `/api/v2/emby/play-rank` 的路由级别是 `AuthUser`，未登录请求根本到不了 handler（不是靠 handler 内部判断）。曾经存在一个 `play_rank_anonymous` 开关和一条 `AuthPublic` 路由，都已删除——在这个框架里 `AuthPublic` 等于完全匿名放行，不要再把任何带隐私的路由注册成 `AuthPublic`。

前端入口由 `/api/v2/system/info` 下发的 `features` 控制：

- `play_rank`：总开关，关闭时侧边栏入口对所有角色隐藏；
- `play_rank_user`：普通用户可见开关，关闭时**管理员入口仍然保留**（侧边栏过滤带 `isAdmin` 判断）。

## 数据源与窗口

- 数据来源 `twilight_playback_records` 表，由 Emby ActivityLog 同步写入（配对播放开始/停止事件，幂等）。**`duration` 字段单位是秒**。
- 聚合在 `store.PlaybackRank(since, limit)`：两条 `GROUP BY` 查询——媒体榜按 `item_id`、用户榜按 `uid`，都带 `played_at >= $1` 过滤。PostgreSQL 不可用时退化为扫描内存副本（`state.PlaybackRecords`，上限 5000 条）做同样聚合。
- 窗口在**后端**计算（`playRankWindow`），不要挪到前端：日榜 = 今天 00:00，周榜 = 本周一 00:00，均按服务器本地时区。

## 接口

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/emby/play-rank` | User | 登录用户榜单，用户名脱敏，无 `uid` |
| GET | `/api/v2/admin/emby/play-rank` | Admin | 完整榜单，含 `uid` 与原始用户名 |

查询参数：

| 参数 | 说明 |
| ---- | ---- |
| `range` | `day`（默认）或 `week` |
| `limit` | 每榜条数，默认 20，上限 100 |
| `refresh` | 传 `1` 时绕过缓存 |

响应含 `range`、`since`（窗口起点）、`updated_at`、总览 `summary`、媒体榜 `media`、用户榜 `users`。

## 缓存

- 60 秒缓存，键为 `range|limit|是否含身份`。**身份参与键计算**，保证管理员的完整用户名不会串到普通响应里。
- `refresh=1` 绕过缓存。
- ActivityLog 同步成功写入记录后会调用 `invalidatePlayRankCache()`，所以管理端点「同步」后立刻能看到新数据。

## 页面

| 路径 | 说明 |
| ---- | ---- |
| `/playrank` | 用户页，脱敏榜单 |
| `/admin/playrank` | 管理页，含 `uid` 与「同步活动日志」按钮（调 `adminGetEmbyActivityLogs`） |

> 若要真正对无账号访客开放，页面必须放在 `(main)` 路由组之外——`(main)/layout.tsx` 在未登录时会跳 `/login`。可参考既有公开页 `webui/src/app/wiki/page.tsx`。
