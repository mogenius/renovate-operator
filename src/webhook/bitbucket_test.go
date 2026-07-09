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

const renovatePRBody = "This PR contains the following updates.\n\n---\n - [x] <!-- rebase-check -->If you want to rebase/retry this PR, check this box"

func TestBitbucketWebhook(t *testing.T) {
	payload := BitbucketEvent{
		PullRequest: &BitbucketPullRequest{ID: 1, Title: "Update dependency", Description: renovatePRBody},
		Repository:  BitbucketRepository{FullName: "ws/repo"},
	}

	updateCalled := false
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeTestRenovateJob("renovate", "job1", "ws/repo")}, nil
		},
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			updateCalled = true
			return nil
		},
	}
	server := &Server{manager: mockManager, logger: logr.Discard()}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/bitbucket", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Key", "pullrequest:updated")

	w := httptest.NewRecorder()
	server.bitbucketWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !updateCalled {
		t.Error("expected project status update to be called")
	}
}

func TestBitbucketWebhookRequiresEventHeader(t *testing.T) {
	server := &Server{manager: &mockWebhookManager{}, logger: logr.Discard()}

	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/bitbucket", bytes.NewReader([]byte("{}")))

	w := httptest.NewRecorder()
	server.bitbucketWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for missing X-Event-Key header, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestBitbucketEventValidation(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		payload BitbucketEvent
		valid   bool
	}{
		{
			name:  "renovate PR description edited with checked checkbox",
			event: "pullrequest:updated",
			payload: BitbucketEvent{
				PullRequest: &BitbucketPullRequest{Description: renovatePRBody},
				Repository:  BitbucketRepository{FullName: "ws/repo"},
			},
			valid: true,
		},
		{
			name:  "renovate PR edited without checked checkbox",
			event: "pullrequest:updated",
			payload: BitbucketEvent{
				PullRequest: &BitbucketPullRequest{Description: "This PR contains the following updates.\n - [ ] <!-- rebase-check -->If you want to rebase/retry this PR, check this box"},
				Repository:  BitbucketRepository{FullName: "ws/repo"},
			},
			valid: false,
		},
		{
			name:  "non-renovate PR is ignored",
			event: "pullrequest:updated",
			payload: BitbucketEvent{
				PullRequest: &BitbucketPullRequest{Description: "some human PR with - [x] a checkbox"},
				Repository:  BitbucketRepository{FullName: "ws/repo"},
			},
			valid: false,
		},
		{
			name:  "renovate PR merged",
			event: "pullrequest:fulfilled",
			payload: BitbucketEvent{
				PullRequest: &BitbucketPullRequest{Description: renovatePRBody},
				Repository:  BitbucketRepository{FullName: "ws/repo"},
			},
			valid: true,
		},
		{
			name:  "renovate PR declined",
			event: "pullrequest:rejected",
			payload: BitbucketEvent{
				PullRequest: &BitbucketPullRequest{Description: renovatePRBody},
				Repository:  BitbucketRepository{FullName: "ws/repo"},
			},
			valid: true,
		},
		{
			name:  "missing pull request payload",
			event: "pullrequest:updated",
			payload: BitbucketEvent{
				Repository: BitbucketRepository{FullName: "ws/repo"},
			},
			valid: false,
		},
		{
			name:  "unsupported event type",
			event: "repo:push",
			payload: BitbucketEvent{
				Repository: BitbucketRepository{FullName: "ws/repo"},
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, valid := parseBitbucketPayload(tt.event, &tt.payload)
			reason := "failed to parse payload"
			if valid {
				valid, reason = isValidWebhookPayload(p)
			}
			if valid != tt.valid {
				t.Errorf("expected valid=%v, got %v (reason: %s)", tt.valid, valid, reason)
			}
		})
	}
}
