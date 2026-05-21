# 版本更新与功能规划

本文档记录 Twilight 的版本号、更新内容、功能边界和后续版本推进流程。当前基线版本为 `0.0.1`。

## 版本策略

Twilight 使用语义化版本号：`MAJOR.MINOR.PATCH`。

| 位置 | 含义 |
| ---- | ---- |
| `MAJOR` | 破坏性变更、数据模型不兼容、部署方式重大调整。 |
| `MINOR` | 新功能、新模块、兼容性增强。 |
| `PATCH` | Bug 修复、安全修复、文档补充、无破坏性小改动。 |

当前项目仍处于早期整理阶段，`0.x` 版本默认表示 API、配置项和数据库结构仍可能调整。进入 `1.0.0` 前必须完成核心功能冻结、迁移策略、测试覆盖和发布流程固化。

## 当前版本

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
| Users | `/api/v1/users` | 注册、个人资料、密码、Emby 绑定/补建、注册码使用、续期、Telegram 绑定、头像和背景资源。 |
| Admin | `/api/v1/admin` | 用户管理、注册队列处理、注册码管理、Emby 管理、求片审核、邀请树、公告、调度任务、Telegram 管理工具。 |
| System | `/api/v1/system` | 系统信息、健康检查、Emby 线路、配置 schema、配置保存、配置清理、管理统计、在线更新。 |
| Emby | `/api/v1/emby` | Emby 状态、媒体库、搜索、最新媒体、会话数；旧线路接口返回弃用提示。 |
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
| 用户服务 | `src/services/user_service.py` | 注册、待补建 Emby、续期、白名单、密码、角色、Emby 同步。 |
| Emby 客户端 | `src/services/emby.py`、`emby_service.py` | Emby 认证、用户、媒体库、会话、活动、策略同步。 |
| 注册队列 | `emby_register_queue.py`、`regcode_use_queue.py` | Emby 补建和卡码使用的排队、状态查询、限流和 token 校验。 |
| 邀请服务 | `invite_service.py` | 邀请关系、邀请码、邀请树、被邀请用户处理。 |
| 求片服务 | `media_service.py` | 搜索聚合、库存检查、求片状态流转。 |
| Bangumi | `bangumi.py`、`bangumi_search.py`、`bangumi_sync.py` | Bangumi 搜索、条目同步、观看记录同步。 |
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
| 用户 | `src/db/user.py` | 用户、角色、Telegram 绑定码、换绑申请。 |
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
