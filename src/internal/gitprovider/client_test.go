package gitprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	api "renovate-operator/api/v1alpha1"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeClient is a test double for GitProviderClient.
type fakeClient struct {
	isForkFn func(ctx context.Context, project string) (bool, error)
}

func (f *fakeClient) IsFork(ctx context.Context, project string) (bool, error) {
	return f.isForkFn(ctx, project)
}

func newTestJob(platform, endpoint, secretRef, namespace string, skipForks bool) *api.RenovateJob {
	return &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: namespace,
		},
		Spec: api.RenovateJobSpec{
			SkipForks: skipForks,
			SecretRef: secretRef,
			Provider: &api.RenovateProvider{
				Name:     platform,
				Endpoint: endpoint,
			},
		},
	}
}

func newTestSecret(name, namespace, tokenKey, tokenValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			tokenKey: []byte(tokenValue),
		},
	}
}

func TestFilterForks_WithFakeClient(t *testing.T) {
	fc := &fakeClient{
		isForkFn: func(ctx context.Context, project string) (bool, error) {
			return project == "org/repo2-fork", nil
		},
	}

	projects := []string{"org/repo1", "org/repo2-fork", "org/repo3"}
	result, err := FilterForks(context.Background(), fc, logr.Discard(), projects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(result), result)
	}
	if result[0] != "org/repo1" || result[1] != "org/repo3" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestFilterForks_EmptyProjects(t *testing.T) {
	fc := &fakeClient{
		isForkFn: func(ctx context.Context, project string) (bool, error) {
			t.Fatal("IsFork should not be called for empty projects")
			return false, nil
		},
	}

	result, err := FilterForks(context.Background(), fc, logr.Discard(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
}

func TestFilterGitHubForks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/org/repo1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fork": false})
	})
	mux.HandleFunc("/repos/org/repo2-fork", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fork": true})
	})
	mux.HandleFunc("/repos/org/repo3", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fork": false})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &GitHubClient{endpoint: server.URL, token: "test-token", httpClient: http.DefaultClient}
	projects := []string{"org/repo1", "org/repo2-fork", "org/repo3"}

	result, err := FilterForks(context.Background(), client, logr.Discard(), projects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(result), result)
	}
	if result[0] != "org/repo1" || result[1] != "org/repo3" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestFilterGitLabForks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/org%2Frepo1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	mux.HandleFunc("/api/v4/projects/org%2Frepo2-fork", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                  2,
			"forked_from_project": map[string]any{"id": 99},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &GitLabClient{endpoint: server.URL + "/api/v4", token: "test-token", httpClient: http.DefaultClient}
	projects := []string{"org/repo1", "org/repo2-fork"}

	result, err := FilterForks(context.Background(), client, logr.Discard(), projects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 project, got %d: %v", len(result), result)
	}
	if result[0] != "org/repo1" {
		t.Fatalf("unexpected result: %v", result)
	}
}

func TestFilterGiteaForks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/org/repo1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fork": false})
	})
	mux.HandleFunc("/api/v1/repos/org/repo2-fork", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fork": true})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &GiteaClient{endpoint: server.URL, token: "test-token", httpClient: http.DefaultClient}
	result, err := FilterForks(context.Background(), client, logr.Discard(), []string{"org/repo1", "org/repo2-fork"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "org/repo1" {
		t.Fatalf("expected [org/repo1], got %v", result)
	}
}

func TestFilterForgejoForks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/org/repo1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fork": true})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Forgejo uses the same GiteaClient
	client := &GiteaClient{endpoint: server.URL, token: "test-token", httpClient: http.DefaultClient}
	result, err := FilterForks(context.Background(), client, logr.Discard(), []string{"org/repo1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
}

func TestFilterBitbucketForks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/2.0/repositories/org/repo1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "org/repo1"})
	})
	mux.HandleFunc("/2.0/repositories/org/repo2-fork", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"full_name": "org/repo2-fork",
			"parent":    map[string]any{"full_name": "upstream/repo2"},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &BitbucketClient{endpoint: server.URL, token: "test-token", httpClient: http.DefaultClient}
	result, err := FilterForks(context.Background(), client, logr.Discard(), []string{"org/repo1", "org/repo2-fork"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "org/repo1" {
		t.Fatalf("expected [org/repo1], got %v", result)
	}
}

func TestFilterForks_APIErrorFailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &GitHubClient{endpoint: server.URL, token: "test-token", httpClient: http.DefaultClient}
	result, err := FilterForks(context.Background(), client, logr.Discard(), []string{"org/repo1", "org/repo2"})
	if err != nil {
		t.Fatalf("unexpected error (should fail-open): %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 projects (fail-open), got %d: %v", len(result), result)
	}
}

func TestNewClientFactory_MissingSecretRef(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	factory := NewClientFactory(c)
	job := newTestJob("github", "https://api.github.com", "", "default", true)

	_, err := factory(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for missing secretRef")
	}
}

func TestNewClientFactory_UnsupportedPlatform(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = api.AddToScheme(scheme)

	secret := newTestSecret("my-secret", "default", "RENOVATE_TOKEN", "test-token")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	factory := NewClientFactory(c)
	job := newTestJob("azure", "https://dev.azure.com", "my-secret", "default", true)

	_, err := factory(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
}

func TestNewClientFactory_NoProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	factory := NewClientFactory(c)
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: api.RenovateJobSpec{
			SkipForks: true,
			SecretRef: "my-secret",
		},
	}

	_, err := factory(context.Background(), job)
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
}

func TestNewClientFactory_ReturnsCorrectClients(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = api.AddToScheme(scheme)

	secret := newTestSecret("my-secret", "default", "RENOVATE_TOKEN", "test-token")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	factory := NewClientFactory(c)

	tests := []struct {
		platform     string
		expectedType string
	}{
		{"github", "*gitprovider.GitHubClient"},
		{"gitlab", "*gitprovider.GitLabClient"},
		{"gitea", "*gitprovider.GiteaClient"},
		{"forgejo", "*gitprovider.GiteaClient"},
		{"bitbucket", "*gitprovider.BitbucketClient"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			job := newTestJob(tt.platform, "https://example.com", "my-secret", "default", true)
			client, err := factory(context.Background(), job)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := typeName(client)
			if got != tt.expectedType {
				t.Fatalf("expected %s, got %s", tt.expectedType, got)
			}
		})
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *GitHubClient:
		return "*gitprovider.GitHubClient"
	case *GitLabClient:
		return "*gitprovider.GitLabClient"
	case *GiteaClient:
		return "*gitprovider.GiteaClient"
	case *BitbucketClient:
		return "*gitprovider.BitbucketClient"
	default:
		return "unknown"
	}
}
