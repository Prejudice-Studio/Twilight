# 邀请树 & 公告系统

> 本文档覆盖 2026-05 引入的两个模块：
>
> 1. **邀请树（Invite Tree）**：用户互相邀请生成 Emby 账号，形成森林结构。
> 2. **公告渲染**：公告支持 Markdown / BBCode / 纯文本三种模式，前端做安全清洗。
>
> 老版本 API 见 [`docs/BACKEND_API.md`](./BACKEND_API.md)。

---

## 1. 邀请树

### 1.1 概念

* **树根（root）**：邀请关系图中没有上级（PARENT_UID）的节点。
* **层级（depth）**：从该节点向上回溯到根的层数（根本身 = 1）。
  示例 `B → A → C`：B=1，A=2，C=3，整树深度为 3。
* **子树（subtree）**：以某节点为根、向下递归的所有后代集合。
* **断开（detach）**：把某用户的 PARENT_UID 边删掉。他名下的子节点不变，
  但他自己晋升为新树根。
* **级联删除（cascade delete）**：以某节点为起点，按指定层级一并删除若干代后代。

### 1.2 配置项（`[SAR]` 段）

| 字段 | 默认 | 说明 |
| ---- | ---- | ---- |
| `invite_enabled` | `false` | 邀请系统总开关。关闭后所有 `/invite/*` 接口返回 403。 |
| `invite_max_depth` | `3` | 整棵邀请树允许的最大层级。`1` 等于禁止邀请。 |
| `invite_limit` | `10` | 每位用户**未使用**邀请码上限（已用的不算）。`-1` 表示无限制。 |
| `invite_require_emby` | `true` | 是否要求邀请人已绑定 Emby 才能生码。 |
| `invite_code_default_days` | `30` | 被邀请人 Emby 账号默认开通天数。`0` / `-1` 表示永久。 |

修改这些字段后会触发整进程重启（沿用现有 [config 重启策略][cfg-restart]）。

[cfg-restart]: ./DEVELOPMENT.md

### 1.3 数据库

新增数据库 `db/invites.db`：

* **`invite_relations`**：`CHILD_UID` 作主键，强制 1 个用户最多 1 个上级。
* **`invite_codes`**：邀请码本体，含 `INVITER_UID`、`DAYS`、`USE_COUNT*` 等。

老库无需迁移；首次启动会自动建表。**`announcements` 表会自动新增 `RENDER_MODE` 列**。

### 1.4 前端入口

* **普通用户**：侧边栏「邀请中心」`/invite`
  * 查看自己的层级 / 上级 / 下级
  * 生成 / 复制 / 撤销邀请码
* **管理员**：侧边栏「邀请森林」`/admin/invite`
  * 星图（SVG 自绘）可视化整棵森林
  * 点击节点查看用户详情、断开上级、级联删除

### 1.5 用户接口

| Method | Path | 描述 |
| ------ | ---- | ---- |
| `GET`  | `/invite/config` | 公开：返回是否启用、最大层级、默认天数 |
| `GET`  | `/invite/me` | 当前用户的上下级、层级、能否邀请 |
| `POST` | `/invite/codes` | 生成邀请码 |
| `GET`  | `/invite/codes` | 列出我生成的邀请码 |
| `DELETE` | `/invite/codes/<code>` | 撤销/删除我生成的邀请码 |
| `POST` | `/invite/check` | 公开：校验邀请码是否可用（按 IP 限流） |
| `POST` | `/invite/use` | 已登录用户使用邀请码创建 Emby 账号 |

### 1.6 管理员接口

| Method | Path | 描述 |
| ------ | ---- | ---- |
| `GET`  | `/admin/invite/tree` | 获取整棵森林（节点 + 边 + 树根 + 配置） |
| `POST` | `/admin/invite/users/<uid>/detach` | 把指定用户从上级断开（自身晋升新树根） |
| `GET`  | `/admin/invite/codes?inviter_uid=<uid>` | 按邀请人列出邀请码 |
| `DELETE` | `/admin/users/<uid>` | 删除用户。新支持 `cascade_depth` 参数（见下） |

`DELETE /admin/users/<uid>` 请求体扩展：

```json
{
  "mode": "with_emby",
  "cascade_depth": 1
}
```

字段说明：

* **`mode`**（推荐）：
  * `with_emby`：本地账户 + Emby 账户一起删（清理邀请关系）
  * `local_only`：仅删本地账户（清理邀请关系，保留 Emby 账号）
  * `emby_only`：仅删 Emby 账号（**本地账户与邀请关系完全保留**）
* **兼容旧字段** `delete_emby`：未传 `mode` 时仍生效（`true` → `with_emby`，`false` → `local_only`）。
* **`cascade_depth`**：
  * `1`（默认）：仅本人
  * `2`：本人 + 直接下级
  * `N`：本人 + 下 N-1 层
  * `0` 或 `>= 999`：整棵子树（不限层级）
* 三种 `mode` 都会按 `cascade_depth` 级联生效。例如 `mode=emby_only, cascade_depth=2`
  会同时删除该用户及其直接下级的 Emby 账号，但保留所有本地账户和上下级关系。
