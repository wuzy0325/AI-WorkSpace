# Project Architecture (DAQ-T-1603)

This project is a standalone Wails desktop app. Unlike other projects in this workspace, it uses a **single Go module** at `apps/desktop-wails` without a separate `services/api-go` module.

## Module Layout

```
apps/desktop-wails/      # Go module "daq-t1603"
├── core/                # Pure domain types (zero imports: hardware, I/O, framework)
├── ports/               # Interface definitions only
├── usecase/             # Orchestration (depends on core + ports only)
├── adapters/
│   ├── hardware/        # T1603 device adapter
│   ├── config/          # JSON profile persistence
│   └── recording/       # CSV data recording
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
cd projects/daq-t1603/apps/desktop-wails

# Run all Go tests
go test ./...

# Run Go vet
go vet ./...

# Build Go code (no CGO required)
go build -buildvcs=false ./...

# Frontend commands
cd frontend
npm install --no-audit --no-fund   # Install dependencies
npm run dev                         # Vite dev server (port 5173)
npm run build                       # Production build
npm run typecheck                   # TypeScript check only
cd ../../..
```

## Usage

### Real hardware
Connect a DAQ-T-1603 device on the network, add a profile with its IP:port in the app.

### Wails desktop app build
```powershell
# Requires CGO and Wails CLI v3
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 build
```
