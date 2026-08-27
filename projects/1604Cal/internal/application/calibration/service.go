package calibration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"cal1604/internal/application/session"
	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/events"
	"cal1604/internal/workflow"
)

// defaultAlarmService 用于报警判定的服务实例。
var defaultAlarmService = workflow.NewAlarmService()

// CalibrationSession 是 domain.WorkflowSession 的类型别名，保持外部包向后兼容。
type CalibrationSession = domain.WorkflowSession

// FittingResult 拟合结果。
type FittingResult struct {
	Slope     float64 `json:"slope"`     // 斜率
	Intercept float64 `json:"intercept"` // 截距
	R2        float64 `json:"r2"`        // 拟合优度 R²
	Points    int     `json:"points"`    // 参与拟合的数据点数
}

// CalibrationResult 校准结果。
type CalibrationResult struct {
	Success       bool                `json:"success"`
	State         domain.SessionState `json:"state"`
	CollectedData map[int][]float64   `json:"collectedData,omitempty"` // pointIndex -> channelData
	Error         string              `json:"error,omitempty"`
}

var (
	// ErrMeasureDeviceNotSet 表示计量设备驱动尚未绑定。
	ErrMeasureDeviceNotSet = errors.New("measure device not set")
	// ErrPressureDeviceNotSet 表示打压设备驱动尚未绑定。
	ErrPressureDeviceNotSet = errors.New("pressure device not set")
	// ErrPointSkipped 表示用户选择跳过当前点。
	ErrPointSkipped = errors.New("point skipped by user")
	// ErrInvalidStartState 表示当前会话状态不允许开始标定。
	ErrInvalidStartState = errors.New("invalid session state for start")
	// ErrNoPendingAlarm 表示当前没有待处理的报警。
	ErrNoPendingAlarm = errors.New("no pending alarm")
	// ErrAutoCollectionRunning 表示自动采集已在运行中。
	ErrAutoCollectionRunning = errors.New("auto collection already running")
	// errAutoCollectionStopped 表示自动采集因报警决策主动停止。
	errAutoCollectionStopped = errors.New("auto collection stopped by alarm decision")
)

// StatusPublisher 广播事件。
type StatusPublisher func(eventType string, data any)

// StartPrerequisiteConfig 定义标定启动门禁配置。
type StartPrerequisiteConfig struct {
	EnforceValveCalibration bool
}

func defaultStartPrerequisiteConfig() StartPrerequisiteConfig {
	// 「阀门=校准模式」是标定与计量启动的必要条件，默认开启门禁。
	// 联调阶段如需放开，可通过 calibration.enforceValveCalibrationGate=false 关闭。
	return StartPrerequisiteConfig{EnforceValveCalibration: true}
}

// Service 校准流程编排服务。
type Service struct {
	mu             sync.Mutex
	coordinator    *workflow.WorkflowCoordinator
	factory        device.DriverFactory
	deviceManager  device.DeviceStore
	driverProvider device.ActiveDriverProvider
	sessionService *session.Service

	measureDrivers map[string]device.MeasureDriver
	measureDevIDs  []string
	pressureDriver device.PressureDriver
	measureDevID   string
	pressureDevID  string

	config             domain.WorkflowConfig
	alarmConfig        domain.AlarmConfig
	pressurePoints     []domain.PressurePoint
	currentPoint       int
	calibrationSession *CalibrationSession

	// skippedDevices 记录本批次中被用户永久跳过的计量设备（deviceID -> 跳过原因）。
	skippedDevices map[string]string

	// autoCollectionCtx 用于控制自动采集 goroutine 的生命周期。
	autoCollectionCtx    context.Context
	autoCollectionCancel context.CancelFunc
	autoCollectionMu     sync.Mutex
	autoCollectWg        sync.WaitGroup

	// alarmCh 用于在自动采集过程中阻塞等待用户确认报警。
	alarmMu      sync.Mutex
	alarmCh      chan string
	alarmPending bool

	// skipDeviceCh 用于在报警等待期间接收"跳过指定设备"的用户决策。
	skipDeviceCh chan string

	// stabilityTimeoutCh 用于等待前端用户对稳定超时的决定。
	stabilityTimeoutCh chan string

	publish StatusPublisher

	startPrerequisiteConfig StartPrerequisiteConfig
}

