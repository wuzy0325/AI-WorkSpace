package usecase

import (
	"strings"
	"testing"

	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"windlabx4/services/api-go/internal/core/traversal"
)

// ===== 七孔插值器 mock =====

// mockSevenInterpolator 是七孔 seveninterp.Interpolator 的可控 mock，
// 同时实现可选能力 Identity() string（含 7 个文件信息）。
type mockSevenInterpolator struct {
	loaded bool
	result seveninterp.InterpolationResult
	err    error
	calls  int
	lastIn seveninterp.InterpolationInput
}

func (m *mockSevenInterpolator) IsLoaded() bool { return m.loaded }
func (m *mockSevenInterpolator) GetValidRange() seveninterp.PrbValidRange {
	return seveninterp.PrbValidRange{AlphaMin: -30, AlphaMax: 30, BetaMin: -30, BetaMax: 30}
}
func (m *mockSevenInterpolator) Calculate(in seveninterp.InterpolationInput) (seveninterp.InterpolationResult, error) {
	m.calls++
	m.lastIn = in
	return m.result, m.err
}
func (m *mockSevenInterpolator) Identity() string {
	return "seven-hole-prb(7=cal/7.prb;1=cal/1.prb;2=cal/2.prb;3=cal/3.prb;4=cal/4.prb;5=cal/5.prb;6=cal/6.prb)"
}

func sevenHoleLabels() map[int]string {
	return map[int]string{
		0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5", 5: "P6", 6: "P7", 16: "Patm", 17: "Tatm",
	}
}

// TestBuildRawPressureSevenHole 七孔原始压力按 9 标签装配（Task 8 验收：
// P1..P7/Patm/Tatm 装配正确，Patm/Tatm 语义与五孔一致）。
func TestBuildRawPressureSevenHole(t *testing.T) {
	labels := sevenHoleLabels()
	refs := singleDeviceRefs("dev-7h", labels)
	provider := &mockChannelUnitProvider{units: map[int]string{
		0: "kPa", 1: "kPa", 2: "kPa", 3: "kPa", 4: "kPa", 5: "kPa", 6: "kPa", 16: "kPa", 17: "°C",
	}}
	values := map[int]float64{
		0: 1.0, 1: 2.0, 2: 3.0, 3: 4.0, 4: 5.0, 5: 6.0, 6: 7.0, 16: 101.325, 17: 20.0,
	}
	raw, in, ok := buildRawPressureForProbe(values, labels, refs, provider, "gauge", probeStrategies[traversal.ProbeTypeSevenHole])
	if !ok {
		t.Fatal("seven-hole assembly must succeed with full labels + provider")
	}
	// kPa -> Pa：探针孔道与 Patm 均换算。
	for i, label := range []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7"} {
		want := float64(i+1) * 1000
		if raw[label] != want {
			t.Errorf("raw[%s] = %v, want %v", label, raw[label], want)
		}
		if in.P[i] != want {
			t.Errorf("input P[%d] = %v, want %v", i, in.P[i], want)
		}
	}
	if raw["Patm"] != 101325.0 {
		t.Errorf("raw[Patm] = %v, want 101325", raw["Patm"])
	}
	if in.PAtm != 101325.0 || in.TAtm != 20.0 {
		t.Errorf("input atmosphere = (%v,%v), want (101325,20)", in.PAtm, in.TAtm)
	}
}

// TestBuildRawPressureSevenHoleShuffled 乱序通道经标签归一化正确：
// 内部通道键与标签的映射不依赖通道顺序。
func TestBuildRawPressureSevenHoleShuffled(t *testing.T) {
	// 标签挂在与孔号不一致的通道上（如 P1 在 CH8、P7 在 CH1）。
	labels := map[int]string{
		8: "P1", 2: "P2", 5: "P3", 0: "P4", 9: "P5", 3: "P6", 1: "P7", 16: "Patm", 17: "Tatm",
	}
	refs := singleDeviceRefs("dev-7h", labels)
	provider := &mockChannelUnitProvider{units: map[int]string{
		8: "Pa", 2: "Pa", 5: "Pa", 0: "Pa", 9: "Pa", 3: "Pa", 1: "Pa", 16: "Pa", 17: "°C",
	}}
	values := map[int]float64{
		8: 100, 2: 200, 5: 300, 0: 400, 9: 500, 3: 600, 1: 700, 16: 101325, 17: 20,
	}
	_, in, ok := buildRawPressureForProbe(values, labels, refs, provider, "gauge", probeStrategies[traversal.ProbeTypeSevenHole])
	if !ok {
		t.Fatal("shuffled assembly must succeed")
	}
	want := [7]float64{100, 200, 300, 400, 500, 600, 700}
	for i, w := range want {
		if in.P[i] != w {
			t.Errorf("input P[%d] = %v, want %v", i, in.P[i], w)
		}
	}
}

