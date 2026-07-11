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
- 执行方式：同步执行，单次运行 8 秒墙钟超时（须大于沙箱网络预算：fetch 约 1500ms HTTP + 500ms DNS；interactions 会真实发送 Telegram 消息）。
- 作用域：脚本会包裹在函数作用域中运行，因此顶层 `return` 可提前结束。
- 提前退出：`exit(message?)` 可正常停止脚本；传入文本时会先追加回复，不会作为错误记录。
- 断言守卫：`assert(condition, message?)` 在条件为真时继续执行，为假时追加提示并正常退出。
- 预览模式：后台沙箱预览中 `ctx.preview=true`，写操作返回 `dry_run=true`，不会修改用户数据。

## 安全边界

沙箱不会暴露文件系统、进程、模块加载器、浏览器对象、原始数据库 state、SQL、数据库连接信息、密码、Token、API Key、BGM Token 明文、Emby 内部 ID 或敏感配置。

`fetch()` 是受限同步能力：只允许公开 `http/https` 的 `GET` / `POST` / `HEAD`，阻断 localhost、内网、链路本地（含云元数据 `169.254.169.254`）、广播与组播目标，禁用跳转和凭据，响应体有限长。除发起前按域名解析校验外，还会在 TCP 拨号阶段对**实际连接到的 IP** 再校验一次，阻断 DNS rebinding（解析时返回公网 IP、连接时切到内网 IP）与 IPv4-mapped IPv6 绕过。`eval`、`Function`、`globalThis`、`fetch`、`setTimeout`、`setInterval` 会被标记为高风险能力；`require`、`process`、浏览器对象、本地存储、cookie、`constructor.constructor` 等仍会被静态阻断。

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
| `regcodes` | namespace | 注册码列表、查询与生成（生成需管理员）。 |
| `invites` | namespace | 邀请码列表与生成（生成需管理员且功能开启）。 |
| `announcements` | namespace | 公告列表与创建（创建需管理员）。 |
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

## 用户接口（`users`）

`users` 命名空间在受控范围内读取与操作用户。读取遵循 getUser 同口径鉴权（普通用户只能读自己，管理员可读任意 UID）；写操作（启停 / 角色 / 到期 / 更新）一律要求当前 Telegram 绑定用户为管理员，支持预览 dry-run、写审计日志，并保持 last-admin 保护。下列「简化别名」是常用写操作的位置参数封装，省去 options 对象，便于脚本直接调用，且完全复用对应完整函数的鉴权、预览与审计语义。

| 函数 | 权限 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `users.current()` / `users.describe()` | 任意 | UserSnapshot | 当前绑定用户脱敏快照。 |
| `users.get(uid)` / `users.byUID(uid)` | 本人/管理员 | UserSnapshot\|null | 按精确 UID 读取脱敏快照。 |
| `users.search(query, limit?)` | 管理员 | UserSnapshot[] | 多字段搜索用户，最多 50 条。 |
| `users.find(query, limit?)` | 管理员 | UserSnapshot[] | `search` 的简化别名，语义一致。 |
| `users.list(options)` | 本人/管理员 | UserSnapshot[] | 普通用户只返回自己；管理员可分页筛选。 |
| `users.exists(uid)` | 本人/管理员 | boolean | 该 UID 是否存在；跨用户查询需管理员，否则返回 false。 |
| `users.hasRole(role)` | 任意 | boolean | 判断当前用户是否具备指定角色。 |
| `users.requireActive()` | 任意 | boolean | 当前用户是否已激活。 |
| `users.setActive(uid, active)` | 管理员 | object | 设置 Web 账号启停。 |
| `users.enable(uid)` | 管理员 | object | `setActive(uid, true)` 的简化别名。 |
| `users.disable(uid)` | 管理员 | object | `setActive(uid, false)` 的简化别名。 |
| `users.setRole(uid, role)` | 管理员 | object | 设置用户角色，保持 last-admin 保护。 |
| `users.setExpiry(uid, expiredAt)` | 管理员 | object | 设置绝对到期时间戳（秒）。 |
| `users.extend(uid, days)` | 管理员 | object | 在当前到期时间（或现在，取较晚者）上顺延 `days` 天；永久用户原样返回 `note:"already_permanent"`。 |
| `users.setLoginNotify(patch)` | 本人 | object | 仅改当前用户登录通知偏好；预览返回 `dry_run`。 |
| `users.update(uid, patch)` | 管理员 | object | 受控更新用户状态/角色/到期/通知，写审计日志。 |

