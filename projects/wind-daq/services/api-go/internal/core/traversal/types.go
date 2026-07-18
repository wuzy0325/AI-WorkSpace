package traversal

// State 遍历测试状态
type State string

const (
	StateIdle        State = "idle"
	StatePreparing   State = "preparing"
	StateMoving      State = "moving"
	StateStabilizing State = "stabilizing"
	StateAcquiring   State = "acquiring"
	StateSaving      State = "saving"
	StateRunning     State = "running" // 兼容旧状态
	StatePaused      State = "paused"
	StateStopped     State = "stopped"
	StateError       State = "error"
	StateCompleted   State = "completed"
)

// IsTerminal 判断是否为终态
func (s State) IsTerminal() bool {
	return s == StateCompleted || s == StateStopped || s == StateError
}

// PointPhase 当前测试点的处理阶段
type PointPhase string

const (
	PhaseMoving      PointPhase = "moving"
	PhaseStabilizing PointPhase = "stabilizing"
	PhaseAcquiring   PointPhase = "acquiring"
	PhaseSaving      PointPhase = "saving"
)

// Point 遍历测试点坐标（4 轴：X/Y/Z/U，对应位移机构全轴能力）
// U 字段零值为 0，旧配置文件无 u 字段时 Go JSON 反序列化自动填 0，向后兼容
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	U float64 `json:"u"`
}

// SaveOptions CSV 落盘选项（与前端 TraversalSaveOptions 对齐）
type SaveOptions struct {
	SavePointId          bool            `json:"savePointId"`
	SaveTimestamp        bool            `json:"saveTimestamp"`
	SaveRawPressure      bool            `json:"saveRawPressure"`
	SaveCalculatedResult bool            `json:"saveCalculatedResult"`
	CustomFields         map[string]bool `json:"customFields,omitempty"`
}

// Config 遍历测试配置
type Config struct {
	TaskID          string       `json:"taskId"`
	DeviceID        string       `json:"deviceId"`
	LayoutPattern   string       `json:"layoutPattern,omitempty"`
	Channels        []int        `json:"channels"`
	Path            []Point      `json:"path"`
	DwellTimeMs     int          `json:"dwellTimeMs"`
	SamplesPerPoint int          `json:"samplesPerPoint"`
	SavePath        string       `json:"savePath,omitempty"`     // CSV 输出文件路径（同时用于断点文件命名）
	SaveFileName    string       `json:"saveFileName,omitempty"` // CSV 输出文件名前缀
	SaveOptions     *SaveOptions `json:"saveOptions,omitempty"`  // 落盘选项
	// ChannelLabels 通道索引→标签的显式映射（如 {0:"P1", 16:"Patm", 17:"Tatm"}）
	// 由前端 ProbeChannelConfig.role 推导而来；为空时回退到"按通道索引升序"的旧行为
	ChannelLabels map[int]string `json:"channelLabels,omitempty"`
	// ChannelRefs 内部通道键→物理通道（设备+硬件通道索引）的映射。
	//
	// 背景：遍历支持跨设备绑定通道（如五孔压力在设备 A、大气压力/温度在设备 B），
	// 且不同设备的硬件通道序号允许重复（各设备均从 0 开始编号）。
	// Channels/ChannelLabels/PointResult.Values 使用的 int 键是"内部键"，
	// 单设备或无冲突时等于硬件通道索引；跨设备序号冲突时为冲突通道分配空闲整数。
	// 采样/归一化等需要访问硬件的路径必须经 ChannelRefs 还原真实设备与通道。
	//
	// 为空（旧配置/旧断点）时由 ResolvedChannelRefs 回退合成：
	// 内部键=硬件通道索引、设备=DeviceID，行为与历史单设备完全一致。
	ChannelRefs map[int]ChannelRef `json:"channelRefs,omitempty"`
	// InterpolationMode 多 PRB 插值模式："normal" / "linear" / "nearest"
	// 仅对 MultiPrbInterpolator 生效；为空时使用插值器自身默认（normal）
	InterpolationMode string `json:"interpolationMode,omitempty"`
	// PProbePressureType 五孔探针 P1-P5 通道的压力传感器类型："gauge"（表压，默认）/ "absolute"（绝压）。
	// 约束：P1-P5 必须为同一类型（5 孔传感器物理上同型号），由全局开关统一表达。
	// Patm 通道始终视为绝压，不消费此字段。
	// 空串在 ParseAndStartTraversal 中兜底为 "gauge"，保证旧配置反序列化兼容。
	PProbePressureType string `json:"pProbePressureType,omitempty"`

	// MotionAxes 参与遍历运动的轴绑定列表，来自前端 motionAxes 配置。
	// 仅对这些「控制器+轴」发送 MoveTo 并等待到位，避免：
	// 1) 对未配置/未接硬件的轴（如 Z/U）强制归零；
	// 2) 对未绑定的真实控制器（即使 autoConnect 已连接）发指令并卡死等待。
	// 为空时（旧配置兼容）保持原行为：对所有已连接控制器的所有轴生成目标。
	MotionAxes []MotionAxisBinding `json:"motionAxes,omitempty"`

	// MotionSafety 运动安全配置：到位容差、严重偏离阈值、跨样本看门狗等。
	// 为 nil 时下游使用 DefaultMotionSafety，保证旧配置反序列化兼容。
	MotionSafety *MotionSafetyConfig `json:"motionSafety,omitempty"`
}

