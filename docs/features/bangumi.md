# Bangumi 同步

本文介绍 Twilight 与 Bangumi（bgm.tv）相关的两块能力：通过 Emby / Jellyfin 播放 Webhook 采集观看记录，以及在求片搜索中使用 Bangumi 作为媒体数据源。同时说明 Webhook 鉴权、重放窗口、幂等去重等真实机制，并核对相关配置项与代码的一致性。

> 重要说明：当前 Go 后端的 Bangumi Webhook **只把播放事件记录为本地观看记录（`PlaybackRecord`）**，用于观看统计与导出，并不会自动调用 Bangumi API“点格子”（添加收藏 / 标记看过）。用户级 `bgm_mode` / `bgm_token` 字段已存在并保存，但当前后端尚无消费它们去做自动点格子的逻辑。本文据此对旧文档中“自动点格子”相关描述做了纠正，详见文末说明。

## 涉及代码

| 关注点 | 源码位置 |
| --- | --- |
| Webhook 鉴权、重放窗口、幂等记录 | `internal/api/bangumi_webhook.go` |
| Webhook 路由注册 | `internal/api/routes.go` |
| Bangumi API 客户端（求片搜索 / 详情） | `internal/api/bangumi_client.go` |
| 观看记录存储与去重 | `internal/store/playback.go` |
| 配置项解析 | `internal/config/config.go` |
| 用户级 `bgm_mode` / `bgm_token` 处理 | `internal/api/handlers.go` |

## 功能开关

总开关由配置项 `BangumiSync.enabled` 控制（对应后端配置字段 `BangumiEnabled`）。开启后：

- 用户侧个人设置才允许写入 `bgm_mode` / `bgm_token`（关闭时写入会返回 `BANGUMI_SYNC_DISABLED`，HTTP 403）。
- Webhook 入口 `POST /api/v1/emby/bangumi/webhook` 才会处理请求；总开关关闭时返回 `BANGUMI_SYNC_DISABLED`，HTTP 400。

`config.toml` 示例：

```toml
[BangumiSync]
enabled = true
webhook_secret = "replace-with-random-secret"
```

`webhook_secret` 对应后端配置字段 `BangumiWebhookSecret`，由 `internal/config/config.go` 读取键 `BangumiSync.webhook_secret`。

> 关于 `auto_add_collection` / `private_collection` / `block_keywords` / `min_progress_percent`：这些键虽然出现在仓库自带的 `config.toml` / `config.production.toml` 的 `[BangumiSync]` 段里，但当前 `internal/config/config.go` **并不读取它们**，后端 `Config` 结构体也没有对应字段，因此它们当前是惰性（无效）配置，不会影响任何行为。

## Bangumi Token 配置

Bangumi 涉及两类 Token，用途不同：

### 全局 Token（求片搜索 / 详情）

全局 Token 仅用于站点级 Bangumi API 请求——即求片功能里用 Bangumi 作为媒体源进行搜索与拉取条目详情（`internal/api/bangumi_client.go` 的 `searchBangumi` / `getBangumi`，由 `internal/api/media_service.go` 调用）。它**不会**作为任何用户点格子的兜底 Token。

```toml
[Global]
bangumi_token = ""
bangumi_api_url = "https://api.bgm.tv/v0"
bangumi_app_id = ""
```

对应后端配置字段与读取键：

| 配置键 | 后端字段 | 默认值 | 用途 |
| --- | --- | --- | --- |
| `Global.bangumi_token` | `BangumiToken` | 空 | 求片调用 Bangumi API 时作为 `Authorization: Bearer` 凭据 |
| `Global.bangumi_api_url` | `BangumiAPIURL` | `https://api.bgm.tv/v0` | Bangumi API 基址（出站请求受 SSRF 校验约束） |
| `Global.bangumi_app_id` | `BangumiAppID` | 空 | Bangumi 应用 ID（保留字段） |

> `BangumiAPIURL` 的出站请求与 Emby / Telegram / TMDB 共享 SSRF 否决策略：拒绝 link-local、云元数据 IP、非 http(s) scheme，以及带 query / fragment 的裸基址。

### 用户个人 Token

每个用户可在个人设置中填写自己的 Bangumi Access Token，并开启同步开关：

- `bgm_mode`（布尔）：是否开启该用户的 Bangumi 同步。
- `bgm_token`（字符串）：该用户的 Bangumi Access Token，长度上限 4096 字节，超出返回 `BANGUMI_TOKEN_TOO_LONG`。

后端在 `internal/store/store.go` 中以 `BGMMode` / `BGMToken` 字段保存，写入逻辑在 `internal/api/handlers.go` 的 `handleUpdateMe`：

