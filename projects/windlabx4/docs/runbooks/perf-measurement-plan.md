# WindLabX4 全项目性能测量计划 + 静态审计结果

> 创建日期：2026-06-23
> 静态审计执行日期：2026-06-23（无运行时 profiling，仅源码 + 已构建产物）
> 依据：`performance-optimization` skill — **先测量，再优化**
> 范围：WindLabX4 工业 DAQ 桌面平台（Wails + Vue 3 + Go）

---

## 0. 总则

本计划遵循 skill 工作流：**MEASURE → IDENTIFY → FIX → VERIFY → GUARD**。

- 静态分析能找到**结构性瓶颈**（导入方式、缺监控、并发模式、整包加载），但**绝对数值**（FPS、p95 延迟）仍需运行时测量
- 每个标注 ⚠️ / ❌ 的发现都附文件:行号或测量证据
- 修复前需走 FIX/VERIFY，不可凭直觉动手

### 标注图例

| 标记 | 含义 |
|---|---|
| ❌ P0 | 已确认问题，需立即处理 |
| ⚠️ P1 | 中风险，需运行时验证后决定 |
| ℹ️ P2 | 长期健康度，做完前两类再说 |
| ✅ | 静态审查无问题 |

---

## 1. 前端（Vue 3 / Wails 渲染层）

### 1.1 ❌ Bundle 体积与首屏加载

**详细文档**：[`perf-frontend-bundle-baseline.md`](./perf-frontend-bundle-baseline.md)

| 指标 | 实测 | budget | 状态 |
|---|---|---|---|
| 首屏 JS（gzip） | **487 KB** | < 200 KB | ❌ 超 2.4× |
| 总 JS（gzip） | 691 KB | < 400 KB | ❌ |
| 总 CSS（gzip） | 28 KB | < 50 KB | ✅ |

**根因**：
- `src/components/traversal/visualization/composables/useECharts.ts:2` 使用 `import * as echarts` 整包导入，禁用 tree-shaking → echarts chunk 407 KB gzip
- `dist/index.html:8` 首屏 `modulepreload` echarts，导致路由没用到也下载

**修复见 bundle baseline 文档 P0 部分**

---

### 1.2 ⚠️ 实时波形图表渲染

**审计目标**：`src/components/device/RealtimeChart.vue`

**发现 #1 ❌ P0：`option` 用 `computed` + 深响应式 `history`，每次 push 触发完整重建**

`RealtimeChart.vue:24-74`：

```ts
const history = computed(() => deviceStore.historyFor(props.deviceId))  // 返回 Array<DataPayload>
const option = computed(() => {
  const data = history.value.slice(-props.maxPoints)
  // 重新 map times、重新 map series、重新构造整个 option 对象
  ...
})
```

- 每次 `deviceStore.appendPayload`（最高 100 Hz，见 acquisition.go:43 maxPublishHz=100）触发 history 变化
- `option` 是 computed，依赖 deep-reactive 数组 → 整个 option 对象重算
- vue-echarts 的 `:option` 是 deep watcher，新对象引用 → 调用 `setOption` 全量更新
- 实际渲染节流仅靠 `props.maxPoints=100` 截断显示长度，**没有 setOption 节流**

**证据 #2 ❌ P0：`deviceStore.historyBuffers` 用普通 `ref<Map<string, DataPayload[]>>`**

`src/stores/deviceStore.ts:12`：

```ts
const historyBuffers = ref<Map<string, DataPayload[]>>(new Map())
```

- 数组每秒 push/shift 高频，触发 Map 深度 reactive 追踪开销
- 所有 channels（每个 DataPayload）字段都被 Vue Proxy 包裹，订阅者越多反应越慢

**发现 #3 ⚠️ P1：`watch(history, ..., { deep: true })` 第 77 行**

```ts
watch(history, () => { loading.value = false }, { deep: true })
```

- `history` 是 computed 返回数组，再加 `deep:true` 等于双重深度追踪
- 在高频更新场景下放大开销

**发现 #4 ⚠️ P1：未启用 echarts `progressive` / `large` 模式**

- 当前 maxPoints=100 时影响不大；但 traversal 可视化可能有数千点（CrossSection/Heatmap/VectorField）→ 未来风险

