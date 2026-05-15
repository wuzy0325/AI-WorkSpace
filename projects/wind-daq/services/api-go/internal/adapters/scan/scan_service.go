package scan

import (
	"context"
	"log/slog"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

// ScanService 设备扫描服务，聚合多个扫描器并去重
type ScanService struct {
	scanners []ports.DeviceScanner
}

// NewScanService 创建扫描服务
func NewScanService(scanners ...ports.DeviceScanner) *ScanService {
	return &ScanService{scanners: scanners}
}

// NewDefaultScanService 创建默认扫描服务（UDP 广播优先）
func NewDefaultScanService() *ScanService {
	return NewScanService(
		NewDaqP1604UDPScanner(),
		NewDaqT1603UDPScanner(),
	)
}

// ScanAll 执行所有扫描器的扫描，合并结果并去重
func (s *ScanService) ScanAll(ctx context.Context, timeout time.Duration) ([]ports.DiscoveredDevice, error) {
	var merged []ports.DiscoveredDevice
	seen := make(map[string]bool)

	for _, scanner := range s.scanners {
		devices, err := scanner.Scan(ctx, timeout)
		if err != nil {
			slog.Error("Device scan failed", "err", err)
			continue
		}

		for _, device := range devices {
			key := ports.DeviceFingerprint(device)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, device)
		}
	}

	return merged, nil
}

// ScanByType 按设备类型扫描
func (s *ScanService) ScanByType(ctx context.Context, deviceType string, timeout time.Duration) ([]ports.DiscoveredDevice, error) {
	switch deviceType {
	case "DAQ-P-1604":
		scanner := NewDaqP1604UDPScanner()
		return scanner.Scan(ctx, timeout)
	case "DAQ-T-1603":
		scanner := NewDaqT1603UDPScanner()
		return scanner.Scan(ctx, timeout)
	default:
		return s.ScanAll(ctx, timeout)
	}
}
