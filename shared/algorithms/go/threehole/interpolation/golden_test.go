package interpolation

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =====================================================================
// 三孔探针插值算法黄金基准测试
//
// 本文件为 ThreeHoleInterpolator 建立黄金基准，防止精度漂移。包含三类用例：
//   - TC-ALGO-04 黄金基准: 已知输入→期望输出对，JSON 存于 testdata/golden/threehole/
//   - TC-ALGO-05 边界用例: 覆盖除零/NaN/Inf/超范围等风险点
//   - TC-ALGO-06 数值稳定性: 微小扰动下输出连续性验证
//
// 设计策略: 三孔算法有真实校准数据 0.8.prb(Ma=0.801, 13 个角度点 -30~+30)。
// 利用校准数据的可逆性——校准压力点(Kb, Alpha)一一对应，用真实校准压力(表压)
// 反算出已知角度，断言 Calculate 返回的 Alpha 误差 < 0.5°(插值允许误差)。
// "黄金"指当前实现的已知行为快照，后续若动插值逻辑会被捕获。
// =====================================================================

// ==================== 黄金基准数据结构 ====================

// goldenCase 三孔黄金基准用例，序列化为 JSON 存于 testdata/golden/threehole/
type goldenCase struct {
	Name       string             `json:"name"`                 // 用例名(唯一标识)
	Region     string             `json:"region"`               // 区域: center/corner/edge
	Desc       string             `json:"desc"`                 // 用例描述(覆盖区域与构造逻辑)
	Input      InterpolationInput `json:"input"`                // 插值输入
	Expected   goldenExpected     `json:"expected"`             // 期望输出
	Tolerance  goldenTolerance    `json:"tolerance"`            // 断言容差
	SkipFields []string           `json:"skipFields,omitempty"` // 跳过断言的字段名(对应 expected 的 JSON key)
}

// goldenExpected 三孔期望输出
// Alpha = 已知输入角度(校准数据可逆性)
// MachNumber/IsValid = 当前实现输出快照(回归基线)
// 注: 三孔无 Beta 字段(只有 Alpha), 无 V 字段(只有 MachNumber/Pt/Ps)
type goldenExpected struct {
	Alpha      float64 `json:"alpha"`
	MachNumber float64 `json:"machNumber"`
	IsValid    bool    `json:"isValid"`
}

// goldenTolerance 三孔断言容差
type goldenTolerance struct {
	Alpha      float64 `json:"alpha"`
	MachNumber float64 `json:"machNumber"`
}

// ==================== 真实校准数据辅助 ====================

// calibPressureRow 真实校准压力数据(表压，单位 Pa)
// 来源: three_hole_customer_test.go 中 getCalibData() 的绝对压减去 PAtm=101425
// 使用表压使 calcMach 中 absPs=ps+pa 的物理意义正确(MachNumber≈0.80 校准值)
type calibPressureRow struct {
	alpha float64 // 校准角度(度)
	p1    float64 // 孔1表压(Pa)
	p2    float64 // 孔2表压(总压方向, Pa)
	p3    float64 // 孔3表压(Pa)
}

// realCalibPressures 真实校准压力数据(Ma=0.801, 13 个角度点 -30~+30)
// 每行对应 0.8.prb 中一个角度点的物理校准压力(表压)
// 验证: deltaP=2*P2-P1-P3, Kb=(P3-P1)/deltaP 与 0.8.prb 的 Kb 列精确匹配
var realCalibPressures = []calibPressureRow{
	{-30, 49544.2, 34010.8, 7804.1},
	{-25, 49883.0, 37295.7, 9399.0},
	{-20, 49075.7, 41869.9, 16029.7},
	{-15, 47193.0, 45376.7, 22801.7},
	{-10, 44785.0, 47882.3, 27803.8},
	{-5, 41441.8, 49350.9, 32263.6},
	{0, 37227.7, 49908.4, 36503.7},
	{5, 32789.0, 49680.6, 40447.8},
	{10, 27987.7, 48849.9, 43880.2},
	{15, 23187.0, 47048.2, 46417.7},
	{20, 18133.8, 44248.6, 48234.2},
	{25, 12213.1, 40701.6, 49472.4},
	{30, 3900.7, 35640.6, 49907.8},
}

