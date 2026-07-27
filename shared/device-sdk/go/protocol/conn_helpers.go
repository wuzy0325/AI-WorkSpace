// Package protocol 提供设备底层协议原语（帧解析、命令收发、单位系数等），
// 覆盖 DAQ-P-1604、T1603 等设备。本文件集中放置跨项目复用的"连接与命令发送"
// 脚手架，避免 wind-daq 与 daq-p1604 两个适配器各自实现导致同一 bug 分叉
// （典型案例：命令尾部 \r\n 曾在两份代码中各犯一次，详见 SendCommandNoNewline 的注释）。
package protocol

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// DialTCP 建立可选绑定本地 IP 的 TCP 连接。localAddress 为空时沿用系统路由。
func DialTCP(address, localAddress string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	network := "tcp"
	if localAddress = strings.TrimSpace(localAddress); localAddress != "" {
		ip := net.ParseIP(localAddress)
		if ip == nil {
			return nil, fmt.Errorf("invalid local address %q", localAddress)
		}
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
			network = "tcp4"
		} else {
			network = "tcp6"
		}
		dialer.LocalAddr = &net.TCPAddr{IP: ip}
	}
	return dialer.Dial(network, address)
}

// StopReasonUserRequested 表示调用方主动停止（StopAcquisition / Disconnect）。
// readLoop 识别到该原因后静默退出，避免误判为连接异常。
const StopReasonUserRequested = "user-requested"

// ReadLoopJoinTimeout 是等待 readLoop 退出的最长时间。
// 超过这个时间不再阻塞调用方，但 readLoop 自身仍会退出（不会泄漏）。
const ReadLoopJoinTimeout = 1 * time.Second

// SendCommandNoNewline 向设备发送命令（纯 ASCII，不附加任何换行符）。
//
// 实测设备（型号 9116 / 固件 00F8）在 w1601 长度前缀模式下将 \r\n 视为命令字符的
// 一部分，对部分命令（如 u01101）返回 N05 数据字段错误。所有命令均不带换行符，
// 由设备自行解析命令边界。
//
// 参数：
//   - conn: 已建立的 TCP 连接（非 nil）；为 nil 时返回错误
//   - cmd: 命令字符串（不含换行符）
//   - timeout: 写超时；<=0 时不设置 deadline（适合调用方已自行管理 deadline 的场景）
//
// 调用方负责加锁（多设备/多 goroutine 场景），本函数只做"设 deadline + 写裸命令"。
func SendCommandNoNewline(conn net.Conn, cmd string, timeout time.Duration) error {
	if conn == nil {
		return errors.New("conn is nil")
	}
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
		defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	}
	_, err := conn.Write([]byte(cmd))
	return err
}

// StopReasonTracker 维护"主动停止原因"，用于 readLoop 区分调用方主动停止
// 与连接意外断开。多次设置只保留首个非空原因。
//
// 嵌入使用：
//
//	type driver struct {
//	    sharedproto.StopReasonTracker
//	    // ...
//	}
//	d.SetStopReason(sharedproto.StopReasonUserRequested)
//	if d.GetStopReason() == sharedproto.StopReasonUserRequested { ... }
type StopReasonTracker struct {
	mu     sync.Mutex
	reason string
}

// SetStopReason 设置主动停止原因。多次设置只保留首个非空值。
func (t *StopReasonTracker) SetStopReason(reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.reason == "" {
		t.reason = reason
	}
}

// GetStopReason 读取主动停止原因（"" 表示未主动停止）。
func (t *StopReasonTracker) GetStopReason() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reason
}

// ClearStopReason 清空主动停止原因，供下一次采集周期复用。
func (t *StopReasonTracker) ClearStopReason() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reason = ""
}

// IsConnectionFault 启发式判定错误是否由连接故障引起，用于日志分级。
//
// 在停止/关闭路径上把 "连接已经断开导致命令失败" 降级为 debug 日志，
// 避免在正常断开/重连流程中刷出大量 warn 噪音。
//
// 仅作日志分级用途，不可作为状态机的输入条件。
func IsConnectionFault(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "reset by peer") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "device disconnected")
}

