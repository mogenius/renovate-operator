package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
)

func secretReaderWith(t *testing.T, objects ...client.Object) client.Reader {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func stepByID(t *testing.T, status SetupStatus, id string) SetupStep {
	t.Helper()
	for _, step := range status.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("step %q not found in %+v", id, status.Steps)
	return SetupStep{}
}

func TestGetSetupStatus_HiddenWhenAuthConfigured(t *testing.T) {
	// The guide is a first-run aid and disappears entirely once an auth
	// provider is configured — even for a session matching the operator-wide
	// admin defaults.
	server := &Server{
		manager:        &mockRenovateJobManager{},
		logger:         logr.Discard(),
		auth:           &OIDCAuth{},
		accessDefaults: AccessDefaults{AdminUsers: []string{"admin@example.com"}},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey,
		&sessionData{Email: "admin@example.com", EmailVerified: true}))
	w := httptest.NewRecorder()
	server.getSetupStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
	var status SetupStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if status.Visible {
		t.Error("setup status must not be visible to non-admins")
	}
	if len(status.Steps) != 0 || len(status.Hints) != 0 {
		t.Errorf("hidden setup status must carry no steps or hints, got %+v", status)
	}
}

func TestComputeSetupStatus_NoJobs(t *testing.T) {
	server := &Server{
		manager: &mockRenovateJobManager{},
		logger:  logr.Discard(),
	}

	status := server.computeSetupStatus(context.Background())

	if !status.Visible {
		t.Fatal("expected visible status")
	}
	if status.Complete {
		t.Error("empty install must not be complete")
	}
	for _, id := range []string{"credentials", "renovatejob", "accepted", "discovery"} {
		if step := stepByID(t, status, id); step.State != setupStatePending {
			t.Errorf("step %q = %q, want pending", id, step.State)
		}
	}
}

func TestComputeSetupStatus_CredentialChecks(t *testing.T) {
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "renovate-secret", Namespace: "default"},
		Data:       map[string][]byte{"GITHUB_COM_TOKEN": []byte("token")},
	}
	keylessSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "keyless", Namespace: "default"},
		Data:       map[string][]byte{"SOMETHING_ELSE": []byte("x")},
	}
	appSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "gh-app", Namespace: "default"},
		Data: map[string][]byte{
			"APP_ID":     []byte("1"),
			"INSTALL_ID": []byte("2"),
			"PEM":        []byte("key"),
		},
	}

	jobWithSecret := func(secretRef string) api.RenovateJob {
		return api.RenovateJob{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{SecretRef: secretRef}}
	}

	tests := []struct {
		name       string
		job        api.RenovateJob
		wantState  string
		wantDetail string
	}{
		{
			name:      "secret with well-known token key verifies",
			job:       jobWithSecret("renovate-secret"),
			wantState: setupStateDone,
		},
		{
			name:       "missing secret is blocked",
			job:        jobWithSecret("does-not-exist"),
			wantState:  setupStateBlocked,
			wantDetail: `secret "does-not-exist" not found`,
		},
		{
			name:       "secret without a token key is blocked",
			job:        jobWithSecret("keyless"),
			wantState:  setupStateBlocked,
			wantDetail: "holds no platform token",
		},
		{
			name:       "job without any credential reference is blocked",
			job:        api.RenovateJob{Name: "job1", Namespace: "default"},
			wantState:  setupStateBlocked,
			wantDetail: "references no credentials",
		},
		{
			name: "github app reference with all keys verifies",
			job: api.RenovateJob{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{
				GithubAppReference: &api.GithubAppReference{
					SecretName: "gh-app", AppIdSecretKey: "APP_ID", InstallationIdSecretKey: "INSTALL_ID", PemSecretKey: "PEM",
				},
			}},
			wantState: setupStateDone,
		},
		{
			name: "github app reference with a missing key is blocked",
			job: api.RenovateJob{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{
				GithubAppReference: &api.GithubAppReference{
					SecretName: "gh-app", AppIdSecretKey: "APP_ID", InstallationIdSecretKey: "INSTALL_ID", PemSecretKey: "WRONG_KEY",
				},
			}},
			wantState:  setupStateBlocked,
			wantDetail: "missing key(s): WRONG_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := []api.RenovateJob{tt.job}
			server := &Server{
				manager:   &mockRenovateJobManager{listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) { return jobs, nil }},
				discovery: &mockDiscoveryAgent{},
				logger:    logr.Discard(),
				setup:     SetupEnvironment{SecretReader: secretReaderWith(t, tokenSecret, keylessSecret, appSecret)},
			}

			status := server.computeSetupStatus(context.Background())
			step := stepByID(t, status, "credentials")
			if step.State != tt.wantState {
				t.Errorf("credentials state = %q (detail %q), want %q", step.State, step.Detail, tt.wantState)
			}
			if tt.wantDetail != "" && !strings.Contains(step.Detail, tt.wantDetail) {
				t.Errorf("credentials detail = %q, want it to contain %q", step.Detail, tt.wantDetail)
			}
		})
	}
}

