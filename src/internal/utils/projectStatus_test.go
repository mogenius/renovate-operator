package utils

import (
	"testing"

	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/types"
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
			result := GetUpdateStatusForProject(proj, &types.RenovateStatusUpdate{Status: tt.desiredStatus})
			if result == nil {
				t.Fatalf("resulting project status is nil for %s", tt.name)
			}
			if result.Status != tt.expectedStatus {
				t.Errorf("%s: expected status %v, got %v", tt.name, tt.expectedStatus, result.Status)
			}
		})
	}
}

func TestGetUpdateStatusForProject_Priority(t *testing.T) {
	t.Run("Schedule with priority=1 sets priority", func(t *testing.T) {
		proj := &api.ProjectStatus{Name: "p", Status: api.JobStatusCompleted}
		result := GetUpdateStatusForProject(proj, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled, Priority: 1})
		if result.Priority != 1 {
			t.Errorf("expected priority 1, got %d", result.Priority)
		}
		if result.Status != api.JobStatusScheduled {
			t.Errorf("expected status scheduled, got %v", result.Status)
		}
	})

	t.Run("Schedule with priority=2 sets priority", func(t *testing.T) {
		proj := &api.ProjectStatus{Name: "p", Status: api.JobStatusCompleted}
		result := GetUpdateStatusForProject(proj, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled, Priority: 2})
		if result.Priority != 2 {
			t.Errorf("expected priority 2, got %d", result.Priority)
		}
	})

	t.Run("Transition to running resets priority to 0", func(t *testing.T) {
		proj := &api.ProjectStatus{Name: "p", Status: api.JobStatusScheduled, Priority: 2}
		result := GetUpdateStatusForProject(proj, &types.RenovateStatusUpdate{Status: api.JobStatusRunning})
		if result.Priority != 0 {
			t.Errorf("expected priority 0 after running transition, got %d", result.Priority)
		}
		if result.Status != api.JobStatusRunning {
			t.Errorf("expected status running, got %v", result.Status)
		}
	})

	t.Run("Re-scheduling with lower priority preserves existing higher priority", func(t *testing.T) {
		proj := &api.ProjectStatus{Name: "p", Status: api.JobStatusScheduled, Priority: 2}
		result := GetUpdateStatusForProject(proj, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled, Priority: 0})
		if result.Priority != 2 {
			t.Errorf("expected priority to remain 2, got %d", result.Priority)
		}
		if result.Status != api.JobStatusScheduled {
			t.Errorf("expected status to remain scheduled, got %v", result.Status)
		}
	})

	t.Run("Cannot schedule a running project, priority unchanged", func(t *testing.T) {
		proj := &api.ProjectStatus{Name: "p", Status: api.JobStatusRunning, Priority: 1}
		result := GetUpdateStatusForProject(proj, &types.RenovateStatusUpdate{Status: api.JobStatusScheduled, Priority: 2})
		if result.Status != api.JobStatusRunning {
			t.Errorf("expected status to remain running, got %v", result.Status)
		}
		if result.Priority != 1 {
			t.Errorf("expected priority to remain 1, got %d", result.Priority)
		}
	})
}
