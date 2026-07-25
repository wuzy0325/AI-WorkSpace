package interpolation

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

type unsupportedSevenHoleInterpolator struct{}

func (unsupportedSevenHoleInterpolator) IsLoaded() bool { return true }
func (unsupportedSevenHoleInterpolator) GetValidRange() seveninterp.PrbValidRange {
	return seveninterp.PrbValidRange{}
}
func (unsupportedSevenHoleInterpolator) Calculate(seveninterp.InterpolationInput) (seveninterp.InterpolationResult, error) {
	return seveninterp.InterpolationResult{}, nil
}

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func syntheticPrbContent() string {
	var b strings.Builder
	b.WriteString("13 13\n")
	for alpha := -30.0; alpha <= 30; alpha += 5 {
		for beta := -30.0; beta <= 30; beta += 5 {
			b.WriteString(fmt.Sprintf("%.6f %.6f %.6f %.6f %.0f %.0f\n",
				alpha/100, beta/100, 0.05, 0.01, alpha, beta))
		}
	}
	return b.String()
}

func syntheticFiveHoleCsvContent() string {
	var b strings.Builder
	b.WriteString("alpha,beta,p1,p2,p3,p4,p5\n")
	for _, alpha := range []float64{-20, 0, 20} {
		for _, beta := range []float64{-20, 0, 20} {
			b.WriteString(fmt.Sprintf("%.0f,%.0f,%.6f,%.6f,%.6f,%.6f,%.6f\n",
				alpha, beta, 100-beta, 200.0, 100+beta, 100+alpha, 100-alpha))
		}
	}
	return b.String()
}

func assertNear(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %v, want %v +/- %v", name, got, want, tolerance)
	}
}

// ==================== readNonEmptyLines ====================

func TestReadNonEmptyLines_SkipsEmptyAndWhitespaceLines(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "test.txt", "line1\n\nline2\n  \n  \nline3\n")

	lines, err := readNonEmptyLines(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"line1", "line2", "line3"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestReadNonEmptyLines_AllEmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "empty.txt", "\n\n\n")

	lines, err := readNonEmptyLines(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}

