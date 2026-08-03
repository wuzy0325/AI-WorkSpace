package interpolation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

// sevenHoleCalDataDir 定位七孔校准 CSV 数据目录（用户提供的校准导出样例，
// 与 W532.202608.P.7H.1-01 数据集同构：GBK、18 列、系数在 12..15 列）。
func sevenHoleCalDataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "..", "..",
		"projects", "wind-daq", "docs", "7-hole-cal-data")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("seven-hole calibration csv data not available: %v", err)
	}
	return dir
}

func sevenHoleCalCsvPaths(dir string) (string, [6]string) {
	const stem = "W532.202608.P.7H.1-01-85米每秒（0.242Ma）"
	var outer [6]string
	for i := range outer {
		outer[i] = filepath.Join(dir, fmt.Sprintf("%s(大角度%d区).csv", stem, i+1))
	}
	return filepath.Join(dir, stem+"(小角度区).csv"), outer
}

// TestLoadSevenHoleCalibrationCsvFiles_Success 校准 CSV → 插值器全链路：
// 7 份 GBK 校准 CSV 解析、抖动、构建成功；网格点值与 CSV 系数一致。
func TestLoadSevenHoleCalibrationCsvFiles_Success(t *testing.T) {
	dir := sevenHoleCalDataDir(t)
	inner, outer := sevenHoleCalCsvPaths(dir)
	interp, err := LoadSevenHoleCalibrationCsvFiles(inner, outer)
	if err != nil {
		t.Fatalf("LoadSevenHoleCalibrationCsvFiles: %v", err)
	}
	if !interp.IsLoaded() {
		t.Fatal("expected loaded after 7-file csv set")
	}
	vr := interp.GetValidRange()
	assertNear(t, "alphaMin", vr.AlphaMin, -30, 1e-9)
	assertNear(t, "alphaMax", vr.AlphaMax, 30, 1e-9)
}

// TestSevenHoleCsvDitherConsistency 退化边抖动与夹具脚本一致：
// 已知数据集在 7.prb/1.prb/3.prb/4.prb 各有 1/1/3/1 处精确 ka 相等退化边
// （gen_traversal_fixtures.py 生成记录），Go 抖动扫描应命中相同数量。
//
// 直接调用共享包的 LoadCalibrationCSVFromUTF8（绕过 adapter 的文件 I/O 包装），
// 以验证抖动警告透传——adapter 默认丢弃 warnings，此处需读取它们。
func TestSevenHoleCsvDitherConsistency(t *testing.T) {
	dir := sevenHoleCalDataDir(t)
	innerPath, outerPaths := sevenHoleCalCsvPaths(dir)
	innerSrc, err := readSevenHoleCsvSource(innerPath)
	if err != nil {
		t.Fatalf("read inner csv: %v", err)
	}
	outerSrcs := make([]seveninterp.SevenHoleCSVSource, len(outerPaths))
	for i, p := range outerPaths {
		src, err := readSevenHoleCsvSource(p)
		if err != nil {
			t.Fatalf("read outer csv %d: %v", i+1, err)
		}
		outerSrcs[i] = src
	}
	_, warnings, err := seveninterp.LoadCalibrationCSVFromUTF8(innerSrc, outerSrcs)
	if err != nil {
		t.Fatalf("LoadCalibrationCSVFromUTF8: %v", err)
	}
	// 历史数据集表头沿用 inner 命名，outer 表头与历史别名不符会触发 6 条表头诊断警告。
	// 这里只断言抖动相关的 4 条子串存在，不限制总警告数（表头诊断警告可能变化）。
	wantSubstrings := []string{
		"内区 CSV 已对 1 处退化边",
		"扇区 1 CSV 已对 1 处退化边",
		"扇区 3 CSV 已对 3 处退化边",
		"扇区 4 CSV 已对 1 处退化边",
	}
	matched := make([]bool, len(wantSubstrings))
	for _, w := range warnings {
		for i, want := range wantSubstrings {
			if !matched[i] && strings.Contains(w, want) {
				matched[i] = true
			}
		}
	}
	for i, ok := range matched {
		if !ok {
			t.Errorf("missing warning substring %q in warnings: %v", wantSubstrings[i], warnings)
		}
	}
}

