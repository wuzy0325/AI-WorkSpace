package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/core/motion"
	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/ports"
	"windlabx4/services/api-go/pkg/wiring"
)

type delayedLatestDataReader struct {
	calls int
	seq   int64
}

func (r *delayedLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	if r.calls <= 2 {
		return device.DataPayload{}, false
	}
	r.seq++
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      r.seq,
		Channels:       []float64{12.5},
		ChannelIndices: []int{0},
	}, true
}

func (r *delayedLatestDataReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

type stoppingLatestDataReader struct {
	calls int
}

func (r *stoppingLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      int64(r.calls),
		Channels:       []float64{float64(r.calls)},
		ChannelIndices: []int{0},
	}, true
}

func (*stoppingLatestDataReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

type waitingLatestDataReader struct {
	once    sync.Once
	started chan struct{}
}

func (r *waitingLatestDataReader) GetLatestData(string) (device.DataPayload, bool) {
	r.once.Do(func() { close(r.started) })
	return device.DataPayload{}, false
}

func (*waitingLatestDataReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

type retryLatestDataReader struct {
	calls int
}

func (r *retryLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	value := 1.0
	if r.calls > 1 {
		value = 1500
	}
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      int64(r.calls),
		Channels:       []float64{value},
		ChannelIndices: []int{0},
	}, true
}

func (*retryLatestDataReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// resumableAcquisitionController 测试用 ports.AcquisitionController 实现，
// 支持运行时切换 acquiring / connected 状态，模拟"停采后恢复"与"掉线后重连"。
// 用 mutex 保护字段，避免与测试主 goroutine 修改并发时的数据竞争。
type resumableAcquisitionController struct {
	mu        sync.Mutex
	acquiring bool
	connected bool
}

func (c *resumableAcquisitionController) IsConnected(string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}
func (c *resumableAcquisitionController) IsAcquiring(string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.acquiring
}

func (c *resumableAcquisitionController) DeviceName(id string) string { return id }
func (*resumableAcquisitionController) StartAcquisition(string) error { return nil }

// AcquisitionStatus 实现 ports.AcquisitionController：
// 未连接 → ReconnectRequired；已连接且 acquiring → Acquiring；否则 → Stopped。
func (c *resumableAcquisitionController) AcquisitionStatus(id string) ports.AcquisitionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected {
		return ports.AcquisitionStatus{State: ports.AcquisitionReconnectRequired, Name: id}
	}
	if c.acquiring {
		return ports.AcquisitionStatus{State: ports.AcquisitionAcquiring, Name: id}
	}
	return ports.AcquisitionStatus{State: ports.AcquisitionStopped, Name: id}
}

func (c *resumableAcquisitionController) SetAcquiring(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquiring = v
}

func (c *resumableAcquisitionController) SetConnected(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = v
}

// perDeviceController 测试用 AcquisitionController：按设备独立维护 acquiring/connected，
// 用于多设备"一台停采、一台正常"的等待恢复场景。
type perDeviceController struct {
	mu        sync.Mutex
	acquiring map[string]bool
	connected map[string]bool
}

func (c *perDeviceController) IsConnected(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected[id]
}

func (c *perDeviceController) IsAcquiring(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.acquiring[id]
}

func (c *perDeviceController) DeviceName(id string) string { return id }
func (*perDeviceController) StartAcquisition(string) error { return nil }

func (c *perDeviceController) AcquisitionStatus(id string) ports.AcquisitionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected[id] {
		return ports.AcquisitionStatus{State: ports.AcquisitionReconnectRequired, Name: id}
	}
	if c.acquiring[id] {
		return ports.AcquisitionStatus{State: ports.AcquisitionAcquiring, Name: id}
	}
	return ports.AcquisitionStatus{State: ports.AcquisitionStopped, Name: id}
}

func (c *perDeviceController) Set(id string, acquiring, connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquiring[id] = acquiring
	c.connected[id] = connected
}

func TestCollectAveragedSamplesWaitsForDelayedFirstData(t *testing.T) {
	reader := &delayedLatestDataReader{}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-1"}

	values, err := manager.collectAveragedSamples("trav-1", []deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v", err)
	}
	if got := values[0]; got != 12.5 {
		t.Fatalf("averaged channel 0 = %v, want 12.5", got)
	}
	if reader.calls != 4 {
		t.Fatalf("GetLatestData calls = %d, want 4", reader.calls)
	}
}

