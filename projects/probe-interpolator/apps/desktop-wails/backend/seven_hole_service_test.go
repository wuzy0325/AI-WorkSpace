package backend

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	seven_interp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

// seven_hole_service_test.go 是 7 孔探针插值服务的单元测试。
//
// 测试数据来源：shared/algorithms/go/sevenhole/interpolation/testdata/prb/{1..7}.prb
// 该 PRB 文件集与算法包 golden 测试共用，保证测试输入满足算法物理约束。
//
// 测试覆盖：
//   - 输入转换（toSevenHoleCoreInput）：表压直传，无 PressureMode 转换
//   - 结果映射（toSevenHoleAppResult）：字段一一对应
//   - 内区插值（golden.json idx=87，pure sideslip α=15°, β≈0°）
//   - 外区插值（golden.json idx=290，sector=3 一般扇区内点）
//   - 批量计算 Data 数组长度与部分失败容错
//   - 并发安全（配合 -race 检测 RWMutex 正确性）
//   - 加载状态查询（IsSevenHolePrbLoaded / GetSevenHoleValidRange）
//
// 适配点（与 5 孔 / 3 孔测试风格一致）：
//   - 通过 sevenHole 子结构注入，匹配 probe-interpolator 的隔离结构
//   - 真实 PRB 文件通过 runtime.Caller 定位，避免硬编码绝对路径

// ==================== 辅助函数 ====================

// sevenHolePrbDir 返回 7 孔 PRB 测试数据所在目录。
// 通过 runtime.Caller 定位本测试文件，向上回溯 5 级到 repo root，
// 再拼接 shared/algorithms/go/sevenhole/interpolation/testdata/prb/。
// 失败时返回空字符串，调用方据此 Skip。
func sevenHolePrbDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	for i := 0; i < 5; i++ {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, "shared", "algorithms", "go", "sevenhole", "interpolation", "testdata", "prb")
}

