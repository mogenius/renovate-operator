package crdmanager

import (
	"context"
	"renovate-operator/config"
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
			Labels: map[string]string{
				JOB_LABEL_RENOVATEJOB: "test-job",
				JOB_LABEL_PROJECT:     "test-project",
				JOB_LABEL_TYPE:        string(ExecutorJobType),
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

	t.Run("existing job", func(t *testing.T) {
		got, err := GetJobByLabel(context.Background(), client, JobSelector{
			RenovateJobName: "test-job",
			JobType:         ExecutorJobType,
			Project:         "test-project",
			Namespace:       "test-ns",
		})
		if err != nil {
			t.Fatalf("GetJob returned error: %v", err)
		}
		if got.Name != "test-job" {
			t.Errorf("got job name %q, want %q", got.Name, "test-job")
		}
	})

	t.Run("non-existing job", func(t *testing.T) {
		_, err := GetJobByLabel(context.Background(), client, JobSelector{
			RenovateJobName: "non-existing",
			JobType:         ExecutorJobType,
			Namespace:       "test-ns",
		})
		if err == nil {
			t.Error("GetJob should return error for non-existing job")
		}
	})
}

func TestCreateJob(t *testing.T) {
	err := config.InitializeConfigModule([]config.ConfigItemDescription{
		{Key: "POD_LABEL_TEMPLATES", Optional: true, Default: "{}"},
	})
	if err != nil {
		t.Fatalf("expected to initialize config module without error, got %v", err)
	}

	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-job",
			Namespace: "test-ns",
			Labels: map[string]string{
				JOB_LABEL_RENOVATEJOB: "new-job",
				JOB_LABEL_TYPE:        string(ExecutorJobType),
			},
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

	_, err = CreateJobWithGeneration(context.Background(), client, job, JobSelector{
		RenovateJobName: "new-job",
		JobType:         ExecutorJobType,
		Namespace:       "test-ns",
	})
	if err != nil {
		t.Fatalf("CreateJob returned error: %v", err)
	}

	// Verify job was created
	got, err := GetJobByLabel(context.Background(), client, JobSelector{
		RenovateJobName: "new-job",
		JobType:         ExecutorJobType,
		Namespace:       "test-ns",
	})
	if err != nil {
		t.Fatalf("Failed to get created job: %v", err)
	}
	if got.Name != "new-job" {
		t.Errorf("got job name %q, want %q", got.Name, "new-job")
	}

	// Verify operator labels were propagated to pod template
	for _, labelKey := range []string{JOB_LABEL_TYPE, JOB_LABEL_RENOVATEJOB, JOB_LABEL_GENERATION} {
		jobVal, jobOk := got.Labels[labelKey]
		tmplVal, tmplOk := got.Spec.Template.Labels[labelKey]
		if !jobOk {
			t.Fatalf("expected job label %s to be set", labelKey)
		}
		if !tmplOk {
			t.Fatalf("expected pod template label %s to be set", labelKey)
		}
		if jobVal != tmplVal {
			t.Errorf("pod template label %s=%q does not match job label %s=%q", labelKey, tmplVal, labelKey, jobVal)
		}
	}
}

func TestCreateJobWithGeneration_PreservesExistingTemplateLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add batch scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "labeled-job",
			Namespace: "test-ns",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"custom-label": "custom-value",
					},
				},
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

	_, err := CreateJobWithGeneration(context.Background(), client, job, JobSelector{
		RenovateJobName: "labeled-job",
		JobType:         DiscoveryJobType,
		Namespace:       "test-ns",
	})
	if err != nil {
		t.Fatalf("CreateJob returned error: %v", err)
	}

	got, err := GetJobByLabel(context.Background(), client, JobSelector{
		RenovateJobName: "labeled-job",
		JobType:         DiscoveryJobType,
		Namespace:       "test-ns",
	})
	if err != nil {
		t.Fatalf("Failed to get created job: %v", err)
	}

	// Custom label must be preserved
	if got.Spec.Template.Labels["custom-label"] != "custom-value" {
		t.Errorf("expected custom-label to be preserved, got %q", got.Spec.Template.Labels["custom-label"])
	}
	// Operator labels must also be present on the pod template
	if got.Spec.Template.Labels[JOB_LABEL_TYPE] != string(DiscoveryJobType) {
		t.Errorf("expected pod template to have %s=%s", JOB_LABEL_TYPE, string(DiscoveryJobType))
	}
	if got.Spec.Template.Labels[JOB_LABEL_RENOVATEJOB] != "labeled-job" {
		t.Errorf("expected pod template to have %s=labeled-job", JOB_LABEL_RENOVATEJOB)
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
				JOB_LABEL_RENOVATEJOB: "test-job",
				JOB_LABEL_TYPE:        string(ExecutorJobType),
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
	_, err = GetJobByLabel(context.Background(), client, JobSelector{
		RenovateJobName: "test-job",
		JobType:         ExecutorJobType,
		Namespace:       "test-ns",
	})
	if err == nil {
		t.Error("Job should be deleted but still exists")
	}
}

func TestDeleteJobWithWait(t *testing.T) {
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
				JOB_LABEL_RENOVATEJOB: "test-job",
				JOB_LABEL_TYPE:        string(ExecutorJobType),
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
	_, err = GetJobByLabel(context.Background(), client, JobSelector{
		RenovateJobName: "test-job",
		JobType:         ExecutorJobType,
		Namespace:       "test-ns",
	})
	if err == nil {
		t.Error("Job should be deleted but still exists")
	}
}
