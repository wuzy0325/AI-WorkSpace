package scan

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/device"
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

func TestNetworkScannerClosesConnWhenDeadlineDoesNotUnblockRead(t *testing.T) {
	conn := newDeadlineIgnoringPacketConn()
	scanner := NewNetworkScanner(WithTimeout(20 * time.Millisecond))
	scanner.listenPacket = func(string, string) (net.PacketConn, error) {
		return conn, nil
	}

	done := make(chan []device.ScanResult, 1)
	go func() {
		done <- scanner.scanWithSocket("probe", 7000, func([]byte, string) *device.ScanResult { return nil })
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		_ = conn.Close()
		t.Fatal("scanWithSocket remained blocked after its deadline")
	}
}

// TestBroadcastTargetsWithTimeoutFallsBack 验证网卡枚举超时后回退到有限广播地址。
// 模拟 net.Interfaces() 长期阻塞的场景（如异常虚拟网卡），
// 确保扫描流程不会在创建 UDP socket 之前永久卡住。
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
	if len(targets) != 1 || targets[0] != limitedBroadcast {
		t.Fatalf("expected limited broadcast fallback, got %v", targets)
	}
}

// failingSendDiscoverySocket 是 mock discoverySocket，Send 总是返回错误，
// Receive 调用次数被计数用于断言"Send 失败后不调用 Receive"（复核修订 finding 3）。
type failingSendDiscoverySocket struct {
	sendErr        error
	receiveCount   int32
	receiveCountMu sync.Mutex
}

func (s *failingSendDiscoverySocket) Send(_ []byte, _ string, _ int) error {
	return s.sendErr
}

