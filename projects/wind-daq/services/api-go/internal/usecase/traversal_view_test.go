package usecase

import (
	"fmt"
	"math"
	"testing"

	"wind-daq/services/api-go/internal/ports"
)

// mockChannelUnitProvider 测试用 ChannelUnitProvider 实现。
//
// 按 channelIndex 返回预设 unit；未预设的通道返回 error，
// 模拟"通道未配置"场景，让 BuildRawPressure 走跳过路径。
type mockChannelUnitProvider struct {
	units map[int]string // channelIndex → unit
}

func (m *mockChannelUnitProvider) ChannelUnit(_ string, channelIndex int) (string, error) {
	if unit, ok := m.units[channelIndex]; ok {
		return unit, nil
	}
	return "", fmt.Errorf("channel %d not configured", channelIndex)
}

// 编译期断言：mockChannelUnitProvider 实现 ports.ChannelUnitProvider。
var _ ports.ChannelUnitProvider = (*mockChannelUnitProvider)(nil)

// floatEq 浮点近似比较（归一化涉及多次乘法，1e-6 容差足够）。
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// TestBuildRawPressure_AbsoluteNormalization 绝压类型归一化集成测试。
//
// 测试前置：
//   - mock unitProvider 配置 P1-P5 + Patm 通道单位为 "kPa"，Tatm 为 "degC"
//   - values: P1=200kPa(绝压), Patm=101.325kPa, Tatm=25℃
//   - pressureType="absolute"
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, unitProvider, "absolute")
//
// 期待结果：
//   - Patm 归一化为 101325 Pa（kPa→Pa 换算，不减）
//   - P1 归一化为 200000-101325=98675 Pa（绝压减 Patm）
//   - Tatm 保持原值 25（温度通道不参与归一化）
func TestBuildRawPressure_AbsoluteNormalization(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", 1: "kPa", 2: "kPa", 3: "kPa", 4: "kPa", 5: "kPa", 6: "degC",
	}}

	raw, input, ok := BuildRawPressure(values, labels, "dev-abs", provider, "absolute")

	if !ok {
		t.Fatalf("expected ok=true (all 7 labels present), got false")
	}
	// Patm 仅换算到 Pa，不减
	if !floatEq(raw["Patm"], 101325) {
		t.Errorf("Patm: expect 101325 Pa, got %v", raw["Patm"])
	}
	// P1-P5 绝压减 Patm：200kPa - 101.325kPa = 98.675kPa = 98675 Pa
	for _, label := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if !floatEq(raw[label], 98675) {
			t.Errorf("%s: expect 98675 Pa (200kPa abs - 101.325kPa atm), got %v", label, raw[label])
		}
	}
	// Tatm 保持原值
	if !floatEq(raw["Tatm"], 25) {
		t.Errorf("Tatm: expect 25 (unchanged), got %v", raw["Tatm"])
	}
	// InterpolationInput 应与 raw 一致
	if !floatEq(input.P1, 98675) || !floatEq(input.PAtm, 101325) || !floatEq(input.TAtm, 25) {
		t.Errorf("InterpolationInput mismatch: P1=%v PAtm=%v TAtm=%v", input.P1, input.PAtm, input.TAtm)
	}
}

// TestBuildRawPressure_GaugeNormalization 表压类型归一化集成测试。
//
// 测试前置：
//   - mock unitProvider 配置 P1-P5 + Patm 单位为 "kPa"
//   - values: P1=200kPa(表压), Patm=101.325kPa
//   - pressureType="gauge"
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, unitProvider, "gauge")
//
// 期待结果：
//   - Patm 归一化为 101325 Pa（kPa→Pa 换算，不减）
//   - P1 归一化为 200000 Pa（表压不减 Patm）
func TestBuildRawPressure_GaugeNormalization(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", 1: "kPa", 2: "kPa", 3: "kPa", 4: "kPa", 5: "kPa", 6: "degC",
	}}

	raw, _, ok := BuildRawPressure(values, labels, "dev-gauge", provider, "gauge")

	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	// Patm 仅换算到 Pa
	if !floatEq(raw["Patm"], 101325) {
		t.Errorf("Patm: expect 101325 Pa, got %v", raw["Patm"])
	}
	// P1-P5 表压不减：200kPa = 200000 Pa
	for _, label := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if !floatEq(raw[label], 200000) {
			t.Errorf("%s: expect 200000 Pa (gauge, no Patm subtraction), got %v", label, raw[label])
		}
	}
}

