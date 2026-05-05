package logStore

import (
	"context"
	"errors"
	"time"

	"renovate-operator/internal/kvstore"

	"github.com/go-logr/logr"
)

// logTTL is the retention period for logs in Valkey.
// Entries are overwritten on each new run, so this guards against orphaned keys.
const logTTL = 30 * 24 * time.Hour

// valkeyLogStore is the Valkey-backed LogStore implementation.
// When kv is nil (Valkey not configured), Save is a no-op and Get returns an
// informational error message so the UI surfaces the misconfiguration.
type valkeyLogStore struct {
	kv     kvstore.KVStore
	logger logr.Logger
}

func (s *valkeyLogStore) Save(namespace, renovateJob, project, logs string) {
	if s.kv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := s.kv.Put(ctx, key(namespace, renovateJob, project), []byte(logs), logTTL)
	if err != nil {
		s.logger.Error(err, "failed to save logs to valkey")
	}
}

func (s *valkeyLogStore) Get(namespace, renovateJob, project string) (string, bool) {
	if s.kv == nil {
		return "log store unavailable: LOG_STORE_MODE is valkey but no Valkey backend is configured", true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := s.kv.Get(ctx, key(namespace, renovateJob, project))
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		return "", false
	}
	if err != nil {
		return "", false
	}
	return string(data), true
}
