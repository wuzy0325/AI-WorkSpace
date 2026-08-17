# Plan: 前端统一实时数据总线（DeviceDataBus）

> **依赖**：[spec-device-data-bus.md](./spec-device-data-bus.md) Revised
> **方法**：5 个可验证阶段；每阶段保持应用可运行

---

## 一、目标架构

```text
deviceStore
  ├─ owns subscribeToDevice / unsubscribeFromDevice
  ├─ owns deviceStatuses / acquiring
  └─ attachStatusListener bridges bus.attach/detach
                         │
                         ▼
deviceApi transport
  ├─ onSnapshot(payload)
  └─ onSnapshotError(deviceId, error)
                         │ only listeners
                         ▼
DeviceDataBus
  ├─ latestByDevice
  ├─ historyByDevice + renderTick 10Hz
  └─ failureByDevice
             │
             ├─ useDeviceLatest / useDeviceLatestMany
             ├─ useDeviceHistory
             └─ useDeviceFailure / useDeviceFailureMany
                         │
                         ▼
main / traversal / calibration / sphere-tank consumers
```

依赖方向固定为：

```text
components -> composables -> stores -> deviceApi
```

`deviceApi` 不导入 store。错误通过事件上行，不通过 API 层直接调用 Bus。

---

## 二、实施顺序

### Step 1：建立 transport 错误事件和 Bus

**目标**：先建立完整的数据和错误通道，不修改消费者。

改动：

1. `deviceApi.getLatest` 不再返回伪造的空 payload，错误向 polling 层冒泡。
2. 新增 `onSnapshotError` listener 集合和 `_notifySnapshotError`。
3. Wails polling catch 发布统一错误事件并保留一次 transport `console.warn`；不再只记录日志。
4. 调整 `subscribeDaqStream`（`sse-client.ts`）：删除 `onError('connected')`，保持 `(err: string) => void` 签名且只报告真实错误；本次不新增 `onConnected`。
5. 调整 `deviceApi.subscribeToDevice` SSE 分支：用 `(err) => _notifySnapshotError(deviceId, new Error(err))` adapter 将字符串错误转换并发布。
6. 创建 DeviceDataBus，唯一监听 snapshot/error，维护 latest/history/failure。
7. Bus 的 `attach/detach` 只控制事件监听和 render tick，不启动设备轮询。

验证：

- Wails polling 和 SSE error 单测均能观察到 `(deviceId, Error)`。
- `getLatest` rejection 进入 polling catch 后既发布 `onSnapshotError`，也只产生一次 transport warning。
- SSE connected 不产生错误，真实错误在订阅创建前后都能由当时已注册的 Bus listener 收到。
- Bus 第 3 次失败进入 failed，下一帧有效快照恢复。
- fake timer 下 history 只在下一个 10Hz tick 更新。
- 现有消费者继续由旧路径工作，无功能变化。

风险：低到中。`getLatest` 行为变化必须与 polling catch 同一阶段提交，不能拆开。

### Step 2：创建 selector composable 和 deviceStore 兼容桥

**目标**：建立消费者 API，同时保留现有公开接口。

改动：

1. 创建 `useDeviceLatest` 和同文件的 `useDeviceLatestMany`。
2. 创建 `useDeviceHistory`、`useDeviceFailure` 和 `useDeviceFailureMany`；多设备聚合采用任一失败语义。
3. composable 自动 `bus.attach()`，scope dispose 时 `bus.detach()`。
4. `deviceStore.latestFor/historyFor/historyVersion/selectedSnapshot` 转发到 Bus。
5. 删除 deviceStore 内部 latest/history/pending/render tick 和快照到 connection 的副作用。
6. 保留 `deviceStatuses`、所有设备订阅方法及 `ensureSubscribed`。
7. `deviceStore.attachStatusListener()` 桥接 Bus 生命周期，继续同步/清理设备订阅。

验证：

- 旧消费者无需修改即可继续工作。
- 兼容 selector 显式读取 Bus version，旧 computed/watch 仍能响应 Map 原地更新。
- `selectedSnapshot` 随 `selectedDeviceId` 和 Bus latestVersion 更新，DeviceDetailPanel 在 Step 3 直接迁移前保持兼容。
- start/stop acquisition 后底层设备轮询仍按原逻辑启停。
- `ensureSubscribed` 仍能覆盖遍历由后端启动采集的场景。
- store API 和 composable API 的类型检查通过。

风险：高。deviceStore 是核心 store；必须先完成代码图影响分析并执行完整前端测试。

### Step 3：迁移主界面和波形图

**目标**：主界面直接使用 Bus，不依赖 deviceStore 的快照兼容 API。

改动：

