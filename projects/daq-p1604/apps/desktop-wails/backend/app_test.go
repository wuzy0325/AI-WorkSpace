package backend

import (
	"strings"
	"testing"

	"daq-p1604/adapters/recording"
	"daq-p1604/core"
	"daq-p1604/ports"
	"daq-p1604/usecase"
)

// mockDevicePort 测试用的设备端口 mock，仅实现 Status 方法返回预设状态。
type mockDevicePort struct {
	states map[string]core.DeviceState
}

func (m *mockDevicePort) Connect(profile core.PressureProfile) error                  { return nil }
func (m *mockDevicePort) Disconnect(id string) error                                   { return nil }
func (m *mockDevicePort) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	return nil, nil
}
func (m *mockDevicePort) StopAcquisition(id string) error             { return nil }
func (m *mockDevicePort) ApplyConfig(id string, cfg core.P1604Config) error { return nil }
func (m *mockDevicePort) SetDataSink(id string, sink func(core.PressureSnapshot)) {}

func (m *mockDevicePort) Status(id string) (core.DeviceState, bool) {
	st, ok := m.states[id]
	return st, ok
}

// newTestApp 构造用于 handleRelayExit 测试的 App 实例。
// logUC 传 nil（EmitLog 内部 nil 检查保护），app 字段保持 nil（emit 方法跳过 Event.Emit）。
func newTestApp(mockDev ports.DevicePort) *App {
	recorder := recording.NewCSVRecorder()
	recordUC := usecase.NewRecordingUsecase(recorder)
	deviceUC := usecase.NewDeviceUsecase(mockDev, nil)
	return NewApp(deviceUC, recordUC, nil, "")
}

// startTestRecording 启动录制到临时目录，辅助测试。
func startTestRecording(t *testing.T, app *App) {
	t.Helper()
	tmpDir := t.TempDir()
	if err := app.recordUC.Start(core.RecordingConfig{
		OutputDir:  tmpDir,
		FilePrefix: "test",
	}); err != nil {
		t.Fatalf("start recording failed: %v", err)
	}
}

// addFakeRelay 向 app.relays 注入一个假的 relay control，模拟其他设备仍在录制。
func addFakeRelay(app *App, deviceID string) {
	app.mu.Lock()
	app.relays[deviceID] = &relayControl{
		cancel: func() {},
		done:   make(chan struct{}),
	}
	app.mu.Unlock()
}

// TestHandleRelayExit_SingleDeviceAutoStopsRecording 验证单设备断连自动停止录制。
// 场景：仅一台设备录制，断连后 handleRelayExit 应自动停止录制并填充 LastError。
func TestHandleRelayExit_SingleDeviceAutoStopsRecording(t *testing.T) {
	mockDev := &mockDevicePort{
		states: map[string]core.DeviceState{
			"dev1": {Status: core.StatusError, Error: "连接断开: connection reset"},
		},
	}
	app := newTestApp(mockDev)
	startTestRecording(t, app)

	// relays 为空（无其他设备），模拟单设备断连场景
	app.handleRelayExit("dev1")

	if app.recordUC.IsActive() {
		t.Error("recording should be auto-stopped after single device disconnect")
	}
	session := app.recordUC.Status()
	if session.LastError == "" {
		t.Error("LastError should be populated with disconnect reason")
	}
	if !strings.Contains(session.LastError, "dev1") {
		t.Errorf("LastError should contain device id 'dev1', got: %s", session.LastError)
	}
}

// TestHandleRelayExit_MultiDeviceEmitsWarning 验证多设备场景断连 emit 警告但不停止录制。
// 场景：两台设备录制，dev1 断连，dev2 仍在录制。handleRelayExit 应 emit 警告，不停止录制。
func TestHandleRelayExit_MultiDeviceEmitsWarning(t *testing.T) {
	mockDev := &mockDevicePort{
		states: map[string]core.DeviceState{
			"dev1": {Status: core.StatusError, Error: "连接断开"},
			"dev2": {Status: core.StatusAcquiring},
		},
	}
	app := newTestApp(mockDev)
	startTestRecording(t, app)
	// 测试结束前必须 Stop 释放 CSV 文件句柄，否则 tmpDir 清理失败（Windows 文件占用）
	defer app.recordUC.Stop()

	// 模拟 dev2 仍在录制
	addFakeRelay(app, "dev2")

	app.handleRelayExit("dev1")

	if !app.recordUC.IsActive() {
		t.Error("recording should continue when other devices still active")
	}
	session := app.recordUC.Status()
	if session.LastError != "" {
		t.Errorf("LastError should NOT be set in multi-device scenario, got: %s", session.LastError)
	}
}

