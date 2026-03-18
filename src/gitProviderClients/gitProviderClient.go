package gitProviderClients

import "context"

// GitProviderClient provides platform-specific repository operations.
type GitProviderClient interface {
	// IsFork returns true if the given project is a fork.
	IsFork(ctx context.Context, project string) (bool, error)

	SearchReposByTopic(ctx context.Context, topic string) ([]Repository, error)
	ListRepoWebhooks(ctx context.Context, owner, repo string) ([]Webhook, error)
	CreateRepoWebhook(ctx context.Context, owner, repo string, opts CreateWebhookOptions) (*Webhook, error)
	DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error
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
