package gitlabProvider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"renovate-operator/gitProviderClients"
	"testing"
)

func TestGetRepositoryInfo(t *testing.T) {
	tests := []struct {
		name string
		body string
		want gitProviderClients.RepositoryInfo
	}{
		{
			name: "plain active repo",
			body: `{"archived": false}`,
			want: gitProviderClients.RepositoryInfo{},
		},
		{
			name: "fork",
			body: `{"forked_from_project": {"id": 1}}`,
			want: gitProviderClients.RepositoryInfo{Fork: true},
		},
		{
			name: "marked for deletion (newer field)",
			body: `{"marked_for_deletion_at": "2026-01-01"}`,
			want: gitProviderClients.RepositoryInfo{PendingDeletion: true},
		},
		{
			name: "marked for deletion (legacy field)",
			body: `{"marked_for_deletion_on": "2026-01-01"}`,
			want: gitProviderClients.RepositoryInfo{PendingDeletion: true},
		},
		{
			name: "empty deletion field is not pending",
			body: `{"marked_for_deletion_at": ""}`,
			want: gitProviderClients.RepositoryInfo{},
		},
		{
			name: "fork and pending deletion",
			body: `{"forked_from_project": {"id": 1}, "marked_for_deletion_at": "2026-01-01"}`,
			want: gitProviderClients.RepositoryInfo{Fork: true, PendingDeletion: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
					t.Errorf("expected PRIVATE-TOKEN 'test-token', got %q", r.Header.Get("PRIVATE-TOKEN"))
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := &GitLabClient{Endpoint: srv.URL, Token: "test-token", HTTPClient: srv.Client()}
			got, err := c.GetRepositoryInfo(context.Background(), "group/repo")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestListRepoWebhooks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// subgroup paths must arrive URL-encoded as a single path segment
		if r.URL.EscapedPath() != "/projects/group%2Fsub%2Frepo1/hooks" {
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
		}
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Errorf("expected private token header, got %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		hooks := []gitlabHook{
			{ID: 1, URL: "https://example.com/webhook", IssuesEvents: true, MergeRequestsEvents: true},
		}
		_ = json.NewEncoder(w).Encode(hooks)
	}))
	defer srv.Close()

	hooks, err := newTestClient(srv.URL).ListRepoWebhooks(context.Background(), "group/sub/repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	if hooks[0].URL != "https://example.com/webhook" {
		t.Errorf("expected webhook URL, got %s", hooks[0].URL)
	}
	// issue + MR flags without extras means the subscription is up to date
	if !hooks[0].EventsUpToDate {
		t.Error("expected hook with issue+MR flags to report EventsUpToDate")
	}
}

func TestCreateRepoWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.EscapedPath() != "/projects/org%2Frepo1/hooks" {
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
		}

		var payload gitlabHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.URL != "https://example.com/webhook" {
			t.Errorf("expected webhook URL, got %s", payload.URL)
		}
		if payload.Token != "secret" {
			t.Errorf("expected hook token, got %s", payload.Token)
		}
		if !payload.IssuesEvents || !payload.MergeRequestsEvents {
			t.Errorf("expected issues+merge request events enabled, got %+v", payload)
		}
		if payload.PushEvents {
			t.Error("expected push events disabled")
		}

		w.WriteHeader(http.StatusCreated)
		payload.ID = 42
		payload.Token = "" // GitLab never returns the token
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	hook, err := newTestClient(srv.URL).CreateRepoWebhook(context.Background(), "org/repo1", gitProviderClients.CreateWebhookOptions{
		URL:       "https://example.com/webhook",
		AuthToken: "secret",
		Active:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.ID != "42" {
		t.Errorf("expected hook ID 42, got %s", hook.ID)
	}
}

func TestUpdateRepoWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.EscapedPath() != "/projects/org%2Frepo1/hooks/42" {
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
		}

		var payload gitlabHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if !payload.IssuesEvents || !payload.MergeRequestsEvents {
			t.Errorf("expected issues+merge request events enabled, got %+v", payload)
		}

		payload.ID = 42
		payload.Token = ""
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	hook, err := newTestClient(srv.URL).UpdateRepoWebhook(context.Background(), "org/repo1", "42", gitProviderClients.CreateWebhookOptions{
		URL:       "https://example.com/webhook",
		AuthToken: "secret",
		Active:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.ID != "42" {
		t.Errorf("expected hook ID 42, got %s", hook.ID)
	}
}

func TestDeleteRepoWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.EscapedPath() != "/projects/org%2Frepo1/hooks/42" {
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).DeleteRepoWebhook(context.Background(), "org/repo1", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTestClient(url string) *GitLabClient {
	return &GitLabClient{Endpoint: url, Token: "test-token", HTTPClient: http.DefaultClient}
}
