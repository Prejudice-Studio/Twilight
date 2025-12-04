<div align="center">

# Twilight 暮光

## Next Generation Emby/Jellyfin Manager

[![Python](https://img.shields.io/badge/Python-3.10+-blue?logo=python&logoColor=white)](https://www.python.org/)
[![Flask](https://img.shields.io/badge/Flask-3.x-green?logo=flask&logoColor=white)](https://flask.palletsprojects.com/)
[![Next.js](https://img.shields.io/badge/Next.js-16.0+-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![SQLite](https://img.shields.io/badge/SQLite-3-blue?logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

[功能特性](#-功能特性) •
[快速开始](#-快速开始) •
[配置说明](#-配置说明) •
[API 文档](#-api-文档) •
[部署指南](#-部署指南) •

</div>

---

## ✨ 功能特性

### 🎬 Emby/Jellyfin 管理
- **用户管理** - 注册、续期、禁用、删除、批量操作
- **媒体库权限** - 灵活的媒体库访问控制
- **会话管理** - 查看、踢出用户会话
- **设备管理** - 设备数量限制、设备移除
- **NSFW 控制** - 可配置的成人内容访问权限
- **账号绑定** - 支持绑定已有 Emby 账号

### 💰 积分系统
- **每日签到** - 可配置的签到奖励和连签加成
- **积分转账** - 用户间积分转账，支持手续费
- **红包系统** - 拼手气红包 / 均分红包
- **积分续期** - 使用积分自动/手动续期账号
- **积分历史** - 完整的积分变动记录
- **积分排行榜** - 实时积分排名

### 🎯 求片功能
- **多源搜索** - 支持 TMDB + Bangumi 联合搜索
- **库存检查** - 自动检查媒体库是否已有（支持季度检查）
- **请求管理** - 用户请求、管理员审核流程
- **智能匹配** - 自动匹配媒体库中的内容

### 📺 Bangumi 同步
- **自动点格子** - 通过 Webhook 接收播放完成事件，自动标记 Bangumi 观看记录
- **多端支持** - 支持 Emby、Jellyfin、Plex Webhook
- **自定义映射** - 无法匹配的番剧可手动添加映射
- **隐私控制** - 可配置观看记录是否公开

### 🔐 安全特性
- **设备限制** - 限制用户最大设备数和同时播放数
- **IP 限制** - IP 黑名单、登录失败锁定
- **登录日志** - 完整的登录记录追踪
- **API 认证** - 支持 API Key 和 Token 双认证
- **会话管理** - 多设备登录管理

### 📡 扩展集成
- **RESTful API** - 完整的 REST API 支持前端开发
- **API Key 接口** - 专门为外部系统设计的 API Key 认证接口
- **Webhook** - 接收和推送 Webhook 事件
- **Telegram Bot** - 可选的 Telegram 机器人交互（需手动开启）

### 🖥️ Web 管理界面
- **现代化 UI** - 基于 Next.js 16 的现代化管理界面
- **响应式设计** - 完美支持桌面和移动设备
- **实时数据** - 实时更新的用户数据和统计信息
- **配置管理** - 管理员可直接在 Web 界面管理配置文件
- **API 测试工具** - 内置 API 测试工具，支持列出所有接口

---

## 🚀 快速开始

### 环境要求

- Python 3.10+
- Node.js 18+ (用于前端)
- Emby Server / Jellyfin Server
- （可选）TMDB API Key
- （可选）Telegram Bot Token

### 安装步骤

#### 1. 克隆项目

```bash
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight
```

#### 2. 后端设置

```bash
# 创建虚拟环境
python -m venv venv

# 激活虚拟环境
# Windows
venv\Scripts\activate
# Linux/macOS
source venv/bin/activate

# 安装依赖
pip install -r requirements.txt

# 复制配置文件
cp config.production.toml config.toml

# 编辑配置文件
# 修改 config.toml 中的 Emby 地址和 Token
```

#### 3. 前端设置

```bash
cd webui

# 安装依赖
npm install

# 配置环境变量（可选）
# 创建 .env.local 文件
echo "NEXT_PUBLIC_API_URL=http://localhost:5000" > .env.local
```

#### 4. 启动服务

**后端**:
```bash
# 在项目根目录
python main.py
```

**前端**:
```bash
# 在 webui 目录
npm run dev
```

访问 `http://localhost:3000` 即可使用 Web 界面。

### Docker 部署（推荐）

```bash
# 构建后端镜像
docker build -t twilight-backend .

# 构建前端镜像
cd webui
docker build -t twilight-frontend .

# 运行后端容器
docker run -d \
  --name twilight-backend \
  -p 5000:5000 \
  -v ./config.toml:/app/config.toml \
  -v ./db:/app/db \
  twilight-backend

# 运行前端容器
docker run -d \
  --name twilight-frontend \
  -p 3000:3000 \
  -e NEXT_PUBLIC_API_URL=http://your-backend-url:5000 \
  twilight-frontend
```

---

## ⚙️ 配置说明

配置文件为 `config.toml`，主要配置项：

### 基础配置

```toml
[Global]
# 是否开启日志
logging = true
# 日志等级 (10=DEBUG, 20=INFO, 30=WARNING, 40=ERROR)
log_level = 20
# 是否开启SQLAlchemy日志
sqlalchemy_log = false
# 是否开启 Telegram 模式
telegram_mode = false
# 是否强制绑定 Telegram
force_bind_telegram = false

# TMDB 配置（用于媒体搜索）
tmdb_api_key = "your_tmdb_api_key"
tmdb_api_url = "https://api.themoviedb.org/3"
```

### Emby 配置

```toml
[Emby]
# Emby 服务器地址
emby_url = "http://127.0.0.1:8096/"
# Emby API Token
emby_token = "your_emby_api_token"
# Emby 地址列表 (用于展示)
emby_url_list = [
    "Direct : http://127.0.0.1:8096/",
    "Sample : http://192.168.1.1:8096/"
]
# NSFW 库 ID (可选)
emby_nsfw = ""
```

### 积分配置

```toml
[SAR]
# 积分名称
score_name = "暮光币"
# 是否开启注册功能
register_mode = false
# 用户数量上限
user_limit = 200

# 签到配置
checkin_base_score = 10          # 签到基础奖励
checkin_streak_bonus = 2         # 连签每天加成
checkin_max_streak_bonus = 20    # 最大连签加成
checkin_random_min = 0           # 随机奖励最小值
checkin_random_max = 5           # 随机奖励最大值

# 自动续期
auto_renew_enabled = false       # 是否允许积分自动续期
auto_renew_days = 30             # 自动续期天数
auto_renew_cost = 100            # 自动续期所需积分
auto_renew_before_days = 3       # 到期前几天自动续期
```

### 设备限制

```toml
[DeviceLimit]
# 是否启用设备限制
device_limit_enabled = false
# 最大设备数
max_devices = 5
# 最大同时播放数
max_streams = 2
# 超限时是否踢掉最早的会话
kick_oldest_session = false
```

### API 服务器配置

```toml
[API]
# API 服务器监听地址
host = "0.0.0.0"
# API 服务器端口
port = 5000
# 是否开启调试模式
debug = false
# Token 过期时间 (秒)
token_expire = 86400
# 是否允许跨域 (CORS)
cors_enabled = true
```

### Bangumi 同步

```toml
[BangumiSync]
# 是否启用 Bangumi 同步功能
enabled = false
# 同步时是否自动添加到收藏（设为"在看"）
auto_add_collection = true
# 观看记录是否设为私有
private_collection = true
# 屏蔽关键词列表
block_keywords = []
# 最小播放进度（百分比）才算看完
min_progress_percent = 80
```

> 📝 完整配置请参考 `config.production.toml`

---

## 📚 API 文档

### API 概览

| 模块 | 前缀 | 说明 | 认证方式 |
|------|------|------|---------|
| Auth | `/api/v1/auth` | 认证登录 | - |
| Users | `/api/v1/users` | 用户管理 | Token |
| Score | `/api/v1/score` | 积分系统 | Token |
| Emby | `/api/v1/emby` | Emby 操作 | Token |
| Admin | `/api/v1/admin` | 管理员接口 | Token (Admin) |
| Media | `/api/v1/media` | 媒体搜索/求片 | Token |
| Stats | `/api/v1/stats` | 播放统计 | Token |
| Webhook | `/api/v1/webhook` | Webhook 接收/推送 | Secret |
| Security | `/api/v1/security` | 安全设置 | Token (Admin) |
| System | `/api/v1/system` | 系统信息 | Token |
| **API Key** | `/api/v1/apikey` | **外部接口** | **API Key** |

### 认证方式

#### 方式一：Bearer Token（前端使用）

```bash
curl -H "Authorization: Bearer your_token" \
  https://your-domain/api/v1/users/me
```

#### 方式二：API Key（外部系统使用）

```bash
# 使用 X-API-Key header
curl -H "X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  https://your-domain/api/v1/apikey/info

# 或使用 Authorization header
curl -H "Authorization: Bearer key-xxxxxxxxxxxxxxxx-yyyyyyyy" \
  https://your-domain/api/v1/apikey/info
```

### 常用接口示例

<details>
<summary><b>用户注册</b></summary>

```http
POST /api/v1/users/register
Content-Type: application/json

{
    "telegram_id": 123456789,
    "username": "newuser",
    "reg_code": "code-xxxxx"
}
```
</details>

<details>
<summary><b>签到</b></summary>

```http
POST /api/v1/score/checkin
Authorization: Bearer xxx

# Response
{
    "success": true,
    "data": {
        "score": 15,
        "balance": 150,
        "streak": 7
    }
}
```
</details>

<details>
<summary><b>媒体搜索</b></summary>

```http
GET /api/v1/media/search?q=进击的巨人&source=all

# Response
{
    "success": true,
    "data": {
        "results": [
            {
                "id": 1429,
                "title": "进击的巨人",
                "source": "tmdb",
                "media_type": "tv"
            }
        ]
    }
}
```
</details>

<details>
<summary><b>API Key 接口 - 获取账号信息</b></summary>

```http
GET /api/v1/apikey/info
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy

# Response
{
    "success": true,
    "data": {
        "uid": 1,
        "username": "user123",
        "active": true,
        "is_expired": false,
        "days_left": 30,
        "score": 1000
    }
}
```
</details>

<details>
<summary><b>API Key 接口 - 签到</b></summary>

```http
POST /api/v1/apikey/score/checkin
X-API-Key: key-xxxxxxxxxxxxxxxx-yyyyyyyy

# Response
{
    "success": true,
    "data": {
        "score": 15,
        "balance": 1015,
        "streak": 8
    }
}
```
</details>

### API Key 专用接口

Twilight 提供了一套专门为外部系统设计的 API Key 接口，与前端使用的接口完全独立。

**主要功能**:
- 账号信息查询
- 账号状态查询
- 账号启用/禁用
- 账号续期
- API Key 管理（刷新、启用、禁用）
- Emby 状态查询和会话管理
- 积分查询、签到、历史记录、排行榜

**详细文档**: 请查看 [API Key 接口文档](docs/API_KEY_API.md)

---

## 🔌 Webhook 配置

### Bangumi 同步 - Emby

1. 进入 Emby 管理面板 → 通知 → 添加 Webhook
2. URL: `https://your-domain/api/v1/webhook/bangumi/emby`
3. 事件：勾选「播放-停止」

### Bangumi 同步 - Jellyfin

1. 安装 Webhook 插件
2. 添加 Generic Destination
3. URL: `https://your-domain/api/v1/webhook/bangumi/jellyfin`
4. 模板：
```json
{"media_type": "{{{ItemType}}}","title": "{{{SeriesName}}}","season": {{{SeasonNumber}}},"episode": {{{EpisodeNumber}}},"user_name": "{{{NotificationUsername}}}"}
```

---

## 🎨 Web 界面功能

### 用户功能
- **仪表盘** - 查看账号状态、积分、签到、观看统计
- **媒体搜索** - 搜索 TMDB 和 Bangumi 的媒体内容
- **积分中心** - 查看积分、签到、转账、历史记录
- **个人设置** - 管理账号信息、绑定 Telegram/Emby、API Key 管理

### 管理员功能
- **用户管理** - 查看、编辑、禁用、删除用户，批量操作
- **注册码管理** - 创建、查看、删除注册码
- **求片审核** - 审核用户的求片请求
- **数据统计** - 查看系统统计数据
- **安全管理** - 查看登录日志、设备管理、IP 黑名单
- **配置管理** - 在线编辑 `config.toml` 配置文件
- **API 测试** - 测试 API 接口，列出所有可用接口

---

## 📖 使用指南

### 首次使用

1. **配置 Emby**
   - 在 `config.toml` 中配置 Emby 服务器地址和 API Token
   - 如需 NSFW 功能，配置 NSFW 媒体库 ID

2. **创建管理员**
   - 在 `config.toml` 的 `[SAR]` 节中配置 `admin_uids` 或 `admin_usernames`
   - 重启服务后，配置的用户将自动成为管理员

3. **启动服务**
   - 启动后端：`python main.py`
   - 启动前端：`cd webui && npm run dev`
   - 访问 `http://localhost:3000`

4. **生成注册码**
   - 使用管理员账号登录 Web 界面
   - 进入「注册码」页面创建注册码

5. **用户注册**
   - 用户使用注册码在前端注册
   - 或通过 Telegram Bot 注册（如已启用）

### API Key 使用

1. **生成 API Key**
   - 登录 Web 界面
   - 进入「个人设置」→「API Key 管理」
   - 点击「生成 API Key」

2. **使用 API Key**
   - 在外部系统中使用 API Key 调用接口
   - 参考 [API Key 接口文档](docs/API_KEY_API.md)

---

## 🔧 高级配置

### Telegram Bot

```toml
[Telegram]
# Telegram Bot API 地址
telegram_api_url = "https://api.telegram.org/bot"
# Bot Token
bot_token = "your_bot_token"
# 管理员 ID
admin_id = [123456789]
# 群组 ID
group_id = [-1001234567890]
# 是否强制加入群组/频道
force_subscribe = false
```

### Webhook 推送

```toml
[Webhook]
# 是否启用 Webhook 功能
webhook_enabled = false
# Webhook 验证密钥
webhook_secret = "your_secret"
# 外部推送端点列表
webhook_endpoints = [
    "https://your-webhook-url.com/endpoint"
]
# 是否启用播放统计
playback_stats_enabled = true
```

### 安全配置

```toml
[Security]
# 是否启用 IP 限制
ip_limit_enabled = false
# 每个用户允许的最大 IP 数量
max_ips_per_user = 10
# 登录失败锁定阈值
login_fail_threshold = 5
# 锁定时间 (分钟)
lockout_minutes = 30
```

---

## 🐛 故障排除

### 常见问题

**Q: 无法连接 Emby 服务器**

A: 检查 `config.toml` 中的 `emby_url` 和 `emby_token` 是否正确，确保 Emby 服务器可访问。

**Q: Web 界面无法访问后端**

A: 检查 `NEXT_PUBLIC_API_URL` 环境变量是否正确，或检查 `next.config.mjs` 中的 rewrites 配置。

**Q: API Key 接口返回 401**

A: 检查 API Key 是否正确，是否已启用，账号是否被禁用。

**Q: 签到失败**

A: 检查用户是否已绑定 Emby 账号，账号是否已激活。

---

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

---

## 🙏 鸣谢

- [Emby](https://emby.media/) / [Jellyfin](https://jellyfin.org/) - 媒体服务器
- [TMDB](https://www.themoviedb.org/) - The Movie Database
- [Bangumi](https://bgm.tv/) - Bangumi番组计划
- [Telegram Bot API](https://core.telegram.org/bots/api) - 机器人 API
- [Next.js](https://nextjs.org/) - Next.js 前端框架
- [Telegram-Jellyfin-Bot](https://github.com/Prejudice-Studio/Telegram-Jellyfin-Bot) - 本组的前代管理器
- [Sakura_embyboss](https://github.com/berry8838/Sakura_embyboss) - 功能参考
- [Bangumi-syncer](https://github.com/SanaeMio/Bangumi-syncer) - Bangumi 同步参考

---

## 📄 许可证

本项目基于 [MIT License](LICENSE) 开源。

---

<div align="center">

**如果这个项目对你有帮助，请给一个 ⭐ Star！**

Made with ❤️ by [Prejudice Studio](https://github.com/Prejudice-Studio/)

</div>