// TestCollectAveragedSamplesWaitsWhenAcquisitionStopsAndResumes 回归测试：
// 采样过程中设备停止采集后重启采集，traversal 应当继续完成本点采样，而不是判失败。
//
// 测试前置：
//   - stoppingLatestDataReader：每次调用返回递增 Timestamp 的新帧
//   - resumableAcquisitionController：初始 acquiring=false，模拟用户已停采集
//
// 测试步骤：
//   - 异步启动 collectAveragedSamples（目标 2 个样本）
//   - 等待 200ms 模拟用户停采集的间隙
//   - 将 acquiring 切回 true 模拟用户重启采集
//
// 期待结果：
//   - 不返回错误，values 与 reader 提供的均值一致
//   - elapsed >= 200ms（证明确实在等待恢复，而非立即完成）
func TestCollectAveragedSamplesWaitsWhenAcquisitionStopsAndResumes(t *testing.T) {
	reader := &stoppingLatestDataReader{}
	controller := &resumableAcquisitionController{acquiring: false, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-resume", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	type result struct {
		values map[int]float64
		err    error
	}
	resultCh := make(chan result, 1)
	start := time.Now()
	go func() {
		values, err := manager.collectAveragedSamples("trav-resume",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
		resultCh <- result{values, err}
	}()

	// 200ms 后恢复采集，验证 traversal 等待期间不会判失败
	time.Sleep(200 * time.Millisecond)
	controller.SetAcquiring(true)

	select {
	case r := <-resultCh:
		elapsed := time.Since(start)
		if r.err != nil {
			t.Fatalf("expected sampling to resume and complete, got error: %v", r.err)
		}
		if elapsed < 200*time.Millisecond {
			t.Fatalf("elapsed = %v, want >= 200ms (should have waited for acquisition resume)", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete within 5s after acquisition resumed")
	}
}

// TestCollectAveragedSamplesBriefStopsDoNotAccumulate 回归测试（spec v4）：
// 单点采样中设备多次"短暂停采-恢复"均不应判失败——设备异常一律无限期等待
// （无累计、无时间预算），恢复后继续完成本点采样。
func TestCollectAveragedSamplesBriefStopsDoNotAccumulate(t *testing.T) {
	reader := &stoppingLatestDataReader{}
	controller := &resumableAcquisitionController{acquiring: false, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-brief-stops", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	type result struct {
		values map[int]float64
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		values, err := manager.collectAveragedSamples("trav-brief-stops",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 20)
		resultCh <- result{values, err}
	}()

	// 多段短暂停采-恢复（设备异常 = 类暂停无限期等待，无 60s 累计超时）。
	for i := 0; i < 3; i++ {
		time.Sleep(250 * time.Millisecond)
		controller.SetAcquiring(true)
		time.Sleep(100 * time.Millisecond)
		controller.SetAcquiring(false)
	}
	// 最后保持采集，让采样凑满 samplesPerPoint 完成。
	controller.SetAcquiring(true)

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("brief recoverable stops must not fail the point, got error: %v", r.err)
		}
		if r.values[0] <= 0 {
			t.Fatalf("averaged channel 0 = %v, want > 0", r.values[0])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete within 5s")
	}
}

// TestCollectAveragedSamplesStoppedWaitsIndefinitelyThenResumes 回归反转（spec v4）：
// 设备保持 STOPPED 不判失败（移除旧 60s 未采集超时），无限期等待；
// 重启采集后恢复并完成采样。
func TestCollectAveragedSamplesStoppedWaitsIndefinitelyThenResumes(t *testing.T) {
	reader := &stoppingLatestDataReader{}
	controller := &resumableAcquisitionController{acquiring: false, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-stopped-wait", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	resultCh := make(chan error, 1)
	go func() {
		_, err := manager.collectAveragedSamples("trav-stopped-wait",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
		resultCh <- err
	}()

	// 设备长时间保持停采（远超旧 300ms 测试覆盖阈值）→ 不应判失败，仍在等待。
	time.Sleep(700 * time.Millisecond)
	select {
	case err := <-resultCh:
		t.Fatalf("stopped device must wait indefinitely, got error: %v", err)
	default:
	}

	// 重启采集 → 恢复完成。
	controller.SetAcquiring(true)
	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("sampling must resume after acquisition restart, got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete after resume")
	}
}

// TestCollectAveragedSamplesAllowsPermanentTraversalPause 回归测试：
// 遍历暂停无限期等待，恢复且设备重启采集后完成采样。
func TestCollectAveragedSamplesAllowsPermanentTraversalPause(t *testing.T) {
	reader := &stoppingLatestDataReader{}
	controller := &resumableAcquisitionController{acquiring: false, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-permanent-pause", State: traversal.StatePaused}
	manager.isPaused = true
	manager.SetAcquisitionController(controller)

	resultCh := make(chan error, 1)
	go func() {
		_, err := manager.collectAveragedSamples("trav-permanent-pause",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
		resultCh <- err
	}()

	// 暂停态（设备也未采集）无限期等待，不触发任何失败。
	time.Sleep(300 * time.Millisecond)
	manager.mu.Lock()
	manager.isPaused = false
	manager.status.State = traversal.StateRunning
	manager.mu.Unlock()
	controller.SetAcquiring(true)

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("permanent traversal pause must be resumable, got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete after traversal resumed")
	}
}

// staleAfterPauseReader 验证暂停后首个样本不得并入暂停前已就绪的旧帧：
//   - dev-a 首次读返回 100（进入 pending），之后返回 aValue（测试在暂停期间切为 200）；
//   - dev-b 在 bOn=true 前不产帧，使 dev-a 已就绪但样本始终凑不齐。
type staleAfterPauseReader struct {
	mu     sync.Mutex
	aRead  chan struct{}
	aValue float64
	bOn    bool
	aCalls int
	bCalls int
}

func newStaleAfterPauseReader() *staleAfterPauseReader {
	return &staleAfterPauseReader{aRead: make(chan struct{}), aValue: 100}
}

func (r *staleAfterPauseReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if deviceID == "dev-a" {
		r.aCalls++
		value := 100.0
		if r.aCalls > 1 {
			value = r.aValue
		}
		if r.aCalls == 1 {
			close(r.aRead)
		}
		return device.DataPayload{DeviceID: deviceID, Timestamp: int64(r.aCalls), Channels: []float64{value}, ChannelIndices: []int{0}}, true
	}
	if !r.bOn {
		return device.DataPayload{}, false
	}
	r.bCalls++
	return device.DataPayload{DeviceID: deviceID, Timestamp: int64(r.bCalls), Channels: []float64{1}, ChannelIndices: []int{0}}, true
}

func (*staleAfterPauseReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// TestCollectAveragedSamplesPauseDoesNotReuseStalePendingFrame 回归测试：
// 暂停期间当前样本已就绪的旧帧（fresh/pending）必须被丢弃，
// 恢复后首个样本从暂停后的新帧重新起算，不得把暂停前旧值并入均值。
func TestCollectAveragedSamplesPauseDoesNotReuseStalePendingFrame(t *testing.T) {
	reader := newStaleAfterPauseReader()
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-pause-stale", State: traversal.StateRunning}

	type result struct {
		values map[int]float64
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		values, err := manager.collectAveragedSamples("trav-pause-stale",
			[]deviceChannelGroup{
				{deviceID: "dev-a", keys: []int{0}, hwIndices: []int{0}},
				{deviceID: "dev-b", keys: []int{1}, hwIndices: []int{0}},
			}, 2)
		resultCh <- result{values, err}
	}()

	// 等 dev-a 已把旧值 100 写入 pending，且 dev-b 未产帧使样本凑不齐
	<-reader.aRead
	time.Sleep(50 * time.Millisecond)
	manager.mu.Lock()
	manager.isPaused = true
	manager.status.State = traversal.StatePaused
	manager.mu.Unlock()

	// 暂停期间切换 dev-a 值，模拟换探头/设备重连
	time.Sleep(50 * time.Millisecond)
	reader.mu.Lock()
	reader.aValue = 200
	reader.bOn = true
	reader.mu.Unlock()

	manager.mu.Lock()
	manager.isPaused = false
	manager.status.State = traversal.StateRunning
	manager.mu.Unlock()

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("collectAveragedSamples returned error: %v", r.err)
		}
		// 通道 0 对应 dev-a：两个样本都应为 200，暂停前的 100 不得并入
		if got := r.values[0]; got != 200 {
			t.Fatalf("averaged channel 0 = %v, want 200 (pre-pause stale frame must not be merged into post-resume sample)", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete within 5s")
	}
}

// TestCollectAveragedSamplesReturnsCancelledWhenAcquisitionStaysStoppedAndTaskCancelled 回归测试：
// 设备持续未采集且用户停止 traversal 时，等待恢复循环应即时响应 isTaskCancelled，
// 立即返回 "acquisition cancelled" 错误，而不是无限挂起。
//
// 测试前置：
//   - stoppingLatestDataReader：持续返回新帧
//   - resumableAcquisitionController：acquiring=false 且不再恢复
//   - manager.isStopped=true：模拟用户停止 traversal
//
// 测试步骤：
//   - 直接调用 collectAveragedSamples（同步）
//
// 期待结果：
//   - 返回非 nil 错误
//   - elapsed < 1s（cancellation 应即时生效，不应等待 stallDeadline）
func TestCollectAveragedSamplesReturnsCancelledWhenAcquisitionStaysStoppedAndTaskCancelled(t *testing.T) {
	reader := &stoppingLatestDataReader{}
	controller := &resumableAcquisitionController{acquiring: false, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-cancel", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	// 模拟用户停止 traversal——isTaskCancelled 据此返回 true
	manager.mu.Lock()
	manager.isStopped = true
	manager.mu.Unlock()

	start := time.Now()
	_, err := manager.collectAveragedSamples("trav-cancel",
		[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if elapsed >= 1*time.Second {
		t.Fatalf("elapsed = %v, want < 1s (cancellation should be prompt, not wait for stallDeadline)", elapsed)
	}
}

func TestRunCurrentPointKeepsStoppedStateWhenSamplingIsCancelled(t *testing.T) {
	reader := &waitingLatestDataReader{started: make(chan struct{})}
	manager := NewTraversalManager(reader, &mockMotionAccess{}, nil, nil, nil)
	config := traversal.Config{
		TaskID:          "trav-stop-during-sampling",
		DeviceID:        "dev-1",
		Channels:        []int{0},
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0}},
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
	}
	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() {
		manager.session.MarkDone()
		_ = manager.lockService.Release(traversalLockResource, config.TaskID)
	})

	result := make(chan error, 1)
	go func() { result <- manager.RunCurrentPoint() }()
	select {
	case <-reader.started:
	case <-time.After(3 * time.Second):
		t.Fatal("RunCurrentPoint did not enter sampling")
	}

	manager.mu.Lock()
	manager.isStopped = true
	manager.status.State = traversal.StateStopped
	manager.session.Cancel()
	manager.mu.Unlock()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("user stop must not return a sampling failure: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunCurrentPoint did not exit after stop")
	}
	status := manager.Status()
	if status.State != traversal.StateStopped {
		t.Fatalf("state = %s, want stopped", status.State)
	}
	if status.LastError != "" {
		t.Fatalf("last error = %q, want empty", status.LastError)
	}
}

func TestRunCurrentPointDoesNotMoveWhenAcquisitionHasStopped(t *testing.T) {
	motionAccess := &originReturnMotion{moveTargets: make(map[motion.AxisName]float64)}
	manager := NewTraversalManager(&delayedLatestDataReader{}, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID:          "trav-no-move",
		DeviceID:        "dev-1",
		Channels:        []int{0},
		Path:            []traversal.Point{{X: 10}},
		SamplesPerPoint: 1,
		MotionAxes:      []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning, TotalPoints: 1}
	controller := &resumableAcquisitionController{acquiring: false, connected: true}
	manager.SetAcquisitionController(controller)

	done := make(chan error, 1)
	go func() { done <- manager.RunCurrentPoint() }()

	// 设备停采 → 点位开始进入无限期等待（spec v4），不下发运动。
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("RunCurrentPoint must wait when acquisition stopped, returned: %v", err)
	default:
	}
	if len(motionAccess.moveTargets) != 0 {
		t.Fatalf("MoveTo called while waiting for acquisition: %v", motionAccess.moveTargets)
	}

	// 恢复采集 → 继续点位流程（下发运动）。
	controller.SetAcquiring(true)
	deadline := time.Now().Add(2 * time.Second)
	for {
		motionAccess.mu.Lock()
		moved := len(motionAccess.moveTargets) > 0
		motionAccess.mu.Unlock()
		if moved {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("MoveTo not issued after acquisition resumed")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 停止遍历，让 RunCurrentPoint 有界退出（不等待运动到位）。
	manager.mu.Lock()
	manager.isStopped = true
	manager.mu.Unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("RunCurrentPoint did not exit after stop")
	}
}

func TestRunCurrentPointMoveCommandFailureStopsBoundAxes(t *testing.T) {
	motionErr := errors.New("B140 command PAC=-120000 failed")
	motionAccess := &mockMotionAccess{
		statuses: []motion.ControllerStatus{{
			ID:        "ctrl-a",
			Connected: true,
			Axes:      []motion.AxisStatus{{Name: motion.AxisZ, Moving: true}},
		}},
		moveToErr: motionErr,
	}
	manager := NewTraversalManager(&mockLatestDataReader{}, motionAccess, nil, nil, nil)
	config := traversal.Config{
		TaskID:     "move-command-failure",
		DeviceID:   "device-1",
		Channels:   []int{0},
		Path:       []traversal.Point{{Z: -60}},
		MotionAxes: []traversal.MotionAxisBinding{{ControllerID: "ctrl-a", Axis: "Z"}},
	}
	snapshot := traversal.TraversalRunSnapshot{
		Config:             config,
		TotalPoints:        1,
		BoundControllerIDs: []string{"ctrl-a"},
	}
	session := newTraversalRunSession(context.Background(), config.TaskID, snapshot)
	session.managedOpts = &ManagedSessionOptions{ProbeID: Probe2}
	manager.mu.Lock()
	manager.config = config
	manager.session = session
	manager.status = traversal.Status{
		TaskID:       config.TaskID,
		State:        traversal.StateRunning,
		TotalPoints:  1,
		CurrentPoint: 0,
	}
	manager.mu.Unlock()

	err := manager.RunCurrentPoint()
	if !errors.Is(err, motionErr) && !contains(err.Error(), motionErr.Error()) {
		t.Fatalf("expected move error, got %v", err)
	}
	if len(motionAccess.stopCalls) == 0 {
		t.Fatal("MoveTo failure must stop bound axes before returning")
	}
	if got := manager.Status().State; got != traversal.StateError {
		t.Fatalf("state = %s, want error", got)
	}
}

// TestRunCurrentPointSamplingWindowIncludesRetryWait 验证 StartedAt/CompletedAt
// 严格遵循 CSV writer 表头契约——"单点总耗时"，包含验证失败的重试等待。
//
// 测试前置：
//   - retryLatestDataReader 第一次返回 1（超出 [1000,2000] 合法范围），第二次返回 1500（合法）
//   - validation 启用，OnInvalid=retry，RetryCount=1 → maxAttempts=2
//
// 测试步骤：
//   - 启动 manager 并执行 RunCurrentPoint
//   - 第一次采样验证失败 → 等待 retryWaitInterval → 第二次采样成功
//
// 期待结果：
//   - sink 收到 1 个 point
//   - CompletedAt - StartedAt >= retryWaitInterval（重试等待计入单点总耗时，与 CSV 契约一致）
func TestRunCurrentPointSamplingWindowIncludesRetryWait(t *testing.T) {
	reader := &retryLatestDataReader{}
	sink := &mockTraversalPointSink{}
	manager := NewTraversalManager(reader, &mockMotionAccess{}, sink, newMockTraversalResultStore(), wiring.NewFileCheckpointStore())
	config := traversal.Config{
		TaskID:          "trav-retry-window",
		DeviceID:        "sim-1",
		Channels:        []int{0},
		ChannelLabels:   map[int]string{0: "P1"},
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0}},
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
		SavePath:        t.TempDir(),
		SaveFileName:    "retry-window",
	}
	manager.SetValidation(&traversal.DataValidationConfig{
		Enabled:    true,
		OnInvalid:  "retry",
		RetryCount: 1,
		PressureRange: map[string]*traversal.PressureRange{
			"P1": {Min: 1000, Max: 2000},
		},
	})

	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })
	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	points, _, _ := sink.snapshot()
	if len(points) != 1 {
		t.Fatalf("written points = %d, want 1", len(points))
	}
	window := time.Duration(points[0].CompletedAt-points[0].StartedAt) * time.Millisecond
	if window < retryWaitInterval {
		t.Fatalf("single-point total time = %v, must include retry wait %v (per CSV contract)", window, retryWaitInterval)
	}
}

