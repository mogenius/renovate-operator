package renovate

import (
	"bytes"
	context "context"
	"encoding/json"
	"fmt"
	"renovate-operator/clientProvider"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (e *discoveryAgent) getDiscoveredProjectsFromJobLogs(ctx context.Context, c client.Client, job *batchv1.Job) ([]string, error) {
	logs, err := e.getLatestSuccessfulPodLog(ctx, c, job)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get logs for job %s: %w", job.Name, err)
	}

	var discovered []string
	if err := json.Unmarshal([]byte(logs), &discovered); err != nil {
		return []string{}, fmt.Errorf("failed to parse discovered projects from logs: %w", err)
	}

	if len(discovered) == 0 {
		return []string{}, nil
	}

	// Sort projects for consistency
	sort.Strings(discovered)

	return discovered, nil
}

// getLatestSuccessfulPodLog fetches the logs from the latest successful pod for a job
func (e *discoveryAgent) getLatestSuccessfulPodLog(ctx context.Context, c client.Client, job *batchv1.Job) (string, error) {
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(job.Namespace), client.MatchingLabels{"job-name": job.Name}); err != nil {
		return "", err
	}

	// Filter successful pods
	var succeededPods []corev1.Pod
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded && pod.Status.StartTime != nil {
			succeededPods = append(succeededPods, pod)
		}
	}
	if len(succeededPods) == 0 {
		return "", nil // No successful pods yet
	}

	// Sort by StartTime descending (latest first)
	sort.Slice(succeededPods, func(i, j int) bool {
		return succeededPods[i].Status.StartTime.After(succeededPods[j].Status.StartTime.Time)
	})
	latestPod := succeededPods[0]

	cp := clientProvider.StaticClientProvider()
	clientset, err := cp.K8sClientSet()
	if err != nil {
		return "", fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	req := clientset.CoreV1().Pods(latestPod.Namespace).GetLogs(latestPod.Name, &corev1.PodLogOptions{})
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer podLogs.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
