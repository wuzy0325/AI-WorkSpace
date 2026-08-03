package hardware

import (
	"errors"
	"fmt"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/pkg/slog"
	"shared.local/device-sdk/go/protocol"
)

// noDataTimeout 是允许的最长无数据时间，超过则判定为连接异常。
// readLoop 入口启动独立 time.AfterFunc timer，到期触发连接毒化。
//
// ADR-009 R0-10：原实现 readLoop 通过 SetReadDeadline(200ms) 让 ReadFrame 周期返回，
// 在循环体累计 time.Since(lastDataAt) > noDataTimeout 检测无数据。问题 Windows 电脑
// deadline 失效时循环体不可达，noDataTimeout 永远不会被触发，半开连接无法自行收敛。
// 独立 timer 不依赖循环体，即使 Read 永久阻塞也能到期触发。
//
// 使用 var 而非 const 是为了允许测试注入短超时（200ms）加速 no-data timer 用例；
// 运行期不应修改，生产代码保持默认 10s。同一包内测试默认串行执行，覆盖安全。
var noDataTimeout = 10 * time.Second

// stopAcquisitionTimeout 限制 StopAcquisition 的总耗时预算。
// 设计动机：readLoop 在该预算内等待 ErrControlACK；超时直接 Close 连接，
// 防止问题电脑 deadline 失效导致永久阻塞（ADR-009）。
// 实测 ACK 在 @f1 后约 1ms 到达，3s 预算留足余量。
var stopAcquisitionTimeout = 3 * time.Second

// stopQuietFallbackTimeout 是 Stop 静默窗口的 Go timer 兜底。
// 问题电脑 SetReadDeadline 失效时，readLoop 会永久阻塞在 conn.Read（静默窗口），
// 只有 StopAcquisition owner 关闭 conn 才能解除。此 timer 在静默窗口（150ms）
// 加余量后到期：若 readLoop 仍未完成（done 未关闭），关闭 conn 解除阻塞，
// 把问题电脑的 Stop 从 stopAcquisitionTimeout（3s）降到约 350ms（连接需重连，
// 符合 ADR-009“问题电脑 deadline 失效时必须 Close 兜底”）。
// 健康电脑上 readLoop 在 deadline 下约 160ms 完成，timer 到期时 done 已关闭，
// 不会误关连接。
const stopQuietFallbackTimeout = 350 * time.Millisecond

const maxConsecutiveFrameErrors = 5

// configSyncTotalTimeout 限制 Connect 阶段配置同步的总耗时预算。
// 设计动机：readAllConfig 串行发送 8 条 @fd 查询命令，最坏每条 1s + 每字节 100ms，
// 极端情况下可超过 8s，会踩穿 Wails binding 默认 5s 调用超时，UI 误判连接失败。
// 4s 预算下：8 条命令平均每条 500ms，正常设备通常 50-200ms 完成，留足重试余量。
// 超过预算立即返回错误，让 Connect fail-fast，操作员可重新点连接。
const configSyncTotalTimeout = 4 * time.Second

// configSyncMinRemaining 单条命令执行前剩余时间的最低阈值。
// 若剩余时间不足此阈值，认为已无法完成下一条命令，提前返回 deadline 错误，
// 避免单条命令跨过总 deadline 导致总耗时超出预算。
const configSyncMinRemaining = 200 * time.Millisecond

const (
	acquisitionReconnectAttempts = 3
	acquisitionReconnectDelay    = time.Second
)

const (
	DAQ_T_1603_DEFAULT_HOST = "192.168.1.7"
	DAQ_T_1603_DEFAULT_PORT = 9000
	DAQ_T_1603_TIMEOUT      = 5 * time.Second
)

type onConfigSyncedFn func(core.DaqT1603HardwareConfig)

type LogEntry struct {
	Level    string
	Category string
	DeviceID string
	Message  string
	Detail   string
}

type DAQT1603 struct {
	mu        sync.RWMutex
	logMu     sync.RWMutex
	writeMu   sync.Mutex
	profile   core.Profile
	status    core.Status
	sink      core.DataSink
	stop      chan struct{}
	acquiring bool
	streaming bool
	stopping  bool
	// stopAbandoned 表示 Stop owner 已在静默兜底（deadline 失效故障电脑）路径
	// 主动废弃连接：readLoop 随后因对端 FIN 收到的 EOF 属预期退出，不应再触发
	// onReadLoopExit 回调或"Read loop exited unexpectedly"告警。连接在下一次
	// StartAcquisition 时透明重连。仅在 connectLocked 成功与 Disconnect 时复位。
	stopAbandoned bool
	// deadlineProbed/deadlineBroken：机器 SetReadDeadline 是否失效的探测
	// 结果（连接建立后的空窗期探测一次，device 级缓存）。部分 Windows
	// 机器安全软件（LSP hook winsock）使 deadline 取消失效，阻塞 Read
	// 永不返回；此时 Stop 静默确认窗口改用 goroutine + 定时器实现，
	// 且连接在 Stop 完成后废弃（遗留读 goroutine 使连接不可复用）。
	deadlineProbed         bool
	deadlineBroken         bool
	conn                   net.Conn
	frameReader            *protocol.T1603FrameReader
	config                 core.DaqT1603HardwareConfig
	onConfigSynced         onConfigSyncedFn
	onReadLoopExit         func(error)
	onLog                  func(LogEntry)
	readErrors             int
	frameErrors            int
	consecutiveFrameErrors int
	readLoopDone           chan struct{}
	dialTCP                func(string, string, time.Duration) (net.Conn, error)
}

func NewDAQT1603(profile core.Profile) *DAQT1603 {
	return &DAQT1603{
		profile: profile,
		config:  profile.DaqT1603Config,
		dialTCP: protocol.DialTCP,
		status: core.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: core.ConnectionDisconnected,
		},
	}
}

func (d *DAQT1603) ID() string { return d.profile.ID }

func (d *DAQT1603) OnConfigSynced(fn onConfigSyncedFn) {
	d.mu.Lock()
	d.onConfigSynced = fn
	d.mu.Unlock()
}

func (d *DAQT1603) OnReadLoopExit(fn func(error)) {
	d.mu.Lock()
	d.onReadLoopExit = fn
	d.mu.Unlock()
}

func (d *DAQT1603) OnLog(fn func(LogEntry)) {
	d.logMu.Lock()
	d.onLog = fn
	d.logMu.Unlock()
}

func (d *DAQT1603) emitLog(level string, category string, message string, detail string) {
	d.logMu.RLock()
	fn := d.onLog
	d.logMu.RUnlock()
	if fn == nil {
		return
	}
	fn(LogEntry{
		Level:    level,
		Category: category,
		DeviceID: d.profile.ID,
		Message:  message,
		Detail:   detail,
	})
}

// sendCommand 发送设置类命令（@fe / @f3 等 ACK 命令）并校验单字节响应。
//
// 响应契约（device-lab/skills/daq-t1603/SKILL.md:683、702）：
//   - 'A'：成功。
//   - 'E'：设备拒绝，完整合法的错误响应；返回 ErrDeviceRejected 业务错误，
//     调用方应终止当前操作但**不**毒化连接（连接边界仍可信）。
//   - 其他字节：非法协议响应，返回协议错误；调用方按 ADR-009 决定是否毒化。
//
// 注意：本函数不直接调用 invalidateConnection；毒化与否由调用方根据错误类型决定。
// ErrDeviceRejected 不应触发毒化（连接仍可用），ErrWatchdogTriggered 必须毒化。
func (d *DAQT1603) sendCommand(conn net.Conn, cmd string) (string, error) {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	resp, err := protocol.SendCommandExact(conn, cmd, 1)
	if err != nil {
		d.emitLog("error", "hardware-recv", "Command failed", err.Error())
		return "", err
	}
	// SendCommandExact 已 TrimRight \r\n 空格，单字节响应直接取首字符。
	body := strings.TrimSpace(resp)
	if len(body) == 0 {
		// 空响应属非法协议响应（设备应答 A/E，不应答空），按协议错误处理。
		err := fmt.Errorf("command %q: empty response (protocol violation)", cmd)
		d.emitLog("error", "hardware-recv", "Protocol violation", err.Error())
		return "", err
	}
	switch body[0] {
	case 'A':
		d.emitLog("debug", "hardware-recv", "Received response", "A")
		return "A", nil
	case 'E':
		// 设备拒绝：业务错误，不毒化连接。
		err := fmt.Errorf("command %q: %w", cmd, protocol.ErrDeviceRejected)
		d.emitLog("warn", "hardware-recv", "Device rejected command", cmd)
		return "", err
	default:
		// 非 A/E 字节：协议错位，连接边界不可信。
		err := fmt.Errorf("command %q: invalid ACK %q (expected A or E)", cmd, body)
		d.emitLog("error", "hardware-recv", "Protocol violation", err.Error())
		return "", err
	}
}

func (d *DAQT1603) sendCommandExact(conn net.Conn, cmd string, n int) (string, error) {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	resp, err := protocol.SendCommandExact(conn, cmd, n)
	if err != nil {
		d.emitLog("error", "hardware-recv", "Command failed", err.Error())
		return "", err
	}
	d.emitLog("debug", "hardware-recv", "Received response", strings.TrimSpace(resp))
	return resp, nil
}

