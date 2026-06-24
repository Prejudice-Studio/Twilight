# Telegram Bot 命令

本文说明 Twilight 内置 Telegram Bot 的命令清单、权限边界、绑定流程与可定制文案。Bot 主要承担绑定、查询、统计和通知职责；密码、系统更新、数据库恢复等高风险操作请在 Web 后台完成。

Bot 由二进制子命令 `bot`（或 `all`）启动，轮询逻辑见 `internal/api/telegram_bot.go`，命令注册表见 `internal/api/telegram_commands.go`，群组内联面板见 `internal/api/telegram_inline.go`，配置字段见 `internal/config/config.go` 的 `[Telegram]` 段。

## 基本规则

| 规则 | 说明 |
| ---- | ---- |
| 私聊优先 | `/bind`、`/me`、`/emby`、`/playinfo`、`/resetpwd`、`/cancel`、`/about` 以及管理员命令 `/stats`、`/admin`、`/userinfo`、`/twfind`、`/twishelp` 仅在私聊生效。 |
| 群聊保护 | 在群聊使用上述账号类命令时，Bot 只回复"请私聊使用"提示，不展示账号状态。`/start`、`/help`、`/twihelp` 在群聊会被替换为群聊提示文案。 |
| 管理员判定 | `telegramAdminID` 判定逻辑：Telegram ID 命中 `[Telegram].admin_id` 列表，或该 Telegram ID 已绑定到一个 `Role == admin` 的 Twilight 账号。 |
| 敏感信息边界 | Bot 默认不展示密码、Token、Emby ID、Telegram ID、服务器线路/地址、数据库连接串等敏感信息；管理员自定义 `/twguser` 模板时可显式使用 `{telegram_userid}`。 |
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
| `/delAccount [原因]` | 删除自己的账号。验证优先级：已绑定邮箱→邮箱验证码；已绑定 Emby→Web密码+Emby密码两步验证；无绑定→直接确认。可附带可选删除原因（记录到审计日志）。禁用状态的 Web/Emby 账号不可自删。示例：`/delAccount 不再使用本服务` 或 `/delAccount emby`。 |

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
| `/banweb <用户> [理由]` | 私聊 | 禁用指定用户的 Web 账号（可选理由，记入操作日志）。受保护账号不可操作。 |
| `/banemby <用户> [理由]` | 私聊 | 单独禁用指定用户的 Emby 账号，不影响 Web 账号（可选理由，记入操作日志）。受保护账号及未绑定 Emby 的用户不可操作。 |
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
| 授予 7 天 / 30 天 / 365 天 / 永久 | 为未绑定 Emby 的用户标记 `PendingEmby`，写入 `emby_grant_locked=true`，并设置对应待补建天数；仅在用户未绑定 Emby、未待开通且非受保护账号时出现。 |
| 删除用户 / 确认删除用户 | 两步确认；确认后删除用户并清除其会话。 |
| 移出群组 / 封禁群组 | 对已绑定 Telegram 的目标执行群组踢出 / 封禁，仅在目标已绑定 Telegram 时出现。 |

面板安全约束：

- 受保护账号（`Role == admin`，或其 Telegram ID 命中管理员判定）禁止被禁用、删除、移出或封禁。
- 群内匿名管理员（以群身份发言、`from.id` 为 0 或带 `sender_chat`）发送 `/twguser` 时，必须先点击"验证管理员身份"内联按钮完成真实身份校验，才会展示面板。
- 每次按钮点击都会重新执行 `telegramAdminID` 校验；非管理员点击会被拒绝并清理消息。
- 面板有效期为 1 分钟，无操作自动删除；每次操作会刷新过期时间。
- 非管理员或匿名身份发起的越权指令，连同提示消息会在 30 秒后自动删除。
- 删除 Emby 账号类操作会尊重用户记录上的 `emby_grant_locked`。通过注册码、白名单码、邀请码、后台授予、Telegram 授予或自助创建获得过 Emby 注册资格的账号，不能通过面板删除 Emby 后再次自助注册。

面板文本可通过 `[Telegram].group_user_panel_template` 自定义，也可在 Web 后台配置页的 Telegram 分组中编辑。留空使用内置模板；未知占位符会原样保留，便于发现拼写错误。模板不提供邮箱、Emby ID、密码、Token 或服务器线路占位符；如确需展示 Telegram ID，可显式使用 `{telegram_userid}`。

常用占位符：

