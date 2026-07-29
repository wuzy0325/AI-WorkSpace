// Package protocol 提供设备底层协议原语（帧解析、命令收发、单位系数等），
// 覆盖 DAQ-P-1604、T1603 等设备。本文件集中放置跨项目复用的"连接与命令发送"
// 脚手架，避免 wind-daq 与 daq-p1604 两个适配器各自实现导致同一 bug 分叉
// （典型案例：命令尾部 \r\n 曾在两份代码中各犯一次，详见 SendCommandNoNewline 的注释）。
package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// DialTCP 建立可选绑定本地 IP 的 TCP 连接。localAddress 为空时沿用系统路由。
//
// ADR-009 watchdog 兜底：net.Dialer.Dial 内部依赖 SetReadDeadline/SetWriteDeadline
// 实现 Timeout。在 ADR-009 那台 Windows 故障机器上 deadline 不可靠，Dial 可能永远
// 不返回，导致上层 Connect 永久卡死（前端"连接中"状态无法翻转）。
//
// 解决思路：把 Dial 放进子 goroutine 跑，主 goroutine 用 time.After 做软超时。
//   - 正常路径：Dial 在 timeout 内返回，主 goroutine 接收结果
//   - 软超时路径：主 goroutine 立即返回 timeout 错误，让上层 fail-fast
//   - Dial 在 timeout 后才返回：通过 select-default 检测到主线程已放弃，关闭 conn 防 FD 泄漏
//   - Dial 永远不返回（Windows deadline 不可靠场景）：泄漏一个 goroutine（4KB 栈），
//     主线程仍能立即返回错误，避免上层卡死。这是可接受代价。
//
// 第 2 条要求"独立 owner 能调用 conn.Close()"——但 Dial 还没返回 conn 句柄时
// 无法 Close。本函数用 timer + goroutine 模式绕过该限制，主线程不依赖 Dial 返回。
func DialTCP(address, localAddress string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		// 无 timeout 时直接 Dial，由调用方自行管理生命周期
		dialer := net.Dialer{}
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

	type dialResult struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan dialResult, 1)

	go func() {
		conn, err := dialer.Dial(network, address)
		select {
		case resultCh <- dialResult{conn, err}:
			// 主线程还在等待，正常交付结果
		default:
			// 主线程已超时返回，关闭 conn 防止 FD 泄漏
			if conn != nil {
				_ = conn.Close()
			}
		}
	}()

	select {
	case r := <-resultCh:
		return r.conn, r.err
	case <-time.After(timeout):
		// 软超时：不等待 Dial 返回，立即返回错误让上层 fail-fast。
		// Dial goroutine 仍可能阻塞（Windows deadline 不可靠），但主线程已释放。
		return nil, fmt.Errorf("dial %s: watchdog timeout (%s) - %w", address, timeout, os.ErrDeadlineExceeded)
	}
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
//
// ADR-009 契约（重要）：
//   - 本函数仅设置 SetWriteDeadline 做"软超时"，**不在 helper 内自动启动 watchdog
//     或 Close conn**（ADR-009 Rejected Alternative #4 明确禁止 helper 内自动 Close，
//     避免与调用方已有的 watchdog / 生命周期管理冲突）。
//   - 在故障 Windows 电脑上 SetWriteDeadline 可能失效，TCP 写缓冲满时 Write 永久阻塞。
//     调用方必须在调用本函数前已启动 WatchdogClose(conn, timeout) 覆盖 Write 阶段，
//     否则一旦 deadline 失效，调用方将永久卡死。
//   - 推荐调用模式（参考 daq-p1604 sendCommandACK / wind-daq sendCommand）：
//       wdStop := WatchdogClose(conn, timeout)
//       defer func() {
//           if wdStop() {
//               _ = conn.SetWriteDeadline(time.Time{})
//           }
//       }()
//       if err := SendCommandNoNewline(conn, cmd, timeout); err != nil {
//           return WrapWatchdogError(err, wdStop, "send "+cmd)
//       }
//   - timeout <= 0 时本函数不设 deadline，调用方必须通过 Close conn 解除阻塞
//     （例如 readLoop 退出后 Close，或外层握手 watchdog 兜底）。
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