// ChannelRef 物理通道引用：内部通道键对应的真实设备与硬件通道索引。
// Index 为该设备 profile 内的硬件通道序号（各设备独立编号，允许跨设备重复）。
type ChannelRef struct {
	DeviceID string `json:"deviceId"`
	Index    int    `json:"index"`
}

// ResolvedChannelRefs 返回内部通道键→物理通道的有效映射。
//
// Config.ChannelRefs 非空时直接返回；为空（旧配置/旧断点/手工构造）时
// 按历史单设备语义合成：每个 Channels 条目的内部键即硬件通道索引、设备为 DeviceID。
// 调用方不应假定返回 map 可写——需要修改时请自行拷贝。
func (c Config) ResolvedChannelRefs() map[int]ChannelRef {
	if len(c.ChannelRefs) > 0 {
		return c.ChannelRefs
	}
	refs := make(map[int]ChannelRef, len(c.Channels))
	for _, ch := range c.Channels {
		refs[ch] = ChannelRef{DeviceID: c.DeviceID, Index: ch}
	}
	return refs
}

// MotionAxisBinding 遍历运动轴绑定：指定逻辑目标由哪台控制器的哪个物理轴执行。
// ControllerID 为空时表示不限制控制器（仅按轴名过滤，兼容旧数据）。
type MotionAxisBinding struct {
	Name         string `json:"name,omitempty"`
	ControllerID string `json:"controllerId,omitempty"`
	Axis         string `json:"axis"`
}

