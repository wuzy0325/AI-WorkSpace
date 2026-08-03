package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
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

func TestParseTCPFrame_RejectsShiftedBinaryFrameWithImpossibleValue(t *testing.T) {
	data := make([]byte, 64)
	// Real hardware rapid restart failure pattern: most channels decode as 0,
	// but one shifted float becomes a large finite impossible temperature.
	binary.LittleEndian.PutUint32(data[4:], math.Float32bits(-10_522_368))

	_, err := ParseTCPFrame(data)
	if err == nil {
		t.Fatal("expected error for shifted binary frame with impossible temperature")
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

func TestParseASCIIFrame_RejectsOutOfRangeValues(t *testing.T) {
	vals := make([]float64, 16)
	for i := range vals {
		vals[i] = 99999
	}

	_, err := ParseASCIIFrame(encodeASCIIFrame(vals))
	if err == nil {
		t.Fatal("expected error for out-of-range ASCII temperatures")
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

func TestSendCommandExactWithoutTerminatorDoesNotDelayNextCommand(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		if cmd, err := readWithTimeout(server, 50*time.Millisecond); err != nil || cmd != "@e3" {
			done <- fmt.Errorf("first command = %q, err = %v", cmd, err)
			return
		}
		if _, err := server.Write([]byte("KKKKKKKKKKKKKKKK")); err != nil {
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

func TestSendCommandExactReturnsWithoutOptionalTerminator(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		_, _ = server.Write([]byte("KKKKKKKKKKKKKKKK"))
	}()

	done := make(chan error, 1)
	go func() {
		resp, err := SendCommandExact(ignored, "@e3", 16)
		if err == nil && resp != "KKKKKKKKKKKKKKKK" {
			err = fmt.Errorf("response = %q", resp)
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendCommandExact returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SendCommandExact waited for an optional response terminator")
	}
}

func TestConsumeOptionalACK_SingleByteA(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)

	frame := make([]byte, 64)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(float32(i+1)))
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
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(float32(i+1)))
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

func TestReadFrame_ExpectedACKAfterCompleteTailFrames(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.ExpectControlACKAfterFrames()

	frame := make([]byte, TCPFrameSize)
	binary.LittleEndian.PutUint32(frame[14*4:], math.Float32bits(27.5))
	go func() {
		payload := append(append(append([]byte{}, frame...), frame...), 'A')
		_, _ = server.Write(payload)
	}()

	for i := 0; i < 2; i++ {
		got, err := reader.ReadFrame()
		if err != nil {
			t.Fatalf("tail frame %d: ReadFrame returned error: %v", i+1, err)
		}
		if !bytes.Equal(got, frame) {
			t.Fatalf("tail frame %d changed", i+1)
		}
	}
	if _, err := reader.ReadFrame(); !errors.Is(err, ErrControlACK) {
		t.Fatalf("ReadFrame error = %v, want ErrControlACK", err)
	}
}

func TestReadFrame_StopDoesNotMistakeFragmentedALeadingTailFrameForACK(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.ExpectControlACKAfterFrames()

	frame := make([]byte, TCPFrameSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(25))
	}
	frame[0] = 'A' // A data byte at the established frame boundary, not the Stop ACK.

	go func() {
		_, _ = server.Write(frame[:1])
		// Exceeds the former 50 ms quiet window. The leading data byte must not
		// be finalized as an N=0 Stop ACK while the rest of the frame is in flight.
		time.Sleep(75 * time.Millisecond)
		_, _ = server.Write(append(frame[1:], 'A'))
	}()

	got, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("tail frame: ReadFrame returned error: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatal("A-leading tail frame changed")
	}
	_ = client.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := reader.ReadFrame(); !errors.Is(err, ErrControlACK) {
		t.Fatalf("Stop ACK error = %v, want ErrControlACK", err)
	}
}

func TestReadFrame_StartACKWinsOverPlausibleSparseMisalignment(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.ExpectControlACK()
	frame := make([]byte, TCPFrameSize)

	go func() { _, _ = server.Write(append([]byte{'A'}, frame...)) }()
	if _, err := reader.ReadFrame(); !errors.Is(err, ErrControlACK) {
		t.Fatalf("ReadFrame error = %v, want ErrControlACK", err)
	}
	got, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error after ACK: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatal("sparse frame changed after consuming start ACK")
	}
}

// TestReadFrame_StartDataFirstWithoutACK 验证 Start ACK 偶发缺失时
// （SKILL.md §3.3.4）：首字节非 'A' 且偏移0帧合法 → 直接按数据帧消费，
// 不要求 ACK，也不判错。
func TestReadFrame_StartDataFirstWithoutACK(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.ExpectControlACK()

	makeFrame := func(v float32) []byte {
		frame := make([]byte, TCPFrameSize)
		for i := 0; i < 16; i++ {
			binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(v))
		}
		return frame
	}
	frame1 := makeFrame(25)
	frame2 := makeFrame(30)

	go func() { _, _ = server.Write(append(append([]byte{}, frame1...), frame2...)) }()

	got, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("first data frame without ACK: ReadFrame returned error: %v", err)
	}
	if !bytes.Equal(got, frame1) {
		t.Fatal("first frame changed when Start ACK was absent")
	}
	got, err = reader.ReadFrame()
	if err != nil {
		t.Fatalf("second data frame without ACK: ReadFrame returned error: %v", err)
	}
	if !bytes.Equal(got, frame2) {
		t.Fatal("second frame changed when Start ACK was absent")
	}
	if reader.HasPendingControlACK() {
		t.Fatal("Start ACK stayed pending after data-first frames")
	}
}

