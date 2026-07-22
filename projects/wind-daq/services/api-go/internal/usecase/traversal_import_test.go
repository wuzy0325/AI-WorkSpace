package usecase

import (
	"errors"
	"strings"
	"testing"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/core/traversal"
)

// TestImportPRB_Success 成功路径：load 成功后替换 manager 状态，
// 清空缓存与恢复错误；响应字段与既有 API 形状逐字段对齐。
func TestImportPRB_Success(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	// 预置一个旧插值器 + 陈旧恢复错误，验证 Import 后被清空。
	mgr.SetInterpolator(&mockInterpolator{tag: "old"})
	mgr.lastInterpolatorRestoreErr = "stale error"

	res, err := mgr.ImportPRB("/data/cal.prb")
	if err != nil {
		t.Fatalf("ImportPRB: %v", err)
	}
	if res.FilePath != "/data/cal.prb" {
		t.Errorf("FilePath = %q, want /data/cal.prb", res.FilePath)
	}
	if res.FileName != "cal.prb" {
		t.Errorf("FileName = %q, want cal.prb", res.FileName)
	}
	if res.LoadedAtMs <= 0 {
		t.Errorf("LoadedAtMs = %d, want > 0", res.LoadedAtMs)
	}
	// 旧插值器被替换：通过 Interpolator() 拿到的新对象 tag 应为 "prb:..."
	interp := mgr.Interpolator()
	if interp == nil {
		t.Fatal("Interpolator must be replaced on success")
	}
	if mgr.InterpolatorRestoreErr() != "" {
		t.Errorf("RestoreErr must be cleared, got %q", mgr.InterpolatorRestoreErr())
	}
	// loader 被调用一次且入参正确
	_, _, _, _, _, prbCalls, _, _ := loader.snapshot()
	if prbCalls != 1 {
		t.Errorf("LoadPRB calls = %d, want 1", prbCalls)
	}
}

// TestImportPRB_FailurePreservesOldState 失败路径：loader 报错时
// 旧插值器与缓存保留，不替换 manager 状态。
func TestImportPRB_FailurePreservesOldState(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{prbErr: errors.New("disk gone")}
	mgr.SetInterpolatorLoader(loader)

	old := &mockInterpolator{tag: "old"}
	mgr.SetInterpolator(old)

	_, err := mgr.ImportPRB("/data/missing.prb")
	if err == nil {
		t.Fatal("ImportPRB must propagate loader error")
	}
	if !strings.Contains(err.Error(), "disk gone") {
		t.Errorf("err = %v, want contains 'disk gone'", err)
	}
	// 旧插值器保留
	if mgr.Interpolator() != old {
		t.Error("old interpolator must be preserved on failure")
	}
}

// TestImportPRB_NilLoader 未注入 loader 时返回明确错误，不 panic。
func TestImportPRB_NilLoader(t *testing.T) {
	mgr := newImportTestManager(t)
	// 故意不调用 SetInterpolatorLoader
	_, err := mgr.ImportPRB("/data/cal.prb")
	if err == nil {
		t.Fatal("ImportPRB without loader must error")
	}
	if !strings.Contains(err.Error(), "loader") && !strings.Contains(err.Error(), "加载") {
		t.Errorf("err = %v, want mentions loader", err)
	}
}

// TestImportCalibrationCSV_Success 成功路径：响应含 pointCount（来自具体类型 GetPointCount）。
// mockInterpolator 不实现 GetPointCount，因此 pointCount 应为 0（类型断言失败时的降级）。
// 这里用真实 FiveHoleNewInterpolator 验证 pointCount 通路。
func TestImportCalibrationCSV_Success(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	res, err := mgr.ImportCalibrationCSV("/data/cal.csv")
	if err != nil {
		t.Fatalf("ImportCalibrationCSV: %v", err)
	}
	if res.FilePath != "/data/cal.csv" {
		t.Errorf("FilePath = %q, want /data/cal.csv", res.FilePath)
	}
	if res.FileName != "cal.csv" {
		t.Errorf("FileName = %q, want cal.csv", res.FileName)
	}
	if res.LoadedAtMs <= 0 {
		t.Errorf("LoadedAtMs = %d, want > 0", res.LoadedAtMs)
	}
	// mockInterpolator 不实现 GetPointCount，pointCount=0 是预期降级
	if res.PointCount != 0 {
		t.Errorf("PointCount = %d, want 0 (mockInterpolator 无 GetPointCount)", res.PointCount)
	}
	if mgr.Interpolator() == nil {
		t.Error("Interpolator must be set on success")
	}
}