// MotionSafetyConfig 运动安全配置
//
// 设计目标：在遍历运动阶段对"目标位置 vs 实际位置"偏差、运动卡死、
// 越过目标、撞限位等异常进行快速失败，避免空跑 120s 兜底超时或撞机。
//
// 字段全部使用指针，零值表示"未配置"——下游通过 Resolve() 合并默认值与
// 按轴覆盖后得到有效配置。这样旧配置 JSON 反序列化时不会因缺字段报错，
// 新配置也只需覆盖关心项。
//
// 单位约定：
//   - 线性轴（X/Y/Z）：mm
//   - 旋转轴（U）：度（°）
//   - 通过 AxisOverrides 按轴覆盖可对不同轴使用不同阈值
type MotionSafetyConfig struct {
	// ArrivalTolerance 到位容差。控制器报告轴停止后，|实际位置-目标| ≤ 此值才视为到位。
	// 超过此值但 < CriticalDeviationLimit 视为"超差"（普通停止 + 终止遍历）。
	// 用户必须根据设备实际精度设置：B140+编码器约 0.005mm，无编码器/WTNMC4A 约 0.05-0.1mm。
	ArrivalTolerance *float64 `json:"arrivalTolerance,omitempty"`

	// CriticalDeviationLimit 严重偏离阈值。超过此值触发急停（EmergencyStop）。
	// 用于识别"轴已停但偏差离谱"的严重异常（如失步、编码器故障、机械松动）。
	CriticalDeviationLimit *float64 `json:"criticalDeviationLimit,omitempty"`

	// NoProgressTimeoutMs 运动中无有效进展的最长时间（毫秒）。
	// 轴报告 Moving=true 但位置长时间无变化时，判定为卡死并普通停止。
	// 必须显著小于 waitForMotionComplete 的 120s 兜底超时。
	NoProgressTimeoutMs *int `json:"noProgressTimeoutMs,omitempty"`

	// ProgressEpsilon 构成"有效进展"的最小位置变化量。
	// 相邻轮询间位置变化 ≥ 此值视为有进展，重置 NoProgressTimeoutMs 计时。
	// 设太小会被传感器噪声误判为有进展，设太大会漏判微动卡死。
	ProgressEpsilon *float64 `json:"progressEpsilon,omitempty"`

	// AxisOverrides 按轴覆盖阈值。键为轴名（与 MotionAxisBinding.Axis 一致）。
	// 覆盖项内不允许再嵌套 AxisOverrides（递归无意义且增加解析复杂度）。
	AxisOverrides map[string]*MotionSafetyConfig `json:"axisOverrides,omitempty"`
}

// 默认运动安全阈值
//
// 这些值是"安全的保守起点"，覆盖大多数设备类型。生产部署前必须通过 HIL
// 测试根据具体设备精度调整。参见 spec §Confirmed Decisions #8。
var (
	defaultArrivalTolerance       = 0.2   // 0.2mm，兼顾定位精度与机械抖动/背隙的容差
	defaultCriticalDeviationLimit = 5.0   // 5mm，明显异常才急停
	defaultNoProgressTimeoutMs    = 2000  // 2s 无进展判卡死
	defaultProgressEpsilon        = 0.001 // 0.001mm，规避编码器噪声
)

// DefaultMotionSafety 返回默认运动安全配置的值拷贝。
// 返回值类型而非指针，避免调用方意外修改包级默认值。
func DefaultMotionSafety() MotionSafetyConfig {
	return MotionSafetyConfig{
		ArrivalTolerance:       ptrFloat64(defaultArrivalTolerance),
		CriticalDeviationLimit: ptrFloat64(defaultCriticalDeviationLimit),
		NoProgressTimeoutMs:    ptrInt(defaultNoProgressTimeoutMs),
		ProgressEpsilon:        ptrFloat64(defaultProgressEpsilon),
	}
}

// ptrFloat64 返回 v 的指针副本，用于构造默认配置。
func ptrFloat64(v float64) *float64 { return &v }

// ptrInt 返回 v 的指针副本，用于构造默认配置。
func ptrInt(v int) *int { return &v }

// coalesceFloat64Ptr 返回 first 非 nil 时 first 的指针拷贝；
// first 为 nil 时返回 second 的指针拷贝；两者皆 nil 时返回 nil。
//
// 用于 MotionSafetyConfig.Resolve/Merge 统一"零值即默认"的字段合并语义：
//   - Resolve 调用 coalesceFloat64Ptr(c.X, resolved.X) 表达"c 优先，否则保留默认值"
//   - Merge  调用 coalesceFloat64Ptr(merged.X, other.X) 表达"merged 优先，否则用 other 填充"
//
// 始终返回新指针（深拷贝值），避免合并结果与源配置共享底层变量。
func coalesceFloat64Ptr(first, second *float64) *float64 {
	if first != nil {
		return ptrFloat64(*first)
	}
	if second != nil {
		return ptrFloat64(*second)
	}
	return nil
}

