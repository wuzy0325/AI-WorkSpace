package hardware

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"daq-p1604/core"
)

const (
	p1604DiscoveryCmd      = "psi9000"
	p1604DiscoverySendPort = 7000
	p1604DiscoveryRecvPort = 7001
	p1604ScanTimeout       = 3 * time.Second
	p1604InterfaceTimeout  = 500 * time.Millisecond
	p1604LimitedBroadcast  = "255.255.255.255"
	p1604ScanResultPrefix  = "scan-daq-p-1604"
)

// P1604Scanner DAQ-P-1604 设备扫描器
type P1604Scanner struct {
	timeout time.Duration
	scanMu  sync.Mutex
}

// NewP1604Scanner 创建 P1604 设备扫描器
func NewP1604Scanner() *P1604Scanner {
	return &P1604Scanner{
		timeout: p1604ScanTimeout,
	}
}

// Scan 扫描局域网内的 P1604 设备
func (s *P1604Scanner) Scan() ([]core.ScanResult, error) {
	if !s.scanMu.TryLock() {
		return nil, fmt.Errorf("device scan already in progress")
	}
	defer s.scanMu.Unlock()

	// 部分异常虚拟网卡会让 Windows 网卡枚举长期阻塞。先做有界枚举，避免
	// 已绑定 7001 后卡住并永久占用端口；超时则退化为有限广播地址。
	targets := broadcastTargetsWithTimeout(p1604InterfaceTimeout, broadcastTargets)

	// 在 7001 端口监听响应
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", p1604DiscoveryRecvPort))
	if err != nil {
		return nil, fmt.Errorf("udp listen on %d: %w", p1604DiscoveryRecvPort, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	// 向所有网段广播地址发送发现命令
	cmd := []byte(p1604DiscoveryCmd)
	for _, t := range targets {
		addr := &net.UDPAddr{IP: net.ParseIP(t), Port: p1604DiscoverySendPort}
		if addr.IP == nil {
			continue
		}
		conn.WriteTo(cmd, addr)
	}

	return readScanResponses(conn, s.timeout), nil
}

func readScanResponses(conn net.PacketConn, timeout time.Duration) []core.ScanResult {
	// Close 是 deadline 的兜底：部分 Windows 网络驱动不会按期唤醒 ReadFrom。
	timer := time.AfterFunc(timeout, func() { _ = conn.Close() })
	defer timer.Stop()

	results := make([]core.ScanResult, 0)
	seen := make(map[string]bool)
	buf := make([]byte, 1024)
	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		result := parseP1604Response(buf[:n], remote.String())
		if result != nil && !seen[result.ID] {
			seen[result.ID] = true
			results = append(results, *result)
		}
	}
	return results
}

// broadcastTargetsWithTimeout 为网卡枚举设置硬性时间上限。
// 超时未返回时回退到有限广播地址，保证扫描流程不会因网卡枚举卡死。
//
// 权衡：超时返回后，仍在阻塞的 enumerate goroutine 无法被取消（net.Interfaces 没有 context 版本），
// 会一直挂起直到其内部 syscall 返回。这是有意接受的泄漏——主扫描流程必然能继续推进，
// 避免因为网卡枚举阻塞导致整个扫描永久卡死。
func broadcastTargetsWithTimeout(timeout time.Duration, enumerate func() []string) []string {
	resultCh := make(chan []string, 1)
	go func() {
		resultCh <- enumerate()
	}()

	select {
	case targets := <-resultCh:
		return targets
	case <-time.After(timeout):
		return []string{p1604LimitedBroadcast}
	}
}

// parseP1604Response 解析 P1604 设备发现响应
// 响应 CSV 格式：<IP>,<MAC>,,<序列号>,<固件版本>,,<端口>,<子网掩码>,<网关>
// remoteAddr 作为 IP 为空时的备选
func parseP1604Response(data []byte, remoteAddr string) *core.ScanResult {
	msg := strings.TrimSpace(string(data))
	parts := strings.Split(msg, ",")
	if len(parts) < 9 {
		return nil
	}

	ip := strings.TrimSpace(parts[0])
	if ip == "" {
		// 响应中 IP 为空时，使用远程地址作为备选
		host, _, _ := net.SplitHostPort(remoteAddr)
		if host != "" {
			ip = host
		}
	}
	port := p1604DefaultPort
	if p, err := strconv.Atoi(strings.TrimSpace(parts[7])); err == nil && p > 0 {
		port = p
	}

	mac := strings.TrimSpace(parts[1])
	serial := strings.TrimSpace(parts[3])
	firmware := strings.TrimSpace(parts[4])

	result := &core.ScanResult{
		Name:            "Discovered DAQ-P-1604",
		Address:         ip,
		Port:            port,
		MacAddress:      mac,
		SerialNumber:    serial,
		FirmwareVersion: firmware,
	}

	if mac != "" {
		result.ID = fmt.Sprintf("%s-%s", p1604ScanResultPrefix, mac)
	} else {
		result.ID = fmt.Sprintf("%s-%s-%d", p1604ScanResultPrefix, ip, port)
	}
	return result
}

// broadcastTargets 获取所有广播目标地址
func broadcastTargets() []string {
	targets := make(map[string]bool)
	targets[p1604LimitedBroadcast] = true

	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{p1604LimitedBroadcast}
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			bcast := broadcastAddr(ipNet.IP.To4(), ipNet.Mask)
			if bcast != "" {
				targets[bcast] = true
			}
		}
	}

	result := make([]string, 0, len(targets))
	for t := range targets {
		result = append(result, t)
	}
	return result
}

// broadcastAddr 计算广播地址
func broadcastAddr(ip net.IP, mask net.IPMask) string {
	if len(ip) != 4 || len(mask) != 4 {
		return ""
	}
	broadcast := make(net.IP, 4)
	for i := range ip {
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast.String()
}
