# Module Design: Go Backend + Vue 3 Frontend (Wails Desktop App)

## 1. Go Backend Module Structure

The tree below is the preferred split-service layout for larger products such as `windlabx4` and `motion-controller`. Small standalone Wails apps may use the same packages directly under `apps/desktop-wails`; see `project-variants.md`.

Organize by responsibility domain. Each module is an independent Go package.

```
internal/
├── core/                    # Pure domain logic, no external dependencies
│   ├── acquisition/         # Acquisition domain: sample control, data buffer, trigger rules
│   ├── calibration/         # Calibration domain: algorithms, coefficient calculation, flow
│   ├── measurement/         # Measurement domain: unit conversion, correction, verdict
│   └── project/             # Project management: test config, condition management
│
├── usecase/                 # Orchestration layer, coordinates core + ports
│   ├── acquire_task.go      # Acquire use case: start/stop acquisition, real-time push
│   ├── calibrate_task.go    # Calibrate use case: run calibration flow, save results
│   ├── device_manager.go    # Device use case: connect/disconnect, status query
│   └── export_task.go       # Export use case: data query, file export
│
├── ports/                   # Interface definitions only
│   ├── device.go            # Device interface: Read, Write, Connect, Disconnect
│   ├── data_sink.go         # DataSink interface: store, query
│   ├── event_bus.go         # EventBus interface: publish/subscribe
│   └── config_repo.go       # ConfigRepo interface: read/write config
│
└── adapters/                # Concrete implementations
        ├── hardware/         # Device adapters: one file per device
        │   ├── daq_p1604.go
        │   ├── stepping_motor.go
        │   └── pressure_probe.go
        ├── db/               # Database adapters
        ├── mq/               # Message/event adapters
        └── config/           # Config file adapters
```

### Backend Rules

| Rule | Description |
|---|---|
| core splits by business domain | Each domain is independent. `acquisition` must not import `calibration`. |
| usecase splits by user operation | One file per complete operation (e.g. "run calibration"). |
| ports contains interfaces only | No implementations, no structs. Interface definitions only. |
| adapters split by technology | hardware for device drivers, db for persistence, one file per concrete implementation. |
| cross-domain coordination in usecase | Logic that touches both acquisition and calibration lives in usecase, never in core. |

## 2. Vue 3 Frontend Module Structure

Standard directory layout is defined in `docs/runbooks/frontend-directory-rules.zh-CN.md`. All existing projects already follow the de facto `views/` + `components/<domain>/` + `components/ui/` pattern.

```
frontend/src/
├── main.ts                 # Entry point
├── App.vue                 # Root component
├── pages/                  # Route-level page components (or views/)
├── components/
│   ├── ui/                 # Base UI primitives (UiButton, UiInput, UiPanel…)
│   ├── layout/             # Layout components (AppShell, MainTopBar…)
│   └── <domain>/           # Business domain components (device, calibration…)
│       ├── components/     # Domain-specific sub-components (optional)
│       ├── composables/    # Domain-specific composables (optional)
│       └── types.ts        # Domain-specific types (optional)
├── stores/                 # Pinia state
├── api/                    # HTTP/Wails API facade (or bridge/)
├── styles/
│   ├── tokens/             # Design tokens (color, spacing, typography…)
│   └── themes/             # Theme overrides (dark.css, light.css)
├── router/                 # Route config
├── composables/            # Cross-domain reusable composables
├── types/                  # Global TypeScript types
├── shared/                 # Cross-domain shared utilities and types
├── bridge/                 # Wails binding wrapper (alternative to api/)
├── core/                   # Framework-agnostic container
└── spikes/                 # Prototyping code (dev only)
```

### Frontend Rules

| Rule | Description |
|---|---|
| pages/ or views/ for route components | Page level only, no business logic. |
| components/ui/ for Ui* primitives | Business-agnostic, no store imports, no domain words. |
| components/<domain>/ for feature code | One directory per business domain. |
| stores/ for Pinia | Calls api/ or bridge/, not hardware or wailsjs/ directly. |
| api/ or bridge/ as single API facade | One per project, not both. |
| composable maps to usecase | One composable corresponds to one backend usecase. |
| no cross-domain imports | device does not import calibration components. Compose via pages/ or events. |
| styles/tokens/ is visual source of truth | Components use tokens, not raw visual values. |

## 3. Wails Binding Layer

```
backend/                     # Wails app shell
├── app.go                   # App struct, mounts all services
├── bindings/
│   ├── device.go            # Device* methods → usecase.DeviceManager
│   ├── acquisition.go       # Acquisition* methods → usecase.AcquireTask
│   └── calibration.go       # Calibration* methods → usecase.CalibrateTask
└── main.go
```

### Binding Rules

| Rule | Description |
|---|---|
| one binding file per domain | Methods map 1:1 to usecase. |
| binding methods only convert params and call usecase | No if/else, no business logic. |
| errors converted to frontend-friendly format | Never expose Go internal error details. |

## 4. Data Flow

```
User Action
    │
    ▼
Vue Component ──calls──▶ composable
                              │
                              ▼
                         bridge layer
                              │
                              ▼
                    Wails Binding (Go)
                              │
                              ▼
                         usecase
                        ╱       ╲
                      core      ports (interface)
                                  │
                                  ▼
                              adapter (concrete impl)
                                  │
                                  ▼
                           Hardware / DB / File
```

## 5. Adding a New Feature Checklist

1. Define domain logic in `core/<domain>/`
2. Define interfaces in `ports/` if external dependency needed
3. Implement adapter in `adapters/<type>/` for concrete dependency
4. Write usecase in `usecase/` orchestrating core + ports
5. Add Wails binding in `backend/bindings/<domain>.go`
6. Add bridge wrapper in `frontend/src/bridge/`
7. Add composable in `frontend/src/modules/<domain>/composables/`
8. Build UI components in `frontend/src/modules/<domain>/components/`
