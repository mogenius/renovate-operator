package giteaProvider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"renovate-operator/gitProviderClients"
)

func newTestClient(url string) *GiteaClient {
	return &GiteaClient{Endpoint: url, Token: "test-token", HTTPClient: http.DefaultClient}
}

func TestCreateRepoWebhook(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var payload giteaHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Type != "gitea" {
			t.Errorf("expected hook type gitea, got %s", payload.Type)
		}
		// the provider applies its fixed subscription
		if len(payload.Events) != 2 || payload.Events[0] != "issues_only" || payload.Events[1] != "pull_request_only" {
			t.Errorf("expected events [issues_only pull_request_only], got %v", payload.Events)
		}
		if payload.Config.URL != "https://example.com/webhook" {
			t.Errorf("expected webhook URL, got %s", payload.Config.URL)
		}
		if payload.Config.AuthorizationHeader != "Bearer secret" {
			t.Errorf("expected authorization header, got %s", payload.Config.AuthorizationHeader)
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

func TestListRepoWebhooks(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		hooks := []giteaHook{
			{ID: 1, Config: giteaHookConfig{URL: "https://example.com/webhook"}},
		}
		_ = json.NewEncoder(w).Encode(hooks)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	hooks, err := newTestClient(srv.URL).ListRepoWebhooks(context.Background(), "org/repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 1 || hooks[0].URL != "https://example.com/webhook" {
		t.Fatalf("unexpected hooks: %+v", hooks)
	}
}

func TestUpdateRepoWebhook(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}

		var raw map[string]any
		_ = json.NewDecoder(r.Body).Decode(&raw)
		if _, hasType := raw["type"]; hasType {
			t.Error("expected no type field on update (immutable)")
		}

		events, _ := raw["events"].([]any)
		if len(events) != 2 || events[0] != "issues_only" || events[1] != "pull_request_only" {
			t.Errorf("expected the fixed subscription, got %v", events)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(giteaHook{ID: 42, Config: giteaHookConfig{URL: "https://example.com/webhook"}, Active: true})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	hook, err := newTestClient(srv.URL).UpdateRepoWebhook(context.Background(), "org/repo1", "42", gitProviderClients.CreateWebhookOptions{
		URL:    "https://example.com/webhook",
		Active: true,
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
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	if err := newTestClient(srv.URL).DeleteRepoWebhook(context.Background(), "org/repo1", "42"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
