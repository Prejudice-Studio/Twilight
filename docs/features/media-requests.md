# 求片系统

求片系统用于搜索 TMDB / Bangumi 条目、检查 Emby 库存、提交用户请求、管理员审核，以及外部下载系统回写处理状态。

## 搜索与详情

- 名称搜索选择“全部来源”时，后端并行请求 TMDB 与 Bangumi，再按来源交错合并到 `limit`。这样单一来源结果较多时不会把另一来源全部截掉，聚合搜索耗时也由两路串行之和降低为较慢一路的耗时。
- `/media/search/tmdb` 与 `/media/search/bangumi` 是来源专用别名，并按路径固定来源，避免调用别名却意外触发全部来源搜索；全部来源搜索使用 `/media/search?source=all`。
- 指定 `source=tmdb` 或 `source=bangumi` 时只请求对应来源；对应来源失败返回 `MEDIA_SEARCH_SOURCE_FAILED`。全部来源模式允许部分成功，并通过响应中的 `warnings` 告知失败来源。
- WebUI 会对同一 `source + id + media_type` 的重复结果进行合并，保留其中更完整的海报、简介、原名、日期、评分和来源链接，而不是只采用第一条记录。
- 点击搜索结果后，搜索结果已有的标题和海报会立即进入详情面板；媒体详情与 Emby 库存检查在后台并行补全。任一路失败都不会清空已有海报或关闭详情，用户仍可查看已有元数据并按既有后端规则提交求片。
- 详情面板展示来源 ID、上映日期、原名、时长、季数/集数、结束日期、来源状态、简介与库存结果；TMDB 详情还会补充标语、排名/评分人数、国家语言、制作方、主创、演员和可用预告，Bangumi 详情会补充排名/评分人数、卷数、平台、播出信息、别名、原作/导演、制作方等 `infobox` 信息。海报 URL、来源链接、官网和预告链接在渲染前分别经过图片 URL 与外部 URL 校验。
- 海报使用独立的纵向比例容器并与标题分离，桌面端详情左右两栏各自处理滚动，移动端改为单列滚动；不要通过绝对定位把标题压在海报底部，也不要依赖固定的共享弹窗高度。
- 前端详情与库存缓存均为页面内有界缓存；详情最多 40 条、库存最多 40 条，页面卸载时清空。切换条目或关闭详情会中止上一条目的在途请求，旧响应不得覆盖新选择。

## 状态规则

求片状态由 `internal/store/media_request.go` 统一管理，handler 不应直接拼接或判断状态字符串。

| 规范状态 | 兼容输入 | 含义 |
| ---- | ---- | ---- |
| `UNHANDLED` | `pending`, `unhandled`, `pending_review` | 待处理 |
| `ACCEPTED` | `accepted`, `approved` | 已接受 |
| `DOWNLOADING` | `downloading`, `download` | 正在下载 |
| `COMPLETED` | `completed`, `complete`, `done` | 已完成 |
| `REJECTED` | `rejected`, `reject` | 已拒绝 |

活跃队列只包含 `UNHANDLED`、`ACCEPTED`、`DOWNLOADING`。用户并发上限、全站并发上限、重复活跃求片检查都必须使用同一套活跃状态判断。

管理端筛选中，`active` 表示活跃队列，`pending` / `unhandled` 只表示 `UNHANDLED`。不要再把 `pending` 同时解释为“待处理”和“活跃队列”，否则前端的待处理、已接受、下载中标签会出现重复数据。

## 创建语义

创建求片必须通过 `store.CreateMediaRequestWithOptions` 完成。用户活跃求片上限、全站活跃求片上限、同源同季活跃求片去重和插入必须在 store 的同一把写锁内完成，避免多个并发请求同时通过 handler 预检后超额写入。

管理员创建求片时仍不受全站活跃队列上限约束，便于在普通用户队列占满时处理紧急情况；该豁免只影响全站上限，不绕过重复活跃求片检查。

## 更新语义

- 管理员接口要求显式传入 `status`，空状态返回 `MEDIA_REQUEST_STATUS_INVALID`。
- 管理员更新备注时，空备注不会覆盖已有 `admin_note`。
- 外部回调接口同样要求显式传入 `status`，并通过 `X-Internal-Secret` 或 `Authorization: Bearer` 校验内部密钥。
- 外部回调的备注使用覆盖语义，允许用空备注清空旧 `admin_note`。

这些语义由 `store.UpdateMediaRequestStatus` 提供，HTTP handler 只负责鉴权、参数读取和错误码映射。
