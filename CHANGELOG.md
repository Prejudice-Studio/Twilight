# 更新日志

所有对 Twilight 项目的重大变更将在此文档中记录。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 文档

- 📚 新增 `docs/README.md` 作为统一文档导航入口
- 📚 重写 `docs/BACKEND_API.md`，修复认证段落结构和模块说明混排问题
- 📚 重写 `docs/FRONTEND_API.md`，明确 Next.js 主前端与历史 Vite 原型目录关系
- 🧹 更新 `README.md` 中的错误命令、失真链接与 Docker 误导描述
- 🧹 更新 `docs/INSTALL.md` 的仓库地址占位符与帮助链接
- 🧹 更新 `docs/DEVELOPMENT.md` 的跨平台命令示例（Windows / Linux）

## [1.0.0] - 2025-01-01

### 新增

#### 核心功能
- ✨ **Redis 支持** - 用于分布式会话存储和缓存
- ✨ **改进认证** - 使用 Redis 替代内存 Token 存储，支持多进程部署
- ✨ **健康检查 API** - `/api/v1/system/health` 端点，监控系统状态
- ✨ **系统统计 API** - `/api/v1/system/stats` 端点，获取 CPU/Memory/Disk 使用率
- ✨ **ASGI 支持** - 新增 `asgi.py` 用于生产级服务器（Uvicorn/Hypercorn）

#### 开发工具
- 🔧 **完整测试框架** - pytest 配置、测试用例、覆盖率报告
- 🔧 **开发脚本** - PowerShell 脚本（`dev.ps1`）简化常见操作
- 🔧 **Makefile** - 跨平台任务自动化
- 🔧 **CI/CD 工作流** - GitHub Actions 自动化测试和检查
- 🔧 **EditorConfig** - 统一编辑器配置

#### 文档
- 📚 **安装部署指南** - `docs/INSTALL.md`（包含 Windows 11 特定说明）
- 📚 **开发指南** - `docs/DEVELOPMENT.md`（编码规范、调试技巧等）
- 📚 **更新日志** - 本文件

### 改进

- ⚡ **异步优化** - 移除自定义 `@async_route` 装饰器，使用 Flask 原生 async 支持
- 🔒 **安全改进** - CORS 不再允许 `*` 配合凭证，需显式配置白名单
- 📦 **依赖管理** - 优化 requirements.txt，分离开发依赖到 requirements-dev.txt
- 🐍 **Windows 优化** - 特别针对 Windows 11 的安装步骤和脚本支持

### 破坏性变更

- ⚠️ **Token 存储变更** - 如使用 Redis，旧的内存 Token 在重启后失效
- ⚠️ **CORS 配置** - `APIConfig.CORS_ORIGINS` 现为必填（不能为空列表）

### 修复

- 🐛 **修复 Token 过期检查** - 正确处理过期 Token 的 Redis 清理
- 🐛 **修复数据库连接** - 改进异步数据库会话管理

### 已知问题

- 📋 Docker 部署尚未实现
- 📋 前后端集成文档仍在完善
- 📋 某些高级功能的单元测试覆盖率较低

---

## [0.5.0] - 2024-12-01

### 新增

- 初始项目结构
- 基础 REST API
- Emby 用户管理
- 积分系统
- Telegram Bot 支持
- Web 管理界面 (Next.js)

### 改进

- 优化数据库性能
- 完善 API 文档

---

## 版本历史说明

升级建议请参考 [安装部署指南](docs/INSTALL.md) 与 [文档导航](docs/README.md)。

## 贡献

欢迎提交 Pull Request 或报告 Issue！请参考 [贡献指南](docs/DEVELOPMENT.md#贡献流程)。
