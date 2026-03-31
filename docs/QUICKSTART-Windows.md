# Windows 11 快速启动指南

欢迎使用 Twilight！本指南将帮助你在 Windows 11 上快速启动和运行项目。

## 前置需求

- Windows 11（或 Windows 10 版本 20H2+）
- Python 3.11+ （从 https://www.python.org 下载）
- Git（从 https://git-scm.com 下载）
- 文本编辑器（建议 VS Code）

## 5 分钟快速开始

### 第一步：克隆项目

```powershell
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight
```

### 第二步：自动设置（推荐）

```powershell
.\dev.ps1 -Task setup
```

这条命令会自动：
1. ✅ 创建虚拟环境
2. ✅ 安装所有依赖
3. ✅ 格式化代码
4. ✅ 运行测试

### 第三步：配置应用

打开 `.env` 文件，编辑以下关键配置：

```bash
# 必需：Emby 服务器配置
TWILIGHT_EMBY_URL=http://127.0.0.1:8096/
TWILIGHT_EMBY_TOKEN=your_emby_api_token_here

# 可选：Redis（用于多进程部署）
TWILIGHT_REDIS_URL=redis://localhost:6379/0

# 可选：Telegram Bot
TWILIGHT_TELEGRAM_MODE=false
TWILIGHT_TELEGRAM_BOT_TOKEN=your_token_here
```

### 第四步：启动应用

```powershell
.\dev.ps1 -Task run
```

访问 API 文档：http://localhost:5000/api/v1/docs

## 常用命令

| 命令 | 说明 |
|------|------|
| `.\dev.ps1 -Task help` | 显示帮助信息 |
| `.\dev.ps1 -Task run` | 启动开发服务器 |
| `.\dev.ps1 -Task test` | 运行单元测试 |
| `.\dev.ps1 -Task lint` | 检查代码风格 |
| `.\dev.ps1 -Task format` | 格式化代码 |
| `.\dev.ps1 -Task clean` | 清理临时文件 |

## 手动设置（如脚本失败）

```powershell
# 1. 创建虚拟环境
python -m venv venv

# 2. 激活虚拟环境
.\venv\Scripts\Activate.ps1

# 3. 升级 pip
python -m pip install --upgrade pip

# 4. 安装依赖
pip install -r requirements.txt

# 5. 配置环境
Copy-Item .env.example .env
notepad .env  # 编辑配置

# 6. 启动应用
python main.py api --debug
```

## 常见问题

### Q: PowerShell 不允许执行脚本

**A**: 运行以下命令允许本地脚本执行：

```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### Q: Python 找不到

**A**: 确保 Python 已正确安装并添加到 PATH。验证：

```powershell
python --version
pip --version
```

### Q: 虚拟环境激活失败

**A**: 使用完整路径或 CMD：

```powershell
# PowerShell
& ".\venv\Scripts\Activate.ps1"

# 或使用 CMD
cmd /c "venv\Scripts\activate.bat"
```

### Q: Redis 连接失败

**A**: Redis 是可选的。应用会自动回退到内存存储。如需启用，使用 Docker：

```powershell
# 需要先安装 Docker Desktop
docker run -d -p 6379:6379 redis:latest
```

### Q: 端口 5000 已被占用

**A**: 修改 `.env` 文件中的 `TWILIGHT_API_PORT` 或使用其他端口：

```powershell
python main.py api --port 8080
```

## 下一步

- 📚 阅读 [完整安装指南](docs/INSTALL.md)
- 🔧 查看 [开发指南](docs/DEVELOPMENT.md)
- 🌐 浏览 [API 文档](docs/BACKEND_API.md)
- 💻 前往 [前端开发](webui/README.md)

## 获取帮助

- 📖 [项目文档](docs/)
- 🐛 [提交 Issue](https://github.com/Prejudice-Studio/Twilight/issues)
- 💬 [讨论](https://github.com/Prejudice-Studio/Twilight/discussions)

---

祝你使用愉快！🎉
