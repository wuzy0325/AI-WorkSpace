package calibration

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// ==================== 七孔探针流场分区判定（spec §3.2 tie-break 规则） ====================
//
// 分区判定是无状态纯函数——给定相同输入永远产生相同输出（确定性可重放）。
// 滞回状态所有权（spec Task 3）：prevRegion/prevSector 由调用方（AutomaticCalibration）持有，
// 每个点采集前注入 Config.PrevRegion/Config.PrevSector；首点 prevRegion="" 跳过滞回。
// DetermineRegion 本身不存储任何状态，SevenHoleAlgorithm 保持空结构体。
//
// 四条 tie-break 规则（spec §3.2）：
//  1. P7 优先：|P7-Pmax|<tol 时判 inner/7（无条件，避免 Kφ 边界符号反转导致分区抖动）
//  2. 编号小优先：外围孔并列最大时选编号最小的孔作为扇区编号 n
//  3. 滞回机制：prevRegion=="outer" 且 prevSector 在 candidates 中且与 candidates 中
//     其他元素环形相邻（1↔2、2↔3、...、6↔1）时保持 prevSector
//  4. 首点无前序：prevRegion="" 时跳过滞回，仅按规则 1、2 判定
//
// 跨大跨度不滞回（spec §3.2 范围限制）：prevSector=1, candidates={3,4} 时
// 1 与 3、1 与 4 都不相邻，不触发滞回，按规则 2 选 n=3。

// SevenHoleTieBreakToleranceRange 定义 TIE_BREAK_TOLERANCE 配置的合法范围
//
// 选值依据（spec §3.2）：
//   - 下界 1 Pa：低于此值会被浮点噪声误触发（扫描阀精度 u1≈20 Pa）
//   - 上界 50 Pa：高于此值会把合法分区误判为边界点（数据集 1 区 φ=30° 处 P1-P2≈74 Pa）
const (
	DefaultSevenHoleTieBreakTolerance = 5.0  // 默认 5 Pa，依据扫描阀精度与数据集边界点压力差综合确定
	MinSevenHoleTieBreakTolerance     = 1.0  // 下界：避免浮点噪声误触发
	MaxSevenHoleTieBreakTolerance     = 50.0 // 上界：避免合法分区被误判为边界点
)

// sevenHoleTieBreakTolerance 包级配置变量，初始为默认值。
//
// 并发安全（code-review I1 修复）：
//   - 通过 sevenHoleTieBreakMu 读写锁保护，支持运行期动态调整
//   - SetSevenHoleTieBreakTolerance 加写锁写入
//   - GetSevenHoleTieBreakTolerance / DetermineRegion 加读锁读取
//   - 读多写少场景下 RWMutex 比 atomic.Pointer[float64] 更直观且兼容现有 API
var (
	sevenHoleTieBreakMu        sync.RWMutex
	sevenHoleTieBreakTolerance = DefaultSevenHoleTieBreakTolerance
)

// SetSevenHoleTieBreakTolerance 设置 TIE_BREAK_TOLERANCE 配置
//
// 参数 tol 必须在 [MinSevenHoleTieBreakTolerance, MaxSevenHoleTieBreakTolerance] 范围内，
// 超出范围返回错误且不修改内部状态（避免配置错误后静默退化）。
//
// 调用时机：校准任务启动前由 SevenHoleAlgorithm.ValidateConfig 或配置加载器调用一次，
// 运行期也可动态调整（加写锁保护，与 DetermineRegion 读路径串行化）。
func SetSevenHoleTieBreakTolerance(tol float64) error {
	if tol < MinSevenHoleTieBreakTolerance || tol > MaxSevenHoleTieBreakTolerance {
		return fmt.Errorf("TIE_BREAK_TOLERANCE=%.6f 超出合法范围 [%.1f, %.1f]",
			tol, MinSevenHoleTieBreakTolerance, MaxSevenHoleTieBreakTolerance)
	}
	sevenHoleTieBreakMu.Lock()
	defer sevenHoleTieBreakMu.Unlock()
	sevenHoleTieBreakTolerance = tol
	return nil
}

// GetSevenHoleTieBreakTolerance 读取当前 TIE_BREAK_TOLERANCE 配置
func GetSevenHoleTieBreakTolerance() float64 {
	sevenHoleTieBreakMu.RLock()
	defer sevenHoleTieBreakMu.RUnlock()
	return sevenHoleTieBreakTolerance
}

// DetermineRegion 七孔探针流场分区判定（spec §3.2）
//
// 函数签名严格按 Task 3 要求：无 tolerance 参数，使用包级配置 sevenHoleTieBreakTolerance。
// 需要自定义 tolerance 的场景（如测试或不同精度传感器）请直接调用 determineRegionImpl。
//
// 输入：
//   - p1..p7: 7 个压力孔的原始通道值（A 基准表压，可正可负）
//   - prevRegion: 上一时刻分区（"inner"/"outer"），首点传空串 ""
//   - prevSector: 上一时刻扇区编号（1..6 内区传 7），首点传 0
//
// 输出：
//   - region: "inner"（内区，7 区）或 "outer"（外区，1~6 区）
//   - n: 扇区编号（内区固定 7，外区 1..6）
//   - boundaryFlag: 边界点标记，无并列时为空串；并列时为 "P7-Pn" 或 "Pn-Pm"（编号升序）
//
// 确定性保证：相同输入永远产生相同输出（无随机性、无时间依赖、无内部状态读取）。
// 并发安全：通过 sevenHoleTieBreakMu.RLock 读取 tolerance 快照（code-review I1 修复）。
func DetermineRegion(p1, p2, p3, p4, p5, p6, p7 float64, prevRegion string, prevSector int) (region string, n int, boundaryFlag string) {
	sevenHoleTieBreakMu.RLock()
	tol := sevenHoleTieBreakTolerance
	sevenHoleTieBreakMu.RUnlock()
	return determineRegionImpl(p1, p2, p3, p4, p5, p6, p7, tol, prevRegion, prevSector)
}

