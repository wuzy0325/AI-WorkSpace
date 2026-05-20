# TS Reference Feature Map for Wind-DAQ Migration

> Scope: reference project at `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ`
>
> Target: rebuilt `projects/wind-daq`

This project is now treated as a clean rebuild, not a patch over the previous Go backend.

## Source Inventory

| Area | Reference source | Migration decision |
|---|---|---|
| TS backend device/acquisition/motion/calibration/traversal/storage services | `src/main/**` | Re-implement behavior in Go hexagonal layers |
| Reference Vue UI | `src/renderer/src/**` | Migrate/adapt into `apps/desktop-wails/frontend` |
| Reference Wails shell | `wails-backend/main.go` | Use only as desktop shell/window option reference |
| Deprecated Wails frontend | `wails-backend/frontend/src/**` | Do not migrate |
| Electron IPC | `src/main/ipc/**`, `window.electron` | Do not migrate; replace with HTTP/WS or Wails bindings |
| Generated Wails JS | `wails-backend/frontend/src/wailsjs/**` | Do not copy; regenerate in target |

## Target Architecture

- `services/api-go/internal/core`: pure domain types, formulas, interpolation, traversal calculations.
- `services/api-go/internal/usecase`: orchestration for devices, acquisition, motion, calibration, traversal, storage, reporting.
- `services/api-go/internal/ports`: interfaces only.
- `services/api-go/internal/adapters`: hardware, config, storage, report, WebSocket, db/mq adapters.
- `apps/desktop-wails/backend`: Wails app host and thin bindings only.
- `apps/desktop-wails/frontend`: Vue 3 operator UI, display and interaction only.

## First Rebuild Slice

1. Define Go domain contracts for device profiles, data payloads, motion axes, calibration configs, traversal configs, and storage settings.
2. Add ports for DAQ device, motion controller, publisher, recorder, scanner, config store, and event bus.
3. Add simulated DAQ and simulated motion adapters first to make the app testable without hardware.
4. Add acquisition hub with latest-data cache and publish-rate controlled event output.
5. Add REST/WS API or Wails bindings around usecases.
6. Create Vue/Wails frontend shell, design tokens, basic stores, and Dashboard/Motion/Calibration/Traversal placeholders.

## Do Not Carry Forward

- Existing deleted Go backend code unless reintroduced test-first from the reference behavior.
- Old Electron IPC route names as architecture.
- Node-only hardware transports in frontend/main-process form.
- Frontend calibration, traversal, or hardware algorithms.