// IsConnResetByPeer 判定错误是否表明对端已主动关闭/重置 TCP 连接（FIN/RST）。
//
// 与 IsConnectionFault 的语义差异：
//   - IsConnectionFault：用于日志降噪，范围宽泛（含 timeout）
//   - IsConnResetByPeer：用于状态机决策，只匹配"对端已不可达"的硬证据
//
// 匹配范围：
//   - io.EOF：对端正常 FIN（如设备固件异常主动关闭）
//   - connection reset by peer：对端 RST
//   - broken pipe：本地向已 RST 的连接写入
//   - wsasend/wsarecv ... aborted：Windows 内核层 WSAECONNABORTED
//     （通常由本地 TCP 栈在检测到对端长时间无响应后主动 RST 半死连接）
//   - connection abort*：跨平台连接中止表述
//
// 用途：Connect 阶段命令（如 u01101）失败时区分"软错误"（解析失败、超时）
// 与"硬错误"（连接已死）。硬错误必须让 Connect 失败，避免把已死的连接
// 塞进 shard.drivers 造成后续 StartAcquisition 爆 WSAECONNABORTED 假象。
func IsConnResetByPeer(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection abort") ||
		strings.Contains(msg, "wsasend") ||
		strings.Contains(msg, "wsarecv")
}

// IsClosedConnError 判断错误是否由连接被主动关闭引起。
// 与 IsConnectionFault 的区别：本函数只匹配 "主动 close" 场景（net.ErrClosed
// 或 "use of closed network connection"），用于 readLoop 退出时区分
// "调用方 Close 触发的读取错误" 与 "网络异常触发的读取错误"。
func IsClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed network connection")
}

// drainMaxIters 是 DrainConnection 的最大读取次数安全上限。
// 3 次足够覆盖 "1 次残留 + 1 次延迟到达 + 1 次确认无数据" 的典型场景。
const drainMaxIters = 3

// DrainConnection 在指定超时内排空连接中的残留数据，返回排空的总字节数。
//
// 启动新采集前调用，避免旧命令响应或流数据污染帧对齐。
// 限制最大循环次数并在首次错误时立即退出，避免长时间阻塞导致模拟器/设备端的
// 命令读取 goroutine 因 deadline 超时提前退出。
//
// 返回值：累计读到的字节数。调用方可据此决定是否打 debug 日志（例如
// wind-daq 原实现就有 "drained residual data" 的日志）。
//
// 与 DrainW1601Response 的区别：本函数用于"通用排空"（启动采集前清空连接缓冲区），
// 直接读裸字节；DrainW1601Response 用于"按帧排空 w1601 应答"，使用 FrameReader。
//
// nil 容忍：conn 为 nil 时返回 0，不 panic。
func DrainConnection(conn net.Conn, timeout time.Duration) int {
	if conn == nil {
		return 0
	}
	buf := make([]byte, 4096)
	totalDrained := 0
	for i := 0; i < drainMaxIters; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
		}
		if err != nil {
			break // 超时或连接错误，均不再继续
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	return totalDrained
}

// DrainW1601Response 排空 w1601 启用应答。
//
// w1601 启用长度前缀后，设备会回一个 A 应答（带长度前缀的 1 字节 ASCII）。
// 若不排空，该应答会被后续 u01101 的 ReadFrame 误读为 u01101 响应。
//
// 超时即返回：设备未发应答或应答已丢失均不视为错误。
// 最多读 2 次以容忍延迟到达的应答。
//
// nil 容忍：reader 或 conn 为 nil 时直接返回，不 panic。
// 退出前清理 read deadline，与 DrainConnection 行为保持一致。
func DrainW1601Response(reader *FrameReader, conn net.Conn, timeout time.Duration) {
	if reader == nil || conn == nil {
		return
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	for i := 0; i < 2; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		_, err := reader.ReadFrame()
		if err != nil {
			// 超时或错误：不再继续读，剩余数据（若有）由后续命令的 ReadFrame 处理
			return
		}
	}
}