// NewService 创建校准服务。
func NewService(
	coordinator *workflow.WorkflowCoordinator,
	factory device.DriverFactory,
	deviceManager device.DeviceStore,
	publisher StatusPublisher,
	driverProvider device.ActiveDriverProvider,
	sessionSvc *session.Service,
) *Service {
	if publisher == nil {
		publisher = func(string, any) {}
	}
	return &Service{
		coordinator:             coordinator,
		factory:                 factory,
		deviceManager:           deviceManager,
		publish:                 publisher,
		driverProvider:          driverProvider,
		sessionService:          sessionSvc,
		stabilityTimeoutCh:      make(chan string, 1),
		skipDeviceCh:            make(chan string, 1),
		startPrerequisiteConfig: defaultStartPrerequisiteConfig(),
		measureDrivers:          make(map[string]device.MeasureDriver),
		skippedDevices:          make(map[string]string),
	}
}

// SetStartPrerequisiteConfig 设置标定启动门禁配置。
func (s *Service) SetStartPrerequisiteConfig(cfg StartPrerequisiteConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startPrerequisiteConfig = cfg
}

// getMeasureDriver 返回首个计量驱动；session 为 nil 时回退到本地字段。
// 注意：调用方必须持有 s.mu（或保证字段不被并发修改）。
func (s *Service) getMeasureDriver() device.MeasureDriver {
	if s.sessionService != nil {
		return s.sessionService.MeasureDriver()
	}
	if len(s.measureDevIDs) == 0 {
		return nil
	}
	return s.measureDrivers[s.measureDevIDs[0]]
}

// MeasureDeviceIDs 返回本批次绑定的全部计量设备 ID（保持绑定顺序）。
// 供 HTTP 层从设备维度结果中提取首个设备数据（兼容字段）使用。
func (s *Service) MeasureDeviceIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessionService != nil {
		return s.sessionService.MeasureDeviceIDs()
	}
	return append([]string(nil), s.measureDevIDs...)
}

// getMeasureDrivers 返回全部计量驱动（设备 ID → 驱动）。
// 优先从 session 服务获取，session 为 nil 时回退到本地字段。
// 注意：调用方必须持有 s.mu（或保证字段不被并发修改）。
func (s *Service) getMeasureDrivers() map[string]device.MeasureDriver {
	if s.sessionService != nil {
		return s.sessionService.MeasureDrivers()
	}
	result := make(map[string]device.MeasureDriver, len(s.measureDrivers))
	for k, v := range s.measureDrivers {
		result[k] = v
	}
	return result
}

// isDeviceBound 判断指定设备是否属于当前批次绑定集合。
// 有 sessionService 时以会话绑定为准，否则以本地驱动表为准。调用方必须持有 s.mu。
func (s *Service) isDeviceBound(deviceID string) bool {
	if s.sessionService != nil {
		for _, id := range s.sessionService.MeasureDeviceIDs() {
			if id == deviceID {
				return true
			}
		}
		return false
	}
	_, ok := s.measureDrivers[deviceID]
	return ok
}

// getPressureDriver 从 session 服务获取打压驱动；session 为 nil 时回退到本地字段。
func (s *Service) getPressureDriver() device.PressureDriver {
	if s.sessionService != nil {
		return s.sessionService.PressureDriver()
	}
	return s.pressureDriver
}