简化别名返回结构与对应完整函数一致：`{ ok, dry_run?, uid?, error?, ... }`。非管理员调用写操作返回 `{ ok:false, error:"admin_required" }`；`users.exists` 在越权或 UID 非法时返回 `false`。

## 数据库接口（`db`）

`db` 命名空间提供受控只读查询与受限写入，不暴露原始 state、SQL 或数据库连接信息。列表函数统一接受 `{ limit, offset }` 选项，`limit` 上限为 50。

| 函数 | 权限 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `db.schema()` | 任意 | object | 受控集合结构和允许字段。 |
| `db.collections()` | 任意 | string[] | 允许查看的受控集合名。 |
| `db.count(name)` | 任意/管理员 | number | 集合计数；管理员专属集合对非管理员返回 -1。 |
| `db.currentUser()` | 任意 | UserSnapshot | 与 `users.current()` 相同。 |
| `db.getUser(uid)` | 本人/管理员 | UserSnapshot\|null | 按精确 UID 读取脱敏快照。 |
| `db.findUsers(query, limit)` | 管理员 | UserSnapshot[] | 多字段搜索用户，最多 50 条。 |
| `db.listUsers(options)` | 本人/管理员 | UserSnapshot[] | 普通用户只返回自己；管理员可分页筛选。 |
| `db.listRegcodes(options)` | 管理员 | RegCodeSnapshot[] | 脱敏注册码快照，不含用户密钥。 |
| `db.listInviteCodes(options)` | 本人/管理员 | InviteCodeSnapshot[] | 管理员看全部；普通用户只看自己拥有的邀请码。 |
| `db.listMediaRequests(options)` | 本人/管理员 | MediaRequestSnapshot[] | 管理员看全部；普通用户只看自己的求片记录。 |
| `db.listAnnouncements(options)` | 任意 | AnnouncementSnapshot[] | 可见公告快照，不含正文。 |
| `db.listTickets(options)` | 本人/管理员 | TicketSnapshot[] | 管理员看全部；普通用户只看自己的工单，不含正文。 |
| `db.listPresets(options)` | 管理员 | PresetSnapshot[] | 开发者 JS 预设元数据，不含代码正文（仅 `code_length`）。 |
| `db.updateCurrentUser(patch)` | 本人 | object | 仅改当前用户登录通知偏好；预览返回 `dry_run`。 |
| `db.updateUser(uid, patch)` | 管理员 | object | 受控更新用户状态/角色/到期/通知，写审计日志。 |

各快照可用字段见 `db.schema()` 返回的对应集合 `fields`。

## 用户快照

`UserSnapshot` 可包含：`uid`、`username`、`email`、`email_masked`、`has_email`、`role`、`role_name`、`active`、`expired_at`、`expire_status`、`created_at`、`register_time`、`has_emby`、`emby_username`、`emby_disabled`、`avatar`、`background`、`bgm_mode`、`bgm_token_set`、`email_verified`、`email_verified_at`、`telegram_bound`、`telegram_id`、`telegram_username`、`notify_on_login_telegram`、`notify_on_login_email`、`legacy_api_key_enabled`、`rebinding_in_progress`、`rebinding_since`。

不会包含密码、Token、API Key、BGM Token 明文、Emby 内部 ID、原始数据库状态或数据库连接信息。

## 生成接口（`regcodes` / `invites` / `announcements`）

