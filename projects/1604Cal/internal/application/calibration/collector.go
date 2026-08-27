package calibration

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/events"
)

// Collect 从计量设备采集数据，返回首个绑定设备的数据（兼容入口）。
// 多设备场景请使用 CollectDevices 获取按设备维度的完整结果。
func (s *Service) Collect(ctx context.Context, pointIndex int) ([]float64, error) {
	devices, err := s.CollectDevices(ctx, pointIndex)
	if err != nil {
		return nil, err
	}
	// 首个设备数据保持兼容：按绑定顺序（用户勾选顺序）取第一台
	s.mu.Lock()
	firstID := ""
	if len(s.measureDevIDs) > 0 {
		firstID = s.measureDevIDs[0]
	}
	s.mu.Unlock()
	if firstID == "" {
		for id := range devices {
			firstID = id
			break
		}
	}
	return devices[firstID], nil
}

// CollectDevices 从所有参与计量设备采集数据（已跳过设备除外），返回 deviceID -> 通道数据。
// 单设备场景：与改造前逻辑一致，结果同时写入 CollectedData 与 CollectedByDevice。
// 多设备场景：并行采集所有参与设备，结果按设备 ID 写入 CollectedByDevice；
// 设备级失败不直接报错，而是写入设备错误状态，由 checkAlarm 触发设备维度报警。
// 所有设备均被跳过时，该点整体标记 skipped 并返回空 map。
func (s *Service) CollectDevices(ctx context.Context, pointIndex int) (map[string][]float64, error) {
	s.mu.Lock()
	if s.sessionService != nil && s.sessionService.MeasureDriver() == nil {
		s.mu.Unlock()
		return nil, ErrMeasureDeviceNotSet
	}
	if s.sessionService == nil && len(s.measureDevIDs) == 0 {
		s.mu.Unlock()
		return nil, ErrMeasureDeviceNotSet
	}
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		s.mu.Unlock()
		return nil, fmt.Errorf("invalid point index: %d", pointIndex)
	}
	measureDrivers := s.getMeasureDrivers()
	// 过滤已跳过的设备
	for devID := range s.skippedDevices {
		delete(measureDrivers, devID)
	}
	targetPressure := s.pressurePoints[pointIndex-1].TargetPressure
	channels := s.config.Channels
	avgCount := s.config.AverageCount
	if avgCount < 1 {
		avgCount = 1
	}
	prec := s.config.Precision
	s.mu.Unlock()

	// 全部设备被跳过：该点整体标记 skipped，不再打压采集
	if len(measureDrivers) == 0 {
		s.updatePointStatus(pointIndex, domain.PointStatusSkipped)
		s.publish(events.EventPointSkipped, map[string]any{
			"pointIndex": pointIndex,
			"reason":     "all devices skipped",
		})
		return map[string][]float64{}, nil
	}

	s.updatePointStatus(pointIndex, domain.PointStatusCollecting)

	// 状态迁移: -> collecting
	if err := s.coordinator.Machine().Transition(domain.SessionStateCollecting); err != nil {
		// 可能已经在 collecting
	}
	s.publishSessionState()

	// 单设备路径：保持原逻辑
	if len(measureDrivers) == 1 {
		results := make(map[string][]float64, 1)
		for devID, measureDriver := range measureDrivers {
			averaged, err := s.collectFromDriver(ctx, pointIndex, measureDriver, targetPressure, channels, avgCount, prec)
			if err != nil {
				return nil, err
			}
			s.persistCollectedData(pointIndex, devID, averaged)
			results[devID] = averaged
		}
		s.mu.Lock()
		s.currentPoint = pointIndex
		s.mu.Unlock()
		s.updatePointStatus(pointIndex, domain.PointStatusCompleted)

		// 状态迁移: collecting -> point_done
		if err := s.coordinator.Machine().Transition(domain.SessionStatePointDone); err != nil {
			// 忽略
		}
		s.publishSessionState()

		for devID, averaged := range results {
			s.publish(events.EventDataCollected, map[string]any{
				"pointIndex": pointIndex,
				"channels":   channels,
				"deviceId":   devID,
				"data":       averaged,
			})
		}

		return results, nil
	}

	// 多设备路径：并行采集
	results := s.collectAllDevices(ctx, pointIndex, measureDrivers, targetPressure, channels, avgCount, prec)

	s.mu.Lock()
	point := &s.pressurePoints[pointIndex-1]
	if point.CollectedByDevice == nil {
		point.CollectedByDevice = make(map[string]domain.DevicePointData)
	}
	for devID, data := range results {
		point.CollectedByDevice[devID] = domain.DevicePointData{
			DeviceID:    devID,
			Collected:   data,
			Status:      domain.PointStatusCompleted,
			CollectTime: time.Now().UTC().Format(time.RFC3339),
		}
	}
	s.currentPoint = pointIndex
	// 设备级失败判定：任一设备已在 markDeviceError 写入 error 状态。
	// 有失败时不置点 completed、不迁移 point_done，由 checkAlarm 转入
	// await_alarm_resolution 等待用户决策（重试/跳过该设备/停止）。
	hasDeviceError := false
	for _, d := range point.CollectedByDevice {
		if d.Status == domain.PointStatusError || d.Error != "" {
			hasDeviceError = true
			break
		}
	}
	s.mu.Unlock()

	if hasDeviceError {
		return results, nil
	}

	s.updatePointStatus(pointIndex, domain.PointStatusCompleted)

	// 状态迁移: collecting -> point_done
	if err := s.coordinator.Machine().Transition(domain.SessionStatePointDone); err != nil {
		// 忽略
	}
	s.publishSessionState()

	for devID, data := range results {
		s.publish(events.EventDataCollected, map[string]any{
			"pointIndex": pointIndex,
			"channels":   channels,
			"deviceId":   devID,
			"data":       data,
		})
	}

	return results, nil
}

