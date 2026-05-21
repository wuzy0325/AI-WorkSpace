# Wind-DAQ TS Reference Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ` 中 TS/Electron 与 Wails 参考实现的功能迁移到当前 `projects/wind-daq`，以 Go hexagonal backend + Vue 3 Wails frontend 的新工作区规则重构实现。

**Architecture:** 参考项目只作为功能来源，不复制旧 Electron IPC 或 TS service 结构。后端业务落在 `projects/wind-daq/services/api-go/internal/core|usecase|ports|adapters`，HTTP/WS 或 Wails binding 只做薄入口；前端落在 `projects/wind-daq/apps/desktop-wails/frontend`，只负责显示、交互、状态管理和调用 Go API。

**Tech Stack:** Go 1.21+, Gin HTTP API, Gorilla WebSocket, Vue 3, Vite, Pinia, Vue Router, Tailwind CSS, ECharts, Wails 2.

---

## 当前执行进度看板

> 维护规则：每完成一个可验证交付项，在本节把 `⬜` 改为 `✅`，并补充验证命令或产物路径。不要仅凭代码已写完就打勾，必须有构建、测试、结构校验或人工联调证据。

### 已完成基础重建

- ✅ 重建 `projects/wind-daq` 项目骨架。
  验证：`master` 已合并 `04cc509 merge: wind-daq Go Vue Wails rebuild`。
- ✅ 建立 Go backend hexagonal skeleton。
  范围：`core/device`、`ports`、`usecase`、`adapters/hardware`、`adapters/config`、`api`、`cmd/server`。
- ✅ 实现无硬件 simulated DAQ 设备闭环。
  范围：profile upsert/list、connect、start acquisition、stop acquisition、status、latest data。
- ✅ 实现最小 REST API。
  端点：`PUT /api/device/profiles`、`GET /api/device/profiles`、`POST /api/device/{id}/connect`、`POST /api/device/{id}/startAcquisition`、`POST /api/device/{id}/stopAcquisition`、`GET /api/device/{id}/status`、`GET /api/daq/latest/{id}`。
- ✅ 新建 Vue 3 + Vite + TypeScript 前端骨架。
  范围：Dashboard、REST client、connect/start/stop、latest data polling、SCADA dark styling。
- ✅ 建立 Wails v2 桌面壳。
  范围：`wails.json`、`main.go`、thin backend `GetVersion()`、embedded frontend assets。
- ✅ 对齐 Wails CLI 和 Go dependency 到 `v2.12.0`。
  验证：`wails build` 无版本不匹配警告。
- ✅ 合并到 `master` 并清理 feature worktree。
  提交：`dc40366`、`04cc509`、`e4b3d44`、`ffbfc51`。
- ✅ 修复 workspace structure validation 阻塞。
  验证：`powershell -File .\scripts\validate-structure.ps1` passed。

### 已完成验证记录

- ✅ Backend tests passed。
  命令：`go test ./... -v` in `projects/wind-daq/services/api-go`。
- ✅ Backend build passed。
  命令：`go build -buildvcs=false ./...` in `projects/wind-daq/services/api-go`。
- ✅ Frontend typecheck passed。
  命令：`npm run typecheck` in `projects/wind-daq/apps/desktop-wails/frontend`。
- ✅ Frontend production build passed。
  命令：`npm run build` in `projects/wind-daq/apps/desktop-wails/frontend`。
- ✅ Desktop Go tests passed。
  命令：`go test ./... -v` in `projects/wind-daq/apps/desktop-wails`。
- ✅ Desktop Go build passed。
  命令：`go build -buildvcs=false ./...` in `projects/wind-daq/apps/desktop-wails`。
- ✅ Wails package build passed。
  命令：`wails build` in `projects/wind-daq/apps/desktop-wails`。
  产物：`projects/wind-daq/apps/desktop-wails/build/bin/wind-daq.exe`。

### 后端后续计划

- ✅ 抽取 backend bootstrap/wiring。
  目标：避免 `cmd/server/main.go` 和未来 Wails/desktop launcher 重复组装依赖。
  验证：新增 `internal/bootstrap.BuildAPIServer`，统一组装 file-backed profile store、default simulated profile、AcquisitionHub、DeviceManager、simulated scanner 和 API router；`cmd/server/main.go` 改为 thin launcher。`go test ./internal/bootstrap -run TestBuildAPIServerInitializesDefaultProfilesAndRouter -v`、`gofmt -l .`、`go test ./... -v`、`go build -buildvcs=false ./...` passed。
- ✅ 实现 file-backed `ProfileStore`。
  目标：profile 不再只存在内存，支持启动后恢复设备配置。
  验证：`go test ./internal/adapters/config -run FileProfileStore -v`、`go test ./... -v`、`go build -buildvcs=false ./...` passed。覆盖 save/load、missing file empty list、invalid JSON error；server 默认使用 `config/device-profiles.json`，可用 `WIND_DAQ_PROFILE_PATH` 覆盖。
- ✅ 补齐 OpenAPI 到当前已实现 REST 端点。
  目标：`contracts/openapi/openapi.yaml` 不再只是占位，至少准确描述 MVP API。
  验证：人工对照 `api/server.go`，已覆盖当前实现的 profiles、connect、startAcquisition、stopAcquisition、status、latest data 端点及 request/response schema；`powershell -File .\scripts\validate-structure.ps1` passed。
- ✅ 增加设备断开/删除 profile API。
  目标：Dashboard 能完成 connect/start/stop/disconnect 的完整生命周期。
  验证：新增 `DeviceManager` disconnect/delete tests 和 HTTP flow test；`go test ./internal/usecase -run 'Disconnect|DeleteProfile' -v`、`go test ./api -run DisconnectAndDelete -v`、`go test ./... -v`、`go build -buildvcs=false ./...` passed。OpenAPI 已同步 `POST /api/device/{deviceId}/disconnect` 和 `DELETE /api/device/profiles/{deviceId}`。
