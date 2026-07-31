package hardware

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"daq-p1604/core"
	sharedproto "shared.local/device-sdk/go/protocol"
)

const zeroCalibrationCoefficients = "-0.008106 -0.012445 -0.020647 -0.000668 -0.015450 -0.030518 -0.015354 -0.005031 -0.021625 -0.006795 -0.019813 0.013423 -0.014782 -0.002360 -0.013113 0.002527"

func framedASCII(payload string) []byte {
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame, uint16(len(frame)))
	copy(frame[2:], payload)
	return frame
}

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

func TestRunConnectionHandshakeTimesOutAndClosesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	started := time.Now()
	err := runConnectionHandshake(client, 20*time.Millisecond, func() error {
		buf := make([]byte, 1)
		_, readErr := client.Read(buf)
		return readErr
	})

	if err == nil || !strings.Contains(err.Error(), "handshake timed out") {
		t.Fatalf("expected handshake timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("handshake timeout took too long: %v", elapsed)
	}
}

func TestConnectRejectsSecondAttemptWhileHandshakeIsRunning(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 2)
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			accepted <- conn
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	profile := core.PressureProfile{
		ID:      "test-connect-in-progress",
		Address: addr.IP.String(),
		Port:    addr.Port,
	}
	adapter := NewP1604Adapter()
	firstDone := make(chan error, 1)
	go func() { firstDone <- adapter.Connect(profile) }()

	var firstConn net.Conn
	select {
	case firstConn = <-accepted:
		defer firstConn.Close()
	case <-time.After(time.Second):
		t.Fatal("first connection was not accepted")
	}

	buf := make([]byte, len("w1601"))
	if _, err := firstConn.Read(buf); err != nil {
		t.Fatalf("read first handshake command: %v", err)
	}
	if got := string(buf); got != "w1601" {
		t.Fatalf("first handshake command = %q, want w1601", got)
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- adapter.Connect(profile) }()
	select {
	case err := <-secondDone:
		if err == nil || !strings.Contains(err.Error(), "connection already in progress") {
			t.Fatalf("second Connect error = %v, want connection already in progress", err)
		}
	case secondConn := <-accepted:
		secondConn.Close()
		t.Fatal("second Connect opened another TCP connection")
	case <-time.After(300 * time.Millisecond):
		t.Fatal("second Connect did not reject the concurrent attempt promptly")
	}

	firstConn.Close()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first Connect did not return after its socket was closed")
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

func TestStopAcquisitionClosesConnWhenReadLoopDoesNotExit(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	const id = "test-stop-stuck-reader"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{Profile: driver.profile, Status: core.StatusAcquiring}
	shard.stopChs[id] = make(chan struct{})
	shard.mu.Unlock()

	err := a.StopAcquisition(id)
	if err == nil || !strings.Contains(err.Error(), "reconnect required") {
		t.Fatalf("StopAcquisition error = %v, want reconnect required", err)
	}

	_ = server.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 16)
	n, readErr := server.Read(buf)
	if n != 0 {
		t.Fatalf("received command after read-loop timeout: %q", string(buf[:n]))
	}
	if readErr == nil {
		t.Fatal("server read should fail after client connection is closed")
	}

	shard.mu.RLock()
	_, stillConnected := shard.drivers[id]
	shard.mu.RUnlock()
	if stillConnected {
		t.Fatal("driver should be removed after read-loop timeout")
	}
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

// TestRollbackAcquisition_DoesNotPanicWhenChannelsAlreadyClosedByHandleConnectionLost
// 验证 ADR-009 复核修订 finding 1 修复：StartAcquisition 失败路径下，
// handleConnectionLost 已关闭并删除 done/ch 后，rollbackAcquisition 不能再次 close，
// 否则触发 close of closed channel panic。
//
// 测试前置：
//   - setupAdapterWithDriver 创建 driver + 预占采集槽位（ch/done 写入 map）
//   - handleConnectionLost 模拟连接故障先行关闭并删除 ch/done
//
// 测试步骤：
//   - 调用 rollbackAcquisition(id, ch, done)
//
// 期待结果：
//   - 不 panic（owner 检查跳过已关闭的 ch/done）
//   - map 中 channels/stopChs 已被清理
func TestRollbackAcquisition_DoesNotPanicWhenChannelsAlreadyClosedByHandleConnectionLost(t *testing.T) {
	id := "test-rollback-double-close"
	a, driver, _, _ := setupAdapterWithDriver(id, core.StatusAcquiring)

	// 预占采集槽位（模拟 StartAcquisition 入口的预占）
	ch := make(chan core.PressureSnapshot, 1)
	done := make(chan struct{})
	shard := a.shard(id)
	shard.mu.Lock()
	shard.channels[id] = ch
	shard.stopChs[id] = done
	shard.mu.Unlock()

	// 模拟连接故障：handleConnectionLost 先行关闭并删除 done/ch
	a.handleConnectionLost(id, driver, errors.New("simulated connection fault"))

	// 回归点：rollbackAcquisition 不能 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("rollbackAcquisition panicked after handleConnectionLost: %v", r)
		}
	}()
	a.rollbackAcquisition(id, ch, done)

	// 验证 map 已被清理
	shard.mu.RLock()
	_, chExists := shard.channels[id]
	_, doneExists := shard.stopChs[id]
	shard.mu.RUnlock()
	if chExists || doneExists {
		t.Errorf("channels/stopChs should be cleaned, chExists=%v, doneExists=%v", chExists, doneExists)
	}
}

// TestRollbackAcquisition_ClosesChannelsWhenOwnerMatches 验证非连接故障路径下
// rollbackAcquisition 正常关闭 ch/done（owner 检查通过）。
//
// 测试前置：
//   - setupAdapterWithDriver 创建 driver，置 conn=nil 让 sendCommandACK 在入口返回
//     "not connected"，避免 mockConn 的 Read 返回 (0,nil) 导致 P1604ReadCommandACK 挂起
//   - 预占采集槽位（ch/done 写入 map）
//
// 测试步骤：
//   - 调用 rollbackAcquisition(id, ch, done)
//
// 期待结果：
//   - ch/done 被关闭（select 不阻塞）
//   - map 中 channels/stopChs 已被清理
func TestRollbackAcquisition_ClosesChannelsWhenOwnerMatches(t *testing.T) {
	id := "test-rollback-normal"
	a, driver, _, _ := setupAdapterWithDriver(id, core.StatusAcquiring)
	driver.conn = nil

	ch := make(chan core.PressureSnapshot, 1)
	done := make(chan struct{})
	shard := a.shard(id)
	shard.mu.Lock()
	shard.channels[id] = ch
	shard.stopChs[id] = done
	shard.mu.Unlock()

	a.rollbackAcquisition(id, ch, done)

	// 验证 ch/done 已关闭
	select {
	case <-done:
	default:
		t.Error("done should be closed by rollbackAcquisition")
	}
	select {
	case <-ch:
	default:
		t.Error("ch should be closed by rollbackAcquisition")
	}

	// 验证 map 已清理
	shard.mu.RLock()
	_, chExists := shard.channels[id]
	_, doneExists := shard.stopChs[id]
	shard.mu.RUnlock()
	if chExists || doneExists {
		t.Errorf("channels/stopChs should be cleaned after rollback")
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
	}, p1604UnitSyncTimeout)
	if err == nil {
		t.Fatal("syncUnitFromHardware should return error on EOF, got nil")
	}
	if !sharedproto.IsConnResetByPeer(err) {
		t.Errorf("returned error should be detected as conn reset, got: %v", err)
	}
}