// writeCommandOnly 写命令但不读响应。
// conn 参数由调用方在持有 d.mu 时传入，避免直接访问 d.conn 产生数据竞态。
//
// ADR-009 R0-2：watchdog 必须在 writeMu.Lock 之前启动。
// 历史背景：原实现先 d.writeMu.Lock() 再 SetWriteDeadline + Write，无 watchdog 兜底。
// 问题 Windows 电脑 SetWriteDeadline 失效时 Write 永久阻塞，writeMu 永远无法释放，
// 后续所有命令路径（applyHardwareConfig / stopAcquisitionLocked 等）死锁。
//
// 整改后：watchdog 在 writeMu.Lock 之前启动，timeout 后强制 Close conn 解除 Write 阻塞，
// writeMu 得以释放。watchdog 触发时返回 ErrWatchdogTriggered，调用方需调用
// invalidateConnection 统一毒化驱动状态。
func (d *DAQT1603) writeCommandOnly(conn net.Conn, cmd string) error {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	if conn == nil {
		return fmt.Errorf("device not connected")
	}
	// R0-2：watchdog 在 Lock 之前启动，确保即使上一任锁持有者在 Write 中阻塞，
	// watchdog 也能 Close conn 解除阻塞，让本 goroutine 最终拿到 writeMu。
	wdStop := protocol.WatchdogClose(conn, DAQ_T_1603_TIMEOUT)
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(DAQ_T_1603_TIMEOUT))
	_, err := conn.Write([]byte(cmd))
	_ = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		d.emitLog("error", "hardware-send", "Command write failed", err.Error())
		// watchdog 触发：conn 已 Close，返回 ErrWatchdogTriggered 让调用方毒化连接。
		if !wdStop() {
			return fmt.Errorf("write %q: %w; %w", cmd, err, protocol.ErrWatchdogTriggered)
		}
		return err
	}
	// Write 成功但 watchdog 触发：conn 已被 Close，后续 I/O 会失败。
	// 返回 ErrWatchdogTriggered 让调用方毒化连接。
	if !wdStop() {
		d.emitLog("warn", "hardware-send", "Command write watchdog triggered after Write",
			"conn closed by watchdog; reconnect required")
		return fmt.Errorf("write %q: %w", cmd, protocol.ErrWatchdogTriggered)
	}
	return nil
}

// Stop排空由唯一readLoop消费完整尾帧和@f1 ACK，再读取至静默确认完成。
// 不启动第二个reader；socket deadline只提供静默窗口，Stop owner独立总超时后直接Close。

func (d *DAQT1603) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connectLocked()
}

// probeDeadlineBroken 检测本机 SetReadDeadline 是否生效：设置 20ms deadline
// 后 goroutine 阻塞读，30ms 定时判定——goroutine 返回 timeout 表示生效；
// 定时先到（goroutine 仍阻塞）表示失效（2026-07-31 实测：500ms deadline 下
// Read 阻塞 >60s）。必须由调用方在连接建立后的空窗期调用（无数据推送，
// 判定不受数据到达干扰）；判定失效时遗留的阻塞读 goroutine 由调用方废弃
// 连接（Close）解除。
func probeDeadlineBroken(conn net.Conn) bool {
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := conn.Read(buf)
		done <- err
	}()
	select {
	case err := <-done:
		_ = conn.SetReadDeadline(time.Time{})
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return false
		}
		// 收到数据或连接级错误：空窗期偶发情况，无法判定，按"生效"处理
		// （走同步路径；若实际失效由 Stop 的 350ms 兜底保证正确性）。
		return false
	case <-time.After(30 * time.Millisecond):
		return true
	}
}

func (d *DAQT1603) connectLocked() error {
	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = DAQ_T_1603_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_T_1603_DEFAULT_PORT
	}

	// ADR-009：net.DialTimeout 内部依赖 deadline，在 Windows 故障机器上不可靠。
	// 改用 protocol.DialTCP（带 watchdog goroutine 兜底），主线程在 timeout 后
	// 立即返回错误而不依赖 Dial 返回，避免 Connect 永久卡死（前端"连接中"无法翻转）。
	conn, err := d.dialTCP(fmt.Sprintf("%s:%d", host, port), "", DAQ_T_1603_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(10 * time.Second)
	}

	d.conn = conn
	d.frameReader = protocol.NewT1603FrameReader(conn)
	// 连接建立后是事务空窗期（设备无数据推送），探测机器 SetReadDeadline
	// 是否失效（安全软件 LSP hook winsock 时失效）结果可靠，device 级缓存
	// 一次。失效则废弃本连接并重连一次：探测的阻塞读 goroutine 遗留在此
	// 连接上，Close 解除后重连获得干净连接，后续 Stop 才能安全走
	// goroutine 静默窗口路径。
	if !d.deadlineProbed {
		d.deadlineProbed = true
		if probeDeadlineBroken(conn) {
			d.deadlineBroken = true
			d.emitLog("warn", "system", "Read deadline broken on this machine",
				"Stop uses goroutine quiet window; conn recycled after each stop")
			// 探测的阻塞读 goroutine 仍挂在本连接上：问题电脑 conn.Close()
			// 在挂起 Read 时阻塞（closesocket 等待未完成重叠读取），直接
			// 调用会卡死 Connect。AbortConnection（CloseWrite FIN + Close）
			// 放后台执行解除探测 goroutine，WatchdogClose 兜底 500ms 内
			// 强制 AbortConnection（与 Stop 兜底的连接收尾一致）。
			go protocol.AbortConnection(conn)
			wdStop := protocol.WatchdogClose(conn, 500*time.Millisecond)
			go func() { _ = wdStop() }()
			d.conn = nil
			d.frameReader = nil
			// 设备仅允许单连接：旧连接的 FIN 传播 + 设备释放槽位需要时间，
			// 立即重连会被设备拒绝（@e3 读取 EOF，2026-07-31 实机复现）。
			// CloseWrite(FIN) 在 AbortConnection 内同步首步执行，此刻已发出；
			// 等一小段让设备处理完 FIN 再拨号。仅此一次（deadlineProbed 已置位）。
			time.Sleep(200 * time.Millisecond)
			return d.connectLocked()
		} else {
			d.emitLog("info", "system", "Read deadline works on this machine",
				"Stop uses sync quiet window; conn reused after stop")
		}
	}
	d.frameReader.SetDeadlineBroken(d.deadlineBroken)
	d.frameReader.SetBinaryMode(d.config.BinaryFormat)
	d.frameReader.SetMetadataMode(d.config.ShowTimestamp)
	d.frameReader.SetSequenceMode(d.config.ShowSequence)
	d.status.Connection = core.ConnectionConnected
	d.status.LastError = ""
	// 连接已恢复（可能由 Stop 静默兜底后的自动重连），readLoop 随后的
	// EOF/断线必须重新视为异常退出并通知上层。
	d.stopAbandoned = false

	if err := d.syncHardwareConfigLocked(conn); err != nil {
		go protocol.AbortConnection(conn)
		wdStop := protocol.WatchdogClose(conn, 500*time.Millisecond)
		go func() { _ = wdStop() }()
		d.conn = nil
		d.frameReader = nil
		// R0-4：watchdog 触发导致的连接失败置 Error 状态，区别于普通连接失败。
		// 普通连接失败（DNS/network unreachable）保持 Disconnected，让用户重试；
		// watchdog 触发意味着 deadline 失效或设备无响应，置 Error 提示重连。
		if errors.Is(err, protocol.ErrWatchdogTriggered) {
			d.status.Connection = core.ConnectionError
			d.status.LastError = fmt.Sprintf("config sync watchdog triggered: %v", err)
		} else {
			d.status.Connection = core.ConnectionDisconnected
		}
		return err
	}

	return nil
}

func (d *DAQT1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.acquiring = false
	d.status.Acquiring = false
	d.stopping = true
	done := d.readLoopDone
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
	}

	if d.conn != nil {
		conn := d.conn
		// ADR-009 对齐：写 @f1 前启动 watchdog，防止问题电脑 SetWriteDeadline 失效
		// 时 Write 永久阻塞并永久持有 writeMu（阻塞后续所有命令路径）。
		wdStop := protocol.WatchdogClose(conn, DAQ_T_1603_TIMEOUT)
		d.writeMu.Lock()
		_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = conn.Write([]byte("@f1"))
		_ = conn.SetWriteDeadline(time.Time{})
		d.writeMu.Unlock()
		wdStop()
		// 故障电脑上 conn.Close() 在挂起 Read 时可能永久阻塞（安全软件 hook
		// winsock）：FIN+Close 放到后台 goroutine，由下方的 readLoop join
		// （3s 超时）收敛。CloseWrite 发送 FIN 让设备释放连接槽位，readLoop
		// 读到对端 EOF 后自行退出，Close 随后完成。
		go protocol.AbortConnection(conn)
		d.conn = nil
		d.frameReader = nil
	}
	if done != nil {
		d.mu.Unlock()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			d.mu.Lock()
			d.status.Connection = core.ConnectionError
			d.status.LastError = "read loop did not exit during disconnect"
			return fmt.Errorf("read loop did not exit during disconnect")
		}
		d.mu.Lock()
	}
	d.streaming = false
	d.stopping = false
	// 连接生命周期已结束，复位 stopAbandoned：下一次 connectLocked 成功后
	// 的 readLoop EOF 必须重新视为异常退出并通知上层。
	d.stopAbandoned = false
	d.readLoopDone = nil

	d.status.Connection = core.ConnectionDisconnected
	return nil
}

