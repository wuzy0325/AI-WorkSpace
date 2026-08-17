package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/ports"
)

// Task 10：dual checkpoint v3 codec 与 probe 前缀。
//
// 覆盖：v3 round-trip 保留全部可靠性字段 / legacy 不读 v3 / dual 不读 v1/v2
// （不自动迁移）/ dual Save 拒绝非 v3 / probe 前缀与 -2/-3 防覆盖。

// makeV3Checkpoint 构造含全部可靠性字段的 v3 checkpoint。
func makeV3Checkpoint() traversal.Checkpoint {
	return traversal.Checkpoint{
		Version:         traversal.DualCheckpointVersion,
		TaskID:          "probe1-task-1",
		State:           traversal.StateStopped,
		CompletedPoints: 2,
		TotalPoints:     5,
		SavePath:        "D:/out/probe1-run.csv",
		CreatedAt:       1785000000000,
		ProbeID:         "probe1",
		Snapshot: traversal.TraversalRunSnapshot{
			Config:             traversal.Config{TaskID: "probe1-task-1", DeviceID: "dev-1", Channels: []int{0, 1}},
			TotalPoints:        5,
			CommittedPoints:    2,
			CommitSeq:          2,
			CSVPath:            "D:/out/probe1-run.csv",
			ResultLogPath:      "D:/out/.traversal/probe1-run.results.jsonl",
			CSVHeaderHash:      "header-hash-abc",
			LastCommitHash:     "commit-hash-def",
			BoundControllerIDs: []string{"ctrl-a"},
		},
	}
}

