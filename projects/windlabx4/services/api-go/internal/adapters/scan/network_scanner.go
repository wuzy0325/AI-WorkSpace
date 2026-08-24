package scan

import (
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"windlabx4/services/api-go/internal/core/device"
)

// 9116 等 P1604 变体不响应 UDP 广播发现（端口 7000 静默），但仍监听 TCP 9000
// 并支持 w1601 握手 + q00 读型号。下面这组常量用于 TCP 探测发现这类设备。
const (
	daqP1604TcpProbePort  = 9000
	daqP1604TcpHandshake  = "w1601"
	daqP1604TcpModelCmd   = "q00"
	tcpProbeDialTimeout   = 700 * time.Millisecond
	tcpProbeReadTimeout   = 700 * time.Millisecond
	maxTcpProbePayloadLen = 256
)

// p1604ModelRe 匹配 P1604 家族型号（如 9116 / 1604 等 3-4 位纯数字型号）。
var p1604ModelRe = regexp.MustCompile(`^\d{3,4}$`)

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
	timeout         time.Duration
	listenPacket    listenPacketFn
	targetsOverride []string // 非空时覆盖默认的全网卡广播目标集合（用于 -iface/-subnet 限定范围）
	scanMu          sync.Mutex
}

type NetworkScannerOption func(*NetworkScanner)

func WithTimeout(timeout time.Duration) NetworkScannerOption {
	return func(s *NetworkScanner) { s.timeout = timeout }
}

// WithTargets 显式指定发现包发送目标（IPv4 地址字符串列表），覆盖默认的全网卡
// 广播目标集合。仅用于需要限定扫描范围的场景（如指定网卡或子网）。
// 传入空切片等同于不覆盖，仍使用默认目标。
func WithTargets(targets ...string) NetworkScannerOption {
	return func(s *NetworkScanner) {
		if len(targets) > 0 {
			s.targetsOverride = targets
		}
	}
}

