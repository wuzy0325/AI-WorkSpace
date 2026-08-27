package calibration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"windlabx4/services/api-go/internal/core/traversal"
)

// RuntimeAccess 校准运行时依赖注入接口
// 由 CalibrationManager 注入，提供通道读取、运动控制等能力
//
// WaitForMotionComplete 返回三元组 (completed, reason, failure)：
//   - completed=true, reason=none, failure=nil：所有参与运动的轴已到位
//   - completed=false, failure!=nil：检测到运动安全故障（调用方应调用 onMotionSafetyFailure 回调）
//   - completed=false, failure=nil, reason≠none：因暂停/停止/取消/超时中断
type RuntimeAccess interface {
	GetChannelValue(deviceID string, channelIndex int) (float64, bool)
	GetLatestTimestamp(deviceID string) (int64, bool)
	// IsAcquiring 返回指定设备是否正在持续产帧。
	// 校准算法在 waitForFreshData 超时后调用，区分"用户停采集"（可恢复）与
	// "设备在采集但帧不更新"（真异常）两类场景。实现应直接委托给设备管理器，
	// 不读硬件、不阻塞。
	IsAcquiring(deviceID string) bool
	MoveToPosition(axis MotionAxisConfig, position float64) error
	WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure)
	StopMotion() error
}

// PositionReader 可选能力接口：读取指定运动轴的当前位置。
//
// 相对坐标模式（Config.CoordinateMode == CoordinateModeRelative）下，
// moveToPoint/MoveToPointWithOrder 需要把测点坐标从"位移量"换算为绝对目标
// （目标 = 当前坐标 + 测点坐标），因此运行时必须能返回轴当前位置。
//
// 通过类型断言而非扩充 RuntimeAccess 接口签名，避免破坏既有外部实现
// （与 MotionSafetyAwareRuntime 的可选扩展模式一致）。不实现此接口的运行时
// 在相对坐标模式下返回明确错误，而不是静默降级为绝对坐标。
type PositionReader interface {
	// GetAxisPosition 返回指定运动轴的当前坐标。
	GetAxisPosition(axis MotionAxisConfig) (float64, error)
}

// ErrPointAborted 暂停打断当前测点的哨兵错误。
// runCalibrationLoop 识别此错误后回退循环索引以重跑同一点，
// 不计入 pointErrorCount。
var ErrPointAborted = errors.New("测点被暂停打断")

// ErrMotionControl marks motion failures that must stop the calibration run.
// Continuing to later points would only advance the displayed target while the
// probe remains at an unknown physical position.
var ErrMotionControl = errors.New("运动控制失败")

// ErrGateConditionFailed 门控条件等待失败（超时）的哨兵错误。
// 门控（球罐判定 / 风洞总压范围判定）超时代表设备或流场条件未就绪，
// 后续测点大概率同样无法满足，继续执行没有意义。runCalibrationLoop 将其视为
// 不可恢复错误，无条件停止校准并返回错误——不受 StopOnError 影响
// （StopOnError 仅决定"普通测点采集失败"是否继续）。否则多点校准在
// StopOnError=false（前端默认）下会跳到下一轮、因 Stop 已置位而以 nil 返回，
// 被 Manager 误判为 StateCompleted。
var ErrGateConditionFailed = errors.New("门控条件等待失败")

