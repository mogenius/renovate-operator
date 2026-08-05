package ui

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "renovate-operator/api/v1alpha1"
)

func TestResolveAccess(t *testing.T) {
	tests := []struct {
		name            string
		job             *api.RenovateJob
		session         *sessionData
		defaults        AccessDefaults
		wantRole        accessRole
		wantPermissions []string
	}{
		{
			name:            "no access configuration hides the job",
			job:             &api.RenovateJob{},
			session:         &sessionData{Groups: []string{"team-a"}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "admin group grants every permission",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-admin"}}}},
			session:         &sessionData{Groups: []string{"team-admin"}},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "reader group grants logs only",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-reader"}}}},
			session:         &sessionData{Groups: []string{"team-reader"}},
			wantRole:        roleReader,
			wantPermissions: []string{permLogs},
		},
		{
			name:            "anonymous read grants no log access by default",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: ptr(true)}}},
			wantRole:        roleReader,
			wantPermissions: []string{},
		},
		{
			name:            "anonymous read logs opt-in grants log access",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: ptr(true), AnonymousReadLogs: ptr(true)}}},
			wantRole:        roleReader,
			wantPermissions: []string{permLogs},
		},
		{
			name:            "anonymous read logs alone grants nothing",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousReadLogs: ptr(true)}}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "operator defaults fill in unset job fields",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-reader"}}}},
			session:         &sessionData{Groups: []string{"team-default-admin"}},
			defaults:        AccessDefaults{AdminGroups: []string{"team-default-admin"}},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "job group list overrides the matching default",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-job-admin"}}}},
			session:         &sessionData{Groups: []string{"team-default-admin"}},
			defaults:        AccessDefaults{AdminGroups: []string{"team-default-admin"}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "deprecated allowedGroups is an alias for adminGroups",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-legacy"}}}, //nolint:staticcheck // deprecated field is intentionally still honoured
			session:         &sessionData{Groups: []string{"team-legacy"}},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name: "deprecated allowedGroups next to access fails closed",
			job: &api.RenovateJob{Spec: api.RenovateJobSpec{
				AllowedGroups: []string{"team-legacy"}, //nolint:staticcheck // deprecated field is intentionally still honoured
				Access:        &api.RenovateJobAccess{AdminGroups: []string{"team-admin"}},
			}},
			session:         &sessionData{Groups: []string{"team-legacy", "team-admin"}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "nil job is never accessible",
			job:             nil,
			session:         &sessionData{Groups: []string{"team-admin"}},
			defaults:        AccessDefaults{AnonymousRead: true},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := resolveAccess(tt.job, tt.session, tt.defaults, logr.Discard())

			if decision.Role != tt.wantRole {
				t.Errorf("role = %q, want %q", decision.Role.String(), tt.wantRole.String())
			}
			if got := decision.permissions(); !slices.Equal(got, tt.wantPermissions) {
				t.Errorf("permissions = %v, want %v", got, tt.wantPermissions)
			}
		})
	}
}

// TestWriteRoutesRequireAdmin walks the registered API routes so a new mutating
// endpoint cannot be added without deciding which permission guards it: the
// count assertion fails until the new route is covered here.
func TestWriteRoutesRequireAdmin(t *testing.T) {
	job := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
		Spec:       api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-reader"}}},
	}

	server := &Server{
		manager: &mockRenovateJobManager{
			getRenovateJobFunc: func(_ context.Context, _, _ string) (*api.RenovateJob, error) {
				return job, nil
			},
		},
		logger:    logr.Discard(),
		discovery: &mockDiscoveryAgent{},
		scheduler: &mockScheduler{},
		auth:      &OIDCAuth{},
		Router:    mux.NewRouter(),
	}
	server.registerApiV1Routes(server.Router)

	var postPaths []string
	err := server.Router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		methods, methodsErr := route.GetMethods()
		if methodsErr != nil || !slices.Contains(methods, http.MethodPost) {
			return nil
		}
		path, pathErr := route.GetPathTemplate()
		if pathErr != nil {
			return pathErr
		}
		postPaths = append(postPaths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk routes: %v", err)
	}

	wantPaths := []string{
		"/api/v1/renovate",
		"/api/v1/renovate/all",
		"/api/v1/renovate/cancel",
		"/api/v1/discovery/start",
	}
	slices.Sort(postPaths)
	slices.Sort(wantPaths)
	if !slices.Equal(postPaths, wantPaths) {
		t.Fatalf("mutating routes = %v, want %v -- a new write route needs a permission and a case here", postPaths, wantPaths)
	}

	body := `{"renovateJob":"job1","namespace":"default","project":"proj"}`
	for _, path := range postPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), sessionContextKey, &sessionData{
				Email:  "reader@example.com",
				Groups: []string{"team-reader"},
			}))

			w := httptest.NewRecorder()
			server.Router.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("reader got status %d for %s, want %d", w.Code, path, http.StatusForbidden)
			}
		})
	}
}

func TestIsAnonymousReadPath(t *testing.T) {
	anonymous := []string{"/", "/index.html", "/logs", "/api/v1/version", "/api/v1/renovatejobs", "/api/v1/logs", "/api/v1/discovery/status"}
	for _, path := range anonymous {
		if !isAnonymousReadPath(path) {
			t.Errorf("expected %q to be reachable without a session", path)
		}
	}

	guarded := []string{"/api/v1/renovate", "/api/v1/renovate/all", "/api/v1/renovate/cancel", "/api/v1/discovery/start", "/api/v1/unknown"}
	for _, path := range guarded {
		if isAnonymousReadPath(path) {
			t.Errorf("expected %q to require a session", path)
		}
	}
}
