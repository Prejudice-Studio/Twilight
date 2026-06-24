# Bangumi 同步

本文介绍 Twilight 与 Bangumi（bgm.tv）相关的完整能力：通过 Emby / Jellyfin 播放 Webhook 采集观看记录 → 自动/手动同步到 Bangumi 点格子（添加收藏 / 标记看过），以及在求片搜索中使用 Bangumi 作为媒体数据源。同时说明 Webhook 鉴权、重放窗口、幂等去重、同步流程、前端页面与 Emby / Jellyfin 端配置等真实机制。

## 涉及代码

| 关注点 | 源码位置 |
| --- | --- |
| Webhook 鉴权、重放窗口、幂等记录 | `internal/api/bangumi_webhook.go` |
| Bangumi 同步服务（搜索匹配 → 收藏 → 标记剧集） | `internal/api/bangumi_sync_service.go` |
| Bangumi 同步 API（用户端 + 管理员端 handler） | `internal/api/bangumi_sync_handlers.go` |
| Bangumi API 客户端（求片搜索 / 详情） | `internal/api/bangumi_client.go` |
| Webhook 路由注册 | `internal/api/routes.go` |
| 观看记录存储与去重 | `internal/store/playback.go` |
| 同步日志存储（`BangumiSyncLog`） | `internal/store/store.go` |
| 配置项解析 | `internal/config/config.go` |
| 用户级 `bgm_mode` / `bgm_token` 处理 | `internal/api/handlers.go` |
| 用户端 Bangumi 仪表盘页面 | `webui/src/app/(main)/bangumi/page.tsx` |
| 管理员 Bangumi 管理页面 | `webui/src/app/(main)/admin/bangumi/page.tsx` |
| 前端 API 客户端 | `webui/src/lib/api.ts` |
| 前端类型定义 | `webui/src/lib/api-types.ts` |

## 功能总览

```
Emby/Jellyfin 播放停止
        │
        ▼
┌──────────────────────┐
│  Webhook (AuthPublic) │  ← X-Twilight-Bangumi-Token / replay window / 幂等去重
└──────┬───────────────┘
       │ 落库 PlaybackRecord (UID / ItemID / Title / SeriesName / IndexNumber / Duration / PlayedAt)
       ▼
┌──────────────────────┐
│  PlaybackRecords      │  本地观看记录（最多 10000 条）
└──────┬───────────────┘
       │ 用户 / 管理员触发同步
       ▼
┌──────────────────────┐
│  Bangumi Sync Service │  用户个人 Token → 搜索匹配 → 添加收藏 → 标记剧集看过
└──────┬───────────────┘
       │ 落库 BangumiSyncLog (success / failed / skipped)
       ▼
     Bangumi (bgm.tv)
```

## 功能开关

Bangumi 功能有两个独立开关，互不影响：

### 同步开关 `BangumiSync.enabled`

对应后端配置字段 `BangumiEnabled`，仅控制同步相关功能：

- Webhook 入口 `POST /api/v1/emby/bangumi/webhook` 处理请求（关闭时返回 `BANGUMI_SYNC_DISABLED`，HTTP 400）。
- 同步触发 API（`POST /api/v1/bangumi/sync/trigger` 和管理员 `POST /api/v1/admin/bangumi/sync/:uid`）。
- 同步历史查看和清除（`GET/DELETE /api/v1/bangumi/sync/history`）。
- 用户设置中的 `bgm_mode` / `bgm_token` 写入（关闭时返回 `BANGUMI_SYNC_DISABLED`，HTTP 403）。

**不影响**：个人收藏查看/管理、收藏修改等管理功能。

### 管理开关 `BangumiSync.manage_enabled`

对应后端配置字段 `BangumiManageEnabled`，仅控制管理相关功能：