| 占位符 | 含义 |
| ---- | ---- |
| `{server_name}` | 站点名称。 |
| `{username}` / `{uid}` | Web 用户名 / UID。 |
| `{role}` / `{role_id}` | 角色名称 / 角色数字。 |
| `{is_admin}` / `{is_protected}` | 是否管理员 / 是否受保护账号。 |
| `{web_status}` / `{web_active}` | Web 账号启用状态。 |
| `{expire_status}` / `{expired_at}` | 到期摘要 / 具体到期时间。 |
| `{register_time}` / `{created_at}` | 注册时间 / 创建时间。 |
| `{telegram_status}` / `{telegram_username}` / `{telegram_userid}` | Telegram 绑定摘要 / Telegram 用户名 / Telegram 用户 ID。无用户名时 `{telegram_username}` 显示 `None`。 |
| `{emby_status}` / `{emby_username}` | 本地 Emby 绑定摘要 / 本地 Emby 用户名。 |
| `{emby_bound_status}` / `{emby_bound}` | 本地 Emby 绑定状态 / 是否已绑定。 |
| `{emby_unbind_allowed}` | 是否允许用户自助解绑 Emby。 |
| `{pending_emby}` / `{pending_emby_days}` | 是否待补建 Emby / 待补建授权天数。 |
| `{registration_source}` / `{registration_code}` | Emby 注册资格来源 / 对应卡码。 |
| `{emby_remote_block}` | 完整 Emby 远端信息块，包含远端用户名、启用状态、权限、隐藏状态与最近活动。 |
| `{emby_remote_status}` / `{emby_remote_username}` | 远端查询状态 / 远端用户名。 |
| `{emby_remote_enabled}` / `{emby_remote_role}` / `{emby_remote_hidden}` | 远端启用状态 / 远端权限 / 是否隐藏。 |
| `{emby_last_activity}` | 远端最近活动时间。 |
| `{bgm_mode}` / `{bgm_token_status}` / `{bgm_sync_status}` | BGM 同步开关 / Token 是否配置 / 同步可用状态。 |
| `{api_key_status}` | 旧 API Key 开关。 |
| `{panel_ttl}` / `{panel_ttl_seconds}` | 面板有效期文本 / 秒数。 |

## 绑定流程

1. 用户在 Web 端生成 Telegram 绑定码。
2. 用户私聊 Bot，发送 `/bind <绑定码>`，或直接发送绑定码本身。
3. Bot 将绑定码转为大写并按正则 `^[A-Za-z0-9]{6,16}$` 校验格式（`telegramBindCodePattern`）。
4. 校验绑定码是否存在、是否过期，以及该 Telegram 是否已被其它账号占用。
5. 若开启了强制加群 / 订阅频道（`force_bind_group` / `force_bind_channel`），会校验该 Telegram 是否已加入指定群组 / 频道，未满足会列出待加入的目标。
6. 通过后写入绑定关系；若绑定码关联了本地 UID，则同步把 Telegram ID 与用户名写入该用户记录，并删除一次性绑定码。

绑定码仅接受 6-16 位字母数字并统一转大写后校验。群聊内不会处理裸绑定码。绑定确认对同一 Telegram 账号做了幂等处理，并对单个 Telegram ID 施加每分钟速率限制，避免反复触发群成员校验。

### Telegram 用户名自动刷新

用户在 Telegram 改了 `@username` 后，已绑定账号里存的 `telegram_username` 会过时。`observeTelegramRoster`（`internal/api/telegram_bot.go`）在处理**任意**来自已绑定用户的更新（私聊 / 群消息 / `chat_member` 事件）时，顺手调用 `refreshTelegramUsername` 被动刷新：仅当能解析到绑定账号、且新用户名非空并与现存不同才写库（无额外 API 调用）；用户名为空（对方删了 `@username`）时保留旧值不清空，以免破坏指名注册码的用户名匹配。此外 `/twguser` 面板渲染时也会经 `getChatMember` 做一次按需刷新。

## 安全边界

- 群聊不处理账号状态、播放统计、绑定码、管理员统计等敏感命令，仅 `/twguser` 在群内可用且受上述面板约束保护。
- 管理员查询摘要仅展示用户名、UID、角色、启用状态、到期状态、Telegram 是否绑定、Emby 是否绑定、是否待开通 Emby。
- Bot 默认模板不展示 Emby ID、Telegram ID、密码、Token、服务线路、数据库连接串；管理员自定义 `/twguser` 模板时可显式使用 `{telegram_userid}`。
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
| `group_user_panel_template` | `TelegramGroupUserPanelTemplate` | 覆盖 `/twguser` 群组用户面板文本，支持上方用户占位符。 |
| `bot_custom_commands` | `TelegramCustomCommands` | 自定义命令应答表（见下）。 |

### 自定义命令

Web 后台的入口是「Telegram 管理 → Bot 指令管理」（`/admin/telegram/commands`）。该页面只编辑 `Telegram.bot_custom_commands`，不会创建第二套 Bot 指令配置：

- 纯文本类型保存为普通回复，运行时只经过 `telegramRenderText` 的基础占位符替换，例如 `{server_name}`、`{bot_username}`、`{user_name}`。
- 自定义 JS 类型从「开发者模式」保存的 JS 预设中选择，推荐保存为 `js:preset:<id>` 动态引用格式。
- 开发者模式支持新建空白 JS 预设、命名、保存、更新和删除；非空脚本保存前必须通过与沙箱预览一致的安全校验。
- Bot 执行时仍只读取 `bot_custom_commands`。使用 `js:preset:<id>` 时会在执行时按预设 ID 读取最新代码；旧格式 `js:<code>` 属于静态代码快照，只有重新保存指令才会改变。

`bot_custom_commands` 允许配置一组"命令 → 固定回复"的映射，命中后直接返回对应文本（`telegramCustomCommandReply`）。每条形如 `命令 = 回复`，命令会被规范化：转小写、补 `/` 前缀、仅允许字母数字与下划线、长度不超过 32 字符（`normalizeTelegramCommand`），重复命令以首次出现为准。自定义命令在内置命令之后匹配，不会覆盖内置命令。

