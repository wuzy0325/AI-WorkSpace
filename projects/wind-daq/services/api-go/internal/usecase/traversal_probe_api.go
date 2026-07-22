// Package usecase — Task 09：calculateRealtime 路由的 transport-neutral 入口。
//
// 本文件提供 ProbePressureInput（导出输入 DTO）与 CalculateRealtimeForAPI
// 单一 dispatch 方法，让 API 不再 import coreinterp/seveninterp 来构造
// InterpolationInput；P6/P7 presence 校验、探针类型一致性检查均下沉到
// usecase 内部完成。响应仍返回具体 InterpolationResult 类型以保持既有
// JSON 字段形状（spec §5.6 兼容）。
package usecase

import (
	"errors"
	"fmt"

	"wind-daq/services/api-go/internal/core/traversal"
)

// ErrMissingSevenHolePressures 七孔请求缺失 P6 或 P7 时返回的 sentinel error。
// 用 errors.Is 识别，方便上层（API/Wails）区分"缺失"与"探针不匹配"两种 400 错误。
var ErrMissingSevenHolePressures = errors.New("七孔实时计算必须显式提供 P6/P7 压力字段")

// ProbePressureInput 是 calculateRealtime 路由的 transport-neutral 输入。
// P6/P7 用 *float64 表达 presence：nil=缺失（七孔必填），&0=已提供（present-zero）。
// 五孔请求可不填 P6/P7（保持与旧 body 兼容）。
type ProbePressureInput struct {
	P1   float64
	P2   float64
	P3   float64
	P4   float64
	P5   float64
	P6   *float64 // 七孔必填；五孔可 nil
	P7   *float64 // 七孔必填；五孔可 nil
	PAtm float64
	TAtm float64
}

// CalculateRealtimeForAPI 是 calculateRealtime 路由的 usecase dispatch 入口。
//
// 行为契约（与既有 API 行为逐字节对齐）：
//   - probeType 为空 → 归一化为五孔（legacy body 兼容）。
//   - 七孔请求缺失 P6/P7（nil）→ 返回 ErrMissingSevenHolePressures（present-zero 语义保留）。
//   - 请求探针类型与 manager 当前 config.ProbeType 不一致 → 拒绝（防陈旧校准误用）。
//   - 未知 probeType → 拒绝。
//   - 返回值为具体 InterpolationResult 类型（五孔 coreinterp.InterpolationResult，
//     七孔 seveninterp.InterpolationResult），保持既有 JSON 字段形状；API 直接
//     writeJSON(result) 即可，无需 import 共享算法包。
func (m *TraversalManager) CalculateRealtimeForAPI(probeType string, in ProbePressureInput) (any, error) {
	requested := normalizeProbeType(probeType)

	// 七孔 P6/P7 presence 校验：present-zero 语义（nil=缺失，&0=已提供）。
	// 校验下沉到 usecase，API 不再持有 *float64 业务逻辑。
	if requested == traversal.ProbeTypeSevenHole {
		if in.P6 == nil || in.P7 == nil {
			return nil, fmt.Errorf("%w: P6 提供=%v, P7 提供=%v", ErrMissingSevenHolePressures, in.P6 != nil, in.P7 != nil)
		}
	}

	// 构造内部 probeCalcInput（*float64 解引用；五孔路径忽略 P[5]/P[6]）
	probeIn := probeCalcInput{
		P:    [7]float64{in.P1, in.P2, in.P3, in.P4, in.P5},
		PAtm: in.PAtm,
		TAtm: in.TAtm,
	}
	if in.P6 != nil {
		probeIn.P[5] = *in.P6
	}
	if in.P7 != nil {
		probeIn.P[6] = *in.P7
	}

	// 类型一致性校验：与 CalculateRealtimeByProbe 共用语义。
	m.mu.RLock()
	current := normalizeProbeType(m.config.ProbeType)
	m.mu.RUnlock()
	if requested != current {
		return nil, fmt.Errorf("请求探针类型 %q 与当前配置 %q 不一致", requested, current)
	}

	// 分发到底层计算路径，返回具体 InterpolationResult 类型。
	switch requested {
	case traversal.ProbeTypeFiveHole:
		// 五孔委托既有 CalculateRealtime（保留 InterpolationCache 路径与完整字段）。
		return m.CalculateRealtime(toFiveHoleInput(probeIn))
	case traversal.ProbeTypeSevenHole:
		// 七孔直调 calculateSevenHole（一期不经缓存，spec §5.2 第 5 条）。
		return m.calculateSevenHole(probeIn)
	}
	return nil, fmt.Errorf("未知探针类型: %q", probeType)
}
