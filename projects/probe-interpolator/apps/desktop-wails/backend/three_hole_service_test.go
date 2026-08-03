package backend

import (
	"sync"
	"testing"

	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
)

// three_hole_service_test.go 是从旧 three-hole-interpolator 项目的 app_test.go 迁移而来。
// 适配点：
//   - App 字段名 multiInterp → threeHole.multiInterp（隔离各探针状态的子结构）
//   - 类型名 InterpolationInput → ThreeHoleInterpolationInput
//   - 类型名 InterpolationResult → ThreeHoleInterpolationResult（应用层）
//   - 函数名 toCoreInput → toThreeHoleCoreInput
//   - 函数名 toAppResult → toThreeHoleAppResult
//
// 测试覆盖：表压绝压转换 / 结果映射 / 批量计算载荷 / 并发安全 / 部分失败 /
// IsThreeHolePrbLoaded / GetThreeHoleMachRange。

// ==================== 辅助函数 ====================

// realThreeHolePrbLines 是真实 3 孔 .prb 文件（CMa=0.801, 13 个角度点）的内容副本。
// 直接用真实校准数据而非合成数据，避免合成数据不满足算法物理约束导致测试误判。
// 数据来源：shared/algorithms/go/threehole/interpolation/testdata/0.8.prb
func realThreeHolePrbLines() []string {
	return []string{
		"0.801",
		"13",
		"-3.910702\t1.502459\t4.895834\t-30",
		"-2.644388\t0.839334\t3.419670\t-25",
		"-1.773387\t0.439397\t2.804780\t-20",
		"-1.174992\t0.220226\t2.512503\t-15",
		"-0.732713\t0.092333\t2.253670\t-10",
		"-0.367181\t0.025364\t2.088049\t-5",
		"-0.027755\t0.003032\t2.000981\t0",
		"0.293167\t0.006285\t1.992505\t5",
		"0.615228\t0.045088\t2.021682\t10",
		"0.948513\t0.117905\t2.129097\t15",
		"1.360212\t0.257700\t2.357089\t20",
		"1.889637\t0.472875\t2.649158\t25",
		"2.633085\t0.828578\t2.994849\t30",
	}
}

// setupThreeHoleLoadedApp 构造一个已加载真实 .prb 的 App，用于多数测试用例。
// 注意：通过 threeHole 子结构注入，匹配 probe-interpolator 的新隔离结构。
func setupThreeHoleLoadedApp() *App {
	interpolator := three_interp.NewThreeHoleInterpolator()
	_, _ = interpolator.LoadPrbData([]three_interp.PrbFileData{
		{FilePath: "0.8.prb", Lines: realThreeHolePrbLines()},
	})
	return &App{threeHole: threeHoleState{multiInterp: interpolator}}
}

// ==================== 输入转换测试（迁移自旧 app_test.go） ====================

// TestToThreeHoleCoreInputGauge 验证 gauge 模式下压力值原样传递给算法包。
func TestToThreeHoleCoreInputGauge(t *testing.T) {
	input := ThreeHoleInterpolationInput{
		P1: 100000, P2: 101000, P3: 102000,
		Patm: 101325, Tatm: 20,
		PressureMode: "gauge",
	}

	core := toThreeHoleCoreInput(input)
	if core.P1 != 100000 || core.P2 != 101000 || core.P3 != 102000 {
		t.Errorf("gauge 模式应原样传递: got P1=%f, P2=%f, P3=%f", core.P1, core.P2, core.P3)
	}
}

// TestToThreeHoleCoreInputAbsolute 验证 absolute 模式下自动减去大气压转为表压。
func TestToThreeHoleCoreInputAbsolute(t *testing.T) {
	input := ThreeHoleInterpolationInput{
		P1: 201325, P2: 202325, P3: 203325,
		Patm: 101325, Tatm: 20,
		PressureMode: "absolute",
	}

	core := toThreeHoleCoreInput(input)
	if core.P1 != 100000 || core.P2 != 101000 || core.P3 != 102000 {
		t.Errorf("absolute 模式应减去 Patm: got P1=%f, P2=%f, P3=%f", core.P1, core.P2, core.P3)
	}
}

