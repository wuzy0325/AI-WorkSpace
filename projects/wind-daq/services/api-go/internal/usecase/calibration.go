package usecase

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

const defaultSampleInterval = 50 * time.Millisecond

// CalibrationManager 校准管理器
// 管理校准任务的生命周期，协调自动校准引擎、采集协调器、CSV写入器等组件
type CalibrationManager struct {
	mu     sync.RWMutex
	reader ports.LatestDataReader
	motion ports.MotionManager
	sink   ports.CalibrationPointSink
	store  ports.CalibrationResultStore

	// 新增组件
	eventPublisher ports.CalibrationEventPublisher
	runtime        ports.CalibrationRuntime
	statusProvider ports.DeviceStatusProvider

	// 校准引擎
	autoEngine       *calibration.AutomaticCalibration
	currentConfig    calibration.Config
	currentStatus    calibration.Status
	currentTaskID    string
	csvWriter        ports.CalibrationCsvWriter
	csvWriterFactory func(calibration.Config) ports.CalibrationCsvWriter
	lastExport       *calibration.ExportPayload
}

func NewCalibrationManager(
	reader ports.LatestDataReader,
	motion ports.MotionManager,
	sink ports.CalibrationPointSink,
	store ports.CalibrationResultStore,
) *CalibrationManager {
	return &CalibrationManager{
		reader: reader,
		motion: motion,
		sink:   sink,
		store:  store,
		currentStatus: calibration.Status{
			State: calibration.StateIdle,
		},
	}
}

// SetEventPublisher 设置事件发布器
func (m *CalibrationManager) SetEventPublisher(p ports.CalibrationEventPublisher) {
	m.eventPublisher = p
}

// SetRuntime 设置校准运行时
func (m *CalibrationManager) SetRuntime(r ports.CalibrationRuntime) {
	m.runtime = r
}

// SetDeviceStatusProvider 设置设备状态查询
func (m *CalibrationManager) SetDeviceStatusProvider(p ports.DeviceStatusProvider) {
	m.statusProvider = p
}

// SetCsvWriter 设置校准 CSV 写入器
// CSV 写入器是字节 I/O 组件，由装配根（pkg/appcontext）注入，
// 避免 usecase 直接依赖 adapters/storage。
func (m *CalibrationManager) SetCsvWriter(w ports.CalibrationCsvWriter) {
	m.csvWriter = w
}

func (m *CalibrationManager) SetCsvWriterFactory(factory func(calibration.Config) ports.CalibrationCsvWriter) {
	m.csvWriterFactory = factory
}