// TestHandleRelayExit_NoActionWhenDeviceNotError 验证设备非 Error 状态时不触发自动停止。
// 场景：用户主动 StopAcquisition 后设备状态为 Connected，handleRelayExit 应直接返回。
func TestHandleRelayExit_NoActionWhenDeviceNotError(t *testing.T) {
	mockDev := &mockDevicePort{
		states: map[string]core.DeviceState{
			"dev1": {Status: core.StatusConnected}, // 主动停止后状态为 Connected
		},
	}
	app := newTestApp(mockDev)
	startTestRecording(t, app)
	// 测试结束前必须 Stop 释放 CSV 文件句柄，否则 tmpDir 清理失败（Windows 文件占用）
	defer app.recordUC.Stop()

	app.handleRelayExit("dev1")

	// 录制应仍活跃（未触发自动停止）
	if !app.recordUC.IsActive() {
		t.Error("recording should NOT be stopped when device status is not Error")
	}
	session := app.recordUC.Status()
	if session.LastError != "" {
		t.Errorf("LastError should NOT be set when device not in Error state, got: %s", session.LastError)
	}
}

// TestHandleRelayExit_NoActionWhenRecordingInactive 验证录制未启动时不触发。
// 场景：设备断连但未在录制，handleRelayExit 应直接返回。
func TestHandleRelayExit_NoActionWhenRecordingInactive(t *testing.T) {
	mockDev := &mockDevicePort{
		states: map[string]core.DeviceState{
			"dev1": {Status: core.StatusError, Error: "连接断开"},
		},
	}
	app := newTestApp(mockDev)
	// 不启动录制

	app.handleRelayExit("dev1")

	// 不应 panic，不应设置 LastError
	session := app.recordUC.Status()
	if session.LastError != "" {
		t.Errorf("LastError should NOT be set when recording inactive, got: %s", session.LastError)
	}
}

// TestHandleRelayExit_AllDevicesDisconnectAutoStopsRecording 验证多设备全部断连
// 场景：先断连的设备 emit 警告（其他设备仍在），最后断连的设备触发自动停止录制。
// 场景：dev1、dev2 同时录制，dev1 先断连（dev2 仍在 → emit 警告），
// 随后 dev2 也断连（len==0 → 自动停止录制并填充 LastError）。
func TestHandleRelayExit_AllDevicesDisconnectAutoStopsRecording(t *testing.T) {
	mockDev := &mockDevicePort{
		states: map[string]core.DeviceState{
			"dev1": {Status: core.StatusError, Error: "连接断开"},
			"dev2": {Status: core.StatusError, Error: "连接断开"},
		},
	}
	app := newTestApp(mockDev)
	startTestRecording(t, app)

	// dev2 仍在录制时，dev1 断连 → emit 警告，录制继续
	addFakeRelay(app, "dev2")
	app.handleRelayExit("dev1")
	if !app.recordUC.IsActive() {
		t.Error("recording should continue when dev2 still active after dev1 disconnect")
	}
	session := app.recordUC.Status()
	if session.LastError != "" {
		t.Errorf("LastError should NOT be set after first device disconnect, got: %s", session.LastError)
	}

	// 模拟 dev2 relay 也退出（clearRelay 已删除 dev2 的 relay）
	app.mu.Lock()
	delete(app.relays, "dev2")
	app.mu.Unlock()

	// dev2 断连，此时 len(relays)==0 → 自动停止录制
	app.handleRelayExit("dev2")
	if app.recordUC.IsActive() {
		t.Error("recording should be auto-stopped after all devices disconnected")
	}
	session = app.recordUC.Status()
	if session.LastError == "" {
		t.Error("LastError should be populated with disconnect reason")
	}
	if !strings.Contains(session.LastError, "dev2") {
		t.Errorf("LastError should contain last disconnected device id 'dev2', got: %s", session.LastError)
	}
}

// TestHandleRelayExit_DoesNotOverrideUserStop 验证竞态保护：
// 用户主动 StopRecording 后 handleRelayExit 触发时，不应把 LastError
// 覆盖为"设备断连自动停止"。
// 场景：录制中，用户主动 Stop，随后 relay goroutine 退出并触发 handleRelayExit。
// 期望：LastError 保持用户主动停止路径留下的空值（或用户意图），
// 不被 handleRelayExit 写入"因设备 dev1 断连自动停止录制"。
func TestHandleRelayExit_DoesNotOverrideUserStop(t *testing.T) {
	mockDev := &mockDevicePort{
		states: map[string]core.DeviceState{
			"dev1": {Status: core.StatusError, Error: "连接断开"},
		},
	}
	app := newTestApp(mockDev)
	startTestRecording(t, app)

	// 用户主动停止录制：recorder.Start 的 CAS 由 true→false
	if err := app.recordUC.Stop(); err != nil {
		t.Fatalf("user Stop failed: %v", err)
	}

	// 此时 handleRelayExit 因 relay 退出而被触发
	app.handleRelayExit("dev1")

	// 录制应仍处于 Idle（未被重新激活）
	if app.recordUC.IsActive() {
		t.Error("recording should remain idle after user Stop")
	}
	// 关键断言：LastError 不应被 handleRelayExit 覆盖为"断连自动停止"
	session := app.recordUC.Status()
	if strings.Contains(session.LastError, "断连自动停止") {
		t.Errorf("LastError should NOT be overridden by auto-stop msg when user already stopped, got: %s", session.LastError)
	}
}
