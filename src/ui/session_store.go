package ui

import (
	"context"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"renovate-operator/internal/kvstore"
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

// NewSessionStore creates a SessionStore wrapping the provided KVStore.
// If kvStore is nil, returns (nil, nil) — indicating cookie-only mode.
func NewSessionStore(kvStore kvstore.KVStore, encryptionKey [32]byte) (SessionStore, error) {
	if kvStore == nil {
		return nil, nil
	}

	gcm, err := newGCM(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM for session store: %w", err)
	}

	return &valkeySessionStore{store: kvStore, gcm: gcm}, nil
}

// valkeySessionStore is a SessionStore backed by a KVStore.
// It handles JSON marshaling and AES-GCM encryption, delegating
// raw storage to the underlying KVStore with a "session:" key prefix.
type valkeySessionStore struct {
	store kvstore.KVStore
	gcm   cipher.AEAD
}

func (v *valkeySessionStore) Save(ctx context.Context, id string, data sessionData, ttl time.Duration) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	sealed, err := sealGCM(v.gcm, payload)
	if err != nil {
		return fmt.Errorf("failed to encrypt session data: %w", err)
	}

	return v.store.Put(ctx, kvstore.JoinKey("session", id), sealed, ttl)
}

func (v *valkeySessionStore) Load(ctx context.Context, id string) (*sessionData, error) {
	raw, err := v.store.Get(ctx, kvstore.JoinKey("session", id))
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session from store: %w", err)
	}

	plaintext, err := openGCM(v.gcm, raw)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt session data: %w", err)
	}

	var data sessionData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &data, nil
}

func (v *valkeySessionStore) Delete(ctx context.Context, id string) error {
	return v.store.Del(ctx, kvstore.JoinKey("session", id))
}

func (v *valkeySessionStore) Close() error {
	return v.store.Close()
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

// sweep removes all expired entries from the store using a two-phase
// approach: collect expired keys under a read lock, then delete them
// under a write lock (with re-check). This avoids blocking Save/Load/Delete
// for the entire map iteration.
func (m *memorySessionStore) sweep() {
	defer m.sweeping.Store(false)
	now := time.Now()

	// Phase 1: collect expired keys under read lock
	m.mu.RLock()
	var expired []string
	for id, entry := range m.entries {
		if now.After(entry.expiresAt) {
			expired = append(expired, id)
		}
	}
	m.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	// Phase 2: delete under write lock with re-check
	m.mu.Lock()
	for _, id := range expired {
		if e, ok := m.entries[id]; ok && now.After(e.expiresAt) {
			delete(m.entries, id)
		}
	}
	m.mu.Unlock()
}
