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
	results := readScanResponses(&packetDiscoverySocket{conn: conn}, 20*time.Millisecond)

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

// failingSendDiscoverySocket 是 mock discoverySocket，Send 总是返回错误，
// Receive 调用次数被计数用于断言"Send 失败后不调用 Receive"（复核修订 finding 3）。
type failingSendDiscoverySocket struct {
	receiveCount   int32
	receiveCountMu sync.Mutex
}

func (s *failingSendDiscoverySocket) Send(_ []byte, _ string, _ int) error {
	return net.ErrClosed
}

func (s *failingSendDiscoverySocket) Receive(_ []byte, _ time.Duration) (int, string, error) {
	s.receiveCountMu.Lock()
	s.receiveCount++
	s.receiveCountMu.Unlock()
	return 0, "", net.ErrClosed
}

func (s *failingSendDiscoverySocket) Close() error { return nil }

func (s *failingSendDiscoverySocket) ReceiveCallCount() int32 {
	s.receiveCountMu.Lock()
	defer s.receiveCountMu.Unlock()
	return s.receiveCount
}

// TestP1604Scanner_SendFailureSkipsReceive 验证 ADR-009 复核修订 finding 3：
// P1604Scanner.Scan 在 Send 失败后必须直接返回 nil, nil，不调用同一 socket 的 Receive。
//
// 测试前置：
//   - P1604Scanner 配置 2s timeout（足够长，若进入 Receive 循环会让测试超时）
//   - openSocket 注入 failingSendDiscoverySocket：Send 总返回 net.ErrClosed
//   - Receive 计数器初始 0
//
// 测试步骤：
//   - 调用 scanner.Scan()
//
// 期待结果：
//   - 返回 nil 错误
//   - 返回空结果（nil 或 len==0）
//   - 耗时应远小于 2s timeout（不进入 Receive 循环）
//   - Receive 调用次数 == 0（Send 失败后不调用 Receive）
//
// 修复前：Send 失败 break 后仍执行 readScanResponses(socket, s.timeout)，Receive 被调用 1 次，
//
//	测试断言 ReceiveCallCount == 0 失败。
//
// 修复后：Send 失败直接 return nil, nil，Receive 从未被调用，测试通过。
func TestP1604Scanner_SendFailureSkipsReceive(t *testing.T) {
	failingSocket := &failingSendDiscoverySocket{}
	scanner := &P1604Scanner{
		timeout: 2 * time.Second,
		openSocket: func(int) (discoverySocket, error) {
			return failingSocket, nil
		},
	}

	started := time.Now()
	results, err := scanner.Scan()
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("Scan should return nil error on Send failure, got: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("Scan should return empty results on Send failure, got %d items", len(results))
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("Scan should return immediately on Send failure (不进入 Receive 循环), took %v", elapsed)
	}
	if count := failingSocket.ReceiveCallCount(); count != 0 {
		t.Fatalf("Receive should NOT be called after Send failure, got %d calls", count)
	}
}
