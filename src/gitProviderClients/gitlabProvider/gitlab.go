package gitlabProvider

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
)

// GitLabClient implements GitProviderClient for the GitLab API.
type GitLabClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func (c *GitLabClient) GetRepositoryInfo(ctx context.Context, project string) (gitProviderClients.RepositoryInfo, error) {
	// GitLab endpoint already includes /api/v4, project path must be URL-encoded
	apiURL := fmt.Sprintf("%s/projects/%s", c.Endpoint, url.PathEscape(project))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return gitProviderClients.RepositoryInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("gitlab API returned status %d for %s: %s", resp.StatusCode, project, string(body))
	}

	var proj struct {
		ForkedFromProject   *json.RawMessage `json:"forked_from_project"`
		MarkedForDeletionAt *string          `json:"marked_for_deletion_at"`
		MarkedForDeletionOn *string          `json:"marked_for_deletion_on"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&proj); err != nil {
		return gitProviderClients.RepositoryInfo{}, fmt.Errorf("failed to decode GitLab API response for %s: %w", project, err)
	}

	pendingDeletion := (proj.MarkedForDeletionAt != nil && *proj.MarkedForDeletionAt != "") ||
		(proj.MarkedForDeletionOn != nil && *proj.MarkedForDeletionOn != "")
	return gitProviderClients.RepositoryInfo{
		Fork:            proj.ForkedFromProject != nil,
		PendingDeletion: pendingDeletion,
	}, nil
}

func (c *GitLabClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.Endpoint, "/")+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTPClient.Do(req)
}

// wire format of the GitLab project hooks API. GitLab models events as boolean
// flags instead of an event list; Token is write-only (never returned).
type gitlabHook struct {
	ID                  int64  `json:"id,omitempty"`
	URL                 string `json:"url"`
	Token               string `json:"token,omitempty"`
	PushEvents          bool   `json:"push_events"`
	IssuesEvents        bool   `json:"issues_events"`
	MergeRequestsEvents bool   `json:"merge_requests_events"`
	NoteEvents          bool   `json:"note_events"`
	TagPushEvents       bool   `json:"tag_push_events"`
}

func (h gitlabHook) toWebhook() gitProviderClients.Webhook {
	return gitProviderClients.Webhook{
		ID:  strconv.FormatInt(h.ID, 10),
		URL: h.URL,
		// GitLab hooks have no enabled/disabled state — existing means active.
		Active: true,
		// Operator-managed hooks subscribe to exactly issue + merge request
		// events (the Renovate checkbox triggers).
		EventsUpToDate: h.IssuesEvents && h.MergeRequestsEvents &&
			!h.PushEvents && !h.NoteEvents && !h.TagPushEvents,
	}
}

func (c *GitLabClient) ListRepoWebhooks(ctx context.Context, project string) ([]gitProviderClients.Webhook, error) {
	var allHooks []gitProviderClients.Webhook
	page := 1
	limit := 50

	for {
		path := fmt.Sprintf("/projects/%s/hooks?per_page=%d&page=%d", url.PathEscape(project), limit, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		var hooks []gitlabHook
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

func (c *GitLabClient) CreateRepoWebhook(ctx context.Context, project string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := gitlabHook{
		URL: opts.URL,
		// GitLab sends the token back in the X-Gitlab-Token header.
		Token: opts.AuthToken,
		// fixed subscription: the events used by the Renovate checkbox triggers
		IssuesEvents:        true,
		MergeRequestsEvents: true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/projects/%s/hooks", url.PathEscape(project)), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	var hook gitlabHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *GitLabClient) UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts gitProviderClients.CreateWebhookOptions) (*gitProviderClients.Webhook, error) {
	payload := gitlabHook{
		URL:   opts.URL,
		Token: opts.AuthToken,
		// fixed subscription: the events used by the Renovate checkbox triggers
		IssuesEvents:        true,
		MergeRequestsEvents: true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	path := fmt.Sprintf("/projects/%s/hooks/%s", url.PathEscape(project), hookID)
	resp, err := c.doRequest(ctx, http.MethodPut, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}

	var hook gitlabHook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("updating webhook: %w", err)
	}
	result := hook.toWebhook()
	return &result, nil
}

func (c *GitLabClient) DeleteRepoWebhook(ctx context.Context, project string, hookID string) error {
	path := fmt.Sprintf("/projects/%s/hooks/%s", url.PathEscape(project), hookID)
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

func decodeResponse(resp *http.Response, target any) error {
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
