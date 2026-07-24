package usecase

import (
	"errors"
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"strconv"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

type scanPending struct {
	results []device.ScanResult
	err     error
	done    chan struct{}
}

type DeviceManager struct {
	mu           sync.RWMutex
	profiles     []device.Profile
	devices      map[string]ports.Device
	store        ports.ProfileStore
	factory      ports.DeviceFactory
	scanner      ports.DeviceScanner
	normalizer   ports.ProfileNormalizer
	dataSink     device.DataSink
	scanInFlight *scanPending

	// connMu 序列化同一 device id 上的 Connect/Disconnect/DeleteProfile，
	// 防止重连场景下两个 goroutine 同时操作同一物理设备（TCP/串口设备
	// 只允许一个会话）。与 m.mu 配合使用：m.mu 保护内存结构，connMu
	// 保护硬件 I/O。
	connMuRegistry sync.Map // map[string]*sync.Mutex
}

func NewDeviceManager(store ports.ProfileStore, factory ports.DeviceFactory, dataSink device.DataSink) (*DeviceManager, error) {
	return NewDeviceManagerWithNormalizer(store, factory, dataSink, nil)
}

// NewDeviceManagerWithNormalizer 创建 DeviceManager 并注入配置规范化器。
// normalizer 可为 nil（跳过规范化）；由装配根注入 adapters/config 实现。
func NewDeviceManagerWithNormalizer(store ports.ProfileStore, factory ports.DeviceFactory, dataSink device.DataSink, normalizer ports.ProfileNormalizer) (*DeviceManager, error) {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return nil, err
	}
	profiles = normalizeProfiles(profiles, normalizer)
	return &DeviceManager{
		profiles:   profiles,
		devices:    make(map[string]ports.Device),
		store:      store,
		factory:    factory,
		normalizer: normalizer,
		dataSink:   dataSink,
	}, nil
}

// connMu 返回某 device id 的连接互斥锁，首次访问时惰性创建。
// 用于 Connect/Disconnect/DeleteProfile 之间的串行化，避免对同一物理设备
// 并发执行硬件 I/O。
func (m *DeviceManager) connMu(id string) *sync.Mutex {
	if mu, ok := m.connMuRegistry.Load(id); ok {
		return mu.(*sync.Mutex)
	}
	actual, _ := m.connMuRegistry.LoadOrStore(id, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (m *DeviceManager) SetScanner(scanner ports.DeviceScanner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scanner = scanner
}

func (m *DeviceManager) ScanDevices() ([]device.ScanResult, error) {
	m.mu.Lock()
	scanner := m.scanner
	if scanner == nil {
		m.mu.Unlock()
		return []device.ScanResult{}, nil
	}
	if m.scanInFlight != nil {
		pending := m.scanInFlight
		m.mu.Unlock()
		<-pending.done
		return pending.results, pending.err
	}
	pending := &scanPending{done: make(chan struct{})}
	m.scanInFlight = pending
	m.mu.Unlock()

	pending.results, pending.err = scanner.Scan()
	close(pending.done)

	m.mu.Lock()
	if m.scanInFlight == pending {
		m.scanInFlight = nil
	}
	m.mu.Unlock()
	return pending.results, pending.err
}

func (m *DeviceManager) GetProfiles() []device.Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]device.Profile(nil), m.profiles...)
}

func (m *DeviceManager) UpsertProfile(profile device.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.normalizer != nil {
		profile = m.normalizer.Normalize(profile)
	}
	for i := range m.profiles {
		if m.profiles[i].ID == profile.ID {
			if profile.Type == device.DeviceDAQP1604 {
				prevUnit := firstChannelUnit(m.profiles[i].Channels)
				nextUnit := firstChannelUnit(profile.Channels)
				if nextUnit != "" && nextUnit != prevUnit {
					if dev, ok := m.devices[profile.ID]; ok {
						if dev.Status().Acquiring {
							return fmt.Errorf("cannot update DAQ-P-1604 unit while acquiring: %s", profile.ID)
						}
						configurable, ok := dev.(ports.UnitConfigurable)
						if !ok {
							return fmt.Errorf("device does not support unit configuration: %s", profile.ID)
						}
						if err := configurable.SetUnit(nextUnit); err != nil {
							return err
						}
					}
				}
			}
			m.profiles[i] = profile
			return m.store.SaveProfiles(m.profiles)
		}
	}
	m.profiles = append(m.profiles, profile)
	return m.store.SaveProfiles(m.profiles)
}

