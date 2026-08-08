package interpolation

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestInterpolatorInterfaceShape locks the interface to the exact five-hole
// shape (IsLoaded / GetValidRange / Calculate, three methods). Identity must
// NOT be part of the interface; it is an optional capability implemented by
// the concrete type only (spec-seven-hole-traversal section 2.2).
func TestInterpolatorInterfaceShape(t *testing.T) {
	iface := reflect.TypeOf((*Interpolator)(nil)).Elem()
	// reflect returns interface methods sorted lexicographically.
	want := []string{"Calculate", "GetValidRange", "IsLoaded"}
	if iface.NumMethod() != len(want) {
		t.Fatalf("Interpolator method count = %d, want %d", iface.NumMethod(), len(want))
	}
	for i, name := range want {
		if m := iface.Method(i); m.Name != name {
			t.Errorf("method[%d] = %s, want %s", i, m.Name, name)
		}
	}
	if _, ok := iface.MethodByName("Identity"); ok {
		t.Error("Interpolator must not contain Identity; it is an optional capability of the concrete type")
	}
}

// TestConcreteIdentityCapability asserts the concrete type exposes Identity()
// via the optional capability interface and that the identity is stable.
func TestConcreteIdentityCapability(t *testing.T) {
	var _ interface{ Identity() string } = (*SevenHolePrbInterpolator)(nil)

	p := &SevenHolePrbInterpolator{}
	first := p.Identity()
	if first == "" {
		t.Error("Identity() returned empty string")
	}
	if got := p.Identity(); got != first {
		t.Errorf("Identity() not stable: %q vs %q", first, got)
	}
}

func jsonKeys(t *testing.T, v any) map[string]bool {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make(map[string]bool, len(m))
	for k := range m {
		keys[k] = true
	}
	return keys
}

// TestInterpolationResultJSONTags locks the JSON contract shared with the
// five-hole package; the "P0"/"Ps" tags are load-bearing for API response
// and CSV computed-column reuse (spec-seven-hole-traversal section 2.2).
//
// Theta/Phi 是 7 孔新增字段（PRB 网格原始角度坐标），与 5 孔结果不共享，
// 但JSON tag 与 Alpha/Beta 同形态，便于前端结果表格组件复用列定义。
func TestInterpolationResultJSONTags(t *testing.T) {
	keys := jsonKeys(t, InterpolationResult{})
	want := []string{"alpha", "beta", "theta", "phi", "machNumber", "velocity", "dynamicPressure", "P0", "Ps", "isValid"}
	if len(keys) != len(want) {
		t.Errorf("key count = %d (%v), want %d", len(keys), keys, len(want))
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("missing JSON key %q", k)
		}
	}
	if keys["warning"] {
		t.Error("warning must be omitted when empty (omitempty)")
	}
}

// TestInterpolationInputJSONTags locks the nine-field input contract
// (P1..P7 + Patm + Tatm) used by the API decoding layer.
func TestInterpolationInputJSONTags(t *testing.T) {
	keys := jsonKeys(t, InterpolationInput{})
	want := []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7", "Patm", "Tatm"}
	if len(keys) != len(want) {
		t.Errorf("key count = %d (%v), want %d", len(keys), keys, len(want))
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("missing JSON key %q", k)
		}
	}
}

// TestPrbValidRangeJSONTags mirrors the five-hole PrbValidRange JSON shape.
func TestPrbValidRangeJSONTags(t *testing.T) {
	keys := jsonKeys(t, PrbValidRange{})
	want := []string{"alphaMin", "alphaMax", "betaMin", "betaMax", "machMin", "machMax"}
	if len(keys) != len(want) {
		t.Errorf("key count = %d (%v), want %d", len(keys), keys, len(want))
	}
	for _, k := range want {
		if !keys[k] {
			t.Errorf("missing JSON key %q", k)
		}
	}
}
