package traversal

import (
	"encoding/json"
	"math"
)

// Point 的 JSON 序列化契约：NaN ↔ null。
//
// 设计原因：
//   - line/rectangle/sector 模式通过 markAxesNaN 把"未配置轴"标记为 NaN，
//     availableAxisTargets / waitForMotionComplete 用 math.IsNaN 跳过这些轴，
//     不会发 MoveTo 也不参与到位判定。运动恢复严格依赖 Config.Path 中的 NaN 语义。
//   - Go 标准库 encoding/json 不支持 float64 的 NaN/Inf 序列化，会返回
//     "json: unsupported value: NaN" 错误，导致 v2 三阶段提交的 result log
//     (json.Marshal) 与 checkpoint (json.MarshalIndent) 全部失败。
//   - 前端 TraversalCoordValue = number | null（shared/types/traversal.ts）
//     早已预期此契约，后端在此处落地。
//
// 序列化：NaN → null；非 NaN → 原始数字。
// 反序列化：null 或缺字段 → NaN（恢复"未配置轴"语义）；数字 → 原值。
// 这样 Config.Path 经 checkpoint 往返后仍能被 availableAxisTargets 正确识别。
//
// 不用 json.Number / 不用 string：保持 wire format 与前端 number | null 对齐，
// 避免引入额外类型转换层。
var _ json.Marshaler = Point{}
var _ json.Unmarshaler = (*Point)(nil)

// marshalFloat 把 float64 序列化为 JSON token：NaN 返回 null，其他走 json.Marshal。
// 抽出避免在每个字段重复 if/else。
func marshalFloat(v float64) []byte {
	if math.IsNaN(v) {
		return []byte("null")
	}
	// json.Marshal 对 float64 非 NaN/Inf 是稳定的；用 strconv.FormatFloat 也行，
	// 但走 json.Marshal 能保证与 encoding/json 的 number 格式完全一致（如整数无小数点）。
	b, _ := json.Marshal(v)
	return b
}

// MarshalJSON 实现 Point 的 JSON 序列化：NaN 字段输出 null。
// 手写对象构造而非用匿名 struct + json.Marshal，避免内层 float64 再次触发
// "unsupported value: NaN" 错误——匿名 struct 不会递归调用 Point.MarshalJSON。
func (p Point) MarshalJSON() ([]byte, error) {
	return []byte(`{"x":` + string(marshalFloat(p.X)) +
		`,"y":` + string(marshalFloat(p.Y)) +
		`,"z":` + string(marshalFloat(p.Z)) +
		`,"u":` + string(marshalFloat(p.U)) + `}`), nil
}

// UnmarshalJSON 实现 Point 的 JSON 反序列化。
//
// 三态语义：
//   - 显式 null → NaN（恢复 markAxesNaN 标记的"未配置轴"，运动恢复必需）
//   - 缺字段    → 0（向后兼容：旧 checkpoint 无 z/u 字段时不应被识别为"未配置"，
//                    types.go Point struct 注释明确要求"U 字段零值为 0"）
//   - 显式数字  → 原值（含 0：用户真的配置了 0 位置）
//
// 关键差异：null ≠ 缺字段。MarshalJSON 永远输出 4 个字段（NaN 输出 null），
// 所以新 checkpoint 经往返后 NaN 严格保留；旧 checkpoint 缺字段则按 0 兼容。
func (p *Point) UnmarshalJSON(data []byte) error {
	// 用临时 struct 解析，字段使用 *float64 指针区分三态：
	//   nil 指针 + JSON 没有该 key  → 缺字段 → 0
	//   nil 指针 + JSON 有 null      → 显式 null → NaN
	// json.Unmarshal 对这两种情况都把 *float64 留作 nil，无法直接区分，
	// 所以走 map[string]json.RawMessage 探测 key 是否存在。
	var raw struct {
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
		Z *float64 `json:"z"`
		U *float64 `json:"u"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// 探测每个 key 是否在 JSON 中显式出现（区分"缺字段"与"显式 null"）
	keys := make(map[string]bool)
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err == nil {
		for k := range probe {
			keys[k] = true
		}
	}
	p.X = floatPtrOrDefault(raw.X, keys["x"])
	p.Y = floatPtrOrDefault(raw.Y, keys["y"])
	p.Z = floatPtrOrDefault(raw.Z, keys["z"])
	p.U = floatPtrOrDefault(raw.U, keys["u"])
	return nil
}

// floatPtrOrDefault 把 *float64 转为 float64：
//   - 非 nil → *v（显式数字，含 0）
//   - nil + key 存在 → NaN（显式 null，恢复 markAxesNaN 语义）
//   - nil + key 缺失 → 0（缺字段，向后兼容旧 checkpoint）
func floatPtrOrDefault(v *float64, keyPresent bool) float64 {
	if v != nil {
		return *v
	}
	if keyPresent {
		// 显式 null：恢复"未配置轴"语义
		return math.NaN()
	}
	// 缺字段：保持 Go 零值兼容（types.go Point 注释要求）
	return 0
}
