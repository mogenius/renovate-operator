package utils

import (
	api "renovate-operator/api/v1alpha1"
	"renovate-operator/internal/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetUpdateStatusForProject(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
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

func validateProjectStatusScheduled(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
	if projectStatus.Status != api.JobStatusRunning {
		projectStatus.Status = api.JobStatusScheduled
		projectStatus.LastTransition = metav1.Now()
		projectStatus.ExecutionOptions = desiredStatus.ExecutionOptions
		if desiredStatus.Priority > projectStatus.Priority {
			projectStatus.Priority = desiredStatus.Priority
		}
	}
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusRunning(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
	if projectStatus.Status == api.JobStatusScheduled {
		projectStatus.Status = api.JobStatusRunning
		projectStatus.LastTransition = metav1.Now()
		projectStatus.Priority = 0
		projectStatus.ExecutionOptions = nil
	}
	projectStatus.Duration = nil
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusCompleted(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusCompleted
		projectStatus.Priority = 0
		projectStatus.LastTransition = metav1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusFailed(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusFailed
		projectStatus.Priority = 0
		projectStatus.LastTransition = metav1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusCancelled(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusCancelled
		projectStatus.Priority = 0
		projectStatus.LastTransition = metav1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func updateRenovateResultStatus(projectStatus *api.RenovateProjectState, status *string) {
	if status != nil {
		projectStatus.RenovateResultStatus = status
	}
}

func updatePRActivity(projectStatus *api.RenovateProjectState, activity *api.PRActivity) {
	if activity != nil {
		projectStatus.PRActivity = activity
	}
}

func updateLogIssues(projectStatus *api.RenovateProjectState, issues *api.LogIssues) {
	if issues != nil {
		projectStatus.LogIssues = issues
	}
}
