# Tasks: WindLabX4 实时波形图性能优化

> 关联: [spec-waveform-performance.md](./spec-waveform-performance.md) · [plan-waveform-performance.md](./plan-waveform-performance.md)
> 阶段: Phase 3 — Tasks（可执行任务清单）

## 实现顺序与依赖

```
T1 ──┐
     ├──► T2 ──┐
T3 ──┘         ├──► T4 ──► T5
               │
               ┘
```

- T1、T3 无依赖，可并行
- T2 依赖 T1（RingBuffer 类型）
- T4 依赖 T2（响应式语义）+ T3（rAF 调度）
- T5 是集成验证，依赖全部

---

## T1: createRingBuffer 环形缓冲区 + 单元测试

- **Acceptance（验收）**:
  - `createRingBuffer<T>(capacity)` 工厂函数可用，capacity ≤ 0 抛错
  - `push` O(1)，缓冲满后覆盖最旧元素
  - `toArray()` 返回 oldest→newest 顺序的 `readonly T[]`
  - `toArray()` 版本号缓存：连续两次调用返回同一引用（===）；push/clear 后引用变化
  - `length` 不超 `capacity`
  - `clear()` 重置后 length=0、toArray=[]
- **Verify（验证）**:
  - `npx vitest run src/composables/__tests__/useRingBuffer.test.ts`
  - 覆盖率 ≥ 95%
- **Files**:
  - 新增 `src/composables/useRingBuffer.ts`
  - 新增 `src/composables/__tests__/useRingBuffer.test.ts`
- **测试用例清单**:
  1. capacity=4，push 3 次 → toArray 长度 3，顺序 [a,b,c]
  2. capacity=4，push 6 次 → toArray 长度 4，顺序覆盖最旧
  3. capacity=1，连续 push → 始终只保留最新
  4. capacity=0 → 抛 Error
  5. clear() 后 length=0、toArray=[]
  6. 泛型验证：`createRingBuffer<{ts:number,v:number}>(capacity)`
  7. **toArray 缓存**：push 后连续两次 toArray 返回同一引用；再次 push 后引用变化
  8. **toArray 缓存 + clear**：clear 后 toArray 返回空数组，与之前引用不同

---

## T2: deviceStore 集成环形缓冲 + shallowRef + 容量对齐

- **Acceptance**:
  - `historyBuffers` 改为 `shallowRef<Map<string, RingBuffer<DataPayload>>>`
  - `pushSnapshot` 内用 `ring.push()`（O(1)），无 shift/splice
  - `historyFor(id)` 返回 `ring.toArray()`，类型仍为 `DataPayload[]`
  - 响应式通过 `shallowRef` + `historyVersion`（`ref<number>`）触发，每次 push 后 `++version`
  - 容量 = `min(storageStore.settings.waveformBufferSize, MAX_HISTORY_POINTS)`
  - `MAX_HISTORY_POINTS` 改为 2000（对齐 `WAVEFORM_BUFFER_MAX`）
  - `pushSnapshot` 检测 ring.capacity ≠ 期望容量时，丢弃旧 ring 用 `createRingBuffer(新容量)` 重建
  - `historyVersion` 暴露在 store return 中，供 T4 computed 依赖
  - 现有测试全部通过（`<=256` 断言更新为 `<=2000`）
- **Verify**:
  - `npx vitest run src/stores/__tests__/deviceStore.test.ts`
  - `npm run typecheck`
  - grep 验证：`deviceStore.ts` 内无 `.shift()` / `splice(0,` 全数组搬移
- **Files**:
  - 改 `src/stores/deviceStore.ts`
  - 改 `src/stores/__tests__/deviceStore.test.ts`
- **新增测试用例**:
  1. 连续 push capacity+10 次 → length === capacity，toArray 最旧元素 = 第 11 条
  2. 同 timestamp 不入缓冲（去重保留）
  3. 每次 push 后 historyVersion 递增（响应式触发证据）
  4. **容量对齐**：mock waveformBufferSize=1500 → ring.capacity=1500
  5. **容量变化重建**：push 100 帧后改 waveformBufferSize=50 → 下次 push 后 ring.capacity=50、length≤50
  6. **安全上限**：mock waveformBufferSize=3000 → ring.capacity=2000（MAX_HISTORY_POINTS 兜底）

---

## T3: useRafThrottle composable + 单元测试

- **Acceptance**:
  - `useRafThrottle(callback)` 返回 `{ markDirty, flush, dispose }`
  - 多次 `markDirty` 在同一帧内合并为一次 callback 调用
  - `dispose()` 取消未执行的 rAF
  - jsdom 无 rAF 时降级为 `setTimeout(fn, 0)`（配合 vi.useFakeTimers 测试合并行为）