- 通道卡片和 DeviceDetailPanel 改用 `useDeviceLatest`，不再读取 `selectedSnapshot` 兼容 API。
- RealtimeChart 改用 `useDeviceHistory`，并保留现有 `timesBuffer/channelValuesBuffer` 容器复用和增量 `setOption` 路径。
- 可见区域读取 `useDeviceFailure`，使用现有局部错误提示展示刷新失败。

验证：

- 卡片 latest 更新目标 ≤50ms。
- chart watch history version，在回调中从 `data.value` 提取到复用 buffer，再执行 `setOption(..., { lazyUpdate: true })`；不长期持有 `data.value` 引用，CPU 不上升。
- 失败提示出现并在恢复快照后消失。

### Step 4：迁移遍历、校准和球罐安全门

**目标**：删除所有业务消费者的直接 snapshot 监听。

改动：

- 遍历根据配置中的唯一 deviceId 集合使用 `useDeviceLatestMany`，保留 `ensureSubscribed`，并用 `useDeviceFailureMany` 实现任一设备失败即整体失败。
- FiveHole、ThreeHole、TotalPressure 使用 `useDeviceLatest` 和 `useDeviceFailure`。
- `useSphereTankGate` 使用 Bus selector，不再直接监听 `deviceApi.onSnapshot`。
- TotalTemperature 手动采点模式保持不变。

验证：

- 单设备和多设备遍历均能读取所有配置通道。
- 三个校准模块与主界面数值一致。
- 球罐安全门启闭条件不回归。
- 各可见区域失败和恢复状态正确。

### Step 5：全局收尾和验收

**目标**：证明迁移完整，不再修改核心设计。

检查：

- 除 DeviceDataBus 外无 `deviceApi.onSnapshot` 或 `onSnapshotError` 消费者。
- 无业务消费者直接读取 `deviceStore.latestSnapshots/historyBuffers`。
- `deviceApi` 无 store import。
- typecheck、build、test 全绿。
- 完成后端中断/恢复、多设备遍历和性能手动验证。
- 更新项目记忆文档前由用户确认。

---

## 三、关键技术决策

### 1. Map 热路径更新

Map 原地更新，version 显式触发响应式计算：

```typescript
function pushSnapshot(payload: DataPayload): void {
  if (!payload?.deviceId || !Array.isArray(payload.channels) || payload.channels.length === 0) return
  latestByDevice.value.set(payload.deviceId, payload)
  latestVersion.value += 1
  pendingSnapshots.set(payload.deviceId, payload)
  recordSuccess(payload.deviceId)
}
```

不在 20Hz 热路径复制整个 Map。selector 读取 O(1)。

`deviceApi._notifyListeners` 统一归一化并过滤无 deviceId 或空 channels 的 payload，使 Wails/SSE 行为一致；`pushSnapshot` 再做防御性校验。有效 snapshot 即 `deviceId` 非空且 `channels.length > 0`；只有有效 snapshot 才更新 latest/history 并清除 failed。空 channels 表示暂无数据而不是 transport error。

### 2. Pinia ref 访问

- store 内部闭包：`latestVersion.value`。
- store 外部和 composable：`bus.latestVersion`。
- composable 对消费者返回 `ComputedRef`，消费者使用 `.value`。
- deviceStore 兼容方法先读取 `bus.latestVersion` / `bus.historyVersion`，再调用 Bus selector。

Pinia setup store 返回的 ref 在 store proxy 外部自动解包；computed 内读取 `bus.latestVersion` 会追踪底层 ref，无需 `.value`。Map 是 shallowRef 且原地修改，仅读取 Map 不会随 `Map.set` 重算，selector/composable 必须显式读取对应 version。

### 3. 多设备遍历

遍历配置先派生去重后的 deviceId 数组，再读取多设备快照：

```typescript
const deviceIds = computed(() => getTraversalDeviceIds(config.value))
const latestSnapshots = useDeviceLatestMany(deviceIds)

const livePressures = computed(() =>
  config.value
    ? buildRealtimePressuresFromSnapshots(config.value, latestSnapshots.value)
    : null,
)
```

不再把遍历数据收窄为单个 `getTraversalDeviceId()`。

### 4. 失败状态

失败状态是响应式数据，不是一次性 callback：

```typescript
interface DeviceFailureState {
  consecutiveFailures: number
  failed: boolean
  lastError: Error | null
  lastFailureAt: number | null
  lastSuccessAt: number | null
}
```

第 3 次失败从 `failed=false` 切换为 true。第 4 次及以后只更新计数和错误时间；下一帧有效 snapshot 清除 failed。

### 5. connection 状态

connection/acquiring 保留在 deviceStore。Bus 不包含 `connectionByDevice`，`pushSnapshot` 不写 `deviceStatuses`。这样既消除旧帧误判，也不增加 status 轮询链路。

