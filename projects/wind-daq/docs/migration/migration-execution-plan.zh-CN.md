# 执行计划：Wind-DAQ 迁移待完善收尾

## 概述

基于 `migration-improvement-backlog.zh-CN.md` 的待完善项和 P0/P1/P2 优先级，将 backlog 分解为 18 个可独立实现和验证的任务。每个任务大小控制在 S/M 级（1-5 个文件），确保单个 session 内可闭环。

当前基线：Go backend 测试通过、Frontend typecheck/build/test 通过、Wails 构建通过，但 structure validation 失败、feature map 滞后、simulated smoke 未正式记录。

## 架构决策

- **HIL 类任务不修改代码**：P1 真实硬件验证只在已有 adapter 上执行人工操作，不引入新代码（除非验证发现必须修的问题）。
- **Feature map 更新和结构修复不许代码修改**：纯文档和配置变更。
- **Calibration/Traversal 业务补齐必须按 vertical slice**：后端 usecase → API → 前端，每步可独立测试。
- **Motion B140/WTNMC4A 暂标为实验性**：如果当前真实 adapter 源码不存在或未编译通过，先标为后续版本，不阻塞 P0 验收。

## 执行状态（2026-05-21）

| Phase | 状态 | 说明 |
|---|---|---|
| Phase 0 基础设施修复 | ✅ 完成 | Task 1-3: 结构校验、feature map 同步、smoke 验证 |
| Phase 1 模拟验收闭环 | ✅ 完成 | Task 4-5: Storage 文案、SSE 确认集成 |
| Phase 2 真实硬件 HIL | ⬜ 部分完成 | Task 6-7 (DAQ HIL): 📋 待硬件; Task 8 (网络扫描): ✅ 已完成 |
| Phase 3 运动控制适配器 | 📋 标记后版本 | Task 9-10: B140/WTNMC4A 无现有文件，已标记 Do not migrate |
| Phase 4 业务工作流 | ✅ 完成 | Task 11-14: Calibration/Traversal 前后端完整 workflow |
| Phase 5 工程质量 | ✅ 完成 | Task 15-17: ECharts 优化、Wails sidecar、测试增强 |
| Phase 6 最终收敛 | ✅ 完成 | Task 18: 全量验证通过，文档收敛 |

---

## 任务列表

### Phase 0：基础设施修复与文档同步

#### Task 1：修复 workspace structure validation

**Description:** 根目录存在 `npm-cache/` 和 `services/` 导致 `validate-structure.ps1` 失败。判断这两个目录的来源，删除或纳入 `workspace.structure.json`，使结构校验通过。

**Acceptance criteria:**
- [ ] `npm-cache/` 已处理（删除或登记）
- [ ] `services/` 已处理（删除或登记）
- [ ] `powershell -File .\scripts\validate-structure.ps1` 输出 `passed`

**Verification:**
- [ ] 结构校验命令通过
- [ ] `go build ./...` 在 backend 仍通过（确认只动了无关文件）

**Dependencies:** None

**Files likely touched:**
- `workspace.structure.json`（若需要登记新目录）
- `docs/decisions/`（若需要补充说明）

**Estimated scope:** Small (1-2 files, no code change)

---

#### Task 2：同步 ts-reference-feature-map.md 与当前源码

**Description:** Feature map 将 `DAQ-P-1604 driver`、`DAQ-T-1603 driver`、`HTTP motion API`、`SSE realtime stream`、`Channel chart` 等标记为 Missing/Partial，但当前源码已有对应实现。逐项对照当前代码树，更新状态为 `Done`（有代码+测试）、`Partial`（有代码但缺 HIL）或保持 `Missing`（确认无实现）。每项 `Partial` 注明剩余工作。

**Acceptance criteria:**
- [ ] 所有已实现的 backend adapter/API 标为 `Done` 或 `Partial`（注明 HIL pending）
- [ ] 所有已实现的前端组件标为 `Done` 或 `Partial`
- [ ] Feature map 中无状态与当前 2026-05-21 源码明显矛盾