这三个命名空间提供受控的列表/查询与生成能力。**所有生成与创建操作都需要当前 Telegram 绑定用户为管理员**；非管理员调用返回 `{ ok:false, error:"admin_required" }`。预览模式（后台沙箱 `ctx.preview=true`）下写操作返回 `{ ok:true, dry_run:true, ... }` 且不修改任何数据。所有成功写操作都会写入审计日志，来源标记为 `telegram_js`。

注册码、邀请码本身是可下发的兑换凭据（不是密钥），因此生成结果会在返回值中包含具体码值，供脚本回显给管理员。

### `regcodes`

| 函数 | 权限 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `regcodes.list(options)` | 管理员 | RegCodeSnapshot[] | 与 `db.listRegcodes` 一致，支持 `{ limit, offset }`。 |
| `regcodes.get(code)` | 管理员 | RegCodeSnapshot\|null | 按精确码值查询脱敏快照。 |
| `regcodes.generate(options)` | 管理员 | object | 批量生成注册/续期/白名单码，写审计日志。 |
| `regcodes.quick(days?, count?, type?)` | 管理员 | object | `generate` 的位置参数简化别名，省去 options 对象。 |

`regcodes.generate(options)` 选项：

| 字段 | 类型 | 默认 | 说明 |
| ---- | ---- | ---- | ---- |
| `count` | number | 1 | 生成数量，范围 1-100。 |
| `type` | number | 1 | 码类型：1 注册码、2 续期码、3 白名单码。 |
| `days` | number | 30 | 兑换后授予/续期天数，上限 36500。 |
| `use_count_limit` | number | 1 | 使用次数上限；`-1` 表示无限。 |
| `validity_time` | number | -1 | 码有效期（秒），`-1` 表示永久。 |
| `note` | string | 空 | 备注，最长 120 字符。 |
| `target_username` | string | 空 | 限定可使用的用户名。 |
| `decoy` | boolean | false | 是否为诱饵码。 |
| `format` / `algorithm` | string | 配置默认 | 码格式与随机算法。 |

返回：`{ ok, dry_run?, codes?: string[], count?, type?, days?, error? }`。

### `invites`

| 函数 | 权限 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `invites.list(options)` | 本人/管理员 | InviteCodeSnapshot[] | 管理员看全部；普通用户只看自己的邀请码。 |
| `invites.generate(options)` | 管理员 | object | 生成一枚邀请码（需 `invite_enabled`），写审计日志。 |
| `invites.quick(days?)` | 管理员 | object | `generate` 的位置参数简化别名（需 `invite_enabled`）。 |

`invites.generate(options)` 选项：

| 字段 | 类型 | 默认 | 说明 |
| ---- | ---- | ---- | ---- |
| `days` | number | 配置默认 | 邀请授予天数，受 `maxCodeDays` 上限约束。 |
| `expires_at` | number | -1 | 邀请码过期时间戳（秒），`-1` 表示永久。 |
| `note` | string | 空 | 备注，最长 255 字符。 |
| `target_username` | string | 空 | 限定可使用的用户名。 |
| `format` / `algorithm` | string | 配置默认 | 码格式与随机算法。 |

返回：`{ ok, dry_run?, code?, invite?, days?, error? }`。功能未开启时返回 `error:"invite_disabled"`。

### `announcements`

| 函数 | 权限 | 返回 | 说明 |
| ---- | ---- | ---- | ---- |
| `announcements.list(options)` | 任意 | AnnouncementSnapshot[] | 可见公告快照，不含正文。 |
| `announcements.create(options)` | 管理员 | object | 创建公告，写审计日志。 |
| `announcements.post(title, content, level?)` | 管理员 | object | `create` 的位置参数简化别名。 |

`announcements.create(options)` 选项：

| 字段 | 类型 | 默认 | 说明 |
| ---- | ---- | ---- | ---- |
| `title` | string | `公告` | 公告标题。 |
| `content` | string | 空 | 公告正文。 |
| `level` | string | `info` | 级别，如 `info`/`warning`/`critical`。 |
| `render_mode` | string | 安全默认 | 渲染模式：`markdown`/`bbcode`/`plain`。 |
| `visible` | boolean | true | 是否可见。 |
| `pinned` | boolean | false | 是否置顶。 |
| `expires_at` | number | 0 | 过期时间戳（秒），0 表示永不过期。 |