type originReturnMotion struct {
	mu          sync.Mutex
	moveTargets map[motion.AxisName]float64
	statuses    []motion.ControllerStatus
}

func (m *originReturnMotion) StatusAll(context.Context) []motion.ControllerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statuses != nil {
		statuses := append([]motion.ControllerStatus(nil), m.statuses...)
		for i := range statuses {
			statuses[i].Axes = append([]motion.AxisStatus(nil), statuses[i].Axes...)
		}
		return statuses
	}
	return []motion.ControllerStatus{{
		ID:        "mc-1",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0, Moving: false},
			{Name: motion.AxisY, Position: 0, Moving: false},
		},
	}}
}

func (m *originReturnMotion) MoveTo(_ context.Context, _ string, axis motion.AxisName, position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.moveTargets[axis] = position
	return nil
}

func (m *originReturnMotion) Stop(_ context.Context, controllerID string, axisName motion.AxisName) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for controllerIndex := range m.statuses {
		if m.statuses[controllerIndex].ID != controllerID {
			continue
		}
		for axisIndex := range m.statuses[controllerIndex].Axes {
			if m.statuses[controllerIndex].Axes[axisIndex].Name == axisName {
				m.statuses[controllerIndex].Axes[axisIndex].Moving = false
			}
		}
	}
	return nil
}

