package crdmanager

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	mgr := NewRenovateJobManager(cl)
	ctx := context.Background()
	list, err := mgr.ListRenovateJobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(list))
	}
}

func TestUpdateProjectStatus_AddAndUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	j := makeJob("job1", "default", nil)
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).Build()
	cl := inner

	mgr := NewRenovateJobManager(cl)
	ctx := context.Background()

	// shorten retries for tests
	// override status update implementation to avoid fake client Status() issues
	oldFn := updateRenovateJobStatusFn
	updateRenovateJobStatusFn = func(ctx context.Context, renovateJob *api.RenovateJob, client client.Client) (*api.RenovateJob, error) {
		if err := client.Update(ctx, renovateJob); err != nil {
			return nil, err
		}
		return loadRenovateJob(ctx, renovateJob.Name, renovateJob.Namespace, client)
	}
	defer func() { updateRenovateJobStatusFn = oldFn }()
	// sanity check: inner client can Get the object
	if err := inner.Get(ctx, client.ObjectKey{Name: "job1", Namespace: "default"}, &api.RenovateJob{}); err != nil {
		t.Fatalf("inner client cannot find object: %v", err)
	}

	// debug: try loadRenovateJob directly using the same client
	if lj, derr := loadRenovateJob(ctx, "job1", "default", cl); derr != nil {
		t.Fatalf("debug: loadRenovateJob direct error: %v", derr)
	} else {
		t.Logf("debug: loadRenovateJob direct succeeded, name=%s ns=%s", lj.Name, lj.Namespace)
	}

	// try direct status update to reproduce behaviour
	if lj, _ := loadRenovateJob(ctx, "job1", "default", cl); true {
		lj.Status.Projects = append(lj.Status.Projects, api.ProjectStatus{Name: "direct", Status: api.JobStatusScheduled})
		if derr := cl.Status().Update(ctx, lj); derr != nil {
			t.Logf("direct status update error: %v", derr)
		} else {
			t.Logf("direct status update succeeded")
		}
	}

	// add new project via manager
	err := mgr.UpdateProjectStatus(ctx, "p1", RenovateJobIdentifier{Name: "job1", Namespace: "default"}, api.JobStatusScheduled)
	if err != nil {
		t.Fatalf("unexpected error adding project: %v", err)
	}

	job, err := mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job: %v", err)
	}
	if len(job.Status.Projects) != 1 || job.Status.Projects[0].Name != "p1" {
		t.Fatalf("expected project p1 to be added, got: %v", job.Status.Projects)
	}

	// update existing project
	err = mgr.UpdateProjectStatus(ctx, "p1", RenovateJobIdentifier{Name: "job1", Namespace: "default"}, api.JobStatusRunning)
	if err != nil {
		t.Fatalf("unexpected error updating project: %v", err)
	}
	job, err = mgr.GetRenovateJob(ctx, "job1", "default")
	if err != nil {
		t.Fatalf("unexpected error getting job after update: %v", err)
	}
	if job.Status.Projects[0].Status != api.JobStatusRunning {
		t.Fatalf("expected status running, got %v", job.Status.Projects[0].Status)
	}
}

func TestUpdateProjectStatusBatched(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	projects := []api.ProjectStatus{{Name: "p1", Status: api.JobStatusRunning}, {Name: "p2", Status: api.JobStatusScheduled}}
	j := makeJob("job1", "default", projects)
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).Build()
	cl := inner

	mgr := NewRenovateJobManager(cl)
	ctx := context.Background()

	// override status update implementation to avoid fake client Status() issues
	oldFn := updateRenovateJobStatusFn
	updateRenovateJobStatusFn = func(ctx context.Context, renovateJob *api.RenovateJob, client client.Client) (*api.RenovateJob, error) {
		if err := client.Update(ctx, renovateJob); err != nil {
			return nil, err
		}
		return loadRenovateJob(ctx, renovateJob.Name, renovateJob.Namespace, client)
	}
	defer func() { updateRenovateJobStatusFn = oldFn }()
	// sanity check: inner client can Get the object
	if err := inner.Get(ctx, client.ObjectKey{Name: "job1", Namespace: "default"}, &api.RenovateJob{}); err != nil {
		t.Fatalf("inner client cannot find object: %v", err)
	}

	// debug: try loadRenovateJob directly using the same client
	if _, derr := loadRenovateJob(ctx, "job1", "default", cl); derr != nil {
		t.Fatalf("debug: loadRenovateJob direct error: %v", derr)
	}

	// predicate: mark non-running projects as scheduled
	predicate := func(p api.ProjectStatus) bool { return p.Status != api.JobStatusRunning }
	err := mgr.UpdateProjectStatusBatched(ctx, predicate, RenovateJobIdentifier{Name: "job1", Namespace: "default"}, api.JobStatusScheduled)
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
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(j).Build()
	cl := inner

	mgr := NewRenovateJobManager(cl)
	ctx := context.Background()

	// override status update implementation to avoid fake client Status() issues
	oldFn := updateRenovateJobStatusFn
	updateRenovateJobStatusFn = func(ctx context.Context, renovateJob *api.RenovateJob, client client.Client) (*api.RenovateJob, error) {
		if err := client.Update(ctx, renovateJob); err != nil {
			return nil, err
		}
		return loadRenovateJob(ctx, renovateJob.Name, renovateJob.Namespace, client)
	}
	defer func() { updateRenovateJobStatusFn = oldFn }()
	// sanity check: inner client can Get the object
	if err := inner.Get(ctx, client.ObjectKey{Name: "job1", Namespace: "default"}, &api.RenovateJob{}); err != nil {
		t.Fatalf("inner client cannot find object: %v", err)
	}
	// debug: try loadRenovateJob directly using the same client
	if _, derr := loadRenovateJob(ctx, "job1", "default", cl); derr != nil {
		t.Fatalf("debug: loadRenovateJob direct error: %v", derr)
	}

	err := mgr.ReconcileProjects(ctx, RenovateJobIdentifier{Name: "job1", Namespace: "default"}, []string{"a", "b"})
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

	mgr := NewRenovateJobManager(cl)
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
