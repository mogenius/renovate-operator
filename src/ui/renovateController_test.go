package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"
	"testing"
	"time"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Mock RenovateJobManager
type mockRenovateJobManager struct {
	listRenovateJobsFunc          func(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error)
	listRenovateJobsFullFunc      func(ctx context.Context) ([]api.RenovateJob, error)
	getProjectsForRenovateJobFunc func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error)
	getLogsForProjectFunc         func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error)
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

func (m *mockRenovateJobManager) GetLogsForProject(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error) {
	if m.getLogsForProjectFunc != nil {
		return m.getLogsForProjectFunc(ctx, jobId, project)
	}
	return "", nil
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

func (m *mockRenovateJobManager) ReconcileProjects(ctx context.Context, jobId *api.RenovateJob, projects []string) error {
	if m.reconcileProjectsFunc != nil {
		return m.reconcileProjectsFunc(ctx, jobId, projects)
	}
	return nil
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

func (m *mockRenovateJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	return nil
}

func (m *mockRenovateJobManager) IsWebhookTokenValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, token string) (bool, error) {
	return true, nil
}
func (r *mockRenovateJobManager) IsWebhookSignatureValid(ctx context.Context, job crdmanager.RenovateJobIdentifier, signature string, body []byte) (bool, error) {
	return true, nil
}

func (m *mockRenovateJobManager) UpdateExecutionOptions(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, options *api.RenovateExecutionOptions) error {
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
	getDiscoveryJobStatusFunc func(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error)
	createDiscoveryJobFunc    func(ctx context.Context, renovateJob api.RenovateJob) error
	waitForDiscoveryJobFunc   func(ctx context.Context, job *api.RenovateJob, generation string) ([]string, error)
}

func (m *mockDiscoveryAgent) Discover(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	return nil, nil
}

func (m *mockDiscoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
	if m.getDiscoveryJobStatusFunc != nil {
		return m.getDiscoveryJobStatusFunc(ctx, job, generation)
	}
	return api.JobStatusScheduled, nil
}

func (m *mockDiscoveryAgent) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (string, error) {
	if m.createDiscoveryJobFunc != nil {
		return "", m.createDiscoveryJobFunc(ctx, renovateJob)
	}
	return "", nil
}

func (m *mockDiscoveryAgent) WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob, generation string) ([]string, error) {
	if m.waitForDiscoveryJobFunc != nil {
		return m.waitForDiscoveryJobFunc(ctx, job, generation)
	}
	return []string{}, nil
}

func TestGetRenovateJobs_Success(t *testing.T) {
	t.Skip("Skipping - needs getRenovateJobs handler to be updated to work with RenovateJobIdentifier interface")
}

func TestGetRenovateJobs_ListError(t *testing.T) {
	t.Skip("Skipping - needs getRenovateJobs handler to be updated to work with RenovateJobIdentifier interface")
}

func TestGetRenovateJobLogs_Success(t *testing.T) {
	mockManager := &mockRenovateJobManager{
		getLogsForProjectFunc: func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error) {
			return `{"level":30,"msg":"starting"}` + "\n" + `{"level":30,"msg":"done"}`, nil
		},
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
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

	var entries []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("Expected valid JSON array, got error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("Expected 2 log entries, got %d", len(entries))
	}
}

