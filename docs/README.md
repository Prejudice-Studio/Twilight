# Twilight 文档导航

按场景快速定位。文档分为三类：**指南（guides）** 面向部署与开发的操作流程，**参考（reference）** 面向接口与配置的速查，**功能专题（features）** 面向单个业务模块的设计与用法。

## 新手与部署

| 文档 | 适用人群 |
| ---- | -------- |
| [项目概览](../README.md) | 所有用户 |
| [安装部署](./guides/install.md) | Linux / systemd / Nginx / PostgreSQL 部署运维 |

## 指南 guides

| 文档 | 用途 |
| ---- | ---- |
| [安装部署](./guides/install.md) | 后端构建、PostgreSQL、HTTPS 反代、systemd 一键脚本、运行数据与密钥 |
| [开发指南](./guides/development.md) | 目录结构、后端/前端命令、API 与安全规范、Git 分支与发布流程 |
| [模块化架构与解耦指南](./guides/modular-architecture.md) | 后端/前端分层边界、依赖方向、大文件拆分顺序与 review 清单 |
| [前端多语言开发](./guides/i18n.md) | WebUI locale 命名、语言文件、翻译接入与新增语言流程 |
| [安全加固](./guides/security.md) | 生产安全基线：CORS、SSRF、限流、密钥、上传、自动更新检查清单 |

## 参考 reference

| 文档 | 用途 |
| ---- | ---- |
| [Go 后端架构与配置](./reference/backend.md) | 目录结构、配置加载、环境变量、Redis、状态存储、迁移、运行日志 |
| [API 路由索引](./reference/api-index.md) | `/api/v1` 完整路由清单、鉴权级别、模块归属（依据 `routes.go`） |
| [后端 API 详参](./reference/backend-api.md) | REST 接口规范、认证、错误码、请求/响应示例 |
| [API Key 外部接入](./reference/api-key.md) | 第三方系统集成、权限矩阵、调用示例 |

## 功能专题 features

| 文档 | 用途 |
| ---- | ---- |
| [注册码与卡码](./features/regcodes.md) | 注册码 / 续期码 / 白名单码 / 诱饵码规则、生成格式与随机算法 |
| [邮箱验证与找回密码](./features/email.md) | SMTP 发信、验证码格式/有效期、强制绑定门、改密二次校验、邮箱找回、域名黑白名单、管理员邮箱管理区 |
| [邀请树](./features/invite.md) | 邀请关系管理、配置、用户/管理员接口、删除与启停级联语义 |
| [公告系统](./features/announcements.md) | 公告渲染模式（plain / markdown / bbcode）与前端安全清洗 |
| [Bangumi 同步](./features/bangumi.md) | Emby Webhook、Bangumi Token、用户个人同步规则 |
| [背景与头像](./features/background.md) | 受控上传资源读取、背景 CSS 安全约束 |
| [Telegram Bot 命令](./features/telegram-bot.md) | Bot 命令、权限边界、群聊安全约束与文案配置 |

## 其他

- Swagger 交互式文档：服务启动后访问 `/api/v1/docs`

## 说明

- 若文档与代码行为冲突，以 `internal/api/`、`internal/store/`、`internal/config/` 与实际接口返回为准。
- 全部文档已对照 Go 后端源码核对；旧 Python 时代的描述（独立 SQLite 库、`X-Twilight-Client` 写请求校验等）已订正。
- 关键架构约定（状态存储单文档模型、PostgreSQL 仅保留 `twilight_state`/`twilight_sessions`/`twilight_runtime_logs`、配置热重载、CORS 与鉴权边界等）见 [开发指南](./guides/development.md) 与 [安全加固](./guides/security.md)。