- 用户 Bangumi 个人页（`GET /api/v1/bangumi/me`），关闭时返回 `bgm_manage_disabled: true`。
- 收藏列表查看（`GET /api/v1/bangumi/collections`），关闭时返回 `BANGUMI_MANAGE_DISABLED`。
- 收藏状态/进度/评分修改（`PATCH /api/v1/bangumi/collections/:subject_id`），关闭时返回 `BANGUMI_MANAGE_DISABLED`。
- 用户设置中 `bgm_manage_mode` 写入（关闭时返回 `BANGUMI_MANAGE_DISABLED`，HTTP 403）。

**不影响**：Webhook、同步触发、同步历史等同步功能。

`config.toml` 示例：

```toml
[BangumiSync]
enabled = true
manage_enabled = true
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

每个用户可在 Bangumi 仪表盘页面或设置页中填写自己的 Bangumi Access Token，并开启同步开关：

- `bgm_mode`（布尔）：是否开启该用户的 Bangumi 同步。
- `bgm_token`（字符串）：该用户的 Bangumi Access Token，长度上限 4096 字节，超出返回 `BANGUMI_TOKEN_TOO_LONG`。

后端在 `internal/store/store.go` 中以 `BGMMode` / `BGMToken` 字段保存，写入逻辑在 `internal/api/handlers.go` 的 `handleUpdateMe`：

- 总开关 `BangumiSync.enabled` 关闭时，写入 `bgm_mode` 或 `bgm_token` 一律 403（`BANGUMI_SYNC_DISABLED`）。
- 开启 `bgm_mode=true` 但既无已存 Token 又未在本次请求带 Token，返回 `BANGUMI_TOKEN_MISSING`，提示先填写个人 Token。

接口对外只回 `bgm_token_set`（是否已配置）和 `bgm_sync_ready`（`bgm_mode && bgm_token != ""`），不回明文 Token。

Token 获取地址：<https://next.bgm.tv/demo/access-token>

## Emby / Jellyfin Webhook 配置

这是 Banugmi 同步的数据来源端配置。你需要让 Emby / Jellyfin 在**播放停止**时向 Twilight 发送 Webhook 通知。

### 第一步：生成随机密钥

```bash
# 生成一个安全的随机密钥（Linux / macOS）
openssl rand -hex 32