命令完成到既有 `refreshStatusFor()` 返回之间允许短暂显示最后一次状态，不使用 snapshot 做乐观修正。

### 6. history 容量和 RingBuffer 替换

Bus 直接调用 `useStorageStore()` 并复用 `computeHistoryCapacity(historyWindowSec, refreshRateHz)`。该依赖方向无反向引用，不改变 `attach()` API，也不新增 `setCapacity()`。

Bus 保存 `lastHistoryCapacity`，每次 tick 即使 pending 为空也检测最新容量。容量变化时先遍历整个 `historyByDevice`，一次性用新容量的空 ring 替换所有现存设备并递增 `historyVersion`，再处理 pending；配置变化开销为一次 O(deviceCount)。这避免容量配置按设备延迟生效，同时不复制旧实现的 `currentMap/newMap` 引用技巧。`pendingSnapshots` 每设备覆盖式保留一帧，由 10Hz tick 消费。

### 7. 多设备失败聚合

`useDeviceFailureMany(deviceIds)` 对 ID 去重，返回 `{ failed, failedDeviceIds, statesByDevice }`。任一设备 failed 即 `failed=true`；空设备列表为 false。恢复只移除已恢复设备，直到 `failedDeviceIds` 为空才清除聚合失败。

---

## 四、风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| deviceStore 桥接后底层轮询未启动 | 实时数据完全停止 | 保留全部 subscription owner 逻辑并覆盖 start/ensure/cleanup 测试 |
| API 层和 Bus 循环依赖 | 初始化异常 | 错误通过 `onSnapshotError` 发布，grep 禁止 deviceApi import store |
| 只覆盖 Wails 错误 | 浏览器/SSE 模式静默失败 | Wails 与 SSE 各有错误传播测试 |
| 多设备遍历只取一个设备 | 部分压力显示为空 | 使用 `useDeviceLatestMany`，添加多设备配置测试 |
| composable 与 store 重复 attach | listener 泄漏 | 全局引用计数、scope dispose 和 listener 数量测试 |
| 错误提示无法恢复 | UI 长期停留失败 | snapshot 成功路径统一 `recordSuccess` |
| history 性能回归 | 波形图卡顿 | 保持 10Hz、Map 原地写、Performance 对比 |
| 容量变化延迟到设备下次来帧 | 配置不能同时生效 | tick 检测容量并全量替换现存 ring，覆盖无 pending 测试 |
| SSE connected 被误报失败 | 开发态立即显示错误 | 从 onError 语义中移除 connected，并测试真实 error |
| 迁移漏掉球罐安全门 | grep 验收失败或安全门失效 | 独立迁移和行为回归任务 |

---

## 五、验证检查点

### 每阶段自动验证

```powershell
npm run typecheck
npm run build
npm run test
```

### Step 1

- [ ] polling/SSE 错误事件测试通过。
- [ ] Bus latest/history/failure/lifecycle 测试通过。
- [ ] 每个测试独立 Pinia，fake timer 无泄漏。

### Step 2

- [ ] deviceStore 现有测试更新后全绿。
- [ ] `subscribeToDevice`、`ensureSubscribed`、cleanup 行为不变。
- [ ] `selectedSnapshot` 兼容转发在 selectedDeviceId/latest 变化时响应。
- [ ] 旧 API 转发能驱动现有页面。

### Step 3

- [ ] 主界面 latest/history 正常。
- [ ] 失败和恢复提示正常。
- [ ] Performance 对比无明显回归。
- [ ] RealtimeChart 保留 `timesBuffer/channelValuesBuffer` 复用，只 watch history version，并在回调中从 `data.value` 提取后增量更新。

### Step 4

- [ ] 单设备和多设备遍历正常。
- [ ] 任一设备失败时聚合提示包含正确设备，逐台恢复后正确清除。
- [ ] 五孔、三孔、总压正常。
- [ ] 球罐安全门正常。

### Step 5

- [ ] 全项目直接订阅 grep 干净。
- [ ] 后端中断与恢复验收通过。
- [ ] typecheck、build、test 全绿。

---

## 六、回滚策略

每个 Step 独立提交并按逆序回滚：

1. Step 5 只有验证和文档，无运行时回滚。
2. Step 4 回滚业务消费者，恢复其旧监听。
3. Step 3 回滚主界面，恢复 deviceStore 兼容 API 消费。
4. Step 2 回滚 deviceStore 内部 latest/history 实现。
5. Task 1.1 内部的 `getLatest` rejection 行为、polling catch 和错误事件发布必须一起回滚，避免 rejection 重新变成静默失败。
6. Task 1.2/1.3 的 Bus 可独立回滚；此时 deviceApi 仍发布无人消费的错误事件，不影响旧快照消费者。

任何时点都必须保证底层设备订阅仍有唯一 owner。
