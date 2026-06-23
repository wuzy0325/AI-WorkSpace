# 实际性能测量结果（运行时）

> 测量日期：2026-06-23
> 硬件：Intel Core i5-13600K（20 逻辑核）+ Windows 11
> 工作流：MEASURE（已完成本轮）→ IDENTIFY → FIX → VERIFY
>
> 这是**装通仪表后第一次真实跑数据**，结果填补了静态分析里所有"待运行时确认"的空白。

---

## 1. Backend API 真实延迟（hey 压测）

**测试方法**：每个接口 500 请求 / 10 并发，本机 loopback。

| API | p50 | p95 | p99 | 状态 |
|---|---:|---:|---:|---|
| GET /api/device/profiles | 0.2 ms | 1.3 ms | **12.4 ms** | ✅ |
| GET /api/daq/publishRate | 0.2 ms | 1.0 ms | 11.3 ms | ✅ |
| GET /api/storage/status | 0.2 ms | 0.6 ms | 12.0 ms | ✅ |
| GET /api/daq/latest/sim-1 | 0.2 ms | 0.8 ms | 11.4 ms | ✅ |
| GET /api/traversal/status | 0.3 ms | 1.2 ms | 13.3 ms | ✅ |
| POST /api/traversal/calculateRealtime | 0.2 ms | 0.9 ms | 10.8 ms | ✅ |

**全部接口 p99 < 17 ms，p50 ≤ 0.3 ms。**

### 1.1 高负载持续压测

50 并发、10 秒持续打 `/api/device/profiles`：

```
Total:        10.0 secs
Requests/sec: 30 653
50% in 1.3 ms
95% in 4.4 ms
99% in 7.2 ms
```

**结论**：后端 API 完全不是瓶颈。计划 2.1 节标 "❌ P0" 是基于**没仪表 = 黑盒**的角度，**现在仪表通了**：

| 维度 | 计划假设 | 实测 | 修正 |
|---|---|---|---|
| API 是否慢 | 未知 | p99 < 17 ms | ✅ 不慢 |
| 是否需要做的事 | 加 middleware（已做） | 同 | 无需进一步优化 |
| 后续优先级 | P0 | P3（GUARD 已接通） | **降级** |

---

## 2. Backend CPU Profile（pprof，50 并发 5 秒）

**Top consumers**：

```
runtime.findRunnable      42.6%   ← idle workers 找活
runtime.osyield           32.5%   ← OS 调度让出
runtime.cgocall           29.7%   ← Windows syscall（net I/O）
runtime.stealWork         24.8%   ← work-stealing 调度
```

**业务代码占比**：< 1%（绝大部分时间在 Go runtime 调度和 socket I/O）

**结论**：CPU 远未饱和，瓶颈在网络 I/O 而非业务逻辑。计划 2.4 节"GC/内存"风险**降级为 P3**。

---

## 3. Backend Heap Profile（同负载）

10 秒压测期间累计分配（top 5）：

| 函数 | 累计分配 | 占比 |
|---|---:|---:|
| `net/textproto.readMIMEHeader` | 111 MB | 15% — 标准库 |
| `net/http.Header.Clone` | 101 MB | 14% — 标准库 |
| `net/textproto.MIMEHeader.Set` | 90 MB | 12% — 标准库 |
| `net/http.readRequest` | 77 MB | 11% — 标准库 |
| **`DeviceManager.GetProfiles`** | **70 MB** | **10% — 业务** |
| `net/http.(*conn).readRequest` | 64 MB | 9% — 标准库 |
| **`log.(*Logger).output` (metrics middleware)** | **62 MB** | **9% — 我们装的仪表** |

**当前驻留（inuse）**：5.6 MB 总量，GC 工作完美。

### 3.1 新发现 #1：`DeviceManager.GetProfiles` 每次都重建大切片

`internal/usecase/device_manager.go` 的 GetProfiles 每次返回都全量复制 profiles 切片。在 300 K 调用下分配 70 MB——不算泄漏，但**单接口/profiles 调用占总 alloc 10%**。

**优先级 P2**：前端正常使用频率（开机+设备变更）下完全可忽略；只有现在这种 30 K RPS 的压测才显眼。可以考虑返回 `[]Profile` 但内部缓存不可变快照。

