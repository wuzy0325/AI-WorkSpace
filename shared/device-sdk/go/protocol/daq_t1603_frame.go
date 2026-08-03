package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
// TCPFrameSizeWithSequence 是 HEAD=1 时的二进制帧长：4 字节帧序号头 + 64 字节 float32 数据。
// 2026-08-03 实机验证（FW 1.04）：HEAD=1 时帧 = [seq uint32 LE][16×float32 LE]。
const TCPFrameSizeWithSequence = 68

// TCPFrameSizeWithSequenceAndTimestamp 是 HEAD=1 且 TIME=1 时的二进制帧长：
// 4 字节序号头 + 8 字节时间戳头 + 64 字节 float32 数据。
// 2026-08-03 实机验证：帧 = [seq uint32 LE][sec uint32 LE][ns uint32 LE][16×float32 LE]。
const TCPFrameSizeWithSequenceAndTimestamp = 76

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
	for _, temp := range temps {
		if math.IsNaN(temp) || math.IsInf(temp, 0) {
			// NaN / Inf 是未接热电偶的正常读数，跳过
			continue
		}
		// 任何值超出物理不可能范围（-1000°C ~ 5000°C）说明这不是温度帧，
		// 立即拒绝。数据帧长度固定（64/72/192 字节），边界由帧长保证，
		// 只要所有非 NaN 值在物理可能范围内即可认定为有效帧。
		if temp < minImpossibleThermocoupleTemp || temp > maxImpossibleThermocoupleTemp {
			return false
		}
	}
	return true
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

// parseBinaryFrameWithSequence parses a 68-byte binary frame with a sequence header.
// Format: [uint32 seq LE][16 × float32 LE]
// Channel order from device is CH15→CH0, results are reversed to CH0→CH15.
func parseBinaryFrameWithSequence(data []byte) (*T1603ParsedFrame, error) {
	if len(data) != TCPFrameSizeWithSequence {
		return nil, fmt.Errorf("invalid frame size: %d, expected %d", len(data), TCPFrameSizeWithSequence)
	}
	seq := binary.LittleEndian.Uint32(data[0:4])
	temps := make([]float64, 16)
	for i := 0; i < 16; i++ {
		temps[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[4+i*4:])))
	}
	for i, j := 0, len(temps)-1; i < j; i, j = i+1, j-1 {
		temps[i], temps[j] = temps[j], temps[i]
	}
	if !looksLikeReasonableTemperatureFrame(temps) {
		return nil, fmt.Errorf("binary frame values out of expected range")
	}
	return &T1603ParsedFrame{SequenceNumber: int(seq), Temperatures: temps}, nil
}

// parseBinaryFrameWithSequenceAndTimestamp parses a 76-byte binary frame with
// sequence and timestamp headers.
// Format: [uint32 seq LE][uint32 sec LE][uint32 ns LE][16 × float32 LE]
func parseBinaryFrameWithSequenceAndTimestamp(data []byte) (*T1603ParsedFrame, error) {
	if len(data) != TCPFrameSizeWithSequenceAndTimestamp {
		return nil, fmt.Errorf("invalid frame size: %d, expected %d", len(data), TCPFrameSizeWithSequenceAndTimestamp)
	}
	seq := binary.LittleEndian.Uint32(data[0:4])
	sec := binary.LittleEndian.Uint32(data[4:8])
	ns := binary.LittleEndian.Uint32(data[8:12])
	ts := float64(sec) + float64(ns)/1e9
	temps := make([]float64, 16)
	for i := 0; i < 16; i++ {
		temps[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[12+i*4:])))
	}
	for i, j := 0, len(temps)-1; i < j; i, j = i+1, j-1 {
		temps[i], temps[j] = temps[j], temps[i]
	}
	if !looksLikeReasonableTemperatureFrame(temps) {
		return nil, fmt.Errorf("binary frame values out of expected range")
	}
	return &T1603ParsedFrame{SequenceNumber: int(seq), HardwareTimestamp: ts, Temperatures: temps}, nil
}

// ParseTCPFrameEx parses a TCP frame with optional metadata prefix.
// The device can prefix data with sequence number (@fe HEAD 1) and
// hardware timestamp (@fe TIME 1). The space-separated format is:
//
//	[seq] [timestamp] t1 t2 ... t16
//
// Fixed-width 192-byte ASCII, 64-byte binary, 68-byte binary-with-sequence,
// 72-byte binary-with-timestamp, and 76-byte binary-with-sequence-and-timestamp
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
	case TCPFrameSizeWithSequence:
		return parseBinaryFrameWithSequence(data)
	case TCPFrameSizeWithSequenceAndTimestamp:
		return parseBinaryFrameWithSequenceAndTimestamp(data)
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

