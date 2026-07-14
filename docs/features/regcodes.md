# 注册码与卡码

本文说明 Twilight 的注册码、续期码、白名单码、诱饵码，以及邀请码经统一入口接入的口径，并对照后端代码（`internal/api/regcode_handlers.go`、`internal/api/code_use_handlers.go`、`internal/api/business.go`、`internal/store/store.go`）核对了字段、生成算法、消费原子性与鉴权限流。

「卡码」在本项目中统称管理员发放的各类一次性或多次性凭证。除邀请码（`InviteCode`，见 [邀请树](./invite.md)）外，注册码 / 续期码 / 白名单码 / 诱饵码都使用同一份 `RegCode` 数据结构，仅靠 `type` 与 `is_decoy` 区分。

## 存储模型

注册码与全部业务状态一样，保存在「单一状态文档」中——要么是 JSON 文件 `db/twilight_go_state.json`，要么是 PostgreSQL `twilight_state` 表（`id=1` 的一行 `jsonb`）。`internal/store` 中 `RegCode` 以 `code` 字符串为键存放于状态文档的 `RegCodes` 映射里，**不存在**独立的注册码数据库文件或单独的表，也没有「建表 / ALTER TABLE」之类的迁移步骤。

## 类型

注册码的核心区分字段是 `type`（整数 `1`/`2`/`3`）。邀请码不属于 `RegCode`，但前端把它和注册码统一收口到 `/users/me/use-code`，后端 `previewCode`（`internal/api/business.go`）自动按字符串先查注册码、再查邀请码来识别来源。

| type / 来源 | 名称 | 使用入口 | 行为 |
| ---- | ---- | -------- | ---- |
| `1` | 注册码 | `POST /api/v1/users/register`、`POST /api/v1/users/me/use-code` | 公开注册时建系统账号并标记 `PendingEmby`（待开通 Emby）；已登录且未绑定 Emby 的用户用它将角色置为普通用户、按 `days` 写入 `PendingEmbyDays`，并续期账号有效期。后续既可创建新的 Emby 账号，也可绑定已有 Emby 账号并消耗该资格。 |
| `2` | 续期码 | `POST /api/v1/users/me/renew`、`POST /api/v1/users/me/use-code` | 已登录用户续期；按 `days` 在原有效期基础上叠加，`days<0` 表示永久，`days=0` 按 30 天处理。 |
| `3` | 白名单码 | `POST /api/v1/users/me/use-code` | 将角色升为白名单（`RoleWhitelist`）、`Active=true`、有效期置为永久；未绑定 Emby 时同时标记 `PendingEmby`（永久天数）以补建 Emby 账号。 |
| 邀请码（`source=invite`） | 邀请码 | `POST /api/v1/users/me/use-code` | 已登录且未绑定 Emby 的用户获得 Emby 开通资格并加入邀请树；后续可创建新的 Emby 账号或绑定已有 Emby 账号。天数受邀请人剩余有效期约束。详见 [邀请树](./invite.md)。 |

> 说明：`previewCode` 把邀请码统一映射为「注册码 type=1」的预览形态（`source=invite`），所以前端只需调一个接口。注册（type=1）、白名单（type=3）以及邀请码都会授予 Emby 开通资格（`codeGrantsEmbyRegistration`）；若当前账号已绑定 Emby，使用这三类会被拒绝并提示改用续期码。待开通资格完成时，无论用户选择创建新 Emby 账号还是绑定已有 Emby 账号，后端都会清理 `PendingEmby` 并按资格天数设置账号有效期。

## 字段

`RegCode` 结构（`internal/store/store.go`）：

