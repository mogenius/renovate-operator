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

// Handler-level tests for the Gitea endpoint.
func TestGiteaWebhook(t *testing.T) {
	payload := GiteaEvent{
		Action: "edited",
		Issue: &GiteaIssue{
			Title: "Dependency Dashboard",
			Body:  "This issue lists Renovate updates and detected dependencies.\n - [x] <!-- manual job -->Check this box to trigger a request for Renovate to run again on this repository",
		},
		Repository: GiteaRepository{FullName: "example/repo"},
	}

	updateCalled := false
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeTestRenovateJob("renovate", "job1", "example/repo")}, nil
		},
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			updateCalled = true
			return nil
		},
	}
	server := &Server{manager: mockManager, logger: logr.Discard()}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitea", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Event", "issues")

	w := httptest.NewRecorder()
	server.giteaWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !updateCalled {
		t.Error("expected project status update to be called")
	}
}

func TestGiteaWebhookRequiresEventHeader(t *testing.T) {
	server := &Server{manager: &mockWebhookManager{}, logger: logr.Discard()}

	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitea", bytes.NewReader([]byte("{}")))
	// note: X-Forgejo-Event must not be accepted on the Gitea endpoint
	req.Header.Set("X-Forgejo-Event", "issues")

	w := httptest.NewRecorder()
	server.giteaWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for missing X-Gitea-Event header, got %d", http.StatusBadRequest, w.Code)
	}
}
