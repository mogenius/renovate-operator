package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdmanager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/types"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
)

// fakeDiscoveryAgent records CreateDiscoveryJob calls for handler tests.
type fakeDiscoveryAgent struct {
	createFunc func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error)
	statusFunc func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error)
}

func (f *fakeDiscoveryAgent) CreateDiscoveryJob(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
	if f.createFunc != nil {
		return f.createFunc(ctx, job, options)
	}
	return "discovery", nil
}

func (f *fakeDiscoveryAgent) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	if f.statusFunc != nil {
		return f.statusFunc(ctx, job)
	}
	return api.JobStatusScheduled, nil
}

func (f *fakeDiscoveryAgent) ProcessDiscoveryJobResult(ctx context.Context, k8sJob *batchv1.Job, jobId crdmanager.RenovateJobIdentifier) error {
	return nil
}

// makeGitHubRenovateJob builds a webhook-enabled GitHub job with discovery filters.
func makeGitHubRenovateJob(namespace, name string, filters ...string) api.RenovateJob {
	return api.RenovateJob{
		Name:      name,
		Namespace: namespace,
		Spec: api.RenovateJobSpec{
			Webhook: &api.RenovateWebhook{
				Enabled: true,
			},
			Provider: &api.RenovateProvider{
				Name: "github",
			},
			DiscoveryFilters: filters,
		},
	}
}

func postGitHubEvent(server *Server, event string, payload any) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", event)
	w := httptest.NewRecorder()
	server.githubWebhook(w, req)
	return w
}

func TestGitHubRepositoryEventStartsDiscovery(t *testing.T) {
	var createdFor []string
	job1 := makeGitHubRenovateJob("renovate", "job1", "example/*")
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{job1}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			if options.TriggerAllProjects {
				t.Error("expected TriggerAllProjects to be false for webhook-triggered discovery")
			}
			createdFor = append(createdFor, job.Fullname())
			return "discovery", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if len(createdFor) != 1 || createdFor[0] != job1.Fullname() {
		t.Errorf("expected discovery to start for %s, got %v", job1.Fullname(), createdFor)
	}
}

func TestGitHubRepositoryEventDeletedStartsDiscovery(t *testing.T) {
	called := false
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/.*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			called = true
			return "discovery", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "deleted",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !called {
		t.Error("expected discovery to be started for a deleted repository")
	}
}

