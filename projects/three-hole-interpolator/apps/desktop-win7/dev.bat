@echo off
chcp 65001 >nul 2>&1
setlocal

set "ROOT_DIR=%~dp0"

echo 正在启动开发服务器...
echo.

echo [1/2] 启动Go后端 (端口18080)...
start "三孔探针插值计算-后端" cmd /k "cd /d "%ROOT_DIR%" && go run . -dev"

echo [2/2] 启动前端开发服务器...
cd /d "%ROOT_DIR%frontend"
call npm run dev

pause