// ErrControlACK reports a complete A response at a frame boundary.
var ErrControlACK = errors.New("DAQ-T-1603 control command acknowledged")

// stopResponseQuietWindow 是 Stop 响应收集的静默确认窗口：边界未就绪
// （帧/ACK 分段未到齐）时等待后续数据段的时间上限。窗口内数据到达即
// 收集，窗口到期仍未就绪则按 buffer 内容判定协议错位（报错毒化）或
// 数据缺失（由上层 Stop 兜底收尾）。实现不依赖 SetReadDeadline
// （部分 Windows 机器 deadline 取消失效会永久阻塞）。
const stopResponseQuietWindow = 150 * time.Millisecond

// ErrDeviceRejected 表示设备对设置命令（@fe / @f3 等）返回 E 拒绝响应。
//
// 语义边界（ADR-009 复核修订）：
//   - E 是设备发出的合法、完整的错误响应，连接协议边界仍可信；
//   - 因此本错误**不**触发 ADR-009 连接毒化（invalidateConnection）；
//   - 调用方应终止当前操作并上报错误，不应继续执行后续命令。
//
// 与 ErrWatchdogTriggered 的区别：
//   - ErrWatchdogTriggered 表示协议边界不可信，必须毒化连接；
//   - ErrDeviceRejected 表示设备业务层拒绝，连接可继续复用。
//
// 调用方通过 errors.Is(err, ErrDeviceRejected) 精确匹配 sentinel，
// 决定是否跳过毒化路径。
var ErrDeviceRejected = errors.New("device rejected command (E response)")

// T1603FrameReader reads frames from a DAQ-T-1603 device over TCP.
// Mode combinations (binary):
//   - BIN=0, metadata=false → 192-byte fixed ASCII
//   - BIN=1, metadata=false → 64-byte fixed binary
//   - BIN=0, metadata=true  → newline-terminated variable ASCII (TIME/HEAD)
//   - BIN=1, metadata=true  → 72-byte fixed binary with timestamp header
//   - BIN=1, sequence=true  → 68-byte fixed binary with sequence header
//   - BIN=1, metadata+seq   → 76-byte fixed binary with seq + timestamp headers
type T1603FrameReader struct {
	conn         net.Conn
	mu           sync.Mutex
	buffer       []byte
	binaryMode   bool
	metadataMode bool
	sequenceMode bool
	ackAfterData []bool
	stopReady    bool
	// deadlineBroken 由上层（DAQT1603，连接建立后空窗期探测）注入：
	// 部分 Windows 机器（安全软件 LSP hook winsock）SetReadDeadline 失效，
	// 阻塞 Read 永不返回。true 时 Stop 静默确认窗口改用 goroutine + 定时器
	// 实现，不依赖 deadline；该路径窗口到期会遗留阻塞读 goroutine，连接
	// 不可复用（dirty），Stop 完成后由上层废弃。
	deadlineBroken bool
	// dirty 标记本连接遗留了阻塞读 goroutine（deadlineBroken 机器的静默
	// 窗口到期时发生）：遗留读会抢走连接上后续事务的数据，连接不可安全
	// 复用，Stop 完成后由上层废弃（自动重连兜底）。
	dirty bool
	// stopTailCh 仅在静默窗口到期、阻塞读 goroutine 仍未返回时赋值；其
	// 后续读到的数据由下一轮 readFrameFixed 循环顶部 drainStopTail 消费
	// 归队，保证字节顺序不被打乱。
	stopTailCh chan stopReadRes
}

// stopReadRes 是 collectingStop 静默窗口内 goroutine 阻塞读的结果。
type stopReadRes struct {
	n   int
	err error
	buf []byte
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

// IsBinaryMode 返回当前 binaryMode 状态，用于测试断言与诊断。
// 与 SetBinaryMode 配对的只读访问器，避免外部直接访问 mu 保护的字段。
func (r *T1603FrameReader) IsBinaryMode() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.binaryMode
}

// SetMetadataMode enables metadata prefix mode.
// When true and binaryMode is false, reads newline-terminated variable-length
// ASCII frames. When true and binaryMode is true, reads 72-byte fixed frames
// with 8-byte hardware timestamp header.
// Set true when @fe TIME is enabled on the device.
func (r *T1603FrameReader) SetMetadataMode(metadata bool) {
	r.mu.Lock()
	r.metadataMode = metadata
	r.mu.Unlock()
}

