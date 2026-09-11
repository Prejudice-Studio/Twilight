# Changelog

本项目的所有重要变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增

- **Telegram 身份历史追踪**: 记录用户 Telegram Username 和 UserID 的变更历史，支持审计和账号安全监控
- **统一模板参数系统**: 支持约 60 个参数的统一模板系统，可用于登录通知、工单通知和 Telegram 群组面板
- **Telegram inline 面板增强**: 
  - 新增邮箱显示功能
  - 新增快速白名单操作
  - 优化面板布局和交互体验
- **V2 数据库兼容性**: V2 表结构使用 `CREATE TABLE IF NOT EXISTS`，支持从 V1 无缝升级

### 改进

- **文档完善**: 
  - 新增 [Telegram 身份历史文档](docs/v2/telegram-identity-history.md)
  - 完善 [模板参数完整参考](docs/v2/template-parameters.md)
  - 优化文档导航结构
- **代码质量**: 修复中文引号导致的编译错误
- **README 简化**: 参照知名中文开源项目优化 README 结构

### 修复

- 修复 ticket_handlers.go 中中文引号导致的编译错误
- 修复模板参数系统集成问题

## [V2.0.0] - 架构重构

### 重大变更

- **架构重构**: 引入应用服务层，将业务逻辑从 HTTP 传输层分离
- **前端迁移**: 从 Next.js 迁移到 SvelteKit SSR
- **数据库统一**: 运行后端收敛为单一 PostgreSQL，消除 JSON 后端
- **许可证变更**: 从 MIT 切换到 AGPL-3.0

### 新增

- **V2 API**: 94.4% 路由覆盖率 (321/340)
- **缓存控制**: 所有 V2 端点明确声明缓存策略
- **安全增强**:
  - CSRF 保护 (Double Submit Cookie)
  - 速率限制 (Redis + 内存降级)
  - 路径遍历防护
  - 输入脱敏 (38 个文件)
  - 审计日志增强
- **JS 沙箱**: 完善的开发者 JS 沙箱，支持 Telegram Bot 自定义命令
  - 8 秒执行超时
  - SSRF 防护
  - DNS rebinding 防护
  - 网络访问限制 (1.5s 超时, 8KB 响应体限制)

### 改进

- **运行日志**: 从 state.json 剥离到独立旁路文件，热路径由整库写降为单行 append
- **工单系统**: 分页加载管理员列表
- **Emby 管理**: Emby 用户同步、设备审查、播放记录追踪
- **求片系统**: TMDB / Bangumi 搜索、库存检查、管理员审核
- **邀请系统**: 邀请关系树、统计分析、数据修复工具

### 文档

- [V2 架构设计](docs/v2/architecture.md)
- [V2 API 迁移指南](docs/v2/migration-guide.md)
- [V2 SSR 前端](docs/v2/frontend-ssr.md)
- [V2 安全审计报告](docs/v2/security-audit-report.md)
- [V1 审计基线](docs/v2/v1-audit.md)

## [V1.x] - 初始版本

### 功能特性

- 用户管理: 注册审核、续期、禁用、白名单、邮箱验证
- 媒体服务: Emby / Jellyfin 账号绑定、开通、同步、线路下发
- 卡码体系: 注册码、续期码、白名单码、诱饵码批量生成
- 邀请系统: 邀请码、续期邀请、邀请关系树、统计分析
- Telegram Bot: 绑定、通知、换绑审核、群组成员管理
- 求片系统: TMDB / Bangumi 搜索、库存检查、管理员审核
- 安全中心: 操作审计、实时日志、设备/IP 审查、风控策略
- 运维后台: 配置热重载、数据库备份/恢复/迁移、调度任务

[Unreleased]: https://github.com/Prejudice-Studio/Twilight/compare/HEAD...HEAD
[V2.0.0]: https://github.com/Prejudice-Studio/Twilight/releases/tag/v2.0.0
[V1.x]: https://github.com/Prejudice-Studio/Twilight/releases/tag/v1.0.0
