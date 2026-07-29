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
const TCPFrameSizeWithTimestamp = 72 // 8 bytes timestamp header + 64 bytes float32 data

// maxReasonableThermocoupleTemp 是温度校验的上限参考值。
// 基于 K 型热电偶物理量程（-200°C ~ 1350°C），
// 可覆盖 K/S/R/B 等常见热电偶类型，同时排除明显异常的未初始化内存值。
const maxReasonableThermocoupleTemp = 1350.0
const minReasonableThermocoupleTemp = -200.0
const maxImpossibleThermocoupleTemp = 5000.0
const minImpossibleThermocoupleTemp = -1000.0

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
		if temp < minImpossibleThermocoupleTemp || temp > maxImpossibleThermocoupleTemp {
			return false
		}
		if temp >= minReasonableThermocoupleTemp && temp <= maxReasonableThermocoupleTemp {
			reasonableCount++
		}
	}
	// 半数通道在合理温度区间即视为有效帧；
	// 16 路设备常有 5~7 路未接热电偶（读数饱和/NaN），不能因此判定整帧错位。
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
	if !looksLikeReasonableTemperatureFrame(temps) {
		return nil, fmt.Errorf("ASCII frame values out of expected range")
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

// parseBinaryFrameWithTimestamp parses a 72-byte binary frame with timestamp header.
// Format: [uint32 seconds LE][uint32 nanoseconds LE][16 × float32 LE]
// Channel order from device is CH15→CH0, results are reversed to CH0→CH15.
func parseBinaryFrameWithTimestamp(data []byte) (*T1603ParsedFrame, error) {
	if len(data) != TCPFrameSizeWithTimestamp {
		return nil, fmt.Errorf("invalid frame size: %d, expected %d", len(data), TCPFrameSizeWithTimestamp)
	}

	sec := binary.LittleEndian.Uint32(data[0:4])
	ns := binary.LittleEndian.Uint32(data[4:8])
	ts := float64(sec) + float64(ns)/1e9

	temps := make([]float64, 16)
	for i := 0; i < 16; i++ {
		bits := binary.LittleEndian.Uint32(data[8+i*4:])
		temps[i] = float64(math.Float32frombits(bits))
	}

	for i, j := 0, len(temps)-1; i < j; i, j = i+1, j-1 {
		temps[i], temps[j] = temps[j], temps[i]
	}
	if !looksLikeReasonableTemperatureFrame(temps) {
		return nil, fmt.Errorf("binary frame values out of expected range")
	}

	return &T1603ParsedFrame{
		HardwareTimestamp: ts,
		Temperatures:      temps,
	}, nil
}

// ParseTCPFrameEx parses a TCP frame with optional metadata prefix.
// The device can prefix data with sequence number (@fe HEAD 1) and
// hardware timestamp (@fe TIME 1). The space-separated format is:
//
//	[seq] [timestamp] t1 t2 ... t16
//
// Fixed-width 192-byte ASCII, 64-byte binary, and 72-byte binary-with-timestamp
// frames are also supported.
func ParseTCPFrameEx(data []byte) (*T1603ParsedFrame, error) {
	switch len(data) {
	case TCPFrameSize:
		temps, err := parseBinaryFrame(data)
		if err != nil {
			return nil, err
		}
		return &T1603ParsedFrame{Temperatures: temps}, nil
	case TCPFrameSizeWithTimestamp:
		return parseBinaryFrameWithTimestamp(data)
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
		if err == nil {
			result.SequenceNumber = seq
			offset = 1
			if len(parts) > 17 {
				ts, err := strconv.ParseFloat(parts[1], 64)
				if err == nil {
					result.HardwareTimestamp = ts
					offset = 2
				}
			}
		} else {
			ts, err := strconv.ParseFloat(parts[0], 64)
			if err != nil {
				return nil, fmt.Errorf("parse metadata token %q: %w", parts[0], err)
			}
			result.HardwareTimestamp = ts
			offset = 1
		}
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
	if !looksLikeReasonableTemperatureFrame(temps) {
		return nil, fmt.Errorf("space-separated frame values out of expected range")
	}
	result.Temperatures = temps
	return result, nil
}

// -- DAQ-T-1603 TCP frame reader --

// ErrIncompleteFrame is returned when ReadFrame has buffered data but
// not yet received a full frame. The caller should retry.
var ErrIncompleteFrame = fmt.Errorf("incomplete frame: waiting for more data")

// T1603FrameReader reads frames from a DAQ-T-1603 device over TCP.
// Mode combinations:
//   - BIN=0, metadata=false → 192-byte fixed ASCII
//   - BIN=1, metadata=false → 64-byte fixed binary
//   - BIN=0, metadata=true  → newline-terminated variable ASCII (TIME/HEAD)
//   - BIN=1, metadata=true  → 72-byte fixed binary with timestamp header
type T1603FrameReader struct {
	conn         net.Conn
	mu           sync.Mutex
	buffer       []byte
	binaryMode   bool
	metadataMode bool
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

// SetMetadataMode enables metadata prefix mode.
// When true and binaryMode is false, reads newline-terminated variable-length
// ASCII frames. When true and binaryMode is true, reads 72-byte fixed frames
// with 8-byte hardware timestamp header.
// Set true when @fe TIME or @fe HEAD is enabled on the device.
func (r *T1603FrameReader) SetMetadataMode(metadata bool) {
	r.mu.Lock()
	r.metadataMode = metadata
	r.mu.Unlock()
}

// ReadFrame reads one complete frame from the device.
// Routing:
//   - metadataMode + binaryMode     → 72-byte fixed binary with timestamp
//   - metadataMode + !binaryMode    → variable-length newline-terminated text
//   - !metadataMode + binaryMode    → 64-byte fixed binary
//   - !metadataMode + !binaryMode   → 192-byte fixed ASCII
func (r *T1603FrameReader) ReadFrame() ([]byte, error) {
	r.mu.Lock()
	mm := r.metadataMode
	bm := r.binaryMode
	r.mu.Unlock()

	// BIN=1, TIME/HEAD=1: 72-byte fixed binary with timestamp header
	// BIN=0, TIME/HEAD=1: variable-length newline-terminated ASCII
	// BIN=1, TIME/HEAD=0: 64-byte fixed binary
	// BIN=0, TIME/HEAD=0: 192-byte fixed ASCII
	if mm && !bm {
		return r.readFrameVariable()
	}
	return r.readFrameFixed()
}

func (r *T1603FrameReader) readFrameFixed() ([]byte, error) {
	r.mu.Lock()
	frameSize := r.frameSizeLocked()
	if frame, ok := r.extractValidFixedFrameLocked(frameSize); ok {
		r.mu.Unlock()
		return frame, nil
	}
	r.mu.Unlock()

	tmp := make([]byte, frameSize)
	for {
		n, err := r.conn.Read(tmp)
		if err != nil {
			return nil, err
		}
		// 防御 (0, nil)：net.Conn 在边缘情况下（清除 deadline 期间、零字节
		// TCP 段）可能返回 0 字节且无 error。作为瞬态处理，continue 而非
		// 返回错误，避免单次零字节读取终止整个采集循环。
		if n == 0 {
			continue
		}

		r.mu.Lock()
		r.buffer = append(r.buffer, tmp[:n]...)
		if frame, ok := r.extractValidFixedFrameLocked(frameSize); ok {
			r.mu.Unlock()
			return frame, nil
		}
		r.mu.Unlock()
	}
}

func (r *T1603FrameReader) extractValidFixedFrameLocked(size int) ([]byte, bool) {
	for len(r.buffer) >= size {
		frame := make([]byte, size)
		copy(frame, r.buffer[:size])
		if _, err := ParseTCPFrameEx(frame); err == nil {
			r.buffer = r.buffer[size:]
			if len(r.buffer) == 0 {
				r.buffer = make([]byte, 0, 256)
			}
			return frame, true
		}

		// A delayed command ACK or residual byte shifts every fixed-size frame.
		// Drop one byte and retry before exposing a corrupted frame upstream.
		r.buffer = r.buffer[1:]
	}
	return nil, false
}

// findFieldEnd scans buf for N space-separated fields and returns the
// byte index right after the Nth field. Returns -1 if fewer than N fields exist.
func findFieldEnd(buf []byte, n int) int {
	count := 0
	inField := false
	for i, b := range buf {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			if inField {
				count++
				if count == n {
					return i
				}
			}
			inField = false
		} else {
			if !inField {
				inField = true
			}
		}
	}
	if inField {
		count++
		if count == n {
			return len(buf)
		}
	}
	return -1
}

// tryExtractFrame attempts to extract a complete metadata frame from the buffer.
// Returns the frame and the end position, or -1 if no complete frame is available.
// Tries 18 tokens first (HEAD+TIME), then 17 (HEAD-only or TIME-only).
func tryExtractFrame(buf []byte) ([]byte, int) {
	for _, need := range []int{18, 17} {
		if end := findFieldEnd(buf, need); end >= 0 {
			if _, err := ParseTCPFrameEx(buf[:end]); err == nil {
				frame := make([]byte, end)
				copy(frame, buf[:end])
				return frame, end
			}
		}
	}
	return nil, -1
}

func (r *T1603FrameReader) readFrameVariable() ([]byte, error) {
	r.mu.Lock()
	if frame, end := tryExtractFrame(r.buffer); end >= 0 {
		r.buffer = r.buffer[end:]
		if len(r.buffer) == 0 {
			r.buffer = make([]byte, 0, 256)
		}
		r.mu.Unlock()
		return frame, nil
	}
	r.mu.Unlock()

	tmp := make([]byte, 256)
	for {
		n, err := r.conn.Read(tmp)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			// 零字节 TCP 段是瞬态，continue 避免终止采集循环
			continue
		}

		r.mu.Lock()
		r.buffer = append(r.buffer, tmp[:n]...)
		if frame, end := tryExtractFrame(r.buffer); end >= 0 {
			r.buffer = r.buffer[end:]
			if len(r.buffer) == 0 {
				r.buffer = make([]byte, 0, 256)
			}
			r.mu.Unlock()
			return frame, nil
		}
		r.mu.Unlock()
	}
}

func (r *T1603FrameReader) frameSizeLocked() int {
	if r.binaryMode {
		if r.metadataMode {
			return TCPFrameSizeWithTimestamp // 72 bytes: 8 header + 64 float32
		}
		return TCPFrameSize // 64 bytes
	}
	return 192
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

// Resync 丢弃缓冲区中 1 字节数据用于重同步。
// 仅在缓冲区有数据时生效；缓冲区为空则不做任何 I/O，
// 等待下一次 ReadFrame 读到新数据后再由调用方决定是否再次 Resync。
// 这样保证 Resync 不会持锁阻塞在 conn.Read 上，避免与
// SetBinaryMode/SetMetadataMode/Reset 等持锁操作互相阻塞。
func (r *T1603FrameReader) Resync() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.buffer) > 0 {
		// 缓冲区有数据，丢弃其中首字节
		r.buffer = r.buffer[1:]
		if len(r.buffer) == 0 {
			r.buffer = make([]byte, 0, 256)
		}
	}
}

