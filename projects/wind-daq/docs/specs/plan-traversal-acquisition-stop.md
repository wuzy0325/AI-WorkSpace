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
| 快照结构体返回 State+Name+LastError，而非 `(state, ok)` | 避免 `ok=false` 暴露歧义；一次 `GetStatus` 锁内读取，消除窗口不一致 |
| 三态仅用于文案/诊断，不驱动超时 | spec 决策：设备异常一律无限期等待 |
| 等待作为 `PointPhase=waiting_acquisition`，不新增顶层 State | 避免 RunTraversalLoop / 前端状态机大改；State 保持原阶段，前端按 phase+字段显示横幅 |
| 等待逻辑抽 `waitForAcquisitionResume` 公共 helper | 点位开始与采样中复用，行为一致 |
| 等待恢复统一清 `fresh/pending`、保留 `lastTimestamps` | 防止停采前旧帧并入恢复后首个样本 |
| 多设备主展示优先级 `RECONNECT_REQUIRED > STOPPED > ACQUIRING` | 最严重的根因优先提示；行为仍是等待 |
| `acquisitionStallTimeout` 10s 保留有界 | 不可见异常（在采无新帧）需自动暴露（spec 已决策） |

## 实施阶段

### Phase 1：后端端口与状态模型

1. `ports/device.go`：新增 `AcquisitionState`/`AcquisitionStatus` 类型 + `AcquisitionController.AcquisitionStatus(id)` 方法。
2. `usecase/device_manager.go`：实现 `AcquisitionStatus`（一次 `GetStatus` → 三态映射）。
3. `core/traversal/types.go`：`PointPhase` 增 `waiting_acquisition`；`Status` 增 `WaitingForAcquisition`/`WaitingDevices`/`WaitingForAcquisitionSinceMs`；新增 `AcquisitionDeviceStatus`。
4. 更新全部 `AcquisitionController` mock（`traversal_*_test.go`、`calibration_*_test.go` 中的 mock 若实现该接口）。

### Phase 2：后端等待逻辑

5. `usecase/traversal_devices.go`：分类改三态、返回按 deviceID 排序的设备状态列表。
6. `usecase/traversal_acquisition.go`：抽 `waitForAcquisitionResume` helper（无限期等待、Stop/Pause 响应、多设备聚合、等待字段写入/清理、恢复时清 `fresh/pending`）；`collectAveragedSamples` 的 not-acquiring 分支改调它；删除 `notAcquiringTotal` 累计逻辑。
7. `usecase/traversal.go`：删除 `acquisitionNotAcquiringTimeout` 常量；`RunCurrentPoint` 点位开始检查改调 helper。

### Phase 3：前端贯通

8. `shared/types/traversal.ts`：`TraversalTestStatus`/`TraversalProgressEvent` 增等待字段；新增 `AcquisitionDeviceStatus` 前端类型。
9. `api/traversalApi.ts`：`getStatus()`/`onProgress()` 透传等待字段。
10. `stores/traversalStore.ts`：`applyProgressEvent()`/`syncRecoveredStatus()` 保留/合并等待字段。
11. `stores/dualTraversalStore.ts`/`dualTraversalRuntime.ts`/`traversalPolling.ts`：dual 链路同字段贯通。
12. 遍历页采集中组件 + 等待横幅组件（legacy 与 dual）+ `traversalErrorMapper.ts` + i18n 文案。

### Phase 4：测试与验证

13. 后端单测（三态映射、无限期等待、恢复、多设备聚合、暂停/停止交互、fresh/pending 重建）。
14. 前端单测（store 透传、组件渲染、dual）。
15. 全量验证命令 + `task release` 冒烟。

## 风险与注意事项

- **删除 60s 语义是行为反转**：既有 I-3/I-4 相关测试（`TestCollectAveragedSamplesContiguousNotAcquiringStillFails` 等）需改为断言"无限期等待"，不可保留旧断言。
- **mock 接口扩展**：任何实现 `ports.AcquisitionController` 的测试 mock 都要补 `AcquisitionStatus`，否则编译失败（这是优点：编译期强制全量更新）。
- **`Connection==Acquiring && !Acquiring` 病态组合**落入 `STOPPED`，记录日志即可，不特殊处理。
- **`acquisitionStallTimeout` 10s** 与等待逻辑互斥：等待期间重置 stallDeadline，等待不计入停滞。
- **前端 store 重建状态会丢字段**：`applyProgressEvent`/`syncRecoveredStatus` 必须显式透传，遗漏会导致等待横幅在进度事件刷新后消失。

## 验收

- 全部验收命令见 spec §验收标准。
- `go test ./internal/... ./api/...` + `npm run typecheck` + `npm run test` + `npm run build` + `-race`。
