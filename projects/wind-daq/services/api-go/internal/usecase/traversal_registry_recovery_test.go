package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// Task 11：registry probe-scoped 恢复 façade。
//
// 覆盖：LoadCheckpoint 唯一候选 / taskID 不匹配 / probeID 不匹配 / v1/v2 拒绝 /
// resume 准入事务（冲突时 checkpoint 与输出文件不变）/ ClearCheckpoint /
// 运行期映射注册与完成注销。

// seedDualCheckpoint 构造 v3 checkpoint 写入 fake store 并登记到 fake index，返回文件路径。
func seedDualCheckpoint(t *testing.T, fx *registryFixture, probeID ProbeID, taskID string, bound []string) string {
	t.Helper()
	csvPath := filepath.Join(t.TempDir(), string(probeID)+"-run.csv")
	cpPath := traversal.ResolveCheckpointPathFromCSV(csvPath)
	cp := traversal.Checkpoint{
		Version:         traversal.DualCheckpointVersion,
		TaskID:          taskID,
		ProbeID:         string(probeID),
		State:           traversal.StateStopped,
		CompletedPoints: 1,
		TotalPoints:     3,
		SavePath:        csvPath,
		CreatedAt:       1785000000000,
		Snapshot: traversal.TraversalRunSnapshot{
			Config: traversal.Config{
				TaskID:   taskID,
				DeviceID: "dev-1",
				Channels: []int{0},
				Path:     []traversal.Point{{X: 0}, {X: 1}, {X: 2}},
			},
			TotalPoints:        3,
			CommitSeq:          1,
			CSVPath:            csvPath,
			BoundControllerIDs: bound,
		},
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		t.Fatalf("序列化 checkpoint: %v", err)
	}
	if err := fx.cpStore.Write(cpPath, data); err != nil {
		t.Fatalf("写入 checkpoint: %v", err)
	}
	fx.index.seed(string(probeID), taskID, cpPath)
	return cpPath
}

func TestTraversal_Resume_HappyPath(t *testing.T) {
	fx := newRegistryFixture(t)
	seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")

	taskID, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "probe1-task-9")
	if err != nil {
		t.Fatalf("ResumeFromCheckpoint: %v", err)
	}
	if taskID != "probe1-task-9" {
		t.Fatalf("应返回 checkpoint 权威 taskID, got %q", taskID)
	}
	manager := fx.factory.manager(Probe1)
	if manager.resumeCalls != 1 {
		t.Fatalf("ResumeManaged 应调用一次, got %d", manager.resumeCalls)
	}
	manager.mu.Lock()
	resumeCp, opts := manager.lastResumeCp, manager.lastOpts
	manager.mu.Unlock()
	if resumeCp.TaskID != taskID || opts.TaskID != taskID {
		t.Fatal("managed Resume 应使用 checkpoint 权威 taskID")
	}
	if opts.Token.ProbeID != Probe1 || opts.Token.Generation != 1 {
		t.Fatalf("SessionToken 不符: %+v", opts.Token)
	}
	if opts.CheckpointSavedCallback == nil || opts.CompletionCallback == nil {
		t.Fatal("完成与 checkpoint 回调均应注入")
	}
	// 准入事务：workflow + controller lease 已持有
	if held, _, acquires, _ := fx.workflow.state(); !held || acquires != 1 {
		t.Fatal("resume 应走完整 workflow lease 准入")
	}
	if fx.controllers.heldCount("ctrl-a") != 1 || fx.activeCount() != 1 {
		t.Fatal("resume 应预占 checkpoint 绑定控制器")
	}
}

func TestTraversal_Resume_TaskIDMismatch(t *testing.T) {
	fx := newRegistryFixture(t)
	seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	_, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "wrong-task")
	if !errors.Is(err, ErrTaskIDMismatch) {
		t.Fatalf("应返回 task_id_mismatch, got %v", err)
	}
	if fx.factory.manager(Probe1) != nil && fx.factory.manager(Probe1).resumeCalls != 0 {
		t.Fatal("taskID 不符不得调用 ResumeManaged")
	}
	if fx.activeCount() != 0 || fx.controllers.totalHeld() != 0 {
		t.Fatal("拒绝后不得登记 session 或预占控制器")
	}
}