func TestComputeSetupStatus_ProgressAndCompletion(t *testing.T) {
	acceptedFalse := api.RenovateJob{
		Name: "job1", Namespace: "default",
		Spec: api.RenovateJobSpec{SecretRef: "renovate-secret"},
		Status: api.RenovateJobStatus{Conditions: []metav1.Condition{{
			Type: api.ConditionAccepted, Status: metav1.ConditionFalse, Reason: "HostNotAllowed", Message: "host evil.example is not allowed",
		}}},
	}
	acceptedJob := api.RenovateJob{
		Name: "job1", Namespace: "default",
		Spec: api.RenovateJobSpec{SecretRef: "renovate-secret"},
	}

	project := func(status api.RenovateProjectStatus) crdmanager.RenovateProjectStatus {
		return crdmanager.RenovateProjectStatus{
			Name: "org/repo", Namespace: "default",
			RenovateProjectState: api.RenovateProjectState{Status: status},
		}
	}

	tests := []struct {
		name            string
		job             api.RenovateJob
		projects        []crdmanager.RenovateProjectStatus
		discoveryStatus api.RenovateProjectStatus
		wantAccepted    string
		wantDiscovery   string
		wantComplete    bool
	}{
		{
			name:          "policy refusal blocks the accepted step",
			job:           acceptedFalse,
			wantAccepted:  setupStateBlocked,
			wantDiscovery: setupStatePending,
		},
		{
			name:            "running discovery is reported while no project exists",
			job:             acceptedJob,
			discoveryStatus: api.JobStatusRunning,
			wantAccepted:    setupStateDone,
			wantDiscovery:   setupStatePending,
		},
		{
			// The runs themselves are the dashboard's business: a discovered
			// project completes the setup regardless of its run status.
			name:          "a discovered project completes the setup",
			job:           acceptedJob,
			projects:      []crdmanager.RenovateProjectStatus{project(api.JobStatusScheduled)},
			wantAccepted:  setupStateDone,
			wantDiscovery: setupStateDone,
			wantComplete:  true,
		},
	}

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "renovate-secret", Namespace: "default"},
		Data:       map[string][]byte{"RENOVATE_TOKEN": []byte("token")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := []api.RenovateJob{tt.job}
			server := &Server{
				manager: &mockRenovateJobManager{
					listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) { return jobs, nil },
					getProjectsForRenovateJobFunc: func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error) {
						return tt.projects, nil
					},
				},
				discovery: &mockDiscoveryAgent{
					getDiscoveryJobStatusFunc: func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
						if tt.discoveryStatus != "" {
							return tt.discoveryStatus, nil
						}
						return api.JobStatusScheduled, nil
					},
				},
				logger: logr.Discard(),
				setup:  SetupEnvironment{SecretReader: secretReaderWith(t, tokenSecret)},
			}

			status := server.computeSetupStatus(context.Background())

			if got := stepByID(t, status, "renovatejob").State; got != setupStateDone {
				t.Errorf("renovatejob state = %q, want done", got)
			}
			if got := stepByID(t, status, "accepted").State; got != tt.wantAccepted {
				t.Errorf("accepted state = %q, want %q", got, tt.wantAccepted)
			}
			if got := stepByID(t, status, "discovery").State; got != tt.wantDiscovery {
				t.Errorf("discovery state = %q, want %q", got, tt.wantDiscovery)
			}
			if status.Complete != tt.wantComplete {
				t.Errorf("complete = %v, want %v", status.Complete, tt.wantComplete)
			}
		})
	}
}

