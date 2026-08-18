package usecase

import (
	"math"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/motion"
	"windlabx4/services/api-go/internal/core/traversal"
)

func TestAvailableAxisTargets_FiltersByControllerID(t *testing.T) {
	sim := motion.ControllerStatus{
		ID:        "sim-motion-1",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0},
			{Name: motion.AxisY, Position: 0},
			{Name: motion.AxisZ, Position: 0},
		},
	}
	real := motion.ControllerStatus{
		ID:        "wtn-mc-1",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0},
			{Name: motion.AxisY, Position: 0},
			{Name: motion.AxisZ, Position: 0},
		},
	}
	point := traversal.Point{X: -30, Y: 0, Z: math.NaN(), U: math.NaN()}
	bindings := []traversal.MotionAxisBinding{
		{ControllerID: "sim-motion-1", Axis: "X"},
		{ControllerID: "sim-motion-1", Axis: "Y"},
	}

	simTargets := availableAxisTargets(sim, point, bindings)
	if len(simTargets) != 2 {
		t.Fatalf("sim targets len=%d want 2, got %#v", len(simTargets), simTargets)
	}
	if simTargets[motion.AxisX] != -30 || simTargets[motion.AxisY] != 0 {
		t.Fatalf("unexpected sim targets: %#v", simTargets)
	}

	realTargets := availableAxisTargets(real, point, bindings)
	if len(realTargets) != 0 {
		t.Fatalf("real controller should be skipped, got %#v", realTargets)
	}
}

// 测试前置：构造 point/bindings，使用 nil cfg 触发默认 0.2 容差（与生产回退路径一致）
// 测试步骤：分别传入空状态列表、断开连接的控制器、缺少绑定轴的控制器
// 期待结果：三种情况下 (allReached, checkedTargets) 均为 (false, 0)，避免状态缺失时提前进入稳定阶段
func TestMotionTargetsReachedWithTolerance_RequiresAtLeastOneConnectedTarget(t *testing.T) {
	point := traversal.Point{X: 10}
	bindings := []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}}

	if allReached, checked := motionTargetsReachedWithTolerance(nil, point, bindings, nil); allReached || checked != 0 {
		t.Fatalf("空状态列表不能判定运动完成: allReached=%v checked=%d", allReached, checked)
	}
	if allReached, checked := motionTargetsReachedWithTolerance([]motion.ControllerStatus{{
		ID:        "mc-1",
		Connected: false,
		Axes:      []motion.AxisStatus{{Name: motion.AxisX, Position: 10}},
	}}, point, bindings, nil); allReached || checked != 0 {
		t.Fatalf("断开连接的目标轴不能判定运动完成: allReached=%v checked=%d", allReached, checked)
	}
	if allReached, checked := motionTargetsReachedWithTolerance([]motion.ControllerStatus{{
		ID:        "mc-1",
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisY, Position: 10}},
	}}, point, bindings, nil); allReached || checked != 0 {
		t.Fatalf("缺少绑定轴时不能判定运动完成: allReached=%v checked=%d", allReached, checked)
	}
}

// 测试前置：构造 axis.Moving=true 与 axis.Moving=false 两种状态，位置与目标一致（偏差 0）
// 测试步骤：先测 Moving=true 应判未到位，再测 Moving=false 且偏差 0 应判到位
// 期待结果：Moving=true → allReached=false；Moving=false 且 |position-target|=0 ≤ 默认容差 → allReached=true
func TestMotionTargetsReachedWithTolerance_RejectsMovingAxisAndAcceptsReachedAxis(t *testing.T) {
	point := traversal.Point{X: 10}
	bindings := []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}}
	status := motion.ControllerStatus{
		ID:        "mc-1",
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisX, Position: 10, Moving: true}},
	}
	if allReached, _ := motionTargetsReachedWithTolerance([]motion.ControllerStatus{status}, point, bindings, nil); allReached {
		t.Fatal("位置更新中的轴不能判定运动完成")
	}
	status.Axes[0].Moving = false
	if allReached, _ := motionTargetsReachedWithTolerance([]motion.ControllerStatus{status}, point, bindings, nil); !allReached {
		t.Fatal("已停止且到达目标的轴应判定运动完成")
	}
}