- 总开关 `BangumiSync.enabled` 关闭时，写入 `bgm_mode` 或 `bgm_token` 一律 403（`BANGUMI_SYNC_DISABLED`）。
- 开启 `bgm_mode=true` 但既无已存 Token 又未在本次请求带 Token，返回 `BANGUMI_TOKEN_MISSING`，提示先填写个人 Token。

接口对外只回 `bgm_token_set`（是否已配置）和 `bgm_sync_ready`（`bgm_mode && bgm_token != ""`），不回明文 Token。

> 当前 Go 后端只保存这两个字段并在管理后台展示状态（“可同步 / 缺少个人 Token / 未启用”），尚未有任何代码用个人 Token 去调用 Bangumi API 点格子。

Token 获取地址：<https://next.bgm.tv/demo/access-token>

## Webhook 鉴权

Webhook 路由：

```text
POST /api/v1/emby/bangumi/webhook
```

该路由鉴权级别为 `AuthPublic`（免登录），凭据是与 `webhook_secret` 匹配的密钥。鉴权在解析请求体之前完成，未通过鉴权的请求不会读取 body，避免无凭据投递大体积 JSON 触发资源放大。

### 密钥来源（优先级）

1. 请求头 `X-Twilight-Bangumi-Token`（推荐）。
2. 请求头 `X-Webhook-Token`（兼容别名）。
3. 查询参数 `?token=`（兼容旧回调，**每次命中都会打 Warn 日志**，提示运维迁移到请求头，因为查询字符串可能被上游代理 / CDN 的 access log 记录）。

无论来自哪一路，密钥都与 `BangumiWebhookSecret` 用常量时间比较（`constantTimeStringEqual`，基于 `crypto/subtle`，并消除了长度不一致引入的 timing 信号）。若 `webhook_secret` 为空或不匹配，返回 HTTP 403（`UNAUTHORIZED`，消息“Webhook 密钥无效”）。

推荐的 Emby 通知地址（用请求头携带密钥，而非 query）：

```text
https://你的后端域名/api/v1/emby/bangumi/webhook
```

并在通知请求头中加入：

```text
X-Twilight-Bangumi-Token: replace-with-random-secret
X-Twilight-Bangumi-Timestamp: <Unix 秒>
```

若只能用 URL 携带密钥，可退化为：

```text
https://你的后端域名/api/v1/emby/bangumi/webhook?token=replace-with-random-secret
```

但生产环境不建议这样做。

### 重放窗口（X-Twilight-Bangumi-Timestamp）

请求头 `X-Twilight-Bangumi-Timestamp` 携带 Unix 秒级时间戳，用于重放保护：

- 容忍窗口为 **300 秒**（`bangumiWebhookReplayWindowSeconds`），覆盖常见的客户端时钟漂移。
- 时间戳与服务器当前时间偏差超过窗口，返回 HTTP 410（`UNAUTHORIZED`，消息“Webhook 请求已过期”）。
- 时间戳非法（无法解析为整数），返回 HTTP 400（“Webhook timestamp 非法”）。
- 缺失该头时不报错，按兼容路径继续处理，但会打 Warn 日志提示运维补齐，此时无法做窗口校验。

### 幂等去重（uid, item_id, played_at）

观看记录写入走 `AddPlaybackRecordIdempotent`（`internal/store/playback.go`），以 `(UID, ItemID, PlayedAt)` 三元组作为幂等键：当三者均非空且已存在相同记录时，跳过写入并返回 `inserted=false`，Webhook 会打 Info 日志“deduplicated by idempotency key”。

- `PlayedAt` 优先取自 `X-Twilight-Bangumi-Timestamp`，因此同一份字节重放会命中相同 `PlayedAt`，即便落在重放窗口内、甚至同一秒内重放，也会被幂等键挡住，避免观看记录无限堆积。
- 缺少时间戳头的兼容路径才回落到 `time.Now()`。
- 这是“重放窗口 + 幂等键”的双层防御：窗口拦窗口外的重放，幂等键兜底窗口内 / 同字节重放。

> `ItemID` 为空的记录（例如管理员手动注入的测试事件）不参与幂等检查，允许重复写入。

## Emby / Jellyfin 通知配置与负载

Webhook 期望接收 JSON 通知。后端从负载中按以下规则解析：

- 事件名取自 `Event` / `NotificationType` / `Name` 任一字段（小写后匹配）。
- 当存在 `Item` 且满足以下任一条件时才会落库：事件名包含 `stop` 或 `played`，或负载带有 `PlaybackPositionTicks` 字段。
- 用户 ID 依次尝试 `UserId` / `UserID` → `User.Id` / `User.ID` → `Session.UserId` / `Session.UserID`，再用 `FindUserByEmbyID` 映射到本地账号；映射不到则不落库（HTTP 仍返回 accepted）。

落库的观看记录字段来源：

