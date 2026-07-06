package githubProvider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"renovate-operator/gitProviderClients"
)

func newTestClient(url string) *GitHubClient {
	return &GitHubClient{Endpoint: url, Token: "test-token", HTTPClient: http.DefaultClient}
}

func TestListRepoWebhooks(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected bearer auth, got %q", r.Header.Get("Authorization"))
		}
		hooks := []githubHook{
			{ID: 1, Name: "web", Config: githubHookConfig{URL: "https://example.com/webhook"}, Active: true},
		}
		_ = json.NewEncoder(w).Encode(hooks)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	hooks, err := newTestClient(srv.URL).ListRepoWebhooks(context.Background(), "org/repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	if hooks[0].URL != "https://example.com/webhook" {
		t.Errorf("expected webhook URL, got %s", hooks[0].URL)
	}
}

func TestCreateRepoWebhook(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var payload githubHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Name != "web" {
			t.Errorf("expected hook name web, got %s", payload.Name)
		}
		if payload.Config.URL != "https://example.com/webhook" {
			t.Errorf("expected webhook URL, got %s", payload.Config.URL)
		}
		if payload.Config.ContentType != "json" {
			t.Errorf("expected content type json, got %s", payload.Config.ContentType)
		}
		if payload.Config.Secret != "secret" {
			t.Errorf("expected HMAC secret, got %s", payload.Config.Secret)
		}

		w.WriteHeader(http.StatusCreated)
		payload.ID = 42
		_ = json.NewEncoder(w).Encode(payload)
	})

	srv := httptest.NewServer(handler)
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
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/org/repo1/hooks/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}

		var payload githubHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Config.Secret != "secret" {
			t.Errorf("expected HMAC secret, got %s", payload.Config.Secret)
		}
		if len(payload.Events) != 2 || payload.Events[0] != "issues" || payload.Events[1] != "pull_request" {
			t.Errorf("expected the fixed subscription, got %v", payload.Events)
		}

		payload.ID = 42
		_ = json.NewEncoder(w).Encode(payload)
	})

	srv := httptest.NewServer(handler)
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
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/org/repo1/hooks/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	err := newTestClient(srv.URL).DeleteRepoWebhook(context.Background(), "org/repo1", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
