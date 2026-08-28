package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/policy"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/types"
)

// Mock RenovateJobManager
type mockRenovateJobManager struct {
	listRenovateJobsFunc          func(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error)
	listRenovateJobsFullFunc      func(ctx context.Context) ([]api.RenovateJob, error)
	getProjectsForRenovateJobFunc func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error)
	streamLogsForProjectFunc      func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (io.ReadCloser, error)
	updateProjectStatusFunc       func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
	getRenovateJobFunc            func(ctx context.Context, name, namespace string) (*api.RenovateJob, error)
	reconcileProjectsFunc         func(ctx context.Context, jobId *api.RenovateJob, projects []string) error
	cancelProjectJobFunc          func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier) error
}

func (m *mockRenovateJobManager) ListRenovateJobs(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error) {
	if m.listRenovateJobsFunc != nil {
		return m.listRenovateJobsFunc(ctx)
	}
	return nil, nil
}

func (m *mockRenovateJobManager) ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error) {
	if m.listRenovateJobsFullFunc != nil {
		return m.listRenovateJobsFullFunc(ctx)
	}
	return nil, nil
}

func (m *mockRenovateJobManager) GetProjectsForRenovateJob(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error) {
	if m.getProjectsForRenovateJobFunc != nil {
		return m.getProjectsForRenovateJobFunc(ctx, jobId)
	}
	return nil, nil
}

func (m *mockRenovateJobManager) StreamLogsForProject(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (io.ReadCloser, error) {
	if m.streamLogsForProjectFunc != nil {
		return m.streamLogsForProjectFunc(ctx, jobId, project)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockRenovateJobManager) UpdateProjectStatus(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	if m.updateProjectStatusFunc != nil {
		return m.updateProjectStatusFunc(ctx, project, jobId, status)
	}
	return nil
}

func (m *mockRenovateJobManager) GetRenovateJob(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
	if m.getRenovateJobFunc != nil {
		return m.getRenovateJobFunc(ctx, name, namespace)
	}
	return nil, nil
}

func (m *mockRenovateJobManager) SyncWebhooks(ctx context.Context, job crdmanager.RenovateJobIdentifier, removedProjects []string) error {
	return nil
}

func (m *mockRenovateJobManager) CleanupWebhooks(ctx context.Context, job crdmanager.RenovateJobIdentifier) error {
	return nil
}

func (m *mockRenovateJobManager) ReconcileProjects(ctx context.Context, jobId *api.RenovateJob, projects []string) ([]string, error) {
	if m.reconcileProjectsFunc != nil {
		return nil, m.reconcileProjectsFunc(ctx, jobId, projects)
	}
	return nil, nil
}

// Implement remaining interface methods as no-ops
func (m *mockRenovateJobManager) LoadRenovateJob(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
	return nil, nil
}

func (m *mockRenovateJobManager) ReloadRenovateJob(ctx context.Context, job *api.RenovateJob) error {
	return nil
}

func (m *mockRenovateJobManager) GetProjects(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, filter func(crdmanager.RenovateProjectStatus) bool) ([]string, error) {
	return nil, nil
}

func (m *mockRenovateJobManager) GetProjectsByStatus(ctx context.Context, job crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdmanager.RenovateProjectStatus, error) {
	return nil, nil
}

func (m *mockRenovateJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p crdmanager.RenovateProjectStatus) bool, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	return nil
}

func (m *mockRenovateJobManager) IsWebhookTokenValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error) {
	return true, nil
}
func (r *mockRenovateJobManager) IsWebhookSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error) {
	return true, nil
}
func (r *mockRenovateJobManager) IsWebhookStandardSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, msgID, timestamp, signature string, body []byte) (bool, error) {
	return true, nil
}

func (m *mockRenovateJobManager) SetAcceptedCondition(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, accepted bool, reason string, message string) error {
	return nil
}
func (m *mockRenovateJobManager) CancelProjectJob(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier) error {
	if m.cancelProjectJobFunc != nil {
		return m.cancelProjectJobFunc(ctx, project, jobId)
	}
	return nil
}

