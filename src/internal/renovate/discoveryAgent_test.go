package renovate

import (
	"context"
	"fmt"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"
	crdManager "renovate-operator/internal/crdManager"
	"renovate-operator/internal/types"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeJobManager is a minimal RenovateJobManager for discoveryAgent tests.
type fakeJobManager struct {
	getJobFn                     func(ctx context.Context, name, namespace string) (*api.RenovateJob, error)
	reconcileProjectsFn          func(ctx context.Context, job *api.RenovateJob, projects []string) error
	updateProjectStatusBatchedFn func(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error
}

func (f *fakeJobManager) GetRenovateJob(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
	if f.getJobFn != nil {
		return f.getJobFn(ctx, name, namespace)
	}
	return &api.RenovateJob{}, nil
}
func (f *fakeJobManager) ReconcileProjects(ctx context.Context, job *api.RenovateJob, projects []string) error {
	if f.reconcileProjectsFn != nil {
		return f.reconcileProjectsFn(ctx, job, projects)
	}
	return nil
}
func (f *fakeJobManager) UpdateProjectStatusBatched(ctx context.Context, fn func(p api.ProjectStatus) bool, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	if f.updateProjectStatusBatchedFn != nil {
		return f.updateProjectStatusBatchedFn(ctx, fn, job, status)
	}
	return nil
}
func (f *fakeJobManager) ListRenovateJobs(ctx context.Context) ([]crdManager.RenovateJobIdentifier, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeJobManager) ListRenovateJobsFull(ctx context.Context) ([]api.RenovateJob, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeJobManager) GetProjectsForRenovateJob(ctx context.Context, job crdManager.RenovateJobIdentifier) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeJobManager) UpdateProjectStatus(ctx context.Context, project string, job crdManager.RenovateJobIdentifier, status *types.RenovateStatusUpdate) error {
	return fmt.Errorf("not implemented")
}
func (f *fakeJobManager) GetProjectsByStatus(ctx context.Context, job crdManager.RenovateJobIdentifier, status api.RenovateProjectStatus) ([]crdManager.RenovateProjectStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeJobManager) GetLogsForProject(ctx context.Context, job crdManager.RenovateJobIdentifier, project string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
func (f *fakeJobManager) IsWebhookTokenValid(ctx context.Context, job crdManager.RenovateJobIdentifier, token string) (bool, error) {
	return true, nil
}
func (f *fakeJobManager) IsWebhookSignatureValid(ctx context.Context, job crdManager.RenovateJobIdentifier, signature string, body []byte) (bool, error) {
	return true, nil
}
func (f *fakeJobManager) UpdateExecutionOptions(ctx context.Context, job crdManager.RenovateJobIdentifier, options *api.RenovateExecutionOptions) error {
	return nil
}
func (f *fakeJobManager) CancelProjectJob(ctx context.Context, project string, job crdManager.RenovateJobIdentifier) error {
	return nil
}

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
	running.Name = "job1-discovery-b6caabe5"
	running.Namespace = "ns"
	running.Labels = map[string]string{
		crdManager.JOB_LABEL_RENOVATEJOB: "job1",
		crdManager.JOB_LABEL_TYPE:        string(crdManager.DiscoveryJobType),
	}

	// failed job
	failed := &batchv1.Job{}
	failed.Name = "job2-discovery-2e2a0d0f"
	failed.Namespace = "ns"
	failed.Status.Failed = 1
	failed.Labels = map[string]string{
		crdManager.JOB_LABEL_RENOVATEJOB: "job2",
		crdManager.JOB_LABEL_TYPE:        string(crdManager.DiscoveryJobType),
	}
	// succeeded job
	succeeded := &batchv1.Job{}
	succeeded.Name = "job3-discovery-b42e63e1"
	succeeded.Namespace = "ns"
	succeeded.Status.Succeeded = 1
	succeeded.Labels = map[string]string{
		crdManager.JOB_LABEL_RENOVATEJOB: "job3",
		crdManager.JOB_LABEL_TYPE:        string(crdManager.DiscoveryJobType),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(running, failed, succeeded).Build()

	daIface := NewDiscoveryAgent(scheme, c, testLogger, nil)
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
			got, err := da.GetDiscoveryJobStatus(context.Background(), job, "")
			if err != nil {
				t.Fatalf("GetDiscoveryJobStatus error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCreateDiscoveryJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "1"},
	})

	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).Build()
	da := NewDiscoveryAgent(scheme, c, testLogger, nil).(*discoveryAgent)

	rj := &api.RenovateJob{}
	rj.Name = "job1"
	rj.Namespace = "ns"

	generation, err := da.CreateDiscoveryJob(context.Background(), *rj, false)
	if err != nil {
		t.Fatalf("CreateDiscoveryJob returned error: %v", err)
	}
	if generation == "" {
		t.Fatalf("expected non-empty generation")
	}

	// verify a k8s Job was actually created
	jobList := &batchv1.JobList{}
	if err := c.List(context.Background(), jobList); err != nil {
		t.Fatalf("listing jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Fatalf("expected 1 job created, got %d", len(jobList.Items))
	}
}

func TestProcessDiscoveryJobResult(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	var capturedProjects []string
	mgr := &fakeJobManager{
		getJobFn: func(ctx context.Context, name, namespace string) (*api.RenovateJob, error) {
			return &api.RenovateJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, nil
		},
		reconcileProjectsFn: func(ctx context.Context, job *api.RenovateJob, projects []string) error {
			capturedProjects = projects
			return nil
		},
	}

	da := NewDiscoveryAgent(scheme, c, testLogger, mgr).(*discoveryAgent)
	da.getDiscoveredProjectsFromJobLogsFn = func(ctx context.Context, c client.Client, job *batchv1.Job) ([]string, error) {
		return []string{"a", "b"}, nil
	}

	// succeeded k8s Job (getJobStatus checks Conditions, not Succeeded counter)
	k8sJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1-discovery-abc", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			},
		},
	}

	if err := da.ProcessDiscoveryJobResult(context.Background(), k8sJob, "job1", "ns"); err != nil {
		t.Fatalf("ProcessDiscoveryJobResult returned error: %v", err)
	}
	if len(capturedProjects) != 2 {
		t.Fatalf("expected 2 projects passed to ReconcileProjects, got %d", len(capturedProjects))
	}
}

