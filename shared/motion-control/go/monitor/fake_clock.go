package monitor

import (
	"sync"
	"time"
)

// FakeClock 是测试用 Clock，支持手动推进时间和手动触发 ticker。
//
// 使用方式：
//
//	clock := NewFakeClock(start)
//	monitor := NewMotionStatusMonitor(..., clock)
//	monitor.RegisterController("c1", fakeController)
//	// 取到 polling loop 创建的 ticker
//	ticker := clock.LatestTicker()
//	// 手动触发一次 tick，模拟一个轮询周期
//	ticker.Fire(clock.Now())
//	// 或一次性触发所有 ticker（多控制器场景）
//	clock.TickAll()
//
// 不用于生产代码：仅服务于 monitor 包及其消费者（manager 等）的测试。
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*FakeTicker
}

// NewFakeClock 创建一个测试用 FakeClock，初始时间为 start。
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now 实现 Clock 接口。
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance 推进 FakeClock 的时间；不自动触发 ticker，测试需显式调用 TickAll
// 或 ticker.Fire 来模拟 tick。这样设计让测试对"何时触发"有完全控制，
// 避免时间推进与 ticker 触发耦合导致的非确定性。
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// NewTicker 实现 Clock 接口，返回 *FakeTicker 便于测试类型断言与手动 Fire。
//
// 每个 FakeTicker 持有 FakeClock 引用，Fire 在已 Stop 时可转发到当前活跃 ticker
//（见 FakeTicker.Fire 注释），支持 pollLoop 自适应频率切换 ticker 的测试场景。
func (c *FakeClock) NewTicker(d time.Duration) Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &FakeTicker{
		clock:  c,
		period: d,
		c:      make(chan time.Time, 1),
	}
	c.tickers = append(c.tickers, t)
	return t
}

// LatestTicker 返回最近创建的未停止 FakeTicker；若全部已停止或无 ticker 返回 nil。
//
// 用途：polling loop 在 interval 变化时会 Stop 旧 ticker、创建新 ticker，
// 测试通过本方法取到"当前活跃"的那个来 Fire。返回未停止的最近一个，
// 跳过被 Stop 的旧 ticker。
func (c *FakeClock) LatestTicker() *FakeTicker {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.tickers) - 1; i >= 0; i-- {
		t := c.tickers[i]
		t.mu.Lock()
		stopped := t.stopped
		t.mu.Unlock()
		if !stopped {
			return t
		}
	}
	return nil
}

// TickAll 向所有未停止的 FakeTicker 发送一次 tick（非阻塞，缓冲满则丢弃）。
//
// 用于多控制器场景：一次调用模拟"所有控制器都被时间推进"。
// 单控制器场景也可用，但直接调 LatestTicker().Fire(now) 更明确。
//
// 实现说明：直接向每个未 Stop 的 ticker.c 发送 tick，不调用 Fire。
// Fire 始终转发到 LatestTicker（见 FakeTicker.Fire 注释），若 TickAll 调用 Fire，
// 多控制器场景下所有 Fire 都会汇聚到 LatestTicker，导致该 ticker 收到多个 tick、
// 其他 ticker 收不到 tick。直接发送到每个 ticker.c 保证每个 pollLoop 都被唤醒一次。
func (c *FakeClock) TickAll() {
	c.mu.Lock()
	now := c.now
	tickers := append([]*FakeTicker(nil), c.tickers...)
	c.mu.Unlock()

	for _, t := range tickers {
		t.mu.Lock()
		stopped := t.stopped
		t.mu.Unlock()
		if stopped {
			continue
		}
		select {
		case t.c <- now:
		default:
		}
	}
}

// FakeTicker 是测试用 Ticker，支持手动 Fire 触发 tick。
//
// 缓冲策略：C() 通道容量为 1，Fire 非阻塞；缓冲满时丢弃新 tick（latest-only 语义）。
// 这模拟生产环境 time.Ticker 的行为：消费慢时 tick 会被丢弃，不阻塞生产者。
//
// clock 字段持有创建此 ticker 的 FakeClock 引用，用于 Fire 转发语义（见 Fire 注释）。
type FakeTicker struct {
	clock   *FakeClock
	period  time.Duration
	c       chan time.Time
	mu      sync.Mutex
	stopped bool
}

// C 实现 Ticker 接口。
func (f *FakeTicker) C() <-chan time.Time { return f.c }

// Stop 实现 Ticker 接口；幂等，可重复调用。
func (f *FakeTicker) Stop() {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
}

// Fire 手动触发一次 tick，向 C 发送指定时间。
//
// 行为：始终通过 LatestTicker 找到当前活跃 ticker 投递 tick。
//   - 若自身不是 LatestTicker（pollLoop 已切换到新 ticker）：转发到 LatestTicker
//   - 若自身是 LatestTicker（无切换场景）：直接发送到自身 C
//   - 若 LatestTicker 为 nil（初始化阶段或全部已 Stop）：兜底发送到自身 C
//
// 始终转发的设计理由：
//
//	Slice 7 自适应频率让 pollLoop 在间隔变化时创建新 ticker、Stop 旧 ticker。
//	测试持有的 ticker 引用可能已过期（pollLoop 已切换到新 ticker）。若 Fire 在未 Stop 时
//	直接发送到自身 c，pollLoop 已切换到新 ticker，tick 不会被消费，测试超时——
//	这不是测试逻辑错误，而是 ticker 引用过期。始终转发到 LatestTicker 让测试
//	"触发一次 tick"的意图落到当前活跃 ticker，无论持有的引用是否过期。
//
//	生产代码不 Fire ticker（等 time.Ticker.C()），转发语义不影响生产行为。
//
// 转发不递归：LatestTicker 返回非 stopped ticker，其 Fire 走"自身是 LatestTicker"路径直接发送。
func (f *FakeTicker) Fire(now time.Time) {
	if f.clock != nil {
		latest := f.clock.LatestTicker()
		if latest != nil && latest != f {
			latest.Fire(now)
			return
		}
	}

	// 自身是 LatestTicker 或无 clock（初始化阶段）：直接发送到自身 c
	f.mu.Lock()
	stopped := f.stopped
	f.mu.Unlock()
	if stopped {
		return
	}
	select {
	case f.c <- now:
	default:
		// 缓冲满，丢弃；模拟 latest-only 语义
	}
}
