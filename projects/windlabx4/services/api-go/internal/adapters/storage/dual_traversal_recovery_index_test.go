package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"windlabx4/services/api-go/internal/ports"
)

func newTestDualIndex(t *testing.T) (*DualTraversalRecoveryIndex, string) {
	t.Helper()
	dir := t.TempDir()
	return NewDualTraversalRecoveryIndex(filepath.Join(dir, DualTraversalRecoveryIndexFileName)), dir
}

func dualCheckpointPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name+".checkpoint.json")
}

func TestDualTraversalRecoveryIndex_RegisterFindUnregister(t *testing.T) {
	index, _ := newTestDualIndex(t)
	ctx := context.Background()
	cpPath := dualCheckpointPath(t, "probe1-traversal-task-1")

	if err := index.Register(ctx, "probe1", "task-1", cpPath); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ref, found, err := index.Find(ctx, "probe1")
	if err != nil || !found {
		t.Fatalf("Find: found=%v err=%v", found, err)
	}
	if ref.TaskID != "task-1" || ref.Path != cpPath {
		t.Fatalf("Find 返回不符: %+v", ref)
	}
	ids, err := index.ListProbeTaskIDs(ctx, "probe1")
	if err != nil || len(ids) != 1 || ids[0] != "task-1" {
		t.Fatalf("ListProbeTaskIDs: ids=%v err=%v", ids, err)
	}
	// 同 probe 同 taskID 重复登记为幂等更新，不视为第二个候选
	if err := index.Register(ctx, "probe1", "task-1", cpPath); err != nil {
		t.Fatalf("同 taskID 重复 Register 应幂等成功: %v", err)
	}
	if err := index.Unregister(ctx, "probe1", "task-1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if _, found, err := index.Find(ctx, "probe1"); err != nil || found {
		t.Fatalf("注销后 Find: found=%v err=%v", found, err)
	}
	// 映射不存在时注销幂等成功
	if err := index.Unregister(ctx, "probe1", "task-1"); err != nil {
		t.Fatalf("重复 Unregister 应幂等成功: %v", err)
	}
}

