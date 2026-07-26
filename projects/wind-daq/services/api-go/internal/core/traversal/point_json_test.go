package traversal

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// TestPointMarshalJSON_NaNToNull 验证 Point.MarshalJSON 把 NaN 字段序列化为 null。
//
// 测试前置：
//   - 构造 Point{X:1, Y:NaN, Z:NaN, U:NaN}（line 模式 markAxesNaN 的典型产出）
//
// 测试步骤：
//   - json.Marshal(p)
//
// 期待结果：
//   - 不返回 error（无 "unsupported value: NaN"）
//   - 输出 JSON 中 X 为 1，Y/Z/U 为 null
func TestPointMarshalJSON_NaNToNull(t *testing.T) {
	p := Point{X: 1, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal Point with NaN: %v", err)
	}
	got := string(data)
	// X 必须是数字 1，Y/Z/U 必须是 null
	if !strings.Contains(got, `"x":1`) {
		t.Errorf("expected x=1 in %s", got)
	}
	if !strings.Contains(got, `"y":null`) {
		t.Errorf("expected y=null in %s", got)
	}
	if !strings.Contains(got, `"z":null`) {
		t.Errorf("expected z=null in %s", got)
	}
	if !strings.Contains(got, `"u":null`) {
		t.Errorf("expected u=null in %s", got)
	}
}

// TestPointMarshalJSON_NormalNumbers 验证非 NaN 数字正常序列化。
//
// 测试前置：
//   - 构造 Point{X:1.5, Y:-2, Z:0, U:100}
//
// 测试步骤：
//   - json.Marshal(p)
//
// 期待结果：
//   - 4 个字段都按数字输出，无 null
func TestPointMarshalJSON_NormalNumbers(t *testing.T) {
	p := Point{X: 1.5, Y: -2, Z: 0, U: 100}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal Point: %v", err)
	}
	got := string(data)
	for _, want := range []string{`"x":1.5`, `"y":-2`, `"z":0`, `"u":100`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in %s", want, got)
		}
	}
}

// TestPointUnmarshalJSON_NullToNaN 验证 Point.UnmarshalJSON 把显式 null 还原为 NaN。
//
// 测试前置：
//   - JSON 字符串 `{"x":1,"y":null,"z":null,"u":null}`
//
// 测试步骤：
//   - json.Unmarshal(data, &p)
//
// 期待结果：
//   - p.X = 1
//   - p.Y/Z/U 为 NaN（运动恢复语义依据）
func TestPointUnmarshalJSON_NullToNaN(t *testing.T) {
	data := []byte(`{"x":1,"y":null,"z":null,"u":null}`)
	var p Point
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.X != 1 {
		t.Errorf("X: got %v, want 1", p.X)
	}
	if !math.IsNaN(p.Y) {
		t.Errorf("Y: got %v, want NaN (from null)", p.Y)
	}
	if !math.IsNaN(p.Z) {
		t.Errorf("Z: got %v, want NaN (from null)", p.Z)
	}
	if !math.IsNaN(p.U) {
		t.Errorf("U: got %v, want NaN (from null)", p.U)
	}
}

// TestPointUnmarshalJSON_MissingKeyToZero 验证缺字段还原为 0（向后兼容旧 checkpoint）。
//
// 测试前置：
//   - JSON 字符串 `{"x":1,"y":0}`（无 z/u 字段，模拟旧版本 checkpoint）
//
// 测试步骤：
//   - json.Unmarshal(data, &p)
//
// 期待结果：
//   - p.X = 1, p.Y = 0
//   - p.Z = 0, p.U = 0（缺字段 → 0，不是 NaN；types.go Point 注释要求）
func TestPointUnmarshalJSON_MissingKeyToZero(t *testing.T) {
	data := []byte(`{"x":1,"y":0}`)
	var p Point
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.X != 1 {
		t.Errorf("X: got %v, want 1", p.X)
	}
	if p.Y != 0 {
		t.Errorf("Y: got %v, want 0", p.Y)
	}
	if p.Z != 0 {
		t.Errorf("Z: got %v, want 0 (missing key → 0)", p.Z)
	}
	if p.U != 0 {
		t.Errorf("U: got %v, want 0 (missing key → 0)", p.U)
	}
}

