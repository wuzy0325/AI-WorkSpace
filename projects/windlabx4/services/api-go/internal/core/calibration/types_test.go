package calibration

import (
	"testing"
)

func TestConfigValidatesDefaults(t *testing.T) {
	cfg := Config{
		TaskID:         "test-1",
		DeviceID:       "dev-1",
		Channels:       []int{0, 1},
		PressurePoints: []float64{0, 50, 100},
		AverageSamples: 5,
	}

	if cfg.TaskID != "test-1" {
		t.Fatalf("expected test-1, got %s", cfg.TaskID)
	}
	if len(cfg.PressurePoints) != 3 {
		t.Fatalf("expected 3 pressure points, got %d", len(cfg.PressurePoints))
	}
}

func TestStateTransitionsAreValid(t *testing.T) {
	states := map[State]bool{
		StateIdle:    true,
		StateRunning: true,
		StatePaused:  true,
		StateStopped: true,
		StateError:   true,
	}

	for s, ok := range states {
		if !ok {
			t.Fatalf("state %s not in valid set", s)
		}
	}
}

func TestPointResultHasAllFields(t *testing.T) {
	r := PointResult{
		PointIndex:     1,
		TargetPressure: 50.0,
		Timestamp:      123456789,
		Values:         map[int]float64{0: 10.5, 1: 20.3},
	}

	if r.PointIndex != 1 {
		t.Fatalf("expected point index 1, got %d", r.PointIndex)
	}
	if r.TargetPressure != 50.0 {
		t.Fatalf("expected 50, got %f", r.TargetPressure)
	}
	if r.Values[0] != 10.5 {
		t.Fatalf("expected channel 0 value 10.5, got %f", r.Values[0])
	}
}

// ==================== 七孔探针类型测试（Task 1） ====================

// TestSevenHoleTypeConstant 验证 TypeSevenHole 常量定义正确
func TestSevenHoleTypeConstant(t *testing.T) {
	if TypeSevenHole != "seven-hole" {
		t.Fatalf("expected TypeSevenHole='seven-hole', got %q", TypeSevenHole)
	}
	// 确保七孔常量与其他类型不重复
	if TypeSevenHole == TypeFiveHole || TypeSevenHole == TypeThreeHole ||
		TypeSevenHole == TypeTotalPressure || TypeSevenHole == TypeTotalTemperature {
		t.Fatalf("TypeSevenHole 与既有类型重复: %q", TypeSevenHole)
	}
}

// TestSevenHoleRawDataFields 验证 SevenHoleRawData 结构包含 P1~P7 + PAtm/TAtm/PTotal/PStatic/TTunnel 字段
// PTotal/PStatic/TTunnel 为指针类型，缺失时为 nil
// JSON omitempty 行为的断言见 adapters/storage/calibration_json_test.go（core/ 不允许 JSON I/O）
func TestSevenHoleRawDataFields(t *testing.T) {
	pTotal := 4073.07
	pStatic := -32.7
	tTunnel := 25.0
	raw := SevenHoleRawData{
		P1:      1192.6,
		P2:      895.2,
		P3:      922.0,
		P4:      1188.6,
		P5:      1090.4,
		P6:      941.7,
		P7:      4075.35,
		PAtm:    98880.0,
		TAtm:    25.0,
		PTotal:  &pTotal,
		PStatic: &pStatic,
		TTunnel: &tTunnel,
	}

	if raw.P1 != 1192.6 || raw.P7 != 4075.35 {
		t.Fatalf("P1/P7 字段值异常: P1=%f P7=%f", raw.P1, raw.P7)
	}
	if raw.PTotal == nil || *raw.PTotal != pTotal {
		t.Fatalf("PTotal 指针字段异常: %v", raw.PTotal)
	}
}

// TestSevenHoleCoefficientsFields 验证 SevenHoleCoefficients 同时含内区与外区系数字段
// 内区：Kalpha/Kbeta/K0/Ks；外区：Ktheta/Kphi/K0Outer/KsOuter（带扇区编号 n 语义，但结构上是单值字段，扇区编号由 Sector 字段携带）
// 实时气动参数：MachNumber/Velocity 指针，缺失时为 nil
// JSON omitempty 行为的断言见 adapters/storage/calibration_json_test.go（core/ 不允许 JSON I/O）
func TestSevenHoleCoefficientsFields(t *testing.T) {
	ma := 0.242
	v := 85.0
	coeffs := SevenHoleCoefficients{
		// 内区
		Kalpha: 0.043,
		Kbeta:  -0.025,
		K0:     0.00056,
		Ks:     -0.110,
		// 外区
		Ktheta:  0.494,
		Kphi:    1.741,
		K0Outer: -0.207,
		KsOuter: -0.260,
		// 实时气动参数
		MachNumber: &ma,
		Velocity:   &v,
	}

	if coeffs.Kalpha != 0.043 || coeffs.K0 != 0.00056 {
		t.Fatalf("内区系数字段异常: Kalpha=%f K0=%f", coeffs.Kalpha, coeffs.K0)
	}
	if coeffs.Ktheta != 0.494 || coeffs.KsOuter != -0.260 {
		t.Fatalf("外区系数字段异常: Ktheta=%f KsOuter=%f", coeffs.Ktheta, coeffs.KsOuter)
	}
	if coeffs.MachNumber == nil || *coeffs.MachNumber != ma {
		t.Fatalf("MachNumber 指针字段异常: %v", coeffs.MachNumber)
	}
}

// TestSevenHoleDataPointImplementsInterface 验证 SevenHoleDataPoint 实现 DataPoint 接口
// 同时验证 GetPointID/GetCoordinates 返回正确值
func TestSevenHoleDataPointImplementsInterface(t *testing.T) {
	var _ DataPoint = (*SevenHoleDataPoint)(nil)

	dp := &SevenHoleDataPoint{
		PointID:     42,
		Coordinates: map[string]float64{"α": 5.0, "β": 10.0},
	}
	if dp.GetPointID() != 42 {
		t.Fatalf("GetPointID 应返回 42，实际 %d", dp.GetPointID())
	}
	coords := dp.GetCoordinates()
	if coords["α"] != 5.0 || coords["β"] != 10.0 {
		t.Fatalf("GetCoordinates 异常: %v", coords)
	}
}
