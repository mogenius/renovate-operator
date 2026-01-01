package webhook

import "testing"

func TestGitHubEventValidation(t *testing.T) {

	tests := []struct {
		name    string
		payload GitHubEvent
		valid   bool
		reason  string
	}{
		{
			name: "valid pull request edited with body change",
			payload: GitHubEvent{
				Action: "edited",
				PullRequest: &GitHubPullRequest{
					Body: "Updated body with checkbox - [x] <!-- rebase-check -->If you want to rebase/retry this PR",
				},
				Repository: GitHubRepository{
					FullName: "example/repo",
				},
			},
			valid:  true,
			reason: "",
		},
		{
			name: "invalid action",
			payload: GitHubEvent{
				Action: "opened",
				PullRequest: &GitHubPullRequest{
					Body: "Some body",
				},
				Repository: GitHubRepository{
					FullName: "example/repo",
				},
			},
			valid:  false,
			reason: "event action is not edited",
		},
		{
			name: "no pull request or issue",
			payload: GitHubEvent{
				Action:     "edited",
				Repository: GitHubRepository{FullName: "example/repo"},
			},
			valid:  false,
			reason: "event is neither pull request nor issue",
		},
		{
			name: "no body change",
			payload: GitHubEvent{
				Action: "edited",
				PullRequest: &GitHubPullRequest{
					Body: "",
				},
				Repository: GitHubRepository{
					FullName: "example/repo",
				},
			},
			valid:  false,
			reason: "no body change detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, reason := isValidGitHubEvent(&tt.payload)
			if valid != tt.valid || reason != tt.reason {
				t.Errorf("expected valid=%v, reason=%q; got valid=%v, reason=%q", tt.valid, tt.reason, valid, reason)
			}
		})
	}
}
