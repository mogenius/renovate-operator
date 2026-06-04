package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"

	"renovate-operator/internal/types"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// fakeManager implements the full RenovateJobManager interface but only the
// methods used by the reconciler are given meaningful behaviour in tests.
type fakeManager struct {
	getFn                        func(ctx context.Context, name, namespace string) (*api.RenovateJob, error)
	reconcileProjectsFn          func(ctx context.Context, job *api.RenovateJob, projects []string) error
	updateProjectStatusBatchedFn func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
}

func (f *fakeManager) ListRenovateJobs(ctx context.Context) ([]crdManager.RenovateJobIdentifier, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeManager) ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error) {
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
func (f *fakeManager) UpdateProjectStatus(ctx context.Context, project string, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	if f.updateProjectStatusBatchedFn != nil {
		return f.updateProjectStatusBatchedFn(ctx, fn, job, status)
	}
	return nil
}
func (m *fakeManager) UpdateExecutionOptions(ctx context.Context, jobId crdManager.RenovateJobIdentifier, options *api.RenovateExecutionOptions) error {
	return nil
}
func (f *fakeManager) CancelProjectJob(ctx context.Context, project string, job crdManager.RenovateJobIdentifier) error {
	return nil
}
func (f *fakeManager) GetProjectsByStatus(ctx context.Context, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeManager) ReconcileProjects(ctx context.Context, job *api.RenovateJob, projects []string) error {
	if f.reconcileProjectsFn != nil {
		return f.reconcileProjectsFn(ctx, job, projects)
	}
	return nil
}
func (f *fakeManager) GetLogsForProject(ctx context.Context, job crdManager.RenovateJobIdentifier, project string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
func (f *fakeManager) UpdateProjectConfigStatus(ctx context.Context, project string, job crdManager.RenovateJobIdentifier, status *string) error {
	return nil
}

func (f *fakeManager) IsWebhookTokenValid(ctx context.Context, job crdManager.RenovateJobIdentifier, token string) (bool, error) {
	return true, nil
}
func (f *fakeManager) IsWebhookSignatureValid(ctx context.Context, job crdManager.RenovateJobIdentifier, signature string, body []byte) (bool, error) {
	return true, nil
}

type fakeWebhookSync struct{}

func (f *fakeWebhookSync) EnsureSyncer(ctx context.Context, logger logr.Logger, renovateJob *api.RenovateJob) {
}
func (f *fakeWebhookSync) RunSync(ctx context.Context, logger logr.Logger, jobName, jobNamespace string) {
}
func (f *fakeWebhookSync) RemoveSyncer(name string) {}

type fakeGithubAppToken struct{}

func (worker *fakeGithubAppToken) EnsureToken(ctx context.Context, job *api.RenovateJob) error {
	return nil
}
func (worker *fakeGithubAppToken) CreateGithubAppTokenFromJob(job *api.RenovateJob) (string, error) {
	return "", nil
}
func (worker *fakeGithubAppToken) CreateGithubAppToken(appID, installationID, pem, githubApi string) (string, error) {
	return "", nil
}

type fakeDiscovery struct {
	createDiscoveryJobFn func(ctx context.Context, job api.RenovateJob) (string, error)
}

func (f *fakeDiscovery) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob, scheduleAfterCompletion bool) (string, error) {
	if f.createDiscoveryJobFn != nil {
		return f.createDiscoveryJobFn(ctx, renovateJob)
	}
	return "gen-1", nil
}
func (f *fakeDiscovery) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob, generation string) (api.RenovateProjectStatus, error) {
	return api.JobStatusCompleted, nil
}
func (f *fakeDiscovery) ProcessDiscoveryJobResult(ctx context.Context, k8sJob *batchv1.Job, renovateJobName string, namespace string) error {
	return nil
}

type fakeScheduler struct {
	addedExpr    string
	addedName    string
	addCalled    bool
	removedNames []string
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
	f.removedNames = append(f.removedNames, name)
	f.removeCalled = true
}

// implement remaining methods of scheduler.Scheduler as no-ops for tests
func (f *fakeScheduler) Start() {}
func (f *fakeScheduler) Stop()  {}
func (f *fakeScheduler) AddSchedule(expr string, name string, fn func()) error {
	// behave like AddScheduleReplaceExisting for tests
	return f.AddScheduleReplaceExisting(expr, name, fn)
}
func (f *fakeScheduler) GetNextRunOnSchedule(schedule string) time.Time { return time.Time{} }

