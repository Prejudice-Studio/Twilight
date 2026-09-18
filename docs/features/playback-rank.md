# 播放排行榜

按 Emby 播放记录聚合出的媒体热度榜与用户观看榜，仿 Sakura EmbyBoss 的榜单形态。窗口有四类：日榜、周榜、月榜，以及覆盖系统已记录全部数据的总榜。

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
- 窗口在**后端**计算（`playRankWindow`），不要挪到前端，一律按服务器本地时区：

| `range` | 起点 |
| ------- | ---- |
| `day`（默认） | 今天 00:00 |
| `week` | 本周一 00:00 |
| `month` | 本月 1 日 00:00 |
| `all` | `0`，即不设下限——覆盖系统已记录的全部播放数据 |

- `days=N` 是「过去 N 天」的滑动窗口，显式给出时优先于 `range`，上限 730 天。它补齐了日历窗口够不到的场景：想看「最近 30 天」不必等到月末。传入后响应里的 `range` 变成 `30d` 这样的形式，因为它同时是缓存键的一部分。

### 为什么要有总榜

系统里沉淀了多少播放数据，和「今天有多少」是两件事。管理员后台默认只同步最近 24 小时的活动日志，日榜/周榜经常是空的，很容易被误读成「系统没记录」。所以：

- 响应里带 `recorded`（`total` / `earliest` / `latest`），是**整库覆盖面**，不随窗口变化；
- 管理端响应另带 `playback_reporting`（`enabled` / `available`），管理员据此确认当前时长口径；
- 窗口内没有数据但 `recorded.total > 0` 时，前端提示切换到总榜，而不是笼统显示「暂无数据」；
- 管理页的「同步活动日志」可选窗口（24 小时 / 3 天 / 7 天 / 30 天）。想让总榜有历史可看，得先用长窗口把日志拉回来——活动日志只能按「过去 N 小时」回拉。

## 两个数据源，两种时长口径

| 数据源 | 依赖 | 时长怎么来的 | 暂停算不算 |
| ------ | ---- | ------------ | ---------- |
| 活动日志（内置） | 无 | 播放开始/停止事件配对，停止时刻减开始时刻 | **算**，挂机也计时 |
| Playback Reporting 插件 | Emby 插件目录里的 **Playback Reporting**（它的 API 路径前缀是 `/emby/user_usage_stats/...`，常被误叫成 UserUsageStats） | `PlayDuration - PauseDuration` | 不算，是真看进去的净时长 |

插件可用时优先用插件（开关 `[Emby] playback_reporting_enabled`，默认开；真正的门槛是探测——没装插件时 Emby 会拒绝那个端点，自动回退活动日志，不用改配置）。

**修正而不是追加**：活动日志早就为同一场播放写过一行，插件若直接再插一行，播放次数就会翻倍。所以 `store.ApplyPlaybackReportingRecords` 会先找同一 `(uid, item_id)`、时间最接近且尚未被插件修正过的那一行，命中就把它的 `duration` 换成净时长并把 `source` 改成 `playback_reporting`；只有活动日志压根没记到那一场才插入新行。

两个源的"这一场播放发生在什么时刻"记法不同（插件可能记开始、活动日志记录停止，且插件的 SQLite 时间戳可能是 UTC），容差取 12 小时并在候选里挑时间最接近的一条。代价是同一集反复重看时可能配错——配错只是两场时长对调，不会凭空多出一次播放；相比配不上导致的"一次播放记两遍"，这个取舍划算。

`twilight_playback_records.source` 就是为这件事加的：没有它就分不清一行时长是墙上时钟差还是净时长。存量行统一标记为 `activity_log`。

## 接口

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/emby/play-rank` | User | 登录用户榜单，用户名脱敏，无 `uid` |
| GET | `/api/v2/admin/emby/play-rank` | Admin | 完整榜单，含 `uid` 与原始用户名 |

查询参数：

| 参数 | 说明 |
| ---- | ---- |
| `range` | `day`（默认）/ `week` / `month` / `all` |
| `days` | 「过去 N 天」滑动窗口，优先于 `range`，上限 730 |
| `limit` | 每榜条数，默认 20，上限 100 |
| `refresh` | 传 `1` 时绕过缓存 |

响应含 `range`、`since`（窗口起点）、`updated_at`、总览 `summary`、媒体榜 `media`、用户榜 `users`，以及整库覆盖面 `recorded`：

```jsonc
{
  "range": "all",
  "since": 0,
  "summary": { "plays": 12, "duration": 3600, "viewers": 3, "items": 5 },
  "recorded": { "total": 4821, "earliest": 1735689600, "latest": 1773000000 }
}
```

`summary` 是当前窗口的统计，`recorded` 是整个库的记录规模——两者刻意分开，别混用。

## 缓存

- 60 秒缓存，键为 `range|since|limit|是否含身份`。**身份参与键计算**，保证管理员的完整用户名不会串到普通响应里。
- `refresh=1` 绕过缓存。
- ActivityLog 同步成功写入记录后会调用 `invalidatePlayRankCache()`，所以管理端点「同步」后立刻能看到新数据。

## 页面

| 路径 | 说明 |
| ---- | ---- |
| `/playrank` | 用户页，脱敏榜单，日/周/月/总榜切换 |
| `/admin/playrank` | 管理页，含 `uid`、同步窗口选择与「同步活动日志」按钮（调 `adminGetEmbyActivityLogs`），并用徽标显示当前时长口径是净时长还是墙上时长 |

> 若要真正对无账号访客开放，页面必须放在 `(main)` 路由组之外——`(main)/layout.tsx` 在未登录时会跳 `/login`。可参考既有公开页 `webui/src/app/wiki/page.tsx`。
