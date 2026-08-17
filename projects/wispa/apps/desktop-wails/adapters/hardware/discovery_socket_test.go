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

// TestPacketDiscoverySocketReceiveWatchdogClosesConn 验证 Receive 的兜底:
// ReadFrom 无视 deadline 永久阻塞时,生产 watchdog timer 到期必须 Close conn 使其返回。
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

// TestPacketDiscoverySocketLoopbackKeepsSocketAlive 反向测试:真实 loopback 收发
// 正常完成时 watchdog 不得误杀健康 socket,第二轮收发必须仍然成功。
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
