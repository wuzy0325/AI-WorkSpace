// Package sim 提供设备协议模拟器框架（TC-HW-SIM-01）。
//
// 设计目标：在本地 127.0.0.1 随机端口启动一个 TCP 服务端，按设备协议
// 生成/注入帧，让真实 adapter（如 DAQP1604、DSA3217）连接它，从而端到端
// 测试 adapter 的协议解析、断连重连、超时、错误帧与多设备并发能力。
//
// 为什么模拟器要分离 FrameProducer 与 CommandResponder：
//   - FrameProducer 负责按设备协议生成线上字节帧（含长度前缀、校验和），
//     不同设备帧格式差异隔离在 producer 里，simulator 核心与协议解耦。
//   - CommandResponder 负责处理 SCPI/文本命令并返回响应，只对命令式设备
//     （DSA3217/T1603/P1604）有意义；流式设备（P1064Pre/WTN_PXI）可不设。
//   - 这样 Simulator 核心只关心 TCP 连接管理 + 帧注入 + 故障注入，可被
//     任意设备类型复用，新增设备只需提供 producer + responder。
//
// 为什么用随机端口（127.0.0.1:0）：多设备并发测试时多个 Simulator 实例
// 同时运行，随机端口避免端口冲突，且不占用固定端口资源。
//
// 本包不是 ports.Device：它是测试工具，给真实 adapter 连接的 TCP 服务端。
// 装配方式见 wiring.go，通过测试 helper 把 profile.Address 指向模拟器地址。
package sim

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// FrameProducer 生成一帧完整的线上字节（含长度前缀、校验和等设备协议封装）。
// seq 是帧序号，可嵌入数据用于排序验证；channels 是通道数。
// 返回的字节可直接写入 TCP 连接供 adapter 的帧解析器读取。
type FrameProducer func(seq int, channels int) ([]byte, error)

// ReadMode 决定 CommandResponder 读取命令的方式。
type ReadMode int

const (
	// ReadModeLine 行模式：读到 '\n' 认为命令完整。适用于命令以 \r\n 结尾的设备
	// （DSA3217、P1604 adapter 的 sendCommand 写 cmd+"\r\n"）。
	ReadModeLine ReadMode = iota
	// ReadModeIdle 空闲模式：读到数据后短暂空闲（无新字节）即认为命令完整。
	// 适用于命令不带分隔符的设备（T1603 adapter 的 writeCommandOnly 写裸命令）。
	ReadModeIdle
)

// Response 是 CommandResponder 处理命令后的响应指令。
// Data 为要写回客户端的字节（nil 表示不回复）。
// StartStream/StopStream 控制模拟器是否开始/停止持续发送数据帧。
type Response struct {
	Data        []byte
	StartStream bool
	StopStream  bool
}

// CommandResponder 处理客户端发来的一行命令，返回响应与流控动作。
// 实现者通过 ReadMode 声明命令读取方式。
type CommandResponder interface {
	ReadMode() ReadMode
	HandleCommand(line []byte) (Response, error)
}

// Simulator 是设备协议模拟器框架的核心契约。
type Simulator interface {
	// Start 启动 TCP 监听，返回后 Addr() 可用。
	Start(ctx context.Context) error
	// Addr 返回监听地址（127.0.0.1:随机端口）。
	Addr() string
	// Close 关闭监听与所有客户端连接，释放资源。
	Close() error
	// InjectFrame 向当前连接注入一帧完整线上字节。
	InjectFrame(frame []byte) error
	// DropNext 丢弃接下来 n 次帧发送（模拟丢帧）。
	DropNext(n int)
	// SetLatency 设置每帧发送间隔延迟。
	SetLatency(d time.Duration)
	// SetFailOnConnect 设置是否拒绝下一次客户端连接（模拟设备故障）。
	SetFailOnConnect(b bool)
	// DisconnectClient 主动断开当前客户端连接（模拟掉线）。
	DisconnectClient()
}

// TCPSimulator 是 Simulator 的标准实现：本地 TCP 服务端 + 故障注入。
type TCPSimulator struct {
	producer   FrameProducer
	responder  CommandResponder
	autoStart  bool // 连接建立后是否自动开始发帧（流式设备）
	channels   int
	defaultLat time.Duration

	listener net.Listener
	addr     string

	mu            sync.Mutex
	currentConn   net.Conn
	acquiring     bool // 是否正在持续发送数据帧
	latency       time.Duration
	dropNext      int
	failOnConnect bool

	// writeMu 串行化对 currentConn 的写操作，避免 sendLoop 与 cmdLoop
	// 交叉写入导致 TCP 帧边界错乱。
	writeMu sync.Mutex

	closed     atomic.Bool
	stopAccept chan struct{}
	acceptDone chan struct{}
}

// NewTCPSimulator 构造一个模拟器实例。
//   - producer: 帧生成器（必填）
//   - responder: 命令响应器（可为 nil，表示流式设备连接后自动发帧）
//   - autoStart: 连接建立后是否立即开始发帧（P1064Pre/WTN_PXI=true）
//   - channels: 通道数，传给 producer
func NewTCPSimulator(producer FrameProducer, responder CommandResponder, autoStart bool, channels int) *TCPSimulator {
	return &TCPSimulator{
		producer:     producer,
		responder:    responder,
		autoStart:    autoStart,
		channels:     channels,
		defaultLat:   10 * time.Millisecond,
		latency:      10 * time.Millisecond,
	}
}

