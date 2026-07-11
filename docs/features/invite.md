# 邀请树

Twilight 的邀请树（Invite Tree）让已注册用户互相邀请生成新的 Emby 账号，形成一片由多棵树组成的「森林」。本文说明邀请树的概念、配置项、前后端入口、用户/管理员接口以及删除与启停的级联语义，所有行为均对照 `internal/api/invite_handlers.go`、`internal/api/invite_admin_handlers.go`、`internal/api/business.go`、`internal/store/store.go`、`internal/config/config.go` 核对。

> 邀请关系与邀请码是「单一状态文档」（`internal/store`）里的字段，不是独立的数据库或单表。JSON 后端存于 `db/twilight_go_state.json`，PostgreSQL 后端存于 `twilight_state` 表（`id=1` 的一行 jsonb）。具体对应字段为 `state.invite_codes`（`map[string]InviteCode`）与 `state.invite_relations`（`map[int64]InviteRelation`）。不存在 `db/invites.db`、`invite_relations` 单表或「首次启动自动建表」。

相关文档：注册码与卡码见 [注册码与卡码](./regcodes.md)，统一卡码入口见 [后端 API 详参](../reference/backend-api.md)，配置项总览见 [Go 后端架构与配置](../reference/backend.md)，全部路由见 [API 路由索引](../reference/api-index.md)。

## 概念

| 术语 | 含义 |
| ---- | ---- |
| 树根（root） | 邀请关系图中没有上级（在 `invite_relations` 中没有以自己为 `child` 的关系）的节点。 |
| 层级（depth） | 从该节点向上回溯到根的层数，根本身 = 1。示例 `C → A → B`（C 邀请了 A，A 邀请了 B）：C=1，A=2，B=3，整树深度为 3。 |
| 子树（subtree） | 以某节点为根、向下递归的所有后代集合。 |
| 断开（detach） | 删除某用户作为 `child` 的那条边（即抹掉它的上级指向）。它名下的子节点不变，但它自己晋升为新的树根。 |
| 级联删除（cascade delete） | 以某节点为起点，按指定层级（`cascade_depth`）一并删除若干代后代。 |

数据模型（`internal/store/store.go`）：

- `InviteRelation`：`{ parent_uid, child_uid, code, created_at }`。在状态文档里以 `child_uid` 作为 map 键，因此「一个用户最多只有一个上级」由结构天然保证。
- `InviteCode`：`{ code, uid, inviter_uid, days, use_count_limit, use_count, used_by_uid, used_at, active, note, used, target_username, created_at, expired_at }`。邀请码由后端生成时强制 `use_count_limit = 1`（一次性使用）。

## 配置项（`[SAR]` 段）

邀请相关字段位于配置文件 `[SAR]` 段（旧版 `[Register]` 段的同名键也兼容读取）。下表的字段名与默认值对照 `internal/config/config.go` 的 `defaults()` 与 `Load()`：

| 字段 | 默认 | 说明 |
| ---- | ---- | ---- |
| `invite_enabled` | `true` | 邀请系统总开关。关闭后不能生成 / 使用新的邀请码；已有直属下级仍可查看，并可继续生成专属续期码或清理到期 Emby 绑定。 |
| `invite_max_depth` | `3` | 整棵邀请树允许的最大层级。生成 / 使用邀请码时，若邀请人当前层级已 `>= invite_max_depth` 则拒绝（无法再向下扩展一层）。 |
| `invite_limit` | `10` | 每位用户**未使用**邀请码的上限（已使用 / 已失效 / 已过期的不计入）。`-1` 表示无限制。 |
| `invite_root_user_limit` | `-1` | 每棵邀请树最多可成功邀请多少用户（按整棵子树后代计数，不含树根本人）。`-1` 表示无限制；仅当 `> 0` 时生效。 |
| `invite_require_emby` | `false` | 是否要求邀请人已绑定 Emby 才能生成邀请码 / 续期码。 |
| `invite_code_default_days` | `30` | 生成邀请码时未显式传 `days` 时采用的默认开通天数。 |
| `permanent_invite_max_days` | `365` | 永久号 / 未设过期的邀请人可签发的邀请码 / 续期码天数上限，也是 `maxCodeDays` 的封顶值。 |

> 配置键也兼容历史别名 `SAR.invite_default_days`（映射到 `invite_code_default_days`）。

