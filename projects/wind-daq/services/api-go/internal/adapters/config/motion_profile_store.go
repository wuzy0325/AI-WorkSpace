package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	}
}

// LoadProfiles 加载配置
func (s *FileMotionProfileStore) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If already loaded, return the cached profiles.
	if s.profiles != nil {
		return append([]motion.MotionControllerProfile(nil), s.profiles...), nil
	}

	// 读取文件
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，返回默认配置
			s.profiles = getDefaultProfiles()
			return append([]motion.MotionControllerProfile(nil), s.profiles...), nil
		}
		return nil, err
	}

	// 解析 JSON
	if len(data) == 0 {
		s.profiles = getDefaultProfiles()
		return append([]motion.MotionControllerProfile(nil), s.profiles...), nil
	}

	if err := json.Unmarshal(data, &s.profiles); err != nil {
		return nil, err
	}

	// 规范化配置：为空的 axes 填充默认值
	s.profiles = normalizeProfiles(s.profiles)

	return append([]motion.MotionControllerProfile(nil), s.profiles...), nil
}

// SaveProfiles 保存配置
func (s *FileMotionProfileStore) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.profiles = append([]motion.MotionControllerProfile(nil), profiles...)

	// Ensure directory exists.
	dir := filepath.Dir(s.filePath)
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
				{Name: motion.AxisX, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: motion.PtrFloat64(10)},
				{Name: motion.AxisY, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: motion.PtrFloat64(10)},
				{Name: motion.AxisZ, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: motion.PtrFloat64(10)},
				{Name: motion.AxisU, Enabled: false, Kind: motion.AxisKindRotary, MaxSpeed: motion.PtrFloat64(10)},
			},
		},
	}
}

// getDefaultAxes 获取默认轴配置（4轴：X/Y/Z/U）
func getDefaultAxes() []motion.AxisConfig {
	return []motion.AxisConfig{
		{Name: motion.AxisX, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: motion.PtrFloat64(10), StepsPerRev: motion.PtrFloat64(1.8), MicroSteps: motion.PtrInt(4), Lead: motion.PtrFloat64(4), GearRatio: motion.PtrFloat64(1), PositionSource: motion.PositionSourceRegister, EncoderScale: motion.PtrFloat64(0.005)},
		{Name: motion.AxisY, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: motion.PtrFloat64(10), StepsPerRev: motion.PtrFloat64(1.8), MicroSteps: motion.PtrInt(4), Lead: motion.PtrFloat64(4), GearRatio: motion.PtrFloat64(1), PositionSource: motion.PositionSourceRegister, EncoderScale: motion.PtrFloat64(0.005)},
		{Name: motion.AxisZ, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: motion.PtrFloat64(10), StepsPerRev: motion.PtrFloat64(1.8), MicroSteps: motion.PtrInt(4), Lead: motion.PtrFloat64(4), GearRatio: motion.PtrFloat64(1), PositionSource: motion.PositionSourceRegister, EncoderScale: motion.PtrFloat64(0.005)},
		{Name: motion.AxisU, Enabled: false, Kind: motion.AxisKindRotary, MaxSpeed: motion.PtrFloat64(10), StepsPerRev: motion.PtrFloat64(1.8), MicroSteps: motion.PtrInt(4), Lead: motion.PtrFloat64(4), GearRatio: motion.PtrFloat64(1), PositionSource: motion.PositionSourceRegister, EncoderScale: motion.PtrFloat64(0.005)},
	}
}

// normalizeAxisConfig 补全轴配置中的 nil 指针字段
func normalizeAxisConfig(axis motion.AxisConfig) motion.AxisConfig {
	if axis.StepsPerRev == nil {
		defaultSteps := 1.8
		axis.StepsPerRev = &defaultSteps
	}
	if axis.MicroSteps == nil {
		defaultMicro := 4
		axis.MicroSteps = &defaultMicro
	}
	if axis.Lead == nil {
		defaultLead := 4.0
		axis.Lead = &defaultLead
	}
	if axis.GearRatio == nil {
		defaultGear := 1.0
		axis.GearRatio = &defaultGear
	}
	if axis.MaxSpeed == nil {
		defaultSpeed := 10.0
		axis.MaxSpeed = &defaultSpeed
	}
	if axis.EncoderScale == nil {
		defaultScale := 0.005
		axis.EncoderScale = &defaultScale
	}
	if axis.PositionSource == "" {
		axis.PositionSource = motion.PositionSourceRegister
	}
	return axis
}

// normalizeProfile 规范化配置：如果 axes 为空，填充默认轴配置；否则补全缺失字段
func normalizeProfile(profile motion.MotionControllerProfile) motion.MotionControllerProfile {
	if len(profile.Axes) == 0 {
		profile.Axes = getDefaultAxes()
	} else {
		for i := range profile.Axes {
			profile.Axes[i] = normalizeAxisConfig(profile.Axes[i])
		}
	}
	return profile
}

// normalizeProfiles 规范化所有配置
func normalizeProfiles(profiles []motion.MotionControllerProfile) []motion.MotionControllerProfile {
	for i := range profiles {
		profiles[i] = normalizeProfile(profiles[i])
	}
	return profiles
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
	return append([]motion.MotionControllerProfile(nil), s.profiles...), nil
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
