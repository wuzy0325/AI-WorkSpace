// simulator.go 实现设备协议模拟器框架核心：TCP 服务端、多客户端管理、
// 命令分发、故障注入与帧广播。协议特定逻辑由注入的 ProtocolHandler 提供。

package sim

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ProtocolHandler 定义设备特定协议行为：如何响应客户端命令、如何生成采集数据帧。
// 每种设备（P1604/T1603/DSA3217 等）实现此接口注入 Simulator，把协议知识与
// 连接管理解耦。
type ProtocolHandler interface {
	// HandleCommand 处理客户端发来的命令字节，返回要回写的响应字节。
	// 返回 nil 表示不回写（如 P1604 采集启停命令，adapter 不读响应）。
	// 返回的字节应是完整线上响应（含长度前缀/校验和等协议封装），
	// Simulator 直接写入连接，不做任何协议加工——Simulator 是哑管道。
	HandleCommand(cmd []byte) []byte

	// StartAcquisition 由 Simulator 在 Start() 时调用一次，注入 emit 回调。
	// 协议处理器据此"武装"采集能力，但实际何时开始 emit 由处理器根据设备
	// 协议命令自行决定（如 P1604 收到 "c 01 1" 后才开始发数据帧）。
	// 这样协议特定的启停语义留在 handler 内，Simulator 不需要知道命令含义。
	StartAcquisition(emit func(frame []byte))

	// StopAcquisition 由 Simulator 在 Close() 时调用，要求处理器停止生成
	// 数据帧并清理 emit goroutine，保证资源释放。
	StopAcquisition()
}

// CommandReader 从连接读取一条完整命令。
// 不同协议的命令分帧方式不同（SCPI 文本按行，二进制按长度前缀/魔数头），
// 故抽象为接口。默认用 LineCommandReader（按行读取），覆盖所有 SCPI 设备；
// 需要二进制命令读取的设备可实现 CommandReaderProvider 提供自定义实现。
type CommandReader interface {
	// ReadCommand 从 r 读取一条完整命令，返回的 cmd 不含分界符。
	// io.EOF 表示连接关闭，其他非 EOF 错误会终止该客户端的处理。
	ReadCommand(r *bufio.Reader) ([]byte, error)
}

// LineCommandReader 按行读取命令（SCPI 文本协议通用）。
// 读到 '\n' 认为命令完整，去掉尾部 '\r'。适用于 P1604/T1603/DSA3217
// 等命令以 "\r\n" 或 "\n" 结尾的设备。
type LineCommandReader struct{}

// ReadCommand 读到 '\n' 后返回去掉尾部 "\r\n" 的命令字节。
// 连接半关闭时若仍有残余命令（EOF 但缓冲非空），也返回该残余命令。
func (LineCommandReader) ReadCommand(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		// 容忍半关闭：EOF 但已读到部分数据时，仍当作最后一条命令派发
		if err == io.EOF && len(line) > 0 {
			return bytes.TrimRight(line, "\r\n"), nil
		}
		return nil, err
	}
	return bytes.TrimRight(line, "\r\n"), nil
}

// CommandReaderProvider 是 ProtocolHandler 的可选扩展接口。
// 实现 ProtocolHandler 的同时实现此接口，可提供自定义命令读取逻辑
// （如 WTN_PXI 的 4 字节长度前缀命令、DAQ-P-1064Pre 的 0xA5 0x5A 头命令）。
// 未实现时 Simulator 使用默认的 LineCommandReader。
type CommandReaderProvider interface {
	CommandReader() CommandReader
}