// TestBuildRawPressure_DegradedNilProvider 降级路径集成测试。
//
// 测试前置：
//   - unitProvider 传 nil（模拟旧测试/离线场景未注入 unitProvider）
//   - values: P1=200, Patm=101.325（任意单位，无归一化意义）
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, nil, "gauge")
//
// 期待结果：
//   - 不 panic
//   - rawPressure 保持原始值（P1=200, Patm=101.325）
//   - ok=false（降级路径 normalized=false，避免插值器拿到非 Pa 单位输入）
func TestBuildRawPressure_DegradedNilProvider(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}

	raw, _, ok := BuildRawPressure(values, labels, "dev-degraded", nil, "gauge")

	// 降级路径：normalized=false，避免插值器拿到非 Pa 单位输入产生错误结果
	if ok {
		t.Fatalf("expected ok=false (degraded path, normalized=false), got true")
	}
	// rawPressure 仍保持原始值（降级保留原值，仅 ok=false 让调用方跳过插值）
	if !floatEq(raw["P1"], 200) {
		t.Errorf("P1: expect 200 (raw, no normalization), got %v", raw["P1"])
	}
	if !floatEq(raw["Patm"], 101.325) {
		t.Errorf("Patm: expect 101.325 (raw, no normalization), got %v", raw["Patm"])
	}
	if !floatEq(raw["Tatm"], 25) {
		t.Errorf("Tatm: expect 25 (raw), got %v", raw["Tatm"])
	}
}

// TestBuildRawPressure_PartialChannelUnitFailure 部分通道单位查询失败测试。
//
// 测试前置：
//   - mock unitProvider 只配置 P1 + Patm 通道单位，P2-P5 查询失败
//   - values: P1=200kPa, P2-P5=200kPa, Patm=101.325kPa
//   - pressureType="gauge"
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, provider, "gauge")
//
// 期待结果：
//   - P1 归一化为 200000 Pa（成功换算）
//   - Patm 归一化为 101325 Pa（成功换算）
//   - P2-P5 保持原值 200（查询失败，跳过换算）
//   - 不 panic，ok=false（P2-P5 归一化失败导致 normalized=false，
//     避免插值器拿到混合单位 P1=200000Pa + P2-P5=200kPa 的错误输入）
func TestBuildRawPressure_PartialChannelUnitFailure(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", // 仅 P1 配置
		5: "kPa", // 仅 Patm 配置
	}}

	raw, _, ok := BuildRawPressure(values, labels, "dev-partial", provider, "gauge")

	// 修复后：P2-P5 归一化失败 → normalized=false → ok=false
	// 避免插值器拿到 P1=200000Pa + P2-P5=200kPa(原值) 的混合单位输入
	if ok {
		t.Fatalf("expected ok=false (P2-P5 normalization failed, normalized=false), got true")
	}
	// P1 成功换算
	if !floatEq(raw["P1"], 200000) {
		t.Errorf("P1: expect 200000 Pa (converted), got %v", raw["P1"])
	}
	// Patm 成功换算
	if !floatEq(raw["Patm"], 101325) {
		t.Errorf("Patm: expect 101325 Pa (converted), got %v", raw["Patm"])
	}
	// P2-P5 查询失败，保持原值
	for _, label := range []string{"P2", "P3", "P4", "P5"} {
		if !floatEq(raw[label], 200) {
			t.Errorf("%s: expect 200 (raw, unit query failed), got %v", label, raw[label])
		}
	}
}

// TestBuildRawPressure_AbsolutePatmQueryFailure 绝压 + Patm 单位查询失败测试。
//
// 修复目标：验证 Patm 失败时不再产生"假表压"（P1-P5 减 0 等于绝压原值）。
//
// 测试前置：
//   - mock unitProvider 只配置 P1-P5 通道单位，Patm 通道未配置（查询返回 error）
//   - values: P1-P5=200kPa, Patm=101.325kPa, Tatm=25
//   - pressureType="absolute"
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, provider, "absolute")
//
// 期待结果：
//   - P1-P5 跳过归一化（绝压 + Patm 失败，避免假表压）
//   - P1-P5 保持原值 200（kPa，非 Pa）
//   - Patm 保持原值 101.325（查询失败）
//   - ok=false（normalized=false，调用方跳过插值）
func TestBuildRawPressure_AbsolutePatmQueryFailure(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	// 只配置 P1-P5 单位，Patm 不配置（查询失败）
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", 1: "kPa", 2: "kPa", 3: "kPa", 4: "kPa",
	}}

	raw, _, ok := BuildRawPressure(values, labels, "dev-abs-patm-fail", provider, "absolute")

	// 绝压 + Patm 失败 → 跳过 P1-P5 归一化 → normalized=false → ok=false
	if ok {
		t.Fatalf("expected ok=false (absolute + Patm failure, skip P1-P5 to avoid fake gauge), got true")
	}
	// P1-P5 未归一化（避免假表压）
	for _, label := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if !floatEq(raw[label], 200) {
			t.Errorf("%s: expect 200 (raw, normalization skipped), got %v", label, raw[label])
		}
	}
	// Patm 查询失败，保持原值
	if !floatEq(raw["Patm"], 101.325) {
		t.Errorf("Patm: expect 101.325 (raw, query failed), got %v", raw["Patm"])
	}
}

