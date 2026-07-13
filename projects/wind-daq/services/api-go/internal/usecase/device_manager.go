package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

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

	// v2 校零组件
	calApplier    *CalibrationApplier
	calSampler    *CalibrationSampler
	calibrating   map[string]struct{}
	calProgress   map[string]device.CalibrationProgress
	calProgressMu sync.RWMutex

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
	loadedProfiles, err := store.LoadProfiles()
	if err != nil {
		return nil, err
	}
	profiles := normalizeProfiles(loadedProfiles, normalizer)
	if !reflect.DeepEqual(loadedProfiles, profiles) {
		if err := store.SaveProfiles(profiles); err != nil {
			return nil, fmt.Errorf("persist normalized profiles: %w", err)
		}
	}
	return &DeviceManager{
		profiles:    profiles,
		devices:     make(map[string]ports.Device),
		store:       store,
		factory:     factory,
		normalizer:  normalizer,
		dataSink:    dataSink,
		calibrating: make(map[string]struct{}),
		calProgress: make(map[string]device.CalibrationProgress),
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
					// 防御：校零进行中切换单位会触发 Calibrate 阶段 3 的 TOCTOU 静默丢弃，
					// 此处提前拒绝，与 SetUnit 行为一致。
					if _, running := m.calibrating[profile.ID]; running {
						return fmt.Errorf("cannot change unit while calibration is in progress")
					}
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
			if err := m.store.SaveProfiles(m.profiles); err != nil {
				return err
			}
			m.loadCalibrationOffsetsLocked(profile.ID)
			return nil
		}
	}
	m.profiles = append(m.profiles, profile)
	if err := m.store.SaveProfiles(m.profiles); err != nil {
		return err
	}
	m.loadCalibrationOffsetsLocked(profile.ID)
	return nil
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
	if m.calApplier != nil {
		m.calApplier.RemoveDevice(id)
	}
	// P2-14：清理校零运行时状态，避免删除后残留：
	//   - calibrating[id]：若删除时正在校零，Calibrate 的 defer 会再次 delete（幂等），
	//     但若不清理，同 ID 新设备复用时会误判"校零进行中"。
	//   - calProgress[id]：Calibrate defer 会写回 {Running:false,...} 重新创建条目，
	//     属轻微泄漏；此处清理后即使被写回，下次 Calibrate 会覆盖，无功能影响。
	delete(m.calibrating, id)
	m.mu.Unlock()
	m.calProgressMu.Lock()
	delete(m.calProgress, id)
	m.calProgressMu.Unlock()

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

	// 防御：校零进行中切换单位会导致 TOCTOU —— Calibrate 阶段 3 检测到单位变化
	// 会静默丢弃该通道的偏移结果，用户收到 200 OK 但偏移未生效且无显式错误。
	// 此处提前拒绝，让用户收到明确错误反馈。
	if _, running := m.calibrating[id]; running {
		return fmt.Errorf("cannot change unit while calibration is in progress")
	}

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
//
//	驱动返回的是 profile 拷贝，外部修改不会污染内部状态。
//
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

// ChannelUnit 实现 ports.ChannelUnitProvider。
//
// 为什么这样设计：遍历测试压力归一化（BuildRawPressure）需要查每个通道的 Unit
// 才能换算到 Pa，但 TraversalManager 不能直接依赖 usecase 兄弟包，通过此方法
// 提供"设备→通道→Unit"的窄查询。从 profiles 中查找，找不到返回 error 让调用方
// 走降级路径（跳过换算记 warning）。
//
// 注意：调用方应容忍 error（如设备未连接或通道不存在），不应中断遍历流程。
func (m *DeviceManager) ChannelUnit(deviceID string, channelIndex int) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	profile, ok := m.findProfileLocked(deviceID)
	if !ok {
		return "", fmt.Errorf("device profile not found: %s", deviceID)
	}
	for _, ch := range profile.Channels {
		if ch.Index == channelIndex {
			return ch.Unit, nil
		}
	}
	return "", fmt.Errorf("channel %d not found in device %s", channelIndex, deviceID)
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

// IsConnected 实现 ports.AcquisitionController。
// 复用 GetStatus 查询，避免在端口适配层重复设备 map 锁逻辑。
//
// "已连接"语义：非 Disconnected 且非 Error 即视为已连接。
// 不能用 == ConnectionConnected 判断——adapter 在 StartAcquisition 时会把
// Connection 改为 ConnectionAcquiring，若严格要求 Connected 会导致设备正在采集时
// IsConnected 误报 false，进而让 CheckPreconditions 的 DeviceConnected 项在
// 设备已采集场景下显示红色"未连接"。
func (m *DeviceManager) IsConnected(id string) bool {
	status, ok := m.GetStatus(id)
	if !ok {
		return false
	}
	return status.Connection != device.ConnectionDisconnected && status.Connection != device.ConnectionError
}

