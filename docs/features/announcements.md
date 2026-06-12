# 公告系统

本文说明 Twilight 的全站公告：数据模型、三种渲染模式（纯文本 / Markdown / BBCode）、前端安全约束、管理员发布页与仪表盘展示位置，以及公开列表接口与管理员 CRUD 接口。

公告是一种由管理员发布、面向全站用户展示的通知。它支持级别标记、置顶、过期时间，并可选用 Markdown / BBCode 等富文本渲染。其它相邻功能见 [邀请树](./invite.md)，鉴权与安全机制见 [安全加固](../guides/security.md)，完整接口契约见 [后端 API 详参](../reference/backend-api.md)。

## 数据模型

公告不是一张独立的数据库表，而是 Twilight「单一状态文档」（`internal/store`）里的一组 `Announcement` 记录。整个业务状态（用户、注册码、邀请关系、邀请码、公告等）统一保存在：

- JSON 文件 `db/twilight_go_state.json`，或
- PostgreSQL 的 `twilight_state` 表中 `id=1` 那一行的 `jsonb` 字段。

公告以 `map[int64]Announcement` 的形式挂在状态文档的 `announcements` 字段下，自增主键来自状态文档里的 `next_announcement_id`。`render_mode` 等都是 `Announcement` 结构体上的普通字段，**不是**新增的数据库列。

> 注意：早期 Python 版本曾以「`announcements` 表新增 `RENDER_MODE` 列、老库启动时自动 `ALTER TABLE`」来描述渲染模式。Go 版本不存在任何建表或 `ALTER TABLE` 行为，`render_mode` 只是状态文档里 `Announcement` 记录的一个字段。

### Announcement 字段

字段定义见 `internal/store/store.go` 的 `Announcement` 结构体：

| 字段（JSON） | 类型 | 默认 | 含义 |
| ---- | ---- | ---- | ---- |
| `id` | int64 | 自增 | 公告主键。新建时由 `next_announcement_id` 分配。 |
| `title` | string | `"公告"` | 标题，可选。创建时若为空，后端回退为「公告」。 |
| `content` | string | — | 正文，**后端原样保存**，不做任何 HTML 解析或转义。 |
| `visible` | bool | `true` | 是否对终端用户可见。`false` 相当于草稿。 |
| `level` | string | `"info"` | 级别：`info` / `notice` / `warning` / `critical`。空值回退为 `info`。 |
| `render_mode` | string | `"plain"` | 渲染模式：`plain` / `markdown` / `bbcode`。空值回退为 `plain`。 |
| `pinned` | bool | `false` | 是否置顶。置顶公告排在列表最前。 |
| `created_by_uid` | int64 | 当前管理员 | 创建者 UID，仅作审计记录，前端不展示作者。 |
| `created_at` | int64 | 创建时间 | Unix 秒级时间戳。 |
| `updated_at` | int64 | 更新时间 | 每次写入都会刷新为当前时间。 |
| `expired_at` | int64 | `0` | 过期时间（Unix 秒）。`0` 或 `<=0` 表示永不过期。后端字段名为 `expired_at`。 |

关于过期字段，后端读取请求体时同时接受 `expires_at` 与 `expired_at` 两个键（见 `internal/api/announcement_handlers.go`），最终存为结构体上的 `ExpiredAt`。前端类型（`webui/src/lib/api-types.ts`）则统一用 `expires_at`，并约定 `-1` 表示永不过期。

### 排序与可见性

`ListAnnouncements(includeHidden bool)`（`internal/store/store.go`）的行为：

- 排序：先按 `pinned`（置顶在前），再按 `id` 倒序（新公告在前）。
- 当 `includeHidden=false`（公开视角）时，过滤掉 `visible=false` 的条目，以及 `expired_at > 0 且已过期` 的条目。
- 当 `includeHidden=true`（管理员视角）时，返回全部公告，包括隐藏与已过期的。

## 渲染模式

`render_mode` 有三种取值，后端通过 `safeAnnouncementRenderMode`（`internal/api/announcement_handlers.go`）做白名单兜底：只接受 `markdown` 与 `bbcode`，其余一律归一化为 `plain`。归一化时会做 `ToLower` + `TrimSpace`。

| 模式 | 说明 |
| ---- | ---- |
| `plain` | 纯文本。保留换行，所有字符按字面展示。 |
| `markdown` | 手写 Markdown 子集渲染。 |
| `bbcode` | BBCode 渲染，内部先转换为 Markdown 子集再渲染。 |