**Verification:**
- [ ] 人工通读 feature map，确认每项能关联到源码文件或测试

**Dependencies:** None（可以先于 Task 1 执行）

**Files likely touched:**
- `projects/wind-daq/docs/migration/ts-reference-feature-map.md`

**Estimated scope:** Small (1 file, doc-only)

---

#### Task 3：执行 integrated smoke checklist 并记录

**Description:** 按 `projects/wind-daq/docs/runbooks/integrated-smoke-checklist.md` 执行 simulated 全流程：启动 API → 启动前端 → 默认 profile → scan → connect → start acquisition → chart 收到数据 → stop → disconnect → storage status。记录每个步骤的截图、日志输出或 console 快照，更新 checklist 勾选状态。

**Acceptance criteria:**
- [ ] API server 启动并返回 `/api/app/version`（如有）
- [ ] Dashboard 显示默认 simulated profile
- [ ] Device scan 返回 simulated scanner 结果
- [ ] Connect → Start 后 realtime chart 刷新数据
- [ ] Stop → Disconnect 干净
- [ ] 无 console error，后端无 panic
- [ ] Storage/Report API 返回预期状态（即便未录制）

**Verification:**
- [ ] Smoke 截图保存在 `projects/wind-daq/build/smoke-*.png`
- [ ] Checklist 中 P0 项已勾选

**Dependencies:** Task 1（结构干净后可稳定运行）

**Files likely touched:**
- `projects/wind-daq/docs/runbooks/integrated-smoke-checklist.md`
- `projects/wind-daq/scripts/smoke-ui.py`（若需补充记录逻辑）

**Estimated scope:** Small (2 files, execution + doc update)

---

### Checkpoint：Phase 0 完成

- [ ] Structure validation 通过
- [ ] Feature map 与当前源码一致
- [ ] Integrated smoke checklist 中 P0 项通过并记录
- [ ] 确认当前基线（backend test/frontend test/build）仍全部通过

---

### Phase 1：模拟环境验收闭环

#### Task 4：复核 Storage/Report 前端真实 API 接入状态

**Description:** 当前 `StorageView.vue` 文案说"后端 API 已就绪但尚未接入 HTTP"，但后端 `api/server.go` 已有 `/api/storage/start|stop|status` 和 `/api/report/generate|status`。检查前端 `storageApi` 是否真的调用了这些端点的 HTTP 请求，还是仅使用本地模拟状态。若已接但文案过期，修正文案；若未接，补接 HTTP 调用并更新 UI 反馈（loading、error、success）。

**Acceptance criteria:**
- [ ] StorageView 开始录制按钮调用 `POST /api/storage/start`
- [ ] StorageView 停止录制按钮调用 `POST /api/storage/stop`
- [ ] 录制状态从 `/api/storage/status` 读取
- [ ] 错误时展示 error message
- [ ] 文案不再说"尚未接入 HTTP"

**Verification:**
- [ ] `npm run typecheck && npm run build` 通过
- [ ] `npm run test` 通过
- [ ] 手动在前端点录制按钮后 console 看到真实 HTTP 请求

**Dependencies:** Task 3（smoke 后发现文案问题）

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/views/StorageView.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/deviceApi.ts`（或新建 `storageApi.ts`）
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/__tests__/`

**Estimated scope:** Medium (3-4 files)

---

#### Task 5：补齐前端 SSE stream 实际接入 Dashboard

**Description:** 当前 `sse-client.ts` 存在，`RealtimeChart.vue` 存在，但 backend SSE 端点 `GET /api/daq/stream/:id` 也有实现。检查 Dashboard 的 DeviceDetailPanel 是否已订阅 SSE 实时流。若仅使用 polling，切换为 SSE 订阅；若已使用 SSE，确认 reconnect/error 处理。

**Acceptance criteria:**
- [ ] Dashboard 使用 SSE 接收实时数据（而非仅 polling）
- [ ] SSE 断线 5s 内自动重连
- [ ] 重连后 chart 不出现重复或断裂数据
- [ ] 后端停止采集后 SSE 正确关闭

