package driver

import "testing"

func TestNormalizePressureUnit(t *testing.T) {
	cases := []struct{ input, want string }{
		{"kpa", "kPa"},
		{"KPA", "kPa"},
		{"kPa", "kPa"},
		{"mpa", "MPa"},
		{"MPa", "MPa"},
		{"pa", "Pa"},
		{"bar", "bar"},
		{"psi", "psi"},
		{"mmhg", "mmHg"},
		{"mmHg", "mmHg"},
		{"atm", "atm"},
		{"inhg", "inHg"},
		{"kgf/cm2", "kgf/cm2"},
	}
	for _, c := range cases {
		got := NormalizePressureUnit(c.input)
		if got != c.want {
			t.Errorf("NormalizePressureUnit(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseConSTGeneralUnit_NormalizesCase(t *testing.T) {
	cases := []struct{ input, want string }{
		{"1130", "Pa"},
		{"1133", "kPa"},
		{"1132", "MPa"},
		{"kpa", "kPa"},
		{"mpa", "MPa"},
		{"mmhg", "mmHg"},
		{"kPa", "kPa"},
	}
	for _, c := range cases {
		got := parseConSTGeneralUnit(c.input)
		if got != c.want {
			t.Errorf("parseConSTGeneralUnit(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestPressureUnitToCode811AOfficialCodes(t *testing.T) {
	// 期望值来自官方《ConST811A通讯指令集》码表：
	// 1130=Pa、1131=GPa、1132=MPa、1133=kPa、1137=bar、1140=atm 等。
	cases := map[string]string{
		"Pa": "1130", "kPa": "1133", "MPa": "1132",
		"bar": "1137", "mbar": "1138", "atm": "1140",
		"psi": "1141", "kgf/cm2": "1145", "inHg": "1156", "mmHg": "1158",
	}
	for unit, want := range cases {
		got, ok := pressureUnitToCode811A(unit)
		if !ok || got != want {
			t.Errorf("pressureUnitToCode811A(%q) = %q, %v; want %q, true", unit, got, ok, want)
		}
	}
	if _, ok := pressureUnitToCode811A("hPa"); ok {
		t.Error("pressureUnitToCode811A(hPa) 应不支持（应用单位集合之外的单位不映射）")
	}
}

func TestParseConST811AUnit_OfficialTable(t *testing.T) {
	// 811A 专用码表：与通用表的关键差异——1131=GPa 而非 Pa，
	// 1134/1135 是 mPa/μPa 而非 mmHg/atm。
	cases := []struct{ input, want string }{
		{"1130", "Pa"},
		{"1131", "GPa"},
		{"1132", "MPa"},
		{"1133", "kPa"},
		{"1134", "mPa"},
		{"1136", "hPa"},
		{"1137", "bar"},
		{"1139", "torr"},
		{"1140", "atm"},
		{"1141", "psi"},
		{"1145", "kgf/cm2"},
		{"1158", "mmHg@0C"},
		{"kPa", "kPa"},
		{"garbage", "MPa"},
	}
	for _, c := range cases {
		got := parseConST811AUnit(c.input)
		if got != c.want {
			t.Errorf("parseConST811AUnit(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseConST820Unit_NormalizesCase(t *testing.T) {
	// 820 的 UNIT:PRESsure? 实机返回字符串单位（大写），并非数字码。
	cases := []struct{ input, want string }{
		{"PA", "Pa"},
		{"KPA", "kPa"},
		{"MPA", "MPa"},
		{"PSI", "psi"},
		{"BAR", "bar"},
		{"MBAR", "mbar"},
		{"kpa", "kPa"},
		{"mpa", "MPa"},
	}
	for _, c := range cases {
		got := parseConST820Unit(c.input)
		if got != c.want {
			t.Errorf("parseConST820Unit(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestPressureUnitToCode820UsesLabVIEWValues(t *testing.T) {
	cases := map[string]string{
		"Pa": "0", "kPa": "1", "MPa": "2", "psi": "3", "kgf/cm2": "10",
	}
	for unit, want := range cases {
		got, ok := pressureUnitToCode820(unit)
		if !ok || got != want {
			t.Errorf("pressureUnitToCode820(%q) = %q, %v; want %q, true", unit, got, ok, want)
		}
	}
}