// collectFromDriver 从单个计量驱动采集数据（多样本平均 + 精度截断）。
func (s *Service) collectFromDriver(ctx context.Context, pointIndex int, measureDriver device.MeasureDriver, targetPressure float64, channels []int, avgCount int, prec int) ([]float64, error) {
	// WTN1604 等设备在校准模式下需要走"按点采集"命令，
	// 不能继续复用普通实时采集命令，否则会出现"设备已连接但采集失败"。
	var averaged []float64
	if pointCollector, ok := measureDriver.(device.CalibrationCapable); ok {
		// 校准点采集同样遵循"多次采样取平均"配置，
		// 仅将单次采样命令从普通采集切换为 C01 校准点采集。
		allSamples := make([][]float64, 0, avgCount)
		for i := 0; i < avgCount; i++ {
			data, err := pointCollector.CollectCalibrationPoint(ctx, pointIndex, targetPressure)
			if err != nil {
				s.markPointError(pointIndex)
				return nil, fmt.Errorf("collect calibration point %d sample %d: %w", pointIndex, i+1, err)
			}
			allSamples = append(allSamples, data)
			time.Sleep(100 * time.Millisecond)
		}
		averaged = domain.AverageSamples(allSamples)
	} else {
		// 多次采集求平均（非校准点采集驱动的通用路径）
		allSamples := make([][]float64, 0, avgCount)
		for i := 0; i < avgCount; i++ {
			data, err := measureDriver.CollectData(ctx, channels)
			if err != nil {
				s.markPointError(pointIndex)
				return nil, fmt.Errorf("collect sample %d: %w", i+1, err)
			}
			allSamples = append(allSamples, data)
			time.Sleep(100 * time.Millisecond)
		}

		// 计算平均值
		averaged = domain.AverageSamples(allSamples)
	}

	// 按配置精度截断采集数据
	if prec > 0 {
		for i := range averaged {
			averaged[i] = domain.RoundToPrecision(averaged[i], prec)
		}
	}
	return averaged, nil
}

