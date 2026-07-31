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

const (
	// adapterChannelCap 采集 channel 缓冲容量。
	// 65536 帧 = 1000Hz 下约 65 秒缓冲，
	// 在 relay 偶发卡顿（如 GC stop-the-world）时提供充足余量。
	adapterChannelCap = 65536

	// adapterChannelBackpressureThreshold 水位告警阈值。
	// 超过 80% 容量时发 warn 日志，提示 relay 消费速度异常。
	// 用整数算式避免 float64→int 常量转换错误。
	adapterChannelBackpressureThreshold = adapterChannelCap * 4 / 5

	// adapterBackpressureWarnIntervalMs 背压 warn 日志限频间隔。
	// 全局共享：10 台同时背压时 5 秒内最多 1 条 warn，避免日志刷屏。
	adapterBackpressureWarnIntervalMs = int64(5000)
)

type acquisitionOperation string

const (
	acquisitionOperationStarting acquisitionOperation = "starting"
	acquisitionOperationStopping acquisitionOperation = "stopping"
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
	mu         sync.RWMutex
	drivers    map[string]*sharedhw.DAQT1603
	status     map[string]*core.DeviceState
	sinks      map[string]func(core.TemperatureSnapshot)
	channels   map[string]chan core.TemperatureSnapshot
	stopChs    map[string]chan struct{}
	operations map[string]acquisitionOperation
	logSink    func(DeviceLogEntry)
	// stateSink 设备状态变更回调（ACQ-010/STB-003）：
	// adapter 在 OnReadLoopExit 等异步状态变化时调用，
	// 由 main.go 注入为 hub.EmitDeviceState，最终通过
	// DeviceService.EmitDeviceState 推送 daq:device-state 事件到前端。
	stateSink func(deviceID string, state core.DeviceState)

	// bpWarnLastMs per-device 背压 warn 限频时间戳。
	// 用 mu 保护（directSink 内已 RLock 拿 sink，但 bpWarnLastMs 写入需 Lock）；
	// 实际上 directSink 调 maybeWarnBackpressure 时不持 a.mu，
	// 这里独立用 bpMu 保护，避免与 a.mu 嵌套死锁。
	bpMu         sync.Mutex
	bpWarnLastMs map[string]int64
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
		drivers:      make(map[string]*sharedhw.DAQT1603),
		status:       make(map[string]*core.DeviceState),
		sinks:        make(map[string]func(core.TemperatureSnapshot)),
		channels:     make(map[string]chan core.TemperatureSnapshot),
		stopChs:      make(map[string]chan struct{}),
		operations:   make(map[string]acquisitionOperation),
		bpWarnLastMs: make(map[string]int64),
	}
}

func (a *T1603Adapter) SetLogSink(sink func(DeviceLogEntry)) {
	a.mu.Lock()
	a.logSink = sink
	a.mu.Unlock()
}

