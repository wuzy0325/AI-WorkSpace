package hardware

import "time"

// daq_t1602_poll.go — DAQ-T-1602 采集轮询间隔控制。
//
// 设备固件采集周期固定 ~100ms，但"读取/保存"频率由软件轮询间隔决定：
// 设 1Hz 就每秒读一帧完整 16 通道快照，设 5Hz 就全速 ~4.9Hz 读取。
// 由此让 T1602 的采集/保存/UI 频率整体跟随全局"刷新频率"设置（1~5Hz），
// 设置即采集频率（spec-daq-t1602 §前端集成约定，2026-08-19 定案）。

// SetPollIntervalFn 设置轮询间隔提供者：每次采样前调用，返回值是两次采样
// 之间的最小间隔；返回 <=0 表示全速（无节流，默认行为）。
//
// 驱动按"帧节拍"控制帧率：
//   - 间隔变化时立即以当前时刻重建节拍基准，设置变更即时生效
//   - 节拍固定步进，读耗时不计入间隔 → 帧率精确等于设置频率
//   - 读耗时超过间隔（如 5Hz vs 设备单帧 ~206ms）时自动追赶，实际帧率
//     回落到设备上限 ~4.9Hz，不会堆叠积压
//
// 并发安全：fn 在 readLoop goroutine 中调用；节拍状态仅 readLoop 访问。
func (d *DAQT1602) SetPollIntervalFn(fn func() time.Duration) {
	d.mu.Lock()
	d.pollIntervalFn = fn
	d.mu.Unlock()
}

// currentPollInterval 读取当前轮询间隔（<=0 表示全速）。
func (d *DAQT1602) currentPollInterval() time.Duration {
	d.mu.RLock()
	fn := d.pollIntervalFn
	d.mu.RUnlock()
	if fn == nil {
		return 0
	}
	return fn()
}

// waitForNextTick 等待到下一帧节拍；返回 false 表示收到停止信号，调用方应退出。
// 仅在 readLoop goroutine 内调用，节拍状态（lastInterval/nextTick）无需加锁。
func (d *DAQT1602) waitForNextTick(stop chan struct{}) bool {
	interval := d.currentPollInterval()
	if interval <= 0 {
		// Returning from full speed must invalidate the old cadence baseline.
		d.lastInterval = 0
		d.nextTick = time.Time{}
		return true
	}
	if interval != d.lastInterval {
		// 间隔变化（设置变更）：以当前时刻重建节拍基准，让新频率立即生效
		d.lastInterval = interval
		d.nextTick = time.Now().Add(interval)
	}
	wait := time.Until(d.nextTick)
	// 节拍固定步进：无论本次读耗时多少，帧间总间隔 = interval
	d.nextTick = d.nextTick.Add(interval)
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}
