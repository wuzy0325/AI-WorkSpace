package main

import (
	"encoding/binary"
	"io"
	"math"
	"testing"
)

func makeWTNPXIFrame(vals ...float32) []byte {
	payload := make([]byte, len(vals)*4)
	for i, v := range vals {
		binary.LittleEndian.PutUint32(payload[i*4:], math.Float32bits(v))
	}
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(len(payload)))
	return append(prefix, payload...)
}

// makeFloat64Frame 构造真实设备数据帧：2 字节协议前缀 + 4 字节大端数组长度 + N × double。
func makeFloat64Frame(vals ...float64) []byte {
	payload := make([]byte, 6+len(vals)*8)
	binary.BigEndian.PutUint32(payload[2:6], uint32(len(vals)))
	for i, v := range vals {
		binary.BigEndian.PutUint64(payload[6+i*8:], math.Float64bits(v))
	}
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(len(payload)))
	return append(prefix, payload...)
}

func TestGUIParseFrame_Valid(t *testing.T) {
	frame := makeWTNPXIFrame(1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5)
	payload, consumed, err := parseFrame(frame)
	if err != nil || consumed != len(frame) {
		t.Fatalf("err=%v consumed=%d want nil/%d", err, consumed, len(frame))
	}
	vals := decodePayload(payload)
	if len(vals) != 8 || vals[0] != 1.5 || vals[7] != 8.5 {
		t.Fatalf("vals=%v", vals)
	}
}

func TestGUIParseFrame_PartialAndResync(t *testing.T) {
	frame := makeWTNPXIFrame(1, 2, 3, 4, 5, 6, 7, 8)
	if _, _, err := parseFrame(frame[:len(frame)-3]); err != io.ErrShortBuffer {
		t.Fatalf("partial err=%v want ErrShortBuffer", err)
	}
	if _, c, err := parseFrame([]byte{0, 0, 0, 0, 1, 2, 3}); err != errResync || c != 1 {
		t.Fatalf("resync err=%v c=%d want errResync/1", err, c)
	}
}

func TestGUIParseHex(t *testing.T) {
	b, ok := parseHex("00 01 02 ff")
	if !ok || len(b) != 4 || b[3] != 0xFF {
		t.Fatalf("hex=%v ok=%v", b, ok)
	}
	if _, ok := parseHex("zz"); ok {
		t.Fatal("invalid hex should be rejected")
	}
}

func TestGUIFormatValue(t *testing.T) {
	if s := formatValue(101325.0); s != "101325.00" {
		t.Fatalf("got %q", s)
	}
	if s := formatValue(0.0000001); s == "0.00" {
		t.Fatalf("tiny value collapsed: %q", s)
	}
}

// TestGUIDecodeFloat64Frame_RealDevice 复现 temp/log.txt 帧 #2：
// 102B payload，float64 大端数据帧，应解析出 12 路通道值。
func TestGUIDecodeFloat64Frame_RealDevice(t *testing.T) {
	l1 := "00000000000C3FF4AB277C00000740F8"
	l2 := "74837CFEFFFD40F8586A20C080060000"
	l3 := "000000000000C0F07BC90B11E7FAC0F0"
	l4 := "7BC90B11E7FAC0F07BC90B11E7FAC0F0"
	l5 := "7BC90B11E7FAC0F07A42619F8A4EC0F0"
	l6 := "7A42619F8A4E40A4158BEE1C810940A4"
	l7 := "158BEE1C8109"
	payload := decodeHexOrFatal(t, l1+l2+l3+l4+l5+l6+l7)

	if len(payload) != 102 {
		t.Fatalf("payload len=%d want 102", len(payload))
	}
	vals := decodePayload(payload)
	if len(vals) != 12 {
		t.Fatalf("vals=%v want 12 values", vals)
	}
	want := []float64{
		1.2917857021093384, 100168.2180166244, 99718.6329960824, 0.0,
		-67516.56520262352, -67516.56520262352, -67516.56520262352, -67516.56520262352,
		-67492.14883379007, -67492.14883379007, 2570.773301020385, 2570.773301020385,
	}
	for i := range want {
		if math.Abs(vals[i]-want[i]) > 1e-6 {
			t.Fatalf("vals[%d]=%v want %v", i, vals[i], want[i])
		}
	}
}

// TestGUIDecodeInfoFrame_RealDevice 复现 temp/log.txt 帧 #1：
// 20B payload 为设备信息帧（TLV，含 "crio"），不应被当作通道值。
func TestGUIDecodeInfoFrame_RealDevice(t *testing.T) {
	payload := decodeHexOrFatal(t, "0000000200000004D1B9C1A6000000046372696F")
	if len(payload) != 20 {
		t.Fatalf("payload len=%d want 20", len(payload))
	}
	info := parseInfoFrame(payload)
	if info == "" {
		t.Fatal("expected info frame, got empty")
	}
	vals := decodePayload(payload)
	if len(vals) != 0 {
		t.Fatalf("info frame should yield no channel values, got %v", vals)
	}
}

// TestGUIDecodeFloat32Frame_Fallback 确认旧 float32 小端帧仍可兜底解析。
func TestGUIDecodeFloat32Frame_Fallback(t *testing.T) {
	frame := makeWTNPXIFrame(1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5)
	payload, _, err := parseFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	vals := decodePayload(payload)
	if len(vals) != 8 || vals[0] != 1.5 {
		t.Fatalf("vals=%v", vals)
	}
}

func decodeHexOrFatal(t *testing.T, s string) []byte {
	t.Helper()
	b, ok := parseHex(s)
	if !ok {
		t.Fatalf("bad hex %q", s)
	}
	return b
}
