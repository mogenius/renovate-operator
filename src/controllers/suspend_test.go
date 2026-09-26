package controllers

import (
	"context"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/renovate"
	"renovate-operator/internal/types"

	"github.com/go-logr/logr/funcr"
)

// A job that is suspended after it was scheduled must lose its cron entry, or the
// schedule registered earlier keeps firing.
func TestReconcileRemovesScheduleWhileSuspended(t *testing.T) {
	job := gateJob("")
	job.Spec.Suspend = new(true)
	r, mgr, sched := gateReconciler(t, job, "api.github.com")

	reconcileOnce(t, r)

	if sched.addCalled {
		t.Error("a suspended job must not be scheduled")
	}
	if len(sched.removedNames) != 1 || sched.removedNames[0] != "test-default" {
		t.Errorf("expected [test-default] to be removed, got %v", sched.removedNames)
	}
	// Suspending is not a policy matter: the job stays accepted.
	if len(mgr.acceptedCalls) != 1 || !mgr.acceptedCalls[0].accepted {
		t.Errorf("expected Accepted=True to be recorded, got %+v", mgr.acceptedCalls)
	}
}

func TestReconcileSchedulesAgainOnceResumed(t *testing.T) {
	job := gateJob("")
	job.Spec.Suspend = new(true)
	r, _, sched := gateReconciler(t, job, "api.github.com")

	reconcileOnce(t, r)
	if sched.addCalled {
		t.Fatal("a suspended job must not be scheduled")
	}

	job.Spec.Suspend = nil
	reconcileOnce(t, r)

	if !sched.addCalled {
		t.Fatal("expected the schedule to come back once the job is resumed")
	}
	if sched.addedExpr != job.Spec.Schedule {
		t.Errorf("expected schedule %q, got %q", job.Spec.Schedule, sched.addedExpr)
	}
}

// Triggers that only queue projects keep working while suspended; the discovery one,
// which would start a Kubernetes Job, waits for the job to be resumed.
func TestHandleAnnotationTriggers_DiscoveryHeldWhileSuspended(t *testing.T) {
	projects := []crdManager.RenovateProjectStatus{
		{Name: "org/a", Status: api.JobStatusCompleted},
		{Name: "org/b", Status: api.JobStatusRunning},
	}
	var scheduled []string
	mgr := &fakeManager{
		updateProjectStatusBatchedFn: func(_ context.Context, fn func(crdManager.RenovateProjectStatus) bool, _ crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
			for _, p := range projects {
				if fn(p) {
					scheduled = append(scheduled, p.Name)
				}
			}
			return nil
		},
	}
	disc := &fakeDiscovery{
		createDiscoveryJobFn: func(ctx context.Context, job api.RenovateJob) (string, error) {
			return "", renovate.ErrRenovateJobSuspended
		},
	}

	renovateJob := makeRenovateJob("test", "default", map[string]string{
		api.TriggerDiscoveryAnnotationKey:   "true",
		api.TriggerScheduleAllAnnotationKey: "true",
	})
	renovateJob.Spec.Suspend = new(true)
	reconciler := &RenovateJobReconciler{
		Discovery: disc,
		Manager:   mgr,
		K8sClient: buildFakeK8sClient(t, renovateJob),
	}

	var logged []string
	logger := funcr.New(func(_, args string) { logged = append(logged, args) }, funcr.Options{Verbosity: 1})

	reconciler.handleAnnotationTriggers(context.Background(), logger, renovateJob, renovateJob)

	// Holding the trigger is the expected outcome, not a failure to report.
	for _, line := range logged {
		if strings.Contains(line, `"error"=`) {
			t.Errorf("expected no error to be logged, got %s", line)
		}
	}
	if len(scheduled) != 1 || scheduled[0] != "org/a" {
		t.Errorf("expected org/a to be queued, got %v", scheduled)
	}
	if _, ok := renovateJob.Annotations[api.TriggerScheduleAllAnnotationKey]; ok {
		t.Error("expected the schedule-all annotation to be removed after processing")
	}
	if renovateJob.Annotations[api.TriggerDiscoveryAnnotationKey] != "true" {
		t.Error("expected the discovery annotation to be kept until the job is resumed")
	}
}

// The gate reads the effective spec, so a template that suspends its jobs works
// without touching them.
func TestReconcileRemovesScheduleOfAJobSuspendedByItsTemplate(t *testing.T) {
	job := gateJob("")
	r, _, sched := gateReconciler(t, job, "api.github.com")
	r.Manager.(*fakeManager).resolveEffectiveFn = func(_ context.Context, raw *api.RenovateJob) (*api.RenovateJob, error) {
		effective := raw.DeepCopy()
		effective.Spec.Suspend = new(true)
		return effective, nil
	}

	reconcileOnce(t, r)

	if sched.addCalled {
		t.Error("a job suspended by its template must not be scheduled")
	}
	if len(sched.removedNames) != 1 || sched.removedNames[0] != "test-default" {
		t.Errorf("expected [test-default] to be removed, got %v", sched.removedNames)
	}
	if job.Spec.Suspend != nil {
		t.Errorf("the raw job must stay untouched, got suspend=%v", *job.Spec.Suspend)
	}
}