// Mock DiscoveryAgent
type mockDiscoveryAgent struct {
	getDiscoveryJobStatusFunc func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error)
	createDiscoveryJobFunc    func(ctx context.Context, renovateJob api.RenovateJob) error
}

func (m *mockDiscoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	if m.getDiscoveryJobStatusFunc != nil {
		return m.getDiscoveryJobStatusFunc(ctx, job)
	}
	return api.JobStatusScheduled, nil
}

func (m *mockDiscoveryAgent) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
	if m.createDiscoveryJobFunc != nil {
		return "", m.createDiscoveryJobFunc(ctx, renovateJob)
	}
	return "", nil
}

func (m *mockDiscoveryAgent) ProcessDiscoveryJobResult(ctx context.Context, k8sJob *batchv1.Job, jobId crdmanager.RenovateJobIdentifier) error {
	return nil
}

func TestGetRenovateJobs_Success(t *testing.T) {
	t.Skip("Skipping - needs getRenovateJobs handler to be updated to work with RenovateJobIdentifier interface")
}

func TestGetRenovateJobs_ListError(t *testing.T) {
	t.Skip("Skipping - needs getRenovateJobs handler to be updated to work with RenovateJobIdentifier interface")
}

func TestGetRenovateJobLogs_Success(t *testing.T) {
	payload := `{"level":30,"msg":"starting"}` + "\n" + `{"level":30,"msg":"done"}`
	mockManager := &mockRenovateJobManager{
		streamLogsForProjectFunc: func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(payload)), nil
		},
		getRenovateJobFunc: func(_ context.Context, _, _ string) (*api.RenovateJob, error) {
			return &api.RenovateJob{}, nil
		},
	}

	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?namespace=default&renovate=job1&project=project1", nil)
	w := httptest.NewRecorder()

	server.getRenovateJobLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if got := strings.Count(body, "\ndata: "); got != 2 {
		t.Errorf("Expected 2 SSE data events, got %d\nbody:\n%s", got, body)
	}
	if !strings.Contains(body, "event: done") {
		t.Error("Expected terminal 'event: done' in SSE response")
	}
}

func TestGetRenovateJobLogs_NonJSONLines(t *testing.T) {
	payload := "not json\n" + `{"level":30,"msg":"valid"}` + "\n\n"
	mockManager := &mockRenovateJobManager{
		streamLogsForProjectFunc: func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(payload)), nil
		},
		getRenovateJobFunc: func(_ context.Context, _, _ string) (*api.RenovateJob, error) {
			return &api.RenovateJob{}, nil
		},
	}

	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?namespace=default&renovate=job1&project=project1", nil)
	w := httptest.NewRecorder()

	server.getRenovateJobLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	// Count data events that are not the terminal "done" event
	dataCount := 0
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, "data: ") && line != "data: {}" {
			dataCount++
		}
	}
	if dataCount != 1 {
		t.Errorf("Expected 1 data event (non-JSON line skipped), got %d\nbody:\n%s", dataCount, body)
	}
}

func TestGetRenovateJsonBody_JSON(t *testing.T) {
	body := map[string]string{
		"renovateJob": "job1",
		"namespace":   "default",
		"project":     "project1",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	result, err := getRenovateJsonBody(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.name != "job1" {
		t.Errorf("Expected name 'job1', got '%s'", result.name)
	}
	if result.namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", result.namespace)
	}
	if result.project != "project1" {
		t.Errorf("Expected project 'project1', got '%s'", result.project)
	}
}

func TestGetRenovateJsonBody_FormValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/?renovateJob=job1&namespace=default&project=project1", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result, err := getRenovateJsonBody(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.name != "job1" {
		t.Errorf("Expected name 'job1', got '%s'", result.name)
	}
}