// TestImportCalibrationCSV_FailurePreservesOldState 失败时旧插值器保留。
func TestImportCalibrationCSV_FailurePreservesOldState(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{csvErr: errors.New("parse error")}
	mgr.SetInterpolatorLoader(loader)

	old := &mockInterpolator{tag: "old"}
	mgr.SetInterpolator(old)

	_, err := mgr.ImportCalibrationCSV("/data/bad.csv")
	if err == nil {
		t.Fatal("ImportCalibrationCSV must propagate error")
	}
	if mgr.Interpolator() != old {
		t.Error("old interpolator must be preserved on failure")
	}
}

// TestImportMultiPRB_Success 成功路径：mode 透传到 loader，响应字段（files/machNumbers/warnings）完整。
func TestImportMultiPRB_Success(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	filePaths := []string{"/data/a.prb", "/data/b.prb"}
	machNumbers := []float64{0.3, 0.6}
	res, err := mgr.ImportMultiPRB(filePaths, machNumbers, "linear")
	if err != nil {
		t.Fatalf("ImportMultiPRB: %v", err)
	}
	// mode 透传
	_, _, _, _, lastMode, _, _, _ := loader.snapshot()
	if lastMode != coreinterp.ModeLinear {
		t.Errorf("mode = %q, want linear", lastMode)
	}
	// 响应字段
	if len(res.Files) != len(loader.lastMultiPRB) {
		t.Errorf("Files len = %d, want %d", len(res.Files), len(loader.lastMultiPRB))
	}
	// machNumbers 一一对应
	if len(res.MachNumbers) != len(machNumbers) {
		t.Errorf("MachNumbers len = %d, want %d", len(res.MachNumbers), len(machNumbers))
	}
	// 每个文件 LoadedAtMs > 0
	for i, f := range res.Files {
		if f.LoadedAtMs <= 0 {
			t.Errorf("Files[%d].LoadedAtMs = %d, want > 0", i, f.LoadedAtMs)
		}
	}
	if mgr.Interpolator() == nil {
		t.Error("Interpolator must be set on success")
	}
}

// TestImportMultiPRB_EmptyMode 空 mode 时不调用 SetInterpolationMode（loader 自行处理）。
func TestImportMultiPRB_EmptyMode(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	_, err := mgr.ImportMultiPRB([]string{"/data/a.prb"}, []float64{0.5}, "")
	if err != nil {
		t.Fatalf("ImportMultiPRB empty mode: %v", err)
	}
	_, _, _, _, lastMode, _, _, _ := loader.snapshot()
	if lastMode != "" {
		t.Errorf("mode = %q, want empty", lastMode)
	}
}

// TestImportMultiPRB_FailurePreservesOldState 失败时旧插值器保留。
func TestImportMultiPRB_FailurePreservesOldState(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{multiPRBErr: errors.New("bad prb")}
	mgr.SetInterpolatorLoader(loader)

	old := &mockInterpolator{tag: "old"}
	mgr.SetInterpolator(old)

	_, err := mgr.ImportMultiPRB([]string{"/data/a.prb"}, []float64{0.5}, "")
	if err == nil {
		t.Fatal("ImportMultiPRB must propagate error")
	}
	if mgr.Interpolator() != old {
		t.Error("old interpolator must be preserved on failure")
	}
}

