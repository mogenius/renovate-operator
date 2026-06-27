package githubProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"renovate-operator/gitProviderClients"
)

// GitHubClient implements GitProviderClient for the GitHub API.
type GitHubClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *GitHubClient) GetRepositoryInfo(ctx context.Context, project string) (gitProviderClients.RepositoryInfo, error) {
	url := fmt.Sprintf("%s/repos/%s", c.Endpoint, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("github API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Fork bool `json:"fork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("failed to decode GitHub API response for %s: %w", project, err)
	}
	// GitHub deletes repositories immediately and has no pending-deletion state.
	return gitProviderClients.RepositoryInfo{Fork: repo.Fork}, nil
}

func (c *GitHubClient) SearchReposByTopic(ctx context.Context, topic string) ([]gitProviderClients.Repository, error) {
	return nil, fmt.Errorf("searching repositories by topic is not supported by GitHub API")
}

func (c *GitHubClient) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("listing webhooks is not supported by GitHub API")
}

func (c *GitHubClient) CreateRepoWebhook(ctx context.Context, owner, repo string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("creating webhooks is not supported by GitHub API")
}

func (c *GitHubClient) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	return fmt.Errorf("deleting webhooks is not supported by GitHub API")
}
