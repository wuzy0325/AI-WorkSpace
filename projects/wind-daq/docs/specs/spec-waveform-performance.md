# Spec: Wind-DAQ 实时波形图性能优化

## Objective（目标）

### 我们在解决什么问题
Wind-DAQ 设备详情面板的实时波形图（`RealtimeChart.vue`）在采集数据累积占满缓冲区后出现明显 UI 卡顿。根因是数据层 `O(n)` 数组搬移 + 渲染层每帧全量重算/全量 setOption + ECharts 平滑曲线与面积渐变的渲染开销叠加。

### 用户是谁
风洞操作员。他们在采集期间长时间盯着波形图观察趋势，卡顿会直接影响他们对设备状态的判断和操作响应。

### 成功标准（可测试）
1. 在 20 Hz 采样、4 通道、`waveformBufferSize = 256` 条件下，连续采集 10 分钟，波形图主线程帧时间 **P95 < 16 ms**（维持 60 fps 不掉帧）。
2. `pushSnapshot` 单次调用时间复杂度从 O(n) 降为 O(1)（环形缓冲区，无 shift/splice 搬移）。
3. 每帧 ECharts 更新走 `setOption` 增量路径（`notMerge: false` + 仅 series.data 变更，按 series name 匹配），不再全量重建 option 对象。
4. 现有功能零回归：Tooltip 精度/单位、通道选择联动、Y 轴单位聚合、主题切换、空状态、autoresize 全部保持。
5. `npm run test` 全绿，新增环形缓冲区单元测试覆盖率 ≥ 90%。

## Tech Stack（技术栈）
- Vue 3.5 + TypeScript 5.8 + Pinia 3
- ECharts 6.1 + vue-echarts 8（CanvasRenderer）
- Vitest 4 + @vue/test-utils 2 + jsdom 29
- 目标：仅 `projects/wind-daq`，不动 `daq-t1603` / `daq-p1604`

## Commands（命令）

```powershell
# 进入前端目录
cd projects\wind-daq\apps\desktop-wails\frontend

# 类型检查
npm run typecheck

# 单元测试
npm run test

# 单个测试文件 watch 模式（开发时用）
npx vitest run src/stores/__tests__/deviceStore.test.ts
npx vitest run src/components/device/__tests__/RealtimeChart.test.ts

# 生产构建（含 vue-tsc 类型检查 + vite build）
npm run build

# 结构校验（提交前强制）
powershell -File ..\..\..\..\validate-frontend-structure.ps1 -CheckFileSize
```

> **提交前强制顺序**：`npm run typecheck` → `npm run test` → `npm run build` → `validate-frontend-structure.ps1`。

## Project Structure（项目结构）

本次改动仅涉及以下文件，不新增目录：

```
projects/wind-daq/apps/desktop-wails/frontend/src/
├── stores/
│   ├── deviceStore.ts                    # 改：环形缓冲区替换 shift + shallowRef + 容量对齐
│   └── __tests__/
│       └── deviceStore.test.ts           # 改：补环形缓冲区边界测试 + 容量变化测试
├── components/device/
│   ├── RealtimeChart.vue                 # 改：增量 setOption + 关闭 smooth/area/animation
│   └── __tests__/
│       └── RealtimeChart.test.ts         # 新增：渲染增量性与视觉降配验证
└── composables/
    ├── useRingBuffer.ts                  # 新增：通用环形缓冲区 composable
    └── useRafThrottle.ts                 # 新增：rAF 合并层 composable
```

> 不在 `shared/` 抽共享件——本次仅 wind-daq 范围。若后续 daq-t1603/daq-p1604 要复用，再单独发起迁移 spec。

## Code Style（代码风格）

遵循工作区规则：**中文注释、类型注解、可扩展设计**。示例（环形缓冲区 composable 草案）：

