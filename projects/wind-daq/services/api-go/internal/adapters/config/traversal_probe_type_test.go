package config

import (
	"encoding/json"
	"testing"

	"wind-daq/services/api-go/internal/core/traversal"
)

// 七孔探针 ProbeType 字段的 JSON 序列化契约测试。
//
// 从 core/traversal/probe_type_test.go 迁移而来：core 层只定义 struct 和常量，
// JSON 序列化/反序列化行为属于 I/O 契约，由 adapters/config 层验证。
// 与 calibration_config_decoder_test.go 风格一致。

// TestProbeTypeLegacyConfig verifies that a legacy config JSON without the
// probeType field deserializes as five-hole (backward compatibility).
func TestProbeTypeLegacyConfig(t *testing.T) {
	legacy := `{"taskId":"t1","deviceId":"dev","channels":[0],"path":[],"dwellTimeMs":100,"samplesPerPoint":10}`
	var c traversal.Config
	if err := json.Unmarshal([]byte(legacy), &c); err != nil {
		t.Fatalf("unmarshal legacy config: %v", err)
	}
	if c.ProbeType != "" {
		t.Errorf("legacy ProbeType = %q, want empty (pre-normalization)", c.ProbeType)
	}
	if c.IsSevenHole() {
		t.Error("legacy config must not be seven-hole")
	}
}

// TestProbeTypeParsing covers explicit values and JSON round-trip.
func TestProbeTypeParsing(t *testing.T) {
	var c traversal.Config
	if err := json.Unmarshal([]byte(`{"taskId":"t","probeType":"seven-hole"}`), &c); err != nil {
		t.Fatal(err)
	}
	if !c.IsSevenHole() {
		t.Error("probeType=seven-hole must report IsSevenHole")
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var round struct {
		ProbeType string `json:"probeType"`
	}
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if round.ProbeType != "seven-hole" {
		t.Errorf("round-trip probeType = %q, want seven-hole", round.ProbeType)
	}

	var five traversal.Config
	if err := json.Unmarshal([]byte(`{"taskId":"t","probeType":"five-hole"}`), &five); err != nil {
		t.Fatal(err)
	}
	if five.IsSevenHole() {
		t.Error("probeType=five-hole must not report IsSevenHole")
	}
	// Empty probeType is omitted from JSON (legacy-shaped output).
	out, err := json.Marshal(traversal.Config{TaskID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["probeType"]; ok {
		t.Error("empty probeType must be omitted from JSON")
	}
}
