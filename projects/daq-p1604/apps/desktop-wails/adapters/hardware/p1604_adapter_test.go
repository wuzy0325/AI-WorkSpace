package hardware

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"daq-p1604/core"
	sharedproto "shared.local/device-sdk/go/protocol"
)

// TestEnableTCPKeepalive_TCPConn 验证对真实 TCP 连接启用 keepalive 成功。
// 使用本地 TCP listener 建立连接，调用后应无错误。
func TestEnableTCPKeepalive_TCPConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	// 后台 accept 一个连接（不读写，仅占位）
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			defer conn.Close()
			// 阻塞保持连接，直到客户端关闭
			buf := make([]byte, 1)
			_, _ = conn.Read(buf)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := enableTCPKeepalive(conn); err != nil {
		t.Fatalf("enableTCPKeepalive on TCP conn failed: %v", err)
	}

	// 验证 conn 仍是 *net.TCPConn 类型（未被包装）
	if _, ok := conn.(*net.TCPConn); !ok {
		t.Fatalf("conn should be *net.TCPConn, got %T", conn)
	}
}

// TestEnableTCPKeepalive_NonTCPConn 验证非 TCP 连接（mock）返回 nil 不报错。
// 模拟器或测试用的 mock conn 不实现 *net.TCPConn，应静默跳过 keepalive 设置。
func TestEnableTCPKeepalive_NonTCPConn(t *testing.T) {
	mock := &mockNonTCPConn{}
	if err := enableTCPKeepalive(mock); err != nil {
		t.Fatalf("enableTCPKeepalive on non-TCP conn should return nil, got: %v", err)
	}
}

// mockNonTCPConn 实现 net.Conn 接口但不是 *net.TCPConn，
// 用于验证 enableTCPKeepalive 对非 TCP 连接的兼容性。
type mockNonTCPConn struct{}

func (m *mockNonTCPConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockNonTCPConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockNonTCPConn) Close() error                       { return nil }
func (m *mockNonTCPConn) LocalAddr() net.Addr                { return nil }
func (m *mockNonTCPConn) RemoteAddr() net.Addr               { return nil }
func (m *mockNonTCPConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockNonTCPConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockNonTCPConn) SetWriteDeadline(t time.Time) error { return nil }

// mockConnWithCloseTracking 记录 Close 调用次数，用于验证 handleConnectionLost 是否关闭 conn。
type mockConnWithCloseTracking struct {
	mu         sync.Mutex
	closeCount int
}

func (m *mockConnWithCloseTracking) Read(b []byte) (n int, err error)  { return 0, nil }
func (m *mockConnWithCloseTracking) Write(b []byte) (n int, err error) { return len(b), nil }
func (m *mockConnWithCloseTracking) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCount++
	return nil
}
func (m *mockConnWithCloseTracking) LocalAddr() net.Addr                { return nil }
func (m *mockConnWithCloseTracking) RemoteAddr() net.Addr               { return nil }
func (m *mockConnWithCloseTracking) SetDeadline(t time.Time) error      { return nil }
func (m *mockConnWithCloseTracking) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConnWithCloseTracking) SetWriteDeadline(t time.Time) error { return nil }

func (m *mockConnWithCloseTracking) CloseCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCount
}

// setupAdapterWithDriver 构造一个带 driver 和 status 的 adapter 用于 handleConnectionLost 测试。
// 返回 adapter、driver、mockConn 和 stateCollector 用于断言。
func setupAdapterWithDriver(id string, status core.DeviceStatus) (*P1604Adapter, *p1604Driver, *mockConnWithCloseTracking, *stateCollector) {
	a := NewP1604Adapter()
	mockConn := &mockConnWithCloseTracking{}
	driver := &p1604Driver{
		profile:   core.PressureProfile{ID: id},
		conn:      mockConn,
		acquiring: status == core.StatusAcquiring,
	}
	st := &core.DeviceState{
		Profile: driver.profile,
		Status:  status,
	}
	st.StatusText = status.String()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = st
	shard.mu.Unlock()

	collector := &stateCollector{}
	a.SetStateSink(func(receivedID string, receivedState core.DeviceState) {
		collector.add(receivedID, receivedState)
	})
	return a, driver, mockConn, collector
}

type stateCollector struct {
	mu     sync.Mutex
	states []struct {
		id    string
		state core.DeviceState
	}
}

func (c *stateCollector) add(id string, state core.DeviceState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states = append(c.states, struct {
		id    string
		state core.DeviceState
	}{id, state})
}

