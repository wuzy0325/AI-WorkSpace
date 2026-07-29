package hardware

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"daq-t1603/core"
)

const scanResultIDPrefix = "scan-daq-t-1603"

const (
	t1603DiscoveryCmd     = "T1603"
	t1603DiscoveryPort    = 7000
	t1603DefaultPort      = 9000
	t1603ScanTimeout      = 3 * time.Second
	t1603LimitedBroadcast = "255.255.255.255"
	// t1603InterfaceTimeout 限制网卡枚举的最长时间。
	// 部分异常虚拟网卡会让 Windows 的 net.Interfaces() 长期阻塞，
	// 若不限制，会在创建 UDP socket 之前就卡住，导致扫描永久不返回。
	t1603InterfaceTimeout = 500 * time.Millisecond
)

type listenPacketFn func(network, address string) (net.PacketConn, error)

type T1603Scanner struct {
	timeout      time.Duration
	listenPacket listenPacketFn
	scanMu       sync.Mutex
}

func NewT1603Scanner() *T1603Scanner {
	return &T1603Scanner{
		timeout: t1603ScanTimeout,
	}
}

func (s *T1603Scanner) Scan() ([]core.ScanResult, error) {
	// 防止并发扫描：重复触发扫描会竞争 UDP socket 并导致结果混乱。
	if !s.scanMu.TryLock() {
		return nil, fmt.Errorf("device scan already in progress")
	}
	defer s.scanMu.Unlock()

	// 网卡枚举有界化：若超过 t1603InterfaceTimeout 仍未完成，
	// 回退到有限广播地址 255.255.255.255，避免在绑定 socket 前永久卡住。
	targets := broadcastTargetsWithTimeout(t1603InterfaceTimeout, broadcastTargets)

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
		return nil, fmt.Errorf("udp listen: %w", err)
	}
	defer socket.Close()

	cmd := []byte(t1603DiscoveryCmd)
	for _, t := range targets {
		if net.ParseIP(t).To4() == nil {
			continue
		}
		_ = socket.Send(cmd, t, t1603DiscoveryPort)
	}

	return readT1603Responses(socket, s.timeout), nil
}

func readT1603Responses(socket discoverySocket, timeout time.Duration) []core.ScanResult {
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

		host, _, splitErr := net.SplitHostPort(remote)
		if splitErr != nil {
			host = remote
		}

		result := parseResponse(buf[:n], host)
		if result == nil {
			continue
		}

		if !seen[result.ID] {
			seen[result.ID] = true
			results = append(results, *result)
		}
	}

	return results
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
		return []string{t1603LimitedBroadcast}
	}
}

func parseResponse(data []byte, remoteHost string) *core.ScanResult {
	msg := strings.TrimSpace(string(data))

	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return parseJSON(jsonData, remoteHost)
	}

	parts := strings.Split(msg, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) >= 8 {
		return parseCSV(parts, remoteHost)
	}

	if msg == "DAQT1603" || strings.HasPrefix(msg, "DAQT1603") {
		return &core.ScanResult{
			ID:      fmt.Sprintf("scan-daq-t-1603-%s", remoteHost),
			Name:    "Discovered DAQ-T-1603",
			Address: remoteHost,
			Port:    t1603DefaultPort,
		}
	}

	return nil
}

func parseJSON(data map[string]interface{}, remoteHost string) *core.ScanResult {
	result := &core.ScanResult{
		Name:    "Discovered DAQ-T-1603",
		Address: remoteHost,
		Port:    t1603DefaultPort,
	}

	if ip, ok := data["ip"].(string); ok && ip != "" {
		result.Address = ip
	}
	if p, ok := data["port"].(float64); ok && p > 0 {
		result.Port = int(p)
	}
	if mac, ok := data["mac"].(string); ok {
		result.MacAddress = mac
	}
	if sn, ok := data["serialNumber"].(string); ok {
		result.SerialNumber = sn
	}
	if fv, ok := data["firmwareVersion"].(string); ok {
		result.FirmwareVersion = fv
	}

	result.ID = scanResultID(result.Address, result.Port, result.MacAddress)
	return result
}

func parseCSV(parts []string, remoteHost string) *core.ScanResult {
	address := parts[0]
	if address == "" {
		address = remoteHost
	}
	port := t1603DefaultPort
	if p, err := strconv.Atoi(strings.TrimSpace(parts[7])); err == nil && p > 0 {
		port = p
	}

	result := &core.ScanResult{
		Name:    "Discovered DAQ-T-1603",
		Address: address,
		Port:    port,
	}

	if len(parts) > 1 && parts[1] != "" {
		result.MacAddress = parts[1]
	}
	if len(parts) > 2 && parts[2] != "" && parts[2] != "0" {
		result.SerialNumber = parts[2]
	}
	if len(parts) > 4 && parts[4] != "" {
		result.FirmwareVersion = parts[4]
	}

	result.ID = scanResultID(result.Address, result.Port, result.MacAddress)
	return result
}

func scanResultID(address string, port int, macAddress string) string {
	if macAddress != "" {
		return fmt.Sprintf("%s-%s", scanResultIDPrefix, macAddress)
	}
	return fmt.Sprintf("%s-%s-%d", scanResultIDPrefix, address, port)
}

func broadcastTargets() []string {
	targets := make(map[string]bool)
	targets[t1603LimitedBroadcast] = true

	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{t1603LimitedBroadcast}
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
