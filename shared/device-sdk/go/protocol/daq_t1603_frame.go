package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// -- DAQ-T-1603 serial frame parsing --

const SerialFrameSize = 46

// ParseSerialFrame parses serial data frame
// Temperature data: 16 channels x 2 bytes, signed short, big-endian, 0.1 C/LSB
func ParseSerialFrame(data []byte) ([]float64, error) {
	if len(data) != SerialFrameSize {
		return nil, fmt.Errorf("invalid frame size: %d, expected %d", len(data), SerialFrameSize)
	}

	temperatures := make([]float64, 16)
	for i := 0; i < 16; i++ {
		raw := int16(binary.BigEndian.Uint16(data[8+i*2:]))
		temperatures[i] = float64(raw) * 0.1
	}
	return temperatures, nil
}

// -- DAQ-T-1603 TCP frame parsing --

const TCPFrameSize = 64
const maxReasonableThermocoupleTemp = 300.0

// ParseTCPFrame parses TCP data frame.
// Auto-detects format based on size:
//
//	64 bytes  -> binary format (16 x float32 LE)
//	192 bytes -> ASCII text format (16 x 12-char fixed-width fields)
//
// Channel order from device is CH15→CH0, results are reversed to CH0→CH15.
func ParseTCPFrame(data []byte) ([]float64, error) {
	switch len(data) {
	case TCPFrameSize:
		return parseBinaryFrame(data)
	case 192:
		return ParseASCIIFrame(data)
	default:
		return nil, fmt.Errorf("invalid frame size: %d, expected 64 or 192", len(data))
	}
}

func parseBinaryFrame(data []byte) ([]float64, error) {
	temperatures := make([]float64, 16)
	for i := 0; i < 16; i++ {
		bits := binary.LittleEndian.Uint32(data[i*4:])
		temperatures[i] = float64(math.Float32frombits(bits))
	}

	for i, j := 0, len(temperatures)-1; i < j; i, j = i+1, j-1 {
		temperatures[i], temperatures[j] = temperatures[j], temperatures[i]
	}

	if !looksLikeReasonableTemperatureFrame(temperatures) {
		return nil, fmt.Errorf("binary frame values out of expected range")
	}

	return temperatures, nil
}

func looksLikeReasonableTemperatureFrame(temps []float64) bool {
	if len(temps) == 0 {
		return false
	}
	reasonableCount := 0
	for _, temp := range temps {
		if math.IsNaN(temp) || math.IsInf(temp, 0) {
			continue
		}
		if temp >= -100 && temp <= maxReasonableThermocoupleTemp {
			reasonableCount++
		}
	}
	return reasonableCount >= len(temps)/2
}

// ParseASCIIFrame parses a 192-byte ASCII text frame.
// Format: 16 x 12-char fixed-width space-separated float fields.
// Device sends CH15→CH0; result is reversed to CH0→CH15.
func ParseASCIIFrame(data []byte) ([]float64, error) {
	if len(data) != 192 {
		return nil, fmt.Errorf("invalid ASCII frame size: %d, expected 192", len(data))
	}

	s := strings.TrimSpace(string(data))
	parts := strings.Fields(s)
	if len(parts) != 16 {
		return nil, fmt.Errorf("expected 16 values, got %d", len(parts))
	}

	temps := make([]float64, 16)
	for i, p := range parts {
		val, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("parse value %d (%q): %w", i, p, err)
		}
		temps[i] = val
	}

	for i, j := 0, len(temps)-1; i < j; i, j = i+1, j-1 {
		temps[i], temps[j] = temps[j], temps[i]
	}

	return temps, nil
}

// T1603ParsedFrame is the result of ParseTCPFrameEx with optional
// metadata prefix (sequence number and hardware timestamp).
type T1603ParsedFrame struct {
	HardwareTimestamp float64
	SequenceNumber    int
	Temperatures      []float64
}

// ParseTCPFrameEx parses a TCP frame with optional metadata prefix.
// The device can prefix data with sequence number (@fe HEAD 1) and
// hardware timestamp (@fe TIME 1). The space-separated format is:
//
//	[seq] [timestamp] t1 t2 ... t16
//
// Fixed-width 192-byte and 64-byte binary frames are also supported.
func ParseTCPFrameEx(data []byte) (*T1603ParsedFrame, error) {
	switch len(data) {
	case TCPFrameSize:
		temps, err := parseBinaryFrame(data)
		if err != nil {
			return nil, err
		}
		return &T1603ParsedFrame{Temperatures: temps}, nil
	case 192:
		temps, err := ParseASCIIFrame(data)
		if err != nil {
			return nil, err
		}
		return &T1603ParsedFrame{Temperatures: temps}, nil
	default:
		return parseSpaceSeparatedFrame(data)
	}
}

