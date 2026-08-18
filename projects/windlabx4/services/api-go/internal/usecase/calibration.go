package usecase

import (
	"context"
	"errors"
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"windlabx4/services/api-go/internal/core/calibration"
	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/core/motion"
	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/ports"
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
// Wails backend 会先做 ResolvePath（相对路径→%APPDATA%\windlabx4\<相对>），
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

// calibrationStopJoinTimeout Stop/Shutdown 等待校准 worker 完全退出
// （writer flush、结果保存、运动归零均完成）的上限。超时后 Stop 返回明确错误，
// 且在旧 session 退出前新 Start 持续被拒绝，禁止 replacement 复用旧任务资源。
// 设为包级变量（非常量）以便测试注入短超时；生产路径不得修改。
var calibrationStopJoinTimeout = 5 * time.Second

// calibrationRunSession 一次校准运行的会话。
//
// 每个 session 捕获自己独占的资源：引擎实例、归一化后的配置、任务 ID、
// 取消函数与退出信号。worker goroutine 只 finalize 自己 session 的资源，
// 不读写后续 session 的引擎/writer/状态；session 的 done 在 finalize 全部
// 完成后才关闭，Start 据此拒绝与旧任务资源重叠的新任务。
type calibrationRunSession struct {
	taskID string
	config calibration.Config
	engine *calibration.AutomaticCalibration
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // worker 完全退出（含 finalize）后关闭
}

// sessionDone 报告 session 的 worker 是否已完全退出（含 finalize）。
func sessionDone(s *calibrationRunSession) bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// CalibrationManager 校准管理器
// 管理校准任务的生命周期，协调自动校准引擎、采集协调器、CSV写入器等组件
type CalibrationManager struct {
	mu     sync.RWMutex
	reader ports.LatestDataReader
	motion ports.MotionManager
	sink   ports.CalibrationPointSink
	store  ports.CalibrationResultStore

	// 新增组件
	eventPublisher       ports.CalibrationEventPublisher
	runtime              ports.CalibrationRuntime
	statusProvider       ports.DeviceStatusProvider
	acquisitionController ports.AcquisitionController

	// 校准引擎与当前会话。
	// autoEngine 保留给既有读取路径（Pause/Resume/Status 等），
	// 写入只发生在 Start 发布新 session 时（持 m.mu）；
	// session 指向当前/最近一次任务会话，任务结束后保留终态供 Status 查询。
	autoEngine       *calibration.AutomaticCalibration
	session          *calibrationRunSession
	currentConfig    calibration.Config
	currentStatus    calibration.Status
	currentTaskID    string
	pauseStartedAt   time.Time
	csvWriter        ports.CalibrationCsvWriter
	csvWriterFactory func(calibration.Config) ports.CalibrationCsvWriter
	lastExport       *calibration.ExportPayload

	// 七孔双 CSV writer 路由（spec Task 9 + §7.1）
	// sevenHoleWriterFactory 工厂端口，按 region+sector 创建独立 writer 实例
	// sevenHoleWriters 当前任务运行期懒加载缓存：key = region 或 "outer_<n>"，
	// value = 已 Initialize 的 writer。任务结束（Stop/Completed）时统一 flush。
	sevenHoleWriterFactory ports.CalibrationWriterFactory
	sevenHoleWriters       map[string]ports.CalibrationCsvWriter
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

// SetAcquisitionController 注入设备采集控制端口。
//
// 装配根在创建 DeviceManager 之后回填：DeviceManager 创建顺序在 CalibrationManager 之后，
// 构造期无法表达"先创建 mgr 再回填"的依赖（与 TraversalManager.SetAcquisitionController 同模式）。
//
// 注入后通过 runtimeAdapter/fallbackRuntime 桥接到 RuntimeAccess.IsAcquiring，
// 供校准算法在 waitForFreshData 超时后区分"用户停采集"（可恢复）与"设备在采集但帧不更新"（真异常）。
// 未注入时 IsAcquiring 恒返回 true，算法回退到原超时失败行为（向后兼容）。
func (m *CalibrationManager) SetAcquisitionController(controller ports.AcquisitionController) {
	m.mu.Lock()
	m.acquisitionController = controller
	m.mu.Unlock()
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

// SetSevenHoleWriterFactory 注入七孔 CSV writer 工厂端口
//
// 装配根（pkg/appcontext / pkg/apiserver）注入同一个 adapters/storage.CalibrationCsvWriter
// 实例：该实例同时实现 CalibrationCsvWriter（单 writer 场景）与 CalibrationWriterFactory
// （七孔多 writer 场景）接口。
//
// 七孔 Start 时通过此工厂为每个 region+sector 创建独立 writer，避免单 writer 实例的
// schema 被 config.Type 重建覆盖（七孔 schema 含 region+sector 路由信息）。
func (m *CalibrationManager) SetSevenHoleWriterFactory(factory ports.CalibrationWriterFactory) {
	m.sevenHoleWriterFactory = factory
}

// Start 启动校准任务
//
// 生命周期模型（run session）：
//   - 所有预启动校验全部通过后才发布 Running 状态/引擎/session，
//     校验失败不留下任何运行态痕迹；
//   - 每次 Start 建立一个 calibrationRunSession，worker goroutine 独占该
//     session 的引擎/配置，finalize（writer flush、结果保存、运动归零）
//     只由该 worker 执行一次，done 在 finalize 完成后才关闭；
//   - 旧 session 未 done 时拒绝新任务（含 Stop 超时后旧 worker 收尾期间），
//     禁止 replacement 复用旧任务资源。
func (m *CalibrationManager) Start(config calibration.Config) error {
	if config.TaskID == "" {
		return fmt.Errorf("taskID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentStatus.State == calibration.StateRunning || m.currentStatus.State == calibration.StatePaused {
		return fmt.Errorf("校准任务已在运行中，请先停止")
	}
	// session 门禁：上一任务 worker 未完全退出（finalize 未完成）时拒绝新任务。
	// Stop 仅设置 Stopped 状态并请求取消，worker 的 flush/保存/归零可能仍在进行。
	if m.session != nil && !sessionDone(m.session) {
		return fmt.Errorf("上一次校准任务尚未完全退出（taskID=%s），请稍后重试", m.session.taskID)
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

	// ============ 预启动校验（全部通过前不发布任何运行态） ============

	// 运动安全配置校验：非法阈值/未绑定轴在启动前拒绝，避免运动中才发现配置错误
	if err := validateCalibrationMotionSafetyConfig(config.MotionSafety, config.MotionAxes); err != nil {
		return fmt.Errorf("运动安全配置校验失败: %w", err)
	}

	// 根据校准类型选择算法（未知类型在此拒绝，不得留下 Running 状态）
	algorithm, err := m.createAlgorithm(config)
	if err != nil {
		return err
	}
	if len(config.ProbeChannels) > 0 {
		if err := algorithm.ValidateConfig(config); err != nil {
			return err
		}
	}

	// 创建事件发布适配器
	publisher := m.createEventPublisher()

	// 创建运行时适配器（注入运动安全配置 + isPaused 回调）
	runtime := m.createRuntime(config.MotionSafety)

	// CSV 实时写入：自动校准类型在 Start 时以覆盖模式初始化 csvWriter，
	// 每个点采集完成后通过 onDataPoint 回调逐点写入，崩溃/断电不丢已采集点。
	// 总温校准使用手动逐点采集，由 CollectCurrentPoint 直接调用 csvWriter。
	autoTypes := map[string]bool{
		string(calibration.TypeFiveHole):      true,
		string(calibration.TypeThreeHole):     true,
		string(calibration.TypeTotalPressure): true,
		// 七孔校准走自动引擎：与五孔/三孔/总压相同的逐点采集流程，
		// 但 CSV 写入走"双 writer 路由"——按 region+sector 分文件落盘
		// （1 内区 + 6 外区，共 7 个 CSV 文件，见 routeSevenHoleDataPoint）。
		string(calibration.TypeSevenHole): true,
	}
	var csvPointSink calibration.DataPointSink
	isSevenHole := config.Type == string(calibration.TypeSevenHole)
	if isSevenHole {
		// 七孔走独立的多 writer 路由分支，不调用 m.csvWriter.Initialize
		// （单 writer 的 schema 会被 config.Type 重建覆盖，无法承载 region+sector 路由）
		csvPointSink = m.buildSevenHoleCsvSink(config)
	} else if autoTypes[config.Type] && config.SavePath != "" && m.csvWriter != nil {
		if err := m.csvWriter.Initialize(config); err != nil {
			slog.Error("calibration csv writer init failed",
				"component", "calibration", "task_id", config.TaskID, "error", err)
		} else {
			writer := m.csvWriter
			csvPointSink = func(dp calibration.DataPoint) {
				if err := writer.AppendPoint(dp); err != nil {
					slog.Error("calibration csv write failed",
						"component", "calibration", "task_id", config.TaskID, "error", err)
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
	engine := calibration.NewAutomaticCalibration(config, publisher, runtime, onDataPoint, onMotionSafetyFailure)
	engine.SetTaskID(config.TaskID)

	// 建立本次运行的 session：worker 独占引擎/配置，ctx 取消驱动有界退出
	ctx, cancel := context.WithCancel(context.Background())
	session := &calibrationRunSession{
		taskID: config.TaskID,
		config: config,
		engine: engine,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	// ============ 全部校验通过，发布运行态 ============
	m.currentConfig = config
	m.currentTaskID = config.TaskID
	m.lastExport = nil
	m.autoEngine = engine
	m.session = session
	m.currentStatus = calibration.Status{
		TaskID:      config.TaskID,
		Type:        config.Type,
		State:       calibration.StateRunning,
		TotalPoints: len(config.Points),
		StartTime:   time.Now().UnixMilli(),
	}
	m.pauseStartedAt = time.Time{}

	// 异步启动校准循环（worker 在锁内启动无害：其 finalize 需等 Start
	// 返回释放 m.mu 后才能写入状态，与既有行为一致）
	go m.runCalibrationSession(session, algorithm)

	return nil
}

// runCalibrationSession 校准 worker：执行本 session 的引擎循环并独占 finalize。
//
// finalize 顺序与单次语义：
//  1. 引擎退出（正常完成/错误/取消）后按 session 归属写入终态与导出载荷；
//  2. 保存结果（store.Save）、flush 单 writer、flush 七孔多 writer、运动归零——
//     每项只由本 worker 执行一次，Stop 不再重复执行；
//  3. 全部完成后才关闭 session.done（defer 保证 panic 也关闭），
//     Start 的 session 门禁据此确认旧任务资源已完全释放。
func (m *CalibrationManager) runCalibrationSession(session *calibrationRunSession, algorithm calibration.Algorithm) {
	defer close(session.done)

	engineErr := session.engine.StartWithContext(session.ctx, algorithm)
	dataPoints := session.engine.GetDataPoints()

	m.mu.Lock()
	// 归属守卫：只写本 session 的状态。session 门禁已保证新旧 worker 不重叠，
	// 此守卫是最后一道防线（防御性，正常情况下恒为 true）。
	owned := m.currentStatus.TaskID == session.taskID
	if owned {
		if m.currentStatus.State != calibration.StateStopped {
			if engineErr != nil {
				m.currentStatus.State = calibration.StateError
				m.currentStatus.LastError = engineErr.Error()
			} else {
				m.currentStatus.State = calibration.StateCompleted
			}
		}

		// 保存导出载荷，供 SaveCsv 按需写入
		m.lastExport = &calibration.ExportPayload{
			Type:       calibration.CalibrationType(session.config.Type),
			Config:     session.config,
			DataPoints: dataPoints,
		}
		m.currentStatus.DataPoints = dataPoints
		m.currentStatus.CompletedPoints = len(dataPoints)
		if m.currentStatus.TotalPoints > 0 {
			m.currentStatus.Progress = float64(len(dataPoints)) / float64(m.currentStatus.TotalPoints) * 100
		}
	}
	statusSnapshot := m.currentStatus
	store := m.store
	writer := m.csvWriter
	m.mu.Unlock()

	// 保存结果（锁外执行：store.Save 可能涉及文件 I/O）
	if store != nil && owned {
		if saveErr := store.Save(session.taskID, statusSnapshot); saveErr != nil {
			slog.Error("calibration save result failed",
				"component", "calibration", "task_id", session.taskID, "error", saveErr)
		}
	}

	// 实时 CSV 写入完成：flush 并关闭文件句柄，下次 Start 可重新 Initialize。
	// 不置 nil：writer 由装配根注入一次，Flush 后 file 已关闭，可复用。
	if writer != nil {
		if flushErr := writer.Flush(); flushErr != nil {
			slog.Error("calibration csv flush failed",
				"component", "calibration", "task_id", session.taskID, "error", flushErr)
		}
	}

	// 七孔多 writer 路由场景：flush 所有 region+sector writer（spec Task 9）。
	// flushAllSevenHoleWriters 设计为锁外调用（内部自摘 map + 锁外 Flush）。
	m.flushAllSevenHoleWriters()

	// 运动归零
	m.returnToHomePosition(session.config)
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
	m.pauseStartedAt = time.Now()
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
	m.settlePauseDurationLocked(time.Now())
	engine := m.autoEngine
	m.mu.Unlock()

	if engine != nil {
		engine.Resume()
	}
	return nil
}

// Stop 停止校准
//
// 停止模型（run session）：
//  1. 锁内：请求引擎停止标志 + 取消 session ctx（dwell/pause/gate/新数据等待
//     立即中断）+ 切换 Stopped 状态；
//  2. 锁外：停止所有运动轴（硬件下发，不持 m.mu）；
//  3. 锁外有界等待 session done（writer flush/结果保存/归零由 worker 独占执行，
//     Stop 不重复执行），最多 calibrationStopJoinTimeout（5s）。
//
// 超时返回明确错误：此后旧 session 未 done 之前 Start 持续被拒绝（session 门禁），
// 旧 worker 退出时会照常完成自己的 finalize，状态归属守卫保证不污染新任务。
//
// 注意：store.Save 失败不再作为 Stop 错误返回——结果保存已收敛到 worker finalize
// 单次执行（错误仅记日志），Stop 的错误仅表达"等待 worker 退出超时"。
func (m *CalibrationManager) Stop() error {
	m.mu.Lock()
	session := m.session
	if session == nil {
		// 从未启动过任务：保持旧行为的幂等停止（置 Stopped + 停运动）
		m.settlePauseDurationLocked(time.Now())
		m.currentStatus.State = calibration.StateStopped
		m.mu.Unlock()
		if err := m.stopMotion(); err != nil {
			slog.Warn("calibration stop motion failed",
				"component", "calibration", "error", err)
		}
		return nil
	}

	// 请求停止：引擎标志位（算法检查路径）+ ctx 取消（可中断等待路径）
	session.engine.Stop()
	session.cancel()

	m.settlePauseDurationLocked(time.Now())
	m.currentStatus.State = calibration.StateStopped
	m.mu.Unlock()

	// 停止运动（用户主动停止路径，错误仅记录不影响流程）
	if err := m.stopMotion(); err != nil {
		slog.Warn("calibration stop motion failed",
			"component", "calibration", "task_id", session.taskID, "error", err)
	}

	// 有界等待 worker 完全退出（finalize 完成）。超时后旧 session 仍在收尾，
	// Start 的 session 门禁会继续拒绝新任务，直到 worker 真正退出。
	select {
	case <-session.done:
		return nil
	case <-time.After(calibrationStopJoinTimeout):
		return fmt.Errorf("calibration stop timed out after %s waiting for task %s to finish (task finalizes in background; new start is rejected until it exits)", calibrationStopJoinTimeout, session.taskID)
	}
}

// Shutdown 停止当前校准任务并有界等待 worker 退出（进程/服务关闭路径专用）。
//
// 语义与 Stop 一致：请求取消 + 停止运动 + 最多等待 calibrationStopJoinTimeout。
// 与 Stop 的区别仅在使用场景：Shutdown 由装配根（Wails ServiceShutdown /
// context-owned apiserver）在服务关闭时调用，调用方应记录超时错误但不必
// 因此中断整体关闭流程。无活动任务时幂等返回 nil。
func (m *CalibrationManager) Shutdown() error {
	return m.Stop()
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

	dataPoint, err := algorithm.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, sampleInterval, nil, m.makeTimestampReader(), m.makeAcquisitionStateProvider(), nil)
	if err != nil {
		return m.fail("采集当前工况点失败: %v", err)
	}

	// 写入CSV
	if m.csvWriter != nil {
		if writeErr := m.csvWriter.AppendPoint(dataPoint); writeErr != nil {
			slog.Error("calibration csv write failed",
				"component", "calibration", "task_id", m.currentTaskID, "error", writeErr)
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

	// 七孔：按 region+sector 分区导出为 7 份参考数据集格式 CSV
	// （1 小角度区 + 6 大角度区，18 列基础格式 + GBK 编码，spec §7.1/§7.2/§7.3）。
	// 单 writer 26 列证书格式会把外区点按内区 schema 落盘（θ/φ 坐标与 Kθ/Kφ 全丢为 0），
	// 且无法被七孔插值加载器按位置契约解析，因此七孔必须走独立分区导出路径。
	if payload.Type == calibration.TypeSevenHole {
		return m.saveSevenHoleZonedCsv(payload, savePath)
	}

	if m.csvWriterFactory == nil {
		return "", fmt.Errorf("CSV写入器未配置")
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
			// AppendPoint 失败时仍要尝试 cleanup Flush（关闭文件句柄）。
			// 若 cleanup Flush 也失败，用 errors.Join 聚合两个错误，确保均可识别（spec Task 20）。
			// 旧实现 `_ = writer.Flush()` 静默丢弃 cleanup 错误，调试困难。
			cleanupErr := writer.Flush()
			return "", errors.Join(err, cleanupErr)
		}
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}
	return writer.Path(), nil
}

// saveSevenHoleZonedCsv 七孔校准结果分区导出（spec §7.1 方案 A + §7.2/§7.3 18 列基础格式）
//
// 数据点按 Region+Sector 分组落盘（与参考数据集 W532.202608.P.7H.1-01 布局一致）：
//   - 内区：<stem>(小角度区).csv
//   - 外区 n：<stem>(大角度<n>区).csv
//
// 扇区边界点共享（spec §6.2 数据集约定）：φ 恰好落在扇区边界（30°/90°/.../330°）的
// 外区点同时写入相邻两个扇区的文件，使每个大角度区文件覆盖完整 13 条 φ 网格线
// （与参考数据集每区 52 行 = 13φ×4θ 的布局对齐）。数据集模式下边界点本身就在两个
// 扇区各采集了一次，共享时按 (θ,φ) 去重，避免文件内出现重复网格点。
//
// 返回内区文件路径（无内区点时返回第一个写入的文件路径），供前端提示导出位置。
func (m *CalibrationManager) saveSevenHoleZonedCsv(payload calibration.ExportPayload, savePath string) (string, error) {
	if m.sevenHoleWriterFactory == nil {
		return "", fmt.Errorf("七孔 CSV 写入器工厂未配置")
	}

	stem := strings.TrimSuffix(savePath, filepath.Ext(savePath))

	innerPoints, outerPoints := groupSevenHoleExportPoints(payload.DataPoints)
	if len(innerPoints) == 0 && len(outerPoints) == 0 {
		return "", fmt.Errorf("校准结果中无七孔数据点，无法保存CSV")
	}

	// 写入顺序：内区在前，外区按扇区 1..6 升序（空分区跳过——
	// 用户可配置只校准部分扇区，空分区不生成文件）
	firstPath := ""
	writeZone := func(region string, sector int, points []*calibration.SevenHoleDataPoint) error {
		if len(points) == 0 {
			return nil
		}
		path := stem + sevenHoleExportFileSuffix(region, sector) + ".csv"
		schema := calibration.NewSevenHoleExportCsvSchema(payload.Config, region, sector)
		writer, err := m.sevenHoleWriterFactory.NewWriterTruncate(path, schema)
		if err != nil {
			return fmt.Errorf("创建七孔导出 writer 失败 (path=%s): %w", path, err)
		}
		for _, dp := range points {
			if err := writer.AppendPoint(dp); err != nil {
				// AppendPoint 失败时仍要 cleanup Flush（关闭文件句柄），错误聚合（spec Task 20）
				return errors.Join(err, writer.Flush())
			}
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		if firstPath == "" {
			firstPath = path
		}
		return nil
	}

	if err := writeZone("inner", 0, innerPoints); err != nil {
		return "", err
	}
	for sector := 1; sector <= 6; sector++ {
		if err := writeZone("outer", sector, outerPoints[sector]); err != nil {
			return "", err
		}
	}
	return firstPath, nil
}

// groupSevenHoleExportPoints 按 region+sector 分组七孔导出数据点，含扇区边界点共享
//
// 返回：innerPoints（内区点，保持原始顺序）、outerPoints（按扇区 1..6 分组）。
// 非 *SevenHoleDataPoint 的数据点跳过（防御：类型异常不阻塞整批导出）。
func groupSevenHoleExportPoints(dataPoints []calibration.DataPoint) ([]*calibration.SevenHoleDataPoint, map[int][]*calibration.SevenHoleDataPoint) {
	innerPoints := make([]*calibration.SevenHoleDataPoint, 0, len(dataPoints))
	outerPoints := make(map[int][]*calibration.SevenHoleDataPoint, 6)
	// 每个扇区已写入的 (θ,φ) 网格点集合——边界点共享到相邻扇区时按 key 去重，
	// 避免数据集模式（边界点两扇区各采一次）在同一文件内产生重复网格点
	outerKeys := make(map[int]map[[2]float64]bool, 6)
	appendOuter := func(sector int, dp *calibration.SevenHoleDataPoint) {
		key := [2]float64{dp.Coordinates["θ"], dp.Coordinates["φ"]}
		keys := outerKeys[sector]
		if keys == nil {
			keys = make(map[[2]float64]bool)
			outerKeys[sector] = keys
		}
		if keys[key] {
			return
		}
		keys[key] = true
		outerPoints[sector] = append(outerPoints[sector], dp)
	}

	for _, dp := range dataPoints {
		shDp, ok := dp.(*calibration.SevenHoleDataPoint)
		if !ok {
			continue
		}
		if shDp.Region == "outer" {
			if shDp.Sector < 1 || shDp.Sector > 6 {
				// 脏数据：外区点扇区编号非法——跳过并告警，不并入内区
				// （并入内区会按缺失的 α/β 键落盘为 0.000 行，
				// 还会在插值加载器中触发重复网格点错误）
				slog.Warn("calibration export skipping outer point with invalid sector",
					"point_id", shDp.PointID, "sector", shDp.Sector)
				continue
			}
			appendOuter(shDp.Sector, shDp)
			// 扇区边界点（φ=30°/90°/.../330°）共享到几何相邻的另一个扇区文件——
			// 邻接关系由 φ 几何位置判定（SevenHoleSectorBoundaryNeighbors），
			// 不能由已分配扇区推算：数据集模式下 Sector 1 同时合法包含
			// φ=330°（邻接 6 区）与 φ=30°（邻接 2 区）两个边界。
			// 仅当点位预设 Sector 确为相邻扇区对之一时才共享（防御脏数据）。
			if lower, upper, ok := calibration.SevenHoleSectorBoundaryNeighbors(shDp.Coordinates["φ"]); ok {
				switch shDp.Sector {
				case lower:
					appendOuter(upper, shDp)
				case upper:
					appendOuter(lower, shDp)
				}
			}
			continue
		}
		innerPoints = append(innerPoints, shDp)
	}
	return innerPoints, outerPoints
}

// sevenHoleExportFileSuffix 拼接七孔分区导出文件命名后缀（spec §7.1 参考数据集命名约定）
//
//   - 内区："(小角度区)"
//   - 外区 n："(大角度<n>区)"
//
// 与逐点采集的 26 列过程记录文件（sevenHoleFileSuffix，"_小角度区" 下划线命名）区分：
// 括号命名 = 18 列参考数据集格式交付文件，下划线命名 = 26 列证书格式过程记录。
func sevenHoleExportFileSuffix(region string, sector int) string {
	if region == "inner" {
		return "(小角度区)"
	}
	return fmt.Sprintf("(大角度%d区)", sector)
}

// PreviewSevenHolePoints 预览七孔校准的点位分布（spec Task 9）
//
// 调用 calibration.GenerateSevenHolePoints 生成完整点位列表，并按 region 聚合统计
// 内/外区点数，返回 SevenHolePreviewResult 供前端"配置向导"实时显示。
//
// 此方法不启动采集、不创建 CSV writer、不创建 runtime——纯点位生成 + 统计。
// 调用方：HTTP API server.go 的 /api/calibration/sevenhole/preview handler、
// Wails backend 的 Binding 方法。两端共用此 usecase 方法，确保点位生成逻辑唯一。
//
// 错误返回：透传 GenerateSevenHolePoints 的错误（步长≤0、范围 min>max 等）。
func (m *CalibrationManager) PreviewSevenHolePoints(config calibration.SevenHoleConfig) (calibration.SevenHolePreviewResult, error) {
	points, err := calibration.GenerateSevenHolePoints(config)
	if err != nil {
		return calibration.SevenHolePreviewResult{}, fmt.Errorf("生成七孔点位失败: %w", err)
	}

	// 按 Region 聚合统计——遍历一次点位列表，内区点 Region="inner"，外区点 Region="outer"
	innerCount := 0
	outerCount := 0
	for _, p := range points {
		if p.Region == "inner" {
			innerCount++
		} else if p.Region == "outer" {
			outerCount++
		}
	}

	return calibration.SevenHolePreviewResult{
		Points:     points,
		TotalCount: len(points),
		InnerCount: innerCount,
		OuterCount: outerCount,
	}, nil
}

// PreviewFiveHolePoints 预览五孔蛇形校准的点位分布（spec Task 10）
//
// 设计要点（plan Slice B4 / spec R-4、R-6）：
//   - 五孔点位生成原本在 API 层直接调 core.GenerateFiveHoleSnakePoints，违反
//     "API 不直接调用点位生成算法"边界。本方法将生成收口到 usecase 层，
//     HTTP/Wails 共用同一入口，确保点位生成逻辑唯一、可观测。
//   - 与 PreviewSevenHolePoints 对称：纯计算、不启动采集、不创建 runtime、不写 CSV；
//     仅依赖 core 公式，不访问 reader/motion/sink/store，nil receiver 也可安全调用。
//   - 返回 []calibration.FiveHoleSnakePoint（与 core.GenerateFiveHoleSnakePoints
//     相同的元素类型），保留 bare-array 语义，避免破坏前端 JSON 契约
//     （HTTP handler 直接 writeJSON(points)，前端收到 [...] 而非 {points:[...]}）。
//
// 错误返回：透传 core.GenerateFiveHoleSnakePoints 的错误（步长≤0 等），
// 包一层中文上下文便于前端直接展示。
func (m *CalibrationManager) PreviewFiveHolePoints(layout calibration.FiveHolePointLayout) ([]calibration.FiveHoleSnakePoint, error) {
	points, err := calibration.GenerateFiveHoleSnakePoints(layout)
	if err != nil {
		return nil, fmt.Errorf("生成五孔点位失败: %w", err)
	}
	return points, nil
}

// buildSevenHoleCsvSink 构建七孔 CSV 写入 sink（按 region+sector 路由）
//
// 设计要点（spec Task 9 + §7.1）：
//   - 七孔按 region+sector 分文件落盘（1 内区 + 6 外区，共 7 个 CSV 文件）
//   - 每个文件有独立的列布局：外区表头 Kθ[n] 中 n 由 sector 替换为具体扇区编号
//   - writer 懒加载：首次出现某 region+sector 时才通过工厂创建并 Initialize，
//     避免无数据的外区文件被创建空文件
//   - 文件命名：在 config.SavePath 基础上去掉 .csv 扩展名，追加区域后缀
//     内区：<base>_小角度区.csv（spec §7.1 数据集命名约定）
//     外区 n：<base>_大角度<n>区.csv
//
// 返回的 sink 在 onDataPoint 回调中调用，按 dp.(*SevenHoleDataPoint) 类型断言后
// 取 Region+Sector 路由到对应 writer。类型断言失败时记录日志并跳过——避免七孔
// 算法误返回非 SevenHoleDataPoint 时整批数据丢失。
//
// 不做错误返回：工厂未注入/Initialize 失败时返回 nil sink，由调用方判断。
// Initialize 失败时记录日志，任务继续（CSV 落盘失败不阻塞采集，与五孔行为一致）。
func (m *CalibrationManager) buildSevenHoleCsvSink(config calibration.Config) calibration.DataPointSink {
	// spec Task 22：拆分 factory 缺失与 SavePath 空两种情况。
	//   - factory 缺失：装配错误（七孔类型应注入 sevenHoleWriterFactory），记 slog.Error。
	//   - SavePath 空：合法可选项（用户不要求落盘 CSV），仅记 slog.Info，不报假错误。
	if m.sevenHoleWriterFactory == nil {
		slog.Error("calibration seven hole writer factory missing",
			"component", "calibration", "task_id", config.TaskID)
		return nil
	}
	if config.SavePath == "" {
		slog.Info("calibration seven hole csv skipped, save path empty",
			"component", "calibration", "task_id", config.TaskID)
		return nil
	}

	// 初始化当前任务的 writer 缓存（每次 Start 重置，避免上次任务残留）
	m.sevenHoleWriters = make(map[string]ports.CalibrationCsvWriter)
	// 保存 base 路径（去 .csv 扩展名），供 routeSevenHoleWriter 拼接区域后缀
	basePath := strings.TrimSuffix(config.SavePath, filepath.Ext(config.SavePath))

	return func(dp calibration.DataPoint) {
		shDp, ok := dp.(*calibration.SevenHoleDataPoint)
		if !ok {
			slog.Warn("calibration seven hole onDataPoint wrong type",
				"component", "calibration", "task_id", config.TaskID,
				"type", fmt.Sprintf("%T", dp))
			return
		}
		// CSV 路由按数据点的 Region 字段决定文件归属。
		// Region 由 seven_hole.go 的 AcquireDataWithChannels 直接取自 point.Region
		// （用户配置的轨迹区域），不再受 DetermineRegion 压力判定影响——
		// 内区轨迹点（α/β）必然路由到内区 CSV，外区轨迹点（θ/φ）必然路由到外区 CSV。
		writer, err := m.routeSevenHoleWriter(config, basePath, shDp.Region, shDp.Sector)
		if err != nil {
			slog.Error("calibration seven hole csv route failed",
				"component", "calibration", "task_id", config.TaskID,
				"region", shDp.Region, "sector", shDp.Sector, "error", err)
			return
		}
		if err := writer.AppendPoint(shDp); err != nil {
			slog.Error("calibration seven hole csv write failed",
				"component", "calibration", "task_id", config.TaskID,
				"region", shDp.Region, "sector", shDp.Sector, "error", err)
		}
	}
}

// routeSevenHoleWriter 按 region+sector 路由到对应 writer（懒加载创建）
//
// 缓存 key 设计：
//   - 内区：key = "inner"（sector 固定为 7，无需区分）
//   - 外区：key = "outer_<n>"（n = 1..6）
//
// writer 创建步骤：
//  1. 按 region+sector 拼接文件路径（basePath + 区域后缀）
//  2. 通过 calibration.NewSevenHoleCsvSchema 构建对应 schema
//  3. 调用工厂 NewWriter 创建并 Initialize writer（文件 I/O 在锁外执行，避免持锁阻塞）
//  4. 缓存到 m.sevenHoleWriters 供后续点复用（加写锁 + double-check 防止重复创建）
//
// 并发安全（code-review C1 修复）：
//   - onDataPoint 在校准 goroutine 中调用本方法（读+写 map）
//   - Stop/Flush 在用户停止 goroutine 中调用 flushAllSevenHoleWriters（读+置 nil map）
//   - 两者并发时通过 m.mu 串行化访问 map，避免数据竞争
//   - 文件 I/O（NewWriter）在锁外执行，避免持锁阻塞其他查询路径（如 Status）
//   - Double-check 模式：若并发期间其他 goroutine 已抢先创建同 key writer，
//     丢弃本次创建的 writer（Flush 关闭文件句柄避免泄漏）并返回已存在的实例
func (m *CalibrationManager) routeSevenHoleWriter(config calibration.Config, basePath, region string, sector int) (ports.CalibrationCsvWriter, error) {
	key := sevenHoleWriterKey(region, sector)

	// 快速路径：RLock 查缓存命中则直接返回（避免加写锁的开销）
	m.mu.RLock()
	if m.sevenHoleWriters != nil {
		if cached, ok := m.sevenHoleWriters[key]; ok {
			m.mu.RUnlock()
			return cached, nil
		}
	}
	m.mu.RUnlock()

	// 慢路径：未命中，创建新 writer（文件 I/O 在锁外执行）
	path := basePath + sevenHoleFileSuffix(region, sector) + ".csv"
	schema := calibration.NewSevenHoleCsvSchema(config, region, sector)
	writer, err := m.sevenHoleWriterFactory.NewWriter(path, schema)
	if err != nil {
		return nil, fmt.Errorf("创建七孔 CSV writer 失败 (path=%s): %w", path, err)
	}

	// 加写锁写入 map，double-check 防止并发期间其他 goroutine 已抢先创建
	m.mu.Lock()
	if m.sevenHoleWriters == nil {
		// flushAllSevenHoleWriters 已在并发中清空 map——任务即将停止
		// 关闭刚创建的 writer 文件句柄避免泄漏，返回错误让调用方跳过本点 CSV 写入
		m.mu.Unlock()
		// 临时 writer 的 Flush 失败仅记录警告：任务即将停止，不阻塞错误返回（spec Task 20）。
		// 旧实现 `_ = writer.Flush()` 静默丢弃错误，调试困难。
		if flushErr := writer.Flush(); flushErr != nil {
			slog.Warn("calibration seven hole temp writer flush failed (cache cleared, discard)",
				"component", "calibration", "error", flushErr)
		}
		return nil, fmt.Errorf("七孔 writer 缓存已清空，任务即将停止")
	}
	if cached, ok := m.sevenHoleWriters[key]; ok {
		// 并发期间其他 goroutine 已创建——丢弃本次创建的 writer
		m.mu.Unlock()
		// 临时 writer 的 Flush 失败仅记录警告：cached writer 仍可用，不影响主流程（spec Task 20）。
		// 旧实现 `_ = writer.Flush()` 静默丢弃错误，调试困难。
		if flushErr := writer.Flush(); flushErr != nil {
			slog.Warn("calibration seven hole temp writer flush failed (double-check discard)",
				"component", "calibration", "key", key, "error", flushErr)
		}
		return cached, nil
	}
	m.sevenHoleWriters[key] = writer
	m.mu.Unlock()
	return writer, nil
}

// sevenHoleWriterKey 生成 writer 缓存 key
//
// 内区统一 "inner"（sector 字段对内区无意义，固定 7）；
// 外区按 sector 区分（1..6 各一个 writer）。
func sevenHoleWriterKey(region string, sector int) string {
	if region == "inner" {
		return "inner"
	}
	return fmt.Sprintf("outer_%d", sector)
}

// sevenHoleFileSuffix 拼接七孔 CSV 文件命名后缀（spec §7.1 数据集命名约定）
//
// 命名规则：
//   - 内区："_小角度区"（spec §7.1 示例 "(小角度区)"）
//   - 外区 n："_大角度<n>区"（spec §7.1 示例 "(大角度1区)"）
//
// GBK 兼容性（project_memory §36）：所有字符均为 GBK 支持的中文，俄文系统 Excel 不乱码。
func sevenHoleFileSuffix(region string, sector int) string {
	if region == "inner" {
		return "_小角度区"
	}
	return fmt.Sprintf("_大角度%d区", sector)
}

// flushAllSevenHoleWriters 刷新并关闭所有七孔 CSV writer
//
// 调用时机：
//   - 异步 goroutine 中校准完成（Completed/Error）
//   - Stop 方法中用户主动停止
//
// 并发安全（code-review C1 修复）：
//   - 锁内仅"摘取 map + 置 nil"两步原子操作（O(1) 短临界区）
//   - 锁外逐个 Flush writer（I/O 可能阻塞，避免持 m.mu 阻塞 Status 等查询路径）
//   - 摘取后 routeSevenHoleWriter 看到 sevenHoleWriters==nil，新数据点 CSV 写入会被跳过
//
// 错误处理：单个 writer flush 失败仅记录日志，不阻塞其他 writer 关闭——
// 避免一个文件损坏导致其他文件无法落盘。
func (m *CalibrationManager) flushAllSevenHoleWriters() {
	m.mu.Lock()
	writers := m.sevenHoleWriters
	m.sevenHoleWriters = nil
	m.mu.Unlock()

	if writers == nil {
		return
	}
	for key, writer := range writers {
		if err := writer.Flush(); err != nil {
			slog.Error("calibration seven hole csv writer flush failed",
				"component", "calibration", "key", key, "error", err)
		}
	}
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
	if !m.pauseStartedAt.IsZero() {
		status.PausedDurationMs += time.Since(m.pauseStartedAt).Milliseconds()
	}
	if status.DataPoints != nil {
		status.DataPoints = append([]calibration.DataPoint(nil), status.DataPoints...)
	}
	// 引擎引用必须在锁内获取（Start 在写锁下发布 m.autoEngine），
	// 解锁后再调用引擎自有锁保护的方法，消除对 m.autoEngine 的无同步读。
	engine := m.autoEngine
	// currentConfig 必须在锁内拷贝：Start 在写锁下发布 m.currentConfig，
	// 解锁后 resolveLivePhysics 通过通道读取外部 I/O（m.reader / m.runtime），
	// 避免持锁调用外部接口导致死锁或长锁。
	config := m.currentConfig
	m.mu.RUnlock()
	// 附加当前点采样进度与当前目标点索引：从 autoEngine 读取算法采集循环实时更新的状态。
	// autoEngine 为 nil（未启动/总温手动模式）时跳过。
	//
	// CurrentPoint 用 currentPointIdx（循环顶部推进）而非 CompletedPoints：
	// currentPointIdx 在 processPoint 循环顶部就推进，早于 moveToPoint，
	// 前端据此显示"目标角度"，能在运动控制器移动前就更新到下一个目标点，
	// 符合校准员"目标先行于实际"的直觉。CompletedPoints 仍代表"已完成采集的点数"。
	if engine != nil {
		current, total := engine.GetSampleProgress()
		status.CurrentSample = current
		status.SamplesPerPoint = total
		status.CurrentPoint = engine.GetCurrentPointIndex()
		// 七孔流场分区当前状态（spec Task 11）：供前端 5Hz 轮询 status 时展示
		// "当前区域 inner / 扇区 3"。其他类型返回零值（omitempty 自动省略）。
		status.CurrentRegion = engine.GetCurrentRegion()
		status.CurrentSector = engine.GetCurrentSector()
	}
	// Task 13：实时物理量快照在锁外计算，绝不写入 m.currentStatus（避免 stale 残留 +
	// writer 污染）。每次调用都即时读取 m.reader，设备离线时自动返回 nil 字段。
	//
	// 终态不再计算 LivePhysics（review P1 缺陷修复）：
	//   - completed/error/stopped 三态下 currentConfig 仍指向旧任务（Stop 只切状态不清 config），
	//     若继续计算，reader 仍在线时会返回最后一帧的实时 Ma/V，前端 store 会保留这个数值
	//     直到下一次轮询，与"终态后端已 StaleClearing"的注释相矛盾。
	//   - 前端 updateStatusFromBackend 终态分支会 stopStatusPolling，导致这一帧 stale physics
	//     永久停留在 UI 上，给操作员"任务还在跑"的错觉。
	//   - 修复：只在 running/paused 时组装 LivePhysics；终态下 status.LivePhysics 保持 nil。
	//   - idle 态也不组装：前端 calibrationStore.calculateAtmosphericPhysics 已本地算出 Ma/V，
	//     后端无需在 idle 态提供 livePhysics，避免依赖 currentConfig 在 idle 态的不可靠性。
	if status.State == calibration.StateRunning || status.State == calibration.StatePaused {
		if lp := m.resolveLivePhysics(config); lp != nil {
			status.LivePhysics = lp
		}
	}
	return status
}

// resolveLivePhysics 基于当前 config 的探针通道配置和 reader 实时数据，
// 计算马赫数/速度快照。必须在 m.mu 解锁后调用（持锁访问 m.reader 是外部 I/O，违反锁外读取约束）。
//
// 返回值语义（Task 13 spec）：
//   - nil：类型不支持实时物理量（总温）或 currentConfig 未配置探针通道
//   - &LivePhysics{nil, nil}：类型支持且通道已配置，但运行期读取失败/必需指针通道缺失
//   - &LivePhysics{&0, &0}：零流量（Pt == Ps，Task 12）—— 有效零，非缺失
//   - &LivePhysics{&ma, &v}：正常计算值
//
// 物理量计算口径：
//   - 五孔/三孔/七孔：PTotal/PStatic 是表压（A 基准），需 A→C 边界转换（+PAtm → 绝压）后
//     委托 AtmosphericDataCalculator.CalculateAll，与既有 calcMachAndVelocity 同口径。
//   - 总压：PTunnelTotal/PTunnelStatic 是表压，同样 A→C 转换。
//   - TAT 优先级：TTunnel > TAtm（spec §4.4），与既有 calcMachAndVelocity/CalculateTotalPressureCoefficients 一致。
//   - 七孔 TTunnel 通道映射由 Task 13 在 ReadProbeChannelsToSevenHoleRaw 补齐。
func (m *CalibrationManager) resolveLivePhysics(config calibration.Config) *calibration.LivePhysics {
	// 类型筛选：总温无 Pt/Ps 物理量概念，不支持实时物理量
	switch calibration.CalibrationType(config.Type) {
	case calibration.TypeFiveHole, calibration.TypeThreeHole,
		calibration.TypeTotalPressure, calibration.TypeSevenHole:
		// 继续
	default:
		return nil
	}

	if len(config.ProbeChannels) == 0 {
		return nil
	}

	channelReader := m.makeChannelReader()

	switch calibration.CalibrationType(config.Type) {
	case calibration.TypeFiveHole:
		raw, err := calibration.ReadProbeChannelsToFiveHoleRaw(config.ProbeChannels, channelReader)
		if err != nil {
			// 通道读取失败（设备离线/必需通道缺失）→ 返回空快照而非整体 nil，
			// 让前端区分"类型支持但当前无数据"与"类型不支持"。
			return &calibration.LivePhysics{}
		}
		// review P1 缺陷修复：传入 raw.TTunnel，使 TAT 优先级 TTunnel > TAtm 在五孔实时物理量
		// 路径生效（与七孔/总压一致）。未配置 tTunnel 通道时 raw.TTunnel 为 nil，
		// computeLivePhysicsFromGauge 自动回退 TAtm。
		return computeLivePhysicsFromGauge(raw.PAtm, raw.TAtm, raw.PTotal, raw.PStatic, raw.TTunnel)
	case calibration.TypeThreeHole:
		raw, err := calibration.ReadProbeChannelsToThreeHoleRaw(config.ProbeChannels, channelReader)
		if err != nil {
			return &calibration.LivePhysics{}
		}
		return computeLivePhysicsFromGauge(raw.PAtm, raw.TAtm, raw.PTotal, raw.PStatic)
	case calibration.TypeTotalPressure:
		raw, err := calibration.ReadProbeChannelsToTotalPressureRaw(config.ProbeChannels, channelReader)
		if err != nil {
			return &calibration.LivePhysics{}
		}
		// 总压 PTunnelTotal/PTunnelStatic 是非指针 float64（必需），取地址复用助手。
		// TTunnel 为 0 视为未配置（TotalPressureRawData.TTunnel 是非指针 float64，
		// 0°C 是合法温度但生产环境极端罕见，与既有 CalculateTotalPressureCoefficients 同口径）。
		var ttunnelPtr *float64
		if raw.TTunnel != 0 {
			tt := raw.TTunnel
			ttunnelPtr = &tt
		}
		return computeLivePhysicsFromGauge(raw.PAtm, raw.TAtm, &raw.PTunnelTotal, &raw.PTunnelStatic, ttunnelPtr)
	case calibration.TypeSevenHole:
		raw, err := calibration.ReadProbeChannelsToSevenHoleRaw(config.ProbeChannels, channelReader)
		if err != nil {
			return &calibration.LivePhysics{}
		}
		return computeLivePhysicsFromGauge(raw.PAtm, raw.TAtm, raw.PTotal, raw.PStatic, raw.TTunnel)
	}
	return nil
}

// computeLivePhysicsFromGauge 通用马赫数/速度计算助手（表压 → 绝压 → AtmosphericDataCalculator）。
//
// 参数：
//   - pAtm：大气压力（绝压，必须 > 0，否则返回 nil 字段）
//   - tAtmC：大气温度（°C，作为 TAT 兜底）
//   - pTotalPtr：风洞总压表压指针，nil → 字段缺失
//   - pStaticPtr：风洞静压表压指针，nil → 字段缺失
//   - ttunnelPtrs：风洞温度指针（可选 variadic），nil/省略 → 回退 tAtmC；非 nil 优先使用
//
// 返回 *LivePhysics（绝不返回 nil 整体——调用方负责决定是否支持类型）：
//   - 字段 nil：pAtm ≤ 0 或 pTotal/pStatic 任一缺失或物理非法（ptAbs < psAbs）
//   - 字段 &0：ptAbs == psAbs（零流量，Task 12）
//   - 字段 &ma/&v：正常计算
//
// 实现委托给 AtmosphericDataCalculator（Task 12 已对零流量返回 Ma=0, nil），
// 与五孔/三孔/总压/七孔既有路径保持同口径。
func computeLivePhysicsFromGauge(
	pAtm, tAtmC float64,
	pTotalPtr, pStaticPtr *float64,
	ttunnelPtrs ...*float64,
) *calibration.LivePhysics {
	lp := &calibration.LivePhysics{}

	// 大气压非法或 Pt/Ps 任一缺失 → 字段保持 nil（缺失语义）
	if pAtm <= 0 || pTotalPtr == nil || pStaticPtr == nil {
		return lp
	}

	// A→C 边界转换：表压 → 绝压
	ptAbs := *pTotalPtr + pAtm
	psAbs := *pStaticPtr + pAtm

	// TAT 优先级：TTunnel > TAtm（spec §4.4）
	tatC := tAtmC
	if len(ttunnelPtrs) > 0 && ttunnelPtrs[0] != nil {
		tatC = *ttunnelPtrs[0]
	}
	tatK := tatC + 273.15
	if tatK <= 0 {
		return lp
	}

	calc := calibration.NewAtmosphericDataCalculator()
	result, err := calc.CalculateAll(ptAbs, psAbs, tatK)
	if err != nil {
		// Pt < Ps 等物理非法 → 字段保持 nil（不报错，Status 路径不传播计算错误）
		return lp
	}

	ma := result.MachNumber
	v := result.TASMach
	lp.MachNumber = &ma
	lp.Velocity = &v
	return lp
}

func (m *CalibrationManager) settlePauseDurationLocked(now time.Time) {
	if m.pauseStartedAt.IsZero() {
		return
	}
	m.currentStatus.PausedDurationMs += now.Sub(m.pauseStartedAt).Milliseconds()
	m.pauseStartedAt = time.Time{}
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
	case calibration.TypeSevenHole:
		// 七孔算法是空结构体（无状态），所有跨点信息（滞回状态、实时回调）通过 Config 注入。
		// 与五孔/三孔/总压一样，启动前由 Manager 在 processPoint 流程中注入 RealtimeCallback/PrevRegion/PrevSector（Task 10 落地）。
		return calibration.NewSevenHoleAlgorithm(), nil
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
	// 调用方（Start）已持 m.mu 写锁，此处不再加锁——避免写锁内嵌套读锁导致死锁。
	// acquisitionController 在 Start 持锁期间不会被并发替换（SetAcquisitionController
	// 仅在装配阶段调用，运行期不切换），直接读字段安全。
	acqCtrl := m.acquisitionController
	if m.runtime != nil {
		return &runtimeAdapter{runtime: m.runtime, acquisitionController: acqCtrl}
	}
	return &fallbackRuntime{
		reader:                m.reader,
		motion:                m.motion,
		motionSafety:          motionSafety,
		acquisitionController: acqCtrl,
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

// makeAcquisitionStateProvider 创建设备采集态查询函数（手动模式 AcquireCurrentPoint 用）。
//
// 自动模式（AutomaticCalibration）通过 RuntimeAccess.IsAcquiring 走 runtimeAdapter/fallbackRuntime
// 路径，无需本函数；手动模式直接调 AcquireDataWithChannels，需要单独构造 acquiringCheck。
//
// acquisitionController 未注入时返回 nil，算法侧回退到原超时失败行为（向后兼容）。
func (m *CalibrationManager) makeAcquisitionStateProvider() calibration.AcquisitionStateProvider {
	m.mu.RLock()
	ctrl := m.acquisitionController
	m.mu.RUnlock()
	if ctrl == nil {
		return nil
	}
	return func(deviceID string) bool {
		return ctrl.IsAcquiring(deviceID)
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
			slog.Warn("calibration return to home failed",
				"component", "calibration",
				"controller_id", axis.ControllerID, "axis", axis.Axis, "error", err)
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

	slog.Error("calibration failed",
		"component", "calibration", "task_id", taskID, "code", code, "message", message)
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
	// spec Task 22：运动安全故障关键字段独立可检索——controller/axis/verdict/target/
	// actual/deviation/point_index/requires_emergency_stop 均为独立 slog 字段。
	slog.Error("calibration motion safety failure",
		"component", "calibration",
		"controller_id", failure.ControllerID,
		"axis", failure.Axis,
		"verdict", failure.Verdict,
		"target", failure.Target,
		"actual", failure.Actual,
		"deviation", deviation,
		"point_index", failure.PointIndex,
		"requires_emergency_stop", failure.Verdict.RequiresEmergencyStop())

	// 1. 急停类裁决 → EmergencyStopProvider 类型断言
	var stopErr error
	if failure.Verdict.RequiresEmergencyStop() {
		if es, ok := runtime.(ports.EmergencyStopProvider); ok {
			stopErr = es.EmergencyStopMotion()
			if stopErr != nil {
				// 急停失败：错误码升级为 ErrEmergencyStopFailed，fallback 到 StopMotion
				slog.Warn("calibration emergency stop failed, fallback to normal stop",
					"component", "calibration",
					"controller_id", failure.ControllerID, "axis", failure.Axis,
					"error", stopErr)
				if fallbackErr := runtime.StopMotion(); fallbackErr != nil {
					// 急停与回退停止双失败：两个根因都必须保留——
					// 错误链用 errors.Join 聚合（双 errors.Is 均可识别），
					// 用户消息同时写入两个根因，避免 fallback 失败被静默吞掉。
					slog.Error("calibration fallback stop also failed",
						"component", "calibration",
						"controller_id", failure.ControllerID, "axis", failure.Axis,
						"error", fallbackErr)
					m.failWithCode("motion safety failure (verdict=%s axis=%s target=%.3f actual=%.3f) and emergency stop failed: %v; fallback stop also failed: %v",
						string(traversal.ErrEmergencyStopFailed),
						failure.Verdict, failure.Axis, failure.Target, failure.Actual, stopErr, fallbackErr)
					m.recordMotionSafetyFailure(failure)
					return fmt.Errorf("emergency stop failed after %s: %w", failure.Verdict, errors.Join(stopErr, fallbackErr))
				}
				m.failWithCode("motion safety failure (verdict=%s axis=%s target=%.3f actual=%.3f) and emergency stop also failed: %v",
					string(traversal.ErrEmergencyStopFailed),
					failure.Verdict, failure.Axis, failure.Target, failure.Actual, stopErr)
				// 急停失败路径同样写入故障快照：前端需要据此区分"急停调用失败"场景
				m.recordMotionSafetyFailure(failure)
				return fmt.Errorf("emergency stop failed after %s: %w", failure.Verdict, stopErr)
			}
		} else {
			// runtime 不支持急停，fallback 到 StopMotion
			slog.Warn("calibration runtime does not support emergency stop, fallback to normal stop",
				"component", "calibration",
				"controller_id", failure.ControllerID, "axis", failure.Axis)
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

func (a *eventPublisherAdapter) OnRegionChanged(event calibration.RegionChangedEvent) {
	a.publisher.PublishRegionChanged(event)
}

// noopEventPublisher 空事件发布器
type noopEventPublisher struct{}

func (n *noopEventPublisher) OnProgress(_ calibration.ProgressEvent)           {}
func (n *noopEventPublisher) OnComplete(_ calibration.CompleteEvent)           {}
func (n *noopEventPublisher) OnRealtime(_ calibration.RealtimeEvent)           {}
func (n *noopEventPublisher) OnRegionChanged(_ calibration.RegionChangedEvent) {}

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
	runtime               ports.CalibrationRuntime
	acquisitionController ports.AcquisitionController
}

func (r *runtimeAdapter) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	return r.runtime.GetChannelValue(deviceID, channelIndex)
}

func (r *runtimeAdapter) GetLatestTimestamp(deviceID string) (int64, bool) {
	return r.runtime.GetLatestTimestamp(deviceID)
}

// IsAcquiring 返回设备采集态。runtimeAdapter 包装的 ports.CalibrationRuntime 不强制暴露采集态，
// 由 CalibrationManager.createRuntime 在构造时按需注入 acquisitionController。
// acquisitionController 为 nil 时按"在采集"处理，算法走原超时失败路径（向后兼容）。
func (r *runtimeAdapter) IsAcquiring(deviceID string) bool {
	if r.acquisitionController == nil {
		return true
	}
	return r.acquisitionController.IsAcquiring(deviceID)
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
	reader                ports.LatestDataReader
	motion                ports.MotionManager
	mu                    sync.Mutex
	targets               map[calibrationMotionAxis]float64
	motionSafety          *traversal.MotionSafetyConfig
	isPaused              func() bool
	acquisitionController ports.AcquisitionController
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

// IsAcquiring 返回设备采集态。acquisitionController 为 nil 时按"在采集"处理，
// 算法走原超时失败路径（向后兼容未注入场景）。
func (f *fallbackRuntime) IsAcquiring(deviceID string) bool {
	if f.acquisitionController == nil {
		return true
	}
	return f.acquisitionController.IsAcquiring(deviceID)
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
			slog.Error("calibration emergency stop failed",
				"component", "calibration", "controller_id", controllerID, "error", err)
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
//
// 为什么用 3 秒有界超时而不是 context.Background()：
// ServiceShutdown 在 Wails v3 主线程同步执行（application.cleanup 经 InvokeSync 投递），
// 此函数串行调用 StatusAll + 每轴 Stop，任一硬件卡住就会阻塞 GUI 主线程，
// 表现为"退出确认后程序无响应"。B140 单命令已有 5 秒 watchdog 兜底，但 queryStatus
// 串行多个命令会累积到 ~70 秒。
//
// 3 秒超时生效路径（B140 sendCommand 的 ctx 取消语义，见 b140_motion.go:1450-1463）：
//   - 未启动的新命令：立即返回 ctx.Err()（watchdogTimeout≤0 分支）
//   - 当前正在执行的命令：进入 case <-ctx.Done() 后仍需 r := <-done 等 watchdog
//     Close conn 解除 Read 阻塞，最长 5 秒
// 因此最坏路径 = 3s 等首命令 ctx 取消 + 5s 等该命令 watchdog Close = 8s 内完成退出。
func stopAllMotion(mgr ports.MotionManager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var firstErr error
	for _, status := range mgr.StatusAll(ctx) {
		for _, axis := range status.Axes {
			if axis.Moving {
				if err := mgr.Stop(ctx, status.ID, axis.Name); err != nil {
					slog.Warn("calibration stop motion failed",
						"component", "calibration",
						"controller_id", status.ID, "axis", axis.Name, "error", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		}
	}
	return firstErr
}
