package gitProviderClients

import "context"

// GitProviderClient provides platform-specific repository operations.
type GitProviderClient interface {
	// GetRepositoryInfo returns skip-relevant metadata about a project. The
	// metadata is fetched in a single platform API call so that fork and
	// pending-deletion filtering do not each incur their own request.
	GetRepositoryInfo(ctx context.Context, project string) (RepositoryInfo, error)

	ListRepoWebhooks(ctx context.Context, project string) ([]Webhook, error)
	CreateRepoWebhook(ctx context.Context, project string, opts CreateWebhookOptions) (*Webhook, error)
	UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts CreateWebhookOptions) (*Webhook, error)
	DeleteRepoWebhook(ctx context.Context, project string, hookID string) error
}

// RepositoryInfo captures repo attributes used to decide whether to skip a
// project during discovery. Not every provider exposes every attribute;
// unsupported attributes are reported as false.
type RepositoryInfo struct {
	// Fork is true if the repository is a fork.
	Fork bool
	// PendingDeletion is true if the repository is marked for delayed deletion.
	// Only GitLab exposes this state.
	PendingDeletion bool
}

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name        string                 `json:"name"`
	Permissions *RepositoryPermissions `json:"permissions,omitempty"`
}

type RepositoryPermissions struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

// Webhook is the provider-agnostic view of a repository webhook.
type Webhook struct {
	// ID of the hook in the platform's identifier format (numeric for
	// Forgejo/Gitea/GitHub/GitLab, a UUID for Bitbucket).
	ID             string
	URL            string
	Active         bool
	EventsUpToDate bool
}

// CreateWebhookOptions describes a webhook to create in provider-agnostic
// terms.
type CreateWebhookOptions struct {
	URL       string
	AuthToken string
	Active    bool
}
