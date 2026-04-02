package logStore

// noopLogStore is the default implementation when LOG_STORE_MODE=disabled.
// All operations are no-ops so there is zero overhead.
type noopLogStore struct{}

func (noopLogStore) Save(_, _, _, _ string)            {}
func (noopLogStore) Get(_, _, _ string) (string, bool) { return "", false }
