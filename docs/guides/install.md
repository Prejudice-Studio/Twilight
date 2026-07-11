# 安装部署

推荐生产部署方式为 Linux + systemd，并在前面放置 Nginx、Caddy 或其他反向代理负责 HTTPS。

## 构建

```bash
git clone <repo-url> twilight
cd twilight

go build -o bin/twilight ./cmd/twilight

cd webui
pnpm install
pnpm build
cd ..
```

## 配置

1. 准备 `config.toml`。
2. 将真实密钥放入 `config.local.toml` 或 `TWILIGHT_*` 环境变量。
3. 不要把 Token、密码、数据库凭据提交进 Git。
4. 简单部署可使用 JSON 状态文件；生产环境建议使用 PostgreSQL。

## systemd

unit 的执行文件必须指向 `bin/twilight`。

```ini
[Service]
WorkingDirectory=/opt/twilight
ExecStart=/opt/twilight/bin/twilight all
Restart=always
```

## 反向代理

- 在代理层终止 HTTPS。
- 只允许可信前端 Origin。
- 正确转发 `X-Forwarded-For` / `X-Forwarded-Proto`。

## 运维备份

至少备份：

- `config.toml`
- `config.local.toml`
- 上传目录
- JSON 状态文件或 PostgreSQL 数据库
- systemd unit 与反向代理配置
