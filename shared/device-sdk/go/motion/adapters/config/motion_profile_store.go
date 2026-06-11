package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// FileMotionProfileStore file-based motion controller profile store
type FileMotionProfileStore struct {
	mu       sync.RWMutex
	filePath string
	profiles []core.MotionControllerProfile
}

// NewFileMotionProfileStore create file store
func NewFileMotionProfileStore(filePath string) *FileMotionProfileStore {
	return &FileMotionProfileStore{
		filePath: filePath,
	}
}

// LoadProfiles load profiles
func (s *FileMotionProfileStore) LoadProfiles() ([]core.MotionControllerProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.profiles != nil {
		return append([]core.MotionControllerProfile(nil), s.profiles...), nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.profiles = getDefaultProfiles()
			return append([]core.MotionControllerProfile(nil), s.profiles...), nil
		}
		return nil, err
	}

	if len(data) == 0 {
		s.profiles = getDefaultProfiles()
		return append([]core.MotionControllerProfile(nil), s.profiles...), nil
	}

	if err := json.Unmarshal(data, &s.profiles); err != nil {
		return nil, err
	}

	s.profiles = normalizeProfiles(s.profiles)

	return append([]core.MotionControllerProfile(nil), s.profiles...), nil
}

// SaveProfiles save profiles
func (s *FileMotionProfileStore) SaveProfiles(profiles []core.MotionControllerProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.profiles = append([]core.MotionControllerProfile(nil), profiles...)

	dir := filepath.Dir(s.filePath)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

func getDefaultProfiles() []core.MotionControllerProfile {
	return []core.MotionControllerProfile{
		{
			ID:          "sim-mc-1",
			Name:        "Simulated Controller 1",
			Type:        core.ControllerTypeSimulated,
			Address:     "127.0.0.1",
			Port:        9000,
			AutoConnect: false,
			Axes: []core.AxisConfig{
				{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
				{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
				{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
				{Name: core.AxisU, Enabled: false, Kind: core.AxisKindRotary, MaxSpeed: core.PtrFloat64(10)},
			},
		},
	}
}

func getDefaultAxes() []core.AxisConfig {
	return []core.AxisConfig{
		{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10), StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
		{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10), StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
		{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10), StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
		{Name: core.AxisU, Enabled: false, Kind: core.AxisKindRotary, MaxSpeed: core.PtrFloat64(10), StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
	}
}

func normalizeAxisConfig(axis core.AxisConfig) core.AxisConfig {
	if axis.StepsPerRev == nil || *axis.StepsPerRev <= 0 {
		defaultSteps := 1.8
		axis.StepsPerRev = &defaultSteps
	}
	if axis.MicroSteps == nil || *axis.MicroSteps < 1 {
		defaultMicro := 4
		axis.MicroSteps = &defaultMicro
	}
	if axis.Lead == nil || *axis.Lead <= 0 {
		defaultLead := 4.0
		axis.Lead = &defaultLead
	}
	if axis.GearRatio == nil || *axis.GearRatio <= 0 {
		defaultGear := 1.0
		axis.GearRatio = &defaultGear
	}
	if axis.MaxSpeed == nil || *axis.MaxSpeed <= 0 {
		defaultSpeed := 10.0
		axis.MaxSpeed = &defaultSpeed
	}
	if axis.EncoderScale == nil || *axis.EncoderScale <= 0 {
		defaultScale := 0.005
		axis.EncoderScale = &defaultScale
	}
	if axis.PositionSource == "" {
		axis.PositionSource = core.PositionSourceRegister
	}
	return axis
}

func normalizeProfile(profile core.MotionControllerProfile) core.MotionControllerProfile {
	if len(profile.Axes) == 0 {
		profile.Axes = getDefaultAxes()
	} else {
		for i := range profile.Axes {
			profile.Axes[i] = normalizeAxisConfig(profile.Axes[i])
		}
	}
	return profile
}

func normalizeProfiles(profiles []core.MotionControllerProfile) []core.MotionControllerProfile {
	for i := range profiles {
		profiles[i] = normalizeProfile(profiles[i])
	}
	return profiles
}

// MemoryMotionProfileStore in-memory motion controller profile store (for testing)
type MemoryMotionProfileStore struct {
	mu       sync.RWMutex
	profiles []core.MotionControllerProfile
}

// NewMemoryMotionProfileStore create memory store
func NewMemoryMotionProfileStore() *MemoryMotionProfileStore {
	return &MemoryMotionProfileStore{
		profiles: getDefaultProfiles(),
	}
}

// LoadProfiles load profiles
func (s *MemoryMotionProfileStore) LoadProfiles() ([]core.MotionControllerProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]core.MotionControllerProfile(nil), s.profiles...), nil
}

// SaveProfiles save profiles
func (s *MemoryMotionProfileStore) SaveProfiles(profiles []core.MotionControllerProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = profiles
	return nil
}

var _ ports.MotionProfileStore = (*FileMotionProfileStore)(nil)
var _ ports.MotionProfileStore = (*MemoryMotionProfileStore)(nil)