func TestFileCheckpointPort_V3RoundTripPreservesReliabilityFields(t *testing.T) {
	store := NewFileCheckpointStore()
	base := filepath.Join(t.TempDir(), "probe1-run.csv")
	port := NewDualCheckpointPort(store, base)
	ctx := context.Background()
	want := makeV3Checkpoint()

	if err := port.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// 磁盘 envelope：version=3，含 probeId 与 boundControllerIds，不含旧模型 Points/LastIndex
	data, err := os.ReadFile(traversal.ResolveCheckpointPathFromCSV(base))
	if err != nil {
		t.Fatalf("读取落盘文件: %v", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.Version != traversal.DualCheckpointVersion {
		t.Fatalf("落盘版本应为 3: version=%d err=%v", header.Version, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析落盘 JSON: %v", err)
	}
	if raw["probeId"] != "probe1" {
		t.Fatalf("落盘应含 probeId: %v", raw["probeId"])
	}
	if _, ok := raw["points"]; ok {
		t.Fatal("v3 不得引入旧模型 Points 字段")
	}
	got, err := port.Load(ctx, want.TaskID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 全部可靠性字段逐一断言
	if got.Version != want.Version || got.TaskID != want.TaskID || got.ProbeID != want.ProbeID ||
		got.CompletedPoints != want.CompletedPoints || got.TotalPoints != want.TotalPoints ||
		got.SavePath != want.SavePath || got.CreatedAt != want.CreatedAt {
		t.Fatalf("round-trip 顶层字段漂移: %+v", got)
	}
	gs, ws := got.Snapshot, want.Snapshot
	if gs.CommitSeq != ws.CommitSeq || gs.CommittedPoints != ws.CommittedPoints ||
		gs.CSVPath != ws.CSVPath || gs.ResultLogPath != ws.ResultLogPath ||
		gs.CSVHeaderHash != ws.CSVHeaderHash || gs.LastCommitHash != ws.LastCommitHash {
		t.Fatalf("round-trip snapshot 可靠性字段漂移: %+v", gs)
	}
	if len(gs.BoundControllerIDs) != 1 || gs.BoundControllerIDs[0] != "ctrl-a" {
		t.Fatalf("round-trip BoundControllerIDs 漂移: %v", gs.BoundControllerIDs)
	}
}

func TestFileCheckpointPort_DualRejectsLegacyVersions(t *testing.T) {
	store := NewFileCheckpointStore()
	base := filepath.Join(t.TempDir(), "run.csv")
	ctx := context.Background()

	// legacy 端口写入 v2 文件
	legacy := NewFileCheckpointPort(store, base)
	cp := makeV3Checkpoint()
	cp.Version = traversal.CheckpointVersion
	cp.ProbeID = ""
	if err := legacy.Save(ctx, cp); err != nil {
		t.Fatalf("legacy Save: %v", err)
	}
	// dual 路径读取 v2：拒绝，不自动迁移
	dual := NewDualCheckpointPort(store, base)
	if _, err := dual.Load(ctx, cp.TaskID); !errors.Is(err, ports.ErrCheckpointVersionMismatch) {
		t.Fatalf("dual 读 v2 应返回 checkpoint_version_mismatch, got %v", err)
	}
	// v1（伪造 version=1）同样拒绝
	data, _ := json.Marshal(map[string]any{"version": 1, "taskId": "old"})
	if err := store.Write(traversal.ResolveCheckpointPathFromCSV(base), data); err != nil {
		t.Fatalf("写入 v1 文件: %v", err)
	}
	if _, err := dual.Load(ctx, "old"); !errors.Is(err, ports.ErrCheckpointVersionMismatch) {
		t.Fatalf("dual 读 v1 应返回 checkpoint_version_mismatch, got %v", err)
	}
}

func TestFileCheckpointPort_LegacyRejectsV3(t *testing.T) {
	store := NewFileCheckpointStore()
	base := filepath.Join(t.TempDir(), "run.csv")
	ctx := context.Background()

	dual := NewDualCheckpointPort(store, base)
	if err := dual.Save(ctx, makeV3Checkpoint()); err != nil {
		t.Fatalf("dual Save: %v", err)
	}
	legacy := NewFileCheckpointPort(store, base)
	if _, err := legacy.Load(ctx, "probe1-task-1"); !errors.Is(err, ports.ErrCheckpointVersionMismatch) {
		t.Fatalf("legacy 读 v3 应返回 checkpoint_version_mismatch, got %v", err)
	}
}

func TestFileCheckpointPort_DualSaveRejectsNonV3(t *testing.T) {
	store := NewFileCheckpointStore()
	base := filepath.Join(t.TempDir(), "run.csv")
	dual := NewDualCheckpointPort(store, base)
	cp := makeV3Checkpoint()
	cp.Version = traversal.CheckpointVersion
	if err := dual.Save(context.Background(), cp); !errors.Is(err, ports.ErrCheckpointVersionMismatch) {
		t.Fatalf("dual Save v2 应返回 checkpoint_version_mismatch, got %v", err)
	}
}

func TestDualCheckpointPortFactory_CreatesDualCodecPort(t *testing.T) {
	factory := NewDualCheckpointPortFactory(NewFileCheckpointStore())
	port, err := factory.Create(context.Background(), filepath.Join(t.TempDir(), "run.csv"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cp := makeV3Checkpoint()
	cp.Version = traversal.CheckpointVersion
	if err := port.Save(context.Background(), cp); !errors.Is(err, ports.ErrCheckpointVersionMismatch) {
		t.Fatalf("工厂创建的端口应为 dual codec, got %v", err)
	}
}

func TestTraversalCsvWriter_ProbePrefix(t *testing.T) {
	// 纯路径派生：前缀加在文件名 stem 前，目录/扩展名不变，幂等
	in := filepath.Join("D:", "out", "traversal_t1.csv")
	want := filepath.Join("D:", "out", "probe1-traversal_t1.csv")
	if got := traversal.ResolveProbePrefixedPath(in, "probe1"); got != want {
		t.Fatalf("probe 前缀派生: got %q want %q", got, want)
	}
	if got := traversal.ResolveProbePrefixedPath(want, "probe1"); got != want {
		t.Fatal("前缀派生应幂等（已带前缀不重复添加）")
	}
	if got := traversal.ResolveProbePrefixedPath(in, ""); got != in {
		t.Fatal("空 probeID 应原样返回")
	}

	// writer 集成：带前缀的文件名落盘，撞名走既有 -2 机制且前缀保留
	dir := t.TempDir()
	base := traversal.ResolveProbePrefixedPath(filepath.Join(dir, "traversal_t1.csv"), "probe1")
	if err := os.WriteFile(base, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	writer := NewTraversalCsvWriter()
	session := ports.TraversalOutputSession{
		TaskID:        "probe1-task-1",
		Mode:          ports.TraversalOutputCreate,
		Path:          base,
		Channels:      []int{0},
		ChannelLabels: map[int]string{0: "P1"},
	}
	if err := writer.Open(context.Background(), session); err != nil {
		t.Fatalf("Open: %v", err)
	}
	actual := writer.OutputPath()
	if filepath.Base(actual) != "probe1-traversal_t1-2.csv" {
		t.Fatalf("撞名后应保留 probe 前缀并追加 -2: %q", actual)
	}
	// 实际最终路径以 OutputPath() 为准；原文件不被覆盖
	if data, _ := os.ReadFile(base); string(data) != "stale" {
		t.Fatal("原文件被覆盖")
	}
	if !strings.HasPrefix(filepath.Base(actual), "probe1-") {
		t.Fatal("实际输出文件名应以 probe ID 为前缀")
	}
	_ = writer.Close(context.Background())
}

// TestFileCheckpointPort_LegacyV2RoundTripUnchanged legacy v2 读写回归：
// 版本路由改造后 v1/v2 解码与写入行为与改造前一致。
func TestFileCheckpointPort_LegacyV2RoundTripUnchanged(t *testing.T) {
	store := NewFileCheckpointStore()
	base := filepath.Join(t.TempDir(), "run.csv")
	port := NewFileCheckpointPort(store, base)
	ctx := context.Background()

	want := makeV3Checkpoint()
	want.Version = traversal.CheckpointVersion
	want.ProbeID = ""
	if err := port.Save(ctx, want); err != nil {
		t.Fatalf("legacy Save: %v", err)
	}
	got, err := port.Load(ctx, want.TaskID)
	if err != nil {
		t.Fatalf("legacy Load: %v", err)
	}
	if got.Version != traversal.CheckpointVersion || got.TaskID != want.TaskID ||
		got.Snapshot.CommitSeq != want.Snapshot.CommitSeq || got.Snapshot.CSVPath != want.Snapshot.CSVPath {
		t.Fatalf("legacy v2 round-trip 漂移: %+v", got)
	}
	// v1（无 Snapshot 可靠字段的旧模型）解码不报错（json 容忍缺失字段）
	v1 := []byte(`{"version":1,"taskId":"old-task","completedPoints":1,"totalPoints":3}`)
	if err := store.Write(traversal.ResolveCheckpointPathFromCSV(base), v1); err != nil {
		t.Fatalf("写入 v1: %v", err)
	}
	if _, err := port.Load(ctx, "old-task"); err != nil {
		t.Fatalf("legacy 读 v1 应保持兼容: %v", err)
	}
}
