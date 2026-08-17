# Spec: 前端统一实时数据总线（DeviceDataBus）

> **状态**：Revised，待用户审阅
> **范围**：wind-daq 前端（`apps/desktop-wails/frontend`）
> **相关文档**：[plan-device-data-bus.md](./plan-device-data-bus.md) / [tasks-device-data-bus.md](./tasks-device-data-bus.md)

---

## 一、Objective（目标）

当前前端存在多份最新快照副本和多个 `deviceApi.onSnapshot` 监听器：

| 消费者 | 当前数据源 | 主要问题 |
|---|---|---|
| 主界面通道卡片 | `deviceStore.latestSnapshots` | 数组查找和独立去重 |
| 主界面波形图 | `deviceStore.historyBuffers` | 历史缓存与其他实时消费者割裂 |
| 遍历侧边栏 | `useTraversalRealtimeData.latestSnapshots` | 独立副本，且可能涉及多个设备 |
| 校准侧边栏 | 各组件直接监听 `onSnapshot` | 重复订阅和重复派生 |
| 球罐安全门 | `useSphereTankGate` 直接监听 `onSnapshot` | 容易在总线迁移时遗漏 |

迁移后达到以下目标：

1. 所有快照消费者共享唯一的 `latestByDevice` 和 `historyByDevice`。
2. `deviceApi.onSnapshot` 和 `deviceApi.onSnapshotError` 各只由 DeviceDataBus 注册一次。
3. 快照错误和恢复状态可通过响应式 selector 读取，不使用一次性 callback 作为 UI 状态源。
4. 设备轮询、连接态和采集态继续由 `deviceStore` 管理，避免职责漂移和循环依赖。
5. 新消费者只组合 selector，不再自行实现监听、去重和失败恢复。

### 成功定义

- latest 类可见 UI 在 `DeviceDataBus.pushSnapshot` 收到有效快照后 50ms 内读取到同一个 payload；该指标不包含后端产生数据到前端 polling/SSE 收到数据的传输延迟。
- history/chart 在下一个 10Hz render tick 后更新，最坏目标延迟 100ms。
- 连续 3 次已报告的传输失败后进入失败态；收到下一帧有效快照后自动恢复。
- Wails polling 模式从第一次已报告的传输失败到 UI 显示失败，目标不超过 3 秒。
- SSE 模式受指数退避影响，不承诺从第一次失败起 3 秒内达到 3 次失败；第 3 次已报告失败发生后，UI 必须在下一次 Vue 更新周期内进入失败态。

---

## 二、职责边界

### DeviceDataBus 负责

- 唯一监听 `deviceApi.onSnapshot` 和 `deviceApi.onSnapshotError`。
- 维护 `latestByDevice`、`historyByDevice` 和 `failureByDevice`。
- 维护 10Hz history render tick。
- 通过 `useStorageStore()` 读取 `historyWindowSec` / `refreshRateHz`，并调用现有 `computeHistoryCapacity()` 计算 history 容量。
- 提供纯 selector 和响应式 composable。
- 使用引用计数管理上述两个全局事件监听器和 render tick。

### deviceStore 继续负责

- `deviceApi.subscribeToDevice(deviceId)` / `unsubscribeFromDevice(deviceId)` 的启停。
- `subscribedDeviceIds`、`syncSnapshotSubscriptions()`、`ensureSubscribed()` 和 cleanup。
- `deviceStatuses`、`statusFor()`、`acquiringFor()`。
- connect/disconnect/start/stop acquisition 等设备操作。
- `attachStatusListener()` 作为现有页面生命周期入口，并桥接 `bus.attach()` / `bus.detach()`。

### deviceApi 负责

- 维护 HTTP polling / SSE 的底层传输。
- 发布快照事件和快照错误事件。
- 不导入任何 Pinia store，避免 `deviceApi -> store -> deviceApi` 循环依赖。

### 明确不做