// SetSequenceMode enables the frame-sequence header mode (@fe HEAD 1).
// When true and binaryMode is true, reads 68-byte fixed frames with a 4-byte
// sequence header; combined with metadata mode reads 76-byte frames with
// sequence + timestamp headers.
func (r *T1603FrameReader) SetSequenceMode(sequence bool) {
	r.mu.Lock()
	r.sequenceMode = sequence
	r.mu.Unlock()
}

// IsSequenceMode 返回当前 sequenceMode 状态，用于测试断言。
func (r *T1603FrameReader) IsSequenceMode() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sequenceMode
}

// ExpectControlACK makes the leading A returned by @f0 visible. 实机约 80% 的启动
// 首字节为 'A'（ACK 前导）；其余启动中 ACK 偶发缺失或数据优先到达（SKILL.md
// §3.3.4）。inspectFixedBufferLocked 通过偏移0/偏移1帧合法性自动对齐，不再要求
// 首字节必须为 'A'。
func (r *T1603FrameReader) ExpectControlACK() {
	r.mu.Lock()
	r.ackAfterData = append(r.ackAfterData, false)
	r.mu.Unlock()
}

// ExpectControlACKAfterFrames expects a terminal ACK after zero or more
// complete frames, as observed for @f1 on the real device.
func (r *T1603FrameReader) ExpectControlACKAfterFrames() {
	r.mu.Lock()
	r.ackAfterData = append(r.ackAfterData, true)
	r.mu.Unlock()
}

// HasPendingControlACK reports whether an earlier control command still has an
// ACK in flight. It lets the connection owner distinguish start and stop ACKs.
func (r *T1603FrameReader) HasPendingControlACK() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ackAfterData) > 0
}

// ReadFrame reads one complete frame from the device.
// Routing:
//   - metadataMode + binaryMode     → 72-byte fixed binary with timestamp
//   - sequenceMode + binaryMode     → 68-byte fixed binary with seq header
//   - metadata+sequence + binary    → 76-byte fixed binary with seq + ts headers
//   - (metadata|sequence) + !binary → variable-length newline-terminated text
//   - !metadata + !sequence + binary → 64-byte fixed binary
//   - !metadata + !sequence + !binary → 192-byte fixed ASCII
func (r *T1603FrameReader) ReadFrame() ([]byte, error) {
	r.mu.Lock()
	mm := r.metadataMode
	sm := r.sequenceMode
	bm := r.binaryMode
	r.mu.Unlock()

	// BIN=0, TIME/HEAD=1: variable-length newline-terminated ASCII
	// (seq-only ASCII is 17 fields, seq+ts ASCII is 18 fields, handled by
	// tryExtractFrame / parseSpaceSeparatedFrame).
	if (mm || sm) && !bm {
		return r.readFrameVariable()
	}
	return r.readFrameFixed()
}

