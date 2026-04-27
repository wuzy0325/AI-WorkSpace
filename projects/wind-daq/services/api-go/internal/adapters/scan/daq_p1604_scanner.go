package scan

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

const (
	p1604DiscoveryCommand    = "psi9000"
	p1604DiscoveryPort       = 7000
	p1604DiscoveryListenPort = 7001
	p1604DiscoveredName      = "Discovered DAQ-P-1604"
)

// DaqP1604UDPScanner DAQ-P-1604 UDP 广播扫描器
type DaqP1604UDPScanner struct{}

// NewDaqP1604UDPScanner 创建 DAQ-P-1604 UDP 扫描器
func NewDaqP1604UDPScanner() *DaqP1604UDPScanner {
	return &DaqP1604UDPScanner{}
}

// Scan 执行 UDP 广播扫描
func (s *DaqP1604UDPScanner) Scan(ctx context.Context, timeout time.Duration) ([]ports.DiscoveredDevice, error) {
	targets := getBroadcastTargets()
	if len(targets) == 0 {
		slog.Warn("No broadcast targets found, falling back to limited broadcast")
		targets = []BroadcastTarget{{LocalAddress: "0.0.0.0", BroadcastIP: "255.255.255.255"}}
	}

	slog.Info("Scanning DAQ-P-1604 via UDP broadcast", "targets", len(targets), "timeout", timeout)

	// 解析广播地址
	broadcastAddrs := make([]*net.UDPAddr, 0, len(targets))
	for _, t := range targets {
		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", t.BroadcastIP, p1604DiscoveryPort))
		if err != nil {
			slog.Warn("Invalid broadcast address", "addr", t.BroadcastIP, "err", err)
			continue
		}
		broadcastAddrs = append(broadcastAddrs, addr)
	}

	if len(broadcastAddrs) == 0 {
		return nil, fmt.Errorf("no valid broadcast addresses")
	}

	// 绑定监听端口
	listenAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", p1604DiscoveryListenPort))
	if err != nil {
		return nil, fmt.Errorf("resolve listen address: %w", err)
	}

	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}
	defer conn.Close()

	// 发送广播
	command := []byte(p1604DiscoveryCommand)
	for _, addr := range broadcastAddrs {
		if _, err := conn.WriteToUDP(command, addr); err != nil {
			slog.Warn("Failed to send broadcast", "addr", addr, "err", err)
		}
	}

	// 接收响应
	var mu sync.Mutex
	devices := make(map[string]ports.DiscoveredDevice)

	deadline := time.Now().Add(timeout)
	buf := make([]byte, 1024)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			break
		default:
		}

		// 设置读取截止时间
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		conn.SetReadDeadline(time.Now().Add(min(remaining, 500*time.Millisecond)))

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		if n == 0 {
			continue
		}

		response := strings.TrimSpace(string(buf[:n]))
		device := parseP1604Response(response)
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

	slog.Info("DAQ-P-1604 UDP scan complete", "found", len(result))
	return result, nil
}

// parseP1604Response 解析 DAQ-P-1604 的 UDP 响应
// 格式：IP,MAC,?,SerialNumber,FirmwareVersion,?,?,?,Port,SubnetMask,Gateway,...
func parseP1604Response(response string) *ports.DiscoveredDevice {
	parts := strings.Split(response, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) < 8 {
		return nil
	}

	address := parts[0]
	port := 0
	if len(parts) > 7 {
		fmt.Sscanf(parts[7], "%d", &port)
	}

	if address == "" || port <= 0 {
		return nil
	}

	device := &ports.DiscoveredDevice{
		ID:   fmt.Sprintf("scan-daq-p-1604-%s-%d", address, port),
		IP:   address,
		Port: port,
		Type: "DAQ-P-1604",
		Name: p1604DiscoveredName,
	}

	if len(parts) > 1 && parts[1] != "" {
		device.MACAddress = parts[1]
	}
	if len(parts) > 3 && parts[3] != "" {
		device.SerialNumber = parts[3]
	}
	if len(parts) > 4 && parts[4] != "" {
		device.FirmwareVersion = parts[4]
	}
	if len(parts) > 8 && parts[8] != "" {
		device.SubnetMask = parts[8]
	}
	if len(parts) > 9 && parts[9] != "" {
		device.Gateway = parts[9]
	}
	device.RawResponse = response

	return device
}