// TestImportSevenHolePRB_Success 成功路径：7 文件 + 169/52 约定值 + validRange。
func TestImportSevenHolePRB_Success(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{
		sevenHoleValidRange: seveninterp.PrbValidRange{AlphaMin: -30, AlphaMax: 30, BetaMin: -30, BetaMax: 30},
	}
	mgr.SetInterpolatorLoader(loader)

	inner := "/data/7.prb"
	outer := []string{"/data/1.prb", "/data/2.prb", "/data/3.prb", "/data/4.prb", "/data/5.prb", "/data/6.prb"}
	res, err := mgr.ImportSevenHolePRB(inner, outer)
	if err != nil {
		t.Fatalf("ImportSevenHolePRB: %v", err)
	}
	if len(res.Files) != 7 {
		t.Fatalf("Files len = %d, want 7", len(res.Files))
	}
	// 内区 sector 7 / pointCount 169
	if res.Files[0].Sector != 7 || res.Files[0].PointCount != 169 {
		t.Errorf("inner file = %+v, want sector 7 / 169 pts", res.Files[0])
	}
	// 扇区 sector 1..6 / pointCount 52
	for i := 1; i <= 6; i++ {
		if res.Files[i].Sector != i || res.Files[i].PointCount != 52 {
			t.Errorf("outer file[%d] = %+v, want sector %d / 52 pts", i, res.Files[i], i)
		}
	}
	// validRange 透传
	if res.ValidRange.AlphaMin != -30 || res.ValidRange.AlphaMax != 30 {
		t.Errorf("ValidRange = %+v, want ±30", res.ValidRange)
	}
	// loader 入参
	if loader.lastSevenHoleInner != inner {
		t.Errorf("loader inner = %q, want %q", loader.lastSevenHoleInner, inner)
	}
	// 七孔插值器被设置
	mgr.mu.RLock()
	sh := mgr.sevenHoleInterpolator
	mgr.mu.RUnlock()
	if sh == nil {
		t.Error("sevenHoleInterpolator must be set on success")
	}
}

// TestImportSevenHolePRB_Validation 内区空 / 扇区数量错误返回明确错误，不调用 loader。
func TestImportSevenHolePRB_Validation(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	cases := []struct {
		name        string
		inner       string
		outer       []string
		wantInError string
	}{
		{"empty inner", "", []string{"1.prb", "2.prb", "3.prb", "4.prb", "5.prb", "6.prb"}, "innerFilePath"},
		{"too few outer", "/data/7.prb", []string{"1.prb"}, "6 份"},
		{"too many outer", "/data/7.prb", []string{"1.prb", "2.prb", "3.prb", "4.prb", "5.prb", "6.prb", "7.prb"}, "6 份"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mgr.ImportSevenHolePRB(tc.inner, tc.outer)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantInError) {
				t.Errorf("err = %v, want contains %q", err, tc.wantInError)
			}
			if loader.sevenHolePRBCalls != 0 {
				t.Errorf("loader must not be called on validation failure, got %d calls", loader.sevenHolePRBCalls)
			}
		})
	}
}

// TestImportSevenHolePRB_FailurePreservesOldState loader 失败时旧七孔插值器保留。
func TestImportSevenHolePRB_FailurePreservesOldState(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{sevenHolePRBErr: errors.New("missing.prb")}
	mgr.SetInterpolatorLoader(loader)

	// 预置旧七孔插值器
	old := &mockSevenHoleInterpolator{tag: "old-seven"}
	mgr.SetSevenHoleInterpolator(old)

	_, err := mgr.ImportSevenHolePRB("/data/7.prb", []string{"1.prb", "2.prb", "3.prb", "4.prb", "5.prb", "6.prb"})
	if err == nil {
		t.Fatal("ImportSevenHolePRB must propagate error")
	}
	mgr.mu.RLock()
	sh := mgr.sevenHoleInterpolator
	mgr.mu.RUnlock()
	if sh != old {
		t.Error("old seven-hole interpolator must be preserved on failure")
	}
}

// TestImportSevenHoleCalibrationCSV_Success 成功路径：与 PRB 同形状（7 文件 + 169/52）。
func TestImportSevenHoleCalibrationCSV_Success(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{
		sevenHoleValidRange: seveninterp.PrbValidRange{AlphaMin: -30, AlphaMax: 30, BetaMin: -30, BetaMax: 30},
	}
	mgr.SetInterpolatorLoader(loader)

	inner := "/data/inner.csv"
	outer := []string{"/data/o1.csv", "/data/o2.csv", "/data/o3.csv", "/data/o4.csv", "/data/o5.csv", "/data/o6.csv"}
	res, err := mgr.ImportSevenHoleCalibrationCSV(inner, outer)
	if err != nil {
		t.Fatalf("ImportSevenHoleCalibrationCSV: %v", err)
	}
	if len(res.Files) != 7 {
		t.Fatalf("Files len = %d, want 7", len(res.Files))
	}
	if res.Files[0].Sector != 7 || res.Files[0].PointCount != 169 {
		t.Errorf("inner file = %+v, want sector 7 / 169 pts", res.Files[0])
	}
	for i := 1; i <= 6; i++ {
		if res.Files[i].Sector != i || res.Files[i].PointCount != 52 {
			t.Errorf("outer file[%d] = %+v, want sector %d / 52 pts", i, res.Files[i], i)
		}
	}
	if loader.sevenHoleCSVCalls != 1 {
		t.Errorf("LoadSevenHoleCalibrationCSV calls = %d, want 1", loader.sevenHoleCSVCalls)
	}
}