func (r *T1603FrameReader) readFrameFixed() ([]byte, error) {
	for {
		if err := r.drainStopTail(); err != nil {
			// 遗留读以错误结束（EOF/closed/timeout 等），交回正常错误路径。
			return nil, err
		}
		r.mu.Lock()
		frameSize := r.frameSizeLocked()
		frame, ack, collectingStop, err := r.inspectFixedBufferLocked(frameSize)
		r.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if ack {
			return nil, ErrControlACK
		}
		if frame != nil {
			return frame, nil
		}

		if collectingStop {
			// Stop 响应收集：静默确认窗口。窗口必须完整走完：A-leading
			// 尾帧的首字节 'A' 与 N=0 ACK 无法从内容区分，立即 finalize
			// 会把帧首字节误判为 Stop ACK。
			//
			// deadline 失效的机器（安全软件 LSP hook winsock，探测由上层
			// 在连接建立后完成并注入 SetDeadlineBroken）不能依赖
			// SetReadDeadline 结束窗口——旧实现此时阻塞 Read 永久卡死，
			// Stop 掉到 350ms 兜底废弃连接（2026-07-31 实机复现 + 裸 TCP
			// 探针验证：@f1 后设备实际已回 1 字节 'A'，卡点在此窗口的
			// Read 上）。
			if !r.deadlineBroken {
				// deadline 生效（正常机器）：同步窗口——SetReadDeadline
				// 到期即静默确认，无遗留读 goroutine，连接可安全复用。
				slog.Debug("Stop quiet window: sync path (deadline works)")
				_ = r.conn.SetReadDeadline(time.Now().Add(stopResponseQuietWindow))
				tmp := make([]byte, frameSize)
				n, err := r.conn.Read(tmp)
				if err != nil {
					if isTimeoutError(err) {
						r.mu.Lock()
						ready, finalizeErr := r.finalizeStopResponseLocked(frameSize)
						r.mu.Unlock()
						if finalizeErr != nil {
							return nil, finalizeErr
						}
						if ready {
							continue
						}
						// buffer 为空：设备未回 ACK，由 readLoop 重试，
						// 最终由上层 quietFallback（350ms）收尾废弃连接。
						return nil, ErrIncompleteFrame
					}
					return nil, err
				}
				if n == 0 {
					continue
				}
				r.mu.Lock()
				r.buffer = append(r.buffer, tmp[:n]...)
				r.mu.Unlock()
				continue
			}
			// deadline 失效（问题机器）：goroutine 阻塞读 + 固定 150ms
			// 窗口——窗口内数据到达即收集（goroutine 收到数据即退出，
			// 无残留），窗口到期先非阻塞收取竞争到达的数据，仍无数据则
			// 挂 stopTailCh 归队并标记 dirty（连接不可复用，由上层废弃）。
			// 到期后按 buffer 验证 N*frameSize+ACK 边界：合法 → 完成收集；
			// 有内容但非法 → 协议错位报错；空 → ErrIncompleteFrame 重试，
			// 最终由上层 quietFallback（350ms）收尾。
			slog.Debug("Stop quiet window: goroutine path (deadline broken)", "conn", r.conn.RemoteAddr())
			deadline := time.Now().Add(stopResponseQuietWindow)
			for {
				if time.Now().After(deadline) {
					break
				}
				ch := make(chan stopReadRes, 1)
				go func() {
					tmp := make([]byte, frameSize)
					n, err := r.conn.Read(tmp)
					ch <- stopReadRes{n: n, err: err, buf: tmp}
				}()
				select {
				case res := <-ch:
					if res.n > 0 {
						r.mu.Lock()
						r.buffer = append(r.buffer, res.buf[:res.n]...)
						r.mu.Unlock()
					}
					if res.err != nil {
						return nil, res.err
					}
				case <-time.After(time.Until(deadline)):
					select {
					case res := <-ch:
						if res.n > 0 {
							r.mu.Lock()
							r.buffer = append(r.buffer, res.buf[:res.n]...)
							r.mu.Unlock()
						}
						if res.err != nil {
							return nil, res.err
						}
					default:
						// goroutine 仍在阻塞：遗留读会抢走后续事务数据，
						// 连接不可复用。数据经 stopTailCh 归队（保顺序），
						// 连接由上层 Stop 完成后废弃（自动重连兜底）。
						r.dirty = true
						r.stopTailCh = ch
					}
				}
			}
			r.mu.Lock()
			ready, finalizeErr := r.finalizeStopResponseLocked(frameSize)
			r.mu.Unlock()
			if finalizeErr != nil {
				return nil, finalizeErr
			}
			if ready {
				continue
			}
			return nil, ErrIncompleteFrame
		}
		tmp := make([]byte, frameSize)
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
		r.mu.Unlock()
	}
}

// SetDeadlineBroken 注入本连接 SetReadDeadline 是否失效的探测结果
// （由上层 DAQT1603 在连接建立后的空窗期探测，见 probeDeadlineBroken）。
// true 时 Stop 静默确认窗口改用 goroutine + 定时器（不依赖 deadline）。
func (r *T1603FrameReader) SetDeadlineBroken(broken bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deadlineBroken = broken
}

// IsDirty 报告本连接是否遗留了阻塞读 goroutine（deadline 失效机器的静默
// 窗口到期时发生）。遗留读会抢走连接上后续事务的数据，连接不可安全复用，
// 上层应在 Stop 完成后废弃连接（自动重连兜底）。
func (r *T1603FrameReader) IsDirty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dirty
}

// drainStopTail 消费 collectingStop 静默窗口遗留的读结果：窗口到期时阻塞
// 读 goroutine 可能仍在等待，其后续读到的数据（含连接复用后新事务的首批
// 数据）经 stopTailCh 归队，保证字节顺序不被打乱。无遗留时零开销返回 nil。
// 调用方不应持锁；返回非 nil error 表示遗留读以错误结束。
func (r *T1603FrameReader) drainStopTail() error {
	ch := r.stopTailCh
	if ch == nil {
		return nil
	}
	select {
	case res := <-ch:
		if res.n > 0 {
			r.mu.Lock()
			r.buffer = append(r.buffer, res.buf[:res.n]...)
			r.mu.Unlock()
		}
		return res.err
	default:
		return nil
	}
}

