// Package usecase — traversal 探针策略注册表（spec-seven-hole-traversal §5.2）
//
// 遍历管线中随探针类型变化的部分（通道标签集、插值器持有/加载判定、实时计算
// 适配）集中在本文件的包级无状态策略表，避免在采集/视图/恢复各处重复
// if probeType 分支。策略函数显式接收 *TraversalManager，表项不捕获任何
// Manager 实例，保证多实例并行运行与并行测试之间不共享状态。
package usecase

import (
	"fmt"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/core/traversal"
)

// probeCalcInput 探针无关插值输入（P 五孔仅用前 5 元素，七孔用全部 7 元素）。
type probeCalcInput struct {
	P    [7]float64 // 探针孔道表压 Pa（P[6] 为七孔中心孔 P7）
	PAtm float64    // 大气压力（绝压 Pa）
	TAtm float64    // 大气温度（℃）
}

// probeCalcResult 探针无关插值结果（落盘/视图共用标量子集）。
type probeCalcResult struct {
	Alpha, Beta, Pt, Ps, Mach, Velocity float64
	IsValid                             bool
	Warning                             string
}

// probeStrategy 按探针类型的策略表项。
//
// 字段说明：
//   - pressureLabels：探针孔道标签集（五孔 P1..P5 / 七孔 P1..P7），驱动
//     BuildRawPressure 的标签归一化与齐全性校验。
//   - isLoaded：当前探针插值器是否已加载（供 HasLoadedInterpolator /
//     CheckPreconditions 按 config.ProbeType 判定）。调用方须持有 m.mu。
//   - calculate：标量子集计算路径，供采集落盘（CalculatedResult）与 API
//     使用；五孔委托既有 CalculateRealtime（保留 InterpolationCache 路径），
//     七孔直接调 m.sevenHoleInterpolator.Calculate（一期不经缓存，§5.2）。
//   - viewCalculate：实时视图（BuildDataPoints）结果构造，按探针自身结果
//     形状返回：五孔返回完整 coreinterp.InterpolationResult（响应形状与既有
//     行为逐字节一致），七孔返回 seveninterp.InterpolationResult。
//     inputReady=false 时返回 IsValid=false 零结果；计算错误写入 Warning。
type probeStrategy struct {
	pressureLabels []string
	isLoaded       func(m *TraversalManager) bool
	calculate      func(m *TraversalManager, in probeCalcInput) (probeCalcResult, error)
	viewCalculate  func(m *TraversalManager, in probeCalcInput, inputReady bool) any
}

// probeStrategies 包级无状态策略表（五项域禁止闭包捕获 Manager 实例）。
var probeStrategies = map[string]probeStrategy{
	traversal.ProbeTypeFiveHole: {
		pressureLabels: []string{"P1", "P2", "P3", "P4", "P5"},
		isLoaded: func(m *TraversalManager) bool {
			return m.interpolator != nil && m.interpolator.IsLoaded()
		},
		calculate:     fiveHoleCalculate,
		viewCalculate: fiveHoleViewCalculate,
	},
	traversal.ProbeTypeSevenHole: {
		pressureLabels: []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7"},
		isLoaded: func(m *TraversalManager) bool {
			return m.sevenHoleInterpolator != nil && m.sevenHoleInterpolator.IsLoaded()
		},
		calculate:     sevenHoleCalculate,
		viewCalculate: sevenHoleViewCalculate,
	},
}

// normalizeProbeType 将旧配置的空 probeType 归一化为五孔；未知非空值原样
// 返回（查表失败按未加载/拒绝处理，配置边界另负责报错）。
func normalizeProbeType(probeType string) string {
	if probeType == "" {
		return traversal.ProbeTypeFiveHole
	}
	return probeType
}

// probeStrategyFor 取探针策略；未知类型返回 ok=false。
func probeStrategyFor(probeType string) (probeStrategy, bool) {
	strategy, ok := probeStrategies[normalizeProbeType(probeType)]
	return strategy, ok
}

// toFiveHoleInput 转换为五孔插值输入（仅用前 5 孔）。
func toFiveHoleInput(in probeCalcInput) coreinterp.InterpolationInput {
	return coreinterp.InterpolationInput{
		P1: in.P[0], P2: in.P[1], P3: in.P[2], P4: in.P[3], P5: in.P[4],
		PAtm: in.PAtm, TAtm: in.TAtm,
	}
}

// toSevenHoleInput 转换为七孔插值输入（全部 7 孔）。
func toSevenHoleInput(in probeCalcInput) seveninterp.InterpolationInput {
	return seveninterp.InterpolationInput{
		P1: in.P[0], P2: in.P[1], P3: in.P[2], P4: in.P[3], P5: in.P[4], P6: in.P[5], P7: in.P[6],
		PAtm: in.PAtm, TAtm: in.TAtm,
	}
}

// probeCalcResultFromFiveHole 取五孔完整结果的标量子集。
func probeCalcResultFromFiveHole(res coreinterp.InterpolationResult) probeCalcResult {
	return probeCalcResult{
		Alpha: res.Alpha, Beta: res.Beta,
		Pt: res.TotalPressure, Ps: res.StaticPressure,
		Mach: res.MachNumber, Velocity: res.Velocity,
		IsValid: res.IsValid, Warning: res.Warning,
	}
}

