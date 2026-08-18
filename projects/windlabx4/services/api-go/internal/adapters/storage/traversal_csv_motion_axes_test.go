// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件覆盖 TraversalCsvWriter.buildAxisRowValues 的物理轴映射逻辑，
// 修复 Bug 2（CSV 表头与列数据物理轴错位）后必须保留的回归网：
//   - 空 motionAxes 走旧路径，保证存量任务字节一致
//   - X→Z / Y→U 重映射：物理 Z/U 列承载逻辑 X/Y 方向值，物理 X/Y 列留空
//   - "绑定为 0" 与 "未绑定" 区分：前者输出 "0.000000"，后者输出空字符串
//   - NaN 输出空字符串：line/rectangle/sector 模式 markAxesNaN 后预期表现
//   - Name/Axis 大小写防御：用户 JSON 直传小写也能正确映射
package storage

import (
	"math"
	"testing"

	"windlabx4/services/api-go/internal/core/traversal"
)

// applyMotionAxesForTest 直接调用 applyConfigLocked 配置 motionAxes，
// 避免每个用例都走 Open 创建文件，专注测试 buildAxisRowValues 的纯映射逻辑。
func applyMotionAxesForTest(t *testing.T, w *TraversalCsvWriter, motionAxes []traversal.MotionAxisBinding) {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.applyConfigLocked(&traversal.SaveOptions{
		SavePointId:          true,
		SaveTimestamp:        true,
		SaveRawPressure:      true,
		SaveCalculatedResult: true,
	}, nil, nil, motionAxes)
}

