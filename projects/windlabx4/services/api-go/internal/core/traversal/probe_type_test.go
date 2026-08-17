package traversal

import "testing"

// TestProbeTypeConstants locks the probe type string values against
// spec-seven-hole-traversal section 2.3 (front/back-end contract).
//
// JSON 序列化/反序列化契约测试已迁移至 adapters/config 层
//（traversal_probe_type_test.go），core 层只保留常量值校验。
func TestProbeTypeConstants(t *testing.T) {
	if ProbeTypeFiveHole != "five-hole" {
		t.Errorf("ProbeTypeFiveHole = %q, want %q", ProbeTypeFiveHole, "five-hole")
	}
	if ProbeTypeSevenHole != "seven-hole" {
		t.Errorf("ProbeTypeSevenHole = %q, want %q", ProbeTypeSevenHole, "seven-hole")
	}
}
