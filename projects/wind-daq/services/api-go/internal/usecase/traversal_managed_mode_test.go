package usecase

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
)

// Task 8：TraversalManager managed Start/Resume 与 probe-scoped config key。
//
// 验证 managed ownership（不触碰 workflow lease / legacy activeIndex）、
// ManagedSessionOptions 冻结、finalize 后恰好一次完成回调、准入回滚不回调、
// probe-scoped 配置键持久化与加载。legacy 行为由既有 traversal_*_test.go 回归保证。

// managedTestOpts 构造测试用 managed 启动选项。
func managedTestOpts() ManagedSessionOptions {
	return ManagedSessionOptions{
		ProbeID: Probe1,
		Token:   SessionToken{ProbeID: Probe1, Generation: 7},
	}
}

// makeManagedTestConfig 单点配置（目标 0 与 mock 静止位置一致，可正常完成）。
func makeManagedTestConfig(taskID string) traversal.Config {
	config := makeTestConfig("")
	config.TaskID = taskID
	config.Path = []traversal.Point{{X: 0, Y: 0, Z: 0}}
	return config
}

// waitManagedCallback 等待完成回调（有界），返回携带的 token。
func waitManagedCallback(t *testing.T, callbacks <-chan SessionToken) SessionToken {
	t.Helper()
	select {
	case token := <-callbacks:
		return token
	case <-time.After(10 * time.Second):
		t.Fatal("等待 managed 完成回调超时")
		return SessionToken{}
	}
}

func TestTraversal_ManagedStart_NoLeaseTouchCompletionAfterFinalize(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	fakeLock := &fakeTraversalLockService{}
	mgr.lockService = fakeLock
	callbacks := make(chan SessionToken, 2)

	opts := managedTestOpts()
	config := makeManagedTestConfig("managed-task-1")
	opts.TaskID = config.TaskID
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }
	if err := mgr.StartManaged(config, opts); err != nil {
		t.Fatalf("StartManaged: %v", err)
	}
	// opts 冻结：调用方事后修改不影响 session 快照
	opts.Token = SessionToken{ProbeID: Probe2, Generation: 99}

	token := waitManagedCallback(t, callbacks)
	if token != (SessionToken{ProbeID: Probe1, Generation: 7}) {
		t.Fatalf("回调 token 应为冻结快照值, got %+v", token)
	}
	// 回调发生时输出端口已 finalize（sink 与 csvPort 不同实例：FinalizeTraversal 已调用）
	mgr.mu.RLock()
	session := mgr.session
	mgr.mu.RUnlock()
	if session != nil && !session.IsDone() {
		t.Fatal("回调时 run goroutine 应已退出")
	}
	// managed 路径不得触碰 workflow lease
	if len(fakeLock.acquired) != 0 || len(fakeLock.released) != 0 {
		t.Fatalf("managed 不得 Acquire/Release workflow lease: acquired=%v released=%v", fakeLock.acquired, fakeLock.released)
	}
	// 恰好一次回调
	select {
	case extra := <-callbacks:
		t.Fatalf("完成回调应恰好一次, got extra %+v", extra)
	default:
	}
	// 任务正常完成
	if status := mgr.Status(); status.CurrentPoint < status.TotalPoints {
		t.Fatalf("任务应完成全部点位: %+v", status)
	}
}

func TestTraversal_ManagedStart_StopPathConvergesToCallback(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	mgr.lockService = &fakeTraversalLockService{}
	callbacks := make(chan SessionToken, 2)
	opts := managedTestOpts()
	config := makeTestConfig("")
	config.TaskID = "managed-task-stop"
	opts.TaskID = config.TaskID
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }

	if err := mgr.StartManaged(config, opts); err != nil {
		t.Fatalf("StartManaged: %v", err)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	token := waitManagedCallback(t, callbacks)
	if token != opts.Token {
		t.Fatalf("Stop 路径应收敛到同一回调, got %+v", token)
	}
	fakeLock := mgr.lockService.(*fakeTraversalLockService)
	if len(fakeLock.acquired) != 0 || len(fakeLock.released) != 0 {
		t.Fatal("Stop 路径同样不得触碰 workflow lease")
	}
}

func TestTraversal_ManagedStart_AdmissionRollbackNoCallback(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	fakeLock := &fakeTraversalLockService{}
	mgr.lockService = fakeLock
	cpErr := errors.New("checkpoint factory boom")
	mgr.checkpointPortFactory = &failingCheckpointPortFactory{err: cpErr}
	callbacks := make(chan SessionToken, 1)
	opts := managedTestOpts()
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }

	config := makeTestConfig("")
	config.TaskID = "managed-task-rollback"
	err := mgr.StartManaged(config, opts)
	if !errors.Is(err, cpErr) {
		t.Fatalf("应返回 checkpoint factory 错误, got %v", err)
	}
	select {
	case token := <-callbacks:
		t.Fatalf("准入回滚不得触发完成回调, got %+v", token)
	default:
	}
	if len(fakeLock.released) != 0 {
		t.Fatal("managed 回滚不得 Release workflow lease（registry 负责准入回滚）")
	}
}

