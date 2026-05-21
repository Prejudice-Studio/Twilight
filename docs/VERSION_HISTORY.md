# 版本更新与功能规划

本文档记录 Twilight 的版本号、更新内容、功能边界和后续版本推进流程。当前版本为 `0.0.3`。

## 版本策略

Twilight 使用语义化版本号：`MAJOR.MINOR.PATCH`。

| 位置 | 含义 |
| ---- | ---- |
| `MAJOR` | 破坏性变更、数据模型不兼容、部署方式重大调整。 |
| `MINOR` | 新功能、新模块、兼容性增强。 |
| `PATCH` | Bug 修复、安全修复、文档补充、无破坏性小改动。 |

当前项目仍处于早期整理阶段，`0.x` 版本默认表示 API、配置项和数据库结构仍可能调整。进入 `1.0.0` 前必须完成核心功能冻结、迁移策略、测试覆盖和发布流程固化。

## 当前版本

### `0.0.3` - Bangumi 点格子与 Emby 媒体库自助显隐

发布日期：2026-05-21

Git 记录：`0.0.2` 提交 `284e612` 之后至当前 `588dd6f`（`Add Bangumi sync and Emby library self-service`）。

定位：在 `0.0.2` 安全与容量保护基础上，新增 Bangumi 自动点格子同步、用户个人 BGM 配置、管理员可见的同步状态，以及 Emby 媒体库默认隐藏与用户自助显隐能力。

更新内容：

- 新增 Bangumi 点格子同步文档 `docs/BANGUMI_SYNC.md`，并在 README、文档导航、示例配置和 `.env.example` 中补充入口与配置说明。
- 新增 `[BangumiSync]` 配置节，支持功能开关、Webhook 密钥、自动收藏、私有收藏、屏蔽关键词和最小播放进度阈值。
- Emby API 新增 `/api/v1/emby/bangumi/webhook`，接收 Emby 播放完成或手动标记已播放事件，并按本地用户映射触发 Bangumi 同步。
- Bangumi 客户端补充当前用户查询、条目收藏、章节收藏、按集数标记等 API 封装，并增强搜索和章节匹配逻辑。
- Bangumi 同步服务新增事件过滤、播放进度判断、屏蔽关键词、条目自动加入收藏、剧集/剧场版构造同步请求和详细错误返回。
- 用户个人设置新增 Bangumi 点格子面板，可保存个人 Bangumi Token、启用/关闭同步、清除 Token；Token 不回显明文。
- 安全调整：点格子只使用用户个人 Bangumi Token；用户未配置个人 Token 时直接跳过，不再调用全局 `Global.bangumi_token` 兜底。
- 管理员用户列表可查看所有用户的 BGM 同步开关、Token 是否已配置和是否可同步，但不展示 Token 明文。
- Emby 配置新增默认隐藏媒体库与可自助显隐媒体库列表；新建、补建、绑定和邀请创建流程会应用默认隐藏策略。
- 用户设置新增媒体库自助显隐入口；仅允许操作管理员开放的媒体库，并通过后端接口校验权限。
- 管理员用户页新增单个用户媒体库权限详情、显示/隐藏媒体库、开启/关闭自助显隐，以及批量开启自助显隐能力。
- 系统配置 schema、用户设置接口、管理员接口和前端 API 类型同步补充 Bangumi 同步与媒体库自助显隐字段。

已知边界：

- Bangumi Webhook 生产环境必须配置高熵 `webhook_secret`；留空时会兼容无密钥调用，不建议公网暴露。
- Bangumi 条目匹配依赖标题、季度、集数和首播日期；标题命名不规范时仍可能需要后续补充自定义映射能力。
- 媒体库自助显隐只对管理员配置的 `emby_self_service_libraries` 生效，不允许用户任意修改完整 Emby 策略。
- 版本号文件仍停留在 `0.0.2`，本段记录的是当前进度相对 `0.0.2` 的目标版本内容；正式发布前需同步更新版本号文件。