// coalesceIntPtr 同 coalesceFloat64Ptr，用于 *int 字段。
func coalesceIntPtr(first, second *int) *int {
	if first != nil {
		return ptrInt(*first)
	}
	if second != nil {
		return ptrInt(*second)
	}
	return nil
}

// Resolve 返回该配置在指定轴上的有效合并值。
// 合并优先级：默认值 < 全局配置 < 按轴覆盖。
// 任一指针字段在覆盖项中为 nil 时继承全局（或默认）值。
// axis 为空字符串时仅返回全局合并结果。
func (c *MotionSafetyConfig) Resolve(axis string) MotionSafetyConfig {
	// 1. 起点：默认值
	resolved := DefaultMotionSafety()

	// 2. 全局配置覆盖默认值（coalesce：c.X 优先，否则保留 resolved.X 默认值）
	if c != nil {
		resolved.ArrivalTolerance = coalesceFloat64Ptr(c.ArrivalTolerance, resolved.ArrivalTolerance)
		resolved.CriticalDeviationLimit = coalesceFloat64Ptr(c.CriticalDeviationLimit, resolved.CriticalDeviationLimit)
		resolved.NoProgressTimeoutMs = coalesceIntPtr(c.NoProgressTimeoutMs, resolved.NoProgressTimeoutMs)
		resolved.ProgressEpsilon = coalesceFloat64Ptr(c.ProgressEpsilon, resolved.ProgressEpsilon)
	}

	// 3. 按轴覆盖全局值（coalesce：override.X 优先，否则保留上一步的 resolved.X）
	if c != nil && axis != "" {
		if override, ok := c.AxisOverrides[axis]; ok && override != nil {
			resolved.ArrivalTolerance = coalesceFloat64Ptr(override.ArrivalTolerance, resolved.ArrivalTolerance)
			resolved.CriticalDeviationLimit = coalesceFloat64Ptr(override.CriticalDeviationLimit, resolved.CriticalDeviationLimit)
			resolved.NoProgressTimeoutMs = coalesceIntPtr(override.NoProgressTimeoutMs, resolved.NoProgressTimeoutMs)
			resolved.ProgressEpsilon = coalesceFloat64Ptr(override.ProgressEpsilon, resolved.ProgressEpsilon)
		}
	}

	// 轴覆盖合并后不再保留 AxisOverrides，避免下游误用
	resolved.AxisOverrides = nil
	return resolved
}

// Merge 将 other 合并到 c（c 优先，other 填充 c 的 nil 字段）。
// 用于将前端配置与默认值合并。返回新值，不修改 c。
func (c *MotionSafetyConfig) Merge(other MotionSafetyConfig) MotionSafetyConfig {
	merged := MotionSafetyConfig{}
	if c != nil {
		merged = *c
	}
	// coalesce：merged.X 优先，否则用 other.X 填充
	merged.ArrivalTolerance = coalesceFloat64Ptr(merged.ArrivalTolerance, other.ArrivalTolerance)
	merged.CriticalDeviationLimit = coalesceFloat64Ptr(merged.CriticalDeviationLimit, other.CriticalDeviationLimit)
	merged.NoProgressTimeoutMs = coalesceIntPtr(merged.NoProgressTimeoutMs, other.NoProgressTimeoutMs)
	merged.ProgressEpsilon = coalesceFloat64Ptr(merged.ProgressEpsilon, other.ProgressEpsilon)
	if merged.AxisOverrides == nil && other.AxisOverrides != nil {
		merged.AxisOverrides = make(map[string]*MotionSafetyConfig, len(other.AxisOverrides))
		for k, v := range other.AxisOverrides {
			if v != nil {
				copy := *v
				merged.AxisOverrides[k] = &copy
			}
		}
	}
	return merged
}

// GridConfig 网格配置（旧接口兼容）
type GridConfig struct {
	XStart float64 `json:"xStart"`
	XEnd   float64 `json:"xEnd"`
	XStep  float64 `json:"xStep"`
	YStart float64 `json:"yStart"`
	YEnd   float64 `json:"yEnd"`
	YStep  float64 `json:"yStep"`
	ZStart float64 `json:"zStart"`
}

