# V2 模板参数完整参考

## 概述

V2 引入了统一的模板参数系统，支持约 60 个参数，可在以下模板中使用：
- 登录通知模板（Telegram / 邮件）
- 工单通知模板（Telegram）
- Telegram 群组用户面板模板

所有参数使用 `{参数名}` 格式，如 `{username}`、`{emby_enabled}` 等。

## 参数分类

### 基础信息

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{server_name}` | 服务器名称 | Twilight |
| `{username}` | 用户名 | alice |
| `{uid}` | 用户 UID | 1001 |

### 角色和权限

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{role}` | 角色名称（简短） | 管理员 / 普通用户 / 白名单用户 |
| `{role_name}` | 角色名称（完整） | 管理员 / 普通用户 / 白名单用户 |
| `{role_id}` | 角色 ID | 0 / 1 / 2 |
| `{is_admin}` | 是否管理员 | 是 / 否 |
| `{is_whitelist}` | 是否白名单用户 | 是 / 否 |
| `{is_protected}` | 是否受保护账号 | 是 / 否 |

### 账号状态

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{web_status}` | Web 账号状态 | 正常 / 已禁用 |
| `{web_active}` | Web 是否激活 | 是 / 否 |
| `{account_enabled}` | 账号启用状态 | 已启用 / 已禁用 |
| `{account_disabled}` | 账号是否禁用 | 是 / 否 |

### 到期信息

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{expire_status}` | 到期状态 | 正常 / 已过期 / 即将到期 |
| `{expired_at}` | 到期时间 | 2026-12-31 23:59:59 / 永久 |
| `{expiry_time}` | 到期时间（同上） | 2026-12-31 23:59:59 / 永久 |
| `{days_until_expiry}` | 到期剩余天数 | 永久 / 已过期 / 30天 / 不足1天 |
| `{is_expired}` | 是否已过期 | 是 / 否 |
| `{is_permanent}` | 是否永久账号 | 是 / 否 |

### 注册信息

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{register_time}` | 注册时间 | 2026-01-15 10:30:00 |
| `{created_at}` | 创建时间 | 2026-01-15 10:30:00 |
| `{registration_source}` | 注册来源 | 注册码 / 邀请码 / 管理员创建 / - |
| `{registration_code}` | 使用的注册码 | REG2024ABCD / - |

### Emby 状态（重点功能）

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{emby_status}` | Emby 状态（简略） | 未绑定 / 已绑定 / 已禁用 |
| `{emby_bound}` | 是否绑定 Emby | 是 / 否 |
| `{emby_bound_status}` | 绑定状态 | 未绑定 / 等待开通 / 已绑定 |
| `{emby_username}` | Emby 用户名 | alice_emby / - |
| `{emby_id}` | Emby 用户 ID | 1234567890abcdef / - |
| `{emby_enabled}` | Emby 是否启用 | 是 / 否 |
| `{emby_disabled}` | Emby 是否禁用 | 是 / 否 |
| `{emby_enabled_status}` | Emby 启用状态 | 已启用 / 已禁用 / - |
| `{emby_disabled_reason}` | Emby 禁用原因 | 未绑定 / 正常 / Web账号被禁用 / 账号已过期 / 已禁用 |
| `{pending_emby}` | 是否等待开通 Emby | 是 / 否 |
| `{pending_emby_days}` | 待开通 Emby 天数 | 7天 / 永久 / - |
| `{emby_grant_locked}` | 是否已使用注册资格 | 是 / 否 |
| `{emby_unbind_allowed}` | 是否允许解绑 Emby | 是 / 否 |

### Emby 远程状态（仅 Telegram 群组面板）

这些参数需要查询远程 Emby 服务器，仅在 Telegram 群组面板模板中可用：

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{emby_remote_status}` | 远程状态 | 正常 / 已禁用 / - |
| `{emby_remote_username}` | 远程用户名 | alice_emby / - |
| `{emby_remote_enabled}` | 远程是否启用 | 是 / 否 / - |
| `{emby_remote_role}` | 远程角色 | 管理员 / 普通用户 / - |
| `{emby_remote_hidden}` | 远程是否隐藏 | 是 / 否 / - |
| `{emby_last_activity}` | 最后活动时间 | 2小时前 / 从未使用 / - |
| `{emby_remote_block}` | 远程状态块（多行） | (完整的远程状态信息) |

### Telegram 绑定状态

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{telegram_status}` | Telegram 状态 | 未绑定 / 已绑定 / 换绑中 |
| `{telegram_bound}` | 是否绑定 Telegram | 是 / 否 |
| `{telegram_username}` | Telegram 用户名 | @alice / - |
| `{telegram_userid}` | Telegram 用户 ID | 123456789 / - |
| `{telegram_id}` | Telegram 用户 ID（同上） | 123456789 / - |
| `{rebinding_in_progress}` | 是否换绑中 | 是 / 否 |