// TestToThreeHoleCoreInputDefaultGauge 验证未指定 PressureMode 时默认按表压处理。
func TestToThreeHoleCoreInputDefaultGauge(t *testing.T) {
	input := ThreeHoleInterpolationInput{
		P1: 100000, P2: 101000, P3: 102000,
		Patm: 101325, Tatm: 20,
		// PressureMode 留空
	}

	core := toThreeHoleCoreInput(input)
	if core.P1 != 100000 || core.P2 != 101000 || core.P3 != 102000 {
		t.Errorf("默认应按 gauge 处理: got P1=%f, P2=%f, P3=%f", core.P1, core.P2, core.P3)
	}
}

// TestToThreeHoleAppResult 验证算法包结果到应用层结果的字段映射。
func TestToThreeHoleAppResult(t *testing.T) {
	core := three_interp.InterpolationResult{
		Alpha:          5.0,
		MachNumber:     0.5,
		TotalPressure:  150000,
		StaticPressure: 100000,
		IterationCount: 5,
		Calculated:     true,
		IsValid:        true,
	}

	r := toThreeHoleAppResult(core)
	if r.Alpha != 5.0 {
		t.Errorf("Alpha = %f, want 5.0", r.Alpha)
	}
	if r.MachNumber != 0.5 {
		t.Errorf("MachNumber = %f, want 0.5", r.MachNumber)
	}
	if !r.IsValid {
		t.Error("IsValid 应为 true")
	}
	if !r.Calculated {
		t.Error("Calculated 应为 true")
	}
	if r.IterationCount != 5 {
		t.Errorf("IterationCount = %d, want 5", r.IterationCount)
	}
	if r.TotalPressure != 150000 {
		t.Errorf("TotalPressure = %f, want 150000", r.TotalPressure)
	}
	if r.StaticPressure != 100000 {
		t.Errorf("StaticPressure = %f, want 100000", r.StaticPressure)
	}
}

// ==================== 批量计算 / 状态查询测试 ====================

// TestBatchCalculateThreeHoleReturnsDataPayload 验证批量计算返回 Data 数组而非单条结果，
// 且数组长度与输入行数一致（即使部分失败也应占位）。
// 使用真实校准工况的输入压力（参考 golden_case_01：α=0、Ma≈0.8）。
func TestBatchCalculateThreeHoleReturnsDataPayload(t *testing.T) {
	app := setupThreeHoleLoadedApp()

	inputs := make([]ThreeHoleInterpolationInput, 0, 3)
	// 围绕中心工况做小扰动，保证 deltaP=2*P2-P1-P3 显著大于 0
	for _, dp := range []float64{0, 50, -50} {
		inputs = append(inputs, ThreeHoleInterpolationInput{
			P1:   37227.7 + dp,
			P2:   49908.4,
			P3:   36503.7 - dp,
			Patm: 101425,
			Tatm: 20,
		})
	}

	resp := app.BatchCalculateThreeHole(inputs)
	if !resp.Success && resp.Data == nil {
		t.Fatalf("批量计算应返回 Data 数组（即便部分失败），got Success=%v Data=%v", resp.Success, resp.Data)
	}
	if len(resp.Data) != len(inputs) {
		t.Errorf("Data 长度 = %d, want %d（每行都应有结果占位）", len(resp.Data), len(inputs))
	}
}

