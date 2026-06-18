package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
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
		expected := float64((i+1)*10) * 0.1
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

// --- ASCII frame tests ---

func encodeASCIIFrame(values []float64) []byte {
	var sb strings.Builder
	for _, v := range values {
		sb.WriteString(fmt.Sprintf("%12.6f", v))
	}
	return []byte(sb.String())
}

func encodeASCIIFrameWithPrefix(seq *int, timestamp *float64, values []float64) []byte {
	parts := make([]string, 0, 18)
	if seq != nil {
		parts = append(parts, strconv.Itoa(*seq))
	}
	if timestamp != nil {
		parts = append(parts, fmt.Sprintf("%.6f", *timestamp))
	}
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%.6f", v))
	}
	return []byte(strings.Join(parts, " "))
}

func TestParseASCIIFrame_Valid(t *testing.T) {
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	data := encodeASCIIFrame(vals)

	temps, err := ParseASCIIFrame(data)
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

func TestParseASCIIFrame_InvalidSize(t *testing.T) {
	_, err := ParseASCIIFrame(make([]byte, 191))
	if err == nil {
		t.Error("expected error for 191-byte frame")
	}
	_, err = ParseASCIIFrame(make([]byte, 193))
	if err == nil {
		t.Error("expected error for 193-byte frame")
	}
}

func TestParseASCIIFrame_InvalidTokens(t *testing.T) {
	data := make([]byte, 192)
	for i := range data {
		data[i] = ' '
	}
	copy(data, "not-a-number")
	_, err := ParseASCIIFrame(data)
	if err == nil {
		t.Error("expected parse error for non-numeric data")
	}
}

func TestParseASCIIFrame_ChannelReversal(t *testing.T) {
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(100 + i)
	}
	data := encodeASCIIFrame(vals)

	temps, err := ParseASCIIFrame(data)
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

func TestParseASCIIFrame_ExampleFromSpec(t *testing.T) {
	vals := make([]float64, 16)
	vals[7] = 39.952503
	data := encodeASCIIFrame(vals)

	temps, err := ParseASCIIFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(temps) != 16 {
		t.Fatalf("expected 16 values, got %d", len(temps))
	}
	if temps[8] != 39.952503 {
		t.Errorf("expected ch8=39.952503 (device ch7 reversed), got %f", temps[8])
	}
	if temps[0] != 0 {
		t.Errorf("expected ch0=0, got %f", temps[0])
	}
}

// --- TCP auto-detect tests ---

func TestParseTCPFrame_AutoDetectBinary(t *testing.T) {
	data := make([]byte, 64)
	for i := 0; i < 16; i++ {
		f := float32(15 - i + 1)
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(f))
	}

	temps, err := ParseTCPFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 16; i++ {
		if temps[i] != float64(i+1) {
			t.Errorf("ch%d: expected %f, got %f", i, float64(i+1), temps[i])
		}
	}
}

