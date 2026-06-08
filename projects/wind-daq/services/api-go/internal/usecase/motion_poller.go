package usecase

import (
	"context"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

type MotionStatusPoller struct {
	manager ports.MotionManager
	status  chan []motion.ControllerStatus
	stop    context.CancelFunc
}

func NewMotionStatusPoller(mgr ports.MotionManager) *MotionStatusPoller {
	return &MotionStatusPoller{
		manager: mgr,
		status:  make(chan []motion.ControllerStatus, 4),
	}
}

func (p *MotionStatusPoller) Start(ctx context.Context) {
	if p.stop != nil {
		p.stop()
	}
	ctx, cancel := context.WithCancel(ctx)
	p.stop = cancel

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				statuses := p.manager.StatusAll(ctx)
				if len(statuses) > 0 {
					select {
					case p.status <- statuses:
					default:
					}
				}
			}
		}
	}()
}

func (p *MotionStatusPoller) Status() <-chan []motion.ControllerStatus {
	return p.status
}

func (p *MotionStatusPoller) Stop() {
	if p.stop != nil {
		p.stop()
	}
}
