package renovate

import (
	"context"
	"errors"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/metricStore"

	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

// The policy check comes first, so suspending a job never hides that it breaks the
// policy: the denial is still counted and logged.
func TestAcceptedCandidatesReportsPolicyDenialOfASuspendedJob(t *testing.T) {
	reg := prometheus.NewRegistry()
	metricStore.Register(reg)
	before := destinationDenials(t, reg)

	e := &renovateExecutor{logger: testLogger, policy: gatePolicy(), manager: &fakeJobManager{}}
	job := policyJob("refused", "https://attacker.example.net")
	job.Spec.Suspend = new(true)

	var logged []string
	logger := funcr.New(func(_, args string) { logged = append(logged, args) }, funcr.Options{})

	candidates := e.acceptedCandidates(log.IntoContext(context.Background(), logger), []api.RenovateJob{job})

	if len(candidates) != 0 {
		t.Fatalf("expected no candidate, got %d", len(candidates))
	}
	if got := destinationDenials(t, reg) - before; got != 1 {
		t.Errorf("expected one policy denial to be counted, got %v", got)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "refused by policy") {
		t.Errorf("expected the refusal to be logged, got %v", logged)
	}
}

// destinationDenials reads the destination policy denials counted so far.
func destinationDenials(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != "renovate_operator_policy_denials_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "check" && lp.GetValue() == "destination" {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}