// Start 启动校准任务
func (m *CalibrationManager) Start(config calibration.Config) error {
	if config.TaskID == "" {
		return fmt.Errorf("taskID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentStatus.State == calibration.StateRunning || m.currentStatus.State == calibration.StatePaused {
		return fmt.Errorf("校准任务已在运行中，请先停止")
	}

	// 兼容旧接口：当调用方仅提供 PressurePoints 而未提供 Points 时，
	// 将每个压力点转换为一个 CalPoint，Coordinates 使用 "pressure" 键。
	// 这样 TotalPoints 与前端期望一致，且 AutomaticCalibration 循环能正常遍历。
	if len(config.Points) == 0 && len(config.PressurePoints) > 0 {
		config.Points = make([]calibration.CalPoint, 0, len(config.PressurePoints))
		for i, p := range config.PressurePoints {
			config.Points = append(config.Points, calibration.CalPoint{
				ID:          i + 1,
				Coordinates: map[string]float64{"pressure": p},
			})
		}
	}
	if config.SamplesPerPoint <= 0 && config.AverageSamples > 0 {
		config.SamplesPerPoint = config.AverageSamples
	}
	if config.SamplesPerPoint <= 0 {
		config.SamplesPerPoint = 1
	}

	m.currentConfig = config
	m.currentTaskID = config.TaskID
	m.lastExport = nil

	// 创建事件发布适配器
	publisher := m.createEventPublisher()

	// 创建运行时适配器
	runtime := m.createRuntime()

	// CSV 实时写入：自动校准类型在 Start 时以覆盖模式初始化 csvWriter，
	// 每个点采集完成后通过 onDataPoint 回调逐点写入，崩溃/断电不丢已采集点。
	// 总温校准使用手动逐点采集，由 CollectCurrentPoint 直接调用 csvWriter。
	autoTypes := map[string]bool{
		string(calibration.TypeFiveHole):      true,
		string(calibration.TypeThreeHole):     true,
		string(calibration.TypeTotalPressure): true,
	}
	var onDataPoint calibration.DataPointSink
	if autoTypes[config.Type] && config.SavePath != "" && m.csvWriter != nil {
		if err := m.csvWriter.Initialize(config); err != nil {
			log.Printf("[CalibrationManager] CSV写入器初始化失败: %v", err)
		} else {
			writer := m.csvWriter
			onDataPoint = func(dp calibration.DataPoint) {
				if err := writer.AppendPoint(dp); err != nil {
					log.Printf("[CalibrationManager] 实时CSV写入失败: %v", err)
				}
			}
		}
	}

	// 创建自动校准引擎
	m.autoEngine = calibration.NewAutomaticCalibration(config, publisher, runtime, onDataPoint)
	m.autoEngine.SetTaskID(config.TaskID)

	// 更新状态
	m.currentStatus = calibration.Status{
		TaskID:      config.TaskID,
		Type:        config.Type,
		State:       calibration.StateRunning,
		TotalPoints: len(config.Points),
		StartTime:   time.Now().UnixMilli(),
	}

	// 根据校准类型选择算法并启动
	algorithm, err := m.createAlgorithm(config)
	if err != nil {
		return err
	}
	if len(config.ProbeChannels) > 0 {
		if err := algorithm.ValidateConfig(config); err != nil {
			return err
		}
	}

	// 异步启动校准循环
	go func() {
		err := m.autoEngine.Start(algorithm)
		m.mu.Lock()
		defer m.mu.Unlock()

		if err != nil {
			m.currentStatus.State = calibration.StateError
			m.currentStatus.LastError = err.Error()
		} else {
			m.currentStatus.State = calibration.StateCompleted
		}

		// 保存导出载荷，供 SaveCsv 按需写入
		dataPoints := m.autoEngine.GetDataPoints()
		m.lastExport = &calibration.ExportPayload{
			Type:       calibration.CalibrationType(config.Type),
			Config:     config,
			DataPoints: dataPoints,
		}
		m.currentStatus.DataPoints = dataPoints
		m.currentStatus.CompletedPoints = len(dataPoints)
		if m.currentStatus.TotalPoints > 0 {
			m.currentStatus.Progress = float64(len(dataPoints)) / float64(m.currentStatus.TotalPoints) * 100
		}

		// 保存结果
		if m.store != nil {
			if saveErr := m.store.Save(config.TaskID, m.currentStatus); saveErr != nil {
				log.Printf("[CalibrationManager] 保存校准结果失败: %v", saveErr)
			}
		}

		// 实时 CSV 写入完成：flush 并关闭文件句柄，下次 Start 可重新 Initialize。
		// 不置 nil：writer 由装配根注入一次，Flush 后 file 已关闭，可复用。
		if m.csvWriter != nil {
			if flushErr := m.csvWriter.Flush(); flushErr != nil {
				log.Printf("[CalibrationManager] CSV刷新失败: %v", flushErr)
			}
		}

		// 运动归零
		m.returnToHomePosition(config)
	}()

	return nil
}

// Pause 暂停校准
func (m *CalibrationManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentStatus.State != calibration.StateRunning {
		return fmt.Errorf("校准未在运行中")
	}

	if m.autoEngine != nil {
		m.autoEngine.Pause()
	}
	m.currentStatus.State = calibration.StatePaused
	return nil
}

// Resume 恢复校准
func (m *CalibrationManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentStatus.State != calibration.StatePaused {
		return fmt.Errorf("校准未在暂停状态")
	}

	if m.autoEngine != nil {
		m.autoEngine.Resume()
	}
	m.currentStatus.State = calibration.StateRunning
	return nil
}

// Stop 停止校准
func (m *CalibrationManager) Stop() error {
	m.mu.Lock()

	if m.autoEngine != nil {
		m.autoEngine.Stop()
	}

	m.currentStatus.State = calibration.StateStopped

	// 保存导出载荷
	if m.autoEngine != nil {
		dataPoints := m.autoEngine.GetDataPoints()
		m.lastExport = &calibration.ExportPayload{
			Type:       calibration.CalibrationType(m.currentConfig.Type),
			Config:     m.currentConfig,
			DataPoints: dataPoints,
		}
		m.currentStatus.DataPoints = dataPoints
		m.currentStatus.CompletedPoints = len(dataPoints)
		if m.currentStatus.TotalPoints > 0 {
			m.currentStatus.Progress = float64(len(dataPoints)) / float64(m.currentStatus.TotalPoints) * 100
		}
	}

	// 保存结果
	if m.store != nil {
		status := m.currentStatus
		m.mu.Unlock()
		if err := m.store.Save(m.currentConfig.TaskID, status); err != nil {
			return fmt.Errorf("保存校准结果失败: %v", err)
		}
	} else {
		m.mu.Unlock()
	}

	// 刷新CSV（总温手动采集模式）。不置 nil：writer 由装配根注入一次，
	// Flush 已关闭内部文件句柄，下次 Start 可再次 Initialize 打开新文件。
	if m.csvWriter != nil {
		if err := m.csvWriter.Flush(); err != nil {
			log.Printf("[CalibrationManager] CSV刷新失败: %v", err)
		}
	}

	// 停止运动
	m.stopMotion()

	return nil
}

// CollectCurrentPoint 手动采集当前工况点（总温校准专用）
func (m *CalibrationManager) CollectCurrentPoint() error {
	m.mu.Lock()
	if m.currentStatus.State != calibration.StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("校准未在运行中")
	}
	if m.reader == nil {
		m.mu.Unlock()
		return fmt.Errorf("数据读取器未配置")
	}
	config := m.currentConfig
	m.mu.Unlock()

	// 使用总温算法手动采集
	algorithm := calibration.NewTotalTemperatureAlgorithm()
	channelReader := m.makeChannelReader()

	pointIdx := 0
	m.mu.RLock()
	if m.autoEngine != nil {
		pointIdx = m.autoEngine.GetCurrentPointIndex()
	}
	m.mu.RUnlock()

	if pointIdx >= len(config.Points) {
		return fmt.Errorf("所有工况点已采集完成")
	}

	point := config.Points[pointIdx]
	sampleInterval := time.Duration(50) * time.Millisecond
	if config.TotalTemperatureConfig != nil && config.TotalTemperatureConfig.SampleInterval > 0 {
		sampleInterval = time.Duration(config.TotalTemperatureConfig.SampleInterval) * time.Millisecond
	}

	dataPoint, err := algorithm.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, sampleInterval)
	if err != nil {
		return m.fail("采集当前工况点失败: %v", err)
	}

	// 写入CSV
	if m.csvWriter != nil {
		if writeErr := m.csvWriter.AppendPoint(dataPoint); writeErr != nil {
			log.Printf("[CalibrationManager] CSV写入失败: %v", writeErr)
		}
	}

	m.mu.Lock()
	m.currentStatus.DataPoints = append(m.currentStatus.DataPoints, dataPoint)
	m.currentStatus.CompletedPoints = len(m.currentStatus.DataPoints)
	if m.currentStatus.TotalPoints > 0 {
		m.currentStatus.Progress = float64(m.currentStatus.CompletedPoints) / float64(m.currentStatus.TotalPoints) * 100
	}
	m.mu.Unlock()

	return nil
}

