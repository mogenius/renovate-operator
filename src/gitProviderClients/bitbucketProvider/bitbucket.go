package bitbucketProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
