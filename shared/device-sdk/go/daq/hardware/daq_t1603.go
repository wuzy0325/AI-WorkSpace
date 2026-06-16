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
	mu             sync.RWMutex
	logMu          sync.RWMutex
	writeMu        sync.Mutex // serializes connection writes (config sync vs @f0/@f1)
	profile        core.Profile
	status         core.Status
	sink           core.DataSink
	stop           chan struct{}
	acquiring      bool
	conn           net.Conn
	frameReader    *protocol.T1603FrameReader
	config         core.DaqT1603HardwareConfig
	onConfigSynced onConfigSyncedFn
	onReadLoopExit func(error)
	onLog          func(LogEntry)
	readErrors     int
	frameErrors    int
	configSyncDone chan struct{} // 配置同步完成后关闭，StartAcquisition 需等待
	readLoopDone   chan struct{} // readLoop 退出后关闭，确保下次 StartAcquisition 前连接无并发读取
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

// OnConfigSynced registers a callback invoked after hardware config is synced.
func (d *DAQT1603) OnConfigSynced(fn onConfigSyncedFn) {
	d.mu.Lock()
	d.onConfigSynced = fn
	d.mu.Unlock()
}

// OnReadLoopExit registers a callback invoked when the acquisition read loop
// exits unexpectedly (error or no-data timeout).
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