开发者模式启用后，可把某条回复写成 `js:` 前缀脚本，让 Bot 在受控 Goja（`github.com/dop251/goja`）沙箱中执行。脚本同步执行，单次运行 8 秒墙钟超时；独立页面 `/admin/developer/js-docs` 会通过 `GET /admin/developer/js-docs` 拉取完整接口文档，并以类似 Swagger 的方式展示内置对象、命名空间、函数、配置键、环境变量和示例。

```toml
[Telegram]
bot_custom_commands = [
  "/hello = js:reply('Hello ' + (user.username || 'user'))",
]
```

沙箱只暴露以下绑定：

| 绑定 | 说明 |
| ---- | ---- |
| `ctx` | 当前 Telegram 上下文摘要：`private_chat`、`command_time`、`preview`。不向脚本暴露 Telegram ID 或群组 ID。 |
| `args` | 命令参数数组。 |
| `user` | 绑定的 Twilight 用户脱敏摘要：`uid`、`username`、`email`、`email_masked`、`role`、`active`、`has_emby`、`email_verified`、`telegram_bound`、`telegram_id`、`telegram_username`、登录通知开关等。不会注入密码哈希、Token、API Key、BGM Token 明文、Emby 内部 ID 或数据库连接信息。 |
| `constants` | 受控常量：`roles.admin/user/whitelist`、`limits.max_replies/max_logs`。 |
| `users` | 当前 Telegram 绑定用户的受控接口：`current()` / `describe()` 脱敏读取，`hasRole(role)` / `requireActive()` 判断，`setLoginNotify({ telegram?, email? })` 仅修改当前用户登录通知偏好。 |
| `text` | 文本辅助函数：`truncate(value, max)`、`joinLines(values)`、`escape(value)`、`numberLines(values)`。 |
| `arrays` | 数组辅助函数：`first(values)`、`compact(values)`、`unique(values)`、`take(values, count)`。 |
| `time` | 时间辅助函数：`now()`、`formatUnix(ts)`。 |
| `interactions` | Telegram 交互辅助函数：`inline(text, actions)` 发送静态 inline keyboard，`waitText(options)` 等待同一用户在限定时间内发送下一条普通文本。 |
| `reply(text)` | 追加一段回复文本，最多 4 段，最终用换行合并发送。 |
| `exit(text?)` | 正常提前结束脚本；传入文本时会先追加一段回复。 |
| `assert(condition, text?)` | 条件为真时继续，条件为假时追加提示并正常退出。 |
| `log(text)` | 写入本次执行的审计详情，最多 8 条。 |
| `auth(role)` | 角色鉴权辅助函数。`admin` 仅管理员；`whitelist` 包含管理员和白名单；`user` 包含所有有效角色。 |
| `config(key)` | 只读白名单系统配置读取。只返回非敏感键，未允许或敏感键返回空字符串并写入沙箱日志。 |
| `env(key)` | 只读白名单环境变量读取。只返回非敏感 `TWILIGHT_*` 键，未允许或敏感键返回空字符串并写入沙箱日志。 |

安全约束：

- `fetch` 是受限同步能力，仅支持公开 `http/https` 的 `GET` / `POST` / `HEAD`；会阻断 localhost、内网、链路本地目标、跳转和凭据。仍不提供 `require`、文件系统或进程能力；配置与环境变量只能通过白名单函数读取非敏感值。
- Token、Secret、密码、API Key、数据库 URL、服务器线路等敏感信息不会注入沙箱，也不会通过 `config` / `env` 返回。
- 后端会静态拒绝危险 token，并用 8 秒墙钟超时中断长循环或卡死脚本。
- `getUser(uid)` / `users.get(uid)` / `users.byUID(uid)` 只支持按精确 UID 读取脱敏快照：普通用户只能读取自己，读取其他用户必须当前 Telegram 绑定用户为管理员；不会返回密码、Token、API Key、BGM Token 明文、Emby 内部 ID 或数据库连接信息。
- `users.search(query, limit)` / `users.list(options)` / `admin.*` 支持管理员受控搜索、列表和单用户写操作；普通用户只能读取自己。开发者模式预览中 `ctx.preview=true`，写操作只返回 `dry_run=true`，不会写入用户数据。
- `regcodes.generate` / `invites.generate` / `announcements.create`（及对应的 `admin.generateRegcode` / `admin.generateInviteCode` / `admin.createAnnouncement`）只允许管理员调用，预览模式只返回 `dry_run=true` 不写入；成功写入会分别记录 `telegram_js_regcode_generate`、`telegram_js_invite_generate`、`telegram_js_announcement_create` 审计日志，来源标记 `telegram_js`。邀请码生成受 `invite_enabled` 功能开关约束。
- 每次执行都会写入 `telegram_js_command_execute` 审计日志；开发者页面的沙箱预检写入 `developer_js_sandbox_preview`。
- Bot 实际执行 `users.setLoginNotify` 成功写入时，会额外记录 `telegram_js_user_notify_update`。
- `interactions.inline` 的 callback 只接受创建该消息的同一 Telegram 用户、同一 chat、同一 message，默认 2 分钟过期；callback 动作只能使用预定义 `answer` / `edit` / `reply` 静态文本，不会再次执行 JS。
- `interactions.waitText` 只消费同一 chat、同一 Telegram 用户的下一条非 `/` 命令文本；等待窗口限制为 1-60 秒，回复内容会截断、脱敏，并在消费后写入 `telegram_js_interaction_wait_text` 审计日志。
- 纯文本自定义命令保持原行为；只有 `js:` 前缀会启用脚本执行，避免破坏历史配置。

