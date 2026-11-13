package gitProviderClients

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
)

func TestFilterProjects(t *testing.T) {
	tests := []struct {
		name     string
		projects []string
		infos    map[string]RepositoryInfo
		errors   map[string]error
		opts     FilterOptions
		// expected is the remaining project list.
		expected []string
		// stats is the expected per-criterion removal count.
		stats FilterStats
	}{
		{
			name:     "no projects",
			projects: []string{},
			opts:     FilterOptions{SkipForks: true, SkipPendingDeletion: true},
			expected: []string{},
		},
		{
			name:     "no criteria keeps everything",
			projects: []string{"repo1", "repo2"},
			infos:    map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {PendingDeletion: true}},
			expected: []string{"repo1", "repo2"},
		},
		{
			name:     "skip forks only",
			projects: []string{"repo1", "repo2", "repo3"},
			infos:    map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {}, "repo3": {PendingDeletion: true}},
			opts:     FilterOptions{SkipForks: true},
			expected: []string{"repo2", "repo3"},
			stats:    FilterStats{ForksRemoved: 1},
		},
		{
			name:     "skip pending deletion only",
			projects: []string{"repo1", "repo2", "repo3"},
			infos:    map[string]RepositoryInfo{"repo1": {Fork: true}, "repo2": {}, "repo3": {PendingDeletion: true}},
			opts:     FilterOptions{SkipPendingDeletion: true},
			expected: []string{"repo1", "repo2"},
			stats:    FilterStats{PendingRemoved: 1},
		},
		{
			name:     "skip both",
			projects: []string{"repo1", "repo2", "repo3", "repo4"},
			infos: map[string]RepositoryInfo{
				"repo1": {Fork: true},
				"repo2": {},
				"repo3": {PendingDeletion: true},
				"repo4": {Fork: true, PendingDeletion: true},
			},
			opts:     FilterOptions{SkipForks: true, SkipPendingDeletion: true},
			expected: []string{"repo2"},
			stats:    FilterStats{ForksRemoved: 2, PendingRemoved: 1},
		},
		{
			name:     "API error treated as kept (fail-open)",
			projects: []string{"repo1", "repo2"},
			infos:    map[string]RepositoryInfo{"repo1": {Fork: true}},
			errors:   map[string]error{"repo2": fmt.Errorf("API error")},
			opts:     FilterOptions{SkipForks: true, SkipPendingDeletion: true},
			expected: []string{"repo2"},
			stats:    FilterStats{ForksRemoved: 1},
		},
		{
			name:     "ignore single repository by exact name",
			projects: []string{"org/repo1", "org/repo2"},
			opts:     FilterOptions{IgnoreRepositories: []string{"org/repo1"}},
			expected: []string{"org/repo2"},
			stats:    FilterStats{IgnoredRepositoriesRemoved: 1},
		},
		{
			name:     "ignore a list of repositories with globs",
			projects: []string{"org/api", "org/legacy-api", "org/web", "other/legacy-web"},
			opts:     FilterOptions{IgnoreRepositories: []string{"org/legacy-*", "other/*"}},
			expected: []string{"org/api", "org/web"},
			stats:    FilterStats{IgnoredRepositoriesRemoved: 2},
		},
		{
			name:     "bare pattern matches repository name",
			projects: []string{"org/legacy-api", "org/other"},
			opts:     FilterOptions{IgnoreRepositories: []string{"legacy-*"}},
			expected: []string{"org/other"},
			stats:    FilterStats{IgnoredRepositoriesRemoved: 1},
		},
		{
			name:     "double star matches nested groups",
			projects: []string{"org/team/legacy", "org/team/api", "org/api"},
			opts:     FilterOptions{IgnoreRepositories: []string{"org/**/legacy"}},
			expected: []string{"org/team/api", "org/api"},
			stats:    FilterStats{IgnoredRepositoriesRemoved: 1},
		},
		{
			name:     "repository glob matching is case-insensitive",
			projects: []string{"Org/Legacy-API", "Org/API"},
			opts:     FilterOptions{IgnoreRepositories: []string{"org/legacy-*"}},
			expected: []string{"Org/API"},
			stats:    FilterStats{IgnoredRepositoriesRemoved: 1},
		},
		{
			name:     "ignore topics case-insensitively",
			projects: []string{"org/repo1", "org/repo2", "org/repo3"},
			infos: map[string]RepositoryInfo{
				"org/repo1": {Topics: []string{"Archived"}},
				"org/repo2": {Topics: []string{"renovate", "archived"}},
				"org/repo3": {Topics: []string{"renovate"}},
			},
			opts:     FilterOptions{IgnoreTopics: []string{"archived"}},
			expected: []string{"org/repo3"},
			stats:    FilterStats{IgnoredTopicsRemoved: 2},
		},
		{
			name:     "platform without topics keeps repositories",
			projects: []string{"org/repo1"},
			infos:    map[string]RepositoryInfo{"org/repo1": {}},
			opts:     FilterOptions{IgnoreTopics: []string{"archived"}},
			expected: []string{"org/repo1"},
		},
		{
			name:     "ignored topic wins over fork",
			projects: []string{"org/repo1"},
			infos:    map[string]RepositoryInfo{"org/repo1": {Fork: true, Topics: []string{"archived"}}},
			opts:     FilterOptions{SkipForks: true, IgnoreTopics: []string{"archived"}},
			expected: []string{},
			stats:    FilterStats{IgnoredTopicsRemoved: 1},
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
			result, stats, err := FilterProjects(context.Background(), client, logger, tt.projects, tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalSlices(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
			if stats != tt.stats {
				t.Errorf("expected stats %+v, got %+v", tt.stats, stats)
			}
		})
	}
}

