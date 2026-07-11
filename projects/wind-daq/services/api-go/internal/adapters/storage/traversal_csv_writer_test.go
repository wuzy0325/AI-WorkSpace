package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
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
	if !strings.Contains(text, "PointId,Timestamp,X,Y,Z,U,P1,P3,Alpha,Beta,Pt,Ps,Mach,SampleCount,DwellMs") {
		t.Fatalf("expected traversal CSV header, got %q", text)
	}
	if !strings.Contains(text, "1,2026-06-24 10:30:00,1.000000,2.000000,0.000000,0.000000,11.500000,33.250000,,,,,,4,1000") {
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
