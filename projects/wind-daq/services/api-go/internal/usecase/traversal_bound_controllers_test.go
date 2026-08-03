package usecase

import (
	"context"
	"errors"
	"testing"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

// Task 9：Stop / EmergencyStop 只作用于启动快照 BoundControllerIDs 中的控制器。
//
// 隔离不变量（spec I1/I6）：probe1 的 Stop、EmergencyStop、位置超差、限位等停机命令
// 绝不发给 probe2 的控制器；双模式禁止"空绑定时操作所有已连接控制器"的回退；
// legacy single 的空绑定回退行为保持不变。

// boundControllersMotion 构造含两台控制器（各两轴）的 mockMotionAccess。
func boundControllersMotion() *mockMotionAccess {
	return &mockMotionAccess{statuses: []motion.ControllerStatus{
		{ID: "ctrl-a", Connected: true, Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Moving: true},
			{Name: motion.AxisY, Moving: true},
		}},
		{ID: "ctrl-b", Connected: true, Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Moving: true},
			{Name: motion.AxisY, Moving: true},
		}},
	}}
}

// newBoundSessionManager 构造注入了活动 session 的 manager。
// snapshotBound 为快照冻结的 BoundControllerIDs；liveAxes 为 m.config 中"当前"绑定
// （用于模拟快照与运行期配置漂移）；managed 标记 session ownership。
func newBoundSessionManager(
	motionAccess *mockMotionAccess,
	snapshotBound []string,
	liveAxes []traversal.MotionAxisBinding,
	managed bool,
) (*TraversalManager, *TraversalRunSession) {
	mgr := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	snapshot := traversal.TraversalRunSnapshot{BoundControllerIDs: snapshotBound}
	session := newTraversalRunSession(context.Background(), "task-bound", snapshot)
	if managed {
		session.managedOpts = &ManagedSessionOptions{
			ProbeID: Probe1,
			Token:   SessionToken{ProbeID: Probe1, Generation: 1},
		}
	}
	mgr.mu.Lock()
	mgr.session = session
	mgr.config = traversal.Config{TaskID: "task-bound", MotionAxes: liveAxes}
	mgr.status = traversal.Status{TaskID: "task-bound", State: traversal.StateRunning}
	mgr.mu.Unlock()
	return mgr, session
}

func stopCallControllers(m *mockMotionAccess) map[string]int {
	counts := make(map[string]int)
	for _, call := range m.stopCalls {
		counts[call.id]++
	}
	return counts
}

// 快照绑定 ctrl-a + 运行期配置漂移到 ctrl-b：Stop 必须按快照只停 ctrl-a。
func TestTraversal_BoundControllers_StopUsesSnapshotNotLiveConfig(t *testing.T) {
	motionAccess := boundControllersMotion()
	mgr, session := newBoundSessionManager(
		motionAccess,
		[]string{"ctrl-a"},
		[]traversal.MotionAxisBinding{{ControllerID: "ctrl-b", Axis: "X"}},
		false,
	)
	session.MarkDone() // 跳过 Stop 的 5s 等待（本测试不关心 loop 退出等待）

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	counts := stopCallControllers(motionAccess)
	if counts["ctrl-a"] == 0 {
		t.Fatal("快照绑定的 ctrl-a 应收到 Stop")
	}
	if counts["ctrl-b"] != 0 {
		t.Fatalf("probe2 控制器 ctrl-b 不得收到任何 Stop, got %d", counts["ctrl-b"])
	}
}

func TestTraversal_BoundControllers_PauseStopsSnapshotAxesBeforeReturning(t *testing.T) {
	motionAccess := boundControllersMotion()
	motionAccess.statusSequence = [][]motion.ControllerStatus{
		motionAccess.statuses,
		{{ID: "ctrl-a", Connected: true, Axes: []motion.AxisStatus{{Name: motion.AxisX, Moving: false}}}},
	}
	mgr, _ := newBoundSessionManager(
		motionAccess,
		[]string{"ctrl-a"},
		[]traversal.MotionAxisBinding{{ControllerID: "ctrl-a", Axis: "X"}},
		true,
	)

	if err := mgr.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if got := mgr.Status().State; got != traversal.StatePaused {
		t.Fatalf("state = %s, want paused", got)
	}
	counts := stopCallControllers(motionAccess)
	if counts["ctrl-a"] == 0 {
		t.Fatal("Pause must stop the bound controller before returning")
	}
	if counts["ctrl-b"] != 0 {
		t.Fatalf("Pause must not stop another probe controller, got %d calls", counts["ctrl-b"])
	}
}

func TestTraversal_BoundControllers_PauseEmergencyStopsWhenNormalStopFails(t *testing.T) {
	motionAccess := boundControllersMotion()
	motionAccess.stopErr = errors.New("B140 ST command failed")
	mgr, _ := newBoundSessionManager(
		motionAccess,
		[]string{"ctrl-a"},
		[]traversal.MotionAxisBinding{{ControllerID: "ctrl-a", Axis: "X"}},
		true,
	)

	err := mgr.Pause()
	if err == nil || !contains(err.Error(), motionAccess.stopErr.Error()) {
		t.Fatalf("Pause should report normal stop failure, got %v", err)
	}
	if len(motionAccess.emergencyStopCalls) != 1 || motionAccess.emergencyStopCalls[0] != "ctrl-a" {
		t.Fatalf("Pause must emergency-stop only the bound controller, got %v", motionAccess.emergencyStopCalls)
	}
	status := mgr.Status()
	if status.State != traversal.StateError {
		t.Fatalf("state = %s, want error after emergency-stop fallback", status.State)
	}
	if status.LastErrorCode != traversal.ErrMotionFailed {
		t.Fatalf("error code = %s, want %s", status.LastErrorCode, traversal.ErrMotionFailed)
	}
}