**建议修复（动手前需运行时确认）**：

```ts
// stores/deviceStore.ts
import { shallowRef } from 'vue'
const historyBuffers = shallowRef<Map<string, DataPayload[]>>(new Map())
// 每次 push 后用 historyBuffers.value = new Map(historyBuffers.value) 触发更新
//   或：保留 ref，但用 shallowReactive 包裹 Map

// components/device/RealtimeChart.vue
import { shallowRef, watchEffect } from 'vue'
const chartOption = shallowRef<EChartsOption>({})
// 用 throttle / requestAnimationFrame 节流 setOption 调用
watchEffect(() => {
  const data = history.value.slice(-props.maxPoints)
  // 用 mutateOption 或 setOption(notMerge=false) 仅更新 series.data
})
```

**Budget**（运行时验证）：
- 100 Hz 推流下 FPS ≥ 50
- Long Task < 1/秒
- 1 小时持续运行 heap 增长 < 50 MB

---

### 1.3 ✅ 路由切换 / 视图加载

**审计目标**：`src/router/index.ts`

**发现**：✅ 全部路由已用 `() => import(...)` lazy 加载，符合最佳实践

```ts
// router/index.ts:7-15
{ component: () => import('@views/main/MainDashboardView.vue') }  // ✅
{ component: () => import('@views/MotionView.vue') }                // ✅
```

**剩余问题**：`MainDashboardView` lazy chunk 自身偏大（332 KB raw / 94 KB gzip），需要内部组件拆分。**见 1.1 修复方案 P1**。

---

### 1.4 ⚠️ Pinia store re-render 风险

**审计目标**：`src/stores/*.ts`（9 个 store，总 2837 行）

**发现 #1 ❌ P0：所有 store 都没用 `shallowRef` / `shallowReactive`**

```
grep -n "shallowRef\|shallowReactive\|markRaw" src/stores/*.ts
  (none)
```

所有大数组、大 Map 都被 Vue 深度 Proxy 化。受影响最大的：

| Store | 行数 | 风险字段 | 严重度 |
|---|---|---|---|
| `traversalStore.ts` | 830 | `dataPoints: ref<TraversalDataPoint[]>` (line 59)，可能数千点 | ❌ P0 |
| `deviceStore.ts` | 454 | `historyBuffers: ref<Map<…>>` 高频 push/shift | ❌ P0 |
| `i18nStore.ts` | 825 | 静态文案 Map，但不会变 → 应 `markRaw` 或 `shallowRef` | ⚠️ P1 |
| `calibrationStore.ts` | 314 | 含 `setInterval` 轮询（line 112） | ℹ️ P2 |

**发现 #2 ⚠️ P1：i18nStore 占首屏 24 KB gzip，但内容是静态**

`stores/i18nStore.ts:5-…` 是两个大 Record<string,string> 常量，作为 reactive 浪费。

**建议修复**：

```ts
// traversalStore.ts
import { shallowRef } from 'vue'
const dataPoints = shallowRef<TraversalDataPoint[]>([])
// push 时用 dataPoints.value = [...dataPoints.value, newPoint]
//   或针对高频路径：dataPoints.value.push(); triggerRef(dataPoints)

// i18nStore.ts
import { markRaw, computed, ref } from 'vue'
const dictionaries = markRaw({ zh, en })
const locale = ref<Locale>('zh')
const t = (key: string) => dictionaries[locale.value][key] ?? key
```

---

### 1.5 ✅ 内存泄漏 / 资源清理

**审计目标**：grep `setInterval` / `setTimeout` / `addEventListener` / `EventSource` 等

**所有 8 个使用定时器的组件均有匹配的 `onBeforeUnmount` / `onUnmounted` 清理 hook**：

| 文件 | 状态 |
|---|---|
| `calibration/five-hole/FiveHoleMain.vue` | ✅ |
| `calibration/three-hole/ThreeHoleMain.vue` | ✅ |
| `calibration/total-pressure/TotalPressureMain.vue` | ✅ |
| `calibration/total-temperature/TotalTemperatureMain.vue` | ✅ |
| `layout/MainBottomBar.vue` | ✅ |
| `motion/MotionControlPanel.vue` | ✅ |
| `traversal/PointsPreview.vue` | ✅ |
| `views/main/MainDashboardView.vue` | ✅ |

