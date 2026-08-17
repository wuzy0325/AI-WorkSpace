package usecase

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/ports"
)

// ==================== 资源锁测试桩（spec Task 21） ====================
//
// fakeTraversalLockService 资源锁测试桩，可注入 Acquire/Release 错误。
// 调用记录在 acquired/released 切片中，供测试断言 Release 是否被调用、
// 调用参数是否正确（resource/holder）。
//
// 用途：覆盖四条 Release 错误路径（Start rollback / abort / Stop / finalize）
// 的错误聚合与告警契约。生产路径走 resourcelock.Default() 单例，行为不变。
type fakeTraversalLockService struct {
	acquireErr error // Acquire 返回的错误（每次调用都返回）
	releaseErr error // Release 返回的错误（每次调用都返回）

	mu       sync.Mutex
	acquired []traversalLockCall
	released []traversalLockCall
}

type traversalLockCall struct {
	resource string
	holder   string
	ttl      time.Duration
}

func (f *fakeTraversalLockService) Acquire(resource, holder string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquired = append(f.acquired, traversalLockCall{resource: resource, holder: holder, ttl: ttl})
	return f.acquireErr
}

func (f *fakeTraversalLockService) Release(resource, holder string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, traversalLockCall{resource: resource, holder: holder})
	return f.releaseErr
}

func (f *fakeTraversalLockService) releaseCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.released)
}

// recordingSlogHandler 捕获 slog 记录供测试断言。
//
// 用于验证 void 路径的 warning 契约和"失败后不记录成功 info"契约（spec Task 21）。
// 通过 slog.SetDefault 临时替换全局 logger，测试结束必须调 restore 恢复。
type recordingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func newRecordingSlogHandler() *recordingSlogHandler {
	return &recordingSlogHandler{}
}

func (h *recordingSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingSlogHandler) WithGroup(_ string) slog.Handler      { return h }

// hasLevelMessage 报告是否存在指定级别且消息包含 substr 的记录。
func (h *recordingSlogHandler) hasLevelMessage(level slog.Level, msgSubstr string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		r := &h.records[i]
		if r.Level == level && strings.Contains(r.Message, msgSubstr) {
			return true
		}
	}
	return false
}

// withRecordingLogger 临时替换 slog.Default() 为录制 handler，返回 handler 和 restore 函数。
// restore 必须在测试结束（含失败路径）调用，避免污染后续测试。
func withRecordingLogger(t *testing.T) (*recordingSlogHandler, func()) {
	t.Helper()
	h := newRecordingSlogHandler()
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	return h, func() { slog.SetDefault(old) }
}

// failingCheckpointPortFactory 永远返回注入错误的断点端口工厂桩（spec Task 21 Path 1）。
type failingCheckpointPortFactory struct {
	err error
}

func (f *failingCheckpointPortFactory) Create(_ context.Context, _ string) (ports.TraversalCheckpointPort, error) {
	return nil, f.err
}

func TestTraversalSessionCancelAndDoneAreIdempotent(t *testing.T) {
	session := newTraversalRunSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
	session.Cancel()
	session.Cancel()
	session.MarkDone()
	session.MarkDone()
	select {
	case <-session.Context().Done():
	default:
		t.Fatal("session context must be cancelled")
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("session done channel must be closed")
	}
}

func TestTraversalOwnershipRejectsStaleTaskUpdate(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.mu.Lock()
	manager.session = newTraversalRunSession(context.Background(), "task-new", traversal.TraversalRunSnapshot{})
	manager.status = traversal.Status{TaskID: "task-new", State: traversal.StateRunning}
	manager.mu.Unlock()

	manager.updatePhase("task-old", traversal.StateMoving, traversal.PhaseMoving, 3, 10)
	status := manager.Status()
	if status.State != traversal.StateRunning || status.CurrentPointIndex != 0 {
		t.Fatalf("stale task changed active state: %+v", status)
	}
}

