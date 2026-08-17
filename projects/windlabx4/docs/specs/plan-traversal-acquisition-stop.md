# 实现计划：遍历测试"设备停止采集"处理流程

> 关联：[spec-traversal-acquisition-stop.md](./spec-traversal-acquisition-stop.md) | [tasks-traversal-acquisition-stop.md](./tasks-traversal-acquisition-stop.md)
> 状态：待实施
> 日期：2026-08-10

## 概述

按 spec v4 实现"设备异常 = 类暂停无限期等待"语义：遍历测试运行期间，设备主动停采（`STOPPED`）或掉线（`RECONNECT_REQUIRED`）时，遍历进入 `waiting_acquisition` 阶段无限期等待，不自动判失败；退出路径为操作员恢复设备 / 暂停 / 停止。同时移除旧 60s 未采集累计超时，保留 10s 采样停滞兜底。

## 现状与目标的差异

| 维度 | 现状 | 目标（spec v4） |
|---|---|---|
| 点位开始 | 设备未采集 → 立即失败 | 进入无限期等待 |
| 采样中 | 连续未采集 60s → 失败 | 无限期等待 |
| 掉线设备 | 与停采同样处理（60s） | 无限期等待，文案区分"重启采集 / 重连" |
| 根因区分 | `IsAcquiring` 仅 bool | `AcquisitionStatus` 三态快照 |
| 等待可见性 | 无 | `Status.WaitingForAcquisition*` + 前端横幅 |
| 超时 | `acquisitionNotAcquiringTimeout` 60s | 删除；`acquisitionStallTimeout` 10s 保留 |
| freshness | 暂停清 `fresh/pending`，not-acquiring 未清 | 所有等待恢复路径统一清 |

## 架构决策

| 决策 | 理由 |
|---|---|
| 端口仅新增 `AcquisitionStatus(id) AcquisitionStatus`，保留既有 4 方法 | 六边形边界：usecase 不依赖 DeviceManager 类型；不破坏既有 mock 之外的方法集 |
| 快照结构体返回 State+Name+LastError，而非 `(state, ok)` | 避免 `ok=false` 暴露歧义；`State` 来自一次 `GetStatus` 锁内读取；`Name` 复用 `DeviceName` 为两次调用，属非关键展示元数据（spec §采集状态快照端口） |
| 三态仅用于文案/诊断，不驱动超时 | spec 决策：设备异常一律无限期等待 |
| 等待作为 `PointPhase=waiting_acquisition`，不新增顶层 State | 避免 RunTraversalLoop / 前端状态机大改；State 保持原阶段，前端按 phase+字段显示横幅 |
| **点位开始显式置 `StateMoving + PhaseWaitingForAcquisition`** | `RunCurrentPoint` 采集态检查在 `updatePhase(moving)` 前，非末点上一点后 State 仍可能为 saving——等待前显式建立本点阶段，首点/后续点公开状态一致，恢复后置 `PhaseMoving` |
| 等待逻辑抽 `waitForAcquisitionResume` 公共 helper；**helper 不持有 `stallDeadline`** | helper 的参数/返回值访问不到采样局部变量 `stallDeadline`；由采样调用方在返回后重置，点位开始调用方无需处理 |
| 恢复 freshness 按设备状态区分 | `STOPPED` 清 `fresh/pending` 保 `lastTimestamps`；`RECONNECT_REQUIRED` 重置基线（-1）并丢首帧——重连后时间戳可能归零/回绕，旧基线会让新帧永不够新而误触发 10s 停滞 |
| 多设备主展示优先级 `RECONNECT_REQUIRED > STOPPED > ACQUIRING` | 最严重的根因优先提示；行为仍是等待；`WaitingDevices` 仅含异常设备，N=len |
| `acquisitionStallTimeout` 10s 保留有界 | 不可见异常（在采无新帧）需自动暴露（spec 已决策） |

## 实施阶段

### Phase 1：后端端口与状态模型

1. `ports/device.go`：新增 `AcquisitionState`/`AcquisitionStatus` 类型 + `AcquisitionController.AcquisitionStatus(id)` 方法。
2. `usecase/device_manager.go`：实现 `AcquisitionStatus`（一次 `GetStatus` → 三态映射）。
3. `core/traversal/types.go`：`PointPhase` 增 `waiting_acquisition`；`Status` 增 `WaitingForAcquisition`/`WaitingDevices`/`WaitingForAcquisitionSinceMs`；新增 `AcquisitionDeviceStatus`。
4. 更新全部 `AcquisitionController` mock（`traversal_*_test.go`、`calibration_*_test.go` 中的 mock 若实现该接口）。

