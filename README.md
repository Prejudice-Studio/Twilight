<div align="center">

![Twilight Logo](Twilight%20Logo.png)

# Twilight 暮光

**面向 Emby / Jellyfin 的用户、邀请、卡码、Bot 与运维管理面板**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![SvelteKit](https://img.shields.io/badge/SvelteKit-SSR-ff3e00?logo=svelte&logoColor=white)](https://kit.svelte.dev/)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue)](LICENSE)
[![Telegram](https://img.shields.io/badge/Telegram-频道-blue?logo=telegram)](https://t.me/Twilightpanel)

[快速开始](#快速开始) · [功能特性](#功能特性) · [在线文档](docs/README.md) · [更新日志](CHANGELOG.md) · [Telegram 群组](https://t.me/TwilightPanelChat)

</div>

---

## 项目简介

Twilight 是一个功能完善的 Emby / Jellyfin 用户管理系统，提供注册审核、卡码续期、邀请关系、Telegram Bot 绑定、设备审查等全方位管理功能。

### 技术栈

- **后端**: Go 1.25+ / PostgreSQL
- **前端**: SvelteKit SSR / TypeScript / Tailwind CSS
- **部署**: Docker / Linux systemd

## 功能特性

🔐 **用户管理** - 注册审核、续期、禁用、白名单、邮箱验证

📺 **媒体服务** - Emby / Jellyfin 账号绑定、开通、同步、线路下发

🎫 **卡码体系** - 注册码、续期码、白名单码、诱饵码批量生成

👥 **邀请系统** - 邀请码、续期邀请、邀请关系树、统计分析

🤖 **Telegram Bot** - 绑定、通知、换绑审核、群组成员管理

🎬 **求片系统** - TMDB / Bangumi 搜索、库存检查、管理员审核

🛡️ **安全中心** - 操作审计、实时日志、设备/IP 审查、风控策略

⚙️ **运维后台** - 配置热重载、数据库备份/恢复/迁移、调度任务

## 快速开始

### Docker Compose（推荐）

```bash
# 克隆仓库
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight

# 配置环境变量
cp deploy/docker/config.docker.toml config.toml
cp deploy/docker/.env.example .env
nano .env  # 修改数据库密码等配置

# 启动服务
docker compose up -d --build

# 访问 http://localhost:3000
```

### Linux 部署

```bash
# 构建后端
go build -o bin/twilight ./cmd/twilight

# 使用 systemd 一键部署
sudo bash deploy/setup-systemd.sh

# 详细说明见文档
```

### 首次初始化

1. 在 `config.toml` 中临时启用 `setup_mode = true`
2. 打开 WebUI，按照向导创建管理员账号
3. 配置 Emby、Telegram、邮箱等（可跳过稍后配置）

完整部署指南：[安装文档](docs/guides/install.md) | [Docker 部署](docs/guides/docker.md)

## 系统截图

<details>
<summary>点击展开查看</summary>

> 待补充：仪表盘、用户管理、Telegram 面板等截图

</details>

## 在线文档

| 文档 | 说明 |
| ---- | ---- |
| [文档中心](docs/README.md) | 所有文档入口 |
| [安装部署](docs/guides/install.md) | Linux、Docker、systemd 部署 |
| [开发指南](docs/guides/development.md) | 开发环境、架构、API 规范 |
| [API 文档](docs/reference/backend-api.md) | REST API 接口文档 |
| [功能专题](docs/features/) | 注册码、邀请、工单等功能说明 |

## 许可证

本项目采用 [GNU AGPL v3](LICENSE) 协议，允许商业使用但必须开源。详见 [NOTICE](NOTICE)。

- ✅ 允许商业部署、销售、托管服务
- ✅ 允许修改和二次开发
- ⚠️ 修改版必须开源并保留原项目署名
- ⚠️ 网络服务必须提供源代码

## 社区支持

- 💬 [Telegram 频道](https://t.me/Twilightpanel) - 更新公告
- 👥 [Telegram 群组](https://t.me/TwilightPanelChat) - 交流讨论
- 🐛 [提交 Issue](https://github.com/Prejudice-Studio/Twilight/issues) - 反馈问题
- 📖 [开发文档](docs/guides/development.md) - 参与贡献

## 鸣谢

感谢以下开源项目：

- [Emby](https://emby.media/) / [Jellyfin](https://jellyfin.org/)
- [SvelteKit](https://kit.svelte.dev/)
- [Sakura_embyboss](https://github.com/berry8838/Sakura_embyboss)
- [Bangumi-syncer](https://github.com/SanaeMio/Bangumi-syncer)

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=Prejudice-Studio/Twilight&type=Date)](https://star-history.com/#Prejudice-Studio/Twilight&Date)

---

<div align="center">

如果 Twilight 对你有帮助，欢迎点一个 ⭐ Star

Made with ❤️ by [Prejudice Studio](https://github.com/Prejudice-Studio/)

</div>
