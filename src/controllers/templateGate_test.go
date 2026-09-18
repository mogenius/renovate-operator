package controllers

import (
	"context"
	"fmt"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func templateGateReconciler(t *testing.T, mgr *fakeManager) (*RenovateJobReconciler, *fakeScheduler) {
	t.Helper()
	sched := &fakeScheduler{}
	return &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: &fakeDiscovery{},
		GithubApp: &fakeGithubAppToken{},
		K8sClient: buildFakeK8sClient(t),
		Policy:    policy.Policy{Disabled: true},
	}, sched
}

func TestReconcileRefusesDanglingTemplateRef(t *testing.T) {
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		job := &api.RenovateJob{Spec: api.RenovateJobSpec{TemplateRef: &api.RenovateJobTemplateRef{Name: "missing"}}}
		job.ObjectMeta = metav1.ObjectMeta{Name: name, Namespace: namespace}
		return job, nil
	}
	mgr.resolveEffectiveFn = func(_ context.Context, _ *api.RenovateJob) (*api.RenovateJob, error) {
		return nil, fmt.Errorf("resolving RenovateJobTemplate %q: not found", "missing")
	}

	r, sched := templateGateReconciler(t, mgr)
	reconcileOnce(t, r)

	if sched.addCalled {
		t.Error("a job with a dangling templateRef must not be scheduled")
	}
	if len(mgr.acceptedCalls) != 1 || mgr.acceptedCalls[0].accepted {
		t.Fatalf("expected one Accepted=False write, got %#v", mgr.acceptedCalls)
	}
	if mgr.acceptedCalls[0].reason != policy.ReasonTemplateNotFound {
		t.Errorf("expected reason %q, got %q", policy.ReasonTemplateNotFound, mgr.acceptedCalls[0].reason)
	}
}

func TestReconcileRefusesSpecIncompleteAfterMerge(t *testing.T) {
	mgr := &fakeManager{}
	// The effective spec still lacks image and parallelism after inheritance.
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		job := &api.RenovateJob{Spec: api.RenovateJobSpec{
			Schedule: "*/5 * * * *",
			Provider: &api.RenovateProvider{Name: "github"},
		}}
		job.ObjectMeta = metav1.ObjectMeta{Name: name, Namespace: namespace}
		return job, nil
	}

	r, sched := templateGateReconciler(t, mgr)
	reconcileOnce(t, r)

	if sched.addCalled {
		t.Error("an incomplete job must not be scheduled")
	}
	if len(mgr.acceptedCalls) != 1 || mgr.acceptedCalls[0].accepted {
		t.Fatalf("expected one Accepted=False write, got %#v", mgr.acceptedCalls)
	}
	if mgr.acceptedCalls[0].reason != policy.ReasonIncompleteSpec {
		t.Errorf("expected reason %q, got %q", policy.ReasonIncompleteSpec, mgr.acceptedCalls[0].reason)
	}
}
