package measurement

import (
	"fmt"
	"math"
	"sort"

	"cal1604/internal/domain"
	"cal1604/internal/events"
)

type Alarm struct {
	PointID           string  `json:"pointId"`
	DeviceID          string  `json:"deviceId,omitempty"`
	TargetPressure    float64 `json:"targetPressure"`
	ActualPressure    float64 `json:"actualPressure"`
	Threshold         float64 `json:"threshold"`
	MaxDeviation      float64 `json:"maxDeviation"`
	OverLimitChannels []int   `json:"overLimitChannels"`
	ErrorMessage      string  `json:"errorMessage,omitempty"`
}

func (s *Service) SetAlarmConfig(cfg domain.AlarmConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alarmConfig = cfg
}

func (s *Service) GetAlarmConfig() domain.AlarmConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alarmConfig
}

func (s *Service) CheckAlarm(point domain.PressurePoint) (*Alarm, error) {
	s.mu.Lock()
	cfg := s.alarmConfig
	workCfg := s.config
	// 深拷贝设备维度数据，避免与 ResolveSkipDevice 并发写 map 产生竞态。
	collectedByDevice := snapshotDevicePointData(point.CollectedByDevice)
	s.mu.Unlock()

	if !cfg.Enabled {
		return nil, nil
	}

	// 量程引用误差：允许偏差 = 量程 x 准确度等级。
	// 当量程为 0（如配置异常或固定单点）时降级为按目标值比例计算。
	span := math.Abs(workCfg.MaxPressure - workCfg.MinPressure)
	allowance := span * workCfg.PrecisionLevel
	if allowance < 1e-10 {
		allowance = math.Abs(point.TargetPressure) * workCfg.PrecisionLevel
	}

	// 单设备兼容：无设备维度数据时走旧字段。
	if len(collectedByDevice) == 0 {
		if len(point.CollectedData) == 0 {
			return nil, nil
		}
		alarm := s.evaluatePointChannels(cfg, workCfg, point, point.CollectedData, allowance, span, "")
		return s.resolveAlarm(alarm, point)
	}

	// 多设备场景：快照后逐设备评估，任一触发即返回（携带 deviceId）。
	deviceIDs := make([]string, 0, len(collectedByDevice))
	for devID := range collectedByDevice {
		deviceIDs = append(deviceIDs, devID)
	}
	sort.Strings(deviceIDs)

	// 第一遍：设备级采集失败优先报警（采集/超时/断开，Collected 为空且带错误状态）。
	for _, devID := range deviceIDs {
		d := collectedByDevice[devID]
		if d.Status == domain.PointStatusError || d.Error != "" {
			return s.resolveAlarm(s.deviceErrorAlarm(cfg, workCfg, point, devID, d.Error), point)
		}
	}

	// 第二遍：逐设备评估通道超限。
	for _, devID := range deviceIDs {
		d := collectedByDevice[devID]
		if len(d.Collected) == 0 {
			continue
		}
		alarm := s.evaluatePointChannels(cfg, workCfg, point, d.Collected, allowance, span, devID)
		if alarm == nil {
			continue
		}
		return s.resolveAlarm(alarm, point)
	}
	return nil, nil
}

// snapshotDevicePointData 深拷贝设备维度数据 map，供锁外安全遍历。
func snapshotDevicePointData(src map[string]domain.DevicePointData) map[string]domain.DevicePointData {
	if len(src) == 0 {
		return nil
	}
	snap := make(map[string]domain.DevicePointData, len(src))
	for k, v := range src {
		cloned := v
		if v.Collected != nil {
			cloned.Collected = append([]float64(nil), v.Collected...)
		}
		snap[k] = cloned
	}
	return snap
}

// deviceErrorAlarm 构造设备级采集失败的报警对象。
func (s *Service) deviceErrorAlarm(cfg domain.AlarmConfig, workCfg domain.WorkflowConfig, point domain.PressurePoint, deviceID, errMsg string) *Alarm {
	var actualPressure float64
	if point.ActualPressure != nil {
		actualPressure = *point.ActualPressure
	}
	return &Alarm{
		PointID:        point.ID,
		DeviceID:       deviceID,
		TargetPressure: point.TargetPressure,
		ActualPressure: actualPressure,
		Threshold:      workCfg.PrecisionLevel,
		MaxDeviation:   0,
		ErrorMessage:   errMsg,
	}
}

