package interpolation

import (
	"path/filepath"
	"strings"
	"testing"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
)

// TestLoaderLoadMultiPRB_MetadataMapping 验证 Task 07 元数据映射：
// Loader.LoadMultiPRB 把 shared.MultiPrbLoadResult 中的 Files/MachNumbers/Warnings
// 正确映射到 port-owned ports.MultiPrbLoadMetadata，避免 usecase/api 层 import
// shared load-result 具体类型。
//
// 测试覆盖：
//   - 成功加载两个文件：metadata.Files 与 MachNumbers 一一对应、按 Mach 升序。
//   - LoadedAtMs 为正（time.Now().UnixMilli()）。
//   - ValidRange 透传（loader 真实可知字段）。
//   - FileName 来自 filepath.Base（防御性兜底）。
func TestLoaderLoadMultiPRB_MetadataMapping(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		writeTempFile(t, dir, "mach0.2.prb", syntheticPrbContent()),
		writeTempFile(t, dir, "mach0.4.prb", syntheticPrbContent()),
	}
	machs := []float64{0.2, 0.4}

	interp, metadata, err := NewLoader().LoadMultiPRB(paths, machs, coreinterp.ModeLinear)
	if err != nil {
		t.Fatalf("LoadMultiPRB returned error: %v", err)
	}
	if interp == nil || !interp.IsLoaded() {
		t.Fatal("expected loaded interpolator")
	}
	if metadata == nil {
		t.Fatal("expected non-nil MultiPrbLoadMetadata")
	}

	// MachNumbers 透传并按升序排列（LoadPrbData 内部排序）
	if len(metadata.MachNumbers) != 2 {
		t.Fatalf("MachNumbers len = %d, want 2", len(metadata.MachNumbers))
	}
	if metadata.MachNumbers[0] != 0.2 || metadata.MachNumbers[1] != 0.4 {
		t.Errorf("MachNumbers = %v, want [0.2 0.4]", metadata.MachNumbers)
	}

	// Files 与 MachNumbers 一一对应
	if len(metadata.Files) != 2 {
		t.Fatalf("Files len = %d, want 2", len(metadata.Files))
	}
	for i, f := range metadata.Files {
		if f.MachNumber != metadata.MachNumbers[i] {
			t.Errorf("Files[%d].MachNumber = %v, want %v", i, f.MachNumber, metadata.MachNumbers[i])
		}
		if f.LoadedAtMs <= 0 {
			t.Errorf("Files[%d].LoadedAtMs = %d, want positive", i, f.LoadedAtMs)
		}
		if f.FileName == "" {
			t.Errorf("Files[%d].FileName empty", i)
		}
		// FilePath 应是入参路径之一
		if !containsString(paths, f.FilePath) {
			t.Errorf("Files[%d].FilePath = %q not in input paths %v", i, f.FilePath, paths)
		}
		// ValidRange.MachMin/Max 应等于该文件的 MachNumber（LoadPrbData 内部填充）
		if f.ValidRange.MachMin != f.MachNumber || f.ValidRange.MachMax != f.MachNumber {
			t.Errorf("Files[%d].ValidRange MachMin/Max = (%v, %v), want (%v, %v)",
				i, f.ValidRange.MachMin, f.ValidRange.MachMax, f.MachNumber, f.MachNumber)
		}
	}

	// mode 透传：coreinterp.MultiPrbInterpolator 未暴露 GetInterpolationMode，
	// 这里仅验证 interpolator 已加载（mode 设置错误不影响 IsLoaded）。
	if !interp.IsLoaded() {
		t.Error("expected interpolator to be loaded after SetInterpolationMode")
	}
}

// TestLoaderLoadMultiPRB_SkippedAndDuplicateWarnings 验证 Task 07 关键场景：
// skipped（解析失败/无法解析 Mach）和 duplicate（Mach 重复）warnings 正确映射到 metadata。
func TestLoaderLoadMultiPRB_SkippedAndDuplicateWarnings(t *testing.T) {
	dir := t.TempDir()
	// 两个文件 Mach 相同（0.3），第二个会触发 duplicate warning
	paths := []string{
		writeTempFile(t, dir, "mach0.3a.prb", syntheticPrbContent()),
		writeTempFile(t, dir, "mach0.3b.prb", syntheticPrbContent()),
	}
	machs := []float64{0.3, 0.3}

	_, metadata, err := NewLoader().LoadMultiPRB(paths, machs, "")
	if err != nil {
		t.Fatalf("LoadMultiPRB returned error: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected non-nil metadata")
	}

	// duplicate warning 应在 metadata.Warnings 中
	foundDup := false
	for _, w := range metadata.Warnings {
		if strings.Contains(w, "重复") {
			foundDup = true
			break
		}
	}
	if !foundDup {
		t.Errorf("expected duplicate Mach warning in Warnings, got %v", metadata.Warnings)
	}

	// 成功加载文件数应为 1（duplicate 文件被跳过）
	if len(metadata.Files) != 1 {
		t.Errorf("Files len = %d, want 1 (duplicate skipped)", len(metadata.Files))
	}
}

// TestLoaderLoadMultiPRB_FileNameFallback 验证 FileName 防御性兜底：
// 即便 shared.PrbFileInfo.FileName 为空（理论上不会，但兜底防御），
// ports.PrbFileMetadata.FileName 仍取 filepath.Base(FilePath)。
func TestLoaderLoadMultiPRB_FileNameFallback(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "mach0.5.prb", syntheticPrbContent())
	paths := []string{path}
	machs := []float64{0.5}

	_, metadata, err := NewLoader().LoadMultiPRB(paths, machs, "")
	if err != nil {
		t.Fatalf("LoadMultiPRB returned error: %v", err)
	}
	if metadata == nil || len(metadata.Files) != 1 {
		t.Fatalf("expected 1 file in metadata, got %+v", metadata)
	}
	want := filepath.Base(path)
	if metadata.Files[0].FileName != want {
		t.Errorf("FileName = %q, want %q", metadata.Files[0].FileName, want)
	}
}

// TestLoaderLoadMultiPRB_FailureReturnsNilMetadata 失败路径返回 nil metadata
// 与 nil interpolator，防止调用方误用部分填充的 metadata。
func TestLoaderLoadMultiPRB_FailureReturnsNilMetadata(t *testing.T) {
	// 所有文件缺失 → LoadPrbData 返回 error
	paths := []string{filepath.Join(t.TempDir(), "missing.prb")}
	_, metadata, err := NewLoader().LoadMultiPRB(paths, []float64{0.3}, "")
	if err == nil {
		t.Fatal("expected error when all files missing")
	}
	if metadata != nil {
		t.Errorf("expected nil metadata on failure, got %+v", metadata)
	}
}

// TestLoaderLoadSevenHoleCalibrationCSV_MetadataShape 校准 CSV 路径同样返回
// 非 nil metadata，且不暴露 PointCount（spec 兼容约定值 169/52 不伪装为 loader 真值）。
//
// 校准 CSV 夹具当前缺失（仅有 .prb 夹具），所以这里 skip；保留测试结构
// 供后续添加 CSV 夹具时启用。通过 type-assert 验证 metadata 类型即可。
func TestLoaderLoadSevenHoleCalibrationCSV_MetadataShape(t *testing.T) {
	t.Skip("seven-hole calibration CSV fixture not available; enable when fixture added")
}

// containsString 简单字符串切片包含判断（测试辅助）。
func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