func (d *DAQT1603) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.acquiring {
		return nil
	}
	if d.stopping {
		return fmt.Errorf("stop in progress")
	}
	if d.streaming && d.conn != nil && d.readLoopDone != nil {
		d.acquiring = true
		d.status.Acquiring = true
		d.status.Connection = core.ConnectionAcquiring
		return nil
	}
	if d.readLoopDone != nil {
		done := d.readLoopDone
		stuckConn := d.conn
		d.mu.Unlock()
		slog.Info("DAQ-T-1603 waiting for readLoop to exit", "device", d.profile.ID)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			// ADR-009 R0-1 整改：join 超时必须废弃连接，禁止清空 readLoopDone
			// 后在原连接上启动第二个 reader。
			//
			// 历史背景：原实现只打 warn 日志，随后清空 readLoopDone 并继续在原连接
			// 上启动新 readLoop。问题 Windows 电脑 deadline 失效导致旧 reader 仍阻塞
			// 在 conn.Read 时，新 reader 会与旧 reader 竞争同一 TCP 字节流，产生
			// 错帧、命令响应被抢读和不可恢复的协议错位。
			//
			// 整改后：join 超时调用 invalidateConnectionAfterReadLoopTimeout
			// 统一 Close conn、清 conn/frameReader、置 Error 状态、返回 reconnect
			// required。不继续发送 @f1/@f0，不启动新 readLoop。
			slog.Warn("DAQ-T-1603 timeout waiting for readLoop to exit; invalidating connection",
				"device", d.profile.ID)
			d.invalidateConnectionAfterReadLoopTimeout(stuckConn, "readLoop did not exit before StartAcquisition; reconnect required")
			// invalidate 内部自加锁配对释放；return 前重新持锁匹配调用方 defer Unlock。
			d.mu.Lock()
			return fmt.Errorf("readLoop did not exit before StartAcquisition; reconnect required")
		}
		d.mu.Lock()
		d.readLoopDone = nil
	}
	if d.conn == nil {
		if err := d.reconnectAcquisitionLocked(); err != nil {
			return fmt.Errorf("reconnect acquisition transport: %w", err)
		}
	}

	if d.frameReader != nil {
		d.frameReader.Reset()
		d.frameReader.ExpectControlACK()
	}

	mask := d.config.ChannelMask
	if mask == "" {
		mask = "FFFF"
	}

	cmd := fmt.Sprintf("@f0 %s 2", mask)
	slog.Info("DAQ-T-1603 sending start acquisition command", "device", d.profile.ID, "cmd", cmd)
	err := d.writeCommandOnly(d.conn, cmd)
	if err != nil {
		// R0-4：writeCommandOnly 返回 ErrWatchdogTriggered 时统一毒化连接。
		// 当前持有 d.mu，直接改字段（与 @f1 路径一致），不调用 invalidateConnection
		// 避免与已持有的锁死锁。watchdog 已 Close conn，此处仅清空 d.conn 引用并置 Error。
		if errors.Is(err, protocol.ErrWatchdogTriggered) {
			d.conn = nil
			d.frameReader = nil
			d.status.Connection = core.ConnectionError
			d.status.LastError = fmt.Sprintf("send %s: watchdog triggered; reconnect required", cmd)
			d.emitLog("error", "hardware-send", "Start command watchdog triggered",
				"conn closed by watchdog; reconnect required")
		}
		return fmt.Errorf("send %s: %w", cmd, err)
	}

	// The single read loop consumes the verified @f0 ACK before data frames.

	d.acquiring = true
	d.streaming = true
	d.stopping = false
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})

	go d.readLoop(d.conn, d.frameReader, d.readLoopDone)
	return nil
}

func (d *DAQT1603) reconnectAcquisitionLocked() error {
	var lastErr error
	for attempt := 1; attempt <= acquisitionReconnectAttempts; attempt++ {
		if err := d.connectLocked(); err == nil {
			return nil
		} else if lastErr == nil {
			lastErr = err
		}
		if attempt < acquisitionReconnectAttempts {
			time.Sleep(acquisitionReconnectDelay)
		}
	}
	return lastErr
}

func (d *DAQT1603) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *DAQT1603) stopAcquisitionLocked() error {
	d.acquiring = false
	d.status.Acquiring = false
	if !d.streaming || d.conn == nil || d.frameReader == nil || d.readLoopDone == nil {
		if d.status.Connection == core.ConnectionAcquiring {
			d.status.Connection = core.ConnectionConnected
		}
		return nil
	}

	conn := d.conn
	fr := d.frameReader
	done := d.readLoopDone
	d.stopping = true
	fr.ExpectControlACKAfterFrames()

	if err := d.writeCommandOnly(conn, "@f1"); err != nil {
		d.mu.Unlock()
		d.invalidateConnection(conn, fmt.Sprintf("send @f1: %v; reconnect required", err))
		d.mu.Lock()
		return fmt.Errorf("send @f1: %w; reconnect required", err)
	}

	// Do not hold d.mu while waiting: readLoop needs it to observe stopping and
	// finish after consuming the terminal ACK. The timeout owner closes conn
	// directly, so a Windows Read that ignores deadlines cannot hang Stop.
	d.mu.Unlock()

	slog.Info("DAQ-T-1603 stop: waiting for readLoop to consume tail + ACK",
		"device", d.profile.ID, "fallback", stopQuietFallbackTimeout)

	// 问题电脑兜底：readLoop 在静默窗口会阻塞在 conn.Read（SetReadDeadline 失效），
	// 仅靠 stopAcquisitionTimeout（3s）会导致每个 Stop 卡满 3s。此 Go timer
	// 在静默窗口到期后若 readLoop 仍未完成，直接 Close conn 解除阻塞，
	// 使 Stop 在 ~350ms 内完成（连接需重连，符合 ADR-009）。
	// 健康电脑 readLoop 约 160ms 完成，timer 到期时 done 已关闭，不误关连接。
	//
	// 修复（2026-07-31 实机验证）：故障电脑上 conn.Close() 在挂起 Read 时可能
	// 永久阻塞（安全软件 hook winsock）。旧实现先 Close 再 close(quietFallback)，
	// Close 卡死导致 quietFallback 永不关闭，Stop 掉到 3s 超时，且设备侧连接
	// 槽位未被释放（设备仅允许单连接，重连直接 EOF）。现改为先
	// close(quietFallback) 让 Stop 立即返回，再在后台 goroutine 中 FIN+Close：
	// CloseWrite 发送 FIN 让设备释放连接槽位并解除 readLoop 读侧阻塞，
	// Close 随后自然完成。
	//
	// 竞态说明：close(quietFallback) 仅发生在 timer 到期时 done 未关闭（readLoop
	// 尚未完成边界校验）的情况。done 分支内二次检查 quietFallback：若 readLoop
	// 是 350ms 后被 FIN/EOF 杀掉才退出（quietFallback 已关闭），同样走毒化分支，
	// 避免"Stop 返回成功但连接已死"；正常路径为非阻塞 default。
	quietFallback := make(chan struct{})
	quietTimer := time.AfterFunc(stopQuietFallbackTimeout, func() {
		defer func() {
			if r := recover(); r != nil {
				d.emitLog("error", "system", "Stop quiet timer panic recovered",
					fmt.Sprintf("%v\n%s", r, debug.Stack()))
			}
		}()
		select {
		case <-done:
			// readLoop 已完成，连接可复用，不关闭。
			d.emitLog("debug", "system", "Stop quiet timer fired; done already closed", "action=none")
		default:
			// 仅关闭 quietFallback 让 Stop 立即返回；连接的 FIN+Close 由
			// finalizeStopQuietFallbackLocked 在置 stopAbandoned 之后发起，
			// 保证 readLoop 收到 EOF 时 stopAbandoned 必然已设置（无竞态）。
			// 注意：问题电脑安全软件（WinDivert/360）对 RST（SetLinger(0)）敏感
			// 可能原生崩溃，AbortConnection 用普通 CloseWrite+Close（与 p1604 一致）。
			d.emitLog("debug", "system", "Stop quiet timer fired; closing quietFallback", "action=close")
			close(quietFallback)
		}
	})
	defer quietTimer.Stop()

	select {
	case <-done:
		// readLoop 正常消费尾帧 + ACK 后退出，Stop 成功且连接可复用。
		// 但需排除两种不可复用场景：
		//   - readLoop 是被 350ms 兜底杀掉后才退出（故障电脑 FIN/EOF），
		//     此时 quietFallback 已关闭，连接已不可复用，走毒化分支；
		//   - 静默确认窗口遗留了阻塞读 goroutine（deadline 失效机器的
		//     goroutine 窗口到期时发生，frameReader.IsDirty()）：遗留读
		//     会抢走连接上后续事务（配置/再次 Start）的数据，连接不可
		//     安全复用，同样走废弃分支（Stop 仍返回 nil，下次 Start
		//     自动重连）。
		d.emitLog("debug", "system", "Stop wait: readLoop done", "branch=done")
		d.mu.Lock()
		select {
		case <-quietFallback:
			d.emitLog("debug", "system", "Stop wait: quiet fallback fired before done", "branch=done-quietFallback")
			return d.finalizeStopQuietFallbackLocked(conn)
		default:
			if d.conn != conn {
				return fmt.Errorf("stop acquisition lost connection; reconnect required")
			}
			if fr.IsDirty() {
				d.emitLog("info", "system", "Stop completed; connection recycled",
					"leftover quiet-window read goroutine makes conn unsafe to reuse")
				return d.finalizeStopQuietFallbackLocked(conn)
			}
			fr.Reset()
			d.streaming = false
			d.stopping = false
			d.stop = nil
			d.readLoopDone = nil
			d.status.Connection = core.ConnectionConnected
			return nil
		}
	case <-quietFallback:
		// 问题电脑：readLoop 阻塞在 conn.Read（deadline 失效），静默窗口到期由
		// Go timer 直接判定 Stop 完成（不依赖 <-done）。连接由
		// finalizeStopQuietFallbackLocked 置 stopAbandoned 后后台 FIN+Close
		// 释放设备槽位；Stop 返回 nil（优雅停止），下次 Start 自动重连。
		d.emitLog("debug", "system", "Stop wait: quiet fallback fired", "branch=quietFallback")
		d.mu.Lock()
		return d.finalizeStopQuietFallbackLocked(conn)
	case <-time.After(stopAcquisitionTimeout):
		reason := fmt.Sprintf("stop ACK or residual drain not completed within %v; reconnect required", stopAcquisitionTimeout)
		d.emitLog("error", "system", "Stop wait: 3s timeout",
			fmt.Sprintf("branch=timeout connIsNil=%v", d.conn == nil))
		d.invalidateConnection(conn, reason)
		d.mu.Lock()
		return fmt.Errorf("%s", reason)
	}
}