### Phase 2：后端等待逻辑

5. `usecase/traversal_devices.go`：分类改三态、返回按 deviceID 排序的**异常设备**状态列表。
6. `usecase/traversal_acquisition.go`：抽 `waitForAcquisitionResume` helper（无限期等待、Stop/Pause 响应、多设备聚合、等待字段写入/清理、**不持有 `stallDeadline`**）；`collectAveragedSamples` 的 not-acquiring 分支改调它，返回后由采样调用方重置 `stallDeadline`，并按设备重建基线（`STOPPED` 清 `fresh/pending`；`RECONNECT_REQUIRED` 重置 `lastTimestamps=-1` 并丢首帧）；删除 `notAcquiringTotal` 累计逻辑。
7. `usecase/traversal.go`：删除 `acquisitionNotAcquiringTimeout` 常量；`RunCurrentPoint` 点位开始前置检查改调 helper，进入等待前先显式置 `StateMoving + PhaseWaitingForAcquisition`，恢复后置 `PhaseMoving`。

### Phase 3：前端贯通

8. `shared/types/traversal.ts`：`TraversalTestStatus`/`TraversalProgressEvent` 增等待字段；新增 `AcquisitionDeviceStatus` 前端类型（仅异常设备）。
9. `api/traversalApi.ts`：`getStatus()`/`onProgress()` 透传等待字段。
10. `stores/traversalStore.ts`：`applyProgressEvent()`/`syncRecoveredStatus()` 保留/合并等待字段。
11. `stores/dualTraversalStore.ts`/`dualTraversalRuntime.ts`/`traversalPolling.ts`：dual 链路同字段贯通。
12. **`usecase/traversal_view.go` `BuildStatusResponse()`**：手工构造 `map[string]any`，新增字段不会自动出现——显式输出等待字段到 legacy `/status` 与 dual probe `/status`，并补 **API 合约测试**直接断言 JSON 字段。
13. 遍历页采集中组件 + 等待横幅组件（legacy 与 dual）+ `traversalErrorMapper.ts` + i18n 文案。

### Phase 4：测试与验证

14. 后端单测（三态映射、无限期等待、恢复、多设备聚合、暂停/停止交互、fresh/pending 重建、**重连时间戳归零/回绕**、等待字段生命周期）。
15. 前端单测（store 透传、组件渲染、dual、API 合约）。
16. 全量验证命令 + `task release` 冒烟。

## 风险与注意事项

- **删除 60s 语义是行为反转**：既有 I-3/I-4 相关测试（`TestCollectAveragedSamplesContiguousNotAcquiringStillFails` 等）需改为断言"无限期等待"，不可保留旧断言。
- **mock 接口扩展**：任何实现 `ports.AcquisitionController` 的测试 mock 都要补 `AcquisitionStatus`，否则编译失败（这是优点：编译期强制全量更新）。
- **`Connection==Acquiring && !Acquiring` 病态组合**落入 `STOPPED`，记录日志即可，不特殊处理。
- **重连后时间戳回绕**：`RECONNECT_REQUIRED → ACQUIRING` 必须重置该设备 `lastTimestamps=-1` 并丢首帧，否则新帧永不够新 → 10s 误判"在采无新帧"。按设备分别处理，不得全局统一重置。
- **`stallDeadline` 所有权**：helper 不持有采样局部变量 `stallDeadline`，采样调用方在 helper 返回后重置，否则恢复后立刻触发旧的 10s 超时。
- **点位开始公开状态**：等待前显式置 `StateMoving + PhaseWaitingForAcquisition`，否则非末点可能以 `StateSaving` 公开（首点 `StateRunning`），状态不一致。
- **`BuildStatusResponse` 手工 map**：`traversal.Status` 新增字段不会自动出现在 HTTP 响应，必须显式加并补 API 合约测试。
- **`Status()` 深复制**：`WaitingDevices` 需深复制（当前只深复制 `Results`），避免调用方与等待循环的 slice 竞态。
- **前端 store 重建状态会丢字段**：`applyProgressEvent`/`syncRecoveredStatus` 必须显式透传，遗漏会导致等待横幅在进度事件刷新后消失。

## 验收

- 全部验收命令见 spec §验收标准。
- `go test ./internal/... ./api/...` + `npm run typecheck` + `npm run test` + `npm run build` + `-race`。
