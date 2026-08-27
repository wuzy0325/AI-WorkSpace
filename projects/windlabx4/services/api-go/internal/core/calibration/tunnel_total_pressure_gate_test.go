package calibration

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeTunnelTotalPressureGate(t *testing.T) {
	// 未配置 → nil
	if gate := NormalizeTunnelTotalPressureGate(Config{}); gate != nil {
		t.Fatalf("未配置时应返回 nil，实际: %+v", gate)
	}
	// 已配置但未启用 → nil
	if gate := NormalizeTunnelTotalPressureGate(Config{TunnelTotalPressureGate: &TunnelTotalPressureGateConfig{Enabled: false}}); gate != nil {
		t.Fatalf("未启用时应返回 nil，实际: %+v", gate)
	}
	// 已启用 → 规整负超时为 0，保留范围
	gate := NormalizeTunnelTotalPressureGate(Config{
		TunnelTotalPressureGate: &TunnelTotalPressureGateConfig{
			Enabled: true, MinTotalPressure: 100, MaxTotalPressure: 200, TimeoutSec: -5,
		},
	})
	if gate == nil {
		t.Fatal("已启用时应返回非 nil")
	}
	if gate.TimeoutSec != 0 {
		t.Fatalf("负超时应规整为 0，实际: %d", gate.TimeoutSec)
	}
	if gate.MinTotalPressure != 100 || gate.MaxTotalPressure != 200 {
		t.Fatalf("范围应原样保留，实际: [%f, %f]", gate.MinTotalPressure, gate.MaxTotalPressure)
	}
}

func TestIsTotalPressureInRange(t *testing.T) {
	gate := &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: 100, MaxTotalPressure: 200}
	cases := []struct {
		name  string
		value float64
		want  bool
	}{
		{"下限之内", 150, true},
		{"正好下限", 100, true},
		{"正好上限", 200, true},
		{"低于下限", 99.9, false},
		{"高于上限", 200.1, false},
	}
	for _, c := range cases {
		if got := IsTotalPressureInRange(gate, c.value); got != c.want {
			t.Errorf("%s: IsTotalPressureInRange(%f) = %v, want %v", c.name, c.value, got, c.want)
		}
	}
	// gate 为 nil 或未启用 → 恒 true（跳过判定）
	if !IsTotalPressureInRange(nil, 1) {
		t.Error("gate 为 nil 时应恒为 true")
	}
	if !IsTotalPressureInRange(&TunnelTotalPressureGateConfig{Enabled: false}, 1) {
		t.Error("未启用时应恒为 true")
	}
}

func TestValidateTunnelTotalPressureGate(t *testing.T) {
	// 未配置 / 未启用 → nil
	if err := ValidateTunnelTotalPressureGate(Config{}); err != nil {
		t.Fatalf("未配置时应返回 nil，实际: %v", err)
	}
	if err := ValidateTunnelTotalPressureGate(Config{TunnelTotalPressureGate: &TunnelTotalPressureGateConfig{Enabled: false}}); err != nil {
		t.Fatalf("未启用时应返回 nil，实际: %v", err)
	}

	validChannels := completeFiveHoleProbeChannels()
	// 每个用例用独立副本，避免共享底层数组导致的前序变异污染
	cloneChannels := func() []ProbeChannel {
		cp := make([]ProbeChannel, len(validChannels))
		copy(cp, validChannels)
		return cp
	}

	// 合法配置（复用 completeFiveHoleConfig 的通道）→ nil
	okConfig := Config{
		Type:          string(TypeFiveHole),
		ProbeChannels: cloneChannels(),
		TunnelTotalPressureGate: &TunnelTotalPressureGateConfig{
			Enabled: true, MinTotalPressure: 50, MaxTotalPressure: 120,
		},
	}
	if err := ValidateTunnelTotalPressureGate(okConfig); err != nil {
		t.Fatalf("合法配置应通过校验，实际: %v", err)
	}

	// 范围非法（min > max）→ 报错
	badRange := okConfig
	badRange.TunnelTotalPressureGate = &TunnelTotalPressureGateConfig{
		Enabled: true, MinTotalPressure: 200, MaxTotalPressure: 100,
	}
	err := ValidateTunnelTotalPressureGate(badRange)
	if err == nil || !strings.Contains(err.Error(), "范围非法") {
		t.Fatalf("min>max 应报范围非法错误，实际: %v", err)
	}

	// 缺少 pTotal 通道 → 报错
	missingChannel := okConfig
	missingChannel.ProbeChannels = cloneChannels()[:6]
	err = ValidateTunnelTotalPressureGate(missingChannel)
	if err == nil || !strings.Contains(err.Error(), "fiveHole.pTotal") {
		t.Fatalf("缺少 pTotal 通道应报错，实际: %v", err)
	}

	// pTotal 未启用 → 视为缺失
	disabledChannel := okConfig
	disabledChannel.ProbeChannels = cloneChannels()
	for i := range disabledChannel.ProbeChannels {
		if disabledChannel.ProbeChannels[i].Role == fiveHoleTotalPressureRole {
			disabledChannel.ProbeChannels[i].Enabled = false
		}
	}
	if err := ValidateTunnelTotalPressureGate(disabledChannel); err == nil {
		t.Fatal("pTotal 未启用时应报错")
	}

	// pTotal 缺少设备ID → 报错
	noDevice := okConfig
	noDevice.ProbeChannels = cloneChannels()
	for i := range noDevice.ProbeChannels {
		if noDevice.ProbeChannels[i].Role == fiveHoleTotalPressureRole {
			noDevice.ProbeChannels[i].DeviceID = ""
		}
	}
	err = ValidateTunnelTotalPressureGate(noDevice)
	if err == nil || !strings.Contains(err.Error(), "设备ID未配置") {
		t.Fatalf("pTotal 缺设备ID应报错，实际: %v", err)
	}

	// pTotal 通道索引无效 → 报错
	badIndex := okConfig
	badIndex.ProbeChannels = cloneChannels()
	for i := range badIndex.ProbeChannels {
		if badIndex.ProbeChannels[i].Role == fiveHoleTotalPressureRole {
			badIndex.ProbeChannels[i].ChannelIndex = -1
		}
	}
	err = ValidateTunnelTotalPressureGate(badIndex)
	if err == nil || !strings.Contains(err.Error(), "通道索引无效") {
		t.Fatalf("pTotal 通道索引无效应报错，实际: %v", err)
	}
}

