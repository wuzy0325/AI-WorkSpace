package manager

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"cal1604/internal/domain"
)

// StorageConfig 设备存储配置。
type StorageConfig struct {
	// StoragePath 存储文件路径，为空时使用默认路径
	StoragePath string
}

// deviceStorageData 设备存储数据结构。
type deviceStorageData struct {
	Version     int             `json:"version"`
	LastUpdated string          `json:"lastUpdated"`
	Devices     []domain.Device `json:"devices"`
}

const storageVersion = 1

// PersistentDeviceManager 支持持久化的设备管理器。
// 嵌入 DeviceManager 复用全部内存 CRUD 操作，仅覆盖需要持久化的方法。
type PersistentDeviceManager struct {
	*DeviceManager
	storagePath string
	dirty       bool
}

// NewPersistentDeviceManager 创建持久化设备管理器。
// 如果 config.StoragePath 为空，使用默认路径（用户配置目录下的 cal1604/devices.json）。
func NewPersistentDeviceManager(config StorageConfig) (*PersistentDeviceManager, error) {
	storagePath := config.StoragePath
	if storagePath == "" {
		defaultPath, err := defaultStoragePath()
		if err != nil {
			return nil, fmt.Errorf("get default storage path: %w", err)
		}
		storagePath = defaultPath
	}

	m := &PersistentDeviceManager{
		DeviceManager: NewDeviceManager(),
		storagePath:   storagePath,
	}

	// 尝试加载已有数据
	if err := m.loadFromDisk(); err != nil {
		// 文件不存在时忽略错误，视为首次启动
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load devices from disk: %w", err)
		}
	}

	return m, nil
}

// defaultStoragePath 返回默认存储路径。
// Windows: %APPDATA%/cal1604/devices.json
// Linux/macOS: ~/.config/cal1604/devices.json
func defaultStoragePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "cal1604", "devices.json"), nil
}

// Upsert 新增或更新设备配置，并持久化到磁盘。
func (m *PersistentDeviceManager) Upsert(dev domain.Device) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devices[dev.ID] = dev
	if err := m.saveToDiskLocked(); err != nil {
		log.Printf("[device] 设备 %s 持久化失败: %v", dev.ID, err)
		m.dirty = true
	}
}

// UpdateStatus 更新设备连接状态，返回是否更新成功。
func (m *PersistentDeviceManager) UpdateStatus(id string, status domain.DeviceStatus) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, ok := m.devices[id]
	if !ok {
		return false
	}

	dev.Status = status
	m.devices[id] = dev
	if err := m.saveToDiskLocked(); err != nil {
		log.Printf("[device] 设备 %s 状态更新持久化失败: %v", id, err)
		m.dirty = true
	}
	return true
}

// Delete 删除指定设备，并持久化到磁盘。
func (m *PersistentDeviceManager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.devices, id)
	if err := m.saveToDiskLocked(); err != nil {
		log.Printf("[device] 删除设备 %s 持久化失败: %v", id, err)
		m.dirty = true
	}
}

// StoragePath 返回当前存储文件路径。
func (m *PersistentDeviceManager) StoragePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.storagePath
}

// TryPersist 尝试将脏标记的数据持久化到磁盘。
// 如果 dirty 为 false 或持久化失败，返回错误。
func (m *PersistentDeviceManager) TryPersist() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.dirty {
		return nil
	}

	if err := m.saveToDiskLocked(); err != nil {
		return fmt.Errorf("persist dirty devices: %w", err)
	}
	m.dirty = false
	return nil
}

// saveToDiskLocked 将设备数据保存到磁盘（调用方必须持有锁）。
func (m *PersistentDeviceManager) saveToDiskLocked() error {
	data := deviceStorageData{
		Version:     storageVersion,
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
		Devices:     m.listSorted(),
	}

	// 已连接设备的单位保留，未连接设备的单位清空（下次连接时从硬件读取）
	for i := range data.Devices {
		if data.Devices[i].Status != domain.DeviceStatusConnected {
			data.Devices[i].Unit = ""
		}
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal devices: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(m.storagePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}

	// 写入临时文件后重命名，保证原子性
	tempPath := m.storagePath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, m.storagePath); err != nil {
		// 重命名失败时清理临时文件
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// loadFromDisk 从磁盘加载设备数据。
func (m *PersistentDeviceManager) loadFromDisk() error {
	data, err := os.ReadFile(m.storagePath)
	if err != nil {
		return err
	}

	var storage deviceStorageData
	if err := json.Unmarshal(data, &storage); err != nil {
		return fmt.Errorf("unmarshal storage: %w", err)
	}

	// 版本检查（未来可用于数据迁移）
	if storage.Version != storageVersion {
		// 目前只有版本1，后续版本可在此添加迁移逻辑
	}

	// 加载设备到内存
	for _, dev := range storage.Devices {
		m.devices[dev.ID] = dev
	}

	return nil
}

// Ensure PersistentDeviceManager 实现 DeviceStore 接口。
var _ interface {
	Upsert(dev domain.Device)
	UpdateStatus(id string, status domain.DeviceStatus) bool
	UpdateUnit(id string, unit string) bool
	Delete(id string)
	Get(id string) (domain.Device, bool)
	List() []domain.Device
	CheckUnitConsistency() (bool, []string)
} = (*PersistentDeviceManager)(nil)
