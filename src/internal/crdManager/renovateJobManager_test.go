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
	"renovate-operator/internal/policy"
	"renovate-operator/internal/types"
	"renovate-operator/internal/utils"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testPolicy allows the hosts the fixtures in this file point at, so tests that
// are not about the destination policy are unaffected by it.
func testPolicy() policy.Policy {
	return policy.Policy{AllowedHosts: []string{"api.github.com", "example.com", "operator.example.com"}}
}

// helper to create a basic RenovateJob (projects are separate RenovateProject CRDs)
func makeJob(name, namespace string) *api.RenovateJob {
	j := &api.RenovateJob{}
	j.Name = name
	j.Namespace = namespace
	j.TypeMeta = metav1.TypeMeta{APIVersion: api.GroupVersion.String(), Kind: "RenovateJob"}
	j.ObjectMeta = metav1.ObjectMeta{Name: name, Namespace: namespace}
	j.Spec = api.RenovateJobSpec{Schedule: "*/5 * * * *"}
	return j
}

// helper to create a RenovateProject CRD owned by the given job.
func makeProject(jobName, namespace, project string, status api.RenovateProjectStatus) *api.RenovateProject {
	rp := &api.RenovateProject{}
	rp.Name = utils.RenovateProjectCRDName(jobName, project)
	rp.Namespace = namespace
	rp.Labels = map[string]string{api.LabelRenovateJob: jobName}
	rp.Spec = api.RenovateProjectSpec{Project: project}
	rp.Status = api.RenovateProjectState{
		Status:         status,
		LastTransition: metav1.Now(),
	}
	return rp
}

func TestListRenovateJobs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j1 := makeJob("job1", "default")
	j2 := makeJob("job2", "kube")

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j1, j2).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil, testPolicy())
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

	j1 := makeJob("job1", "default")
	j2 := makeJob("job2", "kube")
	rp1 := makeProject("job1", "default", "p1", api.JobStatusRunning)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j1, j2, rp1).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil, testPolicy())
	ctx := context.Background()
	list, err := mgr.ListRenovateJobsFull(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(list))
	}
	for _, job := range list {
		if job.Spec.Schedule != "*/5 * * * *" {
			t.Fatalf("expected schedule '*/5 * * * *', got '%s'", job.Spec.Schedule)
		}
	}
	// Verify project is accessible via dedicated CRD
	projects, err := mgr.GetProjectsForRenovateJob(ctx, RenovateJobIdentifier{Name: "job1", Namespace: "default"})
	if err != nil {
		t.Fatalf("unexpected error getting projects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected job1 to have 1 project, got %d", len(projects))
	}
}

func TestUpdateProjectStatus_AddAndUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j := makeJob("job1", "default")
	rp := makeProject("job1", "default", "existingProject", api.JobStatusScheduled)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(j, rp).
		WithStatusSubresource(&api.RenovateJob{}, &api.RenovateProject{}).
		Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil, testPolicy())
	ctx := context.Background()
	jobId := RenovateJobIdentifier{Name: "job1", Namespace: "default"}

	err = mgr.UpdateProjectStatus(ctx, "existingProject", jobId, &types.RenovateStatusUpdate{Status: api.JobStatusRunning})
	if err != nil {
		t.Fatalf("unexpected error updating project: %v", err)
	}

	projects, err := mgr.GetProjectsForRenovateJob(ctx, jobId)
	if err != nil {
		t.Fatalf("unexpected error getting projects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "existingProject" {
		t.Fatalf("got unexpected projects after update: %v", projects)
	}
	if projects[0].Status != api.JobStatusRunning {
		t.Fatalf("expected project status running after update, got: %s", projects[0].Status)
	}

	err = mgr.UpdateProjectStatus(ctx, "notExistingProject", jobId, &types.RenovateStatusUpdate{Status: api.JobStatusRunning})
	if err != ErrProjectNotFound {
		t.Fatalf("expected project not found error updating not existing project")
	}
}

