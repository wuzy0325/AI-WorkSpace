# TS Reference → Wind-DAQ Feature Map

> **Purpose:** Track which features from the TS/Electron reference (`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ`) have been migrated to the Go + Vue + Wails rebuild.

**Status key:** Done / Partial / Missing / Frontend-only / Do not migrate

## Device Management

| Reference Feature | Status | Go Backend | Vue Frontend | Notes |
|---|---|---|---|---|
| Profile CRUD | Done | ✅ core/device + usecase/device_manager | ✅ DeviceManagementDrawer | REST API + HTTP client |
| Device scan (simulated) | Done | ✅ ports.DeviceScanner + simulated scanner | ✅ Drawer scan button | Real network scan pending |
| Device scan (network) | Partial | ✅ adapters/scan/network_scanner.go | ✅ Drawer scan button | UDP broadcast + protocol parsing, enable via WIND_DAQ_NETWORK_SCAN=true |
| Connect / Disconnect | Done | ✅ usecase + API | ✅ MainDashboardView / Drawer | HTTP POST |
| DAQ-T-1603 config | Partial | ✅ API + usecase | ✅ DaqT1603Config component | Hardware config fields only |
| Unit configuration | Partial | ✅ API PUT unit | ✅ DeviceManagementDrawer | Default unit per device type, no dedicated UI dropdown |
| Tare (zero offset) | Partial | ⬜ backend | ✅ deviceStore.setTare | Frontend-only display offset |

## Acquisition

| Reference Feature | Status | Go Backend | Vue Frontend | Notes |
|---|---|---|---|---|
| Simulated DAQ | Done | ✅ simulated.go | ✅ DeviceDetailPanel | 4 channels, 20 Hz |
| Start/Stop | Done | ✅ usecase + API | ✅ MainDashboardView | |
| Latest data polling | Done | ✅ GET /api/daq/latest/:id | ✅ 250ms polling | |
| SSE realtime stream | Done | ✅ GET /api/daq/stream/:id | ✅ sse-client.ts + MainDashboardView | SSE with auto-reconnect integrated |
| History buffer | Done | ✅ AcquisitionHub history | ✅ deviceStore history | 256 capacity ring buffer |
| DAQ-P-1604 driver | Partial | ✅ adapters/hardware/daq_p1604.go | N/A | Adapter exists, HIL pending |
| DAQ-T-1603 driver | Partial | ✅ adapters/hardware/daq_t1603.go | N/A | Adapter exists, HIL pending |
| Channel chart | Done | N/A | ✅ RealtimeChart (vue-echarts) | ECharts line chart with DataZoom |

## Motion Control

| Reference Feature | Status | Go Backend | Vue Frontend | Notes |
|---|---|---|---|---|
| Motion core types | Done | ✅ core/motion/types.go | N/A | |
| Motion ports | Done | ✅ ports/motion.go | N/A | |
| MotionManager usecase | Done | ✅ usecase/motion.go | N/A | |
| Simulated motion | Done | ✅ simulated_motion.go | N/A | |
| B140 driver | Missing | ⬜ adapters/hardware | N/A | No file found — do not migrate (current version scope) |
| WTNMC4A driver | Missing | ⬜ adapters/hardware | N/A | No file found — do not migrate (current version scope) |
| HTTP motion API | Done | ✅ api/server.go (9 routes) | N/A | connect/disconnect/home/stop/status/moveTo/moveBy/jog/emergencyStop |
| Motion page UI | Partial | N/A | ✅ MotionView | Connect/status/jog/MoveTo UI with inline controls |
| Motion control panel | Frontend-only | N/A | ✅ MotionView (inline) | Jog velocity input + axis buttons + MoveTo integrated |

## Calibration

| Reference Feature | Status | Go Backend | Vue Frontend | Notes |
|---|---|---|---|---|
| Calibration core types | Done | ✅ core/calibration/types.go | N/A | |
| CalibrationManager usecase | Done | ✅ usecase/calibration.go | N/A | Start/pause/resume/stop |
| 1604 calibration workflow | Missing | ⬜ | ⬜ | Full IPC-based flow pending |
| Five-hole calibration | Missing | ⬜ | ⬜ | |
| Three-hole calibration | Missing | ⬜ | ⬜ | |
| Total pressure calibration | Missing | ⬜ | ⬜ | |
| Total temperature calibration | Missing | ⬜ | ⬜ | |
| Calibration page shell | Partial | N/A | ✅ CalibrationView | 4 probe type cards |

## Traversal

| Reference Feature | Status | Go Backend | Vue Frontend | Notes |
|---|---|---|---|---|
| Traversal core types | Done | ✅ core/traversal/types.go | N/A | |
| Traversal path interpolation | Done | ✅ core/traversal/path.go | N/A | Linear path interpolation |
| TraversalManager usecase | Done | ✅ usecase/traversal.go | N/A | Start/pause/resume/stop |
| Traversal page shell | Partial | N/A | ✅ TraversalView | 3 feature cards |
| Traversal workflow steps | Missing | ⬜ | ⬜ | HIL/Layout/PRB step components |
| Traversal visualization | Missing | ⬜ | ⬜ | Heatmap, vector, cross-section |

## Storage & Recording

| Reference Feature | Status | Go Backend | Vue Frontend | Notes |
|---|---|---|---|---|
| Recording config | Done | ✅ core/storage | N/A | |
| CSV recording sink | Done | ✅ adapters/storage | N/A | |
| StorageRecorder usecase | Done | ✅ usecase/storage.go | N/A | |
| Recording control UI | Frontend-only | N/A | ✅ RecordingControl | Visual button only |

## UI Shell & Layout

| Reference Feature | Status | Notes |
|---|---|---|
| AppShell | Done | Slot-based layout with header/rail/sidebar/canvas/statusbar |
| MainTopBar | Done | Brand, mode tabs, theme toggle, locale switch |
| AppRailNav | Done | 5 pages + settings |
| MainBottomBar | Done | Start/stop/record buttons, elapsed time, clock |
| DeviceSidebar | Done | Profile list with status indicators |
| DeviceDetailPanel | Done | Channel grid with values, sparklines, range info |
| DeviceManagementDrawer | Done | Profile CRUD + scan |
| GlobalSettingsModal | Done | Theme/language/about |

## Design System

| Reference Feature | Status | Notes |
|---|---|---|
| Color tokens | Done | tokens/color.css + themes/dark.css |
| Spacing tokens | Done | tokens/spacing.css |
| Typography tokens | Done | tokens/typography.css |
| Layout tokens | Done | tokens/layout.css (adjusted to DESIGN.md) |
| Motion tokens | Done | tokens/motion.css |
| Radius tokens | Done | tokens/radius.css |
| Glass utility CSS | Done | styles/glass.css |

## Not Migrating

These items from the reference are explicitly **not** being migrated:

- **Electron main process** (`src/main/index.ts`)
- **Electron IPC registration** (`src/main/ipc/register*.ts`)
- **Electron preload** (`src/preload/`)
- **Electron IPC client** (`electron-api-client.ts`)
- **Old shared/ipc/** module
- **Generated Wails JS** (`wails-backend/frontend/src/wailsjs/`)
- **Old Wails shell** (`wails-backend/`)
- **Reference binary/build artifacts**