func TestReadNonEmptyLines_MissingFile(t *testing.T) {
	_, err := readNonEmptyLines(filepath.Join(t.TempDir(), "nonexistent.txt"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ==================== LoadPrbFile ====================

func TestLoadPrbFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "test.prb", syntheticPrbContent())

	interp, err := LoadPrbFile(path)
	if err != nil {
		t.Fatalf("LoadPrbFile returned error: %v", err)
	}
	if !interp.IsLoaded() {
		t.Fatal("expected interpolator to be loaded")
	}
	vr := interp.GetValidRange()
	assertNear(t, "alphaMin", vr.AlphaMin, -30, 1e-9)
	assertNear(t, "alphaMax", vr.AlphaMax, 30, 1e-9)
	assertNear(t, "betaMin", vr.BetaMin, -30, 1e-9)
	assertNear(t, "betaMax", vr.BetaMax, 30, 1e-9)
}

func TestLoadPrbFile_MissingFile(t *testing.T) {
	_, err := LoadPrbFile(filepath.Join(t.TempDir(), "nonexistent.prb"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadPrbFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "empty.prb", "")

	_, err := LoadPrbFile(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

// ==================== LoadFiveHoleNewFile ====================

func TestLoadFiveHoleNewFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "test.csv", syntheticFiveHoleCsvContent())

	interp, err := LoadFiveHoleNewFile(path)
	if err != nil {
		t.Fatalf("LoadFiveHoleNewFile returned error: %v", err)
	}
	if !interp.IsLoaded() {
		t.Fatal("expected interpolator to be loaded")
	}
}

func TestLoadFiveHoleNewFile_MissingFile(t *testing.T) {
	_, err := LoadFiveHoleNewFile(filepath.Join(t.TempDir(), "nonexistent.csv"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFiveHoleNewFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "empty.csv", "")

	_, err := LoadFiveHoleNewFile(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

// ==================== LoadMultiPrbFiles ====================

func TestLoadMultiPrbFiles_Success(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		writeTempFile(t, dir, "mach0.2.prb", syntheticPrbContent()),
		writeTempFile(t, dir, "mach0.4.prb", syntheticPrbContent()),
	}
	machs := []float64{0.2, 0.4}

	interp, result, err := LoadMultiPrbFiles(paths, machs)
	if err != nil {
		t.Fatalf("LoadMultiPrbFiles returned error: %v", err)
	}
	if !interp.IsLoaded() {
		t.Fatal("expected interpolator to be loaded")
	}
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files loaded, got %d", len(result.Files))
	}
}

func TestLoadMultiPrbFiles_EmptyList(t *testing.T) {
	_, _, err := LoadMultiPrbFiles([]string{}, []float64{})
	if err == nil {
		t.Fatal("expected error for empty file list")
	}
}

func TestLoadMultiPrbFiles_OneFileMissing(t *testing.T) {
	dir := t.TempDir()
	good := writeTempFile(t, dir, "good.prb", syntheticPrbContent())
	paths := []string{good, filepath.Join(dir, "missing.prb")}

	_, _, err := LoadMultiPrbFiles(paths, []float64{0.2, 0.4})
	if err == nil {
		t.Fatal("expected error when one file is missing")
	}
}

func TestLoadMultiPrbFiles_WithEmptyLines(t *testing.T) {
	dir := t.TempDir()
	content := "\n\n" + syntheticPrbContent() + "\n\n"
	path := writeTempFile(t, dir, "withblanks.prb", content)

	interp, err := LoadPrbFile(path)
	if err != nil {
		t.Fatalf("LoadPrbFile with blank lines returned error: %v", err)
	}
	if !interp.IsLoaded() {
		t.Fatal("expected interpolator to be loaded despite blank lines")
	}
}

// ==================== LoadSevenHolePrbFiles ====================

// sevenHoleFixtureDir 定位七孔对拍夹具目录（tasks-seven-hole-traversal Task 6 产物）。
func sevenHoleFixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "..", "..",
		"shared", "algorithms", "go", "sevenhole", "interpolation", "testdata", "prb")
	if _, err := os.Stat(filepath.Join(dir, "7.prb")); err != nil {
		t.Skipf("seven-hole fixture set not available: %v", err)
	}
	return dir
}

func sevenHolePaths(dir string) (string, [6]string) {
	var outer [6]string
	for i := range outer {
		outer[i] = filepath.Join(dir, fmt.Sprintf("%d.prb", i+1))
	}
	return filepath.Join(dir, "7.prb"), outer
}

func TestLoadSevenHolePrbFiles_Success(t *testing.T) {
	dir := sevenHoleFixtureDir(t)
	inner, outer := sevenHolePaths(dir)
	interp, err := LoadSevenHolePrbFiles(inner, outer)
	if err != nil {
		t.Fatalf("LoadSevenHolePrbFiles: %v", err)
	}
	if !interp.IsLoaded() {
		t.Fatal("expected interpolator to be loaded after 7-file set")
	}
	vr := interp.GetValidRange()
	assertNear(t, "alphaMin", vr.AlphaMin, -30, 1e-9)
	assertNear(t, "alphaMax", vr.AlphaMax, 30, 1e-9)
	assertNear(t, "betaMin", vr.BetaMin, -30, 1e-9)
	assertNear(t, "betaMax", vr.BetaMax, 30, 1e-9)
}

func TestLoadSevenHolePrbFiles_MissingInner(t *testing.T) {
	dir := sevenHoleFixtureDir(t)
	_, outer := sevenHolePaths(dir)
	_, err := LoadSevenHolePrbFiles(filepath.Join(dir, "missing-inner.prb"), outer)
	if err == nil {
		t.Fatal("expected error for missing inner file")
	}
	if !strings.Contains(err.Error(), "missing-inner.prb") {
		t.Errorf("error must name the file path, got: %v", err)
	}
}

func TestLoadSevenHolePrbFiles_MissingOuter(t *testing.T) {
	dir := sevenHoleFixtureDir(t)
	inner, outer := sevenHolePaths(dir)
	outer[2] = filepath.Join(dir, "missing-3.prb")
	_, err := LoadSevenHolePrbFiles(inner, outer)
	if err == nil {
		t.Fatal("expected error for missing outer file")
	}
	if !strings.Contains(err.Error(), "missing-3.prb") {
		t.Errorf("error must name the file path, got: %v", err)
	}
}

func TestLoadSevenHolePrbFiles_BadRowCount(t *testing.T) {
	dir := sevenHoleFixtureDir(t)
	inner, outer := sevenHolePaths(dir)
	// 用截断的外区文件（51 行 = 非 13 整数倍）触发动态行数校验
	// 动态化后错误消息含"必须是 13 的整数倍且 ≥26"，不再硬编码 52。
	badOuter := writeTruncatedPrb(t, dir, outer[0], 51)
	outer[0] = badOuter
	_, err := LoadSevenHolePrbFiles(inner, outer)
	if err == nil {
		t.Fatal("expected row-count error")
	}
	if !strings.Contains(err.Error(), "13") || !strings.Contains(err.Error(), "26") {
		t.Errorf("error must mention multiple-of-13 and min 26, got: %v", err)
	}
}

// writeTruncatedPrb 复制 srcPath 的前 n 行数据（保留表头）写入新文件，
// 用于测试动态行数校验。
func writeTruncatedPrb(t *testing.T, dir, srcPath string, n int) string {
	t.Helper()
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read src prb: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if n+1 > len(lines) {
		n = len(lines) - 1
	}
	out := strings.Join(lines[:n+1], "\n") + "\n"
	path := filepath.Join(dir, "truncated.prb")
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		t.Fatalf("write truncated prb: %v", err)
	}
	return path
}

