# 签到与积分续期

Twilight 的签到模块记录每日积分、连续签到和奖励。积分可以在管理员许可后用于手动续期，也可以由用户选择在账号到期时自动续期。自动续期不是提前续费任务，只在调度器的 `check_expired` 发现账号已经到期时执行。

## 配置

签到相关配置位于 `[SAR]`：

| 配置项 | 默认值 | 说明 |
| ---- | ---- | ---- |
| `signin_enabled` | `true` | 签到总开关 |
| `signin_renewal_enabled` | `false` | 允许使用签到积分手动续期 |
| `signin_auto_renewal_enabled` | `false` | 允许用户自行开启到期自动续期 |
| `signin_renewal_cost` | `100` | 每次续期消耗积分 |
| `signin_renewal_days` | `30` | 每次续期增加天数 |

自动续期的全局有效条件为：签到总开关、积分续期开关、自动续期许可全部开启，且消耗和续期天数均大于 `0`。环境变量 `TWILIGHT_SIGNIN_AUTO_RENEWAL_ENABLED` 可覆盖管理员自动续期许可。

## 用户开关

管理员开启全局许可后，符合条件的用户可在签到页的积分续期区域开启「到期自动续期」。前端通过 `PUT /api/v1/users/me` 写入严格布尔字段：

```json
{
  "signin_auto_renewal": true
}
```

开启时后端要求用户是非保护普通账号、已有真实 Emby 绑定、没有待开通状态，并具有非永久的固定有效期。管理员关闭全局许可后，已保存的个人意愿不会触发续期；用户仍可随时关闭个人开关。

`GET /api/v1/signin/me` 的 `renewal` 对象提供：

- `auto_renewal_enabled`：管理员自动续期许可当前是否整体有效。
- `auto_renewal_user_enabled`：用户保存的个人偏好。
- `auto_renewal_available`：当前用户是否具备开启资格。

## 到期处理

`check_expired` 每轮先从 PostgreSQL 刷新一次主状态，确保 API 与 scheduler 分进程运行时也能读取最新个人偏好；随后对每个启用中且已经到期的用户最多尝试一次自动续期。执行前同时检查：

1. 签到、积分续期和管理员自动续期许可全部有效。
2. 用户已开启个人自动续期。
3. 用户是普通账号且未被管理员手动禁用。
4. 用户已有 Emby 绑定，不是 `PendingEmby` 待开通状态。
5. 账号不是永久账号，且到期时间确实已到。
6. 当前积分余额不少于 `signin_renewal_cost`。

余额检查、扣分、资格复核、延长有效期与恢复 Web 启用状态在一个 Store 原子写入中完成。成功后该用户跳过本轮禁用；若本地镜像显示 Emby 已禁用，调度器还会尽力恢复远端 Emby。余额不足、无 Emby 或状态冲突都不会扣分，并继续普通到期禁用与会话清理。

调度结果会报告 `auto_renewed`、`auto_renewal_insufficient`、`auto_renewal_ineligible`、`auto_renewal_failed` 和 `auto_renewal_points_spent` 等汇总字段；成功自动续期写入 `auto_renew_expired_users` 系统审计。
