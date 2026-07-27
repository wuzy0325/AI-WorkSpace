package usecase

import (
	"encoding/json"
	"errors"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
)

// TestClassifyCalculatedResult 三态 Status 填充矩阵。
//
// 测试前置:
//   - probeCalcResult.IsValid/Alpha/Beta/Pt/Ps/Mach 字段可被测试代码直接构造
//   - classifyCalculatedResult 为纯函数,无 manager 依赖
//
// 测试步骤:逐场景构造输入,断言返回的 Valid/Status/数值字段
// 期待结果:5 种路径分别命中 Valid/PrbMissing/Invalid,数值仅在 Valid 时透传
func TestClassifyCalculatedResult(t *testing.T) {
	cases := []struct {
		name               string
		strategyOK         bool
		hasAll             bool
		interpRes          probeCalcResult
		interpErr          error
		interpolatorLoaded bool
		wantValid          bool
		wantStatus         traversal.CalcStatus
	}{
		// ===== PrbMissing 路径(配置层问题,3 种触发条件)=====
		{
			name:       "策略未注册 → PrbMissing",
			strategyOK: false,
			wantStatus: traversal.CalcStatusPrbMissing,
		},
		{
			name:       "通道不全 → PrbMissing",
			strategyOK: true,
			hasAll:     false,
			wantStatus: traversal.CalcStatusPrbMissing,
		},
		{
			name:               "插值器未加载 + err → PrbMissing",
			strategyOK:         true,
			hasAll:             true,
			interpErr:          errors.New("PRB not loaded"),
			interpolatorLoaded: false,
			wantStatus:         traversal.CalcStatusPrbMissing,
		},
		// ===== Invalid 路径(数据层问题,2 种触发条件)=====
		{
			name:               "已加载但 err != nil → Invalid",
			strategyOK:         true,
			hasAll:             true,
			interpErr:          errors.New("input NaN"),
			interpolatorLoaded: true,
			wantStatus:         traversal.CalcStatusInvalid,
		},
		{
			name:               "已加载但 IsValid=false → Invalid",
			strategyOK:         true,
			hasAll:             true,
			interpRes:          probeCalcResult{IsValid: false},
			interpolatorLoaded: true,
			wantStatus:         traversal.CalcStatusInvalid,
		},
		// ===== Valid 路径(成功)=====
		{
			name:               "IsValid=true → Valid + 数值透传",
			strategyOK:         true,
			hasAll:             true,
			interpRes: probeCalcResult{
				IsValid: true, Alpha: 15.32, Beta: -2.10,
				Pt: 101325, Ps: 99800, Mach: 0.325,
			},
			interpolatorLoaded: true,
			wantValid:          true,
			wantStatus:         traversal.CalcStatusValid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCalculatedResult(
				tc.strategyOK,
				tc.hasAll,
				tc.interpRes,
				tc.interpErr,
				tc.interpolatorLoaded,
			)
			if got == nil {
				t.Fatal("classifyCalculatedResult returned nil")
			}
			if got.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", got.Valid, tc.wantValid)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tc.wantStatus)
			}
			if tc.wantValid {
				// Valid 路径:完整断言所有数值字段,防止字段错配(如 Beta 被填成 Alpha)静默通过
				if got.Alpha != tc.interpRes.Alpha {
					t.Errorf("Alpha = %v, want %v", got.Alpha, tc.interpRes.Alpha)
				}
				if got.Beta != tc.interpRes.Beta {
					t.Errorf("Beta = %v, want %v", got.Beta, tc.interpRes.Beta)
				}
				if got.Pt != tc.interpRes.Pt {
					t.Errorf("Pt = %v, want %v", got.Pt, tc.interpRes.Pt)
				}
				if got.Ps != tc.interpRes.Ps {
					t.Errorf("Ps = %v, want %v", got.Ps, tc.interpRes.Ps)
				}
				if got.Mach != tc.interpRes.Mach {
					t.Errorf("Mach = %v, want %v", got.Mach, tc.interpRes.Mach)
				}
			} else {
				// 失败路径:数值字段必须为零值,防止误填(如复制 Valid 分支赋值但漏改条件)
				// 与 Valid 路径断言对称,确保失败路径不会"漏出"数值
				if got.Alpha != 0 || got.Beta != 0 || got.Pt != 0 || got.Ps != 0 || got.Mach != 0 {
					t.Errorf("失败路径数值字段必须为零值, got: Alpha=%v Beta=%v Pt=%v Ps=%v Mach=%v",
						got.Alpha, got.Beta, got.Pt, got.Ps, got.Mach)
				}
			}
		})
	}
}

