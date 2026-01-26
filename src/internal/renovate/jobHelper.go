package renovate

import (
	api "renovate-operator/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// get the renovateprojectstatus from an executing kubernetes job
func getJobStatus(job *batchv1.Job) (api.RenovateProjectStatus, error) {
	if job == nil {
		return api.JobStatusFailed, nil
	}
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return api.JobStatusCompleted, nil
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return api.JobStatusFailed, nil
		}
	}

	return api.JobStatusRunning, nil
}
