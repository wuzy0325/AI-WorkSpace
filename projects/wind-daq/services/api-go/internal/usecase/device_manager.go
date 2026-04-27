package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 设备管理器 ====================
// 负责所有数据采集设备的生命周期管理
// - 配置管理(增删改查)
// - 连接管理(连接/断开)
// - 采集控制(开始/停止)
// - 状态轮询(定时推送状态到前端)

// DeviceManager 设备管理器
// 管理多个设备的配置、连接和采集状态
type DeviceManager struct {
	mu         sync.RWMutex
	devices    map[string]ports.Device // id -> 设备实例映射
	profiles   []device.DeviceProfile  // 设备配置列表(从文件加载)
	store      ports.ProfileStore      // 配置持久化存储
	publisher  ports.DataPublisher     // 数据广播
	factory    ports.DeviceFactory     // 设备驱动工厂
	pollCancel context.CancelFunc      // 状态轮询取消函数
}

// 设备状态轮询间隔
const devicePollInterval = 1 * time.Second

// NewDeviceManager 构建设备管理器
// 参数: store 配置存储, wsHub WebSocket广播Hub
// 返回: *DeviceManager 设备管理器实例
func NewDeviceManager(store ports.ProfileStore, publisher ports.DataPublisher, factory ports.DeviceFactory) *DeviceManager {
	m := &DeviceManager{
		devices:   make(map[string]ports.Device),
		store:     store,
		publisher: publisher,
		factory:   factory,
	}
	// 从存储加载已保存的设备配置
	if profiles, err := store.LoadProfiles(); err == nil && profiles != nil {
		m.profiles = profiles
	}
	return m
}

// GetProfiles 获取所有设备配置
// 返回: []device.DeviceProfile 设备配置列表
func (m *DeviceManager) GetProfiles() []device.DeviceProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profiles
}

// UpsertProfile 添加或更新设备配置
// 参数: profile 设备配置
// 返回: error 错误信息(如果保存失败)
func (m *DeviceManager) UpsertProfile(profile device.DeviceProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 查找是否已存在同ID的配置
	found := false
	for i, p := range m.profiles {
		if p.ID == profile.ID {
			m.profiles[i] = profile
			found = true
			break
		}
	}
	// 不存在则追加到列表
	if !found {
		m.profiles = append(m.profiles, profile)
	}

	// 持久化保存
	if err := m.store.SaveProfiles(m.profiles); err != nil {
		return err
	}
	slog.Info("Device profile upserted", "id", profile.ID, "name", profile.Name)
	return nil
}

// DeleteProfile 删除设备配置
// 参数: id 设备配置ID
// 返回: error 错误信息
func (m *DeviceManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从配置列表中移除
	for i, p := range m.profiles {
		if p.ID == id {
			m.profiles = append(m.profiles[:i], m.profiles[i+1:]...)
			break
		}
	}

	// 如果设备已连接,则断开
	if dev, ok := m.devices[id]; ok {
		dev.Disconnect()
		delete(m.devices, id)
	}

	// 持久化保存
	if err := m.store.SaveProfiles(m.profiles); err != nil {
		return err
	}
	slog.Info("Device profile deleted", "id", id)
	return nil
}

// Connect 连接指定设备
// 根据配置ID查找配置,创建设备实例并连接
// 参数: id 设备配置ID
// 返回: error 错误信息
func (m *DeviceManager) Connect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已连接(已连接或采集中则跳过)
	if dev, ok := m.devices[id]; ok {
		st := dev.Status()
		if st.Connection == device.ConnectionConnected || st.Acquiring {
			return nil // already connected
		}
	}

	// 查找设备配置
	var profile device.DeviceProfile
	found := false
	for _, p := range m.profiles {
		if p.ID == id {
			profile = p
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("device profile not found: %s", id)
	}

	// 根据配置创建设备驱动实例
	devConfig := device.DeviceConfig{
		ID:           profile.ID,
		Name:         profile.Name,
		Type:         profile.Type,
		Transport:    profile.Transport,
		Address:      profile.Address,
		Port:         profile.Port,
		SerialPort:   profile.SerialPort,
		BaudRate:     profile.BaudRate,
		SamplingRate: profile.SamplingRate,
		Channels:     profile.Channels,
	}

	dev, err := m.factory.Create(devConfig)
	if err != nil {
		return fmt.Errorf("failed to create device: %w", err)
	}

	// 应用保存的通道配置
	if len(profile.Channels) > 0 {
		if updatable, ok := dev.(ports.ChannelUpdatable); ok {
			updatable.UpdateChannels(profile.Channels)
		}
	}

	// 设置数据回调(推送到WebSocket)
	dev.SetDataSink(func(payload device.DataPayload) {
		m.publisher.Broadcast(ports.ChannelDAQSnapshot, []device.DataPayload{payload})
	})

	// 执行连接
	if err := dev.Connect(); err != nil {
		return fmt.Errorf("failed to connect device: %w", err)
	}

	m.devices[id] = dev
	slog.Info("Device connected", "id", id, "type", profile.Type)
	return nil
}

