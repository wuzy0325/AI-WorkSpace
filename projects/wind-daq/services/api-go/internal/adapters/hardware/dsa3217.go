package hardware

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"shared.local/device-sdk/go/pkg/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	DSA3217_DEFAULT_HOST = "192.168.3.7"
	DSA3217_DEFAULT_PORT = 23
	DSA3217_TIMEOUT      = 5 * time.Second
	// DSA3217_CMD_TIMEOUT 是命令收发的总超时（Write + Read 之和）。
	//
	// 历史语义：原代码 SetWriteDeadline(DSA3217_TIMEOUT) + SetReadDeadline(DSA3217_TIMEOUT)
	// 各自独立，Write 和 Read 最坏各 5s，总耗时上限 10s。
	// 加 watchdog 后若用 DSA3217_TIMEOUT 会缩为 5s，引入总超时回归。
	// 此常量保持原 2 倍行为，watchdog 在超时后强制 Close conn 兜底。
	DSA3217_CMD_TIMEOUT = 2 * DSA3217_TIMEOUT
	// DSA3217_READ_LOOP_WATCHDOG_TIMEOUT 是 readLoop 内 ReadString 阻塞的兜底超时。
	//
	// 设计依据 ADR-009：readLoop 持 ioMu 期间阻塞 ReadString，若 SetReadDeadline(200ms)
	// 在故障 Windows 电脑上失效，ReadString 会无限阻塞，导致 sendCommand 永久拿不到 ioMu。
	// watchdog 在该超时后强制 Close conn，解除 ReadString 阻塞并释放 ioMu。
	//
	// 取值 2 * DSA3217_CMD_TIMEOUT（20s）：远大于正常 200ms deadline，仅在 deadline
	// 失效的极端情况下触发，避免误杀正常的慢响应设备。StopAcquisition/Disconnect
	// 不会等 watchdog 自然触发，而是通过 ReadLoopJoinTimeout（1s）超时主动 Close conn。
	DSA3217_READ_LOOP_WATCHDOG_TIMEOUT = 2 * DSA3217_CMD_TIMEOUT
)

type DSA3217 struct {
	mu         sync.RWMutex
	ioMu       sync.Mutex
	profile    device.Profile
	status     device.Status
	sink       device.DataSink
	stop       chan struct{}
	acquiring  bool
	scanning   bool
	conn       net.Conn
	reader     *bufio.Reader
	lineBuffer string
	onError    func(err error) // 设备异常退出通知回调
	// readLoopDone 由 readLoop 退出时关闭，供 StopAcquisition/Disconnect 等待协程退出，
	// 避免在 readLoop 仍持 ioMu 阻塞时调用 sendCommand 死锁。
	readLoopDone chan struct{}
	// cmdTimeout 是 sendCommand 的 watchdog 超时；零值时回退到 DSA3217_CMD_TIMEOUT。
	// 暴露为字段供测试覆盖（生产代码保持默认 10s，测试可缩短到 100ms 加速用例）。
	cmdTimeout time.Duration
	// cmdSoftTimeout 是 sendCommand 的 SetReadDeadline/SetWriteDeadline 软超时；
	// 零值时回退到 DSA3217_TIMEOUT 常量（5s）。
	//
	// ADR-009 R0-12：暴露为字段供测试注入短超时（如 50ms），配合 cmdTimeout=200ms
	// 精确测试"soft deadline 先于 watchdog 触发"场景——net.Pipe 的 SetReadDeadline
	// 在普通 Windows 环境下正常兑现，soft deadline 到期后 ReadString 返回 timeout
	// 错误但 conn 仍开放，验证 sendCommand 是否调 invalidateConnection 毒化连接。
	cmdSoftTimeout time.Duration
	// readLoopWatchdog 是 readLoop 内 ReadString 阻塞的 watchdog 超时；零值时回退到
	// DSA3217_READ_LOOP_WATCHDOG_TIMEOUT。同样供测试覆盖。
	readLoopWatchdog time.Duration
}

func NewDSA3217(profile device.Profile) *DSA3217 {
	return &DSA3217{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
		lineBuffer: "",
	}
}

// effectiveCmdTimeout 返回 sendCommand 应使用的 watchdog 超时：
// 字段非零则用字段值，否则回退到 DSA3217_CMD_TIMEOUT 常量。
func (d *DSA3217) effectiveCmdTimeout() time.Duration {
	if d.cmdTimeout > 0 {
		return d.cmdTimeout
	}
	return DSA3217_CMD_TIMEOUT
}

