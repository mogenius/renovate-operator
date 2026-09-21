package githubProvider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestListRepositoriesByProperty_InstallationToken(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			// a full page forces a second request
			repos := make([]string, 0, 100)
			for i := 0; i < 98; i++ {
				repos = append(repos, `{"full_name":"org/filler-`+strconv.Itoa(i)+`","custom_properties":{"owning-team":"other"}}`)
			}
			repos = append(repos,
				`{"full_name":"org/platform","custom_properties":{"owning-team":"software-platform"}}`,
				`{"full_name":"org/archived","archived":true,"custom_properties":{"owning-team":"software-platform"}}`)
			_, _ = w.Write([]byte(`{"total_count":101,"repositories":[` + strings.Join(repos, ",") + `]}`))
		default:
			_, _ = w.Write([]byte(`{"total_count":101,"repositories":[
				{"full_name":"org/multi","custom_properties":{"owning-team":["ground","software-platform"]}},
				{"full_name":"org/unset","custom_properties":{"owning-team":null}},
				{"full_name":"org/noprops"}
			]}`))
		}
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	repos, err := newTestClient(srv.URL).ListRepositoriesByProperty(context.Background(), "owning-team", []string{"software-platform"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"org/multi", "org/platform"}
	if len(repos) != len(want) {
		t.Fatalf("expected %v, got %v", want, repos)
	}
	for i := range want {
		if repos[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, repos)
		}
	}
}

func TestListRepositoriesByProperty_FallsBackToUserRepos(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
	})
	handler.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"full_name":"org/mine","custom_properties":{"owning-team":"gnc"}},{"full_name":"org/theirs","custom_properties":{"owning-team":"fpga"}}]`))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	repos, err := newTestClient(srv.URL).ListRepositoriesByProperty(context.Background(), "owning-team", []string{"gnc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 || repos[0] != "org/mine" {
		t.Fatalf("expected [org/mine], got %v", repos)
	}
}

func TestListRepositoriesByProperty_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).ListRepositoriesByProperty(context.Background(), "owning-team", []string{"gnc"}); err == nil {
		t.Fatal("expected an error on 500")
	}
}
