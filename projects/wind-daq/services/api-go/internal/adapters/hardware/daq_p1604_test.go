package hardware

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

func TestRunDAQP1604HandshakeTimesOutAndClosesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	started := time.Now()
	err := runDAQP1604Handshake(client, 20*time.Millisecond, func() error {
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

func TestDAQP1604StopClosesConnWhenReadLoopDoesNotExit(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1604(device.Profile{ID: "test-stop-stuck-reader", Type: device.DeviceDAQP1604})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.acquiring = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Connection = device.ConnectionAcquiring
	d.status.Acquiring = true
	d.mu.Unlock()

	err := d.StopAcquisition()
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

	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("connection should be cleared after read-loop timeout")
	}
	if status != device.ConnectionError {
		t.Fatalf("connection status = %v, want Error", status)
	}
}

// TestDAQP1604_SyncUnitFromHardware_EOFReturnsError 验证：u01101 读到 io.EOF 时
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
func TestDAQP1604_SyncUnitFromHardware_EOFReturnsError(t *testing.T) {
	server, client := net.Pipe()

	d := NewDAQP1604(device.Profile{
		ID:   "test-sync-eof",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.mu.Unlock()

	// 服务端：读掉 u01101 → sleep → Close 触发客户端 Read 返回 io.EOF
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		time.Sleep(50 * time.Millisecond)
		_ = server.Close()
	}()

	err := d.syncUnitFromHardware(DAQ_P_1604_TIMEOUT)
	if err == nil {
		t.Fatal("syncUnitFromHardware should return error on EOF, got nil")
	}
	if !sharedproto.IsConnResetByPeer(err) {
		t.Errorf("returned error should be detected as conn reset, got: %v", err)
	}

	_ = client.Close()
}

// TestDAQP1604_SetUnit_V01101EOFTriggersOnError 验证：SetUnit 写 v01101 时
// 设备 FIN 关闭连接，应清理 driver 内部状态（conn/frameReader=nil, status=Error）
// 并触发 onError 回调通知 DeviceManager 从 map 中删除。
//
// 预期：
//   - SetUnit 返回 error
//   - d.conn == nil, d.frameReader == nil（driver 已清理）
//   - d.status.Connection == Error
//   - onError 回调被调用
func TestDAQP1604_SetUnit_V01101EOFTriggersOnError(t *testing.T) {
	server, client := net.Pipe()

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-eof",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 注册 onError 回调，记录调用次数
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 服务端：读掉 v01101 → sleep → Close 触发客户端 Read 返回 io.EOF
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		time.Sleep(50 * time.Millisecond)
		_ = server.Close()
	}()

	// 单位从 Pa → psi，触发 v01101 写入路径
	err := d.SetUnit("psi")

	// 1. SetUnit 必须返回 error
	if err == nil {
		t.Fatal("SetUnit should return error on v01101 EOF, got nil")
	}
	// 2. driver 内部状态应被清理
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Error("d.conn should be nil after v01101 EOF cleanup")
	}
	if fr != nil {
		t.Error("d.frameReader should be nil after v01101 EOF cleanup")
	}
	if status != device.ConnectionError {
		t.Errorf("status should be Error, got %v", status)
	}
	// 3. onError 回调应被调用
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked after v01101 EOF")
	}
}

// TestDAQP1604_SetUnit_V01101ErrorKeepsDriver 验证设备拒绝 v01101 时返回错误。
// 不触发 driver 清理，driver 保留在 map 中，前端可继续 Disconnect/重试。
//
// 模拟方式：服务端回复 N01 帧（设备拒绝，软错误）。
func TestDAQP1604_SetUnit_V01101ErrorKeepsDriver(t *testing.T) {
	server, client := net.Pipe()

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-soft",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 服务端：读掉 v01101 → 回复 N01 帧。
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 写一个长度前缀帧：2 字节大端长度 + payload "N01"
		_, _ = server.Write([]byte{0x00, 0x03, 'N', '0', '1'})
	}()

	err := d.SetUnit("psi")

	// 1. SetUnit 必须返回 error（设备拒绝）
	if err == nil {
		t.Fatal("SetUnit should return error on v01101 N01, got nil")
	}
	// 设备拒绝命令不代表 TCP 已断开，driver 必须保留。
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn == nil {
		t.Error("d.conn should REMAIN after soft error (N01)")
	}
	if fr == nil {
		t.Error("d.frameReader should REMAIN after soft error (N01)")
	}
	if status != device.ConnectionConnected {
		t.Errorf("status should remain Connected after soft error, got %v", status)
	}
	// 3. onError 不应被调用
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Error("onError should NOT be invoked on soft error")
	}

	_ = server.Close()
	_ = client.Close()
}