func TestGitHubRepositoryEventIgnoresNonLifecycleAction(t *testing.T) {
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			t.Error("discovery must not start for a non-lifecycle repository action")
			return "", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "renamed",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventFilterMismatchIsIgnored(t *testing.T) {
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "other/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			t.Error("discovery must not start when no filter matches")
			return "", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventIgnoresNonGitHubJob(t *testing.T) {
	job := makeGitHubRenovateJob("renovate", "job1", "example/*")
	job.Spec.Provider.Name = "gitlab"
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{job}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			t.Error("discovery must not start for a non-GitHub job")
			return "", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventScopesToNamespace(t *testing.T) {
	var createdFor []string
	jobs := []api.RenovateJob{
		makeGitHubRenovateJob("renovate", "job1", "example/*"),
		makeGitHubRenovateJob("other", "job2", "example/*"),
	}
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return jobs, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			createdFor = append(createdFor, job.Fullname())
			return "discovery", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	body, _ := json.Marshal(GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github?namespace=renovate", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "repository")
	w := httptest.NewRecorder()
	server.githubWebhook(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if len(createdFor) != 1 || createdFor[0] != jobs[0].Fullname() {
		t.Errorf("expected discovery to start only for %s, got %v", jobs[0].Fullname(), createdFor)
	}
}

func TestGitHubRepositoryEventSkipsRunningDiscovery(t *testing.T) {
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		statusFunc: func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
			return api.JobStatusRunning, nil
		},
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			t.Error("discovery must not start while one is already running")
			return "", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventSuspendedJobIsIgnored(t *testing.T) {
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			return "", renovate.ErrRenovateJobSuspended
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventRequiresAuthentication(t *testing.T) {
	job := makeGitHubRenovateJob("renovate", "job1", "example/*")
	job.Spec.Webhook.Authentication = &api.RenovateWebhookAuth{Enabled: true}
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{job}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			t.Error("discovery must not start without valid credentials")
			return "", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	// no credential at all
	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, w.Code, w.Body.String())
	}

	// valid bearer token
	discovery.createFunc = func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
		return "discovery", nil
	}
	body, _ := json.Marshal(GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "repository")
	req.Header.Set("Authorization", "Bearer token")
	w = httptest.NewRecorder()
	server.githubWebhook(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventMalformedPayload(t *testing.T) {
	server := &Server{manager: &mockWebhookManager{}, discovery: &fakeDiscoveryAgent{}, logger: logr.Discard()}

	req := httptest.NewRequest(http.MethodPost, "/webhook/v1/github", bytes.NewReader([]byte("not json")))
	req.Header.Set("X-GitHub-Event", "repository")
	w := httptest.NewRecorder()
	server.githubWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestGitHubRepositoryEventWithoutDiscoveryAgent(t *testing.T) {
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	server := &Server{manager: mockManager, logger: logr.Discard()}

	w := postGitHubEvent(server, "repository", GitHubRepositoryEvent{
		Action:     "created",
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
}

func TestGitHubInstallationEventMatchesPayloadRepositories(t *testing.T) {
	called := false
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			called = true
			return "discovery", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "installation", GitHubInstallationEvent{
		Action:       "created",
		Installation: GitHubInstallation{ID: 1, Account: GitHubInstallationAccount{Login: "example"}},
		Repositories: []GitHubRepository{{FullName: "example/repo"}},
	})

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !called {
		t.Error("expected discovery to start for installation repositories")
	}
}

func TestGitHubInstallationEventFallsBackToAccountLogin(t *testing.T) {
	called := false
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			called = true
			return "discovery", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "installation", GitHubInstallationEvent{
		Action:       "created",
		Installation: GitHubInstallation{ID: 1, Account: GitHubInstallationAccount{Login: "example"}},
	})

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !called {
		t.Error("expected discovery to start via account login fallback")
	}
}

func TestGitHubInstallationEventIgnoresDeletedAction(t *testing.T) {
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeGitHubRenovateJob("renovate", "job1", "example/*")}, nil
		},
	}
	discovery := &fakeDiscoveryAgent{
		createFunc: func(ctx context.Context, job api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
			t.Error("discovery must not start when the installation is removed")
			return "", nil
		},
	}
	server := &Server{manager: mockManager, discovery: discovery, logger: logr.Discard()}

	w := postGitHubEvent(server, "installation", GitHubInstallationEvent{
		Action:       "deleted",
		Installation: GitHubInstallation{ID: 1, Account: GitHubInstallationAccount{Login: "example"}},
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestGitHubPullRequestEventsStillScheduleProjects(t *testing.T) {
	updateCalled := false
	mockManager := &mockWebhookManager{
		listRenovateJobsFullFunc: func(ctx context.Context) ([]api.RenovateJob, error) {
			return []api.RenovateJob{makeTestRenovateJob("renovate", "job1", "example/repo")}, nil
		},
		updateProjectStatusFunc: func(ctx context.Context, project string, jobId crdmanager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			updateCalled = true
			return nil
		},
	}
	server := &Server{manager: mockManager, discovery: &fakeDiscoveryAgent{}, logger: logr.Discard()}

	w := postGitHubEvent(server, "pull_request", GitHubEvent{
		Action: "edited",
		PullRequest: &GitHubPullRequest{
			Body: "Updated body with checkbox - [x] <!-- rebase-check -->If you want to rebase/retry this PR",
		},
		Repository: GitHubRepository{FullName: "example/repo"},
	})

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d: %s", http.StatusAccepted, w.Code, w.Body.String())
	}
	if !updateCalled {
		t.Error("expected project status update for pull request checkbox event")
	}
}

func TestDiscoveryFiltersMatch(t *testing.T) {
	tests := []struct {
		name       string
		filters    []string
		repository string
		want       bool
	}{
		{name: "no filters matches everything", filters: nil, repository: "example/repo", want: true},
		{name: "exact filter matches", filters: []string{"example/repo"}, repository: "example/repo", want: true},
		{name: "org wildcard matches", filters: []string{"example/*"}, repository: "example/repo", want: true},
		{name: "prefix regex matches", filters: []string{"example/.*"}, repository: "example/repo", want: true},
		{name: "second filter matches", filters: []string{"other/*", "example/*"}, repository: "example/repo", want: true},
		{name: "no filter matches", filters: []string{"other/*"}, repository: "example/repo", want: false},
		{name: "invalid regex is skipped", filters: []string{"["}, repository: "example/repo", want: false},
		{name: "case mismatch does not match", filters: []string{"example/repo"}, repository: "Example/Repo", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := discoveryFiltersMatch(tt.filters, tt.repository); got != tt.want {
				t.Errorf("discoveryFiltersMatch(%v, %q) = %v, want %v", tt.filters, tt.repository, got, tt.want)
			}
		})
	}
}