// determineRegionImpl 分区判定的真正实现，接受显式 tolerance 参数
//
// 抽离此层的目的：
//  1. 测试可直接传入 tolerance，无需修改全局状态（避免测试间相互污染）
//  2. 未来若 SevenHoleConfig 增加每任务独立的 tolerance 字段，可直接复用此函数
//  3. DetermineRegion 作为薄包装保持 Task 3 要求的固定签名
//
// 实现按 spec §3.2 伪代码顺序：规则 1 → 规则 3（滞回） → 规则 2（编号小优先）。
func determineRegionImpl(p1, p2, p3, p4, p5, p6, p7, tol float64, prevRegion string, prevSector int) (region string, n int, boundaryFlag string) {
	pressures := [7]float64{p1, p2, p3, p4, p5, p6, p7}

	// 全局最大值 Pmax = max(P1..P7)
	pMax := pressures[0]
	for i := 1; i < 7; i++ {
		if pressures[i] > pMax {
			pMax = pressures[i]
		}
	}

	// 外围 6 孔最大值 outerMax = max(P1..P6)
	outerMax := pressures[0]
	for i := 1; i < 6; i++ {
		if pressures[i] > outerMax {
			outerMax = pressures[i]
		}
	}

	// candidates: 外围孔中与 outerMax 差值 < tol 的孔编号列表（升序，因按 1..6 顺序遍历）
	// spec §3.2 伪代码: candidates = [i for i in 1..6 if |Pi - outerMax| < TIE_BREAK_TOLERANCE]
	candidates := make([]int, 0, 6)
	for i := 0; i < 6; i++ {
		if math.Abs(pressures[i]-outerMax) < tol {
			candidates = append(candidates, i+1) // 编号 1..6
		}
	}

	// ==================== 规则 1：P7 优先（spec §3.2） ====================
	// |P7-Pmax|<tol 时无条件判内区——P7 与外围孔竞争时优先归入内区，
	// 避免 Kφ 边界符号反转导致的分区抖动（spec §4.3）
	//
	// boundary_flag 判定（spec §3.2 边界点标记）：
	//   - "P7 与外围任一孔并列最大"才标记 "P7-Pn"——即至少有一个外围孔与 Pmax 接近
	//   - P7 单独最大（所有外围孔都明显小于 Pmax）时不算并列，boundary_flag 为空
	//
	// 注意：这里用 Pmax（全局最大值）而非 outerMax 判定并列。
	// candidates 是"外围孔之间的并列"用于规则 2/3，不适用于规则 1 的"P7-外围孔"并列判定。
	if math.Abs(p7-pMax) < tol {
		region = "inner"
		n = 7
		// 找第一个与 Pmax 接近的外围孔（按编号 1..6 升序）
		for i := 0; i < 6; i++ {
			if math.Abs(pressures[i]-pMax) < tol {
				boundaryFlag = fmt.Sprintf("P7-P%d", i+1)
				break
			}
		}
		return
	}

	// ==================== 规则 3：滞回机制（spec §3.2，仅相邻扇区边界生效） ====================
	// 触发条件：
	//   - prevRegion == "outer"（仅 outer→outer 滞回，inner 不滞回）
	//   - len(candidates) >= 2（确有并列，避免无并列时空触发）
	//   - prevSector 在 candidates 中
	//   - prevSector 与 candidates 中其他元素环形相邻（1↔2、2↔3、...、6↔1）
	//
	// 跨大跨度不触发：prevSector=1, candidates={3,4} 时 1 与 3、1 与 4 都不相邻
	if prevRegion == "outer" && len(candidates) >= 2 {
		prevInCandidates := false
		for _, c := range candidates {
			if c == prevSector {
				prevInCandidates = true
				break
			}
		}
		if prevInCandidates {
			// 寻找 candidates 中与 prevSector 环形相邻的其他元素
			for _, c := range candidates {
				if c == prevSector {
					continue
				}
				if isRingAdjacent(prevSector, c, 6) {
					region = "outer"
					n = prevSector
					// boundary_flag 按编号升序排列（candidates 已升序，prevSector 与 c 的大小关系不定）
					if prevSector < c {
						boundaryFlag = fmt.Sprintf("P%d-P%d", prevSector, c)
					} else {
						boundaryFlag = fmt.Sprintf("P%d-P%d", c, prevSector)
					}
					return
				}
			}
		}
	}

	// ==================== 规则 2：编号小优先（spec §3.2） ====================
	// candidates 已按 1..6 顺序升序构建，candidates[0] 即 min(candidates)
	region = "outer"
	n = candidates[0]
	if len(candidates) >= 2 {
		// 取编号最小的两个并列孔作为 boundary_flag
		boundaryFlag = fmt.Sprintf("P%d-P%d", candidates[0], candidates[1])
	}
	// len(candidates)==1 时无并列，boundaryFlag 保持空串
	return
}

// isRingAdjacent 判断两个扇区编号在环形布局（1..N）中是否相邻
//
// 环形布局示例（N=6）：1↔2、2↔3、3↔4、4↔5、5↔6、6↔1
//
// 实现原理：环形中两元素相邻当且仅当其编号差的绝对值为 1 或 N-1
//   - 差 1：常规相邻（如 1↔2、5↔6）
//   - 差 N-1：环形首尾相邻（如 1↔6，差 5=N-1）
//
// 参数：
//   - a, b: 两个扇区编号（1..N）
//   - n: 环形布局总数（七孔探针外区固定为 6）
func isRingAdjacent(a, b, n int) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d == 1 || d == n-1
}

// ==================== SevenHoleAlgorithm 算法主体（spec Task 5） ====================

// SevenHoleAlgorithm 七孔探针校准算法
//
// 设计原则（spec Task 5）：空结构体，无状态——
//   - 滞回状态（prevRegion/prevSector）由 AutomaticCalibration 持有，每个点采集前通过 Config 注入
//   - 实时回调通过 Config.RealtimeCallback 注入（Algorithm 接口签名固定，无法新增参数）
//   - DetermineRegion 本身是无状态纯函数（spec Task 3）
//
// 此设计确保多次调用之间无副作用，且与五孔/三孔/总压的 Algorithm 实现模式一致。
type SevenHoleAlgorithm struct{}