### `0.0.2` - 安全加固、Emby 到期与管理体验完善

发布日期：2026-05-21

Git 记录：`284e612`（`Bump v0.0.2; security & Emby capacity fixes`）。

定位：在 `0.0.1` 基线后补充演示接口、安全防护、Emby 到期处理、邀请/注册容量保护和 Telegram 管理体验。

更新内容：

- 新增 `/api/v1/demo` TestWeb 演示接口，前端演示页改为调用静态模拟 API，不触碰数据库或外部服务。
- Next.js 增加默认安全响应头，并同步补充生产安全文档。
- 为 Cookie 会话写操作增加 `X-Twilight-Client: webui` 校验，降低 CSRF 风险，并补充敏感接口限流与安全说明。
- 完善 Emby 到期处理：到期后只禁用 Emby 账号，不禁用系统账号；续期后恢复 Emby；到期用户不下发线路，前端展示续期提示。
- 管理员用户页新增取消永久有效期入口，可将永久账号改为指定天数后到期。
- 邀请、注册码补建、管理员授予补建资格、Bot 创建用户等路径统一加 Emby 容量锁与上限校验，降低并发超卖风险。
- 注册码使用队列、Emby 补建队列和管理员授予待补建资格流程补充 acquire/release 保护，异常路径会释放容量占位。
- 邀请树新增防环校验，管理员级联启停/删除增加管理员账号保护。
- 邀请树普通用户生成的邀请码/专属续期码新增有效期上限：不能超过自身剩余有效期，永久账号最多授权 365 天。
- 邀请中心新增下级只读状态展示，并支持上级为已到期直属下级生成专属续期码；专属续期码被非目标用户使用会按安全策略禁用误用账号。
- Telegram 换绑申请支持用户备注；管理员可批量选择换绑请求并批量批准或拒绝。
- Telegram 换绑列表增加分页参数校验、批量审核接口和限流保护，降低误操作与刷接口风险。
- Telegram 群组巡检新增“回群后自动启用”配置；退群永久封禁继续由 `ban_on_leave` 控制。
- 管理员接口阻止修改、删除或启用其它管理员账号，降低后台误伤和越权风险。
- 修正 Emby 同步逻辑，避免把“因到期而禁用的 Emby 账号”反向同步为系统账号禁用。
- 统一部分 API 响应码与参数校验，补充调度任务展示字段和配置 schema。
- 移除旧 Windows 开发/启动脚本，并在 `.gitignore` 中忽略本地 shell、batch、PowerShell 脚本。
- 同步更新后端、前端版本号到 `0.0.2`。

已知边界：

- Emby 容量锁使用 Redis 锁；未配置 Redis 时退回进程内锁，多 worker 部署仍建议配置 Redis 以获得跨进程保护。
- 独立 Emby 账号创建会按 Emby 端用户总数检查 `EMBY_USER_LIMIT`，绑定/待补建路径按本站绑定与待补建资格计数。

### `0.0.1` - 基线整理版本

发布日期：2026-05-21

定位：建立统一版本号、整理现有功能地图、明确开发分支和发布流程。

更新内容：

- 将后端包版本、后端运行时版本、前端包版本统一设置为 `0.0.1`。
- 新增版本更新与功能规划文档。
- 新增开发者开发流程文档。
- 明确后续开发必须在 `dev` 分支进行，完成后由仓库维护者手动 push 并合并到 `main`。
- 整理当前项目已存在的后端 API、业务服务、数据库模块、Telegram Bot、前端页面和文档体系。

## 功能地图

### 后端 API

