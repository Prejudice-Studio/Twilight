# Twilight 文档导航

按场景快速定位：

## 新手部署

| 文档 | 适用人群 |
| ---- | -------- |
| [项目概览](../README.md) | 所有用户 |
| [安装部署](./INSTALL.md) | 部署运维 |
| [Windows 快速启动](./QUICKSTART-Windows.md) | Windows 用户首次试用 |

## 后端与接口

| 文档 | 用途 |
| ---- | ---- |
| [后端 API 参考](./BACKEND_API.md) | REST API 接口规范、认证、错误码 |
| [API 路由索引](./API_INDEX.md) | `/api/v1` 完整路由清单、认证级别、模块归属 |
| [API Key 外部接入](./API_KEY_API.md) | 第三方系统集成、权限矩阵 |

## 前端与开发

| 文档 | 用途 |
| ---- | ---- |
| [前端开发](./FRONTEND.md) | Next.js 前端本地开发与联调 |
| [开发指南](./DEVELOPMENT.md) | 编码规范、调试、关键架构决策 |

## 专题

- [背景自定义](./BACKGROUND.md) — 用户自定义主题背景的实现
- [邀请树 & 公告渲染](./INVITE_AND_ANNOUNCEMENTS.md) — 多级邀请森林、Markdown/BBCode 公告
- [安全加固指南](./SECURITY.md) — 生产安全基线、密钥与部署检查清单
- [安全与性能优化记录](./SECURITY_AND_PERFORMANCE_REVIEW.md) — 上传资源、注册队列、定时任务与管理接口加固

## 说明

- Swagger 交互式文档：服务启动后访问 `/api/v1/docs`
- 若文档与代码行为冲突，以 `src/api/` 与实际接口返回为准
- 关键架构决策（媒体库策略、配置重启、`.gitignore` 注意点等）见 [DEVELOPMENT.md](./DEVELOPMENT.md#关键架构决策)
