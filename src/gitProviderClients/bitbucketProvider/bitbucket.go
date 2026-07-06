package bitbucketProvider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"renovate-operator/gitProviderClients"
)

// BitbucketClient implements GitProviderClient for the Bitbucket Cloud API.
type BitbucketClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *BitbucketClient) GetRepositoryInfo(ctx context.Context, project string) (gitProviderClients.RepositoryInfo, error) {
	// Bitbucket Cloud API: GET /2.0/repositories/{workspace}/{repo_slug}
	url := fmt.Sprintf("%s/2.0/repositories/%s", c.Endpoint, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("bitbucket API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var repo struct {
		Parent *json.RawMessage `json:"parent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("failed to decode bitbucket API response for %s: %w", project, err)
	}
	// Bitbucket Cloud has no pending-deletion state.
	return gitProviderClients.RepositoryInfo{Fork: repo.Parent != nil}, nil
}

func (c *BitbucketClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.Endpoint, "/")+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTPClient.Do(req)
}

// wire format of the Bitbucket Cloud webhook subscriptions API. Hooks are
// identified by a UUID (including curly braces); Secret is write-only.
type bitbucketHook struct {
	UUID        string   `json:"uuid,omitempty"`
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	Secret      string   `json:"secret,omitempty"`
	Active      bool     `json:"active"`
	Events      []string `json:"events"`
}

// bitbucketWebhookEvents is the fixed subscription for operator-managed hooks:
// pull request description edits (checkbox interactions), merges, and
// declines. Bitbucket Cloud has no Dependency Dashboard (Renovate does not
// support it there), so no issue events are needed.
var bitbucketWebhookEvents = []string{"pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected"}

func (h bitbucketHook) toWebhook() gitProviderClients.Webhook {
	return gitProviderClients.Webhook{
		ID:             h.UUID,
		URL:            h.URL,
		Active:         h.Active,
		EventsUpToDate: eventsEqual(h.Events, bitbucketWebhookEvents),
	}
}

// eventsEqual compares two event name lists as sets.
func eventsEqual(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	set := make(map[string]struct{}, len(expected))
	for _, event := range expected {
		set[event] = struct{}{}
	}
	for _, event := range actual {
		if _, ok := set[event]; !ok {
			return false
		}
	}
	return true
}

func (c *BitbucketClient) ListRepoWebhooks(ctx context.Context, project string) ([]gitProviderClients.Webhook, error) {
	var allHooks []gitProviderClients.Webhook
	page := 1
	limit := 50

	for {
		path := fmt.Sprintf("/2.0/repositories/%s/hooks?pagelen=%d&page=%d", project, limit, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		var result struct {
			Values []bitbucketHook `json:"values"`
			Next   string          `json:"next"`
		}
		if err := decodeResponse(resp, &result); err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		for _, hook := range result.Values {
			allHooks = append(allHooks, hook.toWebhook())
		}
		if result.Next == "" {
			break
		}
		page++
	}

	return allHooks, nil
}

func (c *BitbucketClient) CreateRepoWebhook(ctx context.Context, project string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := bitbucketHook{
		URL:         opts.URL,
		Description: "renovate-operator",
		Secret:      opts.AuthToken,
		Active:      opts.Active,
		Events:      bitbucketWebhookEvents,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/2.0/repositories/%s/hooks", project), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	var hook bitbucketHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *BitbucketClient) UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := bitbucketHook{
		URL:         opts.URL,
		Description: "renovate-operator",
		Secret:      opts.AuthToken,
		Active:      opts.Active,
		Events:      bitbucketWebhookEvents,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	// hook UUIDs include curly braces and must be path-escaped
	path := fmt.Sprintf("/2.0/repositories/%s/hooks/%s", project, url.PathEscape(hookID))
	resp, err := c.doRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	var hook bitbucketHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *BitbucketClient) DeleteRepoWebhook(ctx context.Context, project string, hookID string) error {
	path := fmt.Sprintf("/2.0/repositories/%s/hooks/%s", project, url.PathEscape(hookID))
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A missing webhook is the desired end state, so treat 404 as success.
	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func decodeResponse(resp *http.Response, target any) error {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