// Simulator 是设备协议模拟器的统一框架。
// 它在本地起 TCP 服务端，让真实 adapter 通过 net.DialTimeout 连接，
// 从而端到端测试协议帧解析、断连重连、超时、错误帧、多设备并发。
type Simulator struct {
	addr      string // 期望监听地址（端口 0 时由系统分配）
	handler   ProtocolHandler
	cmdReader CommandReader // 命令读取器（默认行模式，可由 handler 覆盖）

	listener net.Listener
	actual   string // 实际监听地址（端口 0 分配后用此获取真实地址）

	// clients 跟踪已连接客户端：key=net.Conn，value=*client。
	// 用 sync.Map 支持并发故障注入（InjectFrame/DisconnectAll）与 readLoop 退出清理。
	clients     sync.Map
	clientCount atomic.Int32 // O(1) 准确计数，避免 Range 计数的竞态

	// 故障注入状态（均可在测试中随时并发调用）
	latency   atomic.Int64 // 响应延迟（纳秒），SetLatency 设置
	dropNextN atomic.Int32 // 接下来要丢弃响应的命令数，DropNext 累加
	closed    atomic.Bool  // Simulator 是否已 Close（幂等保护）

	wg sync.WaitGroup // 跟踪 acceptLoop + 所有 client read/writeLoop，Close 时 Wait
}

// NewSimulator 创建模拟器但不开始监听（用 Start 启动）。
// addr 传 "127.0.0.1:0" 让系统分配空闲端口，避免端口冲突；
// 传 "" 等价于 "127.0.0.1:0"。
// handler 必须非 nil，否则 Start 返回错误。
func NewSimulator(addr string, handler ProtocolHandler) *Simulator {
	s := &Simulator{
		addr:      addr,
		handler:   handler,
		cmdReader: LineCommandReader{}, // 默认行模式，覆盖所有 SCPI 设备
	}
	// 若 handler 提供自定义命令读取器，则采用（支持二进制命令设备）
	if crp, ok := handler.(CommandReaderProvider); ok {
		if cr := crp.CommandReader(); cr != nil {
			s.cmdReader = cr
		}
	}
	return s
}

// Start 开始监听并接受连接（每个连接独立 goroutine 处理）。
// 同时调用 handler.StartAcquisition 注入 emit 回调，武装采集能力。
// 重复调用返回错误。
func (s *Simulator) Start() error {
	if s.handler == nil {
		return errors.New("sim: ProtocolHandler is nil")
	}
	if s.listener != nil {
		return errors.New("sim: already started")
	}
	addr := s.addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("sim listen %s: %w", addr, err)
	}
	s.listener = ln
	s.actual = ln.Addr().String()
	// 注入 emit 回调：handler 据此能向所有已连接客户端推送帧。
	// 不在此启动发帧——发帧时机由 handler 按设备协议命令决定。
	s.handler.StartAcquisition(s.broadcast)
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// Addr 返回实际监听地址（端口 0 分配后用此获取真实地址）。
// Start 前调用返回传入的原始地址。
func (s *Simulator) Addr() string {
	if s.actual != "" {
		return s.actual
	}
	return s.addr
}

// Close 停止监听、停止 handler 采集、断开所有客户端，并等待所有 goroutine 退出。
// 幂等：重复调用安全返回 nil。
func (s *Simulator) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.listener != nil {
		_ = s.listener.Close() // 触发 acceptLoop 退出
	}
	// 先停止 handler 的 emit goroutine，避免 Close 期间仍向已关闭客户端写
	s.handler.StopAcquisition()
	// 断开所有客户端：触发各 readLoop 读取错误并退出，writeLoop 经 done 退出
	s.clients.Range(func(_, v any) bool {
		v.(*client).close()
		return true
	})
	s.wg.Wait()
	return nil
}

// === 故障注入 API ===

// SetLatency 设置命令响应延迟（模拟网络延迟/超时）。
// readLoop 在调用 HandleCommand 前睡眠该时长。d<=0 表示无延迟。
func (s *Simulator) SetLatency(d time.Duration) {
	s.latency.Store(int64(d))
}

// DropNext 让接下来 n 个客户端命令不响应（模拟丢包/超时）。
// readLoop 读到命令后若仍有 drop 配额，则直接丢弃、不调用 HandleCommand、
// 不回写。n 可累加（多次调用叠加丢弃次数）。
func (s *Simulator) DropNext(n int) {
	if n <= 0 {
		return
	}
	s.dropNextN.Add(int32(n))
}

