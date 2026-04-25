#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "=========================================="
echo "   Twilight Starting..."
echo "=========================================="

# 启动后端
echo "Starting Backend (All Services)..."
source ./.venv/bin/activate
python main.py all &
BACKEND_PID=$!

# 等待后端初始化
sleep 2

# 启动前端（生产模式）
echo "Starting Frontend..."
cd webui && pnpm start -p 3000 &
FRONTEND_PID=$!
cd "$SCRIPT_DIR"

# 等待前端启动
sleep 5

echo "=========================================="
echo "   All services are launching!"
echo "   Backend: http://127.0.0.1:5000/api/v1/docs"
echo "   Frontend: http://localhost:3000"
echo "=========================================="
echo "Press Ctrl+C to stop all services."

# 捕获退出信号，停止子进程
trap 'kill $BACKEND_PID $FRONTEND_PID 2>/dev/null; exit' INT TERM

wait