// finalizeStopQuietFallbackLocked 在 Stop 静默窗口超时（deadline 失效的故障
// 电脑）时结束 Stop：设备已收到 @f1 停止推送，连接由 Stop owner 主动废弃，
// 后台 FIN+Close 负责释放设备连接槽位，下次 StartAcquisition 自动重连。
//
// 优雅停止契约（2026-07-31 实机验证，取代旧 "reconnect required" 错误）：
//   - Stop 返回 nil：停止动作本身成功，连接废弃是内部细节，上层无需干预；
//   - 置 stopAbandoned（持锁时）：readLoop 随后因对端 FIN 收到的 EOF 属预期
//     退出，不再触发 onReadLoopExit / "Read loop exited unexpectedly" 告警，
//     UI 不再出现"停止失败"与"设备断开"噪音；
//   - 置 Error 状态 + 清空 LastError：内部状态表示"连接待重连"，adapter 层
//     映射为 Connected（非 Disconnected/非 Acquiring），下次 Start 透明恢复。
//
// 调用方必须已持有 d.mu；返回前重新持锁，匹配调用方（stopAcquisitionLocked）
// 的 defer Unlock。连接由后台 goroutine 的 FIN+Close 负责真正关闭。
func (d *DAQT1603) finalizeStopQuietFallbackLocked(conn net.Conn) error {
	d.stopAbandoned = true
	d.conn = nil
	d.frameReader = nil
	d.acquiring = false
	d.streaming = false
	d.stopping = false
	d.stop = nil
	d.readLoopDone = nil
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionError
	d.status.LastError = ""
	d.mu.Unlock()
	d.emitLog("info", "system", "Stop completed; connection closed by Stop owner (not reusable)",
		"next Start reconnects automatically")
	// stopAbandoned 已置位后才发起 FIN+Close：readLoop 的 EOF 一定晚于
	// stopAbandoned 可见，无竞态。
	if conn != nil {
		go protocol.AbortConnection(conn)
	}
	d.mu.Lock()
	return nil
}

// invalidateConnectionAfterReadLoopTimeout 在 readLoop join 超时后强制废弃连接。
//
// ADR-009 决策 2：连接生命周期所有者超时或取消时必须能调用 conn.Close()，
// 不能仅打 warn 日志。本函数关闭 conn 后 readLoop 主循环的 conn.Read 会
// 返回错误并退出，readLoop defer 会自行处理 onReadLoopExit 回调（仅当
// unexpectedErr != nil 时触发，主动停止场景 readLoop 缓存的 stop 已被
// close，isClosedConnError 分支会直接 return，不调用 onReadLoopExit）。
//
// expectedConn 比较避免误杀重连后的新连接：Stop 兜底（故障电脑 FIN/EOF）
// 或 Disconnect 已置 d.conn=nil，或用户已重连（d.conn=newConn），
// 此时 expectedConn != d.conn，本函数 no-op 返回。
//
// 调用约束：本函数内部自加锁，调用方无需持锁；但若调用方已持锁，调用前
// 必须 d.mu.Unlock() 以避免死锁（本函数 d.mu.Lock() 会阻塞）。
func (d *DAQT1603) invalidateConnectionAfterReadLoopTimeout(expectedConn net.Conn, message string) {
	d.mu.Lock()
	conn := d.conn
	if conn != expectedConn {
		// 连接已被替换（重连）或置 nil（Stop 兜底/Disconnect/其他毒化），
		// 本连接已由其他 owner 处理，跳过，避免误杀新连接。
		d.mu.Unlock()
		return
	}
	d.conn = nil
	d.frameReader = nil
	d.acquiring = false
	d.streaming = false
	d.stopping = false
	d.stop = nil
	// ADR-009 R0-1：清空 readLoopDone，避免 StartAcquisition 入口的 join 检查
	// 残留旧 done channel 导致下次启动误判"readLoop 未退出"而再次废弃连接。
	// 旧 readLoop goroutine（若仍存活）退出时会向已 nil 的 readLoopDone 写入，
	// 但 defer close(readLoopDone) 前会先缓存 done 到局部变量并判 nil，安全。
	d.readLoopDone = nil
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionError
	d.status.LastError = message
	d.mu.Unlock()

	if conn != nil {
		// 问题电脑挂起 Read 时 Close 会永久阻塞（closesocket 等待未完成
		// 重叠读取）：直接调用导致连接永不关闭、设备单连接槽位泄漏，
		// 后续所有连接被设备拒绝（EOF，2026-07-31 实机复现）。
		// AbortConnection（CloseWrite FIN + Close）后台执行 + WatchdogClose
		// 500ms 兜底，与 probe 收尾/Stop 兜底一致。
		go protocol.AbortConnection(conn)
		wdStop := protocol.WatchdogClose(conn, 500*time.Millisecond)
		go func() { _ = wdStop() }()
	}
	d.emitLog("error", "system", "Connection invalidated", message)
}

// invalidateConnection 比较 expectedConn 与当前 d.conn，若匹配则统一毒化：
// 清空 conn/frameReader、置 Error 状态、保存 LastError、Close conn。
// 若不匹配（已被替换或置 nil），no-op 返回 false。
//
// ADR-009 R0-4：命令路径（writeCommandOnly / sendCommand / sendCommandExact）
// 返回 ErrWatchdogTriggered 时调用方需调用本函数统一毒化连接。expectedConn
// 比较避免与并发 readLoop 错误或重连后的新 conn 竞争：
//   - readLoop no-data timer 或 terminal read error 已先一步毒化连接（d.conn=nil），
//     此处 expectedConn != nil != d.conn，no-op。
//   - 用户已重连（d.conn=newConn），此处 expectedConn=oldConn != d.conn，no-op。
//   - 连接仍为 expectedConn，匹配则毒化。
//
// 调用约束：本函数内部自加锁，调用方必须已释放 d.mu（不能持锁调用）。
// expectedConn 由调用方在持锁时捕获，避免本函数持锁期间 d.conn 被替换。
// 调用方持有 d.writeMu 时调用安全（锁顺序 mu -> writeMu 不变，本函数不获取 writeMu）。
func (d *DAQT1603) invalidateConnection(expectedConn net.Conn, reason string) bool {
	d.mu.Lock()
	if d.conn != expectedConn {
		// 连接已被替换（重连）或置 nil（readLoop 毒化/Stop/Disconnect），no-op。
		d.mu.Unlock()
		return false
	}
	conn := d.conn
	d.conn = nil
	d.frameReader = nil
	d.acquiring = false
	d.streaming = false
	d.stopping = false
	d.stop = nil
	d.readLoopDone = nil
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionError
	d.status.LastError = reason
	d.mu.Unlock()

	if conn != nil {
		// 同 invalidateConnectionAfterReadLoopTimeout：挂起 Read 时 Close 阻塞
		// → 连接不关闭 → 设备槽位泄漏。后台 AbortConnection + WatchdogClose。
		go protocol.AbortConnection(conn)
		wdStop := protocol.WatchdogClose(conn, 500*time.Millisecond)
		go func() { _ = wdStop() }()
	}
	d.emitLog("error", "system", "Connection invalidated", reason)
	return true
}

func (d *DAQT1603) SetDataSink(sink core.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQT1603) Status() core.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DAQT1603) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *DAQT1603) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DAQT1603) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DAQT1603) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DAQT1603) GetDaqT1603Config() (core.DaqT1603HardwareConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config, nil
}

