# Todo: 前端统一实时数据总线（DeviceDataBus）

> **依赖**：[spec-device-data-bus.md](./spec-device-data-bus.md) + [plan-device-data-bus.md](./plan-device-data-bus.md)
> **规则**：每个任务完成后至少运行定向测试；每个 Step 完成后运行 typecheck、build、完整 test

---

## Step 1：transport 错误事件 + DeviceDataBus

### Task 1.1：为 deviceApi 增加快照错误事件

- [ ] **Task**：在 `src/api/deviceApi.ts` 增加 `onSnapshotError`、内部 listener Set 和 `_notifySnapshotError(deviceId, error)`。删除 `getLatest` 的空 payload fallback；Wails polling catch 发布错误事件并保留一次 transport warning。`sse-client.ts` 删除 `onError('connected')`，保持 `(err: string) => void` 且只报告真实错误；deviceApi SSE 分支用 `(err) => _notifySnapshotError(deviceId, new Error(err))` adapter 发布。本次不新增 `onConnected`。保持 generation token 和轮询调度不变。
- **Acceptance**：
  - `getLatest` rejection 能到达 Wails polling catch。
  - polling catch 随后调用 `_notifySnapshotError`，并且每次失败只记录一次 transport warning。
  - Wails polling 和真实 SSE 错误均携带正确 deviceId；SSE connected 不发布错误。
  - unsubscribe 能移除错误 listener。
  - SSE adapter 调用 deviceApi 内部 `_notifySnapshotError`，错误发生时读取当前 listener Set；deviceApi 不持有 Bus 引用。
  - `_notifyListeners` 归一化后统一过滤无 deviceId 或空 channels 的 payload，Wails/SSE 行为一致。
  - `deviceApi.ts` 不导入任何 store。
- **Verify**：补充 `deviceApi` 单元测试并运行定向测试。
- **Files**：`src/api/deviceApi.ts`、`src/api/sse-client.ts`、对应 API/SSE 测试文件。

### Task 1.2：创建 DeviceDataBus 核心状态

- [ ] **Task**：创建 `src/stores/deviceDataBus.ts`，实现 latest/history/failure 三套 Map、三个 version、覆盖式 `pendingSnapshots`、`pushSnapshot`、`recordFailure`、`recordSuccess` 和三个 selector。Map 热路径原地写入，通过 version 触发响应式更新；`pushSnapshot` 防御性拒绝 deviceId 为空或 channels 为空的快照。
- **Acceptance**：
  - latest 按 deviceId O(1) 覆盖。
  - 第 3 次连续失败进入 failed，第 4 次仍为 failed 但不产生重复跨阈值语义。
  - 有效 snapshot 清除失败并设置 `lastSuccessAt`。
  - 空 channels 不更新 latest/history、不调用 recordSuccess，也不被当作 transport failure。
  - `selectHistory` 明确为 O(1) Map 定位和 O(capacity) 数组物化。
- **Verify**：创建 `src/tests/stores/deviceDataBus.spec.ts`；每个测试重新创建 Pinia。
- **Files**：`src/stores/deviceDataBus.ts`、`src/tests/stores/deviceDataBus.spec.ts`。

### Task 1.3：实现 Bus 生命周期和 history render tick

- [ ] **Task**：实现无参 `attach/detach` 引用计数。首次 attach 注册 snapshot/error listener 并启动 10Hz render tick；最后一次 detach 取消 listener、停止 timer、清空 pending。attach 不调用 `subscribeToDevice`。Bus 直接读取 `useStorageStore().settings` 并复用 `computeHistoryCapacity()`，用 `lastHistoryCapacity` 检测配置变化。
- **Acceptance**：
  - N 次 attach 需要 N 次 detach 才清理。
  - `pushSnapshot` 立即更新 latest，history 在下一个 tick 更新。
  - 相同 timestamp 不重复写 history。
  - 每次 tick 即使 pending 为空，也使用当前 historyWindowSec/refreshRateHz 检测容量。
  - 容量变化时先全量替换所有现存设备 ring、丢弃旧历史并递增 historyVersion，再写入当前 pending 帧；无 pending 的设备也立即应用新容量。
  - 不保留 currentMap/newMap 双引用技巧。
- **Verify**：fake timer 测试覆盖 listener 数量、tick、去重和清理；`afterEach` 恢复 timer/mock。
- **Files**：`src/stores/deviceDataBus.ts`、`src/tests/stores/deviceDataBus.spec.ts`。

