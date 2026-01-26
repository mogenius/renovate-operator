package renovate

import (
	"testing"

	api "renovate-operator/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestGetJobStatus(t *testing.T) {
	tests := []struct {
		name           string
		job            *batchv1.Job
		expectedStatus api.RenovateProjectStatus
	}{
		{
			name:           "nil job returns failed status",
			job:            nil,
			expectedStatus: api.JobStatusFailed,
		},
		{
			name: "job with no conditions is running",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			expectedStatus: api.JobStatusRunning,
		},
		{
			name: "job with complete condition true is completed",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: api.JobStatusCompleted,
		},
		{
			name: "job with complete condition false is running",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expectedStatus: api.JobStatusRunning,
		},
		{
			name: "job with failed condition true is failed",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: api.JobStatusFailed,
		},
		{
			name: "job with failed condition false is running",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expectedStatus: api.JobStatusRunning,
		},
		{
			name: "job with multiple conditions - complete takes precedence",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expectedStatus: api.JobStatusCompleted,
		},
		{
			name: "job with multiple conditions - failed when true",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   batchv1.JobFailed,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: api.JobStatusFailed,
		},
		{
			name: "job with unrelated conditions is running",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobSuspended,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedStatus: api.JobStatusRunning,
		},
		{
			name: "job with active pods is running",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Active:     1,
					Conditions: []batchv1.JobCondition{},
				},
			},
			expectedStatus: api.JobStatusRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := getJobStatus(tt.job)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if status != tt.expectedStatus {
				t.Errorf("expected status %v, got %v", tt.expectedStatus, status)
			}
		})
	}
}
