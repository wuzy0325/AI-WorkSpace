package calibration

import (
	"context"
	"fmt"
	"time"

	"cal1604/internal/application/session"
	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/events"
	"cal1604/internal/workflow"
)

// SetDevices 设置校准使用的设备（单计量设备兼容入口）。
// 委托给 session.Service 处理，同时保持本地驱动引用以供标定流程使用。
func (s *Service) SetDevices(measureDevID, pressureDevID string) error {
	return s.SetMeasureDevices([]string{measureDevID}, pressureDevID)
}

// SetMeasureDevices 设置校准使用的多台计量设备与单台打压设备。
// 委托给 session.Service 处理，同时保持本地驱动引用以供标定流程使用。
func (s *Service) SetMeasureDevices(measureDevIDs []string, pressureDevID string) error {
	if len(measureDevIDs) == 0 {
		return fmt.Errorf("measure device ids must not be empty")
	}

	if s.sessionService != nil {
		if _, err := s.sessionService.BindMeasureDevices(measureDevIDs, pressureDevID, "calibration"); err != nil {
			return err
		}
		s.mu.Lock()
		s.measureDevIDs = append([]string(nil), measureDevIDs...)
		s.measureDevID = measureDevIDs[0]
		s.pressureDevID = pressureDevID
		s.skippedDevices = make(map[string]string)
		s.mu.Unlock()
		return nil
	}

	// 无 sessionService 时，使用 DriverResolver 直接解析驱动。
	s.mu.Lock()
	defer s.mu.Unlock()
	resolver := &session.DriverResolver{
		DeviceManager:  s.deviceManager,
		DriverProvider: s.driverProvider,
		Factory:        s.factory,
	}
	drivers := make(map[string]device.MeasureDriver, len(measureDevIDs))
	for _, id := range measureDevIDs {
		mDrv, err := resolver.ResolveMeasureDriver(id)
		if err != nil {
			return err
		}
		drivers[id] = mDrv
	}
	var pDrv device.PressureDriver
	if pressureDevID != "" {
		var err error
		pDrv, err = resolver.ResolvePressureDriver(pressureDevID)
		if err != nil {
			return err
		}
	}
	s.measureDevIDs = append([]string(nil), measureDevIDs...)
	s.measureDevID = measureDevIDs[0]
	s.pressureDevID = pressureDevID
	s.measureDrivers = drivers
	s.pressureDriver = pDrv
	s.skippedDevices = make(map[string]string)
	return nil
}

// SetConfig 设置校准配置。
func (s *Service) SetConfig(config domain.WorkflowConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
}

// SetAlarmConfig 设置报警配置。
func (s *Service) SetAlarmConfig(config domain.AlarmConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alarmConfig = config
}

// GetAlarmConfig 获取当前报警配置。
func (s *Service) GetAlarmConfig() domain.AlarmConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alarmConfig
}

// GetCalibrationSession 获取当前校准会话（可能为 nil）。
func (s *Service) GetCalibrationSession() *CalibrationSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calibrationSession
}

// SetChannels 设置采集通道。
func (s *Service) SetChannels(channels []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Channels = channels
}

// GetChannels 获取当前通道配置。
func (s *Service) GetChannels() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config.Channels
}

// GeneratePressurePoints 根据配置生成压力点。
// 测点数范围统一为 2~5，超出范围返回错误，禁止隐式裁剪。
// 当 PressureMode 为 roundTrip 时，在正程递增点后追加回程递降点（不含重复的极值点）。
func (s *Service) GeneratePressurePoints() ([]domain.PressurePoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	points := s.config.PointCount
	if points < 2 || points > 5 {
		return nil, fmt.Errorf("pressure points must be between 2 and 5, got %d", points)
	}

	minP := s.config.MinPressure
	maxP := s.config.MaxPressure
	if maxP <= minP {
		return nil, fmt.Errorf("maxPressure(%v) must be greater than minPressure(%v)", maxP, minP)
	}

	prec := s.config.Precision
	if prec < 0 {
		prec = 0
	}

	roundTrip := s.config.PressureMode == domain.PressureModeRoundTrip
	s.pressurePoints = domain.EquidistantPoints(minP, maxP, points, prec, roundTrip)
	s.currentPoint = 0

	return s.pressurePoints, nil
}

// UpdatePointTargetPressure 更新指定压力点的目标压力值。
// 仅允许更新状态为 pending 的压力点，已执行的点不允许修改。
func (s *Service) UpdatePointTargetPressure(pointIndex int, targetPressure float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		return fmt.Errorf("invalid point index: %d", pointIndex)
	}

	point := &s.pressurePoints[pointIndex-1]
	if point.Status != domain.PointStatusPending {
		return fmt.Errorf("cannot update target pressure for point %d with status %s, only pending points can be modified", pointIndex, point.Status)
	}

	if targetPressure < 0 {
		return fmt.Errorf("target pressure must be non-negative, got %v", targetPressure)
	}

	point.TargetPressure = domain.RoundToPrecision(targetPressure, s.config.Precision)
	return nil
}

// GetPressurePoints 获取当前压力点列表。
func (s *Service) GetPressurePoints() []domain.PressurePoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]domain.PressurePoint, len(s.pressurePoints))
	copy(result, s.pressurePoints)
	return result
}