func TestTraversalStateDoesNotOverwriteProtectedState(t *testing.T) {
	protected := []traversal.State{traversal.StatePaused, traversal.StateStopped, traversal.StateError}
	for _, state := range protected {
		t.Run(string(state), func(t *testing.T) {
			manager := NewTraversalManager(nil, nil, nil, nil, nil)
			manager.mu.Lock()
			manager.session = newTraversalRunSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
			manager.status = traversal.Status{TaskID: "task-1", State: state}
			manager.mu.Unlock()

			manager.updatePhase("task-1", traversal.StateAcquiring, traversal.PhaseAcquiring, 2, 5)
			if got := manager.Status().State; got != state {
				t.Fatalf("protected state changed from %s to %s", state, got)
			}
		})
	}
}

func TestTraversalSessionAllowsOnlyOneActiveOwner(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	first, err := manager.beginSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
	if err != nil {
		t.Fatalf("begin first session: %v", err)
	}
	if _, err := manager.beginSession(context.Background(), "task-2", traversal.TraversalRunSnapshot{}); err == nil {
		t.Fatal("expected second active session to be rejected")
	}
	first.MarkDone()
	if _, err := manager.beginSession(context.Background(), "task-2", traversal.TraversalRunSnapshot{}); err != nil {
		t.Fatalf("begin replacement session after done: %v", err)
	}
}

func TestStartRejectsActiveSessionEvenWhenPublicStateIsTerminal(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.mu.Lock()
	active := newTraversalRunSession(context.Background(), "task-active", traversal.TraversalRunSnapshot{})
	manager.session = active
	manager.status = traversal.Status{TaskID: "task-active", State: traversal.StateStopped}
	manager.mu.Unlock()

	err := manager.Start(traversal.Config{
		TaskID:   "task-next",
		DeviceID: "device-1",
		Channels: []int{0},
		Path:     []traversal.Point{{X: 1}},
	})
	if err == nil {
		t.Fatal("expected Start to reject while previous session is still active")
	}
	manager.mu.RLock()
	current := manager.session
	manager.mu.RUnlock()
	if current != active {
		t.Fatal("Start replaced the active session")
	}
}

func TestRunTraversalLoopMarksSessionDoneOnEveryExit(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	session := newTraversalRunSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
	manager.mu.Lock()
	manager.session = session
	manager.status = traversal.Status{TaskID: "task-1", State: traversal.StateStopped}
	manager.mu.Unlock()

	manager.RunTraversalLoop()

	select {
	case <-session.Done():
	default:
		t.Fatal("RunTraversalLoop must mark its session done before returning")
	}
}

// ==================== Traversal Lock Release 错误路径测试（spec Task 21） ====================
//
// 覆盖四条 Release 路径的错误处理契约：
//   - Path 1 (Start rollback, 可返回)  — Release 错误 join 进返回错误
//   - Path 2 (abort, void)             — Release 错误仅 Warn，不影响 void 签名
//   - Path 3 (Stop, 可返回)            — Release 错误 join 进 stopErr
//   - Path 4 (finalize, void)          — 见 traversal_view_test.go
//
// 验收标准：
//   - 可返回路径 join/return release error（errors.Is 可识别）
//   - void 路径 warning（slog.Warn 记录）
//   - 失败后不记录成功 info（Path 4 关键，见 traversal_view_test.go）
//   - 不强制释放他人锁（依赖 resourcelock.Service.Release 自身的 holder 校验，
//     不在 traversal 层做 force-release）

