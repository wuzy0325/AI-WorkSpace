package usecase

import (
	"context"
	"log/slog"
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
		slog.Warn("MotionStatusPoller Start 发现已有轮询任务，先停止旧任务", "component", "MotionStatusPoller")
		p.stop()
	}
	ctx, cancel := context.WithCancel(ctx)
	p.stop = cancel

	slog.Info("MotionStatusPoller Start 开始轮询", "component", "MotionStatusPoller")

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		slog.Info("MotionStatusPoller 轮询 goroutine 已启动", "component", "MotionStatusPoller")

		for {
			select {
			case <-ctx.Done():
				slog.Info("MotionStatusPoller 轮询 goroutine 退出", "component", "MotionStatusPoller")
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
		slog.Info("MotionStatusPoller Stop", "component", "MotionStatusPoller")
		p.stop()
	} else {
		slog.Warn("MotionStatusPoller Stop 跳过：无正在运行的轮询", "component", "MotionStatusPoller")
	}
}
