# Plan: WindLabX4 实时波形图性能优化

> 关联 spec: [spec-waveform-performance.md](./spec-waveform-performance.md)
> 阶段: Phase 2 — Plan（技术实现方案）

## 1. 影响分析（GitNexus 爆破半径）

| 符号 | 调用方 | 风险等级 | 备注 |
|---|---|---|---|
| `pushSnapshot(payload)` | `deviceStore.ts:229` onSnapshot 回调 + 6 处测试 | **LOW** | 仅 store 内部 + 测试调用，无外部直接引用 |
| `historyFor(id)` | `RealtimeChart.vue:34` + `DeviceDetailPanel.vue:201` + 2 处测试 | **LOW** | 返回类型保持 `DataPayload[]` 不变即可零回归 |
| `RealtimeChart.vue` | `DeviceDetailPanel.vue:14,378` 单点引用 | **LOW** | props 契约不变 |
| `MAX_HISTORY_POINTS` | `deviceStore.ts:6` + 测试 `:95` 断言 `<=256` | **MEDIUM** | 提至 2000 对齐 `WAVEFORM_BUFFER_MAX`，测试断言更新 |
| `historyBuffers` (Map) | store 内部独享 | **LOW** | 改 shallowRef + 手动触发 |

**结论**：爆破半径可控，最大风险在 `MAX_HISTORY_POINTS` 从 256 → 2000 会破坏现有 `<=256` 断言——需同步更新测试。

## 2. 既有审计对照

`docs/runbooks/perf-measurement-plan.md` §1.2 已审计此问题，结论与本 spec 一致：
- `option` 用 computed + 深响应式 → 全量重建 ❌
- `historyBuffers` 用 `ref<Map>` → 深度 reactive 追踪开销 ❌
- 无 setOption 节流 ❌

本 Plan 是对该审计的落地实现。

## 3. 架构设计

### 3.1 数据层改造（deviceStore）

```
                    onSnapshot 回调 (20Hz)
                            │
                            ▼
                   pushSnapshot(payload)
                            │
                            │  容量 = min(waveformBufferSize, MAX_HISTORY_POINTS)
                            │  检查 ring.capacity 是否匹配 → 不匹配则丢弃旧 ring 重建
                            │
              ┌─────────────┴─────────────┐
              ▼                           ▼
   latestSnapshots[idx] = n     historyRing.push(n)   ← O(1)
              │                           │
              │                  shallowRef 触发 (++version)
              │                           │
              └──────────┬────────────────┘
                         ▼
                 historyFor(id) → ring.toArray()  ← 版本号缓存，无额外 slice
```

**关键变更**：
1. `historyBuffers: Ref<Map<string, DataPayload[]>>` → `historyBuffers: ShallowRef<Map<string, RingBuffer<DataPayload>>>`
2. `maxPoints` prop 不再用于 slice——ring 容量本身就是 `waveformBufferSize`，`historyFor` 直接全量返回
3. 响应式通过 `shallowRef` + 版本号 `historyVersion: Ref<number>` 触发（`push` 后 `++version`）
4. 容量 = `min(storageStore.settings.waveformBufferSize, MAX_HISTORY_POINTS)`
5. `MAX_HISTORY_POINTS` 从 256 → 2000，对齐 `WAVEFORM_BUFFER_MAX`
6. Settings 变化时：`pushSnapshot` 检测 `ring.capacity !== 期望容量` → 丢弃旧 ring 用 `createRingBuffer(新容量)` 重建

### 3.2 渲染层改造（RealtimeChart）

```
historyFor(deviceId)  ← computed 求值（依赖 historyVersion）
        │
        ▼
optionBase (computed, 仅在 channelIndices/profile/theme 变化时重算)
   ├─ tooltip formatter
   ├─ grid / xAxis / yAxis
   └─ series 骨架（{ name, type, lineStyle, ... } 不含 data）
        │
        ▼
seriesData (computed, 仅 history 变化时重算)
   └─ [{ name: 'CH01', data: [ts,val][] }, { name: 'CH02', data: [...] }, ...]
        │  每个系列都带 name，ECharts 按 name 匹配（不靠数组索引）
        │
        ▼
rAF 调度器 (markDirty → requestAnimationFrame → flush)
   ├─ 收到 history 变化 → markDirty()
   ├─ rAF 回调 → 若 dirty:
   │    setOption({ series: seriesData.value }, { notMerge: false, lazyUpdate: true })
   └─ 合并同一帧内多次 history 变化为一次 setOption
```

**关键变更**：
1. `option` computed 拆为 `optionBase`（静态结构）+ `seriesData`（动态数据，每项含 name）
2. 用 `ref` 拿 VChart exposed `chart: ShallowRef<ECharts>`，手动 `setOption` 增量更新
3. `optionBase` 变化 → `setOption(optionBase, { notMerge: true })` 全量重建（触因：channelIndices/profile/theme 变化）
4. `seriesData` 变化 → rAF 回调节流 → `setOption({ series: [{ name, data }] }, { notMerge: false, lazyUpdate: true })`
   - **ECharts 按 series.name 匹配**，不依赖数组索引，channelIndices 增删后不会串数据