// StartCalibration 开始校准流程（WTN1604 多点校准模式）。
// 若 ControlMode 为 auto，则在准备完成后自动启动采集循环。
// 是否校验阀门状态由 startPrerequisiteConfig.EnforceValveCalibration 控制。
func (s *Service) StartCalibration(ctx context.Context) error {
	s.mu.Lock()

	if s.getMeasureDriver() == nil {
		s.mu.Unlock()
		return ErrMeasureDeviceNotSet
	}

	// 单活工作流冲突校验
	if err := s.coordinator.Begin(workflow.OwnerCalibration); err != nil {
		s.mu.Unlock()
		return err
	}

	// 在锁内提取所需配置
	enforceValveGate := s.startPrerequisiteConfig.EnforceValveCalibration
	measureDrivers := s.getMeasureDrivers()
	measureDevIDs := append([]string(nil), s.measureDevIDs...)
	avgCount := s.config.AverageCount
	channels := s.config.Channels
	pointCount := s.config.PointCount
	if avgCount < 1 {
		avgCount = 1
	}

	// 状态应为 ready（coordinator.Begin 已确保）
	if s.coordinator.State() != domain.SessionStateReady {
		s.coordinator.End()
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrInvalidStartState, s.coordinator.State())
	}

	s.mu.Unlock()

	// 阀门门禁：必须在锁外执行，避免 TCP I/O（最长 3s）阻塞会话锁，
	// 影响其他并发请求。失败 / 非校准态都要回滚 coordinator，
	// 并统一 wrap ErrPrerequisiteNotMet，让 HTTP 层映射到 409 PREREQUISITE_NOT_MET。
	if enforceValveGate {
		if err := device.CheckValveCalibrationGate(ctx, measureDrivers); err != nil {
			s.coordinator.End()
			return err
		}
	}

	// 每台支持校准的计量设备依次开始多点校准（不持有锁，避免 I/O 阻塞影响其他操作）
	for devID, drv := range measureDrivers {
		calDev, ok := drv.(device.CalibrationCapable)
		if !ok {
			continue
		}
		if err := calDev.StartCalibration(ctx, channels, pointCount, avgCount); err != nil {
			s.coordinator.End()
			return fmt.Errorf("start WTN1604 calibration on %s: %w", devID, err)
		}
	}

	// 初始化校准会话记录
	s.mu.Lock()
	s.calibrationSession = &CalibrationSession{
		ID:               fmt.Sprintf("cal-%d", time.Now().UnixMilli()),
		StartTime:        time.Now(),
		Config:           s.config,
		Points:           s.pressurePoints,
		MeasureDeviceID:  s.measureDevID,
		MeasureDeviceIDs: measureDevIDs,
		PressureDeviceID: s.pressureDevID,
		Status:           "running",
	}
	controlMode := s.config.ControlMode
	s.mu.Unlock()

	if controlMode == domain.ControlModeAuto {
		if err := s.RunAutoCollection(ctx); err != nil {
			s.coordinator.End()
			return fmt.Errorf("start auto collection: %w", err)
		}
	}

	return nil
}

// ValidateStartPrerequisites 校验标定启动前置条件是否满足。
// 检查项：计量设备已绑定、通道已选择、配置有效（阀门门禁按配置可开关）。
// 用于 session/start 端点在状态迁移前进行门禁拦截。
func (s *Service) ValidateStartPrerequisites(ctx context.Context) error {
	s.mu.Lock()

	measureDrivers := s.getMeasureDrivers()
	pressureDriver := s.getPressureDriver()
	channels := s.config.Channels
	config := s.config
	enforceValveGate := s.startPrerequisiteConfig.EnforceValveCalibration
	s.mu.Unlock()

	if len(measureDrivers) == 0 {
		return fmt.Errorf("measure device not bound")
	}

	if len(channels) == 0 {
		return fmt.Errorf("no channels selected")
	}

	if config.PointCount < 2 || config.PointCount > 5 {
		return fmt.Errorf("pressure points must be between 2 and 5, got %d", config.PointCount)
	}

	// 按模式化门禁校验打压设备
	if config.ControlMode == domain.ControlModeAuto && pressureDriver == nil {
		return fmt.Errorf("auto mode requires pressure device to be bound")
	}

	if enforceValveGate {
		if err := device.CheckValveCalibrationGate(ctx, measureDrivers); err != nil {
			return err
		}
	}

	// 检查已连接设备单位是否一致
	if s.deviceManager != nil {
		consistent, conflictIDs := s.deviceManager.CheckUnitConsistency()
		if !consistent {
			return fmt.Errorf("device unit mismatch among connected devices: %v", conflictIDs)
		}
	}

	return nil
}

// EndCalibration 结束校准流程，执行确定性资源清理。
// 停止自动采集循环、停止压力控制、结束 WTN1604 校准。
// 不自动切阀（保留人工回阀路径），不持有锁时执行设备 I/O。
func (s *Service) EndCalibration(ctx context.Context) error {
	s.StopAutoCollection()

	// 先获取驱动引用（持有锁），I/O 操作在锁外执行
	s.mu.Lock()
	measureDrivers := s.getMeasureDrivers()
	pressureDriver := s.getPressureDriver()
	s.mu.Unlock()

	// 每台支持校准的计量设备依次结束校准（不持有锁，避免 I/O 阻塞影响其他操作）
	for _, drv := range measureDrivers {
		if calDev, ok := drv.(device.CalibrationCapable); ok {
			_ = calDev.EndCalibration(ctx)
		}
	}

	// 停止压力控制
	if pressureDriver != nil {
		_ = pressureDriver.Stop(ctx)
	}

	// 关闭校准会话记录
	s.mu.Lock()
	if s.calibrationSession != nil {
		now := time.Now()
		s.calibrationSession.EndTime = &now
		s.calibrationSession.Status = "completed"
		s.calibrationSession.Points = s.pressurePoints
	}
	s.mu.Unlock()

	s.coordinator.End()

	return nil
}