- 不新增 `useDeviceStatusStore`。
- 不新增第二套 status 轮询。
- 不修改后端 API、AcquisitionHub 或业务状态机。
- 不重构 generation token、SSE/HTTP transport 或采集命令流程。
- 不迁移总温手动采点模式；TotalTemperature 当前不监听 `deviceApi.onSnapshot`，因此无需迁移。

---

## 三、公开接口

### deviceApi 错误事件

```typescript
type SnapshotErrorCallback = (deviceId: string, error: Error) => void

deviceApi.onSnapshotError(callback: SnapshotErrorCallback): () => void
```

错误来源必须覆盖：

- Wails HTTP polling 的 `getLatest` rejection。
- 非 Wails SSE 的连接/传输错误。

`subscribeDaqStream` 当前通过 `onError('connected')` 表示连接成功。迁移时必须移除这种复用：`sse-client.ts` 的 `onError` 签名保持 `(err: string) => void`，只报告真实的 HTTP、响应类型、响应体、解析或连接中断错误；本次不新增 `onConnected` 回调。`deviceApi.subscribeToDevice` 的 SSE 分支使用 adapter：`(err) => deviceApi._notifySnapshotError(deviceId, new Error(err))`。`_notifySnapshotError` 在错误发生时读取 deviceApi 内部 listener Set 并分发；Bus 仅通过 `onSnapshotError` 注册 listener，deviceApi 不持有 Bus 引用。为避免首个错误发生在 Bus listener 注册前，`deviceStore.attachStatusListener()` 必须先 `bus.attach()`，再执行 `syncSnapshotSubscriptions()`。

SSE 重连成功本身不清除 failed；只有下一帧有效 snapshot 才调用 `recordSuccess`。如果重连后尚未开始或已暂停采集，UI 继续显示“数据刷新失败”，其语义是“当前没有恢复有效数据流”，而不是“SSE 握手仍失败”。

成功路径仍通过 `onSnapshot` 发布；Bus 收到有效快照时调用内部恢复逻辑。`deviceApi` 不直接调用 Bus。

### DeviceDataBus 数据结构

```typescript
export interface DeviceFailureState {
  consecutiveFailures: number
  failed: boolean
  lastError: Error | null
  lastFailureAt: number | null
  lastSuccessAt: number | null
}

export interface DeviceFailureSummary {
  failed: boolean
  failedDeviceIds: readonly string[]
  statesByDevice: ReadonlyMap<string, DeviceFailureState>
}

export const useDeviceDataBus = defineStore('deviceDataBus', () => {
  const latestByDevice = shallowRef(new Map<string, DataPayload>())
  const historyByDevice = shallowRef(new Map<string, RingBuffer<DataPayload>>())
  const failureByDevice = shallowRef(new Map<string, DeviceFailureState>())
  const latestVersion = ref(0)
  const historyVersion = ref(0)
  const failureVersion = ref(0)

  function attach(): void
  function detach(): void
  function pushSnapshot(payload: DataPayload): void
  function recordFailure(deviceId: string, error: Error): void
  function recordSuccess(deviceId: string): void
  function selectLatest(deviceId: string): DataPayload | undefined
  function selectHistory(deviceId: string): readonly DataPayload[]
  function selectFailure(deviceId: string): DeviceFailureState
})
```

内部更新规则：

1. Map 原地 `set`，并递增对应 version，保持热路径写入 O(1)。
2. `pushSnapshot` 立即更新 latest、暂存 history，并把对应设备标记为成功。
3. 失败只在从阈值以下跨越到阈值时进入 failed；后续失败更新计数，不重复产生 toast 式事件。
4. `recordSuccess` 清零连续失败并清除 failed。
5. 相同设备、相同 timestamp 不重复写入 history。

### 响应式触发规则

`latestByDevice`、`historyByDevice`、`failureByDevice` 都是 `shallowRef<Map<...>>`。Map 原地 `set/delete` 不会触发 shallowRef 自身更新，因此响应式 selector/composable 必须在读取 Map 前同时读取对应 version：

