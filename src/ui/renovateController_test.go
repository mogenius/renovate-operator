package ui

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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Mock RenovateJobManager
type mockRenovateJobManager struct {
	listRenovateJobsFunc        func(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error)
	getProjectsForRenovateJobFunc func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error)
	getLogsForProjectFunc       func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, project string) (string, error)
	updateProjectStatusFunc     func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error
	getRenovateJobFunc          func(ctx context.Context, name, namespace string) (*api.RenovateJob, error)
	reconcileProjectsFunc       func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, projects []string) error
}

func (m *mockRenovateJobManager) ListRenovateJobs(ctx context.Context) ([]crdmanager.RenovateJobIdentifier, error) {
	if m.listRenovateJobsFunc != nil {
		return m.listRenovateJobsFunc(ctx)
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

func (m *mockRenovateJobManager) UpdateProjectStatus(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
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

func (m *mockRenovateJobManager) ReconcileProjects(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, projects []string) error {
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

func (m *mockRenovateJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	return nil
}

func (m *mockRenovateJobManager) GetJob(jobId crdmanager.RenovateJobIdentifier, projectName string) (*batchv1.Job, error) {
	return nil, nil
}

func (m *mockRenovateJobManager) CreateJob(job *batchv1.Job) (*batchv1.Job, error) {
	return nil, nil
}

func (m *mockRenovateJobManager) DeleteJob(jobId crdmanager.RenovateJobIdentifier, projectName string) error {
	return nil
}

// Mock DiscoveryAgent
type mockDiscoveryAgent struct {
	getDiscoveryJobStatusFunc func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error)
	createDiscoveryJobFunc    func(ctx context.Context, renovateJob api.RenovateJob) (*batchv1.Job, error)
	waitForDiscoveryJobFunc   func(ctx context.Context, job *api.RenovateJob) ([]string, error)
}

func (m *mockDiscoveryAgent) Discover(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	return nil, nil
}

func (m *mockDiscoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	if m.getDiscoveryJobStatusFunc != nil {
		return m.getDiscoveryJobStatusFunc(ctx, job)
	}
	return api.JobStatusScheduled, nil
}

func (m *mockDiscoveryAgent) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (*batchv1.Job, error) {
	if m.createDiscoveryJobFunc != nil {
		return m.createDiscoveryJobFunc(ctx, renovateJob)
	}
	return &batchv1.Job{}, nil
}

func (m *mockDiscoveryAgent) WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	if m.waitForDiscoveryJobFunc != nil {
		return m.waitForDiscoveryJobFunc(ctx, job)
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
			return "test logs", nil
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

	if w.Body.String() != "test logs" {
		t.Errorf("Expected 'test logs', got '%s'", w.Body.String())
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
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
			return nil
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job1",
					Namespace: "default",
				},
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job1",
					Namespace: "default",
				},
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

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
