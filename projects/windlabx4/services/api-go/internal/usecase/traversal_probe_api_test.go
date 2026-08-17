package usecase

import (
	"encoding/json"
	"errors"
	"testing"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"windlabx4/services/api-go/internal/core/traversal"
)

// TestCalculateRealtimeForAPI_FiveHoleLegacy 五孔默认路径：probeType 空 → 走五孔，
// 返回 coreinterp.InterpolationResult（含完整 V/Vx/Vy/Vz/CAS/SAT/Density 字段）。
// 字段 JSON 形状与既有 API 响应逐字节一致。
func TestCalculateRealtimeForAPI_FiveHoleLegacy(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{} // legacy empty probeType -> five-hole
	mgr.SetInterpolator(&mockInterpolator{})

	res, err := mgr.CalculateRealtimeForAPI("", ProbePressureInput{
		P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, PAtm: 101325, TAtm: 20,
	})
	if err != nil {
		t.Fatalf("CalculateRealtimeForAPI: %v", err)
	}
	// 返回值类型必须是 coreinterp.InterpolationResult（保持 JSON 字段完整），
	// 不能降级为标量子集——否则五孔 API 响应字段会丢失。
	// mockInterpolator.Calculate 返回零值（IsValid=false），所以这里只断言类型。
	fh, ok := res.(coreinterp.InterpolationResult)
	if !ok {
		t.Fatalf("five-hole response type = %T, want coreinterp.InterpolationResult", res)
	}
	// 序列化字段必须包含 V/Vx/Vy/Vz 等五孔扩展字段（spec §5.6 兼容）。
	raw, _ := json.Marshal(fh)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"alpha", "beta", "machNumber", "V", "Vx", "Vy", "Vz", "velocity", "cas", "sat", "dynamicPressure", "density", "P0", "Ps", "isValid"} {
		if _, exists := decoded[key]; !exists {
			t.Errorf("five-hole JSON missing field %q", key)
		}
	}
}

// TestCalculateRealtimeForAPI_SevenHole 七孔路径：返回 seveninterp.InterpolationResult，
// P6/P7 显式提供（含 0），响应字段与既有七孔 API 一致。
func TestCalculateRealtimeForAPI_SevenHole(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	seven := &mockSevenInterpolator{
		loaded: true,
		result: seveninterp.InterpolationResult{IsValid: true, Alpha: 1.5, Beta: -2.5, TotalPressure: 1000, StaticPressure: 900, MachNumber: 0.3, Velocity: 100},
	}
	mgr.SetSevenHoleInterpolator(seven)

	p6, p7 := 6.0, 7.0
	res, err := mgr.CalculateRealtimeForAPI(traversal.ProbeTypeSevenHole, ProbePressureInput{
		P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: &p6, P7: &p7, PAtm: 101325, TAtm: 20,
	})
	if err != nil {
		t.Fatalf("CalculateRealtimeForAPI: %v", err)
	}
	sh, ok := res.(seveninterp.InterpolationResult)
	if !ok {
		t.Fatalf("seven-hole response type = %T, want seveninterp.InterpolationResult", res)
	}
	if !sh.IsValid || sh.Alpha != 1.5 || sh.TotalPressure != 1000 {
		t.Errorf("unexpected seven-hole result: %+v", sh)
	}
	if seven.lastIn.P6 != 6 || seven.lastIn.P7 != 7 {
		t.Errorf("P6/P7 not forwarded: P6=%v P7=%v", seven.lastIn.P6, seven.lastIn.P7)
	}
}

// TestCalculateRealtimeForAPI_SevenHoleZeroPresent 七孔 P6/P7=0 视为已提供（present-zero 语义）。
func TestCalculateRealtimeForAPI_SevenHoleZeroPresent(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true, result: seveninterp.InterpolationResult{IsValid: true}})

	p6, p7 := 0.0, 0.0
	if _, err := mgr.CalculateRealtimeForAPI(traversal.ProbeTypeSevenHole, ProbePressureInput{
		P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: &p6, P7: &p7, PAtm: 101325, TAtm: 20,
	}); err != nil {
		t.Fatalf("P6/P7=0 must be accepted as present, got: %v", err)
	}
}

// TestCalculateRealtimeForAPI_SevenHoleMissingP6P7 七孔 P6/P7 缺失（nil）→ usecase 拒绝。
// 校验在 usecase 内部完成，API 不再持有 *float64 业务逻辑。
func TestCalculateRealtimeForAPI_SevenHoleMissingP6P7(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})

	cases := []struct {
		name string
		in   ProbePressureInput
	}{
		{"both missing", ProbePressureInput{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, PAtm: 101325, TAtm: 20}},
		{"P6 missing", ProbePressureInput{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P7: ptrFloat64(7), PAtm: 101325, TAtm: 20}},
		{"P7 missing", ProbePressureInput{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: ptrFloat64(6), PAtm: 101325, TAtm: 20}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mgr.CalculateRealtimeForAPI(traversal.ProbeTypeSevenHole, tc.in)
			if err == nil {
				t.Fatalf("missing P6/P7 must be rejected")
			}
			if !errors.Is(err, ErrMissingSevenHolePressures) {
				t.Errorf("err = %v, want ErrMissingSevenHolePressures", err)
			}
		})
	}
}

// TestCalculateRealtimeForAPI_ProbeMismatch 请求类型与当前配置不一致 → 拒绝。
func TestCalculateRealtimeForAPI_ProbeMismatch(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{} // five-hole
	mgr.SetInterpolator(&mockInterpolator{})
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})

	p6, p7 := 6.0, 7.0
	_, err := mgr.CalculateRealtimeForAPI(traversal.ProbeTypeSevenHole, ProbePressureInput{
		P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: &p6, P7: &p7,
	})
	if err == nil {
		t.Fatal("seven-hole request against five-hole config must be rejected")
	}
}

// TestCalculateRealtimeForAPI_UnknownType 未知 probeType → 拒绝。
func TestCalculateRealtimeForAPI_UnknownType(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{}
	mgr.SetInterpolator(&mockInterpolator{})

	if _, err := mgr.CalculateRealtimeForAPI("nine-hole", ProbePressureInput{}); err == nil {
		t.Fatal("unknown probe type must be rejected")
	}
}

// ptrFloat64 测试辅助：返回 v 的指针。
func ptrFloat64(v float64) *float64 { return &v }
