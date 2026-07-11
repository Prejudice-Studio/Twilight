<div align="center">

![Twilight Logo](Twilight%20Logo.png)

# Twilight 暮光

面向 Emby / Jellyfin 社群的用户、邀请、卡码、Bot 与运维管理面板。

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

[文档中心](docs/README.md) | [安装部署](docs/guides/install.md) | [API 控制台](/api/v1/docs) | [Telegram 频道](https://t.me/Twilightpanel) | [Telegram 群组](https://t.me/TwilightPanelChat)

</div>

## 项目定位

Twilight 是一个 Go 后端 + Next.js 前端的 Emby / Jellyfin 用户管理系统，适合需要注册审核、卡码续期、邀请关系、Telegram Bot 绑定、设备/IP 审查和后台运维能力的媒体服务器站点。

## 技术架构

- 后端：Go，入口为 `cmd/twilight`，推荐部署方式为 Linux + systemd。
- 前端：Next.js App Router、TypeScript、Tailwind CSS、Radix/shadcn 风格组件。
- 存储：单一状态文档模型，支持 JSON 文件或 PostgreSQL。
- 配置：统一读取 `config.toml`、`config.local.toml` 与 `TWILIGHT_*` 环境变量。

## 常用命令

```powershell
# 后端验证
go test ./...
go vet ./...

# 前端验证
cd webui
pnpm lint
pnpm build
```

## 文档

- [文档导航](docs/README.md)
- [开发指南](docs/guides/development.md)
- [安装部署](docs/guides/install.md)
- [安全加固](docs/guides/security.md)
- [API 路由索引](docs/reference/api-index.md)

如果文档与代码行为不一致，以当前 `internal/api`、`internal/store`、`internal/config` 和 `webui/src/lib/api.ts` 为准。
