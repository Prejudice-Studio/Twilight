# 模块化架构

Twilight 按后端领域、存储模型和前端页面分层维护。改动应尽量落在已有归属边界内。

## 后端分层

| 层级 | 职责 |
| ---- | ---- |
| `internal/api` | HTTP 路由、鉴权、handler、外部服务 client、响应结构 |
| `internal/store` | 状态模型、持久化、迁移和原子更新 |
| `internal/config` | 配置加载、默认值和环境变量覆盖 |
| `internal/security` | 密码、Token 和安全随机数 |

handler 应保持清晰，领域规则优先放在相邻 helper 或 service 中，避免跨模块随意调用。

## 前端分层

| 层级 | 职责 |
| ---- | ---- |
| `webui/src/app` | App Router 页面和路由级组合 |
| `webui/src/components` | 可复用 UI 和功能面板 |
| `webui/src/lib/api.ts` | API 客户端 |
| `webui/src/lib/api-types.ts` | 请求/响应类型 |
| `webui/src/locales` | 用户可见文案 |

## 重构原则

- 优先复用现有 helper、hook 和 UI 组件。
- 只在能减少真实复杂度时新增抽象。
- 大页面可拆为领域组件，但不要为了形式拆分。
- 行为变化必须同步文档和 i18n。
