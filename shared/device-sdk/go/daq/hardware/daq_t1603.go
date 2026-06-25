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
	configSyncDone         chan struct{}
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
	hasDrained := false
	const maxIters = 200 // safety cap: ~20s at 100ms timeout

	for i := 0; i < maxIters; i++ {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
			hasDrained = true
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if hasDrained {
					break
				}
				continue
			}
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
	d.status.Connection = core.ConnectionConnected
	d.configSyncDone = make(chan struct{})

	go d.syncHardwareConfig()

	return nil
}

func (d *DAQT1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	if d.configSyncDone != nil {
		select {
		case <-d.configSyncDone:
		default:
			close(d.configSyncDone)
		}
		d.configSyncDone = nil
	}

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

	if d.configSyncDone != nil {
		syncDone := d.configSyncDone
		d.mu.Unlock()
		slog.Info("DAQ-T-1603 waiting for config sync before acquisition", "device", d.profile.ID)
		<-syncDone
		d.mu.Lock()
		slog.Info("DAQ-T-1603 config sync done, proceeding with acquisition", "device", d.profile.ID)
		if d.acquiring {
			return nil
		}
		if d.conn == nil || d.status.Connection == core.ConnectionDisconnected {
			return fmt.Errorf("device disconnected before acquisition start")
		}
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
				_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
				conn.Write([]byte("@f1"))
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

func (d *DAQT1603) syncHardwareConfig() {
	time.Sleep(300 * time.Millisecond)
	startedAt := time.Now()

	d.mu.RLock()
	conn := d.conn
	isConnected := d.status.Connection == core.ConnectionConnected
	alreadyAcquiring := d.acquiring
	syncDone := d.configSyncDone
	d.mu.RUnlock()

	defer func() {
		if syncDone != nil {
			select {
			case <-syncDone:
			default:
				close(syncDone)
			}
		}
	}()

	if conn == nil || !isConnected || alreadyAcquiring {
		return
	}

	d.emitLog("info", "system", "Starting config sync", "mode=skill-compatible")

	d.writeMu.Lock()
	cfg := d.readAllConfig(conn)
	modeSetOK := false
	if cfg != nil {
		// 强制设备使用 64 字节纯二进制帧（无 TIME/HEAD 前缀），帧读取最稳。
		// 三条命令必须全部成功，否则 FrameReader 与设备模式不一致 → 解析乱码。
		// 任何一条失败：不修改 cfg.BinaryFormat 等字段，让上层用设备实际状态。
		if _, err := d.sendCommand(conn, "@fe BIN 1"); err != nil {
			d.emitLog("warn", "system", "Force BIN mode failed", err.Error())
		} else if _, err := d.sendCommand(conn, "@fe TIME 0"); err != nil {
			d.emitLog("warn", "system", "Force TIME=0 failed", err.Error())
		} else if _, err := d.sendCommand(conn, "@fe HEAD 0"); err != nil {
			d.emitLog("warn", "system", "Force HEAD=0 failed", err.Error())
		} else {
			modeSetOK = true
		}
		if modeSetOK {
			cfg.BinaryFormat = true
			cfg.ShowTimestamp = false
			cfg.ShowSequence = false
		}
	}
	d.writeMu.Unlock()
	if cfg == nil {
		d.emitLog("warn", "system", "Config sync aborted", "nil config returned")
		return
	}

	d.mu.Lock()
	if d.conn == nil || d.status.Connection == core.ConnectionDisconnected || d.acquiring {
		d.mu.Unlock()
		return
	}
	d.config = *cfg
	d.profile.DaqT1603Config = *cfg
	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(cfg.BinaryFormat)
		d.frameReader.SetMetadataMode(cfg.ShowTimestamp || cfg.ShowSequence)
	}
	fn := d.onConfigSynced
	d.mu.Unlock()

	if fn != nil {
		fn(*cfg)
	}

	slog.Info("DAQ-T-1603 config sync completed", "device", d.profile.ID)
	d.emitLog("info", "system", "Config sync completed", fmt.Sprintf("duration=%s mode=skill-compatible", time.Since(startedAt)))
}

func (d *DAQT1603) readAllConfig(conn net.Conn) *core.DaqT1603HardwareConfig {
	cfg := &core.DaqT1603HardwareConfig{
		ChannelMask:  "FFFF",
		SamplingRate: 10,
		AverageCount: 1,
		TriggerMode:  0,
	}

	startedAt := time.Now()
	if resp, err := d.sendCommandExact(conn, "@e3", 16); err == nil && len(resp) == 16 {
		d.logConfigQuery("@e3", startedAt, resp, nil)
		cfg.ThermocoupleTypes = resp
	} else {
		d.logConfigQuery("@e3", startedAt, "", err)
		cfg.ThermocoupleTypes = "KKKKKKKKKKKKKKKK"
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd MCH"); err == nil {
		d.logConfigQuery("@fd MCH", startedAt, resp, nil)
		if len(resp) == 4 || len(resp) == 3 {
			cfg.ChannelMask = strings.TrimSpace(resp)
		}
	} else {
		d.logConfigQuery("@fd MCH", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd SPS"); err == nil {
		d.logConfigQuery("@fd SPS", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.SamplingRate = v
		}
	} else {
		d.logConfigQuery("@fd SPS", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd BIN", 1); err == nil {
		d.logConfigQuery("@fd BIN", startedAt, resp, nil)
		cfg.BinaryFormat = strings.TrimSpace(resp) == "1"
	} else {
		d.logConfigQuery("@fd BIN", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd TIME", 1); err == nil {
		d.logConfigQuery("@fd TIME", startedAt, resp, nil)
		cfg.ShowTimestamp = strings.TrimSpace(resp) == "1"
	} else {
		d.logConfigQuery("@fd TIME", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd HEAD", 1); err == nil {
		d.logConfigQuery("@fd HEAD", startedAt, resp, nil)
		cfg.ShowSequence = strings.TrimSpace(resp) == "1"
	} else {
		d.logConfigQuery("@fd HEAD", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd AVG"); err == nil {
		d.logConfigQuery("@fd AVG", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.AverageCount = v
		}
	} else {
		d.logConfigQuery("@fd AVG", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd TYPE", 1); err == nil {
		d.logConfigQuery("@fd TYPE", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerMode = v
		}
	} else {
		d.logConfigQuery("@fd TYPE", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd TRIG", 1); err == nil {
		d.logConfigQuery("@fd TRIG", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerEdge = v
		}
	} else {
		d.logConfigQuery("@fd TRIG", startedAt, "", err)
	}

	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd TNUM"); err == nil {
		d.logConfigQuery("@fd TNUM", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.TriggerCount = v
		}
	} else {
		d.logConfigQuery("@fd TNUM", startedAt, "", err)
	}

	return cfg
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