func TestTraversal_Resume_ProbeIDMismatch(t *testing.T) {
	fx := newRegistryFixture(t)
	// 映射登记在 probe1 下，但 checkpoint 内容属于 probe2（篡改场景）
	csvPath := filepath.Join(t.TempDir(), "probe1-run.csv")
	cpPath := traversal.ResolveCheckpointPathFromCSV(csvPath)
	cp := traversal.Checkpoint{Version: traversal.DualCheckpointVersion, TaskID: "t-1", ProbeID: "probe2"}
	data, _ := json.Marshal(cp)
	if err := fx.cpStore.Write(cpPath, data); err != nil {
		t.Fatalf("写入 checkpoint: %v", err)
	}
	fx.index.seed("probe1", "t-1", cpPath)

	_, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "t-1")
	if !errors.Is(err, ErrProbeIDMismatch) {
		t.Fatalf("应返回 probe_id_mismatch, got %v", err)
	}
}

func TestTraversal_Resume_CheckpointTaskIDMustMatchIndex(t *testing.T) {
	fx := newRegistryFixture(t)
	cpPath := seedDualCheckpoint(t, fx, Probe1, "index-task", []string{"ctrl-a"})
	data, err := fx.cpStore.Read(cpPath)
	if err != nil {
		t.Fatalf("Read checkpoint: %v", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatalf("Unmarshal checkpoint: %v", err)
	}
	cp.TaskID = "tampered-task"
	data, _ = json.Marshal(cp)
	if err := fx.cpStore.Write(cpPath, data); err != nil {
		t.Fatalf("Write checkpoint: %v", err)
	}

	_, err = fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "index-task")
	if !errors.Is(err, ErrTaskIDMismatch) {
		t.Fatalf("checkpoint/index task mismatch 应拒绝, got %v", err)
	}
}

func TestTraversal_Resume_LegacyVersionRejected(t *testing.T) {
	fx := newRegistryFixture(t)
	csvPath := filepath.Join(t.TempDir(), "run.csv")
	cpPath := traversal.ResolveCheckpointPathFromCSV(csvPath)
	cp := traversal.Checkpoint{Version: traversal.CheckpointVersion, TaskID: "legacy-task"}
	data, _ := json.Marshal(cp)
	if err := fx.cpStore.Write(cpPath, data); err != nil {
		t.Fatalf("写入 v2 checkpoint: %v", err)
	}
	fx.index.seed("probe1", "legacy-task", cpPath)

	_, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "legacy-task")
	if !errors.Is(err, ports.ErrCheckpointVersionMismatch) {
		t.Fatalf("dual 路径遇 v2 应返回 checkpoint_version_mismatch, got %v", err)
	}
}

func TestTraversal_Resume_ControllerConflictKeepsCheckpoint(t *testing.T) {
	fx := newRegistryFixture(t)
	cpPath := seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	before, _ := fx.cpStore.Read(cpPath)
	// 外部持有 ctrl-a（另一 session 占用中）
	if _, err := fx.controllers.Acquire(context.Background(), "ctrl-a", "external", time.Minute); err != nil {
		t.Fatalf("预置外部 lease: %v", err)
	}
	fx.seedPersistedBindings(Probe2, "ctrl-b")

	_, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "probe1-task-9")
	if !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("应返回 resource_conflict, got %v", err)
	}
	// checkpoint 文件与恢复映射保持不变；无 session/lease 残留
	after, _ := fx.cpStore.Read(cpPath)
	if string(before) != string(after) {
		t.Fatal("资源冲突时 checkpoint 文件不得改变")
	}
	if _, found, _ := fx.index.Find(context.Background(), "probe1"); !found {
		t.Fatal("资源冲突时恢复映射不得注销")
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("准入回滚后不得登记 session")
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("本次获取的 workflow lease 应已回滚: held=%v releases=%d", held, releases)
	}
	if fx.factory.manager(Probe1).resumeCalls != 0 {
		t.Fatal("准入失败不得调用 ResumeManaged（append/运动 I/O 之前）")
	}
}

