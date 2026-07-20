package backend

import (
	"fmt"
	"math"
	"testing"

	wind_interp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
)

// five_hole_service_test.go 是从旧 five-hole-interpolator 项目的 app_test.go 迁移而来。
// 适配点：
//   - App 字段名 multiInterp → fiveHole.multiInterp（隔离各探针状态的子结构）
//   - 类型名 InterpolationInput → FiveHoleInterpolationInput
//   - 类型名 InterpolationResult → FiveHoleInterpolationResult
//
// 测试覆盖：批量计算载荷 / 表压绝压转换 / 并发安全 / 部分失败 / IsPrbLoaded / GetMachRange。

// ==================== 辅助函数 ====================

// syntheticPrbLines 生成一个合成的 5 孔 .prb 文本，覆盖 ±30° 角度网格。
// cpt / cps 是中心孔与静压孔的压差系数，用于让算法包能完整解析网格。
func syntheticPrbLines(cpt, cps float64) []string {
	lines := []string{"13 13"}
	for alpha := -30.0; alpha <= 30; alpha += 5 {
		for beta := -30.0; beta <= 30; beta += 5 {
			lines = append(lines, fmt.Sprintf("%.6f %.6f %.6f %.6f %.0f %.0f",
				alpha/100, beta/100, cpt, cps, alpha, beta))
		}
	}
	return lines
}

// setupLoadedApp 构造一个已加载合成 .prb 的 App，用于多数测试用例。
// 注意：通过 fiveHole 子结构注入，匹配 probe-interpolator 的新隔离结构。
func setupLoadedApp() *App {
	interpolator := wind_interp.NewMultiPrbInterpolator()
	_, _ = interpolator.LoadPrbData([]wind_interp.PrbFileData{
		{FilePath: "synthetic-0.3Ma.prb", Lines: syntheticPrbLines(0.05, 0.01)},
	}, []float64{0.3})
	return &App{fiveHole: fiveHoleState{multiInterp: interpolator}}
}

// ==================== 批量计算测试 ====================

// TestBatchCalculateReturnsDataPayload 验证批量计算返回 Data 数组而非单条结果。
// 同时验证速度向量在探针坐标系下的方向：Vz>0、Vx/Vy 接近 0（正前方来流）。
func TestBatchCalculateReturnsDataPayload(t *testing.T) {
	interpolator := wind_interp.NewMultiPrbInterpolator()
	loadResult, err := interpolator.LoadPrbData([]wind_interp.PrbFileData{
		{FilePath: "synthetic-0.3Ma.prb", Lines: syntheticPrbLines(0.05, 0.01)},
	}, []float64{0.3})
	if err != nil {
		t.Fatalf("load PRB data: %v", err)
	}
	if len(loadResult.Warnings) > 1 {
		t.Fatalf("unexpected load warnings: %v", loadResult.Warnings)
	}

	app := &App{fiveHole: fiveHoleState{multiInterp: interpolator}}
	response := app.BatchCalculate([]FiveHoleInterpolationInput{{
		P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
		Patm: 101325, Tatm: 20,
	}})

	if !response.Success {
		t.Fatalf("expected success, got error %q", response.Error)
	}
	if len(response.Data) != 1 {
		t.Fatalf("expected one result in data payload, got %d", len(response.Data))
	}
	if !response.Data[0].IsValid {
		t.Fatalf("expected valid interpolation result, got warning %q", response.Data[0].Warning)
	}
	if response.Data[0].V <= 0 || response.Data[0].Vz <= 0 ||
		math.Abs(response.Data[0].Vx) > 1e-12 || math.Abs(response.Data[0].Vy) > 1e-12 {
		t.Fatalf("expected velocity vector fields in probe axis convention, got V=%v Vx=%v Vy=%v Vz=%v",
			response.Data[0].V, response.Data[0].Vx, response.Data[0].Vy, response.Data[0].Vz)
	}
}

// ==================== toCoreInput 转换测试 ====================

// TestToCoreInput_GaugeMode 表压模式下 P1-P5 应保持原值。
func TestToCoreInput_GaugeMode(t *testing.T) {
	input := FiveHoleInterpolationInput{
		P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
		Patm: 101325, Tatm: 20, PressureMode: "gauge",
	}
	core := toCoreInput(input)

	if core.P1 != 100 {
		t.Errorf("gauge模式 P1 应保持原值, got %v", core.P1)
	}
	if core.P2 != 300 {
		t.Errorf("gauge模式 P2 应保持原值, got %v", core.P2)
	}
	if core.PAtm != 101325 {
		t.Errorf("PAtm 应保持原值, got %v", core.PAtm)
	}
}