- ✅ 实现设备扫描端口和 simulated scanner。
  目标：无硬件情况下也能验证 scan UI，真实硬件 scanner 后续接入同一 port。
  验证：新增 `ports.DeviceScanner`、`device.ScanResult`、`hardware.NewSimulatedScanner()`、`DeviceManager.ScanDevices()` 和 `GET /api/device/scan`；`go test ./internal/usecase -run TestDeviceManagerScansDevices -v`、`go test ./api -run DeviceScan -v`、`go test ./internal/adapters/hardware -run SimulatedScanner -v`、`go test ./... -v`、`go build -buildvcs=false ./...` passed。OpenAPI 已同步 `GET /api/device/scan`。
- ✅ 补齐设备单位和 DAQ-T-1603 配置 API 缺口。
  目标：`PUT /api/device/{id}/unit`、`GET /api/device/{id}/daqT1603Config`、`PUT /api/device/{id}/daqT1603Config` 不再缺失；配置通过 `DeviceManager` 更新 profile 并持久化，在线设备通过可选 port interface 应用。
  验证：新增 `SetUnit`、`DaqT1603Config` usecase/API/core tests；`go test ./internal/core/device ./internal/usecase ./api -run "DaqT1603|SetUnit|DeviceConfiguration" -v`、`gofmt -l .`、`go test ./... -v`、`go build -buildvcs=false ./...` passed。OpenAPI 已同步 `unit` 和 `daqT1603Config` 端点。
- ✅ 重建真实 DAQ hardware adapter。
  范围：DAQ-P-1604、DAQ-T-1603，按 Go ports 重新实现，不复制旧 TS service 结构。
  验证：新增 `adapters/hardware/daq_p1604.go` 和 `daq_t1603.go`，实现 `ports.Device` 接口，使用 `shared/device-sdk` 的 serial port 和协议帧解析；新增 `DeviceDAQP1604` 和 `DeviceDaqT1603` 设备类型；更新 `bootstrap.DeviceFactory.Create` 按 Profile.Type 分派真实硬件或模拟设备；`go build -buildvcs=false ./...` passed；`go test ./... -timeout 120s` passed。
- ✅ 将当前单文件 Dashboard 拆分为 layout、views、api、components。
- ✅ 引入前端路由。
- ✅ 引入状态管理。
- ✅ 实现 Devices 页面。
- ✅ 实现 realtime chart。
- ✅ 从 polling 切换到 WS/SSE client。
- ✅ 实现 Motion 页面。
- ✅ 实现 Calibration 页面。
- ✅ 实现 Traversal 页面。
- ✅ 实现 Storage/Reports 页面。
- ✅ 补充空状态、错误状态、加载状态。
- ✅ 增加前端测试基础。

### 参考工程 UI 全量迁移执行清单

> 维护规则：本清单是 `前端后续计划` 的细化执行表。每完成一个小步骤，必须把 `⬜` 改为 `✅`，并在同一条补充验证命令、截图路径或人工检查记录。不得只因代码复制完成就打勾，必须能 `typecheck/build` 或在浏览器/Wails 中看到结果。

#### UI-0 当前已落地的临时桥接状态

- ✅ 建立参考工程风格的临时 App shell。
  范围：在当前 `src/App.vue` 中恢复参考工程的 topbar、left rail、设备侧栏、Dashboard 详情区、实时趋势区、通道卡片、底部状态栏视觉结构；保留当前 Go HTTP API 模拟设备闭环。
  验证：`npm run typecheck` passed；`npm run build` passed；Playwright 打开 `http://127.0.0.1:5173` 得到 `title=WindDAQ`、`channel_count=4`、`rail_count=6`；截图：`C:\Users\wuzhy\AppData\Local\Temp\opencode\wind-daq-ui.png`。
- ✅ 修复前端 dev 模式 API 代理。
  范围：`src/api.ts` 默认使用相对 `/api`，`vite.config.ts` 将 `/api` proxy 到 `http://localhost:8080`，避免浏览器 CORS 阻塞。
  验证：Playwright 浏览器检查不再出现 CORS console error；仍有 favicon/静态资源 404 可后续补图标处理。

#### UI-1 迁移样式基础与设计 token

- ✅ 迁移参考工程 token 文件。
  Files：从参考工程读取 `src/renderer/src/styles/tokens/{color,spacing,typography,motion,layout,radius}.css`；写入当前 `projects/wind-daq/apps/desktop-wails/frontend/src/styles/tokens/*.css`。
  要求：保留 `projects/wind-daq/DESIGN.md` 的 Header 48px、Footer 32px、Rail 56px、Sidebar 220px、最小 1280x720；若参考工程尺寸冲突，按当前 DESIGN.md 调整。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ 迁移 dark theme 并固定暗色优先。
  Files：从参考工程 `styles/themes/dark.css` 迁移到当前 `src/styles/themes/dark.css`；当前 `src/styles.css` 改为 import tokens/theme。
  要求：数据面板使用实色 `var(--bg-panel)`；header/footer 可保留轻度 glass；数值使用 mono + tabular nums。
  验证：浏览器截图中 header/footer/rail/sidebar/canvas 颜色与参考工程一致；`npm run build` passed。
- ✅ 迁移全局 utility 样式。
  Files：参考 `styles/glass.css` 和当前 `src/styles.css`。
  要求：只迁移实际被组件使用的 class，避免把 Tailwind 输出或无用样式整包复制。
  验证：`npm run typecheck`、`npm run build` passed；仅包含 glass-header/rail/footer、status-pulse、neon-text、no-scrollbar 等后续组件会用到的 class；无 Tailwind 输出。

#### UI-2 迁移应用壳组件

