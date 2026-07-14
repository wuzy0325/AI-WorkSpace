package calibration

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// RuntimeAccess 校准运行时依赖注入接口
// 由 CalibrationManager 注入，提供通道读取、运动控制等能力
type RuntimeAccess interface {
	GetChannelValue(deviceID string, channelIndex int) (float64, bool)
	GetLatestTimestamp(deviceID string) (int64, bool)
	MoveToPosition(axis MotionAxisConfig, position float64) error
	WaitForMotionComplete() error
	StopMotion() error
}

// ErrPointAborted 暂停打断当前测点的哨兵错误。
// runCalibrationLoop 识别此错误后回退循环索引以重跑同一点，
// 不计入 pointErrorCount。
var ErrPointAborted = errors.New("测点被暂停打断")

// ErrMotionControl marks motion failures that must stop the calibration run.
// Continuing to later points would only advance the displayed target while the
// probe remains at an unknown physical position.
var ErrMotionControl = errors.New("运动控制失败")

// AutomaticCalibration 自动循环校准引擎
// 提供 move → wait → gate → acquire → hook → push → next 的模板方法
// 五孔、三孔、总压校准使用此引擎
type AutomaticCalibration struct {
	mu              sync.RWMutex
	config          Config
	eventPublisher  EventPublisher
	runtime         RuntimeAccess
	onDataPoint     func(DataPoint) // 每个点采集完成后的回调（用于实时 CSV 写入等）
	taskID          string
	isRunning       bool
	isPaused        bool
	currentPointIdx int
	dataPoints      []DataPoint
	startTime       int64
	// 当前点采样进度（由算法采集循环通过 onSampleProgress 回调更新）
	// currentSample：当前点已采样本数（1..samplesPerPoint），0 表示尚未开始
	// samplesPerPoint：当前点总采样数，用于 UI 显示"采样 3/10"
	currentSample   int
	samplesPerPoint int
}

// NewAutomaticCalibration 创建自动校准引擎
// onDataPoint 可为 nil；非 nil 时每个点采集完成后同步调用，调用方负责持久化等 I/O。
func NewAutomaticCalibration(config Config, publisher EventPublisher, runtime RuntimeAccess, onDataPoint func(DataPoint)) *AutomaticCalibration {
	return &AutomaticCalibration{
		config:         config,
		eventPublisher: publisher,
		runtime:        runtime,
		onDataPoint:    onDataPoint,
		dataPoints:     make([]DataPoint, 0),
	}
}

// SetTaskID 设置任务ID
func (a *AutomaticCalibration) SetTaskID(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.taskID = id
}

// Start 启动自动校准循环
func (a *AutomaticCalibration) Start(algorithm Algorithm) error {
	a.mu.Lock()
	if a.isRunning {
		a.mu.Unlock()
		return fmt.Errorf("校准已在运行中")
	}

	a.isRunning = true
	a.isPaused = false
	a.currentPointIdx = 0
	a.dataPoints = make([]DataPoint, 0)
	a.startTime = time.Now().UnixMilli()
	a.config.TimestampReader = a.makeTimestampReader()
	a.mu.Unlock()

	log.Printf("[AutomaticCalibration] 启动校准，共 %d 个测点", len(a.config.Points))

	err := a.runCalibrationLoop(algorithm)

	a.mu.Lock()
	a.isRunning = false
	a.mu.Unlock()
	return err
}

