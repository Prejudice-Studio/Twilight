# 安全加固指南

本文档用于生产部署前后的安全检查与日常运维基线。

## 1. 敏感配置与密钥管理

- 不要把真实密钥写入仓库版本历史。
- 推荐做法：
  - 把通用配置放在 `config.toml`。
  - 把真实密钥放在 `config.local.toml`（已被 `.gitignore` 忽略）。
  - 或者使用环境变量（`TWILIGHT_*`）。
- 如果密钥曾经泄露（例如提交到 Git 历史、日志、截图），请立即轮换：
  - Telegram Bot Token
  - Emby API Token / 管理员凭据
  - TMDB/Bangumi Token
  - `Security.bot_internal_secret`

## 2. CORS 与会话安全

- 生产环境不要使用 `cors_origins = ["*"]`。
- 只允许你的前端域名，例如：

```toml
[API]
cors_enabled = true
cors_origins = ["https://app.example.com"]
```

- 若通过 HTTPS 对外提供服务，建议同时启用：
  - `session_cookie_secure = true`
  - 合理的 `session_cookie_samesite`（通常 `Lax`）

## 3. Telegram 相关安全

- 启用 Bot 内部回调时，必须配置强随机的 `Security.bot_internal_secret`。
- Bot 与 API 分离部署时，建议显式配置 `Telegram.bind_confirm_api_url`。
- 开启群组强制校验时，确保 Bot 在目标群有足够权限，避免误判。

## 4. 多进程部署一致性

- 生产多进程建议配置 Redis：
  - 共享会话/Token 状态
  - 共享短时业务状态（如绑定流程）
- 未配置 Redis 时，某些跨进程流程可能退化为“最终一致”。

## 5. 反向代理与暴露面

- 建议用 Nginx/Caddy 暴露单一入口，仅开放 80/443。
- 后端服务（如 5000）尽量仅监听内网或本机。
- 限制管理接口访问来源（网段/IP/WAF）。

## 6. 日志与审计

- 日志中不要打印：
  - Token、密码、密钥原文
  - 完整 Authorization 头
- 建议保留并审计：
  - 管理员关键操作日志
  - 登录失败与封禁日志
  - API Key 调用轨迹

## 7. 最小权限原则

- API Key 仅授予必要 scope。
- 管理员账号数量最小化，长期不使用的高权限账号及时停用。
- Telegram 管理员 ID 仅配置必要人员。

## 8. 上线前检查清单

- [ ] 所有默认密钥/示例密钥已替换
- [ ] `config.local.toml` 与 `.env` 未入库
- [ ] CORS 非通配符
- [ ] HTTPS 与安全 Cookie 已启用
- [ ] Bot 内部密钥已配置并验证
- [ ] 关键日志可追溯但不泄密