func firstChannelUnit(channels []device.ChannelConfig) string {
	for _, channel := range channels {
		unit := strings.TrimSpace(channel.Unit)
		if unit != "" {
			return unit
		}
	}
	return ""
}

func normalizeProfiles(profiles []device.Profile, normalizer ports.ProfileNormalizer) []device.Profile {
	if normalizer == nil {
		return profiles
	}
	normalized := make([]device.Profile, len(profiles))
	for i := range profiles {
		normalized[i] = normalizer.Normalize(profiles[i])
	}
	return normalized
}

// DeleteProfile 删除设备配置（并发安全）。
// 顺序：先持久化（可失败可重试），后修改运行时状态，最后断开硬件。
// 失败回滚保证：SaveProfiles 失败时 m.profiles / m.devices 不变；
// 即使中途崩溃，磁盘与运行时也不会出现不一致。
func (m *DeviceManager) DeleteProfile(id string) error {
	connMu := m.connMu(id)
	connMu.Lock()
	defer connMu.Unlock()

	// Phase 1: 在锁内构造去除目标 ID 后的新切片（不修改 m.profiles）。
	m.mu.Lock()
	found := false
	nextProfiles := make([]device.Profile, 0, len(m.profiles))
	for _, profile := range m.profiles {
		if profile.ID == id {
			found = true
			continue
		}
		nextProfiles = append(nextProfiles, profile)
	}
	if !found {
		m.mu.Unlock()
		return fmt.Errorf("device profile not found: %s", id)
	}
	m.mu.Unlock()

	// Phase 2: 锁外持久化（耗时 I/O，不阻塞其他设备的读操作）。
	// 失败时不修改任何运行时状态，调用方可重试。
	if err := m.store.SaveProfiles(nextProfiles); err != nil {
		return err
	}

	// Phase 3: 持久化成功，提交运行时状态变更。
	m.mu.Lock()
	m.profiles = nextProfiles
	devToDisconnect := m.devices[id]
	delete(m.devices, id)
	m.mu.Unlock()

	// Phase 4: 断开硬件（在 connMu 保护下，与同 id 的 Connect/Disconnect 互斥）。
	if devToDisconnect != nil {
		if err := devToDisconnect.StopAcquisition(); err != nil {
			slog.Warn("DeviceManager.DeleteProfile: StopAcquisition failed",
				"id", id, "error", err)
		}
		if err := devToDisconnect.Disconnect(); err != nil {
			slog.Warn("DeviceManager.DeleteProfile: Disconnect failed",
				"id", id, "error", err)
		}
	}
	return nil
}

func (m *DeviceManager) SetUnit(id string, unit string) error {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return fmt.Errorf("unit is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		configurable, ok := dev.(ports.UnitConfigurable)
		if !ok {
			return fmt.Errorf("device does not support unit configuration: %s", id)
		}
		if err := configurable.SetUnit(unit); err != nil {
			return err
		}
	}
	for i := range m.profiles[profileIndex].Channels {
		m.profiles[profileIndex].Channels[i].Unit = unit
	}
	return m.store.SaveProfiles(m.profiles)
}

func (m *DeviceManager) GetDaqT1603Config(id string) (device.DaqT1603HardwareConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.DaqT1603Configurable); ok {
			return configurable.GetDaqT1603Config()
		}
	}
	profile, ok := m.findProfileLocked(id)
	if !ok {
		return device.DaqT1603HardwareConfig{}, fmt.Errorf("device profile not found: %s", id)
	}
	return profile.DaqT1603Config, nil
}