修改这些字段后会触发整进程重启（保存 `config.toml` 后由外部 supervisor 拉起），详见 [开发指南](../guides/development.md) 与 [Go 后端架构与配置](../reference/backend.md)。

> 邀请码格式是固定的：后端生成时取 `"INV" + 10 位随机串并转大写（形如 `INVXXXXXXXXXX`）。不存在 `invite_code_format` 配置项；`/invite/config` 返回的 `code_format` 字段是常量字符串 `"INV-{random}"`，仅供前端展示提示，不参与实际生成。

## 前端入口

- **普通用户**：侧边栏「邀请中心」`/invite`（`webui/src/app/(main)/invite/page.tsx`）
  - 查看自己的层级、直属上级、完整下级树（不返回多层上级信息）。
  - 生成 / 复制 / 撤销邀请码（仅当邀请系统开启）。
  - 为已有直属下级生成专属续期码；对 Emby 已到期或 Web 已禁用且仍绑定 Emby 的直属下级，可删除其 Emby 账号并断开关系。
- **管理员**：侧边栏「邀请系统管理」`/admin/invite`（`webui/src/app/(main)/admin/invite/page.tsx`）
  - 查看邀请关系、根用户、直属下级与总下级统计。
  - 点击用户查看详情、解除上级关系、级联启停或删除。

当 `invite_enabled` 为关闭时，前端会禁用「生成邀请码」，但保留已有下级的续期码与 Emby 清理入口；管理员「邀请系统管理」入口和后端关系接口不随开关隐藏，便于继续审计和维护既有关系。

## 用户接口

均挂在 `/api/v1` 前缀下，路由见 `internal/api/routes.go` 的 `registerStatsInviteSigninAnnouncementDemoRoutes`。鉴权级别说明见 [API 路由索引](../reference/api-index.md)。

| Method | Path | 鉴权 | 描述 |
| ------ | ---- | ---- | ---- |
| `GET` | `/invite/config` | AuthPublic | 公开返回邀请系统配置：`enabled` / `max_depth` / `invite_limit` / `invite_root_user_limit` / `require_emby` / `default_days` / `code_format` / `permanent_invite_max_days`。 |
| `GET` | `/invite/me` | AuthUser | 当前用户的上级（`parent`）、下级列表（`children`）、子树（`tree`）、层级（`depth`）、能否邀请（`can_invite` + `invite_block_reason`）、可签发的最大天数（`max_code_days`）及自己生成的邀请码列表（`codes`）。 |
| `POST` | `/invite/codes` | AuthUser | 生成邀请码。仅当 `invite_enabled=true` 可用；可选 body：`days`、`expires_at`、`note`、`target_username`。按 UID 限速 10 次/分钟。 |
| `GET` | `/invite/codes` | AuthUser | 列出我生成的邀请码。 |
| `DELETE` | `/invite/codes/:code` | AuthUser | 撤销 / 删除我生成的邀请码（仅限本人创建的码）。 |
| `POST` | `/invite/renew-codes` | AuthUser | 为已加入邀请树的「直属下级」生成专属续期码（实际生成的是 `type=2` 续期注册码，见下）。邀请系统关闭后仍允许给既有直属下级生成。 |
| `POST` | `/invite/children/:uid/detach-expired` | AuthUser | 删除 Emby 并断开直属下级：仅允许目标 Emby 已到期，或 Web 已禁用且仍绑定 Emby；清空绑定与待开通状态并解除上下级关系。 |
| `GET` | `/invite/check` | AuthPublic | 公开校验邀请码是否可用，命中返回 `days` 与邀请人用户名 `inviter`。按 IP 限速 10 次/分钟，防止扫描邀请码空间泄露邀请人信息。 |
| `POST` | `/invite/use` | AuthUser | 已登录用户使用邀请码加入邀请树并标记待开通 Emby。按 UID 限速 10 次/分钟。**Web 前端统一走 `/users/me/use-code`（兼容卡码 / 邀请码两类），此接口为兼容入口。** |

### 生成邀请码（`POST /invite/codes`）

校验顺序（`handleCreateInviteCode`）：UID 限速 → `canInvite`（见下）→ `days` 必须在 `(0, maxCodeDays]` 区间内（`maxCodeDays` 由邀请人剩余有效期推出，永久号上限为 `permanent_invite_max_days`）→ 可选 `expires_at` 必须晚于当前时间 → 可选 `target_username` 须为 3-32 字符且不含路径 / 注入字符 → 生成不冲突的 `INV...` 码并写入状态文档。

### 续期码（`POST /invite/renew-codes`）

为「直属下级」签发的实际上是一张一次性、指名的续期注册码（`type=2`，`target_username` 固定为下级用户名，默认格式 `REN-{random}`），见 [注册码与卡码](./regcodes.md)。校验要点（`handleCreateInviteRenewCode`）：邀请人账号需 `Active`、未过期（`userEntitlementOK`）、若 `invite_require_emby=true` 则需已绑定 Emby；目标必须是当前用户的直属下级且 Web 账号仍启用；续期天数受 `maxCodeDays` 封顶；`validity_hours` 限制在 `1-720` 小时。`invite_enabled=false` 只禁止新邀请码，不阻止既有直属下级续期码。

## 管理员接口

| Method | Path | 鉴权 | 描述 |
| ------ | ---- | ---- | ---- |
| `GET` | `/admin/invite/tree` | AuthAdmin | 返回整片森林：`nodes`（节点）+ `edges`（边）+ `roots`（树根 UID 列表）+ `max_depth`（全局最大深度）+ `config`（当前配置）。 |
| `POST` | `/admin/invite/users/:uid/detach` | AuthAdmin | 把指定用户从上级断开（删除其作为 `child` 的边，自身晋升新树根）。返回 `changed` 表示原本是否有上级。 |
| `GET` | `/admin/invite/codes` | AuthAdmin | 列出全部邀请码（可按邀请人在前端过滤）。 |
| `POST` | `/admin/users/:uid/delete` | AuthAdmin | 删除用户，支持 JSON body 的 `mode` 与 `cascade_depth`（见下，推荐）。 |
| `DELETE` | `/admin/users/:uid` | AuthAdmin | 删除用户兼容入口，保留简单删除和旧客户端调用。 |
| `POST` | `/admin/users/:uid/disable` | AuthAdmin | 禁用用户，支持 `cascade_depth` 级联（见下）。 |
| `POST` | `/admin/users/:uid/enable` | AuthAdmin | 启用用户，支持 `cascade_depth` 级联（见下）。 |

> `/admin/users/:uid/delete`、`/admin/users/:uid`、`/admin/users/:uid/disable`、`/admin/users/:uid/enable` 是通用的用户管理接口，并非邀请模块专属，但其级联参数会沿邀请树展开，因此与邀请树语义强相关，下文一并说明。

### 删除用户（`POST /admin/users/:uid/delete`）

请求体扩展：

```json
{
  "mode": "with_emby",
  "cascade_depth": 1
}
```

`mode`（默认 `local_only`）：

- `with_emby`：删除本地账户 + Emby 账户，并清理邀请关系。
- `local_only`：仅删除本地账户（保留 Emby 账号），并清理邀请关系。
- `emby_only`：仅删除 Emby 账号；**本地账户、上下级关系、邀请码全部保留**（仅清空该用户的 `EmbyID` / `EmbyUsername` 字段）。

`cascade_depth`（默认 `1`，也可通过 query `?cascade_depth=` 传）语义由 `collectCascadeUIDs`（`internal/api/business.go`）决定：

| 取值 | 含义 |
| ---- | ---- |
| `1` | 仅本人 |
| `2` | 本人 + 直接下级 |
| `N`（≥2） | 本人 + 下 N-1 层 |
| `0` | 收紧为「仅本人」（等价于 `1`，避免误传 0 触发全树遍历） |
| `< 0` 或 `>= 999` | 整棵子树（不限层级） |

> 单次级联最多展开 `cascadeMaxResults = 5000` 个 UID（`internal/api/business.go`），超出后截断。

其它行为：

- 三种 `mode` 都会按 `cascade_depth` 级联。例如 `mode=emby_only, cascade_depth=2` 会删除该用户及其直接下级的 Emby 账号，但保留所有本地账户与上下级关系。
- 不能删除「当前登录管理员自己」（`USER_PROTECTED`）。
- 遇到受保护账号（如配置中的管理员）会跳过并记入 `skipped`，避免误删平台管理员。
- 返回结构：`{ deleted: [uid], skipped: [{uid, reason}], failed: [{uid, reason}], mode, cascade_depth }`。

> 真正删除本地账户时，`store.DeleteUser`（`internal/store/store.go`）会级联清理该用户的 `invite_codes`（邀请人 / 接收人任一为该用户的码都删除）与 `invite_relations`（自身作为 `child` 的边 + 自身作为 `parent` 的所有边都删除），从而让原本的直属下级失去上级、晋升为新的树根，不留悬空边。同时还清理该用户的 API Key、求片、签到、设备指纹、登录日志、播放记录、待审换绑请求等。

### 启停级联（`POST /admin/users/:uid/disable` 与 `/enable`）

请求体：

```json
{
  "cascade_depth": 1,
  "reason": "可选"
}
```

- `cascade_depth` 语义与删除接口完全一致（`1`=仅本人，`N`=本人+下 N-1 层，`0`=仅本人，`<0`/`>=999`=整棵子树）。
- 仅翻转用户的 `Active` 状态并同步到 Emby（启用/禁用 Emby 账号）；**邀请关系完全不变**，重新启用即可恢复访问。
- 管理员不允许禁用自己（`USER_PROTECTED`）。
- 翻转通过 `SetUserActiveAtomic` 原子执行：若会禁用最后一个仍处于启用状态的管理员，返回 `ErrLastAdmin`，在级联场景下记入 `skipped`（reason=`last_admin`）而不会悄悄通过。
- 其他受保护账号自动跳过（记入 `skipped`）。
- 返回结构：`{ user, active, affected: [uid], skipped: [{uid, reason}], failed: [{uid, reason}], cascade_depth, enable }`。

## 删除 / 断开 / 启停语义对照

| 场景 | 邀请关系处理 |
| ---- | ----------- |
| 普通删除（`mode=with_emby` 或 `local_only`，`cascade_depth=1`） | 删除该用户作为 `child` 的边 + 作为 `parent` 的所有边；其直属下级失去上级、晋升为新树根。 |
| 级联删除（`cascade_depth>=2` 或 `<0`/`>=999`） | 先按 `cascade_depth` 用 BFS 收集层级内 UID，再逐个执行删除（每个被删用户都触发上述关系清理）。 |
| 仅停用 / 启用（`cascade_depth=1`） | 邀请关系完全不变；仅翻转 `Active` 并同步 Emby。 |
| 级联禁用 / 启用（`cascade_depth>=2` 或 `<0`/`>=999`） | 仅翻转层级内各用户的 `Active` 并同步 Emby；邀请关系完全不动；受保护管理员自动跳过。 |
| `mode=emby_only`（任意 `cascade_depth`） | 仅删除 Emby 账号并清空绑定字段；本地账号、上下级、邀请码全部保留。 |
| 用户自助断开（`POST /invite/children/:uid/detach-expired`） | 仅当目标是 Emby 已到期，或 Web 已禁用且仍绑定 Emby 的直属下级时，删除其 Emby 账号、清空绑定与待开通状态，并解除上下级关系；不改变目标 Web 账号的启用 / 禁用状态。 |
| 管理员断开（`POST /admin/invite/users/:uid/detach`） | 仅删除该用户作为 `child` 的边，自身晋升新树根；不动其 Emby 账号与下级。 |

## 核心校验

### `canInvite`（`internal/api/business.go`）

生成邀请码前的统一判定，依次检查：

1. `invite_enabled = true`（否则「邀请系统未启用」）。
2. 邀请人账号 `Active`（否则「账号已被禁用」）。
3. `userEntitlementOK`：账号未过期（`Active=true` 且 `ExpiredAt` 不在过去）。这是与 `maxCodeDays` 重叠的显式防御纵深，确保「Active 但已过期」的账号无法签发邀请码。
4. 若 `invite_require_emby = true`，邀请人必须已绑定 Emby。
5. `maxCodeDays(user) > 0`：邀请人剩余有效期足以分配天数（永久号取 `permanent_invite_max_days`）。
6. 当前层级 `inviteDepth(uid) < invite_max_depth`。
7. 若 `invite_root_user_limit > 0`，整棵树的后代数量未达上限（`inviteDescendantCount(rootUID)`）。
8. 若 `invite_limit != -1`，未使用的邀请码数量未达 `invite_limit`（仅统计 `active && use_count==0 && 未过期` 的码）。

### 使用邀请码（`handleInviteUse` / `ConsumeInviteCodeAndUpdateUser`）

校验顺序（`handleInviteUse`）：UID 限速 → 邀请码非空 → 当前账号未绑定 Emby（`INVITE_EMBY_ALREADY_BOUND`）→ 邀请码存在且 `active` 且未过期 → 若码限定了 `target_username` 则必须匹配 → 不能使用自己生成的邀请码（`INVITE_SELF_GENERATE`）→ 当前账号尚无上级（`INVITE_ALREADY_HAS_PARENT`，一个用户只能加入一棵树一次）→ 邀请人存在且 `Active` → 邀请人层级未达 `invite_max_depth`（`INVITE_DEPTH_EXCEEDED`）→ 若设了 `invite_root_user_limit` 则树未满（`INVITE_ROOT_FULL`）→ 邀请人 `maxCodeDays > 0`（`INVITER_DAYS_SHORT`）→ Emby 容量未达上限（`EMBY_CAPACITY_REACHED`）。

通过后：

- 实际开通天数 `effectiveDays` 取邀请码的 `days`，若 `<=0` 或超过邀请人剩余天数则收敛到 `maxDays`。
- **原子消费**：`store.ConsumeInviteCodeAndUpdateUser` 在一次加锁的状态变更里完成「`use_count++` / 标记 `used` / 达到 `use_count_limit` 时置 `active=false`」、写入 `invite_relations[childUID]`（指向邀请人）以及更新被邀请人权益。其中再次校验邀请码 `active`、未超用量上限、未过期、邀请人不等于使用者，任一不满足即整体失败回滚。
- 被邀请人会被标记为 `PendingEmby`（待开通），写入用户级 `emby_grant_locked=true`，并把过期时间按 `effectiveDays` 顺延，且不会超过邀请人的过期时间（`boundedInviteExpiry`）。该锁不会因后续删除邀请码或断开邀请关系而解除。

> 邀请码生成时即固定 `use_count_limit = 1`，因此每张邀请码只能被使用一次；用过的码自动失效。被本人撤销时，若已被使用过则保留记录并置 `active=false`，否则直接从状态文档删除（`store.DeleteInviteCode`）。

## 相关错误码

邀请相关错误码定义于 `internal/api/errcode.go`，前端文案见 `webui/src/lib/errcode.ts`。统一响应 envelope 为 `{ success, code, message, data, timestamp }`，code 取下列值之一：

| 错误码 | 触发场景 |
| ------ | -------- |
| `INVITE_DISABLED` | 邀请系统未开启时尝试生成 / 使用新邀请码。 |
| `INVITE_CANNOT_INVITE` | `canInvite` 判定不通过（message 携带具体原因）。 |
| `INVITE_NOT_FOUND` | 邀请码无效 / 已停用 / 已过期 / 邀请人不可用。 |
| `INVITE_SELF_GENERATE` | 使用自己生成的邀请码。 |
| `INVITE_ALREADY_HAS_PARENT` | 当前账号已有上级，不能重复加入。 |
| `INVITE_EMBY_ALREADY_BOUND` | 当前账号已绑定 Emby，不能再用邀请码新建。 |
| `INVITE_TARGET_MISMATCH` | 邀请码限定了 `target_username`，与当前用户不符。 |
| `INVITER_UNAVAILABLE` | 邀请人不存在或已禁用。 |
| `INVITE_DEPTH_EXCEEDED` | 邀请树层级已达上限。 |
| `INVITE_ROOT_FULL` | 邀请树人数已达 `invite_root_user_limit`。 |
| `INVITER_DAYS_SHORT` | 邀请人剩余有效期不足。 |
| `INVITE_DAYS_OUT_OF_RANGE` / `INVITE_RENEW_DAYS_OUT_OF_RANGE` | 邀请码 / 续期码天数超出允许范围。 |
| `INVITE_EXPIRES_BEFORE_NOW` | 邀请码过期时间早于当前时间。 |
| `INVITE_TARGET_USERNAME_INVALID` | 目标用户名格式非法。 |
| `INVITE_GENERATION_CONFLICT` | 多次尝试仍无法生成不冲突的码。 |
| `INVITE_RENEW_USER_DISABLED` / `INVITE_RENEW_REQUIRES_EMBY` / `INVITE_RENEW_BAD_TARGET` / `INVITE_RENEW_NOT_DIRECT_CHILD` | 续期码相关校验失败。 |
| `INVITE_DETACH_NOT_DIRECT_CHILD` / `INVITE_DETACH_NOT_EXPIRED` | 自助断开下级的前置校验失败。 |
| `RATE_LIMITED` | 触发限速（生成 / 使用 / 公开校验）。 |