func TestSyncUnitFromHardware_TimeoutReturnsError(t *testing.T) {
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
	_, _, err := a.syncUnitFromHardware(driver, core.PressureProfile{
		ID:       "test-timeout",
		P1604Cfg: core.P1604Config{Unit: "Pa"},
	}, p1604UnitSyncTimeout)
	if err == nil {
		t.Fatal("syncUnitFromHardware should return error on timeout")
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

func TestZeroCalibration_WhileAcquiringSendsHardwareCommand(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const id = "test-zero-acquiring"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone) // 标记 readLoop 已退出（测试不启动真实 readLoop）
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusAcquiring,
		StatusText: core.StatusAcquiring.String(),
	}
	shard.mu.Unlock()

	// 模拟 readLoop 的 ASCII 响应路由行为：从 client 读取长度前缀帧并投递到 pendingResponse。
	// 真实 readLoop 调用 processPayload 完成同样路由；此处直接复用 driver.pendingResponse
	// 通道以隔离测试 ZeroCalibration 自身的等待/超时/校验逻辑。
	stopReadLoop := make(chan struct{})
	readLoopExited := make(chan struct{})
	go func() {
		defer close(readLoopExited)
		for {
			select {
			case <-stopReadLoop:
				return
			default:
			}
			client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			frame, err := driver.frameReader.ReadFrame()
			if err != nil {
				continue
			}
			if sharedproto.IsASCIIFrame(frame) {
				driver.pendingResponseMu.Lock()
				ch := driver.pendingResponse
				driver.pendingResponseMu.Unlock()
				if ch != nil {
					select {
					case ch <- frame:
					default:
					}
				}
			}
		}
	}()
	defer func() {
		close(stopReadLoop)
		<-readLoopExited
	}()

	command := make(chan string, 1)
	go func() {
		buf := make([]byte, 16)
		n, _ := server.Read(buf)
		command <- string(buf[:n])
		_, _ = server.Write(framedASCII(zeroCalibrationCoefficients))
	}()

	if err := a.ZeroCalibration(id); err != nil {
		t.Fatalf("ZeroCalibration while acquiring returned error: %v", err)
	}

	select {
	case got := <-command:
		if got != "h" {
			t.Fatalf("zero calibration command = %q, want %q", got, "h")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for zero calibration command")
	}

	state, ok := a.Status(id)
	if !ok {
		t.Fatal("device status missing after zero calibration")
	}
	if state.Status != core.StatusAcquiring {
		t.Fatalf("status after zero calibration = %v, want Acquiring", state.Status)
	}
}

func TestZeroCalibration_WaitsForAcquisitionTransition(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const id = "test-zero-transition-lock"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		acquiring:    false,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{Profile: driver.profile, Status: core.StatusConnected}
	shard.mu.Unlock()

	driver.operationMu.Lock()
	done := make(chan error, 1)
	go func() { done <- a.ZeroCalibration(id) }()

	_ = server.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	buf := make([]byte, 16)
	if n, err := server.Read(buf); err == nil || n != 0 {
		t.Fatalf("zero calibration sent command during acquisition transition: %q", string(buf[:n]))
	}
	driver.operationMu.Unlock()
	_ = server.SetReadDeadline(time.Time{})

	go func() {
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := server.Read(buf)
		if string(buf[:n]) == "h" {
			_, _ = server.Write(framedASCII(zeroCalibrationCoefficients))
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ZeroCalibration returned error after transition completed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ZeroCalibration did not continue after transition lock was released")
	}
}

func TestDisconnect_WaitsForAcquisitionTransition(t *testing.T) {
	const id = "test-disconnect-transition-lock"
	a := NewP1604Adapter()
	driver := &p1604Driver{profile: core.PressureProfile{ID: id}}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{Profile: driver.profile, Status: core.StatusConnected}
	shard.mu.Unlock()

	driver.operationMu.Lock()
	done := make(chan error, 1)
	go func() { done <- a.Disconnect(id) }()

	select {
	case err := <-done:
		t.Fatalf("Disconnect completed during acquisition transition: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	driver.operationMu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Disconnect returned error after transition completed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Disconnect did not continue after transition lock was released")
	}
}

func TestApplyConfig_WaitsForAcquisitionTransition(t *testing.T) {
	const id = "test-config-transition-lock"
	cfg := core.P1604Config{Unit: sharedproto.P1604DefaultUnit}
	a := NewP1604Adapter()
	driver := &p1604Driver{profile: core.PressureProfile{ID: id, P1604Cfg: cfg}}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile: core.PressureProfile{ID: id, P1604Cfg: cfg},
		Status:  core.StatusConnected,
	}
	shard.mu.Unlock()

	driver.operationMu.Lock()
	done := make(chan error, 1)
	go func() { done <- a.ApplyConfig(id, cfg) }()

	select {
	case err := <-done:
		t.Fatalf("ApplyConfig completed during acquisition transition: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	driver.operationMu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ApplyConfig returned error after transition completed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ApplyConfig did not continue after transition lock was released")
	}
}

// TestZeroCalibration_IdleSuccess 验证空闲期间请求/响应路径成功。
//
// 测试前置：构造未采集的 driver + idleStopCh/idleLoopDone（idleReadLoop 占位）。
// 测试步骤：服务端读 "h" 命令后回 16 路零位系数，调用 ZeroCalibration。
// 期待结果：返回 nil，driver.idleStopCh 被重启为新 channel（说明 defer 重启了 idleReadLoop）。
func TestZeroCalibration_IdleSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const id = "test-zero-idle"
	a := NewP1604Adapter()
	idleStop := make(chan struct{})
	idleDone := make(chan struct{})
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		idleStopCh:   idleStop,
		idleLoopDone: idleDone,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	go func() {
		<-idleStop
		close(idleDone)
	}()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	go func() {
		buf := make([]byte, 16)
		_, _ = server.Read(buf) // 读 "h" 命令
		_, _ = server.Write(framedASCII(zeroCalibrationCoefficients))
	}()

	if err := a.ZeroCalibration(id); err != nil {
		t.Fatalf("ZeroCalibration idle returned error: %v", err)
	}

	shard.mu.RLock()
	restartedStop := driver.idleStopCh
	shard.mu.RUnlock()
	if restartedStop == nil || restartedStop == idleStop {
		t.Fatal("ZeroCalibration must restart idleReadLoop with a new stop channel")
	}
	// 清理重启的 idleReadLoop
	close(restartedStop)
}