// 快照绑定 ctrl-a：EmergencyStop 只发 ctrl-a（运行期配置漂移不影响）。
func TestTraversal_BoundControllers_EmergencyStopUsesSnapshot(t *testing.T) {
	motionAccess := boundControllersMotion()
	mgr, _ := newBoundSessionManager(
		motionAccess,
		[]string{"ctrl-a"},
		[]traversal.MotionAxisBinding{{ControllerID: "ctrl-b", Axis: "X"}},
		false,
	)

	if err := mgr.emergencyStopMotionControllers(); err != nil {
		t.Fatalf("emergencyStopMotionControllers: %v", err)
	}
	for _, id := range motionAccess.emergencyStopCalls {
		if id == "ctrl-b" {
			t.Fatal("probe2 控制器 ctrl-b 不得收到 EmergencyStop")
		}
	}
	if len(motionAccess.emergencyStopCalls) != 1 || motionAccess.emergencyStopCalls[0] != "ctrl-a" {
		t.Fatalf("EmergencyStop 应只发 ctrl-a: %v", motionAccess.emergencyStopCalls)
	}
}

// 运动安全故障（急停类 verdict）路径：只急停快照中的控制器。
func TestTraversal_BoundControllers_MotionSafetyFailureScopesToSnapshot(t *testing.T) {
	motionAccess := boundControllersMotion()
	mgr, _ := newBoundSessionManager(
		motionAccess,
		[]string{"ctrl-a"},
		nil, // 运行期配置无绑定也不影响快照语义
		false,
	)

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "ctrl-a",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyLimitTriggered,
		Target:       10,
		Actual:       10,
	}
	_ = mgr.handleMotionSafetyFailure(failure)
	for _, id := range motionAccess.emergencyStopCalls {
		if id == "ctrl-b" {
			t.Fatal("限位急停不得发给 probe2 控制器 ctrl-b")
		}
	}
	if len(motionAccess.emergencyStopCalls) == 0 {
		t.Fatal("故障 probe 的 ctrl-a 应收到 EmergencyStop")
	}
}

// 运动安全故障（普通停止类 verdict）路径：只对快照控制器发 Stop。
func TestTraversal_BoundControllers_DeviationStopsOnlyBound(t *testing.T) {
	motionAccess := boundControllersMotion()
	mgr, _ := newBoundSessionManager(
		motionAccess,
		[]string{"ctrl-a"},
		nil,
		false,
	)

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "ctrl-a",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyDeviation,
		Target:       10,
		Actual:       11,
	}
	_ = mgr.handleMotionSafetyFailure(failure)
	counts := stopCallControllers(motionAccess)
	if counts["ctrl-b"] != 0 {
		t.Fatalf("位置超差停止不得发给 probe2 控制器 ctrl-b, got %d", counts["ctrl-b"])
	}
	if counts["ctrl-a"] == 0 {
		t.Fatal("故障 probe 的 ctrl-a 应收到 Stop")
	}
}

// legacy single：快照与配置均无绑定时保持既有回退（急停所有已连接控制器）。
func TestTraversal_BoundControllers_LegacyEmptyBindingFallbackPreserved(t *testing.T) {
	motionAccess := boundControllersMotion()
	mgr, _ := newBoundSessionManager(motionAccess, nil, nil, false)

	if err := mgr.emergencyStopMotionControllers(); err != nil {
		t.Fatalf("emergencyStopMotionControllers: %v", err)
	}
	seen := make(map[string]bool)
	for _, id := range motionAccess.emergencyStopCalls {
		seen[id] = true
	}
	if !seen["ctrl-a"] || !seen["ctrl-b"] {
		t.Fatalf("legacy 空绑定回退应急停全部已连接控制器: %v", motionAccess.emergencyStopCalls)
	}
}

// managed 会话空快照：双模式禁止"操作所有已连接控制器"的兼容回退（spec I1）。
func TestTraversal_BoundControllers_ManagedEmptySnapshotNoBroadcast(t *testing.T) {
	motionAccess := boundControllersMotion()
	mgr, session := newBoundSessionManager(motionAccess, nil, nil, true)

	if err := mgr.emergencyStopMotionControllers(); err != nil {
		t.Fatalf("emergencyStopMotionControllers: %v", err)
	}
	if len(motionAccess.emergencyStopCalls) != 0 {
		t.Fatalf("managed 空快照禁止广播急停: %v", motionAccess.emergencyStopCalls)
	}
	session.MarkDone()
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(motionAccess.stopCalls) != 0 {
		t.Fatalf("managed 空快照禁止广播停止: %v", motionAccess.stopCalls)
	}
}