| 字段（JSON） | 说明 |
| ---- | ---- |
| `code` | 卡码字符串，状态文档中的键。新格式可由 `format` + `random_algorithm` 生成；旧卡码字符串继续原样可用。 |
| `type` | 卡码类型：`1` 注册、`2` 续期、`3` 白名单。 |
| `days` | 授予或增加的账号天数；读出与消费时经 `normalizeRegCodeDays` 规范化，`0` 按 30 天处理，负数统一为 `-1`（永久）。 |
| `validity_time` | 卡码自身有效期，单位「小时」；`-1` 表示永久。判定过期：`created_at + validity_time*3600 <= now`。 |
| `use_count_limit` | 使用次数上限；`-1` 表示不限次数，正整数表示固定次数。 |
| `use_count` | 已使用次数。 |
| `used_by` / `used_by_uids` / `used_by_telegram_ids` | 使用者记录（最近一次 UID、去重后的 UID 列表、去重后的 Telegram ID 列表），仅管理员页面可查。 |
| `active` | 是否启用；停用后不可使用。用满次数时会被自动置为 `false`。 |
| `is_decoy` | 是否为诱饵码（蜜罐）。 |
| `target_username` | 指名卡码：非空时仅限该 Web 用户名（不区分大小写）使用。 |
| `target_telegram_username` / `target_telegram_id` | 指名卡码：非空时仅限匹配的 Telegram 用户名或 Telegram ID 使用；注册时需提供已确认的 Telegram 注册绑定码。 |
| `created_at` / `created_time` | 创建时间戳（秒）；`created_time` 缺省时回落到 `created_at`。 |
| `note` | 管理员备注（创建时截断到 120 字符）。 |
| `source` | 卡码来源：`"admin"`（管理员手动创建）或 `"invite"`（邀请系统自动生成续期码）。历史数据为空字符串，视作 `"admin"`。 |
| `creator_uid` | 创建者 UID。管理员创建时为管理员 UID，邀请续期码为邀请人 UID。 |

> `OTHER`（JSON 元数据）是旧 Python 实现的字段，Go 版本已将 `decoy`、`target_username`、`target_telegram_username`、`target_telegram_id`、`note` 提升为结构体一级字段，不再有 `OTHER` 包裹。

### 来源区分（Source 字段）

`source` 字段区分卡码的创建来源：

- **`"admin"`**：管理员在后台卡码管理页手动生成（`POST /admin/regcodes`）。列表中属于注册码 Tab，默认筛选 `source=admin`。
- **`"invite"`**：邀请系统自动生成（`POST /invite/renew-codes`），邀请人为下级创建续期码时写入 `Source: "invite"`。前端在邀请码 Tab 以合并视图展示（与 `InviteCode` 条目同列渲染）。

管理员列表接口 `GET /admin/regcodes` 支持 `?source=admin|invite` 筛选参数。后端 `handleListRegcodes` 中，`sourceFilter=admin` 会匹配显式 `"admin"` 和历史空值；`sourceFilter=invite` 仅匹配 `"invite"`。

### 取值校验与规范化

创建接口 `handleCreateRegcodes`（`internal/api/regcode_handlers.go`）的校验口径：

- `type`：必须在 `1`-`3` 之间，否则返回 `REGCODE_TYPE_INVALID`。
- `days`：经 `normalizeRegCodeDays` 处理，`0` 规范化为 30 天，负数规范化为 `-1`（永久）；正整数原样保留。管理员要发永久码必须显式传 `-1`。
- `validity_time`：传 `0` 自动转 `-1`；小于 `-1` 报错「卡码有效期只能为 -1 或正整数小时」。
- `use_count_limit`：传 `0` 自动转 `1`；小于 `-1` 报错「使用次数上限只能为 -1 或正整数」。
- `count`：批量生成数量，自动夹取到 `1`-`100`。
- `target_username`：可选；非空时长度须为 3-32 字符，且不含 `/ \ @ : <空字符> < > " ' &`，否则返回 `REGCODE_TARGET_BAD`。
- `target_telegram_username`：可选；可带或不带 `@`，保存时会去掉 `@` 并转小写；长度须为 5-32 字符，只能包含字母、数字和下划线。
- `target_telegram_id`：可选；必须为正整数。
- 三种指名目标（Web 用户名 / TG 用户名 / TG ID）只能指定一种，否则返回 `REGCODE_TARGET_BAD`。

> 持久化时 `UpsertRegCode` 还会兜底：`validity_time==0 -> -1`、`use_count_limit==0 -> 1`、新建且未使用却 `active=false` 时强制 `active=true`。

## 生成格式与随机算法

管理员创建接口 `POST /api/v1/admin/regcodes` 请求示例：

```json
{
  "type": 1,
  "validity_time": -1,
  "use_count_limit": 1,
  "days": 30,
  "count": 1,
  "format": "TW-{type}-{random}",
  "random_algorithm": "base32-20",
  "decoy": false,
  "target_username": "",
  "target_telegram_username": "",
  "target_telegram_id": null,
  "note": ""
}
```

`format` 与 `random_algorithm` 缺省时回落到配置 `[SAR].regcode_format` / `[SAR].regcode_random_algorithm`，再回落到内置默认 `TW-{type}-{random}` 与 `base32-20`。

`generateRegCode`（`internal/api/business.go`）支持的占位符：

