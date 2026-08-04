package usecase

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
)

// registry probe-scoped checkpoint 收尾测试。
//
// 覆盖：运行期 checkpoint 回调不登记恢复映射 / 终态清理（completed/stopped）
// 注销映射并删除 checkpoint 文件 / 旧版本异 taskID 残留映射的升级兼容清除 /
// Unregister 失败保留 completion_failed 并可经 CloseProbe 幂等重试。

func TestManagerRegistry_CheckpointSavedCallbackDoesNotRegisterRecovery(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, _, opts := fx.factory.manager(Probe1).snapshotStart()
	cpPath := traversal.ResolveCheckpointPathFromCSV("D:/out/probe1-run.csv")
	opts.CheckpointSavedCallback(cpPath)
	if _, found, err := fx.index.Find(context.Background(), "probe1"); err != nil || found {
		t.Fatalf("running checkpoint must not be advertised as recoverable: found=%v err=%v", found, err)
	}
	completeSession(fx, Probe1)
}

func TestManagerRegistry_NormalCompletionUnregisterFailureRetainsOwnership(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 终态清理仅在实际存在映射时调用 Unregister（先 Find 再注销）：
	// 预置映射让失败注入真正打在 Unregister 上。
	cpPath := traversal.ResolveCheckpointPathFromCSV(filepath.Join(t.TempDir(), "probe1-run.csv"))
	if err := fx.cpStore.Write(cpPath, []byte(`{"version":3}`)); err != nil {
		t.Fatalf("写入 checkpoint: %v", err)
	}
	fx.index.seed("probe1", "probe1-task-1", cpPath)
	unregisterErr := errors.New("unregister failed")
	fx.index.setErrors(nil, unregisterErr)
	completeSession(fx, Probe1)
	if state, ok := fx.sessionStateOf(Probe1); !ok || state != sessionStateCompletionFailed {
		t.Fatalf("Unregister 失败必须保留 completion_failed, state=%v exists=%v", state, ok)
	}
	if fx.activeCount() != 1 || fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("Unregister 成功前不得递减或释放 lease")
	}

	fx.index.setErrors(nil, nil)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("CloseProbe retry: %v", err)
	}
}

func TestManagerRegistry_CompletionUnregistersOnNormalCompletion(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, _, opts := fx.factory.manager(Probe1).snapshotStart()
	opts.CheckpointSavedCallback(traversal.ResolveCheckpointPathFromCSV("D:/out/probe1-run.csv"))
	// 正常完成（fake manager 状态非 stopped/error）
	completeSession(fx, Probe1)
	if _, found, _ := fx.index.Find(context.Background(), "probe1"); found {
		t.Fatal("正常完成后映射应注销")
	}
}

func TestManagerRegistry_CompletionDiscardsMappingAndCheckpointOnStopped(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// stopped 是最终终态：恢复映射和临时 checkpoint 均应删除。
	csvPath := filepath.Join(t.TempDir(), "probe1-run.csv")
	cpPath := traversal.ResolveCheckpointPathFromCSV(csvPath)
	if err := fx.cpStore.Write(cpPath, []byte(`{"version":3}`)); err != nil {
		t.Fatalf("写入 checkpoint: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	manager.mu.Lock()
	manager.statusState = traversal.StateStopped
	manager.statusCSVPath = csvPath
	manager.mu.Unlock()

	completeSession(fx, Probe1)
	if _, found, err := fx.index.Find(context.Background(), "probe1"); err != nil || found {
		t.Fatalf("stopped 终态不得保留恢复映射: found=%v err=%v", found, err)
	}
	if exists, err := fx.cpStore.Stat(cpPath); err != nil || exists {
		t.Fatalf("stopped 终态不得保留 checkpoint: exists=%v err=%v", exists, err)
	}
}

func TestManagerRegistry_CompletionPurgesStaleForeignMapping(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 旧版本残留：索引中登记着与本 session 不同的 taskID（"停止后保留可恢复任务"
	// 时代的遗留映射，含问题电脑的索引-文件不一致脏状态）。adapter 要求注销 taskID
	// 与登记候选一致，直接以 session.taskID 注销会被拒绝，导致每个终态都
	// completion_failed、probe 被永久占用——本用例覆盖该升级兼容场景。
	stalePath := traversal.ResolveCheckpointPathFromCSV(filepath.Join(t.TempDir(), "old-run.csv"))
	if err := fx.cpStore.Write(stalePath, []byte(`{"version":3}`)); err != nil {
		t.Fatalf("写入残留 checkpoint: %v", err)
	}
	fx.index.seed("probe1", "old-version-task", stalePath)

	completeSession(fx, Probe1)
	// 终态清理必须成功提交（session 移出 map，不得 completion_failed），
	// 残留映射与其 checkpoint 文件一并清除。
	if _, ok := fx.sessionStateOf(Probe1); ok {
		t.Fatal("旧版本残留映射不得阻碍终态清理提交")
	}
	if _, found, _ := fx.index.Find(context.Background(), "probe1"); found {
		t.Fatal("旧版本残留映射应在终态清除")
	}
	if exists, _ := fx.cpStore.Stat(stalePath); exists {
		t.Fatal("旧版本残留 checkpoint 文件应删除")
	}
}
