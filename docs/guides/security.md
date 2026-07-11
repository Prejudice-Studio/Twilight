# 安全加固

本文记录 Twilight 生产环境上线前应检查的安全基线。

## 密钥管理

- 不要提交真实 Token、密码、数据库连接、Telegram Bot Token、Emby API Token、TMDB Key、Bangumi Token 或内部回调密钥。
- 推荐将密钥放入 `config.local.toml` 或 `TWILIGHT_*` 环境变量。
- 如密钥出现在 Git 历史、日志、截图或工单中，应立即轮换。

## 出站 URL

Emby、Bangumi、Telegram、TMDB 等外部服务地址应复用 `validateOutboundBaseURL` 等现有安全边界。

规则：

- 仅允许 `http` / `https`。
- 拒绝空 host 和危险字面量 IP。
- 配置中的 base URL 不应包含 query 或 fragment。
- 使用共享 HTTP client 和重定向策略，不要手写临时 client 绕过检查。

## CORS 与会话

- 生产环境不要使用通配符 CORS 搭配凭据。
- 只配置可信前端 Origin。
- `localhost` / `127.0.0.1` 搭配凭据只应用于开发环境。
- 会话和 CSRF 敏感流程必须经过现有鉴权中间件。

## 上传与文件路径

- 用户可控路径必须通过 `ResolveWithinRoot`。
- 使用已有上传文件名白名单。
- 写入下载文件或上传文件时要防止 symlink TOCTOU。
- 保留大小、MIME 和扩展名检查。

## API 暴露边界

- 公开 `/api/v1/openapi.json` 只暴露公开路由。
- 完整路由清单只允许管理员通过 `/api/v1/system/admin/apis` 查看。
- API Key 必须可撤销，并应具备明确权限边界。

## 管理员操作

- 危险操作必须有确认、toast 反馈和审计日志。
- 保留 last-admin 保护。
- 状态变更 handler 成功后应调用审计 helper。