// persistCollectedData 把单设备的采集结果写入压力点（设备维度 + 单设备兼容字段）。
func (s *Service) persistCollectedData(pointIndex int, devID string, data []float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		return
	}
	point := &s.pressurePoints[pointIndex-1]
	if point.CollectedByDevice == nil {
		point.CollectedByDevice = make(map[string]domain.DevicePointData)
	}
	point.CollectedByDevice[devID] = domain.DevicePointData{
		DeviceID:    devID,
		Collected:   append([]float64(nil), data...),
		Status:      domain.PointStatusCompleted,
		CollectTime: time.Now().UTC().Format(time.RFC3339),
	}
	// 单设备兼容：首个设备数据回填 CollectedData
	if len(s.measureDevIDs) == 1 && s.measureDevIDs[0] == devID {
		point.CollectedData = append([]float64(nil), data...)
	}
}

// collectAllDevices 并行采集所有参与计量设备，返回 deviceID -> 通道数据。
func (s *Service) collectAllDevices(ctx context.Context, pointIndex int, measureDrivers map[string]device.MeasureDriver, targetPressure float64, channels []int, avgCount int, prec int) map[string][]float64 {
	results := make(map[string][]float64, len(measureDrivers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for devID, drv := range measureDrivers {
		wg.Add(1)
		go func(id string, d device.MeasureDriver) {
			defer wg.Done()
			devCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			data, err := s.collectFromDriver(devCtx, pointIndex, d, targetPressure, channels, avgCount, prec)
			if err != nil {
				s.markDeviceError(pointIndex, id, err)
				return
			}
			mu.Lock()
			results[id] = data
			mu.Unlock()
		}(devID, drv)
	}
	wg.Wait()
	return results
}

// markDeviceError 记录指定设备的采集失败状态。
func (s *Service) markDeviceError(pointIndex int, devID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		return
	}
	point := &s.pressurePoints[pointIndex-1]
	if point.CollectedByDevice == nil {
		point.CollectedByDevice = make(map[string]domain.DevicePointData)
	}
	point.CollectedByDevice[devID] = domain.DevicePointData{
		DeviceID: devID,
		Status:   domain.PointStatusError,
		Error:    err.Error(),
	}
	s.publish(events.EventPointError, map[string]any{
		"pointIndex": pointIndex,
		"deviceId":   devID,
		"error":      err.Error(),
	})
}

// Fit 执行数据拟合。
func (s *Service) Fit(ctx context.Context) (*FittingResult, error) {
	s.mu.Lock()
	measureDriver := s.getMeasureDriver()
	if measureDriver == nil {
		s.mu.Unlock()
		return nil, ErrMeasureDeviceNotSet
	}
	s.mu.Unlock()

	// 状态迁移: -> fitting
	if err := s.coordinator.Machine().Transition(domain.SessionStateFitting); err != nil {
		return nil, fmt.Errorf("transition to fitting: %w", err)
	}
	s.publishSessionState()

	// WTN1604 执行拟合
	calDev, ok := measureDriver.(device.CalibrationCapable)
	if !ok {
		// 如果不是 WTN1604，使用软件拟合
		return s.softwareFit(ctx)
	}

	if err := calDev.PerformFitting(ctx); err != nil {
		return nil, fmt.Errorf("perform fitting: %w", err)
	}

	if err := calDev.SaveCoefficients(ctx); err != nil {
		return nil, fmt.Errorf("save coefficients: %w", err)
	}

	// 状态迁移: fitting -> completed
	if err := s.coordinator.Machine().Transition(domain.SessionStateCompleted); err != nil {
		return nil, fmt.Errorf("transition to completed: %w", err)
	}
	s.publishSessionState()

	// WTN1604 拟合不返回系数细节，返回占位结果
	return &FittingResult{Points: len(s.pressurePoints)}, nil
}

// softwareFit 软件侧拟合（非 WTN1604 设备的备选方案）。
func (s *Service) softwareFit(ctx context.Context) (*FittingResult, error) {
	// 简单线性拟合：y = a*x + b
	// 使用最小二乘法
	s.mu.Lock()
	points := s.pressurePoints
	s.mu.Unlock()

	var sumX, sumY, sumXY, sumX2 float64
	n := 0
	for _, p := range points {
		if p.Status == domain.PointStatusCompleted && len(p.CollectedData) > 0 {
			x := p.TargetPressure
			y := p.CollectedData[0] // 使用第一个通道的数据
			sumX += x
			sumY += y
			sumXY += x * y
			sumX2 += x * x
			n++
		}
	}

	if n < 2 {
		return nil, fmt.Errorf("not enough data points for fitting: %d", n)
	}

	// y = a*x + b
	denom := float64(n)*sumX2 - sumX*sumX
	if math.Abs(denom) < 1e-10 {
		return nil, fmt.Errorf("degenerate data for fitting")
	}
	a := (float64(n)*sumXY - sumX*sumY) / denom
	b := (sumY - a*sumX) / float64(n)

	// 计算 R² (拟合优度)
	meanY := sumY / float64(n)
	var ssTot, ssRes float64
	for _, p := range points {
		if p.Status == domain.PointStatusCompleted && len(p.CollectedData) > 0 {
			y := p.CollectedData[0]
			yPred := a*p.TargetPressure + b
			ssTot += (y - meanY) * (y - meanY)
			ssRes += (y - yPred) * (y - yPred)
		}
	}
	r2 := 0.0
	if ssTot > 1e-10 {
		r2 = 1 - ssRes/ssTot
	}

	result := &FittingResult{
		Slope:     a,
		Intercept: b,
		R2:        r2,
		Points:    n,
	}

	s.publish(events.EventFittingCompleted, map[string]any{
		"slope":     a,
		"intercept": b,
		"r2":        r2,
		"points":    n,
	})

	// 状态迁移: fitting -> completed
	if err := s.coordinator.Machine().Transition(domain.SessionStateCompleted); err != nil {
		return nil, fmt.Errorf("transition to completed: %w", err)
	}
	s.publishSessionState()

	return result, nil
}

// ManualPressurize 手动打压：设置目标压力并启动压力控制。
func (s *Service) ManualPressurize(ctx context.Context, target float64) error {
	s.mu.Lock()
	pressureDriver := s.getPressureDriver()
	if pressureDriver == nil {
		s.mu.Unlock()
		return ErrPressureDeviceNotSet
	}
	s.mu.Unlock()

	if err := pressureDriver.SetTargetPressure(ctx, target); err != nil {
		return fmt.Errorf("set target pressure: %w", err)
	}

	if ctrl, ok := pressureDriver.(device.PressureControlCapable); ok {
		if err := ctrl.StartControl(ctx); err != nil {
			return fmt.Errorf("start pressure control: %w", err)
		}
	}

	return nil
}

// ManualCollect 手动采集：对当前压力点执行一次完整采集（含多样本平均）。
func (s *Service) ManualCollect(ctx context.Context) ([]float64, error) {
	s.mu.Lock()
	
	if s.getMeasureDriver() == nil {
		s.mu.Unlock()
		return nil, ErrMeasureDeviceNotSet
	}
	// 找到下一个 pending 点作为当前采集点
	pointIndex := 0
	for i, p := range s.pressurePoints {
		if p.Status == domain.PointStatusPending {
			pointIndex = i + 1
			break
		}
	}
	if pointIndex == 0 {
		s.mu.Unlock()
		return nil, fmt.Errorf("no pending pressure point to collect")
	}
	s.mu.Unlock()

	return s.Collect(ctx, pointIndex)
}