func (r *T1603FrameReader) inspectFixedBufferLocked(size int) ([]byte, bool, bool, error) {
	if len(r.ackAfterData) == 0 {
		frame, ok, err := r.extractFixedFrameLocked(size)
		if err != nil {
			// 正常采集路径的单字节自愈：边界帧非法时，丢弃首字节重试一次。
			// 该字节通常是迟到的 Start/Stop ACK 或前导残杂字节（SKILL.md §3.3.4）。
			// 缓冲不足 size+1 时先等待更多数据，否则无法判断丢弃后能否对齐。
			if len(r.buffer) < size+1 {
				return nil, false, false, nil
			}
			if dframe, dok, derr := r.extractFixedFrameWithDropLocked(size); derr == nil && dok {
				return dframe, false, false, nil
			}
		}
		return frameIf(ok, frame), false, false, err
	}
	if !r.ackAfterData[0] {
		if len(r.buffer) == 0 {
			return nil, false, false, nil
		}
		if r.buffer[0] == 'A' {
			r.consumeControlACKLocked()
			return nil, true, false, nil
		}
		// Start ACK 偶发缺失或迟到（SKILL.md §3.3.4 实机探针）：
		// 首字节非 'A' 时不能判错，而要用偏移0/偏移1的帧合法性确定真实边界。
		//   - 偏移0合法：数据优先、无前导残杂字节，ACK 视为未发送；
		//   - 偏移1合法：存在1个前导残杂字节（上一事务尾字节），丢弃后对齐；
		//   - 两者都非法：才是真正的协议错位，要求重连。
		// 禁止裸搜 0x41：约 25.9°C 的 float32 LE 高字节本身就是 0x41。
		if len(r.buffer) < size+1 {
			return nil, false, false, nil
		}
		if _, err := ParseTCPFrameEx(r.buffer[:size]); err == nil {
			r.resolveMissingLeadingACKLocked()
			frame, ok, err := r.extractFixedFrameLocked(size)
			return frameIf(ok, frame), false, false, err
		}
		if _, err := ParseTCPFrameEx(r.buffer[1 : size+1]); err == nil {
			r.buffer = r.buffer[1:]
			if len(r.buffer) == 0 {
				r.buffer = make([]byte, 0, 256)
			}
			r.resolveMissingLeadingACKLocked()
			frame, ok, err := r.extractFixedFrameLocked(size)
			return frameIf(ok, frame), false, false, err
		}
		return nil, false, false, fmt.Errorf(
			"expected Start ACK 0x41 or aligned first frame at stream boundary, got 0x%02X (neither offset valid)",
			r.buffer[0])
	}
	if !r.stopReady {
		return nil, false, true, nil
	}
	if len(r.buffer) > 1 {
		frame, ok, err := r.extractFixedFrameLocked(size)
		return frameIf(ok, frame), false, false, err
	}
	if len(r.buffer) == 1 && r.buffer[0] == 'A' {
		r.consumeControlACKLocked()
		r.stopReady = false
		return nil, true, false, nil
	}
	return nil, false, false, fmt.Errorf("validated Stop response lost terminal ACK")
}

func frameIf(ok bool, frame []byte) []byte {
	if ok {
		return frame
	}
	return nil
}

func isTimeoutError(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}

func (r *T1603FrameReader) finalizeStopResponseLocked(frameSize int) (bool, error) {
	if len(r.buffer) == 0 {
		return false, nil
	}
	// 迟到的 Start ACK 'A' 可能落在 Stop 收集窗口开头（实机快速点击复现：
	// raw=41 41，即 [延迟 Start ACK][Stop ACK]）。原缓冲不满足 N×64+ACK 时，
	// 尝试丢弃 1 个前导 'A' 再验证。仅当原缓冲非法时才丢，避免误删合法数据帧
	// 首字节（合法帧首字节也可能为 0x41）。
	buf := r.buffer
	valid := len(buf)%frameSize == 1 && buf[len(buf)-1] == 'A'
	if !valid && len(buf) > 1 && buf[0] == 'A' &&
		(len(buf)-1)%frameSize == 1 && buf[len(buf)-1] == 'A' {
		buf = buf[1:]
		valid = true
	}
	if !valid {
		return false, fmt.Errorf("invalid Stop response boundary: bytes=%d expected N*%d+ACK; raw=% X",
			len(r.buffer), frameSize, r.buffer)
	}
	if len(buf) != len(r.buffer) {
		// 已丢弃前导残杂 'A'，同步从 buffer 移除。
		r.buffer = r.buffer[1:]
	}
	r.stopReady = true
	return true, nil
}