func (s *failingSendDiscoverySocket) Receive(buf []byte, _ time.Duration) (int, string, error) {
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

// TestScanWithSocket_SendFailureSkipsReceive 验证 ADR-009 复核修订 finding 3：
// scanWithSocket 在 Send 失败后必须直接返回，不调用同一 socket 的 Receive。
//
// 测试前置：
//
//   - NetworkScanner 配置 100ms timeout（避免超时干扰）
//
//   - listenPacket 返回 nil error（实际未使用，因为 listenPacket 路径会被 failingSendDiscoverySocket 替代）
//
//   - 注入 failingSendDiscoverySocket：Send 总是返回错误
//
//     由于 scanWithSocket 内部通过 openDiscoverySocket 或 listenPacket 创建 socket，
//     不直接支持 socket 注入。本测试通过 listenPacket 注入 packetDiscoverySocket 包装的
//     failingSendDiscoverySocket 不太合适。改为：直接验证 scanWithSocket 在 Send 失败时
//     返回 nil 且耗时极短（不进入 Receive 循环，否则会阻塞到 timeout）。
//
//     替代方案：用 deadlineIgnoringPacketConn（WriteTo 成功但 ReadFrom 永久阻塞）
//     验证 Send 成功时进入 Receive 循环。本测试用 listenPacket 注入会让 Send 成功，
//     不符合"Send 失败"场景。需要更精确的 mock。
//
//     最终方案：通过 packetDiscoverySocket 包装一个 Send 总失败的 mock conn。
//     packetDiscoverySocket.Send 调用 conn.WriteTo，所以 WriteTo 失败即 Send 失败。
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

// TestScanWithSocket_SendFailureSkipsReceive 验证 ADR-009 复核修订 finding 3：
// scanWithSocket 在 Send 失败后必须直接返回 nil，不调用同一 socket 的 Receive。
//
// 测试前置：
//   - NetworkScanner 配置 2s timeout（足够长，若进入 Receive 循环会让测试超时）
//   - listenPacket 注入 failingWritePacketConn：WriteTo 总返回 net.ErrClosed（即 Send 失败）
//   - ReadFrom 计数器初始 0
//
// 测试步骤：
//   - 调用 scanWithSocket("probe", 7000, parser)
//
// 期待结果：
//   - 返回 nil
//   - 耗时应远小于 2s timeout（不进入 Receive 循环）
//   - ReadFrom 调用次数 == 0（Send 失败后不调用 Receive）
//
// 修复前：Send 失败 break 后仍执行 Receive 循环，ReadFrom 被调用 1 次（返回 errClosed 后 break），
//
//	测试断言 ReceiveCallCount == 0 失败。
//
// 修复后：Send 失败直接 return nil，ReadFrom 从未被调用，测试通过。
func TestScanWithSocket_SendFailureSkipsReceive(t *testing.T) {
	failingConn := &failingWritePacketConn{}
	scanner := NewNetworkScanner(WithTimeout(2 * time.Second))
	scanner.listenPacket = func(string, string) (net.PacketConn, error) {
		return failingConn, nil
	}

	started := time.Now()
	devices := scanner.scanWithSocket("probe", 7000, func([]byte, string) *device.ScanResult { return nil })
	elapsed := time.Since(started)

	if devices != nil {
		t.Fatalf("scanWithSocket should return nil on Send failure, got %v", devices)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("scanWithSocket should return immediately on Send failure (不进入 Receive 循环), took %v", elapsed)
	}
	if count := failingConn.ReceiveCallCount(); count != 0 {
		t.Fatalf("Receive should NOT be called after Send failure, got %d calls", count)
	}
}

// TestNetworkScannerRejectsConcurrentScan 验证并发扫描被拒绝。
// 防止重复触发扫描时 UDP socket 竞争和结果混乱。
func TestNetworkScannerRejectsConcurrentScan(t *testing.T) {
	scanner := NewNetworkScanner(WithTimeout(100 * time.Millisecond))

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

type mockPacketConn struct {
	responses    map[string]string
	readBuf      chan []byte
	done         chan struct{}
	closeOnce    sync.Once
	readDeadline time.Time
}

func (m *mockPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	type result struct {
		n    int
		addr net.Addr
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		select {
		case data := <-m.readBuf:
			n := copy(b, data)
			resultCh <- result{n: n, addr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7000}}
		case <-m.done:
			resultCh <- result{err: &net.OpError{Op: "read", Net: "udp", Source: nil, Addr: nil, Err: &timeoutError{}}}
		}
	}()

	var timeout <-chan time.Time
	if !m.readDeadline.IsZero() {
		d := time.Until(m.readDeadline)
		if d <= 0 {
			return 0, nil, &net.OpError{Op: "read", Net: "udp", Source: nil, Addr: nil, Err: &timeoutError{}}
		}
		timeout = time.After(d)
	}

	select {
	case r := <-resultCh:
		return r.n, r.addr, r.err
	case <-timeout:
		return 0, nil, &net.OpError{Op: "read", Net: "udp", Source: nil, Addr: nil, Err: &timeoutError{}}
	}
}

func (m *mockPacketConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	cmd := string(b)
	if resp, ok := m.responses[cmd]; ok {
		select {
		case m.readBuf <- []byte(resp):
		default:
		}
	}
	return len(b), nil
}

func (m *mockPacketConn) Close() error {
	m.closeOnce.Do(func() { close(m.done) })
	return nil
}
func (m *mockPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0}
}
func (m *mockPacketConn) SetDeadline(t time.Time) error      { m.readDeadline = t; return nil }
func (m *mockPacketConn) SetReadDeadline(t time.Time) error  { m.readDeadline = t; return nil }
func (m *mockPacketConn) SetWriteDeadline(_ time.Time) error { return nil }

type timeoutError struct{}

func (*timeoutError) Error() string   { return "timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return false }

func newMockListenPacket(responses map[string]string) listenPacketFn {
	return func(network, address string) (net.PacketConn, error) {
		return &mockPacketConn{
			responses: responses,
			readBuf:   make(chan []byte, 10),
			done:      make(chan struct{}),
		}, nil
	}
}

func TestNetworkScannerReturnsNoErrorOnTimeout(t *testing.T) {
	scanner := NewNetworkScanner(WithTimeout(100 * time.Millisecond))
	results, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil results slice")
	}
}