// TestZeroCalibration_DeviceRejectsWithNxx 验证设备返回 Nxx 时 ZeroCalibration 返回错误。
//
// 测试前置：构造采集中的 driver + 模拟 readLoop 路由 ASCII 响应。
// 测试步骤：服务端读 "h" 后回 "N05" 响应（设备拒绝）。
// 期待结果：ZeroCalibration 返回 error 且包含"被设备拒绝"字样。
func TestZeroCalibration_DeviceRejectsWithNxx(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const id = "test-zero-nxx"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusAcquiring,
		StatusText: core.StatusAcquiring.String(),
	}
	shard.mu.Unlock()

	stopReadLoop := make(chan struct{})
	readLoopExited := make(chan struct{})
	go func() {
		defer close(readLoopExited)
		for {
			select {
			case <-stopReadLoop:
				return
			default:
			}
			client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			frame, err := driver.frameReader.ReadFrame()
			if err != nil {
				continue
			}
			if sharedproto.IsASCIIFrame(frame) {
				driver.pendingResponseMu.Lock()
				ch := driver.pendingResponse
				driver.pendingResponseMu.Unlock()
				if ch != nil {
					select {
					case ch <- frame:
					default:
					}
				}
			}
		}
	}()
	defer func() {
		close(stopReadLoop)
		<-readLoopExited
	}()

	go func() {
		buf := make([]byte, 16)
		_, _ = server.Read(buf) // 读 "h" 命令
		// 回送 "N05" 响应（设备拒绝）
		_, _ = server.Write([]byte{0x00, 0x04, 'N', '0', '5'})
	}()

	err := a.ZeroCalibration(id)
	if err == nil {
		t.Fatal("ZeroCalibration should return error on N05 response, got nil")
	}
	if !strings.Contains(err.Error(), "拒绝") {
		t.Errorf("error should mention device rejection, got: %v", err)
	}
}

func TestVerifyZeroCalibrationResponse_AcceptsSixteenZeroCoefficients(t *testing.T) {
	resp := []byte(zeroCalibrationCoefficients)

	if err := verifyZeroCalibrationResponse(resp); err != nil {
		t.Fatalf("valid zero coefficient response rejected: %v", err)
	}
}

func TestVerifyZeroCalibrationResponse_RejectsMalformedCoefficients(t *testing.T) {
	tests := []struct {
		name string
		resp string
	}{
		{name: "ack only", resp: "A"},
		{name: "too few", resp: "0.1 0.2"},
		{name: "not numeric", resp: "0 0 0 0 0 0 0 bad 0 0 0 0 0 0 0 0"},
		{name: "not finite", resp: "0 0 0 0 0 0 0 NaN 0 0 0 0 0 0 0 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := verifyZeroCalibrationResponse([]byte(tt.resp)); err == nil {
				t.Fatalf("malformed response accepted: %q", tt.resp)
			}
		})
	}
}

// TestZeroCalibration_TimeoutWhenNoResponse 验证设备不响应时 ZeroCalibration 超时返回错误。
//
// 测试前置：构造采集中的 driver + 模拟 readLoop（无响应可路由）。
// 测试步骤：服务端读 "h" 后不回任何响应，ZeroCalibration 等待 p1604CalibrationTimeout。
// 期待结果：返回 timeout 错误。为避免测试卡 2s，临时缩短 timeout。
//
// 注意：本测试不修改全局 p1604CalibrationTimeout，依赖默认 2s 超时，验证后即返回。
// 测试总耗时约 2s，是覆盖"超时"路径的最小代价。
func TestZeroCalibration_TimeoutWhenNoResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	const id = "test-zero-timeout"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusAcquiring,
		StatusText: core.StatusAcquiring.String(),
	}
	shard.mu.Unlock()

	stopReadLoop := make(chan struct{})
	readLoopExited := make(chan struct{})
	go func() {
		defer close(readLoopExited)
		for {
			select {
			case <-stopReadLoop:
				return
			default:
			}
			client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			frame, err := driver.frameReader.ReadFrame()
			if err != nil {
				continue
			}
			if sharedproto.IsASCIIFrame(frame) {
				driver.pendingResponseMu.Lock()
				ch := driver.pendingResponse
				driver.pendingResponseMu.Unlock()
				if ch != nil {
					select {
					case ch <- frame:
					default:
					}
				}
			}
		}
	}()
	defer func() {
		close(stopReadLoop)
		<-readLoopExited
	}()

	go func() {
		buf := make([]byte, 16)
		_, _ = server.Read(buf) // 读 "h" 命令，不回响应
		select {}
	}()

	start := time.Now()
	err := a.ZeroCalibration(id)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ZeroCalibration should return timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Errorf("error should mention timeout, got: %v", err)
	}
	// 验证确实等了约 2s（允许 ±500ms 抖动），而不是立即返回
	if elapsed < p1604CalibrationTimeout-500*time.Millisecond {
		t.Errorf("ZeroCalibration returned too fast: %v (expected ~%v)", elapsed, p1604CalibrationTimeout)
	}
}

func TestPendingResponseDeliveryAndDisconnectAreSerialized(t *testing.T) {
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		driver := &p1604Driver{pendingResponse: make(chan []byte, 1)}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			driver.pendingResponseMu.Lock()
			if ch := driver.pendingResponse; ch != nil {
				select {
				case ch <- []byte("A"):
				default:
				}
			}
			driver.pendingResponseMu.Unlock()
		}()

		go func() {
			defer wg.Done()
			<-start
			driver.pendingResponseMu.Lock()
			if ch := driver.pendingResponse; ch != nil {
				driver.pendingResponse = nil
				close(ch)
			}
			driver.pendingResponseMu.Unlock()
		}()

		close(start)
		wg.Wait()
	}
}