#### JS API 参数速查

完整、实时的函数参数、返回结构和示例以后台独立页面 `/admin/developer/js-docs` 为准；该页面由 `GET /admin/developer/js-docs` 返回结构化文档，管理员鉴权后可查看。下表用于离线阅读时快速定位常用函数：

| 函数 | 参数 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `reply(text)` | `text: string` | `void` | 追加一段回复，最多 4 段，发送前会截断和脱敏。 |
| `exit(text?)` | `text?: string` | `never` | 正常提前结束脚本；可选追加一段回复，不会按运行错误处理。 |
| `assert(condition, text?)` | `condition: any`, `text?: string` | `boolean\|never` | 条件为假时追加提示并调用 `exit()`，适合参数和权限前置校验。 |
| `log(text)` | `text: string` | `void` | 追加本次执行日志，最多 8 条，不要写入敏感信息。 |
| `auth(role)` | `role: string\|number` | `boolean` | 检查当前绑定用户角色，支持 `admin`、`whitelist`、`user` 或数字角色。 |
| `authAdmin()` | 无 | `boolean` | 判断当前绑定用户是否管理员。 |
| `getUser(uid)` | `uid: number\|string` | `UserSnapshot\|null` | 按精确 UID 读取脱敏用户快照；跨用户读取需要管理员。 |
| `config(key)` | `key: string` | `string\|number\|boolean` | 读取白名单内非敏感配置。 |
| `env(key)` | `key: string` | `string` | 读取白名单内非敏感 `TWILIGHT_*` 环境变量。 |
| `fetch(url, options)` | `url: string`, `options.method?: GET\|POST\|HEAD` | `{ ok, status, text, error, blocked }` | 受限同步请求；阻断本机、内网、跳转和凭据。 |
| `setTimeout(fn, ms)` / `setInterval(fn, ms)` | `fn: function`, `ms?: number` | `number` | 兼容包装器；回调会在同一次执行中同步运行，不创建异步任务。 |
| `input.arg(index, fallback)` | `index: number`, `fallback?: string` | `string` | 读取指定位置参数。 |
| `input.has(index)` | `index: number` | `boolean` | 判断指定位置参数是否存在且非空。 |
| `input.flag(name)` | `name: string` | `boolean` | 判断是否存在 `--name` 或 `-name`。 |
| `input.named(name, fallback)` | `name: string`, `fallback?: string` | `string` | 读取 `--name=value`、`--name value`、`-name=value` 或 `-name value`。 |
| `db.schema()` | 无 | `object` | 返回受控集合结构和允许字段，不暴露原始 state。 |
| `db.collections()` | 无 | `string[]` | 返回受控集合名。 |
| `db.count(name)` | `name: string` | `number` | 返回允许的集合计数；无权限返回 `-1`。 |
| `db.currentUser()` / `users.current()` | 无 | `UserSnapshot` | 返回当前绑定用户脱敏快照。 |
| `db.getUser(uid)` / `users.get(uid)` / `users.byUID(uid)` | `uid: number\|string` | `UserSnapshot\|null` | 按精确 UID 读取脱敏快照。 |
| `db.findUsers(query, limit)` / `users.search(query, limit)` / `users.find(query, limit)` | `query: string`, `limit?: number` | `UserSnapshot[]` | 管理员搜索，最多 50 条；`find` 为 `search` 简化别名。 |
| `users.exists(uid)` | `uid: number\|string` | `boolean` | 该 UID 是否存在；跨用户查询需管理员，否则 `false`。 |
| `db.listUsers(options)` / `users.list(options)` | `limit?`, `offset?`, `role?`, `active?` | `UserSnapshot[]` | 管理员可分页/筛选；普通用户仅返回自己。 |
| `db.listRegcodes(options)` | `limit?`, `offset?` | `RegCodeSnapshot[]` | 管理员专用，脱敏注册码快照，不含用户密钥。 |
| `db.listInviteCodes(options)` | `limit?`, `offset?` | `InviteCodeSnapshot[]` | 管理员看全部；普通用户只看自己拥有的邀请码。 |
| `db.listMediaRequests(options)` | `limit?`, `offset?` | `MediaRequestSnapshot[]` | 管理员看全部；普通用户只看自己的求片记录。 |
| `db.listAnnouncements(options)` | `limit?`, `offset?` | `AnnouncementSnapshot[]` | 可见公告快照，不含正文。 |
| `db.listTickets(options)` | `limit?`, `offset?` | `TicketSnapshot[]` | 管理员看全部；普通用户只看自己的工单，不含正文。 |
| `db.listPresets(options)` | `limit?`, `offset?` | `PresetSnapshot[]` | 管理员专用，开发者 JS 预设元数据，仅含 `code_length`。 |
| `db.updateCurrentUser(patch)` | `notify_on_login_telegram?`, `notify_on_login_email?`, `telegram?`, `email?` | `{ ok, dry_run?, user?, error? }` | 只修改当前用户登录通知偏好。 |
| `users.setLoginNotify(options)` | `telegram?: boolean`, `email?: boolean` | `{ ok, dry_run?, user?, error? }` | `db.updateCurrentUser` 的便捷形式。 |
| `users.setActive(uid, active)` / `admin.setActive(uid, active)` | `uid`, `active: boolean` | `{ ok, dry_run?, user?, error? }` | 管理员启停 Web 账号，带最后管理员保护。 |
| `users.enable(uid)` / `users.disable(uid)` | `uid` | `{ ok, dry_run?, user?, error? }` | `setActive(uid, true/false)` 的简化别名。 |
| `users.setRole(uid, role)` / `admin.setRole(uid, role)` | `uid`, `role: number` | `{ ok, dry_run?, user?, error? }` | 管理员修改角色。 |
| `users.setExpiry(uid, expiredAt)` / `admin.setExpiry(uid, expiredAt)` | `uid`, `expiredAt: number` | `{ ok, dry_run?, user?, error? }` | 管理员修改到期时间，`-1` 表示永久。 |
| `users.extend(uid, days)` | `uid`, `days: number` | `{ ok, dry_run?, uid?, expired_at?, note?, error? }` | 在当前到期（或现在，取较晚者）上顺延 `days` 天；永久用户原样返回。 |
| `users.update(uid, patch)` / `admin.updateUser(uid, patch)` | `uid`, `patch` | `{ ok, dry_run?, user?, error? }` | 管理员组合更新受控字段。 |
| `admin.ok()` / `admin.ensure()` | 无 | `boolean` | 管理员快捷判断；`ensure()` 会在失败时写沙箱日志。 |
| `admin.searchUsers(query, limit)` / `admin.listUsers(options)` | 同 `users.search/list` | `UserSnapshot[]` | 管理员快捷读接口。 |
| `regcodes.list(options)` | `limit?`, `offset?` | `RegCodeSnapshot[]` | 管理员专用，等价 `db.listRegcodes`。 |
| `regcodes.get(code)` | `code: string` | `RegCodeSnapshot\|null` | 管理员按精确码值查询脱敏快照。 |
| `regcodes.generate(options)` / `admin.generateRegcode(options)` | `count?`, `type?`, `days?`, `use_count_limit?`, `validity_time?`, `note?`, `target_username?`, `decoy?` | `{ ok, dry_run?, codes?, count?, type?, days?, error? }` | 管理员批量生成注册/续期/白名单码，写审计日志。 |
| `regcodes.quick(days?, count?, type?)` | `days?`, `count?`, `type?` | 同 `regcodes.generate` | `generate` 的位置参数简化别名。 |
| `invites.list(options)` | `limit?`, `offset?` | `InviteCodeSnapshot[]` | 管理员看全部；普通用户看自己的邀请码。 |
| `invites.generate(options)` / `admin.generateInviteCode(options)` | `days?`, `expires_at?`, `note?`, `target_username?` | `{ ok, dry_run?, code?, invite?, days?, error? }` | 管理员生成邀请码（需 `invite_enabled`），写审计日志。 |
| `invites.quick(days?)` | `days?` | 同 `invites.generate` | `generate` 的位置参数简化别名（需 `invite_enabled`）。 |
| `announcements.list(options)` | `limit?`, `offset?` | `AnnouncementSnapshot[]` | 可见公告快照，不含正文。 |
| `announcements.create(options)` / `admin.createAnnouncement(options)` | `title?`, `content?`, `level?`, `render_mode?`, `visible?`, `pinned?`, `expires_at?` | `{ ok, dry_run?, announcement?, error? }` | 管理员创建公告，写审计日志。 |
| `announcements.post(title, content, level?)` | `title`, `content`, `level?` | 同 `announcements.create` | `create` 的位置参数简化别名。 |
| `admin.stats()` / `system.stats()` | 无 | `object` | 返回安全聚合统计；管理员可看到更多计数。 |
| `system.info()` | 无 | `object` | 返回安全系统元信息、功能开关和限制。 |
| `system.feature(key)` | `key: string` | `boolean` | 读取一个安全功能开关。 |
| `text.truncate(value, max)` | `value`, `max?: number` | `string` | 按字符数截断。 |
| `text.joinLines(values)` | `values: any[]` | `string` | 数组转多行文本。 |
| `text.escape(value)` | `value` | `string` | 转义基础 HTML 敏感字符。 |
| `text.numberLines(values)` | `values: any[]` | `string` | 数组转编号列表。 |
| `text.trim/lower/upper(value)` | `value` | `string` | 常用字符串整理。 |
| `text.contains(value, needle)` | `value`, `needle` | `boolean` | 大小写不敏感包含判断。 |
| `text.split(value, separator)` | `value`, `separator?: string` | `string[]` | 拆分字符串。 |
| `text.maskEmail(email)` | `email: string` | `string` | 按后端规则脱敏邮箱。 |
| `text.template(template, data)` | `template: string`, `data: object` | `string` | 替换 `{key}` 占位符。 |
| `arrays.first/last(values)` | `values: any[]` | `any\|undefined` | 取首项或末项。 |
| `arrays.compact(values)` | `values: any[]` | `any[]` | 移除 `null` 和空字符串。 |
| `arrays.unique(values)` | `values: any[]` | `string[]` | 字符串化后去重。 |
| `arrays.take(values, count)` | `values`, `count` | `any[]` | 截取前 N 项。 |
| `arrays.join(values, separator)` | `values`, `separator?: string` | `string` | 数组转字符串。 |
| `arrays.includes(values, value)` | `values`, `value` | `boolean` | 精确字符串包含判断。 |
| `arrays.sortStrings(values)` | `values` | `string[]` | 返回排序后的字符串数组副本。 |
| `time.now()` | 无 | `number` | 当前 Unix 秒。 |
| `time.formatUnix(ts)` | `ts: number` | `string` | Unix 秒转 UTC RFC3339。 |
| `time.fromNow(seconds)` | `seconds: number` | `number` | 当前时间偏移秒数。 |
| `time.addDays(ts, days)` | `ts`, `days` | `number` | 给时间戳增加天数，`ts<=0` 时以当前时间为基准。 |
| `time.duration(seconds)` / `format.duration(seconds)` | `seconds` | `string` | 格式化时长。 |
| `format.bool(value, yes, no)` | `value`, `yes?`, `no?` | `string` | 布尔值转文本。 |
| `format.role(role)` | `role: number` | `string` | 角色 ID 转角色名。 |
| `format.date(ts)` | `ts: number` | `string` | 时间戳转日期文本。 |
| `format.expiry(expiredAt)` | `expiredAt: number` | `string` | 用户到期时间转状态文本。 |
| `format.user(user)` | `UserSnapshot` | `string` | 用户快照转一行摘要。 |
| `format.json(value)` | `value` | `string` | 返回受限长度字符串；结构化 JSON 优先用 `JSON.stringify`。 |
| `interactions.inline(text, actions)` | `text`, `actions[]` | `{ ok, dry_run?, message_id?, actions?, error? }` | 发送静态 inline keyboard。 |
| `interactions.waitText(options)` | `seconds?`, `prompt?`, `reply_prefix?`, `timeout_reply?`, `max_chars?`, `numbered?` | `{ ok, dry_run?, seconds?, error? }` | 等待同一用户下一条普通文本。 |

