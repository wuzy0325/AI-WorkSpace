package events

import (
	"context"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// StartStatusPoller publishes motion statuses until the context is canceled.
func StartStatusPoller(ctx context.Context, interval time.Duration, getStatus func() []core.ControllerStatus, emit func([]core.ControllerStatus)) {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			statuses := getStatus()
			if len(statuses) > 0 {
				emit(statuses)
			}
		}
	}
}