func TestParseDaqP1604ResponseCSV(t *testing.T) {
	csv := "192.168.1.100, AA-BB-CC-DD-EE-FF, 0, SN001, v1.2.3, 1, 1, 9000, 255.255.255.0, 192.168.1.1"
	result := parseDaqP1604Response([]byte(csv), "192.168.1.100:7000")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-P-1604" {
		t.Fatalf("expected DAQ-P-1604 type, got %s", result.Type)
	}
	if result.ID != "scan-daq-p-1604-AA-BB-CC-DD-EE-FF" {
		t.Fatalf("expected ID scan-daq-p-1604-AA-BB-CC-DD-EE-FF, got %s", result.ID)
	}
	if result.Address != "192.168.1.100" {
		t.Fatalf("expected address 192.168.1.100, got %s", result.Address)
	}
	if result.Port != 9000 {
		t.Fatalf("expected port 9000, got %d", result.Port)
	}
	if result.MacAddress != "AA-BB-CC-DD-EE-FF" {
		t.Fatalf("expected MAC AA-BB-CC-DD-EE-FF, got %s", result.MacAddress)
	}
	if result.SerialNumber != "SN001" {
		t.Fatalf("expected serial SN001, got %s", result.SerialNumber)
	}
	if result.FirmwareVersion != "v1.2.3" {
		t.Fatalf("expected firmware v1.2.3, got %s", result.FirmwareVersion)
	}
}

func TestParseDaqP1604ResponseShort(t *testing.T) {
	result := parseDaqP1604Response([]byte("DAQP1604"), "192.168.1.100:7000")

	if result == nil {
		t.Fatal("expected parsed result for short DAQP1604 response")
	}
	if result.Type != "DAQ-P-1604" {
		t.Fatalf("expected DAQ-P-1604 type, got %s", result.Type)
	}
	if result.Address != "192.168.1.100" {
		t.Fatalf("expected address 192.168.1.100, got %q", result.Address)
	}
	if result.Port != daqP1604DefaultPort {
		t.Fatalf("expected port %d, got %d", daqP1604DefaultPort, result.Port)
	}
	if result.ID != "scan-daq-p-1604-192.168.1.100-9000" {
		t.Fatalf("expected ID scan-daq-p-1604-192.168.1.100-9000, got %q", result.ID)
	}
	if !result.Available {
		t.Fatal("expected available")
	}
}

func TestParseDaqT1603ResponseCSV(t *testing.T) {
	// 使用实际 T1603 设备的响应格式：parts[3] 为 "T1603"
	csv := "192.168.1.101, AA-BB-CC-DD-EE-11, 0, T1603, v2.0, 1, 1, 9000"
	result := parseDaqT1603Response([]byte(csv), "192.168.1.101:7000")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-T-1603" {
		t.Fatalf("expected DAQ-T-1603 type, got %s", result.Type)
	}
	if result.ID != "scan-daq-t-1603-AA-BB-CC-DD-EE-11" {
		t.Fatalf("expected ID scan-daq-t-1603-AA-BB-CC-DD-EE-11, got %s", result.ID)
	}
	if result.Address != "192.168.1.101" {
		t.Fatalf("expected address 192.168.1.101, got %s", result.Address)
	}
	if result.Port != 9000 {
		t.Fatalf("expected port 9000, got %d", result.Port)
	}
}