// CalculatedResult 单点的实时插值结果摘要（用于 CSV 落盘列）
// 与 InterpolationResult 解耦：CSV 只需要标量字段
type CalculatedResult struct {
	Valid bool    `json:"valid"`
	Alpha float64 `json:"alpha"`
	Beta  float64 `json:"beta"`
	Pt    float64 `json:"pt"`
	Ps    float64 `json:"ps"`
	Mach  float64 `json:"mach"`
}

type PointStatus string

const (
	PointStatusCompleted PointStatus = "completed"
	PointStatusSkipped   PointStatus = "skipped"
	PointStatusFailed    PointStatus = "failed"
)

// IsCommitted 判断该点是否算"已确认提交"——commitPointV2 成功后即认为提交，
// 含正常完成（Completed）与跳过（Skipped）两类。
// Skipped 之所以算 Committed：跳过的点已通过 commitPointV2 持久化到结果日志与 checkpoint，
// 崩溃恢复时不应重新采点，否则会出现"同一物理位置采两次"的语义错误。
func (s PointStatus) IsCommitted() bool {
	return s == PointStatusCompleted || s == PointStatusSkipped
}

// PointResult 单点测试结果
type PointResult struct {
	TaskID             string            `json:"taskId,omitempty"`
	CommitSeq          uint64            `json:"commitSeq,omitempty"`
	PointIndex         int               `json:"pointIndex"`
	PointStatus        PointStatus       `json:"pointStatus,omitempty"`
	Point              Point             `json:"point"`
	Timestamp          int64             `json:"timestamp"`
	StartedAt          int64             `json:"startedAt,omitempty"`
	CompletedAt        int64             `json:"completedAt,omitempty"`
	Values             map[int]float64   `json:"values"`
	SampleCount        int               `json:"sampleCount"`
	DwellTimeElapsed   int               `json:"dwellTimeElapsed"`
	Calculated         *CalculatedResult `json:"calculated,omitempty"`
	ValidationWarnings []string          `json:"validationWarnings,omitempty"`
	CSVRowHash         string            `json:"csvRowHash,omitempty"`
	CustomValues       map[string]string `json:"customValues,omitempty"`
}

// ErrorCode 遍历测试错误码
type ErrorCode string

const (
	ErrMotionFailed        ErrorCode = "MOTION_FAILED"
	ErrAcquisitionFailed   ErrorCode = "ACQUISITION_FAILED"
	ErrSaveFailed          ErrorCode = "SAVE_FAILED"
	ErrInterpolationFailed ErrorCode = "INTERPOLATION_FAILED"
	ErrUnknown             ErrorCode = "UNKNOWN"

	// 运动安全错误码（v2 新增）
	//
	// 按严重级别分类：
	//   普通停止类（Stop）：ErrPositionDeviation / ErrMotionOvershoot / ErrMotionNoProgress / ErrMotionTimeout
	//   急停类（EmergencyStop）：ErrCriticalPositionDeviation / ErrLimitSwitchTriggered / ErrMotionStatusUnavailable
	//   系统类：ErrEmergencyStopFailed（急停调用失败，已 fallback Stop）
	ErrPositionDeviation         ErrorCode = "POSITION_DEVIATION"
	ErrMotionOvershoot           ErrorCode = "MOTION_OVERSHOOT"
	ErrMotionNoProgress          ErrorCode = "MOTION_NO_PROGRESS"
	ErrCriticalPositionDeviation ErrorCode = "CRITICAL_POSITION_DEVIATION"
	ErrLimitSwitchTriggered      ErrorCode = "LIMIT_SWITCH_TRIGGERED"
	ErrMotionStatusUnavailable   ErrorCode = "MOTION_STATUS_UNAVAILABLE"
	ErrEmergencyStopFailed       ErrorCode = "EMERGENCY_STOP_FAILED"
	ErrMotionTimeout             ErrorCode = "MOTION_TIMEOUT"
)

