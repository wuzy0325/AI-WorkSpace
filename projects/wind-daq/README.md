# Wind-DAQ

## Scope

Wind-DAQ is the wind tunnel DAQ desktop application rebuilt from the TS/Electron reference into this workspace's Go + Vue + Wails architecture.

This project owns:

- DAQ device orchestration for wind tunnel/lab measurement workflows.
- Wails desktop shell and Vue 3 operator UI.
- Go backend usecases for acquisition, device management, motion, calibration, traversal, storage, and reporting.
- Project-specific API contracts, configs, deployment notes, and HIL/integration tests.

Reusable device protocol primitives or shared algorithms that can serve multiple projects belong under `shared/`.

## Folder Notes

- `apps/desktop-wails/frontend`: Vue 3 desktop frontend.
- `apps/desktop-wails/backend`: Wails Go app host and bindings.
- `services/api-go`: Go backend service.
- `contracts`: API/protocol contracts.
- `tests/hil`: hardware-in-loop tests.

## Development Rules

- Follow workspace-level rules in `../../AGENTS.md`.
- Keep business rules in `services/api-go/internal/core`.
- Keep orchestration in `services/api-go/internal/usecase`.
- Keep interfaces in `services/api-go/internal/ports`.
- Keep hardware I/O in `services/api-go/internal/adapters/hardware` or reusable `shared/device-sdk` packages.

## Migration Notes

See `docs/migration/ts-reference-feature-map.md` before implementing migrated functionality.