**Verification:**
- [ ] `npm run typecheck && npm run build` 通过
- [ ] 浏览器 Network tab 可见 `/api/daq/stream/:id` event-stream
- [ ] 手动停止 API 后前端显示连接断开提示，API 恢复后自动重连

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/sse-client.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/deviceStore.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceDetailPanel.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/RealtimeChart.vue`

**Estimated scope:** Medium (3-5 files)

---

### Checkpoint：Phase 1 完成

- [ ] Storage/Report 前端已接真实 API 或明确记录了不接的原因
- [ ] SSE 实时数据流在前端实际工作
- [ ] `npm run typecheck && npm run build && npm run test` 全部通过

---

### Phase 2：真实硬件适配器验收（HIL）

> 以下任务依赖真实硬件可用性，无法纯代码完成。每个任务先确认 adapter 代码是否存在，再执行 HIL 验证。若硬件不可用，将任务标记为 Pending 并记录在 feature map。

#### Task 6：DAQ-P-1604 HIL 验证

**Description:** 在真实 DAQ-P-1604 设备上执行：connect → status → start acquisition → read data frames → stop → disconnect。验证 ASCII frame 解析、单位设置、串口参数一致性。将结果（通过/失败/异常）记录到 `hil-validation-plan.md`。

**Acceptance criteria:**
- [ ] 串口连接成功，status 返回 connected
- [ ] Start 后收到连续数据帧
- [ ] 数据帧解析出合理的通道值（不全是 NaN 或 0）
- [ ] Stop 后数据停止，status 变为 stopped
- [ ] 断线/超时后 adapter 返回 error 状态而非 panic

**Verification:**
- [ ] `go test ./internal/adapters/hardware -run DAQP1604 -v` 通过（如有硬件测试）
- [ ] HIL plan 记录 connect/start/stop/disconnect 时间戳和数据样本

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/docs/runbooks/hil-validation-plan.md`
- `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1604.go`（如果验证发现 bug）

**Estimated scope:** Small-Medium（取决于验证是否发现 bug，HIL 执行本身无代码变更）

---

#### Task 7：DAQ-T-1603 HIL 验证

**Description:** 与 Task 6 相同模式，在真实 DAQ-T-1603 上验证：connect → apply thermocouple config → start → read temp data → stop。重点验证 Thermocouple Type / Cold Junction / Filter Hz 配置写入和后读取。

**Acceptance criteria:**
- [ ] 配置写入后 `GET /api/device/:id/daqT1603Config` 返回预期值
- [ ] Start 后温度通道持续刷新
- [ ] 断偶/超量程时前端显示合理错误（非 NaN）
- [ ] Config 持久化后重启 profile 恢复上次配置

**Verification:**
- [ ] HIL plan 记录所有配置组合的读/写验证
- [ ] 后端日志无 panic

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/docs/runbooks/hil-validation-plan.md`
- `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_t1603.go`（如果验证发现 bug）

**Estimated scope:** Small-Medium

---

#### Task 8：真实网络扫描 adapter 实现

**Description:** 实现 UDP broadcast / network scanner，能发现局域网内的 DAQ-P-1604 和 DAQ-T-1603 设备。遵循现有 `ports.DeviceScanner` 接口，产出 `device.ScanResult` 列表。扫描超时返回空列表而非错误。

**Acceptance criteria:**
- [ ] `ports.DeviceScanner` 新实现在 `adapters/scan/network_scanner.go`
- [ ] 扫描超时（默认 5s）返回空列表，不报错
- [ ] 模拟 scanner 仍可用并保持默认
- [ ] 扫描结果可被 `bootstrap` 替换注入

**Verification:**
- [ ] `go test ./internal/adapters/scan -v` 通过
- [ ] `go test ./... -v` 仍通过（不影响已有测试）
- [ ] `GET /api/device/scan` 在有设备的网络环境返回结果

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/adapters/scan/network_scanner.go`
- `projects/wind-daq/services/api-go/internal/adapters/scan/network_scanner_test.go`
- `projects/wind-daq/services/api-go/internal/adapters/scan/udp_scanner.go`（若复用）
- `projects/wind-daq/services/api-go/internal/adapters/scan/daq_p1604_scanner.go`（若复用）
- `projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go`（可选：接入真实 scanner）