func parseSpaceSeparatedFrame(data []byte) (*T1603ParsedFrame, error) {
	s := strings.TrimSpace(string(data))
	parts := strings.Fields(s)
	if len(parts) < 16 {
		return nil, fmt.Errorf("expected >= 16 values, got %d", len(parts))
	}

	result := &T1603ParsedFrame{}
	offset := 0

	if len(parts) > 16 {
		seq, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse sequence number %q: %w", parts[0], err)
		}
		result.SequenceNumber = seq
		result.HardwareTimestamp, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %q: %w", parts[1], err)
		}
		offset = 2
	}

	temps := make([]float64, 16)
	for i := 0; i < 16; i++ {
		val, err := strconv.ParseFloat(parts[offset+i], 64)
		if err != nil {
			return nil, fmt.Errorf("parse value %d (%q): %w", i, parts[offset+i], err)
		}
		temps[i] = val
	}

	for i, j := 0, len(temps)-1; i < j; i, j = i+1, j-1 {
		temps[i], temps[j] = temps[j], temps[i]
	}
	result.Temperatures = temps
	return result, nil
}

// -- DAQ-T-1603 TCP frame reader --

// ErrIncompleteFrame is returned when ReadFrame has buffered data but
// not yet received a full frame. The caller should retry.
var ErrIncompleteFrame = fmt.Errorf("incomplete frame: waiting for more data")

// T1603FrameReader reads fixed-size frames from a DAQ-T-1603 device over TCP.
// The device sends either 192-byte ASCII frames or 64-byte binary frames,
// without any length-prefix framing.
type T1603FrameReader struct {
	conn       net.Conn
	mu         sync.Mutex
	buffer     []byte
	binaryMode bool
}

// NewT1603FrameReader creates a frame reader for DAQ-T-1603 TCP data.
func NewT1603FrameReader(conn net.Conn) *T1603FrameReader {
	return &T1603FrameReader{
		conn:   conn,
		buffer: make([]byte, 0, 256),
	}
}

// SetBinaryMode switches between ASCII (false) and binary (true) frame mode.
func (r *T1603FrameReader) SetBinaryMode(binary bool) {
	r.mu.Lock()
	r.binaryMode = binary
	r.mu.Unlock()
}

// ReadFrame reads one complete frame from the device.
// Returns the raw frame bytes on success, or an error (including timeout).
func (r *T1603FrameReader) ReadFrame() ([]byte, error) {
	// Phase 1: check if we already have a full frame (fast path, under lock)
	r.mu.Lock()
	frameSize := r.frameSizeLocked()
	if len(r.buffer) >= frameSize {
		frame := r.extractFrameLocked(frameSize)
		r.mu.Unlock()
		return frame, nil
	}
	r.mu.Unlock()

	// Phase 2: read from connection (blocks, no lock held)
	tmp := make([]byte, frameSize)
	n, err := r.conn.Read(tmp)
	if err != nil {
		return nil, err
	}

	// Phase 3: append to buffer and check for complete frame (under lock)
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buffer = append(r.buffer, tmp[:n]...)

	if len(r.buffer) >= frameSize {
		return r.extractFrameLocked(frameSize), nil
	}

	return nil, ErrIncompleteFrame
}

func (r *T1603FrameReader) frameSizeLocked() int {
	if r.binaryMode {
		return 64
	}
	return 192
}

func (r *T1603FrameReader) extractFrameLocked(size int) []byte {
	frame := make([]byte, size)
	copy(frame, r.buffer[:size])
	r.buffer = r.buffer[size:]
	if len(r.buffer) == 0 {
		r.buffer = make([]byte, 0, 256)
	}
	return frame
}

// PrependBytes 将数据前置到缓冲区开头，用于将误读的数据回退到缓冲区
func (r *T1603FrameReader) PrependBytes(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffer = append(data, r.buffer...)
}

// Reset 清空内部缓冲区，用于停止采集后重置读取器状态，
// 避免残留数据干扰下一次采集的帧解析。
func (r *T1603FrameReader) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffer = make([]byte, 0, 256)
}

