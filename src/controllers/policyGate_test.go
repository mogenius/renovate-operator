package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func gateJob(endpoint string) *api.RenovateJob {
	job := &api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: "test", Namespace: "default"}
	job.Spec = api.RenovateJobSpec{
		Schedule: "*/5 * * * *",
		Provider: &api.RenovateProvider{Name: "github", Endpoint: endpoint},
	}
	return job
}

func gateReconciler(t *testing.T, job *api.RenovateJob, allowedHosts ...string) (*RenovateJobReconciler, *fakeManager, *fakeScheduler) {
	t.Helper()
	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return job, nil
	}
	sched := &fakeScheduler{}
	return &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: &fakeDiscovery{},
		GithubApp: &fakeGithubAppToken{},
		K8sClient: buildFakeK8sClient(t),
		Policy:    policy.Policy{AllowedHosts: allowedHosts},
	}, mgr, sched
}

func reconcileOnce(t *testing.T, r *RenovateJobReconciler) ctrl.Result {
	t.Helper()
	req := ctrl.Request{Name: "test", Namespace: "default"}
	res, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return res
}

func TestReconcileRefusesJobWithForeignEndpoint(t *testing.T) {
	job := gateJob("https://attacker.example.net")
	r, mgr, sched := gateReconciler(t, job, "api.github.com")

	res := reconcileOnce(t, r)

	if sched.addCalled {
		t.Error("a refused job must not be scheduled")
	}
	if len(mgr.acceptedCalls) != 1 {
		t.Fatalf("expected one condition write, got %d", len(mgr.acceptedCalls))
	}
	call := mgr.acceptedCalls[0]
	if call.accepted {
		t.Error("expected Accepted=False")
	}
	if call.reason != policy.ReasonDestinationNotAllowed {
		t.Errorf("expected reason %q, got %q", policy.ReasonDestinationNotAllowed, call.reason)
	}
	// The message is what a user sees on `kubectl describe` and in the UI banner, so
	// it has to name the offending host.
	if !strings.Contains(call.message, "attacker.example.net") {
		t.Errorf("expected the message to name the host, got %q", call.message)
	}
	if res.RequeueAfter != 1*time.Minute {
		t.Errorf("expected the job to be retried, got %v", res.RequeueAfter)
	}
}

// A job that was valid and has since been edited into a refused state must lose its
// existing cron entry, otherwise the schedule registered earlier keeps firing.
func TestReconcileRemovesScheduleWhenJobBecomesRefused(t *testing.T) {
	job := gateJob("https://attacker.example.net")
	r, _, sched := gateReconciler(t, job, "api.github.com")

	reconcileOnce(t, r)

	if !sched.removeCalled {
		t.Fatal("expected the schedule of a refused job to be removed")
	}
	if len(sched.removedNames) != 1 || sched.removedNames[0] != "test-default" {
		t.Errorf("expected [test-default] to be removed, got %v", sched.removedNames)
	}
}

func TestReconcileAcceptsAllowlistedJob(t *testing.T) {
	job := gateJob("https://gitlab.example.com/api/v4")
	r, mgr, sched := gateReconciler(t, job, "gitlab.example.com")

	reconcileOnce(t, r)

	if !sched.addCalled {
		t.Error("expected an accepted job to be scheduled")
	}
	if sched.removeCalled {
		t.Error("an accepted job must keep its schedule")
	}
	if len(mgr.acceptedCalls) != 1 || !mgr.acceptedCalls[0].accepted {
		t.Fatalf("expected Accepted=True to be recorded, got %+v", mgr.acceptedCalls)
	}
	if mgr.acceptedCalls[0].reason != policy.ReasonPolicySatisfied {
		t.Errorf("expected reason %q, got %q", policy.ReasonPolicySatisfied, mgr.acceptedCalls[0].reason)
	}
}

// An endpoint the CRD pattern would reject can still reach the operator on an
// object written before that validation existed.
func TestReconcileReportsInvalidURLDistinctly(t *testing.T) {
	job := gateJob("not-a-url")
	r, mgr, _ := gateReconciler(t, job, "api.github.com")

	reconcileOnce(t, r)

	if len(mgr.acceptedCalls) != 1 {
		t.Fatalf("expected one condition write, got %d", len(mgr.acceptedCalls))
	}
	if got := mgr.acceptedCalls[0].reason; got != policy.ReasonInvalidDestinationURL {
		t.Errorf("expected reason %q, got %q", policy.ReasonInvalidDestinationURL, got)
	}
}

// With the policy engine off, a job that violates every operator-side rule still runs,
// and the condition says why it was accepted so the state is visible on the resource.
func TestReconcileAcceptsEverythingWhenPolicyDisabled(t *testing.T) {
	job := gateJob("https://attacker.example.net")
	job.Spec.Image = "ghcr.io/attacker/renovate:latest"
	job.Spec.ServiceAccount = &api.RenovateJobServiceAccount{Name: "renovate-operator"}

	mgr := &fakeManager{}
	mgr.getFn = func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
		return job, nil
	}
	sched := &fakeScheduler{}
	r := &RenovateJobReconciler{
		Manager:   mgr,
		Scheduler: sched,
		Discovery: &fakeDiscovery{},
		GithubApp: &fakeGithubAppToken{},
		K8sClient: buildFakeK8sClient(t),
		Policy:    policy.Policy{Disabled: true},
	}

	reconcileOnce(t, r)

	if !sched.addCalled {
		t.Error("expected the job to be scheduled with enforcement off")
	}
	if len(mgr.acceptedCalls) != 1 || !mgr.acceptedCalls[0].accepted {
		t.Fatalf("expected Accepted=True, got %+v", mgr.acceptedCalls)
	}
	if got := mgr.acceptedCalls[0].reason; got != policy.ReasonPolicyDisabled {
		t.Errorf("expected reason %q so the unsecured state is visible, got %q", policy.ReasonPolicyDisabled, got)
	}
	if !strings.Contains(mgr.acceptedCalls[0].message, "disabled") {
		t.Errorf("expected the message to say the job was not checked, got %q", mgr.acceptedCalls[0].message)
	}
}