// IsAcquiring 实现 ports.AcquisitionController。
// 仅当设备存在且 Status.Acquiring=true 时返回 true。
// 遍历测试 CheckPreconditions 据此真实反映"是否正在持续产帧"。
func (m *DeviceManager) IsAcquiring(id string) bool {
	status, ok := m.GetStatus(id)
	if !ok {
		return false
	}
	return status.Acquiring
}

// 编译期断言：DeviceManager 实现 ports.AcquisitionController。
// StartAcquisition 已存在（device_manager.go:569），加上 IsConnected/IsAcquiring 即满足接口。
var _ ports.AcquisitionController = (*DeviceManager)(nil)

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

// ---- v2 校零方法 ----

// SetCalibrationComponents 注入校零组件（由 bootstrap 调用）。
// 注入后自动遍历已加载 profiles，将存量 CalibrationOffset 写入 calApplier 快照，
// 避免重启后已校零设备在下次 Calibrate 前偏移不生效（冷启动问题）。
//
// 锁策略：整段持写锁完成。原先先 Lock 写组件再释放后 RLock 读 profiles，
// 释放与重锁之间存在窗口，并发 LoadProfiles / UpsertProfile 可能修改 profiles，
// 导致同步到 applier 的偏移已过期。合并为单次 Lock 消除 TOCTOU。
// applier.UpdateOffsets 内部自有独立锁，不会与 m.mu 形成死锁。
func (m *DeviceManager) SetCalibrationComponents(applier *CalibrationApplier, sampler *CalibrationSampler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calApplier = applier
	m.calSampler = sampler

	// 冷启动：将已加载 profile 中的校零偏移同步到 CalibrationApplier
	if applier != nil {
		for _, p := range m.profiles {
			offsets := make(device.CalibrationOffsets)
			for _, ch := range p.Channels {
				if ch.CalibrationOffset != 0 {
					offsets[ch.Index] = ch.CalibrationOffset
				}
			}
			if len(offsets) > 0 {
				applier.UpdateOffsets(p.ID, offsets)
			}
		}
	}
}

// UpdateDataSink 更新 dataSink 回调（由 bootstrap 在 manager 创建后调用，
// 注入含校零功能的 dataSink）。
func (m *DeviceManager) UpdateDataSink(sink device.DataSink) {
	m.mu.Lock()
	m.dataSink = sink
	m.mu.Unlock()
}

// GetChannelsForCalibration 获取设备的通道配置（供 data_sink CalibrationApplier 使用）。
// 返回深拷贝，避免 data_sink 高频并发读写与 SetUnit/UpsertProfile 的写操作竞争底层数组。
func (m *DeviceManager) GetChannelsForCalibration(deviceID string) []device.ChannelConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.profiles {
		if m.profiles[i].ID == deviceID {
			cp := make([]device.ChannelConfig, len(m.profiles[i].Channels))
			copy(cp, m.profiles[i].Channels)
			return cp
		}
	}
	return nil
}

// loadCalibrationOffsetsLocked 从 m.profiles 加载指定设备的校零偏移快照到 CalibrationApplier。
// 调用方必须持有 m.mu 读锁。
func (m *DeviceManager) loadCalibrationOffsetsLocked(deviceID string) {
	if m.calApplier == nil {
		return
	}
	for i := range m.profiles {
		if m.profiles[i].ID == deviceID {
			offsets := make(device.CalibrationOffsets)
			for _, ch := range m.profiles[i].Channels {
				if ch.CalibrationOffset != 0 {
					offsets[ch.Index] = ch.CalibrationOffset
				}
			}
			m.calApplier.UpdateOffsets(deviceID, offsets)
			return
		}
	}
}