// RunAutoCollection 启动自动采集流程，按压力点列表依次执行打压、稳定、采集。
// 该方法在后台 goroutine 中运行，通过 context 控制取消。
func (s *Service) RunAutoCollection(ctx context.Context) error {
	s.autoCollectionMu.Lock()
	if s.autoCollectionCancel != nil {
		s.autoCollectionMu.Unlock()
		return ErrAutoCollectionRunning
	}
	// 使用 context.Background() 而非传入的 ctx（HTTP 请求上下文），
	// 因为 HTTP handler 返回后 r.Context() 会被立即取消，导致自动采集 goroutine 立即退出。
	s.autoCollectionCtx, s.autoCollectionCancel = context.WithCancel(context.Background())
	s.autoCollectionMu.Unlock()

	s.autoCollectWg.Add(1)
	go s.runCollectionLoop()
	return nil
}

// runCollectionLoop 在后台 goroutine 中执行自动采集循环。
func (s *Service) runCollectionLoop() {
	defer s.autoCollectWg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[calibration] auto collection PANIC: %v", r)
			s.publish(events.EventAutoCollectionError, map[string]any{
				"error": fmt.Sprintf("panic: %v", r),
			})
		}
	}()
	defer s.cleanupAutoCollection()

	s.mu.Lock()
	startIndex := s.resumePointIndexLocked()
	totalPoints := len(s.pressurePoints)
	mode := s.config.ControlMode
	s.currentPoint = startIndex
	s.mu.Unlock()

	s.publish(events.EventAutoCollectionStarted, map[string]any{
		"pointCount": totalPoints,
		"mode":       mode,
		"startIndex": startIndex + 1,
	})

	s.executePointLoop(startIndex, totalPoints)
	s.autoVentAfterCollection(totalPoints)
}

// cleanupAutoCollection 清理自动采集的状态标记。
func (s *Service) cleanupAutoCollection() {
	s.autoCollectionMu.Lock()
	s.autoCollectionCancel = nil
	s.autoCollectionCtx = nil
	s.autoCollectionMu.Unlock()
}

// autoVentAfterCollection 采集完成后自动排空打压设备。
func (s *Service) autoVentAfterCollection(totalPoints int) {
	s.autoCollectionMu.Lock()
	ctx := s.autoCollectionCtx
	s.autoCollectionMu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return
	}
	if pressureDriver := s.getPressureDriver(); pressureDriver != nil {
		_ = pressureDriver.Stop(ctx)
	}
	s.publish(events.EventAutoCollectionCompleted, map[string]any{
		"totalPoints": totalPoints,
	})
}

// executePointLoop 遍历压力点列表执行采集。
func (s *Service) executePointLoop(startIndex, totalPoints int) {
	for i := startIndex; i < totalPoints; i++ {
		s.autoCollectionMu.Lock()
		ctx := s.autoCollectionCtx
		s.autoCollectionMu.Unlock()
		if ctx == nil || ctx.Err() != nil {
			break
		}

		s.mu.Lock()
		s.currentPoint = i
		s.mu.Unlock()

		if err := s.collectPoint(ctx, i+1); err != nil {
			s.handlePointError(i, err)
			return
		}

		s.mu.Lock()
		s.currentPoint = i + 1
		s.mu.Unlock()
	}
}

// handlePointError 处理采集过程中的错误，决定是否继续或终止。
func (s *Service) handlePointError(i int, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}

	if errors.Is(err, errAutoCollectionStopped) {
		s.publish(events.EventAutoCollectionStopped, map[string]any{
			"pointIndex": i + 1,
		})
		return
	}

	s.publish(events.EventAutoCollectionError, map[string]any{
		"pointIndex": i + 1,
		"error":      err.Error(),
	})
	_ = s.coordinator.Machine().Transition(domain.SessionStateError)
	s.publishSessionState()
	if s.calibrationSession != nil {
		s.calibrationSession.Status = "error"
	}
}

