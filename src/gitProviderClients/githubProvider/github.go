package githubProvider

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

func (c *GitHubClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.Endpoint, "/")+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTPClient.Do(req)
}

// wire format of the GitHub repository webhooks API
type githubHook struct {
	ID int64 `json:"id,omitempty"`
	// Name is required on create ("web"); edits leave it empty (immutable).
	Name   string           `json:"name,omitempty"`
	Config githubHookConfig `json:"config"`
	Events []string         `json:"events"`
	Active bool             `json:"active"`
}

type githubHookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	// Secret is the HMAC key GitHub uses for the X-Hub-Signature-256 header.
	Secret string `json:"secret,omitempty"`
}

// githubWebhookEvents is the fixed subscription for operator-managed hooks:
// the issue and pull request events used by the Renovate checkbox triggers.
var githubWebhookEvents = []string{"issues", "pull_request"}

func (h githubHook) toWebhook() gitProviderClients.Webhook {
	return gitProviderClients.Webhook{
		ID:             strconv.FormatInt(h.ID, 10),
		URL:            h.Config.URL,
		Active:         h.Active,
		EventsUpToDate: eventsEqual(h.Events, githubWebhookEvents),
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

func (c *GitHubClient) ListRepoWebhooks(ctx context.Context, project string) ([]gitProviderClients.Webhook, error) {
	var allHooks []gitProviderClients.Webhook
	page := 1
	limit := 50

	for {
		path := fmt.Sprintf("/repos/%s/hooks?per_page=%d&page=%d", project, limit, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		var hooks []githubHook
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

func (c *GitHubClient) CreateRepoWebhook(ctx context.Context, project string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := githubHook{
		Name: "web",
		Config: githubHookConfig{
			URL:         opts.URL,
			ContentType: "json",
			Secret:      opts.AuthToken,
		},
		Events: githubWebhookEvents,
		Active: opts.Active,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/hooks", project), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	var hook githubHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *GitHubClient) UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := githubHook{
		Config: githubHookConfig{
			URL:         opts.URL,
			ContentType: "json",
			Secret:      opts.AuthToken,
		},
		Events: githubWebhookEvents,
		Active: opts.Active,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/hooks/%s", project, hookID), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	var hook githubHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *GitHubClient) DeleteRepoWebhook(ctx context.Context, project string, hookID string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/repos/%s/hooks/%s", project, hookID), nil)
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

func decodeResponse(resp *http.Response, target any) error {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
