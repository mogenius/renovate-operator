package crdmanager

import (
	"context"
	"sync"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// countingClient records status writes so a test can prove the manager does not
// rewrite an unchanged condition.
type countingClient struct {
	client.WithWatch
	statusUpdates *int
}

func (c countingClient) Status() client.SubResourceWriter {
	return countingStatusWriter{SubResourceWriter: c.WithWatch.Status(), statusUpdates: c.statusUpdates}
}

type countingStatusWriter struct {
	client.SubResourceWriter
	statusUpdates *int
}

func (w countingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	*w.statusUpdates++
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func conditionManager(t *testing.T, job *api.RenovateJob) (*renovateJobManager, *int) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	writes := 0
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(job).
		WithStatusSubresource(job).
		WithInterceptorFuncs(interceptor.Funcs{}).
		Build()

	return &renovateJobManager{
		client: countingClient{WithWatch: base, statusUpdates: &writes},
		logger: logr.Discard(),
		lock:   &sync.RWMutex{},
		policy: policy.Policy{},
	}, &writes
}

func conditionJob() *api.RenovateJob {
	job := &api.RenovateJob{}
	job.TypeMeta = metav1.TypeMeta{APIVersion: api.GroupVersion.String(), Kind: "RenovateJob"}
	job.ObjectMeta = metav1.ObjectMeta{Name: "job1", Namespace: "default", Generation: 3}
	job.Spec = api.RenovateJobSpec{Schedule: "*/5 * * * *"}
	return job
}

func loadCondition(t *testing.T, mgr *renovateJobManager, job *api.RenovateJob) *metav1.Condition {
	t.Helper()
	stored, err := loadRenovateJob(context.Background(), job.Name, job.Namespace, mgr.client)
	if err != nil {
		t.Fatalf("failed to reload job: %v", err)
	}
	return meta.FindStatusCondition(stored.Status.Conditions, api.ConditionAccepted)
}

func TestSetAcceptedConditionRecordsRefusal(t *testing.T) {
	job := conditionJob()
	mgr, writes := conditionManager(t, job)
	id := RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}

	err := mgr.SetAcceptedCondition(context.Background(), id, false,
		policy.ReasonDestinationNotAllowed, `spec.provider.endpoint: host "attacker.example.net" is not allowed`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := loadCondition(t, mgr, job)
	if cond == nil {
		t.Fatal("expected the Accepted condition to be set")
		return
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected status False, got %s", cond.Status)
	}
	if cond.Reason != policy.ReasonDestinationNotAllowed {
		t.Errorf("expected reason %q, got %q", policy.ReasonDestinationNotAllowed, cond.Reason)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("expected the condition to record the observed generation, got %d", cond.ObservedGeneration)
	}
	if *writes != 1 {
		t.Errorf("expected exactly one status write, got %d", *writes)
	}
}

// The reconciler requeues every minute, so an unchanged condition must not be
// rewritten: otherwise resourceVersion churns forever and every operator in the
// cluster re-reconciles on its own writes.
func TestSetAcceptedConditionIsIdempotent(t *testing.T) {
	job := conditionJob()
	mgr, writes := conditionManager(t, job)
	id := RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}
	ctx := context.Background()

	for range 5 {
		if err := mgr.SetAcceptedCondition(ctx, id, false, policy.ReasonDestinationNotAllowed, "same message"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if *writes != 1 {
		t.Errorf("expected repeated identical calls to write once, got %d writes", *writes)
	}
}

func TestSetAcceptedConditionFlipsBackToAccepted(t *testing.T) {
	job := conditionJob()
	mgr, writes := conditionManager(t, job)
	id := RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}
	ctx := context.Background()

	if err := mgr.SetAcceptedCondition(ctx, id, false, policy.ReasonDestinationNotAllowed, "denied"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mgr.SetAcceptedCondition(ctx, id, true, policy.ReasonPolicySatisfied, "ok now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := loadCondition(t, mgr, job)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("expected the condition to flip to True, got %+v", cond)
	}
	if cond.Reason != policy.ReasonPolicySatisfied {
		t.Errorf("expected reason %q, got %q", policy.ReasonPolicySatisfied, cond.Reason)
	}
	if *writes != 2 {
		t.Errorf("expected one write per genuine change, got %d", *writes)
	}
}

// A message change alone still has to be persisted: the host to add is in the
// message, so a stale one would send the user after the wrong value.
func TestSetAcceptedConditionPersistsMessageChange(t *testing.T) {
	job := conditionJob()
	mgr, writes := conditionManager(t, job)
	id := RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}
	ctx := context.Background()

	if err := mgr.SetAcceptedCondition(ctx, id, false, policy.ReasonDestinationNotAllowed, "host a.example.net is not allowed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mgr.SetAcceptedCondition(ctx, id, false, policy.ReasonDestinationNotAllowed, "host b.example.net is not allowed"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := loadCondition(t, mgr, job)
	if cond == nil || cond.Message != "host b.example.net is not allowed" {
		t.Fatalf("expected the message to be updated, got %+v", cond)
	}
	if *writes != 2 {
		t.Errorf("expected the message change to be written, got %d writes", *writes)
	}
}