// calibAtmPressure 校准数据对应的大气压(Pa)
// 与 three_hole_customer_test.go 中 getCustomerData() 的 pa 字段一致
const calibAtmPressure = 101425.0

// calibInputForAlpha 构造能反算出指定角度的真实校准压力输入
// 查找 realCalibPressures 中对应角度的表压, 补充标准大气参数
// 物理意义: 该输入的 Kb=(P3-P1)/deltaP 精确等于 0.8.prb 中该角度的 Kb 值
func calibInputForAlpha(alpha float64) InterpolationInput {
	for _, row := range realCalibPressures {
		if row.alpha == alpha {
			return InterpolationInput{
				P1:   row.p1,
				P2:   row.p2,
				P3:   row.p3,
				PAtm: calibAtmPressure,
				TAtm: 20, // 摄氏度, 使 calcGamma 返回标准值 1.4
			}
		}
	}
	panic(fmt.Sprintf("校准数据中未找到角度 %v", alpha))
}

// loadRealPrbFile 加载真实 0.8.prb 校准文件构建插值器
// 从 testdata/0.8.prb 读取, 按行分割后传入 LoadPrbData
func loadRealPrbFile(t *testing.T) *ThreeHoleInterpolator {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "0.8.prb"))
	if err != nil {
		t.Fatalf("读取 testdata/0.8.prb 失败: %v", err)
	}
	// 按行分割, 保留 LoadPrbData 内部的空白行过滤逻辑
	lines := strings.Split(string(data), "\n")
	interp := NewThreeHoleInterpolator()
	if _, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "0.8.prb", Lines: lines},
	}); err != nil {
		t.Fatalf("LoadPrbData 失败: %v", err)
	}
	return interp
}

// isFinite 辅助函数: 判断浮点数是否为有限值(非 NaN, 非 Inf)
// 三孔包未提供此函数, 在此定义供黄金测试使用
func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// ==================== 黄金用例定义 ====================

// goldenThreeHoleCases 三孔黄金用例(8 个)
// 覆盖三区域策略:
//   - center(中心区): |α| <= 10, Kb 变化平缓接近线性
//   - edge(边缘区): 15 <= |α| <= 25, Kb 变化较大非线性
//   - corner(角区): |α| = 30, Kb 边界点
//
// Expected.Alpha = 已知输入角度(校准数据可逆性)
// Expected.MachNumber/IsValid 由生成器从当前实现输出填充(回归快照)
func goldenThreeHoleCases() []goldenCase {
	defaultTol := goldenTolerance{Alpha: 0.5, MachNumber: 0.05}
	return []goldenCase{
		{
			Name:      "center_alpha0",
			Region:    "center",
			Desc:      "中心区原点: α=0, Kb=-0.027755, 零偏角对称工况",
			Input:     calibInputForAlpha(0),
			Expected:  goldenExpected{Alpha: 0},
			Tolerance: defaultTol,
		},
		{
			Name:      "center_alpha5",
			Region:    "center",
			Desc:      "中心区: α=5, Kb=0.293167, 小角度正向偏角",
			Input:     calibInputForAlpha(5),
			Expected:  goldenExpected{Alpha: 5},
			Tolerance: defaultTol,
		},
		{
			Name:      "center_alphaneg5",
			Region:    "center",
			Desc:      "中心区: α=-5, Kb=-0.367181, 小角度反向偏角",
			Input:     calibInputForAlpha(-5),
			Expected:  goldenExpected{Alpha: -5},
			Tolerance: defaultTol,
		},
		{
			Name:      "edge_alpha15",
			Region:    "edge",
			Desc:      "边缘区: α=15, Kb=0.948513, 中等角度正向偏角",
			Input:     calibInputForAlpha(15),
			Expected:  goldenExpected{Alpha: 15},
			Tolerance: defaultTol,
		},
		{
			Name:      "edge_alphaneg15",
			Region:    "edge",
			Desc:      "边缘区: α=-15, Kb=-1.174992, 中等角度反向偏角",
			Input:     calibInputForAlpha(-15),
			Expected:  goldenExpected{Alpha: -15},
			Tolerance: defaultTol,
		},
		{
			Name:      "edge_alphaneg25",
			Region:    "edge",
			Desc:      "边缘区: α=-25, Kb=-2.644388, 接近角区边界",
			Input:     calibInputForAlpha(-25),
			Expected:  goldenExpected{Alpha: -25},
			Tolerance: defaultTol,
		},
		{
			Name:      "corner_alpha30",
			Region:    "corner",
			Desc:      "角区边界: α=30, Kb=2.633085, 校准数据上边界点",
			Input:     calibInputForAlpha(30),
			Expected:  goldenExpected{Alpha: 30},
			Tolerance: defaultTol,
		},
		{
			Name:      "corner_alphaneg30",
			Region:    "corner",
			Desc:      "角区边界: α=-30, Kb=-3.910702, 校准数据下边界点",
			Input:     calibInputForAlpha(-30),
			Expected:  goldenExpected{Alpha: -30},
			Tolerance: defaultTol,
		},
	}
}