// TestZeroCalibration_DisconnectionTriggersConnectionLost 验证设备 FIN 时
// ZeroCalibration 触发 handleConnectionLost 清理 driver+conn。
//
// 测试前置：构造空闲 driver + idleStopCh/idleLoopDone（idleReadLoop 占位）。
// 测试步骤：服务端读 "h" 后 sleep 50ms 让客户端进入 Read 阻塞，再 Close 触发 io.EOF。
// 期待结果：ZeroCalibration 返回 error；shard.drivers[id] 被删除；status=Error。
func TestZeroCalibration_DisconnectionTriggersConnectionLost(t *testing.T) {
	server, client := net.Pipe()

	const id = "test-zero-disconnect"
	a := NewP1604Adapter()
	idleStop := make(chan struct{})
	idleDone := make(chan struct{})
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		idleStopCh:   idleStop,
		idleLoopDone: idleDone,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	go func() {
		<-idleStop
		close(idleDone)
	}()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	// 服务端：读 "h" → sleep 50ms 让客户端 ReadFrame 进入阻塞 → Close 触发 io.EOF
	go func() {
		buf := make([]byte, 16)
		_, _ = server.Read(buf)
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()

	err := a.ZeroCalibration(id)
	if err == nil {
		t.Fatal("ZeroCalibration should return error on disconnect, got nil")
	}

	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()
	if driverExists {
		t.Error("driver should be deleted by handleConnectionLost after disconnect")
	}
	if !statusExists {
		t.Fatal("status should still exist (set to Error) for frontend visibility")
	}
	if st.Status != core.StatusError {
		t.Errorf("status should be Error, got %v", st.Status)
	}

	_ = client.Close()
}

// TestApplyConfig_V01101ErrorKeepsDriver 验证设备拒绝 v01101 时返回错误。
// 不触发 handleConnectionLost，driver 保留在 shard，前端可继续 Disconnect/重试。
//
// 模拟方式：服务端回复一个非 A 的 ASCII 帧（模拟设备拒绝）。
// 注意：P1604WriteUnitCoefficient 对非 A 非 N 的响应会返回 "unexpected v01101 response"，
// 对 N 开头返回 "device rejected unit change"。这里用 N01 触发后者（软错误）。
func TestApplyConfig_V01101ErrorKeepsDriver(t *testing.T) {
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

	// 服务端：读掉 v01101 → 回复 N01 帧。
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
	// 设备拒绝命令不代表 TCP 已断开，driver 必须保留。
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

// buildResidualFrame 构造一个二进制压力帧（5-byte header + 16 x float32），
// 用于模拟采集残留帧。帧长度 5 + 64 = 69，加上 2 字节长度前缀共 71 字节。
//
// 第一字节固定 0x01（设备协议规定的二进制流帧头同步标记），让 IsASCIIFrame 返回 false。
func buildResidualFrame() []byte {
	const payloadLen = 5 + 16*4 // header + 16 floats
	frame := make([]byte, 2+payloadLen)
	binary.BigEndian.PutUint16(frame, uint16(payloadLen+2))
	frame[2] = 0x01 // 二进制流帧头
	// 其余字节填 0xBB 制造明显的非 ASCII 模式
	for i := 3; i < len(frame); i++ {
		frame[i] = 0xBB
	}
	return frame
}

// startTestTCPServer 启动一个测试用 TCP 服务端，accept 一个连接，
// 通过 handler 函数让调用方控制写入内容。返回 listener 和服务端 conn。
func startTestTCPServer(t *testing.T, handler func(server net.Conn)) (ln net.Listener, client net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		accepted <- conn
		if handler != nil {
			handler(conn)
		}
	}()
	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	server := <-accepted
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		ln.Close()
	})
	return ln, client
}

// readCommandAndAck 用于测试 handler：读掉客户端发来的命令，然后写入指定的响应序列
func readCommandAndAck(server net.Conn, cmdLen int, responses [][]byte) {
	buf := make([]byte, cmdLen)
	_, _ = server.Read(buf)
	for _, resp := range responses {
		_, _ = server.Write(resp)
	}
}

// TestSendCommandACK_SkipsResidualFramesBeforeACK
//
// 测试前置：构造 TCP 服务端，连接后发送命令前先写 3 个残留压力帧 + 1 个 ACK
// 测试步骤：调用 sendCommandACK("c 02 1")
// 期待结果：返回 nil（成功），3 个残留帧被自动跳过
//
// 优先级 P0：覆盖 StopAcquisition 后快速 StartAcquisition 的现场场景
func TestSendCommandACK_SkipsResidualFramesBeforeACK(t *testing.T) {
	responses := [][]byte{
		buildResidualFrame(), // 残留帧 1
		buildResidualFrame(), // 残留帧 2
		buildResidualFrame(), // 残留帧 3
		framedASCII("A"),     // 真正的 ACK
	}
	_, client := startTestTCPServer(t, func(server net.Conn) {
		readCommandAndAck(server, len("c 02 1"), responses)
	})

	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-skip-residual"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}
	if err := driver.sendCommandACK("c 02 1"); err != nil {
		t.Fatalf("sendCommandACK should skip residual frames and return nil, got: %v", err)
	}
}

// TestSendCommandACK_TooManyResidualFramesReturnsError
//
// 测试前置：构造 TCP 服务端，连接后发送 20+ 个残留压力帧，不发送任何 ACK
// 测试步骤：调用 sendCommandACK("c 02 1")
// 期待结果：返回错误（"too many residual frames"），不误报成功
//
// 优先级 P0：Critical bug 回归测试——循环正常结束不能误报成功
func TestSendCommandACK_TooManyResidualFramesReturnsError(t *testing.T) {
	// 构造 p1604MaxResidualFrameSkips + 2 个残留帧，确保循环跑满上限
	responses := make([][]byte, p1604MaxResidualFrameSkips+2)
	for i := range responses {
		responses[i] = buildResidualFrame()
	}
	_, client := startTestTCPServer(t, func(server net.Conn) {
		readCommandAndAck(server, len("c 02 1"), responses)
	})

	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-too-many-residual"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}
	err := driver.sendCommandACK("c 02 1")
	if err == nil {
		t.Fatal("sendCommandACK must return error when too many residual frames, got nil (false success)")
	}
	if !strings.Contains(err.Error(), "too many residual frames") {
		t.Errorf("error should mention 'too many residual frames', got: %v", err)
	}
}