`UserSnapshot` 主要字段：`uid`、`username`、`email`、`email_masked`、`has_email`、`role`、`role_name`、`active`、`expired_at`、`expire_status`、`created_at`、`register_time`、`has_emby`、`emby_username`、`emby_disabled`、`avatar`、`background`、`bgm_mode`、`bgm_token_set`、`email_verified`、`email_verified_at`、`telegram_bound`、`telegram_id`、`telegram_username`、`notify_on_login_telegram`、`notify_on_login_email`、`legacy_api_key_enabled`、`rebinding_in_progress`、`rebinding_since`。不会包含密码、Token、API Key、BGM Token 明文、Emby 内部 ID、原始数据库状态或数据库连接信息。

常用示例：

```js
// 查看用户输入指令时可读取的全部非敏感上下文。
// 不提供 chat ID、message ID、群组 ID、Emby 内部 ID、Token、API Key 或密码。
const me = users.current();
const lines = [
  "private_chat=" + ctx.private_chat,
  "preview=" + ctx.preview,
  "command_time=" + time.formatUnix(ctx.command_time),
  "args=" + JSON.stringify(args),
  "uid=" + me.uid,
  "username=" + (me.username || "unbound"),
  "role=" + me.role,
  "active=" + me.active,
  "has_emby=" + me.has_emby,
  "email_verified=" + me.email_verified,
  "telegram_bound=" + me.telegram_bound,
  "notify_tg=" + me.notify_on_login_telegram,
  "notify_email=" + me.notify_on_login_email
];
reply(text.truncate(text.joinLines(lines), 1200));
```