// ==================== JSON 加载与断言辅助 ====================

// loadGoldenCases 从指定目录加载所有 .json 黄金用例
func loadGoldenCases(t *testing.T, dir string) []goldenCase {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取黄金用例目录 %s 失败: %v", dir, err)
	}
	var cases []goldenCase
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取黄金用例 %s 失败: %v", path, err)
		}
		var gc goldenCase
		if err := json.Unmarshal(data, &gc); err != nil {
			t.Fatalf("解析黄金用例 %s 失败: %v", path, err)
		}
		cases = append(cases, gc)
	}
	if len(cases) == 0 {
		t.Fatalf("目录 %s 未包含任何黄金用例 JSON", dir)
	}
	return cases
}

// assertGoldenResult 断言三孔插值结果符合黄金用例期望
// 支持 gc.SkipFields 跳过指定字段(对应 expected 的 JSON key: alpha/machNumber/isValid)
func assertGoldenResult(t *testing.T, gc goldenCase, result InterpolationResult) {
	t.Helper()
	skipped := buildSkipSet(gc.SkipFields)

	// Alpha 断言(可跳过)
	if !skipped["alpha"] {
		if math.Abs(result.Alpha-gc.Expected.Alpha) > gc.Tolerance.Alpha {
			t.Errorf("Alpha = %.6f, 期望 %.6f ± %.3f (用例 %s, 区域 %s)",
				result.Alpha, gc.Expected.Alpha, gc.Tolerance.Alpha, gc.Name, gc.Region)
		}
	} else {
		t.Logf("用例 %s: 跳过 Alpha 断言 (skipFields 标记)", gc.Name)
	}

	// MachNumber 断言(可跳过)
	if !skipped["machNumber"] {
		if math.Abs(result.MachNumber-gc.Expected.MachNumber) > gc.Tolerance.MachNumber {
			t.Errorf("MachNumber = %.6f, 期望 %.6f ± %.3f (用例 %s)",
				result.MachNumber, gc.Expected.MachNumber, gc.Tolerance.MachNumber, gc.Name)
		}
	} else {
		t.Logf("用例 %s: 跳过 MachNumber 断言 (skipFields 标记)", gc.Name)
	}

	// IsValid 断言(可跳过)
	if !skipped["isValid"] {
		if result.IsValid != gc.Expected.IsValid {
			t.Errorf("IsValid = %v, 期望 %v (用例 %s, Warning=%q)",
				result.IsValid, gc.Expected.IsValid, gc.Name, result.Warning)
		}
	} else {
		t.Logf("用例 %s: 跳过 IsValid 断言 (skipFields 标记, 当前 IsValid=%v, Warning=%q)",
			gc.Name, result.IsValid, result.Warning)
	}
}

