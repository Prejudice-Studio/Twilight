# 工单系统

工单系统用于用户提交问题、管理员处理和双方持续回复。用户接口受 `Ticket.enabled` 控制；管理员管理接口用于历史维护，不依赖前端入口可见性。

## 状态与回复

工单状态由 `internal/store` 统一归一化：

| 状态 | 含义 |
| ---- | ---- |
| `open` | 待处理 |
| `in_progress` | 处理中 |
| `resolved` | 已解决 |
| `closed` | 已关闭 |

`replies` 是双方对话历史的唯一来源。`admin_note` 只表示最新管理员摘要和旧客户端兼容字段；管理员填写新的 `admin_note` 时，后端会追加一条管理员回复，不能用它覆盖或清空历史回复。

用户回复 `resolved` 但未关闭的工单时，工单会回到 `open` 并清空 `resolved_at`。`closed` 工单拒绝普通用户继续回复、上传或删除附件；管理员仍可在关闭工单上追加排查回复，并维护附件用于诊断。

关闭工单的修改权限由 store helper 按角色兜底执行，不能只依赖 handler 或前端隐藏按钮。

管理员更新状态、优先级、类型、`admin_note` 以及追加管理员回复时，必须通过 store 层一次 mutation 完成，避免先写状态再追加回复导致短暂不一致或部分写入。

管理端工单处理页支持点击单个工单进入会话式详情页。详情页通过 `GET /admin/tickets/{ticket_id}` 读取完整对话，通过 `POST /admin/tickets/{ticket_id}/reply` 单独追加管理员文字回复；粘贴图片仍复用 `/tickets/{ticket_id}/images`，受全局工单图片大小和数量限制。

管理端工单列表默认只返回 `open` / `in_progress`，用于聚焦待处理队列。需要查看历史归档时，前端和外部调用方应显式传 `all=1`；后端同时兼容 `status=all`，两者都会返回 `resolved` / `closed` 等全部状态。

## Telegram 通知

用户新建、回复、上传图片、关闭或重开工单时，会通知已绑定 Telegram 且开启工单通知的管理员。管理员处理工单时仍可通知其他订阅管理员用于协作，但不会把操作者自己的处理动作再推送给操作者本人。

## 并发限额

用户同时待处理 / 处理中的工单数受 `Ticket.user_open_limit` 限制，全站打开工单数受 `Ticket.global_open_limit` 限制。创建工单必须通过 store 层原子入口完成，限额检查和插入在同一把锁内执行，避免多个并发请求同时通过“先计数、后写入”的窗口。

限额命中时 HTTP 层继续返回既有错误码：

- `TICKET_USER_LIMIT_REACHED`
- `TICKET_GLOBAL_LIMIT_REACHED`

## 类型管理

`Ticket.types` 不能为空。管理员新增、删除、重命名类型会写入 store，并同步保存到 `config.toml`，避免热重载或重启后丢失。重命名类型会同步更新已有工单的历史类型字段。

类型名在 store 层强制校验：trim 后不能为空，且最长 50 字节。前端和 handler 可以提前校验以给出更友好的提示，但不能作为唯一防线。