func (c *stateCollector) lastState() (string, core.DeviceState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.states) == 0 {
		return "", core.DeviceState{}, false
	}
	last := c.states[len(c.states)-1]
	return last.id, last.state, true
}

// TestHandleConnectionLost_CleansDriverAndConn 验证 driver 被删除 + conn 被关闭。
func TestHandleConnectionLost_CleansDriverAndConn(t *testing.T) {
	id := "test-device-clean"
	a, driver, mockConn, _ := setupAdapterWithDriver(id, core.StatusAcquiring)

	a.handleConnectionLost(id, driver, errors.New("simulated disconnect"))

	shard := a.shard(id)
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()

	if driverExists {
		t.Error("driver should be deleted from shard.drivers after handleConnectionLost")
	}
	if !statusExists {
		t.Fatal("status should still exist (set to Error) for frontend visibility")
	}
	if st.Status != core.StatusError {
		t.Errorf("status should be Error, got %v", st.Status)
	}
	if st.Error == "" {
		t.Error("status.Error should be populated with disconnect cause")
	}
	if mockConn.CloseCount() != 1 {
		t.Errorf("conn.Close should be called exactly once, got %d", mockConn.CloseCount())
	}
}

// TestHandleConnectionLost_EmitsErrorState 验证 emitState 推送 StatusError 状态到前端。
func TestHandleConnectionLost_EmitsErrorState(t *testing.T) {
	id := "test-device-emit"
	a, driver, _, collector := setupAdapterWithDriver(id, core.StatusAcquiring)

	a.handleConnectionLost(id, driver, errors.New("connection reset"))

	receivedID, receivedState, ok := collector.lastState()
	if !ok {
		t.Fatal("emitState should be called at least once")
	}
	if receivedID != id {
		t.Errorf("emitState should pass device id %s, got %s", id, receivedID)
	}
	if receivedState.Status != core.StatusError {
		t.Errorf("emitted state should be Error, got %v", receivedState.Status)
	}
	if receivedState.Error == "" {
		t.Error("emitted state should contain error message")
	}
}

// TestHandleConnectionLost_GuardsAgainstNewDriver 验证 driver 一致性校验：
// 若 shard.drivers[id] 已被替换为新 driver，旧 driver 触发的 handleConnectionLost 应放弃清理。
func TestHandleConnectionLost_GuardsAgainstNewDriver(t *testing.T) {
	id := "test-device-guard"
	a, oldDriver, oldMockConn, collector := setupAdapterWithDriver(id, core.StatusAcquiring)

	// 模拟 Disconnect 已删除旧 driver 并启动新 driver
	newMockConn := &mockConnWithCloseTracking{}
	newDriver := &p1604Driver{
		profile:   core.PressureProfile{ID: id},
		conn:      newMockConn,
		acquiring: false,
	}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = newDriver
	shard.mu.Unlock()

	// 旧 driver 的 readLoop 触发 handleConnectionLost
	a.handleConnectionLost(id, oldDriver, errors.New("old driver disconnect"))

	// 旧 driver 的 conn 不应被关闭（清理被放弃）
	if oldMockConn.CloseCount() != 0 {
		t.Errorf("old driver conn should NOT be closed, got %d calls", oldMockConn.CloseCount())
	}
	// 新 driver 应仍存在
	shard.mu.RLock()
	cur, exists := shard.drivers[id]
	shard.mu.RUnlock()
	if !exists || cur != newDriver {
		t.Error("new driver should remain untouched")
	}
	// 不应 emit 状态（清理被放弃）
	if _, _, ok := collector.lastState(); ok {
		t.Error("emitState should NOT be called when guard triggers")
	}
}

// TestHandleConnectionLost_SkipsAlreadyDisconnected 验证幂等性：
// 设备已处于 Disconnected 状态时，handleConnectionLost 应直接返回不重复清理。
func TestHandleConnectionLost_SkipsAlreadyDisconnected(t *testing.T) {
	id := "test-device-disconnected"
	a, driver, mockConn, collector := setupAdapterWithDriver(id, core.StatusDisconnected)

	a.handleConnectionLost(id, driver, errors.New("late disconnect event"))

	// 不应关闭 conn（已 Disconnected，无需清理）
	if mockConn.CloseCount() != 0 {
		t.Errorf("conn should NOT be closed when already disconnected, got %d", mockConn.CloseCount())
	}
	// 不应 emit 状态
	if _, _, ok := collector.lastState(); ok {
		t.Error("emitState should NOT be called when already disconnected")
	}
}