// assertAxisRow 校验 buildAxisRowValues 输出与期望切片完全一致，
// 失败时打印 motionAxes / Point / 期望 / 实际，便于定位错位列。
func assertAxisRow(t *testing.T, w *TraversalCsvWriter, p traversal.Point, want []string) {
	t.Helper()
	got := w.buildAxisRowValues(p)
	if len(got) != len(want) {
		t.Fatalf("axis row length: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("axis row[%d] mismatch: got %q, want %q (full got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

// TestBuildAxisRowValuesEmptyMotionAxesPreservesLegacyBehavior 验证旧配置兼容路径：
// motionAxes 为空时按 Point.X/Y/Z/U 字段顺序直接输出，与 Bug 2 修复前的行为字节一致。
// 这是存量任务的回归保护网，确保未升级 motionAxes 配置的实验数据格式不变。
func TestBuildAxisRowValuesEmptyMotionAxesPreservesLegacyBehavior(t *testing.T) {
	w := NewTraversalCsvWriter()
	applyMotionAxesForTest(t, w, nil)

	// 全 0 + 全 NaN 的极端 Point，验证旧路径不做任何过滤
	p := traversal.Point{X: 0, Y: 0, Z: 0, U: 0}
	assertAxisRow(t, w, p, []string{"0.000000", "0.000000", "0.000000", "0.000000"})

	p = traversal.Point{
		X: 10.5,
		Y: math.NaN(),
		Z: -3.2,
		U: math.NaN(),
	}
	// 旧路径下 Y/U 列由 formatFloat 转 NaN 为空字符串
	assertAxisRow(t, w, p, []string{"10.500000", "", "-3.200000", ""})
}

// TestBuildAxisRowValuesRemapsLogicalToPhysicalAxes 验证核心修复场景：
// motionAxes=[{X→Z},{Y→U}] 时，逻辑 X 方向值应写入物理 Z 列，
// 逻辑 Y 方向值应写入物理 U 列，物理 X/Y 列因未绑定输出空字符串。
// 这是用户反馈 Bug 2 的直接复现用例。
func TestBuildAxisRowValuesRemapsLogicalToPhysicalAxes(t *testing.T) {
	w := NewTraversalCsvWriter()
	applyMotionAxesForTest(t, w, []traversal.MotionAxisBinding{
		{Name: "X", Axis: "Z"},
		{Name: "Y", Axis: "U"},
	})

	// line 模式典型场景：Y/Z/U 被 markAxesNaN，实际只有逻辑 X 有值
	p := traversal.Point{
		X: 10.5,
		Y: math.NaN(),
		Z: math.NaN(),
		U: math.NaN(),
	}
	// 修复前：["10.500000","","",""]（数据错位写到 X 列，物理 Z 列空）
	// 修复后：["","","10.500000",""]（数据正确写到物理 Z 列，物理 X 列空）
	assertAxisRow(t, w, p, []string{"", "", "10.500000", ""})
}

// TestBuildAxisRowValuesDistinguishesBoundZeroFromUnbound 验证"绑定为 0"与
// "未绑定"的区分：前者输出 "0.000000"，后者输出空字符串。
// 这是用户审查时关注的边界——若用零值默认值替代 map 的 ok 模式，未绑定列会
// 错误地输出 "0.000000"，污染 CSV 数据让分析者误以为该轴有真实测量值。
func TestBuildAxisRowValuesDistinguishesBoundZeroFromUnbound(t *testing.T) {
	w := NewTraversalCsvWriter()
	// 仅绑定 X→X，物理 Y/Z/U 列均未绑定
	applyMotionAxesForTest(t, w, []traversal.MotionAxisBinding{
		{Name: "X", Axis: "X"},
	})

	p := traversal.Point{X: 0, Y: 5, Z: 7, U: 9}
	// 物理 X 列：逻辑 X=0 已绑定 → "0.000000"
	// 物理 Y/Z/U 列：未绑定 → 空字符串（即使 Point.Y/Z/U 有值也不输出）
	assertAxisRow(t, w, p, []string{"0.000000", "", "", ""})
}

// TestBuildAxisRowValuesHandlesLowercaseAxisNameDefensively 验证大小写防御：
// 用户绕过前端直接构造 JSON 时可能传小写 axis/name，后端应规范化为大写匹配。
// 缺少此防御会导致小写绑定的值静默丢失（map key 大小写敏感）。
func TestBuildAxisRowValuesHandlesLowercaseAxisNameDefensively(t *testing.T) {
	w := NewTraversalCsvWriter()
	applyMotionAxesForTest(t, w, []traversal.MotionAxisBinding{
		{Name: "x", Axis: "z"}, // 小写输入
		{Name: "y", Axis: "u"},
	})

	p := traversal.Point{X: 10, Y: 5, Z: math.NaN(), U: math.NaN()}
	// 大小写规范化后等价于 [{X→Z},{Y→U}]，物理 Z/U 列承载逻辑 X/Y 值
	assertAxisRow(t, w, p, []string{"", "", "10.000000", "5.000000"})
}

// TestBuildAxisRowValuesSkipsIncompleteBindings 验证 Name 或 Axis 为空的
// 绑定条目被跳过：旧数据可能存在 Name 为空的脏绑定，不应让整个 writer 崩溃。
func TestBuildAxisRowValuesSkipsIncompleteBindings(t *testing.T) {
	w := NewTraversalCsvWriter()
	applyMotionAxesForTest(t, w, []traversal.MotionAxisBinding{
		{Name: "", Axis: "Z"},  // Name 缺失 → 跳过
		{Name: "X", Axis: ""},  // Axis 缺失 → 跳过
		{Name: "X", Axis: "X"}, // 合法绑定
	})

	p := traversal.Point{X: 10, Y: 5, Z: 7, U: 9}
	// 只有第三个绑定生效，物理 X 列写 10，其余未绑定列空
	assertAxisRow(t, w, p, []string{"10.000000", "", "", ""})
}

// TestBuildAxisRowValuesAllFourAxesBound 验证 4 轴全绑定场景：
// 前端目前 name 仅允许 'X'|'Y'，但后端 MotionAxisBinding.Name 是 string，
// 防御性测试 4 轴全绑定（如未来 custom 模式扩展 Z/U 方向）能正确映射。
func TestBuildAxisRowValuesAllFourAxesBound(t *testing.T) {
	w := NewTraversalCsvWriter()
	applyMotionAxesForTest(t, w, []traversal.MotionAxisBinding{
		{Name: "X", Axis: "X"},
		{Name: "Y", Axis: "Y"},
		{Name: "Z", Axis: "Z"},
		{Name: "U", Axis: "U"},
	})

	p := traversal.Point{X: 1, Y: 2, Z: 3, U: 4}
	assertAxisRow(t, w, p, []string{"1.000000", "2.000000", "3.000000", "4.000000"})
}
