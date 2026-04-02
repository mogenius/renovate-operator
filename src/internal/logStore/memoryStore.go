package logStore

import (
	"sync"
)

// memoryLogStore is the in-memory implementation when LOG_STORE_MODE=memory.
type memoryLogStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func (s *memoryLogStore) Save(namespace, renovateJob, project, logs string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key(namespace, renovateJob, project)] = logs
}

func (s *memoryLogStore) Get(namespace, renovateJob, project string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs, ok := s.data[key(namespace, renovateJob, project)]
	return logs, ok
}
