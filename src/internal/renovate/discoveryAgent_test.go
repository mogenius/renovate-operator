package renovate

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// minimal logger for tests
var testLogger = logr.Discard()

func TestGetDiscoveryJobStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	// running job
	running := &batchv1.Job{}
	running.Name = "job1-discovery"
	running.Namespace = "ns"

	// failed job
	failed := &batchv1.Job{}
	failed.Name = "job2-discovery"
	failed.Namespace = "ns"
	failed.Status.Failed = 1

	// succeeded job
	succeeded := &batchv1.Job{}
	succeeded.Name = "job3-discovery"
	succeeded.Namespace = "ns"
	succeeded.Status.Succeeded = 1

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(running, failed, succeeded).Build()

	daIface := NewDiscoveryAgent(scheme, c, testLogger)
	da := daIface.(*discoveryAgent)

	tests := []struct {
		name    string
		jobName string
		want    api.RenovateProjectStatus
	}{
		{"running", "job1", api.JobStatusRunning},
		{"failed", "job2", api.JobStatusFailed},
		{"succeeded", "job3", api.JobStatusCompleted},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := &api.RenovateJob{}
			job.Name = tc.jobName
			job.Namespace = "ns"
			got, err := da.GetDiscoveryJobStatus(context.Background(), job)
			if err != nil {
				t.Fatalf("GetDiscoveryJobStatus error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCreateDiscoveryJobAndWait(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	// initialize minimal config used by newDiscoveryJob
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "1"},
	})

	// start with no jobs - enable status subresource for Job
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).Build()

	da := NewDiscoveryAgent(scheme, c, testLogger).(*discoveryAgent)

	// override log extraction to return a deterministic list
	da.getDiscoveredProjectsFromJobLogsFn = func(ctx context.Context, c client.Client, job *batchv1.Job) ([]string, error) {
		return []string{"a", "b"}, nil
	}

	// override status check to return completed immediately
	da.getDiscoveryJobStatusFn = func(ctx context.Context, job *api.RenovateJob) (api.RenovateProjectStatus, error) {
		// Return completed on first call to simulate job completion
		return api.JobStatusCompleted, nil
	}

	// create a RenovateJob and run Discover -> should create the job and return discovered projects
	rj := &api.RenovateJob{}
	rj.Name = "myjob"
	rj.Namespace = "ns"

	projects, err := da.Discover(context.Background(), rj)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// ensure job exists in fake client
	got := &batchv1.Job{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "myjob-discovery", Namespace: "ns"}, got); err != nil {
		t.Fatalf("expected discovery job created: %v", err)
	}
}