// DrainConnection 在指定超时内排空连接中的残留数据，返回排空的总字节数和错误。
//
// 启动新采集前调用，避免旧命令响应或流数据污染帧对齐。
// 限制最大循环次数并在首次错误时立即退出，避免长时间阻塞导致模拟器/设备端的
// 命令读取 goroutine 因 deadline 超时提前退出。
//
// ADR-009 watchdog 兜底：SetReadDeadline 在故障 Windows 电脑不可靠，
// conn.Read 在 deadline 到期后仍可能无限阻塞。watchdog 在独立 timer goroutine 上跑，
// 总预算 = maxIters * timeout + 100ms 余量，超时后强制 conn.Close() 解除阻塞。
// watchdog 触发后 conn 失效，调用方必须废弃连接并重连（不可复用）。
//
// 返回值：
//   - int：累计读到的字节数（即使 watchdog 触发也保留已 drain 的字节数，便于日志诊断）
//   - error：watchdog 触发时返回包含 "watchdog triggered" 的错误；正常路径返回 nil
//
// 调用方必须检查 error：非 nil 表示 conn 已死，需重连；nil 表示 conn 可继续使用。
//
// nil 容忍：conn 为 nil 时返回 (0, nil)，不 panic。
func DrainConnection(conn net.Conn, timeout time.Duration) (int, error) {
	if conn == nil {
		return 0, nil
	}

	// watchdog 总预算覆盖最坏情况下所有循环迭代，预留 100ms 余量给单次 Read 边界。
	// 触发后 conn 失效，调用方必须废弃连接并重连。
	totalBudget := time.Duration(drainMaxIters)*timeout + 100*time.Millisecond
	wdStop := WatchdogClose(conn, totalBudget)

	buf := make([]byte, 4096)
	totalDrained := 0
	var firstErr error
	for i := 0; i < drainMaxIters; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
		}
		if err != nil {
			firstErr = err
			break // 超时或连接错误，均不再继续
		}
	}

	// watchdog 触发时 conn 已 Close，不清 deadline（无效操作 + 反模式）；
	// 未触发时清 deadline 让 conn 可复用，避免残留 deadline 影响后续命令。
	// firstErr 可能为 nil（循环正常结束）或非 nil（首次错误退出），watchdog
	// 触发时统一包装为 "watchdog triggered" 上下文以便调用方识别 conn 已死。
	if !wdStop() {
		err := firstErr
		if err == nil {
			err = net.ErrClosed
		}
		return totalDrained, fmt.Errorf("drain connection: %w; %w", err, ErrWatchdogTriggered)
	}
	_ = conn.SetReadDeadline(time.Time{})
	return totalDrained, nil
}

// maxResidualFrameSkips 是 P1604ReadCommandACK 在等待 ACK 时最多跳过的非 ASCII 残留帧数量。
//
// 快速启停采集场景下，TCP socket 内核接收缓冲和 FrameReader 应用层缓冲可能残留
// 二进制压力帧。这些帧会被 ReadFrame 当作 payload 返回，但不是 ASCII ACK，
// 必须跳过继续读取直到拿到真正的 'A' / 'Nxx'。
// 上限 20 既能覆盖极端残留场景（设备以 1000Hz 发送时约 20ms 数据），
// 又能避免在协议错位时无限循环。
const maxResidualFrameSkips = 20

// NoopWatchdogStop 是 WatchdogClose 的空操作占位，用于 timeout == 0 场景。
// 始终返回 true 表示 watchdog 未触发，调用方仍可继续使用 conn。
//
// 跨项目导出：wind-daq 的 sendCommand 在 timeout == 0 时可使用此占位，
// 保持与 WatchdogClose 相同的 stop 函数签名，统一调用方代码结构。
func NoopWatchdogStop() bool { return true }

