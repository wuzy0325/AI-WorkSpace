package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

const (
	t1603DiscoveryCommand    = "T1603"
	t1603DiscoveryPort       = 7000
	t1603DiscoveryListenPort = 7001
	t1603DeviceTCPPort       = 9000
	t1603DiscoveredName      = "Discovered DAQ-T-1603"
	limitedBroadcastTarget   = "255.255.255.255"
)

// DaqT1603UDPScanner DAQ-T-1603 UDP 广播扫描器
type DaqT1603UDPScanner struct{}

// NewDaqT1603UDPScanner 创建 DAQ-T-1603 UDP 扫描器
func NewDaqT1603UDPScanner() *DaqT1603UDPScanner {
	return &DaqT1603UDPScanner{}
}

// Scan 执行 UDP 广播扫描
func (s *DaqT1603UDPScanner) Scan(ctx context.Context, timeout time.Duration) ([]ports.DiscoveredDevice, error) {
	targets := getBroadcastTargets()
	if len(targets) == 0 {
		slog.Warn("No broadcast targets found, falling back to limited broadcast")
		targets = []BroadcastTarget{{LocalAddress: "0.0.0.0", BroadcastIP: limitedBroadcastTarget}}
	}

	slog.Info("Scanning DAQ-T-1603 via UDP broadcast", "targets", len(targets), "timeout", timeout)

	// 构建广播地址列表（包含 limited broadcast 和 directed broadcast）
	broadcastSet := make(map[string]bool)
	var broadcastAddrs []*net.UDPAddr

	for _, t := range targets {
		// 添加 directed broadcast
		if !broadcastSet[t.BroadcastIP] {
			broadcastSet[t.BroadcastIP] = true
			addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", t.BroadcastIP, t1603DiscoveryPort))
			if err == nil {
				broadcastAddrs = append(broadcastAddrs, addr)
			}
		}
	}

	// 添加 limited broadcast
	if !broadcastSet[limitedBroadcastTarget] {
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", limitedBroadcastTarget, t1603DiscoveryPort))
		if err == nil {
			broadcastAddrs = append(broadcastAddrs, addr)
		}
	}

	if len(broadcastAddrs) == 0 {
		return nil, fmt.Errorf("no valid broadcast addresses")
	}

	// 绑定监听端口
	listenAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", t1603DiscoveryListenPort))
	if err != nil {
		return nil, fmt.Errorf("resolve listen address: %w", err)
	}

	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}
	defer conn.Close()

	// 发送广播
	command := []byte(t1603DiscoveryCommand)
	for _, addr := range broadcastAddrs {
		if _, err := conn.WriteToUDP(command, addr); err != nil {
			slog.Warn("Failed to send broadcast", "addr", addr, "err", err)
		}
	}

	// 接收响应
	var mu sync.Mutex
	devices := make(map[string]ports.DiscoveredDevice)

	deadline := time.Now().Add(timeout)
	buf := make([]byte, 2048)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			break
		default:
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		conn.SetReadDeadline(time.Now().Add(min(remaining, 500*time.Millisecond)))

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		if n == 0 || remoteAddr == nil {
			continue
		}

		response := strings.TrimSpace(string(buf[:n]))
		fallbackAddr := remoteAddr.IP.String()

		device := parseT1603Response(response, fallbackAddr)
		if device == nil {
			continue
		}

		mu.Lock()
		devices[ports.DeviceFingerprint(*device)] = *device
		mu.Unlock()
	}

	var result []ports.DiscoveredDevice
	mu.Lock()
	for _, d := range devices {
		result = append(result, d)
	}
	mu.Unlock()

	slog.Info("DAQ-T-1603 UDP scan complete", "found", len(result))
	return result, nil
}

// parseT1603Response 解析 DAQ-T-1603 的 UDP 响应
// 支持两种格式：JSON 和 CSV
func parseT1603Response(response, fallbackAddress string) *ports.DiscoveredDevice {
	// 先尝试 JSON 格式
	if device := parseT1603JSON(response, fallbackAddress); device != nil {
		return device
	}

	// 回退到 CSV 格式
	return parseT1603CSV(response, fallbackAddress)
}

