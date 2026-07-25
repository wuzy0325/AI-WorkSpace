//go:build e2e_p1603

package hardware

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/ffi"
)

// ============================================================
// DAQ-P-1603 端到端真机验证（多点平均模式）
//
// 运行前提：
//   - 设备 192.168.3.102 在线，已连接 4-20mA 传感器
//   - WTNDAQ16H_64.dll 位于 wind-daq cmd/p1603test/ 目录
//
// 运行方式（从 device-sdk/go 目录执行）：
//   $env:GOWORK="off"; go test ./daq/hardware/ -run TestE2E -tags e2e_p1603 -v -timeout 120s
//
// 验证目标（按用户需求：用户采样率 = 每秒数据条目数）：
//   1. 底层硬件采样率固定 1000Hz，多点平均输出用户速率
//   2. 用户设 1Hz → 1000 点平均 → 每秒输出 1 条 DataPayload
//   3. 用户设 100Hz → 10 点平均 → 每秒输出 ~100 条
//   4. 用户设 500Hz → 2 点平均 → 每秒输出 ~500 条
//   5. 各种速率下无超时、无异常退出
// ============================================================

const e2eDeviceIP = "192.168.3.102"
const e2eDLLPath = "C:\\Users\\wuzhy\\Documents\\D\\SVN\\SoftWare\\trunk\\AI-Workspace\\projects\\wind-daq\\services\\api-go\\cmd\\p1603test\\WTNDAQ16H_64.dll"

// dllOnceFunc 确保 DLL 只初始化一次，所有子测试共享。
var dllOnceFunc = func() func() {
	var done bool
	return func() {
		if done {
			return
		}
		if !ffi.IsWTNDAQ16HInitialized() {
			if err := ffi.InitWTNDAQ16H(e2eDLLPath); err != nil {
				panic(fmt.Sprintf("DLL init failed: %v", err))
			}
		}
		done = true
	}
}()

// chNames 为 16 个压力通道命名。
func chNames() []core.ChannelConfig {
	chs := make([]core.ChannelConfig, 16)
	for i := range chs {
		chs[i] = core.ChannelConfig{
			Index:    i,
			Name:     fmt.Sprintf("CH%d", i+1),
			Enabled:  true,
			Unit:     "Pa",
			RangeMin: 0,
			RangeMax: 1000,
		}
	}
	return chs
}

// makeE2EProfile 构造用于 e2e 测试的 profile。
func makeE2EProfile(userRate int) core.Profile {
	return core.Profile{
		ID:           "e2e-p1603",
		Name:         "E2E DAQ-P-1603",
		Type:         core.DeviceDAQP1603,
		Address:      e2eDeviceIP,
		SamplingRate: userRate,
		Channels:     chNames(),
	}
}

// ============================================================
// TestE2E_MultiPointAveraging_100Hz
// 用户采样率 100Hz，每帧 10 点平均。
// 采集 3 秒，预期收到 ~300 条数据，无超时。
// ============================================================
func TestE2E_MultiPointAveraging_100Hz(t *testing.T) {
	dllOnceFunc()
	testE2ERate(t, 100, 3*time.Second, 0.15)
}

// ============================================================
// TestE2E_MultiPointAveraging_500Hz
// 用户采样率 500Hz，每帧 2 点平均。
// 采集 2 秒，预期收到 ~1000 条数据。
// ============================================================
func TestE2E_MultiPointAveraging_500Hz(t *testing.T) {
	dllOnceFunc()
	testE2ERate(t, 500, 2*time.Second, 0.20)
}

// ============================================================
// TestE2E_MultiPointAveraging_10Hz
// 用户采样率 10Hz，每帧 100 点平均。
// 采集 5 秒，预期收到 ~50 条数据。
// ============================================================
func TestE2E_MultiPointAveraging_10Hz(t *testing.T) {
	dllOnceFunc()
	testE2ERate(t, 10, 5*time.Second, 0.30)
}

// ============================================================
// TestE2E_MultiPointAveraging_1Hz
// 用户采样率 1Hz，每帧 1000 点平均。
// 采集 10 秒，预期收到 ~10 条数据。
// 低频下 ReadBinary 需要等 DLL 缓冲区攒够 1000 点，10s timeout 足够。
// ============================================================
func TestE2E_MultiPointAveraging_1Hz(t *testing.T) {
	dllOnceFunc()
	testE2ERate(t, 1, 10*time.Second, 0.40)
}

// testE2ERate 是 e2e 测试的核心函数。
//
// 流程：
//   1. 构造 DAQP1603 适配器，ApplyConfig（验证采样率范围 [1,500]）
//   2. Connect + StartAcquisition（启动 readLoop 多点平均采集）
//   3. sink 回调通过 atomic 计数数据条目
//   4. 采集 duration 秒后 StopAcquisition + Disconnect
//   5. 断言：实际输出频率在 userRate × (1±tolerance) 范围内
//   6. 断言：无超时、无异常退出
func testE2ERate(t *testing.T, userRate int, duration time.Duration, tolerance float64) {
	t.Helper()

	profile := makeE2EProfile(userRate)
	d := NewDAQP1603(profile)

	if err := d.ApplyConfig(profile); err != nil {
		t.Fatalf("ApplyConfig userRate=%d: %v", userRate, err)
	}

	if err := d.Connect(); err != nil {
		t.Fatalf("Connect userRate=%d: %v", userRate, err)
	}
	defer func() { _ = d.Disconnect() }()

	var payloadCount int32
	d.SetDataSink(func(p core.DataPayload) {
		atomic.AddInt32(&payloadCount, 1)
	})

	t.Logf("开始采集: userRate=%dHz (硬件1000Hz, 每帧%d点平均), 持续%v",
		userRate, 1000/userRate, duration)
	startTime := time.Now()
	if err := d.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition userRate=%d: %v", userRate, err)
	}

	time.Sleep(duration)

	if err := d.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition userRate=%d: %v", userRate, err)
	}
	elapsed := time.Since(startTime)

	totalPayloads := int(atomic.LoadInt32(&payloadCount))
	actualRate := float64(totalPayloads) / elapsed.Seconds()
	expectedRate := float64(userRate)
	lowerBound := expectedRate * (1 - tolerance)
	upperBound := expectedRate * (1 + tolerance)

	t.Logf("  采集完成: elapsed=%v, payloads=%d, actualRate=%.1fHz",
		elapsed.Truncate(time.Millisecond), totalPayloads, actualRate)
	t.Logf("  期望范围: [%.1f, %.1f] Hz, tolerance=%.0f%%",
		lowerBound, upperBound, tolerance*100)

	if actualRate < lowerBound {
		t.Errorf("实际输出频率 %.1fHz < 期望下限 %.1fHz（用户采样率=%dHz）",
			actualRate, lowerBound, userRate)
	}
	if actualRate > upperBound {
		t.Errorf("实际输出频率 %.1fHz > 期望上限 %.1fHz（用户采样率=%dHz）",
			actualRate, upperBound, userRate)
	}

	status := d.Status()
	if status.LastError != "" {
		t.Errorf("readLoop 异常退出: %s", status.LastError)
	}
	if totalPayloads == 0 {
		t.Errorf("未收到任何数据，readLoop 可能在首帧前退出")
	}
}