**echarts 实例**：

- `composables/useECharts.ts:20-21` ✅ 正确 dispose 并清空引用
- `RealtimeChart.vue` 用 `<VChart>` 组件，vue-echarts 自动管理 dispose ✅

**SSE / 轮询客户端**：

- `api/sse-client.ts` 用 reconnect timer，逻辑闭包内可清理 ✅
- `api/traversalApi.ts:89` 全局共享 polling timer，单例管理 ⚠️ 需确认无路由切换泄漏

**剩余建议**：30 个 `onBeforeUnmount` 覆盖了 8 个使用定时器组件，匹配良好。**P2**：加 ESLint `vue/no-unused-vars` + 自定义规则强制 setInterval 必须 paired cleanup。

---

### 1.6 ✅ CSS Bundle

| 指标 | 实测 | budget |
|---|---|---|
| 总 CSS（gzip） | 28 KB | ✅ < 50 KB |

无问题。

---

## 2. Go 后端（services/api-go）

### 2.1 ❌ API Latency 仪表化缺失

**审计目标**：`api/server.go`、`pkg/apiserver/`、`cmd/server/main.go`

**发现 #1 ❌ P0：使用裸 `http.NewServeMux`，无日志/计时中间件**

`api/server.go:50` 仅有 `corsMiddleware`，没有：
- ❌ Request duration log
- ❌ Status code log
- ❌ Panic recovery middleware
- ❌ Request ID / trace

**任何 API 现在出问题都是黑盒**——这是观测性的最大缺口。

**发现 #2 ⚠️ P1：handler 体积巨大，难做局部插桩**

`api/server.go` 单文件 861 行，一个 `NewRouter` 函数 600+ 行匿名 handler。即使想加 timing log，得改 50+ 处。

**建议结构**：

```go
// internal/adapters/http/middleware.go
func WithMetrics(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rec := &statusRecorder{ResponseWriter: w, code: 200}
        next.ServeHTTP(rec, r)
        slog.Info("http.request",
            "method", r.Method, "path", r.URL.Path,
            "status", rec.code, "duration_ms", time.Since(start).Milliseconds())
    })
}

// api/server.go
return WithMetrics(corsMiddleware(mux))
```

**Budget（运行时确认前提目标）**：

| 接口类 | 例子 | p95 budget |
|---|---|---|
| 读元数据 | GET /api/device/profiles | < 50 ms |
| 读最新数据 | GET /api/daq/latest/{id} | < 10 ms |
| 写控制 | POST /api/device/{id}/connect | < 100 ms |
| 计算 | POST /api/traversal/calculateRealtime | < 200 ms |

---

### 2.2 ⚠️ AcquisitionHub 吞吐 / 并发

**审计目标**：`internal/usecase/acquisition.go`、`stream_relay.go`

**正面 ✅**：
- 锁外发送（line 87-103）避免阻塞其他设备数据处理
- 缓冲区满 select-default 丢弃，**不会反压**
- 丢包日志聚合（recordDrops）防刷屏
- publishHz 节流（line 73-77）保护下游

**发现 #1 ⚠️ P1：history slice 增长策略 alloc 频繁**

`acquisition.go:67-71`：

```go
history := append(h.historyByDevice[payload.DeviceID], payload)
if len(history) > h.historyCapacity {
    history = append([]device.DataPayload(nil), history[len(history)-h.historyCapacity:]...)
}
```

满 capacity 后**每次都重新分配 slice 并复制**（256 个 DataPayload），高 publish 频率下 GC 压力可观。

**建议**：用环形缓冲（ring buffer）或 `slice[1:]` 移位（虽然移位也是浅 copy）。

**发现 #2 ⚠️ P1：`stream_relay.go:46` 无 select 的阻塞发送**

```go
case payload, ok := <-ch:
    if !ok { unsub(); return }
    r.payloads <- payload    // ← 阻塞发送，无 default/ctx.Done()
```

- payloads channel 容量 64（line 22）
- 如果 Wails `EventsEmit` 慢或前端慢消费 → relay goroutine 阻塞 → hub 端 subscriber channel 满 → hub drop 触发
- 链路：hub drop 是症状，relay 阻塞是根因之一