// parseT1603JSON 解析 JSON 格式的 T1603 响应
func parseT1603JSON(response, fallbackAddress string) *ports.DiscoveredDevice {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		return nil
	}

	address := fallbackAddress
	if ip, ok := data["ip"].(string); ok && ip != "" {
		address = ip
	}

	port := t1603DeviceTCPPort
	if p, ok := data["port"].(float64); ok && p > 0 {
		port = int(p)
	}

	device := &ports.DiscoveredDevice{
		ID:   fmt.Sprintf("scan-daq-t-1603-%s-%d", address, port),
		IP:   address,
		Port: port,
		Type: "DAQ-T-1603",
		Name: t1603DiscoveredName,
	}

	if v, ok := data["mac"].(string); ok && v != "" {
		device.MACAddress = v
	}
	if v, ok := data["serialNumber"].(string); ok && v != "" {
		device.SerialNumber = v
	}
	if v, ok := data["firmwareVersion"].(string); ok && v != "" {
		device.FirmwareVersion = v
	}
	if v, ok := data["model"].(string); ok && v != "" {
		device.Model = v
	}
	if v, ok := data["subnetMask"].(string); ok && v != "" {
		device.SubnetMask = v
	}
	if v, ok := data["gateway"].(string); ok && v != "" {
		device.Gateway = v
	}
	if v, ok := data["ipMode"].(string); ok {
		if v == "dhcp" || v == "static" {
			device.IPMode = v
		}
	}
	if v, ok := data["tcpConnected"].(bool); ok {
		device.TCPConnected = v
	}
	if v, ok := data["ipAssigned"].(bool); ok {
		device.IPAssigned = v
	}
	device.RawResponse = response

	return device
}

// parseT1603CSV 解析 CSV 格式的 T1603 响应
// 格式1（8+字段）：IP,MAC,?,SerialNumber,Model,FirmwareVersion,TCPConnected,IPAssigned,Port,SubnetMask,...
// 格式2（2+字段）：Model,IP,Port,MAC,...
func parseT1603CSV(response, fallbackAddress string) *ports.DiscoveredDevice {
	parts := strings.Split(response, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) >= 8 {
		address := parts[0]
		if address == "" {
			address = fallbackAddress
		}

		port := t1603DeviceTCPPort
		if len(parts) > 7 {
			if p, err := strconv.Atoi(parts[7]); err == nil && p > 0 {
				port = p
			}
		}

		device := &ports.DiscoveredDevice{
			ID:   fmt.Sprintf("scan-daq-t-1603-%s-%d", address, port),
			IP:   address,
			Port: port,
			Type: "DAQ-T-1603",
			Name: t1603DiscoveredName,
		}

		if len(parts) > 1 && parts[1] != "" {
			device.MACAddress = parts[1]
		}
		if len(parts) > 2 && parts[2] != "" && parts[2] != "0" {
			device.SerialNumber = parts[2]
		}
		if len(parts) > 3 && parts[3] != "" {
			device.Model = parts[3]
		}
		if len(parts) > 4 && parts[4] != "" {
			device.FirmwareVersion = parts[4]
		}
		if len(parts) > 5 {
			device.TCPConnected = parts[5] == "1"
		}
		if len(parts) > 6 {
			device.IPAssigned = parts[6] == "1"
		}
		if len(parts) > 8 && parts[8] != "" {
			device.SubnetMask = parts[8]
		}
		device.RawResponse = response
		return device
	}

	if len(parts) >= 2 {
		address := parts[1]
		if address == "" {
			address = fallbackAddress
		}

		port := t1603DeviceTCPPort
		if len(parts) > 2 {
			if p, err := strconv.Atoi(parts[2]); err == nil && p > 0 {
				port = p
			}
		}

		device := &ports.DiscoveredDevice{
			ID:   fmt.Sprintf("scan-daq-t-1603-%s-%d", address, port),
			IP:   address,
			Port: port,
			Type: "DAQ-T-1603",
			Name: t1603DiscoveredName,
		}

		if len(parts) > 0 && parts[0] != "" {
			device.Model = parts[0]
		}
		if len(parts) > 3 && parts[3] != "" {
			device.MACAddress = parts[3]
		}
		device.RawResponse = response
		return device
	}

	return nil
}