// AutomaticCalibration 自动循环校准引擎
// 提供 move → wait → gate → acquire → hook → push → next 的模板方法
// 五孔、三孔、总压校准使用此引擎
type AutomaticCalibration struct {
	mu             sync.RWMutex
	config         Config
	eventPublisher EventPublisher
	runtime        RuntimeAccess
	onDataPoint    func(DataPoint) // 每个点采集完成后的回调（用于实时 CSV 写入等）
	// onMotionSafetyFailure 运动安全故障回调（由 CalibrationManager 注入）。
	// 引擎层检测到运动安全故障时调用，委托 Manager 执行急停 + 状态写入。
	// core/ 不能导入 usecase/（六边形架构硬约束），通过回调反转依赖。
	// 可为 nil（未注入时故障仅返回 ErrMotionControl，不触发急停和状态快照写入）。
	onMotionSafetyFailure func(*traversal.MotionSafetyFailure) error
	taskID                string
	isRunning             bool
	isPaused              bool
	currentPointIdx       int
	dataPoints            []DataPoint
	startTime             int64
	// 当前点采样进度（由算法采集循环通过 onSampleProgress 回调更新）
	// currentSample：当前点已采样本数（1..samplesPerPoint），0 表示尚未开始
	// samplesPerPoint：当前点总采样数，用于 UI 显示"采样 3/10"
	currentSample   int
	samplesPerPoint int
	// prevRegion/prevSector 七孔流场分区判定的滞回状态（spec Task 10 + §3.2 规则 3）。
	// 由本引擎在每点采集后从 SevenHoleDataPoint.Region/Sector 更新，
	// 下一点采集前注入 config.PrevRegion/PrevSector 传给 DetermineRegion。
	// 首点前为零值（PrevRegion=""，PrevSector=0），符合 spec "首点跳过滞回"语义。
	// 仅 TypeSevenHole 使用，其他类型忽略。
	// 访问模型：runCalibrationLoop → processPoint 单线程串行访问，无需加锁。
	prevRegion string
	prevSector int
}

// NewAutomaticCalibration 创建自动校准引擎
// onDataPoint 可为 nil；非 nil 时每个点采集完成后同步调用，调用方负责持久化等 I/O。
// onMotionSafetyFailure 可为 nil；非 nil 时运动安全故障由回调处理（急停 + 状态写入）。
func NewAutomaticCalibration(
	config Config,
	publisher EventPublisher,
	runtime RuntimeAccess,
	onDataPoint func(DataPoint),
	onMotionSafetyFailure func(*traversal.MotionSafetyFailure) error,
) *AutomaticCalibration {
	return &AutomaticCalibration{
		config:                config,
		eventPublisher:        publisher,
		runtime:               runtime,
		onDataPoint:           onDataPoint,
		onMotionSafetyFailure: onMotionSafetyFailure,
		dataPoints:            make([]DataPoint, 0),
	}
}

// SetTaskID 设置任务ID
func (a *AutomaticCalibration) SetTaskID(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.taskID = id
}

// Start 启动自动校准循环（兼容入口）。
// 语义等同于 StartWithContext(context.Background(), algorithm)：不会被 ctx 取消，
// 仅响应 Pause/Resume/Stop 控制。
func (a *AutomaticCalibration) Start(algorithm Algorithm) error {
	return a.StartWithContext(context.Background(), algorithm)
}

// StartWithContext 启动自动校准循环，ctx 取消时中断引擎自有的等待路径
// （驻留 dwell、暂停恢复等待、球罐闸门等待、算法样本间的新数据等待）。
//
// 返回语义：
//   - ctx 取消（含启动前已取消）：返回 ctx.Err()（errors.Is(err, context.Canceled) 为 true），
//     不发送完成事件，已采集的数据点保留可查询；
//   - 其余路径（正常完成、测点失败、运动失败、Stop 停止）语义与既有 Start 一致，
//     算法结果语义不变。
func (a *AutomaticCalibration) StartWithContext(ctx context.Context, algorithm Algorithm) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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
	// 注入设备采集态查询：算法在 waitForFreshData 超时后调用，
	// 区分"用户停采集"（可恢复，继续等待）与"设备在采集但帧不更新"（真异常，返回超时错误）。
	// runtime 为 nil 时返回 nil，算法侧回退到原超时失败行为（向后兼容）。
	a.config.AcquisitionStateProvider = a.makeAcquisitionStateProvider()
	a.mu.Unlock()

	log.Printf("[AutomaticCalibration] 启动校准，共 %d 个测点", len(a.config.Points))

	err := a.runCalibrationLoop(ctx, algorithm)

	a.mu.Lock()
	a.isRunning = false
	a.mu.Unlock()
	return err
}

