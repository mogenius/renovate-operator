package kvstore

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrKeyNotFound is returned when a key does not exist in the store.
var ErrKeyNotFound = errors.New("key not found")

var ErrValkeyNotConfigured = errors.New("valkey is not configured")

// KVStore is a generic key-value store with TTL support.
type KVStore interface {
	Put(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Del(ctx context.Context, key string) error
	Close() error
}

// JoinKey builds a composite key by joining parts with ":".
func JoinKey(parts ...string) string {
	return strings.Join(parts, ":")
}

// NewKVStore creates a KVStore for the given usage.
// If Valkey is configured (via URL or Host), a Valkey-backed store is returned;
// otherwise ErrValkeyNotConfigured is returned.
func NewKVStore(cfg ValkeyConfig, usage Usage) (KVStore, error) {
	if !cfg.IsConfigured() {
		return nil, ErrValkeyNotConfigured
	}

	valkeyURL := cfg.URLForUsage(usage)
	if valkeyURL == "" {
		return nil, nil
	}

	return NewValkeyKVStore(valkeyURL)
}