* 当遇到下级里的 **管理员账号** 时会跳过并记录到 `skipped`，避免误删平台管理员。
* 返回结构包含 `deleted` / `skipped` / `failed` 三列以及最终采用的 `mode` 与 `cascade_depth`。

### 1.7 核心校验

* `ensure_can_invite`：
  1. 必须 `invite_enabled = true`；
  2. 邀请人账号启用；
  3. 若 `invite_require_emby = true`，必须已绑定 Emby；
  4. 当前层级未达 `invite_max_depth`；
  5. 未使用的邀请码未达 `invite_limit`。
* `apply_invite`：
  * 不能使用自己生成的邀请码；
  * `inviter_depth + 1 > invite_max_depth` 直接拒绝；
  * 原子地消费 `USE_COUNT`，再写入 `invite_relations`。

### 1.8 删除语义

| 场景 | 邀请关系处理 |
| ---- | ----------- |
| 普通删除（`mode=with_emby/local_only`, `cascade_depth=1`） | 该用户为 PARENT 的所有边删掉，子节点晋升为新树根；该用户为 CHILD 的边一并删掉 |
| 级联删除（`mode=with_emby/local_only`, `cascade_depth>=2` 或 `=0`） | 先 BFS 收集 N 层 UID，再按叶子→根顺序逐个走「普通删除」 |
| 仅停用（`active=false`） | 邀请关系完全不变；重新启用即可恢复访问 |
| `mode=emby_only`（任意 `cascade_depth`） | 仅删 Emby 账号；本地账号、上下级、邀请码全部保留 |

---

## 2. 公告系统

### 2.1 字段变化

`announcements` 表新增列：

| 字段 | 类型 | 默认 | 含义 |
| ---- | ---- | ---- | ---- |
| `RENDER_MODE` | `VARCHAR(16)` | `'plain'` | 渲染方式：`plain` / `markdown` / `bbcode` |

老库会在启动时自动 `ALTER TABLE` 增加该列。

API 序列化新增 `render_mode` 字段；管理员创建 / 更新接口允许传 `render_mode`，
也兼容 `text` / `md` / `bb` 等别名（会规范化）。

### 2.2 安全约束（前端）

公告内容由 **后端原样保存**，所有解析都在前端 React 树里完成，
**永远不会触发 `dangerouslySetInnerHTML`**，从根上避免 XSS。

* **Markdown** 渲染：手写小子集，支持 `#~######` 标题、`**粗体**`、`*斜体*`、
  `_斜体_`、`~~删除线~~`、行内代码 `` ` ``、围栏代码块 ```` ``` ````、
  引用 `>`、无序 `- * +`、有序 `1.`、`[label](url)` 链接、自动链接、
  分割线 `--- *** ___`，以及反斜杠转义。
* **BBCode** 渲染：基于栈的解析，**仅** 白名单以下标签，
  其他标签按字面输出：
  * `[b] [i] [u] [s]`
  * `[code]…[/code]`：含换行时升级为块级 `<pre>`
  * `[quote]` / `[quote=作者]`
  * `[url=…]…[/url]` / `[url]…[/url]`：URL 必须为 `http(s)/mailto/相对路径/锚点`，
    否则降级为纯文本
  * `[color=#fff]` / `[color=red]`：仅允许 `#RGB` `#RRGGBB` 或纯字母色名
  * `[size=14px]` / `[size=3]`：仅允许 1-7 或 8-36px
  * `[list]` / `[list=1]` + `[*]`：无序 / 有序列表

* **纯文本** 模式：保留换行 (`whitespace-pre-wrap`)，所有 `<>` 等字符
  由 React 自动 HTML-escape。

### 2.3 管理员页面

`/admin/announcements` 创建/编辑表单新增「渲染方式」下拉框；
表单下方有「预览」区，实时显示当前渲染效果，便于在发布前检查。

### 2.4 仪表盘位置

按需求公告板已从仪表盘顶部下移到**最后一个区块**，
避免占据首屏；登录页 / `/announcements` 独立页面仍正常展示。

---

## 3. 安全 / 兼容性回归点

| 修复 | 位置 |
| ---- | ---- |
| 公告渲染避免 raw HTML | `webui/src/lib/safe-render.tsx` |
| URL/颜色/尺寸白名单 | 同上 |
| 邀请码 IP 限流（公开校验） | `src/api/v1/invite.py::check_invite_code` |
| 邀请人/被邀请人自洽校验 | `src/services/invite_service.py::apply_invite` |
| 删除用户自动清理邀请关系，避免悬空 | `src/services/user_service.py::delete_user` |
| 移动端布局 | `globals.css` (dvh / 防溢出), `header.tsx`, `layout.tsx`, admin pages |

---

## 4. 升级指南

1. **拉取代码后无需手动迁移**：首次启动会自动建 `invites.db`、自动
   `ALTER` `announcements.RENDER_MODE`。
2. **如要启用邀请**：管理员在「配置 → 注册与用户策略」中打开
   `invite_enabled` 并按需调整 `invite_max_depth` / `invite_limit`。
3. **公告渲染**：默认 `plain`，对老数据完全等价；启用 Markdown / BBCode 时
   建议先在管理员页面预览确认。
