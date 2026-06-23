package interpolation

import (
	"testing"
)

// 五孔探针插值基准 —— 用于建立性能基线，配合 perf-measurement-plan.md 4.1 节。
//
// 运行：
//
//	cd shared/algorithms/go/fivehole/interpolation
//	go test -bench=. -benchmem -benchtime=2s
//
// 关注指标：
//   - ns/op：单次 Calculate 耗时；在 1 kHz 采集场景下应 ≪ 1ms（1_000_000 ns）
//   - B/op  ：单次分配字节
//   - allocs/op：单次分配次数，<10 为佳
//
// 三个 benchmark 覆盖典型路径：
//   - 角区（regionCorner）：用 AA1 公式，单次插值
//   - 边缘区（regionEdge）：用 AA2 公式，二次插值
//   - 扩展网格外推：触发 IsExtended 路径
//
// 加载耗时不进 b.N 循环 —— b.ResetTimer 之前完成。

func setupFiveHoleInterpolator(b *testing.B) *FiveHoleNewInterpolator {
	b.Helper()
	interpolator := NewFiveHoleNewInterpolator()
	// 7×7 网格，alpha/beta 从 -30 到 30 步进 10
	alphas := []float64{-30, -20, -10, 0, 10, 20, 30}
	betas := []float64{-30, -20, -10, 0, 10, 20, 30}
	if err := interpolator.LoadPrbLines(syntheticFiveHoleCsv(alphas, betas)); err != nil {
		b.Fatalf("LoadPrbLines: %v", err)
	}
	return interpolator
}

// BenchmarkFiveHoleCalculate_Corner 角区路径（小角度，落在网格中心附近，AA1 一次插值即出结果）
func BenchmarkFiveHoleCalculate_Corner(b *testing.B) {
	interpolator := setupFiveHoleInterpolator(b)
	input := inputForAngles(5, 5) // 小角度，落在 [0,10] 网格区间
	b.ReportAllocs()
	for b.Loop() {
		result, err := interpolator.Calculate(input)
		if err != nil {
			b.Fatalf("Calculate: %v", err)
		}
		if !result.IsValid {
			b.Fatalf("invalid: %s", result.Warning)
		}
	}
}

// BenchmarkFiveHoleCalculate_Edge 边缘区路径（中等角度，触发 AA2 二次插值）
func BenchmarkFiveHoleCalculate_Edge(b *testing.B) {
	interpolator := setupFiveHoleInterpolator(b)
	input := inputForAngles(18, 5) // alpha 接近边界，触发 edge region
	b.ReportAllocs()
	for b.Loop() {
		_, err := interpolator.Calculate(input)
		if err != nil {
			b.Fatalf("Calculate: %v", err)
		}
	}
}

// BenchmarkFiveHoleCalculate_Extended 扩展网格外推路径（超出原始网格）
func BenchmarkFiveHoleCalculate_Extended(b *testing.B) {
	interpolator := setupFiveHoleInterpolator(b)
	input := inputForAngles(35, 35) // 超出 ±30 网格，走扩展插值
	b.ReportAllocs()
	for b.Loop() {
		_, err := interpolator.Calculate(input)
		if err != nil {
			b.Fatalf("Calculate: %v", err)
		}
	}
}

// BenchmarkFiveHoleLoad 加载阶段基准（一次性成本，但启动延迟相关）
func BenchmarkFiveHoleLoad(b *testing.B) {
	alphas := []float64{-30, -20, -10, 0, 10, 20, 30}
	betas := []float64{-30, -20, -10, 0, 10, 20, 30}
	lines := syntheticFiveHoleCsv(alphas, betas)
	b.ReportAllocs()
	for b.Loop() {
		interpolator := NewFiveHoleNewInterpolator()
		if err := interpolator.LoadPrbLines(lines); err != nil {
			b.Fatalf("LoadPrbLines: %v", err)
		}
	}
}
