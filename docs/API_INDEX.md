# Twilight API 索引

本文档是 `/api/v1` 的接口索引，用于快速核对路由、认证级别和归属模块。详细请求/响应示例见 [BACKEND_API.md](./BACKEND_API.md)，API Key 专用接口见 [API_KEY_API.md](./API_KEY_API.md)。

## 标记

| 标记 | 含义 |
| ---- | ---- |
| Public | 无需登录 Token |
| User | 需要登录 Token |
| Admin | 需要管理员登录 Token |
| API Key | 仅外部 API Key 认证 |
| Deprecated | 保留兼容但不建议使用 |

## 规范

| 项 | 约定 |
| -- | ---- |
| Base URL | `/api/v1` |
| 响应格式 | `{ success, message, data, timestamp }` |
| 前端认证 | `Authorization: Bearer <token>` |
| API Key 认证 | `X-API-Key: <key>` 或 `Authorization: ApiKey <key>` |
| 管理接口 | 优先归入 `/admin/*`，系统配置类管理接口归入 `/system/admin/*` |
| 用户自己的资源 | 优先使用 `/users/me/*` |
| 公开资源 | 必须显式记录为 Public，并评估限流和信息泄露风险 |
| 线路接口 | 只允许使用 `GET /system/emby-urls`，不要恢复 `/emby/urls` |
| 上传资源 | 只允许通过 `GET /users/assets/{kind}/{filename}` 受控访问，不公开 `/uploads` |

## Auth

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/auth/login` | Public | 用户名密码登录 |
| POST | `/auth/forgot-password/emby` | Public | 通过 Emby 账号密码验证后重置 Web 登录密码 |
| POST | `/auth/login/telegram` | Public | Telegram 直登 |
| POST | `/auth/login/apikey` | Public | API Key 换登录态 |
| POST | `/auth/logout` | User | 注销当前会话 |
| POST | `/auth/logout/all` | User | 注销全部会话 |
| GET | `/auth/me` | User | 当前登录用户 |
| POST | `/auth/refresh` | User | 刷新 Token |
| GET | `/auth/apikey` | User | 当前用户 API Key |
| POST | `/auth/apikey` | User | 生成或刷新 API Key |
| DELETE | `/auth/apikey` | User | 删除 API Key |
| POST | `/auth/apikey/enable` | User | 启用 API Key |
| GET | `/auth/apikey/permissions` | User | API Key 权限列表 |
| PUT | `/auth/apikey/permissions` | User | 更新 API Key 权限 |

## Users

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/users/register` | Public | 注册系统账号 |
| POST | `/users/me/emby/register` | User | 登录后补建 Emby 账号 |
| GET | `/users/register/emby/status` | Public | Emby 注册队列状态 |
| GET | `/users/check-available` | Public | 注册可用性检查 |
| GET | `/users/me` | User | 当前用户资料 |
| PUT | `/users/me` | User | 更新当前用户资料 |
| PUT | `/users/me/username` | User | 修改用户名 |
| PUT | `/users/me/password` | User | 修改密码（兼容旧入口） |
| POST | `/users/me/password/change` | User | 修改登录密码（兼容旧入口） |
| POST | `/users/me/password/system` | User | 修改系统登录密码 |
| POST | `/users/me/password/emby` | User | 修改 Emby 密码 |
| POST | `/users/me/emby/bind` | User | 绑定已有 Emby 账号 |
| POST | `/users/me/emby/unbind` | User | 解绑 Emby 账号 |
| POST | `/users/regcode/check` | Public | 检查注册码/续期码 |
| POST | `/users/me/renew` | User | 使用续期码续期 |
| POST | `/users/me/use-code` | User | 统一预检/使用注册码、续期码、白名单码、邀请码 |
| GET | `/users/me/devices` | User | 当前用户设备 |
| DELETE | `/users/me/devices/{device_id}` | User | 删除设备 |
| GET | `/users/me/libraries` | User | 当前用户媒体库权限 |
| GET | `/users/me/sessions` | User | 当前用户播放会话 |
| GET | `/users/me/login-history` | User | 当前用户登录历史 |
| GET | `/users/me/telegram` | User | Telegram 绑定状态 |
| POST | `/users/me/telegram/rebind-request` | User | 提交 Telegram 换绑申请 |
| POST | `/users/me/telegram/unbind` | User | 解绑 Telegram |
| GET | `/users/assets/{kind}/{filename}` | User | 受控访问头像/背景上传资源 |
| POST | `/users/me/telegram/bind-code` | User | 生成登录用户 TG 绑定码 |
| POST | `/users/telegram/register/bind-code` | Public | 生成注册用 TG 绑定码 |
| GET | `/users/telegram/register/bind-code/status` | Public | 查询注册用 TG 绑定码状态 |
| POST | `/users/me/telegram/bind-confirm` | User | 确认 TG 绑定 |
| GET | `/users/me/settings` | User | 当前用户设置聚合 |
| GET | `/users/{uid}/background` | User | 获取指定用户背景（本人或管理员） |
| PUT | `/users/me/background` | User | 更新背景配置 |
| DELETE | `/users/me/background` | User | 删除背景配置 |
| POST | `/users/me/background/upload` | User | 上传背景图 |
| GET | `/users/{uid}/avatar` | User | 获取指定用户头像（本人或管理员） |
| POST | `/users/me/avatar/upload` | User | 上传头像 |
| DELETE | `/users/me/avatar` | User | 删除头像 |
| GET | `/users/me/apikeys` | User | 当前用户 API Key 列表 |
| POST | `/users/me/apikeys` | User | 创建当前用户 API Key |
| PUT | `/users/me/apikeys/{key_id}` | User | 更新当前用户 API Key |
| DELETE | `/users/me/apikeys/{key_id}` | User | 删除当前用户 API Key |