// ConsumeOptionalACK drains a leading ACK if present.
// Some firmwares emit a single-byte 'A', others emit "A\n".
// It reads byte-by-byte under a short deadline so split TCP packets do not
// corrupt frame alignment. Non-ACK bytes are preserved in the frame buffer.
//
// ADR-009 watchdog：持 r.mu 期间阻塞 r.conn.Read，watchdog 必须能
// 不依赖 r.mu 直接 Close conn 解除阻塞。watchdog 触发时返回 net.ErrClosed
// 包装的错误，调用方（StartAcquisition）必须把连接标记为失效并重连。
func (r *T1603FrameReader) ConsumeOptionalACK(timeout time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// watchdog 兜底：timeout 后强制 Close r.conn 解除阻塞的 Read。
	// 与 SendCommand 同模式：用 wdChecked 缓存结果避免多次调用 wdStop 死锁。
	wdStop := WatchdogClose(r.conn, timeout)
	var wdResult bool
	wdChecked := false
	checkWd := func() bool {
		if !wdChecked {
			wdResult = wdStop()
			wdChecked = true
		}
		return wdResult
	}

	var resultErr error
	consumed := false

	one := make([]byte, 1)
	var first byte
	haveFirst := false
	if len(r.buffer) > 0 {
		first = r.buffer[0]
		r.buffer = r.buffer[1:]
		haveFirst = true
	} else {
		if err := r.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			resultErr = err
		} else {
			n, err := r.conn.Read(one)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 无 ACK 可读，不是错误；watchdog 仍可能在随后触发
				} else {
					resultErr = err
				}
			} else if n > 0 {
				first = one[0]
				haveFirst = true
			}
		}
	}

	if resultErr == nil && haveFirst {
		if first != 'A' {
			// 非 ACK 字节回退到 buffer 前部，保留给后续 ReadFrame
			r.buffer = append([]byte{first}, r.buffer...)
		} else {
			consumed = true
			// 处理可能的 \n 尾部
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
							resultErr = err
						}
					}
				}
			}
		}
	}

	// 统一在返回前检查 watchdog 状态
	if !checkWd() {
		if resultErr == nil {
			resultErr = net.ErrClosed
		}
		return false, fmt.Errorf("%w (watchdog triggered, conn closed)", resultErr)
	}
	if resultErr == nil {
		// watchdog 未触发且无错误：清 deadline，避免残留 timeout 影响后续 ReadFrame
		_ = r.conn.SetReadDeadline(time.Time{})
		if len(r.buffer) == 0 {
			r.buffer = make([]byte, 0, 256)
		}
		return consumed, nil
	}
	return false, resultErr
}

