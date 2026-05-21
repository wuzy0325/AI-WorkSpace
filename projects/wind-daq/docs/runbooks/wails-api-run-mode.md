# Wails + Go API 运行模式

## 当前架构（生产模式）

```
┌─────────────────────────────────────────────────┐
│ Wails 桌面壳 (apps/desktop-wails)              │
│  ├── backend/app.go — 启动时内嵌 Go API server  │
│  │     apiserver.Start(ctx, ":8080") in goroutine│
│  ├── frontend/ — Vue 3, 通过 HTTP 调内嵌 API   │
│  │     dev 模式下通过 Vite proxy 走外部 API     │
│  └── 退出时 context cancel → API server 关闭    │
└─────────────────────────────────────────────────┘
```

## 运行方式

### 开发模式
```powershell
# 终端 1: 启动 Go API（单独运行，用于前端 dev）
cd projects\wind-daq\services\api-go
$env:WIND_DAQ_PORT="8080"
go run .\cmd\server\main.go

# 终端 2: 启动 Vite 前端（通过 proxy 走 :8080）
cd projects\wind-daq\apps\desktop-wails\frontend
npm run dev

# 终端 3: Wails dev（内嵌也启动 API，注意端口冲突）
cd projects\wind-daq\apps\desktop-wails
wails dev
```

### 生产模式
```powershell
cd projects\wind-daq\apps\desktop-wails
wails build -skipbindings
.\build\bin\wind-daq.exe
```

- 桌面壳启动时自动内嵌启动 Go API server（goroutine）
- 默认尝试 `:8080`，被占用时自动尝试 `:8081`、`:8082`、`:9090`、`:9091`
- 所有端口均被占用时，API 不启动，前端显示 offline 状态
- 应用关闭时 API 自动退出（context cancel）

## 端口冲突处理

| 情况 | 行为 |
|------|------|
| `:8080` 空闲 | 使用 `:8080` |
| `:8080` 被占用 | 依次尝试 `:8081`、`:8082`、`:9090`、`:9091` |
| 所有端口均被占用 | API 不启动，日志记录 warning，前端离线运行 |

前端通过 `try/catch` 处理 API 调用失败，显示错误提示而非白屏。

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `WIND_DAQ_PORT` | `8080` | 首选 API 端口（应用内嵌模式） |
| `WIND_DAQ_ADDR` | `:8080` | API 监听地址（独立运行模式） |
| `WIND_DAQ_PROFILE_PATH` | `config/device-profiles.json` | 设备配置文件路径 |
| `VITE_API_BASE` | `""` (dev proxy) | 前端 API 基础 URL |
