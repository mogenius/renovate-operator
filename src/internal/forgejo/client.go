package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the interface for interacting with the Forgejo REST API.
type Client interface {
	SearchReposByTopic(ctx context.Context, topic string) ([]Repository, error)
	ListRepoWebhooks(ctx context.Context, owner, repo string) ([]Webhook, error)
	CreateRepoWebhook(ctx context.Context, owner, repo string, opts CreateWebhookOptions) (*Webhook, error)
	DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error
}

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name        string              `json:"name"`
	Permissions *RepositoryPermissions `json:"permissions,omitempty"`
}

type RepositoryPermissions struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

type Webhook struct {
	ID     int64         `json:"id"`
	Type   string        `json:"type"`
	Config WebhookConfig `json:"config"`
	Events []string      `json:"events"`
	Active bool          `json:"active"`
}

type WebhookConfig struct {
	URL                  string `json:"url"`
	ContentType          string `json:"content_type"`
	AuthorizationHeader  string `json:"authorization_header,omitempty"`
}

type CreateWebhookOptions struct {
	Type   string        `json:"type"`
	Config WebhookConfig `json:"config"`
	Events []string      `json:"events"`
	Active bool          `json:"active"`
}

type client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Forgejo API client.
func NewClient(baseURL, token string) Client {
	return &client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *client) SearchReposByTopic(ctx context.Context, topic string) ([]Repository, error) {
	var allRepos []Repository
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
			Data []Repository `json:"data"`
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

func (c *client) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]Webhook, error) {
	var allHooks []Webhook
	page := 1
	limit := 50

	for {
		path := fmt.Sprintf("/api/v1/repos/%s/%s/hooks?limit=%d&page=%d", owner, repo, limit, page)
		resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("listing webhooks: %w", err)
		}

		var hooks []Webhook
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

func (c *client) CreateRepoWebhook(ctx context.Context, owner, repo string, opts CreateWebhookOptions) (*Webhook, error) {
	body, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("marshalling webhook options: %w", err)
	}

	path := fmt.Sprintf("/api/v1/repos/%s/%s/hooks", owner, repo)
	resp, err := c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}

	var hook Webhook
	if err := decodeResponse(resp, &hook); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	return &hook, nil
}

func (c *client) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	path := fmt.Sprintf("/api/v1/repos/%s/%s/hooks/%d", owner, repo, hookID)
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func decodeResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

