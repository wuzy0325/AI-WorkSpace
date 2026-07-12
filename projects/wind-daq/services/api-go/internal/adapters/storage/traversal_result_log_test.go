package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

func TestTraversalResultLogWritesAndReadsCompletePreparedRecords(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "run.results.jsonl")
	log := NewTraversalResultLog()
	if err := log.Open(ctx, ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputCreate, Path: path}); err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	want := traversal.PointResult{TaskID: "task-1", CommitSeq: 1, PointIndex: 0, PointStatus: traversal.PointStatusCompleted, ValidationWarnings: []string{"warning"}, CSVRowHash: "row-hash"}
	if err := log.AppendPrepared(ctx, want); err != nil {
		t.Fatalf("AppendPrepared returned error: %v", err)
	}
	if err := log.Sync(ctx); err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	got, err := log.ReadCommitted(ctx, 1)
	if err != nil {
		t.Fatalf("ReadCommitted returned error: %v", err)
	}
	if len(got) != 1 || got[0].TaskID != want.TaskID || got[0].CommitSeq != want.CommitSeq || got[0].CSVRowHash != want.CSVRowHash || len(got[0].ValidationWarnings) != 1 {
		t.Fatalf("ReadCommitted = %+v, want complete result", got)
	}
	if err := log.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestTraversalResultLogAllowsMalformedUncommittedTail(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "run.results.jsonl")
	log := NewTraversalResultLog()
	if err := log.Open(ctx, ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputCreate, Path: path}); err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := log.AppendPrepared(ctx, traversal.PointResult{TaskID: "task-1", CommitSeq: 1}); err != nil {
		t.Fatalf("AppendPrepared returned error: %v", err)
	}
	if err := log.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open result log tail: %v", err)
	}
	if _, err := file.WriteString("{broken"); err != nil {
		t.Fatalf("write malformed tail: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close malformed tail: %v", err)
	}

	resumed := NewTraversalResultLog()
	if err := resumed.Open(ctx, ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputResume, Path: path, CommittedSeq: 1}); err != nil {
		t.Fatalf("Open resume returned error: %v", err)
	}
	if err := resumed.ValidateTail(ctx, 1); err != nil {
		t.Fatalf("ValidateTail returned error: %v", err)
	}
	if err := resumed.TruncateAfter(ctx, 1); err != nil {
		t.Fatalf("TruncateAfter returned error: %v", err)
	}
	if err := resumed.Close(ctx); err != nil {
		t.Fatalf("Close resumed log returned error: %v", err)
	}
}

func TestTraversalResultLogRejectsCommittedGapAndConflict(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "run.results.jsonl")
	content := "{\"version\":1,\"phase\":\"prepared\",\"result\":{\"taskId\":\"task-1\",\"commitSeq\":1}}\n" +
		"{\"version\":1,\"phase\":\"prepared\",\"result\":{\"taskId\":\"task-1\",\"commitSeq\":3}}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result log: %v", err)
	}
	log := NewTraversalResultLog()
	if err := log.Open(ctx, ports.TraversalOutputSession{TaskID: "task-1", Mode: ports.TraversalOutputResume, Path: path, CommittedSeq: 3}); err != nil {
		t.Fatalf("Open resume returned error: %v", err)
	}
	if _, err := log.ReadCommitted(ctx, 3); err == nil {
		t.Fatal("expected committed sequence gap error")
	}
	if err := log.Close(ctx); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}
