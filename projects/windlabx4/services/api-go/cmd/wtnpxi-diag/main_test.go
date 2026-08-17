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

func TestParseFrame_SingleValid(t *testing.T) {
	frame := makeWTNPXIFrame(1.5, 2.5, 3.5, 4.5, 5.5, 6.5, 7.5, 8.5)
	payload, consumed, err := parseFrame(frame)
	if err != nil {
		t.Fatalf("parseFrame error = %v, want nil", err)
	}
	if consumed != len(frame) {
		t.Fatalf("consumed = %d, want %d", consumed, len(frame))
	}
	vals := decodePayload(payload)
	if len(vals) != 8 {
		t.Fatalf("values = %d, want 8", len(vals))
	}
	if vals[0] != 1.5 || vals[7] != 8.5 {
		t.Fatalf("values = %v, want [1.5 ... 8.5]", vals)
	}
}

func TestParseFrame_TwoFramesInOneBuffer(t *testing.T) {
	f1 := makeWTNPXIFrame(1, 2, 3, 4, 5, 6, 7, 8)
	f2 := makeWTNPXIFrame(9, 10, 11, 12, 13, 14, 15, 16)
	buf := append(append([]byte{}, f1...), f2...)

	p1, c1, err := parseFrame(buf)
	if err != nil || c1 != len(f1) {
		t.Fatalf("first frame: payload=%v consumed=%d err=%v", p1, c1, err)
	}
	p2, c2, err := parseFrame(buf[c1:])
	if err != nil || c2 != len(f2) {
		t.Fatalf("second frame: payload=%v consumed=%d err=%v", p2, c2, err)
	}
}

func TestParseFrame_PartialFrame(t *testing.T) {
	frame := makeWTNPXIFrame(1, 2, 3, 4, 5, 6, 7, 8)
	partial := frame[:len(frame)-3]
	payload, consumed, err := parseFrame(partial)
	if err != io.ErrShortBuffer {
		t.Fatalf("err = %v, want ErrShortBuffer", err)
	}
	if payload != nil || consumed != 0 {
		t.Fatalf("payload=%v consumed=%d, want nil/0", payload, consumed)
	}
}

func TestParseFrame_TooShort(t *testing.T) {
	if _, _, err := parseFrame([]byte{0x00, 0x01}); err != io.ErrShortBuffer {
		t.Fatalf("err = %v, want ErrShortBuffer", err)
	}
}

func TestParseFrame_InvalidPrefixZero(t *testing.T) {
	_, consumed, err := parseFrame([]byte{0, 0, 0, 0, 1, 2, 3})
	if err != errResync || consumed != 1 {
		t.Fatalf("err = %v consumed = %d, want errResync/1", err, consumed)
	}
}

func TestParseFrame_InvalidPrefixTooLong(t *testing.T) {
	_, consumed, err := parseFrame([]byte{0x00, 0x02, 0x00, 0x00, 1, 2, 3})
	if err != errResync || consumed != 1 {
		t.Fatalf("err = %v consumed = %d, want errResync/1", err, consumed)
	}
}

func TestParseFrame_ResyncRecovers(t *testing.T) {
	// 在真实帧前混入 2 个垃圾字节，验证逐字节重同步后能恢复解析。
	frame := makeWTNPXIFrame(1, 2, 3, 4, 5, 6, 7, 8)
	buf := append([]byte{0xAB, 0xCD}, frame...)

	var payload []byte
	consumed := 0
	for len(buf) > 0 {
		p, c, err := parseFrame(buf)
		if err == io.ErrShortBuffer {
			t.Fatal("unexpected short buffer")
		}
		if err == errResync {
			buf = buf[c:]
			continue
		}
		payload, consumed = p, c
		break
	}
	if payload == nil {
		t.Fatal("failed to recover frame after resync")
	}
	if consumed != len(frame) {
		t.Fatalf("consumed = %d, want %d", consumed, len(frame))
	}
}

func TestDecodePayload_NotMultipleOfFour(t *testing.T) {
	vals := decodePayload([]byte{0x01, 0x02, 0x03})
	if vals != nil {
		t.Fatalf("vals = %v, want nil", vals)
	}
}

func TestParseHex(t *testing.T) {
	b, ok := parseHex("00 01 02 ff")
	if !ok || len(b) != 4 || b[0] != 0 || b[3] != 0xFF {
		t.Fatalf("parseHex(space) = %v, %v", b, ok)
	}
	b, ok = parseHex("000102ff")
	if !ok || len(b) != 4 {
		t.Fatalf("parseHex(nospace) = %v, %v", b, ok)
	}
	if _, ok = parseHex("zz"); ok {
		t.Fatal("parseHex(invalid) should be false")
	}
	if _, ok = parseHex("123"); ok {
		t.Fatal("parseHex(odd) should be false")
	}
}
