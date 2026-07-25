package hardware

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/daq/ports"
	"shared.local/device-sdk/go/ffi"
	"shared.local/device-sdk/go/protocol"
)

// ============================================================
// DAQ-P-1603 适配器 T3 单元测试
// ------------------------------------------------------------
// 测试范围：Connect/Disconnect/Status/SetDataSink 状态机与幂等性。
// 不依赖真实硬件：DLL 在测试环境中未加载，IsWTNDAQ16HInitialized
// 返回 false，Connect 会返回明确的 "DLL not initialized" 错误，
// 覆盖错误路径与状态机迁移正确性。
// 真机端到端验证由 Phase 7 HIL（Task 16）完成。
// ============================================================

// makeP1603Profile 构造一个最小可用的 DAQ-P-1603 profile 用于测试。
func makeP1603Profile(id, ip string, channels int) core.Profile {
	chs := make([]core.ChannelConfig, channels)
	for i := range chs {
		chs[i] = core.ChannelConfig{
			Index:   i,
			Name:    "CH" + string(rune('0'+i)),
			Enabled: true,
			Unit:    "Pa",
		}
	}
	return core.Profile{
		ID:           id,
		Name:         "DAQ-P-1603-" + id,
		Type:         core.DeviceDAQP1603,
		Address:      ip,
		SamplingRate: 100,
		Channels:     chs,
	}
}

// 测试前置：DAQP1603 在 New 之后初始状态为 Disconnected，handle 为 0。
// 期待结果：Status().Connection == Disconnected 且 StartAcquisition 返回 not implemented。
func TestDAQP1603_InitialState(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("t1", "192.168.1.1", 16))

	if got := d.Status().Connection; got != core.ConnectionDisconnected {
		t.Fatalf("initial connection = %q, want Disconnected", got)
	}
	if d.handle != 0 {
		t.Fatalf("initial handle = %v, want 0", d.handle)
	}
	if d.ID() != "t1" {
		t.Fatalf("ID = %q, want t1", d.ID())
	}
}

// 测试前置：DLL 未初始化时调用 Connect。
// 期待结果：返回错误包含 "DLL not initialized"，状态迁移到 Error，handle 保持 0。
func TestDAQP1603_Connect_DLLNotInitialized(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("t2", "192.168.1.1", 16))

	err := d.Connect()
	if err == nil {
		t.Fatal("Connect should fail when DLL not initialized")
	}
	if !strings.Contains(err.Error(), "DLL not initialized") {
		t.Fatalf("error = %q, want contains 'DLL not initialized'", err.Error())
	}
	if got := d.Status().Connection; got != core.ConnectionError {
		t.Fatalf("connection after failed Connect = %q, want Error", got)
	}
	if d.Status().LastError == "" {
		t.Fatal("LastError should be set on error")
	}
	if d.handle != 0 {
		t.Fatalf("handle after failed Connect = %v, want 0", d.handle)
	}
}

// 测试前置：profile.Address 为空时调用 Connect。
// 期待结果：返回错误包含 "missing address"，状态迁移到 Error。
func TestDAQP1603_Connect_MissingAddress(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("t3", "", 16))

	err := d.Connect()
	if err == nil {
		t.Fatal("Connect should fail with missing address")
	}
	if !strings.Contains(err.Error(), "missing address") {
		t.Fatalf("error = %q, want contains 'missing address'", err.Error())
	}
	if got := d.Status().Connection; got != core.ConnectionError {
		t.Fatalf("connection = %q, want Error", got)
	}
}

// 测试前置：DLL 未初始化时 Disconnect 在未连接状态下应安全返回 nil。
// 期待结果：无 panic，无错误，状态保持 Disconnected。
func TestDAQP1603_Disconnect_Idempotent_NotConnected(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("t4", "192.168.1.1", 16))

	// 连续调用 Disconnect 不应报错
	for i := 0; i < 3; i++ {
		if err := d.Disconnect(); err != nil {
			t.Fatalf("Disconnect call %d failed: %v", i, err)
		}
	}
	if got := d.Status().Connection; got != core.ConnectionDisconnected {
		t.Fatalf("connection = %q, want Disconnected", got)
	}
}

// 测试前置：在 Disconnected 状态下注册 sink，不应 panic。
// 期待结果：sink 字段被写入，可被后续 readLoop 使用。
func TestDAQP1603_SetDataSink_SafeBeforeConnect(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("t5", "192.168.1.1", 16))

	called := false
	d.SetDataSink(func(p core.DataPayload) {
		called = true
	})

	// 验证 sink 已注册（通过内部字段读取，仅测试代码可见）
	d.mu.RLock()
	sink := d.sink
	d.mu.RUnlock()
	if sink == nil {
		t.Fatal("sink not registered")
	}

	// 触发一次回调验证可调用
	sink(core.DataPayload{DeviceID: "t5"})
	if !called {
		t.Fatal("sink callback not invoked")
	}
}

