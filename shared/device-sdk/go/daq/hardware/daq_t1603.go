package hardware

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/protocol"
)

const noDataTimeout = 10 * time.Second
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
	mu                     sync.RWMutex
	logMu                  sync.RWMutex
	writeMu                sync.Mutex
	profile                core.Profile
	status                 core.Status
	sink                   core.DataSink
	stop                   chan struct{}
	acquiring              bool
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
}

func NewDAQT1603(profile core.Profile) *DAQT1603 {
	return &DAQT1603{
		profile: profile,
		config:  profile.DaqT1603Config,
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

func (d *DAQT1603) sendCommand(conn net.Conn, cmd string) (string, error) {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	resp, err := protocol.SendCommand(conn, cmd)
	if err != nil {
		d.emitLog("error", "hardware-recv", "Command failed", err.Error())
		return "", err
	}
	d.emitLog("debug", "hardware-recv", "Received response", strings.TrimSpace(resp))
	return resp, nil
}

func (d *DAQT1603) sendCommandIdle(conn net.Conn, cmd string) (string, error) {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	resp, err := protocol.SendCommandIdle(conn, cmd, 30*time.Millisecond)
	if err != nil {
		d.emitLog("error", "hardware-recv", "Command failed", err.Error())
		return "", err
	}
	d.emitLog("debug", "hardware-recv", "Received response", strings.TrimSpace(resp))
	return resp, nil
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
func (d *DAQT1603) writeCommandOnly(conn net.Conn, cmd string) error {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	if conn == nil {
		return fmt.Errorf("device not connected")
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(DAQ_T_1603_TIMEOUT))
	_, err := conn.Write([]byte(cmd))
	_ = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		d.emitLog("error", "hardware-send", "Command write failed", err.Error())
		return err
	}
	return nil
}

func (d *DAQT1603) drainConnection(conn net.Conn, timeout time.Duration) {
	if conn == nil {
		return
	}
	buf := make([]byte, 4096)
	totalDrained := 0
	// maxIters=10：单次 drainConnection 总耗时上限约 1s（10 × 100ms），
	// 足以吸收快速启停后的残留帧；过大的值（如 50 次 5s）会显著拖长 StartAcquisition。
	const maxIters = 10

	for i := 0; i < maxIters; i++ {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			// 连接已关闭等错误，无需继续
			break
		}
	}
	conn.SetReadDeadline(time.Time{})
	if totalDrained > 0 {
		slog.Debug("DAQ-T-1603 drained residual data", "device", d.profile.ID, "bytes", totalDrained)
		d.emitLog("debug", "system", "Drained residual data", fmt.Sprintf("%d bytes", totalDrained))
	}
}

func (d *DAQT1603) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

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

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), DAQ_T_1603_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(10 * time.Second)
	}

	d.conn = conn
	d.frameReader = protocol.NewT1603FrameReader(conn)
	d.frameReader.SetBinaryMode(d.config.BinaryFormat)
	d.frameReader.SetMetadataMode(d.config.ShowTimestamp || d.config.ShowSequence)
	d.status.Connection = core.ConnectionConnected

	if err := d.syncHardwareConfigLocked(conn); err != nil {
		_ = conn.Close()
		d.conn = nil
		d.frameReader = nil
		d.status.Connection = core.ConnectionDisconnected
		return err
	}

	return nil
}

func (d *DAQT1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
		d.frameReader = nil
	}

	d.status.Connection = core.ConnectionDisconnected
	return nil
}

func (d *DAQT1603) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.acquiring {
		return nil
	}
	if d.conn == nil {
		return fmt.Errorf("device not connected")
	}

	if d.readLoopDone != nil {
		done := d.readLoopDone
		d.mu.Unlock()
		slog.Info("DAQ-T-1603 waiting for readLoop to exit", "device", d.profile.ID)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			slog.Warn("DAQ-T-1603 timeout waiting for readLoop to exit", "device", d.profile.ID)
		}
		d.mu.Lock()
		d.readLoopDone = nil
	}

	if d.conn != nil {
		d.writeMu.Lock()
		_ = d.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		d.conn.Write([]byte("@f1"))
		_ = d.conn.SetWriteDeadline(time.Time{})
		d.writeMu.Unlock()
		time.Sleep(50 * time.Millisecond)
		d.drainConnection(d.conn, 100*time.Millisecond)
	}
	if d.frameReader != nil {
		d.frameReader.Reset()
	}

	mask := d.config.ChannelMask
	if mask == "" {
		mask = "FFFF"
	}

	cmd := fmt.Sprintf("@f0 %s 2", mask)
	slog.Info("DAQ-T-1603 sending start acquisition command", "device", d.profile.ID, "cmd", cmd)
	err := d.writeCommandOnly(d.conn, cmd)
	if err != nil {
		return fmt.Errorf("send %s: %w", cmd, err)
	}

	if d.frameReader != nil {
		// 200ms：覆盖慢固件/重负载下的 ACK 延迟（实测 80-180ms），
		// 短于 200ms 时 ACK 易被当作首个数据帧字节，破坏对齐。
		if _, err := d.frameReader.ConsumeOptionalACK(200 * time.Millisecond); err != nil {
			return fmt.Errorf("drain start ACK: %w", err)
		}
	}

	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})

	go d.readLoop()
	return nil
}