// runCalibrationLoop 校准主循环（模板方法）
func (a *AutomaticCalibration) runCalibrationLoop(algorithm Algorithm) error {
	var pointErrorCount int
	var lastPointError string

	for i := a.GetCurrentPointIndex(); i < len(a.config.Points); i++ {
		if !a.IsRunning() {
			log.Printf("[AutomaticCalibration] 校准被用户停止")
			return nil
		}

		// 等待暂停恢复
		a.waitWhilePaused()

		if !a.IsRunning() {
			return nil
		}

		a.mu.Lock()
		a.currentPointIdx = i
		a.mu.Unlock()
		point := a.config.Points[i]

		log.Printf("[AutomaticCalibration] 处理测点 %d/%d, 坐标: %v", i+1, len(a.config.Points), point.Coordinates)

		if err := a.processPoint(algorithm, point, i); err != nil {
			// 暂停打断：回退索引以重跑同一点，不计入错误计数。
			// 循环顶部会再次调用 waitWhilePaused 阻塞到恢复，无需在此重复等待。
			if errors.Is(err, ErrPointAborted) {
				log.Printf("[AutomaticCalibration] 测点 %d 被暂停打断，等待恢复后重跑", i+1)
				if !a.IsRunning() {
					return nil
				}
				i-- // 抵消循环自增，重跑该点
				continue
			}
			if errors.Is(err, ErrMotionControl) {
				return fmt.Errorf("测点 %d 运动失败: %w", i+1, err)
			}

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
	// 2. 移动到点位
	if err := a.moveToPoint(point, algorithm); err != nil {
		return fmt.Errorf("%w: 移动到测点失败: %w", ErrMotionControl, err)
	}

	// 移动完成后检查暂停——运动可能在过程中被 Pause 打断，
	// StopMotion 会使 WaitForMotionComplete 提前返回，此处捕获暂停态。
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 2. 等待驻留时间（让压力稳定）
	if a.config.DwellTimeMs > 0 {
		time.Sleep(time.Duration(a.config.DwellTimeMs) * time.Millisecond)
	}

	// 驻留后检查暂停
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 3. 等待球罐闸门条件（如果启用）
	if err := a.waitForSphereTankGateIfNeeded(); err != nil {
		return fmt.Errorf("球罐闸门等待失败: %w", err)
	}

	// 闸门条件满足后、采集前检查暂停
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 4. 采集数据
	channelReader := a.makeChannelReader()
	checkAbort := func() bool {
		return !a.IsRunning() || a.IsPaused()
	}
	// 采样进度回调：算法每次采完一个样本调用，更新 AutomaticCalibration 的共享状态，
	// 供 Status() 查询路径读取，驱动前端"当前点采样 i+1/N"显示。
	// 采集开始前先重置为 0，避免显示上一点的残留值。
	a.mu.Lock()
	a.currentSample = 0
	a.samplesPerPoint = a.config.SamplesPerPoint
	a.mu.Unlock()
	onSampleProgress := func(current, total int) {
		a.mu.Lock()
		a.currentSample = current
		a.samplesPerPoint = total
		a.mu.Unlock()
	}
	dataPoint, err := algorithm.AcquireDataWithConfig(point, channelReader, a.config, checkAbort, onSampleProgress)
	// 采集完成后立即清零采样进度，避免移动/驻留/球罐等待期间 UI 仍显示"采样 N/N"
	a.mu.Lock()
	a.currentSample = 0
	a.mu.Unlock()
	if err != nil {
		return fmt.Errorf("数据采集失败: %w", err)
	}

	// 5. 保存数据点
	a.mu.Lock()
	a.dataPoints = append(a.dataPoints, dataPoint)
	a.mu.Unlock()

	// 6. 实时持久化回调（逐点写 CSV 等），失败仅记录不中断校准
	if a.onDataPoint != nil {
		a.onDataPoint(dataPoint)
	}

	// 7. 发送进度更新
	a.sendProgressUpdate(point, dataPoint)

	return nil
}

// checkPausedAndAbort 在点位流程阶段切换处检查暂停态。
// 若已暂停：调用 runtime.StopMotion() 确保运动已停止（幂等），返回 errPointAborted。
// 调用方据此中止当前点并回退循环索引以重跑该点。
func (a *AutomaticCalibration) checkPausedAndAbort() error {
	a.mu.RLock()
	paused := a.isPaused
	a.mu.RUnlock()
	if !paused {
		return nil
	}
	// 已暂停，确保运动停止（Pause 路径通常已下发，此处为防御性幂等调用）。
	if a.runtime != nil {
		_ = a.runtime.StopMotion()
	}
	return ErrPointAborted
}

// moveToPoint 移动到指定点位
// 默认实现按坐标顺序移动各轴；五孔探针等子类可重写以自定义轴运动顺序
func (a *AutomaticCalibration) moveToPoint(point CalPoint, algorithm Algorithm) error {
	if algorithm != nil && algorithm.Type() == TypeFiveHole {
		return a.MoveToPointWithOrder(point, []string{"α", "β"})
	}
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
		if err := a.runtime.MoveToPosition(*axisConfig, position); err != nil {
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
		if err := a.runtime.MoveToPosition(*axisConfig, position); err != nil {
			return fmt.Errorf("移动 %s 轴到 %v 失败: %w", axisName, position, err)
		}
	}

	if err := a.runtime.WaitForMotionComplete(); err != nil {
		return fmt.Errorf("等待运动完成超时: %w", err)
	}

	return nil
}

// waitForSphereTankGateIfNeeded 等待球罐闸门条件
//
// 总超时取 gate.TimeoutSec，<=0 时使用默认 300 秒（5 分钟）。
// 超时后停止校准并返回错误，避免无限等待卡死整个流程。
func (a *AutomaticCalibration) waitForSphereTankGateIfNeeded() error {
	gate := NormalizeSphereTankGateConfig(a.config)
	if gate == nil || !gate.Enabled {
		return nil
	}

	if err := ValidateSphereTankGateConfig(gate); err != nil {
		return err
	}

	// 球罐判定总超时（秒）。0 表示使用默认 300 秒。
	maxWaitSec := gate.TimeoutSec
	if maxWaitSec <= 0 {
		maxWaitSec = 300
	}
	maxWaitMs := maxWaitSec * 1000
	gateWaitStartAt := time.Now().UnixMilli()

	for a.IsRunning() {
		// 暂停时等待
		a.waitWhilePaused()
		if !a.IsRunning() {
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
			a.Stop()
			return fmt.Errorf("球罐判定等待超时（%d 秒）", maxWaitSec)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// waitWhilePaused 等待暂停恢复
func (a *AutomaticCalibration) waitWhilePaused() {
	for {
		a.mu.RLock()
		paused := a.isPaused
		running := a.isRunning
		a.mu.RUnlock()
		if !paused || !running {
			return
		}
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

// makeTimestampReader 创建设备时间戳读取函数
func (a *AutomaticCalibration) makeTimestampReader() TimestampReader {
	return func(deviceID string) (int64, bool) {
		if a.runtime == nil {
			return 0, false
		}
		return a.runtime.GetLatestTimestamp(deviceID)
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
//
// 立即下发运动停止命令（普通 Stop，非急停），打断当前点位的运动；
// 当前正在执行的测点被视为未完成，恢复时由 runCalibrationLoop 回退索引重跑该点。
// 点间暂停（当前点已完成）时不影响已采集数据。
func (a *AutomaticCalibration) Pause() {
	a.mu.Lock()
	if !a.isRunning {
		a.mu.Unlock()
		return
	}
	a.isPaused = true
	runtime := a.runtime
	a.mu.Unlock()

	// 立即停止运动（幂等：点间暂停时无运动轴也会返回 nil）
	if runtime != nil {
		if err := runtime.StopMotion(); err != nil {
			log.Printf("[AutomaticCalibration] 暂停时停止运动失败: %v", err)
		}
	}
	log.Printf("[AutomaticCalibration] 已暂停")
}

// Resume 恢复校准
func (a *AutomaticCalibration) Resume() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.isRunning {
		return
	}
	a.isPaused = false
	log.Printf("[AutomaticCalibration] 已恢复")
}

// Stop 停止校准
func (a *AutomaticCalibration) Stop() {
	a.mu.Lock()
	a.isRunning = false
	a.isPaused = false
	a.mu.Unlock()
	log.Printf("[AutomaticCalibration] 已停止")
}

// IsRunning 是否正在运行
func (a *AutomaticCalibration) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.isRunning
}

// IsPaused 是否暂停
func (a *AutomaticCalibration) IsPaused() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.isPaused
}

// GetDataPoints 获取已采集的数据点
func (a *AutomaticCalibration) GetDataPoints() []DataPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]DataPoint(nil), a.dataPoints...)
}

// GetCurrentPointIndex 获取当前测点索引
func (a *AutomaticCalibration) GetCurrentPointIndex() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentPointIdx
}

// GetSampleProgress 获取当前点采样进度（currentSample, samplesPerPoint）
// 供 usecase 层 Status() 查询路径读取驱动前端"当前点采样 i/N"显示。
// currentSample=0 表示当前点尚未开始采集或已采集完成（下一轮 processPoint 开头会重置）。
func (a *AutomaticCalibration) GetSampleProgress() (int, int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentSample, a.samplesPerPoint
}

// GetStartTime 获取启动时间
func (a *AutomaticCalibration) GetStartTime() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.startTime
}

// GetProgress 获取进度百分比
func (a *AutomaticCalibration) GetProgress() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
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

	a.mu.RLock()
	taskID := a.taskID
	completedPoints := len(a.dataPoints)
	a.mu.RUnlock()

	a.eventPublisher.OnProgress(ProgressEvent{
		TaskID:          taskID,
		WindowTag:       "calibration",
		CurrentPoint:    point,
		CompletedPoints: completedPoints,
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

	a.mu.RLock()
	startTime := a.startTime
	taskID := a.taskID
	successPoints := len(a.dataPoints)
	a.mu.RUnlock()

	duration := int64(0)
	if startTime > 0 {
		duration = time.Now().UnixMilli() - startTime
	}
	a.eventPublisher.OnComplete(CompleteEvent{
		TaskID:        taskID,
		WindowTag:     "calibration",
		Success:       success,
		Error:         errMsg,
		Duration:      duration,
		TotalPoints:   len(a.config.Points),
		SuccessPoints: successPoints,
	})
}

// SendRealtimeUpdate 发送实时数据事件
func (a *AutomaticCalibration) SendRealtimeUpdate(event RealtimeEvent) {
	if a.eventPublisher == nil {
		return
	}
	a.mu.RLock()
	taskID := a.taskID
	a.mu.RUnlock()
	event.TaskID = taskID
	event.WindowTag = "calibration"
	event.Timestamp = time.Now().UnixMilli()
	a.eventPublisher.OnRealtime(event)
}