// TestSendCommandACK_ResidualThenNxxReturnsDeviceError
//
// 测试前置：构造 TCP 服务端，发送 2 个残留帧后跟一个 N05 设备拒绝应答
// 测试步骤：调用 sendCommandACK("c 05 1 0C10")
// 期待结果：返回错误（"device returned error: N05"），跳过残留帧后正确识别拒绝
//
// 优先级 P1：残留帧不应掩盖设备的真实拒绝应答
func TestSendCommandACK_ResidualThenNxxReturnsDeviceError(t *testing.T) {
	responses := [][]byte{
		buildResidualFrame(), // 残留帧 1
		buildResidualFrame(), // 残留帧 2
		framedASCII("N05"),   // 设备拒绝
	}
	_, client := startTestTCPServer(t, func(server net.Conn) {
		readCommandAndAck(server, len("c 05 1 0C10"), responses)
	})

	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-residual-nxx"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}
	err := driver.sendCommandACK("c 05 1 0C10")
	if err == nil {
		t.Fatal("sendCommandACK should return Nxx device error, got nil")
	}
	if !strings.Contains(err.Error(), "device returned error: N05") {
		t.Errorf("error should mention 'device returned error: N05', got: %v", err)
	}
}

// TestSendCommandACK_NoResidualReturnsACKDirectly
//
// 测试前置：构造 TCP 服务端，发送命令后直接回一个 A（无残留帧）
// 测试步骤：调用 sendCommandACK("c 01 1")
// 期待结果：返回 nil（成功），循环只跑一次
//
// 优先级 P1：正常路径不能因跳帧逻辑引入回归
func TestSendCommandACK_NoResidualReturnsACKDirectly(t *testing.T) {
	_, client := startTestTCPServer(t, func(server net.Conn) {
		readCommandAndAck(server, len("c 01 1"), [][]byte{framedASCII("A")})
	})

	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-no-residual"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}
	if err := driver.sendCommandACK("c 01 1"); err != nil {
		t.Fatalf("sendCommandACK should return nil for normal ACK, got: %v", err)
	}
}

// deadlineIgnoringConn 是忽略 SetReadDeadline/SetWriteDeadline 的连接替身，
// 仅在 Close 后返回。用于模拟 Windows 故障环境下 deadline 失效场景（ADR-009）。
//
// 设计依据：ADR-009 要求有界网络 I/O 的测试必须包含忽略 deadline、只在 Close 后
// 返回的连接 double。Windows 内核级 bug 会导致 SetReadDeadline/SetWriteDeadline
// 到期后阻塞 Read/Write 仍不返回，必须依赖 watchdog Close conn 解除阻塞。
//
// 参考 shared/device-sdk/go/protocol/conn_helpers_test.go 同名替身，
// 本替身额外覆盖 SetWriteDeadline 以支持 Write 阶段 watchdog 触发测试（P1-2.b/c）。
type deadlineIgnoringConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func newDeadlineIgnoringConn(inner net.Conn) *deadlineIgnoringConn {
	return &deadlineIgnoringConn{
		Conn:   inner,
		closed: make(chan struct{}),
	}
}

// SetReadDeadline 覆盖为 no-op，模拟 Windows 故障环境下 Read deadline 失效。
func (c *deadlineIgnoringConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 覆盖为 no-op，模拟 Windows 故障环境下 Write deadline 失效。
// 这是 P1-2.b/c 测试的关键：让 Write 阶段无限阻塞，迫使 watchdog 触发 Close。
func (c *deadlineIgnoringConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Close 关闭连接并通知 closed channel，让阻塞中的 Read/Write 立即返回。
func (c *deadlineIgnoringConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// TestP1604SendCommandACK_WatchdogTriggersOnWriteDeadlineIgnoringConn 验证
// Write 阶段 watchdog 兜底（P1-2.b 修复）。
//
// 修复前 bug：sendCommandACK 的 wdStop 在 sendCommand 之后启动，
// Write 阶段完全无 watchdog 覆盖。当 SetWriteDeadline 失效（Write 无限阻塞）时，
// sendCommandACK 永远卡在 sendCommand，无法返回。
//
// 修复后：wdStop 提到 sendCommand 之前，覆盖 Write + Read 全流程。
// Write 阻塞 → watchdog 触发 Close → Write 返回错误 → sendCommandACK 返回
// "watchdog triggered" 错误。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 包装为 deadlineIgnoringConn
//   - 服务端不读取任何数据（确保 Write 阻塞）
//
// 测试步骤：
//   - 调用 sendCommandACK("c 02 1")
//   - 等待 p1604WatchdogTimeout + 余量 内返回
//
// 期待结果：
//   - 返回错误，错误信息包含 "watchdog triggered"
//   - 在 p1604WatchdogTimeout + 2s 余量 内返回（不卡死）
func TestP1604SendCommandACK_WatchdogTriggersOnWriteDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)

	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-write-watchdog"},
		conn:        ignored,
		frameReader: sharedproto.NewFrameReader(ignored),
	}

	// 服务端不读取任何数据，确保 client.Write 阻塞
	// （net.Pipe 无缓冲，Write 必须等对端 Read）

	done := make(chan error, 1)
	go func() {
		done <- driver.sendCommandACK("c 02 1")
	}()

	// 预算：watchdog timeout (5s) + 2s 余量（覆盖 goroutine 调度 + Close 完成时间）
	budget := p1604WatchdogTimeout + 2*time.Second

	var err error
	timedOut := false

	select {
	case err = <-done:
	case <-time.After(budget):
		// RED 阶段：当前代码无 Write-phase watchdog，sendCommandACK 卡死
		timedOut = true
		// Close conn 解除 Write 阻塞，让 goroutine 退出避免泄漏
		_ = ignored.Close()
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("sendCommandACK did not exit after conn close")
		}
	}

	if timedOut {
		t.Fatalf("sendCommandACK hung > %v — Write-phase watchdog missing (P1-2.b)", budget)
	}

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
}