func (m *DeviceManager) ApplyDaqT1603Config(id string, config device.DaqT1603HardwareConfig) error {
	if err := validateDaqT1603Config(config); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		configurable, ok := dev.(ports.DaqT1603Configurable)
		if !ok {
			return fmt.Errorf("device does not support DAQ-T-1603 configuration: %s", id)
		}
		if err := configurable.ApplyDaqT1603Config(config); err != nil {
			return err
		}
	}
	m.profiles[profileIndex].DaqT1603Config = config
	return m.store.SaveProfiles(m.profiles)
}

// GetDsa3217ScanConfig 获取 DSA3217 设备的扫描配置
func (m *DeviceManager) GetDsa3217ScanConfig(id string) (device.DSA3217ScanConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.DSA3217Configurable); ok {
			return configurable.GetDsa3217ScanConfig()
		}
	}
	return device.DSA3217ScanConfig{}, fmt.Errorf("device not connected or does not support DSA3217 configuration: %s", id)
}

// ApplyDsa3217ScanConfig 写入 DSA3217 扫描配置并回读验证
func (m *DeviceManager) ApplyDsa3217ScanConfig(id string, avg int, period int) (device.DSA3217ScanConfig, error) {
	if avg < 1 || avg > 240 {
		return device.DSA3217ScanConfig{}, fmt.Errorf("AVG must be between 1 and 240")
	}
	if period < 73 || period > 65535 {
		return device.DSA3217ScanConfig{}, fmt.Errorf("PERIOD must be between 73 and 65535")
	}

	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()

	if !ok {
		return device.DSA3217ScanConfig{}, fmt.Errorf("device not connected: %s", id)
	}
	configurable, ok := dev.(ports.DSA3217Configurable)
	if !ok {
		return device.DSA3217ScanConfig{}, fmt.Errorf("device does not support DSA3217 configuration: %s", id)
	}
	return configurable.ApplyDsa3217ScanConfig(avg, period)
}

// GetDAQP1603Config 获取 DAQ-P-1603 设备的当前配置 profile。
//
// 已连接设备：通过 DAQP1603Configurable 接口从驱动获取最新 profile（含硬件实际值），
//   驱动返回的是 profile 拷贝，外部修改不会污染内部状态。
// 未连接设备：返回持久化的 profile（与 m.profiles 中保持一致）。
//
// 用于前端在打开配置面板时回显当前实际生效的配置。
func (m *DeviceManager) GetDAQP1603Config(id string) (device.Profile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.DAQP1603Configurable); ok {
			return configurable.GetDAQP1603Config()
		}
	}
	profile, ok := m.findProfileLocked(id)
	if !ok {
		return device.Profile{}, fmt.Errorf("device profile not found: %s", id)
	}
	return profile, nil
}

// ApplyDAQP1603Config 同步 DAQ-P-1603 配置到硬件并回读验证。
//
// 已连接设备：通过 DAQP1603Configurable 接口执行 ReleaseTask → VerifyParam → InitTask
//
//	重新初始化任务（DLL 不支持热更新参数），随后回读 profile 验证生效值。
//
// 未连接设备：返回错误（无法同步到未连接设备）。
// 注意：此方法不更新 m.profiles，调用方应通过 upsertProfile 持久化配置。
// 设计依据：与 ApplyDsa3217ScanConfig 对齐，前端 saveDraft 流程为
//
//	upsertProfile（持久化）→ applyDaqP1603Config（同步硬件 + 回读）。
func (m *DeviceManager) ApplyDAQP1603Config(id string, profile device.Profile) (device.Profile, error) {
	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()

	if !ok {
		return device.Profile{}, fmt.Errorf("device not connected: %s", id)
	}
	configurable, ok := dev.(ports.DAQP1603Configurable)
	if !ok {
		return device.Profile{}, fmt.Errorf("device does not support DAQ-P-1603 configuration: %s", id)
	}
	if err := configurable.ApplyDAQP1603Config(profile); err != nil {
		return device.Profile{}, err
	}
	// 回读 driver 内部 profile（已被 ApplyDAQP1603Config 更新），
	// 让前端能拿到硬件实际生效的采样率与通道配置。
	return configurable.GetDAQP1603Config()
}