**Estimated scope:** Medium (3-5 files)

---

### Checkpoint：Phase 2 完成

- [ ] DAQ-P-1604 HIL 已验证并记录（或标记 Pending + 原因）
- [ ] DAQ-T-1603 HIL 已验证并记录（或标记 Pending + 原因）
- [ ] 网络扫描 adapter 已实现且 `go test` 通过
- [ ] Feature map 已同步 HIL 结果

---

### Phase 3：运动控制真实适配器（实验性质）

#### Task 9：B140 adapter 实现与验证

**Description:** 检查 `adapters/hardware/b140.go` 是否存在以及是否实现 `ports.MotionController`。若适配器代码已存在，执行 HIL 验证：connect → status → home → moveTo → stop → emergencyStop → disconnect。若代码不存在或严重不完整，标记为 Do not migrate（当前版本范围外）并关闭。

**Approach:** 先 grep 确认 `b140.go` 的内容和接口实现情况，再做决策。

**Acceptance criteria:**
- [ ] B140 adapter 实现了 `ports.MotionController`（或明确标记未实现）
- [ ] Connect/status/home/moveTo 在真实 B140 上验证通过（或标记 Pending）
- [ ] EmergencyStop 优先于普通运动命令

**Verification:**
- [ ] `go build ./...` 在有 B140 adapter 文件的情况下通过
- [ ] HIL 记录连接参数、运动结果、急停响应时间

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/adapters/hardware/b140.go`
- `projects/wind-daq/services/api-go/internal/adapters/hardware/b140_test.go`
- `projects/wind-daq/docs/migration/ts-reference-feature-map.md`

**Estimated scope:** Medium (2-3 files, 主要是确认现有代码 + HIL)

---

#### Task 10：WTNMC4A adapter 实现 stub

**Description:** 检查 `adapters/hardware/wtnmc4a.go` 是否存在。WTNMC4A 需要 Windows FFI（`shared/device-sdk/go/ffi/wtnmc4a.go`）。若 FFI 函数需要真实 DLL 才能编译通过，检查是否有 stub/build tag 方案。如果不是阻塞项，标记为"后续版本"，不阻断当前验收。

**Acceptance criteria:**
- [ ] WTNMC4A 状态在 feature map 中明确标记为 `Do not migrate（后续版本）` 或 `Partial + HIL pending`
- [ ] 若当前代码已有机编译 `//go:build windows` tag，不做非必要修改

**Verification:**
- [ ] `go build ./...` 在 windows + 非 windows 上均通过（如有 build tags）

**Dependencies:** Task 9

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/adapters/hardware/wtnmc4a.go`
- `projects/wind-daq/docs/migration/ts-reference-feature-map.md`

**Estimated scope:** Small (1-2 files, 主要是标记决策)

---

### Checkpoint：Phase 3 完成

- [ ] B140 已验证或明确标记
- [ ] WTNMC4A 状态已决策
- [ ] Feature map 同步了 motion 状态

---

### Phase 4：业务工作流完善

#### Task 11：Calibration 后端真实数据管道

**Description:** 当前 `usecase/calibration.go` 已有 Start/Pause/Resume/Stop 状态机，`core/calibration/types.go` 有领域类型。但 workflow 在执行 `CollectCurrentPoint` 时未真正从 `acquisition hub` 读取稳定数据点。补充：从 `ports.LatestDataReader` 读取 N 个样本，计算平均值作为当前 pressure point 的校准数据，保存到内存存储。新增 `ports.CalibrationResultStore` 接口和 `adapters/calibration/memory_store.go`。

**Acceptance criteria:**
- [ ] `CollectCurrentPoint` 从 acquisition hub 读取 `averageSamples` 个数据并计算平均
- [ ] 采集完成时数据点被追加到 `CalibrationResultStore`
- [ ] Workflow 完成时结果可通过 API 查询
- [ ] Start 时校验 DeviceID 对应的 device 是否在采集状态

**Verification:**
- [ ] `go test ./internal/usecase -run Calibration -v` 通过
- [ ] `go test ./internal/adapters/calibration -v` 通过（如果新增）
- [ ] `POST /api/calibration/start` + `POST /api/calibration/collect` + `GET /api/calibration/status` 可工作

**Dependencies:** Task 3（基线稳定后）

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/calibration.go`
- `projects/wind-daq/services/api-go/internal/ports/calibration.go`
- `projects/wind-daq/services/api-go/internal/adapters/calibration/memory_store.go`
- `projects/wind-daq/services/api-go/internal/adapters/calibration/memory_store_test.go`
- `projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go`（wire 新 adapter）
- `projects/wind-daq/services/api-go/api/server.go`（也许需要校准结果 API）