func (d *DAQT1603) writeCommandOnly(cmd string) error {
	d.emitLog("debug", "hardware-send", "Send command", cmd)
	if d.conn == nil {
		return fmt.Errorf("device not connected")
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	// 设置写超时，防止连接异常时无限阻塞
	_ = d.conn.SetWriteDeadline(time.Now().Add(DAQ_T_1603_TIMEOUT))
	_, err := d.conn.Write([]byte(cmd + "\n"))
	_ = d.conn.SetWriteDeadline(time.Time{}) // 清除 deadline，避免影响后续写入
	if err != nil {
		d.emitLog("error", "hardware-send", "Command write failed", err.Error())
		return err
	}
	return nil
}

// drainConnection 清空 TCP 连接中的残留数据，确保下一次操作从干净状态开始。
// 在停止采集后调用，清除设备继续发送的残留帧和 ACK 响应。
func (d *DAQT1603) drainConnection(conn net.Conn, timeout time.Duration) {
	if conn == nil {
		return
	}
	buf := make([]byte, 4096)
	totalDrained := 0
	for i := 0; i < 20; i++ {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := conn.Read(buf)
		if n > 0 {
			totalDrained += n
		}
		if err != nil {
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
	d.configSyncDone = make(chan struct{}) // 初始化配置同步完成信号

	go d.syncHardwareConfig()

	return nil
}

func (d *DAQT1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	// 关闭配置同步通道，防止 StartAcquisition 永远阻塞
	if d.configSyncDone != nil {
		select {
		case <-d.configSyncDone:
			// 已关闭
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

	// 等待上一次 readLoop 完全退出，确保连接上没有并发读取
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

	// 等待配置同步完成，避免与 syncHardwareConfig 的命令/响应冲突
	if d.configSyncDone != nil {
		syncDone := d.configSyncDone
		d.mu.Unlock()
		slog.Info("DAQ-T-1603 waiting for config sync before acquisition", "device", d.profile.ID)
		<-syncDone // 阻塞直到配置同步完成
		d.mu.Lock()
		slog.Info("DAQ-T-1603 config sync done, proceeding with acquisition", "device", d.profile.ID)
		if d.acquiring {
			return nil
		}
		if d.conn == nil || d.status.Connection == core.ConnectionDisconnected {
			return fmt.Errorf("device disconnected before acquisition start")
		}
	}

	// 确保 @f1 已发送并生效：同步发送 @f1 停止命令，
	// 防止异步 @f1 还未执行导致设备仍在输出数据
	if d.conn != nil {
		d.writeMu.Lock()
		_ = d.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		d.conn.Write([]byte("@f1\n"))
		_ = d.conn.SetWriteDeadline(time.Time{})
		d.writeMu.Unlock()
		// 短暂等待设备停止输出
		time.Sleep(50 * time.Millisecond)
		d.drainConnection(d.conn, 100*time.Millisecond)
	}
	// 清空 FrameReader 缓冲区
	if d.frameReader != nil {
		d.frameReader.Reset()
	}

	mask := d.config.ChannelMask
	if mask == "" {
		mask = "FFFF"
	}

	cmd := fmt.Sprintf("@f0 %s 2", mask)
	slog.Info("DAQ-T-1603 sending start acquisition command", "device", d.profile.ID, "cmd", cmd)
	err := d.writeCommandOnly(cmd)
	if err != nil {
		return fmt.Errorf("send %s: %w", cmd, err)
	}

	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(d.config.BinaryFormat)
		d.frameReader.SetMetadataMode(d.config.ShowTimestamp || d.config.ShowSequence)
		// 仅在完整匹配 ACK 时才消费，避免 TCP 分包造成帧边界错位。
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
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == core.ConnectionAcquiring {
		d.status.Connection = core.ConnectionConnected
	}
	// 异步发送 @f1 停止命令，保持 StopAcquisition 响应速度
	if d.conn != nil && wasAcquiring {
		conn := d.conn
		go func() {
			d.writeMu.Lock()
			defer d.writeMu.Unlock()
			d.emitLog("debug", "hardware-send", "Send command", "@f1")
			_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
			if _, err := conn.Write([]byte("@f1\n")); err != nil {
				d.emitLog("warn", "hardware-send", "Stop command write failed", err.Error())
			}
			_ = conn.SetWriteDeadline(time.Time{})
		}()
	}
	// 清空 FrameReader 缓冲区，避免残留数据干扰下一次采集的帧解析
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
	var unexpectedErr error // set when exit is due to error/timeout, not normal stop

	// 捕获当前连接引用，避免 defer 或循环体中 d.conn 被外部 Connect() 替换，
	// 导致 @f1 发送到错误的连接或读取错误连接的帧数据
	conn := d.conn

	defer func() {
		// 仅在异常退出时发送 @f1 和更新状态
		// 正常停止时 stopAcquisitionLocked 已同步发送 @f1 并更新状态
		if unexpectedErr != nil {
			d.writeMu.Lock()
			if conn != nil {
				_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
				conn.Write([]byte("@f1\n"))
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

		// 通知 readLoop 已退出，使下一次 StartAcquisition 可以安全地操作连接
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
			// Normal stop requested via StopAcquisition.
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
				// 连接被主动关闭（Disconnect 调用 conn.Close()），
				// 检查 stop 通道是否已关闭，如果是则视为正常退出
				if isClosedConnError(err) {
					select {
					case <-stop:
						return // 正常停止，不设置 unexpectedErr
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
		d.mu.Unlock()
		slog.Debug("DAQ-T-1603 frame parse error", "device", d.profile.ID, "n", len(data), "error", err)
		d.emitLog("warn", "acquisition", "Frame parse error", fmt.Sprintf("%s raw=% X", err.Error(), data[:min(len(data), 16)]))
		return
	}

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

// -- hardware config sync --

func (d *DAQT1603) syncHardwareConfig() {
	time.Sleep(300 * time.Millisecond)
	startedAt := time.Now()

	d.mu.RLock()
	conn := d.conn
	isConnected := d.status.Connection == core.ConnectionConnected
	alreadyAcquiring := d.acquiring
	syncDone := d.configSyncDone
	d.mu.RUnlock()

	// 确保最终关闭 configSyncDone 通道，避免 StartAcquisition 永远阻塞
	defer func() {
		if syncDone != nil {
			select {
			case <-syncDone:
				// 已关闭
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

	// @e3: 16 thermocouple type chars
	startedAt := time.Now()
	if resp, err := d.sendCommandExact(conn, "@e3", 16); err == nil && len(resp) == 16 {
		d.logConfigQuery("@e3", startedAt, resp, nil)
		cfg.ThermocoupleTypes = resp
	} else {
		d.logConfigQuery("@e3", startedAt, "", err)
		cfg.ThermocoupleTypes = "KKKKKKKKKKKKKKKK"
	}

	// @fd MCH: channel mask (hex)
	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd MCH"); err == nil {
		d.logConfigQuery("@fd MCH", startedAt, resp, nil)
		if len(resp) == 4 || len(resp) == 3 {
			cfg.ChannelMask = strings.TrimSpace(resp)
		}
	} else {
		d.logConfigQuery("@fd MCH", startedAt, "", err)
	}

	// @fd SPS: sampling rate
	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd SPS"); err == nil {
		d.logConfigQuery("@fd SPS", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.SamplingRate = v
		}
	} else {
		d.logConfigQuery("@fd SPS", startedAt, "", err)
	}

	// @fd BIN: binary format flag
	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd BIN", 1); err == nil {
		d.logConfigQuery("@fd BIN", startedAt, resp, nil)
		cfg.BinaryFormat = strings.TrimSpace(resp) == "1"
	} else {
		d.logConfigQuery("@fd BIN", startedAt, "", err)
	}

	// @fd TIME: timestamp flag
	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd TIME", 1); err == nil {
		d.logConfigQuery("@fd TIME", startedAt, resp, nil)
		cfg.ShowTimestamp = strings.TrimSpace(resp) == "1"
	} else {
		d.logConfigQuery("@fd TIME", startedAt, "", err)
	}

	// @fd HEAD: sequence flag
	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd HEAD", 1); err == nil {
		d.logConfigQuery("@fd HEAD", startedAt, resp, nil)
		cfg.ShowSequence = strings.TrimSpace(resp) == "1"
	} else {
		d.logConfigQuery("@fd HEAD", startedAt, "", err)
	}

	// @fd AVG: average count
	startedAt = time.Now()
	if resp, err := d.sendCommandIdle(conn, "@fd AVG"); err == nil {
		d.logConfigQuery("@fd AVG", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.AverageCount = v
		}
	} else {
		d.logConfigQuery("@fd AVG", startedAt, "", err)
	}

	// @fd TYPE: trigger mode
	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd TYPE", 1); err == nil {
		d.logConfigQuery("@fd TYPE", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerMode = v
		}
	} else {
		d.logConfigQuery("@fd TYPE", startedAt, "", err)
	}

	// @fd TRIG: trigger edge
	startedAt = time.Now()
	if resp, err := d.sendCommandExact(conn, "@fd TRIG", 1); err == nil {
		d.logConfigQuery("@fd TRIG", startedAt, resp, nil)
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerEdge = v
		}
	} else {
		d.logConfigQuery("@fd TRIG", startedAt, "", err)
	}

	// @fd TNUM: trigger count
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

// isClosedConnError 判断错误是否由连接被主动关闭引起
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