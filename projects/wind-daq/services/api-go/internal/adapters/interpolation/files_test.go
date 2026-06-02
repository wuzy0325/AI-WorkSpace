package interpolation

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
