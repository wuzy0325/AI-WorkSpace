package scan

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

const (
	daqP1604DiscoveryCmd  = "psi9000"
	daqP1604DiscoveryPort = 7000
	daqP1604DefaultPort   = 9000

	daqT1603DiscoveryCmd  = "T1603"
	daqT1603DiscoveryPort = 7000
	daqT1603DefaultPort   = 9000

	daqP1064PreDiscoveryPort = 1901
	daqP1064PreDefaultPort   = 23 // 1604Pre 默认 TCP 端口（参考 Cursor DAQ 实测值，旧值 9001 不正确）

	defaultScanTimeout = 3 * time.Second
	limitedBroadcast   = "255.255.255.255"
	// interfaceTimeout 限制网卡枚举的最长时间。
	// 部分异常虚拟网卡会让 Windows 的 net.Interfaces() 长期阻塞，
	// 若不限制，会在创建 UDP socket 之前就卡住，导致扫描永久不返回。
	interfaceTimeout = 500 * time.Millisecond
)

const (
	scanDaqP1604Prefix    = "scan-daq-p-1604"
	scanDaqT1603Prefix    = "scan-daq-t-1603"
	scanDaqP1604PrePrefix = "scan-daq-p-1604pre"
)

var commonDiscoveryOctets = []int{7, 9, 101, 102, 104, 200, 202, 254}

func scanResultID(prefix, address string, port int, mac string) string {
	if mac != "" {
		return fmt.Sprintf("%s-%s", prefix, mac)
	}
	return fmt.Sprintf("%s-%s-%d", prefix, address, port)
}

type listenPacketFn func(network, address string) (net.PacketConn, error)

type NetworkScanner struct {
	timeout      time.Duration
	listenPacket listenPacketFn
	scanMu       sync.Mutex
}

type NetworkScannerOption func(*NetworkScanner)

func WithTimeout(timeout time.Duration) NetworkScannerOption {
	return func(s *NetworkScanner) { s.timeout = timeout }
}