// buildSkipSet 将 skipFields 切片转为集合便于 O(1) 查询
// 支持的字段名与 goldenExpected 的 JSON key 对应: alpha/machNumber/isValid
func buildSkipSet(skipFields []string) map[string]bool {
	set := make(map[string]bool, len(skipFields))
	for _, f := range skipFields {
		set[f] = true
	}
	return set
}

// requireNotPanic 包装函数, 确保被测代码不 panic
func requireNotPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("不应 panic, 实际 panic: %v", r)
		}
	}()
	fn()
}

// ==================== TC-ALGO-04: 黄金基准测试 ====================

// TestGoldenGenerate 生成黄金基准 JSON 文件(手动运行)
// 运行: $env:GOLDEN_REGEN=1; go test -run TestGoldenGenerate -v
// 日常测试跳过，避免覆盖已检入的基准
func TestGoldenGenerate(t *testing.T) {
	if os.Getenv("GOLDEN_REGEN") != "1" {
		t.Skip("设置 GOLDEN_REGEN=1 以重新生成黄金基准 JSON")
	}

	// 三孔插值器(真实 0.8.prb 校准数据)
	interp := loadRealPrbFile(t)
	dir := filepath.Join("testdata", "golden", "threehole")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建目录 %s: %v", dir, err)
	}
	for i, c := range goldenThreeHoleCases() {
		result, err := interp.Calculate(c.Input)
		if err != nil {
			t.Fatalf("用例 %s Calculate: %v", c.Name, err)
		}
		// MachNumber/IsValid 取当前实现输出作为回归快照
		c.Expected.MachNumber = result.MachNumber
		c.Expected.IsValid = result.IsValid
		writeGoldenJSON(t, dir, fmt.Sprintf("threehole_case_%02d.json", i+1), c)
	}
}

// writeGoldenJSON 将黄金用例序列化写入 JSON 文件
func writeGoldenJSON(t *testing.T, dir, name string, c goldenCase) {
	t.Helper()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatalf("序列化 %s 失败: %v", name, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", path, err)
	}
	t.Logf("已生成 %s", path)
}

// TestThreeHole_Golden 三孔插值器黄金基准
// 加载 testdata/golden/threehole/*.json, 遍历断言 Alpha/MachNumber/IsValid
func TestThreeHole_Golden(t *testing.T) {
	interp := loadRealPrbFile(t)
	cases := loadGoldenCases(t, filepath.Join("testdata", "golden", "threehole"))
	for _, gc := range cases {
		gc := gc
		t.Run(gc.Name, func(t *testing.T) {
			result, err := interp.Calculate(gc.Input)
			if err != nil {
				t.Fatalf("Calculate 返回错误: %v", err)
			}
			assertGoldenResult(t, gc, result)
		})
	}
}

// =====================================================================
// TC-ALGO-05: 边界用例
//
// 覆盖除零/NaN/Inf/超范围/反物理输入等风险点，确保算法在极端输入下不 panic，
// 并返回合理的 Invalid 状态或回退值。每个边界用例用 requireNotPanic 包裹。
// =====================================================================

// TestThreeHole_Boundary_AllZeroPressures 全零压力输入
// 期望: deltaP=2*0-0-0=0 < 1e-6 触发防护, 返回 IsValid=false, 不 panic
func TestThreeHole_Boundary_AllZeroPressures(t *testing.T) {
	interp := loadRealPrbFile(t)
	input := InterpolationInput{}
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回错误: %v", err)
		}
		if result.IsValid {
			t.Errorf("全零压力应返回 IsValid=false, 实际=true, Warning=%q", result.Warning)
		}
	})
}

