package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Sentinel errors for session store operations.
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = fmt.Errorf("%w: expired", ErrSessionNotFound)
)

// SessionStore is the interface for server-side session storage.
// Sessions are stored behind an opaque session ID; only the ID is
// kept in the browser cookie, avoiding cookie size limits.
type SessionStore interface {
	Save(ctx context.Context, id string, data sessionData, ttl time.Duration) error
	Load(ctx context.Context, id string) (*sessionData, error)
	Delete(ctx context.Context, id string) error
	Close() error
}

// sessionEntry wraps session data with an expiration timestamp.
type sessionEntry struct {
	data      sessionData
	expiresAt time.Time
}

// memorySessionStore is an in-memory SessionStore backed by a map.
// Sessions are lost on pod restart and not shared across replicas.
type memorySessionStore struct {
	mu        sync.RWMutex
	entries   map[string]sessionEntry
	loadCount atomic.Int64
	sweeping  atomic.Bool
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() SessionStore {
	return &memorySessionStore{
		entries: make(map[string]sessionEntry),
	}
}

func (m *memorySessionStore) Save(_ context.Context, id string, data sessionData, ttl time.Duration) error {
	// Deep copy groups slice to avoid caller mutations affecting stored data
	groups := make([]string, len(data.Groups))
	copy(groups, data.Groups)
	stored := data
	stored.Groups = groups

	m.mu.Lock()
	m.entries[id] = sessionEntry{
		data:      stored,
		expiresAt: time.Now().Add(ttl),
	}
	m.mu.Unlock()
	return nil
}

func (m *memorySessionStore) Load(_ context.Context, id string) (*sessionData, error) {
	count := m.loadCount.Add(1)

	m.mu.RLock()
	entry, ok := m.entries[id]
	m.mu.RUnlock()

	if !ok {
		return nil, ErrSessionNotFound
	}

	if time.Now().After(entry.expiresAt) {
		// Lazy expiry: delete the expired entry
		m.mu.Lock()
		delete(m.entries, id)
		m.mu.Unlock()
		return nil, ErrSessionExpired
	}

	// Every 100th Load, sweep for other expired entries (at most one concurrent sweep)
	if count%100 == 0 && m.sweeping.CompareAndSwap(false, true) {
		go m.sweep()
	}

	// Return a deep copy to avoid data races on concurrent reads
	result := entry.data
	groups := make([]string, len(entry.data.Groups))
	copy(groups, entry.data.Groups)
	result.Groups = groups

	return &result, nil
}

func (m *memorySessionStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	delete(m.entries, id)
	m.mu.Unlock()
	return nil
}

func (m *memorySessionStore) Close() error {
	return nil
}

// sweep removes all expired entries from the store.
func (m *memorySessionStore) sweep() {
	defer m.sweeping.Store(false)
	now := time.Now()
	m.mu.Lock()
	for id, entry := range m.entries {
		if now.After(entry.expiresAt) {
			delete(m.entries, id)
		}
	}
	m.mu.Unlock()
}