// TestSyncUnitFromHardware_EOFReturnsError 验证：u01101 读到 io.EOF 时
// syncUnitFromHardware 必须返回 error（连接已死），不能当作软错误吞掉。
//
// 场景：设备拔网线后重连，TCP 拨号成功、w1601 发送成功，但 u01101 交换时
// 设备主动 FIN 关闭连接（对端 EOF）。若吞掉此错误，后续 StartAcquisition
// 的 c 00 命令会爆 WSAECONNABORTED。
//
// 模拟方式：net.Pipe 服务端读掉 u01101 命令后 sleep 让客户端进入 Read 阻塞，
// 再 Close 触发客户端 Read 返回 io.EOF。
//
// 时序要求：必须让客户端先进入 Read 阻塞再 Close。若 Close 先于 Read 调用，
// 客户端会返回 "use of closed network connection"——该错误由 IsClosedConnError
// 匹配，但 IsConnResetByPeer 并不匹配（IsConnResetByPeer 只覆盖对端 FIN/RST
// 硬证据，不覆盖本地主动 Close）。sleep 50ms 确保走 io.EOF 路径，断言更精确。
func TestSyncUnitFromHardware_EOFReturnsError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 服务端：读掉 u01101 命令 → 让客户端进入 Read 阻塞 → Close 触发 EOF
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)           // 读掉 u01101
		time.Sleep(50 * time.Millisecond) // 等客户端进入 Read 阻塞
		server.Close()                    // 触发客户端 Read 返回 io.EOF
	}()

	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-eof"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}

	_, _, err := a.syncUnitFromHardware(driver, core.PressureProfile{
		ID:       "test-eof",
		P1604Cfg: core.P1604Config{Unit: "Pa"},
	})
	if err == nil {
		t.Fatal("syncUnitFromHardware should return error on EOF, got nil")
	}
	if !sharedproto.IsConnResetByPeer(err) {
		t.Errorf("returned error should be detected as conn reset, got: %v", err)
	}
}

// TestSyncUnitFromHardware_TimeoutKeepsProfileUnit 验证：u01101 超时（软错误）
// 不返回 error，保留 profile 单位继续连接流程——兼容旧固件/模拟器。
func TestSyncUnitFromHardware_TimeoutKeepsProfileUnit(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 服务端：读掉 u01101 但永远不回响应，让客户端 SetReadDeadline 超时
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 不写任何响应，阻塞到测试结束
		time.Sleep(5 * time.Second)
	}()

	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-timeout"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}

	// 缩短超时避免测试卡太久：直接调 P1604ReadUnitCoefficient 模拟超时路径
	// syncUnitFromHardware 内部用 p1604UnitSyncTimeout（2s），测试容忍 2s 等待
	unit, note, err := a.syncUnitFromHardware(driver, core.PressureProfile{
		ID:       "test-timeout",
		P1604Cfg: core.P1604Config{Unit: "Pa"},
	})
	if err != nil {
		t.Fatalf("syncUnitFromHardware should NOT return error on timeout, got: %v", err)
	}
	if unit != "" {
		t.Errorf("unit should be empty on timeout (keep profile), got %q", unit)
	}
	if note == "" {
		t.Error("note should describe the failure")
	}
}

