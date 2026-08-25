package calibration

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/traversal"
)

// positionAwareRuntime 实现 PositionReader 的测试运行时：
// 记录每次 MoveToPosition 的目标，并模拟运动到位后轴位置等于目标。
type positionAwareRuntime struct {
	current map[string]float64
	moves   []string
	values  map[string]float64
}

func (r *positionAwareRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	v, ok := r.values[fmt.Sprintf("%s:%d", deviceID, channelIndex)]
	return v, ok
}
func (r *positionAwareRuntime) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }
func (r *positionAwareRuntime) IsAcquiring(_ string) bool                 { return true }
func (r *positionAwareRuntime) GetAxisPosition(axis MotionAxisConfig) (float64, error) {
	if v, ok := r.current[axis.Name]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("轴 %s 无当前位置", axis.Name)
}
func (r *positionAwareRuntime) MoveToPosition(axis MotionAxisConfig, position float64) error {
	r.moves = append(r.moves, fmt.Sprintf("%s=%g", axis.Name, position))
	r.current[axis.Name] = position
	return nil
}
func (r *positionAwareRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	return true, traversal.MotionInterruptNone, nil
}
func (r *positionAwareRuntime) StopMotion() error { return nil }

// TestResolveTargetPositionAbsoluteIsDefault 验证默认（absolute）模式原样返回测点坐标。
func TestResolveTargetPositionAbsoluteIsDefault(t *testing.T) {
	engine := NewAutomaticCalibration(Config{}, nil, &positionAwareRuntime{current: map[string]float64{"α": 100}}, nil, nil)
	got, err := engine.resolveTargetPosition(MotionAxisConfig{Name: "α"}, -20)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got != -20 {
		t.Fatalf("expected absolute target -20, got %v", got)
	}
}

// TestResolveTargetPositionAbsoluteExplicit 验证显式 absolute 模式行为与默认一致。
func TestResolveTargetPositionAbsoluteExplicit(t *testing.T) {
	engine := NewAutomaticCalibration(Config{CoordinateMode: CoordinateModeAbsolute}, nil, &positionAwareRuntime{current: map[string]float64{"α": 100}}, nil, nil)
	got, err := engine.resolveTargetPosition(MotionAxisConfig{Name: "α"}, -20)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got != -20 {
		t.Fatalf("expected absolute target -20, got %v", got)
	}
}

// TestResolveTargetPositionRelative 验证相对坐标模式下目标 = 当前坐标 + 测点坐标。
func TestResolveTargetPositionRelative(t *testing.T) {
	engine := NewAutomaticCalibration(
		Config{CoordinateMode: CoordinateModeRelative},
		nil,
		&positionAwareRuntime{current: map[string]float64{"α": 100}},
		nil,
		nil,
	)
	got, err := engine.resolveTargetPosition(MotionAxisConfig{Name: "α"}, -20)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got != 80 {
		t.Fatalf("expected relative target 80 (100 + -20), got %v", got)
	}
}

// TestResolveTargetPositionRelativeRequiresPositionReader 验证相对坐标模式
// 下运行时未实现 PositionReader 时返回明确错误（不静默降级为绝对坐标）。
func TestResolveTargetPositionRelativeRequiresPositionReader(t *testing.T) {
	// fakeCalibrationRuntime 不实现 PositionReader
	engine := NewAutomaticCalibration(Config{CoordinateMode: CoordinateModeRelative}, nil, &fakeCalibrationRuntime{}, nil, nil)
	_, err := engine.resolveTargetPosition(MotionAxisConfig{Name: "α"}, -20)
	if err == nil {
		t.Fatal("expected error for runtime without PositionReader")
	}
	if !strings.Contains(err.Error(), "PositionReader") {
		t.Fatalf("expected error mentioning PositionReader, got %v", err)
	}
}

// TestAutomaticCalibrationRelativeModeAccumulates 验证相对坐标模式下完整校准循环：
// 每个点的坐标值作为相对当前位置的位移量，连续点位依次累积。
func TestAutomaticCalibrationRelativeModeAccumulates(t *testing.T) {
	config := completeFiveHoleConfig()
	config.CoordinateMode = CoordinateModeRelative
	config.MotionAxes = []MotionAxisConfig{
		{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		{Name: "α", ControllerID: "motion-1", Axis: "X"},
	}
	config.Points = []CalPoint{
		{ID: 1, Coordinates: map[string]float64{"β": 5, "α": -20}},
		{ID: 2, Coordinates: map[string]float64{"β": -5, "α": 10}},
	}
	runtime := &positionAwareRuntime{
		current: map[string]float64{"α": 100, "β": -30},
		values:  completeFiveHoleValues(),
	}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	// 五孔按 α→β 顺序移动（MoveToPointWithOrder）：
	// 点1：α=-20 → 100+(-20)=80；β=5 → -30+5=-25
	// 点2：α=10 → 80+10=90；β=-5 → -25+(-5)=-30
	expected := []string{"α=80", "β=-25", "α=90", "β=-30"}
	if !reflect.DeepEqual(runtime.moves, expected) {
		t.Fatalf("expected relative moves %v, got %v", expected, runtime.moves)
	}
}

// TestAutomaticCalibrationAbsoluteModeIsDefault 验证默认绝对坐标行为：
// 测点坐标即目标绝对位置，与当前位置无关。
func TestAutomaticCalibrationAbsoluteModeIsDefault(t *testing.T) {
	config := completeFiveHoleConfig()
	config.MotionAxes = []MotionAxisConfig{
		{Name: "β", ControllerID: "motion-1", Axis: "Y"},
		{Name: "α", ControllerID: "motion-1", Axis: "X"},
	}
	config.Points = []CalPoint{
		{ID: 1, Coordinates: map[string]float64{"β": 5, "α": -20}},
	}
	runtime := &positionAwareRuntime{
		current: map[string]float64{"α": 100, "β": -30},
		values:  completeFiveHoleValues(),
	}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	expected := []string{"α=-20", "β=5"}
	if !reflect.DeepEqual(runtime.moves, expected) {
		t.Fatalf("expected absolute moves %v, got %v", expected, runtime.moves)
	}
}