// TestLoaderLoadSevenHolePRB ports 层加载：成功返回已加载插值器与中立 metadata；
// 失败路径返回 nil 接口（typed-nil 防护注释同既有 Load 方法）。
//
// Task 07：metadata 仅含 LoadedAtMs/ValidRange——pointCount 不暴露（兼容约定值
// 169/52 不应伪装为 loader 真值）。
func TestLoaderLoadSevenHolePRB(t *testing.T) {
	dir := sevenHoleFixtureDir(t)
	inner, outer := sevenHolePaths(dir)
	interp, metadata, err := NewLoader().LoadSevenHolePRB(inner, outer)
	if err != nil {
		t.Fatalf("Loader.LoadSevenHolePRB: %v", err)
	}
	if interp == nil || !interp.IsLoaded() {
		t.Fatal("expected loaded interpolator from Loader")
	}
	if metadata == nil {
		t.Fatal("expected non-nil SevenHoleLoadMetadata")
	}
	if metadata.LoadedAtMs <= 0 {
		t.Errorf("LoadedAtMs = %d, want positive timestamp", metadata.LoadedAtMs)
	}
	// ValidRange 应与 interpolator 自报一致（loader 真实可知字段）
	if got := metadata.ValidRange; got.AlphaMin > got.AlphaMax {
		t.Errorf("ValidRange invalid: alphaMin=%v > alphaMax=%v", got.AlphaMin, got.AlphaMax)
	}

	badOuter := outer
	badOuter[5] = filepath.Join(dir, "missing-6.prb")
	bad, badMeta, err := NewLoader().LoadSevenHolePRB(inner, badOuter)
	if err == nil {
		t.Fatal("expected error for missing sector file")
	}
	if bad != nil {
		t.Error("failure must return nil interface (typed-nil guard)")
	}
	if badMeta != nil {
		t.Error("failure must return nil metadata")
	}
}

func TestBuildSevenHoleMetadataRejectsUnsupportedInterpolator(t *testing.T) {
	metadata, err := buildSevenHoleMetadata(unsupportedSevenHoleInterpolator{})
	if err == nil {
		t.Fatal("expected unsupported interpolator error")
	}
	if metadata != nil {
		t.Fatal("failure must return nil metadata")
	}
}