func (m *originReturnMotion) EmergencyStop(_ context.Context, controllerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for controllerIndex := range m.statuses {
		if m.statuses[controllerIndex].ID != controllerID {
			continue
		}
		for axisIndex := range m.statuses[controllerIndex].Axes {
			m.statuses[controllerIndex].Axes[axisIndex].Moving = false
		}
	}
	return nil
}

func TestReturnToOriginMovesConfiguredAxesToZero(t *testing.T) {
	motionAccess := &originReturnMotion{moveTargets: make(map[motion.AxisName]float64)}
	manager := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID: "trav-return-origin",
		Path:   []traversal.Point{{X: 1, Y: 2, Z: math.NaN(), U: math.NaN()}},
		MotionAxes: []traversal.MotionAxisBinding{
			{ControllerID: "mc-1", Axis: "X"},
			{ControllerID: "mc-1", Axis: "Y"},
		},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning}

	if err := manager.returnToOrigin(manager.config.TaskID, 1); err != nil {
		t.Fatalf("returnToOrigin returned error: %v", err)
	}
	for _, axis := range []motion.AxisName{motion.AxisX, motion.AxisY} {
		if got, ok := motionAccess.moveTargets[axis]; !ok || got != 0 {
			t.Fatalf("MoveTo target for %s = %v (called=%v), want 0", axis, got, ok)
		}
	}
}

