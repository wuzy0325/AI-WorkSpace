package recording

import (
	"os"
	"path/filepath"
	"testing"

	"daq-t1603/core"
)

func TestStartStop(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	if err := rec.Start(dir, "test"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	session := rec.Status()
	if session.Status != core.RecordingActive {
		t.Fatalf("expected active recording")
	}

	if err := rec.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	session = rec.Status()
	if session.Status != core.RecordingIdle {
		t.Fatalf("expected idle after stop")
	}
}

func TestDoubleStart(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	if err := rec.Start(dir, "test"); err != nil {
		t.Fatal(err)
	}
	if err := rec.Start(dir, "test2"); err == nil {
		t.Fatalf("expected error on double start")
	}
	rec.Stop()
}

func TestWriteAndVerifyFile(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()

	if err := rec.Start(dir, "verify"); err != nil {
		t.Fatal(err)
	}

	snapshot := core.TemperatureSnapshot{
		DeviceID:  "dev1",
		Timestamp: 1700000000000,
		Values:    make([]float64, 16),
		Unit:      "°C",
	}
	for i := range snapshot.Values {
		snapshot.Values[i] = float64(i) * 10.5
	}

	if err := rec.Write(snapshot); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := rec.Stop(); err != nil {
		t.Fatal(err)
	}

	session := rec.Status()
	if session.SnapshotCount != 1 {
		t.Fatalf("expected 1 snapshot, got %d", session.SnapshotCount)
	}

	// Verify file content
	files, err := filepath.Glob(filepath.Join(dir, "verify_*.csv"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no CSV file found")
	}
}

func TestWriteToClosedRecorder(t *testing.T) {
	rec := NewCSVRecorder()
	snapshot := core.TemperatureSnapshot{DeviceID: "d1", Timestamp: 1, Values: make([]float64, 16)}
	// Write without starting should not error (no-op)
	if err := rec.Write(snapshot); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStopIdempotent(t *testing.T) {
	rec := NewCSVRecorder()
	if err := rec.Stop(); err != nil {
		t.Fatalf("Stop on idle should not error: %v", err)
	}
}

func TestWriteAfterStop(t *testing.T) {
	dir := t.TempDir()
	rec := NewCSVRecorder()
	_ = rec.Start(dir, "poststop")
	_ = rec.Stop()

	snapshot := core.TemperatureSnapshot{DeviceID: "d1", Timestamp: 1, Values: make([]float64, 16)}
	if err := rec.Write(snapshot); err != nil {
		t.Fatalf("Write after stop should be no-op, got %v", err)
	}
}

func TestCreateDirIfNotExist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "subdir")
	rec := NewCSVRecorder()
	if err := rec.Start(dir, "nested"); err != nil {
		t.Fatalf("Start with new nested dir: %v", err)
	}
	rec.Stop()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("expected directory to be created")
	}
}
