package gitProviderClients

import "context"

// GitProviderClient provides platform-specific repository operations.
type GitProviderClient interface {
	// IsFork returns true if the given project is a fork.
	IsFork(ctx context.Context, project string) (bool, error)
}
