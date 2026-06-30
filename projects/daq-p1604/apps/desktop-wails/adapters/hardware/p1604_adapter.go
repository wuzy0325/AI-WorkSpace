package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"

	"daq-p1604/core"
	"daq-p1604/ports"
)

const (
	p1604DefaultHost    = "192.168.3.101"
	p1604DefaultPort    = 9000
	p1604ConnectTimeout = 5 * time.Second
	p1604CommandTimeout = 2 * time.Second
	p1604ReadTimeout    = 200 * time.Millisecond
	p1604NumChannels    = 18
	// p1604ReadLoopJoinTimeout Disconnect/StopAcquisition 等待 readLoop 退出的最长时间。
	// 超过这个时间不再阻塞调用方，但 readLoop 自身仍会退出（不会泄漏）。
	p1604ReadLoopJoinTimeout = 1 * time.Second
)

// stopReasonUserRequested 调用方主动停止（StopAcquisition / Disconnect）。
// readLoop 看到此原因时静默退出，不修改设备状态、不触发 emitState。
const stopReasonUserRequested = "user-requested"

// DeviceLogEntry 设备日志条目
type DeviceLogEntry struct {
	Level    string
	Category string
	DeviceID string
	Message  string
	Detail   string
}

// P1604Adapter DAQ-P-1604 硬件适配器
type P1604Adapter struct {
	mu        sync.RWMutex
	drivers   map[string]*p1604Driver
	status    map[string]*core.DeviceState
	sinks     map[string]func(core.PressureSnapshot)
	channels  map[string]chan core.PressureSnapshot
	stopChs   map[string]chan struct{}
	logSink   func(DeviceLogEntry)
	stateSink func(id string, state core.DeviceState) // 连接状态变更回调，用于通知前端
}

// p1604Driver 单个 P1604 设备的驱动实例
type p1604Driver struct {
	profile     core.PressureProfile
	conn        net.Conn
	frameReader *sharedproto.FrameReader
	acquiring   bool
	// readLoopDone 由 readLoop 在退出时关闭。
	// Disconnect / StopAcquisition 在 close(stop) 之后等待此 channel，确保
	// readLoop 不再持有 conn 引用，再安全 conn.Close；同时也避免 Disconnect 返回
	// 后老 readLoop 仍在运行、错误清理后续新建立的同 ID 设备状态。
	readLoopDone chan struct{}
	// stopReason 由触发停止的一方在 close(stop) 之前置位（值为 stopReasonUserRequested
	// 表示主动停止）。readLoop 据此区分 "调用方主动停止" 与 "连接意外断开"。
	stopReason     string
	stopReasonLock sync.Mutex
}

// setStopReason 设置主动停止原因。多次设置只保留首个。
func (d *p1604Driver) setStopReason(reason string) {
	d.stopReasonLock.Lock()
	defer d.stopReasonLock.Unlock()
	if d.stopReason == "" {
		d.stopReason = reason
	}
}

// getStopReason 读取主动停止原因（"" 表示未主动停止）。
func (d *p1604Driver) getStopReason() string {
	d.stopReasonLock.Lock()
	defer d.stopReasonLock.Unlock()
	return d.stopReason
}

// NewP1604Adapter 创建 P1604 硬件适配器
func NewP1604Adapter() *P1604Adapter {
	return &P1604Adapter{
		drivers:  make(map[string]*p1604Driver),
		status:   make(map[string]*core.DeviceState),
		sinks:    make(map[string]func(core.PressureSnapshot)),
		channels: make(map[string]chan core.PressureSnapshot),
		stopChs:  make(map[string]chan struct{}),
	}
}

var _ ports.DevicePort = (*P1604Adapter)(nil)

// SetLogSink 设置日志回调
func (a *P1604Adapter) SetLogSink(sink func(DeviceLogEntry)) {
	a.mu.Lock()
	a.logSink = sink
	a.mu.Unlock()
}

// SetStateSink 设置连接状态变更回调
func (a *P1604Adapter) SetStateSink(sink func(id string, state core.DeviceState)) {
	a.mu.Lock()
	a.stateSink = sink
	a.mu.Unlock()
}

