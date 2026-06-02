package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"daq-t1603/core"
)

type JSONConfigStore struct {
	mu       sync.RWMutex
	filePath string
}

func NewJSONConfigStore(filePath string) *JSONConfigStore {
	return &JSONConfigStore{filePath: filePath}
}

func (s *JSONConfigStore) LoadProfiles() ([]core.TemperatureProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadUnsafe()
}

func (s *JSONConfigStore) SaveProfile(profile core.TemperatureProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	replaced := false
	for i, p := range profiles {
		if p.ID == profile.ID {
			profiles[i] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		profiles = append(profiles, profile)
	}
	return s.saveUnsafe(profiles)
}

func (s *JSONConfigStore) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	filtered := make([]core.TemperatureProfile, 0, len(profiles))
	for _, p := range profiles {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return s.saveUnsafe(filtered)
}

func (s *JSONConfigStore) loadUnsafe() ([]core.TemperatureProfile, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var profiles []core.TemperatureProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *JSONConfigStore) saveUnsafe(profiles []core.TemperatureProfile) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}