// effectiveCmdSoftTimeout 返回 sendCommand 应使用的 SetReadDeadline/SetWriteDeadline 软超时：
// 字段非零则用字段值，否则回退到 DSA3217_TIMEOUT 常量（5s）。
//
// ADR-009 R0-12：测试注入短超时（如 50ms）时，soft deadline 先于 watchdog 触发，
// 验证 sendCommand 在 soft timeout 路径调 invalidateConnection 毒化连接。
func (d *DSA3217) effectiveCmdSoftTimeout() time.Duration {
	if d.cmdSoftTimeout > 0 {
		return d.cmdSoftTimeout
	}
	return DSA3217_TIMEOUT
}

// effectiveReadLoopWatchdog 返回 readLoop 应使用的 watchdog 超时：
// 字段非零则用字段值，否则回退到 DSA3217_READ_LOOP_WATCHDOG_TIMEOUT 常量。
func (d *DSA3217) effectiveReadLoopWatchdog() time.Duration {
	if d.readLoopWatchdog > 0 {
		return d.readLoopWatchdog
	}
	return DSA3217_READ_LOOP_WATCHDOG_TIMEOUT
}

// 编译时接口检查
var _ ports.Device = (*DSA3217)(nil)
var _ ports.TareConfigurable = (*DSA3217)(nil)
var _ ports.DSA3217Configurable = (*DSA3217)(nil)
var _ ports.ErrorNotifiable = (*DSA3217)(nil)

