# Telegram Bot 命令使用文档

本文档说明 Twilight Telegram Bot 的命令、适用场景、权限要求和安全边界。

## 基本规则

| 规则 | 说明 |
| ---- | ---- |
| 普通用户命令 | 主要在 Bot 私聊中使用 |
| 管理员私聊命令 | 需要 `Telegram.admin_id` 中配置的 Telegram ID |
| 群组管理员命令 | 在群组/超级群中使用，按钮回调会再次校验管理员身份 |
| 面板功能 | 依赖 `Telegram.enable_tg_panel = true` |
| 订阅校验 | 开启 `Telegram.force_subscribe` 后，相关命令会检查频道/群组订阅状态 |
| 隐私限制 | Bot 不展示密码、服务器线路、Emby 用户名、Emby ID、Telegram ID |

## 普通用户命令

| 命令 | 场景 | 说明 |
| ---- | ---- | ---- |
| `/start` | 私聊 | 打开主菜单；群组中只提示前往私聊 |
| `/help` | 私聊 | 查看普通帮助 |
| `/twihelp` | 私聊 | `/help` 的别名 |
| `/bind` | 私聊 | 开始 Telegram 绑定流程 |
| `/bind <绑定码>` | 私聊 | 使用网页生成的 8 位绑定码完成绑定 |
| `/me` | 私聊 | 查看个人中心信息 |
| `/cancel` | 私聊 | 取消当前输入型流程，例如绑定码等待 |

### 绑定流程

1. 用户在 Web 端获取 Telegram 绑定码。
2. 用户私聊 Bot 发送 `/bind <绑定码>`。
3. Bot 校验绑定码后完成 Telegram 与系统账号绑定。

如果只发送 `/bind`，Bot 会进入等待绑定码状态，后续直接发送 8 位绑定码即可。

## Emby 相关命令

| 命令 | 权限 | 场景 | 说明 |
| ---- | ---- | ---- | ---- |
| `/emby` | 用户 | 私聊 | 查看 Emby 服务在线状态；不展示线路 |
| `/playinfo` | 已绑定用户 | 私聊 | 查看自己的播放统计 |
| `/resetpwd` | 已绑定用户 | 私聊 | 提示前往 Web 端修改密码 |

说明：出于安全考虑，密码重置、服务器线路、敏感账号信息均不通过 Telegram Bot 展示或操作。

## 管理员私聊命令

这些命令需要发送者的 Telegram ID 在 `Telegram.admin_id` 中。

| 命令 | 说明 |
| ---- | ---- |
| `/twishelp` | 查看管理员帮助 |
| `/admin` | 打开管理员内联面板 |
| `/cancel` | 取消当前管理员输入流程 |
| `/adduser <用户名> <天数>` | 创建系统用户 |
| `/regcode [天数] [数量]` | 生成注册码，默认 `30 1` |
| `/broadcast <内容>` | 向所有已绑定 Telegram 的用户广播 |
| `/stats` | 查看用户与注册码统计 |
| `/userinfo <用户名>` | 查看用户详情，并显示管理按钮 |
| `/twfind <关键词>` | 按系统用户名、UID、TGID、TG 用户名、Emby 标识搜索 |
| `/twbindcheck [关键词]` | 检查 TG 绑定状态；不传关键词则检查全局 |
| `/twforcebind <用户> <TGID>` | 强制绑定指定 TGID 到系统用户 |
| `/twsyncuser <用户>` | 同步指定用户状态到 Emby |
| `/sessions` | 查看 Emby 活跃会话 |
| `/kick <用户名>` | 踢出指定用户的 Emby 播放会话 |

### 私聊管理面板

`/admin` 会打开内联管理面板，支持：

- 用户管理
- 注册码管理
- 系统统计
- Emby 管理
- 广播

按钮回调同样会检查管理员身份。

## 群组管理员命令

群组管理员命令用于在 Telegram 群组里快速查询和处理用户。当前命令：

```text
/twguser <UID/用户名/TGID/关键词>
```

也可以回复某人的消息发送：

```text
/twguser
```

### 查询结果

群组查询只展示非隐私信息：

- 系统用户名
- UID
- 角色
- 启用/禁用状态
- Telegram 是否绑定
- Emby 是否绑定
- 是否有 Emby 开通资格
- 到期状态

不会展示：

- 密码
- 服务器线路
- Emby 用户名
- Emby ID
- Telegram ID

### 群组按钮操作

查询结果下方会按目标用户状态显示按钮：

| 按钮 | 作用 |
| ---- | ---- |
| 给予 Emby 开通资格 | 将用户标记为 `PENDING_EMBY=True`，用户前往 Web 后即可创建 Emby 账号 |
| 禁用 / 启用 | 修改本地用户状态，并尝试同步 Emby 启停状态 |
| 删除 | 二次确认后删除用户，并尝试删除其 Emby 账号 |
| 踢出不封禁 | 将 Telegram 用户踢出群组后立即解除 ban，使其仍可重新加入 |
| 封禁 | 在当前群组封禁 Telegram 用户；如有关联本地用户，也会禁用本地和 Emby |
| 刷新 | 重新读取并展示目标用户状态 |

### 匿名管理员消息

如果管理员以“匿名群组管理员”或频道身份发送 `/twguser`：

1. Bot 不会直接展示查询结果。
2. Bot 会发送“验证管理员身份”按钮。
3. 真实管理员点击按钮后，Bot 使用点击者的 Telegram ID 校验 `Telegram.admin_id`。
4. 校验通过后才展示查询结果和管理按钮。

### 群组权限要求

要使用“踢出不封禁”或“封禁”，Bot 必须是该群管理员，并拥有封禁成员权限。

如果权限不足，按钮会提示操作失败。

## 帮助文本自定义

管理员可通过配置完整覆盖 Bot 帮助文本：

| 配置项 | 说明 |
| ------ | ---- |
| `Telegram.bot_help_text` | 完整覆盖 `/help` 和 `/twihelp` |
| `Telegram.bot_admin_help_text` | 完整覆盖 `/twishelp` |
| `Telegram.bot_help_header` | 旧配置：只在默认普通帮助顶部追加 |
| `Telegram.bot_help_footer` | 旧配置：只在默认普通帮助底部追加 |

支持 `{server_name}` 占位符。文本按 Telegram Markdown 发送，特殊字符需要按 Markdown 规则转义。

## 安全注意事项

- 管理员命令只信任 `Telegram.admin_id`，不要把普通群管理员自动视为 Twilight 管理员。
- 群组 inline 按钮每次点击都会重新鉴权，不依赖发起消息者身份。
- Bot 管理功能不展示线路、密码、Emby 用户名、Emby ID、Telegram ID。
- 删除、封禁、禁用等操作应谨慎使用；删除用户会尝试同步删除 Emby 账号。
- 授予 Emby 开通资格不会直接创建 Emby 账号，用户需要登录 Web 端继续创建。

## 常见用法

### 管理员在群里查询被回复用户

1. 回复目标用户任意消息。
2. 发送 `/twguser`。
3. 查看非隐私摘要。
4. 按需点击“给予 Emby 开通资格”“禁用”“踢出不封禁”等按钮。

### 管理员按 UID 查询用户

```text
/twguser 123
```

### 管理员按关键词搜索用户

```text
/twguser alice
```

如果关键词不能唯一匹配一个用户，Bot 会提示用法或未找到目标。私聊中可用 `/twfind <关键词>` 查看多条匹配结果。
