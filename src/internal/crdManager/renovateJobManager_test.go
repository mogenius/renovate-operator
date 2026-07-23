package crdmanager

import (
	"context"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	"renovate-operator/internal/kvstore"
	"renovate-operator/internal/logStore"
	"renovate-operator/internal/objectstore"
	"renovate-operator/internal/types"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// helper to create a basic RenovateJob
func makeJob(name, namespace string, projects []api.ProjectStatus) *api.RenovateJob {
	j := &api.RenovateJob{}
	j.Name = name
	j.Namespace = namespace
	j.TypeMeta = metav1.TypeMeta{APIVersion: "renovate-operator.mogenius.com/v1alpha1", Kind: "RenovateJob"}
	j.ObjectMeta = metav1.ObjectMeta{Name: name, Namespace: namespace}
	j.Spec = api.RenovateJobSpec{Schedule: "*/5 * * * *"}
	j.Status = api.RenovateJobStatus{Projects: projects}
	return j
}

func TestListRenovateJobs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j1 := makeJob("job1", "default", nil)
	j2 := makeJob("job2", "kube", nil)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j1, j2).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()
	list, err := mgr.ListRenovateJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(list))
	}
}

func TestListRenovateJobsFull(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j1 := makeJob("job1", "default", []api.ProjectStatus{{Name: "p1", Status: api.JobStatusRunning}})
	j2 := makeJob("job2", "kube", nil)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j1, j2).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()
	list, err := mgr.ListRenovateJobsFull(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(list))
	}
	// Verify full data is returned (not just identifiers)
	for _, job := range list {
		if job.Spec.Schedule != "*/5 * * * *" {
			t.Fatalf("expected schedule '*/5 * * * *', got '%s'", job.Spec.Schedule)
		}
		if job.Name == "job1" && len(job.Status.Projects) != 1 {
			t.Fatalf("expected job1 to have 1 project, got %d", len(job.Status.Projects))
		}
	}
}

func TestUpdateProjectStatus_AddAndUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j := makeJob("job1", "default", []api.ProjectStatus{{
		Name:   "existingProject",
		Status: api.JobStatusScheduled,
	}})
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).WithStatusSubresource(&api.RenovateJob{}).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()

	err = mgr.UpdateProjectStatus(ctx, "existingProject", RenovateJobIdentifier{Name: "job1", Namespace: "default"}, &types.RenovateStatusUpdate{Status: api.JobStatusRunning})
	if err != nil {
		t.Fatalf("unexpected error updating project: %v", err)
	}
	job, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job: %v", err)
	}

	if len(job.Status.Projects) != 1 || job.Status.Projects[0].Name != "existingProject" {
		t.Fatalf("got unexcpedted projects after update: %v", job.Status.Projects)
	}

	if job.Status.Projects[0].Status != api.JobStatusRunning {
		t.Fatalf("expected project status running after update, got: %s", job.Status.Projects[0].Status)
	}

	err = mgr.UpdateProjectStatus(ctx, "notExistingProject", RenovateJobIdentifier{Name: "job1", Namespace: "default"}, &types.RenovateStatusUpdate{Status: api.JobStatusRunning})
	if err != ErrProjectNotFound {
		t.Fatalf("expected project not found error updating not existing project")
	}
}

func TestUpdateProjectStatusBatched(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	projects := []api.ProjectStatus{{Name: "p1", Status: api.JobStatusRunning}, {Name: "p2", Status: api.JobStatusScheduled}}
	j := makeJob("job1", "default", projects)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).WithStatusSubresource(&api.RenovateJob{}).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()

	// predicate: mark non-running projects as scheduled
	predicate := func(p api.ProjectStatus) bool { return p.Status != api.JobStatusRunning }
	err = mgr.UpdateProjectStatusBatched(ctx, predicate, RenovateJobIdentifier{Name: "job1", Namespace: "default"}, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled})
	if err != nil {
		t.Fatalf("unexpected error in batched update: %v", err)
	}
	job, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job: %v", err)
	}
	// p1 should remain running, p2 should be scheduled
	if len(job.Status.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(job.Status.Projects))
	}
	foundP2 := false
	for _, p := range job.Status.Projects {
		if p.Name == "p2" {
			foundP2 = true
			if p.Status != api.JobStatusScheduled {
				t.Fatalf("expected p2 scheduled, got %v", p.Status)
			}
		}
	}
	if !foundP2 {
		t.Fatalf("p2 not found after batched update")
	}
}

