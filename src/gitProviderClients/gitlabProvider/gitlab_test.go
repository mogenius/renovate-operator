package gitlabProvider

import (
	"context"
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
