package storage

import (
	"encoding/json"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/calibration"
)

// 这些测试验证 core/calibration 结构体的 JSON 序列化行为（omitempty / 向后兼容）。
// 按 hexagonal 分层约束，core/ 不允许 JSON I/O，因此 JSON 序列化断言放在
// adapters/storage 层——存储/传输是序列化的天然消费者，由 storage 包验证
// 序列化契约最合适。core/calibration/types_test.go 仅保留字段值/常量/接口断言。

// TestCalPointExtendedFieldsOmitempty 验证 CalPoint 新增三字段的 omitempty 行为
// 五孔等已有模块不填新字段时，JSON 序列化结果不应包含新字段（零回归硬约束）
func TestCalPointExtendedFieldsOmitempty(t *testing.T) {
	// 场景 1：五孔场景，不填新字段——序列化结果不得出现 motionCoordinates/region/sector
	legacy := calibration.CalPoint{
		ID:          1,
		Coordinates: map[string]float64{"α": 5.0, "β": 10.0},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy CalPoint: %v", err)
	}
	got := string(raw)
	for _, key := range []string{`"motionCoordinates"`, `"region"`, `"sector"`} {
		if strings.Contains(got, key) {
			t.Fatalf("legacy CalPoint JSON 不应包含 %s，实际: %s", key, got)
		}
	}

	// 场景 2：七孔场景，填满新字段——序列化结果必须包含三个字段
	// 坐标自洽（黄金用例 G5）：θ=30°, φ=330° 按 spec §3.3 公式
	// α=-arctan(tanθ·sinφ)=16.1°, β=arctan(tanθ·cosφ)=26.6°。
	seven := calibration.CalPoint{
		ID:                2,
		Coordinates:       map[string]float64{"θ": 30.0, "φ": 330.0},
		MotionCoordinates: map[string]float64{"α": 16.1, "β": 26.6},
		Region:            "outer",
		Sector:            2,
	}
	raw2, err := json.Marshal(seven)
	if err != nil {
		t.Fatalf("marshal seven-hole CalPoint: %v", err)
	}
	got2 := string(raw2)
	for _, key := range []string{`"motionCoordinates"`, `"region"`, `"sector"`} {
		if !strings.Contains(got2, key) {
			t.Fatalf("seven-hole CalPoint JSON 应包含 %s，实际: %s", key, got2)
		}
	}
}

// TestSevenHoleRawDataPointerFieldsOmitempty 验证 SevenHoleRawData 指针字段
// （PTotal/PStatic/TTunnel）的 omitempty JSON 行为：不填时序列化结果不含对应 key
func TestSevenHoleRawDataPointerFieldsOmitempty(t *testing.T) {
	empty := calibration.SevenHoleRawData{P1: 1.0}
	got, _ := json.Marshal(empty)
	if strings.Contains(string(got), `"pTotal"`) {
		t.Fatalf("空 PTotal 不应序列化: %s", string(got))
	}
}

// TestSevenHoleCoefficientsPointerFieldsOmitempty 验证 SevenHoleCoefficients 指针字段
// （MachNumber/Velocity）的 omitempty JSON 行为：nil 时序列化结果不含对应 key
func TestSevenHoleCoefficientsPointerFieldsOmitempty(t *testing.T) {
	empty := calibration.SevenHoleCoefficients{Kalpha: 0.1}
	got, _ := json.Marshal(empty)
	if strings.Contains(string(got), `"machNumber"`) {
		t.Fatalf("空 MachNumber 不应序列化: %s", string(got))
	}
}

// TestRealtimeEventSevenHoleExtensionBackwardCompatible 验证 RealtimeEvent 追加七孔字段后
// 五孔场景序列化结果不出现 sevenHoleRaw/sevenHoleCoefficients key（向后兼容硬约束）
func TestRealtimeEventSevenHoleExtensionBackwardCompatible(t *testing.T) {
	fhRaw := &calibration.FiveHoleRawData{P1: 1.0, P2: 2.0, P3: 3.0, P4: 4.0, P5: 5.0}
	fhCoeffs := &calibration.FiveHoleCoefficients{Kalpha: 0.1, Kbeta: 0.2, CPT: 0.3, CPS: 0.4}
	fiveOnly := calibration.RealtimeEvent{
		TaskID:               "task-1",
		WindowTag:            "w1",
		Type:                 calibration.TypeFiveHole,
		Timestamp:            1700000000000,
		FiveHoleRaw:          fhRaw,
		FiveHoleCoefficients: fhCoeffs,
	}
	got, err := json.Marshal(fiveOnly)
	if err != nil {
		t.Fatalf("marshal five-hole RealtimeEvent: %v", err)
	}
	s := string(got)
	for _, key := range []string{`"sevenHoleRaw"`, `"sevenHoleCoefficients"`} {
		if strings.Contains(s, key) {
			t.Fatalf("五孔 RealtimeEvent 不应包含 %s（向后兼容）: %s", key, s)
		}
	}

	// 七孔场景：填七孔字段时序列化必须包含
	shRaw := &calibration.SevenHoleRawData{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: 6, P7: 7}
	shCoeffs := &calibration.SevenHoleCoefficients{Kalpha: 0.043}
	sevenEvent := calibration.RealtimeEvent{
		TaskID:                "task-7",
		Type:                  calibration.TypeSevenHole,
		Timestamp:             1700000000001,
		SevenHoleRaw:          shRaw,
		SevenHoleCoefficients: shCoeffs,
	}
	got2, _ := json.Marshal(sevenEvent)
	s2 := string(got2)
	for _, key := range []string{`"sevenHoleRaw"`, `"sevenHoleCoefficients"`} {
		if !strings.Contains(s2, key) {
			t.Fatalf("七孔 RealtimeEvent 应包含 %s: %s", key, s2)
		}
	}
}