// Calibrate 执行校零：订阅 hub 5 秒取均值 → 转为基单位 → 落库 → 更新 CalibrationApplier 快照。
// 所有启用通道并行采样，耗时固定 5 秒。
//
// 取消机制：依赖传入的 ctx（HTTP 层为 r.Context()）链式取消。
// 兜底机制：额外派生 sampleDuration + 2s 的 deadline context，防御 HTTP
// keep-alive 或代理缓冲导致 r.Context() 延迟/丢失连接断开信号的场景，
// 避免后端无限期占用硬件资源。
func (m *DeviceManager) Calibrate(id string, ctx context.Context, targetChannel *int) ([]device.CalibrationResult, error) {
	if m.calSampler == nil {
		return nil, fmt.Errorf("calibration sampler not available")
	}
	// 兜底 deadline：采样时长 + 2s 缓冲（落库 + 网络往返）。
	// 超过此时间即使 ctx 未 cancel 也强制终止，防止硬件资源被无限占用。
	sampleDuration := m.calSampler.duration
	fallbackDeadline := sampleDuration + 2*time.Second
	ctx, cancel := context.WithTimeout(ctx, fallbackDeadline)
	defer cancel()

	m.mu.Lock()
	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("device profile not found: %s", id)
	}
	dev, connected := m.devices[id]
	if !connected || !dev.Status().Acquiring {
		m.mu.Unlock()
		return nil, fmt.Errorf("请先开始采集")
	}
	if _, running := m.calibrating[id]; running {
		m.mu.Unlock()
		return nil, fmt.Errorf("calibration already in progress")
	}
	if targetChannel != nil {
		if _, found := findChannelPosition(m.profiles[profileIndex].Channels, *targetChannel); !found {
			m.mu.Unlock()
			return nil, fmt.Errorf("invalid channel index: %d", *targetChannel)
		}
	}
	m.calibrating[id] = struct{}{}
	startedAt := time.Now()
	m.calProgressMu.Lock()
	m.calProgress[id] = device.CalibrationProgress{Running: true, ChannelIndex: targetChannel}
	m.calProgressMu.Unlock()
	// 深拷贝：Sample 会阻塞 5 秒，期间 SetUnit / UpsertProfile / SetTare 等写操作
	// 可能修改 m.profiles[profileIndex].Channels 的底层数组。若直接共享 slice header，
	// 采样器读到的数据会被并发修改（race），最坏情况读到半截新半截旧的不一致快照。
	// 与 GetChannelsForCalibration 保持一致的深拷贝策略。
	src := m.profiles[profileIndex].Channels
	channels := make([]device.ChannelConfig, len(src))
	copy(channels, src)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.calibrating, id)
		m.mu.Unlock()
		m.calProgressMu.Lock()
		progress := m.calProgress[id]
		progress.Running = false
		progress.ElapsedMs = time.Since(startedAt).Milliseconds()
		m.calProgress[id] = progress
		m.calProgressMu.Unlock()
	}()

	results, err := m.calSampler.Sample(ctx, id, channels, targetChannel, func(sampleCount int) {
		m.calProgressMu.Lock()
		m.calProgress[id] = device.CalibrationProgress{Running: true, ChannelIndex: targetChannel, ElapsedMs: time.Since(startedAt).Milliseconds(), SampleCount: sampleCount}
		m.calProgressMu.Unlock()
	})
	if err != nil {
		return nil, err
	}

	// 落库到 profile + 更新 CalibrationApplier 快照
	m.mu.Lock()
	defer m.mu.Unlock()

	pi, ok := m.findProfileIndexLocked(id)
	if !ok {
		return nil, fmt.Errorf("device profile not found: %s", id)
	}
	nextProfiles := cloneProfiles(m.profiles)
	for _, r := range results {
		found := false
		for j := range nextProfiles[pi].Channels {
			if nextProfiles[pi].Channels[j].Index == r.ChannelIndex {
				// TOCTOU 防御：Sample 阻塞 5 秒，期间 SetUnit 可能修改通道单位。
				// 若当前单位与采样时单位不一致，偏移值的物理含义已不匹配
				// （如采样时 kPa，当前变为 Pa，减偏移会得到 1000 倍错误值）。
				// 此时跳过该通道的落库，避免写入语义错误的偏移。
				if nextProfiles[pi].Channels[j].Unit != r.Unit {
					slog.Warn("calibrate unit changed during sampling, skip offset",
						"device", id,
						"channel", r.ChannelIndex,
						"sampledUnit", r.Unit,
						"currentUnit", nextProfiles[pi].Channels[j].Unit)
					found = true
					break
				}
				nextProfiles[pi].Channels[j].CalibrationOffset = r.Offset
				nextProfiles[pi].Channels[j].CalibrationUnit = r.Unit
				nextProfiles[pi].Channels[j].CalibrationAt = r.At
				// 用户主动校零此通道 = 希望应用偏移。
				// 此前不重置 CalibrationEnabled，导致用户先前通过 toggle 关闭后，
				// 即使重新校零落库了新 offset，Apply 阶段仍因 enabled=false 跳过，
				// 通道值不变化——表现为"校零使能不回来"。
				// 此处对成功采样的通道强制 enabled=true，让校零结果立即生效。
				nextProfiles[pi].Channels[j].CalibrationEnabled = true
				found = true
				break
			}
		}
		if !found {
			slog.Warn("calibrate result channel not in profile", "device", id, "channel", r.ChannelIndex)
			continue
		}
	}

	if err := m.store.SaveProfiles(nextProfiles); err != nil {
		return results, fmt.Errorf("save profiles after calibrate: %w", err)
	}
	m.profiles = nextProfiles
	m.loadCalibrationOffsetsLocked(id)
	return results, nil
}

