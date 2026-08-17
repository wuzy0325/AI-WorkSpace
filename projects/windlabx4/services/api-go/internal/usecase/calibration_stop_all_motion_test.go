// Package usecase — stopAllMotion 有界超时回归测试
//
// 覆盖范围：
//   - stopAllMotion 在 StatusAll 阻塞场景下，必须受 ctx 3s 超时控制，不能无限等待。
//
// 防回归背景：
//   stopAllMotion 原用 context.Background()，B140 通信异常时 StatusAll 串行多个
//   sendCommand 累积 ~70s 阻塞，导致 Wails v3 ServiceShutdown 同步卡死 GUI 主线程
//   （Windows 标记为"无响应"）。改为 3s 有界超时后，最坏 3s+5s=8s 返回。本测试
//   防止后续维护者误改回 Background() 或调整超时导致修复静默失效。
//
// 测试用例遵循三段式：测试前置 / 测试步骤 / 期待结果。
package usecase

import (
	"context"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/motion"
	"windlabx4/services/api-go/internal/ports"
)

// blockingMotionManager 模拟硬件卡住的 MotionManager。
//
// 行为语义：
//   - StatusAll：阻塞到 ctx.Done() 才返回（模拟 B140 queryStatus 卡住）
//   - Stop：阻塞到 ctx.Done() 才返回（模拟 B140 Stop 命令卡住）
//   - 其余方法：no-op
//
// 用途：验证 stopAllMotion 在硬件无响应时受 ctx 超时控制。
type blockingMotionManager struct {
	statusAllCalled chan struct{} // StatusAll 被调用的信号（便于测试同步）
	stopCalled      chan struct{} // Stop 被调用的信号
}

func newBlockingMotionManager() *blockingMotionManager {
	return &blockingMotionManager{
		statusAllCalled: make(chan struct{}, 1),
		stopCalled:      make(chan struct{}, 1),
	}
}

// LoadProfiles / SaveProfiles / GetProfiles / UpsertProfile / DeleteProfile:
// no-op，仅为满足 ports.MotionManager 接口。
func (m *blockingMotionManager) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	return nil, nil
}
func (m *blockingMotionManager) SaveProfiles([]motion.MotionControllerProfile) error { return nil }
func (m *blockingMotionManager) GetProfiles() []motion.MotionControllerProfile        { return nil }
func (m *blockingMotionManager) UpsertProfile(motion.MotionControllerProfile) error   { return nil }
func (m *blockingMotionManager) DeleteProfile(string) error                           { return nil }

// Connect / Disconnect: no-op。
func (m *blockingMotionManager) Connect(context.Context, string) error    { return nil }
func (m *blockingMotionManager) Disconnect(context.Context, string) error { return nil }

