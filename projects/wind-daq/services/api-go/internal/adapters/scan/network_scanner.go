package scan

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
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
	daqP1064PreDefaultPort   = 23

	defaultScanTimeout = 3 * time.Second
	limitedBroadcast   = "255.255.255.255"
)

const (
	scanDaqP1604Prefix    = "scan-daq-p-1604"
	scanDaqT1603Prefix    = "scan-daq-t-1603"
	scanDaqP1064PrePrefix = "scan-daq-p-1064pre"
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
		{cmd: "\xFF\x01\x01\x02", port: daqP1064PreDiscoveryPort, parser: parseDaqP1064PreResponse},
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

	// 只使用广播地址，避免单播重复命令导致设备不响应
	targets := getAllBroadcastTargets()
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

// deviceDispatcher 根据响应内容分发到对应的设备解析函数。
// remoteAddr 格式为 "host:port"，入口处统一提取纯 IP 供下游使用。
func deviceDispatcher(data []byte, remoteAddr string) *device.ScanResult {
	remoteHost := remoteHostFromAddr(remoteAddr)
	msg := strings.TrimSpace(string(data))

	// JSON 响应：根据 model/type 字段区分设备类型，默认按 T1603 处理
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return dispatchJsonResponse(jsonData, remoteHost)
	}

	// 二进制响应：P1064Pre
	if len(data) >= 36 && !isASCIIPrintable(data) {
		if result := parseDaqP1064PreResponse(data, remoteAddr); result != nil {
			return result
		}
	}

	// CSV / 短文本响应
	return parseDaqP1604Response(data, remoteAddr)
}

// dispatchJsonResponse 根据 JSON 中的 model 字段分发到对应的解析函数
func dispatchJsonResponse(jsonData map[string]interface{}, remoteHost string) *device.ScanResult {
	model := getJSONString(jsonData, "model")
	// 如果 model 包含 P1604 标识，按 P1604 处理
	if strings.Contains(strings.ToUpper(model), "P1604") {
		return parseDaqP1604Json(jsonData, remoteHost)
	}
	// 默认按 T1603 处理（保持向后兼容）
	return parseDaqT1603Json(jsonData, remoteHost)
}

func isASCIIPrintable(data []byte) bool {
	n := len(data)
	if n > 5 {
		n = 5
	}
	for _, b := range data[:n] {
		if b < 0x20 || b > 0x7E {
			return false
		}
	}
	return true
}

func parseDaqP1604Response(data []byte, remoteAddr string) *device.ScanResult {
	remoteHost := remoteHostFromAddr(remoteAddr)
	msg := strings.TrimSpace(string(data))
	parts := strings.Split(msg, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// CSV 响应：根据 model 字段（parts[3]）区分设备类型
	// P1604 响应格式：IP,MAC,0,序列号,firmware,...（parts[3] 为序列号，不含 T1603）
	// T1603 响应格式：IP,MAC,0,T1603,firmware,...（parts[3] 为 model "T1603"）
	// 注意：两者 parts[2] 都可能是 "0"，不能用 parts[2] 区分
	if len(parts) >= 6 {
		model := strings.ToUpper(safeGet(parts, 3))
		if strings.Contains(model, "T1603") {
			// model 包含 T1603，按 T1603 解析
			return parseDaqT1603Csv(parts, remoteHost)
		}
		// model 不含 T1603，按 P1604 解析（P1604 的 parts[3] 是序列号）
		return parseDaqP1604Csv(parts)
	}

	if strings.HasPrefix(msg, "DAQP1604") {
		return &device.ScanResult{
			ID:        scanResultID(scanDaqP1604Prefix, remoteHost, daqP1604DefaultPort, ""),
			Name:      "Discovered DAQ-P-1604",
			Type:      device.DeviceDAQP1604,
			Available: true,
			Address:   remoteHost,
			Port:      daqP1604DefaultPort,
		}
	}
	if strings.HasPrefix(msg, "DAQT1603") {
		return &device.ScanResult{
			ID:        scanResultID(scanDaqT1603Prefix, remoteHost, daqT1603DefaultPort, ""),
			Name:      "Discovered DAQ-T-1603",
			Type:      device.DeviceDaqT1603,
			Available: true,
			Address:   remoteHost,
			Port:      daqT1603DefaultPort,
		}
	}

	return nil
}

func parseDaqP1604Csv(parts []string) *device.ScanResult {
	if len(parts) < 6 {
		return nil
	}

	address := parts[0]
	if address == "" {
		return nil
	}
	port := daqP1604DefaultPort
	if p, err := parseInt(safeGet(parts, 7)); err == nil && p > 0 {
		port = p
	}

	mac := safeGet(parts, 1)
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqP1604Prefix, address, port, mac),
		Name:       "Discovered DAQ-P-1604",
		Type:       device.DeviceDAQP1604,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = omitZero(safeGet(parts, 3))
	result.FirmwareVersion = safeGet(parts, 4)
	result.SubnetMask = safeGet(parts, 8)
	result.Gateway = safeGet(parts, 9)

	return result
}