// TestBuildRawPressure_AbsolutePatmConvertFailure 绝压 + Patm 单位换算失败测试。
//
// 修复目标：Patm 单位本身有效（如 degC）但 ConvertToPa 换算失败时，同样不产生假表压。
//
// 测试前置：
//   - mock unitProvider 给 Patm 配置非压力单位 "degC"（ConvertToPa 会返回 error）
//   - P1-P5 配置 kPa，正常换算
//   - pressureType="absolute"
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, provider, "absolute")
//
// 期待结果：
//   - P1-P5 跳过归一化（绝压 + Patm 换算失败）
//   - Patm 保持原值 101.325
//   - ok=false
func TestBuildRawPressure_AbsolutePatmConvertFailure(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	// Patm 配置 degC（非压力单位，ConvertToPa 失败）
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", 1: "kPa", 2: "kPa", 3: "kPa", 4: "kPa",
		5: "degC",
	}}

	raw, _, ok := BuildRawPressure(values, labels, "dev-abs-patm-convert-fail", provider, "absolute")

	if ok {
		t.Fatalf("expected ok=false (Patm convert failed, skip P1-P5), got true")
	}
	// P1-P5 跳过归一化（绝压 + Patm 换算失败）
	for _, label := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if !floatEq(raw[label], 200) {
			t.Errorf("%s: expect 200 (raw, normalization skipped), got %v", label, raw[label])
		}
	}
	if !floatEq(raw["Patm"], 101.325) {
		t.Errorf("Patm: expect 101.325 (raw, convert failed), got %v", raw["Patm"])
	}
}

// TestBuildRawPressure_AbsolutePartialProbeFailure 绝压 + P2-P5 单位查询失败测试。
//
// 修复目标：绝压 + Patm 有效但 P2-P5 失败时，normalized=false，避免部分归一化部分原值。
//
// 测试前置：
//   - mock unitProvider 只配置 P1 + Patm（P2-P5 查询失败）
//   - pressureType="absolute"
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, labels, deviceID, provider, "absolute")
//
// 期待结果：
//   - P1 归一化为表压（200000 - 101325 = 98675 Pa）
//   - P2-P5 保持原值（查询失败）
//   - ok=false（normalized=false，混合单位输入）
func TestBuildRawPressure_AbsolutePartialProbeFailure(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm"}
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", // 仅 P1
		5: "kPa", // 仅 Patm
	}}

	raw, _, ok := BuildRawPressure(values, labels, "dev-abs-partial", provider, "absolute")

	// P2-P5 失败 → normalized=false → ok=false
	if ok {
		t.Fatalf("expected ok=false (P2-P5 normalization failed), got true")
	}
	// P1 成功归一化为表压（绝压 200kPa - 101.325kPa = 98675 Pa）
	if !floatEq(raw["P1"], 98675) {
		t.Errorf("P1: expect 98675 Pa (200000 - 101325, gauge), got %v", raw["P1"])
	}
	// P2-P5 保持原值
	for _, label := range []string{"P2", "P3", "P4", "P5"} {
		if !floatEq(raw[label], 200) {
			t.Errorf("%s: expect 200 (raw, query failed), got %v", label, raw[label])
		}
	}
	// Patm 成功归一化
	if !floatEq(raw["Patm"], 101325) {
		t.Errorf("Patm: expect 101325 Pa (converted), got %v", raw["Patm"])
	}
}

