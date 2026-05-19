# Twilight 安装部署指南

本文档详细说明如何安装和部署 Twilight 系统。

> **前置阅读**
> 作者默认你具备一定的 Linux 与 Python 基础。如果完全不熟悉，建议先了解相关基础再尝试部署。

> **关于 Docker**
> 作者本人不喜欢 Docker，因此 **不提供** Docker 部署方案。
> 如果你需要 Docker，请自行实现；请勿将"没有 Docker 方案"作为 Issue 提交，否则会被直接关闭。

## 目录

- [Twilight 安装部署指南](#twilight-安装部署指南)
  - [目录](#目录)
  - [环境要求](#环境要求)
    - [最低要求](#最低要求)
    - [推荐配置](#推荐配置)
    - [前置说明](#前置说明)
  - [部署步骤](#部署步骤)
    - [前端](#前端)
    - [后端](#后端)
  - [获取帮助](#获取帮助)

## 环境要求

### 最低要求

- **Python**：3.10+
- **数据库**：SQLite（内置）或 PostgreSQL / MySQL（可选）
- **Redis**：可选（用于分布式部署或会话存储）
- **内存**：512 MB+
- **磁盘**：2 GB+（含依赖与数据库）

### 推荐配置

- **Python**：3.11+，搭配 [`uv`](https://github.com/astral-sh/uv) 管理依赖
- **系统**：Linux（Ubuntu 22.04+）或 Windows 11
- **内存**：2 GB+
- **Redis**：按需

### 前置说明

- 不推荐安装在 Windows 设备上
- 前后端分离部署

## 部署步骤

### 前端

1. 登录 Cloudflare 与 GitHub。
2. 打开 <https://github.com/Prejudice-Studio/Twilight> 并 Fork 到自己的账号下。
3. 进入 Cloudflare 主页 → **构建 → 计算 → Workers 和 Pages**。
4. 选择 **创建应用程序 → Continue with GitHub** 并登录 GitHub。
5. 选择 Twilight 项目；**项目名称必须为 `twilight-webui`**，否则会出现问题。
6. 填写以下配置：

   - 构建命令：`pnpm opennextjs-cloudflare build`
   - 部署命令：`pnpm opennextjs-cloudflare deploy`
   - 版本命令：`pnpm opennextjs-cloudflare upload`
   - 高级设置：
     - 路径：`/webui`
     - 环境变量：`NEXT_PUBLIC_API_URL` = `你的后端地址`

7. 保存并部署，等待完成。
8. 可选操作：
   - 把 Pages 绑定到自己 Cloudflare 托管的域名。
   - 在 Pages 的 **构建 → 部署挂钩** 添加 GitHub 仓库，实现自动部署（不放心可不加，手动部署即可）。

### 后端

1. 安装 Python / uv 环境。
2. 克隆仓库并进入目录：

   ```bash
   git clone https://github.com/Prejudice-Studio/Twilight.git
   cd Twilight
   ```

3. 创建并激活 Python / uv 虚拟环境。
4. 安装依赖：

   ```bash
   pip install -r requirements.txt
   # 或
   uv pip install -r requirements.txt
   ```

5. 复制配置模板并编辑：

   ```bash
   cp config.production.toml config.toml
   # 编辑 config.toml，按真实环境填写
   ```

6. 给一键启动脚本执行权限：

   ```bash
   chmod +x start_backend_prod.sh
   ```

7. 执行脚本启动后端。默认端口 `5000`。
8. 在 Cloudflare 中为后端添加 DNS 记录指向你的服务器 IP，确保前端配置的 `NEXT_PUBLIC_API_URL` 与后端地址一致。
9. 在 `config.toml` 中配置好跨源域名（`[API].cors_origins`）。
10. 访问前端并注册账号；在 `config.toml` 中把 `[SAR].admin_uids` 改为你的账号 UID，重启后端后即成为管理员。

> **注意**
> 目前在管理后台保存配置后，程序会自行关闭并 **不会自动重启**。
> 请配合 systemd / docker / supervisor 等具备自动拉起能力的方式启动，
> 或等待后续版本完善。

### 可选环境变量

| 变量 | 用途 | 默认 |
| ---- | ---- | ---- |
| `TWILIGHT_CONFIG_FILE` | 主配置文件路径 | `<项目根>/config.toml` |
| `TWILIGHT_CONFIG_LOCAL_FILE` | 本地覆盖配置（合并到主配置之上，已 `.gitignore`） | `<项目根>/config.local.toml` |
| `TWILIGHT_CONFIG_BACKUP_RETENTION` | `config_backups/` 目录最多保留多少份历史 `.bak`；`<=0` 表示不裁剪 | `20` |

> 启动期会自动迁移老 section、删孤立键、补缺失默认；任何变更前都会先备份到
> `config_backups/`（权限 `0o600`），见 [SECURITY.md §8](./SECURITY.md#8-配置文件自动备份)。

## 获取帮助

- 查看日志：`logs/twilight.log`
- 提交 Issue：<https://github.com/Prejudice-Studio/Twilight/issues>
- 参与讨论：<https://github.com/Prejudice-Studio/Twilight/discussions>
- 代码仓库：<https://github.com/Prejudice-Studio/Twilight>
