# 求片系统

求片系统用于搜索 TMDB / Bangumi 条目、检查 Emby 库存、提交用户请求、管理员审核，以及外部下载系统回写处理状态。

## 状态规则

求片状态由 `internal/store/media_request.go` 统一管理，handler 不应直接拼接或判断状态字符串。

| 规范状态 | 兼容输入 | 含义 |
| ---- | ---- | ---- |
| `UNHANDLED` | `pending`, `unhandled`, `pending_review` | 待处理 |
| `ACCEPTED` | `accepted`, `approved` | 已接受 |
| `DOWNLOADING` | `downloading`, `download` | 正在下载 |
| `COMPLETED` | `completed`, `complete`, `done` | 已完成 |
| `REJECTED` | `rejected`, `reject` | 已拒绝 |

活跃队列只包含 `UNHANDLED`、`ACCEPTED`、`DOWNLOADING`。用户并发上限和全站并发上限都必须使用同一套活跃状态判断。

## 更新语义

- 管理员接口要求显式传入 `status`，空状态返回 `MEDIA_REQUEST_STATUS_INVALID`。
- 管理员更新备注时，空备注不会覆盖已有 `admin_note`。
- 外部回调接口同样要求显式传入 `status`，并通过 `X-Internal-Secret` 或 `Authorization: Bearer` 校验内部密钥。
- 外部回调的备注使用覆盖语义，允许用空备注清空旧 `admin_note`。

这些语义由 `store.UpdateMediaRequestStatus` 提供，HTTP handler 只负责鉴权、参数读取和错误码映射。