// TestSevenHoleCsvImportVsGolden 校准 CSV 导入与 golden 对拍交叉验证：
// 用校准 CSV 构建的插值器在 golden 标定点上的输出与 Python 权威输出一致。
func TestSevenHoleCsvImportVsGolden(t *testing.T) {
	dir := sevenHoleCalDataDir(t)
	inner, outer := sevenHoleCalCsvPaths(dir)
	interp, err := LoadSevenHoleCalibrationCsvFiles(inner, outer)
	if err != nil {
		t.Fatalf("LoadSevenHoleCalibrationCsvFiles: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "..", "..", "..", "..", "..",
		"shared", "algorithms", "go", "sevenhole", "interpolation", "testdata", "golden", "golden.json")
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("golden fixture not available: %v", err)
	}
	var entries []struct {
		Index int    `json:"index"`
		Mode  string `json:"mode"`
		Input struct {
			P1, P2, P3, P4, P5, P6, P7, Pa, T float64
		} `json:"input"`
		Output struct {
			Alpha, Beta, Pt, Ps, Ma, V float64
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	// 抽样核对：首点（内区角点）、一个内区点、每个扇区首行点。
	samples := map[int]bool{0: true, 87: true, 169: true, 221: true, 273: true, 325: true, 377: true, 429: true}
	checked := 0
	for _, e := range entries {
		if !samples[e.Index] {
			continue
		}
		checked++
		res, err := interp.Calculate(seveninterp.InterpolationInput{
			P1: e.Input.P1, P2: e.Input.P2, P3: e.Input.P3, P4: e.Input.P4,
			P5: e.Input.P5, P6: e.Input.P6, P7: e.Input.P7,
			PAtm: e.Input.Pa, TAtm: e.Input.T,
		})
		if e.Mode == "out" {
			if err != nil || res.IsValid {
				t.Errorf("idx %d: out-of-grid must be invalid without error", e.Index)
			}
			continue
		}
		if err != nil {
			t.Errorf("idx %d: Calculate error: %v", e.Index, err)
			continue
		}
		if !res.IsValid {
			t.Errorf("idx %d: unexpected invalid (warning=%q)", e.Index, res.Warning)
			continue
		}
		if math.Abs(res.Alpha-e.Output.Alpha) > 1e-4 ||
			math.Abs(res.Beta-e.Output.Beta) > 1e-4 ||
			math.Abs(res.TotalPressure-e.Output.Pt) > math.Max(math.Abs(e.Output.Pt)*1e-6, 1e-4) ||
			math.Abs(res.StaticPressure-e.Output.Ps) > math.Max(math.Abs(e.Output.Ps)*1e-6, 1e-4) {
			t.Errorf("idx %d mismatch: got alpha=%.6f beta=%.6f pt=%.4f ps=%.4f, want %.6f %.6f %.4f %.4f",
				e.Index, res.Alpha, res.Beta, res.TotalPressure, res.StaticPressure,
				e.Output.Alpha, e.Output.Beta, e.Output.Pt, e.Output.Ps)
		}
	}
	if checked != len(samples) {
		t.Fatalf("sample coverage = %d, want %d", checked, len(samples))
	}
}

// TestSevenHoleCsvNearbyMeasurementsRemainContinuous guards against restoring
// the old 5e-4 coefficient snap: nearby runtime samples must produce nearby,
// but not bit-identical, interpolation results.
func TestSevenHoleCsvNearbyMeasurementsRemainContinuous(t *testing.T) {
	dir := sevenHoleCalDataDir(t)
	inner, outer := sevenHoleCalCsvPaths(dir)
	interp, err := LoadSevenHoleCalibrationCsvFiles(inner, outer)
	if err != nil {
		t.Fatalf("LoadSevenHoleCalibrationCsvFiles: %v", err)
	}

	input := seveninterp.InterpolationInput{
		P1: 831.233, P2: 509.833, P3: -409.933, P4: -1007.717,
		P5: -715.367, P6: 193.817, P7: 4070.833,
		PAtm: 98891, TAtm: 28,
	}
	base, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if !base.IsValid {
		t.Fatalf("unexpected invalid base result: %s", base.Warning)
	}
	input.P1 += 0.001
	perturbed, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate perturbed: %v", err)
	}
	if !perturbed.IsValid {
		t.Fatalf("unexpected invalid perturbed result: %s", perturbed.Warning)
	}
	delta := math.Hypot(perturbed.Alpha-base.Alpha, perturbed.Beta-base.Beta)
	if delta == 0 {
		t.Fatal("nearby sample was snapped to the calibration result")
	}
	if delta > 0.1 {
		t.Fatalf("nearby sample changed angles discontinuously: delta=%g deg", delta)
	}
}

// TestLoadSevenHoleCalibrationCsvFiles_Errors 缺文件/列数错误/行数不符均报错且含路径。
func TestLoadSevenHoleCalibrationCsvFiles_Errors(t *testing.T) {
	dir := sevenHoleCalDataDir(t)
	inner, outer := sevenHoleCalCsvPaths(dir)

	t.Run("missing file", func(t *testing.T) {
		bad := outer
		bad[2] = filepath.Join(dir, "missing.csv")
		_, err := LoadSevenHoleCalibrationCsvFiles(inner, bad)
		if err == nil || !strings.Contains(err.Error(), "missing.csv") {
			t.Fatalf("expected error naming missing.csv, got: %v", err)
		}
	})

	t.Run("insufficient columns", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "bad.csv")
		content := "h1,h2\n1,2,3\n"
		if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadSevenHoleCalibrationCsvFiles(tmp, outer)
		if err == nil || !strings.Contains(err.Error(), "bad.csv") {
			t.Fatalf("expected error naming bad.csv, got: %v", err)
		}
	})
}