// ReacquirePoint 重新采集指定工况点
func (m *CalibrationManager) ReacquirePoint(index int) error {
	m.mu.RLock()
	if m.currentStatus.State != calibration.StateRunning {
		m.mu.RUnlock()
		return fmt.Errorf("校准未在运行中")
	}
	if index < 0 || index >= len(m.currentConfig.Points) {
		m.mu.RUnlock()
		return fmt.Errorf("工况点索引越界: %d", index)
	}
	config := m.currentConfig
	m.mu.RUnlock()

	algorithm := calibration.NewTotalTemperatureAlgorithm()
	channelReader := m.makeChannelReader()
	point := config.Points[index]

	sampleInterval := time.Duration(50) * time.Millisecond
	if config.TotalTemperatureConfig != nil && config.TotalTemperatureConfig.SampleInterval > 0 {
		sampleInterval = time.Duration(config.TotalTemperatureConfig.SampleInterval) * time.Millisecond
	}

	dataPoint, err := algorithm.ReacquirePoint(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, sampleInterval)
	if err != nil {
		return fmt.Errorf("重新采集工况点失败: %w", err)
	}

	// 替换数据点
	m.mu.Lock()
	if index < len(m.currentStatus.DataPoints) {
		m.currentStatus.DataPoints[index] = dataPoint
	}
	m.mu.Unlock()

	return nil
}

// GetExportPayload 获取导出数据
func (m *CalibrationManager) GetExportPayload() *calibration.ExportPayload {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastExport
}