func TestParseDaqT1603ResponseJSON(t *testing.T) {
	json := `{"ip":"192.168.1.102","port":9000,"mac":"CC-DD-EE-FF-00-11","serialNumber":"SN003","firmwareVersion":"v3.0"}`
	result := parseDaqT1603Response([]byte(json), "192.168.1.102:7000")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-T-1603" {
		t.Fatalf("expected DAQ-T-1603 type, got %s", result.Type)
	}
	if result.ID != "scan-daq-t-1603-CC-DD-EE-FF-00-11" {
		t.Fatalf("expected ID scan-daq-t-1603-CC-DD-EE-FF-00-11, got %s", result.ID)
	}
	if result.Address != "192.168.1.102" {
		t.Fatalf("expected address 192.168.1.102, got %s", result.Address)
	}
	if result.MacAddress != "CC-DD-EE-FF-00-11" {
		t.Fatalf("expected MAC CC-DD-EE-FF-00-11, got %s", result.MacAddress)
	}
}

func TestParseDaqT1603ResponseShort(t *testing.T) {
	result := parseDaqT1603Response([]byte("DAQT1603"), "192.168.1.103:7000")

	if result == nil {
		t.Fatal("expected parsed result for short DAQT1603 response")
	}
	if result.Type != "DAQ-T-1603" {
		t.Fatalf("expected DAQ-T-1603 type, got %s", result.Type)
	}
	if result.Address != "192.168.1.103" {
		t.Fatalf("expected address 192.168.1.103, got %q", result.Address)
	}
	if result.Port != daqT1603DefaultPort {
		t.Fatalf("expected port %d, got %d", daqT1603DefaultPort, result.Port)
	}
	if result.ID != "scan-daq-t-1603-192.168.1.103-9000" {
		t.Fatalf("expected ID scan-daq-t-1603-192.168.1.103-9000, got %q", result.ID)
	}
}

func TestParseDaqP1604PreResponse(t *testing.T) {
	data := make([]byte, 36)
	data[5] = 192
	data[6] = 168
	data[7] = 1
	data[8] = 50
	data[9] = 0xAA
	data[10] = 0xBB
	data[11] = 0xCC
	data[12] = 0xDD
	data[13] = 0xEE
	data[14] = 0xFF

	result := parseDaqP1604PreResponse(data, "192.168.1.50:1901")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-P-1604Pre" {
		t.Fatalf("expected DAQ-P-1604Pre type, got %s", result.Type)
	}
	if result.Address != "192.168.1.50" {
		t.Fatalf("expected address 192.168.1.50, got %s", result.Address)
	}
	if result.Port != 23 {
		t.Fatalf("expected port 23, got %d", result.Port)
	}
	if result.MacAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("expected MAC AA:BB:CC:DD:EE:FF, got %s", result.MacAddress)
	}
}

func TestParseDaqP1604PreResponseTooShort(t *testing.T) {
	data := make([]byte, 10)
	result := parseDaqP1604PreResponse(data, "192.168.1.50:1901")
	if result != nil {
		t.Fatal("expected nil for short response")
	}
}

func TestNetworkScannerIgnoresUnknownResponse(t *testing.T) {
	result := parseDaqP1604Response([]byte("UNKNOWN_DEVICE"), "192.168.1.200:7000")
	if result != nil {
		t.Fatal("expected nil for unknown device")
	}
}

func TestComputeBroadcastAddress(t *testing.T) {
	ip := net.ParseIP("192.168.1.100").To4()
	mask := net.IPv4Mask(255, 255, 255, 0)
	broadcast := computeBroadcastAddress(ip, mask)
	if broadcast != "192.168.1.255" {
		t.Fatalf("expected 192.168.1.255, got %s", broadcast)
	}

	ip2 := net.ParseIP("10.0.0.1").To4()
	mask2 := net.IPv4Mask(255, 0, 0, 0)
	broadcast2 := computeBroadcastAddress(ip2, mask2)
	if broadcast2 != "10.255.255.255" {
		t.Fatalf("expected 10.255.255.255, got %s", broadcast2)
	}
}