// probeCalcResultFromSevenHole 取七孔结果的标量子集。
func probeCalcResultFromSevenHole(res seveninterp.InterpolationResult) probeCalcResult {
	return probeCalcResult{
		Alpha: res.Alpha, Beta: res.Beta,
		Pt: res.TotalPressure, Ps: res.StaticPressure,
		Mach: res.MachNumber, Velocity: res.Velocity,
		IsValid: res.IsValid, Warning: res.Warning,
	}
}

// calculateSevenHole 直调七孔插值器（一期不经 InterpolationCache，§5.2 第 5 条）。
func (m *TraversalManager) calculateSevenHole(in probeCalcInput) (seveninterp.InterpolationResult, error) {
	m.mu.RLock()
	interp := m.sevenHoleInterpolator
	m.mu.RUnlock()
	if interp == nil || !interp.IsLoaded() {
		return seveninterp.InterpolationResult{}, fmt.Errorf("七孔PRB插值数据未加载")
	}
	return interp.Calculate(toSevenHoleInput(in))
}

// fiveHoleCalculate 五孔标量计算：委托既有 CalculateRealtime（含缓存路径）。
func fiveHoleCalculate(m *TraversalManager, in probeCalcInput) (probeCalcResult, error) {
	res, err := m.CalculateRealtime(toFiveHoleInput(in))
	if err != nil {
		return probeCalcResult{}, err
	}
	return probeCalcResultFromFiveHole(res), nil
}

// sevenHoleCalculate 七孔标量计算：直调七孔插值器。
func sevenHoleCalculate(m *TraversalManager, in probeCalcInput) (probeCalcResult, error) {
	res, err := m.calculateSevenHole(in)
	if err != nil {
		return probeCalcResult{}, err
	}
	return probeCalcResultFromSevenHole(res), nil
}

// fiveHoleViewCalculate 五孔视图结果：形状与既有 BuildDataPoints 完全一致。
func fiveHoleViewCalculate(m *TraversalManager, in probeCalcInput, inputReady bool) any {
	if !inputReady {
		return coreinterp.InterpolationResult{IsValid: false}
	}
	calculated, err := m.CalculateRealtime(toFiveHoleInput(in))
	if err != nil {
		return coreinterp.InterpolationResult{IsValid: false, Warning: err.Error()}
	}
	return calculated
}

// sevenHoleViewCalculate 七孔视图结果：七孔包结果形状（JSON 字段与五孔对齐）。
func sevenHoleViewCalculate(m *TraversalManager, in probeCalcInput, inputReady bool) any {
	if !inputReady {
		return seveninterp.InterpolationResult{IsValid: false}
	}
	calculated, err := m.calculateSevenHole(in)
	if err != nil {
		return seveninterp.InterpolationResult{IsValid: false, Warning: err.Error()}
	}
	return calculated
}

// SetSevenHoleInterpolator 注入七孔插值器（与五孔 interpolator 字段相互独立）。
// 显式设置后启动恢复的陈旧错误不再适用，一并清除（与 SetInterpolator 同语义）。
func (m *TraversalManager) SetSevenHoleInterpolator(interp seveninterp.Interpolator) {
	m.mu.Lock()
	m.sevenHoleInterpolator = interp
	m.lastInterpolatorRestoreErr = ""
	m.mu.Unlock()
}

// ClearProbeInterpolator 清除指定探针类型的插值器（spec §5.2.1）。
// 前端切换探针类型前调用，防止陈旧校准数据被继续使用；未知类型返回 error。
func (m *TraversalManager) ClearProbeInterpolator(probeType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch probeType {
	case traversal.ProbeTypeFiveHole:
		m.interpolator = nil
		if m.interpCache != nil {
			m.interpCache.Clear()
		}
	case traversal.ProbeTypeSevenHole:
		m.sevenHoleInterpolator = nil
	default:
		return fmt.Errorf("未知探针类型: %q", probeType)
	}
	m.lastInterpolatorRestoreErr = ""
	return nil
}

