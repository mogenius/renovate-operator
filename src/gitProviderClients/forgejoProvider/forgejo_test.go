package forgejoProvider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"renovate-operator/gitProviderClients"
	"testing"
)

func TestListRepoWebhooks(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		hooks := []forgejoHook{
			{ID: 1, Config: forgejoHookConfig{URL: "https://example.com/webhook"}, Events: []string{"issues", "pull_request"}},
		}
		_ = json.NewEncoder(w).Encode(hooks)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	hooks, err := c.ListRepoWebhooks(context.Background(), "org/repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	if hooks[0].URL != "https://example.com/webhook" {
		t.Errorf("expected webhook URL, got %s", hooks[0].URL)
	}
	// a hook reporting exactly the base issue/PR flags is up to date
	if !hooks[0].EventsUpToDate {
		t.Error("expected hook with base issue/PR flags to report EventsUpToDate")
	}
}

func TestCreateRepoWebhook(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var payload forgejoHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Type != "forgejo" {
			t.Errorf("expected hook type forgejo, got %s", payload.Type)
		}
		// the provider applies its fixed subscription
		if len(payload.Events) != 2 || payload.Events[0] != "issues_only" || payload.Events[1] != "pull_request_only" {
			t.Errorf("expected events [issues_only pull_request_only], got %v", payload.Events)
		}
		if payload.Config.URL != "https://example.com/webhook" {
			t.Errorf("expected webhook URL, got %s", payload.Config.URL)
		}
		if payload.Config.ContentType != "json" {
			t.Errorf("expected content type json, got %s", payload.Config.ContentType)
		}
		if payload.AuthorizationHeader != "Bearer secret" {
			t.Errorf("expected authorization header, got %s", payload.AuthorizationHeader)
		}

		w.WriteHeader(http.StatusCreated)
		payload.ID = 42
		_ = json.NewEncoder(w).Encode(payload)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	hook, err := c.CreateRepoWebhook(context.Background(), "org/repo1", gitProviderClients.CreateWebhookOptions{
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
		_ = json.NewEncoder(w).Encode(forgejoHook{ID: 42, Config: forgejoHookConfig{URL: "https://example.com/webhook"}, Active: true})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	hook, err := c.UpdateRepoWebhook(context.Background(), "org/repo1", "42", gitProviderClients.CreateWebhookOptions{
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

	c := NewClient(srv.URL, "test-token")
	err := c.DeleteRepoWebhook(context.Background(), "org/repo1", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteRepoWebhook_NotFoundIsSuccess(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"The target couldn't be found."}`))
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	if err := c.DeleteRepoWebhook(context.Background(), "org/repo1", "42"); err != nil {
		t.Fatalf("expected nil error for already-deleted webhook, got: %v", err)
	}
}