func TestGetAllBroadcastTargets(t *testing.T) {
	targets := getAllBroadcastTargets()
	if len(targets) == 0 {
		t.Fatal("expected at least one broadcast target")
	}

	found := false
	for _, t := range targets {
		if t == limitedBroadcast {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected limited broadcast address in targets")
	}
}

func TestScanResultID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		addr   string
		port   int
		mac    string
		want   string
	}{
		{name: "with MAC", prefix: "scan-daq-p-1604", addr: "10.0.0.1", port: 9000, mac: "AA:BB:CC:DD:EE:FF", want: "scan-daq-p-1604-AA:BB:CC:DD:EE:FF"},
		{name: "without MAC", prefix: "scan-daq-t-1603", addr: "10.0.0.2", port: 9000, mac: "", want: "scan-daq-t-1603-10.0.0.2-9000"},
		{name: "empty MAC with different IP same device", prefix: "scan-daq-p-1604", addr: "192.168.1.50", port: 23, mac: "", want: "scan-daq-p-1604-192.168.1.50-23"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanResultID(tt.prefix, tt.addr, tt.port, tt.mac)
			if got != tt.want {
				t.Errorf("scanResultID(%q, %q, %d, %q) = %q, want %q", tt.prefix, tt.addr, tt.port, tt.mac, got, tt.want)
			}
		})
	}
}

func TestScanResultID_MACPriorityOverIP(t *testing.T) {
	mac := "AA:BB:CC:DD:EE:FF"
	id1 := scanResultID(scanDaqT1603Prefix, "10.0.0.1", 9000, mac)
	id2 := scanResultID(scanDaqT1603Prefix, "10.0.0.99", 9000, mac)
	if id1 != id2 {
		t.Errorf("same MAC should produce same ID regardless of IP: %q vs %q", id1, id2)
	}
}

func TestParseDaqT1603Json_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		data       map[string]interface{}
		remoteHost string
		wantAddr   string
		wantPort   int
		wantID     string
		wantMAC    string
		wantSN     string
		wantFV     string
	}{
		{
			name:       "full fields",
			data:       map[string]interface{}{"ip": "10.0.0.5", "port": float64(9000), "mac": "aa:bb:cc:dd:ee:ff", "serialNumber": "SN001", "firmwareVersion": "v2.0"},
			remoteHost: "10.0.0.5",
			wantAddr:   "10.0.0.5",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-aa:bb:cc:dd:ee:ff",
			wantMAC:    "aa:bb:cc:dd:ee:ff",
			wantSN:     "SN001",
			wantFV:     "v2.0",
		},
		{
			name:       "missing ip falls back to remoteHost",
			data:       map[string]interface{}{"mac": "aa:bb:cc:dd:ee:01"},
			remoteHost: "10.0.0.99",
			wantAddr:   "10.0.0.99",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-aa:bb:cc:dd:ee:01",
			wantMAC:    "aa:bb:cc:dd:ee:01",
		},
		{
			name:       "missing mac falls back to IP-based ID",
			data:       map[string]interface{}{"ip": "10.0.0.7", "serialNumber": "SN002"},
			remoteHost: "10.0.0.7",
			wantAddr:   "10.0.0.7",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-10.0.0.7-9000",
			wantSN:     "SN002",
		},
		{
			name:       "empty fields are omitted",
			data:       map[string]interface{}{"ip": "10.0.0.8", "mac": "", "serialNumber": ""},
			remoteHost: "10.0.0.8",
			wantAddr:   "10.0.0.8",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-10.0.0.8-9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDaqT1603Json(tt.data, tt.remoteHost)
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
			if got.MacAddress != tt.wantMAC {
				t.Errorf("macAddress = %q, want %q", got.MacAddress, tt.wantMAC)
			}
			if got.SerialNumber != tt.wantSN {
				t.Errorf("serialNumber = %q, want %q", got.SerialNumber, tt.wantSN)
			}
			if got.FirmwareVersion != tt.wantFV {
				t.Errorf("firmwareVersion = %q, want %q", got.FirmwareVersion, tt.wantFV)
			}
		})
	}
}

