package hardware

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"shared/device-sdk/go/protocol"
	"shared/device-sdk/go/serialport"
	"wind-daq/services/api-go/internal/core/device"
)

type DAQT1603Device struct {
	*BaseDevice
	mu        sync.Mutex
	conn      net.Conn
	serial    *serialport.Port
	acquiring bool
	cancel    context.CancelFunc
	cmdProto  *CommandProtocol
	channels  []device.ChannelConfig
}

func NewDAQT1603Device(config device.DeviceConfig) *DAQT1603Device {
	if config.Transport == "" {
		config.Transport = device.TransportTCP
	}
	if config.Transport == device.TransportTCP {
		if config.Address == "" {
			config.Address = "192.168.1.7"
		}
		if config.Port == 0 {
			config.Port = 9000
		}
	}
	if config.BaudRate == 0 {
		config.BaudRate = 460800
	}
	return &DAQT1603Device{
		BaseDevice: NewBaseDevice(config),
		cmdProto:   NewCommandProtocol(),
		channels:   defaultT1603Channels(),
	}
}

func defaultT1603Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 16)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{Index: i, Name: fmt.Sprintf("CH%d", i+1), Enabled: true, Unit: "°C", Precision: 2}
	}
	return channels
}

func (d *DAQT1603Device) Connect() error {
	d.setState(device.StateConnecting)

	if d.config.Transport == device.TransportTCP {
		addr := fmt.Sprintf("%s:%d", d.config.Address, d.config.Port)
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			d.setError(err.Error())
			return err
		}
		d.mu.Lock()
		d.conn = conn
		d.mu.Unlock()
	} else {
		cfg := serialport.Config{
			Name: d.config.SerialPort, BaudRate: d.config.BaudRate,
			DataBits: 8, Parity: 0, StopBits: 1, ReadTimeout: 1 * time.Second,
		}
		p, err := serialport.Open(cfg)
		if err != nil {
			d.setError(err.Error())
			return err
		}
		d.mu.Lock()
		d.serial = p
		d.mu.Unlock()
	}

	d.setState(device.StateConnected)
	slog.Info("DAQ-T-1603 connected", "device", d.config.ID, "transport", d.config.Transport)
	return nil
}

func (d *DAQT1603Device) Disconnect() error {
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
	if d.serial != nil {
		d.serial.Close()
		d.serial = nil
	}
	d.acquiring = false
	d.cmdProto.Clear()
	d.mu.Unlock()
	d.setState(device.StateDisconnected)
	return nil
}

func (d *DAQT1603Device) StartAcquisition() error {
	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		return nil
	}
	if d.config.Transport == device.TransportTCP && d.conn != nil {
		mask := d.calculateChannelMask()
		d.conn.Write([]byte(fmt.Sprintf("@f0 %s 2\n", mask)))
	}
	if d.config.Transport == device.TransportSerial && d.serial != nil {
		d.serial.Write([]byte{0x55, 0xAA, 0x03, 0xF0, 0x00, 0x00})
	}
	d.acquiring = true
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.mu.Unlock()
	d.setState(device.StateAcquiring)
	d.setAcquiring(true)
	go d.receiveData(ctx)
	slog.Info("DAQ-T-1603 acquisition started", "device", d.config.ID)
	return nil
}

func (d *DAQT1603Device) StopAcquisition() error {
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	serial := d.serial
	d.acquiring = false
	d.mu.Unlock()
	if d.config.Transport == device.TransportSerial && serial != nil {
		serial.Write([]byte{0x55, 0xAA, 0x03, 0xF1, 0x00, 0x00})
	}
	d.setState(device.StateConnected)
	d.setAcquiring(false)
	slog.Info("DAQ-T-1603 acquisition stopped", "device", d.config.ID)
	return nil
}

func (d *DAQT1603Device) SendCommand(command string, timeoutMs int) (string, error) {
	if d.config.Transport == device.TransportSerial {
		return "E", nil
	}
	d.mu.Lock()
	conn := d.conn
	d.mu.Unlock()
	if conn == nil {
		return "", fmt.Errorf("device %s not connected", d.config.ID)
	}
	return d.cmdProto.SendCommandAndWait(command, func() error {
		_, err := conn.Write([]byte(command + "\n"))
		return err
	}, timeoutMs)
}

func (d *DAQT1603Device) UpdateChannels(channels []device.ChannelConfig) {
	d.mu.Lock()
	d.channels = channels
	d.mu.Unlock()
}

func (d *DAQT1603Device) GetChannels() []device.ChannelConfig {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]device.ChannelConfig, len(d.channels))
	copy(result, d.channels)
	return result
}

func (d *DAQT1603Device) receiveData(ctx context.Context) {
	if d.config.Transport == device.TransportTCP {
		d.receiveTCPData(ctx)
	} else {
		d.receiveSerialData(ctx)
	}
}

func (d *DAQT1603Device) receiveTCPData(ctx context.Context) {
	d.mu.Lock()
	conn := d.conn
	d.mu.Unlock()
	if conn == nil {
		return
	}
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		binaryBuf := make([]byte, protocol.TCPFrameSize)
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := io.ReadFull(conn, binaryBuf)
		if err == nil {
			channels, parseErr := protocol.ParseTCPFrame(binaryBuf)
			if parseErr == nil {
				d.pushData(channels)
				continue
			}
			text := string(binaryBuf)
			if d.cmdProto.DispatchResponse(text) {
				slog.Debug("T1603 command response", "device", d.config.ID, "data", text[:minInt(len(text), 80)])
			}
			continue
		}
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, readErr := conn.Read(buf)
		if readErr != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			continue
		}
		if n > 0 {
			text := string(buf[:n])
			if d.cmdProto.DispatchResponse(text) {
				slog.Debug("T1603 command response", "device", d.config.ID, "data", text[:minInt(len(text), 80)])
			}
		}
	}
}

func (d *DAQT1603Device) receiveSerialData(ctx context.Context) {
	d.mu.Lock()
	s := d.serial
	d.mu.Unlock()
	if s == nil {
		return
	}
	buf := make([]byte, protocol.SerialFrameSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := s.ReadFull(buf); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("DAQ-T-1603 serial read error", "device", d.config.ID, "err", err)
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			continue
		}
		channels, err := protocol.ParseSerialFrame(buf)
		if err != nil {
			slog.Warn("DAQ-T-1603 serial parse error", "device", d.config.ID, "err", err)
			continue
		}
		d.pushData(channels)
	}
}

func (d *DAQT1603Device) pushData(channels []float64) {
	channelIndices := make([]int, len(channels))
	for i := range channelIndices {
		channelIndices[i] = i
	}
	payload := device.DataPayload{
		DeviceID: d.config.ID, Timestamp: time.Now().UnixMilli(),
		Channels: channels, ChannelIndices: channelIndices,
	}
	if sink := d.GetDataSink(); sink != nil {
		sink(payload)
	}
}

func (d *DAQT1603Device) calculateChannelMask() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	mask := 0
	for i := 0; i < 16 && i < len(d.channels); i++ {
		if d.channels[i].Enabled {
			mask |= (1 << i)
		}
	}
	return fmt.Sprintf("%04X", mask)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