func TestReconcileProjects_AddsAndKeepsExisting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	// existing project 'a' present
	projects := []api.ProjectStatus{{Name: "a", Status: api.JobStatusCompleted}}
	j := makeJob("job1", "default", projects)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).WithStatusSubresource(&api.RenovateJob{}).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()

	rJob, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job for reconcile: %v", err)
	}

	_, err = mgr.ReconcileProjects(ctx, rJob, []string{"a", "b"}, "")
	if err != nil {
		t.Fatalf("unexpected error in reconcile: %v", err)
	}
	job, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job: %v", err)
	}
	if len(job.Status.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(job.Status.Projects))
	}
	// ensure a kept its existing status
	var statusA api.RenovateProjectStatus
	var hasB bool
	for _, p := range job.Status.Projects {
		if p.Name == "a" {
			statusA = p.Status
		}
		if p.Name == "b" {
			hasB = true
		}
	}
	if statusA != api.JobStatusCompleted {
		t.Fatalf("expected a to keep completed status, got %v", statusA)
	}
	if !hasB {
		t.Fatalf("expected b to be added")
	}
}

func TestGetProjectsFilters(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	projects := []api.ProjectStatus{{Name: "a", Status: api.JobStatusCompleted}, {Name: "b", Status: api.JobStatusScheduled}}
	j := makeJob("job1", "default", projects)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()

	list, err := mgr.GetProjectsByStatus(ctx, RenovateJobIdentifier{Name: "job1", Namespace: "default"}, api.JobStatusCompleted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "a" {
		t.Fatalf("expected only project a, got %v", list)
	}

	all, err := mgr.GetProjectsForRenovateJob(ctx, RenovateJobIdentifier{Name: "job1", Namespace: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 projects from GetProjectsForRenovateJob, got %d", len(all))
	}
}

// setBaseURL (re)initializes the config singleton with the given
// WEBHOOK_BASE_URL, mirroring how the Helm chart provides it via env.
func setBaseURL(t *testing.T, baseURL string) {
	t.Setenv("WEBHOOK_BASE_URL", baseURL)
	if err := config.InitializeConfigModule([]config.ConfigItemDescription{{Key: "WEBHOOK_BASE_URL", Optional: true}}); err != nil {
		t.Fatalf("failed to initialize config: %v", err)
	}
}

func syncJob(platform string) *api.RenovateJob {
	return &api.RenovateJob{
		Spec: api.RenovateJobSpec{
			Provider: &api.RenovateProvider{Name: platform},
			Webhook: &api.RenovateWebhook{
				Enabled: true,
				Sync:    &api.RenovateWebhookSync{Enabled: true},
			},
		},
	}
}

func TestWebhookURLForJobUsesBaseURLAndPlatformPath(t *testing.T) {
	setBaseURL(t, "https://hooks.example.com/")

	url, err := webhookURLForJob(syncJob("forgejo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://hooks.example.com/webhook/v1/forgejo" {
		t.Errorf("expected base URL plus platform path, got %s", url)
	}
}

func TestWebhookURLForJobErrorsWithoutBaseURL(t *testing.T) {
	setBaseURL(t, "")

	_, err := webhookURLForJob(syncJob("forgejo"))
	if err == nil || !strings.Contains(err.Error(), "WEBHOOK_BASE_URL") {
		t.Fatalf("expected actionable error without base URL, got %v", err)
	}
}

func TestReconcileProjects_TokenSecretName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}

	makeManager := func(projects []api.ProjectStatus) (RenovateJobManager, *api.RenovateJob) {
		j := makeJob("job1", "default", projects)
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).WithStatusSubresource(&api.RenovateJob{}).Build()
		mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
		ctx := context.Background()
		rJob, err := mgr.GetRenovateJob(ctx, "job1", "default")
		if err != nil {
			t.Fatalf("unexpected error getting job: %v", err)
		}
		return mgr, rJob
	}

	ctx := context.Background()

	t.Run("empty tokenSecretName leaves new project with empty field", func(t *testing.T) {
		mgr, rJob := makeManager(nil)
		if _, err := mgr.ReconcileProjects(ctx, rJob, []string{"org/repo"}, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		job, _ := mgr.GetRenovateJob(ctx, "job1", "default")
		if job.Status.Projects[0].TokenSecretName != "" {
			t.Fatalf("expected empty TokenSecretName, got %q", job.Status.Projects[0].TokenSecretName)
		}
	})

	t.Run("tokenSecretName is set on new project", func(t *testing.T) {
		mgr, rJob := makeManager(nil)
		if _, err := mgr.ReconcileProjects(ctx, rJob, []string{"org/repo"}, "job1-github-app-123-abcd"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		job, _ := mgr.GetRenovateJob(ctx, "job1", "default")
		if job.Status.Projects[0].TokenSecretName != "job1-github-app-123-abcd" {
			t.Fatalf("expected TokenSecretName %q, got %q", "job1-github-app-123-abcd", job.Status.Projects[0].TokenSecretName)
		}
	})

	t.Run("tokenSecretName updates carry-over project", func(t *testing.T) {
		existing := []api.ProjectStatus{{Name: "org/repo", Status: api.JobStatusCompleted, TokenSecretName: "old-secret"}}
		mgr, rJob := makeManager(existing)
		if _, err := mgr.ReconcileProjects(ctx, rJob, []string{"org/repo"}, "new-secret"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		job, _ := mgr.GetRenovateJob(ctx, "job1", "default")
		if job.Status.Projects[0].TokenSecretName != "new-secret" {
			t.Fatalf("expected TokenSecretName %q, got %q", "new-secret", job.Status.Projects[0].TokenSecretName)
		}
	})

	t.Run("empty tokenSecretName preserves carry-over project TokenSecretName", func(t *testing.T) {
		existing := []api.ProjectStatus{{Name: "org/repo", Status: api.JobStatusCompleted, TokenSecretName: "existing-secret"}}
		mgr, rJob := makeManager(existing)
		if _, err := mgr.ReconcileProjects(ctx, rJob, []string{"org/repo"}, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		job, _ := mgr.GetRenovateJob(ctx, "job1", "default")
		if job.Status.Projects[0].TokenSecretName != "existing-secret" {
			t.Fatalf("expected TokenSecretName %q to be preserved, got %q", "existing-secret", job.Status.Projects[0].TokenSecretName)
		}
	})
}

// TestReconcileProjects_EnterpriseAppMultiInstallation demonstrates the fault
// described in the code review: when enterprise-app mode creates one discovery
// Job per installation, each call to ReconcileProjects rebuilds Status.Projects
// solely from that installation's discovered repos. The second call therefore
// marks the first installation's repos as "removed" and overwrites them in the
// CRD, even though they are still valid.
//
// This test FAILS with the current implementation and should PASS after the fix.
func TestReconcileProjects_EnterpriseAppMultiInstallation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}

	j := makeJob("job1", "default", nil)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).WithStatusSubresource(&api.RenovateJob{}).Build()
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil)
	ctx := context.Background()

	// Installation A finishes first and writes its repos.
	rJob, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job: %v", err)
	}
	removedA, err := mgr.ReconcileProjects(ctx, rJob, []string{"org/repo-a1", "org/repo-a2"}, "secret-a")
	if err != nil {
		t.Fatalf("installation A reconcile failed: %v", err)
	}
	if len(removedA) != 0 {
		t.Fatalf("expected no removals after installation A, got %v", removedA)
	}

	// Installation B finishes second and writes only its own repos.
	rJob, err = mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job: %v", err)
	}
	removedB, err := mgr.ReconcileProjects(ctx, rJob, []string{"org/repo-b1", "org/repo-b2"}, "secret-b")
	if err != nil {
		t.Fatalf("installation B reconcile failed: %v", err)
	}

	// BUG: the current implementation reports A's repos as "removed" because
	// they are absent from B's project list. This should be empty.
	if len(removedB) != 0 {
		t.Fatalf("installation B incorrectly reported %d removed projects (expected 0): %v", len(removedB), removedB)
	}

	// BUG: Status.Projects should contain repos from both installations, but
	// the current implementation overwrites it with only B's repos.
	job, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job after both reconciles: %v", err)
	}
	if len(job.Status.Projects) != 4 {
		t.Fatalf("expected 4 projects (2 per installation), got %d: %v", len(job.Status.Projects), job.Status.Projects)
	}
	projectNames := make(map[string]struct{}, len(job.Status.Projects))
	for _, p := range job.Status.Projects {
		projectNames[p.Name] = struct{}{}
	}
	for _, expected := range []string{"org/repo-a1", "org/repo-a2", "org/repo-b1", "org/repo-b2"} {
		if _, ok := projectNames[expected]; !ok {
			t.Errorf("project %q missing from Status.Projects after both installations reconciled", expected)
		}
	}
}

func TestWebhookURLForJobErrorsForUnsupportedPlatform(t *testing.T) {
	setBaseURL(t, "https://hooks.example.com")

	_, err := webhookURLForJob(syncJob("bitbucket"))
	if err == nil {
		t.Fatal("expected error for platform without webhook endpoint")
	}
}