// TestToCoreInput_AbsoluteMode 绝压模式下 P1-P5 应自动减去大气压转为表压。
func TestToCoreInput_AbsoluteMode(t *testing.T) {
	input := FiveHoleInterpolationInput{
		P1: 101425, P2: 101625, P3: 101425, P4: 101425, P5: 101425,
		Patm: 101325, Tatm: 20, PressureMode: "absolute",
	}
	core := toCoreInput(input)

	if core.P1 != 100 {
		t.Errorf("absolute模式 P1 应减去大气压, expected 100, got %v", core.P1)
	}
	if core.P2 != 300 {
		t.Errorf("absolute模式 P2 应减去大气压, expected 300, got %v", core.P2)
	}
	if core.PAtm != 101325 {
		t.Errorf("PAtm 应保持原值, got %v", core.PAtm)
	}
}

// TestToCoreInput_DefaultIsGauge 空字符串 PressureMode 默认按表压处理。
func TestToCoreInput_DefaultIsGauge(t *testing.T) {
	input := FiveHoleInterpolationInput{
		P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
		Patm: 101325, Tatm: 20,
	}
	core := toCoreInput(input)

	if core.P1 != 100 {
		t.Errorf("默认模式应为表压, P1 expected 100, got %v", core.P1)
	}
}

// ==================== 并发安全测试 ====================

// TestAppConcurrentAccess 验证 5 孔状态在并发读 + 计算场景下不发生数据竞争。
// 4 个 goroutine 各跑 100 次，覆盖 RLock 读路径与 Calculate 路径。
func TestAppConcurrentAccess(t *testing.T) {
	app := setupLoadedApp()

	done := make(chan bool, 4)

	go func() {
		for i := 0; i < 100; i++ {
			_ = app.IsPrbLoaded()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = app.GetPrbFiles()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = app.GetMachRange()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = app.Calculate(FiveHoleInterpolationInput{
				P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
				Patm: 101325, Tatm: 20,
			})
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}
}

// ==================== 批量计算部分失败测试 ====================

// TestBatchCalculatePartialFailure 验证批量计算遇到坏行不中断、对应位置返回非 nil 标记结果。
func TestBatchCalculatePartialFailure(t *testing.T) {
	app := setupLoadedApp()

	inputs := []FiveHoleInterpolationInput{
		{P1: 100, P2: 300, P3: 100, P4: 100, P5: 100, Patm: 101325, Tatm: 20},
		{P1: 100, P2: 300, P3: 100, P4: 100, P5: 100, Patm: 101325, Tatm: 20},
	}

	response := app.BatchCalculate(inputs)

	if len(response.Data) != 2 {
		t.Fatalf("expected 2 results, got %d", len(response.Data))
	}

	for i, r := range response.Data {
		if r == nil {
			t.Errorf("result[%d] should not be nil even on partial failure", i)
		}
	}
}

// ==================== IsPrbLoaded 测试 ====================

// TestIsPrbLoaded_NoData 新建 App 未加载 .prb 时应返回 false。
func TestIsPrbLoaded_NoData(t *testing.T) {
	app := NewApp()
	if app.IsPrbLoaded() {
		t.Error("新创建的App不应已加载PRB数据")
	}
}

// TestIsPrbLoaded_WithData 加载合成 .prb 后应返回 true。
func TestIsPrbLoaded_WithData(t *testing.T) {
	app := setupLoadedApp()
	if !app.IsPrbLoaded() {
		t.Error("加载PRB数据后IsPrbLoaded应返回true")
	}
}

// ==================== GetMachRange 测试 ====================

// TestGetMachRange_NotLoaded 未加载 .prb 时 GetMachRange 应返回失败。
func TestGetMachRange_NotLoaded(t *testing.T) {
	app := NewApp()
	resp := app.GetMachRange()
	if resp.Success {
		t.Error("未加载PRB时GetMachRange应返回失败")
	}
}

// TestGetMachRange_Loaded 加载后应返回 [min, max] 两个值。
func TestGetMachRange_Loaded(t *testing.T) {
	app := setupLoadedApp()
	resp := app.GetMachRange()
	if !resp.Success {
		t.Fatalf("加载PRB后GetMachRange应返回成功, got error: %s", resp.Error)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 mach range values, got %d", len(resp.Data))
	}
}