```typescript
const latest = computed(() => {
  bus.latestVersion // Pinia store proxy 自动解包并建立依赖
  const id = toValue(deviceId)
  return id ? bus.selectLatest(id) : undefined
})
```

只执行 `computed(() => bus.latestByDevice.get(id))` 是错误实现，不会随 Map 原地修改重算。store 内部闭包递增本地 `latestVersion.value`；store 外通过 Pinia proxy 访问 `bus.latestVersion`，无需 `.value`，但仍会追踪底层 ref。

### Selector composable

接口统一返回 `ComputedRef`，由 composable 自动 attach，并在 scope dispose 时 detach：

```typescript
function useDeviceLatest(
  deviceId: MaybeRefOrGetter<string | null>,
): ComputedRef<DataPayload | undefined>

function useDeviceLatestMany(
  deviceIds: MaybeRefOrGetter<readonly string[]>,
): ComputedRef<readonly DataPayload[]>

function useDeviceHistory(
  deviceId: MaybeRefOrGetter<string | null>,
): {
  data: ComputedRef<readonly DataPayload[]>
  version: ComputedRef<number>
}

function useDeviceFailure(
  deviceId: MaybeRefOrGetter<string | null>,
): ComputedRef<DeviceFailureState>

function useDeviceFailureMany(
  deviceIds: MaybeRefOrGetter<readonly string[]>,
): ComputedRef<DeviceFailureSummary>
```

`useDeviceFailureMany` 对输入 deviceId 去重；采用“任一设备失败则整体失败”语义。`failedDeviceIds` 只包含 `failed=true` 的设备，`statesByDevice` 保留每台设备状态，供 UI 显示具体设备。空输入返回 `failed=false`。

`useDeviceLatestMany` 同样对输入去重，按输入首次出现顺序返回已存在的 payload；尚未收到快照的设备不会产生占位项。消费者必须把结果视为部分集合，`buildRealtimePressuresFromSnapshots` 对缺失设备继续返回缺失通道，而不是假造零值。

`useDeviceHistory.data` 仅随 `historyVersion`（10Hz）重算，并调用 RingBuffer 的 `toArray()`。每次 push 后首次 `toArray()` 会重建缓存，未发生新 push 时返回缓存引用；消费者仍不得假设跨 version 的 `data.value` 引用稳定。

Pinia setup store 的 ref 对外自动解包。composable 中依赖 `bus.latestVersion`、`bus.historyVersion`、`bus.failureVersion`，不写 `bus.xxxVersion.value`；store 内部闭包仍使用本地 ref 的 `.value`。

### 生命周期规则

- `attach()` 只管理全局事件监听和 render tick，不启动任何设备轮询。
- `useDeviceLatest`、`useDeviceLatestMany`、`useDeviceHistory`、`useDeviceFailure` 可在同一组件组合使用，引用计数必须配对。
- `deviceStore.attachStatusListener()` 继续负责页面级设备订阅同步，同时桥接 Bus 生命周期。
- `useTraversalRealtimeData.ensureSubscribed()` 必须保留，因为遍历可能由后端启动采集，不能依赖设备管理页入口。

### History 容量与 pending 机制

采用容量方案 A：DeviceDataBus 直接导入 `useStorageStore` 和 `computeHistoryCapacity`。`storageStore` 不依赖 DeviceDataBus 或 deviceStore，因此该方向不会形成 store 循环依赖；不修改 `attach()` 签名，也不增加 `setCapacity()` 状态同步入口。

`pushSnapshot` 只把每台设备最新一帧写入 `pendingSnapshots: Map<string, DataPayload>`。10Hz render tick 消费 pending：

