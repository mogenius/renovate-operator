package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
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

// suspendServer answers as an admin, since with no auth provider every request is
// one, on an install that offers the suspend button.
func suspendServer(setSuspend func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, suspend bool) error) *Server {
	return &Server{
		manager: &mockRenovateJobManager{
			getRenovateJobFunc: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
				return &api.RenovateJob{Name: name, Namespace: namespace}, nil
			},
			setSuspendFunc: setSuspend,
		},
		discovery:      &mockDiscoveryAgent{},
		logger:         logr.Discard(),
		accessDefaults: AccessDefaults{SuspendFromUI: true},
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

// The button writes spec.suspend on the live object, which a GitOps sync can put
// back, so an install that did not opt in refuses it even to an admin.
func TestSetRenovateJobSuspendIsOffUnlessEnabled(t *testing.T) {
	called := false
	server := suspendServer(func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ bool) error {
		called = true
		return nil
	})
	server.accessDefaults.SuspendFromUI = false

	w := postSuspend(t, server, `{"renovateJob":"job1","namespace":"default","suspend":true}`)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
	if called {
		t.Error("the manager must not be called while the button is off")
	}
}

func TestGetRenovateJobsOffersSuspendOnlyWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		server := &Server{
			manager: &mockRenovateJobManager{
				listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
					return []api.RenovateJob{{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Schedule: "0 * * * *"}}}, nil
				},
			},
			logger:         logr.Discard(),
			discovery:      &mockDiscoveryAgent{},
			scheduler:      &mockScheduler{},
			accessDefaults: AccessDefaults{SuspendFromUI: enabled},
		}

		w := httptest.NewRecorder()
		server.getRenovateJobs(w, httptest.NewRequest(http.MethodGet, "/api/v1/renovatejobs", nil))

		var result []RenovateJobInfo
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil || len(result) != 1 {
			t.Fatalf("enabled=%v: failed to decode one job: %v", enabled, err)
		}
		if got := slices.Contains(result[0].Permissions, permSuspend); got != enabled {
			t.Errorf("enabled=%v: an admin holds the suspend permission: %v", enabled, got)
		}
		if result[0].SuspendFromUI != enabled {
			t.Errorf("enabled=%v: suspendFromUI = %v", enabled, result[0].SuspendFromUI)
		}
	}
}

// suspendAllServer lists jobs as an operator offering the suspend button, and
// records the jobs SetSuspend was called for.
func suspendAllServer(jobs []api.RenovateJob, setSuspend func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, suspend bool) error) *Server {
	return &Server{
		manager: &mockRenovateJobManager{
			listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
				return jobs, nil
			},
			setSuspendFunc: setSuspend,
		},
		logger:         logr.Discard(),
		accessDefaults: AccessDefaults{SuspendFromUI: true},
	}
}

func postSuspendAll(t *testing.T, server *Server, req *http.Request) (*httptest.ResponseRecorder, SuspendAllResult) {
	t.Helper()
	w := httptest.NewRecorder()
	server.setAllRenovateJobsSuspend(w, req)
	var result SuspendAllResult
	if w.Code != http.StatusForbidden && w.Code != http.StatusBadRequest {
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	}
	return w, result
}

func suspendAllRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/renovatejobs/suspend", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestSetAllRenovateJobsSuspendLeavesSuspendedJobsAlone(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "active1", Namespace: "default"},
		{Name: "paused", Namespace: "default", Spec: api.RenovateJobSpec{Suspend: new(true)}},
		{Name: "active2", Namespace: "other"},
	}
	var got []string
	server := suspendAllServer(jobs, func(_ context.Context, jobId crdmanager.RenovateJobIdentifier, suspend bool) error {
		if !suspend {
			t.Errorf("%s/%s: SetSuspend(false), want true", jobId.Namespace, jobId.Name)
		}
		got = append(got, jobId.Namespace+"/"+jobId.Name)
		return nil
	})

	w, result := postSuspendAll(t, server, suspendAllRequest(`{"suspend":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	want := []string{"default/active1", "other/active2"}
	if !slices.Equal(got, want) {
		t.Errorf("SetSuspend called for %v, want %v", got, want)
	}
	if !slices.Equal(result.Changed, want) {
		t.Errorf("changed = %v, want %v", result.Changed, want)
	}
	if !slices.Equal(result.Unchanged, []string{"default/paused"}) {
		t.Errorf("unchanged = %v, want [default/paused]", result.Unchanged)
	}
}

// An admin of some jobs suspends those, and neither touches nor names the jobs
// it only reads or cannot see.
func TestSetAllRenovateJobsSuspendSkipsJobsTheCallerCannotSuspend(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "admin", Namespace: "default", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team"}}}},
		{Name: "reader", Namespace: "default", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team"}}}},
		{Name: "hidden", Namespace: "default", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"someone-else"}}}},
	}
	var got []string
	server := suspendAllServer(jobs, func(_ context.Context, jobId crdmanager.RenovateJobIdentifier, _ bool) error {
		got = append(got, jobId.Name)
		return nil
	})
	server.auth = &OIDCAuth{}

	req := suspendAllRequest(`{"suspend":true}`)
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey, &sessionData{Email: "admin@example.com", Groups: []string{"team"}}))
	w, result := postSuspendAll(t, server, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !slices.Equal(got, []string{"admin"}) {
		t.Errorf("SetSuspend called for %v, want [admin]", got)
	}
	if len(result.Unchanged) != 0 || len(result.Failed) != 0 {
		t.Errorf("the response names jobs the caller cannot suspend: %+v", result)
	}
}

func TestSetAllRenovateJobsSuspendRequiresTheField(t *testing.T) {
	called := false
	server := suspendAllServer([]api.RenovateJob{{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Suspend: new(true)}}},
		func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ bool) error {
			called = true
			return nil
		})

	w, _ := postSuspendAll(t, server, suspendAllRequest(`{}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if called {
		t.Error("the manager must not be called without an explicit suspend value")
	}
}

func TestSetAllRenovateJobsSuspendIsOffUnlessEnabled(t *testing.T) {
	called := false
	server := suspendAllServer([]api.RenovateJob{{Name: "job1", Namespace: "default"}},
		func(_ context.Context, _ crdmanager.RenovateJobIdentifier, _ bool) error {
			called = true
			return nil
		})
	server.accessDefaults.SuspendFromUI = false

	w, _ := postSuspendAll(t, server, suspendAllRequest(`{"suspend":true}`))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
	if called {
		t.Error("the manager must not be called while the button is off")
	}
}

// One job failing must not stop the others, and the caller learns which failed.
func TestSetAllRenovateJobsSuspendReportsFailedJobs(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "broken", Namespace: "default"},
		{Name: "fine", Namespace: "default"},
	}
	server := suspendAllServer(jobs, func(_ context.Context, jobId crdmanager.RenovateJobIdentifier, _ bool) error {
		if jobId.Name == "broken" {
			return errors.New("conflict")
		}
		return nil
	})

	w, result := postSuspendAll(t, server, suspendAllRequest(`{"suspend":true}`))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
	if !slices.Equal(result.Changed, []string{"default/fine"}) || !slices.Equal(result.Failed, []string{"default/broken"}) {
		t.Errorf("changed = %v, failed = %v, want [default/fine] and [default/broken]", result.Changed, result.Failed)
	}
}

// The button is pressed while the Git platform struggles, and the browser that
// pressed it may die right after: the batch must not stop with the request.
func TestSetAllRenovateJobsSuspendOutlivesItsRequest(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "job1", Namespace: "default"},
		{Name: "job2", Namespace: "default"},
	}
	var got []string
	server := suspendAllServer(jobs, func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, _ bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		got = append(got, jobId.Name)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w, _ := postSuspendAll(t, server, suspendAllRequest(`{"suspend":true}`).WithContext(ctx))

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !slices.Equal(got, []string{"job1", "job2"}) {
		t.Errorf("SetSuspend completed for %v, want both jobs", got)
	}
}