// NewSevenHoleAlgorithm 创建七孔探针校准算法实例
func NewSevenHoleAlgorithm() *SevenHoleAlgorithm {
	return &SevenHoleAlgorithm{}
}

// Type 返回校准类型
func (a *SevenHoleAlgorithm) Type() CalibrationType {
	return TypeSevenHole
}

// ValidateConfig 校验七孔探针校准配置
//
// 必需通道角色（11 个，spec §6.1）：
//   - sevenHole.p1~p7：7 个压力孔（外围 6 孔 P1~P6 + 中心孔 P7）
//   - sevenHole.pTotal：风洞参考总压（K0/Ks/Ma 公式分母来源）
//   - sevenHole.pTunnelStatic：风洞参考静压（Ks/Ma 公式分母来源）
//   - sevenHole.pAtm：大气压力（A→C 边界转换用）
//   - sevenHole.tAtm：大气温度（静温/真空速计算用）
//
// 校验规则：
//   - ProbeChannels 非空
//   - 11 角色齐全（与 ReadProbeChannelsToSevenHoleRaw 的 roleMap 严格对应）
//   - SamplesPerPoint > 0
//   - 采样间隔 ≤ 每帧超时（避免 BatchPollIntervalMs > BatchTimeoutMs 导致每帧都超时）
func (a *SevenHoleAlgorithm) ValidateConfig(config Config) error {
	if len(config.ProbeChannels) == 0 {
		return fmt.Errorf("七孔探针校准需要配置探针通道")
	}

	requiredRoles := []string{
		"sevenHole.p1", "sevenHole.p2", "sevenHole.p3", "sevenHole.p4",
		"sevenHole.p5", "sevenHole.p6", "sevenHole.p7",
		"sevenHole.pTotal", "sevenHole.pTunnelStatic",
		"sevenHole.pAtm", "sevenHole.tAtm",
	}
	roleSet := make(map[string]bool)
	for _, ch := range config.ProbeChannels {
		roleSet[ch.Role] = true
	}
	var missingRoles []string
	for _, role := range requiredRoles {
		if !roleSet[role] {
			missingRoles = append(missingRoles, role)
		}
	}
	if len(missingRoles) > 0 {
		return fmt.Errorf("七孔探针校准缺少必需通道角色: %v", missingRoles)
	}

	if config.SamplesPerPoint <= 0 {
		return fmt.Errorf("samplesPerPoint 必须大于0")
	}

	// 采样间隔与超时联动校验（与总压一致，避免每帧都超时）
	if config.AcquisitionSampling != nil {
		intervalMs := config.AcquisitionSampling.BatchPollIntervalMs
		timeoutMs := config.AcquisitionSampling.BatchTimeoutMs
		if intervalMs > 0 && timeoutMs > 0 && intervalMs > timeoutMs {
			return fmt.Errorf("采样间隔(BatchPollIntervalMs=%dms)不能大于每帧超时(BatchTimeoutMs=%dms)", intervalMs, timeoutMs)
		}
	}

	return nil
}

// AcquireData 旧接口实现，仅作 Algorithm 接口兼容。
//
// 七孔自动校准流程实际走 AcquireDataWithConfig（携带 ProbeChannels），
// 此入口因缺乏通道配置无法完成真实采集——直接返回明确错误，
// 避免悄悄走"零值 fallback"老路径导致 Kα=0/Kβ=0/K0=0 无告警。
// 调用方应改用 AcquireDataWithConfig 或 AcquireDataWithChannels。
func (a *SevenHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	return nil, fmt.Errorf("七孔探针 AcquireData 旧接口不支持无通道配置采集，请改用 AcquireDataWithConfig")
}

// AcquireDataWithConfig 自动校准引擎调用入口：使用完整配置（含 ProbeChannels）采集单点。
//
// 从 config 取出 RealtimeCallback/PrevRegion/PrevSector 注入到 AcquireDataWithChannels，
// 算法本身不持有任何状态——所有跨点信息都通过 Config 传递。
func (a *SevenHoleAlgorithm) AcquireDataWithConfig(
	point CalPoint,
	channelReader ChannelValueReader,
	config Config,
	checkAbort func() bool,
	onSampleProgress func(current, total int),
) (DataPoint, error) {
	return a.AcquireDataWithChannels(
		point, channelReader, config.ProbeChannels, config.SamplesPerPoint,
		checkAbort, config.TimestampReader, config.AcquisitionStateProvider, onSampleProgress,
		config.RealtimeCallback, config.PrevRegion, config.PrevSector,
	)
}

