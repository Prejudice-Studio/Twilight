<div align="center">

![Twilight Logo](Twilight%20Logo.png)

# Twilight 暮光

Emby / Jellyfin 用户管理面板。

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![Version](https://img.shields.io/badge/Version-0.0.6-blue)](docs/changelog.md)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

[Telegram 频道](https://t.me/Twilightpanel) · [Telegram 群组](https://t.me/TwilightPanelChat) · [文档导航](docs/README.md)

</div>

## 核心能力

- 用户管理：注册、登录、续期、禁用、删除、白名单、设备与登录记录。
- 卡码体系：注册码、续期码、白名单码、诱饵码、指名码、批量生成与审计。
- 邀请系统：邀请码、续期邀请、邀请树和管理员邀请森林视图。
- 媒体服务：Emby / Jellyfin 账号绑定、创建、解绑、媒体库权限和播放统计。
- 求片系统：TMDB / Bangumi 搜索、库存检查、管理员审核和外部回调。
- Telegram Bot：账号绑定、个人查询、管理员只读查询、群组成员安全管理。
- 运维后台：实时日志、服务器状态、配置热重载、数据库备份/恢复/迁移、Git 更新。

## 文档导航

完整导航见 [文档中心](docs/README.md)。常用入口：

| 文档 | 说明 |
| ---- | ---- |
| [安装部署](docs/guides/install.md) | Linux、systemd、1Panel、Nginx、PostgreSQL 部署 |
| [开发指南](docs/guides/development.md) | 目录结构、后端/前端命令、API 与安全规范、发布流程 |
| [安全加固](docs/guides/security.md) | 生产安全基线与上线检查清单 |
| [Go 后端架构与配置](docs/reference/backend.md) | 后端架构、配置加载、环境变量、Redis、迁移 |
| [API 路由索引](docs/reference/api-index.md) | `/api/v1` 完整路由清单与鉴权级别 |
| [后端 API 详参](docs/reference/backend-api.md) | REST API 规范、认证、错误码、示例 |
| [API Key 外部接入](docs/reference/api-key.md) | 第三方集成与权限矩阵 |
| [注册码与卡码](docs/features/regcodes.md) | 注册码/续期码/白名单码算法和使用规则 |
| [邀请树](docs/features/invite.md) | 邀请森林、级联删除与启停语义 |
| [Telegram Bot 命令](docs/features/telegram-bot.md) | Bot 命令、私聊边界、管理员命令 |
| [Bangumi 同步](docs/features/bangumi.md) | Emby Webhook 与 Bangumi Token 配置 |
| [版本历史](docs/changelog.md) | 更新记录与发布文案 |

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