| 占位符 | 替换为 |
| ---- | ---- |
| `{random}` | 按随机算法生成的随机串（若 `format` 不含该占位符，会自动追加 `-{random}`）。 |
| `{type}` | 类型缩写：`1`→`REG`、`2`→`REN`、`3`→`VIP`。 |
| `{days}` | 天数（规范化后的整数）。 |
| `{index}` | 批量生成时的序号（从 1 开始）。 |
| `{validity}` | 有效期小时数。 |
| `{limit}` | 使用次数上限。 |

可用随机算法（`regCodeRandomPart`）。未识别的算法名一律回落到 `base32-20`：

| 算法 | 说明 |
| ---- | ---- |
| `base32-20` | 默认推荐。20 位易抄写大写字符（去除 `0/1/I/O` 等易混淆字符），适合人工复制和公开发放。 |
| `base32-16` / `base32-24` / `base32-32` | 同字符集的 16/24/32 位变体，长度越长强度越高。 |
| `hex` / `hex20` | 20 位十六进制（大写）。 |
| `hex32` / `hex40` | 32 / 40 位十六进制，适合系统间导入导出。 |
| `alnum-16` / `alnum-24` / `alnum-32` | 易抄写大写字母数字（同 base32 字符集）。 |
| `urlsafe-24` / `urlsafe-32` | URL 安全字符，含大小写字母、数字、`-`、`_`。 |
| `digits-12` / `digits-16` | 纯数字；`digits-12` 仅建议口头传递或低风险场景。 |
| `symbols-16` / `symbols-24` | 大写字母数字 + 部分符号（`!@$%^*_-+=.:`），最高强度。 |
| `uuid` | UUID v4 风格（大写）。 |
| `legacy-sha1` | 旧版 40 位十六进制风格，仅用于兼容历史样式。 |

随机部分由 `crypto/rand` 提供熵（`randomCode` 内部调用 `crypto/rand.Read`，失败直接 panic，不会退化为弱随机）。

### 生成时的去重与冲突

每个卡码最多尝试 20 次：本批已生成的（`seen`）与状态文档中已存在的都视为冲突跳过；20 次仍冲突则返回 `REGCODE_GENERATE_CONFLICT`，提示调整格式或算法。

## 消费的原子性

注册码的「校验 + 计数 + 标记使用者」全程在状态存储的全局写锁下原子完成，避免同一码并发使用导致超额消费：

- `ConsumeRegCode`（`internal/store/store.go`）先 `s.mu.Lock()`，再在 `mutateAndSaveLocked` 内依次调用 `consumableRegCodeLocked` 与 `consumeRegCodeLocked`，两步在同一把锁内不可分割。注册码写入状态文档并跨重启保留。
- `consumableRegCodeLocked` 校验：卡码存在且 `active`，否则 `ErrNotFound`；`use_count_limit != -1 && use_count >= use_count_limit` 则 `ErrConflict`（已用满）；`validity_time > 0 && created_at + validity_time*3600 <= now` 则 `ErrExpired`（已过期）。
- `consumeRegCodeLocked` 执行：`use_count++`，记录 `used_by` / 去重写入 `used_by_uids` / `used_by_telegram_ids`；若达到次数上限则把 `active` 置为 `false`。
- 公开注册路径会在提交注册时通过 `bindStatusHub` 原子消费已确认的 Telegram 注册绑定码，并在同一个闭包中创建本地 Web 账号；随后 `CreateUserForRegistration` 把「用户名 / 邮箱 / Telegram 唯一性查重 + 建账号 + 消费注册码（仅限 type=1、非诱饵、目标匹配）+ 写入用户级 Emby 授权锁」合并在同一把 store 写锁内一次完成。绑定码不是注册码，不写入状态文档，服务重启后失效。
- 登录后使用卡码的路径使用 `ConsumeRegCodeAndUpdateUser` / `ConsumeInviteCodeAndUpdateUser`，把「卡码消费 + 邀请关系创建 + 用户权益更新」合并为一次状态写入；保存失败会整体回滚，不会留下“码已消耗但用户没拿到权益”的半状态。

## 使用入口

### 统一入口 `POST /api/v1/users/me/use-code`（鉴权：AuthUser）

`handleUseCode`（`internal/api/code_use_handlers.go`）接受 `reg_code` 或 `code` 字段，支持注册码、续期码、白名单码与邀请码，后端自动识别来源：

