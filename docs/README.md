# Twilight 文档导航

本文档中心用于快速定位 Twilight 的部署、开发、接口和功能说明。除 `AGENTS.md` 外，项目介绍类文档统一使用中文，避免中英混排和乱码。

## 新手入口

| 文档 | 适用场景 |
| ---- | ---- |
| [项目概览](../README.md) | 了解 Twilight 的定位、架构和常用命令 |
| [安装部署](./guides/install.md) | Linux、systemd、反向代理和备份 |
| [开发指南](./guides/development.md) | 本地开发、验证命令和提交前检查 |
| [安全加固](./guides/security.md) | 生产环境上线前的安全基线 |

## 指南

| 文档 | 用途 |
| ---- | ---- |
| [安装部署](./guides/install.md) | 构建、配置、systemd、反向代理和运维备份 |
| [开发指南](./guides/development.md) | 目录结构、后端/前端命令、API 与 i18n 约定 |
| [模块化架构](./guides/modular-architecture.md) | 后端/前端分层边界、重构规则和代码归属 |
| [前端 i18n](./guides/i18n.md) | 语言文件、翻译键、语言切换和布局稳定性 |
| [Docker 部署](./guides/docker.md) | 不推荐但保留的 Docker 说明与风险提示 |
| [安全加固](./guides/security.md) | 密钥、CORS、SSRF、上传、审计和公开接口边界 |

## 参考

| 文档 | 用途 |
| ---- | ---- |
| [Go 后端参考](./reference/backend.md) | 后端入口、配置、存储、调度器和运行状态 |
| [API 路由索引](./reference/api-index.md) | 由 `routes.go` 对齐的路由表 |
| [后端 API 参考](./reference/backend-api.md) | 响应格式、鉴权、错误码和重点接口说明 |
| [API Key 接入](./reference/api-key.md) | 第三方集成、调用方式和密钥安全 |
| [开发者 JS 沙箱](./reference/developer-js.md) | Telegram Bot JS 扩展能力和安全模型 |

## 功能专题

| 文档 | 用途 |
| ---- | ---- |
| [注册码与卡码](./features/regcodes.md) | 注册码、续期码、白名单码和来源规则 |
| [邮箱验证](./features/email.md) | SMTP、验证码、找回密码、强制绑定和清理 |
| [邀请树](./features/invite.md) | 邀请关系、续期、级联操作和禁用语义 |
| [公告系统](./features/announcements.md) | 公告渲染模式、安全解析和展示规则 |
| [Bangumi 同步](./features/bangumi.md) | 同步、收藏管理、缓存、封面和看过状态 |
| [播放统计](./features/playback-stats.md) | Emby ActivityLog 统计、刷新策略和限制 |
| [背景与头像](./features/background.md) | 上传、安全路径和认证页背景 |
| [Telegram Bot](./features/telegram-bot.md) | 绑定、通知、命令、花名册和 JS 扩展 |

## 在线界面

- 用户 Wiki：`/wiki`
- API 控制台：`/api/v1/docs`
- 公开 OpenAPI 摘要：`/api/v1/openapi.json`
- 管理员完整路由清单：`/api/v1/system/admin/apis`

如果文档与当前代码不一致，以 `internal/api`、`internal/store`、`internal/config` 和 `webui/src/lib/api.ts` 的实际行为为准。