func TestRunRenovateForProject_Success(t *testing.T) {
	mockManager := &mockRenovateJobManager{
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			return nil
		},
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			return &api.RenovateJob{}, nil
		},
	}

	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
	}

	body := map[string]string{
		"renovateJob": "job1",
		"namespace":   "default",
		"project":     "project1",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/renovate", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.runRenovateForProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRunRenovateForProject_MissingParams(t *testing.T) {
	server := &Server{
		manager: &mockRenovateJobManager{},
		logger:  logr.Discard(),
	}

	body := map[string]string{
		"renovateJob": "job1",
		// Missing namespace and project
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/renovate", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.runRenovateForProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestDiscoveryStatusForProject_Success(t *testing.T) {
	mockManager := &mockRenovateJobManager{
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			return &api.RenovateJob{
				Name:      "job1",
				Namespace: "default",
			}, nil
		},
	}

	mockDiscovery := &mockDiscoveryAgent{
		getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
			return api.JobStatusRunning, nil
		},
	}

	server := &Server{
		manager:   mockManager,
		discovery: mockDiscovery,
		logger:    logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/status?namespace=default&renovate=job1", nil)
	w := httptest.NewRecorder()

	server.discoveryStatusForProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result struct {
		Status api.RenovateProjectStatus `json:"status"`
	}
	err := json.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Status != api.JobStatusRunning {
		t.Errorf("Expected status 'running', got '%s'", result.Status)
	}
}

