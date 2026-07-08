package logStore

import (
	"context"
	"fmt"

	"renovate-operator/internal/kvstore"
	"renovate-operator/internal/objectstore"

	"github.com/go-logr/logr"
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

// NewLogStore creates a LogStore based on the provided mode.
// Supported modes: "disabled" (default, no-op), "memory" (in-memory store),
// "valkey" (Valkey-backed; initialises its own KVStore on DB 1),
// "s3" (S3-backed; s3Cfg.Bucket must be set, logPrefix sets the object key prefix).
// valkeyCfg is ignored for non-valkey modes; s3Cfg and logPrefix are ignored for non-s3 modes.
func NewLogStore(logger logr.Logger, mode string, valkeyCfg kvstore.ValkeyConfig, s3Cfg objectstore.S3Config, logPrefix string) (LogStore, error) {
	switch mode {
	case "memory":
		return &memoryLogStore{data: make(map[string]string)}, nil
	case "valkey":
		kv, err := kvstore.NewKVStore(valkeyCfg, kvstore.UsageRenovateLogs)
		if err != nil {
			return nil, err
		}
		return &valkeyLogStore{kv: kv, logger: logger}, nil
	case "s3":
		store, err := newS3LogStore(context.Background(), s3Cfg, logPrefix, logger)
		if err != nil {
			return nil, fmt.Errorf("initializing S3 log store: %w", err)
		}
		return store, nil
	default:
		return noopLogStore{}, nil
	}
}

func key(namespace, renovateJob, project string) string {
	return fmt.Sprintf("RENOVATE_LOGS:%s:%s:%s", namespace, renovateJob, project)
}