## System

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/system/info` | Public | 系统公开信息 |
| GET | `/system/server-icon` | Public | 本地服务器图标 |
| GET | `/system/health` | Public | 健康检查 |
| GET | `/system/stats` | Admin | 系统运行时统计 |
| GET | `/system/emby-urls` | User | 按权限下发 Emby 线路 |
| GET | `/system/config` | User | 用户可见配置 |
| GET | `/system/admin/config` | Admin | 管理员完整配置 |
| GET | `/system/admin/stats` | Admin | 管理统计 |
| GET | `/system/admin/runtime/status` | Admin | Go 进程、主机、数据库和内存状态 |
| GET | `/system/admin/runtime/logs` | Admin | 读取后端内存日志快照 |
| GET | `/system/admin/runtime/logs/stream` | Admin | SSE 实时后端日志流 |
| POST | `/system/admin/update` | Admin | Git 自动更新与可选 systemd 重启调度 |
| GET | `/system/admin/database/status` | Admin | 当前数据库状态 |
| GET | `/system/admin/database/backups` | Admin | 数据库备份列表 |
| POST | `/system/admin/database/backup` | Admin | 创建数据库备份 |
| POST | `/system/admin/database/restore` | Admin | 从受控备份恢复数据库 |
| POST | `/system/admin/database/migrate` | Admin | 数据库迁移预检/执行 |
| GET | `/system/admin/config/toml` | Admin | 读取 TOML 配置 |
| PUT | `/system/admin/config/toml` | Admin | 保存 TOML 配置 |
| GET | `/system/admin/config/schema` | Admin | 配置表单 schema |
| PUT | `/system/admin/config/schema` | Admin | 保存配置表单 |
| POST | `/system/admin/config/sweep` | Admin | 清理配置文件 |
| GET | `/system/admin/apis` | Admin | 当前路由列表 |
| GET | `/system/admin/emby/libraries` | Admin | Emby 媒体库列表 |
| POST | `/system/admin/bot/test` | Admin | Telegram Bot 连通性测试 |

## Emby

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/emby/status` | User | Emby 服务器状态 |
| GET | `/emby/urls` | Deprecated | 已弃用，改用 `/system/emby-urls` |
| GET | `/emby/libraries` | User | 媒体库列表 |
| GET | `/emby/search` | User | Emby 媒体搜索 |
| GET | `/emby/latest` | User | 最新媒体 |
| GET | `/emby/sessions/count` | User | 会话数量 |