```js
// 使用 assert/exit 做参数守卫和正常提前退出。
assert(input.has(0), "Usage: /lookup <uid>");
const uid = Number(input.arg(0));
if (!uid) {
  exit("UID must be a number");
}
const target = getUser(uid);
if (!target) {
  exit("User not found or permission denied");
}
reply(format.user(target));
```

```js
// 查看当前绑定用户摘要（可读取邮箱/Telegram 脱敏联系信息；不会返回 Emby 内部 ID、Token、API Key 或密码）
const me = users.current();
reply("User: " + (me.username || "unbound") + "\nActive: " + me.active);
```

```js
// 管理员按精确 UID 查看脱敏用户摘要；非管理员不能跨用户读取
if (!auth("admin")) {
  reply("Admin only");
  return;
}
const target = getUser(Number(args[0] || 0));
if (!target) {
  reply("User not found or permission denied");
  return;
}
reply([
  "UID: " + target.uid,
  "Username: " + target.username,
  "Active: " + target.active,
  "Has Emby: " + target.has_emby,
  "Email verified: " + target.email_verified
].join("\n"));
```

```js
// 开启当前绑定用户的 Telegram 登录通知；预览模式只 dry-run
const result = users.setLoginNotify({ telegram: true });
reply(result.dry_run ? "Preview only" : "Telegram login notifications enabled");
```

