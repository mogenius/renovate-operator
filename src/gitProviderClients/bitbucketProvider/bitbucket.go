package bitbucketProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"renovate-operator/gitProviderClients"
)

// BitbucketClient implements GitProviderClient for the Bitbucket Cloud API.
type BitbucketClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *BitbucketClient) IsFork(ctx context.Context, project string) (bool, error) {
	// Bitbucket Cloud API: GET /2.0/repositories/{workspace}/{repo_slug}
	url := fmt.Sprintf("%s/2.0/repositories/%s", c.Endpoint, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("bitbucket API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Parent *json.RawMessage `json:"parent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return false, fmt.Errorf("failed to decode bitbucket API response for %s: %w", project, err)
	}
	return repo.Parent != nil, nil
}

func (c *BitbucketClient) SearchReposByTopic(ctx context.Context, topic string) ([]gitProviderClients.Repository, error) {
	return nil, fmt.Errorf("searching repositories by topic is not supported by Bitbucket API")
}

func (c *BitbucketClient) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("listing webhooks is not supported by Bitbucket API")
}

func (c *BitbucketClient) CreateRepoWebhook(ctx context.Context, owner, repo string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	return nil, fmt.Errorf("creating webhooks is not supported by Bitbucket API")
}

func (c *BitbucketClient) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	return fmt.Errorf("deleting webhooks is not supported by Bitbucket API")
}