// SetStateSink 注入设备状态变更回调（ACQ-010/STB-003）。
// 由 main.go 在 hub 就绪后注入，让 adapter 在 OnReadLoopExit 等异步状态变化时
// 通过 hub.EmitDeviceState 推送到前端，避免物理断网后前端 statusMap 不更新。
func (a *T1603Adapter) SetStateSink(sink func(deviceID string, state core.DeviceState)) {
	a.mu.Lock()
	a.stateSink = sink
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

// emitState 在锁外推送设备状态变更（ACQ-010/STB-003）。
// 调用前需先在锁内复制一份 DeviceState（避免锁外读取竞态），
// 然后释放锁再调用此方法。
func (a *T1603Adapter) emitState(deviceID string, state core.DeviceState) {
	a.mu.RLock()
	sink := a.stateSink
	a.mu.RUnlock()
	if sink != nil {
		sink(deviceID, state)
	}
}

// maybeWarnBackpressure 在采集 channel 水位过高时发 per-device 限频 warn 日志。
//
// 限频策略：每设备独立计数，10 台同时背压时各自每
// adapterBackpressureWarnIntervalMs 最多 1 条 warn。
// 用独立 bpMu 保护，避免与 a.mu 嵌套死锁。
func (a *T1603Adapter) maybeWarnBackpressure(deviceID string, queueLen, queueCap int) {
	now := core.TimestampMs()
	a.bpMu.Lock()
	last := a.bpWarnLastMs[deviceID]
	if now-last < adapterBackpressureWarnIntervalMs {
		a.bpMu.Unlock()
		return
	}
	a.bpWarnLastMs[deviceID] = now
	a.bpMu.Unlock()

	a.emitLog(DeviceLogEntry{
		Level:    "warn",
		Category: "acquisition",
		DeviceID: deviceID,
		Message:  "采集 channel 水位过高，relay 消费异常",
		Detail:   fmt.Sprintf("queueLen=%d queueCap=%d deviceID=%s", queueLen, queueCap, deviceID),
	})
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
	if _, exists := a.drivers[profile.ID]; exists {
		a.mu.Unlock()
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	a.mu.Unlock()

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
		// 硬件通信日志（hardware-send/hardware-recv）在驱动层是 debug 级别，
		// 前端默认只显示 info 及以上，导致"通信"分组看不到任何记录。
		// 在此提升为 info，让操作员在前端日志面板可见完整的命令交互流程。
		level := entry.Level
		if (entry.Category == "hardware-send" || entry.Category == "hardware-recv") && level == "debug" {
			level = "info"
		}
		a.emitLog(DeviceLogEntry{
			Level:    level,
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
		delete(a.operations, profile.ID)
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
		// 在锁内复制一份 DeviceState，供锁外 emitState 使用，避免回调读取竞态。
		stateCopy := *st
		a.mu.Unlock()

		// emitLog 在锁外调用：emitLog 内部 RLock a.mu，与上文持有的 Lock 互斥。
		// 此日志对操作员可见"为什么下次 Start 报 not connected"，是排查断线/卡顿问题的关键线索。
		a.emitLog(DeviceLogEntry{
			Level:    "warn",
			Category: "system",
			DeviceID: profile.ID,
			Message:  "Device disconnected due to readLoop exit",
			Detail:   err.Error(),
		})

		// ACQ-010/STB-003：异步断线时主动推送状态到前端，
		// 让 statusMap 从「采集中」直接变为「未连接」，避免依赖轮询。
		a.emitState(profile.ID, stateCopy)

		if driver != nil {
			driver.Disconnect()
		}
	})

	if err := dev.Connect(); err != nil {
		a.emitLog(DeviceLogEntry{
			Level: "error", Category: "hardware-recv", DeviceID: profile.ID,
			Message: fmt.Sprintf("设备 [%s] TCP 连接失败", profile.Name),
			Detail:  fmt.Sprintf("%s:%d — %v", profile.Address, profile.Port, err),
		})
		return fmt.Errorf("connect device %s: %w", profile.ID, err)
	}
	// TCP 连接成功，打印硬件通信日志（前端"通信"分组可见）
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: profile.ID,
		Message: "TCP connected", Detail: fmt.Sprintf("%s:%d", profile.Address, profile.Port),
	})
	if cfg, err := dev.GetDaqT1603Config(); err == nil {
		profile.T1603Cfg.SamplingRate = spsMsToHz(cfg.SamplingRate) // 采集间隔毫秒 → Hz
		profile.SamplingRate = profile.T1603Cfg.SamplingRate
		profile.T1603Cfg.ChannelMask = cfg.ChannelMask
		profile.T1603Cfg.AverageCount = cfg.AverageCount
		profile.T1603Cfg.ShowTimestamp = cfg.ShowTimestamp
		profile.T1603Cfg.ShowSequence = cfg.ShowSequence
		profile.T1603Cfg.ThermocoupleTypes = cfg.ThermocoupleTypes
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.drivers[profile.ID]; exists {
		_ = dev.Disconnect()
		return fmt.Errorf("device %s already connected", profile.ID)
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
	delete(a.operations, id)
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
	// TCP 断开日志（通信层），归类到 hardware-recv 便于前端"通信"分组展示
	a.emitLog(DeviceLogEntry{
		Level: "info", Category: "hardware-recv", DeviceID: id,
		Message: "TCP disconnected",
	})
	return stopErr
}

func (a *T1603Adapter) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	a.mu.Lock()
	if operation, exists := a.operations[id]; exists {
		a.mu.Unlock()
		if operation == acquisitionOperationStopping {
			return nil, fmt.Errorf("device %s stop in progress", id)
		}
		return nil, fmt.Errorf("device %s start in progress", id)
	}
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := a.channels[id]; exists {
		a.mu.Unlock()
		return nil, fmt.Errorf("device %s already acquiring", id)
	}

	// channel 缓冲区大小：adapterChannelCap 帧，在 1000Hz 下提供约 65 秒缓冲，
	// 多设备场景下确保 relay 偶发卡顿时不会立即反压到硬件 readLoop
	ch := make(chan core.TemperatureSnapshot, adapterChannelCap)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done
	a.operations[id] = acquisitionOperationStarting

	directSink := func(snapshot core.TemperatureSnapshot) {
		// 水位监控：超过阈值时发限频 warn 日志。
		// 不丢数据（仍 blocking send），因为 device 侧丢帧比 relay 阻塞更糟；
		// 此处仅做可观测，让操作员感知 relay 消费异常。
		if l := len(ch); l >= adapterChannelBackpressureThreshold {
			a.maybeWarnBackpressure(id, l, cap(ch))
		}
		select {
		case ch <- snapshot:
		case <-done:
		}
	}
	a.sinks[id] = directSink

	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusStarting)
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
		delete(a.operations, id)
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

	a.mu.Lock()
	delete(a.operations, id)
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	a.mu.Unlock()

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
	if operation, exists := a.operations[id]; exists {
		a.mu.Unlock()
		if operation == acquisitionOperationStarting {
			return fmt.Errorf("device %s start in progress", id)
		}
		return fmt.Errorf("device %s stop in progress", id)
	}
	dev, ok := a.drivers[id]
	if !ok {
		a.mu.Unlock()
		return nil
	}
	if done, exists := a.stopChs[id]; exists {
		close(done)
	}
	delete(a.sinks, id)
	chToClose, hasCh := a.channels[id]
	a.operations[id] = acquisitionOperationStopping
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusStopping)
	}
	a.mu.Unlock()

	stopErr := dev.StopAcquisition()
	a.mu.Lock()
	delete(a.operations, id)
	delete(a.stopChs, id)
	delete(a.channels, id)
	if st, exists := a.status[id]; exists {
		if stopErr == nil {
			st.SetStatus(core.StatusConnected)
			st.AcquiringAt = 0
		} else {
			st.SetStatus(core.StatusError)
			st.Error = stopErr.Error()
		}
	}
	a.mu.Unlock()
	if hasCh {
		close(chToClose)
	}
	return stopErr
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
		if _, operating := a.operations[id]; operating {
			return *st, true
		}
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
