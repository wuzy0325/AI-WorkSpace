# WindLabX4

## Scope

WindLabX4 is the wind tunnel DAQ desktop application rebuilt from the Cursor DAQ TS/Electron reference into this workspace's Go + Vue + Wails architecture.

The product goal is UI/workflow parity with Cursor DAQ plus a cleaner implementation architecture. The frontend should look and behave like the reference; backend business logic belongs in Go usecases/core behind typed API boundaries.

This project owns:

- DAQ device orchestration for wind tunnel/lab measurement workflows.
- Wails desktop shell and Vue 3 operator UI.
- Go backend usecases for acquisition, device management, motion, calibration, traversal, storage, and reporting.
- Project-specific API contracts, configs, deployment notes, and HIL/integration tests.

Reusable motion behavior is shared through `shared/motion-control/go` and low-level motion device code through `shared/device-sdk/go/motion`. WindLabX4 must not import `projects/motion-controller/*` packages.

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js LTS
- Wails v3 alpha.95 CLI (`go run github.com/wailsapp/wails/v3/cmd/wails3 ...` or an installed `wails3` wrapper)

### Backend

```powershell
cd projects/WindLabX4/services/api-go
go run ./cmd/server/main.go
```

Starts on `http://localhost:8080`. Default simulated device profile is created automatically.

### Frontend (Vite dev)

```powershell
cd projects/WindLabX4/apps/desktop-wails/frontend
npm install
npm run dev -- --host 127.0.0.1
```

Starts on `http://127.0.0.1:5173`. Dev server proxies `/api` to backend.

### Wails desktop (WindLabX4 主应用)

```powershell
cd projects/WindLabX4/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

前端开发服务器: `http://localhost:5173`

### Five-Hole Interpolator

```powershell
cd projects/five-hole-interpolator/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

The five-hole interpolator is now an independent project. WindLabX4 and the standalone app both use `shared/algorithms/go/fivehole` for interpolation.

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
WindLabX4/
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

Related desktop app:

- `../five-hole-interpolator/apps/desktop-wails/` is the standalone Wails tool for five-hole probe PRB interpolation. Reusable interpolation rules live in `../../shared/algorithms/go/fivehole`.

## Development Rules

- Follow workspace-level rules in `../../AGENTS.md`.
- Keep business rules in `services/api-go/internal/core`.
- Keep orchestration in `services/api-go/internal/usecase`.
- Keep interfaces in `services/api-go/internal/ports`.
- Keep hardware I/O in `services/api-go/internal/adapters/hardware` or reusable `shared/device-sdk` packages.

## Commands

### 后端开发

```powershell
cd services/api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
go test ./internal/... ./api/...
```

### 前端开发

```powershell
cd apps/desktop-wails/frontend
npm run typecheck
npm run build
npm run test        # Vitest unit tests
```

### 桌面应用开发（wails3 dev 热更新）

```powershell
cd apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

### 对外交付打包

```powershell
cd apps/desktop-wails
task release        # 清理旧产物 → 构建前端 → 生产模式 Go 二进制
# 然后使用 NSIS 打包安装包
```

生产构建使用 `-tags production -trimpath -ldflags="-w -s -H windowsgui"`，
详见 `docs/decisions/ADR-004-wails-v3-production-build.md`。

## Migration Notes

Use `docs/migration/README.md` as the single migration documentation entry point.

Before implementing migrated functionality, read:

1. `docs/migration/README.md`
2. `docs/migration/ui-parity-plan.md`
3. `docs/migration/ts-reference-feature-map.md`
