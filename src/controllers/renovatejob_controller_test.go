package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// fakeManager implements the full RenovateJobManager interface but only the
// methods used by the reconciler are given meaningful behaviour in tests.
type fakeManager struct {
	getFn                        func(ctx context.Context, name, namespace string) (*api.RenovateJob, error)
	reconcileProjectsFn          func(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error
	updateProjectStatusBatchedFn func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error
}

func (f *fakeManager) ListRenovateJobs(ctx context.Context) ([]crdManager.RenovateJobIdentifier, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeManager) GetRenovateJob(ctx context.Context, name string, namespace string) (*api.RenovateJob, error) {
	if f.getFn != nil {
		return f.getFn(ctx, name, namespace)
	}
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeManager) GetProjectsForRenovateJob(ctx context.Context, job crdManager.RenovateJobIdentifier) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeManager) UpdateProjectStatus(ctx context.Context, project string, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
	if f.updateProjectStatusBatchedFn != nil {
		return f.updateProjectStatusBatchedFn(ctx, fn, job, status)
	}
	return nil
}
func (f *fakeManager) GetProjectsByStatus(ctx context.Context, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeManager) ReconcileProjects(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error {
	if f.reconcileProjectsFn != nil {
		return f.reconcileProjectsFn(ctx, job, projects)
	}
	return nil
}
func (f *fakeManager) GetLogsForProject(ctx context.Context, job crdManager.RenovateJobIdentifier, project string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (f *fakeManager) IsWebhookTokenValid(ctx context.Context, job crdManager.RenovateJobIdentifier, token string) (bool, error) {
	return true, nil
}

type fakeDiscovery struct {
	discoverFn func(ctx context.Context, job *api.RenovateJob) ([]string, error)
}

func (f *fakeDiscovery) Discover(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	if f.discoverFn != nil {
		return f.discoverFn(ctx, job)
	}
	return []string{}, nil
}
func (f *fakeDiscovery) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob) (*batchv1.Job, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeDiscovery) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	return api.JobStatusCompleted, nil
}
func (f *fakeDiscovery) WaitForDiscoveryJob(ctx context.Context, job *api.RenovateJob) ([]string, error) {
	return []string{}, nil
}

type fakeScheduler struct {
	addedExpr    string
	addedName    string
	addCalled    bool
	removedName  string
	removeCalled bool
	storedFn     func()
	addErr       error
}

func (f *fakeScheduler) AddScheduleReplaceExisting(expr string, name string, fct func()) error {
	f.addedExpr = expr
	f.addedName = name
	f.addCalled = true
	f.storedFn = fct
	return f.addErr
}
func (f *fakeScheduler) RemoveSchedule(name string) {
	f.removedName = name
	f.removeCalled = true
}

// implement remaining methods of scheduler.Scheduler as no-ops for tests
func (f *fakeScheduler) Start() {}
func (f *fakeScheduler) Stop()  {}
func (f *fakeScheduler) AddSchedule(expr string, name string, fn func()) error {
	// behave like AddScheduleReplaceExisting for tests
	return f.AddScheduleReplaceExisting(expr, name, fn)
}
func (f *fakeScheduler) GetNextRun(name string) time.Time { return time.Time{} }

