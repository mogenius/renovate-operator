package utils

import (
	api "renovate-operator/api/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetUpdateStatusForProject determines the new status for a project based on its current status and the desired status update.
func GetUpdateStatusForProject(projectStatus *api.ProjectStatus, desiredStatus api.RenovateProjectStatus) *api.ProjectStatus {
	switch desiredStatus {
	case api.JobStatusScheduled:
		return validateProjectStatusScheduled(projectStatus)
	case api.JobStatusRunning:
		return validateProjectStatusRunning(projectStatus)
	case api.JobStatusCompleted:
		return validateProjectStatusCompleted(projectStatus)
	case api.JobStatusFailed:
		return validateProjectStatusFailed(projectStatus)
	default:
		return projectStatus
	}
}

func validateProjectStatusScheduled(projectStatus *api.ProjectStatus) *api.ProjectStatus {
	// cannot schedule a project that is currently running
	if projectStatus.Status != api.JobStatusRunning {
		projectStatus.Status = api.JobStatusScheduled
	}
	return projectStatus
}

func validateProjectStatusRunning(projectStatus *api.ProjectStatus) *api.ProjectStatus {
	// can only set a project to running if it is currently scheduled
	if projectStatus.Status == api.JobStatusScheduled {
		projectStatus.Status = api.JobStatusRunning
	}
	return projectStatus
}

func validateProjectStatusCompleted(projectStatus *api.ProjectStatus) *api.ProjectStatus {
	// can only set a running project to completed
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusCompleted
		projectStatus.LastRun = v1.Now()
	}
	return projectStatus
}
func validateProjectStatusFailed(projectStatus *api.ProjectStatus) *api.ProjectStatus {
	// can only set a running project to failed
	if projectStatus.Status == api.JobStatusRunning {
		projectStatus.Status = api.JobStatusFailed
		projectStatus.LastRun = v1.Now()
	}
	return projectStatus
}
