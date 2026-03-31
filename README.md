<div align="center">

# Twilight 暮光

## Next Generation Emby/Jellyfin Manager

[![Python](https://img.shields.io/badge/Python-3.10+-blue?logo=python&logoColor=white)](https://www.python.org/)
[![Flask](https://img.shields.io/badge/Flask-3.x-green?logo=flask&logoColor=white)](https://flask.palletsprojects.com/)
[![Next.js](https://img.shields.io/badge/Next.js-16.0+-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![SQLite](https://img.shields.io/badge/SQLite-3-blue?logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

</div>

---

## ✨ 功能特性

| 模块 | 说明 |
|------|------|
| **Emby/Jellyfin 管理** | 用户注册/续期/禁用、媒体库权限控制、NSFW 独立权限、会话与设备管理、账号绑定 |
| **积分系统** | 每日签到（连签加成）、积分转账、拼手气/均分红包、积分续期、排行榜 |
| **求片功能** | TMDB + Bangumi 多源搜索、库存自动检查（含季度）、请求-审核流程 |
| **Bangumi 同步** | Webhook 接收播放事件，自动标记 Bangumi 观看记录，支持 Emby/Jellyfin/Plex |
| **安全** | 设备数/播放数限制、IP 黑名单、登录日志、API Key 细粒度权限（6 种范围） |
| **Web 管理界面** | 基于 Next.js 16 的响应式 UI，可视化配置编辑器、内置 API 测试工具 |
| **扩展集成** | RESTful API、API Key 外部接口、Webhook 推送、可选 Telegram Bot |

---

## 🚀 快速开始

> 详细步骤请参考 **[安装部署指南](docs/INSTALL.md)**

### 环境要求

- **Python** 3.10+（推荐 3.11+）
- **Node.js** 18+（用于前端，推荐 20+）
- **Emby/Jellyfin** 已部署的服务器

### 安装 & 启动

```bash
# 克隆项目
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight

# 创建虚拟环境并安装依赖
python -m venv venv
# Windows: .\venv\Scripts\Activate.ps1
# Linux/macOS: source venv/bin/activate
pip install -r requirements.txt

# 编辑配置（至少填写 Emby 地址和 Token）
# 参考 config.production.toml 获取完整配置项
nano config.toml  # 或 notepad config.toml

# 启动后端
python main.py api
```

### 启动模式

```bash
python main.py api          # 仅 API 服务
python main.py bot          # 仅 Telegram Bot
python main.py scheduler    # 仅定时任务
python main.py all          # 全部启动

# 生产环境
uvicorn asgi:app --host 0.0.0.0 --port 5000 --workers 4
```

### 前端（可选）

```bash
cd webui && pnpm install && pnpm dev
# 访问 http://localhost:3000
```

### 首次使用

1. 在 `config.toml` 中配置 Emby 地址、Token，以及管理员用户名
2. 启动服务后访问 Web 界面注册管理员账号
3. 在管理后台创建注册码，分发给用户

---

## 📚 文档

| 文档 | 说明 |
|------|------|
| [文档导航](docs/README.md) | 统一入口，按角色快速定位 |
| [安装部署指南](docs/INSTALL.md) | 安装、配置、部署详细步骤 |
| [后端 API 文档](docs/BACKEND_API.md) | REST API 接口说明 |
| [API Key 文档](docs/API_KEY_API.md) | 外部系统接入指南 |
| [前端开发文档](docs/FRONTEND_API.md) | 前端技术栈与开发指南 |
| [开发指南](docs/DEVELOPMENT.md) | 编码规范、调试、贡献流程 |

运行时访问 `/api/v1/docs` 查看 Swagger UI 交互式文档。

---

## 🔌 Webhook 配置（Bangumi 同步）

**Emby**：管理面板 → 通知 → Webhook → URL 填 `https://your-domain/api/v1/webhook/bangumi/emby` → 勾选「播放-停止」

**Jellyfin**：安装 Webhook 插件 → Generic Destination → URL 填 `https://your-domain/api/v1/webhook/bangumi/jellyfin`

---

## 🙏 鸣谢

[Emby](https://emby.media/) / [Jellyfin](https://jellyfin.org/)
[TMDB](https://www.themoviedb.org/)
[Bangumi](https://bgm.tv/)
[Next.js](https://nextjs.org/)
[Sakura_embyboss](https://github.com/berry8838/Sakura_embyboss)
[Bangumi-syncer](https://github.com/SanaeMio/Bangumi-syncer)

## 📄 许可证

[MIT License](LICENSE)

---

<div align="center">

**如果这个项目对你有帮助，请给一个 ⭐ Star！**

Made with ❤️ by [Prejudice Studio](https://github.com/Prejudice-Studio/)

</div>
