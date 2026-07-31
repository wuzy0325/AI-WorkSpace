package hardware

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

type deadlineIgnoringPacketConn struct {
	closed    chan struct{}
	closeOnce sync.Once
}

func newDeadlineIgnoringPacketConn() *deadlineIgnoringPacketConn {
	return &deadlineIgnoringPacketConn{closed: make(chan struct{})}
}

func (c *deadlineIgnoringPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.closed
	return 0, nil, net.ErrClosed
}

func (c *deadlineIgnoringPacketConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	return len(b), nil
}

func (c *deadlineIgnoringPacketConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *deadlineIgnoringPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *deadlineIgnoringPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *deadlineIgnoringPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *deadlineIgnoringPacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestT1603ScannerClosesConnWhenDeadlineDoesNotUnblockRead(t *testing.T) {
	conn := newDeadlineIgnoringPacketConn()
	scanner := &T1603Scanner{
		timeout: 20 * time.Millisecond,
		listenPacket: func(string, string) (net.PacketConn, error) {
			return conn, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := scanner.Scan()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		_ = conn.Close()
		t.Fatal("Scan remained blocked after its deadline")
	}
}

// TestT1603BroadcastTargetsWithTimeoutFallsBack 验证网卡枚举超时后回退到有限广播地址。
// 模拟 net.Interfaces() 长期阻塞的场景（如异常虚拟网卡），
// 确保扫描流程不会在创建 UDP socket 之前永久卡住。
func TestT1603BroadcastTargetsWithTimeoutFallsBack(t *testing.T) {
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
	if len(targets) != 1 || targets[0] != t1603LimitedBroadcast {
		t.Fatalf("expected limited broadcast fallback, got %v", targets)
	}
}

// TestT1603ScannerRejectsConcurrentScan 验证并发扫描被拒绝。
// 防止重复触发扫描时 UDP socket 竞争和结果混乱。
func TestT1603ScannerRejectsConcurrentScan(t *testing.T) {
	conn := newDeadlineIgnoringPacketConn()
	scanner := &T1603Scanner{
		timeout: 100 * time.Millisecond,
		listenPacket: func(string, string) (net.PacketConn, error) {
			return conn, nil
		},
	}

	if !scanner.scanMu.TryLock() {
		t.Fatal("first TryLock must succeed")
	}
	// 持有锁的情况下再次扫描应被拒绝
	_, err := scanner.Scan()
	if err == nil {
		t.Fatal("expected error when scan already in progress")
	}
	scanner.scanMu.Unlock()
}

// failingWritePacketConn 是 mock net.PacketConn，WriteTo 总是返回错误（即 Send 失败），
// ReadFrom 计数用于断言"Send 失败后不调用 Receive"（复核修订 finding 3）。
type failingWritePacketConn struct {
	receiveCount int32
	receiveMu    sync.Mutex
}

func (c *failingWritePacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	c.receiveMu.Lock()
	c.receiveCount++
	c.receiveMu.Unlock()
	return 0, nil, net.ErrClosed
}

func (c *failingWritePacketConn) WriteTo(_ []byte, _ net.Addr) (int, error) {
	return 0, net.ErrClosed
}

func (c *failingWritePacketConn) Close() error                     { return nil }
func (c *failingWritePacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *failingWritePacketConn) SetDeadline(time.Time) error      { return nil }
func (c *failingWritePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *failingWritePacketConn) SetWriteDeadline(time.Time) error { return nil }

func (c *failingWritePacketConn) ReceiveCallCount() int32 {
	c.receiveMu.Lock()
	defer c.receiveMu.Unlock()
	return c.receiveCount
}

// TestT1603Scanner_SendFailureSkipsReceive 验证 ADR-009 复核修订 finding 3：
// T1603Scanner.Scan 在 Send 失败后必须直接返回 nil，不调用同一 socket 的 Receive。
//
// 测试前置：
//   - T1603Scanner 配置 2s timeout（足够长，若进入 Receive 循环会让测试超时）
//   - listenPacket 注入 failingWritePacketConn：WriteTo 总返回 net.ErrClosed（即 Send 失败）
//   - ReadFrom 计数器初始 0
//
// 测试步骤：
//   - 调用 scanner.Scan()
//
// 期待结果：
//   - 返回 nil 错误
//   - 返回空结果（nil 或 len==0）
//   - 耗时应远小于 2s timeout（不进入 Receive 循环）
//   - ReadFrom 调用次数 == 0（Send 失败后不调用 Receive）
//
// 修复前：Send 失败 break 后仍调用 readT1603Responses(socket, ...)，ReadFrom 被调用 1 次，
//
//	测试断言 ReceiveCallCount == 0 失败。
//
// 修复后：Send 失败直接 return nil, nil，ReadFrom 从未被调用，测试通过。
func TestT1603Scanner_SendFailureSkipsReceive(t *testing.T) {
	failingConn := &failingWritePacketConn{}
	scanner := &T1603Scanner{
		timeout: 2 * time.Second,
		listenPacket: func(string, string) (net.PacketConn, error) {
			return failingConn, nil
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
	if count := failingConn.ReceiveCallCount(); count != 0 {
		t.Fatalf("Receive should NOT be called after Send failure, got %d calls", count)
	}
}

func TestParseResponse_JSON(t *testing.T) {
	tests := []struct {
		name       string
		json       map[string]interface{}
		remoteHost string
		wantAddr   string
		wantPort   int
		wantID     string
	}{
		{
			name:       "full JSON with all fields",
			json:       map[string]interface{}{"ip": "10.0.0.5", "port": float64(9000), "mac": "aa:bb:cc:dd:ee:ff", "serialNumber": "SN001", "firmwareVersion": "v2.0"},
			remoteHost: "10.0.0.5",
			wantAddr:   "10.0.0.5",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-aa:bb:cc:dd:ee:ff",
		},
		{
			name:       "JSON without serial,firmware",
			json:       map[string]interface{}{"ip": "192.168.1.7", "port": float64(9000), "mac": "11:22:33:44:55:66"},
			remoteHost: "192.168.1.7",
			wantAddr:   "192.168.1.7",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-11:22:33:44:55:66",
		},
		{
			name:       "JSON missing ip falls back to remoteHost host",
			json:       map[string]interface{}{"mac": "aa:bb:cc:dd:ee:01"},
			remoteHost: "10.0.0.99",
			wantAddr:   "10.0.0.99",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-aa:bb:cc:dd:ee:01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.json)
			got := parseResponse(data, tt.remoteHost)
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.Address != tt.wantAddr {
				t.Errorf("address = %q, want %q", got.Address, tt.wantAddr)
			}
			if got.Port != tt.wantPort {
				t.Errorf("port = %d, want %d", got.Port, tt.wantPort)
			}
			if got.ID != tt.wantID {
				t.Errorf("id = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestParseResponse_CSV(t *testing.T) {
	tests := []struct {
		name       string
		csv        string
		remoteHost string
		wantAddr   string
		wantPort   int
		wantMac    string
		wantSN     string
	}{
		{
			name:       "full CSV with all fields",
			csv:        "192.168.1.7, aa:bb:cc:dd:ee:ff, SN001, 0, v2.0, 0, 0, 9000",
			remoteHost: "192.168.1.7",
			wantAddr:   "192.168.1.7",
			wantPort:   9000,
			wantMac:    "aa:bb:cc:dd:ee:ff",
			wantSN:     "SN001",
		},
		{
			name:       "CSV with empty address falls back to remoteHost",
			csv:        ", aa:bb:cc:dd:ee:01, SN002, 0, v1.5, 0, 0, 9000",
			remoteHost: "10.0.0.50",
			wantAddr:   "10.0.0.50",
			wantPort:   9000,
			wantMac:    "aa:bb:cc:dd:ee:01",
			wantSN:     "SN002",
		},
		{
			name:       "CSV with serialNumber 0 should be omitted",
			csv:        "10.0.0.1, mac:01, 0, 0, , 0, 0, 9000",
			remoteHost: "10.0.0.1",
			wantAddr:   "10.0.0.1",
			wantPort:   9000,
			wantMac:    "mac:01",
			wantSN:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResponse([]byte(tt.csv), tt.remoteHost)
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.Address != tt.wantAddr {
				t.Errorf("address = %q, want %q", got.Address, tt.wantAddr)
			}
			if got.Port != tt.wantPort {
				t.Errorf("port = %d, want %d", got.Port, tt.wantPort)
			}
			if got.MacAddress != tt.wantMac {
				t.Errorf("macAddress = %q, want %q", got.MacAddress, tt.wantMac)
			}
			if got.SerialNumber != tt.wantSN {
				t.Errorf("serialNumber = %q, want %q", got.SerialNumber, tt.wantSN)
			}
		})
	}
}

func TestParseResponse_ShortResponse(t *testing.T) {
	got := parseResponse([]byte("DAQT1603"), "192.168.1.99")
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Address != "192.168.1.99" {
		t.Errorf("address = %q, want %q", got.Address, "192.168.1.99")
	}
	if got.Port != t1603DefaultPort {
		t.Errorf("port = %d, want %d", got.Port, t1603DefaultPort)
	}
}

func TestParseResponse_Garbage(t *testing.T) {
	got := parseResponse([]byte("garbage data"), "10.0.0.1")
	if got != nil {
		t.Error("expected nil for unrecognized response")
	}
}

func TestBroadcastAddr(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		mask net.IPMask
		want string
	}{
		{"class C", net.IPv4(192, 168, 1, 10).To4(), net.IPv4Mask(255, 255, 255, 0), "192.168.1.255"},
		{"class B", net.IPv4(10, 0, 0, 5).To4(), net.IPv4Mask(255, 255, 0, 0), "10.0.255.255"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := broadcastAddr(tt.ip, tt.mask)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBroadcastAddr_Invalid(t *testing.T) {
	if got := broadcastAddr(net.IP{1, 2, 3}, nil); got != "" {
		t.Errorf("expected empty for short IP, got %q", got)
	}
}