// TestBuildRawPressure_LegacyLabelsSkipped legacy 路径跳过归一化测试。
//
// 修复目标：labels=nil/空时按索引猜标签不可靠，跳过归一化避免基于错误标签换算。
//
// 测试前置：
//   - labels 传 nil（模拟旧调用方未传 labels）
//   - values 按索引顺序填充
//   - unitProvider 配置所有通道 kPa
//
// 测试步骤：
//   - 调用 BuildRawPressure(values, nil, deviceID, provider, "gauge")
//
// 期待结果：
//   - 不 panic
//   - raw 按 legacy 顺序填充 P1..Tatm（保留兼容）
//   - ok=false（legacy 路径 normalized=false，避免基于错误标签换算）
func TestBuildRawPressure_LegacyLabelsSkipped(t *testing.T) {
	values := map[int]float64{0: 200, 1: 200, 2: 200, 3: 200, 4: 200, 5: 101.325, 6: 25}
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", 1: "kPa", 2: "kPa", 3: "kPa", 4: "kPa", 5: "kPa", 6: "degC",
	}}

	raw, _, ok := BuildRawPressure(values, nil, "dev-legacy", provider, "gauge")

	// legacy 路径：normalized=false，避免基于错误标签换算
	if ok {
		t.Fatalf("expected ok=false (legacy path, normalized=false), got true")
	}
	// raw 按 legacy 顺序填充（兼容旧行为）
	if !floatEq(raw["P1"], 200) {
		t.Errorf("P1: expect 200 (raw, legacy no normalization), got %v", raw["P1"])
	}
	if !floatEq(raw["Patm"], 101.325) {
		t.Errorf("Patm: expect 101.325 (raw, legacy no normalization), got %v", raw["Patm"])
	}
}

// TestCheckPreconditions_MissingPatm ChannelMap 校验缺 Patm 测试。
//
// 测试前置：
//   - TraversalManager 实例，config.ChannelLabels 仅包含 Tatm（缺 Patm）
//
// 测试步骤：
//   - 调用 mgr.CheckPreconditions(nil)
//
// 期待结果：
//   - checks 数组包含 name="ChannelMap" 项
//   - ChannelMap 项 passed=false
//   - ChannelMap 项 message 包含 "Patm channel label is required"
//   - allPassed=false
func TestCheckPreconditions_MissingPatm(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.mu.Lock()
	mgr.config.ChannelLabels = map[int]string{0: "P1", 6: "Tatm"} // 缺 Patm
	mgr.mu.Unlock()

	result := mgr.CheckPreconditions(nil)

	checks, _ := result["checks"].([]map[string]any)
	var channelMap map[string]any
	for _, c := range checks {
		if c["name"] == "ChannelMap" {
			channelMap = c
			break
		}
	}
	if channelMap == nil {
		t.Fatalf("expected ChannelMap check item, got nil")
	}
	if channelMap["passed"] != false {
		t.Errorf("ChannelMap passed: expect false (missing Patm), got %v", channelMap["passed"])
	}
	msg, _ := channelMap["message"].(string)
	if !contains(msg, "Patm channel label is required") {
		t.Errorf("ChannelMap message: expect contains 'Patm channel label is required', got %q", msg)
	}
	if result["allPassed"] != false {
		t.Errorf("allPassed: expect false (ChannelMap failed), got %v", result["allPassed"])
	}
}

// TestCheckPreconditions_AllChannelsMapped ChannelMap 校验齐全测试。
//
// 测试前置：
//   - TraversalManager 实例，config.ChannelLabels 包含 P1-P5 + Patm + Tatm 全部 7 标签
//
// 测试步骤：
//   - 调用 mgr.CheckPreconditions(nil)
//
// 期待结果：
//   - ChannelMap 项 passed=true
//   - ChannelMap 项 message 为 "All required channel labels are mapped"
//   - allPassed 不因 ChannelMap 失败（其他项失败不影响 ChannelMap 通过）
func TestCheckPreconditions_AllChannelsMapped(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.mu.Lock()
	mgr.config.ChannelLabels = map[int]string{
		0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "Patm", 6: "Tatm",
	}
	mgr.mu.Unlock()

	result := mgr.CheckPreconditions(nil)

	checks, _ := result["checks"].([]map[string]any)
	var channelMap map[string]any
	for _, c := range checks {
		if c["name"] == "ChannelMap" {
			channelMap = c
			break
		}
	}
	if channelMap == nil {
		t.Fatalf("expected ChannelMap check item, got nil")
	}
	if channelMap["passed"] != true {
		t.Errorf("ChannelMap passed: expect true (all labels mapped), got %v", channelMap["passed"])
	}
	msg, _ := channelMap["message"].(string)
	if msg != "All required channel labels are mapped" {
		t.Errorf("ChannelMap message: expect 'All required channel labels are mapped', got %q", msg)
	}
}

// contains 字符串子串包含辅助（避免引入 strings 包仅为此一处）。
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