func TestTraversal_Resume_Concurrency(t *testing.T) {
	fx := newRegistryFixture(t)
	seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	ctx := context.Background()

	// 同 probe 并发 resume：准入临界区保证恰好一个成功，无双重占用
	const goroutines = 4
	var ready sync.WaitGroup
	ready.Add(goroutines)
	results := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			ready.Done()
			_, err := fx.registry.ResumeFromCheckpoint(ctx, Probe1, "probe1-task-9")
			results <- err
		}()
	}
	ready.Wait()
	var successes int
	for g := 0; g < goroutines; g++ {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("失败者应返回 already_running, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("并发 resume 应恰好一个成功, got %d", successes)
	}
	if fx.controllers.heldCount("ctrl-a") != 1 || fx.controllers.totalHeld() != 1 {
		t.Fatal("控制器不得被双重占用")
	}
	if fx.activeCount() != 1 {
		t.Fatalf("activeCount 应为 1, got %d", fx.activeCount())
	}
}

func TestManagerRegistry_LoadCheckpoint_UniqueCandidate(t *testing.T) {
	fx := newRegistryFixture(t)
	ctx := context.Background()

	// 无候选：返回 (nil, nil)
	cp, err := fx.registry.LoadCheckpoint(ctx, Probe1)
	if err != nil || cp != nil {
		t.Fatalf("无候选应返回 nil: cp=%v err=%v", cp, err)
	}
	// 唯一候选：返回 checkpoint 内容
	seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	cp, err = fx.registry.LoadCheckpoint(ctx, Probe1)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if cp.TaskID != "probe1-task-9" || cp.ProbeID != "probe1" || cp.Snapshot.CommitSeq != 1 {
		t.Fatalf("候选内容不符: %+v", cp)
	}
	// 跨 probe 隔离：probe2 无候选
	if cp2, err := fx.registry.LoadCheckpoint(ctx, Probe2); err != nil || cp2 != nil {
		t.Fatalf("probe2 应无候选: cp=%v err=%v", cp2, err)
	}
}

func TestManagerRegistry_ClearCheckpoint_ValidatesTaskID(t *testing.T) {
	fx := newRegistryFixture(t)
	cpPath := seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	ctx := context.Background()

	// taskID 不符：映射与文件均不动
	if err := fx.registry.ClearCheckpoint(ctx, Probe1, "wrong-task"); !errors.Is(err, ErrTaskIDMismatch) {
		t.Fatalf("应返回 task_id_mismatch, got %v", err)
	}
	if _, found, _ := fx.index.Find(ctx, "probe1"); !found {
		t.Fatal("taskID 不符时映射不得注销")
	}
	if exists, _ := fx.cpStore.Stat(cpPath); !exists {
		t.Fatal("taskID 不符时文件不得删除")
	}
	// 正确 taskID：原子注销映射 + 删除文件
	if err := fx.registry.ClearCheckpoint(ctx, Probe1, "probe1-task-9"); err != nil {
		t.Fatalf("ClearCheckpoint: %v", err)
	}
	if _, found, _ := fx.index.Find(ctx, "probe1"); found {
		t.Fatal("清除后映射应注销")
	}
	if exists, _ := fx.cpStore.Stat(cpPath); exists {
		t.Fatal("清除后文件应删除")
	}
	// 无候选幂等
	if err := fx.registry.ClearCheckpoint(ctx, Probe1, "probe1-task-9"); err != nil {
		t.Fatalf("无候选应幂等返回 nil: %v", err)
	}
}

func TestManagerRegistry_ClearCheckpoint_RejectsActiveSession(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := fx.registry.ClearCheckpoint(context.Background(), Probe1, "probe1-task-1"); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("运行中清除应拒绝, got %v", err)
	}
	completeSession(fx, Probe1)
}