### Step 1 Gate

- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] `npm run test`

---

## Step 2：selector composable + deviceStore 桥接

### Task 2.1：创建 latest selector composable

- [ ] **Task**：创建 `src/composables/useDeviceLatest.ts`，实现返回 `ComputedRef<DataPayload | undefined>` 的 `useDeviceLatest`，以及返回多个设备 payload 的 `useDeviceLatestMany`。内部依赖 `bus.latestVersion`，自动 attach/dispose。
- **Acceptance**：
  - 单设备返回 API 在所有文档和消费者中统一为 `const latest = useDeviceLatest(id)`。
  - 多设备输入去重，结果不包含未收到快照的设备。
  - Pinia store 外不访问 `bus.latestVersion.value`。
  - computed 显式读取 `bus.latestVersion` 后再调用 selector；测试证明 Map 原地 set 会触发重算。
- **Verify**：单设备、多设备、响应式 deviceId 变化和 scope dispose 测试。
- **Files**：`src/composables/useDeviceLatest.ts`、Bus 测试文件。

### Task 2.2：创建 history/failure selector composable

- [ ] **Task**：创建 `useDeviceHistory.ts` 和 `useDeviceFailure.ts`。history 返回 `{ data, version }`；failure 文件同时导出单设备 `useDeviceFailure` 和多设备 `useDeviceFailureMany`，均返回响应式 computed，不接受 callback。多设备采用任一设备失败语义，并返回失败设备 ID 和各设备状态。
- **Acceptance**：
  - history 仅在 `historyVersion` 变化后重算。
  - history data 调用 ring.toArray()；消费者不依赖跨 version 的数组引用稳定性。
  - failure 在阈值达到和恢复时都更新。
  - 多设备输入去重；空输入不失败；部分恢复后仍保留其余失败设备，全部恢复后才清除聚合失败。
  - 所有 composable dispose 后引用计数归零。
- **Verify**：composable 单元测试。
- **Files**：两个新增 composable、Bus 测试文件。

### Task 2.3：deviceStore 转发快照和历史 API

- [ ] **Task**：修改 `deviceStore.ts`，删除内部 `latestSnapshots`、`historyBuffers`、`pendingSnapshots`、render tick 和原 `pushSnapshot` 实现；`latestFor/historyFor/historyVersion/selectedSnapshot` 转发到 Bus。现有需要公开的 `latestSnapshots` 若仍被测试或旧消费者使用，改为只读 computed 兼容视图，不创建第二份缓存。该视图每次 latestVersion 变化从 Map 物化数组，复杂度 O(deviceCount)，仅用于迁移兼容并应在消费者迁移后另行删除。
- **Acceptance**：旧调用方继续工作，数据实际来自 Bus；没有第二份 latest/history 状态；兼容 selector 显式读取对应 Bus version，Map 原地更新后旧 computed/watch 仍会重算；`selectedSnapshot` 同时响应 selectedDeviceId 和 latestVersion。
- **Verify**：更新 deviceStore 测试，typecheck 和定向测试通过。
- **Files**：`src/stores/deviceStore.ts`、`src/stores/__tests__/deviceStore.test.ts`。

### Task 2.4：保留设备订阅和状态 owner

- [ ] **Task**：确保 `deviceStatuses/statusFor/acquiringFor`、`subscribedDeviceIds`、`syncSnapshotSubscriptions`、`ensureSubscribed`、cleanup 和 start/stop acquisition 逻辑保留。`attachStatusListener` 首次进入时先 `bus.attach()`，再执行 `syncSnapshotSubscriptions()`；对应 detach 时调用 `bus.detach()`，但底层设备订阅仍由 deviceStore 管理。
- **Acceptance**：
  - startAcquisition 能启动 `subscribeToDevice`。
  - 遍历 `ensureSubscribed` 能单独启动目标设备轮询。
  - 最后一次 detach 能清理 deviceStore 拥有的订阅。
  - Bus 不包含 connection Map，snapshot 不修改 deviceStatuses。
  - 命令到 refreshStatusFor 返回之间保留最后状态，不用 snapshot 填补短暂窗口。
- **Verify**：覆盖 start/ensure/cleanup 和旧帧不改变 connection 的测试。
- **Files**：`src/stores/deviceStore.ts`、deviceStore 测试文件。