func TestParseDaqT1603Csv_TableDriven(t *testing.T) {
	split := func(s string) []string {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	tests := []struct {
		name       string
		parts      []string
		remoteHost string
		wantAddr   string
		wantPort   int
		wantID     string
		wantMAC    string
		wantSN     string
	}{
		{
			name:       "full CSV",
			parts:      split("192.168.1.7, aa:bb:cc:dd:ee:ff, SN001, 0, v2.0, 0, 0, 9000"),
			remoteHost: "192.168.1.7",
			wantAddr:   "192.168.1.7",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-aa:bb:cc:dd:ee:ff",
			wantMAC:    "aa:bb:cc:dd:ee:ff",
			wantSN:     "SN001",
		},
		{
			name:       "empty address falls back to remoteHost",
			parts:      split(", aa:bb:cc:dd:ee:01, SN002, 0, v1.5, 0, 0, 9000"),
			remoteHost: "10.0.0.50",
			wantAddr:   "10.0.0.50",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-aa:bb:cc:dd:ee:01",
			wantMAC:    "aa:bb:cc:dd:ee:01",
			wantSN:     "SN002",
		},
		{
			name:       "serial number 0 is omitted",
			parts:      split("10.0.0.1, mac:01, 0, 0, , 0, 0, 9000"),
			remoteHost: "10.0.0.1",
			wantAddr:   "10.0.0.1",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-mac:01",
			wantMAC:    "mac:01",
			wantSN:     "",
		},
		{
			name:       "no MAC falls back to IP-based ID",
			parts:      split("10.0.0.2, , SN003, 0, , 0, 0, 9000"),
			remoteHost: "10.0.0.2",
			wantAddr:   "10.0.0.2",
			wantPort:   9000,
			wantID:     "scan-daq-t-1603-10.0.0.2-9000",
			wantMAC:    "",
			wantSN:     "SN003",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDaqT1603Csv(tt.parts, tt.remoteHost)
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
			if got.MacAddress != tt.wantMAC {
				t.Errorf("macAddress = %q, want %q", got.MacAddress, tt.wantMAC)
			}
			if got.SerialNumber != tt.wantSN {
				t.Errorf("serialNumber = %q, want %q", got.SerialNumber, tt.wantSN)
			}
		})
	}
}

