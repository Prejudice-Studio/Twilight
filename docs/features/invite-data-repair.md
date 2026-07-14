# 邀请关系历史脏数据修复说明

邀请关系断开必须在 `internal/store` 层完成，不能只由 handler 或前端删除单个可见边。

旧版本状态文档可能存在 `invite_relations` 的 map key 与 `InviteRelation.child_uid` 不一致的记录。读取父级、消费新邀请码前的“已有上级”判断、断开关系清理都必须以 `child_uid` 为权威字段。

执行 `DetachInvite(uid)` 时，需要删除所有 `child_uid == uid` 的关系，同时清理该用户占用的邀请码 `used_by_uid`、`used`、`use_count`、`active` 状态。这样 `/invite/me`、管理员邀请树刷新、配置重载或兼容修复逻辑都不能再从旧边或旧邀请码占用记录把上下级关系恢复回来。
