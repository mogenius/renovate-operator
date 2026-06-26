package giteaProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"renovate-operator/gitProviderClients"
	"strings"
	"time"
)

// GiteaClient implements GitProviderClient for the Gitea and Forgejo APIs.
type GiteaClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

// ProviderName returns the metric provider label for Gitea.
func (c *GiteaClient) ProviderName() string { return "gitea" }

func (c *GiteaClient) GetRepositoryInfo(ctx context.Context, project string) (gitProviderClients.RepositoryInfo, error) {
	//trim /api/v1 if it is included in the endpoint, to avoid double /api/v1 in the URL
	endpoint := strings.TrimSuffix(c.Endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/api/v1")

	// Gitea/Forgejo API: GET /api/v1/repos/{owner}/{repo}
	url := fmt.Sprintf("%s/api/v1/repos/%s", endpoint, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	gitProviderClients.RecordProviderRequest(ctx, "gitea", gitProviderClients.OperationGetRepositoryInfo, start, resp, err)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("gitea API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Fork bool `json:"fork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("failed to decode gitea API response for %s: %w", project, err)
	}
	// Gitea/Forgejo have no pending-deletion state.
	return gitProviderClients.RepositoryInfo{Fork: repo.Fork}, nil
}

func (c *GiteaClient) SearchReposByTopic(ctx context.Context, topic string) ([]gitProviderClients.Repository, error) {
	return nil, fmt.Errorf("searching repositories by topic is not supported by Gitea API")
}

func (c *GiteaClient) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("listing webhooks is not supported by Gitea API")
}

func (c *GiteaClient) CreateRepoWebhook(ctx context.Context, owner, repo string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("creating webhooks is not supported by Gitea API")
}

func (c *GiteaClient) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	return fmt.Errorf("deleting webhooks is not supported by Gitea API")
}