// AcquireDataWithChannels 七孔探针数据采集主循环
//
// 采样策略（spec Task 5）：
//  1. samplesPerPoint 次读取，每次读 11 通道
//  2. timestampReader 非 nil 时优先等待设备新帧（避免重复读缓存旧数据）；
//     为 nil 时退化为固定 10ms sleep（与五孔/三孔/总压一致）
//  3. 100ms 节流实时推送：通过 realtimeCallback 回调（spec 明确移除 onRealtime 参数）
//     最后一个样本必发——确保前端拿到最终系数
//  4. 采样进度通过 onSampleProgress(i+1, samplesPerPoint) 回调
//  5. 采样完成后：
//     - 计算平均值（CalculateSevenHoleAverage）
//     - 调用 DetermineRegion 判定分区（使用 prevRegion/prevSector 滞回状态）
//     - 按分区调用 CalculateSevenHoleInnerCoefficients 或 CalculateSevenHoleOuterCoefficients
//     - 计算 P7 标准差（CalculateSevenHoleStdDev）
//     - 读取扫描阀 16 通道原始数据（readRawDeviceChannelsFromProbe，用于 CSV 落盘）
//
// checkAbort 返回 true 时立即返回 ErrPointAborted，由主循环回退索引重跑该点。
func (a *SevenHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	checkAbort func() bool,
	timestampReader TimestampReader,
	acquiringCheck AcquisitionStateProvider,
	onSampleProgress func(current, total int),
	realtimeCallback SevenHoleRealtimeCallback,
	prevRegion string,
	prevSector int,
) (*SevenHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	deviceIDs := collectUniqueDeviceIDs(probeChannels)
	lastTimestamps := make(map[string]int64)

	samples := make([]SevenHoleRawData, 0, samplesPerPoint)
	// 100ms 节流实时推送：避免高频采样时把前端打爆
	// 最后一个样本必发——确保前端拿到最终系数用于显示
	realtimeIntervalMs := int64(100)
	lastRealtimeSentAt := int64(0)

	for i := 0; i < samplesPerPoint; i++ {
		if checkAbort != nil && checkAbort() {
			return nil, ErrPointAborted
		}

		if i > 0 {
			if timestampReader != nil {
				if err := waitForFreshData(deviceIDs, timestampReader, lastTimestamps, freshnessDefaultTimeout, checkAbort, acquiringCheck); err != nil {
					if errors.Is(err, ErrPointAborted) {
						return nil, err
					}
					return nil, fmt.Errorf("等待新数据帧超时: %w", err)
				}
			} else {
				// 无设备时间戳读取能力时退化为固定间隔 sleep，避免全速空转读到同一帧缓存
				time.Sleep(10 * time.Millisecond)
			}
		}

		rawData, err := ReadProbeChannelsToSevenHoleRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取七孔探针通道失败: %w", err)
		}
		samples = append(samples, rawData)

		// 采样进度回调：每次采完一个样本通知上层，驱动 UI 显示"当前点采样 i+1/N"
		if onSampleProgress != nil {
			onSampleProgress(i+1, samplesPerPoint)
		}

		if timestampReader != nil {
			recordLastTimestamps(deviceIDs, timestampReader, lastTimestamps)
		}

		// 100ms 节流实时推送
		// 实时系数基于当前样本（非均值）计算——前端只需看到趋势，最终值由采集结束后的均值计算给出
		//
		// 触发条件（code-review I2 修复，补全首样本必发）：
		//   - i == 0：首样本必发，保证低采样数场景（如 samplesPerPoint=3）下也能拿到至少一次中间推送
		//   - now-lastRealtimeSentAt >= 100ms：节流周期到，发送趋势更新
		//   - i == samplesPerPoint-1：最后样本必发，确保前端拿到最终系数
		now := time.Now().UnixMilli()
		if realtimeCallback != nil && (i == 0 || now-lastRealtimeSentAt >= realtimeIntervalMs || i == samplesPerPoint-1) {
			// 实时推送的 region/sector 优先用预设区域（用户配置的轨迹区域），
			// 与最终落盘的 Region/Sector 一致——避免实时显示"外区"、最终落盘"内区"的闪烁。
			// point.Region 为空时（旧调用路径）回退到 DetermineRegion 压力判定。
			rtRegion := point.Region
			rtSector := point.Sector
			if rtRegion == "" {
				rtRegion, rtSector, _ = DetermineRegion(
					rawData.P1, rawData.P2, rawData.P3, rawData.P4, rawData.P5, rawData.P6, rawData.P7,
					prevRegion, prevSector,
				)
			}
			var realtimeCoeffs SevenHoleCoefficients
			if rtRegion == "inner" {
				realtimeCoeffs, _ = CalculateSevenHoleInnerCoefficients(rawData)
			} else {
				realtimeCoeffs, _ = CalculateSevenHoleOuterCoefficients(rawData, rtSector)
			}
			realtimeCallback(rawData, realtimeCoeffs, rtRegion, rtSector)
			lastRealtimeSentAt = now
		}
	}

	endTime := time.Now().UnixMilli()

	// 1. 计算平均值——后续 DetermineRegion 与系数计算都基于均值（降低单次样本噪声）
	avgData := CalculateSevenHoleAverage(samples)

	// 2. 区域归属：优先使用预设点位配置（point.Region/point.Sector）
	//
	// 校准轨迹的内外区是用户规划明确的——内区点用 α/β 网格，外区点用 θ/φ 网格，
	// GenerateSevenHolePoints 生成点位时已经填充了正确的 Region/Sector 字段。
	// 不应基于实时压力数据"判定"——压力数据在边界点扰动会导致误判，
	// 进而让数据点 Region 错位（CSV 路由错位、前端图表过滤错位）。
	//
	// DetermineRegion 仍调用一次，承担两个职责：
	//   - 始终生成 boundaryFlag 边界点标记（spec §3.2，CSV 边界标记列）
	//   - point.Region 为空时（旧调用路径或简陋测试用例）回退作为区域判定结果
	// 生产路径下 point.Region 已填充，DetermineRegion 的 region/sector 结果被丢弃，
	// 仅采用 boundaryFlag。
	detRegion, detSector, boundaryFlag := DetermineRegion(
		avgData.P1, avgData.P2, avgData.P3, avgData.P4, avgData.P5, avgData.P6, avgData.P7,
		prevRegion, prevSector,
	)
	region := point.Region
	sector := point.Sector
	if region == "" {
		// 兜底：point.Region 未填充时回退到压力判定（向后兼容旧调用路径）
		region = detRegion
		sector = detSector
	}

	// 3. 按预设区域调用系数计算
	var coefficients SevenHoleCoefficients
	var calcErr error
	if region == "inner" {
		coefficients, calcErr = CalculateSevenHoleInnerCoefficients(avgData)
	} else {
		coefficients, calcErr = CalculateSevenHoleOuterCoefficients(avgData, sector)
	}
	if calcErr != nil {
		return nil, fmt.Errorf("七孔系数计算失败 (region=%s, sector=%d): %w", region, sector, calcErr)
	}

	// 4. 计算 P7 标准差（采样稳定性指标）
	stdDev := CalculateSevenHoleStdDev(samples)

	// 5. 读取扫描阀 16 通道原始数据（用于 CSV 落盘，与五孔/三孔/总压保持一致）
	rawDeviceChannels := readRawDeviceChannelsFromProbe(channelReader, probeChannels)

	return &SevenHoleDataPoint{
		PointID:           point.ID,
		Coordinates:       point.Coordinates,
		MotionCoordinates: point.MotionCoordinates,
		Region:            region,
		Sector:            sector,
		BoundaryFlag:      boundaryFlag,
		RawData:           avgData,
		Coefficients:      coefficients,
		SampleCount:       len(samples),
		StdDev:            stdDev,
		StartTime:         startTime,
		EndTime:           endTime,
		RawDeviceChannels: rawDeviceChannels,
	}, nil
}