func (d *DAQT1603) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *DAQT1603) stopAcquisitionLocked() error {
	wasAcquiring := d.acquiring
	done := d.readLoopDone
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == core.ConnectionAcquiring {
		d.status.Connection = core.ConnectionConnected
	}
	if d.conn != nil && wasAcquiring {
		conn := d.conn
		d.writeMu.Lock()
		d.emitLog("debug", "hardware-send", "Send command", "@f1")
		_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		if _, err := conn.Write([]byte("@f1")); err != nil {
			// 连接已被 Disconnect 关闭时这是预期行为（非真异常），
			// 降级到 debug 避免日志被无害错误淹没。
			if isClosedConnError(err) {
				d.emitLog("debug", "hardware-send", "Stop command skipped (conn closed)", err.Error())
			} else {
				d.emitLog("warn", "hardware-send", "Stop command write failed", err.Error())
			}
		}
		_ = conn.SetWriteDeadline(time.Time{})
		d.writeMu.Unlock()
	}
	if wasAcquiring && done != nil {
		d.mu.Unlock()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			slog.Warn("DAQ-T-1603 timeout waiting for readLoop to exit after stop", "device", d.profile.ID)
		}
		d.mu.Lock()
		if d.readLoopDone == done {
			d.readLoopDone = nil
		}
	}
	if d.frameReader != nil {
		d.frameReader.Reset()
	}
	// 排空 TCP 接收缓冲区中的残留数据帧。
	// 残留来源：readLoop 退出时设备可能仍有数据帧在传输中（@f1 停止命令
	// 发出后，设备已排队的帧可能尚未完全到达）。若不排空，后续
	// ApplyDaqT1603Config 的 sendCommand 会把这些残留帧当作命令响应读出，
	// 导致配置命令失败。
	if d.conn != nil {
		d.drainConnection(d.conn, 100*time.Millisecond)
	}
	return nil
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
	d.mu.Lock()
	conn := d.conn
	if conn != nil && d.acquiring {
		d.mu.Unlock()
		return fmt.Errorf("cannot apply DAQ-T-1603 config while acquiring")
	}
	d.mu.Unlock()

	if conn != nil {
		// 排空 TCP 缓冲区中的残留数据帧，防止上次采集停止后的残留数据
		// 被 sendCommand 误读为命令响应（导致配置命令失败）。
		// frameReader.Reset 不在此调用：applyHardwareConfig 走 protocol.SendCommand
		// 读裸字节，不经过 frameReader；且本方法已释放 d.mu，与 StartAcquisition
		// 并发调用时 Reset 会与 readLoop 的 ReadFrame 竞争 frameReader 内部 buffer。
		// frameReader 的 Reset 由 stopAcquisitionLocked / StartAcquisition 在持 d.mu
		// 时统一完成。
		d.drainConnection(conn, 100*time.Millisecond)
		d.writeMu.Lock()
		err := d.applyHardwareConfig(conn, cfg)
		d.writeMu.Unlock()
		if err != nil {
			return err
		}
	}

	d.mu.Lock()
	d.config = cfg
	d.profile.DaqT1603Config = cfg
	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(cfg.BinaryFormat)
		d.frameReader.SetMetadataMode(cfg.ShowTimestamp || cfg.ShowSequence)
	}
	d.mu.Unlock()
	return nil
}

func (d *DAQT1603) readLoop() {
	lastDataAt := time.Now()
	var unexpectedErr error

	conn := d.conn

	defer func() {
		if unexpectedErr != nil {
			d.writeMu.Lock()
			if conn != nil {
				// 发 @f1 确保设备停止推送，连接已断开时属预期（与 stopAcquisitionLocked 一致）。
				// emitLog "Send command @f1" + 成败分支让 readLoop 退出路径可观测：
				// 否则操作员只看到 "Read loop exited unexpectedly" 无法判断设备侧是否真停了。
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
			}
			d.writeMu.Unlock()

			d.mu.Lock()
			d.acquiring = false
			d.stop = nil
			d.status.Acquiring = false
			if d.status.Connection == core.ConnectionAcquiring {
				d.status.Connection = core.ConnectionConnected
			}
			fn := d.onReadLoopExit
			d.mu.Unlock()

			d.emitLog("warn", "system", "Read loop exited unexpectedly", unexpectedErr.Error())
			if fn != nil {
				fn(unexpectedErr)
			}
		}

		d.mu.Lock()
		done := d.readLoopDone
		d.mu.Unlock()
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
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			payload, err := d.frameReader.ReadFrame()
			if err != nil {
				if errors.Is(err, protocol.ErrIncompleteFrame) {
					continue
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					if time.Since(lastDataAt) > noDataTimeout {
						slog.Warn("DAQ-T-1603 no data timeout", "device", d.profile.ID, "since", time.Since(lastDataAt))
						d.emitLog("warn", "acquisition", "No data timeout", time.Since(lastDataAt).String())
						unexpectedErr = fmt.Errorf("no data received for %v", noDataTimeout)
						return
					}
					continue
				}
				if isClosedConnError(err) {
					select {
					case <-stop:
						return
					default:
					}
				}
				d.mu.Lock()
				d.readErrors++
				d.mu.Unlock()
				slog.Debug("DAQ-T-1603 read error", "device", d.profile.ID, "error", err)
				d.emitLog("error", "acquisition", "Read loop error", err.Error())
				unexpectedErr = err
				return
			}
			if len(payload) > 0 {
				lastDataAt = time.Now()
				d.processPayload(payload)
			}
		}
	}
}