// parseDaqP1604Json 解析 P1604 设备的 JSON 格式响应
func parseDaqP1604Json(jsonData map[string]interface{}, remoteHost string) *device.ScanResult {
	address := remoteHost
	if ip, ok := jsonData["ip"].(string); ok && ip != "" {
		address = ip
	}
	port := daqP1604DefaultPort
	if p, ok := jsonData["port"].(float64); ok && p > 0 {
		port = int(p)
	}

	mac, _ := jsonData["mac"].(string)
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqP1604Prefix, address, port, mac),
		Name:       "Discovered DAQ-P-1604",
		Type:       device.DeviceDAQP1604,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = getJSONString(jsonData, "serialNumber")
	result.FirmwareVersion = getJSONString(jsonData, "firmwareVersion")
	result.SubnetMask = getJSONString(jsonData, "subnetMask")
	result.Gateway = getJSONString(jsonData, "gateway")

	return result
}

func parseDaqT1603Response(data []byte, remoteAddr string) *device.ScanResult {
	remoteHost := remoteHostFromAddr(remoteAddr)
	msg := strings.TrimSpace(string(data))

	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return parseDaqT1603Json(jsonData, remoteHost)
	}

	return parseDaqP1604Response(data, remoteAddr)
}

func remoteHostFromAddr(remoteAddr string) string {
	host := remoteAddr
	if splitHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = splitHost
	}
	return host
}

func parseDaqT1603Json(jsonData map[string]interface{}, remoteHost string) *device.ScanResult {
	address := remoteHost
	if ip, ok := jsonData["ip"].(string); ok && ip != "" {
		address = ip
	}
	port := daqT1603DefaultPort
	if p, ok := jsonData["port"].(float64); ok && p > 0 {
		port = int(p)
	}

	mac, _ := jsonData["mac"].(string)
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqT1603Prefix, address, port, mac),
		Name:       "Discovered DAQ-T-1603",
		Type:       device.DeviceDaqT1603,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = getJSONString(jsonData, "serialNumber")
	result.FirmwareVersion = getJSONString(jsonData, "firmwareVersion")
	result.Model = getJSONString(jsonData, "model")
	result.SubnetMask = getJSONString(jsonData, "subnetMask")
	result.Gateway = getJSONString(jsonData, "gateway")
	if mode, ok := jsonData["ipMode"].(string); ok {
		result.IpMode = mode
	}
	if tc, ok := jsonData["tcpConnected"].(bool); ok {
		result.TcpConnected = tc
	}
	if ia, ok := jsonData["ipAssigned"].(bool); ok {
		result.IpAssigned = ia
	}

	return result
}

func parseDaqT1603Csv(parts []string, remoteHost string) *device.ScanResult {
	if len(parts) < 8 {
		return nil
	}

	address := parts[0]
	if address == "" {
		address = remoteHost
	}
	port := daqT1603DefaultPort
	if p, err := parseInt(parts[7]); err == nil && p > 0 {
		port = p
	}

	mac := ""
	if len(parts) > 1 && parts[1] != "" {
		mac = parts[1]
	}
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqT1603Prefix, address, port, mac),
		Name:       "Discovered DAQ-T-1603",
		Type:       device.DeviceDaqT1603,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}

	result.SerialNumber = omitZero(safeGet(parts, 2))
	result.Model = safeGet(parts, 3)
	result.FirmwareVersion = safeGet(parts, 4)
	if tc := safeGet(parts, 5); tc == "1" {
		result.TcpConnected = true
	}
	if ia := safeGet(parts, 6); ia == "1" {
		result.IpAssigned = true
	}
	result.SubnetMask = safeGet(parts, 8)

	return result
}

func parseDaqP1064PreResponse(data []byte, remoteAddr string) *device.ScanResult {
	if len(data) < 36 {
		return nil
	}

	ip := fmt.Sprintf("%d.%d.%d.%d", data[5], data[6], data[7], data[8])
	mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		data[9], data[10], data[11], data[12], data[13], data[14])

	return &device.ScanResult{
		ID:         fmt.Sprintf("scan-daq-p-1064pre-%s-%d", ip, daqP1064PreDefaultPort),
		Name:       "Discovered DAQ-P-1064Pre",
		Type:       device.DeviceDAQP1064Pre,
		Available:  true,
		Address:    ip,
		Port:       daqP1064PreDefaultPort,
		MacAddress: mac,
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
