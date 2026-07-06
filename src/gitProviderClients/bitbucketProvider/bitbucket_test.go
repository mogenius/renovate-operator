package bitbucketProvider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"renovate-operator/gitProviderClients"
)

const hookUUID = "{5f38a608-0000-4000-8000-000000000000}"

func newTestClient(url string) *BitbucketClient {
	return &BitbucketClient{Endpoint: url, Token: "test-token", HTTPClient: http.DefaultClient}
}

func TestListRepoWebhooks(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/2.0/repositories/ws/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected bearer auth, got %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []bitbucketHook{
				{UUID: hookUUID, URL: "https://example.com/webhook", Active: true, Events: []string{"pullrequest:updated", "pullrequest:fulfilled", "pullrequest:rejected"}},
			},
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	hooks, err := newTestClient(srv.URL).ListRepoWebhooks(context.Background(), "ws/repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	if hooks[0].ID != hookUUID {
		t.Errorf("expected UUID hook ID, got %s", hooks[0].ID)
	}
	// exactly the fixed PR events means the subscription is up to date
	if !hooks[0].EventsUpToDate {
		t.Error("expected hook with the fixed PR events to report EventsUpToDate")
	}
}

func TestCreateRepoWebhook(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/2.0/repositories/ws/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var payload bitbucketHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.URL != "https://example.com/webhook" {
			t.Errorf("expected webhook URL, got %s", payload.URL)
		}
		if payload.Secret != "secret" {
			t.Errorf("expected hook secret, got %s", payload.Secret)
		}
		// the provider applies its fixed subscription
		if len(payload.Events) != 3 || payload.Events[0] != "pullrequest:updated" {
			t.Errorf("expected the fixed PR subscription, got %v", payload.Events)
		}

		w.WriteHeader(http.StatusCreated)
		payload.UUID = hookUUID
		payload.Secret = "" // Bitbucket never returns the secret
		_ = json.NewEncoder(w).Encode(payload)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	hook, err := newTestClient(srv.URL).CreateRepoWebhook(context.Background(), "ws/repo1", gitProviderClients.CreateWebhookOptions{
		URL:       "https://example.com/webhook",
		AuthToken: "secret",
		Active:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.ID != hookUUID {
		t.Errorf("expected UUID hook ID, got %s", hook.ID)
	}
}

func TestUpdateRepoWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		// the UUID (including braces) must arrive path-escaped
		if r.URL.EscapedPath() != "/2.0/repositories/ws/repo1/hooks/%7B5f38a608-0000-4000-8000-000000000000%7D" {
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
		}

		var payload bitbucketHook
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.Events) != 3 {
			t.Errorf("expected the fixed PR subscription, got %v", payload.Events)
		}

		payload.UUID = hookUUID
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	hook, err := newTestClient(srv.URL).UpdateRepoWebhook(context.Background(), "ws/repo1", hookUUID, gitProviderClients.CreateWebhookOptions{
		URL:       "https://example.com/webhook",
		AuthToken: "secret",
		Active:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.ID != hookUUID {
		t.Errorf("expected UUID hook ID, got %s", hook.ID)
	}
}

func TestDeleteRepoWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(srv.URL).DeleteRepoWebhook(context.Background(), "ws/repo1", hookUUID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteRepoWebhook_NotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := newTestClient(srv.URL).DeleteRepoWebhook(context.Background(), "ws/repo1", hookUUID); err != nil {
		t.Fatalf("expected nil error for already-deleted webhook, got: %v", err)
	}
}
