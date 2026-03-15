package gitprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GitHubClient implements GitProviderClient for the GitHub API.
type GitHubClient struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

func (c *GitHubClient) IsFork(ctx context.Context, project string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s", c.endpoint, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("github API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Fork bool `json:"fork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return false, fmt.Errorf("failed to decode GitHub API response for %s: %w", project, err)
	}
	return repo.Fork, nil
}
