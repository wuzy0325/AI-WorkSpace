# Project Architecture (WISPA)

This project is a standalone Wails desktop app. It uses a **single Go module** at `apps/desktop-wails` without a separate `services/api-go` module.

## Module Layout

```
apps/desktop-wails/      # Go module "wispa"
├── core/                # Pure domain types (zero imports: hardware, I/O, framework)
├── ports/               # Interface definitions only
├── usecase/             # Orchestration (depends on core + ports only)
├── adapters/
│   ├── hardware/        # P1604 device adapter + simulated adapter
│   ├── config/          # JSON profile persistence
│   ├── recording/       # CSV data recording
│   └── logging/         # File-based structured logging
├── backend/             # Thin Wails binding layer (parameter conversion only)
├── frontend/            # Vue 3 / TypeScript / Pinia / ECharts
│   └── src/
│       ├── bridge/      # ONLY place that imports generated wailsjs/
│       ├── stores/      # Pinia (calls bridge, not wailsjs directly)
│       ├── components/  # Vue components
│       └── views/       # Page-level compositions
└── main.go              # Dependency injection + Wails.Run
```

## Hard Constraints

| Location | Constraint |
|---|---|
| `core/` | zero hardware, zero file I/O, zero serial/network, zero framework imports |
| `ports/` | interface definitions only |
| `usecase/` | zero direct hardware calls — go through `ports` interfaces |
| `backend/` | zero business logic — parameter conversion + usecase calls only |
| `frontend/` | zero direct hardware access, zero backend business logic |
| `frontend/src/bridge/` | only layer allowed to import from `wailsjs/` |

## Commands

```powershell
# Working directory for all Go commands
cd projects/wispa/apps/desktop-wails
$env:GOWORK="off"

# Run all Go tests
go test ./...

# Run Go vet
go vet ./...

# Build Go code (no CGO required)
go build -buildvcs=false ./...

# Frontend commands
cd frontend
npm install --no-audit --no-fund   # Install dependencies
npm run dev                         # Vite dev server
npm run build                       # Production build
npm run typecheck                   # TypeScript check only
cd ../../..
```

## Usage

### Real hardware
Connect a WISPA device on the network, add a profile with its IP:port (default 9000) in the app.

### Wails desktop app build
```powershell
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 build
```

## Project-Specific Notes

- **设备时间戳固件 bug**：WISPA 设备硬件时间戳 fractional 字段以 ~4348Hz 速率递增（应为 1000Hz），每累积约 232ms 跳跃 768ms 校正。CSV Timestamp 列已统一截断到秒级避免展示错误的时间细分。详见 `releases/0.2.2.md`。
- **设备协议**：使用 w1601 长度前缀模式，命令不得追加 `\r\n`。详见 `shared/device-sdk/docs/commands/WISPA.md`。
- **单位映射**：CH17=Pa，CH18=°C，前端锁定不可改。详见 `shared/device-sdk/go/protocol/wispa_unit.go`。

## Win7 LTS Branch (lts/win7)

Win7 兼容版本将桌面壳从 Wails v3 + WebView2 替换为 **Go 1.20.14 + Electron 22.3.27 + net/http**，业务层零改动。

### 架构变化

```
apps/
├── desktop-wails/          # Go 后端（仍是单模块，但移除 Wails 依赖）
│   ├── core/               # + eventbus.go (EventBus 接口) + hub.go (状态容器)
│   ├── backend/            # app.go 改为 hub 模式，移除 application.App 依赖
│   ├── httpserver/         # 新增：HTTP handler + WebSocket hub
│   │   ├── register.go     # 路由注册
│   │   ├── device_handler.go
│   │   ├── recording_handler.go
│   │   ├── log_handler.go
│   │   ├── ws_hub.go       # WebSocket 事件总线（实现 core.EventBus）
│   │   └── helpers.go      # 统一响应信封 apiOK/apiErr
│   ├── frontend/           # 前端 bridge 改为 fetch + WebSocket
│   │   └── src/bridge/
│   │       ├── httpClient.ts   # 新增：统一响应信封解包
│   │       ├── wsClient.ts     # 新增：WebSocket 单例 + 指数退避重连
│   │       ├── deviceBridge.ts # 改写：HTTP + WebSocket
│   │       ├── logBridge.ts    # 改写：HTTP
│   │       └── recordingBridge.ts # 改写：HTTP + WebSocket
│   └── main.go             # 改写：net/http server + embed.FS + 优雅关闭
└── desktop-electron/       # 新增：Electron 22.3.27 桌面壳
    ├── main.cjs            # 主进程：spawn Go 后端 + 创建 BrowserWindow + IPC
    ├── preload.cjs         # contextBridge 暴露 showOpenDialog
    ├── package.json        # electron-builder NSIS 打包配置
    └── scripts/
        └── build-backend.ps1  # Go 1.20.14 + npm build + go build
```

### 关键设计

- **EventBus 抽象**：`core.EventBus` 接口解耦事件推送，Win7 分支由 `httpserver.WSHub` 实现，原 Wails 版由 `application.App.Event.Emit` 实现。backend 层依赖抽象而非具体传输。
- **Hub 状态容器**：集中管理 ctx、relay 协程映射、LogEmitter、EventBus，避免 Service 间循环依赖。
- **WebSocket Hub 并发模型**：单 goroutine 串行处理 register/unregister/broadcast，每客户端独立 send channel（buffered 32）+ writePump goroutine。
- **多参数事件 wire 格式**：`daq:device-state` 是双参数事件 [id, state]，WSHub.Emit 当 data 长度 > 1 时打包为数组推送，前端 onmessage 解构数组。
- **统一响应信封**：`{ok:true, data}` / `{ok:false, error}` 便于前端 fetch 统一处理。
- **前端 bridge 层**：唯一允许 import 后端绑定的层，stores/components 通过 bridge 间接调用，HTTP 替换时上层无感。

### 端口与事件清单

- 监听端口：`127.0.0.1:18182`（与 wista 的 18181 区分）
- WebSocket 端点：`/ws`
- 事件清单（与主线 Wails 版一致）：
  - `daq:log` — 单参数 DeviceLogEvent
  - `daq:recording-status` — 单参数 RecordingSession
  - `daq:recording-warning` — 单参数 RecordingWarningEvent（多设备录制场景某台断连警告）
  - `daq:device-state` — 双参数 [id, state]（设备状态变更）
- HTTP 路由：详见 `apps/desktop-wails/httpserver/register.go`

### 构建命令

```powershell
cd projects/wispa/apps/desktop-electron
npm install                  # 安装 electron + electron-builder
npm run build:backend        # Go 1.20.14 + npm build + go build → backend/wispa-backend.exe
npm run dist:win7            # NSIS 打包 → dist/WISPA-Win7-Setup-0.3.0-win7.1-x64.exe
```

### 与 wista Win7 版的差异

- 端口 18182（wista 用 18181）
- 保留单体 App（wista 拆为 3 个 Service：device/log/recording），main.go 只调用一次 ServiceStartup/Shutdown
- 保留 simulated 模式（`WISPA_MODE=simulated`）用于无硬件场景开发
- 多了 `daq:recording-warning` 事件（多设备录制场景）
- 多了 `/api/device/latest-snapshots` + `/api/device/latest-snapshot/{id}` 端点（替代 daq:payload 事件，前端 500ms 轮询）
- 多了 `/api/recording/start-with-config` 端点（FileRotation + StopConditions）
