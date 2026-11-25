package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"

	"github.com/go-logr/logr"
)

func TestTriggerEventSelection(t *testing.T) {
	editedOnly := &api.WebhookTriggerEvents{PullRequest: []api.PullRequestTriggerEvent{api.PullRequestEdited}}
	withMerged := &api.WebhookTriggerEvents{PullRequest: []api.PullRequestTriggerEvent{api.PullRequestMerged}}
	tests := []struct {
		name          string
		global        *api.WebhookTriggerEvents
		job           *api.WebhookTriggerEvents
		wantTriggered bool
	}{
		{"default: everything triggers", nil, nil, true},
		{"global drops merged", editedOnly, nil, false},
		{"job drops merged", nil, editedOnly, false},
		{"job selects nothing", nil, &api.WebhookTriggerEvents{}, false},
		{"job re-adds merged over global", editedOnly, withMerged, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := makeTestRenovateJob("renovate", "job1", "example/repo")
			job.Spec.Webhook.TriggerOnEvent = tt.job

			triggered := false
			server := &Server{
				manager: &mockWebhookManager{
					listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
						return []api.RenovateJob{job}, nil
					},
					updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
						triggered = true
						return nil
					},
				},
				logger:        logr.Discard(),
				triggerEvents: tt.global,
			}

			body, _ := json.Marshal(GitHubEvent{
				Action:      "closed",
				PullRequest: &GitHubPullRequest{Merged: true},
				Repository:  GitHubRepository{FullName: "example/repo"},
			})
			req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewReader(body))
			req.Header.Set("X-GitHub-Event", "pull_request")
			w := httptest.NewRecorder()
			server.githubWebhook(w, req)

			if triggered != tt.wantTriggered {
				t.Errorf("triggered = %v, want %v (status %d: %s)", triggered, tt.wantTriggered, w.Code, w.Body.String())
			}
		})
	}
}

// Each provider's accepted events map onto the normalized names users configure.
func TestTriggerEventMapping(t *testing.T) {
	pr := &GitHubPullRequest{}
	merged := &GitHubPullRequest{Merged: true}
	issue := &GitHubIssue{}
	giteaPR := &GiteaPullRequest{}
	forgejoPR := &ForgejoPullRequest{}
	tests := []struct {
		got  triggerEvent
		want string
	}{
		{gitHubTriggerEvent(&GitHubEvent{Action: "closed", PullRequest: merged}), "pullRequest.merged"},
		{gitHubTriggerEvent(&GitHubEvent{Action: "closed", PullRequest: pr}), "pullRequest.closed"},
		{gitHubTriggerEvent(&GitHubEvent{Action: "edited", PullRequest: pr}), "pullRequest.edited"},
		{gitHubTriggerEvent(&GitHubEvent{Action: "edited", Issue: issue}), "issues.edited"},
		{gitHubTriggerEvent(&GitHubEvent{Action: "closed", Issue: issue}), "issues.closed"},

		{gitLabTriggerEvent(&GitLabEvent{ObjectKind: "merge_request", ObjectAttributes: ObjectAttributes{Action: "merge"}}), "pullRequest.merged"},
		{gitLabTriggerEvent(&GitLabEvent{ObjectKind: "merge_request", ObjectAttributes: ObjectAttributes{Action: "update"}}), "pullRequest.edited"},
		{gitLabTriggerEvent(&GitLabEvent{ObjectKind: "issue", ObjectAttributes: ObjectAttributes{Action: "update"}}), "issues.edited"},

		{giteaTriggerEvent("issues", &GiteaEvent{Action: "edited"}), "issues.edited"},
		{giteaTriggerEvent("pull_request", &GiteaEvent{Action: "closed", PullRequest: &GiteaPullRequest{Merged: true}}), "pullRequest.merged"},
		{giteaTriggerEvent("pull_request", &GiteaEvent{Action: "closed", PullRequest: giteaPR}), "pullRequest.closed"},
		{giteaTriggerEvent("pull_request", &GiteaEvent{Action: "reopened", PullRequest: giteaPR}), "pullRequest.reopened"},
		{giteaTriggerEvent("pull_request", &GiteaEvent{Action: "edited", PullRequest: giteaPR}), "pullRequest.edited"},

		{forgejoTriggerEvent("issues", &ForgejoEvent{Action: "edited"}), "issues.edited"},
		{forgejoTriggerEvent("pull_request", &ForgejoEvent{Action: "closed", PullRequest: &ForgejoPullRequest{Merged: true}}), "pullRequest.merged"},
		{forgejoTriggerEvent("pull_request", &ForgejoEvent{Action: "reopened", PullRequest: forgejoPR}), "pullRequest.reopened"},

		{bitbucketTriggerEvent("pullrequest:fulfilled"), "pullRequest.merged"},
		{bitbucketTriggerEvent("pullrequest:rejected"), "pullRequest.closed"},
		{bitbucketTriggerEvent("pullrequest:updated"), "pullRequest.edited"},
	}
	for _, tt := range tests {
		if tt.got.String() != tt.want {
			t.Errorf("got %s, want %s", tt.got, tt.want)
		}
	}
}
