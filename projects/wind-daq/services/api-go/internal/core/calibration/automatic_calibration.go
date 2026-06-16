package calibration

import (
	"fmt"
	"log"
	"time"
)

// RuntimeAccess 校准运行时依赖注入接口
// 由 CalibrationManager 注入，提供通道读取、运动控制等能力
type RuntimeAccess interface {
	// GetChannelValue 获取指定设备通道的当前值
	GetChannelValue(deviceID string, channelIndex int) (float64, bool)
	// MoveToPosition 移动指定轴到目标位置
	MoveToPosition(axisName string, position float64) error
	// WaitForMotionComplete 等待所有轴运动完成
	WaitForMotionComplete() error
}

// AutomaticCalibration 自动循环校准引擎
// 提供 move → wait → gate → acquire → hook → push → next 的模板方法
// 五孔、三孔、总压校准使用此引擎
type AutomaticCalibration struct {
	config          Config
	eventPublisher  EventPublisher
	runtime         RuntimeAccess
	taskID          string
	isRunning       bool
	isPaused        bool
	currentPointIdx int
	dataPoints      []DataPoint
	startTime       int64
}

// NewAutomaticCalibration 创建自动校准引擎
func NewAutomaticCalibration(config Config, publisher EventPublisher, runtime RuntimeAccess) *AutomaticCalibration {
	return &AutomaticCalibration{
		config:         config,
		eventPublisher: publisher,
		runtime:        runtime,
		dataPoints:     make([]DataPoint, 0),
	}
}

// SetTaskID 设置任务ID
func (a *AutomaticCalibration) SetTaskID(id string) {
	a.taskID = id
}

// Start 启动自动校准循环
func (a *AutomaticCalibration) Start(algorithm Algorithm) error {
	if a.isRunning {
		return fmt.Errorf("校准已在运行中")
	}

	a.isRunning = true
	a.isPaused = false
	a.currentPointIdx = 0
	a.dataPoints = make([]DataPoint, 0)
	a.startTime = time.Now().UnixMilli()

	log.Printf("[AutomaticCalibration] 启动校准，共 %d 个测点", len(a.config.Points))

	err := a.runCalibrationLoop(algorithm)

	a.isRunning = false
	return err
}

