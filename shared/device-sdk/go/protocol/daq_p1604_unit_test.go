package protocol

import (
	"encoding/binary"
	"math"
	"net"
	"strings"
	"testing"
	"time"
)

// writeFrame 写入一个带 2 字节大端长度前缀的帧（模拟设备响应）
func writeFrame(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	frameLen := uint16(len(payload) + 2)
	buf := make([]byte, 2, int(frameLen))
	binary.BigEndian.PutUint16(buf, frameLen)
	buf = append(buf, []byte(payload)...)
	if _, err := conn.Write(buf); err != nil {
		t.Logf("write frame payload=%q: %v", payload, err)
	}
}

func TestP1604MatchUnitByCoefficient_KnownUnits(t *testing.T) {
	cases := []struct {
		unit string
	}{
		{"psi"}, {"Pa"}, {"kPa"}, {"MPa"}, {"kgf/cm²"},
	}
	for _, c := range cases {
		coeff := P1604PressureUnitCoefficient[c.unit]
		got, ok := P1604MatchUnitByCoefficient(coeff)
		if !ok {
			t.Errorf("unit %s (coeff=%v): expected match, got not found", c.unit, coeff)
			continue
		}
		if got != c.unit {
			t.Errorf("unit %s (coeff=%v): expected %q, got %q", c.unit, coeff, c.unit, got)
		}
	}
}

// TestP1604MatchUnitByCoefficient_Float32Precision 验证设备以 float32 存储系数带来的精度损失
// 仍能正确匹配。例如 Pa 系数 6894.757 在 float32 中为 6894.756836。
func TestP1604MatchUnitByCoefficient_Float32Precision(t *testing.T) {
	cases := []struct {
		name     string
		coeff    float64
		expected string
	}{
		{"Pa float32 loss", 6894.756836, "Pa"},
		{"psi exact", 1.000000, "psi"},
		{"kPa float32 loss", 6.894757, "kPa"},
	}
	for _, c := range cases {
		got, ok := P1604MatchUnitByCoefficient(c.coeff)
		if !ok {
			t.Errorf("%s (coeff=%v): expected match, got not found", c.name, c.coeff)
			continue
		}
		if got != c.expected {
			t.Errorf("%s (coeff=%v): expected %q, got %q", c.name, c.coeff, c.expected, got)
		}
	}
}

func TestP1604MatchUnitByCoefficient_UnknownCoeff(t *testing.T) {
	got, ok := P1604MatchUnitByCoefficient(12345.6789)
	if ok {
		t.Errorf("expected no match for unknown coeff, got %q", got)
	}
}

func TestP1604IsSupportedUnit(t *testing.T) {
	if !P1604IsSupportedUnit("psi") {
		t.Error("psi should be supported")
	}
	if !P1604IsSupportedUnit("kgf/cm²") {
		t.Error("kgf/cm² should be supported")
	}
	if P1604IsSupportedUnit("bar") {
		t.Error("bar should not be supported")
	}
	if P1604IsSupportedUnit("") {
		t.Error("empty string should not be supported")
	}
}

func TestP1604ReadUnitCoefficient_Valid(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 模拟设备端：读取 u01101 命令，返回系数 6.894757（kPa）
	go func() {
		// 先读客户端发来的 u01101 命令（纯 ASCII，无换行符）
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 返回 kPa 系数
		writeFrame(t, server, "6.894757")
	}()

	fr := NewFrameReader(client)
	coeff, err := P1604ReadUnitCoefficient(fr, client, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if coeff != 6.894757 {
		t.Errorf("expected 6.894757, got %v", coeff)
	}
}

func TestP1604ReadUnitCoefficient_ZeroTimeoutDoesNotSetDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tracked := &deadlineTrackingConn{Conn: client}
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "1.000000")
	}()

	coeff, err := P1604ReadUnitCoefficient(NewFrameReader(tracked), tracked, 0)
	if err != nil {
		t.Fatalf("read coefficient without deadline: %v", err)
	}
	if coeff != 1 {
		t.Fatalf("coefficient = %v, want 1", coeff)
	}
	if tracked.readDeadlineCalls != 0 || tracked.writeDeadlineCalls != 0 {
		t.Fatalf("deadline calls: read=%d write=%d, want 0", tracked.readDeadlineCalls, tracked.writeDeadlineCalls)
	}
}

func TestP1604ReadUnitCoefficient_RejectsMultiValueResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "6894.756836 0.000000 0.000000 0.000000 0.000000")
	}()

	fr := NewFrameReader(client)
	if _, err := P1604ReadUnitCoefficient(fr, client, time.Second); err == nil {
		t.Fatal("expected error for multi-value response")
	}
}

func TestP1604ReadUnitCoefficient_DeviceError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 设备返回 N05 错误
		writeFrame(t, server, "N05")
	}()

	fr := NewFrameReader(client)
	_, err := P1604ReadUnitCoefficient(fr, client, time.Second)
	if err == nil {
		t.Fatal("expected error for device N05 response")
	}
	if !strings.Contains(err.Error(), "N05") {
		t.Errorf("error should mention N05, got: %v", err)
	}
}

func TestP1604ReadUnitCoefficient_InvalidNumber(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "abc-not-a-number")
	}()

	fr := NewFrameReader(client)
	_, err := P1604ReadUnitCoefficient(fr, client, time.Second)
	if err == nil {
		t.Fatal("expected error for non-numeric response")
	}
}

func TestP1604ReadUnitCoefficient_NilArgs(t *testing.T) {
	_, err := P1604ReadUnitCoefficient(nil, nil, time.Second)
	if err == nil {
		t.Fatal("expected error for nil reader/conn")
	}
}

func TestP1604WriteUnitCoefficient_Success(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 设备返回 A 表示成功
		writeFrame(t, server, "A")
	}()

	fr := NewFrameReader(client)
	if err := P1604WriteUnitCoefficient(fr, client, 6894.757, time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestP1604WriteUnitCoefficient_DeviceReject(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "N07")
	}()

	fr := NewFrameReader(client)
	err := P1604WriteUnitCoefficient(fr, client, 6894.757, time.Second)
	if err == nil {
		t.Fatal("expected error for device N07 rejection")
	}
	if !strings.Contains(err.Error(), "N07") {
		t.Errorf("error should mention N07, got: %v", err)
	}
}

func TestP1604WriteUnitCoefficient_RejectsUnexpectedResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		writeFrame(t, server, "A ")
	}()

	err := P1604WriteUnitCoefficient(NewFrameReader(client), client, 6894.757, time.Second)
	if err == nil || !strings.Contains(err.Error(), "unexpected v01101 response") {
		t.Fatalf("expected strict response error, got %v", err)
	}
}

func TestP1604WriteUnitCoefficient_InvalidCoeff(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	fr := NewFrameReader(client)
	// 负数、零、NaN、+Inf 均应被拒绝
	cases := []float64{-1.0, 0.0, math.NaN(), math.Inf(1)}
	for _, c := range cases {
		err := P1604WriteUnitCoefficient(fr, client, c, time.Second)
		if err == nil {
			t.Errorf("expected error for invalid coeff %v", c)
		}
	}
}