// TestPointUnmarshalJSON_ExplicitZero 验证显式 0 还原为 0（不是 NaN）。
//
// 测试前置：
//   - JSON 字符串 `{"x":0,"y":0,"z":0,"u":0}`
//
// 测试步骤：
//   - json.Unmarshal(data, &p)
//
// 期待结果：
//   - 4 个字段都是 0（显式数字 0，与 null 不同）
func TestPointUnmarshalJSON_ExplicitZero(t *testing.T) {
	data := []byte(`{"x":0,"y":0,"z":0,"u":0}`)
	var p Point
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.X != 0 || p.Y != 0 || p.Z != 0 || p.U != 0 {
		t.Errorf("expected all 0, got X=%v Y=%v Z=%v U=%v", p.X, p.Y, p.Z, p.U)
	}
}

// TestPointRoundTrip_NaNPreserved 验证 NaN 经 Marshal→Unmarshal 往返后严格保留。
//
// 这是运动恢复的关键契约：Config.Path 中的 NaN 经 checkpoint 序列化→反序列化后，
// 仍能被 availableAxisTargets 通过 math.IsNaN 识别并跳过对应轴。
//
// 测试前置：
//   - 构造 Point{X:1.5, Y:NaN, Z:NaN, U:NaN}
//
// 测试步骤：
//   - json.Marshal(p) → data
//   - json.Unmarshal(data, &p2)
//
// 期待结果：
//   - p2.X = 1.5
//   - p2.Y/Z/U = NaN（往返一致）
func TestPointRoundTrip_NaNPreserved(t *testing.T) {
	p1 := Point{X: 1.5, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}
	data, err := json.Marshal(p1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var p2 Point
	if err := json.Unmarshal(data, &p2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p2.X != 1.5 {
		t.Errorf("X: got %v, want 1.5", p2.X)
	}
	if !math.IsNaN(p2.Y) {
		t.Errorf("Y: got %v, want NaN (round-trip)", p2.Y)
	}
	if !math.IsNaN(p2.Z) {
		t.Errorf("Z: got %v, want NaN (round-trip)", p2.Z)
	}
	if !math.IsNaN(p2.U) {
		t.Errorf("U: got %v, want NaN (round-trip)", p2.U)
	}
}

// TestPointRoundTrip_InNestedStruct 验证 Point 在嵌套结构（如 Config.Path）中
// 也能正确序列化 NaN 为 null。
//
// 测试前置：
//   - 构造 Config{Path: []Point{{X:1, Y:NaN, Z:NaN, U:NaN}}}
//
// 测试步骤：
//   - json.Marshal(config) → data
//   - 验证 data 含 null
//   - json.Unmarshal(data, &config2)
//   - 验证 config2.Path[0].Y/Z/U = NaN
//
// 期待结果：
//   - 嵌套场景下 Point.MarshalJSON 仍生效
//   - 反序列化还原 NaN
func TestPointRoundTrip_InNestedStruct(t *testing.T) {
	config := Config{
		TaskID: "task-nested",
		Path:   []Point{{X: 1, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal Config with NaN Point: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"y":null`) {
		t.Errorf("expected y=null in nested Config JSON: %s", got)
	}

	var config2 Config
	if err := json.Unmarshal(data, &config2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(config2.Path) != 1 {
		t.Fatalf("Path length: got %d, want 1", len(config2.Path))
	}
	p := config2.Path[0]
	if p.X != 1 {
		t.Errorf("X: got %v, want 1", p.X)
	}
	if !math.IsNaN(p.Y) || !math.IsNaN(p.Z) || !math.IsNaN(p.U) {
		t.Errorf("expected Y/Z/U = NaN after round-trip, got Y=%v Z=%v U=%v",
			p.Y, p.Z, p.U)
	}
}
