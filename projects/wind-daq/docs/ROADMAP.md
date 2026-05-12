# Wind-DAQ Development Roadmap

## Project Overview

Wind-DAQ is a wind tunnel data acquisition system. The active architecture is:

- Desktop shell: Tauri 2
- Frontend: Vue 3 + Vite
- Backend: Rust service crate under `services/api-rs`
- Architecture: hexagonal (`core` -> `usecase` -> `ports` -> `adapters`)
- Communication: Tauri commands, REST, or WebSocket depending on feature needs

The previous desktop/backend implementation has been removed from the active project structure.

## Current State

| Area | Path | Status |
|---|---|---|
| Tauri desktop shell | `apps/desktop-tauri/src-tauri/` | Placeholder created |
| Vue frontend | `apps/desktop-tauri/frontend/` | Placeholder created |
| Rust backend crate | `services/api-rs/` | Placeholder created |
| Domain core | `services/api-rs/src/core/` | Placeholder created |
| Ports | `services/api-rs/src/ports/` | Placeholder created |
| Usecases | `services/api-rs/src/usecase/` | Placeholder created |
| Hardware adapters | `services/api-rs/src/adapters/hardware/` | Placeholder created |
| Contracts | `contracts/` | Existing contract location retained |
| Project structure docs | `docs/STRUCTURE.md` | Updated for Rust + Tauri |

## Architecture Direction

```
Tauri Desktop App
  ├─ Vue 3 frontend
  └─ src-tauri thin shell / command bridge
       ↓
Rust backend service
  ├─ usecase
  ├─ core + ports
  └─ adapters
       ↓
Hardware / DB / files / messages
```

Rules:

- Tauri is a thin shell. It does not implement acquisition, calibration, motion control, or persistence rules.
- Vue owns UI state and rendering only. It does not access hardware or calibration algorithms directly.
- Rust backend owns device communication, data processing, calibration algorithms, state machines, and persistence workflows.
- Device-specific protocol code belongs in `services/api-rs/src/adapters/hardware` or reusable `shared/device-sdk`.

## Next Milestones

### Phase 1: Rust Backend Foundation

- [ ] Define core domain models for device, motion, acquisition, calibration, traversal, and storage.
- [ ] Define `ports` traits for devices, motion controllers, data sinks, config repositories, event streams, and scanners.
- [ ] Port pure algorithms from legacy implementations into `core`.
- [ ] Add unit tests for pure formulas and state transitions.

### Phase 2: Adapter Implementation

- [ ] Implement simulated DAQ device and simulated motion controller.
- [ ] Implement hardware adapters for DAQ-P1604 and DAQ-T1603.
- [ ] Add config, storage, scan, and report adapters.
- [ ] Add timeouts, retries, and explicit error mapping for all device I/O.

### Phase 3: API and Realtime Channel

- [ ] Decide per workflow whether it uses Tauri commands, REST, or WebSocket.
- [ ] Expose device, acquisition, motion, calibration, traversal, storage, report, and scan usecases.
- [ ] Define realtime event channel names and payload schemas.
- [ ] Keep transport handlers thin: validate input, call usecase, map response.

### Phase 4: Desktop Frontend

- [ ] Implement Vue app shell following `DESIGN.md`.
- [ ] Add API/WebSocket clients under `frontend/src/api`.
- [ ] Build feature modules for Live Acquisition, Device Manager, Motion Control, Calibration, Storage Settings, Report View, and Settings.
- [ ] Keep calibration formulas and hardware behavior out of Vue components and composables.

### Phase 5: Packaging and Verification

- [ ] Configure Tauri build for Windows desktop packaging.
- [ ] Add integration tests under `tests/integration`.
- [ ] Add HIL tests under `tests/hil`.
- [ ] Run structure validation before merging architecture changes.

## Development Commands

```powershell
# Structure validation
powershell -File .\scripts\validate-structure.ps1

# Rust workspace checks, once Rust is installed
cargo fmt --all -- --check
cargo check --workspace

# Desktop app, from project desktop directory
Set-Location .\projects\wind-daq\apps\desktop-tauri
npm run dev
npm run tauri
```

## Reference Documents

| File | Purpose |
|---|---|
| `docs/STRUCTURE.md` | Wind-DAQ project structure and boundaries |
| `../../CLAUDE.md` | Workspace architecture rules and decision tree |
| `../../docs/architecture/module-design.md` | Rust backend + Vue/Tauri module design |
| `../../docs/runbooks/development-rules.md` | Development rules and verification loop |
