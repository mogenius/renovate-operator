package crdmanager

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	// Create a test job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-ns",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

	t.Run("existing job", func(t *testing.T) {
		got, err := GetJob(context.Background(), client, "test-job", "test-ns")
		if err != nil {
			t.Fatalf("GetJob returned error: %v", err)
		}
		if got.Name != "test-job" {
			t.Errorf("got job name %q, want %q", got.Name, "test-job")
		}
	})

	t.Run("non-existing job", func(t *testing.T) {
		_, err := GetJob(context.Background(), client, "non-existing", "test-ns")
		if err == nil {
			t.Error("GetJob should return error for non-existing job")
		}
	})
}

func TestCreateJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-job",
			Namespace: "test-ns",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "test:latest",
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	err := CreateJob(context.Background(), client, job)
	if err != nil {
		t.Fatalf("CreateJob returned error: %v", err)
	}

	// Verify job was created
	got, err := GetJob(context.Background(), client, "new-job", "test-ns")
	if err != nil {
		t.Fatalf("Failed to get created job: %v", err)
	}
	if got.Name != "new-job" {
		t.Errorf("got job name %q, want %q", got.Name, "new-job")
	}
}

func TestDeleteJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	// Create a test job with a pod
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-ns",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
			Labels: map[string]string{
				"job-name": "test-job",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test",
					Image: "test:latest",
				},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, pod).Build()

	err := DeleteJob(context.Background(), client, job)
	if err != nil {
		t.Fatalf("DeleteJob returned error: %v", err)
	}

	// Verify job was deleted
	_, err = GetJob(context.Background(), client, "test-job", "test-ns")
	if err == nil {
		t.Error("Job should be deleted but still exists")
	}
}