func TestAvailableAxisTargets_EmptyBindingsKeepsLegacyAllAxes(t *testing.T) {
	status := motion.ControllerStatus{
		ID: "any",
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX},
			{Name: motion.AxisY},
		},
	}
	point := traversal.Point{X: 1, Y: 2}
	targets := availableAxisTargets(status, point, nil)
	if len(targets) != 2 {
		t.Fatalf("legacy empty bindings should target all axes, got %#v", targets)
	}
}

func TestAvailableAxisTargets_EmptyControllerIDMatchesAnyController(t *testing.T) {
	status := motion.ControllerStatus{
		ID: "wtn-mc-1",
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX},
			{Name: motion.AxisY},
		},
	}
	point := traversal.Point{X: 5, Y: 6}
	bindings := []traversal.MotionAxisBinding{
		{ControllerID: "", Axis: "X"},
	}
	targets := availableAxisTargets(status, point, bindings)
	if len(targets) != 1 || targets[motion.AxisX] != 5 {
		t.Fatalf("empty controllerId should match any controller X axis, got %#v", targets)
	}
}

func TestAvailableAxisTargets_MapsLogicalTargetsToSelectedPhysicalAxes(t *testing.T) {
	status := motion.ControllerStatus{
		ID: "mc-1",
		Axes: []motion.AxisStatus{
			{Name: motion.AxisZ},
			{Name: motion.AxisU},
		},
	}
	point := traversal.Point{X: 25, Y: 45, Z: math.NaN(), U: math.NaN()}
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", ControllerID: "mc-1", Axis: "Z"},
		{Name: "Y", ControllerID: "mc-1", Axis: "U"},
	}

	targets := availableAxisTargets(status, point, bindings)
	if len(targets) != 2 || targets[motion.AxisZ] != 25 || targets[motion.AxisU] != 45 {
		t.Fatalf("logical X/Y targets should map to physical Z/U axes, got %#v", targets)
	}
}

func TestMotionAxesForPath_RectangleSkipsUnusedAxes(t *testing.T) {
	path := []traversal.Point{
		{X: -30, Y: -30, Z: math.NaN(), U: math.NaN()},
		{X: -25, Y: -30, Z: math.NaN(), U: math.NaN()},
	}
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", ControllerID: "xy-controller", Axis: "X"},
		{Name: "Y", ControllerID: "xy-controller", Axis: "Y"},
		{Name: "Z", ControllerID: "z-controller", Axis: "Z"},
		{Name: "U", ControllerID: "u-controller", Axis: "U"},
	}

	got := motionAxesForPath(bindings, path)
	if len(got) != 2 || got[0].Name != "X" || got[1].Name != "Y" {
		t.Fatalf("rectangle path should retain only X/Y bindings, got %#v", got)
	}
}

func TestMotionAxesForPath_RectangleKeepsSelectedPhysicalAxes(t *testing.T) {
	path := []traversal.Point{{X: 25, Y: 45, Z: math.NaN(), U: math.NaN()}}
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", ControllerID: "mc-1", Axis: "Z"},
		{Name: "Y", ControllerID: "mc-1", Axis: "U"},
		{Name: "Z", ControllerID: "mc-2", Axis: "X"},
		{Name: "U", ControllerID: "mc-2", Axis: "Y"},
	}

	activeBindings := motionAxesForPath(bindings, path)
	if len(activeBindings) != 2 || activeBindings[0].Axis != "Z" || activeBindings[1].Axis != "U" {
		t.Fatalf("rectangle logical X/Y should retain their selected physical Z/U axes, got %#v", activeBindings)
	}

	status := motion.ControllerStatus{
		ID:   "mc-1",
		Axes: []motion.AxisStatus{{Name: motion.AxisZ}, {Name: motion.AxisU}},
	}
	targets := availableAxisTargets(status, path[0], activeBindings)
	if len(targets) != 2 || targets[motion.AxisZ] != 25 || targets[motion.AxisU] != 45 {
		t.Fatalf("logical X/Y coordinates should target selected physical Z/U axes, got %#v", targets)
	}
}

func TestMotionAxesForPath_CustomKeepsFiniteFourAxes(t *testing.T) {
	path := []traversal.Point{{X: 1, Y: 2, Z: 3, U: 4}}
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", ControllerID: "mc-1", Axis: "X"},
		{Name: "Y", ControllerID: "mc-1", Axis: "Y"},
		{Name: "Z", ControllerID: "mc-1", Axis: "Z"},
		{Name: "U", ControllerID: "mc-1", Axis: "U"},
	}

	got := motionAxesForPath(bindings, path)
	if len(got) != len(bindings) {
		t.Fatalf("custom four-axis path should retain all bindings, got %#v", got)
	}
}

