# Twilight 数据迁移包

本文记录 Twilight V2 使用的专用 ZIP 格式。当前提交只建立格式与安全解析核心；数据库读取、静态资源收集、导入事务和管理页面会在后续独立模块中接入，不能把当前格式包 API 误认为已经完成整站备份。

## 外层结构

无密码包包含：

```text
manifest.json
manifest.sha256
payload.zip
```

密码包包含：

```text
manifest.json
manifest.sha256
payload.enc
```

`manifest.json` 保持可读取，用于在导入前展示格式版本、Twilight 版本、数据库结构版本、导出时间和文件清单；业务数据位于载荷中。密码模式只让清单元数据保持可见，不在清单中写入密码或密钥。

载荷中的文件路径必须位于以下命名空间之一：

- `data/`：数据库导出数据与专用表数据
- `config/`：明确允许迁移的配置数据
- `resources/`：头像、背景、封面等经过白名单确认的静态资源

禁止绝对路径、反斜杠、`.`、`..`、符号链接、目录条目和重复路径。导入器必须先写入受控临时目录，冲突检查完成后才能进入正式资源目录。

## Manifest

格式版本固定为 `twilight-export/v1`。每个文件记录：

- 相对路径
- 字节大小
- SHA-256
- 业务类型：`data`、`config` 或 `resource`
- 可选 MIME 类型

导出方和导入方都限制文件数量、单文件大小、总解压大小和 manifest 大小。当前核心默认限制为 4096 个文件、单文件 128 MiB、总解压 512 MiB、manifest 1 MiB。接口层还必须对上传请求本身设置更小或等效的 `MaxBytesReader` 上限。

## 密码保护

密码模式使用：

- Argon2id 派生 32 字节 AES-256 密钥
- 每个包随机 16 字节 Salt
- 每个包随机 12 字节 GCM Nonce
- AES-256-GCM 完整性认证
- 最终 manifest 作为 GCM AAD，防止清单被单独替换

不使用 MD5、直接 SHA-256 截断、自制 XOR 或 Base64 伪加密。密码不进入日志、URL、响应 JSON 或 manifest。当前实现要求非空密码至少 8 个 UTF-8 字节，并限制最大长度，防止异常输入消耗过多资源。

## 导入顺序

完整导入必须按以下顺序执行：

1. 限制上传大小并读取安全的 ZIP 结构
2. 校验格式版本和 manifest
3. 校验 manifest SHA-256
4. 验证密码并解密载荷
5. 校验路径、文件类型、数量、大小和每个文件 SHA-256
6. 检查 Twilight/数据库结构兼容性
7. 检查用户、ID、配置和资源冲突
8. 请求管理员确认
9. 在数据库事务和临时资源目录中执行导入
10. 复核数据库与资源完整性，失败时回滚并清理孤儿临时文件

任何导入处理器都不得因为前端隐藏按钮而跳过管理员鉴权、确认或后端冲突检查。

## 当前导出读取

`internal/store.Store.ExportMigrationFiles` 已提供 PostgreSQL 一致性读取基础。它在一个 `REPEATABLE READ` 只读事务中读取：

- `twilight_state` 主状态文档
- `twilight_runtime_logs` 运行日志
- `twilight_audit_logs` 操作审计
- `twilight_telegram_roster` Telegram 花名册
- `twilight_telegram_runtime` 更新游标
- `twilight_playback_records` 全量播放记录（包含数据库行 ID 和创建时间）

结果被拆为 `data/state.json`、`data/runtime-logs.json`、`data/audit-logs.json`、`data/telegram-roster.json`、`data/telegram-runtime.json` 和 `data/playback-records.json`，再交由 `internal/migration.Create` 生成 ZIP。主状态中的兼容性日志字段会在导出前移除，避免同一行被导入两次；播放记录兼容切片保留给历史读取者，完整独立表另行导出。

`twilight_sessions` 不属于迁移数据：其中包含短期会话凭据，导出会扩大凭据泄露和跨实例会话混淆风险。导入后用户需要重新登录。配置文件和 `UploadDir` 下的静态资源尚未由该读取方法自动加入，接入时必须先经过资源白名单和安全路径检查。

`internal/store.Store.ImportMigrationArchive` 已提供数据恢复基础。它要求归档先经过 `migration.Open`，随后在一个可回滚的 Serializable 事务中替换主状态、运行日志、审计日志、Telegram 花名册、Telegram 更新游标和完整播放记录，并重建自增序列与播放记录的有限内存兼容窗口。数据库失败时事务不会留下半套状态；方法成功提交后才更新 Store 内存快照。当前恢复核心不触碰 `twilight_sessions`，也不会把 `config/*` 或 `resources/*` 直接写入文件系统。

恢复前仍必须由上层完成格式、密码、版本、资源、配置和冲突预检，并在最终确认后调用。不要把 `ImportMigrationArchive` 暴露为无需管理员权限的通用 Store 操作。

## 实现位置

- 格式、加密、manifest 和 ZIP 安全解析：`internal/migration`
- PostgreSQL 一致性数据读取：`internal/store/migration_export.go`
- PostgreSQL 数据恢复：`internal/store/migration_import.go`
- 管理员 HTTP 预检/导入/导出：`internal/api`
- SSR 管理页面：`webui-v2/src/routes/(app)/admin/migration` 或数据库页面中的迁移区域

格式核心通过 `go test ./internal/migration` 验证。新增导入功能必须补充恶意 ZIP、错误密码、manifest 篡改、路径穿越、重复文件、大小上限和失败回滚测试。
