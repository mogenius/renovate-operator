package renovate

import (
	"context"
	"strings"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/policy"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func policyScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	return scheme
}

func createdJobs(t *testing.T, c client.Client) []batchv1.Job {
	t.Helper()
	list := &batchv1.JobList{}
	if err := c.List(context.Background(), list); err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	return list.Items
}

// gatePolicy permits what the fixtures below use, so each test exercises the one
// rule it is about.
func gatePolicy() policy.Policy {
	return policy.Policy{
		AllowedHosts: []string{"api.github.com"},
		// Listed in the spelling the fixtures use: matching is literal.
		AllowedImages: []string{"renovate/renovate"},
	}
}

func policyJob(name, endpoint string) api.RenovateJob {
	job := api.RenovateJob{}
	job.ObjectMeta = metav1.ObjectMeta{Name: name, Namespace: "default"}
	job.Spec = api.RenovateJobSpec{
		Schedule: "*/5 * * * *",
		Image:    "renovate/renovate:latest",
		Provider: &api.RenovateProvider{Name: "github", Endpoint: endpoint},
	}
	return job
}

// Discovery is reachable from the UI and from an annotation trigger, not only from
// the reconciler that already gates, so it refuses independently.
func TestCreateDiscoveryJobRefusesForeignEndpoint(t *testing.T) {
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	da := NewDiscoveryAgent(scheme, c, testLogger, nil, nil, gatePolicy())

	job := policyJob("job1", "https://attacker.example.net")
	_, err := da.CreateDiscoveryJob(context.Background(), job, DiscoveryJobOptions{})
	if err == nil {
		t.Fatal("expected discovery to be refused for a non-allowlisted endpoint")
	}
	if !strings.Contains(err.Error(), "attacker.example.net") {
		t.Errorf("expected the denied host in the error, got: %v", err)
	}

	if jobs := createdJobs(t, c); len(jobs) != 0 {
		t.Errorf("expected no Kubernetes Job to be created, got %d", len(jobs))
	}
}

// A refused job must not take its siblings down with it.
func TestDispatchScheduledSkipsOnlyTheRefusedJob(t *testing.T) {
	scheme := policyScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	e := &renovateExecutor{
		client: c,
		scheme: scheme,
		logger: testLogger,
		policy: gatePolicy(),
	}

	refused := policyJob("refused", "https://attacker.example.net")
	refused.Status.Projects = []api.ProjectStatus{{Name: "org/a", Status: api.JobStatusScheduled}}

	allowed := policyJob("allowed", "")
	allowed.Status.Projects = []api.ProjectStatus{{Name: "org/b", Status: api.JobStatusScheduled}}

	candidates := e.acceptedCandidates(context.Background(), []api.RenovateJob{refused, allowed})

	if len(candidates) != 1 {
		t.Fatalf("expected exactly the allowed job's project to be a candidate, got %d", len(candidates))
	}
	if candidates[0].project.Name != "org/b" {
		t.Errorf("expected org/b to survive, got %s", candidates[0].project.Name)
	}
}
