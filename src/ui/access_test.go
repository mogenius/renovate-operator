package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"

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
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true)}}},
			wantRole:        roleReader,
			wantPermissions: []string{},
		},
		{
			name:            "anonymous read logs opt-in grants log access",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true), AnonymousReadLogs: new(true)}}},
			wantRole:        roleReader,
			wantPermissions: []string{permLogs},
		},
		{
			name:            "anonymous read logs alone grants nothing",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousReadLogs: new(true)}}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "authorization disabled makes any session an admin",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-admin"}}}},
			session:         &sessionData{Email: "nobody@example.com", Groups: []string{"team-unrelated"}},
			defaults:        AccessDefaults{AuthorizationDisabled: true},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "authorization disabled grants a session admin on an unconfigured job",
			job:             &api.RenovateJob{},
			session:         &sessionData{Email: "nobody@example.com"},
			defaults:        AccessDefaults{AuthorizationDisabled: true},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "authorization disabled still denies requests without a session",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-admin"}}}},
			defaults:        AccessDefaults{AuthorizationDisabled: true},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "authorization disabled still honours anonymous read",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true), AnonymousReadLogs: new(true)}}},
			defaults:        AccessDefaults{AuthorizationDisabled: true},
			wantRole:        roleReader,
			wantPermissions: []string{permLogs},
		},
		{
			name: "authorization disabled overrides a conflicting access configuration for a session",
			job: &api.RenovateJob{Spec: api.RenovateJobSpec{
				AllowedGroups: []string{"team-legacy"}, //nolint:staticcheck // deprecated field is intentionally still honoured
				Access:        &api.RenovateJobAccess{AdminGroups: []string{"team-admin"}},
			}},
			session:         &sessionData{Email: "nobody@example.com"},
			defaults:        AccessDefaults{AuthorizationDisabled: true},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "admin user matched by email",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"me@example.com"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			// The homelab case: a personal GitHub account is in no org, so it has
			// no group to match, and its email may be private.
			name:            "admin user matched by username when the email differs",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"octocat"}}}},
			session:         &sessionData{Email: "octocat@github", Username: "octocat", EmailVerified: true},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "user match is case-insensitive",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"Me@Example.COM"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "reader user grants logs only",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderUsers: []string{"me@example.com"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true},
			wantRole:        roleReader,
			wantPermissions: []string{permLogs},
		},
		{
			name:            "unlisted user gets nothing",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"me@example.com"}}}},
			session:         &sessionData{Email: "someone@example.com", Username: "someone", EmailVerified: true},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			// An IdP that lets an account set an arbitrary address would
			// otherwise let it claim any adminUsers entry.
			name:            "unverified email does not match a user rule",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"victim@example.com"}}}},
			session:         &sessionData{Email: "victim@example.com", EmailVerified: false},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "unverified email still allows a username match",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"octocat"}}}},
			session:         &sessionData{Email: "spoofed@example.com", Username: "octocat", EmailVerified: false},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			// An empty identity must never match an empty configured entry.
			name:            "blank session identity matches nothing",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{""}}}},
			session:         &sessionData{},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "user rule works without any group support",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"me@example.com"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true, Groups: nil},
			defaults:        AccessDefaults{AdminUsers: []string{"other@example.com"}},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "default admin users apply when the job sets none",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-reader"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true},
			defaults:        AccessDefaults{AdminUsers: []string{"me@example.com"}},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
		},
		{
			name:            "admin user outranks a reader group match",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"me@example.com"}, ReaderGroups: []string{"team-reader"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true, Groups: []string{"team-reader"}},
			wantRole:        roleAdmin,
			wantPermissions: []string{permLogs, permTrigger, permTriggerAll, permCancel, permDiscovery},
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
			// Inheritance is per field and REPLACES, it does not merge: a job that
			// sets a list drops the operator-wide one for that field. Pinned here
			// because "a job can only add to the defaults" would be a dangerous
			// thing to believe while writing a per-job rule.
			name:            "job group list overrides the matching default",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-job-admin"}}}},
			session:         &sessionData{Groups: []string{"team-default-admin"}},
			defaults:        AccessDefaults{AdminGroups: []string{"team-default-admin"}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "job user list overrides the matching default",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminUsers: []string{"someone-else@example.com"}}}},
			session:         &sessionData{Email: "me@example.com", EmailVerified: true},
			defaults:        AccessDefaults{AdminUsers: []string{"me@example.com"}},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			// The three-state pointer: an explicit false revokes an enabled default.
			name:            "job anonymousRead false opts out of an enabled default",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(false)}}},
			defaults:        AccessDefaults{AnonymousRead: true},
			wantRole:        roleNone,
			wantPermissions: []string{},
		},
		{
			name:            "job anonymousReadLogs false opts out of an enabled default",
			job:             &api.RenovateJob{Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousReadLogs: new(false)}}},
			defaults:        AccessDefaults{AnonymousRead: true, AnonymousReadLogs: true},
			wantRole:        roleReader,
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
		Name: "job1", Namespace: "default",
		Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{ReaderGroups: []string{"team-reader"}}},
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