func (d *DAQT1603) ApplyDaqT1603Config(cfg core.DaqT1603HardwareConfig) error {
	// 驱动连接时强制开启 HEAD（帧序号），apply 路径保持一致：
	// 不允许调用方传入 ShowSequence=false 把设备切回无序号帧，
	// 否则与连接时的 HEAD=1 协议边界不一致，帧长从 68 变 64 导致解析错位。
	// 设备实际不支持 HEAD 的固件（temp）在 connect 时已回退 ShowSequence=false，
	// 此时保持 false 即可（读取 d.config 实际值）。
	d.mu.RLock()
	actualSeq := d.config.ShowSequence
	d.mu.RUnlock()
	if actualSeq {
		cfg.ShowSequence = true
	}
	d.mu.Lock()
	conn := d.conn
	if conn != nil && d.acquiring {
		d.mu.Unlock()
		return fmt.Errorf("cannot apply DAQ-T-1603 config while acquiring")
	}
	if conn != nil && d.streaming {
		d.mu.Unlock()
		return fmt.Errorf("cannot apply DAQ-T-1603 config while physical stream is active; disconnect first")
	}
	d.mu.Unlock()

	if conn != nil {
		// 停止路径在返回前由唯一 readLoop 消费 @f1 ACK，并排空ACK后迟到的旧流字节，
		// 因此配置命令从确定的协议边界开始。
		// 历史背景：原 drainConnection 通过阻塞 Read + watchdog Close 排空 TCP 缓冲区，
		// 违反 ADR-009 决策 8——空缓冲是正常状态，watchdog 到期只能证明探测无法完成，
		// 不能证明物理连接故障。问题 Windows 电脑 deadline 失效时健康连接会被误杀。
		//
		// ADR-009 finding 4 整改：watchdog 必须在 writeMu.Lock 之前启动，覆盖锁等待阶段。
		// 历史背景：原实现直接 d.writeMu.Lock()，若前序操作（如 stopAcquisitionLocked
		// 的 @f1 Write 或 applyHardwareConfig 内部命令）持 writeMu 阻塞在 Write 上
		// （SetWriteDeadline 失效），ApplyDaqT1603Config 会永久卡在 writeMu.Lock()。
		// watchdog 触发后 close conn 解除前序 Write 阻塞，writeMu 得以释放。
		// 获取锁后立即检查 watchdog 是否触发：触发则跳过 applyHardwareConfig（conn 已死），
		// 直接毒化连接返回错误。
		wdStop := protocol.WatchdogClose(conn, DAQ_T_1603_TIMEOUT)
		d.writeMu.Lock()
		// 锁等待期间 watchdog 可能已触发（conn 已 Close）。检查并走毒化路径，
		// 避免 applyHardwareConfig 在已死 conn 上做无效 I/O。
		wdTriggered := !wdStop()
		var err error
		if !wdTriggered {
			err = d.applyHardwareConfig(conn, cfg)
		}
		d.writeMu.Unlock()
		if wdTriggered {
			// 锁等待期间 watchdog 触发：conn 已死，毒化连接返回错误。
			// expectedConn=conn 由前面持锁时捕获，invalidateConnection 内部比较
			// d.conn != expectedConn 时 no-op（避免与并发重连竞争）。
			d.invalidateConnection(conn, "apply config: writeMu lock wait watchdog triggered; reconnect required")
			return fmt.Errorf("apply config: writeMu lock wait watchdog triggered; reconnect required; %w", protocol.ErrWatchdogTriggered)
		}
		if err != nil {
			// R0-4：applyHardwareConfig 返回 ErrWatchdogTriggered 时统一毒化连接。
			// watchdog 已 Close conn，d.conn 仍指向已死 conn，必须清空。
			// 此处未持有 d.mu / d.writeMu，可安全调用 invalidateConnection。
			// expectedConn=conn 由前面持锁时捕获，避免与并发 readLoop 错误竞争。
			if errors.Is(err, protocol.ErrWatchdogTriggered) {
				d.invalidateConnection(conn, "config apply watchdog triggered; reconnect required")
				return err
			}
			// finding 9 复核修订（High 1）：applyHardwareConfig 中途收到 'E' 拒绝时，
			// 前序命令可能已部分生效（如 @fe BIN 0 已切换设备到 ASCII，后续 @fe TIME 1
			// 被 'E' 拒绝）。设备实际状态与本地 d.config / FrameReader 模式可能不一致。
			// E 本身不毒化连接（业务错误），但必须重新读取设备实际 BIN/TIME/HEAD 状态
			// 并同步到本地 cfg 与 FrameReader，否则后续 readLoop 会按错误模式解析帧。
			//
			// resyncHardwareConfigMode 失败时返回 resync 错误（而非原始 E 错误），
			// 让调用方感知"模式不可恢复"必须重连；成功时返回原始 E 错误但本地已同步。
			resyncErr := d.resyncHardwareConfigMode(conn, &cfg)
			if resyncErr != nil {
				if errors.Is(resyncErr, protocol.ErrWatchdogTriggered) {
					d.invalidateConnection(conn, "config resync watchdog triggered; reconnect required")
				}
				return fmt.Errorf("apply config failed (%v) and resync also failed: %w", err, resyncErr)
			}
			// resync 成功：把设备实际模式同步到本地，仍返回原始错误让调用方感知
			// 配置未完全应用。readLoop 会按设备实际模式解析帧。
			d.mu.Lock()
			d.config = cfg
			d.profile.DaqT1603Config = cfg
			if d.frameReader != nil {
				d.frameReader.SetBinaryMode(cfg.BinaryFormat)
				d.frameReader.SetMetadataMode(cfg.ShowTimestamp)
				d.frameReader.SetSequenceMode(cfg.ShowSequence)
			}
			d.mu.Unlock()
			return err
		}
	}

	d.mu.Lock()
	d.config = cfg
	d.profile.DaqT1603Config = cfg
	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(cfg.BinaryFormat)
		d.frameReader.SetMetadataMode(cfg.ShowTimestamp)
		d.frameReader.SetSequenceMode(cfg.ShowSequence)
	}
	d.mu.Unlock()
	return nil
}

// resyncHardwareConfigMode 在 applyHardwareConfig 中途失败后重新读取设备实际
// BIN / TIME / HEAD 状态，同步到传入的 cfg 指针。
//
// 业务背景（finding 9 复核修订 High 1）：
//   - applyHardwareConfig 按顺序下发 @fe BIN/TIME/HEAD 等命令；
//   - 若前几条成功、后续某条返回 'E'，设备已部分切换（如 BIN 已改但 TIME 未改）；
//   - 本地 d.config 仍保存旧值，FrameReader 模式可能与设备不一致；
//   - 本函数通过 @fd BIN / @fd TIME / @fd HEAD 重新查询并更新 cfg，让调用方
//     在返回错误前把本地状态对齐到设备实际状态。
//
// 容错策略：
//   - 单条 @fd 查询失败（非 watchdog）：记录 warn，保留 cfg 原值（已尽力）；
//   - watchdog 触发：立即返回 ErrWatchdogTriggered，调用方毒化连接；
//   - 查询成功：更新 cfg 对应字段。
//
// 注意：本函数在 writeMu 已释放后调用（applyHardwareConfig 返回后才调用），
// 不存在与 readLoop 的锁竞争；但 @fd 查询仍走 sendCommandExact，依赖跳帧逻辑
// 处理 readLoop 退出时可能残留的压力帧。
func (d *DAQT1603) resyncHardwareConfigMode(conn net.Conn, cfg *core.DaqT1603HardwareConfig) error {
	queries := []struct {
		cmd    string
		target *bool
		name   string
	}{
		{"@fd BIN", &cfg.BinaryFormat, "BIN"},
		{"@fd TIME", &cfg.ShowTimestamp, "TIME"},
		{"@fd HEAD", &cfg.ShowSequence, "HEAD"},
	}
	for _, q := range queries {
		resp, err := d.sendCommandExact(conn, q.cmd, 1)
		if err != nil {
			if errors.Is(err, protocol.ErrWatchdogTriggered) {
				return err
			}
			// 非 watchdog 错误：记录 warn 保留原值，继续查询其他字段。
			d.emitLog("warn", "system", "Resync query failed", fmt.Sprintf("cmd=%s err=%s", q.cmd, err.Error()))
			continue
		}
		val := strings.TrimSpace(resp)
		switch val {
		case "1":
			*q.target = true
		case "0":
			*q.target = false
		default:
			// 非法响应：记录 warn 保留原值，不中止其他字段查询。
			d.emitLog("warn", "system", "Resync query invalid response",
				fmt.Sprintf("cmd=%s resp=%q expected 0/1", q.cmd, val))
		}
	}
	return nil
}

