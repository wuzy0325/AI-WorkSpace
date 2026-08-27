#!/bin/bash
# 启动脚本 - 同时启动后端和前端

echo "正在启动 1604 统一校准系统..."
echo ""

# 启动后端
echo "[1/2] 启动 Go API 服务 (端口 8080)..."
cd "$(dirname "$0")"
go run ./cmd/server &
BACKEND_PID=$!
echo "后端进程 PID: $BACKEND_PID"

# 等待后端启动
sleep 2

# 启动前端
echo ""
echo "[2/2] 启动 Vue 前端服务 (端口 5173)..."
cd web
npm run dev &
FRONTEND_PID=$!
echo "前端进程 PID: $FRONTEND_PID"

echo ""
echo "=========================================="
echo "系统已启动！"
echo ""
echo "后端 API: http://localhost:8080"
echo "前端页面: http://localhost:5173"
echo ""
echo "按 Ctrl+C 停止服务"
echo "=========================================="

# 等待用户中断
trap "kill $BACKEND_PID $FRONTEND_PID; exit" INT
wait