package calstore

import (
	"sync"

	"wind-daq/services/api-go/internal/core/traversal"
)

type TraversalResultStore struct {
	mu   sync.RWMutex
	data map[string]traversal.Status
}

func NewTraversalResultStore() *TraversalResultStore {
	return &TraversalResultStore{data: make(map[string]traversal.Status)}
}

func (s *TraversalResultStore) Save(taskID string, status traversal.Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[taskID] = status
	return nil
}

func (s *TraversalResultStore) Get(taskID string) (traversal.Status, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.data[taskID]
	return status, ok
}
