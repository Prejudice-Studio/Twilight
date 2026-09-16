# Telegram 身份历史记录

## 概述

V2 引入了 Telegram 身份历史记录功能，用于追踪用户 Telegram Username 和 UserID 的变更历史。这对于审计、账号安全监控和用户支持场景非常有用。

## 数据库设计

### 表结构

```sql
CREATE TABLE IF NOT EXISTS twilight_telegram_identity_history (
    id bigserial PRIMARY KEY,
    uid bigint NOT NULL,
    telegram_id bigint NOT NULL,
    telegram_username text NOT NULL DEFAULT '',
    change_type text NOT NULL DEFAULT 'update',
    recorded_at timestamptz NOT NULL DEFAULT now(),
    recorded_unix bigint NOT NULL
);
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | bigserial | 自增主键 |
| `uid` | bigint | 用户 UID |
| `telegram_id` | bigint | Telegram UserID |
| `telegram_username` | text | Telegram Username（不含 @ 前缀） |
| `change_type` | text | 变更类型：`bind`（绑定）、`update`（更新）、`unbind`（解绑） |
| `recorded_at` | timestamptz | 记录时间（带时区） |
| `recorded_unix` | bigint | Unix 时间戳（秒） |

### 索引

```sql
CREATE INDEX IF NOT EXISTS idx_telegram_identity_history_uid 
    ON twilight_telegram_identity_history(uid);

CREATE INDEX IF NOT EXISTS idx_telegram_identity_history_telegram_id 
    ON twilight_telegram_identity_history(telegram_id);

CREATE INDEX IF NOT EXISTS idx_telegram_identity_history_recorded_at 
    ON twilight_telegram_identity_history(recorded_at DESC);
```

## 自动记录触发点

系统在以下场景自动记录 Telegram 身份变更：

### 1. 绑定（bind）

- 用户首次绑定 Telegram 账号
- 管理员强制绑定
- 注册时通过绑定码绑定

### 2. 更新（update）

- 用户 Telegram Username 发生变化
- 用户 Telegram UserID 发生变化（极少见，但理论上可能）
- 系统通过 Telegram API 检测到身份变化

### 3. 解绑（unbind）

- 用户主动解绑 Telegram
- 管理员强制解绑
- 换绑操作（先解绑旧账号，再绑定新账号）

## API 接口

### 查询用户身份历史

**端点**: `GET /api/v2/admin/users/{uid}/telegram/identity-history`

**鉴权**: Admin

**响应示例**:

```json
{
  "success": true,
  "data": {
    "history": [
      {
        "id": 123,
        "uid": 1001,
        "telegram_id": 123456789,
        "telegram_username": "alice",
        "change_type": "bind",
        "recorded_at": "2026-09-01T10:00:00Z",
        "recorded_unix": 1725184800
      },
      {
        "id": 124,
        "uid": 1001,
        "telegram_id": 123456789,
        "telegram_username": "alice_new",
        "change_type": "update",
        "recorded_at": "2026-09-10T15:30:00Z",
        "recorded_unix": 1725979800
      }
    ],
    "total": 2
  }
}
```

### 按 Telegram ID 查询

**端点**: `GET /api/v2/admin/telegram/identity-history/{telegram_id}`

**鉴权**: Admin

**用途**: 查找某个 Telegram ID 关联过的所有用户

## 存储层接口

### RecordTelegramIdentity

记录一次身份变更：

```go
func (s *Store) RecordTelegramIdentity(
    uid int64, 
    telegramID int64, 
    telegramUsername string, 
    changeType string,
) error
```

### GetTelegramIdentityHistory

获取用户身份历史：

```go
func (s *Store) GetTelegramIdentityHistory(uid int64) ([]TelegramIdentityHistory, error)
```

### GetTelegramIdentityHistoryByTelegramID

按 Telegram ID 查询：

```go
func (s *Store) GetTelegramIdentityHistoryByTelegramID(
    telegramID int64,
) ([]TelegramIdentityHistory, error)
```

### CleanupOldTelegramIdentityHistory

清理旧记录（建议通过定时任务调用）：

```go
func (s *Store) CleanupOldTelegramIdentityHistory(days int) error
```

### GetLatestTelegramIdentity

获取最新身份记录：

```go
func (s *Store) GetLatestTelegramIdentity(uid int64) (*TelegramIdentityHistory, error)
```

## 使用场景

### 1. 账号安全审计

管理员可以查看用户 Telegram 身份的变更历史，识别异常行为：

- 短时间内频繁换绑
- Username 频繁变更
- 同一 Telegram ID 关联多个账号

### 2. 用户支持

当用户声称账号被盗或换绑失败时，管理员可以：

- 查看完整的绑定/解绑历史
- 确认历史 Username 和 UserID
- 验证用户身份

### 3. 数据分析

- 统计用户换绑频率
- 分析 Telegram 账号活跃度
- 识别机器人账号模式

## 数据保留策略

### 默认保留

- 所有记录默认永久保留
- 建议定期清理 1 年以上的旧记录

### 手动清理

管理员可以通过 API 或数据库直接清理：

```sql
DELETE FROM twilight_telegram_identity_history
WHERE recorded_at < NOW() - INTERVAL '365 days';
```

### 自动清理（推荐）

通过定时任务调用存储层接口：

```go
// 清理 365 天前的记录
err := store.CleanupOldTelegramIdentityHistory(365)
```

## 隐私考虑

### 数据敏感性

- Telegram Username 和 UserID 属于个人信息
- 历史记录可能暴露用户身份变更模式
- 仅管理员可访问，不对普通用户开放

### GDPR 合规

如果服务面向欧盟用户，需要考虑：

- 用户有权请求删除历史记录
- 数据保留期限应在隐私政策中明确
- 提供数据导出功能

## 向后兼容性

### 数据库迁移

表使用 `CREATE TABLE IF NOT EXISTS`，支持无缝升级：

- V1 实例升级到 V2 时自动创建表
- 不影响现有数据
- 历史记录从升级时刻开始累积

### V1 实例升级

V1 实例升级到 V2 后：

1. 表自动创建
2. 现有绑定关系不会生成历史记录
3. 后续变更开始记录
4. 可选：管理员手动为现有绑定补充初始记录

## 性能考虑

### 写入性能

- 每次绑定/解绑/更新仅写入一条记录
- 使用批量插入优化高并发场景
- 索引设计支持快速查询

### 查询性能

- `uid` 和 `telegram_id` 索引支持快速查找
- `recorded_at` 降序索引支持时间范围查询
- 分页查询避免大结果集

### 存储空间

- 单条记录约 100 字节
- 100 万用户，每年平均 2 次变更 = 200 万条记录 = 约 200 MB
- 建议定期归档或清理旧数据

## 相关文档

- [Telegram Bot 命令](../features/telegram-bot.md)
- [V2 数据库设计](./database-split-design.md)
- [V2 安全审计报告](./security-audit-report.md)