// TestBuildRawPressureSevenHoleMissingLabel 缺标签时 ok=false（由 hasAllLabels 兜底）。
func TestBuildRawPressureSevenHoleMissingLabel(t *testing.T) {
	labels := map[int]string{0: "P1", 1: "P2", 16: "Patm", 17: "Tatm"} // 缺 P3..P7
	refs := singleDeviceRefs("dev-7h", labels)
	provider := &mockChannelUnitProvider{units: map[int]string{0: "Pa", 1: "Pa", 16: "Pa"}}
	values := map[int]float64{0: 100, 1: 200, 16: 101325, 17: 20}
	_, _, ok := buildRawPressureForProbe(values, labels, refs, provider, "gauge", probeStrategies[traversal.ProbeTypeSevenHole])
	if ok {
		t.Fatal("missing P3..P7 labels must yield ok=false")
	}
}

// TestCalculateRealtimeByProbe 七孔走新字段、五孔走旧缓存路径；
// 显式类型与当前配置不一致时拒绝（Task 8 验收）。
func TestCalculateRealtimeByProbe(t *testing.T) {
	t.Run("seven-hole uses dedicated field", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
		seven := &mockSevenInterpolator{
			loaded: true,
			result: seveninterp.InterpolationResult{IsValid: true, Alpha: 1.5, Beta: -2.5, TotalPressure: 1000, StaticPressure: 900, MachNumber: 0.3, Velocity: 100},
		}
		mgr.SetSevenHoleInterpolator(seven)
		res, err := mgr.CalculateRealtimeByProbe(traversal.ProbeTypeSevenHole, probeCalcInput{P: [7]float64{1, 2, 3, 4, 5, 6, 7}, PAtm: 101325, TAtm: 20})
		if err != nil {
			t.Fatalf("CalculateRealtimeByProbe: %v", err)
		}
		if seven.calls != 1 {
			t.Fatalf("seven-hole interpolator calls = %d, want 1", seven.calls)
		}
		if !res.IsValid || res.Alpha != 1.5 || res.Beta != -2.5 || res.Pt != 1000 || res.Ps != 900 || res.Mach != 0.3 || res.Velocity != 100 {
			t.Errorf("unexpected result: %+v", res)
		}
		if seven.lastIn.P7 != 7 {
			t.Errorf("P7 not forwarded: %v", seven.lastIn.P7)
		}
	})

	t.Run("five-hole uses legacy CalculateRealtime path", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{} // legacy empty probeType -> five-hole
		mgr.SetInterpolator(&mockInterpolator{})
		res, err := mgr.CalculateRealtimeByProbe("", probeCalcInput{PAtm: 101325, TAtm: 20})
		if err != nil {
			t.Fatalf("legacy empty probeType must dispatch five-hole: %v", err)
		}
		_ = res
	})

	t.Run("mismatched explicit type rejected", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{} // five-hole
		mgr.SetInterpolator(&mockInterpolator{})
		mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
		if _, err := mgr.CalculateRealtimeByProbe(traversal.ProbeTypeSevenHole, probeCalcInput{}); err == nil {
			t.Fatal("seven-hole request against five-hole config must be rejected")
		}
	})

	t.Run("mismatched explicit type rejected (reverse)", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
		mgr.SetInterpolator(&mockInterpolator{})
		mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
		if _, err := mgr.CalculateRealtimeByProbe(traversal.ProbeTypeFiveHole, probeCalcInput{}); err == nil {
			t.Fatal("five-hole request against seven-hole config must be rejected")
		}
	})

	t.Run("unknown type rejected", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{}
		if _, err := mgr.CalculateRealtimeByProbe("nine-hole", probeCalcInput{}); err == nil {
			t.Fatal("unknown probe type must be rejected")
		}
	})

	t.Run("seven-hole not loaded", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
		if _, err := mgr.CalculateRealtimeByProbe(traversal.ProbeTypeSevenHole, probeCalcInput{}); err == nil {
			t.Fatal("unloaded seven-hole interpolator must error")
		}
	})
}