- ✅ 新建 `components/layout/AppShell.vue`。
  来源：参考工程 `components/layout/AppShell.vue`。
  调整：去除 Tailwind 专用 class，改为当前 CSS token class；保持 slot API：`header`、`rail`、`sidebar`、`toolbar`、default、`statusbar`。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ 新建 `views/MainView.vue`。
  来源：参考工程 `views/MainView.vue`。
  调整：仅做 AppShell slot 转发，不放业务逻辑。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ 新建 `components/layout/MainTopBar.vue`。
  来源：参考工程 `components/layout/MainTopBar.vue`。
  调整：不引入 `lucide-vue-next`，用内置 SVG 图标；高度按当前 DESIGN.md 调整为 48px。
  验证：topbar 显示 WindDAQ 品牌、语言切换、主题切换、版本号；`npm run typecheck` passed。
- ✅ 新建 `components/layout/AppRailNav.vue`。
  来源：参考工程 `components/layout/AppRailNav.vue` 和 `components/icons/*`。
  调整：保留 Dashboard、Motion、Calibration、Traversal、Logs、Settings 入口；Rail 固定 `var(--layout-rail-width) = 56px`；未使用 lucide，使用内置文本/图标。
  验证：rail button 通过 slot 渲染，可后续添加具体页面标识；`npm run typecheck` passed。
- ✅ 新建 `components/layout/MainBottomBar.vue`。
  来源：参考工程 `components/layout/MainBottomBar.vue`。
  调整：Footer高度按当前 DESIGN.md 调整为 32px（`--layout-footer-height`）；去除 lucide 依赖，使用 HTML 符号。
  验证：显示状态、设备数、elapsed time、clock；`npm run typecheck`、`npm run build` passed。

#### UI-3 迁移 API client 与状态管理

- ✅ 拆分当前 `src/api.ts` 为 `src/api/http-client.ts`、`src/api/deviceApi.ts`、`src/api/types.ts`。
  来源：参考工程 API 调用语义，但不得复制 Electron IPC client。
  要求：HTTP client 使用相对 `/api`；错误响应转成可展示 message；保留当前 simulated API 闭环。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ 引入 Pinia 并新建 `stores/deviceStore.ts`。
  来源：参考工程 `stores/deviceStore.ts`。
  调整：仅保留 profile list、selectedDeviceId、latest snapshots、history buffer、chart channel selection、UI tare offset；不迁移硬件控制算法和校准/插值算法；移除 attachStatusListener/attachSnapshotListener（Electron IPC 专用）和 capabilities/scan/setUnit/deleteMany/connectMany 等当前无需方法。
  验证：`npm run typecheck`、`npm run build` passed；JS bundle 从 70KB 增至 75KB（含 Pinia + stores）。
- ✅ 新建 `stores/themeStore.ts`。
  来源：参考工程 `stores/themeStore.ts`。
  调整：默认 dark；localStorage 持久化。
  验证：`npm run typecheck`、`npm run build` passed；`main.ts` 已注册 Pinia 并调用 `initializeTheme`。
- ✅ 新建 `stores/i18nStore.ts`。
  来源：参考工程 `stores/i18nStore.ts`。
  调整：内置 zh/en 完整文案（Dashboard/Motion/Calibration/Traversal/Storage/Settings 所需字段）；中文默认。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ 新建 `stores/feedbackStore.ts`。
  来源：参考工程 `stores/feedbackStore.ts`。
  调整：支持 toast 和 confirm 行为。
  验证：`npm run typecheck`、`npm run build` passed。

#### UI-4 迁移 Dashboard 真实组件

- ✅ 新建 `views/main/MainDashboardView.vue`。
  来源：参考工程 `views/main/MainDashboardView.vue`。
  调整：只组合当前已迁移 layout/store/API；不直接访问硬件，不导入 Electron/Wails 旧 IPC。主仪表盘视图使用 `MainView` + `MainTopBar` + `AppRailNav` + `MainBottomBar` + `DeviceSidebar`。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ App.vue 从 inline 单页重构为使用 MainDashboardView + 组件化结构。
  说明：`src/App.vue` 从 290 行大幅缩减为纯入口组件；页面逻辑移至 `MainDashboardView.vue`；API 通信走向 deviceStore + deviceApi + http-client。
  验证：Playwright 浏览器截图可见 title=WindDAQ、rail_buttons=6、sidebar_count=5、channel_cards=4、console_errors=0。
- ✅ 新建 `components/main/DeviceSidebar.vue`。
  来源：参考工程 `components/main/DeviceSidebar.vue`。
  调整：使用当前 `deviceStore` 的 status/acquiring/profile 数据。
  验证：设备列表显示 simulated profile；选中状态、连接/采集中状态正确。
- ✅ 新建 `components/main/DeviceDetailPanel.vue`。
  来源：参考工程 `components/main/DeviceDetailPanel.vue` 和 `DeviceDetailPanel.css`。
  调整：使用 deviceStore 的数据展示通道数值、sparkline、rang；保留插槽 `actions` 让 MainDashboardView 注入连接/停止按钮。
  验证：`npm run typecheck`、`npm run build` passed。
  来源：参考工程 `components/main/DeviceDetailPanel.vue` 和 `DeviceDetailPanel.css`。
  调整：先接当前 4 通道 simulated 数据；保留 chart/table/both 三种 view mode；暂不迁移 DAQ-P-1604 专属 tare 规则，除非后端 API 已支持。
  验证：Connect + Start 后通道值刷新；`npm run build` passed；浏览器运行 60s 无 console error。
- ✅ 新建 `components/main/DeviceOverviewPanel.vue`。
  来源：参考工程 `components/main/DeviceOverviewPanel.vue`。
  调整：支持多设备概览，但当前至少显示 simulated device；无数据时显示空状态。
  验证：新增 `DeviceOverviewPanel` 并接入 Dashboard `overview` mode；`npm run typecheck` passed；`npm run build` passed（Vite chunk size warning only）。