// runCalibrationLoop 校准主循环（模板方法）
func (a *AutomaticCalibration) runCalibrationLoop(ctx context.Context, algorithm Algorithm) error {
	var pointErrorCount int
	var lastPointError string

	for i := a.GetCurrentPointIndex(); i < len(a.config.Points); i++ {
		// ctx 取消优先于其他状态判定：取消语义必须与正常停止/完成区分
		if err := ctx.Err(); err != nil {
			return err
		}
		if !a.IsRunning() {
			log.Printf("[AutomaticCalibration] 校准被用户停止")
			return nil
		}

		// 等待暂停恢复（ctx 取消时立即返回）
		if err := a.waitWhilePaused(ctx); err != nil {
			return err
		}

		if !a.IsRunning() {
			return nil
		}

		a.mu.Lock()
		a.currentPointIdx = i
		a.mu.Unlock()
		point := a.config.Points[i]

		log.Printf("[AutomaticCalibration] 处理测点 %d/%d, 坐标: %v", i+1, len(a.config.Points), point.Coordinates)

		if err := a.processPoint(ctx, algorithm, point, i); err != nil {
			// ctx 取消优先：取消可能以 ErrPointAborted 或包装错误的形式从等待路径返回
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
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
			// 门控条件超时：不可恢复，无条件停止并返回错误（不受 StopOnError 影响）。
			// 门控超时前引擎已自行 Stop()，此分支确保把错误带出循环，
			// 避免 StopOnError=false 时跳到下一轮、以 !IsRunning()→nil 伪装成成功完成。
			if errors.Is(err, ErrGateConditionFailed) {
				return fmt.Errorf("测点 %d 门控条件失败: %w", i+1, err)
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
func (a *AutomaticCalibration) processPoint(ctx context.Context, algorithm Algorithm, point CalPoint, index int) error {
	// 2. 移动到点位
	if err := a.moveToPoint(point, algorithm); err != nil {
		return fmt.Errorf("%w: 移动到测点失败: %w", ErrMotionControl, err)
	}

	// 移动完成后检查暂停——运动可能在过程中被 Pause 打断，
	// StopMotion 会使 WaitForMotionComplete 提前返回，此处捕获暂停态。
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 2. 等待驻留时间（让压力稳定）；ctx 取消时立即退出
	if a.config.DwellTimeMs > 0 {
		if err := sleepContext(ctx, time.Duration(a.config.DwellTimeMs)*time.Millisecond); err != nil {
			return err
		}
	}

	// 驻留后检查暂停
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 3. 等待球罐闸门条件（如果启用）
	if err := a.waitForSphereTankGateIfNeeded(ctx); err != nil {
		return fmt.Errorf("球罐闸门等待失败: %w", err)
	}

	// 闸门条件满足后、采集前检查暂停
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 3.5 等待风洞总压范围条件（五孔探针校准专用，如果启用）
	if err := a.waitForTotalPressureGateIfNeeded(ctx); err != nil {
		return fmt.Errorf("风洞总压范围判定失败: %w", err)
	}

	// 总压范围条件满足后、采集前检查暂停
	if err := a.checkPausedAndAbort(); err != nil {
		return err
	}

	// 4. 采集数据
	channelReader := a.makeChannelReader()
	checkAbort := func() bool {
		return ctx.Err() != nil || !a.IsRunning() || a.IsPaused()
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
	// 七孔专用：每点采集前注入 RealtimeCallback + PrevRegion/PrevSector（spec Task 10）。
	// RealtimeCallback：将七孔算法的实时数据包装为 RealtimeEvent 后调 eventPublisher.OnRealtime，
	// core 层不依赖具体发布器实现（六边形架构：core 仅依赖 EventPublisher 接口）。
	// PrevRegion/PrevSector：取自引擎自身持有的滞回状态（首点为零值，跳过滞回判定）。
	// 通过 config 注入而非算法参数：Algorithm.AcquireDataWithConfig 签名固定（spec Task 5 约束）。
	//
	// 并发安全（code-review C2 修复）：
	//   - prevRegion/prevSector 与 GetCurrentRegion/GetCurrentSector 共用 a.mu 保护
	//   - 这里在 RLock 内读取快照后注入 config，避免与 Status() 路径并发读取产生数据竞争
	if algorithm.Type() == TypeSevenHole {
		a.config.RealtimeCallback = a.makeSevenHoleRealtimeCallback(point)
		a.mu.RLock()
		a.config.PrevRegion = a.prevRegion
		a.config.PrevSector = a.prevSector
		a.mu.RUnlock()
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

	// 七孔专用：从数据点取 Region/Sector，先推送分区变更事件，再更新引擎滞回状态（spec Task 10 + Task 11）。
	// 顺序敏感：必须在更新前推送，使 RegionChangedEvent.PrevRegion/PrevSector 反映上一时刻值。
	// 类型断言失败不阻塞流程（仅记日志，下一点按首点语义处理）。
	//
	// 并发安全（code-review C2 修复）：
	//   - prevRegion/prevSector 通过 a.mu 保护——与 GetCurrentRegion/GetCurrentSector 共用同一锁
	//   - 锁内读取 prev 值并写入新值（原子），锁外调用 sendRegionChangedEvent（避免持锁调用外部 I/O）
	//   - 事件参数携带 prev 值，避免 sendRegionChangedEvent 内部再次无锁读取（I3 修复）
	if algorithm.Type() == TypeSevenHole {
		newRegion, newSector, ok := extractSevenHoleRegionSector(dataPoint)
		if ok {
			a.mu.Lock()
			prevRegion := a.prevRegion
			prevSector := a.prevSector
			a.prevRegion = newRegion
			a.prevSector = newSector
			a.mu.Unlock()
			a.sendRegionChangedEvent(newRegion, newSector, prevRegion, prevSector, index)
		} else {
			log.Printf("[AutomaticCalibration] 七孔数据点类型断言失败: %T，跳过滞回状态更新", dataPoint)
		}
	}

	// 6. 实时持久化回调（逐点写 CSV 等），失败仅记录不中断校准
	if a.onDataPoint != nil {
		a.onDataPoint(dataPoint)
	}

	// 7. 发送进度更新
	a.sendProgressUpdate(point, dataPoint)

	return nil
}

// makeSevenHoleRealtimeCallback 构造七孔实时数据推送回调。
//
// 将 SevenHoleAlgorithm 的实时回调签名
// (raw, coeffs, region, sector) 包装为 RealtimeEvent 后调
// eventPublisher.OnRealtime，core 层不依赖具体发布器实现。
//
// eventPublisher 为 nil 时返回 nil（算法侧会跳过推送，spec Task 5 已约定）。
// point 在闭包内捕获，用于填充 RealtimeEvent.Point（前端展示当前目标点位）。
func (a *AutomaticCalibration) makeSevenHoleRealtimeCallback(point CalPoint) SevenHoleRealtimeCallback {
	if a.eventPublisher == nil {
		return nil
	}
	publisher := a.eventPublisher
	// 闭包内不持有 a 的引用（避免循环引用 + 显式只读 publisher），
	// point 为值传递的 CalPoint 副本，避免外部修改影响。
	pointCopy := point
	return func(raw SevenHoleRawData, coeffs SevenHoleCoefficients, region string, sector int) {
		// rawData/coeffs 取地址构造指针字段，符合 RealtimeEvent 七孔字段语义
		// （指针 + omitempty，非七孔类型序列化时不出现 sevenHoleRaw key）。
		rawCopy := raw
		coeffsCopy := coeffs
		evt := RealtimeEvent{
			Type:                  TypeSevenHole,
			FiveHoleRaw:           nil,
			FiveHoleCoefficients:  nil,
			ThreeHoleRaw:          nil,
			ThreeHoleCoefficients: nil,
			SevenHoleRaw:          &rawCopy,
			SevenHoleCoefficients: &coeffsCopy,
			Point:                 &pointCopy,
		}
		// region/sector 通过 Point 字段已携带（CalPoint.Region/Sector），
		// RealtimeEvent 自身不重复暴露——避免字段语义重叠。
		publisher.OnRealtime(evt)
	}
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

// resolveTargetPosition 按坐标模式把测点坐标换算为运动目标绝对位置。
//
//   - 绝对坐标（默认）：测点坐标即目标绝对位置，原样返回。
//   - 相对坐标：目标 = 当前坐标 + 测点坐标（把测点坐标视为相对当前位置的位移量）。
//     运行时必须实现 PositionReader 才能读取当前位置；否则返回明确错误，
//     避免相对坐标被静默降级为绝对坐标。
func (a *AutomaticCalibration) resolveTargetPosition(axis MotionAxisConfig, value float64) (float64, error) {
	if a.config.CoordinateMode != CoordinateModeRelative {
		return value, nil
	}
	pr, ok := a.runtime.(PositionReader)
	if !ok {
		return 0, fmt.Errorf("相对坐标模式需要运行时支持当前位置读取（PositionReader）")
	}
	cur, err := pr.GetAxisPosition(axis)
	if err != nil {
		return 0, fmt.Errorf("读取 %s 轴当前位置失败: %w", axis.Name, err)
	}
	return cur + value, nil
}

// moveToPoint 移动到指定点位
// 默认实现按坐标顺序移动各轴；五孔/七孔探针走 MoveToPointWithOrder 严格 α→β 顺序
func (a *AutomaticCalibration) moveToPoint(point CalPoint, algorithm Algorithm) error {
	if algorithm != nil && algorithm.Type() == TypeFiveHole {
		return a.MoveToPointWithOrder(point, []string{"α", "β"})
	}
	if algorithm != nil && algorithm.Type() == TypeSevenHole {
		// 七孔按 α→β 顺序下发运动控制器（spec §3.4 + Task 10）。
		// 双坐标模型：MoveToPointWithOrder 优先读 point.MotionCoordinates，
		// 外区点由 (θ,φ) 换算得到的 (α',β')；内区点 MotionCoordinates==Coordinates。
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

		// 按坐标模式换算目标（相对坐标：目标 = 当前坐标 + 测点坐标）
		target, err := a.resolveTargetPosition(*axisConfig, position)
		if err != nil {
			return err
		}

		log.Printf("[AutomaticCalibration] 移动 %s 轴到 %v", axisName, target)
		if err := a.runtime.MoveToPosition(*axisConfig, target); err != nil {
			return fmt.Errorf("移动 %s 轴到 %v 失败: %w", axisName, target, err)
		}
	}

	// 等待所有轴运动完成（三元组返回值）
	return a.waitMotionCompleteOrAbort()
}

// waitMotionCompleteOrAbort 等待运动完成并按返回值分支处理。
//
// 分支策略：
//   - failure != nil：委托 onMotionSafetyFailure 回调处理急停 + 状态写入，返回 ErrMotionControl 终止校准
//   - completed=true：正常到位，返回 nil
//   - reason=Paused：返回 ErrPointAborted，runCalibrationLoop 回退索引重跑该点
//   - reason=Stopped/Cancelled：返回 ErrMotionControl 终止校准（不重跑）
//   - reason=Timeout：返回包装 ErrMotionControl 的超时错误
func (a *AutomaticCalibration) waitMotionCompleteOrAbort() error {
	completed, reason, failure := a.runtime.WaitForMotionComplete()

	// 1. 运动安全故障 → 委托 Manager 处理急停 + 状态写入
	if failure != nil {
		if a.onMotionSafetyFailure != nil {
			return a.onMotionSafetyFailure(failure)
		}
		return fmt.Errorf("%w: 运动安全故障: verdict=%s axis=%s target=%.3f actual=%.3f",
			ErrMotionControl, failure.Verdict, failure.Axis, failure.Target, failure.Actual)
	}

	// 2. 正常到位
	if completed {
		return nil
	}

	// 3. 按 reason 分支（非故障中断）
	switch reason {
	case traversal.MotionInterruptPaused:
		return ErrPointAborted // 暂停，回退索引重跑
	case traversal.MotionInterruptStopped, traversal.MotionInterruptCancelled:
		return fmt.Errorf("%w: 用户停止/取消", ErrMotionControl)
	case traversal.MotionInterruptTimeout:
		return fmt.Errorf("%w: 运动超时未完成（>120s）", ErrMotionControl)
	default:
		return nil
	}
}

// MoveToPointWithOrder 按指定轴顺序移动到点位（五孔/七孔共用：先α后β）
//
// 双坐标优先级（spec §3.4 + Task 10）：
//   - 优先读 point.MotionCoordinates[axisName]——七孔外区点由 (θ,φ) 换算得到的 (α',β')
//   - MotionCoordinates 为 nil 或不含该轴时回退到 point.Coordinates[axisName]
//     （五孔/三孔/总压/总温等不填 MotionCoordinates 的场景保持原行为）
func (a *AutomaticCalibration) MoveToPointWithOrder(point CalPoint, axisOrder []string) error {
	if a.runtime == nil || len(a.config.MotionAxes) == 0 {
		return nil
	}

	for _, axisName := range axisOrder {
		position, ok := lookupAxisPosition(point, axisName)
		if !ok {
			return fmt.Errorf("测点缺少 %s 坐标", axisName)
		}

		axisConfig := a.findAxisConfig(axisName)
		if axisConfig == nil {
			return fmt.Errorf("未找到轴 %s 的运动配置", axisName)
		}

		// 按坐标模式换算目标（相对坐标：目标 = 当前坐标 + 测点坐标）。
		// 七孔外区点的运动坐标 (α',β') 同样参与换算——相对位移语义对运动坐标一致适用。
		target, err := a.resolveTargetPosition(*axisConfig, position)
		if err != nil {
			return err
		}

		log.Printf("[AutomaticCalibration] 移动 %s 轴到 %v", axisName, target)
		if err := a.runtime.MoveToPosition(*axisConfig, target); err != nil {
			return fmt.Errorf("移动 %s 轴到 %v 失败: %w", axisName, target, err)
		}
	}

	// 等待所有轴运动完成（三元组返回值，五孔/七孔分轴顺序下每次独立判定）
	return a.waitMotionCompleteOrAbort()
}

// lookupAxisPosition 按双坐标优先级查找轴位置（spec §3.4 + Task 10）：
//  1. point.MotionCoordinates 非 nil 且包含 axisName → 用 MotionCoordinates 值
//     （七孔外区点的运动坐标 (α',β')，由 GenerateSevenHolePoints 按 §3.3 正向公式换算）
//  2. 否则回退到 point.Coordinates[axisName]（五孔/三孔/总压/总温及七孔内区点路径）
//  3. 两处都没有 → ok=false（调用方按"测点缺少坐标"报错）
func lookupAxisPosition(point CalPoint, axisName string) (float64, bool) {
	if point.MotionCoordinates != nil {
		if v, ok := point.MotionCoordinates[axisName]; ok {
			return v, true
		}
	}
	if v, ok := point.Coordinates[axisName]; ok {
		return v, true
	}
	return 0, false
}

// waitForSphereTankGateIfNeeded 等待球罐闸门条件
//
// 总超时取 gate.TimeoutSec，<=0 时使用默认 300 秒（5 分钟）。
// 超时后停止校准并返回错误，避免无限等待卡死整个流程。
// ctx 取消时立即返回 ctx.Err()。
func (a *AutomaticCalibration) waitForSphereTankGateIfNeeded(ctx context.Context) error {
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
		if err := ctx.Err(); err != nil {
			return err
		}
		// 暂停时等待
		if err := a.waitWhilePaused(ctx); err != nil {
			return err
		}
		if !a.IsRunning() {
			return nil
		}

		// 读取稳定时间通道
		stableRaw, ok := a.runtime.GetChannelValue(gate.StableTimeChannel.DeviceID, gate.StableTimeChannel.ChannelIndex)
		stableTimeSec, err := ParseSphereTankStableTimeSec(stableRaw, ok)
		if err != nil {
			// 读取失败，继续等待
			if err := sleepContext(ctx, 100*time.Millisecond); err != nil {
				return err
			}
			continue
		}

		if IsSphereTankGateSatisfied(gate, stableTimeSec) {
			return nil
		}

		if time.Now().UnixMilli()-gateWaitStartAt > int64(maxWaitMs) {
			a.Stop()
			return fmt.Errorf("%w: 球罐判定等待超时（%d 秒）", ErrGateConditionFailed, maxWaitSec)
		}

		if err := sleepContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}

	return nil
}

// waitForTotalPressureGateIfNeeded 等待风洞总压进入配置范围（五孔探针校准专用）。
//
// 仅在 TypeFiveHole 且配置了启用的 TunnelTotalPressureGate 时生效：
// 每个测点采集前读取 fiveHole.pTotal 通道当前值，若在 [min, max] 范围内则立即返回
// （开始采集），否则轮询等待直至进入范围。
//
// 总超时取 gate.TimeoutSec，<=0 时使用默认 300 秒（与球罐闸门一致）。
// 超时后停止校准并返回错误，避免无限等待卡死整个流程；ctx 取消时立即返回 ctx.Err()。
func (a *AutomaticCalibration) waitForTotalPressureGateIfNeeded(ctx context.Context) error {
	if a.config.Type != string(TypeFiveHole) {
		return nil
	}
	gate := NormalizeTunnelTotalPressureGate(a.config)
	if gate == nil {
		return nil
	}

	if err := ValidateTunnelTotalPressureGate(a.config); err != nil {
		return err
	}

	channel, ok := findFiveHoleTotalPressureChannel(a.config)
	if !ok {
		// 校验已确保通道存在，此处为防御性兜底
		return fmt.Errorf("未找到启用的 %s 通道", fiveHoleTotalPressureRole)
	}

	// 记录进入门控时 fiveHole.pTotal 设备的当前时间戳：要求后续读数来自本测点
	// 运动 + 驻留之后的采集周期（新帧），避免接受运动前缓存的旧总压值
	// （设备停止采集或数据陈旧时，时间戳不再前进，门控将持续等待直至超时）。
	entryTS, hasEntryTS := int64(0), false
	if a.runtime != nil {
		entryTS, hasEntryTS = a.runtime.GetLatestTimestamp(channel.DeviceID)
	}

	// 总压判定总超时（秒）。0 表示使用默认 300 秒。
	maxWaitSec := gate.TimeoutSec
	if maxWaitSec <= 0 {
		maxWaitSec = 300
	}
	maxWaitMs := maxWaitSec * 1000
	gateWaitStartAt := time.Now().UnixMilli()

	for a.IsRunning() {
		if err := ctx.Err(); err != nil {
			return err
		}
		// 暂停时等待
		if err := a.waitWhilePaused(ctx); err != nil {
			return err
		}
		if !a.IsRunning() {
			return nil
		}

		// 读取风洞总压通道当前值（runtime 为 nil 时视为通道不可读，继续等待）
		var value float64
		var ok bool
		if a.runtime != nil {
			value, ok = a.runtime.GetChannelValue(channel.DeviceID, channel.ChannelIndex)
		}
		if ok {
			// 新鲜度判定：设备无时间戳能力（fake/旧驱动，hasEntryTS=false）时退化为仅按值判定；
			// 有时间戳时要求当前帧晚于进入门控时刻（当前采集周期新数据），否则视为陈旧缓存。
			fresh := true
			if hasEntryTS {
				curTS, hasCurTS := a.runtime.GetLatestTimestamp(channel.DeviceID)
				fresh = hasCurTS && curTS > entryTS
			}
			if fresh && IsTotalPressureInRange(gate, value) {
				return nil
			}
		}

		if time.Now().UnixMilli()-gateWaitStartAt > int64(maxWaitMs) {
			a.Stop()
			return fmt.Errorf("%w: 风洞总压范围判定等待超时（%d 秒），当前总压未进入配置范围 [%.2f, %.2f]",
				ErrGateConditionFailed, maxWaitSec, gate.MinTotalPressure, gate.MaxTotalPressure)
		}

		if err := sleepContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}

	return nil
}

// waitWhilePaused 等待暂停恢复；ctx 取消时立即返回 ctx.Err()。
func (a *AutomaticCalibration) waitWhilePaused(ctx context.Context) error {
	for {
		a.mu.RLock()
		paused := a.isPaused
		running := a.isRunning
		a.mu.RUnlock()
		if !paused || !running {
			return nil
		}
		if err := sleepContext(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

// sleepContext 等待 d 到期或 ctx 取消；取消时返回 ctx.Err()。
// 替代引擎自有等待路径中的无条件 time.Sleep，使取消不被 sleep 阻断。
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
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

// makeAcquisitionStateProvider 创建设备采集态查询函数。
// 委托给 RuntimeAccess.IsAcquiring——core 层不依赖 ports，由 usecase 层的 runtimeAdapter
// 桥接到 ports.AcquisitionController.IsAcquiring。runtime 为 nil 时返回 nil（向后兼容）。
func (a *AutomaticCalibration) makeAcquisitionStateProvider() AcquisitionStateProvider {
	if a.runtime == nil {
		return nil
	}
	return func(deviceID string) bool {
		return a.runtime.IsAcquiring(deviceID)
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

// sendRegionChangedEvent 推送七孔流场分区变更事件（spec Task 11）。
//
// 推送规则：
//   - 首点（index==0）：必推送一次，PrevRegion=nil、PrevSector=nil（JSON 序列化为 null），
//     BoundaryFlag="first"
//   - 后续点：当 region 或 sector 与上一时刻（prevRegion/prevSector 参数）不同时推送：
//   - inner↔outer 切换 → BoundaryFlag="inner-outer"
//   - 同 outer 但扇区变化 → BoundaryFlag="sector-switch"
//   - region 与 sector 均不变时不推送（避免噪声）
//
// 调用时机：调用方在 a.mu 锁内读取并更新 prevRegion/prevSector 后调用本方法，
// 通过参数显式传入 prev 值——避免本函数内再次无锁读取 a.prevRegion/prevSector
// 产生数据竞争（code-review I3 修复）。
//
// taskID/totalPoints 读取与 sendProgressUpdate 一致：在 RLock 下读取后释放再调用 OnRegionChanged，
// 避免持有锁调用外部代码（OnRegionChanged 实现可能触发 SSE/Wails EventEmit 等 I/O）。
func (a *AutomaticCalibration) sendRegionChangedEvent(region string, sector int, prevRegion string, prevSector int, index int) {
	if a.eventPublisher == nil {
		return
	}

	a.mu.RLock()
	taskID := a.taskID
	totalPoints := len(a.config.Points)
	a.mu.RUnlock()

	publisher := a.eventPublisher

	// 首点：PrevRegion/PrevSector 必为 nil（JSON null），BoundaryFlag="first"
	if index == 0 {
		publisher.OnRegionChanged(RegionChangedEvent{
			TaskID:       taskID,
			WindowTag:    "calibration",
			Region:       region,
			Sector:       sector,
			PrevRegion:   nil,
			PrevSector:   nil,
			BoundaryFlag: "first",
			PointIndex:   index,
			TotalPoints:  totalPoints,
			Timestamp:    time.Now().UnixMilli(),
		})
		return
	}

	// 后续点：region 与 sector 均不变时跳过推送
	if region == prevRegion && sector == prevSector {
		return
	}

	// 区域切换：inner↔outer；扇区切换：同 outer 但扇区编号变化
	boundaryFlag := "sector-switch"
	if prevRegion != region {
		boundaryFlag = "inner-outer"
	}
	publisher.OnRegionChanged(RegionChangedEvent{
		TaskID:       taskID,
		WindowTag:    "calibration",
		Region:       region,
		Sector:       sector,
		PrevRegion:   &prevRegion,
		PrevSector:   &prevSector,
		BoundaryFlag: boundaryFlag,
		PointIndex:   index,
		TotalPoints:  totalPoints,
		Timestamp:    time.Now().UnixMilli(),
	})
}

// extractSevenHoleRegionSector 从 DataPoint 提取七孔的 Region/Sector 字段。
// 类型断言失败返回 ok=false，调用方据此决定是否跳过滞回状态更新与分区事件推送。
func extractSevenHoleRegionSector(dp DataPoint) (region string, sector int, ok bool) {
	sh, okT := dp.(*SevenHoleDataPoint)
	if !okT {
		return "", 0, false
	}
	return sh.Region, sh.Sector, true
}

// GetCurrentRegion 返回当前七孔流场分区（spec Task 11）。
// 语义：返回最近一次采集完成后的 Region；校准启动后首点采集完成前为 ""（前端 omitempty 忽略）。
// 五孔/三孔/总压/总温类型返回零值。
func (a *AutomaticCalibration) GetCurrentRegion() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prevRegion
}

// GetCurrentSector 返回当前七孔流场扇区（spec Task 11）。
// 语义：返回最近一次采集完成后的 Sector；首点采集完成前为 0（前端 omitempty 忽略）。
func (a *AutomaticCalibration) GetCurrentSector() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prevSector
}