func TestTraversal_ManagedStart_RejectsNonAcquiringReferencedDevice(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	mgr.SetAcquisitionController(&mockAcquisitionController{
		connected: map[string]bool{"probe-device-id": true, "environment-device-id": true},
		acquiring: map[string]bool{"probe-device-id": true, "environment-device-id": false},
		names:     map[string]string{"environment-device-id": "环境采集仪"},
	})
	callbacks := make(chan SessionToken, 1)
	opts := managedTestOpts()
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }
	config := makeManagedTestConfig("managed-device-check")
	config.DeviceID = "probe-device-id"
	config.Channels = []int{0, 1}
	config.ChannelRefs = map[int]traversal.ChannelRef{
		0: {DeviceID: "probe-device-id", Index: 0},
		1: {DeviceID: "environment-device-id", Index: 0},
	}

	err := mgr.StartManaged(config, opts)
	if err == nil || !contains(err.Error(), "环境采集仪") || contains(err.Error(), "environment-device-id") {
		t.Fatalf("expected readable pre-start acquisition error, got %v", err)
	}
	if status := mgr.Status(); status.State != traversal.StateIdle {
		t.Fatalf("managed session must not start after failed acquisition check, got %+v", status)
	}
	select {
	case token := <-callbacks:
		t.Fatalf("rejected managed start must not invoke completion callback, got %+v", token)
	default:
	}
}

func TestTraversal_ManagedStart_RejectsInvalidOpts(t *testing.T) {
	config := makeTestConfig("")
	config.TaskID = "managed-task-opts"
	valid := managedTestOpts()
	valid.CompletionCallback = func(SessionToken) {}

	cases := []struct {
		name   string
		mutate func(*ManagedSessionOptions)
	}{
		{"非法 probeID", func(o *ManagedSessionOptions) { o.ProbeID = "probe9" }},
		{"token 与 probeID 不一致", func(o *ManagedSessionOptions) { o.Token.ProbeID = Probe2 }},
		{"缺少完成回调", func(o *ManagedSessionOptions) { o.CompletionCallback = nil }},
		{"configKey 不匹配", func(o *ManagedSessionOptions) { o.ConfigKey = "traversal.probe2" }},
		{"taskID 不一致", func(o *ManagedSessionOptions) { o.TaskID = "other-task" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr, _, _, _ := newCheckpointTestManager()
			opts := valid
			tc.mutate(&opts)
			if err := mgr.StartManaged(config, opts); err == nil {
				t.Fatal("应拒绝非法 ManagedSessionOptions")
			}
		})
	}
}

func TestTraversal_ManagedResume_ManagedOwnership(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	fakeLock := &fakeTraversalLockService{}
	mgr.lockService = fakeLock
	callbacks := make(chan SessionToken, 1)
	opts := managedTestOpts()
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }

	config := makeManagedTestConfig("managed-cp-task")
	cp := traversal.Checkpoint{
		Version:         2,
		TaskID:          config.TaskID,
		State:           traversal.StateStopped,
		CompletedPoints: 0,
		TotalPoints:     len(config.Path),
		Snapshot: traversal.TraversalRunSnapshot{
			Config:      config,
			TotalPoints: len(config.Path),
		},
	}
	opts.TaskID = cp.TaskID
	taskID, err := mgr.ResumeManaged(cp, opts)
	if err != nil {
		t.Fatalf("ResumeManaged: %v", err)
	}
	if taskID != cp.TaskID {
		t.Fatalf("返回权威 taskID 不符: %q vs %q", taskID, cp.TaskID)
	}
	token := waitManagedCallback(t, callbacks)
	if token != opts.Token {
		t.Fatalf("managed resume 完成回调 token 不符: %+v", token)
	}
	if len(fakeLock.acquired) != 0 || len(fakeLock.released) != 0 {
		t.Fatal("managed resume 不得触碰 workflow lease")
	}
	// 从断点继续：CompletedPoints 之后的点全部完成
	if status := mgr.Status(); status.CurrentPoint < status.TotalPoints {
		t.Fatalf("恢复后应完成剩余点位: %+v", status)
	}
}

