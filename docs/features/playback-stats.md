# 播放统计

播放统计基于 Emby ActivityLog，独立于 Bangumi Webhook 的播放记录。

## 开关与采集

- 配置键：`[Emby] emby_playback_stats_enabled = true`
- 采集函数：`fetchAndStoreEmbyActivityLogs`
- 单次最多拉取 500 条带用户信息的 Emby 活动日志。
- 存储上限：`maxEmbyActivityLogs = 20000`
- 日志按 Emby log ID 去重，超过上限后裁剪最旧记录。

## 接口

| 路径 | 权限 | 说明 |
| ---- | ---- | ---- |
| `GET /api/v1/emby/playback-stats` | User | 当前用户统计 |
| `GET /api/v1/admin/emby/playback-stats` | Admin | 全站、自己或指定用户统计 |
| `GET /api/v1/admin/emby/playback-stats/:uid` | Admin | 指定 UID 统计 |
| `GET /api/v1/admin/emby/activity-logs` | Admin | 原始 Emby 活动日志 |

## 查询参数

- `scope=self|global|user`：统计范围，普通用户只能查看自己。
- `uid=<uid>`：指定本地用户。
- `days=7|30|90|365`：滚动周期。
- `today=1`：仅当天，优先级高于 `days`。
- `limit=10|20|50|100`：榜单条数。
- `sort=plays|name`：节目榜排序。
- `refresh=1`：统计前强制刷新 Emby 活动日志。

## 限制

Emby ActivityLog 能稳定提供播放事件、用户、节目名和时间，但不稳定提供完整播放时长。因此 `total_duration` 目前是保留字段，默认返回 `0`。
