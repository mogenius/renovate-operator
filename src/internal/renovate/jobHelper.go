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
// Also returns a human readable duration string and the numeric duration.
// The numeric duration is zero when the job has no StartTime yet.
func getJobStatus(job *batchv1.Job) (api.RenovateProjectStatus, string, time.Duration, error) {
	if job == nil {
		return api.JobStatusFailed, "", 0, nil
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
	var duration time.Duration
	if job.Status.StartTime != nil {
		var endTime = job.Status.CompletionTime
		if endTime == nil {
			// If not completed, use current time
			endTime = &v1.Time{Time: time.Now()}
		}
		duration = endTime.Sub(job.Status.StartTime.Time)
		durationStr = humanDuration(duration)
	}

	return status, durationStr, duration, nil
}

// jobFailureReason derives a coarse failure reason for the job_failures metric from the
// k8s Job's JobFailed condition. A nil job means the job was not found.
// Returns one of: timeout, backoff_exceeded, job_not_found, pod_error, unknown.
func jobFailureReason(job *batchv1.Job) string {
	if job == nil {
		return "job_not_found"
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type != batchv1.JobFailed || condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Reason {
		case "DeadlineExceeded":
			return "timeout"
		case "BackoffLimitExceeded":
			return "backoff_exceeded"
		case "PodFailurePolicy":
			return "pod_error"
		}
		return "unknown"
	}
	return "unknown"
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
