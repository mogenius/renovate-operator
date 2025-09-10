package crdmanager

import (
	"context"
	"fmt"
	"renovate-operator/internal/utils"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetJob(ctx context.Context, client crclient.Client, jobName string, namespace string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := utils.Retry(utils.DefaultRetryAttempts, utils.DefaultRetrySleep, func() error {
		return client.Get(ctx, types.NamespacedName{
			Name:      jobName,
			Namespace: namespace,
		}, job)
	})
	return job, err
}

func DeleteJob(ctx context.Context, client crclient.Client, job *batchv1.Job) error {

	return utils.Retry(utils.DefaultRetryAttempts, utils.DefaultRetrySleep, func() error {
		// First, delete all pods owned by the job
		var podList corev1.PodList
		err := client.List(
			ctx,
			&podList,
			crclient.InNamespace(job.Namespace),
			crclient.MatchingLabels{"job-name": job.Name},
		)
		if err != nil {
			fmt.Printf("failed to list pods for job %s: %v\n", job.Name, err)
		} else {
			for _, pod := range podList.Items {
				err := client.Delete(ctx, &pod)
				if err != nil {
					fmt.Printf("failed to delete pod %s: %v\n", pod.Name, err)
				}
			}
		}
		// Then, delete the job itself
		return client.Delete(ctx, job)
	})
}

func CreateJob(ctx context.Context, client crclient.Client, job *batchv1.Job) error {
	return utils.Retry(utils.DefaultRetryAttempts, utils.DefaultRetrySleep, func() error {
		return client.Create(ctx, job)
	})
}

func getLastJobLog(ctx context.Context, clientset kubernetes.Interface, job *batchv1.Job) (string, error) {
	ns := job.Namespace

	// Use Job's label selector
	selector := metav1.FormatLabelSelector(job.Spec.Selector)

	pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", fmt.Errorf("listing pods for job %s: %w", job.Name, err)
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for job %s", job.Name)
	}

	// Sort pods by creation timestamp (newest last)
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].CreationTimestamp.Time.Before(pods.Items[j].CreationTimestamp.Time)
	})

	// Last pod (most recent)
	lastPod := pods.Items[len(pods.Items)-1]

	// Get logs from first container (adjust if multiple containers)
	req := clientset.CoreV1().Pods(ns).GetLogs(lastPod.Name, &corev1.PodLogOptions{
		Container: lastPod.Spec.Containers[0].Name,
	})

	logs, err := req.Do(ctx).Raw()
	if err != nil {
		return "", fmt.Errorf("getting logs from pod %s: %w", lastPod.Name, err)
	}

	return string(logs), nil
}
