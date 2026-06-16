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
	p1604NumPressure    = 16
)

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
	mu       sync.RWMutex
	drivers  map[string]*p1604Driver
	status   map[string]*core.DeviceState
	sinks    map[string]func(core.PressureSnapshot)
	channels map[string]chan core.PressureSnapshot
	stopChs  map[string]chan struct{}
	logSink  func(DeviceLogEntry)
}

// p1604Driver 单个 P1604 设备的驱动实例
type p1604Driver struct {
	profile     core.PressureProfile
	conn        net.Conn
	frameReader *sharedproto.FrameReader
	acquiring   bool
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

func (a *P1604Adapter) emitLog(entry DeviceLogEntry) {
	a.mu.RLock()
	sink := a.logSink
	a.mu.RUnlock()
	if sink != nil {
		sink(entry)
	}
}

// Connect 连接设备
func (a *P1604Adapter) Connect(profile core.PressureProfile) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.drivers[profile.ID]; exists {
		return fmt.Errorf("device %s already connected", profile.ID)
	}

	host := profile.Address
	if host == "" {
		host = p1604DefaultHost
	}
	port := profile.Port
	if port <= 0 {
		port = p1604DefaultPort
	}

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

	a.drivers[profile.ID] = driver
	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		StatusText:  core.StatusConnected.String(),
		ConnectedAt: core.TimestampMs(),
	}

	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware", DeviceID: profile.ID,
		Message: "Device connected", Detail: fmt.Sprintf("%s:%d", host, port),
	})
	return nil
}

// Disconnect 断开设备连接
func (a *P1604Adapter) Disconnect(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	_ = a.stopAcquisitionLocked(id)
	driver, ok := a.drivers[id]
	if !ok {
		return nil
	}
	delete(a.drivers, id)
	delete(a.sinks, id)
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusDisconnected)
	}
	return driver.conn.Close()
}

// StartAcquisition 启动数据采集
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

	// 配置数据流参数：c 00 <st> <mask> <sync> <per> <fmt> <mode>
	periodMs := driver.profile.P1604Cfg.SamplingRate
	if periodMs < 10 {
		periodMs = 100 // 默认 100ms
	}
	if err := driver.sendCommand(fmt.Sprintf("c 00 1 FFFF 1 %d 7 0", periodMs)); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 配置流返回内容：0810 = 压力 + 大气压力 + 大气温度
	if err := driver.sendCommand("c 05 1 0810"); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("set stream content: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 启动数据流
	if err := driver.sendCommand("c 01 1"); err != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("start stream: %w", err)
	}

	ch := make(chan core.PressureSnapshot, 8192)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done
	driver.acquiring = true

	directSink := func(snapshot core.PressureSnapshot) {
		select {
		case ch <- snapshot:
		case <-done:
		}
	}
	a.sinks[id] = directSink

	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	a.mu.Unlock()

	// 启动读取循环
	go a.readLoop(id, driver, done)

	return ch, nil
}

// StopAcquisition 停止数据采集
func (a *P1604Adapter) StopAcquisition(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopAcquisitionLocked(id)
}

func (a *P1604Adapter) stopAcquisitionLocked(id string) error {
	driver, ok := a.drivers[id]
	if !ok {
		return nil
	}

	if driver.acquiring {
		if err := driver.sendCommand("c 02 1"); err != nil {
			slog.Warn("DAQ-P-1604 stop stream command failed", "device", id, "error", err)
		}
		driver.acquiring = false
	}

	if done, ok := a.stopChs[id]; ok {
		close(done)
		delete(a.stopChs, id)
	}
	delete(a.sinks, id)

	if ch, ok := a.channels[id]; ok {
		close(ch)
		delete(a.channels, id)
	}

	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusConnected)
	}
	return nil
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
func (a *P1604Adapter) readLoop(id string, driver *p1604Driver, stop <-chan struct{}) {
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
				// 连接断开，清理状态
				a.mu.Lock()
				st, exists := a.status[id]
				if !exists || st.Status == core.StatusDisconnected {
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
				st.SetStatus(core.StatusConnected)
				st.AcquiringAt = 0
				a.mu.Unlock()
				return
			}
			if len(payload) > 0 {
				a.processPayload(id, payload)
			}
		}
	}
}

// processPayload 处理接收到的数据帧
func (a *P1604Adapter) processPayload(id string, data []byte) {
	// 区分 ASCII 响应和二进制帧
	if sharedproto.IsASCIIFrame(data) {
		// ASCII 响应（命令确认等），忽略
		return
	}

	// 解析二进制数据帧（ParseStreamFrame 已处理前 16 通道逆序）
	channels, err := sharedproto.ParseStreamFrame(data)
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
		sink(core.PressureSnapshot{
			DeviceID:  id,
			Timestamp: core.TimestampMs(),
			Values:    channels,
			Unit:      unit,
		})
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

// isConnectionFault 判断是否为连接故障
func isConnectionFault(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "reset by peer") ||
		strings.Contains(message, "device disconnected")
}