// InjectFrame 向所有已连接客户端推送一个自定义帧（模拟错误帧/脏帧）。
// frame 是完整线上字节（含长度前缀/校验和等），Simulator 原样写入。
// 无客户端时为 no-op（帧被丢弃）。
func (s *Simulator) InjectFrame(frame []byte) {
	s.broadcast(frame)
}

// DisconnectAll 关闭所有客户端连接但保持监听（模拟设备掉线后可重连）。
// 会触发真实 adapter 的 readLoop 读取错误并调用 ErrorNotifiable 回调，
// 从而测试 DeviceManager 的异常清理与重连逻辑。
func (s *Simulator) DisconnectAll() {
	s.clients.Range(func(_, v any) bool {
		v.(*client).close()
		return true
	})
}

// ClientCount 返回当前已连接客户端数。
func (s *Simulator) ClientCount() int {
	return int(s.clientCount.Load())
}

// DroppedFrames 返回所有客户端因 writeCh 缓冲满而丢弃的帧总数。
// 用于测试断言背压丢帧：慢客户端或不读 conn 的客户端会触发 send 的 default
// 分支丢帧，本方法聚合各 client.dropped 计数。并发安全（best-effort 遍历
// clients，遍历期间新增/移除的客户端可能不计入，但单帧计数的原子读是准确的）。
func (s *Simulator) DroppedFrames() int32 {
	var total int32
	s.clients.Range(func(_, v any) bool {
		total += v.(*client).dropped.Load()
		return true
	})
	return total
}

// SplitAddr 将 "host:port" 拆分为 host 和 port，便于从模拟器地址构造 device.Profile。
// 端口解析失败时返回 0。
func SplitAddr(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

// === 内部实现 ===

// acceptLoop 接受客户端连接，为每个连接创建 client 并启动 read/write goroutine。
//
// 错误处理策略（避免临时错误导致模拟器永久不可用，便于多设备并发测试时诊断）：
//   - net.ErrClosed：listener 被 Close 触发，属正常退出（Close 主动关闭），静默 return。
//   - 其他错误（如 EMFILE/too many open files 等临时资源耗尽）：用 slog 记录警告后
//     短暂退避（50ms）并继续重试 Accept，避免 acceptLoop 直接退出导致后续连接永久
//     无法接受。退避防 busy loop，不退出保证 listener 开放期间能恢复接受新连接。
func (s *Simulator) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// listener 被 Close 触发：正常退出，静默返回
				return
			}
			// 临时错误（如 EMFILE）：记录警告后短暂退避重试，避免永久不可用。
			// 不退出 acceptLoop，保证 listener 仍开放期间能恢复接受新连接。
			slog.Warn("sim acceptLoop: Accept 失败，将退避重试",
				"addr", s.actual, "err", err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		c := &client{
			conn:    conn,
			br:      bufio.NewReader(conn),
			writeCh: make(chan []byte, 64), // 缓冲 64 帧，慢客户端下背压丢帧而非阻塞
			done:    make(chan struct{}),
		}
		s.clients.Store(conn, c)
		s.clientCount.Add(1)
		s.wg.Add(2)
		go s.readLoop(c)
		go s.writeLoop(c)
	}
}

// readLoop 读取客户端命令，应用故障注入（drop/latency）后交 handler 处理，
// 并把响应写入 writeCh 串行化发送。连接关闭时退出并清理 client。
func (s *Simulator) readLoop(c *client) {
	defer s.wg.Done()
	defer func() {
		c.close()         // 幂等：确保 done 关闭、conn 关闭
		s.removeClient(c) // 从 clients 移除并递减计数
	}()
	for {
		cmd, err := s.cmdReader.ReadCommand(c.br)
		if err != nil {
			return // EOF 或错误 → 连接关闭，退出
		}
		// 故障注入：丢弃本次命令的响应
		if s.tryDrop() {
			continue
		}
		// 故障注入：模拟响应延迟
		if d := time.Duration(s.latency.Load()); d > 0 {
			t := time.NewTimer(d)
			select {
			case <-c.done:
				t.Stop()
				return
			case <-t.C:
			}
		}
		resp := s.handler.HandleCommand(cmd)
		if len(resp) > 0 {
			c.send(resp)
		}
	}
}