func (m *DeviceManager) GetCalibrationProgress(id string) device.CalibrationProgress {
	m.calProgressMu.RLock()
	defer m.calProgressMu.RUnlock()
	return m.calProgress[id]
}

// GetCalibration 读取通道的完整校零记录。
func (m *DeviceManager) GetCalibration(id string, channelIndex int) (device.CalibrationRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profile, ok := m.findProfileLocked(id)
	if !ok {
		return device.CalibrationRecord{}, fmt.Errorf("device profile not found: %s", id)
	}
	position, found := findChannelPosition(profile.Channels, channelIndex)
	if !found {
		return device.CalibrationRecord{}, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	channel := profile.Channels[position]
	return device.CalibrationRecord{ChannelIndex: channel.Index, Offset: channel.CalibrationOffset, Unit: channel.CalibrationUnit, At: channel.CalibrationAt, Enabled: calibrationEnabledForProfile(profile.Type, channel)}, nil
}

// ClearCalibration 清除通道的校零偏移。
func (m *DeviceManager) ClearCalibration(id string, channelIndex int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pi, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	nextProfiles := cloneProfiles(m.profiles)
	position, found := findChannelPosition(nextProfiles[pi].Channels, channelIndex)
	if !found {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	nextProfiles[pi].Channels[position].CalibrationOffset = 0
	nextProfiles[pi].Channels[position].CalibrationUnit = ""
	nextProfiles[pi].Channels[position].CalibrationAt = 0
	if err := m.store.SaveProfiles(nextProfiles); err != nil {
		return err
	}
	m.profiles = nextProfiles
	m.loadCalibrationOffsetsLocked(id)
	return nil
}

// SetCalibrationEnabled 设置通道校零使能开关（仅 DAQ-P-1603）。
func (m *DeviceManager) SetCalibrationEnabled(id string, channelIndex int, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pi, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if m.profiles[pi].Type != device.DeviceDAQP1603 {
		return fmt.Errorf("calibration enable is only configurable for DAQ-P-1603")
	}
	nextProfiles := cloneProfiles(m.profiles)
	position, found := findChannelPosition(nextProfiles[pi].Channels, channelIndex)
	if !found {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	if !device.NewUnitConverter().SupportsZeroCalibration(nextProfiles[pi].Channels[position].Unit) {
		return fmt.Errorf("channel %d does not support zero calibration", channelIndex)
	}
	nextProfiles[pi].Channels[position].CalibrationEnabled = enabled
	if err := m.store.SaveProfiles(nextProfiles); err != nil {
		return err
	}
	m.profiles = nextProfiles
	return nil
}

// GetCalibrationEnabled 读取通道校零使能状态。
func (m *DeviceManager) GetCalibrationEnabled(id string, channelIndex int) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profile, ok := m.findProfileLocked(id)
	if !ok {
		return false, fmt.Errorf("device profile not found: %s", id)
	}
	position, found := findChannelPosition(profile.Channels, channelIndex)
	if !found {
		return false, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return calibrationEnabledForProfile(profile.Type, profile.Channels[position]), nil
}

func findChannelPosition(channels []device.ChannelConfig, channelIndex int) (int, bool) {
	for i := range channels {
		if channels[i].Index == channelIndex {
			return i, true
		}
	}
	return 0, false
}

func calibrationEnabledForProfile(profileType device.Type, channel device.ChannelConfig) bool {
	if !device.NewUnitConverter().SupportsZeroCalibration(channel.Unit) {
		return false
	}
	if profileType != device.DeviceDAQP1603 {
		return true
	}
	return channel.CalibrationEnabled
}

func cloneProfiles(profiles []device.Profile) []device.Profile {
	cloned := append([]device.Profile(nil), profiles...)
	for i := range cloned {
		cloned[i].Channels = append([]device.ChannelConfig(nil), profiles[i].Channels...)
	}
	return cloned
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

// 编译期接口断言：保证 DeviceManager 实现 ports.ChannelUnitProvider。
// 若 ChannelUnit 签名变更导致不再实现，编译期立即报错而非运行时崩溃。
var _ ports.ChannelUnitProvider = (*DeviceManager)(nil)