// -- DAQ-T-1603 command/response helpers --

const (
	cmdTimeout     = time.Second
	cmdTailTimeout = 100 * time.Millisecond
	cmdIdleWindow  = 30 * time.Millisecond
	readLineBuffer = 1024
	// cmdWatchdogTimeout 是 SendCommand/SendCommandIdle/SendCommandExact 的
	// watchdog 兜底超时。设为 2*cmdTimeout 覆盖 Write(1s) + 首字节 Read(1s)
	// 的最坏情况；后续字节由 cmdTailTimeout(100ms) deadline 在正常路径兜底。
	// ADR-009：deadline 在某些 Windows 电脑不可靠，watchdog 是独立兜底机制。
	cmdWatchdogTimeout = 2 * cmdTimeout
)

// SendCommand sends a text command and reads a newline-terminated response.
//
// ADR-009 watchdog：入口启动 WatchdogClose 兜底，deadline 失效时强制 Close conn
// 解除 Read 阻塞。成功路径在 watchdog 未触发时清 deadline，避免残留 cmdTailTimeout
// 影响后续命令（daq-t1603 0.7.2 修复点）。
func SendCommand(conn net.Conn, cmd string) (string, error) {
	// watchdog 兜底：timeout 内未完成则强制 Close conn 解除阻塞。
	// 返回的 wdStop 必须在函数返回前调用一次以取消计时器；
	// 多次调用在第一次返回 true 后会死锁（timer.Stop 行为），故用 wdCalled 缓存结果。
	wdStop := WatchdogClose(conn, cmdWatchdogTimeout)
	var wdResult bool
	wdChecked := false
	checkWd := func() bool {
		if !wdChecked {
			wdResult = wdStop()
			wdChecked = true
		}
		return wdResult
	}

	var buf bytes.Buffer
	var resultErr error
	var partialResponse string

	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		resultErr = fmt.Errorf("send %q: %w", cmd, err)
	} else {
		conn.SetReadDeadline(time.Now().Add(cmdTimeout))
		one := make([]byte, 1)
		for {
			n, err := conn.Read(one)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// deadline 触发：有部分响应则返回，否则返回错误
					if buf.Len() > 0 {
						partialResponse = strings.TrimRight(buf.String(), "\r ")
						break
					}
					resultErr = err
					break
				}
				resultErr = fmt.Errorf("read response for %q: %w", cmd, err)
				break
			}
			if n == 0 {
				continue
			}
			if one[0] == '\n' {
				partialResponse = strings.TrimRight(buf.String(), "\r ")
				break
			}
			buf.WriteByte(one[0])
			conn.SetReadDeadline(time.Now().Add(cmdTailTimeout))
		}
	}

	// 统一在返回前检查 watchdog 状态：
	// - 触发（false）：conn 已 Close，丢弃部分响应并附加上下文
	// - 未触发（true）：conn 仍有效，清 deadline
	if !checkWd() {
		if resultErr == nil {
			resultErr = net.ErrClosed
		}
		return "", fmt.Errorf("%w; %w", resultErr, ErrWatchdogTriggered)
	}
	// 清除 Read/Write deadline，避免过期的绝对时间影响后续命令。
	// watchdog 未触发才到达此处，conn 仍有效，SetDeadline 失败可忽略。
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
	if resultErr != nil {
		return "", resultErr
	}
	return partialResponse, nil
}

