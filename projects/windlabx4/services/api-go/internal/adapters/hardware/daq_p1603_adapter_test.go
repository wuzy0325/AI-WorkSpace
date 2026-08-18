package hardware

import (
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

// ============================================================
// DAQ-P-1603 thin wrapper 单元测试
// ------------------------------------------------------------
// 测试范围：Connect/Disconnect/Status/SetDataSink 状态机与幂等性。
// 不依赖真实硬件：shared SDK 的 DAQP1603.Connect 在 DLL 未初始化时
// 会返回明确错误，覆盖适配器的错误传播路径。
// 真机端到端验证由 Phase 7 HIL（Task 16）完成。
// ============================================================

func makeAdapterProfile(id, ip string) device.Profile {
	return device.Profile{
		ID:           id,
		Name:         "DAQ-P-1603-" + id,
		Type:         device.DeviceDAQP1603,
		Address:      ip,
		SamplingRate: 500,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "CH1", Enabled: true, Unit: "Pa"},
			{Index: 1, Name: "CH2", Enabled: true, Unit: "Pa"},
		},
	}
}

// 测试前置：NewDAQP1603Adapter 后初始状态为 Disconnected，driver 为 nil。
// 期待结果：Status().Connection == Disconnected，ID 与 profile 一致。
func TestDAQP1603Adapter_InitialState(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a1", "192.168.1.1"))

	if got := a.Status().Connection; got != device.ConnectionDisconnected {
		t.Fatalf("initial connection = %q, want Disconnected", got)
	}
	if a.ID() != "a1" {
		t.Fatalf("ID = %q, want a1", a.ID())
	}
}

// 测试前置：DLL 未初始化时调用 Connect。
// 期待结果：返回错误包含 "DAQ-P-1603 connect"，driver 保持 nil。
func TestDAQP1603Adapter_Connect_DLLNotInitialized(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a2", "192.168.1.1"))

	err := a.Connect()
	if err == nil {
		t.Fatal("Connect should fail when DLL not initialized")
	}
	if !strings.Contains(err.Error(), "DAQ-P-1603 connect") {
		t.Fatalf("error = %q, want contains 'DAQ-P-1603 connect'", err.Error())
	}
	// 失败后 driver 应保持 nil，避免 Disconnect 误调用 driver
	a.mu.RLock()
	driverNil := a.driver == nil
	a.mu.RUnlock()
	if !driverNil {
		t.Fatal("driver should remain nil after failed Connect")
	}
}

// 测试前置：未连接时连续调用 Disconnect。
// 期待结果：无 panic，无错误，状态保持 Disconnected。
func TestDAQP1603Adapter_Disconnect_Idempotent_NotConnected(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a3", "192.168.1.1"))

	for i := 0; i < 3; i++ {
		if err := a.Disconnect(); err != nil {
			t.Fatalf("Disconnect call %d failed: %v", i, err)
		}
	}
}

// 测试前置：未连接时调用 SetDataSink。
// 期待结果：无 panic，sink 缓存在 adapter 内（Connect 时再转发到 driver）。
func TestDAQP1603Adapter_SetDataSink_BeforeConnect(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a4", "192.168.1.1"))

	called := false
	a.SetDataSink(func(p device.DataPayload) {
		called = true
	})

	a.mu.RLock()
	sinkNil := a.sink == nil
	a.mu.RUnlock()
	if sinkNil {
		t.Fatal("sink should be cached before Connect")
	}
	_ = called // 仅验证 SetDataSink 不 panic
}

// 测试前置：StartAcquisition 在未连接时返回 "device not connected"。
// 期待结果：错误信息包含 "not connected"，不触发 driver 调用。
func TestDAQP1603Adapter_StartAcquisition_NotConnected(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a5", "192.168.1.1"))

	err := a.StartAcquisition()
	if err == nil {
		t.Fatal("StartAcquisition should fail when not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("error = %q, want contains 'not connected'", err.Error())
	}
}

// 测试前置：StopAcquisition 在未连接时返回 nil（幂等，不视为错误）。
// 期待结果：无错误。
func TestDAQP1603Adapter_StopAcquisition_NotConnected(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a6", "192.168.1.1"))

	if err := a.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition on disconnected device failed: %v", err)
	}
}

// 测试前置：SetOnError 在 Phase 2 阶段仅存储回调，不触发任何 driver 行为。
// 期待结果：无 panic，onError 字段被写入。
func TestDAQP1603Adapter_SetOnError(t *testing.T) {
	a := NewDAQP1603Adapter(makeAdapterProfile("a7", "192.168.1.1"))

	called := false
	a.SetOnError(func(err error) {
		called = true
	})

	a.mu.RLock()
	fn := a.onError
	a.mu.RUnlock()
	if fn == nil {
		t.Fatal("onError not registered")
	}
	fn(nil)
	if !called {
		t.Fatal("onError callback not invoked")
	}
}

// 测试前置：mapToSharedProfileP1603 在 profile.Channels 为空时补齐 16 个默认通道。
// 期待结果：返回 sharedcore.Profile 的 Channels 长度为 16，且每个通道 Enabled=true。
func TestDAQP1603Adapter_MapToSharedProfile_EmptyChannels(t *testing.T) {
	p := device.Profile{
		ID:           "a8",
		Type:         device.DeviceDAQP1603,
		Address:      "192.168.1.1",
		SamplingRate: 500,
		// Channels 故意留空
	}
	sp := mapToSharedProfileP1603(p)

	if len(sp.Channels) != 16 {
		t.Fatalf("channels length = %d, want 16", len(sp.Channels))
	}
	for i, ch := range sp.Channels {
		if !ch.Enabled {
			t.Fatalf("channel %d not enabled", i)
		}
	}
	if sp.Type != "DAQ-P-1603" {
		t.Fatalf("type = %q, want DAQ-P-1603", sp.Type)
	}
}

// 测试前置：mapToSharedProfileP1603 在 profile.Channels 已有时按实际内容映射。
// 期待结果：通道数与输入一致（最多 16），字段逐项对应。
func TestDAQP1603Adapter_MapToSharedProfile_WithChannels(t *testing.T) {
	p := device.Profile{
		ID:           "a9",
		Type:         device.DeviceDAQP1603,
		Address:      "192.168.1.1",
		SamplingRate: 200,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "P1", Enabled: true, Unit: "Pa", Precision: 3},
			{Index: 1, Name: "T1", Enabled: false, Unit: "degC", Precision: 2},
		},
	}
	sp := mapToSharedProfileP1603(p)

	if len(sp.Channels) != 2 {
		t.Fatalf("channels length = %d, want 2", len(sp.Channels))
	}
	if sp.Channels[0].Name != "P1" || !sp.Channels[0].Enabled {
		t.Fatalf("channel 0 mapping wrong: %+v", sp.Channels[0])
	}
	if sp.Channels[1].Name != "T1" || sp.Channels[1].Enabled {
		t.Fatalf("channel 1 mapping wrong: %+v", sp.Channels[1])
	}
	if sp.SamplingRate != 200 {
		t.Fatalf("SamplingRate = %d, want 200", sp.SamplingRate)
	}
}
