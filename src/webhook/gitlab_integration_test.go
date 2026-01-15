package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"testing"

	"github.com/go-logr/logr"
)

func TestGitLabWebhook_Integration(t *testing.T) {
	tests := []struct {
		name               string
		payload            GitLabEvent
		namespace          string
		job                string
		expectedStatus     int
		expectedMessage    string
		shouldCallUpdate   bool
		updateProjectError error
	}{
		{
			name: "valid merge request with checkbox checked",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				EventType:  "merge_request",
				Project: Project{
					ID:                185,
					Name:              "baynes-applications",
					Namespace:         "kubernetes",
					PathWithNamespace: "mogenius/mo-argocd-applications",
				},
				ObjectAttributes: ObjectAttributes{
					ID:     109564,
					Action: "update",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Previous: "Some description\n - [ ] <!-- rebase-check -->If you want to rebase/retry this MR",
						Current:  "Some description\n - [x] <!-- rebase-check -->If you want to rebase/retry this MR",
					},
				},
			},
			namespace:        "renovate-operator",
			job:              "1-gitops",
			expectedStatus:   http.StatusAccepted,
			expectedMessage:  "renovate job scheduled",
			shouldCallUpdate: true,
		},
		{
			name: "valid issue update with checkbox checked",
			payload: GitLabEvent{
				ObjectKind: "issue",
				EventType:  "issue",
				Project: Project{
					ID:                100,
					Name:              "test-project",
					Namespace:         "test",
					PathWithNamespace: "test/test-project",
				},
				ObjectAttributes: ObjectAttributes{
					ID:     12345,
					Action: "update",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Previous: "Old description",
						Current:  "Updated description\n - [x] <!-- rebase-all-open-prs -->**Click on this checkbox to rebase all",
					},
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusAccepted,
			expectedMessage:  "renovate job scheduled",
			shouldCallUpdate: true,
		},
		{
			name: "invalid object kind - note",
			payload: GitLabEvent{
				ObjectKind: "note",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					PathWithNamespace: "test/project",
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
		{
			name: "invalid action - open",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "open",
				},
				Project: Project{
					PathWithNamespace: "test/project",
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
		{
			name: "no description change",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					PathWithNamespace: "test/project",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Previous: "",
						Current:  "",
					},
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
		{
			name: "checkbox not checked",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					PathWithNamespace: "test/project",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Current: "Some update\n - [ ] <!-- rebase-check -->If you want to rebase/retry this MR",
					},
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
		{
			name: "not a renovate checkbox",
			payload: GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					PathWithNamespace: "test/project",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Current: "Some update\n - [x] Regular checkbox, not renovate",
					},
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateCalled := false
			mockManager := &mockWebhookManager{
				updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
					updateCalled = true
					if project != tt.payload.Project.PathWithNamespace {
						t.Errorf("expected project %s, got %s", tt.payload.Project.PathWithNamespace, project)
					}
					if jobId.Name != tt.job {
						t.Errorf("expected job name %s, got %s", tt.job, jobId.Name)
					}
					if jobId.Namespace != tt.namespace {
						t.Errorf("expected namespace %s, got %s", tt.namespace, jobId.Namespace)
					}
					if status != api.JobStatusScheduled {
						t.Errorf("expected status %s, got %s", api.JobStatusScheduled, status)
					}
					return tt.updateProjectError
				},
			}

			server := &Server{
				manager: mockManager,
				logger:  logr.Discard(),
			}

			// Create request body
			body, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			// Create request
			url := "/webhook/v1/gitlab"
			if tt.namespace != "" || tt.job != "" {
				url += "?"
				if tt.namespace != "" {
					url += "namespace=" + tt.namespace
				}
				if tt.job != "" {
					if tt.namespace != "" {
						url += "&"
					}
					url += "job=" + tt.job
				}
			}
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			// Record response
			w := httptest.NewRecorder()

			// Call handler
			server.gitLabWebhook(w, req)

			// Check response
			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check response body
			var response map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if message, ok := response["message"]; ok {
				if message != tt.expectedMessage {
					t.Errorf("expected message %q, got %q", tt.expectedMessage, message)
				}
			}

			// Verify update was called if expected
			if updateCalled != tt.shouldCallUpdate {
				t.Errorf("expected updateCalled=%v, got %v", tt.shouldCallUpdate, updateCalled)
			}
		})
	}
}