### 邮箱信息

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{email}` | 邮箱地址 | alice@example.com / - |
| `{email_bound}` | 是否绑定邮箱 | 是 / 否 |
| `{email_verified}` | 邮箱是否已验证 | 是 / 否 |
| `{email_verified_status}` | 邮箱验证状态 | 已验证 / 未验证 |
| `{email_verified_at}` | 邮箱验证时间 | 2026-01-20 15:00:00 / - |

### Bangumi 同步

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{bgm_mode}` | Bangumi 同步模式 | 已启用 / 未启用 |
| `{bgm_token_status}` | Bangumi Token 状态 | 已配置 / 未配置 |
| `{bgm_sync_status}` | Bangumi 同步状态 | 未启用 / 缺少个人 Token / 可同步 |

### API Key

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{api_key_status}` | API Key 状态 | 已启用 / 未启用 |
| `{api_key_enabled}` | API Key 是否启用 | 是 / 否 |

### 通知设置

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{notify_login_telegram}` | 登录 Telegram 通知 | 是 / 否 |
| `{notify_login_email}` | 登录邮件通知 | 是 / 否 |
| `{notify_ticket_telegram}` | 工单 Telegram 通知 | 是 / 否 |

### 特殊参数（特定模板）

#### 登录通知专用

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{time}` | 登录时间 | 2026-09-11 14:30:00 |
| `{ip}` | 登录 IP | 192.168.1.100 |
| `{device}` | 登录设备 | Chrome 120 / Mobile Safari |

#### 工单通知专用

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{ticket_id}` | 工单 ID | 1001 |
| `{title}` | 工单标题 | 无法登录 Emby |
| `{status}` | 工单状态 | 待处理 / 处理中 / 已解决 / 已关闭 |
| `{priority}` | 工单优先级 | 低 / 中 / 高 / 紧急 |
| `{type}` | 工单类型 | 技术支持 / 账号问题 / 其他 |
| `{admin_note}` | 管理员备注 | (管理员的回复内容) |
| `{admin_note_content}` | 管理员备注内容（格式化） | (带格式的回复内容) |
| `{time}` | 通知时间 | 2026-09-11 14:30:00 |

#### Telegram 面板专用

| 参数 | 说明 | 示例值 |
|------|------|--------|
| `{panel_ttl}` | 面板有效期 | 1 分钟 |
| `{panel_ttl_seconds}` | 面板有效期（秒） | 60 |

## 模板示例

### 登录通知 - 简洁版

```
🔐 新登录通知

账号：{username}
时间：{time}
IP：{ip}
设备：{device}

{server_name}
```

### 登录通知 - 详细版

```
🔐 新登录通知

== 基本信息 ==
用户：{username} (UID: {uid})
角色：{role_name}
时间：{time}
IP：{ip}
设备：{device}

== 账号状态 ==
Web 账号：{account_enabled}
到期状态：{expire_status}
剩余天数：{days_until_expiry}

== Emby 状态 ==
绑定状态：{emby_bound_status}
启用状态：{emby_enabled_status}
禁用原因：{emby_disabled_reason}

== Telegram ==
绑定状态：{telegram_status}
用户名：{telegram_username}

如非本人操作，请立即修改密码！
访问 {server_name} 管理您的账号。
```

### 工单通知 - 默认版

```
🎫 工单更新通知
{server_name}
━━━━━━━━━━━━━━
🆔 #{ticket_id}  {title}
📊 状态：{status}
🕒 {time}
{admin_note_content}
```

### 工单通知 - 增强版

```
🎫 工单更新通知
{server_name}
━━━━━━━━━━━━━━
🆔 工单 #{ticket_id}
📋 {title}
📊 状态：{status}
⚡ 优先级：{priority}
🏷️ 类型：{type}
🕒 更新时间：{time}

== 您的账号状态 ==
👤 用户：{username} ({role_name})
📺 Emby：{emby_enabled_status}
⏰ 到期：{days_until_expiry}
✉️ 邮箱：{email_verified_status}

{admin_note_content}

💡 提示：在面板中回复此工单以继续沟通
```

### Telegram 群组面板 - 默认版

