# 播放统计

Twilight 的播放统计独立于 Bangumi Webhook。它使用 Emby `/System/ActivityLog/Entries` 活动日志采集播放事件，写入 `store.EmbyActivityLogs`，再按范围和时间窗口聚合。

## 开关与采集

- 配置项：`[Emby] emby_playback_stats_enabled = true`。
- 采集函数：`fetchAndStoreEmbyActivityLogs`，单次从 Emby 拉取最近 500 条带用户信息的活动日志。
- 存储上限：`maxEmbyActivityLogs = 20000`，按 Emby 日志 ID 去重，超过后裁剪最旧记录。
- 自动刷新：`GET /admin/emby/activity-logs` 与播放统计接口默认会触发 2 分钟节流的轻量自动刷新；`?refresh=1` 会强制立即拉取。
- 调度任务：`sync_emby_activity_logs` 仍可按后台调度周期主动同步，适合生产环境保持统计数据持续更新。

## 接口

| 路径 | 权限 | 说明 |
| ---- | ---- | ---- |
| `GET /api/v1/emby/playback-stats` | User | 当前用户统计 |
| `GET /api/v1/admin/emby/playback-stats` | Admin | 全站、自己或指定用户统计 |
| `GET /api/v1/admin/emby/playback-stats/:uid` | Admin | 指定 UID 统计 |
| `GET /api/v1/admin/emby/activity-logs` | Admin | 原始 Emby 活动日志 |

常用参数：

- `scope=self|global|user`：查看范围；普通用户只能 `self`。
- `uid=<uid>`：`scope=user` 时指定本地用户 UID。
- `days=7|30|90|365`：最近 N 天。
- `today=1`：只看当天，优先级高于 `days`。
- `limit=10|20|50|100`：排行榜条数。
- `sort=plays|name`：节目榜排序。
- `refresh=1`：查询前强制刷新 Emby 活动日志。

## 前端行为

- `/stats/playback` 默认自动加载统计，并在页面可见时按用户选择的 30 秒 / 60 秒 / 5 分钟间隔轮询。
- 手动刷新按钮会强制拉取最新 Emby 活动日志，再重新聚合统计。
- `/admin/emby` 的活动日志面板默认每 30 秒刷新一次，可关闭。
- `/admin/emby` 的设备 / IP 审查页默认每 30 秒刷新在线状态；手动刷新会带 `refresh=1` 跳过后端短缓存。

## 限制

Emby ActivityLog 能稳定提供播放事件、用户、时间与条目名，但不保证提供完整播放时长。因此当前统计中的总时长为保留字段，默认返回 `0`；如需精确播放时长，应接入会话停止事件或 Emby playback reporting 数据源。