// collectPoint 采集单个压力点：打压 -> 稳定监控 -> 采集 -> 报警检查。
// 使用迭代循环替代递归，设置最大重试次数避免栈溢出风险。
func (s *Service) collectPoint(ctx context.Context, pointIndex int) error {
	s.publish(events.EventPointStarted, map[string]any{"pointIndex": pointIndex})

	const maxRetries = 3
	for retry := 0; retry <= maxRetries; retry++ {
		if retry > 0 {
			// 重试前重置压力点状态，重新打压
			s.mu.Lock()
			if pointIndex >= 1 && pointIndex <= len(s.pressurePoints) {
				s.pressurePoints[pointIndex-1].Status = domain.PointStatusPending
				s.pressurePoints[pointIndex-1].CollectedData = nil
				s.pressurePoints[pointIndex-1].ActualPressure = nil
			}
			s.mu.Unlock()
			s.publish(events.EventPointRecollect, map[string]any{"pointIndex": pointIndex})
		}

		if err := s.Pressurize(ctx, pointIndex); err != nil {
			if errors.Is(err, ErrPointSkipped) {
				s.updatePointStatus(pointIndex, domain.PointStatusSkipped)
				s.publish(events.EventPointSkipped, map[string]any{"pointIndex": pointIndex})
				return nil
			}
			return fmt.Errorf("pressurize point %d: %w", pointIndex, err)
		}

		// Pressurize() 返回表示打压已完成且压力稳定，直接进入采集。
		data, err := s.Collect(ctx, pointIndex)
		if err != nil {
			return fmt.Errorf("collect point %d: %w", pointIndex, err)
		}

		// 报警检查
		action, err := s.checkAlarm(ctx, pointIndex, data)
		if err != nil {
			return fmt.Errorf("alarm check point %d: %w", pointIndex, err)
		}

		switch action {
		case workflow.AlarmDecisionRecollect:
			// 继续重试循环
			continue

		case workflow.AlarmDecisionSkip:
			s.mu.Lock()
			if pointIndex >= 1 && pointIndex <= len(s.pressurePoints) {
				s.pressurePoints[pointIndex-1].CollectedData = nil
			}
			s.mu.Unlock()
			s.updatePointStatus(pointIndex, domain.PointStatusSkipped)
			s.publish(events.EventPointSkipped, map[string]any{"pointIndex": pointIndex})
			return nil

		case workflow.AlarmDecisionStop:
			s.publish(events.EventPointStopped, map[string]any{"pointIndex": pointIndex})
			return errAutoCollectionStopped
		}

		// AlarmDecisionContinue — 正常完成
		// 点完成事件携带 deviceId（单设备场景取首个绑定设备，spec 要求）。
		pointDonePayload := map[string]any{"pointIndex": pointIndex, "data": data}
		if devIDs := s.MeasureDeviceIDs(); len(devIDs) > 0 {
			pointDonePayload["deviceId"] = devIDs[0]
		}
		s.publish(events.EventPointCompleted, pointDonePayload)
		return nil
	}

	return fmt.Errorf("point %d exceeded max retries (%d)", pointIndex, maxRetries)
}

// checkAlarm 检查采集数据是否触发报警，若触发则阻塞等待用户决策。
// 多设备场景：优先检测设备级采集失败（采集/超时/断开），其次对每台设备独立评估超限通道，
// 任一设备触发即报警，事件携带 deviceId。设备维度数据在锁内快照，避免并发写竞态。
// 返回决策动作：continue/skip/recollect/stop，或设备级跳过（由 skipDeviceCh 接收）。
func (s *Service) checkAlarm(ctx context.Context, pointIndex int, data []float64) (string, error) {
	s.mu.Lock()
	point := s.pressurePoints[pointIndex-1]
	alarmConfig := s.alarmConfig
	channels := s.config.Channels
	maxPressure := s.config.MaxPressure
	minPressure := s.config.MinPressure
	collectedByDevice := s.snapshotCollectedByDeviceLocked(pointIndex)
	s.mu.Unlock()

	// 单设备兼容：data 为 Collect 返回的单设备数据，直接走原逻辑
	if len(collectedByDevice) == 0 {
		if len(data) == 0 {
			return workflow.AlarmDecisionContinue, nil
		}
		triggered, overLimit, maxDev, details := s.evaluateChannels(alarmConfig, point.TargetPressure, maxPressure, minPressure, channels, data)
		if !triggered {
			return workflow.AlarmDecisionContinue, nil
		}
		return s.awaitAlarmDecision(ctx, pointIndex, point, "", overLimit, maxDev, details, "")
	}

	// 按设备 ID 排序保证确定性
	deviceIDs := make([]string, 0, len(collectedByDevice))
	for devID := range collectedByDevice {
		deviceIDs = append(deviceIDs, devID)
	}
	sort.Strings(deviceIDs)

	// 第一遍：设备级采集失败优先报警（采集/超时/断开，Collected 为空且带错误状态）。
	for _, devID := range deviceIDs {
		d := collectedByDevice[devID]
		if d.Status == domain.PointStatusError || d.Error != "" {
			return s.awaitAlarmDecision(ctx, pointIndex, point, devID, nil, 0, nil, d.Error)
		}
	}

	// 第二遍：逐设备评估通道超限。
	var triggeredDeviceID string
	var triggeredOverLimit []int
	var triggeredMaxDev float64
	var triggeredDetails map[int]float64

	for _, devID := range deviceIDs {
		d := collectedByDevice[devID]
		if len(d.Collected) == 0 {
			continue
		}
		triggered, overLimit, maxDev, details := s.evaluateChannels(alarmConfig, point.TargetPressure, maxPressure, minPressure, channels, d.Collected)
		if triggered {
			triggeredDeviceID = devID
			triggeredOverLimit = overLimit
			triggeredMaxDev = maxDev
			triggeredDetails = details
			break
		}
	}

	if triggeredDeviceID == "" {
		return workflow.AlarmDecisionContinue, nil
	}

	return s.awaitAlarmDecision(ctx, pointIndex, point, triggeredDeviceID, triggeredOverLimit, triggeredMaxDev, triggeredDetails, "")
}

