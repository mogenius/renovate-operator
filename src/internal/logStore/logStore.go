package logStore

import (
	"fmt"
	"sync"
)

// LogStore holds the most-recent log output for completed Renovate executor jobs,
// keyed by (namespace, renovateJob, project). Entries are overwritten on every new
// run, so memory growth is bounded by the number of distinct projects.
type LogStore interface {
	// Save stores logs for the given project, overwriting any previously saved value.
	Save(namespace, renovateJob, project, logs string)
	// Get retrieves the most-recently saved logs for the given project.
	// Returns (logs, true) if found, ("", false) otherwise.
	Get(namespace, renovateJob, project string) (string, bool)
}

// noopLogStore is the default implementation when LOG_STORE_MODE=disabled.
// All operations are no-ops so there is zero overhead.
type noopLogStore struct{}

func (noopLogStore) Save(_, _, _, _ string)         {}
func (noopLogStore) Get(_, _, _ string) (string, bool) { return "", false }

// memoryLogStore is the in-memory implementation when LOG_STORE_MODE=memory.
type memoryLogStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func key(namespace, renovateJob, project string) string {
	return fmt.Sprintf("RENOVATE_LOGS:%s:%s:%s", namespace, renovateJob, project)
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

// NewLogStore creates a LogStore based on the provided mode.
// Supported modes: "disabled" (default, no-op), "memory" (in-memory store).
func NewLogStore(mode string) LogStore {
	switch mode {
	case "memory":
		return &memoryLogStore{data: make(map[string]string)}
	default:
		return noopLogStore{}
	}
}
