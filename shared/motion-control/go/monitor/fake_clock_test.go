package monitor

import (
	"testing"
	"time"
)

// TestFakeClockNow 验证 FakeClock.Now 返回初始时间，Advance 后正确推进。
// 这是测试时序逻辑的基础：所有"按时间判定"的代码必须通过 Clock.Now() 获取时间，
// 测试通过 Advance 控制时间，禁止用 time.Sleep 等待真实时间。
func TestFakeClockNow(t *testing.T) {
	start := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	if !clock.Now().Equal(start) {
		t.Fatalf("Now() = %v, want %v", clock.Now(), start)
	}

	clock.Advance(150 * time.Millisecond)
	want := start.Add(150 * time.Millisecond)
	if !clock.Now().Equal(want) {
		t.Fatalf("After Advance(150ms), Now() = %v, want %v", clock.Now(), want)
	}
}

// TestFakeClockNewTicker 验证 FakeClock.NewTicker 返回可手动触发的 FakeTicker。
// 测试通过 LatestTicker() 取到 polling loop 创建的 ticker，再 Fire 模拟一次 tick。
func TestFakeClockNewTicker(t *testing.T) {
	clock := NewFakeClock(time.Now())
	ticker := clock.NewTicker(100 * time.Millisecond)

	fakeTicker, ok := ticker.(*FakeTicker)
	if !ok {
		t.Fatalf("NewTicker returned %T, want *FakeTicker", ticker)
	}
	if fakeTicker.period != 100*time.Millisecond {
		t.Fatalf("period = %v, want 100ms", fakeTicker.period)
	}
	if got := clock.LatestTicker(); got != fakeTicker {
		t.Fatalf("LatestTicker() returned different instance than NewTicker")
	}
}

// TestFakeTickerFire 验证 FakeTicker.Fire 向 C 发送指定时间。
func TestFakeTickerFire(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC))
	ticker := clock.NewTicker(100 * time.Millisecond)
	fakeTicker := ticker.(*FakeTicker)

	fireTime := clock.Now()
	fakeTicker.Fire(fireTime)

	select {
	case got := <-ticker.C():
		if !got.Equal(fireTime) {
			t.Fatalf("C() received %v, want %v", got, fireTime)
		}
	default:
		t.Fatal("C() should have received a tick after Fire")
	}
}

// TestFakeTickerStop 验证 Stop 后 Fire 不再发送。
// 这对应生产代码 time.Ticker.Stop 的语义：停止后不再产生 tick。
func TestFakeTickerStop(t *testing.T) {
	clock := NewFakeClock(time.Now())
	ticker := clock.NewTicker(100 * time.Millisecond)
	fakeTicker := ticker.(*FakeTicker)

	ticker.Stop()
	fakeTicker.Fire(clock.Now())

	select {
	case <-ticker.C():
		t.Fatal("C() should not receive after Stop")
	default:
		// OK
	}
}

// TestFakeTickerFireNonBlocking 验证 Fire 是非阻塞的（缓冲满则丢弃）。
// 这模拟 latest-only 语义：polling loop 还没消费上一个 tick 时，新 tick 直接丢弃，
// 避免慢消费导致 channel 满、阻塞生产者。
func TestFakeTickerFireNonBlocking(t *testing.T) {
	clock := NewFakeClock(time.Now())
	ticker := clock.NewTicker(100 * time.Millisecond)
	fakeTicker := ticker.(*FakeTicker)

	now := clock.Now()
	fakeTicker.Fire(now)
	fakeTicker.Fire(now) // 缓冲满，应丢弃，不阻塞

	select {
	case <-ticker.C():
		// 第一次 Fire 已投递
	default:
		t.Fatal("first Fire should have been delivered")
	}

	// 第二次 Fire 应被丢弃
	select {
	case <-ticker.C():
		t.Fatal("second Fire should have been dropped (latest-only)")
	default:
		// OK
	}
}

// TestFakeClockTickAll 验证 TickAll 触发所有未停止的 ticker。
// 用于测试 polling loop 多控制器场景：一次 TickAll 模拟所有控制器同时被时间推进。
func TestFakeClockTickAll(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC))
	ticker1 := clock.NewTicker(100 * time.Millisecond).(*FakeTicker)
	ticker2 := clock.NewTicker(500 * time.Millisecond).(*FakeTicker)

	clock.Advance(100 * time.Millisecond)
	clock.TickAll()

	// 两个 ticker 都应收到一次 tick
	select {
	case <-ticker1.C():
	default:
		t.Fatal("ticker1 should have received a tick after TickAll")
	}
	select {
	case <-ticker2.C():
	default:
		t.Fatal("ticker2 should have received a tick after TickAll")
	}
}

// TestRealClockNow 验证 RealClock.Now 返回真实时间。
// 仅做 smoke test，确保 RealClock 实现正确，生产代码可用。
func TestRealClockNow(t *testing.T) {
	clock := RealClock{}
	before := time.Now()
	got := clock.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Fatalf("RealClock.Now() = %v, want between %v and %v", got, before, after)
	}
}
