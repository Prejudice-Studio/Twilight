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

## 并发限额

用户同时待处理 / 处理中的工单数受 `Ticket.user_open_limit` 限制，全站打开工单数受 `Ticket.global_open_limit` 限制。创建工单必须通过 store 层原子入口完成，限额检查和插入在同一把锁内执行，避免多个并发请求同时通过“先计数、后写入”的窗口。

限额命中时 HTTP 层继续返回既有错误码：

- `TICKET_USER_LIMIT_REACHED`
- `TICKET_GLOBAL_LIMIT_REACHED`

## 类型管理

`Ticket.types` 不能为空。管理员新增、删除、重命名类型会写入 store，并同步保存到 `config.toml`，避免热重载或重启后丢失。重命名类型会同步更新已有工单的历史类型字段。
