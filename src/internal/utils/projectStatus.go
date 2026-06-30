package utils

import (
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/types"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetUpdateStatusForProject determines the new status for a project based on its current status and the desired status update.
func GetUpdateStatusForProject(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	switch desiredStatus.Status {
	case api.JobStatusScheduled:
		return validateProjectStatusScheduled(projectStatus, desiredStatus)
	case api.JobStatusRunning:
		return validateProjectStatusRunning(projectStatus, desiredStatus)
	case api.JobStatusCompleted:
		return validateProjectStatusCompleted(projectStatus, desiredStatus)
	case api.JobStatusFailed:
		return validateProjectStatusFailed(projectStatus, desiredStatus)
	case api.JobStatusCancelled:
		return validateProjectStatusCancelled(projectStatus, desiredStatus)
	default:
		return projectStatus
	}
}

func validateProjectStatusScheduled(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// cannot schedule a project that is currently running
	if projectStatus.Status != api.JobStatusRunning {
		projectStatus.Status = api.JobStatusScheduled
		if desiredStatus.Priority > projectStatus.Priority {
			projectStatus.Priority = desiredStatus.Priority
		}
	}
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusRunning(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a project to running if it is currently scheduled
	if projectStatus.Status == api.JobStatusScheduled {
		projectStatus.Status = api.JobStatusRunning
		projectStatus.Priority = 0
	}
	projectStatus.Duration = nil
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusCompleted(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a running project to completed
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusCompleted
		projectStatus.Priority = 0
		projectStatus.LastRun = v1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusFailed(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a running project to failed
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusFailed
		projectStatus.Priority = 0
		projectStatus.LastRun = v1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusCancelled(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a running project to cancelled
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusCancelled
		projectStatus.Priority = 0
		projectStatus.LastRun = v1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func updateRenovateResultStatus(projectStatus *api.ProjectStatus, status *string) {
	if status != nil {
		projectStatus.RenovateResultStatus = status
	}
}

func updatePRActivity(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) {
	if desiredStatus.PRActivity == nil {
		return
	}
	projectStatus.PRActivity = desiredStatus.PRActivity
	projectStatus.ApprovalsNeededSince = desiredStatus.ApprovalsNeededSince
}

// NextApprovalsNeededSince returns the start of the current pending-approval streak:
// the previous timestamp while approvals remain, a fresh now when a streak begins, or nil when none are pending.
func NextApprovalsNeededSince(prev *v1.Time, needsApproval int, now v1.Time) *v1.Time {
	if needsApproval <= 0 {
		return nil
	}
	if prev != nil {
		return prev
	}
	return &now
}

func updateLogIssues(projectStatus *api.ProjectStatus, issues *api.LogIssues) {
	if issues != nil {
		projectStatus.LogIssues = issues
	}
}
