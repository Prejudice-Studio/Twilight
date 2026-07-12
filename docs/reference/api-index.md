# API 路由索引

本文是 Twilight 后端 `/api/v1` 接口的速查索引，用于快速核对每条路由的请求方法、路径、鉴权级别和所属模块。本文严格依据 `internal/api/routes.go` 中 `a.add(method, path, authLevel, handler)` 的真实注册逐条整理；详细的请求/响应示例见 [后端 API 详参](../reference/backend-api.md)，外部 API Key 接入说明见 [API Key 外部接入](../reference/api-key.md)。

> Emby ActivityLog 同步会配对播放开始/停止事件并幂等写入 `PlaybackRecords`，供 Bangumi 同步等内部流程复用。播放统计页面、统计 API 与导出入口已移除；在线人数只统计 Emby `/Sessions` 中带 `NowPlayingItem` 的正在播放会话。

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

> 说明：`X-Twilight-Client` 只用于前端请求识别与 CORS 允许头，不参与鉴权。少数有副作用的 `GET`（如绑定码创建）还要求 `X-Twilight-Intent` 显式声明操作意图，用于拦截预取/探测误触发。

## 根与文档

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/` | Public | 根路由 |
| GET | `/api/v1/openapi.json` | Public | OpenAPI 规范文档 |
| GET | `/api/v1/docs` | Public | 在线 API 文档页 |

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
| POST | `/api/v1/users/me/renew` | User | 使用续期码续期 |
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
| GET | `/api/v1/system/health` | Public | 健康检查 |
| GET | `/api/v1/system/stats` | Admin | 系统运行时统计 |
| GET | `/api/v1/system/emby-urls` | User | 按权限下发 Emby 线路 |
| POST | `/api/v1/system/emby-urls/probe` | User | 探测 Emby 线路连通性 |
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
| GET | `/api/v1/media/search` | User | 聚合搜索 |
| GET | `/api/v1/media/search/tmdb` | User | TMDB 搜索 |
| GET | `/api/v1/media/search/bangumi` | User | Bangumi 搜索 |
| GET | `/api/v1/media/search/id/{source_type}/{media_id}` | User | 按源 ID 搜索详情 |
| GET | `/api/v1/media/detail` | User | 媒体详情 |
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
| GET | `/api/v1/admin/emby/device-audit` | Admin | Emby 登录用户设备/IP 审查（按用户聚合）：`/Devices` 设备清单 + 实时会话 IP（解析掉端口）+ 活动日志历史登录 IP，映射完整本地账号（网页/Emby/Telegram） |
| GET | `/api/v1/admin/emby/activity-logs` | Admin | 本地 Emby 活动日志；`?refresh=1` 手动从 Emby 拉取并入库 |
| GET | `/api/v1/admin/emby/activity` | Admin | Emby 活动记录 |
| GET | `/api/v1/admin/emby/users` | Admin | Emby 用户列表 |
| POST | `/api/v1/admin/emby/broadcast` | Admin | Emby 广播消息 |
| POST | `/api/v1/admin/emby/test` | Admin | 测试 Emby 连接 |
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
| GET | `/api/v1/admin/media-requests` | Admin | 求片管理列表 |
| PUT | `/api/v1/admin/media-requests/{request_id}` | Admin | 更新求片状态 |
| DELETE | `/api/v1/admin/media-requests/{request_id}` | Admin | 删除求片 |
| PUT | `/api/v1/admin/media-requests/by-key/{require_key}` | Admin | 按 key 更新求片 |
| DELETE | `/api/v1/admin/media-requests/by-key/{require_key}` | Admin | 按 key 删除求片 |
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
| GET | `/api/v1/admin/email/verifications` | Admin | 邮箱验证记录审查：在用验证码（脱敏，不含验证码/哈希）+ 已绑邮箱账号及验证状态 + 统计 |
| POST | `/api/v1/admin/email/verifications/cleanup` | Admin | 手动清理所有已过期的在用验证码 |
| DELETE | `/api/v1/admin/email/verifications/{id}` | Admin | 撤销指定在用验证码记录（立即失效） |
| POST | `/api/v1/admin/users/kick-no-emby` | Admin | 踢出无 Emby 账号的用户 |
| GET | `/api/v1/admin/invite/tree` | Admin | 邀请树 |
| POST | `/api/v1/admin/invite/users/{uid}/detach` | Admin | 将用户脱离邀请关系 |
| GET | `/api/v1/admin/invite/codes` | Admin | 管理员视角邀请码列表 |
| GET | `/api/v1/admin/violations` | Admin | 违规记录列表 |
| DELETE | `/api/v1/admin/violations/{violation_id}` | Admin | 删除单条违规记录 |
| POST | `/api/v1/admin/violations/clear` | Admin | 清空违规记录 |
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
| GET | `/api/v1/admin/telegram/roster/stats` | Admin | Telegram 花名册统计 |
| POST | `/api/v1/admin/telegram/rejoined-users/enable` | Admin | 启用重新入群用户 |
| POST | `/api/v1/admin/telegram/kick-unbound` | Admin | 踢出未绑定 Telegram 的用户 |
| GET | `/api/v1/admin/scheduler/jobs` | Admin | 定时任务列表 |
| POST | `/api/v1/admin/scheduler/jobs/{job_id}/run` | Admin | 手动执行任务 |
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

> 公告以字段形式保存在单一状态文档（`internal/store`，对应 JSON 文件 `db/twilight_go_state.json` 或 PostgreSQL `twilight_state` 表）中，不存在独立的公告表或建表/迁移逻辑。

## Invite

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/invite/config` | Public | 邀请系统公开配置 |
| GET | `/api/v1/invite/me` | User | 我的邀请状态 |
| POST | `/api/v1/invite/codes` | User | 生成邀请码（邀请系统关闭时拒绝） |
| POST | `/api/v1/invite/renew-codes` | User | 为已有直属下级生成指名续期码（邀请系统关闭时仍允许） |
| GET | `/api/v1/invite/codes` | User | 我的邀请码列表 |
| DELETE | `/api/v1/invite/codes/{code}` | User | 删除/停用邀请码 |
| POST | `/api/v1/invite/children/{uid}/detach-expired` | User | 删除 Emby 并断开 Emby 已到期或 Web 已禁用的直属下级 |
| GET | `/api/v1/invite/check` | Public | 校验邀请码 |
| POST | `/api/v1/invite/use` | User | 使用邀请码开通 Emby（兼容旧入口） |

> 邀请关系与邀请码同样以字段形式存在于单一状态文档中（`invite_relations`、邀请码等），不存在独立的 `db/invites.db` 或单独的邀请关系表。

## Signin

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/api/v1/signin/config` | Public | 签到公开配置 |
| GET | `/api/v1/signin/me` | User | 我的签到摘要 |
| POST | `/api/v1/signin` | User | 签到 |
| POST | `/api/v1/signin/renew` | User | 使用签到积分续期（需管理员开启） |
| GET | `/api/v1/signin/history` | User | 签到历史 |

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
