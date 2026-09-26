package renovate

import (
	"context"
	"errors"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// The UI and the discovery annotation reach CreateDiscoveryJob without going through
// the reconciler, which only takes the schedule away.
func TestCreateDiscoveryJobRefusesSuspendedJob(t *testing.T) {
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	da := NewDiscoveryAgent(scheme, c, testLogger, nil, nil, gatePolicy())

	job := policyJob("job1", "")
	job.Spec.Suspend = new(true)
	_, err := da.CreateDiscoveryJob(context.Background(), job, DiscoveryJobOptions{TriggerAllProjects: true})
	if !errors.Is(err, ErrRenovateJobSuspended) {
		t.Fatalf("expected ErrRenovateJobSuspended, got %v", err)
	}

	if jobs := createdJobs(t, c); len(jobs) != 0 {
		t.Errorf("expected no Kubernetes Job to be created, got %d", len(jobs))
	}
}

// A suspended job keeps its queue, and does not hold up its siblings.
func TestAcceptedCandidatesSkipsSuspendedJob(t *testing.T) {
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	mgr := &fakeJobManager{
		getProjectsByStatusFn: func(_ context.Context, job crdManager.RenovateJobIdentifier, _ api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
			return []crdManager.RenovateProjectStatus{{Name: "org/" + job.Name, Status: api.JobStatusScheduled}}, nil
		},
	}

	e := &renovateExecutor{
		client:  c,
		scheme:  scheme,
		logger:  testLogger,
		policy:  gatePolicy(),
		manager: mgr,
	}

	suspended := policyJob("suspended", "")
	suspended.Spec.Suspend = new(true)
	active := policyJob("active", "")

	candidates := e.acceptedCandidates(context.Background(), []api.RenovateJob{suspended, active})

	if len(candidates) != 1 {
		t.Fatalf("expected only the active job's project to be a candidate, got %d", len(candidates))
	}
	if candidates[0].project.Name != "org/active" {
		t.Errorf("expected org/active to be dispatched, got %s", candidates[0].project.Name)
	}
}
