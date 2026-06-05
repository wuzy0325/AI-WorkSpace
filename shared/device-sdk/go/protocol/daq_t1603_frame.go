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

// ParseTCPFrame parses TCP data frame.
// Auto-detects format based on size:
//   64 bytes  -> binary format (16 x float32 LE)
//   192 bytes -> ASCII text format (16 x 12-char fixed-width fields)
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

	return temperatures, nil
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

// -- DAQ-T-1603 command/response helpers --

const (
	cmdTimeout     = 5 * time.Second
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
