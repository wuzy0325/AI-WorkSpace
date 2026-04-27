package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// TraversalService handles traversal test workflows
type TraversalService struct {
	mu            sync.Mutex
	status        traversal.TraversalStatus
	config        *traversal.TraversalConfig
	currentStep   int
	totalSteps    int
	deviceManager *DeviceManager
	motionManager *MotionManager
	publisher     ports.DataPublisher
	cancelFunc    context.CancelFunc
}

// NewTraversalService creates a traversal service
func NewTraversalService(deviceManager *DeviceManager, motionManager *MotionManager, publisher ports.DataPublisher) *TraversalService {
	return &TraversalService{
		status:        traversal.TraversalIdle,
		deviceManager: deviceManager,
		motionManager: motionManager,
		publisher:     publisher,
	}
}

// Start starts a traversal test
func (s *TraversalService) Start(config traversal.TraversalConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == traversal.TraversalRunning {
		return fmt.Errorf("traversal already running")
	}
	s.config = &config
	s.status = traversal.TraversalRunning
	s.currentStep = 0
	s.totalSteps = int((config.EndPos-config.StartPos)/config.StepSize) + 1

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel

	go s.runLoop(ctx)
	slog.Info("Traversal started", "steps", s.totalSteps)
	return nil
}

// Pause pauses the traversal
func (s *TraversalService) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != traversal.TraversalRunning {
		return fmt.Errorf("not running")
	}
	s.status = traversal.TraversalPaused
	return nil
}

// Resume resumes the traversal
func (s *TraversalService) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != traversal.TraversalPaused {
		return fmt.Errorf("not paused")
	}
	s.status = traversal.TraversalRunning
	return nil
}

// Stop stops the traversal
func (s *TraversalService) Stop() {
	s.mu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.status = traversal.TraversalIdle
	s.mu.Unlock()
}

// GetProgress returns the current progress
func (s *TraversalService) GetProgress() traversal.TraversalProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return traversal.TraversalProgress{
		CurrentStep: s.currentStep,
		TotalSteps:  s.totalSteps,
		Status:      s.status,
	}
}

func (s *TraversalService) runLoop(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		if s.status == traversal.TraversalRunning {
			s.status = traversal.TraversalDone
		}
		s.mu.Unlock()
	}()

	for i := 0; i < s.totalSteps; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Wait if paused
		for {
			s.mu.Lock()
			paused := s.status == traversal.TraversalPaused
			s.mu.Unlock()
			if !paused {
				break
			}
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}

		pos := s.config.StartPos + float64(i)*s.config.StepSize
		if err := s.motionManager.MoveTo(s.config.ControllerID, motion.AxisName(s.config.Axis), pos); err != nil {
			slog.Error("Traversal move failed", "step", i, "err", err)
			continue
		}

		select {
		case <-time.After(time.Duration(s.config.DwellTimeMs) * time.Millisecond):
		case <-ctx.Done():
			return
		}

		s.mu.Lock()
		s.currentStep = i + 1
		s.mu.Unlock()

		if s.publisher != nil {
			s.publisher.Broadcast(ports.ChannelTraversalProgress, traversal.TraversalProgress{
				CurrentPos:  pos,
				CurrentStep: i + 1,
				TotalSteps:  s.totalSteps,
				Status:      traversal.TraversalRunning,
			})
		}
	}

	if s.publisher != nil {
		s.publisher.Broadcast(ports.ChannelTraversalComplete, map[string]interface{}{
			"success":    true,
			"totalSteps": s.totalSteps,
			"timestamp":  time.Now(),
		})
	}
}
