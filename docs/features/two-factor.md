# 双重验证（2FA）

作者：Carinoasd

Twilight 支持验证器 App 的 6 位 TOTP 验证码和一次性备用码。普通用户和管理员都可在「账户设置」中启用。启用后，密码登录和 Telegram 扫码登录都会要求第二步验证。

## 部署与后台开关

先升级所有 API、Bot 和 Scheduler 进程，再开放此功能。不要让旧 API 与新 API 混合提供登录服务。升级不会自动为用户启用 2FA。

1. 在服务器生成独立的 32 字节随机密钥，例如使用 `openssl rand -base64 32`。不要提交到 Git；配置文件应仅由服务账号读取。
2. 将密钥写入 `config.toml` 或 `config.local.toml` 的 `[Security]` 段：`two_factor_key = "<生成值>"`。也可继续设置环境变量 `TWILIGHT_TWO_FACTOR_KEY`；Linux systemd 可通过 `EnvironmentFile` 引入。优先级为非空环境变量 → 本地覆盖文件 → 主配置文件；无效的非空环境变量会使 2FA 不可用，不会退回使用文件密钥。所有 API 副本及执行备份恢复、迁移的环境必须使用相同密钥。
3. 文件修改遵循现有配置热重载；环境变量修改需重启服务。该密钥不进入网页配置表单，原始 TOML 和配置备份预览只显示遮罩，网页保存不能新增、替换或删除此密钥。更换密钥会使已有验证器密文无法解密；从环境变量改为文件配置时应复制同一个值，不能重新生成。
4. 在后台安全设置打开 `Security.two_factor_enrollment_enabled`（默认 `false`）。关闭此开关只停止新设置；已经启用的账户仍然必须完成第二步验证。

例如，在服务器的 `config.local.toml` 中加入（如已有 `[Security]` 段，请在该段内添加，勿重复建段）：

```toml
[Security]
two_factor_key = "<openssl rand -base64 32 生成的值>"
```

数据库备份包含加密后的验证器数据；迁移包即使设置了密码，也不携带部署密钥。密钥必须另外安全备份。服务器上的原始配置备份可能包含主配置文件中的密钥，应按凭据文件保护。缺失或错误时，系统不会自动放行已有 2FA 账户。

## 启用与日常使用

- 输入当前 Twilight 密码，使用验证器 App 扫描设置二维码；也可手动输入密钥。设置请求有效期为 10 分钟。
- 输入验证码确认后，生成 10 枚一次性备用码。仅本次展示，可主动复制或下载；界面刷新后不能重新查看原文。
- 启用后会退出所有设备。保存备用码后重新登录。刚用于启用的验证码已被消费，需要等待验证器产生下一组验证码，或使用备用码登录。
- 第二步登录请求有效期为 3 分钟，最多 5 次错误尝试，并有账户/IP 限流。每枚备用码只能使用一次；使用它登录不会自动关闭 2FA。
- 关闭 2FA 或重新生成备用码，需要当前密码及有效验证码/备用码。重新生成后所有旧备用码失效，两种操作都会退出所有设备。
- 网络中断可能发生在凭据已经消费之后。页面不会自动重发；请重新开始登录，必要时改用另一枚备用码。

密码找回、Emby 找回密码和管理员重设密码不会清除已启用的 2FA。API Key 仍只用于原有 API 授权，不能兑换网页会话。

## 丢失验证器与备用码

先使用备用码登录，再重新配置个人验证器。如果验证器和备用码全部丢失，站点维护者应先线下核实用户身份，再在服务器项目目录运行：

```text
twilight reset-2fa --uid <UID> --confirm RESET_2FA_<UID> --config config.toml
```

将两处 `<UID>` 替换为同一真实 UID。该命令仅重置指定账户，记录控制台审计并吊销会话、扫码及第二步请求。它不需要旧加密密钥，也不会提供远程免验证重置接口。普通密码恢复不能代替这个流程。

## 备份与迁移

- 已启用的验证器密文、备用码摘要与消费状态进入一致性快照。临时设置、扫码和第二步登录请求不随备份迁移。
- 导入有 2FA 数据的备份前，目标环境必须配置匹配的密钥；密钥验证失败会拒绝写入。
- 恢复会覆盖账户的 2FA 状态并吊销所有会话。没有 2FA 数据的旧备份会清除目标账户的 2FA，恢复前必须检查预览及保护性备份。
- 恢复历史快照也会恢复那个时点的备用码状态；恢复后应重新生成备用码，避免后来使用过的旧码重新有效。

## API

全部接口禁止缓存。正式会话仅在完成全部登录步骤后签发；第一步成功的响应可能改为 `data.two_factor_required=true`、`data.request` 和 `data.expires_at`，此时没有 `token` 或 `user`。V1 密码登录同样遵守此契约。

| 方法 | 路径 | 权限与请求 |
| --- | --- | --- |
| GET | `/api/v2/settings/two-factor` | 登录用户；只返回自身状态、剩余码数量、设置开放状态和密钥就绪布尔值 |
| POST | `/api/v2/settings/two-factor/setup` | 登录用户；`password`；返回设置请求、密钥、`otpauth` URI 与期限 |
| POST | `/api/v2/settings/two-factor/enable` | 原登录会话；`request`、`code`；一次性返回 `recovery_codes` |
| DELETE | `/api/v2/settings/two-factor` | 登录用户；`password`、`code`、`recovery` |
| POST | `/api/v2/settings/two-factor/recovery-codes` | 登录用户；`password`、`code`、`recovery`；一次性返回新备用码 |
| POST | `/api/v2/auth/two-factor` | 第一阶段凭据；`request`、`code`、`recovery`；沿用第一阶段的 `X-Twilight-Device` |
| DELETE | `/api/v2/auth/two-factor` | `request`；撤销持有的设置或登录请求，不泄露其归属 |

`recovery=true` 表示使用备用码，否则使用验证器验证码。凭据只通过 JSON 正文传递，不进入 URL。错误码：`AUTH_TWO_FACTOR_INVALID`（请求失效）、`AUTH_TWO_FACTOR_CODE_INVALID`（验证码错误/重放）、`AUTH_TWO_FACTOR_UNAVAILABLE`（功能或密钥不可用）。

## 安全实现

TOTP 遵循 [RFC 6238](https://www.rfc-editor.org/info/rfc6238/)，使用 30 秒时间步，接受前后一个时间步的时钟偏差，事务记录已消费时间步以阻止重放。验证器密钥以 AES-256-GCM 加密并绑定 UID；备用码具有 128 位随机熵，只保存摘要。

凭据消费、状态变更和会话签发共享数据库锁顺序。数据库是会话有效性的最终依据，其他 API 进程持有的 Redis/内存缓存不能恢复已撤销的会话。认证因素修改时重新验证身份、提供一次性恢复码的流程参考 [OWASP 多因素认证指南](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html)。
