package hardware

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
)

// DAQ-T-1602 真机测试：默认 t.Skip，设置 DAQ_T1602_REAL=1 后连真实设备
// 192.168.3.201:502 运行（对齐既有真机测试的环境变量门控约定）。

func TestDAQT1602RealDeviceConnectAndPoll(t *testing.T) {
	if os.Getenv("DAQ_T1602_REAL") != "1" {
		t.Skip("set DAQ_T1602_REAL=1 to run against 192.168.3.201:502")
	}

	device := NewDAQT1602(core.Profile{
		ID:      "t1602-real-connect-poll",
		Name:    "T1602 real device",
		Type:    core.DeviceDaqT1602,
		Address: "192.168.3.201",
		Port:    502,
	})
	device.OnLog(func(entry LogEntry) {
		if entry.Level == "warn" || entry.Level == "error" {
			t.Logf("driver %s: %s (%s)", entry.Level, entry.Message, entry.Detail)
		}
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	cfg, err := device.GetDaqT1602Config()
	if err != nil {
		t.Fatalf("GetDaqT1602Config returned error: %v", err)
	}
	t.Logf("channel type codes: %v", cfg.TypeCodes)

	payloads := make(chan core.DataPayload, 32)
	device.SetDataSink(func(payload core.DataPayload) {
		select {
		case payloads <- payload:
		default:
		}
	})
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	var frames atomic.Int64
	deadline := time.After(10 * time.Second)
	started := time.Now()
loop:
	for {
		select {
		case payload := <-payloads:
			frames.Add(1)
			if len(payload.Channels) != 16 {
				t.Fatalf("channel count = %d, want 16", len(payload.Channels))
			}
		case <-deadline:
			break loop
		}
	}
	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	count := frames.Load()
	rate := float64(count) / time.Since(started).Seconds()
	t.Logf("captured %d frames, rate %.2f Hz (spec: ~4.9 Hz)", count, rate)
	if count == 0 {
		t.Fatal("no frames captured from real device")
	}
	// 固件串行节流 ~4.9Hz 上限留 20% 余量；超过说明帧被拆分/重复 emit。
	if rate > 6.0 {
		t.Fatalf("frame rate %.2f Hz exceeds expected ~4.9 Hz ceiling", rate)
	}
}

// TestDAQT1602RealDeviceTypeWriteReadback 验证类型写回路径（spec Q3 先决验证）：
// 读回原值 → 写同值（不改设备状态的安全写）→ FC6 回显 + FC3 读回校验。
func TestDAQT1602RealDeviceTypeWriteReadback(t *testing.T) {
	if os.Getenv("DAQ_T1602_REAL") != "1" {
		t.Skip("set DAQ_T1602_REAL=1 to run against 192.168.3.201:502")
	}

	device := NewDAQT1602(core.Profile{
		ID:      "t1602-real-type-write",
		Type:    core.DeviceDaqT1602,
		Address: "192.168.3.201",
		Port:    502,
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	cfg, err := device.GetDaqT1602Config()
	if err != nil {
		t.Fatalf("GetDaqT1602Config returned error: %v", err)
	}
	t.Logf("original type codes: %v", cfg.TypeCodes)

	// 写回当前值：完整走 FC6 写 + FC3 读回校验路径，但不改变设备实际配置，
	// 避免无校准环境下把通道类型写成未知状态。
	if err := device.ApplyDaqT1602Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1602Config (write-back same values) returned error: %v", err)
	}
}
