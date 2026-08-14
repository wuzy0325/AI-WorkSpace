package hardware

import (
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
)

// startT1602CountingAcquisition 连接 + 启动采集，返回帧计数器与停止函数。
// sink 只做原子计数（非阻塞，符合驱动 sink 契约）。
func startT1602CountingAcquisition(t *testing.T, device *DAQT1602) (*atomic.Int64, func()) {
	t.Helper()
	var frames atomic.Int64
	device.SetDataSink(func(p core.DataPayload) { frames.Add(1) })
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	return &frames, func() {
		if err := device.StopAcquisition(); err != nil {
			t.Fatalf("StopAcquisition returned error: %v", err)
		}
		if err := device.Disconnect(); err != nil {
			t.Fatalf("Disconnect returned error: %v", err)
		}
	}
}

// 默认（未设置轮询间隔）：全速读取，帧率只受 fake 设备响应速度限制。
func TestDAQT1602PollIntervalDefaultFullSpeed(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	defer server.Close()

	frames, stop := startT1602CountingAcquisition(t, device)
	time.Sleep(300 * time.Millisecond)
	n := frames.Load()
	stop()
	if n < 2 {
		t.Fatalf("full-speed frames in 300ms = %d, want >= 2", n)
	}
}

// 设置 500ms 轮询间隔：600ms 内只能收到 1 帧（首帧节拍 ~500ms），节流生效。
func TestDAQT1602PollIntervalThrottlesFrameRate(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	defer server.Close()
	device.SetPollIntervalFn(func() time.Duration { return 500 * time.Millisecond })

	frames, stop := startT1602CountingAcquisition(t, device)
	time.Sleep(600 * time.Millisecond)
	n := frames.Load()
	stop()
	if n < 1 || n > 2 {
		t.Fatalf("throttled frames in 600ms (500ms interval) = %d, want 1~2", n)
	}
}

// 运行中动态修改轮询间隔：设置变更后下一帧立即按新频率到达（无需重启采集）。
func TestDAQT1602PollIntervalDynamicChange(t *testing.T) {
	fake := &t1602FakeDevice{}
	device, server := newT1602TestDevice(fake)
	defer server.Close()
	var intervalMs atomic.Int64
	intervalMs.Store(500)
	device.SetPollIntervalFn(func() time.Duration {
		return time.Duration(intervalMs.Load()) * time.Millisecond
	})

	frames, stop := startT1602CountingAcquisition(t, device)

	// 慢相位：500ms 间隔，600ms 内 1~2 帧
	time.Sleep(600 * time.Millisecond)
	slow := frames.Load()
	if slow < 1 || slow > 2 {
		t.Fatalf("slow-phase frames (500ms interval) = %d, want 1~2", slow)
	}

	// 切换到全速：帧率显著提升
	intervalMs.Store(0)
	time.Sleep(600 * time.Millisecond)
	fast := frames.Load() - slow
	stop()
	if fast < 3 {
		t.Fatalf("fast-phase frames after switching to full speed = %d, want >= 3", fast)
	}
}

func TestDAQT1602PollIntervalRebuildsCadenceAfterFullSpeed(t *testing.T) {
	var interval atomic.Int64
	device := &DAQT1602{}
	device.SetPollIntervalFn(func() time.Duration { return time.Duration(interval.Load()) })
	device.lastInterval = 50 * time.Millisecond
	device.nextTick = time.Now().Add(-time.Second)

	if !device.waitForNextTick(make(chan struct{})) {
		t.Fatal("full-speed wait unexpectedly stopped")
	}
	interval.Store(int64(50 * time.Millisecond))
	started := time.Now()
	if !device.waitForNextTick(make(chan struct{})) {
		t.Fatal("throttled wait unexpectedly stopped")
	}
	if elapsed := time.Since(started); elapsed < 35*time.Millisecond {
		t.Fatalf("returning to the same interval used stale cadence: elapsed=%v, want >=35ms", elapsed)
	}
}
