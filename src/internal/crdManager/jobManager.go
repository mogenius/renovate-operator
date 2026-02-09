package crdmanager

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	JOB_LABEL_TYPE       = "renovate-operator.mogenius.com/job-type"
	JOB_LABEL_NAME       = "renovate-operator.mogenius.com/job-name"
	JOB_LABEL_GENERATION = "renovate-operator.mogenius.com/generation"
)

type JobType string

const (
	DiscoveryJobType JobType = "discovery"
	ExecutorJobType  JobType = "executor"
)

type JobSelector struct {
	JobName   string
	JobType   JobType
	Namespace string
}

// GetJobByLabel retrieves a single job matching the given labels.
// Returns an error if no job is found.
// If multiple jobs match, the most recently created one is returned.
func GetJobByLabel(ctx context.Context, client crclient.Client, selector JobSelector) (*batchv1.Job, error) {
	allJobs, err := GetJobsByLabel(ctx, client, selector)
	if err != nil {
		return nil, err
	}
	if len(allJobs) == 0 {
		return nil, errors.NewNotFound(batchv1.Resource("jobs"), selector.JobName)
	}
	// get the newest job in case there are multiple jobs for the same project (e.g. due to multiple executions)
	var currentJob *batchv1.Job
	var maxGen int64 = -1

	for i := range allJobs {
		genStr, exists := allJobs[i].Labels[JOB_LABEL_GENERATION]
		var gen int64 = 0 // Default to 0 for missing/invalid labels
		if exists {
			parsedGen, err := strconv.ParseInt(genStr, 10, 64)
			if err == nil {
				gen = parsedGen
			}
		}
		// Always select a job, prefer highest generation
		if gen > maxGen || currentJob == nil {
			maxGen = gen
			currentJob = &allJobs[i]
		}
	}
	return currentJob, nil
}

// Retrieve all Jobs by our standard labels
func GetJobsByLabel(ctx context.Context, client crclient.Client, selector JobSelector) ([]batchv1.Job, error) {

	jobList := &batchv1.JobList{}
	err := client.List(ctx, jobList, crclient.InNamespace(selector.Namespace), crclient.MatchingLabels{
		JOB_LABEL_NAME: selector.JobName,
		JOB_LABEL_TYPE: string(selector.JobType),
	})
	if err != nil {
		return nil, fmt.Errorf("listing jobs with label %s: %w", selector.JobName, err)
	}
	return jobList.Items, nil
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
func CreateJobWithGeneration(ctx context.Context, client crclient.Client, job *batchv1.Job, selector JobSelector) error {
	generation := fmt.Sprintf("%d", time.Now().Unix())

	job.Labels[JOB_LABEL_GENERATION] = generation

	// Create immediately - no deletion needed first
	err := client.Create(ctx, job)
	if err != nil {
		return fmt.Errorf("creating job with generateName %s: %w", job.GenerateName, err)
	}

	go func() {
		// Create a background context with a timeout
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_ = cleanupOldGenerations(cleanupCtx, client, selector, generation)
	}()

	return nil
}

// Delete jobs that aren't the current generation
func cleanupOldGenerations(ctx context.Context, client crclient.Client, selector JobSelector, currentGen string) error {
	allJobs, err := GetJobsByLabel(ctx, client, selector)
	if err != nil {
		return err
	}

	for _, job := range allJobs {
		gen, exists := job.Labels[JOB_LABEL_GENERATION]

		if !exists || gen != currentGen {
			// This is an old generation - safe to delete
			_ = DeleteJob(ctx, client, &job)
		}
	}
	return nil
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
