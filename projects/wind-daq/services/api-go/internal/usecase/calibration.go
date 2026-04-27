package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 校准服务 ====================
// 处理探针自动校准工作流
// 支持: 五孔探针、三孔探针、总压探针、总温探针

// CalibrationService 校准服务
// 管理自动校准任务的生命周期
type CalibrationService struct {
	mu           sync.Mutex
	taskID       string                         // 任务ID
	status       calibration.CalibrationStatus  // 校准状态
	config       *calibration.CalibrationConfig // 校准配置
	currentPoint int                            // 当前点位索引
	completedPts int                            // 已完成点数
	startTime    time.Time                      // 开始时间
	lastError    string                         // 最后错误

	deviceManager *DeviceManager      // 设备管理器
	motionManager *MotionManager      // 运动控制器管理器
	publisher     ports.DataPublisher // 数据广播
	cancelFunc    context.CancelFunc  // 取消函数
}

// NewCalibrationService 创建校准服务
// 参数: deviceManager 设备管理器, motionManager 运动控制器管理器, wsHub WebSocket Hub
// 返回: *CalibrationService 校准服务实例
func NewCalibrationService(deviceManager *DeviceManager, motionManager *MotionManager, publisher ports.DataPublisher) *CalibrationService {
	return &CalibrationService{
		status:        calibration.CalIdle,
		deviceManager: deviceManager,
		motionManager: motionManager,
		publisher:     publisher,
	}
}

// Start 启动校准任务
// 参数: config 校准配置
// 返回: (任务ID, 错误信息)
func (s *CalibrationService) Start(config calibration.CalibrationConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == calibration.CalRunning || s.status == calibration.CalPaused {
		return "", fmt.Errorf("calibration already running (task: %s)", s.taskID)
	}

	s.taskID = fmt.Sprintf("cal-%d", time.Now().UnixMilli())
	s.config = &config
	s.status = calibration.CalRunning
	s.currentPoint = 0
	s.completedPts = 0
	s.startTime = time.Now()
	s.lastError = ""

	// 创建取消上下文
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel

	// 后台运行校准循环
	go s.runCalibrationLoop(ctx)

	slog.Info("Calibration started", "taskId", s.taskID, "type", config.Type, "points", len(config.Points))
	return s.taskID, nil
}

// Pause 暂停校准
func (s *CalibrationService) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != calibration.CalRunning {
		return fmt.Errorf("calibration not running")
	}
	s.status = calibration.CalPaused
	slog.Info("Calibration paused", "taskId", s.taskID)
	return nil
}

// Resume 恢复校准
func (s *CalibrationService) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != calibration.CalPaused {
		return fmt.Errorf("calibration not paused")
	}
	s.status = calibration.CalRunning
	slog.Info("Calibration resumed", "taskId", s.taskID)
	return nil
}

// Stop 停止校准
func (s *CalibrationService) Stop() {
	s.mu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.status = calibration.CalIdle
	s.mu.Unlock()
	slog.Info("Calibration stopped", "taskId", s.taskID)
}

// GetStatus 获取校准状态
// 返回: *calibration.CalibrationTaskStatus 任务状态
func (s *CalibrationService) GetStatus() *calibration.CalibrationTaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	if s.config != nil {
		total = len(s.config.Points)
	}
	return &calibration.CalibrationTaskStatus{
		TaskID:          s.taskID,
		Status:          s.status,
		CurrentPoint:    s.currentPoint,
		CompletedPoints: s.completedPts,
		TotalPoints:     total,
		StartTime:       s.startTime,
		LastError:       s.lastError,
	}
}

// SaveConfig 保存校准配置
func (s *CalibrationService) SaveConfig(config calibration.CalibrationConfig) error {
	// TODO: 持久化到JSON文件
	slog.Info("Calibration config saved", "type", config.Type)
	return nil
}

// GetConfig 获取校准配置
func (s *CalibrationService) GetConfig() *calibration.CalibrationConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config
}