func TestUpdateProjectStatusBatched(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j := makeJob("job1", "default")
	rp1 := makeProject("job1", "default", "p1", api.JobStatusRunning)
	rp2 := makeProject("job1", "default", "p2", api.JobStatusScheduled)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(j, rp1, rp2).
		WithStatusSubresource(&api.RenovateJob{}, &api.RenovateProject{}).
		Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil, testPolicy())
	ctx := context.Background()
	jobId := RenovateJobIdentifier{Name: "job1", Namespace: "default"}

	// predicate: mark non-running projects as scheduled
	predicate := func(p RenovateProjectStatus) bool { return p.Status != api.JobStatusRunning }
	err = mgr.UpdateProjectStatusBatched(ctx, predicate, jobId, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled})
	if err != nil {
		t.Fatalf("unexpected error in batched update: %v", err)
	}

	projects, err := mgr.GetProjectsForRenovateJob(ctx, jobId)
	if err != nil {
		t.Fatalf("unexpected error getting projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	foundP2 := false
	for _, p := range projects {
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

	j := makeJob("job1", "default")
	// existing project 'a' with Completed status
	rp := makeProject("job1", "default", "a", api.JobStatusCompleted)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(j, rp).
		WithStatusSubresource(&api.RenovateJob{}, &api.RenovateProject{}).
		Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil, testPolicy())
	ctx := context.Background()
	jobId := RenovateJobIdentifier{Name: "job1", Namespace: "default"}

	rJob, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job for reconcile: %v", err)
	}

	_, err = mgr.ReconcileProjects(ctx, rJob, []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error in reconcile: %v", err)
	}

	projects, err := mgr.GetProjectsForRenovateJob(ctx, jobId)
	if err != nil {
		t.Fatalf("unexpected error getting projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	var statusA api.RenovateProjectStatus
	var hasB bool
	for _, p := range projects {
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

	j := makeJob("job1", "default")
	rp1 := makeProject("job1", "default", "a", api.JobStatusCompleted)
	rp2 := makeProject("job1", "default", "b", api.JobStatusScheduled)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j, rp1, rp2).Build()

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{}, objectstore.S3Config{}, "")
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log, nil, testPolicy())
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

func TestWebhookURLForJobPrefersSpecBaseURL(t *testing.T) {
	setBaseURL(t, "https://hooks.example.com")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = "https://renovate.internal.example/"

	url, err := webhookURLForJob(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://renovate.internal.example/webhook/v1/forgejo" {
		t.Errorf("expected spec base URL to take precedence, got %s", url)
	}
}

func TestWebhookURLForJobFallsBackToEnvBaseURL(t *testing.T) {
	setBaseURL(t, "https://hooks.example.com")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = ""

	url, err := webhookURLForJob(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://hooks.example.com/webhook/v1/forgejo" {
		t.Errorf("expected fallback to WEBHOOK_BASE_URL, got %s", url)
	}
}

func TestWebhookURLForJobUsesSpecBaseURLWithoutEnv(t *testing.T) {
	setBaseURL(t, "")

	job := syncJob("forgejo")
	job.Spec.Webhook.BaseURL = "https://renovate.internal.example"

	url, err := webhookURLForJob(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://renovate.internal.example/webhook/v1/forgejo" {
		t.Errorf("expected spec base URL to be sufficient on its own, got %s", url)
	}
}

func TestWebhookURLForJobErrorsWithoutBaseURL(t *testing.T) {
	setBaseURL(t, "")

	_, err := webhookURLForJob(syncJob("forgejo"))
	if err == nil || !strings.Contains(err.Error(), "WEBHOOK_BASE_URL") {
		t.Fatalf("expected actionable error without base URL, got %v", err)
	}
	if !strings.Contains(err.Error(), "spec.webhook.baseUrl") {
		t.Errorf("expected error to mention the per-job override, got %v", err)
	}
}

func TestWebhookURLForJobErrorsForUnsupportedPlatform(t *testing.T) {
	setBaseURL(t, "https://hooks.example.com")

	_, err := webhookURLForJob(syncJob("bitbucket"))
	if err == nil {
		t.Fatal("expected error for platform without webhook endpoint")
	}
}