// TestP1604SendCommandACK_SoftTimeoutClosesConnAndReturnsSentinel 验证 ADR-009 R0-12：
// 当 SetReadDeadline 正常兑现（soft deadline 先于 watchdog 触发）时，sendCommandACK
// 必须强制 Close conn 阻断迟到响应，并返回 ErrWatchdogTriggered sentinel 让调用方
// 通过 IsWatchdogTriggered 毒化驱动状态。
//
// 修复前 bug：soft deadline 兑现时 sendCommandACK 仅返回普通 timeout 错误，不清
// deadline、不 Close conn。迟到响应随后进入 TCP 流被下一条命令消费，导致协议错位
// （如 c 00 的 ACK 被上一条 c 01 的迟到 ACK 冒充）。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 正常工作）
//   - 临时覆盖 p1604CommandResponseTimeout 为 100ms 加速
//   - 服务端读取命令但不发响应（确保 client.Read 在 deadline 到期后返回 timeout）
//
// 测试步骤：
//   - 调用 sendCommandACK("c 02 1")
//   - 等待函数返回后，服务端再写入一帧迟到响应（模拟设备延迟回复）
//
// 期待结果：
//   - 函数在 5s 预算内返回
//   - 错误通过 errors.Is(err, ErrWatchdogTriggered) 精确匹配 sentinel
//   - conn 已被 Close：服务端写入迟到响应时 Write 失败
func TestP1604SendCommandACK_SoftTimeoutClosesConnAndReturnsSentinel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 sendCommandACK Close，不在 defer 中重复 Close

	// 临时覆盖 soft deadline 为 100ms 加速测试。
	// p1604WatchdogTimeout（5s const）远大于 100ms，确保 soft deadline 先触发。
	origTimeout := p1604CommandResponseTimeout
	p1604CommandResponseTimeout = 100 * time.Millisecond
	t.Cleanup(func() { p1604CommandResponseTimeout = origTimeout })

	driver := &p1604Driver{
		profile:     core.PressureProfile{ID: "test-soft-timeout"},
		conn:        client,
		frameReader: sharedproto.NewFrameReader(client),
	}

	// 服务端读取命令但不发响应，让 client.Read 在 soft deadline 到期后返回 timeout
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 故意不写响应，触发 soft timeout
	}()

	done := make(chan error, 1)
	go func() {
		done <- driver.sendCommandACK("c 02 1")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected soft timeout error wrapping ErrWatchdogTriggered, got nil")
		}
		if !sharedproto.IsWatchdogTriggered(err) {
			t.Errorf("error must wrap ErrWatchdogTriggered for soft timeout, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sendCommandACK did not return within 5s budget")
	}

	// 验证 conn 已被 Close：服务端写入迟到响应应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte{0x00, 0x01, 'A'}); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout")
	}
	wg.Wait()
}

// TestP1604ZeroCalibrationDirect_WatchdogTriggersOnWriteDeadlineIgnoringConn 验证
// zeroCalibrationDirect Write 阶段 watchdog 兜底（P1-2.c 修复）。
//
// 修复前 bug：zeroCalibrationDirect 的 wdStopRead 在 sendCommand("h") 之后启动，
// Write 阶段完全无 watchdog 覆盖。当 SetWriteDeadline 失效时，ZeroCalibration
// 永远卡在 sendCommand，无法返回。
//
// 修复后：wdStop 提到 sendCommand("h") 之前，覆盖 Write + Read 全流程。
//
// 测试前置：
//   - 构造未采集的 driver + idleStopCh/idleLoopDone（idleReadLoop 占位）
//   - client 包装为 deadlineIgnoringConn
//   - 服务端不读取任何数据（确保 Write 阻塞）
//
// 测试步骤：
//   - 调用 a.zeroCalibrationDirect(id, driver)
//   - 等待 p1604WatchdogTimeout + 余量 内返回
//
// 期待结果：
//   - 返回错误，错误信息包含 "watchdog triggered"
//   - 在 p1604WatchdogTimeout + 2s 余量 内返回（不卡死）
func TestP1604ZeroCalibrationDirect_WatchdogTriggersOnWriteDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)

	const id = "test-zero-write-watchdog"
	a := NewP1604Adapter()
	idleStop := make(chan struct{})
	idleDone := make(chan struct{})
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         ignored,
		frameReader:  sharedproto.NewFrameReader(ignored),
		idleStopCh:   idleStop,
		idleLoopDone: idleDone,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	go func() {
		<-idleStop
		close(idleDone)
	}()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	// 服务端不读取任何数据，确保 client.Write 阻塞

	done := make(chan error, 1)
	go func() {
		done <- a.zeroCalibrationDirect(id, driver)
	}()

	budget := p1604WatchdogTimeout + 2*time.Second

	var err error
	timedOut := false

	select {
	case err = <-done:
	case <-time.After(budget):
		timedOut = true
		_ = ignored.Close()
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("zeroCalibrationDirect did not exit after conn close")
		}
	}

	if timedOut {
		t.Fatalf("zeroCalibrationDirect hung > %v — Write-phase watchdog missing (P1-2.c)", budget)
	}

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
}

// TestP1604ReadLoop_InvalidatesConnOnNoDataTimeout 验证 ADR-009 R0-10：
// readLoop 入口启动的独立 no-data timer 在 deadline 失效（Read 永久阻塞）且
// 无任何数据到达时，必须在 p1604NoDataTimeout 到期后独立触发 handleConnectionLost——
// 清理 driver、close conn、置 Error 状态、通知前端。
//
// 关键差异（与 terminal read error 测试的对比）：
//   - terminal read error：对端 EOF → ReadFrame 返回错误 → readLoop 调 handleConnectionLost
//   - no-data timer：无数据 → timer 到期 → 调 handleConnectionLost → Close conn
//     → ReadFrame 返回 closed 错误 → readLoop 再次调 handleConnectionLost（被 driver 身份检查跳过）
//
// timer 必须独立于 readLoop 循环体执行。本测试用 deadlineIgnoringConn 让 Read
// 永久阻塞（模拟 Windows 故障环境下 deadline 失效），循环体不可达，
// 仅靠 timer 到期能触发 handleConnectionLost。若 timer 未启动或依赖循环体，测试会超时。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn（Read 永久阻塞）
//   - driver.conn / frameReader / readLoopDone 已设置，acquiring=true
//   - driver 已注册到 shard.drivers[id]，status 已注册到 shard.status[id]
//   - p1604NoDataTimeout 临时覆盖为 200ms（生产默认 5s）
//   - server 端不写任何数据，让 client Read 永久阻塞
//
// 测试步骤：
//   - 启动 readLoop goroutine
//   - 等待 noDataTimeout(200ms) + 余量(800ms) = 1s 预算
//
// 期待结果：
//   - shard.drivers[id] 被删除（handleConnectionLost 清理）
//   - shard.status[id].Status = StatusError
//   - driver.conn 已被 Close（deadlineIgnoringConn.closed 已关闭）
//   - readLoopDone 已关闭（readLoop 已退出）
func TestP1604ReadLoop_InvalidatesConnOnNoDataTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// 临时覆盖 p1604NoDataTimeout 为 200ms 加速测试。
	// 同一包内测试默认串行执行，覆盖安全；t.Cleanup 确保恢复。
	origTimeout := p1604NoDataTimeout
	p1604NoDataTimeout = 200 * time.Millisecond
	t.Cleanup(func() { p1604NoDataTimeout = origTimeout })

	const id = "test-p1604-nodata"
	a := NewP1604Adapter()
	ignored := newDeadlineIgnoringConn(client)
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         ignored,
		frameReader:  sharedproto.NewFrameReader(ignored),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile: driver.profile,
		Status:  core.StatusAcquiring,
	}
	shard.mu.Unlock()

	stop := make(chan struct{})
	go a.readLoop(id, driver, stop)

	// 预算 1s：覆盖 200ms noDataTimer + Read 在 conn Close 后返回的延迟 + defer 清理。
	// 若 timer 未触发，readLoop 永久阻塞，select 会超时 fatal。
	select {
	case <-driver.readLoopDone:
	case <-time.After(1 * time.Second):
		t.Fatal("readLoop did not exit within 1s; no-data timer may not have fired")
	}

	// 短暂等待 handleConnectionLost 完成状态清理
	time.Sleep(100 * time.Millisecond)

	// 验证 driver 已被 handleConnectionLost 清理
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()
	if driverExists {
		t.Error("driver should be deleted from shard.drivers after no-data timer fired")
	}
	if !statusExists {
		t.Error("shard.status[id] should still exist (handleConnectionLost sets Error, not deletes)")
	}
	if st != nil && st.Status != core.StatusError {
		t.Errorf("status = %v, want Error", st.Status)
	}

	// 验证 conn 已被 Close（deadlineIgnoringConn.closed channel 已关闭）
	select {
	case <-ignored.closed:
		// 预期：conn 已被 Close
	default:
		t.Error("conn should be closed after no-data timer fired")
	}
}

