# DAQ-T-1603

Standalone Wails desktop application for DAQ-T-1603 data acquisition, monitoring, and CSV recording.

## Architecture

This project intentionally uses a single Go module under `apps/desktop-wails` instead of a separate `services/api-go` module. It still follows the workspace hexagonal boundaries.

```text
apps/desktop-wails/
  core/               # pure domain types
  ports/              # interfaces only
  usecase/            # orchestration
  adapters/           # hardware, config, recording implementations
  backend/            # Wails binding layer only
  frontend/           # Vue 3 / Vite / Pinia / ECharts UI
```

See `CLAUDE.md` for the project-specific constraints.

## Modes

Real hardware mode only. Configure a DAQ-T-1603 profile in the app with the device IP and port.

## Commands

```powershell
cd projects/daq-t1603/apps/desktop-wails
go test ./...
go vet ./...
go build -buildvcs=false ./...
```

```powershell
cd projects/daq-t1603/apps/desktop-wails/frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

```powershell
cd projects/daq-t1603/apps/desktop-wails
wails generate module
wails build
```

## Shared Code

- `shared/device-sdk/go/daq` contains reusable DAQ domain and hardware SDK code.
- Project-specific recording, profile selection, and Wails UI wiring stay inside this project.

## Development Rules

- `core/` must stay free of hardware, file I/O, serial/network, and framework imports.
- `ports/` contains interfaces only.
- `usecase/` calls devices through ports.
- `backend/` is parameter conversion plus usecase calls only.
- `frontend/src/bridge/` is the only frontend layer allowed to import generated `wailsjs/` bindings.
