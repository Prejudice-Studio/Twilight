# API 路由索引

本文是 Twilight 后端 `/api/v1` 接口的速查索引，用于快速核对每条路由的请求方法、路径、鉴权级别和所属模块。本文严格依据 `internal/api/routes.go` 中 `a.add(method, path, authLevel, handler)` 的真实注册逐条整理；详细的请求/响应示例见 [后端 API 详参](../reference/backend-api.md)，外部 API Key 接入说明见 [API Key 外部接入](../reference/api-key.md)。

> Emby ActivityLog 同步会配对播放开始/停止事件并幂等写入 `PlaybackRecords`，供 Bangumi 同步等内部流程复用。播放统计页面、统计 API 与导出入口已移除；仪表盘在线人数只显示总数，只统计 Emby `/Sessions` 中带 `NowPlayingItem` 的正在播放会话。

## 鉴权标记

后端在 `internal/api/app.go` 中以 `AuthLevel` 枚举区分四类鉴权来源，路由表里的级别会直接映射为本文表格中的标记。

| 标记 | 源枚举 | 含义 |
| ---- | ------ | ---- |
| Public | `AuthPublic` | 免登录，任何来源均可访问 |
| User | `AuthUser` | 需要有效登录会话（Cookie 或 `Authorization: Bearer <token>`） |
| Admin | `AuthAdmin` | 需要登录会话，且账号 `Role == RoleAdmin` |
| API Key | `AuthAPIKey` | 仅接受外部 API Key 凭据，不接受登录会话 |
| Deprecated | — | 仍保留兼容、但不建议使用的路由（鉴权级别见各行说明） |

鉴权判定逻辑集中在 `authenticate`：`AuthPublic` 直接放行；`AuthAPIKey` 走 `authenticateAPIKey`；其余先 `authenticateUser`，再校验账号 `Active`，最后对 `AuthAdmin` 追加角色校验。

## 规范约定

| 项 | 约定 |
| -- | ---- |
| Base URL | `/api/v1` |
| 响应封装 | 统一 envelope：`{ success, code, error_code, message, data, timestamp }`（见 `internal/api/response.go`，`data`/`error_code` 为空时省略） |
| 会话鉴权 | 登录态会话 Cookie，或 `Authorization: Bearer <token>` |
| API Key 鉴权 | `X-API-Key: <key>`、`Authorization: ApiKey <key>` / `Authorization: Bearer <key>`，或 `?apikey=<key>` 查询串（仅当该 Key 开启 `AllowQuery` 时生效，见 `internal/api/app.go` 的 `authenticateAPIKey`） |
| Cookie 写请求 | 不要求 CSRF 令牌，也不做额外来源校验；有效登录会话、Bearer Token 或 API Key 即为鉴权边界 |
| 管理接口归置 | 业务管理接口归入 `/admin/*`，系统配置/运维类归入 `/system/admin/*` |
| 用户自有资源 | 优先使用 `/users/me/*` |
| 公开资源 | 必须显式标记为 Public，并评估限流与信息泄露风险 |
| 线路接口 | 推荐使用 `GET /system/emby-urls`；`GET /emby/urls` 已弃用 |
| 上传资源 | 仅允许通过 `GET /users/assets/{kind}/{filename}` 受控访问，不公开 `/uploads` 目录 |

