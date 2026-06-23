# shared/algorithms 性能基线

> 测量日期：2026-06-23
> 硬件：13th Gen Intel(R) Core(TM) i5-13600K，20 逻辑核
> Go 版本：见 `go.work`
> 基准代码：`shared/algorithms/go/fivehole/interpolation/five_hole_new_interpolator_bench_test.go`
> 工作流：MEASURE（已完成）→ IDENTIFY → FIX → VERIFY → GUARD

---

## 1. 测量命令

```bash
cd shared/algorithms/go/fivehole/interpolation
go test -bench=. -benchmem -benchtime=2s -run=^$
```

`-run=^$` 跳过单元测试只跑 benchmark；`-benchtime=2s` 每个 benchmark 至少跑 2 秒求稳定。

---

## 2. 基线结果（7×7 网格 fixture）

| 基准 | ns/op | B/op | allocs/op | 含义 |
|---|---:|---:|---:|---|
| `FiveHoleCalculate_Corner` | **4 071** | 3 032 | 136 | 角区路径（AA1 一次插值）|
| `FiveHoleCalculate_Edge` | **3 381** | 2 512 | 110 | 边缘区路径（AA2 二次插值）|
| `FiveHoleCalculate_Extended` | **13 300** | 12 440 | 622 | 扩展网格外推（最慢路径）|
| `FiveHoleLoad` | 348 762 | 230 395 | 3 816 | 加载阶段（一次性）|

### 2.1 关键观察

✅ **单次插值绝对耗时合理**：4–13 µs，远低于 1 ms 预算。

⚠️ **alloc 数偏高**：
- Corner / Edge 路径每次 100+ alloc，3 KB 内存 → 1 kHz 调用时 = **3 MB/s GC 压力**
- Extended 路径单次 622 alloc / 12 KB → 高频外推会触发频繁 GC

⚠️ **Extended 路径比 Corner 慢 3.3×**：网格外推有改进空间，但典型工况下数据点应该都在网格内，Extended 出现频率应该 <5%。

---

## 3. Budget 评估

| 指标 | 实测 | budget | 状态 |
|---|---|---|---|
| Corner 单次耗时 | 4.07 µs | < 100 µs | ✅ |
| Edge 单次耗时 | 3.38 µs | < 100 µs | ✅ |
| Extended 单次耗时 | 13.30 µs | < 100 µs | ✅ |
| Corner allocs/op | 136 | < 30 | ⚠️ 偏高 |
| Extended allocs/op | 622 | < 100 | ❌ |
| Load 耗时 | 0.35 ms | < 100 ms | ✅ |

### 3.1 1 kHz 场景 CPU 推算

- Corner 4 µs/次 × 1000 次/秒 = **4 ms/s = 0.4% CPU**
- Extended 13 µs/次 × 1000 次/秒 = **13 ms/s = 1.3% CPU**

CPU 完全不是瓶颈。**GC 压力**才是真问题（3–12 MB/s）。

---

## 4. 优化建议（不要现在动）

按 ROI 排序，**等运行时确认 GC 是否真的成为瓶颈再做**：

### P1：减少 Extended 路径 alloc
- 622 次 alloc 来自哪？需要 `go test -bench=...-Extended -memprofile=mem.out` 看
- 大概率是 extension grid lookup 时反复 `make(map…)` / `append`
- 可考虑预分配复用 buffer

### P1：Calculate 入口加 sync.Pool
- 如果 GC 真的是热点，对 InterpolationResult / 临时切片用 sync.Pool

### P2：Corner/Edge alloc 100+ 也偏高
- 但绝对值 3 KB，1 kHz 下 3 MB/s 不算极端
- 排查目标：bilinearInterpolateCoefficient 内是否每次构造小 map/slice

---

## 5. 何时复测

- 修改任何 `five_hole_new_interpolator.go` 主路径函数后
- 修改 `prb_interpolator.go` / `multi_prb_interpolator.go` 后
- 大版本前（grafana baseline 对比）

### 复现脚本（可放 CI）

```bash
go test -bench=. -benchmem -benchtime=2s -run=^$ \
  ./shared/algorithms/go/fivehole/interpolation \
  | tee bench-current.txt

# 与基线对比：
go install golang.org/x/perf/cmd/benchstat@latest
benchstat bench-baseline.txt bench-current.txt
```

---

## 6. 未覆盖的基准（TODO）

- `shared/algorithms/go/threehole/interpolation` — 三孔探针，存在 `three_hole_test.go` 但无 benchmark
- `multi_prb_interpolator` 的 multi-Mach 线性混合路径
- `prb_interpolator` 独立 benchmark

**优先级 P2**：先用五孔基线确认整体 GC 行为，再补三孔与 PRB。

---

## 7. 整体结论

- **CPU 不是瓶颈**（最坏情况 1.3% CPU @ 1 kHz）
- **关注点是 alloc 数与 GC 压力**，特别是 Extended 路径
- **没有运行时 profile 之前不动**；建议下一步在 hot 场景跑 `pprof -alloc_objects` 看真实分布
- 当前基线足够当 GUARD：任何 PR 让 Corner > 5 µs 或 allocs > 200 都视为回归
