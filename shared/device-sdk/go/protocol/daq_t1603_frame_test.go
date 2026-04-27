package protocol

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseSerialFrame_Valid(t *testing.T) {
	data := make([]byte, 46)
	binary.BigEndian.PutUint16(data[8:], 123)
	v := int16(-50)
	binary.BigEndian.PutUint16(data[10:], uint16(v))
	binary.BigEndian.PutUint16(data[12:], 375)

	temps, err := ParseSerialFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(temps) != 16 {
		t.Fatalf("expected 16 temperatures, got %d", len(temps))
	}
	if temps[0] != 12.3 {
		t.Errorf("ch0: expected 12.3, got %f", temps[0])
	}
	if temps[1] != -5.0 {
		t.Errorf("ch1: expected -5.0, got %f", temps[1])
	}
	if temps[2] != 37.5 {
		t.Errorf("ch2: expected 37.5, got %f", temps[2])
	}
}

func TestParseSerialFrame_InvalidSize(t *testing.T) {
	_, err := ParseSerialFrame(make([]byte, 45))
	if err == nil {
		t.Error("expected error for 45-byte frame")
	}
	_, err = ParseSerialFrame(make([]byte, 47))
	if err == nil {
		t.Error("expected error for 47-byte frame")
	}
}

func TestParseSerialFrame_TempConversion(t *testing.T) {
	tests := []struct {
		raw      int16
		expected float64
	}{
		{0, 0.0},
		{10, 1.0},
		{-5, -0.5},
		{255, 25.5},
		{-100, -10.0},
		{1000, 100.0},
		{-273, -27.3},
	}
	for _, tt := range tests {
		data := make([]byte, 46)
		binary.BigEndian.PutUint16(data[8:], uint16(tt.raw))
		temps, err := ParseSerialFrame(data)
		if err != nil {
			t.Fatalf("unexpected error for raw=%d: %v", tt.raw, err)
		}
		if temps[0] != tt.expected {
			t.Errorf("raw=%d: expected %f, got %f", tt.raw, tt.expected, temps[0])
		}
	}
}

func TestParseSerialFrame_AllChannels(t *testing.T) {
	data := make([]byte, 46)
	for i := 0; i < 16; i++ {
		binary.BigEndian.PutUint16(data[8+i*2:], uint16(int16((i+1)*10)))
	}
	temps, err := ParseSerialFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 16; i++ {
		expected := float64((i + 1) * 10) * 0.1
		if temps[i] != expected {
			t.Errorf("ch%d: expected %f, got %f", i, expected, temps[i])
		}
	}
}

func TestParseTCPFrame_Valid(t *testing.T) {
	data := make([]byte, 64)
	for i := 0; i < 16; i++ {
		f := float32(15 - i + 1)
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(f))
	}

	temps, err := ParseTCPFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(temps) != 16 {
		t.Fatalf("expected 16 temperatures, got %d", len(temps))
	}
	for i := 0; i < 16; i++ {
		expected := float64(i + 1)
		if temps[i] != expected {
			t.Errorf("ch%d: expected %f, got %f", i, expected, temps[i])
		}
	}
}

func TestParseTCPFrame_InvalidSize(t *testing.T) {
	_, err := ParseTCPFrame(make([]byte, 63))
	if err == nil {
		t.Error("expected error for 63-byte frame")
	}
	_, err = ParseTCPFrame(make([]byte, 65))
	if err == nil {
		t.Error("expected error for 65-byte frame")
	}
}

func TestParseTCPFrame_ChannelReversal(t *testing.T) {
	data := make([]byte, 64)
	for i := 0; i < 16; i++ {
		f := float32(100 + i)
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(f))
	}

	temps, err := ParseTCPFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if temps[0] != 115.0 {
		t.Errorf("expected ch0=115, got %f", temps[0])
	}
	if temps[15] != 100.0 {
		t.Errorf("expected ch15=100, got %f", temps[15])
	}
}

func TestParseTCPFrame_FloatValues(t *testing.T) {
	data := make([]byte, 64)
	binary.LittleEndian.PutUint32(data[0:4], math.Float32bits(36.5))
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(37.0))
	binary.LittleEndian.PutUint32(data[60:64], math.Float32bits(38.5))

	temps, err := ParseTCPFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if temps[15] != 36.5 {
		t.Errorf("expected ch15=36.5, got %f", temps[15])
	}
	if temps[14] != 37.0 {
		t.Errorf("expected ch14=37.0, got %f", temps[14])
	}
	if temps[0] != 38.5 {
		t.Errorf("expected ch0=38.5, got %f", temps[0])
	}
}