func TestTraversal_ManagedResume_RejectsNonAcquiringReferencedDevice(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	mgr.SetAcquisitionController(&mockAcquisitionController{
		connected: map[string]bool{"probe-device-id": true, "environment-device-id": true},
		acquiring: map[string]bool{"probe-device-id": true, "environment-device-id": false},
		names:     map[string]string{"environment-device-id": "环境采集仪"},
	})
	callbacks := make(chan SessionToken, 1)
	opts := managedTestOpts()
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }
	config := makeManagedTestConfig("managed-resume-device-check")
	config.DeviceID = "probe-device-id"
	config.Channels = []int{0, 1}
	config.ChannelRefs = map[int]traversal.ChannelRef{
		0: {DeviceID: "probe-device-id", Index: 0},
		1: {DeviceID: "environment-device-id", Index: 0},
	}
	cp := traversal.Checkpoint{
		Version:         2,
		TaskID:          config.TaskID,
		State:           traversal.StateStopped,
		CompletedPoints: 0,
		TotalPoints:     len(config.Path),
		Snapshot: traversal.TraversalRunSnapshot{
			Config:      config,
			TotalPoints: len(config.Path),
		},
	}
	opts.TaskID = cp.TaskID

	_, err := mgr.ResumeManaged(cp, opts)
	if err == nil || !contains(err.Error(), "环境采集仪") || contains(err.Error(), "environment-device-id") {
		t.Fatalf("expected readable pre-resume acquisition error, got %v", err)
	}
	if status := mgr.Status(); status.State != traversal.StateIdle {
		t.Fatalf("managed session must not resume after failed acquisition check, got %+v", status)
	}
	select {
	case token := <-callbacks:
		t.Fatalf("rejected managed resume must not invoke completion callback, got %+v", token)
	default:
	}
}

func TestTraversal_ManagedConfigKey_PersistAndReload(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	store := &fakeConfigStore{data: make(map[string][]byte)}
	mgr.configStore = store
	mgr.SetConfigKey("traversal.probe1")

	mgr.SaveConfigRaw(json.RawMessage(`{"taskId":"x","probeType":"five-hole","motionAxes":[{"controllerId":"ctrl-a","axis":"X"}]}`))
	if _, ok := store.data["traversal.probe1"]; !ok {
		t.Fatal("SaveConfigRaw 应持久化到 traversal.probe1")
	}
	if _, ok := store.data["traversal"]; ok {
		t.Fatal("managed manager 不得写 legacy traversal 键")
	}
	// 同键新 manager：SetConfigKey 后立即按 probe 键加载
	mgr2, _, _, _ := newCheckpointTestManager()
	mgr2.configStore = store
	mgr2.SetConfigKey("traversal.probe1")
	if raw := mgr2.GetConfigRaw(); len(raw) == 0 {
		t.Fatal("SetConfigKey 后应按 probe 键重新加载持久化配置")
	}
	// legacy 默认键不受影响：未 SetConfigKey 的 manager 仍用 "traversal"
	mgr3, _, _, _ := newCheckpointTestManager()
	mgr3.configStore = store
	mgr3.SaveConfigRaw(json.RawMessage(`{"taskId":"legacy"}`))
	if _, ok := store.data["traversal"]; !ok {
		t.Fatal("legacy manager 应继续使用 traversal 键")
	}
}

func TestTraversal_LegacyStart_KeepsDirectLeaseOwnership(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	fakeLock := &fakeTraversalLockService{}
	mgr.lockService = fakeLock

	config := makeTestConfig("")
	config.TaskID = "legacy-task-1"
	if err := mgr.Start(config); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fakeLock.acquired) != 1 || fakeLock.acquired[0].holder != "legacy-task-1" {
		t.Fatalf("legacy Start 应自行 Acquire workflow lease: %+v", fakeLock.acquired)
	}
	// legacy Start 不启动主循环（ParseAndStartTraversal 负责）；手动驱动完成
	mgr.mu.Lock()
	mgr.status.State = traversal.StateStopped
	mgr.mu.Unlock()
	mgr.RunTraversalLoop()
	if len(fakeLock.released) != 1 || fakeLock.released[0].holder != "legacy-task-1" {
		t.Fatalf("legacy finalize 应自行 Release workflow lease: %+v", fakeLock.released)
	}
}

func TestTraversal_ManagedDoneChannel(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	// 无活动 session：已关闭通道
	select {
	case <-mgr.Done():
	default:
		t.Fatal("无 session 时 Done 应返回已关闭通道")
	}
	callbacks := make(chan SessionToken, 1)
	opts := managedTestOpts()
	opts.CompletionCallback = func(token SessionToken) { callbacks <- token }
	config := makeTestConfig("")
	config.TaskID = "managed-task-done"
	if err := mgr.StartManaged(config, opts); err != nil {
		t.Fatalf("StartManaged: %v", err)
	}
	waitManagedCallback(t, callbacks)
	// 完成后 session done 关闭
	select {
	case <-mgr.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("完成后 Done 通道应关闭")
	}
}