func TestReturnToOriginSkipsAxesUnusedByPath(t *testing.T) {
	motionAccess := &originReturnMotion{moveTargets: make(map[motion.AxisName]float64)}
	manager := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID: "trav-return-origin-line",
		Path:   []traversal.Point{{X: 10, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning}

	if err := manager.returnToOrigin(manager.config.TaskID, 1); err != nil {
		t.Fatalf("returnToOrigin returned error: %v", err)
	}
	if len(motionAccess.moveTargets) != 1 || motionAccess.moveTargets[motion.AxisX] != 0 {
		t.Fatalf("return should only move path axis X, got %#v", motionAccess.moveTargets)
	}
}

func TestReturnToOriginWaitsForResume(t *testing.T) {
	motionAccess := &originReturnMotion{
		moveTargets: make(map[motion.AxisName]float64),
		statuses: []motion.ControllerStatus{{
			ID: "mc-1", Connected: true,
			Axes: []motion.AxisStatus{{Name: motion.AxisX, Position: 1, Moving: true}},
		}},
	}
	manager := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID:     "trav-return-origin-pause",
		Path:       []traversal.Point{{X: 1, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
		MotionAxes: []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning}

	done := make(chan error, 1)
	go func() { done <- manager.returnToOrigin(manager.config.TaskID, 1) }()
	time.Sleep(2 * motionCompletePoll)
	if err := manager.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	time.Sleep(2 * motionCompletePoll)
	select {
	case err := <-done:
		t.Fatalf("returnToOrigin exited while paused: %v", err)
	default:
	}

	motionAccess.mu.Lock()
	motionAccess.statuses[0].Axes[0] = motion.AxisStatus{Name: motion.AxisX, Position: 0}
	motionAccess.mu.Unlock()
	time.Sleep(2 * motionCompletePoll)
	select {
	case err := <-done:
		t.Fatalf("returnToOrigin completed while paused at origin: %v", err)
	default:
	}
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("returnToOrigin after resume: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("returnToOrigin did not continue after resume")
	}
}

// TestRunCurrentPointSkipLastPersistsCompletedState 回归测试（B1）：
// 最后一个点被 skip（validation OnInvalid=skip 且校验失败）时，
// 持久化到 result store 的最终状态必须是 StateCompleted，
// 不得是置 Completed 之前捕获的过期快照（StateSaving/StateAcquiring 等）。
//
// 修复前 skip 分支在锁内先捕获 saveStatus，returnToOrigin 之后才置 Completed，
// 导致 store.Save 写入的是旧状态快照；修复后镜像正常分支，先置 Completed 再捕获。
func TestRunCurrentPointSkipLastPersistsCompletedState(t *testing.T) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	config := traversal.Config{
		TaskID:          "trav-skip-last-point",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5"},
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0}},
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
		SavePath:        t.TempDir(),
		SaveFileName:    "skip-last",
	}
	// 校验恒失败（P1 读数 1 不在 [1000,2000]）+ OnInvalid=skip → 唯一（最后）点走 skip 分支
	manager.SetValidation(&traversal.DataValidationConfig{
		Enabled:   true,
		OnInvalid: "skip",
		PressureRange: map[string]*traversal.PressureRange{
			"P1": {Min: 1000, Max: 2000},
		},
	})

	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	saved, ok := store.Get(config.TaskID)
	if !ok {
		t.Fatal("expected final status persisted to result store when last point is skipped")
	}
	if saved.State != traversal.StateCompleted {
		t.Fatalf("persisted state = %q, want %q (skip 末点不得持久化置 Completed 之前的过期快照)",
			saved.State, traversal.StateCompleted)
	}
}

// returnOriginFailMotion 仅在目标为 0（回零）时让 MoveTo 失败，
// 非零目标的点位运动正常，用于构造"采集成功但回零失败"的场景。
type returnOriginFailMotion struct {
	mockMotionAccess
}