func TestGetRenovateJobLogs_NonJSONLines(t *testing.T) {
	mockManager := &mockRenovateJobManager{
		getLogsForProjectFunc: func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error) {
			return "not json\n" + `{"level":30,"msg":"valid"}` + "\n\n", nil
		},
		getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
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

	var entries []json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("Expected valid JSON array, got error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("Expected 1 valid log entry (non-JSON line skipped), got %d", len(entries))
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job1",
					Namespace: "default",
				},
			}, nil
		},
	}

	mockDiscovery := &mockDiscoveryAgent{
		getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job1",
					Namespace: "default",
				},
			}, nil
		},
	}

	mockDiscovery := &mockDiscoveryAgent{
		getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job1",
					Namespace: "default",
				},
			}, nil
		},
	}

	mockDiscovery := &mockDiscoveryAgent{
		getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
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

func (m *mockScheduler) Start()                                                {}
func (m *mockScheduler) Stop()                                                 {}
func (m *mockScheduler) AddSchedule(expr string, name string, fn func()) error { return nil }
func (m *mockScheduler) AddScheduleReplaceExisting(expr string, name string, fn func()) error {
	return nil
}
func (m *mockScheduler) RemoveSchedule(name string) {}
func (m *mockScheduler) GetNextRunOnSchedule(schedule string) time.Time {
	return time.Now().Add(24 * time.Hour)
}

func TestFilterRenovateJobsByGroups(t *testing.T) {
	tests := []struct {
		name        string
		jobs        []api.RenovateJob
		authEnabled bool
		session     *sessionData
		wantLen     int
		wantJobs    []string // job names expected in result
	}{
		{
			name: "auth disabled - all jobs visible",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "job2"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-b"}}},
			},
			authEnabled: false,
			session:     nil,
			wantLen:     2,
			wantJobs:    []string{"job1", "job2"},
		},
		{
			name: "auth enabled but no session - empty list (security)",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
			},
			authEnabled: true,
			session:     nil,
			wantLen:     0,
			wantJobs:    []string{},
		},
		{
			name: "user with matching group sees job",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-a"}},
			wantLen:     1,
			wantJobs:    []string{"job1"},
		},
		{
			name: "user without matching group sees nothing",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-b"}},
			wantLen:     0,
			wantJobs:    []string{},
		},
		{
			name: "user with no groups sees nothing",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: nil},
			wantLen:     0,
			wantJobs:    []string{},
		},
		{
			name: "job without allowedGroups visible to all authenticated users",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: nil}},
				{ObjectMeta: metav1.ObjectMeta{Name: "job2"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-a"}},
			wantLen:     2,
			wantJobs:    []string{"job1", "job2"},
		},
		{
			name: "user with multiple groups sees all matching jobs",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "job2"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-b"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "job3"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-c"}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-a", "team-b"}},
			wantLen:     2,
			wantJobs:    []string{"job1", "job2"},
		},
		{
			name: "job with multiple groups matches any user group",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a", "team-b", "team-c"}}},
			},
			authEnabled: true,
			session:     &sessionData{Groups: []string{"team-b"}},
			wantLen:     1,
			wantJobs:    []string{"job1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRenovateJobsByGroups(tt.jobs, tt.authEnabled, tt.session, nil)
			if len(result) != tt.wantLen {
				t.Errorf("filterRenovateJobsByGroups() len = %v, want %v", len(result), tt.wantLen)
			}
			// Verify expected jobs are in result
			for _, wantName := range tt.wantJobs {
				found := false
				for _, job := range result {
					if job.Name == wantName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected job %s not found in result", wantName)
				}
			}
		})
	}
}

