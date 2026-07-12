package usecase

import (
	"math"
	"testing"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
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

func TestMotionTargetsReached_RequiresAtLeastOneConnectedTarget(t *testing.T) {
	point := traversal.Point{X: 10}
	bindings := []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}}

	if motionTargetsReached(nil, point, bindings) {
		t.Fatal("空状态列表不能判定运动完成")
	}
	if motionTargetsReached([]motion.ControllerStatus{{
		ID:        "mc-1",
		Connected: false,
		Axes:      []motion.AxisStatus{{Name: motion.AxisX, Position: 10}},
	}}, point, bindings) {
		t.Fatal("断开连接的目标轴不能判定运动完成")
	}
	if motionTargetsReached([]motion.ControllerStatus{{
		ID:        "mc-1",
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisY, Position: 10}},
	}}, point, bindings) {
		t.Fatal("缺少绑定轴时不能判定运动完成")
	}
}

func TestMotionTargetsReached_RejectsMovingAxisAndAcceptsReachedAxis(t *testing.T) {
	point := traversal.Point{X: 10}
	bindings := []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}}
	status := motion.ControllerStatus{
		ID:        "mc-1",
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisX, Position: 10, Moving: true}},
	}
	if motionTargetsReached([]motion.ControllerStatus{status}, point, bindings) {
		t.Fatal("位置更新中的轴不能判定运动完成")
	}
	status.Axes[0].Moving = false
	if !motionTargetsReached([]motion.ControllerStatus{status}, point, bindings) {
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