// ==================== 七孔探针点位生成（spec Task 6 / §6.2 / §9.1） ====================
//
// 七孔校准点位分内区（α-β 网格）与外区（θ-φ 网格）两套：
//   - 内区点：Coordinates={α,β}, MotionCoordinates={α,β}（相同），Region="inner", Sector=7
//   - 外区点：Coordinates={θ,φ}, MotionCoordinates={α',β'}（按 §3.3 正向公式换算），Region="outer", Sector=n（1~6）
//
// 两种校准模式（spec §6.2）：
//   - 完整模式（产品默认）：内区 169 + 外区 504 = 673 点
//   - 数据集模式（验证基准）：内区 169 + 外区 312 = 481 点
//
// 蛇形顺序（spec Task 6）：外层 β/φ 循环，奇数行 α/θ 反向
//
// 扇区编号计算（spec §3.3 / §6.2）：
//   - 外区点 Sector 由 φ 所在扇区决定：sector = (φ / 60°) 取整 + 1，范围 1..6
//   - φ=0°→Sector 1（P1 孔位），φ=60°→Sector 2（P2 孔位），...，φ=300°→Sector 6（P6 孔位）
//   - φ=360° 等价于 φ=0°，归入 Sector 1
//
// 浮点 round 到 1 位小数（spec Task 6）：
//
//	math.Round((Min+float64(i)*Step)*10) / 10

// SevenHoleMode 七孔校准模式枚举
type SevenHoleMode string

const (
	// SevenHoleModeFull 完整模式（产品默认，673 点）
	// 内区 α∈[-30°,30°] 步长 5° × β∈[-30°,30°] 步长 5° = 169 点
	// 外区 θ∈[30°,60°] 步长 5° × φ∈[0°,355°] 步长 5° = 504 点
	SevenHoleModeFull SevenHoleMode = "full"

	// SevenHoleModeDataset 数据集模式（验证基准，481 点）
	// 内区 169 点同完整模式
	// 外区 θ∈{30°,35°,40°,45°}（4 个值，不可配置）× 每扇区 φ 跨 60° 步长 5° = 13 点/扇区 × 6 扇区 = 312 点
	// 扇区边界不共享，无需去重（spec §6.2 / Task 6 验收标准）
	SevenHoleModeDataset SevenHoleMode = "dataset"
)

// SevenHoleConfig 七孔校准点位生成配置
//
// 字段语义：
//   - Mode：校准模式（"full"/"dataset"），决定外区 θ 与 φ 的取值策略
//   - InnerAlphaMin/Max/Step：内区 α 范围与步长（度），完整/数据集模式都使用此配置
//   - InnerBetaMin/Max/Step：内区 β 范围与步长（度）
//   - OuterThetaMin/Max/Step：外区 θ 范围与步长（度），仅完整模式生效；数据集模式忽略并使用硬编码 {30°,35°,40°,45°}
//   - OuterPhiMin/Max/Step：外区 φ 范围与步长（度），仅完整模式生效；数据集模式按扇区独立配置
//   - Serpentine：是否启用蛇形走位（true 时奇数行 α/θ 反向）
//
// 推荐默认值（spec §6.2）：内区 [-30°,30°] 步长 5°；外区 θ [30°,60°] 步长 5°、φ [0°,355°] 步长 5°
type SevenHoleConfig struct {
	Mode           SevenHoleMode `json:"mode"`
	InnerAlphaMin  float64       `json:"innerAlphaMin"`
	InnerAlphaMax  float64       `json:"innerAlphaMax"`
	InnerAlphaStep float64       `json:"innerAlphaStep"`
	InnerBetaMin   float64       `json:"innerBetaMin"`
	InnerBetaMax   float64       `json:"innerBetaMax"`
	InnerBetaStep  float64       `json:"innerBetaStep"`
	OuterThetaMin  float64       `json:"outerThetaMin"`
	OuterThetaMax  float64       `json:"outerThetaMax"`
	OuterThetaStep float64       `json:"outerThetaStep"`
	OuterPhiMin    float64       `json:"outerPhiMin"`
	OuterPhiMax    float64       `json:"outerPhiMax"`
	OuterPhiStep   float64       `json:"outerPhiStep"`
	Serpentine     bool          `json:"serpentine"`
}

// 数据集模式硬编码外区 θ 取值（spec §6.2 / Task 6 验收标准）
//
// 数据集 W532.202608.P.7H.1-01 的外区仅取这 4 个 θ 值，
// 用于算法正确性验证与回归测试，不可配置。
var sevenHoleDatasetThetaValues = []float64{30.0, 35.0, 40.0, 45.0}

// 数据集模式每扇区 φ 跨度（度）与步长（度）（spec §6.2）
//
// 每扇区跨 60°（如 Sector 1 覆盖 φ∈[-30°,+30°]，归一化到 [0°,360°) 后为 [330°,360°)∪[0°,30°]），
// 步长 5° → 13 点/扇区（含两端）。
//
// 扇区边界点不共享（spec §6.2）：Sector 1 的 φ=30° 与 Sector 2 的 φ=30° 是两个独立的点，
// 分别采集并归入各自扇区。
const (
	sevenHoleDatasetSectorSpan = 60.0
	sevenHoleDatasetPhiStep    = 5.0
)