| 模块 | 路径前缀 | 当前能力 |
| ---- | -------- | -------- |
| Auth | `/api/v1/auth` | 登录、登出、会话刷新、当前用户、API Key 管理、Telegram/API Key 登录入口。 |
| Users | `/api/v1/users` | 注册、个人资料、密码、Emby 绑定/补建、注册码使用、续期、Telegram 绑定、Bangumi 点格子配置、媒体库自助显隐、头像和背景资源。 |
| Admin | `/api/v1/admin` | 用户管理、注册队列处理、注册码管理、Emby 管理、媒体库权限、求片审核、邀请树、公告、调度任务、Telegram 管理工具。 |
| System | `/api/v1/system` | 系统信息、健康检查、Emby 线路、配置 schema、配置保存、配置清理、管理统计、在线更新。 |
| Emby | `/api/v1/emby` | Emby 状态、媒体库、搜索、最新媒体、会话数、Bangumi 点格子 Webhook；旧线路接口返回弃用提示。 |
| Media | `/api/v1/media` | TMDB/Bangumi 搜索、媒体详情、库存检查、求片提交和用户求片管理。 |
| API Key | `/api/v1/apikey` | 外部系统专用认证、状态、启停、续期、权限、Emby 操作和卡码使用。 |
| Batch | `/api/v1/batch` | 批量用户操作、导出、播放统计批量查询和提醒。 |
| Stats | `/api/v1/stats` | 当前用户和管理员视角播放统计。 |
| Security | `/api/v1/security` | 登录历史、设备管理、IP 黑名单、可疑行为。 |
| Invite | `/api/v1/invite` | 邀请配置、邀请码生成/删除/使用、邀请状态。 |
| Signin | `/api/v1/signin` | 签到配置、签到、签到历史、积分摘要。 |
| Announcements | `/api/v1/announcements` | 公开公告列表。 |
| Demo | `/api/v1/demo` | TestWeb 演示专用模拟接口，只返回静态预设假数据；模拟操作忽略请求体，不触碰数据库或外部服务。 |
| OpenAPI | `/api/v1/openapi.json`、`/api/v1/docs` | 运行时 API 描述和 Swagger UI。 |

### 业务服务

| 服务 | 文件 | 当前能力 |
| ---- | ---- | -------- |
| 用户服务 | `src/services/user_service.py` | 注册、待补建 Emby、续期、白名单、密码、角色、Emby 同步、媒体库自助显隐。 |
| Emby 客户端 | `src/services/emby.py`、`emby_service.py` | Emby 认证、用户、媒体库、会话、活动、策略同步、默认隐藏媒体库。 |
| 注册队列 | `emby_register_queue.py`、`regcode_use_queue.py` | Emby 补建和卡码使用的排队、状态查询、限流和 token 校验。 |
| 邀请服务 | `invite_service.py` | 邀请关系、邀请码、邀请树、被邀请用户处理。 |
| 求片服务 | `media_service.py` | 搜索聚合、库存检查、求片状态流转。 |
| Bangumi | `bangumi.py`、`bangumi_search.py`、`bangumi_sync.py` | Bangumi 搜索、条目同步、Emby Webhook 观看记录点格子同步。 |
| TMDB | `tmdb.py` | TMDB 搜索和详情。 |
| Telegram | `telegram_runtime.py`、`telegram_membership.py` | Bot 运行时、群成员校验、退群处理。 |
| 安全服务 | `security_service.py` | 登录审计、设备、IP 黑名单。 |
| 签到服务 | `signin_service.py` | 签到、积分、连续签到奖励。 |
| 定时任务 | `scheduler_service.py` | APScheduler、任务计划、运行历史、手动触发。 |
| 系统更新 | `system_update_service.py` | Git 更新检查和管理员触发更新。 |
| 通知 | `notification.py` | 站内/外部通知聚合入口。 |

### 数据库模块