// writeLoop 从 writeCh 取帧写入连接，串行化所有写操作（响应 + 注入帧 + 采集帧），
// 避免交叉写导致 TCP 帧边界错乱。写错误或 done 关闭时退出。
func (s *Simulator) writeLoop(c *client) {
	defer s.wg.Done()
	for {
		select {
		case frame := <-c.writeCh:
			if _, err := c.conn.Write(frame); err != nil {
				// 写失败：连接已坏，通知 readLoop 退出
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// broadcast 向所有已连接客户端推送一帧（用于 handler 的 emit 与 InjectFrame）。
// 非阻塞：客户端 writeCh 满则丢帧，避免慢客户端拖累广播。
func (s *Simulator) broadcast(frame []byte) {
	s.clients.Range(func(_, v any) bool {
		v.(*client).send(frame)
		return true
	})
}

// tryDrop 尝试消费一次 drop 配额。返回 true 表示本次命令应被丢弃。
// 用 CAS 循环保证并发安全（多 readLoop 不会重复消费同一配额）。
func (s *Simulator) tryDrop() bool {
	for {
		n := s.dropNextN.Load()
		if n <= 0 {
			return false
		}
		if s.dropNextN.CompareAndSwap(n, n-1) {
			return true
		}
	}
}

// removeClient 从 clients 移除 client 并递减计数。
func (s *Simulator) removeClient(c *client) {
	if _, ok := s.clients.LoadAndDelete(c.conn); ok {
		s.clientCount.Add(-1)
	}
}

// client 表示一个已连接的客户端连接及其读写 goroutine 状态。
type client struct {
	conn      net.Conn
	br        *bufio.Reader
	// writeCh 故意永不关闭：client.close 只关 done 与 conn，保证 send 向 writeCh
	// 投递永不会因通道关闭而 panic。即使 client 已关闭，send 先经 closed.Load
	// 检查快速返回；若检查后并发关闭，select 的 default 分支也只会丢帧而非 panic。
	// 关闭一个已存在帧的 writeCh 会让 writeLoop 的 range 触发 panic，得不偿失，
	// 故采用"永不关闭 + closed 标志位 + select default"的组合保证安全。
	writeCh   chan []byte
	done      chan struct{} // 关闭时通知 writeLoop 退出
	closed    atomic.Bool
	closeOnce sync.Once
	// dropped 统计因 writeCh 缓冲满而在 send 的 default 分支丢弃的帧数，
	// 便于测试断言背压丢帧（见 Simulator.DroppedFrames）。
	dropped atomic.Int32
}

// close 幂等关闭客户端：关闭 done 通道与连接。
// 多次调用安全（closeOnce 保护）。注意不关闭 writeCh（见 writeCh 注释）。
func (c *client) close() {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.done)
		_ = c.conn.Close()
	})
}

// send 非阻塞地向 writeCh 投递一帧。
// 客户端已关闭或缓冲满时丢弃，避免阻塞调用方（故障注入可在任意 goroutine 调用）。
// 缓冲满时递增 dropped 计数，便于测试断言背压丢帧（见 Simulator.DroppedFrames）。
// 安全性依赖 writeCh 永不关闭（见 client.writeCh 注释），故 select 不会 panic。
func (c *client) send(frame []byte) {
	if c.closed.Load() {
		return
	}
	select {
	case c.writeCh <- frame:
	default:
		// 缓冲满：丢帧（慢客户端背压，优于阻塞整个模拟器），并计数便于诊断
		c.dropped.Add(1)
	}
}