// GenerateSevenHolePoints 根据配置生成七孔校准的所有点位（spec §9.1 接口契约）
//
// 返回 []CalPoint，每个点同时填充 Coordinates（逻辑坐标）和 MotionCoordinates（运动坐标）：
//   - 内区点：Coordinates={α,β}, MotionCoordinates={α,β}（相同），Region="inner", Sector=7
//   - 外区点：Coordinates={θ,φ}, MotionCoordinates={α',β'}（按 §3.3 正向公式换算），Region="outer", Sector=n（1~6）
//
// 蛇形顺序（spec Task 6）：外层 β/φ 循环，奇数行 α/θ 反向（reverse := Serpentine && bi%2 == 1）
//
// 完整模式：内区 169 + 外区 504 = 673 点
// 数据集模式：内区 169 + 外区 312 = 481 点
//
// 错误返回：
//   - 步长 ≤ 0（无法计算点数）
//   - 范围 min > max（无有效点位）
//   - 内区步长未配置（Mode 字段空时默认完整模式）
func GenerateSevenHolePoints(config SevenHoleConfig) ([]CalPoint, error) {
	// 默认模式：空 Mode 视为完整模式（spec §6.2 产品默认）
	mode := config.Mode
	if mode == "" {
		mode = SevenHoleModeFull
	}

	// 1. 生成内区点位（两种模式共用）
	innerPoints, err := generateSevenHoleInnerPoints(config)
	if err != nil {
		return nil, fmt.Errorf("内区点位生成失败: %w", err)
	}

	// 2. 生成外区点位（按模式分流）
	var outerPoints []CalPoint
	if mode == SevenHoleModeDataset {
		outerPoints, err = generateSevenHoleDatasetOuterPoints(config)
	} else {
		outerPoints, err = generateSevenHoleFullOuterPoints(config)
	}
	if err != nil {
		return nil, fmt.Errorf("外区点位生成失败: %w", err)
	}

	// 3. 合并：内区在前，外区在后（spec §6.2 遍历顺序）
	//    外区点位 ID 需要从内区之后继续编号——子生成器各自从 1 开始独立编号，
	//    合并时必须重编号，否则内/外区会出现 ID 冲突。
	points := make([]CalPoint, 0, len(innerPoints)+len(outerPoints))
	points = append(points, innerPoints...)
	innerCount := len(innerPoints)
	for i := range outerPoints {
		outerPoints[i].ID = innerCount + i + 1
	}
	points = append(points, outerPoints...)

	return points, nil
}

// SevenHolePreviewResult 七孔点位预览结果（spec Task 9）
//
// 用于前端"配置向导"在用户调整 α/β/θ/φ 范围与步长时实时显示总点数与内/外区分布，
// 让操作员在启动校准前确认点位规模，避免配置错误导致 600+ 点采集浪费。
//
// 字段说明：
//   - Points：完整点位列表（含 Coordinates + MotionCoordinates + Region + Sector）
//   - TotalCount：总点数 = InnerCount + OuterCount
//   - InnerCount：内区点数（α-β 网格，默认 169 点）
//   - OuterCount：外区点数（6 扇区 × φ/θ 网格，完整模式 504 点 / 数据集模式 312 点）
//
// Points 字段供前端可视化布点（如未启动校准时的"点位预览图"）；TotalCount 等聚合字段
// 供前端状态栏直接显示，避免前端遍历 600+ 点计算分布。
type SevenHolePreviewResult struct {
	Points     []CalPoint `json:"points"`
	TotalCount int        `json:"totalCount"`
	InnerCount int        `json:"innerCount"`
	OuterCount int        `json:"outerCount"`
}

// generateSevenHoleInnerPoints 生成内区点位（α-β 网格，蛇形顺序）
//
// 蛇形顺序（spec Task 6）：外层 β 循环，奇数行 α 反向
//   - bi 偶数行：α 从 AlphaMin 升序到 AlphaMax
//   - bi 奇数行（且 Serpentine=true）：α 从 AlphaMax 降序到 AlphaMin
//
// 内区点字段填充：
//   - Coordinates = {"α": alpha, "β": beta}
//   - MotionCoordinates = {"α": alpha, "β": beta}（与逻辑坐标相同）
//   - Region = "inner", Sector = 7
func generateSevenHoleInnerPoints(config SevenHoleConfig) ([]CalPoint, error) {
	if config.InnerAlphaStep <= 0 || config.InnerBetaStep <= 0 {
		return nil, fmt.Errorf("内区步长必须 > 0 (alpha=%.6f, beta=%.6f)", config.InnerAlphaStep, config.InnerBetaStep)
	}
	if config.InnerAlphaMin > config.InnerAlphaMax || config.InnerBetaMin > config.InnerBetaMax {
		return nil, fmt.Errorf("内区范围 min > max (alpha: %.6f > %.6f, beta: %.6f > %.6f)",
			config.InnerAlphaMin, config.InnerAlphaMax, config.InnerBetaMin, config.InnerBetaMax)
	}

	alphaCount := int(math.Round((config.InnerAlphaMax-config.InnerAlphaMin)/config.InnerAlphaStep)) + 1
	betaCount := int(math.Round((config.InnerBetaMax-config.InnerBetaMin)/config.InnerBetaStep)) + 1

	points := make([]CalPoint, 0, alphaCount*betaCount)
	id := 1

	for bi := 0; bi < betaCount; bi++ {
		beta := roundTo1Decimal(config.InnerBetaMin + float64(bi)*config.InnerBetaStep)
		// 蛇形走位：奇数行 α 反向（spec Task 6）
		reverse := config.Serpentine && bi%2 == 1

		for ai := 0; ai < alphaCount; ai++ {
			alphaIdx := ai
			if reverse {
				alphaIdx = alphaCount - 1 - ai
			}
			alpha := roundTo1Decimal(config.InnerAlphaMin + float64(alphaIdx)*config.InnerAlphaStep)

			points = append(points, CalPoint{
				ID:                id,
				Coordinates:       map[string]float64{"α": alpha, "β": beta},
				MotionCoordinates: map[string]float64{"α": alpha, "β": beta},
				Region:            "inner",
				Sector:            7,
			})
			id++
		}
	}

	return points, nil
}

