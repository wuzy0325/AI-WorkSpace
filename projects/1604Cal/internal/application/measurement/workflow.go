package measurement

import (
	"context"
	"fmt"
	"time"

	"cal1604/internal/application/session"
	apperrors "cal1604/internal/errors"
	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/events"
	"cal1604/internal/workflow"
)

// Session 是 domain.WorkflowSession 的类型别名。
type Session = domain.WorkflowSession

// StartWorkflow 启动 measurement 自己的业务流程会话。
// 当前阶段仅完成"参数 + 点位计划 -> ready 会话"的收口，
// 自动/手动采集编排在后续任务继续补齐。
// 启动前会进行门禁校验：当 EnforceValveCalibration=true 时，
// 阀门必须处于 calibration 状态，否则直接拒绝。
func (s *Service) StartWorkflow(ctx context.Context, channels []int) error {
	s.mu.Lock()

	if len(s.sess.MeasureDrivers()) == 0 {
		s.mu.Unlock()
		return session.ErrMeasureDeviceNotSet
	}

	if len(s.points) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("%w: measurement points not generated", apperrors.ErrInvalidArgument)
	}

	enforceValveGate := s.startPrerequisiteConfig.EnforceValveCalibration
	measureDrivers := s.sess.MeasureDrivers()
	s.mu.Unlock()

	// 阀门门禁：所有已绑定计量设备的阀门均须处于校准模式。
	// 在状态迁移与 coordinator.Begin 之前完成，避免门禁失败时还要回滚工作流。
	// 读阀 I/O 不持有 s.mu。
	if enforceValveGate {
		if err := device.CheckValveCalibrationGate(ctx, measureDrivers); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.config.Channels = append([]int(nil), channels...)

	// 单活工作流注册
	if err := s.coordinator.Begin(workflow.OwnerMeasurement); err != nil {
		return err
	}

	currentState := s.coordinator.State()
	if currentState != domain.SessionStateReady {
		s.rows = nil
		// error → idle → ready（error 不允许直接到 ready）
		if currentState == domain.SessionStateError {
			if err := s.coordinator.Machine().Transition(domain.SessionStateIdle); err != nil {
				return fmt.Errorf("start measurement workflow: %w", err)
			}
		}
		if err := s.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
			return fmt.Errorf("start measurement workflow: %w", err)
		}
	}

	s.session = &Session{
		ID:               fmt.Sprintf("measurement-%d", time.Now().UnixMilli()),
		StartTime:        time.Now(),
		Config:           s.config,
		Points:           append([]domain.PressurePoint(nil), s.points...),
		MeasureDeviceID:  s.sess.MeasureDeviceID(),
		MeasureDeviceIDs: s.sess.MeasureDeviceIDs(),
		PressureDeviceID: s.sess.PressureDeviceID(),
		Status:           s.coordinator.State(),
	}

	s.publish(events.EventMeasurementStateChanged, map[string]any{"state": string(domain.SessionStateReady)})
	return nil
}

// GetSession 返回当前 measurement 流程会话快照。
func (s *Service) GetSession() *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil
	}

	cloned := *s.session
	cloned.Points = make([]domain.PressurePoint, len(s.session.Points))
	for i, point := range s.session.Points {
		cloned.Points[i] = point
		if point.CollectedData != nil {
			cloned.Points[i].CollectedData = append([]float64(nil), point.CollectedData...)
		}
	}
	if len(cloned.Config.Channels) > 0 {
		cloned.Config.Channels = append([]int(nil), cloned.Config.Channels...)
	}
	return &cloned
}
