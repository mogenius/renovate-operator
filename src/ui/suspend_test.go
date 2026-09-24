package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/renovate"
)

func TestGetRenovateJobsReportsSuspendedJob(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "paused", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 * * * *", Suspend: new(true)}},
		{Name: "active", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 * * * *"}},
	}
	server := &Server{
		manager: &mockRenovateJobManager{
			listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
				return jobs, nil
			},
		},
		logger:    logr.Discard(),
		discovery: &mockDiscoveryAgent{},
		scheduler: &mockScheduler{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/renovatejobs", nil)
	w := httptest.NewRecorder()
	server.getRenovateJobs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	byName := make(map[string]map[string]any, len(result))
	for _, job := range result {
		byName[job["name"].(string)] = job
	}

	paused, active := byName["paused"], byName["active"]
	if paused["suspended"] != true {
		t.Errorf("expected suspended=true, got %v", paused["suspended"])
	}
	if _, ok := paused["nextSchedule"]; ok {
		t.Errorf("a suspended job has no next run, got %v", paused["nextSchedule"])
	}
	if _, ok := active["suspended"]; ok {
		t.Errorf("expected suspended to be left out for an active job, got %v", active["suspended"])
	}
	if _, ok := active["nextSchedule"]; !ok {
		t.Error("expected a next run for the active job")
	}
}

func TestRunDiscoveryForProjectRefusesSuspendedJob(t *testing.T) {
	server := &Server{
		manager: &mockRenovateJobManager{
			getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
				return &api.RenovateJob{Name: name, Namespace: namespace, Spec: api.RenovateJobSpec{Suspend: new(true)}}, nil
			},
		},
		discovery: &mockDiscoveryAgent{
			getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
				return api.JobStatusCompleted, nil
			},
			createDiscoveryJobFunc: func(ctx context.Context, job api.RenovateJob) error {
				return renovate.ErrRenovateJobSuspended
			},
		},
		logger: logr.Discard(),
	}

	body, _ := json.Marshal(map[string]string{"renovateJob": "job1", "namespace": "default"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/discovery/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.runDiscoveryForProject(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}
