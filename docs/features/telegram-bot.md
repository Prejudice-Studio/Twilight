# Telegram Bot 命令

本文说明 Twilight 内置 Telegram Bot 的命令清单、权限边界、绑定流程与可定制文案。Bot 主要承担绑定、查询、统计和通知职责；密码、系统更新、数据库恢复等高风险操作请在 Web 后台完成。

Bot 由二进制子命令 `bot`（或 `all`）启动，轮询逻辑见 `internal/api/telegram_bot.go`，命令注册表见 `internal/api/telegram_commands.go`，群组内联面板见 `internal/api/telegram_inline.go`，配置字段见 `internal/config/config.go` 的 `[Telegram]` 段。

## 基本规则

| 规则 | 说明 |
| ---- | ---- |
| 私聊优先 | `/bind`、`/me`、`/emby`、`/playinfo`、`/resetpwd`、`/cancel`、`/about` 以及管理员命令 `/stats`、`/admin`、`/userinfo`、`/twfind`、`/twishelp` 仅在私聊生效。 |
| 群聊保护 | 在群聊使用上述账号类命令时，Bot 只回复"请私聊使用"提示，不展示账号状态。`/start`、`/help`、`/twihelp` 在群聊会被替换为群聊提示文案。 |
| 管理员判定 | `telegramAdminID` 判定逻辑：Telegram ID 命中 `[Telegram].admin_id` 列表，或该 Telegram ID 已绑定到一个 `Role == admin` 的 Twilight 账号。 |
| 敏感信息边界 | Bot 不展示密码、Token、Emby ID、Telegram ID、服务器线路/地址、数据库连接串等敏感信息。 |
| 写操作边界 | 私聊命令全部为只读。唯一的写操作入口是群组 `/twguser` 内联面板，且每次按钮点击都重新校验管理员身份（详见后文）。 |
| 命令解析 | 命令统一转小写并剥离 `@botname` 后缀（`telegramCommand`），因此在群里写 `/stats@yourbot` 也能识别。 |

## 用户命令

下列命令均要求私聊（群聊会被拦截或转为提示）。

| 命令 | 说明 |
| ---- | ---- |
| `/start` | 显示 Bot 入口与常用命令；可被 `bot_start_text` 完整覆盖。 |
| `/help` | 显示帮助；管理员会额外看到管理员命令清单。 |
| `/twihelp` | `/help` 的别名，行为完全一致。 |
| `/about` | 查看服务说明。 |
| `/bind <绑定码>` | 使用 Web 端生成的绑定码完成 Telegram 绑定；无参数时回复绑定提示文案。 |
| `<绑定码>` | 私聊里直接发送 6-16 位字母数字绑定码（不带 `/`）也可完成绑定。 |
| `/me` | 查看当前 Telegram 绑定的 Twilight 账号摘要。 |
| `/emby` | 查看账号本地状态、到期、Emby 绑定、服务器是否配置以及连通性（不展示服务器地址）。 |
| `/playinfo` | 查看近 30 天播放次数、总时长与最近 5 条播放摘要。 |
| `/resetpwd` | 提示前往 Web 端修改密码；Bot 不接收、不生成也不发送密码。 |
| `/cancel` | 回复"已取消当前 Bot 操作"。 |

`/emby` 的连通性检测仅在配置了 Emby 地址时进行，结果分为"正常 / 不可用 / 未检测"，不会展示服务器 URL。

## 管理员命令

下列命令要求私聊且通过管理员判定，未授权时统一回复"没有管理员权限。"。

| 命令 | 场景 | 说明 |
| ---- | ---- | ---- |
| `/admin` | 私聊 | 显示管理员只读查询入口列表。 |
| `/stats` | 私聊 | 统计用户总数、活跃用户、Telegram 已绑定、Emby 已绑定、待开通 Emby、注册码数量、邀请码数量。 |
| `/userinfo <关键词>` | 私聊 | 查询单个用户摘要；匹配命中多个时提示缩小关键词。 |
| `/twfind <关键词>` | 私聊 | 搜索用户并返回最多 10 条非敏感摘要列表。 |
| `/twishelp` | 私聊 | 查看管理员帮助文案。 |
| `/twguser <关键词>` | 群聊 / 私聊 | 打开群组用户管理面板（带内联操作按钮）。 |
| `/twguser`（回复目标消息） | 群聊 | 回复某成员消息后发送，按其 Telegram 绑定关系定位对应 Twilight 用户并打开面板。 |

`/userinfo` 与 `/twfind` 的搜索关键词支持用户名、邮箱、UID、Telegram ID、Telegram 用户名、Emby 用户名、Emby ID 进行匹配（`telegramUserMatches`），但返回的摘要不会展示 Telegram ID、Emby ID 等敏感字段。

### 群组用户管理面板（`/twguser` 内联操作）

> 提示：旧文档曾描述群组 `/twguser` 为"只读查询、不提供 inline 写操作按钮"。实际代码（`internal/api/telegram_inline.go`）提供了一组带写操作的内联按钮，下表予以更正。

`/twguser` 命中目标用户后会发送一条带内联键盘的面板消息，可执行以下操作：

| 按钮 | 行为 |
| ---- | ---- |
| 刷新 | 重新拉取并展示用户当前状态。 |
| 启用 / 禁用 Web 账号 | 切换账号 `Active`；如已绑定 Emby 且配置了服务器，会同步调整 Emby 用户启用状态。 |
| 授予注册资格 | 为未绑定 Emby 的用户标记 `PendingEmby` 并设置有效天数（取自邀请默认天数 / 自由注册天数 / 兜底 30 天），仅在用户未绑定 Emby、未待开通且非受保护账号时出现。 |
| 删除用户 / 确认删除用户 | 两步确认；确认后删除用户并清除其会话。 |
| 移出群组 / 封禁群组 | 对已绑定 Telegram 的目标执行群组踢出 / 封禁，仅在目标已绑定 Telegram 时出现。 |