// SetOnError 设置设备异常退出回调，实现 ports.ErrorNotifiable 接口
func (d *DSA3217) SetOnError(fn func(err error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
}

func (d *DSA3217) ID() string { return d.profile.ID }

func (d *DSA3217) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = DSA3217_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DSA3217_DEFAULT_PORT
	}

	// ADR-009 R0-7：net.DialTimeout 依赖 Dial 内部 deadline，Windows 故障机器
	// deadline 不可靠时 Dial 可能永远不返回，前端"连接中"状态无法翻转。
	// 改用 sharedproto.DialTCP（goroutine + time.After 软超时 + abandoned 信号），
	// 主线程在 timeout 后立即返回错误，不依赖 Dial 兑现 deadline。
	conn, err := sharedproto.DialTCP(fmt.Sprintf("%s:%d", host, port), "", DSA3217_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	slog.Info("DSA3217 TCP connected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "address", host, "port", port)

	d.conn = conn
	d.reader = bufio.NewReader(conn)
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *DSA3217) Disconnect() error {
	// Phase 1：关闭 stop 通道，通知 readLoop 主动退出。
	d.mu.Lock()
	shouldStop, _ := d.stopAcquisitionLocked()
	done := d.readLoopDone
	// ADR-009 finding 2：捕获 expectedConn，join 超时后仅关闭此 conn，
	// 避免与并发 Connect 的新连接误杀。
	expectedConn := d.conn
	d.mu.Unlock()

	// Phase 2：等待 readLoop 退出，超时则强制 Close conn 兜底（ADR-009）。
	// readLoop 可能持 ioMu 阻塞在 ReadString 上（deadline 失效），
	// 此时 sendCommand 无法获取 ioMu，Disconnect 也不能依赖 sendStopCommandIfConnected。
	// 唯一可靠的取消机制是 conn.Close()，由 invalidateConnectionAfterReadLoopTimeout 执行。
	if shouldStop && done != nil {
		select {
		case <-done:
			// readLoop 正常退出，ioMu 已释放。
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			// readLoop 卡死，强制 invalidate 连接（Close conn + 置 Error + 调 onError）。
			d.invalidateConnectionAfterReadLoopTimeout(expectedConn, "read loop did not exit on disconnect; reconnect required")
		}
	}

	// Phase 3：锁内取 conn 引用并置 nil，避免在 mu.Lock 内执行 I/O（Close 在锁外）。
	// 删除 sendStopCommandIfConnected：readLoop 退出后 TCP FIN 足够停止设备扫描，
	// 若 readLoop 卡死则连接已 Close，无法发送 STOP；若强行发送 STOP 会与 readLoop 竞争 ioMu 死锁。
	// ADR-009 R2-2：保留 Phase 2 invalidate 设置的 Error 状态，仅正常关闭时置 Disconnected。
	// 原实现无条件覆盖为 Disconnected，丢失"连接被强制关闭"诊断状态，前端误判为"正常断开"。
	d.mu.Lock()
	conn := d.conn
	d.conn = nil
	d.reader = nil
	if d.status.Connection != device.ConnectionError {
		d.status.Connection = device.ConnectionDisconnected
	}
	d.mu.Unlock()

	// Phase 4：锁外 Close conn，被 watchdog 或 invalidate 关闭过的 conn 再次 Close 无害。
	// 后台 detach：即使 readLoop 已退出，LSP 环境 Close 仍可能阻塞（卡死不阻塞 Disconnect）。
	if conn != nil {
		go sharedproto.AbortConnection(conn)
	}
	slog.Info("DSA3217 TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)
	return nil
}

func (d *DSA3217) StartAcquisition() error {
	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		return nil
	}
	if d.conn == nil {
		d.mu.Unlock()
		return fmt.Errorf("device not connected")
	}

	// 等待上一次 readLoop 完全退出，避免旧 goroutine 与新采集竞争 ioMu 或 conn。
	// 上一次 StopAcquisition 已 close(stop)，但 readLoop 可能因 deadline 失效还在 ReadString 阻塞。
	// 若不等待，新 readLoop 与旧 readLoop 都会争抢 ioMu，导致 sendCommand 死锁。
	if d.readLoopDone != nil {
		done := d.readLoopDone
		// ADR-009 finding 2：捕获 expectedConn，join 超时后仅关闭此 conn，
		// 避免与并发 Disconnect -> Connect 的新连接误杀。
		expectedConn := d.conn
		d.mu.Unlock()
		select {
		case <-done:
			// 旧 readLoop 已退出，继续启动新采集。
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			// 旧 readLoop 卡死，强制 invalidate 连接，调用方需重新 Connect。
			d.invalidateConnectionAfterReadLoopTimeout(expectedConn, "previous read loop did not exit; reconnect required")
			return fmt.Errorf("previous read loop did not exit; reconnect required")
		}
		d.mu.Lock()
	}
	d.mu.Unlock()

	if _, err := d.sendCommand("SCAN"); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.acquiring {
		return nil
	}
	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = device.ConnectionAcquiring
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	stop := d.stop
	d.scanning = true

	go d.readLoop(stop)
	return nil
}

func (d *DSA3217) StopAcquisition() error {
	d.mu.Lock()
	shouldStop, err := d.stopAcquisitionLocked()
	done := d.readLoopDone
	// ADR-009 finding 2：捕获 expectedConn，join 超时后仅关闭此 conn，
	// 避免与并发 Disconnect -> Connect 的新连接误杀。
	expectedConn := d.conn
	d.mu.Unlock()

	if !shouldStop {
		return err
	}

	// 等待 readLoop 退出后再发送 STOP 命令，避免命令与 readLoop 的 ReadString 竞争 ioMu。
	// readLoop 卡死时（持 ioMu 阻塞 ReadString 且 deadline 失效），唯一可靠取消机制是 conn.Close。
	// 由 invalidateConnectionAfterReadLoopTimeout 执行，触发后 readLoop 的 ReadString 立即返回错误释放 ioMu。
	if done != nil {
		select {
		case <-done:
			// readLoop 正常退出，ioMu 已释放，可以安全发送 STOP 命令停止设备扫描。
			d.sendStopCommandIfConnected()
		case <-time.After(sharedproto.ReadLoopJoinTimeout):
			// readLoop 卡死，强制 invalidate 连接。连接已 Close，无法发送 STOP。
			d.invalidateConnectionAfterReadLoopTimeout(expectedConn, "read loop did not exit after stop; reconnect required")
			return fmt.Errorf("read loop did not exit after stop; reconnect required")
		}
	}
	return err
}

func (d *DSA3217) stopAcquisitionLocked() (bool, error) {
	shouldStop := d.acquiring
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.scanning = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == device.ConnectionAcquiring {
		d.status.Connection = device.ConnectionConnected
	}

	return shouldStop, nil
}

func (d *DSA3217) sendStopCommandIfConnected() {
	d.mu.RLock()
	connected := d.conn != nil
	d.mu.RUnlock()
	if connected {
		_, _ = d.sendCommand("STOP")
	}
}

// invalidateConnection 置连接为 Error 状态并通知上层（onError）。
//
// 使用场景：
//   - readLoop watchdog 触发（ReadString 永久阻塞）
//   - StopAcquisition/Disconnect 等待 readLoopDone 超时（readLoop 卡死）
//
// 调用方语义：连接已不可用，必须重新 Connect 才能恢复。Close 在锁外执行，
// 避免在 mu.Lock 内做 I/O 与 readLoop 的 Read 竞争。
//
// ADR-009 finding 2 修复：expectedConn 比较避免误杀新连接。
// 调用方必须在触发故障前捕获 d.conn（通常在 RLock 后取引用），并传入此参数。
// 仅当 d.conn 仍是 expectedConn 时才清空 d.conn/reader 并置 Error 状态；
// 若 d.conn 已被 Disconnect -> Connect 替换为新连接，仅关闭旧 expectedConn，
// 不修改状态、不通知 onError，避免旧命令的 invalidation 误杀新连接。
func (d *DSA3217) invalidateConnection(expectedConn net.Conn, message string) {
	d.mu.Lock()
	currentConn := d.conn
	if currentConn != expectedConn {
		// d.conn 已被替换为新连接（Disconnect -> Connect）或已置 nil，
		// 旧命令的 invalidation 不应误杀新连接。仅关闭旧 expectedConn，不修改状态。
		d.mu.Unlock()
		if expectedConn != nil {
			// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞。
			go sharedproto.AbortConnection(expectedConn)
		}
		return
	}
	d.conn = nil
	d.reader = nil
	d.acquiring = false
	d.scanning = false
	d.stop = nil
	d.status.Acquiring = false
	d.status.Connection = device.ConnectionError
	d.status.LastError = message
	fn := d.onError
	d.mu.Unlock()

	if expectedConn != nil {
		// 后台 detach：LSP 环境挂起 Read 时 Close 可能永久阻塞（同 t1603 模式）。
		go sharedproto.AbortConnection(expectedConn)
	}
	if fn != nil {
		fn(fmt.Errorf("%s", message))
	}
}

// invalidateConnectionAfterReadLoopTimeout 在 StopAcquisition/Disconnect 等待
// readLoop 退出超时时调用：强制 Close conn 解除 readLoop 阻塞，并标记连接为 Error。
//
// 命名与 daq_p1604.go 标杆对齐，便于跨项目 grep 复用同一模式。
//
// ADR-009 finding 2：同样接收 expectedConn，仅在 d.conn 仍是触发故障时的 conn 时
// 才置 Error 状态；若连接已被替换，仅关闭旧 conn 不修改状态。
func (d *DSA3217) invalidateConnectionAfterReadLoopTimeout(expectedConn net.Conn, message string) {
	d.invalidateConnection(expectedConn, message)
}

func (d *DSA3217) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DSA3217) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DSA3217) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *DSA3217) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DSA3217) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DSA3217) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DSA3217) sendCommand(cmd string) (string, error) {
	// 先用 RLock 取 conn 引用，再启 watchdog，最后 ioMu.Lock。
	//
	// 顺序不能反（ADR-009）：若先 ioMu.Lock 再启 watchdog，readLoop 持 ioMu 阻塞
	// ReadString（deadline 失效）时 sendCommand 会卡在 ioMu.Lock 上，watchdog 永远不启动，
	// 形成死锁。watchdog 在 ioMu.Lock 之前启动，覆盖"等 ioMu + Write + Read"全流程，
	// 触发后 Close conn 解除 readLoop 阻塞，readLoop 释放 ioMu 后本函数才能继续。
	d.mu.RLock()
	conn := d.conn
	d.mu.RUnlock()

	if conn == nil {
		return "", fmt.Errorf("not connected")
	}

	// watchdog 兜底（ADR-009）：SetReadDeadline 在某些 Windows 电脑不可靠，
	// ReadString 在 deadline 到期后仍可能无限阻塞。watchdog 在超时后强制 Close conn
	// 解除阻塞，避免 sendCommand 永久卡死。
	//
	// 超时值：用 DSA3217_CMD_TIMEOUT（2 * DSA3217_TIMEOUT）保持原代码
	// Write 5s + Read 5s = 10s 的总超时行为，避免引入回归。
	//
	// wdStop 返回 false 表示 watchdog 已触发，conn 已失效，调用方需重连。
	// WatchdogClose 返回的 stop 函数本身幂等（内部 sync.Once），
	// WrapWatchdogError + defer wdStop() 的多次调用模式安全。
	wdStop := sharedproto.WatchdogClose(conn, d.effectiveCmdTimeout())

	d.ioMu.Lock()
	defer d.ioMu.Unlock()

	// 再次检查 reader：等待 ioMu 期间 Disconnect 可能已置 d.reader=nil。
	d.mu.RLock()
	reader := d.reader
	d.mu.RUnlock()
	if reader == nil {
		_ = wdStop()
		return "", fmt.Errorf("not connected")
	}

	// defer LIFO 顺序：
	//   1. 后注册先执行：wdStop() 停止 watchdog 计时器（幂等，可多次调用）
	//   2. 先注册后执行：清除 deadline，避免过期的绝对时间被 readLoop 继承导致
	//      ReadString 永远立即返回 timeout、CPU 死循环
	// 注意：watchdog 触发后 conn 已 Close，清除 deadline 失败被忽略无害。
	defer func() {
		conn.SetWriteDeadline(time.Time{})
		conn.SetReadDeadline(time.Time{})
	}()
	defer func() { _ = wdStop() }()

	conn.SetWriteDeadline(time.Now().Add(d.effectiveCmdSoftTimeout()))
	// 命令收发用 Info 级别：ring buffer 默认 Info 阈值即可透传，前端 showHardware
	// 开关通过 catFilter 控制可见性；stderr / file sink 由 CategorySkipHandler 跳过，
	// 避免状态查询期间高频命令帧刷屏文件与终端。
	slog.Info("DSA3217 command send", "category", "hardware-send", "component", "hardware", "device", d.profile.ID, "command", cmd)
	if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
		// ADR-009 R0-12：watchdog 触发（wdStop 返回 false）或 soft deadline 兑现
		// （net.Error.Timeout()==true）都意味着协议边界已不可信——迟到响应可能随后
		// 进入 TCP 流被下一命令消费。统一调 invalidateConnection 毒化连接，并返回
		// ErrWatchdogTriggered sentinel 让调用方通过 IsWatchdogTriggered 判定。
		//
		// soft deadline 兑现场景：OS 正常兑现 SetWriteDeadline，Write 在 deadline 到期
		// 后返回 timeout 错误。此时 watchdog 可能尚未触发（wdStop 返回 true），但 conn
		// 的协议边界已不可信——设备可能正在持续重传或乱序响应，迟到字节会污染下一命令。
		// 必须强制 Close conn 阻断迟到响应，不能仅返回 timeout 错误保留 conn。
		if isSoftOrHardTimeout(err, wdStop) {
			d.invalidateConnection(conn, fmt.Sprintf("sendCommand %s write timeout: %v", cmd, err))
			return "", fmt.Errorf("write command: %w; %w", err, sharedproto.ErrWatchdogTriggered)
		}
		return "", fmt.Errorf("write command: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(d.effectiveCmdSoftTimeout()))
	resp, err := reader.ReadString('\n')
	if err != nil {
		// ADR-009 R0-12：Read 路径同 Write——soft deadline 或 watchdog 任一触发都毒化连接。
		// soft deadline 兑现场景：SetReadDeadline 正常工作，ReadString 在 deadline 到期后
		// 返回 timeout 错误但 conn 仍开放。若仅返回普通错误保留 conn，迟到响应会随后到达
		// 被下一命令或 readLoop 消费，导致协议错位（如 LIST S 的迟到响应被 SCAN 命令读取）。
		if isSoftOrHardTimeout(err, wdStop) {
			d.invalidateConnection(conn, fmt.Sprintf("sendCommand %s read timeout: %v", cmd, err))
			return "", fmt.Errorf("read response: %w; %w", err, sharedproto.ErrWatchdogTriggered)
		}
		return "", fmt.Errorf("read response: %w", err)
	}

	trimmed := strings.TrimRight(resp, "\r\n")
	slog.Info("DSA3217 command response", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "response", trimmed)
	return trimmed, nil
}

func (d *DSA3217) SetAvg(value int) error {
	if value < 1 || value > 240 {
		return fmt.Errorf("AVG must be between 1 and 240")
	}
	_, err := d.sendCommand(fmt.Sprintf("SET AVG %d", value))
	return err
}

func (d *DSA3217) SetPeriod(value int) error {
	if value < 73 || value > 65535 {
		return fmt.Errorf("PERIOD must be between 73 and 65535")
	}
	_, err := d.sendCommand(fmt.Sprintf("SET PERIOD %d", value))
	return err
}

func (d *DSA3217) SaveConfig() error {
	_, err := d.sendCommand("SAVE")
	return err
}

func (d *DSA3217) ReadScanConfig() (avg, period int, unit string, err error) {
	resp, err := d.sendCommand("LIST S")
	if err != nil {
		return 0, 0, "", err
	}

	lines := strings.Split(resp, "\r\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SET AVG") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				avg, _ = strconv.Atoi(parts[2])
			}
		} else if strings.HasPrefix(line, "SET PERIOD") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				period, _ = strconv.Atoi(parts[2])
			}
		} else if strings.HasPrefix(line, "SET UNITSCAN") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				unit = parts[2]
			}
		}
	}

	return avg, period, unit, nil
}