```js
// 清理参数并输出
const values = arrays.unique(arrays.compact(args));
reply(text.truncate(text.joinLines(values), 120));
```

```js
// 发送静态 inline 操作。点击后只执行预设 answer/edit/reply，不会再次运行 JS。
interactions.inline("Choose an action", [
  { text: "Status", answer: "OK", edit: "Status acknowledged" },
  { text: "Help", reply: "Use /help for commands" }
]);
```

```js
// 等待同一用户 30 秒内发送下一条普通文本，并以编号形式回复前 120 个字符
interactions.waitText({
  seconds: 30,
  prompt: "Send one line in 30 seconds",
  reply_prefix: "Received:",
  max_chars: 120,
  numbered: true
});
```

更多示例：

```js
// 回显参数、flag 和命名选项。示例命令：/tool ping --uid 10001 --force
const lines = [
  "command=" + input.command,
  "first=" + input.arg(0, "none"),
  "has_second=" + input.has(1),
  "force=" + input.flag("force"),
  "uid=" + input.named("uid", "missing"),
  "text=" + input.text
];
reply(text.joinLines(lines));
```

```js
// 使用模板生成当前用户摘要。
reply(text.template("Hi {name}\nUID: {uid}\nEmail: {email}\nRole: {role}\nExpiry: {expiry}", {
  name: user.username || "unbound",
  uid: user.uid,
  email: user.email_masked || "none",
  role: user.role_name,
  expiry: user.expire_status
}));
```

```js
// 查询当前用户邮箱和登录通知状态。
const me = users.current();
reply([
  "email=" + (me.email_masked || "none"),
  "verified=" + format.bool(me.email_verified, "yes", "no"),
  "notify_email=" + format.bool(me.notify_on_login_email, "on", "off"),
  "notify_tg=" + format.bool(me.notify_on_login_telegram, "on", "off")
].join("\n"));
```

```js
// 用 on/off 参数开关当前用户登录通知。
const enable = input.flag("on") || text.lower(input.first) === "on";
const disable = input.flag("off") || text.lower(input.first) === "off";
if (!enable && !disable) {
  reply("Usage: /notify on|off");
  return;
}
const result = users.setLoginNotify({ telegram: enable, email: enable });
reply(result.dry_run ? "Preview only" : ("Notifications " + (enable ? "enabled" : "disabled")));
```

```js
// 管理员按关键词搜索用户，输出前 5 条摘要。
if (!admin.ensure()) return;
const query = input.named("q", input.text);
const rows = admin.searchUsers(query, 5);
if (!rows.length) {
  reply("No users matched: " + query);
  return;
}
reply(text.numberLines(rows.map(function(u) {
  return format.user(u);
})));
```

```js
// 管理员分页列出启用中的普通用户。
if (!admin.ensure()) return;
const rows = admin.listUsers({
  limit: 10,
  offset: Number(input.named("offset", 0)),
  role: roles.user,
  active: true
});
reply(rows.length ? text.numberLines(rows.map(format.user)) : "No users");
```

```js
// 管理员按天数设置用户到期时间。示例：/setexp --uid 10001 --days 30
if (!admin.ensure()) return;
const uid = Number(input.named("uid", 0));
const days = Number(input.named("days", 7));
if (!uid || days < 1 || days > 3650) {
  reply("Usage: /setexp --uid 10001 --days 30");
  return;
}
const result = admin.setExpiry(uid, time.addDays(time.now(), days));
reply(result.ok ? ("New expiry: " + format.expiry(result.user.expired_at)) : ("Failed: " + result.error));
```

```js
// 管理员禁用用户前要求显式 --confirm，避免误触。
if (!admin.ensure()) return;
const uid = Number(input.named("uid", 0));
if (!uid) {
  reply("Usage: /disable --uid 10001 --confirm");
  return;
}
if (!input.flag("confirm")) {
  const target = users.get(uid);
  reply("Preview: would disable " + (target ? format.user(target) : ("#" + uid)) + "\nAdd --confirm to execute.");
  return;
}
const result = admin.setActive(uid, false);
reply(result.ok ? ("Disabled #" + uid) : ("Failed: " + result.error));
```

```js
// 受限 fetch 读取公开 JSON。注意：内网、localhost、跳转和凭据都会被阻断。
const res = fetch("https://example.com/status.json");
if (!res.ok) {
  reply("fetch failed: " + (res.error || res.status));
  return;
}
try {
  const data = JSON.parse(res.text);
  reply("status=" + (data.status || "unknown"));
} catch (e) {
  reply("invalid json: " + text.truncate(res.text, 120));
}
```