// TestThreeHole_Boundary_SymmetricInput 对称输入
// P1=P3 → Kb=0, 校准数据中 α=0 对应 Kb=-0.027755
// Kb=0 介于 α=0(Kb=-0.027755) 和 α=5(Kb=0.293167) 之间, 插值得到 Alpha≈0.43°
// 期望 |Alpha| < 1°(对称输入应接近零偏角)
func TestThreeHole_Boundary_SymmetricInput(t *testing.T) {
	interp := loadRealPrbFile(t)
	input := InterpolationInput{P1: 40000, P2: 50000, P3: 40000, PAtm: calibAtmPressure, TAtm: 20}
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 错误: %v", err)
		}
		if math.Abs(result.Alpha) > 1.0 {
			t.Errorf("对称输入 Alpha 应≈0, 实际=%.6f", result.Alpha)
		}
		t.Logf("对称输入: Alpha=%.6f Kb=0 介于 α=0 和 α=5 之间", result.Alpha)
	})
}

// TestThreeHole_Boundary_TinyDelta 微小压力差
// deltaP = 2*P2-P1-P3 < 1e-6 触发防护, 不 panic
// 期望: 返回 IsValid=false, Warning 提示压力差分接近零
func TestThreeHole_Boundary_TinyDelta(t *testing.T) {
	interp := loadRealPrbFile(t)
	// P1=100, P2=100, P3=100 → deltaP=0 < 1e-6
	input := InterpolationInput{P1: 100, P2: 100, P3: 100, PAtm: calibAtmPressure, TAtm: 20}
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 错误: %v", err)
		}
		if result.IsValid {
			t.Errorf("微小 deltaP 应返回 IsValid=false, 实际=true, Warning=%q", result.Warning)
		}
		if result.Warning == "" {
			t.Errorf("微小 deltaP 应产生非空 Warning 提示")
		}
		t.Logf("微小 deltaP: IsValid=%v Warning=%q", result.IsValid, result.Warning)
	})
}

// TestThreeHole_Boundary_NaNInput NaN 压力输入
// deltaP 或 kbTemp 计算产生 NaN, 防护层拦截, 返回 IsValid=false
func TestThreeHole_Boundary_NaNInput(t *testing.T) {
	interp := loadRealPrbFile(t)
	input := InterpolationInput{P1: math.NaN(), P2: 50000, P3: 40000, PAtm: calibAtmPressure, TAtm: 20}
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回错误: %v", err)
		}
		if result.IsValid {
			t.Errorf("NaN 输入应返回 IsValid=false, 实际=true")
		}
	})
}

// TestThreeHole_Boundary_InfInput Inf 压力输入
// Inf 传播使 kbTemp=Inf, 防护层(math.IsInf)拦截, 返回 IsValid=false
func TestThreeHole_Boundary_InfInput(t *testing.T) {
	interp := loadRealPrbFile(t)
	input := InterpolationInput{P1: math.Inf(1), P2: 50000, P3: 40000, PAtm: calibAtmPressure, TAtm: 20}
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回错误: %v", err)
		}
		if result.IsValid {
			t.Errorf("Inf 输入应返回 IsValid=false, 实际=true, Warning=%q", result.Warning)
		}
		t.Logf("Inf 输入: Alpha=%.3f IsValid=%v MachNumber=%.4f Warning=%q",
			result.Alpha, result.IsValid, result.MachNumber, result.Warning)
	})
}

// TestThreeHole_Boundary_OutOfRange 超出校准范围
// 构造 Kb > 2.633085(校准上界) 的输入, 触发外推 Warning
// P2=60000, P1=10000, P3=90000 → deltaP=20000, Kb=(90000-10000)/20000=4.0 > 2.633085
func TestThreeHole_Boundary_OutOfRange(t *testing.T) {
	interp := loadRealPrbFile(t)
	input := InterpolationInput{P1: 10000, P2: 60000, P3: 90000, PAtm: calibAtmPressure, TAtm: 20}
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 错误: %v", err)
		}
		// Kb=4.0 超出校准上界 2.633085, 应触发外推 Warning 或钳位到 α=30
		if !isFinite(result.Alpha) {
			t.Errorf("Alpha 应为有限值: Alpha=%v", result.Alpha)
		}
		t.Logf("超出范围(Kb=4.0): Alpha=%.3f IsValid=%v MachNumber=%.4f Warning=%q",
			result.Alpha, result.IsValid, result.MachNumber, result.Warning)
	})
}