func TestGitLabWebhook_MissingQueryParams(t *testing.T) {
	tests := []struct {
		name           string
		namespace      string
		job            string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing namespace",
			namespace:      "",
			job:            "test-job",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "missing namespace or job query parameter",
		},
		{
			name:           "missing job",
			namespace:      "default",
			job:            "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "missing namespace or job query parameter",
		},
		{
			name:           "missing both parameters",
			namespace:      "",
			job:            "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "missing namespace or job query parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := &mockWebhookManager{}
			server := &Server{
				manager: mockManager,
				logger:  logr.Discard(),
			}

			payload := GitLabEvent{
				ObjectKind: "merge_request",
				ObjectAttributes: ObjectAttributes{
					Action: "update",
				},
				Project: Project{
					PathWithNamespace: "test/repo",
				},
				Changes: Changes{
					Description: ChangeDescription{
						Current: "- [x] <!-- rebase-check -->If you want to rebase/retry this MR, check this box",
					},
				},
			}

			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			url := "/webhook/v1/gitlab"
			if tt.namespace != "" || tt.job != "" {
				url += "?"
				if tt.namespace != "" {
					url += "namespace=" + tt.namespace
				}
				if tt.job != "" {
					if tt.namespace != "" {
						url += "&"
					}
					url += "job=" + tt.job
				}
			}

			req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			server.gitLabWebhook(w, req)
			t.Logf("url %s", url)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if response["error"] != tt.expectedError {
				t.Errorf("expected error %q, got %q", tt.expectedError, response["error"])
			}
		})
	}
}

func TestGitLabWebhook_InvalidJSON(t *testing.T) {
	mockManager := &mockWebhookManager{}
	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitlab?namespace=default&job=test-job", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.gitLabWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["error"] != "failed to decode payload" {
		t.Errorf("expected error 'failed to decode payload', got %q", response["error"])
	}
}

func TestGitLabWebhook_RealWorldPayload(t *testing.T) {
	// Using a realistic payload similar to test/webhook/gitlab.http
	payload := GitLabEvent{
		ObjectKind: "merge_request",
		EventType:  "merge_request",
		Project: Project{
			ID:                185,
			Name:              "baynes-applications",
			Namespace:         "kubernetes",
			PathWithNamespace: "infrastructure/kubernetes/baynes-applications",
		},
		ObjectAttributes: ObjectAttributes{
			ID:     109564,
			Action: "update",
		},
		Changes: Changes{
			Description: ChangeDescription{
				Previous: "This MR contains the following updates:\n\n| Package | Update | Change |\n|---|---|---|\n| [policy-reporter](https://kyverno.github.io/policy-reporter) | patch | `3.7.0` -> `3.7.1` |\n\n - [ ] <!-- rebase-check -->If you want to rebase/retry this MR, check this box",
				Current:  "This MR contains the following updates:\n\n| Package | Update | Change |\n|---|---|---|\n| [policy-reporter](https://kyverno.github.io/policy-reporter) | patch | `3.7.0` -> `3.7.1` |\n\n - [x] <!-- rebase-check -->If you want to rebase/retry this MR, check this box",
			},
		},
	}

	updateCalled := false
	mockManager := &mockWebhookManager{
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
			updateCalled = true
			return nil
		},
	}

	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/gitlab?namespace=renovate-operator&job=1-gitops", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.gitLabWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["message"] != "renovate job scheduled" {
		t.Errorf("expected message 'renovate job scheduled', got %q", response["message"])
	}

	if !updateCalled {
		t.Error("expected UpdateProjectStatus to be called")
	}
}