// TestStart_CheckpointPortFailure_JoinsReleaseError 验证 Path 1：
// checkpointPortFactory.Create 失败时，Start rollback 路径调用 Release；
// 若 Release 同时失败，返回的错误应通过 errors.Join 聚合 cpErr 和 releaseErr，
// 调用方可通过 errors.Is 同时识别两个错误。
//
// 测试前置：manager 注入 fakeLock（Release 返回 releaseErr）和 failingCheckpointPortFactory
// 测试步骤：调用 Start 触发 checkpoint factory 失败路径
// 期待结果：返回错误同时包含 cpErr 和 releaseErr；Release 被调用一次
//
// 修复场景（spec Task 21）：旧实现 `_ = Release(...)` 静默丢弃 Release 错误，
// 只返回 cpErr。修复后用 errors.Join 聚合，确保 Release 失败可被调用方识别。
func TestStart_CheckpointPortFailure_JoinsReleaseError(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	cpErr := errors.New("checkpoint factory boom")
	releaseErr := errors.New("release denied: held by other")
	manager.lockService = &fakeTraversalLockService{releaseErr: releaseErr}
	manager.checkpointPortFactory = &failingCheckpointPortFactory{err: cpErr}

	config := traversal.Config{
		TaskID:   "task-cp-fail",
		DeviceID: "device-1",
		Channels: []int{0},
		Path:     []traversal.Point{{X: 1}},
		// SavePath 留空 → snapshot.CSVPath = "traversal_task-cp-fail.csv"（非空）
		// 满足 `checkpointPortFactory != nil && snapshot.CSVPath != ""` 进入分支
	}

	err := manager.Start(config)
	if err == nil {
		t.Fatal("Start 应在 checkpoint factory 失败时返回错误，实际返回 nil")
	}
	// 验证 cpErr 可识别（旧实现也能通过此项）
	if !errors.Is(err, cpErr) {
		t.Errorf("Start 错误应包含 checkpoint 错误 %v，实际: %v", cpErr, err)
	}
	// 验证 releaseErr 可识别（spec Task 21 修复点：旧实现只返回 cpErr，此项红灯）
	if !errors.Is(err, releaseErr) {
		t.Errorf("Start 错误应包含 release 错误 %v（errors.Join 聚合），实际: %v", releaseErr, err)
	}
	// 验证 Release 被调用一次（rollback 路径触发）
	fakeLock, ok := manager.lockService.(*fakeTraversalLockService)
	if !ok {
		t.Fatalf("manager.lockService 类型断言失败: %T", manager.lockService)
	}
	if got := fakeLock.releaseCount(); got != 1 {
		t.Errorf("Release 调用次数 = %d，期望 1", got)
	}
}

// TestAbortStartLocked_ReleaseFailure_LogsWarning 验证 Path 2：
// abortStartLocked（void 函数）调用 Release 失败时，仅记录 slog.Warn，
// 不影响 void 签名（无错误返回），也不静默丢弃错误。
//
// 测试前置：manager 注入 fakeLock（Release 返回 releaseErr），构造一个活动 session
// 测试步骤：调用 abortStartLocked
// 期待结果：Release 被调用；slog.Warn 记录 release 失败；函数无返回值（void）
//
// 修复场景（spec Task 21）：旧实现 `_ = Release(...)` 静默丢弃错误。
// 修复后用 slog.Warn 记录，确保运维可观测。
func TestAbortStartLocked_ReleaseFailure_LogsWarning(t *testing.T) {
	handler, restore := withRecordingLogger(t)
	defer restore()

	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	releaseErr := errors.New("release denied: held by other")
	manager.lockService = &fakeTraversalLockService{releaseErr: releaseErr}

	session := newTraversalRunSession(context.Background(), "task-abort", traversal.TraversalRunSnapshot{})
	manager.abortStartLocked(session, "task-abort", "test abort trigger", traversal.ErrSaveFailed)

	// 验证 Release 被调用
	fakeLock, ok := manager.lockService.(*fakeTraversalLockService)
	if !ok {
		t.Fatalf("manager.lockService 类型断言失败: %T", manager.lockService)
	}
	if got := fakeLock.releaseCount(); got != 1 {
		t.Errorf("Release 调用次数 = %d，期望 1", got)
	}
	// 验证 Warn 日志记录了 release 失败（spec Task 21 修复点：旧实现无任何日志）
	if !handler.hasLevelMessage(slog.LevelWarn, "release") {
		t.Error("abortStartLocked Release 失败时应记录 slog.Warn（含 'release' 关键字），实际未记录")
	}
}

