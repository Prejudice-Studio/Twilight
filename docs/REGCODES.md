# 注册码与卡码说明

本文说明 Twilight 的注册码、续期码、白名单码、诱饵码以及新旧卡码兼容口径。

## 类型

| type | 名称 | 使用入口 | 行为 |
| ---- | ---- | -------- | ---- |
| `1` | 注册码 | `POST /users/register`、`POST /users/me/use-code` | 新用户注册系统账号并获得补建 Emby 资格；已登录且未绑定 Emby 的用户可用它创建 Emby 账号。 |
| `2` | 续期码 | `POST /users/me/renew`、`POST /users/me/use-code` | 已绑定 Emby 的用户续期；天数 `0` 或 `-1` 表示永久。 |
| `3` | 白名单码 | `POST /users/me/use-code` | 授予白名单角色和永久有效期；未绑定 Emby 时会要求同时创建 Emby 账号。 |

## 字段

| 字段 | 说明 |
| ---- | ---- |
| `CODE` | 卡码字符串，主键。新格式可由 `format` 和 `random_algorithm` 生成；旧卡码字符串继续原样可用。 |
| `TYPE` | 卡码类型：`1` 注册、`2` 续期、`3` 白名单。 |
| `DAYS` | 授予或增加的账号天数；`0`、负数会规范化为 `-1`，表示永久。 |
| `VALIDITY_TIME` | 卡码自身有效期，单位小时；`-1` 表示永久。过期卡码不能通过公开校验、续期或统一使用入口。 |
| `USE_COUNT_LIMIT` | 使用次数上限；`-1` 表示不限次数，大于 `0` 表示固定次数。 |
| `USE_COUNT` | 已使用次数。 |
| `ACTIVE` | 是否启用。停用后不可使用。 |
| `UID` / `TELEGRAM_ID` | 使用者记录，管理员页面可查看使用者详情。 |
| `OTHER` | JSON 元数据，当前用于 `decoy` 诱饵标记和 `note` 备注。 |

## 生成格式

管理员创建接口 `POST /admin/regcodes` 支持：

```json
{
  "type": 1,
  "validity_time": -1,
  "use_count_limit": 1,
  "days": 30,
  "count": 1,
  "format": "TW-{type}-{random}",
  "random_algorithm": "base32-20",
  "decoy": false
}
```

可用占位符：`{random}`、`{type}`、`{days}`、`{index}`、`{validity}`、`{limit}`。

可用随机算法：

| 算法 | 说明 |
| ---- | ---- |
| `base32-20` | 默认推荐。20 位大写易抄写字符，移除易混淆字符，适合人工复制和公开发放。 |
| `base32-24` | 更高强度的易抄写字符，适合高价值或公开渠道发放。 |
| `hex32` | 32 位十六进制，约 128-bit 强度，适合系统间导入导出。 |
| `hex20` | 旧默认格式，继续支持。 |
| `base32-16` | 较短易抄写格式。 |
| `alnum-24` | 24 位大写字母数字，高强度。 |
| `alnum-16` | 16 位大写字母数字。 |
| `urlsafe-24` | 24 位 URL 安全字符，包含大小写字母、数字、`-`、`_`。 |
| `digits-16` | 16 位纯数字，比 `digits-12` 更安全。 |
| `digits-12` | 12 位纯数字，仅建议口头传递或低风险场景。 |
| `uuid` | 标准 UUID v4。 |
| `legacy-sha1` | 旧版 SHA1 截断风格，仅用于兼容历史样式。 |

## 兼容性

- 旧卡码不需要迁移：数据库以 `CODE` 为准查询，字符串格式不会影响使用。
- 随机算法只影响新生成卡码的随机部分样式，不会改变旧卡码校验逻辑。
- 历史 `DAYS` 为空时业务层按默认 30 天处理；`0` 和负数统一视为永久。
- 旧入口 `POST /users/me/renew` 保留，但会执行与统一入口一致的启用、次数和过期检查。
- 前端删除、更新和查看使用者均会对路径中的卡码做 URL 编码，兼容包含 `:`、`.`、`_`、`-` 的新格式卡码。

## 安全与鉴权

- `POST /users/regcode/check` 是公开接口，仅返回类型、天数和有效性，不返回使用者信息；按 IP 限流防枚举。
- `POST /users/register` 使用注册码时额外限流，并通过注册锁避免同一码并发注册。
- `POST /users/me/use-code`、`POST /users/me/renew` 需要登录，并检查启用状态、次数上限和卡码自身有效期。
- `GET/POST/PUT/DELETE /admin/regcodes` 均需要管理员鉴权。
- 诱饵码不会在公开检查中暴露；已登录用户使用后会按 `[SAR].regcode_decoy_action` 执行安全动作。
