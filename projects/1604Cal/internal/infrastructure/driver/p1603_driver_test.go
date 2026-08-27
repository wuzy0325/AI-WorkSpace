package driver

// P1603 驱动 thin adapter 单测。
//
// 说明：device-sdk 的 sharedhw.DAQP1603 在 NewDAQP1603 构造时不触碰 DLL
// （DLL 仅在 Connect 时初始化），因此测试可构造无连接的 dev 实例，
// 通过 onDataFrame 注入帧直接验证缓存桥接、配置翻译与软件归零逻辑。

import (
	"context"
	"fmt"
	"testing"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"cal1604/internal/domain"
)

// testP1603Config 构造带自定义量程的 P1603 设备配置。
func testP1603Config() domain.Device {
	channels := domain.DefaultP1603Channels()
	// 通道 1 量程 0~10000 Pa（模拟 0-10000Pa 压力变送器）
	channels[0].RangeMin = 0
	channels[0].RangeMax = 10000
	channels[0].Unit = "kPa"
	// 通道 2 保持默认 ±5000 Pa
	return domain.Device{
		ID:       "p1603-1",
		Name:     "P1603采集",
		Type:     domain.DeviceTypeMeasure,
		Model:    "DAQ-P-1603",
		Host:     "192.168.1.50",
		Unit:     "kPa",
		Channels: channels,
	}
}

// TestP1603Driver_BuildSharedProfile 配置翻译：Index 1→0 转换、量程/单位透传、默认回退。
func TestP1603Driver_BuildSharedProfile(t *testing.T) {
	// 已配置量程的设备
	d := NewP1603Driver(testP1603Config())
	profile := d.buildSharedProfile()

	if len(profile.Channels) != 16 {
		t.Fatalf("expected 16 shared channels, got %d", len(profile.Channels))
	}
	// 通道 1（1-based）→ device-sdk Index 0（0-based）
	if profile.Channels[0].Index != 0 {
		t.Errorf("ch1 Index: expected 0 (0-based), got %d", profile.Channels[0].Index)
	}
	if profile.Channels[0].RangeMin != 0 || profile.Channels[0].RangeMax != 10000 {
		t.Errorf("ch1 range: expected [0,10000], got [%v,%v]", profile.Channels[0].RangeMin, profile.Channels[0].RangeMax)
	}
	if profile.Channels[0].Unit != "kPa" {
		t.Errorf("ch1 unit: expected kPa, got %s", profile.Channels[0].Unit)
	}
	// 通道 16（1-based）→ Index 15
	if profile.Channels[15].Index != 15 {
		t.Errorf("ch16 Index: expected 15, got %d", profile.Channels[15].Index)
	}
	if profile.SamplingRate != p1603SamplingRateHz {
		t.Errorf("sampling rate: expected %d, got %d", p1603SamplingRateHz, profile.SamplingRate)
	}
	if profile.Type != sharedcore.DeviceDAQP1603 {
		t.Errorf("type: expected DAQ-P-1603, got %s", profile.Type)
	}
}

// TestP1603Driver_BuildSharedProfileLeadingChannelsDisabled 验证禁用 CH1/CH2 后，
// SDK Profile 仍输出完整 16 通道。计量业务固定请求 1..16，若在 Profile 中过滤
// 禁用通道，CollectData 会因帧中缺少 CH1/CH2 而使自动采集进入错误状态。
func TestP1603Driver_BuildSharedProfileLeadingChannelsDisabled(t *testing.T) {
	config := testP1603Config()
	config.Channels[0].Enabled = false
	config.Channels[1].Enabled = false

	profile := NewP1603Driver(config).buildSharedProfile()
	if len(profile.Channels) != 16 {
		t.Fatalf("expected all 16 physical channels, got %d", len(profile.Channels))
	}
	for i, ch := range profile.Channels {
		if ch.Index != i {
			t.Fatalf("shared channel %d index: expected %d, got %d", i+1, i, ch.Index)
		}
		if !ch.Enabled {
			t.Fatalf("shared channel %d must remain enabled for full-frame collection", i+1)
		}
	}
	if profile.Channels[0].RangeMin != 0 || profile.Channels[0].RangeMax != 10000 {
		t.Fatalf("disabled CH1 range was not preserved: [%v,%v]", profile.Channels[0].RangeMin, profile.Channels[0].RangeMax)
	}
}

