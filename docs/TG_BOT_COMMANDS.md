# Telegram Bot 命令文档

本文档说明 `golang` 分支当前 Bot 命令、权限边界和安全约束。Bot 主要负责绑定、查询、统计和通知；涉及用户写入、删除、密码、系统更新、数据库恢复等高风险操作，应在 Web 后台完成。

## 基本规则

| 规则 | 说明 |
| ---- | ---- |
| 私聊优先 | `/bind`、`/me`、`/emby`、`/playinfo`、`/resetpwd`、`/stats`、`/userinfo`、`/twfind` 等账号或管理员命令只能在私聊使用。 |
| 群聊保护 | 群聊中使用账号类命令时，Bot 只提示去私聊，避免泄露账号状态。 |
| 管理员判定 | `Telegram.admin_id` 中的 Telegram ID，或已绑定 Twilight 管理员账号的 Telegram ID，才可使用管理员命令。 |
| 敏感信息 | Bot 不展示密码、Token、Emby ID、Emby 线路、服务器 API Key、PostgreSQL/Redis 密钥。 |
| 写操作边界 | Bot 不执行删除用户、封禁、踢会话、改密码、系统更新、数据库迁移/恢复等高风险写操作。 |

## 用户命令

| 命令 | 场景 | 说明 |
| ---- | ---- | ---- |
| `/start` | 私聊 | 显示 Bot 入口和常用命令。 |
| `/help` | 私聊 | 显示普通帮助；管理员会额外看到管理员命令提示。 |
| `/twihelp` | 私聊 | `/help` 的别名。 |
| `/about` | 私聊 | 查看服务说明。 |
| `/bind <绑定码>` | 私聊 | 使用 Web 端生成的绑定码完成 Telegram 绑定。 |
| `<绑定码>` | 私聊 | 直接发送 6-16 位字母数字绑定码也可完成绑定。 |
| `/me` | 私聊 | 查看当前 Telegram 绑定的 Twilight 账号摘要。 |
| `/emby` | 私聊 | 查看当前账号 Emby 绑定、本地状态、到期状态和服务器连通性摘要。 |
| `/playinfo` | 私聊 | 查看近 30 天播放次数、总时长和最近播放摘要。 |
| `/resetpwd` | 私聊 | 提示前往 Web 端修改密码；Bot 不生成和发送密码。 |
| `/cancel` | 私聊 | 取消当前 Bot 操作。 |

## 管理员命令

| 命令 | 场景 | 说明 |
| ---- | ---- | ---- |
| `/twishelp` | 私聊 | 查看管理员帮助。 |
| `/admin` | 私聊 | 显示管理员只读查询入口。 |
| `/stats` | 私聊 | 查看用户、Telegram 绑定、Emby 绑定、注册码、邀请码等统计。 |
| `/userinfo <关键词>` | 私聊 | 查询单个用户摘要；支持用户名、UID、Telegram ID、Telegram 用户名、Emby 用户名或 Emby ID 匹配，但结果不展示敏感 ID。 |
| `/twfind <关键词>` | 私聊 | 搜索用户并返回多条非敏感摘要。 |
| `/twguser <关键词>` | 群聊/私聊 | 管理员只读查询用户摘要。 |
| `/twguser` | 群聊回复目标用户消息 | 按目标 Telegram 绑定关系查询对应 Twilight 用户摘要。 |

## 绑定流程

1. 用户在 Web 端生成 Telegram 绑定码。
2. 用户私聊 Bot 发送 `/bind <绑定码>`，或直接发送绑定码。
3. Bot 校验绑定码格式、有效期和已绑定关系。
4. 校验通过后，Bot 写入绑定关系；如果绑定码关联了本地 UID，会同步写入用户的 Telegram 信息。

绑定码仅允许 6-16 位字母数字，且会被转换为大写后校验。群聊内不会处理绑定码。

## 安全边界

- 群聊里不处理账号状态、播放统计、绑定码、管理员统计等敏感命令。
- 管理员查询只展示用户名、UID、角色、启用状态、到期状态、Telegram 是否绑定、Emby 是否绑定、是否待开通 Emby。
- Bot 不展示 Emby ID、Telegram ID、密码、Token、服务线路、数据库连接串。
- `/emby` 只展示是否配置、是否可连通，不展示服务器地址。
- `/playinfo` 只展示播放摘要，不展示外部服务凭据。
- 群聊 `/twguser` 是只读查询，不提供 inline 写操作按钮。

## 文案配置

以下配置项可在 `[Telegram]` 中维护；留空时使用 Go 后端内置文案。

| 配置项 | 说明 |
| ------ | ---- |
| `bot_start_text` | 覆盖私聊 `/start` 完整文案。 |
| `bot_group_start_text` | 覆盖群聊 `/start` 提示。 |
| `bot_start_title` | 内置 `/start` 文案标题。 |
| `bot_start_intro` | 内置 `/start` 简介。 |
| `bot_bind_prompt_text` | `/bind` 无参数时的提示。 |
| `bot_help_text` | 覆盖 `/help` 和 `/twihelp` 完整文案。 |
| `bot_admin_help_text` | 覆盖 `/twishelp` 完整文案。 |
| `bot_help_header` | 追加到内置普通帮助顶部。 |
| `bot_help_footer` | 追加到内置普通帮助底部。 |
| `bot_about` | `/about` 服务说明。 |

支持占位符：`{server_name}`、`{bot_username}`、`{user_name}`。当前 Go Bot 使用纯文本发送，不依赖 Markdown 转义。
