#!/bin/sh
set -e

echo "=========================================="
echo "   Twilight Docker Starting..."
echo "=========================================="

# 运行数据库迁移
echo "Running database migration..."
python migrate.py 2>/dev/null || true

# 启动后端
echo "Starting Backend..."
uvicorn asgi:app --host 0.0.0.0 --port 5000 --workers "${WORKERS:-2}" &
BACKEND_PID=$!

# 等待后端就绪
sleep 3

# 启动前端（如果存在构建产物）
if [ -f /app/webui/server.js ]; then
    echo "Starting Frontend..."
    cd /app/webui
    HOSTNAME=0.0.0.0 PORT=3000 node server.js &
    FRONTEND_PID=$!
    cd /app
fi

echo "=========================================="
echo "   All services started!"
echo "   Backend:  http://0.0.0.0:5000/api/v1/docs"
if [ -f /app/webui/server.js ]; then
    echo "   Frontend: http://0.0.0.0:3000"
fi
echo "=========================================="

# 捕获退出信号
cleanup() {
    echo "Shutting down..."
    kill $BACKEND_PID 2>/dev/null
    [ -n "$FRONTEND_PID" ] && kill $FRONTEND_PID 2>/dev/null
    exit 0
}

trap cleanup INT TERM

# 等待子进程
wait
