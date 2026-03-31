package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crdmanager "renovate-operator/internal/crdManager"

	"github.com/go-logr/logr"
)

func TestMemoryRepoCache(t *testing.T) {
	cache := NewMemoryRepoCache()
	ctx := context.Background()
	key := "test-key"
	repos := map[string]RepoPermission{
		"owner/repo1": {CanWrite: true},
		"owner/repo2": {CanWrite: false},
	}

	// Miss on empty cache
	if _, ok := cache.Get(ctx, key); ok {
		t.Fatal("expected cache miss")
	}

	// Hit after set
	cache.Set(ctx, key, repos, 5*time.Minute)
	got, ok := cache.Get(ctx, key)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(got))
	}
	if !got["owner/repo1"].CanWrite {
		t.Error("expected owner/repo1 to have write access")
	}
	if got["owner/repo2"].CanWrite {
		t.Error("expected owner/repo2 to be read-only")
	}

	// Expired entry returns miss
	cache.Set(ctx, key, repos, -1*time.Second)
	if _, ok := cache.Get(ctx, key); ok {
		t.Fatal("expected cache miss for expired entry")
	}
}

func TestRepoCacheKey(t *testing.T) {
	k1 := repoCacheKey("token-a", "https://example.com")
	k2 := repoCacheKey("token-b", "https://example.com")
	k3 := repoCacheKey("token-a", "https://other.com")

	if k1 == k2 {
		t.Fatal("different tokens should produce different keys")
	}
	if k1 == k3 {
		t.Fatal("different endpoints should produce different keys")
	}
	if k1 != repoCacheKey("token-a", "https://example.com") {
		t.Fatal("same inputs should produce same key")
	}
}

func TestFetchForgejoUserReposWithPermissions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"full_name": "alice/owned", "permissions": map[string]bool{"admin": true, "push": true, "pull": true}},
				{"full_name": "alice/collab", "permissions": map[string]bool{"admin": false, "push": true, "pull": true}},
				{"full_name": "bob/public", "permissions": map[string]bool{"admin": false, "push": false, "pull": true}},
			},
		})
	}))
	defer srv.Close()

	repos, err := fetchForgejoUserRepos(context.Background(), srv.URL, "test-token", logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}
	if !repos["alice/owned"].CanWrite {
		t.Error("expected alice/owned to have write access (admin)")
	}
	if !repos["alice/collab"].CanWrite {
		t.Error("expected alice/collab to have write access (push)")
	}
	if repos["bob/public"].CanWrite {
		t.Error("expected bob/public to be read-only")
	}
}

func TestFetchForgejoUserReposUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
	}))
	defer srv.Close()

	_, err := fetchForgejoUserRepos(context.Background(), srv.URL, "bad-token", logr.Discard())
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestFetchGitHubUserReposWithPermissions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"full_name": "alice/repo1", "permissions": map[string]bool{"admin": false, "push": true, "pull": true}},
			{"full_name": "bob/repo2", "permissions": map[string]bool{"admin": false, "push": false, "pull": true}},
		})
	}))
	defer srv.Close()

	got, err := fetchGitHubUserRepos(context.Background(), srv.URL, "test-token", logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(got))
	}
	if !got["alice/repo1"].CanWrite {
		t.Error("expected alice/repo1 to have write access")
	}
	if got["bob/repo2"].CanWrite {
		t.Error("expected bob/repo2 to be read-only")
	}
}

func TestFilterProjectsByAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"full_name": "alice/repo1", "permissions": map[string]bool{"push": true, "pull": true}},
				{"full_name": "alice/repo2", "permissions": map[string]bool{"push": false, "pull": true}},
			},
		})
	}))
	defer srv.Close()

	cache := NewMemoryRepoCache()
	session := &sessionData{
		Email:       "alice@example.com",
		AccessToken: "test-token",
	}

	projects := []crdmanager.RenovateProjectStatus{
		{Name: "alice/repo1"},
		{Name: "alice/repo2"},
		{Name: "alice/repo3"}, // user doesn't have access
		{Name: "bob/secret"},  // user doesn't have access
	}

	filtered := filterProjectsByAccess(
		context.Background(), session, "forgejo", srv.URL, projects, cache, logr.Discard(),
	)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered projects, got %d", len(filtered))
	}
	if filtered[0].Name != "alice/repo1" || filtered[1].Name != "alice/repo2" {
		t.Errorf("unexpected filtered projects: %v", filtered)
	}
}

