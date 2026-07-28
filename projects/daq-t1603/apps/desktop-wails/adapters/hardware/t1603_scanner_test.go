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
