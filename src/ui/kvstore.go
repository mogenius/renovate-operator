package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ErrKeyNotFound is returned when a key does not exist in the store.
var ErrKeyNotFound = errors.New("key not found")

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

// ValkeyConfig holds the configuration for connecting to Valkey.
// Either URL or Host must be set; URL takes precedence.
type ValkeyConfig struct {
	URL      string
	Host     string
	Port     string
	Password string
}

// NewKVStore creates a KVStore based on the provided configuration.
// If Valkey is configured (via URL or Host), a Valkey-backed store is returned;
// otherwise (nil, nil) is returned.
func NewKVStore(cfg ValkeyConfig) (KVStore, error) {
	valkeyURL := cfg.URL
	if valkeyURL == "" {
		valkeyURL = buildValkeyURL(cfg.Host, cfg.Port, cfg.Password)
	}

	if valkeyURL != "" {
		return NewValkeyKVStore(valkeyURL)
	}

	return nil, nil
}

// buildValkeyURL constructs a Valkey URL from host, port, and password.
// Returns "" if host is empty. Uses the redis:// scheme (wire-compatible protocol).
func buildValkeyURL(host, port, password string) string {
	if host == "" {
		return ""
	}
	if port == "" {
		port = "6379"
	}
	var userInfo string
	if password != "" {
		userInfo = ":" + url.QueryEscape(password) + "@"
	}
	return fmt.Sprintf("redis://%s%s:%s/0", userInfo, host, port)
}
