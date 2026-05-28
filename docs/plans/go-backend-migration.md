# Migration Plan: Go Backend → Hexagonal Architecture

**Status:** Completed
**Date:** 2026-04-23
**Source:** `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\backend-go\`
**Target:** `projects/wind-daq/services/api-go/`

## 1. Current State Analysis

Current module: `github.com/daq-app/backend-go`, flat package structure under `internal/`.

### Package Dependency Graph

```
cmd/main.go
├── api/ (server + handler/* + ws/*)
├── internal/config/
├── internal/acquisition/ ← depends on: api/ws, internal/driver
├── internal/device/      ← depends on: api/ws, internal/config, internal/driver
├── internal/motion/      ← depends on: api/ws, pkg/ffi, pkg/serialport
├── internal/calibration/ ← depends on: api/ws, internal/device, internal/motion
├── internal/traversal/   ← depends on: api/ws, internal/device, internal/motion
├── internal/storage/     ← depends on: internal/driver
├── internal/report/      ← depends on: nothing (stdlib only)
├── internal/interpolation/ ← depends on: nothing (pure math, NOT WIRED)
├── internal/scan/        ← depends on: internal/driver
└── pkg/ (protocol, serialport, ffi)
```

### Key Problems

1. **No separation between domain logic and infrastructure** — `device.Manager` directly imports `api/ws`
2. **Interfaces mixed with implementations** — `internal/driver/` has both `Device` interface AND concrete drivers
3. **`internal/runtime/` is dead code** — defined but never wired
4. **`internal/interpolation/` is dead code** — defined but never imported
5. **Business logic depends on WebSocket** — calibration, traversal, acquisition all import `api/ws`

## 2. Target Architecture

```
projects/wind-daq/services/api-go/
├── cmd/
│   └── server/main.go              # Entry point, DI wiring
├── internal/
│   ├── core/                       # Pure domain logic (zero external deps)
│   │   ├── device/                 # Device domain types
│   │   │   └── types.go            # DeviceType, DataPayload, ChannelConfig, etc.
│   │   ├── motion/                 # Motion domain types
│   │   │   └── types.go            # AxisName, MotionControllerType, AxisConfig, etc.
│   │   ├── calibration/            # Calibration domain
│   │   │   ├── types.go            # CalibrationType, CalibrationPoint, Coefficients, etc.
│   │   │   └── formulas.go         # Pure math (CalculateFiveHoleCoefficients, etc.)
│   │   ├── interpolation/          # Interpolation domain (pure math)
│   │   │   ├── prb_interpolator.go
│   │   │   └── multi_prb_interpolator.go
│   │   └── traversal/              # Traversal domain types
│   │       └── types.go            # TraversalConfig, TraversalProgress, etc.
│   ├── ports/                      # Interface definitions (zero implementations)
│   │   ├── device.go               # Device, ChannelUpdatable, CommandSendable
│   │   ├── motion.go               # MotionController interface
│   │   ├── data_sink.go            # DataSink, DataPublisher
│   │   ├── event_bus.go            # EventBus (publish/subscribe)
│   │   ├── config_repo.go          # ConfigRepo (load/save config)
│   │   └── scan.go                 # DeviceScanner interface
│   ├── usecase/                    # Orchestration (depends on core + ports only)
│   │   ├── device_manager.go       # Device lifecycle orchestration
│   │   ├── motion_manager.go       # Motion lifecycle orchestration
│   │   ├── acquisition.go          # Acquisition hub orchestration
│   │   ├── calibration.go          # Calibration workflow
│   │   ├── traversal.go            # Traversal workflow
│   │   ├── storage.go              # Recording start/stop
│   │   └── scan.go                 # Device discovery orchestration
│   └── adapters/                   # Concrete implementations
│       ├── hardware/               # Device & motion drivers
│       │   ├── base_device.go      # Shared base struct
│       │   ├── factory.go          # Device creation factory
│       │   ├── command_protocol.go # Command-response queue
│       │   ├── daq_p1604.go        # DAQ-P-1604 pressure scanner
│       │   ├── daq_t1603.go        # DAQ-T-1603 thermocouple reader
│       │   ├── wtn_pxi.go          # WTN PXI (future)
│       │   ├── simulated.go        # Simulated DAQ device
│       │   ├── b140.go             # B140 4-axis controller
│       │   ├── wtnmc4a.go          # WTNMC4A controller
│       │   ├── simulated_motion.go # Simulated motion controller
│       │   └── axis_helpers.go     # Motion axis utilities
│       ├── config/                 # File-based JSON config
│       │   └── manager.go
│       ├── storage/                # CSV file recording
│       │   └── service.go
│       ├── report/                 # Report generation
│       │   └── service.go
│       ├── scan/                   # Network device scanning
│       │   ├── scanner.go
│       │   ├── scan_service.go
│       │   ├── udp_scanner.go
│       │   ├── daq_p1604_scanner.go
│       │   └── daq_t1603_scanner.go
│       └── ws/                     # WebSocket transport
│           ├── hub.go
│           ├── client.go
│           └── channels.go
├── api/                            # HTTP + WS server (thin routing layer)
│   ├── server.go
│   ├── middleware.go
│   └── handler/
│       ├── app.go
│       ├── device.go
│       ├── daq.go
│       ├── motion.go
│       ├── calibration.go
│       ├── traversal.go
│       ├── storage.go
│       ├── report.go
│       └── scan.go
├── go.mod
├── go.sum
└── Makefile

shared/device-sdk/go/              # Cross-project reusable device primitives
├── go.mod
├── protocol/
│   ├── daq_p1604_frame.go
│   └── daq_t1603_frame.go
├── serialport/
│   └── port.go
└── ffi/
    ├── wtnmc4a.go
    └── wtnmc4a_stub.go
```

## 3. Migration Steps

### Phase 1: Scaffold & Module Setup
- [x] 1.1 Create Go module at `projects/wind-daq/services/api-go/`
- [x] 1.2 Create directory structure (core/*, ports/*, usecase/*, adapters/*, api/*)
- [x] 1.3 Create shared/device-sdk/go module
- [x] 1.4 Create `go.work` at workspace root
- [x] 1.5 Copy go.mod dependencies, run `go mod tidy`

### Phase 2: Core Layer (Pure Domain)
- [x] 2.1 Move domain types from `internal/driver/device.go` → `core/device/types.go`
- [x] 2.2 Move domain types from `internal/motion/types.go` → `core/motion/types.go`
- [x] 2.3 Move `internal/calibration/types.go` → `core/calibration/types.go`
- [x] 2.4 Move `internal/calibration/formulas.go` → `core/calibration/formulas.go` (pure math, verify zero imports)
- [x] 2.5 Extract reusable five-hole interpolation algorithms to `shared/algorithms/go/fivehole`
- [x] 2.6 Extract traversal domain types from `internal/traversal/service.go` → `core/traversal/types.go`
- [x] 2.7 Verify: `go build ./internal/core/...` passes, zero external imports

### Phase 3: Ports Layer (Interfaces)
- [x] 3.1 Extract `Device`, `ChannelUpdatable`, `CommandSendable` from `internal/driver/device.go` → `ports/device.go`
- [x] 3.2 Extract `MotionController` from `internal/motion/types.go` → `ports/motion.go`
- [x] 3.3 Define `DataSink` and `DataPublisher` in `ports/data_sink.go`
- [x] 3.4 Define `EventBus` in `ports/event_bus.go` (from runtime/event_bus.go, cleaned up)
- [x] 3.5 Define `ConfigRepo` in `ports/config_repo.go` (abstracting config.Manager)
- [x] 3.6 Define `DeviceScanner` in `ports/scan.go` (from scan/scan_service.go)
- [x] 3.7 Verify: `go build ./internal/ports/...` passes, zero implementations

### Phase 4: Shared Device SDK
- [x] 4.1 Create `shared/device-sdk/go/go.mod` as separate module
- [x] 4.2 Move `pkg/protocol/*` → `shared/device-sdk/go/protocol/*`
- [x] 4.3 Move `pkg/serialport/*` → `shared/device-sdk/go/serialport/*`
- [x] 4.4 Move `pkg/ffi/*` → `shared/device-sdk/go/ffi/*`
- [x] 4.5 Verify: `go build` in shared module passes

### Phase 5: Adapters Layer (Implementations)
- [x] 5.1 Move driver implementations → `adapters/hardware/` (daq_p1604, daq_t1603, simulated, base_device, factory, command_protocol)
- [x] 5.2 Move motion implementations → `adapters/hardware/` (b140, wtnmc4a, simulated_motion, axis_helpers)
- [x] 5.3 Move `internal/config/*` → `adapters/config/`
- [x] 5.4 Move `internal/storage/*` → `adapters/storage/`
- [x] 5.5 Move `internal/report/*` → `adapters/report/`
- [x] 5.6 Move `internal/scan/*` → `adapters/scan/`
- [x] 5.7 Move `api/ws/*` → `adapters/ws/`
- [x] 5.8 Update all imports in adapter files to use `ports/` interfaces instead of concrete types
- [x] 5.9 Verify: `go build ./internal/adapters/...` passes

### Phase 6: Usecase Layer (Refactor to Depend on Interfaces)
- [x] 6.1 Refactor `device.Manager` → `usecase/device_manager.go` (depend on ports.Device, ports.EventBus, ports.ConfigRepo)
- [x] 6.2 Refactor `motion.Manager` → `usecase/motion_manager.go` (depend on ports.MotionController, ports.EventBus)
- [x] 6.3 Refactor `acquisition.Hub` → `usecase/acquisition.go` (depend on ports.DataPublisher, ports.DataSink)
- [x] 6.4 Refactor `calibration.Service` → `usecase/calibration.go` (depend on ports.Device via usecase.DeviceManager, ports.MotionController via usecase.MotionManager, ports.EventBus)
- [x] 6.5 Refactor `traversal.Service` → `usecase/traversal.go` (same pattern as calibration)
- [x] 6.6 Create `usecase/storage.go` (thin wrapper over ports)
- [x] 6.7 Create `usecase/scan.go` (wrap ports.DeviceScanner)
- [x] 6.8 Verify: `go build ./internal/usecase/...` passes, no imports of adapters/*

### Phase 7: API Layer
- [x] 7.1 Move `api/server.go`, `api/middleware.go` (update imports)
- [x] 7.2 Move `api/handler/*` (update to call usecase layer, not internal packages directly)
- [x] 7.3 Verify: `go build ./api/...` passes

### Phase 8: Wire Everything
- [x] 8.1 Write `cmd/server/main.go` — DI container: create adapters, inject into usecases, pass to API
- [x] 8.2 Verify: `go build ./cmd/server/` produces binary
- [ ] 8.3 Run and test: binary starts, serves HTTP, WebSocket connects

### Phase 9: Cleanup & Verification
- [x] 9.1 Delete dead code: `internal/runtime/` — confirmed removed
- [x] 9.2 Delete old `pkg/protocol/`, `pkg/serialport/`, `pkg/ffi/` — confirmed removed (new `pkg/apiserver/` is intentional)
- [x] 9.3 Run `go vet ./...` — pass
- [x] 9.4 Run `scripts/validate-structure.ps1` — pass
- [x] 9.5 Update `workspace.structure.json` — pass
- [ ] 9.6 Commit with conventional commit message (pending user)

## 4. Module Names

| Module | Path | go.mod module name |
|---|---|---|
| API service | `projects/wind-daq/services/api-go/` | `wind-daq/services/api-go` |
| Shared device SDK | `shared/device-sdk/go/` | `shared/device-sdk/go` |

`go.work` at workspace root:

```go
go 1.21

use (
    projects/wind-daq/services/api-go
    shared/device-sdk/go
)
```

## 5. Key Refactoring Rules

1. **core/ must have zero imports from ports/, usecase/, adapters/, api/** — only stdlib and other core/ packages
2. **ports/ must have zero implementations** — interfaces and type definitions only
3. **usecase/ imports core/ and ports/ only** — never imports adapters/ or api/
4. **adapters/ import ports/ and core/ only** — implement ports interfaces
5. **api/ imports usecase/ only** — handlers call usecase methods
6. **cmd/ wires everything** — creates adapters, injects into usecases, passes to API

## 6. Risk & Mitigation

| Risk | Mitigation |
|---|---|
| Import cycle errors during refactoring | Build incrementally after each phase |
| Breaking WebSocket channel names (frontend depends on them) | Keep channel names identical in adapters/ws/channels.go |
| Config file format changes | Keep JSON format identical, just move code |
| WTNMC4A FFI only works on Windows | Keep build tags, test with `GOOS=windows` |
