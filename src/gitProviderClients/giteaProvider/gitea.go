package giteaProvider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"renovate-operator/gitProviderClients"
	"strconv"
	"strings"
)

// GiteaClient implements GitProviderClient for the Gitea and Forgejo APIs.
type GiteaClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

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

	resp, err := c.HTTPClient.Do(req)
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

func (c *GiteaClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	//trim /api/v1 if it is included in the endpoint, to avoid double /api/v1 in the URL
	endpoint := strings.TrimSuffix(c.Endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/api/v1")

	req, err := http.NewRequestWithContext(ctx, method, endpoint+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTPClient.Do(req)
}

// wire format of the Gitea/Forgejo hooks API
type giteaHook struct {
	ID int64 `json:"id"`
	// Type is required on create; edits leave it empty (it cannot change).
	Type   string          `json:"type,omitempty"`
	Config giteaHookConfig `json:"config"`
	Events []string        `json:"events"`
	Active bool            `json:"active"`
}

type giteaHookConfig struct {
	URL                 string `json:"url"`
	ContentType         string `json:"content_type"`
	AuthorizationHeader string `json:"authorization_header,omitempty"`
}

// giteaWebhookEvents is the fixed subscription for operator-managed hooks:
// just the base issue and pull request events. The aggregate names "issues"/
// "pull_request" would enable every sub-event (assign, label, milestone,
// comment, review, sync), which the operator never needs.
var giteaWebhookEvents = []string{"issues_only", "pull_request_only"}

// giteaReportedEvents is how the hooks API reports that subscription back:
// the enabled base flags are listed as "issues"/"pull_request".
var giteaReportedEvents = []string{"issues", "pull_request"}

func (h giteaHook) toWebhook() gitProviderClients.Webhook {
	return gitProviderClients.Webhook{
		ID:             strconv.FormatInt(h.ID, 10),
		URL:            h.Config.URL,
		Active:         h.Active,
		EventsUpToDate: eventsEqual(h.Events, giteaReportedEvents),
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

func (c *GiteaClient) ListRepoWebhooks(ctx context.Context, project string) ([]gitProviderClients.Webhook, error) {
	var allHooks []gitProviderClients.Webhook
	page := 1
	limit := 50

	for {
		path := fmt.Sprintf("/api/v1/repos/%s/hooks?limit=%d&page=%d", project, limit, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		var hooks []giteaHook
		if err := decodeResponse(resp, &hooks); err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		if len(hooks) == 0 {
			break
		}
		for _, hook := range hooks {
			allHooks = append(allHooks, hook.toWebhook())
		}
		if len(hooks) < limit {
			break
		}
		page++
	}

	return allHooks, nil
}

func (c *GiteaClient) CreateRepoWebhook(ctx context.Context, project string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := giteaHook{
		Type: "gitea",
		Config: giteaHookConfig{
			URL:         opts.URL,
			ContentType: "json",
		},
		Events: giteaWebhookEvents,
		Active: opts.Active,
	}
	if opts.AuthToken != "" {
		payload.Config.AuthorizationHeader = "Bearer " + opts.AuthToken
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	path := fmt.Sprintf("/api/v1/repos/%s/hooks", project)
	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	var hook giteaHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *GiteaClient) UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := giteaHook{
		Config: giteaHookConfig{
			URL:         opts.URL,
			ContentType: "json",
		},
		Events: giteaWebhookEvents,
		Active: opts.Active,
	}
	if opts.AuthToken != "" {
		payload.Config.AuthorizationHeader = "Bearer " + opts.AuthToken
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	path := fmt.Sprintf("/api/v1/repos/%s/hooks/%s", project, hookID)
	resp, err := c.doRequest(ctx, http.MethodPatch, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	var hook giteaHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *GiteaClient) DeleteRepoWebhook(ctx context.Context, project string, hookID string) error {
	path := fmt.Sprintf("/api/v1/repos/%s/hooks/%s", project, hookID)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A missing webhook is the desired end state, so treat 404 as success.
	// Drain the body first so the connection can be reused (Gitea returns JSON on 404).
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