// ErrorCodeFor 返回 MotionSafetyVerdict 对应的错误码。
//
// 用于在故障处理阶段将 verdict 映射为对外的 ErrorCode。
// MotionSafetyOK 和 MotionSafetyArrived 不会作为故障返回，
// 调用此函数前应已通过 IsFailure() 过滤。
func ErrorCodeFor(v MotionSafetyVerdict) ErrorCode {
	switch v {
	case MotionSafetyDeviation:
		return ErrPositionDeviation
	case MotionSafetyOvershoot:
		return ErrMotionOvershoot
	case MotionSafetyNoProgress:
		return ErrMotionNoProgress
	case MotionSafetyCriticalDeviation:
		return ErrCriticalPositionDeviation
	case MotionSafetyLimitTriggered:
		return ErrLimitSwitchTriggered
	case MotionSafetyStatusUnavailable:
		return ErrMotionStatusUnavailable
	default:
		return ErrUnknown
	}
}

// MotionSafetyVerdict 运动安全判定结果
//
// 用于 EvaluateMotionSafety 纯函数和跨样本看门狗的返回值，
// 描述单次快照或跨样本观察到的运动状态分类。判定优先级：
//  1. 撞限位（LimitTriggered）— 立即急停
//  2. 运动中（OK）— 不判偏差，交给看门狗检测卡死/越过
//  3. 轴已停 → 检查偏差：到位 / 严重偏离 / 超差
//
// NoProgress 和 Overshoot 由跨样本看门狗判定，不来自单次快照。
type MotionSafetyVerdict string

const (
	// MotionSafetyOK 正常运动中，继续等待
	MotionSafetyOK MotionSafetyVerdict = "ok"
	// MotionSafetyArrived 已到位（轴停且偏差 ≤ ArrivalTolerance）
	MotionSafetyArrived MotionSafetyVerdict = "arrived"
	// MotionSafetyDeviation 超差：轴已停但偏差 > ArrivalTolerance 且 < CriticalDeviationLimit
	// 普通停止 + 终止遍历
	MotionSafetyDeviation MotionSafetyVerdict = "deviation"
	// MotionSafetyCriticalDeviation 严重偏离：轴已停且偏差 ≥ CriticalDeviationLimit
	// 急停 + 终止遍历
	MotionSafetyCriticalDeviation MotionSafetyVerdict = "critical_deviation"
	// MotionSafetyLimitTriggered 撞限位：PosLimit 或 NegLimit 触发
	// 急停 + 终止遍历
	MotionSafetyLimitTriggered MotionSafetyVerdict = "limit_triggered"
	// MotionSafetyNoProgress 运动中无进展：Moving=true 但位置长时间无变化
	// 普通停止 + 终止遍历
	MotionSafetyNoProgress MotionSafetyVerdict = "no_progress"
	// MotionSafetyOvershoot 越过目标：Moving=true 且位置已穿越目标位置
	// 普通停止 + 终止遍历
	MotionSafetyOvershoot MotionSafetyVerdict = "overshoot"
	// MotionSafetyStatusUnavailable 状态不可用：控制器掉线/已急停/目标轴连续缺失
	// 急停 + 终止遍历
	// 与 CriticalDeviation 区分：此类异常下 target/actual 可能无法读取（填 0），
	// 错误码映射到 ErrMotionStatusUnavailable 而非 ErrCriticalPositionDeviation，
	// 避免操作员误以为是位置偏离问题。
	MotionSafetyStatusUnavailable MotionSafetyVerdict = "status_unavailable"
)

// RequiresEmergencyStop 判定该 verdict 是否需要触发急停（而非普通停止）。
//
// 急停场景：严重偏离、撞限位、状态不可用——这些异常意味着设备可能失控，
// 必须瞬时停止避免撞机。普通超差/无进展/越过目标用减速停止即可。
func (v MotionSafetyVerdict) RequiresEmergencyStop() bool {
	switch v {
	case MotionSafetyCriticalDeviation,
		MotionSafetyLimitTriggered,
		MotionSafetyStatusUnavailable:
		return true
	default:
		return false
	}
}