// snapshotCollectedByDeviceLocked 在锁内深拷贝指定压力点的设备维度数据。
// 调用方必须持有 s.mu。返回的快照可安全地在锁外遍历，避免与 ResolveSkipDevice 并发写 map 产生竞态。
func (s *Service) snapshotCollectedByDeviceLocked(pointIndex int) map[string]domain.DevicePointData {
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		return nil
	}
	src := s.pressurePoints[pointIndex-1].CollectedByDevice
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

// evaluateChannels 对单设备通道数据执行报警判定，返回是否触发、超限通道、最大偏差与详情。
func (s *Service) evaluateChannels(alarmConfig domain.AlarmConfig, target, maxPressure, minPressure float64, channels []int, data []float64) (bool, []int, float64, map[int]float64) {
	// 多通道报警判定
	channelData := make(map[int]float64)
	for i, ch := range channels {
		if i < len(data) {
			channelData[ch] = data[i]
		}
	}

	result := defaultAlarmService.EvaluateMultiChannel(alarmConfig, target, maxPressure, minPressure, channelData)
	if !result.Triggered {
		return false, nil, 0, nil
	}
	return true, result.OverLimitChannels, result.MaxDeviation, result.ChannelDetails
}

// awaitAlarmDecision 触发报警后进入等待用户决策状态。
// deviceID 为空表示单设备场景。deviceErr 非空表示该设备采集失败（采集/超时/断开）。
func (s *Service) awaitAlarmDecision(ctx context.Context, pointIndex int, point domain.PressurePoint, deviceID string, overLimit []int, maxDev float64, details map[int]float64, deviceErr string) (string, error) {
	// 触发报警，进入等待报警确认状态
	if err := s.coordinator.Machine().Transition(domain.SessionStateAwaitAlarmResolution); err != nil {
		// 若状态机不支持该迁移，记录日志后继续（不阻塞采集流程）
		log.Printf("[calibration] checkAlarm: transition to await_alarm_resolution failed: %v", err)
		return workflow.AlarmDecisionContinue, nil
	}
	s.publishSessionState()

	s.alarmMu.Lock()
	s.alarmCh = make(chan string, 1)
	s.alarmPending = true
	alarmCh := s.alarmCh
	s.alarmMu.Unlock()

	payload := map[string]any{
		"pointIndex":        pointIndex,
		"targetPressure":    point.TargetPressure,
		"overLimitChannels": overLimit,
		"maxDeviation":      maxDev,
		"channelDetails":    details,
	}
	if deviceID != "" {
		payload["deviceId"] = deviceID
	}
	if deviceErr != "" {
		payload["error"] = deviceErr
	}
	s.publish("alarm.triggered", payload)

	select {
	case deviceID2 := <-s.skipDeviceCh:
		// 用户选择跳过指定设备：整批继续，但该设备从剩余流程移除。
		// 当前点其余设备已采集成功，点整体标记完成（被跳设备保持 error 状态，
		// 其已完成压力点数据保留，剩余点由 executePointLoop 过滤不再采集）。
		s.alarmMu.Lock()
		s.alarmPending = false
		s.alarmCh = nil
		s.alarmMu.Unlock()
		s.updatePointStatus(pointIndex, domain.PointStatusCompleted)
		_ = s.coordinator.Machine().Transition(domain.SessionStatePointDone)
		s.publishSessionState()
		s.publish(events.EventCalibrationAlarmResolved, map[string]any{
			"pointIndex": pointIndex,
			"decision":   "skip_device",
			"deviceId":   deviceID2,
			"triggered":  true,
		})
		return workflow.AlarmDecisionContinue, nil

	case decision := <-alarmCh:
		s.alarmMu.Lock()
		s.alarmPending = false
		s.alarmCh = nil
		s.alarmMu.Unlock()

		if err := defaultAlarmService.ValidateDecision(decision); err != nil {
			return "", err
		}

		s.publish(events.EventCalibrationAlarmResolved, map[string]any{
			"pointIndex": pointIndex,
			"decision":   decision,
			"deviceId":   deviceID,
			"triggered":  true,
		})

		switch decision {
		case workflow.AlarmDecisionStop:
			_ = s.coordinator.Machine().Transition(domain.SessionStateStopped)
			s.publishSessionState()
			return workflow.AlarmDecisionStop, nil
		case workflow.AlarmDecisionRecollect:
			_ = s.coordinator.Machine().Transition(domain.SessionStatePointDone)
			s.publishSessionState()
			return workflow.AlarmDecisionRecollect, nil
		case workflow.AlarmDecisionSkip:
			_ = s.coordinator.Machine().Transition(domain.SessionStatePointDone)
			s.publishSessionState()
			return workflow.AlarmDecisionSkip, nil
		default:
			// 用户确认继续（忽略设备异常）：点收尾为 completed，
			// 失败设备保持 error 状态但不再阻断流程。
			s.updatePointStatus(pointIndex, domain.PointStatusCompleted)
			_ = s.coordinator.Machine().Transition(domain.SessionStatePointDone)
			s.publishSessionState()
			return workflow.AlarmDecisionContinue, nil
		}
	case <-ctx.Done():
		s.alarmMu.Lock()
		s.alarmPending = false
		s.alarmCh = nil
		s.alarmMu.Unlock()
		return "", ctx.Err()
	}
}