// WatchdogClose 启动独立 watchdog 计时器，超时后强制 Close conn。
//
// 设计依据 ADR-009：SetReadDeadline 在某些 Windows 电脑不可靠，
// Read 在 deadline 到期后仍可能无限阻塞。必须有独立 owner 能在不等待
// 阻塞 goroutine 或其 mutex 的情况下调用 conn.Close() 解除阻塞。
//
// 跨项目导出：wind-daq 的 daq_p1064pre / dsa3217 sendCommand 通过
// import sharedproto 后调用 sharedproto.WatchdogClose(conn, timeout)，
// 避免在每个项目内复制 watchdog 实现。
//
// 调用方必须保证 timeout > 0；timeout == 0 时应使用 NoopWatchdogStop 占位，
// 避免触发 time.AfterFunc(0) 立即执行。本函数入口也会做防御性检查：
// conn == nil 或 timeout <= 0 时直接返回 NoopWatchdogStop，防止 timer goroutine
// 对 nil conn 调用 Close 导致 panic，或 timeout=0 导致立即关闭连接。
//
// 返回 stop 函数：
//   - 返回 true 表示 watchdog 未触发（正常完成），调用方仍可继续使用 conn
//   - 返回 false 表示 watchdog 已触发（conn 已被 Close），调用方必须放弃该 conn
//
// 调用方应在操作完成后立即调用 stop() 取消计时器；watchdog 已触发时
// stop 会等待 timer goroutine 完成 Close，避免与外层 defer Close 竞争。
//
// stop 函数幂等：多次调用安全。WrapWatchdogError + defer wdStop() 的常见
// 调用模式会触发多次 stop，必须保证第二次调用不阻塞、不重复等待。
// 修复前 bug：timer.Stop() 在首次调用成功后返回 true，第二次调用返回 false
// 进入 <-timedOut 等待，但 timer 从未触发时 timedOut 永不 close → 永久阻塞。
//
// 注意：watchdog 触发后 conn 失效，禁止复用。
func WatchdogClose(conn net.Conn, timeout time.Duration) func() bool {
	// 防御性检查：conn == nil 时 timer goroutine 调 conn.Close() 会 panic；
	// timeout <= 0 时 time.AfterFunc(0) 立即触发关闭 conn。
	// 返回 NoopWatchdogStop 占位，让调用方的 defer wdStop() + WrapWatchdogError 模式统一工作。
	if conn == nil || timeout <= 0 {
		return NoopWatchdogStop
	}
	timedOut := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		// 强制 Close 解除阻塞在 Read 上的 goroutine。
		// 即使 Close 返回错误（已被外层关闭）也无害。
		_ = conn.Close()
		close(timedOut)
	})
	var once sync.Once
	var triggered bool // 一次赋值后只读，受 once 保护无需额外锁
	return func() bool {
		once.Do(func() {
			if !timer.Stop() {
				// 计时器已触发（或已被其他 once 调用停止后再次调用），
				// 等待 timer goroutine 完成 Close 操作。
				<-timedOut
				triggered = true
			}
			// timer.Stop() 返回 true 表示计时器成功停止（未触发），
			// triggered 保持 false，后续调用直接复用此结果。
		})
		// 返回 true 表示 watchdog 未触发（conn 仍可用），
		// 返回 false 表示 watchdog 已触发（conn 已 Close，必须放弃）。
		return !triggered
	}
}