func TestDiscoveryStatusForProject_NotFound(t *testing.T) {
	mockManager := &mockRenovateJobManager{
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			return &api.RenovateJob{
				Name:      "job1",
				Namespace: "default",
			}, nil
		},
	}

	mockDiscovery := &mockDiscoveryAgent{
		getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
			return "", k8serrors.NewNotFound(schema.GroupResource{}, "job1")
		},
	}

	server := &Server{
		manager:   mockManager,
		discovery: mockDiscovery,
		logger:    logr.Discard(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/status?namespace=default&renovate=job1", nil)
	w := httptest.NewRecorder()

	server.discoveryStatusForProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result struct {
		Status api.RenovateProjectStatus `json:"status"`
	}
	err := json.NewDecoder(w.Body).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// When not found, it should return scheduled
	if result.Status != api.JobStatusScheduled {
		t.Errorf("Expected status 'scheduled', got '%s'", result.Status)
	}
}

func TestRunDiscoveryForProject_AlreadyRunning(t *testing.T) {
	mockManager := &mockRenovateJobManager{
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			return &api.RenovateJob{
				Name:      "job1",
				Namespace: "default",
			}, nil
		},
	}

	mockDiscovery := &mockDiscoveryAgent{
		getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
			return api.JobStatusRunning, nil
		},
	}

	server := &Server{
		manager:   mockManager,
		discovery: mockDiscovery,
		logger:    logr.Discard(),
	}

	body := map[string]string{
		"renovateJob": "job1",
		"namespace":   "default",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/discovery/start", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.runDiscoveryForProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Additional mock types needed for authorization tests
type mockScheduler struct{}

func (m *mockScheduler) Start()                                                          {}
func (m *mockScheduler) Stop()                                                           {}
func (m *mockScheduler) AddSchedule(expr string, namespace, job string, fn func()) error { return nil }
func (m *mockScheduler) AddScheduleReplaceExisting(expr string, namespace, job string, fn func()) error {
	return nil
}
func (m *mockScheduler) RemoveSchedule(namespace, job string) {}
func (m *mockScheduler) GetNextRunOnSchedule(schedule, key string) time.Time {
	return time.Now().Add(24 * time.Hour)
}

func TestFilterReadableJobs(t *testing.T) {
	adminGroups := []string{"team-admin"}
	readerGroups := []string{"team-reader"}

	tests := []struct {
		name        string
		jobs        []api.RenovateJob
		authEnabled bool
		session     *sessionData
		defaults    AccessDefaults
		wantJobs    []string
		wantRoles   []string
	}{
		{
			name: "auth disabled - every job is admin",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: adminGroups}}},
				{Name: "job2"},
			},
			authEnabled: false,
			wantJobs:    []string{"job1", "job2"},
			wantRoles:   []string{"admin", "admin"},
		},
		{
			name: "auth enabled without session - nothing visible",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: adminGroups}}},
			},
			authEnabled: true,
			wantJobs:    []string{},
		},
		{
			name: "job without access configuration is hidden (fail closed)",
			jobs: []api.RenovateJob{
				{Name: "job1"},
				{Name: "job2", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-admin"}},
			wantJobs:    []string{},
		},
		{
			name: "admin group grants admin, reader group grants reader",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: adminGroups}}},
				{Name: "job2", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: readerGroups}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-admin", "team-reader"}},
			wantJobs:    []string{"job1", "job2"},
			wantRoles:   []string{"admin", "reader"},
		},
		{
			name: "admin wins when a user holds both group kinds",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: readerGroups, AdminGroups: adminGroups}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-reader", "team-admin"}},
			wantJobs:    []string{"job1"},
			wantRoles:   []string{"admin"},
		},
		{
			name: "deprecated allowedGroups still grants admin",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-legacy"}}}, //nolint:staticcheck // deprecated field is intentionally still honoured
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-legacy"}},
			wantJobs:    []string{"job1"},
			wantRoles:   []string{"admin"},
		},
		{
			name: "group names are matched case-insensitively",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"Team-Admin"}}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"TEAM-ADMIN"}},
			wantJobs:    []string{"job1"},
			wantRoles:   []string{"admin"},
		},
		{
			name: "operator defaults apply to jobs that set no groups",
			jobs: []api.RenovateJob{
				{Name: "job1"},
				{Name: "job2", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-other"}}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-default"}},
			defaults:    AccessDefaults{AdminGroups: []string{"team-default"}},
			wantJobs:    []string{"job1"},
			wantRoles:   []string{"admin"},
		},
		{
			name: "anonymous read grants reader without a session",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true)}}},
				{Name: "job2", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: adminGroups}}},
			},
			authEnabled: true,
			wantJobs:    []string{"job1"},
			wantRoles:   []string{"reader"},
		},
		{
			name: "anonymous read is a floor for sessions without matching groups",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true), AdminGroups: adminGroups}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-unrelated"}},
			wantJobs:    []string{"job1"},
			wantRoles:   []string{"reader"},
		},
		{
			name: "job opts out of the anonymous read default",
			jobs: []api.RenovateJob{
				{Name: "job1", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(false)}}},
			},
			authEnabled: true,
			defaults:    AccessDefaults{AnonymousRead: true},
			wantJobs:    []string{},
		},
		{
			name: "access and deprecated allowedGroups together fail closed",
			jobs: []api.RenovateJob{
				{
					Name: "job1",
					Spec: api.RenovateJobSpec{
						AllowedGroups: []string{"team-legacy"}, //nolint:staticcheck // deprecated field is intentionally still honoured
						Access:        &api.RenovateJobAccess{AdminGroups: adminGroups},
					},
				},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-legacy", "team-admin"}},
			wantJobs:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{logger: logr.Discard(), accessDefaults: tt.defaults}
			if tt.authEnabled {
				server.auth = &OIDCAuth{}
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.session != nil {
				req = req.WithContext(context.WithValue(req.Context(), sessionContextKey, tt.session))
			}

			jobs, decisions := server.filterReadableJobs(req, tt.jobs)

			if len(jobs) != len(tt.wantJobs) {
				t.Fatalf("filterReadableJobs() len = %d, want %d", len(jobs), len(tt.wantJobs))
			}
			for i, wantName := range tt.wantJobs {
				if jobs[i].Name != wantName {
					t.Errorf("job[%d] = %q, want %q", i, jobs[i].Name, wantName)
				}
				if i < len(tt.wantRoles) && decisions[i].Role.String() != tt.wantRoles[i] {
					t.Errorf("job %q role = %q, want %q", jobs[i].Name, decisions[i].Role.String(), tt.wantRoles[i])
				}
			}
		})
	}
}