// classifyCalculatedResult 根据插值流程的关键信号分类 CalculatedResult 的三态 Status 并构造实例。
//
// 抽取动机:RunCurrentPoint 内联的 switch 5 分支无法单元测试,且本逻辑是 CSV/UI
// 失败原因区分的核心,必须高可信——抽成纯函数后可逐分支断言。
//
// 三态判定矩阵(与 UI 实时插值卡片三态一一对应):
//   - strategyOK=false / hasAll=false → PrbMissing(配置层未就绪)
//   - interpErr!=nil + !interpolatorLoaded → PrbMissing(插值器未加载)
//   - interpErr!=nil + interpolatorLoaded  → Invalid(已加载但其他 err,如输入校验)
//   - interpRes.IsValid=true  → Valid + 数值
//   - interpRes.IsValid=false → Invalid(压力越界等数据层问题)
//
// 参数 interpolatorLoaded 由调用方通过 HasLoadedInterpolatorFor 获取,
// 保持纯函数特性(不依赖 manager 状态)。
//
// 命名说明:动词 classify 表达"按信号判定属于哪一态",比 build 更准确——
// 调用方读到 classifyCalculatedResult 即预期返回 Valid/PrbMissing/Invalid 三态之一。
func classifyCalculatedResult(
	strategyOK bool,
	hasAll bool,
	interpRes probeCalcResult,
	interpErr error,
	interpolatorLoaded bool,
) *traversal.CalculatedResult {
	if !strategyOK || !hasAll {
		// 策略未注册或通道不全:配置层未就绪
		return &traversal.CalculatedResult{
			Valid:  false,
			Status: traversal.CalcStatusPrbMissing,
		}
	}
	switch {
	case interpErr != nil && !interpolatorLoaded:
		// 插值器未加载 → 配置层问题
		return &traversal.CalculatedResult{
			Valid:  false,
			Status: traversal.CalcStatusPrbMissing,
		}
	case interpErr != nil:
		// 已加载但 err != nil(如未来新增的输入校验 err) → 数据层问题
		return &traversal.CalculatedResult{
			Valid:  false,
			Status: traversal.CalcStatusInvalid,
		}
	case interpRes.IsValid:
		return &traversal.CalculatedResult{
			Valid:  true,
			Status: traversal.CalcStatusValid,
			Alpha:  interpRes.Alpha,
			Beta:   interpRes.Beta,
			Pt:     interpRes.Pt,
			Ps:     interpRes.Ps,
			Mach:   interpRes.Mach,
		}
	default:
		// 已加载但 IsValid=false → 数据层问题(压力越界等)
		return &traversal.CalculatedResult{
			Valid:  false,
			Status: traversal.CalcStatusInvalid,
		}
	}
}

// HasLoadedInterpolatorFor 判定指定探针类型的插值器是否已加载 PRB/CSV 数据集。
//
// 设计动机:
// 前端 store.hasLoadedInterpolator 是 UI 三态判定的真相源(不依赖后端 warning 文本),
// 后端 CSV 落盘的 Status 也需要按相同真相源区分 PrbMissing / Invalid,
// 避免依赖 interpErr != nil 这种脆弱判定(未来插值器新增其他 err 会被误判为配置层问题)。
//
// 与 CalculateRealtimeByProbe 不同,此方法不做"请求类型与当前配置一致"校验,
// 仅回答"该探针类型的插值器是否已加载"——供采集循环在 err 路径下分类 Status 使用。
// 未知探针类型返回 false(与 strategy.isLoaded 行为一致)。
func (m *TraversalManager) HasLoadedInterpolatorFor(probeType string) bool {
	requested := normalizeProbeType(probeType)
	strategy, ok := probeStrategyFor(requested)
	if !ok {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return strategy.isLoaded(m)
}

// CalculateRealtimeByProbe 按显式探针类型分发实时插值（spec §5.2 第 4 条）。
// 请求类型必须与 Manager 当前 config.ProbeType 一致：不一致时拒绝计算，
// 不读取另一类型插值器；未知类型返回 error。
func (m *TraversalManager) CalculateRealtimeByProbe(probeType string, in probeCalcInput) (probeCalcResult, error) {
	m.mu.RLock()
	current := normalizeProbeType(m.config.ProbeType)
	m.mu.RUnlock()
	requested := normalizeProbeType(probeType)
	strategy, ok := probeStrategyFor(requested)
	if !ok {
		return probeCalcResult{}, fmt.Errorf("未知探针类型: %q", probeType)
	}
	if requested != current {
		return probeCalcResult{}, fmt.Errorf("请求探针类型 %q 与当前配置 %q 不一致", requested, current)
	}
	return strategy.calculate(m, in)
}

// CalculateSevenHoleRealtime 是七孔实时插值的 API 入口包装：
// 以七孔包输入/输出类型对外（避免 API 层接触内部 probeCalcInput/probeCalcResult），
// 内部经 CalculateRealtimeByProbe 完成"显式类型与当前配置一致"校验（spec §5.6）。
func (m *TraversalManager) CalculateSevenHoleRealtime(input seveninterp.InterpolationInput) (seveninterp.InterpolationResult, error) {
	res, err := m.CalculateRealtimeByProbe(traversal.ProbeTypeSevenHole, probeCalcInput{
		P:    [7]float64{input.P1, input.P2, input.P3, input.P4, input.P5, input.P6, input.P7},
		PAtm: input.PAtm,
		TAtm: input.TAtm,
	})
	if err != nil {
		return seveninterp.InterpolationResult{}, err
	}
	return seveninterp.InterpolationResult{
		Alpha:           res.Alpha,
		Beta:            res.Beta,
		MachNumber:      res.Mach,
		Velocity:        res.Velocity,
		DynamicPressure: res.Pt - res.Ps,
		TotalPressure:   res.Pt,
		StaticPressure:  res.Ps,
		IsValid:         res.IsValid,
		Warning:         res.Warning,
	}, nil
}
