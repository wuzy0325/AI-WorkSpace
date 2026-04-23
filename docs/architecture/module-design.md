# Module Design: Go Backend + Vue 3 Frontend (Wails Desktop App)

## 1. Go Backend Module Structure

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

Organize by feature domain. Each module is a self-contained directory.

```
frontend/src/
├── modules/                 # Business modules by feature domain
│   ├── device/              # Device management: connect, status, config
│   │   ├── components/      # DevicePanel, DeviceStatus, PortConfig
│   │   ├── composables/     # useDevice.ts (calls backend device methods)
│   │   └── types.ts
│   ├── acquisition/         # Real-time acquisition: waveform, sample params
│   │   ├── components/      # WaveformChart, SampleConfig, ChannelSelector
│   │   ├── composables/     # useAcquisition.ts
│   │   └── types.ts
│   ├── calibration/         # Calibration: wizard, results display
│   │   ├── components/      # CalibWizard, CalibResult, CoeffTable
│   │   ├── composables/     # useCalibration.ts
│   │   └── types.ts
│   ├── history/             # Historical data: query, replay, export
│   └── settings/            # System settings: preferences, configuration
│
├── shared/                  # Internal frontend sharing
│   ├── components/          # Common UI components (buttons, tables, dialogs)
│   ├── composables/         # Common hooks (useToast, useLoading)
│   ├── utils/               # Utility functions (formatting, conversion)
│   └── styles/              # Global styles, theme variables
│
├── layouts/                 # Layout components (main frame, sidebar)
├── bridge/                  # Wails binding wrapper (unified backend calls)
└── App.vue
```

### Frontend Rules

| Rule | Description |
|---|---|
| modules split by feature domain | Each module is self-contained (components + composables + types). |
| shared for cross-module reuse | Only promote to shared when 2+ modules use it. |
| bridge layer wraps backend calls | Never call Wails API directly in composables. Go through bridge. |
| composable maps to usecase | One composable corresponds to one backend usecase. |
| no direct cross-module imports | acquisition does not import calibration components. Compose via events or layout layer. |

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
