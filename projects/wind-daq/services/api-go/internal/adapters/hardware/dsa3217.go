package hardware

import (
	"bufio"
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	DSA3217_DEFAULT_HOST = "192.168.3.7"
	DSA3217_DEFAULT_PORT = 23
	DSA3217_TIMEOUT      = 5 * time.Second
)

type DSA3217 struct {
	mu              sync.RWMutex
	ioMu            sync.Mutex
	profile         device.Profile
	status          device.Status
	sink            device.DataSink
	stop            chan struct{}
	acquiring       bool
	scanning        bool
	conn            net.Conn
	reader          *bufio.Reader
	lineBuffer      string
	dataHandler     chan string
	responseHandler chan string
	onError         func(err error) // 设备异常退出通知回调
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

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), DSA3217_TIMEOUT)
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
	d.mu.Lock()
	shouldStop, _ := d.stopAcquisitionLocked()
	d.mu.Unlock()

	if shouldStop {
		d.sendStopCommandIfConnected()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
		d.reader = nil
	}
	slog.Info("DSA3217 TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)

	d.status.Connection = device.ConnectionDisconnected
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
	stop := d.stop
	d.scanning = true

	go d.readLoop(stop)
	return nil
}

func (d *DSA3217) StopAcquisition() error {
	d.mu.Lock()
	shouldStop, err := d.stopAcquisitionLocked()
	d.mu.Unlock()
	if shouldStop {
		d.sendStopCommandIfConnected()
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

func (d *DSA3217) readLoop(stop <-chan struct{}) {
	if d.reader == nil {
		return
	}

	var unexpectedErr error

	defer func() {
		// 异常退出时更新状态并通知上层
		if unexpectedErr != nil {
			d.mu.Lock()
			d.acquiring = false
			d.scanning = false
			d.stop = nil
			d.status.Acquiring = false
			d.status.LastError = unexpectedErr.Error()
			if d.status.Connection == device.ConnectionAcquiring {
				d.status.Connection = device.ConnectionConnected
			}
			fn := d.onError
			d.mu.Unlock()

			slog.Warn("DSA3217 read loop exited unexpectedly", "device", d.profile.ID, "error", unexpectedErr)
			if fn != nil {
				fn(unexpectedErr)
			}
		}
	}()

	for {
		select {
		case <-stop:
			return
		default:
			d.ioMu.Lock()
			d.mu.RLock()
			conn := d.conn
			reader := d.reader
			d.mu.RUnlock()
			if reader == nil {
				d.ioMu.Unlock()
				return
			}
			if conn != nil {
				_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			}
			line, err := reader.ReadString('\n')
			d.ioMu.Unlock()
			if err != nil {
				// 读取超时不视为异常，继续等待
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				slog.Debug("DSA3217 read error", "device", d.profile.ID, "error", err)
				unexpectedErr = err
				return
			}

			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				continue
			}

			d.parseDataLine(line)
		}
	}
}

func (d *DSA3217) parseDataLine(line string) {
	d.mu.RLock()
	scanning := d.scanning
	sink := d.sink
	channels := d.profile.Channels
	d.mu.RUnlock()

	if !scanning || sink == nil {
		return
	}

	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		if v, err := strconv.ParseFloat(part, 64); err == nil {
			values = append(values, v)
		}
	}

	if len(values) == 0 {
		return
	}

	indices := make([]int, 0, len(channels))
	channelValues := make([]float64, 0, len(channels))

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if ch.Index >= 0 && ch.Index < len(values) {
			indices = append(indices, ch.Index)
			channelValues = append(channelValues, values[ch.Index])
		}
	}

	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
		Timestamp:      device.NowMs(),
		Channels:       channelValues,
		ChannelIndices: indices,
	})
}

func (d *DSA3217) sendCommand(cmd string) (string, error) {
	d.ioMu.Lock()
	defer d.ioMu.Unlock()

	d.mu.RLock()
	conn := d.conn
	d.mu.RUnlock()

	if conn == nil {
		return "", fmt.Errorf("not connected")
	}

	// 命令路径退出时清除读/写 deadline，避免过期的绝对时间
	// 被 readLoop 继承导致 ReadString 永远立即返回 timeout、CPU 死循环。
	defer func() {
		conn.SetWriteDeadline(time.Time{})
		conn.SetReadDeadline(time.Time{})
	}()

	conn.SetWriteDeadline(time.Now().Add(DSA3217_TIMEOUT))
	// 收发细节降级到 Debug：状态查询期间命令频繁，INFO 会刷爆 ring buffer 与日志文件。
	slog.Debug("DSA3217 command send", "category", "hardware-send", "component", "hardware", "device", d.profile.ID, "command", cmd)
	if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
		return "", err
	}

	d.mu.RLock()
	reader := d.reader
	d.mu.RUnlock()

	if reader == nil {
		return "", fmt.Errorf("not connected")
	}

	conn.SetReadDeadline(time.Now().Add(DSA3217_TIMEOUT))
	resp, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimRight(resp, "\r\n")
	slog.Debug("DSA3217 command response", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "response", trimmed)
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