func (d *DAQT1603) readLoop(conn net.Conn, fr *protocol.T1603FrameReader, done chan struct{}) {
	// ADR-009 R0-10：no-data owner 必须独立于 read goroutine 与其 mutex。
	//
	// 历史背景：原实现 readLoop 通过 SetReadDeadline(200ms) 让 ReadFrame 周期返回，
	// 在循环体累计 `time.Since(lastDataAt) > noDataTimeout` 检测无数据。问题 Windows 电脑
	// deadline 失效时循环体不可达，noDataTimeout 永远不会被触发，半开连接无法自行收敛。
	//
	// 整改后：readLoop 入口启动独立 time.AfterFunc timer，到期通过 expected conn 比较
	// 安全 Close 旧连接、置 Error 状态。timer 不依赖 readLoop 循环体执行，即使 Read
	// 永久阻塞也能到期触发。每次收到有效帧调用 Reset 续期；readLoop 退出 defer Stop。
	//
	// 不调用 onReadLoopExit：让 readLoop defer 的 unexpectedErr 路径统一处理通知，
	// 避免同一故障被回调两次。timer Close 后 readLoop 的 Read 返回错误进入 defer，
	// defer 检测 unexpectedErr != nil 调用 invalidateConnectionAfterReadLoopTimeout
	// 统一清理（重置 status=Error、清空 conn/frameReader、调 onReadLoopExit）。
	//
	// expected conn 比较避免与重连后的新 conn 竞争：Stop/Disconnect 已置 d.conn=nil
	// 或重连后 d.conn 是新连接，timer 触发时 d.conn != expectedConn 直接跳过。
	// 快照 noDataTimeout 到局部变量：测试会覆盖全局 noDataTimeout 加速（200ms），
	// t.Cleanup 在测试结束时恢复原值。若 timer 回调内读取全局变量，可能在与
	// t.Cleanup 写入并发时触发 data race。局部变量在 timer 创建时已固化，回调
	// 读取栈上变量无 race 风险。
	noDataTimeoutSnapshot := noDataTimeout
	noDataTimer := time.AfterFunc(noDataTimeoutSnapshot, func() {
		defer func() {
			if r := recover(); r != nil {
				d.emitLog("error", "system", "no-data timer panic recovered",
					fmt.Sprintf("%v\n%s", r, debug.Stack()))
			}
		}()
		// 锁内比较 expected conn，避免与重连后的新 conn 竞争。
		d.mu.Lock()
		currentConn := d.conn
		if currentConn != conn {
			// 连接已被替换（重连）或置 nil（Stop/Disconnect），跳过。
			d.mu.Unlock()
			return
		}
		// 统一毒化：清空 conn/frameReader、置 Error 状态、保存 LastError。
		// 不调 onReadLoopExit，让 readLoop defer 统一通知（避免双重回调）。
		d.conn = nil
		d.frameReader = nil
		d.acquiring = false
		d.stop = nil
		d.readLoopDone = nil
		d.status.Acquiring = false
		d.status.Connection = core.ConnectionError
		d.status.LastError = fmt.Sprintf("no data received for %v", noDataTimeoutSnapshot)
		d.mu.Unlock()

		// 锁外 Close expected conn 解除 readLoop 的 Read 阻塞。
		// 挂起 Read 时 Close 在问题电脑可能阻塞（closesocket 等待未完成
		// 重叠读取），后台 AbortConnection + WatchdogClose 兜底，避免
		// no-data timer 回调自身被卡住。
		if currentConn != nil {
			go protocol.AbortConnection(currentConn)
			wdStop := protocol.WatchdogClose(currentConn, 500*time.Millisecond)
			go func() { _ = wdStop() }()
		}
		d.emitLog("warn", "acquisition", "No data timeout, conn closed by watchdog",
			fmt.Sprintf("duration=%v", noDataTimeoutSnapshot))
	})
	// readLoop 退出时停止 timer，避免 timer 在 readLoop 已退出后误触发。
	// Stop 不等待已 firing 的回调完成，但回调内 expected conn 比较能正确处理
	// readLoop 退出后 timer 才 fire 的场景（此时 d.conn 已被 defer 清理）。
	defer noDataTimer.Stop()

	var unexpectedErr error

	// panic 兜底：任何 readLoop 内部 panic（含其调用的 onReadLoopExit 回调）
	// 都记录到日志并吞掉，避免 Wails GUI 下进程直接闪退且无法诊断。
	defer func() {
		if r := recover(); r != nil {
			d.emitLog("error", "system", "readLoop panic recovered",
				fmt.Sprintf("%v\n%s", r, debug.Stack()))
		}
	}()

	defer func() {
		if unexpectedErr != nil {
			// 优雅停止契约：Stop 静默兜底已置 stopAbandoned（连接由 Stop owner
			// 主动废弃，FIN 由 finalize 在置位后才发出），readLoop 随后收到的
			// EOF 是预期退出——不再发 @f1、不再 invalidate、不触发
			// onReadLoopExit，避免 UI 出现"停止失败/设备断开"噪音。
			d.mu.RLock()
			abandoned := d.stopAbandoned
			d.mu.RUnlock()
			if abandoned {
				slog.Debug("DAQ-T-1603 read loop exited after Stop owner close",
					"device", d.profile.ID, "error", unexpectedErr)
				if done != nil {
					close(done)
				}
				return
			}
			// ADR-009 finding 4 整改：watchdog 必须在 writeMu.Lock 之前启动，覆盖锁等待阶段。
			// 历史背景：原实现直接 d.writeMu.Lock()，若前序操作（如 ApplyDaqT1603Config
			// 或 stopAcquisitionLocked）持 writeMu 阻塞在 Write 上（SetWriteDeadline 失效），
			// readLoop defer 会永久卡在 writeMu.Lock()，导致 readLoopDone 永不 close，
			// 上层 Stop/Disconnect join 超时也无法收敛。watchdog 触发后 close conn 解除
			// 前序 Write 阻塞，writeMu 得以释放。
			// 锁等待期间 watchdog 触发时跳过 @f1 Write（conn 已死），仅释放锁继续走毒化路径。
			var wdStop func() bool
			if conn != nil {
				wdStop = protocol.WatchdogClose(conn, 1*time.Second)
			}
			d.writeMu.Lock()
			wdTriggered := wdStop != nil && !wdStop()
			if !wdTriggered && conn != nil {
				// 发 @f1 确保设备停止推送，连接已断开时属预期（与 stopAcquisitionLocked 一致）。
				// emitLog "Send command @f1" + 成败分支让 readLoop 退出路径可观测：
				// 否则操作员只看到 "Read loop exited unexpectedly" 无法判断设备侧是否真停了。
				//
				// watchdog 兜底：ADR-009——readLoop 异常退出路径同样需要 Close 兜底，
				// 避免 @f1 Write 在 deadline 失效时永久阻塞导致 defer 无法完成。
				// 触发后 conn 失效，本函数后续不再使用 conn，仅清理状态。
				d.emitLog("debug", "hardware-send", "Send command", "@f1")
				_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
				if _, err := conn.Write([]byte("@f1")); err != nil {
					if isClosedConnError(err) {
						d.emitLog("debug", "hardware-send", "Stop command skipped (conn closed)", err.Error())
					} else {
						d.emitLog("warn", "hardware-send", "Stop command write failed", err.Error())
					}
				}
				_ = conn.SetWriteDeadline(time.Time{})
				if !wdStop() {
					d.emitLog("warn", "hardware-send", "Stop command write watchdog triggered",
						"conn closed by watchdog; reconnect required")
				}
			}
			d.writeMu.Unlock()

			// ADR-009 R0-11：terminal read error 必须调用 invalidateConnectionAfterReadLoopTimeout
			// 统一毒化连接：清空 conn/frameReader、close conn、置 Error 状态、保存 LastError。
			// 历史背景：原实现仅把 status 从 Acquiring 改回 Connected，未清空 conn/frameReader，
			// 也未 close conn。EOF/RST 后连接已死，下次 StartAcquisition 会用旧 conn 发命令爆
			// WSAECONNABORTED。
			//
			// 调用约束：本函数内部自加锁，调用前不持锁。@f1 Write 已在锁外完成。
			d.emitLog("warn", "system", "Read loop exited unexpectedly", unexpectedErr.Error())
			d.invalidateConnectionAfterReadLoopTimeout(conn, unexpectedErr.Error())

			// onReadLoopExit 由本 defer 显式调用（invalidate 不触发该回调）。
			// 在 invalidate 之后调用：调用方读取到的 status.Connection 已是 Error，
			// 符合 ADR-009 决策。
			d.mu.Lock()
			fn := d.onReadLoopExit
			d.mu.Unlock()
			if fn != nil {
				fn(unexpectedErr)
			}
		}

		if done != nil {
			close(done)
		}
	}()

	for {
		d.mu.RLock()
		stop := d.stop
		d.mu.RUnlock()
		if stop == nil {
			return
		}

		select {
		case <-stop:
			return
		default:
			// SetReadDeadline 仍保留作为单次 Read 的软超时，让循环体能周期性
			// 重新检查 stop channel（ADR-009 R0-10：no-data 检测由独立 timer 负责，
			// deadline 失效场景由 timer 兜底 Close conn）。
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			payload, err := fr.ReadFrame()
			if err != nil {
				if errors.Is(err, protocol.ErrControlACK) {
					d.mu.RLock()
					stopping := d.stopping
					d.mu.RUnlock()
					if stopping && !fr.HasPendingControlACK() {
						// 协议契约（2026-07-31 实测验证，详见 docs/audits/
						// 2026-07-30-daq-t1603-acquisition-control-hardware-report.zh-CN.md 第11章）：
						// Stop 响应 = N 个完整合法帧 + 单字节 'A' ACK，ACK 是事务终止边界。
						// TCP 同一连接保证字节有序，ACK 前的字节不会重排到 ACK 后。
						// FrameReader 已在静默窗口后验证完整 N×frameSize+ACK 边界，
						// 因此此处可安全完成 Stop。
						// 边界错乱由 isResyncableReadError 的 Stop 上下文分支失效连接兜底。
						return
					}
					continue
				}
				if errors.Is(err, protocol.ErrIncompleteFrame) {
					continue
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// deadline 软超时：循环体重新检查 stop channel，不视为异常。
					// no-data 检测由独立 noDataTimer 负责，不依赖循环体执行。
					continue
				}
				if isClosedConnError(err) {
					d.mu.RLock()
					stopping := d.stopping
					d.mu.RUnlock()
					if stopping {
						return
					}
				}
				if isResyncableReadError(err) {
					d.mu.RLock()
					stopping := d.stopping
					d.mu.RUnlock()
					if stopping {
						// Stop 事务期间边界必须严格可信：错帧立即终止并毒化连接，
						// 不走 resync。原因：Stop 期间 acquiring=false，readLoop 只消费
						// 尾帧维持边界，错帧说明边界已错乱，resync 会掩盖问题导致
						// 下次 Start 才失败，增加诊断难度。符合 SKILL.md 第 1021 行
						// "固定边界解析失败时废弃连接"原则。
						unexpectedErr = fmt.Errorf("invalid frame while waiting for Stop ACK: %w", err)
						return
					}
					fr.Reset()
					d.emitLog("warn", "acquisition", "Frame misalignment; resyncing",
						err.Error())
					continue
				}
				d.mu.Lock()
				d.readErrors++
				d.mu.Unlock()
				slog.Debug("DAQ-T-1603 read error", "device", d.profile.ID, "error", err)
				// 优雅停止后（stopAbandoned）的 EOF 是 Stop owner 主动废弃连接的
				// 预期结果，降级为 debug 日志；其他 read error 保持 error 级别。
				d.mu.RLock()
				abandoned := d.stopAbandoned
				d.mu.RUnlock()
				if abandoned {
					d.emitLog("debug", "acquisition", "Read loop exited after Stop owner close", err.Error())
				} else {
					d.emitLog("error", "acquisition", "Read loop error", err.Error())
				}
				unexpectedErr = err
				return
			}
			if len(payload) > 0 {
				// 收到有效帧，续期 no-data timer。
				// Reset 是原子操作，无需加锁；即使 timer 已 fire 也能安全 Reset（time.AfterFunc 文档保证）。
				noDataTimer.Reset(noDataTimeout)
				d.mu.RLock()
				acquiring := d.acquiring
				d.mu.RUnlock()
				if acquiring {
					d.processPayload(payload)
				}
			}
		}
	}
}

