# Wind-DAQ Structure

## Directories

### Frontend (`apps/desktop-wails/frontend/src/`)

| Path | Description |
|---|---|
| `api/` | HTTP client (`http-client.ts`), typed API modules (`deviceApi.ts`, `types.ts`) |
| `components/device/` | DeviceManagementDrawer, DaqT1603Config, RecordingControl |
| `components/layout/` | AppShell, MainView, MainTopBar, AppRailNav, MainBottomBar, GlobalSettingsModal |
| `components/main/` | DeviceSidebar, DeviceDetailPanel |
| `stores/` | Pinia stores: deviceStore, themeStore, i18nStore, feedbackStore |
| `stores/__tests__/` | Vitest store unit tests |
| `styles/tokens/` | Design tokens: color, spacing, typography, motion, layout, radius |
| `styles/themes/` | dark.css |
| `views/` | MainDashboardView, MotionView, CalibrationView, TraversalView, LogViewer |
| `views/main/` | MainDashboardView layout |

### Backend (`services/api-go/`)

| Path | Description |
|---|---|
| `cmd/server/` | HTTP API server entrypoint |
| `api/` | HTTP handlers (thin routing, no business logic) |
| `internal/core/` | Pure domain types (device, motion, calibration, storage, traversal) |
| `internal/ports/` | Interface definitions (Device, Publisher, ProfileStore, etc.) |
| `internal/usecase/` | Orchestration (DeviceManager, AcquisitionHub, MotionManager, CalibrationManager, etc.) |
| `internal/adapters/` | Implementations (hardware, config, storage, scan) |
| `internal/bootstrap/` | Server wiring |

### Other

| Path | Description |
|---|---|
| `contracts/openapi/` | OpenAPI 3.0 specification |
| `config/` | Device profile JSON persistence |
| `docs/migration/` | Reference-to-Go migration analysis |