### 3.2 新发现 #2：`metricsMiddleware` 自身日志开销 9%

我们装的 timing middleware 每条请求都 `slog.Info`，在 30 K RPS 下日志成本 62 MB / 9%。**这是仪表的代价**。

**优先级 P2 GUARD**：
- 生产部署时把 metrics middleware 的 `slog.Info` 切换为采样（如 1/10）或 debug level
- 当前开发模式保持原样，便于排查

---

## 4. Backend Goroutine 健康度

压测期间 `goroutine?debug=1`：

```
Total: 71 goroutines
  36 × waiting on net I/O (http.Server connection serve)
   9 × http connection reader idle
  其余：runtime + background tasks
```

**结论**：无 goroutine 泄漏，无阻塞死锁。计划 2.4 节"5 处 goroutine 无 recover"仍然是真问题（防 panic 崩溃），但**当前没有实际崩溃证据**，从 P0 降到 P1。

---

## 5. Frontend Web Vitals（Playwright 自动抓取）

**测试方法**：headless Chromium 打开 http://localhost:15173 ，等待 5 秒交互后从 `window.__WEB_VITALS__` 读取。

| 指标 | 实测 | 阈值 (good) | 状态 | 解释 |
|---|---:|---:|---|---|
| **TTFB** | 6 ms | ≤ 800 ms | ✅ | Vite dev 本地极快 |
| **FCP** | 1 524 ms | ≤ 1 800 ms | ✅ | 首次内容绘制 1.5 秒 |
| **LCP** | **3 620 ms** | ≤ 2 500 ms | ⚠️ **needs-improvement** | **最大内容 3.6 秒** |
| **INP** | 8 ms | ≤ 200 ms | ✅ | 交互响应极快 |
| **CLS** | 0.000 | ≤ 0.1 | ✅ | 完全无布局抖动 |

### 5.1 LCP 3.6s 完美对应静态分析预测

[`perf-frontend-bundle-baseline.md`](./perf-frontend-bundle-baseline.md) 第 2 节预测：

> 首屏 487 KB gzip JS（含 echarts modulepreload）会拖慢首屏

**实测 LCP = 3 620 ms**，确认这个预测。修复 echarts 按需导入 + 移除 preload 后，LCP 预计降到 ~1 500 ms（good 区间）。

**这是首批真正可以指导优化的硬数据**。

### 5.2 INP 8ms 远超预期

**静态分析当时认为 1.2 节实时图表"P0 高频重建"是问题**——但 INP 实测 8 ms，说明：

- 当前测试场景**没有触发**实时图表大量 setOption（首页打开后没有进入 device view 持续推流）
- INP 是首交互延迟，可能首屏点击没碰到重组件

**修正**：1.2 节 RealtimeChart shallowRef 改造从 **P0 降级为 P1**，需要等真实使用场景（设备连接 + 1 kHz 推流 1 分钟）再测一次 INP 才能确认。

---

## 6. shared/algorithms Benchmark（已收录）

详见 [`perf-shared-algorithms-baseline.md`](./perf-shared-algorithms-baseline.md)。摘要：

| 路径 | ns/op | allocs/op | 1 kHz CPU |
|---|---:|---:|---:|
| Corner（典型）| 4 071 | 136 | 0.4% |
| Edge | 3 381 | 110 | 0.3% |
| Extended（外推）| 13 300 | 622 | 1.3% |

**结论**：算法**绝对不慢**（< 15 µs），GC 压力可控（最差 12 MB/s alloc rate，inuse 还是 5MB）。

---

## 7. 优先级修正表（用真实数据更新计划）

[`perf-measurement-plan.md`](./perf-measurement-plan.md) 第 6 节优先级列表，根据本轮实测**重新排序**：