1. Bus 保存 `lastHistoryCapacity`。每次 tick 从 storageStore 当前设置重新计算目标容量，即使 pending 为空也执行容量检测。
2. 目标容量变化时遍历整个 `historyByDevice`，为所有现存设备一次性创建新 RingBuffer 并替换 Map 条目，丢弃旧历史；该操作是配置变化时一次性的 O(deviceCount)，并递增 `historyVersion` 使空历史立即反映到图表。
3. 完成全量容量重建后再遍历 pending；ring 不存在时按当前容量创建，并把当前 pending 帧写入。
4. 不沿用旧实现中 `currentMap` 与 `newMap` 并存的引用技巧。
5. flush 完成后清空 pending；最后一次 detach 也清空 pending，避免重新 attach 后消费旧帧。

旧实现只在设备有 pending 帧时延迟重建其 ring，这不会造成单个 ring 的“混合容量”或直接导致 ECharts 异常，但会让新容量配置延迟生效。本设计选择全量重建以保证所有设备在同一 tick 应用新容量。

### 有效快照规则

“有效 snapshot”明确定义为：归一化后 `deviceId` 非空且 `channels.length > 0` 的 snapshot。`deviceApi._notifyListeners` 是 Wails polling 和 SSE 共用的发布边界，只发布有效 snapshot；Bus 的 `pushSnapshot` 再做同样的防御性校验。只有有效 snapshot 才更新 latest/history 并调用 `recordSuccess` 清除 failed。空 channels 表示当前无可展示采集数据：不进入 Bus、不覆盖状态，也不算传输失败。HTTP/SSE 的真实错误通过 `onSnapshotError` 单独报告。

---

## 四、Project Structure（项目结构）

### 新增文件

```text
src/stores/deviceDataBus.ts
src/composables/useDeviceLatest.ts
src/composables/useDeviceHistory.ts
src/composables/useDeviceFailure.ts
src/tests/stores/deviceDataBus.spec.ts
```

`useDeviceLatestMany` 与 `useDeviceLatest` 放在同一文件，避免为单一变体增加文件。

### 修改文件

```text
src/api/deviceApi.ts
src/api/sse-client.ts
src/stores/deviceStore.ts
src/composables/useTraversalRealtimeData.ts
src/composables/useSphereTankGate.ts
src/components/main/DeviceOverviewPanel.vue
src/components/main/DeviceDetailPanel.vue
src/components/device/RealtimeChart.vue
src/components/traversal/TraversalMain.vue
src/components/calibration/five-hole/FiveHoleMain.vue
src/components/calibration/three-hole/ThreeHoleMain.vue
src/components/calibration/total-pressure/TotalPressureMain.vue
```

消费失败状态的具体展示位置以现有页面错误/提示组件为准，不新增全局 toast 系统。

### 保持不变

- `deviceStore.deviceStatuses/statusFor/acquiringFor` 的状态所有权。
- `deviceApi.subscribeToDevice` 的 generation token 和调度方式。
- `ringBuffer.ts`。
- 后端所有文件。
- TotalTemperature 手动采点流程。

---

## 五、Testing Strategy（测试策略）

### 单元和集成测试

每个测试使用新的 `setActivePinia(createPinia())`，并在 `afterEach` 中执行：

- `vi.clearAllTimers()` / `vi.useRealTimers()`。
- 释放所有 attach。
- 清理 `deviceApi` listener 和 mock。

必须覆盖：

1. latest 按设备 O(1) 覆盖，version 递增。
2. 多设备 selector 返回所有已存在 payload，不丢失跨设备通道。
3. attach/detach 引用计数、全局监听器和 render tick 启停。
4. history timestamp 去重、容量变化和 10Hz flush。
5. 第 3 次连续失败进入 failed，第 4 次不重复跨阈值通知语义。
6. 成功快照清除 failed 并更新 `lastSuccessAt`。
7. Wails polling error 和 SSE error 都发布 `onSnapshotError`。
8. `deviceApi` 不导入 `deviceDataBus`。
9. history 容量变化直接替换对应 ring，旧历史丢弃且当前 pending 帧写入新 ring。
10. 空 channels 不发布 snapshot、不会清除既有失败态。
11. 多设备失败聚合采用任一失败语义，并保留失败设备列表。

### 手动验证