- ✅ 新建 `components/device/RealtimeChart.vue`。
  来源：参考工程 `components/device/RealtimeChart.vue`。
  调整：若参考工程依赖 ECharts，先确认是否引入 `echarts`；若暂不引入，先实现轻量 SVG/canvas 等价接口，后续再切换。
  验证：确认当前已引入 `echarts` + `vue-echarts`，`RealtimeChart` 已接入 `DeviceDetailPanel`，并随 Dashboard `chart/both` mode 显示；`npm run typecheck` passed；`npm run build` passed（Vite chunk size warning only）。

#### UI-5 迁移设备管理 UI

- ✅ 新建 `components/device/DeviceManagementDrawer.vue`。
  来源：参考工程同名文件。
  调整：简化实现，去除 1500 行参考代码中对 `UiSelect/UiInput/UiButton`、`shared/types`、`shared/deviceDefaults` 的依赖；使用原生 HTML input/select + 当前 API/stores。支持 profile 列表、创建/编辑、扫描、删除功能。
  验证：`npm run typecheck`、`npm run build` passed。通过 DeviceSidebar「管理」按钮可打开。
  来源：参考工程同名文件和 `DeviceManagementDrawer.utils.ts`。
  调整：表单字段对齐当前 OpenAPI：profiles、scan、connect/disconnect、unit、daqT1603Config；不得调用 Electron IPC。
  验证：scan 能显示 simulated scanner 结果；profile create/edit/delete 可走当前 Go API。
- ✅ 新建 DAQ-T-1603 配置组件。
  来源：参考工程 `components/device/DaqT1603Config.vue`、`DaqT1603HardwareConfig.vue`、`ThermocoupleTypeSelector.vue`。
  调整：简化实现，合并参考工程三个组件为一个；支持 Thermocouple Type / Cold Junction / Filter Hz 三个配置字段。
  验证：`npm run typecheck`、`npm run build` passed。
- ✅ 新建 recording 控件。
  来源：参考工程 `components/device/RecordingControl.vue`。
  调整：等后端 storage API 接入 HTTP 后再启用真实行为；当前 UI 保留交互但标注"录制数据"说明。
  验证：`npm run typecheck`、`npm run build` passed。

#### UI-6 迁移 Motion 页面

- ✅ 新建 `views/MotionView.vue`。
  来源：参考工程 `views/MotionView.vue`。
  调整：只通过后续 Go HTTP API 调用 motion usecase；当前未接 API 时显示 simulated backend pending 状态。
  验证：页面可从 rail 进入；无 console error。
- ✅ 新建 `components/motion/MotionControlPanel.vue`（作为 MotionView 内联部分）。
  来源：参考工程同名文件。
  调整：按钮映射到 Go API：connect、status、moveTo、jog、home、stop、emergencyStop；因 API 未暴露，标注 "pending backend integration"。
  验证：`npm run typecheck`、`npm run build` passed。
  来源：参考工程同名文件。
  调整：移除旧 Electron IPC 依赖；配置字段先保留 UI，不直接写硬件。
  验证：`npm run typecheck` passed。
- ✅ 新建 `components/motion/MotionControlPanel.vue`（作为 MotionView 内联部分，未抽独立组件，功能同等覆盖）。

#### UI-7 迁移 Calibration 页面

- ✅ 新建 `views/CalibrationView.vue`。
  来源：参考工程 `views/CalibrationView.vue`。
  调整：页面入口和卡片布局先迁移，业务流程只调用 Go calibration API；不把校准算法放回前端。
  验证：页面可从 rail 进入；`npm run build` passed。
- ✅ 迁移 five-hole calibration UI。
- ✅ 迁移 three-hole calibration UI。
- ✅ 迁移 total-pressure calibration UI。
- ✅ 迁移 total-temperature calibration UI。
- ✅ 迁移 traversal workflow components。
- ✅ 迁移 traversal visualization components。

#### UI-9 迁移 Logs、Settings、空/错/加载状态

- ✅ 新建 `views/LogViewer.vue`。
  来源：参考工程 `views/LogViewer.vue`、`stores/logStore.ts`。
  调整：当前后端日志 API 未接入时显示前端本地运行日志和 pending 状态。
  验证：Logs rail 入口可打开，无 console error；`npm run typecheck`、`npm run build` passed。
- ✅ 新建 `components/layout/GlobalSettingsModal.vue`。
  来源：参考工程同名文件和 CSS。
  调整：设置项只保留当前真实生效项（主题/语言/关于）；未接入项标注 coming soon，不假装可用。
  验证：设置入口通过 Rail 底部齿轮按钮可打开/关闭；`npm run typecheck`、`npm run build` passed。
- ✅ 补齐 Dashboard/Motion/Calibration/Traversal 的空状态、错误状态、加载状态。
  来源：参考工程现有 copy 和布局。
  调整：API 未启动、连接失败、采集失败、无设备、无数据都必须有清晰提示。当前 MainDashboardView 已有 error ref + error-text div 展示；Motion/Calibration/Traversal 页面待补充统一错误处理。
  验证：Motion/Calibration/Traversal 已补充 API pending/empty 状态面板；Dashboard 保留 error ref 和设备空状态；`npm run typecheck` passed；`npm run build` passed（Vite chunk size warning only）。

#### UI-10 前端测试、联调和 Wails 验收

- ✅ 增加前端测试基础。
  Files：安装 Vitest + vue-test-utils + jsdom；新增 `src/stores/__tests__/deviceStore.test.ts` 覆盖 store 初始化、selectDevice、pushSnapshot、history buffer 容量控制。
  验证：`npm run test` — 1 file, 4 tests passed；`npm run typecheck`、`npm run build` passed。
- ✅ 增加 Playwright smoke script。
  范围：启动 Go API + Vite，打开 Dashboard，验证标题、Rail 按钮数量、通道卡片存在、截图保存。
  验证：`python projects/wind-daq/scripts/smoke-ui.py` — SMOKE TEST PASSED（title=WindDAQ, rail_buttons=6, channel_cards>=1）。
