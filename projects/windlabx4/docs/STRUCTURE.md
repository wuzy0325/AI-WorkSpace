# WindLabX4 Structure

## Directories

### Frontend (`apps/desktop-wails/frontend/src/`)

Directory structure follows `docs/runbooks/frontend-directory-rules.zh-CN.md`.

| Path | Description |
|---|---|
| `pages/` or `views/` | 页面级路由组件 |
| `components/ui/` | 基础 UI 控件（UiButton, UiInput, UiSelect, UiPanel…） |
| `components/layout/` | 布局组件（AppShell, MainTopBar, AppRailNav, MainBottomBar） |
| `components/feedback/` | 反馈组件（UiToastHost, UiConfirmDialog） |
| `components/main/` | 主面板组件（DeviceSidebar, DeviceDetailPanel） |
| `components/device/` | 设备管理组件（DeviceManagementDrawer, DaqT1603Config, RecordingControl） |
| `components/calibration/` | 校准 UI（五孔/三孔/总压/总温） |
| `components/traversal/` | 遍历 UI 与可视化 |
| `components/motion/` | 运动控制配置面板 |
| `components/icons/` | 图标组件 |
| `stores/` | Pinia stores |
| `api/` | HTTP/Wails API facade |
| `styles/tokens/` | 设计 token（color, spacing, typography, radius, motion, layout） |
| `styles/themes/` | 主题覆盖（dark.css, light.css） |
| `router/` | 路由配置 |
| `composables/` | 跨组件可复用 composable |
| `shared/` | 共享类型与 UI-only helper |
| `types/` | 全局类型声明 |
| `core/` | 框架无关容器与常量 |
| `spikes/` | 实验性代码（仅开发态） |

> **架构约束**: 前端只负责展示、交互和临时 UI 状态。所有业务规则、配置持久化、设备控制、
> 插值/校准/运动控制逻辑必须在 Go 后端或 shared 模块中。配置持久化通过后端
> `adapters/config` 实现，前端通过 API 调用。

### Backend (`services/api-go/`)

| Path | Description |
|---|---|
| `cmd/server/` | HTTP API server entrypoint |
| `api/` | HTTP handlers (thin routing, no business logic) |
| `internal/core/` | Pure domain types and algorithms (zero hardware imports, zero file I/O) |
| `internal/ports/` | Interface definitions only (Device, MotionAccess, AppConfigStore, ProfileStore, etc.) |
| `internal/usecase/` | Orchestration (DeviceManager, AcquisitionHub, MotionManager, CalibrationManager, TraversalManager, ConfigManager) |
| `internal/adapters/` | Implementations (hardware, config, storage, scan, interpolation, calstore, report) |
| `internal/adapters/config/` | Configuration persistence (device profiles, app config, default profiles) |
| `internal/bootstrap/` | Server wiring |

> **架构约束**: `core/` 零硬件导入、零文件I/O、零网络依赖。`usecase/` 通过 `ports/`
> 接口访问外部资源，不直接依赖 `adapters/`。`api/` 只做 HTTP 路由和请求/响应转换。

### Shared Algorithms (`shared/algorithms/go/fivehole/`)

| Path | Description |
|---|---|
| `interpolation/` | 五孔插值算法 canonical 实现（PrbInterpolator, FiveHoleNewInterpolator, MultiPrbInterpolator） |

> **唯一权威**: 所有五孔插值算法只保留此位置。WindLabX4 项目通过
> `internal/adapters/interpolation` 适配器调用，不得复制算法代码。

### Shared Motion (`shared/motion-control/go/`, `shared/device-sdk/go/motion/`)

| Path | Description |
|---|---|
| `shared/device-sdk/go/motion/` | 运动控制底层类型、ports、硬件 adapter、模拟器 |
| `shared/motion-control/go/` | MotionManager、profile 持久化、HTTP route glue、状态轮询 helper |

> **边界**: WindLabX4 不得导入 `projects/motion-controller/*`。可复用 motion 行为必须来自工作空间级 `shared/*` 模块。WindLabX4 项目内仅保留 DAQ、校准、遍历、存储、报告等产品特定逻辑和 wiring。

### Other

| Path | Description |
|---|---|
| `contracts/openapi/` | OpenAPI 3.0 specification |
| `config/` | Device profile JSON persistence |
| `docs/migration/` | Reference-to-Go migration analysis |
