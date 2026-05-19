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
- **退群完全封禁模式（`Telegram.ban_on_leave`）**：
  - 默认 `false`；开启后定时巡检发现退群用户会被 Bot 永久 `ban_chat_member`（不会自动解封），无法重新加入。
  - 依赖 Bot 在每个 `GROUP_ID` 群里是管理员且具备"封禁成员"权限。
  - 开启时巡检的"重新入群识别"分支会被跳过；如需放行某个被永封 ID，需手动在 TG 群里解封。
  - 误判 = 不可逆，上线前请先以 `require_group_membership=true` + `ban_on_leave=false` 观察 1～2 周巡检日志。

## 3.1 API 速率限制

- 主要登录、注册、TG 绑定、密码/绑定相关接口都已启用滑动窗口限流（见 [BACKEND_API.md §3.1](./BACKEND_API.md#31-速率限制)）。
- 配额维度：IP / UID / 业务 key（如绑定码、request_id）。
- 命中后返回 `HTTP 429` + `retry_after` 提示，并写 `logger.warning`。
- 配额是单进程内存的（`src/core/utils.py::_RATE_BUCKETS`），多 worker 部署下每个进程独立计数；如有进一步加固需求，可改为基于 Redis 的实现。

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

## 8. 配置文件自动备份

- 启动期 `sweep_config_toml` 会自动迁移老 section、补齐缺失默认、删除孤立键，并在写回前备份到 `config_backups/<file>.<timestamp>.<reason>.bak`，权限收紧为 `0o600`。
- 兼容旧逻辑：同时维护一份 `config.toml.backup` 单文件副本便于人工快速恢复。
- 保留份数：默认 20 份，环境变量 `TWILIGHT_CONFIG_BACKUP_RETENTION` 可覆盖（`<=0` 表示不裁剪）。超出按 mtime 从旧到新淘汰。
- 备份目录已加入 `.gitignore`，不要 commit。

## 9. 上线前检查清单

- [ ] 所有默认密钥/示例密钥已替换
- [ ] `config.local.toml` 与 `.env` 未入库
- [ ] CORS 非通配符
- [ ] HTTPS 与安全 Cookie 已启用
- [ ] Bot 内部密钥已配置并验证
- [ ] 关键日志可追溯但不泄密
- [ ] 公开端点的速率限制阈值已按预期流量评估（[BACKEND_API.md §3.1](./BACKEND_API.md#31-速率限制)）
- [ ] `Telegram.ban_on_leave` 评估过：要开就先确认 Bot 在每个群都有封禁权限，并准备好"误封解除"的运维流程