func (m *returnOriginFailMotion) MoveTo(ctx context.Context, id string, axis motion.AxisName, position float64) error {
	if position == 0 {
		return fmt.Errorf("simulated return-to-origin failure")
	}
	return m.mockMotionAccess.MoveTo(ctx, id, axis, position)
}

// TestRunCurrentPointReturnToOriginFailureStillCompletes 回归测试：
// 数据全部采完后回零失败（MoveTo(0) 注入错误）只应记录 Status.Warning，
// 最终状态仍为 Completed，LastError/LastErrorCode 清空，
// result store 持久化的也是完成态——回零失败不得把已采完的测试判为失败。
func TestRunCurrentPointReturnToOriginFailureStillCompletes(t *testing.T) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &returnOriginFailMotion{mockMotionAccess{
		statuses: []motion.ControllerStatus{{
			ID: "mc-1", Connected: true,
			Axes: []motion.AxisStatus{
				{Name: motion.AxisX, Position: 10, Homed: true, Moving: false},
				{Name: motion.AxisY, Position: 0, Homed: true, Moving: false},
				{Name: motion.AxisZ, Position: 0, Homed: true, Moving: false},
			},
		}},
	}}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	config := traversal.Config{
		TaskID:          "trav-return-origin-fail",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5"},
		Path:            []traversal.Point{{X: 10, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
		SavePath:        t.TempDir(),
		SaveFileName:    "return-origin-fail",
	}

	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v（回零失败应降级为 warning，不应报错）", err)
	}

	saved, ok := store.Get(config.TaskID)
	if !ok {
		t.Fatal("expected final status persisted to result store")
	}
	if saved.State != traversal.StateCompleted {
		t.Fatalf("persisted state = %q, want %q（数据已采完，回零失败不判测试失败）",
			saved.State, traversal.StateCompleted)
	}
	if saved.Warning == "" {
		t.Fatal("expected Warning recorded for failed return-to-origin")
	}
	if saved.LastError != "" || saved.LastErrorCode != "" {
		t.Fatalf("LastError/LastErrorCode should be cleared, got %q / %q", saved.LastError, saved.LastErrorCode)
	}
}

// TestCompleteAfterReturnToOriginSemantics 单元测试：
// 用户停止/取消（errReturnToOriginAborted）原样透传；运动失败降级为 Warning。
func TestCompleteAfterReturnToOriginSemantics(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "t", State: traversal.StateRunning}

	if err := manager.completeAfterReturnToOrigin("t", nil); err != nil {
		t.Fatalf("nil error should pass through, got %v", err)
	}
	if err := manager.completeAfterReturnToOrigin("t", errReturnToOriginAborted); !errors.Is(err, errReturnToOriginAborted) {
		t.Fatalf("abort should pass through, got %v", err)
	}
	if manager.status.Warning != "" {
		t.Fatalf("abort path must not record Warning, got %q", manager.status.Warning)
	}

	manager.setErrorLocked("return to origin timed out", traversal.ErrMotionTimeout)
	if err := manager.completeAfterReturnToOrigin("t", fmt.Errorf("return to origin timed out")); err != nil {
		t.Fatalf("motion failure should be downgraded to warning, got %v", err)
	}
	if manager.status.Warning != "return to origin timed out" {
		t.Fatalf("Warning = %q, want %q", manager.status.Warning, "return to origin timed out")
	}
	if manager.status.LastError != "" || manager.status.LastErrorCode != "" {
		t.Fatalf("error state should be cleared, got %q / %q", manager.status.LastError, manager.status.LastErrorCode)
	}
}

// slowFrameLatestDataReader 按真实时间节流产帧：每 frameInterval 产出一帧新数据，
// 模拟低频设备（如 2Hz 帧率）。帧时间戳 = 已产出帧序号，保证帧去重语义生效。
type slowFrameLatestDataReader struct {
	frameInterval time.Duration
	start         time.Time
	value         float64
}

func (r *slowFrameLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	frame := int64(time.Since(r.start) / r.frameInterval)
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      frame,
		Channels:       []float64{r.value},
		ChannelIndices: []int{0},
	}, true
}

func (*slowFrameLatestDataReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// TestCollectAveragedSamplesLowFrameRateNotKilledByOverallTimeout 回归测试：
// 低频设备（800ms 一帧）采 4 个样本需要约 2.4s——旧的 2s 固定总体超时会必然失败；
// 改为停滞超时后，只要持续有新帧/新样本就不判超时，应正常完成。
func TestCollectAveragedSamplesLowFrameRateNotKilledByOverallTimeout(t *testing.T) {
	reader := &slowFrameLatestDataReader{frameInterval: 800 * time.Millisecond, start: time.Now(), value: 42}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-low-fps"}

	start := time.Now()
	values, err := manager.collectAveragedSamples("trav-low-fps",
		[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 4)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v（低频设备多样本采集不应被总体超时杀死）", err)
	}
	if got := values[0]; got != 42 {
		t.Fatalf("averaged channel 0 = %v, want 42", got)
	}
	if elapsed < 2*time.Second {
		t.Fatalf("elapsed = %v, want >= 2s（4 帧 × 800ms 应超过旧的 2s 总体超时，证明语义已改）", elapsed)
	}
}

// oneFrameThenSilentReader 只产出一帧后永远不再出新帧，用于验证停滞超时仍会触发。
type oneFrameThenSilentReader struct{}

func (*oneFrameThenSilentReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      1,
		Channels:       []float64{1},
		ChannelIndices: []int{0},
	}, true
}

