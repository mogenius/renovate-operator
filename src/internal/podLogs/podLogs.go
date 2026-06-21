package podLogs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodLogReader abstracts reading logs from Kubernetes pods for a given Job.
type PodLogReader interface {
	// GetLastJobLog returns the complete logs from the most recent pod of a job (any phase).
	GetLastJobLog(ctx context.Context, job *batchv1.Job) (string, error)
	// StreamJobLogs opens a streaming log connection to the most recent pod.
	// follow=true keeps the stream open until the container exits (running pod).
	StreamJobLogs(ctx context.Context, job *batchv1.Job, follow bool) (io.ReadCloser, error)
	// GetSucceededJobLog returns the complete logs from the most recent succeeded pod of a job.
	GetSucceededJobLog(ctx context.Context, job *batchv1.Job) (string, error)
}

type podLogReader struct {
	clientset kubernetes.Interface
}

func New(clientset kubernetes.Interface) PodLogReader {
	return &podLogReader{clientset: clientset}
}

// findMostRecentPod returns the most recent pod (by creation timestamp) for a Job using its label selector.
func (r *podLogReader) findMostRecentPod(ctx context.Context, job *batchv1.Job) (*corev1.Pod, error) {
	ns := job.Namespace
	selector := metav1.FormatLabelSelector(job.Spec.Selector)
	pods, err := r.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing pods for job %s: %w", job.Name, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for job %s", job.Name)
	}
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].CreationTimestamp.Time.Before(pods.Items[j].CreationTimestamp.Time)
	})
	return &pods.Items[len(pods.Items)-1], nil
}

func (r *podLogReader) GetLastJobLog(ctx context.Context, job *batchv1.Job) (string, error) {
	pod, err := r.findMostRecentPod(ctx, job)
	if err != nil {
		return "", err
	}
	req := r.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: pod.Spec.Containers[0].Name,
	})
	raw, err := req.Do(ctx).Raw()
	if err != nil {
		return "", fmt.Errorf("getting logs from pod %s: %w", pod.Name, err)
	}
	return string(raw), nil
}

func (r *podLogReader) StreamJobLogs(ctx context.Context, job *batchv1.Job, follow bool) (io.ReadCloser, error) {
	pod, err := r.findMostRecentPod(ctx, job)
	if err != nil {
		return nil, err
	}
	req := r.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: pod.Spec.Containers[0].Name,
		Follow:    follow,
	})
	return req.Stream(ctx)
}

func (r *podLogReader) GetSucceededJobLog(ctx context.Context, job *batchv1.Job) (string, error) {
	pods, err := r.clientset.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + job.Name,
	})
	if err != nil {
		return "", err
	}

	var succeeded []corev1.Pod
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodSucceeded && p.Status.StartTime != nil {
			succeeded = append(succeeded, p)
		}
	}
	if len(succeeded) == 0 {
		return "", fmt.Errorf("no successful pods found for job %s", job.Name)
	}

	sort.Slice(succeeded, func(i, j int) bool {
		return succeeded[i].Status.StartTime.After(succeeded[j].Status.StartTime.Time)
	})
	latest := succeeded[0]

	req := r.clientset.CoreV1().Pods(latest.Namespace).GetLogs(latest.Name, &corev1.PodLogOptions{})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = stream.Close() }()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(stream); err != nil {
		return "", err
	}
	return buf.String(), nil
}