func (r *T1603FrameReader) resolveMissingLeadingACKLocked() {
	if len(r.ackAfterData) > 0 && !r.ackAfterData[0] {
		r.ackAfterData = r.ackAfterData[1:]
	}
}

func (r *T1603FrameReader) consumeControlACKLocked() {
	r.buffer = r.buffer[1:]
	for len(r.buffer) > 0 && (r.buffer[0] == '\r' || r.buffer[0] == '\n') {
		r.buffer = r.buffer[1:]
	}
	r.ackAfterData = r.ackAfterData[1:]
}

func (r *T1603FrameReader) extractFixedFrameLocked(size int) ([]byte, bool, error) {
	if len(r.buffer) < size {
		return nil, false, nil
	}
	frame := make([]byte, size)
	copy(frame, r.buffer[:size])
	if _, err := ParseTCPFrameEx(frame); err != nil {
		return nil, false, fmt.Errorf("invalid frame at established %d-byte boundary: %w", size, err)
	}
	r.buffer = r.buffer[size:]
	if len(r.buffer) == 0 {
		r.buffer = make([]byte, 0, 256)
	}
	return frame, true, nil
}

// extractFixedFrameWithDropLocked 丢弃缓冲区首字节后提取一帧，用于正常采集路径
// 的单字节自愈（迟到的 ACK / 前导残杂字节导致的 1 字节偏移）。仅允许丢弃 1 字节。
func (r *T1603FrameReader) extractFixedFrameWithDropLocked(size int) ([]byte, bool, error) {
	if len(r.buffer) < size+1 {
		return nil, false, nil
	}
	frame := make([]byte, size)
	copy(frame, r.buffer[1:size+1])
	if _, err := ParseTCPFrameEx(frame); err != nil {
		return nil, false, fmt.Errorf("invalid frame at established %d-byte boundary after 1-byte drop: %w", size, err)
	}
	r.buffer = r.buffer[size+1:]
	if len(r.buffer) == 0 {
		r.buffer = make([]byte, 0, 256)
	}
	return frame, true, nil
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
	if len(r.ackAfterData) > 0 && len(r.buffer) > 0 && r.buffer[0] == 'A' && len(r.buffer) < 17 {
		r.buffer = r.buffer[1:]
		r.ackAfterData = r.ackAfterData[1:]
		r.mu.Unlock()
		return nil, ErrControlACK
	}
	if frame, end := tryExtractFrame(r.buffer); end >= 0 {
		r.buffer = r.buffer[end:]
		r.resolveMissingLeadingACKLocked()
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
		if len(r.ackAfterData) > 0 && len(r.buffer) > 0 && r.buffer[0] == 'A' && tryVariableACK(r.buffer) {
			r.buffer = r.buffer[1:]
			r.ackAfterData = r.ackAfterData[1:]
			r.mu.Unlock()
			return nil, ErrControlACK
		}
		if frame, end := tryExtractFrame(r.buffer); end >= 0 {
			r.buffer = r.buffer[end:]
			r.resolveMissingLeadingACKLocked()
			if len(r.buffer) == 0 {
				r.buffer = make([]byte, 0, 256)
			}
			r.mu.Unlock()
			return frame, nil
		}
		r.mu.Unlock()
	}
}

func tryVariableACK(buffer []byte) bool {
	if len(buffer) == 1 {
		return true
	}
	_, end := tryExtractFrame(buffer)
	if end >= 0 {
		return false
	}
	_, end = tryExtractFrame(buffer[1:])
	return end >= 0
}

