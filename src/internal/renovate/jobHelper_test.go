package renovate

import (
	"context"
	"testing"

	api "renovate-operator/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetJobStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}

	t.Run("job running", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

		status, err := getJobStatus("test-job", "test-ns", client)
		if err != nil {
			t.Fatalf("getJobStatus returned error: %v", err)
		}
		if status != api.JobStatusRunning {
			t.Errorf("expected status %v, got %v", api.JobStatusRunning, status)
		}
	})

	t.Run("job completed", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
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

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

		status, err := getJobStatus("test-job", "test-ns", client)
		if err != nil {
			t.Fatalf("getJobStatus returned error: %v", err)
		}
		if status != api.JobStatusCompleted {
			t.Errorf("expected status %v, got %v", api.JobStatusCompleted, status)
		}
	})

	t.Run("job failed", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

		status, err := getJobStatus("test-job", "test-ns", client)
		if err != nil {
			t.Fatalf("getJobStatus returned error: %v", err)
		}
		if status != api.JobStatusFailed {
			t.Errorf("expected status %v, got %v", api.JobStatusFailed, status)
		}
	})

	t.Run("job not found", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).Build()

		status, err := getJobStatus("non-existing", "test-ns", client)
		if err != nil {
			t.Error("getJobStatus should not return error for non-existing job")
		}
		if status != api.JobStatusFailed {
			t.Errorf("expected status %v for error case, got %v", api.JobStatusFailed, status)
		}
	})
}

func TestDeleteJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	t.Run("delete existing job", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "test-ns",
			},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

		err := deleteJob("test-job", "test-ns", client)
		if err != nil {
			t.Fatalf("deleteJob returned error: %v", err)
		}

		// Verify job was deleted
		var jobList batchv1.JobList
		err = client.List(context.Background(), &jobList)
		if err != nil {
			t.Fatalf("failed to list jobs: %v", err)
		}
		if len(jobList.Items) != 0 {
			t.Error("job should be deleted but still exists")
		}
	})

	t.Run("delete non-existing job", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).Build()

		err := deleteJob("non-existing", "test-ns", client)
		if err != nil {
			t.Errorf("deleteJob should not return error for non-existing job, got: %v", err)
		}
	})
}