func NewNetworkScanner(opts ...NetworkScannerOption) *NetworkScanner {
	s := &NetworkScanner{
		timeout:      defaultScanTimeout,
		listenPacket: net.ListenPacket,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *NetworkScanner) Scan() ([]device.ScanResult, error) {
	// 防止并发扫描：重复触发扫描会竞争 UDP socket 并导致结果混乱。
	if !s.scanMu.TryLock() {
		log.Printf("[scan] 扫描已在进行中，拒绝并发请求")
		return nil, fmt.Errorf("device scan already in progress")
	}
	defer s.scanMu.Unlock()

	log.Printf("[scan] 开始设备扫描, timeout=%v", s.timeout)

	type scanTask struct {
		cmd    string
		port   int
		parser func([]byte, string) *device.ScanResult
	}

	// 顺序执行每种设备类型的扫描，避免 Windows 上并发 UDP socket 的冲突
	tasks := []scanTask{
		{cmd: daqT1603DiscoveryCmd, port: daqT1603DiscoveryPort, parser: deviceDispatcher},
		{cmd: daqP1604DiscoveryCmd, port: daqP1604DiscoveryPort, parser: deviceDispatcher},
		{cmd: "\xFF\x01\x01\x02", port: daqP1064PreDiscoveryPort, parser: parseDaqP1604PreResponse},
	}

	seen := make(map[string]bool)
	var results []device.ScanResult

	for _, task := range tasks {
		devices := s.scanWithSocket(task.cmd, task.port, task.parser)
		for _, d := range devices {
			if !seen[d.ID] {
				seen[d.ID] = true
				results = append(results, d)
			}
		}
	}

	if results == nil {
		results = []device.ScanResult{}
	}
	log.Printf("[scan] 扫描完成, 共发现 %d 个设备", len(results))
	return results, nil
}

// scanWithSocket 为单个设备类型创建独立的 UDP socket，发送发现命令并收集响应。
// 只发送广播地址，避免向同一设备重复发送单播命令导致设备不响应。
func (s *NetworkScanner) scanWithSocket(
	cmd string,
	port int,
	parser func([]byte, string) *device.ScanResult,
) []device.ScanResult {
	conn, err := s.listenPacket("udp4", ":0")
	if err != nil {
		log.Printf("[scan] 创建 socket 失败 cmd=%q: %v", cmd, err)
		return nil
	}
	defer conn.Close()

	// 只使用广播地址，避免单播重复命令导致设备不响应。
	// 网卡枚举有界化：若超过 interfaceTimeout 仍未完成，回退到有限广播地址，
	// 避免在绑定 socket 前因异常虚拟网卡导致扫描永久卡住。
	targets := broadcastTargetsWithTimeout(interfaceTimeout, getAllBroadcastTargets)
	for _, target := range targets {
		addr := net.ParseIP(target)
		if addr == nil {
			continue
		}
		dest := &net.UDPAddr{IP: addr, Port: port}
		if _, err := conn.WriteTo([]byte(cmd), dest); err != nil {
			log.Printf("[scan] 发送命令 %q 到 %s:%d 失败: %v", cmd, target, port, err)
		}
	}

	if err := conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil
	}
	// 部分Windows网络驱动不会在deadline到期时唤醒ReadFrom，
	// 用定时器在超时后强制Close作为兜底，确保接收循环一定退出。
	watchdog := time.AfterFunc(s.timeout, func() { _ = conn.Close() })
	defer watchdog.Stop()

	seen := make(map[string]bool)
	var devices []device.ScanResult
	buf := make([]byte, 1024)
	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			break
		}
		raw := string(buf[:n])
		log.Printf("[scan] socket %q 收到响应 from=%s len=%d data=%q", cmd, remote.String(), n, raw)
		result := parser(buf[:n], remote.String())
		if result != nil {
			log.Printf("[scan] socket %q 解析成功 id=%s type=%s", cmd, result.ID, result.Type)
			if !seen[result.ID] {
				seen[result.ID] = true
				devices = append(devices, *result)
			}
		} else {
			log.Printf("[scan] socket %q 解析失败", cmd)
		}
	}
	return devices
}

// broadcastTargetsWithTimeout 为网卡枚举设置硬性时间上限。
// 超时未返回时回退到 limitedBroadcast，保证扫描流程不会因网卡枚举卡死。
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
		return []string{limitedBroadcast}
	}
}

func getAllDiscoveryTargets() []string {
	targets := getAllBroadcastTargets()

	unicastIPs := make(map[string]bool)
	interfaces, err := net.Interfaces()
	if err != nil {
		return targets
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
			base := ipNet.IP.To4()
			for _, o := range commonDiscoveryOctets {
				candidate := net.IPv4(base[0], base[1], base[2], byte(o)).String()
				if !unicastIPs[candidate] {
					unicastIPs[candidate] = true
				}
			}
		}
	}

	for ip := range unicastIPs {
		targets = append(targets, ip)
	}
	return targets
}

func getAllBroadcastTargets() []string {
	targets := make(map[string]bool)
	targets[limitedBroadcast] = true

	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{limitedBroadcast}
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
			broadcast := computeBroadcastAddress(ipNet.IP.To4(), ipNet.Mask)
			if broadcast != "" {
				targets[broadcast] = true
			}
		}
	}

	result := make([]string, 0, len(targets))
	for t := range targets {
		result = append(result, t)
	}
	return result
}

func computeBroadcastAddress(ip net.IP, mask net.IPMask) string {
	if len(ip) != 4 || len(mask) != 4 {
		return ""
	}
	broadcast := make(net.IP, 4)
	for i := range ip {
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast.String()
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func safeGet(parts []string, index int) string {
	if index < len(parts) {
		return strings.TrimSpace(parts[index])
	}
	return ""
}

func omitZero(s string) string {
	if s == "" || s == "0" {
		return ""
	}
	return s
}

func getJSONString(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func init() {
	log.SetFlags(log.Ltime | log.Lshortfile)
}
