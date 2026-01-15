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

func TestGitHubWebhook_Integration(t *testing.T) {
	tests := []struct {
		name               string
		payload            GitHubEvent
		namespace          string
		job                string
		expectedStatus     int
		expectedMessage    string
		shouldCallUpdate   bool
		updateProjectError error
	}{
		{
			name: "valid issue edited with checkbox checked",
			payload: GitHubEvent{
				Action: "edited",
				Issue: &GitHubIssue{
					ID:     618,
					Number: 618,
					Body:   "Dependency Dashboard\n - [x] <!-- rebase-check -->If you want to rebase/retry this PR",
				},
				Repository: GitHubRepository{
					ID:       935867937,
					Name:     "mo-argocd-applications",
					FullName: "mogenius/mo-argocd-applications",
				},
			},
			namespace:        "renovate-operator",
			job:              "1-gitops",
			expectedStatus:   http.StatusAccepted,
			expectedMessage:  "renovate job scheduled",
			shouldCallUpdate: true,
		},
		{
			name: "valid pull request edited with checkbox checked",
			payload: GitHubEvent{
				Action: "edited",
				PullRequest: &GitHubPullRequest{
					ID:     123,
					Number: 456,
					Body:   "Update dependency\n - [x] <!-- rebase-check -->If you want to rebase/retry this",
				},
				Repository: GitHubRepository{
					ID:       935867937,
					Name:     "test-repo",
					FullName: "mogenius/test-repo",
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusAccepted,
			expectedMessage:  "renovate job scheduled",
			shouldCallUpdate: true,
		},
		{
			name: "invalid action - opened",
			payload: GitHubEvent{
				Action: "opened",
				Issue: &GitHubIssue{
					Body: "Some body",
				},
				Repository: GitHubRepository{
					FullName: "mogenius/test",
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
		{
			name: "no pull request or issue",
			payload: GitHubEvent{
				Action: "edited",
				Repository: GitHubRepository{
					FullName: "mogenius/test",
				},
			},
			namespace:        "default",
			job:              "test-job",
			expectedStatus:   http.StatusOK,
			expectedMessage:  "event ignored",
			shouldCallUpdate: false,
		},
		{
			name: "empty body",
			payload: GitHubEvent{
				Action: "edited",
				Issue: &GitHubIssue{
					Body: "",
				},
				Repository: GitHubRepository{
					FullName: "mogenius/test",
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
			payload: GitHubEvent{
				Action: "edited",
				Issue: &GitHubIssue{
					Body: "Dependency Dashboard\n - [ ] <!-- rebase-check -->If you want to rebase/retry this PR",
				},
				Repository: GitHubRepository{
					FullName: "mogenius/test",
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
					if project != tt.payload.Repository.FullName {
						t.Errorf("expected project %s, got %s", tt.payload.Repository.FullName, project)
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
			url := "/webhook/v1/github"
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
			server.githubWebhook(w, req)

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

func TestGitHubWebhook_MissingQueryParams(t *testing.T) {
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

			payload := GitHubEvent{
				Action: "edited",
				Issue: &GitHubIssue{
					Body: "- [x] <!-- rebase-check -->If you want to rebase/retry this PR",
				},
				Repository: GitHubRepository{
					FullName: "test/repo",
				},
			}

			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			url := "/webhook/v1/github"
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
			server.githubWebhook(w, req)

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

func TestGitHubWebhook_InvalidJSON(t *testing.T) {
	mockManager := &mockWebhookManager{}
	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github?namespace=default&job=test-job", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	server.githubWebhook(w, req)

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