```ts
// composables/useRingBuffer.ts
// 定长环形缓冲区：push O(1)，无数组搬移。
// 用于实时波形图历史数据存储，替代 Array.shift() 的 O(n) 行为。
// toArray() 内置版本号缓存：仅在 push/clear 后首次调用重建数组，后续直接返回缓存引用。
export interface RingBuffer<T> {
  /** 追加一个元素；缓冲区满时覆盖最旧元素，并使 toArray 缓存失效 */
  push(item: T): void
  /** 按时间顺序返回当前全部元素（ oldest → newest ）。版本号未变时返回缓存引用 */
  toArray(): readonly T[]
  /** 当前元素数量（≤ capacity） */
  readonly length: number
  /** 容量上限 */
  readonly capacity: number
  /** 清空缓冲区并使缓存失效 */
  clear(): void
}

export function createRingBuffer<T>(capacity: number): RingBuffer<T> {
  if (capacity <= 0) throw new Error(`RingBuffer capacity must be > 0, got ${capacity}`)
  const buf: T[] = new Array<T>(capacity)
  let head = 0       // 下一个写入位置
  let size = 0       // 当前有效元素数
  let version = 0    // 版本号：push/clear 时递增，toArray 据此判断是否需要重建
  let cached: readonly T[] | null = null  // 缓存结果，避免每次 toArray 都新建数组

  return {
    capacity,
    get length() { return size },
    push(item: T) {
      buf[head] = item
      head = (head + 1) % capacity
      if (size < capacity) size += 1
      version += 1
      cached = null
    },
    toArray() {
      if (cached !== null) return cached
      if (size < capacity) {
        cached = buf.slice(0, size) as readonly T[]
      } else {
        cached = [...buf.slice(head), ...buf.slice(0, head)] as readonly T[]
      }
      return cached
    },
    clear() { head = 0; size = 0; version += 1; cached = null },
  }
}
```

**关键约定**：
- 所有公开函数有显式返回类型注解
- `push` / `toArray` / `clear` 三个原语即够，不过度设计
- `toArray()` 返回 `readonly T[]`，调用方不应突变——突变不被追踪
- 容量 ≤ 0 抛错，fail-fast 而非静默
- 版本号缓存：h → 连续两次 toArray 返回同一引用，配合 shallowRef 避免不必要的 computed 重新求值

### 波形图缓冲区容量设计

```
用户设置 waveformBufferSize（storageStore，50～2000，默认 100，步长 50）
        │
        ▼
pushSnapshot 获取容量 = min(waveformBufferSize, MAX_HISTORY_POINTS)
        │   MAX_HISTORY_POINTS = 2000 作为安全硬上限
        ▼
创建/复用 RingBuffer<DataPayload>(容量)
        │
        │  用户修改 settings.waveformBufferSize 时：
        │  下次 pushSnapshot 检测 ring.capacity !== 期望容量 → 丢弃旧 ring 重建
        │  （丢失旧数据可接受——用户改大就是想要更多空间，改小就是裁剪）
        ▼
historyFor(id) → ring.toArray()  ← 直接全量返回，不再 slice(-maxPoints)
```

**理由**：store 存 256 而 chart 只用 100 的"双层缓冲"没有意义——环形缓冲 O(1) 后开销固定，容量直接对齐用户设置即可。

## Testing Strategy（测试策略）

### 框架与位置
- Vitest + @vue/test-utils，测试文件与源码同目录 `__tests__/` 子文件夹
- 已有 `src/stores/__tests__/deviceStore.test.ts`，在其上补充环形缓冲边界用例
- 新增 `src/components/device/__tests__/RealtimeChart.test.ts`

### 测试层级与覆盖目标

| 层级 | 关心的问题 | 测试方式 |
|---|---|---|
| **单元**：`createRingBuffer` | push 满 / 绕环 / toArray 缓存 / clear / 容量边界 / 版本号语义 | 纯函数测试，无 DOM |
| **单元**：`deviceStore.pushSnapshot` | 环形缓冲满后覆盖最旧、length 不超 capacity、settings 变化后重建 | mock `deviceApi.onSnapshot`，连续 push capacity+10 次 |
| **组件**：`RealtimeChart` | 增量 setOption 调用次数 < 全量、smooth=false、areaStyle 不存在、animation=false、channelIndices 变化时 optionBase 重建 | mount + spy `chartInstance.setOption`，推 N 帧断言调用模式 |

