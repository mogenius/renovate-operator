package utils

import (
	api "renovate-operator/api/v1alpha1"
)

// GetUpdateStatusForProject determines the new status for a project based on its current status and the desired status update.
func GetUpdateStatusForProject(currentStatus, desiredStatus api.RenovateProjectStatus) api.RenovateProjectStatus {
	switch desiredStatus {
	case api.JobStatusScheduled:
		return validateProjectStatusScheduled(currentStatus)
	case api.JobStatusRunning:
		return validateProjectStatusRunning(currentStatus)
	case api.JobStatusCompleted:
		return validateProjectStatusCompleted(currentStatus)
	case api.JobStatusFailed:
		return validateProjectStatusFailed(currentStatus)
	default:
		return desiredStatus
	}
}

func validateProjectStatusScheduled(currentStatus api.RenovateProjectStatus) api.RenovateProjectStatus {
	// cannot schedule a project that is currently running
	if currentStatus == api.JobStatusRunning {
		return api.JobStatusRunning
	}
	return api.JobStatusScheduled
}

func validateProjectStatusRunning(currentStatus api.RenovateProjectStatus) api.RenovateProjectStatus {
	// can only set a project to running if it is currently scheduled
	if currentStatus == api.JobStatusScheduled {
		return api.JobStatusRunning
	}
	return currentStatus
}

func validateProjectStatusCompleted(currentStatus api.RenovateProjectStatus) api.RenovateProjectStatus {
	// can only set a running project to completed
	if currentStatus == api.JobStatusRunning {
		return api.JobStatusCompleted
	}
	return currentStatus
}
func validateProjectStatusFailed(currentStatus api.RenovateProjectStatus) api.RenovateProjectStatus {
	// can only set a running project to failed
	if currentStatus == api.JobStatusRunning {
		return api.JobStatusFailed
	}
	return currentStatus
}
