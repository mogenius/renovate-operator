package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// makeTestAuthRenovateJob builds a RenovateJob with webhook authentication enabled.
func makeTestAuthRenovateJob(namespace, name string) api.RenovateJob {
	job := makeTestRenovateJob(namespace, name, "org/repo")
	job.Spec.Webhook.Authentication = &api.RenovateWebhookAuth{Enabled: true}
	return job
}

func TestScheduleAllProjects(t *testing.T) {
	const token = "s3cret"

	type batchCall struct {
		jobId  crdmanager.RenovateJobIdentifier
		filter func(p crdmanager.RenovateProjectStatus) bool
		status *types.RenovateStatusUpdate
	}

	tests := []struct {
		name       string
		url        string
		headers    map[string]string
		jobs       []api.RenovateJob
		batchErr   error
		wantStatus int
		wantBatch  bool
		wantSingle bool
	}{
		{
			name:       "bearer token schedules every project of the job",
			url:        "/webhook/v1/schedule?all=true&job=job1&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusOK,
			wantBatch:  true,
		},
		{
			name:       "gitlab token header schedules every project of the job",
			url:        "/webhook/v1/schedule?all=1&job=job1&namespace=ns1",
			headers:    map[string]string{"X-Gitlab-Token": token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusOK,
			wantBatch:  true,
		},
		{
			name:       "missing namespace",
			url:        "/webhook/v1/schedule?all=true&job=job1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing job",
			url:        "/webhook/v1/schedule?all=true&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "project and all together",
			url:        "/webhook/v1/schedule?all=true&job=job1&namespace=ns1&project=org%2Frepo",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid all value",
			url:        "/webhook/v1/schedule?all=yes&job=job1&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "job with authentication disabled is refused",
			url:        "/webhook/v1/schedule?all=true&job=job1&namespace=ns1",
			jobs:       []api.RenovateJob{makeTestRenovateJob("ns1", "job1", "org/repo")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong token",
			url:        "/webhook/v1/schedule?all=true&job=job1&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer wrong"},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no credential",
			url:        "/webhook/v1/schedule?all=true&job=job1&namespace=ns1",
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unknown job",
			url:        "/webhook/v1/schedule?all=true&job=job2&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "batch failure",
			url:        "/webhook/v1/schedule?all=true&job=job1&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			batchErr:   errors.New("conflict"),
			wantStatus: http.StatusInternalServerError,
			wantBatch:  true,
		},
		{
			name:       "all=false keeps the single project path",
			url:        "/webhook/v1/schedule?all=false&job=job1&namespace=ns1&project=org%2Frepo",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusOK,
			wantSingle: true,
		},
		{
			name:       "missing project without all is still rejected",
			url:        "/webhook/v1/schedule?job=job1&namespace=ns1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			jobs:       []api.RenovateJob{makeTestAuthRenovateJob("ns1", "job1")},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var batches []batchCall
			var singles int
			mockManager := &mockWebhookManager{
				listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
					return tt.jobs, nil
				},
				isWebhookTokenValidFunc: func(ctx context.Context, job crdmanager.RenovateJobIdentifier, got string) (bool, error) {
					return got == token, nil
				},
				updateProjectStatusBatchedFunc: func(ctx context.Context, fn func(p crdmanager.RenovateProjectStatus) bool, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
					batches = append(batches, batchCall{jobId: jobId, filter: fn, status: status})
					return tt.batchErr
				},
				updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
					singles++
					return nil
				},
			}
			router := mux.NewRouter()
			RegisterWebhookRoutes(router, &Server{manager: mockManager, logger: logr.Discard()})

			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, w.Code, w.Body.String())
			}
			if tt.wantSingle != (singles == 1) {
				t.Errorf("expected single project update %v, got %d calls", tt.wantSingle, singles)
			}
			if !tt.wantBatch {
				if len(batches) != 0 {
					t.Fatalf("expected no batch update, got %d", len(batches))
				}
				return
			}
			if len(batches) != 1 {
				t.Fatalf("expected one batch update, got %d", len(batches))
			}

			got := batches[0]
			if want := (crdmanager.RenovateJobIdentifier{Name: "job1", Namespace: "ns1"}); got.jobId != want {
				t.Errorf("expected job %+v, got %+v", want, got.jobId)
			}
			if got.status.Status != api.JobStatusScheduled || got.status.Priority != 1 {
				t.Errorf("expected Scheduled at priority 1, got %s at %d", got.status.Status, got.status.Priority)
			}
			if got.status.ExecutionOptions != nil {
				t.Errorf("expected no execution options, got %+v", got.status.ExecutionOptions)
			}
			for status, want := range map[api.RenovateProjectStatus]bool{
				api.JobStatusRunning:   false,
				api.JobStatusScheduled: true,
				api.JobStatusCompleted: true,
				api.JobStatusFailed:    true,
			} {
				if selected := got.filter(crdmanager.RenovateProjectStatus{Name: "org/repo", Status: status}); selected != want {
					t.Errorf("project in status %s: expected selected=%v, got %v", status, want, selected)
				}
			}
		})
	}
}