## Media

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/media/search` | User | 聚合搜索 |
| GET | `/media/search/tmdb` | User | TMDB 搜索 |
| GET | `/media/search/bangumi` | User | Bangumi 搜索 |
| GET | `/media/search/id/{source_type}/{media_id}` | User | 按源 ID 搜索 |
| GET | `/media/detail` | User | 媒体详情 |
| GET | `/media/tmdb/{tmdb_id}` | User | TMDB 详情 |
| GET | `/media/bangumi/{bgm_id}` | User | Bangumi 详情 |
| POST | `/media/inventory/check` | User | 检查库存 |
| GET | `/media/inventory/search` | User | 搜索库存 |
| POST | `/media/request` | User | 提交求片 |
| GET | `/media/request/my` | User | 我的求片 |
| GET | `/media/request/pending` | User | 待处理求片 |
| PUT | `/media/request/{request_id}/status` | User | 更新求片状态 |
| POST | `/media/request/external/update` | User | 外部更新求片 |
| GET | `/media/request/by-key/{require_key}` | User | 按 key 查询求片 |
| DELETE | `/media/request/by-key/{require_key}` | User | 按 key 删除求片 |
| GET | `/media/request/{request_id}` | User | 求片详情 |
| DELETE | `/media/request/{request_id}` | User | 删除求片 |

## Admin

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/admin/users` | Admin | 用户列表 |
| PUT | `/admin/me/update` | Admin | 更新管理员自身信息 |
| GET | `/admin/users/{uid}` | Admin | 用户详情 |
| POST | `/admin/users/{uid}/disable` | Admin | 禁用用户 |
| POST | `/admin/users/{uid}/enable` | Admin | 启用用户 |
| PUT | `/admin/users/{uid}` | Admin | 更新用户 |
| DELETE | `/admin/users/{uid}` | Admin | 删除用户 |
| DELETE | `/admin/users/{uid}/emby` | Admin | 删除用户 Emby 账号 |
| POST | `/admin/users/{uid}/force-unbind` | Admin | 强制解除本地绑定 |
| POST | `/admin/users/sync-bindings` | Admin | 同步绑定状态 |
| POST | `/admin/users/{uid}/renew` | Admin | 管理员续期 |
| POST | `/admin/emby/force-set-password` | Admin | 强制设置 Emby 密码 |
| POST | `/admin/users/{uid}/reset-password` | Admin | 重置密码 |
| POST | `/admin/users/{uid}/kick` | Admin | 踢下线 |
| GET | `/admin/users/{uid}/libraries` | Admin | 用户媒体库权限 |
| PUT | `/admin/users/{uid}/libraries` | Admin | 更新用户媒体库权限 |
| PUT | `/admin/users/{uid}/admin` | Admin | 设置管理员权限 |
| POST | `/admin/users/{uid}/unbind-telegram` | Admin | 解绑 Telegram |
| POST | `/admin/users/{uid}/bind-telegram` | Admin | 强制绑定 Telegram |
| GET | `/admin/telegram/rebind-requests` | Admin | TG 换绑申请列表 |
| POST | `/admin/telegram/rebind-requests/{request_id}/approve` | Admin | 通过换绑申请 |
| POST | `/admin/telegram/rebind-requests/{request_id}/reject` | Admin | 拒绝换绑申请 |
| GET | `/admin/users/by-telegram/{telegram_id}` | Admin | 按 TGID 查用户 |
| POST | `/admin/emby/sync` | Admin | 同步 Emby 用户 |
| GET | `/admin/regcodes` | Admin | 注册码列表 |
| POST | `/admin/regcodes` | Admin | 创建注册码 |
| GET | `/admin/regcodes/{code}/users` | Admin | 查看注册码使用者详情 |
| PUT | `/admin/regcodes/{code}` | Admin | 更新注册码备注 |
| DELETE | `/admin/regcodes/{code}` | Admin | 删除注册码 |
| GET | `/admin/media-requests` | Admin | 求片管理列表 |
| PUT | `/admin/media-requests/{request_id}` | Admin | 更新求片 |
| DELETE | `/admin/media-requests/{request_id}` | Admin | 删除求片 |
| PUT | `/admin/media-requests/by-key/{require_key}` | Admin | 按 key 更新求片 |
| DELETE | `/admin/media-requests/by-key/{require_key}` | Admin | 按 key 删除求片 |
| GET | `/admin/emby/sessions` | Admin | Emby 会话 |
| GET | `/admin/emby/activity` | Admin | Emby 活动 |
| POST | `/admin/emby/broadcast` | Admin | Emby 广播 |
| POST | `/admin/whitelist` | Admin | 设置白名单 |
| GET | `/admin/stats` | Admin | 管理统计 |
| POST | `/admin/emby/test` | Admin | 测试 Emby 连接 |
| GET | `/admin/emby/users` | Admin | Emby 用户列表 |
| POST | `/admin/emby/cleanup-orphans` | Admin | 清理孤儿绑定 |
| POST | `/admin/emby/import-users` | Admin | 导入 Emby 用户 |
| POST | `/admin/emby/reset-bindings` | Admin | 重置 Emby 绑定 |
| POST | `/admin/emby/delete-unlinked` | Admin | 删除未绑定 Emby 用户 |
| POST | `/admin/users/bulk-expire` | Admin | 批量过期 |
| POST | `/admin/users/bulk-enable-disabled` | Admin | 批量启用禁用用户 |
| POST | `/admin/users/cleanup-invalid` | Admin | 清理无效用户 |
| POST | `/admin/users/kick-no-emby` | Admin | 踢出无 Emby 用户 |
| GET | `/admin/invite/tree` | Admin | 邀请树 |
| POST | `/admin/invite/users/{uid}/detach` | Admin | 脱离邀请关系 |
| GET | `/admin/invite/codes` | Admin | 管理员邀请码列表 |
| GET | `/admin/announcements` | Admin | 公告列表 |
| POST | `/admin/announcements` | Admin | 创建公告 |
| PUT | `/admin/announcements/{announcement_id}` | Admin | 更新公告 |
| DELETE | `/admin/announcements/{announcement_id}` | Admin | 删除公告 |
| GET | `/admin/scheduler/jobs` | Admin | 定时任务列表 |
| POST | `/admin/scheduler/jobs/{job_id}/run` | Admin | 手动执行任务 |
| GET | `/admin/scheduler/jobs/{job_id}/last-run` | Admin | 最近执行 |
| GET | `/admin/scheduler/jobs/{job_id}/history` | Admin | 执行历史 |
| PUT | `/admin/scheduler/jobs/{job_id}/schedule` | Admin | 修改触发器 |
| DELETE | `/admin/scheduler/jobs/{job_id}/schedule` | Admin | 恢复默认触发器 |
| POST | `/admin/emby/create-standalone` | Admin | 创建独立 Emby 用户 |
| POST | `/admin/users/{uid}/bind-emby` | Admin | 绑定 Emby 到用户 |
| GET | `/admin/telegram/roster/stats` | Admin | Telegram 花名册统计 |
| POST | `/admin/telegram/rejoined-users/enable` | Admin | 启用重新入群用户 |
| POST | `/admin/telegram/kick-unbound` | Admin | 踢出未绑定 TG 用户 |

