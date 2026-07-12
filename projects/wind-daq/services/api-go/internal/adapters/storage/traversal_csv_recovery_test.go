package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

func TestTraversalCsvWriterResumeReusesAndTruncatesOriginalFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "run.csv")
	writer := NewTraversalCsvWriter()
	if err := writer.Open(ctx, ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputCreate, Path: path}); err != nil {
		t.Fatalf("Open create returned error: %v", err)
	}
	for seq := uint64(1); seq <= 2; seq++ {
		if _, err := writer.Append(ctx, traversal.PointResult{TaskID: "task-1", CommitSeq: seq, PointIndex: int(seq - 1), PointStatus: traversal.PointStatusCompleted}); err != nil {
			t.Fatalf("Append seq %d returned error: %v", seq, err)
		}
	}
	if err := writer.Sync(ctx); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if err := writer.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	resumed := NewTraversalCsvWriter()
	if err := resumed.Open(ctx, ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputResume, Path: path, CommittedSeq: 1}); err != nil {
		t.Fatalf("Open resume returned error: %v", err)
	}
	state, err := resumed.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if state.Rows != 1 || state.CommitSeq != 1 || !state.TailValid {
		t.Fatalf("Inspect = %+v, want one committed valid row", state)
	}
	if err := resumed.Close(ctx); err != nil {
		t.Fatalf("Close resumed writer returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "run-2.csv")); !os.IsNotExist(err) {
		t.Fatalf("resume unexpectedly created run-2.csv: %v", err)
	}
}

func TestTraversalCsvWriterRejectsUninitializedAppend(t *testing.T) {
	_, err := NewTraversalCsvWriter().Append(context.Background(), traversal.PointResult{})
	if err == nil {
		t.Fatal("expected uninitialized append error")
	}
}

func TestTraversalCsvWriterResumeRejectsHeaderMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.csv")
	if err := os.WriteFile(path, append(append([]byte{}, utf8BOM...), []byte("bad,header\n")...), 0o644); err != nil {
		t.Fatalf("write malformed CSV: %v", err)
	}
	err := NewTraversalCsvWriter().Open(context.Background(), ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputResume, Path: path})
	if err == nil {
		t.Fatal("expected header mismatch error")
	}
}
