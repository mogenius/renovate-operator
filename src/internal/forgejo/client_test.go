package forgejo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchReposByTopic(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			t.Errorf("expected auth header 'token test-token', got %q", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("topic") != "true" {
			t.Errorf("expected topic=true, got %q", r.URL.Query().Get("topic"))
		}
		if r.URL.Query().Get("q") != "renovate" {
			t.Errorf("expected q=renovate, got %q", r.URL.Query().Get("q"))
		}

		page := r.URL.Query().Get("page")
		if page == "2" {
			json.NewEncoder(w).Encode(map[string][]Repository{"data": {}})
			return
		}

		repos := []Repository{
			{ID: 1, FullName: "org/repo1", Name: "repo1", Permissions: &RepositoryPermissions{Admin: true}},
			{ID: 2, FullName: "org/repo2", Name: "repo2", Permissions: &RepositoryPermissions{Admin: false}},
		}
		json.NewEncoder(w).Encode(map[string][]Repository{"data": repos})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	repos, err := c.SearchReposByTopic(context.Background(), "renovate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].FullName != "org/repo1" {
		t.Errorf("expected org/repo1, got %s", repos[0].FullName)
	}
}

func TestSearchReposByTopic_Pagination(t *testing.T) {
	callCount := 0
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/search", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		page := r.URL.Query().Get("page")

		// Return 50 repos on page 1 to trigger pagination, empty on page 2
		if page == "1" {
			repos := make([]Repository, 50)
			for i := range repos {
				repos[i] = Repository{ID: int64(i), FullName: "org/repo" + r.URL.Query().Get("page") + "-" + string(rune('a'+i))}
			}
			json.NewEncoder(w).Encode(map[string][]Repository{"data": repos})
			return
		}

		repos := []Repository{{ID: 100, FullName: "org/last-repo"}}
		json.NewEncoder(w).Encode(map[string][]Repository{"data": repos})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	repos, err := c.SearchReposByTopic(context.Background(), "renovate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 51 {
		t.Fatalf("expected 51 repos, got %d", len(repos))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestListRepoWebhooks(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		hooks := []Webhook{
			{ID: 1, Config: WebhookConfig{URL: "https://example.com/webhook"}},
		}
		json.NewEncoder(w).Encode(hooks)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	hooks, err := c.ListRepoWebhooks(context.Background(), "org", "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(hooks))
	}
	if hooks[0].Config.URL != "https://example.com/webhook" {
		t.Errorf("expected webhook URL, got %s", hooks[0].Config.URL)
	}
}

func TestCreateRepoWebhook(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/org/repo1/hooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var opts CreateWebhookOptions
		json.NewDecoder(r.Body).Decode(&opts)
		if opts.Config.URL != "https://example.com/webhook" {
			t.Errorf("expected webhook URL, got %s", opts.Config.URL)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Webhook{ID: 42, Config: opts.Config})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	hook, err := c.CreateRepoWebhook(context.Background(), "org", "repo1", CreateWebhookOptions{
		Type:   "forgejo",
		Config: WebhookConfig{URL: "https://example.com/webhook", ContentType: "json"},
		Events: []string{"push"},
		Active: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.ID != 42 {
		t.Errorf("expected hook ID 42, got %d", hook.ID)
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
	err := c.DeleteRepoWebhook(context.Background(), "org", "repo1", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/api/v1/repos/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"unauthorized"}`))
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := NewClient(srv.URL, "bad-token")
	_, err := c.SearchReposByTopic(context.Background(), "renovate")
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}
