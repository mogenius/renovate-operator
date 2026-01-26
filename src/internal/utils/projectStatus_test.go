package utils

import (
	"testing"

	api "renovate-operator/api/v1alpha1"
)

func TestGetUpdateStatusForProject(t *testing.T) {
	tests := []struct {
		name           string
		currentStatus  api.RenovateProjectStatus
		desiredStatus  api.RenovateProjectStatus
		expectedStatus api.RenovateProjectStatus
	}{
		{
			name:           "Schedule from Running",
			currentStatus:  api.JobStatusRunning,
			desiredStatus:  api.JobStatusScheduled,
			expectedStatus: api.JobStatusRunning,
		},
		{
			name:           "Run from Scheduled",
			currentStatus:  api.JobStatusScheduled,
			desiredStatus:  api.JobStatusRunning,
			expectedStatus: api.JobStatusRunning,
		},
		{
			name:           "Complete from Running",
			currentStatus:  api.JobStatusRunning,
			desiredStatus:  api.JobStatusCompleted,
			expectedStatus: api.JobStatusCompleted,
		},
		{
			name:           "Complete from Scheduled",
			currentStatus:  api.JobStatusScheduled,
			desiredStatus:  api.JobStatusCompleted,
			expectedStatus: api.JobStatusScheduled,
		},
		{
			name:           "Fail from Running",
			currentStatus:  api.JobStatusRunning,
			desiredStatus:  api.JobStatusFailed,
			expectedStatus: api.JobStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := &api.ProjectStatus{
				Name:   "test-project",
				Status: tt.currentStatus,
			}
			result := GetUpdateStatusForProject(proj, tt.desiredStatus)
			if result == nil {
				t.Fatalf("resulting project status is nil for %s", tt.name)
			}
			if result.Status != tt.expectedStatus {
				t.Errorf("%s: expected status %v, got %v", tt.name, tt.expectedStatus, result.Status)
			}
		})
	}
}
