@echo off
echo ==========================================
echo    Twilight Starting...
echo ==========================================

:: 启动后端
echo Starting Backend (All Services)...
start "Twilight Backend" cmd /k "cd /d %~dp0 && .\\venv\\Scripts\\activate && python main.py all"

:: 等待 2 秒让后端初始化
timeout /t 2 /nobreak > nul

:: 启动前端
echo Starting Frontend...
start "Twilight Frontend" cmd /k "cd /d %~dp0webui && npm run dev"

echo ==========================================
echo    All services are launching!
echo    Backend: http://127.0.0.1:5000/api/v1/docs
echo    Frontend: http://localhost:3000
echo ==========================================
pause