**建议**：

```go
select {
case r.payloads <- payload:
case <-ctx.Done():
    return
default:
    // 计数丢弃；或保留最新
}
```

**发现 #3 ℹ️ P2：写锁粒度大**

`OnData` 持锁包括 history slice 重建 + map 写。N 设备高频时可能竞争。

**Budget（HIL 验证）**：
- 8 通道 × 1 kHz 持续 10 分钟丢包率 < 0.1%
- 端到端延迟（采集 → 前端） < 50 ms p95
- Heap 平稳，CPU < 30%

---

### 2.3 ✅ 数据库 / 持久化

**审计结论**：项目**不使用 SQL/ORM**。持久化全部走 CSV / JSON 文件：

| 路径 | 用途 |
|---|---|
| `internal/adapters/config/file_profile_store.go` | 设备 profile 持久化 |
| `internal/adapters/storage/csv_sink.go` | 采集数据落盘 |
| `internal/adapters/storage/calibration_csv_writer.go` | 标定 CSV |
| `internal/adapters/storage/traversal_csv_writer.go` | 遍历 CSV |
| `internal/adapters/storage/file_checkpoint_store.go` | 断点恢复 |
| `pkg/appcontext/context.go` | 应用上下文 |

**发现 #1 ⚠️ P1：所有写都是同步 `os.WriteFile`**

热路径（CSV 采集落盘）若在 request goroutine 上同步 fsync，会拖慢响应。需运行时确认是否在 hot path。

**Budget（运行时确认）**：
- CSV 批量 flush 间隔 ≥ 100 ms，单次写 < 10 ms
- profile 写仅在配置变更时发生，单次 < 50 ms 可接受

---

### 2.4 ⚠️ 内存 / GC / Goroutine

**审计目标**：goroutine 启动点 + alloc 模式

**发现 #1 ❌ P0：5 个 goroutine 启动点，0 个 `defer recover()`**

```bash
grep -rn "go func\|go [a-zA-Z]*(" internal/ pkg/  → 18 处（含 cmd）
grep -rn "defer recover"                          → 0 处
```

文件清单：
- `internal/usecase/calibration.go:144`
- `internal/usecase/stream_relay.go:37`
- `pkg/apiserver/apiserver.go:81, 85`
- `apps/desktop-wails/backend/app.go:144, 167` (frontend relay)

**风险**：任一 goroutine panic 会杀进程，标定/遍历测试期间丢数据。

**建议**：建一个 `safeGo` helper：

```go
func safeGo(name string, fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                slog.Error("goroutine panic", "name", name, "panic", r,
                    "stack", string(debug.Stack()))
            }
        }()
        fn()
    }()
}
```

**发现 #2 ℹ️ P2：未使用 `sync.Pool`**

热路径（acquisition, traversal CSV append）每次都新建 buffer。Profiler 确认前先不动。

---

### 2.5 ⚠️ 启动时间

**审计目标**：`internal/bootstrap/bootstrap.go`（124 行）+ `cmd/server/main.go`

**发现 #1 ⚠️ P1：`ensureDefaultProfiles` 同步加载 + 验证**

`bootstrap.go:51`：启动时同步读 profile 文件 + 校验默认值。若文件损坏会阻塞启动。

**发现 #2 ✅ Bootstrap 顺序合理**

无设备探测/网络调用，全部内存 wiring。启动应在 < 1 s。

**Budget（运行时）**：
- `wails dev` 启动到首页可点击 < 3 s
- 后端 BuildAPIServer 调用 < 500 ms

---

## 3. Wails 桥接层（apps/desktop-wails/backend）

### 3.1 ⚠️ Wails Binding 数量大

**审计目标**：`apps/desktop-wails/backend/app.go`（623 行）

**发现 #1 ⚠️ P1：30+ Bound methods，部分高频**

候选高频方法：
- `DeviceGetLatestData(deviceID)` — 若前端轮询代替 EventsEmit 路径，每秒多次调用
- `MotionGetStatus()` — 见 `motionApi.ts:422` 前端 250 ms 轮询

**Bind 调用 = JSON 序列化两次（Go → JS）**，频繁调用成本累积。

