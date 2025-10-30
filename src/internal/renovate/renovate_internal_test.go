package renovate

import (
	"os"
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/config"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetJobTimeoutSeconds_DefaultAndEnv(t *testing.T) {
	if err := os.Unsetenv("JOB_TIMEOUT_SECONDS"); err != nil {
		t.Fatalf("failed to unset env: %v", err)
	}
	if err := os.Unsetenv("JOB_TIMEOUT_SECONDS"); err != nil {
		t.Fatalf("failed to unset env: %v", err)
	}
	// initialize config module with default description used by getJobTimeoutSeconds
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "1800"},
	})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}
	if v := getJobTimeoutSeconds(); v == nil || *v != int64(1800) {
		t.Fatalf("expected default timeout 1800, got %v", v)
	}

	if err := os.Setenv("JOB_TIMEOUT_SECONDS", "600"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	// re-init so config reads env
	_ = config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "JOB_TIMEOUT_SECONDS", Optional: true, Default: "1800"},
	})

	if v := getJobTimeoutSeconds(); v == nil || *v != int64(600) {
		t.Fatalf("expected timeout 600 from env, got %v", v)
	}
}

func TestGetJobStatusAndDeleteJob(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithObjects(job).Build()

	status, err := getJobStatus("test-job", "default", cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != api.JobStatusCompleted {
		t.Fatalf("expected completed status, got %v", status)
	}

	if err := deleteJob("test-job", "default", cl); err != nil {
		t.Fatalf("unexpected error deleting job: %v", err)
	}
}