// IsFailure 判定该 verdict 是否表示运动安全故障（非 OK / 非 Arrived）。
func (v MotionSafetyVerdict) IsFailure() bool {
	return v != MotionSafetyOK && v != MotionSafetyArrived
}

// MotionSafetyFailure 运动安全故障快照
//
// 判定失败时立即捕获事故现场，错误处理阶段不再读硬件状态——
// 因为从判定失败到调用 Stop/EmergencyStop 之间，硬件状态可能已经变化，
// 重新读取会丢失关键证据。
type MotionSafetyFailure struct {
	ControllerID   string              `json:"controllerId"`
	ControllerName string              `json:"controllerName,omitempty"`
	Axis           string              `json:"axis"`
	Verdict        MotionSafetyVerdict `json:"verdict"`
	Target         float64             `json:"target"`
	Actual         float64             `json:"actual"`
	PointIndex     int                 `json:"pointIndex"`
}

// MotionInterruptReason 描述 waitForMotionComplete 非故障中断的不可变原因。
//
// 设计动机：原遍历实现将暂停/停止/取消/超时都返回 (false, nil)，调用方事后读 isPaused
// 推断中断类型。若 Resume 操作恰好发生在"等待函数返回"与"调用方读 isPaused"之间，
// isPaused 已被清零，正常暂停会被误判为超时并终止任务。
// 改为返回不可变的原因枚举，调用方直接按原因分支处理，不再读共享状态。
//
// 导出位置：core/traversal 包，使 core/calibration 与 usecase 两个包可共享类型安全。
// 原未导出定义 motionInterruptReason 在 usecase/traversal_acquisition.go，
// 校准模块运动安全移植时提升为导出类型（spec-calibration-motion-safety.md AD-1）。
type MotionInterruptReason int

const (
	// MotionInterruptNone 未中断（completed=true）
	MotionInterruptNone MotionInterruptReason = iota
	// MotionInterruptPaused 用户暂停
	MotionInterruptPaused
	// MotionInterruptStopped 用户停止
	MotionInterruptStopped
	// MotionInterruptCancelled ctx 取消（外部 Stop）
	MotionInterruptCancelled
	// MotionInterruptTimeout 120s 兜底超时
	MotionInterruptTimeout
)

// Status 遍历测试状态
type Status struct {
	TaskID                  string        `json:"taskId"`
	State                   State         `json:"state"`
	CurrentPoint            int           `json:"currentPoint"`
	CommittedPoints         int           `json:"committedPoints"`
	CurrentPointIndex       int           `json:"currentPointIndex"`
	CurrentPointCoordinates *Point        `json:"currentPointCoordinates,omitempty"`
	CurrentPointPhase       PointPhase    `json:"currentPointPhase,omitempty"`
	TotalPoints             int           `json:"totalPoints"`
	Results                 []PointResult `json:"results"`
	LastError               string        `json:"lastError,omitempty"`
	LastErrorCode           ErrorCode     `json:"lastErrorCode,omitempty"`
	StartedAt               int64         `json:"startedAt,omitempty"`
	ValidationWarnings      []string      `json:"validationWarnings,omitempty"`
	// Warning 非致命运行警告（当前唯一来源：回零失败）。
	// 数据全部采完后回零失败不判测试失败（State 仍为 Completed），仅在此记录提示，
	// 前端据此向操作员展示"回零未完成"警告。
	Warning string `json:"warning,omitempty"`
	// CSVPath 实际落盘的 CSV 文件完整路径。
	// 由 Start/ResumeFromCheckpoint 在 openReliabilityPorts 之后写入：
	// csvPort.Open 在 Create 模式撞名时会自动追加 -2/-3 后缀（openCreateUnique），
	// 实际路径可能与 ResolveOutputPath(config) 计算的预期路径不同。
	// 前端侧边栏据此展示真实文件名，避免显示预期路径误导操作员。
	// 空串表示尚未启动或未注入 v2 csvPort（旧路径不写）。
	CSVPath string `json:"csvPath,omitempty"`
	// MotionSafetyFailure 运动安全故障现场快照。
	// 仅在 handleMotionSafetyFailure 路径写入，其他错误路径（采集失败/保存失败等）保持 nil。
	// 前端轮询拿到后用于在运行状态栏展示故障现场（控制器/轴/verdict/目标/实际/点号），
	// 避免 lastError 字符串正则解析的不稳定。
	MotionSafetyFailure *MotionSafetyFailure `json:"motionSafetyFailure,omitempty"`
}