// generateSevenHoleFullOuterPoints 完整模式外区点位生成（θ-φ 网格，蛇形顺序）
//
// 遍历顺序（与基准数据集 W532.202608.P.7H.1-01 对齐，code-review 调整）：
//   - 外层：θ 循环（俯仰角，慢轴）—— 先固定俯仰角，扫描一圈方位
//   - 内层：φ 循环（滚转角，旋转台快速轴）—— φ 递增扫描
//     物理含义：俯仰角变更代价高（探针姿态调整），滚转角代价低（旋转台扫描），
//     先固定 θ 扫一圈 φ 可减少机构换姿态次数，与既有校准数据时序完全一致。
//
// 蛇形走位（spec Task 6）：奇数 θ 行的 φ 反向
//   - 例：θ=30°（第0行）φ 正向；θ=35°（第1行）φ 反向
//   - 旋转台无需回程即可直接进入下一俯仰角，缩短校准时间
//
// 外区点字段填充：
//   - Coordinates = {"θ": theta, "φ": phi}
//   - MotionCoordinates = {"α": alpha', "β": beta'}（按 §3.3 正向公式换算）
//   - Region = "outer", Sector = computeSectorFromPhi(phi)
func generateSevenHoleFullOuterPoints(config SevenHoleConfig) ([]CalPoint, error) {
	if config.OuterThetaStep <= 0 || config.OuterPhiStep <= 0 {
		return nil, fmt.Errorf("外区步长必须 > 0 (theta=%.6f, phi=%.6f)", config.OuterThetaStep, config.OuterPhiStep)
	}
	if config.OuterThetaMin > config.OuterThetaMax || config.OuterPhiMin > config.OuterPhiMax {
		return nil, fmt.Errorf("外区范围 min > max (theta: %.6f > %.6f, phi: %.6f > %.6f)",
			config.OuterThetaMin, config.OuterThetaMax, config.OuterPhiMin, config.OuterPhiMax)
	}

	thetaCount := int(math.Round((config.OuterThetaMax-config.OuterThetaMin)/config.OuterThetaStep)) + 1
	phiCount := int(math.Round((config.OuterPhiMax-config.OuterPhiMin)/config.OuterPhiStep)) + 1

	// 完整模式 ID 从内区之后继续（内区 169 点占用 ID 1..169，外区从 170 开始）
	// 调用方 GenerateSevenHolePoints 先生成内区再生成外区，外区 ID 起始 = 内区点数 + 1
	// 但此处 generateSevenHoleFullOuterPoints 不知道内区点数，由 GenerateSevenHolePoints 重新编号
	// 简化：此处 ID 从 1 开始，最后由 GenerateSevenHolePoints 合并时重编号
	points := make([]CalPoint, 0, thetaCount*phiCount)
	id := 1

	// 外层：θ 循环（俯仰角，慢轴）
	for ti := 0; ti < thetaCount; ti++ {
		theta := roundTo1Decimal(config.OuterThetaMin + float64(ti)*config.OuterThetaStep)
		// 蛇形走位：奇数 θ 行的 φ 反向（spec Task 6）
		reverse := config.Serpentine && ti%2 == 1

		// 内层：φ 循环（滚转角，旋转台快速轴）
		for pi := 0; pi < phiCount; pi++ {
			phiIdx := pi
			if reverse {
				phiIdx = phiCount - 1 - pi
			}
			phi := roundTo1Decimal(config.OuterPhiMin + float64(phiIdx)*config.OuterPhiStep)

			alpha, beta := ConvertThetaPhiToAlphaBeta(theta, phi)
			sector := computeSectorFromPhi(phi)

			points = append(points, CalPoint{
				ID:                id,
				Coordinates:       map[string]float64{"θ": theta, "φ": phi},
				MotionCoordinates: map[string]float64{"α": roundTo1Decimal(alpha), "β": roundTo1Decimal(beta)},
				Region:            "outer",
				Sector:            sector,
			})
			id++
		}
	}

	return points, nil
}

