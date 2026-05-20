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
- ⬜ 重建真实 DAQ hardware adapter。
  范围：DAQ-P-1604、DAQ-T-1603，按 Go ports 重新实现，不复制旧 TS service 结构。
  验证：protocol-level unit tests + simulated/no-hardware tests；真实硬件验证另列 HIL。
- ⬜ 增加采集历史环形缓存。
  目标：支持图表、校准、遍历读取 recent samples，而不是只有 latest。
  验证：AcquisitionHub tests 覆盖容量、覆盖、并发读。
- ⬜ 增加 WebSocket 或 SSE 实时推送。
  目标：替换前端 250ms polling，支持 dashboard live stream。
  验证：API tests 或 integration test 覆盖 subscribe、publish、disconnect。
- ⬜ 实现 storage recording usecase。
  目标：采集 payload 可按配置写入文件/数据目录。
  验证：fake clock/temp dir tests，覆盖 start/stop/write/error。
- ⬜ 重建 motion core/usecase/ports。
  范围：connect、status、moveTo、moveBy、jog、home、stop、emergencyStop。
  验证：fake motion adapter tests 覆盖状态机和 emergency stop。
- ⬜ 重建 calibration core/usecase。
  范围：校准流程、暂停/恢复/停止、状态查询、数据采集依赖。
  验证：fake acquisition/motion/storage ports tests。
- ⬜ 重建 traversal core/usecase。
  范围：点位遍历、插值、进度、暂停/恢复/停止、数据落盘入口。
  验证：状态机 tests + interpolation pure core tests。
- ⬜ 重建 report generation usecase。
  目标：基于 storage/calibration/traversal 结果生成报告入口。
  验证：temp dir tests，覆盖输出文件和错误路径。

### 前端后续计划

- ⬜ 将当前单文件 Dashboard 拆分为 layout、views、api、components。
  目标：为后续 Motion/Calibration/Traversal 页面扩展做结构准备。
  验证：`npm run typecheck`、`npm run build`。
- ⬜ 引入前端路由。
  页面：Dashboard、Devices、Motion、Calibration、Traversal、Storage/Reports、Settings。
  验证：build passed，主要导航可人工点击。
- ⬜ 引入状态管理。
  目标：保留 UI state、profile selection、latest snapshots；不放硬件算法和校准算法。
  验证：typecheck + store unit tests（如测试框架加入）。
- ⬜ 实现 Devices 页面。
  功能：scan、profile list、create/edit、connect/disconnect、status。
  验证：against simulated API manual flow。
- ⬜ 实现 realtime chart。
  目标：展示采集曲线、通道开关、最新值、stale 状态。
  验证：simulated acquisition 运行 60s 无 console error。
- ⬜ 从 polling 切换到 WS/SSE client。
  目标：封装 reconnect、backoff、parse guard、unsubscribe。
  验证：断开/重连 manual test。
- ⬜ 实现 Motion 页面。
  功能：连接、状态、moveTo/moveBy/jog/home/stop/emergencyStop。
  验证：fake/simulated motion backend 联调。
- ⬜ 实现 Calibration 页面。
  功能：任务创建、启动、暂停、恢复、停止、状态、结果查看。
  验证：fake backend flow + typecheck/build。
- ⬜ 实现 Traversal 页面。
  功能：路径/点位配置、启动、暂停、恢复、停止、进度、结果入口。
  验证：fake backend flow + typecheck/build。
- ⬜ 实现 Storage/Reports 页面。
  功能：存储配置、录制状态、报告生成入口、输出路径提示。
  验证：manual flow + build。
- ⬜ 补充空状态、错误状态、加载状态。
  目标：无硬件、API 未启动、连接失败、采集失败都能给出清晰 UI。
  验证：manual checklist。
- ⬜ 增加前端测试基础。
  目标：至少覆盖 API client 和关键 store；E2E 后续再加。
  验证：新增 test command passed。

### 桌面与联调后续计划

- ⬜ 明确 Wails 与 Go API 的运行关系。
  当前状态：Wails 只嵌入前端；Go API 需要单独运行。
  待决策：Wails 启动内嵌 API、启动 sidecar API，或保持外部 API dev 模式。
- ⬜ 在 Wails frontend 显示 `GetVersion()`。
  目标：验证 Wails binding 通道可用，但不把业务逻辑放进 desktop backend。
  验证：desktop app manual check。
- ⬜ 增加 desktop dev/build 文档。
  内容：API server、Vite dev、Wails dev、Wails build、端口和 env var。
  验证：新同事按 README 可启动 MVP。
- ⬜ 增加 integrated smoke test checklist。
  流程：启动 API、启动前端/Wails、创建 simulated profile、connect、start、观察数据、stop。
  验证：人工执行并在文档记录日期。
- ⬜ 真实硬件 HIL 验证计划。
  范围：设备型号、连接方式、测试数据、失败处理、日志采集。
  验证：单独 HIL runbook。

### 文档维护计划

- ⬜ 更新 `projects/wind-daq/README.md` 到当前 MVP 运行方式。
- ⬜ 更新 `projects/wind-daq/docs/STRUCTURE.md` 到实际目录结构。
- ⬜ 更新 `projects/wind-daq/docs/migration/ts-reference-feature-map.md`，把每个参考功能标成 Done/Partial/Missing/Do not migrate。
- ✅ 更新 `projects/wind-daq/contracts/openapi/openapi.yaml`，与当前 REST API 对齐。
- ⬜ 每完成一个后端/前端/桌面任务，同步更新本节勾选状态和验证证据。

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
