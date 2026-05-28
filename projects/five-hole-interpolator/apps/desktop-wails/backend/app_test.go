package backend

import (
	"fmt"
	"math"
	"testing"

	wind_interp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
)

// ==================== 辅助函数 ====================

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

func setupLoadedApp() *App {
	interpolator := wind_interp.NewMultiPrbInterpolator()
	_, _ = interpolator.LoadPrbData([]wind_interp.PrbFileData{
		{FilePath: "synthetic-0.3Ma.prb", Lines: syntheticPrbLines(0.05, 0.01)},
	}, []float64{0.3})
	return &App{multiInterp: interpolator}
}

// ==================== 批量计算测试 ====================

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

	app := &App{multiInterp: interpolator}
	response := app.BatchCalculate([]InterpolationInput{{
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

func TestToCoreInput_GaugeMode(t *testing.T) {
	input := InterpolationInput{
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

func TestToCoreInput_AbsoluteMode(t *testing.T) {
	input := InterpolationInput{
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

func TestToCoreInput_DefaultIsGauge(t *testing.T) {
	input := InterpolationInput{
		P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
		Patm: 101325, Tatm: 20,
	}
	core := toCoreInput(input)

	if core.P1 != 100 {
		t.Errorf("默认模式应为表压, P1 expected 100, got %v", core.P1)
	}
}

// ==================== 并发安全测试 ====================

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
			_ = app.Calculate(InterpolationInput{
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

func TestBatchCalculatePartialFailure(t *testing.T) {
	app := setupLoadedApp()

	inputs := []InterpolationInput{
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

func TestIsPrbLoaded_NoData(t *testing.T) {
	app := NewApp()
	if app.IsPrbLoaded() {
		t.Error("新创建的App不应已加载PRB数据")
	}
}

func TestIsPrbLoaded_WithData(t *testing.T) {
	app := setupLoadedApp()
	if !app.IsPrbLoaded() {
		t.Error("加载PRB数据后IsPrbLoaded应返回true")
	}
}

// ==================== GetMachRange 测试 ====================

func TestGetMachRange_NotLoaded(t *testing.T) {
	app := NewApp()
	resp := app.GetMachRange()
	if resp.Success {
		t.Error("未加载PRB时GetMachRange应返回失败")
	}
}

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
