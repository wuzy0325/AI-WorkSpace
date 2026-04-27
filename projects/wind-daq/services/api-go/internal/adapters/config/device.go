package config

import "wind-daq/services/api-go/internal/core/device"

// ==================== 设备配置存储 ====================
// 负责设备配置持久化(保存到文件)
// 嵌入Manager提供基础文件操作

// DeviceStore 设备配置存储
// 专门用于存储设备配置(DeviceProfile)
type DeviceStore struct {
	*Manager // 嵌入基础配置管理器
}

// NewDeviceStore 构建设备配置存储
// 参数: m 基础配置管理器
// 返回: *DeviceStore 设备配置存储实例
func NewDeviceStore(m *Manager) *DeviceStore {
	return &DeviceStore{Manager: m}
}

// LoadProfiles 加载设备配置列表
// 返回: []device.DeviceProfile 配置列表, error 错误信息
func (s *DeviceStore) LoadProfiles() ([]device.DeviceProfile, error) {
	var profiles []device.DeviceProfile
	if err := s.Load("device-profiles", &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

// SaveProfiles 保存设备配置列表
// 参数: profiles 配置列表
// 返回: error 错误信息
func (s *DeviceStore) SaveProfiles(profiles []device.DeviceProfile) error {
	return s.Save("device-profiles", profiles)
}