// runCalibrationLoop 校准主循环（模板方法）
func (a *AutomaticCalibration) runCalibrationLoop(algorithm Algorithm) error {
	var pointErrorCount int
	var lastPointError string

	for i := a.currentPointIdx; i < len(a.config.Points); i++ {
		if !a.isRunning {
			log.Printf("[AutomaticCalibration] 校准被用户停止")
			return nil
		}

		// 等待暂停恢复
		a.waitWhilePaused()

		if !a.isRunning {
			return nil
		}

		a.currentPointIdx = i
		point := a.config.Points[i]

		log.Printf("[AutomaticCalibration] 处理测点 %d/%d, 坐标: %v", i+1, len(a.config.Points), point.Coordinates)

		if err := a.processPoint(algorithm, point, i); err != nil {
			log.Printf("[AutomaticCalibration] 测点 %d 采集失败: %v", i+1, err)
			pointErrorCount++
			lastPointError = err.Error()

			if a.config.StopOnError {
				return fmt.Errorf("测点 %d 失败: %w", i+1, err)
			}
			// 继续下一个点
			continue
		}
	}

	// 循环结束
	if pointErrorCount > 0 {
		errMsg := fmt.Sprintf("%d 个测点采集失败。最后错误: %s", pointErrorCount, lastPointError)
		a.sendCompletionEvent(false, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	a.sendCompletionEvent(true, "")
	return nil
}

// processPoint 处理单个测点的完整流程
func (a *AutomaticCalibration) processPoint(algorithm Algorithm, point CalPoint, index int) error {
	// 1. 移动到目标点位
	if err := a.moveToPoint(point); err != nil {
		return fmt.Errorf("移动到测点失败: %w", err)
	}

	// 2. 等待驻留时间（让压力稳定）
	if a.config.DwellTimeMs > 0 {
		time.Sleep(time.Duration(a.config.DwellTimeMs) * time.Millisecond)
	}

	// 3. 等待球罐闸门条件（如果启用）
	if err := a.waitForSphereTankGateIfNeeded(); err != nil {
		return fmt.Errorf("球罐闸门等待失败: %w", err)
	}

	// 4. 采集数据
	channelReader := a.makeChannelReader()
	dataPoint, err := algorithm.AcquireData(point, channelReader, a.config.SamplesPerPoint)
	if err != nil {
		return fmt.Errorf("数据采集失败: %w", err)
	}

	// 5. 保存数据点
	a.dataPoints = append(a.dataPoints, dataPoint)

	// 6. 发送进度更新
	a.sendProgressUpdate(point, dataPoint)

	return nil
}

// moveToPoint 移动到指定点位
// 默认实现按坐标顺序移动各轴；五孔探针等子类可重写以自定义轴运动顺序
func (a *AutomaticCalibration) moveToPoint(point CalPoint) error {
	if a.runtime == nil {
		return nil // 无运动控制时跳过
	}

	if len(a.config.MotionAxes) == 0 {
		return nil
	}

	for axisName, position := range point.Coordinates {
		// 查找对应的运动轴配置
		axisConfig := a.findAxisConfig(axisName)
		if axisConfig == nil {
			return fmt.Errorf("未找到轴 %s 的运动配置", axisName)
		}

		log.Printf("[AutomaticCalibration] 移动 %s 轴到 %v", axisName, position)
		if err := a.runtime.MoveToPosition(axisName, position); err != nil {
			return fmt.Errorf("移动 %s 轴到 %v 失败: %w", axisName, position, err)
		}
	}

	// 等待所有轴运动完成
	if err := a.runtime.WaitForMotionComplete(); err != nil {
		return fmt.Errorf("等待运动完成超时: %w", err)
	}

	return nil
}

// MoveToPointWithOrder 按指定轴顺序移动到点位（五孔探针专用：先α后β）
func (a *AutomaticCalibration) MoveToPointWithOrder(point CalPoint, axisOrder []string) error {
	if a.runtime == nil || len(a.config.MotionAxes) == 0 {
		return nil
	}

	for _, axisName := range axisOrder {
		position, ok := point.Coordinates[axisName]
		if !ok {
			return fmt.Errorf("测点缺少 %s 坐标", axisName)
		}

		axisConfig := a.findAxisConfig(axisName)
		if axisConfig == nil {
			return fmt.Errorf("未找到轴 %s 的运动配置", axisName)
		}

		log.Printf("[AutomaticCalibration] 移动 %s 轴到 %v", axisName, position)
		if err := a.runtime.MoveToPosition(axisName, position); err != nil {
			return fmt.Errorf("移动 %s 轴到 %v 失败: %w", axisName, position, err)
		}
	}

	if err := a.runtime.WaitForMotionComplete(); err != nil {
		return fmt.Errorf("等待运动完成超时: %w", err)
	}

	return nil
}

// waitForSphereTankGateIfNeeded 等待球罐闸门条件
func (a *AutomaticCalibration) waitForSphereTankGateIfNeeded() error {
	gate := NormalizeSphereTankGateConfig(a.config)
	if gate == nil || !gate.Enabled {
		return nil
	}

	if err := ValidateSphereTankGateConfig(gate); err != nil {
		return err
	}

	maxWaitMs := 300000 // 5分钟超时
	gateWaitStartAt := time.Now().UnixMilli()

	for a.isRunning {
		// 暂停时等待
		a.waitWhilePaused()
		if !a.isRunning {
			return nil
		}

		// 读取稳定时间通道
		stableRaw, ok := a.runtime.GetChannelValue(gate.StableTimeChannel.DeviceID, gate.StableTimeChannel.ChannelIndex)
		stableTimeSec, err := ParseSphereTankStableTimeSec(stableRaw, ok)
		if err != nil {
			// 读取失败，继续等待
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if IsSphereTankGateSatisfied(gate, stableTimeSec) {
			return nil
		}

		if time.Now().UnixMilli()-gateWaitStartAt > int64(maxWaitMs) {
			a.isRunning = false
			return fmt.Errorf("球罐判定等待超时（%d 秒）", maxWaitMs/1000)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// waitWhilePaused 等待暂停恢复
func (a *AutomaticCalibration) waitWhilePaused() {
	for a.isPaused && a.isRunning {
		time.Sleep(100 * time.Millisecond)
	}
}

// makeChannelReader 创建通道读取函数
func (a *AutomaticCalibration) makeChannelReader() ChannelValueReader {
	return func(deviceID string, channelIndex int) (float64, bool) {
		if a.runtime == nil {
			return 0, false
		}
		return a.runtime.GetChannelValue(deviceID, channelIndex)
	}
}

// findAxisConfig 查找逻辑轴名对应的运动轴配置
func (a *AutomaticCalibration) findAxisConfig(axisName string) *MotionAxisConfig {
	normalized := normalizeAxisName(axisName)
	for i := range a.config.MotionAxes {
		if normalizeAxisName(a.config.MotionAxes[i].Name) == normalized {
			return &a.config.MotionAxes[i]
		}
	}
	return nil
}

// normalizeAxisName 标准化轴名
func normalizeAxisName(name string) string {
	switch name {
	case "alpha", "Alpha", "ALPHA", "a", "A":
		return "α"
	case "beta", "Beta", "BETA", "b", "B":
		return "β"
	case "theta", "Theta", "THETA", "th", "TH":
		return "θ"
	default:
		return name
	}
}

// Pause 暂停校准
func (a *AutomaticCalibration) Pause() {
	if !a.isRunning {
		return
	}
	a.isPaused = true
	log.Printf("[AutomaticCalibration] 已暂停")
}

// Resume 恢复校准
func (a *AutomaticCalibration) Resume() {
	if !a.isRunning {
		return
	}
	a.isPaused = false
	log.Printf("[AutomaticCalibration] 已恢复")
}

// Stop 停止校准
func (a *AutomaticCalibration) Stop() {
	a.isRunning = false
	a.isPaused = false
	log.Printf("[AutomaticCalibration] 已停止")
}

// IsRunning 是否正在运行
func (a *AutomaticCalibration) IsRunning() bool {
	return a.isRunning
}

// IsPaused 是否暂停
func (a *AutomaticCalibration) IsPaused() bool {
	return a.isPaused
}

// GetDataPoints 获取已采集的数据点
func (a *AutomaticCalibration) GetDataPoints() []DataPoint {
	return a.dataPoints
}

// GetCurrentPointIndex 获取当前测点索引
func (a *AutomaticCalibration) GetCurrentPointIndex() int {
	return a.currentPointIdx
}

// GetStartTime 获取启动时间
func (a *AutomaticCalibration) GetStartTime() int64 {
	return a.startTime
}

// GetProgress 获取进度百分比
func (a *AutomaticCalibration) GetProgress() float64 {
	if len(a.config.Points) == 0 {
		return 0
	}
	return float64(len(a.dataPoints)) / float64(len(a.config.Points)) * 100
}

// sendProgressUpdate 发送进度更新事件
func (a *AutomaticCalibration) sendProgressUpdate(point CalPoint, dataPoint DataPoint) {
	if a.eventPublisher == nil {
		return
	}
	a.eventPublisher.OnProgress(ProgressEvent{
		TaskID:          a.taskID,
		WindowTag:       "calibration",
		CurrentPoint:    point,
		CompletedPoints: len(a.dataPoints),
		TotalPoints:     len(a.config.Points),
		LatestData:      dataPoint,
		Timestamp:       time.Now().UnixMilli(),
	})
}

// sendCompletionEvent 发送完成事件
func (a *AutomaticCalibration) sendCompletionEvent(success bool, errMsg string) {
	if a.eventPublisher == nil {
		return
	}
	duration := int64(0)
	if a.startTime > 0 {
		duration = time.Now().UnixMilli() - a.startTime
	}
	a.eventPublisher.OnComplete(CompleteEvent{
		TaskID:        a.taskID,
		WindowTag:     "calibration",
		Success:       success,
		Error:         errMsg,
		Duration:      duration,
		TotalPoints:   len(a.config.Points),
		SuccessPoints: len(a.dataPoints),
	})
}

// SendRealtimeUpdate 发送实时数据事件
func (a *AutomaticCalibration) SendRealtimeUpdate(event RealtimeEvent) {
	if a.eventPublisher == nil {
		return
	}
	event.TaskID = a.taskID
	event.WindowTag = "calibration"
	event.Timestamp = time.Now().UnixMilli()
	a.eventPublisher.OnRealtime(event)
}