// TestP1603Driver_LoadTareOffsetsForDisabledChannels 验证 UI/报警未选择的通道
// 仍加载持久化归零偏移，因为计量数据帧始终包含全部 16 个物理通道。
func TestP1603Driver_LoadTareOffsetsForDisabledChannels(t *testing.T) {
	config := testP1603Config()
	config.Channels[0].Enabled = false
	config.Channels[0].TareOffset = 12.5
	d := NewP1603Driver(config)
	dev := sharedhw.NewDAQP1603(d.buildSharedProfile())

	d.loadTareOffsets(dev)

	if got := d.tareOffsets[1]; got != 12.5 {
		t.Fatalf("disabled CH1 tare offset: expected 12.5, got %v", got)
	}
}

// TestP1603Driver_BuildSharedProfileFallback 无通道配置时回退 16 通道默认（±5000 Pa）。
func TestP1603Driver_BuildSharedProfileFallback(t *testing.T) {
	d := NewP1603Driver(domain.Device{ID: "p", Host: "192.168.1.50"})
	profile := d.buildSharedProfile()
	if len(profile.Channels) != 16 {
		t.Fatalf("expected 16 fallback channels, got %d", len(profile.Channels))
	}
	if profile.Channels[0].RangeMin != -5000 || profile.Channels[0].RangeMax != 5000 {
		t.Errorf("fallback ch1 range: expected [-5000,5000], got [%v,%v]",
			profile.Channels[0].RangeMin, profile.Channels[0].RangeMax)
	}
	if profile.Channels[0].Unit != "Pa" {
		t.Errorf("fallback unit: expected Pa, got %s", profile.Channels[0].Unit)
	}
}