// SendCommandIdle sends a text command and finishes the response after a short
// silent window once at least one byte has arrived. This matches DAQ-T-1603
// query commands that return short ASCII payloads without a trailing newline.
//
// ADR-009 watchdog：与 SendCommand 同模式，watchdog 兜底 + 成功路径清 deadline。
func SendCommandIdle(conn net.Conn, cmd string, idleWindow time.Duration) (string, error) {
	wdStop := WatchdogClose(conn, cmdWatchdogTimeout)
	var wdResult bool
	wdChecked := false
	checkWd := func() bool {
		if !wdChecked {
			wdResult = wdStop()
			wdChecked = true
		}
		return wdResult
	}

	var buf bytes.Buffer
	var resultErr error
	var partialResponse string

	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		resultErr = fmt.Errorf("send %q: %w", cmd, err)
	} else {
		conn.SetReadDeadline(time.Now().Add(cmdTimeout))
		one := make([]byte, 1)
		for {
			n, err := conn.Read(one)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					if buf.Len() > 0 {
						partialResponse = strings.TrimRight(buf.String(), "\r\n ")
						break
					}
					resultErr = err
					break
				}
				resultErr = fmt.Errorf("read response for %q: %w", cmd, err)
				break
			}
			if n == 0 {
				continue
			}
			if one[0] == '\n' {
				partialResponse = strings.TrimRight(buf.String(), "\r\n ")
				break
			}
			buf.WriteByte(one[0])
			conn.SetReadDeadline(time.Now().Add(idleWindow))
		}
	}

	if !checkWd() {
		if resultErr == nil {
			resultErr = net.ErrClosed
		}
		return "", fmt.Errorf("%w; %w", resultErr, ErrWatchdogTriggered)
	}
	// 清除 Read/Write deadline，避免过期的绝对时间影响后续命令。
	// watchdog 未触发才到达此处，conn 仍有效，SetDeadline 失败可忽略。
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
	if resultErr != nil {
		return "", resultErr
	}
	return partialResponse, nil
}