5. `smooth: false` / `areaStyle: undefined` / `animation: false`
6. 删除 `watch(history, ..., { deep: true })`——shallowRef 下 deep watch 无意义且漏触发
7. `onBeforeUnmount` 中调用 `rAF.dispose()`

### 3.3 rAF 合并层设计

```ts
// composable: useRafThrottle(callback: () => void)
// 同一帧内多次 markDirty 合并为一次 callback 调用
let dirty = false
let rafId: number | null = null

function markDirty() {
  dirty = true
  if (rafId === null) {
    rafId = requestAnimationFrame(flush)
  }
}

function flush() {
  rafId = null
  if (!dirty) return
  dirty = false
  callback()
}

function dispose() {
  if (rafId !== null) cancelAnimationFrame(rafId)
  rafId = null
}
```

20Hz 推送 < 60fps rAF 节拍，正常情况下每帧单独 flush；若后端突发补帧（如 5 帧同毫秒到达），5 次 `markDirty` 合并为 1 次 `setOption`。

### 3.4 VChart 实例获取（已确认，无需 spike）

`node_modules/vue-echarts/dist/index.d.ts` L78：
```ts
chart: vue0.ShallowRef<echarts_core0.ECharts | undefined, echarts_core0.ECharts | undefined>;
```

VChart 组件通过 template ref 暴露 `chart` 属性，是 `ShallowRef<ECharts>`：
```ts
const vchartRef = ref<InstanceType<typeof VChart> | null>(null)
// vchartRef.value.chart → ECharts 实例
```

## 4. 实现顺序（依赖图）

```
[T1] createRingBuffer composable + 测试
   │  无依赖，纯函数，可独立验证
   ▼
[T2] deviceStore 集成环形缓冲 + shallowRef + 容量对齐 + 测试更新
   │  依赖 T1
   ▼
[T3] useRafThrottle composable + 测试
   │  无依赖，纯调度逻辑，可独立验证
   ▼
[T4] RealtimeChart 拆 optionBase/seriesData + name 匹配增量 setOption + rAF + 视觉降配
   │  依赖 T2 (响应式语义已变) + T3
   ▼
[T5] 集成验证：typecheck + test + build + 结构校验 + 人工冒烟
```

**可并行**：T1 与 T3 无依赖，可同时开发。T2 必须在 T1 后（依赖 RingBuffer 类型）。T4 必须在 T2、T3 后。

## 5. 风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| `shallowRef` + 手动版本号触发，遗漏某次 push 导致 UI 不更新 | 中 | 高（图不动） | T2 测试覆盖"连续 push N 次后 historyFor 返回 N 条" + version 递增 |
| `toArray()` 每次新建数组，computed 频繁调用产生 GC 压力 | 低 | 低 | RingBuffer 内建版本号缓存：push 后首次 toArray 重建，后续返回同一引用 |
| ECharts `setOption({ series })` 按 name 匹配数据，若 name 冲突或未提供 name 会错乱 | 低 | 高 | optionBase 中 series 带 name（现有已带），增量数据也带 name；测试断言 channelIndices 增删后数据不错位 |
| rAF 在 Tab 隐藏时不触发（浏览器节流）导致积压 | 低 | 低 | Tab 隐藏不渲染无所谓；恢复可见后下次 markDirty 直接触发（dirty=true 不会丢） |
| 现有测试 `<=256` 断言失败 | 高 | 低 | T2 同步更新断言为 `<=2000`（新 MAX_HISTORY_POINTS） |
| Settings 变化后 ring 丢弃旧数据再重建，瞬间白屏 | 中 | 低（可接受）| 新建空的 ring，下一帧 push 后立即有数据；用户改大本来就是要新空间 |

## 6. 验证检查点

| 检查点 | 时机 | 命令 |
|---|---|---|
| CP1: T1 完成 | createRingBuffer 测试全绿 + 缓存命命中 | `npx vitest run src/composables/__tests__/useRingBuffer.test.ts` |
| CP2: T2 完成 | deviceStore 测试全绿 + typecheck + grep 无 shift | `npm run test && npm run typecheck` |
| CP3: T3 完成 | useRafThrottle 测试全绿 | `npx vitest run src/composables/__tests__/useRafThrottle.test.ts` |
| CP4: T4 完成 | RealtimeChart 测试 + 全量 test + typecheck | `npm run test && npm run typecheck` |
| CP5: 提交前 | build + 结构校验 | `npm run build && validate-frontend-structure.ps1 -CheckFileSize` |

## 7. 非目标（明确排除）

- 不做降采样（LTTB 等）——20Hz 不需要
- 不改 daq-t1603 / daq-p1604
- 不抽 shared/ 共享件
- 不改 tooltip formatter / 通道颜色 / Y 轴单位聚合
- 不放开 waveformBufferSize 校验上限（保持 2000）
- 不引入新 npm 依赖