// TestThreeHole_Boundary_ZeroAtm 零大气参数
// A2 后 PAtm<=0 属非法大气压，calcMach 校验前置条件（patm>0）应判无效。
func TestThreeHole_Boundary_ZeroAtm(t *testing.T) {
	interp := loadRealPrbFile(t)
	// 使用 α=5 的校准压力, 但 PAtm=0
	input := calibInputForAlpha(5)
	input.PAtm = 0
	input.TAtm = 0
	requireNotPanic(t, func() {
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 错误: %v", err)
		}
		if result.IsValid {
			t.Errorf("PAtm=0 应返回 IsValid=false（大气压非法），实际=true, Warning=%q", result.Warning)
		}
		if result.Warning == "" {
			t.Errorf("PAtm=0 应产生非空 Warning")
		}
		if !isFinite(result.MachNumber) {
			t.Errorf("MachNumber 应为有限值: Mach=%v", result.MachNumber)
		}
		t.Logf("零大气参数: Alpha=%.3f MachNumber=%.4f IsValid=%v Warning=%q",
			result.Alpha, result.MachNumber, result.IsValid, result.Warning)
	})
}

// =====================================================================
// TC-ALGO-06: 数值稳定性
//
// 对中心区工况(α=5)施加 ±0.1Pa 随机扰动 100 次, 验证:
//  1. 输出 Alpha 无 NaN/Inf
//  2. 输出连续(相邻扰动输出差 < 0.5°, 全体 max-min < 1.0°)
//  3. IsValid 稳定(不因微小扰动翻转)
//  4. 物理参数(MachNumber)无 NaN/Inf
//
// 使用固定随机种子保证测试可复现。选用中心区是因为其插值平滑、不应有跳变。
// 三孔只有 3 个孔压(P1/P2/P3), 扰动函数只扰动这 3 个。
// =====================================================================

// perturbThreeHole 对 3 个孔压施加 ±amplitude Pa 的随机扰动, 返回扰动后输入
// 使用 *rand.Rand 保证可复现, 不污染全局 rand 源
func perturbThreeHole(base InterpolationInput, rng *rand.Rand, amplitude float64) InterpolationInput {
	out := base
	// 在 [-amplitude, +amplitude] 区间均匀扰动
	out.P1 += (rng.Float64()*2 - 1) * amplitude
	out.P2 += (rng.Float64()*2 - 1) * amplitude
	out.P3 += (rng.Float64()*2 - 1) * amplitude
	return out
}

// threeHoleStabilityMetrics 三孔稳定性统计
type threeHoleStabilityMetrics struct {
	AlphaMin, AlphaMax float64 // Alpha 极值范围
	AlphaMaxJump       float64 // 相邻扰动 Alpha 最大跳变
	IsValidFlips       int     // IsValid 翻转次数
	HasNaNOrInf        bool    // 是否出现 NaN/Inf
}