func TestParseTCPFrame_AutoDetectASCII(t *testing.T) {
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	data := encodeASCIIFrame(vals)

	temps, err := ParseTCPFrame(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 16; i++ {
		if temps[i] != float64(i+1) {
			t.Errorf("ch%d: expected %f, got %f", i, float64(i+1), temps[i])
		}
	}
}

func TestParseTCPFrame_AutoDetectInvalidSize(t *testing.T) {
	_, err := ParseTCPFrame(make([]byte, 128))
	if err == nil {
		t.Error("expected error for 128-byte frame")
	}
}

func readWithTimeout(conn net.Conn, timeout time.Duration) (string, error) {
	type result struct {
		data string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		ch <- result{string(buf[:n]), err}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout")
	}
}

func TestSendCommandExactDrainsSplitCRLF(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		if cmd, err := readWithTimeout(server, 50*time.Millisecond); err != nil || cmd != "@e3" {
			done <- fmt.Errorf("first command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write([]byte("KKKKKKKKKKKKKKKK\r")); err != nil {
			done <- err
			return
		}
		time.Sleep(10 * time.Millisecond)
		if _, err := server.Write([]byte("\n")); err != nil {
			done <- err
			return
		}
		if cmd, err := readWithTimeout(server, 50*time.Millisecond); err != nil || cmd != "@fd MCH" {
			done <- fmt.Errorf("second command = %q, err = %v", cmd, err)
			return
		}
		_, err := server.Write([]byte("FFFF\n"))
		done <- err
	}()

	resp, err := SendCommandExact(client, "@e3", 16)
	if err != nil {
		t.Fatalf("SendCommandExact returned error: %v", err)
	}
	if resp != "KKKKKKKKKKKKKKKK" {
		t.Fatalf("SendCommandExact response = %q", resp)
	}

	resp, err = SendCommand(client, "@fd MCH")
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	if resp != "FFFF" {
		t.Fatalf("SendCommand response = %q, want FFFF", resp)
	}
	if err := <-done; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestConsumeOptionalACK_SingleByteA(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)

	frame := make([]byte, 64)
	for i := range frame {
		frame[i] = byte(i)
	}

	go func() {
		_, _ = server.Write(append([]byte{'A'}, frame...))
	}()

	consumed, err := reader.ConsumeOptionalACK(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("ConsumeOptionalACK returned error: %v", err)
	}
	if !consumed {
		t.Fatalf("expected ACK to be consumed")
	}

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if len(raw) != 64 {
		t.Fatalf("frame length = %d, want 64", len(raw))
	}
	for i := range raw {
		if raw[i] != frame[i] {
			t.Fatalf("frame[%d] = %d, want %d", i, raw[i], frame[i])
		}
	}
}

func TestConsumeOptionalACK_AWithNewline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)

	frame := make([]byte, 64)
	for i := range frame {
		frame[i] = byte(255 - i)
	}

	go func() {
		_, _ = server.Write(append([]byte{'A', '\n'}, frame...))
	}()

	consumed, err := reader.ConsumeOptionalACK(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("ConsumeOptionalACK returned error: %v", err)
	}
	if !consumed {
		t.Fatalf("expected ACK to be consumed")
	}

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if len(raw) != 64 {
		t.Fatalf("frame length = %d, want 64", len(raw))
	}
	for i := range raw {
		if raw[i] != frame[i] {
			t.Fatalf("frame[%d] = %d, want %d", i, raw[i], frame[i])
		}
	}
}

func TestReadFrame_PrefixedASCIIFrameWithNewline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetMetadataMode(true)

	seq := 123
	timestamp := 1712345678.123456
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeASCIIFrameWithPrefix(&seq, &timestamp, vals)
	frameWithNewline := append(frame, '\n')

	go func() {
		_, _ = server.Write(frameWithNewline[:60])
		time.Sleep(10 * time.Millisecond)
		_, _ = server.Write(frameWithNewline[60:])
	}()

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if string(raw) != string(frame) {
		t.Fatalf("ReadFrame returned incomplete frame: got %q want %q", string(raw), string(frame))
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.SequenceNumber != seq {
		t.Fatalf("sequence = %d, want %d", result.SequenceNumber, seq)
	}
	if result.HardwareTimestamp != timestamp {
		t.Fatalf("timestamp = %f, want %f", result.HardwareTimestamp, timestamp)
	}
	if len(result.Temperatures) != 16 {
		t.Fatalf("expected 16 temperatures, got %d", len(result.Temperatures))
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Fatalf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestReadFrame_FixedWidthASCIIFrameFromChunks(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)

	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeASCIIFrame(vals)
	if len(frame) != 192 {
		t.Fatalf("expected fixed-width ASCII frame to be 192 bytes, got %d", len(frame))
	}

	go func() {
		_, _ = server.Write(frame)
	}()

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if string(raw) != string(frame) {
		t.Fatalf("ReadFrame returned wrong frame: got %q want %q", string(raw), string(frame))
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if len(result.Temperatures) != 16 {
		t.Fatalf("expected 16 temperatures, got %d", len(result.Temperatures))
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Fatalf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}
