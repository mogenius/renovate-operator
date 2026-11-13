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
		// webhook, cron and annotation triggers carry no options; they must not turn a
		// queued one-off run (debug, custom branch, dry run) into a plain one
		if desiredStatus.ExecutionOptions != nil {
			projectStatus.ExecutionOptions = desiredStatus.ExecutionOptions
		}
		if desiredStatus.Priority > projectStatus.Priority {
			projectStatus.Priority = desiredStatus.Priority
		}
	} else if desiredStatus.Priority > projectStatus.Priority && desiredStatus.ExecutionOptions == nil {
		// A webhook or UI trigger for a project that is already running asks for a run
		// that sees what changed after this one started (a merge Renovate itself just made,
		// a checkbox ticked mid-run). Keep its priority as a pending follow-up, which
		// finishRun turns into a new queue entry when this run ends. The running project
		// itself is left untouched, LastTransition included. A one-off run with its own
		// options is not remembered: those options would otherwise outlive the request.
		projectStatus.Priority = desiredStatus.Priority
	}
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

// finishRun moves a Running project to its final status, or back to Scheduled when a
// trigger arrived during the run (a non-zero Priority, see validateProjectStatusScheduled),
// keeping that priority so the follow-up is dispatched ahead of the cron backlog.
func finishRun(projectStatus *api.RenovateProjectState, final api.RenovateProjectStatus) {
	if projectStatus.Status != api.JobStatusRunning {
		return
	}
	if projectStatus.Priority > 0 {
		projectStatus.Status = api.JobStatusScheduled
	} else {
		projectStatus.Status = final
	}
	projectStatus.LastTransition = metav1.Now()
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
	finishRun(projectStatus, api.JobStatusCompleted)
	projectStatus.Duration = desiredStatus.Duration
	updateRenovateResultStatus(projectStatus, desiredStatus.RenovateResultStatus)
	updatePRActivity(projectStatus, desiredStatus.PRActivity)
	updateLogIssues(projectStatus, desiredStatus.LogIssues)
	return projectStatus
}

func validateProjectStatusFailed(projectStatus *api.RenovateProjectState, desiredStatus *types.RenovateStatusUpdate) *api.RenovateProjectState {
	finishRun(projectStatus, api.JobStatusFailed)
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
