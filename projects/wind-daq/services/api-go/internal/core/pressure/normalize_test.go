package pressure

import (
	"math"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
)

// TestNormalizePressureToGaugePa 验证绝压→表压归一化的全单位路径。
// 覆盖 spec 测试用例表前 7 项：Pa/kPa/MPa/psi/kgf/cm²/kgfcm2 alias/Patm 换算。
func TestNormalizePressureToGaugePa(t *testing.T) {
	uc := device.NewUnitConverter()
	const patm = 101325.0 // 1 atm = 101325 Pa

	cases := []struct {
		name         string
		value        float64
		unit         string
		pressureType string
		want         float64
		wantErr      bool
		errTolerance float64 // 允许误差（部分单位系数为浮点近似）
	}{
		{"Pa+表压原样", 1000.0, "Pa", "gauge", 1000.0, false, 1e-6},
		{"kPa+表压", 5.0, "kPa", "gauge", 5000.0, false, 1e-6},
		{"MPa+绝压", 0.2, "MPa", "absolute", 200000.0 - patm, false, 1e-6},
		{"psi+绝压", 14.7, "psi", "absolute", 14.7*6894.757 - patm, false, 0.1},
		{"kgf/cm²+表压", 1.0, "kgf/cm2", "gauge", 98066.5, false, 1e-6},
		{"kgfcm2 alias 与 kgf/cm2 等价", 1.0, "kgfcm2", "gauge", 98066.5, false, 1e-6},
		{"空串 pressureType 等价 gauge", 5.0, "kPa", "", 5000.0, false, 1e-6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePressureToGaugePa(tc.value, tc.unit, tc.pressureType, patm, uc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 error，实际 got=%v err=nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("未期望 error: %v", err)
			}
			if math.Abs(got-tc.want) > tc.errTolerance {
				t.Errorf("value=%v unit=%s type=%s: 期望 %v，实际 %v", tc.value, tc.unit, tc.pressureType, tc.want, got)
			}
		})
	}
}

// TestNormalizePressureToGaugePa_ConverterNil 验证 converter nil 时返回明确 error。
// 防止调用方忘记注入导致空指针 panic。
func TestNormalizePressureToGaugePa_ConverterNil(t *testing.T) {
	_, err := NormalizePressureToGaugePa(1000.0, "Pa", "gauge", 101325.0, nil)
	if err == nil {
		t.Fatal("converter=nil 时期望 error，实际 nil")
	}
}

// TestNormalizePressureToGaugePa_UnknownUnit 验证未知单位返回 error。
// 错误消息需包含原单位字符串，便于调用方日志定位。
func TestNormalizePressureToGaugePa_UnknownUnit(t *testing.T) {
	uc := device.NewUnitConverter()
	_, err := NormalizePressureToGaugePa(1.0, "degC", "gauge", 101325.0, uc)
	if err == nil {
		t.Fatal("未知单位期望 error，实际 nil")
	}
	if !contains(err.Error(), "degC") {
		t.Errorf("错误消息应包含原单位 'degC'，实际: %v", err)
	}
}

// TestConvertToPa 验证 Patm 通道专用换算：仅单位换算，不减大气压。
func TestConvertToPa(t *testing.T) {
	uc := device.NewUnitConverter()

	cases := []struct {
		name    string
		value   float64
		unit    string
		want    float64
		wantErr bool
	}{
		{"kPa→Pa", 101.325, "kPa", 101325.0, false},
		{"Pa 原样", 1000.0, "Pa", 1000.0, false},
		{"MPa→Pa", 0.1, "MPa", 100000.0, false},
		{"kgf/cm2→Pa", 1.0, "kgf/cm2", 98066.5, false},
		{"未知单位报错", 1.0, "degC", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertToPa(tc.value, tc.unit, uc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 error，实际 got=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("未期望 error: %v", err)
			}
			if math.Abs(got-tc.want) > 1e-6 {
				t.Errorf("value=%v unit=%s: 期望 %v，实际 %v", tc.value, tc.unit, tc.want, got)
			}
		})
	}
}

// TestConvertToPa_ConverterNil 验证 ConvertToPa 在 converter nil 时也返回 error。
func TestConvertToPa_ConverterNil(t *testing.T) {
	_, err := ConvertToPa(101.325, "kPa", nil)
	if err == nil {
		t.Fatal("converter=nil 时期望 error，实际 nil")
	}
}

// contains 简单字符串包含检查，避免引入 strings 包仅为单测。
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