```js
// 复杂示例：搜索用户、统计子集并输出有界摘要。
if (!admin.ensure()) return;
const query = input.named("q", input.text);
const rows = admin.searchUsers(query, 20);
const active = rows.filter(function(u) { return u.active; }).length;
const withEmail = rows.filter(function(u) { return u.has_email; }).length;
const preview = arrays.take(rows.map(function(u) {
  return "#" + u.uid + " " + (u.username || "unknown") + " " + format.bool(u.active, "on", "off") + " " + (u.email_masked || "no-email");
}), 10);
reply(text.truncate(text.joinLines([
  "query=" + query,
  "matched=" + rows.length,
  "active=" + active,
  "email_bound=" + withEmail,
  "---",
  text.numberLines(preview)
]), 1200));
```

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
### 开发者 JS 指令补充

- 开发者模式由仪表盘输入 `DEBUGMODE` 后二次验证管理员密码开启；再次输入 `DEBUGMODE` 并验证会关闭。关闭后服务端会阻断所有 `js:` / `js:preset:<id>` 指令、inline callback 和 waitText 等 JS 交互，但不会删除 JS 预设或 Telegram 指令配置。纯文本自定义命令继续可用。
- 推荐在 Telegram 管理的 Bot 指令中保存 `js:preset:<id>`。该格式动态引用开发者模式中保存的预设，预设更新后已绑定指令会读取最新代码；旧格式 `js:<code>` 仍兼容，但属于静态代码快照。
- JS 运行时会自动注入 `ctx`、`command`、`args`、`user`、`constants`、`users`、`db`、`text`、`arrays`、`time`、`interactions`、`reply()`、`exit()`、`assert()`、`log()`、`auth()`、`authAdmin()`、`getUser()`、`config()`、`env()`、`fetch()`、`setTimeout()`、`setInterval()`。
- `db.*` 是受控数据库接口，提供 `schema()`、`collections()`、`count(name)`、`currentUser()`、`getUser(uid)`、`findUsers(query, limit)`、`listUsers(options)`、`updateCurrentUser(patch)`、`updateUser(uid, patch)`。用户快照可包含邮箱、Telegram 用户名/ID 等联系信息；但不暴露原始 state、SQL、密码、Token、API Key、BGM Token 明文、Emby 内部 ID 或数据库连接信息。跨用户搜索和写入仅限管理员，写操作会写入审计日志；预览模式为 dry-run。
- `fetch()` 为受限同步能力，仅支持 `http/https` 的 `GET` / `POST` / `HEAD`，阻断 localhost、内网、链路本地目标，禁用跳转和凭据，响应体限长并脱敏。`eval`、`Function`、`globalThis`、`fetch`、`setTimeout`、`setInterval` 会被标记为高风险但不再静态拒绝；`require`、`process`、浏览器对象、本地存储、cookie、`constructor.constructor` 等仍会被阻断。

## Developer JS expanded user APIs

JS runtime now exposes a richer but still controlled user and system API surface:

- `user` / `users.current()` / `getUser(uid)` include user profile and contact metadata: `email`, `email_masked`, `has_email`, `telegram_id`, `telegram_username`, `emby_username`, `role_name`, `expire_status`, `avatar`, `background`, `bgm_mode`, `bgm_token_set`, `pending_emby`, `pending_emby_days`, `legacy_api_key_enabled`, login notification flags, and rebinding state.
- Sensitive implementation fields are still never injected: password hashes, raw tokens, API key hashes or full keys, BGM token values, Emby internal IDs, database URLs, and config secrets.
- Admin-only read helpers:
  - `users.search(query, limit)` / `db.findUsers(query, limit)` search by UID, username, email, Telegram username/ID, or Emby username. Results are capped at 50.
  - `users.list(options)` / `db.listUsers(options)` list sanitized users. Non-admin users only receive themselves; admins may pass `limit`, `offset`, `role`, and `active`.
- Admin-only write helpers:
  - `users.setActive(uid, active)`
  - `users.setRole(uid, role)`
  - `users.setExpiry(uid, expiredAt)`
  - `users.update(uid, patch)` / `db.updateUser(uid, patch)`
  - Accepted combined patch fields are `active`, `role`, `expired_at`, `notify_on_login_telegram`, `notify_on_login_email`, and the `telegram` / `email` aliases. Runtime writes audit logs and enforces last-admin protection.
- `system.info()` returns safe site metadata, feature flags, and limits. `system.feature(key)` reads one safe boolean feature flag. Neither helper returns raw secret config values.
- Convenience aliases and helpers:
  - `me` is an alias of `user`; `roles` is an alias of `constants.roles`.
  - `input` exposes `text`, `first`, `rest`, `count`, plus `arg(index, fallback)`, `has(index)`, `flag(name)`, and `named(name, fallback)` for commands such as `/lookup --q alice --force`.
  - `admin.*` provides shorter admin helpers: `ok()`, `ensure()`, `searchUsers()`, `listUsers()`, `updateUser()`, `setActive()`, `setRole()`, `setExpiry()`, and `stats()`.
  - `format.*` provides common output helpers: `bool()`, `role()`, `date()`, `expiry()`, `duration()`, `user()`, and `json()`.
  - `text.*`, `arrays.*`, and `time.*` include extra helpers for trimming/case conversion/templates, joining/sorting arrays, and calculating timestamps.
- Scripts execute inside a function scope, so a top-level `return` can be used to stop a command early.
