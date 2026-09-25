package crdmanager

import (
	"context"
	"sync"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func suspendManager(t *testing.T, objs ...client.Object) *renovateJobManager {
	t.Helper()
	return &renovateJobManager{
		client: resolveClient(t, objs...),
		logger: logr.Discard(),
		lock:   &sync.RWMutex{},
		policy: policy.Policy{},
	}
}

func suspendJob(templateRef *api.RenovateJobTemplateRef) *api.RenovateJob {
	job := &api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: "job1", Namespace: "renovate"}
	job.Spec = api.RenovateJobSpec{Schedule: "*/5 * * * *", TemplateRef: templateRef}
	return job
}

// storedSuspend returns the job's own spec.suspend as stored, and its effective value.
func storedSuspend(t *testing.T, mgr *renovateJobManager, job *api.RenovateJob) (*bool, bool) {
	t.Helper()
	stored, err := loadRenovateJob(context.Background(), job.Name, job.Namespace, mgr.client)
	if err != nil {
		t.Fatalf("failed to reload job: %v", err)
	}
	effective, err := resolveEffectiveJob(context.Background(), stored, mgr.client)
	if err != nil {
		t.Fatalf("failed to resolve job: %v", err)
	}
	if stored.Spec.Schedule != job.Spec.Schedule {
		t.Errorf("schedule changed to %q: only suspend is the UI's to rewrite", stored.Spec.Schedule)
	}
	return stored.Spec.Suspend, effective.Spec.GetSuspend()
}

func TestSetSuspendWithoutTemplate(t *testing.T) {
	job := suspendJob(nil)
	mgr := suspendManager(t, job)
	jobId := RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}

	if err := mgr.SetSuspend(context.Background(), jobId, true); err != nil {
		t.Fatalf("SetSuspend(true) failed: %v", err)
	}
	if own, effective := storedSuspend(t, mgr, job); own == nil || !*own || !effective {
		t.Fatalf("expected an explicit suspend: true, got own=%v effective=%v", own, effective)
	}

	// Resuming clears the field rather than writing false: the default already is.
	if err := mgr.SetSuspend(context.Background(), jobId, false); err != nil {
		t.Fatalf("SetSuspend(false) failed: %v", err)
	}
	if own, effective := storedSuspend(t, mgr, job); own != nil || effective {
		t.Fatalf("expected the field cleared, got own=%v effective=%v", own, effective)
	}
}

// A job suspended by its template needs an explicit false to resume, and goes back
// to following the template when suspended again.
func TestSetSuspendOverridesATemplateOnlyWhenNeeded(t *testing.T) {
	template := &api.RenovateJobTemplate{}
	template.ObjectMeta = metav1.ObjectMeta{Name: "paused", Namespace: "renovate"}
	template.Spec = api.RenovateJobSpec{Suspend: new(true)}
	job := suspendJob(&api.RenovateJobTemplateRef{Name: "paused"})
	mgr := suspendManager(t, template, job)
	jobId := RenovateJobIdentifier{Name: job.Name, Namespace: job.Namespace}

	if err := mgr.SetSuspend(context.Background(), jobId, false); err != nil {
		t.Fatalf("SetSuspend(false) failed: %v", err)
	}
	if own, effective := storedSuspend(t, mgr, job); own == nil || *own || effective {
		t.Fatalf("expected an explicit suspend: false over the template, got own=%v effective=%v", own, effective)
	}

	if err := mgr.SetSuspend(context.Background(), jobId, true); err != nil {
		t.Fatalf("SetSuspend(true) failed: %v", err)
	}
	if own, effective := storedSuspend(t, mgr, job); own != nil || !effective {
		t.Fatalf("expected the field cleared so the template applies, got own=%v effective=%v", own, effective)
	}
}

func TestSetSuspendReportsAMissingJob(t *testing.T) {
	mgr := suspendManager(t, suspendJob(nil))

	err := mgr.SetSuspend(context.Background(), RenovateJobIdentifier{Name: "gone", Namespace: "renovate"}, true)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected a NotFound error, got %v", err)
	}
}
