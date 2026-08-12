# Tasks: 遍历测试"设备停止采集"处理流程

> 关联：[spec-traversal-acquisition-stop.md](./spec-traversal-acquisition-stop.md) | [plan-traversal-acquisition-stop.md](./plan-traversal-acquisition-stop.md)
> 状态：待实施，批准后进入 BUILD
> 日期：2026-08-10

## Phase 1: 后端端口与状态模型

### Task 1: `ports.AcquisitionController` 新增 `AcquisitionStatus` 快照

**Description:** 在 `internal/ports/device.go` 的 `AcquisitionController` 接口新增 `AcquisitionStatus(id string) AcquisitionStatus`，并定义 `AcquisitionState`（`AcquisitionAcquiring/AcquisitionStopped/AcquisitionReconnectRequired`）与 `AcquisitionStatus`（State/Name/LastError）类型。保留既有 `IsConnected/IsAcquiring/StartAcquisition/DeviceName` 方法。

**Acceptance criteria:**
- [ ] `AcquisitionController` 新增 `AcquisitionStatus(id string) AcquisitionStatus`，既有方法签名不变
- [ ] `AcquisitionStatus.LastError` 仅 `ReconnectRequired` 且设备仍在 map 时有值，其余为空
- [ ] `ports` 包零实现（六边形边界保持）

**Verification:**
- [ ] `go build ./internal/ports/` 通过

**Dependencies:** None

**Files likely touched:**
- `services/api-go/internal/ports/device.go`

**Estimated scope:** S

---

### Task 2: `DeviceManager` 实现三态映射快照

**Description:** 在 `internal/usecase/device_manager.go` 实现 `AcquisitionStatus(id)`：一次 `GetStatus(id)`（锁内读取）按 spec 判定顺序映射——

```
if !ok || Connection==Error || Connection==Disconnected  → ReconnectRequired
if Acquiring                                            → Acquiring
else                                                    → Stopped
```

`Name` 复用 `DeviceName(id)`（fallback id）；`LastError` 取自 `status.LastError`。

**Acceptance criteria:**
- [ ] `ok==false` / `Connection==Error` / `Disconnected` → `ReconnectRequired`
- [ ] `Acquiring==true`（任意 Connection）→ `Acquiring`
- [ ] `Connection∈{Connected,Acquiring} && !Acquiring` → `Stopped`
- [ ] `Connection==Error && Acquiring==true` → `ReconnectRequired`（Error 优先）
- [ ] `Connection==Acquiring && !Acquiring` → `Stopped`
- [ ] 单次 `GetStatus` 调用构建完整快照，无二次查询

**Verification:**
- [ ] 新增 `TestDeviceManager_AcquisitionStatus_*` 覆盖上表各组合
- [ ] `go test ./internal/usecase/ -run DeviceManager`

**Dependencies:** Task 1

**Files likely touched:**
- `services/api-go/internal/usecase/device_manager.go`
- `services/api-go/internal/usecase/device_manager_test.go`

**Estimated scope:** S

---

### Task 3: `traversal.Status` 与 `PointPhase` 新增等待字段/阶段

**Description:** 在 `internal/core/traversal/types.go`：`PointPhase` 新增 `PhaseWaitingForAcquisition = "waiting_acquisition"`；`Status` 新增 `WaitingForAcquisition bool`、`WaitingDevices []AcquisitionDeviceStatus`、`WaitingForAcquisitionSinceMs int64`；新增 `AcquisitionDeviceStatus{Name,State,SinceMs}` 类型。

**Acceptance criteria:**
- [ ] 新增字段均有 `omitempty`，旧 JSON 反序列化不破坏
- [ ] `AcquisitionDeviceStatus.State` 取值 `"acquiring"|"stopped"|"reconnect_required"`
- [ ] `waiting_acquisition` 作为 `PointPhase` 合法值，不影响 `isSubState` 判定

**Verification:**
- [ ] `go test ./internal/core/traversal/`
- [ ] `go vet ./internal/core/...`

**Dependencies:** None

**Files likely touched:**
- `services/api-go/internal/core/traversal/types.go`
- `services/api-go/internal/core/traversal/types_test.go`（如存在）

**Estimated scope:** S

---

### Task 4: 更新全部 `AcquisitionController` mock

**Description:** 所有实现 `ports.AcquisitionController` 的测试 mock 补 `AcquisitionStatus(id)` 方法：`traversal_acquisition_test.go` 的 `resumableAcquisitionController`、`traversal_view_test.go` 的 `mockAcquisitionController` 等。mock 需支持运行时切换三态，供等待/恢复/掉线测试使用。

**Acceptance criteria:**
- [ ] 编译通过：`go build ./...`
- [ ] 每个 mock 的 `AcquisitionStatus` 反映其当前 acquiring/connection 状态
- [ ] 既有 mock 行为（`IsAcquiring` 等）不变

**Verification:**
- [ ] `go test ./internal/usecase/` 编译通过（mock 更新后）