func TestComputeSetupStatus_Hints(t *testing.T) {
	server := &Server{
		manager: &mockRenovateJobManager{},
		logger:  logr.Discard(),
		auth:    &OIDCAuth{},
		setup:   SetupEnvironment{PolicyEnabled: true, LogStorageConfigured: false},
	}

	status := server.computeSetupStatus(context.Background())

	want := map[string]bool{"auth": true, "policy": true, "logStorage": false}
	if len(status.Hints) != len(want) {
		t.Fatalf("expected %d hints, got %+v", len(want), status.Hints)
	}
	for _, hint := range status.Hints {
		expected, ok := want[hint.ID]
		if !ok {
			t.Errorf("unexpected hint %q", hint.ID)
			continue
		}
		if hint.Done != expected {
			t.Errorf("hint %q done = %v, want %v", hint.ID, hint.Done, expected)
		}
	}
}

func TestGetSetupSecretCheck(t *testing.T) {
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "renovate-secret", Namespace: "default"},
		Data:       map[string][]byte{"GITHUB_COM_TOKEN": []byte("token")},
	}
	appSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gh-app", Namespace: "default",
			Labels: map[string]string{api.LabelAllowRef: "true"},
		},
		Data: map[string][]byte{
			"APP_ID":     []byte("1"),
			"INSTALL_ID": []byte("2"),
			"PEM":        []byte("key"),
		},
	}

	server := &Server{
		logger:    logr.Discard(),
		manager:   &mockRenovateJobManager{},
		discovery: &mockDiscoveryAgent{},
		setup: SetupEnvironment{
			OwnNamespace: "default",
			SecretReader: secretReaderWith(t, tokenSecret, appSecret),
		},
	}

	get := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/secret"+query, nil)
		w := httptest.NewRecorder()
		server.getSetupSecretCheck(w, req)
		return w
	}

	type checkResult struct {
		Found            bool `json:"found"`
		HasToken         bool `json:"hasToken"`
		HasGithubAppKeys bool `json:"hasGithubAppKeys"`
		HasAllowRefLabel bool `json:"hasAllowRefLabel"`
	}
	decode := func(t *testing.T, w *httptest.ResponseRecorder) checkResult {
		t.Helper()
		var result checkResult
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		return result
	}

	t.Run("missing parameters", func(t *testing.T) {
		if w := get(""); w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("token secret", func(t *testing.T) {
		w := get("?namespace=default&name=renovate-secret")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		result := decode(t, w)
		if !result.Found || !result.HasToken || result.HasGithubAppKeys || result.HasAllowRefLabel {
			t.Errorf("unexpected result %+v", result)
		}
	})

	t.Run("github app secret with allow-ref label", func(t *testing.T) {
		result := decode(t, get("?namespace=default&name=gh-app"))
		if !result.Found || result.HasToken || !result.HasGithubAppKeys || !result.HasAllowRefLabel {
			t.Errorf("unexpected result %+v", result)
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		result := decode(t, get("?namespace=default&name=nope"))
		if result.Found || result.HasToken || result.HasGithubAppKeys || result.HasAllowRefLabel {
			t.Errorf("unexpected result %+v", result)
		}
	})

	t.Run("foreign namespace is refused", func(t *testing.T) {
		// Nothing about secrets outside the operator's namespace and the
		// namespaces of existing RenovateJobs may leak: the endpoint answers
		// without a session, so an arbitrary namespace would make it a
		// cluster-wide secret existence oracle.
		if w := get("?namespace=kube-system&name=renovate-secret"); w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for a foreign namespace, got %d", w.Code)
		}
	})

	t.Run("namespace of an existing job is allowed", func(t *testing.T) {
		jobServer := &Server{
			logger:    logr.Discard(),
			discovery: &mockDiscoveryAgent{},
			manager: &mockRenovateJobManager{
				listRenovateJobsFullFunc: func(context.Context) ([]api.RenovateJob, error) {
					return []api.RenovateJob{{
						ObjectMeta: metav1.ObjectMeta{Name: "renovate", Namespace: "team-a"},
					}}, nil
				},
			},
			setup: SetupEnvironment{
				OwnNamespace: "renovate-operator",
				SecretReader: secretReaderWith(t, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "renovate-secret", Namespace: "team-a"},
					Data:       map[string][]byte{"RENOVATE_TOKEN": []byte("token")},
				}),
			},
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/secret?namespace=team-a&name=renovate-secret", nil)
		w := httptest.NewRecorder()
		jobServer.getSetupSecretCheck(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for a job namespace, got %d", w.Code)
		}
		if result := decode(t, w); !result.Found || !result.HasToken {
			t.Errorf("unexpected result %+v", result)
		}
	})

	t.Run("refused without a known own namespace", func(t *testing.T) {
		// An unresolved own namespace must not widen the probe: with no jobs
		// to compare against either, every namespace is refused rather than
		// allowed.
		blindServer := &Server{
			logger:    logr.Discard(),
			manager:   &mockRenovateJobManager{},
			discovery: &mockDiscoveryAgent{},
			setup:     SetupEnvironment{SecretReader: secretReaderWith(t, tokenSecret)},
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/secret?namespace=default&name=renovate-secret", nil)
		w := httptest.NewRecorder()
		blindServer.getSetupSecretCheck(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 without a known own namespace, got %d", w.Code)
		}
	})

	t.Run("closed once setup is complete", func(t *testing.T) {
		// The probe exists for the guide's first step. Once the first run is
		// finished nothing consults it, so it must not stay reachable.
		doneServer := &Server{
			logger:    logr.Discard(),
			discovery: &mockDiscoveryAgent{},
			manager: &mockRenovateJobManager{
				listRenovateJobsFullFunc: func(context.Context) ([]api.RenovateJob, error) {
					return []api.RenovateJob{{
						ObjectMeta: metav1.ObjectMeta{Name: "renovate", Namespace: "default"},
					}}, nil
				},
			},
			setup: SetupEnvironment{
				OwnNamespace: "default",
				SecretReader: secretReaderWith(t, tokenSecret),
			},
		}
		doneServer.setupComplete.Store(true)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/secret?namespace=default&name=renovate-secret", nil)
		w := httptest.NewRecorder()
		doneServer.getSetupSecretCheck(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 once setup is complete, got %d", w.Code)
		}
	})

	t.Run("closed when the state is unreadable", func(t *testing.T) {
		// No job list means no verdict on whether setup is finished; the probe
		// closes rather than answering on an unknown state.
		blindServer := &Server{
			logger:    logr.Discard(),
			discovery: &mockDiscoveryAgent{},
			manager: &mockRenovateJobManager{
				listRenovateJobsFullFunc: func(context.Context) ([]api.RenovateJob, error) {
					return nil, errors.New("api down")
				},
			},
			setup: SetupEnvironment{
				OwnNamespace: "default",
				SecretReader: secretReaderWith(t, tokenSecret),
			},
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/secret?namespace=default&name=renovate-secret", nil)
		w := httptest.NewRecorder()
		blindServer.getSetupSecretCheck(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 with an unreadable state, got %d", w.Code)
		}
	})

	t.Run("hidden when auth is configured", func(t *testing.T) {
		authServer := &Server{
			logger:    logr.Discard(),
			discovery: &mockDiscoveryAgent{},
			auth:      &OIDCAuth{},
			setup:     SetupEnvironment{SecretReader: secretReaderWith(t, tokenSecret)},
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/secret?namespace=default&name=renovate-secret", nil)
		w := httptest.NewRecorder()
		authServer.getSetupSecretCheck(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 with auth configured, got %d", w.Code)
		}
	})
}

func TestComputeSetupStatus_CompletionLatchesAndResets(t *testing.T) {
	projectCalls := 0
	jobs := []api.RenovateJob{{Name: "job1", Namespace: "default", Spec: api.RenovateJobSpec{SecretRef: "renovate-secret"}}}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "renovate-secret", Namespace: "default"},
		Data:       map[string][]byte{"RENOVATE_TOKEN": []byte("token")},
	}

	server := &Server{
		manager: &mockRenovateJobManager{
			listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
				return jobs, nil
			},
			getProjectsForRenovateJobFunc: func(ctx context.Context, jobId crdmanager.RenovateJobIdentifier) ([]crdmanager.RenovateProjectStatus, error) {
				projectCalls++
				return []crdmanager.RenovateProjectStatus{{
					Name: "org/repo", Namespace: "default",
					RenovateProjectState: api.RenovateProjectState{Status: api.JobStatusCompleted},
				}}, nil
			},
		},
		discovery: &mockDiscoveryAgent{},
		logger:    logr.Discard(),
		setup:     SetupEnvironment{SecretReader: secretReaderWith(t, tokenSecret)},
	}

	first := server.computeSetupStatus(context.Background())
	if !first.Complete {
		t.Fatalf("expected complete status, got %+v", first)
	}

	second := server.computeSetupStatus(context.Background())
	if !second.Complete || !second.Visible {
		t.Fatalf("latched status must stay visible and complete, got %+v", second)
	}
	if projectCalls != 1 {
		t.Errorf("expected the completion latch to skip project checks, got %d calls", projectCalls)
	}
	if len(second.Hints) == 0 {
		t.Error("latched status must still carry hints")
	}

	// Deleting every job means starting over: the latch resets and the guide
	// reports a fresh install again.
	jobs = nil
	third := server.computeSetupStatus(context.Background())
	if third.Complete || !third.Visible {
		t.Fatalf("deleting every job must reset the latch, got %+v", third)
	}
	if step := stepByID(t, third, "renovatejob"); step.State != setupStatePending {
		t.Errorf("renovatejob step = %q after reset, want pending", step.State)
	}
}