func NewNetworkScanner(opts ...NetworkScannerOption) *NetworkScanner {
	s := &NetworkScanner{
		timeout: defaultScanTimeout,
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
		{cmd: daqT1603DiscoveryCmd, port: daqT1603DiscoveryPort, parser: parseDaqT1603Response},
		{cmd: daqP1604DiscoveryCmd, port: daqP1604DiscoveryPort, parser: parseDaqP1604Response},
		{cmd: "\xFF\x01\x01\x02", port: daqP1064PreDiscoveryPort, parser: parseDaqP1604PreResponse},
	}

	seen := make(map[string]bool)
	addrSeen := make(map[string]bool)
	var results []device.ScanResult

	for _, task := range tasks {
		devices := s.scanWithSocket(task.cmd, task.port, task.parser)
		for _, d := range devices {
			if !seen[d.ID] && !addrSeen[d.Address] {
				seen[d.ID] = true
				addrSeen[d.Address] = true
				results = append(results, d)
			}
		}
	}

	// TCP 探测：发现不响应 UDP 广播发现的 P1604 变体（如型号 9116）。
	// 这些设备仍监听 TCP 9000，可通过 w1601 握手 + q00 读型号后识别。
	// 测试模式下（listenPacket 被 mock 注入）跳过，避免对真实网络发起探测。
	if s.listenPacket == nil {
		var tcpCandidates []string
		if len(s.targetsOverride) > 0 {
			tcpCandidates = s.targetsOverride
		} else {
			tcpCandidates = unicastDiscoveryTargets()
		}
		for _, d := range s.scanWithTCPProbe(tcpCandidates) {
			if !seen[d.ID] && !addrSeen[d.Address] {
				seen[d.ID] = true
				addrSeen[d.Address] = true
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
	var socket discoverySocket
	var err error
	if s.listenPacket != nil {
		var conn net.PacketConn
		conn, err = s.listenPacket("udp4", ":0")
		if err == nil {
			socket = &packetDiscoverySocket{conn: conn}
		}
	} else {
		socket, err = openDiscoverySocket(0)
	}
	if err != nil {
		log.Printf("[scan] 创建 socket 失败 cmd=%q: %v", cmd, err)
		return nil
	}
	defer socket.Close()

	// 只使用广播地址，避免单播重复命令导致设备不响应。
	// 网卡枚举有界化：若超过 interfaceTimeout 仍未完成，回退到有限广播地址，
	// 避免在绑定 socket 前因异常虚拟网卡导致扫描永久卡住。
	// 若调用方通过 WithTargets 显式指定了目标（如 -iface/-subnet 限定范围），
	// 则直接使用该目标集合，跳过网卡枚举。
	var targets []string
	if len(s.targetsOverride) > 0 {
		targets = s.targetsOverride
	} else {
		targets = broadcastTargetsWithTimeout(interfaceTimeout, getAllBroadcastTargets)
	}
	for _, target := range targets {
		addr := net.ParseIP(target)
		if addr == nil {
			continue
		}
		if err := socket.Send([]byte(cmd), addr.String(), port); err != nil {
			// ADR-009 finding 6：Send 失败说明 socket handle 已被 watchdog Closesocket 销毁，
			// 后续 Send/Receive 不可复用（契约：超时后 socket 不可复用）。
			// 复核修订 finding 3 修复：直接返回空结果，不进入 Receive 循环——
			// 旧实现 break 后仍执行下方 Receive 循环，会继续调用同一 socket 的 Receive，
			// 但 socket handle 可能已被 watchdog 销毁，行为不可预期。
			// 调用方 defer socket.Close() 释放资源。
			log.Printf("[scan] 发送命令 %q 到 %s:%d 失败, 终止本轮扫描并跳过 Receive: %v", cmd, target, port, err)
			return nil
		}
	}

	seen := make(map[string]bool)
	var devices []device.ScanResult
	buf := make([]byte, 1024)
	deadline := time.Now().Add(s.timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		n, remote, err := socket.Receive(buf, remaining)
		if err != nil {
			break
		}
		raw := string(buf[:n])
		log.Printf("[scan] socket %q 收到响应 from=%s len=%d data=%q", cmd, remote, n, raw)
		result := parser(buf[:n], remote)
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

// unicastDiscoveryTargets 返回默认全网卡扫描时用于 TCP 探测的单播候选地址。
// 复用 commonDiscoveryOctets 在每个非回环 IPv4 网卡的同网段生成候选（如 .7/.9/.../.254），
// 与默认 UDP 扫描的候选末位一致，但不包含广播地址（TCP 探测只针对单播主机）。
func unicastDiscoveryTargets() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	targets := make(map[string]bool)
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
				targets[net.IPv4(base[0], base[1], base[2], byte(o)).String()] = true
			}
		}
	}
	return setToSlice(targets)
}

// scanWithTCPProbe 并发探测候选单播地址上的 TCP 9000 端口，识别不响应 UDP 广播的
// P1604 家族设备（如型号 9116）。对每个候选：建立 TCP 连接 → 发送 w1601 握手启用
// 长度前缀 → 发送 q00 读取型号；型号符合 P1604 模式则记为 DAQ-P-1604。
func (s *NetworkScanner) scanWithTCPProbe(candidates []string) []device.ScanResult {
	var mu sync.Mutex
	var devices []device.ScanResult
	var wg sync.WaitGroup
	for _, ip := range candidates {
		if net.ParseIP(ip) == nil {
			continue
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if res := probeP1604TCP(ip); res != nil {
				mu.Lock()
				devices = append(devices, *res)
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return devices
}

// probeP1604TCP 对单台主机做 P1604 TCP 探测，命中返回 ScanResult，否则返回 nil。
func probeP1604TCP(ip string) *device.ScanResult {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, fmt.Sprintf("%d", daqP1604TcpProbePort)), tcpProbeDialTimeout)
	if err != nil {
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(tcpProbeReadTimeout))

	// 握手启用长度前缀模式；即使握手 ACK 未读也不影响后续读型号。
	if _, err := conn.Write([]byte(daqP1604TcpHandshake)); err != nil {
		return nil
	}
	_, _ = readLengthPrefixed(conn)

	if _, err := conn.Write([]byte(daqP1604TcpModelCmd)); err != nil {
		return nil
	}
	model, err := readLengthPrefixed(conn)
	if err != nil {
		return nil
	}
	model = strings.TrimSpace(model)
	if !p1604ModelRe.MatchString(model) {
		return nil
	}
	return &device.ScanResult{
		ID:        scanResultID(scanDaqP1604Prefix, ip, daqP1604TcpProbePort, ""),
		Name:      "Discovered DAQ-P-1604",
		Type:      device.DeviceDAQP1604,
		Available: true,
		Address:   ip,
		Port:      daqP1604TcpProbePort,
		Model:     model,
	}
}

// readLengthPrefixed 读取 P1604 长度前缀帧：2 字节大端长度（含长度字段本身）+ 载荷。
// 例：型号响应 [0x00 0x07 ' ' '9' '1' '1' '6']，长度 7 = 2 字节头 + 5 字节载荷 " 9116"。
func readLengthPrefixed(conn net.Conn) (string, error) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return "", err
	}
	n := int(hdr[0])<<8 | int(hdr[1])
	if n < 2 || n > maxTcpProbePayloadLen {
		return "", fmt.Errorf("invalid tcp probe frame length %d", n)
	}
	buf := make([]byte, n-2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// ScopedDiscoveryTargets 计算限定范围内的发现包发送目标（IPv4 地址字符串列表）。
//
// 两种限定方式（二选一，均返回供 WithTargets 使用的目标集合）：
//   - ifaceName 非空：仅针对该网卡，复用 commonDiscoveryOctets 生成单播发现候选
//     （如 .7/.9/.../.254），与默认全网卡扫描逻辑一致；找不到网卡时返回错误。
//   - subnetCIDR 非空：解析 CIDR，对网络地址的每个 commonDiscoveryOctets 末位生成
//     单播候选（如 192.168.1.0/24 → 192.168.1.7, .9, ...），用于只扫指定子网。
//
// 两者都为空时返回 nil（表示调用方应使用默认全网卡广播目标）。
// 仅生成单播发现候选，不加入受限广播地址 255.255.255.255——限定范围场景下用户
// 已明确希望缩小发送面，避免向无关网段打广播。
func ScopedDiscoveryTargets(ifaceName, subnetCIDR string) ([]string, error) {
	switch {
	case subnetCIDR != "":
		return subnetDiscoveryTargets(subnetCIDR)
	case ifaceName != "":
		return ifaceDiscoveryTargets(ifaceName)
	default:
		return nil, nil
	}
}

func ifaceDiscoveryTargets(name string) ([]string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("找不到网卡 %q: %w", name, err)
	}
	if iface.Flags&net.FlagUp == 0 {
		return nil, fmt.Errorf("网卡 %q 未启用", name)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("读取网卡 %q 地址失败: %w", name, err)
	}
	targets := make(map[string]bool)
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
			continue
		}
		base := ipNet.IP.To4()
		for _, o := range commonDiscoveryOctets {
			targets[net.IPv4(base[0], base[1], base[2], byte(o)).String()] = true
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("网卡 %q 没有可用的 IPv4 地址", name)
	}
	return setToSlice(targets), nil
}

func subnetDiscoveryTargets(cidr string) ([]string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("无效的 CIDR %q: %w", cidr, err)
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("仅支持 IPv4 子网, %q 不是 IPv4", cidr)
	}
	base := ipNet.IP.To4()
	targets := make(map[string]bool)
	for _, o := range commonDiscoveryOctets {
		targets[net.IPv4(base[0], base[1], base[2], byte(o)).String()] = true
	}
	return setToSlice(targets), nil
}

func setToSlice(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for t := range m {
		result = append(result, t)
	}
	return result
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
