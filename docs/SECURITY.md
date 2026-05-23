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
- `cors_origins` 只填写 Origin，不填写路径、查询串或片段；例如写 `https://app.example.com`，不要写 `https://app.example.com/admin`。
- 配置中的尾斜杠会被规范化，但非法 scheme、带路径的 Origin 和通配符都不会被当作可信凭据接口来源。
- 只允许你的前端域名，例如：

```toml
[API]
cors_enabled = true
cors_origins = ["https://app.example.com"]
```

- 若通过 HTTPS 对外提供服务，建议同时启用：
  - `session_cookie_secure = true`
  - 合理的 `session_cookie_samesite`（通常 `Lax`）
- 使用 Cookie 会话执行写操作时，前端必须带 `X-Twilight-Client: webui`；后端会拒绝缺少该头的 Cookie 写请求，用于阻断 CSRF 表单提交。

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
- Go 后端在配置 Redis 时使用 Redis 共享限流计数；未配置 Redis 时降级为单进程内存计数。

## 4. 多进程部署一致性

- 生产多进程/多实例建议配置 Redis：
  - 共享会话/Token 状态
  - 共享 API 速率限制计数
- 未配置 Redis 时，会话和限流只在当前 Go 进程内生效。

## 5. 反向代理与暴露面

- 建议用 Nginx/Caddy 暴露单一入口，仅开放 80/443。
- 后端服务（如 5000）尽量仅监听内网或本机。
- 限制管理接口访问来源（网段/IP/WAF）。
- API 与前端默认发送基础安全响应头，包括 `X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy` 与 `Permissions-Policy`；反向代理如覆盖这些头，应保持同等或更严格策略。

## 5.1 前端资源与背景图

- 用户自定义背景只允许安全的图片 URL/路径、`blob:`、常见图片 `data:` URL 与渐变函数；`javascript:`、`file:` 等非预期协议会被前端忽略。
- 如果允许用户使用外部图片作为背景或头像，请优先使用 HTTPS，避免混合内容和第三方 Referer 泄漏。
- Next.js 已关闭 `X-Powered-By` 指纹头；如通过 CDN/反向代理暴露前端，请同步隐藏上游技术栈指纹。

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
- API Key 不能自行修改权限；权限变更必须通过已登录 Web 端完成，避免只读 Key 自提权。
- 管理员账号数量最小化，长期不使用的高权限账号及时停用。
- Telegram 管理员 ID 仅配置必要人员。
- Telegram 管理员私聊仅保留只读查询与统计能力；添加用户、生成注册码、广播、强制绑定、踢出会话等写操作应统一走 Web 后台。

## 7.1 Emby 管理员账号安全隔离

- **核心规则**：如果一个用户绑定的 Emby 账号在 Emby 端具有 `IsAdministrator` 权限，但该用户在 Twilight 系统中不是管理员（`Role != 0`），则该用户的所有敏感操作将被禁止。
- 被禁止的操作包括但不限于：
  - 修改系统密码 / 生成新密码
  - 修改 Emby 密码
  - 修改个人资料（用户名、邮箱等）
  - 解绑 Emby 账号
  - 媒体库自助切换
- 绑定 Emby 账号时，非系统管理员用户不允许绑定 Emby 管理员账号。
- 此机制防止以下攻击场景：用户通过绑定 Emby 管理员账号，利用密码修改等功能间接控制 Emby 服务器。
- 系统管理员（`Role == 0`）不受此限制。
- 如果用户已经绑定了 Emby 管理员账号，需要联系系统管理员处理。

## 7.2 管理员保护

- 系统禁止移除最后一个管理员的权限，防止管理员锁死。
- 降级管理员角色前会检查剩余活跃管理员数量，至少保留一个。

## 7.3 Emby 用户上限

- `Register.emby_user_limit` 使用同一个容量口径：已绑定 Emby 的系统用户、`PENDING_EMBY=True` 的待开通资格、自由注册/卡码队列中正在创建的请求都会占用名额。
- 注册码注册、邀请码开通、用户自助补建、手动绑定、管理员授予开通资格、独立 Emby 账号创建等路径都应在创建或新增绑定前检查容量。
- 删除 Emby 账号或清理待开通资格后会释放对应名额；独立 Emby 账号不写入本地用户表，因此还会额外读取 Emby 端总用户数做兜底。

## 8. 配置文件自动备份

- 启动期 `sweep_config_toml` 会自动迁移老 section、补齐缺失默认、删除孤立键，并在写回前备份到 `config_backups/<file>.<timestamp>.<reason>.bak`，权限收紧为 `0o600`。
- 兼容旧逻辑：同时维护一份 `config.toml.backup` 单文件副本便于人工快速恢复。
- 保留份数：默认 20 份，环境变量 `TWILIGHT_CONFIG_BACKUP_RETENTION` 可覆盖（`<=0` 表示不裁剪）。超出按 mtime 从旧到新淘汰。
- 备份目录已加入 `.gitignore`，不要 commit。

## 9. 数据库与自动更新安全

- 数据库备份、恢复、迁移接口均要求管理员登录；恢复目标会限制在配置的备份目录内，拒绝 `../` 路径穿越。
- 迁移到 PostgreSQL 前先使用管理端预检，确认目标连接成功、快照大小和实体计数符合预期。
- 切换 `database.driver` 后需要重启后端；仅迁移数据不会让当前进程自动切换已打开的 store。
- PostgreSQL 自动建库只在目标库不存在时触发，使用配置中的同一 PostgreSQL 用户连接 `postgres` / `template1` 维护库执行 `CREATE DATABASE`；生产环境建议使用只授予当前实例所需权限的专用用户。
- 如果 PostgreSQL 没有管理员但旧 JSON 状态里已有管理员，Go 后端会临时使用 JSON 状态启动，避免管理员在迁移前被空库锁在登录外；迁移完成并确认后再重启到 PostgreSQL。
- 如果 Go 状态没有 active 管理员但存在旧 `db/users.db`，Go 后端会尝试通过系统 `sqlite3` 只读导入 active 管理员账号。该引导只读取固定 `users.db`，拒绝符号链接，不接受前端传入路径，不全量迁移普通用户。
- Git 自动更新只允许 HTTPS 仓库 URL，不允许 URL 内携带用户名/密码/token。
- 自动更新默认拒绝 dirty worktree；先执行安全预检，再执行拉取。需要本地补丁长期存在时，请先合并或提交，不要依赖强制覆盖。
- 自动更新使用 `git pull --ff-only`，不会做 rebase、merge 或 reset。

## 10. 上线前检查清单

- [ ] 所有默认密钥/示例密钥已替换
- [ ] `config.local.toml` 与 `.env` 未入库
- [ ] CORS 非通配符
- [ ] HTTPS 与安全 Cookie 已启用
- [ ] Bot 内部密钥已配置并验证
- [ ] 关键日志可追溯但不泄密
- [ ] 公开端点的速率限制阈值已按预期流量评估（[BACKEND_API.md §3.1](./BACKEND_API.md#31-速率限制)）
- [ ] `Telegram.ban_on_leave` 评估过：要开就先确认 Bot 在每个群都有封禁权限，并准备好"误封解除"的运维流程
- [ ] 数据库备份/恢复/迁移预检已在测试环境跑过
- [ ] Git 自动更新预检显示 worktree clean，且仓库 URL 不含凭据