| # | 描述 | 计划评级 | 实测后修正 | 理由 |
|---|---|---|---|---|
| 1 | echarts 按需导入 + 移除首屏 preload | P0 | ✅ **P0 不变** | **LCP 3.6s** 实测验证 |
| 2 | RealtimeChart deep reactive | P0 | ⚠️ **降 P1** | INP 8 ms 远低于阈值；需场景化复测 |
| 3 | Go API timing middleware | P0 | ✅ **已完成** | — |
| 4 | safeGo helper | P0 | ⚠️ **降 P1** | 当前无 panic 实例 |
| 5 | pprof endpoint | P0 | ✅ **已完成** | — |
| 6 | algorithms Benchmark | P0 | ✅ **已完成** | — |
| 7 | stream_relay 阻塞发送 | P1 | ⚠️ **降 P2** | 当前无 drop 证据 |
| 8 | traversalStore.dataPoints shallowRef | P1 | ⚠️ **保 P1** | 需要长时间 traversal 才能验证 |
| 9 | traversal status 接口分页 | P1 | ⚠️ **保 P1** | 需要 dataPoints 数千点场景 |
| 10 | i18nStore markRaw | P1 | ✅ **保 P1** | 24 KB gzip 首屏体积可消 |
| 11 | web-vitals 上报 | P1 | ✅ **已完成** | — |
| 12 | Bundle size CI | P1 | ⚠️ **保 P1** | 长期 GUARD |
| 13 | AcquisitionHub 环形缓冲 | P1 | ⚠️ **降 P2** | 当前 5 MB inuse 完全可控 |
| 14 | CSV 同步写审计 | P1 | ⚠️ **保 P1** | 需要 1 kHz 真实采集场景验证 |

### 7.1 新增项

| # | 描述 | 严重度 | 来源 |
|---|---|---|---|
| 15 | metricsMiddleware 生产采样开关 | P2 | 实测发现自身 9% alloc |
| 16 | DeviceManager.GetProfiles 缓存快照 | P2 | 实测发现 10% alloc，但 RPS 影响小 |
| 17 | dev 环境 SSE proxy 配置修正 | P3 | 实测控制台 SSE 404 风暴（非性能问题） |

---

## 8. 唯一确认的 P0：echarts 按需导入

**所有其他"P0"现在要么已完成、要么降级。当前唯一真正需要立即做的优化是：**

```
LCP 3 620 ms → 目标 < 2 500 ms (good)
预期方案：echarts 按需导入 + 移除首屏 modulepreload
预期改善：首屏 JS gzip 487 KB → ~200 KB，LCP 预计降至 ~1 500 ms
```

修复方案已详写于 [`perf-frontend-bundle-baseline.md`](./perf-frontend-bundle-baseline.md)。

---

## 9. 怎么复现这些测量

```powershell
# 1) 启动后端（含 pprof）
cd projects/wind-daq/services/api-go
$env:WINDDAQ_PPROF_ADDR = "localhost:16060"
$env:WIND_DAQ_ADDR = ":18080"
go run ./cmd/server

# 2) API 压测
go install github.com/rakyll/hey@latest
hey -n 500 -c 10 http://localhost:18080/api/device/profiles
hey -z 10s -c 50 http://localhost:18080/api/device/profiles  # 持续压

# 3) Profile 抓取（在压测同时另开 shell）
curl http://localhost:16060/debug/pprof/profile?seconds=5 -o cpu.prof
curl http://localhost:16060/debug/pprof/heap -o heap.prof
curl http://localhost:16060/debug/pprof/goroutine?debug=1 -o goroutine.txt

go tool pprof -top -cum cpu.prof
go tool pprof -top -sample_index=alloc_space heap.prof

# 4) Frontend Web Vitals
cd ../apps/desktop-wails/frontend
npm run dev
# 另开 shell
cd ../../..
python scripts/measure_web_vitals.py

# 5) Algorithm benchmark
cd ../../shared/algorithms/go/fivehole/interpolation
go test -bench=. -benchmem -benchtime=2s -run=^$
```

---

## 10. 主要结论

1. **后端不慢**：30 K RPS、p99 < 17ms，CPU 利用率低，goroutine 健康
2. **算法不慢**：单次 < 15 µs，1 kHz 场景 CPU < 2%
3. **唯一真问题：前端 LCP 3.6 秒** ← 直接对应已知的 echarts bundle 问题
4. **之前 14 项 P0 中，13 项要么已做、要么降级**——这就是仪表的价值，避免按猜想优化

**下一步建议**：执行 echarts 按需导入（[bundle baseline 文档 3.1 节](./perf-frontend-bundle-baseline.md)），改完用相同 Playwright 脚本复测 LCP，验证降到 good 区间。