## Stats

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/stats/me` | User | 当前用户播放统计 |
| GET | `/stats/user/{uid}` | Admin | 指定用户播放统计 |

## Security

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/security/devices` | Admin | 设备列表 |
| POST | `/security/devices/{device_id}/block` | Admin | 拉黑设备 |
| POST | `/security/devices/{device_id}/trust` | Admin | 信任设备 |
| GET | `/security/login-history` | Admin | 登录历史 |
| GET | `/security/login-history/{uid}` | Admin | 指定用户登录历史 |
| GET | `/security/ip/blacklist` | Admin | IP 黑名单 |
| POST | `/security/ip/blacklist` | Admin | 添加 IP 黑名单 |
| DELETE | `/security/ip/blacklist` | Admin | 删除 IP 黑名单 |
| GET | `/security/suspicious` | Admin | 可疑行为 |
| GET | `/security/users/{uid}/devices` | Admin | 指定用户设备 |
| POST | `/security/users/{uid}/devices/{device_id}/block` | Admin | 拉黑指定用户设备 |

## Batch

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| POST | `/batch/users/disable` | Admin | 批量禁用用户 |
| POST | `/batch/users/enable` | Admin | 批量启用用户 |
| POST | `/batch/users/renew` | Admin | 批量续期用户 |
| POST | `/batch/users/delete` | Admin | 批量删除用户 |
| GET | `/batch/export/users` | Admin | 导出用户 |
| GET | `/batch/export/playback` | Admin | 导出播放数据 |
| GET | `/batch/watch-stats` | Admin | 播放统计列表 |
| GET | `/batch/watch-stats/{uid}` | Admin | 指定用户播放统计 |
| GET | `/batch/watch-stats/global` | Admin | 全局播放统计 |
| GET | `/batch/expiring-users` | Admin | 临期用户 |
| POST | `/batch/send-reminders` | Admin | 发送提醒 |

