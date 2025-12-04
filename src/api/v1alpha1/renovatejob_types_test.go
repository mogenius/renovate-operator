package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenovateJob_Fullname(t *testing.T) {
	tests := []struct {
		name      string
		jobName   string
		namespace string
		want      string
	}{
		{
			name:      "simple names",
			jobName:   "my-job",
			namespace: "default",
			want:      "my-job-default",
		},
		{
			name:      "with dashes",
			jobName:   "my-renovate-job",
			namespace: "my-namespace",
			want:      "my-renovate-job-my-namespace",
		},
		{
			name:      "empty namespace",
			jobName:   "job",
			namespace: "",
			want:      "job-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rj := &RenovateJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.jobName,
					Namespace: tt.namespace,
				},
			}

			got := rj.Fullname()
			if got != tt.want {
				t.Errorf("Fullname() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenovateJob_DeepCopyObject(t *testing.T) {
	t.Run("non-nil object", func(t *testing.T) {
		original := &RenovateJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "default",
			},
			Spec: RenovateJobSpec{
				Schedule:    "0 0 * * *",
				Parallelism: 2,
			},
		}

		copied := original.DeepCopyObject()
		if copied == nil {
			t.Fatal("DeepCopyObject() returned nil")
		}

		copiedJob, ok := copied.(*RenovateJob)
		if !ok {
			t.Fatal("DeepCopyObject() did not return *RenovateJob")
		}

		if copiedJob.Name != original.Name {
			t.Errorf("Name not copied correctly: got %v, want %v", copiedJob.Name, original.Name)
		}
		if copiedJob.Namespace != original.Namespace {
			t.Errorf("Namespace not copied correctly: got %v, want %v", copiedJob.Namespace, original.Namespace)
		}
		if copiedJob.Spec.Schedule != original.Spec.Schedule {
			t.Errorf("Spec.Schedule not copied correctly: got %v, want %v", copiedJob.Spec.Schedule, original.Spec.Schedule)
		}
	})

	t.Run("nil object", func(t *testing.T) {
		var original *RenovateJob = nil
		copied := original.DeepCopyObject()
		if copied != nil {
			t.Error("DeepCopyObject() should return nil for nil input")
		}
	})
}

func TestRenovateProjectStatus_Constants(t *testing.T) {
	// Verify that the constants have the expected values
	tests := []struct {
		status RenovateProjectStatus
		want   string
	}{
		{JobStatusScheduled, "scheduled"},
		{JobStatusRunning, "running"},
		{JobStatusCompleted, "completed"},
		{JobStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.want {
				t.Errorf("constant value = %v, want %v", tt.status, tt.want)
			}
		})
	}
}
