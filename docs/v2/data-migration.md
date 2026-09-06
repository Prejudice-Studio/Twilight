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

## 实现位置

- 格式、加密、manifest 和 ZIP 安全解析：`internal/migration`
- PostgreSQL 数据读取和恢复：`internal/store`
- 管理员 HTTP 预检/导入/导出：`internal/api`
- SSR 管理页面：`webui-v2/src/routes/(app)/admin/migration` 或数据库页面中的迁移区域

格式核心通过 `go test ./internal/migration` 验证。新增导入功能必须补充恶意 ZIP、错误密码、manifest 篡改、路径穿越、重复文件、大小上限和失败回滚测试。
