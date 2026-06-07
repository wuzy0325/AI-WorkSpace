package hardware

import (
	"fmt"
	"strings"
	"sync"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"daq-t1603/core"
	"daq-t1603/ports"
)

// hzToSpsMs 将频率(Hz)转换为设备 SPS 采集间隔(毫秒)
// 设备 SPS 参数含义为采集间隔毫秒，实际频率 = 1000/SPS
func hzToSpsMs(hz int) int {
	if hz <= 0 {
		return 10 // 默认 10ms = 100Hz
	}
	return 1000 / hz
}

// spsMsToHz 将设备 SPS 采集间隔(毫秒)转换为频率(Hz)
func spsMsToHz(ms int) int {
	if ms <= 0 {
		return 100 // 默认 100Hz
	}
	return 1000 / ms
}

type T1603Adapter struct {
	mu       sync.RWMutex
	drivers  map[string]*sharedhw.DAQT1603
	status   map[string]*core.DeviceState
	sinks    map[string]func(core.TemperatureSnapshot)
	channels map[string]chan core.TemperatureSnapshot
	stopChs  map[string]chan struct{}
	logSink  func(DeviceLogEntry)
}

type DeviceLogEntry struct {
	Level    string
	Category string
	DeviceID string
	Message  string
	Detail   string
}

func NewT1603Adapter() *T1603Adapter {
	return &T1603Adapter{
		drivers:  make(map[string]*sharedhw.DAQT1603),
		status:   make(map[string]*core.DeviceState),
		sinks:    make(map[string]func(core.TemperatureSnapshot)),
		channels: make(map[string]chan core.TemperatureSnapshot),
		stopChs:  make(map[string]chan struct{}),
	}
}

func (a *T1603Adapter) SetLogSink(sink func(DeviceLogEntry)) {
	a.mu.Lock()
	a.logSink = sink
	a.mu.Unlock()
}

func (a *T1603Adapter) emitLog(entry DeviceLogEntry) {
	a.mu.RLock()
	sink := a.logSink
	a.mu.RUnlock()
	if sink != nil {
		sink(entry)
	}
}

var _ ports.DevicePort = (*T1603Adapter)(nil)

func mapT1603SharedConfig(cfg core.T1603Config) sharedcore.DaqT1603HardwareConfig {
	tcTypes := cfg.ThermocoupleTypes
	if len(tcTypes) != 16 {
		tcTypes = "KKKKKKKKKKKKKKKK"
	}
	mask := cfg.ChannelMask
	if mask == "" {
		mask = "FFFF"
	}
	return sharedcore.DaqT1603HardwareConfig{
		ThermocoupleTypes: tcTypes,
		ChannelMask:       mask,
		SamplingRate:      hzToSpsMs(cfg.SamplingRate), // Hz → 采集间隔毫秒
		BinaryFormat:      false,
		AverageCount:      cfg.AverageCount,
		ShowTimestamp:     cfg.ShowTimestamp,
		ShowSequence:      cfg.ShowSequence,
	}
}

func (a *T1603Adapter) Connect(profile core.TemperatureProfile) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.drivers[profile.ID]; exists {
		return fmt.Errorf("device %s already connected", profile.ID)
	}

	sharedProfile := sharedcore.Profile{
		ID:             profile.ID,
		Name:           profile.Name,
		Type:           sharedcore.DeviceDaqT1603,
		Address:        profile.Address,
		Port:           profile.Port,
		SamplingRate:   hzToSpsMs(profile.SamplingRate), // Hz → 采集间隔毫秒
		DaqT1603Config: mapT1603SharedConfig(profile.T1603Cfg),
		Channels: func() []sharedcore.ChannelConfig {
			ch := make([]sharedcore.ChannelConfig, 16)
			for i := range ch {
				cfg := sharedcore.ChannelConfig{
					Index: i, Name: fmt.Sprintf("CH%02d", i+1),
					Enabled: true, Unit: "°C", Precision: 2,
				}
				if i < len(profile.Channels) {
					pc := profile.Channels[i]
					cfg.Name = pc.Name
					cfg.Enabled = pc.Enabled
					cfg.Unit = pc.Unit
					cfg.Precision = pc.Precision
				}
				ch[i] = cfg
			}
			return ch
		}(),
	}
	dev := sharedhw.NewDAQT1603(sharedProfile)
	dev.OnLog(func(entry sharedhw.LogEntry) {
		a.emitLog(DeviceLogEntry{
			Level:    entry.Level,
			Category: entry.Category,
			DeviceID: entry.DeviceID,
			Message:  entry.Message,
			Detail:   entry.Detail,
		})
	})

	dev.OnConfigSynced(func(cfg sharedcore.DaqT1603HardwareConfig) {
		a.mu.Lock()
		st, ok := a.status[profile.ID]
		if !ok {
			a.mu.Unlock()
			return
		}
		profile.T1603Cfg.SamplingRate = spsMsToHz(cfg.SamplingRate) // 采集间隔毫秒 → Hz
		profile.T1603Cfg.ChannelMask = cfg.ChannelMask
		profile.T1603Cfg.AverageCount = cfg.AverageCount
		profile.T1603Cfg.ShowTimestamp = cfg.ShowTimestamp
		profile.T1603Cfg.ShowSequence = cfg.ShowSequence
		profile.T1603Cfg.ThermocoupleTypes = cfg.ThermocoupleTypes
		st.Profile = profile
		a.mu.Unlock()
	})

	dev.OnReadLoopExit(func(err error) {
		a.mu.Lock()
		// 如果设备已经断开连接，说明是主动 Disconnect 导致的连接关闭，
		// readLoop 收到 "use of closed network connection" 是正常行为，不需要处理
		st, exists := a.status[profile.ID]
		if !exists || st.Status == core.StatusDisconnected {
			a.mu.Unlock()
			return
		}
		delete(a.sinks, profile.ID)
		if done, ok := a.stopChs[profile.ID]; ok {
			close(done)
			delete(a.stopChs, profile.ID)
		}
		if ch, ok := a.channels[profile.ID]; ok {
			close(ch)
			delete(a.channels, profile.ID)
		}
		st.Status = core.StatusConnected
		st.AcquiringAt = 0
		a.mu.Unlock()
	})

	if err := dev.Connect(); err != nil {
		return fmt.Errorf("connect device %s: %w", profile.ID, err)
	}

	a.drivers[profile.ID] = dev
	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		ConnectedAt: core.TimestampMs(),
	}
	return nil
}