// StatusAll 阻塞到 ctx 取消才返回，模拟 B140 queryStatus 通信卡住。
// 返回空切片（无 Moving 轴），让 stopAllMotion 跳过 Stop 循环。
func (m *blockingMotionManager) StatusAll(ctx context.Context) []motion.ControllerStatus {
	select {
	case m.statusAllCalled <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil
}

func (m *blockingMotionManager) MoveTo(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *blockingMotionManager) MoveBy(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *blockingMotionManager) Jog(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *blockingMotionManager) Home(context.Context, string, motion.AxisName) error { return nil }

// Stop 阻塞到 ctx 取消才返回，模拟 B140 Stop 命令卡住。
func (m *blockingMotionManager) Stop(ctx context.Context, _ string, _ motion.AxisName) error {
	select {
	case m.stopCalled <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (m *blockingMotionManager) EmergencyStop(context.Context, string) error            { return nil }
func (m *blockingMotionManager) ResetEmergencyStop(context.Context, string) error      { return nil }
func (m *blockingMotionManager) DefinePosition(context.Context, string, motion.AxisName, float64) error {
	return nil
}

// 编译期接口守卫：保证 blockingMotionManager 实现 ports.MotionManager。
var _ ports.MotionManager = (*blockingMotionManager)(nil)

// TestStopAllMotion_BoundedByContextTimeout 验证 stopAllMotion 受 3s ctx 超时控制。
//
// 测试前置：
//   - 注入 blockingMotionManager，其 StatusAll 阻塞到 ctx.Done() 才返回
//   - 记录测试开始时间
//
// 测试步骤：
//   - 调用 stopAllMotion(mgr)
//
// 期待结果：
//   - 调用在 3s+2s 容差内返回（3s ctx 超时 + 少量调度开销），证明受 ctx 控制
//   - 若改回 context.Background()，本测试会 10s 超时失败
func TestStopAllMotion_BoundedByContextTimeout(t *testing.T) {
	// 测试前置
	mgr := newBlockingMotionManager()
	start := time.Now()
	// 用 10s 作为测试硬超时兜底，远大于 stopAllMotion 内部 3s 上限；
	// 若修复回归为 Background()，10s 后测试失败而非无限挂起。
	done := make(chan error, 1)
	go func() { done <- stopAllMotion(mgr) }()

	// 等待 StatusAll 被调用，确认 mock 真的进入阻塞路径
	select {
	case <-mgr.statusAllCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("StatusAll 未在 2s 内被调用，mock 未按预期工作")
	}

	// 测试步骤 + 期待结果
	select {
	case err := <-done:
		elapsed := time.Since(start)
		// 期望 ≤ 5s（3s ctx 超时 + 2s 容差）。若超时则证明 ctx 未生效。
		if elapsed > 5*time.Second {
			t.Fatalf("stopAllMotion 耗时 %v 超过 5s 上限，ctx 超时未生效（err=%v）", elapsed, err)
		}
		t.Logf("stopAllMotion 在 %v 内返回，ctx 超时生效（err=%v）", elapsed, err)
	case <-time.After(10 * time.Second):
		t.Fatal("stopAllMotion 10s 未返回，疑似改回 context.Background() 导致无界阻塞")
	}
}

// TestStopAllMotion_StopCallRespectsContextTimeout 验证 Stop 循环也受 ctx 超时控制。
//
// 测试前置：
//   - 注入 blockingMotionManager，返回一个 Moving=true 的轴（触发 Stop 调用）
//   - 用自定义包装器覆盖 StatusAll，返回固定状态后让 Stop 阻塞
//
// 测试步骤：
//   - 调用 stopAllMotion(mgr)
//
// 期待结果：
//   - 调用在 5s 容差内返回（3s ctx 超时 + 2s 容差）
//   - Stop 被调用且受 ctx 控制解除阻塞
func TestStopAllMotion_StopCallRespectsContextTimeout(t *testing.T) {
	// 测试前置：构造一个 Moving 轴的状态
	movingStatus := []motion.ControllerStatus{
		{
			ID:        "B140-1",
			Connected: true,
			Axes: []motion.AxisStatus{
				{Name: "X", Moving: true},
			},
		},
	}
	mgr := &stopBlockingOnStopManager{
		status:   movingStatus,
		stopDone: make(chan struct{}, 1),
	}
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- stopAllMotion(mgr) }()

	// 测试步骤 + 期待结果
	select {
	case err := <-done:
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			t.Fatalf("stopAllMotion 耗时 %v 超过 5s 上限，Stop 路径 ctx 超时未生效（err=%v）", elapsed, err)
		}
		// 确认 Stop 确实被调用过
		select {
		case <-mgr.stopDone:
		default:
			t.Fatal("Stop 未被调用，测试未覆盖 Stop 路径")
		}
		t.Logf("stopAllMotion（Stop 路径）在 %v 内返回（err=%v）", elapsed, err)
	case <-time.After(10 * time.Second):
		t.Fatal("stopAllMotion 10s 未返回，Stop 路径疑似无界阻塞")
	}
}

// stopBlockingOnStopManager StatusAll 立即返回（带 Moving 轴），Stop 阻塞到 ctx 取消。
type stopBlockingOnStopManager struct {
	status   []motion.ControllerStatus
	stopDone chan struct{}
}

func (m *stopBlockingOnStopManager) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	return nil, nil
}
func (m *stopBlockingOnStopManager) SaveProfiles([]motion.MotionControllerProfile) error { return nil }
func (m *stopBlockingOnStopManager) GetProfiles() []motion.MotionControllerProfile        { return nil }
func (m *stopBlockingOnStopManager) UpsertProfile(motion.MotionControllerProfile) error   { return nil }
func (m *stopBlockingOnStopManager) DeleteProfile(string) error                           { return nil }
func (m *stopBlockingOnStopManager) Connect(context.Context, string) error                { return nil }
func (m *stopBlockingOnStopManager) Disconnect(context.Context, string) error             { return nil }
func (m *stopBlockingOnStopManager) StatusAll(context.Context) []motion.ControllerStatus  { return m.status }
func (m *stopBlockingOnStopManager) MoveTo(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *stopBlockingOnStopManager) MoveBy(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *stopBlockingOnStopManager) Jog(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *stopBlockingOnStopManager) Home(context.Context, string, motion.AxisName) error { return nil }
func (m *stopBlockingOnStopManager) Stop(ctx context.Context, _ string, _ motion.AxisName) error {
	select {
	case m.stopDone <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}
func (m *stopBlockingOnStopManager) EmergencyStop(context.Context, string) error { return nil }
func (m *stopBlockingOnStopManager) ResetEmergencyStop(context.Context, string) error {
	return nil
}
func (m *stopBlockingOnStopManager) DefinePosition(context.Context, string, motion.AxisName, float64) error {
	return nil
}

var _ ports.MotionManager = (*stopBlockingOnStopManager)(nil)