// TestZeroCalibration_ResponseTimeoutInvalidatesConn 验证 ADR-009 R0-12 + R1-2：
// 采集期间校零，设备不回响应导致 select 超时后，必须调 handleConnectionLost 毒化连接
// （删除 driver、置 status=Error、close conn），不能仅返回 timeout 错误保留 conn。
//
// 整改前 bug：zeroCalibrationViaReadLoop 的 select 超时只返回错误，不调 handleConnectionLost。
// 后果：连接已半开（设备无响应但 TCP 未断），迟到响应可能随后进入 TCP 流被下一条命令消费
// 导致协议错位；driver 仍保留在 shard.drivers，后续 StartAcquisition 会复用死连接爆 WSAECONNABORTED。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - driver.acquiring=true，走 zeroCalibrationViaReadLoop 路径
//   - mock readLoop goroutine 循环 ReadFrame，将 ASCII 帧投递到 pendingResponse
//   - server 读取 "h" 命令但不回响应
//
// 测试步骤：
//   - 调用 ZeroCalibration
//   - 等待 p1604CalibrationTimeout（2s）+ 余量
//
// 期待结果：
//   - 返回错误包含"超时"
//   - shard.drivers[id] 被删除（handleConnectionLost 清理）
//   - shard.status[id].Status = StatusError
//   - conn 已被 Close
func TestZeroCalibration_ResponseTimeoutInvalidatesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	const id = "test-zero-resp-timeout-invalidate"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         client,
		frameReader:  sharedproto.NewFrameReader(client),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusAcquiring,
		StatusText: core.StatusAcquiring.String(),
	}
	shard.mu.Unlock()

	// mock readLoop：循环 ReadFrame，将 ASCII 帧投递到 pendingResponse
	stopReadLoop := make(chan struct{})
	readLoopExited := make(chan struct{})
	go func() {
		defer close(readLoopExited)
		for {
			select {
			case <-stopReadLoop:
				return
			default:
			}
			client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			frame, err := driver.frameReader.ReadFrame()
			if err != nil {
				continue
			}
			if sharedproto.IsASCIIFrame(frame) {
				driver.pendingResponseMu.Lock()
				ch := driver.pendingResponse
				driver.pendingResponseMu.Unlock()
				if ch != nil {
					select {
					case ch <- frame:
					default:
					}
				}
			}
		}
	}()
	defer func() {
		close(stopReadLoop)
		<-readLoopExited
	}()

	// server 读取 "h" 命令但不回响应，让 select 超时
	go func() {
		buf := make([]byte, 16)
		_, _ = server.Read(buf)
		select {}
	}()

	start := time.Now()
	err := a.ZeroCalibration(id)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ZeroCalibration should return timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Errorf("error should mention timeout, got: %v", err)
	}
	if elapsed < p1604CalibrationTimeout-500*time.Millisecond {
		t.Errorf("ZeroCalibration returned too fast: %v (expected ~%v)", elapsed, p1604CalibrationTimeout)
	}

	// 短暂等待 handleConnectionLost 完成状态清理
	time.Sleep(100 * time.Millisecond)

	// 验证 driver 已被 handleConnectionLost 清理
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()
	if driverExists {
		t.Error("driver should be deleted by handleConnectionLost after response timeout")
	}
	if !statusExists {
		t.Fatal("status should still exist (set to Error) for frontend visibility")
	}
	if st.Status != core.StatusError {
		t.Errorf("status should be Error, got %v", st.Status)
	}

	_ = client.Close()
}

// TestZeroCalibrationViaReadLoop_WriteWatchdogTriggersConnectionLost 验证 ADR-009 R1-2：
// 采集期间校零，sendCommand 的 Write 永久阻塞（SetWriteDeadline 失效）时，
// zeroCalibrationViaReadLoop 的 watchdog 必须在 p1604WatchdogTimeout 后 close conn，
// 解除 Write 阻塞，并调 handleConnectionLost 毒化连接。
//
// 整改前 bug：zeroCalibrationViaReadLoop 无 watchdog 兜底，sendCommand 内部仅 SetWriteDeadline。
// SetWriteDeadline 失效时 Write 永久阻塞，operationMu 永远无法释放，后续 Disconnect/StopAcquisition
// 永久等待。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn（SetWriteDeadline 失效）
//   - driver.acquiring=true，走 zeroCalibrationViaReadLoop 路径
//   - p1604WatchdogTimeout 临时覆盖为 200ms 加速
//   - server 不读取任何数据，确保 client.Write 阻塞
//
// 测试步骤：
//   - 调用 ZeroCalibration
//   - 等待 watchdog 触发（200ms）+ 余量
//
// 期待结果：
//   - 返回错误包含"watchdog triggered"
//   - shard.drivers[id] 被删除（handleConnectionLost 清理）
//   - shard.status[id].Status = StatusError
//   - conn 已被 Close（deadlineIgnoringConn.closed channel 已关闭）
func TestZeroCalibrationViaReadLoop_WriteWatchdogTriggersConnectionLost(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// 临时覆盖 p1604WatchdogTimeout 为 200ms 加速测试。
	origWatchdog := p1604WatchdogTimeout
	p1604WatchdogTimeout = 200 * time.Millisecond
	t.Cleanup(func() { p1604WatchdogTimeout = origWatchdog })

	ignored := newDeadlineIgnoringConn(client)

	const id = "test-zero-write-watchdog-r1-2"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         ignored,
		frameReader:  sharedproto.NewFrameReader(ignored),
		acquiring:    true,
		readLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusAcquiring,
		StatusText: core.StatusAcquiring.String(),
	}
	shard.mu.Unlock()

	// server 不读取任何数据，确保 client.Write 阻塞
	go func() {
		select {}
	}()

	done := make(chan error, 1)
	go func() {
		done <- a.ZeroCalibration(id)
	}()

	budget := p1604WatchdogTimeout + 2*time.Second
	var err error
	timedOut := false

	select {
	case err = <-done:
	case <-time.After(budget):
		timedOut = true
		_ = ignored.Close()
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("ZeroCalibration did not exit after conn close")
		}
	}

	if timedOut {
		t.Fatalf("ZeroCalibration hung > %v — Write-phase watchdog missing (R1-2)", budget)
	}

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}

	// 验证 driver 已被 handleConnectionLost 清理
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()
	if driverExists {
		t.Error("driver should be deleted by handleConnectionLost after watchdog triggered")
	}
	if !statusExists {
		t.Fatal("status should still exist (set to Error) for frontend visibility")
	}
	if st.Status != core.StatusError {
		t.Errorf("status should be Error, got %v", st.Status)
	}

	// 验证 conn 已被 Close
	select {
	case <-ignored.closed:
		// 预期：conn 已被 Close
	default:
		t.Error("conn should be closed after watchdog triggered")
	}
}