// Connect 连接设备（并发安全）。
// 通过 per-id 互斥锁串行化同一设备的 Connect/Disconnect/DeleteProfile，
// 避免对同一物理设备并发执行硬件 I/O（TCP/串口设备只允许一个会话）。
// 不同设备 id 之间仍然并行。
//
// 内部仍采用三阶段：
//
//	Phase 1: 检查是否已连接、查找 profile（m.mu RLock）。
//	Phase 2: 创建适配器 + 硬件连接（耗时操作，在 connMu 保护下进行）。
//	Phase 3: 原子写入 devices map（m.mu Lock）。
func (m *DeviceManager) Connect(id string) error {
	connMu := m.connMu(id)
	connMu.Lock()
	defer connMu.Unlock()

	// Phase 1: 锁内快速检查
	m.mu.RLock()
	if _, ok := m.devices[id]; ok {
		m.mu.RUnlock()
		return nil // 已连接，幂等返回
	}
	profile, ok := m.findProfileLocked(id)
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}

	// Phase 2: connMu 保护下执行耗时操作（创建适配器 + TCP 连接等 I/O）
	dev, err := m.factory.Create(profile)
	if err != nil {
		return err
	}
	if m.dataSink != nil {
		dev.SetDataSink(m.dataSink)
	}
	if err := dev.Connect(); err != nil {
		return err
	}

	// Phase 3: 锁内原子写入 devices map
	m.mu.Lock()

	// connMu 已保证同 id 不会并发 Connect，但保留 existing 检查作为防御性兜底；
	// 若意外发生（如未来代码路径绕过 connMu），丢弃新连接并显式记日志。
	// 注意：onError 回调必须在 dev 真正写入 map 之后才注册，否则
	// 丢弃 dev 时触发的 readLoop 退出会执行回调，把 existing 设备从 map 中误删。
	if existing, ok := m.devices[id]; ok && existing != nil {
		m.mu.Unlock()
		if disconnectErr := dev.Disconnect(); disconnectErr != nil {
			slog.Warn("DeviceManager.Connect: discarding new connection failed to disconnect cleanly, resource may leak",
				"id", id, "error", disconnectErr)
		}
		return nil
	}
	m.devices[id] = dev
	m.mu.Unlock()

	// Phase 4: dev 已被 manager 接管，此时再注册异常退出回调。
	// 回调内通过 identity 比较（m.devices[id] == dev）确保只在 dev 仍是当前注册项时才删除，
	// 避免后续 Connect 替换 map 项后被旧实例的回调误删。
	if notifiable, ok := dev.(ports.ErrorNotifiable); ok {
		deviceID := profile.ID
		notifiable.SetOnError(func(err error) {
			slog.Warn("DeviceManager: 设备异常退出", "device", deviceID, "error", err)
			m.mu.Lock()
			if current, ok := m.devices[deviceID]; ok && current == dev {
				delete(m.devices, deviceID)
			}
			m.mu.Unlock()
		})
	}

	return nil
}

// Disconnect 断开设备连接（并发安全）。
// 通过 per-id 互斥锁串行化同一设备的 Connect/Disconnect/DeleteProfile。
//
// 内部两阶段：
//
//	Phase 1 (锁内): 从 devices map 中原子删除。
//	Phase 2 (connMu 下): 停止采集 + 断开硬件。与同 id 的 Connect 互斥。
//
// 返回 StopAcquisition 和 Disconnect 的合并错误。
func (m *DeviceManager) Disconnect(id string) error {
	connMu := m.connMu(id)
	connMu.Lock()
	defer connMu.Unlock()

	// Phase 1: 锁内原子删除
	m.mu.Lock()
	dev, ok := m.devices[id]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.devices, id)
	m.mu.Unlock()

	// Phase 2: connMu 保护下执行耗时操作（停止采集 + 断开硬件连接）
	var errs []error
	if err := dev.StopAcquisition(); err != nil {
		errs = append(errs, err)
	}
	if err := dev.Disconnect(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// StartAcquisition 启动设备采集（并发安全）。
// 通过 per-id connMu 与 Connect/Disconnect/DeleteProfile 互斥，
// 避免 Disconnect 已从 devices map 中移除设备、即将销毁底层连接的窗口期，
// 在同一对象上启动一次 manager 已不再追踪的采集。
func (m *DeviceManager) StartAcquisition(id string) error {
	connMu := m.connMu(id)
	connMu.Lock()
	defer connMu.Unlock()

	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("device not connected: %s", id)
	}
	return dev.StartAcquisition()
}