func TestHasIntersection(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{"both empty", []string{}, []string{}, false},
		{"one empty", []string{"a"}, []string{}, false},
		{"no intersection", []string{"a", "b"}, []string{"c", "d"}, false},
		{"exact match", []string{"a"}, []string{"a"}, true},
		{"partial intersection", []string{"a", "b", "c"}, []string{"c", "d"}, true},
		{"subset", []string{"a"}, []string{"a", "b", "c"}, true},
		{"nil slices", nil, nil, false},
		{"one nil", []string{"a"}, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasIntersection(tt.a, tt.b); got != tt.want {
				t.Errorf("hasIntersection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRenovateJobs_WithAuthorization(t *testing.T) {
	tests := []struct {
		name          string
		jobs          []api.RenovateJob
		sessionGroups []string
		authEnabled   bool
		wantCount     int
	}{
		{
			name: "authenticated user with matching group",
			jobs: []api.RenovateJob{
				{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-a"}}},
			},
			sessionGroups: []string{"team-a"},
			authEnabled:   true,
			wantCount:     1,
		},
		{
			name: "authenticated user without matching group",
			jobs: []api.RenovateJob{
				{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-a"}}},
			},
			sessionGroups: []string{"team-b"},
			authEnabled:   true,
			wantCount:     0,
		},
		{
			name: "auth disabled - all jobs visible",
			jobs: []api.RenovateJob{
				{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-a"}}},
				{Name: "job2", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-b"}}},
			},
			sessionGroups: nil,
			authEnabled:   false,
			wantCount:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := &mockRenovateJobManager{
				listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
					return tt.jobs, nil
				},
			}

			server := &Server{
				manager:   mockManager,
				logger:    logr.Discard(),
				discovery: &mockDiscoveryAgent{},
				scheduler: &mockScheduler{},
			}

			if tt.authEnabled {
				// Set a dummy auth provider to enable auth
				server.auth = &OIDCAuth{}
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/renovatejobs", nil)

			if tt.authEnabled && tt.sessionGroups != nil {
				session := &sessionData{
					Email:  "test@example.com",
					Groups: tt.sessionGroups,
				}
				ctx := context.WithValue(req.Context(), sessionContextKey, session)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			server.getRenovateJobs(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}

			var result []RenovateJobInfo
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("Expected %d jobs, got %d", tt.wantCount, len(result))
			}
		})
	}
}

func TestGetRenovateJobs_ManyMissingDiscoveryJobsReturnsQuickly(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	const jobCount = 5
	jobs := make([]api.RenovateJob, 0, jobCount)
	for i := range jobCount {
		jobs = append(jobs, api.RenovateJob{
			Name:      "job-" + string(rune('a'+i)),
			Namespace: "default",
			Spec:      api.RenovateJobSpec{Schedule: "0 0 * * *"},
		})
	}

	mockManager := &mockRenovateJobManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return jobs, nil
		},
	}

	server := &Server{
		manager:   mockManager,
		logger:    logr.Discard(),
		discovery: renovate.NewDiscoveryAgent(scheme, fake.NewClientBuilder().WithScheme(scheme).Build(), logr.Discard(), nil, nil, policy.Policy{}),
		scheduler: &mockScheduler{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/renovatejobs", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	server.getRenovateJobs(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result []RenovateJobInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result) != jobCount {
		t.Fatalf("Expected %d jobs, got %d", jobCount, len(result))
	}
	if elapsed > 750*time.Millisecond {
		t.Fatalf("getRenovateJobs took %s for %d jobs without discovery jobs, expected it to return without per-job polling", elapsed, jobCount)
	}
}

func TestGetRenovateJobLogs_Authorization(t *testing.T) {
	tests := []struct {
		name           string
		job            *api.RenovateJob
		userGroups     []string
		authEnabled    bool
		wantStatusCode int
	}{
		{
			name: "authorized user can access logs",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-a"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "user without access gets 404 so the job's existence stays hidden",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "reader by group match may stream logs",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-b"}}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "anonymous reader without log access gets 403",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true)}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "anonymous reader with log access may stream logs",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true), AnonymousReadLogs: new(true)}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "auth disabled - all users can access",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    false,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := &mockRenovateJobManager{
				getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
					return tt.job, nil
				},
				streamLogsForProjectFunc: func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader(`{"level":30,"msg":"test"}`)), nil
				},
			}

			server := &Server{
				manager: mockManager,
				logger:  logr.Discard(),
			}

			if tt.authEnabled {
				server.auth = &OIDCAuth{}
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?namespace=default&renovate=job1&project=test", nil)

			if tt.authEnabled {
				session := &sessionData{
					Email:  "test@example.com",
					Groups: tt.userGroups,
				}
				ctx := context.WithValue(req.Context(), sessionContextKey, session)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			server.getRenovateJobLogs(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("Expected status %d, got %d", tt.wantStatusCode, w.Code)
			}
		})
	}
}

func TestAuthorizeJobAccess_DirectBypassAttempt(t *testing.T) {
	// Test that users cannot bypass authorization by directly calling endpoints
	// with correct namespace/job name but without proper group membership

	job := &api.RenovateJob{
		Name: "secret-job", Namespace: "default",
		Spec: api.RenovateJobSpec{AllowedGroups: []string{"admin"}},
	}

	mockManager := &mockRenovateJobManager{
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			return job, nil
		},
	}

	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
		auth:    &OIDCAuth{}, // Auth enabled
	}

	// User with wrong group tries to access job by knowing its name
	session := &sessionData{
		Email:  "attacker@example.com",
		Groups: []string{"regular-user"},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), sessionContextKey, session)
	req = req.WithContext(ctx)

	_, decision := server.resolveJobAccess(req, "default", "secret-job")
	if decision.canRead() {
		t.Error("User should not be able to read a job whose groups they do not hold")
	}
}