// TestReadFrame_NormalAcquisitionSelfHealOnStrayByte 验证正常采集路径中，
// 边界帧非法时丢弃 1 个前导字节（迟到的 ACK/残杂字节）后自愈，不丢失后续数据。
func TestReadFrame_NormalAcquisitionSelfHealOnStrayByte(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)

	frame := make([]byte, TCPFrameSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(27.5))
	}
	frame2 := make([]byte, TCPFrameSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame2[i*4:], math.Float32bits(30))
	}

	// 流中插入一个 0x41 前导字节（模拟迟到的 ACK 被当作数据消费）。
	go func() { _, _ = server.Write(append(append([]byte{'A'}, frame...), frame2...)) }()

	got, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("frame after stray byte: ReadFrame returned error: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatal("frame changed after 1-byte self-heal")
	}
	got, err = reader.ReadFrame()
	if err != nil {
		t.Fatalf("second frame: ReadFrame returned error: %v", err)
	}
	if !bytes.Equal(got, frame2) {
		t.Fatal("second frame changed")
	}
}

// TestReadFrame_StartLeadingStrayByteOffset1 验证 Start 时存在1个前导残杂字节、
// 帧从偏移1开始的情况：应丢弃前导字节，按偏移1对齐并返回首帧。
func TestReadFrame_StartLeadingStrayByteOffset1(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.ExpectControlACK()

	frame := make([]byte, TCPFrameSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(27.5))
	}

	go func() { _, _ = server.Write(append([]byte{0x00}, frame...)) }()
	got, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("first frame after leading stray byte: ReadFrame returned error: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Fatal("frame after leading stray byte changed")
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

func TestParseTCPFrameEx_RejectsOutOfRangeSpaceSeparatedValues(t *testing.T) {
	vals := make([]float64, 16)
	for i := range vals {
		vals[i] = 99999
	}
	seq := 123
	timestamp := 1712345678.123456

	_, err := ParseTCPFrameEx(encodeASCIIFrameWithPrefix(&seq, &timestamp, vals))
	if err == nil {
		t.Fatal("expected error for out-of-range prefixed ASCII temperatures")
	}
}

// --- 72-byte binary timestamp frame tests ---

func encodeBinaryFrameWithTimestamp(sec uint32, nsec uint32, values []float64) []byte {
	frame := make([]byte, 72)
	binary.LittleEndian.PutUint32(frame[0:4], sec)
	binary.LittleEndian.PutUint32(frame[4:8], nsec)
	for i, v := range values {
		binary.LittleEndian.PutUint32(frame[8+i*4:], math.Float32bits(float32(v)))
	}
	return frame
}

func TestParseTCPFrameEx_72ByteBinaryTimestamp(t *testing.T) {
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}

	frame := encodeBinaryFrameWithTimestamp(1781803881, 179316583, vals)
	if len(frame) != 72 {
		t.Fatalf("frame length = %d, want 72", len(frame))
	}

	result, err := ParseTCPFrameEx(frame)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}

	expectedTS := 1781803881.179316583
	if result.HardwareTimestamp != expectedTS {
		t.Errorf("timestamp = %f, want %f", result.HardwareTimestamp, expectedTS)
	}
	if len(result.Temperatures) != 16 {
		t.Fatalf("expected 16 temperatures, got %d", len(result.Temperatures))
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Errorf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestParseTCPFrameEx_72ByteInvalidSize(t *testing.T) {
	_, err := ParseTCPFrameEx(make([]byte, 71))
	if err == nil {
		t.Error("expected error for 71-byte frame")
	}
	_, err = ParseTCPFrameEx(make([]byte, 73))
	if err == nil {
		t.Error("expected error for 73-byte frame")
	}
}

func TestParseTCPFrameEx_72ByteRejectsOutOfRangeValues(t *testing.T) {
	vals := make([]float64, 16)
	for i := range vals {
		vals[i] = 99999
	}

	_, err := ParseTCPFrameEx(encodeBinaryFrameWithTimestamp(1781803881, 179316583, vals))
	if err == nil {
		t.Fatal("expected error for out-of-range binary timestamp temperatures")
	}
}

// encodeBinaryFrameWithSequence 构造 HEAD=1 时的 68 字节帧：[seq uint32 LE][16×float32 LE]。
func encodeBinaryFrameWithSequence(seq uint32, values []float64) []byte {
	frame := make([]byte, TCPFrameSizeWithSequence)
	binary.LittleEndian.PutUint32(frame[0:4], seq)
	for i, v := range values {
		binary.LittleEndian.PutUint32(frame[4+i*4:], math.Float32bits(float32(v)))
	}
	return frame
}

func TestParseTCPFrameEx_68ByteBinarySequence(t *testing.T) {
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithSequence(1234, vals)
	if len(frame) != TCPFrameSizeWithSequence {
		t.Fatalf("frame length = %d, want %d", len(frame), TCPFrameSizeWithSequence)
	}

	result, err := ParseTCPFrameEx(frame)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.SequenceNumber != 1234 {
		t.Fatalf("sequence = %d, want 1234", result.SequenceNumber)
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

func TestParseTCPFrameEx_68ByteRejectsOutOfRangeValues(t *testing.T) {
	vals := make([]float64, 16)
	for i := range vals {
		vals[i] = 99999
	}
	_, err := ParseTCPFrameEx(encodeBinaryFrameWithSequence(1, vals))
	if err == nil {
		t.Fatal("expected error for out-of-range binary sequence temperatures")
	}
}

// encodeBinaryFrameWithSequenceAndTimestamp 构造 HEAD=1 且 TIME=1 时的 76 字节帧：
// [seq uint32 LE][sec uint32 LE][ns uint32 LE][16×float32 LE]。
func encodeBinaryFrameWithSequenceAndTimestamp(seq, sec, ns uint32, values []float64) []byte {
	frame := make([]byte, TCPFrameSizeWithSequenceAndTimestamp)
	binary.LittleEndian.PutUint32(frame[0:4], seq)
	binary.LittleEndian.PutUint32(frame[4:8], sec)
	binary.LittleEndian.PutUint32(frame[8:12], ns)
	for i, v := range values {
		binary.LittleEndian.PutUint32(frame[12+i*4:], math.Float32bits(float32(v)))
	}
	return frame
}

func TestParseTCPFrameEx_76ByteBinarySequenceAndTimestamp(t *testing.T) {
	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithSequenceAndTimestamp(99, 1781803881, 179316583, vals)
	if len(frame) != TCPFrameSizeWithSequenceAndTimestamp {
		t.Fatalf("frame length = %d, want %d", len(frame), TCPFrameSizeWithSequenceAndTimestamp)
	}

	result, err := ParseTCPFrameEx(frame)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.SequenceNumber != 99 {
		t.Fatalf("sequence = %d, want 99", result.SequenceNumber)
	}
	expectedTS := 1781803881.179316583
	if result.HardwareTimestamp != expectedTS {
		t.Errorf("timestamp = %f, want %f", result.HardwareTimestamp, expectedTS)
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Fatalf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestParseTCPFrameEx_76ByteRejectsOutOfRangeValues(t *testing.T) {
	vals := make([]float64, 16)
	for i := range vals {
		vals[i] = 99999
	}
	_, err := ParseTCPFrameEx(encodeBinaryFrameWithSequenceAndTimestamp(1, 1781803881, 179316583, vals))
	if err == nil {
		t.Fatal("expected error for out-of-range binary seq+ts temperatures")
	}
}

func TestReadFrame_BinarySequenceMode(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.SetSequenceMode(true)

	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithSequence(42, vals)

	go func() {
		_, _ = server.Write(frame)
	}()

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if len(raw) != TCPFrameSizeWithSequence {
		t.Fatalf("frame length = %d, want %d", len(raw), TCPFrameSizeWithSequence)
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.SequenceNumber != 42 {
		t.Fatalf("sequence = %d, want 42", result.SequenceNumber)
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Fatalf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestReadFrame_BinarySequenceAndTimestampMode(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.SetSequenceMode(true)
	reader.SetMetadataMode(true)

	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithSequenceAndTimestamp(7, 1781803881, 179316583, vals)

	go func() {
		_, _ = server.Write(frame)
	}()

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if len(raw) != TCPFrameSizeWithSequenceAndTimestamp {
		t.Fatalf("frame length = %d, want %d", len(raw), TCPFrameSizeWithSequenceAndTimestamp)
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.SequenceNumber != 7 {
		t.Fatalf("sequence = %d, want 7", result.SequenceNumber)
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Fatalf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestReadFrame_BinaryTimestampMode(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.SetMetadataMode(true)

	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithTimestamp(1781803881, 179316583, vals)

	go func() {
		_, _ = server.Write(frame)
	}()

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if len(raw) != 72 {
		t.Fatalf("frame length = %d, want 72", len(raw))
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}

	expectedTS := 1781803881.179316583
	if result.HardwareTimestamp != expectedTS {
		t.Errorf("timestamp = %f, want %f", result.HardwareTimestamp, expectedTS)
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Errorf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestReadFrame_BinaryTimestampModeFromChunks(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.SetMetadataMode(true)

	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithTimestamp(1781803881, 179316583, vals)

	go func() {
		_, _ = server.Write(frame[:40])
		time.Sleep(10 * time.Millisecond)
		_, _ = server.Write(frame[40:])
	}()

	raw, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame returned error: %v", err)
	}
	if len(raw) != 72 {
		t.Fatalf("frame length = %d, want 72", len(raw))
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.HardwareTimestamp != 1781803881.179316583 {
		t.Errorf("timestamp = %f, want 1781803881.179316583", result.HardwareTimestamp)
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Errorf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestReadFrame_BinaryTimestampModeAfterACK(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)
	reader.SetMetadataMode(true)

	vals := make([]float64, 16)
	for i := 0; i < 16; i++ {
		vals[i] = float64(15 - i + 1)
	}
	frame := encodeBinaryFrameWithTimestamp(1781803881, 179316583, vals)

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
	if len(raw) != 72 {
		t.Fatalf("frame length = %d, want 72", len(raw))
	}

	result, err := ParseTCPFrameEx(raw)
	if err != nil {
		t.Fatalf("ParseTCPFrameEx returned error: %v", err)
	}
	if result.HardwareTimestamp != 1781803881.179316583 {
		t.Errorf("timestamp = %f, want 1781803881.179316583", result.HardwareTimestamp)
	}
	for i := 0; i < 16; i++ {
		want := float64(i + 1)
		if result.Temperatures[i] != want {
			t.Errorf("temperature[%d] = %f, want %f", i, result.Temperatures[i], want)
		}
	}
}

func TestParseTCPFrame_PlausibleFourByteShiftIsIndistinguishable(t *testing.T) {
	frame := make([]byte, TCPFrameSize)
	binary.LittleEndian.PutUint32(frame[14*4:], math.Float32bits(27.5))
	shiftedCandidate := append([]byte{0, 0, 0, 0}, frame[:60]...)

	if _, err := ParseTCPFrame(shiftedCandidate); err != nil {
		t.Fatalf("four-byte shifted sparse frame should demonstrate protocol ambiguity, got %v", err)
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

// =================================================================
// ADR-009 watchdog 兜底测试（P0-6）
// -----------------------------------------------------------------
// 设计依据 ADR-009：SetReadDeadline 在某些 Windows 电脑不可靠，
// Read 在 deadline 到期后仍可能无限阻塞。helper 必须有独立
// watchdog 计时器，超时强制 Close conn 解除阻塞。
// 测试用 deadlineIgnoringConn（SetReadDeadline no-op）模拟
// 故障 Windows 场景，验证 watchdog 在预算内返回并附加上下文。
// =================================================================

// TestT1603SendCommand_WatchdogTriggersOnDeadlineIgnoringConn 验证 SendCommand
// 在 deadline 失效场景下由 watchdog 兜底，预算内返回错误且包含 "watchdog triggered"。
//
// 前置：包装 client 为 deadlineIgnoringConn（SetReadDeadline 被 no-op），
// server 端读出命令但不写响应（确保 Read 阻塞而非 Write 阻塞）。
// 期待：SendCommand 在 cmdTimeout(1s) + 安全余量内返回错误，
// 错误信息包含 "watchdog triggered"。
func TestT1603SendCommand_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)

	// server 端读出命令让 client Write 完成，但不写响应让 client Read 阻塞
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 不写任何响应，触发 client Read 阻塞
	}()

	// 总预算 3s（cmdWatchdogTimeout=2s + 1s 余量），watchdog 应在 ~2s 内触发
	done := make(chan error, 1)
	go func() {
		_, err := SendCommand(ignored, "@fd MCH")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected watchdog-triggered error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") {
			t.Errorf("error should mention 'watchdog triggered', got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendCommand did not return within 3s budget; watchdog likely not armed")
	}
}

// TestT1603SendCommandIdle_WatchdogTriggersOnDeadlineIgnoringConn 验证 SendCommandIdle
// 在 deadline 失效场景下由 watchdog 兜底。
func TestT1603SendCommandIdle_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 不写任何响应
	}()

	done := make(chan error, 1)
	go func() {
		_, err := SendCommandIdle(ignored, "@fd SPS", 30*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected watchdog-triggered error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") {
			t.Errorf("error should mention 'watchdog triggered', got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendCommandIdle did not return within 3s budget; watchdog likely not armed")
	}
}

// TestT1603SendCommandExact_WatchdogTriggersOnDeadlineIgnoringConn 验证 SendCommandExact
// 在 deadline 失效场景下由 watchdog 兜底。
// io.ReadFull 内部循环 conn.Read 不重设 deadline，必须由外层 watchdog 兜底。
func TestT1603SendCommandExact_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 不写任何响应
	}()

	done := make(chan error, 1)
	go func() {
		_, err := SendCommandExact(ignored, "@e3", 16)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected watchdog-triggered error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") {
			t.Errorf("error should mention 'watchdog triggered', got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendCommandExact did not return within 3s budget; watchdog likely not armed")
	}
}

// TestT1603ConsumeOptionalACK_DoesNotCloseHealthyConnWhenNoACK 验证
// ConsumeOptionalACK 在无 ACK 场景下不会关闭健康连接。
//
// ADR-009 决策 8：可选 ACK 是"无数据也正常"的操作，watchdog 到期只能证明
// 探测无法完成，不能证明物理连接故障。原实现通过 WatchdogClose 在 timeout
// 后强制 Close conn，违反该决策。整改后仅依赖 SetReadDeadline 软超时，
// timeout 到期返回 (false, nil)，连接保持开放。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - server 不发送任何数据（模拟无 ACK，正常状态）
//
// 测试步骤：
//   - 调用 reader.ConsumeOptionalACK(50ms)
//
// 期待结果：
//   - 返回 (false, nil)（无 ACK，正常结果）
//   - 连接未被关闭：server.Write 成功，client.Read 能读到数据
//   - 后续 ReadFrame 仍可正常工作
func TestT1603ConsumeOptionalACK_DoesNotCloseHealthyConnWhenNoACK(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	reader := NewT1603FrameReader(client)
	reader.SetBinaryMode(true)

	consumed, err := reader.ConsumeOptionalACK(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("ConsumeOptionalACK returned error on healthy conn with no ACK: %v", err)
	}
	if consumed {
		t.Fatal("expected consumed=false when no ACK sent")
	}

	// 验证连接未被关闭。
	// net.Pipe 的 Write 阻塞等待 Read，需用 goroutine 并发读写避免死锁。
	readCh := make(chan struct {
		data string
		err  error
	}, 1)
	go func() {
		_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := client.Read(buf)
		readCh <- struct {
			data string
			err  error
		}{string(buf[:n]), err}
	}()

	// 短暂等待确保 client.Read goroutine 已启动并阻塞在 Read 上
	time.Sleep(20 * time.Millisecond)

	// server.Write 应能成功（client.Read 在等待）
	_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := server.Write([]byte("alive")); err != nil {
		t.Fatalf("server.Write failed after ConsumeOptionalACK, conn was killed: %v", err)
	}

	// 等待 client.Read 完成
	select {
	case r := <-readCh:
		if r.err != nil {
			t.Fatalf("client.Read failed after ConsumeOptionalACK, conn was killed: %v", r.err)
		}
		if r.data != "alive" {
			t.Fatalf("client.Read got %q, want %q", r.data, "alive")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("client.Read did not complete within 500ms")
	}
}

// TestT1603SendCommand_ClearsDeadlineOnSuccess 验证 SendCommand 成功路径
// 在 watchdog 未触发时清 deadline（避免残留 cmdTailTimeout=100ms 影响后续命令）。
//
// 前置：server 端在收到命令后立即返回完整响应（含 \n）。
// 期待：SendCommand 成功返回，client 的 read deadline 被清除（time.Time{}）。
func TestT1603SendCommand_ClearsDeadlineOnSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 复用同包 conn_helpers_test.go 中的 deadlineTrackingConn（已扩展记录 lastReadDeadline）
	tracked := &deadlineTrackingConn{Conn: client}

	go func() {
		// 等待命令到达后立即返回响应
		buf := make([]byte, 64)
		if _, err := server.Read(buf); err != nil {
			return
		}
		_, _ = server.Write([]byte("FFFF\n"))
	}()

	resp, err := SendCommand(tracked, "@fd MCH")
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	if resp != "FFFF" {
		t.Fatalf("response = %q, want FFFF", resp)
	}

	// 验证 read deadline 被清除为 time.Time{}（零值）
	got := tracked.lastReadDeadlineValue()
	if !got.IsZero() {
		t.Fatalf("read deadline not cleared after success: %v", got)
	}
}

// TestT1603SendCommand_SoftTimeoutClosesConnAndReturnsSentinel 验证 ADR-009 R0-12：
// 当 SetReadDeadline 正常兑现（soft deadline 先于 watchdog 触发）时，SendCommand 必须
// 强制 Close conn 阻断迟到响应，并返回包装 ErrWatchdogTriggered sentinel 的错误让调用方
// 统一毒化驱动状态。
//
// 修复前 bug：soft deadline 兑现时 helper 仅返回普通 timeout 错误，不 Close conn。
// 迟到响应随后进入 TCP 流被下一条命令消费，导致协议错位。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 真有效，与 deadlineIgnoringConn 相反）
//   - server 端读出命令后写一个非 '\n' 字节再停止发送，让 SendCommand 进入 cmdTailTimeout
//     阶段（100ms）而非 cmdTimeout 阶段（1s），加速测试
//
// 测试步骤：
//   - 调用 SendCommand，server 写一个字节后停止
//   - 等待 cmdTailTimeout(100ms) soft deadline 触发
//   - 函数返回后，server 再写入迟到响应
//
// 期待结果：
//   - 函数在 2s 预算内返回
//   - 错误通过 errors.Is(err, ErrWatchdogTriggered) 精确匹配 sentinel
//   - conn 已被 helper Close：server 写入迟到响应时 Write 失败
func TestT1603SendCommand_SoftTimeoutClosesConnAndReturnsSentinel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 helper Close，不在 defer 中重复 Close

	go func() {
		// 读出命令让 client Write 完成
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 写一个非 '\n' 字节让 SendCommand 进入 cmdTailTimeout 阶段（100ms 而非 1s）
		_, _ = server.Write([]byte("X"))
		// 不再写任何数据，让 client Read 在 cmdTailTimeout(100ms) 后触发 soft timeout
	}()

	done := make(chan error, 1)
	go func() {
		_, err := SendCommand(client, "@fd MCH")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected soft timeout error wrapping ErrWatchdogTriggered, got nil")
		}
		if !errors.Is(err, ErrWatchdogTriggered) {
			t.Errorf("error must wrap ErrWatchdogTriggered for soft timeout, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendCommand did not return within 2s budget; soft timeout likely not triggered")
	}

	// 验证 conn 已被 Close：服务端写入迟到响应应失败。
	// 设置 WriteDeadline 防止 net.Pipe 无缓冲 Write 永久阻塞。
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("late-response\n")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout helper")
	}
}

// TestT1603SendCommandIdle_SoftTimeoutClosesConnAndReturnsSentinel 验证 ADR-009 R0-12：
// SendCommandIdle 在首字节 cmdTimeout soft deadline 兑现时（未收到任何字节）必须
// Close conn 并返回 ErrWatchdogTriggered。已收到字节后的 idleWindow timeout 是正常
// 结束语义，不视为 soft timeout。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 真有效）
//   - server 端读出命令后不写任何响应，让 client Read 在 cmdTimeout(1s) 后触发 soft timeout
//
// 测试步骤：
//   - 调用 SendCommandIdle，idleWindow=30ms
//   - 等待 cmdTimeout(1s) soft deadline 触发
//
// 期待结果：
//   - 函数在 2s 预算内返回
//   - 错误通过 errors.Is(err, ErrWatchdogTriggered) 精确匹配 sentinel
//   - conn 已被 helper Close
func TestT1603SendCommandIdle_SoftTimeoutClosesConnAndReturnsSentinel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 helper Close

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 不写任何响应，让 client Read 在 cmdTimeout(1s) 后触发 soft timeout
	}()

	done := make(chan error, 1)
	go func() {
		_, err := SendCommandIdle(client, "@fd SPS", 30*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected soft timeout error wrapping ErrWatchdogTriggered, got nil")
		}
		if !errors.Is(err, ErrWatchdogTriggered) {
			t.Errorf("error must wrap ErrWatchdogTriggered for soft timeout, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendCommandIdle did not return within 2s budget; soft timeout likely not triggered")
	}

	// 验证 conn 已被 Close
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("late\n")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout helper")
	}
}

// TestT1603SendCommandExact_SoftTimeoutClosesConnAndReturnsSentinel 验证 ADR-009 R0-12：
// SendCommandExact 在 io.ReadFull 中途 soft deadline 兑现时必须 Close conn 并返回
// ErrWatchdogTriggered。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 真有效）
//   - server 端读出命令后写部分字节（少于 n）再停止，让 io.ReadFull 在中途触发 deadline
//
// 测试步骤：
//   - 调用 SendCommandExact，n=16
//   - server 写 5 字节后停止
//   - 等待 cmdTimeout(1s) soft deadline 触发
//
// 期待结果：
//   - 函数在 2s 预算内返回
//   - 错误通过 errors.Is(err, ErrWatchdogTriggered) 精确匹配 sentinel
//   - conn 已被 helper Close
func TestT1603SendCommandExact_SoftTimeoutClosesConnAndReturnsSentinel(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 helper Close

	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 写部分字节（少于 n=16）让 io.ReadFull 进入中途等待
		_, _ = server.Write([]byte("partial"))
		// 不再写数据，让 client Read 在 cmdTimeout(1s) 后触发 soft timeout
	}()

	done := make(chan error, 1)
	go func() {
		_, err := SendCommandExact(client, "@e3", 16)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected soft timeout error wrapping ErrWatchdogTriggered, got nil")
		}
		if !errors.Is(err, ErrWatchdogTriggered) {
			t.Errorf("error must wrap ErrWatchdogTriggered for soft timeout, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendCommandExact did not return within 2s budget; soft timeout likely not triggered")
	}

	// 验证 conn 已被 Close
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("late-response-data")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout helper")
	}
}
