package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// resolveSavePath 归一化校准 CSV 保存路径，保证行为确定性。
//
// 规则：
//   - 空路径原样返回（由调用方决定是否允许空）
//   - 绝对路径：filepath.Clean 规整分隔符与 ".."
//   - 相对路径：filepath.Abs 基于当前工作目录转绝对，避免 os.MkdirAll
//     在不同工作目录下创建到不同位置（行为不稳定）
//
// 这是所有调用入口（Wails backend、HTTP API）的统一防御性兜底。
// Wails backend 会先做 ResolvePath（相对路径→%APPDATA%\wind-daq\<相对>），
// 此时路径已是绝对，本函数仅做 Clean，无副作用。
func resolveSavePath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Abs(p)
}

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

	// 归一化保存路径：相对路径转绝对，避免 csv_writer 的 os.MkdirAll
	// 基于不确定的工作目录创建目录。空路径保留（自动校准类型允许为空，
	// 表示不实时落盘 CSV）。
	if config.SavePath != "" {
		resolved, err := resolveSavePath(config.SavePath)
		if err != nil {
			return fmt.Errorf("解析保存路径失败: %w", err)
		}
		config.SavePath = resolved
	}

	m.currentConfig = config
	m.currentTaskID = config.TaskID
	m.lastExport = nil

	// 创建事件发布适配器
	publisher := m.createEventPublisher()

	// 运动安全配置校验：非法阈值/未绑定轴在启动前拒绝，避免运动中才发现配置错误
	if err := validateCalibrationMotionSafetyConfig(config.MotionSafety, config.MotionAxes); err != nil {
		return fmt.Errorf("运动安全配置校验失败: %w", err)
	}

	// 创建运行时适配器（注入运动安全配置 + isPaused 回调）
	runtime := m.createRuntime(config.MotionSafety)

	// CSV 实时写入：自动校准类型在 Start 时以覆盖模式初始化 csvWriter，
	// 每个点采集完成后通过 onDataPoint 回调逐点写入，崩溃/断电不丢已采集点。
	// 总温校准使用手动逐点采集，由 CollectCurrentPoint 直接调用 csvWriter。
	autoTypes := map[string]bool{
		string(calibration.TypeFiveHole):      true,
		string(calibration.TypeThreeHole):     true,
		string(calibration.TypeTotalPressure): true,
	}
	var csvPointSink calibration.DataPointSink
	if autoTypes[config.Type] && config.SavePath != "" && m.csvWriter != nil {
		if err := m.csvWriter.Initialize(config); err != nil {
			log.Printf("[CalibrationManager] CSV写入器初始化失败: %v", err)
		} else {
			writer := m.csvWriter
			csvPointSink = func(dp calibration.DataPoint) {
				if err := writer.AppendPoint(dp); err != nil {
					log.Printf("[CalibrationManager] 实时CSV写入失败: %v", err)
				}
			}
		}
	}

	// 总温校准走手动 CollectCurrentPoint 路径（autoTypes 不含它），
	// 但同样需要在 Start 时 Initialize csvWriter 打开文件并写表头，
	// 否则后续 CollectCurrentPoint 调用 AppendPoint 会因 writer 未初始化
	// 直接返回 "CSV写入器未初始化" 错误，CSV 文件不会被创建。
	// Initialize 失败（路径不可写/磁盘满）时直接返回错误，让 Start 失败而非
	// 让用户在 CollectCurrentPoint 时才发现 CSV 写不进去——后者会让校准员误以为
	// 已采集的数据已落盘，实际全部丢失。
	if config.Type == string(calibration.TypeTotalTemperature) && config.SavePath != "" && m.csvWriter != nil {
		if err := m.csvWriter.Initialize(config); err != nil {
			return fmt.Errorf("总温校准 CSV 写入器初始化失败: %w", err)
		}
	}

	var onDataPoint calibration.DataPointSink
	if autoTypes[config.Type] {
		onDataPoint = func(dp calibration.DataPoint) {
			m.mu.Lock()
			if m.currentStatus.TaskID == config.TaskID {
				m.currentStatus.DataPoints = append(m.currentStatus.DataPoints, dp)
				m.currentStatus.CompletedPoints = len(m.currentStatus.DataPoints)
				// CurrentPoint 不在此处设置：它表示"当前正在处理的点索引"，
				// 由 Status() 从 autoEngine.GetCurrentPointIndex() 实时读取。
				// currentPointIdx 在 processPoint 循环顶部推进（早于 moveToPoint），
				// 让前端"目标角度"先于"实际角度"变化，符合校准员"目标先行"的直觉。
				if m.currentStatus.TotalPoints > 0 {
					m.currentStatus.Progress = float64(m.currentStatus.CompletedPoints) / float64(m.currentStatus.TotalPoints) * 100
				}
			}
			m.mu.Unlock()

			if csvPointSink != nil {
				csvPointSink(dp)
			}
		}
	}

	// 创建自动校准引擎（注入运动安全故障回调，引擎层通过回调委托 Manager 执行急停 + 状态写入）
	onMotionSafetyFailure := func(failure *traversal.MotionSafetyFailure) error {
		return m.handleCalibrationMotionSafetyFailure(runtime, failure)
	}
	m.autoEngine = calibration.NewAutomaticCalibration(config, publisher, runtime, onDataPoint, onMotionSafetyFailure)
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

		if m.currentStatus.State != calibration.StateStopped {
			if err != nil {
				m.currentStatus.State = calibration.StateError
				m.currentStatus.LastError = err.Error()
			} else {
				m.currentStatus.State = calibration.StateCompleted
			}
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
//
// 锁内只做状态校验与切换，autoEngine.Pause()（含 runtime.StopMotion() 硬件下发）
// 移到锁外执行，避免硬件通信阻塞时长时间持有 m.mu 阻塞 Status/Start 等查询路径。
func (m *CalibrationManager) Pause() error {
	m.mu.Lock()
	if m.currentStatus.State != calibration.StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("校准未在运行中")
	}
	m.currentStatus.State = calibration.StatePaused
	engine := m.autoEngine
	m.mu.Unlock()

	if engine != nil {
		engine.Pause()
	}
	return nil
}

