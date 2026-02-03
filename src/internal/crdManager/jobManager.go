package crdmanager

import (
	"context"
	"fmt"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetJob(ctx context.Context, client crclient.Client, jobName string, namespace string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: namespace,
	}, job)
	return job, err
}

func DeleteJobAndWaitForDeletion(ctx context.Context, client crclient.Client, job *batchv1.Job) error {
	jobName := job.Name

	policy := metav1.DeletePropagationForeground
	err := client.Delete(ctx, job, &crclient.DeleteOptions{
		PropagationPolicy: &policy})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete job %s: %w", jobName, err)
	}

	err = wait.PollUntilContextTimeout(
		ctx,
		200*time.Millisecond,
		3*time.Second,
		true,
		func(ctx context.Context) (bool, error) {
			_, err := GetJob(ctx, client, jobName, job.Namespace)
			if errors.IsNotFound(err) {
				return true, nil // job is gone
			}
			return false, err
		},
	)
	if err != nil {
		return fmt.Errorf("timed out waiting for job %s to be deleted", jobName)
	}
	return nil
}

func DeleteJob(ctx context.Context, client crclient.Client, job *batchv1.Job) error {
	policy := metav1.DeletePropagationBackground
	err := client.Delete(ctx, job, &crclient.DeleteOptions{
		PropagationPolicy: &policy})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete job %s: %w", job.Name, err)
	}
	return nil
}

func CreateJob(ctx context.Context, client crclient.Client, job *batchv1.Job) error {
	return client.Create(ctx, job)
}

// GetLastJobLog retrieves the logs from the most recent pod of a job
func GetLastJobLog(ctx context.Context, clientset kubernetes.Interface, job *batchv1.Job) (string, error) {
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