func TestProcessDiscoveryJobResult_NilJob(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	da := NewDiscoveryAgent(scheme, c, testLogger, nil).(*discoveryAgent)

	if err := da.ProcessDiscoveryJobResult(context.Background(), nil, "job1", "ns"); err != nil {
		t.Fatalf("expected nil error for nil job, got: %v", err)
	}
}

func TestProcessDiscoveryJobResult_RunningJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	da := NewDiscoveryAgent(scheme, c, testLogger, nil).(*discoveryAgent)

	runningJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1-discovery-abc", Namespace: "ns"},
	}
	if err := da.ProcessDiscoveryJobResult(context.Background(), runningJob, "job1", "ns"); err != nil {
		t.Fatalf("expected nil error for running job, got: %v", err)
	}
}

func TestEnsureRedisURLSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "VALKEY_URL", Optional: true, Default: "redis://redis.svc.cluster.local:6379/0"},
		{Key: "VALKEY_HOST", Optional: true, Default: ""},
		{Key: "VALKEY_PORT", Optional: true, Default: "6379"},
		{Key: "VALKEY_PASSWORD", Optional: true, Default: ""},
	})

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	if err := ensureRedisURLSecret(context.Background(), cl, "ns"); err != nil {
		t.Fatalf("ensureRedisURLSecret returned error: %v", err)
	}

	secret := &corev1.Secret{}
	if err := cl.Get(context.Background(), client.ObjectKey{Name: redisURLSecretName, Namespace: "ns"}, secret); err != nil {
		t.Fatalf("expected secret to be created: %v", err)
	}
	if got := string(secret.Data["redis-url"]); got != "redis://redis.svc.cluster.local:6379/1" {
		t.Fatalf("expected redis-url secret data, got %q", got)
	}
}
