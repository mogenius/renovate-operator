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
	default:
		return projectStatus
	}
}

func validateProjectStatusScheduled(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// cannot schedule a project that is currently running
	if projectStatus.Status != api.JobStatusRunning {
		projectStatus.Status = api.JobStatusScheduled
	}
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	return projectStatus
}

func validateProjectStatusRunning(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a project to running if it is currently scheduled
	if projectStatus.Status == api.JobStatusScheduled {
		projectStatus.Status = api.JobStatusRunning
	}
	projectStatus.Duration = nil
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	return projectStatus
}

func validateProjectStatusCompleted(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a running project to completed
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusCompleted
		projectStatus.LastRun = v1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	return projectStatus
}
func validateProjectStatusFailed(projectStatus *api.ProjectStatus, desiredStatus *types.RenovateStatusUpdate) *api.ProjectStatus {
	// can only set a running project to failed
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusFailed
		projectStatus.LastRun = v1.Now()
	}
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	return projectStatus
}

func updateRenovateResultStatus(projectStatus *api.ProjectStatus, status *string) {
	if status != nil {
		projectStatus.RenovateResultStatus = status
	}
}