func (d *DAQT1603) processPayload(data []byte) {
	d.mu.RLock()
	sink := d.sink
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

		if consecutive >= maxConsecutiveFrameErrors && d.frameReader != nil {
			slog.Warn("DAQ-T-1603 auto-resync triggered", "device", d.profile.ID, "consecutiveErrors", consecutive)
			d.emitLog("warn", "acquisition", "Auto-resync triggered",
				fmt.Sprintf("consecutive=%d, skipping 1 byte to re-align", consecutive))
			d.frameReader.Resync()
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
	// HEAD（序号前缀）当前未在 UI 暴露，强制关闭。
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
	if _, err := d.sendCommand(conn, "@fe HEAD 0"); err != nil {
		d.emitLog("warn", "system", "Force HEAD=0 failed", err.Error())
		return fmt.Errorf("force HEAD=0: %w", err)
	}
	cfg.BinaryFormat = true
	cfg.ShowTimestamp = d.config.ShowTimestamp
	cfg.ShowSequence = false

	d.config = *cfg
	d.profile.DaqT1603Config = *cfg
	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(cfg.BinaryFormat)
		d.frameReader.SetMetadataMode(cfg.ShowTimestamp || cfg.ShowSequence)
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

// readAllConfig 逐条查询硬件配置。单条查询失败时记 warn 并保留字段默认值后继续，
// 不因某条辅助查询（旧固件可能不支持的 @fd AVG/TNUM 等）而整体放弃连接——
// 这是与同步化之前一致的容错语义。唯一会硬失败的边界是总预算耗尽
// （checkConfigSyncDeadline），用于对完全不响应的设备 fail-fast。
// BIN/TIME/HEAD 的读回值随后会被 syncHardwareConfigLocked 的 @fe 强制命令覆盖，
// 因此其读失败无害。
func (d *DAQT1603) readAllConfig(conn net.Conn, deadline time.Time) (*core.DaqT1603HardwareConfig, error) {
	cfg := &core.DaqT1603HardwareConfig{
		ChannelMask:       "FFFF",
		SamplingRate:      10,
		AverageCount:      1,
		TriggerMode:       0,
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
	}

	// query 在总预算内发送一条命令；失败时记日志并保留默认值，返回是否成功。
	// 总预算耗尽时返回 deadline 错误，由调用方硬失败。
	query := func(cmd string, fn func(resp string)) error {
		if err := checkConfigSyncDeadline(deadline); err != nil {
			return err
		}
		startedAt := time.Now()
		resp, err := d.sendCommandIdle(conn, cmd)
		if err != nil {
			d.logConfigQuery(cmd, startedAt, "", err)
			return nil // 单条失败：保留默认值，继续
		}
		d.logConfigQuery(cmd, startedAt, resp, nil)
		fn(resp)
		return nil
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
			return nil
		}
		d.logConfigQuery(cmd, startedAt, resp, nil)
		fn(resp)
		return nil
	}

	// @e3 热电偶类型（16 字节）
	if err := readExact("@e3", 16, func(resp string) {
		if len(resp) == 16 {
			cfg.ThermocoupleTypes = resp
		}
	}); err != nil {
		return nil, err
	}

	// @fd MCH 通道掩码
	if err := query("@fd MCH", func(resp string) {
		if len(resp) == 4 || len(resp) == 3 {
			cfg.ChannelMask = strings.TrimSpace(resp)
		}
	}); err != nil {
		return nil, err
	}

	// @fd SPS 采样间隔
	if err := query("@fd SPS", func(resp string) {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.SamplingRate = v
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

	// 以下为辅助配置，旧固件可能不支持，失败即保留默认值
	if err := query("@fd AVG", func(resp string) {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.AverageCount = v
		}
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
	if err := query("@fd TNUM", func(resp string) {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.TriggerCount = v
		}
	}); err != nil {
		return nil, err
	}

	return cfg, nil
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

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