# 或使用 PowerShell（Windows）
[Convert]::ToHexString((New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes(32))
```

将生成的密钥填入 `config.toml`：

```toml
[BangumiSync]
webhook_secret = "你生成的随机密钥"
```

重启 Twilight 后端使配置生效。

### 第二步：在 Emby 中添加 Webhook 通知

1. 打开 Emby 管理后台 → **通知**（Notifications）。
2. 点击 **+ 添加通知**（Add Notification）。
3. 通知类型选择 **Webhook**。
4. 填写以下配置：

| 配置项 | 值 |
| --- | --- |
| **Webhook URL** | `https://你的Twilight域名/api/v1/emby/bangumi/webhook` |
| **Webhook 请求头** | 见下方 |
| **事件** | 勾选 **播放停止**（Playback Stop） |

**请求头配置（推荐方式）：**

添加自定义请求头：

| 请求头名 | 值 |
| --- | --- |
| `X-Twilight-Bangumi-Token` | 你在第一步生成的 `webhook_secret` |
| `Content-Type` | `application/json` |

截图式参考——Emby Webhook 通知配置示例：

```
┌─────────────────────────────────────────────────────────────┐
│ 名称:        Twilight Bangumi Sync                          │
│ 通知方式:    Webhook                                         │
│                                                             │
│ Webhook URL: https://你的域名/api/v1/emby/bangumi/webhook    │
│                                                             │
│ 自定义请求头:                                                │
│   X-Twilight-Bangumi-Token: your-secret-here                 │
│   Content-Type: application/json                            │
│                                                             │
│ ☑ 播放停止 (Playback Stop)                                   │
│ ☑ 向所有用户发送                                            │
│ ☑ 包含项目数据                                              │
└─────────────────────────────────────────────────────────────┘
```

> **注意**：必须勾选「包含项目数据」（Send all item properties / Include item data），否则 Emby 发送的 JSON 不包含 `Item.Id`、`Item.SeriesName`、`Item.IndexNumber` 等关键字段，导致同步匹配失败。

### 第三步：在 Jellyfin 中添加 Webhook 通知

Jellyfin 10.9+ 使用插件方式配置 Webhook：

1. 在 Jellyfin 管理后台 → **插件** → **目录**，安装 **Webhook** 插件。
2. 重启 Jellyfin 后，管理后台 → **插件** → **Webhook** → **添加 Generic Destination**。
3. 填写以下配置：

| 配置项 | 值 |
| --- | --- |
| **Webhook Name** | Twilight Bangumi Sync |
| **Webhook Url** | `https://你的Twilight域名/api/v1/emby/bangumi/webhook` |
| **Notification Type** | 勾选 **Playback Stop** |
| **Request Header** 的 Key | `X-Twilight-Bangumi-Token` |
| **Request Header** 的 Value | 你在第一步生成的 `webhook_secret` |
| **Template** | 留空（使用 Jellyfin 默认 JSON 负载） |
| **Send All Properties** | 开启（`true`） |

### 第四步（可选）：配置 Emby Webhook 插件自动添加时间戳头

如果你使用插件版 Emby Webhook（如 Jellyfin 的 Webhook 插件或 Emby 的第三方 webhook 扩展），可额外配置 `X-Twilight-Bangumi-Timestamp` 请求头为 `{Timestamp}` 或 `{Now}` 模板变量（取决于具体插件），以启用重放窗口校验。Emby 原生 Webhook 不支持动态请求头模板，缺少该头时仅打 Warn 日志、正常处理。

### 第五步：验证 Webhook 是否正常

在 Emby / Jellyfin 中播放任意媒体，等待数秒后**停止播放**。然后检查 Twilight 后端日志，应能看到类似输出：

```
bangumi webhook playback record stored   uid=123  item_id=abc  title="第 3 话"
```

或去重日志（同一播放记录重复投递）：

```
bangumi webhook playback record deduplicated by idempotency key
```

如果看不到任何日志，请检查：

- Emby / Jellyfin 的 Webhook 通知日志（通常在其管理面板的「通知日志」中可见发送状态与响应码）。
- 确认 Emby 账号已在 Twilight 中绑定（`FindUserByEmbyID` 能映射到本地账号）。
- 确认通知事件勾选了「播放停止」并开启了「包含项目数据」。

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

无论来自哪一路，密钥都与 `BangumiWebhookSecret` 用常量时间比较（`constantTimeStringEqual`，基于 `crypto/subtle`，并消除了长度不一致引入的 timing 信号）。若 `webhook_secret` 为空或不匹配，返回 HTTP 403（`UNAUTHORIZED`，消息"Webhook 密钥无效"）。

若只能用 URL 携带密钥，可退化为：

```text
https://你的后端域名/api/v1/emby/bangumi/webhook?token=replace-with-random-secret
```

但生产环境不建议这样做。

### 重放窗口（X-Twilight-Bangumi-Timestamp）

请求头 `X-Twilight-Bangumi-Timestamp` 携带 Unix 秒级时间戳，用于重放保护：

- 容忍窗口为 **300 秒**（`bangumiWebhookReplayWindowSeconds`），覆盖常见的客户端时钟漂移。
- 时间戳与服务器当前时间偏差超过窗口，返回 HTTP 410（`UNAUTHORIZED`，消息"Webhook 请求已过期"）。
- 时间戳非法（无法解析为整数），返回 HTTP 400（"Webhook timestamp 非法"）。
- 缺失该头时不报错，按兼容路径继续处理，但会打 Warn 日志提示运维补齐，此时无法做窗口校验。

### 幂等去重（uid, item_id, played_at）

观看记录写入走 `AddPlaybackRecordIdempotent`（`internal/store/playback.go`），以 `(UID, ItemID, PlayedAt)` 三元组作为幂等键：当三者均非空且已存在相同记录时，跳过写入并返回 `inserted=false`，Webhook 会打 Info 日志"deduplicated by idempotency key"。

- `PlayedAt` 优先取自 `X-Twilight-Bangumi-Timestamp`，因此同一份字节重放会命中相同 `PlayedAt`，即便落在重放窗口内、甚至同一秒内重放，也会被幂等键挡住，避免观看记录无限堆积。
- 缺少时间戳头的兼容路径才回落到 `time.Now()`。
- 这是"重放窗口 + 幂等键"的双层防御：窗口拦窗口外的重放，幂等键兜底窗口内 / 同字节重放。

> `ItemID` 为空的记录（例如管理员手动注入的测试事件）不参与幂等检查，允许重复写入。

## Emby / Jellyfin 通知负载与记录字段

Webhook 期望接收 JSON 通知。后端从负载中按以下规则解析：

- 事件名取自 `Event` / `NotificationType` / `Name` 任一字段（小写后匹配）。
- 当存在 `Item` 且满足以下任一条件时才会落库：事件名包含 `stop` 或 `played`，或负载带有 `PlaybackPositionTicks` 字段。
- 用户 ID 依次尝试 `UserId` / `UserID` → `User.Id` / `User.ID` → `Session.UserId` / `Session.UserID`，再用 `FindUserByEmbyID` 映射到本地账号；映射不到则不落库（HTTP 仍返回 accepted）。

落库的观看记录字段来源：

| 记录字段 | 来源 | 用途 |
| --- | --- | --- |
| `UID` | 由 Emby 用户 ID 映射出的本地账号 UID | 归属用户标识 |
| `ItemID` | `Item.Id` / `Item.ID` | 幂等去重键、同步日志关联 |
| `Title` | `Item.Name`，回退 `Item.SeriesName` | 条目搜索查询 |
| `SeriesName` | `Item.SeriesName` | Bangumi 搜索匹配（优先于 Title） |
| `MediaType` | `Item.Type` | 条目类型标识 |
| `IndexNumber` | `Item.IndexNumber` | Bangumi 剧集编号（标记第几话看过） |
| `Duration`（秒） | `PlaybackPositionTicks / 1e7`，为 0 时回退 `Item.RunTimeTicks / 1e7` | 观看进度统计 |
| `PlayedAt` | `X-Twilight-Bangumi-Timestamp`，缺失时为 `time.Now()` | 幂等去重键、时间排序 |

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
    "IndexNumber": 3,
    "RunTimeTicks": 14400000000
  }
}
```

无论是否成功落库，鉴权与重放校验通过后接口都会返回成功 envelope，`data` 形如：

```json
{ "accepted": true, "subject_name": "番剧名", "episode": 3 }
```

## 同步到 Bangumi（点格子）

### 同步流程

当用户或管理员触发同步时，后端 `syncBangumiForUser`（`internal/api/bangumi_sync_service.go`）执行以下流程：

1. **读取用户状态**：检查 `BGMMode` 与 `BGMToken`，不满足则跳过。
2. **获取未同步记录**：读取用户所有 `PlaybackRecord`，排除已在 `BangumiSyncLog` 中标记为 `success` 的条目。
3. **逐条同步**（每步均检查 context 取消以确保可中断）：
   - **搜索匹配**：以 `SeriesName`（回退 `Title`）在 Bangumi 搜索 `/search/subjects`，取第一条匹配结果。
   - **添加收藏**：对匹配到的 `subject_id` 调用 `POST /users/-/collections/{subject_id}`，`type=3`（看过），如已在收藏中（400/409）不视为错误。
   - **标记剧集**：若 `IndexNumber > 0`，调用 `POST /users/-/collections/{subject_id}/episodes`，`type=2`（看过）标记该剧集。
   - **写入日志**：成功 / 失败均写入 `BangumiSyncLog`，成功记录含 `SubjectID`、`SubjectName`、`Episode`。

4. **返回摘要**：`synced` / `skipped` / `failed` 三计数 + 详细 `logs`。

### 触发方式

| 触发方式 | 接口 | 鉴权 |
| --- | --- | --- |
| 用户手动触发 | `POST /api/v1/bangumi/sync/trigger` | `AuthUser` |
| 管理员触发单个用户 | `POST /api/v1/admin/bangumi/sync/:uid` | `AuthAdmin` |

> 同步超时默认 5 分钟（管理员触发时 `context.WithTimeout` 5min），用户触发沿用请求 context。

### 同步日志

同步日志以 `BangumiSyncLog` 实体持久化（`internal/store/store.go`），最多保留 5000 条，超出自动裁剪。日志字段：

| 字段 | 说明 |
| --- | --- |
| `ID` | 自增 ID |
| `UID` | 用户 UID |
| `RecordItemID` | 关联的播放记录 `ItemID` |
| `SubjectID` | 匹配到的 Bangumi 条目 ID |
| `SubjectName` | 匹配到的 Bangumi 条目标题 |
| `Episode` | 标记的剧集编号 |
| `Status` | `success` / `failed` / `skipped` |
| `Message` | 结果描述 |
| `CreatedAt` | 同步时间（Unix 秒） |

用户可查看自己的最近 50 条同步日志，管理员可查看任意用户的最近 100 条日志。用户和管理员均可清除同步日志。

## 观看记录的去向

成功落库的 `PlaybackRecord` 进入单一状态文档（`internal/store`）的 `PlaybackRecords` 列表，最多保留 10000 条（超出按时间裁剪）。这些记录用于：

- **Bangumi 同步**：同步服务读取未同步记录，调用 Bangumi API 点格子。
- 观看统计接口（`handlers.go` 的观看统计 / 全站统计）。
- 管理员导出 CSV（`/api/v1/...` 播放统计导出）。
- Telegram Bot 的个人观看汇总。

## 前端页面

### 用户端：Bangumi 仪表盘

路径：`/bangumi`

功能：

- **同步状态卡片**：四格统计（总记录数 / 已同步数 / 就绪状态 / Token 配置状态）。
- **同步操作**：「开始同步」按钮手动触发同步；「清除历史」按钮清除同步日志。
- **Bangumi 设置**：开启/关闭同步开关；开启/关闭管理功能开关；填写 / 清除个人 Access Token。
- **同步历史**：最近 50 条同步日志，含状态图标（成功/失败/跳过）、匹配的 Bangumi 条目名、时间。
- **个人收藏视图**（需开启管理功能）：显示 Bangumi 账号关联信息（头像、昵称、签名）、在看/想看/看过三列精选卡片（每类 8 条）。
  - 卡片显示：封面、标题、进度话数、评分（StarRating 组件）。
  - 「进度/状态」按钮打开编辑对话框，支持修改：观看状态（想看/看过/在看）、观看进度（话数）、评分（0-10 点选）。
  - 「查看全部」打开分页对话框浏览该分类所有条目。
  - 每张卡片底部显示 Bangumi 详情链接，点击跳转 Bangumi 条目页。

### 管理员端：Bangumi 管理

路径：`/admin/bangumi`

功能：

- **用户列表**（分页展示，默认每页 20 条，支持搜索用户名/UID 和上下翻页）：所有用户及其 Bangumi 同步状态（同步开关 / 管理开关 / Token 配置 / 就绪 / 播放记录数 / 已同步数）。
- **逐用户操作**：
  - 「播放记录」按钮：弹窗查看该用户的播放记录列表，已同步条目标注匹配的 Bangumi 条目名。
  - 「同步日志」按钮：弹窗查看该用户的同步历史。
  - 「同步」按钮：为该用户手动触发一次同步。
  - 删除按钮：清除该用户的同步日志。

## 求片中的 Bangumi 数据源

求片搜索可使用 Bangumi 作为媒体源：

- 路由 `GET /api/v1/media/search/bangumi`（`AuthUser`）与 `GET /api/v1/media/bangumi/:bgm_id`（`AuthUser`）。
- 搜索请求 `POST {BangumiAPIURL}/search/subjects`，`filter.type` 取 `[2, 6]`（动画 / 三次元），允许 NSFW，按 `match` 排序。
- 详情请求 `GET {BangumiAPIURL}/subjects/{id}`，`id` 必须为正整数。
- 凭据为 `Global.bangumi_token`（若配置则加 `Authorization: Bearer`），与用户个人 Token 无关。

返回结果被规整为统一媒体结构，包含标题（优先 `name_cn`）、海报、类型（书籍 / 动画 / 音乐 / 游戏 / 三次元）、简介、首播日期、评分、标签等。

## API 路由索引

### 用户端

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/bangumi/sync/status` | `AuthUser` | 获取当前用户的同步状态与最近日志 |
| `POST` | `/api/v1/bangumi/sync/trigger` | `AuthUser` | 手动触发一次同步 |
| `GET` | `/api/v1/bangumi/sync/history` | `AuthUser` | 获取同步历史日志（`?limit=`） |
| `DELETE` | `/api/v1/bangumi/sync/history` | `AuthUser` | 清除当前用户的同步历史 |
| `GET` | `/api/v1/bangumi/me` | `AuthUser` | 获取 Bangumi 用户资料 + 在看/想看/看过精选（各 8 条） |
| `GET` | `/api/v1/bangumi/collections` | `AuthUser` | 分页获取 Bangumi 收藏列表（`?type=&limit=&offset=`） |
| `PATCH` | `/api/v1/bangumi/collections/:subject_id` | `AuthUser` | 修改收藏状态/进度/评分（优先 PATCH，404 时回退 POST） |

