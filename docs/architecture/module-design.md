# Module Design: Rust Backend + Vue 3 Frontend (Tauri Desktop App)

## 1. Rust Backend Module Structure

Organize by architectural responsibility. Each module should stay small and domain-named.

```
services/api-rs/src/
├── bin/
│   └── server.rs              # Server entry point and dependency wiring only
│
├── core/                      # Pure domain logic, no external dependencies
│   ├── acquisition/           # Acquisition domain: sample control, buffers, trigger rules
│   ├── calibration/           # Calibration domain: algorithms, coefficient calculation, flow
│   ├── measurement/           # Measurement domain: unit conversion, correction, verdict
│   └── project/               # Project management: test config, condition management
│
├── usecase/                   # Orchestration layer, coordinates core + ports
│   ├── acquire_task.rs        # Acquire use case: start/stop acquisition, real-time push
│   ├── calibrate_task.rs      # Calibrate use case: run calibration flow, save results
│   ├── device_manager.rs      # Device use case: connect/disconnect, status query
│   └── export_task.rs         # Export use case: data query, file export
│
├── ports/                     # Trait definitions only
│   ├── device.rs              # Device trait: read, write, connect, disconnect
│   ├── data_sink.rs           # DataSink trait: store, query
│   ├── event_bus.rs           # EventBus trait: publish/subscribe
│   └── config_repo.rs         # ConfigRepo trait: read/write config
│
└── adapters/                  # Concrete implementations
    ├── hardware/              # Device adapters: one module per device
    │   ├── daq_p1604.rs
    │   ├── stepping_motor.rs
    │   └── pressure_probe.rs
    ├── db/                    # Database adapters
    ├── mq/                    # Message/event adapters
    └── config/                # Config file adapters
```

### Backend Rules

| Rule | Description |
|---|---|
| core splits by business domain | Each domain is independent. `acquisition` must not import `calibration` unless there is an explicit domain-level reason. |
| usecase splits by user operation | One module per complete operation, such as `run_calibration`. |
| ports contains traits only | No behavior implementations. Keep external dependency traits stable. |
| adapters split by technology | `hardware` for device drivers, `db` for persistence, one module per concrete implementation. |
| cross-domain coordination in usecase | Logic that touches both acquisition and calibration lives in `usecase`, never in `core`. |
| async at boundaries | Keep async runtime/framework types out of `core`; isolate them in `bin`, `usecase`, or adapters as appropriate. |

## 2. Vue 3 Frontend Module Structure

Organize by feature domain. Each module is a self-contained directory.

```
frontend/src/
├── modules/                 # Business modules by feature domain
│   ├── device/              # Device management: connect, status, config
│   │   ├── components/      # DevicePanel, DeviceStatus, PortConfig
│   │   ├── composables/     # useDevice.ts (calls API client)
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
│   ├── components/          # Common UI components
│   ├── composables/         # Common hooks
│   ├── utils/               # Formatting, conversion, API helpers
│   └── styles/              # Global styles, theme variables
│
├── layouts/                 # Layout components
├── api/                     # Rust backend HTTP/WebSocket client
└── App.vue
```

### Frontend Rules

| Rule | Description |
|---|---|
| modules split by feature domain | Each module is self-contained: components + composables + types. |
| shared for cross-module reuse | Only promote to shared when 2+ modules use it. |
| API client wraps backend calls | Never call hardware, filesystem, or calibration algorithms directly from Vue. |
| composable maps to usecase | One composable should correspond to one Rust backend usecase area. |
| no direct cross-module imports | Acquisition does not import calibration components. Compose via events or layout layer. |

## 3. Tauri Desktop Shell

```
apps/desktop-tauri/
├── frontend/                # Vue 3 UI
└── src-tauri/               # Tauri startup, native shell, command bridge
```

### Shell Rules

| Rule | Description |
|---|---|
| no business logic | Tauri shell code does not implement calibration, acquisition, device control, or persistence rules. |
| bridge only | Tauri may start/stop backend services, expose app metadata, or provide desktop-specific capabilities. |
| Rust owns backend behavior | Device communication, data processing, state machines, and APIs live in `services/api-rs`. |

## 4. Data Flow

```
User Action
    │
    ▼
Vue Component ──calls──▶ composable
                              │
                              ▼
                         API client
                              │
                              ▼
                   Rust backend API / WS
                              │
                              ▼
                          usecase
                         ╱       ╲
                       core      ports (traits)
                                   │
                                   ▼
                               adapter (concrete impl)
                                   │
                                   ▼
                            Hardware / DB / File
```

## 5. Adding a New Feature Checklist

1. Define domain logic in `services/api-rs/src/core/<domain>/`.
2. Define traits in `services/api-rs/src/ports/` if an external dependency is needed.
3. Implement adapters in `services/api-rs/src/adapters/<type>/`.
4. Write usecases in `services/api-rs/src/usecase/` orchestrating core + ports.
5. Expose the usecase through the Rust backend API/WebSocket layer.
6. Add frontend API wrappers in `frontend/src/api/`.
7. Add composables in `frontend/src/modules/<domain>/composables/`.
8. Build UI components in `frontend/src/modules/<domain>/components/`.