func (a *P1604Adapter) emitLog(entry DeviceLogEntry) {
	a.mu.RLock()
	sink := a.logSink
	a.mu.RUnlock()
	if sink != nil {
		sink(entry)
	}
}

// emitState 通知前端设备状态变更
func (a *P1604Adapter) emitState(id string) {
	a.mu.RLock()
	sink := a.stateSink
	st, exists := a.status[id]
	a.mu.RUnlock()
	if sink != nil && exists {
		sink(id, *st)
	}
}

// Connect 连接设备
// 锁策略：仅在读写共享状态时持锁，TCP 拨号和 w1601 命令在锁外执行
func (a *P1604Adapter) Connect(profile core.PressureProfile) error {
	a.mu.Lock()
	if _, exists := a.drivers[profile.ID]; exists {
		a.mu.Unlock()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	a.mu.Unlock()

	host := profile.Address
	if host == "" {
		host = p1604DefaultHost
	}
	port := profile.Port
	if port <= 0 {
		port = p1604DefaultPort
	}

	// TCP 拨号在锁外执行，避免阻塞其他设备操作
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), p1604ConnectTimeout)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	driver := &p1604Driver{
		profile:     profile,
		conn:        conn,
		frameReader: sharedproto.NewFrameReader(conn),
	}

	// 连接后必须先发 w1601 启用长度前缀模式
	if err := driver.sendCommand("w1601"); err != nil {
		conn.Close()
		return fmt.Errorf("enable length prefix: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	a.mu.Lock()
	// 二次检查：拨号期间可能已被其他 goroutine 连接
	if _, exists := a.drivers[profile.ID]; exists {
		a.mu.Unlock()
		conn.Close()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	a.drivers[profile.ID] = driver
	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		StatusText:  core.StatusConnected.String(),
		ConnectedAt: core.TimestampMs(),
	}
	a.mu.Unlock()

	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware", DeviceID: profile.ID,
		Message: "Device connected", Detail: fmt.Sprintf("%s:%d", host, port),
	})
	return nil
}

// Disconnect 断开设备连接
//
// 关闭顺序对竞态修复至关重要：
//  1. 锁内：标记主动停止原因 + close(stop) 通知 readLoop 退出 + 清理共享状态。
//  2. 锁外：等待 readLoop 退出（join），确保它不再持有 driver.conn 引用。
//  3. 锁外：发送停止命令、conn.Close。
//
// 这样可以避免 readLoop 与 conn.Close 并发、以及老 readLoop 在 Disconnect 返回后
// 错误清理新设备状态的问题。
func (a *P1604Adapter) Disconnect(id string) error {
	a.mu.Lock()
	driver, ok := a.drivers[id]
	wasAcquiring := ok && driver != nil && driver.acquiring
	// 在 close(stop) 之前标记主动停止原因，readLoop 看到该原因会静默退出
	if wasAcquiring && driver != nil {
		driver.setStopReason(stopReasonUserRequested)
	}
	if done, exists := a.stopChs[id]; exists {
		close(done)
		delete(a.stopChs, id)
	}
	if ch, exists := a.channels[id]; exists {
		close(ch)
		delete(a.channels, id)
	}
	delete(a.sinks, id)
	if !ok {
		a.mu.Unlock()
		return nil
	}
	delete(a.drivers, id)
	if driver != nil {
		driver.acquiring = false
	}
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusDisconnected)
		st.Error = ""
		st.AcquiringAt = 0
	}
	connected := driver != nil && driver.conn != nil
	a.mu.Unlock()

	// 等待 readLoop 退出后再操作连接，避免 ReadFrame 与 Close 竞争。
	// join 超时仅是兜底，正常情况下 readLoop 在 200ms 读超时内就会观察到 stop。
	if driver != nil && wasAcquiring {
		driver.joinReadLoop(id, p1604ReadLoopJoinTimeout)
	}

	// 在锁外执行 I/O：发送停止命令和关闭连接
	if wasAcquiring && connected && driver != nil {
		if err := driver.sendCommand("c 02 1"); err != nil {
			if isConnectionFault(err) {
				slog.Debug("DAQ-P-1604 stop stream on disconnect: connection already gone", "device", id, "error", err)
			} else {
				slog.Warn("DAQ-P-1604 stop stream on disconnect failed", "device", id, "error", err)
			}
		}
	}
	if driver != nil && driver.conn != nil {
		_ = driver.conn.Close()
	}

	// 通知前端状态变更
	a.emitState(id)
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware", DeviceID: id,
		Message: "Device disconnected",
	})
	return nil
}