// Pressurize 对指定压力点执行打压。
func (s *Service) Pressurize(ctx context.Context, pointIndex int) error {
	s.mu.Lock()
	pressureDriver := s.getPressureDriver()
	if pressureDriver == nil {
		s.mu.Unlock()
		return ErrPressureDeviceNotSet
	}
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		s.mu.Unlock()
		return fmt.Errorf("invalid point index: %d", pointIndex)
	}
	targetPressure := s.pressurePoints[pointIndex-1].TargetPressure
	s.mu.Unlock()

	s.updatePointStatus(pointIndex, domain.PointStatusPressurizing)

	// 状态迁移: -> pressurizing
	if err := s.coordinator.Machine().Transition(domain.SessionStatePressurizing); err != nil {
		return fmt.Errorf("transition to pressurizing: %w", err)
	}
	s.publishSessionState()

	// 设置目标压力
	if err := pressureDriver.SetTargetPressure(ctx, targetPressure); err != nil {
		s.markPointError(pointIndex)
		return fmt.Errorf("set target pressure: %w", err)
	}

	// 启动压力控制
	if ctrl, ok := pressureDriver.(device.PressureControlCapable); ok {
		if err := ctrl.StartControl(ctx); err != nil {
			s.markPointError(pointIndex)
			return fmt.Errorf("start pressure control: %w", err)
		}
	}

	// 稳定等待：压力首次进入容差范围时才切换到 stabilizing，
	// 避免压力远离目标时就显示"稳定中"。
	if err := s.waitForStabilityWithMonitor(ctx, pointIndex, targetPressure); err != nil {
		return fmt.Errorf("wait for stability: %w", err)
	}

	// 读取实际压力
	actual, err := s.getPressureDriver().ReadCurrentPressure(ctx)
	if err == nil {
		s.mu.Lock()
		s.pressurePoints[pointIndex-1].ActualPressure = &actual
		s.mu.Unlock()
	}

	s.publish(events.EventPressureApplied, map[string]any{
		"pointIndex":     pointIndex,
		"targetPressure": targetPressure,
		"actualPressure": actual,
	})

	return nil
}

// waitForStabilityWithMonitor 等待压力稳定，对齐计量工作台的判稳逻辑。
// 支持硬件判稳（StabilityStatusProvider）+ 软件判稳双路径。
// 超时取 config.StabilityTimeoutMs（默认 120s），读取压力连续失败时跳过而非报错。
// 每次 tick 发布 calibration.stability.update 事件，前端可实时展示稳定计时和进度。
// 超时时发布 timeout 事件等待用户决策（continue/skip）。
// 压力首次进入容差范围时才从 pressurizing 切换到 stabilizing，避免压力远离目标就显示"稳定中"。
func (s *Service) waitForStabilityWithMonitor(ctx context.Context, pointIndex int, targetPressure float64) error {
	stableWaitMs := s.config.StableWaitMs
	if stableWaitMs <= 0 {
		stableWaitMs = 5000
	}
	stabilityTimeoutMs := s.config.StabilityTimeoutMs
	if stabilityTimeoutMs <= 0 {
		stabilityTimeoutMs = 120000
	}

	deadline := time.Now().Add(time.Duration(stabilityTimeoutMs) * time.Millisecond)
	stableDuration := time.Duration(stableWaitMs) * time.Millisecond
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	pressureDriver := s.getPressureDriver()
	deviceStability, hasDeviceStability := pressureDriver.(device.StabilityStatusProvider)

	// 软件判稳：使用固定容差 0.001，与计量工作台保持一致
	monitor := workflow.NewStabilityMonitor(0.001, stableDuration, nil)

	// 标记是否已从 pressurizing 切换到 stabilizing
	transitionedToStabilizing := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pollCtx := device.WithPollContext(ctx)
			if time.Now().After(deadline) {
				// 超时：发布事件并等待前端用户决定
				s.publish(events.EventCalibrationStabilityTimeout, map[string]any{
					"pointIndex":    s.currentPoint,
					"targetPressure": targetPressure,
				})

				select {
				case decision := <-s.stabilityTimeoutCh:
					switch decision {
					case "continue":
						// 用户选择继续等待，重置超时倒计时
						deadline = time.Now().Add(time.Duration(stabilityTimeoutMs) * time.Millisecond)
						continue
					case "skip":
						return ErrPointSkipped
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			var status workflow.StabilityStatus
			if hasDeviceStability {
				// 设备判稳路径：SCPI 设备硬件自行判断压力稳定，软件仅依赖硬件 IsStable 标志。
				// 设备报告稳定时 FeedSample 偏差为 0（累积器继续计时）；
				// 设备报告不稳定时 FeedSample 大偏差（累积器重置）。
				stable, err := deviceStability.IsStable(pollCtx)
				if err != nil {
					continue
				}
				feedVal := targetPressure
				if !stable {
					feedVal = targetPressure + 1000
				}
				status = monitor.FeedSample(targetPressure, feedVal)
			} else {
				currentVal, valErr := pressureDriver.ReadCurrentPressure(pollCtx)
				if valErr != nil {
					continue
				}
				status = monitor.FeedSample(targetPressure, currentVal)
			}

			// 压力首次进入容差范围时，才从 pressurizing 切换到 stabilizing
			if status.IsInRange && !transitionedToStabilizing {
				transitionedToStabilizing = true
				s.updatePointStatus(pointIndex, domain.PointStatusStabilizing)
				if err := s.coordinator.Machine().Transition(domain.SessionStateStabilizing); err != nil {
					// 可能已经在 stabilizing，忽略
				}
				s.publishSessionState()
			}

			// 每次 tick 发布统一的稳定状态更新（对齐计量 measurement.stability.update）
			s.publish(events.EventCalibrationStabilityUpdate, map[string]any{
				"isStable":           status.IsStable,
				"isInRange":          status.IsInRange,
				"currentValue":       status.CurrentValue,
				"targetValue":        status.TargetValue,
				"deviation":          status.Deviation,
				"stableDurationMs":   status.StableDurationMs,
				"requiredDurationMs": status.RequiredDurationMs,
				"progress":           status.Progress,
			})

			if status.IsStable {
				return nil
			}
		}
	}
}