// TestProbeStrategyStateless 两个 Manager 并行加载不同插值器时计算互不串扰
// （包级策略表不捕获 Manager 实例）。
func TestProbeStrategyStateless(t *testing.T) {
	mgrFive := NewTraversalManager(nil, nil, nil, nil, nil)
	mgrFive.config = traversal.Config{}
	mgrFive.SetInterpolator(&mockInterpolator{})

	mgrSeven := NewTraversalManager(nil, nil, nil, nil, nil)
	mgrSeven.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	seven := &mockSevenInterpolator{loaded: true, result: seveninterp.InterpolationResult{IsValid: true, Alpha: 42}}
	mgrSeven.SetSevenHoleInterpolator(seven)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			if _, err := mgrFive.CalculateRealtimeByProbe("", probeCalcInput{}); err != nil {
				t.Errorf("five-hole dispatch: %v", err)
				return
			}
		}
	}()
	for i := 0; i < 50; i++ {
		res, err := mgrSeven.CalculateRealtimeByProbe(traversal.ProbeTypeSevenHole, probeCalcInput{})
		if err != nil {
			t.Fatalf("seven-hole dispatch: %v", err)
		}
		if res.Alpha != 42 {
			t.Fatalf("cross-talk detected: seven-hole manager got alpha=%v", res.Alpha)
		}
	}
	<-done
	if seven.calls != 50 {
		t.Errorf("seven-hole interpolator calls = %d, want 50", seven.calls)
	}
}

// TestCheckPreconditionsProbeAware CheckPreconditions 经策略表探针感知：
// 七孔配置未加载 -> PRB 项失败；已加载 -> 通过；五孔行为不变。
func TestCheckPreconditionsProbeAware(t *testing.T) {
	prbPassed := func(mgr *TraversalManager) bool {
		result := mgr.CheckPreconditions(nil)
		for _, c := range result["checks"].([]map[string]any) {
			if c["name"] == "PRB" {
				return c["passed"].(bool)
			}
		}
		t.Fatal("PRB check missing")
		return false
	}

	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	if prbPassed(mgr) {
		t.Error("seven-hole config without interpolator must fail PRB check")
	}
	// 仅注入五孔插值器：七孔配置仍不得通过（防陈旧校准误过前置检查）。
	mgr.SetInterpolator(&mockInterpolator{})
	if prbPassed(mgr) {
		t.Error("five-hole interpolator must not satisfy seven-hole config")
	}
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
	if !prbPassed(mgr) {
		t.Error("loaded seven-hole interpolator must pass PRB check")
	}

	five := NewTraversalManager(nil, nil, nil, nil, nil)
	five.config = traversal.Config{}
	five.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
	if prbPassed(five) {
		t.Error("seven-hole interpolator must not satisfy five-hole config")
	}
	five.SetInterpolator(&mockInterpolator{})
	if !prbPassed(five) {
		t.Error("five-hole behavior must be unchanged (legacy config)")
	}
}

