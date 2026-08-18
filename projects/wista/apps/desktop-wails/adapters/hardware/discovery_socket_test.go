package hardware

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"
)

// fullyBlockingPacketConn 模拟现场故障机(ADR-009 第 6 条要求的 double):
// 忽略所有 deadline,ReadFrom/WriteTo 只在 Close 后返回。
//
// 与 wispa discovery_socket_test.go 中的同名替身保持完全一致行为,
// 便于跨项目 grep 同款验收模式。
type fullyBlockingPacketConn struct {
	closed chan struct{}
	once   sync.Once
}

func newFullyBlockingPacketConn() *fullyBlockingPacketConn {
	return &fullyBlockingPacketConn{closed: make(chan struct{})}
}

func (c *fullyBlockingPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.closed
	return 0, nil, net.ErrClosed
}

func (c *fullyBlockingPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	<-c.closed
	return 0, net.ErrClosed
}

func (c *fullyBlockingPacketConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *fullyBlockingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *fullyBlockingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *fullyBlockingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fullyBlockingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func (c *fullyBlockingPacketConn) wasClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// TestPacketDiscoverySocketReceiveWatchdogClosesConn 验证 Receive 的兜底(ADR-009 R0-8):
// ReadFrom 无视 deadline 永久阻塞时,生产 watchdog timer 到期必须 Close conn 使其返回。
//
// 验收依据 ADR-009 第 6 条:"硬件网络测试必须包含一个忽略所有 deadline、仅在 Close 后
// 解除 Read / ReadFrom 的测试连接。"
func TestPacketDiscoverySocketReceiveWatchdogClosesConn(t *testing.T) {
	conn := newFullyBlockingPacketConn()
	socket := &packetDiscoverySocket{conn: conn}

	done := make(chan error, 1)
	go func() {
		_, _, err := socket.Receive(make([]byte, 1024), 50*time.Millisecond)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after watchdog close, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Receive 未在 watchdog 预算内返回——deadline 失效时无硬兜底")
	}
	if !conn.wasClosed() {
		t.Fatal("watchdog 必须 Close 阻塞的 conn")
	}
}

// TestPacketDiscoverySocketSendWatchdogClosesConn 验证 Send 的兜底(ADR-009 R0-9):
// WriteTo 阻塞时,独立 watchdog 必须在 discoverySendTimeout 预算内 Close conn。
//
// 验收依据 ADR-009 第 5 条:"UDP 扫描保留 deadline,同时使用
// time.AfterFunc(timeout, conn.Close) 作为硬兜底。"
// 同时依据 ADR-009 第 49 行:"一次性 UDP scanner 还必须覆盖 Send 阶段;
// 若 watchdog 在 Send 后、Receive 前才建立,阻塞的 WriteTo / Sendto 仍无硬边界。"
func TestPacketDiscoverySocketSendWatchdogClosesConn(t *testing.T) {
	conn := newFullyBlockingPacketConn()
	socket := &packetDiscoverySocket{conn: conn}

	started := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- socket.Send([]byte("probe"), "192.0.2.1", 7000)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after watchdog close, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Send 未在 discoverySendTimeout 预算内返回——WriteTo 阻塞无独立 Close owner")
	}
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("Send 超出预算: %v", elapsed)
	}
	if !conn.wasClosed() {
		t.Fatal("watchdog 必须 Close 阻塞的 conn")
	}
}

// TestPacketDiscoverySocketLoopbackKeepsSocketAlive 反向测试(ADR-009 第 8 条):
// 真实 loopback 收发正常完成时 watchdog 不得误杀健康 socket,第二轮收发必须仍然成功。
//
// 验收依据 ADR-009 第 8 条:"可选 ACK、空缓冲探测、quiet-window 和 drain 等
// '无数据也正常'的操作,不得通过阻塞 Read + watchdog Close 判断结果。"
// 同时依据验收标准第 3 条:"不误杀健康连接"。
func TestPacketDiscoverySocketLoopbackKeepsSocketAlive(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	socket := &packetDiscoverySocket{conn: conn}
	defer func() { _ = socket.Close() }()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	payload := []byte("loopback-probe")
	for round := 0; round < 2; round++ {
		if err := socket.Send(payload, "127.0.0.1", port); err != nil {
			t.Fatalf("round %d send: %v", round, err)
		}
		buf := make([]byte, 1024)
		n, _, err := socket.Receive(buf, time.Second)
		if err != nil {
			t.Fatalf("round %d receive: %v", round, err)
		}
		if !bytes.Equal(buf[:n], payload) {
			t.Fatalf("round %d payload mismatch: %q", round, buf[:n])
		}
	}
}