func TestDetectLockedOutJobs(t *testing.T) {
	jobWithoutAccess := api.RenovateJob{Name: "job1", Namespace: "default"}
	jobWithAccess := api.RenovateJob{Name: "job2", Namespace: "default", Spec: api.RenovateJobSpec{
		Access: &api.RenovateJobAccess{AdminUsers: []string{"admin@example.com"}},
	}}

	tests := []struct {
		name       string
		provider   AuthProvider
		defaults   AccessDefaults
		jobs       []api.RenovateJob
		wantLocked []string
	}{
		{
			name: "no auth provider - nothing locked out",
			jobs: []api.RenovateJob{jobWithoutAccess},
		},
		{
			name:     "authorization disabled - nothing locked out",
			provider: &OIDCAuth{},
			defaults: AccessDefaults{AuthorizationDisabled: true},
			jobs:     []api.RenovateJob{jobWithoutAccess},
		},
		{
			name:       "job without access and empty defaults is locked out",
			provider:   &OIDCAuth{},
			jobs:       []api.RenovateJob{jobWithoutAccess, jobWithAccess},
			wantLocked: []string{"default/job1"},
		},
		{
			name:     "operator-wide admin users unlock jobs without own access",
			provider: &OIDCAuth{},
			defaults: AccessDefaults{AdminUsers: []string{"admin@example.com"}},
			jobs:     []api.RenovateJob{jobWithoutAccess},
		},
		{
			name:     "anonymous read default unlocks jobs without own access",
			provider: &OIDCAuth{},
			defaults: AccessDefaults{AnonymousRead: true},
			jobs:     []api.RenovateJob{jobWithoutAccess},
		},
		{
			name:     "no jobs - no warning even with empty defaults",
			provider: &OIDCAuth{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, locked := detectLockedOutJobs(tt.provider, tt.defaults, tt.jobs)

			if len(tt.wantLocked) == 0 {
				if verdict != nil {
					t.Fatalf("expected no warning, got %+v (locked %v)", verdict, locked)
				}
				return
			}

			if verdict == nil {
				t.Fatal("expected a warning, got none")
			}
			if verdict.Reason != ReasonNoAccessRules {
				t.Errorf("reason = %q, want %q", verdict.Reason, ReasonNoAccessRules)
			}
			if len(locked) != len(tt.wantLocked) {
				t.Fatalf("locked jobs = %v, want %v", locked, tt.wantLocked)
			}
			for i, want := range tt.wantLocked {
				if locked[i] != want {
					t.Errorf("locked[%d] = %q, want %q", i, locked[i], want)
				}
			}
		})
	}
}
