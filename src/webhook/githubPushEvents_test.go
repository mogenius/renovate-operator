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

func pushPayload(ref string) GitHubPushEvent {
	return GitHubPushEvent{
		Ref:        ref,
		Repository: GitHubPushRepository{FullName: "example/repo", DefaultBranch: "main"},
	}
}

// pushTestServer serves one job owning example/repo and records scheduled projects.
func pushTestServer(job api.RenovateJob, scheduled *[]string) *Server {
	manager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{job}, nil
		},
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			if status.Status != api.JobStatusScheduled {
				return nil
			}
			*scheduled = append(*scheduled, jobId.Name+":"+project)
			return nil
		},
	}
	return &Server{manager: manager, discovery: &fakeDiscoveryAgent{}, logger: logr.Discard()}
}

func optedInJob() api.RenovateJob {
	job := makeTestRenovateJob("renovate", "job1", "example/repo")
	job.Spec.Webhook.TriggerOnPush = true
	job.Spec.Provider = &api.RenovateProvider{Name: "github"}
	return job
}

func gitlabOptedInJob() api.RenovateJob {
	job := optedInJob()
	job.Spec.Provider = &api.RenovateProvider{Name: "gitlab"}
	return job
}

func TestGitHubPushEvent(t *testing.T) {
	deleted := pushPayload("refs/heads/main")
	deleted.Deleted = true

	tests := []struct {
		name          string
		job           api.RenovateJob
		payload       GitHubPushEvent
		wantStatus    int
		wantScheduled bool
	}{
		{"default branch schedules a run", optedInJob(), pushPayload("refs/heads/main"), http.StatusAccepted, true},
		{"other branch is ignored", optedInJob(), pushPayload("refs/heads/feature"), http.StatusOK, false},
		{"renovate branch is ignored", optedInJob(), pushPayload("refs/heads/renovate/foo-1.x"), http.StatusOK, false},
		{"tag is ignored", optedInJob(), pushPayload("refs/tags/main"), http.StatusOK, false},
		{"branch deletion is ignored", optedInJob(), deleted, http.StatusOK, false},
		{"job without triggerOnPush is ignored", makeTestRenovateJob("renovate", "job1", "example/repo"), pushPayload("refs/heads/main"), http.StatusOK, false},
		{"non-GitHub job is ignored", gitlabOptedInJob(), pushPayload("refs/heads/main"), http.StatusOK, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var scheduled []string
			w := postGitHubEvent(pushTestServer(tt.job, &scheduled), "push", tt.payload)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
			want := []string{}
			if tt.wantScheduled {
				want = []string{"job1:example/repo"}
			}
			if len(scheduled) != len(want) || (len(want) == 1 && scheduled[0] != want[0]) {
				t.Errorf("expected scheduled=%v, got %v", tt.wantScheduled, scheduled)
			}
		})
	}
}

func TestGitHubPushEventRequiresAuthentication(t *testing.T) {
	job := optedInJob()
	job.Spec.Webhook.Authentication = &api.RenovateWebhookAuth{Enabled: true}
	var scheduled []string
	server := pushTestServer(job, &scheduled)

	w := postGitHubEvent(server, "push", pushPayload("refs/heads/main"))
	if w.Code != http.StatusUnauthorized || len(scheduled) != 0 {
		t.Errorf("expected 401 without scheduling, got %d and %v", w.Code, scheduled)
	}

	body, _ := json.Marshal(pushPayload("refs/heads/main"))
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("Authorization", "Bearer token")
	w = httptest.NewRecorder()
	server.githubWebhook(w, req)
	if w.Code != http.StatusAccepted || len(scheduled) != 1 {
		t.Errorf("expected 202 with one scheduled run, got %d and %v", w.Code, scheduled)
	}
}

func TestGitHubPushEventPrefersOptedInJob(t *testing.T) {
	plain := makeTestRenovateJob("renovate", "plain", "example/repo")
	optedIn := optedInJob()
	optedIn.Name = "opted-in"
	var scheduled []string
	server := pushTestServer(plain, &scheduled)
	server.manager.(*mockWebhookManager).listRenovateJobsFullFunc = func(ctx context.Context) ([]api.RenovateJob, error) {
		return []api.RenovateJob{plain, optedIn}, nil
	}

	w := postGitHubEvent(server, "push", pushPayload("refs/heads/main"))
	if w.Code != http.StatusAccepted || len(scheduled) != 1 || scheduled[0] != "opted-in:example/repo" {
		t.Errorf("expected only the opted-in job to be scheduled, got %d and %v", w.Code, scheduled)
	}
}

func TestGitHubPushEventMalformedPayload(t *testing.T) {
	server := &Server{manager: &mockWebhookManager{}, discovery: &fakeDiscoveryAgent{}, logger: logr.Discard()}
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewReader([]byte("not json")))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	server.githubWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
