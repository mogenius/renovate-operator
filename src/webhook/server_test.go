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
	"github.com/gorilla/mux"
)

// A sender that hangs up before the operator answers (GitLab gives up after 10 seconds) must
// not cancel the work: the trigger is still scheduled, under a context that is alive but
// bounded.
func TestWebhookRoutesOutliveTheSender(t *testing.T) {
	payload := GitLabEvent{
		ObjectKind:       "issue",
		Project:          Project{PathWithNamespace: "org/repo"},
		ObjectAttributes: ObjectAttributes{Action: "update"},
		Changes: Changes{Description: ChangeDescription{
			Current: "- [x] <!-- rebase-all-open-prs -->rebase all",
		}},
	}

	var scheduled bool
	var ctxErr error
	var hasDeadline bool
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeTestRenovateJob("ns", "job", "org/repo")}, nil
		},
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			scheduled = true
			ctxErr = ctx.Err()
			_, hasDeadline = ctx.Deadline()
			return nil
		},
	}
	router := mux.NewRouter()
	RegisterWebhookRoutes(router, &Server{manager: mockManager, logger: logr.Discard()})

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	gone, hangUp := context.WithCancel(context.Background())
	hangUp()
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitlab", bytes.NewReader(body)).WithContext(gone)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !scheduled {
		t.Fatal("the trigger was not scheduled")
	}
	if ctxErr != nil {
		t.Errorf("handler context was cancelled with the sender's: %v", ctxErr)
	}
	if !hasDeadline {
		t.Error("handler context has no deadline, so a stuck delivery would never be released")
	}
}
