package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"shared/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

type DAQP1604Device struct {
	*BaseDevice
	mu        sync.Mutex
	conn      net.Conn
	acquiring bool
	cancel    context.CancelFunc
	cmdProto  *CommandProtocol
	channels  []device.ChannelConfig
}

func NewDAQP1604Device(config device.DeviceConfig) *DAQP1604Device {
	if config.Address == "" {
		config.Address = "192.168.3.101"
	}
	if config.Port == 0 {
		config.Port = 9000
	}
	return &DAQP1604Device{
		BaseDevice: NewBaseDevice(config),
		cmdProto:   NewCommandProtocol(),
		channels:   defaultP1604Channels(),
	}
}

func defaultP1604Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{Index: i, Name: fmt.Sprintf("CH%d", i+1), Enabled: true, Unit: "Pa", Precision: 3}
	}
	channels[16] = device.ChannelConfig{Index: 16, Name: "AtmosphericPressure", Enabled: true, Unit: "Pa", Precision: 0}
	channels[17] = device.ChannelConfig{Index: 17, Name: "Temperature", Enabled: true, Unit: "°C", Precision: 2}
	return channels
}

func (d *DAQP1604Device) Connect() error {
	d.setState(device.StateConnecting)
	addr := fmt.Sprintf("%s:%d", d.config.Address, d.config.Port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		d.setError(err.Error())
		return err
	}
	d.mu.Lock()
	d.conn = conn
	d.mu.Unlock()

	resp, err := d.SendCommand("w1601", 1000)
	if err != nil {
		conn.Close()
		d.setError(err.Error())
		return fmt.Errorf("send w1601 command: %w", err)
	}
	if len(resp) > 0 && resp[0] == 'N' {
		conn.Close()
		d.setError(resp)
		return fmt.Errorf("w1601 command rejected: %s", resp)
	}

	d.setState(device.StateConnected)
	slog.Info("DAQ-P-1604 connected", "device", d.config.ID, "addr", addr)
	return nil
}

func (d *DAQP1604Device) Disconnect() error {
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	d.acquiring = false
	d.cmdProto.Clear()
	d.mu.Unlock()
	d.setState(device.StateDisconnected)
	return nil
}

func (d *DAQP1604Device) StartAcquisition() error {
	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		return nil
	}
	conn := d.conn
	if conn == nil {
		d.mu.Unlock()
		return fmt.Errorf("not connected")
	}

	samplingRate := d.config.SamplingRate
	if samplingRate <= 0 {
		samplingRate = 10
	}
	periodMs := 1000 / samplingRate
	if periodMs < 10 {
		periodMs = 10
	}
	streamID := 1

	configCmd := fmt.Sprintf("c 00 %d FFFF 1 %d 7 0", streamID, periodMs)
	d.mu.Unlock()
	paramsResp, err := d.SendCommand(configCmd, 1000)
	d.mu.Lock()
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("send stream config command: %w", err)
	}
	if len(paramsResp) > 0 && paramsResp[0] == 'N' {
		d.mu.Unlock()
		return fmt.Errorf("stream config rejected: %s", paramsResp)
	}

	contentCmd := fmt.Sprintf("c 05 %d 0810", streamID)
	d.mu.Unlock()
	contentResp, err := d.SendCommand(contentCmd, 1000)
	d.mu.Lock()
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("send stream content command: %w", err)
	}
	if len(contentResp) > 0 && contentResp[0] == 'N' {
		d.mu.Unlock()
		return fmt.Errorf("stream content rejected: %s", contentResp)
	}

	startCmd := fmt.Sprintf("c 01 %d", streamID)
	d.mu.Unlock()
	startResp, err := d.SendCommand(startCmd, 1000)
	d.mu.Lock()
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("send start stream command: %w", err)
	}
	if len(startResp) > 0 && startResp[0] == 'N' {
		d.mu.Unlock()
		return fmt.Errorf("start stream rejected: %s", startResp)
	}

	d.acquiring = true
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.mu.Unlock()

	d.setState(device.StateAcquiring)
	d.setAcquiring(true)
	go d.receiveData(ctx)
	slog.Info("DAQ-P-1604 acquisition started", "device", d.config.ID, "periodMs", periodMs)
	return nil
}

func (d *DAQP1604Device) StopAcquisition() error {
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	conn := d.conn
	d.acquiring = false
	d.mu.Unlock()

	if conn != nil {
		if resp, err := d.SendCommand("c 02 1", 1000); err != nil {
			slog.Warn("DAQ-P-1604 stop stream command error", "device", d.config.ID, "err", err)
		} else if len(resp) > 0 && resp[0] == 'N' {
			slog.Warn("DAQ-P-1604 stop stream rejected", "device", d.config.ID, "resp", resp)
		}
	}

	d.setState(device.StateConnected)
	d.setAcquiring(false)
	slog.Info("DAQ-P-1604 acquisition stopped", "device", d.config.ID)
	return nil
}

func (d *DAQP1604Device) SendCommand(command string, timeoutMs int) (string, error) {
	d.mu.Lock()
	conn := d.conn
	d.mu.Unlock()
	if conn == nil {
		return "", fmt.Errorf("device %s not connected", d.config.ID)
	}
	return d.cmdProto.SendCommandAndWait(command, func() error {
		_, err := conn.Write([]byte(command))
		return err
	}, timeoutMs)
}

func (d *DAQP1604Device) UpdateChannels(channels []device.ChannelConfig) {
	d.mu.Lock()
	d.channels = channels
	d.mu.Unlock()
}

func (d *DAQP1604Device) GetChannels() []device.ChannelConfig {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]device.ChannelConfig, len(d.channels))
	copy(result, d.channels)
	return result
}

func (d *DAQP1604Device) receiveData(ctx context.Context) {
	d.mu.Lock()
	conn := d.conn
	d.mu.Unlock()
	if conn == nil {
		return
	}

	reader := protocol.NewFrameReader(conn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		frame, err := reader.ReadFrame()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("DAQ-P-1604 frame read error", "device", d.config.ID, "err", err)
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			continue
		}

		if protocol.IsASCIIFrame(frame) {
			text := string(frame)
			if d.cmdProto.DispatchResponse(text) {
				slog.Debug("DAQ-P-1604 command response", "device", d.config.ID, "data", text[:min(len(text), 80)])
			}
			continue
		}

		channels, err := protocol.ParseStreamFrame(frame)
		if err != nil {
			slog.Warn("DAQ-P-1604 parse error", "device", d.config.ID, "err", err)
			continue
		}

		channelIndices := make([]int, len(channels))
		for i := range channelIndices {
			channelIndices[i] = i
		}
		payload := device.DataPayload{
			DeviceID:       d.config.ID,
			Timestamp:      time.Now().UnixMilli(),
			Channels:       channels,
			ChannelIndices: channelIndices,
		}
		if sink := d.GetDataSink(); sink != nil {
			sink(payload)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
