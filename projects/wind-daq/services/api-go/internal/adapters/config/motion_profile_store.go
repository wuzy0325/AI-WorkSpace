package config

import (
	"encoding/json"
	"os"
	"sync"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

// FileMotionProfileStore 基于文件的运动控制器配置存储
type FileMotionProfileStore struct {
	mu       sync.RWMutex
	filePath string
	profiles []motion.MotionControllerProfile
}

// NewFileMotionProfileStore 创建文件存储
func NewFileMotionProfileStore(filePath string) *FileMotionProfileStore {
	return &FileMotionProfileStore{
		filePath: filePath,
		profiles: make([]motion.MotionControllerProfile, 0),
	}
}

// LoadProfiles 加载配置
func (s *FileMotionProfileStore) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已经加载过，直接返回
	if s.profiles != nil {
		return s.profiles, nil
	}

	// 读取文件
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，返回默认配置
			s.profiles = getDefaultProfiles()
			return s.profiles, nil
		}
		return nil, err
	}

	// 解析 JSON
	if len(data) == 0 {
		s.profiles = getDefaultProfiles()
		return s.profiles, nil
	}

	if err := json.Unmarshal(data, &s.profiles); err != nil {
		return nil, err
	}

	return s.profiles, nil
}

// SaveProfiles 保存配置
func (s *FileMotionProfileStore) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.profiles = profiles

	// 确保目录存在
	dir := s.filePath[:len(s.filePath)-len("/motion-profiles.json")]
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}

	// 写入文件
	return os.WriteFile(s.filePath, data, 0644)
}

// getDefaultProfiles 获取默认配置
func getDefaultProfiles() []motion.MotionControllerProfile {
	return []motion.MotionControllerProfile{
		{
			ID:          "sim-mc-1",
			Name:        "模拟控制器 1",
			Type:        motion.ControllerTypeSimulated,
			Address:     "127.0.0.1",
			Port:        9000,
			AutoConnect: false,
			Axes: []motion.AxisConfig{
				{Name: motion.AxisX, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: ptrFloat64(10)},
				{Name: motion.AxisY, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: ptrFloat64(10)},
				{Name: motion.AxisZ, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: ptrFloat64(10)},
				{Name: motion.AxisU, Enabled: false, Kind: motion.AxisKindRotary, MaxSpeed: ptrFloat64(10)},
			},
		},
	}
}

// MemoryMotionProfileStore 内存运动控制器配置存储（用于测试）
type MemoryMotionProfileStore struct {
	mu       sync.RWMutex
	profiles []motion.MotionControllerProfile
}

// NewMemoryMotionProfileStore 创建内存存储
func NewMemoryMotionProfileStore() *MemoryMotionProfileStore {
	return &MemoryMotionProfileStore{
		profiles: getDefaultProfiles(),
	}
}

// LoadProfiles 加载配置
func (s *MemoryMotionProfileStore) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profiles, nil
}

// SaveProfiles 保存配置
func (s *MemoryMotionProfileStore) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = profiles
	return nil
}

// Ensure FileMotionProfileStore implements ports.MotionProfileStore
var _ ports.MotionProfileStore = (*FileMotionProfileStore)(nil)

// ptrFloat64 创建 float64 指针
func ptrFloat64(v float64) *float64 {
	return &v
}
