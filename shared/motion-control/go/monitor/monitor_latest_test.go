package monitor

import (
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// TestMonitorLatestEmptyByDefault 验证无控制器时 Latest 返回零值 StatusSnapshot。
// 这是 monitor 的"未启动"语义：消费者必须能区分"无数据"和"有数据"。
func TestMonitorLatestEmptyByDefault(t *testing.T) {
	m := NewMotionStatusMonitor(DefaultConfig(), NewFakeClock(time.Now()), DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	snap := m.Latest()
	if snap.Sequence != 0 {
		t.Fatalf("Sequence = %d, want 0", snap.Sequence)
	}
	if len(snap.Controllers) != 0 {
		t.Fatalf("len(Controllers) = %d, want 0", len(snap.Controllers))
	}
	if !snap.PublishedAt.IsZero() {
		t.Fatalf("PublishedAt = %v, want zero", snap.PublishedAt)
	}
}

// TestMonitorLatestControllerNotRegistered 验证未注册的 controller 返回 (zero, false)。
// 消费者通过 ok=false 区分"控制器不存在"和"存在但无快照"。
func TestMonitorLatestControllerNotRegistered(t *testing.T) {
	m := NewMotionStatusMonitor(DefaultConfig(), NewFakeClock(time.Now()), DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	_, ok := m.LatestController("nonexistent")
	if ok {
		t.Fatal("LatestController should return false for unregistered controller")
	}
}

// TestMonitorLatestDoesNotReadHardware 验证 Latest 不触发硬件读取。
//
// spec Interface Contract: "Latest - 只读内存，不访问硬件"。
// 如果 Latest 触发硬件读取，多个 HTTP status 请求会放大硬件 I/O，违背 monitor 设计目标。
func TestMonitorLatestDoesNotReadHardware(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	m.RegisterController("c1", fc)
	// defer Shutdown 保证 pollLoop goroutine 退出，避免 goroutine 泄漏
	//（RegisterController 启动的 pollLoop 持 rootCtx 派生 ctx，仅在 Shutdown 时取消）
	defer m.Shutdown()

	before := fc.statusCallCount()
	for i := 0; i < 10; i++ {
		_ = m.Latest()
	}
	after := fc.statusCallCount()

	if before != after {
		t.Fatalf("Latest triggered %d hardware reads, want 0", after-before)
	}
}

// TestMonitorLatestControllerDoesNotReadHardware 验证 LatestController 不触发硬件读取。
func TestMonitorLatestControllerDoesNotReadHardware(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	m.RegisterController("c1", fc)
	// defer Shutdown 保证 pollLoop goroutine 退出，避免 goroutine 泄漏
	defer m.Shutdown()

	before := fc.statusCallCount()
	for i := 0; i < 10; i++ {
		_, _ = m.LatestController("c1")
	}
	after := fc.statusCallCount()

	if before != after {
		t.Fatalf("LatestController triggered %d hardware reads, want 0", after-before)
	}
}

// TestMonitorLatestReturnsDeepCopy 验证 Latest 返回深拷贝，消费者修改不影响 monitor 内部状态。
//
// spec Interface Contract: "Latest - 返回深拷贝或只读安全副本"
// spec Data Model: "Status、Controllers 和嵌套 Axes 对 monitor 外部是不可变值"
func TestMonitorLatestReturnsDeepCopy(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	m.RegisterController("c1", fc)
	// defer Shutdown 保证 pollLoop goroutine 退出，避免 goroutine 泄漏
	defer m.Shutdown()

	// 手动注入快照（Slice 2 尚未实现 polling loop，用直接注入测试 deep copy）
	cs := m.controllerStateLocked("c1")
	if cs == nil {
		t.Fatal("controller state not found after RegisterController")
	}
	cs.mu.Lock()
	cs.lastSnap = &ControllerStatusSnapshot{
		ControllerID: "c1",
		Generation:   1,
		Sequence:     5,
		Status: core.ControllerStatus{
			ID:        "c1",
			Connected: true,
			Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 10.0, Moving: false}},
		},
	}
	cs.mu.Unlock()

	snap := m.Latest()
	if len(snap.Controllers) != 1 {
		t.Fatalf("len(Controllers) = %d, want 1", len(snap.Controllers))
	}

	// 修改返回的快照，验证不影响 monitor 内部状态
	snap.Controllers[0].Status.Axes[0].Position = 999.0
	snap.Controllers[0].Status.Axes = append(snap.Controllers[0].Status.Axes, core.AxisStatus{Name: core.AxisY})

	// 再次 Latest，验证内部状态未被污染
	snap2 := m.Latest()
	if len(snap2.Controllers) != 1 {
		t.Fatalf("after mutation, len(Controllers) = %d, want 1", len(snap2.Controllers))
	}
	if snap2.Controllers[0].Status.Axes[0].Position != 10.0 {
		t.Fatalf("internal state was mutated: Position = %v, want 10.0", snap2.Controllers[0].Status.Axes[0].Position)
	}
	if len(snap2.Controllers[0].Status.Axes) != 1 {
		t.Fatalf("internal state was mutated: len(Axes) = %d, want 1", len(snap2.Controllers[0].Status.Axes))
	}
}

// TestMonitorLatestControllerReturnsDeepCopy 验证 LatestController 返回深拷贝。
func TestMonitorLatestControllerReturnsDeepCopy(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	m.RegisterController("c1", fc)
	// defer Shutdown 保证 pollLoop goroutine 退出，避免 goroutine 泄漏
	defer m.Shutdown()

	cs := m.controllerStateLocked("c1")
	cs.mu.Lock()
	cs.lastSnap = &ControllerStatusSnapshot{
		ControllerID: "c1",
		Sequence:     5,
		Status: core.ControllerStatus{
			ID:   "c1",
			Axes: []core.AxisStatus{{Name: core.AxisX, Position: 10.0}},
		},
	}
	cs.mu.Unlock()

	snap, ok := m.LatestController("c1")
	if !ok {
		t.Fatal("LatestController returned false, want true")
	}
	if snap.Status.Axes[0].Position != 10.0 {
		t.Fatalf("Position = %v, want 10.0", snap.Status.Axes[0].Position)
	}

	// 修改返回的快照
	snap.Status.Axes[0].Position = 999.0
	snap.Status.Axes = append(snap.Status.Axes, core.AxisStatus{Name: core.AxisY})

	// 再次获取，验证内部状态未被污染
	snap2, _ := m.LatestController("c1")
	if snap2.Status.Axes[0].Position != 10.0 {
		t.Fatalf("internal state was mutated: Position = %v, want 10.0", snap2.Status.Axes[0].Position)
	}
	if len(snap2.Status.Axes) != 1 {
		t.Fatalf("internal state was mutated: len(Axes) = %d, want 1", len(snap2.Status.Axes))
	}
}

// TestMonitorFreshnessPolicyReturnsInjected 验证 FreshnessPolicy() 返回构造时注入的策略。
//
// 消费者（如校准/遍历）需要 policy 在调用瞬间计算 Freshness，不固化在快照中（spec Decision 4）。
func TestMonitorFreshnessPolicyReturnsInjected(t *testing.T) {
	policy := DefaultFreshnessPolicy{StaleThreshold: 750 * time.Millisecond}
	m := NewMotionStatusMonitor(DefaultConfig(), NewFakeClock(time.Now()), policy)

	got := m.FreshnessPolicy()
	// 类型断言验证返回的就是注入的 policy
	dp, ok := got.(DefaultFreshnessPolicy)
	if !ok {
		t.Fatalf("FreshnessPolicy() returned %T, want DefaultFreshnessPolicy", got)
	}
	if dp.StaleThreshold != 750*time.Millisecond {
		t.Fatalf("StaleThreshold = %v, want 750ms", dp.StaleThreshold)
	}
}
