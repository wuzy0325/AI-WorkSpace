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