面板安全约束：

- 受保护账号（`Role == admin`，或其 Telegram ID 命中管理员判定）禁止被禁用、删除、移出或封禁。
- 群内匿名管理员（以群身份发言、`from.id` 为 0 或带 `sender_chat`）发送 `/twguser` 时，必须先点击"验证管理员身份"内联按钮完成真实身份校验，才会展示面板。
- 每次按钮点击都会重新执行 `telegramAdminID` 校验；非管理员点击会被拒绝并清理消息。
- 面板有效期为 1 分钟，无操作自动删除；每次操作会刷新过期时间。
- 非管理员或匿名身份发起的越权指令，连同提示消息会在 30 秒后自动删除。

## 绑定流程

1. 用户在 Web 端生成 Telegram 绑定码。
2. 用户私聊 Bot，发送 `/bind <绑定码>`，或直接发送绑定码本身。
3. Bot 将绑定码转为大写并按正则 `^[A-Za-z0-9]{6,16}$` 校验格式（`telegramBindCodePattern`）。
4. 校验绑定码是否存在、是否过期，以及该 Telegram 是否已被其它账号占用。
5. 若开启了强制加群 / 订阅频道（`force_bind_group` / `force_bind_channel`），会校验该 Telegram 是否已加入指定群组 / 频道，未满足会列出待加入的目标。
6. 通过后写入绑定关系；若绑定码关联了本地 UID，则同步把 Telegram ID 与用户名写入该用户记录，并删除一次性绑定码。

绑定码仅接受 6-16 位字母数字并统一转大写后校验。群聊内不会处理裸绑定码。绑定确认对同一 Telegram 账号做了幂等处理，并对单个 Telegram ID 施加每分钟速率限制，避免反复触发群成员校验。

## 安全边界

- 群聊不处理账号状态、播放统计、绑定码、管理员统计等敏感命令，仅 `/twguser` 在群内可用且受上述面板约束保护。
- 管理员查询摘要仅展示用户名、UID、角色、启用状态、到期状态、Telegram 是否绑定、Emby 是否绑定、是否待开通 Emby。
- Bot 不展示 Emby ID、Telegram ID、密码、Token、服务线路、数据库连接串。
- `/emby` 只展示是否配置、是否可连通，不展示服务器地址。
- `/playinfo` 只展示播放摘要，不展示外部服务凭据。
- Bot 处理过程对每条 update 做 panic 隔离，日志中的 panic 文本与敏感内容都会经脱敏处理。

## 文案配置项

下列字段在 `config.toml` 的 `[Telegram]` 段维护，留空时使用 Go 后端内置文案。可在 Web 管理端配置编辑器中修改。

| 配置项 | 对应字段 | 说明 |
| ------ | ------ | ---- |
| `bot_start_text` | `TelegramBotStartText` | 覆盖私聊 `/start` 的完整文案。 |
| `bot_group_start_text` | `TelegramBotGroupStartText` | 覆盖群聊里 `/start`、`/help`、`/twihelp` 的提示文案。 |
| `bot_start_title` | `TelegramBotStartTitle` | 内置 `/start` 文案标题（默认 `Twilight Bot`）。 |
| `bot_start_intro` | `TelegramBotStartIntro` | 内置 `/start` 简介段。 |
| `bot_bind_prompt_text` | `TelegramBotBindPromptText` | `/bind` 无参数时的提示文案。 |
| `bot_help_text` | `TelegramBotHelpText` | 覆盖 `/help` 与 `/twihelp` 的完整文案。 |
| `bot_admin_help_text` | `TelegramBotAdminHelpText` | 覆盖 `/twishelp` 的完整文案。 |
| `bot_help_header` | `TelegramBotHelpHeader` | 追加到内置普通帮助顶部。 |
| `bot_help_footer` | `TelegramBotHelpFooter` | 追加到内置普通帮助底部。 |
| `bot_about` | `TelegramBotAbout` | `/about` 服务说明文案。 |
| `bot_custom_commands` | `TelegramCustomCommands` | 自定义命令应答表（见下）。 |

### 自定义命令

`bot_custom_commands` 允许配置一组"命令 → 固定回复"的映射，命中后直接返回对应文本（`telegramCustomCommandReply`）。每条形如 `命令 = 回复`，命令会被规范化：转小写、补 `/` 前缀、仅允许字母数字与下划线、长度不超过 32 字符（`normalizeTelegramCommand`），重复命令以首次出现为准。自定义命令在内置命令之后匹配，不会覆盖内置命令。

### 文案占位符

自定义文案支持以下占位符（`telegramRenderText`）：

| 占位符 | 当前替换值 |
| ------ | ---- |
| `{server_name}` | 应用名称（`AppName`）。 |
| `{bot_username}` | 当前替换为空字符串。 |
| `{user_name}` | 当前替换为空字符串。 |

Go Bot 使用纯文本发送消息，不依赖 Markdown 转义。

## 相关配置与扩展

强制加群 / 订阅、退群封禁（`ban_on_leave`）、重新入群自动恢复（`auto_enable_rejoined`）、群成员校验并发度等行为属于 Bot 运行策略而非命令，配置字段集中在 `[Telegram]` 段，详见 [Go 后端架构与配置](../reference/backend.md)。其它功能文档参见 [文档导航](../README.md)。