// Disconnect 断开指定设备
// 参数: id 设备ID
// 返回: error 错误信息
func (m *DeviceManager) Disconnect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, ok := m.devices[id]
	if !ok {
		return nil
	}

	if err := dev.Disconnect(); err != nil {
		return err
	}
	delete(m.devices, id)
	slog.Info("Device disconnected", "id", id)
	return nil
}

// StartAcquisition 开始指定设备的数据采集
// 参数: id 设备ID
// 返回: error 错误信息
func (m *DeviceManager) StartAcquisition(id string) error {
	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("device not found: %s", id)
	}
	return dev.StartAcquisition()
}

// StartAcquisitionAll 开始所有已连接设备的采集
// 返回: int 成功启动的设备数量
func (m *DeviceManager) StartAcquisitionAll() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	started := 0
	for id, dev := range m.devices {
		if dev.Status().Connection == device.ConnectionConnected {
			if err := dev.StartAcquisition(); err != nil {
				slog.Warn("Failed to start acquisition", "id", id, "err", err)
			} else {
				started++
			}
		}
	}
	if started > 0 {
		slog.Info("Acquisition started", "count", started)
		m.broadcastStatus()
	}
	return started
}

// StopAcquisition 停止指定设备的采集
// 参数: id 设备ID
// 返回: error 错误信息
func (m *DeviceManager) StopAcquisition(id string) error {
	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("device not found: %s", id)
	}
	return dev.StopAcquisition()
}

// StopAcquisitionAll 停止所有设备的采集
func (m *DeviceManager) StopAcquisitionAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, dev := range m.devices {
		if err := dev.StopAcquisition(); err != nil {
			slog.Warn("Failed to stop acquisition", "id", id, "err", err)
		}
	}
	slog.Info("Acquisition stopped for all devices")
	m.broadcastStatus()
}

// GetInstances 获取当前活跃的所有设备实例
// 返回: []device.DeviceInstance 设备实例列表(仅包含已连接的)
func (m *DeviceManager) GetInstances() []device.DeviceInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instances := make([]device.DeviceInstance, 0, len(m.devices))
	for _, dev := range m.devices {
		st := dev.Status()
		instances = append(instances, device.DeviceInstance{
			ProfileID: dev.ID(),
			Status:    string(st.Connection),
			Acquiring: st.Acquiring,
			LastError: st.LastError,
		})
	}
	return instances
}

// GetStatusAll 获取所有设备状态
// 返回: []device.DeviceStatus 状态列表
func (m *DeviceManager) GetStatusAll() []device.DeviceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]device.DeviceStatus, 0, len(m.devices))
	for _, dev := range m.devices {
		statuses = append(statuses, dev.Status())
	}
	return statuses
}

// SetDataSink 设置全局数据回调
// 将所有设备的数据统一推送
func (m *DeviceManager) SetDataSink(sink device.DataSink) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, dev := range m.devices {
		dev.SetDataSink(sink)
	}
}

// StartStatusPolling 启动状态轮询(1Hz频率)
// 参数: ctx 上下文(用于取消)
func (m *DeviceManager) StartStatusPolling(ctx context.Context) {
	pollCtx, cancel := context.WithCancel(ctx)
	m.pollCancel = cancel

	ticker := time.NewTicker(devicePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return
		case <-ticker.C:
			m.broadcastStatus()
		}
	}
}

// StopStatusPolling 停止状态轮询
func (m *DeviceManager) StopStatusPolling() {
	if m.pollCancel != nil {
		m.pollCancel()
	}
}

// broadcastStatus 将设备状态广播到前端
func (m *DeviceManager) broadcastStatus() {
	instances := m.GetInstances()
	if m.publisher != nil && len(instances) > 0 {
		m.publisher.Broadcast(ports.ChannelDeviceStatus, instances)
	}
}

// Shutdown 关闭管理器(断开所有设备)
// 参数: ctx 上下文(用于超时控制)
func (m *DeviceManager) Shutdown(ctx context.Context) {
	m.StopStatusPolling()
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, dev := range m.devices {
		if err := dev.Disconnect(); err != nil {
			slog.Warn("Error disconnecting device during shutdown", "id", id, "err", err)
		}
	}
	m.devices = make(map[string]ports.Device)
}