### 管理员端

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/admin/bangumi/users` | `AuthAdmin` | 列出所有用户的 Bangumi 同步状态 |
| `GET` | `/api/v1/admin/bangumi/records/:uid` | `AuthAdmin` | 查看某用户的播放记录（`?limit=`） |
| `POST` | `/api/v1/admin/bangumi/sync/:uid` | `AuthAdmin` | 为某用户触发同步 |
| `GET` | `/api/v1/admin/bangumi/logs/:uid` | `AuthAdmin` | 查看某用户的同步日志（`?limit=`） |
| `DELETE` | `/api/v1/admin/bangumi/logs/:uid` | `AuthAdmin` | 清除某用户的同步日志 |

### Webhook（公开）

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/v1/emby/bangumi/webhook` | `AuthPublic` | 接收 Emby/Jellyfin 播放通知 |

## 错误码

| 错误码 | HTTP | 触发场景 |
| --- | --- | --- |
| `BANGUMI_SYNC_DISABLED` | 400 / 403 | 同步开关未开启（Webhook / 触发同步 / 同步历史 / 写入同步设置） |
| `BANGUMI_MANAGE_DISABLED` | 400 / 403 | 管理开关未开启（收藏查看 / 收藏修改 / 写入管理设置） |
| `UNAUTHORIZED` | 403 | Webhook 密钥为空或不匹配 |
| `UNAUTHORIZED` | 410 | 时间戳超出重放窗口（"Webhook 请求已过期"） |
| `UNAUTHORIZED` | 400 | 时间戳非法（"Webhook timestamp 非法"） |
| `BANGUMI_TOKEN_TOO_LONG` | 400 | 个人 `bgm_token` 超过 4096 字节 |
| `BANGUMI_TOKEN_MISSING` | 400 | 开启 `bgm_mode` 或触发同步但未提供个人 Token |