- 主界面卡片与校准/遍历同时显示同一设备时数值一致。
- 多设备遍历配置可以从每台设备读取对应通道。
- 停止后端后，各可见消费区域显示“数据刷新失败”；恢复后提示自动消失。
- 球罐安全门行为不回归。
- 波形图 10Hz 更新且 CPU 占用不上升。

### 命令

```powershell
cd projects\wind-daq\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

---

## 六、Success Criteria（验收标准）

- [ ] **S1**：快照数据只有一份 `latestByDevice`，所有实时消费者读取同一个 payload。
- [ ] **S2**：历史数据只有一份 `historyByDevice`，波形图通过 `useDeviceHistory` 读取。
- [ ] **S3**：Wails polling 和 SSE 错误都进入响应式失败状态，恢复快照能自动清除失败。
- [ ] **S4**：Wails polling 从第一次已报告错误到可见 UI 失败提示不超过 3 秒；SSE 在第 3 次已报告错误后一个 Vue 更新周期内显示。
- [ ] **S5**：`deviceStatuses` 不再由快照写入修改，connection/acquiring 以后端 status 为准。
- [ ] **S6**：多设备遍历、五孔、三孔、总压和球罐安全门均完成迁移。
- [ ] **S7**：除 DeviceDataBus 外无 `deviceApi.onSnapshot` / `onSnapshotError` 消费者。
- [ ] **N1**：latest Map 读写 O(1)；history Map 定位 O(1)，`toArray()` 物化为 O(capacity)。
- [ ] **N2**：20Hz 输入下 CPU 占用不上升，history 仍限制为 10Hz。
- [ ] **N3**：typecheck、build、test 全绿，现有功能不回归。

---

## 七、已确认决策

1. **失败阈值**：默认连续 3 次；不做用户配置。
2. **失败展示**：各可见消费区域使用现有局部提示模式，不新增全局 toast。
3. **connection owner**：继续由 `deviceStore` 通过 status API 维护，不新增 status store。
4. **兼容 API**：迁移期间保留 `deviceStore.latestFor/historyFor/historyVersion` 转发；消费者完成迁移后仍暂留，另行清理。
5. **总温模块**：手动采点模式不纳入。
6. **CalibrationStream**：后端 dead code 清理不纳入。
7. **设备订阅 owner**：继续由 `deviceStore` 管理，Bus 不直接调用 `subscribeToDevice`。
8. **history 容量来源**：Bus 直接读取 storageStore 并复用 `computeHistoryCapacity()`；不向 `attach()` 传容量，不新增 `setCapacity()`。
9. **多设备失败语义**：任一配置设备 failed 即聚合 failed；UI 可从 `failedDeviceIds` 标出具体设备。

兼容转发必须显式读取对应 Bus version 后再调用 selector，确保仍通过旧 API 的 computed/watch 能追踪 Map 原地更新；兼容视图只能派生，不能缓存第二份快照或历史数据。

---

## 八、Boundaries（边界）

### Always do

1. selector 只读，不修改状态。
2. 新快照消费者必须使用 Bus composable。
3. 失败和恢复都必须可观测。
4. 每个阶段完成后运行 typecheck、build、test。

### Ask first

1. 修改 render tick 频率或 history 容量公式。
2. 修改后端 API 或 transport 类型。
3. 删除 deviceStore 兼容 API。
4. 引入新依赖。

### Never do

1. 不允许 `deviceApi` import Pinia store。
2. 不允许创建第二份 latest snapshot 缓存。
3. 不允许用快照推断 connection/acquiring。
4. 不允许删除 `ensureSubscribed` 而没有等价的设备轮询启动路径。
5. 不允许静默吞掉 polling/SSE 错误。

删除快照对 connection 的乐观更新后，设备命令完成到 `refreshStatusFor()` 返回之间，UI 保留最后一次后端状态，可能存在一个短暂旧状态窗口。该窗口以 status API 返回为边界，不允许用 snapshot 填补；start/stop/connect/disconnect 流程必须继续 await 或触发既有状态刷新。