func (*oneFrameThenSilentReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// TestCollectAveragedSamplesStallTimeoutWhenNoNewSample 回归测试：
// 凑不齐新样本（设备静默/帧不再更新）时，停滞超时仍应在 acquisitionStallTimeout 后报错，
// 而不是因为"完成样本重置"语义退化为永不超时。
func TestCollectAveragedSamplesStallTimeoutWhenNoNewSample(t *testing.T) {
	reader := &oneFrameThenSilentReader{}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-stall"}

	start := time.Now()
	_, err := manager.collectAveragedSamples("trav-stall",
		[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected stall timeout error when no new complete sample arrives")
	}
	if elapsed < acquisitionStallTimeout || elapsed > acquisitionStallTimeout+2*time.Second {
		t.Fatalf("elapsed = %v, want ≈ acquisitionStallTimeout(%v)", elapsed, acquisitionStallTimeout)
	}
}

// TestCollectAveragedSamplesNotAcquiringWaitClearsAllPending 回归测试：
// 多设备采样中一台停采触发等待，恢复后**所有设备**（含一直正常的设备）的 fresh/pending
// 都必须重建——正常设备等待前已就绪的 pending 旧值不得并入恢复后首个样本。
func TestCollectAveragedSamplesNotAcquiringWaitClearsAllPending(t *testing.T) {
	reader := newStaleAfterPauseReader()
	controller := &perDeviceController{
		acquiring: map[string]bool{"dev-a": true, "dev-b": true},
		connected: map[string]bool{"dev-a": true, "dev-b": true},
	}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-wait-clear", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	type result struct {
		values map[int]float64
		err    error
	}
	resultCh := make(chan result, 1)
	go func() {
		values, err := manager.collectAveragedSamples("trav-wait-clear",
			[]deviceChannelGroup{
				{deviceID: "dev-a", keys: []int{0}, hwIndices: []int{0}},
				{deviceID: "dev-b", keys: []int{1}, hwIndices: []int{0}},
			}, 2)
		resultCh <- result{values, err}
	}()

	// 等 dev-a 已把旧值 100 写入 pending、dev-b 未产帧使样本凑不齐（fresh[a] 持久）。
	<-reader.aRead
	time.Sleep(50 * time.Millisecond)

	// dev-b 停采 → 进入等待，不判失败。
	controller.Set("dev-b", false, true)
	time.Sleep(100 * time.Millisecond)
	select {
	case r := <-resultCh:
		t.Fatalf("must wait while device stopped, got: values=%v err=%v", r.values, r.err)
	default:
	}

	// 等待期间 dev-a 值切换为 200（模拟正常设备在等待期间持续采集出更晚帧）。
	reader.mu.Lock()
	reader.aValue = 200
	reader.bOn = true
	reader.mu.Unlock()

	// dev-b 恢复采集 → 等待结束；恢复后首个样本必须用 dev-a 的新值 200。
	controller.Set("dev-b", true, true)

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("collectAveragedSamples returned error: %v", r.err)
		}
		if got := r.values[0]; got != 200 {
			t.Fatalf("averaged channel 0 = %v, want 200 (pre-wait stale frame of acquiring device must not be merged)", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete")
	}
}

// reconnectResetReader 测试用 LatestDataReader：
//   - after=false：时间戳为大值（1000+seq），建立"重连前"旧基线；
//   - after=true：时间戳回绕为小值（seq），模拟设备重连后计数器复位。
type reconnectResetReader struct {
	mu    sync.Mutex
	after bool
	seq   int64
}

func (r *reconnectResetReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	if !r.after {
		return device.DataPayload{DeviceID: deviceID, Timestamp: 1000 + r.seq, Channels: []float64{100}, ChannelIndices: []int{0}}, true
	}
	return device.DataPayload{DeviceID: deviceID, Timestamp: r.seq, Channels: []float64{200}, ChannelIndices: []int{0}}, true
}

func (r *reconnectResetReader) setAfter(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.after = v
}

func (*reconnectResetReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// TestCollectAveragedSamplesReconnectWaitsAndRebaselines 回归测试（spec v4）：
// 设备掉线（ReconnectRequired）→ 无限期等待不判失败；
// 重连后时间戳回绕为小值也能正常出样本——恢复路径重置 lastTimestamps=-1
// 并丢弃首帧，否则新帧永不够新会被误判"在采但无新帧"（10s 停滞）。
func TestCollectAveragedSamplesReconnectWaitsAndRebaselines(t *testing.T) {
	reader := &reconnectResetReader{}
	controller := &resumableAcquisitionController{acquiring: true, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-reconnect", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	resultCh := make(chan error, 1)
	go func() {
		_, err := manager.collectAveragedSamples("trav-reconnect",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 50)
		resultCh <- err
	}()

	// 设备先采集一阵（建立旧时间戳基线 + 积累部分样本），再掉线。
	time.Sleep(150 * time.Millisecond)
	controller.SetConnected(false)
	controller.SetAcquiring(false)

	// 掉线（ReconnectRequired）无限期等待：不应判失败。
	time.Sleep(400 * time.Millisecond)
	select {
	case err := <-resultCh:
		t.Fatalf("reconnect_required must wait indefinitely, got error: %v", err)
	default:
	}

	// 重连并重启采集；时间戳回绕（计数器复位为小值）。
	reader.setAfter(true)
	controller.SetConnected(true)
	controller.SetAcquiring(true)

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("sampling must complete after reconnect (with reset timestamps), got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete after reconnect")
	}
}

// TestCollectAveragedSamplesReconnectRequiredStoppedAcquiringStillRebaselines
// 回归测试（code-review Critical-2）：
// 设备经历 reconnect_required → stopped（重连但未启动采集）→ acquiring 的典型
// 恢复路径时，等待 helper 必须保留"曾重连"信息并执行时间戳基线重建。
// 修复前 helper 只返回恢复前最后一次分类（stopped），调用方因此跳过 rebase，
// 重连后归零的新帧会继续用旧 lastTimestamps 比较并在 10s 后触发停滞超时。
func TestCollectAveragedSamplesReconnectRequiredStoppedAcquiringStillRebaselines(t *testing.T) {
	reader := &reconnectResetReader{}
	controller := &resumableAcquisitionController{acquiring: true, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-reconnect-stopped", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	resultCh := make(chan error, 1)
	go func() {
		_, err := manager.collectAveragedSamples("trav-reconnect-stopped",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 50)
		resultCh <- err
	}()

	// 设备先采集一阵（建立旧时间戳基线），再掉线。
	time.Sleep(150 * time.Millisecond)
	controller.SetConnected(false)
	controller.SetAcquiring(false)

	// 掉线（ReconnectRequired）无限期等待。
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-resultCh:
		t.Fatalf("reconnect_required must wait indefinitely, got error: %v", err)
	default:
	}

	// 重连但尚未启动采集（Stopped）：等待继续，helper 不得把曾 ReconnectRequired 降级。
	reader.setAfter(true)
	controller.SetConnected(true)
	time.Sleep(150 * time.Millisecond)
	select {
	case err := <-resultCh:
		t.Fatalf("stopped (reconnected but not acquiring) must keep waiting, got error: %v", err)
	default:
	}

	// 启动采集（Acquiring）→ 恢复；必须按"曾重连"重建时间戳基线（新帧时间戳归零）。
	controller.SetAcquiring(true)

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("sampling must complete after reconnect -> stopped -> acquiring (was-reconnect info lost), got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete after reconnect -> stopped -> acquiring")
	}
}

// reconnectStaleCacheReader 模拟 hub 在设备断开时保留旧连接缓存帧（真实
// AcquisitionHub 的 latest 在断开时不清除，GetLatestData 在重连后首帧到达前
// 仍返回旧帧）：
//   - after=false：返回旧连接帧（时间戳 1000+seq），GetLatestTimestamp 返回最新旧帧；
//   - after=true：先返回 staleReads 次旧连接缓存帧（时间戳 = 进入 ReconnectRequired
//     时捕获的 staleTimestamp，即断开前最后一帧），之后才返回时间戳归零的新帧。
type reconnectStaleCacheReader struct {
	mu         sync.Mutex
	after      bool
	seq        int64
	staleReads int
	staleTs    int64
}

func (r *reconnectStaleCacheReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.after {
		r.seq++
		r.staleTs = 1000 + r.seq
		return device.DataPayload{DeviceID: deviceID, Timestamp: r.staleTs, Channels: []float64{100}, ChannelIndices: []int{0}}, true
	}
	if r.staleReads > 0 {
		r.staleReads--
		return device.DataPayload{DeviceID: deviceID, Timestamp: r.staleTs, Channels: []float64{100}, ChannelIndices: []int{0}}, true
	}
	r.seq++
	return device.DataPayload{DeviceID: deviceID, Timestamp: r.seq, Channels: []float64{200}, ChannelIndices: []int{0}}, true
}

func (r *reconnectStaleCacheReader) GetLatestTimestamp(string) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.staleTs, r.staleTs > 0
}