func TestParseDaqP1604Csv_TableDriven(t *testing.T) {
	split := func(s string) []string {
		parts := strings.Split(s, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	tests := []struct {
		name     string
		parts    []string
		wantAddr string
		wantPort int
		wantID   string
		wantMAC  string
		wantSN   string
	}{
		{
			name:     "full fields with MAC",
			parts:    split("192.168.1.100, AA:BB:CC:DD:EE:FF, 0, SN001, v1.0, 1, 1, 9000"),
			wantAddr: "192.168.1.100",
			wantPort: 9000,
			wantID:   "scan-daq-p-1604-AA:BB:CC:DD:EE:FF",
			wantMAC:  "AA:BB:CC:DD:EE:FF",
			wantSN:   "SN001",
		},
		{
			name:     "empty MAC falls back to IP-based ID",
			parts:    split("192.168.1.101, , 0, SN002, v1.0, 1, 1, 9000"),
			wantAddr: "192.168.1.101",
			wantPort: 9000,
			wantID:   "scan-daq-p-1604-192.168.1.101-9000",
			wantMAC:  "",
			wantSN:   "SN002",
		},
		{
			name:     "empty address returns nil",
			parts:    split(", AA:BB:CC:DD:EE:01, 0, SN003, v1.0, 1, 1, 9000"),
			wantAddr: "",
			wantPort: 0,
		},
		{
			name:     "serial number 0 is omitted",
			parts:    split("10.0.0.1, mac:01, 0, 0, , 0, 0, 9000"),
			wantAddr: "10.0.0.1",
			wantPort: 9000,
			wantID:   "scan-daq-p-1604-mac:01",
			wantMAC:  "mac:01",
			wantSN:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDaqP1604Csv(tt.parts)
			if tt.wantAddr == "" {
				if got != nil {
					t.Errorf("expected nil for empty address, got %+v", got)
				}
				return
			}
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
			if got.MacAddress != tt.wantMAC {
				t.Errorf("macAddress = %q, want %q", got.MacAddress, tt.wantMAC)
			}
			if got.SerialNumber != tt.wantSN {
				t.Errorf("serialNumber = %q, want %q", got.SerialNumber, tt.wantSN)
			}
		})
	}
}

func TestParseDaqT1603Json_ExtraFields(t *testing.T) {
	data := map[string]interface{}{
		"ip":              "10.0.0.9",
		"port":            float64(9000),
		"mac":             "aa:bb:cc:dd:ee:ff",
		"serialNumber":    "SN009",
		"firmwareVersion": "v3.1",
		"model":           "T1603-Pro",
		"subnetMask":      "255.255.255.0",
		"gateway":         "10.0.0.1",
		"ipMode":          "dhcp",
		"tcpConnected":    true,
		"ipAssigned":      true,
	}
	result := parseDaqT1603Json(data, "10.0.0.9")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Model != "T1603-Pro" {
		t.Errorf("model = %q, want %q", result.Model, "T1603-Pro")
	}
	if result.SubnetMask != "255.255.255.0" {
		t.Errorf("subnetMask = %q, want %q", result.SubnetMask, "255.255.255.0")
	}
	if result.Gateway != "10.0.0.1" {
		t.Errorf("gateway = %q, want %q", result.Gateway, "10.0.0.1")
	}
	if result.IpMode != "dhcp" {
		t.Errorf("ipMode = %q, want %q", result.IpMode, "dhcp")
	}
	if !result.TcpConnected {
		t.Error("expected TcpConnected = true")
	}
	if !result.IpAssigned {
		t.Error("expected IpAssigned = true")
	}
}

func TestParseDaqT1603Csv_ExtraFields(t *testing.T) {
	parts := strings.Split("10.0.0.10, aa:bb:cc:dd:ee:10, SN010, ModelX, v3.0, 1, 1, 9000, 255.255.0.0", ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	result := parseDaqT1603Csv(parts, "10.0.0.10")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Model != "ModelX" {
		t.Errorf("model = %q, want %q", result.Model, "ModelX")
	}
	if !result.TcpConnected {
		t.Error("expected TcpConnected = true")
	}
	if !result.IpAssigned {
		t.Error("expected IpAssigned = true")
	}
	if result.SubnetMask != "255.255.0.0" {
		t.Errorf("subnetMask = %q, want %q", result.SubnetMask, "255.255.0.0")
	}
}

func TestParseDaqP1604Csv_ExtraFields(t *testing.T) {
	parts := strings.Split("10.0.0.20, aa:bb:cc:dd:ee:20, 0, SN020, v2.0, 1, 1, 9000, 255.255.255.0, 10.0.0.1", ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	result := parseDaqP1604Csv(parts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SerialNumber != "SN020" {
		t.Errorf("serialNumber = %q, want %q", result.SerialNumber, "SN020")
	}
	if result.SubnetMask != "255.255.255.0" {
		t.Errorf("subnetMask = %q, want %q", result.SubnetMask, "255.255.255.0")
	}
	if result.Gateway != "10.0.0.1" {
		t.Errorf("gateway = %q, want %q", result.Gateway, "10.0.0.1")
	}
}

func TestGetAllDiscoveryTargets_IncludesUnicast(t *testing.T) {
	targets := getAllDiscoveryTargets()
	if len(targets) == 0 {
		t.Fatal("expected at least one discovery target")
	}
	foundLimited := false
	for _, tgt := range targets {
		if tgt == limitedBroadcast {
			foundLimited = true
			break
		}
	}
	if !foundLimited {
		t.Error("expected limited broadcast in discovery targets")
	}
}

func TestSafeGet(t *testing.T) {
	parts := []string{"a", "b", "c"}
	if got := safeGet(parts, 0); got != "a" {
		t.Errorf("safeGet(0) = %q, want %q", got, "a")
	}
	if got := safeGet(parts, 2); got != "c" {
		t.Errorf("safeGet(2) = %q, want %q", got, "c")
	}
	if got := safeGet(parts, 5); got != "" {
		t.Errorf("safeGet(5) = %q, want empty", got)
	}
}

func TestOmitZero(t *testing.T) {
	if got := omitZero(""); got != "" {
		t.Errorf("omitZero('') = %q, want empty", got)
	}
	if got := omitZero("0"); got != "" {
		t.Errorf("omitZero('0') = %q, want empty", got)
	}
	if got := omitZero("SN001"); got != "SN001" {
		t.Errorf("omitZero('SN001') = %q, want 'SN001'", got)
	}
}

func TestParseDaqT1603Response_Garbage(t *testing.T) {
	got := parseDaqT1603Response([]byte("garbage data"), "10.0.0.1:7000")
	if got != nil {
		t.Error("expected nil for unrecognized response")
	}
}

func TestNetworkScanner_WithMockListenPacket(t *testing.T) {
	mockListen := newMockListenPacket(map[string]string{
		"T1603": `{"ip":"192.168.1.10","port":9000,"mac":"AA:BB:CC:DD:EE:01","serialNumber":"SN-TEST"}`,
	})

	scanner := &NetworkScanner{
		timeout:      100 * time.Millisecond,
		listenPacket: mockListen,
	}

	results, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	found := false
	for _, r := range results {
		if r.ID == "scan-daq-t-1603-AA:BB:CC:DD:EE:01" {
			found = true
			if r.Address != "192.168.1.10" {
				t.Errorf("expected address 192.168.1.10, got %q", r.Address)
			}
		}
	}
	if !found {
		t.Errorf("expected T1603 device in scan results, got %+v", results)
	}
}

func TestNetworkScanner_MockListenPacket_TimeoutReturnsEmpty(t *testing.T) {
	mockListen := newMockListenPacket(nil)

	scanner := &NetworkScanner{
		timeout:      50 * time.Millisecond,
		listenPacket: mockListen,
	}

	results, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results on timeout, got %d", len(results))
	}
}

// TestScanNeverDetectsDaqT1602 DAQ-T-1602 是标准 Modbus TCP（端口 502）设备，
// 不响应 T1603 私有广播发现协议；扫描器不得把任何响应误识别为 DAQ-T-1602
// （spec-daq-t1602 §触点枚举：T1602 仅支持手动添加，不做自动发现）。
func TestScanNeverDetectsDaqT1602(t *testing.T) {
	payloads := [][]byte{
		// CSV 发现响应，model 为 T1602
		[]byte("192.168.3.201, AA-BB-CC-DD-EE-22, 0, T1602, v1.0, 1, 1, 502"),
		// JSON 发现响应，model 为 T1602
		[]byte(`{"ip":"192.168.3.201","port":502,"model":"T1602","mac":"AA-BB-CC-DD-EE-22"}`),
		// Modbus TCP 帧（MBAP + FC4 请求），不应被识别为任何已知设备
		{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x04, 0x00, 0x00, 0x00, 0x08},
		// 短前缀
		[]byte("DAQT1602"),
	}
	for _, payload := range payloads {
		if result := deviceDispatcher(payload, "192.168.3.201:502"); result != nil && result.Type == device.DeviceDaqT1602 {
			t.Fatalf("dispatcher misidentified payload %q as DAQ-T-1602: %+v", payload, result)
		}
	}
}
