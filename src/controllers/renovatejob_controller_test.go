package controllers

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"

	"renovate-operator/internal/types"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
func (f *fakeManager) StreamLogsForProject(ctx context.Context, job crdManager.RenovateJobIdentifier, project string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
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
func (f *fakeWebhookSync) RunSync(ctx context.Context, logger logr.Logger, jobId crdManager.RenovateJobIdentifier) {
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

func (f *fakeDiscovery) CreateDiscoveryJob(ctx context.Context, renovateJob api.RenovateJob, options renovate.DiscoveryJobOptions) (string, error) {
	if f.createDiscoveryJobFn != nil {
		return f.createDiscoveryJobFn(ctx, renovateJob)
	}
	return "gen-1", nil
}
func (f *fakeDiscovery) GetDiscoveryJobStatus(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
	return api.JobStatusCompleted, nil
}
func (f *fakeDiscovery) ProcessDiscoveryJobResult(ctx context.Context, k8sJob *batchv1.Job, renovateJobId crdManager.RenovateJobIdentifier) error {
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

func (f *fakeScheduler) AddScheduleReplaceExisting(expr string, namespace, job string, fct func()) error {
	f.addedExpr = expr
	f.addedName = job + "-" + namespace
	f.addCalled = true
	f.storedFn = fct
	return f.addErr
}
func (f *fakeScheduler) RemoveSchedule(namespace, job string) {
	f.removedNames = append(f.removedNames, job+"-"+namespace)
	f.removeCalled = true
}

// implement remaining methods of scheduler.Scheduler as no-ops for tests
func (f *fakeScheduler) Start() {}
func (f *fakeScheduler) Stop()  {}
func (f *fakeScheduler) AddSchedule(expr string, namespace, job string, fn func()) error {
	// behave like AddScheduleReplaceExisting for tests
	return f.AddScheduleReplaceExisting(expr, namespace, job, fn)
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
		Manager:     mgr,
		Scheduler:   sched,
		Discovery:   &fakeDiscovery{},
		WebhookSync: &fakeWebhookSync{},
		GithubApp:   &fakeGithubAppToken{},
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
		Manager:     mgr,
		Scheduler:   sched,
		Discovery:   &fakeDiscovery{},
		WebhookSync: &fakeWebhookSync{},
		GithubApp:   &fakeGithubAppToken{},
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

func buildFakeK8sClient(t *testing.T, objs ...crclient.Object) crclient.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func makeRenovateJob(name, namespace string, annotations map[string]string) *api.RenovateJob {
	return &api.RenovateJob{
		TypeMeta:   metav1.TypeMeta{APIVersion: "renovate-operator.mogenius.com/v1alpha1", Kind: "RenovateJob"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
		Spec:       api.RenovateJobSpec{Schedule: "*/5 * * * *"},
	}
}

// TestHandleAnnotationTriggers_Discovery verifies that the discovery annotation triggers
// CreateDiscoveryJob and is removed from the RenovateJob on success.
func TestHandleAnnotationTriggers_Discovery(t *testing.T) {
	discoveryTriggered := false
	disc := &fakeDiscovery{
		createDiscoveryJobFn: func(ctx context.Context, job api.RenovateJob) (string, error) {
			discoveryTriggered = true
			return "gen-1", nil
		},
	}

	renovateJob := makeRenovateJob("test", "default", map[string]string{
		crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_DISCOVERY: "true",
	})
	reconciler := &RenovateJobReconciler{
		Discovery: disc,
		Manager:   &fakeManager{},
		K8sClient: buildFakeK8sClient(t, renovateJob),
	}

	reconciler.handleAnnotationTriggers(context.Background(), logr.Discard(), renovateJob)

	if !discoveryTriggered {
		t.Fatal("expected CreateDiscoveryJob to be called")
	}
	if _, ok := renovateJob.Annotations[crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_DISCOVERY]; ok {
		t.Fatal("expected discovery annotation to be removed after processing")
	}
}

// TestHandleAnnotationTriggers_ScheduleAll verifies that the schedule-all annotation sets all
// non-running projects to Scheduled and is removed from the RenovateJob on success.
func TestHandleAnnotationTriggers_ScheduleAll(t *testing.T) {
	projects := []api.ProjectStatus{
		{Name: "org/a", Status: api.JobStatusCompleted},
		{Name: "org/b", Status: api.JobStatusRunning},
		{Name: "org/c", Status: api.JobStatusFailed},
	}
	var scheduled []string
	mgr := &fakeManager{
		updateProjectStatusBatchedFn: func(_ context.Context, fn func(api.ProjectStatus) bool, _ crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			for _, p := range projects {
				if fn(p) {
					scheduled = append(scheduled, p.Name)
				}
			}
			return nil
		},
	}

	renovateJob := makeRenovateJob("test", "default", map[string]string{
		crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE_ALL: "true",
	})
	reconciler := &RenovateJobReconciler{
		Discovery: &fakeDiscovery{},
		Manager:   mgr,
		K8sClient: buildFakeK8sClient(t, renovateJob),
	}

	reconciler.handleAnnotationTriggers(context.Background(), logr.Discard(), renovateJob)

	// org/b is Running and must be excluded; org/a and org/c must be scheduled
	if len(scheduled) != 2 {
		t.Fatalf("expected 2 projects scheduled, got %v", scheduled)
	}
	for _, want := range []string{"org/a", "org/c"} {
		found := false
		for _, got := range scheduled {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q to be scheduled, got %v", want, scheduled)
		}
	}
	if _, ok := renovateJob.Annotations[crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE_ALL]; ok {
		t.Fatal("expected schedule-all annotation to be removed after processing")
	}
}

// TestHandleAnnotationTriggers_Schedule verifies that the schedule annotation schedules only
// the listed non-running projects and is removed from the RenovateJob on success.
func TestHandleAnnotationTriggers_Schedule(t *testing.T) {
	projects := []api.ProjectStatus{
		{Name: "org/p1", Status: api.JobStatusCompleted},
		{Name: "org/p2", Status: api.JobStatusRunning}, // in list but running — must be excluded
		{Name: "org/p3", Status: api.JobStatusFailed},  // not in list — must be excluded
	}
	var scheduled []string
	mgr := &fakeManager{
		updateProjectStatusBatchedFn: func(_ context.Context, fn func(api.ProjectStatus) bool, _ crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			for _, p := range projects {
				if fn(p) {
					scheduled = append(scheduled, p.Name)
				}
			}
			return nil
		},
	}

	renovateJob := makeRenovateJob("test", "default", map[string]string{
		crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE: "org/p1, org/p2",
	})
	reconciler := &RenovateJobReconciler{
		Discovery: &fakeDiscovery{},
		Manager:   mgr,
		K8sClient: buildFakeK8sClient(t, renovateJob),
	}

	reconciler.handleAnnotationTriggers(context.Background(), logr.Discard(), renovateJob)

	if len(scheduled) != 1 || scheduled[0] != "org/p1" {
		t.Fatalf("expected only org/p1 to be scheduled, got %v", scheduled)
	}
	if _, ok := renovateJob.Annotations[crdManager.RENOVATEJOB_ANNOTATION_TRIGGER_SCHEDULE]; ok {
		t.Fatal("expected schedule annotation to be removed after processing")
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
		Manager:     mgr,
		Scheduler:   sched,
		Discovery:   &fakeDiscovery{},
		WebhookSync: &fakeWebhookSync{},
	}

	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "test", Namespace: "default"}}
	_, err := reconciler.Reconcile(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
