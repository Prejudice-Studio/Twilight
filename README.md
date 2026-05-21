<div align="center">

# Twilight 暮光

新一代 Emby / Jellyfin 用户管理面板

[![Python](https://img.shields.io/badge/Python-3.10+-blue?logo=python&logoColor=white)](https://www.python.org/) [![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js&logoColor=white)](https://nextjs.org/) [![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

[Telegram 频道](https://t.me/Twilightpanel) · [Telegram 群组](https://t.me/TwilightPanelChat) · [文档导航](docs/README.md)

</div>

## 说明

> 本项目大量代码由 LLM 辅助生成，仅有部分人工修缮。请在使用前自行审查代码安全性与可靠性。任何因使用本项目导致的损失与责任均与项目维护者无关。

> 项目默认使用者具备基础 Linux、Python、前端与 Emby/Jellyfin 运维能力。当前不提供官方 Docker 部署方案。

> 所使用的LLM模型列表:
> Claude Code Opus 4.6 / 4.6-Thinking / 4.7 / 4.7-Thinking
> ChatGPT 5.3 CodeX / 5.5 

## 功能概览

| 模块 | 能力 |
| ---- | ---- |
| 用户管理 | 注册、续期、禁用、删除、角色、白名单、Emby 绑定 |
| Emby / Jellyfin | 账号创建、绑定同步、媒体库权限、播放会话管理 |
| 注册与续期 | 注册码、续期码、白名单码、待补建 Emby 账号 |
| 求片系统 | TMDB / Bangumi 搜索、库存检查、请求审核与外部更新 |
| Telegram Bot | 用户绑定、个人信息、管理员私聊命令、群组管理工具 |
| 安全与审计 | 登录记录、设备管理、IP 黑名单、API Key |
| 管理后台 | Next.js 响应式界面、配置编辑、定时任务 |

## 环境要求

- Python 3.10+，推荐 3.11+
- Node.js 22+
- 已部署的 Emby 或 Jellyfin 服务
- Linux 服务器基础使用能力

## 快速开始

详细部署步骤请看：[安装部署指南](docs/INSTALL.md)

| 文档 | 说明 |
| ---- | ---- |
| [安装部署](docs/INSTALL.md) | 后端、前端、服务配置 |
| [Telegram Bot 命令](docs/TG_BOT_COMMANDS.md) | Bot 命令与群组管理说明 |
| [Bangumi 点格子同步](docs/BANGUMI_SYNC.md) | Emby Webhook 与 Bangumi Token 配置 |
| [后端 API](docs/BACKEND_API.md) | REST API 规范与重点接口 |
| [API Key 外部接入](docs/API_KEY_API.md) | 外部系统调用说明 |
| [安全加固](docs/SECURITY.md) | 生产安全检查清单 |
| [版本更新与规划](docs/VERSION_HISTORY.md) | 当前版本、功能地图与后续规划 |
| [开发者流程](docs/DEVELOPER_WORKFLOW.md) | 分支、验证、合并与发布流程 |

服务启动后可访问 `/api/v1/docs` 查看 Swagger UI。

## 社区

- Telegram 频道：<https://t.me/Twilightpanel>
- Telegram 群组：<https://t.me/TwilightPanelChat>
- GitHub Issues：用于反馈可复现问题和功能建议

## 鸣谢

- [Emby](https://emby.media/)
- [Jellyfin](https://jellyfin.org/)
- [TMDB](https://www.themoviedb.org/)
- [Bangumi 番组计划](https://bgm.tv/)
- [Next.js](https://nextjs.org/)
- [Sakura_embyboss](https://github.com/berry8838/Sakura_embyboss)
- [Bangumi-syncer](https://github.com/SanaeMio/Bangumi-syncer)

## 贡献者

<div align="center">

[![Contributors](https://contrib.rocks/image?repo=Prejudice-Studio/Twilight)](https://github.com/Prejudice-Studio/Twilight/graphs/contributors)

</div>

## Star

[![Star History Chart](https://api.star-history.com/svg?repos=Prejudice-Studio/Twilight&type=Date)](https://star-history.com/#Prejudice-Studio/Twilight&Date)

</div>

<div align="center">

如果 Twilight 对你有帮助，欢迎点一个 Star。

Made with ❤️ by [Prejudice Studio](https://github.com/Prejudice-Studio/)

</div>