// generateSevenHoleDatasetOuterPoints 数据集模式外区点位生成（spec §6.2）
//
// 数据集模式硬编码（spec §6.2 / Task 6 验收标准）：
//   - θ 取固定值 {30°, 35°, 40°, 45°}（4 个值，忽略 config.OuterThetaMin/Max/Step）
//   - φ 按扇区跨 60° 步长 5° = 13 点/扇区，6 扇区共 78 个 φ 值（扇区边界不共享，无需去重）
//   - 总点数：4 × 13 × 6 = 312
//
// 扇区 φ 范围（spec §3.1 扇区居中约定，code-review C3 修复）：
//   - Sector 1 (P1 孔位)：φ ∈ [-30°, +30°]，归一化到 [0°,360°) 后跨 0°：[330°,360°) ∪ [0°,30°]
//   - Sector 2 (P2 孔位)：φ ∈ [30°, 90°]
//   - Sector 3 (P3 孔位)：φ ∈ [90°, 150°]
//   - Sector 4 (P4 孔位)：φ ∈ [150°, 210°]
//   - Sector 5 (P5 孔位)：φ ∈ [210°, 270°]
//   - Sector 6 (P6 孔位)：φ ∈ [270°, 330°]
//
// 扇区居中原理：每个扇区的中心 φ = (sector-1)×60°，对应 Pn 孔位的方位角。
// Sector 1 中心 φ=0°（+Y 方向，P1 孔位）→ φ ∈ [-30°, +30°] 覆盖 P1 孔位 ±30° 范围。
//
// 注意：扇区边界 φ 值（如 φ=30°）同时出现在两个扇区中——
// spec §6.2 明确"扇区边界不共享，无需去重"，每个扇区独立采集。
//
// 遍历顺序（与基准数据集 W532.202608.P.7H.1-01 对齐，code-review 调整）：
//   - 最外层：4 个 θ 值（30° → 35° → 40° → 45°）—— 先固定俯仰角
//   - 中层：6 个扇区（Sector 1 → 2 → ... → 6）—— 跨所有扇区连续扫描 360°
//   - 内层：13 个 φ 值（扇区起始 -30° → +30°，步长 5°）—— φ 是旋转台快速轴
//     物理含义：俯仰角变更代价高（探针姿态调整），滚转角代价低（旋转台扫描），
//     先固定 θ 走完一圈 360°（跨所有 6 扇区）再切下一个 θ，θ 轴换次数 = 3 次
//     （vs 按扇区分组的 24 次），大幅减少姿态调整次数。
//
// 蛇形走位（spec Task 6，config.Serpentine=true 时启用，默认 false）：
//   - 奇数 θ 行的扇区顺序和 φ 方向都反向
//   - 例：θ=30°（第0行）Sector 1→2→...→6, φ 正向（330°→30°→90°→...→330°），停在 330°
//     θ=35°（第1行）Sector 6→5→...→1, φ 反向（330°→270°→...→30°→330°），停在 330°
//   - 旋转台无需回程即可直接进入下一俯仰角，θ 轴切换时 φ 轴停在 330° 不动
//
// 非蛇形（默认，与基准数据集一致）：
//   - 每个 θ 行都从 Sector 1 起 φ 正向扫描到 Sector 6 末尾（330°）
//   - θ 轴切换时 φ 轴从 Sector 6 末尾（330°）跳回 Sector 1 起点（330°）——
//     由于两个位置 φ 值相同，φ 轴实际不动
func generateSevenHoleDatasetOuterPoints(config SevenHoleConfig) ([]CalPoint, error) {
	// 数据集模式忽略 config 的 OuterTheta/OuterPhi 配置，使用硬编码值
	thetaValues := sevenHoleDatasetThetaValues                                  // 4 个 θ 值
	phiPerSector := int(sevenHoleDatasetSectorSpan/sevenHoleDatasetPhiStep) + 1 // 13 点/扇区

	points := make([]CalPoint, 0, len(thetaValues)*phiPerSector*6)
	id := 1

	// 最外层：4 个 θ 值，先固定俯仰角
	for ti := 0; ti < len(thetaValues); ti++ {
		theta := roundTo1Decimal(thetaValues[ti])
		// 蛇形走位：奇数 θ 行的扇区顺序和 φ 方向都反向（spec Task 6）
		reverse := config.Serpentine && ti%2 == 1

		// 中层：6 个扇区（蛇形反向时顺序倒置：6→5→...→1）
		for si := 0; si < 6; si++ {
			sectorIdx := si
			if reverse {
				sectorIdx = 5 - si
			}
			sector := sectorIdx + 1

			// 扇区 φ 起始角度（spec §3.1 扇区居中）：sectorPhiStart = (sector-1)×60° - 30°
			// Sector 1 起始 -30°（归一化后 330°），Sector 2 起始 30°，...
			sectorPhiStart := float64(sector-1)*sevenHoleDatasetSectorSpan - sevenHoleDatasetSectorSpan/2

			// 内层：13 个 φ 值（蛇形反向时方向倒置）
			for pi := 0; pi < phiPerSector; pi++ {
				// φ 计算后归一化到 [0°, 360°)，处理 Sector 1 跨 0° 的情况
				// 例：sectorPhiStart=-30°, pi=0 → phi=-30° → 归一化 330°
				//     sectorPhiStart=-30°, pi=6 → phi=0°   → 归一化 0°
				//     sectorPhiStart=-30°, pi=12 → phi=30° → 归一化 30°
				phiIdx := pi
				if reverse {
					phiIdx = phiPerSector - 1 - pi
				}
				phiRaw := sectorPhiStart + float64(phiIdx)*sevenHoleDatasetPhiStep
				phi := math.Mod(phiRaw+360.0, 360.0)
				phi = roundTo1Decimal(phi)

				alpha, beta := ConvertThetaPhiToAlphaBeta(theta, phi)

				points = append(points, CalPoint{
					ID:                id,
					Coordinates:       map[string]float64{"θ": theta, "φ": phi},
					MotionCoordinates: map[string]float64{"α": roundTo1Decimal(alpha), "β": roundTo1Decimal(beta)},
					Region:            "outer",
					Sector:            sector,
				})
				id++
			}
		}
	}

	return points, nil
}

// computeSectorFromPhi 根据 φ 计算扇区编号（spec §3.3 / §6.2）
//
// 扇区划分：每 60° 一个扇区，从 +Y 轴（φ=0°）起算
//   - φ ∈ [0°, 60°)  → Sector 1 (P1 孔位)
//   - φ ∈ [60°, 120°) → Sector 2 (P2 孔位)
//   - φ ∈ [120°, 180°) → Sector 3 (P3 孔位)
//   - φ ∈ [180°, 240°) → Sector 4 (P4 孔位)
//   - φ ∈ [240°, 300°) → Sector 5 (P5 孔位)
//   - φ ∈ [300°, 360°) → Sector 6 (P6 孔位)
//   - φ = 360° 等价于 φ = 0°，归入 Sector 1
//
// 注意：扇区边界点（如 φ=60°）的归属由 DetermineRegion 运行时按压力数据判定，
// 此处点位生成的 Sector 字段仅用于初始标注，最终 Sector 以 DetermineRegion 结果为准。
func computeSectorFromPhi(phiDeg float64) int {
	// 归一化到 [0°, 360°)
	normalized := math.Mod(phiDeg, 360.0)
	if normalized < 0 {
		normalized += 360.0
	}
	// φ=360° 归入 Sector 1（与 φ=0° 等价）
	if normalized >= 360.0-1e-9 {
		return 1
	}
	sector := int(normalized/sevenHoleDatasetSectorSpan) + 1
	if sector < 1 {
		sector = 1
	}
	if sector > 6 {
		sector = 6
	}
	return sector
}

// roundTo1Decimal 浮点 round 到 1 位小数（spec Task 6）
//
// 用于角度坐标的标准化——避免浮点累积误差导致 CSV/图表展示出现 30.0000000001 等长尾。
// 公式：math.Round((value)*10) / 10
func roundTo1Decimal(value float64) float64 {
	return math.Round(value*10) / 10
}