// TestCalculatedResultJSONSerialization JSON 契约:验证 Status 字段序列化行为。
//
// 测试前置:
//   - CalculatedResult 已新增 Status CalcStatus 字段(json:"status,omitempty")
//   - 失败路径 Valid=false 时 Status 必填;成功路径 Valid=true 时 Status 也必填
//
// 测试步骤:构造三种状态的 CalculatedResult,JSON 序列化后反序列化到 map 断言字段
// 期待结果:
//   - Valid=true:JSON 含 valid=true + status="valid" + 数值字段(真实值)
//   - Valid=false + PrbMissing:JSON 含 valid=false + status="prb_missing" + 数值字段为零值
//   - Valid=false + Invalid:JSON 含 valid=false + status="invalid" + 数值字段为零值
//
// 注意:Go json 默认序列化 float64 零值为 0(不是省略字段);前端用 valid/status 判定状态,
// 数值字段在失败路径虽存在但为 0,前端不应在 valid=false 时读取数值列。
func TestCalculatedResultJSONSerialization(t *testing.T) {
	t.Run("Valid=true 完整序列化", func(t *testing.T) {
		c := &traversal.CalculatedResult{
			Valid:  true,
			Status: traversal.CalcStatusValid,
			Alpha:  15.32,
			Beta:   -2.10,
			Pt:     101325,
			Ps:     99800,
			Mach:   0.325,
		}
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// 必须含 valid + status + 数值字段
		for _, key := range []string{"valid", "status", "alpha", "beta", "pt", "ps", "mach"} {
			if _, ok := m[key]; !ok {
				t.Errorf("missing field %q in JSON: %s", key, raw)
			}
		}
		if m["status"] != "valid" {
			t.Errorf("status = %v, want \"valid\"", m["status"])
		}
	})

	t.Run("PrbMissing 数值字段为零值", func(t *testing.T) {
		c := &traversal.CalculatedResult{
			Valid:  false,
			Status: traversal.CalcStatusPrbMissing,
		}
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m["valid"] != false {
			t.Errorf("valid = %v, want false", m["valid"])
		}
		if m["status"] != "prb_missing" {
			t.Errorf("status = %v, want \"prb_missing\"", m["status"])
		}
		// Go json 默认输出 float64 零值为 0,显式断言防止未来改为 omitempty 后契约悄悄变化
		for _, key := range []string{"alpha", "beta", "pt", "ps", "mach"} {
			if v, ok := m[key]; !ok || v != float64(0) {
				t.Errorf("%s = %v, want 0 (zero value)", key, v)
			}
		}
	})

	t.Run("Invalid 数值字段为零值", func(t *testing.T) {
		c := &traversal.CalculatedResult{
			Valid:  false,
			Status: traversal.CalcStatusInvalid,
		}
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if m["valid"] != false {
			t.Errorf("valid = %v, want false", m["valid"])
		}
		if m["status"] != "invalid" {
			t.Errorf("status = %v, want \"invalid\"", m["status"])
		}
		for _, key := range []string{"alpha", "beta", "pt", "ps", "mach"} {
			if v, ok := m[key]; !ok || v != float64(0) {
				t.Errorf("%s = %v, want 0 (zero value)", key, v)
			}
		}
	})
}

// TestHasLoadedInterpolatorFor 验证 HasLoadedInterpolatorFor 在不同探针类型/加载状态下的判定。
//
// 测试前置:mockInterpolator.IsLoaded() 返回 true;未 SetInterpolator 时 manager.interpolator==nil
//
// 测试步骤:
//   - 五孔已加载 → true
//   - 五孔未加载(不调用 SetInterpolator) → false
//   - 七孔已加载 → true
//   - 七孔未加载 → false
//   - 未知探针类型 → false
//
// 期待结果:与 strategy.isLoaded(m) 一致,且未知类型回退 false
func TestHasLoadedInterpolatorFor(t *testing.T) {
	t.Run("五孔已加载", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeFiveHole}
		mgr.SetInterpolator(&mockInterpolator{})
		if !mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeFiveHole) {
			t.Error("five-hole must report loaded after SetInterpolator")
		}
	})

	t.Run("五孔未加载", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeFiveHole}
		if mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeFiveHole) {
			t.Error("five-hole must report not-loaded without SetInterpolator")
		}
	})

	t.Run("七孔已加载", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
		mgr.SetSevenHoleInterpolator(&mockSevenInterpolator{loaded: true})
		if !mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeSevenHole) {
			t.Error("seven-hole must report loaded after SetSevenHoleInterpolator")
		}
	})

	t.Run("七孔未加载", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.config = traversal.Config{ProbeType: traversal.ProbeTypeSevenHole}
		if mgr.HasLoadedInterpolatorFor(traversal.ProbeTypeSevenHole) {
			t.Error("seven-hole must report not-loaded without SetSevenHoleInterpolator")
		}
	})

	t.Run("未知探针类型 → false", func(t *testing.T) {
		mgr := NewTraversalManager(nil, nil, nil, nil, nil)
		if mgr.HasLoadedInterpolatorFor("unknown-type") {
			t.Error("unknown probe type must return false, not panic")
		}
	})
}
