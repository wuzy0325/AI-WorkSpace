package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/traversal"
)

func TestTraversalCsvWriterWritesRowsAndReportsOutputPath(t *testing.T) {
	dir := t.TempDir()
	writer := NewTraversalCsvWriter()
	cfg := traversal.Config{
		TaskID:       "trav-1",
		Channels:     []int{3, 1},
		SavePath:     dir,
		SaveFileName: "run",
		ChannelLabels: map[int]string{
			1: "P1",
			3: "P3",
		},
	}

	if err := writer.InitializeTraversal(cfg); err != nil {
		t.Fatalf("InitializeTraversal returned error: %v", err)
	}
	if got, want := writer.OutputPath(), filepath.Join(dir, "run.csv"); got != want {
		t.Fatalf("OutputPath = %q, want %q", got, want)
	}
	if err := writer.WriteTraversalPoint(traversal.PointResult{
		PointIndex:       0,
		Point:            traversal.Point{X: 1, Y: 2},
		Timestamp:        time.Date(2026, 6, 24, 10, 30, 0, 0, time.Local).UnixMilli(),
		StartedAt:        time.Date(2026, 6, 24, 10, 29, 58, 0, time.Local).UnixMilli(),
		CompletedAt:      time.Date(2026, 6, 24, 10, 30, 0, 0, time.Local).UnixMilli(),
		Values:           map[int]float64{1: 11.5, 3: 33.25},
		SampleCount:      4,
		DwellTimeElapsed: 1000,
	}); err != nil {
		t.Fatalf("WriteTraversalPoint returned error: %v", err)
	}
	if err := writer.FinalizeTraversal(); err != nil {
		t.Fatalf("FinalizeTraversal returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "run.csv"))
	if err != nil {
		t.Fatalf("read traversal csv: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "PointId,Timestamp,X,Y,Z,U,P1,P3,Alpha,Beta,Pt,Ps,Mach,CalcStatus,SampleCount,DwellMs,StartedAt,CompletedAt") {
		t.Fatalf("expected traversal CSV header, got %q", text)
	}
	// Calculated 未填(calc==nil):Alpha~Mach + CalcStatus 共 6 列写空,与旧行为一致
	if !strings.Contains(text, "1,2026-06-24 10:30:00,1.000000,2.000000,0.000000,0.000000,11.500000,33.250000,,,,,,,4,1000,2026-06-24 10:29:58,2026-06-24 10:30:00") {
		t.Fatalf("expected traversal CSV row, got %q", text)
	}
}

func TestTraversalCsvWriterReturnsFlushError(t *testing.T) {
	writer := NewTraversalCsvWriter()
	if err := writer.InitializeTraversal(traversal.Config{TaskID: "trav-1", SavePath: t.TempDir(), SaveFileName: "run"}); err != nil {
		t.Fatalf("InitializeTraversal returned error: %v", err)
	}

	writer.mu.Lock()
	if err := writer.file.Close(); err != nil {
		writer.mu.Unlock()
		t.Fatalf("close backing file: %v", err)
	}
	writer.mu.Unlock()

	err := writer.WriteTraversalPoint(traversal.PointResult{Point: traversal.Point{X: 1, Y: 2}})
	if err == nil {
		t.Fatal("expected flush error, got nil")
	}
	if !strings.Contains(err.Error(), "刷新遍历 CSV 行失败") {
		t.Fatalf("expected flush error context, got %v", err)
	}
}

// TestBuildLabelEntriesSevenHole 七孔 9 标签经优先级表排序为 P1..P7,Patm,Tatm，
// 其余未知标签通道按通道索引升序追加（spec §5.5）。
func TestBuildLabelEntriesSevenHole(t *testing.T) {
	channels := []int{17, 6, 20, 1, 16, 3, 5, 2, 4, 0}
	labels := map[int]string{
		0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "P6", 6: "P7",
		16: "Patm", 17: "Tatm",
	}
	entries := buildLabelEntries(channels, labels)
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Label)
	}
	want := []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7", "Patm", "Tatm", "CH20"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestBuildLabelEntriesFiveHoleUnchanged 五孔标签排序不受新增 P6/P7 优先级影响：
// 输出与既有行为逐字节一致。
func TestBuildLabelEntriesFiveHoleUnchanged(t *testing.T) {
	channels := []int{17, 16, 4, 3, 2, 1, 0}
	labels := map[int]string{
		0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 16: "Patm", 17: "Tatm",
	}
	entries := buildLabelEntries(channels, labels)
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Label)
	}
	want := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestTraversalCsvWriterSevenHoleColumns 七孔 9 标签数据写 CSV：
// 原始压力列序为 P1..P7,Patm,Tatm，计算列位置与五孔一致。
func TestTraversalCsvWriterSevenHoleColumns(t *testing.T) {
	dir := t.TempDir()
	writer := NewTraversalCsvWriter()
	cfg := traversal.Config{
		TaskID:       "trav-7h",
		Channels:     []int{0, 1, 2, 3, 4, 5, 6, 16, 17},
		SavePath:     dir,
		SaveFileName: "run7",
		ChannelLabels: map[int]string{
			0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "P6", 6: "P7",
			16: "Patm", 17: "Tatm",
		},
	}
	if err := writer.InitializeTraversal(cfg); err != nil {
		t.Fatalf("InitializeTraversal: %v", err)
	}
	if err := writer.WriteTraversalPoint(traversal.PointResult{
		PointIndex: 0,
		Point:      traversal.Point{X: 1, Y: 2},
		Timestamp:  time.Date(2026, 7, 18, 10, 0, 0, 0, time.Local).UnixMilli(),
		Values: map[int]float64{
			0: 100, 1: 200, 2: 300, 3: 400, 4: 500, 5: 600, 6: 700,
			16: 101325, 17: 20,
		},
		SampleCount:      4,
		DwellTimeElapsed: 1000,
	}); err != nil {
		t.Fatalf("WriteTraversalPoint: %v", err)
	}
	if err := writer.FinalizeTraversal(); err != nil {
		t.Fatalf("FinalizeTraversal: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "run7.csv"))
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "P1,P2,P3,P4,P5,P6,P7,Patm,Tatm,Alpha,Beta,Pt,Ps,Mach,CalcStatus,SampleCount,DwellMs") {
		t.Fatalf("expected seven-hole column order P1..P7,Patm,Tatm + computed columns, got %q", text)
	}
	if !strings.Contains(text, "100.000000,200.000000,300.000000,400.000000,500.000000,600.000000,700.000000,101325.000000,20.000000") {
		t.Fatalf("expected nine raw-pressure values in column order, got %q", text)
	}
}
