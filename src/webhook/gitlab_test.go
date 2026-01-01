package webhook

import "testing"

func TestIsValidGitLabEvent(t *testing.T) {
	tests := []struct {
		name    string
		payload GitLabEvent
		valid   bool
		reason  string
	}{
		{
			name: "valid merge request edited with body change",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					Name:              "repo",
					PathWithNamespace: "example/repo",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Current: "Updated description with checkbox - [x] <!-- rebase-check -->If you want to rebase/retry this MR",
					},
				},
			},
			valid:  true,
			reason: "",
		},
		{
			name: "invalid action",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "open",
				},
				Project: Project{
					Name:              "repo",
					PathWithNamespace: "example/repo",
				},
			},
			valid:  false,
			reason: "event action is not update",
		},
		{
			name: "no merge request or issue",
			payload: GitLabEvent{
				ObjectKind: "note",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					Name:              "repo",
					PathWithNamespace: "example/repo",
				},
			},
			valid:  false,
			reason: "object kind is not merge_request or issue",
		},
		{
			name: "no description change",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					Name:              "repo",
					PathWithNamespace: "example/repo",
				},
			},
			valid:  false,
			reason: "no description change detected",
		},
		{
			name: "not a valid renovate checkbox change",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					Name:              "repo",
					PathWithNamespace: "example/repo",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Current: "not a valid checkbox",
					},
				},
			},
			valid:  false,
			reason: "not a valid renovate checkbox change",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, reason := isValidGitLabEvent(&tt.payload)
			if valid != tt.valid || reason != tt.reason {
				t.Errorf("expected valid=%v, reason=%q; got valid=%v, reason=%q", tt.valid, tt.reason, valid, reason)
			}
		})
	}
}