返回：`{ ok, dry_run?, announcement?, error? }`。

### `admin` 便捷方法

`admin` 命名空间额外提供与上述等价的快捷方法，便于管理员脚本直接调用：

| 函数 | 等价于 |
| ---- | ---- |
| `admin.generateRegcode(options)` | `regcodes.generate(options)` |
| `admin.generateInviteCode(options)` | `invites.generate(options)` |
| `admin.createAnnouncement(options)` | `announcements.create(options)` |

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

### 公告列表（简单）

```js
const rows = db.listAnnouncements({ limit: 5 });
if (!rows.length) {
  exit("No announcements");
}
reply(text.numberLines(rows.map(function(an) {
  return an.title + (an.pinned ? " [pinned]" : "");
})));
```

### 我的求片（简单）

```js
const rows = db.listMediaRequests({ limit: 10 });
if (!rows.length) {
  exit("No media requests");
}
reply(text.numberLines(rows.map(function(m) {
  return m.title + " [" + m.status + "]";
})));
```

### 注册码统计报表（复杂·管理员）

```js
assert(admin.ensure(), "Admin only");

const limit = Number(input.named("limit", 20));
const rows = db.listRegcodes({ limit: limit, offset: Number(input.named("offset", 0)) });
const total = db.count("regcodes");

let active = 0;
let usedUp = 0;
const preview = arrays.take(rows.map(function(c) {
  if (c.active) active++;
  if (c.use_count_limit > 0 && c.use_count >= c.use_count_limit) usedUp++;
  const left = c.use_count_limit > 0 ? (c.use_count_limit - c.use_count) : "∞";
  return c.code + " " + c.type_name + " uses=" + c.use_count + "/" + (c.use_count_limit || "∞") + " left=" + left;
}), 12);

reply(text.joinLines([
  "total=" + total + " page=" + rows.length,
  "active=" + active + " used_up=" + usedUp,
  "----",
  text.numberLines(preview)
]));
```

### 工单分诊（复杂·管理员）

```js
assert(admin.ensure(), "Admin only");

const rows = db.listTickets({ limit: 50 }).filter(function(t) {
  return t.status !== "closed" && t.status !== "resolved";
});
if (!rows.length) {
  exit("No open tickets");
}

const oldest = rows.slice().sort(function(a, b) {
  return a.created_at - b.created_at;
})[0];

reply(text.joinLines([
  "open=" + rows.length,
  "oldest #" + oldest.id + " [" + oldest.status + "]",
  "title: " + text.truncate(oldest.title || "(none)", 80),
  "since: " + time.formatUnix(oldest.created_at)
]));
```

### 生成注册码（管理员）

```js
assert(admin.ensure(), "Admin only");

const count = Number(input.named("count", 1));
const days = Number(input.named("days", 30));
const result = regcodes.generate({ count: count, type: 1, days: days, use_count_limit: 1 });
if (!result.ok) {
  exit("Failed: " + result.error);
}
if (result.dry_run) {
  exit("Preview: would generate " + result.count + " codes");
}

reply(text.joinLines([
  "Generated " + result.count + " regcode(s), days=" + result.days,
  text.numberLines(result.codes)
]));
```

### 生成邀请码（管理员）

```js
assert(admin.ensure(), "Admin only");

const result = invites.generate({ days: Number(input.named("days", 30)) });
if (!result.ok) {
  exit("Failed: " + result.error);
}
reply(result.dry_run ? "Preview: invite would be created" : ("Invite code: " + result.code));
```

### 创建公告（管理员）

```js
assert(admin.ensure(), "Admin only");

const title = input.named("title", "公告");
const content = input.rest || "(no content)";
const result = announcements.create({ title: title, content: content, level: "info", render_mode: "markdown" });
if (!result.ok) {
  exit("Failed: " + result.error);
}
reply(result.dry_run ? "Preview: announcement would be created" : "Announcement created");
```