```
Twilight 群组用户面板

== 用户 ==
用户名: {username}
UID: {uid}
角色: {role}
受保护: {is_protected}

== Web 账号 ==
状态: {web_status}
到期: {expire_status}
注册时间: {register_time}

== 绑定 ==
Telegram: {telegram_status}
Emby: {emby_status}
{emby_remote_block}

== 安全提示 ==
面板 {panel_ttl} 无操作会自动删除；每次按钮操作都会重新校验管理员身份。
群内面板不展示邮箱、Emby ID、Telegram ID、Token、密码或服务器线路。
```

### Telegram 群组面板 - 详细版

```
🎛️ Twilight 群组用户面板

👤 用户信息
━━━━━━━━━━━━━━
用户名: {username}
UID: {uid}
角色: {role_name}
受保护: {is_protected}

💻 Web 账号
━━━━━━━━━━━━━━
状态: {account_enabled}
到期: {expire_status}
剩余: {days_until_expiry}
注册: {register_time}
来源: {registration_source}

📺 Emby 服务
━━━━━━━━━━━━━━
绑定: {emby_bound_status}
状态: {emby_enabled_status}
原因: {emby_disabled_reason}
用户名: {emby_username}
{emby_remote_block}

💬 Telegram
━━━━━━━━━━━━━━
状态: {telegram_status}
用户名: {telegram_username}
用户ID: {telegram_id}

✉️ 邮箱
━━━━━━━━━━━━━━
地址: {email}
验证: {email_verified_status}

📊 其他服务
━━━━━━━━━━━━━━
Bangumi: {bgm_sync_status}
API Key: {api_key_status}

🔔 通知设置
━━━━━━━━━━━━━━
登录 TG 通知: {notify_login_telegram}
登录邮件通知: {notify_login_email}
工单 TG 通知: {notify_ticket_telegram}

⚠️ 安全提示
━━━━━━━━━━━━━━
面板 {panel_ttl} 无操作会自动删除
每次按钮操作都会重新校验管理员身份
群内面板不展示敏感信息（邮箱、ID、密码等）
```

## 参数值说明

### 布尔值参数

所有 `{xxx_enabled}` / `{xxx_disabled}` / `{is_xxx}` 格式的参数返回：
- `是` - 真值
- `否` - 假值

### 状态参数

状态参数（如 `{emby_status}`、`{telegram_status}`）返回中文描述性文本。

### 时间参数

时间参数格式：`YYYY-MM-DD HH:MM:SS`，例如 `2026-09-11 14:30:00`

特殊值：
- `-` - 未设置/不适用
- `永久` - 无到期时间

### 占位符处理

- **已知参数**：替换为实际值
- **未知参数**：保持原样（如 `{unknown_param}` 不会被替换）
- **空值参数**：根据类型显示 `-` 或 `否`

## 最佳实践

1. **保持简洁**：不要在单个模板中使用过多参数，保持可读性
2. **分组信息**：使用分隔线和标题组织信息
3. **关键信息前置**：将最重要的信息放在前面
4. **测试模板**：修改后测试各种场景（过期用户、未绑定用户等）
5. **考虑长度**：Telegram 消息有长度限制（4096 字符）
6. **使用表情**：适当使用表情符号提升可读性（Telegram 支持）

## 常见问题

### Q: 为什么某些参数显示为 `-`？

**A**: 参数值为空或不适用时显示 `-`，例如未绑定邮箱时 `{email}` 显示 `-`。

### Q: Telegram 面板的远程参数不显示怎么办？

**A**: 远程参数（`{emby_remote_*}`）需要查询 Emby 服务器，如果 Emby 未配置或连接失败，这些参数会显示 `-`。

### Q: 可以在模板中使用 HTML 标签吗？

**A**: 仅工单通知模板支持 HTML（Telegram HTML 模式），其他模板为纯文本。支持的标签：`<b>`、`<i>`、`<code>`、`<pre>`、`<a>`、`<blockquote>`。

### Q: 参数大小写敏感吗？

**A**: 是的。必须使用小写形式，如 `{username}` 而非 `{Username}` 或 `{USERNAME}`。

### Q: 如何在模板中使用花括号字面量？

**A**: 目前模板系统会尝试解析所有 `{xxx}` 格式的内容，未识别的参数会保持原样。如需显示字面量花括号，可以使用全角字符 `｛｝`。

## 参数变更历史

### V2.0.0 (2026-09)
- ✅ 新增约 50 个参数
- ✅ 引入统一参数系统
- ✅ 所有模板支持完整用户状态参数
- ✅ 新增 Emby 禁用原因、到期剩余天数等关键参数

### V1.x
- 基础参数：`username`、`uid`、`time`、`ip`、`device`、`server_name`
- 工单参数：`ticket_id`、`title`、`status`、`admin_note`
