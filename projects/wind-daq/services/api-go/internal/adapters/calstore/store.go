package calstore

import (
	"sync"

	"wind-daq/services/api-go/internal/core/calibration"
)

type MemoryResultStore struct {
	mu   sync.RWMutex
	data map[string]calibration.Status
}

func NewMemoryResultStore() *MemoryResultStore {
	return &MemoryResultStore{data: make(map[string]calibration.Status)}
}

func (s *MemoryResultStore) Save(taskID string, status calibration.Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[taskID] = status
	return nil
}

func (s *MemoryResultStore) Get(taskID string) (calibration.Status, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.data[taskID]
	return status, ok
}
