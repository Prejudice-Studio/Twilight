# 开发者 JS 沙箱参考

本文档说明 Twilight 开发者模式中的 Telegram Bot 自定义 JS 指令。后台独立文档页为 `/admin/developer/js-docs`，由 `GET /api/v1/admin/developer/js-docs` 提供结构化数据，管理员登录后可查看完整函数参数表、返回值和示例。

## 启用与入口

- 在仪表盘兑换码输入框输入 `DEBUGMODE`，再完成管理员密码二次验证，可开启开发者模式。
- 开启后可在「开发者模式」中创建、命名、保存 JS 预设。
- 在「Telegram 管理 -> Bot 指令管理」中创建自定义指令，类型选择「自定义 JS」并选择预设。
- 推荐保存为 `js:preset:<id>` 动态引用格式；预设更新后指令会读取最新代码。旧格式 `js:<code>` 仍兼容，但属于静态代码快照。
- 再次输入 `DEBUGMODE` 并验证会关闭开发者模式。关闭后所有 `js:` / `js:preset:<id>` 指令、inline callback 和 waitText 交互都会被服务端阻断，但预设和指令配置不会被删除。

## 运行模型

- JS 引擎：Goja（`github.com/dop251/goja`）。
- 执行方式：同步执行，单次运行 200ms 超时。
- 作用域：脚本会包裹在函数作用域中运行，因此顶层 `return` 可提前结束。
- 提前退出：`exit(message?)` 可正常停止脚本；传入文本时会先追加回复，不会作为错误记录。
- 断言守卫：`assert(condition, message?)` 在条件为真时继续执行，为假时追加提示并正常退出。
- 预览模式：后台沙箱预览中 `ctx.preview=true`，写操作返回 `dry_run=true`，不会修改用户数据。

## 安全边界

沙箱不会暴露文件系统、进程、模块加载器、浏览器对象、原始数据库 state、SQL、数据库连接信息、密码、Token、API Key、BGM Token 明文、Emby 内部 ID 或敏感配置。

`fetch()` 是受限同步能力：只允许公开 `http/https` 的 `GET` / `POST` / `HEAD`，阻断 localhost、内网、链路本地目标，禁用跳转和凭据，响应体有限长。`eval`、`Function`、`globalThis`、`fetch`、`setTimeout`、`setInterval` 会被标记为高风险能力；`require`、`process`、浏览器对象、本地存储、cookie、`constructor.constructor` 等仍会被静态阻断。

## 全局绑定

| 名称 | 类型 | 说明 |
| ---- | ---- | ---- |
| `ctx` | object | 当前执行上下文：`private_chat`、`command_time`、`preview`、`command`。 |
| `command` | object | 指令触发对象：`name`、`args`、`text`、`private_chat`、`preview`、`from_id`。 |
| `input` | object | 参数解析对象：`text`、`first`、`rest`、`count`、`arg()`、`has()`、`flag()`、`named()`。 |
| `args` | string[] | 指令参数数组，不包含命令名。 |
| `user` / `me` | object | 当前 Telegram 绑定用户的脱敏快照。 |
| `constants` / `roles` | object | 角色、运行限制等常量。 |
| `db` | namespace | 受控数据库读写接口。 |
| `users` | namespace | 当前用户和管理员用户操作接口。 |
| `admin` | namespace | 管理员快捷接口。 |
| `system` | namespace | 安全系统元信息、功能开关、统计。 |
| `text` / `arrays` / `time` / `format` | namespace | 文本、数组、时间和格式化工具。 |
| `interactions` | namespace | Telegram inline 和等待文本交互。 |

## 核心函数

| 函数 | 参数 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `reply(text)` | `text: string` | `void` | 追加一段回复，最多 4 段。 |
| `exit(text?)` | `text?: string` | `never` | 正常提前结束脚本；可选追加一段回复。 |
| `assert(condition, text?)` | `condition: any`, `text?: string` | `boolean\|never` | 条件为假时提示并退出。 |
| `log(text)` | `text: string` | `void` | 追加本次执行日志，最多 8 条。 |
| `auth(role)` | `role: string\|number` | `boolean` | 检查当前用户角色。 |
| `authAdmin()` | 无 | `boolean` | 判断当前用户是否管理员。 |
| `getUser(uid)` | `uid: number\|string` | `UserSnapshot\|null` | 按精确 UID 读取脱敏用户快照；跨用户读取需要管理员。 |
| `config(key)` | `key: string` | `string\|number\|boolean` | 读取白名单内非敏感配置。 |
| `env(key)` | `key: string` | `string` | 读取白名单内非敏感环境变量。 |
| `fetch(url, options?)` | `url: string`, `options.method?: string` | object | 受限同步 HTTP 请求。 |

完整参数表由 `/admin/developer/js-docs` 动态展示，后端新增沙箱 API 时必须同步更新该端点和本文档。

## 用户快照

`UserSnapshot` 可包含：`uid`、`username`、`email`、`email_masked`、`has_email`、`role`、`role_name`、`active`、`expired_at`、`expire_status`、`created_at`、`register_time`、`has_emby`、`emby_username`、`emby_disabled`、`avatar`、`background`、`bgm_mode`、`bgm_token_set`、`email_verified`、`email_verified_at`、`telegram_bound`、`telegram_id`、`telegram_username`、`notify_on_login_telegram`、`notify_on_login_email`、`legacy_api_key_enabled`、`rebinding_in_progress`、`rebinding_since`。

不会包含密码、Token、API Key、BGM Token 明文、Emby 内部 ID、原始数据库状态或数据库连接信息。

## 常用示例

### 参数校验与提前退出

```js
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

### 当前用户状态

```js
const me = users.current();
reply(text.template("Hi {name}\nEmail: {email}\nRole: {role}\nExpiry: {expiry}", {
  name: me.username || "unbound",
  email: me.email_masked || "none",
  role: me.role_name,
  expiry: me.expire_status
}));
```

### 管理员搜索用户

```js
assert(admin.ensure(), "Admin only");

const query = input.named("q", input.text);
const rows = admin.searchUsers(query, 5);
if (!rows.length) {
  exit("No users matched: " + query);
}

reply(text.numberLines(rows.map(function(u) {
  return format.user(u);
})));
```

### 管理员设置到期时间

```js
assert(admin.ensure(), "Admin only");

const uid = Number(input.named("uid", 0));
const days = Number(input.named("days", 7));
if (!uid || days < 1 || days > 3650) {
  exit("Usage: /setexp --uid 10001 --days 30");
}

const result = admin.setExpiry(uid, time.addDays(time.now(), days));
reply(result.ok ? ("New expiry: " + format.expiry(result.user.expired_at)) : ("Failed: " + result.error));
```

### Inline 菜单

```js
const me = users.current();
interactions.inline("Account menu for " + (me.username || "user"), [
  { text: "Status", answer: "Status checked", edit: format.user(me) },
  { text: "Email", reply: "Email: " + (me.email_masked || "none") },
  { text: "Help", reply: "Use /help for built-in commands." }
]);
```

### 等待下一条文本

```js
interactions.waitText({
  seconds: 45,
  prompt: "Send the note text within 45 seconds.",
  reply_prefix: "Saved note:",
  timeout_reply: "Timed out; no note saved.",
  max_chars: 200
});
```

### 受限 fetch

```js
const res = fetch("https://example.com/status.json");
if (!res.ok) {
  exit("fetch failed: " + (res.error || res.status));
}

try {
  const data = JSON.parse(res.text);
  reply("status=" + (data.status || "unknown"));
} catch (e) {
  reply("invalid json: " + text.truncate(res.text, 120));
}
```