**Dependencies:** Task 1

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_acquisition_test.go`
- `services/api-go/internal/usecase/traversal_view_test.go`
- 其他实现该接口的测试文件

**Estimated scope:** M

## Phase 2: 后端等待逻辑

### Task 5: `traversal_devices.go` 三态分类 + 设备状态列表

**Description:** `referencedAcquisitionDevices` 改为基于 `AcquisitionStatus` 三态分类，返回按 deviceID 字典序排序的设备状态列表；`firstNonAcquiringDevice` 语义改为"第一个非 Acquiring 设备"（供文案主展示），按 `ReconnectRequired > Stopped > Acquiring` 优先级排序。

**Acceptance criteria:**
- [ ] 分类结果与 Task 2 映射一致
- [ ] 列表按 deviceID 稳定排序
- [ ] 主展示设备优先 `ReconnectRequired`

**Verification:**
- [ ] `go test ./internal/usecase/ -run TraversalDevice`

**Dependencies:** Task 1, 2

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_devices.go`

**Estimated scope:** S

---

### Task 6: 抽 `waitForAcquisitionResume` 公共等待 helper

**Description:** 在 `internal/usecase/traversal_acquisition.go` 抽取 `waitForAcquisitionResume(taskID string, groups []deviceChannelGroup, acquire func() []AcquisitionDeviceStatus) ([]AcquisitionDeviceStatus, error)`：

- 无限期等待，直到全部设备 `Acquiring` / Stop / Pause（与暂停分支同一循环模式，10ms tick + `pausedLoopIdle`）。
- 每次 tick 重新分类；等待期间写入 `Status.WaitingForAcquisition*` 字段（含 `SinceMs`，仅进入等待时设置一次），退出时清理。
- 恢复时由调用方清 `fresh/pending`（helper 不直接依赖采样内部状态）。
- 等待期间重置 `stallDeadline`；Pause → 冻结（语义等同暂停），Resume 后重新分类。
- 返回恢复后的设备状态列表供调用方继续。

**Acceptance criteria:**
- [ ] `STOPPED`/`RECONNECT_REQUIRED` 均无限期等待，长时间不恢复不返回错误
- [ ] Stop → 即时返回；Pause → 无限期；Resume 后重新分类
- [ ] 等待字段在进入/恢复/暂停各阶段正确写入/清理
- [ ] 移除 60s 累计逻辑，无 `acquisitionNotAcquiringTimeout` 引用

**Verification:**
- [ ] 新增 `TestWaitForAcquisitionResume_*` 单测（见 Task 10）
- [ ] `go test -race ./internal/usecase/ -run "Acquisition|Pause"`

**Dependencies:** Task 3, 4, 5

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_acquisition.go`

**Estimated scope:** M

---

### Task 7: `collectAveragedSamples` 接入等待 helper

**Description:** `collectAveragedSamples` 的 not-acquiring 分支改为调用 `waitForAcquisitionResume`；恢复后清 `fresh`/`pending`（保留 `lastTimestamps`）再继续采样。删除 `notAcquiringTotal` 累计、`notAcquiringLastSample` 与 60s 判定。

**Acceptance criteria:**
- [ ] 采样中设备停采/掉线 → 进入等待，恢复后从新帧继续
- [ ] 恢复后首个样本不含停采前旧帧（fresh/pending 已清）
- [ ] `acquisitionStallTimeout` 仍生效：`Acquiring` 但无新帧 10s 失败
- [ ] 旧 `TestCollectAveragedSamplesContiguousNotAcquiringStillFails` 语义反转 → 改为断言"无限期等待"

**Verification:**
- [ ] `go test ./internal/usecase/ -run CollectAveragedSamples`
- [ ] `go test -race ./internal/usecase/ -run "CollectAveragedSamples|Pause"`

**Dependencies:** Task 6

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_acquisition.go`
- `services/api-go/internal/usecase/traversal_acquisition_test.go`

**Estimated scope:** M

---

### Task 8: `RunCurrentPoint` 点位开始接入等待 + 删除 60s 常量

**Description:** `RunCurrentPoint` 的点位开始采集态检查改为调用 `waitForAcquisitionResume`（不再立即 `failWithCode`）；`internal/usecase/traversal.go` 删除 `acquisitionNotAcquiringTimeout` 常量及其注释。

**Acceptance criteria:**
- [ ] 点位开始设备停采/掉线 → 无限期等待，恢复后继续点位流程
- [ ] 等待时 `CurrentPointIndex` 不推进
- [ ] 启动阶段（`ParseAndStartTraversal`/`StartManaged`）仍拒绝未采集设备（既有行为不变）

**Verification:**
- [ ] `go test ./internal/usecase/ -run "RunCurrentPoint|StartManaged|ParseAndStart"`
- [ ] 全仓无 `acquisitionNotAcquiringTimeout` 引用

