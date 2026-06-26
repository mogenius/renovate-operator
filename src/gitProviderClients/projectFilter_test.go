package gitProviderClients

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
)

func TestFilterProjects(t *testing.T) {
	tests := []struct {
		name                string
		projects            []string
		infos               map[string]RepositoryInfo
		errors              map[string]error
		skipForks           bool
		skipPendingDeletion bool
		expected            []string
	}{
		{
			name:                "no projects",
			projects:            []string{},
			skipForks:           true,
			skipPendingDeletion: true,
			expected:            []string{},
		},
		{
			name:                "both criteria off keeps everything",
			projects:            []string{"repo1", "repo2"},
			infos:               map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {PendingDeletion: true}},
			skipForks:           false,
			skipPendingDeletion: false,
			expected:            []string{"repo1", "repo2"},
		},
		{
			name:                "skip forks only",
			projects:            []string{"repo1", "repo2", "repo3"},
			infos:               map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {}, "repo3": {PendingDeletion: true}},
			skipForks:           true,
			skipPendingDeletion: false,
			expected:            []string{"repo2", "repo3"},
		},
		{
			name:                "skip pending deletion only",
			projects:            []string{"repo1", "repo2", "repo3"},
			infos:               map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {}, "repo3": {PendingDeletion: true}},
			skipForks:           false,
			skipPendingDeletion: true,
			expected:            []string{"repo1", "repo2"},
		},
		{
			name:                "skip both",
			projects:            []string{"repo1", "repo2", "repo3", "repo4"},
			infos:               map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {}, "repo3": {PendingDeletion: true}, "repo4": {Fork: true, PendingDeletion: true}},
			skipForks:           true,
			skipPendingDeletion: true,
			expected:            []string{"repo2"},
		},
		{
			name:                "API error treated as kept (fail-open)",
			projects:            []string{"repo1", "repo2"},
			infos:               map[string]RepositoryInfo{"repo1": {Fork: true}},
			errors:              map[string]error{"repo2": fmt.Errorf("API error")},
			skipForks:           true,
			skipPendingDeletion: true,
			expected:            []string{"repo2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGitProviderClient{
				getRepositoryInfoFunc: func(ctx context.Context, project string) (RepositoryInfo, error) {
					if err, ok := tt.errors[project]; ok {
						return RepositoryInfo{}, err
					}
					return tt.infos[project], nil
				},
			}

			logger := logr.Logger{}
			result, _, err := FilterProjects(context.Background(), client, logger, tt.projects, tt.skipForks, tt.skipPendingDeletion)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalSlices(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

type mockGitProviderClient struct {
	getRepositoryInfoFunc func(ctx context.Context, project string) (RepositoryInfo, error)
}

func (m *mockGitProviderClient) GetRepositoryInfo(ctx context.Context, project string) (RepositoryInfo, error) {
	if m.getRepositoryInfoFunc != nil {
		return m.getRepositoryInfoFunc(ctx, project)
	}
	return RepositoryInfo{}, nil
}

func (c *mockGitProviderClient) SearchReposByTopic(ctx context.Context, topic string) ([]Repository, error) {
	return nil, fmt.Errorf("searching repositories by topic is not supported")
}

func (c *mockGitProviderClient) ListRepoWebhooks(ctx context.Context, owner, repo string) ([]Webhook, error) {
	return nil, fmt.Errorf("listing webhooks is not supported")
}

func (c *mockGitProviderClient) CreateRepoWebhook(ctx context.Context, owner, repo string, opts CreateWebhookOptions) (*Webhook, error) {
	return nil, fmt.Errorf("creating webhooks is not supported")
}

func (c *mockGitProviderClient) DeleteRepoWebhook(ctx context.Context, owner, repo string, hookID int64) error {
	return fmt.Errorf("deleting webhooks is not supported")
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
