package driver

import (
	"strings"
	"testing"
)

// TestIsWTN1604NACK 校验 N 错误码识别规则。
// 协议：N 开头 + 纯数字（如 N09、N03、N123）视作设备拒绝。
func TestIsWTN1604NACK(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"N09", true},
		{"N3", true},
		{"N123", true},
		{"N", false},   // 单字母不算
		{"A", false},   // 成功响应
		{"N0A", false}, // 含非数字
		{"NACK", false},
		{"", false},
		{"3", false}, // 阀门状态数字本身不算错误
	}
	for _, c := range cases {
		got := isWTN1604NACK(c.input)
		if got != c.want {
			t.Errorf("isWTN1604NACK(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

// TestParseValveReadResponse 覆盖读阀响应所有分支：
// NACK 拒绝 / 数字 1 / 数字 2,3 / 数字 0 / 文本同义词 / 未识别。
func TestParseValveReadResponse(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		want     string
		wantErr  bool
		errMatch string
	}{
		{"NACK_N09", "N09", "", true, "device rejected"},
		{"NACK_N3", "N3", "", true, "device rejected"},
		{"digit_1_calibration", "A1", ValveStateCalibration, false, ""},
		{"digit_1_no_prefix", "1", ValveStateCalibration, false, ""},
		{"digit_2_measurement", "A2", ValveStateMeasurement, false, ""},
		{"digit_3_measurement_run_state", "A3", ValveStateMeasurement, false, ""},
		{"digit_0_unknown", "A0", ValveStateUnknown, false, ""},
		{"text_calibration", "calibration", ValveStateCalibration, false, ""},
		{"text_measurement", "measurement", ValveStateMeasurement, false, ""},
		{"text_unknown_returns_unknown", "garbage", ValveStateUnknown, false, ""},
		{"empty_returns_unknown", "", ValveStateUnknown, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseValveReadResponse(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if c.errMatch != "" && !strings.Contains(err.Error(), c.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), c.errMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("parseValveReadResponse(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// TestValveSetCommandFor 校验业务态到命令字的映射。
func TestValveSetCommandFor(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"calibration", "w0C01", false},
		{"Calibration", "w0C01", false},
		{"1", "w0C01", false},
		{"measurement", "w0C00", false},
		{"2", "w0C00", false},
		{"unknown", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := valveSetCommandFor(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("valveSetCommandFor(%q) expected error", c.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("valveSetCommandFor(%q) unexpected err: %v", c.input, err)
		}
		if got != c.want {
			t.Errorf("valveSetCommandFor(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestInterpretValveSetResponse 覆盖写阀响应三类分支：
// A 成功 / Nxx 设备拒绝 / 其他协议异常。
func TestInterpretValveSetResponse(t *testing.T) {
	if err := interpretValveSetResponse("w0C01", "A"); err != nil {
		t.Errorf("\"A\" should be success, got err: %v", err)
	}

	err := interpretValveSetResponse("w0C01", "N09")
	if err == nil || !strings.Contains(err.Error(), "device rejected valve command w0C01: N09") {
		t.Errorf("N09 should map to device rejected error, got: %v", err)
	}

	err = interpretValveSetResponse("w0C01", "weird")
	if err == nil || !strings.Contains(err.Error(), `set valve status failed: response "weird"`) {
		t.Errorf("unexpected response should map to protocol error, got: %v", err)
	}
}
