# Wind-DAQ

## Scope

Wind-DAQ is the wind tunnel DAQ desktop application rebuilt from the Cursor DAQ TS/Electron reference into this workspace's Go + Vue + Wails architecture.

The product goal is UI/workflow parity with Cursor DAQ plus a cleaner implementation architecture. The frontend should look and behave like the reference; backend business logic belongs in Go usecases/core behind typed API boundaries.

This project owns:

- DAQ device orchestration for wind tunnel/lab measurement workflows.
- Wails desktop shell and Vue 3 operator UI.
- Go backend usecases for acquisition, device management, motion, calibration, traversal, storage, and reporting.
- Project-specific API contracts, configs, deployment notes, and HIL/integration tests.

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js LTS
- Wails CLI v2.12.0+ (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Backend

```powershell
cd projects/wind-daq/services/api-go
go run ./cmd/server/main.go
```

Starts on `http://localhost:8080`. Default simulated device profile is created automatically.

### Frontend (Vite dev)

```powershell
cd projects/wind-daq/apps/desktop-wails/frontend
npm install
npm run dev -- --host 127.0.0.1
```

Starts on `http://127.0.0.1:5173`. Dev server proxies `/api` to backend.

### Wails desktop (Wind-DAQ 主应用)

```powershell
cd projects/wind-daq/apps/desktop-wails
wails dev
```

前端开发服务器: `http://localhost:5173`

### Wails desktop (五孔探针插值计算)

```powershell
cd projects/wind-daq/apps/five-hole-interpolator
wails dev
```

前端开发服务器: `http://localhost:5174`

> **注意**: 两个 Wails 应用使用不同的 Vite 端口（5173 / 5174），可同时 `wails dev` 启动互不干扰。

### Config

| Variable | Default | Description |
|---|---|---|
| `WIND_DAQ_ADDR` | `:8080` | Backend HTTP listen address |
| `WIND_DAQ_PROFILE_PATH` | `config/device-profiles.json` | Profile persistence file |

### Verification

1. Start backend and frontend
2. Open `http://127.0.0.1:5173`
3. Click "Connect + Start" to run simulated acquisition
4. See 4 channel cards updating with data
5. Click "Stop" to stop acquisition

## Folder Notes

```
wind-daq/
├── apps/desktop-wails/
│   ├── frontend/          # Vue 3 + Vite + Pinia + Vitest
│   │   ├── src/
│   │   │   ├── api/       # HTTP client + typed API modules
│   │   │   ├── components/
│   │   │   │   ├── device/    # DeviceManagementDrawer, DaqT1603Config, RecordingControl
│   │   │   │   ├── layout/    # AppShell, MainTopBar, AppRailNav, MainBottomBar
│   │   │   │   └── main/      # DeviceSidebar, DeviceDetailPanel
│   │   │   ├── stores/        # Pinia: device, theme, i18n, feedback
│   │   │   ├── styles/        # Tokens, themes, utility CSS
│   │   │   └── views/         # MainDashboardView, MotionView, CalibrationView, TraversalView, LogViewer
│   │   ├── src/__tests__/     # Vitest store tests
│   │   └── vite.config.ts     # Dev proxy to :8080
│   └── backend/           # Wails Go host (thin bindings only)
├── services/api-go/
│   ├── cmd/server/        # Entry point
│   ├── api/               # HTTP handlers (thin)
│   └── internal/
│       ├── core/          # Pure domain types
│       ├── ports/         # Interface definitions
│       ├── usecase/       # Orchestration
│       └── adapters/      # Hardware, config, storage implementations
├── contracts/             # OpenAPI spec
└── docs/
    └── migration/         # Migration entry, UI parity plan, feature map
```

Additional desktop app:

- `apps/five-hole-interpolator/` is a dedicated Wails desktop tool for five-hole probe PRB interpolation. Keep its Vue UI and Wails bindings in the app directory; keep reusable interpolation rules in `services/api-go/internal/core/interpolation` or a usecase layer when orchestration grows.

## Development Rules

- Follow workspace-level rules in `../../AGENTS.md`.
- Keep business rules in `services/api-go/internal/core`.
- Keep orchestration in `services/api-go/internal/usecase`.
- Keep interfaces in `services/api-go/internal/ports`.
- Keep hardware I/O in `services/api-go/internal/adapters/hardware` or reusable `shared/device-sdk` packages.

## Commands

```powershell
cd services/api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
go test ./internal/... ./api/...
```

```powershell
cd apps/desktop-wails/frontend
npm run typecheck
npm run build
npm run test        # Vitest unit tests
```

```powershell
cd apps/desktop-wails
wails build
```

## Migration Notes

Use `docs/migration/README.md` as the single migration documentation entry point.

Before implementing migrated functionality, read:

1. `docs/migration/README.md`
2. `docs/migration/ui-parity-plan.md`
3. `docs/migration/ts-reference-feature-map.md`
