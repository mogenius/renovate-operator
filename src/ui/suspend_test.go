package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// suspendServer lists jobs as an operator offering the suspend button, and
// answers as an admin, since with no auth provider every request is one.
func suspendServer(jobs []api.RenovateJob, setSuspend func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, suspend bool) error) *Server {
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

func suspendRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/renovatejob/suspend", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// postSuspend decodes the result only for the statuses that carry one.
func postSuspend(t *testing.T, server *Server, req *http.Request) (*httptest.ResponseRecorder, SuspendResult) {
	t.Helper()
	w := httptest.NewRecorder()
	server.setRenovateJobsSuspend(w, req)
	var result SuspendResult
	if w.Code == http.StatusOK || w.Code == http.StatusInternalServerError {
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	}
	return w, result
}

// recordSetSuspend returns a SetSuspend that records "namespace/name=suspend".
func recordSetSuspend(got *[]string) func(context.Context, crdmanager.RenovateJobIdentifier, bool) error {
	return func(_ context.Context, jobId crdmanager.RenovateJobIdentifier, suspend bool) error {
		*got = append(*got, fmt.Sprintf("%s/%s=%v", jobId.Namespace, jobId.Name, suspend))
		return nil
	}
}

func TestSetRenovateJobsSuspendSetsTheRequestedState(t *testing.T) {
	for _, suspend := range []bool{true, false} {
		jobs := []api.RenovateJob{{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Suspend: new(!suspend)}}}
		var got []string
		server := suspendServer(jobs, recordSetSuspend(&got))

		body, _ := json.Marshal(map[string]any{
			"renovateJobs": []map[string]string{{"name": "job1", "namespace": "default"}},
			"suspend":      suspend,
		})
		w, result := postSuspend(t, server, suspendRequest(string(body)))

		if w.Code != http.StatusOK {
			t.Fatalf("suspend=%v: expected status %d, got %d", suspend, http.StatusOK, w.Code)
		}
		if want := []string{fmt.Sprintf("default/job1=%v", suspend)}; !slices.Equal(got, want) {
			t.Errorf("suspend=%v: SetSuspend calls %v, want %v", suspend, got, want)
		}
		if !slices.Equal(result.Changed, []string{"default/job1"}) {
			t.Errorf("suspend=%v: changed = %v, want [default/job1]", suspend, result.Changed)
		}
	}
}

// Callers of the endpoint from before renovateJobs keep working.
func TestSetRenovateJobsSuspendAcceptsTheSingleJobShape(t *testing.T) {
	var got []string
	server := suspendServer([]api.RenovateJob{{Name: "job1", Namespace: "default"}}, recordSetSuspend(&got))

	w, _ := postSuspend(t, server, suspendRequest(`{"renovateJob":"job1","namespace":"default","suspend":true}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !slices.Equal(got, []string{"default/job1=true"}) {
		t.Errorf("SetSuspend calls %v, want [default/job1=true]", got)
	}
}

// Suspend all names jobs already suspended too when the dashboard is stale:
// those are reported, not rewritten.
func TestSetRenovateJobsSuspendLeavesJobsInTheRequestedStateAlone(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "active1", Namespace: "default"},
		{Name: "paused", Namespace: "default", Spec: api.RenovateJobSpec{Suspend: new(true)}},
		{Name: "active2", Namespace: "other"},
	}
	var got []string
	server := suspendServer(jobs, recordSetSuspend(&got))

	w, result := postSuspend(t, server, suspendRequest(`{"suspend":true,"renovateJobs":[
		{"name":"active1","namespace":"default"},
		{"name":"paused","namespace":"default"},
		{"name":"active2","namespace":"other"}]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if want := []string{"default/active1=true", "other/active2=true"}; !slices.Equal(got, want) {
		t.Errorf("SetSuspend calls %v, want %v", got, want)
	}
	if !slices.Equal(result.Changed, []string{"default/active1", "other/active2"}) {
		t.Errorf("changed = %v", result.Changed)
	}
	if !slices.Equal(result.Unchanged, []string{"default/paused"}) {
		t.Errorf("unchanged = %v, want [default/paused]", result.Unchanged)
	}
}

// A request naming a job the caller may not suspend changes nothing, not even
// the jobs it may suspend, and one naming a job it cannot read does not learn
// whether that job exists.
func TestSetRenovateJobsSuspendRefusesJobsTheCallerCannotSuspend(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "admin", Namespace: "default", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team"}}}},
		{Name: "reader", Namespace: "default", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team"}}}},
		{Name: "hidden", Namespace: "default", Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"someone-else"}}}},
	}
	tests := []struct {
		job        string
		wantStatus int
	}{
		{"reader", http.StatusForbidden},
		{"hidden", http.StatusNotFound},
		{"missing", http.StatusNotFound},
	}
	for _, tt := range tests {
		var got []string
		server := suspendServer(jobs, recordSetSuspend(&got))
		server.auth = &OIDCAuth{}

		req := suspendRequest(`{"suspend":true,"renovateJobs":[{"name":"admin","namespace":"default"},{"name":"` + tt.job + `","namespace":"default"}]}`)
		req = req.WithContext(context.WithValue(req.Context(), sessionContextKey, &sessionData{Email: "admin@example.com", Groups: []string{"team"}}))
		w, _ := postSuspend(t, server, req)

		if w.Code != tt.wantStatus {
			t.Errorf("%s: expected status %d, got %d", tt.job, tt.wantStatus, w.Code)
		}
		if len(got) != 0 {
			t.Errorf("%s: SetSuspend called for %v, want no change at all", tt.job, got)
		}
	}
}