func (m *CalibrationManager) SaveCsv(taskID string, savePath string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("taskID is required")
	}
	if savePath == "" {
		return "", fmt.Errorf("保存路径为空")
	}
	if m.csvWriterFactory == nil {
		return "", fmt.Errorf("CSV写入器未配置")
	}

	m.mu.RLock()
	var payload calibration.ExportPayload
	if m.lastExport != nil && m.lastExport.Config.TaskID == taskID {
		payload = *m.lastExport
	} else if m.currentConfig.TaskID == taskID {
		payload = calibration.ExportPayload{
			Type:       calibration.CalibrationType(m.currentConfig.Type),
			Config:     m.currentConfig,
			DataPoints: append([]calibration.DataPoint(nil), m.currentStatus.DataPoints...),
		}
	} else {
		m.mu.RUnlock()
		return "", fmt.Errorf("校准结果不存在: %s", taskID)
	}
	m.mu.RUnlock()

	if len(payload.DataPoints) == 0 {
		return "", fmt.Errorf("校准结果为空，无法保存CSV")
	}

	payload.Config.SavePath = savePath
	writer := m.csvWriterFactory(payload.Config)
	if writer == nil {
		return "", fmt.Errorf("CSV写入器未配置")
	}
	if err := writer.Initialize(payload.Config); err != nil {
		return "", err
	}
	for _, point := range payload.DataPoints {
		if err := writer.AppendPoint(point); err != nil {
			_ = writer.Flush()
			return "", err
		}
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}
	return writer.Path(), nil
}

// GetResult 获取校准结果
func (m *CalibrationManager) GetResult(taskID string) (calibration.Status, bool) {
	if m.store == nil {
		return calibration.Status{}, false
	}
	return m.store.Get(taskID)
}

// Status 获取当前状态
func (m *CalibrationManager) Status() calibration.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentStatus
}

// GetTotalTemperatureState 获取总温校准专用状态
func (m *CalibrationManager) GetTotalTemperatureState() *calibration.TotalTemperatureState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentConfig.Type != string(calibration.TypeTotalTemperature) {
		return nil
	}

	// 构建通道映射
	channels := make(map[string]calibration.ChannelRef)
	for _, ch := range m.currentConfig.ProbeChannels {
		if ch.Enabled {
			channels[ch.Role] = calibration.ChannelRef{
				DeviceID:     ch.DeviceID,
				ChannelIndex: ch.ChannelIndex,
			}
		}
	}

	algorithm := calibration.NewTotalTemperatureAlgorithm()
	channelReader := m.makeChannelReader()

	var targetMa float64
	if len(m.currentConfig.Points) > 0 {
		if ma, ok := m.currentConfig.Points[0].Coordinates["Ma"]; ok {
			targetMa = ma
		}
	}

	machTolerance := float64(0.01)
	if m.currentConfig.TotalTemperatureConfig != nil {
		machTolerance = m.currentConfig.TotalTemperatureConfig.MachTolerance
	}

	state, err := algorithm.GetState(channelReader, channels, targetMa, machTolerance)
	if err != nil {
		return nil
	}
	return state
}

// ==================== 内部方法 ====================

// createAlgorithm 根据校准类型创建算法实例
func (m *CalibrationManager) createAlgorithm(config calibration.Config) (calibration.Algorithm, error) {
	switch calibration.CalibrationType(config.Type) {
	case calibration.TypeFiveHole:
		return calibration.NewFiveHoleAlgorithm(), nil
	case calibration.TypeThreeHole:
		return calibration.NewThreeHoleAlgorithm(), nil
	case calibration.TypeTotalPressure:
		return calibration.NewTotalPressureAlgorithm(), nil
	case calibration.TypeTotalTemperature:
		return calibration.NewTotalTemperatureAlgorithm(), nil
	default:
		return nil, fmt.Errorf("未知校准类型: %s", config.Type)
	}
}

// createEventPublisher 创建事件发布适配器
func (m *CalibrationManager) createEventPublisher() calibration.EventPublisher {
	if m.eventPublisher == nil {
		return &noopEventPublisher{}
	}
	return &eventPublisherAdapter{publisher: m.eventPublisher}
}

// createRuntime 创建运行时适配器
func (m *CalibrationManager) createRuntime() calibration.RuntimeAccess {
	if m.runtime != nil {
		return &runtimeAdapter{runtime: m.runtime}
	}
	return &fallbackRuntime{reader: m.reader, motion: m.motion}
}

// makeChannelReader 创建通道读取函数
func (m *CalibrationManager) makeChannelReader() calibration.ChannelValueReader {
	return func(deviceID string, channelIndex int) (float64, bool) {
		if m.runtime != nil {
			return m.runtime.GetChannelValue(deviceID, channelIndex)
		}
		if m.reader == nil {
			return 0, false
		}
		payload, ok := m.reader.GetLatestData(deviceID)
		if !ok {
			return 0, false
		}
		return valuesForChannelIndex(payload, channelIndex), true
	}
}

