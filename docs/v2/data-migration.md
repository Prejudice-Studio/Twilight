# Twilight 数据迁移包

本文记录 Twilight V2 使用的专用 ZIP 格式，以及当前管理员导出/导入边界。格式安全解析、PostgreSQL 一致性数据读取、受控静态资源收集和管理员 HTTP 预检/执行接口已经接入；V2 管理页面随后接入。配置策略和资源原子落盘规则见下文。

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

## 静态资源收集

`internal/api/migration_resources.go` 提供受控的 `UploadDir` 资源收集器。它只读取以下目录：

- `avatar/` → `resources/avatars/`
- `background/` → `resources/backgrounds/`
- `tickets/<ticket_id>/` → `resources/tickets/<ticket_id>/`
- `server-icon/` → `resources/server-icon/`
- `auth-background/` → `resources/auth-background/`
- `bangumi/` → `resources/bangumi/`

收集器不会把真实机器路径写入清单。根目录和每级条目都拒绝符号链接、非普通文件、路径越界和异常文件名；不存在的可选目录视为空目录，UploadDir 下其它目录不会被打包。单文件、总大小和文件数沿用迁移格式的有界预算，读取前后会复核文件状态，发现并发变化时终止快照。

`twilight_sessions` 不属于迁移数据：其中包含短期会话凭据，导出会扩大凭据泄露和跨实例会话混淆风险。导入后用户需要重新登录。静态资源由管理员导出编排显式追加，不能把整个 UploadDir 直接加入归档。

## 配置导出策略

`config/effective.toml` 是按当前生效配置和已知 schema 生成的迁移配置，不是原始配置文件的逐字复制；真实机器路径、环境变量和受保护的管理员身份段不会被迁移配置覆盖。`config/policy.json` 记录配置格式与是否包含密钥：

- 无密码导出：所有 schema 标记为 secret 的字段写成 `__TWILIGHT_SECRET_UNCHANGED__` 哨兵。导入时保留目标实例现有密钥。
- 密码导出：在归档已由 Argon2id + AES-256-GCM 保护时，才允许写入当前生效密钥。导入仍只在管理员明确勾选“应用配置”后执行。
- 导入不会覆盖目标实例的 `Admin` 身份、数据库目录、系统更新源等本地边界配置；未选择应用配置时只导入业务数据和资源。

密码只存在于请求 body/multipart 字段的处理生命周期，不进入 URL、日志、清单或响应 JSON。

## 管理员接口

迁移面板默认关闭，需开启 `Database.migration_panel_enabled`。接口全部要求管理员会话：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/api/v1/system/admin/migration/status` | 返回格式、容量和允许的资源命名空间 |
| POST | `/api/v1/system/admin/migration/export` | 生成 ZIP；JSON body 可选 `password`，空值表示无密码 |
| POST | `/api/v1/system/admin/migration/import` | multipart 上传 `archive`，首次请求只生成预览；确认字段为 `IMPORT_TWILIGHT_DATA` |

导入字段包括 `password`、`preview`、`apply_config` 和 `resource_mode`。`resource_mode=preserve`（默认）遇到不同内容的资源时只报告冲突；`resource_mode=replace` 需要管理员确认后覆盖。数据库导入失败会回滚数据库和已写入的资源，配置应用失败也会恢复原配置。

`internal/store.Store.ImportMigrationArchive` 已提供数据恢复基础。它要求归档先经过 `migration.Open`，随后在一个可回滚的 Serializable 事务中替换主状态、运行日志、审计日志、Telegram 花名册、Telegram 更新游标和完整播放记录，并重建自增序列与播放记录的有限内存兼容窗口。数据库失败时事务不会留下半套状态；方法成功提交后才更新 Store 内存快照。当前恢复核心不触碰 `twilight_sessions`，也不会把 `config/*` 或 `resources/*` 直接写入文件系统。

恢复前仍必须由上层完成格式、密码、版本、资源、配置和冲突预检，并在最终确认后调用。不要把 `ImportMigrationArchive` 暴露为无需管理员权限的通用 Store 操作。

## 实现位置

- 格式、加密、manifest 和 ZIP 安全解析：`internal/migration`
- PostgreSQL 一致性数据读取：`internal/store/migration_export.go`
- PostgreSQL 数据恢复：`internal/store/migration_import.go`
- 管理员 HTTP 预检/导入/导出：`internal/api/migration_handlers.go`
- UploadDir 资源收集：`internal/api/migration_resources.go`
- SSR 管理页面：`webui-v2/src/routes/(app)/admin/migration` 或数据库页面中的迁移区域

格式核心通过 `go test ./internal/migration` 验证；资源收集、路径冲突和失败回滚通过 `go test ./internal/api` 覆盖。新增导入功能必须继续补充恶意 ZIP、错误密码、manifest 篡改、路径穿越、重复文件、大小上限和失败回滚测试。