// evaluatePointChannels 对单个设备通道数据执行报警判定。
func (s *Service) evaluatePointChannels(cfg domain.AlarmConfig, workCfg domain.WorkflowConfig, point domain.PressurePoint, data []float64, allowance, span float64, deviceID string) *Alarm {
	enabledCh := cfg.EnabledChannels
	if len(enabledCh) == 0 {
		for i := range data {
			enabledCh = append(enabledCh, i+1)
		}
	}

	var overLimit []int
	maxDev := 0.0

	for _, ch := range enabledCh {
		if ch < 1 || ch > len(data) {
			continue
		}
		collectedVal := data[ch-1]
		dev := math.Abs(collectedVal - point.TargetPressure)

		if dev > allowance {
			overLimit = append(overLimit, ch)
			if dev > maxDev {
				maxDev = dev
			}
		}
	}

	if len(overLimit) == 0 {
		return nil
	}

	// maxDeviation 表示最大偏差占量程的比值（FS 百分比小数形式）。
	// 量程为 0 时退化为相对目标值的比例，目标值也为 0 时为 0。
	var maxDevRatio float64
	switch {
	case span > 1e-10:
		maxDevRatio = maxDev / span
	case point.TargetPressure != 0:
		maxDevRatio = maxDev / math.Abs(point.TargetPressure)
	}

	var actualPressure float64
	if point.ActualPressure != nil {
		actualPressure = *point.ActualPressure
	}

	return &Alarm{
		PointID:           point.ID,
		DeviceID:          deviceID,
		TargetPressure:    point.TargetPressure,
		ActualPressure:    actualPressure,
		Threshold:         workCfg.PrecisionLevel,
		MaxDeviation:      maxDevRatio,
		OverLimitChannels: overLimit,
	}
}

// resolveAlarm 设置 pending 状态并发布报警事件。
func (s *Service) resolveAlarm(alarm *Alarm, point domain.PressurePoint) (*Alarm, error) {
	s.mu.Lock()
	s.alarmPending = true
	s.currentAlarm = alarm
	s.mu.Unlock()

	s.publish(events.EventMeasurementAlarmTriggered, alarm)

	return alarm, nil
}

func (s *Service) IsAlarmPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alarmPending
}

// GetCurrentAlarm 返回当前挂起报警的快照；无挂起报警时返回 nil。
// 用于页面刷新后通过 HTTP 恢复报警详情（SSE 事件已错过）。
func (s *Service) GetCurrentAlarm() *Alarm {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alarmPending || s.currentAlarm == nil {
		return nil
	}
	snapshot := *s.currentAlarm
	if s.currentAlarm.OverLimitChannels != nil {
		snapshot.OverLimitChannels = append([]int(nil), s.currentAlarm.OverLimitChannels...)
	}
	return &snapshot
}

func (s *Service) ResolveAlarm(decision string) error {
	s.mu.Lock()
	if !s.alarmPending {
		s.mu.Unlock()
		return fmt.Errorf("no alarm pending")
	}

	alarmCh := s.alarmCh
	s.mu.Unlock()

	if alarmCh != nil {
		select {
		case alarmCh <- decision:
		default:
		}
	}

	s.mu.Lock()
	s.alarmPending = false
	s.currentAlarm = nil
	s.mu.Unlock()

	s.publish(events.EventMeasurementAlarmResolved, map[string]string{
		"decision": decision,
	})

	return nil
}

// GetStabilityTimeoutPending 返回稳定超时挂起状态与对应压力点序号。
func (s *Service) GetStabilityTimeoutPending() (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stabilityTimeoutPending, s.stabilityTimeoutPointIndex
}

// ResolveStabilityTimeout 接收前端用户对稳定超时的决定。
// 仅在确实存在挂起时投递，防止无人等待时决策滞留通道，
// 被下一次超时误消费（跳过点未弹窗）。
func (s *Service) ResolveStabilityTimeout(decision string) {
	s.mu.Lock()
	pending := s.stabilityTimeoutPending
	ch := s.stabilityTimeoutCh
	s.mu.Unlock()

	if !pending || ch == nil {
		return
	}

	select {
	case ch <- decision:
	default:
	}
}

// ResolveSkipDevice 用户选择永久跳过指定计量设备。
// 该设备从本批次剩余压力点中移除，已完成压力点数据保留并标记设备级 skipped + 原因。
func (s *Service) ResolveSkipDevice(deviceID, reason string) error {
	s.mu.Lock()
	if _, already := s.skippedDevices[deviceID]; already {
		s.mu.Unlock()
		return fmt.Errorf("device %s already skipped", deviceID)
	}
	s.skippedDevices[deviceID] = reason
	// 剩余压力点标记该设备 skipped + 原因；已完成点的数据保留。
	// 收集被修改的压力点快照，逐点发布完整点状态，保证前端按 point.id 正常更新。
	var updatedPoints []domain.PressurePoint
	for i := range s.points {
		p := &s.points[i]
		if p.CollectedByDevice == nil {
			p.CollectedByDevice = make(map[string]domain.DevicePointData)
		}
		if d, ok := p.CollectedByDevice[deviceID]; ok && d.Status == domain.PointStatusCompleted {
			continue
		}
		p.CollectedByDevice[deviceID] = domain.DevicePointData{
			DeviceID:   deviceID,
			Status:     domain.PointStatusSkipped,
			SkipReason: reason,
		}
		updatedPoints = append(updatedPoints, *p)
	}
	s.mu.Unlock()

	for i := range updatedPoints {
		s.publish(events.EventMeasurementPointStatus, updatedPoints[i])
	}
	return nil
}