// Ignored repositories are dropped before any metadata request, so they never
// cost a platform API call.
func TestFilterProjectsIgnoreRepositoriesSkipsAPICall(t *testing.T) {
	calls := 0
	client := &mockGitProviderClient{
		getRepositoryInfoFunc: func(ctx context.Context, project string) (RepositoryInfo, error) {
			calls++
			return RepositoryInfo{}, nil
		},
	}

	result, stats, err := FilterProjects(context.Background(), client, logr.Logger{}, []string{"org/ignored", "org/kept"},
		FilterOptions{IgnoreRepositories: []string{"org/ignored"}, SkipForks: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalSlices(result, []string{"org/kept"}) {
		t.Errorf("expected [org/kept], got %v", result)
	}
	if calls != 1 {
		t.Errorf("expected 1 metadata call, got %d", calls)
	}
	if stats.IgnoredRepositoriesRemoved != 1 {
		t.Errorf("expected 1 ignored repository, got %d", stats.IgnoredRepositoriesRemoved)
	}
}

// Metadata criteria fail open when no provider client is available.
func TestFilterProjectsNilProviderFailsOpen(t *testing.T) {
	result, stats, err := FilterProjects(context.Background(), nil, logr.Logger{}, []string{"org/repo"},
		FilterOptions{SkipForks: true, IgnoreTopics: []string{"archived"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equalSlices(result, []string{"org/repo"}) {
		t.Errorf("expected [org/repo], got %v", result)
	}
	if stats != (FilterStats{}) {
		t.Errorf("expected no removals, got %+v", stats)
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

func (c *mockGitProviderClient) ListRepoWebhooks(ctx context.Context, project string) ([]Webhook, error) {
	return nil, fmt.Errorf("listing webhooks is not supported")
}

func (c *mockGitProviderClient) CreateRepoWebhook(ctx context.Context, project string, opts CreateWebhookOptions) (*Webhook, error) {
	return nil, fmt.Errorf("creating webhooks is not supported")
}

func (c *mockGitProviderClient) UpdateRepoWebhook(ctx context.Context, project string, hookID string, opts CreateWebhookOptions) (*Webhook, error) {
	return nil, nil
}

func (c *mockGitProviderClient) DeleteRepoWebhook(ctx context.Context, project string, hookID string) error {
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