// TestResolveJobAccessAvoidsDoubleFetch verifies we don't fetch jobs twice
func TestResolveJobAccessAvoidsDoubleFetch(t *testing.T) {
	fetchCount := 0
	mockManager := &mockRenovateJobManager{
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			fetchCount++
			return &api.RenovateJob{}, nil
		},
	}

	server := &Server{
		manager: mockManager,
		logger:  logr.Discard(),
		auth:    nil, // Auth disabled for simplicity
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	job, decision := server.resolveJobAccess(req, "default", "job1")

	if !decision.canWrite() {
		t.Error("Expected admin access when no auth provider is configured")
	}
	if job == nil {
		t.Error("Expected job to be returned")
	}
	if fetchCount != 1 {
		t.Errorf("Expected GetRenovateJob to be called exactly once, got %d calls", fetchCount)
	}
}

func TestRunRenovateForAllProjects_Authorization(t *testing.T) {
	tests := []struct {
		name           string
		job            *api.RenovateJob
		userGroups     []string
		authEnabled    bool
		wantStatusCode int
	}{
		{
			name: "authorized user can trigger all projects",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-a"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "user without access gets 404 so the job's existence stays hidden",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name: "reader gets 403 on write actions",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-b"}}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "auth disabled - all users can trigger",
			job: &api.RenovateJob{
				Name: "job1", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    false,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := &mockRenovateJobManager{
				getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
					return tt.job, nil
				},
			}

			server := &Server{
				manager: mockManager,
				logger:  logr.Discard(),
			}

			if tt.authEnabled {
				server.auth = &OIDCAuth{}
			}

			body := map[string]string{
				"renovateJob": "job1",
				"namespace":   "default",
			}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/renovate/all", bytes.NewReader(jsonBody))
			req.Header.Set("Content-Type", "application/json")

			if tt.authEnabled {
				session := &sessionData{
					Email:  "test@example.com",
					Groups: tt.userGroups,
				}
				ctx := context.WithValue(req.Context(), sessionContextKey, session)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()
			server.runRenovateForAllProjects(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("Expected status %d, got %d", tt.wantStatusCode, w.Code)
			}
		})
	}
}