// TestCheckPreconditionsRespectsRequestProbeType 双变体恢复场景的回归测试：
// 前端 activateProbeType 切换后尚未保存到后端（m.config.ProbeType 仍是旧值），
// 此时调用 checkPreconditions 必须按请求 config 的 ProbeType 判定，
// 否则切到已加载侧仍会按陈旧 m.config.ProbeType 误报"未加载 PRB"。
func TestCheckPreconditionsRespectsRequestProbeType(t *testing.T) {
	prbPassed := func(mgr *TraversalManager, cfg *traversal.Config) bool {
		result := mgr.CheckPreconditions(cfg)
		for _, c := range result["checks"].([]map[string]any) {
			if c["name"] == "PRB" {
				return c["passed"].(bool)
			}
		}
		t.Fatal("PRB check missing")
		return false
	}

	// 后端 m.config.ProbeType=five-hole（陈旧），但仅七孔插值器加载成功。
	// 场景：用户切换到七孔侧，前端 config.probeType=seven-hole 但尚未保存。
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeFiveHole}
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})

	// 不传 config：按陈旧 m.config.ProbeType=five-hole 判定，五孔未加载→失败
	if prbPassed(mgr, nil) {
		t.Error("nil config must fall back to m.config.ProbeType=five-hole and fail (five-hole not loaded)")
	}

	// 传入 config.ProbeType=seven-hole：必须按请求类型判定→七孔已加载→通过
	// 这是修复"切换探针类型后误报未加载 PRB"的关键路径
	sevenCfg := &traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	if !prbPassed(mgr, sevenCfg) {
		t.Error("config.ProbeType=seven-hole must pass (request probeType overrides stale m.config.ProbeType)")
	}

	// 反向场景：m.config.ProbeType=seven-hole（陈旧），仅五孔加载成功
	mgr2 := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr2.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	mgr2.SetInterpolator(&mockInterpolator{})

	if prbPassed(mgr2, nil) {
		t.Error("nil config must fall back to m.config.ProbeType=seven-hole and fail (seven-hole not loaded)")
	}
	fiveCfg := &traversal.Config{ProbeType: traversal.ProbeTypeFiveHole}
	if !prbPassed(mgr2, fiveCfg) {
		t.Error("config.ProbeType=five-hole must pass (request probeType overrides stale m.config.ProbeType)")
	}

	// 双变体都加载成功：任一类型传入都必须通过（覆盖最常见切换场景）
	mgr3 := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr3.config = traversal.Config{ProbeType: traversal.ProbeTypeFiveHole}
	mgr3.SetInterpolator(&mockInterpolator{})
	mgr3.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
	if !prbPassed(mgr3, &traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}) {
		t.Error("both loaded: seven-hole request must pass")
	}
	if !prbPassed(mgr3, &traversal.Config{ProbeType: traversal.ProbeTypeFiveHole}) {
		t.Error("both loaded: five-hole request must pass")
	}
}

// TestClearProbeInterpolator 五孔/七孔分别只清指定类型；未知类型报错；
// 清除后对应类型前置检查失败。
func TestClearProbeInterpolator(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.SetInterpolator(&mockInterpolator{})
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})

	if err := mgr.ClearProbeInterpolator(traversal.ProbeTypeSevenHole); err != nil {
		t.Fatalf("clear seven-hole: %v", err)
	}
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	if mgr.HasLoadedInterpolator() {
		t.Error("seven-hole interpolator must be cleared")
	}
	mgr.config = traversal.Config{}
	if !mgr.HasLoadedInterpolator() {
		t.Error("five-hole interpolator must be retained after clearing seven-hole")
	}

	if err := mgr.ClearProbeInterpolator(traversal.ProbeTypeFiveHole); err != nil {
		t.Fatalf("clear five-hole: %v", err)
	}
	if mgr.HasLoadedInterpolator() {
		t.Error("five-hole interpolator must be cleared")
	}

	if err := mgr.ClearProbeInterpolator("nine-hole"); err == nil {
		t.Fatal("unknown probe type must error")
	}
}

// TestInterpolatorIdentitySevenHole 七孔具体插值器经可选能力类型断言返回
// 稳定标识（含 7 个文件名信息），不得静默退化为空串。
func TestInterpolatorIdentitySevenHole(t *testing.T) {
	mgr := NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
	if got := mgr.interpolatorIdentity(); got != "" {
		t.Errorf("nil seven-hole interpolator identity = %q, want empty", got)
	}
	mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
	id := mgr.interpolatorIdentity()
	if id == "" {
		t.Fatal("seven-hole identity must not be empty (type assertion path)")
	}
	for _, name := range []string{"7.prb", "1.prb", "2.prb", "3.prb", "4.prb", "5.prb", "6.prb"} {
		if !strings.Contains(id, name) {
			t.Errorf("identity %q missing %q", id, name)
		}
	}
	if id != mgr.interpolatorIdentity() {
		t.Error("identity must be stable")
	}

	// 五孔配置不受七孔插值器影响。
	five := NewTraversalManager(nil, nil, nil, nil, nil)
	five.config = traversal.Config{}
	five.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
	if got := five.interpolatorIdentity(); got != "" {
		t.Errorf("five-hole config must not read seven-hole identity, got %q", got)
	}
}
