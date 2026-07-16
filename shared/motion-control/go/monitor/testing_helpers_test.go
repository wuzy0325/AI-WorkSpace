package monitor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// fakeMotionController 是测试用 MotionController 实现，统计 Status() 调用次数。
//
// 用途：
//   - 验证 monitor 多消费者只触发一轮 Status()（single-flight 行为）
//   - 验证 Latest/Subscribe 不触发硬件读取
//   - 通过 statusToReturn/errToReturn 模拟各种硬件响应
//   - 通过 gate 模拟"Status 阻塞"场景，验证 single-flight 与 Unregister 的取消语义
//
// 不用于生产代码：仅 monitor 包及其测试消费者使用。
type fakeMotionController struct {
	profile        core.MotionControllerProfile
	statusToReturn core.ControllerStatus
	errToReturn    error
	delay          time.Duration // 模拟慢硬件；> 0 时 Status() 阻塞 delay
	// gate 非 nil 时 Status() 阻塞直到 gate 关闭或 ctx 取消。
	// 用于 single-flight 测试：阻塞首个 Status()，验证后续 tick 不触发并发 Status()。
	gate chan struct{}

	statusCalls int32 // atomic；race-safe 读取用 atomic.LoadInt32
	mu          sync.Mutex
	// statusCalled 是测试同步信号：每次 Status() 被调用时非阻塞发送。
	// 测试通过 <-fc.statusCalled 等待 polling goroutine 真正触发采集，避免 time.Sleep。
	// nil 表示不发送信号（默认）；测试需要时显式初始化为带缓冲 channel。
	statusCalled chan<- struct{}
}

func newFakeController(id string) *fakeMotionController {
	return &fakeMotionController{
		profile: core.MotionControllerProfile{ID: id},
		statusToReturn: core.ControllerStatus{
			ID:        id,
			Name:      id,
			Connected: true,
			Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 0, Moving: false}},
		},
	}
}

// Status 实现 ports.MotionController。
// 每次调用原子递增 statusCalls，便于测试断言硬件读取次数。
//
// gate 语义：若 gate 非 nil，阻塞直到 gate 关闭或 ctx 取消。
// 这是测试 single-flight 的关键——首个 Status() 阻塞时，后续 tick 不应触发新的 Status()。
func (f *fakeMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	atomic.AddInt32(&f.statusCalls, 1)
	// 同步信号：非阻塞发送，测试通过此信号等待 polling 真正进入 Status()
	if f.statusCalled != nil {
		select {
		case f.statusCalled <- struct{}{}:
		default:
		}
	}
	if f.gate != nil {
		select {
		case <-ctx.Done():
			return core.ControllerStatus{}, ctx.Err()
		case <-f.gate:
		}
	}
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return core.ControllerStatus{}, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statusToReturn, f.errToReturn
}

// statusCallCount 返回 Status() 被调用的次数（race-safe）。
func (f *fakeMotionController) statusCallCount() int {
	return int(atomic.LoadInt32(&f.statusCalls))
}

// setStatus 更新 Status() 返回值（race-safe）。
func (f *fakeMotionController) setStatus(s core.ControllerStatus) {
	f.mu.Lock()
	f.statusToReturn = s
	f.mu.Unlock()
}

// setErr 更新 Status() 返回的错误（race-safe）。
// 测试通过此方法切换成功/失败模式，避免直接字段赋值在 -race 下报警。
func (f *fakeMotionController) setErr(err error) {
	f.mu.Lock()
	f.errToReturn = err
	f.mu.Unlock()
}

// 以下方法为 ports.MotionController 接口的桩实现，monitor 测试不关心其行为。
func (f *fakeMotionController) Connect(ctx context.Context) error                      { return nil }
func (f *fakeMotionController) Disconnect(ctx context.Context) error                   { return nil }
func (f *fakeMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	return nil
}
func (f *fakeMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	return nil
}
func (f *fakeMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	return nil
}
func (f *fakeMotionController) Home(ctx context.Context, axis core.AxisName) error      { return nil }
func (f *fakeMotionController) Stop(ctx context.Context, axis core.AxisName) error      { return nil }
func (f *fakeMotionController) EmergencyStop(ctx context.Context) error                 { return nil }
func (f *fakeMotionController) ResetEmergencyStop(ctx context.Context) error            { return nil }
func (f *fakeMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	return nil
}
func (f *fakeMotionController) GetProfile() core.MotionControllerProfile { return f.profile }
func (f *fakeMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	return nil
}

// 编译期断言：fakeMotionController 实现 ports.MotionController 接口。
// 接口签名变化时立即报错，避免运行时类型断言失败。
var _ ports.MotionController = (*fakeMotionController)(nil)
