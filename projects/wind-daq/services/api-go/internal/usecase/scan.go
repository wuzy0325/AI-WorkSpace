package usecase

import (
	"context"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

type ScanService struct {
	provider ports.ScanService
}

func NewScanService(provider ports.ScanService) *ScanService {
	return &ScanService{provider: provider}
}

func (s *ScanService) ScanAll(ctx context.Context, timeout time.Duration) ([]ports.DiscoveredDevice, error) {
	return s.provider.ScanAll(ctx, timeout)
}

func (s *ScanService) ScanByType(ctx context.Context, deviceType string, timeout time.Duration) ([]ports.DiscoveredDevice, error) {
	return s.provider.ScanByType(ctx, deviceType, timeout)
}