**Estimated scope:** Medium (4-6 files)

---

#### Task 12：Calibration 前端 workflow 联动

**Description:** 在现有 `CalibrationView.vue` 基础上增加真实的 workflow step 交互：选择 probe type → 设置 pressure points → start → 观察当前 point 进度 → collect → 自动进入下一 point → complete 时显示结果。使用现有的 calibration API client。

**Acceptance criteria:**
- [ ] 用户可配置 pressure points 列表
- [ ] Start 后显示当前 point (e.g. "Point 3/10")
- [ ] Collect 后自动进入下一 point
- [ ] 全部完成后显示"校准完成"状态
- [ ] 错误时显示 error text

**Verification:**
- [ ] `npm run typecheck && npm run build` 通过
- [ ] 手动流程：创建 simulated device → connect → start acquisition → 进入 calibration 页面 → 执行步骤

**Dependencies:** Task 11

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/views/CalibrationView.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/calibrationApi.ts`（如果缺少）
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/calibrationStore.ts`（如果缺少）

**Estimated scope:** Medium (2-3 files)

---

#### Task 13：Traversal 后端采集 + 运动联动

**Description:** 当前 `usecase/traversal.go` 的 `RunCurrentPoint` 执行 motion moveTo，但到位后未从 acquisition hub 读取数据。补充：moveTo 完成后调用 `ports.LatestDataReader` 读取当前数据，写入 `ports.TraversalPointSink`。新增 path progress 持久化以支持 pause/resume 恢复。

**Acceptance criteria:**
- [ ] `RunCurrentPoint` 执行顺序：moveTo → wait for position reached → read data → write to sink → advance to next point
- [ ] Pause 在任意 step 后生效，不丢失当前 point
- [ ] Resume 继续未完成的 point
- [ ] Stop 释放 motion 并标记任务为 stopped

**Verification:**
- [ ] `go test ./internal/usecase -run Traversal -v` 通过
- [ ] fake motion + fake acquisition 自动化测试覆盖完整路径

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/traversal.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_test.go`
- `projects/wind-daq/services/api-go/internal/core/traversal/types.go`（若 progress 字段不足）

**Estimated scope:** Medium (2-3 files)

---

#### Task 14：Traversal 前端路径规划和可视化

**Description:** 在 `TraversalView.vue` 中增加路径规划 UI：选择扫描区域点、设置 step size、预览 interpolation 结果。后端 `core/traversal/path.go` 已有 `InterpolateLinearPath`，前端调用 API 或直接调用（如果纯计算无副作用）。可视化方面增加简单的 2D 网格/路径线 canvas 或 SVG。

**Acceptance criteria:**
- [ ] 用户可输入 3+ 个路径点并预览插值路径
- [ ] 支持 HIL / Layout / PRB 三种模式选择（UI 级别）
- [ ] Start 后显示当前到达点和已采集数据
- [ ] Pause/Resume/Stop 反映在可视化上

**Verification:**
- [ ] `npm run typecheck && npm run build` 通过
- [ ] 手动 simulated 流程可跑通

**Dependencies:** Task 13

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/views/TraversalView.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/PathPlanner.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/TraversalCanvas.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/traversalStore.ts`

**Estimated scope:** Medium-Large (4-6 files, 但主要是 Vue 组件)

---