func (r *reconnectStaleCacheReader) setAfter(v bool, staleReads int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.after = v
	r.staleReads = staleReads
}

// TestCollectAveragedSamplesReconnectIgnoresStaleCacheFrame 回归测试（code-review
// Critical-1）：重连后状态已恢复为 Acquiring、但 hub 连续返回若干次旧连接缓存帧
// （时间戳与进入 ReconnectRequired 时捕获一致），之后才返回时间戳归零的新帧。
// 修复前旧缓存帧会被当成"重连后首帧"建立基线，新帧（时间戳更小）全部被判为
// "同一帧"跳过，10s 后错误触发停滞超时。修复后精确旧帧被忽略，第一个不同时间戳
// 的新帧才成为新基线。
func TestCollectAveragedSamplesReconnectIgnoresStaleCacheFrame(t *testing.T) {
	reader := &reconnectStaleCacheReader{}
	controller := &resumableAcquisitionController{acquiring: true, connected: true}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-stale-cache", State: traversal.StateRunning}
	manager.SetAcquisitionController(controller)

	resultCh := make(chan error, 1)
	go func() {
		_, err := manager.collectAveragedSamples("trav-stale-cache",
			[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 50)
		resultCh <- err
	}()

	// 设备先采集一阵（建立旧时间戳基线），再掉线。
	time.Sleep(150 * time.Millisecond)
	controller.SetConnected(false)
	controller.SetAcquiring(false)

	// 掉线（ReconnectRequired）无限期等待。
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-resultCh:
		t.Fatalf("reconnect_required must wait indefinitely, got error: %v", err)
	default:
	}

	// 重连并重启采集：hub 缓存仍返回旧连接最后一帧（时间戳与捕获一致）staleReads 次，
	// 之后才返回时间戳归零的新帧——模拟"状态已恢复但新连接首帧未到"的窗口。
	reader.setAfter(true, 5)
	controller.SetConnected(true)
	controller.SetAcquiring(true)

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("sampling must complete even when hub returns stale cached frames after reconnect, got error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("collectAveragedSamples did not complete: stale cached frame timestamp locked new (reset) frames")
	}
}