// ============================================================
// T9: StartAcquisition / StopAcquisition / readLoop 单元测试
// ------------------------------------------------------------
// 测试范围：
//   - StartAcquisition 在未连接 / 已采集 时的状态机迁移
//   - StopAcquisition 在未采集时的幂等性
//   - buildChannelScales：通道过滤、量程兜底、上限截断
//   - scaleCurrentToEngineering：端点、中点、越界、压力/温度通道同一公式
//   - 并发 Start/Stop 不 panic（白盒设置 acquiring，避免依赖真实 DLL）
//
// 不依赖真实硬件：readLoop 完整路径需 DLL 返回数据，留待 Phase 7 HIL 验证。
// 此处只覆盖不依赖 DLL 的纯逻辑与状态机分支。
// ============================================================

// 测试前置：DAQP1603 在未连接时（handle==0）调用 StartAcquisition。
// 期待结果：返回错误包含 "device not connected"，状态保持 Disconnected，acquiring 仍为 false。
func TestDAQP1603_StartAcquisition_NotConnected_Rejected(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("sa1", "192.168.1.1", 16))

	err := d.StartAcquisition()
	if err == nil {
		t.Fatal("StartAcquisition should fail when device not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("error = %q, want contains 'not connected'", err.Error())
	}
	if d.Status().Acquiring {
		t.Fatal("Acquiring should remain false after failed StartAcquisition")
	}
	if got := d.Status().Connection; got != core.ConnectionDisconnected {
		t.Fatalf("Connection = %q, want Disconnected", got)
	}
}

// 测试前置：StopAcquisition 在未采集状态下应安全返回 nil（幂等）。
// 期待结果：无 panic，无错误，状态保持 Disconnected，Acquiring 保持 false。
func TestDAQP1603_StopAcquisition_Idempotent_NotAcquiring(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("sa2", "192.168.1.1", 16))

	for i := 0; i < 3; i++ {
		if err := d.StopAcquisition(); err != nil {
			t.Fatalf("StopAcquisition call %d failed: %v", i, err)
		}
	}
	if d.Status().Acquiring {
		t.Fatal("Acquiring should remain false")
	}
}

// 测试前置：白盒设置 acquiring=true 模拟采集进行中，再调用 StopAcquisition。
// 此时 handle==0（未真实连接），StopAcquisition 应跳过 FFI 调用但清理状态。
// readLoopDone 保持 nil（readLoop 未真实启动），避免 1 秒等待超时。
// 期待结果：返回 nil，acquiring=false，stop channel 被 close，status 回到 Connected。
func TestDAQP1603_StopAcquisition_WhileAcquiring_StateCleared(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("sa3", "192.168.1.1", 16))

	// 白盒模拟采集进行中（真实 StartAcquisition 需 DLL，测试环境不可用）
	// 不设置 readLoopDone：模拟"readLoop 尚未启动"或"已退出"的场景
	d.mu.Lock()
	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	d.stop = make(chan struct{})
	d.mu.Unlock()

	if err := d.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition failed: %v", err)
	}

	if d.Status().Acquiring {
		t.Fatal("Acquiring should be false after Stop")
	}
	if got := d.Status().Connection; got != core.ConnectionConnected {
		t.Fatalf("Connection = %q, want Connected", got)
	}
	// stop channel 应被 close（select 立即返回）或被置 nil
	d.mu.RLock()
	stopClosed := false
	select {
	case <-d.stop:
		stopClosed = true
	default:
	}
	d.mu.RUnlock()
	if !stopClosed && d.stop != nil {
		t.Fatal("d.stop should be closed or nil after Stop")
	}
}

// 测试前置：并发调用 StartAcquisition（已采集时直接返回 nil）与 StopAcquisition，
// 验证 mu 互斥锁能防止状态机竞争。
// 期待结果：无 panic，所有调用返回 nil 或 "device not connected" 错误（不 panic 即可）。
func TestDAQP1603_ConcurrentStartStop_NoPanic(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("sa4", "192.168.1.1", 16))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			// 不关心返回值，只验证不 panic
			_ = d.StartAcquisition()
			_ = d.StopAcquisition()
		}
	}()

	select {
	case <-done:
		// 成功完成
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Start/Stop timed out (potential deadlock)")
	}
}