### Checkpoint：Phase 4 完成

- [ ] Calibration 后端 workflow 通过自动化测试
- [ ] Calibration 前端 workflow 联动通过 typecheck/build
- [ ] Traversal 后端运动+采集联动通过自动化测试
- [ ] Traversal 前端路径规划 UI 通过 typecheck/build
- [ ] `go test ./...` 全部通过
- [ ] `npm run typecheck && npm run build && npm run test` 全部通过

---

### Phase 5：工程质量与体验

#### Task 15：ECharts code splitting 优化

**Description:** 当前 `npm run build` 有 ECharts chunk > 500 KB warning。确认前端路由级 code splitting 已生效（`LogViewer`、`StorageView` 等被动态 import 分开打包）。ECharts 因为 `RealtimeChart` 在 Dashboard 直接 import，无法拆到独立 chunk。选项 A：对 `vue-echarts` 做动态 import + 分包；选项 B：接受当前体积并在 build 配置中提高 chunkSizeWarningLimit。

**Acceptance criteria:**
- [ ] 或 `npm run build` 不再输出 chunk size warning
- [ ] 或 `vite.config.ts` 显式设置 `chunkSizeWarningLimit` 并在 README 中记录

**Verification:**
- [ ] `npm run build` 输出中无 warning（或明确标记已知）

**Dependencies:** Phase 0-1 完成后

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/vite.config.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/RealtimeChart.vue`（若做动态 import）

**Estimated scope:** Small (1-2 files)

---

#### Task 16：Wails 生产 API sidecar 策略

**Description:** 明确并实现桌面应用双击启动时 API server 的自动启动策略。方案 A：Wails backend 在 `startup` 钩子中以 goroutine 启动 `bootstrap.BuildAPIServer` 的 HTTP server，随窗口关闭而 shutdown。方案 B：保持分离模式，在文档中明确要求用户手动启动 API server。推荐方案 A（用户无额外操作）。

**Acceptance criteria:**
- [ ] `wind-daq.exe` 双击启动后 API 自动可用，无需用户手动执行 `go run`
- [ ] 应用窗口关闭后 API 进程自动退出
- [ ] 端口冲突时弹出错误提示而非静默失败
- [ ] 文档（README）同步说明

**Verification:**
- [ ] `wails build` 通过并生成新 exe
- [ ] 双击 exe 后前端可直接操作，无需手动启动 API
- [ ] 关闭窗口后 `netstat -ano | findstr :8080` 无残留

**Dependencies:** Task 3（需要一个干净的模拟验收基线）

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/main.go`
- `projects/wind-daq/apps/desktop-wails/backend/app.go`
- `projects/wind-daq/apps/desktop-wails/backend/bindings/app.go`
- `projects/wind-daq/docs/runbooks/wails-api-run-mode.md`
- `projects/wind-daq/README.md`

**Estimated scope:** Medium (3-5 files)

---

#### Task 17：前端测试覆盖增强

**Description:** 补充前端单元测试覆盖当前缺失的关键文件和路径：API client error mapping、SSE reconnect/parse error、DeviceManagementDrawer CRUD、MotionView/CalibrationView/TraversalView 的 empty/error 和正常状态。

**Acceptance criteria:**
- [ ] 新增测试覆盖 `http-client.ts` 的 error response 映射
- [ ] 新增测试覆盖 `sse-client.ts` 的 reconnect 和 parse error
- [ ] 新增 DeviceManagementDrawer mock API 集成测试
- [ ] 现有 26 tests 不退化

**Verification:**
- [ ] `npm run test` 输出测试计数大于 26
- [ ] `npm run typecheck && npm run build` 通过