func (d *DAQT1603) processPayload(data []byte) {
	d.mu.RLock()
	sink := d.sink
	fr := d.frameReader
	d.mu.RUnlock()

	if sink == nil || len(data) < 8 {
		return
	}

	result, err := protocol.ParseTCPFrameEx(data)
	if err != nil {
		d.mu.Lock()
		d.frameErrors++
		d.consecutiveFrameErrors++
		consecutive := d.consecutiveFrameErrors
		d.mu.Unlock()

		slog.Debug("DAQ-T-1603 frame parse error", "device", d.profile.ID, "n", len(data), "error", err)
		d.emitLog("warn", "acquisition", "Frame parse error", fmt.Sprintf("%s raw=% X", err.Error(), data[:min(len(data), 16)]))

		// 快照 fr（持锁读取）：noDataTimer 回调会置 d.frameReader=nil，
		// 无锁读 d.frameReader 与该写入构成 data race。
		if consecutive >= maxConsecutiveFrameErrors && fr != nil {
			slog.Warn("DAQ-T-1603 auto-resync triggered", "device", d.profile.ID, "consecutiveErrors", consecutive)
			d.emitLog("warn", "acquisition", "Auto-resync triggered",
				fmt.Sprintf("consecutive=%d, skipping 1 byte to re-align", consecutive))
			fr.Resync()
			d.mu.Lock()
			d.consecutiveFrameErrors = 0
			d.mu.Unlock()
		}
		return
	}

	d.mu.Lock()
	d.consecutiveFrameErrors = 0
	d.mu.Unlock()

	indices := make([]int, len(result.Temperatures))
	for i := range result.Temperatures {
		indices[i] = i
	}

	sink(core.DataPayload{
		DeviceID:          d.profile.ID,
		Timestamp:         core.NowMs(),
		HardwareTimestamp: result.HardwareTimestamp,
		SequenceNumber:    result.SequenceNumber,
		Channels:          result.Temperatures,
		ChannelIndices:    indices,
	})
}

func (d *DAQT1603) syncHardwareConfigLocked(conn net.Conn) error {
	startedAt := time.Now()
	deadline := startedAt.Add(configSyncTotalTimeout)
	if conn == nil {
		return fmt.Errorf("device not connected")
	}

	d.emitLog("info", "system", "Starting config sync", "mode=skill-compatible")

	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	cfg, err := d.readAllConfig(conn, deadline)
	if err != nil {
		d.emitLog("warn", "system", "Config sync aborted", err.Error())
		return err
	}

	// 强制二进制帧（BIN 1）。TIME 前缀按用户配置（d.config.ShowTimestamp）决定：
	// 启用时设备发 72 字节带时间戳帧，FrameReader 的 metadata 模式在下方同步开启；
	// 禁用时退回 64 字节纯二进制帧（帧读取最稳，默认行为）。
	// HEAD（帧序号前缀）强制开启：每帧携带连续帧序号，可用于检测丢帧/错帧，
	// 同时提高帧边界自校验能力（2026-08-03 实机验证，见 docs/audits/）。
	// 开启 HEAD 后二进制帧为 68 字节（HEAD+TIME 时为 76 字节）。
	// 三条命令必须全部成功，否则 FrameReader 与设备模式不一致。
	// 每条命令前检查总 deadline，避免单条命令跨过预算导致总耗时超出。
	if err := checkConfigSyncDeadline(deadline); err != nil {
		d.emitLog("warn", "system", "Config sync deadline exceeded before BIN", err.Error())
		return err
	}
	if _, err := d.sendCommand(conn, "@fe BIN 1"); err != nil {
		d.emitLog("warn", "system", "Force BIN mode failed", err.Error())
		return fmt.Errorf("force BIN mode: %w", err)
	}
	// temp 型号固件兜底（device-lab/skills/daq-t1603/SKILL.md:494、992）：
	// temp 固件对 @fe BIN 1 仍回 ACK 'A' 但实际不切换二进制模式，@fd BIN 始终返回 0。
	// 若仅依赖 ACK 判定成功，FrameReader 会按 64 字节二进制解析设备实际发送的 ASCII 帧，
	// 导致帧错位。整改后：发送 @fe BIN 1 后**重新查询 @fd BIN**，以读回值作为最终事实。
	//   - 读回 "1"：启用二进制模式（默认期望路径）；
	//   - 读回 "0"：保持 ASCII 模式，记录 warn 让用户感知到 temp 固件限制；
	//   - 读失败或读回非法值：**中止同步**（finding 9 复核修订）。
	//
	// finding 9 复核修订（Medium 3）：
	//   旧实现对非 watchdog 错误记录 warn 后假定 BIN=1 继续同步，重新引入"ACK 不代表
	//   生效"的同类风险。本查询的存在意义就是验证配置生效，验证失败时猜测模式等同于
	//   放弃验证。修订后任何验证失败都中止同步，让调用方感知错误并决定是否重连。
	//   watchdog 触发由调用方按 ADR-009 毒化连接，本函数返回 ErrWatchdogTriggered。
	binaryActual, binVerifyErr := d.queryBinaryMode(conn, deadline)
	if binVerifyErr != nil {
		if errors.Is(binVerifyErr, protocol.ErrWatchdogTriggered) {
			return fmt.Errorf("verify BIN mode: %w", binVerifyErr)
		}
		// 非 watchdog 错误（含非法响应）：中止同步，不再猜测 BIN=1。
		d.emitLog("error", "system", "BIN mode verify failed, abort config sync", binVerifyErr.Error())
		return fmt.Errorf("verify BIN mode: %w", binVerifyErr)
	}
	if binaryActual {
		cfg.BinaryFormat = true
	} else {
		// temp 固件或不支持 BIN 的设备：回退 ASCII，FrameReader 按定长 192 字节解析。
		d.emitLog("warn", "system", "BIN=1 not effective (temp firmware?), fallback to ASCII",
			"@fd BIN returned 0 after @fe BIN 1")
		cfg.BinaryFormat = false
	}
	if err := checkConfigSyncDeadline(deadline); err != nil {
		d.emitLog("warn", "system", "Config sync deadline exceeded before TIME", err.Error())
		return err
	}
	if _, err := d.sendCommand(conn, fmt.Sprintf("@fe TIME %d", boolFlag(d.config.ShowTimestamp))); err != nil {
		d.emitLog("warn", "system", "Set TIME mode failed", err.Error())
		return fmt.Errorf("set TIME mode: %w", err)
	}
	if err := checkConfigSyncDeadline(deadline); err != nil {
		d.emitLog("warn", "system", "Config sync deadline exceeded before HEAD", err.Error())
		return err
	}
	if _, err := d.sendCommand(conn, "@fe HEAD 1"); err != nil {
		d.emitLog("warn", "system", "Force HEAD=1 failed", err.Error())
		return fmt.Errorf("force HEAD=1: %w", err)
	}
	// 读取回 HEAD 验证生效（与 BIN 验证同策略：ACK 不代表生效）。
	if err := checkConfigSyncDeadline(deadline); err != nil {
		d.emitLog("warn", "system", "Config sync deadline exceeded before HEAD verify", err.Error())
		return err
	}
	headResp, err := d.sendCommandExact(conn, "@fd HEAD", 1)
	if err != nil {
		d.emitLog("warn", "system", "HEAD verify query failed", err.Error())
		return fmt.Errorf("verify HEAD: %w", err)
	}
	headEffective := strings.TrimSpace(headResp) == "1"
	if !headEffective {
		// HEAD=1 未生效（temp 固件或设备忽略 @fe HEAD 1）：真正回退到无序号帧，
		// 否则 FrameReader 按 68 字节序号帧解析设备实际发送的 64 字节帧会错位。
		d.emitLog("warn", "system", "HEAD=1 not effective, fallback to no-sequence",
			fmt.Sprintf("@fd HEAD returned %q after @fe HEAD 1", strings.TrimSpace(headResp)))
	}
	cfg.ShowTimestamp = d.config.ShowTimestamp
	cfg.ShowSequence = headEffective

	d.config = *cfg
	d.profile.DaqT1603Config = *cfg
	fr := d.frameReader
	if fr != nil {
		fr.SetBinaryMode(cfg.BinaryFormat)
		fr.SetMetadataMode(cfg.ShowTimestamp)
		fr.SetSequenceMode(cfg.ShowSequence)
	}

	// 通知上层适配器镜像硬件实际配置（采样率、通道掩码、热电偶类型等）。
	// 注意：回调在 d.mu 持有期间触发，调用方（适配器 Connect）必须已释放自身锁，
	// 否则回调重入适配器锁会自死锁（见 daq-t1603 / wind-daq 适配器均在 dev.Connect 前
	// 释放 a.mu）。
	if fn := d.onConfigSynced; fn != nil {
		fn(*cfg)
	}

	slog.Info("DAQ-T-1603 config sync completed", "device", d.profile.ID)
	d.emitLog("info", "system", "Config sync completed", fmt.Sprintf("duration=%s mode=skill-compatible", time.Since(startedAt)))
	return nil
}