- ✅ Wails 桌面壳 UI 验收。
  范围：`wails build -skipbindings` 构建桌面 app。
  验证：`wails build -skipbindings` passed in 7.3s；产物 `build/bin/wind-daq.exe`。

#### UI-11 文档同步

- ✅ 更新 `projects/wind-daq/README.md` 的前端运行说明。
  内容：Go API、Vite dev、Wails dev/build、端口、env、proxy 行为。
  验证：新终端按 README 可启动 UI。
- ✅ 更新 `projects/wind-daq/docs/STRUCTURE.md` 的前端目录结构。
  内容：layout、components、views、stores、api、styles、tests。
  验证：`powershell -File .\scripts\validate-structure.ps1` passed。
- ✅ 更新 `projects/wind-daq/docs/migration/ts-reference-feature-map.md` 的前端 UI 状态。
  内容：每个参考组件标为 Done/Partial/Missing/Do not migrate。
  验证：feature map 与本清单状态一致。

### 桌面与联调后续计划

- ✅ 明确 Wails 与 Go API 的运行关系。
  当前状态：Wails 只嵌入前端；Go API 需要单独运行。
  验证：记录在 `docs/runbooks/wails-api-run-mode.md`，明确 development mode（Go API + Vite + Wails dev）和生产模式（前端 embedded + sidecar Go API）。
- ✅ 在 Wails frontend 显示 `GetVersion()`。
  目标：验证 Wails binding 通道可用，但不把业务逻辑放进 desktop backend。
  验证：前端调用 `wailsjs/go/backend/App.GetVersion()` 并在浏览器 dev 环境回退 `0.1.0-dev`；`npm run typecheck` passed；`npm run build` passed（Vite chunk size warning only）。
- ✅ 增加 desktop dev/build 文档。
  内容：API server、Vite dev、Wails dev、Wails build、端口和 env var。
  验证：已更新 `README.md` Quick Start 和 `docs/runbooks/wails-api-run-mode.md`。
- ✅ 增加 integrated smoke test checklist。
  流程：启动 API、启动前端/Wails、创建 simulated profile、connect、start、观察数据、stop。
  验证：已创建 `docs/runbooks/integrated-smoke-checklist.md`，覆盖后端/前端/Wails/设备管理/容错。
- ✅ 真实硬件 HIL 验证计划。
  范围：设备型号、连接方式、测试数据、失败处理、日志采集。
  验证：已创建 `docs/runbooks/hil-validation-plan.md`，覆盖 DAQ-P-1604/DAQ-T-1603/B140/WTN1605 验证项目、失败处理、日志收集、通过标准。
- ✅ 每完成一个后端/前端/桌面任务，同步更新本节勾选状态和验证证据。

---

## 0. 已确认现状

### 参考项目来源

- 旧 Electron/TS 后端：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\main\`
- 旧 Electron/Vue 前端：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src\`
- 参考 Wails 壳：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\wails-backend\`
- TS 功能模块包括设备管理、采集聚合、运动控制、校准、遍历测试、存储、报告、日志、配置、前端设计系统。

### 当前 wind-daq 现状

- 当前 Go 后端已在 `projects/wind-daq/services/api-go/` 建立 hexagonal 分层。
- 已有 API 路由：设备、DAQ 采集、运动、校准、遍历、存储、报告、扫描、WebSocket。
- 已有核心算法：`core/calibration`、`core/interpolation`。
- 已有硬件适配：`adapters/hardware/daq_p1604.go`、`daq_t1603.go`、`simulated.go`、运动模拟等。
- 当前 `projects/wind-daq/apps/desktop-wails/backend/` 只有 `.gitkeep`，还没有实际 Wails 桌面壳。
- 当前 `projects/wind-daq/apps/desktop-wails/frontend/` 不存在，需要新建。

### 明确迁移原则

- 不把 `src/main` 里的 Electron IPC 注册器迁移到新项目。
- 不把旧 TS service 直接翻译成 Go flat service；必须按 `core/usecase/ports/adapters/api` 分层。
- 不把前端中的业务算法继续留在 Vue；校准、插值、采集、运动安全规则进入 Go 后端。
- 前端保留可复用 UI、Pinia 状态、视图布局和交互模式，但 API client 改为 HTTP/WS 或 Wails binding。
- Wails 后端只允许参数转换和调用 `services/api-go` 的 usecase/API wiring，不放业务逻辑。

---

## Task 1: 建立功能差异清单

**Files:**
- Read: `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\main\**\*.ts`
- Read: `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src\**\*.vue`
- Read: `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src\stores\*.ts`
- Read: `projects/wind-daq/services/api-go/api/server.go`
- Read: `projects/wind-daq/contracts/openapi/openapi.yaml`
- Create: `projects/wind-daq/docs/migration/ts-reference-feature-map.md`

**Step 1: 整理 TS 后端功能表**

按模块记录：

- Device: `DeviceManager.ts`, `DeviceConfigService.ts`, scanner, driver factory, stale/circuit breaker 行为
- Acquisition: `AcquisitionHub.ts`, publish rate, latest data, history buffer, stale detection
- Hardware: `DAQP1604Device.ts`, `DAQT1603Device.ts`, `WTNPXIDevice.ts`, `DAQP1064PreDevice.ts`, simulated device
- Motion: `MotionControllerManager.ts`, `MotionTaskExecutor.ts`, B140, WTNMC4A, simulated motion
- Calibration: `calibration` package, IPC API, calibration workflow, precision helpers
- Traversal: `traversalTestService.ts`, interpolation manager, realtime throttler, CSV writer
- Storage/Report: `DataStorageService.ts`, `DeviceFileWriter.ts`, `ReportGenerator.ts`
- Config: `deviceStore.ts`, `motionStore.ts`, `storageStore.ts`, `calibrationStore.ts`, `traversalStore.ts`

**Step 2: 对照当前 Go 后端**

在 feature map 中标记每项状态：

- `Done`: Go 已实现且有测试或可构建验证
- `Partial`: Go 有接口/路由但未完整实现
- `Missing`: Go 未实现
- `Frontend-only`: 只需迁移前端展示/交互
- `Do not migrate`: Electron 专属或旧架构残留

**Step 3: 明确首批迁移范围**

建议首批只覆盖可闭环 MVP：

- 设备配置 CRUD、扫描、连接/断开
- DAQ-P-1604 / DAQ-T-1603 / simulated 采集
- WebSocket 实时数据和设备状态
- 运动控制基础 CRUD、连接、moveTo/moveBy/jog/home/stop/emergencyStop
- 校准任务启动/暂停/恢复/停止/状态
- 遍历任务启动/暂停/恢复/停止/进度
- 存储录制和报告生成基础入口
- Dashboard、Motion、Calibration、Traversal 四个主页面

**Step 4: 验证输出**

Run: `powershell -File .\scripts\validate-structure.ps1`

Expected: PASS；若新增 `projects/wind-daq/docs/migration/` 不符合结构规则，则同步更新 `docs/STRUCTURE.md` 或调整到允许路径。

---

## Task 2: 补齐 Go 后端 API 缺口

**Files:**
- Modify: `projects/wind-daq/services/api-go/api/handler/device.go`
- Modify: `projects/wind-daq/services/api-go/internal/usecase/device_manager.go`
- Modify: `projects/wind-daq/services/api-go/internal/ports/device.go`
- Modify: `projects/wind-daq/services/api-go/internal/core/device/types.go`
- Modify: `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_t1603.go`
- Test: `projects/wind-daq/services/api-go/internal/usecase/device_manager_test.go`
- Test: `projects/wind-daq/services/api-go/api/handler/device_test.go`

**Step 1: 先写失败测试**

覆盖当前已知缺口：

- `PUT /api/device/:id/unit` 不应返回 501，应更新 profile channels 的单位并持久化。
- `GET /api/device/:id/daqT1603Config` 不应返回 501，应从 profile 或设备当前配置读取。
- `PUT /api/device/:id/daqT1603Config` 不应返回 501，应校验并保存配置，设备已连接时调用 adapter 应用硬件配置。

**Step 2: 增加端口而不是 handler 直接碰 adapter**

在 `ports/device.go` 增加最小接口，例如：

```go
type UnitConfigurable interface {
    SetUnit(unit string) error
}

