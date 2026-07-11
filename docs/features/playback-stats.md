# 播放统计

Twilight 的播放统计独立于 Bangumi Webhook。它参考 `ichinosekotomi11/Twilight` 的实现逻辑：使用 Emby `/System/ActivityLog/Entries` 活动日志采集 `playback.start` / `playback.stop` 事件，按用户与媒体条目配对计算时长，并幂等写入 `store.PlaybackRecords`。统计接口聚合本地播放记录，避免每次查询都临时扫描 Emby 活动日志。

## 开关与采集

- 配置项：`[Emby] emby_playback_stats_enabled = true`。
- 采集函数：`fetchAndStoreEmbyActivityLogsSince`，按 500 条分页读取，直到覆盖请求时间窗口或达到 20000 条上限。
- 活动日志存储：`maxEmbyActivityLogs = 20000`，按 Emby 日志 ID 去重，超过后裁剪最旧记录。
- 播放记录存储：同步活动日志后会配对 `playback.start` / `playback.stop`，按 `(uid, item_id, played_at)` 幂等写入 `store.PlaybackRecords`。
- 自动刷新：`GET /admin/emby/activity-logs` 与播放统计接口默认会触发 2 分钟节流的轻量自动刷新；`?refresh=1` 会强制立即拉取。
- 调度任务：`sync_emby_activity_logs` 仍可按后台调度周期主动同步，适合生产环境保持统计数据持续更新。支持 `since_hours` 参数控制同步窗口（默认 24h，最大 720h）。

## 数据库持久化

- PostgreSQL 用户自动创建 `twilight_playback_records` 表，字段 `uid, item_id, title, series_name, media_type, index_number, duration, played_at`。
- 唯一约束 `(uid, item_id, played_at)` 配合 `INSERT ... ON CONFLICT DO NOTHING` 确保幂等写入。
- 表上建有 `uid`、`played_at DESC`、`item_id` 索引，支持按用户、时间、条目快速查询和聚合。
- 查询优先走数据库（`PlaybackRecords` / `PlaybackRecordSummary`），JSON 文件模式回退到状态文档的内存记录。
- 备份/迁移时间戳旧数据通过 `DeletePlaybackRecordsBefore(cutoff)` 清理过期记录。

## 接口

| 路径 | 权限 | 说明 |
| ---- | ---- | ---- |
| `GET /api/v1/emby/playback-stats` | User | 当前用户统计 |
| `GET /api/v1/admin/emby/playback-stats` | Admin | 全站、自己或指定用户统计 |
| `GET /api/v1/admin/emby/playback-stats/:uid` | Admin | 指定 UID 统计 |
| `GET /api/v1/admin/emby/activity-logs` | Admin | 原始 Emby 活动日志 |
| `GET /api/v1/emby/now-playing` | User | 当前正在播放的会话列表 |
| `GET /api/v1/batch/export/playback` | Admin | 导出 CSV（支持 from/to 日期范围） |

常用参数：

- `scope=self|global|user`：查看范围；普通用户只能 `self`。
- `uid=<uid>`：`scope=user` 时指定本地用户 UID。
- `period=today|week|month|custom`：统计周期。
- `days=7|30|90|365`：最近 N 天。
- `from=YYYY-MM-DD&to=YYYY-MM-DD`：自定义日期范围（查询和导出均支持）。
- `group_by=day|week|month`：趋势聚合粒度。
- `media_type=all|movie|series|other`：媒体类型筛选。
- `query=<text>`：按标题或剧集名筛选。
- `min_duration=<seconds>`：过滤短播放。
- `limit=10|20|50|100`：排行榜条数。
- `sort=plays|duration|name`：排行榜排序。
- `refresh=1`：查询前强制刷新 Emby 活动日志并写入播放记录。

## 在线人数与正在观看

Emby `/Sessions` 会包含空闲或保留会话。Twilight 的在线人数只统计存在 `NowPlayingItem` 的会话，即"当前正在播放"的人数。仪表盘将在线人数、电影数、剧集数和集数放在独立的媒体概览卡片中，不再混入 Emby 服务器状态卡。

`GET /api/v1/emby/now-playing` 返回当前正在播放的会话详情，包括条目名称、剧集名、封面图 URL、用户名、播放进度。仪表盘"正在观看"卡片每 30 秒轮询一次，最多展示 8 个当前播放条目。

## 前端行为

- `/stats/playback` 默认自动加载统计，并在页面可见时按用户选择的 30 秒 / 60 秒 / 5 分钟间隔轮询。
- 手动刷新按钮会强制拉取最新 Emby 活动日志，后端同步后先写入 `PlaybackRecords`，再重新聚合统计。
- 管理员可切换 scope=global/user/self 查看不同范围数据，支持日期范围、媒体类型、最低时长等筛选。
- 管理员可使用"导出 CSV"按钮下载符合当前日期范围的播放记录，包含 UID、用户名、标题、剧集名、媒体类型、时长等字段。
- `/admin/emby` 的活动日志面板默认每 30 秒刷新一次，可关闭。
- `/admin/emby` 的设备 / IP 审查页默认每 30 秒刷新在线状态；手动刷新会带 `refresh=1` 跳过后端短缓存。

## 限制

统计时长由同一用户、同一媒体条目的开始与停止事件配对计算，单次播放最多计入 12 小时。查询窗口会额外向前读取 12 小时，以覆盖"窗口开始前播放、窗口内停止"的场景；无法配对的孤立事件不会伪造时长。所有聚合结果最终来自 `PlaybackRecords`，因此 JSON 与 PostgreSQL 存储后端都会随状态文档一起备份、迁移和恢复。
