# V2 观看事件模型

## 当前进度

`internal/playback` 提供与 HTTP、Emby 和 PostgreSQL 无关的可信事件状态机，`internal/store/trusted_playback.go` 已将它接入 PostgreSQL 的事件、播放段和每日桶表。当前仍不启用旧版统计页面，也不改变现有 Emby ActivityLog 保留和播放记录兼容逻辑；V2 观看统计接口需要在完成鉴权、外部事件映射和管理员/用户 DTO 后再开放。

## 事件

每个事件至少包含：

- `event_id`：客户端或服务端生成的稳定幂等 ID；重复投递不能重复累计。
- `uid`、`playback_id`、`device_id`、`item_id`：播放归属和设备隔离字段。
- `type`：`started`、`playing`、`paused`、`resumed`、`stopped`、`completed` 或 `expired`。
- `at`：客户端事件时间；`received_at`：服务端接收时间。
- `sequence`：同一播放流的单调序号，旧序号不会倒退状态。
- `time_zone`：事件发生时使用的 IANA 时区；缺省使用 UTC。

服务端只把事件时间作为播放段边界提示。客户端时间最多允许比接收时间领先 5 分钟，播放段累计最多 12 小时；缺失停止事件时由 `expired` 收敛，不能无限增长。

## 状态规则

- `started`、`playing`、`resumed` 进入播放中状态。
- `paused` 只结算此前仍在播放的区间，暂停期间不累计。
- `stopped`、`completed`、`expired` 结算最后一段并关闭播放段。
- 同一播放的乱序或重复序列不改变已经确认的状态。
- 播放跨本地日期时按照事件时区切分日桶，DST 由 Go 的 IANA 时区规则处理。

## PostgreSQL 持久化边界

数据库适配器在同一事务中完成：先按 `(uid, event_id)` 幂等插入原始事件，再锁定播放段，调用领域状态机，更新播放段和每日桶。重复事件返回原处理结果；相同事件 ID 携带不同 payload 会被拒绝；数据库失败不会只更新内存。每日桶仍是可重建派生数据，后续管理端需要提供校验摘要。

用户删除时，`twilight_state` 与三张可信观看表在同一个 PostgreSQL 事务中提交，避免状态已删除但观看数据残留。数据库迁移包会在同一一致性快照中导出并在 Serializable 事务中恢复 `trusted-playback-events.json`、`trusted-playback-segments.json` 和 `trusted-playback-daily.json`；旧迁移包缺少这三个可选文件时按空集合兼容。

外部 Emby ActivityLog 仍是审计/历史来源，不能直接当成浏览器可信事件；其配对结果只能经过明确的来源标记和同一状态机处理。普通用户只能读取自己的摘要，管理员统计必须单独鉴权，仪表盘在线人数也不等于观看时长统计。