| 记录字段 | 来源 |
| --- | --- |
| `UID` | 由 Emby 用户 ID 映射出的本地账号 UID |
| `ItemID` | `Item.Id` / `Item.ID` |
| `Title` | `Item.Name`，回退 `Item.SeriesName` |
| `MediaType` | `Item.Type` |
| `Duration`（秒） | `PlaybackPositionTicks / 1e7`，为 0 时回退 `Item.RunTimeTicks / 1e7` |
| `PlayedAt` | `X-Twilight-Bangumi-Timestamp`，缺失时为 `time.Now()` |

通知负载示例：

```json
{
  "Event": "playback.stop",
  "User": { "Id": "emby-user-id", "Name": "embyname" },
  "Item": {
    "Type": "Episode",
    "Id": "12345",
    "Name": "第 3 话",
    "SeriesName": "番剧名",
    "RunTimeTicks": 14400000000
  }
}
```

无论是否成功落库，鉴权与重放校验通过后接口都会返回成功 envelope，`data` 形如：

```json
{ "accepted": true, "subject_name": "番剧名", "episode": 3 }
```

## 观看记录的去向

成功落库的 `PlaybackRecord` 进入单一状态文档（`internal/store`）的 `PlaybackRecords` 列表，最多保留 10000 条（超出按时间裁剪）。这些记录用于：

- 观看统计接口（`handlers.go` 的观看统计 / 全站统计）。
- 管理员导出 CSV（`/api/v1/...` 播放统计导出）。
- Telegram Bot 的个人观看汇总。

它们不会触发任何对 Bangumi API 的写操作。

## 求片中的 Bangumi 数据源

求片搜索可使用 Bangumi 作为媒体源：

- 路由 `GET /api/v1/media/search/bangumi`（`AuthUser`）与 `GET /api/v1/media/bangumi/:bgm_id`（`AuthUser`）。
- 搜索请求 `POST {BangumiAPIURL}/search/subjects`，`filter.type` 取 `[2, 6]`（动画 / 三次元），允许 NSFW，按 `match` 排序。
- 详情请求 `GET {BangumiAPIURL}/subjects/{id}`，`id` 必须为正整数。
- 凭据为 `Global.bangumi_token`（若配置则加 `Authorization: Bearer`），与用户个人 Token 无关。

返回结果被规整为统一媒体结构，包含标题（优先 `name_cn`）、海报、类型（书籍 / 动画 / 音乐 / 游戏 / 三次元）、简介、首播日期、评分、标签等。

## 错误码

| 错误码 | HTTP | 触发场景 |
| --- | --- | --- |
| `BANGUMI_SYNC_DISABLED` | 400 / 403 | 总开关未开启（Webhook 400；写入用户设置 403） |
| `UNAUTHORIZED` | 403 | Webhook 密钥为空或不匹配 |
| `UNAUTHORIZED` | 410 | 时间戳超出重放窗口（“Webhook 请求已过期”） |
| `UNAUTHORIZED` | 400 | 时间戳非法（“Webhook timestamp 非法”） |
| `BANGUMI_TOKEN_TOO_LONG` | 400 | 个人 `bgm_token` 超过 4096 字节 |
| `BANGUMI_TOKEN_MISSING` | 400 | 开启 `bgm_mode` 但未提供个人 Token |

## 排错

- Webhook 返回“Bangumi 同步未启用”：检查 `BangumiSync.enabled=true`。
- Webhook 返回“Webhook 密钥无效”（403）：检查请求头 `X-Twilight-Bangumi-Token`（或兼容的 `X-Webhook-Token` / `?token=`）是否与 `webhook_secret` 一致。
- Webhook 返回“Webhook 请求已过期”（410）：检查 Emby 与后端时钟是否同步，必要时校准 NTP；偏差需在 300 秒内。
- Webhook 返回“Webhook timestamp 非法”（400）：`X-Twilight-Bangumi-Timestamp` 必须是 Unix 秒级整数。
- 日志出现“仍在使用 ?token= 查询参数”Warn：把密钥从 URL 迁移到请求头。
- 日志出现“deduplicated by idempotency key”Info：同一 `(uid, item_id, played_at)` 被重复投递，属正常去重，不是错误。
- 接口返回成功但未生成观看记录：确认该 Emby 账号已在 Twilight 中绑定（`FindUserByEmbyID` 能映射到本地账号），且事件名 / 字段满足落库条件。
- 用户设置写 `bgm_mode` / `bgm_token` 报 403：先开启 `BangumiSync.enabled`。
- 启用同步报 `BANGUMI_TOKEN_MISSING`：先填写个人 Token 再开启 `bgm_mode`。

## 相对文档

- 安全机制（CSRF、SSRF、鉴权级别）：[../guides/security.md](../guides/security.md)
- 后端架构与配置项：[../reference/backend.md](../reference/backend.md)
- API 路由索引：[../reference/api-index.md](../reference/api-index.md)
- 求片功能相关 API：[../reference/backend-api.md](../reference/backend-api.md)
