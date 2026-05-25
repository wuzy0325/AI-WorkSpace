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

	return profiles, nil
}

var legacyTypeMap = map[string]device.Type{
	"simulated":     device.DeviceSimulated,
	"DAQ_P_1604":    device.DeviceDAQP1604,
	"DAQ_T_1603":    device.DeviceDaqT1603,
	"DAQ_P_1064Pre": device.DeviceDAQP1064Pre,
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