// ConsumeOptionalACK drains a leading ACK if present.
// Some firmwares emit a single-byte 'A', others emit "A\n".
// It reads byte-by-byte under a short deadline so split TCP packets do not
// corrupt frame alignment. Non-ACK bytes are preserved in the frame buffer.
func (r *T1603FrameReader) ConsumeOptionalACK(timeout time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	one := make([]byte, 1)
	var first byte
	haveFirst := false
	if len(r.buffer) > 0 {
		first = r.buffer[0]
		r.buffer = r.buffer[1:]
		haveFirst = true
	} else {
		if err := r.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return false, err
		}
		n, err := r.conn.Read(one)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				_ = r.conn.SetReadDeadline(time.Time{})
				return false, nil
			}
			_ = r.conn.SetReadDeadline(time.Time{})
			return false, err
		}
		if n > 0 {
			first = one[0]
			haveFirst = true
		}
	}

	if !haveFirst {
		_ = r.conn.SetReadDeadline(time.Time{})
		return false, nil
	}

	if first != 'A' {
		r.buffer = append([]byte{first}, r.buffer...)
		_ = r.conn.SetReadDeadline(time.Time{})
		return false, nil
	}

	// Support both a single-byte 'A' ACK and the older "A\n" variant.
	if len(r.buffer) > 0 {
		if r.buffer[0] == '\n' {
			r.buffer = r.buffer[1:]
		}
	} else {
		if err := r.conn.SetReadDeadline(time.Now().Add(timeout)); err == nil {
			n, err := r.conn.Read(one)
			if err == nil && n > 0 {
				if one[0] != '\n' {
					r.buffer = append([]byte{one[0]}, r.buffer...)
				}
			} else if err != nil {
				if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
					_ = r.conn.SetReadDeadline(time.Time{})
					return false, err
				}
			}
		}
	}

	_ = r.conn.SetReadDeadline(time.Time{})
	if len(r.buffer) == 0 {
		r.buffer = make([]byte, 0, 256)
	}
	return true, nil
}

// -- DAQ-T-1603 command/response helpers --

const (
	cmdTimeout     = 5 * time.Second
	cmdTailTimeout = 100 * time.Millisecond
	cmdIdleWindow  = 30 * time.Millisecond
	readLineBuffer = 1024
)

// SendCommand sends a text command and reads a newline-terminated response.
func SendCommand(conn net.Conn, cmd string) (string, error) {
	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
		return "", fmt.Errorf("send %q: %w", cmd, err)
	}

	conn.SetReadDeadline(time.Now().Add(cmdTimeout))
	var buf bytes.Buffer
	one := make([]byte, 1)
	for {
		n, err := conn.Read(one)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if buf.Len() > 0 {
					return strings.TrimRight(buf.String(), "\r "), nil
				}
				return "", err
			}
			return "", fmt.Errorf("read response for %q: %w", cmd, err)
		}
		if n == 0 {
			continue
		}
		if one[0] == '\n' {
			return strings.TrimRight(buf.String(), "\r "), nil
		}
		buf.WriteByte(one[0])
		// Some devices return short ASCII payloads without a trailing newline.
		// After the first byte arrives, switch to a short inter-byte timeout so
		// responses like "FFFF" return promptly instead of blocking for cmdTimeout.
		conn.SetReadDeadline(time.Now().Add(cmdTailTimeout))
	}
}

// SendCommandIdle sends a text command and finishes the response after a short
// silent window once at least one byte has arrived. This matches DAQ-T-1603
// query commands that return short ASCII payloads without a trailing newline.
func SendCommandIdle(conn net.Conn, cmd string, idleWindow time.Duration) (string, error) {
	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
		return "", fmt.Errorf("send %q: %w", cmd, err)
	}

	conn.SetReadDeadline(time.Now().Add(cmdTimeout))
	var buf bytes.Buffer
	one := make([]byte, 1)
	for {
		n, err := conn.Read(one)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if buf.Len() > 0 {
					return strings.TrimRight(buf.String(), "\r\n "), nil
				}
				return "", err
			}
			return "", fmt.Errorf("read response for %q: %w", cmd, err)
		}
		if n == 0 {
			continue
		}
		if one[0] == '\n' {
			return strings.TrimRight(buf.String(), "\r\n "), nil
		}
		buf.WriteByte(one[0])
		conn.SetReadDeadline(time.Now().Add(idleWindow))
	}
}

// SendCommandExact sends a text command and reads exactly n bytes as response.
// After reading n bytes, drains any trailing \r\n to avoid corrupting the next command.
func SendCommandExact(conn net.Conn, cmd string, n int) (string, error) {
	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
		return "", fmt.Errorf("send %q: %w", cmd, err)
	}

	conn.SetReadDeadline(time.Now().Add(cmdTimeout))
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", fmt.Errorf("read exact %d for %q: %w", n, cmd, err)
	}

	// Drain trailing line endings byte-by-byte; TCP may split "\r\n".
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	one := make([]byte, 1)
	for i := 0; i < 2; i++ {
		n, err := conn.Read(one)
		if err != nil || n == 0 {
			break
		}
		if one[0] != '\r' && one[0] != '\n' {
			break
		}
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	}

	return string(bytes.TrimRight(buf, "\r\n ")), nil
}