func TestManagerRegistry_ClearCheckpoint_SerializesWithResumeAdmission(t *testing.T) {
	fx := newRegistryFixture(t)
	cpPath := seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	entered, unblock := fx.index.setFindBlock()
	defer unblock()

	resumeDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "probe1-task-9")
		resumeDone <- err
	}()
	<-entered
	clearDone := make(chan error, 1)
	go func() { clearDone <- fx.registry.ClearCheckpoint(context.Background(), Probe1, "probe1-task-9") }()
	select {
	case err := <-clearDone:
		t.Fatalf("ClearCheckpoint crossed in-flight Resume admission: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	if err := <-resumeDone; err != nil {
		t.Fatalf("ResumeFromCheckpoint: %v", err)
	}
	if err := <-clearDone; !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Resume commit 后 ClearCheckpoint 应拒绝 active session, got %v", err)
	}
	if exists, _ := fx.cpStore.Stat(cpPath); !exists {
		t.Fatal("active resume 的 checkpoint 不得被 ClearCheckpoint 删除")
	}
}

func TestManagerRegistry_CheckpointSavedCallbackRegistersMapping(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, _, opts := fx.factory.manager(Probe1).snapshotStart()
	cpPath := traversal.ResolveCheckpointPathFromCSV("D:/out/probe1-run.csv")
	opts.CheckpointSavedCallback(cpPath)
	ref, found, err := fx.index.Find(context.Background(), "probe1")
	if err != nil || !found {
		t.Fatalf("回调后应登记映射: found=%v err=%v", found, err)
	}
	if ref.TaskID != "probe1-task-1" || ref.Path != cpPath {
		t.Fatalf("映射内容不符: %+v", ref)
	}
	// 同 taskID 重复登记幂等
	if err := fx.index.Register(context.Background(), "probe1", "probe1-task-1", cpPath); err != nil {
		t.Fatalf("同 taskID 重复登记应幂等: %v", err)
	}
	completeSession(fx, Probe1)
}

func TestManagerRegistry_CheckpointRegisterFailureStopsAndRetainsCompletion(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	_, _, opts := manager.snapshotStart()
	manager.setOnStop(func() { opts.CompletionCallback(opts.Token) })
	registerErr := errors.New("register failed")
	fx.index.setErrors(registerErr, nil)

	opts.CheckpointSavedCallback("D:/out/probe1.checkpoint.json")
	deadline := time.After(time.Second)
	for manager.stopCallCount() != 1 {
		select {
		case <-deadline:
			t.Fatal("Register 失败必须异步请求停止 session")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if state, ok := fx.sessionStateOf(Probe1); !ok || state != sessionStateCompletionFailed {
		t.Fatalf("Register 失败必须保持可重试 completion_failed, state=%v exists=%v", state, ok)
	}
	if fx.activeCount() != 1 || fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("recovery mapping 修复前不得释放 ownership")
	}

	fx.index.setErrors(nil, nil)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("CloseProbe retry: %v", err)
	}
}

func TestManagerRegistry_CheckpointRegisterFailureStopIsAsyncAndIdempotent(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	_, _, opts := manager.snapshotStart()
	unblockStop := manager.setStopBlock()
	defer unblockStop()
	fx.index.setErrors(errors.New("register failed"), nil)

	returned := make(chan struct{})
	go func() {
		opts.CheckpointSavedCallback("D:/out/probe1.checkpoint.json")
		opts.CheckpointSavedCallback("D:/out/probe1.checkpoint.json")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("checkpoint callback 不得同步等待阻塞的 manager.Stop")
	}
	deadline := time.After(time.Second)
	for manager.stopCallCount() != 1 {
		select {
		case <-deadline:
			t.Fatalf("重复登记失败只应启动一个 Stop, calls=%d", manager.stopCallCount())
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestManagerRegistry_NormalCompletionUnregisterFailureRetainsOwnership(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
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

func TestManagerRegistry_CompletionKeepsMappingOnStopped(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// stopped 终态 + checkpoint 文件存在：映射保留（spec I6 可恢复）
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
	ref, found, err := fx.index.Find(context.Background(), "probe1")
	if err != nil || !found {
		t.Fatalf("stopped 终态应保留恢复映射: found=%v err=%v", found, err)
	}
	if ref.TaskID != "probe1-task-1" || ref.Path != cpPath {
		t.Fatalf("保留映射内容不符: %+v", ref)
	}
}