func TestValidateSectorOrigin_RequiresBoundAxesAtZero(t *testing.T) {
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", ControllerID: "mc-1", Axis: "Z"},
		{Name: "Y", ControllerID: "mc-1", Axis: "U"},
	}
	statuses := []motion.ControllerStatus{{
		ID: "mc-1", Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisZ, Position: 0.05},
			{Name: motion.AxisU, Position: 2.5},
		},
	}}

	err := validateSectorOrigin(statuses, bindings, nil)
	if err == nil || !strings.Contains(err.Error(), "U") || !strings.Contains(err.Error(), "2.5") {
		t.Fatalf("expected non-zero U axis error with current position, got %v", err)
	}

	statuses[0].Axes[1].Position = 0.1
	if err := validateSectorOrigin(statuses, bindings, nil); err != nil {
		t.Fatalf("positions within default arrival tolerance should pass: %v", err)
	}
}

func TestValidateSectorOrigin_RejectsMovingAxis(t *testing.T) {
	bindings := []traversal.MotionAxisBinding{{Name: "X", ControllerID: "mc-1", Axis: "X"}}
	statuses := []motion.ControllerStatus{{
		ID: "mc-1", Connected: true,
		Axes: []motion.AxisStatus{{Name: motion.AxisX, Position: 0, Moving: true}},
	}}

	err := validateSectorOrigin(statuses, bindings, nil)
	if err == nil || !strings.Contains(err.Error(), "moving") {
		t.Fatalf("expected moving axis error, got %v", err)
	}
}

// TestValidateSectorOrigin_RejectsNaNPosition 回归测试（B3）：
// 轴位置为 NaN 时 math.Abs(NaN) > tolerance 恒为 false，
// 不显式拦截会让位置反馈缺失的轴静默通过原点校验。
func TestValidateSectorOrigin_RejectsNaNPosition(t *testing.T) {
	bindings := []traversal.MotionAxisBinding{{Name: "X", ControllerID: "mc-1", Axis: "X"}}
	statuses := []motion.ControllerStatus{{
		ID: "mc-1", Connected: true,
		Axes: []motion.AxisStatus{{Name: motion.AxisX, Position: math.NaN()}},
	}}

	err := validateSectorOrigin(statuses, bindings, nil)
	if err == nil || !strings.Contains(err.Error(), "NaN") {
		t.Fatalf("expected NaN position error, got %v", err)
	}
}

func TestValidateSectorOriginRejectsMismatchedController(t *testing.T) {
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", ControllerID: "missing", Axis: "X"},
		{Name: "Y", ControllerID: "missing", Axis: "Y"},
	}
	statuses := []motion.ControllerStatus{{
		ID: "other", Connected: true,
		Axes: []motion.AxisStatus{{Name: motion.AxisX}, {Name: motion.AxisY}},
	}}

	err := validateSectorOrigin(statuses, bindings, nil)
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("mismatched controller must not fall back by axis name, got %v", err)
	}
}

func TestValidateSectorOriginRequiresExplicitController(t *testing.T) {
	bindings := []traversal.MotionAxisBinding{
		{Name: "X", Axis: "X"},
		{Name: "Y", Axis: "Y"},
	}

	err := validateSectorOrigin(nil, bindings, nil)
	if err == nil || !strings.Contains(err.Error(), "explicit controller") {
		t.Fatalf("sector origin must require explicit controller bindings, got %v", err)
	}
}

