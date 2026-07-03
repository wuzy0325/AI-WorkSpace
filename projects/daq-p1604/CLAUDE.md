# Project Architecture (DAQ-P-1604)

This project is a standalone Wails desktop app. It uses a **single Go module** at `apps/desktop-wails` without a separate `services/api-go` module.

## Module Layout

```
apps/desktop-wails/      # Go module "daq-p1604"
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
cd projects/daq-p1604/apps/desktop-wails
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
Connect a DAQ-P-1604 device on the network, add a profile with its IP:port (default 9000) in the app.

### Wails desktop app build
```powershell
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 build
```

## Project-Specific Notes

- **设备时间戳固件 bug**：DAQ-P-1604 设备硬件时间戳 fractional 字段以 ~4348Hz 速率递增（应为 1000Hz），每累积约 232ms 跳跃 768ms 校正。CSV Timestamp 列已统一截断到秒级避免展示错误的时间细分。详见 `releases/0.2.2.md`。
- **设备协议**：使用 w1601 长度前缀模式，命令不得追加 `\r\n`。详见 `shared/device-sdk/docs/commands/daq-p-1604.md`。
- **单位映射**：CH17=Pa，CH18=°C，前端锁定不可改。详见 `shared/device-sdk/go/protocol/daq_p1604_unit.go`。