// checkConfigSyncDeadline 检查 config-sync 总预算是否已耗尽。
// 在每条 @fd/@fe 命令前调用，剩余时间不足 configSyncMinRemaining 即返回 deadline 错误，
// 让 Connect fail-fast 而不是踩穿 UI 调用超时。
func checkConfigSyncDeadline(deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining <= configSyncMinRemaining {
		return fmt.Errorf("config sync deadline exceeded (remaining=%s, budget=%s)", remaining, configSyncTotalTimeout)
	}
	return nil
}

// queryBinaryMode 发送 @fd BIN 查询当前二进制模式状态。
// 仅在 syncHardwareConfigLocked 中被调用，用于 @fe BIN 1 后验证配置是否实际生效，
// 识别 temp 型号固件"ACK 但不切换"的行为。
//
// 严格校验语义（finding 9 复核修订）：
//   - "1"：设备处于 BIN=1（二进制模式）；
//   - "0"：设备处于 BIN=0（ASCII 模式，含 temp 固件或不支持 BIN 的型号）；
//   - 其他任何响应（E、X、空字符串、多字节等）：协议错误，返回非 nil err，
//     调用方应中止同步而非猜测模式。
//
// 历史问题：旧实现 `strings.TrimSpace(resp) == "1"` 把所有非 "1" 响应（包括
// 'E'、'X' 等非法字节）都当成合法 "0" 回退 ASCII，掩盖了协议错位。
//
// 返回值：
//   - (true, nil)：设备处于 BIN=1；
//   - (false, nil)：设备处于 BIN=0；
//   - (_, err)：查询失败或响应非法；ErrWatchdogTriggered 表示协议边界不可信需毒化连接，
//     其他错误由调用方决定是否容忍（finding 9 修订：非 watchdog 错误应中止同步）。
func (d *DAQT1603) queryBinaryMode(conn net.Conn, deadline time.Time) (bool, error) {
	if err := checkConfigSyncDeadline(deadline); err != nil {
		return false, err
	}
	resp, err := d.sendCommandExact(conn, "@fd BIN", 1)
	if err != nil {
		return false, err
	}
	// 严格校验：只接受 "1" 或 "0"，其他响应视为协议错误。
	// 旧实现把 E/X 等非法字节当成 "0" 回退 ASCII，会掩盖协议错位。
	switch strings.TrimSpace(resp) {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("@fd BIN: invalid response %q (expected \"0\" or \"1\")", resp)
	}
}

// readAllConfig 查询具有固定响应长度的硬件配置。单条查询失败时记 warn 并保留
// 已保存配置后继续。SPS/AVG/TNUM 的响应无分隔符且长度可变，无法在 Windows
// deadline 失效时安全判定结束，因此连接阶段不查询这三个字段。
// 不因某条辅助查询失败而整体放弃连接——
// 这是与同步化之前一致的容错语义。两类错误会硬失败：
//   - 总预算耗尽（checkConfigSyncDeadline）：用于对完全不响应的设备 fail-fast
//   - watchdog 触发（ErrWatchdogTriggered）：conn 已被强制 Close，后续命令必失败，
//     继续循环只会浪费 0ms（后续命令立即失败），但仍走完 10 条命令的循环开销，
//     且掩盖真正的失败原因（用户看到 "force BIN mode: use of closed network connection"
//     而非 "config sync aborted: watchdog triggered"）
//
// BIN/TIME/HEAD 的读回值随后会被 syncHardwareConfigLocked 的 @fe 强制命令覆盖，
// 因此其读失败无害（前提：失败原因不是 watchdog 触发）。
func (d *DAQT1603) readAllConfig(conn net.Conn, deadline time.Time) (*core.DaqT1603HardwareConfig, error) {
	cfg := d.config
	if cfg.ChannelMask == "" {
		cfg.ChannelMask = "FFFF"
	}
	if cfg.SamplingRate <= 0 {
		cfg.SamplingRate = 10
	}
	if cfg.AverageCount <= 0 {
		cfg.AverageCount = 1
	}
	if cfg.ThermocoupleTypes == "" {
		cfg.ThermocoupleTypes = "KKKKKKKKKKKKKKKK"
	}

	// @e3 / @fd BIN/TIME/HEAD 使用定长读，单独处理。
	readExact := func(cmd string, n int, fn func(resp string)) error {
		if err := checkConfigSyncDeadline(deadline); err != nil {
			return err
		}
		startedAt := time.Now()
		resp, err := d.sendCommandExact(conn, cmd, n)
		if err != nil {
			d.logConfigQuery(cmd, startedAt, "", err)
			// watchdog 触发：conn 已被强制 Close，后续命令必失败，立即返回错误。
			if errors.Is(err, protocol.ErrWatchdogTriggered) {
				return err
			}
			return nil
		}
		d.logConfigQuery(cmd, startedAt, resp, nil)
		fn(resp)
		return nil
	}

	// 实机响应为 16 字节类型数据加单个 LF，不含 CR。
	if err := readExact("@e3", 17, func(resp string) {
		value := strings.TrimSuffix(resp, "\n")
		if len(value) == 16 {
			cfg.ThermocoupleTypes = value
		}
	}); err != nil {
		return nil, err
	}

	if err := readExact("@fd MCH", 4, func(resp string) {
		if len(resp) == 4 {
			cfg.ChannelMask = strings.TrimSpace(resp)
		}
	}); err != nil {
		return nil, err
	}

	// @fd BIN / TIME / HEAD：读回值随后被 @fe 强制命令覆盖，读失败无害
	if err := readExact("@fd BIN", 1, func(resp string) {
		cfg.BinaryFormat = strings.TrimSpace(resp) == "1"
	}); err != nil {
		return nil, err
	}
	if err := readExact("@fd TIME", 1, func(resp string) {
		cfg.ShowTimestamp = strings.TrimSpace(resp) == "1"
	}); err != nil {
		return nil, err
	}
	if err := readExact("@fd HEAD", 1, func(resp string) {
		cfg.ShowSequence = strings.TrimSpace(resp) == "1"
	}); err != nil {
		return nil, err
	}

	if err := readExact("@fd TYPE", 1, func(resp string) {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerMode = v
		}
	}); err != nil {
		return nil, err
	}
	if err := readExact("@fd TRIG", 1, func(resp string) {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerEdge = v
		}
	}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (d *DAQT1603) logConfigQuery(cmd string, startedAt time.Time, value string, err error) {
	if err != nil {
		d.emitLog("warn", "system", "Config query failed", fmt.Sprintf("cmd=%s duration=%s error=%s", cmd, time.Since(startedAt), err.Error()))
		return
	}
	d.emitLog("debug", "system", "Config query completed", fmt.Sprintf("cmd=%s duration=%s value=%s", cmd, time.Since(startedAt), strings.TrimSpace(value)))
}

func (d *DAQT1603) applyHardwareConfig(conn net.Conn, cfg core.DaqT1603HardwareConfig) error {
	if cfg.ThermocoupleTypes != "" {
		if len(cfg.ThermocoupleTypes) != 16 {
			return fmt.Errorf("thermocoupleTypes must be 16 characters")
		}
		if _, err := d.sendCommand(conn, "@f3 0"+cfg.ThermocoupleTypes+"0"); err != nil {
			return err
		}
	}

	commands := []string{
		fmt.Sprintf("@fe BIN %d", boolFlag(cfg.BinaryFormat)),
		fmt.Sprintf("@fe TIME %d", boolFlag(cfg.ShowTimestamp)),
		fmt.Sprintf("@fe HEAD %d", boolFlag(cfg.ShowSequence)),
	}
	if cfg.SamplingRate > 0 {
		commands = append(commands, fmt.Sprintf("@fe SPS %d", cfg.SamplingRate))
	}
	if cfg.AverageCount > 0 {
		commands = append(commands, fmt.Sprintf("@fe AVG %d", cfg.AverageCount))
	}
	commands = append(commands,
		fmt.Sprintf("@fe TYPE %d", cfg.TriggerMode),
		fmt.Sprintf("@fe TRIG %d", cfg.TriggerEdge),
	)
	if cfg.TriggerCount > 0 {
		commands = append(commands, fmt.Sprintf("@fe TNUM %d", cfg.TriggerCount))
	}

	for _, cmd := range commands {
		if _, err := d.sendCommand(conn, cmd); err != nil {
			return err
		}
	}
	return nil
}

func boolFlag(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isClosedConnError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "use of closed network connection")
}

func isResyncableReadError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "invalid frame at established")
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