// readSevenHoleTestPrb 读取指定名称的 7 孔 PRB 测试文件并返回非空行切片。
// 文件缺失时 Skip 而非 Fatal，避免在未拉取子模块的环境下整批测试失败。
func readSevenHoleTestPrb(t *testing.T, name string) []string {
	t.Helper()
	dir := sevenHolePrbDir(t)
	if dir == "" {
		t.Skipf("无法定位 7 孔 PRB 测试数据目录")
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读取 %s 失败: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// setupSevenHoleLoadedApp 构造一个已加载真实 PRB 文件集的 App，用于多数测试用例。
// 加载顺序与生产代码一致：内区（7.prb）最先，外区 1..6 顺序跟随。
// prbFiles 字段同步填充，与生产代码 LoadSevenHolePrbFiles 行为对齐。
func setupSevenHoleLoadedApp(t *testing.T) *App {
	t.Helper()
	interpolator := seven_interp.NewSevenHolePrbInterpolator()

	innerLines := readSevenHoleTestPrb(t, "7.prb")
	if err := interpolator.LoadInnerPrbLines(innerLines, "7.prb"); err != nil {
		t.Fatalf("加载内区 7.prb 失败: %v", err)
	}

	prbFiles := []SevenHolePrbFileInfo{
		{FilePath: "7.prb", FileName: "7.prb", Sector: 0},
	}

	for sector := 1; sector <= 6; sector++ {
		name := string(rune('0' + sector)) + ".prb"
		outerLines := readSevenHoleTestPrb(t, name)
		if err := interpolator.LoadOuterPrbLines(sector, outerLines, name); err != nil {
			t.Fatalf("加载外区 %s 失败: %v", name, err)
		}
		prbFiles = append(prbFiles, SevenHolePrbFileInfo{
			FilePath: name, FileName: name, Sector: sector,
		})
	}

	if !interpolator.IsLoaded() {
		t.Fatal("7 孔 PRB 加载后 IsLoaded 应为 true")
	}

	return &App{sevenHole: sevenHoleState{interpolator: interpolator, prbFiles: prbFiles}}
}

// ==================== 输入转换测试 ====================

// TestToSevenHoleCoreInputDirectMapping 验证 7 孔输入字段直传，无 PressureMode 转换。
// spec §1.1 强制 7 孔输入为表压，应用层不提供 PressureMode 字段，所有 P 值原样透传。
func TestToSevenHoleCoreInputDirectMapping(t *testing.T) {
	input := SevenHoleInterpolationInput{
		P1: 100, P2: 200, P3: 300, P4: 400, P5: 500, P6: 600, P7: 700,
		Patm: 101325, Tatm: 20,
	}

	core := toSevenHoleCoreInput(input)
	if core.P1 != 100 || core.P2 != 200 || core.P3 != 300 ||
		core.P4 != 400 || core.P5 != 500 || core.P6 != 600 || core.P7 != 700 {
		t.Errorf("7 孔表压应原样传递: got P1=%f P2=%f P3=%f P4=%f P5=%f P6=%f P7=%f",
			core.P1, core.P2, core.P3, core.P4, core.P5, core.P6, core.P7)
	}
	if core.PAtm != 101325 {
		t.Errorf("PAtm 应原样传递: got %f", core.PAtm)
	}
	if core.TAtm != 20 {
		t.Errorf("TAtm 应原样传递: got %f", core.TAtm)
	}
}

// TestToSevenHoleAppResult 验证算法包结果到应用层结果的字段映射。
// 重点：TotalPressure → P0、StaticPressure → Ps（JSON tag 一致性）。
func TestToSevenHoleAppResult(t *testing.T) {
	core := seven_interp.InterpolationResult{
		Alpha:           -15.5,
		Beta:            12.3,
		MachNumber:      0.45,
		Velocity:        150.0,
		DynamicPressure: 5000.0,
		TotalPressure:   12000.0,
		StaticPressure:  7000.0,
		IsValid:         true,
		Warning:         "",
	}

	r := toSevenHoleAppResult(core)
	if r.Alpha != -15.5 {
		t.Errorf("Alpha = %f, want -15.5", r.Alpha)
	}
	if r.Beta != 12.3 {
		t.Errorf("Beta = %f, want 12.3", r.Beta)
	}
	if r.MachNumber != 0.45 {
		t.Errorf("MachNumber = %f, want 0.45", r.MachNumber)
	}
	if r.Velocity != 150.0 {
		t.Errorf("Velocity = %f, want 150.0", r.Velocity)
	}
	if r.DynamicPressure != 5000.0 {
		t.Errorf("DynamicPressure = %f, want 5000.0", r.DynamicPressure)
	}
	if r.P0 != 12000.0 {
		t.Errorf("P0 (TotalPressure) = %f, want 12000.0", r.P0)
	}
	if r.Ps != 7000.0 {
		t.Errorf("Ps (StaticPressure) = %f, want 7000.0", r.Ps)
	}
	if !r.IsValid {
		t.Error("IsValid 应为 true")
	}
}

// ==================== 单点计算测试（真实校准工况） ====================

// TestCalculateSevenHoleInnerZone 验证内区插值（小角度模式）。
// 数据来源：golden.json idx=87，pure sideslip α=15°, β≈0°。
// 此工况 P7 最大，触发内区插值路径。
func TestCalculateSevenHoleInnerZone(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)

	input := SevenHoleInterpolationInput{
		P1: 213.633, P2: -1551.4, P3: -1269.183, P4: -9.0,
		P5: 2093.683, P6: 2049.133, P7: 3699.367,
		Patm: 98876.0, Tatm: 28.0,
	}

	resp := app.CalculateSevenHole(input)
	if !resp.Success {
		t.Fatalf("内区插值应成功, got error: %s", resp.Error)
	}
	if !resp.Data.IsValid {
		t.Fatalf("内区插值应返回有效结果, got warning: %q", resp.Data.Warning)
	}

	// 期望值来自 golden.json idx=87（新全精度 PRB；模式 little，网格节点附近）
	const angleTol = 1e-4
	const pressTol = 1e-3
	if math.Abs(resp.Data.Alpha-14.999999998880819) > angleTol {
		t.Errorf("Alpha = %.8f, want 15.00000000 (内区 pure sideslip)", resp.Data.Alpha)
	}
	if math.Abs(resp.Data.Beta-(-0.000000001216380)) > angleTol {
		t.Errorf("Beta = %.8f, want 0.00000000 (内区 pure sideslip)", resp.Data.Beta)
	}
	if math.Abs(resp.Data.P0-4060.382999954085790) > pressTol {
		t.Errorf("P0 = %.6f, want 4060.38300000", resp.Data.P0)
	}
	if math.Abs(resp.Data.Ps-(-29.233000022617585)) > pressTol {
		t.Errorf("Ps = %.6f, want -29.23300002", resp.Data.Ps)
	}
	if math.Abs(resp.Data.MachNumber-0.241353359691929) > 1e-6 {
		t.Errorf("MachNumber = %.8f, want 0.24135336", resp.Data.MachNumber)
	}
}

// TestCalculateSevenHoleOuterZone 验证外区插值（大角度模式）。
// 数据来源：golden.json idx=290，sector=3 一般扇区内点工况。
// 此工况 P3 最大，触发外区扇区 3 插值路径。
func TestCalculateSevenHoleOuterZone(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)

	input := SevenHoleInterpolationInput{
		P1: -2587.667, P2: 1488.2, P3: 3963.717, P4: 835.75,
		P5: -2521.75, P6: -2647.617, P7: 1338.967,
		Patm: 98876.0, Tatm: 28.0,
	}

	resp := app.CalculateSevenHole(input)
	if !resp.Success {
		t.Fatalf("外区插值应成功, got error: %s", resp.Error)
	}
	if !resp.Data.IsValid {
		t.Fatalf("外区插值应返回有效结果, got warning: %q", resp.Data.Warning)
	}

	// 期望值来自 golden.json idx=290（新全精度 PRB；mode big sector=3）
	const angleTol = 1e-4
	const pressTol = 1e-3
	if math.Abs(resp.Data.Alpha-(-33.344110981854953)) > angleTol {
		t.Errorf("Alpha = %.8f, want -33.34411098 (外区 sector=3)", resp.Data.Alpha)
	}
	if math.Abs(resp.Data.Beta-(-13.467834225808051)) > angleTol {
		t.Errorf("Beta = %.8f, want -13.46783423 (外区 sector=3)", resp.Data.Beta)
	}
	if math.Abs(resp.Data.P0-4069.750000029121566) > pressTol {
		t.Errorf("P0 = %.6f, want 4069.75000003", resp.Data.P0)
	}
	if math.Abs(resp.Data.Ps-(-36.500000036759950)) > pressTol {
		t.Errorf("Ps = %.6f, want -36.50000004", resp.Data.Ps)
	}
}