**发现 #2 ✅ 大数据走 EventsEmit 而非 binding return**

`app.go:154` `runtime.EventsEmit(ctx, "daq:payload", payload)` — 数据流不走 binding，这是正确设计。

**Budget**：单次 binding 往返 p95 < 5 ms（需前端 `performance.now()` 测）

---

### 3.2 ⚠️ IPC Payload 体积

**审计目标**：高频/大 payload 调用

**发现 #1 ⚠️ P1：`daq:payload` event 包含完整 channels 数组**

每次 `EventsEmit` 序列化整个 `DataPayload`（含 channels[]、channelIndices[]）。8 通道 + metadata ≈ 200–500 字节，100 Hz 下约 50 KB/s 序列化负担。**目前估计可接受**，但若通道数增加需关注。

**发现 #2 ⚠️ P1：遍历结果 traversal status 大 payload**

`api/server.go:465` `BuildStatusResponse()` 含 `dataPoints` 全量（可能数千点）。前端 `traversalApi.ts:89` 每 500 ms 轮询此接口。

**建议**：
- status 接口只返 metadata + delta（cursor）
- 历史 dataPoints 分页或 lazy 拉

---

## 4. shared/* 共享库

### 4.1 ⚠️ shared/algorithms — 缺基准测试

**审计目标**：`shared/algorithms/go/`

**发现 #1 ❌ P0：3 244 行核心算法代码、0 Benchmark**

```bash
grep -rn "^func Benchmark" shared/algorithms/
  (none)
```

主热路径 `FiveHoleNewInterpolator.Calculate`（`five_hole_new_interpolator.go:242`，复杂度高）**完全没有基准基线**。

**发现 #2 ⚠️ P1：构造期大量 map alloc，但 Calculate 路径未审计**

`five_hole_new_interpolator.go` 中：
- line 167, 385, 391, 491, 504, 583, 785–787, 816 等多处 `make(map…)` / `make([]float64…)`
- 大部分在 Load/Index 阶段（一次性），但 Calculate 内部 (line 242) 还需要看完整流程

**建议**：先加 Benchmark 测量 Calculate hot path，确定单次插值时间 + alloc 数：

```go
// shared/algorithms/go/fivehole/interpolation/benchmark_test.go
func BenchmarkFiveHoleCalculate(b *testing.B) {
  f := setupLoadedInterpolator(b)
  input := InterpolationInput{P1: ..., P2: ..., ...}
  b.ResetTimer()
  for i := 0; i < b.N; i++ {
    _, _ = f.Calculate(input)
  }
}
// 跑：go test -bench=. -benchmem ./shared/algorithms/...
```

**Budget**：单次 Calculate < 100 µs；100 µs × 1 kHz = 10% CPU，可接受

---

### 4.2 ℹ️ shared/device-sdk

**审计目标**：`shared/device-sdk/go/`

**发现**：
- 同样**无 Benchmark**
- 设备解析代码（daq_t1603, wtnmc4a 等）需运行时 profile
- 离线 unit test 存在 ✅

**P2**：等 4.1 算法基准跑出来后，按需补 device-sdk benchmark。

---

## 5. ❌ 可观测性整体缺失

**审计目标**：现有监控/日志/CI 仪表

| 项目 | 现状 | 缺口 |
|---|---|---|
| web-vitals 上报 | ❌ 未集成 | 加 `web-vitals` 包，在 `main.ts` 上报 |
| 前端 Long Task 检测 | ❌ | `PerformanceObserver` |
| Go API latency middleware | ❌ | 见 2.1 |
| `net/http/pprof` | ❌ | 未导入，无 heap/cpu profile 入口 |
| Bundle size CI | ❌ | 见 1.1 GUARD |
| Go benchmark CI | ❌ | 没有 `.github/workflows/` |
| Drop count 暴露指标 | ⚠️ | acquisition.go drop 仅 log，未暴露 /metrics |

**建议优先级**：

| 优先级 | 项 | 工作量 |
|---|---|---|
| P0 | Go API timing middleware（2.1）| 1 h |
| P0 | `net/http/pprof` dev 端点 | 15 min |
| P1 | web-vitals 上报到 console / 本地 log | 30 min |
| P1 | Bundle size CI guard | 1 h |
| P2 | drop count / 队列深度 metrics endpoint | 2 h |