func TestFilterProjectsByAccessNoToken(t *testing.T) {
	projects := []crdmanager.RenovateProjectStatus{
		{Name: "alice/repo1"},
		{Name: "alice/repo2"},
	}

	// No access token — should return all projects unchanged
	session := &sessionData{Email: "alice@example.com"}
	result := filterProjectsByAccess(
		context.Background(), session, "forgejo", "https://example.com", projects, NewMemoryRepoCache(), logr.Discard(),
	)
	if len(result) != 2 {
		t.Fatalf("expected all projects returned when no token, got %d", len(result))
	}

	// Nil session — should return all projects unchanged
	result = filterProjectsByAccess(
		context.Background(), nil, "forgejo", "https://example.com", projects, NewMemoryRepoCache(), logr.Discard(),
	)
	if len(result) != 2 {
		t.Fatalf("expected all projects returned with nil session, got %d", len(result))
	}
}

func TestFilterProjectsByAccessUnsupportedPlatform(t *testing.T) {
	projects := []crdmanager.RenovateProjectStatus{
		{Name: "alice/repo1"},
	}
	session := &sessionData{Email: "alice@example.com", AccessToken: "token"}

	result := filterProjectsByAccess(
		context.Background(), session, "bitbucket", "https://example.com", projects, NewMemoryRepoCache(), logr.Discard(),
	)
	if len(result) != 1 {
		t.Fatalf("expected all projects returned for unsupported platform, got %d", len(result))
	}
}

func TestFilterProjectsByAccessCachesResult(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"full_name": "alice/repo1", "permissions": map[string]bool{"push": true, "pull": true}},
			},
		})
	}))
	defer srv.Close()

	cache := NewMemoryRepoCache()
	session := &sessionData{Email: "alice@example.com", AccessToken: "test-token"}
	projects := []crdmanager.RenovateProjectStatus{{Name: "alice/repo1"}}

	// First call — cache miss, hits API
	filterProjectsByAccess(context.Background(), session, "forgejo", srv.URL, projects, cache, logr.Discard())
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	// Second call — cache hit, no additional API call
	filterProjectsByAccess(context.Background(), session, "forgejo", srv.URL, projects, cache, logr.Discard())
	if callCount != 1 {
		t.Fatalf("expected still 1 API call after cache hit, got %d", callCount)
	}
}

func TestCanUserWriteRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"full_name": "alice/writable", "permissions": map[string]bool{"push": true, "pull": true}},
				{"full_name": "alice/readonly", "permissions": map[string]bool{"push": false, "pull": true}},
			},
		})
	}))
	defer srv.Close()

	cache := NewMemoryRepoCache()
	session := &sessionData{Email: "alice@example.com", AccessToken: "test-token"}

	// Writable repo
	if !canUserWriteRepo(context.Background(), session, "forgejo", srv.URL, "alice/writable", cache, logr.Discard()) {
		t.Error("expected write access for alice/writable")
	}

	// Read-only repo
	if canUserWriteRepo(context.Background(), session, "forgejo", srv.URL, "alice/readonly", cache, logr.Discard()) {
		t.Error("expected no write access for alice/readonly")
	}

	// Repo not in user's list
	if canUserWriteRepo(context.Background(), session, "forgejo", srv.URL, "bob/secret", cache, logr.Discard()) {
		t.Error("expected no write access for repo not in user's list")
	}

	// No token — should fail open (return true)
	noTokenSession := &sessionData{Email: "alice@example.com"}
	if !canUserWriteRepo(context.Background(), noTokenSession, "forgejo", srv.URL, "anything", cache, logr.Discard()) {
		t.Error("expected fail-open when no token")
	}

	// Nil session — should fail open
	if !canUserWriteRepo(context.Background(), nil, "forgejo", srv.URL, "anything", cache, logr.Discard()) {
		t.Error("expected fail-open with nil session")
	}
}