// Resume 恢复校准
//
// 同 Pause：锁内只切状态，autoEngine.Resume() 移到锁外执行。
func (m *CalibrationManager) Resume() error {
	m.mu.Lock()
	if m.currentStatus.State != calibration.StatePaused {
		m.mu.Unlock()
		return fmt.Errorf("校准未在暂停状态")
	}
	m.currentStatus.State = calibration.StateRunning
	engine := m.autoEngine
	m.mu.Unlock()

	if engine != nil {
		engine.Resume()
	}
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

	// 停止运动（用户主动停止路径，错误仅记录不影响流程）
	if err := m.stopMotion(); err != nil {
		log.Printf("[CalibrationManager] Stop 停止运动失败: %v", err)
	}

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

	dataPoint, err := algorithm.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, sampleInterval, nil, m.makeTimestampReader(), nil)
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

	// 归一化保存路径：相对路径转绝对，保证 csv_writer 写入位置确定
	resolvedPath, err := resolveSavePath(savePath)
	if err != nil {
		return "", fmt.Errorf("解析保存路径失败: %w", err)
	}
	savePath = resolvedPath

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
	status := m.currentStatus
	if status.DataPoints != nil {
		status.DataPoints = append([]calibration.DataPoint(nil), status.DataPoints...)
	}
	m.mu.RUnlock()
	// 附加当前点采样进度与当前目标点索引：从 autoEngine 读取算法采集循环实时更新的状态。
	// autoEngine 为 nil（未启动/总温手动模式）时跳过。
	//
	// CurrentPoint 用 currentPointIdx（循环顶部推进）而非 CompletedPoints：
	// currentPointIdx 在 processPoint 循环顶部就推进，早于 moveToPoint，
	// 前端据此显示"目标角度"，能在运动控制器移动前就更新到下一个目标点，
	// 符合校准员"目标先行于实际"的直觉。CompletedPoints 仍代表"已完成采集的点数"。
	if m.autoEngine != nil {
		current, total := m.autoEngine.GetSampleProgress()
		status.CurrentSample = current
		status.SamplesPerPoint = total
		status.CurrentPoint = m.autoEngine.GetCurrentPointIndex()
	}
	return status
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
//
// 参数 motionSafety 为运动安全配置（来自 config.MotionSafety），注入到 fallbackRuntime
// 用于运动安全循环中的到位容差/严重偏离/看门狗判定。为 nil 时下游使用默认值。
// isPaused 回调延迟绑定到 m.autoEngine（autoEngine 在本函数返回后创建，
// 闭包捕获 m 引用，运行时读取 m.autoEngine.IsPaused()）。
func (m *CalibrationManager) createRuntime(motionSafety *traversal.MotionSafetyConfig) calibration.RuntimeAccess {
	if m.runtime != nil {
		return &runtimeAdapter{runtime: m.runtime}
	}
	return &fallbackRuntime{
		reader:       m.reader,
		motion:       m.motion,
		motionSafety: motionSafety,
		isPaused: func() bool {
			m.mu.RLock()
			engine := m.autoEngine
			m.mu.RUnlock()
			return engine != nil && engine.IsPaused()
		},
	}
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

// makeTimestampReader 创建设备时间戳读取函数
func (m *CalibrationManager) makeTimestampReader() calibration.TimestampReader {
	return func(deviceID string) (int64, bool) {
		if m.runtime != nil {
			return m.runtime.GetLatestTimestamp(deviceID)
		}
		if m.reader == nil {
			return 0, false
		}
		return m.reader.GetLatestTimestamp(deviceID)
	}
}

// validateCalibrationMotionSafetyConfig 校准模块运动安全配置校验入口。
//
// 校准模块使用 calibration.MotionAxisConfig（带逻辑名 Name），而 traversal
// 的 validateMotionSafetyConfig 期望 traversal.MotionAxisBinding（仅 ControllerID/Axis）。
// 本函数负责类型转换后委托给 traversal 模块的统一校验逻辑，避免重复实现校验规则。
//
// 校验规则（详见 traversal_config.go::validateMotionSafetyConfig）：
//  1. 浮点字段有限（非 NaN/Inf）
//  2. ArrivalTolerance > 0、ProgressEpsilon > 0
//  3. CriticalDeviationLimit > ArrivalTolerance（合并后跨字段校验）
//  4. NoProgressTimeoutMs >= 2*轮询周期
//  5. AxisOverrides 键必须为已绑定轴名
//
// cfg 为 nil 时直接返回 nil（旧配置兼容，下游使用默认值）。
func validateCalibrationMotionSafetyConfig(cfg *traversal.MotionSafetyConfig, motionAxes []calibration.MotionAxisConfig) error {
	if cfg == nil {
		return nil
	}
	bindings := make([]traversal.MotionAxisBinding, 0, len(motionAxes))
	for _, a := range motionAxes {
		if a.Axis == "" {
			continue
		}
		bindings = append(bindings, traversal.MotionAxisBinding{
			ControllerID: a.ControllerID,
			Axis:         a.Axis,
		})
	}
	return validateMotionSafetyConfig(cfg, bindings)
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

// stopMotion 停止所有运动并返回第一处错误。
//
// 返回 error 而非 void：handleCalibrationMotionSafetyFailure 需要据此
// 区分"停止成功"与"停止也失败"两种场景，前者走标准故障错误链，
// 后者需附加 stop 错误到错误链以暴露根因。旧 void 调用方（Stop）
// 直接忽略返回值即可。
func (m *CalibrationManager) stopMotion() error {
	if m.motion == nil {
		return nil
	}
	return stopAllMotion(m.motion)
}

// fail 设置错误状态（薄包装，向后兼容既有调用方；内部委托 failWithCode 写入空错误码）
func (m *CalibrationManager) fail(format string, args ...any) error {
	return m.failWithCode(format, "", args...)
}

// failWithCode 设置错误状态并写入结构化错误码。
//
// 行为：
//   - 设置 StateError + LastError(format 格式化结果) + LastErrorCode(code)
//   - 清空 MotionSafetyFailure（避免非运动安全错误路径残留快照；运动安全路径在 failWithCode 之后单独调用 recordMotionSafetyFailure 写入）
//   - 返回格式化后的 error
//
// 注意：与遍历测试 failWithCode 签名一致 (format, code, args...)，便于跨模块维护。
func (m *CalibrationManager) failWithCode(format string, code string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.currentStatus.State = calibration.StateError
	m.currentStatus.LastError = message
	m.currentStatus.LastErrorCode = code
	// 清空运动安全故障快照：failWithCode 是所有错误路径的公共出口，
	// 非运动安全错误路径不应残留上一次的故障快照。
	// 运动安全路径在 failWithCode 之后调用 recordMotionSafetyFailure 重新写入。
	m.currentStatus.MotionSafetyFailure = nil
	taskID := m.currentStatus.TaskID
	m.mu.Unlock()

	log.Printf("[CalibrationManager] 校准失败 taskID=%s code=%s err=%s", taskID, code, message)
	return fmt.Errorf("%s", message)
}

// recordMotionSafetyFailure 写入运动安全故障现场快照（不影响 state/lastError）。
//
// 由 handleCalibrationMotionSafetyFailure 在 failWithCode 之后调用，
// 将故障现场写入 currentStatus.MotionSafetyFailure，供前端轮询展示。
// 必须在 failWithCode 之后调用——failWithCode 会清空该字段。
func (m *CalibrationManager) recordMotionSafetyFailure(failure *traversal.MotionSafetyFailure) {
	if failure == nil {
		return
	}
	// 拷贝一份避免外部修改影响 status 中的快照
	snapshot := *failure
	m.mu.Lock()
	m.currentStatus.MotionSafetyFailure = &snapshot
	m.mu.Unlock()
}

// handleCalibrationMotionSafetyFailure 运动安全故障处理（由引擎层通过 onMotionSafetyFailure 回调调用）。
//
// 处理步骤：
//  1. 急停类裁决（CriticalDeviation/LimitTriggered/StatusUnavailable）：
//     通过 ports.EmergencyStopProvider 类型断言获取急停能力；
//     可用则 EmergencyStopMotion；不可用或失败时 fallback 到 StopMotion。
//  2. 普通停止类裁决（Deviation/NoProgress/Overshoot）：StopMotion。
//  3. failWithCode 写入 StateError + LastError + LastErrorCode。
//  4. recordMotionSafetyFailure 写入故障现场快照（在 failWithCode 之后，避免被清空）。
//
// 返回值：运动安全故障错误，引擎层据此终止校准（ErrMotionControl 语义）。
func (m *CalibrationManager) handleCalibrationMotionSafetyFailure(runtime calibration.RuntimeAccess, failure *traversal.MotionSafetyFailure) error {
	if failure == nil {
		return nil
	}

	deviation := failure.Actual - failure.Target
	log.Printf("[CalibrationManager] 运动安全故障 controller=%s axis=%s verdict=%s target=%.3f actual=%.3f deviation=%.3f pointIndex=%d requiresEmergencyStop=%v",
		failure.ControllerID, failure.Axis, failure.Verdict,
		failure.Target, failure.Actual, deviation, failure.PointIndex,
		failure.Verdict.RequiresEmergencyStop())

	// 1. 急停类裁决 → EmergencyStopProvider 类型断言
	var stopErr error
	if failure.Verdict.RequiresEmergencyStop() {
		if es, ok := runtime.(ports.EmergencyStopProvider); ok {
			stopErr = es.EmergencyStopMotion()
			if stopErr != nil {
				// 急停失败：错误码升级为 ErrEmergencyStopFailed，fallback 到 StopMotion
				log.Printf("[CalibrationManager] 急停失败，回退到普通停止: %v", stopErr)
				_ = runtime.StopMotion()
				m.failWithCode("motion safety failure (verdict=%s axis=%s target=%.3f actual=%.3f) and emergency stop also failed: %v",
					string(traversal.ErrEmergencyStopFailed),
					failure.Verdict, failure.Axis, failure.Target, failure.Actual, stopErr)
				// 急停失败路径同样写入故障快照：前端需要据此区分"急停调用失败"场景
				m.recordMotionSafetyFailure(failure)
				return fmt.Errorf("emergency stop failed after %s: %w", failure.Verdict, stopErr)
			}
		} else {
			// runtime 不支持急停，fallback 到 StopMotion
			log.Printf("[CalibrationManager] runtime 不支持急停，回退到普通停止")
			stopErr = runtime.StopMotion()
		}
	} else {
		// 2. 普通停止类 → StopMotion
		stopErr = runtime.StopMotion()
	}

	// 3. 写入结构化错误 + 4. 写入故障现场快照（必须在 failWithCode 之后）
	errorCode := traversal.ErrorCodeFor(failure.Verdict)
	m.failWithCode("motion safety failure: verdict=%s axis=%s target=%.3f actual=%.3f deviation=%.3f",
		string(errorCode),
		failure.Verdict, failure.Axis, failure.Target, failure.Actual, deviation)
	m.recordMotionSafetyFailure(failure)

	if stopErr != nil {
		return fmt.Errorf("motion safety %s (stop also failed: %w)", failure.Verdict, stopErr)
	}
	return fmt.Errorf("motion safety %s on axis %s (target=%.3f actual=%.3f)",
		failure.Verdict, failure.Axis, failure.Target, failure.Actual)
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
//
// 包装 ports.CalibrationRuntime（外部注入，常为 Wails binding 适配器），
// 桥接到 calibration.RuntimeAccess（引擎层期望的接口）。
//
// 接口适配策略：
//   - WaitForMotionComplete：通过 ports.MotionSafetyAwareRuntime 类型断言判断
//     被包装对象是否提供运动安全感知能力。支持时透传三元组；不支持时
//     fallback 到旧 WaitForMotionComplete() error，错误映射为 (false, timeout, nil)。
//   - EmergencyStopMotion：始终实现，内部类型断言被包装对象是否支持
//     ports.EmergencyStopProvider。支持时透传；不支持时 fallback 到 StopMotion。
type runtimeAdapter struct {
	runtime ports.CalibrationRuntime
}

func (r *runtimeAdapter) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	return r.runtime.GetChannelValue(deviceID, channelIndex)
}

func (r *runtimeAdapter) GetLatestTimestamp(deviceID string) (int64, bool) {
	return r.runtime.GetLatestTimestamp(deviceID)
}

func (r *runtimeAdapter) MoveToPosition(axis calibration.MotionAxisConfig, position float64) error {
	return r.runtime.MoveToPosition(axis, position)
}

// WaitForMotionComplete 返回运动等待结果的三元组（completed, reason, failure）。
//
// 若被包装的 runtime 实现 ports.MotionSafetyAwareRuntime，透传其三元组语义；
// 否则 fallback 到旧 WaitForMotionComplete() error：
//   - nil  → (true, MotionInterruptNone, nil) 表示已到位
//   - err  → (false, MotionInterruptTimeout, nil) 表示等待失败但无故障快照
//
// fallback 映射为 Timeout 而非 None：旧实现返回 error 通常代表超时，
// 引擎层据此走 ErrMotionControl 终止路径，符合"无法确认到位则不再继续"的安全语义。
func (r *runtimeAdapter) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	if safety, ok := r.runtime.(ports.MotionSafetyAwareRuntime); ok {
		return safety.WaitForMotionCompleteWithSafety()
	}
	if err := r.runtime.WaitForMotionComplete(); err != nil {
		return false, traversal.MotionInterruptTimeout, nil
	}
	return true, traversal.MotionInterruptNone, nil
}

// StopMotion 透传到被包装的 runtime。
func (r *runtimeAdapter) StopMotion() error {
	return r.runtime.StopMotion()
}

// EmergencyStopMotion 急停所有参与校准的运动控制器。
//
// 始终实现 ports.EmergencyStopProvider：内部类型断言被包装对象是否支持急停。
// 支持时透传；不支持时 fallback 到 StopMotion——保证任何 runtime 注入都能
// 至少做到减速停止，避免 handleCalibrationMotionSafetyFailure 中类型断言失败
// 导致急停类故障被静默降级。
func (r *runtimeAdapter) EmergencyStopMotion() error {
	if es, ok := r.runtime.(ports.EmergencyStopProvider); ok {
		return es.EmergencyStopMotion()
	}
	return r.runtime.StopMotion()
}

// fallbackRuntime 回退运行时（直接使用 reader 和 motion 端口）
//
// 在未注入 ports.CalibrationRuntime 时由 createRuntime 构造，提供完整的
// 运动安全循环（到位判定 + EvaluateMotionSafety + 跨样本看门狗 + 120s 兜底）。
//
// 字段：
//   - motionSafety：运动安全配置（来自 config.MotionSafety），nil 时下游使用 traversal.DefaultMotionSafety。
//     注意：调用方允许传 nil——(*MotionSafetyConfig).Resolve 首行 `if c != nil` 兜底，
//     nil 时直接返回 DefaultMotionSafety()，不会 panic（参见 core/traversal/types.go 的 Resolve）。
//     fallbackRuntime.calibrationTargetsReached 与 WaitForMotionComplete 内部均依赖此 nil-safe 行为，
//     修改 Resolve 时必须保留 nil 兜底，否则 fallbackRuntime 路径会崩溃。
//   - isPaused：暂停态回调（延迟绑定到 CalibrationManager.autoEngine.IsPaused），
//     运动循环每轮询周期检查；返回 true 时立即返回 (false, MotionInterruptPaused, nil)
type fallbackRuntime struct {
	reader       ports.LatestDataReader
	motion       ports.MotionManager
	mu           sync.Mutex
	targets      map[calibrationMotionAxis]float64
	motionSafety *traversal.MotionSafetyConfig
	isPaused     func() bool
}

type calibrationMotionAxis struct {
	controllerID string
	axis         motion.AxisName
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

func (f *fallbackRuntime) GetLatestTimestamp(deviceID string) (int64, bool) {
	if f.reader == nil {
		return 0, false
	}
	return f.reader.GetLatestTimestamp(deviceID)
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
	axisName := motion.AxisName(axis.Axis)
	if err := f.motion.MoveTo(ctx, axis.ControllerID, axisName, position); err != nil {
		return err
	}
	f.mu.Lock()
	if f.targets == nil {
		f.targets = make(map[calibrationMotionAxis]float64)
	}
	f.targets[calibrationMotionAxis{controllerID: axis.ControllerID, axis: axisName}] = position
	f.mu.Unlock()
	return nil
}

// WaitForMotionComplete 等待所有目标轴到位或检测到运动安全故障。
//
// 返回三元组 (completed, reason, failure)：
//   - completed=true, reason=none, failure=nil：所有目标轴到位
//   - completed=false, failure!=nil：检测到运动安全故障（撞限位/超差/严重偏离/无进展/越过目标）
//   - completed=false, failure=nil, reason≠none：暂停/超时等非故障中断
//
// 实现移植自 usecase/traversal_acquisition.go::waitForMotionComplete，
// 保持判定优先级与故障快照原则一致：
//  1. 到位检查（motionTargetsReachedForCalibration）
//  2. 暂停检查（isPaused 回调，避免事后读共享状态产生竞态）
//  3. 每轴 EvaluateMotionSafety 单次快照判定（撞限位/到位/超差/严重偏离）
//  4. 跨样本看门狗 Observe（无进展/越过目标）
//  5. 120s 兜底超时（返回 (false, timeout, nil)）
//
// 与遍历实现的差异：fallbackRuntime 不持有 ctx 取消信号（calibration 引擎层
// 通过 isPaused/isRunning 检查实现中断），故省略 ctx.Done() 分支。
// pointIndex 取自 fallbackRuntime 自身记录的最新点位（由下次 MoveToPosition 时更新）。
func (f *fallbackRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	if f.motion == nil {
		return true, traversal.MotionInterruptNone, nil
	}

	// 拷贝目标快照避免长时间持锁
	f.mu.Lock()
	targets := make(map[calibrationMotionAxis]float64, len(f.targets))
	for axis, target := range f.targets {
		targets[axis] = target
	}
	f.mu.Unlock()
	if len(targets) == 0 {
		return true, traversal.MotionInterruptNone, nil
	}

	ticker := time.NewTicker(motionCompletePoll)
	defer ticker.Stop()
	deadline := time.Now().Add(motionCompleteTimeout)
	ctx := context.Background()
	watchdog := newMotionWatchdog()
	statusMissCounter := make(map[calibrationMotionAxis]int)

	for {
		select {
		case <-ticker.C:
			// 1. 优先检查到位：deadline 边界附近先判到位避免假超时
			statuses := f.motion.StatusAll(ctx)
			if f.calibrationTargetsReached(statuses, targets) {
				f.mu.Lock()
				f.targets = nil
				f.mu.Unlock()
				return true, traversal.MotionInterruptNone, nil
			}

			// 2. 暂停检查（非故障中断，返回不可变原因避免竞态）
			if f.isPaused != nil && f.isPaused() {
				return false, traversal.MotionInterruptPaused, nil
			}

			if failure := validateCalibrationMotionStatuses(statuses, targets, statusMissCounter); failure != nil {
				return false, traversal.MotionInterruptNone, failure
			}

			// 3+4. 每轴 EvaluateMotionSafety + 跨样本看门狗 Observe
			for _, status := range statuses {
				if !status.Connected {
					continue
				}
				for _, axisStatus := range status.Axes {
					key := calibrationMotionAxis{controllerID: status.ID, axis: axisStatus.Name}
					target, hasTarget := targets[key]
					if !hasTarget {
						continue
					}
					// 按轴解析有效配置（合并默认值 + 全局 + 按轴覆盖）；
					// motionSafety 为 nil 时 Resolve 走 DefaultMotionSafety
					resolved := f.motionSafety.Resolve(string(axisStatus.Name))
					verdict := EvaluateMotionSafety(axisStatus, target, resolved)
					if verdict.IsFailure() {
						return false, traversal.MotionInterruptNone, &traversal.MotionSafetyFailure{
							ControllerID:   status.ID,
							ControllerName: status.Name,
							Axis:           string(axisStatus.Name),
							Verdict:        verdict,
							Target:         target,
							Actual:         axisStatus.Position,
							PointIndex:     0, // fallbackRuntime 不持有当前点索引，引擎层在错误包装时附加
						}
					}
					if fl := watchdog.Observe(status.ID, axisStatus, target, resolved, 0); fl != nil {
						fl.ControllerName = status.Name
						return false, traversal.MotionInterruptNone, fl
					}
				}
			}

			// 5. 120s 兜底超时
			if time.Now().After(deadline) {
				return false, traversal.MotionInterruptTimeout, nil
			}
		}
	}
}

func validateCalibrationMotionStatuses(
	statuses []motion.ControllerStatus,
	targets map[calibrationMotionAxis]float64,
	statusMissCounter map[calibrationMotionAxis]int,
) *traversal.MotionSafetyFailure {
	statusByController := make(map[string]motion.ControllerStatus, len(statuses))
	for _, status := range statuses {
		statusByController[status.ID] = status
	}

	for targetAxis, target := range targets {
		status, exists := statusByController[targetAxis.controllerID]
		if !exists || !status.Connected {
			statusMissCounter[targetAxis]++
			if statusMissCounter[targetAxis] >= 3 {
				return calibrationStatusUnavailableFailure(targetAxis, target, 0)
			}
			continue
		}
		if status.EmergencyStopped {
			return calibrationStatusUnavailableFailure(targetAxis, target, 0)
		}

		axisFound := false
		for _, axis := range status.Axes {
			if axis.Name == targetAxis.axis {
				axisFound = true
				break
			}
		}
		if !axisFound {
			statusMissCounter[targetAxis]++
			if statusMissCounter[targetAxis] >= 3 {
				return calibrationStatusUnavailableFailure(targetAxis, target, 0)
			}
			continue
		}
		delete(statusMissCounter, targetAxis)
	}
	return nil
}

func calibrationStatusUnavailableFailure(axis calibrationMotionAxis, target, actual float64) *traversal.MotionSafetyFailure {
	return &traversal.MotionSafetyFailure{
		ControllerID: axis.controllerID,
		Axis:         string(axis.axis),
		Verdict:      traversal.MotionSafetyStatusUnavailable,
		Target:       target,
		Actual:       actual,
	}
}

// calibrationTargetsReached 判断所有目标轴是否到位。
// 到位条件：轴已停（!Moving）且 |position-target| ≤ ArrivalTolerance（按轴解析）。
// 与遍历模块 motionTargetsReachedWithTolerance 等价，但目标以 (controllerID,axis)→position
// 映射表达（fallbackRuntime 内部跟踪格式），而非遍历的 traversal.Point+MotionAxes。
func (f *fallbackRuntime) calibrationTargetsReached(
	statuses []motion.ControllerStatus,
	targets map[calibrationMotionAxis]float64,
) bool {
	checked := 0
	for _, status := range statuses {
		if !status.Connected {
			continue
		}
		for _, axisStatus := range status.Axes {
			key := calibrationMotionAxis{controllerID: status.ID, axis: axisStatus.Name}
			target, hasTarget := targets[key]
			if !hasTarget {
				continue
			}
			checked++
			resolved := f.motionSafety.Resolve(string(axisStatus.Name))
			tolerance := *resolved.ArrivalTolerance
			if axisStatus.Moving || math.Abs(axisStatus.Position-target) > tolerance {
				return false
			}
		}
	}
	return checked > 0
}

// EmergencyStopMotion 急停所有参与校准的运动控制器。
//
// 始终实现 ports.EmergencyStopProvider：对每个有目标位置记录的控制器调用
// motion.EmergencyStop（控制器级，所有轴瞬时停止）。
// 至少一台急停失败时 fallback 到 stopAllMotion 减速停，保证不会"全部急停失败仍继续运动"。
// 聚合所有错误返回，调用方据此决定是否升级错误码。
func (f *fallbackRuntime) EmergencyStopMotion() error {
	if f.motion == nil {
		return nil
	}
	f.mu.Lock()
	controllerIDs := make(map[string]bool, len(f.targets))
	for k := range f.targets {
		if k.controllerID != "" {
			controllerIDs[k.controllerID] = true
		}
	}
	f.mu.Unlock()

	if len(controllerIDs) == 0 {
		// 无目标记录：对所有已连接控制器急停（防御性兜底）
		ctx := context.Background()
		for _, status := range f.motion.StatusAll(ctx) {
			if status.Connected {
				controllerIDs[status.ID] = true
			}
		}
	}

	ctx := context.Background()
	var errs []error
	for controllerID := range controllerIDs {
		if err := f.motion.EmergencyStop(ctx, controllerID); err != nil {
			log.Printf("[CalibrationManager] 急停失败 %s: %v", controllerID, err)
			errs = append(errs, fmt.Errorf("controller %s: %w", controllerID, err))
		}
	}
	if len(errs) > 0 {
		// 至少一台失败：fallback 减速停，避免剩余运动轴继续运动
		if stopErr := stopAllMotion(f.motion); stopErr != nil {
			errs = append(errs, fmt.Errorf("fallback stop also failed: %w", stopErr))
		}
		f.mu.Lock()
		f.targets = nil
		f.mu.Unlock()
		return errors.Join(errs...)
	}
	f.mu.Lock()
	f.targets = nil
	f.mu.Unlock()
	return nil
}

// StopMotion 立即停止所有运动轴（普通 Stop，减速停止）。
// 暂停时由引擎调用以打断当前点位运动；与 EmergencyStopMotion 的差异见 ports.EmergencyStopProvider 文档。
func (f *fallbackRuntime) StopMotion() error {
	if f.motion == nil {
		return nil
	}
	err := stopAllMotion(f.motion)
	f.mu.Lock()
	f.targets = nil
	f.mu.Unlock()
	return err
}

// stopAllMotion 停止所有运动控制器中 Moving=true 的轴。
// CalibrationManager.stopMotion 与 fallbackRuntime.StopMotion 共用此逻辑。
func stopAllMotion(mgr ports.MotionManager) error {
	ctx := context.Background()
	var firstErr error
	for _, status := range mgr.StatusAll(ctx) {
		for _, axis := range status.Axes {
			if axis.Moving {
				if err := mgr.Stop(ctx, status.ID, axis.Name); err != nil {
					log.Printf("[CalibrationManager] 停止运动失败 %s/%s: %v", status.ID, axis.Name, err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		}
	}
	return firstErr
}