// TestStop_ReleaseFailure_JoinsError 验证 Path 3：
// Stop（可返回函数）调用 Release 失败时，releaseErr 应 join 进 stopErr 返回。
//
// 测试前置：manager 注入 fakeLock（Release 返回 releaseErr），状态设为 Running
// 测试步骤：调用 Stop
// 期待结果：返回错误包含 releaseErr；Release 被调用一次
//
// 修复场景（spec Task 21）：旧实现 `_ = Release(...)` 静默丢弃错误，
// Stop 只返回 stopMotionAxes 错误。修复后用 errors.Join 聚合，确保 Release 失败可识别。
func TestStop_ReleaseFailure_JoinsError(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	releaseErr := errors.New("release denied: held by other")
	manager.lockService = &fakeTraversalLockService{releaseErr: releaseErr}
	manager.mu.Lock()
	manager.config = traversal.Config{TaskID: "task-stop"}
	manager.status = traversal.Status{TaskID: "task-stop", State: traversal.StateRunning}
	manager.mu.Unlock()

	err := manager.Stop()
	if err == nil {
		t.Fatal("Stop 应在 Release 失败时返回错误，实际返回 nil")
	}
	// 验证 releaseErr 可识别（spec Task 21 修复点：旧实现 Release 错误被丢弃，此项红灯）
	if !errors.Is(err, releaseErr) {
		t.Errorf("Stop 错误应包含 release 错误 %v（errors.Join 聚合），实际: %v", releaseErr, err)
	}
	// 验证 Release 被调用一次
	fakeLock, ok := manager.lockService.(*fakeTraversalLockService)
	if !ok {
		t.Fatalf("manager.lockService 类型断言失败: %T", manager.lockService)
	}
	if got := fakeLock.releaseCount(); got != 1 {
		t.Errorf("Release 调用次数 = %d，期望 1", got)
	}
}

// TestStop_ReleaseSuccess_NoExtraError 验证 Path 3 正常路径：
// Release 成功时，Stop 不应附加 nil 错误（errors.Join 忽略 nil），返回 nil。
//
// 测试前置：manager 注入 fakeLock（Release 返回 nil），状态设为 Running
// 测试步骤：调用 Stop
// 期待结果：返回 nil；Release 被调用一次
func TestStop_ReleaseSuccess_NoExtraError(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.lockService = &fakeTraversalLockService{} // releaseErr = nil
	manager.mu.Lock()
	manager.config = traversal.Config{TaskID: "task-stop-ok"}
	manager.status = traversal.Status{TaskID: "task-stop-ok", State: traversal.StateRunning}
	manager.mu.Unlock()

	err := manager.Stop()
	if err != nil {
		t.Errorf("Stop Release 成功时应返回 nil，实际: %v", err)
	}
	fakeLock, ok := manager.lockService.(*fakeTraversalLockService)
	if !ok {
		t.Fatalf("manager.lockService 类型断言失败: %T", manager.lockService)
	}
	if got := fakeLock.releaseCount(); got != 1 {
		t.Errorf("Release 调用次数 = %d，期望 1", got)
	}
}

func TestStop_RemovesTemporaryCheckpoint(t *testing.T) {
	checkpointStore := newFakeCheckpointStore()
	csvPath := "D:/out/task-stop.csv"
	checkpointPath := traversal.ResolveCheckpointPathFromCSV(csvPath)
	if err := checkpointStore.Write(checkpointPath, []byte(`{"version":2}`)); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	manager := NewTraversalManager(nil, nil, nil, nil, checkpointStore)
	manager.lockService = &fakeTraversalLockService{}
	manager.mu.Lock()
	manager.config = traversal.Config{TaskID: "task-stop"}
	manager.status = traversal.Status{TaskID: "task-stop", State: traversal.StateRunning, CSVPath: csvPath}
	manager.mu.Unlock()

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if exists, err := checkpointStore.Stat(checkpointPath); err != nil || exists {
		t.Fatalf("stopped traversal must not retain checkpoint: exists=%v err=%v", exists, err)
	}
}