- `check_only=true`：仅返回预览（类型、天数、确认文案、是否需要 Emby 用户名/密码 `requires_emby_credentials` 等），**不消费**卡码、不改账号。
- 正式使用时按来源消费：邀请码走 `ConsumeInviteCodeAndUpdateUser` 并建立邀请关系；注册码走 `ConsumeRegCodeAndUpdateUser`。消费、邀请关系和用户字段更新在同一次 store 写锁内完成。
- 授予 Emby 资格的卡码（注册/白名单/邀请）在消费前会做 Emby 容量检查（`embyCapacityReachedExcluding`），超限返回 `EMBY_CAPACITY_REACHED`。
- 已绑定 Emby 的账号使用注册/白名单/邀请码会被拒绝（`CODE_ALREADY_EMBY_BOUND`），应改用续期码。
- 注册码、白名单码、邀请码、后台授予、Telegram 面板授予以及自助创建 Emby 都会在用户记录写入 `emby_grant_locked=true` 和来源字段。自助创建 Emby 按 `registration_source=regcode` 的注册资格处理，即使没有实际卡码字符串也会被视为已使用过注册类资格。后续是否允许自助解绑 Emby 只看用户自身字段，不再依赖 `RegCode.used_by_*` 或 `InviteRelations`；删除注册码、清理使用记录、断开邀请关系不会解除这个锁。

同一处理逻辑也挂在 `POST /api/v1/apikey/use-code`（鉴权：AuthAPIKey），供外部系统接入，见 [API Key 外部接入](../reference/api-key.md)。

### 公开校验 `GET /api/v1/users/regcode/check`（鉴权：AuthPublic）

`handleRegcodeCheck` 仅用于注册前的卡码预览，输入 `reg_code`，返回 `type`、`type_name`、`days`、`valid`：

- **按 IP 限流**：每分钟最多 10 次，超限返回 `RATE_LIMITED`，防止枚举。
- **不泄露使用者信息**：返回里没有 `used_by` / Telegram 等字段。
- 诱饵码（`is_decoy`）与指名码（任一 `target_*` 非空）一律按「不存在」处理，返回 `REGCODE_NOT_FOUND`，避免在公开接口暴露其存在。

### 旧续期入口 `POST /api/v1/users/me/renew`（鉴权：AuthUser）

`handleRenew`（`internal/api/handlers.go`）保留为兼容入口：必须提供 `reg_code`，且预览结果必须是 `source=regcode` 且 `type==2`（续期码），否则报错。续期经 `ConsumeRegCodeAndUpdateUser` 在同一把锁内完成卡码消费与用户续期，并用 `renewExpiryAndReactivate` 顺带解禁因到期被停用的非邀请账号。

### 公开注册 `POST /api/v1/users/register`（鉴权：AuthPublic）

当 `[SAR].register_code_limit` 开启且非空库首次注册（`bootstrapMode`，即用户数已不为 0）时，注册必须带有效的 type=1 注册码：