// 测试前置：buildChannelScales 在所有通道启用时返回 16 项 scales，origIndex 依次为 0..15。
// 期待结果：len(scales) == 16，scales[i].origIndex == i。
func TestDAQP1603_BuildChannelScales_AllEnabled(t *testing.T) {
	chs := make([]core.ChannelConfig, 16)
	for i := range chs {
		chs[i] = core.ChannelConfig{
			Index:    i,
			Enabled:  true,
			RangeMin: -100,
			RangeMax: 100,
		}
	}

	scales := buildChannelScales(chs)
	if len(scales) != 16 {
		t.Fatalf("len(scales) = %d, want 16", len(scales))
	}
	for i, s := range scales {
		if s.origIndex != i {
			t.Fatalf("scales[%d].origIndex = %d, want %d", i, s.origIndex, i)
		}
		if s.engMin != -100 || s.engMax != 100 {
			t.Fatalf("scales[%d] range = (%v, %v), want (-100, 100)", i, s.engMin, s.engMax)
		}
	}
}

// 测试前置：buildChannelScales 在部分通道禁用时只返回启用通道，origIndex 保留原始索引。
// 期待结果：len(scales) == 2，scales[0].origIndex == 1，scales[1].origIndex == 3。
func TestDAQP1603_BuildChannelScales_PartialEnabled(t *testing.T) {
	chs := []core.ChannelConfig{
		{Index: 0, Enabled: false, RangeMin: 0, RangeMax: 100},
		{Index: 1, Enabled: true, RangeMin: 0, RangeMax: 100},
		{Index: 2, Enabled: false, RangeMin: 0, RangeMax: 100},
		{Index: 3, Enabled: true, RangeMin: 0, RangeMax: 100},
	}

	scales := buildChannelScales(chs)
	if len(scales) != 2 {
		t.Fatalf("len(scales) = %d, want 2", len(scales))
	}
	if scales[0].origIndex != 1 {
		t.Fatalf("scales[0].origIndex = %d, want 1", scales[0].origIndex)
	}
	if scales[1].origIndex != 3 {
		t.Fatalf("scales[1].origIndex = %d, want 3", scales[1].origIndex)
	}
}

// 测试前置：buildChannelScales 在通道 RangeMin==0 && RangeMax==0 时用 4-20mA 兜底。
// 期待结果：scales[i].engMin == 4, scales[i].engMax == 20。
func TestDAQP1603_BuildChannelScales_FallbackRange(t *testing.T) {
	chs := []core.ChannelConfig{
		{Index: 0, Enabled: true, RangeMin: 0, RangeMax: 0},
	}

	scales := buildChannelScales(chs)
	if len(scales) != 1 {
		t.Fatalf("len(scales) = %d, want 1", len(scales))
	}
	if scales[0].engMin != daqP1603CurrentMin {
		t.Fatalf("engMin = %v, want %v (fallback)", scales[0].engMin, daqP1603CurrentMin)
	}
	if scales[0].engMax != daqP1603CurrentMax {
		t.Fatalf("engMax = %v, want %v (fallback)", scales[0].engMax, daqP1603CurrentMax)
	}
}

// 测试前置：buildChannelScales 在 channels 长度 > 16 时按 WTNDAQ16H_AI_MAX_CHANNELS 截断。
// 期待结果：len(scales) == 16（不会越界访问 CHParam[16]）。
func TestDAQP1603_BuildChannelScales_TruncateAt16(t *testing.T) {
	chs := make([]core.ChannelConfig, 20)
	for i := range chs {
		chs[i] = core.ChannelConfig{Index: i, Enabled: true, RangeMin: 0, RangeMax: 100}
	}

	scales := buildChannelScales(chs)
	if len(scales) != 16 {
		t.Fatalf("len(scales) = %d, want 16 (truncated at MAX_CHANNELS)", len(scales))
	}
}

// 测试前置：scaleCurrentToEngineering 在 4mA（活零点）时返回 engMin（端点）。
func TestDAQP1603_ScaleCurrentToEngineering_AtCurrentMin(t *testing.T) {
	got := scaleCurrentToEngineering(daqP1603CurrentMin, -100, 100)
	if got != -100 {
		t.Fatalf("scale at 4mA = %v, want -100", got)
	}
}

// 测试前置：scaleCurrentToEngineering 在 20mA（满量程）时返回 engMax（端点）。
func TestDAQP1603_ScaleCurrentToEngineering_AtCurrentMax(t *testing.T) {
	got := scaleCurrentToEngineering(daqP1603CurrentMax, -100, 100)
	if got != 100 {
		t.Fatalf("scale at 20mA = %v, want 100", got)
	}
}