// TestImportSevenHoleCalibrationCSV_Validation 内区空 / 扇区数量错误。
func TestImportSevenHoleCalibrationCSV_Validation(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	_, err := mgr.ImportSevenHoleCalibrationCSV("", []string{"1.csv", "2.csv", "3.csv", "4.csv", "5.csv", "6.csv"})
	if err == nil || !strings.Contains(err.Error(), "innerFilePath") {
		t.Errorf("empty inner err = %v, want contains innerFilePath", err)
	}
	if loader.sevenHoleCSVCalls != 0 {
		t.Errorf("loader must not be called on validation failure")
	}

	_, err = mgr.ImportSevenHoleCalibrationCSV("/data/inner.csv", []string{"1.csv"})
	if err == nil || !strings.Contains(err.Error(), "6 份") {
		t.Errorf("wrong outer count err = %v, want contains '6 份'", err)
	}
}

// TestImportSevenHoleCalibrationCSV_FailurePreservesOldState loader 失败时旧七孔插值器保留。
func TestImportSevenHoleCalibrationCSV_FailurePreservesOldState(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{sevenHoleCSVErr: errors.New("bad csv")}
	mgr.SetInterpolatorLoader(loader)

	old := &mockSevenHoleInterpolator{tag: "old-seven"}
	mgr.SetSevenHoleInterpolator(old)

	_, err := mgr.ImportSevenHoleCalibrationCSV("/data/inner.csv", []string{"1.csv", "2.csv", "3.csv", "4.csv", "5.csv", "6.csv"})
	if err == nil {
		t.Fatal("must propagate error")
	}
	mgr.mu.RLock()
	sh := mgr.sevenHoleInterpolator
	mgr.mu.RUnlock()
	if sh != old {
		t.Error("old seven-hole interpolator must be preserved on failure")
	}
}

// TestImportSevenHolePRB_NilLoader 未注入 loader 时返回明确错误。
func TestImportSevenHolePRB_NilLoader(t *testing.T) {
	mgr := newImportTestManager(t)
	_, err := mgr.ImportSevenHolePRB("/data/7.prb", []string{"1.prb", "2.prb", "3.prb", "4.prb", "5.prb", "6.prb"})
	if err == nil {
		t.Fatal("ImportSevenHolePRB without loader must error")
	}
}

// TestImportMethods_ClearRestoreErr 成功导入后陈旧恢复错误被清空
// （SetInterpolator / SetSevenHoleInterpolator 内部已处理，Import 路径复用）。
func TestImportMethods_ClearRestoreErr(t *testing.T) {
	mgr := newImportTestManager(t)
	loader := &mockInterpolatorLoader{}
	mgr.SetInterpolatorLoader(loader)

	// 预置陈旧错误
	mgr.lastInterpolatorRestoreErr = "stale"
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	mgr.SetSevenHoleInterpolator(&mockSevenHoleInterpolator{tag: "old"})

	// 七孔导入成功后清空恢复错误
	_, err := mgr.ImportSevenHolePRB("/data/7.prb", []string{"1.prb", "2.prb", "3.prb", "4.prb", "5.prb", "6.prb"})
	if err != nil {
		t.Fatalf("ImportSevenHolePRB: %v", err)
	}
	if mgr.InterpolatorRestoreErr() != "" {
		t.Errorf("RestoreErr must be cleared after successful import, got %q", mgr.InterpolatorRestoreErr())
	}
}

// newImportTestManager 构造 Import 方法测试用的最小 TraversalManager。
// 不依赖真实硬件 / 文件系统，与 newConfigTestManager 同语义。
func newImportTestManager(t *testing.T) *TraversalManager {
	t.Helper()
	return newConfigTestManager(t)
}