**Dependencies:** Phase 0-1 完成后（测试环境稳定）

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/__tests__/http-client.test.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/__tests__/sse-client.test.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/__tests__/DeviceManagementDrawer.test.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/views/__tests__/MotionView.test.ts`
- `projects/wind-daq/apps/desktop-wails/frontend/src/views/__tests__/CalibrationView.test.ts`

**Estimated scope:** Medium (4-5 test files)

---

### Checkpoint：Phase 5 完成

- [ ] Build 无 chunk size warning（或已记录接受）
- [ ] Wails sidecar 策略实现并验收通过
- [ ] 前端新增 5+ 测试用例
- [ ] `npm run test` 全部通过

---

### Phase 6：最终收敛

#### Task 18：全量验证与文档收敛

**Description:** 执行最终的全量验证集合：structure validation → backend tests/vet/build → frontend typecheck/tests/build → Wails build → integrated smoke checklist → feature map 一致性检查。修正文档中任何过期或矛盾的描述。

**Acceptance criteria:**
- [ ] `powershell -File .\scripts\validate-structure.ps1` 通过
- [ ] backend `go test ./... && go vet ./... && go build ./...` 全部通过
- [ ] frontend `npm run test && npm run typecheck && npm run build` 全部通过
- [ ] `wails build` 通过
- [ ] `integrated-smoke-checklist.md` 主流程全部勾选
- [ ] `ts-reference-feature-map.md` 全部项与源码和 HIL 结果一致
- [ ] `README.md` 的 Quick Start 与当前实际一致

**Verification:**
- [ ] 全量验证命令输出无错误
- [ ] 所有文档引用与实际文件路径一致

**Dependencies:** Task 1-17 全部完成

**Files likely touched:**
- `projects/wind-daq/README.md`
- `projects/wind-daq/docs/STRUCTURE.md`
- `projects/wind-daq/docs/migration/ts-reference-feature-map.md`
- `projects/wind-daq/docs/runbooks/integrated-smoke-checklist.md`
- `projects/wind-daq/docs/runbooks/hil-validation-plan.md`
- `docs/plans/2026-05-20-wind-daq-ts-migration.md`

**Estimated scope:** Medium (4-6 文档文件)

---

## 风险与应对

| 风险 | 影响 | 应对 |
|---|---|---|
| DAQ-P-1604 / DAQ-T-1603 真实硬件不可用或已退役 | Phase 2 阻塞 | HIL 任务标记为 Pending，feature map 标 `Partial (HIL pending)`，不阻塞 P0/P2 验收 |
| B140/WTNMC4A adapter 代码不完整或编译不过 | Phase 3 阻塞 | 评估后直接标记 `Do not migrate（后续版本）`，清理空间 |
| 前端 Storage/Report 接入后发现 API 返回格式不匹配 | Task 4 需修后端 | 修后端 API 格式 + 前端对齐，编入同一 task |
| Wails sidecar 方案 A 与现有 `bootstrap.BuildAPIServer` 冲突 | Task 16 | 回退方案 B（分离模式），文档明确要求手动启动 |
| 校准/遍历在 simulated 环境与真实硬件表现差异大 | Task 11-14 | 用 fake 硬件测试覆盖状态机逻辑；真实差异在 HIL 阶段补测 |

## 执行顺序建议

```
Phase 0 ─── Task 1 → Task 2 → Task 3
                │
Phase 1 ─── Task 4 → Task 5
                │
Phase 2 ─── Task 6 → Task 7 → Task 8
                │
Phase 3 ─── Task 9 → Task 10
                │
Phase 4 ─── Task 11 → Task 12 → Task 13 → Task 14
                │
Phase 5 ─── Task 15 → Task 16 → Task 17
                │
Phase 6 ─── Task 18
```

每个 phase 之间可独立 review 和 checkpoint。Phase 0 必须先完成；Phase 1-5 之间无严格前后依赖（但第 6-14 的 HIL 建议在模拟验收通过后进行）。

## 待确认问题

- B140 和 WTNMC4A 当前是否有真实 adapter 源码？（需要 grep 确认实际内容，不是仅看文件名存在）
- `npm-cache/` 和 `services/` 是 CI 产物还是手动误放？（决定删除还是登记）
- Wails sidecar 方案选 A（内嵌 API）还是 B（分离）？（推荐 A，但需要确认 `bootstrap` 在 desktop 模块可导入）
- SSE 目前前端是 polling + SSE 双轨还是仅 polling？（决定 Task 5 实际工作量）
