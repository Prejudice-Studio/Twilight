# Twilight 安装部署指南

本文档详细说明如何在不同操作系统上安装和部署 Twilight 系统。

## 目录

- [环境要求](#环境要求)
- [Windows 11 安装](#windows-11-安装)
- [Linux 安装](#linux-安装)
- [Docker 部署](#docker-部署)
- [配置说明](#配置说明)
- [故障排除](#故障排除)

## 环境要求

### 最低要求

- **Python**: 3.10+
- **数据库**: SQLite（内置）或 PostgreSQL/MySQL（可选）
- **Redis**: 可选（用于分布式部署或会话存储）
- **内存**: 512MB+
- **磁盘**: 2GB+（包含依赖和数据库）

### 推荐配置

- **Python**: 3.11+
- **系统**: Linux (Ubuntu 22.04+) 或 Windows 11
- **内存**: 2GB+
- **Redis**: 可选但推荐用于生产环境

## Windows 11 安装

### 前置准备

1. **安装 Python**
   ```powershell
   # 从 https://www.python.org 下载 Python 3.11+ Windows 安装程序
   # 或使用包管理器
   winget install Python.Python.3.11
   
   # 验证安装
   python --version
   pip --version
   ```

2. **准备项目目录**
   ```powershell
   # 克隆或下载项目
   git clone https://github.com/Prejudice-Studio/Twilight.git
   cd Twilight
   ```

### 安装步骤

1. **创建虚拟环境**
   ```powershell
   # 创建虚拟环境
   python -m venv venv
   
   # 激活虚拟环境（PowerShell）
   .\venv\Scripts\Activate.ps1
   
   # 或使用 Command Prompt (cmd.exe)
   venv\Scripts\activate.bat
   ```

   **常见问题**: 如果出现执行策略错误，运行：
   ```powershell
   Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
   ```

2. **升级 pip 和工具**
   ```powershell
   python -m pip install --upgrade pip setuptools wheel
   ```

3. **安装依赖**
   ```powershell
   # 安装生产依赖
   pip install -r requirements.txt
   
   # （可选）安装开发工具
   pip install -r requirements-dev.txt
   ```

4. **配置环境变量**
   ```powershell
   # 复制 .env.example 为 .env
   Copy-Item .env.example .env
   
   # 编辑 .env 文件配置各项参数
   notepad .env
   ```

5. **初始化数据库**
   ```powershell
   # 数据库会在首次运行时自动创建
   # 升级版本后运行迁移脚本，添加新列：
   python migrate.py
   ```

### 运行应用

#### 开发模式

```powershell
# 运行 API 服务器（开发）
python main.py api --debug

# 运行 Telegram Bot（如果启用）
python main.py bot

# 运行定时任务
python main.py scheduler

# 同时运行所有服务
python main.py all
```

访问 API：`http://localhost:5000/api/v1/docs`

#### 生产模式

```powershell
# 使用 Uvicorn ASGI 服务器
pip install uvicorn

# 运行应用
uvicorn asgi:app --host 0.0.0.0 --port 5000 --workers 4

# 或部署为 Windows 服务...
```

### 测试

```powershell
# 运行单元测试
pytest tests/ -v

# 生成覆盖率报告
pytest tests/ --cov=src --cov-report=html
```

## Linux 安装

### Ubuntu 22.04+ 安装步骤

```bash
# 安装 Python 及依赖
sudo apt update
sudo apt install -y python3.11 python3.11-venv python3.11-dev
sudo apt install -y git redis-server

# 克隆项目
git clone https://github.com/Prejudice-Studio/Twilight.git
cd Twilight

# 创建虚拟环境
python3.11 -m venv venv
source venv/bin/activate

# 安装依赖
pip install -r requirements.txt

# 配置
cp .env.example .env
nano .env  # 编辑配置

# 测试运行
python main.py api
```

### 配置为系统服务

创建 `/etc/systemd/system/twilight.service`：

```ini
[Unit]
Description=Twilight Emby Management System
After=network.target redis-server.service

[Service]
Type=simple
User=twilight
WorkingDirectory=/opt/twilight
Environment="PATH=/opt/twilight/venv/bin"
ExecStart=/opt/twilight/venv/bin/uvicorn asgi:app --host 0.0.0.0 --port 5000
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl start twilight
sudo systemctl enable twilight
sudo systemctl status twilight
```

## Docker 部署

当前仓库暂未提供官方 `Dockerfile` 与 `docker-compose.yml`。

建议先使用本指南中的裸机部署流程完成验证，再根据你的环境自定义容器镜像。

## 配置说明

### config.toml

主要配置文件，支持以下部分：

```toml
[Global]
logging = true
log_level = 20
redis_url = "redis://localhost:6379/0"

[API]
host = "0.0.0.0"
port = 5000
debug = false
cors_enabled = true
cors_origins = ["http://localhost:3000", "https://example.com"]

[Emby]
emby_url = "http://127.0.0.1:8096/"
emby_token = "your_token_here"

[Telegram]
bot_token = "your_bot_token"
admin_id = [123456789]

[SAR]
score_name = "暮光币"
register_mode = true
user_limit = 200
```

### 环境变量

可通过 `.env` 文件或系统环境变量覆盖 `config.toml`：

```bash
# .env 文件示例
TWILIGHT_REDIS_URL=redis://localhost:6379/0
TWILIGHT_API_PORT=5000
TWILIGHT_EMBY_TOKEN=your_emby_token
```

**优先级**: 环境变量 > config.toml > 默认值

### Redis 配置

如需使用 Redis 用于会话存储和缓存：

```bash
# Windows 上安装 Redis (使用 Docker 或 WSL)
docker run -d -p 6379:6379 redis:latest

# 配置环境变量
TWILIGHT_REDIS_URL=redis://localhost:6379/0
```

## 故障排除

### Windows 常见问题

#### 1. PowerShell 执行策略错误

```
不能加载文件 ...\Activate.ps1，因为在此系统上禁止执行脚本
```

**解决**:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

#### 2. Redis 连接失败

如果没有安装 Redis，应用会自动回退到内存存储，但功能受限。建议：

- **开发环境**: 使用 Docker Desktop 运行 Redis
  ```powershell
  docker run -d -p 6379:6379 redis:latest
  ```

- **生产环境**: 部署新的 Redis 实例或使用云服务

#### 3. 端口占用

```powershell
# 检查端口占用情况
netstat -ano | findstr :5000

# 杀死占用进程（替换 PID）
taskkill /PID <PID> /F
```

### Linux 常见问题

#### 1. 权限不足

```bash
# 为用户添加 sudo 权限
sudo usermod -aG sudo username

# 或创建专用用户
sudo useradd -m -s /bin/bash twilight
```

#### 2. 端口权限

```bash
# Linux 下运行 1024 以下端口需要 root
# 建议使用 nginx 反向代理到 5000
```

## 更新和维护

```powershell
# 拉取最新代码
git pull

# 更新依赖
pip install -r requirements.txt --upgrade

# 运行数据库迁移（每次更新后）
python migrate.py

# 查看已安装的包版本
pip list
```

## 性能优化建议

1. **启用 Redis**：用于会话存储和缓存
2. **使用生产级 ASGI 服务器**：Uvicorn + Nginx
3. **启用日志**：`TWILIGHT_LOGGING=true`
4. **定期清理临时文件**：数据库 WAL 文件
5. **监控资源使用**：使用 `psutil` 或系统监控工具

## 获取帮助

- 查看日志：`logs/twilight.log`
- 提交 Issue：https://github.com/Prejudice-Studio/Twilight/issues
- 参与讨论：https://github.com/Prejudice-Studio/Twilight/discussions
- 代码仓库：https://github.com/Prejudice-Studio/Twilight