// SendCommandExact sends a text command and reads exactly n bytes as response.
// After reading n bytes, drains any trailing \r\n to avoid corrupting the next command.
//
// ADR-009 watchdog：io.ReadFull 内部循环 conn.Read 不重设 deadline，
// 必须由外层 watchdog 兜底；watchdog 触发时 conn 被 Close，io.ReadFull
// 返回 io.ErrUnexpectedEOF 或 net.ErrClosed，统一包装为 "watchdog triggered"。
func SendCommandExact(conn net.Conn, cmd string, n int) (string, error) {
	wdStop := WatchdogClose(conn, cmdWatchdogTimeout)
	var wdResult bool
	wdChecked := false
	checkWd := func() bool {
		if !wdChecked {
			wdResult = wdStop()
			wdChecked = true
		}
		return wdResult
	}

	var resultErr error
	var response string

	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		resultErr = fmt.Errorf("send %q: %w", cmd, err)
	} else {
		conn.SetReadDeadline(time.Now().Add(cmdTimeout))
		buf := make([]byte, n)
		if _, err := io.ReadFull(conn, buf); err != nil {
			resultErr = fmt.Errorf("read exact %d for %q: %w", n, cmd, err)
		} else {
			// 排空尾部 \r\n；watchdog 仍生效，覆盖尾部读取的 deadline 失效场景
			conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			one := make([]byte, 1)
			for i := 0; i < 2; i++ {
				rn, err := conn.Read(one)
				if err != nil || rn == 0 {
					break
				}
				if one[0] != '\r' && one[0] != '\n' {
					break
				}
				conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			}
			response = string(bytes.TrimRight(buf, "\r\n "))
		}
	}

	if !checkWd() {
		if resultErr == nil {
			resultErr = net.ErrClosed
		}
		return "", fmt.Errorf("%w; %w", resultErr, ErrWatchdogTriggered)
	}
	// 清除 Read/Write deadline，避免过期的绝对时间影响后续命令。
	// watchdog 未触发才到达此处，conn 仍有效，SetDeadline 失败可忽略。
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})
	if resultErr != nil {
		return "", resultErr
	}
	return response, nil
}