- **Verify**:
  - `npx vitest run src/composables/__tests__/useRafThrottle.test.ts`
  - 覆盖率 ≥ 90%
- **Files**:
  - 新增 `src/composables/useRafThrottle.ts`
  - 新增 `src/composables/__tests__/useRafThrottle.test.ts`
- **测试用例清单**:
  1. markDirty 5 次 → vi.runAllTimers → callback 调用 1 次
  2. dispose 后 markDirty 不触发 callback
  3. flush 同步执行 pending dirty（dirty 清零）
  4. rAF 回调后 dirty 清零，下次 markDirty 重新调度

---

## T4: RealtimeChart 拆 optionBase/seriesData + 增量 setOption + rAF + 视觉降配

- **Acceptance**:
  - 用 template ref 拿 VChart exposed `chart: ShallowRef<ECharts>`
  - `optionBase` computed：仅 channelIndices/profile/theme 变化时重算
    - 含 tooltip formatter、grid、axis、series 骨架（`{ name, type:'line', lineStyle, itemStyle }` 不含 data）
  - `seriesData` computed：仅 `historyVersion` 变化时重算
    - 返回格式：`[{ name: 'CH01', data: [[ts,val],...] }, { name: 'CH02', data: [...] }]`
    - 每个系列都带 `name`，ECharts 按 name 匹配数据而非数组索引
  - `optionBase` 变化 → `chart.setOption(optionBase, { notMerge: true })` 全量重建
  - `seriesData` 变化 → `chart.setOption({ series: seriesData.value }, { notMerge: false, lazyUpdate: true })` 增量更新
  - 增量更新通过 `useRafThrottle` 合并：`watch(seriesData, markDirty, { flush: 'sync' })` → rAF 回调内 setOption
  - 视觉降配：`smooth: false`、`areaStyle: undefined`、`animation: false`
  - props 契约不变（deviceId/channelIndices/maxPoints）
  - **删除** `watch(history, ..., { deep: true })`——shallowRef 下无效且多余
  - `onBeforeUnmount` 调用 `rAF dispose()`
- **Verify**:
  - `npx vitest run src/components/device/__tests__/RealtimeChart.test.ts`
  - `npm run typecheck`
  - grep 验证：`smooth: false`、无 `areaStyle`、`animation: false`
  - 测试断言：推 10 帧 → setOption 调用 ≤ 10 次（不爆发）
- **Files**:
  - 改 `src/components/device/RealtimeChart.vue`
  - 新增 `src/components/device/__tests__/RealtimeChart.test.ts`
- **测试用例清单**:
  1. mount 后推 1 帧 → setOption 至少调用 1 次（初始化）
  2. 推 5 帧 → setOption 增量调用 ≤ 6 次（1 初始化 + 5 增量，rAF+ flush= 'sync' → 无合并到微任务）
  3. option 含 `smooth: false`、无 `areaStyle`、`animation: false`
  4. channelIndices 变化 → optionBase 重算（setOption notMerge:true 至少 1 次）
  5. unmount → rAF dispose，无泄漏警告

---

## T5: 集成验证

- **Acceptance**:
  - 全量 `npm run test` 全绿
  - `npm run typecheck` 零错误
  - `npm run build` 成功
  - `validate-frontend-structure.ps1 -CheckFileSize` 通过
  - 人工冒烟通过（见下方步骤清单）
- **Verify**:
  - `cd projects\WindLabX4\apps\desktop-wails\frontend && npm run test && npm run typecheck && npm run build`
  - `powershell -File ..\..\..\..\validate-frontend-structure.ps1 -CheckFileSize`
- **人工冒烟步骤**:
  1. 启动采集 → 确认波形图从空状态切换到曲线绘制
  2. 等待缓冲占满（100pt @ 20Hz ≈ 5s）→ 确认曲线连续滚动无停顿/跳帧
  3. 用鼠标拖拽/滚轮缩放波形图 → 确认交互流畅（无 1s 以上卡顿）
  4. 在通道选择中勾选/取消一个通道 → 确认曲线增减、图例同步、颜色正确
  5. 停止采集 → 重启采集 → 确认历史清空重新绘制
- **Files**: 无新增（验证任务）

---

## 执行规则

- 每个任务完成立即跑对应 Verify 命令，绿了才进下一个
- 改 `deviceStore.ts` / `RealtimeChart.vue` 前已有 GitNexus 影响分析（见 plan §1），风险 LOW
- VChart API 已确认：`vchartRef.value.chart` 是 `ShallowRef<ECharts>`，无需 spike
- 任何任务失败 → 停在该任务，不进下一个