// TestApplyConfig_V01101EOFTriggersConnectionLost 验证：ApplyConfig 写 v01101 时
// 设备 FIN 关闭连接，应触发 handleConnectionLost 清理 driver，避免后续
// StartAcquisition 爆 WSAECONNABORTED。
//
// 模拟方式：net.Pipe 服务端读掉 v01101 命令后 sleep 让客户端进入 Read 阻塞，
// 再 Close 触发客户端 Read 返回 io.EOF（与 syncUnitFromHardware EOF 测试同构）。
//
// 预期：
//   - ApplyConfig 返回 error
//   - shard.drivers[id] 被删除（handleConnectionLost 清理）
//   - shard.status[id] 保留且 Status=Error
//   - conn 被 Close
func TestApplyConfig_V01101EOFTriggersConnectionLost(t *testing.T) {
	server, client := net.Pipe()

	a := NewP1604Adapter()
	id := "test-apply-eof"
	profile := core.PressureProfile{
		ID:       id,
		P1604Cfg: core.P1604Config{Unit: "Pa"},
	}
	driver := &p1604Driver{
		profile:     profile,
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	// 服务端：读掉 v01101 → sleep → Close 触发客户端 Read 返回 io.EOF
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()

	// 单位从 Pa → psi，触发 v01101 写入路径
	err := a.ApplyConfig(id, core.P1604Config{Unit: "psi"})

	// 1. ApplyConfig 必须返回 error
	if err == nil {
		t.Fatal("ApplyConfig should return error on v01101 EOF, got nil")
	}
	// 2. driver 应被 handleConnectionLost 清理
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()
	if driverExists {
		t.Error("driver should be deleted by handleConnectionLost after v01101 EOF")
	}
	// 3. status 应保留且为 Error
	if !statusExists {
		t.Fatal("status should still exist (set to Error) for frontend visibility")
	}
	if st.Status != core.StatusError {
		t.Errorf("status should be Error, got %v", st.Status)
	}
	if st.Error == "" {
		t.Error("status.Error should be populated with disconnect cause")
	}

	// 关闭 client（server 已在 goroutine 内 Close），避免 fd 泄漏告警
	_ = client.Close()
}

// TestApplyConfig_V01101SoftErrorKeepsDriver 验证：v01101 软错误（如设备返回 N05）
// 不触发 handleConnectionLost，driver 保留在 shard，前端可继续 Disconnect/重试。
//
// 模拟方式：服务端回复一个非 A 的 ASCII 帧（模拟设备拒绝）。
// 注意：P1604WriteUnitCoefficient 对非 A 非 N 的响应会返回 "unexpected v01101 response"，
// 对 N 开头返回 "device rejected unit change"。这里用 N01 触发后者（软错误）。
func TestApplyConfig_V01101SoftErrorKeepsDriver(t *testing.T) {
	server, client := net.Pipe()

	a := NewP1604Adapter()
	id := "test-apply-soft"
	profile := core.PressureProfile{
		ID:       id,
		P1604Cfg: core.P1604Config{Unit: "Pa"},
	}
	driver := &p1604Driver{
		profile:     profile,
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	// 服务端：读掉 v01101 → 回复 N01 帧（设备拒绝，软错误）
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 写一个长度前缀帧：2 字节大端长度 + payload "N01"
		_, _ = server.Write([]byte{0x00, 0x03, 'N', '0', '1'})
	}()

	err := a.ApplyConfig(id, core.P1604Config{Unit: "psi"})

	// 1. ApplyConfig 必须返回 error（设备拒绝）
	if err == nil {
		t.Fatal("ApplyConfig should return error on v01101 N01, got nil")
	}
	// 2. driver 必须保留（软错误不触发 handleConnectionLost）
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	shard.mu.RUnlock()
	if !driverExists {
		t.Error("driver should REMAIN after soft error (N01), handleConnectionLost must NOT be triggered")
	}
	// 3. status 应仍为 Connected（不是 Error）
	shard.mu.RLock()
	st, _ := shard.status[id]
	shard.mu.RUnlock()
	if st.Status != core.StatusConnected {
		t.Errorf("status should remain Connected after soft error, got %v", st.Status)
	}

	_ = server.Close()
	_ = client.Close()
}

func TestApplyConfig_StopsIdleLoopBeforeUnitWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	a := NewP1604Adapter()
	id := "test-apply-idle"
	profile := core.PressureProfile{
		ID:       id,
		P1604Cfg: core.P1604Config{Unit: "Pa"},
	}
	idleStop := make(chan struct{})
	idleDone := make(chan struct{})
	driver := &p1604Driver{
		profile:      profile,
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		idleStopCh:   idleStop,
		idleLoopDone: idleDone,
	}
	go func() {
		<-idleStop
		close(idleDone)
	}()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	idleStoppedBeforeWrite := make(chan bool, 1)
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		select {
		case <-idleStop:
			idleStoppedBeforeWrite <- true
		default:
			idleStoppedBeforeWrite <- false
		}
		_, _ = server.Write([]byte{0x00, 0x03, 'A'})
	}()

	if err := a.ApplyConfig(id, core.P1604Config{Unit: "psi"}); err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}
	if !<-idleStoppedBeforeWrite {
		t.Fatal("ApplyConfig must stop idleReadLoop before writing the unit coefficient")
	}

	shard.mu.RLock()
	restartedStop := driver.idleStopCh
	shard.mu.RUnlock()
	if restartedStop == nil || restartedStop == idleStop {
		t.Fatal("ApplyConfig must restart idleReadLoop with a new stop channel")
	}
	close(restartedStop)
}