// ResolveAlarm 用户确认报警，传入决策动作：continue/skip/recollect/stop。
func (s *Service) ResolveAlarm(decision string) error {
	if err := defaultAlarmService.ValidateDecision(decision); err != nil {
		return err
	}

	s.alarmMu.Lock()
	defer s.alarmMu.Unlock()

	if !s.alarmPending || s.alarmCh == nil {
		return ErrNoPendingAlarm
	}

	select {
	case s.alarmCh <- decision:
		// 事件由 checkAlarm 收到决策后统一发布，避免重复 SSE 推送。
		return nil
	default:
		return fmt.Errorf("alarm channel blocked")
	}
}

// ResolveSkipDevice 用户选择永久跳过指定计量设备。
// 该设备从本批次剩余压力点中移除，已完成压力点数据保留并标记设备级 skipped + 原因。
// 通过 skipDeviceCh 通知阻塞中的采集循环继续。
func (s *Service) ResolveSkipDevice(deviceID, reason string) error {
	s.mu.Lock()
	if !s.isDeviceBound(deviceID) {
		s.mu.Unlock()
		return fmt.Errorf("device %s not bound in this batch", deviceID)
	}
	if _, already := s.skippedDevices[deviceID]; already {
		s.mu.Unlock()
		return fmt.Errorf("device %s already skipped", deviceID)
	}
	s.skippedDevices[deviceID] = reason
	// 剩余压力点（含当前点）标记该设备 skipped + 原因；已完成点的数据保留。
	for i := range s.pressurePoints {
		p := &s.pressurePoints[i]
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
	}
	s.mu.Unlock()

	s.publish(events.EventPointSkipped, map[string]any{
		"deviceId":   deviceID,
		"skipReason": reason,
	})

	select {
	case s.skipDeviceCh <- deviceID:
	default:
	}
	return nil
}

// RetryPoint 重试指定压力点，将其状态重置为 pending 并重新采集。
func (s *Service) RetryPoint(ctx context.Context, pointIndex int) error {
	s.mu.Lock()
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		s.mu.Unlock()
		return fmt.Errorf("invalid point index: %d", pointIndex)
	}
	s.pressurePoints[pointIndex-1].Status = domain.PointStatusPending
	s.pressurePoints[pointIndex-1].CollectedData = nil
	s.pressurePoints[pointIndex-1].ActualPressure = nil
	s.currentPoint = pointIndex - 1
	controlMode := s.config.ControlMode
	hasPressureDriver := s.getPressureDriver() != nil
	s.mu.Unlock()

	s.publish(events.EventPointRetry, map[string]any{"pointIndex": pointIndex})

	// 手动模式且未连接打压设备时，仅重置为待确认，
	// 由操作者再次确认后执行采集，不自动触发打压链路。
	if controlMode == domain.ControlModeManual && !hasPressureDriver {
		return nil
	}

	return s.collectPoint(ctx, pointIndex)
}