> 注意：`bootstrapMode`（空库首注册）仅用于豁免注册码要求，**不再**赋予管理员身份。管理员身份只来自配置文件的 `Admin.uids` / `Admin.usernames`（见 [后端架构 · 管理员引导](../reference/backend.md#首个管理员引导)）。

- 注册整体按 IP 限流（`rate_limit_register_per_10m`）；带注册码时再叠加一道 `register:regcode:<ip>` 限流，每分钟 10 次。
- 注册码校验同样排除诱饵码、`type!=1`、指名目标不匹配与不可用状态。若注册卡码指定了 TG 用户名或 TG ID，注册请求必须携带已确认的 `telegram_bind_code`，后端用绑定码中的 Telegram 身份做匹配。
- 注册提交会再次确认内存绑定码仍处于 `register` 场景、已确认且未过期，并在绑定码 hub 的消费流程内创建账号；创建成功后绑定码立即删除，创建失败则保留绑定码以便用户修正用户名、邮箱或注册码后重试。
- 启动和配置热重载会自适应修复历史残留：已绑定 Emby 但仍带 `PendingEmby` 的用户会清掉待开通状态；已记录注册码/邀请码使用但用户侧缺失 `emby_grant_locked`、来源或待开通资格的账号，会从既有使用记录恢复这些字段，不会额外消费新卡码或重建邀请关系。

## 管理接口（鉴权：AuthAdmin）

| 方法与路径 | 处理函数 | 说明 |
| ---- | ---- | ---- |
| `GET /api/v1/admin/regcodes` | `handleListRegcodes` | 分页 + 过滤（`status`/`type`/`search`）+ 排序列出注册码。 |
| `POST /api/v1/admin/regcodes` | `handleCreateRegcodes` | 批量生成。 |
| `PUT /api/v1/admin/regcodes/:code` | `handleUpdateRegcode` | 部分更新：仅改 payload 中出现的字段，支持 `note`、`active`（停用/启用）、`validity_time`（小时，-1 永久）、`days`、`use_count_limit`。校验口径同创建（`validity_time`/`use_count_limit` 的 0 归一、`days` 封顶 36500）。DTO 另返回计算字段 `expires_at`（创建时间+有效小时，-1=永久）。 |
| `DELETE /api/v1/admin/regcodes/:code` | `handleDeleteRegcode` | 删除单个卡码，包含已有使用记录的卡码也会直接从状态文档移除。 |
| `POST /api/v1/admin/regcodes/batch-delete` | `handleBatchDeleteRegcodes` | 批量删除，需确认短语 `confirm=BATCH_DELETE_REGCODES`，单次上限 200。 |
| `GET /api/v1/admin/regcodes/:code/users` | `handleRegcodeUsers` | 查看某卡码的使用者详情（按 UID / Telegram 解析）。 |
| `POST /api/v1/admin/regcodes/:code/clear-usage` | `handleClearRegcodeUsage` | 清理使用记录（`use_count`、`used_by_*` 归零并重新启用），需确认短语 `confirm=CLEAR_REGCODE_USAGE`；不影响已注册用户的账号，也不会解除用户记录上的 `emby_grant_locked`。 |

> 删除语义：`DeleteRegCode` / `DeleteRegCodes` 均为物理删除；即使卡码已有使用记录，也会直接从状态文档移除。

### 数据库一致性护栏

当运行时实际后端与配置声明的后端不一致，或 JSON 后端当前运行的 `state_file` 与配置中的 `state_file` 不一致（`runtimeDatabaseMismatch`，见 `internal/api/storage_guard.go`）时，所有注册码「写入类」操作（创建、更新、删除、批量删除、清理使用、以及通过 use-code/renew 消费注册码）都会被拦截，返回 `409` + `REGCODE_STORAGE_MISMATCH`，提示先完成数据库迁移并重启，确认前后端一致后再操作，以免写到错误的存储里。

## 兼容性

- 旧卡码不需要迁移：消费按 `code` 字符串精确查表，字符串格式不影响使用。
- 随机算法只影响新生成卡码的随机部分样式，不改变旧卡码的校验逻辑。
- 历史 `days` 为空/为 0 时按 30 天处理；负数按 `-1`（永久）处理。
- 前端在删除、更新、查看使用者时会对路径中的卡码做 URL 编码，兼容含 `:`、`.`、`_`、`-` 的新格式卡码。

## 诱饵码（蜜罐）

诱饵码（`is_decoy=true`）用于诱捕扫码 / 撞码行为：

- 不会出现在公开 `regcode/check` 结果里，也无法用于注册或被识别为有效卡码。
- 登录用户经 `previewCode` 触碰诱饵码（或用错指名码）时，`recordViolation` 会写入违规日志，并按 `[SAR].regcode_decoy_action`（`DecoyAction`，默认 `log_only`）执行处置：`log_only` 仅记录，`disable_user` 禁用账号并立即失效其会话，`disable_emby` 关闭其 Emby 账号。
- 受保护账号（如管理员）即便触碰诱饵码也只记录、不执行处罚。

## 鉴权与限流速览

| 入口 | 鉴权级别 | 限流 / 防护 |
| ---- | ---- | ---- |
| `GET /api/v1/users/regcode/check` | AuthPublic | 每 IP 10 次/分钟；不返回使用者；隐藏诱饵码与指名码 |
| `POST /api/v1/users/register` | AuthPublic | 注册整体按 IP 限流；带注册码时叠加每 IP 10 次/分钟；建账号、绑定码消费与注册码消费同锁原子 |
| `POST /api/v1/users/me/use-code` | AuthUser | `check_only` 预览不消费；消费与用户权益更新在全局锁下原子完成 |
| `POST /api/v1/apikey/use-code` | AuthAPIKey | 同 use-code 逻辑，供外部接入 |
| `POST /api/v1/users/me/renew` | AuthUser | 仅接受 type=2 续期码 |
| `GET/POST/PUT/DELETE /api/v1/admin/regcodes*` | AuthAdmin | 批量删除/清理需确认短语；写操作受数据库一致性护栏约束 |

Cookie 鉴权的变更类请求不要求 CSRF 令牌，也不做额外来源校验；Bearer Token 与 API Key 按各自鉴权路径处理。统一响应 envelope 为 `{ success, code, message, data, timestamp }`。鉴权级别与响应约定详见 [API 路由索引](../reference/api-index.md) 与 [后端 API 详参](../reference/backend-api.md)。
