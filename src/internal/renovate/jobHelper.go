package renovate

import (
	context "context"
	"fmt"

	api "renovate-operator/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// get the renovateprojectstatus from an executing kubernetes job
func getJobStatus(name string, namespace string, cient client.Client) (api.RenovateProjectStatus, error) {
	job := &batchv1.Job{}
	err := cient.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, job)
	if err != nil {
		if errors.IsNotFound(err) {
			return api.JobStatusFailed, nil
		}
		return api.JobStatusFailed, fmt.Errorf("failed to get job %s/%s: %w", namespace, name, err)
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

func deleteJob(name string, namespace string, cient client.Client) error {
	ctx := context.Background()

	job := &batchv1.Job{}
	err := cient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, job)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return cient.Delete(ctx, job)
}

// func isJobRunning(name string, namespace string, client client.Client) (bool, error) {
// 	status, err := getJobStatus(name, namespace, client)
// 	return status == JobStatusRunning, err
// }
// func isJobCompleted(name string, namespace string, client client.Client) (bool, error) {
// 	status, err := getJobStatus(name, namespace, client)
// 	return status == JobStatusCompleted, err
// }