// TestCalculateSevenHoleNotLoaded 验证未加载 PRB 时返回失败响应。
func TestCalculateSevenHoleNotLoaded(t *testing.T) {
	app := &App{}
	resp := app.CalculateSevenHole(SevenHoleInterpolationInput{
		P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 100,
		Patm: 101325, Tatm: 20,
	})
	if resp.Success {
		t.Error("未加载 PRB 时 Success 应为 false")
	}
	if resp.Error == "" {
		t.Error("未加载 PRB 时 Error 应有提示")
	}
}

// ==================== 批量计算测试 ====================

// TestBatchCalculateSevenHoleReturnsDataPayload 验证批量计算返回 Data 数组，
// 长度与输入一致（即便部分失败也应占位）。
func TestBatchCalculateSevenHoleReturnsDataPayload(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)

	// 第 1 行：内区 pure sideslip 工况（boundary.json idx=87）
	// 第 2 行：内区 pure attack 工况（boundary.json idx=123）
	inputs := []SevenHoleInterpolationInput{
		{
			P1: 213.633, P2: -1551.4, P3: -1269.183, P4: -9.0,
			P5: 2093.683, P6: 2049.133, P7: 3699.367,
			Patm: 98876.0, Tatm: 28.0,
		},
		{
			P1: 2425.283, P2: 1094.2, P3: -868.45, P4: -1403.633,
			P5: -804.133, P6: 960.55, P7: 3699.767,
			Patm: 98879.0, Tatm: 28.0,
		},
	}

	resp := app.BatchCalculateSevenHole(inputs)
	if resp.Data == nil {
		t.Fatalf("Data 不应为 nil")
	}
	if len(resp.Data) != len(inputs) {
		t.Errorf("Data 长度 = %d, want %d（每行都应有结果占位）", len(resp.Data), len(inputs))
	}
	// 两行都应成功
	for i, r := range resp.Data {
		if !r.IsValid {
			t.Errorf("第 %d 行应成功, got warning: %q", i+1, r.Warning)
		}
	}
}

// TestBatchCalculateSevenHolePartialFailure 验证批量计算遇到坏行时不中断，标记失败继续。
// 第 1 行用真实工况（成功），第 2 行 P1..P7 全相等触发 1e-12 分母保护（返回错误）。
func TestBatchCalculateSevenHolePartialFailure(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)

	inputs := []SevenHoleInterpolationInput{
		{ // 内区 pure sideslip，正常
			P1: 213.633, P2: -1551.4, P3: -1269.183, P4: -9.0,
			P5: 2093.683, P6: 2049.133, P7: 3699.367,
			Patm: 98876.0, Tatm: 28.0,
		},
		{ // P1..P7 全相等，p7 == pAvg 触发内区分母保护
			P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 100,
			Patm: 101325, Tatm: 20,
		},
	}

	resp := app.BatchCalculateSevenHole(inputs)
	if resp.Data == nil {
		t.Fatalf("Data 不应为 nil")
	}
	if len(resp.Data) != 2 {
		t.Fatalf("Data 长度 = %d, want 2", len(resp.Data))
	}
	// 第 1 行应成功
	if !resp.Data[0].IsValid {
		t.Errorf("第 1 行应成功, got warning: %q", resp.Data[0].Warning)
	}
	// 第 2 行应失败（IsValid=false 且有 Warning）
	if resp.Data[1].IsValid {
		t.Errorf("第 2 行应失败（p7==pAvg 触发分母保护），got IsValid=true")
	}
	if resp.Data[1].Warning == "" {
		t.Error("第 2 行失败时 Warning 应有提示")
	}
}