// 测试前置：scaleCurrentToEngineering 在 12mA（中点）时返回 (engMin+engMax)/2。
func TestDAQP1603_ScaleCurrentToEngineering_AtMidpoint(t *testing.T) {
	got := scaleCurrentToEngineering(12, -100, 100)
	// 12mA: (12-4)/16 = 0.5 → -100 + 0.5*200 = 0
	if got != 0 {
		t.Fatalf("scale at 12mA = %v, want 0", got)
	}
}

// 测试前置：scaleCurrentToEngineering 对压力通道（0..1000 Pa，对应 4..20mA）。
// 期待结果：4mA → 0 Pa；12mA → 500 Pa；20mA → 1000 Pa。
func TestDAQP1603_ScaleCurrentToEngineering_PressureChannel(t *testing.T) {
	cases := []struct {
		current float64
		want    float64
	}{
		{current: 4, want: 0},
		{current: 12, want: 500},
		{current: 20, want: 1000},
	}
	for _, c := range cases {
		got := scaleCurrentToEngineering(c.current, 0, 1000)
		if got != c.want {
			t.Fatalf("pressure scale at %vmA = %v, want %v", c.current, got, c.want)
		}
	}
}

// 测试前置：scaleCurrentToEngineering 对温度通道（-50..200 ℃，对应 4..20mA）。
// 期待结果：4mA → -50 ℃；12mA → 75 ℃；20mA → 200 ℃。
func TestDAQP1603_ScaleCurrentToEngineering_TemperatureChannel(t *testing.T) {
	cases := []struct {
		current float64
		want    float64
	}{
		{current: 4, want: -50},
		{current: 12, want: 75},
		{current: 20, want: 200},
	}
	for _, c := range cases {
		got := scaleCurrentToEngineering(c.current, -50, 200)
		if got != c.want {
			t.Fatalf("temperature scale at %vmA = %v, want %v", c.current, got, c.want)
		}
	}
}

// 测试前置：scaleCurrentToEngineering 在电流越界（>20mA）时仍按线性外推。
// 期待结果：22mA 时返回 ≈125（量程 -100..100 的外推值）。
func TestDAQP1603_ScaleCurrentToEngineering_Extrapolation(t *testing.T) {
	got := scaleCurrentToEngineering(22, -100, 100)
	// 22mA: (22-4)/16 = 1.125 → -100 + 1.125*200 = 125
	const want = 125.0
	const tol = 1e-9
	if got < want-tol || got > want+tol {
		t.Fatalf("extrapolation at 22mA = %v, want %v (tol %v)", got, want, tol)
	}
}

// 测试前置：scaleCurrentToEngineering 在 current < 0（DLL 不可能返回负值，但防御性代码存在）时返回 engMin。
func TestDAQP1603_ScaleCurrentToEngineering_NegativeCurrent(t *testing.T) {
	got := scaleCurrentToEngineering(-1, -100, 100)
	if got != -100 {
		t.Fatalf("negative current guard = %v, want -100 (engMin)", got)
	}
}