// Test createScheduler: ensure the scheduled function performs discovery and manager calls
func TestCreateScheduler_DiscoveryAndManagerInteraction(t *testing.T) {
	calledReconcile := false
	var gotProjects []string
	calledUpdate := false

	mgr := &fakeManager{}
	mgr.reconcileProjectsFn = func(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error {
		calledReconcile = true
		gotProjects = projects
		return nil
	}
	mgr.updateProjectStatusBatchedFn = func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
		calledUpdate = true
		// run the predicate on a sample project to ensure no panic
		_ = fn(api.ProjectStatus{Name: "p1", Status: api.JobStatusRunning})
		return nil
	}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status: api.RenovateJobStatus{
				Projects: []api.ProjectStatus{{Name: "p1", Status: api.JobStatusScheduled}},
			},
		}, nil
	}

	disc := &fakeDiscovery{}
	disc.discoverFn = func(ctx context.Context, job *api.RenovateJob) ([]string, error) {
		return []string{"p1", "p2"}, nil
	}

	sched := &fakeScheduler{}

	reconciler := &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: disc,
	}

	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	// create the schedule (stores the function)
	createScheduler(logger, renovateJob, reconciler)

	if !sched.addCalled {
		t.Fatalf("expected scheduler AddScheduleReplaceExisting to be called")
	}
	if sched.storedFn == nil {
		t.Fatalf("expected stored schedule function to be set")
	}

	// invoke the scheduled function and assert interactions
	sched.storedFn()

	if !calledReconcile {
		t.Fatalf("expected ReconcileProjects to be called")
	}
	if !calledUpdate {
		t.Fatalf("expected UpdateProjectStatusBatched to be called")
	}
	if len(gotProjects) != 2 || gotProjects[0] != "p1" || gotProjects[1] != "p2" {
		t.Fatalf("unexpected projects discovered: %v", gotProjects)
	}
}

// Test: when Discovery returns an error, the scheduled function should abort and not call manager methods
func TestCreateScheduler_DiscoveryErrorAborts(t *testing.T) {
	calledReconcile := false
	calledUpdate := false

	mgr := &fakeManager{}
	mgr.reconcileProjectsFn = func(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error {
		calledReconcile = true
		return nil
	}
	mgr.updateProjectStatusBatchedFn = func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
		calledUpdate = true
		return nil
	}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
	}

	disc := &fakeDiscovery{}
	disc.discoverFn = func(ctx context.Context, job *api.RenovateJob) ([]string, error) {
		return nil, fmt.Errorf("discover boom")
	}

	sched := &fakeScheduler{}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	createScheduler(logger, renovateJob, reconciler)
	if sched.storedFn == nil {
		t.Fatalf("expected stored function to be set")
	}
	// invoke
	sched.storedFn()

	if calledReconcile {
		t.Fatalf("expected ReconcileProjects NOT to be called when discovery fails")
	}
	if calledUpdate {
		t.Fatalf("expected UpdateProjectStatusBatched NOT to be called when discovery fails")
	}
}

// Test: when ReconcileProjects returns an error, the scheduled function should abort and not call UpdateProjectStatusBatched
func TestCreateScheduler_ReconcileErrorAborts(t *testing.T) {
	calledReconcile := false
	calledUpdate := false

	mgr := &fakeManager{}
	mgr.reconcileProjectsFn = func(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error {
		calledReconcile = true
		return fmt.Errorf("reconcile boom")
	}
	mgr.updateProjectStatusBatchedFn = func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
		calledUpdate = true
		return nil
	}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
	}

	disc := &fakeDiscovery{}
	disc.discoverFn = func(ctx context.Context, job *api.RenovateJob) ([]string, error) {
		return []string{"p1"}, nil
	}

	sched := &fakeScheduler{}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	createScheduler(logger, renovateJob, reconciler)
	if sched.storedFn == nil {
		t.Fatalf("expected stored function to be set")
	}
	// invoke
	sched.storedFn()

	if !calledReconcile {
		t.Fatalf("expected ReconcileProjects to be called")
	}
	if calledUpdate {
		t.Fatalf("expected UpdateProjectStatusBatched NOT to be called when reconcile fails")
	}
}