// Test createScheduler: ensure the scheduled function creates a discovery job
func TestCreateScheduler_DiscoveryAndManagerInteraction(t *testing.T) {
	calledCreate := false

	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status: api.RenovateJobStatus{
				Projects: []api.ProjectStatus{{Name: "p1", Status: api.JobStatusScheduled}},
			},
		}, nil
	}

	disc := &fakeDiscovery{}
	disc.createDiscoveryJobFn = func(ctx context.Context, job api.RenovateJob) (string, error) {
		calledCreate = true
		return "gen-1", nil
	}

	sched := &fakeScheduler{}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc, WebhookSync: &fakeWebhookSync{}}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	createScheduler(logger, renovateJob, reconciler)

	if !sched.addCalled {
		t.Fatalf("expected scheduler AddScheduleReplaceExisting to be called")
	}
	if sched.storedFn == nil {
		t.Fatalf("expected stored schedule function to be set")
	}

	sched.storedFn()

	if !calledCreate {
		t.Fatalf("expected CreateDiscoveryJob to be called")
	}
}

// Test: when CreateDiscoveryJob returns an error, the scheduled function should abort
func TestCreateScheduler_DiscoveryErrorAborts(t *testing.T) {
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
	}

	disc := &fakeDiscovery{}
	disc.createDiscoveryJobFn = func(ctx context.Context, job api.RenovateJob) (string, error) {
		return "", fmt.Errorf("create boom")
	}

	sched := &fakeScheduler{}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc, WebhookSync: &fakeWebhookSync{}}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	createScheduler(logger, renovateJob, reconciler)
	if sched.storedFn == nil {
		t.Fatalf("expected stored function to be set")
	}
	// should not panic
	sched.storedFn()
}

// Test: the scheduled function uses the freshly fetched RenovateJob, not the one captured at schedule-creation time
func TestCreateScheduler_UsesFreshRenovateJob(t *testing.T) {
	var discoveredJob *api.RenovateJob

	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       api.RenovateJobSpec{Schedule: "*/1 * * * *", Image: "renovate/renovate:39"},
		}, nil
	}

	disc := &fakeDiscovery{}
	disc.createDiscoveryJobFn = func(ctx context.Context, job api.RenovateJob) (string, error) {
		discoveredJob = &job
		return "gen-1", nil
	}

	sched := &fakeScheduler{}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: disc, WebhookSync: &fakeWebhookSync{}}
	logger := logr.Discard()

	originalJob := &api.RenovateJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       api.RenovateJobSpec{Schedule: "*/1 * * * *", Image: "renovate/renovate:38"},
	}
	createScheduler(logger, originalJob, reconciler)
	sched.storedFn()

	if discoveredJob == nil {
		t.Fatalf("expected CreateDiscoveryJob to be called")
	}
	if discoveredJob.Spec.Image != "renovate/renovate:39" {
		t.Fatalf("expected fresh image 'renovate/renovate:39', got '%s'", discoveredJob.Spec.Image)
	}
}

// Test: when Scheduler.AddScheduleReplaceExisting returns an error, createScheduler should not panic
func TestCreateScheduler_SchedulerAddError(t *testing.T) {
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
	}

	sched := &fakeScheduler{addErr: fmt.Errorf("add boom")}
	reconciler := &RenovateJobReconciler{Manager: mgr, Scheduler: sched, Discovery: &fakeDiscovery{}, WebhookSync: &fakeWebhookSync{}}
	logger := logr.Discard()
	renovateJob := &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: api.RenovateJobSpec{Schedule: "*/1 * * * *"}}

	createScheduler(logger, renovateJob, reconciler)

	if !sched.addCalled {
		t.Fatalf("expected AddScheduleReplaceExisting to be called")
	}
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
		Manager:        mgr,
		Scheduler:      sched,
		Discovery:      &fakeDiscovery{},
		WebhookSync: &fakeWebhookSync{},
		GithubApp:      &fakeGithubAppToken{},
	}

	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "test", Namespace: "default"}}
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
		Manager:        mgr,
		Scheduler:      sched,
		Discovery:      &fakeDiscovery{},
		WebhookSync: &fakeWebhookSync{},
		GithubApp:      &fakeGithubAppToken{},
	}

	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "test", Namespace: "default"}}
	res, err := reconciler.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sched.removeCalled {
		t.Fatalf("expected RemoveSchedule to be called")
	}
	expectedName := "test-default"
	if len(sched.removedNames) != 1 || sched.removedNames[0] != expectedName {
		t.Fatalf("expected removed names [%s], got %v", expectedName, sched.removedNames)
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
		Manager:        mgr,
		Scheduler:      sched,
		Discovery:      &fakeDiscovery{},
		WebhookSync: &fakeWebhookSync{},
	}

	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "test", Namespace: "default"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