| 模块 | 文件 | 数据 |
| ---- | ---- | ---- |
| 用户 | `src/db/user.py` | 用户、角色、Telegram 绑定码、换绑申请、BGM Token 状态、媒体库自助显隐标记。 |
| 注册码 | `src/db/regcode.py` | 注册码、续期码、白名单码、诱饵码、使用记录。 |
| API Key | `src/db/apikey.py` | API Key、权限、启停状态。 |
| 邀请 | `src/db/invite.py` | 邀请码和邀请关系。 |
| 求片 | `src/db/require.py`、`bangumi.py` | TMDB/Bangumi 求片请求。 |
| 播放 | `src/db/playback.py` | 播放历史与统计来源。 |
| 登录 | `src/db/login_log.py` | 登录历史、设备和客户端信息。 |
| 签到 | `src/db/signin.py` | 签到记录、积分流水。 |
| 公告 | `src/db/announcement.py` | 公告内容、可见性、过期时间。 |
| 调度 | `src/db/scheduler_schedule.py`、`scheduler_run.py` | 任务计划覆盖与运行历史。 |
| Telegram 花名册 | `src/db/telegram_roster.py` | 群成员观察记录。 |

### 前端页面

| 区域 | 页面 |
| ---- | ---- |
| 认证 | 登录、注册、忘记密码。 |
| 用户 | 仪表盘、媒体搜索/求片、签到积分、邀请码、公告、设置、外观、背景、API Key。 |
| 管理 | 用户管理、Emby 管理、注册码管理、配置管理、定时任务、公告管理、求片管理、邀请管理、统计、Telegram 换绑申请、系统测试页。 |
| 测试/演示 | `testweb*` 页面复刻真实前端主要界面，但只调用 `/api/v1/demo/*` 模拟接口，不执行真实业务操作。 |

### Telegram Bot

| 模块 | 能力 |
| ---- | ---- |
| 用户命令 | `/start`、绑定、个人信息、帮助、基础 Emby 信息。 |
| 管理命令 | 用户查询、Bot 状态、Emby 状态、管理辅助操作。 |
| 群组管理 | 群内用户查询、管理员身份校验、退群/成员资格相关处理。 |

### 文档体系

| 文档 | 用途 |
| ---- | ---- |
| `README.md` | 项目入口、功能概览、快速文档链接。 |
| `docs/README.md` | 文档导航。 |
| `docs/INSTALL.md` | 部署安装。 |
| `docs/FRONTEND.md` | 前端开发和联调。 |
| `docs/DEVELOPMENT.md` | 架构决策和编码规则。 |
| `docs/DEVELOPER_WORKFLOW.md` | Git 分支、开发、验证、合并流程。 |
| `docs/BACKEND_API.md`、`API_INDEX.md`、`API_KEY_API.md` | API 规范和索引。 |
| `docs/REGCODES.md` | 卡码类型、生成、安全和兼容性。 |
| `docs/SECURITY.md` | 生产安全基线。 |

## 版本步进流程

1. 所有开发从 `dev` 分支开始，不直接在 `main` 开发。
2. 开发前确认 `dev` 基于最新 `main`。
3. 修改代码、配置或文档时同步更新对应文档。
4. 若变更用户可见功能或行为，更新本文件的“未发布”记录或新增目标版本段落。
5. 发布前统一更新版本号位置：`src/__init__.py`、`pyproject.toml`、`webui/package.json`、`webui/package-lock.json`。
6. 运行验证：至少执行 Python 编译检查、前端 lint；涉及接口或业务流程时补充专项测试。
7. 由维护者检查 `git diff`，确认无密钥、无本地配置、无生成物误提交。
8. 维护者手动 push `dev` 并创建合并请求或本地合并到 `main`。
9. 合并后由维护者打 tag，例如 `v0.0.1`，并发布变更说明。

## 发布检查清单

- 版本号已更新且前后端一致。
- 文档已同步更新。
- `docs/API_INDEX.md` 与新增/变更接口一致。
- `docs/SECURITY.md` 或相关安全说明已覆盖新增风险。
- 配置变更已写入 `config.py`、schema、示例配置和迁移规则。
- 未提交 `config.toml` 中的真实密钥、数据库、上传文件、备份文件。
- 已在 `dev` 分支完成验证，`main` 仅由维护者合并。