### Step 2 Gate

- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] `npm run test`
- [ ] 手动确认主界面、遍历和一个校准页面仍有实时数据

---

## Step 3：主界面迁移

### Task 3.1：迁移通道卡片、详情面板和失败提示

- [ ] **Task**：将主界面通道卡片的 `deviceStore.latestFor/latestSnapshots` 读取改为 `useDeviceLatest`；将 `DeviceDetailPanel.vue` 的 `deviceStore.selectedSnapshot` 改为以 selectedDeviceId 调用 `useDeviceLatest`。同一可见区域使用 `useDeviceFailure`，复用现有局部提示样式显示“数据刷新失败”，成功恢复时自动隐藏。
- **Acceptance**：卡片和详情面板与 Bus 返回同一 payload；业务组件不再读取 selectedSnapshot；从 `bus.pushSnapshot` 调用起 latest 更新目标 ≤50ms（不含传输延迟）；失败/恢复提示正常。
- **Verify**：组件测试或手动测试；grep 主界面无旧快照读取。
- **Files**：`src/components/main/DeviceOverviewPanel.vue`、`src/components/main/DeviceDetailPanel.vue` 及其现有测试/提示组件。

### Task 3.2：迁移 RealtimeChart

- [ ] **Task**：RealtimeChart 使用一次 `useDeviceHistory(() => props.deviceId)` 调用，并复用其 `data/version`；禁止在 watch/getHistory 中重复创建 composable。保留现有 `timesBuffer/channelValuesBuffer` 容器复用，watch `version` 后从 `data.value` 提取时间戳/通道值，再执行当前的增量 `chart.setOption(..., { lazyUpdate: true })`；不直接长期持有 `data.value` 引用。
- **Acceptance**：chart 在下一个 10Hz tick 更新；无 `deviceStore.historyVersion/historyFor` 引用；CPU 不上升。
- **Verify**：手动波形验证和 Chrome Performance 对比。
- **Files**：`src/components/device/RealtimeChart.vue`。

### Step 3 Gate

- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] `npm run test`

---

## Step 4：遍历、校准和球罐安全门迁移

### Task 4.1：多设备遍历迁移

- [ ] **Task**：把单值 `getTraversalDeviceId` 调整为去重后的 `getTraversalDeviceIds`，用 `useDeviceLatestMany` 构造 `buildRealtimePressuresFromSnapshots` 输入。删除独立 `latestSnapshots` 和 `subscribeSnapshot`，但保留 `ensureSubscribed`，并让它为所有配置设备逐个调用 `deviceStore.ensureSubscribed`。
- **Acceptance**：
  - 单设备和多设备配置均能得到全部压力。
  - `useTraversalRealtimeData.ts` 无直接 `deviceApi.onSnapshot`。
  - TraversalMain 删除旧 `subscribeSnapshot` 生命周期代码，但保留 `ensureSubscribed` 调用。
  - 遍历使用 `useDeviceFailureMany`；任一设备失败即整体提示，并能显示 `failedDeviceIds`；所有失败设备恢复后才清除提示。
- **Verify**：新增多设备配置测试 + 手动遍历验证。
- **Files**：`useTraversalRealtimeData.ts`、`TraversalMain.vue` 及测试。

本任务不默认重构共享的 `findChannelValue`。当前快照设备数很小，缺少性能证据；若 Performance 录制证明数组查找成为热点，再单独评估在 `buildRealtimePressuresFromSnapshots` 单次计算内建立 `Map<deviceId, DataPayload>`，避免无关扩张共享 helper。

### Task 4.2：FiveHole 迁移

- [ ] **Task**：删除直接 snapshot listener，使用 `useDeviceLatest` 和 `useDeviceFailure`。
- **Acceptance**：数值与主界面一致；失败和恢复提示正常；无直接 listener。
- **Verify**：定向测试和手动五孔验证。
- **Files**：`src/components/calibration/five-hole/FiveHoleMain.vue` 及测试。

### Task 4.3：ThreeHole 迁移

- [ ] **Task**：同 Task 4.2，作用于 ThreeHole。
- **Acceptance**：同 Task 4.2。
- **Verify**：定向测试和手动三孔验证。
- **Files**：`src/components/calibration/three-hole/ThreeHoleMain.vue` 及测试。

### Task 4.4：TotalPressure 迁移

