package kvstore

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

var ErrValkeyNotConfigured = errors.New("valkey is not configured")

const (
	ValkeyDataBaseSessionStore  = 0
	ValkeyDataBaseRenovateCache = 1
	ValkeyDataBaseRenovateLogs  = 2
)

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

func (cfg *ValkeyConfig) IsConfigured() bool {
	return cfg.URL != "" || cfg.Host != ""
}

// NewKVStore creates a KVStore based on the provided configuration.
// If Valkey is configured (via URL or Host), a Valkey-backed store is returned;
// otherwise (nil, nil) is returned.
func NewKVStore(cfg ValkeyConfig, db int) (KVStore, error) {
	if !cfg.IsConfigured() {
		return nil, ErrValkeyNotConfigured
	}

	valkeyURL := cfg.URL
	if valkeyURL == "" {
		valkeyURL = BuildValkeyURL(cfg.Host, cfg.Port, cfg.Password, db)
	} else {
		valkeyURL = withDB(valkeyURL, db)
	}

	if valkeyURL == "" {
		return nil, nil
	}

	return NewValkeyKVStore(valkeyURL)
}

// withDB replaces the database path component of a Valkey/Redis URL.
func withDB(rawURL string, db int) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Path = fmt.Sprintf("/%d", db)
	return u.String()
}

// BuildValkeyURL constructs a Valkey URL from host, port, and password.
// Returns "" if host is empty. Uses the redis:// scheme (wire-compatible protocol).
func BuildValkeyURL(host, port, password string, db int) string {
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
	return fmt.Sprintf("redis://%s%s:%s/%d", userInfo, host, port, db)
}
