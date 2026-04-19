package forgejoProvider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"renovate-operator/gitProviderClients"
	"strconv"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ForgejoClient implements GitProviderClient for the Forgejo API.
type ForgejoClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func NewClient(endpoint, token string) *ForgejoClient {
	return &ForgejoClient{
		Endpoint:   endpoint,
		Token:      token,
		HTTPClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
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

func (c *ForgejoClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	reqURL := c.Endpoint + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTPClient.Do(req)
}

func (c *ForgejoClient) SearchReposByTopic(ctx context.Context, topic string) ([]gitProviderClients.Repository, error) {
	var allRepos []gitProviderClients.Repository
	page := 1
	limit := 50

	for {
		params := url.Values{}
		params.Set("topic", "true")
		params.Set("q", topic)
		params.Set("limit", strconv.Itoa(limit))
		params.Set("page", strconv.Itoa(page))

		resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/repos/search?"+params.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("searching repos by topic: %w", err)
		}

		var result struct {
			Data []gitProviderClients.Repository `json:"data"`
		}
		if err := decodeResponse(resp, &result); err != nil {
			return nil, fmt.Errorf("searching repos by topic: %w", err)
		}

		if len(result.Data) == 0 {
			break
		}
		allRepos = append(allRepos, result.Data...)
		if len(result.Data) < limit {
			break
		}
		page++
	}

	return allRepos, nil
}

func (c *ForgejoClient) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]gitProviderClients.Webhook, error) {
	var allHooks []gitProviderClients.Webhook
	page := 1
	limit := 50

	for {
		path := fmt.Sprintf("/api/v1/repos/%s/%s/hooks?limit=%d&page=%d", owner, repo, limit, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		var hooks []gitProviderClients.Webhook
		if err := decodeResponse(resp, &hooks); err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		if len(hooks) == 0 {
			break
		}
		allHooks = append(allHooks, hooks...)
		if len(hooks) < limit {
			break
		}
		page++
	}

	return allHooks, nil
}

func (c *ForgejoClient) CreateRepoWebhook(ctx context.Context, owner, repo string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	body, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	path := fmt.Sprintf("/api/v1/repos/%s/%s/hooks", owner, repo)
	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	var hook gitProviderClients.Webhook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	return &hook, nil
}

func (c *ForgejoClient) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	path := fmt.Sprintf("/api/v1/repos/%s/%s/hooks/%d", owner, repo, hookID)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func decodeResponse(resp *http.Response, target interface{}) error {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
