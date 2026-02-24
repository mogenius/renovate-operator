package renovate

import (
	"fmt"
	api "renovate-operator/api/v1alpha1"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// get the renovateprojectstatus from an executing kubernetes job
// Also returns a human readable duration string
func getJobStatus(job *batchv1.Job) (api.RenovateProjectStatus, string, error) {
	if job == nil {
		return api.JobStatusFailed, "", nil
	}

	var status api.RenovateProjectStatus
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			status = api.JobStatusCompleted
			break
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			status = api.JobStatusFailed
			break
		}
	}
	if status == "" {
		status = api.JobStatusRunning
	}

	// Calculate duration
	var durationStr string
	if job.Status.StartTime != nil {
		var endTime = job.Status.CompletionTime
		if endTime == nil {
			// If not completed, use current time
			endTime = &v1.Time{Time: time.Now()}
		}
		duration := endTime.Sub(job.Status.StartTime.Time)
		durationStr = humanDuration(duration)
	}

	return status, durationStr, nil
}

// humanDuration returns a human readable duration string
func humanDuration(dur time.Duration) string {
	if dur.Hours() >= 1 {
		return fmt.Sprintf("%.0fh %.0fm %.0fs", dur.Hours(), dur.Minutes()-float64(int(dur.Hours())*60), dur.Seconds()-float64(int(dur.Minutes())*60))
	} else if dur.Minutes() >= 1 {
		return fmt.Sprintf("%.0fm %.0fs", dur.Minutes(), dur.Seconds()-float64(int(dur.Minutes())*60))
	}
	return fmt.Sprintf("%.0fs", dur.Seconds())
}
