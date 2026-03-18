package gitlabProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"renovate-operator/gitProviderClients"
)

// GitLabClient implements GitProviderClient for the GitLab API.
type GitLabClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *GitLabClient) IsFork(ctx context.Context, project string) (bool, error) {
	// GitLab endpoint already includes /api/v4, project path must be URL-encoded
	apiURL := fmt.Sprintf("%s/projects/%s", c.Endpoint, url.PathEscape(project))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("gitlab API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var proj struct {
		ForkedFromProject *json.RawMessage `json:"forked_from_project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&proj); err != nil {
		return false, fmt.Errorf("failed to decode GitLab API response for %s: %w", project, err)
	}
	return proj.ForkedFromProject != nil, nil
}

func (c *GitLabClient) SearchReposByTopic(ctx context.Context, topic string) ([]gitProviderClients.Repository, error) {
	return nil, fmt.Errorf("searching repositories by topic is not supported by GitLab API")
}

func (c *GitLabClient) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("listing webhooks is not supported by GitLab API")
}

func (c *GitLabClient) CreateRepoWebhook(ctx context.Context, owner, repo string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("creating webhooks is not supported by GitLab API")
}

func (c *GitLabClient) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	return fmt.Errorf("deleting webhooks is not supported by GitLab API")
}
