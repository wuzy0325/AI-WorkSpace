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

const (
	DAQ_T_1603_DEFAULT_HOST = "192.168.1.7"
	DAQ_T_1603_DEFAULT_PORT = 9000
	DAQ_T_1603_TIMEOUT      = 5 * time.Second
)

type onConfigSyncedFn func(core.DaqT1603HardwareConfig)
type onReadLoopExitFn func(error)

type DAQT1603 struct {
	mu             sync.RWMutex
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
	onReadLoopExit onReadLoopExitFn
	readErrors     int
	frameErrors    int
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

// OnReadLoopExit registers a callback invoked when the read loop exits on error.
func (d *DAQT1603) OnReadLoopExit(fn onReadLoopExitFn) {
	d.mu.Lock()
	d.onReadLoopExit = fn
	d.mu.Unlock()
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

	go d.syncHardwareConfig()

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

	mask := d.config.ChannelMask
	if mask == "" {
		mask = "FFFF"
	}
	cmd := fmt.Sprintf("@f0 %s 2", mask)
	d.writeMu.Lock()
	_, err := d.conn.Write([]byte(cmd + "\n"))
	d.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("send %s: %w", cmd, err)
	}

	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(d.config.BinaryFormat)
	}

	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	d.stop = make(chan struct{})
	stop := d.stop

	go d.readLoop(stop)
	return nil
}

func (d *DAQT1603) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *DAQT1603) stopAcquisitionLocked() error {
	if d.conn != nil {
		d.writeMu.Lock()
		d.conn.Write([]byte("@f1\n"))
		d.writeMu.Unlock()
	}
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == core.ConnectionAcquiring {
		d.status.Connection = core.ConnectionConnected
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
	}
	d.mu.Unlock()
	return nil
}

func (d *DAQT1603) readLoop(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
			d.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			payload, err := d.frameReader.ReadFrame()
			if err != nil {
				if errors.Is(err, protocol.ErrIncompleteFrame) {
					continue
				}
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				d.mu.Lock()
				d.readErrors++
				fn := d.onReadLoopExit
				d.mu.Unlock()
				slog.Debug("DAQ-T-1603 read error", "device", d.profile.ID, "error", err)
				if fn != nil {
					fn(err)
				}
				return
			}
			if len(payload) > 0 {
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

	temps, err := protocol.ParseTCPFrame(data)
	if err != nil {
		d.mu.Lock()
		d.frameErrors++
		d.mu.Unlock()
		slog.Debug("DAQ-T-1603 frame parse error", "device", d.profile.ID, "n", len(data), "error", err)
		return
	}

	indices := make([]int, len(temps))
	for i := range temps {
		indices[i] = i
	}

	sink(core.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      core.NowMs(),
		Channels:       temps,
		ChannelIndices: indices,
	})
}

// -- hardware config sync --

func (d *DAQT1603) syncHardwareConfig() {
	time.Sleep(300 * time.Millisecond)

	d.mu.RLock()
	conn := d.conn
	isConnected := d.status.Connection == core.ConnectionConnected
	d.mu.RUnlock()

	if conn == nil || !isConnected {
		return
	}

	d.writeMu.Lock()
	cfg := d.readAllConfig(conn)
	d.writeMu.Unlock()
	if cfg == nil {
		return
	}

	d.mu.Lock()
	if d.conn == nil || d.status.Connection == core.ConnectionDisconnected {
		d.mu.Unlock()
		return
	}
	d.config = *cfg
	d.profile.DaqT1603Config = *cfg
	if d.frameReader != nil {
		d.frameReader.SetBinaryMode(cfg.BinaryFormat)
	}
	fn := d.onConfigSynced
	d.mu.Unlock()

	if fn != nil {
		fn(*cfg)
	}
}

func (d *DAQT1603) readAllConfig(conn net.Conn) *core.DaqT1603HardwareConfig {
	cfg := &core.DaqT1603HardwareConfig{
		ChannelMask:  "FFFF",
		SamplingRate: 10,
		AverageCount: 1,
		TriggerMode:  0,
	}

	// @e3: 16 thermocouple type chars
	if resp, err := protocol.SendCommandExact(conn, "@e3", 16); err == nil && len(resp) == 16 {
		cfg.ThermocoupleTypes = resp
	} else {
		cfg.ThermocoupleTypes = "KKKKKKKKKKKKKKKK"
	}

	// @fd MCH: channel mask (hex)
	if resp, err := protocol.SendCommand(conn, "@fd MCH"); err == nil {
		if len(resp) == 4 || len(resp) == 3 {
			cfg.ChannelMask = strings.TrimSpace(resp)
		}
	}

	// @fd SPS: sampling rate
	if resp, err := protocol.SendCommand(conn, "@fd SPS"); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.SamplingRate = v
		}
	}

	// @fd BIN: binary format flag
	if resp, err := protocol.SendCommand(conn, "@fd BIN"); err == nil {
		cfg.BinaryFormat = strings.TrimSpace(resp) == "1"
	}

	// @fd TIME: timestamp flag
	if resp, err := protocol.SendCommand(conn, "@fd TIME"); err == nil {
		cfg.ShowTimestamp = strings.TrimSpace(resp) == "1"
	}

	// @fd HEAD: sequence flag
	if resp, err := protocol.SendCommand(conn, "@fd HEAD"); err == nil {
		cfg.ShowSequence = strings.TrimSpace(resp) == "1"
	}

	// @fd AVG: average count
	if resp, err := protocol.SendCommand(conn, "@fd AVG"); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.AverageCount = v
		}
	}

	// @fd TYPE: trigger mode
	if resp, err := protocol.SendCommand(conn, "@fd TYPE"); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerMode = v
		}
	}

	// @fd TRIG: trigger edge
	if resp, err := protocol.SendCommand(conn, "@fd TRIG"); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil {
			cfg.TriggerEdge = v
		}
	}

	// @fd TNUM: trigger count
	if resp, err := protocol.SendCommand(conn, "@fd TNUM"); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(resp)); err == nil && v > 0 {
			cfg.TriggerCount = v
		}
	}

	return cfg
}

func (d *DAQT1603) applyHardwareConfig(conn net.Conn, cfg core.DaqT1603HardwareConfig) error {
	if cfg.ThermocoupleTypes != "" {
		if len(cfg.ThermocoupleTypes) != 16 {
			return fmt.Errorf("thermocoupleTypes must be 16 characters")
		}
		if _, err := protocol.SendCommand(conn, "@f3 0"+cfg.ThermocoupleTypes+"0"); err != nil {
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
		if _, err := protocol.SendCommand(conn, cmd); err != nil {
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
