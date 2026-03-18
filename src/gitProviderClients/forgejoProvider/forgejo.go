package forgejoProvider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ForgejoClient implements GitProviderClient for the Forgejo API.
type ForgejoClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *ForgejoClient) IsFork(ctx context.Context, project string) (bool, error) {
	// Forgejo API: GET /api/v1/repos/{owner}/{repo}

	//trim /api/v1 if it is included in the endpoint, to avoid double /api/v1 in the URL
	endpoint := strings.TrimSuffix(c.Endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/api/v1")

	url := fmt.Sprintf("%s/api/v1/repos/%s", endpoint, project)
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
		return false, fmt.Errorf("forgejo API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Fork bool `json:"fork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return false, fmt.Errorf("failed to decode forgejo API response for %s: %w", project, err)
	}
	return repo.Fork, nil
}