// Start 启动 TCP 监听。使用 127.0.0.1:0 让操作系统分配随机端口，
// 支持多设备并发测试（多个 Simulator 实例同时运行于不同端口）。
func (s *TCPSimulator) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("sim listen: %w", err)
	}
	s.listener = ln
	s.addr = ln.Addr().String()
	s.stopAccept = make(chan struct{})
	s.acceptDone = make(chan struct{})
	go s.acceptLoop()
	return nil
}

// Addr 返回监听地址。
func (s *TCPSimulator) Addr() string {
	return s.addr
}

// Close 关闭监听与当前客户端连接，等待 acceptLoop 退出。
// 幂等：重复调用安全。
func (s *TCPSimulator) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.stopAccept != nil {
		close(s.stopAccept)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.disconnectClientLocked()
	if s.acceptDone != nil {
		<-s.acceptDone
	}
	return nil
}

// InjectFrame 向当前连接注入一帧完整线上字节（绕过 producer 与延迟）。
// 用于测试 adapter 对特定帧的解析能力。
func (s *TCPSimulator) InjectFrame(frame []byte) error {
	s.mu.Lock()
	conn := s.currentConn
	s.mu.Unlock()
	if conn == nil {
		return ErrNoClient
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := conn.Write(frame)
	return err
}

// DropNext 让接下来 n 次帧发送被丢弃（producer 仍被调用，但帧不写入连接）。
// 用于测试 adapter 的丢帧/重同步能力。
func (s *TCPSimulator) DropNext(n int) {
	s.mu.Lock()
	if n < 0 {
		n = 0
	}
	s.dropNext += n
	s.mu.Unlock()
}

// SetLatency 设置每帧发送间隔。0 表示用默认 10ms。
func (s *TCPSimulator) SetLatency(d time.Duration) {
	s.mu.Lock()
	if d <= 0 {
		d = s.defaultLat
	}
	s.latency = d
	s.mu.Unlock()
}

// SetFailOnConnect 设置是否拒绝下一次客户端连接。
// true 时 Accept 到的连接会被立即关闭，模拟设备故障/拒绝服务。
func (s *TCPSimulator) SetFailOnConnect(b bool) {
	s.mu.Lock()
	s.failOnConnect = b
	s.mu.Unlock()
}

// DisconnectClient 主动断开当前客户端连接，模拟设备掉线。
// 会触发 adapter 的 readLoop 读取错误并调用 ErrorNotifiable 回调。
func (s *TCPSimulator) DisconnectClient() {
	s.disconnectClientLocked()
}

// WaitClient 等待模拟器接受客户端连接（Accept 完成）。
// 用于测试同步：Dial 后调用以确保 InjectFrame 不会因 currentConn 尚未设置而失败。
func (s *TCPSimulator) WaitClient(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		conn := s.currentConn
		s.mu.Unlock()
		if conn != nil {
			return nil
		}
		time.Sleep(2 * time.Millisecond)
	}
	return ErrNoClient
}

// disconnectClientLocked 断开当前连接并重置 acquiring 状态。
// 调用方自行决定是否持锁。
func (s *TCPSimulator) disconnectClientLocked() {
	s.mu.Lock()
	conn := s.currentConn
	s.currentConn = nil
	s.acquiring = false
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// acceptLoop 接受客户端连接。每次只保留一个客户端：新连接到来时关闭旧连接。
func (s *TCPSimulator) acceptLoop() {
	defer close(s.acceptDone)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopAccept:
				return
			default:
				slog.Debug("sim accept error", "error", err)
				return
			}
		}
		s.handleNewConnection(conn)
	}
}