### 收藏管理约束

Bangumi API 对 `ep_status`（完成度）的修改有限制：**只能用于修改书籍类条目（subject_type=1）的完成度**。对于动画/剧集（subject_type=2）修改 `ep_status` 会返回 400。因此：

- 后端 `updateBangumiCollection` 仅当 `ep_status > 0` 时才将其包含在请求体中。
- 前端编辑对话框在切换收藏类型为非「在看」（type=3）或「看过」（type=2）时自动将 `ep_status` 置为 0 并阻止发送。
- 如需修改动画/剧集的剧集进度，请直接使用 Bangumi 网站操作。

### 收藏修改请求处理

`PATCH /api/v1/bangumi/collections/:subject_id` 处理流程：

1. 先尝试 `PATCH /v0/users/-/collections/{subject_id}`（修改已有收藏）。
2. 若收到 404（条目未收藏），回退 `POST /v0/users/-/collections/{subject_id}`（新建收藏）。
3. `ep_status` 仅当 > 0 时才包含在请求体中（适配 Bangumi API 约束）。
4. 请求体固定包含 `type`（收藏类型）和 `rate`（评分）。

## 排错

### Webhook 端

- Webhook 返回"Bangumi 同步未启用"：检查 `BangumiSync.enabled=true`。
- Webhook 返回"Webhook 密钥无效"（403）：检查请求头 `X-Twilight-Bangumi-Token`（或兼容的 `X-Webhook-Token` / `?token=`）是否与 `webhook_secret` 一致。
- Webhook 返回"Webhook 请求已过期"（410）：检查 Emby 与后端时钟是否同步，必要时校准 NTP；偏差需在 300 秒内。
- Webhook 返回"Webhook timestamp 非法"（400）：`X-Twilight-Bangumi-Timestamp` 必须是 Unix 秒级整数。
- 日志出现"仍在使用 ?token= 查询参数"Warn：把密钥从 URL 迁移到请求头。
- 日志出现"deduplicated by idempotency key"Info：同一 `(uid, item_id, played_at)` 被重复投递，属正常去重，不是错误。
- 接口返回成功但未生成观看记录：确认该 Emby 账号已在 Twilight 中绑定（`FindUserByEmbyID` 能映射到本地账号），且事件名 / 字段满足落库条件。同时确认 Emby Webhook 通知开启了「包含项目数据」。
- 记录中缺少 `SeriesName` / `IndexNumber`：检查 Emby/Jellyfin Webhook 配置中是否开启了「发送所有属性 / Include item data」，未开启会导致 JSON 负载缺少这些关键字段，影响同步匹配精度。

### 同步端

- 用户设置写 `bgm_mode` / `bgm_token` 报 403：先开启 `BangumiSync.enabled`。
- 启用同步报 `BANGUMI_TOKEN_MISSING`：先填写个人 Token 再开启 `bgm_mode`。
- 同步时大量 `failed`（匹配失败）：检查用户 Token 是否有效（可在 Bangumi 个人设置中重新获取）。匹配依赖 `SeriesName` 命中率，若 Emby 中条目名与 Bangumi 差异大（如译名不一致），可能导致匹配失败。当前使用 Bangumi `/search/subjects` 取第一条结果，精度有限。
- 同步成功但 Bangumi 上没有标记：确认 Bangumi Access Token 是否过期；确认 Bangumi API 是否可达（`Global.bangumi_api_url`）。

## 相对文档

- 安全机制（CORS、SSRF、鉴权级别）：[../guides/security.md](../guides/security.md)
- 后端架构与配置项：[../reference/backend.md](../reference/backend.md)
- API 路由索引：[../reference/api-index.md](../reference/api-index.md)
- 求片功能相关 API：[../reference/backend-api.md](../reference/backend-api.md)
