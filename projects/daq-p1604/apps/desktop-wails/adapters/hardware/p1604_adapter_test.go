package hardware

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"daq-p1604/core"
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

func (m *mockNonTCPConn) Read(b []byte) (n int, err error)    { return 0, nil }
func (m *mockNonTCPConn) Write(b []byte) (n int, err error)   { return len(b), nil }
func (m *mockNonTCPConn) Close() error                         { return nil }
func (m *mockNonTCPConn) LocalAddr() net.Addr                  { return nil }
func (m *mockNonTCPConn) RemoteAddr() net.Addr                 { return nil }
func (m *mockNonTCPConn) SetDeadline(t time.Time) error        { return nil }
func (m *mockNonTCPConn) SetReadDeadline(t time.Time) error    { return nil }
func (m *mockNonTCPConn) SetWriteDeadline(t time.Time) error   { return nil }

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
	mu      sync.Mutex
	states  []struct {
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