func TestStopAcquisition_WhileIdleDoesNotStartSecondIdleLoop(t *testing.T) {
	a := NewP1604Adapter()
	id := "test-stop-idle"
	idleStop := make(chan struct{})
	idleDone := make(chan struct{})
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         &mockNonTCPConn{},
		idleStopCh:   idleStop,
		idleLoopDone: idleDone,
	}
	go func() {
		<-idleStop
		close(idleDone)
	}()
	defer func() {
		select {
		case <-idleStop:
		default:
			close(idleStop)
		}
	}()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{Status: core.StatusConnected}
	shard.mu.Unlock()

	if err := a.StopAcquisition(id); err != nil {
		t.Fatalf("StopAcquisition failed: %v", err)
	}

	shard.mu.RLock()
	currentStop := driver.idleStopCh
	shard.mu.RUnlock()
	if currentStop != idleStop {
		if currentStop != nil {
			close(currentStop)
		}
		t.Fatal("StopAcquisition replaced an already running idleReadLoop")
	}
}

// TestSyncChannelsUnit_PressureChannelsFollowGlobalUnit
//
// 测试前置：构造 18 通道 profile，CH1-CH16 初始单位为 psi（profile 默认值）
// 测试步骤：调用 syncChannelsUnit(channels, "Pa")
// 期待结果：CH1-CH16 全部变为 Pa，CH17/CH18 不受影响（由下一个用例覆盖）
//
// 优先级 P0：硬件单位同步后通道卡片必须跟随全局单位，否则用户看到陈旧 psi
func TestSyncChannelsUnit_PressureChannelsFollowGlobalUnit(t *testing.T) {
	channels := make([]core.ChannelConfig, 18)
	for i := range channels {
		channels[i] = core.ChannelConfig{Index: i, Unit: "psi"}
	}

	syncChannelsUnit(channels, "Pa")

	for i := 0; i < 16; i++ {
		if channels[i].Unit != "Pa" {
			t.Errorf("CH%d (index %d) unit should be Pa, got %s", i+1, i, channels[i].Unit)
		}
	}
}

// TestSyncChannelsUnit_SpecialChannelsLocked
//
// 测试前置：构造 18 通道 profile，CH17/CH18 初始单位被污染为 psi
// 测试步骤：调用 syncChannelsUnit(channels, "kPa")
// 期待结果：CH17 锁 Pa，CH18 锁 °C，不受全局 kPa 影响
//
// 优先级 P0：大气压力/温度是独立物理量，绝不能跟随压力单位
func TestSyncChannelsUnit_SpecialChannelsLocked(t *testing.T) {
	channels := make([]core.ChannelConfig, 18)
	for i := range channels {
		channels[i] = core.ChannelConfig{Index: i, Unit: "psi"}
	}

	syncChannelsUnit(channels, "kPa")

	if channels[16].Unit != "Pa" {
		t.Errorf("CH17 (大气压力) unit should be locked to Pa, got %s", channels[16].Unit)
	}
	if channels[17].Unit != "°C" {
		t.Errorf("CH18 (大气温度) unit should be locked to °C, got %s", channels[17].Unit)
	}
}

// TestSyncChannelsUnit_EmptyGlobalUnitKeepsPressureUnits
//
// 测试前置：构造 18 通道 profile，CH1-CH16 单位为 psi
// 测试步骤：调用 syncChannelsUnit(channels, "")（模拟硬件单位读取失败的兜底）
// 期待结果：CH1-CH16 保持原值 psi（不覆盖），CH17/CH18 仍锁定 Pa/°C
//
// 优先级 P1：hwUnit 为空时 adapter 不会进入同步分支，但函数本身应防御性保留原值
func TestSyncChannelsUnit_EmptyGlobalUnitKeepsPressureUnits(t *testing.T) {
	channels := make([]core.ChannelConfig, 18)
	for i := range channels {
		channels[i] = core.ChannelConfig{Index: i, Unit: "psi"}
	}

	syncChannelsUnit(channels, "")

	if channels[0].Unit != "psi" {
		t.Errorf("CH1 unit should remain psi when globalUnit empty, got %s", channels[0].Unit)
	}
	if channels[16].Unit != "Pa" {
		t.Errorf("CH17 unit should still be locked to Pa even with empty globalUnit, got %s", channels[16].Unit)
	}
	if channels[17].Unit != "°C" {
		t.Errorf("CH18 unit should still be locked to °C even with empty globalUnit, got %s", channels[17].Unit)
	}
}

// TestSyncChannelsUnit_EmptySliceNoPanic
//
// 测试前置：channels 为空切片
// 测试步骤：调用 syncChannelsUnit(nil, "Pa")
// 期待结果：不 panic
//
// 优先级 P2：防御性测试，避免边界 panic 阻塞连接流程
func TestSyncChannelsUnit_EmptySliceNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("syncChannelsUnit panicked on empty slice: %v", r)
		}
	}()
	syncChannelsUnit(nil, "Pa")
	syncChannelsUnit([]core.ChannelConfig{}, "Pa")
}
