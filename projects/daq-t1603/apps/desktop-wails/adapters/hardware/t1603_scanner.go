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
)

type listenPacketFn func(network, address string) (net.PacketConn, error)

type T1603Scanner struct {
	timeout      time.Duration
	listenPacket listenPacketFn
}

func NewT1603Scanner() *T1603Scanner {
	return &T1603Scanner{
		timeout:      t1603ScanTimeout,
		listenPacket: net.ListenPacket,
	}
}

func (s *T1603Scanner) Scan() ([]core.ScanResult, error) {
	conn, err := s.listenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("udp listen: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	// Some Windows network drivers do not wake ReadFrom when a deadline expires.
	watchdog := time.AfterFunc(s.timeout, func() { _ = conn.Close() })
	defer watchdog.Stop()

	cmd := []byte(t1603DiscoveryCmd)
	targets := broadcastTargets()
	for _, t := range targets {
		addr := &net.UDPAddr{IP: net.ParseIP(t), Port: t1603DiscoveryPort}
		if addr.IP == nil {
			continue
		}
		conn.WriteTo(cmd, addr)
	}

	var mu sync.Mutex
	var results []core.ScanResult
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

		host, _, splitErr := net.SplitHostPort(remote.String())
		if splitErr != nil {
			host = remote.String()
		}

		result := parseResponse(buf[:n], host)
		if result == nil {
			continue
		}

		mu.Lock()
		if !seen[result.ID] {
			seen[result.ID] = true
			results = append(results, *result)
		}
		mu.Unlock()
	}

	if results == nil {
		results = []core.ScanResult{}
	}
	return results, nil
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