> 旧文档曾提到后端会兼容 `text` / `md` / `bb` 等别名并做规范化。当前 Go 实现的 `safeAnnouncementRenderMode` **不识别这些别名**：除 `markdown`、`bbcode` 之外的任何输入（含 `text` / `md` / `bb`）都会被归一化为 `plain`。

实际渲染全部在前端完成，渲染器位于 `webui/src/lib/safe-render.tsx`，导出组件 `SafeAnnouncementContent({ content, mode })`。

## 前端安全约束

公告内容由后端原样保存，所有解析都在前端 React 树里完成，**永远不会使用 `dangerouslySetInnerHTML`**，从根上规避 XSS。具体约束（均见 `webui/src/lib/safe-render.tsx`）：

- **纯文本（plain）**：用 `whitespace-pre-wrap break-words` 直接渲染原始字符串，保留换行；所有 `<`、`>` 等字符由 React 自动 HTML 转义。
- **Markdown**：手写小型解析器输出 React 元素，不经过任何 HTML 字符串。支持的语法：
  - 标题 `#` 到 `######`（按层级渲染 `h1`~`h6`）；
  - 行内强调：`**粗体**`、`__粗体__`、`*斜体*`、`_斜体_`、`~~删除线~~`；
  - 行内代码 `` `code` `` 与围栏代码块 ```` ``` ````（可带语言标注，渲染为 `pre`）；
  - 引用 `>`；
  - 无序列表 `- * +`、有序列表 `1.`；
  - 分割线（`---` / `***` / `___` 等三个以上同字符）；
  - 图片 `![alt](url)`、链接 `[label](url)`、`http(s)://` 自动链接；
  - 反斜杠转义 `\`。
- **BBCode**：先用正则把白名单 BBCode 标签翻译成等价 Markdown，再交给 Markdown 渲染器。当前实现的标签映射：

  | BBCode | 转换为 |
  | ---- | ---- |
  | `[b]...[/b]` | `**...**` |
  | `[i]...[/i]` | `*...*` |
  | `[s]...[/s]` | `~~...~~` |
  | `[u]...[/u]` | 去标签保留文本（下划线无 Markdown 等价，原样保留内容） |
  | `[code]...[/code]` | `` `...` ``（行内代码） |
  | `[quote]...[/quote]` | `> ...`（引用） |
  | `[url=链接]文本[/url]` | `[文本](链接)` |
  | `[url]...[/url]` | 去标签保留文本 |

  未在表中的标签会保留原样字符，最终按 Markdown / 纯文本处理。

- **URL 安全校验**：链接与图片的 URL 都要过白名单校验：
  - 链接（`isSafeUrl`）：允许 `http:` / `https:` / `mailto:` 协议、站内绝对路径 `/...`、页内锚点 `#...`；拒绝以 `//` 开头的协议相对 URL，以及含控制字符的串。安全链接渲染为 `<a target="_blank" rel="noopener noreferrer nofollow ugc">`，不安全则降级为字面文本。
  - 图片（`isSafeImageUrl`）：只允许 `http(s):` 协议或站内绝对路径 `/...`；同样拒绝 `//` 开头与控制字符。`<img>` 带 `loading="lazy"` 与 `referrerPolicy="no-referrer"`，不安全则降级为字面文本。

> 旧文档把 BBCode / Markdown 的能力描述得更广（如 `[color]`、`[size]`、`[list]`、`[spoiler]`、`[center]`、`[img]`、任务列表、表格等，以及颜色 / 尺寸白名单和基于栈的 BBCode 解析器）。**当前 `safe-render.tsx` 并未实现这些**：BBCode 仅支持上表中的 `b/i/s/u/code/quote/url` 映射，Markdown 也没有任务列表 / 表格 / `[*]` 列表项等。管理员发布页下拉框的提示文案（`webui/src/app/(main)/admin/announcements/page.tsx` 中的 `RENDER_OPTIONS` hint）仍沿用了旧的较宽描述，与实际渲染能力不完全一致。

## 管理员发布页

管理员侧边栏「公告管理」对应页面 `webui/src/app/(main)/admin/announcements/page.tsx`，前端路由 `/admin/announcements`。功能：

- 列表展示全部公告（含隐藏与已过期），带「显示已隐藏」「显示已过期」开关、级别徽标、置顶 / 隐藏 / 编辑 / 删除操作，以及分页控件。
- 新建 / 编辑对话框包含：标题（可选）、内容（最多 10000 字）、级别下拉、**渲染方式下拉**（纯文本 / Markdown / BBCode）、截止时间（`datetime-local`，留空表示永久）、置顶开关、立即可见开关。
- 表单内嵌「预览」区，使用同一个 `SafeAnnouncementContent` 组件实时渲染当前内容，便于发布前确认效果。