// GetDsa3217ScanConfig 实现 ports.DSA3217Configurable 接口
func (d *DSA3217) GetDsa3217ScanConfig() (device.DSA3217ScanConfig, error) {
	avg, period, unit, err := d.ReadScanConfig()
	if err != nil {
		return device.DSA3217ScanConfig{}, err
	}
	fps := "--"
	if avg >= 1 && period >= 73 {
		fps = fmt.Sprintf("%.3f", 1.0/(float64(period)*1e-6*16*float64(avg)))
	}
	return device.DSA3217ScanConfig{
		Avg:    avg,
		Period: period,
		Fps:    fps,
		Unit:   unit,
	}, nil
}

// ApplyDsa3217ScanConfig 实现 ports.DSA3217Configurable 接口，写入后回读验证
func (d *DSA3217) ApplyDsa3217ScanConfig(avg int, period int) (device.DSA3217ScanConfig, error) {
	if err := d.SetAvg(avg); err != nil {
		return device.DSA3217ScanConfig{}, err
	}
	if err := d.SetPeriod(period); err != nil {
		return device.DSA3217ScanConfig{}, err
	}
	if err := d.SaveConfig(); err != nil {
		return device.DSA3217ScanConfig{}, err
	}
	// 写入后回读确认生效
	verify, err := d.GetDsa3217ScanConfig()
	if err != nil {
		return device.DSA3217ScanConfig{}, err
	}
	if avg >= 1 && verify.Avg != avg {
		slog.Warn("DSA3217 AVG 写入验证不匹配", "device", d.profile.ID, "expected", avg, "actual", verify.Avg)
	}
	if period >= 73 && verify.Period != period {
		slog.Warn("DSA3217 PERIOD 写入验证不匹配", "device", d.profile.ID, "expected", period, "actual", verify.Period)
	}
	return verify, nil
}