// runCalibrationLoop 校准主循环
// 后台执行: 移动到点位 -> 等待稳定 -> 采集数据 -> 推送进度
func (s *CalibrationService) runCalibrationLoop(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		if s.status == calibration.CalRunning {
			s.status = calibration.CalCompleted
		}
		s.mu.Unlock()

		s.sendCompleteEvent(true, "", 0)
	}()

	for i, point := range s.config.Points {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 暂停时等待
		s.waitIfPaused(ctx)
		if ctx.Err() != nil {
			return
		}

		s.mu.Lock()
		s.currentPoint = i
		s.mu.Unlock()

		// 1. 移动到目标点位
		s.sendRealtimeEvent("moving", &point, nil)
		if err := s.moveToPoint(point); err != nil {
			slog.Error("Move failed", "point", i, "err", err)
			if s.config.StopOnError {
				s.mu.Lock()
				s.status = calibration.CalError
				s.lastError = err.Error()
				s.mu.Unlock()
				s.sendCompleteEvent(false, err.Error(), 0)
				return
			}
			continue
		}

		// 2. 等待稳定
		s.sendRealtimeEvent("waiting", &point, nil)
		select {
		case <-time.After(time.Duration(s.config.DwellTimeMs) * time.Millisecond):
		case <-ctx.Done():
			return
		}

		// 3. 采集数据
		data, err := s.acquireData(point)
		if err != nil {
			slog.Error("Acquire failed", "point", i, "err", err)
			if s.config.StopOnError {
				s.mu.Lock()
				s.status = calibration.CalError
				s.lastError = err.Error()
				s.mu.Unlock()
				s.sendCompleteEvent(false, err.Error(), 0)
				return
			}
			continue
		}

		// 4. 更新进度
		s.mu.Lock()
		s.completedPts++
		s.mu.Unlock()

		s.sendProgressEvent(i, data)
		s.sendRealtimeEvent("data", &point, data)
	}
}