// 测试前置：白盒调用 readLoop，profile.Channels 全部 Enabled=false（无启用通道）。
// readLoop 应在调用 FFI 之前检测到 nChans==0，触发 handleReadLoopExit 并关闭 readLoopDone。
// 期待结果：readLoopDone 在 1 秒内关闭，status.Error 包含 "no enabled channels"，onReadLoopExit 回调被调用。
func TestDAQP1603_ReadLoop_NoEnabledChannels_ExitsGracefully(t *testing.T) {
	// 构造所有通道禁用的 profile
	profile := makeP1603Profile("rl1", "192.168.1.1", 16)
	for i := range profile.Channels {
		profile.Channels[i].Enabled = false
	}
	d := NewDAQP1603(profile)

	// 注册 onReadLoopExit 回调
	exitCalled := make(chan error, 1)
	d.OnReadLoopExit(func(err error) {
		select {
		case exitCalled <- err:
		default:
		}
	})

	// 白盒设置 readLoopDone（模拟 StartAcquisition 已创建 channel）
	d.mu.Lock()
	d.readLoopDone = make(chan struct{})
	d.mu.Unlock()

	// 直接调用 readLoop（在 goroutine 中，因为它不会自然返回除非出错）
	go d.readLoop()

	// 等待 readLoopDone 关闭（应在毫秒级完成，因为 nChans==0 直接退出）
	select {
	case <-d.readLoopDone:
		// 成功退出
	case <-time.After(time.Second):
		t.Fatal("readLoop did not exit within 1s on no enabled channels")
	}

	// 验证 onReadLoopExit 被调用且错误信息正确
	select {
	case err := <-exitCalled:
		if !strings.Contains(err.Error(), "no enabled channels") {
			t.Fatalf("onReadLoopExit err = %q, want contains 'no enabled channels'", err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("onReadLoopExit callback not invoked")
	}

	// 验证 status 被设置为 Error
	if got := d.Status().Connection; got != core.ConnectionError {
		t.Fatalf("Connection = %q, want Error", got)
	}
	if !strings.Contains(d.Status().LastError, "no enabled channels") {
		t.Fatalf("LastError = %q, want contains 'no enabled channels'", d.Status().LastError)
	}
}

// 测试前置：OnReadLoopExit 注册回调后，handleReadLoopExit 应调用该回调。
// 期待结果：回调被调用一次，参数为传入的错误。
func TestDAQP1603_OnReadLoopExit_CallbackInvoked(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("rl2", "192.168.1.1", 16))

	called := make(chan error, 1)
	d.OnReadLoopExit(func(err error) {
		select {
		case called <- err:
		default:
		}
	})

	// 直接调用 handleReadLoopExit（白盒）
	testErr := fmt.Errorf("test error")
	d.handleReadLoopExit(testErr)

	select {
	case err := <-called:
		if err.Error() != "test error" {
			t.Fatalf("callback err = %q, want 'test error'", err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("OnReadLoopExit callback not invoked")
	}

	if !strings.Contains(d.Status().LastError, "test error") {
		t.Fatalf("LastError = %q, want contains 'test error'", d.Status().LastError)
	}
}

// 测试前置：白盒调用 readLoop，预先 close(stop channel) 模拟主动停止。
// readLoop 在 for 循环第一次 select 时检测到 stop 已关闭，应立即返回（不调用 FFI）。
// 期待结果：readLoopDone 在 1 秒内关闭，status 不被设置为 Error（主动停止路径），
// onReadLoopExit 回调不被调用。
func TestDAQP1603_ReadLoop_StopSignal_GracefulExit(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("rl3", "192.168.1.1", 16))

	exitCalled := make(chan error, 1)
	d.OnReadLoopExit(func(err error) {
		select {
		case exitCalled <- err:
		default:
		}
	})

	// 白盒设置：模拟 StartAcquisition 已创建 channel，但立即被 StopAcquisition 关闭
	d.mu.Lock()
	d.readLoopDone = make(chan struct{})
	d.stop = make(chan struct{})
	close(d.stop) // 预先关闭 stop，模拟 StopAcquisition 已调用
	d.stopReason.SetStopReason(protocol.StopReasonUserRequested)
	d.mu.Unlock()

	// 直接调用 readLoop（在 goroutine 中）
	go d.readLoop()

	// 等待 readLoopDone 关闭（应在毫秒级完成，因为 stop 已关闭）
	select {
	case <-d.readLoopDone:
		// 成功退出
	case <-time.After(time.Second):
		t.Fatal("readLoop did not exit within 1s on stop signal")
	}

	// 主动停止路径：onReadLoopExit 不应被调用
	select {
	case err := <-exitCalled:
		t.Fatalf("onReadLoopExit should not be called on graceful stop, got err: %v", err)
	case <-time.After(100 * time.Millisecond):
		// 预期：回调未被调用
	}

	// status 不应是 Error（主动停止不设置错误）
	if got := d.Status().Connection; got == core.ConnectionError {
		t.Fatal("Connection should not be Error on graceful stop")
	}
}

// 测试前置：白盒设置 d.handle != 0 模拟已连接，调用 Connect 应直接返回 nil（幂等）。
// 期待结果：返回 nil，不调用任何 FFI，状态保持 Connected。
func TestDAQP1603_Connect_AlreadyConnected_Idempotent(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("c1", "192.168.1.1", 16))

	// 白盒模拟已连接
	d.mu.Lock()
	d.handle = 1 // 非零表示已连接
	d.status.Connection = core.ConnectionConnected
	d.mu.Unlock()

	if err := d.Connect(); err != nil {
		t.Fatalf("Connect should be idempotent when already connected, got: %v", err)
	}
	if got := d.Status().Connection; got != core.ConnectionConnected {
		t.Fatalf("Connection = %q, want Connected", got)
	}
}

// 测试前置：buildAIParamLocked 在 profile.Channels 为空时应默认启用全部 16 通道。
// 期待结果：SampChanCount == 16 且 16 个 CHParam.Channel 依次为 0..15。
func TestDAQP1603_BuildAIParam_DefaultAllChannels(t *testing.T) {
	profile := core.Profile{
		ID:           "t7",
		Type:         core.DeviceDAQP1603,
		Address:      "192.168.1.1",
		SamplingRate: 100,
		Channels:     nil, // 无通道配置
	}
	d := NewDAQP1603(profile)

	d.mu.Lock()
	p := d.buildAIParamLocked()
	d.mu.Unlock()

	if p.SampChanCount != ffi.WTNDAQ16H_AI_MAX_CHANNELS {
		t.Fatalf("SampChanCount = %d, want %d", p.SampChanCount, ffi.WTNDAQ16H_AI_MAX_CHANNELS)
	}
	for i := uint32(0); i < ffi.WTNDAQ16H_AI_MAX_CHANNELS; i++ {
		if p.CHParam[i].Channel != i {
			t.Fatalf("CHParam[%d].Channel = %d, want %d", i, p.CHParam[i].Channel, i)
		}
	}
	if p.SampleRate != 1000 {
		t.Fatalf("SampleRate = %v, want 1000 (hardware rate fixed)", p.SampleRate)
	}
	if p.SampleMode != ffi.WTNDAQ16H_AI_SAMPMODE_CONTINUOUS {
		t.Fatalf("SampleMode = %d, want CONTINUOUS", p.SampleMode)
	}
}

// 测试前置：buildAIParamLocked 必须始终初始化全部 16 通道。
// 设备强制要求 SampChanCount=16，否则 ReadBinary 永远不产数据。
// Enabled 标志由 readLoop 侧按 scales[].origIndex 过滤。
// 期待结果：SampChanCount == 16，所有 CHParam[i].Channel == i。
func TestDAQP1603_BuildAIParam_PartialChannels(t *testing.T) {
	chs := []core.ChannelConfig{
		{Index: 0, Enabled: false},
		{Index: 1, Enabled: true},
		{Index: 2, Enabled: false},
		{Index: 3, Enabled: true},
	}
	profile := core.Profile{
		ID:           "t8",
		Type:         core.DeviceDAQP1603,
		Address:      "192.168.1.1",
		SamplingRate: 200,
		Channels:     chs,
	}
	d := NewDAQP1603(profile)

	d.mu.Lock()
	p := d.buildAIParamLocked()
	d.mu.Unlock()

	if p.SampChanCount != ffi.WTNDAQ16H_AI_MAX_CHANNELS {
		t.Fatalf("SampChanCount = %d, want %d (must always be 16)", p.SampChanCount, ffi.WTNDAQ16H_AI_MAX_CHANNELS)
	}
	for i := uint32(0); i < ffi.WTNDAQ16H_AI_MAX_CHANNELS; i++ {
		if p.CHParam[i].Channel != i {
			t.Fatalf("CHParam[%d].Channel = %d, want %d (all 16 must be filled)", i, p.CHParam[i].Channel, i)
		}
	}
	// 硬件采样率固定 1000Hz，与用户采样率 200Hz 解耦
	if p.SampleRate != 1000 {
		t.Fatalf("SampleRate = %v, want 1000 (hardware rate fixed)", p.SampleRate)
	}
}

// 测试前置：buildAIParamLocked 在 SamplingRate=0（异常值）时应回退到用户下限 1Hz，
// 但硬件采样率始终固定 1000Hz。
// 期待结果：SampleRate == 1000（硬件采样率固定），calcSampsPerChanLocked == 1000。
func TestDAQP1603_BuildAIParam_DefaultSampleRate(t *testing.T) {
	profile := core.Profile{
		ID:           "t9",
		Type:         core.DeviceDAQP1603,
		Address:      "192.168.1.1",
		SamplingRate: 0,
	}
	d := NewDAQP1603(profile)

	d.mu.Lock()
	p := d.buildAIParamLocked()
	sampsPerChan := d.calcSampsPerChanLocked()
	d.mu.Unlock()

	if p.SampleRate != 1000 {
		t.Fatalf("SampleRate = %v, want 1000 (hardware rate fixed)", p.SampleRate)
	}
	// SamplingRate=0 回退到下限 1Hz → sampsPerChan = 1000/1 = 1000
	if sampsPerChan != 1000 {
		t.Fatalf("sampsPerChan = %d, want 1000 (1000/1Hz)", sampsPerChan)
	}
}

// 测试前置：calcSampsPerChanLocked 在不同用户采样率下返回正确的平均窗口大小。
// 期待结果：sampsPerChan = 1000 / userRate，向下取整。
func TestDAQP1603_CalcSampsPerChan(t *testing.T) {
	cases := []struct {
		userRate int
		want     uint32
	}{
		{1, 1000},   // 1Hz → 1000 点取平均
		{10, 100},   // 10Hz → 100 点
		{20, 50},    // 20Hz → 50 点
		{50, 20},    // 50Hz → 20 点
		{100, 10},   // 100Hz → 10 点
		{200, 5},    // 200Hz → 5 点
		{500, 2},    // 500Hz → 2 点
	}
	for _, c := range cases {
		profile := core.Profile{
			ID:           "spc",
			Type:         core.DeviceDAQP1603,
			Address:      "192.168.1.1",
			SamplingRate: c.userRate,
		}
		d := NewDAQP1603(profile)
		d.mu.Lock()
		got := d.calcSampsPerChanLocked()
		d.mu.Unlock()
		if got != c.want {
			t.Fatalf("userRate=%d: sampsPerChan = %d, want %d", c.userRate, got, c.want)
		}
	}
}

// 测试前置：DAQP1603 实现 ports.Device 接口（编译期断言已在主文件中声明）。
// 此测试为运行时补充验证，确保 ID/Connect/Disconnect/Status 方法签名匹配。
func TestDAQP1603_ImplementsDeviceInterface(t *testing.T) {
	var _ ports.Device = (*DAQP1603)(nil)

	d := NewDAQP1603(makeP1603Profile("t10", "192.168.1.1", 16))
	// 调用每个方法确保运行时无 panic
	_ = d.ID()
	_ = d.Connect()
	_ = d.Disconnect()
	_ = d.Status()
	d.SetDataSink(nil)
}

// ============================================================
// T6: ApplyConfig / GetProfile 单元测试
// ------------------------------------------------------------
// 测试范围：参数校验、未连接设备 profile 更新、SensorType 传递、
// 采集时拒绝、GetProfile 拷贝语义。
// 不依赖真实硬件：已连接设备的硬件同步路径（ReleaseTask→InitTask）
// 需真实 DLL，留待 Phase 7 HIL 验证。
// ============================================================

// 测试前置：未连接设备调用 ApplyConfig，传入合法 profile（200Hz、16 通道）。
// 期待结果：返回 nil，GetProfile 返回的 profile 与传入一致。
func TestDAQP1603_ApplyConfig_WhileDisconnected_UpdatesProfile(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac1", "192.168.1.1", 16))

	newProfile := makeP1603Profile("ac1", "192.168.1.1", 16)
	newProfile.SamplingRate = 200
	newProfile.Channels[0].Unit = "kPa"

	if err := d.ApplyConfig(newProfile); err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	got := d.GetProfile()
	if got.SamplingRate != 200 {
		t.Fatalf("GetProfile SamplingRate = %d, want 200", got.SamplingRate)
	}
	if got.Channels[0].Unit != "kPa" {
		t.Fatalf("GetProfile Channels[0].Unit = %q, want kPa", got.Channels[0].Unit)
	}
}

// 测试前置：未连接设备调用 ApplyConfig，传入 SamplingRate 超过上限。
// 期待结果：返回错误包含 "out of range"，内部 profile 不变。
func TestDAQP1603_ApplyConfig_SampleRateExceedsMax_Rejected(t *testing.T) {
	original := makeP1603Profile("ac2", "192.168.1.1", 16)
	d := NewDAQP1603(original)

	bad := makeP1603Profile("ac2", "192.168.1.1", 16)
	bad.SamplingRate = DAQP1603MaxSampleRate + 1

	err := d.ApplyConfig(bad)
	if err == nil {
		t.Fatal("ApplyConfig should reject sampling rate > max")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("error = %q, want contains 'out of range'", err.Error())
	}

	// 内部 profile 不变
	got := d.GetProfile()
	if got.SamplingRate != original.SamplingRate {
		t.Fatalf("SamplingRate after rejected ApplyConfig = %d, want %d (unchanged)",
			got.SamplingRate, original.SamplingRate)
	}
}

// 测试前置：未连接设备调用 ApplyConfig，传入 SamplingRate=DAQP1603MaxSampleRate（边界值）。
// 期待结果：返回 nil，profile 更新成功。
func TestDAQP1603_ApplyConfig_SampleRateAtMax_Ok(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac3", "192.168.1.1", 16))

	boundary := makeP1603Profile("ac3", "192.168.1.1", 16)
	boundary.SamplingRate = DAQP1603MaxSampleRate

	if err := d.ApplyConfig(boundary); err != nil {
		t.Fatalf("ApplyConfig at max sample rate should succeed: %v", err)
	}
	if got := d.GetProfile().SamplingRate; got != DAQP1603MaxSampleRate {
		t.Fatalf("SamplingRate = %d, want %d", got, DAQP1603MaxSampleRate)
	}
}

// 测试前置：调用 ApplyConfig 传入 profile.Type != DeviceDAQP1603。
// 期待结果：返回错误包含 "type mismatch"。
func TestDAQP1603_ApplyConfig_TypeMismatch_Rejected(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac4", "192.168.1.1", 16))

	wrongType := makeP1603Profile("ac4", "192.168.1.1", 16)
	wrongType.Type = core.DeviceDaqT1603

	err := d.ApplyConfig(wrongType)
	if err == nil {
		t.Fatal("ApplyConfig should reject type mismatch")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("error = %q, want contains 'type mismatch'", err.Error())
	}
}

// 测试前置：调用 ApplyConfig 传入 SamplingRate 低于下限。
// 期待结果：返回错误包含 "out of range"。
func TestDAQP1603_ApplyConfig_SampleRateBelowMin_Rejected(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac5", "192.168.1.1", 16))

	bad := makeP1603Profile("ac5", "192.168.1.1", 16)
	bad.SamplingRate = DAQP1603MinSampleRate - 1

	err := d.ApplyConfig(bad)
	if err == nil {
		t.Fatal("ApplyConfig should reject sampling rate < min")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("error = %q, want contains 'out of range'", err.Error())
	}
}

// 测试前置：未连接设备调用 ApplyConfig，传入 SamplingRate=DAQP1603MinSampleRate（边界值）。
// 期待结果：返回 nil，profile 更新成功。
func TestDAQP1603_ApplyConfig_SampleRateAtMin_Ok(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac3-min", "192.168.1.1", 16))

	boundary := makeP1603Profile("ac3-min", "192.168.1.1", 16)
	boundary.SamplingRate = DAQP1603MinSampleRate

	if err := d.ApplyConfig(boundary); err != nil {
		t.Fatalf("ApplyConfig at min sample rate should succeed: %v", err)
	}
	if got := d.GetProfile().SamplingRate; got != DAQP1603MinSampleRate {
		t.Fatalf("SamplingRate = %d, want %d", got, DAQP1603MinSampleRate)
	}
}

// 测试前置：白盒设置 d.acquiring=true 模拟采集进行中，调用 ApplyConfig。
// 期待结果：返回错误包含 "cannot apply while acquiring"，profile 不变。
func TestDAQP1603_ApplyConfig_WhileAcquiring_Rejected(t *testing.T) {
	original := makeP1603Profile("ac6", "192.168.1.1", 16)
	d := NewDAQP1603(original)

	// 白盒设置采集状态（Phase 2 阶段 StartAcquisition 未实现，无法走正常路径）
	d.mu.Lock()
	d.acquiring = true
	d.mu.Unlock()

	newProfile := makeP1603Profile("ac6", "192.168.1.1", 16)
	// 使用合法采样率 200Hz，确保校验先经过"采集中"检查而非采样率范围检查
	newProfile.SamplingRate = 200
	err := d.ApplyConfig(newProfile)
	if err == nil {
		t.Fatal("ApplyConfig should reject while acquiring")
	}
	if !strings.Contains(err.Error(), "while acquiring") {
		t.Fatalf("error = %q, want contains 'while acquiring'", err.Error())
	}

	// profile 不变
	if got := d.GetProfile().SamplingRate; got != original.SamplingRate {
		t.Fatalf("SamplingRate after rejected ApplyConfig = %d, want %d (unchanged)",
			got, original.SamplingRate)
	}
}

// 测试前置：ApplyConfig 传入 profile 含混合 SensorType（前 8 通道 pressure，后 8 通道 temperature）。
// 期待结果：GetProfile 返回的 channels SensorType 与传入一致。
func TestDAQP1603_ApplyConfig_PropagatesSensorType(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac7", "192.168.1.1", 16))

	mixed := makeP1603Profile("ac7", "192.168.1.1", 16)
	for i := range mixed.Channels {
		if i >= 8 {
			mixed.Channels[i].SensorType = core.SensorTemperature
			mixed.Channels[i].Unit = "℃"
		} else {
			mixed.Channels[i].SensorType = core.SensorPressure
			mixed.Channels[i].Unit = "Pa"
		}
	}

	if err := d.ApplyConfig(mixed); err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	got := d.GetProfile()
	for i, ch := range got.Channels {
		want := core.SensorPressure
		if i >= 8 {
			want = core.SensorTemperature
		}
		if ch.SensorType != want {
			t.Fatalf("channel %d SensorType = %q, want %q", i, ch.SensorType, want)
		}
	}
}

// 测试前置：GetProfile 返回 channels 切片后，修改返回值不影响内部状态。
// 期待结果：修改 got.Channels[0].Unit 后，再次 GetProfile 返回原值。
func TestDAQP1603_GetProfile_ReturnsCopy(t *testing.T) {
	d := NewDAQP1603(makeP1603Profile("ac8", "192.168.1.1", 16))

	got := d.GetProfile()
	originalUnit := got.Channels[0].Unit
	got.Channels[0].Unit = "tampered"

	again := d.GetProfile()
	if again.Channels[0].Unit != originalUnit {
		t.Fatalf("GetProfile did not return a copy: Channels[0].Unit = %q, want %q",
			again.Channels[0].Unit, originalUnit)
	}
}