// StartAcquisition 启动数据采集
// 锁策略：仅在状态检查和状态更新时持锁，所有 sendCommand 和 Sleep 在锁外执行
func (a *P1604Adapter) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	a.mu.Lock()
	driver, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := a.channels[id]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s already acquiring", id)
	}
	// 预占采集槽位，防止并发重复启动
	ch := make(chan core.PressureSnapshot, 8192)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done
	a.mu.Unlock()

	// 以下命令在锁外执行，避免阻塞其他设备操作
	// 配置数据流参数：c 00 <st> <mask> <sync> <per> <fmt> <mode>
	periodMs := driver.profile.P1604Cfg.SamplingRate
	if periodMs < 10 {
		periodMs = 100 // 默认 100ms
	}
	if err := driver.sendCommand(fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", periodMs)); err != nil {
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 配置流返回内容：0010=压力，0400=设备时间戳，0800=大气压力/温度
	// 掩码 0C10 = 压力 + 设备时间戳 + 大气数据
	if err := driver.sendCommand("c 05 1 0C10"); err != nil {
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("set stream content: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 启动数据流
	if err := driver.sendCommand("c 01 1"); err != nil {
		a.rollbackAcquisition(id, ch, done)
		return nil, fmt.Errorf("start stream: %w", err)
	}

	directSink := func(snapshot core.PressureSnapshot) {
		select {
		case ch <- snapshot:
		case <-done:
		}
	}

	a.mu.Lock()
	a.sinks[id] = directSink
	driver.acquiring = true
	// 重置 stop 状态，准备启动新的 readLoop
	driver.readLoopDone = make(chan struct{})
	driver.stopReasonLock.Lock()
	driver.stopReason = ""
	driver.stopReasonLock.Unlock()
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	a.mu.Unlock()

	// 启动读取循环
	go a.readLoop(id, driver, done)

	return ch, nil
}

// rollbackAcquisition 启动失败时回滚采集状态
// 在锁外执行停止命令，并在锁内清理采集槽位
func (a *P1604Adapter) rollbackAcquisition(id string, ch chan core.PressureSnapshot, done chan struct{}) {
	// 尝试停止可能已部分配置的流（在锁外执行 I/O）。
	// 启动失败常见原因之一就是连接异常，用 isConnectionFault 把这种场景从 warn 降到 debug。
	if err := a.driverSendCommandSafe(id, "c 02 1"); err != nil {
		if isConnectionFault(err) {
			slog.Debug("DAQ-P-1604 rollback stop stream: connection already gone", "device", id, "error", err)
		} else {
			slog.Warn("DAQ-P-1604 rollback stop stream failed", "device", id, "error", err)
		}
	}
	close(done)

	a.mu.Lock()
	delete(a.channels, id)
	delete(a.stopChs, id)
	delete(a.sinks, id)
	a.mu.Unlock()
	close(ch)
}

// driverSendCommandSafe 在锁内安全获取 driver 后发送命令
func (a *P1604Adapter) driverSendCommandSafe(id, cmd string) error {
	a.mu.RLock()
	driver, ok := a.drivers[id]
	a.mu.RUnlock()
	if !ok || driver == nil {
		return fmt.Errorf("device %s not connected", id)
	}
	return driver.sendCommand(cmd)
}

// StopAcquisition 停止数据采集
//
// 与 Disconnect 相同的关闭顺序：
//  1. 锁内：标记主动停止 + close(stop) + 清理共享状态。
//  2. 锁外：等待 readLoop 退出。
//  3. 锁外：发送停止命令（不关 conn，连接保留以便后续重新 StartAcquisition）。
func (a *P1604Adapter) StopAcquisition(id string) error {
	a.mu.Lock()
	driver, ok := a.drivers[id]
	wasAcquiring := ok && driver != nil && driver.acquiring
	if wasAcquiring && driver != nil {
		driver.setStopReason(stopReasonUserRequested)
	}
	if done, exists := a.stopChs[id]; exists {
		close(done)
		delete(a.stopChs, id)
	}
	if ch, exists := a.channels[id]; exists {
		close(ch)
		delete(a.channels, id)
	}
	delete(a.sinks, id)
	connected := ok && driver != nil && driver.conn != nil
	if driver != nil {
		driver.acquiring = false
	}
	if st, exists := a.status[id]; exists {
		if connected {
			st.SetStatus(core.StatusConnected)
		} else {
			st.SetStatus(core.StatusDisconnected)
		}
		st.AcquiringAt = 0
	}
	a.mu.Unlock()

	// 等待 readLoop 退出，避免它和后续命令并发使用同一 conn
	if driver != nil && wasAcquiring {
		driver.joinReadLoop(id, p1604ReadLoopJoinTimeout)
	}

	// 仅在确实在采集且连接有效时，才在锁外发送停止命令
	if wasAcquiring && connected && driver != nil {
		if err := driver.sendCommand("c 02 1"); err != nil {
			if isConnectionFault(err) {
				slog.Debug("DAQ-P-1604 stop stream: connection already gone", "device", id, "error", err)
			} else {
				slog.Warn("DAQ-P-1604 stop stream command failed", "device", id, "error", err)
			}
		}
	}
	return nil
}

// joinReadLoop 等待 readLoop 关闭其 done channel；超时仅日志，不阻塞调用方。
// 调用方通常先 close(stop) + 标记 stopReason，然后调用本方法等待 readLoop 退出，
// 再安全 conn.Close。driver 为 nil 或 readLoop 未启动时直接返回。
func (d *p1604Driver) joinReadLoop(id string, timeout time.Duration) {
	if d == nil || d.readLoopDone == nil {
		return
	}
	select {
	case <-d.readLoopDone:
	case <-time.After(timeout):
		slog.Warn("DAQ-P-1604 readLoop join timeout", "device", id, "timeout", timeout)
	}
}

// Status 获取设备状态
func (a *P1604Adapter) Status(id string) (core.DeviceState, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	st, ok := a.status[id]
	if !ok {
		return core.DeviceState{}, false
	}
	driver, hasDriver := a.drivers[id]
	if hasDriver && driver.acquiring {
		st.SetStatus(core.StatusAcquiring)
	} else if hasDriver {
		st.SetStatus(core.StatusConnected)
	}
	return *st, true
}

// ApplyConfig 应用设备配置（下次 StartAcquisition 时生效）
func (a *P1604Adapter) ApplyConfig(id string, cfg core.P1604Config) error {
	a.mu.Lock()
	if st, exists := a.status[id]; exists {
		st.Profile.P1604Cfg = cfg
	}
	a.mu.Unlock()
	return nil
}

// SetDataSink 设置数据回调
func (a *P1604Adapter) SetDataSink(id string, sink func(core.PressureSnapshot)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sinks[id] = sink
}

// readLoop 读取设备数据帧
//
// 退出路径：
//  1. stop channel 关闭（调用方主动停止）→ 静默退出。
//  2. ReadFrame 返回非 timeout 错误：
//     - 若 driver.stopReason 已置位（调用方刚刚 close(stop) 但 select 尚未轮到）→
//       静默退出，不修改任何共享状态（避免与 StopAcquisition/Disconnect 的清理冲突）。
//     - 否则视为连接意外断开，标记设备为 Error 状态并通知前端。
//
// 退出前必须 close(readLoopDone)，让 Disconnect/StopAcquisition 能 join 到本协程。
func (a *P1604Adapter) readLoop(id string, driver *p1604Driver, stop <-chan struct{}) {
	defer close(driver.readLoopDone)

	for {
		select {
		case <-stop:
			return
		default:
			driver.conn.SetReadDeadline(time.Now().Add(p1604ReadTimeout))
			payload, err := driver.frameReader.ReadFrame()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				// 调用方主动停止时 ReadFrame 会因 conn 关闭/超时返回错误，
				// 此时不应再修改共享状态——清理工作已由 Stop/Disconnect 完成。
				if driver.getStopReason() != "" {
					return
				}
				a.handleConnectionLost(id, driver, err)
				return
			}
			if len(payload) > 0 {
				a.processPayload(id, payload)
			}
		}
	}
}

// handleConnectionLost 处理连接意外断开：清理共享状态并通知前端。
// 仅由 readLoop 在非主动停止场景调用。
func (a *P1604Adapter) handleConnectionLost(id string, driver *p1604Driver, cause error) {
	a.mu.Lock()
	st, exists := a.status[id]
	if !exists || st.Status == core.StatusDisconnected {
		a.mu.Unlock()
		return
	}
	// 校验当前 drivers[id] 仍是本 readLoop 关联的 driver。
	// 若 Disconnect 已删除并启动新 driver，则放弃清理避免误伤新设备。
	if cur, ok := a.drivers[id]; !ok || cur != driver {
		a.mu.Unlock()
		return
	}
	delete(a.sinks, id)
	if done, ok := a.stopChs[id]; ok {
		close(done)
		delete(a.stopChs, id)
	}
	if ch, ok := a.channels[id]; ok {
		close(ch)
		delete(a.channels, id)
	}
	driver.acquiring = false
	st.SetStatus(core.StatusError)
	st.Error = fmt.Sprintf("连接断开: %v", cause)
	st.AcquiringAt = 0
	a.mu.Unlock()

	a.emitState(id)
	level := "error"
	if !isConnectionFault(cause) {
		// 非典型连接故障（解析错误等）降级为 warn
		level = "warn"
	}
	a.emitLog(DeviceLogEntry{
		Level: level, Category: "hardware", DeviceID: id,
		Message: "Connection lost", Detail: cause.Error(),
	})
}

// processPayload 处理接收到的数据帧
func (a *P1604Adapter) processPayload(id string, data []byte) {
	// 区分 ASCII 响应和二进制帧
	if sharedproto.IsASCIIFrame(data) {
		// ASCII 响应（命令确认等），忽略
		return
	}

	// 解析二进制数据帧（含设备时间戳 + 大气数据）
	// 掩码 0C10 = 0010(压力) | 0400(设备时间戳) | 0800(大气数据)
	channels, deviceTimestampMs, err := sharedproto.ParseStreamFrameEx(data, true, true)
	if err != nil {
		a.emitLog(DeviceLogEntry{
			Level: "debug", Category: "hardware", DeviceID: id,
			Message: "Frame parse error", Detail: err.Error(),
		})
		return
	}

	if len(channels) != p1604NumChannels {
		a.emitLog(DeviceLogEntry{
			Level: "warn", Category: "hardware", DeviceID: id,
			Message: "Unexpected channel count", Detail: fmt.Sprintf("expected %d, got %d", p1604NumChannels, len(channels)),
		})
		return
	}

	a.mu.RLock()
	sink := a.sinks[id]
	unit := "psi"
	if st, ok := a.status[id]; ok && st.Profile.P1604Cfg.Unit != "" {
		unit = st.Profile.P1604Cfg.Unit
	}
	a.mu.RUnlock()

	if sink != nil {
		snapshot := core.PressureSnapshot{
			DeviceID:  id,
			Timestamp: core.TimestampMs(),
			Values:    channels,
			Unit:      unit,
		}
		// 设备时间戳转换为秒（float64），供 CSV 录制器的时间格式化使用
		// csv_recorder 将 HardwareTimestamp 解释为秒.纳秒
		if deviceTimestampMs > 0 {
			snapshot.HardwareTimestamp = float64(deviceTimestampMs) / 1000.0
		}
		sink(snapshot)
	}
}

// sendCommand 发送命令到设备
func (d *p1604Driver) sendCommand(cmd string) error {
	if d.conn == nil {
		return fmt.Errorf("not connected")
	}
	d.conn.SetWriteDeadline(time.Now().Add(p1604CommandTimeout))
	_, err := d.conn.Write([]byte(cmd + "\r\n"))
	return err
}

// isConnectionFault 启发式判定错误是否由连接故障引起。
// 用于在停止/关闭路径上把 "连接已经断开导致命令失败" 降级为 debug 日志，
// 避免在正常断开/重连流程中刷出大量 warn 噪音。
//
// 仅作日志分级用途，不可作为状态机的输入条件。
func isConnectionFault(err error) bool {
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
