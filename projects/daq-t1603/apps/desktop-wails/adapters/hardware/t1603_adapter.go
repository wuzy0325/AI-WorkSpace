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
		BinaryFormat:      true,
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
		SamplingRate:   hzToSpsMs(profile.T1603Cfg.SamplingRate), // Hz → 采集间隔毫秒
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
		profile.SamplingRate = profile.T1603Cfg.SamplingRate
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
		// readLoop 异常退出意味着连接已损坏（断线/超时/对端关闭等），
		// 必须断开驱动并清理 drivers 表，否则下次 StartAcquisition 会在坏连接上重试
		driver := a.drivers[profile.ID]
		delete(a.drivers, profile.ID)
		delete(a.sinks, profile.ID)
		if done, ok := a.stopChs[profile.ID]; ok {
			close(done)
			delete(a.stopChs, profile.ID)
		}
		if ch, ok := a.channels[profile.ID]; ok {
			close(ch)
			delete(a.channels, profile.ID)
		}
		st.SetStatus(core.StatusDisconnected)
		st.AcquiringAt = 0
		a.mu.Unlock()

		if driver != nil {
			driver.Disconnect()
		}
	})

	if err := dev.Connect(); err != nil {
		return fmt.Errorf("connect device %s: %w", profile.ID, err)
	}

	a.drivers[profile.ID] = dev
	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		StatusText:  core.StatusConnected.String(),
		ConnectedAt: core.TimestampMs(),
	}
	return nil
}

func (a *T1603Adapter) Disconnect(id string) error {
	// 在锁内只做状态翻转和驱动取出；硬件 I/O（StopAcquisition / Disconnect）
	// 一律放到锁外，避免与 OnReadLoopExit/OnConfigSynced 回调相互死锁。
	a.mu.Lock()
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil
	}
	// 标记停止意图并清理共享 channel/sink，让后续回调走静默退出分支。
	if done, exists := a.stopChs[id]; exists {
		close(done)
		delete(a.stopChs, id)
	}
	delete(a.sinks, id)
	delete(a.drivers, id)
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusDisconnected)
	}
	// channels[id] 由 dev.StopAcquisition() 在锁外完成后再关闭，避免与生产者并发。
	chToClose, hasCh := a.channels[id]
	if hasCh {
		delete(a.channels, id)
	}
	a.mu.Unlock()

	// 锁外：先停止采集，再关闭驱动；任一步骤失败都不影响其余清理。
	var stopErr error
	if dev != nil {
		stopErr = dev.StopAcquisition()
	}
	if hasCh {
		close(chToClose)
	}
	if dev == nil {
		return nil
	}
	if err := dev.Disconnect(); err != nil {
		return err
	}
	return stopErr
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

	// channel 缓冲区大小：65536 帧，在 1000Hz 下提供约 65 秒缓冲，
	// 多设备场景下确保 CSV flush 阻塞时不会立即反压到硬件 readLoop
	ch := make(chan core.TemperatureSnapshot, 65536)
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
		st.SetStatus(core.StatusAcquiring)
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
			st.SetStatus(core.StatusConnected)
			st.AcquiringAt = 0
		}
		a.mu.Unlock()

		if isConnectionFault(err) {
			_ = a.Disconnect(id)
		}
		return nil, fmt.Errorf("start acquisition %s: %w", id, err)
	}

	// 校验采集启动后状态，防止与 StopAcquisition 的竞态
	// 如果 stopChs[id] 已被清除，说明在 dev.StartAcquisition() 期间 StopAcquisition 被调用，
	// 此时需要停止驱动层采集并清理状态
	a.mu.Lock()
	_, channelExists := a.channels[id]
	stopCh, stopChExists := a.stopChs[id]
	a.mu.Unlock()

	if !channelExists || !stopChExists {
		_ = dev.StopAcquisition()
		return nil, fmt.Errorf("start acquisition %s: stopped concurrently", id)
	}

	// 检查 done 通道是否已关闭（StopAcquisition 在锁外关闭了 done）
	select {
	case <-stopCh:
		_ = dev.StopAcquisition()
		a.mu.Lock()
		delete(a.channels, id)
		delete(a.stopChs, id)
		delete(a.sinks, id)
		if st, exists := a.status[id]; exists {
			st.SetStatus(core.StatusConnected)
			st.AcquiringAt = 0
		}
		a.mu.Unlock()
		return nil, fmt.Errorf("start acquisition %s: stopped concurrently", id)
	default:
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
	// 锁内只标记停止 + 提取驱动句柄；dev.StopAcquisition 必须在锁外调用，
	// 否则会与 OnReadLoopExit/OnConfigSynced 回调互锁。
	a.mu.Lock()
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil
	}
	if done, exists := a.stopChs[id]; exists {
		close(done)
		delete(a.stopChs, id)
	}
	delete(a.sinks, id)
	chToClose, hasCh := a.channels[id]
	if hasCh {
		delete(a.channels, id)
	}
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusConnected)
	}
	a.mu.Unlock()

	stopErr := dev.StopAcquisition()
	if hasCh {
		close(chToClose)
	}
	return stopErr
}

// stopAcquisitionLocked 保留供内部需要在已持锁路径下使用；调用方需自行确保
// dev.StopAcquisition 不会回调 a.mu，否则务必改用 StopAcquisition。
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
		st.SetStatus(core.StatusConnected)
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
		// 信任驱动层状态：硬件层是 acquiring/connection 的唯一真相源。
		// 此前用 'st.Status != StatusAcquiring' 守卫想抑制闪烁，但会使
		// StopAcquisition 后 Status() 永远卡在 Acquiring，反映不到 Connected。
		if ds.Connection == sharedcore.ConnectionDisconnected {
			st.SetStatus(core.StatusDisconnected)
		} else if ds.Acquiring {
			st.SetStatus(core.StatusAcquiring)
		} else {
			st.SetStatus(core.StatusConnected)
		}
	} else {
		st.StatusText = st.Status.String()
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