// 测试前置：模拟"前端用别名 sim-motion-1 但后端 profile.ID 是 UUID"的真实场景
// 测试步骤：构造绑定 controllerId="sim-motion-1"，模拟控制器 ID="f3c70fdd-..."，Name="模拟运动控制器"
// 期待结果：resolveMotionAxes 返回全部 controllerId 被清空的回退版本；
//
//	再用回退后的 motionAxes 调 availableAxisTargets，应对模拟控制器生成 X 轴目标
func TestResolveMotionAxes_FallsBackWhenControllerIDMismatchesAll(t *testing.T) {
	sim := motion.ControllerStatus{
		ID:        "f3c70fdd-0e1c-4d99-86ae-c4c5387e41c2",
		Name:      "模拟运动控制器",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0},
		},
	}
	statuses := []motion.ControllerStatus{sim}
	// 用户实际配置：controllerId 用了前端别名 "sim-motion-1"，既不是 ID 也不是 Name
	bindings := []traversal.MotionAxisBinding{
		{ControllerID: "sim-motion-1", Axis: "X"},
	}

	resolved := resolveMotionAxes(bindings, statuses)
	if len(resolved) != 1 {
		t.Fatalf("resolved len=%d want 1", len(resolved))
	}
	if resolved[0].ControllerID != "" {
		t.Fatalf("controllerId should be cleared on fallback, got %q", resolved[0].ControllerID)
	}
	if resolved[0].Axis != "X" {
		t.Fatalf("axis name should be preserved, got %q", resolved[0].Axis)
	}

	// 验证回退后能给模拟控制器生成目标
	point := traversal.Point{X: -30, Y: 0, Z: 0, U: 0}
	targets := availableAxisTargets(sim, point, resolved)
	if len(targets) != 1 || targets[motion.AxisX] != -30 {
		t.Fatalf("fallback should generate X target for simulated controller, got %#v", targets)
	}
}

// 测试前置：构造匹配场景（controllerId 匹配 status.ID），确保不触发回退
// 测试步骤：bindings[0].ControllerID = sim.ID
// 期待结果：resolveMotionAxes 原样返回，availableAxisTargets 对 sim 生成目标，对 real 返回 nil
func TestResolveMotionAxes_PreservesBindingsWhenMatched(t *testing.T) {
	sim := motion.ControllerStatus{
		ID:        "f3c70fdd-0e1c-4d99-86ae-c4c5387e41c2",
		Name:      "模拟运动控制器",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0},
		},
	}
	real := motion.ControllerStatus{
		ID:        "ab953efa-f84b-47d1-9ff8-f1f22fe4977e",
		Name:      "141",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0},
		},
	}
	statuses := []motion.ControllerStatus{sim, real}
	bindings := []traversal.MotionAxisBinding{
		{ControllerID: "f3c70fdd-0e1c-4d99-86ae-c4c5387e41c2", Axis: "X"},
	}

	resolved := resolveMotionAxes(bindings, statuses)
	if resolved[0].ControllerID != "f3c70fdd-0e1c-4d99-86ae-c4c5387e41c2" {
		t.Fatalf("matched bindings should not be modified, got %q", resolved[0].ControllerID)
	}

	// 模拟控制器应生成目标，真实控制器应返回 nil（保持严格过滤）
	point := traversal.Point{X: -30}
	simTargets := availableAxisTargets(sim, point, resolved)
	if len(simTargets) != 1 || simTargets[motion.AxisX] != -30 {
		t.Fatalf("sim should get X target, got %#v", simTargets)
	}
	realTargets := availableAxisTargets(real, point, resolved)
	if len(realTargets) != 0 {
		t.Fatalf("real controller should be skipped when matched, got %#v", realTargets)
	}
}

// 测试前置：用控制器名称作为 controllerId（前端用语义名称而非 UUID）
// 测试步骤：bindings[0].ControllerID = sim.Name，与 sim.ID 不匹配
// 期待结果：resolveMotionAxes 触发全局回退，清空 controllerId 后按轴名匹配成功
func TestResolveMotionAxes_FallsBackForNameControllerID(t *testing.T) {
	sim := motion.ControllerStatus{
		ID:        "f3c70fdd-0e1c-4d99-86ae-c4c5387e41c2",
		Name:      "模拟运动控制器",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0},
		},
	}
	statuses := []motion.ControllerStatus{sim}
	bindings := []traversal.MotionAxisBinding{
		{ControllerID: "模拟运动控制器", Axis: "X"},
	}

	resolved := resolveMotionAxes(bindings, statuses)
	if resolved[0].ControllerID != "" {
		t.Fatalf("name-as-controllerId should trigger fallback (clear controllerId), got %q", resolved[0].ControllerID)
	}

	point := traversal.Point{X: 12.5}
	targets := availableAxisTargets(sim, point, resolved)
	if len(targets) != 1 || targets[motion.AxisX] != 12.5 {
		t.Fatalf("fallback should generate X target, got %#v", targets)
	}
}
