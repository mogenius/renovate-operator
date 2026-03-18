package giteaProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GiteaClient implements GitProviderClient for the Gitea and Forgejo APIs.
type GiteaClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *GiteaClient) IsFork(ctx context.Context, project string) (bool, error) {
	// Gitea/Forgejo API: GET /api/v1/repos/{owner}/{repo}
	url := fmt.Sprintf("%s/api/v1/repos/%s", c.Endpoint, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("gitea API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Fork bool `json:"fork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return false, fmt.Errorf("failed to decode gitea API response for %s: %w", project, err)
	}
	return repo.Fork, nil
}
