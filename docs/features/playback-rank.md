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
- 聚合在 `store.PlaybackRank(PlaybackRankOptions{Since, Limit, GroupBy, SortBy})`：两条 `GROUP BY` 查询——媒体榜按 `item_id`（或按剧名，见下）、用户榜按 `uid`，都带 `played_at >= $1` 过滤。PostgreSQL 不可用时退化为扫描内存副本（`state.PlaybackRecords`，上限 5000 条）做同样聚合。用结构体传参而不是逐个传参，是因为四个参数里两个字符串两个整数，按位置传很容易错位——`since` 与 `limit` 换个位置不报编译错，只会静默返回错的数据。
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

## 媒体榜的两种聚合方式

| `group_by` | 一行代表 | 用途 |
| ---------- | -------- | ---- |
| `item`（默认） | 一集电视剧 / 一部电影 | 哪一集最受欢迎 |
| `series` | 一整部剧（所有集合并） | 哪部剧最受欢迎 |

`series` 模式按 `COALESCE(NULLIF(series_name,''), NULLIF(title,''), item_id)` 分组——表里没有存 SeriesId，只能用剧名归并，**同名剧会被并成一行**，这是已知的近似。返回行的 `episodes` 是这一行覆盖了多少个不同 item：item 模式下恒为 1，series 模式下是这部剧被看过的集数。「播放 30 次」到底是 30 人各看一集、还是一个人刷同一集 30 遍，光看 `plays` 分不出来，必须靠它。

未知取值一律退回 `item`：宁可给明细，也不能让未经白名单校验的字符串进到 `GROUP BY`。

### 排序口径：次数 vs 时长

`sort_by` 取 `plays`（默认）或 `duration`，**同时作用于媒体榜与用户榜**。两个指标回答的不是同一个问题：

- `plays`：被点开过多少次，偏向热度 / 流行度；
- `duration`：一共占用了多少时间，偏向实际投入。

一部 20 分钟的番剧刷 30 遍（10 小时）和一部三小时电影看 1 次（3 小时），按次数排是番剧第一、按时长排是电影第一——**两个榜单的名次是反的**，所以必须由调用方显式选，不能替它默认。

> 历史问题：早期媒体榜写死按次数排、用户榜写死按时长排，两个榜的「第一名」根本不是同一个口径，也没法切换。现在统一由 `sort_by` 决定。

排序表达式只能从 `playbackRankOrderBy` 的两个白名单常量里拼，**绝不能把外部字符串流进 `ORDER BY`**；次排序键固定用另一个指标（同分时顺序稳定，否则每次刷新名次都在跳），最后跟唯一键（`item_id` / 列序号 / `uid`）兜底做到完全确定。内存兜底路径的比较器 `playbackRankLess` 与之一一对应——PG 降级时名次不能变。

### 集数标识

季号与集号**不在播放记录表里**，而是每次构建榜单时由 `playRankEpisodes` 向 Emby 批量取 `ParentIndexNumber` / `IndexNumber` 补上，下发 `season_number` / `episode_number` 两个数字。这么做是为了让历史记录不用迁移就能显示集数；Emby 不可用时拿不到编号，只是少了这个徽标，榜单本身照常返回。`series` 模式下整部剧没有「第几集」可言，不会下发这两个字段。

后端只给数字、**不拼显示文案**：拼好的字符串没法翻译，也没法让不同语言按自己的习惯表达。显示规则在前端 `play-rank-media-label.tsx`：

| 拿到的编号 | 显示 | 文案 key |
| --- | --- | --- |
| 季号 + 集号 | `S1E8` | `playRank.seasonEpisode` |
| 只有集号 | 第 8 集 / Episode 8 | `playRank.episodeOnly` |
| 都没有，但这一行是剧集 | 未知（弱化描边徽标） | `playRank.episodeUnknown` |
| 电影 / 音乐 | 不显示徽标 | — |

「未知」占位不能省：只有剧名没有集号的一行看起来会像在统计整部剧，实际是一次单集播放。**更不能用 `S1E8` 之类的默认值顶上**——那是拿编造的编号冒充真实数据，误导性比留白更强。判断是否「本该有集数」的依据是媒体类型为剧集或带剧名；电影、音乐没有集数概念，不给徽标。真实编号用实心徽标、「未知」用描边弱化，两者不能长得一样，否则看的人会把「没查到」当成「查到了就是这个」。

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
| `group_by` | `item`（默认，逐集/逐部）/ `series`（按整部剧聚合） |
| `sort_by` | `plays`（默认，按播放次数）/ `duration`（按累计时长） |
| `refresh` | 传 `1` 时绕过缓存 |

响应含 `range`、`since`（窗口起点）、`group_by`、`sort_by`、`updated_at`、总览 `summary`、媒体榜 `media`、用户榜 `users`，以及整库覆盖面 `recorded`。媒体榜每行带 `episodes`（覆盖的条目数），剧集另有可选的 `season_number` / `episode_number` 两个数字：

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

- 60 秒缓存，键为 `range|since|limit|group_by|sort_by|是否含身份`。**身份参与键计算**，保证管理员的完整用户名不会串到普通响应里；`sort_by` 同样参与，否则两种排序会互相覆盖。
- `refresh=1` 绕过缓存。
- ActivityLog 同步成功写入记录后会调用 `invalidatePlayRankCache()`，所以管理端点「同步」后立刻能看到新数据。

## 页面

| 路径 | 说明 |
| ---- | ---- |
| `/playrank` | 用户页，脱敏榜单，日/周/月/总榜切换 + 按单集/按整部剧切换 + 按次数/按时长切换 |
| `/admin/playrank` | 管理页，含 `uid`、同步窗口选择与「同步活动日志」按钮（调 `adminGetEmbyActivityLogs`），并用徽标显示当前时长口径是净时长还是墙上时长 |

两个页面的媒体榜共用 `webui/src/components/play-rank-media-label.tsx` 渲染标题区（剧名 + 集数徽标或「未知」占位 + 单集标题），改样式改一处即可。集数的文案走 `playRank.seasonEpisode` / `playRank.episodeOnly` / `playRank.episodeUnknown` 三条 i18n key，改显示方式不用动后端。

两个榜都把当前排序指标那一列用正常字重显示、另一列压暗，表头同步高亮：两列数字并排时，不标出来就看不出这一屏究竟是照哪一列排的。

> 若要真正对无账号访客开放，页面必须放在 `(main)` 路由组之外——`(main)/layout.tsx` 在未登录时会跳 `/login`。可参考既有公开页 `webui/src/app/wiki/page.tsx`。