// PauseAutoCollection 暂停自动采集，将状态机迁移到 paused。
func (s *Service) PauseAutoCollection() error {
	s.StopAutoCollection()

	if err := s.coordinator.Machine().Transition(domain.SessionStatePaused); err != nil {
		return fmt.Errorf("transition to paused: %w", err)
	}
	s.publishSessionState()
	return nil
}

// ResumeAutoCollection 恢复自动采集，重新启动从当前点的自动采集循环。
func (s *Service) ResumeAutoCollection(ctx context.Context) error {
	s.mu.Lock()
	s.currentPoint = s.resumePointIndexLocked()
	s.mu.Unlock()

	if err := s.coordinator.Machine().Transition(domain.SessionStatePressurizing); err != nil {
		return fmt.Errorf("transition to pressurizing: %w", err)
	}
	s.publishSessionState()
	return s.RunAutoCollection(ctx)
}

// StopAutoCollection 停止自动采集，取消后台 goroutine 并等待退出。
func (s *Service) StopAutoCollection() {
	s.autoCollectionMu.Lock()
	cancel := s.autoCollectionCancel
	s.autoCollectionCancel = nil
	s.autoCollectionCtx = nil
	s.autoCollectionMu.Unlock()

	if cancel != nil {
		cancel()
		s.autoCollectWg.Wait()
	}

	s.alarmMu.Lock()
	if s.alarmPending && s.alarmCh != nil {
		select {
		case s.alarmCh <- workflow.AlarmDecisionStop:
		default:
		}
		s.alarmPending = false
		s.alarmCh = nil
	}
	s.alarmMu.Unlock()
}

// IsAutoCollectionRunning 返回自动采集是否正在运行。
// ResolveStabilityTimeout 接收前端用户对稳定超时的决定。
// decision: "continue" 继续等待， "skip" 跳过当前点。
func (s *Service) ResolveStabilityTimeout(decision string) {
	select {
	case s.stabilityTimeoutCh <- decision:
	default:
	}
}

func (s *Service) IsAutoCollectionRunning() bool {
	s.autoCollectionMu.Lock()
	defer s.autoCollectionMu.Unlock()
	return s.autoCollectionCancel != nil
}

func (s *Service) markPointError(pointIndex int) {
	s.updatePointStatus(pointIndex, domain.PointStatusError)
}

func (s *Service) publishSessionState() {
	s.publish(events.EventSessionStateChanged, map[string]any{
		"state": string(s.coordinator.State()),
	})
}

// updatePointStatus 更新压力点状态并发布统一事件。
func (s *Service) updatePointStatus(pointIndex int, status string) {
	s.mu.Lock()
	if pointIndex < 1 || pointIndex > len(s.pressurePoints) {
		s.mu.Unlock()
		return
	}
	point := &s.pressurePoints[pointIndex-1]
	point.Status = status
	pointSnapshot := *point
	if point.CollectedData != nil {
		pointSnapshot.CollectedData = append([]float64(nil), point.CollectedData...)
	}
	s.mu.Unlock()

	s.publish(events.EventCalibrationPointStatus, pointSnapshot)
}

// resumePointIndexLocked 计算恢复自动采集时的起始压力点下标（0-based）。
func (s *Service) resumePointIndexLocked() int {
	if len(s.pressurePoints) == 0 {
		return 0
	}

	start := s.currentPoint
	if start < 0 {
		start = 0
	}
	if start >= len(s.pressurePoints) {
		return len(s.pressurePoints)
	}

	for i := start; i < len(s.pressurePoints); i++ {
		if !isPointTerminalStatus(s.pressurePoints[i].Status) {
			return i
		}
	}

	for i := 0; i < start; i++ {
		if !isPointTerminalStatus(s.pressurePoints[i].Status) {
			return i
		}
	}

	return len(s.pressurePoints)
}

func isPointTerminalStatus(status string) bool {
	return status == domain.PointStatusCompleted || status == domain.PointStatusSkipped
}
