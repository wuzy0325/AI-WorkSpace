package manager

import (
	"sort"
	"strings"
	"sync"

	"cal1604/internal/domain"
)

// DeviceManager 负责维护设备配置与单位一致性检查。
type DeviceManager struct {
	mu      sync.RWMutex
	devices map[string]domain.Device
}

// NewDeviceManager 创建内存版设备管理器。
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{
		devices: make(map[string]domain.Device),
	}
}

// Upsert 新增或更新设备配置。
func (m *DeviceManager) Upsert(dev domain.Device) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devices[dev.ID] = dev
}

// UpdateStatus 更新设备连接状态，返回是否更新成功。
func (m *DeviceManager) UpdateStatus(id string, status domain.DeviceStatus) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, ok := m.devices[id]
	if !ok {
		return false
	}

	dev.Status = status
	m.devices[id] = dev
	return true
}

// UpdateUnit 更新设备单位，不触发持久化（单位从硬件实时读取）。
func (m *DeviceManager) UpdateUnit(id string, unit string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, ok := m.devices[id]
	if !ok {
		return false
	}

	dev.Unit = unit
	m.devices[id] = dev
	return true
}

// Delete 删除指定设备。
func (m *DeviceManager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.devices, id)
}

// Get 查询指定设备。
func (m *DeviceManager) Get(id string) (domain.Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dev, ok := m.devices[id]
	return dev, ok
}

// listSorted 返回按 ID 升序排列的设备列表快照（调用方必须持有 mu 读锁）。
func (m *DeviceManager) listSorted() []domain.Device {
	result := make([]domain.Device, 0, len(m.devices))
	for _, dev := range m.devices {
		result = append(result, dev)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// List 返回设备快照，按设备 ID 升序排列以保证顺序稳定。
// Go map 遍历顺序随机化，必须显式排序否则前端每次轮询顺序可能不同。
func (m *DeviceManager) List() []domain.Device {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listSorted()
}

// CheckUnitConsistency 检查全部**已连接**设备单位是否一致。
// 未连接设备不参与判定（其 Unit 可能为配置默认值，与硬件实际单位不同）。
// 返回值依次为：是否一致、冲突设备 ID 列表（升序）。
func (m *DeviceManager) CheckUnitConsistency() (bool, []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connected := make([]domain.Device, 0, len(m.devices))
	for _, dev := range m.devices {
		if dev.Status == domain.DeviceStatusConnected {
			connected = append(connected, dev)
		}
	}

	if len(connected) <= 1 {
		return true, nil
	}

	baseline := ""
	conflicts := make([]string, 0)

	for _, dev := range connected {
		unit := strings.ToLower(strings.TrimSpace(dev.Unit))
		if baseline == "" && unit != "" {
			baseline = unit
			continue
		}

		if unit == "" || (baseline != "" && unit != baseline) {
			conflicts = append(conflicts, dev.ID)
		}
	}

	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return false, conflicts
	}

	return true, nil
}
