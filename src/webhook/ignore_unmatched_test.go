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

func TestIgnoreUnmatchedProjects(t *testing.T) {
	const managedProject = "group/managed"

	authedJob := makeTestRenovateJob("default", "job", managedProject)
	authedJob.Spec.Webhook.Authentication = &api.RenovateWebhookAuth{Enabled: true}

	tests := []struct {
		name            string
		ignoreUnmatched bool
		project         string
		expectedStatus  int
		expectedField   string
		expectedValue   string
	}{
		{
			name:            "unmatched project is rejected by default",
			ignoreUnmatched: false,
			project:         "group/unmanaged",
			expectedStatus:  http.StatusUnauthorized,
			expectedField:   "error",
			expectedValue:   "unauthorized",
		},
		{
			name:            "unmatched project is ignored when enabled",
			ignoreUnmatched: true,
			project:         "group/unmanaged",
			expectedStatus:  http.StatusOK,
			expectedField:   "message",
			expectedValue:   "event ignored",
		},
		{
			name:            "failed authentication is still rejected when enabled",
			ignoreUnmatched: true,
			project:         managedProject,
			expectedStatus:  http.StatusUnauthorized,
			expectedField:   "error",
			expectedValue:   "unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			server := &Server{
				manager: &mockWebhookManager{
					listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
						return []api.RenovateJob{authedJob}, nil
					},
					isWebhookTokenValidFunc: func(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error) {
						return false, nil
					},
					updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
						updateCalled = true
						return nil
					},
				},
				logger:                  logr.Discard(),
				ignoreUnmatchedProjects: tt.ignoreUnmatched,
			}

			body, err := json.Marshal(GitLabEvent{
				ObjectKind:       "merge_request",
				Project:          Project{PathWithNamespace: tt.project},
				ObjectAttributes: ObjectAttributes{Action: "merge"},
			})
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitlab", bytes.NewReader(body))
			req.Header.Set("X-Gitlab-Token", "wrong-token")
			w := httptest.NewRecorder()
			server.gitLabWebhook(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			var response map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if response[tt.expectedField] != tt.expectedValue {
				t.Errorf("expected %s %q, got %q", tt.expectedField, tt.expectedValue, response[tt.expectedField])
			}
			if updateCalled {
				t.Error("expected no project status update")
			}
		})
	}
}
