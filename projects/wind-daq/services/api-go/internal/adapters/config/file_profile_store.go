package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	return profiles, nil
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