// Test: when UpdateProjectStatusBatched returns an error, it should be invoked and handled
func TestCreateScheduler_UpdateProjectStatusBatchedError(t *testing.T) {
	calledReconcile := false
	calledUpdate := false

	mgr := &fakeManager{}
	mgr.reconcileProjectsFn = func(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error {
		calledReconcile = true
		return nil
	}
	mgr.updateProjectStatusBatchedFn = func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
		calledUpdate = true
		return fmt.Errorf("update batched boom")
	}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
	}

	disc := &fakeDiscovery{}
	disc.discoverFn = func(ctx context.Context, job *api.RenovateJob) ([]string, error) {
		return []string{"p1"}, nil
	}

	sched := &fakeScheduler{}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	createScheduler(logger, renovateJob, reconciler)
	if sched.storedFn == nil {
		t.Fatalf("expected stored function to be set")
	}

	// invoke
	sched.storedFn()

	if !calledReconcile {
		t.Fatalf("expected ReconcileProjects to be called")
	}
	if !calledUpdate {
		t.Fatalf("expected UpdateProjectStatusBatched to be called")
	}
}

// Test: when Scheduler.AddScheduleReplaceExisting returns an error, createScheduler should not panic and should log the error
func TestCreateScheduler_SchedulerAddError(t *testing.T) {
	mgr := &fakeManager{}
	mgr.reconcileProjectsFn = func(ctx context.Context, job crdManager.RenovateJobIdentifier, projects []string) error {
		return nil
	}
	mgr.updateProjectStatusBatchedFn = func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) error {
		return nil
	}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
	}

	disc := &fakeDiscovery{}
	disc.discoverFn = func(ctx context.Context, job *api.RenovateJob) ([]string, error) { return []string{}, nil }

	sched := &fakeScheduler{addErr: fmt.Errorf("add boom")}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	// should not panic
	createScheduler(logger, renovateJob, reconciler)

	// AddScheduleReplaceExisting was called but returned an error, so storedFn should be set but Add returned error
	if !sched.addCalled {
		t.Fatalf("expected AddScheduleReplaceExisting to be called")
	}
	// Because AddScheduleReplaceExisting returned an error, we expect storedFn to be set (it is stored before error)
	if sched.storedFn == nil {
		t.Fatalf("expected storedFn to be present even if add failed")
	}
}

// controller-runtime Manager expects a Scheduler interface; create the rest of the
// methods as no-ops to satisfy the interface if any exist. If the real
// scheduler.Scheduler contains more methods, tests only need the two above.

// Test: when the manager returns a RenovateJob, Reconcile should call Scheduler.AddScheduleReplaceExisting
func TestReconcile_CreateSchedule(t *testing.T) {
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       api.RenovateJobSpec{Schedule: "*/5 * * * *"},
		}, nil
	}

	sched := &fakeScheduler{}

	reconciler := &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: &fakeDiscovery{},
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
	res, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sched.addCalled {
		t.Fatalf("expected AddScheduleReplaceExisting to be called")
	}
	expectedName := "test-default"
	if sched.addedName != expectedName {
		t.Fatalf("expected schedule name %s, got %s", expectedName, sched.addedName)
	}
	if res.RequeueAfter != 1*time.Minute {
		t.Fatalf("expected RequeueAfter 1m, got %v", res.RequeueAfter)
	}
}

// Test: when the manager returns NotFound, Reconcile should call Scheduler.RemoveSchedule
func TestReconcile_RemoveScheduleOnNotFound(t *testing.T) {
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return nil, kerrors.NewNotFound(schema.GroupResource{Group: "v1alpha1", Resource: "renovatejobs"}, name)
	}

	sched := &fakeScheduler{}

	reconciler := &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: &fakeDiscovery{},
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
	res, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sched.removeCalled {
		t.Fatalf("expected RemoveSchedule to be called")
	}
	expectedName := "test-default"
	if sched.removedName != expectedName {
		t.Fatalf("expected removed name %s, got %s", expectedName, sched.removedName)
	}
	if res.RequeueAfter != 1*time.Minute {
		t.Fatalf("expected RequeueAfter 1m, got %v", res.RequeueAfter)
	}
}

// Test: when the manager returns an error (not NotFound), Reconcile should return the error
func TestReconcile_ReturnsErrorOnManagerFailure(t *testing.T) {
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return nil, fmt.Errorf("boom")
	}

	sched := &fakeScheduler{}

	reconciler := &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: &fakeDiscovery{},
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