**Dependencies:** Task 6, 7

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_acquisition.go`
- `services/api-go/internal/usecase/traversal.go`
- `services/api-go/internal/usecase/traversal_config.go`（启动检查文案统一，如需要）

**Estimated scope:** S

## Phase 3: 前端贯通

### Task 9: 前端类型与 API 透传（legacy + dual）

**Description:** `shared/types/traversal.ts` 的 `TraversalTestStatus`/`TraversalProgressEvent` 新增等待字段与 `AcquisitionDeviceStatus` 类型；`api/traversalApi.ts` 的 `getStatus()`/`onProgress()` 透传；`stores/traversalStore.ts` 的 `applyProgressEvent()`/`syncRecoveredStatus()` 显式保留/合并等待字段；`stores/dualTraversalStore.ts`/`dualTraversalRuntime.ts`/`traversalPolling.ts` 同步贯通。

**Acceptance criteria:**
- [ ] `getStatus` 返回的等待字段能进入 `TraversalTestStatus`
- [ ] `onProgress` 事件的等待字段在 `applyProgressEvent` 重建后不丢失
- [ ] dual 路由（probe1/probe2）各自等待字段独立正确
- [ ] 类型与后端 JSON 契约一致

**Verification:**
- [ ] `npm run typecheck`
- [ ] `npm run test -- --run stores/traversalStore stores/dualTraversalStore`

**Dependencies:** Task 3（后端字段）

**Files likely touched:**
- `apps/desktop-wails/frontend/src/shared/types/traversal.ts`
- `apps/desktop-wails/frontend/src/api/traversalApi.ts`
- `apps/desktop-wails/frontend/src/stores/traversalStore.ts`
- `apps/desktop-wails/frontend/src/stores/dualTraversalStore.ts`
- `apps/desktop-wails/frontend/src/stores/dualTraversalRuntime.ts`
- `apps/desktop-wails/frontend/src/api/traversalPolling.ts`

**Estimated scope:** M

---

### Task 10: 等待横幅组件 + 文案

**Description:** 遍历页采集中组件在 `currentPointPhase == "waiting_acquisition"` 时显示等待横幅：主展示设备名 + "等 N 台设备" + 已等待时长；文案按 `state` 区分（`stopped` → "等待设备 X 恢复采集"；`reconnect_required` → "设备 X 已断开，请重新连接并启动采集"）。legacy 与 dual 各自实现/复用组件。`traversalErrorMapper.ts` + i18n 字典补充等待文案。

**Acceptance criteria:**
- [ ] legacy 与 dual 在等待态显示横幅
- [ ] 时长由 `WaitingForAcquisitionSinceMs` 计算，轮询节奏刷新
- [ ] 暂停时不显示横幅（暂停 UI 接管）
- [ ] 文案本地化（zh/en）

**Verification:**
- [ ] `npm run typecheck` + `npm run test`（组件单测）
- [ ] `npm run build`

**Dependencies:** Task 9

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/traversal/*`（采集中状态/横幅）
- `apps/desktop-wails/frontend/src/components/traversal/dual/*`
- `apps/desktop-wails/frontend/src/stores/i18nStore.ts`
- `apps/desktop-wails/frontend/src/api/traversalErrorMapper.ts`

**Estimated scope:** M

## Phase 4: 测试与验证

### Task 11: 后端确定性测试（fake clock / fake controller）

**Description:** 用可切换状态的 mock controller（Task 4）覆盖 spec §测试要点：

- 三态映射全组合
- `STOPPED` 长时间（>60s 模拟）不失败、恢复后继续
- `RECONNECT_REQUIRED` 同样无限期等待、重连后恢复
- 点位开始等待后继续（不再立即失败）
- 多设备聚合主展示优先级
- 等待中 Stop 即时退出、Pause 冻结/Resume 重新分类
- `fresh/pending` 恢复后重建（首个样本不含旧帧）
- `Status.WaitingForAcquisition*` 各阶段正确切换

**Acceptance criteria:**
- [ ] 上述每项至少一个断言测试
- [ ] `-race` 无数据竞争

**Verification:**
- [ ] `go test -race ./internal/usecase/ -run "Acquisition|Pause|RunCurrentPoint|CollectAveragedSamples"`
- [ ] `go test ./internal/... ./api/...`

**Dependencies:** Task 4-8

**Files likely touched:**
- `services/api-go/internal/usecase/traversal_acquisition_test.go`
- `services/api-go/internal/usecase/traversal_bound_controllers_test.go`
- 其他用例文件

**Estimated scope:** L

---

### Task 12: 前端单测 + 全量验证

**Description:** 为 store 透传、等待横幅组件（legacy + dual）补充单测；运行全量验证命令。

**Acceptance criteria:**
- [ ] legacy `traversalStore` 等待字段在进度事件/恢复状态同步下保留
- [ ] dual store 两路等待独立
- [ ] 组件在 `waiting_acquisition` 渲染横幅、暂停不渲染

**Verification:**
- [ ] `npm run typecheck` / `npm run test` / `npm run build`
- [ ] `go test ./internal/... ./api/...`
- [ ] `go test -race ./internal/usecase/ -run "Acquisition|Pause|Traversal"`
- [ ] `task release` + 冒烟（可选，交付时执行）

**Dependencies:** Task 10, 11

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/__tests__/*`
- `apps/desktop-wails/frontend/src/components/traversal/__tests__/*`
- `apps/desktop-wails/frontend/src/components/traversal/dual/__tests__/*`

**Estimated scope:** M