---

## 6. 优先级汇总（按 ROI 排序）

| # | 描述 | 严重度 | 工作量 | 文档章节 |
|---|---|---|---|---|
| 1 | echarts 按需导入 + 移除首屏 preload | ❌ P0 | 2 h | 1.1 |
| 2 | RealtimeChart + deviceStore shallowRef 改造 | ❌ P0 | 4 h | 1.2 |
| 3 | Go API 加 latency / panic middleware | ❌ P0 | 1 h | 2.1 |
| 4 | `safeGo` helper + 5 处 goroutine 接入 | ❌ P0 | 2 h | 2.4 |
| 5 | 加 pprof dev endpoint | ❌ P0 | 15 min | 5 |
| 6 | shared/algorithms 加 Benchmark | ❌ P0 | 2 h | 4.1 |
| 7 | stream_relay.go select non-blocking 发送 | ⚠️ P1 | 1 h | 2.2 |
| 8 | traversalStore.dataPoints 改 shallowRef | ⚠️ P1 | 2 h | 1.4 |
| 9 | traversal status 接口分页/增量 | ⚠️ P1 | 4 h | 3.2 |
| 10 | i18nStore 用 markRaw | ⚠️ P1 | 30 min | 1.4 |
| 11 | web-vitals 上报 | ⚠️ P1 | 30 min | 5 |
| 12 | Bundle size CI | ⚠️ P1 | 1 h | 1.1 + 5 |
| 13 | AcquisitionHub history 环形缓冲 | ⚠️ P1 | 3 h | 2.2 |
| 14 | CSV 同步写审计 + 异步化 | ⚠️ P1 | 4 h | 2.3 |

**预估总工**：26 小时（约 1 周专注开发）

---

## 7. 推进顺序建议

```
本周（必做）
├── [P0 #5] pprof 端点（15 min — 给后续打基础）
├── [P0 #3] API latency middleware（1 h — 之后所有后端测量都靠它）
├── [P0 #1] echarts 按需导入（2 h — 用户体感立即改善）
├── [P0 #6] algorithms Benchmark（2 h — 建立硬数据基线）

下周
├── [P0 #4] safeGo（避免现场崩溃丢数据）
├── [P0 #2] RealtimeChart shallowRef（高频场景）
├── [P1 #11] web-vitals + #12 Bundle CI（GUARD 接通）

第三周
├── 剩余 P1 项按场景需要推进
```

---

## 8. 不动数值化"绝对值"，只动结构

本计划所有发现都是**静态分析**得出的，未运行任何 profile。下面这些**绝对数字**必须运行时测量后才能下结论：

- 实时图表 FPS / Long Task 数
- API p95 延迟具体数
- 五孔 Calculate 实际单次耗时
- CSV 写实际阻塞时长
- Hub drop 率（取决于硬件配速 vs 订阅者消费速）

**所以下一步**：先按 P0 接通仪表（pprof + middleware + benchmark + web-vitals），再用真数据填回这些 budget。

---

## 9. 验证文档存放位置

```
projects/WindLabX4/docs/runbooks/
├── perf-measurement-plan.md             ✅ 本文档
├── perf-frontend-bundle-baseline.md     ✅ 1.1 详细修复方案
├── perf-frontend-chart-baseline.md      ← 1.2 修复后补
├── perf-backend-api-baseline.md         ← 2.1 接通 middleware 后
├── perf-backend-acquisition-baseline.md ← 2.2 HIL 压测后
├── perf-shared-algorithms-baseline.md   ← 4.1 Benchmark 跑完后
└── perf-overview.md                     (汇总各 baseline 链接)
```

---

## 10. 立即行动建议

我建议下一步**只做"接通仪表"**，不动业务代码：

1. ⚡ 加 Go API timing middleware（1 h）
2. ⚡ 加 pprof dev 端点（15 min）
3. ⚡ 加 web-vitals 控制台上报（30 min）
4. ⚡ 加 shared/algorithms Benchmark（2 h）

合计 4 小时，**零业务风险**，之后所有"绝对值"决定才有数据支撑。

要执行哪几项？或者你想先看某一个问题的代码细节？