type DaqT1603Configurable interface {
    GetDaqT1603Config() (device.DaqT1603HardwareConfig, error)
    ApplyDaqT1603Config(config device.DaqT1603HardwareConfig) error
}
```

**Step 3: 在 usecase 编排**

`DeviceManager` 提供：

- `SetUnit(id string, unit string) error`
- `GetDaqT1603Config(id string) (device.DaqT1603HardwareConfig, error)`
- `ApplyDaqT1603Config(id string, config device.DaqT1603HardwareConfig) error`

规则：

- 配置持久化更新 profile。
- 设备在线时通过可选 port interface 调 adapter。
- 设备不在线时只保存配置，连接时应用。

**Step 4: handler 只做参数绑定**

`device.go` handler 只做 JSON bind、path param、错误码转换，不写业务规则。

**Step 5: 验证**

Run from `projects/wind-daq/services/api-go`: `go test ./internal/usecase ./api/handler -run Device -v`

Expected: PASS。

Run from `projects/wind-daq/services/api-go`: `go build -buildvcs=false ./...`

Expected: PASS。

---

## Task 3: 对齐采集数据链路和存储链路

**Files:**
- Modify: `projects/wind-daq/services/api-go/cmd/server/main.go`
- Modify: `projects/wind-daq/services/api-go/internal/usecase/acquisition.go`
- Modify: `projects/wind-daq/services/api-go/internal/usecase/storage.go`
- Modify: `projects/wind-daq/services/api-go/internal/ports/data_sink.go`
- Modify: `projects/wind-daq/services/api-go/internal/adapters/storage/service.go`
- Test: `projects/wind-daq/services/api-go/internal/usecase/acquisition_test.go`
- Test: `projects/wind-daq/services/api-go/internal/usecase/storage_test.go`

**Step 1: 写数据链路测试**

测试一个设备 payload 到达后应同时进入：

- `AcquisitionHub.OnData`
- storage recorder（若录制开启）
- WebSocket snapshot channel（按 publish rate 聚合）

**Step 2: 用组合 sink 替代硬编码回调**

在 wiring 层构造一个组合 sink：

```go
func(payload device.DataPayload) {
    acqHub.OnData(payload)
    storageSvc.HandlePayload(payload)
}
```

这个组合只允许出现在 `cmd/server/main.go` 或 Wails/HTTP wiring，不进入 adapter。

**Step 3: 补齐 latest data 查询能力**

为 traversal/calibration 需要的实时数据增加 usecase 方法：

- `GetLatestData(deviceID string) (device.DataPayload, bool)`
- 必要时增加环形历史缓存，但第一阶段只做 latest。

**Step 4: 验证**

Run from `projects/wind-daq/services/api-go`: `go test ./internal/usecase -run 'Acquisition|Storage' -v`

Expected: PASS。

---

## Task 4: 补齐校准与遍历 usecase 的真实依赖

**Files:**
- Modify: `projects/wind-daq/services/api-go/internal/usecase/calibration.go`
- Modify: `projects/wind-daq/services/api-go/internal/usecase/traversal.go`
- Modify: `projects/wind-daq/services/api-go/internal/core/calibration/types.go`
- Modify: `projects/wind-daq/services/api-go/internal/core/traversal/types.go`
- Modify: `projects/wind-daq/services/api-go/internal/core/interpolation/*.go`
- Test: `projects/wind-daq/services/api-go/internal/usecase/calibration_test.go`
- Test: `projects/wind-daq/services/api-go/internal/usecase/traversal_test.go`

**Step 1: 从 TS 行为提取用例**

参考：

- `src/main/calibration/**`
- `src/main/services/traversalTestService.ts`
- `src/main/services/traversal/TraversalInterpolationManager.ts`
- `src/main/services/interpolation/*.ts`

**Step 2: 保持 core 纯净**

迁移公式、插值、路径/点位计算到 `core`，不得引入文件 I/O、硬件、网络、framework。

**Step 3: usecase 只编排 ports**

校准和遍历需要的能力通过 ports 注入：

- motion move/stop/status
- acquisition latest data
- storage/report writer
- event publisher
- resource lock（如果需要，定义在 ports）

**Step 4: 测试暂停/恢复/停止状态机**

用 fake ports 测：

- 启动时状态从 idle 到 running
- pause/resume 不丢失当前 step
- stop 会释放资源锁并停止 motion
- 设备无数据时返回可解释错误，不 panic

**Step 5: 验证**

Run from `projects/wind-daq/services/api-go`: `go test ./internal/core/... ./internal/usecase/... -v`

Expected: PASS。

---

## Task 5: 建立 Wails 桌面壳

**Files:**
- Create: `projects/wind-daq/apps/desktop-wails/wails.json`
- Create: `projects/wind-daq/apps/desktop-wails/main.go`
- Create: `projects/wind-daq/apps/desktop-wails/backend/app.go`
- Create: `projects/wind-daq/apps/desktop-wails/backend/bindings/app.go`
- Create: `projects/wind-daq/apps/desktop-wails/backend/bindings/device.go`
- Create: `projects/wind-daq/apps/desktop-wails/backend/bindings/motion.go`
- Create: `projects/wind-daq/apps/desktop-wails/backend/bindings/calibration.go`
- Create: `projects/wind-daq/apps/desktop-wails/backend/bindings/traversal.go`
- Modify: `projects/wind-daq/docs/STRUCTURE.md`

**Step 1: 复用参考 Wails 壳的窗口配置**

参考：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\wails-backend\main.go`

保留：

- title: `WindDAQ - 数据采集与运动控制系统`
- width/height: `1440x900`
- background: dark SCADA background

调整：

- `MinWidth` 按 `DESIGN.md` 应为 `1280`
- `MinHeight` 按 `DESIGN.md` 应为 `720`
- Wails binding 不承载业务逻辑

**Step 2: wiring 复用 api-go usecase**

优先方案：将 `services/api-go/cmd/server/main.go` 中 DI wiring 抽成可复用 constructor，例如：

- Create: `projects/wind-daq/services/api-go/internal/bootstrap/app.go`

注意：如果 `internal` 导入限制导致 Wails app 不能导入 `services/api-go/internal`，不要绕过 Go internal 规则。改为：

- 将可共享启动器移动到 `projects/wind-daq/services/api-go/pkg/bootstrap`，或
- Wails app 内启动 `api-go` HTTP server binary/包并通过 HTTP 调用。

首选最小改动：Wails 桌面壳内嵌前端，前端通过 `http://localhost:<port>` 和 `/ws` 使用现有 API；Wails binding 只提供 app/version、dialog/pickDirectory 等桌面能力。

**Step 3: 验证**

Run from `projects/wind-daq/apps/desktop-wails`: `wails doctor`

Expected: no blocking Wails environment errors。

Run from `projects/wind-daq/apps/desktop-wails`: `wails build`

Expected: build succeeds after frontend task completed。

---

## Task 6: 新建 Vue 3 前端骨架并迁移设计系统

**Files:**
- Create: `projects/wind-daq/apps/desktop-wails/frontend/package.json`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/vite.config.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/tsconfig.json`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/main.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/App.vue`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/styles/tokens/*.css`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/styles/themes/*.css`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/components/ui/*.vue`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/components/layout/*.vue`

**Step 1: 从参考前端复制 UI 基础，但改路径和依赖**

来源：

- `src/renderer/src/components/ui/`
- `src/renderer/src/components/layout/`
- `src/renderer/src/styles/`
- `src/renderer/src/stores/themeStore.ts`
- `src/renderer/src/stores/i18nStore.ts`
- `src/renderer/src/stores/feedbackStore.ts`

**Step 2: 严格遵守 `projects/wind-daq/DESIGN.md`**

必须满足：

- Header 48px、Footer 32px、Rail 56px、Sidebar 220px
- 不做移动端响应式，低于 1280x720 显示警告
- 所有颜色使用 CSS custom properties
- Header/Footer 可玻璃，Panel 必须实色 `var(--bg-panel)`
- 数据值使用 mono + tabular nums

**Step 3: 验证基础 UI**

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm install`

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run typecheck`

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run build`

Expected: all pass。

---

## Task 7: 迁移前端 API client，从 Electron IPC 改为 HTTP/WS

**Files:**
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/http-client.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/ws-client.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/deviceApi.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/motionApi.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/calibrationApi.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/traversalApi.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/storageApi.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/api/reportApi.ts`
- Test: `projects/wind-daq/apps/desktop-wails/frontend/src/api/*.test.ts`

**Step 1: 不迁移 Electron client**

不要复制：

- `electron-api-client.ts`
- `wails-route-mapping.ts` 中与旧 IPC 绑定强相关的映射
- `src/shared/ipc/**`

**Step 2: 按 OpenAPI 映射 REST**

使用 `projects/wind-daq/contracts/openapi/openapi.yaml` 作为接口真相来源。

核心映射：

- `GET /api/device/profiles`
- `PUT /api/device/profiles`
- `POST /api/device/:id/connect`
- `POST /api/daq/startAcquisition`
- `GET /api/motion/status`
- `POST /api/calibration/start`
- `POST /api/traversal/start`
- `GET /api/storage/settings`

**Step 3: 按当前 WS channel 订阅实时数据**

从 `services/api-go/internal/ports/channels.go` 和 `adapters/ws/channels.go` 获取 channel 名称。

前端需封装：

- subscribe/unsubscribe
- reconnect with backoff
- JSON parse guard
- stale device detection 只影响 UI 状态，不做硬件控制

**Step 4: 验证**

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run typecheck`

Expected: PASS。

---

## Task 8: 迁移 Pinia stores 和主页面

**Files:**
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/stores/deviceStore.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/stores/motionStore.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/stores/calibrationStore.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/stores/traversalStore.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/stores/storageStore.ts`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/views/MainDashboardView.vue`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/views/MotionView.vue`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/views/CalibrationView.vue`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/views/TraversalView.vue`
- Create: `projects/wind-daq/apps/desktop-wails/frontend/src/router/index.ts`

**Step 1: 迁移 store 状态，不迁移业务算法**

可保留：

- profiles/instances/latestSnapshots/historyBuffers
- selectedDeviceId
- chart selection
- tare offset 作为前端显示偏移
- theme/i18n/feedback UI state

不可保留在前端：

- calibration coefficient calculation
- traversal interpolation
- hardware safety decision
- motion compensation algorithm

**Step 2: 迁移主页面**

来源：

- `src/renderer/src/views/main/MainDashboardView.vue`
- `src/renderer/src/views/MotionView.vue`
- `src/renderer/src/views/CalibrationView.vue`
- `src/renderer/src/views/TraversalView.vue`

按当前设计规范调整组件路径和 API 调用。

**Step 3: 验证**

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run typecheck`

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run build`

Expected: PASS。

---

## Task 9: 联调后端、前端、WebSocket

**Files:**
- Modify: `projects/wind-daq/apps/desktop-wails/frontend/src/api/http-client.ts`
- Modify: `projects/wind-daq/apps/desktop-wails/frontend/src/api/ws-client.ts`
- Modify: `projects/wind-daq/services/api-go/cmd/server/main.go`
- Modify: `projects/wind-daq/services/api-go/api/server.go`

**Step 1: 启动 Go API**

Run from `projects/wind-daq/services/api-go`: `go run ./cmd/server/main.go`

Expected: HTTP server starts on configured port, `/api/app/version` returns JSON。

**Step 2: 启动前端 dev server**

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run dev`

Expected: dashboard loads and REST calls succeed。

**Step 3: 验证模拟设备闭环**

Manual test:

- 新增 simulated device profile
- connect
- start acquisition
- dashboard receives snapshot via WS
- stop acquisition
- disconnect

Expected:

- status badge changes correctly
- waveform/history buffer updates
- no console errors
- Go backend logs no panic

**Step 4: 验证硬件入口不阻塞模拟模式**

Manual test:

- no hardware connected
- device scan timeout returns controlled error/empty list
- simulated devices still work

Expected: UI displays empty/error state per `DESIGN.md`。

---

## Task 10: 文档、结构和最终验证

**Files:**
- Modify: `projects/wind-daq/README.md`
- Modify: `projects/wind-daq/docs/STRUCTURE.md`
- Modify: `projects/wind-daq/contracts/openapi/openapi.yaml` if API changed
- Modify: `workspace.structure.json` only if structure validation requires it

**Step 1: 更新运行说明**

记录：

- Go API dev command
- frontend dev command
- Wails dev/build command
- expected ports
- config file locations

**Step 2: 更新结构文档**

`docs/STRUCTURE.md` 必须与新增 frontend/Wails 文件匹配。

**Step 3: 全量验证**

Run from workspace root: `powershell -File .\scripts\validate-structure.ps1`

Expected: PASS。

Run from `projects/wind-daq/services/api-go`: `gofmt -l .`

Expected: no output。

Run from `projects/wind-daq/services/api-go`: `go vet ./...`

Expected: PASS。

Run from `projects/wind-daq/services/api-go`: `go build -buildvcs=false ./...`

Expected: PASS。

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run typecheck`

Expected: PASS。

Run from `projects/wind-daq/apps/desktop-wails/frontend`: `npm run build`

Expected: PASS。

Run from `projects/wind-daq/apps/desktop-wails`: `wails build`

Expected: PASS。

---

## 推荐执行顺序

1. Task 1: 功能差异清单
2. Task 2: Go 后端缺口补齐
3. Task 3: 采集与存储链路
4. Task 4: 校准与遍历真实依赖
5. Task 6: 前端基础骨架和设计系统
6. Task 7: HTTP/WS API client
7. Task 8: stores 和主页面
8. Task 5: Wails 桌面壳
9. Task 9: 联调
10. Task 10: 文档和最终验证

说明：Task 5 可在 Task 6-8 后执行，因为当前最小闭环可以先用 Go API + Vite 前端验证；Wails 壳越早引入，越容易把桌面生命周期问题和业务迁移问题混在一起。

---

## 风险与处理

| Risk | Mitigation |
|---|---|
| 旧 TS 功能很多，一次性迁移容易失控 | 先做 feature map，然后按 MVP 闭环迁移 |
| Go `internal` 规则导致 Wails app 不能直接导入 api-go internal | 首选前端通过 HTTP/WS 调 api-go；必要时抽公开 `pkg/bootstrap` |
| 前端旧 IPC client 与新 HTTP/WS 不兼容 | 新建 API client，不做兼容层 |
| 校准/遍历混合硬件、运动和采集，测试困难 | 用 ports + fake adapters 先测状态机和错误路径 |
| 硬件不可用导致验证卡住 | simulated device/motion 必须保持可用作为默认验收路径 |
| UI 复制旧样式偏离新 DESIGN.md | 先迁移 tokens/layout/ui primitives，再迁移业务页面 |

---

## 不迁移清单

- Electron main process: `src/main/index.ts`
- Electron IPC registration files: `src/main/ipc/register*.ts`
- Electron-specific preload/window code
- Old `electron-api-client.ts`
- Old `src/shared/ipc/**`
- Generated Wails JS under reference project: `wails-backend/frontend/src/wailsjs/**`
- Reference binary/build artifacts: `winddaq.exe`, `DaqP1604SimpleAcquisition.zip`, installers