// handleNewConnection 处理新接受的客户端连接。
// 关闭旧连接（只允许一个客户端），根据配置启动 sendLoop 与 cmdLoop。
func (s *TCPSimulator) handleNewConnection(conn net.Conn) {
	s.mu.Lock()
	if s.failOnConnect {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	old := s.currentConn
	s.currentConn = conn
	startStream := s.autoStart
	if old != nil {
		// 新连接到来，旧连接让位。先清空 currentConn 再关闭，
		// 避免旧 sendLoop 检测 currentConn==old 时误判。
		s.mu.Unlock()
		_ = old.Close()
		s.mu.Lock()
		s.currentConn = conn
	}
	if startStream {
		s.acquiring = true
	}
	s.mu.Unlock()

	// done 在读 goroutine（cmdLoop 或 readUntilClosed）退出时关闭，
	// 用于通知 sendLoop 停止。
	done := make(chan struct{})
	go s.sendLoop(conn, done)
	if s.responder != nil {
		go s.cmdLoop(conn, done)
	} else {
		// 无 responder 的流式设备：起一个读 goroutine 仅用于检测连接关闭，
		// adapter 不发命令，读到数据丢弃即可。
		go s.readUntilClosed(conn, done)
	}
}

// sendLoop 按 latency 间隔生成帧并写入连接。
// acquiring=false 时不发帧（等待命令式设备触发 StartStream）。
// DropNext 控制丢弃接下来 n 帧。
func (s *TCPSimulator) sendLoop(conn net.Conn, done <-chan struct{}) {
	seq := 0
	for {
		select {
		case <-done:
			return
		default:
		}

		s.mu.Lock()
		acquiring := s.acquiring
		latency := s.latency
		dropNext := s.dropNext
		if dropNext > 0 {
			s.dropNext--
		}
		s.mu.Unlock()

		if !acquiring {
			// 未采集：短暂等待，避免空转占用 CPU
			timer := time.NewTimer(5 * time.Millisecond)
			select {
			case <-done:
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		if dropNext > 0 {
			// 丢弃这一帧（seq 仍递增，保持时序）
			seq++
			s.sleep(done, latency)
			continue
		}

		frame, err := s.producer(seq, s.channels)
		if err != nil {
			slog.Debug("sim producer error", "seq", seq, "error", err)
			seq++
			continue
		}

		s.writeMu.Lock()
		// 连接已被替换或关闭，退出
		if !s.isCurrentConnLocked(conn) {
			s.writeMu.Unlock()
			return
		}
		_, err = conn.Write(frame)
		s.writeMu.Unlock()

		if err != nil {
			return
		}
		seq++
		s.sleep(done, latency)
	}
}

// sleep 在 done 或 timer 先触发时返回，保证 sendLoop 能及时响应连接关闭。
func (s *TCPSimulator) sleep(done <-chan struct{}, d time.Duration) {
	if d <= 0 {
		d = s.defaultLat
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

// cmdLoop 读取客户端命令并交给 responder 处理。
// 根据 responder.ReadMode() 选择行模式或空闲模式读取。
func (s *TCPSimulator) cmdLoop(conn net.Conn, done chan<- struct{}) {
	defer close(done)
	mode := s.responder.ReadMode()
	if mode == ReadModeLine {
		s.cmdLoopLine(conn)
	} else {
		s.cmdLoopIdle(conn)
	}
}

// cmdLoopLine 行模式：用 bufio 读到 '\n' 为一条命令。
func (s *TCPSimulator) cmdLoopLine(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		// 行模式命令通常即时响应，用较长 deadline 等待命令到达
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		if !s.dispatchCommand(conn, line) {
			return
		}
	}
}

// cmdLoopIdle 空闲模式：读到数据后短暂无新字节即认为命令完整。
// 用于 T1603 这类命令不带分隔符的设备。
func (s *TCPSimulator) cmdLoopIdle(conn net.Conn) {
	var buf []byte
	tmp := make([]byte, 64)
	const idleWindow = 10 * time.Millisecond

	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(tmp)
		if err != nil {
			// 超时前若有残余命令，先派发
			if len(buf) > 0 {
				s.dispatchCommand(conn, buf)
				buf = buf[:0]
			}
			return
		}
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			// 持续读直到空闲：短超时再读，无新数据即认为命令完整
			for {
				_ = conn.SetReadDeadline(time.Now().Add(idleWindow))
				n2, err2 := conn.Read(tmp)
				if n2 > 0 {
					buf = append(buf, tmp[:n2]...)
				}
				if err2 != nil {
					break
				}
			}
			if len(buf) > 0 {
				if !s.dispatchCommand(conn, buf) {
					return
				}
				buf = buf[:0]
			}
		}
	}
}

// dispatchCommand 调用 responder 处理一条命令，并执行响应与流控动作。
// 返回 false 表示当前连接已不是 currentConn，调用方应退出 goroutine。
func (s *TCPSimulator) dispatchCommand(conn net.Conn, line []byte) bool {
	resp, err := s.responder.HandleCommand(line)
	if err != nil {
		slog.Debug("sim responder error", "cmd", string(line), "error", err)
	}

	// 先写响应，再更新流控状态：保证客户端先收到响应再收到数据帧
	if len(resp.Data) > 0 {
		s.writeMu.Lock()
		if !s.isCurrentConnLocked(conn) {
			s.writeMu.Unlock()
			return false
		}
		_, _ = conn.Write(resp.Data)
		s.writeMu.Unlock()
	}

	s.mu.Lock()
	if resp.StartStream {
		s.acquiring = true
	}
	if resp.StopStream {
		s.acquiring = false
	}
	s.mu.Unlock()
	return true
}

// readUntilClosed 用于无 responder 的流式设备：仅检测连接关闭以通知 sendLoop。
func (s *TCPSimulator) readUntilClosed(conn net.Conn, done chan<- struct{}) {
	defer close(done)
	buf := make([]byte, 1024)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err := conn.Read(buf)
		if err != nil {
			return
		}
	}
}

// isCurrentConnLocked 在已持有 writeMu 的情况下判断 conn 是否仍为当前连接。
func (s *TCPSimulator) isCurrentConnLocked(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentConn == conn
}

// ErrNoClient 表示当前没有客户端连接，无法注入帧。
var ErrNoClient = errors.New("sim: no client connected")
