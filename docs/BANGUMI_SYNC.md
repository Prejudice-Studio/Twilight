# Bangumi 点格子同步配置

Twilight 可接收 Emby 通知 Webhook，在用户看完番剧单集或剧场版后调用 Bangumi API 自动点格子。实现思路参考 [Bangumi-syncer](https://github.com/SanaeMio/Bangumi-syncer)：只处理看完事件，按 Emby 用户映射到本地账号，并且只使用该用户自己的 Bangumi Token 同步。

## 功能开关

管理员进入 `管理后台 -> 配置管理 -> Bangumi 点格子`，开启 `enabled` 后：

- 用户侧 `个人设置` 才会显示 Bangumi 点格子配置面板。
- Emby Webhook 入口才会处理同步请求。
- 用户仍需在个人设置中启用 `Bangumi 同步`，否则该用户的播放不会同步。

对应 `config.toml`：

```toml
[BangumiSync]
enabled = true
webhook_secret = "replace-with-random-secret"
auto_add_collection = true
private_collection = false
block_keywords = []
min_progress_percent = 80
```

## Token 配置

推荐每个用户在 `个人设置 -> Bangumi 同步` 填写自己的 Bangumi Access Token，并开启同步。

全局 Bangumi Token 仅用于站点级 Bangumi API 请求，不会作为点格子兜底 Token：

```toml
[Global]
bangumi_token = ""
```

Token 获取地址：<https://next.bgm.tv/demo/access-token>

## Emby 通知配置

Webhook 地址：

```text
https://你的后端域名/api/v1/emby/bangumi/webhook?token=replace-with-random-secret
```

如果 `webhook_secret` 为空，可以不带 `token`，但生产环境不建议这样做。

Emby 需要发送 JSON 通知，至少包含以下事件之一：

- `item.markplayed`
- `playback.stop`

`playback.stop` 会优先读取 `PlaybackInfo.PlayedToCompletion=true`；没有该字段时，按 `min_progress_percent` 判断播放进度。

通知负载需包含 `User` 与 `Item` 信息，例如：

```json
{
  "Event": "playback.stop",
  "User": { "Id": "emby-user-id", "Name": "embyname" },
  "Item": {
    "Type": "Episode",
    "SeriesName": "番剧名",
    "ParentIndexNumber": 1,
    "IndexNumber": 3,
    "PremiereDate": "2026-04-01T00:00:00.0000000Z"
  },
  "PlaybackInfo": {
    "PlayedToCompletion": true
  }
}
```

## 同步规则

- 只同步已绑定 Emby 的本地用户。
- 用户个人 `BGM_MODE` 未开启时跳过。
- 只使用用户个人 `BGM_TOKEN`；用户未配置个人 Token 时直接跳过，不使用全局 `Global.bangumi_token`。
- 剧集用 `SeriesName + ParentIndexNumber + IndexNumber` 匹配 Bangumi 条目与章节。
- 剧场版用电影标题匹配，并默认点第 1 个本篇章节。
- `auto_add_collection=true` 时，未收藏条目会先加入收藏并设为“在看”。
- `block_keywords` 命中的标题不会同步。

## 排错

- 用户侧看不到配置面板：确认 `BangumiSync.enabled=true` 并刷新页面。
- Webhook 返回“Webhook 密钥无效”：确认 URL 中的 `token` 与 `webhook_secret` 一致。
- 返回“未找到绑定该 Emby 用户的本地账号”：确认该 Emby 账号已在 Twilight 中绑定。
- 返回“用户未配置 Bangumi Token”：让用户在个人设置中填写个人 Token。
- 返回“未找到匹配的 Bangumi 条目”：可尝试调整 Emby 番剧标题、首播日期，或后续补充自定义映射。
