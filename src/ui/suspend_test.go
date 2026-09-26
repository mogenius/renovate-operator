package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
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

// suspendServer answers as an admin: with no auth provider every request is one.
func suspendServer(setSuspend func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, suspend bool) error) *Server {
	return &Server{
		manager: &mockRenovateJobManager{
			getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
				return &api.RenovateJob{Name: name, Namespace: namespace}, nil
			},
			setSuspendFunc: setSuspend,
		},
		discovery: &mockDiscoveryAgent{},
		logger:    logr.Discard(),
	}
}

func postSuspend(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/renovatejob/suspend", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.setRenovateJobSuspend(w, req)
	return w
}

func TestSetRenovateJobSuspendSetsTheRequestedState(t *testing.T) {
	for _, suspend := range []bool{true, false} {
		var got *bool
		var gotJob crdmanager.RenovateJobIdentifier
		server := suspendServer(func(_ context.Context, jobId crdmanager.RenovateJobIdentifier, s bool) error {
			got, gotJob = &s, jobId
			return nil
		})

		body, _ := json.Marshal(map[string]any{"renovateJob": "job1", "namespace": "default", "suspend": suspend})
		w := postSuspend(t, server, string(body))

		if w.Code != http.StatusOK {
			t.Fatalf("suspend=%v: expected status %d, got %d", suspend, http.StatusOK, w.Code)
		}
		if got == nil || *got != suspend {
			t.Errorf("suspend=%v: manager got %v", suspend, got)
		}
		if gotJob.Name != "job1" || gotJob.Namespace != "default" {
			t.Errorf("suspend=%v: manager got job %+v", suspend, gotJob)
		}
	}
}

// Leaving the field out must not read as false and resume a suspended job.
func TestSetRenovateJobSuspendRequiresTheField(t *testing.T) {
	called := false
	server := suspendServer(func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ bool) error {
		called = true
		return nil
	})

	w := postSuspend(t, server, `{"renovateJob":"job1","namespace":"default"}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if called {
		t.Error("the manager must not be called without an explicit suspend value")
	}
}

func TestSetRenovateJobSuspendReportsAFailedUpdate(t *testing.T) {
	server := suspendServer(func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ bool) error {
		return errors.New("conflict")
	})

	w := postSuspend(t, server, `{"renovateJob":"job1","namespace":"default","suspend":true}`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
