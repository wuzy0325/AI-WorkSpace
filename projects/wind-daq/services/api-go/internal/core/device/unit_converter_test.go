package device

import (
	"math"
	"testing"
)

// TestUnitConverter_PressureToBaseUnit 验证压力单位到基单位 Pa 的换算。
// 覆盖 11 个注册单位 + kgf/cm2 alias，确保所有系数与 pressureFamily 注册表一致。
func TestUnitConverter_PressureToBaseUnit(t *testing.T) {
	uc := NewUnitConverter()

	cases := []struct {
		name    string
		unit    string
		value   float64
		want    float64
		wantErr bool
	}{
		{"Pa 原样", "Pa", 1000.0, 1000.0, false},
		{"kPa ×1000", "kPa", 5.0, 5000.0, false},
		{"MPa ×1e6", "MPa", 0.2, 200000.0, false},
		{"mmH2O", "mmH2O", 1000.0, 9806.65, false},
		{"mmHg", "mmHg", 1.0, 133.322, false},
		{"psi", "psi", 1.0, 6894.757, false},
		{"bar", "bar", 1.0, 100000.0, false},
		{"mbar", "mbar", 1.0, 100.0, false},
		{"inH2O", "inH2O", 1.0, 249.0889, false},
		{"inHg", "inHg", 1.0, 3386.389, false},
		{"kgfcm2 历史字面量", "kgfcm2", 1.0, 98066.5, false},
		{"kgf/cm2 alias 与 kgfcm2 等价", "kgf/cm2", 1.0, 98066.5, false},
		{"未知单位报错", "V", 1.0, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := uc.ToBaseUnit(tc.value, tc.unit)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望 error，实际 got=%v err=nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("未期望 error: %v", err)
			}
			if math.Abs(got-tc.want) > 1e-6 {
				t.Errorf("unit=%s value=%v: 期望 %v，实际 %v", tc.unit, tc.value, tc.want, got)
			}
		})
	}
}

// TestUnitConverter_KgfAliasEquivalence 验证 kgf/cm2 与 kgfcm2 在双向换算上完全等价。
// 防止 alias 引入后两 key 指向同值但 FromBaseUnit 路径不一致的边缘情况。
func TestUnitConverter_KgfAliasEquivalence(t *testing.T) {
	uc := NewUnitConverter()
	const value = 98066.5 // 1 kgf/cm2 = 1 kgfcm2 = 98066.5 Pa

	paFromAlias, err := uc.ToBaseUnit(1.0, "kgf/cm2")
	if err != nil {
		t.Fatalf("kgf/cm2 ToBaseUnit failed: %v", err)
	}
	paFromLegacy, err := uc.ToBaseUnit(1.0, "kgfcm2")
	if err != nil {
		t.Fatalf("kgfcm2 ToBaseUnit failed: %v", err)
	}
	if paFromAlias != paFromLegacy {
		t.Errorf("ToBaseUnit 不一致: alias=%v legacy=%v", paFromAlias, paFromLegacy)
	}

	backAlias, err := uc.FromBaseUnit(value, "kgf/cm2")
	if err != nil {
		t.Fatalf("kgf/cm2 FromBaseUnit failed: %v", err)
	}
	backLegacy, err := uc.FromBaseUnit(value, "kgfcm2")
	if err != nil {
		t.Fatalf("kgfcm2 FromBaseUnit failed: %v", err)
	}
	if math.Abs(backAlias-backLegacy) > 1e-9 {
		t.Errorf("FromBaseUnit 不一致: alias=%v legacy=%v", backAlias, backLegacy)
	}

	// BaseUnitFor 应返回相同基单位
	baseAlias, ok1 := uc.BaseUnitFor("kgf/cm2")
	baseLegacy, ok2 := uc.BaseUnitFor("kgfcm2")
	if !ok1 || !ok2 || baseAlias != baseLegacy || baseAlias != "Pa" {
		t.Errorf("BaseUnitFor 不一致: alias=%q ok=%v legacy=%q ok=%v", baseAlias, ok1, baseLegacy, ok2)
	}
}

// TestUnitConverter_TemperatureBoundary 验证温度单位族的边界行为，
// 确保 pressureFamily 新增 alias 未破坏温度族 lookup 逻辑。
func TestUnitConverter_TemperatureBoundary(t *testing.T) {
	uc := NewUnitConverter()

	// ℃ 原样
	got, err := uc.ToBaseUnit(25.0, "℃")
	if err != nil || got != 25.0 {
		t.Errorf("℃ 换算异常: got=%v err=%v", got, err)
	}

	// ℉ 特殊分支
	got, err = uc.ToBaseUnit(32.0, "℉")
	if err != nil || got != 0.0 {
		t.Errorf("℉→℃ 异常: got=%v err=%v（期望 0）", got, err)
	}

	// 压力单位不应被误判为温度
	if _, err := uc.ToBaseUnit(1.0, "kPa"); err != nil {
		t.Errorf("kPa 不应报错: %v", err)
	}
}

func TestUnitConverterSupportsTemperatureAliasesUsedByProfiles(t *testing.T) {
	uc := NewUnitConverter()
	for _, unit := range []string{"℃", "°C", "degC"} {
		got, err := uc.ToBaseUnit(25, unit)
		if err != nil || got != 25 {
			t.Fatalf("unit %q: expected 25 C, got %v, err=%v", unit, got, err)
		}
	}
	for _, unit := range []string{"℉", "°F", "degF"} {
		got, err := uc.ToBaseUnit(32, unit)
		if err != nil || got != 0 {
			t.Fatalf("unit %q: expected 0 C, got %v, err=%v", unit, got, err)
		}
	}
}