## Announcements

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/announcements` | Public | 公开公告列表 |

## Invite

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/invite/config` | Public | 邀请系统公开配置 |
| GET | `/invite/me` | User | 我的邀请状态 |
| POST | `/invite/codes` | User | 生成邀请码 |
| GET | `/invite/codes` | User | 我的邀请码 |
| DELETE | `/invite/codes/{code}` | User | 删除或停用邀请码 |
| POST | `/invite/renew-codes` | User | 为已到期直属下级生成专属续期码 |
| POST | `/invite/check` | Public | 校验邀请码 |
| POST | `/invite/use` | User | 使用邀请码开通 Emby（兼容旧入口） |

## Signin

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/signin/config` | Public | 签到公开配置 |
| GET | `/signin/me` | User | 我的签到摘要 |
| POST | `/signin` | User | 签到 |
| GET | `/signin/history` | User | 签到历史 |

## Demo

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/demo/bootstrap` | Public | TestWeb 演示聚合数据，返回预设假数据 |
| GET | `/demo/auth/me` | Public | TestWeb 演示当前用户 |
| GET | `/demo/system/info` | Public | TestWeb 演示系统信息 |
| GET | `/demo/admin/users` | Public | TestWeb 演示用户列表 |
| GET | `/demo/admin/regcodes` | Public | TestWeb 演示注册码列表 |
| GET | `/demo/media/search` | Public | TestWeb 演示媒体搜索 |
| POST/PUT/DELETE | `/demo/action/{action_name}` | Public | TestWeb 演示写操作模拟结果；动作名白名单校验，忽略请求体，不执行真实操作 |

## API Key

| 方法 | 路径 | 认证 | 说明 |
| ---- | ---- | ---- | ---- |
| GET | `/apikey/info` | API Key | Key 绑定用户信息 |
| GET | `/apikey/status` | API Key | Key 状态 |
| POST | `/apikey/enable` | API Key | 启用当前账号 |
| POST | `/apikey/disable` | API Key | 禁用当前账号 |
| POST | `/apikey/renew` | API Key | 续期当前账号 |
| POST | `/apikey/key/refresh` | API Key | 刷新 API Key |
| GET | `/apikey/permissions` | API Key | 权限列表 |
| PUT | `/apikey/permissions` | API Key | 禁止：API Key 不能自行修改权限 |
| POST | `/apikey/key/disable` | API Key | 禁用 Key |
| POST | `/apikey/key/enable` | API Key | 启用 Key |
| GET | `/apikey/emby/status` | API Key | Emby 状态 |
| POST | `/apikey/emby/kick` | API Key | 踢下线 |
| POST | `/apikey/use-code` | API Key | 使用卡码 |