> 前端的 `adminListAnnouncements` 会附带 `page` / `per_page` / `include_invisible` / `include_expired` 查询参数，但当前 Go 端 `handleAdminAnnouncements` 直接返回 `ListAnnouncements(true)` 的全量结果，**并未解析这些查询参数**，也不返回 `page` / `per_page` / `pages` 字段。分页与过滤目前实际由前端在拿到全量后自行处理。

## 仪表盘与公开展示

公告组件 `AnnouncementBoard`（`webui/src/components/announcement-board.tsx`）有两处使用：

- **仪表盘**（`webui/src/app/(main)/dashboard/page.tsx`）：以 `<AnnouncementBoard splitPinned />` 放在页面**最后一个区块**，避免占据首屏。`splitPinned` 模式会把「置顶公告」与「最新公告」分成两组分别展示与折叠。
- **独立公告页**（`webui/src/app/(main)/announcements/page.tsx`，路由 `/announcements`）：以时间线视图展示全部公告（`limit=200`、`collapseAfter=200`、`showEmptyState`），不分置顶 / 最新两组。

两处都通过公开接口 `GET /announcements` 拉取数据，并用 `SafeAnnouncementContent` 渲染正文。

### 长内容折叠

`AnnouncementCard` 组件（`announcement-board.tsx` 行64）支持单条公告的长内容折叠：
- 默认 maxHeight 300px，内容超出自动截断隐藏
- 使用 `useRef` + `useEffect` 检测实际内容高度（`scrollHeight > maxHeight + 8`）
- 超出时自动显示「展开 / 收起」按钮
- 按钮文案通过 i18n 键 `announcements.expand` / `announcements.collapse` 控制

### Markdown 列表渲染

`safe-render.tsx` 使用 `list-inside`（非默认的 `list-outside`）保证 `•` 圆点始终在容器内可见，避免被父级 `overflow: hidden` 裁剪。

## 接口

统一响应 envelope 为 `{ success, code, message, data, timestamp }`。鉴权级别与安全约定详见 [安全加固](../guides/security.md)；完整字段契约见 [后端 API 详参](../reference/backend-api.md)，路由总表见 [API 路由索引](../reference/api-index.md)。

路由注册见 `internal/api/routes.go`，handler 见 `internal/api/announcement_handlers.go`。

### 公开列表

| 方法 | 路径 | 鉴权 | 说明 |
| ---- | ---- | ---- | ---- |
| `GET` | `/api/v1/announcements` | `AuthPublic` | 返回对终端用户可见的公告（已过滤隐藏与过期）。 |

`data` 形如 `{ "announcements": [...], "total": <数量> }`。该接口免登录，登录页、仪表盘、独立公告页均可直接调用。前端会附带 `?limit=` 参数，但后端当前未据此截断结果。

### 管理员 CRUD

均为 `AuthAdmin`（登录会话且 `Role == RoleAdmin`）。变更类方法不要求额外令牌，只校验有效管理员会话或等价管理员鉴权。

| 方法 | 路径 | 说明 |
| ---- | ---- | ---- |
| `GET` | `/api/v1/admin/announcements` | 列出全部公告（含隐藏与过期）。`data` 为 `{ announcements, total }`。 |
| `POST` | `/api/v1/admin/announcements` | 新建公告。成功返回 201 与新建记录。 |
| `PUT` | `/api/v1/admin/announcements/:announcement_id` | 更新公告。未传字段沿用既有值；`created_by_uid` / `created_at` 保持不变。 |
| `DELETE` | `/api/v1/admin/announcements/:announcement_id` | 删除公告。不存在则返回未找到错误。 |

创建 / 更新接受的请求体字段（`internal/api/announcement_handlers.go`）：

```json
{
  "title": "维护通知",
  "content": "正文，原样保存",
  "level": "info",
  "render_mode": "markdown",
  "pinned": false,
  "visible": true,
  "expires_at": 1733000000
}
```

字段语义：

- `title` 省略或为空时回退为「公告」。
- `render_mode` 经 `safeAnnouncementRenderMode` 兜底，非 `markdown` / `bbcode` 一律存为 `plain`。
- `level` 为空时回退为 `info`。
- `expires_at`（或别名 `expired_at`）为 Unix 秒，`0` / `<=0` 表示永久。
- 更新时只覆盖请求体里出现的字段，其余沿用既有公告的值。
