# Twilight 文档导航

按场景快速定位：

## 新手部署

| 文档 | 适用人群 |
| ---- | -------- |
| [项目概览](../README.md) | 所有用户 |
| [安装部署](./INSTALL.md) | Linux 部署运维 |

## 后端与接口

| 文档 | 用途 |
| ---- | ---- |
| [后端 API 参考](./BACKEND_API.md) | REST API 接口规范、认证、错误码 |
| [Go 后端说明](./GO_BACKEND.md) | Go 后端启动、配置、Redis 与验证说明 |
| [API 路由索引](./API_INDEX.md) | `/api/v1` 完整路由清单、认证级别、模块归属 |
| [API Key 外部接入](./API_KEY_API.md) | 第三方系统集成、权限矩阵 |
| [注册码与卡码说明](./REGCODES.md) | 注册码、续期码、白名单码和诱饵码规则 |

## 前端与开发

| 文档 | 用途 |
| ---- | ---- |
| [前端开发](./FRONTEND.md) | Next.js 前端本地开发与联调 |
| [开发指南](./DEVELOPMENT.md) | 编码规范、调试、关键架构决策 |
| [开发者流程](./DEVELOPER_WORKFLOW.md) | Git 分支、开发验证、维护者合并与发布流程 |
| [版本更新与规划](./VERSION_HISTORY.md) | 版本号、功能地图、路线图和发布检查清单 |

## 专题

- [背景自定义](./BACKGROUND.md) — 用户自定义主题背景的实现
- [Bangumi 点格子同步](./BANGUMI_SYNC.md) — Emby Webhook、Bangumi Token 与用户面板配置
- [Telegram Bot 命令](./TG_BOT_COMMANDS.md) — Bot 普通命令、管理员只读查询和群聊安全边界
- [注册码与卡码说明](./REGCODES.md) — 卡码类型、生成格式、兼容性与安全口径
- [版本更新与规划](./VERSION_HISTORY.md) — 当前版本、功能地图和后续版本步进流程
- [安全加固指南](./SECURITY.md) — 生产安全基线、密钥与部署检查清单
- [安全与性能优化记录](./SECURITY_AND_PERFORMANCE_REVIEW.md) — 上传资源、注册队列、定时任务与管理接口加固

## 说明

- Swagger 交互式文档：服务启动后访问 `/api/v1/docs`
- 若文档与代码行为冲突，以 `internal/api/` 与实际接口返回为准
- 关键架构决策（媒体库策略、配置重启、`.gitignore` 注意点等）见 [DEVELOPMENT.md](./DEVELOPMENT.md#关键架构决策)