// TestValidateTunnelTotalPressureGateRejectsNonFiniteAndZeroRange 回归测试（code-review Important 1）：
// 范围边界必须为有限数（NaN/Inf 拒绝），且上下限不同时为 0——
// JSON null 解码到非指针 float64 会落为 0，null/null 若被当作合法 [0,0] 范围会导致门控永不满足。
func TestValidateTunnelTotalPressureGateRejectsNonFiniteAndZeroRange(t *testing.T) {
	validChannels := completeFiveHoleProbeChannels()
	base := Config{
		Type:          string(TypeFiveHole),
		ProbeChannels: validChannels,
	}

	cases := []struct {
		name string
		gate *TunnelTotalPressureGateConfig
		want string
	}{
		{"min=NaN", &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: math.NaN(), MaxTotalPressure: 100}, "有效数字"},
		{"max=NaN", &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: 50, MaxTotalPressure: math.NaN()}, "有效数字"},
		{"min=+Inf", &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: math.Inf(1), MaxTotalPressure: 100}, "有效数字"},
		{"max=-Inf", &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: 50, MaxTotalPressure: math.Inf(-1)}, "有效数字"},
		{"[0,0] null 形态", &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: 0, MaxTotalPressure: 0}, "范围未配置"},
	}
	for _, c := range cases {
		config := base
		config.TunnelTotalPressureGate = c.gate
		err := ValidateTunnelTotalPressureGate(config)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 应返回含 %q 的错误，实际: %v", c.name, c.want, err)
		}
	}

	// 合法范围 [0, 5000]（min=0 但 max>0）应通过
	okConfig := base
	okConfig.TunnelTotalPressureGate = &TunnelTotalPressureGateConfig{Enabled: true, MinTotalPressure: 0, MaxTotalPressure: 5000}
	if err := ValidateTunnelTotalPressureGate(okConfig); err != nil {
		t.Fatalf("合法范围 [0, 5000] 应通过校验，实际: %v", err)
	}
}

func TestFindFiveHoleTotalPressureChannel(t *testing.T) {
	ch, ok := findFiveHoleTotalPressureChannel(Config{ProbeChannels: completeFiveHoleProbeChannels()})
	if !ok || ch.DeviceID != "dev-1" || ch.ChannelIndex != 18 {
		t.Fatalf("应找到 pTotal 通道 dev-1:18，实际: %+v, ok=%v", ch, ok)
	}
	// 未启用 → 找不到
	config := Config{ProbeChannels: completeFiveHoleProbeChannels()}
	for i := range config.ProbeChannels {
		if config.ProbeChannels[i].Role == fiveHoleTotalPressureRole {
			config.ProbeChannels[i].Enabled = false
		}
	}
	if _, ok := findFiveHoleTotalPressureChannel(config); ok {
		t.Fatal("pTotal 未启用时不应找到")
	}
}