// TestP1603Driver_CollectDataFromCache 缓存桥接：sink 注入帧 → CollectData 按 1-based 取通道值。
func TestP1603Driver_CollectDataFromCache(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	// 直接构造 dev 实例（不连 DLL），模拟已连接状态
	d.dev = sharedhw.NewDAQP1603(d.buildSharedProfile())
	d.acquiring = true

	// 模拟 device-sdk readLoop 推送一帧：ChannelIndices 为 0-based
	d.onDataFrame(d.dev, sharedcore.DataPayload{
		Channels:       []float64{1234.5, -100.0, 300.0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	})

	ctx := context.Background()
	values, err := d.CollectData(ctx, []int{1, 2, 3})
	if err != nil {
		t.Fatalf("collect data: %v", err)
	}
	want := []float64{1234.5, -100.0, 300.0}
	if len(values) != len(want) {
		t.Fatalf("expected %d values, got %d: %v", len(want), len(values), values)
	}
	for i, w := range want {
		if values[i] != w {
			t.Errorf("channel %d: expected %v, got %v", i+1, w, values[i])
		}
	}
}

// TestP1603Driver_OnDataFrameReusesCache 验证 100Hz 回调复用帧缓存，
// 避免长时间采集时每帧分配 map 造成持续 GC 压力。
func TestP1603Driver_OnDataFrameReusesCache(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	d.dev = sharedhw.NewDAQP1603(d.buildSharedProfile())
	payload := sharedcore.DataPayload{
		Channels:       []float64{1, 2},
		ChannelIndices: []int{0, 1},
	}

	d.onDataFrame(d.dev, payload)
	first := d.latestFrame
	d.onDataFrame(d.dev, sharedcore.DataPayload{
		Channels:       []float64{3, 4},
		ChannelIndices: []int{0, 1},
	})

	if first == nil || d.latestFrame == nil {
		t.Fatal("expected frame cache to be initialized")
	}
	if fmt.Sprintf("%p", first) != fmt.Sprintf("%p", d.latestFrame) {
		t.Fatal("expected frame cache map to be reused")
	}
	if d.latestFrame[1] != 3 || d.latestFrame[2] != 4 {
		t.Fatalf("expected latest values [3 4], got %v", d.latestFrame)
	}
	d.onDataFrame(d.dev, sharedcore.DataPayload{
		Channels:       []float64{5},
		ChannelIndices: []int{0},
	})
	if _, ok := d.latestFrame[2]; ok {
		t.Fatalf("expected channels absent from latest frame to be removed, got %v", d.latestFrame)
	}
}

// TestP1603Driver_CollectDataNotConnected 未连接时 CollectData 返回明确错误。
func TestP1603Driver_CollectDataNotConnected(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	// dev 为 nil（未连接）
	if _, err := d.CollectData(context.Background(), []int{1}); err == nil {
		t.Fatal("expected not-connected error")
	}
}

// TestP1603Driver_StaleDevCallbackIgnored 代际保护：旧 dev 的迟到回调不污染新缓存。
// 模拟 Disconnect（d.dev=nil）后旧 readLoop 仍回调 → 必须被忽略。
func TestP1603Driver_StaleDevCallbackIgnored(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	oldDev := sharedhw.NewDAQP1603(d.buildSharedProfile())
	d.dev = oldDev

	// 旧 dev 回调：d.dev 仍指向 oldDev，应写入
	d.onDataFrame(oldDev, sharedcore.DataPayload{
		Channels:       []float64{111.0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	})
	d.mu.Lock()
	if !d.frameValid {
		d.mu.Unlock()
		t.Fatal("expected callback from current dev to update frame")
	}
	d.mu.Unlock()

	// 模拟 Disconnect：d.dev=nil（旧 dev 还在跑）
	d.mu.Lock()
	d.dev = nil
	d.mu.Unlock()

	// 旧 dev 迟到回调：必须被忽略（d.dev != oldDev）
	d.onDataFrame(oldDev, sharedcore.DataPayload{
		Channels:       []float64{222.0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	})
	d.mu.Lock()
	// 缓存应保留旧值（未被 222 覆盖），且 frameValid 仍为 true（上一帧数据）
	if v, ok := d.latestFrame[1]; !ok || v != 111.0 {
		d.mu.Unlock()
		t.Errorf("stale callback should not overwrite cache: expected ch1=111.0, got %v (ok=%v)", v, ok)
	}
	d.mu.Unlock()
}

// TestP1603Driver_CalibrateZero 软件归零：读当前值 → SetTare(0-based, 当前值)，
// 且后续 CollectData 扣除偏移（展示值 = 原始值 - offset）。
func TestP1603Driver_CalibrateZero(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	d.dev = sharedhw.NewDAQP1603(d.buildSharedProfile())
	d.acquiring = true

	// 通道 1 当前读数 1234.5（0-based index 0）
	d.onDataFrame(d.dev, sharedcore.DataPayload{
		Channels:       []float64{1234.5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	})

	ctx := context.Background()
	results, err := d.CalibrateZero(ctx, []int{1})
	if err != nil {
		t.Fatalf("calibrate zero: %v", err)
	}
	if len(results) != 1 || results[0] != 1234.5 {
		t.Errorf("calibrate zero results: expected [1234.5], got %v", results)
	}

	// 验证 TareOffset 已写入 dev（0-based index 0）
	tare, err := d.dev.GetTare(0)
	if err != nil {
		t.Fatalf("get tare: %v", err)
	}
	if tare != 1234.5 {
		t.Errorf("tare offset: expected 1234.5, got %v", tare)
	}

	// 归零后重新注入一帧（原始值仍 1234.5），CollectData 必须扣除偏移 → 0
	d.onDataFrame(d.dev, sharedcore.DataPayload{
		Channels:       []float64{1234.5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	})
	values, err := d.CollectData(ctx, []int{1})
	if err != nil {
		t.Fatalf("collect after tare: %v", err)
	}
	if values[0] != 0 {
		t.Errorf("collected value after tare: expected 0 (1234.5-1234.5), got %v", values[0])
	}
}

// TestP1603Driver_CalibrateZeroConcurrentCollect 校零与实时采集并发。
//
// 背景：真实环境下实时数据轮询（CollectData 加锁读 tareOffsets）与用户点击
// 校零（CalibrateZero 写 tareOffsets）会并发。若 map 写在锁外，会产生数据竞争：
// 轻则读到撕裂/空 map 导致归零偏移未生效（采集值退回原始未校零读数），重则 Go
// 并发 map 读写直接 panic。本用例并发反复执行 CalibrateZero 与 CollectData，
// 验证归零后采集值始终扣除偏移（≈0），且不 panic。
func TestP1603Driver_CalibrateZeroConcurrentCollect(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	d.dev = sharedhw.NewDAQP1603(d.buildSharedProfile())
	d.acquiring = true

	// 固定帧：原始值恒 1234.5（0-based index 0）
	inject := func() {
		d.onDataFrame(d.dev, sharedcore.DataPayload{
			Channels:       []float64{1234.5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		})
	}
	inject()

	ctx := context.Background()
	stop := make(chan struct{})
	errCh := make(chan error, 1)

	// 并发采集方：模拟 100Hz 实时轮询，反复读取通道 1。
	go func() {
		defer close(errCh)
		for {
			select {
			case <-stop:
				return
			default:
			}
			vals, err := d.CollectData(ctx, []int{1})
			if err != nil {
				errCh <- err
				return
			}
			// 归零后采集值应始终扣除偏移（约 0），若读到原始值 1234.5 说明
			// 归零偏移丢失（旧竞态下可稳定复现）。
			if vals[0] != 0 {
				errCh <- fmt.Errorf("collected raw value %v during/after zero, tare not applied", vals[0])
				return
			}
		}
	}()

	// 并发校零方：反复对通道 1 校零，每次都应返回原始读数 1234.5。
	for i := 0; i < 200; i++ {
		inject()
		results, err := d.CalibrateZero(ctx, []int{1})
		if err != nil {
			close(stop)
			<-errCh
			t.Fatalf("calibrate zero: %v", err)
		}
		if len(results) != 1 || results[0] != 1234.5 {
			close(stop)
			<-errCh
			t.Fatalf("calibrate zero results: expected [1234.5], got %v", results)
		}
	}
	close(stop)
	if err := <-errCh; err != nil {
		t.Fatalf("concurrent collect: %v", err)
	}
}

// TestP1603Driver_ValveStub 阀门桩：恒 calibration / SetValveStatus 空操作。
func TestP1603Driver_ValveStub(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	ctx := context.Background()

	status, err := d.ReadValveStatus(ctx)
	if err != nil {
		t.Fatalf("read valve status: %v", err)
	}
	if status != "calibration" {
		t.Errorf("expected calibration, got %s", status)
	}
	if err := d.SetValveStatus(ctx, "measurement"); err != nil {
		t.Errorf("set valve status should be no-op, got: %v", err)
	}
}

// TestP1603Driver_UnitStub 单位桩：ReadUnit 返回配置单位，SetUnit 软件层更新。
func TestP1603Driver_UnitStub(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	ctx := context.Background()

	unit, err := d.ReadUnit(ctx)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if unit != "kPa" {
		t.Errorf("ReadUnit: expected kPa (from device Unit), got %s", unit)
	}

	if err := d.SetUnit(ctx, "Pa"); err != nil {
		t.Fatalf("set unit: %v", err)
	}
	unit, _ = d.ReadUnit(ctx)
	if unit != "Pa" {
		t.Errorf("ReadUnit after SetUnit: expected Pa, got %s", unit)
	}

	// 空单位必须拒绝
	if err := d.SetUnit(ctx, ""); err == nil {
		t.Error("expected error for empty unit")
	}
}

// TestP1603Driver_FullScaleNotSupported 满量程校准返回明确不支持错误。
func TestP1603Driver_FullScaleNotSupported(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	if _, err := d.CalibrateFullScale(context.Background(), []int{1}, 100); err == nil {
		t.Fatal("expected not-supported error")
	}
}

// TestP1603Driver_ResetNotSupported Reset 返回明确不支持。
func TestP1603Driver_ResetNotSupported(t *testing.T) {
	d := NewP1603Driver(testP1603Config())
	if err := d.Reset(context.Background()); err == nil {
		t.Fatal("expected not-supported error")
	}
}
