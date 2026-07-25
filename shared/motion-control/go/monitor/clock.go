package monitor

import "time"

// Clock 抽象时间源与定时器，便于测试注入。
//
// 设计理由（spec Code Style: 不使用 time.Sleep 断言轮询时序；注入 clock/ticker）：
//   - 生产代码使用 RealClock，包装标准库 time
//   - 测试代码使用 FakeClock（fake_clock.go），手动推进时间、手动触发 ticker
//   - 所有需要"按时间判定"的逻辑（Freshness 计算、fast window 过期、轮询间隔）
//     必须通过 Clock.Now() 获取时间，禁止 time.Sleep 等待真实时间
//
// polling loop 通过 NewTicker 创建 ticker，等待 tick 触发采集；
// interval 变化时（moving/idle 切换）Stop 旧 ticker 再新建一个。
type Clock interface {
	// Now 返回当前时间。
	Now() time.Time
	// NewTicker 创建一个周期触发器；测试可手动 Fire 模拟 tick。
	NewTicker(d time.Duration) Ticker
}

// Ticker 是周期触发器抽象。
//
// 语义对齐标准库 time.Ticker：
//   - C() 返回 tick 通道，每次 tick 向通道发送当前时间
//   - Stop() 停止触发器；停止后 C() 不再产生 tick
//   - 生产实现缓冲为 1（标准库行为），FakeTicker 也缓冲为 1
type Ticker interface {
	// C 返回 tick 通道。
	C() <-chan time.Time
	// Stop 停止触发器；可重复调用，幂等。
	Stop()
}

// RealClock 是生产用 Clock，包装标准库 time。
type RealClock struct{}

// Now 实现 Clock 接口。
func (RealClock) Now() time.Time { return time.Now() }

// NewTicker 实现 Clock 接口。
func (RealClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{t: time.NewTicker(d)}
}

// realTicker 包装标准库 time.Ticker，实现 Ticker 接口。
type realTicker struct {
	t *time.Ticker
}

// C 实现 Ticker 接口。
func (r *realTicker) C() <-chan time.Time { return r.t.C }

// Stop 实现 Ticker 接口。
func (r *realTicker) Stop() { r.t.Stop() }
