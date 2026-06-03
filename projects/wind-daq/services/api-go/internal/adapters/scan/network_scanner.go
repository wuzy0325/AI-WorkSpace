package scan

import (
	"encoding/json"
	"fmt"
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
	daqP1064PreDefaultPort   = 23

	defaultScanTimeout = 3 * time.Second
	limitedBroadcast   = "255.255.255.255"
)

const (
	scanDaqP1604Prefix    = "scan-daq-p-1604"
	scanDaqT1603Prefix    = "scan-daq-t-1603"
	scanDaqP1064PrePrefix = "scan-daq-p-1064pre"
)

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
	targets := getAllBroadcastTargets()

	var mu sync.Mutex
	var allResults []device.ScanResult
	var wg sync.WaitGroup

	scanners := []struct {
		name string
		fn   func([]string) ([]device.ScanResult, error)
	}{
		{"DAQ-P-1604", s.scanDaqP1604},
		{"DAQ-T-1603", s.scanDaqT1603},
		{"DAQ-P-1064Pre", s.scanDaqP1064Pre},
	}

	for _, sc := range scanners {
		wg.Add(1)
		go func(name string, fn func([]string) ([]device.ScanResult, error)) {
			defer wg.Done()
			results, err := fn(targets)
			if err != nil {
				return
			}
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
		}(sc.name, sc.fn)
	}

	wg.Wait()

	seen := make(map[string]bool)
	deduped := make([]device.ScanResult, 0, len(allResults))
	for _, r := range allResults {
		key := r.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, r)
	}

	if deduped == nil {
		deduped = []device.ScanResult{}
	}
	return deduped, nil
}

func (s *NetworkScanner) scanDaqP1604(targets []string) ([]device.ScanResult, error) {
	return s.udpScan(daqP1604DiscoveryCmd, daqP1604DiscoveryPort, targets, parseDaqP1604Response)
}

func (s *NetworkScanner) scanDaqT1603(targets []string) ([]device.ScanResult, error) {
	return s.udpScan(daqT1603DiscoveryCmd, daqT1603DiscoveryPort, targets, parseDaqT1603Response)
}

func (s *NetworkScanner) scanDaqP1064Pre(targets []string) ([]device.ScanResult, error) {
	cmd := []byte{0xFF, 0x01, 0x01, 0x02}
	return s.udpScanBytes(cmd, daqP1064PreDiscoveryPort, targets, parseDaqP1064PreResponse)
}

type responseParser func(data []byte, remoteAddr string) *device.ScanResult

func (s *NetworkScanner) udpScan(cmd string, port int, targets []string, parser responseParser) ([]device.ScanResult, error) {
	return s.udpScanBytes([]byte(cmd), port, targets, parser)
}

func (s *NetworkScanner) udpScanBytes(cmd []byte, port int, targets []string, parser responseParser) ([]device.ScanResult, error) {
	conn, err := s.listenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("udp listen: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	for _, target := range targets {
		addr := &net.UDPAddr{IP: net.ParseIP(target), Port: port}
		if addr.IP == nil {
			continue
		}
		conn.WriteTo(cmd, addr)
	}

	var results []device.ScanResult
	seen := make(map[string]bool)
	buf := make([]byte, 1024)

	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			break
		}

		result := parser(buf[:n], remote.String())
		if result != nil && !seen[result.ID] {
			seen[result.ID] = true
			results = append(results, *result)
		}
	}

	if results == nil {
		results = []device.ScanResult{}
	}
	return results, nil
}

func parseDaqP1604Response(data []byte, remoteAddr string) *device.ScanResult {
	msg := strings.TrimSpace(string(data))
	parts := strings.Split(msg, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) < 8 {
		if strings.HasPrefix(msg, "DAQP1604") {
			remoteHost := remoteHostFromAddr(remoteAddr)
			return &device.ScanResult{
				ID:        scanResultID(scanDaqP1604Prefix, remoteHost, daqP1604DefaultPort, ""),
				Name:      "Discovered DAQ-P-1604",
				Type:      device.DeviceDAQP1604,
				Available: true,
				Address:   remoteHost,
				Port:      daqP1604DefaultPort,
			}
		}
		return nil
	}

	return parseDaqP1604Csv(parts)
}

func parseDaqP1604Csv(parts []string) *device.ScanResult {
	address := parts[0]
	port := daqP1604DefaultPort
	if p, err := parseInt(parts[7]); err == nil && p > 0 {
		port = p
	}

	if address == "" {
		return nil
	}

	mac := ""
	if len(parts) > 1 && parts[1] != "" {
		mac = parts[1]
	}
	result := &device.ScanResult{
		ID:         scanResultID(scanDaqP1604Prefix, address, port, mac),
		Name:       "Discovered DAQ-P-1604",
		Type:       device.DeviceDAQP1604,
		Available:  true,
		Address:    address,
		Port:       port,
		MacAddress: mac,
	}
	if len(parts) > 3 && parts[3] != "" && parts[3] != "0" {
		result.SerialNumber = parts[3]
	}
	if len(parts) > 4 && parts[4] != "" {
		result.FirmwareVersion = parts[4]
	}

	return result
}

func parseDaqT1603Response(data []byte, remoteAddr string) *device.ScanResult {
	msg := strings.TrimSpace(string(data))

	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &jsonData); err == nil {
		return parseDaqT1603Json(jsonData, remoteAddr)
	}

	parts := strings.Split(msg, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) >= 8 {
		return parseDaqT1603Csv(parts, remoteAddr)
	}

	if strings.HasPrefix(msg, "DAQT1603") {
		remoteHost := remoteHostFromAddr(remoteAddr)
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

	if sn, ok := jsonData["serialNumber"].(string); ok {
		result.SerialNumber = sn
	}
	if fv, ok := jsonData["firmwareVersion"].(string); ok {
		result.FirmwareVersion = fv
	}

	return result
}

func parseDaqT1603Csv(parts []string, remoteHost string) *device.ScanResult {
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

	if len(parts) > 2 && parts[2] != "" && parts[2] != "0" {
		result.SerialNumber = parts[2]
	}
	if len(parts) > 4 && parts[4] != "" {
		result.FirmwareVersion = parts[4]
	}

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
