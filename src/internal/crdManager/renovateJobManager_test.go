package crdmanager

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/kvstore"
	"renovate-operator/internal/logStore"
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

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{})
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log)
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

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{})
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log)
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

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{})
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log)
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
	if err != ProjectNotFound {
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

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{})
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log)
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

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{})
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log)
	ctx := context.Background()

	rJob, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job for reconcile: %v", err)
	}

	err = mgr.ReconcileProjects(ctx, rJob, []string{"a", "b"})
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

	log, err := logStore.NewLogStore(logr.Logger{}, "memory", kvstore.ValkeyConfig{})
	if err != nil {
		t.Fatalf("failed to initialise logStore")
	}
	mgr := NewRenovateJobManager(cl, nil, logr.Logger{}, log)
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