// Leaving the field out must not read as false and resume a suspended job, and
// a request must name at least one job.
func TestSetRenovateJobsSuspendRequiresItsFields(t *testing.T) {
	for _, body := range []string{
		`{"renovateJobs":[{"name":"job1","namespace":"default"}]}`,
		`{"suspend":true}`,
		`{"suspend":true,"renovateJobs":[]}`,
		`{"suspend":true,"renovateJobs":[{"name":"job1"}]}`,
		`{"suspend":true,"renovateJob":"job1"}`,
	} {
		var got []string
		server := suspendServer([]api.RenovateJob{{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{Suspend: new(true)}}}, recordSetSuspend(&got))

		w, _ := postSuspend(t, server, suspendRequest(body))

		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected status %d, got %d", body, http.StatusBadRequest, w.Code)
		}
		if len(got) != 0 {
			t.Errorf("%s: SetSuspend called for %v", body, got)
		}
	}
}

// The button writes spec.suspend on the live object, which a GitOps sync can put
// back, so an install that did not opt in refuses it even to an admin.
func TestSetRenovateJobsSuspendIsOffUnlessEnabled(t *testing.T) {
	var got []string
	server := suspendServer([]api.RenovateJob{{Name: "job1", Namespace: "default"}}, recordSetSuspend(&got))
	server.accessDefaults.SuspendFromUI = false

	w, _ := postSuspend(t, server, suspendRequest(`{"suspend":true,"renovateJobs":[{"name":"job1","namespace":"default"}]}`))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
	if len(got) != 0 {
		t.Errorf("SetSuspend called for %v while the button is off", got)
	}
}

// One job failing must not stop the others, and the caller learns which failed.
func TestSetRenovateJobsSuspendReportsFailedJobs(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "broken", Namespace: "default"},
		{Name: "fine", Namespace: "default"},
	}
	server := suspendServer(jobs, func(_ context.Context, jobId crdmanager.RenovateJobIdentifier, _ bool) error {
		if jobId.Name == "broken" {
			return errors.New("conflict")
		}
		return nil
	})

	w, result := postSuspend(t, server, suspendRequest(`{"suspend":true,"renovateJobs":[
		{"name":"broken","namespace":"default"},{"name":"fine","namespace":"default"}]}`))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
	if !slices.Equal(result.Changed, []string{"default/fine"}) || !slices.Equal(result.Failed, []string{"default/broken"}) {
		t.Errorf("changed = %v, failed = %v, want [default/fine] and [default/broken]", result.Changed, result.Failed)
	}
}

// Suspend all is pressed while the Git platform struggles, and the browser that
// pressed it may die right after: the batch must not stop with the request.
func TestSetRenovateJobsSuspendOutlivesItsRequest(t *testing.T) {
	jobs := []api.RenovateJob{
		{Name: "job1", Namespace: "default"},
		{Name: "job2", Namespace: "default"},
	}
	var got []string
	server := suspendServer(jobs, func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier, _ bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		got = append(got, jobId.Name)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := suspendRequest(`{"suspend":true,"renovateJobs":[{"name":"job1","namespace":"default"},{"name":"job2","namespace":"default"}]}`)
	w, _ := postSuspend(t, server, req.WithContext(ctx))

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !slices.Equal(got, []string{"job1", "job2"}) {
		t.Errorf("SetSuspend completed for %v, want both jobs", got)
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
