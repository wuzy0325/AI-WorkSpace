package hardware

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"wispa/core"
)

const (
	p1604DiscoveryCmd      = "psi9000"
	p1604DiscoverySendPort = 7000
	p1604DiscoveryRecvPort = 7001
	p1604ScanTimeout       = 3 * time.Second
	p1604InterfaceTimeout  = 500 * time.Millisecond
	p1604LimitedBroadcast  = "255.255.255.255"
	p1604ScanResultPrefix  = "scan-WISPA"
)

// P1604Scanner WISPA 设备扫描器
type P1604Scanner struct {
	timeout time.Duration
	scanMu  sync.Mutex
	// openSocket 抽象 socket 创建，便于测试注入 mock socket 验证 ADR-009 finding 3
	// （Send 失败后不进入 Receive 循环）。生产路径调用包级 openDiscoverySocket。
	openSocket func(localPort int) (discoverySocket, error)
}

// NewP1604Scanner 创建 P1604 设备扫描器
func NewP1604Scanner() *P1604Scanner {
	return &P1604Scanner{
		timeout:    p1604ScanTimeout,
		openSocket: openDiscoverySocket,
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
	socket, err := s.openSocket(p1604DiscoveryRecvPort)
	if err != nil {
		return nil, fmt.Errorf("udp listen on %d: %w", p1604DiscoveryRecvPort, err)
	}
	defer socket.Close()

	// 向所有网段广播地址发送发现命令
	cmd := []byte(p1604DiscoveryCmd)
	for _, t := range targets {
		if net.ParseIP(t).To4() == nil {
			continue
		}
		if err := socket.Send(cmd, t, p1604DiscoverySendPort); err != nil {
			// ADR-009 finding 6：Send 失败说明 socket handle 已被 watchdog Closesocket 销毁，
			// 后续 Send/Receive 不可复用（契约：超时后 socket 不可复用）。
			// 复核修订 finding 3 修复：直接返回空结果，不进入 Receive 循环——
			// 旧实现 break 后仍调用 readScanResponses(socket, ...)，会继续调用同一 socket 的
			// Receive，但 socket handle 可能已被 watchdog 销毁，行为不可预期。
			// 调用方 defer socket.Close() 释放资源。
			return nil, nil
		}
	}

	return readScanResponses(socket, s.timeout), nil
}

func readScanResponses(socket discoverySocket, timeout time.Duration) []core.ScanResult {
	results := make([]core.ScanResult, 0)
	seen := make(map[string]bool)
	buf := make([]byte, 1024)
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		n, remote, err := socket.Receive(buf, remaining)
		if err != nil {
			break
		}
		result := parseP1604Response(buf[:n], remote)
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
		Name:            "Discovered WISPA",
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
