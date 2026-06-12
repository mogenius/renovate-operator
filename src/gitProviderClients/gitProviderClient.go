package gitProviderClients

import "context"

// GitProviderClient provides platform-specific repository operations.
type GitProviderClient interface {
	// GetRepositoryInfo returns skip-relevant metadata about a project. The
	// metadata is fetched in a single platform API call so that fork and
	// pending-deletion filtering do not each incur their own request.
	GetRepositoryInfo(ctx context.Context, project string) (RepositoryInfo, error)

	SearchReposByTopic(ctx context.Context, topic string) ([]Repository, error)
	ListRepoWebhooks(ctx context.Context, owner, repo string) ([]Webhook, error)
	CreateRepoWebhook(ctx context.Context, owner, repo string, opts CreateWebhookOptions) (*Webhook, error)
	DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error
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

type Webhook struct {
	ID     int64         `json:"id"`
	Type   string        `json:"type"`
	Config WebhookConfig `json:"config"`
	Events []string      `json:"events"`
	Active bool          `json:"active"`
}

type WebhookConfig struct {
	URL                 string `json:"url"`
	ContentType         string `json:"content_type"`
	AuthorizationHeader string `json:"authorization_header,omitempty"`
}

type CreateWebhookOptions struct {
	Type   string        `json:"type"`
	Config WebhookConfig `json:"config"`
	Events []string      `json:"events"`
	Active bool          `json:"active"`
}
