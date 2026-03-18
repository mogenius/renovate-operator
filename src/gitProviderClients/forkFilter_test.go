package gitProviderClients

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
)

func TestFilterForks(t *testing.T) {
	tests := []struct {
		name     string
		projects []string
		forked   map[string]bool
		errors   map[string]error
		expected []string
	}{
		{
			name:     "no projects",
			projects: []string{},
			forked:   map[string]bool{},
			errors:   map[string]error{},
			expected: []string{},
		},
		{
			name:     "all non-forks",
			projects: []string{"repo1", "repo2"},
			forked:   map[string]bool{"repo1": false, "repo2": false},
			errors:   map[string]error{},
			expected: []string{"repo1", "repo2"},
		},
		{
			name:     "all forks",
			projects: []string{"repo1", "repo2"},
			forked:   map[string]bool{"repo1": true, "repo2": true},
			errors:   map[string]error{},
			expected: []string{},
		},
		{
			name:     "mixed forks and non-forks",
			projects: []string{"repo1", "repo2", "repo3"},
			forked:   map[string]bool{"repo1": false, "repo2": true, "repo3": false},
			errors:   map[string]error{},
			expected: []string{"repo1", "repo3"},
		},
		{
			name:     "API error treated as non-fork",
			projects: []string{"repo1", "repo2"},
			forked:   map[string]bool{"repo1": false},
			errors:   map[string]error{"repo2": fmt.Errorf("API error")},
			expected: []string{"repo1", "repo2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGitProviderClient{
				isForkFunc: func(ctx context.Context, project string) (bool, error) {
					if err, ok := tt.errors[project]; ok {
						return false, err
					}
					return tt.forked[project], nil
				},
			}

			logger := logr.Logger{}
			result, err := FilterForks(context.Background(), client, logger, tt.projects)
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
	isForkFunc func(ctx context.Context, project string) (bool, error)
}

func (m *mockGitProviderClient) IsFork(ctx context.Context, project string) (bool, error) {
	return m.isForkFunc(ctx, project)
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
