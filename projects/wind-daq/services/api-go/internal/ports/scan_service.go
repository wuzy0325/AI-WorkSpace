package ports

import (
	"context"
	"time"
)

type ScanService interface {
	ScanAll(ctx context.Context, timeout time.Duration) ([]DiscoveredDevice, error)
	ScanByType(ctx context.Context, deviceType string, timeout time.Duration) ([]DiscoveredDevice, error)
}