### 覆盖率要求
- `useRingBuffer.ts` 行覆盖 ≥ 95%（小而关键）
- `deviceStore.pushSnapshot` 环形缓冲分支 ≥ 90%
- `RealtimeChart` 增量路径 ≥ 80%（DOM/Canvas 交互难以全覆盖，聚焦 option 构造与 setOption 调用）

### 不测什么
- 不测 ECharts 内部 canvas 像素输出（黑盒，依赖浏览器）
- 不测真实 20 Hz 时序（用 fake timer 控制即可）
- 不测主题切换像素差异（已有 e2e 覆盖）

## Boundaries（边界）

### Always（始终遵守）
- 每次改 `deviceStore.ts` 或 `RealtimeChart.vue` 前运行 `gitnexus_impact` 评估爆破半径，向用户报告风险等级
- 改完每个任务跑 `npm run typecheck` + `npm run test`
- 提交前跑完整 `typecheck` + `test` + `build` + `validate-frontend-structure.ps1`
- 中文注释，类型注解齐全
- 保持 `RealtimeChart` 对外 props 契约不变（`deviceId` / `channelIndices` / `maxPoints`）

### Ask First（先问再做）
- 改 `MAX_HISTORY_POINTS` 常量值（影响内存上限，需用户确认）
- 引入任何新 npm 依赖
- 修改 `storageStore.waveformBufferSize` 默认值或校验范围
- 修改 Wails 绑定签名（本 spec 不应触及，但若意外需要必须先问）

### Never（绝不做）
- 不删 `deviceStore.test.ts` 中已通过的测试用例
- 不改 `daq-t1603` / `daq-p1604` 任何文件
- 不引入 `shared/` 共享件（本次范围外）
- 不改 `RealtimeChart` 的 tooltip formatter / 通道颜色映射 / Y 轴单位聚合逻辑
- 不提交 secrets / 不动 vendor 目录
- 不在 `RealtimeChart.vue` 中访问硬件或写校准算法（工作区硬约束）

## Success Criteria（成功标准，细化可测试）

1. **性能**：vitest bench 或手动 perf 标记下，连续 push 10000 次快照，`pushSnapshot` 平均耗时较现状降低 ≥ 70%。
2. **复杂度**：`pushSnapshot` 内部不再出现 `shift()` / `splice(0, k)` 任何形式的全数组搬移（grep 验证）。
3. **渲染**：推 100 帧 → `chartInstance.setOption` 调用 ≤ 100 次（不爆发），增量 setOption 按 series name 匹配数据而非数组索引。
4. **视觉**：`smooth` === false、`areaStyle` === undefined、`animation` === false（grep option 构造代码验证）。
5. **回归**：现有 `deviceStore.test.ts` 全绿；Tooltip formatter 多测（若有）保持通过；通道选择/主题切换人工冒烟通过。
6. **结构**：`validate-frontend-structure.ps1 -CheckFileSize` 通过，新增文件大小不触发告警。
7. **缓存**：`toArray()` 连续两次调用返回同一引用（===），push 后缓存失效返回新引用。
8. **容量同步**：用户修改 `waveformBufferSize` 后，下次 pushSnapshot 自动重建 ring，新 ring 容量与设置一致。

## 已确认决策（Open Questions 已关闭）

1. ✅ **`MAX_HISTORY_POINTS` 对齐**：容量 = `min(waveformBufferSize, MAX_HISTORY_POINTS)`，MAX 从 256 提至 2000 与 `WAVEFORM_BUFFER_MAX` 对齐作为安全硬上限。
2. ✅ **rAF 合并层**：加上，兜底后端突发补帧。
3. ✅ **缓冲上限不动**：`waveformBufferSize` 校验范围保持 50～2000 不变。
4. ✅ **视觉降配**：smooth = false、无 areaStyle、animation = false。
5. ✅ **渲染改造方式**：ref 拿 VChart exposed `chart: ShallowRef<ECharts>` 手动 setOption 增量。
6. ✅ **范围**：仅 wind-daq。