func (r *T1603FrameReader) frameSizeLocked() int {
	if r.binaryMode {
		if r.sequenceMode && r.metadataMode {
			return TCPFrameSizeWithSequenceAndTimestamp // 76: seq + ts + 64 float32
		}
		if r.sequenceMode {
			return TCPFrameSizeWithSequence // 68: seq + 64 float32
		}
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
	r.ackAfterData = nil
	r.stopReady = false
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

// ConsumeOptionalACK drains a leading ACK if present for diagnostic tools.
// Production uses the single-reader control ACK state above; this helper keeps
// non-ACK bytes in the frame buffer for legacy probes.
//
// ADR-009 R0-3 整改：移除 watchdog Close 兜底。
// 历史背景：原实现通过 WatchdogClose 在 timeout 后强制 Close conn 解除阻塞的
// Read，违反 ADR-009 决策 8——可选 ACK 是"无数据也正常"的操作，watchdog 到期
// 只能证明探测无法完成，不能证明物理连接故障。问题 Windows 电脑 deadline 失效
// 时健康连接会被误杀。
//
// 整改后：仅依赖 SetReadDeadline 软超时。timeout 到期返回 (false, nil) 表示
// "无 ACK"，是正常结果。调用方需自行决定是否在 deadline 失效场景下用外层
// watchdog 兜底（如诊断工具的进程级 watchdog）。
//
// 注意：本函数仅在诊断工具（freqprobe/frameprobe）中使用，生产路径不再调用。
func (r *T1603FrameReader) ConsumeOptionalACK(timeout time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var resultErr error
	consumed := false

	one := make([]byte, 2)
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
			// 无论结果如何，清 deadline 避免残留 timeout 影响后续 ReadFrame
			_ = r.conn.SetReadDeadline(time.Time{})
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 无 ACK 可读，是正常结果，不是错误
				} else {
					resultErr = err
				}
			} else if n > 0 {
				first = one[0]
				haveFirst = true
				if n > 1 {
					r.buffer = append(r.buffer, one[1:n]...)
				}
			}
		}
	}

	if resultErr == nil && haveFirst {
		if first != 'A' {
			// 非 ACK 字节回退到 buffer 前部，保留给后续 ReadFrame
			r.buffer = append([]byte{first}, r.buffer...)
		} else {
			consumed = true
			for len(r.buffer) > 0 && (r.buffer[0] == '\r' || r.buffer[0] == '\n') {
				r.buffer = r.buffer[1:]
			}
		}
	}

	if resultErr == nil {
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
//
// ADR-009 R0-12：soft deadline 触发时（net.Error.Timeout()），即使有部分响应也必须
// 毒化连接——迟到响应可能随后进入 TCP 流被下一条命令消费，导致协议错位。
// 整改后：soft timeout 时强制 Close conn 并返回 ErrWatchdogTriggered，让调用方
// 统一毒化驱动状态。
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
	// softTimeoutTriggered 标记 soft deadline 触发，需强制毒化连接（R0-12）。
	var softTimeoutTriggered bool

	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		// Write 阶段任何错误（timeout、broken pipe、RST 等）都意味着协议边界不可信。
		// R0-12 + finding 3：强制 Close conn 阻断迟到响应，标记毒化让调用方废弃连接。
		softTimeoutTriggered = true
		_ = conn.Close()
		resultErr = fmt.Errorf("send %q: %w", cmd, err)
	} else {
		conn.SetReadDeadline(time.Now().Add(cmdTimeout))
		one := make([]byte, 1)
		for {
			n, err := conn.Read(one)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// R0-12：soft deadline 触发，协议边界已不可信。
					// 即使有部分响应也必须毒化连接：迟到响应可能随后进入 TCP 流，
					// 被下一条命令消费导致协议错位。强制 Close conn 阻断迟到响应。
					softTimeoutTriggered = true
					_ = conn.Close()
					if buf.Len() > 0 {
						partialResponse = strings.TrimRight(buf.String(), "\r ")
					}
					resultErr = err
					break
				}
				// ADR-009 finding 3：非 timeout 错误（EOF、RST、io.ErrUnexpectedEOF 等）
				// 同样意味着协议边界不可信。设备可能在发送部分数据后断连，迟到响应
				// 可能随后进入 TCP 流被下一条命令消费导致协议错位。强制 Close conn
				// 并标记毒化，让调用方通过 IsWatchdogTriggered 触发 invalidateConnection。
				softTimeoutTriggered = true
				_ = conn.Close()
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
	// R0-12: softTimeoutTriggered 也视为 conn 已死，统一返回 ErrWatchdogTriggered。
	// 丢弃 partialResponse：协议边界不可信时部分响应不可用。
	if softTimeoutTriggered || !checkWd() {
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
//
// ADR-009 R0-12：首字节 cmdTimeout 触发视为 soft timeout——协议边界不可信，
// 迟到响应可能随后进入 TCP 流被下一条命令消费。整改后：首字节 timeout 时
// 强制 Close conn 并返回 ErrWatchdogTriggered。
// 注意：收到至少一个字节后的 idleWindow timeout 是正常结束语义（静默窗口
// 检测到响应结束），不视为 soft timeout，返回成功。
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
	// softTimeoutTriggered 标记首字节 soft deadline 触发，需强制毒化连接（R0-12）。
	var softTimeoutTriggered bool

	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		// Write 阶段任何错误（timeout、broken pipe、RST 等）都意味着协议边界不可信。
		// R0-12 + finding 3：强制 Close conn 阻断迟到响应，标记毒化让调用方废弃连接。
		softTimeoutTriggered = true
		_ = conn.Close()
		resultErr = fmt.Errorf("send %q: %w", cmd, err)
	} else {
		conn.SetReadDeadline(time.Now().Add(cmdTimeout))
		one := make([]byte, 1)
		for {
			n, err := conn.Read(one)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					if buf.Len() > 0 {
						// 已收到至少一个字节后的 idleWindow timeout：
						// 正常结束语义（静默窗口检测），返回部分响应。
						partialResponse = strings.TrimRight(buf.String(), "\r\n ")
						break
					}
					// 首字节 cmdTimeout 触发：soft timeout，毒化连接（R0-12）。
					softTimeoutTriggered = true
					_ = conn.Close()
					resultErr = err
					break
				}
				// ADR-009 finding 3：非 timeout 错误（EOF、RST、io.ErrUnexpectedEOF 等）
				// 同样意味着协议边界不可信。设备可能在发送部分数据后断连，迟到响应
				// 可能随后进入 TCP 流被下一条命令消费导致协议错位。强制 Close conn
				// 并标记毒化，让调用方通过 IsWatchdogTriggered 触发 invalidateConnection。
				softTimeoutTriggered = true
				_ = conn.Close()
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

	// R0-12: softTimeoutTriggered 也视为 conn 已死，统一返回 ErrWatchdogTriggered。
	if softTimeoutTriggered || !checkWd() {
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
// DAQ-T-1603 fixed-length responses have no trailing delimiter, so the function
// returns immediately after io.ReadFull completes.
//
// ADR-009 watchdog：io.ReadFull 内部循环 conn.Read 不重设 deadline，
// 必须由外层 watchdog 兜底；watchdog 触发时 conn 被 Close，io.ReadFull
// 返回 io.ErrUnexpectedEOF 或 net.ErrClosed，统一包装为 "watchdog triggered"。
//
// ADR-009 R0-12：io.ReadFull 中途 soft deadline 触发时返回 net.Error timeout
// 或 io.ErrUnexpectedEOF（部分字节后 EOF）。两种情况都意味着协议边界不可信：
// 设备可能继续发送剩余字节，被下一条命令消费导致协议错位。整改后：检测到
// soft timeout 时强制 Close conn 并返回 ErrWatchdogTriggered。
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
	// softTimeoutTriggered 标记 soft deadline 触发，需强制毒化连接（R0-12）。
	var softTimeoutTriggered bool

	conn.SetWriteDeadline(time.Now().Add(cmdTimeout))
	if _, err := conn.Write([]byte(cmd)); err != nil {
		// Write 阶段任何错误（timeout、broken pipe、RST 等）都意味着协议边界不可信。
		// R0-12 + finding 3：强制 Close conn 阻断迟到响应，标记毒化让调用方废弃连接。
		softTimeoutTriggered = true
		_ = conn.Close()
		resultErr = fmt.Errorf("send %q: %w", cmd, err)
	} else {
		conn.SetReadDeadline(time.Now().Add(cmdTimeout))
		buf := make([]byte, n)
		if _, err := io.ReadFull(conn, buf); err != nil {
			// ADR-009 finding 3：io.ReadFull 中途任何错误（timeout、EOF、
			// io.ErrUnexpectedEOF 短读等）都意味着协议边界不可信。设备可能在发送
			// 部分数据后断连，迟到响应可能随后进入 TCP 流被下一条命令消费。
			// 强制 Close conn 并标记毒化，让调用方通过 IsWatchdogTriggered
			// 触发 invalidateConnection。
			softTimeoutTriggered = true
			_ = conn.Close()
			resultErr = fmt.Errorf("read exact %d for %q: %w", n, cmd, err)
		} else {
			response = string(bytes.TrimRight(buf, "\r\n "))
		}
	}

	// R0-12: softTimeoutTriggered 也视为 conn 已死，统一返回 ErrWatchdogTriggered。
	if softTimeoutTriggered || !checkWd() {
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
