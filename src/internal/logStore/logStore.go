package logStore

import (
	"fmt"

	"renovate-operator/internal/kvstore"

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
// "valkey" (Valkey-backed; initialises its own KVStore on DB 1).
// cfg is ignored for non-valkey modes.
func NewLogStore(logger logr.Logger, mode string, cfg kvstore.ValkeyConfig) (LogStore, error) {
	switch mode {
	case "memory":
		return &memoryLogStore{data: make(map[string]string)}, nil
	case "valkey":
		kv, err := kvstore.NewKVStore(cfg, kvstore.ValkeyDataBaseRenovateLogs)
		if err != nil {
			return nil, err
		}
		return &valkeyLogStore{kv: kv, logger: logger}, nil
	default:
		return noopLogStore{}, nil
	}
}

func key(namespace, renovateJob, project string) string {
	return fmt.Sprintf("RENOVATE_LOGS:%s:%s:%s", namespace, renovateJob, project)
}