// interpretACKPayload 解析 DAQ-P-1604 命令应答 payload。
//
// 设备应答规则：
//   - "A"   表示命令成功
//   - "Nxx" 表示设备错误码（如 N05 数据字段错误）
//   - 其他  表示协议错位或残留数据，返回 unexpected
//
// 仅校验 payload 内容，不涉及帧读取；调用方负责确保 payload 来自完整帧。
func interpretACKPayload(payload []byte) error {
	response := string(payload)
	if response == "A" {
		return nil
	}
	if strings.HasPrefix(response, "N") {
		return fmt.Errorf("device returned error: %s", response)
	}
	return fmt.Errorf("unexpected command response: %q", response)
}

// ErrWatchdogTriggered 是 watchdog 触发后附加到错误的 sentinel 值。
//
// 使用 errors.Is(err, ErrWatchdogTriggered) 替代字符串匹配，保证即使
// 错误消息文案未来调整也不会静默失效。WrapWatchdogError、DrainConnection、
// P1604ReadCommandACK 在 watchdog 触发时统一用 %w 包装此 sentinel。
var ErrWatchdogTriggered = errors.New("watchdog triggered, conn closed")

// WrapWatchdogError 在 Read/Write 失败时附加 watchdog 上下文。
//
// 调用方在 Read/Write 失败路径调用此 helper 替代手动 if 判断：
//   - wdStop 返回 false（watchdog 已触发）：附加 "(watchdog triggered, conn closed)" 上下文
//     并用 %w 包装 ErrWatchdogTriggered sentinel，供 errors.Is 精确匹配
//   - wdStop 返回 true（watchdog 未触发）：仅附加 op 前缀
//
// op 参数应描述失败的具体操作（如 "read response header"、"write start acquisition"），
// 便于在错误日志中定位是哪一步失败。
//
// 注意：本函数不负责停止 watchdog，调用方仍需 defer 调用 wdStop 取消计时器，
// 否则 watchdog 计时器泄漏。
//
// 跨项目导出：wind-daq 的 daq_p1064pre / dsa3217 在 Read 和 Write 失败路径都用此 helper
// 统一错误包装模式，避免在每个调用点重复 if !wdStop() {...} 模板。
//
// 参数名 op（operation）避免与本包已 import 的标准库 context 包名冲突。
func WrapWatchdogError(err error, wdStop func() bool, op string) error {
	if err == nil {
		return nil
	}
	if !wdStop() {
		return fmt.Errorf("%s: %w; %w", op, err, ErrWatchdogTriggered)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// IsWatchdogTriggered 判断错误是否由 watchdog 触发导致。
//
// 用于调用方在 Read/Write 路径收到错误后，判断是否需要废弃连接（invalidate）。
//
// 判定依据：errors.Is(err, ErrWatchdogTriggered) 精确匹配 sentinel，
// 不依赖错误消息字面量。WrapWatchdogError、DrainConnection、P1604ReadCommandACK
// 在 watchdog 触发时统一用 %w 包装 ErrWatchdogTriggered。
//
// 跨项目导出：wind-daq 的 daq_p1604.go sendCommandACK 在 Read 阶段用此 helper
// 判断是否调用 invalidateConnection。
func IsWatchdogTriggered(err error) bool {
	return errors.Is(err, ErrWatchdogTriggered)
}

// P1604ReadCommandACK 读取并校验 DAQ-P-1604 命令应答。
//
// 实现要点（ADR-009）：
//  1. watchdog 兜底：timeout > 0 时启动独立计时器，超时强制 Close conn，
//     即使 SetReadDeadline 在故障 Windows 电脑上失效也能解除阻塞。
//  2. 跳帧循环：快速启停后残留二进制压力帧会污染 ACK 读取，循环跳过非 ASCII 帧，
//     直到读到 ASCII ACK（'A' / 'Nxx'）或达到 maxResidualFrameSkips 上限。
//  3. 上限保护：超过 maxResidualFrameSkips 仍读到非 ASCII 帧时返回错误，
//     避免协议错位时无限循环。
//  4. 总超时控制：循环外记录开始时间，循环内用 time.Until 计算剩余 deadline，
//     避免设备持续发送非 ASCII 帧导致总耗时 = maxResidualFrameSkips * timeout（最长 100s）。
//  5. 清除残留 deadline：timeout > 0 时退出前 SetReadDeadline(time.Time{})，
//     避免影响后续命令的 deadline 语义（daq-p1604 0.7.2 修复点）。
//
// timeout > 0 时同时设置 deadline + watchdog，双保险；
// timeout == 0 时不设 deadline 也不启动 watchdog，由调用方通过关闭连接解除阻塞
// （保持向后兼容语义，握手路径由外层 runHandshakeWatchdog 兜底）。
// 跳帧循环对所有 timeout 值都生效，覆盖握手期间的残留帧污染。
//
// 成功应答固定为 3 字节：[0x00, 0x03, 'A']。前两个字节是包含前缀的
// 整帧长度，FrameReader 返回的 payload 必须精确等于 "A"。
func P1604ReadCommandACK(reader *FrameReader, conn net.Conn, timeout time.Duration) error {
	if reader == nil {
		return fmt.Errorf("frame reader is nil")
	}
	if conn == nil {
		return fmt.Errorf("conn is nil")
	}

	// watchdog 仅在 timeout > 0 时启动；timeout == 0 时由调用方负责 Close。
	// 不启动 watchdog 时用 NoopWatchdogStop 占位，保持后续 !wdStop() 判断统一。
	var wdStop func() bool = NoopWatchdogStop
	if timeout > 0 {
		wdStop = WatchdogClose(conn, timeout)
	}

	// 总超时控制：循环外记录开始时间，循环内用剩余时间作为单次 Read deadline。
	// 避免设备持续发送非 ASCII 帧导致总耗时无限延长。
	// timeout == 0 时不参与总超时控制，由外层 Close 决定何时解除阻塞。
	overallStart := time.Now()

	var ackErr error
	skipped := 0
	for ; skipped < maxResidualFrameSkips; skipped++ {
		if timeout > 0 {
			// 每次循环用剩余总超时设置 deadline，覆盖旧值。
			// 剩余不足时立即设为 1ms 触发超时，避免负数 AddValue。
			remaining := time.Until(overallStart.Add(timeout))
			if remaining <= 0 {
				ackErr = fmt.Errorf("read command response: %w", context.DeadlineExceeded)
				break
			}
			_ = conn.SetReadDeadline(time.Now().Add(remaining))
		}
		payload, readErr := reader.ReadFrame()
		if readErr != nil {
			ackErr = fmt.Errorf("read command response: %w", readErr)
			break
		}
		// ASCII 帧为 ACK 应答；非 ASCII 视为残留压力帧跳过继续读。
		if len(payload) > 0 && IsASCIIFrame(payload) {
			ackErr = interpretACKPayload(payload)
			break
		}
	}
	// 跳帧循环到上限仍无 ACK，视为协议错位。
	if ackErr == nil && skipped == maxResidualFrameSkips {
		ackErr = fmt.Errorf("too many residual frames (>%d) while waiting for ACK", maxResidualFrameSkips)
	}

	// watchdog 触发时 conn 已被 Close，返回错误需附加上下文。
	// wdStop 返回 false 表示 watchdog 已触发（conn 已失效）。
	// 先检查 wdStop 再清除 deadline：watchdog 触发时 conn 已 Close，
	// 清除 deadline 调用会失败被忽略，逻辑上应直接返回错误。
	if !wdStop() {
		if ackErr == nil {
			ackErr = net.ErrClosed
		}
		return fmt.Errorf("%w; %w", ackErr, ErrWatchdogTriggered)
	}

	// 清除残留 deadline，避免影响后续命令的 deadline 语义。
	// 仅在 timeout > 0 时调用，保持 timeout == 0 不触碰 deadline 的语义。
	// 此时 watchdog 未触发，conn 仍有效，清除 deadline 必然成功。
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Time{})
	}

	return ackErr
}
