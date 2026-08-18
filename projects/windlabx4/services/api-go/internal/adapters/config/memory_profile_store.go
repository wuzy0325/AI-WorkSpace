package config

import (
	"sync"

	"windlabx4/services/api-go/internal/core/device"
)

type MemoryProfileStore struct {
	mu       sync.RWMutex
	profiles []device.Profile
}

func NewMemoryProfileStore(profiles []device.Profile) *MemoryProfileStore {
	return &MemoryProfileStore{profiles: append([]device.Profile(nil), profiles...)}
}

func (s *MemoryProfileStore) LoadProfiles() ([]device.Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]device.Profile(nil), s.profiles...), nil
}

func (s *MemoryProfileStore) SaveProfiles(profiles []device.Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = append([]device.Profile(nil), profiles...)
	return nil
}