// isSoftOrHardTimeout 判定 sendCommand 的 Write/Read 错误是否需要毒化连接。
//
// ADR-009 R0-12：两类错误都意味着协议边界已不可信，必须 Close conn 阻断迟到响应：
//   - hard timeout：watchdog 已触发（wdStop 返回 false），conn 已被强制 Close
//   - soft timeout：OS 兑现 SetReadDeadline/SetWriteDeadline（net.Error.Timeout()==true），
//     conn 仍开放但迟到响应可能随后进入 TCP 流被下一命令消费
//
// 非 timeout 错误（如 EOF/RST/broken pipe）由调用方自行判定，本函数返回 false。
// 调用方应继续走 errors.Is(err, ErrWatchdogTriggered) 或 IsConnResetByPeer 等路径
// 处理其他 terminal 错误。
//
// 参数 op 仅用于日志上下文，未在本函数内使用；保留为未来扩展（如区分 write/read 路径）。
func isSoftOrHardTimeout(err error, wdStop func() bool, _ ...string) bool {
	if err == nil {
		return false
	}
	// hard timeout：watchdog 触发，conn 已被强制 Close
	if !wdStop() {
		return true
	}
	// soft timeout：OS 兑现 deadline，net.Error.Timeout() 返回 true
	// bufio.Reader.ReadString 在 Read deadline 到期后返回的错误可能包装 net.Error，
	// 用 errors.As 解包多层包装找到 net.Error 接口实现。
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
