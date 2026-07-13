package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTraversalActiveIndexRegistersFindsUpdatesAndUnregistersTasks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	index := NewTraversalActiveIndex(filepath.Join(dir, "active.json"), dir)
	first := filepath.Join(dir, "task-1.checkpoint.json")
	updated := filepath.Join(dir, "task-1-new.checkpoint.json")
	second := filepath.Join(dir, "task-2.checkpoint.json")
	if err := index.Register(ctx, "task-1", first); err != nil {
		t.Fatalf("Register first task returned error: %v", err)
	}
	if err := index.Register(ctx, "task-2", second); err != nil {
		t.Fatalf("Register second task returned error: %v", err)
	}
	if err := index.Register(ctx, "task-1", updated); err != nil {
		t.Fatalf("Update first task returned error: %v", err)
	}
	ref, found, err := index.Find(ctx, "task-1")
	if err != nil || !found || ref.Path != updated {
		t.Fatalf("Find task-1 = %+v, %v, %v", ref, found, err)
	}
	if err := index.Unregister(ctx, "task-1"); err != nil {
		t.Fatalf("Unregister returned error: %v", err)
	}
	if _, found, err := index.Find(ctx, "task-1"); err != nil || found {
		t.Fatalf("Find unregistered task = found %v, err %v", found, err)
	}
	if _, found, err := index.Find(ctx, "task-2"); err != nil || !found {
		t.Fatalf("second task was not preserved: found %v, err %v", found, err)
	}
}

func TestTraversalActiveIndexAcceptsCheckpointInExternalOutputDirectory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	outputDir := t.TempDir()
	path := filepath.Join(dir, "active.json")
	index := NewTraversalActiveIndex(path, dir)
	checkpointPath := filepath.Join(outputDir, "run.checkpoint.json")
	if err := index.Register(ctx, "task-1", checkpointPath); err != nil {
		t.Fatalf("Register external checkpoint returned error: %v", err)
	}
	ref, found, err := index.Find(ctx, "task-1")
	if err != nil || !found || ref.Path != checkpointPath {
		t.Fatalf("Find external checkpoint = %+v, %v, %v", ref, found, err)
	}
}

func TestTraversalActiveIndexRejectsInvalidCheckpointPathAndCorruption(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "active.json")
	index := NewTraversalActiveIndex(path, dir)
	if err := index.Register(ctx, "task-1", filepath.Join(dir, "outside.json")); err == nil {
		t.Fatal("expected invalid checkpoint path error")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}
	if _, _, err := index.Find(ctx, "task-1"); err == nil {
		t.Fatal("expected corrupt index error")
	}
}