- [ ] **Task**：同 Task 4.2，作用于 TotalPressure。
- **Acceptance**：同 Task 4.2。
- **Verify**：定向测试和手动总压验证。
- **Files**：`src/components/calibration/total-pressure/TotalPressureMain.vue` 及测试。

### Task 4.5：球罐安全门迁移

- [ ] **Task**：修改 `useSphereTankGate.ts`，删除直接 snapshot listener，改为读取 Bus selector。保持原启闭条件和组件生命周期不变。
- **Acceptance**：无直接 `deviceApi.onSnapshot`；有效/无效压力下安全门行为与迁移前一致。
- **Verify**：补充或更新球罐安全门测试。
- **Files**：`src/composables/useSphereTankGate.ts` 及测试。

### Task 4.6：记录 TotalTemperature 范围决策

- [ ] **Task**：确认 TotalTemperature 继续使用手动采点模式，不修改代码。
- **Acceptance**：全局 grep 时不把非 snapshot 手动 API 误判为遗留消费者。
- **Files**：无。

### Step 4 Gate

- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] `npm run test`
- [ ] 手动验证遍历、五孔、三孔、总压和球罐安全门

---

## Step 5：全局收尾

### Task 5.1：静态残留检查

- [ ] grep `deviceApi.onSnapshot`：除 `deviceDataBus.ts` 外无消费者。
- [ ] grep `deviceApi.onSnapshotError`：除 `deviceDataBus.ts` 和 API 测试外无消费者。
- [ ] grep `deviceStore.latestSnapshots/historyBuffers`：无业务消费者直接读取。
- [ ] grep `from '@stores/deviceDataBus'` 或等价路径：`deviceApi.ts` 中不存在。
- [ ] grep `subscribeSnapshot`：无已删除遍历/校准生命周期残留。

### Task 5.2：中断和恢复验收

- [ ] **Task**：Wails 模式通过桌面壳启动采集后停止本地 HTTP server，验证 polling 错误。SSE 模式按下列 PowerShell 命令分别启动独立 API server 和纯 Vite（不是 `wails3 dev`），浏览器访问 `http://127.0.0.1:9246` 并确认 `isWailsAvailable() === false`，随后中断 `/api/daq/stream/{deviceId}`。两种模式都记录失败并验证恢复后下一帧有效 snapshot 清除提示。

```powershell
# 终端 1，工作目录 projects\WindLabX4\services\api-go；默认监听 :8080
go run ./cmd/server

# 终端 2，工作目录 projects\WindLabX4\apps\desktop-wails\frontend
$env:VITE_API_TARGET = 'http://127.0.0.1:8080'
npm run dev
```
- **Acceptance**：Wails polling 从首次已报告失败起 3 秒内显示；浏览器 SSE 在第 3 次已报告失败后一个 Vue 更新周期内显示，不要求受 backoff 影响的前三次失败在 3 秒内完成；SSE connected 不误报失败；UI 不冻结；失败状态不产生重复 toast。
- **Verify**：录屏或测试记录。

### Task 5.3：多设备和性能验收

- [ ] **Task**：验证至少两个 deviceId 的遍历配置；对比迁移前后 chart Performance 录制。
- **Acceptance**：全部配置通道有值；20Hz 输入下 CPU 无明显上升；history 保持 10Hz。

### Task 5.4：更新项目记忆

- [ ] **Task**：经用户确认后记录：Bus 是快照/历史/刷新失败唯一数据源；deviceStore 是设备轮询与 status owner；禁止业务消费者直接监听 snapshot；API 层不得依赖 store。
- **Acceptance**：内容与最终实现一致，不提前写入未落地规则。

### Final Gate

- [ ] `npm run typecheck`
- [ ] `npm run build`
- [ ] `npm run test`
- [ ] 所有静态检查和手动验收通过

---

## 任务依赖

```text
1.1 -> 1.2 -> 1.3
              |
              v
2.1 -> 2.2 -> 2.3 -> 2.4
                       |
                       +-> 3.1 -> 3.2
                       |
                       +-> 4.1 / 4.2 / 4.3 / 4.4 / 4.5 / 4.6
                                      |
                                      v
                              5.1 -> 5.2 -> 5.3 -> 5.4
```

Step 3 与 Step 4 只能在 Step 2.4 和 Step 2 Gate 完成后并行；两者修改的文件集合独立。Step 5 必须等待两个分支全部完成。