func TestDualTraversalRecoveryIndex_DuplicateRegistrationRejected(t *testing.T) {
	index, _ := newTestDualIndex(t)
	ctx := context.Background()

	if err := index.Register(ctx, "probe1", "task-1", dualCheckpointPath(t, "a")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := index.Register(ctx, "probe1", "task-2", dualCheckpointPath(t, "b"))
	if !errors.Is(err, ports.ErrRecoverableTaskExists) {
		t.Fatalf("第二个 taskID 应返回 ErrRecoverableTaskExists, got %v", err)
	}
	// 拒绝后原候选保持不变
	ref, found, err := index.Find(ctx, "probe1")
	if err != nil || !found || ref.TaskID != "task-1" {
		t.Fatalf("拒绝后候选应保持 task-1: %+v found=%v err=%v", ref, found, err)
	}
}

func TestDualTraversalRecoveryIndex_TaskIDAcrossProbesRejected(t *testing.T) {
	index, _ := newTestDualIndex(t)
	ctx := context.Background()

	if err := index.Register(ctx, "probe1", "task-1", dualCheckpointPath(t, "a")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := index.Register(ctx, "probe2", "task-1", dualCheckpointPath(t, "b"))
	if !errors.Is(err, ports.ErrTaskIDRegisteredToOtherProbe) {
		t.Fatalf("跨 probe 相同 taskID 应返回 ErrTaskIDRegisteredToOtherProbe, got %v", err)
	}
	if _, found, err := index.Find(ctx, "probe2"); err != nil || found {
		t.Fatalf("probe2 不应存在候选: found=%v err=%v", found, err)
	}
}

func TestDualTraversalRecoveryIndex_CrossProbeIsolation(t *testing.T) {
	index, _ := newTestDualIndex(t)
	ctx := context.Background()
	cp1 := dualCheckpointPath(t, "probe1-task")
	cp2 := dualCheckpointPath(t, "probe2-task")

	if err := index.Register(ctx, "probe1", "task-1", cp1); err != nil {
		t.Fatalf("Register probe1: %v", err)
	}
	if err := index.Register(ctx, "probe2", "task-2", cp2); err != nil {
		t.Fatalf("Register probe2: %v", err)
	}
	ref1, found1, _ := index.Find(ctx, "probe1")
	ref2, found2, _ := index.Find(ctx, "probe2")
	if !found1 || ref1.TaskID != "task-1" || ref1.Path != cp1 {
		t.Fatalf("probe1 候选不符: %+v found=%v", ref1, found1)
	}
	if !found2 || ref2.TaskID != "task-2" || ref2.Path != cp2 {
		t.Fatalf("probe2 候选不符: %+v found=%v", ref2, found2)
	}
	// 注销 probe1 不影响 probe2
	if err := index.Unregister(ctx, "probe1", "task-1"); err != nil {
		t.Fatalf("Unregister probe1: %v", err)
	}
	if _, found, err := index.Find(ctx, "probe1"); err != nil || found {
		t.Fatalf("probe1 应已注销: found=%v err=%v", found, err)
	}
	if _, found, err := index.Find(ctx, "probe2"); err != nil || !found {
		t.Fatalf("probe2 不应受影响: found=%v err=%v", found, err)
	}
	// 注销后同 probe 可登记新候选
	if err := index.Register(ctx, "probe1", "task-3", dualCheckpointPath(t, "probe1-task3")); err != nil {
		t.Fatalf("注销后重新 Register: %v", err)
	}
	// 错误 taskID 不能清除 probe2 的候选
	if err := index.Unregister(ctx, "probe2", "task-1"); err == nil {
		t.Fatal("taskID 不一致的 Unregister 应返回错误")
	}
	if _, found, err := index.Find(ctx, "probe2"); err != nil || !found {
		t.Fatalf("probe2 候选不应被错误 taskID 清除: found=%v err=%v", found, err)
	}
}

func TestDualTraversalRecoveryIndex_AtomicReplace(t *testing.T) {
	index, dir := newTestDualIndex(t)
	ctx := context.Background()

	if err := index.Register(ctx, "probe1", "task-1", dualCheckpointPath(t, "a")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := index.Register(ctx, "probe2", "task-2", dualCheckpointPath(t, "b")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := index.Unregister(ctx, "probe1", "task-1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	// 每次操作后磁盘文件都是完整有效的 v1 envelope（原子替换，无半截写）
	data, err := os.ReadFile(filepath.Join(dir, DualTraversalRecoveryIndexFileName))
	if err != nil {
		t.Fatalf("读取索引文件: %v", err)
	}
	var envelope dualTraversalRecoveryIndexFile
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("索引文件应为有效 JSON: %v", err)
	}
	if envelope.Version != DualTraversalRecoveryIndexVersion {
		t.Fatalf("envelope 版本应为 %d, got %d", DualTraversalRecoveryIndexVersion, envelope.Version)
	}
	if len(envelope.Probes) != 1 || len(envelope.Probes["probe2"]) != 1 {
		t.Fatalf("envelope 内容不符: %+v", envelope.Probes)
	}
	// 原子替换不残留临时文件
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("原子替换残留临时文件: %s", entry.Name())
		}
	}
}

func TestDualTraversalRecoveryIndex_MissingFileReturnsEmpty(t *testing.T) {
	index, _ := newTestDualIndex(t)
	ctx := context.Background()

	if _, found, err := index.Find(ctx, "probe1"); err != nil || found {
		t.Fatalf("文件不存在时 Find 应返回空: found=%v err=%v", found, err)
	}
	ids, err := index.ListProbeTaskIDs(ctx, "probe1")
	if err != nil || len(ids) != 0 {
		t.Fatalf("文件不存在时 ListProbeTaskIDs 应返回空: ids=%v err=%v", ids, err)
	}
	if err := index.Unregister(ctx, "probe1", "task-x"); err != nil {
		t.Fatalf("文件不存在时 Unregister 应幂等成功: %v", err)
	}
}

func TestDualTraversalRecoveryIndex_LegacyFileUntouched(t *testing.T) {
	index, dir := newTestDualIndex(t)
	ctx := context.Background()

	// 同目录放置 legacy 活动索引文件，dual 操作不得读取/覆盖它
	legacyPath := filepath.Join(dir, "traversal-active-index.json")
	legacyContent := []byte(`{"version":1,"tasks":{"legacy-task":"D:/out/legacy.checkpoint.json"}}`)
	if err := os.WriteFile(legacyPath, legacyContent, 0o644); err != nil {
		t.Fatalf("写入 legacy 文件: %v", err)
	}
	if err := index.Register(ctx, "probe1", "task-1", dualCheckpointPath(t, "a")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := index.Unregister(ctx, "probe1", "task-1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	got, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("读取 legacy 文件: %v", err)
	}
	if string(got) != string(legacyContent) {
		t.Fatalf("legacy 索引文件不得被修改: %s", got)
	}
	// dual 索引文件名与 legacy 不同
	if _, err := os.Stat(filepath.Join(dir, DualTraversalRecoveryIndexFileName)); err != nil {
		t.Fatalf("dual 索引文件应独立存在: %v", err)
	}
}