// TestZeroCalibration_LockWaitWatchdogTriggersConnectionLost 验证 ADR-009 R1-2：
// 前序操作持锁卡死（sendCommand Write 阻塞）时，ZeroCalibration 的外层 watchdog
// 必须在 p1604WatchdogTimeout 后 close conn，解除前序操作的 Write 阻塞，
// 让 operationMu 得以释放。ZeroCalibration 获取锁后检查 watchdog 已触发，
// 调 handleConnectionLost 毒化连接。
//
// 整改前 bug：ZeroCalibration 直接 operationMu.Lock()，锁等待阶段无 watchdog 兜底。
// 前序操作持锁卡死时 ZeroCalibration 永久阻塞在 operationMu.Lock() 上。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn（SetWriteDeadline 失效）
//   - 前序 goroutine 获取 operationMu 后调用 sendCommand（Write 永久阻塞）
//   - p1604WatchdogTimeout 临时覆盖为 200ms 加速
//   - server 不读取任何数据，确保 client.Write 阻塞
//
// 测试步骤：
//   - 启动前序 goroutine 持锁卡死
//   - 调用 ZeroCalibration（等待 operationMu）
//   - 等待 watchdog 触发（200ms）+ 余量
//
// 期待结果：
//   - 返回错误包含"watchdog triggered"
//   - shard.drivers[id] 被删除（handleConnectionLost 清理）
//   - shard.status[id].Status = StatusError
//   - conn 已被 Close
func TestZeroCalibration_LockWaitWatchdogTriggersConnectionLost(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// 临时覆盖 p1604WatchdogTimeout 为 200ms 加速测试。
	origWatchdog := p1604WatchdogTimeout
	p1604WatchdogTimeout = 200 * time.Millisecond
	t.Cleanup(func() { p1604WatchdogTimeout = origWatchdog })

	ignored := newDeadlineIgnoringConn(client)

	const id = "test-zero-lock-wait-watchdog"
	a := NewP1604Adapter()
	driver := &p1604Driver{
		profile:      core.PressureProfile{ID: id},
		conn:         ignored,
		frameReader:  sharedproto.NewFrameReader(ignored),
		acquiring:    false, // 非采集状态，走 zeroCalibrationDirect 路径
		readLoopDone: make(chan struct{}),
		idleStopCh:   make(chan struct{}),
		idleLoopDone: make(chan struct{}),
	}
	close(driver.readLoopDone)
	go func() {
		<-driver.idleStopCh
		close(driver.idleLoopDone)
	}()

	shard := a.shard(id)
	shard.mu.Lock()
	shard.drivers[id] = driver
	shard.status[id] = &core.DeviceState{
		Profile:    driver.profile,
		Status:     core.StatusConnected,
		StatusText: core.StatusConnected.String(),
	}
	shard.mu.Unlock()

	// server 不读取任何数据，确保 client.Write 阻塞
	go func() {
		select {}
	}()

	// 前序 goroutine：获取 operationMu 后调用 sendCommand（Write 永久阻塞）
	// sendCommand 内部仅 SetWriteDeadline，deadlineIgnoringConn 让其失效，Write 永久阻塞。
	priorStarted := make(chan struct{})
	priorExited := make(chan struct{})
	go func() {
		defer close(priorExited)
		driver.operationMu.Lock()
		defer driver.operationMu.Unlock()
		close(priorStarted)
		// sendCommand 会永久阻塞（Write 阻塞 + SetWriteDeadline 失效）
		// ZeroCalibration 的外层 watchdog close conn 后，Write 返回错误，goroutine 退出
		_ = driver.sendCommand("h")
	}()

	// 等待前序 goroutine 持锁
	select {
	case <-priorStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("prior goroutine did not acquire operationMu within 1s")
	}

	// ZeroCalibration 等待 operationMu，外层 watchdog 在 200ms 后触发
	done := make(chan error, 1)
	go func() {
		done <- a.ZeroCalibration(id)
	}()

	budget := p1604WatchdogTimeout + 2*time.Second
	var err error
	timedOut := false

	select {
	case err = <-done:
	case <-time.After(budget):
		timedOut = true
		_ = ignored.Close()
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("ZeroCalibration did not exit after conn close")
		}
	}

	if timedOut {
		t.Fatalf("ZeroCalibration hung > %v — lock-wait watchdog missing (R1-2)", budget)
	}

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}

	// 等待前序 goroutine 退出
	select {
	case <-priorExited:
	case <-time.After(2 * time.Second):
		t.Fatal("prior goroutine did not exit within 2s after watchdog triggered")
	}

	// 验证 driver 已被 handleConnectionLost 清理
	shard.mu.RLock()
	_, driverExists := shard.drivers[id]
	st, statusExists := shard.status[id]
	shard.mu.RUnlock()
	if driverExists {
		t.Error("driver should be deleted by handleConnectionLost after lock-wait watchdog triggered")
	}
	if !statusExists {
		t.Fatal("status should still exist (set to Error) for frontend visibility")
	}
	if st.Status != core.StatusError {
		t.Errorf("status should be Error, got %v", st.Status)
	}

	// 验证 conn 已被 Close
	select {
	case <-ignored.closed:
		// 预期：conn 已被 Close
	default:
		t.Error("conn should be closed after lock-wait watchdog triggered")
	}
}
