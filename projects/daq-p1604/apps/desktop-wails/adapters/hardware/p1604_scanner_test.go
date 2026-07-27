package hardware

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestBroadcastTargetsWithTimeoutFallsBack(t *testing.T) {
	release := make(chan struct{})
	started := time.Now()
	targets := broadcastTargetsWithTimeout(20*time.Millisecond, func() []string {
		<-release
		return []string{"192.168.1.255"}
	})
	close(release)

	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("broadcast target fallback took too long: %v", elapsed)
	}
	if len(targets) != 1 || targets[0] != p1604LimitedBroadcast {
		t.Fatalf("expected limited broadcast fallback, got %v", targets)
	}
}

func TestReadScanResponsesClosesSocketWhenDeadlineDoesNotWakeRead(t *testing.T) {
	conn := newBlockingPacketConn()
	started := time.Now()
	results := readScanResponses(conn, 20*time.Millisecond)

	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("scan read timeout took too long: %v", elapsed)
	}
	if !conn.wasClosed() {
		t.Fatal("scan timeout must close the UDP socket")
	}
	if len(results) != 0 {
		t.Fatalf("expected no scan results, got %v", results)
	}
}

type blockingPacketConn struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingPacketConn() *blockingPacketConn {
	return &blockingPacketConn{closed: make(chan struct{})}
}

func (c *blockingPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.closed
	return 0, nil, net.ErrClosed
}

func (c *blockingPacketConn) WriteTo(p []byte, _ net.Addr) (int, error) { return len(p), nil }
func (c *blockingPacketConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}
func (c *blockingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *blockingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingPacketConn) SetWriteDeadline(time.Time) error { return nil }
func (c *blockingPacketConn) wasClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}