V2 基础协议入口使用 `/api/v2`，当前只提供不带秘密的能力协商和 API liveness：

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/system/health` | Public | 仅确认 API 进程可处理请求，不探测数据库或 Emby |
| GET | `/api/v2/system/capabilities` | Public | 返回 V2 版本、兼容版本、公开 feature 和受限上传额度 |
| GET | `/api/v2/system/info` | Public | 返回 SSR 外壳和初始化页所需的安全系统摘要；不返回上游地址、Token 或配置秘密 |
| GET | `/api/v2/admin/health/api` | Admin | 独立检测 API 进程；私有 `no-store`，不检测数据库或 Emby |
| GET | `/api/v2/admin/health/database` | Admin | 独立检测当前数据库连接和状态快照；私有 `no-store` |
| GET | `/api/v2/admin/health/emby` | Admin | 独立从后端连接 Emby 并读取有限服务状态；私有 `no-store`，失败不泄露上游诊断 |
| GET | `/api/v2/admin/stats` | Admin | 返回用户、注册码、Redis 回退、路由和运行时间摘要；私有 `no-store`，不包含播放统计 |
| POST | `/api/v2/auth/login` | Public | 使用 Web 用户名/邮箱和密码创建会话；响应不缓存 |
| POST | `/api/v2/auth/login/apikey` | Public | 使用 API Key 创建会话；响应不缓存 |
| POST | `/api/v2/auth/login/telegram` | Public | 保留 Telegram 直登录兼容入口；当前按策略返回不可用 |
| GET | `/api/v2/auth/me` | User | 返回当前会话的用户摘要；私有 `no-store` |
| POST | `/api/v2/auth/logout` | User | 注销当前会话 |
| POST | `/api/v2/auth/logout/all` | User | 注销当前用户的全部会话 |
| POST | `/api/v2/auth/refresh` | User | 轮换当前会话 |
| POST | `/api/v2/auth/password/emby` | Public | 通过 Emby 凭据找回 Web 密码；错误响应不暴露上游细节 |
| POST | `/api/v2/auth/password/email/request` | Public | 发送邮箱找回验证码；统一响应防止账号枚举 |
| POST | `/api/v2/auth/password/email/reset` | Public | 校验邮箱验证码并重置 Web 密码 |
| POST | `/api/v2/registration` | Public | 创建 Web 账号；后端最终校验密码、注册码和 Telegram 绑定码 |
| GET | `/api/v2/registration/availability` | Public | 返回注册开关、容量和用户名可用性摘要 |
| POST | `/api/v2/registration/telegram/bind-code` | Public | 创建注册阶段 Telegram 绑定码；需要 WebUI intent 头 |
| GET | `/api/v2/dashboard/summary` | User | 聚合当前用户、公开能力和在线人数摘要；Emby 失败时通过 `viewers.available=false` 独立降级 |
| GET | `/api/v2/settings` | User | 当前用户设置、Telegram/Emby 状态和密码安全策略；私有 `no-store` |
| PUT | `/api/v2/settings/preferences` | User | 更新通知、自动续期和密码安全偏好；仅接受严格 JSON 布尔值 |
| GET | `/api/v2/settings/appearance` | User | 一次返回当前账号头像与背景配置；私有 `no-store` |
| PUT | `/api/v2/settings/appearance/background` | User | 更新安全渐变、上传背景资源引用及显示参数；最终校验由 Go 完成 |
| DELETE | `/api/v2/settings/appearance/background` | User | 清除当前账号背景配置 |
| POST | `/api/v2/settings/appearance/background/upload` | User | 上传并应用浅色或深色背景图片；使用 multipart 的 `file` 与 `type` 字段 |
| POST | `/api/v2/settings/appearance/avatar/upload` | User | 上传并应用当前账号头像；使用 multipart 的 `file` 字段 |
| DELETE | `/api/v2/settings/appearance/avatar` | User | 清除当前账号头像 |
| POST | `/api/v2/settings/email/send-code` | User | 发送邮箱绑定或密码操作验证码；邮箱和用途由后端校验 |
| POST | `/api/v2/settings/email/verify` | User | 校验邮箱绑定验证码并完成当前账号邮箱验证 |
| POST | `/api/v2/settings/password/system` | User | 修改 Web 密码；后端校验旧密码、强度、邮箱验证码和会话轮换 |
| POST | `/api/v2/settings/password/emby` | User | 修改当前绑定的 Emby 密码；后端执行单一身份凭据和远端更新策略 |
| POST | `/api/v2/settings/emby/bind` | User | 使用现有 Emby 凭据绑定当前账号 |
| POST | `/api/v2/settings/emby/register` | User | 按资格创建并绑定 Emby 账号 |
| POST | `/api/v2/settings/emby/unbind` | User | 按后端资格解除当前账号的 Emby 绑定 |
| GET | `/api/v2/settings/apikeys` | User | 返回当前账号的掩码 API Key 列表；私有 `no-store` |
| POST | `/api/v2/settings/apikeys` | User | 创建 API Key；明文仅在当前响应中返回一次 |
| PUT | `/api/v2/settings/apikeys/{key_id}` | User | 更新当前账号指定 API Key 的名称、启用和限速设置 |
| DELETE | `/api/v2/settings/apikeys/{key_id}` | User | 删除当前账号指定 API Key |
| GET | `/api/v2/announcements` | User | 聚合当前账号可见公告与未确认的强制阅读公告；私有 `no-store` 响应 |
| POST | `/api/v2/announcements/ack` | User | 去重确认当前账号的强制阅读公告；最终归属与状态由后端复核 |
| GET | `/api/v2/signin/summary` | User | 聚合签到摘要、公开奖励规则和最近 30 条记录；私有 `no-store` 响应 |
| POST | `/api/v2/signin` | User | V2 签到动作；复用签到开关、幂等日期判断、积分记录和审计 |
| POST | `/api/v2/signin/renew` | User | V2 积分续期动作；复用 Emby 绑定、余额和 Store 原子扣分校验 |
| PUT | `/api/v2/signin/preferences` | User | V2 用户自动续期开关；只接受严格布尔值并复用资格、审计规则 |
| GET | `/api/v2/invite/summary` | User | 聚合邀请配置、当前关系、直属下级、邀请树和本人邀请码；私有 `no-store` 响应 |
| POST | `/api/v2/invite/codes` | User | V2 创建邀请码；复用邀请开关、限额、目标用户和审计规则 |
| POST | `/api/v2/invite/renew-codes` | User | V2 创建直属下级续期码；复用真实 Emby、有效期和原子存储校验 |
| DELETE | `/api/v2/invite/codes/{code}` | User | V2 删除本人邀请码；删除邀请码不解除已建立的邀请关系 |
| POST | `/api/v2/invite/children/{uid}/detach-expired` | User | V2 删除符合条件的直属下级 Emby 并断开关系；邀请关闭时仍可维护历史关系 |
| POST | `/api/v2/invite/me/detach-expired` | User | V2 当前用户主动清理符合条件的上级关系及 Emby |
| GET | `/api/v2/bangumi/summary` | User | 聚合 Bangumi 同步状态、公开账号资料、五类收藏数量与最近条目；私有 `no-store` 响应，Token 不出现在响应 |
| POST | `/api/v2/bangumi/sync` | User | V2 手动触发 Bangumi 同步；复用功能开关、Token、外部同步和审计规则 |
| DELETE | `/api/v2/bangumi/sync/history` | User | V2 清除当前用户的 Bangumi 同步日志 |
| PUT | `/api/v2/bangumi/preferences` | User | V2 更新 Bangumi Token/同步与管理模式；后端重新校验功能开关和 Token |
| GET | `/api/v2/bangumi/collections` | User | V2 服务端分页读取单类收藏；外部读取和本地缓存由后端控制 |
| PATCH | `/api/v2/bangumi/collections/{subject_id}` | User | V2 更新单条收藏状态、评分或进度 |
| GET | `/api/v2/admin/bangumi/users` | Admin | V2 管理员 Bangumi 用户分页资源；仅返回当前页状态与有限计数 |
| GET | `/api/v2/admin/bangumi/users/{uid}/records` | Admin | V2 按 UID 按需读取有界播放记录详情 |
| POST | `/api/v2/admin/bangumi/users/{uid}/sync` | Admin | V2 为指定用户手动触发 Bangumi 同步 |
| GET | `/api/v2/admin/bangumi/users/{uid}/logs` | Admin | V2 按 UID 按需读取有界同步日志 |
| DELETE | `/api/v2/admin/bangumi/users/{uid}/logs` | Admin | V2 清除指定用户同步日志 |
| GET | `/api/v2/admin/announcements` | Admin | V2 公告分页资源；服务端筛选隐藏/过期状态，不缓存 |
| POST | `/api/v2/admin/announcements` | Admin | V2 创建公告；复用字段白名单、渲染模式归一化和审计 |
| PUT | `/api/v2/admin/announcements/{announcement_id}` | Admin | V2 更新公告 |
| DELETE | `/api/v2/admin/announcements/{announcement_id}` | Admin | V2 删除公告 |
| GET | `/api/v2/admin/audit-logs` | Admin | V2 操作审计日志分页资源；服务端筛选、参数化排序且不缓存 |
| DELETE | `/api/v2/admin/audit-logs/{log_id}` | Admin | V2 删除单条审计日志；兼容 `log_id` 路由参数 |
| POST | `/api/v2/admin/audit-logs/clear` | Admin | V2 清空审计日志；需要 `CLEAR_AUDIT_LOGS` |
| POST | `/api/v2/admin/audit-logs/prune` | Admin | V2 按条数/天数裁剪审计日志；需要 `PRUNE_AUDIT_LOGS` |
| GET | `/api/v2/admin/config/schema` | Admin | V2 读取脱敏结构化配置 schema；不返回服务器路径或 secret 明文 |
| PUT | `/api/v2/admin/config/schema` | Admin | V2 保存结构化配置；复用字段白名单、secret 哨兵、备份、热重载和审计 |
| GET | `/api/v2/admin/config/toml` | Admin | V2 读取脱敏 TOML；不返回服务器路径或 secret 明文 |
| PUT | `/api/v2/admin/config/toml` | Admin | V2 保存 TOML；复用解析、受保护字段、备份、热重载和失败回滚 |
| GET | `/api/v2/admin/config/backups` | Admin | V2 读取配置备份元数据；不返回备份路径 |
| POST | `/api/v2/admin/config/backup` | Admin | V2 创建配置备份 |
| GET | `/api/v2/admin/config/backups/{name}` | Admin | V2 读取脱敏配置备份预览；文件名由后端安全校验 |
| DELETE | `/api/v2/admin/config/backups/{name}` | Admin | V2 删除配置备份 |
| POST | `/api/v2/admin/config/restore` | Admin | V2 生成恢复预览或确认恢复；执行恢复需要 `RESTORE_CONFIG_BACKUP` |
| POST | `/api/v2/admin/config/sweep` | Admin | V2 整理配置并热重载 |
| POST | `/api/v2/admin/config/upload-auth-background` | Admin | V2 上传认证页背景图；复用 MIME、大小、路径和限流校验 |
| GET | `/api/v2/admin/database/status` | Admin | V2 数据库状态安全摘要；不返回状态文件、备份目录或数据库拓扑 |
| GET | `/api/v2/admin/database/backups` | Admin | V2 数据库备份元数据；不返回服务器路径 |
| GET | `/api/v2/admin/database/backups/{name}` | Admin | V2 读取数据库备份摘要；文件名由后端路径安全校验 |
| DELETE | `/api/v2/admin/database/backups/{name}` | Admin | V2 删除数据库备份 |
| POST | `/api/v2/admin/database/backup` | Admin | V2 创建数据库备份 |
| POST | `/api/v2/admin/database/restore` | Admin | V2 生成恢复预览或确认恢复；执行恢复需要 `RESTORE_DATABASE_BACKUP` |
| POST | `/api/v2/admin/database/migrate` | Admin | V2 生成数据库迁移预览或执行迁移；执行需要 `MIGRATE_DATABASE`，继续受功能开关与路径安全约束 |
| GET | `/api/v2/admin/runtime/status` | Admin | V2 读取有限运行时状态；私有 `no-store`，不返回配置秘密或文件路径 |
| GET | `/api/v2/admin/runtime/logs` | Admin | V2 读取有界运行日志快照；私有 `no-store`，不启用浏览器流式连接 |
| GET | `/api/v2/admin/scheduler/jobs` | Admin | V2 读取调度任务摘要；服务端批量读取运行状态，私有 `no-store` |
| POST | `/api/v2/admin/scheduler/jobs/{job_id}/run` | Admin | V2 手动启动指定任务；参数由服务端按任务白名单归一化 |
| POST | `/api/v2/admin/scheduler/jobs/{job_id}/terminate` | Admin | V2 请求终止指定运行中的任务 |
| GET | `/api/v2/admin/scheduler/jobs/{job_id}/last-run` | Admin | V2 按需读取指定任务最近一次运行结果 |
| GET | `/api/v2/admin/scheduler/jobs/{job_id}/history` | Admin | V2 按需读取指定任务最多 20 条运行历史 |
| PUT | `/api/v2/admin/scheduler/jobs/{job_id}/schedule` | Admin | V2 保存调度计划和受限运行参数 |
| DELETE | `/api/v2/admin/scheduler/jobs/{job_id}/schedule` | Admin | V2 恢复指定任务默认计划 |

V2 业务模块迁移采用兼容 adapter；未列入 V2 的接口仍使用 `/api/v1`，不能由前端自行拼接版本路径。

配置管理的 V2 资源位于 `/api/v2/admin/config/*`。schema、TOML 和备份读取均为管理员私有 `no-store` 资源；响应不包含服务器文件系统路径，敏感配置保持脱敏哨兵。保存、备份、整理、上传认证背景图和恢复操作均通过 V2 资源提交，恢复仍要求 `RESTORE_CONFIG_BACKUP` 确认短语。

> 说明：`X-Twilight-Client` 只用于前端请求识别与 CORS 允许头，不参与鉴权。少数有副作用的 `GET`（如绑定码创建）还要求 `X-Twilight-Intent` 显式声明操作意图，用于拦截预取/探测误触发。

## 根与文档

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/` | Public | 根路由 |
| GET | `/api/v1/openapi.json` | Public | OpenAPI 规范文档 |
| GET | `/api/v1/docs` | Public | 在线 API 控制台；未登录只显示公开 OpenAPI，管理员登录后显示完整路由清单 |

## Auth

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/api/v1/auth/login` | Public | 用户名密码登录 |
| POST | `/api/v1/auth/forgot-password/emby` | Public | 通过 Emby 账号密码验证后重置 Web 登录密码 |
| POST | `/api/v1/auth/password/email/request` | Public | 邮箱找回密码第一步：向已验证邮箱发送验证码（统一成功响应防枚举） |
| POST | `/api/v1/auth/password/email/reset` | Public | 邮箱找回密码第二步：校验验证码并重置登录密码 |
| POST | `/api/v1/auth/login/telegram` | Public | Telegram 直登入口（当前由 `handleDirectLoginUnavailable` 返回不可用） |
| POST | `/api/v1/auth/login/apikey` | Public | 用 API Key 换取登录会话 |
| POST | `/api/v1/auth/logout` | User | 注销当前会话 |
| POST | `/api/v1/auth/logout/all` | User | 注销该用户全部会话 |
| GET | `/api/v1/auth/me` | User | 当前登录用户资料 |
| POST | `/api/v1/auth/refresh` | User | 刷新会话 |
| GET | `/api/v1/auth/apikey` | User | 查看旧版（账号级）API Key 状态 |
| POST | `/api/v1/auth/apikey` | User | 生成或刷新旧版 API Key |
| DELETE | `/api/v1/auth/apikey` | User | 删除旧版 API Key |
| POST | `/api/v1/auth/apikey/enable` | User | 启用旧版 API Key |
| GET | `/api/v1/auth/apikey/permissions` | User | 查看旧版 API Key 权限 |
| PUT | `/api/v1/auth/apikey/permissions` | User | 更新旧版 API Key 权限 |

## Users

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/api/v1/users/register` | Public | 注册系统账号 |
| GET | `/api/v1/users/check-available` | Public | 注册可用性检查（用户名等） |
| GET | `/api/v1/users/regcode/check` | Public | 预检注册码/续期码/卡码 |
| GET | `/api/v1/users/telegram/register/bind-code` | Public | 生成注册用 Telegram 绑定码（需 `X-Twilight-Intent: create-bind-code`） |
| GET | `/api/v1/users/telegram/register/bind-code/status` | Public | 查询注册用 Telegram 绑定码状态 |
| GET | `/api/v1/users/telegram/register/bind-code/ws` | Public | WebSocket 订阅注册用 Telegram 绑定码状态 |
| POST | `/api/v1/users/me/telegram/bind-confirm` | Public | 确认 Telegram 绑定（安全确认流程） |
| GET | `/api/v1/users/register/emby/status` | Public | 查询 Emby 注册队列状态 |
| GET | `/api/v1/users/me` | User | 当前用户资料 |
| PUT | `/api/v1/users/me` | User | 更新当前用户资料 |
| PUT | `/api/v1/users/me/username` | User | 修改用户名 |
| PUT | `/api/v1/users/me/password` | User | 修改密码（兼容旧入口） |
| POST | `/api/v1/users/me/password/change` | User | 修改登录密码（兼容旧入口） |
| POST | `/api/v1/users/me/password/system` | User | 修改系统登录密码 |
| POST | `/api/v1/users/me/password/emby` | User | 修改 Emby 密码 |
| POST | `/api/v1/users/me/email/send-code` | User | 发送邮箱验证码（用途 bind / change_password / change_emby_password） |
| POST | `/api/v1/users/me/email/verify` | User | 校验 bind 验证码并完成邮箱绑定 + 标记已验证 |
| POST | `/api/v1/users/me/emby/bind` | User | 绑定已有 Emby 账号 |
| POST | `/api/v1/users/me/emby/register` | User | 登录后补建 Emby 账号（PENDING_EMBY 流程） |
| POST | `/api/v1/users/me/emby/unbind` | User | 先禁用远端 Emby；成功后清理本地绑定，禁用失败则保留本地绑定 |
| POST | `/api/v1/users/me/renew` | User | 使用续期码续期（需已绑定 Emby） |
| POST | `/api/v1/users/me/use-code` | User | 统一预检/使用注册码、续期码、白名单码、邀请码 |
| GET | `/api/v1/users/me/use-code/status` | User | 查询 use-code 异步队列状态 |
| GET | `/api/v1/users/me/devices` | User | 当前用户设备列表 |
| DELETE | `/api/v1/users/me/devices/{device_id}` | User | 删除指定设备 |
| GET | `/api/v1/users/me/sessions` | User | 当前用户播放会话 |
| GET | `/api/v1/users/me/login-history` | User | 当前用户登录历史 |
| GET | `/api/v1/users/me/telegram` | User | Telegram 绑定状态 |
| POST | `/api/v1/users/me/telegram/rebind-request` | User | 提交 Telegram 换绑申请 |
| POST | `/api/v1/users/me/telegram/unbind` | User | 解绑 Telegram |
| GET | `/api/v1/users/me/telegram/bind-code` | User | 生成登录用户的 Telegram 绑定码（需 `X-Twilight-Intent: create-bind-code`） |
| GET | `/api/v1/users/me/settings` | User | 当前用户设置聚合 |
| GET | `/api/v1/users/{uid}/background` | User | 获取指定用户背景（本人或管理员） |
| PUT | `/api/v1/users/me/background` | User | 更新背景配置 |
| DELETE | `/api/v1/users/me/background` | User | 删除背景配置 |
| POST | `/api/v1/users/me/background/upload` | User | 上传背景图 |
| GET | `/api/v1/users/{uid}/avatar` | User | 获取指定用户头像（本人或管理员） |
| POST | `/api/v1/users/me/avatar/upload` | User | 上传头像 |
| DELETE | `/api/v1/users/me/avatar` | User | 删除头像 |
| GET | `/api/v1/users/assets/{kind}/{filename}` | User | 受控访问头像/背景上传资源 |
| GET | `/api/v1/users/me/apikeys` | User | 当前用户 API Key 列表 |
| POST | `/api/v1/users/me/apikeys` | User | 创建当前用户 API Key |
| PUT | `/api/v1/users/me/apikeys/{key_id}` | User | 更新当前用户 API Key |
| DELETE | `/api/v1/users/me/apikeys/{key_id}` | User | 删除当前用户 API Key |

## Setup

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/setup/status` | Public | 查询初始化向导是否可用 |
| POST | `/api/v1/setup/complete` | Public | 一次性完成初始化，需空系统硬门控与 WebUI intent 头 |

## System

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/system/info` | Public | 系统公开信息 |
| GET | `/api/v1/system/server-icon` | Public | 服务器图标 |
| GET | `/api/v1/system/health` | Public | 轻量 API 存活摘要（不探测数据库或 Emby） |
| GET | `/api/v1/system/health/api` | Admin | API 服务健康检查（状态页单项检测） |
| GET | `/api/v1/system/health/database` | Admin | 数据库健康检查（状态页单项检测） |
| GET | `/api/v1/system/health/emby` | Admin | Emby 健康检查（状态页单项检测） |
| GET | `/api/v1/system/stats` | Admin | 系统运行时统计 |
| GET | `/api/v1/system/emby-urls` | User | 按权限下发 Emby 线路 |
| POST | `/api/v1/system/emby-urls/probe` | User | 探测 Emby 线路连通性 |
| GET | `/api/v1/system/emby-stats` | User | 按配置读取 Emby 媒体库数量 |
| GET | `/api/v1/system/emby-viewers` | User | 当前正在播放人数（仅总数） |
| GET | `/api/v1/system/config` | User | 用户可见配置 |
| GET | `/api/v1/system/admin/config` | Admin | 管理员完整配置 |
| GET | `/api/v1/system/admin/stats` | Admin | 管理统计 |
| GET | `/api/v1/system/admin/runtime/status` | Admin | Go 进程、主机、数据库与内存状态 |
| GET | `/api/v1/system/admin/runtime/logs` | Admin | 读取后端内存日志快照 |
| GET | `/api/v1/system/admin/runtime/logs/stream` | Admin | SSE 实时后端日志流 |
| POST | `/api/v1/system/admin/update` | Admin | Git 自动更新与可选 systemd 重启调度 |
| POST | `/api/v1/system/admin/server-icon/upload` | Admin | 上传服务器图标 |
| GET | `/api/v1/system/admin/database/status` | Admin | 当前数据库状态 |
| GET | `/api/v1/system/admin/database/backups` | Admin | 数据库备份列表 |
| GET | `/api/v1/system/admin/database/backups/{name}` | Admin | 查看指定数据库备份详情 |
| DELETE | `/api/v1/system/admin/database/backups/{name}` | Admin | 删除指定数据库备份 |
| POST | `/api/v1/system/admin/database/backup` | Admin | 创建数据库备份 |
| POST | `/api/v1/system/admin/database/restore` | Admin | 从受控备份恢复数据库 |
| POST | `/api/v1/system/admin/database/migrate` | Admin | 数据库迁移预检/执行 |
| GET | `/api/v1/system/admin/migration/status` | Admin | Twilight 迁移包能力、容量和资源命名空间 |
| POST | `/api/v1/system/admin/migration/export` | Admin | 导出 Twilight 数据、配置和受控静态资源 ZIP |
| POST | `/api/v1/system/admin/migration/import` | Admin | multipart 迁移包预览/确认导入 |
| GET | `/api/v1/system/admin/config/toml` | Admin | 读取 TOML 配置 |
| PUT | `/api/v1/system/admin/config/toml` | Admin | 保存 TOML 配置（安全校验版） |
| GET | `/api/v1/system/admin/config/schema` | Admin | 配置表单 schema |
| PUT | `/api/v1/system/admin/config/schema` | Admin | 保存配置表单（安全校验版） |
| GET | `/api/v1/system/admin/config/backups` | Admin | 配置备份列表 |
| POST | `/api/v1/system/admin/config/backup` | Admin | 创建配置备份 |
| GET | `/api/v1/system/admin/config/backups/{name}` | Admin | 查看指定配置备份详情 |
| DELETE | `/api/v1/system/admin/config/backups/{name}` | Admin | 删除指定配置备份 |
| POST | `/api/v1/system/admin/config/restore` | Admin | 从备份恢复配置 |
| POST | `/api/v1/system/admin/config/sweep` | Admin | 手动整理配置文件（迁移历史段、删孤立键、补默认值） |
| GET | `/api/v1/system/admin/apis` | Admin | 当前路由列表 |
| POST | `/api/v1/system/admin/bot/test` | Admin | Telegram Bot 连通性测试 |

> 配置管理页面继续保留迁移模块的兼容入口，但邮箱、Telegram、邀请、安全配置推荐分别在「邮箱管理」「Telegram 管理」「邀请系统管理」「安全中心」维护。`/system/admin/config/toml` 与 `/system/admin/config/schema` 返回的敏感字段会脱敏；未修改的 secret 以服务端哨兵值保留，不回显明文。

## Emby

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/emby/status` | User | Emby 服务器状态 |
| GET | `/api/v1/emby/urls` | Public（Deprecated） | 已弃用，改用 `/system/emby-urls` |
| GET | `/api/v1/emby/search` | User | Emby 媒体搜索 |
| GET | `/api/v1/emby/latest` | User | 最新媒体 |
| GET | `/api/v1/emby/sessions/count` | User | 当前会话数量 |
| POST | `/api/v1/emby/bangumi/webhook` | Public | Bangumi Webhook 回调入口（按时间戳/签名校验，见 `internal/api/bangumi_webhook.go`） |

## Bangumi Sync

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/bangumi/sync/status` | User | 获取当前用户的 Bangumi 同步状态与最近日志 |
| POST | `/api/v1/bangumi/sync/trigger` | User | 手动触发一次 Bangumi 同步 |
| GET | `/api/v1/bangumi/sync/history` | User | 获取同步历史日志（`?limit=`） |
| DELETE | `/api/v1/bangumi/sync/history` | User | 清除当前用户的同步历史 |
| GET | `/api/v1/bangumi/me` | User | 获取 Bangumi 用户资料与收藏精选（管理开关控制） |
| GET | `/api/v1/bangumi/collections` | User | 分页获取收藏列表（`?type=&limit=&offset=&refresh=1`，可返回缓存） |
| PATCH | `/api/v1/bangumi/collections/{subject_id}` | User | 修改收藏状态、进度与评分，并失效当前用户收藏缓存 |

## Media

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/media/search` | User | TMDB/Bangumi 并行聚合搜索；全部来源交错合并并返回部分失败 warnings |
| GET | `/api/v1/media/search/tmdb` | User | TMDB 搜索 |
| GET | `/api/v1/media/search/bangumi` | User | Bangumi 搜索 |
| GET | `/api/v1/media/search/id/{source_type}/{media_id}` | User | 按源 ID 查询海报与完整媒体详情；TMDB/Bangumi 返回来源可用的扩展元数据 |
| GET | `/api/v1/media/detail` | User | 媒体详情、海报、主创/制作/演员、评分和来源扩展字段 |
| GET | `/api/v1/media/tmdb/{tmdb_id}` | User | TMDB 详情 |
| GET | `/api/v1/media/bangumi/{bgm_id}` | User | Bangumi 详情 |
| POST | `/api/v1/media/inventory/check` | User | 检查库存 |
| GET | `/api/v1/media/inventory/search` | User | 搜索库存 |
| POST | `/api/v1/media/request` | User | 提交求片 |
| GET | `/api/v1/media/request/my` | User | 我的求片 |
| GET | `/api/v1/media/request/pending` | Admin | 待处理求片列表 |
| PUT | `/api/v1/media/request/{request_id}/status` | Admin | 更新求片状态（须显式传 `status`） |
| POST | `/api/v1/media/request/external/update` | Public | 外部回调更新求片（依赖内部密钥校验，须显式传 `status`） |
| GET | `/api/v1/media/request/by-key/{require_key}` | User | 按 key 查询求片 |
| DELETE | `/api/v1/media/request/by-key/{require_key}` | User | 按 key 删除求片 |
| GET | `/api/v1/media/request/{request_id}` | User | 求片详情 |
| DELETE | `/api/v1/media/request/{request_id}` | User | 删除求片 |

> 注：`/media/request/external/update` 路由本身注册为 Public，真正的访问控制来自请求体/请求头携带的内部密钥（`X-Internal-Secret` 或 `Authorization: Bearer`，见 `internal/api/media_request_handlers.go`），并非登录会话。

V2 用户端媒体资源（默认 SSR 前端使用）：

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v2/media/search` | User | 受限 TMDB/Bangumi 聚合搜索；返回 `items`、`total` 和来源降级提示，不缓存 |
| GET | `/api/v2/media/search/{source}` | User | 按路径来源搜索，`source` 为 `tmdb` 或 `bangumi` |
| GET | `/api/v2/media/detail` | User | 返回 `{item}` 包装的媒体详情资源 |
| POST | `/api/v2/media/inventory/check` | User | 检查 Emby 库存；响应不缓存，外部错误不会把内部地址返回给浏览器 |
| GET | `/api/v2/media/requests` | User | 当前用户的求片资源集合，返回 `items` 和 `total` |
| POST | `/api/v2/media/requests` | User | 创建求片；继续复用后端邮箱、Telegram、库存、队列上限、去重和审计规则 |
| GET | `/api/v2/media/requests/by-key/{require_key}` | User | 按业务 key 读取本人或管理员可访问的求片 |
| DELETE | `/api/v2/media/requests/by-key/{require_key}` | User | 删除本人可访问的求片 |

## Tickets

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/tickets` | User | 当前用户工单摘要分页；`page` / `per_page` 上限分别为 1000000 / 100，回复正文、工单正文和附件 URL 需通过单条详情读取 |
| POST | `/api/v1/tickets` | User | 创建工单 |
| GET | `/api/v1/tickets/{ticket_id}` | User | 读取本人单条工单及完整双方回复、附件；非本人统一返回工单不存在 |
| POST | `/api/v1/tickets/{ticket_id}/reply` | User | 追加工单回复；用户回复已解决工单会重新进入待处理 |
| POST | `/api/v1/tickets/{ticket_id}/close` | User | 用户关闭自己的工单 |
| POST | `/api/v1/tickets/{ticket_id}/reopen` | User | 用户重开已关闭工单 |
| PUT | `/api/v1/tickets/{ticket_id}/notify-telegram` | User | 切换单工单 Telegram 通知 |
| POST | `/api/v1/tickets/{ticket_id}/images` | User | 上传工单交流图片 |
| GET | `/api/v1/tickets/{ticket_id}/images/{filename}` | User | 读取工单图片 |
| DELETE | `/api/v1/tickets/{ticket_id}/images/{filename}` | User | 删除工单图片；关闭后普通用户不可删除 |
| GET | `/api/v2/tickets` | User | V2 当前用户工单摘要分页；返回 `items` 与 `pagination`，不返回正文、回复或附件 URL |
| POST | `/api/v2/tickets` | User | V2 创建工单，复用服务端类型、配额、通知和审计规则 |
| GET | `/api/v2/tickets/{ticket_id}` | User | V2 读取本人单条工单及完整双方回复、附件；非本人统一返回工单不存在 |
| POST | `/api/v2/tickets/{ticket_id}/replies` | User | V2 追加工单回复，不覆盖已有对话 |
| POST | `/api/v2/tickets/{ticket_id}/close` | User | V2 用户关闭自己的工单 |
| POST | `/api/v2/tickets/{ticket_id}/reopen` | User | V2 用户重开已关闭工单 |
| PUT | `/api/v2/tickets/{ticket_id}/notify-telegram` | User | V2 切换单工单 Telegram 通知 |
| POST | `/api/v2/tickets/{ticket_id}/attachments` | User | V2 上传工单交流图片，复用全局大小/数量/真实类型限制 |
| GET | `/api/v2/tickets/{ticket_id}/attachments/{filename}` | User | V2 读取本人已登记的工单附件 |
| DELETE | `/api/v2/tickets/{ticket_id}/attachments/{filename}` | User | V2 删除工单附件；关闭后普通用户不可删除 |

## Admin

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/admin/users` | Admin | 用户列表 |
| POST | `/api/v1/admin/developer-mode/activate` | Admin | 仪表盘 `DEBUGMODE` 二次验证后切换全局开发者模式 |
| POST | `/api/v1/admin/developer/js-sandbox` | Admin | 在受控沙箱中预检/执行 Telegram JS 自定义指令片段 |
| GET | `/api/v1/admin/developer/js-docs` | Admin | 获取 Telegram JS 沙箱引擎、内置对象、函数参数表、返回说明、示例和限制说明 |
| GET | `/api/v1/admin/developer/js-presets` | Admin | 列出开发者模式 JS 预设 |
| POST | `/api/v1/admin/developer/js-presets` | Admin | 创建开发者模式 JS 预设；允许空白代码草稿 |
| PUT | `/api/v1/admin/developer/js-presets/{preset_id}` | Admin | 更新开发者模式 JS 预设 |
| DELETE | `/api/v1/admin/developer/js-presets/{preset_id}` | Admin | 删除开发者模式 JS 预设 |
| PUT | `/api/v1/admin/me/update` | Admin | 更新管理员自身信息 |
| GET | `/api/v1/admin/users/{uid}` | Admin | 用户详情 |
| PUT | `/api/v1/admin/users/{uid}` | Admin | 更新用户 |
| DELETE | `/api/v1/admin/users/{uid}` | Admin | 删除用户 |
| POST | `/api/v1/admin/users/{uid}/delete` | Admin | 删除用户（推荐；支持 JSON body 的 `mode` 与 `cascade_depth`） |
| POST | `/api/v1/admin/users/{uid}/disable` | Admin | 禁用用户 |
| POST | `/api/v1/admin/users/{uid}/enable` | Admin | 启用用户 |
| DELETE | `/api/v1/admin/users/{uid}/emby` | Admin | 删除用户的 Emby 账号 |
| POST | `/api/v1/admin/users/{uid}/force-unbind` | Admin | 强制解除本地绑定 |
| POST | `/api/v1/admin/users/{uid}/registration-queue/clear` | Admin | 清空指定用户的注册队列 |
| POST | `/api/v1/admin/users/registration-queue/clear` | Admin | 清空注册队列 |
| POST | `/api/v1/admin/users/registration-queue/grant-entitlement-and-clear` | Admin | 批量授予资格并清空注册队列 |
| POST | `/api/v1/admin/users/{uid}/registration-entitlement` | Admin | 授予指定用户注册资格 |
| POST | `/api/v1/admin/users/{uid}/registration-entitlement/dequeue` | Admin | 授予资格并出队 |
| POST | `/api/v1/admin/users/sync-bindings` | Admin | 同步绑定状态 |
| POST | `/api/v1/admin/users/{uid}/renew` | Admin | 管理员为用户续期 |
| POST | `/api/v1/admin/users/{uid}/cancel-permanent` | Admin | 取消永久有效（与续期同 handler） |
| POST | `/api/v1/admin/users/{uid}/reset-password` | Admin | 重置用户密码 |
| POST | `/api/v1/admin/users/{uid}/kick` | Admin | 将用户踢下线 |
| PUT | `/api/v1/admin/users/{uid}/admin` | Admin | 设置/取消管理员角色 |
| POST | `/api/v1/admin/users/{uid}/unbind-telegram` | Admin | 解绑用户 Telegram |
| POST | `/api/v1/admin/users/{uid}/bind-telegram` | Admin | 强制为用户绑定 Telegram |
| GET | `/api/v1/admin/users/by-telegram/{telegram_id}` | Admin | 按 Telegram ID 查用户 |
| POST | `/api/v1/admin/emby/force-set-password` | Admin | 强制设置 Emby 密码（与重置密码同 handler） |
| POST | `/api/v1/admin/emby/sync` | Admin | 同步 Emby 用户 |
| GET | `/api/v1/admin/emby/sessions` | Admin | Emby 实时会话（含 `remote_endpoint` IP） |
| GET | `/api/v1/admin/emby/now-playing` | Admin | 仪表盘管理员观看摘要（用户、媒体和进度） |
| GET | `/api/v1/admin/emby/device-audit` | Admin | 手动读取 Emby 登录用户设备/IP 审查（按用户聚合）；支持 `page`、`per_page`、`search`，过滤 Twilight 自身客户端，离线设备按设备名/客户端/版本聚合并返回 count，单个来源失败时降级返回其余数据，失败字段为通用文案 |
| GET | `/api/v1/admin/emby/activity-logs` | Admin | 本地 Emby 活动日志；`?refresh=1` 手动从 Emby 拉取并入库 |
| GET | `/api/v1/admin/emby/activity` | Admin | Emby 活动记录 |
| GET | `/api/v1/admin/emby/users` | Admin | Emby 用户列表；支持 `page`、`per_page`、`search`、`link`、`attribute`，失效本地绑定使用独立分页 |
| POST | `/api/v1/admin/emby/broadcast` | Admin | Emby 广播消息 |
| POST | `/api/v1/admin/emby/test` | Admin | 后端测试 Emby 连接、用户列表、媒体库列表，并尝试本机 Emby 候选地址 |
| POST | `/api/v1/admin/emby/cleanup-orphans` | Admin | 清理孤儿绑定 |
| POST | `/api/v1/admin/emby/import-users` | Admin | 导入 Emby 用户 |
| POST | `/api/v1/admin/emby/reset-bindings` | Admin | 重置 Emby 绑定 |
| POST | `/api/v1/admin/emby/delete-unlinked` | Admin | 删除未绑定的 Emby 用户 |
| POST | `/api/v1/admin/emby/create-standalone` | Admin | 创建独立 Emby 用户（不写本地 users 表） |
| POST | `/api/v1/admin/users/{uid}/bind-emby` | Admin | 为用户绑定/强绑 Emby（冲突走 200+success=false 携带 conflict 详情） |
| GET | `/api/v1/admin/regcodes` | Admin | 注册码列表 |
| POST | `/api/v1/admin/regcodes` | Admin | 创建注册码 |
| POST | `/api/v1/admin/regcodes/batch-delete` | Admin | 批量删除注册码 |
| PUT | `/api/v1/admin/regcodes/{code}` | Admin | 更新注册码 |
| DELETE | `/api/v1/admin/regcodes/{code}` | Admin | 删除注册码 |
| GET | `/api/v1/admin/regcodes/{code}/users` | Admin | 查看注册码使用者 |
| POST | `/api/v1/admin/regcodes/{code}/clear-usage` | Admin | 清理注册码使用记录 |
| GET | `/api/v2/admin/regcodes` | Admin | V2 注册码资源集合；返回 `items` 与有界 `pagination`，支持类型/状态/来源/搜索/排序筛选 |
| POST | `/api/v2/admin/regcodes` | Admin | V2 批量生成注册码；复用注册码参数、存储保护和审计 |
| POST | `/api/v2/admin/regcodes/batch-delete` | Admin | V2 批量物理删除注册码；继续要求确认短语和数量上限 |
| GET | `/api/v2/admin/regcodes/{code}` | Admin | V2 单注册码资源 |
| PATCH | `/api/v2/admin/regcodes/{code}` | Admin | V2 局部更新注册码状态、有效期、次数、天数和备注 |
| DELETE | `/api/v2/admin/regcodes/{code}` | Admin | V2 删除注册码并清理其引用 |
| GET | `/api/v2/admin/regcodes/{code}/usage` | Admin | V2 按需读取注册码使用者和 Telegram-only 使用记录 |
| POST | `/api/v2/admin/regcodes/{code}/usage/clear` | Admin | V2 清理注册码使用记录；继续要求确认短语 |
| GET | `/api/v2/admin/invite/tree` | Admin | V2 邀请关系树资源；返回 `item`，仅管理端读取且不缓存 |
| GET | `/api/v2/admin/invite/codes` | Admin | V2 邀请码资源集合；服务端搜索并返回 `items` 与有界 `pagination` |
| POST | `/api/v2/admin/invite/users/{uid}/detach` | Admin | V2 断开指定邀请关系 |
| POST | `/api/v2/admin/invite/users/{uid}/detach-delete-emby` | Admin | V2 断开关系并删除下级 Emby 账号 |
| POST | `/api/v2/admin/invite/users/detach-batch` | Admin | V2 批量断开/删除 Emby；复用保护与审计规则 |
| POST | `/api/v2/admin/invite/quick-maintenance` | Admin | V2 邀请快捷维护；支持断开、续期、子树和全量范围 |
| POST | `/api/v2/admin/invite/users/{uid}/disable` | Admin | V2 级联禁用邀请树用户 |
| POST | `/api/v2/admin/invite/users/{uid}/enable` | Admin | V2 级联启用邀请树用户 |
| POST | `/api/v2/admin/invite/users/{uid}/delete` | Admin | V2 级联删除本地/Emby 用户 |
| GET/PUT | `/api/v2/admin/invite/config/schema` | Admin | V2 邀请配置 SSR 读写资源；最终字段白名单仍由配置 handler 执行 |
| GET | `/api/v2/admin/media-requests` | Admin | V2 求片管理资源集合；服务端完成状态/来源/关键词筛选、同名聚合和分页，返回 `items`、`pagination`、状态计数，不缓存 |
| PUT | `/api/v2/admin/media-requests/{request_id}` | Admin | V2 按 ID 更新求片状态；兼容入口，管理端优先使用 require_key |
| DELETE | `/api/v2/admin/media-requests/{request_id}` | Admin | V2 按 ID 删除求片；兼容入口 |
| PUT | `/api/v2/admin/media-requests/by-key/{require_key}` | Admin | V2 按 key 更新求片；支持 `If-Match` revision 冲突保护 |
| PUT | `/api/v2/admin/media-requests/batch` | Admin | V2 原子批量更新同名求片；校验 1-100 个 key/revision 后一次性持久化 |
| PUT | `/api/v2/admin/media-requests/batch/by-key` | Admin | V2 批量更新兼容别名 |
| DELETE | `/api/v2/admin/media-requests/by-key/{require_key}` | Admin | V2 按 key 删除求片；支持 `If-Match` revision 冲突保护 |
| GET | `/api/v2/admin/users` | Admin | V2 管理员用户资源集合；服务端完成搜索、角色/状态筛选、排序和分页，返回 `items` 与 `pagination`，不缓存 |
| POST | `/api/v2/admin/users` | Admin | V2 创建 Web 用户；复用后端密码、角色、邮箱、Telegram 冲突校验和审计 |
| GET | `/api/v2/admin/users/{uid}` | Admin | V2 单用户资源；返回受限公开用户字段和 `admin_action_state` |
| PUT/DELETE | `/api/v2/admin/users/{uid}` | Admin | V2 更新或删除用户；沿用原子权限、保护账号和 Emby 清理规则 |
| POST | `/api/v2/admin/users/{uid}/disable` | Admin | V2 禁用用户，可按级联深度处理邀请下级 |
| POST | `/api/v2/admin/users/{uid}/enable` | Admin | V2 启用用户，可按级联深度处理邀请下级 |
| POST | `/api/v2/admin/users/{uid}/renew` | Admin | V2 管理员续期用户；`days=-1` 表示永久 |
| POST | `/api/v2/admin/users/{uid}/emby/enable` | Admin | V2 启用用户 Emby 账号 |
| POST | `/api/v2/admin/users/{uid}/emby/disable` | Admin | V2 禁用用户 Emby 账号 |
| POST | `/api/v2/admin/users/{uid}/force-unbind` | Admin | V2 强制解除本地 Emby 绑定 |
| POST | `/api/v2/admin/users/{uid}/refresh-status` | Admin | V2 手动刷新用户 Telegram/Emby 外部状态 |
| POST | `/api/v2/admin/users/{uid}/unbind-telegram` | Admin | V2 解绑用户 Telegram |
| POST | `/api/v2/admin/users/{uid}/admin` | Admin | V2 设置或取消管理员角色 |
| POST | `/api/v2/admin/users/{uid}/delete` | Admin | V2 删除用户；支持 `mode` 与 `cascade_depth`，推荐用于 SSR 管理操作 |
| GET | `/api/v2/admin/emby/users` | Admin | V2 Emby 账号资源集合；服务端搜索、筛选和双分页本地孤儿绑定，手动读取且不缓存 |
| GET | `/api/v2/admin/emby/device-audit` | Admin | V2 手动设备/IP 审查；按 Emby 用户聚合、过滤 Twilight 自身客户端，`refresh=1` 才强制刷新远端数据 |
| GET | `/api/v2/admin/emby/activity-logs` | Admin | V2 本地 Emby 活动日志；仅 `refresh=1` 时从 Emby 同步并入库 |
| POST | `/api/v2/admin/emby/activity-logs/sync` | Admin | V2 手动同步 Emby 活动日志并入库；默认读取最近 24 小时，可用 `since_hours` 调整范围 |
| POST | `/api/v2/admin/emby/test` | Admin | V2 后端 Emby 连通性与本机候选探测 |
| POST | `/api/v2/admin/emby/broadcast` | Admin | V2 向在线 Emby 会话发送广播，保留审计 |
| POST | `/api/v2/admin/emby/sync` | Admin | V2 同步本地 Emby 用户名映射 |
| POST | `/api/v2/admin/emby/import-users` | Admin | V2 扫描可导入的非管理员 Emby 账号 |
| POST | `/api/v2/admin/emby/delete-unlinked` | Admin | V2 删除未绑定 Emby 账号；后端保留管理员保护与审计 |
| POST | `/api/v2/admin/emby/cleanup-orphans` | Admin | V2 清理本地孤儿绑定 |
| POST | `/api/v2/admin/emby/reset-bindings` | Admin | V2 经确认后重置所有本地 Emby 绑定 |
| POST | `/api/v2/admin/emby/create-standalone` | Admin | V2 创建不关联 Web 账号的 Emby 用户 |
| POST | `/api/v2/admin/emby/force-set-password` | Admin | V2 强制重置 Emby 密码；明文仅存在本次响应 |
| POST | `/api/v2/admin/emby/users/{emby_id}/enable` | Admin | V2 启用单个 Emby 用户 |
| POST | `/api/v2/admin/emby/users/{emby_id}/disable` | Admin | V2 禁用单个 Emby 用户 |
| POST | `/api/v2/admin/emby/users/{emby_id}/kick` | Admin | V2 踢出单个 Emby 用户会话 |
| GET | `/api/v1/admin/media-requests` | Admin | 求片管理列表；支持 `status/source/q/page/per_page`，返回状态计数与分页元数据，不缓存 |
| PUT | `/api/v1/admin/media-requests/{request_id}` | Admin | 更新求片状态 |
| DELETE | `/api/v1/admin/media-requests/{request_id}` | Admin | 删除求片 |
| PUT | `/api/v1/admin/media-requests/by-key/{require_key}` | Admin | 按 key 更新求片；支持 `If-Match` revision 冲突保护 |
| PUT | `/api/v1/admin/media-requests/batch/by-key` | Admin | 原子批量更新 1-100 条求片；逐条校验 key 与 revision |
| DELETE | `/api/v1/admin/media-requests/by-key/{require_key}` | Admin | 按 key 删除求片；支持 `If-Match` revision 冲突保护 |
| GET | `/api/v1/admin/tickets` | Admin | 紧凑工单队列；默认仅返回待处理/处理中，`all=1` 或 `status=all` 返回全部；回复与附件只返回计数 |
| GET | `/api/v1/admin/tickets/{ticket_id}` | Admin | 管理员读取单个工单及完整对话，用于会话式处理页 |
| PUT | `/api/v1/admin/tickets/{ticket_id}` | Admin | 更新工单状态、优先级、类型和管理员内部摘要，不追加聊天回复 |
| POST | `/api/v1/admin/tickets/{ticket_id}/reply` | Admin | 管理员追加文字回复，不需要同时提交状态/类型/优先级表单 |
| DELETE | `/api/v1/admin/tickets/{ticket_id}` | Admin | 删除工单并清理附件目录 |
| GET | `/api/v1/admin/ticket-types` | Admin | 获取工单类型 |
| POST | `/api/v1/admin/ticket-types` | Admin | 新增工单类型 |
| PUT | `/api/v1/admin/ticket-types` | Admin | 重命名工单类型，并同步已有工单 |
| DELETE | `/api/v1/admin/ticket-types` | Admin | 删除工单类型；已有工单保留历史类型 |
| GET | `/api/v2/admin/tickets` | Admin | V2 工单资源集合；返回 `items` 与 `pagination`，默认仅返回待处理/处理中摘要 |
| GET | `/api/v2/admin/tickets/{ticket_id}` | Admin | V2 单工单资源；返回完整回复时间线和附件元数据 |
| PATCH | `/api/v2/admin/tickets/{ticket_id}` | Admin | V2 局部更新状态、优先级、类型或内部摘要 |
| POST | `/api/v2/admin/tickets/{ticket_id}/replies` | Admin | V2 追加管理员文字回复，不覆盖已有回复 |
| DELETE | `/api/v2/admin/tickets/{ticket_id}` | Admin | V2 删除工单并清理附件目录 |
| POST | `/api/v2/admin/tickets/{ticket_id}/attachments` | Admin | V2 上传工单附件，复用图片大小/数量/真实类型限制 |
| GET | `/api/v2/admin/tickets/{ticket_id}/attachments/{filename}` | Admin | V2 读取已登记的工单附件 |
| DELETE | `/api/v2/admin/tickets/{ticket_id}/attachments/{filename}` | Admin | V2 删除单个工单附件 |
| GET | `/api/v2/admin/ticket-types` | Admin | V2 工单类型资源集合 |
| POST | `/api/v2/admin/ticket-types` | Admin | V2 新增工单类型 |
| PATCH | `/api/v2/admin/ticket-types/{ticket_type}` | Admin | V2 按路径类型名重命名工单类型 |
| DELETE | `/api/v2/admin/ticket-types/{ticket_type}` | Admin | V2 按路径类型名删除工单类型 |
| POST | `/api/v1/admin/whitelist` | Admin | 设置白名单 |
| GET | `/api/v1/admin/stats` | Admin | 管理统计 |
| POST | `/api/v1/admin/users/bulk-expire` | Admin | 批量过期用户 |
| POST | `/api/v1/admin/users/bulk-enable-disabled` | Admin | 批量启用被禁用用户 |
| POST | `/api/v1/admin/users/cleanup-invalid` | Admin | 预览/清理无效用户（执行需确认短语） |
| POST | `/api/v1/admin/users/clear-stale-pending-emby` | Admin | 清理长期 PENDING_EMBY 的陈旧用户 |
| POST | `/api/v1/admin/users/clear-emails` | Admin | 预览/清空所有用户邮箱设置（执行需确认短语） |
| POST | `/api/v1/admin/users/{uid}/bind-email` | Admin | 强制把用户绑定到指定邮箱（`force` 可跳过名单/占用校验） |
| POST | `/api/v1/admin/users/{uid}/email/verified` | Admin | 置/撤销用户邮箱验证状态（不改邮箱） |
| POST | `/api/v1/admin/email/test` | Admin | 用当前 SMTP 配置发送测试邮件 |
| GET | `/api/v1/admin/email/verifications` | Admin | 邮箱验证审查；支持 `view=pending|accounts|summary`、`page`、`per_page`、`search`、`verified=all|verified|unverified`，列表视图分页且不返回另一类记录；无 `view` 时保留兼容全量响应 |
| POST | `/api/v1/admin/email/verifications/cleanup` | Admin | 手动清理所有已过期的在用验证码 |
| DELETE | `/api/v1/admin/email/verifications/{id}` | Admin | 撤销指定在用验证码记录（立即失效） |

| POST | `/api/v2/admin/email/test` | Admin | V2 使用当前 SMTP 配置发送测试邮件 |
| GET | `/api/v2/admin/email/verifications` | Admin | V2 邮箱验证审查分页资源；支持视图、搜索和验证状态筛选，私有不缓存 |
| POST | `/api/v2/admin/email/verifications/cleanup` | Admin | V2 清理过期邮箱验证码 |
| POST | `/api/v2/admin/email/verifications/clear-unverified` | Admin | V2 清理未验证账号邮箱 |
| DELETE | `/api/v2/admin/email/verifications/{id}` | Admin | V2 撤销指定邮箱验证码 |
| POST | `/api/v1/admin/users/kick-no-emby` | Admin | 踢出无 Emby 账号的用户 |
| GET | `/api/v1/admin/invite/tree` | Admin | 邀请树；邀请关闭时隐藏没有真实关系的孤立持码用户 |
| POST | `/api/v1/admin/invite/users/{uid}/detach` | Admin | 将用户脱离邀请关系；邀请关闭后仍可维护历史关系 |
| POST | `/api/v1/admin/invite/users/{uid}/detach-delete-emby` | Admin | 断开指定用户邀请关系并删除其远端 Emby 账号 |
| POST | `/api/v1/admin/invite/users/detach-batch` | Admin | 批量断开邀请关系；可删除全部所选 Emby，或用 `only_emby_disabled=true` 仅删除已禁用 Emby 并断开 |
| POST | `/api/v1/admin/invite/quick-maintenance` | Admin | 按所选/子树/全站断开并续期；`renew_days=-1` 为永久，已禁用 Web 账号只断开不续期 |
| GET | `/api/v1/admin/invite/codes` | Admin | 管理员视角邀请码列表 |
| GET | `/api/v1/admin/violations` | Admin | 违规记录列表 |
| DELETE | `/api/v1/admin/violations/{violation_id}` | Admin | 删除单条违规记录 |
| POST | `/api/v1/admin/violations/clear` | Admin | 清空违规记录 |

| GET | `/api/v2/admin/violations` | Admin | V2 违规审计分页资源；支持 `type`、`search`、`page`、`per_page`，私有不缓存 |
| DELETE | `/api/v2/admin/violations/{violation_id}` | Admin | V2 删除单条违规记录 |
| POST | `/api/v2/admin/violations/clear` | Admin | V2 清空违规记录；需要 `CLEAR_VIOLATIONS` 确认短语 |
| GET | `/api/v1/admin/audit-logs` | Admin | 操作审计日志列表（支持 category/action/uid/search 筛选与分页） |
| DELETE | `/api/v1/admin/audit-logs/{log_id}` | Admin | 删除单条操作审计日志 |
| POST | `/api/v1/admin/audit-logs/clear` | Admin | 清空全部审计日志（需确认短语 `CLEAR_AUDIT_LOGS`） |
| GET | `/api/v1/admin/bangumi/users` | Admin | 列出所有用户的 Bangumi 同步状态 |
| GET | `/api/v1/admin/bangumi/records/{uid}` | Admin | 查看某用户的播放记录 |
| POST | `/api/v1/admin/bangumi/sync/{uid}` | Admin | 为某用户触发 Bangumi 同步 |
| GET | `/api/v1/admin/bangumi/logs/{uid}` | Admin | 查看某用户的 Bangumi 同步日志 |
| DELETE | `/api/v1/admin/bangumi/logs/{uid}` | Admin | 清除某用户的 Bangumi 同步日志 |
| GET | `/api/v1/admin/telegram/rebind-requests` | Admin | Telegram 换绑申请列表 |
| POST | `/api/v1/admin/telegram/rebind-requests/{request_id}/approve` | Admin | 通过换绑申请 |
| POST | `/api/v1/admin/telegram/rebind-requests/{request_id}/reject` | Admin | 拒绝换绑申请 |
| POST | `/api/v1/admin/telegram/rebind-requests/batch` | Admin | 批量审核换绑申请 |
| GET | `/api/v2/admin/telegram/rebind-requests` | Admin | V2 Telegram 换绑申请分页资源；私有不缓存 |
| POST | `/api/v2/admin/telegram/rebind-requests/{request_id}/approve` | Admin | V2 批准换绑申请 |
| POST | `/api/v2/admin/telegram/rebind-requests/{request_id}/reject` | Admin | V2 拒绝换绑申请 |
| POST | `/api/v2/admin/telegram/rebind-requests/batch` | Admin | V2 批量审核换绑申请 |
| POST | `/api/v2/admin/telegram/rebind-requests/revoke-approved` | Admin | V2 撤销全部未使用的已批准换绑许可 |
| GET | `/api/v2/admin/telegram/commands/catalog` | Admin | V2 Telegram Bot 内置指令目录与禁用状态；私有不缓存 |
| GET | `/api/v2/admin/telegram/roster/stats` | Admin | V2 Telegram 花名册摘要；私有不缓存 |
| POST | `/api/v2/admin/telegram/test` | Admin | V2 手动测试 Telegram Bot 连通性；私有不缓存 |
| GET | `/api/v1/admin/telegram/commands/catalog` | Admin | Telegram Bot 内置指令目录与禁用状态 |
| GET | `/api/v1/admin/telegram/roster/stats` | Admin | Telegram 花名册统计 |
| POST | `/api/v1/admin/telegram/rejoined-users/enable` | Admin | 启用重新入群用户 |
| POST | `/api/v1/admin/telegram/kick-unbound` | Admin | 踢出未绑定 Telegram 的用户 |
| GET | `/api/v1/admin/scheduler/jobs` | Admin | 定时任务列表 |
| POST | `/api/v1/admin/scheduler/jobs/{job_id}/run` | Admin | 手动执行任务；`cleanup_emby_devices` 用于清理 Emby 历史设备记录，支持 `dry_run`、`max_workers`、`skip_usernames` |
| POST | `/api/v1/admin/scheduler/jobs/{job_id}/terminate` | Admin | 终止正在运行的任务 |
| GET | `/api/v1/admin/scheduler/jobs/{job_id}/last-run` | Admin | 最近执行结果 |
| GET | `/api/v1/admin/scheduler/jobs/{job_id}/history` | Admin | 执行历史 |
| PUT | `/api/v1/admin/scheduler/jobs/{job_id}/schedule` | Admin | 修改触发器 |
| DELETE | `/api/v1/admin/scheduler/jobs/{job_id}/schedule` | Admin | 恢复默认触发器 |
| GET | `/api/v1/admin/announcements` | Admin | 公告列表 |
| POST | `/api/v1/admin/announcements` | Admin | 创建公告 |
| PUT | `/api/v1/admin/announcements/{announcement_id}` | Admin | 更新公告 |
| DELETE | `/api/v1/admin/announcements/{announcement_id}` | Admin | 删除公告 |

## Security

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/security/devices` | User | 当前用户设备列表 |
| POST | `/api/v1/security/devices/{device_id}/block` | User | 拉黑自己的设备 |
| POST | `/api/v1/security/devices/{device_id}/trust` | User | 信任自己的设备 |
| GET | `/api/v1/security/login-history` | User | 当前用户登录历史 |
| GET | `/api/v1/security/login-history/{uid}` | Admin | 指定用户登录历史 |
| GET | `/api/v1/security/ip/blacklist` | Admin | IP 黑名单 |
| POST | `/api/v1/security/ip/blacklist` | Admin | 添加 IP 黑名单 |
| DELETE | `/api/v1/security/ip/blacklist` | Admin | 删除 IP 黑名单 |
| GET | `/api/v1/security/suspicious` | Admin | 可疑行为 |
| GET | `/api/v1/security/users/{uid}/devices` | Admin | 指定用户设备列表 |
| POST | `/api/v1/security/users/{uid}/devices/{device_id}/block` | Admin | 拉黑指定用户的设备 |

## Batch

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/api/v1/batch/users/disable` | Admin | 批量禁用用户 |
| POST | `/api/v1/batch/users/enable` | Admin | 批量启用用户 |
| POST | `/api/v1/batch/users/renew` | Admin | 批量续期用户 |
| POST | `/api/v1/batch/users/delete` | Admin | 批量删除用户 |
| POST | `/api/v1/batch/users/emby-unbind-lock` | Admin | 批量禁止用户自助解绑 Emby |
| POST | `/api/v1/batch/users/emby-grant-clear` | Admin | 批量清理无 Emby 账号用户的注册码/邀请码使用记录（解除误判的"已用过注册资格"锁定） |
| GET | `/api/v1/batch/export/users` | Admin | 导出用户 |
| GET | `/api/v1/batch/expiring-users` | Admin | 临期用户 |
| POST | `/api/v1/batch/send-reminders` | Admin | 发送到期提醒 |

## Announcements

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/announcements` | Public | 公开公告列表 |

> 公告以字段形式保存在单一状态文档（`internal/store`，唯一运行后端 PostgreSQL 的 `twilight_state` 表 jsonb 行）中，不存在独立的公告表或建表/迁移逻辑。

## Invite

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/invite/config` | Public | 邀请系统公开配置 |
| GET | `/api/v1/invite/me` | User | 我的邀请状态 |
| POST | `/api/v1/invite/codes` | User | 生成邀请码（邀请系统关闭时拒绝） |
| POST | `/api/v1/invite/renew-codes` | User | 为已有直属下级生成指名续期码（邀请系统关闭时仍允许） |
| GET | `/api/v1/invite/codes` | User | 我的邀请码列表 |
| DELETE | `/api/v1/invite/codes/{code}` | User | 删除/停用邀请码 |
| POST | `/api/v1/invite/me/detach-expired` | User | Emby 已到期/已禁用或 Web 已禁用时，自助删除自己的 Emby 账号并断开邀请关系 |
| POST | `/api/v1/invite/children/{uid}/detach-expired` | User | 删除 Emby 并断开 Emby 已到期/已禁用或 Web 已禁用的直属下级（邀请系统关闭时仍允许） |
| GET | `/api/v1/invite/check` | Public | 校验邀请码 |
| POST | `/api/v1/invite/use` | User | 使用邀请码开通 Emby（兼容旧入口） |

> 邀请关系与邀请码同样以字段形式存在于单一状态文档中（`invite_relations`、邀请码等），不存在独立的 `db/invites.db` 或单独的邀请关系表。

## Signin

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/signin/config` | Public | 签到公开配置 |
| GET | `/api/v1/signin/me` | User | 我的签到摘要、积分续期与个人自动续期状态 |
| POST | `/api/v1/signin` | User | 签到 |
| POST | `/api/v1/signin/renew` | User | 使用签到积分续期（需管理员开启且已绑定 Emby） |
| GET | `/api/v1/signin/history` | User | 签到历史 |

用户通过 `PUT /api/v1/users/me` 的 `signin_auto_renewal` 严格布尔字段开关个人自动续期；管理员还需先开启全局自动续期许可。详细条件见 [签到与积分续期](../features/signin.md)。

## API Key

外部 API Key 专用接口，全部为 API Key 鉴权。接入方式与权限模型见 [API Key 外部接入](../reference/api-key.md)。

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/apikey/info` | API Key | Key 绑定用户信息 |
| GET | `/api/v1/apikey/status` | API Key | Key 状态 |
| POST | `/api/v1/apikey/enable` | API Key | 启用当前账号 |
| POST | `/api/v1/apikey/disable` | API Key | 禁用当前账号 |
| POST | `/api/v1/apikey/renew` | API Key | 续期当前账号 |
| POST | `/api/v1/apikey/key/refresh` | API Key | 刷新 API Key |
| GET | `/api/v1/apikey/permissions` | API Key | 权限列表 |
| PUT | `/api/v1/apikey/permissions` | API Key | 禁止：API Key 不能自行修改权限（始终拒绝） |
| POST | `/api/v1/apikey/key/disable` | API Key | 禁用 Key |
| POST | `/api/v1/apikey/key/enable` | API Key | 启用 Key |
| GET | `/api/v1/apikey/emby/status` | API Key | Emby 状态 |
| POST | `/api/v1/apikey/emby/kick` | API Key | 将账号踢下线 |
| POST | `/api/v1/apikey/use-code` | API Key | 使用卡码/注册码 |
