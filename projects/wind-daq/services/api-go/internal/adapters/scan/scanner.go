package scan

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

const (
	daqP1604Port = 9000
	daqT1603Port = 9000
	scanTimeout  = 3 * time.Second
)

// ScanDAQP1604TCP 通过 TCP 连接尝试扫描 DAQ-P-1604 设备（回退方案）
func ScanDAQP1604TCP(ctx context.Context, subnet string) ([]ports.DiscoveredDevice, error) {
	return scanTCP(ctx, subnet, daqP1604Port, "DAQ-P-1604")
}

// ScanDAQT1603TCP 通过 TCP 连接尝试扫描 DAQ-T-1603 设备（回退方案）
func ScanDAQT1603TCP(ctx context.Context, subnet string) ([]ports.DiscoveredDevice, error) {
	return scanTCP(ctx, subnet, daqT1603Port, "DAQ-T-1603")
}

// scanTCP 通过 TCP 连接尝试扫描设备
func scanTCP(ctx context.Context, subnet string, port int, deviceType string) ([]ports.DiscoveredDevice, error) {
	var devices []ports.DiscoveredDevice

	ips, err := generateIPList(subnet)
	if err != nil {
		return nil, fmt.Errorf("parse subnet: %w", err)
	}

	slog.Info("Scanning devices (TCP)", "type", deviceType, "subnet", subnet, "port", port, "hosts", len(ips))

	ch := make(chan ports.DiscoveredDevice, len(ips))
	sem := make(chan struct{}, 50)

	for _, ip := range ips {
		select {
		case <-ctx.Done():
			break
		default:
		}

		sem <- struct{}{}
		go func(addr string) {
			defer func() { <-sem }()
			if tryConnect(addr, port, scanTimeout) {
				ch <- ports.DiscoveredDevice{
					ID:   fmt.Sprintf("scan-%s-%s-%d", deviceType, addr, port),
					IP:   addr,
					Port: port,
					Type: deviceType,
					Name: fmt.Sprintf("Discovered %s", deviceType),
				}
			}
		}(ip)
	}

	go func() {
		for i := 0; i < len(ips); i++ {
			<-sem
		}
		close(ch)
	}()

	for d := range ch {
		devices = append(devices, d)
	}

	slog.Info("TCP scan complete", "type", deviceType, "found", len(devices))
	return devices, nil
}

// tryConnect 尝试 TCP 连接
func tryConnect(ip string, port int, timeout time.Duration) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// generateIPList 从子网生成 IP 列表
func generateIPList(subnet string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}

	var ips []string
	ip := ipnet.IP.Mask(ipnet.Mask)
	for {
		ips = append(ips, ip.String())
		incIP(ip)
		if !ipnet.Contains(ip) {
			break
		}
		if len(ips) > 256 {
			break
		}
	}
	return ips, nil
}

// incIP IP 地址递增
func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