func TestFilterRenovateJobsByGroups_WithDefaults(t *testing.T) {
	tests := []struct {
		name                 string
		jobs                 []api.RenovateJob
		authEnabled          bool
		session              *sessionData
		defaultAllowedGroups []string
		wantLen              int
		wantJobs             []string
	}{
		{
			name: "job without allowedGroups uses defaults - user has default group",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: nil}},
			},
			authEnabled:          true,
			session:              &sessionData{Groups: []string{"default-team"}},
			defaultAllowedGroups: []string{"default-team"},
			wantLen:              1,
			wantJobs:             []string{"job1"},
		},
		{
			name: "job without allowedGroups uses defaults - user lacks default group",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: nil}},
			},
			authEnabled:          true,
			session:              &sessionData{Groups: []string{"team-a"}},
			defaultAllowedGroups: []string{"default-team"},
			wantLen:              0,
			wantJobs:             []string{},
		},
		{
			name: "job with explicit allowedGroups ignores defaults",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
			},
			authEnabled:          true,
			session:              &sessionData{Groups: []string{"default-team"}},
			defaultAllowedGroups: []string{"default-team"},
			wantLen:              0,
			wantJobs:             []string{},
		},
		{
			name: "job without allowedGroups and no defaults - visible to all authenticated users",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: nil}},
			},
			authEnabled:          true,
			session:              &sessionData{Groups: []string{"team-a"}},
			defaultAllowedGroups: nil,
			wantLen:              1,
			wantJobs:             []string{"job1"},
		},
		{
			name: "multiple defaults - user has one",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: nil}},
			},
			authEnabled:          true,
			session:              &sessionData{Groups: []string{"team-b"}},
			defaultAllowedGroups: []string{"team-a", "team-b", "team-c"},
			wantLen:              1,
			wantJobs:             []string{"job1"},
		},
		{
			name: "mixed jobs - some with explicit groups, some using defaults",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1"}, Spec: api.RenovateJobSpec{AllowedGroups: nil}},
				{ObjectMeta: metav1.ObjectMeta{Name: "job2"}, Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-a"}}},
			},
			authEnabled:          true,
			session:              &sessionData{Groups: []string{"team-a"}},
			defaultAllowedGroups: []string{"default-team"},
			wantLen:              1,
			wantJobs:             []string{"job2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRenovateJobsByGroups(tt.jobs, tt.authEnabled, tt.session, tt.defaultAllowedGroups)
			if len(result) != tt.wantLen {
				t.Errorf("filterRenovateJobsByGroups() len = %v, want %v", len(result), tt.wantLen)
			}
			// Verify expected jobs are in result
			for _, wantName := range tt.wantJobs {
				found := false
				for _, job := range result {
					if job.Name == wantName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected job %s not found in result", wantName)
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
				{ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-a"}}},
			},
			sessionGroups: []string{"team-a"},
			authEnabled:   true,
			wantCount:     1,
		},
		{
			name: "authenticated user without matching group",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-a"}}},
			},
			sessionGroups: []string{"team-b"},
			authEnabled:   true,
			wantCount:     0,
		},
		{
			name: "auth disabled - all jobs visible",
			jobs: []api.RenovateJob{
				{ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-a"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "job2", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "0 0 * * *", AllowedGroups: []string{"team-b"}}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-a"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "unauthorized user gets 403",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "auth disabled - all users can access",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
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
				getLogsForProjectFunc: func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error) {
					return "test logs", nil
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
		ObjectMeta: metav1.ObjectMeta{Name: "secret-job", Namespace: "default"},
		Spec:       api.RenovateJobSpec{AllowedGroups: []string{"admin"}},
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

	authorized := server.authorizeJobAccess(req, "default", "secret-job")
	if authorized {
		t.Error("User should not be authorized to access job with different group")
	}
}

// TestAuthorizeAndGetJobAvoidsDoubleFetch verifies we don't fetch jobs twice
func TestAuthorizeAndGetJobAvoidsDoubleFetch(t *testing.T) {
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

	// First call to authorizeAndGetJob
	job, authorized := server.authorizeAndGetJob(req, "default", "job1")

	if !authorized {
		t.Error("Expected authorization to succeed")
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
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-a"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "unauthorized user gets 403",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "auth disabled - all users can trigger",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
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

func TestUpdateExecutionOptions_Authorization(t *testing.T) {
	tests := []struct {
		name           string
		job            *api.RenovateJob
		userGroups     []string
		authEnabled    bool
		wantStatusCode int
	}{
		{
			name: "authorized user can update execution options",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-a"},
			authEnabled:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name: "unauthorized user gets 403",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
			},
			userGroups:     []string{"team-b"},
			authEnabled:    true,
			wantStatusCode: http.StatusForbidden,
		},
		{
			name: "auth disabled - all users can update",
			job: &api.RenovateJob{
				ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
				Spec:       api.RenovateJobSpec{AllowedGroups: []string{"team-a"}},
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

			body := map[string]interface{}{
				"renovateJob": "job1",
				"namespace":   "default",
				"debug":       true,
			}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/executionOptions", bytes.NewReader(jsonBody))
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
			server.updateExecutionOptions(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("Expected status %d, got %d", tt.wantStatusCode, w.Code)
			}
		})
	}
}