// returnToHomePosition 运动归零
func (m *CalibrationManager) returnToHomePosition(config calibration.Config) {
	if m.motion == nil || len(config.MotionAxes) == 0 {
		return
	}

	ctx := context.Background()
	for _, axis := range config.MotionAxes {
		if err := m.motion.MoveTo(ctx, axis.ControllerID, motion.AxisName(axis.Axis), 0); err != nil {
			log.Printf("[CalibrationManager] 归零失败 %s/%s: %v", axis.ControllerID, axis.Axis, err)
		}
	}
}

// stopMotion 停止所有运动
func (m *CalibrationManager) stopMotion() {
	if m.motion == nil {
		return
	}

	ctx := context.Background()
	for _, status := range m.motion.StatusAll(ctx) {
		for _, axis := range status.Axes {
			if axis.Moving {
				if err := m.motion.Stop(ctx, status.ID, axis.Name); err != nil {
					log.Printf("[CalibrationManager] 停止运动失败 %s/%s: %v", status.ID, axis.Name, err)
				}
			}
		}
	}
}

// fail 设置错误状态
func (m *CalibrationManager) fail(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.currentStatus.State = calibration.StateError
	m.currentStatus.LastError = message
	m.mu.Unlock()
	return fmt.Errorf("%s", message)
}

// valuesForChannelIndex 从数据载荷中提取指定通道索引的值
func valuesForChannelIndex(payload device.DataPayload, channelIndex int) float64 {
	for i, idx := range payload.ChannelIndices {
		if idx == channelIndex && i < len(payload.Channels) {
			return payload.Channels[i]
		}
	}
	return 0
}

// ==================== 适配器类型 ====================

// eventPublisherAdapter 事件发布适配器
type eventPublisherAdapter struct {
	publisher ports.CalibrationEventPublisher
}

func (a *eventPublisherAdapter) OnProgress(event calibration.ProgressEvent) {
	a.publisher.PublishProgress(event)
}

func (a *eventPublisherAdapter) OnComplete(event calibration.CompleteEvent) {
	a.publisher.PublishComplete(event)
}

func (a *eventPublisherAdapter) OnRealtime(event calibration.RealtimeEvent) {
	a.publisher.PublishRealtime(event)
}

// noopEventPublisher 空事件发布器
type noopEventPublisher struct{}

func (n *noopEventPublisher) OnProgress(_ calibration.ProgressEvent) {}
func (n *noopEventPublisher) OnComplete(_ calibration.CompleteEvent) {}
func (n *noopEventPublisher) OnRealtime(_ calibration.RealtimeEvent) {}

// runtimeAdapter 运行时适配器
type runtimeAdapter struct {
	runtime ports.CalibrationRuntime
}

func (r *runtimeAdapter) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	return r.runtime.GetChannelValue(deviceID, channelIndex)
}

func (r *runtimeAdapter) MoveToPosition(axis calibration.MotionAxisConfig, position float64) error {
	return r.runtime.MoveToPosition(axis, position)
}

func (r *runtimeAdapter) WaitForMotionComplete() error {
	return r.runtime.WaitForMotionComplete()
}

// fallbackRuntime 回退运行时（使用旧的 reader 和 motion 接口）
type fallbackRuntime struct {
	reader ports.LatestDataReader
	motion ports.MotionManager
}

func (f *fallbackRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	if f.reader == nil {
		return 0, false
	}
	payload, ok := f.reader.GetLatestData(deviceID)
	if !ok {
		return 0, false
	}
	return valuesForChannelIndex(payload, channelIndex), true
}

func (f *fallbackRuntime) MoveToPosition(axis calibration.MotionAxisConfig, position float64) error {
	if f.motion == nil {
		return fmt.Errorf("运动控制器未配置")
	}
	if axis.ControllerID == "" {
		return fmt.Errorf("运动控制器ID未配置")
	}
	if axis.Axis == "" {
		return fmt.Errorf("运动轴未配置")
	}
	ctx := context.Background()
	for _, s := range f.motion.StatusAll(ctx) {
		if s.ID == axis.ControllerID && !s.Connected {
			return fmt.Errorf("运动控制器未连接: %s", axis.ControllerID)
		}
	}
	return f.motion.MoveTo(ctx, axis.ControllerID, motion.AxisName(axis.Axis), position)
}

func (f *fallbackRuntime) WaitForMotionComplete() error {
	// 简化实现：等待一段时间
	time.Sleep(500 * time.Millisecond)
	return nil
}