// TestThreeHoleConcurrentAccess 验证多 goroutine 并发调用 CalculateThreeHole 不会触发 race。
// 配合 `go test -race` 检测，确保 threeHoleState 的 RWMutex 保护正确。
func TestThreeHoleConcurrentAccess(t *testing.T) {
	app := setupThreeHoleLoadedApp()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			// 围绕真实工况做小扰动，保证 deltaP 不为零
			input := ThreeHoleInterpolationInput{
				P1:   37227.7 + float64(idx),
				P2:   49908.4,
				P3:   36503.7 - float64(idx),
				Patm: 101425,
				Tatm: 20,
			}
			_ = app.CalculateThreeHole(input)
			_ = app.IsThreeHolePrbLoaded()
			_ = app.GetThreeHoleMachRange()
		}(i)
	}
	wg.Wait()
}

// TestBatchCalculateThreeHolePartialFailure 验证批量计算遇到坏行时不中断，标记失败继续。
// 第 1 行用真实校准工况（α≈0、Ma≈0.8），第 2 行让 deltaP=2*P2-P1-P3=0 触发算法边界。
func TestBatchCalculateThreeHolePartialFailure(t *testing.T) {
	app := setupThreeHoleLoadedApp()

	inputs := []ThreeHoleInterpolationInput{
		{P1: 37227.7, P2: 49908.4, P3: 36503.7, Patm: 101425, Tatm: 20}, // deltaP≈26000，正常
		{P1: 100000, P2: 100000, P3: 100000, Patm: 101425, Tatm: 20},    // deltaP=0，触发边界
	}

	resp := app.BatchCalculateThreeHole(inputs)
	if resp.Data == nil {
		t.Fatalf("Data 不应为 nil")
	}
	if len(resp.Data) != 2 {
		t.Fatalf("Data 长度 = %d, want 2", len(resp.Data))
	}
	// 第 1 行应成功（IsValid=true）
	if !resp.Data[0].IsValid {
		t.Errorf("第 1 行应成功，got IsValid=false Warning=%q", resp.Data[0].Warning)
	}
	// 第 2 行应失败（IsValid=false 且有 Warning）
	if resp.Data[1].IsValid {
		t.Errorf("第 2 行应失败（deltaP=0），got IsValid=true")
	}
	if resp.Data[1].Warning == "" {
		t.Error("第 2 行失败时 Warning 应有提示")
	}
}

// TestIsThreeHolePrbLoaded_NoData 验证未加载 .prb 时 IsThreeHolePrbLoaded 返回 false。
func TestIsThreeHolePrbLoaded_NoData(t *testing.T) {
	app := &App{}
	if app.IsThreeHolePrbLoaded() {
		t.Error("未加载 .prb 时应返回 false")
	}
}

// TestIsThreeHolePrbLoaded_WithData 验证加载 .prb 后 IsThreeHolePrbLoaded 返回 true。
func TestIsThreeHolePrbLoaded_WithData(t *testing.T) {
	app := setupThreeHoleLoadedApp()
	if !app.IsThreeHolePrbLoaded() {
		t.Error("加载 .prb 后应返回 true")
	}
}

// TestGetThreeHoleMachRange_NotLoaded 验证未加载时返回失败响应。
func TestGetThreeHoleMachRange_NotLoaded(t *testing.T) {
	app := &App{}
	resp := app.GetThreeHoleMachRange()
	if resp.Success {
		t.Error("未加载时 Success 应为 false")
	}
	if resp.Error == "" {
		t.Error("未加载时 Error 应有提示")
	}
}

// TestGetThreeHoleMachRange_Loaded 验证加载后返回 [min, max] 范围。
func TestGetThreeHoleMachRange_Loaded(t *testing.T) {
	app := setupThreeHoleLoadedApp()
	resp := app.GetThreeHoleMachRange()
	if !resp.Success {
		t.Fatalf("加载后 Success 应为 true, got error: %s", resp.Error)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("MachRange 长度 = %d, want 2", len(resp.Data))
	}
	// 真实 .prb 的 CMa=0.801，单文件加载时 min=max=0.801
	if resp.Data[0] != 0.801 || resp.Data[1] != 0.801 {
		t.Errorf("MachRange = [%f, %f], want [0.801, 0.801]", resp.Data[0], resp.Data[1])
	}
}