// waitIfPaused 暂停时等待
func (s *CalibrationService) waitIfPaused(ctx context.Context) {
	for {
		s.mu.Lock()
		paused := s.status == calibration.CalPaused
		s.mu.Unlock()
		if !paused {
			return
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// moveToPoint 移动到目标点位
func (s *CalibrationService) moveToPoint(point calibration.CalibrationPoint) error {
	for _, axis := range s.config.MotionAxes {
		var position float64
		switch axis.Axis {
		case "X":
			position = point.X
		case "Y":
			position = point.Y
		case "Z":
			position = point.Z
		default:
			continue
		}
		if err := s.motionManager.MoveTo(axis.ControllerID, motion.AxisName(axis.Axis), position); err != nil {
			return fmt.Errorf("move %s axis: %w", axis.Axis, err)
		}
	}
	return nil
}

// acquireData 采集数据,根据校准类型分发
func (s *CalibrationService) acquireData(point calibration.CalibrationPoint) (interface{}, error) {
	channelData, err := s.readChannelData()
	if err != nil {
		return nil, fmt.Errorf("read channel data: %w", err)
	}

	s.mu.Lock()
	calType := s.config.Type
	samplesPerPoint := s.config.SamplesPerPoint
	probeChannels := s.config.ProbeChannels
	s.mu.Unlock()

	if samplesPerPoint <= 0 {
		samplesPerPoint = 1
	}

	// 根据校准类型调用不同的采集方法
	switch calType {
	case calibration.CalFiveHole:
		return s.acquireFiveHoleData(point, channelData, probeChannels, samplesPerPoint)
	case calibration.CalThreeHole:
		return s.acquireThreeHoleData(point, channelData, probeChannels, samplesPerPoint)
	case calibration.CalTotalPressure:
		return s.acquireTotalPressureData(point, channelData, probeChannels, samplesPerPoint)
	case calibration.CalTotalTemperature:
		return s.acquireTotalTemperatureData(point, channelData, probeChannels, samplesPerPoint)
	default:
		return nil, fmt.Errorf("unknown calibration type: %s", calType)
	}
}

// readChannelData 从所有配置的通道读取最新数据
func (s *CalibrationService) readChannelData() (map[string]calibration.ChannelData, error) {
	if s.deviceManager == nil {
		return nil, fmt.Errorf("device manager not initialized")
	}

	instances := s.deviceManager.GetInstances()
	result := make(map[string]calibration.ChannelData)
	for _, inst := range instances {
		if inst.Status != string(device.ConnectionConnected) {
			continue
		}
		result[inst.ProfileID] = calibration.ChannelData{
			DeviceID: inst.ProfileID,
			Channels: make(map[int]float64),
		}
	}

	s.mu.Lock()
	hasChannels := len(s.config.ProbeChannels) > 0
	s.mu.Unlock()

	if hasChannels && len(result) == 0 {
		return nil, fmt.Errorf("no connected devices available for data acquisition")
	}

	return result, nil
}

// getChannelValue 从数据映射中提取通道值
func getChannelValue(data map[string]calibration.ChannelData, deviceID string, channel int) (float64, error) {
	d, ok := data[deviceID]
	if !ok {
		return 0, fmt.Errorf("device %s not found in data", deviceID)
	}
	v, ok := d.Channels[channel]
	if !ok {
		return 0, fmt.Errorf("channel %d not found in device %s", channel, deviceID)
	}
	return v, nil
}

// acquireFiveHoleData 采集五孔探针校准数据
func (s *CalibrationService) acquireFiveHoleData(point calibration.CalibrationPoint, data map[string]calibration.ChannelData, channels []calibration.ProbeChannelConfig, samples int) (interface{}, error) {
	p1, err := s.getChannelValueByRole(data, channels, "p1")
	if err != nil {
		return nil, fmt.Errorf("get P1: %w", err)
	}
	p2, err := s.getChannelValueByRole(data, channels, "p2")
	if err != nil {
		return nil, fmt.Errorf("get P2: %w", err)
	}
	p3, err := s.getChannelValueByRole(data, channels, "p3")
	if err != nil {
		return nil, fmt.Errorf("get P3: %w", err)
	}
	p4, err := s.getChannelValueByRole(data, channels, "p4")
	if err != nil {
		return nil, fmt.Errorf("get P4: %w", err)
	}
	p5, err := s.getChannelValueByRole(data, channels, "p5")
	if err != nil {
		return nil, fmt.Errorf("get P5: %w", err)
	}
	pAtm, err := s.getChannelValueByRole(data, channels, "pAtm")
	if err != nil {
		return nil, fmt.Errorf("get PAtm: %w", err)
	}
	pTotal, err := s.getChannelValueByRole(data, channels, "pTotal")
	if err != nil {
		return nil, fmt.Errorf("get PTotal: %w", err)
	}
	tAtm, err := s.getChannelValueByRole(data, channels, "tAtm")
	if err != nil {
		return nil, fmt.Errorf("get TAtm: %w", err)
	}

	raw := calibration.FiveHoleRawData{
		P1: p1, P2: p2, P3: p3, P4: p4, P5: p5,
		PAtm: pAtm, TAtm: tAtm, PTotal: pTotal,
	}

	coeffs := calibration.CalculateFiveHoleCoefficients(raw)

	return map[string]interface{}{
		"pointIndex":   point.Index,
		"rawData":      raw,
		"coefficients": coeffs,
	}, nil
}

// acquireThreeHoleData 采集三孔探针校准数据
func (s *CalibrationService) acquireThreeHoleData(point calibration.CalibrationPoint, data map[string]calibration.ChannelData, channels []calibration.ProbeChannelConfig, samples int) (interface{}, error) {
	p1, err := s.getChannelValueByRole(data, channels, "p1")
	if err != nil {
		return nil, fmt.Errorf("get P1: %w", err)
	}
	p2, err := s.getChannelValueByRole(data, channels, "p2")
	if err != nil {
		return nil, fmt.Errorf("get P2: %w", err)
	}
	p3, err := s.getChannelValueByRole(data, channels, "p3")
	if err != nil {
		return nil, fmt.Errorf("get P3: %w", err)
	}
	pAtm, err := s.getChannelValueByRole(data, channels, "pAtm")
	if err != nil {
		return nil, fmt.Errorf("get PAtm: %w", err)
	}
	pTotal, err := s.getChannelValueByRole(data, channels, "pTotal")
	if err != nil {
		return nil, fmt.Errorf("get PTotal: %w", err)
	}

	raw := calibration.ThreeHoleRawData{
		P1: p1, P2: p2, P3: p3,
		PAtm: pAtm, PTotal: pTotal,
	}

	coeffs := calibration.CalculateThreeHoleCoefficients(raw)

	return map[string]interface{}{
		"pointIndex":   point.Index,
		"rawData":      raw,
		"coefficients": coeffs,
	}, nil
}

// acquireTotalPressureData 采集总压探针校准数据
func (s *CalibrationService) acquireTotalPressureData(point calibration.CalibrationPoint, data map[string]calibration.ChannelData, channels []calibration.ProbeChannelConfig, samples int) (interface{}, error) {
	pAtm, err := s.getChannelValueByRole(data, channels, "pAtm")
	if err != nil {
		return nil, fmt.Errorf("get PAtm: %w", err)
	}
	tAtm, err := s.getChannelValueByRole(data, channels, "tAtm")
	if err != nil {
		return nil, fmt.Errorf("get TAtm: %w", err)
	}
	pTunnelTotal, err := s.getChannelValueByRole(data, channels, "pTunnelTotal")
	if err != nil {
		return nil, fmt.Errorf("get PTunnelTotal: %w", err)
	}
	pTunnelStatic, err := s.getChannelValueByRole(data, channels, "pTunnelStatic")
	if err != nil {
		return nil, fmt.Errorf("get PTunnelStatic: %w", err)
	}
	tTunnel, err := s.getChannelValueByRole(data, channels, "tTunnel")
	if err != nil {
		return nil, fmt.Errorf("get TTunnel: %w", err)
	}
	pProbeTotal, err := s.getChannelValueByRole(data, channels, "pProbeTotal")
	if err != nil {
		return nil, fmt.Errorf("get PProbeTotal: %w", err)
	}

	raw := calibration.TotalPressureRawData{
		PAtm:          pAtm,
		TAtm:          tAtm,
		PTunnelTotal:  pTunnelTotal,
		PTunnelStatic: pTunnelStatic,
		TTunnel:       tTunnel,
		PProbeTotal:   pProbeTotal,
	}

	coeffs := calibration.CalculateTotalPressureCoefficients(raw)

	return map[string]interface{}{
		"pointIndex":   point.Index,
		"rawData":      raw,
		"coefficients": coeffs,
	}, nil
}

// acquireTotalTemperatureData 采集总温探针校准数据
func (s *CalibrationService) acquireTotalTemperatureData(point calibration.CalibrationPoint, data map[string]calibration.ChannelData, channels []calibration.ProbeChannelConfig, samples int) (interface{}, error) {
	pAtm, err := s.getChannelValueByRole(data, channels, "pAtm")
	if err != nil {
		return nil, fmt.Errorf("get PAtm: %w", err)
	}
	tAtm, err := s.getChannelValueByRole(data, channels, "tAtm")
	if err != nil {
		return nil, fmt.Errorf("get TAtm: %w", err)
	}
	pTotal, err := s.getChannelValueByRole(data, channels, "pTotal")
	if err != nil {
		return nil, fmt.Errorf("get PTotal: %w", err)
	}
	pStatic, err := s.getChannelValueByRole(data, channels, "pStatic")
	if err != nil {
		return nil, fmt.Errorf("get PStatic: %w", err)
	}
	tProbe, err := s.getChannelValueByRole(data, channels, "tProbe")
	if err != nil {
		return nil, fmt.Errorf("get TProbe: %w", err)
	}
	tStandard, err := s.getChannelValueByRole(data, channels, "tStandard")
	if err != nil {
		return nil, fmt.Errorf("get TStandard: %w", err)
	}

	machNumber := calibration.CalculateMachNumber(pTotal+pAtm, pStatic+pAtm)
	recoveryCoeff := calibration.CalculateRecoveryCoefficient(tProbe, tStandard)

	return map[string]interface{}{
		"pointIndex":             point.Index,
		"machNumber":             machNumber,
		"recoveryCoefficient":    recoveryCoeff,
		"totalPressure":          pTotal,
		"staticPressure":         pStatic,
		"atmosphericPressure":    pAtm,
		"atmosphericTemperature": tAtm,
		"testProbeTemp":          tProbe,
		"standardProbeTemp":      tStandard,
	}, nil
}

// getChannelValueByRole 根据角色获取通道值
func (s *CalibrationService) getChannelValueByRole(data map[string]calibration.ChannelData, channels []calibration.ProbeChannelConfig, role string) (float64, error) {
	for _, ch := range channels {
		if string(ch.Role) == role {
			return getChannelValue(data, ch.DeviceID, ch.Channel)
		}
	}
	return 0, fmt.Errorf("channel with role %s not found", role)
}

// sendProgressEvent 发送进度事件
func (s *CalibrationService) sendProgressEvent(current int, data interface{}) {
	if s.publisher == nil {
		return
	}
	s.mu.Lock()
	total := len(s.config.Points)
	taskID := s.taskID
	s.mu.Unlock()
	s.publisher.Broadcast(ports.ChannelCalibProgress, calibration.CalibrationProgressEvent{
		TaskID:          taskID,
		CurrentPoint:    current,
		CompletedPoints: s.completedPts,
		TotalPoints:     total,
		LatestData:      data,
		Timestamp:       time.Now(),
	})
}

// sendCompleteEvent 发送完成事件
func (s *CalibrationService) sendCompleteEvent(success bool, errMsg string, duration float64) {
	if s.publisher == nil {
		return
	}
	s.mu.Lock()
	taskID := s.taskID
	total := len(s.config.Points)
	s.mu.Unlock()
	s.publisher.Broadcast(ports.ChannelCalibComplete, calibration.CalibrationCompleteEvent{
		TaskID:      taskID,
		Success:     success,
		Error:       errMsg,
		Duration:    time.Since(s.startTime).Seconds(),
		TotalPoints: total,
		Timestamp:   time.Now(),
	})
}

// sendRealtimeEvent 发送实时事件
func (s *CalibrationService) sendRealtimeEvent(eventType string, point *calibration.CalibrationPoint, data interface{}) {
	if s.publisher == nil {
		return
	}
	s.mu.Lock()
	taskID := s.taskID
	s.mu.Unlock()
	s.publisher.Broadcast(ports.ChannelCalibRealtime, calibration.CalibrationRealtimeEvent{
		TaskID:    taskID,
		Type:      eventType,
		Point:     point,
		Data:      data,
		Timestamp: time.Now(),
	})
}
