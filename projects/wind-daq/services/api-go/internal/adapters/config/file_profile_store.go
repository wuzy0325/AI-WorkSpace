package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
)

type FileProfileStore struct {
	mu   sync.Mutex
	path string
}

func NewFileProfileStore(path string) *FileProfileStore {
	return &FileProfileStore{path: path}
}

func (s *FileProfileStore) LoadProfiles() ([]device.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []device.Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}

	var profiles []device.Profile
	if err := json.Unmarshal(content, &profiles); err != nil {
		return nil, fmt.Errorf("decode profiles: %w", err)
	}

	migrated := migrateDeviceTypes(content, s.path)
	if migrated {
		var migratedProfiles []device.Profile
		if err := json.Unmarshal(content, &migratedProfiles); err == nil {
			profiles = migratedProfiles
		}
	}

	normalizeSensorTypes(profiles)

	return profiles, nil
}

var legacyTypeMap = map[string]device.Type{
	"simulated":     device.DeviceSimulated,
	"DAQ_P_1604":    device.DeviceDAQP1604,
	"DAQ_T_1603":    device.DeviceDaqT1603,
	"DAQ_P_1604Pre": device.DeviceDAQP1604Pre,
}

func migrateDeviceTypes(content []byte, path string) bool {
	raw := string(content)
	changed := false
	for old, newType := range legacyTypeMap {
		if old == string(newType) {
			continue
		}
		quoted := `"` + old + `"`
		newQuoted := `"` + string(newType) + `"`
		if strings.Contains(raw, quoted) {
			raw = strings.ReplaceAll(raw, quoted, newQuoted)
			changed = true
		}
	}
	if changed {
		_ = os.WriteFile(path, []byte(raw), 0o600)
	}
	return changed
}

// normalizeSensorTypes 确保所有 ChannelConfig.SensorType 不为空。
// 旧 profile（DAQ-P-1604 / DAQ-T-1603 / 历史 SIMULATED）不含 sensorType 字段，
// 反序列化后该字段为零值空字符串。统一在加载入口兜底为 "pressure"，
// 让所有读路径拿到的 ChannelConfig 都有合法的 SensorType 值。
func normalizeSensorTypes(profiles []device.Profile) {
	for i := range profiles {
		for j := range profiles[i].Channels {
			if profiles[i].Channels[j].SensorType == "" {
				profiles[i].Channels[j].SensorType = device.SensorPressure
			}
		}
	}
}

func (s *FileProfileStore) SaveProfiles(profiles []device.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	content, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	if err := os.WriteFile(s.path, append(content, '\n'), 0o600); err != nil {
		return fmt.Errorf("write profiles: %w", err)
	}
	return nil
}
