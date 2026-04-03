package logStore

import "fmt"

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
// Supported modes: "disabled" (default, no-op), "memory" (in-memory store).
func NewLogStore(mode string) LogStore {
	switch mode {
	case "memory":
		return &memoryLogStore{data: make(map[string]string)}
	default:
		return noopLogStore{}
	}
}

func key(namespace, renovateJob, project string) string {
	return fmt.Sprintf("RENOVATE_LOGS:%s:%s:%s", namespace, renovateJob, project)
}