func (a *T1603Adapter) Disconnect(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	_ = a.stopAcquisitionLocked(id)
	dev, ok := a.drivers[id]
	if !ok {
		return nil
	}
	delete(a.drivers, id)
	delete(a.sinks, id)
	if st, exists := a.status[id]; exists {
		st.Status = core.StatusDisconnected
	}
	return dev.Disconnect()
}

func (a *T1603Adapter) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	a.mu.Lock()
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := a.channels[id]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s already acquiring", id)
	}

	ch := make(chan core.TemperatureSnapshot, 8192)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done

	directSink := func(snapshot core.TemperatureSnapshot) {
		select {
		case ch <- snapshot:
		case <-done:
		}
	}
	a.sinks[id] = directSink

	if st, exists := a.status[id]; exists {
		st.Status = core.StatusAcquiring
		st.AcquiringAt = core.TimestampMs()
	}
	a.mu.Unlock()

	dev.SetDataSink(func(payload sharedcore.DataPayload) {
		values := make([]float64, len(payload.Channels))
		copy(values, payload.Channels)
		a.mu.RLock()
		sink := a.sinks[id]
		unit := "°C"
		if st, ok := a.status[id]; ok && len(st.Profile.Channels) > 0 {
			unit = st.Profile.Channels[0].Unit
		}
		a.mu.RUnlock()
		if sink != nil {
			sink(core.TemperatureSnapshot{
				DeviceID:          id,
				Timestamp:         payload.Timestamp,
				HardwareTimestamp: payload.HardwareTimestamp,
				Values:            values,
				Unit:              unit,
			})
		}
	})

	if err := dev.StartAcquisition(); err != nil {
		a.mu.Lock()
		delete(a.channels, id)
		delete(a.stopChs, id)
		delete(a.sinks, id)
		if st, exists := a.status[id]; exists {
			st.Status = core.StatusConnected
			st.AcquiringAt = 0
		}
		a.mu.Unlock()

		if isConnectionFault(err) {
			_ = a.Disconnect(id)
		}
		return nil, fmt.Errorf("start acquisition %s: %w", id, err)
	}
	return ch, nil
}

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

func (a *T1603Adapter) StopAcquisition(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopAcquisitionLocked(id)
}

func (a *T1603Adapter) stopAcquisitionLocked(id string) error {
	dev, ok := a.drivers[id]
	if !ok {
		return nil
	}

	if done, ok := a.stopChs[id]; ok {
		close(done)
		delete(a.stopChs, id)
	}
	delete(a.sinks, id)

	if err := dev.StopAcquisition(); err != nil {
		return err
	}

	if ch, ok := a.channels[id]; ok {
		close(ch)
		delete(a.channels, id)
	}

	if st, exists := a.status[id]; exists {
		st.Status = core.StatusConnected
	}
	return nil
}

func (a *T1603Adapter) Status(id string) (core.DeviceState, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	st, ok := a.status[id]
	if !ok {
		return core.DeviceState{}, false
	}
	dev, hasDev := a.drivers[id]
	if hasDev {
		ds := dev.Status()
		if ds.Connection == sharedcore.ConnectionDisconnected {
			st.Status = core.StatusDisconnected
		} else if ds.Acquiring {
			st.Status = core.StatusAcquiring
		} else {
			st.Status = core.StatusConnected
		}
	}
	return *st, true
}

func (a *T1603Adapter) ApplyConfig(id string, cfg core.T1603Config) error {
	a.mu.Lock()
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("device %s not connected", id)
	}
	if st, exists := a.status[id]; exists {
		st.Profile.T1603Cfg = cfg
	}
	a.mu.Unlock()

	// 在锁外调用硬件通信，避免与 OnReadLoopExit/OnConfigSynced 回调死锁
	return dev.ApplyDaqT1603Config(mapT1603SharedConfig(cfg))
}

func (a *T1603Adapter) SetDataSink(id string, sink func(core.TemperatureSnapshot)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sinks[id] = sink
}
