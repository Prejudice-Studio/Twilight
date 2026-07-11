# 后端 API 参考

所有接口默认以 `/api/v1` 为前缀。

## 响应格式

大部分 JSON 响应使用统一 envelope：

```json
{
  "success": true,
  "message": "OK",
  "data": {}
}
```

错误响应同样使用该结构，`success=false`，并可能包含错误码。

## 鉴权方式

- 浏览器用户使用登录态 Cookie。
- 第三方集成使用 API Key，详见 [API Key 接入](./api-key.md)。
- 管理接口需要管理员权限，除非接口明确标注为公开。

## 文档接口

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| GET | `/api/v1` | API 根信息 |
| GET | `/api/v1/docs` | API 控制台 |
| GET | `/api/v1/openapi.json` | 仅公开路由的 OpenAPI 摘要 |
| GET | `/api/v1/system/admin/apis` | 管理员完整路由清单 |

公开 OpenAPI 不应枚举管理员接口，避免泄露完整攻击面。

## Emby 活动日志与播放统计

| 方法 | 路径 | 权限 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/admin/emby/activity-logs` | Admin | 原始 Emby 活动日志；默认节流自动刷新，`refresh=1` 强制刷新，`auto=0` 禁用隐式刷新 |
| GET | `/api/v1/emby/playback-stats` | User | 当前用户播放统计 |
| GET | `/api/v1/admin/emby/playback-stats` | Admin | 全站、自己或指定用户播放统计 |
| GET | `/api/v1/admin/emby/playback-stats/{uid}` | Admin | 指定本地 UID 的播放统计 |

常用参数：`scope`、`uid`、`days`、`today=1`、`limit`、`sort=plays|name`、`refresh=1`。

## 功能开关

后端 handler 必须直接检查功能开关。前端隐藏入口只是体验优化，不能替代服务端拒绝。

常见开关包括：

- `InviteEnabled`
- `SigninEnabled`
- `MediaRequestEnabled`
- `BangumiEnabled`
- `BangumiManageEnabled`
- `RegisterEnabled`
- `emailConfigured()`

## 更新规则

新增或修改接口时：

1. 更新 `internal/api/routes.go`。
2. 若前端使用，更新 `webui/src/lib/api.ts` 和 `api-types.ts`。
3. 更新 [API 路由索引](./api-index.md) 与相关功能文档。