// runThreeHoleStabilityProbe 执行 N 次扰动, 收集稳定性统计
// interpolator 必须已加载校准数据; base 为基准输入(中心区工况)
func runThreeHoleStabilityProbe(t *testing.T, interp *ThreeHoleInterpolator, base InterpolationInput, iterations int) (threeHoleStabilityMetrics, []bool) {
	t.Helper()
	rng := rand.New(rand.NewSource(42)) // 固定种子, 保证可复现

	m := threeHoleStabilityMetrics{
		AlphaMin: math.Inf(1),
		AlphaMax: math.Inf(-1),
	}
	var prevAlpha float64
	first := true
	validity := make([]bool, 0, iterations)

	for i := 0; i < iterations; i++ {
		input := perturbThreeHole(base, rng, 0.1)
		result, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("第 %d 次扰动 Calculate 错误: %v", i, err)
		}
		validity = append(validity, result.IsValid)

		// NaN/Inf 检测
		if !isFinite(result.Alpha) || !isFinite(result.MachNumber) {
			m.HasNaNOrInf = true
			t.Errorf("第 %d 次扰动出现非有限值: Alpha=%v MachNumber=%v",
				i, result.Alpha, result.MachNumber)
			continue
		}
		// 极值范围
		if result.Alpha < m.AlphaMin {
			m.AlphaMin = result.Alpha
		}
		if result.Alpha > m.AlphaMax {
			m.AlphaMax = result.Alpha
		}
		// 相邻跳变
		if !first {
			if d := math.Abs(result.Alpha - prevAlpha); d > m.AlphaMaxJump {
				m.AlphaMaxJump = d
			}
		}
		first = false
		prevAlpha = result.Alpha
	}

	// 统计 IsValid 翻转
	for i := 1; i < len(validity); i++ {
		if validity[i] != validity[i-1] {
			m.IsValidFlips++
		}
	}
	return m, validity
}

// TestThreeHole_Stability_Perturbation 三孔插值器扰动稳定性
// 中心区工况(α=5) ±0.1Pa 扰动 100 次:
//   - 输出无 NaN/Inf
//   - Alpha 连续(相邻跳变 < 0.5°, 全体极差 < 1.0°)
//   - IsValid 稳定(不翻转)
//
// 阈值说明: ±0.1Pa 扰动在 3 个孔压上, 三孔 Alpha 灵敏度约 1°/Pa(中心区),
// 故相邻跳变可达 ±0.3°。设 0.5° 阈值既允许正常扰动噪声, 又能捕获区域切换跳变。
func TestThreeHole_Stability_Perturbation(t *testing.T) {
	interp := loadRealPrbFile(t)
	base := calibInputForAlpha(5) // 中心区基准工况

	m, _ := runThreeHoleStabilityProbe(t, interp, base, 100)

	// 1. 无 NaN/Inf
	if m.HasNaNOrInf {
		t.Errorf("扰动过程出现 NaN/Inf, 算法数值不稳定")
	}
	// 2. 相邻扰动跳变 < 0.5°(平滑性)
	if m.AlphaMaxJump > 0.5 {
		t.Errorf("Alpha 相邻扰动跳变过大: %.6f° (阈值 0.5°)", m.AlphaMaxJump)
	}
	// 3. 全体极差 < 1.0°(±0.1Pa 扰动不应造成 >1° 漂移)
	if alphaRange := m.AlphaMax - m.AlphaMin; alphaRange > 1.0 {
		t.Errorf("Alpha 扰动极差过大: %.6f° (阈值 1.0°)", alphaRange)
	}
	// 4. IsValid 稳定(不翻转)
	if m.IsValidFlips > 0 {
		t.Errorf("IsValid 在扰动下翻转 %d 次, 期望 0(中心区应稳定有效)", m.IsValidFlips)
	}
	t.Logf("三孔扰动统计: Alpha[%.4f, %.4f] maxJump=%.6f°, flips=%d",
		m.AlphaMin, m.AlphaMax, m.AlphaMaxJump, m.IsValidFlips)
}

// TestThreeHole_Stability_DeterministicSeed 验证扰动测试可复现
// 同一种子两次运行, 输出必须完全一致(否则测试不可靠)
func TestThreeHole_Stability_DeterministicSeed(t *testing.T) {
	rng1 := rand.New(rand.NewSource(42))
	rng2 := rand.New(rand.NewSource(42))
	base := calibInputForAlpha(5)
	for i := 0; i < 10; i++ {
		a := perturbThreeHole(base, rng1, 0.1)
		b := perturbThreeHole(base, rng2, 0.1)
		if a != b {
			t.Fatalf("第 %d 次扰动不一致: a=%+v b=%+v (种子不可复现)", i, a, b)
		}
	}
}
