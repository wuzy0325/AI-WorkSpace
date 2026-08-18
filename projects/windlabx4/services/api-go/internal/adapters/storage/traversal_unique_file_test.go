package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"windlabx4/services/api-go/internal/ports"
)

// 回归：同一天、同名重跑（目标文件已存在）时，v2 Open 路径应自动追加
// -2/-3 后缀另存，而非报错拒绝启动，也绝不覆盖历史数据。
func TestTraversalCsvWriterOpenAutoRenamesOnCollision(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "run.csv")

	// 预置同名历史文件，模拟"同天重跑"
	if err := os.WriteFile(base, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	writer := NewTraversalCsvWriter()
	session := ports.TraversalOutputSession{
		TaskID:        "trav-1",
		Mode:          ports.TraversalOutputCreate,
		Path:          base,
		Channels:      []int{1, 3},
		ChannelLabels: map[int]string{1: "P1", 3: "P3"},
	}
	if err := writer.Open(context.Background(), session); err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	state, err := writer.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	want := filepath.Join(dir, "run-2.csv")
	if state.Path != want {
		t.Fatalf("Open wrote to %q, want %q", state.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected unique file %q to exist: %v", want, err)
	}
	// 原文件内容必须保持不被覆盖
	if data, _ := os.ReadFile(base); string(data) != "stale" {
		t.Fatalf("original file was overwritten: %q", data)
	}
	_ = writer.Close(context.Background())

	// 再次重跑：应编到 -3
	writer2 := NewTraversalCsvWriter()
	if err := writer2.Open(context.Background(), session); err != nil {
		t.Fatalf("second Open returned error: %v", err)
	}
	state2, _ := writer2.Inspect(context.Background())
	if state2.Path != filepath.Join(dir, "run-3.csv") {
		t.Fatalf("second Open wrote to %q, want run-3.csv", state2.Path)
	}
	_ = writer2.Close(context.Background())
}

// 回归：结果日志与 CSV 同步，Create 模式撞名时同样自动 -2/-3 另存。
func TestTraversalResultLogOpenAutoRenamesOnCollision(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "run.results.jsonl")

	if err := os.WriteFile(base, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	log := NewTraversalResultLog()
	session := ports.TraversalOutputSession{
		TaskID: "trav-1",
		Mode:   ports.TraversalOutputCreate,
		Path:   base,
	}
	if err := log.Open(context.Background(), session); err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "run.results-2.jsonl")); err != nil {
		t.Fatalf("expected unique result log file to exist: %v", err)
	}
	if data, _ := os.ReadFile(base); string(data) != "stale" {
		t.Fatalf("original result log was overwritten: %q", data)
	}
	_ = log.Close(context.Background())
}
