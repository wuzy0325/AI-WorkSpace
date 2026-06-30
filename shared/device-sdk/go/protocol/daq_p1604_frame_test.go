package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"net"
	"testing"
)

func TestParseStreamFrame_Valid(t *testing.T) {
	data := make([]byte, 77)
	data[0] = 0x01 // 协议规定二进制流帧头第 0 字节固定为 0x01
	for i := 0; i < 18; i++ {
		f := float32(i + 1)
		binary.BigEndian.PutUint32(data[5+i*4:], math.Float32bits(f))
	}

	channels, err := ParseStreamFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(channels) != 18 {
		t.Fatalf("expected 18 channels, got %d", len(channels))
	}

	for i := 0; i < 16; i++ {
		expected := float64(16 - i)
		if channels[i] != expected {
			t.Errorf("pressure channel %d: expected %f, got %f", i, expected, channels[i])
		}
	}

	if channels[16] != 17.0 {
		t.Errorf("atm pressure: expected 17, got %f", channels[16])
	}
	if channels[17] != 18.0 {
		t.Errorf("atm temp: expected 18, got %f", channels[17])
	}
}

func TestParseStreamFrame_TooShort(t *testing.T) {
	_, err := ParseStreamFrame(make([]byte, 70))
	if err == nil {
		t.Fatal("expected error for short frame")
	}
}

func TestParseStreamFrame_PressureReversal(t *testing.T) {
	data := make([]byte, 77)
	data[0] = 0x01 // 协议规定二进制流帧头第 0 字节固定为 0x01
	for i := 0; i < 16; i++ {
		f := float32(16 - i)
		binary.BigEndian.PutUint32(data[5+i*4:], math.Float32bits(f))
	}
	binary.BigEndian.PutUint32(data[5+16*4:], math.Float32bits(17.5))
	binary.BigEndian.PutUint32(data[5+17*4:], math.Float32bits(18.5))

	channels, err := ParseStreamFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 16; i++ {
		expected := float64(i + 1)
		if channels[i] != expected {
			t.Errorf("pressure channel %d: expected %f, got %f", i, expected, channels[i])
		}
	}
	if channels[16] != 17.5 {
		t.Errorf("CH17: expected 17.5, got %f", channels[16])
	}
	if channels[17] != 18.5 {
		t.Errorf("CH18: expected 18.5, got %f", channels[17])
	}
}

func TestIsASCIIFrame_AllASCII(t *testing.T) {
	if !IsASCIIFrame([]byte("Hello, World! This is ASCII.\r\n")) {
		t.Error("expected true for ASCII data")
	}
}

func TestIsASCIIFrame_Binary(t *testing.T) {
	if IsASCIIFrame([]byte{0x00, 0x01, 0x02, 0xFF}) {
		t.Error("expected false for binary data")
	}
}

func TestIsASCIIFrame_CRLF(t *testing.T) {
	if !IsASCIIFrame([]byte{0x0d, 0x0a, 0x0d, 0x0a}) {
		t.Error("expected true for CR/LF only")
	}
}

func TestIsASCIIFrame_BinaryAfter64(t *testing.T) {
	data := make([]byte, 70)
	for i := 0; i < 64; i++ {
		data[i] = 0x41
	}
	data[64] = 0xFF
	if !IsASCIIFrame(data) {
		t.Error("expected true: binary past 64 bytes is ignored")
	}
}

func TestIsASCIIFrame_BinaryWithin64(t *testing.T) {
	data := make([]byte, 70)
	for i := 0; i < 60; i++ {
		data[i] = 0x41
	}
	data[30] = 0xFF
	if IsASCIIFrame(data) {
		t.Error("expected false: binary within first 64 bytes")
	}
}

func TestIsASCIIFrame_Empty(t *testing.T) {
	if !IsASCIIFrame([]byte{}) {
		t.Error("expected true for empty data")
	}
}

func TestNewFrameReader(t *testing.T) {
	server, client := net.Pipe()
	server.Close()
	client.Close()
	fr := NewFrameReader(server)
	if fr == nil {
		t.Fatal("NewFrameReader returned nil")
	}
}

func TestFrameReader_ValidFrame(t *testing.T) {
	server, client := net.Pipe()

	payload := []byte{0x01, 0x02, 0x03, 0x04}
	frameLen := uint16(len(payload) + 2)
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, frameLen)

	go func() {
		client.Write(lenBuf)
		client.Write(payload)
		client.Close()
	}()

	fr := NewFrameReader(server)
	received, err := fr.ReadFrame()
	server.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(received, payload) {
		t.Errorf("expected %v, got %v", payload, received)
	}
}

func TestFrameReader_InvalidLength(t *testing.T) {
	server, client := net.Pipe()

	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, 1)

	go func() {
		client.Write(lenBuf)
		client.Close()
	}()

	fr := NewFrameReader(server)
	_, err := fr.ReadFrame()
	server.Close()
	if err == nil {
		t.Fatal("expected error for invalid frame length")
	}
}