// StopAcquisition 停止设备采集（并发安全）。
// 同 StartAcquisition：通过 connMu 与连接生命周期操作互斥。
func (m *DeviceManager) StopAcquisition(id string) error {
	connMu := m.connMu(id)
	connMu.Lock()
	defer connMu.Unlock()

	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return dev.StopAcquisition()
}

func (m *DeviceManager) GetStatus(id string) (device.Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dev, ok := m.devices[id]
	if !ok {
		return device.Status{}, false
	}
	return dev.Status(), true
}

func (m *DeviceManager) findProfileLocked(id string) (device.Profile, bool) {
	for _, profile := range m.profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return device.Profile{}, false
}

func (m *DeviceManager) findProfileIndexLocked(id string) (int, bool) {
	for i := range m.profiles {
		if m.profiles[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

func (m *DeviceManager) SetTare(id string, channelIndex int, offset float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.TareConfigurable); ok {
			if err := configurable.SetTare(channelIndex, offset); err != nil {
				return err
			}
		}
	}
	if channelIndex >= 0 && channelIndex < len(m.profiles[profileIndex].Channels) {
		m.profiles[profileIndex].Channels[channelIndex].TareOffset = offset
	}
	return m.store.SaveProfiles(m.profiles)
}

func (m *DeviceManager) GetTare(id string, channelIndex int) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.TareConfigurable); ok {
			return configurable.GetTare(channelIndex)
		}
	}
	profile, ok := m.findProfileLocked(id)
	if !ok {
		return 0, fmt.Errorf("device profile not found: %s", id)
	}
	if channelIndex < 0 || channelIndex >= len(profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return profile.Channels[channelIndex].TareOffset, nil
}

func (m *DeviceManager) ClearTare(id string, channelIndex int) error {
	return m.SetTare(id, channelIndex, 0)
}

func validateDaqT1603Config(config device.DaqT1603HardwareConfig) error {
	if strings.TrimSpace(config.ThermocoupleTypes) == "" {
		return fmt.Errorf("thermocoupleTypes is required")
	}
	if len(config.ThermocoupleTypes) != 16 {
		return fmt.Errorf("thermocoupleTypes must be exactly 16 characters (one per channel)")
	}
	validTypes := "KJTENRSB"
	for _, c := range config.ThermocoupleTypes {
		if !strings.ContainsRune(validTypes, c) {
			return fmt.Errorf("thermocoupleTypes contains invalid type %q; valid: K,J,T,E,N,R,S,B", c)
		}
	}
	if config.ChannelMask != "" {
		if _, err := strconv.ParseUint(config.ChannelMask, 16, 16); err != nil {
			return fmt.Errorf("channelMask must be a hex value in 0000-FFFF range")
		}
	}
	if config.SamplingRate <= 0 {
		return fmt.Errorf("samplingRate must be greater than zero")
	}
	if config.AverageCount < 1 || config.AverageCount > 100 {
		return fmt.Errorf("averageCount must be between 1 and 100")
	}
	if config.TriggerMode != 0 && config.TriggerMode != 2 {
		return fmt.Errorf("triggerMode must be 0 (software) or 2 (hardware)")
	}
	if config.TriggerEdge < 0 || config.TriggerEdge > 2 {
		return fmt.Errorf("triggerEdge must be 0 (rising), 1 (falling), or 2 (change)")
	}
	if config.TriggerCount < 0 {
		return fmt.Errorf("triggerCount must be non-negative")
	}
	if config.OpenCircuitCheck != "" {
		if _, err := strconv.ParseUint(config.OpenCircuitCheck, 16, 16); err != nil {
			return fmt.Errorf("openCircuitCheck must be a hex mask in 0000-FFFF range")
		}
	}
	return nil
}
