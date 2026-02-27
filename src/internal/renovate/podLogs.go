package renovate

import (
	"bufio"
	"bytes"
	context "context"
	"encoding/json"
	"fmt"
	"renovate-operator/clientProvider"
	"sort"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (e *discoveryAgent) getDiscoveredProjectsFromJobLogs(ctx context.Context, c client.Client, job *batchv1.Job) ([]string, error) {
	logs, err := e.getLatestSuccessfulPodLog(ctx, c, job)
	if err != nil {
		return []string{}, fmt.Errorf("failed to get logs for job %s: %w", job.Name, err)
	}

	discovered, err := parseDiscoveredProjects(logs)
	if err != nil {
		return []string{}, fmt.Errorf("failed to parse discovered projects from logs: %w", err)
	}

	if len(discovered) == 0 {
		return []string{}, nil
	}

	// Sort projects for consistency
	sort.Strings(discovered)

	return discovered, nil
}

// parseDiscoveredProjects extracts the JSON string array from discovery pod logs.
// It first tries to parse the entire log as JSON. If that fails (e.g. due to
// stderr output mixed into the logs), it scans line by line for a valid JSON array.
func parseDiscoveredProjects(logs string) ([]string, error) {
	// Fast path: try parsing the entire log as a JSON array
	discovered, err := parseLineForDiscoveredProjects(logs)
	if err == nil {
		return discovered, nil
	}

	// Fallback: scan line by line for a JSON array (handles stderr mixed into logs)
	scanner := bufio.NewScanner(strings.NewReader(logs))
	for scanner.Scan() {
		discovered, err = parseLineForDiscoveredProjects(scanner.Text())
		if err == nil {
			return discovered, nil
		}
	}

	return nil, fmt.Errorf("no valid JSON array found in discovery logs (%d bytes)", len(logs))
}

func parseLineForDiscoveredProjects(line string) ([]string, error) {
	line = strings.TrimSpace(line)

	if len(line) == 0 || line[0] != '[' {
		return nil, fmt.Errorf("line does not start with '[': %s", line)
	}

	// Fast path: try parsing as a plain string array
	var discovered []string
	if err := json.Unmarshal([]byte(line), &discovered); err == nil {
		return discovered, nil
	}

	// Fallback: parse as mixed array (elements can be strings or objects with "repository" field)
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}

	discovered = make([]string, 0, len(raw))
	for _, elem := range raw {
		var s string
		if err := json.Unmarshal(elem, &s); err == nil {
			discovered = append(discovered, s)
			continue
		}
		var obj struct {
			Repository string `json:"repository"`
		}
		if err := json.Unmarshal(elem, &obj); err == nil && obj.Repository != "" {
			discovered = append(discovered, obj.Repository)
			continue
		}
		return nil, fmt.Errorf("unsupported element in discovered projects array: %s", elem)
	}

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
		return "", fmt.Errorf("no successful pods found for job %s", job.Name)
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
	defer func() {
		_ = podLogs.Close()
	}()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(podLogs)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
