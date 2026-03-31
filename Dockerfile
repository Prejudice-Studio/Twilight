# ========================
# Stage 1: 前端构建
# ========================
FROM node:20-alpine AS frontend-builder

WORKDIR /app/webui

# 安装 pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# 先复制包管理文件以利用 Docker 缓存
COPY webui/package.json webui/pnpm-lock.yaml webui/pnpm-workspace.yaml ./

RUN pnpm install --frozen-lockfile

# 复制前端源码
COPY webui/ ./

# 设置 API 地址（容器内后端地址）
ENV NEXT_PUBLIC_API_URL=http://localhost:5000

RUN pnpm build

# ========================
# Stage 2: Python 后端
# ========================
FROM python:3.11-slim AS backend

# 安装运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 先复制依赖文件以利用 Docker 缓存
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt && \
    pip install --no-cache-dir uvicorn gunicorn

# 复制后端源码
COPY main.py asgi.py migrate.py ./
COPY src/ ./src/

# 复制默认配置文件（运行时可通过挂载覆盖）
COPY config.toml ./config.toml

# 创建数据目录
RUN mkdir -p /app/db /app/uploads/avatars /app/uploads/backgrounds /app/logs

# 暴露端口
EXPOSE 5000

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:5000/api/v1/system/health || exit 1

# 默认启动命令：使用 uvicorn 运行 ASGI 应用
CMD ["uvicorn", "asgi:app", "--host", "0.0.0.0", "--port", "5000", "--workers", "2"]

# ========================
# Stage 3: 完整部署（后端 + 前端静态）
# ========================
FROM backend AS full

# 安装 Node.js 运行时（用于 Next.js standalone 模式）
RUN apt-get update && apt-get install -y --no-install-recommends \
    nodejs \
    && rm -rf /var/lib/apt/lists/*

# 复制前端构建产物
COPY --from=frontend-builder /app/webui/.next/standalone /app/webui/
COPY --from=frontend-builder /app/webui/.next/static /app/webui/.next/static
COPY --from=frontend-builder /app/webui/public /app/webui/public

# 复制启动脚本
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

EXPOSE 3000

ENTRYPOINT ["/app/docker-entrypoint.sh"]