const CheckpointVersion = 2

type TraversalRunSnapshot struct {
	Config               Config                `json:"config"`
	Validation           *DataValidationConfig `json:"validation,omitempty"`
	Stabilization        *StabilizationConfig  `json:"stabilization,omitempty"`
	InterpolatorIdentity string                `json:"interpolatorIdentity,omitempty"`
	SaveOptions          *SaveOptions          `json:"saveOptions,omitempty"`
	TotalPoints          int                   `json:"totalPoints"`
	CommittedPoints      int                   `json:"committedPoints"`
	CommitSeq            uint64                `json:"commitSeq"`
	CSVPath              string                `json:"csvPath"`
	ResultLogPath        string                `json:"resultLogPath"`
	CSVHeaderHash        string                `json:"csvHeaderHash,omitempty"`
	LastCommitHash       string                `json:"lastCommitHash,omitempty"`
}

// Checkpoint 断点恢复信息
//
// 配置单源真相：Config 只通过 Snapshot.Config 持有，不再保留独立的 []byte 副本。
// 早期版本同时写入 Config []byte 与 Snapshot.Config 两份，存在双份冗余且
// 增加同步维护成本；v2 统一从 Snapshot.Config 读取，节省存储空间并避免分歧。
type Checkpoint struct {
	Version         int                  `json:"version"`
	TaskID          string               `json:"taskId"`
	State           State                `json:"state"`
	Snapshot        TraversalRunSnapshot `json:"snapshot"`
	CompletedPoints int                  `json:"completedPoints"`
	TotalPoints     int                  `json:"totalPoints"`
	LastPoint       *Point               `json:"lastPoint,omitempty"`
	SavePath        string               `json:"savePath"`
	CreatedAt       int64                `json:"createdAt"`
}

// DataValidationConfig 数据验证配置
type DataValidationConfig struct {
	Enabled        bool                      `json:"enabled"`
	PressureRange  map[string]*PressureRange `json:"pressureRange,omitempty"`
	SpikeDetection *SpikeDetectionConfig     `json:"spikeDetection,omitempty"`
	OnInvalid      string                    `json:"onInvalid"` // skip | retry | continue
	RetryCount     int                       `json:"retryCount,omitempty"`
}

// PressureRange 压力范围限制
type PressureRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// SpikeDetectionConfig 尖峰检测配置
type SpikeDetectionConfig struct {
	Enabled   bool    `json:"enabled"`
	Threshold float64 `json:"threshold"` // 百分比阈值
}

// StabilizationConfig 稳定等待配置
type StabilizationConfig struct {
	Mode        string                 `json:"mode"` // fixed | adaptive
	FixedTimeMs int                    `json:"fixedTimeMs,omitempty"`
	Adaptive    *AdaptiveStabilization `json:"adaptive,omitempty"`
}

// AdaptiveStabilization 自适应稳定配置
type AdaptiveStabilization struct {
	MaxWaitMs          int     `json:"maxWaitMs"`
	MinWaitMs          int     `json:"minWaitMs"`
	StabilityThreshold float64 `json:"stabilityThreshold"` // 百分比变化阈值
	CheckIntervalMs    int     `json:"checkIntervalMs"`
	ConsecutiveChecks  int     `json:"consecutiveChecks"` // 连续稳定检查次数
}