// ==================== 并发安全测试 ====================

// TestSevenHoleConcurrentAccess 验证多 goroutine 并发调用 CalculateSevenHole 不会触发 race。
// 配合 `go test -race` 检测，确保 sevenHoleState 的 RWMutex 保护正确。
func TestSevenHoleConcurrentAccess(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			// 围绕真实工况做小扰动，避免完全相同的输入
			input := SevenHoleInterpolationInput{
				P1:   213.633 + float64(idx),
				P2:   -1551.4,
				P3:   -1269.183,
				P4:   -9.0,
				P5:   2093.683,
				P6:   2049.133,
				P7:   3699.367,
				Patm: 98876.0,
				Tatm: 28.0,
			}
			_ = app.CalculateSevenHole(input)
			_ = app.IsSevenHolePrbLoaded()
			_ = app.GetSevenHoleValidRange()
		}(i)
	}
	wg.Wait()
}

// ==================== 加载状态查询测试 ====================

// TestIsSevenHolePrbLoaded_NoData 验证未加载时返回 false。
func TestIsSevenHolePrbLoaded_NoData(t *testing.T) {
	app := &App{}
	if app.IsSevenHolePrbLoaded() {
		t.Error("未加载 PRB 时应返回 false")
	}
}

// TestIsSevenHolePrbLoaded_WithData 验证加载后返回 true。
func TestIsSevenHolePrbLoaded_WithData(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)
	if !app.IsSevenHolePrbLoaded() {
		t.Error("加载 PRB 后应返回 true")
	}
}

// TestGetSevenHoleValidRange_NotLoaded 验证未加载时返回失败响应。
func TestGetSevenHoleValidRange_NotLoaded(t *testing.T) {
	app := &App{}
	resp := app.GetSevenHoleValidRange()
	if resp.Success {
		t.Error("未加载时 Success 应为 false")
	}
	if resp.Error == "" {
		t.Error("未加载时 Error 应有提示")
	}
}

// TestGetSevenHoleValidRange_Loaded 验证加载后返回内区网格角度范围 ±30°。
// 注意：MachMin/MachMax 恒为 0（7 孔无马赫数范围概念，spec §2.2）。
func TestGetSevenHoleValidRange_Loaded(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)
	resp := app.GetSevenHoleValidRange()
	if !resp.Success {
		t.Fatalf("加载后 Success 应为 true, got error: %s", resp.Error)
	}
	// 内区网格 a/b ∈ [-30, +30] 步长 5（13×13=169 点）
	if resp.Data.AlphaMin != -30 || resp.Data.AlphaMax != 30 {
		t.Errorf("AlphaMin/Max = [%f, %f], want [-30, 30]", resp.Data.AlphaMin, resp.Data.AlphaMax)
	}
	if resp.Data.BetaMin != -30 || resp.Data.BetaMax != 30 {
		t.Errorf("BetaMin/Max = [%f, %f], want [-30, 30]", resp.Data.BetaMin, resp.Data.BetaMax)
	}
	// 7 孔 MachMin/MachMax 恒为 0（无马赫数范围）
	if resp.Data.MachMin != 0 || resp.Data.MachMax != 0 {
		t.Errorf("MachMin/Max = [%f, %f], want [0, 0]", resp.Data.MachMin, resp.Data.MachMax)
	}
}

// TestGetSevenHolePrbFiles_Loaded 验证加载后返回 7 个文件信息，顺序为内区 + 外区 1..6。
func TestGetSevenHolePrbFiles_Loaded(t *testing.T) {
	app := setupSevenHoleLoadedApp(t)
	files := app.GetSevenHolePrbFiles()
	if len(files) != 7 {
		t.Fatalf("应返回 7 个文件, got %d", len(files))
	}
	// 第 1 个是内区（Sector=0）
	if files[0].Sector != 0 {
		t.Errorf("第 1 个文件应为内区 Sector=0, got %d", files[0].Sector)
	}
	if files[0].FileName != "7.prb" {
		t.Errorf("第 1 个文件名应为 7.prb, got %q", files[0].FileName)
	}
	// 第 2..7 个是外区 1..6
	for i := 1; i < 7; i++ {
		if files[i].Sector != i {
			t.Errorf("第 %d 个文件应为外区 Sector=%d, got %d", i+1, i, files[i].Sector)
		}
		expectedName := string(rune('0'+i)) + ".prb"
		if files[i].FileName != expectedName {
			t.Errorf("第 %d 个文件名应为 %s, got %q", i+1, expectedName, files[i].FileName)
		}
	}
}