// groupsJob is a job restricting access by group, the configuration that cannot
// be enforced without a group-capable auth provider.
func groupsJob(name string) api.RenovateJob {
	return api.RenovateJob{
		Name: name, Namespace: "default",
		Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AdminGroups: []string{"team-admin"}}},
	}
}

func TestDetectAccessMisconfiguration(t *testing.T) {
	groupless := &GitHubOAuth{}                 // SupportsGroups() == false
	withGroups := &GitHubOAuth{orgGroups: true} // SupportsGroups() == true

	tests := []struct {
		name             string
		provider         AuthProvider
		defaults         AccessDefaults
		jobs             []api.RenovateJob
		wantReason       string
		wantAffectedJobs []string
	}{
		{
			name:     "no provider ignores access rules entirely",
			provider: nil,
			defaults: AccessDefaults{AdminGroups: []string{"team-admin"}},
			jobs:     []api.RenovateJob{groupsJob("job1")},
		},
		{
			name:     "group-capable provider is enforceable",
			provider: withGroups,
			defaults: AccessDefaults{AdminGroups: []string{"team-admin"}},
			jobs:     []api.RenovateJob{groupsJob("job1")},
		},
		{
			name:     "groupless provider without group rules is enforceable",
			provider: groupless,
			defaults: AccessDefaults{AnonymousRead: true},
			jobs:     []api.RenovateJob{{Name: "job1"}},
		},
		{
			name:       "groupless provider with default groups is not enforceable",
			provider:   groupless,
			defaults:   AccessDefaults{ReaderGroups: []string{"team-reader"}},
			wantReason: ReasonGroupsUnsupported,
		},
		{
			name:             "groupless provider with per-job groups is not enforceable",
			provider:         groupless,
			jobs:             []api.RenovateJob{{Name: "plain"}, groupsJob("job1")},
			wantReason:       ReasonGroupsUnsupported,
			wantAffectedJobs: []string{"default/job1"},
		},
		{
			// The group rules are never evaluated, so they cannot hide anything and
			// there is nothing to warn about.
			name:     "groupless provider with authorization disabled is enforceable",
			provider: groupless,
			defaults: AccessDefaults{ReaderGroups: []string{"team-reader"}, AuthorizationDisabled: true},
			jobs:     []api.RenovateJob{groupsJob("job1")},
		},
		{
			name:     "groupless provider with deprecated allowedGroups is not enforceable",
			provider: groupless,
			jobs: []api.RenovateJob{{
				Name: "legacy", Namespace: "default",
				Spec: api.RenovateJobSpec{AllowedGroups: []string{"team-legacy"}}, //nolint:staticcheck // deprecated field is intentionally still honoured
			}},
			wantReason:       ReasonGroupsUnsupported,
			wantAffectedJobs: []string{"default/legacy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, affected := detectAccessMisconfiguration(tt.provider, tt.defaults, tt.jobs)

			if tt.wantReason == "" {
				if got != nil {
					t.Fatalf("misconfiguration = %+v, want none", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("misconfiguration = none, want reason %q", tt.wantReason)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Message == "" {
				t.Error("message is empty, the UI has nothing to show")
			}
			if !slices.Equal(affected, tt.wantAffectedJobs) {
				t.Errorf("affected jobs = %v, want %v", affected, tt.wantAffectedJobs)
			}
		})
	}
}

// TestUnenforceableAccessHidesEverything pins the fail-closed behaviour: while
// the rules cannot be enforced, no job is listed and no per-job endpoint answers,
// including for a job whose own rules would otherwise grant access.
func TestUnenforceableAccessHidesEverything(t *testing.T) {
	job := groupsJob("job1")
	anonymous := api.RenovateJob{
		Name: "public", Namespace: "default",
		Spec: api.RenovateJobSpec{Access: &api.RenovateJobAccess{AnonymousRead: new(true)}},
	}

	server := &Server{
		manager: &mockRenovateJobManager{
			listRenovateJobsFullFunc: func(_ context.Context) ([]api.RenovateJob, error) {
				return []api.RenovateJob{job, anonymous}, nil
			},
			getRenovateJobFunc: func(_ context.Context, name, _ string) (*api.RenovateJob, error) {
				return &anonymous, nil
			},
		},
		logger:    logr.Discard(),
		discovery: &mockDiscoveryAgent{},
		scheduler: &mockScheduler{},
		auth:      &GitHubOAuth{},
		Router:    mux.NewRouter(),
	}
	server.registerApiV1Routes(server.Router)

	t.Run("job list is empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/renovatejobs", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var jobs []RenovateJobInfo
		if err := json.Unmarshal(w.Body.Bytes(), &jobs); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(jobs) != 0 {
			t.Errorf("jobs = %v, want none while access rules are unenforceable", jobs)
		}
	})

	// Anonymous read on the job would normally allow this, which is the point:
	// an unenforceable rule set suppresses it too rather than guessing.
	t.Run("per-job read is a 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/discovery/status?namespace=default&renovate=public", nil))

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("access status reports the reason", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/access/status", nil))

		var status struct {
			Misconfigured bool   `json:"misconfigured"`
			Reason        string `json:"reason"`
			Message       string `json:"message"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !status.Misconfigured {
			t.Error("misconfigured = false, want true")
		}
		if status.Reason != ReasonGroupsUnsupported {
			t.Errorf("reason = %q, want %q", status.Reason, ReasonGroupsUnsupported)
		}
		if status.Message == "" {
			t.Error("message is empty, the banner has nothing to show")
		}
	})
}

// TestAccessCheckCacheAvoidsRepeatedListing pins the reason the cache exists:
// the per-job endpoints are polled by the dashboard and reachable without a
// session, so each request must not list every RenovateJob.
func TestAccessCheckCacheAvoidsRepeatedListing(t *testing.T) {
	listCalls := 0
	server := &Server{
		manager: &mockRenovateJobManager{
			listRenovateJobsFullFunc: func(_ context.Context) ([]api.RenovateJob, error) {
				listCalls++
				return []api.RenovateJob{groupsJob("job1")}, nil
			},
			getRenovateJobFunc: func(_ context.Context, _, _ string) (*api.RenovateJob, error) {
				job := groupsJob("job1")
				return &job, nil
			},
		},
		logger:    logr.Discard(),
		discovery: &mockDiscoveryAgent{},
		scheduler: &mockScheduler{},
		auth:      &GitHubOAuth{}, // SupportsGroups() == false
		Router:    mux.NewRouter(),
	}
	server.registerApiV1Routes(server.Router)

	for range 5 {
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/discovery/status?namespace=default&renovate=job1", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d while access rules are unenforceable", w.Code, http.StatusNotFound)
		}
	}

	if listCalls != 1 {
		t.Errorf("listed renovatejobs %d times across 5 requests, want 1 (verdict should be cached)", listCalls)
	}
}

// TestAccessCheckCacheLogsOnlyTransitions keeps the log usable for a polling
// dashboard: the verdict is re-checked constantly but only changes rarely.
func TestAccessCheckCacheLogsOnlyTransitions(t *testing.T) {
	var cache accessCheckCache
	broken := &AccessMisconfiguration{Reason: ReasonGroupsUnsupported}

	if cache.store(nil) {
		t.Error("first enforceable verdict reported a transition, want none")
	}
	if !cache.store(broken) {
		t.Error("becoming unenforceable did not report a transition")
	}
	if cache.store(broken) {
		t.Error("repeating the same verdict reported a transition")
	}
	if !cache.store(nil) {
		t.Error("recovering did not report a transition")
	}
}

func TestAccessCheckCacheExpiry(t *testing.T) {
	var cache accessCheckCache

	if _, ok := cache.load(); ok {
		t.Error("empty cache reported a usable verdict")
	}

	cache.store(&AccessMisconfiguration{Reason: ReasonGroupsUnsupported})
	verdict, ok := cache.load()
	if !ok || verdict == nil {
		t.Fatal("freshly stored verdict was not returned")
	}

	// Backdate past the TTL rather than sleeping through it.
	cache.mu.Lock()
	cache.computed = time.Now().Add(-accessCheckTTL - time.Second)
	cache.mu.Unlock()

	if _, ok := cache.load(); ok {
		t.Error("expired verdict was reported as usable")
	}
}

// TestAccessStatusCleanWhenEnforceable is the other half: a group-capable
// provider must not raise the banner.
func TestAccessStatusCleanWhenEnforceable(t *testing.T) {
	server := &Server{
		manager:        &mockRenovateJobManager{},
		logger:         logr.Discard(),
		discovery:      &mockDiscoveryAgent{},
		scheduler:      &mockScheduler{},
		auth:           &OIDCAuth{},
		accessDefaults: AccessDefaults{AdminGroups: []string{"team-admin"}},
		Router:         mux.NewRouter(),
	}
	server.registerApiV1Routes(server.Router)

	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/access/status", nil))

	var status struct {
		Misconfigured bool `json:"misconfigured"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.Misconfigured {
		t.Error("misconfigured = true, want false for a group-capable provider")
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
