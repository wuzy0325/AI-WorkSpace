package usecase

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/traversal"
)

// Task 10（usecase 侧）：managed session 写 v3 checkpoint、legacy 写 v2、
// managed Start 输出文件名带 probe 前缀。

// TestTraversal_ManagedCommitPointV2_WritesV3Checkpoint 验证 managed session 的
// 阶段3 checkpoint 落盘为 v3（ProbeID + BoundControllerIDs），legacy 保持 v2。
func TestTraversal_ManagedCommitPointV2_WritesV3Checkpoint(t *testing.T) {
	build := func(managed bool, savedPath *string) (*TraversalManager, *TraversalRunSession) {
		mgr, _, _, _ := newCheckpointTestManager()
		session := newTraversalRunSession(context.Background(), "task-v3", traversal.TraversalRunSnapshot{
			Config:             traversal.Config{TaskID: "task-v3", DeviceID: "dev-1", Channels: []int{0}},
			TotalPoints:        3,
			CSVPath:            filepath.Join(t.TempDir(), "run.csv"),
			BoundControllerIDs: []string{"ctrl-a"},
		})
		if managed {
			opts := managedTestOpts()
			opts.CheckpointSavedCallback = func(path string) { *savedPath = path }
			session.managedOpts = &opts
		}
		mgr.mu.Lock()
		mgr.session = session
		mgr.config = traversal.Config{TaskID: "task-v3"}
		mgr.status = traversal.Status{TaskID: "task-v3", State: traversal.StateRunning}
		mgr.mu.Unlock()
		return mgr, session
	}

	readCheckpoint := func(t *testing.T, mgr *TraversalManager, csvPath string) traversal.Checkpoint {
		t.Helper()
		data, err := mgr.checkpointStore.Read(traversal.ResolveCheckpointPathFromCSV(csvPath))
		if err != nil {
			t.Fatalf("读取 checkpoint: %v", err)
		}
		var cp traversal.Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			t.Fatalf("解析 checkpoint: %v", err)
		}
		return cp
	}

	// managed：v3 + ProbeID + BoundControllerIDs + 回调通知
	var savedPath string
	mgr, session := build(true, &savedPath)
	result := &traversal.PointResult{TaskID: "task-v3", CommitSeq: 1, PointIndex: 0, Values: map[int]float64{0: 1}}
	if err := mgr.commitPointV2("task-v3", result); err != nil {
		t.Fatalf("commitPointV2: %v", err)
	}
	cp := readCheckpoint(t, mgr, session.snapshot.CSVPath)
	if cp.Version != traversal.DualCheckpointVersion {
		t.Fatalf("managed 应写 v3, got v%d", cp.Version)
	}
	if cp.ProbeID != "probe1" {
		t.Fatalf("v3 应含 ProbeID, got %q", cp.ProbeID)
	}
	if len(cp.Snapshot.BoundControllerIDs) != 1 || cp.Snapshot.BoundControllerIDs[0] != "ctrl-a" {
		t.Fatalf("v3 应含 BoundControllerIDs, got %v", cp.Snapshot.BoundControllerIDs)
	}
	if savedPath == "" || !strings.HasSuffix(savedPath, ".checkpoint.json") {
		t.Fatalf("managed 应触发 CheckpointSavedCallback: %q", savedPath)
	}

	// legacy：v2、无 ProbeID、无回调
	var savedPath2 string
	mgr2, session2 := build(false, &savedPath2)
	if err := mgr2.commitPointV2("task-v3", result); err != nil {
		t.Fatalf("legacy commitPointV2: %v", err)
	}
	cp2 := readCheckpoint(t, mgr2, session2.snapshot.CSVPath)
	if cp2.Version != traversal.CheckpointVersion {
		t.Fatalf("legacy 应继续写 v2, got v%d", cp2.Version)
	}
	if cp2.ProbeID != "" {
		t.Fatalf("legacy 不得写 ProbeID, got %q", cp2.ProbeID)
	}
	if savedPath2 != "" {
		t.Fatal("legacy 不得触发 CheckpointSavedCallback")
	}
}

// TestTraversal_ManagedStart_AppliesProbePrefix 验证 managed Start 的输出路径带 probe 前缀，
// 结果日志/checkpoint 路径同源派生；legacy 无前缀。
func TestTraversal_ManagedStart_AppliesProbePrefix(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	opts := managedTestOpts()
	opts.CompletionCallback = func(SessionToken) {}
	config := makeManagedTestConfig("managed-prefix-task")
	config.SavePath = t.TempDir()
	config.SaveFileName = "traversal-run.csv"

	if err := mgr.StartManaged(config, opts); err != nil {
		t.Fatalf("StartManaged: %v", err)
	}
	mgr.mu.RLock()
	session := mgr.session
	mgr.mu.RUnlock()
	if session == nil {
		t.Fatal("session 应已创建")
	}
	base := filepath.Base(session.snapshot.CSVPath)
	if !strings.HasPrefix(base, "probe1-") {
		t.Fatalf("managed 输出文件名应以 probe ID 为前缀: %q", base)
	}
	if !strings.HasPrefix(filepath.Base(session.snapshot.ResultLogPath), "probe1-") {
		t.Fatalf("结果日志应同源带前缀: %q", session.snapshot.ResultLogPath)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// legacy：无前缀
	mgr2, _, _, _ := newCheckpointTestManager()
	if err := mgr2.Start(config); err != nil {
		t.Fatalf("legacy Start: %v", err)
	}
	mgr2.mu.RLock()
	legacySession := mgr2.session
	mgr2.mu.RUnlock()
	if strings.HasPrefix(filepath.Base(legacySession.snapshot.CSVPath), "probe1-") {
		t.Fatalf("legacy 输出不得加 probe 前缀: %q", legacySession.snapshot.CSVPath)
	}
	mgr2.mu.Lock()
	mgr2.status.State = traversal.StateStopped
	mgr2.mu.Unlock()
	mgr2.RunTraversalLoop()
}
